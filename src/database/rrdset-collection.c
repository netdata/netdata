// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdset-collection.h"
#include "rrddim-collection.h"

time_t rrdset_set_update_every_s(RRDSET *st, time_t update_every_s) {
    if(unlikely(update_every_s == st->update_every))
        return st->update_every;

    internal_error(true, "RRDSET '%s' switching update every from %d to %d",
                   rrdset_id(st), (int)st->update_every, (int)update_every_s);

    time_t prev_update_every_s = (time_t) st->update_every;
    st->update_every = (int) update_every_s;

    // switch update every to the storage engine
    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        for (size_t tier = 0; tier < nd_profile.storage_tiers; tier++) {
            if (rd->tiers[tier].sch)
                storage_engine_store_change_collection_frequency(
                    rd->tiers[tier].sch,
                    (int)(st->rrdhost->db[tier].tier_grouping * st->update_every));
        }
    }
    rrddim_foreach_done(rd);

    return prev_update_every_s;
}

void rrdset_finalize_collection(RRDSET *st, bool dimensions_too) {
    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_TXT(NDF_NIDL_NODE, rrdhost_hostname(st->rrdhost)),
        ND_LOG_FIELD_TXT(NDF_NIDL_CONTEXT, rrdset_context(st)),
        ND_LOG_FIELD_TXT(NDF_NIDL_INSTANCE, rrdset_name(st)),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    RRDHOST *host = st->rrdhost;

    rrdset_flag_set(st, RRDSET_FLAG_COLLECTION_FINISHED);

    if(dimensions_too) {
        RRDDIM *rd;
        rrddim_foreach_read(rd, st)
            rrddim_finalize_collection_and_check_retention(rd);
        rrddim_foreach_done(rd);
    }

    for(size_t tier = 0; tier < nd_profile.storage_tiers; tier++) {
        STORAGE_ENGINE *eng = st->rrdhost->db[tier].eng;
        if(!eng) continue;

        if(st->smg[tier]) {
            storage_engine_metrics_group_release(eng->seb, host->db[tier].si, st->smg[tier]);
            st->smg[tier] = NULL;
        }
    }

    rrdset_pluginsd_receive_unslot_and_cleanup(st);
}

// ----------------------------------------------------------------------------
// RRDSET - reset a chart

static void rrdset_collection_reset(RRDSET *st) {
    netdata_log_debug(D_RRD_CALLS, "rrdset_collection_reset() %s", rrdset_name(st));

    st->last_collected_time.tv_sec = 0;
    st->last_collected_time.tv_usec = 0;
    st->last_updated.tv_sec = 0;
    st->last_updated.tv_usec = 0;
    st->db.current_entry = 0;
    st->counter = 0;
    st->counter_done = 0;

    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        rd->collector.last_collected_time.tv_sec = 0;
        rd->collector.last_collected_time.tv_usec = 0;
        rd->collector.counter = 0;

        if(!rrddim_flag_check(rd, RRDDIM_FLAG_ARCHIVED)) {
            for(size_t tier = 0; tier < nd_profile.storage_tiers;tier++)
                storage_engine_store_flush(rd->tiers[tier].sch);
        }
    }
    rrddim_foreach_done(rd);
}

// ----------------------------------------------------------------------------
// RRDSET - data collection iteration control

static inline void last_collected_time_align(RRDSET *st) {
    st->last_collected_time.tv_sec -= st->last_collected_time.tv_sec % st->update_every;

    if(unlikely(rrdset_flag_check(st, RRDSET_FLAG_STORE_FIRST)))
        st->last_collected_time.tv_usec = 0;
    else
        st->last_collected_time.tv_usec = 500000;
}

static inline void last_updated_time_align(RRDSET *st) {
    st->last_updated.tv_sec -= st->last_updated.tv_sec % st->update_every;
    st->last_updated.tv_usec = 0;
}

void rrdset_timed_next(RRDSET *st, struct timeval now, usec_t duration_since_last_update) {
#ifdef NETDATA_INTERNAL_CHECKS
    char *discard_reason = NULL;
    usec_t discarded = duration_since_last_update;
#endif

    if(unlikely(rrdset_flag_check(st, RRDSET_FLAG_SYNC_CLOCK))) {
        // the chart needs to be re-synced to current time
        rrdset_flag_clear(st, RRDSET_FLAG_SYNC_CLOCK);

        // discard the duration supplied
        duration_since_last_update = 0;

#ifdef NETDATA_INTERNAL_CHECKS
        if(!discard_reason) discard_reason = "SYNC CLOCK FLAG";
#endif
    }

    if(unlikely(!st->last_collected_time.tv_sec)) {
        // the first entry
        duration_since_last_update = st->update_every * USEC_PER_SEC;
#ifdef NETDATA_INTERNAL_CHECKS
        if(!discard_reason) discard_reason = "FIRST DATA COLLECTION";
#endif
    }
    else if(unlikely(!duration_since_last_update)) {
        // no dt given by the plugin
        duration_since_last_update = dt_usec(&now, &st->last_collected_time);
#ifdef NETDATA_INTERNAL_CHECKS
        if(!discard_reason) discard_reason = "NO USEC GIVEN BY COLLECTOR";
#endif
    }
    else {
        // microseconds has the time since the last collection
        susec_t since_last_usec = dt_usec_signed(&now, &st->last_collected_time);

        if(unlikely(since_last_usec < 0)) {
// oops! the database is in the future
#ifdef NETDATA_INTERNAL_CHECKS
            netdata_log_info("RRD database for chart '%s' on host '%s' is %0.5" NETDATA_DOUBLE_MODIFIER
                             " secs in the future (counter #%u, update #%u). Adjusting it to current time."
                             , rrdset_id(st)
                                 , rrdhost_hostname(st->rrdhost)
                                 , (NETDATA_DOUBLE)-since_last_usec / USEC_PER_SEC
                             , st->counter
                             , st->counter_done
            );
#endif

            duration_since_last_update = 0;

#ifdef NETDATA_INTERNAL_CHECKS
            if(!discard_reason) discard_reason = "COLLECTION TIME IN FUTURE";
#endif
        }
        else if(unlikely((usec_t)since_last_usec > (usec_t)(st->update_every * 5 * USEC_PER_SEC))) {
// oops! the database is too far behind
#ifdef NETDATA_INTERNAL_CHECKS
            netdata_log_info("RRD database for chart '%s' on host '%s' is %0.5" NETDATA_DOUBLE_MODIFIER
                             " secs in the past (counter #%u, update #%u). Adjusting it to current time.",
                             rrdset_id(st), rrdhost_hostname(st->rrdhost), (NETDATA_DOUBLE)since_last_usec / USEC_PER_SEC,
                             st->counter, st->counter_done);
#endif

            duration_since_last_update = (usec_t)since_last_usec;

#ifdef NETDATA_INTERNAL_CHECKS
            if(!discard_reason) discard_reason = "COLLECTION TIME TOO FAR IN THE PAST";
#endif
        }

#ifdef NETDATA_INTERNAL_CHECKS
        if(since_last_usec > 0 && (susec_t) duration_since_last_update < since_last_usec) {
            static __thread susec_t min_delta = USEC_PER_SEC * 3600, permanent_min_delta = 0;
            static __thread time_t last_time_s = 0;

            // the first time initialize it so that it will make the check later
            if(last_time_s == 0) last_time_s = now.tv_sec + 60;

            susec_t delta = since_last_usec - (susec_t) duration_since_last_update;
            if(delta < min_delta) min_delta = delta;

            if(now.tv_sec >= last_time_s + 60) {
                last_time_s = now.tv_sec;

                if(min_delta > permanent_min_delta) {
                    netdata_log_info("MINIMUM MICROSECONDS DELTA of thread %d increased from %"PRIi64" to %"PRIi64" (+%"PRIi64")", gettid_cached(), permanent_min_delta, min_delta, min_delta - permanent_min_delta);
                    permanent_min_delta = min_delta;
                }

                min_delta = USEC_PER_SEC * 3600;
            }
        }
#endif
    }

    netdata_log_debug(D_RRD_CALLS, "rrdset_timed_next() for chart %s with duration since last update %"PRIu64" usec", rrdset_name(st), duration_since_last_update);
    rrdset_debug(st, "NEXT: %"PRIu64" microseconds", duration_since_last_update);

    internal_error(discarded && discarded != duration_since_last_update,
                   "host '%s', chart '%s': discarded data collection time of %"PRIu64" usec, "
                   "replaced with %"PRIu64" usec, reason: '%s'"
                   , rrdhost_hostname(st->rrdhost)
                       , rrdset_id(st)
                       , discarded
                   , duration_since_last_update
                   , discard_reason?discard_reason:"UNDEFINED"
    );

    st->usec_since_last_update = duration_since_last_update;
}

inline void rrdset_next_usec_unfiltered(RRDSET *st, usec_t duration_since_last_update) {
    if(unlikely(!st->last_collected_time.tv_sec || !duration_since_last_update || (rrdset_flag_check(st, RRDSET_FLAG_SYNC_CLOCK)))) {
        // call the full next_usec() function
        rrdset_next_usec(st, duration_since_last_update);
        return;
    }

    st->usec_since_last_update = duration_since_last_update;
}

inline void rrdset_next_usec(RRDSET *st, usec_t duration_since_last_update) {
    struct timeval now;

    now_realtime_timeval(&now);
    rrdset_timed_next(st, now, duration_since_last_update);
}

// ----------------------------------------------------------------------------
// RRDSET - process the collected values for all dimensions of a chart

static inline usec_t rrdset_init_last_collected_time(RRDSET *st, struct timeval now) {
    st->last_collected_time = now;
    last_collected_time_align(st);

    usec_t last_collect_ut = st->last_collected_time.tv_sec * USEC_PER_SEC + st->last_collected_time.tv_usec;

    rrdset_debug(st, "initialized last collected time to %0.3" NETDATA_DOUBLE_MODIFIER, (NETDATA_DOUBLE)last_collect_ut / USEC_PER_SEC);

    return last_collect_ut;
}

static inline usec_t rrdset_update_last_collected_time(RRDSET *st) {
    usec_t last_collect_ut = st->last_collected_time.tv_sec * USEC_PER_SEC + st->last_collected_time.tv_usec;
    usec_t ut = last_collect_ut + st->usec_since_last_update;
    st->last_collected_time.tv_sec = (time_t) (ut / USEC_PER_SEC);
    st->last_collected_time.tv_usec = (suseconds_t) (ut % USEC_PER_SEC);

    rrdset_debug(st, "updated last collected time to %0.3" NETDATA_DOUBLE_MODIFIER, (NETDATA_DOUBLE)last_collect_ut / USEC_PER_SEC);

    return last_collect_ut;
}

static inline void rrdset_init_last_updated_time(RRDSET *st) {
    // copy the last collected time to last updated time
    st->last_updated.tv_sec  = st->last_collected_time.tv_sec;
    st->last_updated.tv_usec = st->last_collected_time.tv_usec;

    if(rrdset_flag_check(st, RRDSET_FLAG_STORE_FIRST))
        st->last_updated.tv_sec -= st->update_every;

    last_updated_time_align(st);
}

__thread size_t rrdset_done_statistics_points_stored_per_tier[RRD_STORAGE_TIERS];

// caching of dimensions rrdset_done() and rrdset_done_interpolate() loop through
struct rda_item {
    const DICTIONARY_ITEM *item;
    RRDDIM *rd;
    bool reset_or_overflow;
};

static __thread struct rda_item *thread_rda = NULL;
static __thread size_t thread_rda_entries = 0;

static struct rda_item *rrdset_thread_rda_get(size_t *dimensions) {

    if(unlikely(!thread_rda || (*dimensions) > thread_rda_entries)) {
        size_t old_mem = thread_rda_entries * sizeof(struct rda_item);
        freez(thread_rda);
        thread_rda_entries = *dimensions;
        size_t new_mem = thread_rda_entries * sizeof(struct rda_item);
        thread_rda = mallocz(new_mem);

        __atomic_add_fetch(&netdata_buffers_statistics.rrdset_done_rda_size, new_mem - old_mem, __ATOMIC_RELAXED);
    }

    *dimensions = thread_rda_entries;
    return thread_rda;
}

void rrdset_thread_rda_free(void) {
    __atomic_sub_fetch(&netdata_buffers_statistics.rrdset_done_rda_size, thread_rda_entries * sizeof(struct rda_item), __ATOMIC_RELAXED);

    freez(thread_rda);
    thread_rda = NULL;
    thread_rda_entries = 0;
}

static inline size_t rrdset_done_interpolate(
    RRDSET_STREAM_BUFFER *rsb
    , RRDSET *st
    , struct rda_item *rda_base
    , size_t rda_slots
    , usec_t update_every_ut
    , usec_t last_stored_ut
    , usec_t next_store_ut
    , usec_t last_collect_ut
    , usec_t now_collect_ut
    , char store_this_entry
) {
    RRDDIM *rd;

    size_t stored_entries = 0;     // the number of entries we have stored in the db, during this call to rrdset_done()

    usec_t first_ut = last_stored_ut, last_ut = 0;
    (void)first_ut;

    ssize_t iterations = (ssize_t)((now_collect_ut - last_stored_ut) / (update_every_ut));
    if((now_collect_ut % (update_every_ut)) == 0) iterations++;

    size_t counter = st->counter;
    long current_entry = st->db.current_entry;

    for( ; next_store_ut <= now_collect_ut ; last_collect_ut = next_store_ut, next_store_ut += update_every_ut, iterations-- ) {

        internal_error(iterations < 0,
                       "RRDSET: '%s': iterations calculation wrapped! "
                       "first_ut = %"PRIu64", last_stored_ut = %"PRIu64", next_store_ut = %"PRIu64", now_collect_ut = %"PRIu64""
                       , rrdset_id(st)
                           , first_ut
                       , last_stored_ut
                       , next_store_ut
                       , now_collect_ut
        );

        rrdset_debug(st, "last_stored_ut = %0.3" NETDATA_DOUBLE_MODIFIER " (last updated time)", (NETDATA_DOUBLE)last_stored_ut/USEC_PER_SEC);
        rrdset_debug(st, "next_store_ut  = %0.3" NETDATA_DOUBLE_MODIFIER " (next interpolation point)", (NETDATA_DOUBLE)next_store_ut/USEC_PER_SEC);

        last_ut = next_store_ut;

        ml_chart_update_begin(st);

        struct rda_item *rda;
        size_t dim_id;
        for(dim_id = 0, rda = rda_base ; dim_id < rda_slots ; ++dim_id, ++rda) {
            rd = rda->rd;
            if(unlikely(!rd)) continue;

            SN_FLAGS storage_flags = SN_DEFAULT_FLAGS;

            if (rda->reset_or_overflow)
                storage_flags |= SN_FLAG_RESET;

            NETDATA_DOUBLE new_value;

            switch(rd->algorithm) {
                case RRD_ALGORITHM_INCREMENTAL:
                    new_value = (NETDATA_DOUBLE)
                        (      rd->collector.calculated_value
                         * (NETDATA_DOUBLE)(next_store_ut - last_collect_ut)
                         / (NETDATA_DOUBLE)(now_collect_ut - last_collect_ut)
                        );

                    rrdset_debug(st, "%s: CALC2 INC " NETDATA_DOUBLE_FORMAT " = "
                                 NETDATA_DOUBLE_FORMAT
                                     " * (%"PRIu64" - %"PRIu64")"
                                     " / (%"PRIu64" - %"PRIu64""
                                 , rrddim_name(rd)
                                     , new_value
                                 , rd->collector.calculated_value
                                 , next_store_ut, last_collect_ut
                                 , now_collect_ut, last_collect_ut
                    );

                    rd->collector.calculated_value -= new_value;
                    new_value += rd->collector.last_calculated_value;
                    rd->collector.last_calculated_value = 0;
                    new_value /= (NETDATA_DOUBLE)st->update_every;

                    if(unlikely(next_store_ut - last_stored_ut < update_every_ut)) {

                        rrdset_debug(st, "%s: COLLECTION POINT IS SHORT " NETDATA_DOUBLE_FORMAT " - EXTRAPOLATING",
                                     rrddim_name(rd)
                                         , (NETDATA_DOUBLE)(next_store_ut - last_stored_ut)
                        );

                        new_value = new_value * (NETDATA_DOUBLE)(st->update_every * USEC_PER_SEC) / (NETDATA_DOUBLE)(next_store_ut - last_stored_ut);
                    }
                    break;

                case RRD_ALGORITHM_ABSOLUTE:
                case RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL:
                case RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL:
                default:
                    if(iterations == 1) {
                        // this is the last iteration
                        // do not interpolate
                        // just show the calculated value

                        new_value = rd->collector.calculated_value;
                    }
                    else {
                        // we have missed an update
                        // interpolate in the middle values

                        new_value = (NETDATA_DOUBLE)
                            (   (     (rd->collector.calculated_value - rd->collector.last_calculated_value)
                              * (NETDATA_DOUBLE)(next_store_ut - last_collect_ut)
                              / (NETDATA_DOUBLE)(now_collect_ut - last_collect_ut)
                                  )
                             +  rd->collector.last_calculated_value
                            );

                        rrdset_debug(st, "%s: CALC2 DEF " NETDATA_DOUBLE_FORMAT " = ((("
                                         "(" NETDATA_DOUBLE_FORMAT " - " NETDATA_DOUBLE_FORMAT ")"
                                         " * %"PRIu64""
                                         " / %"PRIu64") + " NETDATA_DOUBLE_FORMAT, rrddim_name(rd)
                                         , new_value
                                     , rd->collector.calculated_value, rd->collector.last_calculated_value
                                     , (next_store_ut - first_ut)
                                         , (now_collect_ut - first_ut), rd->collector.last_calculated_value
                        );
                    }
                    break;
            }

            time_t current_time_s = (time_t) (next_store_ut / USEC_PER_SEC);

            if(unlikely(!store_this_entry)) {
                (void) ml_dimension_is_anomalous(rd, current_time_s, 0, false);

                if(rsb->wb && rsb->v2)
                    stream_send_rrddim_metrics_v2(rsb, rd, next_store_ut, NAN, SN_FLAG_NONE);

                rrddim_store_metric(rd, next_store_ut, NAN, SN_FLAG_NONE);
                continue;
            }

            if(likely(rrddim_check_updated(rd) && rd->collector.counter > 1 && iterations < gap_when_lost_iterations_above)) {
                uint32_t dim_storage_flags = storage_flags;

                if (ml_dimension_is_anomalous(rd, current_time_s, new_value, true)) {
                    // clear anomaly bit: 0 -> is anomalous, 1 -> not anomalous
                    dim_storage_flags &= ~((storage_number)SN_FLAG_NOT_ANOMALOUS);
                }

                if(rsb->wb && rsb->v2)
                    stream_send_rrddim_metrics_v2(rsb, rd, next_store_ut, new_value, dim_storage_flags);

                rrddim_store_metric(rd, next_store_ut, new_value, dim_storage_flags);
                rd->collector.last_stored_value = new_value;
            }
            else {
                (void) ml_dimension_is_anomalous(rd, current_time_s, 0, false);

                rrdset_debug(st, "%s: STORE[%ld] = NON EXISTING ", rrddim_name(rd), current_entry);

                if(rsb->wb && rsb->v2)
                    stream_send_rrddim_metrics_v2(rsb, rd, next_store_ut, NAN, SN_FLAG_NONE);

                rrddim_store_metric(rd, next_store_ut, NAN, SN_FLAG_NONE);
                rd->collector.last_stored_value = NAN;
            }

            stored_entries++;
        }

        ml_chart_update_end(st);

        st->counter = ++counter;
        st->db.current_entry = current_entry = ((current_entry + 1) >= st->db.entries) ? 0 : current_entry + 1;

        st->last_updated.tv_sec = (time_t) (last_ut / USEC_PER_SEC);
        st->last_updated.tv_usec = 0;

        last_stored_ut = next_store_ut;
    }

    /*
    st->counter = counter;
    st->current_entry = current_entry;

    if(likely(last_ut)) {
        st->last_updated.tv_sec = (time_t) (last_ut / USEC_PER_SEC);
        st->last_updated.tv_usec = 0;
    }
*/

    return stored_entries;
}

void rrdset_done(RRDSET *st) {
    struct timeval now;

    now_realtime_timeval(&now);
    rrdset_timed_done(st, now, /* pending_rrdset_next = */ st->counter_done != 0);
}

void rrdset_timed_done(RRDSET *st, struct timeval now, bool pending_rrdset_next) {
    if(unlikely(!service_running(SERVICE_COLLECTORS))) return;

    RRDSET_STREAM_BUFFER stream_buffer = { .wb = NULL, };
    if(unlikely(rrdhost_has_stream_sender_enabled(st->rrdhost)))
        stream_buffer = stream_send_metrics_init(st, now.tv_sec);

    spinlock_lock(&st->data_collection_lock);

    if (pending_rrdset_next)
        rrdset_timed_next(st, now, 0ULL);

    netdata_log_debug(D_RRD_CALLS, "rrdset_done() for chart '%s'", rrdset_name(st));

    RRDDIM *rd;

    char
        store_this_entry = 1,   // boolean: 1 = store this entry, 0 = don't store this entry
        first_entry = 0;        // boolean: 1 = this is the first entry seen for this chart, 0 = all other entries

    usec_t
        last_collect_ut = 0,    // the timestamp in microseconds, of the last collected value
        now_collect_ut = 0,     // the timestamp in microseconds, of this collected value (this is NOW)
        last_stored_ut = 0,     // the timestamp in microseconds, of the last stored entry in the db
        next_store_ut = 0,      // the timestamp in microseconds, of the next entry to store in the db
        update_every_ut = st->update_every * USEC_PER_SEC; // st->update_every in microseconds

    RRDSET_FLAGS rrdset_flags = rrdset_flag_check(st, ~0);
    if(unlikely(rrdset_flags & RRDSET_FLAG_COLLECTION_FINISHED)) {
        spinlock_unlock(&st->data_collection_lock);
        return;
    }

    if (unlikely(rrdset_flags & RRDSET_FLAG_OBSOLETE)) {
        netdata_log_error("Chart '%s' has the OBSOLETE flag set, but it is collected.", rrdset_id(st));
        rrdset_isnot_obsolete___safe_from_collector_thread(st);
    }

    // check if the chart has a long time to be updated
    if(unlikely(st->usec_since_last_update > MAX(st->db.entries, 60) * update_every_ut)) {
        nd_log_daemon(NDLP_DEBUG, "host '%s', chart '%s': took too long to be updated (counter #%u, update #%u, %0.3" NETDATA_DOUBLE_MODIFIER
                                  " secs). Resetting it.", rrdhost_hostname(st->rrdhost), rrdset_id(st), st->counter, st->counter_done,
                      (NETDATA_DOUBLE)st->usec_since_last_update / USEC_PER_SEC);
        rrdset_collection_reset(st);
        st->usec_since_last_update = update_every_ut;
        store_this_entry = 0;
        first_entry = 1;
    }

    rrdset_debug(st, "microseconds since last update: %"PRIu64"", st->usec_since_last_update);

    // set last_collected_time
    if(unlikely(!st->last_collected_time.tv_sec)) {
        // it is the first entry
        // set the last_collected_time to now
        last_collect_ut = rrdset_init_last_collected_time(st, now) - update_every_ut;

        // the first entry should not be stored
        store_this_entry = 0;
        first_entry = 1;
    }
    else {
        // it is not the first entry
        // calculate the proper last_collected_time, using usec_since_last_update
        last_collect_ut = rrdset_update_last_collected_time(st);
    }

    // if this set has not been updated in the past
    // we fake the last_update time to be = now - usec_since_last_update
    if(unlikely(!st->last_updated.tv_sec)) {
        // it has never been updated before
        // set a fake last_updated, in the past using usec_since_last_update
        rrdset_init_last_updated_time(st);

        // the first entry should not be stored
        store_this_entry = 0;
        first_entry = 1;
    }

    // check if we will re-write the entire data set
    if(unlikely(dt_usec(&st->last_collected_time, &st->last_updated) > st->db.entries * update_every_ut &&
                 st->rrd_memory_mode != RRD_DB_MODE_DBENGINE)) {
        nd_log_daemon(NDLP_DEBUG, "'%s': too old data (last updated at %" PRId64 ".%" PRId64 ", last collected at %" PRId64 ".%" PRId64 "). "
                                  "Resetting it. Will not store the next entry.",
                      rrdset_id(st),
                      (int64_t)st->last_updated.tv_sec,
                      (int64_t)st->last_updated.tv_usec,
                      (int64_t)st->last_collected_time.tv_sec,
                      (int64_t)st->last_collected_time.tv_usec);
        rrdset_collection_reset(st);
        rrdset_init_last_updated_time(st);

        st->usec_since_last_update = update_every_ut;

        // the first entry should not be stored
        store_this_entry = 0;
        first_entry = 1;
    }

    // these are the 3 variables that will help us in interpolation
    // last_stored_ut = the last time we added a value to the storage
    // now_collect_ut = the time the current value has been collected
    // next_store_ut  = the time of the next interpolation point
    now_collect_ut = st->last_collected_time.tv_sec * USEC_PER_SEC + st->last_collected_time.tv_usec;
    last_stored_ut = st->last_updated.tv_sec * USEC_PER_SEC + st->last_updated.tv_usec;
    next_store_ut  = (st->last_updated.tv_sec + st->update_every) * USEC_PER_SEC;

    if(unlikely(!st->counter_done)) {
        // set a fake last_updated to jump to current time
        rrdset_init_last_updated_time(st);

        last_stored_ut = st->last_updated.tv_sec * USEC_PER_SEC + st->last_updated.tv_usec;
        next_store_ut  = (st->last_updated.tv_sec + st->update_every) * USEC_PER_SEC;

        if(unlikely(rrdset_flags & RRDSET_FLAG_STORE_FIRST)) {
            store_this_entry = 1;
            last_collect_ut = next_store_ut - update_every_ut;

            rrdset_debug(st, "Fixed first entry.");
        }
        else {
            store_this_entry = 0;

            rrdset_debug(st, "Will not store the next entry.");
        }
    }

    st->counter_done++;

    if(stream_buffer.wb && !stream_buffer.v2)
        stream_send_rrdset_metrics_v1(&stream_buffer, st);

    size_t rda_slots = dictionary_entries(st->rrddim_root_index);
    struct rda_item *rda_base = rrdset_thread_rda_get(&rda_slots);

    size_t dim_id;
    size_t dimensions = 0;
    struct rda_item *rda = rda_base;
    total_number collected_total = 0;
    total_number last_collected_total = 0;
    rrddim_foreach_read(rd, st) {
        if(rd_dfe.counter >= rda_slots)
            break;

        rda = &rda_base[dimensions++];

        if(rrddim_flag_check(rd, RRDDIM_FLAG_ARCHIVED)) {
            rda->item = NULL;
            rda->rd = NULL;
            rda->reset_or_overflow = false;
            continue;
        }

        // store the dimension in the array
        rda->item = dictionary_acquired_item_dup(st->rrddim_root_index, rd_dfe.item);
        rda->rd = dictionary_acquired_item_value(rda->item);
        rda->reset_or_overflow = false;

        // calculate totals
        if(likely(rrddim_check_updated(rd))) {
            // if the new is smaller than the old (an overflow, or reset), set the old equal to the new
            // to reset the calculation (it will give zero as the calculation for this second)
            if(unlikely(rd->algorithm == RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL && rd->collector.last_collected_value > rd->collector.collected_value)) {
                netdata_log_debug(D_RRD_STATS, "'%s' / '%s': RESET or OVERFLOW. Last collected value = " COLLECTED_NUMBER_FORMAT ", current = " COLLECTED_NUMBER_FORMAT
                                  , rrdset_id(st)
                                      , rrddim_name(rd)
                                      , rd->collector.last_collected_value
                                  , rd->collector.collected_value
                );

                if(!(rrddim_option_check(rd, RRDDIM_OPTION_DONT_DETECT_RESETS_OR_OVERFLOWS)))
                    rda->reset_or_overflow = true;

                rd->collector.last_collected_value = rd->collector.collected_value;
            }

            last_collected_total += rd->collector.last_collected_value;
            collected_total += rd->collector.collected_value;

            if(unlikely(rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE))) {
                netdata_log_error("Dimension %s in chart '%s' has the OBSOLETE flag set, but it is collected.", rrddim_name(rd), rrdset_id(st));
                rrddim_isnot_obsolete___safe_from_collector_thread(st, rd);
            }
        }
    }
    rrddim_foreach_done(rd);
    rda_slots = dimensions;

    rrdset_debug(st, "last_collect_ut = %0.3" NETDATA_DOUBLE_MODIFIER " (last collection time)", (NETDATA_DOUBLE)last_collect_ut/USEC_PER_SEC);
    rrdset_debug(st, "now_collect_ut  = %0.3" NETDATA_DOUBLE_MODIFIER " (current collection time)", (NETDATA_DOUBLE)now_collect_ut/USEC_PER_SEC);
    rrdset_debug(st, "last_stored_ut  = %0.3" NETDATA_DOUBLE_MODIFIER " (last updated time)", (NETDATA_DOUBLE)last_stored_ut/USEC_PER_SEC);
    rrdset_debug(st, "next_store_ut   = %0.3" NETDATA_DOUBLE_MODIFIER " (next interpolation point)", (NETDATA_DOUBLE)next_store_ut/USEC_PER_SEC);

    // process all dimensions to calculate their values
    // based on the collected figures only
    // at this stage we do not interpolate anything
    for(dim_id = 0, rda = rda_base ; dim_id < rda_slots ; ++dim_id, ++rda) {
        rd = rda->rd;
        if(unlikely(!rd)) continue;

        if(unlikely(!rrddim_check_updated(rd))) {
            rd->collector.calculated_value = 0;
            continue;
        }

        rrdset_debug(st, "%s: START "
                         " last_collected_value = " COLLECTED_NUMBER_FORMAT
                         " collected_value = " COLLECTED_NUMBER_FORMAT
                         " last_calculated_value = " NETDATA_DOUBLE_FORMAT
                         " calculated_value = " NETDATA_DOUBLE_FORMAT
                     , rrddim_name(rd)
                         , rd->collector.last_collected_value
                     , rd->collector.collected_value
                     , rd->collector.last_calculated_value
                     , rd->collector.calculated_value
        );

        switch(rd->algorithm) {
            case RRD_ALGORITHM_ABSOLUTE:
                rd->collector.calculated_value = (NETDATA_DOUBLE)rd->collector.collected_value
                                                 * (NETDATA_DOUBLE)rd->multiplier
                                                 / (NETDATA_DOUBLE)rd->divisor;

                rrdset_debug(st, "%s: CALC ABS/ABS-NO-IN " NETDATA_DOUBLE_FORMAT " = "
                             COLLECTED_NUMBER_FORMAT
                                 " * " NETDATA_DOUBLE_FORMAT
                                 " / " NETDATA_DOUBLE_FORMAT
                             , rrddim_name(rd)
                                 , rd->collector.calculated_value
                             , rd->collector.collected_value
                             , (NETDATA_DOUBLE)rd->multiplier
                             , (NETDATA_DOUBLE)rd->divisor
                );
                break;

            case RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL:
                if(unlikely(!collected_total))
                    rd->collector.calculated_value = 0;
                else
                    // the percentage of the current value
                    // over the total of all dimensions
                    rd->collector.calculated_value =
                        (NETDATA_DOUBLE)100
                        * (NETDATA_DOUBLE)rd->collector.collected_value
                        / (NETDATA_DOUBLE)collected_total;

                rrdset_debug(st, "%s: CALC PCENT-ROW " NETDATA_DOUBLE_FORMAT " = 100"
                                 " * " COLLECTED_NUMBER_FORMAT
                                 " / " COLLECTED_NUMBER_FORMAT
                             , rrddim_name(rd)
                                 , rd->collector.calculated_value
                             , rd->collector.collected_value
                             , collected_total
                );
                break;

            case RRD_ALGORITHM_INCREMENTAL:
                if(unlikely(rd->collector.counter <= 1)) {
                    rd->collector.calculated_value = 0;
                    continue;
                }

                // If the new is smaller than the old (an overflow, or reset), set the old equal to the new
                // to reset the calculation (it will give zero as the calculation for this second).
                // It is imperative to set the comparison to uint64_t since type collected_number is signed and
                // produces wrong results as far as incremental counters are concerned.
                if(unlikely((uint64_t)rd->collector.last_collected_value > (uint64_t)rd->collector.collected_value)) {
                    netdata_log_debug(D_RRD_STATS, "'%s' / '%s': RESET or OVERFLOW. Last collected value = " COLLECTED_NUMBER_FORMAT ", current = " COLLECTED_NUMBER_FORMAT
                                      , rrdset_id(st)
                                          , rrddim_name(rd)
                                          , rd->collector.last_collected_value
                                      , rd->collector.collected_value);

                    if(!(rrddim_option_check(rd, RRDDIM_OPTION_DONT_DETECT_RESETS_OR_OVERFLOWS)))
                        rda->reset_or_overflow = true;

                    uint64_t last = (uint64_t)rd->collector.last_collected_value;
                    uint64_t new = (uint64_t)rd->collector.collected_value;
                    uint64_t max = (uint64_t)rd->collector.collected_value_max;
                    uint64_t cap = 0;

                    // Signed values are handled by exploiting two's complement which will produce positive deltas
                    if (max > 0x00000000FFFFFFFFULL)
                        cap = 0xFFFFFFFFFFFFFFFFULL; // handles signed and unsigned 64-bit counters
                    else
                        cap = 0x00000000FFFFFFFFULL; // handles signed and unsigned 32-bit counters

                    uint64_t delta = cap - last + new;
                    uint64_t max_acceptable_rate = (cap / 100) * MAX_INCREMENTAL_PERCENT_RATE;

                    // If the delta is less than the maximum acceptable rate and the previous value was near the cap
                    // then this is an overflow. There can be false positives such that a reset is detected as an
                    // overflow.
                    // TODO: remember recent history of rates and compare with current rate to reduce this chance.
                    if (delta < max_acceptable_rate) {
                        rd->collector.calculated_value +=
                            (NETDATA_DOUBLE) delta
                            * (NETDATA_DOUBLE) rd->multiplier
                            / (NETDATA_DOUBLE) rd->divisor;
                    } else {
                        // This is a reset. Any overflow with a rate greater than MAX_INCREMENTAL_PERCENT_RATE will also
                        // be detected as a reset instead.
                        rd->collector.calculated_value += (NETDATA_DOUBLE)0;
                    }
                }
                else {
                    rd->collector.calculated_value +=
                        (NETDATA_DOUBLE) (rd->collector.collected_value - rd->collector.last_collected_value)
                        * (NETDATA_DOUBLE) rd->multiplier
                        / (NETDATA_DOUBLE) rd->divisor;
                }

                rrdset_debug(st, "%s: CALC INC PRE " NETDATA_DOUBLE_FORMAT " = ("
                             COLLECTED_NUMBER_FORMAT " - " COLLECTED_NUMBER_FORMAT
                                 ")"
                                 " * " NETDATA_DOUBLE_FORMAT
                                 " / " NETDATA_DOUBLE_FORMAT
                             , rrddim_name(rd)
                                 , rd->collector.calculated_value
                             , rd->collector.collected_value, rd->collector.last_collected_value
                             , (NETDATA_DOUBLE)rd->multiplier
                             , (NETDATA_DOUBLE)rd->divisor
                );
                break;

            case RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL:
                if(unlikely(rd->collector.counter <= 1)) {
                    rd->collector.calculated_value = 0;
                    continue;
                }

                // the percentage of the current increment
                // over the increment of all dimensions together
                if(unlikely(collected_total == last_collected_total))
                    rd->collector.calculated_value = 0;
                else
                    rd->collector.calculated_value =
                        (NETDATA_DOUBLE)100
                        * (NETDATA_DOUBLE)(rd->collector.collected_value - rd->collector.last_collected_value)
                        / (NETDATA_DOUBLE)(collected_total - last_collected_total);

                rrdset_debug(st, "%s: CALC PCENT-DIFF " NETDATA_DOUBLE_FORMAT " = 100"
                                 " * (" COLLECTED_NUMBER_FORMAT " - " COLLECTED_NUMBER_FORMAT ")"
                                 " / (" COLLECTED_NUMBER_FORMAT " - " COLLECTED_NUMBER_FORMAT ")"
                             , rrddim_name(rd)
                                 , rd->collector.calculated_value
                             , rd->collector.collected_value, rd->collector.last_collected_value
                             , collected_total, last_collected_total
                );
                break;

            default:
                // make the default zero, to make sure
                // it gets noticed when we add new types
                rd->collector.calculated_value = 0;

                rrdset_debug(st, "%s: CALC " NETDATA_DOUBLE_FORMAT " = 0"
                             , rrddim_name(rd)
                                 , rd->collector.calculated_value
                );
                break;
        }

        rrdset_debug(st, "%s: PHASE2 "
                         " last_collected_value = " COLLECTED_NUMBER_FORMAT
                         " collected_value = " COLLECTED_NUMBER_FORMAT
                         " last_calculated_value = " NETDATA_DOUBLE_FORMAT
                         " calculated_value = " NETDATA_DOUBLE_FORMAT
                     , rrddim_name(rd)
                         , rd->collector.last_collected_value
                     , rd->collector.collected_value
                     , rd->collector.last_calculated_value
                     , rd->collector.calculated_value
        );
    }

    // at this point we have all the calculated values ready
    // it is now time to interpolate values on a second boundary

    // #ifdef NETDATA_INTERNAL_CHECKS
    //     if(unlikely(now_collect_ut < next_store_ut && st->counter_done > 1)) {
    //         // this is collected in the same interpolation point
    //         rrdset_debug(st, "THIS IS IN THE SAME INTERPOLATION POINT");
    //         netdata_log_info("INTERNAL CHECK: host '%s', chart '%s' collection %zu is in the same interpolation point: short by %llu microseconds", st->rrdhost->hostname, rrdset_name(st), st->counter_done, next_store_ut - now_collect_ut);
    //     }
    // #endif

    rrdset_done_interpolate(
        &stream_buffer
        , st
        , rda_base
        , rda_slots
        , update_every_ut
        , last_stored_ut
        , next_store_ut
        , last_collect_ut
        , now_collect_ut
        , store_this_entry
    );

    for(dim_id = 0, rda = rda_base ; dim_id < rda_slots ; ++dim_id, ++rda) {
        rd = rda->rd;
        if(unlikely(!rd)) continue;

        if(unlikely(!rrddim_check_updated(rd)))
            continue;

        rrdset_debug(st, "%s: setting last_collected_value (old: " COLLECTED_NUMBER_FORMAT ") to last_collected_value (new: " COLLECTED_NUMBER_FORMAT ")", rrddim_name(rd), rd->collector.last_collected_value, rd->collector.collected_value);

        rd->collector.last_collected_value = rd->collector.collected_value;

        switch(rd->algorithm) {
            case RRD_ALGORITHM_INCREMENTAL:
                if(unlikely(!first_entry)) {
                    rrdset_debug(st, "%s: setting last_calculated_value (old: " NETDATA_DOUBLE_FORMAT ") to "
                                     "last_calculated_value (new: " NETDATA_DOUBLE_FORMAT ")"
                                 , rrddim_name(rd)
                                     , rd->collector.last_calculated_value + rd->collector.calculated_value
                                 , rd->collector.calculated_value);

                    rd->collector.last_calculated_value += rd->collector.calculated_value;
                }
                else {
                    rrdset_debug(st, "THIS IS THE FIRST POINT");
                }
                break;

            case RRD_ALGORITHM_ABSOLUTE:
            case RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL:
            case RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL:
                rrdset_debug(st, "%s: setting last_calculated_value (old: " NETDATA_DOUBLE_FORMAT ") to "
                                 "last_calculated_value (new: " NETDATA_DOUBLE_FORMAT ")"
                             , rrddim_name(rd)
                                 , rd->collector.last_calculated_value
                             , rd->collector.calculated_value);

                rd->collector.last_calculated_value = rd->collector.calculated_value;
                break;
        }

        rd->collector.calculated_value = 0;
        rd->collector.collected_value = 0;
        rrddim_clear_updated(rd);

        rrdset_debug(st, "%s: END "
                         " last_collected_value = " COLLECTED_NUMBER_FORMAT
                         " collected_value = " COLLECTED_NUMBER_FORMAT
                         " last_calculated_value = " NETDATA_DOUBLE_FORMAT
                         " calculated_value = " NETDATA_DOUBLE_FORMAT
                     , rrddim_name(rd)
                         , rd->collector.last_collected_value
                     , rd->collector.collected_value
                     , rd->collector.last_calculated_value
                     , rd->collector.calculated_value
        );
    }

    spinlock_unlock(&st->data_collection_lock);
    stream_send_rrdset_metrics_finished(&stream_buffer, st);

    // ALL DONE ABOUT THE DATA UPDATE
    // --------------------------------------------------------------------

    for(dim_id = 0, rda = rda_base; dim_id < rda_slots ; ++dim_id, ++rda) {
        rd = rda->rd;
        if(unlikely(!rd)) continue;

        dictionary_acquired_item_release(st->rrddim_root_index, rda->item);
        rda->item = NULL;
        rda->rd = NULL;
    }

    rrdcontext_collected_rrdset(st);

    store_metric_collection_completed();
}

// SPDX-License-Identifier: GPL-3.0-or-later

#include "replication.h"
#include "Judy.h"

#define MAX_SENDER_BUFFER_PERCENTAGE_ALLOWED 30
#define MIN_SENDER_BUFFER_PERCENTAGE_ALLOWED 10

static time_t replicate_chart_timeframe(BUFFER *wb, RRDSET *st, time_t after, time_t before, bool enable_streaming) {
    size_t dimensions = rrdset_number_of_dimensions(st);

    struct storage_engine_query_ops *ops = &st->rrdhost->db[0].eng->api.query_ops;

    struct {
        DICTIONARY *dict;
        const DICTIONARY_ITEM *rda;
        RRDDIM *rd;
        struct storage_engine_query_handle handle;
        STORAGE_POINT sp;
        bool enabled;
    } data[dimensions];

    memset(data, 0, sizeof(data));

    if(enable_streaming && st->last_updated.tv_sec > before) {
        internal_error(true, "REPLAY: '%s' overwriting replication before from %llu to %llu",
                       rrdset_id(st),
                       (unsigned long long)before,
                       (unsigned long long)st->last_updated.tv_sec
        );
        before = st->last_updated.tv_sec;
    }

    // prepare our array of dimensions
    {
        RRDDIM *rd;
        rrddim_foreach_read(rd, st) {
            if (rd_dfe.counter >= dimensions)
                break;

            if(rd->exposed) {
                data[rd_dfe.counter].dict = rd_dfe.dict;
                data[rd_dfe.counter].rda = dictionary_acquired_item_dup(rd_dfe.dict, rd_dfe.item);
                data[rd_dfe.counter].rd = rd;

                ops->init(rd->tiers[0]->db_metric_handle, &data[rd_dfe.counter].handle, after, before);

                data[rd_dfe.counter].enabled = true;
            }
            else
                data[rd_dfe.counter].enabled = false;
        }
        rrddim_foreach_done(rd);
    }

    time_t now = after + 1, actual_after = 0, actual_before = 0; (void)actual_before;
    while(now <= before) {
        time_t min_start_time = 0, min_end_time = 0;
        for (size_t i = 0; i < dimensions && data[i].rd; i++) {
            if(!data[i].enabled) continue;

            // fetch the first valid point for the dimension
            int max_skip = 100;
            while(data[i].sp.end_time < now && !ops->is_finished(&data[i].handle) && max_skip-- > 0)
                data[i].sp = ops->next_metric(&data[i].handle);

            internal_error(max_skip <= 0,
                           "REPLAY: host '%s', chart '%s', dimension '%s': db does not advance the query beyond time %llu",
                            rrdhost_hostname(st->rrdhost), rrdset_id(st), rrddim_id(data[i].rd), (unsigned long long) now);

            if(data[i].sp.end_time < now)
                continue;

            if(!min_start_time) {
                min_start_time = data[i].sp.start_time;
                min_end_time = data[i].sp.end_time;
            }
            else {
                min_start_time = MIN(min_start_time, data[i].sp.start_time);
                min_end_time = MIN(min_end_time, data[i].sp.end_time);
            }
        }

        time_t wall_clock_time = now_realtime_sec();
        if(min_start_time > wall_clock_time + 1 || min_end_time > wall_clock_time + 1) {
            internal_error(true,
                           "REPLAY: host '%s', chart '%s': db provided future start time %llu or end time %llu (now is %llu)",
                            rrdhost_hostname(st->rrdhost), rrdset_id(st),
                           (unsigned long long)min_start_time,
                           (unsigned long long)min_end_time,
                           (unsigned long long)wall_clock_time);
            break;
        }

        if(min_end_time < now) {
            internal_error(true,
                           "REPLAY: host '%s', chart '%s': no data on any dimension beyond time %llu",
                           rrdhost_hostname(st->rrdhost), rrdset_id(st), (unsigned long long)now);
            break;
        }

        if(min_end_time <= min_start_time)
            min_start_time = min_end_time - st->update_every;

        if(!actual_after) {
            actual_after = min_end_time;
            actual_before = min_end_time;
        }
        else
            actual_before = min_end_time;

        buffer_sprintf(wb, PLUGINSD_KEYWORD_REPLAY_BEGIN " '' %llu %llu %llu\n"
                       , (unsigned long long)min_start_time
                       , (unsigned long long)min_end_time
                       , (unsigned long long)wall_clock_time
                       );

        // output the replay values for this time
        for (size_t i = 0; i < dimensions && data[i].rd; i++) {
            if(!data[i].enabled) continue;

            if(data[i].sp.start_time <= min_end_time && data[i].sp.end_time >= min_end_time)
                buffer_sprintf(wb, PLUGINSD_KEYWORD_REPLAY_SET " \"%s\" " NETDATA_DOUBLE_FORMAT " \"%s\"\n",
                               rrddim_id(data[i].rd), data[i].sp.sum, data[i].sp.flags & SN_FLAG_RESET ? "R" : "");
            else
                buffer_sprintf(wb, PLUGINSD_KEYWORD_REPLAY_SET " \"%s\" NAN \"E\"\n",
                               rrddim_id(data[i].rd));
        }

        now = min_end_time + 1;
    }

#ifdef NETDATA_INTERNAL_CHECKS
    if(actual_after) {
        char actual_after_buf[LOG_DATE_LENGTH + 1], actual_before_buf[LOG_DATE_LENGTH + 1];
        log_date(actual_after_buf, LOG_DATE_LENGTH, actual_after);
        log_date(actual_before_buf, LOG_DATE_LENGTH, actual_before);
        internal_error(true,
                       "REPLAY: host '%s', chart '%s': sending data %llu [%s] to %llu [%s] (requested %llu [delta %lld] to %llu [delta %lld])",
                       rrdhost_hostname(st->rrdhost), rrdset_id(st),
                       (unsigned long long)actual_after, actual_after_buf, (unsigned long long)actual_before, actual_before_buf,
                       (unsigned long long)after, (long long)(actual_after - after), (unsigned long long)before, (long long)(actual_before - before));
    }
    else
        internal_error(true,
                       "REPLAY: host '%s', chart '%s': nothing to send (requested %llu to %llu)",
                       rrdhost_hostname(st->rrdhost), rrdset_id(st),
                       (unsigned long long)after, (unsigned long long)before);
#endif

    // release all the dictionary items acquired
    // finalize the queries
    for(size_t i = 0; i < dimensions && data[i].rda ;i++) {
        if(!data[i].enabled) continue;

        ops->finalize(&data[i].handle);
        dictionary_acquired_item_release(data[i].dict, data[i].rda);
    }

    return before;
}

static void replicate_chart_collection_state(BUFFER *wb, RRDSET *st) {
    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        if(!rd->exposed) continue;

        buffer_sprintf(wb, PLUGINSD_KEYWORD_REPLAY_RRDDIM_STATE " \"%s\" %llu %lld " NETDATA_DOUBLE_FORMAT " " NETDATA_DOUBLE_FORMAT "\n",
                       rrddim_id(rd),
                       (usec_t)rd->last_collected_time.tv_sec * USEC_PER_SEC + (usec_t)rd->last_collected_time.tv_usec,
                       rd->last_collected_value,
                       rd->last_calculated_value,
                       rd->last_stored_value
        );
    }
    rrddim_foreach_done(rd);

    buffer_sprintf(wb, PLUGINSD_KEYWORD_REPLAY_RRDSET_STATE " %llu %llu\n",
                   (usec_t)st->last_collected_time.tv_sec * USEC_PER_SEC + (usec_t)st->last_collected_time.tv_usec,
                   (usec_t)st->last_updated.tv_sec * USEC_PER_SEC + (usec_t)st->last_updated.tv_usec
    );
}

bool replicate_chart_response(RRDHOST *host, RRDSET *st, bool start_streaming, time_t after, time_t before) {
    time_t query_after = after;
    time_t query_before = before;
    time_t now = now_realtime_sec();
    time_t tolerance = 2;   // sometimes from the time we get this value, to the time we check,
                            // a data collection has been made
                            // so, we give this tolerance to detect invalid timestamps

    // find the first entry we have
    time_t first_entry_local = rrdset_first_entry_t(st);
    if(first_entry_local > now + tolerance) {
        internal_error(true,
                       "RRDSET: '%s' first time %llu is in the future (now is %llu)",
                       rrdset_id(st), (unsigned long long)first_entry_local, (unsigned long long)now);
        first_entry_local = now;
    }

    if (query_after < first_entry_local)
        query_after = first_entry_local;

    // find the latest entry we have
    time_t last_entry_local = st->last_updated.tv_sec;
    if(!last_entry_local) {
        internal_error(true,
                       "RRDSET: '%s' last updated time zero. Querying db for last updated time.",
                       rrdset_id(st));
        last_entry_local = rrdset_last_entry_t(st);
    }

    if(last_entry_local > now + tolerance) {
        internal_error(true,
                       "RRDSET: '%s' last updated time %llu is in the future (now is %llu)",
                       rrdset_id(st), (unsigned long long)last_entry_local, (unsigned long long)now);
        last_entry_local = now;
    }

    if (query_before > last_entry_local)
        query_before = last_entry_local;

    // if the parent asked us to start streaming, then fill the rest with the data that we have
    if (start_streaming)
        query_before = last_entry_local;

    if (query_after > query_before) {
        time_t tmp = query_before;
        query_before = query_after;
        query_after = tmp;
    }

    bool enable_streaming = (start_streaming || query_before == last_entry_local || !after || !before) ? true : false;

    // we might want to optimize this by filling a temporary buffer
    // and copying the result to the host's buffer in order to avoid
    // holding the host's buffer lock for too long
    BUFFER *wb = sender_start(host->sender);
    {
        // pass the original after/before so that the parent knows about
        // which time range we responded
        buffer_sprintf(wb, PLUGINSD_KEYWORD_REPLAY_BEGIN " \"%s\"\n", rrdset_id(st));

        if(after != 0 && before != 0)
            before = replicate_chart_timeframe(wb, st, query_after, query_before, enable_streaming);
        else {
            after = 0;
            before = 0;
            enable_streaming = true;
        }

        if(enable_streaming)
            replicate_chart_collection_state(wb, st);

        // end with first/last entries we have, and the first start time and
        // last end time of the data we sent
        buffer_sprintf(wb, PLUGINSD_KEYWORD_REPLAY_END " %d %llu %llu %s %llu %llu\n",
                       (int)st->update_every, (unsigned long long)first_entry_local, (unsigned long long)last_entry_local,
                       enable_streaming ? "true" : "false", (unsigned long long)after, (unsigned long long)before);
    }
    sender_commit(host->sender, wb);

    return enable_streaming;
}

static bool send_replay_chart_cmd(send_command callback, void *callback_data, RRDSET *st, bool start_streaming, time_t after, time_t before) {

    if(st->rrdhost->receiver && (!st->rrdhost->receiver->replication_first_time_t || after < st->rrdhost->receiver->replication_first_time_t))
        st->rrdhost->receiver->replication_first_time_t = after;

#ifdef NETDATA_INTERNAL_CHECKS
    if(after && before) {
        char after_buf[LOG_DATE_LENGTH + 1], before_buf[LOG_DATE_LENGTH + 1];
        log_date(after_buf, LOG_DATE_LENGTH, after);
        log_date(before_buf, LOG_DATE_LENGTH, before);
        internal_error(true,
                       "REPLAY: host '%s', chart '%s': sending replication request %llu [%s] to %llu [%s], start streaming: %s",
                       rrdhost_hostname(st->rrdhost), rrdset_id(st),
                       (unsigned long long)after, after_buf, (unsigned long long)before, before_buf,
                       start_streaming?"true":"false");
    }
    else {
        internal_error(true,
                       "REPLAY: host '%s', chart '%s': sending empty replication request, start streaming: %s",
                       rrdhost_hostname(st->rrdhost), rrdset_id(st),
                       start_streaming?"true":"false");
    }
#endif

#ifdef NETDATA_INTERNAL_CHECKS
    internal_error(
            st->replay.after != 0 || st->replay.before != 0,
            "REPLAY ERROR: host '%s', chart '%s': sending replication request, while there is another inflight",
            rrdhost_hostname(st->rrdhost), rrdset_id(st)
            );

    st->replay.start_streaming = start_streaming;
    st->replay.after = after;
    st->replay.before = before;
#endif

    debug(D_REPLICATION, PLUGINSD_KEYWORD_REPLAY_CHART " \"%s\" \"%s\" %llu %llu\n",
          rrdset_id(st), start_streaming ? "true" : "false", (unsigned long long)after, (unsigned long long)before);

    char buffer[2048 + 1];
    snprintfz(buffer, 2048, PLUGINSD_KEYWORD_REPLAY_CHART " \"%s\" \"%s\" %llu %llu\n",
                      rrdset_id(st), start_streaming ? "true" : "false",
                      (unsigned long long)after, (unsigned long long)before);

    int ret = callback(buffer, callback_data);
    if (ret < 0) {
        error("REPLICATION: failed to send replication request to child (error %d)", ret);
        return false;
    }

    return true;
}

bool replicate_chart_request(send_command callback, void *callback_data, RRDHOST *host, RRDSET *st,
                             time_t first_entry_child, time_t last_entry_child,
                             time_t prev_first_entry_wanted, time_t prev_last_entry_wanted)
{
    time_t now = now_realtime_sec();

    // if replication is disabled, send an empty replication request
    // asking no data
    if (unlikely(!rrdhost_option_check(host, RRDHOST_OPTION_REPLICATION))) {
        internal_error(true,
                       "REPLAY: host '%s', chart '%s': sending empty replication request because replication is disabled",
                       rrdhost_hostname(host), rrdset_id(st));

        return send_replay_chart_cmd(callback, callback_data, st, true, 0, 0);
    }

    // Child has no stored data
    if (!last_entry_child) {
        error("REPLAY: host '%s', chart '%s': sending empty replication request because child has no stored data",
              rrdhost_hostname(host), rrdset_id(st));

        return send_replay_chart_cmd(callback, callback_data, st, true, 0, 0);
    }

    // Nothing to get if the chart has not dimensions
    if (!rrdset_number_of_dimensions(st)) {
        error("REPLAY: host '%s', chart '%s': sending empty replication request because chart has no dimensions",
              rrdhost_hostname(host), rrdset_id(st));

        return send_replay_chart_cmd(callback, callback_data, st, true, 0, 0);
    }

    // if the child's first/last entries are nonsensical, resume streaming
    // without asking for any data
    if (first_entry_child <= 0) {
        error("REPLAY: host '%s', chart '%s': sending empty replication because first entry of the child is invalid (%llu)",
              rrdhost_hostname(host), rrdset_id(st), (unsigned long long)first_entry_child);

        return send_replay_chart_cmd(callback, callback_data, st, true, 0, 0);
    }

    if (first_entry_child > last_entry_child) {
        error("REPLAY: host '%s', chart '%s': sending empty replication because child timings are invalid (first entry %llu > last entry %llu)",
              rrdhost_hostname(host), rrdset_id(st), (unsigned long long)first_entry_child, (unsigned long long)last_entry_child);

        return send_replay_chart_cmd(callback, callback_data, st, true, 0, 0);
    }

    time_t last_entry_local = rrdset_last_entry_t(st);
    if(last_entry_local > now) {
        internal_error(true,
                       "REPLAY: host '%s', chart '%s': local last entry time %llu is in the future (now is %llu). Adjusting it.",
                       rrdhost_hostname(host), rrdset_id(st), (unsigned long long)last_entry_local, (unsigned long long)now);
        last_entry_local = now;
    }

    // should never happen but if it does, start streaming without asking for any data
    if (last_entry_local > last_entry_child) {
        error("REPLAY: host '%s', chart '%s': sending empty replication request because our last entry (%llu) in later than the child one (%llu)",
              rrdhost_hostname(host), rrdset_id(st), (unsigned long long)last_entry_local, (unsigned long long)last_entry_child);

        return send_replay_chart_cmd(callback, callback_data, st, true, 0, 0);
    }

    time_t first_entry_wanted;
    if (prev_first_entry_wanted && prev_last_entry_wanted) {
        first_entry_wanted = prev_last_entry_wanted;
        if ((now - first_entry_wanted) > host->rrdpush_seconds_to_replicate)
            first_entry_wanted = now - host->rrdpush_seconds_to_replicate;
    }
    else
        first_entry_wanted = MAX(last_entry_local, first_entry_child);

    time_t last_entry_wanted = first_entry_wanted + host->rrdpush_replication_step;
    last_entry_wanted = MIN(last_entry_wanted, last_entry_child);

    bool start_streaming = (last_entry_wanted == last_entry_child);

    return send_replay_chart_cmd(callback, callback_data, st, start_streaming, first_entry_wanted, last_entry_wanted);
}

// ----------------------------------------------------------------------------
// replication thread

// replication request in sender DICTIONARY
// used for de-duplicating the requests
struct replication_request {
    struct sender_state *sender;        // the sender we should put the reply at
    STRING *chart_id;                   // the chart of the request
    time_t after;                       // the start time of the query (maybe zero) key for sorting (JudyL)
    time_t before;                      // the end time of the query (maybe zero)
    bool start_streaming;               // true, when the parent wants to send the rest of the data (before is overwritten) and enable normal streaming

    usec_t sender_last_flush_ut;        // the timestamp of the sender, at the time we indexed this request
    Word_t unique_id;                   // auto-increment, later requests have bigger
    bool found;                         // used as a result boolean for the find call
    bool indexed_in_judy;               // true when the request is indexed in judy
};

// replication sort entry in JudyL array
// used for sorting all requests, across all nodes
struct replication_sort_entry {
    struct replication_request *rq;

    size_t unique_id;              // used as a key to identify the sort entry - we never access its contents
};

// the global variables for the replication thread
static struct replication_thread {
    netdata_mutex_t mutex;

    size_t added;
    size_t executed;
    size_t removed;
    time_t first_time_t;
    size_t requests_count;
    Word_t next_unique_id;
    struct replication_request *requests;

    Word_t last_after;
    Word_t last_unique_id;

    size_t skipped_not_connected;
    size_t skipped_no_room;
    size_t sender_resets;
    size_t waits;

    Pvoid_t JudyL_array;
} rep = {
        .mutex = NETDATA_MUTEX_INITIALIZER,
        .added = 0,
        .executed = 0,
        .first_time_t = 0,
        .requests_count = 0,
        .next_unique_id = 1,
        .skipped_no_room = 0,
        .skipped_not_connected = 0,
        .sender_resets = 0,
        .waits = 0,
        .requests = NULL,
        .JudyL_array = NULL,
};

static __thread int replication_recursive_mutex_recursions = 0;

static void replication_recursive_lock() {
    if(++replication_recursive_mutex_recursions == 1)
        netdata_mutex_lock(&rep.mutex);

#ifdef NETDATA_INTERNAL_CHECKS
    if(replication_recursive_mutex_recursions < 0 || replication_recursive_mutex_recursions > 2)
        fatal("REPLICATION: recursions is %d", replication_recursive_mutex_recursions);
#endif
}

static void replication_recursive_unlock() {
    if(--replication_recursive_mutex_recursions == 0)
        netdata_mutex_unlock(&rep.mutex);

#ifdef NETDATA_INTERNAL_CHECKS
    if(replication_recursive_mutex_recursions < 0 || replication_recursive_mutex_recursions > 2)
        fatal("REPLICATION: recursions is %d", replication_recursive_mutex_recursions);
#endif
}

// ----------------------------------------------------------------------------
// replication sort entry management

static struct replication_sort_entry *replication_sort_entry_create(struct replication_request *rq) {
    struct replication_sort_entry *rse = mallocz(sizeof(struct replication_sort_entry));

    rrdpush_sender_pending_replication_requests_plus_one(rq->sender);

    // copy the request
    rse->rq = rq;
    rse->unique_id = rep.next_unique_id++;

    // save the unique id into the request, to be able to delete it later
    rq->unique_id = rse->unique_id;
    rq->indexed_in_judy = false;
    return rse;
}

static void replication_sort_entry_destroy(struct replication_sort_entry *rse) {
    freez(rse);
}

static struct replication_sort_entry *replication_sort_entry_add(struct replication_request *rq) {
    replication_recursive_lock();

    struct replication_sort_entry *rse = replication_sort_entry_create(rq);

    if(rq->after < (time_t)rep.last_after) {
        // make it find this request first
        rep.last_after = rq->after;
        rep.last_unique_id = rq->unique_id;
    }

    rep.added++;
    rep.requests_count++;

    Pvoid_t *inner_judy_ptr;

    // find the outer judy entry, using after as key
    inner_judy_ptr = JudyLGet(rep.JudyL_array, (Word_t) rq->after, PJE0);
    if(!inner_judy_ptr)
        inner_judy_ptr = JudyLIns(&rep.JudyL_array, (Word_t) rq->after, PJE0);

    // add it to the inner judy, using unique_id as key
    Pvoid_t *item = JudyLIns(inner_judy_ptr, rq->unique_id, PJE0);
    *item = rse;
    rq->indexed_in_judy = true;

    if(!rep.first_time_t || rq->after < rep.first_time_t)
        rep.first_time_t = rq->after;

    replication_recursive_unlock();

    return rse;
}

static bool replication_sort_entry_unlink_and_free_unsafe(struct replication_sort_entry *rse, Pvoid_t **inner_judy_ppptr) {
    bool inner_judy_deleted = false;

    rep.removed++;
    rep.requests_count--;

    rrdpush_sender_pending_replication_requests_minus_one(rse->rq->sender);

    rse->rq->indexed_in_judy = false;

    // delete it from the inner judy
    JudyLDel(*inner_judy_ppptr, rse->rq->unique_id, PJE0);

    // if no items left, delete it from the outer judy
    if(**inner_judy_ppptr == NULL) {
        JudyLDel(&rep.JudyL_array, rse->rq->after, PJE0);
        inner_judy_deleted = true;
    }

    // free memory
    replication_sort_entry_destroy(rse);

    return inner_judy_deleted;
}

static void replication_sort_entry_del(struct replication_request *rq) {
    Pvoid_t *inner_judy_pptr;
    struct replication_sort_entry *rse_to_delete = NULL;

    replication_recursive_lock();
    if(rq->indexed_in_judy) {

        inner_judy_pptr = JudyLGet(rep.JudyL_array, rq->after, PJE0);
        if (inner_judy_pptr) {
            Pvoid_t *our_item_pptr = JudyLGet(*inner_judy_pptr, rq->unique_id, PJE0);
            if (our_item_pptr) {
                rse_to_delete = *our_item_pptr;
                replication_sort_entry_unlink_and_free_unsafe(rse_to_delete, &inner_judy_pptr);
            }
        }

        if (!rse_to_delete)
            fatal("Cannot find sort entry to delete for host '%s', chart '%s', time %ld.",
                  rrdhost_hostname(rq->sender->host), string2str(rq->chart_id), rq->after);

    }

    replication_recursive_unlock();
}

static inline PPvoid_t JudyLFirstOrNext(Pcvoid_t PArray, Word_t * PIndex, bool first) {
    if(unlikely(first))
        return JudyLFirst(PArray, PIndex, PJE0);

    return JudyLNext(PArray, PIndex, PJE0);
}

static struct replication_request replication_request_get_first_available() {
    Pvoid_t *inner_judy_pptr;

    replication_recursive_lock();

    struct replication_request rq = (struct replication_request){ .found = false };


    if(unlikely(!rep.last_after || !rep.last_unique_id)) {
        rep.last_after = 0;
        rep.last_unique_id = 0;
    }

    bool find_same_after = true;
    while(!rq.found && (inner_judy_pptr = JudyLFirstOrNext(rep.JudyL_array, &rep.last_after, find_same_after))) {
        Pvoid_t *our_item_pptr;

        while(!rq.found && (our_item_pptr = JudyLNext(*inner_judy_pptr, &rep.last_unique_id, PJE0))) {
            struct replication_sort_entry *rse = *our_item_pptr;
            struct sender_state *s = rse->rq->sender;

            bool sender_is_connected =
                    rrdhost_flag_check(s->host, RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED);

            bool sender_has_been_flushed_since_this_request =
                    rse->rq->sender_last_flush_ut != rrdpush_sender_get_flush_time(s);

            bool sender_has_room_to_spare =
                    s->buffer_used_percentage <= MAX_SENDER_BUFFER_PERCENTAGE_ALLOWED;

            if(unlikely(!sender_is_connected || sender_has_been_flushed_since_this_request)) {
                rep.skipped_not_connected++;
                if(replication_sort_entry_unlink_and_free_unsafe(rse, &inner_judy_pptr))
                    break;
            }

            else if(sender_has_room_to_spare) {
                // copy the request to return it
                rq = *rse->rq;

                // set the return result to found
                rq.found = true;

                if(replication_sort_entry_unlink_and_free_unsafe(rse, &inner_judy_pptr))
                    break;
            }
            else
                rep.skipped_no_room++;
        }

        // call JudyLNext from now on
        find_same_after = false;

        // prepare for the next iteration on the outer loop
        rep.last_unique_id = 0;
    }

    replication_recursive_unlock();
    return rq;
}

// ----------------------------------------------------------------------------
// replication request management

static void replication_request_react_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value __maybe_unused, void *sender_state __maybe_unused) {
    struct sender_state *s = sender_state; (void)s;
    struct replication_request *rq = value;

    RRDSET *st = rrdset_find(rq->sender->host, string2str(rq->chart_id));
    if(!st) {
        internal_error(true, "REPLAY: chart '%s' not found on host '%s'",
                       string2str(rq->chart_id), rrdhost_hostname(rq->sender->host));
    }
    else
        rrdset_flag_set(st, RRDSET_FLAG_SENDER_REPLICATION_QUEUED);

    // IMPORTANT:
    // We use the react instead of the insert callback
    // because we want the item to be atomically visible
    // to our replication thread, immediately after.

    // If we put this at the insert callback, the item is not guaranteed
    // to be atomically visible to others, so the replication thread
    // may see the replication sort entry, but fail to find the dictionary item
    // related to it.

    replication_sort_entry_add(rq);

    // this request is about a unique chart for this sender
    rrdpush_sender_replicating_charts_plus_one(s);
}

static bool replication_request_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *sender_state) {
    struct sender_state *s = sender_state; (void)s;
    struct replication_request *rq = old_value; (void)rq;
    struct replication_request *rq_new = new_value;

    internal_error(
            true,
            "STREAM %s [send to %s]: REPLAY ERROR: ignoring duplicate replication command received for chart '%s' (existing from %llu to %llu [%s], new from %llu to %llu [%s])",
            rrdhost_hostname(s->host), s->connected_to, dictionary_acquired_item_name(item),
            (unsigned long long)rq->after, (unsigned long long)rq->before, rq->start_streaming ? "true" : "false",
            (unsigned long long)rq_new->after, (unsigned long long)rq_new->before, rq_new->start_streaming ? "true" : "false");

//    bool updated_after = false, updated_before = false, updated_start_streaming = false, updated = false;
//
//    if(rq_new->after < rq->after && rq_new->after != 0)
//        updated_after = true;
//
//    if(rq_new->before > rq->before)
//        updated_before = true;
//
//    if(rq_new->start_streaming != rq->start_streaming)
//        updated_start_streaming = true;
//
//    if(updated_after || updated_before || updated_start_streaming) {
//        replication_recursive_lock();
//
//        if(rq->indexed_in_judy)
//            replication_sort_entry_del(rq);
//
//        if(rq_new->after < rq->after && rq_new->after != 0)
//            rq->after = rq_new->after;
//
//        if(rq->after == 0)
//            rq->before = 0;
//        else if(rq_new->before > rq->before)
//            rq->before = rq_new->before;
//
//        rq->start_streaming = rq->start_streaming;
//        replication_sort_entry_add(rq);
//
//        replication_recursive_unlock();
//        updated = true;
//
//        internal_error(
//                true,
//                "STREAM %s [send to %s]: REPLAY ERROR: updated duplicate replication command for chart '%s' (from %llu to %llu [%s])",
//                rrdhost_hostname(s->host), s->connected_to, dictionary_acquired_item_name(item),
//                (unsigned long long)rq->after, (unsigned long long)rq->before, rq->start_streaming ? "true" : "false");
//    }

    string_freez(rq_new->chart_id);
    return false;
}

static void replication_request_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *sender_state __maybe_unused) {
    struct replication_request *rq = value;

    // this request is about a unique chart for this sender
    rrdpush_sender_replicating_charts_minus_one(rq->sender);

    if(rq->indexed_in_judy)
        replication_sort_entry_del(rq);

    string_freez(rq->chart_id);
}


// ----------------------------------------------------------------------------
// public API

void replication_add_request(struct sender_state *sender, const char *chart_id, time_t after, time_t before, bool start_streaming) {
    struct replication_request rq = {
            .sender = sender,
            .chart_id = string_strdupz(chart_id),
            .after = after,
            .before = before,
            .start_streaming = start_streaming,
            .sender_last_flush_ut = rrdpush_sender_get_flush_time(sender),
    };

    dictionary_set(sender->replication_requests, chart_id, &rq, sizeof(struct replication_request));
}

void replication_sender_delete_pending_requests(struct sender_state *sender) {
    // allow the dictionary destructor to go faster on locks
    replication_recursive_lock();
    dictionary_flush(sender->replication_requests);
    replication_recursive_unlock();
}

void replication_init_sender(struct sender_state *sender) {
    sender->replication_requests = dictionary_create(DICT_OPTION_DONT_OVERWRITE_VALUE);
    dictionary_register_react_callback(sender->replication_requests, replication_request_react_callback, sender);
    dictionary_register_conflict_callback(sender->replication_requests, replication_request_conflict_callback, sender);
    dictionary_register_delete_callback(sender->replication_requests, replication_request_delete_callback, sender);
}

void replication_cleanup_sender(struct sender_state *sender) {
    // allow the dictionary destructor to go faster on locks
    replication_recursive_lock();
    dictionary_destroy(sender->replication_requests);
    replication_recursive_unlock();
}

void replication_recalculate_buffer_used_ratio_unsafe(struct sender_state *s) {
    size_t available = cbuffer_available_size_unsafe(s->host->sender->buffer);
    size_t percentage = (s->buffer->max_size - available) * 100 / s->buffer->max_size;

    if(percentage > MAX_SENDER_BUFFER_PERCENTAGE_ALLOWED)
        s->replication_reached_max = true;

    if(s->replication_reached_max &&
        percentage <= MIN_SENDER_BUFFER_PERCENTAGE_ALLOWED) {
        s->replication_reached_max = false;
        replication_recursive_lock();
        rep.last_after = 0;
        rep.last_unique_id = 0;
        rep.sender_resets++;
        replication_recursive_unlock();
    }

    s->buffer_used_percentage = percentage;
}

// ----------------------------------------------------------------------------
// replication thread

static void replication_main_cleanup(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    // custom code
    worker_unregister();

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

#define WORKER_JOB_FIND_NEXT                            1
#define WORKER_JOB_QUERYING                             2
#define WORKER_JOB_DELETE_ENTRY                         3
#define WORKER_JOB_FIND_CHART                           4
#define WORKER_JOB_STATISTICS                           5
#define WORKER_JOB_ACTIVATE_ENABLE_STREAMING            6
#define WORKER_JOB_CUSTOM_METRIC_PENDING_REQUESTS       7
#define WORKER_JOB_CUSTOM_METRIC_COMPLETION             8
#define WORKER_JOB_CUSTOM_METRIC_ADDED                  9
#define WORKER_JOB_CUSTOM_METRIC_DONE                   10
#define WORKER_JOB_CUSTOM_METRIC_SKIPPED_NOT_CONNECTED  11
#define WORKER_JOB_CUSTOM_METRIC_SKIPPED_NO_ROOM        12
#define WORKER_JOB_CUSTOM_METRIC_SENDER_RESETS          13
#define WORKER_JOB_CUSTOM_METRIC_WAITS                  14

void *replication_thread_main(void *ptr __maybe_unused) {
    netdata_thread_cleanup_push(replication_main_cleanup, ptr);

    worker_register("REPLICATION");

    worker_register_job_name(WORKER_JOB_FIND_NEXT, "find next");
    worker_register_job_name(WORKER_JOB_QUERYING, "querying");
    worker_register_job_name(WORKER_JOB_DELETE_ENTRY, "dict delete");
    worker_register_job_name(WORKER_JOB_FIND_CHART, "find chart");
    worker_register_job_name(WORKER_JOB_ACTIVATE_ENABLE_STREAMING, "enable streaming");

    worker_register_job_custom_metric(WORKER_JOB_CUSTOM_METRIC_PENDING_REQUESTS, "pending requests", "requests", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(WORKER_JOB_CUSTOM_METRIC_COMPLETION, "completion", "%", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(WORKER_JOB_CUSTOM_METRIC_ADDED, "added requests", "requests/s", WORKER_METRIC_INCREMENTAL_TOTAL);
    worker_register_job_custom_metric(WORKER_JOB_CUSTOM_METRIC_DONE, "finished requests", "requests/s", WORKER_METRIC_INCREMENTAL_TOTAL);
    worker_register_job_custom_metric(WORKER_JOB_CUSTOM_METRIC_SKIPPED_NOT_CONNECTED, "not connected requests", "requests/s", WORKER_METRIC_INCREMENTAL_TOTAL);
    worker_register_job_custom_metric(WORKER_JOB_CUSTOM_METRIC_SKIPPED_NO_ROOM, "no room requests", "requests/s", WORKER_METRIC_INCREMENTAL_TOTAL);
    worker_register_job_custom_metric(WORKER_JOB_CUSTOM_METRIC_SENDER_RESETS, "sender resets", "resets/s", WORKER_METRIC_INCREMENTAL_TOTAL);
    worker_register_job_custom_metric(WORKER_JOB_CUSTOM_METRIC_WAITS, "waits", "waits/s", WORKER_METRIC_INCREMENTAL_TOTAL);

    time_t latest_first_time_t = 0;

    while(!netdata_exit) {
        worker_is_busy(WORKER_JOB_FIND_NEXT);
        struct replication_request rq = replication_request_get_first_available();

        worker_is_busy(WORKER_JOB_STATISTICS);
        worker_set_metric(WORKER_JOB_CUSTOM_METRIC_PENDING_REQUESTS, (NETDATA_DOUBLE)rep.requests_count);
        worker_set_metric(WORKER_JOB_CUSTOM_METRIC_ADDED, (NETDATA_DOUBLE)rep.added);
        worker_set_metric(WORKER_JOB_CUSTOM_METRIC_DONE, (NETDATA_DOUBLE)rep.executed);
        worker_set_metric(WORKER_JOB_CUSTOM_METRIC_SKIPPED_NOT_CONNECTED, (NETDATA_DOUBLE)rep.skipped_not_connected);
        worker_set_metric(WORKER_JOB_CUSTOM_METRIC_SKIPPED_NO_ROOM, (NETDATA_DOUBLE)rep.skipped_no_room);
        worker_set_metric(WORKER_JOB_CUSTOM_METRIC_SENDER_RESETS, (NETDATA_DOUBLE)rep.sender_resets);
        worker_set_metric(WORKER_JOB_CUSTOM_METRIC_WAITS, (NETDATA_DOUBLE)rep.waits);

        if(latest_first_time_t) {
            time_t now = now_realtime_sec();
            time_t total = now - rep.first_time_t;
            time_t done = latest_first_time_t - rep.first_time_t;
            worker_set_metric(WORKER_JOB_CUSTOM_METRIC_COMPLETION, (NETDATA_DOUBLE)done * 100.0 / (NETDATA_DOUBLE)total);
        }

        if(!rq.found) {
            worker_is_idle();

            if(!rep.requests_count)
                worker_set_metric(WORKER_JOB_CUSTOM_METRIC_COMPLETION, 100.0);

            // make it start from the beginning
            rep.last_after = 0;
            rep.last_unique_id = 0;

            rep.waits++;

            sleep_usec(1000 * USEC_PER_MS);
            continue;
        }
        else {
            // delete the request from the dictionary
            worker_is_busy(WORKER_JOB_DELETE_ENTRY);
            dictionary_del(rq.sender->replication_requests, string2str(rq.chart_id));
        }

        worker_is_busy(WORKER_JOB_FIND_CHART);
        RRDSET *st = rrdset_find(rq.sender->host, string2str(rq.chart_id));
        if(!st) {
            internal_error(true, "REPLAY ERROR: chart '%s' not found on host '%s'",
                           string2str(rq.chart_id), rrdhost_hostname(rq.sender->host));

            continue;
        }

        if(!rrdset_flag_check(st, RRDSET_FLAG_SENDER_REPLICATION_IN_PROGRESS)) {
            rrdset_flag_set(st, RRDSET_FLAG_SENDER_REPLICATION_IN_PROGRESS);
            rrdset_flag_clear(st, RRDSET_FLAG_SENDER_REPLICATION_FINISHED);
            rrdhost_sender_replicating_charts_plus_one(st->rrdhost);
        }
        rrdset_flag_clear(st, RRDSET_FLAG_SENDER_REPLICATION_QUEUED);

        worker_is_busy(WORKER_JOB_QUERYING);

        latest_first_time_t = rq.after;

        if(rq.after < rq.sender->replication_first_time || !rq.sender->replication_first_time)
            rq.sender->replication_first_time = rq.after;

        if(rq.before < rq.sender->replication_current_time || !rq.sender->replication_current_time)
            rq.sender->replication_current_time = rq.before;

        netdata_thread_disable_cancelability();

        // send the replication data
        bool start_streaming = replicate_chart_response(st->rrdhost, st,
                                                        rq.start_streaming, rq.after, rq.before);

        netdata_thread_enable_cancelability();

        rep.executed++;

        if(start_streaming && rq.sender_last_flush_ut == rrdpush_sender_get_flush_time(rq.sender)) {
            worker_is_busy(WORKER_JOB_ACTIVATE_ENABLE_STREAMING);

            // enable normal streaming if we have to
            // but only if the sender buffer has not been flushed since we started

            if(rrdset_flag_check(st, RRDSET_FLAG_SENDER_REPLICATION_IN_PROGRESS)) {
                rrdset_flag_clear(st, RRDSET_FLAG_SENDER_REPLICATION_IN_PROGRESS);
                rrdset_flag_set(st, RRDSET_FLAG_SENDER_REPLICATION_FINISHED);
                rrdhost_sender_replicating_charts_minus_one(st->rrdhost);
            }
            else
                internal_error(true, "REPLAY ERROR: received start streaming command for chart '%s' or host '%s', but the chart is not in progress replicating",
                               string2str(rq.chart_id), rrdhost_hostname(st->rrdhost));
        }
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}

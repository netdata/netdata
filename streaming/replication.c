// SPDX-License-Identifier: GPL-3.0-or-later

#include "replication.h"
#include "Judy.h"

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
            "REPLAY: host '%s', chart '%s': sending replication request, while there is another inflight",
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

static size_t sender_buffer_used_percent(struct sender_state *s) {
    netdata_mutex_lock(&s->mutex);
    size_t available = cbuffer_available_size_unsafe(s->host->sender->buffer);
    netdata_mutex_unlock(&s->mutex);

    return (s->host->sender->buffer->max_size - available) * 100 / s->host->sender->buffer->max_size;
}


// ----------------------------------------------------------------------------
// replication thread

// replication request in sender DICTIONARY
// used for de-duplicating the requests
struct replication_request {
    struct sender_state *sender;
    usec_t sender_last_flush_ut;
    STRING *chart_id;
    time_t after;                       // key for sorting (JudyL)
    time_t before;
    bool start_streaming;
    bool found;
};

// replication sort entry in JudyL array
// used for sorting all requests, across all nodes
struct replication_sort_entry {
    struct replication_request req;

    const void *unique_id;              // used as a key to identify the sort entry - we never access its contents
    bool executed;
    struct replication_sort_entry *next;
};

// the global variables for the replication thread
static struct replication_thread {
    netdata_mutex_t mutex;

    size_t added;
    size_t removed;
    time_t first_time_t;
    size_t requests_count;
    struct replication_request *requests;

    Pvoid_t JudyL_array;
} rep = {
        .mutex = NETDATA_MUTEX_INITIALIZER,
        .added = 0,
        .removed = 0,
        .first_time_t = 0,
        .requests_count = 0,
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

static struct replication_sort_entry *replication_sort_entry_create(struct replication_request *r, const void *unique_id) {
    struct replication_sort_entry *t = mallocz(sizeof(struct replication_sort_entry));

    // copy the request
    t->req = *r;
    t->req.chart_id = string_dup(r->chart_id);


    t->unique_id = unique_id;
    t->executed = false;
    t->next = NULL;
    return t;
}

static void replication_sort_entry_destroy(struct replication_sort_entry *t) {
    string_freez(t->req.chart_id);
    freez(t);
}

static struct replication_sort_entry *replication_sort_entry_add(struct replication_request *r, const void *unique_id) {
    struct replication_sort_entry *t = replication_sort_entry_create(r, unique_id);

    replication_recursive_lock();

    rep.added++;

    Pvoid_t *PValue;

    PValue = JudyLGet(rep.JudyL_array, (Word_t) r->after, PJE0);
    if(!PValue)
        PValue = JudyLIns(&rep.JudyL_array, (Word_t) r->after, PJE0);

    t->next = *PValue;
    *PValue = t;

    if(!rep.first_time_t || r->after < rep.first_time_t)
        rep.first_time_t = r->after;

    replication_recursive_unlock();

    return t;
}

static void replication_sort_entry_del(struct sender_state *sender, STRING *chart_id, time_t after, const DICTIONARY_ITEM *item) {
    Pvoid_t *PValue;
    struct replication_sort_entry *to_delete = NULL;

    replication_recursive_lock();

    rep.removed++;

    PValue = JudyLGet(rep.JudyL_array, after, PJE0);
    if(PValue) {
        struct replication_sort_entry *t = *PValue;
        t->executed = true; // make sure we don't get it again

        if(!t->next) {
            // we are alone here, delete the judy entry

            if(t->unique_id != item)
                fatal("Item to delete is not matching host '%s', chart '%s', time %ld.",
                      rrdhost_hostname(sender->host), string2str(chart_id), after);

            to_delete = t;
            JudyLDel(&rep.JudyL_array, after, PJE0);
        }
        else {
            // find our entry in the linked list

            struct replication_sort_entry *t_old = NULL;
            do {
                if(t->unique_id == item) {
                    to_delete = t;

                    if(t_old)
                        t_old->next = t->next;
                    else
                        *PValue = t->next;

                    break;
                }

                t_old = t;
                t = t->next;

            } while(t);
        }
    }

    if(!to_delete)
        fatal("Cannot find sort entry to delete for host '%s', chart '%s', time %ld.",
              rrdhost_hostname(sender->host), string2str(chart_id), after);

    replication_recursive_unlock();

    replication_sort_entry_destroy(to_delete);
}

static struct replication_request replication_request_get_first_available() {
    struct replication_sort_entry *found = NULL;
    Pvoid_t *PValue;
    Word_t Index;

    replication_recursive_lock();

    rep.requests_count = JudyLCount(rep.JudyL_array, 0, 0xFFFFFFFF, PJE0);
    if(!rep.requests_count) {
        replication_recursive_unlock();
        return (struct replication_request){ .found = false };
    }

    Index = 0;
    PValue = JudyLFirst(rep.JudyL_array, &Index, PJE0);
    while(!found && PValue) {
        struct replication_sort_entry *t;

        for(t = *PValue; t ;t = t->next) {
            if(!t->executed
                && sender_buffer_used_percent(t->req.sender) <= 10
                && t->req.sender_last_flush_ut == __atomic_load_n(&t->req.sender->last_flush_time_ut, __ATOMIC_SEQ_CST)
                ) {
                found = t;
                found->executed = true;
                break;
            }
        }

        if(!found)
            PValue = JudyLNext(rep.JudyL_array, &Index, PJE0);
    }

    // copy the values we need, while we have the lock
    struct replication_request ret;

    if(found) {
        ret = found->req;
        ret.chart_id = string_dup(ret.chart_id);
        ret.found = true;
    }
    else
        ret.found = false;

    replication_recursive_unlock();

    return ret;
}

// ----------------------------------------------------------------------------
// replication request management

static void replication_request_react_callback(const DICTIONARY_ITEM *item, void *value __maybe_unused, void *sender_state __maybe_unused) {
    struct sender_state *s = sender_state; (void)s;
    struct replication_request *r = value;

    // IMPORTANT:
    // We use the react instead of the insert callback
    // because we want the item to be atomically visible
    // to our replication thread, immediately after.

    // If we put this at the insert callback, the item is not guaranteed
    // to be atomically visible to others, so the replication thread
    // may see the replication sort entry, but fail to find the dictionary item
    // related to it.

    replication_sort_entry_add(r, item);
    __atomic_fetch_add(&r->sender->replication_pending_requests, 1, __ATOMIC_SEQ_CST);
}

static bool replication_request_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *sender_state) {
    struct sender_state *s = sender_state; (void)s;
    struct replication_request *r = old_value; (void)r;
    struct replication_request *r_new = new_value;

    internal_error(
            true,
            "STREAM %s [send to %s]: ignoring duplicate replication command received for chart '%s' (existing from %llu to %llu [%s], new from %llu to %llu [%s])",
            rrdhost_hostname(s->host), s->connected_to, dictionary_acquired_item_name(item),
            (unsigned long long)r->after, (unsigned long long)r->before, r->start_streaming ? "true" : "false",
            (unsigned long long)r_new->after, (unsigned long long)r_new->before, r_new->start_streaming ? "true" : "false");

    string_freez(r_new->chart_id);

    return false;
}

static void replication_request_delete_callback(const DICTIONARY_ITEM *item, void *value, void *sender_state __maybe_unused) {
    struct replication_request *r = value;

    replication_sort_entry_del(r->sender, r->chart_id, r->after, item);

    string_freez(r->chart_id);
    __atomic_fetch_sub(&r->sender->replication_pending_requests, 1, __ATOMIC_SEQ_CST);
}


// ----------------------------------------------------------------------------
// public API

void replication_add_request(struct sender_state *sender, const char *chart_id, time_t after, time_t before, bool start_streaming) {
    struct replication_request tmp = {
            .sender = sender,
            .chart_id = string_strdupz(chart_id),
            .after = after,
            .before = before,
            .start_streaming = start_streaming,
            .sender_last_flush_ut = __atomic_load_n(&sender->last_flush_time_ut, __ATOMIC_SEQ_CST),
    };

    dictionary_set(sender->replication_requests, chart_id, &tmp, sizeof(struct replication_request));
}

void replication_flush_sender(struct sender_state *sender) {
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

// ----------------------------------------------------------------------------
// replication thread

static void replication_main_cleanup(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    // custom code
    worker_unregister();

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

#define WORKER_JOB_ITERATION 1
#define WORKER_JOB_REPLAYING 2
#define WORKER_JOB_CUSTOM_METRIC_PENDING_REQUESTS 3
#define WORKER_JOB_CUSTOM_METRIC_COMPLETION 4
#define WORKER_JOB_CUSTOM_METRIC_ADDED 5
#define WORKER_JOB_CUSTOM_METRIC_DONE 6

void *replication_thread_main(void *ptr __maybe_unused) {
    netdata_thread_cleanup_push(replication_main_cleanup, ptr);

    worker_register("REPLICATION");

    worker_register_job_name(WORKER_JOB_ITERATION, "iteration");
    worker_register_job_name(WORKER_JOB_REPLAYING, "replaying");

    worker_register_job_custom_metric(WORKER_JOB_CUSTOM_METRIC_PENDING_REQUESTS, "pending requests", "requests", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(WORKER_JOB_CUSTOM_METRIC_COMPLETION, "completion", "%", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(WORKER_JOB_CUSTOM_METRIC_ADDED, "added requests", "requests/s", WORKER_METRIC_INCREMENTAL_TOTAL);
    worker_register_job_custom_metric(WORKER_JOB_CUSTOM_METRIC_DONE, "finished requests", "requests/s", WORKER_METRIC_INCREMENTAL_TOTAL);

    while(!netdata_exit) {
        worker_is_busy(WORKER_JOB_ITERATION);

        // this call also updates our statistics
        struct replication_request r = replication_request_get_first_available();

        if(r.found) {
            // delete the request from the dictionary
            dictionary_del(r.sender->replication_requests, string2str(r.chart_id));
        }

        worker_set_metric(WORKER_JOB_CUSTOM_METRIC_PENDING_REQUESTS, (NETDATA_DOUBLE)rep.requests_count);
        worker_set_metric(WORKER_JOB_CUSTOM_METRIC_ADDED, (NETDATA_DOUBLE)rep.added);
        worker_set_metric(WORKER_JOB_CUSTOM_METRIC_DONE, (NETDATA_DOUBLE)rep.removed);

        if(!r.found && !rep.requests_count) {
            worker_set_metric(WORKER_JOB_CUSTOM_METRIC_COMPLETION, 100.0);
            worker_is_idle();
            sleep_usec(1000 * USEC_PER_MS);
            continue;
        }

        if(!r.found) {
            worker_is_idle();
            sleep_usec(1 * USEC_PER_MS);
            continue;
        }

        RRDSET *st = rrdset_find(r.sender->host, string2str(r.chart_id));
        if(!st) {
            internal_error(true, "REPLAY: chart '%s' not found on host '%s'",
                           string2str(r.chart_id), rrdhost_hostname(r.sender->host));

            continue;
        }

        worker_is_busy(WORKER_JOB_REPLAYING);

        time_t latest_first_time_t = r.after;

        if(r.after < r.sender->replication_first_time || !r.sender->replication_first_time)
            r.sender->replication_first_time = r.after;

        if(r.before < r.sender->replication_min_time || !r.sender->replication_min_time)
            r.sender->replication_min_time = r.before;

        netdata_thread_disable_cancelability();

        // send the replication data
        bool start_streaming = replicate_chart_response(st->rrdhost, st,
                                                        r.start_streaming, r.after, r.before);

        netdata_thread_enable_cancelability();

        if(start_streaming && r.sender_last_flush_ut == __atomic_load_n(&r.sender->last_flush_time_ut, __ATOMIC_SEQ_CST)) {
            __atomic_fetch_add(&r.sender->receiving_metrics, 1, __ATOMIC_SEQ_CST);

            // enable normal streaming if we have to
            // but only if the sender buffer has not been flushed since we started

            debug(D_REPLICATION, "Enabling metric streaming for chart %s.%s",
                  rrdhost_hostname(r.sender->host), rrdset_id(st));

            rrdset_flag_set(st, RRDSET_FLAG_SENDER_REPLICATION_FINISHED);
        }

        // statistics
        {
            time_t now = now_realtime_sec();
            time_t total = now - rep.first_time_t;
            time_t done = latest_first_time_t - rep.first_time_t;
            worker_set_metric(WORKER_JOB_CUSTOM_METRIC_COMPLETION, (NETDATA_DOUBLE)done * 100.0 / (NETDATA_DOUBLE)total);
        }
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}

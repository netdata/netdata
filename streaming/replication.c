// SPDX-License-Identifier: GPL-3.0-or-later

#include "replication.h"

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
        error("failed to send replay request to child (ret=%d)", ret);
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

    // should never happen but it if does, start streaming without asking
    // for any data
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

struct replication_request {
    struct sender_state *sender;
    STRING *chart_id;
    bool start_streaming;
    time_t after;
    time_t before;

    size_t refcount;
    bool deleted;

    RRDSET *st;

    struct replication_request *next;
    struct replication_request *prev;
};

struct replication_thread {
    bool thread_is_running;
    netdata_mutex_t mutex;

    size_t requests_count;
    struct replication_request *requests;
};

static struct replication_thread rep = {
        .thread_is_running = false,
        .mutex = NETDATA_MUTEX_INITIALIZER,
        .requests_count = 0,
        .requests = NULL,
};

void replication_lock() {
    netdata_mutex_lock(&rep.mutex);
}

void replication_unlock() {
    netdata_mutex_unlock(&rep.mutex);
}

static void replication_del_request_unsafe(struct replication_request *r) {
    if(!r->refcount) {
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(rep.requests, r, prev, next);
        rep.requests_count--;
        string_freez(r->chart_id);
        freez(r);
    }
    else
        // currently being executed
        r->deleted = true;
}

void replication_add_request(struct sender_state *sender, const char *chart_id, time_t after, time_t before, bool start_streaming) {
    replication_lock();

    struct replication_request *r = mallocz(sizeof(struct replication_request));
    r->sender = sender;
    r->chart_id = string_strdupz(chart_id);
    r->after = after;
    r->before = before;
    r->start_streaming = start_streaming;
    r->refcount = 0;
    r->deleted = false;
    r->st = NULL;
    r->next = NULL;
    r->prev = NULL;

    DOUBLE_LINKED_LIST_APPEND_UNSAFE(rep.requests, r, prev, next);
    rep.requests_count++;

    replication_unlock();
}

void replication_flush_sender(struct sender_state *sender) {
    replication_lock();

    struct replication_request *r = rep.requests;
    while(r) {
        if(r->sender == sender) {
            struct replication_request *to_delete = r;
            r = r->next;

            replication_del_request_unsafe(to_delete);
        }
        else
            r = r->next;
    }

    replication_unlock();
}

static int replication_request_compar(const void *a, const void *b) {
    struct replication_request *r1 = *(struct replication_request **)a;
    struct replication_request *r2 = *(struct replication_request **)b;

    if(r1->after < r2->after)
        return -1;
    if(r1->after > r2->after)
        return 1;
    return 0;
}

static struct replication_request *replication_get_oldest_request_unsafe(void) {

    size_t entries = rep.requests_count;
    struct replication_request **array = mallocz(sizeof(struct replication_request *) * entries);

    struct replication_request *r = rep.requests;
    size_t i;
    for(i = 0; i < entries && r ;i++, r = r->next)
        array[i] = r;

    if(i != entries)
        entries = i;

    qsort(array, entries, sizeof(struct replication_request *), replication_request_compar);

    for(i = 0; i < entries ;i++) {
        r = array[i];

        if(r->refcount)
            // currently being executed
            continue;

        r->st = rrdset_find(r->sender->host, string2str(r->chart_id));
        if(!r->st) {
            internal_error(true,
                           "STREAM %s [send to %s]: cannot find chart '%s' to satisfy pending replication command."
                           , rrdhost_hostname(r->sender->host), r->sender->connected_to, string2str(r->chart_id));

            replication_del_request_unsafe(r);
        }
        else
            return r;
    }

    return NULL;
}

void *replication_thread_main(void *ptr __maybe_unused) {

    while(!netdata_exit) {
        replication_lock();

        if(!rep.requests || !rep.requests_count) {
            replication_unlock();
            sleep_usec(1000 * USEC_PER_MS);
            continue;
        }

        struct replication_request *r = replication_get_oldest_request_unsafe();
        if(!r) {
            replication_unlock();
            sleep_usec(1000 * USEC_PER_MS);
            continue;
        }

        if(r->after < r->sender->replication_first_time || !r->sender->replication_first_time)
            r->sender->replication_first_time = r->after;

        if(r->before < r->sender->replication_min_time || !r->sender->replication_min_time)
            r->sender->replication_min_time = r->before;

        r->refcount++;
        replication_unlock();
        netdata_thread_disable_cancelability();

        // send the replication data
        bool start_streaming = replicate_chart_response(r->st->rrdhost, r->st,
                                                        r->start_streaming, r->after, r->before);

        netdata_thread_enable_cancelability();
        replication_lock();

        if (likely(!r->deleted)) {
            // enable normal streaming if we have to
            if (start_streaming) {
                debug(D_REPLICATION, "Enabling metric streaming for chart %s.%s",
                      rrdhost_hostname(r->sender->host), rrdset_id(r->st));

                rrdset_flag_set(r->st, RRDSET_FLAG_SENDER_REPLICATION_FINISHED);
            }
        }

        r->refcount--;
        replication_del_request_unsafe(r);

        replication_unlock();
    }

    return NULL;
}

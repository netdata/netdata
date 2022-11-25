// SPDX-License-Identifier: GPL-3.0-or-later

#include "replication.h"
#include "Judy.h"

#define MAX_SENDER_BUFFER_PERCENTAGE_ALLOWED 20
#define MIN_SENDER_BUFFER_PERCENTAGE_ALLOWED 10

// ----------------------------------------------------------------------------
// sending replication replies

static time_t replicate_chart_timeframe(BUFFER *wb, RRDSET *st, time_t after, time_t before, bool enable_streaming, time_t wall_clock_time) {
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
        internal_error(true, "STREAM_SENDER REPLAY: 'host:%s/chart:%s' has start_streaming = true, adjusting replication before timestamp from %llu to %llu",
                       rrdhost_hostname(st->rrdhost), rrdset_id(st),
                       (unsigned long long)before,
                       (unsigned long long)st->last_updated.tv_sec
        );
        before = st->last_updated.tv_sec;
    }

    // prepare our array of dimensions
    {
        RRDDIM *rd;
        rrddim_foreach_read(rd, st) {
            if (rd_dfe.counter >= dimensions) {
                internal_error(true, "STREAM_SENDER REPLAY ERROR: 'host:%s/chart:%s' has more dimensions than the replicated ones",
                               rrdhost_hostname(st->rrdhost), rrdset_id(st));
                break;
            }

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
                           "STREAM_SENDER REPLAY ERROR: 'host:%s/chart:%s/dim:%s': db does not advance the query beyond time %llu",
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

        if(min_start_time > wall_clock_time + 1 || min_end_time > wall_clock_time + st->update_every + 1) {
            internal_error(true,
                           "STREAM_SENDER REPLAY ERROR: 'host:%s/chart:%s': db provided future start time %llu or end time %llu (now is %llu)",
                            rrdhost_hostname(st->rrdhost), rrdset_id(st),
                           (unsigned long long)min_start_time,
                           (unsigned long long)min_end_time,
                           (unsigned long long)wall_clock_time);
            break;
        }

        if(min_end_time < now) {
#ifdef NETDATA_LOG_REPLICATION_REQUESTS
            internal_error(true,
                           "STREAM_SENDER REPLAY: 'host:%s/chart:%s': no data on any dimension beyond time %llu",
                           rrdhost_hostname(st->rrdhost), rrdset_id(st), (unsigned long long)now);
#endif // NETDATA_LOG_REPLICATION_REQUESTS
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

#ifdef NETDATA_LOG_REPLICATION_REQUESTS
    if(actual_after) {
        char actual_after_buf[LOG_DATE_LENGTH + 1], actual_before_buf[LOG_DATE_LENGTH + 1];
        log_date(actual_after_buf, LOG_DATE_LENGTH, actual_after);
        log_date(actual_before_buf, LOG_DATE_LENGTH, actual_before);
        internal_error(true,
                       "STREAM_SENDER REPLAY: 'host:%s/chart:%s': sending data %llu [%s] to %llu [%s] (requested %llu [delta %lld] to %llu [delta %lld])",
                       rrdhost_hostname(st->rrdhost), rrdset_id(st),
                       (unsigned long long)actual_after, actual_after_buf, (unsigned long long)actual_before, actual_before_buf,
                       (unsigned long long)after, (long long)(actual_after - after), (unsigned long long)before, (long long)(actual_before - before));
    }
    else
        internal_error(true,
                       "STREAM_SENDER REPLAY: 'host:%s/chart:%s': nothing to send (requested %llu to %llu)",
                       rrdhost_hostname(st->rrdhost), rrdset_id(st),
                       (unsigned long long)after, (unsigned long long)before);
#endif // NETDATA_LOG_REPLICATION_REQUESTS

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
                       "STREAM_SENDER REPLAY ERROR: 'host:%s/chart:%s' db first time %llu is in the future (now is %llu)",
                       rrdhost_hostname(st->rrdhost), rrdset_id(st),
                       (unsigned long long)first_entry_local, (unsigned long long)now);
        first_entry_local = now;
    }

    if (query_after < first_entry_local)
        query_after = first_entry_local;

    // find the latest entry we have
    time_t last_entry_local = st->last_updated.tv_sec;
    if(!last_entry_local) {
        internal_error(true,
                       "STREAM_SENDER REPLAY ERROR: 'host:%s/chart:%s' RRDSET reports last updated time zero.",
                       rrdhost_hostname(st->rrdhost), rrdset_id(st));
        last_entry_local = rrdset_last_entry_t(st);
        if(!last_entry_local) {
            internal_error(true,
                           "STREAM_SENDER REPLAY ERROR: 'host:%s/chart:%s' db reports last time zero.",
                           rrdhost_hostname(st->rrdhost), rrdset_id(st));
            last_entry_local = now;
        }
    }

    if(last_entry_local > now + tolerance) {
        internal_error(true,
                       "STREAM_SENDER REPLAY ERROR: 'host:%s/chart:%s' last updated time %llu is in the future (now is %llu)",
                       rrdhost_hostname(st->rrdhost), rrdset_id(st),
                       (unsigned long long)last_entry_local, (unsigned long long)now);
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

    buffer_sprintf(wb, PLUGINSD_KEYWORD_REPLAY_BEGIN " \"%s\"\n", rrdset_id(st));

    if(after != 0 && before != 0)
        before = replicate_chart_timeframe(wb, st, query_after, query_before, enable_streaming, now);
    else {
        after = 0;
        before = 0;
        enable_streaming = true;
    }

    // get again the world clock time
    time_t world_clock_time = now_realtime_sec();
    if(enable_streaming) {
        if(now < world_clock_time) {
            // we needed time to execute this request
            // so, the parent will need to replicate more data
            enable_streaming = false;
        }
        else
            replicate_chart_collection_state(wb, st);
    }

    // end with first/last entries we have, and the first start time and
    // last end time of the data we sent
    buffer_sprintf(wb, PLUGINSD_KEYWORD_REPLAY_END " %d %llu %llu %s %llu %llu %llu\n",

                   // current chart update every
                   (int)st->update_every

                   // child first db time, child end db time
                   , (unsigned long long)first_entry_local, (unsigned long long)last_entry_local

                   // start streaming boolean
                   , enable_streaming ? "true" : "false"

                   // after requested, before requested ('before' can be altered by the child when the request had enable_streaming true)
                   , (unsigned long long)after, (unsigned long long)before

                   // child world clock time
                   , (unsigned long long)world_clock_time
                   );

    sender_commit(host->sender, wb);

    return enable_streaming;
}

// ----------------------------------------------------------------------------
// sending replication requests

struct replication_request_details {
    struct {
        send_command callback;
        void *data;
    } caller;

    RRDHOST *host;
    RRDSET *st;

    struct {
        time_t first_entry_t;               // the first entry time the child has
        time_t last_entry_t;                // the last entry time the child has
        time_t world_time_t;                // the current time of the child
    } child_db;

    struct {
        time_t first_entry_t;               // the first entry time we have
        time_t last_entry_t;                // the last entry time we have
        bool last_entry_t_adjusted_to_now;  // true, if the last entry time was in the future and we fixed
        time_t now;                         // the current local world clock time
    } local_db;

    struct {
        time_t from;                        // the starting time of the entire gap we have
        time_t to;                          // the ending time of the entire gap we have
    } gap;

    struct {
        time_t after;                       // the start time we requested previously from this child
        time_t before;                      // the end time we requested previously from this child
    } last_request;

    struct {
        time_t after;                       // the start time of this replication request - the child will add 1 second
        time_t before;                      // the end time of this replication request
        bool start_streaming;               // true when we want the child to send anything remaining and start streaming - the child will overwrite 'before'
    } wanted;
};

static bool send_replay_chart_cmd(struct replication_request_details *r, const char *msg __maybe_unused) {
    RRDSET *st = r->st;

    if(st->rrdhost->receiver && (!st->rrdhost->receiver->replication_first_time_t || r->wanted.after < st->rrdhost->receiver->replication_first_time_t))
        st->rrdhost->receiver->replication_first_time_t = r->wanted.after;

#ifdef NETDATA_LOG_REPLICATION_REQUESTS
    st->replay.log_next_data_collection = true;

    char wanted_after_buf[LOG_DATE_LENGTH + 1] = "", wanted_before_buf[LOG_DATE_LENGTH + 1] = "";

    if(r->wanted.after)
        log_date(wanted_after_buf, LOG_DATE_LENGTH, r->wanted.after);

    if(r->wanted.before)
        log_date(wanted_before_buf, LOG_DATE_LENGTH, r->wanted.before);

    internal_error(true,
                   "REPLAY: 'host:%s/chart:%s' sending replication request %ld [%s] to %ld [%s], start streaming '%s': %s: "
                   "last[%ld - %ld] child[%ld - %ld, now %ld %s] local[%ld - %ld %s, now %ld] gap[%ld - %ld %s] %s"
                   , rrdhost_hostname(r->host), rrdset_id(r->st)
                   , r->wanted.after, wanted_after_buf
                   , r->wanted.before, wanted_before_buf
                   , r->wanted.start_streaming ? "YES" : "NO"
                   , msg
                   , r->last_request.after, r->last_request.before
                   , r->child_db.first_entry_t, r->child_db.last_entry_t
                   , r->child_db.world_time_t, (r->child_db.world_time_t == r->local_db.now) ? "SAME" : (r->child_db.world_time_t < r->local_db.now) ? "BEHIND" : "AHEAD"
                   , r->local_db.first_entry_t, r->local_db.last_entry_t
                   , r->local_db.last_entry_t_adjusted_to_now?"FIXED":"RAW", r->local_db.now
                   , r->gap.from, r->gap.to
                   , (r->gap.from == r->wanted.after) ? "FULL" : "PARTIAL"
                   , (st->replay.after != 0 || st->replay.before != 0) ? "OVERLAPPING" : ""
                   );

    st->replay.start_streaming = r->wanted.start_streaming;
    st->replay.after = r->wanted.after;
    st->replay.before = r->wanted.before;
#endif // NETDATA_LOG_REPLICATION_REQUESTS

    char buffer[2048 + 1];
    snprintfz(buffer, 2048, PLUGINSD_KEYWORD_REPLAY_CHART " \"%s\" \"%s\" %llu %llu\n",
              rrdset_id(st), r->wanted.start_streaming ? "true" : "false",
              (unsigned long long)r->wanted.after, (unsigned long long)r->wanted.before);

    int ret = r->caller.callback(buffer, r->caller.data);
    if (ret < 0) {
        error("REPLAY ERROR: 'host:%s/chart:%s' failed to send replication request to child (error %d)",
              rrdhost_hostname(r->host), rrdset_id(r->st), ret);
        return false;
    }

    return true;
}

bool replicate_chart_request(send_command callback, void *callback_data, RRDHOST *host, RRDSET *st,
                             time_t first_entry_child, time_t last_entry_child, time_t child_world_time,
                             time_t prev_first_entry_wanted, time_t prev_last_entry_wanted)
{
    struct replication_request_details r = {
            .caller = {
                    .callback = callback,
                    .data = callback_data,
            },

            .host = host,
            .st = st,

            .child_db = {
                    .first_entry_t = first_entry_child,
                    .last_entry_t = last_entry_child,
                    .world_time_t = child_world_time,
            },

            .local_db = {
                    .first_entry_t = rrdset_first_entry_t(st),
                    .last_entry_t = rrdset_last_entry_t(st),
                    .last_entry_t_adjusted_to_now = false,
                    .now  = now_realtime_sec(),
            },

            .last_request = {
                    .after = prev_first_entry_wanted,
                    .before = prev_last_entry_wanted,
            },

            .wanted = {
                    .after = 0,
                    .before = 0,
                    .start_streaming = true,
            },
    };

    // check our local database retention
    if(r.local_db.last_entry_t > r.local_db.now) {
        r.local_db.last_entry_t = r.local_db.now;
        r.local_db.last_entry_t_adjusted_to_now = true;
    }

    // let's find the GAP we have
    if(!r.last_request.after || !r.last_request.before) {
        // there is no previous request

        if(r.local_db.last_entry_t)
            // we have some data, let's continue from the last point we have
            r.gap.from = r.local_db.last_entry_t;
        else
            // we don't have any data, the gap is the max timeframe we are allowed to replicate
            r.gap.from = r.local_db.now - r.host->rrdpush_seconds_to_replicate;

    }
    else {
        // we had sent a request - let's continue at the point we left it
        // for this we don't take into account the actual data in our db
        // because the child may also have gaps and we need to get over it
        r.gap.from = r.last_request.before;
    }

    // we want all the data up to now
    r.gap.to = r.local_db.now;

    // The gap is now r.gap.from -> r.gap.to

    if (unlikely(!rrdhost_option_check(host, RRDHOST_OPTION_REPLICATION)))
        return send_replay_chart_cmd(&r, "empty replication request, replication is disabled");

    if (unlikely(!r.child_db.last_entry_t))
        return send_replay_chart_cmd(&r, "empty replication request, child has no stored data");

    if (unlikely(!rrdset_number_of_dimensions(st)))
        return send_replay_chart_cmd(&r, "empty replication request, chart has no dimensions");

    if (r.child_db.first_entry_t <= 0)
        return send_replay_chart_cmd(&r, "empty replication request, first entry of the child db first entry is invalid");

    if (r.child_db.first_entry_t > r.child_db.last_entry_t)
        return send_replay_chart_cmd(&r, "empty replication request, child timings are invalid (first entry > last entry)");

    if (r.local_db.last_entry_t > r.child_db.last_entry_t)
        return send_replay_chart_cmd(&r, "empty replication request, local last entry is later than the child one");

    // let's find what the child can provide to fill that gap

    if(r.child_db.first_entry_t > r.gap.from)
        // the child does not have all the data - let's get what it has
        r.wanted.after = r.child_db.first_entry_t;
    else
        // ok, the child can fill the entire gap we have
        r.wanted.after = r.gap.from;

    if(r.gap.to - r.wanted.after > host->rrdpush_replication_step)
        // the duration is too big for one request - let's take the first step
        r.wanted.before = r.wanted.after + host->rrdpush_replication_step;
    else
        // wow, we can do it in one request
        r.wanted.before = r.gap.to;

    // don't ask from the child more than it has
    if(r.wanted.before > r.child_db.last_entry_t)
        r.wanted.before = r.child_db.last_entry_t;

    if(r.wanted.after > r.wanted.before)
        r.wanted.after = r.wanted.before;

    // the child should start streaming immediately if the wanted duration is small or we reached the last entry of the child
    r.wanted.start_streaming = (r.local_db.now - r.wanted.after <= host->rrdpush_replication_step || r.wanted.before == r.child_db.last_entry_t);

    // the wanted timeframe is now r.wanted.after -> r.wanted.before
    // send it
    return send_replay_chart_cmd(&r, "OK");
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

    size_t pending;
    size_t added;
    size_t executed;
    size_t removed;
    size_t last_executed;
    time_t first_time_t;
    Word_t next_unique_id;
    struct replication_request *requests;

    Word_t last_after;
    Word_t last_unique_id;

    size_t skipped_not_connected;
    size_t skipped_no_room;
    size_t sender_resets;
    size_t waits;

    Pvoid_t JudyL_array;
} replication_globals = {
        .mutex = NETDATA_MUTEX_INITIALIZER,
        .pending = 0,
        .added = 0,
        .executed = 0,
        .last_executed = 0,
        .first_time_t = 0,
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
        netdata_mutex_lock(&replication_globals.mutex);

#ifdef NETDATA_INTERNAL_CHECKS
    if(replication_recursive_mutex_recursions < 0 || replication_recursive_mutex_recursions > 2)
        fatal("REPLICATION: recursions is %d", replication_recursive_mutex_recursions);
#endif
}

static void replication_recursive_unlock() {
    if(--replication_recursive_mutex_recursions == 0)
        netdata_mutex_unlock(&replication_globals.mutex);

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
    rse->unique_id = replication_globals.next_unique_id++;

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

    if(rq->after < (time_t)replication_globals.last_after) {
        // make it find this request first
        replication_globals.last_after = rq->after;
        replication_globals.last_unique_id = rq->unique_id;
    }

    replication_globals.added++;
    replication_globals.pending++;

    Pvoid_t *inner_judy_ptr;

    // find the outer judy entry, using after as key
    inner_judy_ptr = JudyLGet(replication_globals.JudyL_array, (Word_t) rq->after, PJE0);
    if(!inner_judy_ptr)
        inner_judy_ptr = JudyLIns(&replication_globals.JudyL_array, (Word_t) rq->after, PJE0);

    // add it to the inner judy, using unique_id as key
    Pvoid_t *item = JudyLIns(inner_judy_ptr, rq->unique_id, PJE0);
    *item = rse;
    rq->indexed_in_judy = true;

    if(!replication_globals.first_time_t || rq->after < replication_globals.first_time_t)
        replication_globals.first_time_t = rq->after;

    replication_recursive_unlock();

    return rse;
}

static bool replication_sort_entry_unlink_and_free_unsafe(struct replication_sort_entry *rse, Pvoid_t **inner_judy_ppptr) {
    bool inner_judy_deleted = false;

    replication_globals.removed++;
    replication_globals.pending--;

    rrdpush_sender_pending_replication_requests_minus_one(rse->rq->sender);

    rse->rq->indexed_in_judy = false;

    // delete it from the inner judy
    JudyLDel(*inner_judy_ppptr, rse->rq->unique_id, PJE0);

    // if no items left, delete it from the outer judy
    if(**inner_judy_ppptr == NULL) {
        JudyLDel(&replication_globals.JudyL_array, rse->rq->after, PJE0);
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

        inner_judy_pptr = JudyLGet(replication_globals.JudyL_array, rq->after, PJE0);
        if (inner_judy_pptr) {
            Pvoid_t *our_item_pptr = JudyLGet(*inner_judy_pptr, rq->unique_id, PJE0);
            if (our_item_pptr) {
                rse_to_delete = *our_item_pptr;
                replication_sort_entry_unlink_and_free_unsafe(rse_to_delete, &inner_judy_pptr);
            }
        }

        if (!rse_to_delete)
            fatal("REPLAY: 'host:%s/chart:%s' Cannot find sort entry to delete for time %ld.",
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


    if(unlikely(!replication_globals.last_after || !replication_globals.last_unique_id)) {
        replication_globals.last_after = 0;
        replication_globals.last_unique_id = 0;
    }

    bool find_same_after = true;
    while(!rq.found && (inner_judy_pptr = JudyLFirstOrNext(replication_globals.JudyL_array, &replication_globals.last_after, find_same_after))) {
        Pvoid_t *our_item_pptr;

        while(!rq.found && (our_item_pptr = JudyLNext(*inner_judy_pptr, &replication_globals.last_unique_id, PJE0))) {
            struct replication_sort_entry *rse = *our_item_pptr;
            struct sender_state *s = rse->rq->sender;

            bool sender_is_connected =
                    rrdhost_flag_check(s->host, RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED);

            bool sender_has_been_flushed_since_this_request =
                    rse->rq->sender_last_flush_ut != rrdpush_sender_get_flush_time(s);

            bool sender_has_room_to_spare =
                    s->buffer_used_percentage <= MAX_SENDER_BUFFER_PERCENTAGE_ALLOWED;

            if(unlikely(!sender_is_connected || sender_has_been_flushed_since_this_request)) {
                replication_globals.skipped_not_connected++;
                if(replication_sort_entry_unlink_and_free_unsafe(rse, &inner_judy_pptr))
                    break;
            }

            else if(sender_has_room_to_spare) {
                // copy the request to return it
                rq = *rse->rq;
                rq.chart_id = string_dup(rq.chart_id);

                // set the return result to found
                rq.found = true;

                if(replication_sort_entry_unlink_and_free_unsafe(rse, &inner_judy_pptr))
                    break;
            }
            else
                replication_globals.skipped_no_room++;
        }

        // call JudyLNext from now on
        find_same_after = false;

        // prepare for the next iteration on the outer loop
        replication_globals.last_unique_id = 0;
    }

    replication_recursive_unlock();
    return rq;
}

// ----------------------------------------------------------------------------
// replication request management

static void replication_request_react_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value __maybe_unused, void *sender_state __maybe_unused) {
    struct sender_state *s = sender_state; (void)s;
    struct replication_request *rq = value;

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

    replication_recursive_lock();

    if(!rq->indexed_in_judy) {
        replication_sort_entry_add(rq);
        internal_error(
                true,
                "STREAM %s [send to %s]: REPLAY: 'host:%s/chart:%s' adding duplicate replication command received (existing from %llu to %llu [%s], new from %llu to %llu [%s])",
                rrdhost_hostname(s->host), s->connected_to, rrdhost_hostname(s->host), dictionary_acquired_item_name(item),
                (unsigned long long)rq->after, (unsigned long long)rq->before, rq->start_streaming ? "true" : "false",
                (unsigned long long)rq_new->after, (unsigned long long)rq_new->before, rq_new->start_streaming ? "true" : "false");
    }
    else {
        internal_error(
                true,
                "STREAM %s [send to %s]: REPLAY: 'host:%s/chart:%s' ignoring duplicate replication command received (existing from %llu to %llu [%s], new from %llu to %llu [%s])",
                rrdhost_hostname(s->host), s->connected_to, rrdhost_hostname(s->host),
                dictionary_acquired_item_name(item),
                (unsigned long long) rq->after, (unsigned long long) rq->before, rq->start_streaming ? "true" : "false",
                (unsigned long long) rq_new->after, (unsigned long long) rq_new->before, rq_new->start_streaming ? "true" : "false");
    }

    replication_recursive_unlock();

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
        replication_globals.last_after = 0;
        replication_globals.last_unique_id = 0;
        replication_globals.sender_resets++;
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
#define WORKER_JOB_CHECK_CONSISTENCY                    15

#define ITERATIONS_IDLE_WITHOUT_PENDING_TO_RUN_SENDER_VERIFICATION 10

static size_t verify_host_charts_are_streaming_now(RRDHOST *host) {
    if(host->sender) {
        size_t pending_requests = host->sender->replication_pending_requests;
        size_t dict_entries = dictionary_entries(host->sender->replication_requests);

        internal_error(
                !pending_requests && dict_entries,
                "REPLICATION SUMMARY: 'host:%s' reports %zu pending replication requests, but its chart replication index says there are %zu charts pending replication",
                rrdhost_hostname(host), pending_requests, dict_entries);
    }

    size_t ok = 0;
    size_t errors = 0;

    RRDSET *st;
    rrdset_foreach_read(st, host) {
        RRDSET_FLAGS flags = rrdset_flag_check(st, RRDSET_FLAG_SENDER_REPLICATION_IN_PROGRESS | RRDSET_FLAG_SENDER_REPLICATION_FINISHED);

        bool is_error = false;

        if(!flags) {
            internal_error(
                    true,
                    "REPLICATION SUMMARY: 'host:%s/chart:%s' is neither IN PROGRESS nor FINISHED",
                    rrdhost_hostname(host), rrdset_id(st)
            );
            is_error = true;
        }

        if(!(flags & RRDSET_FLAG_SENDER_REPLICATION_FINISHED) || (flags & RRDSET_FLAG_SENDER_REPLICATION_IN_PROGRESS)) {
            internal_error(
                    true,
                    "REPLICATION SUMMARY: 'host:%s/chart:%s' is IN PROGRESS although replication is finished",
                    rrdhost_hostname(host), rrdset_id(st)
            );
            is_error = true;
        }

        if(is_error)
            errors++;
        else
            ok++;
    }
    rrdset_foreach_done(st);

    internal_error(errors,
                   "REPLICATION SUMMARY: 'host:%s' finished replicating %zu charts, but %zu charts are still in progress although replication finished",
                   rrdhost_hostname(host), ok, errors);

    return errors;
}

static void verify_all_hosts_charts_are_streaming_now(void) {
#ifdef NETDATA_INTERNAL_CHECKS
    worker_is_busy(WORKER_JOB_CHECK_CONSISTENCY);

    size_t errors = 0;
    RRDHOST *host;
    dfe_start_reentrant(rrdhost_root_index, host)
        errors += verify_host_charts_are_streaming_now(host);
    dfe_done(host);

    size_t executed = replication_globals.executed;
    internal_error(true, "REPLICATION SUMMARY: finished, executed %zu replication requests, %zu charts pending replication", executed - replication_globals.last_executed, errors);
    replication_globals.last_executed = executed;
#else
    ;
#endif
}

void *replication_thread_main(void *ptr __maybe_unused) {
    netdata_thread_cleanup_push(replication_main_cleanup, ptr);

    worker_register("REPLICATION");

    worker_register_job_name(WORKER_JOB_FIND_NEXT, "find next");
    worker_register_job_name(WORKER_JOB_QUERYING, "querying");
    worker_register_job_name(WORKER_JOB_DELETE_ENTRY, "dict delete");
    worker_register_job_name(WORKER_JOB_FIND_CHART, "find chart");
    worker_register_job_name(WORKER_JOB_ACTIVATE_ENABLE_STREAMING, "enable streaming");
    worker_register_job_name(WORKER_JOB_CHECK_CONSISTENCY, "check consistency");
    worker_register_job_name(WORKER_JOB_STATISTICS, "statistics");

    worker_register_job_custom_metric(WORKER_JOB_CUSTOM_METRIC_PENDING_REQUESTS, "pending requests", "requests", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(WORKER_JOB_CUSTOM_METRIC_COMPLETION, "completion", "%", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(WORKER_JOB_CUSTOM_METRIC_ADDED, "added requests", "requests/s", WORKER_METRIC_INCREMENTAL_TOTAL);
    worker_register_job_custom_metric(WORKER_JOB_CUSTOM_METRIC_DONE, "finished requests", "requests/s", WORKER_METRIC_INCREMENTAL_TOTAL);
    worker_register_job_custom_metric(WORKER_JOB_CUSTOM_METRIC_SKIPPED_NOT_CONNECTED, "not connected requests", "requests/s", WORKER_METRIC_INCREMENTAL_TOTAL);
    worker_register_job_custom_metric(WORKER_JOB_CUSTOM_METRIC_SKIPPED_NO_ROOM, "no room requests", "requests/s", WORKER_METRIC_INCREMENTAL_TOTAL);
    worker_register_job_custom_metric(WORKER_JOB_CUSTOM_METRIC_SENDER_RESETS, "sender resets", "resets/s", WORKER_METRIC_INCREMENTAL_TOTAL);
    worker_register_job_custom_metric(WORKER_JOB_CUSTOM_METRIC_WAITS, "waits", "waits/s", WORKER_METRIC_INCREMENTAL_TOTAL);

    // start from 100% completed
    worker_set_metric(WORKER_JOB_CUSTOM_METRIC_COMPLETION, 100.0);

    time_t latest_first_time_t = 0;
    long run_verification_countdown = LONG_MAX; // LONG_MAX to prevent an initial verification when no replication ever took place
    usec_t last_now_mono_ut = now_monotonic_usec();

    while(!netdata_exit) {

        // statistics
        usec_t now_mono_ut = now_monotonic_usec();
        if(unlikely(now_mono_ut - last_now_mono_ut > default_rrd_update_every * USEC_PER_SEC)) {
            last_now_mono_ut = now_mono_ut;

            if(!replication_globals.pending && run_verification_countdown-- == 0) {
                replication_globals.first_time_t = 0; // reset the statistics about completion percentage
                verify_all_hosts_charts_are_streaming_now();
            }

            worker_is_busy(WORKER_JOB_STATISTICS);

            if(latest_first_time_t && replication_globals.pending) {
                // completion percentage statistics
                time_t now = now_realtime_sec();
                time_t total = now - replication_globals.first_time_t;
                time_t done = latest_first_time_t - replication_globals.first_time_t;
                worker_set_metric(WORKER_JOB_CUSTOM_METRIC_COMPLETION,
                                  (NETDATA_DOUBLE) done * 100.0 / (NETDATA_DOUBLE) total);
            }
            else
                worker_set_metric(WORKER_JOB_CUSTOM_METRIC_COMPLETION, 100.0);

            worker_set_metric(WORKER_JOB_CUSTOM_METRIC_PENDING_REQUESTS, (NETDATA_DOUBLE)replication_globals.pending);
            worker_set_metric(WORKER_JOB_CUSTOM_METRIC_ADDED, (NETDATA_DOUBLE)replication_globals.added);
            worker_set_metric(WORKER_JOB_CUSTOM_METRIC_DONE, (NETDATA_DOUBLE)replication_globals.executed);
            worker_set_metric(WORKER_JOB_CUSTOM_METRIC_SKIPPED_NOT_CONNECTED, (NETDATA_DOUBLE)replication_globals.skipped_not_connected);
            worker_set_metric(WORKER_JOB_CUSTOM_METRIC_SKIPPED_NO_ROOM, (NETDATA_DOUBLE)replication_globals.skipped_no_room);
            worker_set_metric(WORKER_JOB_CUSTOM_METRIC_SENDER_RESETS, (NETDATA_DOUBLE)replication_globals.sender_resets);
            worker_set_metric(WORKER_JOB_CUSTOM_METRIC_WAITS, (NETDATA_DOUBLE)replication_globals.waits);
        }

        worker_is_busy(WORKER_JOB_FIND_NEXT);
        struct replication_request rq = replication_request_get_first_available();

        if(unlikely(!rq.found)) {
            // make it scan all the pending requests next time
            replication_globals.last_after = 0;
            replication_globals.last_unique_id = 0;

            replication_globals.waits++;

            worker_is_idle();
            sleep_usec(((replication_globals.pending) ? 10 : 1000) * USEC_PER_MS);
            continue;
        }

        run_verification_countdown = ITERATIONS_IDLE_WITHOUT_PENDING_TO_RUN_SENDER_VERIFICATION;

        // delete the request from the dictionary
        worker_is_busy(WORKER_JOB_DELETE_ENTRY);
        if(!dictionary_del(rq.sender->replication_requests, string2str(rq.chart_id)))
            error("REPLAY ERROR: 'host:%s/chart:%s' failed to be deleted from sender pending charts index",
                  rrdhost_hostname(rq.sender->host), string2str(rq.chart_id));

        worker_is_busy(WORKER_JOB_FIND_CHART);
        RRDSET *st = rrdset_find(rq.sender->host, string2str(rq.chart_id));
        if(!st) {
            internal_error(true, "REPLAY ERROR: 'host:%s/chart:%s' not found",
                           rrdhost_hostname(rq.sender->host), string2str(rq.chart_id));

            continue;
        }

        worker_is_busy(WORKER_JOB_QUERYING);

        latest_first_time_t = rq.after;

        if(rq.after < rq.sender->replication_first_time || !rq.sender->replication_first_time)
            rq.sender->replication_first_time = rq.after;

        if(rq.before < rq.sender->replication_current_time || !rq.sender->replication_current_time)
            rq.sender->replication_current_time = rq.before;

        netdata_thread_disable_cancelability();

        // send the replication data
        bool start_streaming = replicate_chart_response(
                st->rrdhost, st, rq.start_streaming, rq.after, rq.before);

        netdata_thread_enable_cancelability();

        replication_globals.executed++;

        if(start_streaming && rq.sender_last_flush_ut == rrdpush_sender_get_flush_time(rq.sender)) {
            worker_is_busy(WORKER_JOB_ACTIVATE_ENABLE_STREAMING);

            // enable normal streaming if we have to
            // but only if the sender buffer has not been flushed since we started

            if(rrdset_flag_check(st, RRDSET_FLAG_SENDER_REPLICATION_IN_PROGRESS)) {
                rrdset_flag_clear(st, RRDSET_FLAG_SENDER_REPLICATION_IN_PROGRESS);
                rrdset_flag_set(st, RRDSET_FLAG_SENDER_REPLICATION_FINISHED);
                rrdhost_sender_replicating_charts_minus_one(st->rrdhost);

#ifdef NETDATA_LOG_REPLICATION_REQUESTS
                internal_error(true, "STREAM_SENDER REPLAY: 'host:%s/chart:%s' streaming starts",
                               rrdhost_hostname(st->rrdhost), rrdset_id(st));
#endif
            }
            else
                internal_error(true, "REPLAY ERROR: 'host:%s/chart:%s' received start streaming command, but the chart is not in progress replicating",
                               rrdhost_hostname(st->rrdhost), string2str(rq.chart_id));
        }

        string_freez(rq.chart_id);
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}

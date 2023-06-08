// SPDX-License-Identifier: GPL-3.0-or-later

#include "replication.h"
#include "Judy.h"

#define STREAMING_START_MAX_SENDER_BUFFER_PERCENTAGE_ALLOWED 50ULL
#define MAX_REPLICATION_MESSAGE_PERCENT_SENDER_BUFFER 25ULL
#define MAX_SENDER_BUFFER_PERCENTAGE_ALLOWED 50ULL
#define MIN_SENDER_BUFFER_PERCENTAGE_ALLOWED 10ULL

#define WORKER_JOB_FIND_NEXT                            1
#define WORKER_JOB_QUERYING                             2
#define WORKER_JOB_DELETE_ENTRY                         3
#define WORKER_JOB_FIND_CHART                           4
#define WORKER_JOB_PREPARE_QUERY                        5
#define WORKER_JOB_CHECK_CONSISTENCY                    6
#define WORKER_JOB_BUFFER_COMMIT                        7
#define WORKER_JOB_CLEANUP                              8
#define WORKER_JOB_WAIT                                 9

// master thread worker jobs
#define WORKER_JOB_STATISTICS                           10
#define WORKER_JOB_CUSTOM_METRIC_PENDING_REQUESTS       11
#define WORKER_JOB_CUSTOM_METRIC_SKIPPED_NO_ROOM        12
#define WORKER_JOB_CUSTOM_METRIC_COMPLETION             13
#define WORKER_JOB_CUSTOM_METRIC_ADDED                  14
#define WORKER_JOB_CUSTOM_METRIC_DONE                   15
#define WORKER_JOB_CUSTOM_METRIC_SENDER_RESETS          16
#define WORKER_JOB_CUSTOM_METRIC_SENDER_FULL            17

#define ITERATIONS_IDLE_WITHOUT_PENDING_TO_RUN_SENDER_VERIFICATION 30
#define SECONDS_TO_RESET_POINT_IN_TIME 10

static struct replication_query_statistics replication_queries = {
        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
        .queries_started = 0,
        .queries_finished = 0,
        .points_read = 0,
        .points_generated = 0,
};

struct replication_query_statistics replication_get_query_statistics(void) {
    netdata_spinlock_lock(&replication_queries.spinlock);
    struct replication_query_statistics ret = replication_queries;
    netdata_spinlock_unlock(&replication_queries.spinlock);
    return ret;
}

size_t replication_buffers_allocated = 0;

size_t replication_allocated_buffers(void) {
    return __atomic_load_n(&replication_buffers_allocated, __ATOMIC_RELAXED);
}

// ----------------------------------------------------------------------------
// sending replication replies

struct replication_dimension {
    STORAGE_POINT sp;
    struct storage_engine_query_handle handle;
    bool enabled;
    bool skip;

    DICTIONARY *dict;
    const DICTIONARY_ITEM *rda;
    RRDDIM *rd;
};

struct replication_query {
    RRDSET *st;

    struct {
        time_t first_entry_t;
        time_t last_entry_t;
    } db;

    struct {                         // what the parent requested
        time_t after;
        time_t before;
        bool enable_streaming;
    } request;

    struct {                         // what the child will do
        time_t after;
        time_t before;
        bool enable_streaming;

        bool locked_data_collection;
        bool execute;
        bool interrupted;
        STREAM_CAPABILITIES capabilities;
    } query;

    time_t wall_clock_time;

    size_t points_read;
    size_t points_generated;

    STORAGE_ENGINE_BACKEND backend;
    struct replication_request *rq;

    size_t dimensions;
    struct replication_dimension data[];
};

static struct replication_query *replication_query_prepare(
        RRDSET *st,
        time_t db_first_entry,
        time_t db_last_entry,
        time_t requested_after,
        time_t requested_before,
        bool requested_enable_streaming,
        time_t query_after,
        time_t query_before,
        bool query_enable_streaming,
        time_t wall_clock_time,
        STREAM_CAPABILITIES capabilities
) {
    size_t dimensions = rrdset_number_of_dimensions(st);
    struct replication_query *q = callocz(1, sizeof(struct replication_query) + dimensions * sizeof(struct replication_dimension));
    __atomic_add_fetch(&replication_buffers_allocated, sizeof(struct replication_query) + dimensions * sizeof(struct replication_dimension), __ATOMIC_RELAXED);

    q->dimensions = dimensions;
    q->st = st;

    q->db.first_entry_t = db_first_entry;
    q->db.last_entry_t = db_last_entry;

    q->request.after = requested_after,
    q->request.before = requested_before,
    q->request.enable_streaming = requested_enable_streaming,

    q->query.after = query_after;
    q->query.before = query_before;
    q->query.enable_streaming = query_enable_streaming;
    q->query.capabilities = capabilities;

    q->wall_clock_time = wall_clock_time;

    if (!q->dimensions || !q->query.after || !q->query.before) {
        q->query.execute = false;
        q->dimensions = 0;
        return q;
    }

    if(q->query.enable_streaming) {
        netdata_spinlock_lock(&st->data_collection_lock);
        q->query.locked_data_collection = true;

        if (st->last_updated.tv_sec > q->query.before) {
#ifdef NETDATA_LOG_REPLICATION_REQUESTS
            internal_error(true,
                           "STREAM_SENDER REPLAY: 'host:%s/chart:%s' "
                           "has start_streaming = true, "
                           "adjusting replication before timestamp from %llu to %llu",
                           rrdhost_hostname(st->rrdhost), rrdset_id(st),
                           (unsigned long long) q->query.before,
                           (unsigned long long) st->last_updated.tv_sec
            );
#endif
            q->query.before = MIN(st->last_updated.tv_sec, wall_clock_time);
        }
    }

    q->backend = st->rrdhost->db[0].eng->backend;

    // prepare our array of dimensions
    size_t count = 0;
    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        if (unlikely(!rd || !rd_dfe.item || !rd->exposed))
            continue;

        if (unlikely(rd_dfe.counter >= q->dimensions)) {
            internal_error(true,
                           "STREAM_SENDER REPLAY ERROR: 'host:%s/chart:%s' has more dimensions than the replicated ones",
                           rrdhost_hostname(st->rrdhost), rrdset_id(st));
            break;
        }

        struct replication_dimension *d = &q->data[rd_dfe.counter];

        d->dict = rd_dfe.dict;
        d->rda = dictionary_acquired_item_dup(rd_dfe.dict, rd_dfe.item);
        d->rd = rd;

        storage_engine_query_init(q->backend, rd->tiers[0].db_metric_handle, &d->handle, q->query.after, q->query.before,
                     q->query.locked_data_collection ? STORAGE_PRIORITY_HIGH : STORAGE_PRIORITY_LOW);
        d->enabled = true;
        d->skip = false;
        count++;
    }
    rrddim_foreach_done(rd);

    if(!count) {
        // no data for this chart

        q->query.execute = false;

        if(q->query.locked_data_collection) {
            netdata_spinlock_unlock(&st->data_collection_lock);
            q->query.locked_data_collection = false;
        }

    }
    else {
        // we have data for this chart

        q->query.execute = true;
    }

    return q;
}

static void replication_send_chart_collection_state(BUFFER *wb, RRDSET *st, STREAM_CAPABILITIES capabilities) {
    NUMBER_ENCODING encoding = (capabilities & STREAM_CAP_IEEE754) ? NUMBER_ENCODING_BASE64 : NUMBER_ENCODING_DECIMAL;
    RRDDIM *rd;
    rrddim_foreach_read(rd, st){
        if (!rd->exposed) continue;

        buffer_fast_strcat(wb, PLUGINSD_KEYWORD_REPLAY_RRDDIM_STATE " '",
                           sizeof(PLUGINSD_KEYWORD_REPLAY_RRDDIM_STATE) - 1 + 2);
        buffer_fast_strcat(wb, rrddim_id(rd), string_strlen(rd->id));
        buffer_fast_strcat(wb, "' ", 2);
        buffer_print_uint64_encoded(wb, encoding, (usec_t) rd->last_collected_time.tv_sec * USEC_PER_SEC +
                                    (usec_t) rd->last_collected_time.tv_usec);
        buffer_fast_strcat(wb, " ", 1);
        buffer_print_int64_encoded(wb, encoding, rd->last_collected_value);
        buffer_fast_strcat(wb, " ", 1);
        buffer_print_netdata_double_encoded(wb, encoding, rd->last_calculated_value);
        buffer_fast_strcat(wb, " ", 1);
        buffer_print_netdata_double_encoded(wb, encoding, rd->last_stored_value);
        buffer_fast_strcat(wb, "\n", 1);
    }
    rrddim_foreach_done(rd);

    buffer_fast_strcat(wb, PLUGINSD_KEYWORD_REPLAY_RRDSET_STATE " ", sizeof(PLUGINSD_KEYWORD_REPLAY_RRDSET_STATE) - 1 + 1);
    buffer_print_uint64_encoded(wb, encoding, (usec_t) st->last_collected_time.tv_sec * USEC_PER_SEC + (usec_t) st->last_collected_time.tv_usec);
    buffer_fast_strcat(wb, " ", 1);
    buffer_print_uint64_encoded(wb, encoding, (usec_t) st->last_updated.tv_sec * USEC_PER_SEC + (usec_t) st->last_updated.tv_usec);
    buffer_fast_strcat(wb, "\n", 1);
}

static void replication_query_finalize(BUFFER *wb, struct replication_query *q, bool executed) {
    size_t dimensions = q->dimensions;

    if(wb && q->query.enable_streaming)
        replication_send_chart_collection_state(wb, q->st, q->query.capabilities);

    if(q->query.locked_data_collection) {
        netdata_spinlock_unlock(&q->st->data_collection_lock);
        q->query.locked_data_collection = false;
    }

    // release all the dictionary items acquired
    // finalize the queries
    size_t queries = 0;

    for (size_t i = 0; i < dimensions; i++) {
        struct replication_dimension *d = &q->data[i];
        if (unlikely(!d->enabled)) continue;

        storage_engine_query_finalize(&d->handle);

        dictionary_acquired_item_release(d->dict, d->rda);

        // update global statistics
        queries++;
    }

    if(executed) {
        netdata_spinlock_lock(&replication_queries.spinlock);
        replication_queries.queries_started += queries;
        replication_queries.queries_finished += queries;
        replication_queries.points_read += q->points_read;
        replication_queries.points_generated += q->points_generated;

        if(q->st && q->st->rrdhost->sender) {
            struct sender_state *s = q->st->rrdhost->sender;
            s->replication.latest_completed_before_t = q->query.before;
        }

        netdata_spinlock_unlock(&replication_queries.spinlock);
    }

    __atomic_sub_fetch(&replication_buffers_allocated, sizeof(struct replication_query) + dimensions * sizeof(struct replication_dimension), __ATOMIC_RELAXED);
    freez(q);
}

static void replication_query_align_to_optimal_before(struct replication_query *q) {
    if(!q->query.execute || q->query.enable_streaming)
        return;

    size_t dimensions = q->dimensions;
    time_t expanded_before = 0;

    for (size_t i = 0; i < dimensions; i++) {
        struct replication_dimension *d = &q->data[i];
        if(unlikely(!d->enabled)) continue;

        time_t new_before = storage_engine_align_to_optimal_before(&d->handle);
        if (!expanded_before || new_before < expanded_before)
            expanded_before = new_before;
    }

    if(expanded_before > q->query.before                                 && // it is later than the original
        (expanded_before - q->query.before) / q->st->update_every < 1024 && // it is reasonable (up to a page)
        expanded_before < q->st->last_updated.tv_sec                     && // it is not the chart's last updated time
        expanded_before < q->wall_clock_time)                               // it is not later than the wall clock time
        q->query.before = expanded_before;
}

static bool replication_query_execute(BUFFER *wb, struct replication_query *q, size_t max_msg_size) {
    replication_query_align_to_optimal_before(q);

    NUMBER_ENCODING encoding = (q->query.capabilities & STREAM_CAP_IEEE754) ? NUMBER_ENCODING_BASE64 : NUMBER_ENCODING_DECIMAL;
    time_t after = q->query.after;
    time_t before = q->query.before;
    size_t dimensions = q->dimensions;
    time_t wall_clock_time = q->wall_clock_time;

    bool finished_with_gap = false;
    size_t points_read = 0, points_generated = 0;

#ifdef NETDATA_LOG_REPLICATION_REQUESTS
    time_t actual_after = 0, actual_before = 0;
#endif

    time_t now = after + 1;
    time_t last_end_time_in_buffer = 0;
    while(now <= before) {
        time_t min_start_time = 0, max_start_time = 0, min_end_time = 0, max_end_time = 0, min_update_every = 0, max_update_every = 0;
        for (size_t i = 0; i < dimensions ;i++) {
            struct replication_dimension *d = &q->data[i];
            if(unlikely(!d->enabled || d->skip)) continue;

            // fetch the first valid point for the dimension
            int max_skip = 1000;
            while(d->sp.end_time_s < now && !storage_engine_query_is_finished(&d->handle) && max_skip-- >= 0) {
                d->sp = storage_engine_query_next_metric(&d->handle);
                points_read++;
            }

            if(max_skip <= 0) {
                d->skip = true;

                error_limit_static_global_var(erl, 1, 0);
                error_limit(&erl,
                            "STREAM_SENDER REPLAY ERROR: 'host:%s/chart:%s/dim:%s': db does not advance the query "
                            "beyond time %llu (tried 1000 times to get the next point and always got back a point in the past)",
                            rrdhost_hostname(q->st->rrdhost), rrdset_id(q->st), rrddim_id(d->rd),
                            (unsigned long long) now);

                continue;
            }

            if(unlikely(d->sp.end_time_s < now || d->sp.end_time_s < d->sp.start_time_s))
                // this dimension does not provide any data
                continue;

            time_t update_every = d->sp.end_time_s - d->sp.start_time_s;
            if(unlikely(!update_every))
                update_every = q->st->update_every;

            if(unlikely(!min_update_every))
                min_update_every = update_every;

            if(unlikely(!min_start_time))
                min_start_time = d->sp.start_time_s;

            if(unlikely(!min_end_time))
                min_end_time = d->sp.end_time_s;

            min_update_every = MIN(min_update_every, update_every);
            max_update_every = MAX(max_update_every, update_every);

            min_start_time = MIN(min_start_time, d->sp.start_time_s);
            max_start_time = MAX(max_start_time, d->sp.start_time_s);

            min_end_time = MIN(min_end_time, d->sp.end_time_s);
            max_end_time = MAX(max_end_time, d->sp.end_time_s);
        }

        if (unlikely(min_update_every != max_update_every ||
                     min_start_time != max_start_time)) {

            time_t fix_min_start_time;
            if(last_end_time_in_buffer &&
                last_end_time_in_buffer >= min_start_time &&
                last_end_time_in_buffer <= max_start_time) {
                fix_min_start_time = last_end_time_in_buffer;
            }
            else
                fix_min_start_time = min_end_time - min_update_every;

#ifdef NETDATA_INTERNAL_CHECKS
            error_limit_static_global_var(erl, 1, 0);
            error_limit(&erl, "REPLAY WARNING: 'host:%s/chart:%s' "
                              "misaligned dimensions, "
                              "update every (min: %ld, max: %ld), "
                              "start time (min: %ld, max: %ld), "
                              "end time (min %ld, max %ld), "
                              "now %ld, last end time sent %ld, "
                              "min start time is fixed to %ld",
                        rrdhost_hostname(q->st->rrdhost), rrdset_id(q->st),
                        min_update_every, max_update_every,
                        min_start_time, max_start_time,
                        min_end_time, max_end_time,
                        now, last_end_time_in_buffer,
                        fix_min_start_time
                        );
#endif

            min_start_time = fix_min_start_time;
        }

        if(likely(min_start_time <= now && min_end_time >= now)) {
            // we have a valid point

            if (unlikely(min_end_time == min_start_time))
                min_start_time = min_end_time - q->st->update_every;

#ifdef NETDATA_LOG_REPLICATION_REQUESTS
            if (unlikely(!actual_after))
                actual_after = min_end_time;

            actual_before = min_end_time;
#endif

            if(buffer_strlen(wb) > max_msg_size && last_end_time_in_buffer) {
                q->query.before = last_end_time_in_buffer;
                q->query.enable_streaming = false;

                internal_error(true, "REPLICATION: current buffer size %zu is more than the "
                                     "max message size %zu for chart '%s' of host '%s'. "
                                     "Interrupting replication request (%ld to %ld, %s) at %ld to %ld, %s.",
                               buffer_strlen(wb), max_msg_size, rrdset_id(q->st), rrdhost_hostname(q->st->rrdhost),
                               q->request.after, q->request.before, q->request.enable_streaming?"true":"false",
                               q->query.after, q->query.before, q->query.enable_streaming?"true":"false");

                q->query.interrupted = true;

                break;
            }
            last_end_time_in_buffer = min_end_time;

            buffer_fast_strcat(wb, PLUGINSD_KEYWORD_REPLAY_BEGIN " '' ", sizeof(PLUGINSD_KEYWORD_REPLAY_BEGIN) - 1 + 4);
            buffer_print_uint64_encoded(wb, encoding, min_start_time);
            buffer_fast_strcat(wb, " ", 1);
            buffer_print_uint64_encoded(wb, encoding, min_end_time);
            buffer_fast_strcat(wb, " ", 1);
            buffer_print_uint64_encoded(wb, encoding, wall_clock_time);
            buffer_fast_strcat(wb, "\n", 1);

            // output the replay values for this time
            for (size_t i = 0; i < dimensions; i++) {
                struct replication_dimension *d = &q->data[i];
                if (unlikely(!d->enabled)) continue;

                if (likely( d->sp.start_time_s <= min_end_time &&
                            d->sp.end_time_s >= min_end_time &&
                            !storage_point_is_unset(d->sp) &&
                            !storage_point_is_gap(d->sp))) {

                    buffer_fast_strcat(wb, PLUGINSD_KEYWORD_REPLAY_SET " \"", sizeof(PLUGINSD_KEYWORD_REPLAY_SET) - 1 + 2);
                    buffer_fast_strcat(wb, rrddim_id(d->rd), string_strlen(d->rd->id));
                    buffer_fast_strcat(wb, "\" ", 2);
                    buffer_print_netdata_double_encoded(wb, encoding, d->sp.sum);
                    buffer_fast_strcat(wb, " ", 1);
                    buffer_print_sn_flags(wb, d->sp.flags, q->query.capabilities & STREAM_CAP_INTERPOLATED);
                    buffer_fast_strcat(wb, "\n", 1);

                    points_generated++;
                }
            }

            now = min_end_time + 1;
        }
        else if(unlikely(min_end_time < now))
            // the query does not progress
            break;
        else {
            // we have gap - all points are in the future
            now = min_start_time;

            if(min_start_time > before && !points_generated) {
                before = q->query.before = min_start_time - 1;
                finished_with_gap = true;
                break;
            }
        }
    }

#ifdef NETDATA_LOG_REPLICATION_REQUESTS
    if(actual_after) {
        char actual_after_buf[LOG_DATE_LENGTH + 1], actual_before_buf[LOG_DATE_LENGTH + 1];
        log_date(actual_after_buf, LOG_DATE_LENGTH, actual_after);
        log_date(actual_before_buf, LOG_DATE_LENGTH, actual_before);
        internal_error(true,
                       "STREAM_SENDER REPLAY: 'host:%s/chart:%s': sending data %llu [%s] to %llu [%s] (requested %llu [delta %lld] to %llu [delta %lld])",
                       rrdhost_hostname(q->st->rrdhost), rrdset_id(q->st),
                       (unsigned long long)actual_after, actual_after_buf, (unsigned long long)actual_before, actual_before_buf,
                       (unsigned long long)after, (long long)(actual_after - after), (unsigned long long)before, (long long)(actual_before - before));
    }
    else
        internal_error(true,
                       "STREAM_SENDER REPLAY: 'host:%s/chart:%s': nothing to send (requested %llu to %llu)",
                       rrdhost_hostname(q->st->rrdhost), rrdset_id(q->st),
                       (unsigned long long)after, (unsigned long long)before);
#endif // NETDATA_LOG_REPLICATION_REQUESTS

    q->points_read += points_read;
    q->points_generated += points_generated;

    if(last_end_time_in_buffer < before - q->st->update_every)
        finished_with_gap = true;

    return finished_with_gap;
}

static struct replication_query *replication_response_prepare(
        RRDSET *st,
        bool requested_enable_streaming,
        time_t requested_after,
        time_t requested_before,
        STREAM_CAPABILITIES capabilities
        ) {
    time_t wall_clock_time = now_realtime_sec();

    if(requested_after > requested_before) {
        // flip them
        time_t t = requested_before;
        requested_before = requested_after;
        requested_after = t;
    }

    if(requested_after > wall_clock_time) {
        requested_after = 0;
        requested_before = 0;
        requested_enable_streaming = true;
    }

    if(requested_before > wall_clock_time) {
        requested_before = wall_clock_time;
        requested_enable_streaming = true;
    }

    time_t query_after = requested_after;
    time_t query_before = requested_before;
    bool query_enable_streaming = requested_enable_streaming;

    time_t db_first_entry = 0, db_last_entry = 0;
    rrdset_get_retention_of_tier_for_collected_chart(
            st, &db_first_entry, &db_last_entry, wall_clock_time, 0);

    if(requested_after == 0 && requested_before == 0 && requested_enable_streaming == true) {
        // no data requested - just enable streaming
        ;
    }
    else {
        if (query_after < db_first_entry)
            query_after = db_first_entry;

        if (query_before > db_last_entry)
            query_before = db_last_entry;

        // if the parent asked us to start streaming, then fill the rest with the data that we have
        if (requested_enable_streaming)
            query_before = db_last_entry;

        if (query_after > query_before) {
            time_t tmp = query_before;
            query_before = query_after;
            query_after = tmp;
        }

        query_enable_streaming = (requested_enable_streaming ||
                                  query_before == db_last_entry ||
                                  !requested_after ||
                                  !requested_before) ? true : false;
    }

    return replication_query_prepare(
            st,
            db_first_entry, db_last_entry,
            requested_after, requested_before, requested_enable_streaming,
            query_after, query_before, query_enable_streaming,
            wall_clock_time, capabilities);
}

void replication_response_cancel_and_finalize(struct replication_query *q) {
    replication_query_finalize(NULL, q, false);
}

static bool sender_is_still_connected_for_this_request(struct replication_request *rq);

bool replication_response_execute_and_finalize(struct replication_query *q, size_t max_msg_size) {
    NUMBER_ENCODING encoding = (q->query.capabilities & STREAM_CAP_IEEE754) ? NUMBER_ENCODING_BASE64 : NUMBER_ENCODING_DECIMAL;
    struct replication_request *rq = q->rq;
    RRDSET *st = q->st;
    RRDHOST *host = st->rrdhost;

    // we might want to optimize this by filling a temporary buffer
    // and copying the result to the host's buffer in order to avoid
    // holding the host's buffer lock for too long
    BUFFER *wb = sender_start(host->sender);

    buffer_fast_strcat(wb, PLUGINSD_KEYWORD_REPLAY_BEGIN " '", sizeof(PLUGINSD_KEYWORD_REPLAY_BEGIN) - 1 + 2);
    buffer_fast_strcat(wb, rrdset_id(st), string_strlen(st->id));
    buffer_fast_strcat(wb, "'\n", 2);

//    buffer_sprintf(wb, PLUGINSD_KEYWORD_REPLAY_BEGIN " \"%s\"\n", rrdset_id(st));

    bool locked_data_collection = q->query.locked_data_collection;
    q->query.locked_data_collection = false;

    bool finished_with_gap = false;
    if(q->query.execute)
        finished_with_gap = replication_query_execute(wb, q, max_msg_size);

    time_t after = q->request.after;
    time_t before = q->query.before;
    bool enable_streaming = q->query.enable_streaming;

    replication_query_finalize(wb, q, q->query.execute);
    q = NULL; // IMPORTANT: q is invalid now

    // get a fresh retention to send to the parent
    time_t wall_clock_time = now_realtime_sec();
    time_t db_first_entry, db_last_entry;
    rrdset_get_retention_of_tier_for_collected_chart(st, &db_first_entry, &db_last_entry, wall_clock_time, 0);

    // end with first/last entries we have, and the first start time and
    // last end time of the data we sent

    buffer_fast_strcat(wb, PLUGINSD_KEYWORD_REPLAY_END " ", sizeof(PLUGINSD_KEYWORD_REPLAY_END) - 1 + 1);
    buffer_print_int64_encoded(wb, encoding, st->update_every);
    buffer_fast_strcat(wb, " ", 1);
    buffer_print_uint64_encoded(wb, encoding, db_first_entry);
    buffer_fast_strcat(wb, " ", 1);
    buffer_print_uint64_encoded(wb, encoding, db_last_entry);

    buffer_fast_strcat(wb, enable_streaming ? " true  " : " false ", 7);

    buffer_print_uint64_encoded(wb, encoding, after);
    buffer_fast_strcat(wb, " ", 1);
    buffer_print_uint64_encoded(wb, encoding, before);
    buffer_fast_strcat(wb, " ", 1);
    buffer_print_uint64_encoded(wb, encoding, wall_clock_time);
    buffer_fast_strcat(wb, "\n", 1);

    worker_is_busy(WORKER_JOB_BUFFER_COMMIT);
    sender_commit(host->sender, wb, STREAM_TRAFFIC_TYPE_REPLICATION);
    worker_is_busy(WORKER_JOB_CLEANUP);

    if(enable_streaming) {
        if(sender_is_still_connected_for_this_request(rq)) {
            // enable normal streaming if we have to
            // but only if the sender buffer has not been flushed since we started

            if(rrdset_flag_check(st, RRDSET_FLAG_SENDER_REPLICATION_IN_PROGRESS)) {
                rrdset_flag_clear(st, RRDSET_FLAG_SENDER_REPLICATION_IN_PROGRESS);
                rrdset_flag_set(st, RRDSET_FLAG_SENDER_REPLICATION_FINISHED);
                rrdhost_sender_replicating_charts_minus_one(st->rrdhost);

                if(!finished_with_gap)
                    st->upstream_resync_time_s = 0;

#ifdef NETDATA_LOG_REPLICATION_REQUESTS
                internal_error(true, "STREAM_SENDER REPLAY: 'host:%s/chart:%s' streaming starts",
                           rrdhost_hostname(st->rrdhost), rrdset_id(st));
#endif
            }
            else
                internal_error(true, "REPLAY ERROR: 'host:%s/chart:%s' received start streaming command, but the chart is not in progress replicating",
                               rrdhost_hostname(st->rrdhost), rrdset_id(st));
        }
    }

    if(locked_data_collection)
        netdata_spinlock_unlock(&st->data_collection_lock);

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
        time_t wall_clock_time;             // the current time of the child
        bool fixed_last_entry;              // when set we set the last entry to wall clock time
    } child_db;

    struct {
        time_t first_entry_t;               // the first entry time we have
        time_t last_entry_t;                // the last entry time we have
        time_t wall_clock_time;                         // the current local world clock time
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

static void replicate_log_request(struct replication_request_details *r, const char *msg) {
#ifdef NETDATA_INTERNAL_CHECKS
    internal_error(true,
#else
    error_limit_static_global_var(erl, 1, 0);
    error_limit(&erl,
#endif
                "REPLAY ERROR: 'host:%s/chart:%s' child sent: "
                "db from %ld to %ld%s, wall clock time %ld, "
                "last request from %ld to %ld, "
                "issue: %s - "
                "sending replication request from %ld to %ld, start streaming %s",
                rrdhost_hostname(r->st->rrdhost), rrdset_id(r->st),
                r->child_db.first_entry_t,
                r->child_db.last_entry_t, r->child_db.fixed_last_entry ? " (fixed)" : "",
                r->child_db.wall_clock_time,
                r->last_request.after,
                r->last_request.before,
                msg,
                r->wanted.after,
                r->wanted.before,
                r->wanted.start_streaming ? "true" : "false");
}

static bool send_replay_chart_cmd(struct replication_request_details *r, const char *msg, bool log) {
    RRDSET *st = r->st;

    if(log)
        replicate_log_request(r, msg);

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
                   "last[%ld - %ld] child[%ld - %ld, now %ld %s] local[%ld - %ld, now %ld] gap[%ld - %ld %s] %s"
                   , rrdhost_hostname(r->host), rrdset_id(r->st)
                   , r->wanted.after, wanted_after_buf
                   , r->wanted.before, wanted_before_buf
                   , r->wanted.start_streaming ? "YES" : "NO"
                   , msg
                   , r->last_request.after, r->last_request.before
                   , r->child_db.first_entry_t, r->child_db.last_entry_t
                   , r->child_db.wall_clock_time, (r->child_db.wall_clock_time == r->local_db.wall_clock_time) ? "SAME" : (r->child_db.wall_clock_time < r->local_db.wall_clock_time) ? "BEHIND" : "AHEAD"
                   , r->local_db.first_entry_t, r->local_db.last_entry_t
                   , r->local_db.wall_clock_time
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
                             time_t child_first_entry, time_t child_last_entry, time_t child_wall_clock_time,
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
                    .first_entry_t = child_first_entry,
                    .last_entry_t = child_last_entry,
                    .wall_clock_time = child_wall_clock_time,
                    .fixed_last_entry = false,
            },

            .local_db = {
                    .first_entry_t = 0,
                    .last_entry_t = 0,
                    .wall_clock_time  = now_realtime_sec(),
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

    if(r.child_db.last_entry_t > r.child_db.wall_clock_time) {
        replicate_log_request(&r, "child's db last entry > child's wall clock time");
        r.child_db.last_entry_t = r.child_db.wall_clock_time;
        r.child_db.fixed_last_entry = true;
    }

    rrdset_get_retention_of_tier_for_collected_chart(r.st, &r.local_db.first_entry_t, &r.local_db.last_entry_t, r.local_db.wall_clock_time, 0);

    // let's find the GAP we have
    if(!r.last_request.after || !r.last_request.before) {
        // there is no previous request

        if(r.local_db.last_entry_t)
            // we have some data, let's continue from the last point we have
            r.gap.from = r.local_db.last_entry_t;
        else
            // we don't have any data, the gap is the max timeframe we are allowed to replicate
            r.gap.from = r.local_db.wall_clock_time - r.host->rrdpush_seconds_to_replicate;

    }
    else {
        // we had sent a request - let's continue at the point we left it
        // for this we don't take into account the actual data in our db
        // because the child may also have gaps, and we need to get over it
        r.gap.from = r.last_request.before;
    }

    // we want all the data up to now
    r.gap.to = r.local_db.wall_clock_time;

    // The gap is now r.gap.from -> r.gap.to

    if (unlikely(!rrdhost_option_check(host, RRDHOST_OPTION_REPLICATION)))
        return send_replay_chart_cmd(&r, "empty replication request, replication is disabled", false);

    if (unlikely(!rrdset_number_of_dimensions(st)))
        return send_replay_chart_cmd(&r, "empty replication request, chart has no dimensions", false);

    if (unlikely(!r.child_db.first_entry_t || !r.child_db.last_entry_t))
        return send_replay_chart_cmd(&r, "empty replication request, child has no stored data", false);

    if (unlikely(r.child_db.first_entry_t < 0 || r.child_db.last_entry_t < 0))
        return send_replay_chart_cmd(&r, "empty replication request, child db timestamps are invalid", true);

    if (unlikely(r.child_db.first_entry_t > r.child_db.wall_clock_time))
        return send_replay_chart_cmd(&r, "empty replication request, child db first entry is after its wall clock time", true);

    if (unlikely(r.child_db.first_entry_t > r.child_db.last_entry_t))
        return send_replay_chart_cmd(&r, "empty replication request, child timings are invalid (first entry > last entry)", true);

    if (unlikely(r.local_db.last_entry_t > r.child_db.last_entry_t))
        return send_replay_chart_cmd(&r, "empty replication request, local last entry is later than the child one", false);

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

    if(r.wanted.after > r.wanted.before) {
        r.wanted.after = 0;
        r.wanted.before = 0;
        r.wanted.start_streaming = true;
        return send_replay_chart_cmd(&r, "empty replication request, wanted after computed bigger than wanted before", true);
    }

    // the child should start streaming immediately if the wanted duration is small, or we reached the last entry of the child
    r.wanted.start_streaming = (r.local_db.wall_clock_time - r.wanted.after <= host->rrdpush_replication_step ||
            r.wanted.before >= r.child_db.last_entry_t ||
            r.wanted.before >= r.child_db.wall_clock_time ||
            r.wanted.before >= r.local_db.wall_clock_time);

    // the wanted timeframe is now r.wanted.after -> r.wanted.before
    // send it
    return send_replay_chart_cmd(&r, "OK", false);
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

    usec_t sender_last_flush_ut;        // the timestamp of the sender, at the time we indexed this request
    Word_t unique_id;                   // auto-increment, later requests have bigger

    bool start_streaming;               // true, when the parent wants to send the rest of the data (before is overwritten) and enable normal streaming
    bool indexed_in_judy;               // true when the request is indexed in judy
    bool not_indexed_buffer_full;       // true when the request is not indexed because the sender is full
    bool not_indexed_preprocessing;     // true when the request is not indexed, but it is pending in preprocessing

    // prepare ahead members - preprocessing
    bool found;                         // used as a result boolean for the find call
    bool executed;                      // used to detect if we have skipped requests while preprocessing
    RRDSET *st;                         // caching of the chart during preprocessing
    struct replication_query *q;        // the preprocessing query initialization
};

// replication sort entry in JudyL array
// used for sorting all requests, across all nodes
struct replication_sort_entry {
    struct replication_request *rq;

    size_t unique_id;              // used as a key to identify the sort entry - we never access its contents
};

#define MAX_REPLICATION_THREADS 20 // + 1 for the main thread

// the global variables for the replication thread
static struct replication_thread {
    ARAL *aral_rse;

    SPINLOCK spinlock;

    struct {
        size_t pending;                 // number of requests pending in the queue

        // statistics
        size_t added;                   // number of requests added to the queue
        size_t removed;                 // number of requests removed from the queue
        size_t pending_no_room;         // number of requests skipped, because the sender has no room for responses
        size_t senders_full;             // number of times a sender reset our last position in the queue
        size_t sender_resets;           // number of times a sender reset our last position in the queue
        time_t first_time_t;            // the minimum 'after' we encountered

        struct {
            Word_t after;
            Word_t unique_id;
            Pvoid_t JudyL_array;
        } queue;

    } unsafe;                           // protected from replication_recursive_lock()

    struct {
        Word_t unique_id;               // the last unique id we gave to a request (auto-increment, starting from 1)
        size_t executed;                // the number of replication requests executed
        size_t latest_first_time;       // the 'after' timestamp of the last request we executed
        size_t memory;                  // the total memory allocated by replication
    } atomic;                           // access should be with atomic operations

    struct {
        size_t last_executed;           // caching of the atomic.executed to report number of requests executed since last time

        netdata_thread_t **threads_ptrs;
        size_t threads;
    } main_thread;                      // access is allowed only by the main thread

} replication_globals = {
        .aral_rse = NULL,
        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
        .unsafe = {
                .pending = 0,

                .added = 0,
                .removed = 0,
                .pending_no_room = 0,
                .sender_resets = 0,
                .senders_full = 0,

                .first_time_t = 0,

                .queue = {
                        .after = 0,
                        .unique_id = 0,
                        .JudyL_array = NULL,
                },
        },
        .atomic = {
                .unique_id = 0,
                .executed = 0,
                .latest_first_time = 0,
                .memory = 0,
        },
        .main_thread = {
                .last_executed = 0,
                .threads = 0,
                .threads_ptrs = NULL,
        },
};

size_t replication_allocated_memory(void) {
    return __atomic_load_n(&replication_globals.atomic.memory, __ATOMIC_RELAXED);
}

#define replication_set_latest_first_time(t) __atomic_store_n(&replication_globals.atomic.latest_first_time, t, __ATOMIC_RELAXED)
#define replication_get_latest_first_time() __atomic_load_n(&replication_globals.atomic.latest_first_time, __ATOMIC_RELAXED)

static inline bool replication_recursive_lock_mode(char mode) {
    static __thread int recursions = 0;

    if(mode == 'L') { // (L)ock
        if(++recursions == 1)
            netdata_spinlock_lock(&replication_globals.spinlock);
    }
    else if(mode == 'U') { // (U)nlock
        if(--recursions == 0)
            netdata_spinlock_unlock(&replication_globals.spinlock);
    }
    else if(mode == 'C') { // (C)heck
        if(recursions > 0)
            return true;
        else
            return false;
    }
    else
        fatal("REPLICATION: unknown lock mode '%c'", mode);

#ifdef NETDATA_INTERNAL_CHECKS
    if(recursions < 0)
        fatal("REPLICATION: recursions is %d", recursions);
#endif

    return true;
}

#define replication_recursive_lock() replication_recursive_lock_mode('L')
#define replication_recursive_unlock() replication_recursive_lock_mode('U')
#define fatal_when_replication_is_not_locked_for_me() do { \
    if(!replication_recursive_lock_mode('C')) \
        fatal("REPLICATION: reached %s, but replication is not locked by this thread.", __FUNCTION__); \
} while(0)

void replication_set_next_point_in_time(time_t after, size_t unique_id) {
    replication_recursive_lock();
    replication_globals.unsafe.queue.after = after;
    replication_globals.unsafe.queue.unique_id = unique_id;
    replication_recursive_unlock();
}

// ----------------------------------------------------------------------------
// replication sort entry management

static struct replication_sort_entry *replication_sort_entry_create(struct replication_request *rq) {
    struct replication_sort_entry *rse = aral_mallocz(replication_globals.aral_rse);
    __atomic_add_fetch(&replication_globals.atomic.memory, sizeof(struct replication_sort_entry), __ATOMIC_RELAXED);

    rrdpush_sender_pending_replication_requests_plus_one(rq->sender);

    // copy the request
    rse->rq = rq;
    rse->unique_id = __atomic_add_fetch(&replication_globals.atomic.unique_id, 1, __ATOMIC_SEQ_CST);

    // save the unique id into the request, to be able to delete it later
    rq->unique_id = rse->unique_id;
    rq->indexed_in_judy = false;
    rq->not_indexed_buffer_full = false;
    rq->not_indexed_preprocessing = false;
    return rse;
}

static void replication_sort_entry_destroy(struct replication_sort_entry *rse) {
    aral_freez(replication_globals.aral_rse, rse);
    __atomic_sub_fetch(&replication_globals.atomic.memory, sizeof(struct replication_sort_entry), __ATOMIC_RELAXED);
}

static void replication_sort_entry_add(struct replication_request *rq) {
    if(rrdpush_sender_replication_buffer_full_get(rq->sender)) {
        rq->indexed_in_judy = false;
        rq->not_indexed_buffer_full = true;
        rq->not_indexed_preprocessing = false;
        replication_recursive_lock();
        replication_globals.unsafe.pending_no_room++;
        replication_recursive_unlock();
        return;
    }

    // cache this, because it will be changed
    bool decrement_no_room = rq->not_indexed_buffer_full;

    struct replication_sort_entry *rse = replication_sort_entry_create(rq);

    replication_recursive_lock();

    if(decrement_no_room)
        replication_globals.unsafe.pending_no_room--;

//    if(rq->after < (time_t)replication_globals.protected.queue.after &&
//        rq->sender->buffer_used_percentage <= MAX_SENDER_BUFFER_PERCENTAGE_ALLOWED &&
//       !replication_globals.protected.skipped_no_room_since_last_reset) {
//
//        // make it find this request first
//        replication_set_next_point_in_time(rq->after, rq->unique_id);
//    }

    replication_globals.unsafe.added++;
    replication_globals.unsafe.pending++;

    Pvoid_t *inner_judy_ptr;

    // find the outer judy entry, using after as key
    size_t mem_before_outer_judyl = JudyLMemUsed(replication_globals.unsafe.queue.JudyL_array);
    inner_judy_ptr = JudyLIns(&replication_globals.unsafe.queue.JudyL_array, (Word_t) rq->after, PJE0);
    size_t mem_after_outer_judyl = JudyLMemUsed(replication_globals.unsafe.queue.JudyL_array);
    if(unlikely(!inner_judy_ptr || inner_judy_ptr == PJERR))
        fatal("REPLICATION: corrupted outer judyL");

    // add it to the inner judy, using unique_id as key
    size_t mem_before_inner_judyl = JudyLMemUsed(*inner_judy_ptr);
    Pvoid_t *item = JudyLIns(inner_judy_ptr, rq->unique_id, PJE0);
    size_t mem_after_inner_judyl = JudyLMemUsed(*inner_judy_ptr);
    if(unlikely(!item || item == PJERR))
        fatal("REPLICATION: corrupted inner judyL");

    *item = rse;
    rq->indexed_in_judy = true;
    rq->not_indexed_buffer_full = false;
    rq->not_indexed_preprocessing = false;

    if(!replication_globals.unsafe.first_time_t || rq->after < replication_globals.unsafe.first_time_t)
        replication_globals.unsafe.first_time_t = rq->after;

    replication_recursive_unlock();

    __atomic_add_fetch(&replication_globals.atomic.memory, (mem_after_inner_judyl - mem_before_inner_judyl) + (mem_after_outer_judyl - mem_before_outer_judyl), __ATOMIC_RELAXED);
}

static bool replication_sort_entry_unlink_and_free_unsafe(struct replication_sort_entry *rse, Pvoid_t **inner_judy_ppptr, bool preprocessing) {
    fatal_when_replication_is_not_locked_for_me();

    bool inner_judy_deleted = false;

    replication_globals.unsafe.removed++;
    replication_globals.unsafe.pending--;

    rrdpush_sender_pending_replication_requests_minus_one(rse->rq->sender);

    rse->rq->indexed_in_judy = false;
    rse->rq->not_indexed_preprocessing = preprocessing;

    size_t memory_saved = 0;

    // delete it from the inner judy
    size_t mem_before_inner_judyl = JudyLMemUsed(**inner_judy_ppptr);
    JudyLDel(*inner_judy_ppptr, rse->rq->unique_id, PJE0);
    size_t mem_after_inner_judyl = JudyLMemUsed(**inner_judy_ppptr);
    memory_saved = mem_before_inner_judyl - mem_after_inner_judyl;

    // if no items left, delete it from the outer judy
    if(**inner_judy_ppptr == NULL) {
        size_t mem_before_outer_judyl = JudyLMemUsed(replication_globals.unsafe.queue.JudyL_array);
        JudyLDel(&replication_globals.unsafe.queue.JudyL_array, rse->rq->after, PJE0);
        size_t mem_after_outer_judyl = JudyLMemUsed(replication_globals.unsafe.queue.JudyL_array);
        memory_saved += mem_before_outer_judyl - mem_after_outer_judyl;
        inner_judy_deleted = true;
    }

    // free memory
    replication_sort_entry_destroy(rse);

    __atomic_sub_fetch(&replication_globals.atomic.memory, memory_saved, __ATOMIC_RELAXED);

    return inner_judy_deleted;
}

static void replication_sort_entry_del(struct replication_request *rq, bool buffer_full) {
    Pvoid_t *inner_judy_pptr;
    struct replication_sort_entry *rse_to_delete = NULL;

    replication_recursive_lock();
    if(rq->indexed_in_judy) {

        inner_judy_pptr = JudyLGet(replication_globals.unsafe.queue.JudyL_array, rq->after, PJE0);
        if (inner_judy_pptr) {
            Pvoid_t *our_item_pptr = JudyLGet(*inner_judy_pptr, rq->unique_id, PJE0);
            if (our_item_pptr) {
                rse_to_delete = *our_item_pptr;
                replication_sort_entry_unlink_and_free_unsafe(rse_to_delete, &inner_judy_pptr, false);

                if(buffer_full) {
                    replication_globals.unsafe.pending_no_room++;
                    rq->not_indexed_buffer_full = true;
                }
            }
        }

        if (!rse_to_delete)
            fatal("REPLAY: 'host:%s/chart:%s' Cannot find sort entry to delete for time %ld.",
                  rrdhost_hostname(rq->sender->host), string2str(rq->chart_id), rq->after);

    }

    replication_recursive_unlock();
}

static struct replication_request replication_request_get_first_available() {
    Pvoid_t *inner_judy_pptr;

    replication_recursive_lock();

    struct replication_request rq_to_return = (struct replication_request){ .found = false };

    if(unlikely(!replication_globals.unsafe.queue.after || !replication_globals.unsafe.queue.unique_id)) {
        replication_globals.unsafe.queue.after = 0;
        replication_globals.unsafe.queue.unique_id = 0;
    }

    Word_t started_after = replication_globals.unsafe.queue.after;

    size_t round = 0;
    while(!rq_to_return.found) {
        round++;

        if(round > 2)
            break;

        if(round == 2) {
            if(started_after == 0)
                break;

            replication_globals.unsafe.queue.after = 0;
            replication_globals.unsafe.queue.unique_id = 0;
        }

        bool find_same_after = true;
        while (!rq_to_return.found && (inner_judy_pptr = JudyLFirstThenNext(replication_globals.unsafe.queue.JudyL_array, &replication_globals.unsafe.queue.after, &find_same_after))) {
            Pvoid_t *our_item_pptr;

            if(unlikely(round == 2 && replication_globals.unsafe.queue.after > started_after))
                break;

            while (!rq_to_return.found && (our_item_pptr = JudyLNext(*inner_judy_pptr, &replication_globals.unsafe.queue.unique_id, PJE0))) {
                struct replication_sort_entry *rse = *our_item_pptr;
                struct replication_request *rq = rse->rq;

                // copy the request to return it
                rq_to_return = *rq;
                rq_to_return.chart_id = string_dup(rq_to_return.chart_id);

                // set the return result to found
                rq_to_return.found = true;

                if (replication_sort_entry_unlink_and_free_unsafe(rse, &inner_judy_pptr, true))
                    // we removed the item from the outer JudyL
                    break;
            }

            // prepare for the next iteration on the outer loop
            replication_globals.unsafe.queue.unique_id = 0;
        }
    }

    replication_recursive_unlock();
    return rq_to_return;
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

    if(!rq->indexed_in_judy && rq->not_indexed_buffer_full && !rq->not_indexed_preprocessing) {
        // we can replace this command
        internal_error(
                true,
                "STREAM %s [send to %s]: REPLAY: 'host:%s/chart:%s' replacing duplicate replication command received (existing from %llu to %llu [%s], new from %llu to %llu [%s])",
                rrdhost_hostname(s->host), s->connected_to, rrdhost_hostname(s->host), dictionary_acquired_item_name(item),
                (unsigned long long)rq->after, (unsigned long long)rq->before, rq->start_streaming ? "true" : "false",
                (unsigned long long)rq_new->after, (unsigned long long)rq_new->before, rq_new->start_streaming ? "true" : "false");

        rq->after = rq_new->after;
        rq->before = rq_new->before;
        rq->start_streaming = rq_new->start_streaming;
    }
    else if(!rq->indexed_in_judy && !rq->not_indexed_preprocessing) {
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
        replication_sort_entry_del(rq, false);

    else if(rq->not_indexed_buffer_full) {
        replication_recursive_lock();
        replication_globals.unsafe.pending_no_room--;
        replication_recursive_unlock();
    }

    string_freez(rq->chart_id);
}

static bool sender_is_still_connected_for_this_request(struct replication_request *rq) {
    return rq->sender_last_flush_ut == rrdpush_sender_get_flush_time(rq->sender);
};

static bool replication_execute_request(struct replication_request *rq, bool workers) {
    bool ret = false;

    if(!rq->st) {
        if(likely(workers))
            worker_is_busy(WORKER_JOB_FIND_CHART);

        rq->st = rrdset_find(rq->sender->host, string2str(rq->chart_id));
    }

    if(!rq->st) {
        internal_error(true, "REPLAY ERROR: 'host:%s/chart:%s' not found",
                       rrdhost_hostname(rq->sender->host), string2str(rq->chart_id));

        goto cleanup;
    }

    netdata_thread_disable_cancelability();

    if(!rq->q) {
        if(likely(workers))
            worker_is_busy(WORKER_JOB_PREPARE_QUERY);

        rq->q = replication_response_prepare(
                rq->st,
                rq->start_streaming,
                rq->after,
                rq->before,
                rq->sender->capabilities);
    }

    if(likely(workers))
        worker_is_busy(WORKER_JOB_QUERYING);

    // send the replication data
    rq->q->rq = rq;
    replication_response_execute_and_finalize(
            rq->q, (size_t)((unsigned long long)rq->sender->host->sender->buffer->max_size * MAX_REPLICATION_MESSAGE_PERCENT_SENDER_BUFFER / 100ULL));

    rq->q = NULL;
    netdata_thread_enable_cancelability();

    __atomic_add_fetch(&replication_globals.atomic.executed, 1, __ATOMIC_RELAXED);

    ret = true;

cleanup:
    if(rq->q) {
        replication_response_cancel_and_finalize(rq->q);
        rq->q = NULL;
    }

    string_freez(rq->chart_id);
    worker_is_idle();
    return ret;
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
            .indexed_in_judy = false,
            .not_indexed_buffer_full = false,
            .not_indexed_preprocessing = false,
    };

    if(!sender->replication.oldest_request_after_t || rq.after < sender->replication.oldest_request_after_t)
        sender->replication.oldest_request_after_t = rq.after;

    if(start_streaming && rrdpush_sender_get_buffer_used_percent(sender) <= STREAMING_START_MAX_SENDER_BUFFER_PERCENTAGE_ALLOWED)
        replication_execute_request(&rq, false);

    else
        dictionary_set(sender->replication.requests, chart_id, &rq, sizeof(struct replication_request));
}

void replication_sender_delete_pending_requests(struct sender_state *sender) {
    // allow the dictionary destructor to go faster on locks
    dictionary_flush(sender->replication.requests);
}

void replication_init_sender(struct sender_state *sender) {
    sender->replication.requests = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                              NULL, sizeof(struct replication_request));

    dictionary_register_react_callback(sender->replication.requests, replication_request_react_callback, sender);
    dictionary_register_conflict_callback(sender->replication.requests, replication_request_conflict_callback, sender);
    dictionary_register_delete_callback(sender->replication.requests, replication_request_delete_callback, sender);
}

void replication_cleanup_sender(struct sender_state *sender) {
    // allow the dictionary destructor to go faster on locks
    replication_recursive_lock();
    dictionary_destroy(sender->replication.requests);
    replication_recursive_unlock();
}

void replication_recalculate_buffer_used_ratio_unsafe(struct sender_state *s) {
    size_t available = cbuffer_available_size_unsafe(s->host->sender->buffer);
    size_t percentage = (s->buffer->max_size - available) * 100 / s->buffer->max_size;

    if(unlikely(percentage > MAX_SENDER_BUFFER_PERCENTAGE_ALLOWED && !rrdpush_sender_replication_buffer_full_get(s))) {
        rrdpush_sender_replication_buffer_full_set(s, true);

        struct replication_request *rq;
        dfe_start_read(s->replication.requests, rq) {
            if(rq->indexed_in_judy)
                replication_sort_entry_del(rq, true);
        }
        dfe_done(rq);

        replication_recursive_lock();
        replication_globals.unsafe.senders_full++;
        replication_recursive_unlock();
    }
    else if(unlikely(percentage < MIN_SENDER_BUFFER_PERCENTAGE_ALLOWED && rrdpush_sender_replication_buffer_full_get(s))) {
        rrdpush_sender_replication_buffer_full_set(s, false);

        struct replication_request *rq;
        dfe_start_read(s->replication.requests, rq) {
            if(!rq->indexed_in_judy && (rq->not_indexed_buffer_full || rq->not_indexed_preprocessing))
                replication_sort_entry_add(rq);
        }
        dfe_done(rq);

        replication_recursive_lock();
        replication_globals.unsafe.senders_full--;
        replication_globals.unsafe.sender_resets++;
        // replication_set_next_point_in_time(0, 0);
        replication_recursive_unlock();
    }

    rrdpush_sender_set_buffer_used_percent(s, percentage);
}

// ----------------------------------------------------------------------------
// replication thread

static size_t verify_host_charts_are_streaming_now(RRDHOST *host) {
    internal_error(
            host->sender &&
            !rrdpush_sender_pending_replication_requests(host->sender) &&
            dictionary_entries(host->sender->replication.requests) != 0,
            "REPLICATION SUMMARY: 'host:%s' reports %zu pending replication requests, but its chart replication index says there are %zu charts pending replication",
            rrdhost_hostname(host),
            rrdpush_sender_pending_replication_requests(host->sender),
            dictionary_entries(host->sender->replication.requests)
            );

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
    worker_is_busy(WORKER_JOB_CHECK_CONSISTENCY);

    size_t errors = 0;
    RRDHOST *host;
    dfe_start_read(rrdhost_root_index, host)
        errors += verify_host_charts_are_streaming_now(host);
    dfe_done(host);

    size_t executed = __atomic_load_n(&replication_globals.atomic.executed, __ATOMIC_RELAXED);
    info("REPLICATION SUMMARY: finished, executed %zu replication requests, %zu charts pending replication",
         executed - replication_globals.main_thread.last_executed, errors);
    replication_globals.main_thread.last_executed = executed;
}

static void replication_initialize_workers(bool master) {
    worker_register("REPLICATION");
    worker_register_job_name(WORKER_JOB_FIND_NEXT, "find next");
    worker_register_job_name(WORKER_JOB_QUERYING, "querying");
    worker_register_job_name(WORKER_JOB_DELETE_ENTRY, "dict delete");
    worker_register_job_name(WORKER_JOB_FIND_CHART, "find chart");
    worker_register_job_name(WORKER_JOB_PREPARE_QUERY, "prepare query");
    worker_register_job_name(WORKER_JOB_CHECK_CONSISTENCY, "check consistency");
    worker_register_job_name(WORKER_JOB_BUFFER_COMMIT, "commit");
    worker_register_job_name(WORKER_JOB_CLEANUP, "cleanup");
    worker_register_job_name(WORKER_JOB_WAIT, "wait");

    if(master) {
        worker_register_job_name(WORKER_JOB_STATISTICS, "statistics");
        worker_register_job_custom_metric(WORKER_JOB_CUSTOM_METRIC_PENDING_REQUESTS, "pending requests", "requests", WORKER_METRIC_ABSOLUTE);
        worker_register_job_custom_metric(WORKER_JOB_CUSTOM_METRIC_SKIPPED_NO_ROOM, "no room requests", "requests", WORKER_METRIC_ABSOLUTE);
        worker_register_job_custom_metric(WORKER_JOB_CUSTOM_METRIC_COMPLETION, "completion", "%", WORKER_METRIC_ABSOLUTE);
        worker_register_job_custom_metric(WORKER_JOB_CUSTOM_METRIC_ADDED, "added requests", "requests/s", WORKER_METRIC_INCREMENTAL_TOTAL);
        worker_register_job_custom_metric(WORKER_JOB_CUSTOM_METRIC_DONE, "finished requests", "requests/s", WORKER_METRIC_INCREMENTAL_TOTAL);
        worker_register_job_custom_metric(WORKER_JOB_CUSTOM_METRIC_SENDER_RESETS, "sender resets", "resets/s", WORKER_METRIC_INCREMENTAL_TOTAL);
        worker_register_job_custom_metric(WORKER_JOB_CUSTOM_METRIC_SENDER_FULL, "senders full", "senders", WORKER_METRIC_ABSOLUTE);
    }
}

#define REQUEST_OK (0)
#define REQUEST_QUEUE_EMPTY (-1)
#define REQUEST_CHART_NOT_FOUND (-2)

static __thread struct replication_thread_pipeline {
    int max_requests_ahead;
    struct replication_request *rqs;
    int rqs_last_executed, rqs_last_prepared;
    size_t queue_rounds;
} rtp = {
        .max_requests_ahead = 0,
        .rqs = NULL,
        .rqs_last_executed = 0,
        .rqs_last_prepared = 0,
        .queue_rounds = 0,
};

static void replication_pipeline_cancel_and_cleanup(void) {
    if(!rtp.rqs)
        return;

    struct replication_request *rq;
    size_t cancelled = 0;

    do {
        if (++rtp.rqs_last_executed >= rtp.max_requests_ahead)
            rtp.rqs_last_executed = 0;

        rq = &rtp.rqs[rtp.rqs_last_executed];

        if (rq->q) {
            internal_fatal(rq->executed, "REPLAY FATAL: query has already been executed!");
            internal_fatal(!rq->found, "REPLAY FATAL: orphan q in rq");

            replication_response_cancel_and_finalize(rq->q);
            rq->q = NULL;
            cancelled++;
        }

        rq->executed = true;
        rq->found = false;

    } while (rtp.rqs_last_executed != rtp.rqs_last_prepared);

    internal_error(true, "REPLICATION: cancelled %zu inflight queries", cancelled);

    freez(rtp.rqs);
    rtp.rqs = NULL;
    rtp.max_requests_ahead = 0;
    rtp.rqs_last_executed = 0;
    rtp.rqs_last_prepared = 0;
    rtp.queue_rounds = 0;
}

static int replication_pipeline_execute_next(void) {
    struct replication_request *rq;

    if(unlikely(!rtp.rqs)) {
        rtp.max_requests_ahead = (int)get_netdata_cpus() / 2;

        if(rtp.max_requests_ahead > libuv_worker_threads * 2)
            rtp.max_requests_ahead = libuv_worker_threads * 2;

        if(rtp.max_requests_ahead < 2)
            rtp.max_requests_ahead = 2;

        rtp.rqs = callocz(rtp.max_requests_ahead, sizeof(struct replication_request));
        __atomic_add_fetch(&replication_buffers_allocated, rtp.max_requests_ahead * sizeof(struct replication_request), __ATOMIC_RELAXED);
    }

    // fill the queue
    do {
        if(++rtp.rqs_last_prepared >= rtp.max_requests_ahead) {
            rtp.rqs_last_prepared = 0;
            rtp.queue_rounds++;
        }

        internal_fatal(rtp.rqs[rtp.rqs_last_prepared].q,
                       "REPLAY FATAL: slot is used by query that has not been executed!");

        worker_is_busy(WORKER_JOB_FIND_NEXT);
        rtp.rqs[rtp.rqs_last_prepared] = replication_request_get_first_available();
        rq = &rtp.rqs[rtp.rqs_last_prepared];

        if(rq->found) {
            if (!rq->st) {
                worker_is_busy(WORKER_JOB_FIND_CHART);
                rq->st = rrdset_find(rq->sender->host, string2str(rq->chart_id));
            }

            if (rq->st && !rq->q) {
                worker_is_busy(WORKER_JOB_PREPARE_QUERY);
                rq->q = replication_response_prepare(
                        rq->st,
                        rq->start_streaming,
                        rq->after,
                        rq->before,
                        rq->sender->capabilities);
            }

            rq->executed = false;
        }

    } while(rq->found && rtp.rqs_last_prepared != rtp.rqs_last_executed);

    // pick the first usable
    do {
        if (++rtp.rqs_last_executed >= rtp.max_requests_ahead)
            rtp.rqs_last_executed = 0;

        rq = &rtp.rqs[rtp.rqs_last_executed];

        if(rq->found) {
            internal_fatal(rq->executed, "REPLAY FATAL: query has already been executed!");

            if (rq->sender_last_flush_ut != rrdpush_sender_get_flush_time(rq->sender)) {
                // the sender has reconnected since this request was queued,
                // we can safely throw it away, since the parent will resend it
                replication_response_cancel_and_finalize(rq->q);
                rq->executed = true;
                rq->found = false;
                rq->q = NULL;
            }
            else if (rrdpush_sender_replication_buffer_full_get(rq->sender)) {
                // the sender buffer is full, so we can ignore this request,
                // it has already been marked as 'preprocessed' in the dictionary,
                // and the sender will put it back in when there is
                // enough room in the buffer for processing replication requests
                replication_response_cancel_and_finalize(rq->q);
                rq->executed = true;
                rq->found = false;
                rq->q = NULL;
            }
            else {
                // we can execute this,
                // delete it from the dictionary
                worker_is_busy(WORKER_JOB_DELETE_ENTRY);
                dictionary_del(rq->sender->replication.requests, string2str(rq->chart_id));
            }
        }
        else
            internal_fatal(rq->q, "REPLAY FATAL: slot status says slot is empty, but it has a pending query!");

    } while(!rq->found && rtp.rqs_last_executed != rtp.rqs_last_prepared);

    if(unlikely(!rq->found)) {
        worker_is_idle();
        return REQUEST_QUEUE_EMPTY;
    }

    replication_set_latest_first_time(rq->after);

    bool chart_found = replication_execute_request(rq, true);
    rq->executed = true;
    rq->found = false;
    rq->q = NULL;

    if(unlikely(!chart_found)) {
        worker_is_idle();
        return REQUEST_CHART_NOT_FOUND;
    }

    worker_is_idle();
    return REQUEST_OK;
}

static void replication_worker_cleanup(void *ptr __maybe_unused) {
    replication_pipeline_cancel_and_cleanup();
    worker_unregister();
}

static void *replication_worker_thread(void *ptr) {
    replication_initialize_workers(false);

    netdata_thread_cleanup_push(replication_worker_cleanup, ptr);

    while(service_running(SERVICE_REPLICATION)) {
        if(unlikely(replication_pipeline_execute_next() == REQUEST_QUEUE_EMPTY)) {
            sender_thread_buffer_free();
            worker_is_busy(WORKER_JOB_WAIT);
            worker_is_idle();
            sleep_usec(1 * USEC_PER_SEC);
        }
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}

static void replication_main_cleanup(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    replication_pipeline_cancel_and_cleanup();

    int threads = (int)replication_globals.main_thread.threads;
    for(int i = 0; i < threads ;i++) {
        netdata_thread_join(*replication_globals.main_thread.threads_ptrs[i], NULL);
        freez(replication_globals.main_thread.threads_ptrs[i]);
        __atomic_sub_fetch(&replication_buffers_allocated, sizeof(netdata_thread_t), __ATOMIC_RELAXED);
    }
    freez(replication_globals.main_thread.threads_ptrs);
    replication_globals.main_thread.threads_ptrs = NULL;
    __atomic_sub_fetch(&replication_buffers_allocated, threads * sizeof(netdata_thread_t *), __ATOMIC_RELAXED);

    aral_destroy(replication_globals.aral_rse);
    replication_globals.aral_rse = NULL;

    // custom code
    worker_unregister();

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void replication_initialize(void) {
    replication_globals.aral_rse = aral_create("rse", sizeof(struct replication_sort_entry),
                                               0, 65536, aral_by_size_statistics(),
                                               NULL, NULL, false, false);
}

void *replication_thread_main(void *ptr __maybe_unused) {
    replication_initialize_workers(true);

    int threads = config_get_number(CONFIG_SECTION_DB, "replication threads", 1);
    if(threads < 1 || threads > MAX_REPLICATION_THREADS) {
        error("replication threads given %d is invalid, resetting to 1", threads);
        threads = 1;
    }

    if(--threads) {
        replication_globals.main_thread.threads = threads;
        replication_globals.main_thread.threads_ptrs = mallocz(threads * sizeof(netdata_thread_t *));
        __atomic_add_fetch(&replication_buffers_allocated, threads * sizeof(netdata_thread_t *), __ATOMIC_RELAXED);

        for(int i = 0; i < threads ;i++) {
            char tag[NETDATA_THREAD_TAG_MAX + 1];
            snprintfz(tag, NETDATA_THREAD_TAG_MAX, "REPLAY[%d]", i + 2);
            replication_globals.main_thread.threads_ptrs[i] = mallocz(sizeof(netdata_thread_t));
            __atomic_add_fetch(&replication_buffers_allocated, sizeof(netdata_thread_t), __ATOMIC_RELAXED);
            netdata_thread_create(replication_globals.main_thread.threads_ptrs[i], tag,
                                  NETDATA_THREAD_OPTION_JOINABLE, replication_worker_thread, NULL);
        }
    }

    netdata_thread_cleanup_push(replication_main_cleanup, ptr);

    // start from 100% completed
    worker_set_metric(WORKER_JOB_CUSTOM_METRIC_COMPLETION, 100.0);

    long run_verification_countdown = LONG_MAX; // LONG_MAX to prevent an initial verification when no replication ever took place
    bool slow = true; // control the time we sleep - it has to start with true!
    usec_t last_now_mono_ut = now_monotonic_usec();
    time_t replication_reset_next_point_in_time_countdown = SECONDS_TO_RESET_POINT_IN_TIME; // restart from the beginning every 10 seconds

    size_t last_executed = 0;
    size_t last_sender_resets = 0;

    while(service_running(SERVICE_REPLICATION)) {

        // statistics
        usec_t now_mono_ut = now_monotonic_usec();
        if(unlikely(now_mono_ut - last_now_mono_ut > default_rrd_update_every * USEC_PER_SEC)) {
            last_now_mono_ut = now_mono_ut;

            worker_is_busy(WORKER_JOB_STATISTICS);
            replication_recursive_lock();

            size_t current_executed = __atomic_load_n(&replication_globals.atomic.executed, __ATOMIC_RELAXED);
            if(last_executed != current_executed) {
                run_verification_countdown = ITERATIONS_IDLE_WITHOUT_PENDING_TO_RUN_SENDER_VERIFICATION;
                last_executed = current_executed;
                slow = false;
            }

            if(replication_reset_next_point_in_time_countdown-- == 0) {
                // once per second, make it scan all the pending requests next time
                replication_set_next_point_in_time(0, 0);
//                replication_globals.protected.skipped_no_room_since_last_reset = 0;
                replication_reset_next_point_in_time_countdown = SECONDS_TO_RESET_POINT_IN_TIME;
            }

            if(--run_verification_countdown == 0) {
                if (!replication_globals.unsafe.pending && !replication_globals.unsafe.pending_no_room) {
                    // reset the statistics about completion percentage
                    replication_globals.unsafe.first_time_t = 0;
                    replication_set_latest_first_time(0);

                    verify_all_hosts_charts_are_streaming_now();

                    run_verification_countdown = LONG_MAX;
                    slow = true;
                }
                else
                    run_verification_countdown = ITERATIONS_IDLE_WITHOUT_PENDING_TO_RUN_SENDER_VERIFICATION;
            }

            time_t latest_first_time_t = replication_get_latest_first_time();
            if(latest_first_time_t && replication_globals.unsafe.pending) {
                // completion percentage statistics
                time_t now = now_realtime_sec();
                time_t total = now - replication_globals.unsafe.first_time_t;
                time_t done = latest_first_time_t - replication_globals.unsafe.first_time_t;
                worker_set_metric(WORKER_JOB_CUSTOM_METRIC_COMPLETION,
                                  (NETDATA_DOUBLE) done * 100.0 / (NETDATA_DOUBLE) total);
            }
            else
                worker_set_metric(WORKER_JOB_CUSTOM_METRIC_COMPLETION, 100.0);

            worker_set_metric(WORKER_JOB_CUSTOM_METRIC_PENDING_REQUESTS, (NETDATA_DOUBLE)replication_globals.unsafe.pending);
            worker_set_metric(WORKER_JOB_CUSTOM_METRIC_ADDED, (NETDATA_DOUBLE)replication_globals.unsafe.added);
            worker_set_metric(WORKER_JOB_CUSTOM_METRIC_DONE, (NETDATA_DOUBLE)__atomic_load_n(&replication_globals.atomic.executed, __ATOMIC_RELAXED));
            worker_set_metric(WORKER_JOB_CUSTOM_METRIC_SKIPPED_NO_ROOM, (NETDATA_DOUBLE)replication_globals.unsafe.pending_no_room);
            worker_set_metric(WORKER_JOB_CUSTOM_METRIC_SENDER_RESETS, (NETDATA_DOUBLE)replication_globals.unsafe.sender_resets);
            worker_set_metric(WORKER_JOB_CUSTOM_METRIC_SENDER_FULL, (NETDATA_DOUBLE)replication_globals.unsafe.senders_full);

            replication_recursive_unlock();
            worker_is_idle();
        }

        if(unlikely(replication_pipeline_execute_next() == REQUEST_QUEUE_EMPTY)) {

            worker_is_busy(WORKER_JOB_WAIT);
            replication_recursive_lock();

            // the timeout also defines now frequently we will traverse all the pending requests
            // when the outbound buffers of all senders is full
            usec_t timeout;
            if(slow) {
                // no work to be done, wait for a request to come in
                timeout = 1000 * USEC_PER_MS;
                sender_thread_buffer_free();
            }

            else if(replication_globals.unsafe.pending > 0) {
                if(replication_globals.unsafe.sender_resets == last_sender_resets)
                    timeout = 1000 * USEC_PER_MS;

                else {
                    // there are pending requests waiting to be executed,
                    // but none could be executed at this time.
                    // try again after this time.
                    timeout = 100 * USEC_PER_MS;
                }

                last_sender_resets = replication_globals.unsafe.sender_resets;
            }
            else {
                // no requests pending, but there were requests recently (run_verification_countdown)
                // so, try in a short time.
                // if this is big, one chart replicating will be slow to finish (ping - pong just one chart)
                timeout = 10 * USEC_PER_MS;
                last_sender_resets = replication_globals.unsafe.sender_resets;
            }

            replication_recursive_unlock();

            worker_is_idle();
            sleep_usec(timeout);

            // make it scan all the pending requests next time
            replication_set_next_point_in_time(0, 0);
            replication_reset_next_point_in_time_countdown = SECONDS_TO_RESET_POINT_IN_TIME;

            continue;
        }
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}

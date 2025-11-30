// SPDX-License-Identifier: GPL-3.0-or-later

#include "pluginsd_replication.h"
#include "streaming/stream-receiver-internals.h"
#include "streaming/stream-replication-receiver.h"
#include "streaming/stream-waiting-list.h"
#include "web/api/queries/backfill.h"
#include "database/rrddim-collection.h"

static bool backfill_callback(size_t successful_dims __maybe_unused, size_t failed_dims __maybe_unused, struct backfill_request_data *brd) {
    if(!object_state_acquire(&brd->host->state_id, brd->host_state_id)) {
        // this may happen because the host got reconnected

        nd_log(NDLS_DAEMON, NDLP_DEBUG,
                "PLUGINSD REPLAY ERROR: 'host:%s' failed to acquire host for sending replication"
               " command for 'chart:%s'",
                rrdhost_hostname(brd->host),
                rrdset_id(brd->st));

        return false;
    }

    __atomic_sub_fetch(&brd->host->stream.rcv.status.replication.backfill_pending, 1, __ATOMIC_RELAXED);

    bool rc = replicate_chart_request(send_to_plugin, brd->parser, brd->host, brd->st,
                                      brd->first_entry_child, brd->last_entry_child, brd->child_wall_clock_time,
                                      0, 0);

    if (!rc) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
            "PLUGINSD REPLAY ERROR: 'host:%s' failed to initiate replication for 'chart:%s' - replication may not proceed for this instance.",
            rrdhost_hostname(brd->host),
            rrdset_id(brd->st));
    }

    object_state_release(&brd->host->state_id);
    return rc;
}

PARSER_RC pluginsd_chart_definition_end(char **words, size_t num_words, PARSER *parser) {
    const char *first_entry_txt = get_word(words, num_words, 1);
    const char *last_entry_txt = get_word(words, num_words, 2);
    const char *wall_clock_time_txt = get_word(words, num_words, 3);

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_CHART_DEFINITION_END);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    RRDSET *st = pluginsd_require_scope_chart(parser, PLUGINSD_KEYWORD_CHART_DEFINITION_END, PLUGINSD_KEYWORD_CHART);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    time_t first_entry_child = (first_entry_txt && *first_entry_txt) ? (time_t)str2ul(first_entry_txt) : 0;
    time_t last_entry_child = (last_entry_txt && *last_entry_txt) ? (time_t)str2ul(last_entry_txt) : 0;
    time_t child_wall_clock_time = (wall_clock_time_txt && *wall_clock_time_txt) ? (time_t)str2ul(wall_clock_time_txt) : now_realtime_sec();

    bool ok = true;
    RRDSET_FLAGS old = rrdset_flag_set_and_clear(
        st, RRDSET_FLAG_RECEIVER_REPLICATION_IN_PROGRESS, RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED);

    if(!(old & RRDSET_FLAG_RECEIVER_REPLICATION_IN_PROGRESS)) {
        if(rrdhost_receiver_replicating_charts_plus_one(st->rrdhost) == 1)
            pulse_host_status(host, PULSE_HOST_STATUS_RCV_REPLICATING, 0);

        __atomic_add_fetch(&host->stream.rcv.status.replication.counter_in, 1, __ATOMIC_RELAXED);

#ifdef REPLICATION_TRACKING
        st->stream.rcv.who = REPLAY_WHO_ME;
#endif

#ifdef NETDATA_LOG_REPLICATION_REQUESTS
        st->replay.start_streaming = false;
        st->replay.after = 0;
        st->replay.before = 0;
#endif

        struct backfill_request_data brd = {
            .host_state_id = object_state_id(&host->state_id),
            .parser = parser,
            .host = host,
            .st = st,
            .first_entry_child = first_entry_child,
            .last_entry_child = last_entry_child,
            .child_wall_clock_time = child_wall_clock_time,
        };

        __atomic_add_fetch(&host->stream.rcv.status.replication.backfill_pending, 1, __ATOMIC_RELAXED);

        if(!rrdset_flag_check(st, RRDSET_FLAG_BACKFILLED_HIGH_TIERS)) {
            ok = backfill_request_add(st, backfill_callback, &brd);
            if (!ok)
                ok = backfill_callback(0, 0, &brd);
            else
                rrdset_flag_set(st, RRDSET_FLAG_BACKFILLED_HIGH_TIERS);
        }
        else
            ok = backfill_callback(0, 0, &brd);
    }
    else {
        // this is normal, since dimensions may be added to a chart,
        // and the child will send another CHART_DEFINITION_END command.

#ifdef NETDATA_LOG_REPLICATION_REQUESTS
        internal_error(true, "REPLAY: 'host:%s/chart:%s' not sending duplicate replication request",
                       rrdhost_hostname(st->rrdhost), rrdset_id(st));
#endif
    }

    stream_thread_received_metadata();

    return ok ? PARSER_RC_OK : PARSER_RC_ERROR;
}

ALWAYS_INLINE PARSER_RC pluginsd_replay_begin(char **words, size_t num_words, PARSER *parser) {
    int idx = 1;
    ssize_t slot = pluginsd_parse_rrd_slot(words, num_words);
    if(slot >= 0) idx++;

    char *id = get_word(words, num_words, idx++);
    char *start_time_str = get_word(words, num_words, idx++);
    char *end_time_str = get_word(words, num_words, idx++);
    char *child_now_str = get_word(words, num_words, idx++);

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_REPLAY_BEGIN);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    RRDSET *st;
    if (likely(!id || !*id))
        st = pluginsd_require_scope_chart(parser, PLUGINSD_KEYWORD_REPLAY_BEGIN, PLUGINSD_KEYWORD_REPLAY_BEGIN);
    else
        st = pluginsd_rrdset_cache_get_from_slot(parser, host, id, slot, PLUGINSD_KEYWORD_REPLAY_BEGIN);

    if(!st) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    if(!pluginsd_set_scope_chart(parser, st, PLUGINSD_KEYWORD_REPLAY_BEGIN))
        return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    if(start_time_str && end_time_str) {
        time_t start_time = (time_t) str2ull_encoded(start_time_str);
        time_t end_time = (time_t) str2ull_encoded(end_time_str);

        time_t wall_clock_time = 0, tolerance;
        bool wall_clock_comes_from_child; (void)wall_clock_comes_from_child;
        if(child_now_str) {
            wall_clock_time = (time_t) str2ull_encoded(child_now_str);
            tolerance = st->update_every + 1;
            wall_clock_comes_from_child = true;
        }

        if(wall_clock_time <= 0) {
            wall_clock_time = now_realtime_sec();
            tolerance = st->update_every + 5;
            wall_clock_comes_from_child = false;
        }

#ifdef NETDATA_LOG_REPLICATION_REQUESTS
        internal_error(
                (!st->replay.start_streaming && (end_time < st->replay.after || start_time > st->replay.before)),
                "REPLAY ERROR: 'host:%s/chart:%s' got a " PLUGINSD_KEYWORD_REPLAY_BEGIN " from %ld to %ld, which does not match our request (%ld to %ld).",
                rrdhost_hostname(st->rrdhost), rrdset_id(st), start_time, end_time, st->replay.after, st->replay.before);

        internal_error(
                true,
                "REPLAY: 'host:%s/chart:%s' got a " PLUGINSD_KEYWORD_REPLAY_BEGIN " from %ld to %ld, child wall clock is %ld (%s), had requested %ld to %ld",
                rrdhost_hostname(st->rrdhost), rrdset_id(st),
                start_time, end_time, wall_clock_time, wall_clock_comes_from_child ? "from child" : "parent time",
                st->replay.after, st->replay.before);
#endif

        if(start_time && end_time && start_time < wall_clock_time + tolerance && end_time < wall_clock_time + tolerance && start_time < end_time) {
            if (unlikely(end_time - start_time != st->update_every))
                rrdset_set_update_every_s(st, end_time - start_time);

            st->last_collected_time.tv_sec = end_time;
            st->last_collected_time.tv_usec = 0;

            st->last_updated.tv_sec = end_time;
            st->last_updated.tv_usec = 0;

            st->counter++;
            st->counter_done++;

            // these are only needed for db mode RAM, ALLOC
            st->db.current_entry++;
            if(st->db.current_entry >= st->db.entries)
                st->db.current_entry -= st->db.entries;

            parser->user.replay.start_time = start_time;
            parser->user.replay.end_time = end_time;
            parser->user.replay.start_time_ut = (usec_t) start_time * USEC_PER_SEC;
            parser->user.replay.end_time_ut = (usec_t) end_time * USEC_PER_SEC;
            parser->user.replay.wall_clock_time = wall_clock_time;
            parser->user.replay.rset_enabled = true;

            return PARSER_RC_OK;
        }

        nd_log(NDLS_DAEMON, NDLP_ERR,
               "PLUGINSD REPLAY ERROR: 'host:%s/chart:%s' got a " PLUGINSD_KEYWORD_REPLAY_BEGIN
               " from %ld to %ld, but timestamps are invalid "
               "(now is %ld [%s], tolerance %ld). Ignoring " PLUGINSD_KEYWORD_REPLAY_SET,
               rrdhost_hostname(st->rrdhost), rrdset_id(st), start_time, end_time,
               wall_clock_time, wall_clock_comes_from_child ? "child wall clock" : "parent wall clock",
               tolerance);
    }

    // the child sends an RBEGIN without any parameters initially
    // setting rset_enabled to false, means the RSET should not store any metrics
    // to store metrics, the RBEGIN needs to have timestamps
    parser->user.replay.start_time = 0;
    parser->user.replay.end_time = 0;
    parser->user.replay.start_time_ut = 0;
    parser->user.replay.end_time_ut = 0;
    parser->user.replay.wall_clock_time = 0;
    parser->user.replay.rset_enabled = false;
    return PARSER_RC_OK;
}

ALWAYS_INLINE PARSER_RC pluginsd_replay_set(char **words, size_t num_words, PARSER *parser) {
    int idx = 1;
    ssize_t slot = pluginsd_parse_rrd_slot(words, num_words);
    if(slot >= 0) idx++;

    char *dimension = get_word(words, num_words, idx++);
    char *value_str = get_word(words, num_words, idx++);
    char *flags_str = get_word(words, num_words, idx++);

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_REPLAY_SET);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    RRDSET *st = pluginsd_require_scope_chart(parser, PLUGINSD_KEYWORD_REPLAY_SET, PLUGINSD_KEYWORD_REPLAY_BEGIN);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    if(!parser->user.replay.rset_enabled) {
        nd_log_limit_static_thread_var(erl, 1, 0);
        nd_log_limit(&erl, NDLS_COLLECTORS, NDLP_ERR,
                     "PLUGINSD REPLAY ERROR: 'host:%s/chart:%s' got a %s but it is disabled by %s errors",
                     rrdhost_hostname(host), rrdset_id(st), PLUGINSD_KEYWORD_REPLAY_SET, PLUGINSD_KEYWORD_REPLAY_BEGIN);

        // we have to return OK here
        return PARSER_RC_OK;
    }

    RRDDIM *rd = pluginsd_acquire_dimension(host, st, dimension, slot, PLUGINSD_KEYWORD_REPLAY_SET);
    if(!rd) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    st->pluginsd.set = true;

    if (unlikely(!parser->user.replay.start_time || !parser->user.replay.end_time)) {
        
        nd_log(NDLS_DAEMON, NDLP_ERR, 
            "PLUGINSD REPLAY ERROR: 'host:%s/chart:%s/dim:%s' got a %s with "
            "invalid timestamps %ld to %ld from a %s. Disabling it.",
            rrdhost_hostname(host), rrdset_id(st), dimension, PLUGINSD_KEYWORD_REPLAY_SET,
            parser->user.replay.start_time, parser->user.replay.end_time, PLUGINSD_KEYWORD_REPLAY_BEGIN);
        
        return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);
    }

    if (unlikely(!value_str || !*value_str))
        value_str = "NAN";

    if(unlikely(!flags_str))
        flags_str = "";

    if (likely(value_str)) {
        NETDATA_DOUBLE value = str2ndd_encoded(value_str, NULL);
        SN_FLAGS flags = pluginsd_parse_storage_number_flags(flags_str);

        if (!netdata_double_isnumber(value) || (flags == SN_EMPTY_SLOT)) {
            value = NAN;
            flags = SN_EMPTY_SLOT;
        }

        rrddim_store_metric(rd, parser->user.replay.end_time_ut, value, flags);
        rd->collector.last_collected_time.tv_sec = parser->user.replay.end_time;
        rd->collector.last_collected_time.tv_usec = 0;
        rd->collector.counter++;
    }

    return PARSER_RC_OK;
}

ALWAYS_INLINE PARSER_RC pluginsd_replay_rrddim_collection_state(char **words, size_t num_words, PARSER *parser) {
    if(parser->user.replay.rset_enabled == false)
        return PARSER_RC_OK;

    int idx = 1;
    ssize_t slot = pluginsd_parse_rrd_slot(words, num_words);
    if(slot >= 0) idx++;

    char *dimension = get_word(words, num_words, idx++);
    char *last_collected_ut_str = get_word(words, num_words, idx++);
    char *last_collected_value_str = get_word(words, num_words, idx++);
    char *last_calculated_value_str = get_word(words, num_words, idx++);
    char *last_stored_value_str = get_word(words, num_words, idx++);

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_REPLAY_RRDDIM_STATE);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    RRDSET *st = pluginsd_require_scope_chart(parser, PLUGINSD_KEYWORD_REPLAY_RRDDIM_STATE, PLUGINSD_KEYWORD_REPLAY_BEGIN);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    if(st->pluginsd.set) {
        // reset pos to reuse the same RDAs
        st->pluginsd.pos = 0;
        st->pluginsd.set = false;
    }

    RRDDIM *rd = pluginsd_acquire_dimension(host, st, dimension, slot, PLUGINSD_KEYWORD_REPLAY_RRDDIM_STATE);
    if(!rd) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    usec_t dim_last_collected_ut = (usec_t)rd->collector.last_collected_time.tv_sec * USEC_PER_SEC + (usec_t)rd->collector.last_collected_time.tv_usec;
    usec_t last_collected_ut = last_collected_ut_str ? str2ull_encoded(last_collected_ut_str) : 0;
    if(last_collected_ut > dim_last_collected_ut) {
        rd->collector.last_collected_time.tv_sec = (time_t)(last_collected_ut / USEC_PER_SEC);
        rd->collector.last_collected_time.tv_usec = (suseconds_t)(last_collected_ut % USEC_PER_SEC);
    }

    rd->collector.last_collected_value = last_collected_value_str ? str2ll_encoded(last_collected_value_str) : 0;
    rd->collector.last_calculated_value = last_calculated_value_str ? str2ndd_encoded(last_calculated_value_str, NULL) : 0;
    rd->collector.last_stored_value = last_stored_value_str ? str2ndd_encoded(last_stored_value_str, NULL) : 0.0;

    return PARSER_RC_OK;
}

ALWAYS_INLINE PARSER_RC pluginsd_replay_rrdset_collection_state(char **words, size_t num_words, PARSER *parser) {
    if(parser->user.replay.rset_enabled == false)
        return PARSER_RC_OK;

    char *last_collected_ut_str = get_word(words, num_words, 1);
    char *last_updated_ut_str = get_word(words, num_words, 2);

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_REPLAY_RRDSET_STATE);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    RRDSET *st = pluginsd_require_scope_chart(parser, PLUGINSD_KEYWORD_REPLAY_RRDSET_STATE,
                                              PLUGINSD_KEYWORD_REPLAY_BEGIN);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    usec_t chart_last_collected_ut = (usec_t)st->last_collected_time.tv_sec * USEC_PER_SEC + (usec_t)st->last_collected_time.tv_usec;
    usec_t last_collected_ut = last_collected_ut_str ? str2ull_encoded(last_collected_ut_str) : 0;
    if(last_collected_ut > chart_last_collected_ut) {
        st->last_collected_time.tv_sec = (time_t)(last_collected_ut / USEC_PER_SEC);
        st->last_collected_time.tv_usec = (suseconds_t)(last_collected_ut % USEC_PER_SEC);
    }

    usec_t chart_last_updated_ut = (usec_t)st->last_updated.tv_sec * USEC_PER_SEC + (usec_t)st->last_updated.tv_usec;
    usec_t last_updated_ut = last_updated_ut_str ? str2ull_encoded(last_updated_ut_str) : 0;
    if(last_updated_ut > chart_last_updated_ut) {
        st->last_updated.tv_sec = (time_t)(last_updated_ut / USEC_PER_SEC);
        st->last_updated.tv_usec = (suseconds_t)(last_updated_ut % USEC_PER_SEC);
    }

    st->counter++;
    st->counter_done++;

    return PARSER_RC_OK;
}

ALWAYS_INLINE PARSER_RC pluginsd_replay_end(char **words, size_t num_words, PARSER *parser) {
    if (num_words < 7) { // accepts 7, but the 7th is optional
        nd_log(NDLS_DAEMON, NDLP_ERR, "REPLAY: malformed " PLUGINSD_KEYWORD_REPLAY_END " command");
        RRDSET *st = pluginsd_get_scope_chart(parser);
        if(st)
            st->replication_empty_response_count = 0;
        return PARSER_RC_ERROR;
    }

    const char *update_every_child_txt = get_word(words, num_words, 1);
    const char *first_entry_child_txt = get_word(words, num_words, 2);
    const char *last_entry_child_txt = get_word(words, num_words, 3);
    const char *start_streaming_txt = get_word(words, num_words, 4);
    const char *first_entry_requested_txt = get_word(words, num_words, 5);
    const char *last_entry_requested_txt = get_word(words, num_words, 6);
    const char *child_world_time_txt = get_word(words, num_words, 7); // optional

    time_t update_every_child = (time_t) str2ull_encoded(update_every_child_txt);
    time_t first_entry_child = (time_t) str2ull_encoded(first_entry_child_txt);
    time_t last_entry_child = (time_t) str2ull_encoded(last_entry_child_txt);

    bool start_streaming = stream_parse_enable_streaming(start_streaming_txt);
    time_t first_entry_requested = (time_t) str2ull_encoded(first_entry_requested_txt);
    time_t last_entry_requested = (time_t) str2ull_encoded(last_entry_requested_txt);

    // the optional child world time
    time_t child_world_time = (child_world_time_txt && *child_world_time_txt) ? (time_t) str2ull_encoded(
            child_world_time_txt) : now_realtime_sec();

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_REPLAY_END);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);
    __atomic_add_fetch(&host->stream.rcv.status.replication.counter_in, 1, __ATOMIC_RELAXED);

    RRDSET *st = pluginsd_require_scope_chart(parser, PLUGINSD_KEYWORD_REPLAY_END, PLUGINSD_KEYWORD_REPLAY_BEGIN);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

#ifdef NETDATA_LOG_REPLICATION_REQUESTS
    internal_error(true,
                   "PLUGINSD REPLAY: 'host:%s/chart:%s': got a " PLUGINSD_KEYWORD_REPLAY_END " child db from %llu to %llu, start_streaming %s, had requested from %llu to %llu, wall clock %llu",
                   rrdhost_hostname(host), rrdset_id(st),
                   (unsigned long long)first_entry_child, (unsigned long long)last_entry_child,
                   start_streaming?"true":"false",
                   (unsigned long long)first_entry_requested, (unsigned long long)last_entry_requested,
                   (unsigned long long)child_world_time
                   );
#endif

    parser->user.data_collections_count++;

    // Reset empty response counter when we receive actual data
    if(parser->user.replay.rset_enabled && st)
        st->replication_empty_response_count = 0;

    if(parser->user.replay.rset_enabled && st->rrdhost->receiver) {
        time_t now = now_realtime_sec();
        time_t started = st->rrdhost->receiver->replication.first_time_s;
        time_t current = parser->user.replay.end_time;

        if(started && current > started) {
            host->stream.rcv.status.replication.percent = (NETDATA_DOUBLE) (current - started) * 100.0 / (NETDATA_DOUBLE) (now - started);
            worker_set_metric(WORKER_RECEIVER_JOB_REPLICATION_COMPLETION,
                              host->stream.rcv.status.replication.percent);
        }
    }

    parser->user.replay.start_time = 0;
    parser->user.replay.end_time = 0;
    parser->user.replay.start_time_ut = 0;
    parser->user.replay.end_time_ut = 0;
    parser->user.replay.wall_clock_time = 0;
    parser->user.replay.rset_enabled = false;

    st->counter++;
    st->counter_done++;
    store_metric_collection_completed();

#ifdef NETDATA_LOG_REPLICATION_REQUESTS
    st->replay.start_streaming = false;
    st->replay.after = 0;
    st->replay.before = 0;
    if(start_streaming)
        st->replay.log_next_data_collection = true;
#endif

    if(start_streaming)
        st->replication_empty_response_count = 0;

    if (start_streaming) {
#ifdef REPLICATION_TRACKING
        st->stream.rcv.who = REPLAY_WHO_FINISHED;
#endif

        if (st->update_every != update_every_child)
            rrdset_set_update_every_s(st, update_every_child);

        RRDSET_FLAGS old = rrdset_flag_set_and_clear(
            st, RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED,
            RRDSET_FLAG_RECEIVER_REPLICATION_IN_PROGRESS | RRDSET_FLAG_SYNC_CLOCK);

        if(!(old & RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED)) {
            if(rrdhost_receiver_replicating_charts_minus_one(st->rrdhost) == 0)
                pulse_host_status(host, PULSE_HOST_STATUS_RCV_RUNNING, 0);
        }
        else
            nd_log(NDLS_DAEMON, NDLP_WARNING,
                "PLUGINSD REPLAY ERROR: 'host:%s/chart:%s' got a " PLUGINSD_KEYWORD_REPLAY_END " "
                "with enable_streaming = true, but there was no replication in progress for this chart.",
                rrdhost_hostname(host), rrdset_id(st));

        pluginsd_clear_scope_chart(parser, PLUGINSD_KEYWORD_REPLAY_END);

        host->stream.rcv.status.replication.percent = 100.0;
        worker_set_metric(WORKER_RECEIVER_JOB_REPLICATION_COMPLETION, host->stream.rcv.status.replication.percent);

        stream_thread_received_replication();

        return PARSER_RC_OK;
    }

    // ========================================================================
    // SAFETY NET: Detect stuck replication loops
    // ========================================================================
    //
    // We received start_streaming=false, which means we need to send another
    // replication request. However, we need to detect if we're stuck in an
    // infinite retry loop where no progress is being made.
    //
    // This can happen when:
    // 1. Parent already has newer data than child
    // 2. Child keeps splitting responses due to buffer constraints
    // 3. Network issues causing repeated empty/failed responses

    // Check parent's current retention to detect if we're already caught up
    time_t local_first_entry = 0, local_last_entry = 0;
    rrdset_get_retention_of_tier_for_collected_chart(
        st, &local_first_entry, &local_last_entry, now_realtime_sec(), 0);

    // Detect suspicious pattern: parent requested data but is already caught up
    // This indicates we're in a loop where child keeps splitting responses
    // even though parent doesn't need more data.
    bool parent_already_caught_up = (local_last_entry >= last_entry_child);
    bool requested_non_empty_range = (first_entry_requested != 0 || last_entry_requested != 0);
    bool is_suspicious_response = (requested_non_empty_range && parent_already_caught_up);

    bool should_check_for_stuck_replication = false;

    // Track consecutive suspicious responses - applies to all builds
    if(is_suspicious_response) {
        st->replication_empty_response_count++;
        // After 3 consecutive suspicious responses, we need to investigate
        if(st->replication_empty_response_count >= 3) {
            should_check_for_stuck_replication = true;
        }
    } else {
        // Reset counter if this was a legitimate response (parent still catching up)
        st->replication_empty_response_count = 0;
    }

    if (should_check_for_stuck_replication) {
        // We already have local_first_entry and local_last_entry from above

        // Check multiple conditions to ensure we're truly stuck:
        //
        // Condition 1: Parent has data that covers or exceeds child's retention
        // (We already checked this in parent_already_caught_up, but verify again)
        bool parent_has_equal_or_newer_data = (local_last_entry >= last_entry_child);

        // Calculate the gap for logging purposes
        time_t gap_to_child = (last_entry_child > local_last_entry) ?
                              (last_entry_child - local_last_entry) : 0;

        // Condition 2: Parent's data is reasonably recent
        time_t wall_clock = now_realtime_sec();
        bool parent_data_is_recent = (local_last_entry > 0 &&
                                     (wall_clock - local_last_entry) < 300);

        // Only finish replication if parent has equal or newer data than child
        // Do NOT terminate if there's any gap, as that would cause data loss
        if (parent_has_equal_or_newer_data) {

            // Log with appropriate level based on confidence
            ND_LOG_FIELD_PRIORITY level = (parent_has_equal_or_newer_data && parent_data_is_recent) ?
                                          NDLP_INFO : NDLP_WARNING;

            nd_log(NDLS_DAEMON, level,
                   "PLUGINSD REPLAY: 'host:%s/chart:%s' detected stuck replication loop. "
                   "Parent last entry: %llu, Child last entry: %llu, Gap: %llu seconds, "
                   "Empty responses: %u. Forcing replication to finish.",
                   rrdhost_hostname(host), rrdset_id(st),
                   (unsigned long long)local_last_entry,
                   (unsigned long long)last_entry_child,
                   (unsigned long long)gap_to_child,
                   (unsigned int)st->replication_empty_response_count
            );

            st->replication_empty_response_count = 0;

            // IMPORTANT: Mark as finished and decrement counter NOW, before sending final request.
            // This prevents infinite loops even if child continues to respond with start_streaming=false.
            // The next REPLAY_END will see FINISHED flag and handle accordingly.
            RRDSET_FLAGS old = rrdset_flag_set_and_clear(
                st, RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED,
                RRDSET_FLAG_RECEIVER_REPLICATION_IN_PROGRESS | RRDSET_FLAG_SYNC_CLOCK);

            if(!(old & RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED)) {
                if(rrdhost_receiver_replicating_charts_minus_one(st->rrdhost) == 0)
                    pulse_host_status(host, PULSE_HOST_STATUS_RCV_RUNNING, 0);
            }

            pluginsd_clear_scope_chart(parser, PLUGINSD_KEYWORD_REPLAY_END);
            host->stream.rcv.status.replication.percent = 100.0;
            worker_set_metric(WORKER_RECEIVER_JOB_REPLICATION_COMPLETION, host->stream.rcv.status.replication.percent);

            // Send one final request to notify child. If child responds with start_streaming=true,
            // it will start streaming. If it responds with start_streaming=false, the next
            // REPLAY_END will see the FINISHED flag and log a warning but not loop forever.
            bool ok = replicate_chart_request(send_to_plugin, parser, host, st,
                                             first_entry_child, last_entry_child, child_world_time,
                                             0, 0);  // prev_wanted = 0,0 to trigger empty request path

            return ok ? PARSER_RC_OK : PARSER_RC_ERROR;
        }
    }

#ifdef REPLICATION_TRACKING
    st->stream.rcv.who = REPLAY_WHO_ME;
#endif

    pluginsd_clear_scope_chart(parser, PLUGINSD_KEYWORD_REPLAY_END);

    rrdcontext_updated_retention_rrdset(st);

    bool ok = replicate_chart_request(send_to_plugin, parser, host, st,
                                      first_entry_child, last_entry_child, child_world_time,
                                      first_entry_requested, last_entry_requested);

    return ok ? PARSER_RC_OK : PARSER_RC_ERROR;
}

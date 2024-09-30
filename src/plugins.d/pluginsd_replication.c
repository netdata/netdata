// SPDX-License-Identifier: GPL-3.0-or-later

#include "pluginsd_replication.h"

PARSER_RC pluginsd_replay_begin(char **words, size_t num_words, PARSER *parser) {
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

        netdata_log_error("PLUGINSD REPLAY ERROR: 'host:%s/chart:%s' got a " PLUGINSD_KEYWORD_REPLAY_BEGIN
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

PARSER_RC pluginsd_replay_set(char **words, size_t num_words, PARSER *parser) {
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
                     "PLUGINSD: 'host:%s/chart:%s' got a %s but it is disabled by %s errors",
                     rrdhost_hostname(host), rrdset_id(st), PLUGINSD_KEYWORD_REPLAY_SET, PLUGINSD_KEYWORD_REPLAY_BEGIN);

        // we have to return OK here
        return PARSER_RC_OK;
    }

    RRDDIM *rd = pluginsd_acquire_dimension(host, st, dimension, slot, PLUGINSD_KEYWORD_REPLAY_SET);
    if(!rd) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    st->pluginsd.set = true;

    if (unlikely(!parser->user.replay.start_time || !parser->user.replay.end_time)) {
        netdata_log_error("PLUGINSD: 'host:%s/chart:%s/dim:%s' got a %s with invalid timestamps %ld to %ld from a %s. Disabling it.",
                          rrdhost_hostname(host),
                          rrdset_id(st),
                          dimension,
                          PLUGINSD_KEYWORD_REPLAY_SET,
                          parser->user.replay.start_time,
                          parser->user.replay.end_time,
                          PLUGINSD_KEYWORD_REPLAY_BEGIN);
        return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);
    }

    if (unlikely(!value_str || !*value_str))
        value_str = "NAN";

    if(unlikely(!flags_str))
        flags_str = "";

    if (likely(value_str)) {
        RRDDIM_FLAGS rd_flags = rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE | RRDDIM_FLAG_ARCHIVED);

        if(!(rd_flags & RRDDIM_FLAG_ARCHIVED)) {
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
        else {
            nd_log_limit_static_global_var(erl, 1, 0);
            nd_log_limit(&erl, NDLS_COLLECTORS, NDLP_WARNING,
                         "PLUGINSD: 'host:%s/chart:%s/dim:%s' has the ARCHIVED flag set, but it is replicated. "
                         "Ignoring data.",
                         rrdhost_hostname(st->rrdhost), rrdset_id(st), rrddim_name(rd));
        }
    }

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_replay_rrddim_collection_state(char **words, size_t num_words, PARSER *parser) {
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
        rd->collector.last_collected_time.tv_usec = (last_collected_ut % USEC_PER_SEC);
    }

    rd->collector.last_collected_value = last_collected_value_str ? str2ll_encoded(last_collected_value_str) : 0;
    rd->collector.last_calculated_value = last_calculated_value_str ? str2ndd_encoded(last_calculated_value_str, NULL) : 0;
    rd->collector.last_stored_value = last_stored_value_str ? str2ndd_encoded(last_stored_value_str, NULL) : 0.0;

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_replay_rrdset_collection_state(char **words, size_t num_words, PARSER *parser) {
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
        st->last_collected_time.tv_usec = (last_collected_ut % USEC_PER_SEC);
    }

    usec_t chart_last_updated_ut = (usec_t)st->last_updated.tv_sec * USEC_PER_SEC + (usec_t)st->last_updated.tv_usec;
    usec_t last_updated_ut = last_updated_ut_str ? str2ull_encoded(last_updated_ut_str) : 0;
    if(last_updated_ut > chart_last_updated_ut) {
        st->last_updated.tv_sec = (time_t)(last_updated_ut / USEC_PER_SEC);
        st->last_updated.tv_usec = (last_updated_ut % USEC_PER_SEC);
    }

    st->counter++;
    st->counter_done++;

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_replay_end(char **words, size_t num_words, PARSER *parser) {
    if (num_words < 7) { // accepts 7, but the 7th is optional
        netdata_log_error("REPLAY: malformed " PLUGINSD_KEYWORD_REPLAY_END " command");
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

    bool start_streaming = (strcmp(start_streaming_txt, "true") == 0);
    time_t first_entry_requested = (time_t) str2ull_encoded(first_entry_requested_txt);
    time_t last_entry_requested = (time_t) str2ull_encoded(last_entry_requested_txt);

    // the optional child world time
    time_t child_world_time = (child_world_time_txt && *child_world_time_txt) ? (time_t) str2ull_encoded(
            child_world_time_txt) : now_realtime_sec();

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_REPLAY_END);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

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

    if(parser->user.replay.rset_enabled && st->rrdhost->receiver) {
        time_t now = now_realtime_sec();
        time_t started = st->rrdhost->receiver->replication_first_time_t;
        time_t current = parser->user.replay.end_time;

        if(started && current > started) {
            host->rrdpush_receiver_replication_percent = (NETDATA_DOUBLE) (current - started) * 100.0 / (NETDATA_DOUBLE) (now - started);
            worker_set_metric(WORKER_RECEIVER_JOB_REPLICATION_COMPLETION,
                              host->rrdpush_receiver_replication_percent);
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

    if (start_streaming) {
        if (st->update_every != update_every_child)
            rrdset_set_update_every_s(st, update_every_child);

        if(rrdset_flag_check(st, RRDSET_FLAG_RECEIVER_REPLICATION_IN_PROGRESS)) {
            rrdset_flag_set(st, RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED);
            rrdset_flag_clear(st, RRDSET_FLAG_RECEIVER_REPLICATION_IN_PROGRESS);
            rrdset_flag_clear(st, RRDSET_FLAG_SYNC_CLOCK);
            rrdhost_receiver_replicating_charts_minus_one(st->rrdhost);
        }
#ifdef NETDATA_LOG_REPLICATION_REQUESTS
        else
            internal_error(true, "REPLAY ERROR: 'host:%s/chart:%s' got a " PLUGINSD_KEYWORD_REPLAY_END " with enable_streaming = true, but there is no replication in progress for this chart.",
                  rrdhost_hostname(host), rrdset_id(st));
#endif

        pluginsd_clear_scope_chart(parser, PLUGINSD_KEYWORD_REPLAY_END);

        host->rrdpush_receiver_replication_percent = 100.0;
        worker_set_metric(WORKER_RECEIVER_JOB_REPLICATION_COMPLETION, host->rrdpush_receiver_replication_percent);

        return PARSER_RC_OK;
    }

    pluginsd_clear_scope_chart(parser, PLUGINSD_KEYWORD_REPLAY_END);

    rrdcontext_updated_retention_rrdset(st);

    bool ok = replicate_chart_request(send_to_plugin, parser, host, st,
                                      first_entry_child, last_entry_child, child_world_time,
                                      first_entry_requested, last_entry_requested);
    return ok ? PARSER_RC_OK : PARSER_RC_ERROR;
}

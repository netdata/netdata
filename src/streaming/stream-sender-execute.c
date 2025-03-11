// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream-thread.h"
#include "stream-replication-receiver.h"
#include "stream-replication-sender.h"

struct inflight_stream_function {
    struct sender_state *sender;
    STRING *transaction;
    usec_t received_ut;
};

static void stream_execute_function_callback(BUFFER *func_wb, int code, void *data) {
    struct inflight_stream_function *tmp = data;
    struct sender_state *s = tmp->sender;

    if(rrdhost_can_stream_metadata_to_parent(s->host)) {
        // for functions we use a new buffer, to avoid keeping a big buffer in memory
        CLEAN_BUFFER *wb = buffer_create(0, NULL);

        pluginsd_function_result_begin_to_buffer(
            wb, string2str(tmp->transaction), code,
            content_type_id2string(func_wb->content_type), func_wb->expires);

        buffer_fast_strcat(wb, buffer_tostring(func_wb), buffer_strlen(func_wb));
        pluginsd_function_result_end_to_buffer(wb);

        sender_commit_clean_buffer(s, wb, STREAM_TRAFFIC_TYPE_FUNCTIONS);

        internal_error(true, "STREAM SND '%s' [to %s]: FUNCTION transaction %s sending back response (%zu bytes, %"PRIu64" usec).",
                       rrdhost_hostname(s->host), s->remote_ip,
                       string2str(tmp->transaction),
                       buffer_strlen(func_wb),
                       now_realtime_usec() - tmp->received_ut);
    }

    string_freez(tmp->transaction);
    buffer_free(func_wb);
    freez(tmp);
}

static void stream_execute_function_progress_callback(void *data, size_t done, size_t all) {
    struct inflight_stream_function *tmp = data;
    struct sender_state *s = tmp->sender;

    if(rrdhost_can_stream_metadata_to_parent(s->host)) {
        CLEAN_BUFFER *wb = buffer_create(0, NULL);

        buffer_sprintf(wb, PLUGINSD_KEYWORD_FUNCTION_PROGRESS " '%s' %zu %zu\n",
                       string2str(tmp->transaction), done, all);

        sender_commit_clean_buffer(s, wb, STREAM_TRAFFIC_TYPE_FUNCTIONS);
    }
}

static void execute_commands_function(struct sender_state *s, const char *command, const char *transaction, const char *timeout_s, const char *function, BUFFER *payload, const char *access, const char *source) {
    worker_is_busy(WORKER_SENDER_JOB_EXECUTE_FUNCTION);
    nd_log(NDLS_ACCESS, NDLP_INFO, NULL);

    if(!transaction || !*transaction || !timeout_s || !*timeout_s || !function || !*function) {
        netdata_log_error("STREAM SND '%s' [to %s]: %s execution command is incomplete (transaction = '%s', timeout = '%s', function = '%s'). Ignoring it.",
                          rrdhost_hostname(s->host), s->remote_ip,
                          command,
                          transaction?transaction:"(unset)",
                          timeout_s?timeout_s:"(unset)",
                          function?function:"(unset)");
    }
    else {
        int timeout = str2i(timeout_s);
        if(timeout <= 0) timeout = PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT;

        struct inflight_stream_function *tmp = callocz(1, sizeof(struct inflight_stream_function));
        tmp->received_ut = now_realtime_usec();
        tmp->sender = s;
        tmp->transaction = string_strdupz(transaction);
        BUFFER *wb = buffer_create(1024, &netdata_buffers_statistics.buffers_functions);

        rrd_function_run(s->host, wb, timeout,
                         http_access_from_hex_mapping_old_roles(access), function, false, transaction,
                         stream_execute_function_callback, tmp,
                         stream_has_capability(s, STREAM_CAP_PROGRESS) ? stream_execute_function_progress_callback : NULL,
                         stream_has_capability(s, STREAM_CAP_PROGRESS) ? tmp : NULL,
                         NULL, NULL, payload, source, true);
    }
}

struct deferred_function {
    const char *transaction;
    const char *timeout_s;
    const char *function;
    const char *access;
    const char *source;
};

static void execute_deferred_function(struct sender_state *s, void *data) {
    struct deferred_function *dfd = data;
    execute_commands_function(s, s->thread.defer.end_keyword,
                              dfd->transaction, dfd->timeout_s,
                              dfd->function, s->thread.defer.payload,
                              dfd->access, dfd->source);
}

static void execute_deferred_json(struct sender_state *s, void *data) {
    const char *keyword = data;

    if(strcmp(keyword, PLUGINSD_KEYWORD_JSON_CMD_STREAM_PATH) == 0)
        stream_path_set_from_json(s->host, buffer_tostring(s->thread.defer.payload), true);
    else
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM SND '%s' [to %s]: unknown JSON keyword '%s' with payload: %s",
               rrdhost_hostname(s->host), s->remote_ip,
               keyword, buffer_tostring(s->thread.defer.payload));
}

static void cleanup_deferred_json(struct sender_state *s __maybe_unused, void *data) {
    const char *keyword = data;
    freez((void *)keyword);
}

static void cleanup_deferred_function(struct sender_state *s __maybe_unused, void *data) {
    struct deferred_function *dfd = data;
    freez((void *)dfd->transaction);
    freez((void *)dfd->timeout_s);
    freez((void *)dfd->function);
    freez((void *)dfd->access);
    freez((void *)dfd->source);
    freez(dfd);
}

static void cleanup_deferred_data(struct sender_state *s) {
    if(s->thread.defer.cleanup)
        s->thread.defer.cleanup(s, s->thread.defer.action_data);

    buffer_free(s->thread.defer.payload);
    s->thread.defer.payload = NULL;
    s->thread.defer.end_keyword = NULL;
    s->thread.defer.action = NULL;
    s->thread.defer.cleanup = NULL;
    s->thread.defer.action_data = NULL;
}

void stream_sender_execute_commands_cleanup(struct sender_state *s) {
    cleanup_deferred_data(s);
}

// This is just a placeholder until the gap filling state machine is inserted
void stream_sender_execute_commands(struct sender_state *s) {
    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_CB(NDF_REQUEST, line_splitter_reconstruct_line, &s->thread.rbuf.line),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

#ifdef NETDATA_LOG_STREAM_SENDER
    if(!s->log.received)
        s->log.received = buffer_create(0, NULL);
#endif

    char *start = s->thread.rbuf.b, *end = &s->thread.rbuf.b[s->thread.rbuf.read_len], *newline;
    *end = '\0';
    for( ; start < end ; start = newline + 1) {
        newline = strchr(start, '\n');

        if(!newline) {
            if(s->thread.defer.end_keyword) {
                buffer_strcat(s->thread.defer.payload, start);
                start = end;
            }
            break;
        }

        *newline = '\0';
        s->thread.rbuf.line.count++;

        if(s->thread.defer.end_keyword) {
            if(strcmp(start, s->thread.defer.end_keyword) == 0) {
#ifdef NETDATA_LOG_STREAM_SENDER
                buffer_strcat(s->log.received, buffer_tostring(s->thread.defer.payload));
                buffer_strcat(s->log.received, "\n");
                buffer_strcat(s->log.received, s->thread.defer.end_keyword);
                buffer_strcat(s->log.received, "\n");
                stream_sender_log_payload(s, s->log.received, STREAM_TRAFFIC_TYPE_METADATA, true);
#endif
                s->thread.defer.action(s, s->thread.defer.action_data);
                cleanup_deferred_data(s);
            }
            else {
                buffer_strcat(s->thread.defer.payload, start);
                buffer_putc(s->thread.defer.payload, '\n');
            }

            continue;
        }

#ifdef NETDATA_LOG_STREAM_SENDER
        buffer_reset(s->log.received);
        buffer_strcat(s->log.received, start);
        buffer_strcat(s->log.received, "\n");
#endif

        s->thread.rbuf.line.num_words = quoted_strings_splitter_whitespace(start, s->thread.rbuf.line.words, PLUGINSD_MAX_WORDS);
        const char *command = get_word(s->thread.rbuf.line.words, s->thread.rbuf.line.num_words, 0);

        if(command && strcmp(command, PLUGINSD_CALL_FUNCTION) == 0) {
#ifdef NETDATA_LOG_STREAM_SENDER
            stream_sender_log_payload(s, s->log.received, STREAM_TRAFFIC_TYPE_FUNCTIONS, true);
#endif
            char *transaction  = get_word(s->thread.rbuf.line.words, s->thread.rbuf.line.num_words, 1);
            char *timeout_s    = get_word(s->thread.rbuf.line.words, s->thread.rbuf.line.num_words, 2);
            char *function     = get_word(s->thread.rbuf.line.words, s->thread.rbuf.line.num_words, 3);
            char *access       = get_word(s->thread.rbuf.line.words, s->thread.rbuf.line.num_words, 4);
            char *source       = get_word(s->thread.rbuf.line.words, s->thread.rbuf.line.num_words, 5);

            execute_commands_function(s, command, transaction, timeout_s, function, NULL, access, source);
        }
        else if(command && strcmp(command, PLUGINSD_CALL_FUNCTION_PAYLOAD_BEGIN) == 0) {
            char *transaction  = get_word(s->thread.rbuf.line.words, s->thread.rbuf.line.num_words, 1);
            char *timeout_s    = get_word(s->thread.rbuf.line.words, s->thread.rbuf.line.num_words, 2);
            char *function     = get_word(s->thread.rbuf.line.words, s->thread.rbuf.line.num_words, 3);
            char *access       = get_word(s->thread.rbuf.line.words, s->thread.rbuf.line.num_words, 4);
            char *source       = get_word(s->thread.rbuf.line.words, s->thread.rbuf.line.num_words, 5);
            char *content_type = get_word(s->thread.rbuf.line.words, s->thread.rbuf.line.num_words, 6);

            s->thread.defer.end_keyword = PLUGINSD_CALL_FUNCTION_PAYLOAD_END;
            s->thread.defer.payload = buffer_create(0, NULL);
            s->thread.defer.payload->content_type = content_type_string2id(content_type);
            s->thread.defer.action = execute_deferred_function;
            s->thread.defer.cleanup = cleanup_deferred_function;

            struct deferred_function *dfd = callocz(1, sizeof(*dfd));
            dfd->transaction = strdupz(transaction ? transaction : "");
            dfd->timeout_s = strdupz(timeout_s ? timeout_s : "");
            dfd->function = strdupz(function ? function : "");
            dfd->access = strdupz(access ? access : "");
            dfd->source = strdupz(source ? source : "");

            s->thread.defer.action_data = dfd;
        }
        else if(command && strcmp(command, PLUGINSD_CALL_FUNCTION_CANCEL) == 0) {
            worker_is_busy(WORKER_SENDER_JOB_EXECUTE_FUNCTION);
#ifdef NETDATA_LOG_STREAM_SENDER
            stream_sender_log_payload(s, s->log.received, STREAM_TRAFFIC_TYPE_FUNCTIONS, true);
#endif
            nd_log(NDLS_ACCESS, NDLP_DEBUG, NULL);

            char *transaction = get_word(s->thread.rbuf.line.words, s->thread.rbuf.line.num_words, 1);
            if(transaction && *transaction)
                rrd_function_cancel(transaction);
        }
        else if(command && strcmp(command, PLUGINSD_CALL_FUNCTION_PROGRESS) == 0) {
            worker_is_busy(WORKER_SENDER_JOB_EXECUTE_FUNCTION);
#ifdef NETDATA_LOG_STREAM_SENDER
            stream_sender_log_payload(s, s->log.received, STREAM_TRAFFIC_TYPE_FUNCTIONS, true);
#endif
            nd_log(NDLS_ACCESS, NDLP_DEBUG, NULL);

            char *transaction = get_word(s->thread.rbuf.line.words, s->thread.rbuf.line.num_words, 1);
            if(transaction && *transaction)
                rrd_function_progress(transaction);
        }
        else if (command && strcmp(command, PLUGINSD_KEYWORD_REPLAY_CHART) == 0) {
            worker_is_busy(WORKER_SENDER_JOB_EXECUTE_REPLAY);
#ifdef NETDATA_LOG_STREAM_SENDER
            stream_sender_log_payload(s, s->log.received, STREAM_TRAFFIC_TYPE_REPLICATION, true);
#endif

            __atomic_add_fetch(&s->host->stream.snd.status.replication.counter_in, 1, __ATOMIC_RELAXED);

            // do not log replication commands received - way too many!
            // nd_log(NDLS_ACCESS, NDLP_DEBUG, NULL);

            const char *chart_id = get_word(s->thread.rbuf.line.words, s->thread.rbuf.line.num_words, 1);
            const char *start_streaming = get_word(s->thread.rbuf.line.words, s->thread.rbuf.line.num_words, 2);
            const char *after = get_word(s->thread.rbuf.line.words, s->thread.rbuf.line.num_words, 3);
            const char *before = get_word(s->thread.rbuf.line.words, s->thread.rbuf.line.num_words, 4);

            if (!chart_id || !start_streaming || !after || !before) {
                netdata_log_error("STREAM REPLAY ERROR '%s' [send to %s] %s command is incomplete"
                                  " (chart=%s, start_streaming=%s, after=%s, before=%s)",
                                  rrdhost_hostname(s->host), s->remote_ip,
                                  command,
                                  chart_id ? chart_id : "(unset)",
                                  start_streaming ? start_streaming : "(unset)",
                                  after ? after : "(unset)",
                                  before ? before : "(unset)");
            }
            else {
#ifdef REPLICATION_TRACKING
                RRDSET *st = rrdset_find(s->host, chart_id);
                if(st)
                    st->stream.snd.who = REPLAY_WHO_ME;
#endif

                replication_sender_request_add(
                    s, chart_id,
                    strtoll(after, NULL, 0),
                    strtoll(before, NULL, 0),
                    stream_parse_enable_streaming(start_streaming));
            }
        }
        else if(command && strcmp(command, PLUGINSD_KEYWORD_NODE_ID) == 0) {
            worker_is_busy(WORKER_SENDER_JOB_EXECUTE_META);
#ifdef NETDATA_LOG_STREAM_SENDER
            stream_sender_log_payload(s, s->log.received, STREAM_TRAFFIC_TYPE_METADATA, true);
#endif
            char *claim_id_str = get_word(s->thread.rbuf.line.words, s->thread.rbuf.line.num_words, 1);
            char *node_id_str = get_word(s->thread.rbuf.line.words, s->thread.rbuf.line.num_words, 2);
            char *url = get_word(s->thread.rbuf.line.words, s->thread.rbuf.line.num_words, 3);

            stream_sender_get_node_and_claim_id_from_parent(s, claim_id_str, node_id_str, url);
        }
        else if(command && strcmp(command, PLUGINSD_KEYWORD_JSON) == 0) {
            worker_is_busy(WORKER_SENDER_JOB_EXECUTE_META);

            char *keyword = get_word(s->thread.rbuf.line.words, s->thread.rbuf.line.num_words, 1);

            s->thread.defer.end_keyword = PLUGINSD_KEYWORD_JSON_END;
            s->thread.defer.payload = buffer_create(0, NULL);
            s->thread.defer.action = execute_deferred_json;
            s->thread.defer.cleanup = cleanup_deferred_json;
            s->thread.defer.action_data = strdupz(keyword);
        }
        else {
            netdata_log_error("STREAM SND '%s' [to %s] received unknown command over connection: %s",
                              rrdhost_hostname(s->host), s->remote_ip, s->thread.rbuf.line.words[0]?s->thread.rbuf.line.words[0]:"(unset)");
        }

        line_splitter_reset(&s->thread.rbuf.line);
    }

    if (start < end) {
        memmove(s->thread.rbuf.b, start, end-start);
        s->thread.rbuf.read_len = end - start;
    }
    else {
        s->thread.rbuf.b[0] = '\0';
        s->thread.rbuf.read_len = 0;
    }
}

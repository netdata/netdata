// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream-sender-internals.h"

static struct {
    const char *response;
    size_t length;
    int32_t version;
    bool dynamic;
    const char *error;
    int worker_job_id;
    int postpone_reconnect_seconds;
    ND_LOG_FIELD_PRIORITY priority;
} stream_responses[] = {
    {
        .response = START_STREAMING_PROMPT_VN,
        .length = sizeof(START_STREAMING_PROMPT_VN) - 1,
        .version = STREAM_HANDSHAKE_OK_V3, // and above
        .dynamic = true,                 // dynamic = we will parse the version / capabilities
        .error = NULL,
        .worker_job_id = 0,
        .postpone_reconnect_seconds = 0,
        .priority = NDLP_INFO,
    },
    {
        .response = START_STREAMING_PROMPT_V2,
        .length = sizeof(START_STREAMING_PROMPT_V2) - 1,
        .version = STREAM_HANDSHAKE_OK_V2,
        .dynamic = false,
        .error = NULL,
        .worker_job_id = 0,
        .postpone_reconnect_seconds = 0,
        .priority = NDLP_INFO,
    },
    {
        .response = START_STREAMING_PROMPT_V1,
        .length = sizeof(START_STREAMING_PROMPT_V1) - 1,
        .version = STREAM_HANDSHAKE_OK_V1,
        .dynamic = false,
        .error = NULL,
        .worker_job_id = 0,
        .postpone_reconnect_seconds = 0,
        .priority = NDLP_INFO,
    },
    {
        .response = START_STREAMING_ERROR_SAME_LOCALHOST,
        .length = sizeof(START_STREAMING_ERROR_SAME_LOCALHOST) - 1,
        .version = STREAM_HANDSHAKE_PARENT_IS_LOCALHOST,
        .dynamic = false,
        .error = "remote server rejected this stream, the host we are trying to stream is its localhost",
        .worker_job_id = WORKER_SENDER_CONNECTOR_JOB_DISCONNECT_BAD_HANDSHAKE,
        .postpone_reconnect_seconds = 60 * 60, // the IP may change, try it every hour
        .priority = NDLP_DEBUG,
    },
    {
        .response = START_STREAMING_ERROR_ALREADY_STREAMING,
        .length = sizeof(START_STREAMING_ERROR_ALREADY_STREAMING) - 1,
        .version = STREAM_HANDSHAKE_PARENT_NODE_ALREADY_CONNECTED,
        .dynamic = false,
        .error = "remote server rejected this stream, the host we are trying to stream is already streamed to it",
        .worker_job_id = WORKER_SENDER_CONNECTOR_JOB_DISCONNECT_BAD_HANDSHAKE,
        .postpone_reconnect_seconds = 2 * 60, // 2 minutes
        .priority = NDLP_DEBUG,
    },
    {
        .response = START_STREAMING_ERROR_NOT_PERMITTED,
        .length = sizeof(START_STREAMING_ERROR_NOT_PERMITTED) - 1,
        .version = STREAM_HANDSHAKE_PARENT_DENIED_ACCESS,
        .dynamic = false,
        .error = "remote server denied access, probably we don't have the right API key?",
        .worker_job_id = WORKER_SENDER_CONNECTOR_JOB_DISCONNECT_BAD_HANDSHAKE,
        .postpone_reconnect_seconds = 1 * 60, // 1 minute
        .priority = NDLP_ERR,
    },
    {
        .response = START_STREAMING_ERROR_BUSY_TRY_LATER,
        .length = sizeof(START_STREAMING_ERROR_BUSY_TRY_LATER) - 1,
        .version = STREAM_HANDSHAKE_PARENT_BUSY_TRY_LATER,
        .dynamic = false,
        .error = "remote server is currently busy, we should try later",
        .worker_job_id = WORKER_SENDER_CONNECTOR_JOB_DISCONNECT_BAD_HANDSHAKE,
        .postpone_reconnect_seconds = 2 * 60, // 2 minutes
        .priority = NDLP_NOTICE,
    },
    {
        .response = START_STREAMING_ERROR_INTERNAL_ERROR,
        .length = sizeof(START_STREAMING_ERROR_INTERNAL_ERROR) - 1,
        .version = STREAM_HANDSHAKE_PARENT_INTERNAL_ERROR,
        .dynamic = false,
        .error = "remote server is encountered an internal error, we should try later",
        .worker_job_id = WORKER_SENDER_CONNECTOR_JOB_DISCONNECT_BAD_HANDSHAKE,
        .postpone_reconnect_seconds = 5 * 60, // 5 minutes
        .priority = NDLP_CRIT,
    },
    {
        .response = START_STREAMING_ERROR_INITIALIZATION,
        .length = sizeof(START_STREAMING_ERROR_INITIALIZATION) - 1,
        .version = STREAM_HANDSHAKE_PARENT_IS_INITIALIZING,
        .dynamic = false,
        .error = "remote server is initializing, we should try later",
        .worker_job_id = WORKER_SENDER_CONNECTOR_JOB_DISCONNECT_BAD_HANDSHAKE,
        .postpone_reconnect_seconds = 30, // 30 seconds
        .priority = NDLP_NOTICE,
    },

    // terminator
    {
        .response = NULL,
        .length = 0,
        .version = STREAM_HANDSHAKE_CONNECT_HANDSHAKE_FAILED,
        .dynamic = false,
        .error = "remote node response is not understood, is it Netdata?",
        .worker_job_id = WORKER_SENDER_CONNECTOR_JOB_DISCONNECT_BAD_HANDSHAKE,
        .postpone_reconnect_seconds = 1 * 60, // 1 minute
        .priority = NDLP_ERR,
    }
};

#define CONN_UPGRADE_VAL "upgrade"
static int stream_connect_upgrade_prelude(RRDHOST *host __maybe_unused, struct sender_state *s) {

    char http[HTTP_HEADER_SIZE + 1];
    snprintfz(http, HTTP_HEADER_SIZE,
              "GET " NETDATA_STREAM_URL HTTP_1_1 HTTP_ENDL
              "Upgrade: " NETDATA_STREAM_PROTO_NAME HTTP_ENDL
              "Connection: Upgrade"
              HTTP_HDR_END);

    ssize_t bytes;
    bytes = nd_sock_send_timeout(&s->sock, http, strlen(http), 0, 1000);
    if (bytes <= 0) {
        error_report("Error writing to remote");
        return 1;
    }

    bytes = nd_sock_recv_timeout(&s->sock, http, HTTP_HEADER_SIZE, 0, 1000);
    if (bytes <= 0) {
        error_report("Error reading from remote");
        return 1;
    }

    rbuf_t buf = rbuf_create(bytes);
    rbuf_push(buf, http, bytes);

    http_parse_ctx ctx;
    http_parse_ctx_create(&ctx, HTTP_PARSE_INITIAL);
    ctx.flags |= HTTP_PARSE_FLAG_DONT_WAIT_FOR_CONTENT;

    int rc;
    //    while((rc = parse_http_response(buf, &ctx)) == HTTP_PARSE_NEED_MORE_DATA);
    rc = parse_http_response(buf, &ctx);

    if (rc != HTTP_PARSE_SUCCESS) {
        error_report("Failed to parse HTTP response sent. (%d)", rc);
        goto err_cleanup;
    }
    if (ctx.http_code == HTTP_RESP_MOVED_PERM) {
        const char *hdr = get_http_header_by_name(&ctx, "location");
        if (hdr)
            error_report("HTTP response is %d Moved Permanently (location: \"%s\") instead of expected %d Switching Protocols.", ctx.http_code, hdr, HTTP_RESP_SWITCH_PROTO);
        else
            error_report("HTTP response is %d instead of expected %d Switching Protocols.", ctx.http_code, HTTP_RESP_SWITCH_PROTO);
        goto err_cleanup;
    }
    if (ctx.http_code == HTTP_RESP_NOT_FOUND) {
        error_report("HTTP response is %d instead of expected %d Switching Protocols. Parent version too old.", ctx.http_code, HTTP_RESP_SWITCH_PROTO);
        // TODO set some flag here that will signify parent is older version
        // and to try connection without rrdpush_http_upgrade_prelude next time
        goto err_cleanup;
    }
    if (ctx.http_code != HTTP_RESP_SWITCH_PROTO) {
        error_report("HTTP response is %d instead of expected %d Switching Protocols", ctx.http_code, HTTP_RESP_SWITCH_PROTO);
        goto err_cleanup;
    }

    const char *hdr = get_http_header_by_name(&ctx, "connection");
    if (!hdr) {
        error_report("Missing \"connection\" header in reply");
        goto err_cleanup;
    }
    if (strncmp(hdr, CONN_UPGRADE_VAL, strlen(CONN_UPGRADE_VAL)) != 0) {
        error_report("Expected \"connection: " CONN_UPGRADE_VAL "\"");
        goto err_cleanup;
    }

    hdr = get_http_header_by_name(&ctx, "upgrade");
    if (!hdr) {
        error_report("Missing \"upgrade\" header in reply");
        goto err_cleanup;
    }
    if (strncmp(hdr, NETDATA_STREAM_PROTO_NAME, strlen(NETDATA_STREAM_PROTO_NAME)) != 0) {
        error_report("Expected \"upgrade: " NETDATA_STREAM_PROTO_NAME "\"");
        goto err_cleanup;
    }

    netdata_log_debug(D_STREAM, "STREAM SNDer upgrade to \"" NETDATA_STREAM_PROTO_NAME "\" successful");
    rbuf_free(buf);
    http_parse_ctx_destroy(&ctx);
    return 0;
err_cleanup:
    rbuf_free(buf);
    http_parse_ctx_destroy(&ctx);
    return 1;
}

static bool
stream_connect_validate_first_response(RRDHOST *host, struct sender_state *s, char *http, size_t http_length) {
    int32_t version = STREAM_HANDSHAKE_CONNECT_HANDSHAKE_FAILED;

    int i;
    for(i = 0; stream_responses[i].response ; i++) {
        if(stream_responses[i].dynamic &&
            http_length > stream_responses[i].length && http_length < (stream_responses[i].length + 30) &&
            strncmp(http, stream_responses[i].response, stream_responses[i].length) == 0) {

            version = str2i(&http[stream_responses[i].length]);
            break;
        }
        else if(http_length == stream_responses[i].length && strcmp(http, stream_responses[i].response) == 0) {
            version = stream_responses[i].version;

            break;
        }
    }

    if(version >= STREAM_HANDSHAKE_OK_V1) {
        stream_parent_set_host_reconnect_delay(
            host, STREAM_HANDSHAKE_SP_CONNECTED, stream_send.parents.reconnect_delay_s);
        s->capabilities = convert_stream_version_to_capabilities(version, host, true);
        s->host->stream.snd.status.reason = (STREAM_HANDSHAKE)s->capabilities;
        return true;
    }

    ND_LOG_FIELD_PRIORITY priority = stream_responses[i].priority;
    const char *error = stream_responses[i].error;
    int worker_job_id = stream_responses[i].worker_job_id;
    int delay = stream_responses[i].postpone_reconnect_seconds;

    worker_is_busy(worker_job_id);
    stream_parent_set_host_connect_failure_reason(host, version, delay);

    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_I64(NDF_RESPONSE_CODE, stream_handshake_error_to_response_code(version)),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    char buf[RFC3339_MAX_LENGTH];
    rfc3339_datetime_ut(buf, sizeof(buf), stream_parent_get_reconnection_ut(host->stream.snd.parents.current), 0, false);

    nd_log(NDLS_DAEMON, priority,
           "STREAM CONNECT '%s' [to %s]: %s - will retry in %d secs, at %s",
           rrdhost_hostname(host), s->remote_ip, error, delay, buf);

    return false;
}

bool stream_connect(struct sender_state *s, uint16_t default_port, time_t timeout) {
    worker_is_busy(WORKER_SENDER_CONNECTOR_JOB_CONNECTING);

    RRDHOST *host = s->host;

    // make sure the socket is closed
    nd_sock_close(&s->sock);

    s->hops = (int16_t)(rrdhost_ingestion_hops(s->host) + 1);

    // reset this to make sure we have its current value
    s->sock.verify_certificate = netdata_ssl_validate_certificate_sender;
    s->sock.ctx = netdata_ssl_streaming_sender_ctx;

    pulse_host_status(s->host, PULSE_HOST_STATUS_SND_PENDING, 0);
    if(!stream_parent_connect_to_one(
            &s->sock, host, default_port, timeout,
            s->remote_ip, sizeof(s->remote_ip) - 1,
            &host->stream.snd.parents.current)) {

        if(s->sock.error != ND_SOCK_ERR_NO_DESTINATION_AVAILABLE) {
            nd_log(NDLS_DAEMON, NDLP_WARNING, "can't connect to a parent, last error: %s",
                   ND_SOCK_ERROR_2str(s->sock.error));
        }

        nd_sock_close(&s->sock);
        return false;
    }

    // reset our capabilities to default
    s->capabilities = stream_our_capabilities(host, true);

    /* TODO: During the implementation of #7265 switch the set of variables to HOST_* and CONTAINER_* if the
             version negotiation resulted in a high enough version.
    */
    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_strcat(wb, "STREAM ");
    buffer_key_value_urlencode(wb, "key", string2str(host->stream.snd.api_key));
    buffer_key_value_urlencode(wb, "&hostname", rrdhost_hostname(host));
    buffer_key_value_urlencode(wb, "&registry_hostname", rrdhost_registry_hostname(host));
    buffer_key_value_urlencode(wb, "&machine_guid", host->machine_guid);
    buffer_sprintf(wb, "&update_every=%d", (int)nd_profile.update_every);
    buffer_key_value_urlencode(wb, "&os", rrdhost_os(host));
    buffer_key_value_urlencode(wb, "&timezone", rrdhost_timezone(host));
    buffer_key_value_urlencode(wb, "&abbrev_timezone", rrdhost_abbrev_timezone(host));
    buffer_sprintf(wb, "&utc_offset=%d", host->utc_offset);
    buffer_sprintf(wb, "&hops=%d", s->hops);
    buffer_sprintf(wb, "&ver=%u", s->capabilities);
    rrdhost_system_info_to_url_encode_stream(wb, host->system_info);
    buffer_key_value_urlencode(wb, "&NETDATA_PROTOCOL_VERSION", STREAMING_PROTOCOL_VERSION);
    buffer_strcat(wb, HTTP_1_1 HTTP_ENDL);
    buffer_sprintf(wb, "User-Agent: %s/%s" HTTP_ENDL, rrdhost_program_name(host), rrdhost_program_version(host));
    buffer_strcat(wb, "Accept: */*" HTTP_HDR_END);

    if (s->parent_using_h2o && stream_connect_upgrade_prelude(host, s)) {
        worker_is_busy(WORKER_SENDER_CONNECTOR_JOB_DISCONNECT_CANT_UPGRADE_CONNECTION);
        nd_sock_close(&s->sock);
        stream_parent_set_host_connect_failure_reason(host, STREAM_HANDSHAKE_SND_DISCONNECT_HTTP_UPGRADE_FAILED, 60);
        return false;
    }

    ssize_t len = (ssize_t)buffer_strlen(wb);
    ssize_t bytes = nd_sock_send_timeout(&s->sock, (void *)buffer_tostring(wb), len, 0, timeout);
    if(bytes <= 0) { // timeout is 0
        ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_I64(NDF_RESPONSE_CODE, stream_handshake_error_to_response_code(STREAM_HANDSHAKE_CONNECT_SEND_TIMEOUT)),
            ND_LOG_FIELD_END(),
        };
        ND_LOG_STACK_PUSH(lgs);

        worker_is_busy(WORKER_SENDER_CONNECTOR_JOB_DISCONNECT_TIMEOUT);
        nd_sock_close(&s->sock);

        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM CONNECT '%s' [to %s]: failed to send HTTP header to remote netdata.",
               rrdhost_hostname(host), s->remote_ip);

        stream_parent_set_host_connect_failure_reason(host, STREAM_HANDSHAKE_CONNECT_SEND_TIMEOUT, 60);
        return false;
    }

    char response[4096];
    bytes = nd_sock_recv_timeout(&s->sock, response, sizeof(response) - 1, 0, timeout);
    if(bytes <= 0) { // timeout is 0
        nd_sock_close(&s->sock);

        ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_I64(NDF_RESPONSE_CODE, stream_handshake_error_to_response_code(STREAM_HANDSHAKE_CONNECT_RECEIVE_TIMEOUT)),
            ND_LOG_FIELD_END(),
        };
        ND_LOG_STACK_PUSH(lgs);

        worker_is_busy(WORKER_SENDER_CONNECTOR_JOB_DISCONNECT_TIMEOUT);

        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM CONNECT '%s' [to %s]: remote netdata does not respond.",
               rrdhost_hostname(host), s->remote_ip);

        stream_parent_set_host_connect_failure_reason(host, STREAM_HANDSHAKE_CONNECT_RECEIVE_TIMEOUT, 30);
        return false;
    }
    response[bytes] = '\0';

    if(!stream_connect_validate_first_response(host, s, response, bytes)) {
        nd_sock_close(&s->sock);
        return false;
    }

    stream_compression_initialize(s);

    log_sender_capabilities(s);

    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_I64(NDF_RESPONSE_CODE, HTTP_RESP_OK),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "STREAM CONNECT '%s' [to %s]: connected to parent...",
           rrdhost_hostname(host), s->remote_ip);

    return true;
}

#define MAX_CONNECTORS 1

struct connector {
    int8_t id;
    pid_t tid;
    ND_THREAD *thread;
    struct completion completion;

    Word_t idx;

    size_t nodes;

    struct {
        // the incoming queue of the connector thread
        // all other threads leave new senders here, to be connected to their parents
        SPINLOCK spinlock;
        SENDERS_JudyLSet senders;
    } queue;
};

static inline Word_t get_unique_idx(struct connector *cn, STRCNT_CMD cmd) {
    Word_t t = STRCNT_CMD_MAX - 1;
    Word_t reserved_bits = (sizeof(Word_t) * 8) - __builtin_clz(t);
    return (__atomic_add_fetch(&cn->idx, 1, __ATOMIC_RELAXED) << reserved_bits) | cmd;
}

static struct {
    int id;
    struct connector connectors[MAX_CONNECTORS];
} connector_globals = { 0 };

bool stream_connector_is_signaled_to_stop(struct sender_state *s) {
    return __atomic_load_n(&s->exit.shutdown, __ATOMIC_RELAXED);
}

struct connector *stream_connector_get(struct sender_state *s) {
    stream_sender_lock(s);

    if(s->connector.id < 0 || s->connector.id >= MAX_CONNECTORS) {
        // assign this to the dispatcher with fewer nodes

        static SPINLOCK spinlock = SPINLOCK_INITIALIZER;
        spinlock_lock(&spinlock);
        int min_slot = 0;
        size_t min_nodes = __atomic_load_n(&connector_globals.connectors[0].nodes, __ATOMIC_RELAXED);
        for(int i = 1; i < MAX_CONNECTORS ;i++) {
            size_t nodes = __atomic_load_n(&connector_globals.connectors[i].nodes, __ATOMIC_RELAXED);
            if(nodes < min_nodes) {
                min_nodes = nodes;
                min_slot = i;
            }
        }
        __atomic_add_fetch(&connector_globals.connectors[min_slot].nodes, 1, __ATOMIC_RELAXED);
        s->connector.id = min_slot;
        spinlock_unlock(&spinlock);
    }

    struct connector *sc = &connector_globals.connectors[s->connector.id];
    stream_sender_unlock(s);

    return sc;
}

void stream_connector_requeue(struct sender_state *s, STRCNT_CMD cmd) {
    struct connector *sc = stream_connector_get(s);

    switch(cmd) {
        case STRCNT_CMD_CONNECT:
            nd_log(NDLS_DAEMON, NDLP_DEBUG,
                "STREAM CONNECT '%s' [to parent]: adding host in connector queue...",
                rrdhost_hostname(s->host));

            pulse_host_status(s->host, PULSE_HOST_STATUS_SND_PENDING, 0);
            break;

        case STRCNT_CMD_REMOVE:
            break;

        default:
            fatal("STREAM CONNECT '%s': invalid cmd %d", rrdhost_hostname(s->host), cmd);
    }

    spinlock_lock(&sc->queue.spinlock);
    SENDERS_SET(&sc->queue.senders, get_unique_idx(sc, cmd), s);
    spinlock_unlock(&sc->queue.spinlock);

    // signal the connector to catch the job
    completion_mark_complete_a_job(&sc->completion);
}

void stream_connector_add(struct sender_state *s) {
    // multiple threads may come here - only one should be able to pass through
    stream_sender_lock(s);
    if(!rrdhost_has_stream_sender_enabled(s->host) || !s->host->stream.snd.destination || !s->host->stream.snd.api_key) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "STREAM CONNECT '%s' [disabled]: host has streaming disabled - not sending data to a parent.",
               rrdhost_hostname(s->host));
        stream_sender_unlock(s);
        return;
    }
    if(rrdhost_flag_check(s->host, RRDHOST_FLAG_STREAM_SENDER_ADDED)) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG, "STREAM CONNECT '%s' [duplicate]: host has already added to sender - ignoring request.",
               rrdhost_hostname(s->host));
        stream_sender_unlock(s);
        return;
    }
    rrdhost_flag_set(s->host, RRDHOST_FLAG_STREAM_SENDER_ADDED);
    rrdhost_flag_clear(s->host, RRDHOST_FLAG_STREAM_SENDER_CONNECTED | RRDHOST_FLAG_STREAM_SENDER_READY_4_METRICS);
    stream_sender_unlock(s);

    nd_sock_close(&s->sock);
    s->parent_using_h2o = stream_send.parents.h2o;

    stream_parents_host_reset(s->host, 0);

    // do not call this with any locks held
    stream_connector_requeue(s, STRCNT_CMD_CONNECT);
}

static void stream_connector_remove(struct sender_state *s) {
    struct connector *sc = stream_connector_get(s);
    __atomic_sub_fetch(&sc->nodes, 1, __ATOMIC_RELAXED);

    nd_log(NDLS_DAEMON, NDLP_NOTICE,
           "STREAM CNT '%s' [to %s]: streaming connector removed host: %s (signaled to stop)",
           rrdhost_hostname(s->host), s->remote_ip, stream_handshake_error_to_string(s->exit.reason));

    STREAM_HANDSHAKE reason = s->exit.reason ? s->exit.reason : STREAM_HANDSHAKE_DISCONNECT_SIGNALED_TO_STOP;
    pulse_host_status(s->host, PULSE_HOST_STATUS_SND_OFFLINE, reason);
    stream_sender_remove(s, reason);
}

static void stream_connector_thread(void *ptr) {
    struct connector *sc = ptr;
    sc->tid = gettid_cached();

    nd_thread_can_run_sql(false);

    worker_register("STREAMCNT");
    worker_register_job_name(WORKER_SENDER_CONNECTOR_JOB_CONNECTING, "connect");
    worker_register_job_name(WORKER_SENDER_CONNECTOR_JOB_CONNECTED, "connected");
    worker_register_job_name(WORKER_SENDER_CONNECTOR_JOB_REMOVED, "removed");
    worker_register_job_name(WORKER_SENDER_CONNECTOR_JOB_DISCONNECT_BAD_HANDSHAKE, "bad handshake");
    worker_register_job_name(WORKER_SENDER_CONNECTOR_JOB_DISCONNECT_TIMEOUT, "timeout");
    worker_register_job_name(WORKER_SENDER_CONNECTOR_JOB_DISCONNECT_CANT_UPGRADE_CONNECTION, "cant upgrade");

    worker_register_job_custom_metric(WORKER_SENDER_CONNECTOR_JOB_QUEUED_NODES, "queued nodes", "nodes", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(WORKER_SENDER_CONNECTOR_JOB_CONNECTED_NODES, "connected nodes", "nodes", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(WORKER_SENDER_CONNECTOR_JOB_FAILED_NODES, "failed nodes", "nodes", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(WORKER_SENDER_CONNECTOR_JOB_CANCELLED_NODES, "cancelled nodes", "nodes", WORKER_METRIC_ABSOLUTE);

    unsigned job_id = 0;
    size_t exiting = 0;
    while(exiting <= 5) {
        worker_is_idle();
        job_id = completion_wait_for_a_job_with_timeout(&sc->completion, job_id, exiting ? 250 : 1000);
        size_t nodes = 0, connected_nodes = 0, failed_nodes = 0, cancelled_nodes = 0;

        if(!service_running(SERVICE_STREAMING_CONNECTOR))
            exiting++;

        spinlock_lock(&sc->queue.spinlock);
        Word_t idx = 0;
        for(struct sender_state *s = SENDERS_FIRST(&sc->queue.senders, &idx);
             s;
             s = SENDERS_NEXT(&sc->queue.senders, &idx)) {
            nodes++;

            ND_LOG_STACK lgs[] = {
                ND_LOG_FIELD_STR(NDF_NIDL_NODE, s->host->hostname),
                ND_LOG_FIELD_UUID(NDF_MESSAGE_ID, &streaming_to_parent_msgid),
                ND_LOG_FIELD_END(),
            };
            ND_LOG_STACK_PUSH(lgs);

            if(stream_connector_is_signaled_to_stop(s)) {
                cancelled_nodes++;
                SENDERS_DEL(&sc->queue.senders, idx);
                spinlock_unlock(&sc->queue.spinlock);

                // do not have the connector lock when calling these
                stream_sender_on_disconnect(s);
                stream_connector_remove(s);

                spinlock_lock(&sc->queue.spinlock);
                continue;
            }

            STRCNT_CMD cmd = idx & (STRCNT_CMD_CONNECT| STRCNT_CMD_REMOVE);
            if(unlikely(exiting))
                cmd = STRCNT_CMD_REMOVE;

            switch(cmd) {
                case STRCNT_CMD_CONNECT:
                    spinlock_unlock(&sc->queue.spinlock);
                    worker_is_busy(WORKER_SENDER_CONNECTOR_JOB_CONNECTING);

                    // do not have the connector lock when calling these
                    bool move_to_sender =
                        stream_connect(s, stream_send.parents.default_port, stream_send.parents.timeout_s);

                    spinlock_lock(&sc->queue.spinlock);

                    if (move_to_sender) {
                        connected_nodes++;

                        worker_is_busy(WORKER_SENDER_CONNECTOR_JOB_CONNECTED);
                        SENDERS_DEL(&sc->queue.senders, idx);
                        spinlock_unlock(&sc->queue.spinlock);

                        // do not have the connector lock when calling these
                        stream_sender_on_connect(s);
                        stream_sender_add_to_queue(s);

                        spinlock_lock(&sc->queue.spinlock);
                    }
                    else
                        failed_nodes++;

                    break;

                case STRCNT_CMD_REMOVE:
                    worker_is_busy(WORKER_SENDER_CONNECTOR_JOB_REMOVED);
                    SENDERS_DEL(&sc->queue.senders, idx);
                    spinlock_unlock(&sc->queue.spinlock);

                    // do not have the connector lock when calling these
                    stream_sender_on_disconnect(s);
                    stream_sender_remove(s, s->exit.reason);

                    spinlock_lock(&sc->queue.spinlock);
                    break;

                default:
                    fatal("STREAM CONNECT '%s': invalid cmd %d", rrdhost_hostname(s->host), cmd);
            }

            worker_is_idle();
        }
        spinlock_unlock(&sc->queue.spinlock);

        worker_set_metric(WORKER_SENDER_CONNECTOR_JOB_QUEUED_NODES, (NETDATA_DOUBLE)nodes);
        worker_set_metric(WORKER_SENDER_CONNECTOR_JOB_CONNECTED_NODES, (NETDATA_DOUBLE)connected_nodes);
        worker_set_metric(WORKER_SENDER_CONNECTOR_JOB_FAILED_NODES, (NETDATA_DOUBLE)failed_nodes);
        worker_set_metric(WORKER_SENDER_CONNECTOR_JOB_CANCELLED_NODES, (NETDATA_DOUBLE)cancelled_nodes);
    }
}

void stream_connector_remove_host(RRDHOST *host) {
    if(!host || !host->sender) return;

    struct connector *sc = stream_connector_get(host->sender);

    spinlock_lock(&sc->queue.spinlock);
    Word_t idx = 0;
    for(struct sender_state *s = SENDERS_FIRST(&sc->queue.senders, &idx);
         s;
         s = SENDERS_NEXT(&sc->queue.senders, &idx)) {

        if(s != host->sender)
            continue;

        ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_STR(NDF_NIDL_NODE, s->host->hostname),
            ND_LOG_FIELD_UUID(NDF_MESSAGE_ID, &streaming_to_parent_msgid),
            ND_LOG_FIELD_END(),
        };
        ND_LOG_STACK_PUSH(lgs);

        SENDERS_DEL(&sc->queue.senders, idx);
        spinlock_unlock(&sc->queue.spinlock);

        // do not have the connector lock when calling these
        stream_sender_on_disconnect(s);
        stream_sender_remove(s, s->exit.reason);

        spinlock_lock(&sc->queue.spinlock);
        break;
    }

    spinlock_unlock(&sc->queue.spinlock);
}

bool stream_connector_init(struct sender_state *s) {
    static SPINLOCK spinlock = SPINLOCK_INITIALIZER;
    if(!s) return false;

    spinlock_lock(&spinlock);

    struct connector *sc = stream_connector_get(s);

    if(!sc->thread) {
        sc->id = (int8_t)(sc - connector_globals.connectors); // find the slot number
        if(&connector_globals.connectors[sc->id] != sc)
            fatal("STREAM CONNECT '%s': connector ID and slot do not match!", rrdhost_hostname(s->host));

        spinlock_init(&sc->queue.spinlock);
        completion_init(&sc->completion);

        char tag[NETDATA_THREAD_TAG_MAX + 1];
        snprintfz(tag, NETDATA_THREAD_TAG_MAX, THREAD_TAG_STREAM_SENDER "-CN" "[%d]",
            sc->id);

        sc->thread = nd_thread_create(tag, NETDATA_THREAD_OPTION_DEFAULT, stream_connector_thread, sc);
        if (!sc->thread)
            nd_log_daemon(NDLP_ERR,
                          "STREAM CONNECT '%s': failed to create new thread for client.",
                          rrdhost_hostname(s->host));
    }

    spinlock_unlock(&spinlock);

    return sc->thread != NULL;
}

void stream_connector_cancel_threads(void) {
    for(int id = 0; id < MAX_CONNECTORS ; id++)
        nd_thread_signal_cancel(connector_globals.connectors[id].thread);
}

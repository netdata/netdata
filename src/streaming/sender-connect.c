// SPDX-License-Identifier: GPL-3.0-or-later

#include "sender-internals.h"

void rrdpush_sender_thread_close_socket(struct sender_state *s) {
    rrdhost_flag_clear(s->host, RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED | RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS);

    netdata_ssl_close(&s->ssl);

    if(s->rrdpush_sender_socket != -1) {
        close(s->rrdpush_sender_socket);
        s->rrdpush_sender_socket = -1;
    }

    // do not flush the circular buffer here
    // this function is called sometimes with the sender lock, sometimes without the lock
}

void rrdpush_encode_variable(stream_encoded_t *se, RRDHOST *host) {
    se->os_name = (host->system_info->host_os_name)?url_encode(host->system_info->host_os_name):strdupz("");
    se->os_id = (host->system_info->host_os_id)?url_encode(host->system_info->host_os_id):strdupz("");
    se->os_version = (host->system_info->host_os_version)?url_encode(host->system_info->host_os_version):strdupz("");
    se->kernel_name = (host->system_info->kernel_name)?url_encode(host->system_info->kernel_name):strdupz("");
    se->kernel_version = (host->system_info->kernel_version)?url_encode(host->system_info->kernel_version):strdupz("");
}

void rrdpush_clean_encoded(stream_encoded_t *se) {
    if (se->os_name) {
        freez(se->os_name);
        se->os_name = NULL;
    }

    if (se->os_id) {
        freez(se->os_id);
        se->os_id = NULL;
    }

    if (se->os_version) {
        freez(se->os_version);
        se->os_version = NULL;
    }

    if (se->kernel_name) {
        freez(se->kernel_name);
        se->kernel_name = NULL;
    }

    if (se->kernel_version) {
        freez(se->kernel_version);
        se->kernel_version = NULL;
    }
}

struct {
    const char *response;
    const char *status;
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
        .status = RRDPUSH_STATUS_CONNECTED,
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
        .status = RRDPUSH_STATUS_CONNECTED,
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
        .status = RRDPUSH_STATUS_CONNECTED,
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
        .status = RRDPUSH_STATUS_LOCALHOST,
        .version = STREAM_HANDSHAKE_ERROR_LOCALHOST,
        .dynamic = false,
        .error = "remote server rejected this stream, the host we are trying to stream is its localhost",
        .worker_job_id = WORKER_SENDER_JOB_DISCONNECT_BAD_HANDSHAKE,
        .postpone_reconnect_seconds = 60 * 60, // the IP may change, try it every hour
        .priority = NDLP_DEBUG,
    },
    {
        .response = START_STREAMING_ERROR_ALREADY_STREAMING,
        .length = sizeof(START_STREAMING_ERROR_ALREADY_STREAMING) - 1,
        .status = RRDPUSH_STATUS_ALREADY_CONNECTED,
        .version = STREAM_HANDSHAKE_ERROR_ALREADY_CONNECTED,
        .dynamic = false,
        .error = "remote server rejected this stream, the host we are trying to stream is already streamed to it",
        .worker_job_id = WORKER_SENDER_JOB_DISCONNECT_BAD_HANDSHAKE,
        .postpone_reconnect_seconds = 2 * 60, // 2 minutes
        .priority = NDLP_DEBUG,
    },
    {
        .response = START_STREAMING_ERROR_NOT_PERMITTED,
        .length = sizeof(START_STREAMING_ERROR_NOT_PERMITTED) - 1,
        .status = RRDPUSH_STATUS_PERMISSION_DENIED,
        .version = STREAM_HANDSHAKE_ERROR_DENIED,
        .dynamic = false,
        .error = "remote server denied access, probably we don't have the right API key?",
        .worker_job_id = WORKER_SENDER_JOB_DISCONNECT_BAD_HANDSHAKE,
        .postpone_reconnect_seconds = 1 * 60, // 1 minute
        .priority = NDLP_ERR,
    },
    {
        .response = START_STREAMING_ERROR_BUSY_TRY_LATER,
        .length = sizeof(START_STREAMING_ERROR_BUSY_TRY_LATER) - 1,
        .status = RRDPUSH_STATUS_RATE_LIMIT,
        .version = STREAM_HANDSHAKE_BUSY_TRY_LATER,
        .dynamic = false,
        .error = "remote server is currently busy, we should try later",
        .worker_job_id = WORKER_SENDER_JOB_DISCONNECT_BAD_HANDSHAKE,
        .postpone_reconnect_seconds = 2 * 60, // 2 minutes
        .priority = NDLP_NOTICE,
    },
    {
        .response = START_STREAMING_ERROR_INTERNAL_ERROR,
        .length = sizeof(START_STREAMING_ERROR_INTERNAL_ERROR) - 1,
        .status = RRDPUSH_STATUS_INTERNAL_SERVER_ERROR,
        .version = STREAM_HANDSHAKE_INTERNAL_ERROR,
        .dynamic = false,
        .error = "remote server is encountered an internal error, we should try later",
        .worker_job_id = WORKER_SENDER_JOB_DISCONNECT_BAD_HANDSHAKE,
        .postpone_reconnect_seconds = 5 * 60, // 5 minutes
        .priority = NDLP_CRIT,
    },
    {
        .response = START_STREAMING_ERROR_INITIALIZATION,
        .length = sizeof(START_STREAMING_ERROR_INITIALIZATION) - 1,
        .status = RRDPUSH_STATUS_INITIALIZATION_IN_PROGRESS,
        .version = STREAM_HANDSHAKE_INITIALIZATION,
        .dynamic = false,
        .error = "remote server is initializing, we should try later",
        .worker_job_id = WORKER_SENDER_JOB_DISCONNECT_BAD_HANDSHAKE,
        .postpone_reconnect_seconds = 2 * 60, // 2 minute
        .priority = NDLP_NOTICE,
    },

    // terminator
    {
        .response = NULL,
        .length = 0,
        .status = RRDPUSH_STATUS_BAD_HANDSHAKE,
        .version = STREAM_HANDSHAKE_ERROR_BAD_HANDSHAKE,
        .dynamic = false,
        .error = "remote node response is not understood, is it Netdata?",
        .worker_job_id = WORKER_SENDER_JOB_DISCONNECT_BAD_HANDSHAKE,
        .postpone_reconnect_seconds = 1 * 60, // 1 minute
        .priority = NDLP_ERR,
    }
};

static inline bool rrdpush_sender_validate_response(RRDHOST *host, struct sender_state *s, char *http, size_t http_length) {
    int32_t version = STREAM_HANDSHAKE_ERROR_BAD_HANDSHAKE;

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
        rrdpush_destination_set_reconnect_delay(host->destination, version, now_realtime_sec() + s->reconnect_delay);
        s->capabilities = convert_stream_version_to_capabilities(version, host, true);
        return true;
    }

    ND_LOG_FIELD_PRIORITY priority = stream_responses[i].priority;
    const char *error = stream_responses[i].error;
    const char *status = stream_responses[i].status;
    int worker_job_id = stream_responses[i].worker_job_id;
    int delay = stream_responses[i].postpone_reconnect_seconds;

    worker_is_busy(worker_job_id);
    rrdpush_sender_thread_close_socket(s);
    rrdpush_destination_set_reconnect_delay(host->destination, version, now_realtime_sec() + delay);

    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_TXT(NDF_RESPONSE_CODE, status),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    char buf[RFC3339_MAX_LENGTH];
    rfc3339_datetime_ut(buf, sizeof(buf), rrdpush_destination_get_reconnection_t(host->destination) * USEC_PER_SEC, 0, false);

    nd_log(NDLS_DAEMON, priority,
           "STREAM %s [send to %s]: %s - will retry in %d secs, at %s",
           rrdhost_hostname(host), s->connected_to, error, delay, buf);

    return false;
}

unsigned char alpn_proto_list[] = {
    18, 'n', 'e', 't', 'd', 'a', 't', 'a', '_', 's', 't', 'r', 'e', 'a', 'm', '/', '2', '.', '0',
    8, 'h', 't', 't', 'p', '/', '1', '.', '1'
};

#define CONN_UPGRADE_VAL "upgrade"

static bool rrdpush_sender_connect_ssl(struct sender_state *s) {
    RRDHOST *host = s->host;
    bool ssl_required = host ? rrdpush_destination_is_ssl(host->destination) : false;

    netdata_ssl_close(&host->sender->ssl);

    if(!ssl_required)
        return true;

    if (netdata_ssl_open_ext(&host->sender->ssl, netdata_ssl_streaming_sender_ctx, s->rrdpush_sender_socket, alpn_proto_list, sizeof(alpn_proto_list))) {
        if(!netdata_ssl_connect(&host->sender->ssl)) {
            // couldn't connect

            ND_LOG_STACK lgs[] = {
                ND_LOG_FIELD_TXT(NDF_RESPONSE_CODE, RRDPUSH_STATUS_SSL_ERROR),
                ND_LOG_FIELD_END(),
            };
            ND_LOG_STACK_PUSH(lgs);

            worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_SSL_ERROR);
            rrdpush_sender_thread_close_socket(s);
            rrdpush_destination_set_reconnect_delay(host->destination, STREAM_HANDSHAKE_ERROR_SSL_ERROR, now_realtime_sec() + 5 * 60);
            return false;
        }

        if (netdata_ssl_validate_certificate_sender &&
            security_test_certificate(host->sender->ssl.conn)) {
            // certificate is not valid

            ND_LOG_STACK lgs[] = {
                ND_LOG_FIELD_TXT(NDF_RESPONSE_CODE, RRDPUSH_STATUS_INVALID_SSL_CERTIFICATE),
                ND_LOG_FIELD_END(),
            };
            ND_LOG_STACK_PUSH(lgs);

            worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_SSL_ERROR);
            netdata_log_error("SSL: closing the stream connection, because the server SSL certificate is not valid.");
            rrdpush_sender_thread_close_socket(s);
            rrdpush_destination_set_reconnect_delay(host->destination, STREAM_HANDSHAKE_ERROR_INVALID_CERTIFICATE, now_realtime_sec() + 5 * 60);
            return false;
        }

        return true;
    }

    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_TXT(NDF_RESPONSE_CODE, RRDPUSH_STATUS_CANT_ESTABLISH_SSL_CONNECTION),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    netdata_log_error("SSL: failed to establish connection.");
    return false;
}

static int rrdpush_http_upgrade_prelude(RRDHOST *host, struct sender_state *s) {

    char http[HTTP_HEADER_SIZE + 1];
    snprintfz(http, HTTP_HEADER_SIZE,
              "GET " NETDATA_STREAM_URL HTTP_1_1 HTTP_ENDL
              "Upgrade: " NETDATA_STREAM_PROTO_NAME HTTP_ENDL
              "Connection: Upgrade"
              HTTP_HDR_END);

    ssize_t bytes = send_timeout(
        &host->sender->ssl,
        s->rrdpush_sender_socket,
        http,
        strlen(http),
        0,
        1000);

    bytes = recv_timeout(
        &host->sender->ssl,
        s->rrdpush_sender_socket,
        http,
        HTTP_HEADER_SIZE,
        0,
        1000);

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
    if (strncmp(hdr, CONN_UPGRADE_VAL, strlen(CONN_UPGRADE_VAL))) {
        error_report("Expected \"connection: " CONN_UPGRADE_VAL "\"");
        goto err_cleanup;
    }

    hdr = get_http_header_by_name(&ctx, "upgrade");
    if (!hdr) {
        error_report("Missing \"upgrade\" header in reply");
        goto err_cleanup;
    }
    if (strncmp(hdr, NETDATA_STREAM_PROTO_NAME, strlen(NETDATA_STREAM_PROTO_NAME))) {
        error_report("Expected \"upgrade: " NETDATA_STREAM_PROTO_NAME "\"");
        goto err_cleanup;
    }

    netdata_log_debug(D_STREAM, "Stream sender upgrade to \"" NETDATA_STREAM_PROTO_NAME "\" successful");
    rbuf_free(buf);
    http_parse_ctx_destroy(&ctx);
    return 0;
err_cleanup:
    rbuf_free(buf);
    http_parse_ctx_destroy(&ctx);
    return 1;
}

static bool sender_send_connection_request(RRDHOST *host, int default_port, int timeout, struct sender_state *s) {

    struct timeval tv = {
        .tv_sec = timeout,
        .tv_usec = 0
    };

    // make sure the socket is closed
    rrdpush_sender_thread_close_socket(s);

    s->rrdpush_sender_socket = connect_to_one_of_destinations(
        host
        , default_port
        , &tv
        , &s->reconnects_counter
        , s->connected_to
        , sizeof(s->connected_to)-1
        , &host->destination
    );

    if(unlikely(s->rrdpush_sender_socket == -1)) {
        // netdata_log_error("STREAM %s [send to %s]: could not connect to parent node at this time.", rrdhost_hostname(host), host->rrdpush_send_destination);
        return false;
    }

    // netdata_log_info("STREAM %s [send to %s]: initializing communication...", rrdhost_hostname(host), s->connected_to);

    // reset our capabilities to default
    s->capabilities = stream_our_capabilities(host, true);

    /* TODO: During the implementation of #7265 switch the set of variables to HOST_* and CONTAINER_* if the
             version negotiation resulted in a high enough version.
    */
    stream_encoded_t se;
    rrdpush_encode_variable(&se, host);

    host->sender->hops = host->system_info->hops + 1;

    char http[HTTP_HEADER_SIZE + 1];
    int eol = snprintfz(http, HTTP_HEADER_SIZE,
                        "STREAM "
                        "key=%s"
                        "&hostname=%s"
                        "&registry_hostname=%s"
                        "&machine_guid=%s"
                        "&update_every=%d"
                        "&os=%s"
                        "&timezone=%s"
                        "&abbrev_timezone=%s"
                        "&utc_offset=%d"
                        "&hops=%d"
                        "&ml_capable=%d"
                        "&ml_enabled=%d"
                        "&mc_version=%d"
                        "&ver=%u"
                        "&NETDATA_INSTANCE_CLOUD_TYPE=%s"
                        "&NETDATA_INSTANCE_CLOUD_INSTANCE_TYPE=%s"
                        "&NETDATA_INSTANCE_CLOUD_INSTANCE_REGION=%s"
                        "&NETDATA_SYSTEM_OS_NAME=%s"
                        "&NETDATA_SYSTEM_OS_ID=%s"
                        "&NETDATA_SYSTEM_OS_ID_LIKE=%s"
                        "&NETDATA_SYSTEM_OS_VERSION=%s"
                        "&NETDATA_SYSTEM_OS_VERSION_ID=%s"
                        "&NETDATA_SYSTEM_OS_DETECTION=%s"
                        "&NETDATA_HOST_IS_K8S_NODE=%s"
                        "&NETDATA_SYSTEM_KERNEL_NAME=%s"
                        "&NETDATA_SYSTEM_KERNEL_VERSION=%s"
                        "&NETDATA_SYSTEM_ARCHITECTURE=%s"
                        "&NETDATA_SYSTEM_VIRTUALIZATION=%s"
                        "&NETDATA_SYSTEM_VIRT_DETECTION=%s"
                        "&NETDATA_SYSTEM_CONTAINER=%s"
                        "&NETDATA_SYSTEM_CONTAINER_DETECTION=%s"
                        "&NETDATA_CONTAINER_OS_NAME=%s"
                        "&NETDATA_CONTAINER_OS_ID=%s"
                        "&NETDATA_CONTAINER_OS_ID_LIKE=%s"
                        "&NETDATA_CONTAINER_OS_VERSION=%s"
                        "&NETDATA_CONTAINER_OS_VERSION_ID=%s"
                        "&NETDATA_CONTAINER_OS_DETECTION=%s"
                        "&NETDATA_SYSTEM_CPU_LOGICAL_CPU_COUNT=%s"
                        "&NETDATA_SYSTEM_CPU_FREQ=%s"
                        "&NETDATA_SYSTEM_TOTAL_RAM=%s"
                        "&NETDATA_SYSTEM_TOTAL_DISK_SIZE=%s"
                        "&NETDATA_PROTOCOL_VERSION=%s"
                        HTTP_1_1 HTTP_ENDL
                        "User-Agent: %s/%s\r\n"
                        "Accept: */*\r\n\r\n"
                        , host->rrdpush.send.api_key
                        , rrdhost_hostname(host)
                            , rrdhost_registry_hostname(host)
                            , host->machine_guid
                        , default_rrd_update_every
                        , rrdhost_os(host)
                            , rrdhost_timezone(host)
                            , rrdhost_abbrev_timezone(host)
                            , host->utc_offset
                        , host->sender->hops
                        , host->system_info->ml_capable
                        , host->system_info->ml_enabled
                        , host->system_info->mc_version
                        , s->capabilities
                        , (host->system_info->cloud_provider_type) ? host->system_info->cloud_provider_type : ""
                        , (host->system_info->cloud_instance_type) ? host->system_info->cloud_instance_type : ""
                        , (host->system_info->cloud_instance_region) ? host->system_info->cloud_instance_region : ""
                        , se.os_name
                        , se.os_id
                        , (host->system_info->host_os_id_like) ? host->system_info->host_os_id_like : ""
                        , se.os_version
                        , (host->system_info->host_os_version_id) ? host->system_info->host_os_version_id : ""
                        , (host->system_info->host_os_detection) ? host->system_info->host_os_detection : ""
                        , (host->system_info->is_k8s_node) ? host->system_info->is_k8s_node : ""
                        , se.kernel_name
                        , se.kernel_version
                        , (host->system_info->architecture) ? host->system_info->architecture : ""
                        , (host->system_info->virtualization) ? host->system_info->virtualization : ""
                        , (host->system_info->virt_detection) ? host->system_info->virt_detection : ""
                        , (host->system_info->container) ? host->system_info->container : ""
                        , (host->system_info->container_detection) ? host->system_info->container_detection : ""
                        , (host->system_info->container_os_name) ? host->system_info->container_os_name : ""
                        , (host->system_info->container_os_id) ? host->system_info->container_os_id : ""
                        , (host->system_info->container_os_id_like) ? host->system_info->container_os_id_like : ""
                        , (host->system_info->container_os_version) ? host->system_info->container_os_version : ""
                        , (host->system_info->container_os_version_id) ? host->system_info->container_os_version_id : ""
                        , (host->system_info->container_os_detection) ? host->system_info->container_os_detection : ""
                        , (host->system_info->host_cores) ? host->system_info->host_cores : ""
                        , (host->system_info->host_cpu_freq) ? host->system_info->host_cpu_freq : ""
                        , (host->system_info->host_ram_total) ? host->system_info->host_ram_total : ""
                        , (host->system_info->host_disk_space) ? host->system_info->host_disk_space : ""
                        , STREAMING_PROTOCOL_VERSION
                        , rrdhost_program_name(host)
                            , rrdhost_program_version(host)
    );
    http[eol] = 0x00;
    rrdpush_clean_encoded(&se);

    if(!rrdpush_sender_connect_ssl(s))
        return false;

    if (s->parent_using_h2o && rrdpush_http_upgrade_prelude(host, s)) {
        ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_TXT(NDF_RESPONSE_CODE, RRDPUSH_STATUS_CANT_UPGRADE_CONNECTION),
            ND_LOG_FIELD_END(),
        };
        ND_LOG_STACK_PUSH(lgs);

        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_CANT_UPGRADE_CONNECTION);
        rrdpush_sender_thread_close_socket(s);
        rrdpush_destination_set_reconnect_delay(host->destination, STREAM_HANDSHAKE_ERROR_HTTP_UPGRADE, now_realtime_sec() + 1 * 60);
        return false;
    }

    ssize_t len = (ssize_t)strlen(http);
    ssize_t bytes = send_timeout(
        &host->sender->ssl,
        s->rrdpush_sender_socket,
        http,
        len,
        0,
        timeout);

    if(bytes <= 0) { // timeout is 0
        ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_TXT(NDF_RESPONSE_CODE, RRDPUSH_STATUS_TIMEOUT),
            ND_LOG_FIELD_END(),
        };
        ND_LOG_STACK_PUSH(lgs);

        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_TIMEOUT);
        rrdpush_sender_thread_close_socket(s);

        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM %s [send to %s]: failed to send HTTP header to remote netdata.",
               rrdhost_hostname(host), s->connected_to);

        rrdpush_destination_set_reconnect_delay(host->destination, STREAM_HANDSHAKE_ERROR_SEND_TIMEOUT, now_realtime_sec() + 1 * 60);
        return false;
    }

    bytes = recv_timeout(
        &host->sender->ssl,
        s->rrdpush_sender_socket,
        http,
        HTTP_HEADER_SIZE,
        0,
        timeout);

    if(bytes <= 0) { // timeout is 0
        ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_TXT(NDF_RESPONSE_CODE, RRDPUSH_STATUS_TIMEOUT),
            ND_LOG_FIELD_END(),
        };
        ND_LOG_STACK_PUSH(lgs);

        worker_is_busy(WORKER_SENDER_JOB_DISCONNECT_TIMEOUT);
        rrdpush_sender_thread_close_socket(s);

        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM %s [send to %s]: remote netdata does not respond.",
               rrdhost_hostname(host), s->connected_to);

        rrdpush_destination_set_reconnect_delay(host->destination, STREAM_HANDSHAKE_ERROR_RECEIVE_TIMEOUT, now_realtime_sec() + 30);
        return false;
    }

    if(sock_setnonblock(s->rrdpush_sender_socket) < 0)
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "STREAM %s [send to %s]: cannot set non-blocking mode for socket.",
               rrdhost_hostname(host), s->connected_to);
    sock_setcloexec(s->rrdpush_sender_socket);

    if(sock_enlarge_out(s->rrdpush_sender_socket) < 0)
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "STREAM %s [send to %s]: cannot enlarge the socket buffer.",
               rrdhost_hostname(host), s->connected_to);

    http[bytes] = '\0';
    if(!rrdpush_sender_validate_response(host, s, http, bytes))
        return false;

    rrdpush_compression_initialize(s);

    log_sender_capabilities(s);

    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_TXT(NDF_RESPONSE_CODE, RRDPUSH_STATUS_CONNECTED),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "STREAM %s: connected to %s...",
           rrdhost_hostname(host), s->connected_to);

    return true;
}

bool attempt_to_connect(struct sender_state *state) {
    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_UUID(NDF_MESSAGE_ID, &streaming_to_parent_msgid),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    state->send_attempts = 0;

    // reset the bytes we have sent for this session
    state->sent_bytes_on_this_connection = 0;
    memset(state->sent_bytes_on_this_connection_per_type, 0, sizeof(state->sent_bytes_on_this_connection_per_type));

    if(sender_send_connection_request(state->host, state->default_port, state->timeout, state)) {
        // reset the buffer, to properly send charts and metrics
        rrdpush_sender_on_connect(state->host);

        // send from the beginning
        state->begin = 0;

        // make sure the next reconnection will be immediate
        state->not_connected_loops = 0;

        // let the data collection threads know we are ready
        rrdhost_flag_set(state->host, RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED);

        rrdpush_sender_after_connect(state->host);

        return true;
    }

    // we couldn't connect

    // increase the failed connections counter
    state->not_connected_loops++;

    // slow re-connection on repeating errors
    usec_t now_ut = now_monotonic_usec();
    usec_t end_ut = now_ut + USEC_PER_SEC * state->reconnect_delay;
    while(now_ut < end_ut) {
        if(nd_thread_signaled_to_cancel())
            return false;

        sleep_usec(100 * USEC_PER_MS); // seconds
        now_ut = now_monotonic_usec();
    }

    return false;
}

bool rrdpush_sender_connect(struct sender_state *s) {
    worker_is_busy(WORKER_SENDER_JOB_CONNECT);

    time_t now_s = now_monotonic_sec();
    rrdpush_sender_cbuffer_recreate_timed(s, now_s, false, true);
    rrdpush_sender_execute_commands_cleanup(s);

    rrdhost_flag_clear(s->host, RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS);
    s->flags &= ~SENDER_FLAG_OVERFLOW;
    s->read_len = 0;
    s->buffer->read = 0;
    s->buffer->write = 0;

    if(!attempt_to_connect(s))
        return false;

    if(rrdhost_sender_should_exit(s))
        return false;

    s->last_traffic_seen_t = now_monotonic_sec();
    stream_path_send_to_parent(s->host);
    rrdpush_sender_send_claimed_id(s->host);
    rrdpush_send_host_labels(s->host);
    rrdpush_send_global_functions(s->host);
    s->replication.oldest_request_after_t = 0;

    rrdhost_flag_set(s->host, RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS);

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "STREAM %s [send to %s]: enabling metrics streaming...",
           rrdhost_hostname(s->host), s->connected_to);

    return true;
}

// Either the receiver lost the connection or the host is being destroyed.
// The sender mutex guards thread creation, any spurious data is wiped on reconnection.
void rrdpush_sender_thread_stop(RRDHOST *host, STREAM_HANDSHAKE reason, bool wait) {
    if (!host->sender)
        return;

    sender_lock(host->sender);

    if(rrdhost_flag_check(host, RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN)) {

        host->sender->exit.shutdown = true;
        host->sender->exit.reason = reason;

        // signal it to cancel
        nd_thread_signal_cancel(host->rrdpush_sender_thread);
    }

    sender_unlock(host->sender);

    if(wait) {
        sender_lock(host->sender);
        while(host->sender->tid) {
            sender_unlock(host->sender);
            sleep_usec(10 * USEC_PER_MS);
            sender_lock(host->sender);
        }
        sender_unlock(host->sender);
    }
}

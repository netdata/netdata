// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream-sender-internals.h"
#include "stream-replication-sender.h"

#define TIME_TO_CONSIDER_PARENTS_SIMILAR 120

struct stream_parent {
    STRING *destination;                        // the parent destination
    bool ssl;                                   // the parent uses SSL

    bool banned_permanently;                    // when the parent is the origin of this host
    bool banned_for_this_session;               // when the parent is before us in the streaming path
    bool banned_temporarily_erroneous;          // when the parent is blocked by another node we host
    STREAM_HANDSHAKE reason;
    uint32_t attempts;                          // how many times we have tried to connect to this parent
    usec_t since_ut;                            // the last time we tried to connect to it
    usec_t postpone_until_ut;                   // based on the reason, a randomized time to wait for reconnection

    struct {
        ND_UUID host_id;                        // the machine_guid of the agent
        int status;                             // the response code of the stream_info call
        uint32_t nonce;                         // a random 32-bit number
        size_t nodes;                           // how many nodes the parent has
        size_t receivers;                       // how many receivers the parent has

        // these are from RRDHOST_STATUS and can only be used when status == 200
        RRDHOST_DB_STATUS db_status;
        RRDHOST_DB_LIVENESS db_liveness;
        RRDHOST_INGEST_TYPE ingest_type;
        RRDHOST_INGEST_STATUS ingest_status;
        time_t db_first_time_s;                 // the oldest timestamp for us in the parent's database
        time_t db_last_time_s;                  // the latest timestamp for us in the parent's database
    } remote;

    struct {
        size_t batch;                           // the batch priority (>= 1, 0 == excluded)
        size_t order;                           // the final order of the parent (>= 1, 0 == excluded)
        bool random;                            // this batch has more than 1 parents, so we flipped coins to select order
        bool info;                              // we go stream info from the parent
        bool skipped;                           // we skipped this parent for some reason
    } selection;

    STREAM_PARENT *prev;
    STREAM_PARENT *next;
};

// --------------------------------------------------------------------------------------------------------------------
// block unresponsive parents for some time, to allow speeding up the connection of the rest

struct blocked_parent {
    STRING *destination;
    usec_t until;
};

DEFINE_JUDYL_TYPED(BLOCKED_PARENTS, struct blocked_parent *);
static BLOCKED_PARENTS_JudyLSet blocked_parents_set = { 0 };
static RW_SPINLOCK blocked_parents_spinlock = RW_SPINLOCK_INITIALIZER;

static void block_parent_for_all_nodes(STREAM_PARENT *d, time_t duration_s) {
    rw_spinlock_write_lock(&blocked_parents_spinlock);

    struct blocked_parent *p = BLOCKED_PARENTS_GET(&blocked_parents_set, (Word_t)d->destination);
    if(!p) {
        p = callocz(1, sizeof(*p));
        p->destination = string_dup(d->destination);
        BLOCKED_PARENTS_SET(&blocked_parents_set, (Word_t)p->destination, p);
    }
    p->until = now_monotonic_usec() + duration_s * USEC_PER_SEC;

    rw_spinlock_write_unlock(&blocked_parents_spinlock);
}

static bool is_a_blocked_parent(STREAM_PARENT *d) {
    rw_spinlock_read_lock(&blocked_parents_spinlock);

    struct blocked_parent *p = BLOCKED_PARENTS_GET(&blocked_parents_set, (Word_t)d->destination);
    bool ret = p && p->until > now_monotonic_usec();

    rw_spinlock_read_unlock(&blocked_parents_spinlock);
    return ret;
}

// --------------------------------------------------------------------------------------------------------------------

STREAM_HANDSHAKE stream_parent_get_disconnect_reason(STREAM_PARENT *d) {
    if(!d) return STREAM_HANDSHAKE_PARENT_INTERNAL_ERROR;
    return d->reason;
}

void stream_parent_set_host_disconnect_reason(RRDHOST *host, STREAM_HANDSHAKE reason, time_t since) {
    host->stream.snd.status.reason = reason;
    struct stream_parent *d = host->stream.snd.parents.current;
    if(!d) return;
    d->since_ut = since * USEC_PER_SEC;
    d->reason = reason;
}

static inline usec_t randomize_wait_ut(time_t min, time_t max) {
    min = (min < SENDER_MIN_RECONNECT_DELAY ? SENDER_MIN_RECONNECT_DELAY : min);
    if(max < min) max = min;

    usec_t min_ut = min * USEC_PER_SEC;
    usec_t max_ut = max * USEC_PER_SEC;
    usec_t wait_ut = min_ut + os_random(max_ut - min_ut);
    return now_realtime_usec() + wait_ut;
}

void stream_parents_host_reset(RRDHOST *host, STREAM_HANDSHAKE reason) {
    usec_t until_ut = randomize_wait_ut(stream_send.parents.reconnect_delay_s / 2, stream_send.parents.reconnect_delay_s + 5);
    rw_spinlock_write_lock(&host->stream.snd.parents.spinlock);
    for (STREAM_PARENT *d = host->stream.snd.parents.all; d; d = d->next) {
        d->postpone_until_ut = until_ut;
        d->banned_for_this_session = false;
        d->reason = reason;
    }
    rw_spinlock_write_unlock(&host->stream.snd.parents.spinlock);
}

static void stream_parent_set_reconnect_delay(STREAM_PARENT *d, STREAM_HANDSHAKE reason, time_t secs) {
    if(!d) return;
    d->reason = reason;
    d->postpone_until_ut = randomize_wait_ut(5, secs);
}

void stream_parent_set_host_reconnect_delay(RRDHOST *host, STREAM_HANDSHAKE reason, time_t secs) {
    stream_parent_set_reconnect_delay(host->stream.snd.parents.current, reason, secs);
}

static void stream_parent_set_connect_failure_reason(RRDHOST *host, STREAM_PARENT *d, STREAM_HANDSHAKE reason, time_t secs) {
    host->stream.snd.status.reason = reason;
    pulse_host_status(host, PULSE_HOST_STATUS_SND_NO_DST_FAILED, reason);
    pulse_sender_connection_failed(d ? string2str(d->destination) : NULL, reason);
    stream_parent_set_reconnect_delay(d, reason, secs);
}

void stream_parent_set_host_connect_failure_reason(RRDHOST *host, STREAM_HANDSHAKE reason, time_t secs) {
    stream_parent_set_connect_failure_reason(host, host->stream.snd.parents.current, reason, secs);
}

usec_t stream_parent_get_reconnection_ut(STREAM_PARENT *d) {
    return d ? d->postpone_until_ut : 0;
}

bool stream_parent_is_ssl(STREAM_PARENT *d) {
    return d ? d->ssl : false;
}

usec_t stream_parent_handshake_error_to_json(BUFFER *wb, RRDHOST *host) {
    usec_t last_attempt = 0;
    rw_spinlock_read_lock(&host->stream.snd.parents.spinlock);
    for(STREAM_PARENT *d = host->stream.snd.parents.all; d ; d = d->next) {
        if(d->since_ut > last_attempt)
            last_attempt = d->since_ut;

        buffer_json_add_array_item_string(wb, stream_handshake_error_to_string(d->reason));
    }
    rw_spinlock_read_unlock(&host->stream.snd.parents.spinlock);
    return last_attempt;
}

void rrdhost_stream_parents_to_json(BUFFER *wb, RRDHOST_STATUS *s) {
    char buf[1024];

    rw_spinlock_read_lock(&s->host->stream.snd.parents.spinlock);

    usec_t now_ut = now_realtime_usec();
    STREAM_PARENT *d;
    for (d = s->host->stream.snd.parents.all; d; d = d->next) {
        buffer_json_add_array_item_object(wb);
        buffer_json_member_add_uint64(wb, "attempts", d->attempts + 1);
        {
            if (d->ssl) {
                snprintfz(buf, sizeof(buf) - 1, "%s:SSL", string2str(d->destination));
                buffer_json_member_add_string(wb, "destination", buf);
            }
            else
                buffer_json_member_add_string(wb, "destination", string2str(d->destination));

            buffer_json_member_add_datetime_rfc3339(wb, "since", d->since_ut, false);
            buffer_json_member_add_duration_ut(wb, "age", d->since_ut < now_ut ? (int64_t)(now_ut - d->since_ut) : 0);

            if(!d->banned_for_this_session && !d->banned_permanently && !d->banned_temporarily_erroneous) {
                buffer_json_member_add_string(wb, "last_handshake", stream_handshake_error_to_string(d->reason));

                if (d->postpone_until_ut > now_ut) {
                    buffer_json_member_add_datetime_rfc3339(wb, "next_check", d->postpone_until_ut, false);
                    buffer_json_member_add_duration_ut(wb, "next_in", (int64_t)(d->postpone_until_ut - now_ut));
                }

                if(d->selection.batch) {
                    buffer_json_member_add_uint64(wb, "batch", d->selection.batch);
                    buffer_json_member_add_uint64(wb, "order", d->selection.order);
                    buffer_json_member_add_boolean(wb, "random", d->selection.random);
                }

                buffer_json_member_add_boolean(wb, "info", d->selection.info);
                buffer_json_member_add_boolean(wb, "skipped", d->selection.skipped);
            }
            else {
                if(d->banned_permanently)
                    buffer_json_member_add_string(wb, "ban", "it is the localhost");
                else if(d->banned_for_this_session)
                    buffer_json_member_add_string(wb, "ban", "it is our parent");
                else if(d->banned_temporarily_erroneous)
                    buffer_json_member_add_string(wb, "ban", "it is erroneous");
            }
        }
        buffer_json_object_close(wb); // each candidate
    }

    rw_spinlock_read_unlock(&s->host->stream.snd.parents.spinlock);
}

void rrdhost_stream_parent_ssl_init(struct sender_state *s) {
    static SPINLOCK sp = SPINLOCK_INITIALIZER;
    spinlock_lock(&sp);

    if(netdata_ssl_streaming_sender_ctx || !s->host) {
        spinlock_unlock(&sp);
        goto cleanup;
    }

    rw_spinlock_read_lock(&s->host->stream.snd.parents.spinlock);

    for(STREAM_PARENT *d = s->host->stream.snd.parents.all; d ; d = d->next) {
        if (d->ssl) {
            // we need to initialize SSL

            netdata_ssl_initialize_ctx(NETDATA_SSL_STREAMING_SENDER_CTX);

            ssl_security_location_for_context(
                netdata_ssl_streaming_sender_ctx,
                string2str(stream_send.parents.ssl_ca_file),
                string2str(stream_send.parents.ssl_ca_path));

            // stop the loop
            break;
        }
    }

    rw_spinlock_read_unlock(&s->host->stream.snd.parents.spinlock);
    spinlock_unlock(&sp);

cleanup:
    s->sock.ctx = netdata_ssl_streaming_sender_ctx;
    s->sock.verify_certificate = netdata_ssl_validate_certificate_sender;
}

static void stream_parent_nd_sock_error_to_reason(STREAM_PARENT *d, ND_SOCK *sock) {
    switch (sock->error) {
        case ND_SOCK_ERR_CONNECTION_REFUSED:
            d->reason = STREAM_HANDSHAKE_SP_CONNECTION_REFUSED;
            d->postpone_until_ut = randomize_wait_ut(30, 60);
            block_parent_for_all_nodes(d, 30);
            break;

        case ND_SOCK_ERR_CANNOT_RESOLVE_HOSTNAME:
            d->reason = STREAM_HANDSHAKE_SP_CANT_RESOLVE_HOSTNAME;
            d->postpone_until_ut = randomize_wait_ut(30, 60);
            block_parent_for_all_nodes(d, 30);
            break;

        case ND_SOCK_ERR_NO_HOST_IN_DEFINITION:
            d->reason = STREAM_HANDSHAKE_SP_NO_HOST_IN_DESTINATION;
            d->banned_for_this_session = true;
            d->postpone_until_ut = randomize_wait_ut(30, 60);
            block_parent_for_all_nodes(d, 30);
            break;

        case ND_SOCK_ERR_TIMEOUT:
            d->reason = STREAM_HANDSHAKE_SP_CONNECT_TIMEOUT;
            d->postpone_until_ut = randomize_wait_ut(300, d->remote.nodes < 10 ? 600 : 900);
            block_parent_for_all_nodes(d, 300);
            break;

        case ND_SOCK_ERR_SSL_INVALID_CERTIFICATE:
            d->reason = STREAM_HANDSHAKE_CONNECT_INVALID_CERTIFICATE;
            d->postpone_until_ut = randomize_wait_ut(300, 600);
            block_parent_for_all_nodes(d, 300);
            break;

        case ND_SOCK_ERR_SSL_CANT_ESTABLISH_SSL_CONNECTION:
        case ND_SOCK_ERR_SSL_FAILED_TO_OPEN:
            d->reason = STREAM_HANDSHAKE_CONNECT_SSL_ERROR;
            d->postpone_until_ut = randomize_wait_ut(60, 180);
            block_parent_for_all_nodes(d, 60);
            break;

        default:
        case ND_SOCK_ERR_POLL_ERROR:
        case ND_SOCK_ERR_FAILED_TO_CREATE_SOCKET:
        case ND_SOCK_ERR_UNKNOWN_ERROR:
            d->reason = STREAM_HANDSHAKE_PARENT_INTERNAL_ERROR;
            d->postpone_until_ut = randomize_wait_ut(30, 60);
            break;

        case ND_SOCK_ERR_THREAD_CANCELLED:
        case ND_SOCK_ERR_NO_DESTINATION_AVAILABLE:
            d->reason = STREAM_HANDSHAKE_PARENT_INTERNAL_ERROR;
            d->postpone_until_ut = randomize_wait_ut(30, 60);
            break;
    }
}

int stream_info_to_json_v1(BUFFER *wb, const char *machine_guid) {
    pulse_parent_stream_info_received_request();

    buffer_reset(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    RRDHOST_STATUS status = { 0 };
    int ret = HTTP_RESP_OK;
    RRDHOST *host;
    if(!machine_guid || !*machine_guid || !(host = rrdhost_find_by_guid(machine_guid)))
        ret = HTTP_RESP_NOT_FOUND;
    else
        rrdhost_status(host, now_realtime_sec(), &status, RRDHOST_STATUS_BASIC);

    buffer_json_member_add_uint64(wb, "version", 1);
    buffer_json_member_add_uint64(wb, "status", ret);
    buffer_json_member_add_uuid(wb, "host_id", localhost->host_id.uuid);
    buffer_json_member_add_uint64(wb, "nodes", dictionary_entries(rrdhost_root_index));
    buffer_json_member_add_uint64(wb, "receivers", stream_receivers_currently_connected());
    buffer_json_member_add_uint64(wb, "nonce", os_random32());

    if(ret == HTTP_RESP_OK) {
        if((status.ingest.status == RRDHOST_INGEST_STATUS_ARCHIVED || status.ingest.status == RRDHOST_INGEST_STATUS_OFFLINE) &&
            !stream_control_children_should_be_accepted())
           status.ingest.status = RRDHOST_INGEST_STATUS_INITIALIZING;

        buffer_json_member_add_string(wb, "db_status", rrdhost_db_status_to_string(status.db.status));
        buffer_json_member_add_string(wb, "db_liveness", rrdhost_db_liveness_to_string(status.db.liveness));
        buffer_json_member_add_string(wb, "ingest_type", rrdhost_ingest_type_to_string(status.ingest.type));
        buffer_json_member_add_string(wb, "ingest_status", rrdhost_ingest_status_to_string(status.ingest.status));
        buffer_json_member_add_uint64(wb, "first_time_s", status.db.first_time_s);
        buffer_json_member_add_uint64(wb, "last_time_s", status.db.last_time_s);
    }

    buffer_json_finalize(wb);
    return ret;
}

static bool stream_info_json_parse_v1(struct json_object *jobj, const char *path, STREAM_PARENT *d, BUFFER *error) {
    uint32_t version = 0; (void)version;
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "version", version, error, true);

    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "status", d->remote.status, error, true);
    JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "host_id", d->remote.host_id.uuid, error, true);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "nodes", d->remote.nodes, error, true);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "receivers", d->remote.receivers, error, true);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "nonce", d->remote.nonce, error, true);

    if(d->remote.status == HTTP_RESP_OK) {
        JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "first_time_s", d->remote.db_first_time_s, error, true);
        JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "last_time_s", d->remote.db_last_time_s, error, true);
        JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, path, "db_status", RRDHOST_DB_STATUS_2id, d->remote.db_status, error, true);
        JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, path, "db_liveness", RRDHOST_DB_LIVENESS_2id, d->remote.db_liveness, error, true);
        JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, path, "ingest_type", RRDHOST_INGEST_TYPE_2id, d->remote.ingest_type, error, true);
        JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, path, "ingest_status", RRDHOST_INGEST_STATUS_2id, d->remote.ingest_status, error, true);
        return true;
    }

    buffer_sprintf(error, "status reported (%d) is not OK (%d)", d->remote.status, HTTP_RESP_OK);

    d->remote.db_first_time_s = 0;
    d->remote.db_last_time_s = 0;
    d->remote.db_status = 0;
    d->remote.db_liveness = 0;
    d->remote.ingest_type = 0;
    d->remote.ingest_status = 0;

    return false;
}

static bool stream_info_fetch(STREAM_PARENT *d, const char *uuid, int default_port, ND_SOCK *sender_sock, bool ssl, const char *hostname) {
    ND_LOG_STACK lgs[] = {
        ND_LOG_FIELD_STR(NDF_DST_IP, d->destination),
        ND_LOG_FIELD_I64(NDF_DST_PORT, default_port),
        ND_LOG_FIELD_TXT(NDF_REQUEST_METHOD, "GET"),
        ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    char buf[HTTP_HEADER_SIZE];
    CLEAN_ND_SOCK sock = ND_SOCK_INIT(sender_sock->ctx, sender_sock->verify_certificate);

    // Build HTTP request
    snprintf(buf, sizeof(buf),
             "GET /api/v3/stream_info?machine_guid=%s" HTTP_1_1 HTTP_ENDL
             "Host: %s" HTTP_ENDL
             "User-Agent: %s/%s" HTTP_ENDL
             "Accept: */*" HTTP_ENDL
             "Accept-Encoding: identity" HTTP_ENDL  // disable chunked encoding
             "TE: identity" HTTP_ENDL               // disable chunked encoding
             "Pragma: no-cache" HTTP_ENDL
             "Cache-Control: no-cache" HTTP_ENDL
             "Connection: close" HTTP_HDR_END,
             uuid,
             string2str(d->destination),
             rrdhost_program_name(localhost),
             rrdhost_program_version(localhost));

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "STREAM PARENTS '%s': fetching stream info from '%s'...",
           hostname, string2str(d->destination));

    pulse_stream_info_sent_request();

    // Establish connection
    d->reason = STREAM_HANDSHAKE_SP_CONNECTING;
    if (!nd_sock_connect_to_this(&sock, string2str(d->destination), default_port, 5, ssl)) {
        d->selection.info = false;
        stream_parent_nd_sock_error_to_reason(d, &sock);
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "STREAM PARENTS '%s': failed to connect for stream info to '%s': %s",
               hostname, string2str(d->destination),
               ND_SOCK_ERROR_2str(sock.error));
        return false;
    }

    // Send HTTP request
    ssize_t sent = nd_sock_send_timeout(&sock, buf, strlen(buf), 0, 5);
    if (sent <= 0) {
        d->selection.info = false;
        stream_parent_nd_sock_error_to_reason(d, &sock);
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "STREAM PARENTS '%s': failed to send stream info request to '%s': %s",
               hostname, string2str(d->destination),
               ND_SOCK_ERROR_2str(sock.error));
        return false;
    }

    // Receive HTTP response
    size_t total_received = 0;
    size_t payload_received = 0;
    size_t content_length = 0;
    char *payload_start = NULL;

    while (!payload_received || content_length < payload_received) {
        size_t remaining = sizeof(buf) - total_received;

        if (remaining <= 1) {
            nd_log(NDLS_DAEMON, NDLP_WARNING,
                   "STREAM PARENTS '%s': stream info receive buffer is full while receiving response from '%s'",
                   hostname, string2str(d->destination));
            d->selection.info = false;
            d->reason = STREAM_HANDSHAKE_PARENT_INTERNAL_ERROR;
            return false;
        }

        ssize_t received = nd_sock_recv_timeout(&sock, buf + total_received, remaining - 1, 0, 5);
        if (received <= 0) {
            nd_log(NDLS_DAEMON, NDLP_WARNING,
                   "STREAM PARENTS '%s': socket receive error while querying stream info on '%s' "
                   "(total received %zu, payload received %zu, content length %zu): %s",
                   hostname, string2str(d->destination),
                   total_received, payload_received, content_length,
                   ND_SOCK_ERROR_2str(sock.error));

            d->selection.info = false;
            stream_parent_nd_sock_error_to_reason(d, &sock);
            return false;
        }

        total_received += received;
        buf[total_received] = '\0';

        if(!payload_start) {
            char *headers_end = strstr(buf, HTTP_HDR_END);
            if (!headers_end)
                // we have not received the whole header yet
                continue;

            payload_start = headers_end + sizeof(HTTP_HDR_END) - 1;
        }

        // the payload size so far
        payload_received = total_received - (payload_start - buf);

        if(!content_length) {
            char *content_length_ptr = strstr(buf, "Content-Length: ");
            if (!content_length_ptr) {
                nd_log(NDLS_DAEMON, NDLP_WARNING,
                       "STREAM PARENTS '%s': stream info response from '%s' does not have a Content-Length",
                       hostname, string2str(d->destination));

                d->selection.info = false;
                d->reason = STREAM_HANDSHAKE_PARENT_INTERNAL_ERROR;
                return false;
            }
            content_length = strtoul(content_length_ptr + strlen("Content-Length: "), NULL, 10);
            if (!content_length) {
                nd_log(NDLS_DAEMON, NDLP_WARNING,
                       "STREAM PARENTS '%s': stream info response from '%s' has invalid Content-Length",
                       hostname, string2str(d->destination));

                d->selection.info = false;
                d->reason = STREAM_HANDSHAKE_PARENT_INTERNAL_ERROR;
                return false;
            }
        }
    }

    // Parse HTTP response and extract JSON
    CLEAN_JSON_OBJECT *jobj = json_tokener_parse(payload_start);
    if (!jobj) {
        d->selection.info = false;
        d->reason = STREAM_HANDSHAKE_SP_NO_STREAM_INFO;
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "STREAM PARENTS '%s': failed to parse stream info response from '%s', JSON data: %s",
               hostname, string2str(d->destination), payload_start);
        return false;
    }

    CLEAN_BUFFER *error = buffer_create(0, NULL);

    if(!stream_info_json_parse_v1(jobj, "", d, error)) {
        d->selection.info = false;
        d->reason = STREAM_HANDSHAKE_SP_NO_STREAM_INFO;
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "STREAM PARENTS '%s': failed to extract fields from JSON stream info response from '%s': %s"
               " - JSON data: %s",
               hostname, string2str(d->destination),
               buffer_tostring(error),
               payload_start);
        return false;
    }

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "STREAM PARENTS '%s': received stream_info data from '%s': "
           "status: %d, nodes: %zu, receivers: %zu, first_time_s: %ld, last_time_s: %ld, "
           "db status: %s, db liveness: %s, ingest type: %s, ingest status: %s",
           hostname, string2str(d->destination),
           d->remote.status, d->remote.nodes, d->remote.receivers,
           d->remote.db_first_time_s, d->remote.db_last_time_s,
           RRDHOST_DB_STATUS_2str(d->remote.db_status),
           RRDHOST_DB_LIVENESS_2str(d->remote.db_liveness),
           RRDHOST_INGEST_TYPE_2str(d->remote.ingest_type),
           RRDHOST_INGEST_STATUS_2str(d->remote.ingest_status));

    d->selection.info = true;
    d->reason = STREAM_HANDSHAKE_NEVER;
    return true;
}

static int compare_last_time(const void *a, const void *b) {
    STREAM_PARENT *parent_a = *(STREAM_PARENT **)a;
    STREAM_PARENT *parent_b = *(STREAM_PARENT **)b;

    if (parent_a->remote.db_last_time_s < parent_b->remote.db_last_time_s) return 1;
    else if (parent_a->remote.db_last_time_s > parent_b->remote.db_last_time_s) return -1;
    else {
        if(parent_a->since_ut < parent_b->since_ut) return -1;
        else if(parent_a->since_ut > parent_b->since_ut) return 1;
        else {
            if(parent_a->attempts < parent_b->attempts) return -1;
            else if(parent_a->attempts > parent_b->attempts) return 1;
            else return 0;
        }
    }
}

bool stream_parent_connect_to_one_unsafe(
    ND_SOCK *sender_sock,
    RRDHOST *host,
    int default_port,
    time_t timeout,
    char *connected_to,
    size_t connected_to_size,
    STREAM_PARENT **destination)
{
    sender_sock->error = ND_SOCK_ERR_NO_DESTINATION_AVAILABLE;

    // count the parents
    size_t size = 0;
    for (STREAM_PARENT *d = host->stream.snd.parents.all; d; d = d->next) {
        d->selection.order = 0;
        d->selection.batch = 0;
        d->selection.random = false;
        d->selection.info = false;
        d->selection.skipped = true;
        size++;
    }

    // do we have any parents?
    if(!size) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG, "STREAM PARENTS '%s': no parents configured", rrdhost_hostname(host));
        return false;
    }

    STREAM_PARENT *array[size];
    usec_t now_ut = now_realtime_usec();

    // fetch stream info for all of them and put them in the array
    size_t count = 0, skipped_but_useful = 0, skipped_not_useful = 0, potential = 0;
    for (STREAM_PARENT *d = host->stream.snd.parents.all; d && count < size ; d = d->next) {
        if (nd_thread_signaled_to_cancel()) {
            sender_sock->error = ND_SOCK_ERR_THREAD_CANCELLED;
            return false;
        }

        // make sure they all have a random number
        // this is taken from the parent, but if the stream_info call fails,
        // we generate a random number for every parent here
        d->remote.nonce = os_random32();
        d->banned_temporarily_erroneous = is_a_blocked_parent(d);

        if (d->banned_permanently || d->banned_for_this_session)
            continue;

        if (d->banned_temporarily_erroneous) {
            potential++;
            host->stream.snd.status.reason = d->reason;
            continue;
        }

        if (d->postpone_until_ut > now_ut) {
            skipped_but_useful++;
            potential++;
            host->stream.snd.status.reason = d->reason;
            nd_log(NDLS_DAEMON, NDLP_DEBUG,
                   "STREAM PARENTS '%s': skipping useful parent '%s': POSTPONED FOR %ld SECS MORE: %s",
                   rrdhost_hostname(host),
                   string2str(d->destination),
                   (time_t)((d->postpone_until_ut - now_ut) / USEC_PER_SEC),
                   stream_handshake_error_to_string(d->reason));
            continue;
        }

        if(stream_info_fetch(d, host->machine_guid, default_port,
                              sender_sock, stream_parent_is_ssl(d), rrdhost_hostname(host))) {
            switch(d->remote.ingest_type) {
                case RRDHOST_INGEST_TYPE_VIRTUAL:
                case RRDHOST_INGEST_TYPE_LOCALHOST:
                    d->reason = STREAM_HANDSHAKE_PARENT_IS_LOCALHOST;
                    d->since_ut = now_ut;
                    d->postpone_until_ut = randomize_wait_ut(3600, 7200);
                    d->banned_permanently = true;
                    skipped_not_useful++;

                    if(rrdhost_is_host_in_stream_path_before_us(host, d->remote.host_id, 1)) {
                        // we passed hops == 1, to make sure this succeeds only when the parent
                        // is the origin child of this node
                        nd_log(NDLS_DAEMON, NDLP_INFO,
                               "STREAM PARENTS '%s': destination '%s' is banned permanently because it is the origin server",
                               rrdhost_hostname(host), string2str(d->destination));
                    }
                    else {
                        nd_log(NDLS_DAEMON, NDLP_WARNING,
                               "STREAM PARENTS '%s': destination '%s' is banned permanently because it is the origin server, "
                               "but it is not in the stream path before us!",
                               rrdhost_hostname(host), string2str(d->destination));
                    }
                    continue;

                default:
                case RRDHOST_INGEST_TYPE_CHILD:
                case RRDHOST_INGEST_TYPE_ARCHIVED:
                    break;
            }

            switch(d->remote.ingest_status) {
                case RRDHOST_INGEST_STATUS_INITIALIZING:
                    d->reason = STREAM_HANDSHAKE_PARENT_IS_INITIALIZING;
                    d->since_ut = now_ut;
                    d->postpone_until_ut = randomize_wait_ut(30, 60);
                    pulse_sender_stream_info_failed(string2str(d->destination), d->reason);
                    skipped_but_useful++;
                    potential++;
                    host->stream.snd.status.reason = d->reason;
                    nd_log(NDLS_DAEMON, NDLP_DEBUG,
                           "STREAM PARENTS '%s': skipping useful parent '%s': %s",
                           rrdhost_hostname(host), string2str(d->destination),
                           stream_handshake_error_to_string(d->reason));
                    continue;

                case RRDHOST_INGEST_STATUS_REPLICATING:
                case RRDHOST_INGEST_STATUS_ONLINE:
                    if(rrdhost_is_host_in_stream_path_before_us(host, d->remote.host_id, host->sender->hops)) {
                        d->reason = STREAM_HANDSHAKE_PARENT_NODE_ALREADY_CONNECTED;
                        d->since_ut = now_ut;
                        d->postpone_until_ut = randomize_wait_ut(3600, 7200);
                        d->banned_for_this_session = true;
                        skipped_not_useful++;
                        nd_log(NDLS_DAEMON, NDLP_INFO,
                               "STREAM PARENTS '%s': destination '%s' is banned for this session, because it is in our path before us.",
                               rrdhost_hostname(host), string2str(d->destination));
                        pulse_sender_stream_info_failed(string2str(d->destination), d->reason);
                        continue;
                    }
//                    else {
//                        skip = true;
//                        if(!netdata_conf_is_parent()) {
//                            nd_log(NDLS_DAEMON, NDLP_INFO,
//                                   "STREAM PARENTS '%s': destination '%s' reports I am already connected.",
//                                   rrdhost_hostname(host), string2str(d->destination));
//                        }
//                    }
                    break;

                default:
                case RRDHOST_INGEST_STATUS_OFFLINE:
                    break;
            }
        }
        else
            pulse_sender_stream_info_failed(string2str(d->destination), d->reason);

        d->selection.skipped = false;
        d->selection.batch = count + 1;
        d->selection.order = count + 1;
        array[count++] = d;
    }

    // can we use any parent?
    if(!count) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "STREAM PARENTS '%s': no parents available (%zu skipped but useful, %zu skipped not useful, %zu potential)",
               rrdhost_hostname(host), skipped_but_useful, skipped_not_useful, potential);

        if(!potential) {
            if(host->stream.snd.status.reason != STREAM_HANDSHAKE_SP_NO_DESTINATION) {
                host->stream.snd.status.reason = STREAM_HANDSHAKE_SP_NO_DESTINATION;
                pulse_sender_connection_failed(NULL, host->stream.snd.status.reason);
            }
            pulse_host_status(host, PULSE_HOST_STATUS_SND_NO_DST, 0);
        }

        return false;
    }

    // order the parents in the array the way we want to connect
    if(count > 1) {
        qsort(array, count, sizeof(STREAM_PARENT *), compare_last_time);

        size_t base = 0, batch = 0;
        while (base < count) {
            // find how many have similar db_last_time_s;
            size_t similar = 1;
            if(!array[base]->remote.nonce) array[base]->remote.nonce = os_random32();
            time_t tB = array[base]->remote.db_last_time_s;
            for (size_t i = base + 1; i < count; i++) {
                time_t tN = array[i]->remote.db_last_time_s;
                if ((tN > tB && tN - tB <= TIME_TO_CONSIDER_PARENTS_SIMILAR) ||
                               (tB - tN <= TIME_TO_CONSIDER_PARENTS_SIMILAR))
                    similar++;
                else
                    break;
            }

            // if we have only 1 similar, move on
            if (similar == 1) {
                nd_log(NDLS_DAEMON, NDLP_DEBUG,
                       "STREAM PARENTS '%s': reordering keeps parent No %zu, '%s'",
                       rrdhost_hostname(host), base, string2str(array[base]->destination));
                array[base]->selection.order = base + 1;
                array[base]->selection.batch = batch + 1;
                array[base]->selection.random = false;
                base++;
                batch++;
                continue;
            }
            else {
                // reorder the parents who have similar db_last_time

                while (similar > 1) {
                    size_t chosen = base;
                    for(size_t i = base + 1 ; i < base + similar ;i++) {
                        uint32_t i_nonce = array[i]->remote.nonce | os_random32();
                        uint32_t chosen_nonce = array[chosen]->remote.nonce | os_random32();
                        if(i_nonce > chosen_nonce) chosen = i;
                    }

                    if (chosen != base)
                        SWAP(array[base], array[chosen]);

                    nd_log(NDLS_DAEMON, NDLP_DEBUG,
                           "STREAM PARENTS '%s': random reordering of %zu similar parents (slots %zu to %zu), No %zu is '%s'",
                           rrdhost_hostname(host),
                           similar, base, base + similar,
                           base, string2str(array[base]->destination));

                    array[base]->selection.order = base + 1;
                    array[base]->selection.batch = batch + 1;
                    array[base]->selection.random = true;
                    base++;
                    similar--;
                }

                // the last one of the similar
                array[base]->selection.order = base + 1;
                array[base]->selection.batch = batch + 1;
                array[base]->selection.random = true;
                base++;
                batch++;
            }
        }
    }
    else {
        array[0]->selection.order = 1;
        array[0]->selection.batch = 1;
        array[0]->selection.random = false;

        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "STREAM PARENTS '%s': only 1 parent is available: '%s'",
               rrdhost_hostname(host), string2str(array[0]->destination));
    }

    // now the parents are sorted based on preference of connection
    for(size_t i = 0; i < count ;i++) {
        STREAM_PARENT *d = array[i];

        if(d->postpone_until_ut > now_ut)
            continue;

        if(nd_thread_signaled_to_cancel()) {
            sender_sock->error = ND_SOCK_ERR_THREAD_CANCELLED;
            host->stream.snd.status.reason = STREAM_HANDSHAKE_DISCONNECT_SIGNALED_TO_STOP;
            pulse_host_status(host, PULSE_HOST_STATUS_SND_OFFLINE, host->stream.snd.status.reason);
            return false;
        }

        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "STREAM PARENTS '%s': connecting to '%s' (default port: %d, parent %zu of %zu)...",
               rrdhost_hostname(host), string2str(d->destination), default_port,
               i + 1, count);

        ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_STR(NDF_DST_IP, d->destination),
            ND_LOG_FIELD_I64(NDF_DST_PORT, default_port),
            ND_LOG_FIELD_END(),
        };
        ND_LOG_STACK_PUSH(lgs);

        d->since_ut = now_ut;
        d->attempts++;
        pulse_host_status(host, PULSE_HOST_STATUS_SND_CONNECTING, 0);
        if (nd_sock_connect_to_this(sender_sock, string2str(d->destination),
                                    default_port, timeout, stream_parent_is_ssl(d))) {

            if (connected_to && connected_to_size)
                strncpyz(connected_to, string2str(d->destination), connected_to_size);

            *destination = d;

            // move the current item to the end of the list
            // without this, this destination will break the loop again and again
            // not advancing the destinations to find one that may work
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(host->stream.snd.parents.all, d, prev, next);
            DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(host->stream.snd.parents.all, d, prev, next);

            nd_log(NDLS_DAEMON, NDLP_DEBUG,
                   "STREAM PARENTS '%s': connected to '%s' (default port: %d, fd %d)...",
                   rrdhost_hostname(host), string2str(d->destination), default_port,
                   sender_sock->fd);

            sender_sock->error = ND_SOCK_ERR_NONE;
            host->stream.snd.status.reason = STREAM_HANDSHAKE_SP_CONNECTED;
            pulse_host_status(host, PULSE_HOST_STATUS_SND_CONNECTING, host->stream.snd.status.reason);
            return true;
        }
        else {
            stream_parent_nd_sock_error_to_reason(d, sender_sock);
            host->stream.snd.status.reason = d->reason;
            pulse_sender_connection_failed(string2str(d->destination), d->reason);
            pulse_host_status(host, PULSE_HOST_STATUS_SND_CONNECTING, host->stream.snd.status.reason);
            nd_log(NDLS_DAEMON, NDLP_DEBUG,
                   "STREAM PARENTS '%s': stream connection to '%s' failed (default port: %d): %s",
                   rrdhost_hostname(host),
                   string2str(d->destination), default_port,
                   ND_SOCK_ERROR_2str(sender_sock->error));
        }
    }

    pulse_host_status(host, PULSE_HOST_STATUS_SND_OFFLINE, 0);
    return false;
}

bool stream_parent_connect_to_one(
    ND_SOCK *sender_sock,
    RRDHOST *host,
    int default_port,
    time_t timeout,
    char *connected_to,
    size_t connected_to_size,
    STREAM_PARENT **destination) {

    rw_spinlock_read_lock(&host->stream.snd.parents.spinlock);
    bool rc = stream_parent_connect_to_one_unsafe(
        sender_sock, host, default_port, timeout, connected_to, connected_to_size, destination);
    rw_spinlock_read_unlock(&host->stream.snd.parents.spinlock);
    return rc;
}

// --------------------------------------------------------------------------------------------------------------------
// create stream parents linked list

struct stream_parent_init_tmp {
    RRDHOST *host;
    STREAM_PARENT *list;
    int count;
};

static bool stream_parent_add_one_unsafe(char *entry, void *data) {
    struct stream_parent_init_tmp *t = data;

    STREAM_PARENT *d = callocz(1, sizeof(STREAM_PARENT));
    char *colon_ssl = strstr(entry, ":SSL");
    if(colon_ssl) {
        *colon_ssl = '\0';
        d->ssl = true;
    }
    else
        d->ssl = false;

    d->destination = string_strdupz(entry);
    d->since_ut = now_realtime_usec();

    __atomic_add_fetch(&netdata_buffers_statistics.rrdhost_senders, sizeof(STREAM_PARENT), __ATOMIC_RELAXED);

    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(t->list, d, prev, next);

    t->count++;
    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "STREAM PARENTS '%s': added streaming destination No %d: '%s'",
           rrdhost_hostname(t->host), t->count, string2str(d->destination));

    return false; // we return false, so that we will get all defined destinations
}

void rrdhost_stream_parents_update_from_destination(RRDHOST *host) {
    rw_spinlock_write_lock(&host->stream.snd.parents.spinlock);
    rrdhost_stream_parents_free(host, true);

    if(host->stream.snd.destination) {
        struct stream_parent_init_tmp t = {
            .host = host,
            .list = NULL,
            .count = 0,
        };
        foreach_entry_in_connection_string(string2str(host->stream.snd.destination), stream_parent_add_one_unsafe, &t);
        host->stream.snd.parents.all = t.list;
    }

    rw_spinlock_write_unlock(&host->stream.snd.parents.spinlock);
}

void rrdhost_stream_parents_free(RRDHOST *host, bool having_write_lock) {
    if(!having_write_lock)
        rw_spinlock_write_lock(&host->stream.snd.parents.spinlock);

    while (host->stream.snd.parents.all) {
        STREAM_PARENT *tmp = host->stream.snd.parents.all;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(host->stream.snd.parents.all, tmp, prev, next);
        string_freez(tmp->destination);
        freez(tmp);
        __atomic_sub_fetch(&netdata_buffers_statistics.rrdhost_senders, sizeof(STREAM_PARENT), __ATOMIC_RELAXED);
    }

    host->stream.snd.parents.all = NULL;

    if(!having_write_lock)
        rw_spinlock_write_unlock(&host->stream.snd.parents.spinlock);
}

void rrdhost_stream_parents_init(RRDHOST *host) {
    rw_spinlock_init(&host->stream.snd.parents.spinlock);
}

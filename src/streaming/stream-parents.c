// SPDX-License-Identifier: GPL-3.0-or-later

#include "sender-internals.h"

#define TIME_TO_CONSIDER_PARENTS_SIMILAR 120

struct stream_parent {
    STRING *destination;
    bool ssl;
    bool banned_permanently;
    bool banned_for_this_session;
    STREAM_HANDSHAKE reason;
    uint32_t attempts;
    usec_t since_ut;
    usec_t postpone_until_ut;

    struct {
        ND_UUID host_id;
        int status;
        size_t nodes;
        size_t receivers;
        RRDHOST_DB_STATUS db_status;
        RRDHOST_DB_LIVENESS db_liveness;
        RRDHOST_INGEST_TYPE ingest_type;
        RRDHOST_INGEST_STATUS ingest_status;
        time_t db_first_time_s;
        time_t db_last_time_s;
        uint32_t nonce;
    } remote;

    struct {
        size_t batch;
        size_t order;
        bool random;
        bool info;
    } selection;

    STREAM_PARENT *prev;
    STREAM_PARENT *next;
};

STREAM_HANDSHAKE stream_parent_get_disconnect_reason(STREAM_PARENT *d) {
    if(!d) return STREAM_HANDSHAKE_INTERNAL_ERROR;
    return d->reason;
}

void stream_parent_set_disconnect_reason(STREAM_PARENT *d, STREAM_HANDSHAKE reason, time_t since) {
    if(!d) return;
    d->since_ut = since * USEC_PER_SEC;
    d->reason = reason;
}

static inline usec_t randomize_wait_ut(time_t secs) {
    usec_t delay_ut = (secs < SENDER_MIN_RECONNECT_DELAY ? SENDER_MIN_RECONNECT_DELAY : secs) * USEC_PER_SEC;
    usec_t wait_ut = (SENDER_MIN_RECONNECT_DELAY * USEC_PER_SEC) +
                     os_random(delay_ut - (SENDER_MIN_RECONNECT_DELAY * USEC_PER_SEC));
    return now_realtime_usec() + wait_ut;
}

void rrdhost_stream_parents_reset(RRDHOST *host, STREAM_HANDSHAKE reason) {
    usec_t until_ut = randomize_wait_ut(stream_send.parents.reconnect_delay_s);
    for (STREAM_PARENT *d = host->stream.snd.parents.all; d; d = d->next) {
        d->postpone_until_ut = until_ut;
        d->banned_for_this_session = false;
        d->reason = reason;
    }
}

void stream_parent_set_reconnect_delay(STREAM_PARENT *d, STREAM_HANDSHAKE reason, time_t secs) {
    if(!d) return;
    d->reason = reason;
    d->postpone_until_ut = randomize_wait_ut(secs);
}

usec_t stream_parent_get_reconnection_ut(STREAM_PARENT *d) {
    return d ? d->postpone_until_ut : 0;
}

bool stream_parent_is_ssl(STREAM_PARENT *d) {
    return d ? d->ssl : false;
}

usec_t stream_parent_handshake_error_to_json(BUFFER *wb, RRDHOST *host) {
    usec_t last_attempt = 0;
    for(STREAM_PARENT *d = host->stream.snd.parents.all; d ; d = d->next) {
        if(d->since_ut > last_attempt)
            last_attempt = d->since_ut;

        buffer_json_add_array_item_string(wb, stream_handshake_error_to_string(d->reason));
    }
    return last_attempt;
}

void rrdhost_stream_parents_to_json(BUFFER *wb, RRDHOST_STATUS *s) {
    char buf[1024];

    STREAM_PARENT *d;
    for (d = s->host->stream.snd.parents.all; d; d = d->next) {
        buffer_json_add_array_item_object(wb);
        buffer_json_member_add_uint64(wb, "attempts", d->attempts);
        {
            if (d->ssl) {
                snprintfz(buf, sizeof(buf) - 1, "%s:SSL", string2str(d->destination));
                buffer_json_member_add_string(wb, "destination", buf);
            }
            else
                buffer_json_member_add_string(wb, "destination", string2str(d->destination));

            buffer_json_member_add_uint64(wb, "since", d->since_ut / USEC_PER_SEC);
            buffer_json_member_add_uint64(wb, "age", d->since_ut ? s->now - (d->since_ut / USEC_PER_SEC) : 0);

            if(!d->banned_for_this_session && !d->banned_permanently) {
                buffer_json_member_add_string(wb, "last_handshake", stream_handshake_error_to_string(d->reason));

                if (d->postpone_until_ut > (usec_t)(s->now * USEC_PER_SEC)) {
                    buffer_json_member_add_datetime_rfc3339(wb, "next_check", d->postpone_until_ut, false);
                    buffer_json_member_add_duration_ut(wb, "next_in", (int64_t)d->postpone_until_ut - (int64_t)(s->now * USEC_PER_SEC));
                }

                buffer_json_member_add_uint64(wb, "batch", d->selection.batch);
                buffer_json_member_add_uint64(wb, "order", d->selection.order);
                buffer_json_member_add_boolean(wb, "random", d->selection.random);
                buffer_json_member_add_boolean(wb, "info", d->selection.info);
            }
            else {
                buffer_json_member_add_string(
                    wb, "ban", d->banned_permanently ? "permanent" : (d->banned_for_this_session ? "session" : "none"));
            }
        }
        buffer_json_object_close(wb); // each candidate
    }
}

void rrdhost_stream_parent_ssl_init(struct sender_state *s) {
    static SPINLOCK sp = NETDATA_SPINLOCK_INITIALIZER;
    spinlock_lock(&sp);

    if(netdata_ssl_streaming_sender_ctx || !s->host) {
        spinlock_unlock(&sp);
        goto cleanup;
    }

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

    spinlock_unlock(&sp);

cleanup:
    s->sock.ctx = netdata_ssl_streaming_sender_ctx;
    s->sock.verify_certificate = netdata_ssl_validate_certificate_sender;
}

static bool stream_info_parse(struct json_object *jobj, const char *path, STREAM_PARENT *d, BUFFER *error) {
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "status", d->remote.status, error, true);
    JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, "host_id", d->remote.host_id.uuid, error, true);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "nodes", d->remote.nodes, error, true);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "receivers", d->remote.receivers, error, true);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "nonce", d->remote.nonce, error, true);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "first_time_s", d->remote.db_first_time_s, error, true);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, "last_time_s", d->remote.db_last_time_s, error, true);
    JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, path, "db_status", RRDHOST_DB_STATUS_2id, d->remote.db_status, error, true);
    JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, path, "db_liveness", RRDHOST_DB_LIVENESS_2id, d->remote.db_liveness, error, true);
    JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, path, "ingest_type", RRDHOST_INGEST_TYPE_2id, d->remote.ingest_type, error, true);
    JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, path, "ingest_status", RRDHOST_INGEST_STATUS_2id, d->remote.ingest_status, error, true);
    return true;
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
             "Pragma: no-cache" HTTP_ENDL
             "Cache-Control: no-cache" HTTP_ENDL
             "Connection: close" HTTP_HDR_END,
             uuid,
             string2str(d->destination),
             rrdhost_program_name(localhost),
             rrdhost_program_version(localhost));

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "STREAM %s: fetching stream info from '%s'...",
           hostname, string2str(d->destination));

    // Establish connection
    if (!nd_sock_connect_to_this(&sock, string2str(d->destination), default_port, 5, ssl)) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "STREAM %s: failed to connect for stream info to '%s': %s",
               hostname, string2str(d->destination),
               ND_SOCK_ERROR_2str(sock.error));
        return false;
    }

    // Send HTTP request
    ssize_t sent = nd_sock_send_timeout(&sock, buf, strlen(buf), 0, 5);
    if (sent <= 0) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "STREAM %s: failed to send stream info request to '%s': %s",
               hostname, string2str(d->destination),
               ND_SOCK_ERROR_2str(sock.error));
        return false;
    }

    // Receive HTTP response
    ssize_t received = nd_sock_recv_timeout(&sock, buf, sizeof(buf) - 1, 0, 5);
    if (received <= 0) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "STREAM %s: failed to receive stream info response from '%s': %s",
               hostname, string2str(d->destination),
               ND_SOCK_ERROR_2str(sock.error));
        return false;
    }
    buf[received] = '\0';

    // Parse HTTP response and extract JSON
    char *json_start = strstr(buf, HTTP_HDR_END);
    if (!json_start) return false;
    json_start += sizeof(HTTP_HDR_END) - 1; // skip the entire header

    CLEAN_JSON_OBJECT *jobj = json_tokener_parse(json_start);
    if (!jobj) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "STREAM %s: failed to parse JSON stream info response from '%s'",
               hostname, string2str(d->destination));
        return false;
    }

    CLEAN_BUFFER *error = buffer_create(0, NULL);

    if(!stream_info_parse(jobj, "", d, error)) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "STREAM %s: failed to extract fields from JSON stream info response from '%s': %s",
               hostname, string2str(d->destination),
               buffer_tostring(error));
        return false;
    }

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "STREAM %s: received stream_info data from '%s': "
           "status: %d, nodes: %zu, receivers: %zu, first_time_s: %ld, last_time_s: %ld, "
           "db status: %s, db liveness: %s, ingest type: %s, ingest status: %s",
           hostname, string2str(d->destination),
           d->remote.status, d->remote.nodes, d->remote.receivers,
           d->remote.db_first_time_s, d->remote.db_last_time_s,
           RRDHOST_DB_STATUS_2str(d->remote.db_status),
           RRDHOST_DB_LIVENESS_2str(d->remote.db_liveness),
           RRDHOST_INGEST_TYPE_2str(d->remote.ingest_type),
           RRDHOST_INGEST_STATUS_2str(d->remote.ingest_status));

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

bool stream_parent_connect_to_one(
    ND_SOCK *sender_sock,
    RRDHOST *host,
    int default_port,
    time_t timeout,
    size_t *reconnects_counter,
    char *connected_to,
    size_t connected_to_size,
    STREAM_PARENT **destination)
{
    sender_sock->error = ND_SOCK_ERR_NO_DESTINATION_AVAILABLE;

    // count the parents
    size_t size = 0;
    for (STREAM_PARENT *d = host->stream.snd.parents.all; d; d = d->next) size++;

    // do we have any parents?
    if(!size) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG, "STREAM %s: no parents configured", rrdhost_hostname(host));
        return false;
    }

    STREAM_PARENT *array[size];
    usec_t now_ut = now_realtime_usec();

    // fetch stream info for all of them and put them in the array
    size_t count = 0, skipped_but_useful = 0, skipped_not_useful = 0;
    for (STREAM_PARENT *d = host->stream.snd.parents.all; d && count < size ; d = d->next) {
        if (nd_thread_signaled_to_cancel()) {
            sender_sock->error = ND_SOCK_ERR_THREAD_CANCELLED;
            return false;
        }

        // make sure they all have a random number
        // this is taken from the parent, but if the stream_info call fails
        // we generate a random number for every parent here
        d->remote.nonce = os_random32();

        if (d->banned_permanently || d->banned_for_this_session)
            continue;

        if (d->postpone_until_ut > now_ut) {
            skipped_but_useful++;
            nd_log(NDLS_DAEMON, NDLP_DEBUG,
                   "STREAM %s: skipping useful parent '%s': POSTPONED FOR %ld SECS MORE: %s",
                   rrdhost_hostname(host),
                   string2str(d->destination),
                   (time_t)((d->postpone_until_ut - now_ut) / USEC_PER_SEC),
                   stream_handshake_error_to_string(d->reason));
            continue;
        }

        bool skip = false;
        d->reason = STREAM_HANDSHAKE_CONNECTING;
        if(stream_info_fetch(d, host->machine_guid, default_port,
                              sender_sock, stream_parent_is_ssl(d), rrdhost_hostname(host))) {
            d->selection.info = true;

            switch(d->remote.ingest_type) {
                case RRDHOST_INGEST_TYPE_VIRTUAL:
                case RRDHOST_INGEST_TYPE_LOCALHOST:
                    d->reason = STREAM_HANDSHAKE_ERROR_LOCALHOST;
                    if(rrdhost_is_host_in_stream_path(host, d->remote.host_id, host->sender->hops)) {
                        d->since_ut = now_ut;
                        d->banned_permanently = true;
                        skipped_not_useful++;
                        nd_log(NDLS_DAEMON, NDLP_NOTICE,
                               "STREAM %s: destination '%s' is banned permanently because it is the origin server",
                               rrdhost_hostname(host), string2str(d->destination));
                        continue;
                    }
                    else
                        skip = true;
                    break;

                default:
                case RRDHOST_INGEST_TYPE_CHILD:
                case RRDHOST_INGEST_TYPE_ARCHIVED:
                    break;
            }

            switch(d->remote.ingest_status) {
                case RRDHOST_INGEST_STATUS_INITIALIZING:
                    d->reason = STREAM_HANDSHAKE_INITIALIZATION;
                    skip = true;
                    break;

                case RRDHOST_INGEST_STATUS_REPLICATING:
                case RRDHOST_INGEST_STATUS_ONLINE:
                    d->reason = STREAM_HANDSHAKE_ERROR_ALREADY_CONNECTED;
                    if(rrdhost_is_host_in_stream_path(host, d->remote.host_id, host->sender->hops)) {
                        d->since_ut = now_ut;
                        d->banned_for_this_session = true;
                        skipped_not_useful++;
                        nd_log(NDLS_DAEMON, NDLP_NOTICE,
                               "STREAM %s: destination '%s' is banned for this session, because it is in our path before us.",
                               rrdhost_hostname(host), string2str(d->destination));
                        continue;
                    }
                    else
                        skip = true;
                    break;

                default:
                case RRDHOST_INGEST_STATUS_OFFLINE:
                    break;
            }
        }
        else
            d->selection.info = false;

        if(skip) {
            skipped_but_useful++;
            nd_log(NDLS_DAEMON, NDLP_DEBUG,
                   "STREAM %s: skipping useful parent '%s': %s",
                   rrdhost_hostname(host),
                   string2str(d->destination),
                   stream_handshake_error_to_string(d->reason));
        }
        else
            array[count++] = d;
    }

    // can we use any parent?
    if(!count) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "STREAM %s: no parents available (%zu skipped but useful, %zu skipped not useful)",
               rrdhost_hostname(host),
            skipped_but_useful, skipped_not_useful);
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
            for (size_t i = base + 1; i < count; i++) {
                if (array[i]->remote.db_last_time_s - array[i - 1]->remote.db_last_time_s <= TIME_TO_CONSIDER_PARENTS_SIMILAR)
                    similar++;
                else
                    break;

                similar++;
            }

            // if we have only 1 similar, move on
            if (similar == 1) {
                nd_log(NDLS_DAEMON, NDLP_DEBUG,
                       "STREAM %s: reordering keeps parent No %zu, '%s'",
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
                    size_t best = base;
                    for(size_t i = base + 1 ; i < base + similar ;i++) {
                        if((array[i]->remote.nonce | os_random32()) > (array[best]->remote.nonce | os_random32()))
                            best = i;
                    }

                    size_t chosen = best;
                    if (chosen != base)
                        SWAP(array[base], array[chosen]);

                    const char *selected = string2str(array[base]->destination);

                    nd_log(NDLS_DAEMON, NDLP_DEBUG,
                           "STREAM %s: random reordering of %zu similar parents (slots %zu to %zu), No %zu is '%s'",
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
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "STREAM %s: only 1 parent is available: '%s'",
               rrdhost_hostname(host), string2str(array[0]->destination));
    }

    // now the parents are sorted based on preference of connection
    for(size_t i = 0; i < count ;i++) {
        STREAM_PARENT *d = array[i];

        if(d->postpone_until_ut > now_ut)
            continue;

        if(nd_thread_signaled_to_cancel()) {
            sender_sock->error = ND_SOCK_ERR_THREAD_CANCELLED;
            return false;
        }

        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "STREAM %s: connecting to '%s' (default port: %d, parent %zu of %zu)...",
               rrdhost_hostname(host), string2str(d->destination), default_port,
               i + 1, count);

        if (reconnects_counter)
            *reconnects_counter += 1;

        ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_STR(NDF_DST_IP, d->destination),
            ND_LOG_FIELD_I64(NDF_DST_PORT, default_port),
            ND_LOG_FIELD_END(),
        };
        ND_LOG_STACK_PUSH(lgs);

        d->since_ut = now_ut;
        d->attempts++;
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
                   "STREAM %s: connected to '%s' (default port: %d, fd %d)...",
                   rrdhost_hostname(host), string2str(d->destination), default_port,
                   sender_sock->fd);

            sender_sock->error = ND_SOCK_ERR_NONE;
            return true;
        }
        else {
            d->postpone_until_ut = now_ut + 5 * 60 * USEC_PER_SEC;

            switch(sender_sock->error) {
                case ND_SOCK_ERR_CONNECTION_REFUSED:
                    d->reason = STREAM_HANDSHAKE_CONNECTION_REFUSED;
                    break;

                case ND_SOCK_ERR_CANNOT_RESOLVE_HOSTNAME:
                    d->reason = STREAM_HANDSHAKE_CANT_RESOLVE_HOSTNAME;
                    break;

                case ND_SOCK_ERR_NO_HOST_IN_DEFINITION:
                    d->reason = STREAM_HANDSHAKE_NO_HOST_IN_DESTINATION;
                    break;

                case ND_SOCK_ERR_TIMEOUT:
                    d->reason = STREAM_HANDSHAKE_CONNECT_TIMEOUT;
                    break;

                case ND_SOCK_ERR_SSL_INVALID_CERTIFICATE:
                    d->reason = STREAM_HANDSHAKE_ERROR_INVALID_CERTIFICATE;
                    break;

                case ND_SOCK_ERR_SSL_CANT_ESTABLISH_SSL_CONNECTION:
                case ND_SOCK_ERR_SSL_FAILED_TO_OPEN:
                    d->reason = STREAM_HANDSHAKE_ERROR_SSL_ERROR;
                    break;

                default:
                case ND_SOCK_ERR_POLL_ERROR:
                case ND_SOCK_ERR_FAILED_TO_CREATE_SOCKET:
                case ND_SOCK_ERR_UNKNOWN_ERROR:
                    d->reason = STREAM_HANDSHAKE_INTERNAL_ERROR;
                    break;

                case ND_SOCK_ERR_THREAD_CANCELLED:
                case ND_SOCK_ERR_NO_DESTINATION_AVAILABLE:
                    d->reason = STREAM_HANDSHAKE_INTERNAL_ERROR;
                    break;
            }

            nd_log(NDLS_DAEMON, NDLP_DEBUG,
                   "STREAM %s: stream connection to '%s' failed (default port: %d): %s",
                   rrdhost_hostname(host),
                   string2str(d->destination), default_port,
                   ND_SOCK_ERROR_2str(sender_sock->error));
        }
    }

    return false;
}

struct stream_parent_init_tmp {
    RRDHOST *host;
    STREAM_PARENT *list;
    int count;
};

static bool stream_parent_add_one(char *entry, void *data) {
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

    __atomic_add_fetch(&netdata_buffers_statistics.rrdhost_senders, sizeof(STREAM_PARENT), __ATOMIC_RELAXED);

    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(t->list, d, prev, next);

    t->count++;
    nd_log_daemon(NDLP_INFO, "STREAM %s: added streaming destination No %d: '%s'",
                  rrdhost_hostname(t->host), t->count, string2str(d->destination));

    return false; // we return false, so that we will get all defined destinations
}

void rrdhost_stream_parents_init(RRDHOST *host) {
    if(!host->stream.snd.destination) return;

    rrdhost_stream_parents_free(host);

    struct stream_parent_init_tmp t = {
        .host = host,
        .list = NULL,
        .count = 0,
    };
    foreach_entry_in_connection_string(string2str(host->stream.snd.destination), stream_parent_add_one, &t);
    host->stream.snd.parents.all = t.list;
}

void rrdhost_stream_parents_free(RRDHOST *host) {
    while (host->stream.snd.parents.all) {
        STREAM_PARENT *tmp = host->stream.snd.parents.all;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(host->stream.snd.parents.all, tmp, prev, next);
        string_freez(tmp->destination);
        freez(tmp);
        __atomic_sub_fetch(&netdata_buffers_statistics.rrdhost_senders, sizeof(STREAM_PARENT), __ATOMIC_RELAXED);
    }

    host->stream.snd.parents.all = NULL;
}

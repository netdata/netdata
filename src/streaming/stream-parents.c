// SPDX-License-Identifier: GPL-3.0-or-later

#include "sender-internals.h"

struct stream_parent {
    STRING *destination;
    bool ssl;
    uint32_t attempts;
    time_t since;
    time_t postpone_reconnection_until;
    STREAM_HANDSHAKE reason;

    struct {
        int status;
        size_t nodes;
        size_t receivers;
        RRDHOST_DB_STATUS db_status;
        RRDHOST_DB_LIVENESS db_liveness;
        RRDHOST_INGEST_TYPE ingest_type;
        RRDHOST_INGEST_STATUS ingest_status;
        time_t db_first_time_s;
        time_t db_last_time_s;
    } remote;

    STREAM_PARENT *prev;
    STREAM_PARENT *next;
};

void stream_parent_set_disconnect_reason(STREAM_PARENT *d, STREAM_HANDSHAKE reason, time_t since) {
    if(!d) return;
    d->since = since;
    d->reason = reason;
}

void stream_parent_set_reconnect_delay(STREAM_PARENT *d, STREAM_HANDSHAKE reason, time_t postpone_reconnection_until) {
    if(!d) return;
    d->reason = reason;
    d->postpone_reconnection_until = postpone_reconnection_until;
}

time_t stream_parent_get_reconnection_t(STREAM_PARENT *d) {
    return d ? d->postpone_reconnection_until : 0;
}

bool stream_parent_is_ssl(STREAM_PARENT *d) {
    return d ? d->ssl : false;
}

time_t stream_parent_handshake_error_to_json(BUFFER *wb, RRDHOST *host) {
    time_t last_attempt = 0;
    for(STREAM_PARENT *d = host->stream.snd.parents.all; d ; d = d->next) {
        if(d->since > last_attempt)
            last_attempt = d->since;

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

            buffer_json_member_add_time_t(wb, "since", d->since);
            buffer_json_member_add_time_t(wb, "age", s->now - d->since);
            buffer_json_member_add_string(wb, "last_handshake", stream_handshake_error_to_string(d->reason));
            if(d->postpone_reconnection_until > s->now) {
                buffer_json_member_add_time_t(wb, "next_check", d->postpone_reconnection_until);
                buffer_json_member_add_time_t(wb, "next_in", d->postpone_reconnection_until - s->now);
            }
        }
        buffer_json_object_close(wb); // each candidate
    }
}

void rrdhost_stream_parent_reset_postpone_time(RRDHOST *host) {
    uint32_t wait = stream_send.parents.reconnect_delay_s;
    time_t now = now_realtime_sec();
    for (STREAM_PARENT *d = host->stream.snd.parents.all; d; d = d->next)
        d->postpone_reconnection_until = now + wait;
}

void rrdhost_stream_parent_ssl_init(RRDHOST *host) {
    static SPINLOCK sp = NETDATA_SPINLOCK_INITIALIZER;
    spinlock_lock(&sp);

    if(netdata_ssl_streaming_sender_ctx || !host) {
        spinlock_unlock(&sp);
        return;
    }

    for(STREAM_PARENT *d = host->stream.snd.parents.all; d ; d = d->next) {
        if (d->ssl) {
            // we need to initialize SSL

            netdata_ssl_initialize_ctx(NETDATA_SSL_STREAMING_SENDER_CTX);
            ssl_security_location_for_context(netdata_ssl_streaming_sender_ctx,
                                              string2str(stream_send.parents.ssl_ca_file), string2str(stream_send.parents.ssl_ca_path));

            // stop the loop
            break;
        }
    }

    spinlock_unlock(&sp);
}

static bool fetch_stream_info(STREAM_PARENT *d, const char *uuid, int default_port, ND_SOCK *s) {
    char buf[HTTP_HEADER_SIZE];
    CLEAN_ND_SOCK sock = {
        .ctx = s->ctx,
        .verify_certificate = s->verify_certificate,
    };

    const char *dst = string2str(d->destination);

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

    // Establish connection
    if (!nd_sock_connect_to_this(&sock, string2str(d->destination), default_port, 5, false))
        return false;

    // Send HTTP request
    ssize_t sent = nd_sock_send_timeout(&sock, buf, strlen(buf), 0, 5);
    if (sent <= 0) return false;

    // Receive HTTP response
    ssize_t received = nd_sock_recv_timeout(&sock, buf, sizeof(buf) - 1, 0, 5);
    if (received <= 0) return false;
    buf[received] = '\0';

    // Parse HTTP response and extract JSON
    char *json_start = strstr(buf, HTTP_HDR_END);
    if (!json_start) return false;
    json_start += sizeof(HTTP_HDR_END) - 1; // skip the entire header

    CLEAN_JSON_OBJECT *jobj = json_tokener_parse(json_start);
    if (!jobj) return false;

    CLEAN_BUFFER *error = buffer_create(0, NULL);

    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, "", "status", d->remote.status, error, true);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, "", "nodes", d->remote.nodes, error, true);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, "", "receivers", d->remote.receivers, error, true);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, "", "first_time_s", d->remote.db_first_time_s, error, true);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, "", "last_time_s", d->remote.db_last_time_s, error, true);
    JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, "", "db_status", RRDHOST_DB_STATUS_2id, d->remote.db_status, error, true);
    JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, "", "db_liveness", RRDHOST_DB_LIVENESS_2id, d->remote.db_liveness, error, true);
    JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, "", "ingest_type", RRDHOST_INGEST_TYPE_2id, d->remote.ingest_type, error, true);
    JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, "", "ingest_status", RRDHOST_INGEST_STATUS_2id, d->remote.ingest_status, error, true);

    return true;
}

static int compare_last_time(const void *a, const void *b) {
    STREAM_PARENT *parent_a = *(STREAM_PARENT **)a;
    STREAM_PARENT *parent_b = *(STREAM_PARENT **)b;

    if (parent_a->remote.db_last_time_s < parent_b->remote.db_last_time_s) return 1;
    else if (parent_a->remote.db_last_time_s > parent_b->remote.db_last_time_s) return -1;
    else {
        if(parent_a->since < parent_b->since) return -1;
        else if(parent_a->since > parent_b->since) return 1;
        else {
            if(parent_a->attempts < parent_b->attempts) return -1;
            else if(parent_a->attempts > parent_b->attempts) return 1;
            else return 0;
        }
    }
}

bool stream_parent_connect_to_one(
    ND_SOCK *s,
    RRDHOST *host,
    int default_port,
    time_t timeout,
    size_t *reconnects_counter,
    char *connected_to,
    size_t connected_to_size,
    STREAM_PARENT **destination)
{
    s->error = ND_SOCK_ERR_NO_PARENT_AVAILABLE;

    // count the parents
    size_t size = 0;
    for (STREAM_PARENT *d = host->stream.snd.parents.all; d; d = d->next) size++;

    // do we have any parents?
    if(!size) return false;
    STREAM_PARENT *array[size];

    // fetch stream info for all of them and put them in the array
    size_t count = 0;
    for (STREAM_PARENT *d = host->stream.snd.parents.all; d; d = d->next) {
        if (nd_thread_signaled_to_cancel()) {
            s->error = ND_SOCK_ERR_THREAD_CANCELLED;
            return false;
        }

        if (d->postpone_reconnection_until > now_realtime_sec())
            continue;

        if(!fetch_stream_info(d, host->machine_guid, default_port, s))
            memset(&d->remote, 0, sizeof(d->remote));
        else if((d->remote.ingest_type == RRDHOST_INGEST_TYPE_CHILD || d->remote.ingest_type == RRDHOST_INGEST_TYPE_ARCHIVED)  &&
                 (d->remote.ingest_status == RRDHOST_INGEST_STATUS_OFFLINE || d->remote.ingest_status == RRDHOST_INGEST_STATUS_ARCHIVED) &&
                 d->remote.db_status == RRDHOST_DB_STATUS_QUERYABLE &&
                 d->remote.db_liveness == RRDHOST_DB_LIVENESS_STALE)
            array[count++] = d;
    }

    // can we use any parent?
    if(!count) return false;

    // sort the array
    qsort(array, count, sizeof(STREAM_PARENT *), compare_last_time);

    size_t base = 0;
    while(base < count) {
        // find how many have similar db_last_time_s;
        size_t similar = 1;
        for (size_t i = base + 1; i < count; i++) {
            if (array[i]->remote.db_last_time_s - array[i - 1]->remote.db_last_time_s <= 60)
                similar++;
            else
                break;
        }

        // if we have only 1 similar, move on
        if(similar == 1) {
            base++;
            continue;
        }

        // reorder the parents who have similar db_last_time
        while (similar > 1) {
            size_t chosen = base + os_random(similar);
            if (chosen != base) SWAP(array[base], array[chosen]);
            base++;
            similar--;
        }
    }

    // now the parents are sorted based on preference of connection
    for(size_t i = 0; i < count ;i++) {
        STREAM_PARENT *d = array[i];

        time_t now = now_realtime_sec();

        if(nd_thread_signaled_to_cancel()) {
            s->error = ND_SOCK_ERR_THREAD_CANCELLED;
            return false;
        }

        if(d->postpone_reconnection_until > now)
            continue;

        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "STREAM %s: connecting to '%s' (default port: %d)...",
               rrdhost_hostname(host), string2str(d->destination), default_port);

        if (reconnects_counter)
            *reconnects_counter += 1;

        d->since = now;
        d->attempts++;
        if (nd_sock_connect_to_this(s, string2str(d->destination), default_port, timeout, stream_parent_is_ssl(d))) {
            if (connected_to && connected_to_size)
                strncpyz(connected_to, string2str(d->destination), connected_to_size);

            *destination = d;

            // move the current item to the end of the list
            // without this, this destination will break the loop again and again
            // not advancing the destinations to find one that may work
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(host->stream.snd.parents.all, d, prev, next);
            DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(host->stream.snd.parents.all, d, prev, next);

            s->error = ND_SOCK_ERR_NONE;
            return true;
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
    nd_log_daemon(NDLP_INFO, "STREAM: added streaming destination No %d: '%s' to host '%s'", t->count, string2str(d->destination), rrdhost_hostname(t->host));

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

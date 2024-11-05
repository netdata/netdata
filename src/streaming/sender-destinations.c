// SPDX-License-Identifier: GPL-3.0-or-later

#include "sender-internals.h"

void rrdpush_reset_destinations_postpone_time(RRDHOST *host) {
    uint32_t wait = (host->sender) ? host->sender->reconnect_delay : 5;
    time_t now = now_realtime_sec();
    for (struct rrdpush_destinations *d = host->destinations; d; d = d->next)
        d->postpone_reconnection_until = now + wait;
}

void rrdpush_sender_ssl_init(RRDHOST *host) {
    static SPINLOCK sp = NETDATA_SPINLOCK_INITIALIZER;
    spinlock_lock(&sp);

    if(netdata_ssl_streaming_sender_ctx || !host) {
        spinlock_unlock(&sp);
        return;
    }

    for(struct rrdpush_destinations *d = host->destinations; d ; d = d->next) {
        if (d->ssl) {
            // we need to initialize SSL

            netdata_ssl_initialize_ctx(NETDATA_SSL_STREAMING_SENDER_CTX);
            ssl_security_location_for_context(netdata_ssl_streaming_sender_ctx, stream_conf_ssl_ca_file, stream_conf_ssl_ca_path);

            // stop the loop
            break;
        }
    }

    spinlock_unlock(&sp);
}

int connect_to_one_of_destinations(
    RRDHOST *host,
    int default_port,
    struct timeval *timeout,
    size_t *reconnects_counter,
    char *connected_to,
    size_t connected_to_size,
    struct rrdpush_destinations **destination)
{
    int sock = -1;

    for (struct rrdpush_destinations *d = host->destinations; d; d = d->next) {
        time_t now = now_realtime_sec();

        if(nd_thread_signaled_to_cancel())
            return -1;

        if(d->postpone_reconnection_until > now)
            continue;

        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "STREAM %s: connecting to '%s' (default port: %d)...",
               rrdhost_hostname(host), string2str(d->destination), default_port);

        if (reconnects_counter)
            *reconnects_counter += 1;

        d->since = now;
        d->attempts++;
        sock = connect_to_this(string2str(d->destination), default_port, timeout);

        if (sock != -1) {
            if (connected_to && connected_to_size)
                strncpyz(connected_to, string2str(d->destination), connected_to_size);

            *destination = d;

            // move the current item to the end of the list
            // without this, this destination will break the loop again and again
            // not advancing the destinations to find one that may work
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(host->destinations, d, prev, next);
            DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(host->destinations, d, prev, next);

            break;
        }
    }

    return sock;
}

struct destinations_init_tmp {
    RRDHOST *host;
    struct rrdpush_destinations *list;
    int count;
};

static bool destinations_init_add_one(char *entry, void *data) {
    struct destinations_init_tmp *t = data;

    struct rrdpush_destinations *d = callocz(1, sizeof(struct rrdpush_destinations));
    char *colon_ssl = strstr(entry, ":SSL");
    if(colon_ssl) {
        *colon_ssl = '\0';
        d->ssl = true;
    }
    else
        d->ssl = false;

    d->destination = string_strdupz(entry);

    __atomic_add_fetch(&netdata_buffers_statistics.rrdhost_senders, sizeof(struct rrdpush_destinations), __ATOMIC_RELAXED);

    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(t->list, d, prev, next);

    t->count++;
    nd_log_daemon(NDLP_INFO, "STREAM: added streaming destination No %d: '%s' to host '%s'", t->count, string2str(d->destination), rrdhost_hostname(t->host));

    return false; // we return false, so that we will get all defined destinations
}

void rrdpush_destinations_init(RRDHOST *host) {
    if(!host->rrdpush.send.destination) return;

    rrdpush_destinations_free(host);

    struct destinations_init_tmp t = {
        .host = host,
        .list = NULL,
        .count = 0,
    };

    foreach_entry_in_connection_string(host->rrdpush.send.destination, destinations_init_add_one, &t);

    host->destinations = t.list;
}

void rrdpush_destinations_free(RRDHOST *host) {
    while (host->destinations) {
        struct rrdpush_destinations *tmp = host->destinations;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(host->destinations, tmp, prev, next);
        string_freez(tmp->destination);
        freez(tmp);
        __atomic_sub_fetch(&netdata_buffers_statistics.rrdhost_senders, sizeof(struct rrdpush_destinations), __ATOMIC_RELAXED);
    }

    host->destinations = NULL;
}


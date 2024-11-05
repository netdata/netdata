// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SENDER_DESTINATIONS_H
#define NETDATA_SENDER_DESTINATIONS_H

#include "libnetdata/libnetdata.h"
#include "stream-handshake.h"
#include "database/rrd.h"

struct rrdpush_destinations {
    STRING *destination;
    bool ssl;
    uint32_t attempts;
    time_t since;
    time_t postpone_reconnection_until;
    STREAM_HANDSHAKE reason;

    struct rrdpush_destinations *prev;
    struct rrdpush_destinations *next;
};

void rrdpush_sender_ssl_init(RRDHOST *host);

void rrdpush_reset_destinations_postpone_time(RRDHOST *host);

void rrdpush_destinations_init(RRDHOST *host);
void rrdpush_destinations_free(RRDHOST *host);

int connect_to_one_of_destinations(
    RRDHOST *host,
    int default_port,
    struct timeval *timeout,
    size_t *reconnects_counter,
    char *connected_to,
    size_t connected_to_size,
    struct rrdpush_destinations **destination);

#endif //NETDATA_SENDER_DESTINATIONS_H

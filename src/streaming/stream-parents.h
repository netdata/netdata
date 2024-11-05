// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_PARENTS_H
#define NETDATA_STREAM_PARENTS_H

#include "libnetdata/libnetdata.h"
#include "stream-handshake.h"

struct rrdhost;
struct rrdhost_status;
struct stream_parent;
typedef struct stream_parent STREAM_PARENT;

void rrdpush_sender_ssl_init(struct rrdhost *host);

void rrdpush_reset_destinations_postpone_time(struct rrdhost *host);

void rrdpush_destinations_init(struct rrdhost *host);
void rrdpush_destinations_free(struct rrdhost *host);

int connect_to_one_of_destinations(
    struct rrdhost *host,
    int default_port,
    struct timeval *timeout,
    size_t *reconnects_counter,
    char *connected_to,
    size_t connected_to_size,
    STREAM_PARENT **destination);

void rrdpush_sender_destinations_to_json(BUFFER *wb, struct rrdhost_status *s);
void rrdpush_destination_set_disconnect_reason(STREAM_PARENT *d, STREAM_HANDSHAKE reason, time_t since);
void rrdpush_destination_set_reconnect_delay(STREAM_PARENT *d, STREAM_HANDSHAKE reason, time_t postpone_reconnection_until);
time_t rrdpush_destination_get_reconnection_t(STREAM_PARENT *d);
bool rrdpush_destination_is_ssl(STREAM_PARENT *d);

time_t rrdpush_destinations_handshare_error_to_json(BUFFER *wb, struct rrdhost *host);

#endif //NETDATA_STREAM_PARENTS_H

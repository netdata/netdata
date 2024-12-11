// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_PARENTS_H
#define NETDATA_STREAM_PARENTS_H

#include "libnetdata/libnetdata.h"

struct rrdhost;
struct rrdhost_status;
struct stream_parent;
typedef struct stream_parent STREAM_PARENT;

typedef struct rrdhost_stream_parents {
    RW_SPINLOCK spinlock;
    STREAM_PARENT *all;         // a linked list of possible destinations
    STREAM_PARENT *current;     // the current destination from the above list
} RRDHOST_STREAM_PARENTS;

#include "stream-handshake.h"
#include "rrdhost-status.h"

void rrdhost_stream_parent_ssl_init(struct sender_state *s);

int stream_info_to_json_v1(BUFFER *wb, const char *machine_guid);

void rrdhost_stream_parents_reset(RRDHOST *host, STREAM_HANDSHAKE reason);

void rrdhost_stream_parents_update_from_destination(RRDHOST *host);
void rrdhost_stream_parents_free(struct rrdhost *host, bool having_write_lock);

bool stream_parent_connect_to_one(
    ND_SOCK *sender_sock,
    struct rrdhost *host,
    int default_port,
    time_t timeout,
    char *connected_to,
    size_t connected_to_size,
    STREAM_PARENT **destination);

void rrdhost_stream_parents_to_json(BUFFER *wb, struct rrdhost_status *s);
STREAM_HANDSHAKE stream_parent_get_disconnect_reason(STREAM_PARENT *d);
void stream_parent_set_disconnect_reason(STREAM_PARENT *d, STREAM_HANDSHAKE reason, time_t since);
void stream_parent_set_reconnect_delay(STREAM_PARENT *d, STREAM_HANDSHAKE reason, time_t secs);
usec_t stream_parent_get_reconnection_ut(STREAM_PARENT *d);
bool stream_parent_is_ssl(STREAM_PARENT *d);

usec_t stream_parent_handshake_error_to_json(BUFFER *wb, struct rrdhost *host);

void rrdhost_stream_parents_init(RRDHOST *host);

#endif //NETDATA_STREAM_PARENTS_H

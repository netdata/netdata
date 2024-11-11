// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_PARENTS_H
#define NETDATA_STREAM_PARENTS_H

#include "libnetdata/libnetdata.h"
#include "stream-handshake.h"
#include "rrdhost-status.h"

struct rrdhost;
struct rrdhost_status;
struct stream_parent;
typedef struct stream_parent STREAM_PARENT;

void rrdhost_stream_parent_ssl_init(struct sender_state *s);

int stream_info_json_generate(BUFFER *wb, const char *machine_guid);

void rrdhost_stream_parents_reset(RRDHOST *host, STREAM_HANDSHAKE reason);

void rrdhost_stream_parents_init(struct rrdhost *host);
void rrdhost_stream_parents_free(struct rrdhost *host);

bool stream_parent_connect_to_one(
    ND_SOCK *sender_sock,
    struct rrdhost *host,
    int default_port,
    time_t timeout,
    size_t *reconnects_counter,
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

#endif //NETDATA_STREAM_PARENTS_H

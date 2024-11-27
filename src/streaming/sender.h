// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SENDER_H
#define NETDATA_SENDER_H

#include "libnetdata/libnetdata.h"

typedef enum __attribute__((packed)) {
    STREAM_TRAFFIC_TYPE_REPLICATION = 0,
    STREAM_TRAFFIC_TYPE_FUNCTIONS,
    STREAM_TRAFFIC_TYPE_METADATA,
    STREAM_TRAFFIC_TYPE_DATA,

    // terminator
    STREAM_TRAFFIC_TYPE_MAX,
} STREAM_TRAFFIC_TYPE;

struct rrdhost;
struct sender_state;

#include "stream-handshake.h"
#include "stream-capabilities.h"
#include "stream-parents.h"

// thread buffer for sending data upstream (to a parent)
BUFFER *sender_start(struct sender_state *s);
void sender_commit(struct sender_state *s, BUFFER *wb, STREAM_TRAFFIC_TYPE type);
void sender_thread_buffer_free(void);

// starting and stopping senders
void *localhost_sender_start(void *ptr);
void rrdhost_sender_start(struct rrdhost *host);
void rrdhost_sender_signal_to_stop_and_wait(struct rrdhost *host, STREAM_HANDSHAKE reason, bool wait);
void stream_threads_cancel(void);

// managing host sender structures
void rrdhost_sender_structures_init(struct rrdhost *host);
void rrdhost_sender_structures_free(struct rrdhost *host);

// querying host sender information
bool rrdhost_sender_is_connected_with_ssl(struct rrdhost *host);
bool rrdhost_sender_has_compression(struct rrdhost *host);
bool rrdhost_sender_has_capabilities(struct rrdhost *host, STREAM_CAPABILITIES capabilities);

#include "replication.h"

#endif //NETDATA_SENDER_H

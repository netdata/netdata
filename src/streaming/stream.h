// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_H
#define NETDATA_STREAM_H 1

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
struct receiver_state;

#include "stream-conf.h"
#include "stream-handshake.h"
#include "stream-capabilities.h"
#include "stream-parents.h"

// thread buffer for sending data upstream (to a parent)
BUFFER *sender_start(struct sender_state *s);
void sender_commit(struct sender_state *s, BUFFER *wb, STREAM_TRAFFIC_TYPE type);
void sender_commit_thread_buffer_free(void);

// starting and stopping senders
void *stream_sender_start_localhost(void *ptr);
void stream_sender_start_host(struct rrdhost *host);
void stream_sender_signal_to_stop_and_wait(struct rrdhost *host, STREAM_HANDSHAKE reason, bool wait);

// managing host sender structures
void stream_sender_structures_init(struct rrdhost *host);
void stream_sender_structures_free(struct rrdhost *host);

// querying host sender information
bool stream_sender_is_connected_with_ssl(struct rrdhost *host);
bool stream_sender_has_compression(struct rrdhost *host);
bool stream_sender_has_capabilities(struct rrdhost *host, STREAM_CAPABILITIES capabilities);

// receiver API
uint32_t stream_receivers_currently_connected(void);
int stream_receiver_accept_connection(struct web_client *w, char *decoded_query_string, void *h2o_ctx);
bool receiver_has_capability(struct rrdhost *host, STREAM_CAPABILITIES caps);
void stream_receiver_free(struct receiver_state *rpt);
bool stream_receiver_signal_to_stop_and_wait(struct rrdhost *host, STREAM_HANDSHAKE reason);
char *stream_receiver_program_version_strdupz(struct rrdhost *host);

#include "replication.h"
#include "rrdhost-status.h"
#include "protocol/commands.h"
#include "stream-path.h"

void stream_threads_cancel(void);

#endif //NETDATA_STREAM_H

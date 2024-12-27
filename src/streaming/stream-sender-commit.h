// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_SENDER_COMMIT_H
#define NETDATA_STREAM_SENDER_COMMIT_H

#include "libnetdata/libnetdata.h"
#include "stream-traffic-types.h"

struct rrdhost;
struct sender_state;
struct receiver_state;

struct sender_buffer {
    pid_t receiver_tid;
    BUFFER *wb;
    bool used;
    size_t our_recreates;
    size_t sender_recreates;
    const char *last_function;
};
void sender_buffer_destroy(struct sender_buffer *commit);

// thread buffer for sending data upstream (to a parent)

void sender_thread_buffer_free(void);
BUFFER *sender_thread_buffer_with_trace(struct sender_state *s, const char *func);
#define sender_thread_buffer(s) sender_thread_buffer_with_trace(s, __FUNCTION__)

// commit the global host buffer
// this is the preferred buffer for stream threads (unified receiver / sender threads)
// these threads require a buffer that can remain intact while switching hosts
BUFFER *sender_host_buffer_with_trace(struct rrdhost *host, const char *func);
#define sender_host_buffer(host) sender_host_buffer_with_trace(host, __FUNCTION__)

// commit a buffer acquired with sender_thread_buffer()
// this is the preferred buffer for dedicated workers sending a lot of messages (like replication)
// these threads need to maintain enough allocation for repeated use of the buffer
void sender_thread_commit_with_trace(struct sender_state *s, BUFFER *wb, STREAM_TRAFFIC_TYPE type, const char *func);
#define sender_commit(s, wb, type) sender_thread_commit_with_trace(s, wb, type, __FUNCTION__)

// commit any buffer
// this is the preferred buffer for occasional senders, as it avoids constant buffer allocations
void sender_buffer_commit(struct sender_state *s, BUFFER *wb, struct sender_buffer *commit, STREAM_TRAFFIC_TYPE type);
#define sender_commit_clean_buffer(s, wb, type) sender_buffer_commit(s, wb, NULL, type)

#endif //NETDATA_STREAM_SENDER_COMMIT_H

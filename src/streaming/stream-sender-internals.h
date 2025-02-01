// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_SENDER_INTERNALS_H
#define NETDATA_STREAM_SENDER_INTERNALS_H

#include "stream.h"
#include "stream-thread.h"
#include "h2o-common.h"
#include "aclk/https_client.h"
#include "stream-parents.h"
#include "stream-circular-buffer.h"

// connector thread
#define WORKER_SENDER_CONNECTOR_JOB_CONNECTING                          0
#define WORKER_SENDER_CONNECTOR_JOB_CONNECTED                           1
#define WORKER_SENDER_CONNECTOR_JOB_REMOVED                             2
#define WORKER_SENDER_CONNECTOR_JOB_DISCONNECT_BAD_HANDSHAKE            3
#define WORKER_SENDER_CONNECTOR_JOB_DISCONNECT_TIMEOUT                  4
#define WORKER_SENDER_CONNECTOR_JOB_DISCONNECT_CANT_UPGRADE_CONNECTION  5
#define WORKER_SENDER_CONNECTOR_JOB_QUEUED_NODES                        6
#define WORKER_SENDER_CONNECTOR_JOB_CONNECTED_NODES                     7
#define WORKER_SENDER_CONNECTOR_JOB_FAILED_NODES                        8
#define WORKER_SENDER_CONNECTOR_JOB_CANCELLED_NODES                     9

#define CONNECTED_TO_SIZE 100

#include "stream-compression/compression.h"
#include "stream-conf.h"

typedef void (*stream_defer_action_t)(struct sender_state *s, void *data);
typedef void (*stream_defer_cleanup_t)(struct sender_state *s, void *data);

struct sender_state {
    SPINLOCK spinlock;
    STREAM_CAPABILITIES capabilities;
    STREAM_CAPABILITIES disabled_capabilities;
    int16_t hops;
    bool parent_using_h2o;
    WAITQ waitq;
    ND_SOCK sock;
    RRDHOST *host;

    time_t last_state_since_t;                  // the timestamp of the last state (online/offline) change
    STREAM_CIRCULAR_BUFFER *scb;                // sender buffer

    struct {
        struct stream_opcode msg;   // the template for sending a message to the dispatcher - protected by sender_lock()

        // this is a property of stream_sender_send_msg_to_dispatcher()
        // protected by dispatcher->messages.spinlock
        // DO NOT READ OR WRITE ANYWHERE
        uint32_t msg_slot;      // ensures a opcode queue that can never get full

        struct compressor_state compressor;

        struct {
            size_t size;
            char *b;
            ssize_t read_len;
            struct line_splitter line;
        } rbuf;

        struct {
            const char *end_keyword;
            BUFFER *payload;
            stream_defer_action_t action;
            stream_defer_cleanup_t cleanup;
            void *action_data;
        } defer;

        nd_poll_event_t wanted;
        usec_t last_traffic_ut;
        struct pollfd_meta meta;
    } thread;

    struct {
        int8_t id;                              // the connector id - protected by sender_lock()
    } connector;

    struct {
        bool shutdown;                          // when set, the sender should stop sending this host
        STREAM_HANDSHAKE reason;                // the reason we decided to stop this sender
    } exit;

    struct {
        uint32_t last_counter_sum;              // copy from the host, to detect progress
        usec_t last_progress_ut;                // last time we found some progress (monotonic)
        usec_t last_checked_ut;                 // last time we checked for stalled progress (monotonic)

        DICTIONARY *requests;                   // de-duplication of replication requests, per chart
        time_t oldest_request_after_t;          // the timestamp of the oldest replication request
        time_t latest_completed_before_t;       // the timestamp of the latest replication request

        struct {
            size_t pending_requests;            // the currently outstanding replication requests
            size_t charts_replicating;          // the number of unique charts having pending replication requests (on every request one is added and is removed when we finish it - it does not track completion of the replication for this chart)
            bool reached_max;                   // true when the sender buffer should not get more replication responses
        } atomic;
    } replication;

#ifdef NETDATA_LOG_STREAM_SENDER
    struct {
        SPINLOCK spinlock;
        struct timespec first_call;
        BUFFER *received;
        FILE *fp;
    } log;
#endif

    char remote_ip[CONNECTED_TO_SIZE + 1];      // We don't know which proxy we connect to, passed back from socket.c
};

#define stream_sender_lock(sender) spinlock_lock(&(sender)->spinlock)
#define stream_sender_unlock(sender) spinlock_unlock(&(sender)->spinlock)
#define stream_sender_trylock(sender) spinlock_trylock(&(sender)->spinlock)

#define stream_sender_replication_buffer_full_set(sender, value) __atomic_store_n(&((sender)->replication.atomic.reached_max), value, __ATOMIC_SEQ_CST)
#define stream_sender_replication_buffer_full_get(sender) __atomic_load_n(&((sender)->replication.atomic.reached_max), __ATOMIC_SEQ_CST)

#define stream_sender_replicating_charts(sender) __atomic_load_n(&((sender)->replication.atomic.charts_replicating), __ATOMIC_RELAXED)
#define stream_sender_replicating_charts_plus_one(sender) __atomic_add_fetch(&((sender)->replication.atomic.charts_replicating), 1, __ATOMIC_RELAXED)
#define stream_sender_replicating_charts_minus_one(sender) __atomic_sub_fetch(&((sender)->replication.atomic.charts_replicating), 1, __ATOMIC_RELAXED)
#define stream_sender_replicating_charts_zero(sender) __atomic_store_n(&((sender)->replication.atomic.charts_replicating), 0, __ATOMIC_RELAXED)

#define stream_sender_pending_replication_requests(sender) __atomic_load_n(&((sender)->replication.atomic.pending_requests), __ATOMIC_RELAXED)
#define stream_sender_pending_replication_requests_plus_one(sender) __atomic_add_fetch(&((sender)->replication.atomic.pending_requests), 1, __ATOMIC_RELAXED)
#define stream_sender_pending_replication_requests_minus_one(sender) __atomic_sub_fetch(&((sender)->replication.atomic.pending_requests), 1, __ATOMIC_RELAXED)
#define stream_sender_pending_replication_requests_zero(sender) __atomic_store_n(&((sender)->replication.atomic.pending_requests), 0, __ATOMIC_RELAXED)

void stream_sender_add_to_connector_queue(RRDHOST *host);

void stream_sender_execute_commands_cleanup(struct sender_state *s);
void stream_sender_execute_commands(struct sender_state *s);

bool stream_connect(struct sender_state *s, uint16_t default_port, time_t timeout);

bool stream_sender_is_host_stopped(struct sender_state *s);

void stream_sender_send_opcode(struct sender_state *s, struct stream_opcode msg);

void stream_sender_add_to_queue(struct sender_state *s);

// stream connector
typedef enum __attribute__((packed)) {
    STRCNT_CMD_NONE = 0,
    STRCNT_CMD_CONNECT,
    STRCNT_CMD_REMOVE,

    // terminator
    STRCNT_CMD_MAX,
} STRCNT_CMD;

bool stream_connector_init(struct sender_state *s);
void stream_connector_cancel_threads(void);
void stream_connector_add(struct sender_state *s);
void stream_connector_requeue(struct sender_state *s, STRCNT_CMD cmd);
bool stream_connector_is_signaled_to_stop(struct sender_state *s);

void stream_sender_on_connect(struct sender_state *s);
void stream_sender_on_disconnect(struct sender_state *s);

void stream_sender_remove(struct sender_state *s, STREAM_HANDSHAKE reason);

#ifdef NETDATA_LOG_STREAM_SENDER
void stream_sender_log_payload(struct sender_state *s, BUFFER *payload, STREAM_TRAFFIC_TYPE type, bool inbound);
#else
#define stream_sender_log_payload(s, payload, type, inbound) debug_dummy()
#endif

#endif //NETDATA_STREAM_SENDER_INTERNALS_H

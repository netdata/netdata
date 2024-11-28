// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_SENDER_INTERNALS_H
#define NETDATA_STREAM_SENDER_INTERNALS_H

#include "stream.h"
#include "stream-thread.h"
#include "h2o-common.h"
#include "aclk/https_client.h"
#include "stream-parents.h"

// connector thread
#define WORKER_SENDER_CONNECTOR_JOB_CONNECTING                          0
#define WORKER_SENDER_CONNECTOR_JOB_CONNECTED                           1
#define WORKER_SENDER_CONNECTOR_JOB_DISCONNECT_BAD_HANDSHAKE            2
#define WORKER_SENDER_CONNECTOR_JOB_DISCONNECT_TIMEOUT                  3
#define WORKER_SENDER_CONNECTOR_JOB_DISCONNECT_CANT_UPGRADE_CONNECTION  4
#define WORKER_SENDER_CONNECTOR_JOB_QUEUED_NODES                        5
#define WORKER_SENDER_CONNECTOR_JOB_CONNECTED_NODES                     6
#define WORKER_SENDER_CONNECTOR_JOB_FAILED_NODES                        7
#define WORKER_SENDER_CONNECTOR_JOB_CANCELLED_NODES                     8

#define CONNECTED_TO_SIZE 100

#define CBUFFER_INITIAL_SIZE (16 * 1024)
#define CBUFFER_INITIAL_MAX_SIZE (1024 * 1024)
#define THREAD_BUFFER_INITIAL_SIZE (CBUFFER_INITIAL_SIZE / 2)

#include "stream-compression/compression.h"
#include "stream-conf.h"

typedef void (*rrdpush_defer_action_t)(struct sender_state *s, void *data);
typedef void (*rrdpush_defer_cleanup_t)(struct sender_state *s, void *data);

typedef enum __attribute__((packed)) {
    SENDER_MSG_NONE                             = 0,
    SENDER_MSG_ENABLE_SENDING                   = (1 << 0), // move traffic around as soon as possible
    SENDER_MSG_RECONNECT_OVERFLOW               = (1 << 1), // reconnect the node, it has buffer overflow
    SENDER_MSG_RECONNECT_WITHOUT_COMPRESSION    = (1 << 2), // reconnect the node, but disable compression
    SENDER_MSG_STOP_RECEIVER_LEFT               = (1 << 3), // disconnect the node, the receiver left
    SENDER_MSG_STOP_HOST_CLEANUP                = (1 << 4), // disconnect the node, it is being de-allocated
} SENDER_OP;

struct sender_op {
    int32_t thread_slot;                // the dispatcher id this message refers to
    int32_t snd_run_slot;               // the run slot of the dispatcher this message refers to
    uint32_t session;                   // random number used to verify that the message the dispatcher receives is for this sender
    SENDER_OP op;                       // the actual message to be delivered
    struct sender_state *sender;
};

struct sender_state {
    SPINLOCK spinlock;

    RRDHOST *host;
    STREAM_CAPABILITIES capabilities;
    STREAM_CAPABILITIES disabled_capabilities;
    int16_t hops;

    ND_SOCK sock;

    struct {
        struct sender_op msg;   // the template for sending a message to the dispatcher - protected by sender_lock()

        // this is a property of stream_sender_send_msg_to_dispatcher()
        // protected by dispatcher->messages.spinlock
        // DO NOT READ OR WRITE ANYWHERE
        uint32_t msg_slot;      // ensures a dispatcher queue that can never get full

        // statistics about our compression efficiency
        size_t bytes_compressed;
        size_t bytes_uncompressed;

        // the current buffer statistics
        // these SHOULD ALWAYS BE CALCULATED ON EVERY sender_unlock() IF THE BUFFER WAS MODIFIED
        size_t bytes_outstanding;
        size_t bytes_available;
        NETDATA_DOUBLE buffer_ratio;

        // statistics about successful sends
        size_t sends;
        size_t bytes_sent;
        size_t bytes_sent_by_type[STREAM_TRAFFIC_TYPE_MAX];

        int32_t slot;
        struct pollfd_slotted pfd;
    } thread;

    struct {
        int8_t id;                              // the connector id - protected by sender_lock()
    } connector;

    char connected_to[CONNECTED_TO_SIZE + 1];   // We don't know which proxy we connect to, passed back from socket.c
    time_t last_traffic_seen_t;
    time_t last_state_since_t;                  // the timestamp of the last state (online/offline) change

    struct {
        struct circular_buffer *cb;
        size_t recreates;
    } sbuf;

    struct {
        char b[PLUGINSD_LINE_MAX + 1];
        ssize_t read_len;
        struct line_splitter line;
    } rbuf;

    struct compressor_state compressor;

#ifdef NETDATA_LOG_STREAM_SENDER
    FILE *stream_log_fp;
#endif

    struct {
        bool shutdown;                          // when set, the sender should stop sending this host
        STREAM_HANDSHAKE reason;                // the reason we decided to stop this sender
    } exit;

    struct {
        DICTIONARY *requests;                   // de-duplication of replication requests, per chart
        time_t oldest_request_after_t;          // the timestamp of the oldest replication request
        time_t latest_completed_before_t;       // the timestamp of the latest replication request

        struct {
            size_t pending_requests;            // the currently outstanding replication requests
            size_t charts_replicating;          // the number of unique charts having pending replication requests (on every request one is added and is removed when we finish it - it does not track completion of the replication for this chart)
            bool reached_max;                   // true when the sender buffer should not get more replication responses
        } atomic;

    } replication;

    struct {
        size_t buffer_used_percentage;          // the current utilization of the sending buffer
        usec_t last_flush_time_ut;              // the last time the sender flushed the sending buffer in USEC
    } atomic;

    struct {
        const char *end_keyword;
        BUFFER *payload;
        rrdpush_defer_action_t action;
        rrdpush_defer_cleanup_t cleanup;
        void *action_data;
    } defer;

    bool parent_using_h2o;

    // for the sender/connector threads
    struct sender_state *prev, *next;
};

#define sender_lock(sender) spinlock_lock(&(sender)->spinlock)
#define sender_unlock(sender) spinlock_unlock(&(sender)->spinlock)

#define rrdpush_sender_replication_buffer_full_set(sender, value) __atomic_store_n(&((sender)->replication.atomic.reached_max), value, __ATOMIC_SEQ_CST)
#define rrdpush_sender_replication_buffer_full_get(sender) __atomic_load_n(&((sender)->replication.atomic.reached_max), __ATOMIC_SEQ_CST)

#define rrdpush_sender_set_buffer_used_percent(sender, value) __atomic_store_n(&((sender)->atomic.buffer_used_percentage), value, __ATOMIC_RELAXED)
#define rrdpush_sender_get_buffer_used_percent(sender) __atomic_load_n(&((sender)->atomic.buffer_used_percentage), __ATOMIC_RELAXED)

#define rrdpush_sender_set_flush_time(sender) __atomic_store_n(&((sender)->atomic.last_flush_time_ut), now_realtime_usec(), __ATOMIC_RELAXED)
#define rrdpush_sender_get_flush_time(sender) __atomic_load_n(&((sender)->atomic.last_flush_time_ut), __ATOMIC_RELAXED)

#define rrdpush_sender_replicating_charts(sender) __atomic_load_n(&((sender)->replication.atomic.charts_replicating), __ATOMIC_RELAXED)
#define rrdpush_sender_replicating_charts_plus_one(sender) __atomic_add_fetch(&((sender)->replication.atomic.charts_replicating), 1, __ATOMIC_RELAXED)
#define rrdpush_sender_replicating_charts_minus_one(sender) __atomic_sub_fetch(&((sender)->replication.atomic.charts_replicating), 1, __ATOMIC_RELAXED)
#define rrdpush_sender_replicating_charts_zero(sender) __atomic_store_n(&((sender)->replication.atomic.charts_replicating), 0, __ATOMIC_RELAXED)

#define rrdpush_sender_pending_replication_requests(sender) __atomic_load_n(&((sender)->replication.atomic.pending_requests), __ATOMIC_RELAXED)
#define rrdpush_sender_pending_replication_requests_plus_one(sender) __atomic_add_fetch(&((sender)->replication.atomic.pending_requests), 1, __ATOMIC_RELAXED)
#define rrdpush_sender_pending_replication_requests_minus_one(sender) __atomic_sub_fetch(&((sender)->replication.atomic.pending_requests), 1, __ATOMIC_RELAXED)
#define rrdpush_sender_pending_replication_requests_zero(sender) __atomic_store_n(&((sender)->replication.atomic.pending_requests), 0, __ATOMIC_RELAXED)

void stream_sender_add_to_connector_queue(RRDHOST *host);

void stream_sender_execute_commands_cleanup(struct sender_state *s);
void stream_sender_execute_commands(struct sender_state *s);

bool stream_connect(struct sender_state *s, uint16_t default_port, time_t timeout);

bool stream_sender_is_host_stopped(struct sender_state *s);

void stream_sender_send_msg_to_dispatcher(struct sender_state *s, struct sender_op msg);

void stream_sender_thread_data_added_data_unsafe(struct sender_state *s, STREAM_TRAFFIC_TYPE type, uint64_t bytes_compressed, uint64_t bytes_uncompressed);

void stream_sender_add_to_queue(struct sender_state *s);

// stream connector
bool stream_connector_init(struct sender_state *s);
void stream_connector_cancel_threads(void);
void stream_connector_add(struct sender_state *s);
void stream_connector_requeue(struct sender_state *s);
bool stream_connector_is_signaled_to_stop(struct sender_state *s);

void stream_sender_on_connect(struct sender_state *s);

void stream_sender_remove(struct sender_state *s);

#endif //NETDATA_STREAM_SENDER_INTERNALS_H

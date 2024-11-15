// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SENDER_INTERNALS_H
#define NETDATA_SENDER_INTERNALS_H

#include "rrdpush.h"
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

// dispatcher thread
#define WORKER_SENDER_DISPATCHER_JOB_LIST                               0
#define WORKER_SENDER_DISPATCHER_JOB_DEQUEUE                            1
#define WORKER_SENDER_DISPATCHER_JOB_POLL_ERROR                         2
#define WORKER_SENDER_DISPATCHER_JOB_PIPE_READ                          3
#define WORKER_SENDER_DISPATCHER_JOB_SOCKET_RECEIVE                     4
#define WORKER_SENDER_DISPATCHER_JOB_SOCKET_SEND                        5
#define WORKER_SENDER_DISPATCHER_JOB_EXECUTE                            6
#define WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_OVERFLOW                7
#define WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_TIMEOUT                 8
#define WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_SOCKET_ERROR            9
#define WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_PARENT_CLOSED           10
#define WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_RECEIVE_ERROR           11
#define WORKER_SENDER_DISPATCHER_JOB_DISCONNECT_SEND_ERROR              12

// dispatcher execute requests
#define WORKER_SENDER_DISPATCHER_JOB_REPLAY_REQUEST                     13
#define WORKER_SENDER_DISPATCHER_JOB_FUNCTION_REQUEST                   14

// dispatcher metrics
#define WORKER_SENDER_DISPATCHER_JOB_NODES                              15
#define WORKER_SENDER_DISPATCHER_JOB_BUFFER_RATIO                       16
#define WORKER_SENDER_DISPATCHER_JOB_BYTES_RECEIVED                     17
#define WORKER_SENDER_DISPATCHER_JOB_BYTES_SENT                         18
#define WORKER_SENDER_DISPATCHER_JOB_BYTES_COMPRESSED                   19
#define WORKER_SENDER_DISPATCHER_JOB_BYTES_UNCOMPRESSED                 20
#define WORKER_SENDER_DISPATCHER_JOB_BYTES_COMPRESSION_RATIO            21
#define WORKER_SENDER_DISPATHCER_JOB_REPLAY_DICT_SIZE                   22
#define WORKER_SENDER_DISPATHCER_JOB_MESSAGES                           23

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 24
#error WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 25
#endif

#define CONNECTED_TO_SIZE 100

#define CBUFFER_INITIAL_SIZE (16 * 1024)
#define THREAD_BUFFER_INITIAL_SIZE (CBUFFER_INITIAL_SIZE / 2)

typedef enum __attribute__((packed)) {
    SENDER_FLAG_OVERFLOW     = (1 << 0), // The buffer has been overflown
} SENDER_FLAGS;

#include "stream-compression/compression.h"
#include "stream-conf.h"

typedef void (*rrdpush_defer_action_t)(struct sender_state *s, void *data);
typedef void (*rrdpush_defer_cleanup_t)(struct sender_state *s, void *data);

typedef enum __attribute__((packed)) {
    SENDER_MSG_INTERACTIVE = 0,
    SENDER_MSG_RECONNECT,
    SENDER_MSG_STOP,
} SENDER_MSG;

struct pipe_msg {
    uint32_t magic;
    uint32_t slot;
    SENDER_MSG msg;
};

struct sender_state {
    SPINLOCK spinlock;

    RRDHOST *host;
    SENDER_FLAGS flags;
    STREAM_CAPABILITIES capabilities;
    STREAM_CAPABILITIES disabled_capabilities;
    int16_t hops;

    ND_SOCK sock;

    struct {
        int id;
        bool interactive;                       // used internally by the dispatcher to optimize sending in batches
        bool interactive_sent;
        size_t bytes_compressed;
        size_t bytes_uncompressed;
        size_t bytes_outstanding;
        size_t bytes_available;
        NETDATA_DOUBLE buffer_ratio;
        struct pipe_msg pollfd;
        uint32_t pollfd_slot;
    } dispatcher;

    char connected_to[CONNECTED_TO_SIZE + 1];   // We don't know which proxy we connect to, passed back from socket.c
    size_t send_attempts;
    size_t sent_bytes_on_this_connection;
    time_t last_traffic_seen_t;
    time_t last_state_since_t;              // the timestamp of the last state (online/offline) change

    struct {
        struct circular_buffer *cb;
        size_t recreates;
    } sbuf;

    struct {
        char b[PLUGINSD_LINE_MAX + 1];
        ssize_t read_len;
        struct line_splitter line;
    } rbuf;

    // Metrics are collected asynchronously by collector threads calling rrdset_done_push(). This can also trigger
    // the lazy creation of the sender thread - both cases (buffer access and thread creation) are guarded here.
    size_t sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_MAX];

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

void stream_sender_start_host_routing(RRDHOST *host);

void stream_sender_reconnect(struct sender_state *s);

void rrdpush_sender_execute_commands_cleanup(struct sender_state *s);
void rrdpush_sender_execute_commands(struct sender_state *s);

bool stream_sender_connect(struct sender_state *s, uint16_t default_port, time_t timeout);

bool stream_sender_is_host_stopped(struct sender_state *s);

void stream_sender_send_msg_to_dispatcher(struct sender_state *s, struct pipe_msg msg);

void stream_sender_update_dispatcher_added_data_unsafe(struct sender_state *s, uint64_t bytes_compressed, uint64_t bytes_uncompressed);

void stream_sender_dispatcher_add_to_queue(struct sender_state *s);

bool stream_sender_connector_init(void);
void stream_sender_connector_cancel_threads(void);
void stream_sender_connector_remove_unlinked(struct sender_state *s);
void stream_sender_connector_add_unlinked(struct sender_state *s);
void stream_sender_connector_requeue(struct sender_state *s);

bool stream_sender_is_signaled_to_stop(struct sender_state *s);
void stream_sender_on_connect(struct sender_state *s);

#endif //NETDATA_SENDER_INTERNALS_H

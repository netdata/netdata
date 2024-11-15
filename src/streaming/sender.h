// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SENDER_H
#define NETDATA_SENDER_H

#include "libnetdata/libnetdata.h"

#define CONNECTED_TO_SIZE 100

#define CBUFFER_INITIAL_SIZE (16 * 1024)
#define THREAD_BUFFER_INITIAL_SIZE (CBUFFER_INITIAL_SIZE / 2)

typedef enum __attribute__((packed)) {
    STREAM_TRAFFIC_TYPE_REPLICATION = 0,
    STREAM_TRAFFIC_TYPE_FUNCTIONS,
    STREAM_TRAFFIC_TYPE_METADATA,
    STREAM_TRAFFIC_TYPE_DATA,
    STREAM_TRAFFIC_TYPE_DYNCFG,

    // terminator
    STREAM_TRAFFIC_TYPE_MAX,
} STREAM_TRAFFIC_TYPE;

typedef enum __attribute__((packed)) {
    SENDER_FLAG_OVERFLOW     = (1 << 0), // The buffer has been overflown
} SENDER_FLAGS;

typedef struct {
    char *os_name;
    char *os_id;
    char *os_version;
    char *kernel_name;
    char *kernel_version;
} stream_encoded_t;

#include "stream-handshake.h"
#include "stream-capabilities.h"
#include "stream-compression/compression.h"
#include "stream-parents.h"
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
    RRDHOST *host;
    SENDER_FLAGS flags;

    ND_SOCK sock;

    struct {
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

    // Metrics are collected asynchronously by collector threads calling rrdset_done_push(). This can also trigger
    // the lazy creation of the sender thread - both cases (buffer access and thread creation) are guarded here.
    SPINLOCK spinlock;
    struct circular_buffer *buffer;
    char read_buffer[PLUGINSD_LINE_MAX + 1];
    ssize_t read_len;
    STREAM_CAPABILITIES capabilities;
    STREAM_CAPABILITIES disabled_capabilities;

    size_t sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_MAX];

    int16_t hops;

    struct line_splitter line;
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
        time_t last_buffer_recreate_s;          // true when the sender buffer should be re-created
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

#define rrdpush_sender_last_buffer_recreate_get(sender) __atomic_load_n(&(sender)->atomic.last_buffer_recreate_s, __ATOMIC_RELAXED)
#define rrdpush_sender_last_buffer_recreate_set(sender, value) __atomic_store_n(&(sender)->atomic.last_buffer_recreate_s, value, __ATOMIC_RELAXED)

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

BUFFER *sender_start(struct sender_state *s);
void sender_commit(struct sender_state *s, BUFFER *wb, STREAM_TRAFFIC_TYPE type);

void *stream_sender_start_localhost(void *ptr);
void stream_sender_signal_to_stop_and_wait(RRDHOST *host, STREAM_HANDSHAKE reason, bool wait);

void sender_thread_buffer_free(void);

void stream_sender_start_host(RRDHOST *host);

int sender_write_pipe_fd(struct sender_state *s);

void stream_sender_cancel_threads(void);

void stream_sender_reconnect(struct sender_state *s);

#include "replication.h"

#endif //NETDATA_SENDER_H

// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream.h"
#include "stream-sender-internals.h"

#define STREAM_CIRCULAR_BUFFER_ADAPT_TO_TIMES_MAX_SIZE 3

struct stream_circular_buffer {
    struct circular_buffer *cb;
    STREAM_CIRCULAR_BUFFER_STATS stats;

    struct {
        size_t max_size;
        size_t buffer_used_percentage;          // the current utilization of the sending buffer
        usec_t since_ut;                        // the last time the sender flushed the sending buffer in microseconds
    } atomic;
};

STREAM_CIRCULAR_BUFFER *stream_circular_buffer_create(void) {
    STREAM_CIRCULAR_BUFFER *scb = callocz(1, sizeof(*scb));
    scb->cb = cbuffer_new(CBUFFER_INITIAL_SIZE, CBUFFER_INITIAL_MAX_SIZE, &netdata_buffers_statistics.cbuffers_streaming);
    scb->atomic.max_size = scb->cb->max_size;
    scb->atomic.buffer_used_percentage = 0;
    scb->atomic.since_ut = now_monotonic_usec();
    scb->stats.buffer_ratio = 0.0;
    return scb;
}

// returns true if it increased the buffer size
inline bool stream_circular_buffer_set_max_size_unsafe(STREAM_CIRCULAR_BUFFER *scb, size_t msg_size, bool force) {
    if(force || scb->cb->max_size > msg_size * STREAM_CIRCULAR_BUFFER_ADAPT_TO_TIMES_MAX_SIZE) {
        scb->cb->max_size = msg_size * STREAM_CIRCULAR_BUFFER_ADAPT_TO_TIMES_MAX_SIZE;
        __atomic_store_n(&scb->atomic.max_size, scb->cb->max_size, __ATOMIC_RELAXED);
        return true;
    }

    return false;
}

inline void stream_sender_set_buffer_used_percent(STREAM_CIRCULAR_BUFFER *scb, size_t value) {
    __atomic_store_n(&((scb)->atomic.buffer_used_percentage), value, __ATOMIC_RELAXED);
}

inline size_t stream_sender_get_buffer_used_percent(STREAM_CIRCULAR_BUFFER *scb) {
    return __atomic_load_n(&((scb)->atomic.buffer_used_percentage), __ATOMIC_RELAXED);
}

size_t stream_circular_buffer_get_max_size(STREAM_CIRCULAR_BUFFER *scb) {
    return __atomic_load_n(&scb->atomic.max_size, __ATOMIC_RELAXED);
}

void stream_circular_buffer_recreate_timed_unsafe(STREAM_CIRCULAR_BUFFER *scb, usec_t now_ut, bool force) {
    static __thread usec_t last_reset_time_ut = 0;

    if(!force && now_ut - last_reset_time_ut < 300 * USEC_PER_SEC)
        return;

    last_reset_time_ut = now_ut;

    scb->stats.recreates++; // we increase even if we don't do it, to have sender_start() recreate its buffers

    if(scb->cb && scb->cb->size > CBUFFER_INITIAL_SIZE) {
        cbuffer_free(scb->cb);
        scb->cb = cbuffer_new(CBUFFER_INITIAL_SIZE, stream_send.buffer_max_size, &netdata_buffers_statistics.cbuffers_streaming);
    }
}

inline usec_t stream_circular_buffer_get_since_ut(STREAM_CIRCULAR_BUFFER *scb) {
    return __atomic_load_n(&((scb)->atomic.since_ut), __ATOMIC_RELAXED);
}

void stream_circular_buffer_flush_unsafe(STREAM_CIRCULAR_BUFFER *scb) {
    __atomic_store_n(&((scb)->atomic.since_ut), now_monotonic_usec(), __ATOMIC_RELAXED);

    // flush the output buffer from any data it may have
    cbuffer_flush(scb->cb);
    stream_circular_buffer_recreate_timed_unsafe(scb, now_monotonic_usec(), true);
}

void stream_circular_buffer_destroy(STREAM_CIRCULAR_BUFFER *scb) {
    cbuffer_free(scb->cb);
    freez(scb);
}

// returns the current max size of the circular buffer
size_t stream_circular_buffer_get_max_size_unsafe(STREAM_CIRCULAR_BUFFER *scb) {
    return scb->cb->max_size;
}

// returns the current size of the circular buffer
size_t stream_circular_buffer_get_size_unsafe(STREAM_CIRCULAR_BUFFER *scb) {
    return scb->cb->size;
}

static inline void stream_circular_buffer_recalc(STREAM_CIRCULAR_BUFFER *scb) {
    scb->stats.bytes_max_size = scb->cb->max_size;
    scb->stats.bytes_size = scb->cb->size;
    scb->stats.bytes_outstanding = cbuffer_next_unsafe(scb->cb, NULL);
    scb->stats.bytes_available = cbuffer_available_size_unsafe(scb->cb);
    scb->stats.buffer_ratio = (double)(scb->cb->max_size -  scb->stats.bytes_available) * 100.0 / (double)scb->cb->max_size;
    stream_sender_set_buffer_used_percent(scb, (size_t)round(scb->stats.buffer_ratio));
}

// adds data to the circular buffer, returns false when it can't (buffer is full)
bool stream_circular_buffer_add_unsafe(STREAM_CIRCULAR_BUFFER *scb, const char *data, size_t bytes, size_t uncompressed) {
    stream_circular_buffer_set_max_size_unsafe(scb, uncompressed * STREAM_CIRCULAR_BUFFER_ADAPT_TO_TIMES_MAX_SIZE, false);

    scb->stats.adds++;
    scb->stats.bytes_added += bytes;
    bool rc = cbuffer_add_unsafe(scb->cb, data, bytes) == 0;
    if(rc)
        stream_circular_buffer_recalc(scb);
    return rc;
}

size_t stream_circular_buffer_get_unsafe(STREAM_CIRCULAR_BUFFER *scb, char **chunk) {
    return cbuffer_next_unsafe(scb->cb, chunk);
}

void stream_circular_buffer_del_unsafe(STREAM_CIRCULAR_BUFFER *scb, size_t bytes) {
    scb->stats.sends++;
    scb->stats.bytes_sent += bytes;
    cbuffer_remove_unsafe(scb->cb, bytes);
    stream_circular_buffer_recalc(scb);
}

STREAM_CIRCULAR_BUFFER_STATS stream_circular_buffer_stats_unsafe(STREAM_CIRCULAR_BUFFER *scb) {
    return scb->stats;
}

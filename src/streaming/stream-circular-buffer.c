// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream.h"
#include "stream-sender-internals.h"

struct stream_circular_buffer {
    struct circular_buffer *cb;
    STREAM_CIRCULAR_BUFFER_STATS stats;

    usec_t last_recreate_ut;            // recreates are only used to shrink the buffer, they are normal during operation
    usec_t last_sent_ut;                // the last time we removed or flushed data from the buffer

    struct {
        // the current max size of the buffer
        size_t max_size;

        // the current utilization of the buffer
        size_t buffer_ratio;

        // the last time we flushed the buffer
        // by monitoring this we can know if the system was reconnected
        usec_t last_flush_ut;
    } atomic;
};

static inline void stream_circular_buffer_stats_update_unsafe(STREAM_CIRCULAR_BUFFER *scb) {
    scb->stats.bytes_size = scb->cb->size;
    scb->stats.bytes_max_size = scb->cb->max_size;
    scb->stats.bytes_outstanding = cbuffer_next_unsafe(scb->cb, NULL);
    scb->stats.bytes_available = cbuffer_available_size_unsafe(scb->cb);
    scb->stats.buffer_ratio = (double)(scb->cb->max_size -  scb->stats.bytes_available) * 100.0 / (double)scb->cb->max_size;

    __atomic_store_n(&((scb)->atomic.buffer_ratio), (size_t)round(scb->stats.buffer_ratio), __ATOMIC_RELAXED);
}

STREAM_CIRCULAR_BUFFER *stream_circular_buffer_create(void) {
    STREAM_CIRCULAR_BUFFER *scb = callocz(1, sizeof(*scb));
    scb->cb = cbuffer_new(CBUFFER_INITIAL_SIZE, CBUFFER_INITIAL_MAX_SIZE, &netdata_buffers_statistics.cbuffers_streaming);
    stream_circular_buffer_stats_update_unsafe(scb);
    return scb;
}

// returns true if it increased the buffer size
bool stream_circular_buffer_set_max_size_unsafe(STREAM_CIRCULAR_BUFFER *scb, size_t max_size, bool force) {
    if(force || scb->cb->max_size < max_size) {
        scb->cb->max_size = max_size;
        scb->stats.bytes_max_size = scb->cb->max_size;
        __atomic_store_n(&scb->atomic.max_size, scb->cb->max_size, __ATOMIC_RELAXED);
        stream_circular_buffer_stats_update_unsafe(scb);
        return true;
    }

    return false;
}

void stream_circular_buffer_flush_unsafe(STREAM_CIRCULAR_BUFFER *scb, size_t buffer_max_size) {
    usec_t now_ut = now_monotonic_usec();
    __atomic_store_n(&scb->atomic.last_flush_ut, now_ut, __ATOMIC_RELAXED);

    // flush the output buffer from any data it may have
    scb->last_sent_ut = now_ut;
    cbuffer_flush(scb->cb);
    memset(&scb->stats, 0, sizeof(scb->stats));
    stream_circular_buffer_set_max_size_unsafe(scb, buffer_max_size, true);
    stream_circular_buffer_recreate_timed_unsafe(scb, now_monotonic_usec(), true);
}

inline size_t stream_sender_get_buffer_used_percent(STREAM_CIRCULAR_BUFFER *scb) {
    return __atomic_load_n(&scb->atomic.buffer_ratio, __ATOMIC_RELAXED);
}

size_t stream_circular_buffer_get_max_size(STREAM_CIRCULAR_BUFFER *scb) {
    return __atomic_load_n(&scb->atomic.max_size, __ATOMIC_RELAXED);
}

void stream_circular_buffer_recreate_timed_unsafe(STREAM_CIRCULAR_BUFFER *scb, usec_t now_ut, bool force) {
    if(!force && (scb->stats.bytes_outstanding || now_ut - scb->last_recreate_ut < 300 * USEC_PER_SEC))
        return;

    scb->last_recreate_ut = now_ut;

    scb->stats.recreates++; // we increase even if we don't do it, to have sender_start() recreate its buffers

    if(scb->cb && scb->cb->size > CBUFFER_INITIAL_SIZE) {
        size_t max_size = scb->cb->max_size;
        cbuffer_free(scb->cb);
        scb->cb = cbuffer_new(CBUFFER_INITIAL_SIZE, max_size, &netdata_buffers_statistics.cbuffers_streaming);
    }
}

inline usec_t stream_circular_buffer_last_flush_ut(STREAM_CIRCULAR_BUFFER *scb) {
    return __atomic_load_n(&((scb)->atomic.last_flush_ut), __ATOMIC_RELAXED);
}

inline usec_t stream_circular_buffer_last_sent_ut(STREAM_CIRCULAR_BUFFER *scb) {
    // this is ok without locks and atomics, since only the stream threads
    // can actually remove data and call this
    return scb->last_sent_ut;
}

void stream_circular_buffer_destroy(STREAM_CIRCULAR_BUFFER *scb) {
    if(!scb) return;
    cbuffer_free(scb->cb);
    freez(scb);
}

// adds data to the circular buffer, returns false when it can't (buffer is full)
bool stream_circular_buffer_add_unsafe(
    STREAM_CIRCULAR_BUFFER *scb, const char *data,
    size_t bytes_actual, size_t bytes_uncompressed, STREAM_TRAFFIC_TYPE type, bool autoscale) {
    scb->stats.adds++;
    scb->stats.bytes_added += bytes_actual;
    scb->stats.bytes_uncompressed += bytes_uncompressed;
    scb->stats.bytes_sent_by_type[type] += bytes_actual;

    if(unlikely(autoscale && cbuffer_available_size_unsafe(scb->cb) < bytes_actual))
        stream_circular_buffer_set_max_size_unsafe(scb, scb->cb->max_size * 2, true);

    if(unlikely(cbuffer_add_unsafe(scb->cb, data, bytes_actual) != 0))
        return false;

    stream_circular_buffer_stats_update_unsafe(scb);
    return true;
}

// return the first available chunk at the beginning of the buffer
size_t stream_circular_buffer_get_unsafe(STREAM_CIRCULAR_BUFFER *scb, char **chunk) {
    return cbuffer_next_unsafe(scb->cb, chunk);
}

// removes data from the beginning of the circular buffer
void stream_circular_buffer_del_unsafe(STREAM_CIRCULAR_BUFFER *scb, size_t bytes, usec_t now_ut) {
    scb->last_sent_ut = now_ut ? now_ut : now_monotonic_usec();
    scb->stats.sends++;
    scb->stats.bytes_sent += bytes;
    cbuffer_remove_unsafe(scb->cb, bytes);
    stream_circular_buffer_stats_update_unsafe(scb);
}

// returns a copy of the current circular buffer statistics
STREAM_CIRCULAR_BUFFER_STATS *stream_circular_buffer_stats_unsafe(STREAM_CIRCULAR_BUFFER *scb) {
    return &scb->stats;
}

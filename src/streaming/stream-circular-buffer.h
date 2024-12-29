// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_CIRCULAR_BUFFER_H
#define NETDATA_STREAM_CIRCULAR_BUFFER_H

#include "libnetdata/libnetdata.h"
#include "stream-traffic-types.h"

#ifdef __cplusplus
extern "C" {
#endif

#define CBUFFER_INITIAL_SIZE (16 * 1024)
#define CBUFFER_INITIAL_MAX_SIZE (10 * 1024 * 1024)
#define THREAD_BUFFER_INITIAL_SIZE (8192)

#define STREAM_CIRCULAR_BUFFER_ADAPT_TO_TIMES_MAX_SIZE 3

typedef struct stream_circular_buffer_stats {
    size_t adds;
    size_t sends;
    size_t recreates;

    size_t bytes_added;
    size_t bytes_uncompressed;
    size_t bytes_sent;

    uint32_t bytes_size;
    uint32_t bytes_max_size;
    uint32_t bytes_outstanding;
    uint32_t bytes_available;

    double buffer_ratio;

    size_t bytes_sent_by_type[STREAM_TRAFFIC_TYPE_MAX];
} STREAM_CIRCULAR_BUFFER_STATS;

struct stream_circular_buffer;
typedef struct stream_circular_buffer STREAM_CIRCULAR_BUFFER;

// --------------------------------------------------------------------------------------------------------------------
// management

STREAM_CIRCULAR_BUFFER *stream_circular_buffer_create(void);
void stream_circular_buffer_destroy(STREAM_CIRCULAR_BUFFER *scb);

// flushes all data in the buffer
void stream_circular_buffer_flush_unsafe(STREAM_CIRCULAR_BUFFER *scb, size_t buffer_max_size);

// recreates the buffer, but it does so every 5 minutes and only if the buffer has no data in it
// it does not alter the since_ut time of the buffer, so this is assumed to be the same session
// use this after deleting data from the buffer, to minimize the memory footprint of the buffer
void stream_circular_buffer_recreate_timed_unsafe(STREAM_CIRCULAR_BUFFER *scb, usec_t now_ut, bool force);

// returns true if it increased the buffer size
// if it changes the size, it updates the statistics
bool stream_circular_buffer_set_max_size_unsafe(STREAM_CIRCULAR_BUFFER *scb, size_t max_size, bool force);

// returns a pointer to the current circular buffer statistics
// copy it if you plan to use it without a lock
STREAM_CIRCULAR_BUFFER_STATS *stream_circular_buffer_stats_unsafe(STREAM_CIRCULAR_BUFFER *scb);

// --------------------------------------------------------------------------------------------------------------------
// atomic operations - no lock needed

// returns the max size of the buffer in bytes
size_t stream_circular_buffer_get_max_size(STREAM_CIRCULAR_BUFFER *scb);

// returns the current buffer used ratio
size_t stream_sender_get_buffer_used_percent(STREAM_CIRCULAR_BUFFER *scb);

// return the monotonic timestamp of the last time the buffer was created
usec_t stream_circular_buffer_last_flush_ut(STREAM_CIRCULAR_BUFFER *scb);

// return the monotonic timestamp of the last time we removed data from the buffer
usec_t stream_circular_buffer_last_sent_ut(STREAM_CIRCULAR_BUFFER *scb);

// --------------------------------------------------------------------------------------------------------------------
// data operations (add, get, remove data from/to the buffer)

// adds data to the end of the circular buffer, returns false when it can't (buffer is full)
// it updates the statistics
bool stream_circular_buffer_add_unsafe(
    STREAM_CIRCULAR_BUFFER *scb, const char *data, size_t bytes_actual, size_t bytes_uncompressed,
    STREAM_TRAFFIC_TYPE type, bool autoscale);

// returns a pointer to the beginning of the buffer, and its size in bytes
size_t stream_circular_buffer_get_unsafe(STREAM_CIRCULAR_BUFFER *scb, char **chunk);

// removes data from the beginning of circular buffer
// it updates the statistics
void stream_circular_buffer_del_unsafe(STREAM_CIRCULAR_BUFFER *scb, size_t bytes, usec_t now_ut);

#ifdef __cplusplus
}
#endif

#endif //NETDATA_STREAM_CIRCULAR_BUFFER_H

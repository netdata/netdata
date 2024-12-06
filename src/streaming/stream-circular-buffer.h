// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_CIRCULAR_BUFFER_H
#define NETDATA_STREAM_CIRCULAR_BUFFER_H

#include "libnetdata/libnetdata.h"

#define CBUFFER_INITIAL_SIZE (65536 * 1024)
#define CBUFFER_INITIAL_MAX_SIZE (10 * 1024 * 1024)
#define THREAD_BUFFER_INITIAL_SIZE (8192)

typedef struct stream_circular_buffer_stats {
    size_t adds;
    size_t sends;
    size_t recreates;

    size_t bytes_added;
    size_t bytes_uncompressed;

    size_t bytes_sent;

    size_t bytes_size;
    size_t bytes_max_size;
    size_t bytes_outstanding;
    size_t bytes_available;

    double buffer_ratio;
} STREAM_CIRCULAR_BUFFER_STATS;

struct stream_circular_buffer;
typedef struct stream_circular_buffer STREAM_CIRCULAR_BUFFER;

size_t stream_circular_buffer_get_max_size(STREAM_CIRCULAR_BUFFER *scb);

void stream_circular_buffer_recreate_timed_unsafe(STREAM_CIRCULAR_BUFFER *scb, usec_t now_ut, bool force);
void stream_circular_buffer_flush_unsafe(STREAM_CIRCULAR_BUFFER *scb);

usec_t stream_circular_buffer_get_since_ut(STREAM_CIRCULAR_BUFFER *scb);

void stream_sender_set_buffer_used_percent(STREAM_CIRCULAR_BUFFER *scb, size_t value);
size_t stream_sender_get_buffer_used_percent(STREAM_CIRCULAR_BUFFER *scb);

// returns true if it increased the buffer size
bool stream_circular_buffer_set_max_size_unsafe(STREAM_CIRCULAR_BUFFER *scb, size_t msg_size, bool force);

// returns the current max size of the circular buffer
size_t stream_circular_buffer_get_max_size_unsafe(STREAM_CIRCULAR_BUFFER *scb);

// returns the current size of the circular buffer
size_t stream_circular_buffer_get_size_unsafe(STREAM_CIRCULAR_BUFFER *scb);

// adds data to the end of the circular buffer, returns false when it can't (buffer is full)
bool stream_circular_buffer_add_unsafe(STREAM_CIRCULAR_BUFFER *scb, const char *data, size_t bytes, size_t uncompressed);

// returns a pointer to the beginning of the buffer, and its size in bytes
size_t stream_circular_buffer_get_unsafe(STREAM_CIRCULAR_BUFFER *scb, char **chunk);

// removes data from the beginning of circular buffer
void stream_circular_buffer_del_unsafe(STREAM_CIRCULAR_BUFFER *scb, size_t bytes);


STREAM_CIRCULAR_BUFFER *stream_circular_buffer_create(void);
void stream_circular_buffer_destroy(STREAM_CIRCULAR_BUFFER *scb);

STREAM_CIRCULAR_BUFFER_STATS stream_circular_buffer_stats_unsafe(STREAM_CIRCULAR_BUFFER *scb);

#endif //NETDATA_STREAM_CIRCULAR_BUFFER_H

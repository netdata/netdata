// Copyright: SPDX-License-Identifier:  GPL-3.0-only

#ifndef CRINGBUFFER_INTERNAL_H
#define CRINGBUFFER_INTERNAL_H

struct rbuf_t {
    char *data;

    // points to next byte where we can write
    char *head;
    // points to oldest (next to be poped) readable byte
    char *tail;

    // to avoid calculating data + size
    // all the time
    char *end;

    size_t size;
    size_t size_data;
};

/* this exists so that it can be tested by unit tests
 * without optimization that resets head and tail to
 * beginning if buffer empty
 */
inline static int rbuf_bump_tail_noopt(rbuf_t buffer, size_t bytes)
{
    if (bytes > buffer->size_data)
        return 0;
    int i = buffer->tail - buffer->data;
    buffer->tail = &buffer->data[(i + bytes) % buffer->size];
    buffer->size_data -= bytes;

    return 1;
}

#endif

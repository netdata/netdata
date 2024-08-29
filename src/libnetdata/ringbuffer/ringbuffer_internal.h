// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef RINGBUFFER_INTERNAL_H
#define RINGBUFFER_INTERNAL_H

#include "ringbuffer.h"

struct rbuf {
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

typedef struct rbuf *rbuf_t;

#endif

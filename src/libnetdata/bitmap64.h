// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_BITMAP64_H
#define NETDATA_BITMAP64_H

#include <stdbool.h>
#include <stdint.h>
#include <assert.h>

typedef uint64_t bitmap64_t;

#define BITMAP64_INITIALIZER 0

static inline void bitmap64_set(bitmap64_t *bitmap, int position)
{
    assert(position >= 0 && position < 64);

    *bitmap |= (1ULL << position);
}

static inline void bitmap64_clear(bitmap64_t *bitmap, int position)
{
    assert(position >= 0 && position < 64);

    *bitmap &= ~(1ULL << position);
}

static inline bool bitmap64_get(const bitmap64_t *bitmap, int position)
{
    assert(position >= 0 && position < 64);

    return (*bitmap & (1ULL << position));
}

#endif // NETDATA_BITMAP64_H

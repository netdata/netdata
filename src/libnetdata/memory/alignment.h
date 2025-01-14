// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ALIGNMENT_H
#define NETDATA_ALIGNMENT_H

#include "../common.h"

#if ENV32BIT
#define SYSTEM_REQUIRED_ALIGNMENT (sizeof(uintptr_t) * 2)
#else
#define SYSTEM_REQUIRED_ALIGNMENT (alignof(uintptr_t))
#endif

static inline size_t memory_alignment(size_t size, size_t alignment) __attribute__((const));
static inline size_t memory_alignment(size_t size, size_t alignment) {
    // return (size + alignment - 1) & ~(alignment - 1); // assumes alignment is power of 2
    return ((size + alignment - 1) / alignment) * alignment;
}

static inline size_t natural_alignment(size_t size) __attribute__((const));
static inline size_t natural_alignment(size_t size) {
    return memory_alignment(size, SYSTEM_REQUIRED_ALIGNMENT);
}

#endif //NETDATA_ALIGNMENT_H

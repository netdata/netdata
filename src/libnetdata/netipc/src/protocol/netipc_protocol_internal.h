#ifndef NETIPC_PROTOCOL_INTERNAL_H
#define NETIPC_PROTOCOL_INTERNAL_H

#include "netipc/netipc_protocol.h"

#include <stddef.h>
#include <string.h>

/*
 * Safe multiplication check: returns true if count * entry_size would
 * overflow size_t. Portable across 32-bit and 64-bit without triggering
 * -Wtype-limits.
 */
static inline bool mul_would_overflow(size_t count, size_t entry_size)
{
    return entry_size != 0 && count > SIZE_MAX / entry_size;
}

#endif /* NETIPC_PROTOCOL_INTERNAL_H */

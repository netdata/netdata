// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDSET_PLUGINSD_ARRAY_H
#define NETDATA_RRDSET_PLUGINSD_ARRAY_H

// This header must be included AFTER rrddim.h to get the full struct pluginsd_rrddim definition

#include "rrddim.h"

// --------------------------------------------------------------------------------------------------------------------
// Reference-counted array for pluginsd dimension caching
//
// This structure provides thread-safe access to the dimension cache array used by the pluginsd protocol.
// The reference counting ensures that:
// 1. The array is not freed while any thread is still using it
// 2. Concurrent readers can safely iterate without locks
// 3. Writers can atomically replace the array without blocking readers
// --------------------------------------------------------------------------------------------------------------------

typedef struct pluginsd_rrddim_array {
    int32_t refcount;                    // Reference count (atomic)
    size_t size;                         // Number of entries in the array
    struct pluginsd_rrddim entries[];    // Flexible array member
} PRD_ARRAY;

// --------------------------------------------------------------------------------------------------------------------
// API Functions
// --------------------------------------------------------------------------------------------------------------------

// Create a new array with the specified size and refcount=1
static inline PRD_ARRAY *prd_array_create(size_t size) {
    PRD_ARRAY *arr = callocz(1, sizeof(PRD_ARRAY) + size * sizeof(struct pluginsd_rrddim));
    arr->refcount = 1;
    arr->size = size;
    return arr;
}

// Acquire a reference to the array stored in the atomic pointer location
// Returns NULL if no array exists or if the array is being freed (refcount <= 0)
// The caller MUST call prd_array_release() when done
static inline PRD_ARRAY *prd_array_acquire(PRD_ARRAY **array_ptr) {
    PRD_ARRAY *arr;
    int32_t refcount;

    do {
        arr = __atomic_load_n(array_ptr, __ATOMIC_ACQUIRE);
        if (!arr)
            return NULL;

        refcount = __atomic_load_n(&arr->refcount, __ATOMIC_ACQUIRE);
        if (refcount <= 0)
            return NULL;  // Array is being freed

        // Try to atomically increment the refcount
        // If it changed between load and CAS, retry
    } while (!__atomic_compare_exchange_n(&arr->refcount, &refcount, refcount + 1,
                                          false, __ATOMIC_ACQ_REL, __ATOMIC_ACQUIRE));

    return arr;
}

// Release a reference to the array
// If this was the last reference (refcount becomes 0), the array is freed
// Safe to call with NULL
static inline void prd_array_release(PRD_ARRAY *arr) {
    if (!arr)
        return;

    int32_t old_refcount = __atomic_fetch_sub(&arr->refcount, 1, __ATOMIC_ACQ_REL);

    if (old_refcount == 1) {
        // We were the last reference - free the array
        // Note: The caller is responsible for releasing any RRDDIM_ACQUIRED references
        // in the entries before the final release
        freez(arr);
    }
}

// Atomically replace the array pointer with a new array
// Returns the old array (caller must release it) or NULL if there was no old array
// The new_arr can be NULL to clear the array
static inline PRD_ARRAY *prd_array_replace(PRD_ARRAY **array_ptr, PRD_ARRAY *new_arr) {
    return __atomic_exchange_n(array_ptr, new_arr, __ATOMIC_ACQ_REL);
}

// Get the current array without acquiring a reference (for quick NULL checks)
// WARNING: The returned pointer may become invalid at any time
// Only use this for NULL checks, not for accessing array contents
static inline PRD_ARRAY *prd_array_get_unsafe(PRD_ARRAY **array_ptr) {
    return __atomic_load_n(array_ptr, __ATOMIC_ACQUIRE);
}

#endif // NETDATA_RRDSET_PLUGINSD_ARRAY_H

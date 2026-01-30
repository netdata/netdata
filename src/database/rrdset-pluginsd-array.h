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
//
// IMPORTANT: prd_array_acquire() requires a spinlock to prevent a race condition where another thread
// could replace and free the array between loading the pointer and incrementing the refcount.
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
// Returns NULL if no array exists
// The caller MUST call prd_array_release() when done
//
// IMPORTANT: This function requires a spinlock to prevent use-after-free:
// Without the lock, between loading the array pointer and incrementing its refcount,
// another thread could replace the pointer and free the old array.
static inline PRD_ARRAY *prd_array_acquire(PRD_ARRAY **array_ptr, SPINLOCK *spinlock) {
    spinlock_lock(spinlock);

    PRD_ARRAY *arr = *array_ptr;
    if (arr) {
        // Safe to access arr->refcount because we hold the spinlock
        // and any replace+release operation also needs this lock
        __atomic_fetch_add(&arr->refcount, 1, __ATOMIC_ACQ_REL);
    }

    spinlock_unlock(spinlock);
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
//
// IMPORTANT: When the returned old array will be released (potentially freeing it),
// the caller MUST hold the same spinlock used by prd_array_acquire() to prevent races.
static inline PRD_ARRAY *prd_array_replace(PRD_ARRAY **array_ptr, PRD_ARRAY *new_arr) {
    return __atomic_exchange_n(array_ptr, new_arr, __ATOMIC_ACQ_REL);
}

// Get the current array without acquiring a reference (for quick NULL checks or
// when external synchronization guarantees the array won't be freed)
// WARNING: The returned pointer may become invalid at any time unless:
// - The caller holds the spinlock, OR
// - The caller is the collector thread with collector_tid set (preventing cleanup)
static inline PRD_ARRAY *prd_array_get_unsafe(PRD_ARRAY **array_ptr) {
    return __atomic_load_n(array_ptr, __ATOMIC_ACQUIRE);
}

#endif // NETDATA_RRDSET_PLUGINSD_ARRAY_H

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDSET_PLUGINSD_ARRAY_H
#define NETDATA_RRDSET_PLUGINSD_ARRAY_H

// This header must be included AFTER rrddim.h to get the full struct pluginsd_rrddim definition

#include "rrddim.h"

// --------------------------------------------------------------------------------------------------------------------
// Reference-counted array for pluginsd dimension caching
//
// This structure provides thread-safe access to the dimension cache array used by the pluginsd protocol.
// The reference counting ensures that the array is not freed while any thread is still using it.
//
// THREAD SAFETY - LIFECYCLE SEPARATION:
// -------------------------------------
// The design relies on collector and cleanup never running concurrently on the same chart:
//
// 1. collector_tid: Primary synchronization mechanism
//    - Collector sets collector_tid BEFORE accessing the array
//    - Collector clears collector_tid AFTER all operations are complete
//    - Cleanup code checks collector_tid and SKIPS if non-zero
//    - This allows the collector to use lock-free operations (get_unsafe, replace, release)
//
// 2. spinlock + refcount: Coordinates concurrent cleanup operations
//    - prd_array_acquire(): Takes spinlock, loads pointer, increments refcount
//    - Used by cleanup code when collector is NOT active
//    - Prevents races between multiple cleanup threads
//
// 3. Lifecycle guarantee: In production, cleanup only runs when:
//    - Stream receiver is stopped (collector thread terminated)
//    - collector_tid is explicitly cleared before cleanup
//    - Therefore, collector's replace+release never races with cleanup's acquire
//
// HOT PATH (collector active, collector_tid set): Lock-free
// CLEANUP PATH (collector stopped, collector_tid == 0): Uses spinlock
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
    rrd_slot_memory_added(sizeof(PRD_ARRAY) + size * sizeof(struct pluginsd_rrddim));
    return arr;
}

// Acquire a reference to the array when spinlock is ALREADY HELD
// Returns NULL if no array exists
// The caller MUST call prd_array_release() when done
// Use this when you need to do additional checks (e.g., collector_tid) under the same spinlock
static inline PRD_ARRAY *prd_array_acquire_locked(PRD_ARRAY **array_ptr) {
    PRD_ARRAY *arr = *array_ptr;
    if (arr) {
        __atomic_fetch_add(&arr->refcount, 1, __ATOMIC_ACQ_REL);
    }
    return arr;
}

// Acquire a reference to the array stored in the atomic pointer location
// Returns NULL if no array exists
// The caller MUST call prd_array_release() when done
//
// IMPORTANT: Only call this when collector_tid == 0 (collector not active).
// Uses spinlock to coordinate with other cleanup operations.
static inline PRD_ARRAY *prd_array_acquire(PRD_ARRAY **array_ptr, SPINLOCK *spinlock) {
    spinlock_lock(spinlock);
    PRD_ARRAY *arr = prd_array_acquire_locked(array_ptr);
    spinlock_unlock(spinlock);
    return arr;
}

// Release a reference to the array
// If this was the last reference (refcount becomes 0), the array is freed
// Safe to call with NULL
static inline void prd_array_release(PRD_ARRAY *arr) {
    if (!arr)
        return;

    int32_t old_refcount = __atomic_load_n(&arr->refcount, __ATOMIC_ACQUIRE);
    while(true) {
        if(unlikely(old_refcount <= 0)) {
            // Keep the object stable and avoid driving refcount further negative on
            // repeated misuse. Log in all builds; internal_fatal adds extra checks.
            nd_log_limit_static_global_var(erl_prd_refcount_underflow, 1, 0);
            nd_log_limit(&erl_prd_refcount_underflow, NDLS_DAEMON, NDLP_WARNING,
                         "PRD_ARRAY: refcount underflow (was %d) - double release detected",
                         old_refcount);
            internal_fatal(true,
                           "PRD_ARRAY: refcount underflow (was %d) - double release detected", old_refcount);
            return;
        }

        if(__atomic_compare_exchange_n(&arr->refcount, &old_refcount, old_refcount - 1,
                                       false, __ATOMIC_ACQ_REL, __ATOMIC_ACQUIRE))
            break;
    }

    if(old_refcount == 1) {
        // We were the last reference - free the array
        // Note: The caller is responsible for releasing any RRDDIM_ACQUIRED references
        // in the entries before the final release
        rrd_slot_memory_removed(sizeof(PRD_ARRAY) + arr->size * sizeof(struct pluginsd_rrddim));
        freez(arr);
    }
}

// Atomically replace the array pointer with a new array
// Returns the old array (caller must release it) or NULL if there was no old array
// The new_arr can be NULL to clear the array
//
// Thread safety depends on context:
// - Collector (collector_tid set): No spinlock needed - cleanup will skip
// - Cleanup (collector_tid == 0): Should hold spinlock to coordinate with other cleanup
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

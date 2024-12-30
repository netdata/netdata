// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

#define WRITER_LOCKED (-65536)

// ----------------------------------------------------------------------------
// rw_spinlock implementation

void rw_spinlock_init_with_trace(RW_SPINLOCK *rw_spinlock, const char *func __maybe_unused) {
    rw_spinlock->writer = 0;
    rw_spinlock->counter = 0;
}

bool rw_spinlock_tryread_lock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func) {
    size_t spins = 0;

    REFCOUNT expected = rw_spinlock->counter;
    while (true) {
        if(expected == WRITER_LOCKED)
            // writer is active
            return false;

        if(expected < 0)
            fatal("RW_SPINLOCK: refcount found negative, on %s(), called from %s()", __FUNCTION__, func);

        // increment reader count
        if (__atomic_compare_exchange_n(
                &rw_spinlock->counter,
                &expected,
                expected + 1,
                false,              // Strong CAS
                __ATOMIC_ACQUIRE,   // Success memory order
                __ATOMIC_RELAXED    // Failure memory order
                ))
            break;

        spins++;
    }

    worker_spinlock_contention(func, spins);
    nd_thread_rwspinlock_read_locked();
    return true;
}

void rw_spinlock_read_lock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func) {
    size_t spins = 0;

    REFCOUNT expected = rw_spinlock->counter;

    // we should not increase it if it is negative (a writer holds the lock)
    if(expected == WRITER_LOCKED) expected = 0;

    while (true) {
        if(expected < 0)
            fatal("RW_SPINLOCK: refcount found negative, on %s(), called from %s()", __FUNCTION__, func);

        // Attempt to increment reader count
        if (__atomic_compare_exchange_n(
                &rw_spinlock->counter,
                &expected,
                expected + 1,
                false,              // Strong CAS
                __ATOMIC_ACQUIRE,   // Success memory order
                __ATOMIC_RELAXED    // Failure memory order
                ))
            break;

        spins++;

        if (expected == WRITER_LOCKED) {
            // writer is active

            // we should not increase it if it is negative (a writer holds the lock)
            expected = 0;

            // wait a bit before retrying
            tinysleep();
            yield_the_processor();
        }
    }

    worker_spinlock_contention(func, spins);
    nd_thread_rwspinlock_read_locked();
}

void rw_spinlock_read_unlock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func __maybe_unused) {
    REFCOUNT x = __atomic_sub_fetch(&rw_spinlock->counter, 1, __ATOMIC_RELEASE);
    if (x < 0)
        fatal("RW_SPINLOCK: readers is negative %d, on %s called from %s()", x, __FUNCTION__, func);

    nd_thread_rwspinlock_read_unlocked();
}

bool rw_spinlock_trywrite_lock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func) {
    REFCOUNT expected = 0;

    // Attempt to acquire writer lock when no readers or writers are active
    if (!__atomic_compare_exchange_n(
            &rw_spinlock->counter,
            &expected,
            WRITER_LOCKED,
            false,              // Strong CAS
            __ATOMIC_ACQUIRE,   // Success memory order
            __ATOMIC_RELAXED    // Failure memory order
            )) {
        return false;
    }

    __atomic_store_n(&rw_spinlock->writer, gettid_cached(), __ATOMIC_RELAXED);
    worker_spinlock_contention(func, 0);
    nd_thread_rwspinlock_write_locked();
    return true;
}

void rw_spinlock_write_lock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func) {
    size_t spins = 0;

    while (true) {
        REFCOUNT expected = 0;

        // Attempt to acquire writer lock when no readers or writers are active
        if (__atomic_compare_exchange_n(
                &rw_spinlock->counter,
                &expected,
                WRITER_LOCKED,
                false,              // Strong CAS
                __ATOMIC_ACQUIRE,   // Success memory order
                __ATOMIC_RELAXED    // Failure memory order
                )) {
            break;
        }

        spins++;
        tinysleep();
    }

    __atomic_store_n(&rw_spinlock->writer, gettid_cached(), __ATOMIC_RELAXED);
    worker_spinlock_contention(func, spins);
    nd_thread_rwspinlock_write_locked();
}

void rw_spinlock_write_unlock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func __maybe_unused) {
#ifdef NETDATA_INTERNAL_CHECKS
    int32_t x = __atomic_load_n(&rw_spinlock->counter, __ATOMIC_RELAXED);
    if (x != WRITER_LOCKED)
        fatal("RW_SPINLOCK: writer unlock encountered unexpected state: %d, on %s() called from %s()", x, __FUNCTION__, func);
#endif

    __atomic_store_n(&rw_spinlock->writer, 0, __ATOMIC_RELAXED);
    __atomic_store_n(&rw_spinlock->counter, 0, __ATOMIC_RELEASE); // Release writer lock
    nd_thread_rwspinlock_write_unlocked();
}

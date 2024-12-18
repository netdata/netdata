// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

// ----------------------------------------------------------------------------
// rw_spinlock implementation

void rw_spinlock_init_with_trace(RW_SPINLOCK *rw_spinlock, const char *func __maybe_unused) {
    rw_spinlock->counter = 0;
}

bool rw_spinlock_tryread_lock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func) {
    size_t spins = 0;
    while (true) {
        int32_t count = __atomic_load_n(&rw_spinlock->counter, __ATOMIC_RELAXED);
        if (count == -1) {
            // Writer is active
            return false;
        }

        // Attempt to increment reader count
        if (__atomic_compare_exchange_n(
                &rw_spinlock->counter,
                &count,
                count + 1,
                false, // Strong CAS
                __ATOMIC_ACQUIRE, // Success memory order
                __ATOMIC_RELAXED  // Failure memory order
                )) {
            break;
        }
    }

    worker_spinlock_contention(func, spins);
    nd_thread_rwspinlock_read_locked();

    return true;
}

void rw_spinlock_read_lock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func) {
    size_t spins = 0;
    usec_t usec = 1;
    while (true) {
        int32_t count = __atomic_load_n(&rw_spinlock->counter, __ATOMIC_RELAXED);
        if (count == -1) {
            // Writer is active, spin
            spins++;
            microsleep(usec);
            usec *= 2;
            continue;
        }

        // Attempt to increment reader count
        if (__atomic_compare_exchange_n(
                &rw_spinlock->counter,
                &count,
                count + 1,
                false, // Strong CAS
                __ATOMIC_ACQUIRE, // Success memory order
                __ATOMIC_RELAXED  // Failure memory order
                )) {
            break;
        }
    }

    worker_spinlock_contention(func, spins);
    nd_thread_rwspinlock_read_locked();
}

void rw_spinlock_read_unlock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func __maybe_unused) {
#ifndef NETDATA_INTERNAL_CHECKS
    __atomic_sub_fetch(&rw_spinlock->counter, 1, __ATOMIC_RELEASE);
#else
    int32_t x = __atomic_sub_fetch(&rw_spinlock->counter, 1, __ATOMIC_RELEASE);
    if(x < 0)
        fatal("RW_SPINLOCK: readers is negative %d", x);
#endif

    nd_thread_rwspinlock_read_unlocked();
}

bool rw_spinlock_trywrite_lock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func) {
    int32_t expected = 0;

    // Attempt to acquire writer lock when no readers or writers are active
    if (!__atomic_compare_exchange_n(
            &rw_spinlock->counter,
            &expected,
            -1,
            false, // Strong CAS
            __ATOMIC_ACQUIRE, // Success memory order
            __ATOMIC_RELAXED  // Failure memory order
            ))
        return false;

    worker_spinlock_contention(func, 0);
    nd_thread_rwspinlock_write_locked();
    return true;
}

void rw_spinlock_write_lock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func) {
    size_t spins = 0;

    while (true) {
        int32_t expected = 0;

        // Attempt to acquire writer lock when no readers or writers are active
        if (__atomic_compare_exchange_n(
                &rw_spinlock->counter,
                &expected,
                -1,
                false, // Strong CAS
                __ATOMIC_ACQUIRE, // Success memory order
                __ATOMIC_RELAXED  // Failure memory order
                )) {
            break;
        }

        // Spin if readers or another writer is active
        spins++;
    }

    worker_spinlock_contention(func, spins);
    nd_thread_rwspinlock_write_locked();
}

void rw_spinlock_write_unlock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func __maybe_unused) {
    __atomic_store_n(&rw_spinlock->counter, 0, __ATOMIC_RELEASE); // Release writer lock
    nd_thread_rwspinlock_write_unlocked();
}

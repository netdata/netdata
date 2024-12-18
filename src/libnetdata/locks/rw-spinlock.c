// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

#define MAX_USEC 1024 // Maximum backoff limit in microseconds
#define SPIN_THRESHOLD 10 // Spins before introducing sleep

static __thread int32_t locks_held_by_thread = 0; // Thread-local counter for locks held

// ----------------------------------------------------------------------------
// rw_spinlock implementation

void rw_spinlock_init_with_trace(RW_SPINLOCK *rw_spinlock, const char *func __maybe_unused) {
    rw_spinlock->counter = 0;
    rw_spinlock->writers_waiting = false;
}

bool rw_spinlock_tryread_lock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func) {
    size_t spins = 0;

    int32_t count = __atomic_load_n(&rw_spinlock->counter, __ATOMIC_RELAXED);
    if (count == -1 || (!locks_held_by_thread && __atomic_load_n(&rw_spinlock->writers_waiting, __ATOMIC_RELAXED))) {
        // Writer is active or waiting
        return false;
    }

    // Attempt to increment reader count
    if (!__atomic_compare_exchange_n(
            &rw_spinlock->counter,
            &count,
            count + 1,
            false, // Strong CAS
            __ATOMIC_ACQUIRE, // Success memory order
            __ATOMIC_RELAXED  // Failure memory order
            ))
        return false;

    locks_held_by_thread++;
    worker_spinlock_contention(func, spins);
    nd_thread_rwspinlock_read_locked();

    return true;
}

void rw_spinlock_read_lock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func) {
    size_t spins = 0;
    usec_t usec = 1;

    while (true) {
        int32_t count = __atomic_load_n(&rw_spinlock->counter, __ATOMIC_RELAXED);
        if (count == -1 || (!locks_held_by_thread && __atomic_load_n(&rw_spinlock->writers_waiting, __ATOMIC_RELAXED))) {
            // Writer is active or waiting, spin
            spins++;
            microsleep(usec);
            usec = usec >= MAX_USEC ? MAX_USEC : usec * 2;
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
                ))
            break;

        if (++spins > SPIN_THRESHOLD)
            tinysleep();
    }

    locks_held_by_thread++;
    worker_spinlock_contention(func, spins);
    nd_thread_rwspinlock_read_locked();
}

void rw_spinlock_read_unlock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func __maybe_unused) {
#ifndef NETDATA_INTERNAL_CHECKS
    __atomic_sub_fetch(&rw_spinlock->counter, 1, __ATOMIC_RELEASE);
#else
    int32_t x = __atomic_sub_fetch(&rw_spinlock->counter, 1, __ATOMIC_RELEASE);
    if (x < 0)
        fatal("RW_SPINLOCK: readers is negative %d", x);
#endif

    locks_held_by_thread--;
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
            )) {
        return false;
    }

    worker_spinlock_contention(func, 0);
    nd_thread_rwspinlock_write_locked();
    return true;
}

void rw_spinlock_write_lock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func) {
    size_t spins = 0;

    __atomic_add_fetch(&rw_spinlock->writers_waiting, 1, __ATOMIC_RELAXED);

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
        if (++spins > SPIN_THRESHOLD)
            tinysleep();
    }

    __atomic_sub_fetch(&rw_spinlock->writers_waiting, 1, __ATOMIC_RELAXED);
    worker_spinlock_contention(func, spins);
    nd_thread_rwspinlock_write_locked();
}

void rw_spinlock_write_unlock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func __maybe_unused) {
#ifdef NETDATA_INTERNAL_CHECKS
    int32_t x = __atomic_load_n(&rw_spinlock->counter, __ATOMIC_RELAXED);
    if (x != -1)
        fatal("RW_SPINLOCK: writer unlock encountered unexpected state: %d", x);
#endif

    __atomic_store_n(&rw_spinlock->counter, 0, __ATOMIC_RELEASE); // Release writer lock
    nd_thread_rwspinlock_write_unlocked();
}

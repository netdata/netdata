// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

#define MAX_USEC 512 // Maximum backoff limit in microseconds

// ----------------------------------------------------------------------------
// spinlock implementation
// https://www.youtube.com/watch?v=rmGJc9PXpuE&t=41s

#ifndef SPINLOCK_IMPL_WITH_MUTEX

ALWAYS_INLINE void spinlock_init_with_trace(SPINLOCK *spinlock, const char *func __maybe_unused) {
    memset(spinlock, 0, sizeof(SPINLOCK));
}

ALWAYS_INLINE void spinlock_lock_with_trace(SPINLOCK *spinlock, const char *func) {
    size_t spins = 0;
    usec_t usec = 1;

    while (true) {
        if (!__atomic_load_n(&spinlock->locked, __ATOMIC_RELAXED) &&
            !__atomic_test_and_set(&spinlock->locked, __ATOMIC_ACQUIRE)) {
            // Acquired the lock
            break;
        }

        // Backoff strategy with exponential growth
        spins++;
        microsleep(usec);
        usec = usec >= MAX_USEC ? MAX_USEC : usec * 2;
    }

#ifdef NETDATA_INTERNAL_CHECKS
    spinlock->spins += spins;
    spinlock->locker_pid = gettid_cached();
#endif

    nd_thread_spinlock_locked();
    worker_spinlock_contention(func, spins);
}

ALWAYS_INLINE void spinlock_unlock_with_trace(SPINLOCK *spinlock, const char *func __maybe_unused) {
#ifdef NETDATA_INTERNAL_CHECKS
    spinlock->locker_pid = 0;
#endif

    __atomic_clear(&spinlock->locked, __ATOMIC_RELEASE);

    nd_thread_spinlock_unlocked();
}

ALWAYS_INLINE bool spinlock_trylock_with_trace(SPINLOCK *spinlock, const char *func __maybe_unused) {
    if (!__atomic_load_n(&spinlock->locked, __ATOMIC_RELAXED) &&
        !__atomic_test_and_set(&spinlock->locked, __ATOMIC_ACQUIRE)) {
        // Acquired the lock
        nd_thread_spinlock_locked();
        return true;
    }

    return false;
}

#endif // SPINLOCK_IMPL_WITH_MUTEX

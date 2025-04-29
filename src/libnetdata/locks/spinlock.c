// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

#define MAX_USEC 512 // Maximum backoff limit in microseconds

// ----------------------------------------------------------------------------
// Deadlock detection function
void spinlock_deadlock_detect(usec_t *timestamp, const char *type, const char *func) {
    if (!*timestamp) {
        // First time checking - initialize the timestamp
        *timestamp = now_monotonic_high_precision_usec();
        return;
    }

    // Check if we've exceeded the timeout
    usec_t now = now_monotonic_high_precision_usec();
    if (now - *timestamp >= SPINLOCK_DEADLOCK_TIMEOUT_SEC * USEC_PER_SEC) {
        // We've been spinning for too long - likely deadlock
        fatal("DEADLOCK DETECTED: %s in function '%s' could not be acquired for %"PRIi64" seconds",
              type, func, (int64_t)((now - *timestamp) / USEC_PER_SEC));
    }
}

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
    usec_t deadlock_timestamp = 0;

    while (true) {
        if (!__atomic_load_n(&spinlock->locked, __ATOMIC_RELAXED) &&
            !__atomic_test_and_set(&spinlock->locked, __ATOMIC_ACQUIRE)) {
            // Acquired the lock
            break;
        }

        // Backoff strategy with exponential growth
        spins++;
        
        // Check for deadlock every SPINS_BEFORE_DEADLOCK_CHECK iterations
        if ((spins % SPINS_BEFORE_DEADLOCK_CHECK) == 0) {
            spinlock_deadlock_detect(&deadlock_timestamp, "spinlock", func);
        }
        
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

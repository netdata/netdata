// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

#define MAX_USEC 512 // Maximum backoff limit in microseconds

// ----------------------------------------------------------------------------
// Deadlock detection function
void spinlock_deadlock_detect(usec_t *timestamp, const char *type, const char *func) {
    if (!*timestamp) {
        // First time checking - initialize the timestamp
        *timestamp = now_monotonic_usec();
        return;
    }

    // Check if we've exceeded the timeout
    usec_t now = now_monotonic_usec();
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

// Holder-aware deadlock detector, used only by SPINLOCK_TRACKED. Identical to
// spinlock_deadlock_detect() but, on timeout, names the current holder (tid,
// acquire-site function, and how long it has held the lock) in the fatal
// message so the event identifies WHO holds the lock, not just the waiter.
static void spinlock_tracked_deadlock_detect(usec_t *timestamp, const char *func, SPINLOCK_TRACKED *spinlock) {
    if (!*timestamp) {
        *timestamp = now_monotonic_usec();
        return;
    }

    usec_t now = now_monotonic_usec();
    if (now - *timestamp >= SPINLOCK_DEADLOCK_TIMEOUT_SEC * USEC_PER_SEC) {
        pid_t holder = __atomic_load_n(&spinlock->holder_tid, __ATOMIC_RELAXED);
        const char *holder_func = __atomic_load_n(&spinlock->holder_func, __ATOMIC_RELAXED);
        usec_t since = __atomic_load_n(&spinlock->holder_since_ut, __ATOMIC_RELAXED);
        int64_t waited_s = (int64_t)((now - *timestamp) / USEC_PER_SEC);

        if (holder)
            fatal("DEADLOCK DETECTED: spinlock in function '%s' could not be acquired for %"PRIi64" seconds "
                  "[holder: tid=%d func='%s' held_for=%"PRIi64"s]",
                  func, waited_s, holder, holder_func ? holder_func : "(unknown)",
                  since ? (int64_t)((now - since) / USEC_PER_SEC) : (int64_t)-1);
        else
            fatal("DEADLOCK DETECTED: spinlock in function '%s' could not be acquired for %"PRIi64" seconds "
                  "[holder: NONE - lock shows unlocked, possible corruption/stuck byte]",
                  func, waited_s);
    }
}

// Record the calling thread as the current holder of a tracked spinlock.
// Called immediately after the lock is acquired (lock and trylock paths).
static ALWAYS_INLINE void spinlock_tracked_record_holder(SPINLOCK_TRACKED *spinlock, const char *func) {
    __atomic_store_n(&spinlock->holder_tid, gettid_cached(), __ATOMIC_RELAXED);
    __atomic_store_n(&spinlock->holder_func, func, __ATOMIC_RELAXED);
    __atomic_store_n(&spinlock->holder_since_ut, now_monotonic_usec(), __ATOMIC_RELAXED);
}

// Shared spin-acquire core for both plain SPINLOCK and SPINLOCK_TRACKED. When
// `tracked` is non-NULL the holder fields are recorded on acquire and the
// holder-aware detector is used on contention. Being static + ALWAYS_INLINE,
// the NULL passed by spinlock_lock_with_trace() constant-folds away, so the
// plain spinlock path compiles to exactly the same code as before (no added
// branch, no holder stores).
static ALWAYS_INLINE void spinlock_lock_core(SPINLOCK *spinlock, const char *func, SPINLOCK_TRACKED *tracked) {
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
            if (tracked)
                spinlock_tracked_deadlock_detect(&deadlock_timestamp, func, tracked);
            else
                spinlock_deadlock_detect(&deadlock_timestamp, "spinlock", func);
        }

        microsleep(usec);
        usec = usec >= MAX_USEC ? MAX_USEC : usec * 2;
    }

#ifdef NETDATA_INTERNAL_CHECKS
    spinlock->spins += spins;
    spinlock->locker_pid = gettid_cached();
#endif

    if (tracked)
        spinlock_tracked_record_holder(tracked, func);

    nd_thread_spinlock_locked();
    worker_spinlock_contention(func, spins);
}

ALWAYS_INLINE void spinlock_lock_with_trace(SPINLOCK *spinlock, const char *func) {
    spinlock_lock_core(spinlock, func, NULL);
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

// ----------------------------------------------------------------------------
// SPINLOCK_TRACKED: holder-recording spinlock (see spinlock.h).

ALWAYS_INLINE void spinlock_tracked_init_with_trace(SPINLOCK_TRACKED *spinlock, const char *func __maybe_unused) {
    memset(spinlock, 0, sizeof(SPINLOCK_TRACKED));
}

ALWAYS_INLINE void spinlock_tracked_lock_with_trace(SPINLOCK_TRACKED *spinlock, const char *func) {
    spinlock_lock_core(&spinlock->spinlock, func, spinlock);
}

ALWAYS_INLINE void spinlock_tracked_unlock_with_trace(SPINLOCK_TRACKED *spinlock, const char *func __maybe_unused) {
    // clear the holder while still holding the lock, then release
    __atomic_store_n(&spinlock->holder_tid, 0, __ATOMIC_RELAXED);
    spinlock_unlock_with_trace(&spinlock->spinlock, func);
}

ALWAYS_INLINE bool spinlock_tracked_trylock_with_trace(SPINLOCK_TRACKED *spinlock, const char *func) {
    if (spinlock_trylock_with_trace(&spinlock->spinlock, func)) {
        spinlock_tracked_record_holder(spinlock, func);
        return true;
    }

    return false;
}

#endif // SPINLOCK_IMPL_WITH_MUTEX

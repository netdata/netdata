// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SPINLOCK_H
#define NETDATA_SPINLOCK_H

#include "libnetdata/common.h"

#ifdef SPINLOCK_IMPL_WITH_MUTEX
typedef struct netdata_spinlock
{
    netdata_mutex_t inner;
} SPINLOCK;
#else
typedef struct netdata_spinlock
{
    bool locked;
#ifdef NETDATA_INTERNAL_CHECKS
    pid_t locker_pid;
    size_t spins;
#endif
} SPINLOCK;
#endif

#ifdef SPINLOCK_IMPL_WITH_MUTEX
#define SPINLOCK_INITIALIZER { .inner = PTHREAD_MUTEX_INITIALIZER }

#define spinlock_lock(spinlock) netdata_mutex_lock(&((spinlock)->inner))
#define spinlock_unlock(spinlock) netdata_mutex_unlock(&((spinlock)->inner))
#define spinlock_trylock(spinlock) (netdata_mutex_trylock(&((spinlock)->inner)) == 0)
#define spinlock_init(spinlock) netdata_mutex_init(&((spinlock)->inner))
#else
#ifdef NETDATA_INTERNAL_CHECKS
#define SPINLOCK_INITIALIZER { .locked = false, .locker_pid = 0, .spins = 0 }
#else
#define SPINLOCK_INITIALIZER { .locked = false }
#endif

#define SPINLOCK_DEADLOCK_TIMEOUT_SEC 3600 // Number of seconds to wait before declaring a deadlock
#define SPINS_BEFORE_DEADLOCK_CHECK 100000

// Helper function to detect deadlocks
void spinlock_deadlock_detect(usec_t *timestamp, const char *type, const char *func);

void spinlock_init_with_trace(SPINLOCK *spinlock, const char *func);
#define spinlock_init(spinlock) spinlock_init_with_trace(spinlock, __FUNCTION__)

void spinlock_lock_with_trace(SPINLOCK *spinlock, const char *func);
#define spinlock_lock(spinlock) spinlock_lock_with_trace(spinlock, __FUNCTION__)

void spinlock_unlock_with_trace(SPINLOCK *spinlock, const char *func __maybe_unused);
#define spinlock_unlock(spinlock) spinlock_unlock_with_trace(spinlock, __FUNCTION__)

bool spinlock_trylock_with_trace(SPINLOCK *spinlock, const char *func __maybe_unused);
#define spinlock_trylock(spinlock) spinlock_trylock_with_trace(spinlock, __FUNCTION__)

#endif

// ----------------------------------------------------------------------------
// SPINLOCK_TRACKED: a spinlock that records its current holder (thread id,
// acquire-site function, and acquire time) so the deadlock detector can name
// the holder, not just the waiter, in its fatal message.
//
// Unlike SPINLOCK's locker_pid (NETDATA_INTERNAL_CHECKS-only), the holder is
// recorded in PRODUCTION. The cost (a cached gettid, a monotonic clock read,
// and a few relaxed stores per acquire) is paid ONLY by the few known-
// contended locks that opt in to this type; every plain SPINLOCK is untouched
// and pays nothing. Use it on locks whose 3600s deadlock fatals are
// un-triagable without holder identity.
//
// Available under both spinlock implementations. The spinning implementation
// integrates a holder-aware deadlock detector into the spin loop; the
// mutex-backed implementation (SPINLOCK_IMPL_WITH_MUTEX) only records the
// holder fields, since it does not spin and cannot self-detect deadlocks.
#define SPINLOCK_HOLDER_FUNC_MAX 48 // enough for any acquire-site __FUNCTION__

typedef struct netdata_spinlock_tracked {
    SPINLOCK spinlock;
    // The holder fields are NOT cleared on unlock; they describe the most
    // recent holder and are overwritten by the next acquirer. The detector
    // reads them only while the lock is held, so a free lock is never reported.
    pid_t holder_tid;           // gettid_cached() of the most recent holder; 0 before first acquire
    // The acquire-site function name is copied INLINE (not stored as a pointer)
    // so the detector can format it without dereferencing a possibly-wild
    // pointer: when the lock memory is corrupted (the case the detector exists
    // to report), a stored pointer would be garbage and crash vsnprintf, while
    // an inline buffer is part of the lock's own (mapped) memory and is read
    // with a bounded "%.*s". Empty before the first acquire.
    char holder_func[SPINLOCK_HOLDER_FUNC_MAX];
    usec_t holder_since_ut;     // monotonic time the most recent holder acquired the lock
} SPINLOCK_TRACKED;

void spinlock_tracked_init_with_trace(SPINLOCK_TRACKED *spinlock, const char *func);
#define spinlock_tracked_init(spinlock) spinlock_tracked_init_with_trace(spinlock, __FUNCTION__)

void spinlock_tracked_lock_with_trace(SPINLOCK_TRACKED *spinlock, const char *func);
#define spinlock_tracked_lock(spinlock) spinlock_tracked_lock_with_trace(spinlock, __FUNCTION__)

void spinlock_tracked_unlock_with_trace(SPINLOCK_TRACKED *spinlock, const char *func);
#define spinlock_tracked_unlock(spinlock) spinlock_tracked_unlock_with_trace(spinlock, __FUNCTION__)

bool spinlock_tracked_trylock_with_trace(SPINLOCK_TRACKED *spinlock, const char *func);
#define spinlock_tracked_trylock(spinlock) spinlock_tracked_trylock_with_trace(spinlock, __FUNCTION__)

#endif //NETDATA_SPINLOCK_H

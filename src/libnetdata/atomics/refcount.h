// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_REFCOUNT_H
#define NETDATA_REFCOUNT_H

#include "libnetdata/common.h"
#include <inttypes.h>

typedef int32_t REFCOUNT;

// the max number of references supported
// we use this to prevent overflowing the reference counter
#define REFCOUNT_MAX (1 * 1000 * 1000 * 1000)

// We set REFCOUNT_DELETED to a big negative,
// but to a value we can easily recognize while debugging.
#define REFCOUNT_DELETED (-2 * 1000 * 1000 * 1000)

// The error is a negative number, so that refcount > 0 is still
// good for checking if an acquired succeeded
#define REFCOUNT_ERROR INT32_MIN

/*
 * When debugging:
 *
 * 1. refcount 0 to 1 billion => the object is referenced
 * 2. refcount -1 billion to -1 => double releases or corruption
 * 2. refcount -2 billion to -1 billion => marked for deletion, with active references
 *                (this happens when you use refcount_acquire_for_deletion_and_wait())
 * 4. refcount outside -2 billion to 1 billion => memory corruption
 */

#define refcount_references(refcount) __atomic_load_n(refcount, __ATOMIC_RELAXED)
#define refcount_increment(refcount) __atomic_add_fetch(refcount, 1, __ATOMIC_ACQUIRE)
#define refcount_decrement(refcount) __atomic_sub_fetch(refcount, 1, __ATOMIC_RELEASE)
#define REFCOUNT_ACQUIRED(refcount) (refcount > 0)

#define REFCOUNT_VALID(refcount) \
    (((refcount) >= 0 && (refcount) <= REFCOUNT_MAX) || \
    ((refcount) >= REFCOUNT_DELETED && (refcount) <= -REFCOUNT_MAX))

// returns the non-usable refcount found when it fails, the final refcount when it succeeds
ALWAYS_INLINE WARNUNUSED
static REFCOUNT refcount_acquire_advanced_with_trace(REFCOUNT *refcount, const char *func __maybe_unused) {
    REFCOUNT expected = refcount_references(refcount);
    REFCOUNT desired;

    do {
        if(!REFCOUNT_VALID(expected))
            fatal("REFCOUNT %d is invalid (detected at %s(), called from %s())", expected, __FUNCTION__, func);

        if(expected >= REFCOUNT_MAX)
            return REFCOUNT_ERROR;

        if(expected < 0)
            return expected;

        desired = expected + 1;
    } while(!__atomic_compare_exchange_n(refcount, &expected, desired, false, __ATOMIC_ACQUIRE, __ATOMIC_RELAXED));

    return desired;
}

ALWAYS_INLINE WARNUNUSED
static bool refcount_acquire_with_trace(REFCOUNT *refcount, const char *func) {
    return REFCOUNT_ACQUIRED(refcount_acquire_advanced_with_trace(refcount, func));
}

// returns the number of references remaining
ALWAYS_INLINE
static REFCOUNT refcount_release_with_trace(REFCOUNT *refcount, const char *func __maybe_unused) {
    REFCOUNT expected, desired;

    do {
        expected = refcount_references(refcount);
        if(!REFCOUNT_VALID(expected))
            fatal("REFCOUNT %d is invalid (detected at %s(), called from %s())", expected, __FUNCTION__, func);

//        // the following is a valid case when using refcount_acquire_for_deletion_and_wait_with_trace()
//        if(expected <= 0)
//            fatal("REFCOUNT cannot release a refcount of %d (detected at %s(), called from %s())", expected, __FUNCTION__, func);

        desired = expected - 1;
    } while(!__atomic_compare_exchange_n(refcount, &expected, desired, false, __ATOMIC_RELEASE, __ATOMIC_RELAXED));

    return desired;
}

// returns true when the item can be deleted, false when the item is currently referenced
ALWAYS_INLINE WARNUNUSED
static bool refcount_acquire_for_deletion_with_trace(REFCOUNT *refcount, const char *func __maybe_unused) {
    REFCOUNT expected = 0;
    REFCOUNT desired = REFCOUNT_DELETED;

    if(__atomic_compare_exchange_n(refcount, &expected, desired, false, __ATOMIC_ACQUIRE, __ATOMIC_RELAXED))
        return true;

    if(!REFCOUNT_VALID(expected))
        fatal("REFCOUNT %d is invalid (detected at %s(), called from %s())", expected, __FUNCTION__, func);

    return false;
}

ALWAYS_INLINE WARNUNUSED
static REFCOUNT refcount_release_and_acquire_for_deletion_advanced_with_trace(REFCOUNT *refcount, const char *func __maybe_unused) {
    REFCOUNT expected, desired;

    do {
        expected = refcount_references(refcount);
        if (!REFCOUNT_VALID(expected))
            fatal("REFCOUNT %d is invalid (detected at %s(), called from %s())", expected, __FUNCTION__, func);

        if (expected == 1) {
            // we can get it for deletion
            desired = REFCOUNT_DELETED;
            if (__atomic_compare_exchange_n(refcount, &expected, desired, false, __ATOMIC_ACQUIRE, __ATOMIC_RELAXED))
                return desired;
        }
        else {
            // we can only release it
            desired = expected - 1;
            if (__atomic_compare_exchange_n(refcount, &expected, desired, false, __ATOMIC_RELEASE, __ATOMIC_RELAXED))
                return desired;
        }
    } while (true);
}

ALWAYS_INLINE WARNUNUSED
static bool refcount_release_and_acquire_for_deletion_with_trace(REFCOUNT *refcount, const char *func __maybe_unused) {
    return refcount_release_and_acquire_for_deletion_advanced_with_trace(refcount, func) == REFCOUNT_DELETED;
}

// this sleeps for 1 nanosecond (posix systems), or Sleep(0) on Windows
void tinysleep(void);

ALWAYS_INLINE
static bool refcount_acquire_for_deletion_and_wait_with_trace(REFCOUNT *refcount, const char *func) {
    REFCOUNT expected = refcount_references(refcount);
    REFCOUNT desired;

    do {
        if(!REFCOUNT_VALID(expected))
            fatal("REFCOUNT %d is invalid (detected at %s(), called from %s())", expected, __FUNCTION__, func);

        if(expected < 0)
            return false;

        desired = REFCOUNT_DELETED + expected;
    } while(!__atomic_compare_exchange_n(refcount, &expected, desired, false, __ATOMIC_ACQUIRE, __ATOMIC_RELAXED));

    while(__atomic_load_n(refcount, __ATOMIC_ACQUIRE) != REFCOUNT_DELETED) {
        tinysleep();
    }

    return true;
}

#define refcount_acquire_advanced(refcount) refcount_acquire_advanced_with_trace(refcount, __FUNCTION__ )
#define refcount_acquire(refcount) refcount_acquire_with_trace(refcount, __FUNCTION__)
#define refcount_release(refcount) refcount_release_with_trace(refcount, __FUNCTION__)
#define refcount_acquire_for_deletion(refcount) refcount_acquire_for_deletion_with_trace(refcount, __FUNCTION__)
#define refcount_release_and_acquire_for_deletion(refcount) refcount_release_and_acquire_for_deletion_with_trace(refcount, __FUNCTION__)
#define refcount_release_and_acquire_for_deletion_advanced(refcount) refcount_release_and_acquire_for_deletion_advanced_with_trace(refcount, __FUNCTION__)
#define refcount_acquire_for_deletion_and_wait(refcount) refcount_acquire_for_deletion_and_wait_with_trace(refcount, __FUNCTION__)

#endif //NETDATA_REFCOUNT_H

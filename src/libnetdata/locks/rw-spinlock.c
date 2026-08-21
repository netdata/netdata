// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

// Back-off tuning.
//
// YIELDS_BEFORE_SLEEP: how many yields a waiter performs before it starts
// sleeping. microsleep(1) is a nanosleep() whose real cost is the kernel timer
// slack (~65us on Linux), three orders of magnitude longer than the critical
// sections these locks protect, so sleeping must not be the FIRST response to
// contention. Measured: with sleep-first, 94.9% of writer acquisitions behind 8
// churning readers landed in the 65-131us bucket - the slack, not the drain.
//
// MAX_USEC: the sleep ceiling. Under writer priority every wait in this file is
// bounded by a single critical section rather than by a queue of waiters, so the
// old 512us ceiling was mis-sized: it made a losing reader retry only ~2000
// times per second, which is what turned writer priority into a reader lockout.
//
// yield_the_processor() (sched_yield) rather than a busy spin: Netdata runs on
// single-CPU appliances, where spinning prevents the lock holder from running at
// all.
#define YIELDS_BEFORE_SLEEP 16
#define MAX_USEC 32 // Maximum backoff limit in microseconds

#define WRITER_BIT (1U << 31)
#define READER_MASK (~WRITER_BIT)

// ----------------------------------------------------------------------------
// rw_spinlock implementation

// One back-off step for a waiter. Yields while `yields` is below the threshold,
// then falls back to sleeping with an exponential ramp capped at MAX_USEC.
// Callers that change what they are waiting for reset both counters, so the
// cheap yields are re-tried for the new wait.
//
// Deliberately NEVER_INLINE. The lock functions are always_inline and LTO is on
// (USE_LTO), so anything inlined here is duplicated into every one of the ~100
// lock call sites in the tree. This code runs only after a waiter has already
// lost the lock, where the cost of a call is nothing next to a yield or a sleep,
// while the .text it would add is paid by every caller - including the
// uncontended fast paths that never reach it.
static NEVER_INLINE void rw_spinlock_backoff(size_t *yields, usec_t *usec) {
    if((*yields)++ < YIELDS_BEFORE_SLEEP) {
        yield_the_processor();
        return;
    }

    microsleep(*usec);
    *usec = (*usec >= MAX_USEC) ? MAX_USEC : *usec * 2;
}

void rw_spinlock_init_with_trace(RW_SPINLOCK *rw_spinlock, const char *func __maybe_unused) {
    rw_spinlock->writer = 0;
    rw_spinlock->counter = 0;
}

ALWAYS_INLINE bool rw_spinlock_tryread_lock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func) {
    size_t spins = 0;

    uint32_t val = __atomic_add_fetch(&rw_spinlock->counter, 1, __ATOMIC_ACQUIRE);

    // Check if a writer holds the lock
    if (val & WRITER_BIT) {
        // Undo our increment and fail
        __atomic_sub_fetch(&rw_spinlock->counter, 1, __ATOMIC_RELEASE);
        return false;
    }

    worker_spinlock_contention(func, spins);
    nd_thread_rwspinlock_read_locked();
    return true;
}

ALWAYS_INLINE void rw_spinlock_read_lock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func) {
    size_t spins = 0;
    size_t yields = 0;
    usec_t usec = 1;
    usec_t deadlock_timestamp = 0;

    while (true) {
        // Optimistically increment reader count
        uint32_t val = __atomic_add_fetch(&rw_spinlock->counter, 1, __ATOMIC_ACQUIRE);

        // Check if a writer holds the lock
        if (!(val & WRITER_BIT)) {
            // no writer, we are in
            worker_spinlock_contention(func, spins);
            nd_thread_rwspinlock_read_locked();
            return;
        }

        // Undo our increment and retry
        __atomic_sub_fetch(&rw_spinlock->counter, 1, __ATOMIC_RELEASE);

        spins++;
        
        // Check for deadlock every SPINS_BEFORE_DEADLOCK_CHECK iterations
        if ((spins % SPINS_BEFORE_DEADLOCK_CHECK) == 0) {
            spinlock_deadlock_detect(&deadlock_timestamp, "rw-spinlock read lock", func);
        }

        rw_spinlock_backoff(&yields, &usec);
    }
}

ALWAYS_INLINE void rw_spinlock_read_unlock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func __maybe_unused) {
    __atomic_sub_fetch(&rw_spinlock->counter, 1, __ATOMIC_RELEASE);
    nd_thread_rwspinlock_read_unlocked();
}

ALWAYS_INLINE bool rw_spinlock_trywrite_lock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func) {
    // Optimistically set writer bit
    uint32_t old = __atomic_fetch_or(&rw_spinlock->counter, WRITER_BIT, __ATOMIC_ACQUIRE);

    if(old == 0) {
        rw_spinlock->writer = gettid_cached();
        worker_spinlock_contention(func, 0);
        nd_thread_rwspinlock_write_locked();
        return true;
    }

    // Check if we were the only one
    if (old & WRITER_BIT) {
        // there is a writer inside (keep the writer bit there)
    }
    else /* if ((old & READER_MASK) != 0) */ {
        // there are readers inside, remove the writer bit we added
        __atomic_and_fetch(&rw_spinlock->counter, ~WRITER_BIT, __ATOMIC_RELEASE);
    }

    return false;
}

ALWAYS_INLINE void rw_spinlock_write_lock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func) {
    size_t spins = 0;
    size_t yields = 0;
    usec_t usec = 1;
    usec_t deadlock_timestamp = 0;

    // Writer-priority state. Once this writer flips WRITER_BIT from 0 to 1,
    // it keeps the bit set across subsequent iterations and just polls for
    // readers to drain. New readers see WRITER_BIT and back off in the
    // reader path, so they cannot starve a queued writer.
    bool i_own_writer_bit = false;

    while (1) {
        if (!i_own_writer_bit) {
            uint32_t old = __atomic_fetch_or(&rw_spinlock->counter, WRITER_BIT, __ATOMIC_ACQUIRE);

            if (old == 0)
                // no readers, no writers - we acquired
                goto acquired;

            if (old & WRITER_BIT) {
                // another writer already had WRITER_BIT set; we did not flip it.
                // Spin without modifying state until that writer releases.
            }
            else /* (old & READER_MASK) != 0 */ {
                // only readers were present and WE just flipped WRITER_BIT.
                // Keep it set so new readers back off; wait for existing
                // readers to drain.
                i_own_writer_bit = true;

                // Restart the back-off from the bottom: from here on nobody can
                // enter the lock (new readers see WRITER_BIT, other writers see
                // it too), so a long sleep would keep the lock idle-but-blocked
                // after the last reader drains. The drain is typically a few
                // nanoseconds away, so the yields matter most here.
                yields = 0;
                usec = 1;
            }
        }
        else if (__atomic_load_n(&rw_spinlock->counter, __ATOMIC_ACQUIRE) == WRITER_BIT) {
            // We own WRITER_BIT and readers have drained. New readers see
            // WRITER_BIT and back off; existing readers decremented the
            // counter as they released.
            goto acquired;
        }

        spins++;

        // Check for deadlock every SPINS_BEFORE_DEADLOCK_CHECK iterations
        if ((spins % SPINS_BEFORE_DEADLOCK_CHECK) == 0) {
            spinlock_deadlock_detect(&deadlock_timestamp, "rw-spinlock write lock", func);
        }

        rw_spinlock_backoff(&yields, &usec);
    }

acquired:
    rw_spinlock->writer = gettid_cached();
    worker_spinlock_contention(func, spins);
    nd_thread_rwspinlock_write_locked();
}

ALWAYS_INLINE void rw_spinlock_write_unlock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func __maybe_unused) {
    rw_spinlock->writer = 0;
    __atomic_and_fetch(&rw_spinlock->counter, ~WRITER_BIT, __ATOMIC_RELEASE);
    nd_thread_rwspinlock_write_unlocked();
}

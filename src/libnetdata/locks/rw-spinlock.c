// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

#define MAX_USEC 512 // Maximum backoff limit in microseconds

#define WRITER_BIT (1U << 31)
#define READER_MASK (~WRITER_BIT)

// ----------------------------------------------------------------------------
// rw_spinlock implementation

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
        
        microsleep(usec);
        usec = usec >= MAX_USEC ? MAX_USEC : usec * 2;
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

        microsleep(usec);
        usec = usec >= MAX_USEC ? MAX_USEC : usec * 2;
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

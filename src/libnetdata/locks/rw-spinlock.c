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

    while (1) {
        // Optimistically set writer bit
        uint32_t old = __atomic_fetch_or(&rw_spinlock->counter, WRITER_BIT, __ATOMIC_ACQUIRE);

        // Check if we were the only one
        if (old == 0) {
            rw_spinlock->writer = gettid_cached();
            worker_spinlock_contention(func, spins);
            nd_thread_rwspinlock_write_locked();
            return;
        }

        // Check if we were the only one
        if (old & WRITER_BIT) {
            // there is a writer inside (keep the writer bit there)
        }
        else /* if ((old & READER_MASK) != 0) */ {
            // there are readers inside, remove the writer bit we added
            __atomic_and_fetch(&rw_spinlock->counter, ~WRITER_BIT, __ATOMIC_RELEASE);
        }

        spins++;
        microsleep(usec);
        usec = usec >= MAX_USEC ? MAX_USEC : usec * 2;
    }
}

ALWAYS_INLINE void rw_spinlock_write_unlock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func __maybe_unused) {
    rw_spinlock->writer = 0;
    __atomic_and_fetch(&rw_spinlock->counter, ~WRITER_BIT, __ATOMIC_RELEASE);
    nd_thread_rwspinlock_write_unlocked();
}

// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

// ----------------------------------------------------------------------------
// rw_spinlock implementation

void rw_spinlock_init_with_trace(RW_SPINLOCK *rw_spinlock, const char *func) {
    rw_spinlock->readers = 0;
    rw_spinlock->writers_waiting = 0;
    spinlock_init_with_trace(&rw_spinlock->spinlock, func);
}

void rw_spinlock_read_lock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func) {
    size_t spins = 0;
    while(1) {
        spinlock_lock_with_trace(&rw_spinlock->spinlock, func);
        if (!rw_spinlock->writers_waiting) {
            __atomic_add_fetch(&rw_spinlock->readers, 1, __ATOMIC_RELAXED);
            spinlock_unlock_with_trace(&rw_spinlock->spinlock, func);
            break;
        }

        spinlock_unlock_with_trace(&rw_spinlock->spinlock, func);
        yield_the_processor(); // let the writer run
        spins++;
    }

    worker_spinlock_contention(func, spins);
    nd_thread_rwspinlock_read_locked();
}

void rw_spinlock_read_unlock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func __maybe_unused) {
#ifndef NETDATA_INTERNAL_CHECKS
    __atomic_sub_fetch(&rw_spinlock->readers, 1, __ATOMIC_RELAXED);
#else
    int32_t x = __atomic_sub_fetch(&rw_spinlock->readers, 1, __ATOMIC_RELAXED);
    if(x < 0)
        fatal("RW_SPINLOCK: readers is negative %d", x);
#endif

    nd_thread_rwspinlock_read_unlocked();
}

void rw_spinlock_write_lock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func) {
    size_t spins = 0;
    for(size_t i = 1; true ;i++) {
        spinlock_lock_with_trace(&rw_spinlock->spinlock, func);

        if(__atomic_load_n(&rw_spinlock->readers, __ATOMIC_RELAXED) == 0) {
            if(spins != 0)
                rw_spinlock->writers_waiting--;
            break;
        }

        if(spins == 0)
            rw_spinlock->writers_waiting++;

        // Busy wait until all readers have released their locks.
        spinlock_unlock_with_trace(&rw_spinlock->spinlock, func);
        if(i == 8 * 2) {
            i = 0;
            tinysleep();
        }
        spins++;
    }

    worker_spinlock_contention(func, spins);
    nd_thread_rwspinlock_write_locked();
}

void rw_spinlock_write_unlock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func) {
    spinlock_unlock_with_trace(&rw_spinlock->spinlock, func);
    nd_thread_rwspinlock_write_unlocked();
}

bool rw_spinlock_tryread_lock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func) {
    if(spinlock_trylock_with_trace(&rw_spinlock->spinlock, func)) {
        __atomic_add_fetch(&rw_spinlock->readers, 1, __ATOMIC_RELAXED);
        spinlock_unlock_with_trace(&rw_spinlock->spinlock, func);
        nd_thread_rwspinlock_read_locked();
        return true;
    }

    return false;
}

bool rw_spinlock_trywrite_lock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func) {
    if(spinlock_trylock_with_trace(&rw_spinlock->spinlock, func)) {
        if (__atomic_load_n(&rw_spinlock->readers, __ATOMIC_RELAXED) == 0) {
            // No readers, we've successfully acquired the write lock
            nd_thread_rwspinlock_write_locked();
            return true;
        }
        else {
            // There are readers, unlock the spinlock and return false
            spinlock_unlock_with_trace(&rw_spinlock->spinlock, func);
        }
    }

    return false;
}


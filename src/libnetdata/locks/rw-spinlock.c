// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

// ----------------------------------------------------------------------------
// rw_spinlock implementation

void rw_spinlock_init(RW_SPINLOCK *rw_spinlock) {
    rw_spinlock->readers = 0;
    rw_spinlock->writers_waiting = 0;
    spinlock_init(&rw_spinlock->spinlock);
}

void rw_spinlock_read_lock(RW_SPINLOCK *rw_spinlock) {
    while(1) {
        spinlock_lock(&rw_spinlock->spinlock);
        if (!rw_spinlock->writers_waiting) {
            __atomic_add_fetch(&rw_spinlock->readers, 1, __ATOMIC_RELAXED);
            spinlock_unlock(&rw_spinlock->spinlock);
            break;
        }

        spinlock_unlock(&rw_spinlock->spinlock);
        yield_the_processor(); // let the writer run
    }

    nd_thread_rwspinlock_read_locked();
}

void rw_spinlock_read_unlock(RW_SPINLOCK *rw_spinlock) {
#ifndef NETDATA_INTERNAL_CHECKS
    __atomic_sub_fetch(&rw_spinlock->readers, 1, __ATOMIC_RELAXED);
#else
    int32_t x = __atomic_sub_fetch(&rw_spinlock->readers, 1, __ATOMIC_RELAXED);
    if(x < 0)
        fatal("RW_SPINLOCK: readers is negative %d", x);
#endif

    nd_thread_rwspinlock_read_unlocked();
}

void rw_spinlock_write_lock(RW_SPINLOCK *rw_spinlock) {
    size_t spins = 0;
    for(size_t i = 1; true ;i++) {
        spinlock_lock(&rw_spinlock->spinlock);

        if(__atomic_load_n(&rw_spinlock->readers, __ATOMIC_RELAXED) == 0) {
            if(spins != 0)
                rw_spinlock->writers_waiting--;
            break;
        }

        if(spins == 0)
            rw_spinlock->writers_waiting++;

        // Busy wait until all readers have released their locks.
        spinlock_unlock(&rw_spinlock->spinlock);
        if(i == 8 * 2) {
            i = 0;
            tinysleep();
        }
        spins++;
    }

    (void)spins;

    nd_thread_rwspinlock_write_locked();
}

void rw_spinlock_write_unlock(RW_SPINLOCK *rw_spinlock) {
    spinlock_unlock(&rw_spinlock->spinlock);
    nd_thread_rwspinlock_write_unlocked();
}

bool rw_spinlock_tryread_lock(RW_SPINLOCK *rw_spinlock) {
    if(spinlock_trylock(&rw_spinlock->spinlock)) {
        __atomic_add_fetch(&rw_spinlock->readers, 1, __ATOMIC_RELAXED);
        spinlock_unlock(&rw_spinlock->spinlock);
        nd_thread_rwspinlock_read_locked();
        return true;
    }

    return false;
}

bool rw_spinlock_trywrite_lock(RW_SPINLOCK *rw_spinlock) {
    if(spinlock_trylock(&rw_spinlock->spinlock)) {
        if (__atomic_load_n(&rw_spinlock->readers, __ATOMIC_RELAXED) == 0) {
            // No readers, we've successfully acquired the write lock
            nd_thread_rwspinlock_write_locked();
            return true;
        }
        else {
            // There are readers, unlock the spinlock and return false
            spinlock_unlock(&rw_spinlock->spinlock);
        }
    }

    return false;
}



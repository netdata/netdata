// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

// ----------------------------------------------------------------------------
// spinlock implementation
// https://www.youtube.com/watch?v=rmGJc9PXpuE&t=41s

#ifdef SPINLOCK_IMPL_WITH_MUTEX
void spinlock_init(SPINLOCK *spinlock)
{
    netdata_mutex_init(&spinlock->inner);
}
#else
void spinlock_init(SPINLOCK *spinlock)
{
    memset(spinlock, 0, sizeof(SPINLOCK));
}
#endif

#ifndef SPINLOCK_IMPL_WITH_MUTEX
static inline void spinlock_lock_internal(SPINLOCK *spinlock)
{
#ifdef NETDATA_INTERNAL_CHECKS
    size_t spins = 0;
#endif

    for(int i = 1;
         __atomic_load_n(&spinlock->locked, __ATOMIC_RELAXED) ||
         __atomic_test_and_set(&spinlock->locked, __ATOMIC_ACQUIRE)
             ; i++
    ) {

#ifdef NETDATA_INTERNAL_CHECKS
        spins++;
#endif

        if(unlikely(i % 8 == 0)) {
            i = 0;
            tinysleep();
        }
    }

    // we have the lock

#ifdef NETDATA_INTERNAL_CHECKS
    spinlock->spins += spins;
    spinlock->locker_pid = gettid_cached();
#endif

    nd_thread_spinlock_locked();
}
#endif // SPINLOCK_IMPL_WITH_MUTEX

#ifndef SPINLOCK_IMPL_WITH_MUTEX
static inline void spinlock_unlock_internal(SPINLOCK *spinlock)
{
#ifdef NETDATA_INTERNAL_CHECKS
    spinlock->locker_pid = 0;
#endif

    __atomic_clear(&spinlock->locked, __ATOMIC_RELEASE);

    nd_thread_spinlock_unlocked();
}
#endif // SPINLOCK_IMPL_WITH_MUTEX

#ifndef SPINLOCK_IMPL_WITH_MUTEX
static inline bool spinlock_trylock_internal(SPINLOCK *spinlock) {
    if(!__atomic_load_n(&spinlock->locked, __ATOMIC_RELAXED) &&
        !__atomic_test_and_set(&spinlock->locked, __ATOMIC_ACQUIRE)) {
        // we got the lock
        nd_thread_spinlock_locked();
        return true;
    }

    return false;
}
#endif // SPINLOCK_IMPL_WITH_MUTEX

#ifdef SPINLOCK_IMPL_WITH_MUTEX
void spinlock_lock(SPINLOCK *spinlock)
{
    netdata_mutex_lock(&spinlock->inner);
}
#else
void spinlock_lock(SPINLOCK *spinlock)
{
    spinlock_lock_internal(spinlock);
}
#endif

#ifdef SPINLOCK_IMPL_WITH_MUTEX
void spinlock_unlock(SPINLOCK *spinlock)
{
    netdata_mutex_unlock(&spinlock->inner);
}
#else
void spinlock_unlock(SPINLOCK *spinlock)
{
    spinlock_unlock_internal(spinlock);
}
#endif

#ifdef SPINLOCK_IMPL_WITH_MUTEX
bool spinlock_trylock(SPINLOCK *spinlock)
{
    return netdata_mutex_trylock(&spinlock->inner) == 0;
}
#else
bool spinlock_trylock(SPINLOCK *spinlock)
{
    return spinlock_trylock_internal(spinlock);
}
#endif

#ifdef SPINLOCK_IMPL_WITH_MUTEX
void spinlock_lock_cancelable(SPINLOCK *spinlock)
{
    netdata_mutex_lock(&spinlock->inner);
}
#else
void spinlock_lock_cancelable(SPINLOCK *spinlock)
{
    spinlock_lock_internal(spinlock);
}
#endif

#ifdef SPINLOCK_IMPL_WITH_MUTEX
void spinlock_unlock_cancelable(SPINLOCK *spinlock)
{
    netdata_mutex_unlock(&spinlock->inner);
}
#else
void spinlock_unlock_cancelable(SPINLOCK *spinlock)
{
    spinlock_unlock_internal(spinlock);
}
#endif

#ifdef SPINLOCK_IMPL_WITH_MUTEX
bool spinlock_trylock_cancelable(SPINLOCK *spinlock)
{
    return netdata_mutex_trylock(&spinlock->inner) == 0;
}
#else
bool spinlock_trylock_cancelable(SPINLOCK *spinlock)
{
    return spinlock_trylock_internal(spinlock);
}
#endif

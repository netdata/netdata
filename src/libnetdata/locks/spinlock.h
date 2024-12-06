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
#else
#define SPINLOCK_INITIALIZER { .locked = false }
#endif

void spinlock_init(SPINLOCK *spinlock);
void spinlock_lock(SPINLOCK *spinlock);
void spinlock_unlock(SPINLOCK *spinlock);
bool spinlock_trylock(SPINLOCK *spinlock);

void spinlock_lock_cancelable(SPINLOCK *spinlock);
void spinlock_unlock_cancelable(SPINLOCK *spinlock);
bool spinlock_trylock_cancelable(SPINLOCK *spinlock);

#endif //NETDATA_SPINLOCK_H

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
#define spinlock_init(spinlock) netdata_mutex_init(&((spinlock)->inner)
#else
#ifdef NETDATA_INTERNAL_CHECKS
#define SPINLOCK_INITIALIZER { .locked = false, .locker_pid = 0, .spins = 0 }
#else
#define SPINLOCK_INITIALIZER { .locked = false }
#endif

void spinlock_init_with_trace(SPINLOCK *spinlock, const char *func);
#define spinlock_init(spinlock) spinlock_init_with_trace(spinlock, __FUNCTION__)

void spinlock_lock_with_trace(SPINLOCK *spinlock, const char *func);
#define spinlock_lock(spinlock) spinlock_lock_with_trace(spinlock, __FUNCTION__)

void spinlock_unlock_with_trace(SPINLOCK *spinlock, const char *func __maybe_unused);
#define spinlock_unlock(spinlock) spinlock_unlock_with_trace(spinlock, __FUNCTION__)

bool spinlock_trylock_with_trace(SPINLOCK *spinlock, const char *func __maybe_unused);
#define spinlock_trylock(spinlock) spinlock_trylock_with_trace(spinlock, __FUNCTION__)

#endif

#endif //NETDATA_SPINLOCK_H

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RW_SPINLOCK_H
#define NETDATA_RW_SPINLOCK_H

#include "libnetdata/common.h"
#include "spinlock.h"

typedef struct netdata_rw_spinlock {
    int32_t readers;
    int32_t writers_waiting;
    SPINLOCK spinlock;
} RW_SPINLOCK;

#define RW_SPINLOCK_INITIALIZER { .readers = 0, .spinlock = SPINLOCK_INITIALIZER}

void rw_spinlock_init(RW_SPINLOCK *rw_spinlock);
void rw_spinlock_read_lock(RW_SPINLOCK *rw_spinlock);
void rw_spinlock_read_unlock(RW_SPINLOCK *rw_spinlock);
void rw_spinlock_write_lock(RW_SPINLOCK *rw_spinlock);
void rw_spinlock_write_unlock(RW_SPINLOCK *rw_spinlock);
bool rw_spinlock_tryread_lock(RW_SPINLOCK *rw_spinlock);
bool rw_spinlock_trywrite_lock(RW_SPINLOCK *rw_spinlock);

#endif //NETDATA_RW_SPINLOCK_H

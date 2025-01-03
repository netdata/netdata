// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RW_SPINLOCK_H
#define NETDATA_RW_SPINLOCK_H

#include "libnetdata/common.h"
#include "spinlock.h"

typedef struct netdata_rw_spinlock {
    pid_t writer;
    uint32_t counter;
} RW_SPINLOCK;

#define RW_SPINLOCK_INITIALIZER { .counter = 0, .writer = 0, }

void rw_spinlock_init_with_trace(RW_SPINLOCK *rw_spinlock, const char *func);
void rw_spinlock_read_lock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func);
void rw_spinlock_read_unlock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func);
void rw_spinlock_write_lock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func);
void rw_spinlock_write_unlock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func);
bool rw_spinlock_tryread_lock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func);
bool rw_spinlock_trywrite_lock_with_trace(RW_SPINLOCK *rw_spinlock, const char *func);


#define rw_spinlock_init(rw_spinlock) rw_spinlock_init_with_trace(rw_spinlock, __FUNCTION__)
#define rw_spinlock_read_lock(rw_spinlock) rw_spinlock_read_lock_with_trace(rw_spinlock, __FUNCTION__)
#define rw_spinlock_read_unlock(rw_spinlock) rw_spinlock_read_unlock_with_trace(rw_spinlock, __FUNCTION__)
#define rw_spinlock_write_lock(rw_spinlock) rw_spinlock_write_lock_with_trace(rw_spinlock, __FUNCTION__)
#define rw_spinlock_write_unlock(rw_spinlock) rw_spinlock_write_unlock_with_trace(rw_spinlock, __FUNCTION__)
#define rw_spinlock_tryread_lock(rw_spinlock) rw_spinlock_tryread_lock_with_trace(rw_spinlock, __FUNCTION__)
#define rw_spinlock_trywrite_lock(rw_spinlock) rw_spinlock_trywrite_lock_with_trace(rw_spinlock, __FUNCTION__)

#endif //NETDATA_RW_SPINLOCK_H

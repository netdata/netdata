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

// This rw-spinlock is writer-priority: once a writer is waiting, it holds
// WRITER_BIT set while it waits for the current readers to drain, and new
// readers back off until the writer has acquired and released the lock. This is
// what prevents a continuous reader stream from starving a writer.
//
// PRECONDITION - recursive read locks:
// A thread that already holds the read lock MUST NOT take the read lock again
// on the same rw-spinlock. It is safe only while no writer is pending, which
// the caller cannot control, so it must not be relied upon. If a writer becomes
// pending between the two acquisitions, the second rw_spinlock_read_lock()
// backs off waiting for that writer, while the writer waits for the caller's
// first read reference to drain: both block forever.
//
// Traversal helpers that release the read lock around each callback (for
// example the dictionary's DICTIONARY_LOCK_REENTRANT mode, which unlocks before
// returning each item and re-locks in dictionary_foreach_next()) do not hold a
// read reference across the body and therefore do not create this cycle.
//
// Use rw_spinlock_tryread_lock() when a nested read is genuinely needed: it
// fails instead of blocking when a writer is pending.

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

int rw_spinlock_unittest(void);

#endif //NETDATA_RW_SPINLOCK_H

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WAITQ_H
#define NETDATA_WAITQ_H

#include "libnetdata/libnetdata.h"

/*
 * WAITING QUEUE
 * Like a spinlock, but:
 *
 * 1. Waiters get a sequence number (FIFO)
 * 2. FIFO is respected within each priority
 * 3. Higher priority threads get in first
 *
 * This is equivalent to 3 atomic operations for lock, and 1 for unlock.
 *
 * As lightweight and fast as it can be.
 * About 3M thread switches/s per WAITING QUEUE, on modern hardware.
 *
 * Be careful: higher priority threads can starve the rest!
 *
 */

typedef enum __attribute__((packed)) {
    WAITQ_PRIO_URGENT = 0,      // will be first
    WAITQ_PRIO_HIGH,            // will be second
    WAITQ_PRIO_NORMAL,          // will be third
    WAITQ_PRIO_LOW,             // will be last

    // terminator
    WAITQ_PRIO_MAX,
} WAITQ_PRIORITY;

typedef struct waiting_queue {
    SPINLOCK spinlock;          // protects the actual resource
    pid_t writer;               // the pid the thread currently holding the lock
    uint64_t current_priority;  // current highest priority attempting to acquire
    uint32_t last_seqno;        // for FIFO ordering within same priority
} WAITQ;

#define WAITQ_INITIALIZER (WAITQ){ .spinlock = SPINLOCK_INITIALIZER, .current_priority = 0, .last_seqno = 0, }

// Initialize a waiting queue
void waitq_init(WAITQ *waitq);

// Destroy a waiting queue - must be empty
void waitq_destroy(WAITQ *wq);

// Returns true when the queue is acquired
bool waitq_try_acquire_with_trace(WAITQ *waitq, WAITQ_PRIORITY priority, const char *func);
#define waitq_try_acquire(waitq, priority) waitq_try_acquire_with_trace(waitq, priority, __FUNCTION__)

// Returns when it is our turn to run
// Returns time spent waiting in microseconds
void waitq_acquire_with_trace(WAITQ *waitq, WAITQ_PRIORITY priority, const char *func);
#define waitq_acquire(waitq, priority) waitq_acquire_with_trace(waitq, priority, __FUNCTION__)

// Mark that we are done - wakes up the next in line
void waitq_release(WAITQ *waitq);

int unittest_waiting_queue(void);

#endif // NETDATA_WAITQ_H

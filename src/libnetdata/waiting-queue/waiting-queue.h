// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WAITING_QUEUE_H
#define NETDATA_WAITING_QUEUE_H

#include "libnetdata/libnetdata.h"

/*
 * WAITING QUEUE
 * Like a mutex, or a spinlock, but:
 *
 * 1. Waiters get a sequence number (FIFO)
 * 2. FIFO is respected within each priority
 * 3. Higher priority threads get in first
 * 4. No wasting of resources, there are no spins
 *
 * When there are no other waiters, this is equivalent to 2 atomic
 * operations for lock, and 2 for unlock.
 *
 * As lightweight and fast as it can be.
 * About 0.5M thread switches/s per WAITING QUEUE, on modern hardware.
 *
 * Be careful: higher priority threads can starve the rest!
 *
 */

typedef struct waiting_queue WAITING_QUEUE;

typedef enum __attribute__((packed)) {
    WAITING_QUEUE_PRIO_URGENT = 0,  // will be first
    WAITING_QUEUE_PRIO_HIGH,        // will be second
    WAITING_QUEUE_PRIO_NORMAL,      // will be third
    WAITING_QUEUE_PRIO_LOW,         // will be last

    // terminator
    WAITING_QUEUE_PRIO_MAX,
} WAITING_QUEUE_PRIORITY;

// Initialize a waiting queue
WAITING_QUEUE *waiting_queue_create(void);

// Destroy a waiting queue - must be empty
void waiting_queue_destroy(WAITING_QUEUE *wq);

// Returns true when the queue is acquired
bool waiting_queue_try_acquire(WAITING_QUEUE *wq);

// Returns when it is our turn to run
// Returns time spent waiting in microseconds
void waiting_queue_acquire(WAITING_QUEUE *wq, WAITING_QUEUE_PRIORITY priority);

// Mark that we are done - wakes up the next in line
void waiting_queue_release(WAITING_QUEUE *wq);

// Return the number of threads currently waiting
size_t waiting_queue_waiting(WAITING_QUEUE *wq);

int unittest_waiting_queue(void);

#endif // NETDATA_WAITING_QUEUE_H

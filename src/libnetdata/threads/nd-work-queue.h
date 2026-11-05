// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ND_WORK_QUEUE_H
#define NETDATA_ND_WORK_QUEUE_H

#include "../libnetdata.h"

// A bounded pool of dedicated threads executing submitted jobs in FIFO order.
//
// It exists for work that blocks on something the libuv threadpool has to execute: such work MUST NOT
// run on the libuv threadpool itself, or enough of it occupies every pool thread and nothing is left to
// run what it waits for (the agent freezes). Data queries are the case in point: they wait on dbengine
// query preparation and extent reads, which run on the libuv threadpool.
//
// - Threads are created on demand, up to max_threads. A thread idle for idle_timeout_ut exits (0 = never),
//   and a later submit creates a new one when needed.
// - A new job goes to the most recently idle thread, so under a steady trickle of jobs the extra threads
//   stay idle and exit: the pool shrinks to the concurrency the load needs.
// - Every accepted job runs exactly once, also after nd_work_queue_stop(): stopping only refuses new jobs,
//   so a caller waiting on a submitted job is never left waiting forever.
// - The queue does not own the job data; the job callback does.

typedef struct nd_work_queue ND_WORK_QUEUE;

typedef void (*nd_work_queue_job_cb)(void *data);

// called on each new worker thread before it runs its first job (worker registration, job names)
typedef void (*nd_work_queue_thread_init_cb)(void *data);

ND_WORK_QUEUE *nd_work_queue_create(const char *tag, size_t max_threads, usec_t idle_timeout_ut,
                                    nd_work_queue_thread_init_cb thread_init, void *thread_init_data);

// Returns false when the job was not accepted (the queue is stopped, or no worker thread could be
// created): the caller still owns the data and decides what to do with it.
bool nd_work_queue_submit(ND_WORK_QUEUE *wq, nd_work_queue_job_cb cb, void *data);

// Refuse new jobs. Jobs already accepted still run; the worker threads exit when the queue is empty.
void nd_work_queue_stop(ND_WORK_QUEUE *wq);

// Stop the queue, wait up to timeout_ut for its threads to exit, and free it.
// The caller MUST ensure no nd_work_queue_submit() on this queue is running or can start (the queue
// is freed under it otherwise); nd_work_queue_stop() alone has no such requirement.
// Returns false (and frees nothing) when a thread is still running a job at the deadline: the queue
// and its threads are then deliberately leaked, since a running job may still use them.
bool nd_work_queue_stop_and_free(ND_WORK_QUEUE *wq, usec_t timeout_ut);

int nd_work_queue_unittest(void);

#endif //NETDATA_ND_WORK_QUEUE_H

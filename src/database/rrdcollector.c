// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdcollector.h"
#include "rrdcollector-internals.h"

// Each function points to this collector structure
// so that when the collector exits, all of them will
// be invalidated (running == false)
// The last function using this collector
// frees the structure too (or when the collector calls
// rrdset_collector_finished()).

struct rrd_collector {
    REFCOUNT refcount;
    REFCOUNT refcount_dispatcher;
    pid_t tid;
    bool running;
};

// Each thread that adds RRDSET functions has to call
// rrdset_collector_started() and rrdset_collector_finished()
// to create the collector structure.

__thread struct rrd_collector *thread_rrd_collector = NULL;

inline bool rrd_collector_running(struct rrd_collector *rdc) {
    return __atomic_load_n(&rdc->running, __ATOMIC_RELAXED);
}

inline pid_t rrd_collector_tid(struct rrd_collector *rdc) {
    return rdc->tid;
}

bool rrd_collector_dispatcher_acquire(struct rrd_collector *rdc) {
    return refcount_acquire(&rdc->refcount_dispatcher);
}

void rrd_collector_dispatcher_release(struct rrd_collector *rdc) {
    refcount_release(&rdc->refcount_dispatcher);
}

static void rrd_collector_free(struct rrd_collector *rdc) {
    if(rrd_collector_running(rdc) || !refcount_acquire_for_deletion(&rdc->refcount))
        // the collector is still referenced by charts.
        // leave it hanging there, the last chart will actually free it.
        return;

    // we can free it now
    freez(rdc);
}

// called once per collector
void rrd_collector_started(void) {
    if(!thread_rrd_collector)
        thread_rrd_collector = callocz(1, sizeof(struct rrd_collector));

    thread_rrd_collector->tid = gettid_cached();
    __atomic_store_n(&thread_rrd_collector->running, true, __ATOMIC_RELAXED);
}

// called once per collector
void rrd_collector_finished(void) {
    if(!thread_rrd_collector)
        return;

    __atomic_store_n(&thread_rrd_collector->running, false, __ATOMIC_RELAXED);

    // wait for any cancellation requests to be dispatched;
    // the problem is that cancellation requests require a structure allocated by the collector,
    // so, while cancellation requests are being dispatched, this structure is accessed.
    // delaying the exit of the thread is required to avoid cleaning up this structure.

    while(!refcount_acquire_for_deletion(&thread_rrd_collector->refcount_dispatcher))
        sleep_usec(1 * USEC_PER_MS);

    rrd_collector_free(thread_rrd_collector);
    thread_rrd_collector = NULL;
}

static bool rrd_collector_acquire(struct rrd_collector *rdc) {
    if(!rdc || !rrd_collector_running(rdc))
        return false;

    return refcount_acquire(&rdc->refcount);
}

struct rrd_collector *rrd_collector_acquire_current_thread(void) {
    rrd_collector_started();

    if(!rrd_collector_acquire(thread_rrd_collector))
        internal_fatal(true, "FUNCTIONS: Trying to acquire a the current thread collector, that is currently exiting.");

    return thread_rrd_collector;
}

void rrd_collector_release(struct rrd_collector *rdc) {
    if(unlikely(!rdc)) return;

    if(refcount_release(&rdc->refcount) == 0)
        rrd_collector_free(rdc);
}

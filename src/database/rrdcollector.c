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
    int32_t refcount;
    int32_t refcount_dispatcher;
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
    int32_t expected = __atomic_load_n(&rdc->refcount_dispatcher, __ATOMIC_RELAXED);
    int32_t wanted;
    do {
        if(expected < 0)
            return false;

        wanted = expected + 1;
    } while(!__atomic_compare_exchange_n(&rdc->refcount_dispatcher, &expected, wanted, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED));

    return true;
}

void rrd_collector_dispatcher_release(struct rrd_collector *rdc) {
    __atomic_sub_fetch(&rdc->refcount_dispatcher, 1, __ATOMIC_RELAXED);
}

static void rrd_collector_free(struct rrd_collector *rdc) {
    if(rdc->running)
        return;

    int32_t expected = 0;
    if(!__atomic_compare_exchange_n(&rdc->refcount, &expected, -1, false, __ATOMIC_ACQUIRE, __ATOMIC_RELAXED)) {
        // the collector is still referenced by charts.
        // leave it hanging there, the last chart will actually free it.
        return;
    }

    // we can free it now
    freez(rdc);
}

// called once per collector
void rrd_collector_started(void) {
    if(!thread_rrd_collector)
        thread_rrd_collector = callocz(1, sizeof(struct rrd_collector));

    thread_rrd_collector->tid = gettid();
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

    int32_t expected = 0;
    while(!__atomic_compare_exchange_n(&thread_rrd_collector->refcount_dispatcher, &expected, -1, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED)) {
        expected = 0;
        sleep_usec(1 * USEC_PER_MS);
    }

    rrd_collector_free(thread_rrd_collector);
    thread_rrd_collector = NULL;
}

bool rrd_collector_acquire(struct rrd_collector *rdc) {

    int32_t expected = __atomic_load_n(&rdc->refcount, __ATOMIC_RELAXED), wanted = 0;
    do {
        if(expected < 0 || !rrd_collector_running(rdc))
            return false;

        wanted = expected + 1;
    } while(!__atomic_compare_exchange_n(&rdc->refcount, &expected, wanted, false, __ATOMIC_ACQUIRE, __ATOMIC_RELAXED));

    return true;
}

struct rrd_collector *rrd_collector_acquire_current_thread(void) {
    rrd_collector_started();

    if(!rrd_collector_acquire(thread_rrd_collector))
        internal_fatal(true, "FUNCTIONS: Trying to acquire a the current thread collector, that is currently exiting.");

    return thread_rrd_collector;
}

void rrd_collector_release(struct rrd_collector *rdc) {
    if(unlikely(!rdc)) return;

    int32_t expected = __atomic_load_n(&rdc->refcount, __ATOMIC_RELAXED), wanted = 0;
    do {
        if(expected < 0)
            return;

        if(expected == 0) {
            internal_fatal(true, "FUNCTIONS: Trying to release a collector that is not acquired.");
            return;
        }

        wanted = expected - 1;
    } while(!__atomic_compare_exchange_n(&rdc->refcount, &expected, wanted, false, __ATOMIC_RELEASE, __ATOMIC_RELAXED));

    if(wanted == 0)
        rrd_collector_free(rdc);
}

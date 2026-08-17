// SPDX-License-Identifier: GPL-3.0-or-later

#include "nrpc-serving.h"
#include "nrpc-serving-internals.h"

// Each function points to this provider structure
// so that when the provider thread exits, all of them will
// be invalidated (running == false)
// The last function using this provider
// frees the structure too (or when the provider thread calls
// rrd_function_provider_finished()).

struct rrd_function_provider {
    REFCOUNT refcount;
    REFCOUNT refcount_dispatcher;
    pid_t tid;
    bool running;
};

// Each thread that registers functions has to call
// rrd_function_provider_started() and rrd_function_provider_finished()
// to create and tear down the provider structure.

__thread struct rrd_function_provider *thread_rrd_function_provider = NULL;

inline bool rrd_function_provider_running(struct rrd_function_provider *rdc) {
    return __atomic_load_n(&rdc->running, __ATOMIC_RELAXED);
}

inline pid_t rrd_function_provider_tid(struct rrd_function_provider *rdc) {
    return rdc->tid;
}

bool rrd_function_provider_dispatcher_acquire(struct rrd_function_provider *rdc) {
    return refcount_acquire(&rdc->refcount_dispatcher);
}

void rrd_function_provider_dispatcher_release(struct rrd_function_provider *rdc) {
    refcount_release(&rdc->refcount_dispatcher);
}

static void rrd_function_provider_free(struct rrd_function_provider *rdc) {
    if(rrd_function_provider_running(rdc) || !refcount_acquire_for_deletion(&rdc->refcount))
        // the collector is still referenced by charts.
        // leave it hanging there, the last chart will actually free it.
        return;

    // we can free it now
    freez(rdc);
}

// called once per collector
void rrd_function_provider_started(void) {
    if(!thread_rrd_function_provider)
        thread_rrd_function_provider = callocz(1, sizeof(struct rrd_function_provider));

    thread_rrd_function_provider->tid = gettid_cached();
    __atomic_store_n(&thread_rrd_function_provider->running, true, __ATOMIC_RELAXED);
}

// called once per collector
void rrd_function_provider_finished(void) {
    if(!thread_rrd_function_provider)
        return;

    __atomic_store_n(&thread_rrd_function_provider->running, false, __ATOMIC_RELAXED);

    // wait for any cancellation requests to be dispatched;
    // the problem is that cancellation requests require a structure allocated by the collector,
    // so, while cancellation requests are being dispatched, this structure is accessed.
    // delaying the exit of the thread is required to avoid cleaning up this structure.
    //
    // mark-for-deletion + wait in one step: new dispatcher acquisitions fail
    // from the CAS on, so the drain cannot be extended by new dispatches (the
    // old 1ms-sleep retry loop left the count at 0 between retries, letting
    // new dispatchers in indefinitely)
    (void)refcount_acquire_for_deletion_and_wait(&thread_rrd_function_provider->refcount_dispatcher);

    rrd_function_provider_free(thread_rrd_function_provider);
    thread_rrd_function_provider = NULL;
}

static bool rrd_function_provider_acquire(struct rrd_function_provider *rdc) {
    if(!rdc || !rrd_function_provider_running(rdc))
        return false;

    return refcount_acquire(&rdc->refcount);
}

struct rrd_function_provider *rrd_function_provider_acquire_current_thread(void) {
    rrd_function_provider_started();

    if(!rrd_function_provider_acquire(thread_rrd_function_provider))
        internal_fatal(true, "FUNCTIONS: Trying to acquire a the current thread collector, that is currently exiting.");

    return thread_rrd_function_provider;
}

void rrd_function_provider_release(struct rrd_function_provider *rdc) {
    if(unlikely(!rdc)) return;

    if(refcount_release(&rdc->refcount) == 0)
        rrd_function_provider_free(rdc);
}

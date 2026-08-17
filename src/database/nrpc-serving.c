// SPDX-License-Identifier: GPL-3.0-or-later

#include "nrpc-serving.h"
#include "nrpc-serving-internals.h"

// Each function points to this serving handle
// so that when the serving thread exits, all of them will
// be invalidated (running == false)
// The last function using this serving handle
// frees the structure too (or when the serving thread calls
// nrpc_serving_finished()).

struct nrpc_serving_handle {
    REFCOUNT entry_refcount;
    REFCOUNT dispatcher_refcount;
    pid_t tid;
    bool running;
};

// Each thread that registers functions has to call
// nrpc_serving_started() and nrpc_serving_finished()
// to create and tear down the serving handle.

__thread struct nrpc_serving_handle *nrpc_thread_serving = NULL;

inline bool nrpc_serving_running(struct nrpc_serving_handle *serving) {
    return __atomic_load_n(&serving->running, __ATOMIC_RELAXED);
}

inline pid_t nrpc_serving_tid(struct nrpc_serving_handle *serving) {
    return serving->tid;
}

bool nrpc_serving_dispatcher_acquire(struct nrpc_serving_handle *serving) {
    return refcount_acquire(&serving->dispatcher_refcount);
}

void nrpc_serving_dispatcher_release(struct nrpc_serving_handle *serving) {
    refcount_release(&serving->dispatcher_refcount);
}

static void nrpc_serving_free(struct nrpc_serving_handle *serving) {
    if(nrpc_serving_running(serving) || !refcount_acquire_for_deletion(&serving->entry_refcount))
        // the serving handle is still referenced by registered methods.
        // leave it hanging there, the last chart will actually free it.
        return;

    // we can free it now
    freez(serving);
}

// called once per serving thread
void nrpc_serving_started(void) {
    if(!nrpc_thread_serving)
        nrpc_thread_serving = callocz(1, sizeof(struct nrpc_serving_handle));

    nrpc_thread_serving->tid = gettid_cached();
    __atomic_store_n(&nrpc_thread_serving->running, true, __ATOMIC_RELAXED);
}

// called once per serving thread
void nrpc_serving_finished(void) {
    if(!nrpc_thread_serving)
        return;

    __atomic_store_n(&nrpc_thread_serving->running, false, __ATOMIC_RELAXED);

    // wait for any cancellation requests to be dispatched;
    // the problem is that cancellation requests require a structure allocated by the serving thread,
    // so, while cancellation requests are being dispatched, this structure is accessed.
    // delaying the exit of the thread is required to avoid cleaning up this structure.
    //
    // mark-for-deletion + wait in one step: new dispatcher acquisitions fail
    // from the CAS on, so the drain cannot be extended by new dispatches (the
    // old 1ms-sleep retry loop left the count at 0 between retries, letting
    // new dispatchers in indefinitely)
    (void)refcount_acquire_for_deletion_and_wait(&nrpc_thread_serving->dispatcher_refcount);

    nrpc_serving_free(nrpc_thread_serving);
    nrpc_thread_serving = NULL;
}

static bool nrpc_serving_acquire(struct nrpc_serving_handle *serving) {
    if(!serving || !nrpc_serving_running(serving))
        return false;

    return refcount_acquire(&serving->entry_refcount);
}

struct nrpc_serving_handle *nrpc_serving_current_thread_acquire(void) {
    nrpc_serving_started();

    if(!nrpc_serving_acquire(nrpc_thread_serving))
        internal_fatal(true, "NRPC: trying to acquire the current thread's serving handle while it is exiting.");

    return nrpc_thread_serving;
}

void nrpc_serving_release(struct nrpc_serving_handle *serving) {
    if(unlikely(!serving)) return;

    if(refcount_release(&serving->entry_refcount) == 0)
        nrpc_serving_free(serving);
}

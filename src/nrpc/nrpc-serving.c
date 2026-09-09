// SPDX-License-Identifier: GPL-3.0-or-later

#include "nrpc-serving-internals.h"
#include "nrpc-lifetime.h"

// The liveness token of a thread that registers functions. Every method
// registered by that thread points at its handle, so when the thread exits,
// all of them become unavailable at once.
//
// It is the shared two-counter shell (NRPC_LIFETIME): the thread holds the
// base entry ref between nrpc_serving_started() and nrpc_serving_finished(),
// each registered method holds one more, and whoever drops the last one frees
// the handle - which is routinely AFTER the thread is gone, because methods
// outlive their serving thread until something unregisters them.
struct nrpc_serving_handle {
    NRPC_LIFETIME lifetime;
    pid_t tid;
};

__thread struct nrpc_serving_handle *nrpc_thread_serving = NULL;

inline bool nrpc_serving_running(struct nrpc_serving_handle *serving) {
    return nrpc_lifetime_alive(&serving->lifetime);
}

inline pid_t nrpc_serving_tid(struct nrpc_serving_handle *serving) {
    return serving->tid;
}

bool nrpc_serving_dispatcher_acquire(struct nrpc_serving_handle *serving) {
    return nrpc_lifetime_dispatcher_acquire(&serving->lifetime);
}

void nrpc_serving_dispatcher_release(struct nrpc_serving_handle *serving) {
    nrpc_lifetime_dispatcher_release(&serving->lifetime);
}

// called once per serving thread (idempotent - the registration paths call it
// on every register, which only refreshes the tid)
void nrpc_serving_started(void) {
    if(!nrpc_thread_serving) {
        nrpc_thread_serving = callocz(1, sizeof(struct nrpc_serving_handle));
        nrpc_lifetime_init(&nrpc_thread_serving->lifetime);
    }

    nrpc_thread_serving->tid = gettid_cached();
}

// called once per serving thread
void nrpc_serving_finished(void) {
    if(!nrpc_thread_serving)
        return;

    // The canonical NRPC_LIFETIME teardown. Retiring first is what delays
    // the thread's exit until every in-flight cancel/progress dispatch has
    // left: those dispatches read a structure this thread owns, so the thread
    // must not vanish underneath them. Marking and waiting is one step, so new
    // dispatchers cannot keep extending the drain.
    nrpc_lifetime_retire(&nrpc_thread_serving->lifetime);

    // drop the thread's base ref; registered methods may still hold theirs, and
    // then the last of them frees the handle
    if(nrpc_lifetime_entry_release(&nrpc_thread_serving->lifetime))
        freez(nrpc_thread_serving);

    nrpc_thread_serving = NULL;
}

struct nrpc_serving_handle *nrpc_serving_current_thread_acquire(void) {
    nrpc_serving_started();

    // cannot fail: the thread-local is non-NULL only while this thread holds
    // the base ref (nrpc_serving_finished() clears it)
    nrpc_lifetime_entry_acquire(&nrpc_thread_serving->lifetime);

    return nrpc_thread_serving;
}

void nrpc_serving_release(struct nrpc_serving_handle *serving) {
    if(unlikely(!serving)) return;

    if(nrpc_lifetime_entry_release(&serving->lifetime))
        freez(serving);
}

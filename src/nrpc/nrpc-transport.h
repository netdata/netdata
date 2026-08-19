// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NRPC_TRANSPORT_H
#define NETDATA_NRPC_TRANSPORT_H

#include "libnetdata/libnetdata.h"

// The refcounted lifetime shell for a function transport - the object function
// executors, cancel/progress hooks and GC use to reach the connection that
// registered a function (for pluginsd/streaming: the PARSER), without ever
// dereferencing it after death.
//
// Two refcounts, the nrpc_serving_handle shape (nrpc-serving.c):
//
// - entry refcount: one ref per holder that STORES the transport - registry
//   entries (PLUGIN/STREAM source only), the DYNCFG node pin, the
//   in-flight-call cancel/progress hook pins, the lookup-time pin, plus a base
//   ref held by the transport's owner (the parser). The struct is freed when
//   this reaches zero - possibly long after the owner died (normal for
//   streaming: registry entries survive disconnect). The destructor is
//   owner-independent: it never touches `data`.
//
// - dispatcher refcount: transient holders around every send (handler,
//   cancel hook, progress hook, GC sends): acquire-or-fail. The owner's teardown
//   marks the transport dead, drains this counter with
//   refcount_acquire_for_deletion_and_wait() (deadlock-free: entry refs live
//   on the OTHER counter), then frees the owner (`data` becomes invalid), then
//   drops the base entry ref. Post-drain acquires fail; survivors' stored
//   pointers stay a valid (dead) transport until the entries release them.
//
// This is a generic daemon-side type (not pluginsd-specific) so the function
// registry can release entry refs from its cleanup hook without an upward
// dependency on plugins.d - the same layering direction that keeps streaming
// headers out of the registry.
struct nrpc_transport {
    void *data;                     // the owner's payload (pluginsd: PARSER*); NEVER dereference without a dispatcher ref
    REFCOUNT entry_refcount;
    REFCOUNT dispatcher_refcount;   // its deletion mark IS the liveness gate: negative once teardown began
};

// created with the base entry ref (owned by the caller) and an open dispatcher counter
static inline struct nrpc_transport *nrpc_transport_create(void *data) {
    // explicit cast: this header reaches C++ TUs through the nrpc.h umbrella
    struct nrpc_transport *t = (struct nrpc_transport *)callocz(1, sizeof(struct nrpc_transport));
    t->data = data;
    t->entry_refcount = 1;          // the base ref, dropped by _owner_release()
    t->dispatcher_refcount = 0;
    return t;
}

static inline struct nrpc_transport *nrpc_transport_entry_acquire(struct nrpc_transport *t) {
    if(!t) return NULL;
    if(!refcount_acquire(&t->entry_refcount))
        // cannot happen while any holder legally stores the transport (each
        // holder owns a ref); a failure here is a use-after-release bug
        fatal("NRPC: transport entry acquire on a released transport");
    return t;
}

static inline void nrpc_transport_entry_release(struct nrpc_transport *t) {
    if(!t) return;
    if(refcount_release(&t->entry_refcount) == 0)
        // last entry ref gone; the dispatcher counter is already drained
        // (the owner drops its base ref only after the drain), so nothing
        // can reach this struct anymore
        freez(t);
}

// acquire-or-fail around a send; false once the owner started tearing down
static inline bool nrpc_transport_dispatcher_acquire(struct nrpc_transport *t) {
    if(!t) return false;
    return refcount_acquire(&t->dispatcher_refcount);
}

static inline void nrpc_transport_dispatcher_release(struct nrpc_transport *t) {
    refcount_release(&t->dispatcher_refcount);
}

// owner teardown, step 1: mark dead and drain the dispatchers - the deletion
// mark on the dispatcher refcount is itself what makes every subsequent
// acquire fail. After this returns, no dispatcher holds or can acquire the
// transport; `data` may be freed by the owner. Deadlock-free: entry refs live
// on the other counter.
static inline void nrpc_transport_mark_dead_and_drain(struct nrpc_transport *t) {
    (void)refcount_acquire_for_deletion_and_wait(&t->dispatcher_refcount);
}

// owner teardown, step 2 (after `data` was invalidated): drop the base ref
static inline void nrpc_transport_owner_release(struct nrpc_transport *t) {
    t->data = NULL;
    nrpc_transport_entry_release(t);
}

#endif // NETDATA_NRPC_TRANSPORT_H

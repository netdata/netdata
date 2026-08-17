// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NRPC_TRANSPORT_H
#define NETDATA_NRPC_TRANSPORT_H

#include "libnetdata/libnetdata.h"

// The refcounted lifetime shell for a function transport - the object function
// executors, cancellers, progressers and GC use to reach the connection that
// registered a function (for pluginsd/streaming: the PARSER), without ever
// dereferencing it after death.
//
// Two refcounts, the rrd_function_provider shape (nrpc-serving.c):
//
// - entry refcount: one ref per holder that STORES the transport - registry
//   entries (COLLECTOR/STREAMING source only), the DYNCFG node pin, the
//   broker-record canceller/progresser pins, the lookup-time pin, plus a base
//   ref held by the transport's owner (the parser). The struct is freed when
//   this reaches zero - possibly long after the owner died (normal for
//   streaming: registry entries survive disconnect). The destructor is
//   owner-independent: it never touches `data`.
//
// - dispatcher refcount: transient holders around every send (execute_cb,
//   canceller, progresser, GC sends): acquire-or-fail. The owner's teardown
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
struct rrd_function_transport {
    void *data;                     // the owner's payload (pluginsd: PARSER*); NEVER dereference without a dispatcher ref
    REFCOUNT entry_refcount;
    REFCOUNT dispatcher_refcount;   // its deletion mark IS the liveness gate: negative once teardown began
};

// created with the base entry ref (owned by the caller) and an open dispatcher counter
static inline struct rrd_function_transport *rrd_function_transport_create(void *data) {
    struct rrd_function_transport *t = callocz(1, sizeof(struct rrd_function_transport));
    t->data = data;
    t->entry_refcount = 1;          // the base ref, dropped by _owner_release()
    t->dispatcher_refcount = 0;
    return t;
}

static inline struct rrd_function_transport *rrd_function_transport_entry_acquire(struct rrd_function_transport *t) {
    if(!t) return NULL;
    if(!refcount_acquire(&t->entry_refcount))
        // cannot happen while any holder legally stores the transport (each
        // holder owns a ref); a failure here is a use-after-release bug
        fatal("FUNCTIONS TRANSPORT: entry acquire on a released transport");
    return t;
}

static inline void rrd_function_transport_entry_release(struct rrd_function_transport *t) {
    if(!t) return;
    if(refcount_release(&t->entry_refcount) == 0)
        // last entry ref gone; the dispatcher counter is already drained
        // (the owner drops its base ref only after the drain), so nothing
        // can reach this struct anymore
        freez(t);
}

// acquire-or-fail around a send; false once the owner started tearing down
static inline bool rrd_function_transport_dispatcher_acquire(struct rrd_function_transport *t) {
    if(!t) return false;
    return refcount_acquire(&t->dispatcher_refcount);
}

static inline void rrd_function_transport_dispatcher_release(struct rrd_function_transport *t) {
    refcount_release(&t->dispatcher_refcount);
}

// owner teardown, step 1: mark dead and drain the dispatchers - the deletion
// mark on the dispatcher refcount is itself what makes every subsequent
// acquire fail. After this returns, no dispatcher holds or can acquire the
// transport; `data` may be freed by the owner. Deadlock-free: entry refs live
// on the other counter.
static inline void rrd_function_transport_mark_dead_and_drain(struct rrd_function_transport *t) {
    (void)refcount_acquire_for_deletion_and_wait(&t->dispatcher_refcount);
}

// owner teardown, step 2 (after `data` was invalidated): drop the base ref
static inline void rrd_function_transport_owner_release(struct rrd_function_transport *t) {
    t->data = NULL;
    rrd_function_transport_entry_release(t);
}

#endif // NETDATA_NRPC_TRANSPORT_H

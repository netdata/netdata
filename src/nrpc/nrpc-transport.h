// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NRPC_TRANSPORT_H
#define NETDATA_NRPC_TRANSPORT_H

#include "libnetdata/libnetdata.h"
#include "nrpc-lifetime.h"

// The lifetime shell for a function transport - the object function
// executors, cancel/progress hooks and GC use to reach the connection that
// registered a function (for pluginsd/streaming: the PARSER), without ever
// dereferencing it after death.
//
// The two-counter model, the teardown order and the reason `data` may only be
// touched under a dispatcher ref are all documented once on NRPC_LIFETIME;
// this is that shell plus the owner's payload.
//
// The owner's teardown is the canonical three steps: mark_dead_and_drain(),
// then free the owner (`data` becomes invalid), then owner_release().
//
// The type is deliberately generic (not pluginsd-specific) so the function
// registry can release entry refs from its cleanup hook without an upward
// dependency on plugins.d - the same layering direction that keeps streaming
// headers out of the registry. `data` is opaque here; pluginsd is named only
// as the example of what an owner puts in it.
struct nrpc_transport {
    NRPC_LIFETIME lifetime;
    void *data;                     // the owner's payload (pluginsd: PARSER*); NEVER dereference without a dispatcher ref
};

static inline struct nrpc_transport *nrpc_transport_create(void *data) {
    // explicit cast: this header reaches C++ TUs through the umbrella header
    struct nrpc_transport *t = (struct nrpc_transport *)callocz(1, sizeof(struct nrpc_transport));
    nrpc_lifetime_init(&t->lifetime);
    t->data = data;
    return t;
}

static inline struct nrpc_transport *nrpc_transport_entry_acquire(struct nrpc_transport *t) {
    if(!t) return NULL;
    nrpc_lifetime_entry_acquire(&t->lifetime);
    return t;
}

static inline void nrpc_transport_entry_release(struct nrpc_transport *t) {
    if(!t) return;
    if(nrpc_lifetime_entry_release(&t->lifetime))
        // last entry ref gone; the dispatcher counter is already drained
        // (the owner drops its base ref only after the drain), so nothing
        // can reach this struct anymore
        freez(t);
}

// acquire-or-fail around a send; false once the owner started tearing down
static inline bool nrpc_transport_dispatcher_acquire(struct nrpc_transport *t) {
    if(!t) return false;
    return nrpc_lifetime_dispatcher_acquire(&t->lifetime);
}

static inline void nrpc_transport_dispatcher_release(struct nrpc_transport *t) {
    nrpc_lifetime_dispatcher_release(&t->lifetime);
}

// owner teardown, step 1 (the retire() of the canonical NRPC_LIFETIME
// steps): after this returns, no dispatcher holds or can acquire the
// transport, so `data` may be freed by the owner
static inline void nrpc_transport_mark_dead_and_drain(struct nrpc_transport *t) {
    nrpc_lifetime_retire(&t->lifetime);
}

// owner teardown, step 3 (after `data` was invalidated): drop the base ref
static inline void nrpc_transport_owner_release(struct nrpc_transport *t) {
    t->data = NULL;
    nrpc_transport_entry_release(t);
}

#endif // NETDATA_NRPC_TRANSPORT_H

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NRPC_LIFETIME_H
#define NETDATA_NRPC_LIFETIME_H

#include "libnetdata/libnetdata.h"

// The lifetime shell shared by the nRPC objects that outlive their creator:
// the serving handle, the transport, and the registry entry's operation
// gate. The first two use the full shell; the registry entry uses only the
// dispatcher half - its struct memory is pinned by dictionary item refcounts
// instead of entry refs, so the three teardown steps below do not apply to
// it (its own field comment carries that story).
//
// Every "gate" in this component IS this dispatcher counter - the registry's
// operation gate, the serving handle's cancel/progress dispatch gate, the
// transport's send gate; only the object being gated differs.
//
// Two counters, because they answer two different questions:
//
// - entry refs: one per party that STORES the object - method descriptors,
//   in-flight-call hook pins, the dyncfg pin, the lookup-time pin - plus the
//   BASE ref the creator holds. The object is freed when this reaches zero,
//   which may be long after the creator died (normal for streaming:
//   registered methods survive a disconnect). So nothing reachable through
//   an entry ref alone may touch creator-owned memory.
//
// - dispatcher refs: transient, taken around every USE, acquire-or-fail.
//   The deletion mark on this counter IS the liveness gate - once retire()
//   sets it, every later acquire fails and keeps failing. Holding a
//   dispatcher ref is therefore the proof that the creator's memory is still
//   valid for as long as the ref is held.
//
// For the full-shell users, teardown is always the same three steps, in
// this order:
//   1. retire()          - mark dead and drain the dispatchers
//   2. invalidate whatever the creator owned (it is now unreachable)
//   3. entry_release()   - drop the base ref; the last holder frees the object
//
// The counters are separate precisely so step 1 cannot deadlock: entry refs
// are long-lived and are NOT what retire() waits for.
typedef struct nrpc_lifetime {
    REFCOUNT entry_refcount;
    REFCOUNT dispatcher_refcount;
} NRPC_LIFETIME;

// starts with the base entry ref (owned by the creator) and an open
// dispatcher counter
static inline void nrpc_lifetime_init(NRPC_LIFETIME *l) {
    l->entry_refcount = 1;
    l->dispatcher_refcount = 0;
}

// has retire() run? a plain load, for callers that only want to know whether
// the object is still usable and are not about to use it
static inline bool nrpc_lifetime_alive(NRPC_LIFETIME *l) {
    return refcount_references(&l->dispatcher_refcount) >= 0;
}

// cannot fail while any holder legally stores the object (each holder owns a
// ref, and the creator's base ref covers the window before the first one)
static inline void nrpc_lifetime_entry_acquire(NRPC_LIFETIME *l) {
    if(!refcount_acquire(&l->entry_refcount))
        fatal("NRPC: entry acquire on a released object");
}

// true when the caller took the last ref out - it must free the object now
static inline bool nrpc_lifetime_entry_release(NRPC_LIFETIME *l) {
    return refcount_release(&l->entry_refcount) == 0;
}

static inline bool nrpc_lifetime_dispatcher_acquire(NRPC_LIFETIME *l) {
    return refcount_acquire(&l->dispatcher_refcount);
}

static inline void nrpc_lifetime_dispatcher_release(NRPC_LIFETIME *l) {
    refcount_release(&l->dispatcher_refcount);
}

// teardown step 1: mark dead and wait for the in-flight dispatchers to leave.
// The mark itself is what makes every subsequent acquire fail, so the drain
// cannot be extended by new arrivals.
static inline void nrpc_lifetime_retire(NRPC_LIFETIME *l) {
    (void)refcount_acquire_for_deletion_and_wait(&l->dispatcher_refcount);
}

#endif // NETDATA_NRPC_LIFETIME_H

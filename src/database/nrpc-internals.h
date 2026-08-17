// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NRPC_INTERNALS_H
#define NETDATA_NRPC_INTERNALS_H

#include "rrd.h"

#include "nrpc-serving-internals.h"
#include "nrpc-transport.h"

// The per-host function registry behind the opaque NRPC_REGISTRY handle.
// It owns the definitions dictionary; the host back-pointer is what the
// dictionary callbacks use to reach the host they serve.
struct nrpc_registry {
    RRDHOST *host;                  // back-pointer for the dictionary callbacks
    DICTIONARY *dict;               // the function definitions, keyed by sanitized name

    // Pending FUNCTION_DEL queue towards the parent. Deleters (any thread)
    // insert the sanitized name here BEFORE setting
    // RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED; the streaming renderer clears the
    // flag FIRST and then snapshots-and-clears this set under the spinlock, so
    // a del landing after the snapshot re-sets the flag with its entry already
    // queued and nothing is ever stranded. Only populated when the host has a
    // stream sender configured (never-streaming hosts must not grow it).
    struct {
        SPINLOCK spinlock;
        DICTIONARY *dict;           // keyed by sanitized name, no value (a set)
    } pending_dels;
};

struct nrpc_method {
    bool sync;                      // when true, the function is called synchronously
    bool unregistered;              // when true, the function is unavailable
    NRPC_METHOD_FLAGS options;   // NRPC_METHOD_FLAGS

    // who registered this entry. Swapped together with handler_data as ONE
    // pair by the conflict callback, so ownership decisions (who may overwrite,
    // and later who releases the transport) always key on the value the entry
    // actually holds.
    NRPC_SOURCE source;

    HTTP_ACCESS access;
    STRING *help;
    STRING *tags;
    int timeout;                    // the default timeout of the function
    int priority;
    uint32_t version;

    nrpc_handler_cb_t handler;
    void *handler_data;

    OBJECT_STATE_ID rrdhost_state_id;
    struct nrpc_serving_handle *serving;

    // LEAF spinlock guarding the entry's SWAPPED fields: the (source,
    // handler_data) pair, handler, and the help/tags STRINGs. The
    // conflict callback takes it (inside the dictionary index write lock)
    // around the swaps and the displaced releases/frees; readers take it
    // standalone AFTER the standard item acquire (the item ref pins the
    // entry memory, this lock pins the swapped CONTENTS) to capture the
    // execute pair or byte-copy the strings. Leaf: never acquire any other
    // lock while holding it.
    SPINLOCK leaf_spinlock;
};

// does the registration source store a transport in handler_data?
// (INTERNAL data is caller-owned and never a transport)
static inline bool nrpc_source_has_transport(NRPC_SOURCE source) {
    return source == NRPC_SOURCE_PLUGIN || source == NRPC_SOURCE_STREAM;
}

// Capture-at-find: the execute pair captured under the leaf spinlock, with the
// transport entry-pinned iff the source is transport-bearing. Executors NEVER
// re-read method->handler / handler_data at call time - a stale capture
// degrades to a clean 503 via the transport's acquire-or-fail.
struct nrpc_capture {
    nrpc_handler_cb_t handler;
    void *handler_data;
    struct nrpc_transport *transport_pin;   // entry-pinned; NULL for INTERNAL sources
};

void nrpc_method_capture(NRPC_METHOD_ACQUIRED *acquired, struct nrpc_capture *out);

static inline void nrpc_capture_release(struct nrpc_capture *c) {
    nrpc_transport_entry_release(c->transport_pin);
    c->transport_pin = NULL;
}

static inline size_t nrpc_strlen_bounded(const char *s, size_t max) {
    size_t len = strnlen(s, max + 1);
    if(unlikely(len > max))
        fatal("NRPC: string exceeds maximum supported length.");

    return len;
}

// NRPC_METHOD_ACQUIRED is an acquired item of the registry dictionary - the
// handle type is opaque outside the module, these helpers unwrap it inside.
static inline struct nrpc_method *nrpc_method_acquired_value(NRPC_METHOD_ACQUIRED *acquired) {
    return dictionary_acquired_item_value((const DICTIONARY_ITEM *)acquired);
}

bool nrpc_method_is_available(RRDHOST *host, struct nrpc_method *method);
int nrpc_registry_find(RRDHOST *host, BUFFER *wb, const char *name, size_t key_length, NRPC_METHOD_ACQUIRED **out_acquired);

#endif //NETDATA_NRPC_INTERNALS_H

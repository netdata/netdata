// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDFUNCTIONS_INTERNALS_H
#define NETDATA_RRDFUNCTIONS_INTERNALS_H

#include "rrd.h"

#include "rrdfunctions-provider-internals.h"
#include "rrdfunctions-transport.h"

// The per-host function registry behind the opaque RRD_FUNCTIONS handle.
// It owns the definitions dictionary; the host back-pointer is what the
// dictionary callbacks use to reach the host they serve.
struct rrd_functions {
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

struct rrd_host_function {
    bool sync;                      // when true, the function is called synchronously
    bool unregistered;              // when true, the function is unavailable
    RRD_FUNCTION_OPTIONS options;   // RRD_FUNCTION_OPTIONS

    // who registered this entry. Swapped together with execute_cb_data as ONE
    // pair by the conflict callback, so ownership decisions (who may overwrite,
    // and later who releases the transport) always key on the value the entry
    // actually holds.
    RRD_FUNCTION_REG_SOURCE source;

    HTTP_ACCESS access;
    STRING *help;
    STRING *tags;
    int timeout;                    // the default timeout of the function
    int priority;
    uint32_t version;

    rrd_function_execute_cb_t execute_cb;
    void *execute_cb_data;

    OBJECT_STATE_ID rrdhost_state_id;
    struct rrd_function_provider *provider;

    // LEAF spinlock guarding the entry's SWAPPED fields: the (source,
    // execute_cb_data) pair, execute_cb, and the help/tags STRINGs. The
    // conflict callback takes it (inside the dictionary index write lock)
    // around the swaps and the displaced releases/frees; readers take it
    // standalone AFTER the standard item acquire (the item ref pins the
    // entry memory, this lock pins the swapped CONTENTS) to capture the
    // execute pair or byte-copy the strings. Leaf: never acquire any other
    // lock while holding it.
    SPINLOCK leaf_spinlock;
};

// does the registration source store a transport in execute_cb_data?
// (INTERNAL data is caller-owned and never a transport)
static inline bool rrd_function_source_has_transport(RRD_FUNCTION_REG_SOURCE source) {
    return source == RRD_FUNCTION_REG_SOURCE_COLLECTOR || source == RRD_FUNCTION_REG_SOURCE_STREAMING;
}

// Capture-at-find: the execute pair captured under the leaf spinlock, with the
// transport entry-pinned iff the source is transport-bearing. Executors NEVER
// re-read rdcf->execute_cb / execute_cb_data at call time - a stale capture
// degrades to a clean 503 via the transport's acquire-or-fail.
struct rrd_function_capture {
    rrd_function_execute_cb_t execute_cb;
    void *execute_cb_data;
    struct rrd_function_transport *transport_pin;   // entry-pinned; NULL for INTERNAL sources
};

void rrd_function_acquired_capture(RRD_FUNCTION_ACQUIRED *rfa, struct rrd_function_capture *out);

static inline void rrd_function_capture_release(struct rrd_function_capture *c) {
    rrd_function_transport_entry_release(c->transport_pin);
    c->transport_pin = NULL;
}

static inline size_t rrd_functions_strlen_bounded(const char *s, size_t max) {
    size_t len = strnlen(s, max + 1);
    if(unlikely(len > max))
        fatal("RRDFUNCTIONS: string exceeds maximum supported length.");

    return len;
}

// RRD_FUNCTION_ACQUIRED is an acquired item of the registry dictionary - the
// handle type is opaque outside the module, these helpers unwrap it inside.
static inline struct rrd_host_function *rrd_function_acquired_value(RRD_FUNCTION_ACQUIRED *rfa) {
    return dictionary_acquired_item_value((const DICTIONARY_ITEM *)rfa);
}

bool rrd_function_is_available(struct rrd_host_function *rdcf, RRDHOST *host);
int rrd_functions_find_by_name(RRDHOST *host, BUFFER *wb, const char *name, size_t key_length, RRD_FUNCTION_ACQUIRED **out_acquired);

#endif //NETDATA_RRDFUNCTIONS_INTERNALS_H

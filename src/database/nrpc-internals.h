// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NRPC_INTERNALS_H
#define NETDATA_NRPC_INTERNALS_H

#include "libnetdata/libnetdata.h"

#include "nrpc.h"
#include "nrpc-serving-internals.h"
#include "nrpc-transport.h"
#include "daemon/pulse/pulse-dictionary.h"

// The per-host function registry: the value of one entry of the
// component-global registries index (see nrpc_registry_acquire), keyed by the
// host's ND_UUID identity. It owns the definitions dictionary; everything it
// may need from its owner lives in the embedded owner vtable, written only
// under owner_spinlock (init, takeover, DISARM) and snapshotted under the
// same lock by every reader. A held outer HANDLE deliberately outlives the
// entry (that is the point of the in-flight held pair): a path reached
// through such a handle after destroy finds the vtable DISARMED (name freed,
// epoch NULL, callbacks NULL) and degrades to a no-op instead of calling
// into a dying owner.
struct nrpc_registry {
    ND_UUID host_id;                // the entry's own identity (the index key, parsed back)

    // Serializes ownership transitions and every vtable access:
    // nrpc_registry_init()'s takeover of a dying predecessor's entry,
    // nrpc_registry_destroy()'s owner check + index delete + DISARM, and the
    // reader snapshots (nrpc_registry_owner_*). LOCK RULE: order is this ->
    // the outer index locks (destroy deletes while holding it) and this ->
    // the inner dictionary locks (takeover flushes while holding it) - so it
    // is NEVER taken while any inner dictionary lock is held: not in the
    // inner insert/conflict callbacks (the epoch id is pre-stamped through
    // the value bytes instead) and not inside a dfe traversal (the catalog
    // snapshots the epoch once before iterating).
    SPINLOCK owner_spinlock;
    bool dead;                      // set under owner_spinlock when destroy unlinked the entry

    // the owner vtable (see struct nrpc_registry_owner). name is the
    // component's OWN copy; epoch/data/callbacks are the owner's, valid
    // while armed. Disarm clears every field.
    struct {
        STRING *name;
        OBJECT_STATE *epoch;
        void *data;
        void (*changed)(void *data, bool arm_manifest);
        bool (*wants_del_journal)(void *data);
    } owner;

    DICTIONARY *dict;               // the function definitions, keyed by sanitized name

    // Pending FUNCTION_DEL queue towards the parent. Deleters (any thread)
    // insert the sanitized name here BEFORE the owner's changed callback
    // updates the owner's changed flag; the streaming caller clears that flag
    // FIRST and the renderer then snapshots-and-clears this set under the
    // spinlock, so a del landing after the snapshot re-sets the flag with its
    // entry already queued and nothing is ever stranded. Only populated when
    // the owner's wants_del_journal callback answers true (never-streaming
    // hosts must not grow it).
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

    OBJECT_STATE_ID epoch_id;
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

// ----------------------------------------------------------------------------
// the component-global registries index

// Acquire the registry entry of a host identity. The returned outer item pins
// the struct nrpc_registry (and, through it, the inner dictionaries' memory)
// until nrpc_registry_release() - a concurrent host teardown unlinks the entry
// from the index but cannot reclaim it while handles are held. Returns NULL
// when the identity has no live entry (unknown host, archived host,
// UUID_ZERO, or before the index exists).
struct nrpc_registry *nrpc_registry_acquire(ND_UUID host_id, const DICTIONARY_ITEM **item_out);
void nrpc_registry_release(const DICTIONARY_ITEM *item);

// A held method entry: the outer item pins the registry, the inner item pins
// the method. Self-contained - releasing needs no identity lookup, so a
// release AFTER the host's registry entry was unlinked (async completion,
// shutdown unwind) cannot dangle. Release order is inner-then-outer.
// The public opaque handle NRPC_METHOD_ACQUIRED is this struct; the call
// record embeds it by value, nrpc_method_authorize() heap-allocates it for
// its out_acquired callers.
struct nrpc_method_acquired {
    const DICTIONARY_ITEM *outer_item;  // pins the struct nrpc_registry
    const DICTIONARY_ITEM *inner_item;  // pins the struct nrpc_method
};

static inline struct nrpc_method *nrpc_method_acquired_value(NRPC_METHOD_ACQUIRED *acquired) {
    return dictionary_acquired_item_value(acquired->inner_item);
}

static inline struct nrpc_registry *nrpc_method_acquired_registry(NRPC_METHOD_ACQUIRED *acquired) {
    return dictionary_acquired_item_value(acquired->outer_item);
}

// release the held pair WITHOUT freeing the struct (for by-value holders)
void nrpc_method_acquired_release_pair(struct nrpc_method_acquired *acquired);

// _at() is PURE (no locks) - the form for traversals holding inner
// dictionary locks; the registry form snapshots the epoch itself and MUST
// NOT be called under inner dictionary locks (LOCK RULE: owner_spinlock is
// never taken inner-side, because the takeover holds it across the inner
// flush)
bool nrpc_method_is_available_at(struct nrpc_method *method, bool armed, OBJECT_STATE_ID epoch_id);
bool nrpc_method_is_available(struct nrpc_registry *registry, struct nrpc_method *method);
int nrpc_registry_find(struct nrpc_registry *registry, BUFFER *wb, const char *name, size_t key_length,
                       const DICTIONARY_ITEM **out_inner_item);

// vtable snapshots - each takes owner_spinlock, copies what it needs,
// releases, then acts; a DISARMED entry answers the safe default
void nrpc_registry_owner_changed(struct nrpc_registry *registry, bool arm_manifest);
bool nrpc_registry_owner_wants_del_journal(struct nrpc_registry *registry);
// returns false when the entry is DISARMED - and then substitutes id 0,
// which is indistinguishable from a live epoch at id 0, so callers MUST
// consult the return value, never the id alone. ONE deliberate exception:
// the register path discards it, because a method stamped with the
// substituted 0 on a disarmed entry can never become available (disarmed is
// terminal) - do not "fix" that site into taking extra locks.
bool nrpc_registry_owner_epoch(struct nrpc_registry *registry, OBJECT_STATE_ID *out_id);
STRING *nrpc_registry_owner_name_dup(struct nrpc_registry *registry); // caller string_freez()s; NULL if disarmed

#endif //NETDATA_NRPC_INTERNALS_H

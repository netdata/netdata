// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NRPC_INTERNALS_H
#define NETDATA_NRPC_INTERNALS_H

#include "libnetdata/libnetdata.h"

#include "nrpc.h"
#include "nrpc-serving-internals.h"
#include "nrpc-transport.h"

// The per-host function registry: the value of one entry of the
// component-global registries index (see nrpc_registry_acquire), keyed by the
// owning host OBJECT.
//
// Vocabulary used throughout: OUTER = that component-global index
// (owner -> registry); INNER = this registry's own dictionaries (`dict`,
// name -> slot, and `pending_dels.dict`). An "outer handle" is an acquired
// item of the outer index: it pins this struct's memory, but NOT the
// dictionaries it points to - that is the gate's job (the lifetime field).
//
// The registry owns the definitions dictionary; everything it may need from
// its owner lives in the embedded owner vtable, written only under
// owner_spinlock (init, DISARM) and snapshotted under the same lock by every
// reader. A held outer HANDLE may outlive the entry (destroy unlinks the
// entry while transient operations still hold handles): a path reached
// through such a handle after destroy finds the vtable DISARMED (name freed,
// epoch NULL, callbacks NULL) and degrades to a no-op instead of calling
// into a dying owner.
struct nrpc_registry {
    // The entry's identity: the index key, and the argument the owner's
    // callbacks receive. IMMUTABLE for the entry's whole lifetime - assigned
    // before the entry enters the index and never rewritten, not even by
    // disarm, so it is read lock-free (the key on delete, the label in logs).
    // Disarm does not need to clear it: with the callbacks NULLed there is
    // nobody left to hand it to.
    NRPC_OWNER id;

    // Serializes every vtable access: nrpc_registry_destroy()'s index delete
    // + DISARM, and the reader snapshots (nrpc_registry_owner_*). LOCK RULE:
    // order is this -> the outer index locks (destroy deletes while holding
    // it), so it is NEVER taken while any inner dictionary lock is held: not
    // in the inner insert/conflict callbacks (the epoch id is pre-stamped
    // through the value bytes instead) and not inside a dfe traversal (the
    // catalog snapshots the epoch once before iterating).
    SPINLOCK owner_spinlock;

    // the owner vtable (see struct nrpc_registry_owner). name is the
    // component's OWN copy; epoch/callbacks are the owner's, valid while
    // armed. Disarm clears every field.
    struct {
        STRING *name;
        OBJECT_STATE *epoch;
        void (*changed)(NRPC_OWNER id, bool arm_manifest);
        bool (*wants_del_journal)(NRPC_OWNER id);
    } owner;

    // The operation GATE for the inner dictionaries below (only the
    // dispatcher half of NRPC_LIFETIME is used - the outer index's item
    // refcounts already play the entry-ref role):
    //
    // - What it closes: a held outer handle pins THIS STRUCT's memory, but
    //   not the dictionaries it points to. The gate is what keeps them
    //   alive while in use.
    // - How it is taken: every public operation that reaches the inner
    //   dictionaries through an outer handle wraps its WHOLE inner-
    //   dictionary use in ONE dispatcher acquire-or-fail
    //   (nrpc_registry_dispatcher_acquire); a failed acquire is the
    //   ordinary "no live registry / not available" answer.
    // - Teardown: nrpc_registry_destroy() disarms and unlinks the entry,
    //   then retires the gate (mark dead + drain), and only then destroys
    //   the inner dictionaries - a reader either finished before the drain
    //   or can never enter again.
    // - Exempt: held descriptors (NRPC_METHOD_ACQUIRED, the call record's
    //   ref). Releasing one touches no dictionary, so late releases (async
    //   completion, shutdown unwind) need no gate and can never be refused.
    // - INVARIANT - gated sections MUST NOT block indefinitely: some
    //   destroy paths run under the rrd write lock and the retire drain
    //   waits for every gated reader, so a blocking gated section becomes
    //   an agent-wide stall. No gated section takes an rrd lock (in-gate
    //   owner access is limited to the owner_spinlock snapshots and the
    //   lock-free wants_del_journal callback; the changed callback fires
    //   outside the gate) - keep it that way. Blocking includes I/O:
    //   logging in gated sections stays level-filtered debug lines or rare
    //   warnings.
    NRPC_LIFETIME lifetime;

    DICTIONARY *dict;               // the function definitions, keyed by sanitized name

    // Pending FUNCTION_DEL queue towards the parent (the owner callback
    // name, wants_del_journal, refers to this same set). Deleters (any thread)
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

// The index key of an owner, which doubles as its label in log lines: the
// handle in hex. A dictionary key is a C string, so the raw pointer bytes
// (which contain NULs) cannot be used directly; hex is printable, fixed width,
// and stable for the entry's whole lifetime. The creation log line in
// nrpc_registry_init() maps a label back to a hostname.
#define NRPC_OWNER_KEY_LEN UINT64_HEX_MAX_LENGTH
static inline void nrpc_owner_str(char out[NRPC_OWNER_KEY_LEN], NRPC_OWNER owner) {
    print_uint64_hex_full(out, (uint64_t)(uintptr_t)owner.ptr);
}

// The method value model splits IDENTITY from STATE. Identity is the
// dictionary entry: the sanitized name keys a stable SLOT. State is one
// registration event: an IMMUTABLE, refcounted DESCRIPTOR the slot points
// to. A re-registration builds a complete new descriptor and the conflict
// callback swaps the slot's pointer as ONE unit - no field is ever mutated
// in place, so a reader that pinned a descriptor reads it lock-free for as
// long as it holds the ref, and there is nothing mutable left to race on.
//
// (Two "descriptor" nouns exist: struct nrpc_method_desc is the CALLER's
// registration input, stack-filled and never stored; the DESCRIPTOR proper
// is struct nrpc_method below, the immutable object built from it.)

// One per registration event. IMMUTABLE after construction; refcounted.
// Construction (nrpc_method_create) acquires everything the descriptor
// references - the help/tags STRING copies, the transport entry ref for
// transport-bearing sources, the serving-handle entry ref, the epoch stamp -
// and the destructor, run by whoever drops the last ref, releases them.
// No other cleanup path exists: the register unwind, the conflict
// displacement and the delete callback all funnel through
// nrpc_method_release().
struct nrpc_method {
    REFCOUNT refcount;

    bool sync;                      // when true, the function is called synchronously
    NRPC_METHOD_FLAGS options;      // NRPC_METHOD_FLAGS
    NRPC_SOURCE source;             // who registered; decides whether handler_data is a transport
    HTTP_ACCESS access;
    STRING *help;
    STRING *tags;
    int timeout;                    // the default timeout of the function
    int priority;                   // normalized at construction: 0 -> NRPC_PRIORITY_DEFAULT
    uint32_t version;

    nrpc_handler_cb_t handler;
    void *handler_data;             // transport-bearing sources: entry-pinned for the descriptor's lifetime

    // stamped at construction, BEFORE the insert (LOCK RULE: owner_spinlock
    // is never taken inside inner-dictionary callbacks, so the id travels in
    // the descriptor); 0 when the entry was already disarmed - such a
    // descriptor is never available, the correct degradation for a
    // registration racing the owner's teardown
    OBJECT_STATE_ID epoch_id;

    struct nrpc_serving_handle *serving; // entry-pinned for the descriptor's lifetime
};

// The dictionary value. The spinlock guards ONE thing: the atomicity of
// {load pointer + acquire ref [+ read tombstone]} against {swap pointer}.
// Nothing is freed, copied or compared under it. Why a lock survives at all:
// a lone refcount cannot make load-then-acquire safe (the swapper can drop
// the last ref between the reader's load and its increment); the
// alternatives are deferred reclamation - machinery this component should
// not grow - or this four-line mutual-exclusion window.
struct nrpc_method_slot {
    SPINLOCK spinlock;
    struct nrpc_method *method;

    // The delete-window TOMBSTONE: set by an unregister between its
    // validation and its dictionary_del, cleared by any re-registration,
    // read atomically with the pointer by nrpc_slot_acquire() - so lookups
    // in that window answer "unregistered by the plugin" instead of using
    // a dying entry. Slot-side, because it is a property of the NAME's
    // current lifecycle, not of any one registration.
    bool unregistered;
};

// Safe ONLY while another ref provably pins the descriptor - the slot's own
// ref under the slot lock (nrpc_slot_acquire), or a ref the caller already
// holds. This counter CANNOT detect use-after-release: refcount_acquire()
// from zero succeeds, so the mutual exclusion in nrpc_slot_acquire() is the
// guarantee, not the count. The fatal below only catches a negative
// (corrupted) counter - treat it as an assertion, never as protection.
static inline void nrpc_method_acquire(struct nrpc_method *method) {
    if(!refcount_acquire(&method->refcount))
        fatal("NRPC: method descriptor acquire on a corrupted refcount");
}

// drops one ref; the last holder runs the destructor
void nrpc_method_release(struct nrpc_method *method);

// Pin the slot's current descriptor. The returned ref is the caller's to
// release. The tombstone is read as one atomic pair with the pointer (out
// param, NULL-tolerant), so the delete-window error text can never misreport
// against a descriptor the tombstone does not belong to.
//
// PRECONDITION: the caller must be holding this slot's dictionary entry in
// place across the call - either an acquired item reference, or a read-locked
// traversal. Emptying the slot is the value destructor's job, and the two
// forms block it differently: an outstanding item reference defers the
// destructor, while the traversal's read lock excludes the write-locked delete
// it runs under. Reach a slot holding neither and the descriptor can be torn
// out from under this lock, leaving NULL to dereference.
static inline struct nrpc_method *nrpc_slot_acquire(struct nrpc_method_slot *slot, bool *unregistered_out) {
    spinlock_lock(&slot->spinlock);
    struct nrpc_method *method = slot->method;
    nrpc_method_acquire(method); // cannot fail: the slot's own ref is live under the lock
    if(unregistered_out)
        *unregistered_out = slot->unregistered;
    spinlock_unlock(&slot->spinlock);
    return method;
}

// Install a new descriptor, clearing the tombstone (a registration always
// revives the entry), and hand the displaced one back - its release NEVER
// runs under the slot lock.
static inline struct nrpc_method *nrpc_slot_swap(struct nrpc_method_slot *slot, struct nrpc_method *method) {
    spinlock_lock(&slot->spinlock);
    struct nrpc_method *displaced = slot->method;
    slot->method = method;
    slot->unregistered = false;
    spinlock_unlock(&slot->spinlock);
    return displaced;
}

// does the registration source store a transport in handler_data?
// (DAEMON-source data is caller-owned and never a transport)
static inline bool nrpc_source_has_transport(NRPC_SOURCE source) {
    return source == NRPC_SOURCE_PLUGIN || source == NRPC_SOURCE_STREAM;
}

static inline size_t nrpc_strlen_bounded(const char *s, size_t max) {
    size_t len = strnlen(s, max + 1);
    if(unlikely(len > max))
        fatal("NRPC: string exceeds maximum supported length.");

    return len;
}

// the longest command we will carry: PLUGINSD_LINE_MAX minus room for the rest
// of the FUNCTION line (keyword, transaction id, timeout, access, source and
// quoting). A command longer than this cannot be forwarded to a plugin - the
// plugin's line reader is PLUGINSD_LINE_MAX and it gives up, and its whole
// process exits, on a line it cannot terminate.
#define MAX_FUNCTION_LENGTH (PLUGINSD_LINE_MAX - 512)

// Sanitize a CALLER-SUPPLIED command into an owned buffer; NULL-tolerant
// (sanitizes to ""). The caller owns the result - CLEAN_CHAR_P frees it.
//
// TRUNCATES rather than refusing, and that is why it does NOT use
// nrpc_strlen_bounded(): a command arrives from an HTTP query parameter, so
// fatal()ing on an over-long one would hand any caller a way to kill the agent.
// Do NOT reuse this for strings the daemon itself controls - those keep
// nrpc_strlen_bounded(), whose fatal() is a real assertion about our callers.
//
// Two things the size has to respect:
//
// - the bound is MAX_FUNCTION_LENGTH, not PLUGINSD_LINE_MAX. An over-long
//   command does NOT simply fail: the lookup strips trailing words, so
//   "<real-function> <15KB of junk>" still resolves, and the WHOLE sanitized
//   command is then forwarded to the plugin as its function argument. Keeping
//   it under MAX_FUNCTION_LENGTH is what leaves the 512 bytes that constant
//   reserves for the rest of the FUNCTION line - the plugin's line reader
//   gives up and exits its process on a line it cannot terminate, so
//   staying inside the documented budget is cheap insurance.
//
// - the destination is sized for the sanitizer's worst-case GROWTH, not for
//   the input length. text_sanitize() hex-encodes each byte of an invalid
//   UTF-8 sequence into two characters, so output can be twice the input -
//   and a command truncated mid-expansion can resolve to a DIFFERENT
//   function than the caller named.
static inline char *nrpc_sanitize_name_dupz(const char *src) {
    if(!src) src = "";

    size_t len = strnlen(src, MAX_FUNCTION_LENGTH);
    size_t size = (len * 2 < MAX_FUNCTION_LENGTH ? len * 2 : MAX_FUNCTION_LENGTH) + 1;

    char *dst = mallocz(size);
    nrpc_sanitize_name(dst, src, size);
    return dst;
}

// ----------------------------------------------------------------------------
// the component-global registries index

// Acquire the registry entry of an owner. The returned outer item pins the
// struct nrpc_registry (and, through it, the inner dictionaries' memory) until
// nrpc_registry_release() - a concurrent host teardown unlinks the entry from
// the index but cannot reclaim it while handles are held. Returns NULL when
// the owner has no live entry (unknown host, archived host, NRPC_OWNER_NONE,
// or before the index exists).
struct nrpc_registry *nrpc_registry_acquire(NRPC_OWNER owner, const DICTIONARY_ITEM **item_out);
void nrpc_registry_release(const DICTIONARY_ITEM *item);

// The operation gate around inner-dictionary use (see the lifetime field in
// struct nrpc_registry): acquire-or-fail BEFORE touching registry->dict or
// registry->pending_dels through an outer handle, release when the operation's
// inner-dictionary work is done. False means destroy has started for this
// entry - answer what an absent registry answers.
static inline bool nrpc_registry_dispatcher_acquire(struct nrpc_registry *registry) {
    return nrpc_lifetime_dispatcher_acquire(&registry->lifetime);
}

static inline void nrpc_registry_dispatcher_release(struct nrpc_registry *registry) {
    nrpc_lifetime_dispatcher_release(&registry->lifetime);
}

// A held method: an OWNED descriptor reference, nothing else. Self-contained
// - releasing touches no dictionary at all, so a release AFTER the host's
// registry entry was destroyed (async completion, shutdown unwind) cannot
// dangle. The public opaque handle NRPC_METHOD_ACQUIRED is this struct;
// nrpc_method_authorize() heap-allocates it for its out_acquired callers,
// the call record holds the descriptor pointer directly.
struct nrpc_method_acquired {
    struct nrpc_method *method;
};

// _at() is PURE (no locks) on a PINNED descriptor - safe under any
// dictionary lock. The delete-window tombstone lives slot-side and is the
// CALLER's check: nrpc_slot_acquire() hands descriptor and tombstone out as
// one atomic pair. The registry form snapshots the epoch itself and MUST
// NOT be called under inner dictionary locks (LOCK RULE: owner_spinlock is
// never taken inner-side).
bool nrpc_method_is_available_at(struct nrpc_method *method, bool armed, OBJECT_STATE_ID epoch_id);
bool nrpc_method_is_available(struct nrpc_registry *registry, struct nrpc_method *method);
// `name` must already be sanitized; it is not modified (the traversal works
// on an internal copy). On HTTP_RESP_OK, *out_method is an OWNED descriptor
// ref (the caller releases it); the dictionary item is already released -
// the ref is the pin. MUST be called inside the registry's operation gate.
int nrpc_registry_find(struct nrpc_registry *registry, BUFFER *wb, const char *name,
                       struct nrpc_method **out_method);

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

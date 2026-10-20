// SPDX-License-Identifier: GPL-3.0-or-later

// Method definitions: the component-global registries index, per-owner
// registry entries and their teardown, the descriptor lifecycle, and
// register / unregister / lookup / availability.

#include "nrpc-internals.h"

// the component-owned "functions" memory-attribution category, read by the
// daemon's pulse charts
struct dictionary_stats dictionary_stats_category_functions = { .name = "functions" };

// Bytes currently held by method descriptors. They are heap allocations, not
// dictionary values, so they fall outside dictionary_stats_category_functions
// - this counter is what keeps them visible in the pulse "functions" memory
// attribution (precedent: nrpc_buffers_functions for the call BUFFERs).
size_t nrpc_methods_functions = 0;

// ----------------------------------------------------------------------------
// the descriptor lifecycle: construct -> (swap) -> release; ONE destructor

// Build one immutable descriptor from an already-validated registration.
// Runs on the register path, OUTSIDE all dictionary callbacks - which is
// what lets it acquire everything the descriptor references (the LOCK RULE
// at the insert callback forbids owner_spinlock inside inner-dict callbacks,
// and the epoch snapshot needs it). The ONLY release path, for every
// outcome (stored, displaced, deleted, unwound), is nrpc_method_release().
static struct nrpc_method *nrpc_method_create(const struct nrpc_method_desc *desc, const char *tags,
                                              NRPC_METHOD_FLAGS options, struct nrpc_registry *registry) {
    struct nrpc_method *method = callocz(1, sizeof(*method));
    __atomic_add_fetch(&nrpc_methods_functions, sizeof(*method), __ATOMIC_RELAXED);

    method->refcount = 1; // the slot's ref, transferred with the pointer on store
    method->sync = desc->sync;
    method->options = options;
    method->source = desc->source;
    method->access = desc->access;
    method->help = string_strdupz(desc->help);
    method->tags = string_strdupz(tags);
    method->timeout = desc->timeout_s;
    method->priority = desc->priority;
    method->version = desc->version;
    method->handler = desc->handler;
    method->handler_data = desc->handler_data;

    // normalized at construction so that first- and re-registration agree:
    // a priority-0 registration renders as the default on every path
    if(!method->priority)
        method->priority = NRPC_PRIORITY_DEFAULT;

    // the descriptor stores the transport (handler_data) for transport-
    // bearing sources - take the entry ref this storage owns; released by
    // the destructor under the stored tag. DAEMON-source data is caller-owned
    // and never pinned.
    if(nrpc_source_has_transport(method->source) && method->handler_data)
        nrpc_transport_entry_acquire(method->handler_data);

    method->serving = nrpc_serving_current_thread_acquire();

    // 0 when the entry is already disarmed - such a descriptor is never
    // available. Under a concurrent epoch transition the stamp can be one
    // epoch stale, which degrades to "unavailable until the next
    // re-registration" - the safe direction.
    nrpc_registry_owner_epoch(registry, &method->epoch_id);

    return method;
}

static void nrpc_method_free(struct nrpc_method *method) {
    nrpc_serving_release(method->serving);

    // release the transport ref this descriptor owns - keyed on the tag it
    // holds (descriptors are immutable, so tag and data can never diverge).
    // NULL-safe; DAEMON-source data is caller-owned and never released.
    if(nrpc_source_has_transport(method->source))
        nrpc_transport_entry_release(method->handler_data);

    string_freez(method->help);
    string_freez(method->tags);

    __atomic_sub_fetch(&nrpc_methods_functions, sizeof(*method), __ATOMIC_RELAXED);
    freez(method);
}

void nrpc_method_release(struct nrpc_method *method) {
    if(!method) return;
    if(refcount_release(&method->refcount) == 0)
        nrpc_method_free(method);
}

// Descriptors are compared by CONTENT to derive the conflict callback's
// changed verdict. STRING pointer equality IS content equality (strings are
// interned). epoch_id and serving are deliberately included: a reconnect
// re-registration carries a fresh serving handle (and usually a fresh
// epoch), so it always reports changed - which is correct, the function's
// availability just changed hands.
static bool nrpc_method_equal(struct nrpc_method *a, struct nrpc_method *b) {
    return a->sync == b->sync &&
           a->options == b->options &&
           a->source == b->source &&
           a->access == b->access &&
           a->help == b->help &&
           a->tags == b->tags &&
           a->timeout == b->timeout &&
           a->priority == b->priority &&
           a->version == b->version &&
           a->handler == b->handler &&
           a->handler_data == b->handler_data &&
           a->epoch_id == b->epoch_id &&
           a->serving == b->serving;
}

// ----------------------------------------------------------------------------
// the inner dictionary's callbacks - the value is a SLOT

// LOCK RULE for these callbacks: they run under the inner dictionary's index
// write lock, which is inner-side of owner_spinlock. Taking owner_spinlock
// here would invert that order, so they MUST NOT use the vtable snapshot
// helpers; everything owner-derived (the epoch id) was stamped into the
// descriptor at construction, before the insert.
static void nrpc_registry_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct nrpc_method_slot *slot = value;

    // the descriptor pointer arrived in the value bytes with its ref already
    // owned by this slot; only the slot's own machinery is born here
    spinlock_init(&slot->spinlock);
    slot->unregistered = false;
}

static void nrpc_registry_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value,
                                          void *data __maybe_unused) {
    struct nrpc_method_slot *slot = value;
    nrpc_method_release(slot->method);
    slot->method = NULL;
}

static bool nrpc_registry_conflict_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value,
                                            void *new_value, void *data) {
    struct nrpc_registry *registry = data;
    struct nrpc_method_slot *slot = value;
    struct nrpc_method_slot *incoming = new_value;

    // The incoming descriptor replaces the current one as ONE unit - no
    // field surgery, so there is no window in which a reader can observe a
    // half-updated registration, and nothing here needs the owner vtable.
    struct nrpc_method *installed = incoming->method;
    struct nrpc_method *displaced = nrpc_slot_swap(slot, installed);
    incoming->method = NULL; // ownership moved into the slot

    // The changed verdict feeds the dictionary's version counter and react
    // callback only - this dictionary registers no react callback and
    // nothing reads its version, so the equality-driven verdict is inert
    // beyond its own bookkeeping (do not build on it without checking).
    bool changed = !nrpc_method_equal(displaced, installed);

    if(changed) {
        char owner_str[NRPC_OWNER_KEY_LEN];
        nrpc_owner_str(owner_str, registry->id);
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: method '%s' of host %s re-registered with changes",
               dictionary_acquired_item_name(item), owner_str);
    }

    // The routine case here is an IDENTICAL re-registration - a child
    // re-sends its whole function list on every flag set and reconnect, and
    // the dictionary fires this callback even for identical values - so one
    // small descriptor is allocated and freed per function per re-send:
    // accepted churn, the price of never mutating a live registration.
    nrpc_method_release(displaced);

    return changed;
}

// ----------------------------------------------------------------------------
// the component-global registries index

static struct {
    SPINLOCK spinlock;              // guards lazy creation of the index only
    DICTIONARY *dict;               // owner handle (hex) -> struct nrpc_registry
} nrpc_registries = {
    .spinlock = SPINLOCK_INITIALIZER,
    .dict = NULL,
};

// Runs under the outer index hooks while the entry is inserted, so a
// concurrent acquire can never observe a half-initialized registry. The
// callbacks of the inner dictionary get the STORED value as their data
// pointer - the stack temporary the caller filled is only a byte source.
static void nrpc_registries_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct nrpc_registry *registry = value;

    registry->dict = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                &dictionary_stats_category_functions, sizeof(struct nrpc_method_slot));

    dictionary_register_insert_callback(registry->dict, nrpc_registry_insert_cb, registry);
    dictionary_register_delete_callback(registry->dict, nrpc_registry_delete_cb, registry);
    dictionary_register_conflict_callback(registry->dict, nrpc_registry_conflict_cb, registry);

    // eagerly created: deleters include non-stream threads (e.g. dyncfg), so
    // the set must exist for the whole registry lifetime, not on first use
    spinlock_init(&registry->pending_dels.spinlock);
    registry->pending_dels.dict = dictionary_create(DICT_OPTION_SINGLE_THREADED); // guarded by pending_dels.spinlock

    spinlock_init(&registry->owner_spinlock);

    // the operation gate opens with the entry; only the dispatcher counter is
    // used (the entry-ref base stays with nobody - the outer index's item
    // refcounts pin the struct instead)
    nrpc_lifetime_init(&registry->lifetime);
}

static void nrpc_registries_index_init(void) {
    if(__atomic_load_n(&nrpc_registries.dict, __ATOMIC_ACQUIRE)) return;

    spinlock_lock(&nrpc_registries.spinlock);
    if(!nrpc_registries.dict) {
        // No delete callback by design: nrpc_registry_destroy() is the ONLY
        // deleter of entries and destroys the inner dictionaries itself; any
        // future second deletion route would silently leak them.
        DICTIONARY *dict = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                      &dictionary_stats_category_functions, sizeof(struct nrpc_registry));
        dictionary_register_insert_callback(dict, nrpc_registries_insert_cb, NULL);
        __atomic_store_n(&nrpc_registries.dict, dict, __ATOMIC_RELEASE);
    }
    spinlock_unlock(&nrpc_registries.spinlock);
}

void nrpc_registries_destroy(void) {
    DICTIONARY *dict = __atomic_load_n(&nrpc_registries.dict, __ATOMIC_ACQUIRE);
    if(!dict) return;

    // PRECONDITION: runs after every owner destroyed its entry. An entry
    // surviving here would silently leak its inner dictionaries (the outer
    // dict has no delete callback by design). In-flight calls hold
    // descriptor references, never registry handles, so call teardown
    // ordering does not matter here; only transient operation handles reach
    // this index, and shutdown is single-threaded by this point.
    internal_fatal(dictionary_entries(dict) != 0,
                   "NRPC: the registries index still has %zu entries at shutdown",
                   (size_t)dictionary_entries(dict));

    __atomic_store_n(&nrpc_registries.dict, NULL, __ATOMIC_RELEASE);
    dictionary_destroy(dict);
}

struct nrpc_registry *nrpc_registry_acquire(NRPC_OWNER owner, const DICTIONARY_ITEM **item_out) {
    *item_out = NULL;

    DICTIONARY *dict = __atomic_load_n(&nrpc_registries.dict, __ATOMIC_ACQUIRE);
    if(!dict || !nrpc_owner_is_set(owner))
        return NULL;

    char key[NRPC_OWNER_KEY_LEN];
    nrpc_owner_str(key, owner);

    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(dict, key);
    if(!item)
        return NULL;

    *item_out = item;
    return dictionary_acquired_item_value(item);
}

void nrpc_registry_release(const DICTIONARY_ITEM *item) {
    if(!item) return;
    dictionary_acquired_item_release(__atomic_load_n(&nrpc_registries.dict, __ATOMIC_ACQUIRE), item);
}

// ----------------------------------------------------------------------------
// owner vtable snapshots: take owner_spinlock, copy what is needed, release,
// then act - so the synchronous DISARM in destroy makes every later access a
// safe no-op, and callbacks are never invoked while holding component locks

void nrpc_registry_owner_changed(struct nrpc_registry *registry, bool arm_manifest) {
    spinlock_lock(&registry->owner_spinlock);
    void (*cb)(NRPC_OWNER, bool) = registry->owner.changed;
    spinlock_unlock(&registry->owner_spinlock);

    // registry->id is immutable, so it needs no snapshot: a non-NULL callback
    // means the entry was still armed, and the id it belongs to cannot change
    if(cb)
        cb(registry->id, arm_manifest);
}

bool nrpc_registry_owner_wants_del_journal(struct nrpc_registry *registry) {
    spinlock_lock(&registry->owner_spinlock);
    bool (*cb)(NRPC_OWNER) = registry->owner.wants_del_journal;
    spinlock_unlock(&registry->owner_spinlock);

    return cb ? cb(registry->id) : false;
}

bool nrpc_registry_owner_epoch(struct nrpc_registry *registry, OBJECT_STATE_ID *out_id) {
    spinlock_lock(&registry->owner_spinlock);
    OBJECT_STATE *epoch = registry->owner.epoch;
    // deref under the lock: while armed the pointer is valid (disarm precedes
    // owner teardown on every path), and disarm NULLs it under this lock
    OBJECT_STATE_ID id = epoch ? object_state_id(epoch) : 0;
    spinlock_unlock(&registry->owner_spinlock);

    *out_id = id;
    return epoch != NULL;
}

STRING *nrpc_registry_owner_name_dup(struct nrpc_registry *registry) {
    spinlock_lock(&registry->owner_spinlock);
    STRING *name = string_dup(registry->owner.name); // NULL-safe
    spinlock_unlock(&registry->owner_spinlock);
    return name;
}

void nrpc_registry_init(const struct nrpc_registry_owner *owner) {
    if(unlikely(!nrpc_owner_is_set(owner->id))) {
        // a programming error, not a runtime condition: an unset owner would
        // key an entry nothing can ever look up again (acquire refuses the
        // unset handle), so it would sit in the index until shutdown
        internal_fatal(true, "NRPC: registry init without an owner");
        return;
    }

    if(unlikely(!owner->epoch)) {
        // also load-bearing, not just a missing feature: a NULL epoch is how
        // this component marks an entry DISARMED, so an entry created without
        // one would read as permanently doomed - and the retry below would
        // never converge. Refuse it in every build, not only in debug.
        internal_fatal(true, "NRPC: registry init without an epoch");
        return;
    }

    nrpc_registries_index_init();

    DICTIONARY *dict = __atomic_load_n(&nrpc_registries.dict, __ATOMIC_ACQUIRE);

    char key[NRPC_OWNER_KEY_LEN];
    nrpc_owner_str(key, owner->id);

    // The key IS the owner, so a live entry under this key can only be THIS
    // owner's - an owner re-initializing an entry it still holds (un-archive).
    // There is no same-key-different-owner case to arbitrate; that only existed
    // while entries were keyed on the machine guid, which two host objects
    // could share.
    //
    // What DOES need arbitrating is this owner's own destroy running
    // concurrently (owners are not required to prevent that - see the
    // NRPC_OWNER contract). A destroy disarms the entry and deletes
    // it from the index inside ONE critical section under owner_spinlock, so
    // taking that lock puts us unambiguously on one side of it:
    //
    // - armed  -> the entry is ours and alive; nothing to do.
    // - disarmed -> a destroy owns it now, and because it deleted the entry
    //   before releasing the lock we just waited on, the next lookup MUST
    //   miss. So the retry is bounded to one extra pass and lands on create.
    //
    // Without this check an init whose lookup landed just before a destroy
    // would return happily and then have its entry deleted underneath it,
    // leaving the owner with no registry at all.
    for(;;) {
        const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(dict, key);
        if(!item)
            break;

        struct nrpc_registry *existing = dictionary_acquired_item_value(item);
        OBJECT_STATE_ID ignored;
        bool armed = nrpc_registry_owner_epoch(existing, &ignored);
        dictionary_acquired_item_release(dict, item);

        if(armed)
            return;
    }

    struct nrpc_registry tmp = {
        .id = owner->id,
        // the dictionaries and the owner spinlock are initialized by the
        // outer insert callback, on the stored value; the id and the vtable
        // fields reach the stored value as tmp's bytes
        .owner = {
            .name = string_strdupz(owner->name),
            .epoch = owner->epoch,
            .changed = owner->changed,
            .wants_del_journal = owner->wants_del_journal,
        },
    };
    struct nrpc_registry *stored = dictionary_set(dict, key, &tmp, sizeof(tmp));
    if(unlikely(!stored || stored->owner.name != tmp.owner.name)) {
        // Either the index was destroyed under us, or a concurrent creator for
        // this same owner won under DONT_OVERWRITE (the index has no conflict
        // callback, so dictionary_set() hands back the entry that is already
        // there). In both cases tmp's name copy never reached an entry and is
        // still ours to free.
        string_freez(tmp.owner.name);
        return;
    }

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "NRPC: function registry %s created for host '%s'",
           key, owner->name ? owner->name : "(unset)");
}

void nrpc_registry_destroy(NRPC_OWNER owner) {
    const DICTIONARY_ITEM *item;
    struct nrpc_registry *registry = nrpc_registry_acquire(owner, &item);
    if(!registry)
        // this owner never had an entry, or it was already destroyed - both
        // are ordinary: owners may be torn down without ever having been
        // initialized, and an owner may run its teardown more than once
        // SEQUENTIALLY (the index lookup is the guard for repeats, and it is
        // keyed on the caller's own token, so it can never reach anyone
        // else's entry). CONCURRENT destroys of the same owner are the
        // owner's to serialize - two could both pass this lookup and reach
        // the dictionary destroys below twice; the only production caller
        // runs under the rrd write lock, which serializes them today.
        return;

    spinlock_lock(&registry->owner_spinlock);

    // Synchronous DISARM, the load-bearing half of the teardown safety
    // argument: from this point every vtable snapshot answers the safe
    // default (no epoch -> methods unavailable, no callbacks -> owner not
    // notified, no name -> "[disarmed]" in logs), so a mutation that already
    // holds this entry degrades to a no-op instead of calling into a dying
    // owner. In-progress snapshots taken BEFORE the disarm are bounded by
    // the owner's thread lifecycle (sender joined before destroy; receiver
    // bounded wait; dyncfg calls resolve the entry per operation).
    // registry->id is deliberately NOT cleared: it is the key this delete
    // needs and the label the logs need, and with the callbacks gone there is
    // nobody left to hand it to.
    string_freez(registry->owner.name);
    registry->owner.name = NULL;
    registry->owner.epoch = NULL;
    registry->owner.changed = NULL;
    registry->owner.wants_del_journal = NULL;

    // Delete from the index while still holding the owner lock, so disarming
    // and unlinking are ONE critical section: nothing can observe an entry
    // that is disarmed but still reachable.
    char key[NRPC_OWNER_KEY_LEN];
    nrpc_owner_str(key, owner);
    DICTIONARY *dict = __atomic_load_n(&nrpc_registries.dict, __ATOMIC_ACQUIRE);
    dictionary_del(dict, key);

    spinlock_unlock(&registry->owner_spinlock);

    // Retire the operation gate: mark it dead (every later
    // nrpc_registry_dispatcher_acquire fails, and keeps failing) and drain the
    // readers already inside their gated sections. MUST run after
    // owner_spinlock is released: gated sections snapshot the owner's epoch
    // under owner_spinlock, so retiring while holding it would deadlock
    // against the drain. The gated sections' side of the contract - they must
    // never block indefinitely - is documented on the registry's lifetime
    // field.
    nrpc_lifetime_retire(&registry->lifetime);

    // Destroy the inner dictionaries AFTER the entry left the index and the
    // gate drained, outside the index locks, so one host's teardown cannot
    // stall other hosts' lookups. The gate is the whole safety story AGAINST
    // READERS (it does nothing for a concurrent same-owner destroy - see the
    // early-return comment above): in-flight calls hold descriptor
    // references, not dictionary items, so nothing outside this function
    // keeps either dictionary alive and both destroys run the immediate-free
    // branch - and no consumer can be inside them now, or enter them again.
    // The definitions dictionary's delete callbacks release the slots'
    // descriptor refs here; descriptors still pinned by in-flight calls
    // simply outlive their dictionary. The struct memory both dictionaries
    // are reached through stays pinned by any held outer handle.
    dictionary_destroy(registry->pending_dels.dict);
    dictionary_destroy(registry->dict);

    nrpc_registry_release(item);

    // having dropped our handle, collect the deleted outer entry now (a
    // transient operation may still hold a handle - then this collects
    // nothing and the next collection gets it)
    dictionary_garbage_collect(dict);
}

void nrpc_method_acquired_release(NRPC_METHOD_ACQUIRED *acquired) {
    if(!acquired) return;
    // an owned descriptor ref, nothing else - no dictionary is touched, so
    // this needs no operation gate and is safe at any point after destroy
    nrpc_method_release(acquired->method);
    freez(acquired);
}

// ----------------------------------------------------------------------------

static inline bool nrpc_method_is_restricted(const char *name, const char *tags) {
    return (name && name[0] == '_' && name[1] == '_') || (tags && strstr(tags, NRPC_TAG_HIDDEN) != NULL);
}

// Reports whether a function name is a reserved dynamic-configuration name -
// the `config` catch-all or any per-config `config <id>` function. Exposed so
// the pluginsd FUNCTION handler can refuse external registrations that would
// otherwise overwrite (swap) the built-in dyncfg execute callback.
bool nrpc_method_name_is_dyncfg(const char *name) {
    if(!name || !*name)
        return false;

    if(strncmp(name, PLUGINSD_FUNCTION_CONFIG, sizeof(PLUGINSD_FUNCTION_CONFIG) - 1) != 0)
        return false;

    char c = name[sizeof(PLUGINSD_FUNCTION_CONFIG) - 1];
    if(c == 0 || isspace((uint8_t)c))
        return true;

    return false;
}

static inline NRPC_METHOD_FLAGS nrpc_method_flags_for(const char *name, const char *tags) {
    if(nrpc_method_name_is_dyncfg(name))
        return NRPC_METHOD_FLAG_DYNCFG;

    return nrpc_method_is_restricted(name, tags) ? NRPC_METHOD_FLAG_RESTRICTED : NRPC_METHOD_FLAG_NONE;
}

void nrpc_method_register(const struct nrpc_method_desc *desc) {
    internal_fatal(!nrpc_owner_is_set(desc->owner), "NRPC: method registration without an owner");
    internal_fatal(!desc->name, "NRPC: method registration without a name");
    internal_fatal(!desc->help, "NRPC: method registration without help text");
    internal_fatal(!desc->handler, "NRPC: method registration without a handler");
    internal_fatal(desc->source == NRPC_SOURCE_UNSET, "NRPC: method registration without a source");

    const char *name = desc->name;

    const DICTIONARY_ITEM *registry_item;
    struct nrpc_registry *registry = nrpc_registry_acquire(desc->owner, &registry_item);
    if(!registry) {
        // no live registry for this owner (unknown or archived host)
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: not registering method '%s': the given host has no function registry",
               name ? name : "(unset)");
        return;
    }

    // the operation gate: held until the inner-dictionary work is done (the
    // owner-changed notification fires outside it - a destroy in between
    // finds the vtable disarmed and the notification degrades to a no-op)
    if(!nrpc_registry_dispatcher_acquire(registry)) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: not registering method '%s': the host's function registry is being destroyed",
               name ? name : "(unset)");
        nrpc_registry_release(registry_item);
        return;
    }

    CLEAN_STRING *owner_name = nrpc_registry_owner_name_dup(registry);
    const char *host_name = owner_name ? string2str(owner_name) : "[disarmed]";

    const char *tags = desc->tags;
    if(!tags || !*tags)
        tags = "top";

    size_t key_size = nrpc_strlen_bounded(name, PLUGINSD_LINE_MAX) + 1;
    CLEAN_CHAR_P *key = mallocz(key_size);
    nrpc_sanitize_name(key, name, key_size);

    // text_sanitize() wipes a result that is entirely underscores, so a name like "__" sanitizes
    // to "". Such a name is unusable (it could never be looked up) and it would also defeat the
    // "__" prefix check in nrpc_method_is_restricted() below, so refuse it outright.
    if(unlikely(!*key)) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "NRPC: refusing to register method '%s' on host '%s': the name sanitizes to an empty string",
               name, host_name);
        nrpc_registry_dispatcher_release(registry);
        nrpc_registry_release(registry_item);
        return;
    }

    // Reserved dynamic-configuration function names ("config", "config <id>") are
    // owned exclusively by the dyncfg subsystem. A local plugin (NRPC_SOURCE_PLUGIN)
    // registering one would collide in the registry and make
    // nrpc_registry_conflict_cb() swap the built-in dyncfg execute callback
    // out, hijacking the config tree. Local plugins do dyncfg via the DYNCFG
    // protocol (not FUNCTION), so no legitimate PLUGIN-source registration uses these
    // names.
    //
    // Streaming is the exception and MUST be preserved: a child streams a single
    // synthetic "config" proxy (dyncfg_add_streaming()) so the parent can forward
    // config commands to it. That registration targets the child's host (never the
    // parent's own host), so it cannot hijack anything. DAEMON-source
    // registrations ARE the dyncfg subsystem itself.
    //
    // Everything from here on - this enforcement, the log line below, and the
    // flags classification after it - keys on the SANITIZED name: the registry
    // is indexed by the sanitized form, which strips leading spaces/control
    // chars, so a raw name like " config" or "\tconfig" must classify exactly
    // like the key it will land on (and a raw name in the log could carry
    // newlines and malform the journal).
    if(desc->source == NRPC_SOURCE_PLUGIN && nrpc_method_name_is_dyncfg(key)) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "NRPC: 'host:%s' attempted to register reserved dynamic-configuration method '%s' from a plugin. Ignoring it.",
               host_name, key);
        nrpc_registry_dispatcher_release(registry);
        nrpc_registry_release(registry_item);
        return;
    }

    NRPC_METHOD_FLAGS options = nrpc_method_flags_for(key, tags);

    // The complete descriptor is built HERE, outside all dictionary
    // callbacks: construction owns the epoch snapshot (LOCK RULE at the
    // insert callback), the serving-handle ref and the transport ref. On
    // insert the slot takes the ref over via the value bytes; on conflict
    // the callback swaps it in and releases the displaced one.
    struct nrpc_method *method = nrpc_method_create(desc, tags, options, registry);

    struct nrpc_method_slot tmp = { .method = method };

    struct nrpc_method_slot *slot = dictionary_set(registry->dict, key, &tmp, sizeof(tmp));
    if(unlikely(!slot)) {
        // Dictionary destroyed - unreachable for THIS registry while the
        // gate is held (destroy retires the gate before destroying the
        // inner dictionaries), kept as a defensive unwind. ONE destructor,
        // every path: the full release also returns the serving ref
        // construction took - anything less would leak it, and a leaked
        // serving ref permanently hangs that thread's
        // nrpc_serving_finished() drain.
        nrpc_method_release(method);
        nrpc_registry_dispatcher_release(registry);
        nrpc_registry_release(registry_item);
        return;
    }

    nrpc_registry_dispatcher_release(registry);

    // Notify the owner that the visible function set changed. Deliberately NO
    // cancellation of a queued pending_dels entry for a re-added name: a
    // stale DEL is harmless (the re-list in the same committed payload is
    // ground truth and re-affirms the function), while cancelling could
    // swallow the ONLY prune signal the parent will ever get - parents prune
    // on FUNCTION_DEL, never on a re-list.
    //
    // arm_manifest: only DYNCFG is excluded - it is derived from the key, so
    // such an entry can never enter or leave the manifest. RESTRICTED comes
    // from the tags and can be added by a re-registration, which drops the
    // function OUT of the manifest - that transition has to be reported too.
    // Fired after the insert so the report always follows the change.
    nrpc_registry_owner_changed(registry, !(options & NRPC_METHOD_FLAG_DYNCFG));

    nrpc_registry_release(registry_item);
}

bool nrpc_method_unregister(NRPC_OWNER owner, const char *name, NRPC_SOURCE source) {
    internal_fatal(source == NRPC_SOURCE_UNSET, "NRPC: method unregistration without a source");

    if(unlikely(!name || !*name))
        return false;

    const DICTIONARY_ITEM *registry_item;
    struct nrpc_registry *registry = nrpc_registry_acquire(owner, &registry_item);
    if(!registry)
        return false;

    // the operation gate: held from the lookup through the pending_dels
    // insert and the garbage collection (the owner-changed notification
    // fires outside it)
    if(!nrpc_registry_dispatcher_acquire(registry)) {
        nrpc_registry_release(registry_item);
        return false;
    }

    size_t key_size = nrpc_strlen_bounded(name, PLUGINSD_LINE_MAX) + 1;
    CLEAN_CHAR_P *key = mallocz(key_size);
    nrpc_sanitize_name(key, name, key_size);

    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(registry->dict, key);
    if(!item) {
        nrpc_registry_dispatcher_release(registry);
        nrpc_registry_release(registry_item);
        return false;
    }

    struct nrpc_method_slot *slot = dictionary_acquired_item_value(item);

    // Pin the descriptor this delete is authorized against. Every check
    // below reads THIS descriptor - immutable, so none of them can race a
    // concurrent re-registration - and the identity check further down
    // verifies the slot still holds it before anything is tombstoned.
    struct nrpc_method *method = nrpc_slot_acquire(slot, NULL);

    if(source == NRPC_SOURCE_PLUGIN) {
        if(!nrpc_thread_serving || method->serving != nrpc_thread_serving) {
            nd_log(NDLS_DAEMON, NDLP_WARNING,
                   "NRPC: refusing to unregister method '%s' - "
                   "serving-thread mismatch (registered by %s, unregister requested by %s)",
                   name,
                   method->serving ? "another serving thread" : "unknown",
                   nrpc_thread_serving ? "current serving thread" : "a thread with no serving handle");
            nrpc_method_release(method);
            dictionary_acquired_item_release(registry->dict, item);
            nrpc_registry_dispatcher_release(registry);
            nrpc_registry_release(registry_item);
            return false;
        }
    }

    // Both del verdicts come from the descriptor the identity check below
    // authorizes - never from a fresh slot read, which could belong to a
    // registration this delete has no business judging:
    //
    // dyncfg config functions are intentionally never streamed to parents
    // (nrpc_catalog_render_global_functions skips NRPC_METHOD_FLAG_DYNCFG),
    // so their removals must not be streamed either;
    bool is_dyncfg = (method->options & NRPC_METHOD_FLAG_DYNCFG);

    // and whether this function is part of the cloud manifest, deciding the
    // manifest arm after the delete
    bool in_manifest = !(method->options & (NRPC_METHOD_FLAG_DYNCFG | NRPC_METHOD_FLAG_RESTRICTED));

    // The FUNCTION_DEL protocol command (from external plugins or streaming
    // children) must never remove dyncfg config functions; those are owned and
    // removed exclusively by the dyncfg subsystem (DAEMON-source deletes).
    if(source != NRPC_SOURCE_DAEMON && is_dyncfg) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "NRPC: refusing to unregister dyncfg method '%s' via FUNCTION_DEL", name);
        nrpc_method_release(method);
        dictionary_acquired_item_release(registry->dict, item);
        nrpc_registry_dispatcher_release(registry);
        nrpc_registry_release(registry_item);
        return false;
    }

    // IDENTITY CHECK + tombstone, one slot-locked section: verify the slot
    // still holds the descriptor the checks above authorized (pointer
    // compare - ABA-free, our ref prevents recycling) before tombstoning.
    // This closes the check-then-delete TOCTOU: a re-registration landing
    // between our checks and this point makes the compare fail and the
    // delete aborts, instead of removing a registration it never judged.
    // The tombstone makes the function unavailable to concurrent lookups in
    // the delete window and lets nrpc_registry_find() report the specific
    // "unregistered by the plugin" error there.
    //
    // Two CONCURRENT unregisters can both pass this check (the delete
    // callback is deferred while item references are held, so the slot
    // still points at the descriptor for both) and both return true - the
    // del, the queue insert and the flag set are all idempotent, so that
    // is harmless.
    spinlock_lock(&slot->spinlock);
    bool still_current = (slot->method == method);
    if(still_current)
        slot->unregistered = true;
    spinlock_unlock(&slot->spinlock);

    if(!still_current) {
        // The refusal window includes byte-identical re-sends: every
        // registration event builds a fresh descriptor, so an identical
        // re-list racing this delete also fails the pointer compare. The
        // registration stays (the delete judged a superseded event), no
        // FUNCTION_DEL is queued, and the next full re-list re-affirms the
        // function - both sides converge on the re-list as ground truth.
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: not unregistering method '%s' - it was re-registered while the delete was validating", name);
        nrpc_method_release(method);
        dictionary_acquired_item_release(registry->dict, item);
        nrpc_registry_dispatcher_release(registry);
        nrpc_registry_release(registry_item);
        return false;
    }

    // KNOWN, ACCEPTED window: between the slot unlock above and the del
    // below, a conflict re-registration can swap in a fresh descriptor
    // (clearing the tombstone) and the key-based del still removes it. The
    // slot lock cannot be held across dictionary_del - lock order is
    // index -> slot, and the inversion would ABBA against the conflict
    // callback. The loss self-heals on the next full FUNCTION re-list: the
    // re-list is ground truth, DELs are a change-log.
    dictionary_del(registry->dict, key);
    dictionary_acquired_item_release(registry->dict, item);

    // Queue the FUNCTION_DEL for the streaming renderer instead of sending it
    // synchronously: the deleting thread must never block on sender
    // backpressure, and the renderer (which owns the wire format) drains the
    // queue on the next flag poll or reconnect push. Ordering contract: insert
    // BEFORE the owner's changed callback updates the flag - the renderer
    // clears the flag first and then snapshots the set, so a del can never be
    // stranded (see the comment on pending_dels). Not populated
    // when the owner has no del journal consumer (a LIVE question through the
    // vtable - the owner's sender can appear and disappear), else it would
    // grow for the process lifetime on never-streaming hosts. dyncfg global
    // deletes keep their quirk: they report the change but never emit
    // FUNCTION_DEL.
    if(!is_dyncfg && nrpc_registry_owner_wants_del_journal(registry)) {
        spinlock_lock(&registry->pending_dels.spinlock);
        dictionary_set(registry->pending_dels.dict, key, NULL, 0);
        spinlock_unlock(&registry->pending_dels.spinlock);
    }

    dictionary_garbage_collect(registry->dict);

    nrpc_method_release(method);
    nrpc_registry_dispatcher_release(registry);

    // after the delete, so the report always follows the change; the manifest
    // is armed only for entries it can contain (not DYNCFG, not RESTRICTED)
    nrpc_registry_owner_changed(registry, in_manifest);

    nrpc_registry_release(registry_item);
    return true;
}

// PURE availability check on a PINNED descriptor against a pre-snapshotted
// epoch - takes no locks, safe under any dictionary lock. The delete-window
// tombstone lives slot-side and is the CALLER's check (nrpc_slot_acquire
// hands it out atomically with the descriptor). armed == false (a disarmed
// entry, its owner tearing down) makes every method unavailable.
bool nrpc_method_is_available_at(struct nrpc_method *method, bool armed, OBJECT_STATE_ID epoch_id) {
    if(!nrpc_serving_running(method->serving))
        return false;

    if(!armed)
        return false;

    if(method->epoch_id != epoch_id)
        return false;

    return true;
}

// convenience for call sites that hold NO inner dictionary lock (it takes
// owner_spinlock through the epoch snapshot - see the LOCK RULE at the
// insert callback); traversals snapshot once and use the _at() form
bool nrpc_method_is_available(struct nrpc_registry *registry, struct nrpc_method *method) {
    OBJECT_STATE_ID epoch_id;
    bool armed = nrpc_registry_owner_epoch(registry, &epoch_id);
    return nrpc_method_is_available_at(method, armed, epoch_id);
}

int nrpc_registry_find(struct nrpc_registry *registry, BUFFER *wb, const char *name,
                       struct nrpc_method **out_method) {
    // A MUTABLE right-sized copy: the loop below strips one trailing word at a
    // time, so "fn arg1 arg2" degrades to "fn arg1" and then to "fn". The
    // caller's string must survive intact - the in-flight record keeps it.
    size_t key_length = strnlen(name, MAX_FUNCTION_LENGTH);
    CLEAN_CHAR_P *buffer = mallocz(key_length + 1);
    memcpy(buffer, name, key_length);
    buffer[key_length] = '\0';

    // an INDEX, not a pointer: the strip walks down to the start of the buffer
    // and a pointer form would have to form `buffer - 1` to terminate, which is
    // undefined behaviour (harmless in practice, but it is one-past-the-start
    // of a heap block)
    size_t strip = key_length;

    OBJECT_STATE_ID epoch_id = 0;
    bool armed = nrpc_registry_owner_epoch(registry, &epoch_id);

    bool found = false;
    bool was_unregistered = false;
    struct nrpc_method *method = NULL;
    while (buffer[0]) {
        const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(registry->dict, buffer);
        if(item) {
            found = true;

            struct nrpc_method_slot *slot = dictionary_acquired_item_value(item);
            bool unregistered;
            method = nrpc_slot_acquire(slot, &unregistered);

            // the descriptor ref IS the pin - the item can go at once, and
            // prefix-retry intermediates never leak a ref past this loop
            dictionary_acquired_item_release(registry->dict, item);

            if(!unregistered && nrpc_method_is_available_at(method, armed, epoch_id)) {
                break;
            }
            else {
                CLEAN_STRING *owner_name = nrpc_registry_owner_name_dup(registry);
                nd_log(NDLS_DAEMON, NDLP_DEBUG,
                       "NRPC: method '%s' is not available. "
                       "host '%s', serving = { tid: %d, running: %s }, epoch { owner: %u, stamped: %u }",
                       name,
                       owner_name ? string2str(owner_name) : "[disarmed]",
                       nrpc_serving_tid(method->serving),
                       nrpc_serving_running(method->serving) ? "yes" : "no",
                       epoch_id, method->epoch_id
                       );

                was_unregistered = unregistered;
                nrpc_method_release(method);
                method = NULL;
            }
        }

        // Strip a word from the end, so "fn arg1 arg2" resolves to "fn".
        // Consequence worth knowing: an over-long command does not simply
        // fail - "<real-fn> <junk>" still resolves, and the WHOLE sanitized
        // command is forwarded to the plugin (which is why the sanitizer
        // bounds it to MAX_FUNCTION_LENGTH).
        while (strip > 0 && !isspace((uint8_t)buffer[strip - 1])) buffer[--strip] = '\0';

        // strip all spaces
        while (strip > 0 && isspace((uint8_t)buffer[strip - 1])) buffer[--strip] = '\0';
    }

    buffer_flush(wb);

    *out_method = method;

    if(!method) {
        if(found) {
            if(was_unregistered)
                return nrpc_call_error(wb,
                                               "This function has been unregistered by the plugin.",
                                               HTTP_RESP_SERVICE_UNAVAILABLE);
            else
                return nrpc_call_error(wb,
                                               "The plugin that registered this feature, is not currently running.",
                                               HTTP_RESP_SERVICE_UNAVAILABLE);
        }
        else
            return nrpc_call_error(wb,
                                           "This feature is not available on this host at this time.",
                                           HTTP_RESP_NOT_FOUND);
    }

    return HTTP_RESP_OK;
}

bool nrpc_method_available(NRPC_OWNER owner, const char *function) {
    const DICTIONARY_ITEM *registry_item;
    struct nrpc_registry *registry = nrpc_registry_acquire(owner, &registry_item);
    if(!registry)
        return false;

    // the operation gate: a registry whose destroy started has no available
    // methods - the same answer an absent registry gives
    if(!nrpc_registry_dispatcher_acquire(registry)) {
        nrpc_registry_release(registry_item);
        return false;
    }

    bool ret = false;
    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(registry->dict, function);
    if(item) {
        struct nrpc_method_slot *slot = dictionary_acquired_item_value(item);
        bool unregistered;
        struct nrpc_method *method = nrpc_slot_acquire(slot, &unregistered);
        if(!unregistered && nrpc_method_is_available(registry, method))
            ret = true;

        nrpc_method_release(method);
        dictionary_acquired_item_release(registry->dict, item);
    }

    nrpc_registry_dispatcher_release(registry);
    nrpc_registry_release(registry_item);
    return ret;
}

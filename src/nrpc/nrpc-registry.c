// SPDX-License-Identifier: GPL-3.0-or-later

#include "nrpc-internals.h"

#define MAX_FUNCTION_LENGTH (PLUGINSD_LINE_MAX - 512) // we need some space for the rest of the line

// the component-owned "functions" memory-attribution category, read by the
// daemon's pulse charts (declared in nrpc.h)
struct dictionary_stats dictionary_stats_category_functions = { .name = "functions" };

// ----------------------------------------------------------------------------

// Each host owns a function registry (struct nrpc_registry) holding its
// function definitions, keyed by sanitized method name. The registries
// themselves live in a component-global index keyed by the host OBJECT
// (NRPC_OWNER), so an owner needs no component field inside its own struct.

// ----------------------------------------------------------------------------

// LOCK RULE for this callback and the conflict callback below: they run under
// the inner dictionary's index write lock, which is inner-side of
// owner_spinlock. Taking owner_spinlock here would invert that order, so these
// callbacks MUST NOT use the vtable snapshot helpers; everything they need
// from the owner (the epoch id) is snapshotted by the register path BEFORE the
// insert and travels in the value bytes. The log lines label the entry by its
// IMMUTABLE id instead of the owner's name, which would need the lock.
static void nrpc_registry_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *func, void *data __maybe_unused) {
    struct nrpc_method *method = func;

    nrpc_serving_started();
    method->serving = nrpc_serving_current_thread_acquire();

    // method->epoch_id was pre-stamped by nrpc_method_register() from the
    // owner's epoch (0 when the entry was already disarmed - such a method is
    // never available, the correct degradation for a registration racing the
    // owner's teardown)
    method->unregistered = false;

    spinlock_init(&method->leaf_spinlock);

    // the entry stores the transport (handler_data) for transport-bearing
    // sources - take the entry ref this storage owns; released by
    // nrpc_method_cleanup() under the stored tag
    if(nrpc_source_has_transport(method->source) && method->handler_data)
        nrpc_transport_entry_acquire(method->handler_data);

    if(!method->priority)
        method->priority = NRPC_PRIORITY_DEFAULT;
}

static void nrpc_method_cleanup(struct nrpc_method *method) {
    nrpc_serving_release(method->serving);

    // release the transport ref this (tag, data) pair owns - keyed on the tag
    // the struct actually HOLDS (the conflict callback swaps them as one pair,
    // so the displaced pair always lands here together). NULL-safe; INTERNAL
    // data is caller-owned and never released.
    if(nrpc_source_has_transport(method->source))
        nrpc_transport_entry_release(method->handler_data);

    string_freez(method->help);
    string_freez(method->tags);
}

static void nrpc_registry_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *func,
                                          void *data __maybe_unused) {
    struct nrpc_method *method = func;
    nrpc_method_cleanup(method);
}

static bool nrpc_registry_conflict_cb(const DICTIONARY_ITEM *item __maybe_unused, void *func,
                                            void *new_func, void *data) {
    struct nrpc_registry *registry = data;
    struct nrpc_method *method = func;
    struct nrpc_method *new_method = new_func;

    // NO vtable access here (see the LOCK RULE above the insert callback):
    // the epoch id arrives pre-stamped in new_method, and the log lines carry
    // the entry's immutable identity instead of a name that would need
    // the owner lock to read safely
    OBJECT_STATE_ID state_id = new_method->epoch_id;
    char owner_str[NRPC_OWNER_KEY_LEN];
    nrpc_owner_str(owner_str, registry->id);

    nrpc_serving_started();

    bool changed = false;

    if(__atomic_load_n(&method->unregistered, __ATOMIC_ACQUIRE)) {
        __atomic_store_n(&method->unregistered, false, __ATOMIC_RELEASE);
        changed = true;
    }

    if(method->serving != nrpc_thread_serving) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: method '%s' of host %s changed serving thread from %d to %d",
               dictionary_acquired_item_name(item), owner_str,
               nrpc_serving_tid(method->serving), nrpc_serving_tid(nrpc_thread_serving));

        new_method->serving = method->serving;
        method->serving = nrpc_serving_current_thread_acquire();
        changed = true;
    }

    if(method->epoch_id != state_id) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: method '%s' of host %s changed state id from %u to %u",
               dictionary_acquired_item_name(item), owner_str,
               method->epoch_id, state_id);

        method->epoch_id = state_id;
        changed = true;
    }

    // Everything from here to the end swaps fields that concurrent readers
    // capture or byte-copy under the entry's leaf spinlock (an item reference
    // pins the entry memory but NOT the swapped contents - a displaced STRING
    // or transport can be freed the moment it leaves the entry). Take the leaf
    // lock around the swaps AND the displaced releases/frees below, so a
    // reader either sees the old consistent contents or the new ones.
    spinlock_lock(&method->leaf_spinlock);

    if(method->handler != new_method->handler) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: method '%s' of host %s changed handler",
               dictionary_acquired_item_name(item), owner_str);

        SWAP(method->handler, new_method->handler);
        changed = true;
    }

    if(method->help != new_method->help) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: method '%s' of host %s changed help text",
               dictionary_acquired_item_name(item), owner_str);

        SWAP(method->help, new_method->help);
        changed = true;
    }

    if(method->tags != new_method->tags) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: method '%s' of host %s changed tags",
               dictionary_acquired_item_name(item), owner_str);

        SWAP(method->tags, new_method->tags);
        changed = true;
    }

    if(method->options != new_method->options) {
        // options are derived from the name and the tags (nrpc_method_flags_for());
        // keeping them in sync with the swapped tags is what makes a re-registration
        // with the "hidden" tag actually restrict the function
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: method '%s' of host %s changed flags",
               dictionary_acquired_item_name(item), owner_str);

        SWAP(method->options, new_method->options);
        changed = true;
    }

    if(method->timeout != new_method->timeout) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: method '%s' of host %s changed timeout (from %d to %d)",
               dictionary_acquired_item_name(item), owner_str,
               method->timeout, new_method->timeout);

        SWAP(method->timeout, new_method->timeout);
        changed = true;
    }

    if(method->version != new_method->version) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: method '%s' of host %s changed version (from %"PRIu32", to %"PRIu32")",
               dictionary_acquired_item_name(item), owner_str,
               method->version, new_method->version);

        SWAP(method->version, new_method->version);
        changed = true;
    }

    if(method->priority != new_method->priority) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: method '%s' of host %s changed priority",
               dictionary_acquired_item_name(item), owner_str);

        SWAP(method->priority, new_method->priority);
        changed = true;
    }

    if(method->access != new_method->access) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: method '%s' of host %s changed access level",
               dictionary_acquired_item_name(item), owner_str);

        SWAP(method->access, new_method->access);
        changed = true;
    }

    if(method->sync != new_method->sync) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: method '%s' of host %s changed sync/async mode",
               dictionary_acquired_item_name(item), owner_str);

        SWAP(method->sync, new_method->sync);
        changed = true;
    }

    // (source, handler_data) swap as ONE pair inside one conditional - never
    // as two independently-conditional swaps - so a later cleanup/release always
    // keys on the ownership tag matching the data it actually holds.
    //
    // Transport accounting (the serving-handle pattern in this same callback, see
    // new_method->serving above): when a DIFFERENT pair is installed, acquire
    // an entry ref iff the INSTALLED tag is transport-bearing; the displaced
    // pair lands in new_method for nrpc_method_cleanup(new_method) below to
    // release under the DISPLACED tag. When NO swap occurs (equal pair -
    // routine: a child re-sends its whole function list on every flag set and
    // reconnect, and the dictionary fires this callback even for identical
    // values), NEUTRALIZE new_method (data NULL, non-transport tag) so the
    // unconditional cleanup release is a no-op. Invariant: an equal-pair
    // conflict nets ZERO refs - by neutralization, not by skipping the
    // release.
    if(method->handler_data != new_method->handler_data || method->source != new_method->source) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: method '%s' of host %s changed handler data or registration source",
               dictionary_acquired_item_name(item), owner_str);

        if(nrpc_source_has_transport(new_method->source) && new_method->handler_data)
            nrpc_transport_entry_acquire(new_method->handler_data);

        SWAP(method->handler_data, new_method->handler_data);
        SWAP(method->source, new_method->source);
        changed = true;
    }
    else {
        new_method->handler_data = NULL;
        new_method->source = NRPC_SOURCE_DAEMON;
    }

    // under the leaf lock: frees the DISPLACED strings/transport ref, which a
    // concurrent leaf-locked reader may otherwise still be copying
    nrpc_method_cleanup(new_method);

    spinlock_unlock(&method->leaf_spinlock);

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
                                                &dictionary_stats_category_functions, sizeof(struct nrpc_method));

    dictionary_register_insert_callback(registry->dict, nrpc_registry_insert_cb, registry);
    dictionary_register_delete_callback(registry->dict, nrpc_registry_delete_cb, registry);
    dictionary_register_conflict_callback(registry->dict, nrpc_registry_conflict_cb, registry);

    // eagerly created: deleters include non-stream threads (e.g. dyncfg), so
    // the set must exist for the whole registry lifetime, not on first use
    spinlock_init(&registry->pending_dels.spinlock);
    registry->pending_dels.dict = dictionary_create(DICT_OPTION_SINGLE_THREADED); // guarded by pending_dels.spinlock

    spinlock_init(&registry->owner_spinlock);
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

    // PRECONDITION: runs after every owner destroyed its entry, and after
    // nrpc_inflight_calls_destroy() released every held pair - no outer
    // handle may survive this point, or its release would reach a NULL index.
    // An entry surviving here would also silently leak its inner dictionaries
    // (the outer dict has no delete callback by design).
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
    // NRPC_OWNER contract in nrpc.h). A destroy disarms the entry and deletes
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
        // initialized, and an owner may run its teardown more than once.
        // The index lookup is the whole guard: it is keyed on the caller's own
        // token, so it can never reach anyone else's entry.
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

    // Destroy the inner dictionaries AFTER the entry left the index and
    // outside the index locks, so one host's teardown cannot stall other
    // hosts' lookups. The definitions dictionary's destroy defers while
    // references exist (in-flight calls); pending_dels holds no references
    // and dies immediately. The struct memory both are reached through stays
    // pinned by any held outer handle, so a late release through call->held
    // cannot dangle.
    dictionary_destroy(registry->pending_dels.dict);
    dictionary_destroy(registry->dict);

    nrpc_registry_release(item);
    dictionary_garbage_collect(dict);
}

void nrpc_method_acquired_release_pair(struct nrpc_method_acquired *acquired) {
    // release order: inner (the method entry) first, then the outer handle
    // that pins the registry the inner dictionary belongs to
    if(acquired->inner_item) {
        struct nrpc_registry *registry = nrpc_method_acquired_registry(acquired);
        dictionary_acquired_item_release(registry->dict, acquired->inner_item);
        acquired->inner_item = NULL;
    }
    if(acquired->outer_item) {
        nrpc_registry_release(acquired->outer_item);
        acquired->outer_item = NULL;
    }
}

void nrpc_method_acquired_release(NRPC_METHOD_ACQUIRED *acquired) {
    if(!acquired) return;
    nrpc_method_acquired_release_pair(acquired);
    freez(acquired);
}

// Capture-at-find: snapshot the execute pair under the entry's leaf spinlock,
// entry-pinning the transport for transport-bearing sources. The caller holds
// the acquired item (pinning the entry memory); the leaf lock closes the
// window where a concurrent re-registration swaps the pair and frees the
// displaced transport between our read and our pin. The pin attaches ONLY to
// the entry the find returned - never to prefix-retry intermediates, which
// are released inside the find loop before this can run.
void nrpc_method_capture(NRPC_METHOD_ACQUIRED *acquired, struct nrpc_capture *out) {
    struct nrpc_method *method = nrpc_method_acquired_value(acquired);

    spinlock_lock(&method->leaf_spinlock);
    out->handler = method->handler;
    out->handler_data = method->handler_data;
    out->transport_pin = (nrpc_source_has_transport(method->source) && method->handler_data)
                             ? nrpc_transport_entry_acquire(method->handler_data)
                             : NULL;
    spinlock_unlock(&method->leaf_spinlock);
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
    // parent's own host), so it cannot hijack anything. INTERNAL
    // registrations ARE the dyncfg subsystem (dyncfg.c, dyncfg-tree.c).
    //
    // Enforced on the SANITIZED key: the registry is indexed by the sanitized
    // form, which strips leading spaces/control chars, so a raw name like
    // " config" or "\tconfig" classifies exactly like the key it would land on.
    if(desc->source == NRPC_SOURCE_PLUGIN && nrpc_method_name_is_dyncfg(key)) {
        // Log the sanitized name (what we actually classify on), not the raw name:
        // a raw name with leading whitespace/control chars or embedded newlines
        // would make this log line misleading or malformed.
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "NRPC: 'host:%s' attempted to register reserved dynamic-configuration method '%s' from a plugin. Ignoring it.",
               host_name, key);
        nrpc_registry_release(registry_item);
        return;
    }

    // Classify from the sanitized key, not the raw name: the key is what the dictionary is
    // indexed by, so classifying the raw name lets " config" land on the "config" key without
    // the DYNCFG option, which would hand a dyncfg-reserved entry to a regular function.
    // Captured before the insert because the conflict callback swaps options with the
    // previous value, leaving tmp.options holding the old one.
    NRPC_METHOD_FLAGS options = nrpc_method_flags_for(key, tags);

    struct nrpc_method tmp = {
        .serving = NULL,
        .sync = desc->sync,
        .timeout = desc->timeout_s,
        .version = desc->version,
        .priority = desc->priority,
        .options = options,
        .access = desc->access,
        .source = desc->source,
        .handler = desc->handler,
        .handler_data = desc->handler_data,
        .help = string_strdupz(desc->help),
        .tags = string_strdupz(tags),
    };

    // Snapshot the owner's epoch as close to the insert as possible: the
    // insert and conflict callbacks run under the inner index write lock and
    // must not touch the vtable (LOCK RULE at the insert callback), so the id
    // travels in the value bytes. 0 when the entry is already disarmed - such
    // a method is never available. Under a concurrent epoch transition the
    // stamp can be one epoch stale, which degrades to "unavailable until the
    // next re-registration" - the safe direction, and the same race class the
    // old in-callback read had (the epoch is mutated by threads that never
    // hold the inner lock).
    nrpc_registry_owner_epoch(registry, &tmp.epoch_id);

    const DICTIONARY_ITEM *item = dictionary_set_and_acquire_item(registry->dict, key, &tmp, sizeof(tmp));
    if(unlikely(!item)) {
        // dictionary destroyed (host teardown or shutdown in progress) -
        // neither the insert nor the conflict callback ran, so tmp's strings
        // are still ours and no transport ref was taken; unwind cleanly
        string_freez(tmp.help);
        string_freez(tmp.tags);
        nrpc_registry_release(registry_item);
        return;
    }

    dictionary_acquired_item_release(registry->dict, item);

    // Notify the owner that the visible function set changed. Deliberately NO
    // cancellation of a queued pending_dels entry here: the queue is a
    // change-log and the renderer's full re-list is the ground truth in the
    // same committed payload, so a stale DEL for a re-added function heals
    // within one drain (DEL lines render first, then the re-list re-affirms
    // it). Cancelling here can swallow the ONLY prune signal the parent will
    // ever get: a concurrent del can land its registry delete AFTER this
    // insert while its queue insert lands BEFORE the cancellation, leaving
    // the function absent from the registry with no DEL queued - and parents
    // prune only on FUNCTION_DEL, never on a re-list.
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

    size_t key_size = nrpc_strlen_bounded(name, PLUGINSD_LINE_MAX) + 1;
    CLEAN_CHAR_P *key = mallocz(key_size);
    nrpc_sanitize_name(key, name, key_size);

    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(registry->dict, key);
    if(!item) {
        nrpc_registry_release(registry_item);
        return false;
    }

    struct nrpc_method *method = dictionary_acquired_item_value(item);

    if(source == NRPC_SOURCE_PLUGIN) {
        if(!nrpc_thread_serving || method->serving != nrpc_thread_serving) {
            nd_log(NDLS_DAEMON, NDLP_WARNING,
                   "NRPC: refusing to unregister method '%s' - "
                   "serving-thread mismatch (registered by %s, unregister requested by %s)",
                   name,
                   method->serving ? "another serving thread" : "unknown",
                   nrpc_thread_serving ? "current serving thread" : "a thread with no serving handle");
            dictionary_acquired_item_release(registry->dict, item);
            nrpc_registry_release(registry_item);
            return false;
        }
    }

    // dyncfg config functions are intentionally never streamed to parents
    // (nrpc_catalog_render_global_functions skips NRPC_METHOD_FLAG_DYNCFG),
    // so their removals must not be streamed either. Capture this before releasing.
    bool is_dyncfg = (method->options & NRPC_METHOD_FLAG_DYNCFG);

    // whether this function is part of the cloud manifest; captured here because
    // method is released (and may be freed) before we can queue the manifest refresh
    bool in_manifest = !(method->options & (NRPC_METHOD_FLAG_DYNCFG | NRPC_METHOD_FLAG_RESTRICTED));

    // The FUNCTION_DEL protocol command (from external plugins or streaming
    // children) must never remove dyncfg config functions; those are owned and
    // removed exclusively by the dyncfg subsystem (INTERNAL deletes).
    if(source != NRPC_SOURCE_DAEMON && is_dyncfg) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "NRPC: refusing to unregister dyncfg method '%s' via FUNCTION_DEL", name);
        dictionary_acquired_item_release(registry->dict, item);
        nrpc_registry_release(registry_item);
        return false;
    }

    // Mark the function unregistered before removing it from the dictionary.
    // The flag makes the function unavailable to any thread that already holds
    // an acquired reference during the delete window (it cannot be freed until
    // that reference is released), and lets nrpc_registry_find() report
    // a specific "unregistered by the plugin" error in that window.
    __atomic_store_n(&method->unregistered, true, __ATOMIC_RELEASE);

    // Delete from the index while still holding our acquired reference, then
    // release: this unlinks the item before any concurrent re-registration could
    // resurrect it through the conflict callback (a re-add now allocates a fresh
    // item). The value is freed once the last reference - this one plus any
    // in-flight execution - is released.
    dictionary_del(registry->dict, key);
    dictionary_acquired_item_release(registry->dict, item);

    // Queue the FUNCTION_DEL for the streaming renderer instead of sending it
    // synchronously: the deleting thread must never block on sender
    // backpressure, and the renderer (which owns the wire format) drains the
    // queue on the next flag poll or reconnect push. Ordering contract: insert
    // BEFORE the owner's changed callback updates the flag - the renderer
    // clears the flag first and then snapshots the set, so a del can never be
    // stranded (see the struct comment in nrpc-internals.h). Not populated
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

    // after the delete, so the report always follows the change; the manifest
    // is armed only for entries it can contain (not DYNCFG, not RESTRICTED)
    nrpc_registry_owner_changed(registry, in_manifest);

    nrpc_registry_release(registry_item);
    return true;
}

// PURE availability check against a pre-snapshotted epoch - safe under any
// dictionary lock (takes none). armed == false (a disarmed entry, its owner
// tearing down) makes every method unavailable.
bool nrpc_method_is_available_at(struct nrpc_method *method, bool armed, OBJECT_STATE_ID epoch_id) {
    if(__atomic_load_n(&method->unregistered, __ATOMIC_ACQUIRE))
        return false;

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

int nrpc_registry_find(struct nrpc_registry *registry, BUFFER *wb, const char *name, size_t key_length,
                       const DICTIONARY_ITEM **out_inner_item) {
    char buffer[MAX_FUNCTION_LENGTH + 1];
    strncpyz(buffer, name, sizeof(buffer) - 1);
    key_length = strnlen(buffer, sizeof(buffer));
    char *s = NULL;

    OBJECT_STATE_ID state_id = 0;
    bool armed = nrpc_registry_owner_epoch(registry, &state_id);

    bool found = false;
    bool was_unregistered = false;
    const DICTIONARY_ITEM *item = NULL;
    while (buffer[0]) {
        if((item = dictionary_get_and_acquire_item(registry->dict, buffer))) {
            found = true;

            struct nrpc_method *method = dictionary_acquired_item_value(item);
            if(nrpc_method_is_available_at(method, armed, state_id)) {
                break;
            }
            else {
                // the owner's stream-thread details this line used to carry
                // are host internals the component no longer sees
                CLEAN_STRING *owner_name = nrpc_registry_owner_name_dup(registry);
                nd_log(NDLS_DAEMON, NDLP_DEBUG,
                       "Method '%s' is not available. "
                       "host '%s', serving = { tid: %d, running: %s }, host state { id: %u, expected %u }",
                       name,
                       owner_name ? string2str(owner_name) : "[disarmed]",
                       nrpc_serving_tid(method->serving),
                       nrpc_serving_running(method->serving) ? "yes" : "no",
                       state_id, method->epoch_id
                       );

                was_unregistered = __atomic_load_n(&method->unregistered, __ATOMIC_ACQUIRE);
                dictionary_acquired_item_release(registry->dict, item);
                item = NULL;
            }
        }

        // if s == NULL, set it to the end of the buffer;
        // this should happen only the first time
        if (unlikely(!s))
            s = &buffer[key_length - 1];

        // skip a word from the end
        while (s >= buffer && !isspace((uint8_t)*s)) *s-- = '\0';

        // skip all spaces
        while (s >= buffer && isspace((uint8_t)*s)) *s-- = '\0';
    }

    buffer_flush(wb);

    *out_inner_item = item;

    if(!item) {
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

    bool ret = false;
    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(registry->dict, function);
    if(item) {
        struct nrpc_method *method = dictionary_acquired_item_value(item);
        if(nrpc_method_is_available(registry, method))
            ret = true;

        dictionary_acquired_item_release(registry->dict, item);
    }

    nrpc_registry_release(registry_item);
    return ret;
}

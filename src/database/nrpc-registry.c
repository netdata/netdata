// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd.h"
#include "nrpc-internals.h"

#define MAX_FUNCTION_LENGTH (PLUGINSD_LINE_MAX - 512) // we need some space for the rest of the line

// ----------------------------------------------------------------------------

// Each host identity owns a function registry (struct nrpc_registry) holding
// its function definitions, keyed by sanitized method name. The registries
// themselves live in a component-global index keyed by the host's ND_UUID,
// so the component needs no field inside RRDHOST.

// ----------------------------------------------------------------------------

static void nrpc_registry_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *func, void *data) {
    struct nrpc_registry *registry = data;
    RRDHOST *host = __atomic_load_n(&registry->host, __ATOMIC_ACQUIRE);
    struct nrpc_method *method = func;

    nrpc_serving_started();
    method->serving = nrpc_serving_current_thread_acquire();
    method->rrdhost_state_id = object_state_id(&host->state_id);
    method->unregistered = false;

    spinlock_init(&method->leaf_spinlock);

    // the entry stores the transport (handler_data) for transport-bearing
    // sources - take the entry ref this storage owns; released by
    // nrpc_method_cleanup() under the stored tag
    if(nrpc_source_has_transport(method->source) && method->handler_data)
        nrpc_transport_entry_acquire(method->handler_data);

    if(!method->priority)
        method->priority = NRPC_PRIORITY_DEFAULT;

//    internal_error(true, "NRPC: registering method '%s' on host '%s', serving tid %d, %s",
//                   dictionary_acquired_item_name(item), rrdhost_hostname(host),
//                   method->serving->tid, method->serving->running ? "running" : "NOT running");
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
    RRDHOST *host = __atomic_load_n(&registry->host, __ATOMIC_ACQUIRE); (void)host;
    struct nrpc_method *method = func;
    struct nrpc_method *new_method = new_func;

    nrpc_serving_started();

    bool changed = false;

    if(__atomic_load_n(&method->unregistered, __ATOMIC_ACQUIRE)) {
        __atomic_store_n(&method->unregistered, false, __ATOMIC_RELEASE);
        changed = true;
    }

    if(method->serving != nrpc_thread_serving) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: method '%s' of host '%s' changed serving thread from %d to %d",
               dictionary_acquired_item_name(item), rrdhost_hostname(host),
               nrpc_serving_tid(method->serving), nrpc_serving_tid(nrpc_thread_serving));

        new_method->serving = method->serving;
        method->serving = nrpc_serving_current_thread_acquire();
        changed = true;
    }

    if(method->rrdhost_state_id != object_state_id(&host->state_id)) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: method '%s' of host '%s' changed state id from %u to %u",
               dictionary_acquired_item_name(item), rrdhost_hostname(host),
               method->rrdhost_state_id,
               object_state_id(&host->state_id));

        method->rrdhost_state_id = object_state_id(&host->state_id);
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
               "NRPC: method '%s' of host '%s' changed handler",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        SWAP(method->handler, new_method->handler);
        changed = true;
    }

    if(method->help != new_method->help) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: method '%s' of host '%s' changed help text",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        SWAP(method->help, new_method->help);
        changed = true;
    }

    if(method->tags != new_method->tags) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: method '%s' of host '%s' changed tags",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        SWAP(method->tags, new_method->tags);
        changed = true;
    }

    if(method->options != new_method->options) {
        // options are derived from the name and the tags (nrpc_method_flags_for());
        // keeping them in sync with the swapped tags is what makes a re-registration
        // with the "hidden" tag actually restrict the function
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: method '%s' of host '%s' changed flags",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        SWAP(method->options, new_method->options);
        changed = true;
    }

    if(method->timeout != new_method->timeout) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: method '%s' of host '%s' changed timeout (from %d to %d)",
               dictionary_acquired_item_name(item), rrdhost_hostname(host),
               method->timeout, new_method->timeout);

        SWAP(method->timeout, new_method->timeout);
        changed = true;
    }

    if(method->version != new_method->version) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: method '%s' of host '%s' changed version (from %"PRIu32", to %"PRIu32")",
               dictionary_acquired_item_name(item), rrdhost_hostname(host),
               method->version, new_method->version);

        SWAP(method->version, new_method->version);
        changed = true;
    }

    if(method->priority != new_method->priority) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: method '%s' of host '%s' changed priority",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        SWAP(method->priority, new_method->priority);
        changed = true;
    }

    if(method->access != new_method->access) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: method '%s' of host '%s' changed access level",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        SWAP(method->access, new_method->access);
        changed = true;
    }

    if(method->sync != new_method->sync) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: method '%s' of host '%s' changed sync/async mode",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

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
               "NRPC: method '%s' of host '%s' changed handler data or registration source",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

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

//    internal_error(true, "NRPC: registering method '%s' on host '%s', serving tid %d, %s",
//                   dictionary_acquired_item_name(item), rrdhost_hostname(host),
//                   method->serving->tid, method->serving->running ? "running" : "NOT running");

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
    DICTIONARY *dict;               // compact host uuid -> struct nrpc_registry
} nrpc_registries = {
    .spinlock = SPINLOCK_INITIALIZER,
    .dict = NULL,
};

static inline void nrpc_registry_key(char out[UUID_COMPACT_STR_LEN], ND_UUID host_id) {
    uuid_unparse_lower_compact(host_id.uuid, out);
}

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
    registry->dead = false;
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

    // PRECONDITION: runs after rrdhost_free_all() (every entry deleted) and
    // nrpc_inflight_calls_destroy() (every held pair released) - no outer
    // handle may survive this point, or its release would reach a NULL index.
    // An entry surviving here would also silently leak its inner dictionaries
    // (the outer dict has no delete callback by design).
    internal_fatal(dictionary_entries(dict) != 0,
                   "NRPC: the registries index still has %zu entries at shutdown",
                   (size_t)dictionary_entries(dict));

    __atomic_store_n(&nrpc_registries.dict, NULL, __ATOMIC_RELEASE);
    dictionary_destroy(dict);
}

struct nrpc_registry *nrpc_registry_acquire(ND_UUID host_id, const DICTIONARY_ITEM **item_out) {
    *item_out = NULL;

    DICTIONARY *dict = __atomic_load_n(&nrpc_registries.dict, __ATOMIC_ACQUIRE);
    if(!dict || UUIDiszero(host_id))
        return NULL;

    char key[UUID_COMPACT_STR_LEN];
    nrpc_registry_key(key, host_id);

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

void nrpc_registry_init(RRDHOST *host) {
    nrpc_registries_index_init();

    if(UUIDiszero(host->host_id)) {
        // A host with an unparsable machine guid has no identity to key an
        // entry on (two such hosts would collide on the zero key), so it gets
        // no function registry - everything degrades to no-ops for it. Not
        // internal_fatal: the dbengine unittest deliberately creates its
        // fixture host with a non-uuid machine guid.
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "NRPC: not creating the function registry of host '%s': its machine guid did not parse, so it has no host identity",
               rrdhost_hostname(host));
        return;
    }

    DICTIONARY *dict = __atomic_load_n(&nrpc_registries.dict, __ATOMIC_ACQUIRE);

    char key[UUID_COMPACT_STR_LEN];
    nrpc_registry_key(key, host->host_id);

    for(;;) {
        const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(dict, key);
        if(!item)
            break;

        struct nrpc_registry *existing = dictionary_acquired_item_value(item);

        spinlock_lock(&existing->owner_spinlock);

        if(existing->dead) {
            // a destroy unlinked this entry after our lookup found it; its
            // index delete already ran (same critical section that set dead),
            // so retrying the lookup now misses and we create fresh
            spinlock_unlock(&existing->owner_spinlock);
            dictionary_acquired_item_release(dict, item);
            continue;
        }

        if(existing->host == host) {
            // idempotency (e.g. un-archive re-init on the same RRDHOST)
            spinlock_unlock(&existing->owner_spinlock);
            dictionary_acquired_item_release(dict, item);
            return;
        }

        // TAKEOVER: a dying same-identity predecessor was unlinked from the
        // host index (rrd_wrlock released) but has not reached
        // nrpc_registry_destroy() yet - the documented teardown gap of the
        // unregister-node path. This host won rrdhost_index_add_by_guid(), so
        // the entry is rightfully its: flush the predecessor's methods and
        // re-point ownership. The predecessor's destroy will then find OUR
        // ownership under this same lock and leave the entry alone.
        //
        // The flush runs under rrd_wrlock + owner_spinlock, which is safe
        // because the method delete callbacks are bounded and lock-free:
        // nrpc_method_cleanup() only does refcount releases and freez/
        // string_freez - no path back into any rrd or component lock.
        dictionary_flush(existing->dict);
        spinlock_lock(&existing->pending_dels.spinlock);
        dictionary_flush(existing->pending_dels.dict);
        spinlock_unlock(&existing->pending_dels.spinlock);
        RRDHOST *old_host = existing->host;
        __atomic_store_n(&existing->host, host, __ATOMIC_RELEASE);

        spinlock_unlock(&existing->owner_spinlock);

        // the predecessor is only unlinked at this instant, not yet freed,
        // so its hostname is still readable
        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "NRPC: host '%s' took over the function-registry entry of dying same-identity predecessor '%s'",
               rrdhost_hostname(host), rrdhost_hostname(old_host));

        dictionary_acquired_item_release(dict, item);
        return;
    }

    struct nrpc_registry tmp = {
        .host_id = host->host_id,
        .host = host,
        // the dictionaries and the owner spinlock are initialized by the
        // outer insert callback, on the stored value
    };
    dictionary_set(dict, key, &tmp, sizeof(tmp));
}

void nrpc_registry_destroy(RRDHOST *host) {
    const DICTIONARY_ITEM *item;
    struct nrpc_registry *registry = nrpc_registry_acquire(host->host_id, &item);
    if(!registry)
        return;

    spinlock_lock(&registry->owner_spinlock);

    if(registry->host != host || registry->dead) {
        // Ownership guard: either a machine-guid collision loser (freed
        // without ever being indexed) that must never delete the winner's
        // live entry, or a dying predecessor whose entry a same-identity
        // successor already took over (init's takeover) - in both cases the
        // entry belongs to someone else now and its inner dictionaries are
        // that owner's to destroy.
        spinlock_unlock(&registry->owner_spinlock);
        nrpc_registry_release(item);
        return;
    }

    // Delete from the index while still holding the owner lock: setting dead
    // and unlinking in ONE critical section is what lets init's takeover path
    // trust that once it observes dead, the stale item is already unlinked -
    // a successor entry under the same key can only be created after this
    // delete, so this delete can never hit a successor's entry.
    registry->dead = true;

    char key[UUID_COMPACT_STR_LEN];
    nrpc_registry_key(key, host->host_id);
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
    internal_fatal(UUIDiszero(desc->host_id), "NRPC: method registration without a host identity");
    internal_fatal(!desc->name, "NRPC: method registration without a name");
    internal_fatal(!desc->help, "NRPC: method registration without help text");
    internal_fatal(!desc->handler, "NRPC: method registration without a handler");
    internal_fatal(desc->source == NRPC_SOURCE_UNSET, "NRPC: method registration without a source");

    const char *name = desc->name;

    const DICTIONARY_ITEM *registry_item;
    struct nrpc_registry *registry = nrpc_registry_acquire(desc->host_id, &registry_item);
    if(!registry) {
        // no live registry for this identity (unknown or archived host)
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "NRPC: not registering method '%s': the given host identity has no function registry",
               name ? name : "(unset)");
        return;
    }

    RRDHOST *host = __atomic_load_n(&registry->host, __ATOMIC_ACQUIRE);

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
               name, rrdhost_hostname(host));
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
    // parent's localhost built-in), so it cannot hijack anything. INTERNAL
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
               rrdhost_hostname(host), key);
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

    // Deliberately NO cancellation of a queued pending_dels entry here:
    // the queue is a change-log and the renderer's full re-list is the
    // ground truth in the same committed payload, so a stale DEL for a
    // re-added function heals within one drain (DEL lines render first,
    // then the re-list re-affirms it). Cancelling here can swallow the
    // ONLY prune signal the parent will ever get: a concurrent del can
    // land its registry delete AFTER this insert while its queue insert
    // lands BEFORE the cancellation, leaving the function absent from the
    // registry with no DEL queued - and parents prune only on
    // FUNCTION_DEL, never on a re-list.
    rrdhost_flag_set(host, RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED);

    dictionary_acquired_item_release(registry->dict, item);

    // Refresh the cloud function manifest. Only DYNCFG is excluded: it is derived from the
    // key, so such an entry can never enter or leave the manifest. RESTRICTED comes from the
    // tags and can be added by a re-registration, which drops the function OUT of the
    // manifest - that transition has to be reported too, so it must not be filtered here.
    // Placed after the insert so the arm always follows the change it reports.
    if(!(options & NRPC_METHOD_FLAG_DYNCFG))
        aclk_arm_node_manifest(host);

    nrpc_registry_release(registry_item);
}

bool nrpc_method_unregister(ND_UUID host_id, const char *name, NRPC_SOURCE source) {
    internal_fatal(source == NRPC_SOURCE_UNSET, "NRPC: method unregistration without a source");

    if(unlikely(!name || !*name))
        return false;

    const DICTIONARY_ITEM *registry_item;
    struct nrpc_registry *registry = nrpc_registry_acquire(host_id, &registry_item);
    if(!registry)
        return false;

    RRDHOST *host = __atomic_load_n(&registry->host, __ATOMIC_ACQUIRE);

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
    // (stream_sender_send_host_functions skips NRPC_METHOD_FLAG_DYNCFG),
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
    // BEFORE setting the flag - the renderer clears the flag first and then
    // snapshots the set, so a del can never be stranded (see the struct
    // comment in nrpc-internals.h). Not populated when the host has no
    // sender configured, else it would grow for the process lifetime on
    // never-streaming hosts. dyncfg global deletes keep their quirk: they set
    // the flag but never emit FUNCTION_DEL.
    if(!is_dyncfg && rrdhost_has_stream_sender_enabled(host)) {
        spinlock_lock(&registry->pending_dels.spinlock);
        dictionary_set(registry->pending_dels.dict, key, NULL, 0);
        spinlock_unlock(&registry->pending_dels.spinlock);
    }

    rrdhost_flag_set(host, RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED);

    dictionary_garbage_collect(registry->dict);

    // after the delete, so the arm always follows the change it reports
    if(in_manifest)
        aclk_arm_node_manifest(host);

    nrpc_registry_release(registry_item);
    return true;
}

bool nrpc_method_is_available(RRDHOST *host, struct nrpc_method *method) {
    if(__atomic_load_n(&method->unregistered, __ATOMIC_ACQUIRE))
        return false;

    if(!nrpc_serving_running(method->serving))
        return false;

    if(method->rrdhost_state_id != object_state_id(&host->state_id))
        return false;

    return true;
}

int nrpc_registry_find(struct nrpc_registry *registry, BUFFER *wb, const char *name, size_t key_length,
                       const DICTIONARY_ITEM **out_inner_item) {
    char buffer[MAX_FUNCTION_LENGTH + 1];
    strncpyz(buffer, name, sizeof(buffer) - 1);
    key_length = strnlen(buffer, sizeof(buffer));
    char *s = NULL;

    RRDHOST *host = __atomic_load_n(&registry->host, __ATOMIC_ACQUIRE);
    OBJECT_STATE_ID state_id = object_state_id(&host->state_id);

    bool found = false;
    bool was_unregistered = false;
    const DICTIONARY_ITEM *item = NULL;
    while (buffer[0]) {
        if((item = dictionary_get_and_acquire_item(registry->dict, buffer))) {
            found = true;

            struct nrpc_method *method = dictionary_acquired_item_value(item);
            if(nrpc_method_is_available(host, method)) {
                break;
            }
            else {

                nd_log(NDLS_DAEMON, NDLP_DEBUG,
                       "Method '%s' is not available. "
                       "host '%s', serving = { tid: %d, running: %s }, host tid { rcv: %d, snd: %d }, host state { id: %u, expected %u }, hops: %d",
                       name,
                       rrdhost_hostname(host),
                       nrpc_serving_tid(method->serving),
                       nrpc_serving_running(method->serving) ? "yes" : "no",
                       host->stream.rcv.status.tid, host->stream.snd.status.tid,
                       state_id, method->rrdhost_state_id,
                       rrdhost_ingestion_hops(host)
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

bool nrpc_method_available(RRDHOST *host, const char *function) {
    if(!host)
        return false;

    const DICTIONARY_ITEM *registry_item;
    struct nrpc_registry *registry = nrpc_registry_acquire(host->host_id, &registry_item);
    if(!registry)
        return false;

    bool ret = false;
    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(registry->dict, function);
    if(item) {
        struct nrpc_method *method = dictionary_acquired_item_value(item);
        if(nrpc_method_is_available(host, method))
            ret = true;

        dictionary_acquired_item_release(registry->dict, item);
    }

    nrpc_registry_release(registry_item);
    return ret;
}

// SPDX-License-Identifier: GPL-3.0-or-later

// Publishing the registry: the shared iteration core plus every consumer -
// the streaming FUNCTION re-list, the JSON views, the dictionary exporter,
// and the cloud node-instance manifest.

#include "nrpc-internals.h"

// ----------------------------------------------------------------------------
// the iteration core

// Walks the host's registry dictionary and hands the callback a
// self-contained view of every visited entry.
//
// Each visit pins the entry's CURRENT descriptor once (nrpc_slot_acquire)
// and reads it lock-free: the descriptor is immutable, so the view's
// help/tags are borrowed pointers valid for the whole visit - no byte
// copies, no per-entry lock. A concurrent re-registration swaps the slot to
// a new descriptor but cannot touch or free the pinned one.
static size_t nrpc_catalog_registry_foreach(struct nrpc_registry *registry, NRPC_CATALOG_FILTER filter,
                                            nrpc_method_view_cb_t cb, void *data) {
    size_t dyncfg_count = 0;

    // Snapshot the owner's epoch ONCE, before the traversal: dfe holds the
    // inner items read lock for the whole loop, and taking owner_spinlock
    // under it would invert against the takeover's owner -> inner flush (the
    // LOCK RULE at the registry's insert callback). One consistent epoch for
    // the whole pass is also the correct visibility semantics.
    OBJECT_STATE_ID epoch_id = 0;
    bool armed = nrpc_registry_owner_epoch(registry, &epoch_id);

    struct nrpc_method_slot *slot;
    dfe_start_read(registry->dict, slot) {
        bool unregistered;
        struct nrpc_method *m = nrpc_slot_acquire(slot, &unregistered);

        if(unregistered || !nrpc_method_is_available_at(m, armed, epoch_id)) {
            nrpc_method_release(m);
            continue;
        }

        struct nrpc_method_view v = { .name = slot_dfe.name, .options = m->options };
        bool visit = false;

        switch(filter) {
            case NRPC_CATALOG_FILTER_USER:
                visit = !(v.options & (NRPC_METHOD_FLAG_DYNCFG | NRPC_METHOD_FLAG_RESTRICTED));
                break;

            case NRPC_CATALOG_FILTER_STREAM_GLOBAL:
                if(v.options & NRPC_METHOD_FLAG_DYNCFG)
                    dyncfg_count++;
                visit = !(v.options & NRPC_METHOD_FLAG_DYNCFG);
                break;
        }

        if(visit) {
            v.timeout = m->timeout;
            v.access = m->access;
            v.priority = m->priority;
            v.version = m->version;
            v.help = string2str(m->help);
            v.tags = string2str(m->tags);
            cb(&v, data);
        }

        nrpc_method_release(m);
    }
    dfe_done(slot);

    return dyncfg_count;
}

size_t nrpc_catalog_host_foreach(NRPC_OWNER owner, NRPC_CATALOG_FILTER filter, nrpc_method_view_cb_t cb, void *data) {
    const DICTIONARY_ITEM *registry_item;
    struct nrpc_registry *registry = nrpc_registry_acquire(owner, &registry_item);
    if(!registry) return 0;

    // the operation gate (every catalog consumer takes it around its whole
    // traversal): a registry whose destroy started visits nothing - the same
    // answer an absent one gives
    if(!nrpc_registry_dispatcher_acquire(registry)) {
        nrpc_registry_release(registry_item);
        return 0;
    }

    size_t dyncfg_count = nrpc_catalog_registry_foreach(registry, filter, cb, data);

    nrpc_registry_dispatcher_release(registry);
    nrpc_registry_release(registry_item);
    return dyncfg_count;
}

// ----------------------------------------------------------------------------
// the streaming emitter

static void stream_global_function_cb(const struct nrpc_method_view *v, void *data) {
    BUFFER *wb = data;

    buffer_sprintf(wb
                   , PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\" \"%s\" "HTTP_ACCESS_FORMAT" %d %"PRIu32"\n"
                   , v->name
                   , v->timeout
                   , v->help
                   , v->tags
                   , (HTTP_ACCESS_FORMAT_CAST)v->access
                   , v->priority
                   , v->version
    );
}

// Renders the parent-facing view of this host's global functions: first the
// queued FUNCTION_DEL lines, then the full FUNCTION re-list - one buffer,
// one commit by the caller.
//
// CALLER DUTIES:
// - clear the host's functions-changed flag BEFORE calling. That clear is
//   the streaming side's half of the ordering protocol with
//   nrpc_method_unregister(): the deleter queues its name BEFORE re-setting
//   the flag, and this renderer snapshots-and-clears the queue under its
//   lock - so a del landing after our snapshot re-sets the flag with its
//   entry already queued, and the next poll drains it. Nothing is lost.
// - hold the sender's global_functions_spinlock across {render + commit}.
//   The two streaming callers run on DIFFERENT threads (the flag poll on
//   collection/receiver threads, the reconnect push on the sender thread);
//   without it, a stale rendered buffer could commit its FUNCTION_DEL lines
//   after a fresh re-list.
//
// `can_function_del` is the streaming caller's verdict
// (STREAM_CAP_FUNCTION_DEL + metadata readiness) - this renderer
// deliberately knows nothing about streaming. When the verdict is false the
// queue snapshot is DISCARDED, which is what prevents unbounded growth on
// parents without FUNCDEL support.
//
// RETURNS the count of dyncfg-backed entries seen: the caller appends the
// synthetic "config" line AFTER this returns and BEFORE its commit, still
// under its render+commit lock, when the count is non-zero and its peer has
// the DYNCFG capability - dyncfg is an application on top of this
// component, so the verdict and the line are the caller's.
size_t nrpc_catalog_render_global_functions(NRPC_OWNER owner, BUFFER *wb, bool can_function_del) {
    // a host without a live registry entry (an archived host racing the
    // sender's flag poll) emits nothing: the caller already cleared the flag
    const DICTIONARY_ITEM *registry_item;
    struct nrpc_registry *registry = nrpc_registry_acquire(owner, &registry_item);
    if(!registry)
        return 0;

    // the operation gate, covering BOTH inner dictionaries: the pending_dels
    // snapshot below is what makes destroy's immediate free of that
    // dictionary safe (pending_dels.spinlock does not serialize against
    // destroy - only the gate does). It ends when this renderer returns; the
    // caller's commit happens outside it.
    if(!nrpc_registry_dispatcher_acquire(registry)) {
        nrpc_registry_release(registry_item);
        return 0;
    }

    spinlock_lock(&registry->pending_dels.spinlock);
    if(dictionary_entries(registry->pending_dels.dict)) {
        if(can_function_del) {
            void *t;
            dfe_start_read(registry->pending_dels.dict, t) {
                buffer_sprintf(wb, PLUGINSD_KEYWORD_FUNCTION_DEL " GLOBAL \"%s\"\n", t_dfe.name);
            }
            dfe_done(t);
        }
        dictionary_flush(registry->pending_dels.dict);
    }
    spinlock_unlock(&registry->pending_dels.spinlock);

    size_t configs = nrpc_catalog_registry_foreach(registry, NRPC_CATALOG_FILTER_STREAM_GLOBAL,
                                                   stream_global_function_cb, wb);

    nrpc_registry_dispatcher_release(registry);
    nrpc_registry_release(registry_item);
    return configs;
}

// ----------------------------------------------------------------------------
// the JSON renderer

static void host_function2json_cb(const struct nrpc_method_view *v, void *data) {
    BUFFER *wb = data;

    buffer_json_member_add_object(wb, v->name);
    {
        buffer_json_member_add_string(wb, "help", v->help);
        buffer_json_member_add_int64(wb, "timeout", v->timeout);
        buffer_json_member_add_uint64(wb, "version", (uint64_t)v->version);
        buffer_json_member_add_array(wb, "options");
        {
            // every registered method is host-wide; the constant "GLOBAL"
            // is a wire-compatibility artifact of the retired per-chart
            // scope and consumers still expect it
            buffer_json_add_array_item_string(wb, "GLOBAL");
        }
        buffer_json_array_close(wb);
        buffer_json_member_add_string(wb, "tags", v->tags);
        http_access2buffer_json_array(wb, "access", v->access);
        buffer_json_member_add_uint64(wb, "priority", v->priority);
    }
    buffer_json_object_close(wb);
}

void nrpc_catalog_host2json(NRPC_OWNER owner, BUFFER *wb) {
    // the "functions" key is OMITTED entirely when the host has no live
    // registry entry - it is not emitted as an empty object
    const DICTIONARY_ITEM *registry_item;
    struct nrpc_registry *registry = nrpc_registry_acquire(owner, &registry_item);
    if(!registry) return;

    // the operation gate: a destroy in progress omits the key, exactly like
    // an absent registry
    if(!nrpc_registry_dispatcher_acquire(registry)) {
        nrpc_registry_release(registry_item);
        return;
    }

    buffer_json_member_add_object(wb, "functions");

    nrpc_catalog_registry_foreach(registry, NRPC_CATALOG_FILTER_USER, host_function2json_cb, wb);

    buffer_json_object_close(wb);

    nrpc_registry_dispatcher_release(registry);
    nrpc_registry_release(registry_item);
}

// ----------------------------------------------------------------------------
// the dictionary exporter

struct nrpc_host_to_dict_ctx {
    DICTIONARY *dst;
    void *value;
    size_t value_size;
    const char **help;
    const char **tags;
    HTTP_ACCESS *access;
    int *priority;
    uint32_t *version;
};

static void host_function_to_dict_cb(const struct nrpc_method_view *v, void *data) {
    struct nrpc_host_to_dict_ctx *ctx = data;

    // OWNED copies: the destination dictionary's callbacks free them (the
    // conflict callback for losers, the delete callback for entries)
    if(ctx->help)
        *ctx->help = strdupz(v->help);

    if(ctx->tags)
        *ctx->tags = strdupz(v->tags);

    if(ctx->access)
        *ctx->access = v->access;

    if(ctx->priority)
        *ctx->priority = v->priority;

    if(ctx->version)
        *ctx->version = v->version;

    size_t function_name_len = nrpc_strlen_bounded(v->name, PLUGINSD_LINE_MAX);
    if(unlikely(function_name_len > SIZE_MAX - UINT64_MAX_LENGTH - sizeof(NRPC_VERSION_SEPARATOR)))
        fatal("NRPC: method key is too large.");

    size_t key_size = UINT64_MAX_LENGTH + sizeof(NRPC_VERSION_SEPARATOR) + function_name_len;
    CLEAN_CHAR_P *key = mallocz(key_size);
    snprintfz(key, key_size, "%"PRIu32 NRPC_VERSION_SEPARATOR "%s",
              v->version, v->name);

    dictionary_set(ctx->dst, key, ctx->value, ctx->value_size);
}

void nrpc_catalog_host_to_dict(NRPC_OWNER owner, DICTIONARY *dst, void *value, size_t value_size,
                            const char **help, const char **tags, HTTP_ACCESS *access, int *priority, uint32_t *version) {
    if(!dst) return;

    const DICTIONARY_ITEM *registry_item;
    struct nrpc_registry *registry = nrpc_registry_acquire(owner, &registry_item);
    if(!registry) return;

    // the operation gate - taken BEFORE the dictionary_entries() pre-check,
    // which already dereferences the inner dictionary
    if(!nrpc_registry_dispatcher_acquire(registry)) {
        nrpc_registry_release(registry_item);
        return;
    }

    if(!dictionary_entries(registry->dict)) {
        nrpc_registry_dispatcher_release(registry);
        nrpc_registry_release(registry_item);
        return;
    }

    struct nrpc_host_to_dict_ctx ctx = {
        .dst = dst,
        .value = value,
        .value_size = value_size,
        .help = help,
        .tags = tags,
        .access = access,
        .priority = priority,
        .version = version,
    };

    nrpc_catalog_registry_foreach(registry, NRPC_CATALOG_FILTER_USER, host_function_to_dict_cb, &ctx);

    nrpc_registry_dispatcher_release(registry);
    nrpc_registry_release(registry_item);
}

// ----------------------------------------------------------------------------
// the ACLK node-instance manifest

static void manifest_entry_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value,
                                     void *data __maybe_unused) {
    struct nrpc_manifest_entry *e = value;
    freez((void *)e->help);
    freez((void *)e->tags);
}

static void manifest_entry_cb(const struct nrpc_method_view *v, void *data) {
    DICTIONARY *dst = data;

    struct nrpc_manifest_entry e = {
        .help = strdupz(v->help),
        .tags = strdupz(v->tags),
        .access = v->access,
        .priority = v->priority,
        .version = v->version,
    };

    dictionary_set(dst, v->name, &e, sizeof(e));
}

DICTIONARY *nrpc_catalog_manifest_dict(NRPC_OWNER owner) {
    DICTIONARY *dst = dictionary_create(DICT_OPTION_SINGLE_THREADED);
    dictionary_register_delete_callback(dst, manifest_entry_delete_cb, NULL);

    const DICTIONARY_ITEM *registry_item;
    struct nrpc_registry *registry = nrpc_registry_acquire(owner, &registry_item);
    if(!registry) return dst;

    // the operation gate - taken BEFORE the dictionary_entries() pre-check,
    // which already dereferences the inner dictionary; a destroy in progress
    // answers the empty manifest an absent registry answers
    if(!nrpc_registry_dispatcher_acquire(registry)) {
        nrpc_registry_release(registry_item);
        return dst;
    }

    if(dictionary_entries(registry->dict))
        nrpc_catalog_registry_foreach(registry, NRPC_CATALOG_FILTER_USER, manifest_entry_cb, dst);

    nrpc_registry_dispatcher_release(registry);
    nrpc_registry_release(registry_item);
    return dst;
}

// Folds one more string into a running digest. Seeding each call with the previous digest is what
// keeps the field boundary part of the result: "ab" then "c" does not fold to the same value as "a"
// then "bc", because the first call already digests different input - so no separator byte or length
// prefix is needed (the same 64-bit collision caveat as below applies).
//
// NULL folds like "": claim_id is the only field digested here that can actually be NULL, and the
// proto omits it, which is indistinguishable on the wire from an empty one.
static inline XXH64_hash_t manifest_hash_str(XXH64_hash_t seed, const char *s) {
    if(!s) s = "";
    return XXH3_64bits_withSeed(s, strlen(s), seed);
}

uint64_t nrpc_catalog_manifest_hash(DICTIONARY *dict, const char *node_id, const char *claim_id) {
    // The identity of the node is part of the content: the cloud keys the manifest by node_id,
    // so the same function list under a different node_id is a different manifest and must send.
    XXH64_hash_t ids = manifest_hash_str(0, node_id);
    ids = manifest_hash_str(ids, claim_id);

    // Per-entry digests are XOR-combined, so the result does not depend on dictionary traversal
    // order. Every digest starts from the entry's name, which is a dictionary key and therefore
    // unique, so two entries can only cancel out under XOR by colliding in 64 bits - not by
    // construction. At ~2^-64 that is accepted here: this is change detection, not security.
    //
    // Two fields are digested as stored rather than as transmitted. tags: the proto splits them on
    // whitespace, so re-spacing the same tags looks like a change here. access: the proto maps each
    // bit through http_access_map[], so a bit missing from that map would be transmitted as nothing
    // while still counting here - today its static_asserts make such a bit fail the build, so this
    // is a guard against a future divergence, not a live one. Both err toward one extra publish,
    // never toward a missed one - which is the only safe direction.
    XXH64_hash_t entries = 0;
    if(dict) {
        struct nrpc_manifest_entry *e;
        dfe_start_read(dict, e) {
            // padding-free by construction (three uint32_t), so the bytes hashed are only the
            // values - a member of a different width would need explicit packing. uint32_t is also
            // wide enough for access: the manifest proto writer
            // (generate_update_node_instance_manifest_message) pins
            // HTTP_ACCESS_ALL to contiguous bits from 0, so a value that
            // would truncate here fails that build first.
            struct {
                uint32_t access;
                uint32_t priority;
                uint32_t version;
            } numbers = {
                .access = (uint32_t)e->access,
                // the value the proto transmits: a non-positive priority is published as the
                // default (see generate_update_node_instance_manifest_message())
                .priority = e->priority > 0 ? (uint32_t)e->priority : NRPC_PRIORITY_DEFAULT,
                .version = e->version,
            };

            XXH64_hash_t h = manifest_hash_str(0, e_dfe.name);
            h = manifest_hash_str(h, e->help);
            h = manifest_hash_str(h, e->tags);
            h = XXH3_64bits_withSeed(&numbers, sizeof(numbers), h);
            entries ^= h;
        }
        dfe_done(e);
    }

    return XXH3_64bits_withSeed(&entries, sizeof(entries), ids);
}

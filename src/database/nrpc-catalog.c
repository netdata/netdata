// SPDX-License-Identifier: GPL-3.0-or-later

#include "nrpc-internals.h"
#include "nrpc-catalog.h"

// ----------------------------------------------------------------------------
// the iteration core

// Walks one registry dictionary (the host's, or a chart's view of it) and
// hands the callback a self-contained snapshot of every visited entry.
//
// availability_host may be NULL to skip the host-state check (the historical
// semantics of instances resolved through the contexts index).
//
// The snapshot - including the help/tags byte copies - is taken under the
// entry's LEAF spinlock: an item reference pins the entry memory but NOT the
// swapped contents, and the conflict callback frees displaced STRINGs under
// that same lock, so a copy taken outside it could read freed bytes.
static size_t nrpc_catalog_view_foreach(DICTIONARY *dict, RRDHOST *availability_host,
                                         NRPC_CATALOG_FILTER filter,
                                         nrpc_method_view_cb_t cb, void *data) {
    if(!dict) return 0;

    size_t dyncfg_count = 0;

    struct nrpc_method *t;
    dfe_start_read(dict, t) {
        if(!nrpc_method_is_available(availability_host, t)) continue;

        struct nrpc_method_view v = { .name = t_dfe.name };
        char *help = NULL, *tags = NULL;
        bool visit = false;

        spinlock_lock(&t->leaf_spinlock);

        v.options = t->options;

        switch(filter) {
            case NRPC_CATALOG_FILTER_USER:
                visit = !(v.options & (NRPC_METHOD_FLAG_DYNCFG | NRPC_METHOD_FLAG_RESTRICTED));
                break;

            case NRPC_CATALOG_FILTER_STREAM_CHART:
                visit = !(v.options & NRPC_METHOD_FLAG_DYNCFG);
                break;

            case NRPC_CATALOG_FILTER_STREAM_GLOBAL:
                if(v.options & NRPC_METHOD_FLAG_DYNCFG)
                    dyncfg_count++;
                visit = !(v.options & (NRPC_METHOD_FLAG_LOCAL | NRPC_METHOD_FLAG_DYNCFG));
                break;
        }

        if(visit) {
            v.timeout = t->timeout;
            v.access = t->access;
            v.priority = t->priority;
            v.version = t->version;
            help = strdupz(string2str(t->help));
            tags = strdupz(string2str(t->tags));
        }

        spinlock_unlock(&t->leaf_spinlock);

        if(visit) {
            v.help = help;
            v.tags = tags;
            cb(&v, data);
            freez(help);
            freez(tags);
        }
    }
    dfe_done(t);

    return dyncfg_count;
}

size_t nrpc_catalog_host_foreach(RRDHOST *host, NRPC_CATALOG_FILTER filter, nrpc_method_view_cb_t cb, void *data) {
    if(!host || !host->rpc_registry) return 0;
    return nrpc_catalog_view_foreach(host->rpc_registry->dict, host, filter, cb, data);
}

size_t nrpc_catalog_rrdset_foreach(RRDSET *st, NRPC_CATALOG_FILTER filter, nrpc_method_view_cb_t cb, void *data) {
    if(!st || !st->functions_view) return 0;
    return nrpc_catalog_view_foreach(st->functions_view, st->rrdhost, filter, cb, data);
}

void nrpc_catalog_rrdset_view_destroy(RRDSET *st) {
    dictionary_destroy(st->functions_view);
    st->functions_view = NULL;
}

// ----------------------------------------------------------------------------
// the streaming emitters

static void stream_chart_function_cb(const struct nrpc_method_view *v, void *data) {
    BUFFER *wb = data;

    buffer_sprintf(wb
                   , PLUGINSD_KEYWORD_FUNCTION " \"%s\" %d \"%s\" \"%s\" "HTTP_ACCESS_FORMAT" %d %"PRIu32"\n"
                   , v->name
                   , v->timeout
                   , v->help
                   , v->tags
                   , (HTTP_ACCESS_FORMAT_CAST)v->access
                   , v->priority
                   , v->version
    );
}

void stream_sender_send_rrdset_functions(RRDSET *st, BUFFER *wb) {
    nrpc_catalog_rrdset_foreach(st, NRPC_CATALOG_FILTER_STREAM_CHART, stream_chart_function_cb, wb);
}

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
// queued FUNCTION_DEL lines, then the full FUNCTION re-list - one buffer, one
// commit by the caller. `can_function_del` is the streaming caller's verdict
// (STREAM_CAP_FUNCTION_DEL + metadata readiness) - this renderer deliberately
// knows nothing about streaming; when the verdict is false the snapshot is
// DISCARDED, matching the old silent drop and preventing unbounded growth on
// parents without FUNCDEL support.
void stream_sender_send_host_functions(RRDHOST *host, BUFFER *wb, bool dyncfg, bool can_function_del) {
    // Ordering contract with nrpc_method_unregister(): clear the flag FIRST, then
    // snapshot-and-clear the pending set under its lock. The deleter inserts
    // into the set BEFORE setting the flag, so a del landing after our
    // snapshot re-sets the flag with its entry already queued - the next poll
    // drains it; nothing is ever lost. NOTE: the two streaming callers run on
    // DIFFERENT threads (the flag poll on collection/receiver threads, the
    // reconnect push on the sender thread) and can race each other too - the
    // flag/spinlock protocol keeps the QUEUE lossless, but the callers must
    // additionally hold the sender's global_functions_spinlock across
    // {render + commit}, or a stale rendered buffer could commit its
    // FUNCTION_DEL lines after a fresh re-list (see the lock's comment in
    // stream-sender-internals.h).
    rrdhost_flag_clear(host, RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED);

    // after the flag clear, so a NULL registry (archived host racing the sender's
    // flag poll) behaves exactly like the old NULL-tolerant dictionary traversal:
    // flag cleared, nothing emitted
    if(!host->rpc_registry)
        return;

    struct nrpc_registry *registry = host->rpc_registry;
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

    size_t configs = nrpc_catalog_host_foreach(host, NRPC_CATALOG_FILTER_STREAM_GLOBAL,
                                                stream_global_function_cb, wb);

    if(dyncfg && configs)
        dyncfg_add_streaming(wb);
}

// ----------------------------------------------------------------------------
// the JSON renderers

static void chart_function2json_cb(const struct nrpc_method_view *v, void *data) {
    BUFFER *wb = data;

    buffer_json_member_add_object(wb, v->name);
    {
        buffer_json_member_add_string_or_empty(wb, "help", v->help);
        buffer_json_member_add_int64(wb, "timeout", (int64_t)v->timeout);
        buffer_json_member_add_uint64(wb, "version", (uint64_t)v->version);

        char options[65];
        snprintfz(
            options, 64
            , "%s%s"
            , (v->options & NRPC_METHOD_FLAG_LOCAL) ? "LOCAL " : ""
            , (v->options & NRPC_METHOD_FLAG_GLOBAL) ? "GLOBAL" : ""
        );

        buffer_json_member_add_string_or_empty(wb, "options", options);
        buffer_json_member_add_string_or_empty(wb, "tags", v->tags);
        http_access2buffer_json_array(wb, "access", v->access);
        buffer_json_member_add_uint64(wb, "priority", v->priority);
    }
    buffer_json_object_close(wb);
}

void nrpc_catalog_chart2json(RRDSET *st, BUFFER *wb) {
    nrpc_catalog_rrdset_foreach(st, NRPC_CATALOG_FILTER_USER, chart_function2json_cb, wb);
}

static void host_function2json_cb(const struct nrpc_method_view *v, void *data) {
    BUFFER *wb = data;

    buffer_json_member_add_object(wb, v->name);
    {
        buffer_json_member_add_string(wb, "help", v->help);
        buffer_json_member_add_int64(wb, "timeout", v->timeout);
        buffer_json_member_add_uint64(wb, "version", (uint64_t)v->version);
        buffer_json_member_add_array(wb, "options");
        {
            if (v->options & NRPC_METHOD_FLAG_GLOBAL)
                buffer_json_add_array_item_string(wb, "GLOBAL");
            if (v->options & NRPC_METHOD_FLAG_LOCAL)
                buffer_json_add_array_item_string(wb, "LOCAL");
        }
        buffer_json_array_close(wb);
        buffer_json_member_add_string(wb, "tags", v->tags);
        http_access2buffer_json_array(wb, "access", v->access);
        buffer_json_member_add_uint64(wb, "priority", v->priority);
    }
    buffer_json_object_close(wb);
}

void nrpc_catalog_host2json(RRDHOST *host, BUFFER *wb) {
    if(!host || !host->rpc_registry) return;

    buffer_json_member_add_object(wb, "functions");

    nrpc_catalog_host_foreach(host, NRPC_CATALOG_FILTER_USER, host_function2json_cb, wb);

    buffer_json_object_close(wb);
}

// ----------------------------------------------------------------------------
// the dictionary exporters

struct nrpc_chart_to_dict_ctx {
    DICTIONARY *dst;
    void *value;
    size_t value_size;
};

static void chart_function_to_dict_cb(const struct nrpc_method_view *v, void *data) {
    struct nrpc_chart_to_dict_ctx *ctx = data;
    dictionary_set(ctx->dst, v->name, ctx->value, ctx->value_size);
}

void nrpc_catalog_chart_to_dict(RRDSET *st, DICTIONARY *dst, void *value, size_t value_size) {
    if(!st || !st->functions_view || !dst) return;

    struct nrpc_chart_to_dict_ctx ctx = { .dst = dst, .value = value, .value_size = value_size };

    // host==NULL availability semantics preserved: instances resolved through
    // the contexts index skip the host-state check
    nrpc_catalog_view_foreach(st->functions_view, NULL, NRPC_CATALOG_FILTER_USER,
                               chart_function_to_dict_cb, &ctx);
}

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

void nrpc_catalog_host_to_dict(RRDHOST *host, DICTIONARY *dst, void *value, size_t value_size,
                            const char **help, const char **tags, HTTP_ACCESS *access, int *priority, uint32_t *version) {
    if(!host || !host->rpc_registry || !dictionary_entries(host->rpc_registry->dict) || !dst) return;

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

    nrpc_catalog_host_foreach(host, NRPC_CATALOG_FILTER_USER, host_function_to_dict_cb, &ctx);
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

DICTIONARY *nrpc_catalog_manifest_dict(RRDHOST *host) {
    DICTIONARY *dst = dictionary_create(DICT_OPTION_SINGLE_THREADED);
    dictionary_register_delete_callback(dst, manifest_entry_delete_cb, NULL);

    if(!host || !host->rpc_registry || !dictionary_entries(host->rpc_registry->dict)) return dst;

    nrpc_catalog_host_foreach(host, NRPC_CATALOG_FILTER_USER, manifest_entry_cb, dst);

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
            // wide enough for access: node_manifest.cc pins HTTP_ACCESS_ALL to contiguous bits from
            // 0, so a value that would truncate here fails that build first.
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

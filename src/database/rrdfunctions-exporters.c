// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdfunctions-internals.h"
#include "rrdfunctions-exporters.h"

void stream_sender_send_rrdset_functions(RRDSET *st, BUFFER *wb) {
    if(!st->functions_view)
        return;

    struct rrd_host_function *t;
    dfe_start_read(st->functions_view, t) {
        if(!rrd_function_is_available(t, st->rrdhost)) continue;
        if(t->options & RRD_FUNCTION_DYNCFG) continue;

        buffer_sprintf(wb
                       , PLUGINSD_KEYWORD_FUNCTION " \"%s\" %d \"%s\" \"%s\" "HTTP_ACCESS_FORMAT" %d %"PRIu32"\n"
                       , t_dfe.name
                       , t->timeout
                       , string2str(t->help)
                       , string2str(t->tags)
                       , (HTTP_ACCESS_FORMAT_CAST)t->access
                       , t->priority
                       , t->version
        );
    }
    dfe_done(t);
}

void stream_sender_send_global_rrdhost_functions(RRDHOST *host, BUFFER *wb, bool dyncfg) {
    rrdhost_flag_clear(host, RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED);

    size_t configs = 0;

    struct rrd_host_function *tmp;
    dfe_start_read(host->functions, tmp) {
        if(!rrd_function_is_available(tmp, host)) continue;
        if(tmp->options & RRD_FUNCTION_LOCAL) continue;
        if(tmp->options & RRD_FUNCTION_DYNCFG) {
            // we should not send dyncfg to this parent
            configs++;
            continue;
        }

        buffer_sprintf(wb
                       , PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\" \"%s\" "HTTP_ACCESS_FORMAT" %d %"PRIu32"\n"
                       , tmp_dfe.name
                       , tmp->timeout
                       , string2str(tmp->help)
                       , string2str(tmp->tags)
                       , (HTTP_ACCESS_FORMAT_CAST)tmp->access
                       , tmp->priority
                       , tmp->version
        );
    }
    dfe_done(tmp);

    if(dyncfg && configs)
        dyncfg_add_streaming(wb);
}

static void functions2json(DICTIONARY *functions, BUFFER *wb, RRDHOST *host) {
    struct rrd_host_function *t;
    dfe_start_read(functions, t) {
        if(!rrd_function_is_available(t, host)) continue;
        if(t->options & (RRD_FUNCTION_DYNCFG| RRD_FUNCTION_RESTRICTED)) continue;

        buffer_json_member_add_object(wb, t_dfe.name);
        {
            buffer_json_member_add_string_or_empty(wb, "help", string2str(t->help));
            buffer_json_member_add_int64(wb, "timeout", (int64_t) t->timeout);
            buffer_json_member_add_uint64(wb, "version", (uint64_t) t->version);

            char options[65];
            snprintfz(
                options, 64
                , "%s%s"
                , (t->options & RRD_FUNCTION_LOCAL) ? "LOCAL " : ""
                , (t->options & RRD_FUNCTION_GLOBAL) ? "GLOBAL" : ""
            );

            buffer_json_member_add_string_or_empty(wb, "options", options);
            buffer_json_member_add_string_or_empty(wb, "tags", string2str(t->tags));
            http_access2buffer_json_array(wb, "access", t->access);
            buffer_json_member_add_uint64(wb, "priority", t->priority);
        }
        buffer_json_object_close(wb);
    }
    dfe_done(t);
}

void chart_functions2json(RRDSET *st, BUFFER *wb) {
    if(!st || !st->functions_view) return;

    functions2json(st->functions_view, wb, st->rrdhost);
}

void host_functions2json(RRDHOST *host, BUFFER *wb) {
    if(!host || !host->functions) return;

    buffer_json_member_add_object(wb, "functions");

    struct rrd_host_function *t;
    dfe_start_read(host->functions, t) {
        if(!rrd_function_is_available(t, host)) continue;
        if(t->options & (RRD_FUNCTION_DYNCFG| RRD_FUNCTION_RESTRICTED)) continue;

        buffer_json_member_add_object(wb, t_dfe.name);
        {
            buffer_json_member_add_string(wb, "help", string2str(t->help));
            buffer_json_member_add_int64(wb, "timeout", t->timeout);
            buffer_json_member_add_uint64(wb, "version", (uint64_t) t->version);
            buffer_json_member_add_array(wb, "options");
            {
                if (t->options & RRD_FUNCTION_GLOBAL)
                    buffer_json_add_array_item_string(wb, "GLOBAL");
                if (t->options & RRD_FUNCTION_LOCAL)
                    buffer_json_add_array_item_string(wb, "LOCAL");
            }
            buffer_json_array_close(wb);
            buffer_json_member_add_string(wb, "tags", string2str(t->tags));
            http_access2buffer_json_array(wb, "access", t->access);
            buffer_json_member_add_uint64(wb, "priority", t->priority);
        }
        buffer_json_object_close(wb);
    }
    dfe_done(t);

    buffer_json_object_close(wb);
}

void chart_functions_to_dict(DICTIONARY *rrdset_functions_view, DICTIONARY *dst, void *value, size_t value_size) {
    if(!rrdset_functions_view || !dst) return;

    struct rrd_host_function *t;
    dfe_start_read(rrdset_functions_view, t) {
        if(!rrd_function_is_available(t, NULL)) continue;
        if(t->options & (RRD_FUNCTION_DYNCFG| RRD_FUNCTION_RESTRICTED)) continue;

        dictionary_set(dst, t_dfe.name, value, value_size);
    }
    dfe_done(t);
}

void host_functions_to_dict(RRDHOST *host, DICTIONARY *dst, void *value, size_t value_size,
                            STRING **help, STRING **tags, HTTP_ACCESS *access, int *priority, uint32_t *version) {
    if(!host || !host->functions || !dictionary_entries(host->functions) || !dst) return;

    struct rrd_host_function *t;
    dfe_start_read(host->functions, t) {
        if(!rrd_function_is_available(t, host)) continue;
        if(t->options & (RRD_FUNCTION_DYNCFG| RRD_FUNCTION_RESTRICTED)) continue;

        if(help)
            *help = t->help;

        if(tags)
            *tags = t->tags;

        if(access)
            *access = t->access;

        if(priority)
            *priority = t->priority;

        if(version)
            *version = t->version;

        size_t function_name_len = rrd_functions_strlen_bounded(t_dfe.name, PLUGINSD_LINE_MAX);
        if(unlikely(function_name_len > SIZE_MAX - UINT64_MAX_LENGTH - sizeof(RRDFUNCTIONS_VERSION_SEPARATOR)))
            fatal("RRDFUNCTIONS: function key is too large.");

        size_t key_size = UINT64_MAX_LENGTH + sizeof(RRDFUNCTIONS_VERSION_SEPARATOR) + function_name_len;
        CLEAN_CHAR_P *key = mallocz(key_size);
        snprintfz(key, key_size, "%"PRIu32 RRDFUNCTIONS_VERSION_SEPARATOR "%s",
                  t->version, t_dfe.name);

        dictionary_set(dst, key, value, value_size);
    }
    dfe_done(t);
}

static void manifest_entry_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value,
                                     void *data __maybe_unused) {
    struct rrd_function_manifest_entry *e = value;
    freez((void *)e->help);
    freez((void *)e->tags);
}

DICTIONARY *host_functions_to_manifest_dict(RRDHOST *host) {
    DICTIONARY *dst = dictionary_create(DICT_OPTION_SINGLE_THREADED);
    dictionary_register_delete_callback(dst, manifest_entry_delete_cb, NULL);

    if(!host || !host->functions || !dictionary_entries(host->functions)) return dst;

    struct rrd_host_function *t;
    dfe_start_read(host->functions, t) {
        if(!rrd_function_is_available(t, host)) continue;
        if(t->options & (RRD_FUNCTION_DYNCFG | RRD_FUNCTION_RESTRICTED)) continue;

        // Copy the bytes rather than taking a STRING reference. The conflict callback swaps
        // help/tags and frees the displaced STRING under the dictionary INDEX lock, while this
        // traversal only holds the ITEMS lock - so string_dup() here would be a refcount
        // read-modify-write on a STRING that can already be freed, which also fatal()s on a
        // deleted string. Every sibling exporter reads these the same way.
        struct rrd_function_manifest_entry e = {
            .help = strdupz(string2str(t->help)),
            .tags = strdupz(string2str(t->tags)),
            .access = t->access,
            .priority = t->priority,
            .version = t->version,
        };

        dictionary_set(dst, t_dfe.name, &e, sizeof(e));
    }
    dfe_done(t);

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

uint64_t manifest_dict_hash(DICTIONARY *dict, const char *node_id, const char *claim_id) {
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
        struct rrd_function_manifest_entry *e;
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
                .priority = e->priority > 0 ? (uint32_t)e->priority : RRDFUNCTIONS_PRIORITY_DEFAULT,
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

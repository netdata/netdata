// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd.h"
#include "nrpc-internals.h"

#define MAX_FUNCTION_LENGTH (PLUGINSD_LINE_MAX - 512) // we need some space for the rest of the line

// ----------------------------------------------------------------------------

// each host owns a function registry (RRD_FUNCTIONS) holding its function
// definitions; an RRDSET gets a view into it on demand (only when a function
// is added to that RRDSET)

// ----------------------------------------------------------------------------

static void rrd_functions_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *func, void *data) {
    struct rrd_functions *functions = data;
    RRDHOST *host = functions->host;
    struct rrd_host_function *rdcf = func;

    nrpc_serving_started();
    rdcf->serving = nrpc_serving_current_thread_acquire();
    rdcf->rrdhost_state_id = object_state_id(&host->state_id);
    rdcf->unregistered = false;

    spinlock_init(&rdcf->leaf_spinlock);

    // the entry stores the transport (execute_cb_data) for transport-bearing
    // sources - take the entry ref this storage owns; released by
    // rrd_functions_cleanup() under the stored tag
    if(rrd_function_source_has_transport(rdcf->source) && rdcf->execute_cb_data)
        nrpc_transport_entry_acquire(rdcf->execute_cb_data);

    if(!rdcf->priority)
        rdcf->priority = RRDFUNCTIONS_PRIORITY_DEFAULT;

//    internal_error(true, "FUNCTIONS: adding function '%s' on host '%s', collection tid %d, %s",
//                   dictionary_acquired_item_name(item), rrdhost_hostname(host),
//                   rdcf->serving->tid, rdcf->serving->running ? "running" : "NOT running");
}

static void rrd_functions_cleanup(struct rrd_host_function *rdcf) {
    nrpc_serving_release(rdcf->serving);

    // release the transport ref this (tag, data) pair owns - keyed on the tag
    // the struct actually HOLDS (the conflict callback swaps them as one pair,
    // so the displaced pair always lands here together). NULL-safe; INTERNAL
    // data is caller-owned and never released.
    if(rrd_function_source_has_transport(rdcf->source))
        nrpc_transport_entry_release(rdcf->execute_cb_data);

    string_freez(rdcf->help);
    string_freez(rdcf->tags);
}

static void rrd_functions_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *func,
                                          void *data __maybe_unused) {
    struct rrd_host_function *rdcf = func;
    rrd_functions_cleanup(rdcf);
}

static bool rrd_functions_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *func,
                                            void *new_func, void *data) {
    struct rrd_functions *functions = data;
    RRDHOST *host = functions->host; (void)host;
    struct rrd_host_function *rdcf = func;
    struct rrd_host_function *new_rdcf = new_func;

    nrpc_serving_started();

    bool changed = false;

    if(__atomic_load_n(&rdcf->unregistered, __ATOMIC_ACQUIRE)) {
        __atomic_store_n(&rdcf->unregistered, false, __ATOMIC_RELEASE);
        changed = true;
    }

    if(rdcf->serving != nrpc_thread_serving) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed collector from %d to %d",
               dictionary_acquired_item_name(item), rrdhost_hostname(host),
               nrpc_serving_tid(rdcf->serving), nrpc_serving_tid(nrpc_thread_serving));

        new_rdcf->serving = rdcf->serving;
        rdcf->serving = nrpc_serving_current_thread_acquire();
        changed = true;
    }

    if(rdcf->rrdhost_state_id != object_state_id(&host->state_id)) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed state id from %u to %u",
               dictionary_acquired_item_name(item), rrdhost_hostname(host),
               rdcf->rrdhost_state_id,
               object_state_id(&host->state_id));

        rdcf->rrdhost_state_id = object_state_id(&host->state_id);
        changed = true;
    }

    // Everything from here to the end swaps fields that concurrent readers
    // capture or byte-copy under the entry's leaf spinlock (an item reference
    // pins the entry memory but NOT the swapped contents - a displaced STRING
    // or transport can be freed the moment it leaves the entry). Take the leaf
    // lock around the swaps AND the displaced releases/frees below, so a
    // reader either sees the old consistent contents or the new ones.
    spinlock_lock(&rdcf->leaf_spinlock);

    if(rdcf->execute_cb != new_rdcf->execute_cb) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed execute callback",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        SWAP(rdcf->execute_cb, new_rdcf->execute_cb);
        changed = true;
    }

    if(rdcf->help != new_rdcf->help) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed help text",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        SWAP(rdcf->help, new_rdcf->help);
        changed = true;
    }

    if(rdcf->tags != new_rdcf->tags) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed tags",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        SWAP(rdcf->tags, new_rdcf->tags);
        changed = true;
    }

    if(rdcf->options != new_rdcf->options) {
        // options are derived from the name and the tags (get_function_options());
        // keeping them in sync with the swapped tags is what makes a re-registration
        // with the "hidden" tag actually restrict the function
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed options",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        SWAP(rdcf->options, new_rdcf->options);
        changed = true;
    }

    if(rdcf->timeout != new_rdcf->timeout) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed timeout (from %d to %d)",
               dictionary_acquired_item_name(item), rrdhost_hostname(host),
               rdcf->timeout, new_rdcf->timeout);

        SWAP(rdcf->timeout, new_rdcf->timeout);
        changed = true;
    }

    if(rdcf->version != new_rdcf->version) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed version (from %"PRIu32", to %"PRIu32")",
               dictionary_acquired_item_name(item), rrdhost_hostname(host),
               rdcf->version, new_rdcf->version);

        SWAP(rdcf->version, new_rdcf->version);
        changed = true;
    }

    if(rdcf->priority != new_rdcf->priority) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed priority",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        SWAP(rdcf->priority, new_rdcf->priority);
        changed = true;
    }

    if(rdcf->access != new_rdcf->access) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed access level",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        SWAP(rdcf->access, new_rdcf->access);
        changed = true;
    }

    if(rdcf->sync != new_rdcf->sync) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed sync/async mode",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        SWAP(rdcf->sync, new_rdcf->sync);
        changed = true;
    }

    // (source, execute_cb_data) swap as ONE pair inside one conditional - never
    // as two independently-conditional swaps - so a later cleanup/release always
    // keys on the ownership tag matching the data it actually holds.
    //
    // Transport accounting (the collector pattern in this same callback, see
    // new_rdcf->serving above): when a DIFFERENT pair is installed, acquire
    // an entry ref iff the INSTALLED tag is transport-bearing; the displaced
    // pair lands in new_rdcf for rrd_functions_cleanup(new_rdcf) below to
    // release under the DISPLACED tag. When NO swap occurs (equal pair -
    // routine: a child re-sends its whole function list on every flag set and
    // reconnect, and the dictionary fires this callback even for identical
    // values), NEUTRALIZE new_rdcf (data NULL, non-transport tag) so the
    // unconditional cleanup release is a no-op. Invariant: an equal-pair
    // conflict nets ZERO refs - by neutralization, not by skipping the
    // release.
    if(rdcf->execute_cb_data != new_rdcf->execute_cb_data || rdcf->source != new_rdcf->source) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed execute callback data or registration source",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        if(rrd_function_source_has_transport(new_rdcf->source) && new_rdcf->execute_cb_data)
            nrpc_transport_entry_acquire(new_rdcf->execute_cb_data);

        SWAP(rdcf->execute_cb_data, new_rdcf->execute_cb_data);
        SWAP(rdcf->source, new_rdcf->source);
        changed = true;
    }
    else {
        new_rdcf->execute_cb_data = NULL;
        new_rdcf->source = RRD_FUNCTION_REG_SOURCE_INTERNAL;
    }

//    internal_error(true, "FUNCTIONS: adding function '%s' on host '%s', collection tid %d, %s",
//                   dictionary_acquired_item_name(item), rrdhost_hostname(host),
//                   rdcf->serving->tid, rdcf->serving->running ? "running" : "NOT running");

    // under the leaf lock: frees the DISPLACED strings/transport ref, which a
    // concurrent leaf-locked reader may otherwise still be copying
    rrd_functions_cleanup(new_rdcf);

    spinlock_unlock(&rdcf->leaf_spinlock);

    return changed;
}

void rrd_functions_host_init(RRDHOST *host) {
    if(host->functions) return;

    struct rrd_functions *functions = callocz(1, sizeof(struct rrd_functions));
    functions->host = host;
    functions->dict = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                 &dictionary_stats_category_functions, sizeof(struct rrd_host_function));

    dictionary_register_insert_callback(functions->dict, rrd_functions_insert_callback, functions);
    dictionary_register_delete_callback(functions->dict, rrd_functions_delete_callback, functions);
    dictionary_register_conflict_callback(functions->dict, rrd_functions_conflict_callback, functions);

    // eagerly created: deleters include non-stream threads (e.g. dyncfg), so
    // the set must exist for the whole registry lifetime, not on first use
    spinlock_init(&functions->pending_dels.spinlock);
    functions->pending_dels.dict = dictionary_create(DICT_OPTION_SINGLE_THREADED); // guarded by pending_dels.spinlock

    host->functions = functions;
}

void rrd_functions_host_destroy(RRDHOST *host) {
    if(!host->functions) return;

    dictionary_destroy(host->functions->pending_dels.dict);
    dictionary_destroy(host->functions->dict);
    freez(host->functions);
    host->functions = NULL;
}

void rrd_function_acquired_release(RRDHOST *host, RRD_FUNCTION_ACQUIRED *rfa) {
    if(!rfa) return;
    dictionary_acquired_item_release(host->functions->dict, (const DICTIONARY_ITEM *)rfa);
}

// Capture-at-find: snapshot the execute pair under the entry's leaf spinlock,
// entry-pinning the transport for transport-bearing sources. The caller holds
// the acquired item (pinning the entry memory); the leaf lock closes the
// window where a concurrent re-registration swaps the pair and frees the
// displaced transport between our read and our pin. The pin attaches ONLY to
// the entry the find returned - never to prefix-retry intermediates, which
// are released inside the find loop before this can run.
void rrd_function_acquired_capture(RRD_FUNCTION_ACQUIRED *rfa, struct rrd_function_capture *out) {
    struct rrd_host_function *rdcf = rrd_function_acquired_value(rfa);

    spinlock_lock(&rdcf->leaf_spinlock);
    out->execute_cb = rdcf->execute_cb;
    out->execute_cb_data = rdcf->execute_cb_data;
    out->transport_pin = (rrd_function_source_has_transport(rdcf->source) && rdcf->execute_cb_data)
                             ? nrpc_transport_entry_acquire(rdcf->execute_cb_data)
                             : NULL;
    spinlock_unlock(&rdcf->leaf_spinlock);
}

// ----------------------------------------------------------------------------

static inline bool is_function_restricted(const char *name, const char *tags) {
    return (name && name[0] == '_' && name[1] == '_') || (tags && strstr(tags, RRDFUNCTIONS_TAG_HIDDEN) != NULL);
}

// Reports whether a function name is a reserved dynamic-configuration name -
// the `config` catch-all or any per-config `config <id>` function. Exposed so
// the pluginsd FUNCTION handler can refuse external registrations that would
// otherwise overwrite (swap) the built-in dyncfg execute callback.
bool rrd_function_name_is_dyncfg(const char *name) {
    if(!name || !*name)
        return false;

    if(strncmp(name, PLUGINSD_FUNCTION_CONFIG, sizeof(PLUGINSD_FUNCTION_CONFIG) - 1) != 0)
        return false;

    char c = name[sizeof(PLUGINSD_FUNCTION_CONFIG) - 1];
    if(c == 0 || isspace((uint8_t)c))
        return true;

    return false;
}

static inline RRD_FUNCTION_OPTIONS get_function_options(RRDSET *st, const char *name, const char *tags) {
    if(rrd_function_name_is_dyncfg(name))
        return RRD_FUNCTION_DYNCFG;

    RRD_FUNCTION_OPTIONS options = st ? RRD_FUNCTION_LOCAL : RRD_FUNCTION_GLOBAL;

    return options | (is_function_restricted(name, tags) ? RRD_FUNCTION_RESTRICTED : 0);
}

void rrd_function_add(RRDHOST *host, RRDSET *st, const char *name, int timeout, int priority, uint32_t version,
                      const char *help, const char *tags,
                      HTTP_ACCESS access, bool sync, RRD_FUNCTION_REG_SOURCE source,
                      rrd_function_execute_cb_t execute_cb, void *execute_cb_data) {

    // RRDSET *st may be NULL in this function
    // to create a GLOBAL function

    if(!tags || !*tags)
        tags = "top";

    size_t key_size = rrd_functions_strlen_bounded(name, PLUGINSD_LINE_MAX) + 1;
    CLEAN_CHAR_P *key = mallocz(key_size);
    rrd_functions_sanitize(key, name, key_size);

    // text_sanitize() wipes a result that is entirely underscores, so a name like "__" sanitizes
    // to "". Such a name is unusable (it could never be looked up) and it would also defeat the
    // "__" prefix check in is_function_restricted() below, so refuse it outright.
    if(unlikely(!*key)) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "FUNCTIONS: refusing to register function '%s' on host '%s': the name sanitizes to an empty string",
               name, rrdhost_hostname(host));
        return;
    }

    // Reserved dynamic-configuration function names ("config", "config <id>") are
    // owned exclusively by the dyncfg subsystem. A local plugin (COLLECTOR)
    // registering one would collide in the registry and make
    // rrd_functions_conflict_callback() swap the built-in dyncfg execute callback
    // out, hijacking the config tree. Local plugins do dyncfg via the DYNCFG
    // protocol (not FUNCTION), so no legitimate COLLECTOR registration uses these
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
    if(source == RRD_FUNCTION_REG_SOURCE_COLLECTOR && rrd_function_name_is_dyncfg(key)) {
        // Log the sanitized name (what we actually classify on), not the raw name:
        // a raw name with leading whitespace/control chars or embedded newlines
        // would make this log line misleading or malformed.
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "FUNCTIONS: 'host:%s' attempted to register reserved dynamic-configuration function '%s' from a collector. Ignoring it.",
               rrdhost_hostname(host), key);
        return;
    }

    if(st && !st->functions_view)
        st->functions_view = dictionary_create_view(host->functions->dict);

    // Classify from the sanitized key, not the raw name: the key is what the dictionary is
    // indexed by, so classifying the raw name lets " config" land on the "config" key without
    // the DYNCFG option, which would hand a dyncfg-reserved entry to a regular function.
    // Captured before the insert because the conflict callback swaps options with the
    // previous value, leaving tmp.options holding the old one.
    RRD_FUNCTION_OPTIONS options = get_function_options(st, key, tags);

    struct rrd_host_function tmp = {
        .serving = NULL,
        .sync = sync,
        .timeout = timeout,
        .version = version,
        .priority = priority,
        .options = options,
        .access = access,
        .source = source,
        .execute_cb = execute_cb,
        .execute_cb_data = execute_cb_data,
        .help = string_strdupz(help),
        .tags = string_strdupz(tags),
    };
    const DICTIONARY_ITEM *item = dictionary_set_and_acquire_item(host->functions->dict, key, &tmp, sizeof(tmp));
    if(unlikely(!item)) {
        // dictionary destroyed (shutdown in progress) - neither the insert nor
        // the conflict callback ran, so tmp's strings are still ours and no
        // transport ref was taken; unwind cleanly
        string_freez(tmp.help);
        string_freez(tmp.tags);
        return;
    }

    if(st)
        dictionary_view_set(st->functions_view, key, item);
    else {
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
    }

    dictionary_acquired_item_release(host->functions->dict, item);

    // Refresh the cloud function manifest. Only DYNCFG is excluded: it is derived from the
    // key, so such an entry can never enter or leave the manifest. RESTRICTED comes from the
    // tags and can be added by a re-registration, which drops the function OUT of the
    // manifest - that transition has to be reported too, so it must not be filtered here.
    // Placed after the insert so the arm always follows the change it reports.
    if(!(options & RRD_FUNCTION_DYNCFG))
        aclk_arm_node_manifest(host);
}

bool rrd_function_del(RRDHOST *host, RRDSET *st, const char *name, RRD_FUNCTION_REG_SOURCE source) {
    if(unlikely(!name || !*name))
        return false;

    size_t key_size = rrd_functions_strlen_bounded(name, PLUGINSD_LINE_MAX) + 1;
    CLEAN_CHAR_P *key = mallocz(key_size);
    rrd_functions_sanitize(key, name, key_size);

    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(host->functions->dict, key);
    if(!item)
        return false;

    struct rrd_host_function *rdcf = dictionary_acquired_item_value(item);

    if(source == RRD_FUNCTION_REG_SOURCE_COLLECTOR) {
        if(!nrpc_thread_serving || rdcf->serving != nrpc_thread_serving) {
            nd_log(NDLS_DAEMON, NDLP_WARNING,
                   "FUNCTIONS: refusing to unregister function '%s' - "
                   "collector mismatch (registered by %s, unregister requested by %s)",
                   name,
                   rdcf->serving ? "another collector" : "unknown",
                   nrpc_thread_serving ? "current collector" : "non-collector thread");
            dictionary_acquired_item_release(host->functions->dict, item);
            return false;
        }
    }

    // dyncfg config functions are intentionally never streamed to parents
    // (stream_sender_send_global_rrdhost_functions skips RRD_FUNCTION_DYNCFG),
    // so their removals must not be streamed either. Capture this before releasing.
    bool is_dyncfg = (rdcf->options & RRD_FUNCTION_DYNCFG);

    // whether this function is part of the cloud manifest; captured here because
    // rdcf is released (and may be freed) before we can queue the manifest refresh
    bool in_manifest = !(rdcf->options & (RRD_FUNCTION_DYNCFG | RRD_FUNCTION_RESTRICTED));

    // The FUNCTION_DEL protocol command (from external plugins or streaming
    // children) must never remove dyncfg config functions; those are owned and
    // removed exclusively by the dyncfg subsystem (INTERNAL deletes).
    if(source != RRD_FUNCTION_REG_SOURCE_INTERNAL && is_dyncfg) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "FUNCTIONS: refusing to unregister dyncfg function '%s' via FUNCTION_DEL", name);
        dictionary_acquired_item_release(host->functions->dict, item);
        return false;
    }

    // Mark the function unregistered before removing it from the dictionary.
    // The flag makes the function unavailable to any thread that already holds
    // an acquired reference during the delete window (it cannot be freed until
    // that reference is released), and lets rrd_functions_find_by_name() report
    // a specific "unregistered by the plugin" error in that window.
    __atomic_store_n(&rdcf->unregistered, true, __ATOMIC_RELEASE);

    if(st && st->functions_view)
        dictionary_del(st->functions_view, key);

    // Delete from the index while still holding our acquired reference, then
    // release: this unlinks the item before any concurrent re-registration could
    // resurrect it through the conflict callback (a re-add now allocates a fresh
    // item). The value is freed once the last reference - this one plus any
    // in-flight execution - is released.
    dictionary_del(host->functions->dict, key);
    dictionary_acquired_item_release(host->functions->dict, item);

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
    if(!st && !is_dyncfg && rrdhost_has_stream_sender_enabled(host)) {
        spinlock_lock(&host->functions->pending_dels.spinlock);
        dictionary_set(host->functions->pending_dels.dict, key, NULL, 0);
        spinlock_unlock(&host->functions->pending_dels.spinlock);
    }

    if(!st)
        rrdhost_flag_set(host, RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED);

    dictionary_garbage_collect(host->functions->dict);

    // after the delete, so the arm always follows the change it reports
    if(in_manifest)
        aclk_arm_node_manifest(host);

    return true;
}

bool rrd_function_is_available(struct rrd_host_function *rdcf, RRDHOST *host) {
    if(__atomic_load_n(&rdcf->unregistered, __ATOMIC_ACQUIRE))
        return false;

    if(!nrpc_serving_running(rdcf->serving))
        return false;

    if(host && rdcf->rrdhost_state_id != object_state_id(&host->state_id))
        return false;

    return true;
}

int rrd_functions_find_by_name(RRDHOST *host, BUFFER *wb, const char *name, size_t key_length, RRD_FUNCTION_ACQUIRED **out_acquired) {
    char buffer[MAX_FUNCTION_LENGTH + 1];
    strncpyz(buffer, name, sizeof(buffer) - 1);
    key_length = strnlen(buffer, sizeof(buffer));
    char *s = NULL;

    OBJECT_STATE_ID state_id = object_state_id(&host->state_id);

    bool found = false;
    bool was_unregistered = false;
    const DICTIONARY_ITEM *item = NULL;
    if(host->functions) {
        while (buffer[0]) {
            if((item = dictionary_get_and_acquire_item(host->functions->dict, buffer))) {
                found = true;

                struct rrd_host_function *rdcf = dictionary_acquired_item_value(item);
                if(rrd_function_is_available(rdcf, host)) {
                    break;
                }
                else {

                    nd_log(NDLS_DAEMON, NDLP_DEBUG,
                           "Function '%s' is not available. "
                           "host '%s', collector = { tid: %d, running: %s }, host tid { rcv: %d, snd: %d }, host state { id: %u, expected %u }, hops: %d",
                           name,
                           rrdhost_hostname(host),
                           nrpc_serving_tid(rdcf->serving),
                           nrpc_serving_running(rdcf->serving) ? "yes" : "no",
                           host->stream.rcv.status.tid, host->stream.snd.status.tid,
                           state_id, rdcf->rrdhost_state_id,
                           rrdhost_ingestion_hops(host)
                           );

                    was_unregistered = __atomic_load_n(&rdcf->unregistered, __ATOMIC_ACQUIRE);
                    dictionary_acquired_item_release(host->functions->dict, item);
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
    }

    buffer_flush(wb);

    *out_acquired = (RRD_FUNCTION_ACQUIRED *)item;

    if(!item) {
        if(found) {
            if(was_unregistered)
                return rrd_call_function_error(wb,
                                               "This function has been unregistered by the plugin.",
                                               HTTP_RESP_SERVICE_UNAVAILABLE);
            else
                return rrd_call_function_error(wb,
                                               "The plugin that registered this feature, is not currently running.",
                                               HTTP_RESP_SERVICE_UNAVAILABLE);
        }
        else
            return rrd_call_function_error(wb,
                                           "This feature is not available on this host at this time.",
                                           HTTP_RESP_NOT_FOUND);
    }

    return HTTP_RESP_OK;
}

bool rrd_function_available(RRDHOST *host, const char *function) {
    if(!host || !host->functions)
        return false;

    bool ret = false;
    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(host->functions->dict, function);
    if(item) {
        struct rrd_host_function *rdcf = dictionary_acquired_item_value(item);
        if(rrd_function_is_available(rdcf, host))
            ret = true;

        dictionary_acquired_item_release(host->functions->dict, item);
    }

    return ret;
}

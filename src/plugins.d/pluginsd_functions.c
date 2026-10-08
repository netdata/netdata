// SPDX-License-Identifier: GPL-3.0-or-later

#include "pluginsd_functions.h"

#define LOG_FUNCTIONS false

// ----------------------------------------------------------------------------
// execution of functions

static void pluginsd_calls_insert_cb(const DICTIONARY_ITEM *item, void *func, void *parser_ptr) {
    struct pluginsd_call *pf = func;

    PARSER  *parser = parser_ptr;

    // leave this code as default, so that when the dictionary is destroyed this will be sent back to the caller
    pf->code = HTTP_RESP_SERVICE_UNAVAILABLE;

    const char *transaction = dictionary_acquired_item_name(item);

    int rc = uuid_parse_flexi(transaction, pf->transaction);
    if(rc != 0)
        netdata_log_error("PLUGINSD: FUNCTION '%s': cannot parse the transaction UUID (the wire name of the call_id)", string2str(pf->function));

    CLEAN_BUFFER *buffer = buffer_create(1024, NULL);
    if(pf->payload && buffer_strlen(pf->payload)) {
        buffer_sprintf(
            buffer,
            PLUGINSD_CALL_FUNCTION_PAYLOAD_BEGIN " %s %d \"%s\" \""HTTP_ACCESS_FORMAT"\" \"%s\" \"%s\"\n",
            transaction,
            pf->timeout_s,
            string2str(pf->function),
            (HTTP_ACCESS_FORMAT_CAST)pf->access,
            pf->source ? pf->source : "",
            content_type_id2string(pf->payload->content_type)
            );

        buffer_fast_strcat(buffer, buffer_tostring(pf->payload), buffer_strlen(pf->payload));
        buffer_strcat(buffer, "\nFUNCTION_PAYLOAD_END\n");
    }
    else {
        buffer_sprintf(
            buffer,
            PLUGINSD_CALL_FUNCTION " %s %d \"%s\" \""HTTP_ACCESS_FORMAT"\" \"%s\"\n",
            transaction,
            pf->timeout_s,
            string2str(pf->function),
            (HTTP_ACCESS_FORMAT_CAST)pf->access,
            pf->source ? pf->source : ""
            );
    }

    // send the command to the plugin
    // IMPORTANT: make sure all commands are sent in 1 call, because in streaming they may interfere with others
    ssize_t ret = send_to_plugin(buffer_tostring(buffer), parser, STREAM_TRAFFIC_TYPE_FUNCTIONS);
    pf->sent_monotonic_ut = now_monotonic_usec();

    if(ret < 0) {
        pf->sent_successfully = false;

        pf->code = HTTP_RESP_SERVICE_UNAVAILABLE;
        netdata_log_error("PLUGINSD: FUNCTION '%s': failed to send it to the plugin, error %zd", string2str(pf->function), ret);
        nrpc_call_error(pf->result_body_wb, "Failed to send this request to the plugin that offered it.", pf->code);
    }
    else {
        pf->sent_successfully = true;

        internal_error(LOG_FUNCTIONS,
                       "PLUGINSD: FUNCTION '%s' with transaction '%s' sent to the plugin (%zd bytes, in %"PRIu64" usec)",
                       string2str(pf->function), dictionary_acquired_item_name(item), ret,
                       pf->sent_monotonic_ut - pf->started_monotonic_ut);
    }
}

static bool pluginsd_calls_conflict_cb(const DICTIONARY_ITEM *item __maybe_unused, void *func __maybe_unused, void *new_func, void *parser_ptr __maybe_unused) {
    struct pluginsd_call *pf = new_func;

    netdata_log_error("PLUGINSD_PARSER: duplicate UUID on pending function '%s' detected. Ignoring the second one.", string2str(pf->function));
    pf->code = nrpc_call_error(pf->result_body_wb, "This transaction is already in progress.", HTTP_RESP_BAD_REQUEST);

    // delivering the SECOND caller's result here, inside the insertion
    // critical section, is safe: that caller's waiter mutex is not yet taken
    // (it is acquired only after its handler returns)
    pf->result.cb(pf->result_body_wb, pf->code, pf->result.data);

    // the loser's owned allocations - the old code leaked payload and source
    string_freez(pf->function);
    buffer_free((void *)pf->payload);
    freez((void *)pf->source);

    return false;
}

static void pluginsd_calls_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *func, void *parser_ptr) {
    struct pluginsd_call *pf = func;
    struct parser *parser = (struct parser *)parser_ptr; (void)parser;

    internal_error(LOG_FUNCTIONS,
                   "PLUGINSD: FUNCTION '%s' result of transaction '%s' received from the plugin "
                   "(%zu bytes, request %"PRIu64" usec, response %"PRIu64" usec)",
                   string2str(pf->function), dictionary_acquired_item_name(item),
                   buffer_strlen(pf->result_body_wb),
                   pf->sent_monotonic_ut - pf->started_monotonic_ut, now_realtime_usec() - pf->sent_monotonic_ut);

    if(pf->code == HTTP_RESP_SERVICE_UNAVAILABLE && !buffer_strlen(pf->result_body_wb))
        nrpc_call_error(pf->result_body_wb, "The plugin that was servicing this request, exited before responding.", pf->code);

    pf->result.cb(pf->result_body_wb, pf->code, pf->result.data);

    string_freez(pf->function);
    buffer_free((void *)pf->payload);
    freez((void *)pf->source);
}

void pluginsd_calls_init(PARSER *parser) {
    // the transport is what everyone who may need to reach this parser later
    // stores (registry entries, cancel/progress hooks, dyncfg nodes); the
    // base entry ref created here is dropped by parser_destroy() after the
    // parser is freed
    parser->inflight.transport = nrpc_transport_create(parser);

    parser->inflight.calls = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE, &dictionary_stats_category_functions, 0);
    dictionary_register_insert_callback(parser->inflight.calls, pluginsd_calls_insert_cb, parser);
    dictionary_register_delete_callback(parser->inflight.calls, pluginsd_calls_delete_cb, parser);
    dictionary_register_conflict_callback(parser->inflight.calls, pluginsd_calls_conflict_cb, parser);
}

// destroy_all: called from parser_destroy() AFTER the transport was marked
// dead and drained, so no execute/cancel/progress/GC can be inside the
// container and a late handler insert gets NULL from the destroyed
// dictionary (its existing 503 branch answers the caller). The pre-destroy
// drain delivers already-pending victims OUTSIDE the locks; the remaining
// live records are delivered by dictionary_destroy() through the delete
// callback - the destroy-time 503 sweep (each record's code was pre-set to
// 503 at insert, so callers waiting on this plugin get a clean 503). A
// straggler that goes pending after the drain is delivered by
// dictionary_destroy() under the lock - bounded, and post-drain no
// waiter-mutex counter-party exists.
void pluginsd_calls_cleanup(PARSER *parser) {
    if(parser->inflight.calls)
        dictionary_garbage_collect_and_deliver(parser->inflight.calls);
    dictionary_destroy(parser->inflight.calls);
    parser->inflight.calls = NULL;
}

// UAF-B destroy-mid-defer: the parser died between RESULT_BEGIN and
// RESULT_END. Only two defer families exist:
//   - the function family: action_data is an OWNED transaction STRING, the
//     response ALIASES pf->result_body_wb (never freed here), and defer.item
//     is the stashed acquired inflight item;
//   - the JSON family: action_data is NULL and the response is OWNED.
// Rule: free action_data always (owned STRING or NULL); free the response
// only for the JSON family; release the stashed item only for the function
// family. The release (without a del) leaves the record to the G3 sweep in
// pluginsd_calls_cleanup(), which delivers it through the normal
// delete callback - and the dictionary is never queued-for-destruction
// forever behind our reference.
void pluginsd_calls_release_deferred(PARSER *parser) {
    if(parser->defer.item) {
        struct pluginsd_call *pf = dictionary_acquired_item_value(parser->defer.item);

        // a truncated stream must not report success; a plugin's own error
        // code (e.g. 404) survives truncation
        if(pf->code >= 200 && pf->code < 300)
            pf->code = HTTP_RESP_SERVICE_UNAVAILABLE;

        dictionary_acquired_item_release(parser->inflight.calls, parser->defer.item);
        parser->defer.item = NULL;
    }

    if(parser->defer.action_data) {
        // function family: the owned transaction STRING
        string_freez((STRING *)parser->defer.action_data);
        parser->defer.action_data = NULL;
    }
    else if(parser->defer.response) {
        // JSON family: the owned response buffer
        buffer_free(parser->defer.response);
    }

    parser->defer.response = NULL;
    parser->defer.action = NULL;
    parser->defer.end_keyword = NULL;
}

// ----------------------------------------------------------------------------

// Expire timed-out transactions. The container phase (traversal, expiry
// writes, the CANCEL notification to the plugin) runs under the parser dict
// WRITE lock in ALL invocations - this is what makes the plain
// smaller_monotonic_timeout_ut field sound; if this phase ever moves outside
// the lock it needs a CAS loop, not relaxed atomics. Delivery (the delete
// callbacks that hand results to waiters) happens strictly OUTSIDE the lock:
// victims are dup'd under the locked traversal WITH their keys copied (the
// item-owned key is freed with the item, and a concurrent sweep can free it
// between our release and our del), then per-victim release-dup-then-
// del-by-the-copied-key - the zero-ref del path is the only one that fires
// the delete callback lock-free, and external releases never fire callbacks.
// A victim some other reference keeps alive (e.g. a RESULT_BEGIN defer) goes
// pending instead and is delivered by a later sweep. Enabling invariant (hard
// constraint): no thread may hold a waiter mutex while acquiring this
// dictionary's items lock - the keyed cancel/progress hooks touch only the
// index lock, and handler always precedes the wait-mutex acquisition.
void pluginsd_calls_garbage_collect(PARSER  *parser, usec_t now_ut) {
    struct gc_victim {
        const DICTIONARY_ITEM *item;
        char *key;
        struct gc_victim *next;
    } *victims = NULL;

    dictionary_write_lock(parser->inflight.calls);

    parser->inflight.smaller_monotonic_timeout_ut = 0;
    size_t skipped = 0;
    struct pluginsd_call *pf;
    dfe_start_write(parser->inflight.calls, pf) {
        // Broker-keyed deadline: the parser dict key IS the compact
        // transaction id (the wire name of the call_id), so this is one O(1) in-flight-table lookup per entry.
        usec_t stop_ut;
        if(unlikely(!nrpc_call_deadline(pf_dfe.name, &stop_ut))) {
            // Invariant failure: every parser record must have an in-flight call record
            // (pluginsd always registers sync=false, incl. the dyncfg
            // intercept, so the in-flight call record outlives the parser entry).
            // SKIP, never reap: reaping would run the delete-callback chain
            // into memory whose invariant already failed.
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "PLUGINSD: transaction '%s' has no in-flight call record; skipping it during garbage collection",
                   pf_dfe.name);
            skipped++;
            continue;
        }

        usec_t effective_ut = nrpc_effective_deadline_ut(stop_ut);

        if (effective_ut < now_ut) {
            // a previous GC pass (serialized by the write lock) has already
            // cancelled this record and queued its deletion; it just has not
            // executed the del yet (dels run after unlock)
            if(pf->gc_collected)
                continue;
            pf->gc_collected = true;

            internal_error(true,
                           "PLUGINSD: FUNCTION '%s' removing expired transaction '%s', after %"PRIu64" usec.",
                           string2str(pf->function), pf_dfe.name, now_ut - pf->started_monotonic_ut);

            if(!buffer_strlen(pf->result_body_wb) || pf->code == HTTP_RESP_OK)
                pf->code = nrpc_call_error(pf->result_body_wb,
                                                   "Timeout waiting for a response.",
                                                   HTTP_RESP_GATEWAY_TIMEOUT);

            // Notify the plugin that the transaction has been cancelled due to timeout,
            // so it can stop any in-progress work for this transaction.
            // GC runs only from handler, whose caller holds a transport
            // dispatcher ref - so this send is inside a guarded section.
            char buffer[2048];
            snprintfz(buffer, sizeof(buffer), PLUGINSD_CALL_FUNCTION_CANCEL " %s\n", pf_dfe.name);
            send_to_plugin(buffer, pf->parser, STREAM_TRAFFIC_TYPE_FUNCTIONS);

            struct gc_victim *v = mallocz(sizeof(*v));
            v->item = dictionary_acquired_item_dup(parser->inflight.calls, pf_dfe.item);
            v->key = strdupz(pf_dfe.name);
            v->next = victims;
            victims = v;
        }

        else if(!parser->inflight.smaller_monotonic_timeout_ut || effective_ut < parser->inflight.smaller_monotonic_timeout_ut)
            parser->inflight.smaller_monotonic_timeout_ut = effective_ut;
    }
    dfe_done(pf);

    // the all-skipped guard: with the timeout left at 0, every future
    // submission would re-run this GC and re-log the misses - push the next
    // attempt one extension away instead
    if(!parser->inflight.smaller_monotonic_timeout_ut && skipped)
        parser->inflight.smaller_monotonic_timeout_ut = nrpc_effective_deadline_ut(now_ut);

    dictionary_write_unlock(parser->inflight.calls);

    // deliver outside all container locks
    while(victims) {
        struct gc_victim *v = victims;
        victims = v->next;

        dictionary_acquired_item_release(parser->inflight.calls, v->item);
        dictionary_del(parser->inflight.calls, v->key);
        freez(v->key);
        freez(v);
    }

    // trailing drain: anything that went pending (a del that raced another
    // reference) is delivered now, outside the locks; free when nothing is
    // pending
    dictionary_garbage_collect_and_deliver(parser->inflight.calls);
}

// ----------------------------------------------------------------------------

// Keyed and self-validating (UAF-A fix): `data` is the transport, the
// transaction re-validates against THIS parser's inflight dictionary in O(1) -
// no pointer-identity scan over recyclable storage, so a recycled record can
// never false-match and cancel the wrong transaction. The dispatcher ref
// guards the whole send (UAF-D class): once the parser starts tearing down,
// the acquire fails and we bail cleanly.
static void pluginsd_function_cancel_to_plugin(const char *transaction, void *data) {
    struct nrpc_transport *t = data;

    if(!nrpc_transport_dispatcher_acquire(t)) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "PLUGINSD: FUNCTION_CANCEL for transaction '%s', but the plugin is not running.",
               transaction ? transaction : "(unset)");
        return;
    }

    PARSER *parser = t->data;

    const DICTIONARY_ITEM *item = transaction && *transaction
        ? dictionary_get_and_acquire_item(parser->inflight.calls, transaction)
        : NULL;

    if(item) {
        internal_error(true, "PLUGINSD: sending function cancellation to plugin for transaction '%s'", transaction);

        char buffer[2048];
        snprintfz(buffer, sizeof(buffer), PLUGINSD_CALL_FUNCTION_CANCEL " %s\n", transaction);

        // send the command to the plugin
        send_to_plugin(buffer, parser, STREAM_TRAFFIC_TYPE_FUNCTIONS);

        dictionary_acquired_item_release(parser->inflight.calls, item);
    }
    else
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "PLUGINSD: FUNCTION_CANCEL request didn't match any pending function requests in pluginsd.d.");

    nrpc_transport_dispatcher_release(t);
}

// `data` is the transport; the dispatcher ref guards the whole send (UAF-A/D
// class). The keyed lookup in THIS parser's dictionary is the validation - a
// transaction belonging to another parser simply misses.
static void pluginsd_function_progress_to_plugin(const char *transaction, void *data) {
    struct nrpc_transport *t = data;

    if(!transaction || !*transaction) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "PLUGINSD: FUNCTION_PROGRESS request without transaction!");
        return;
    }

    if(!nrpc_transport_dispatcher_acquire(t)) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "PLUGINSD: FUNCTION_PROGRESS for transaction '%s', but the plugin is not running.", transaction);
        return;
    }

    PARSER *parser = t->data;
    DICTIONARY *dict = parser->inflight.calls;

    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(dict, transaction);
    if(!item) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "PLUGINSD: FUNCTION_PROGRESS request for transaction '%s' that is not in progress!", transaction);
        nrpc_transport_dispatcher_release(t);
        return;
    }

    internal_error(true, "PLUGINSD: sending function progress to plugin for transaction '%s'", transaction);

    char buffer[512];
    snprintfz(buffer, sizeof(buffer), PLUGINSD_CALL_FUNCTION_PROGRESS " %s\n", transaction);

    // send the command to the plugin
    ssize_t ret = send_to_plugin(buffer, parser, STREAM_TRAFFIC_TYPE_FUNCTIONS);
    if(ret != (ssize_t)strlen(buffer)) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "PLUGINSD: FUNCTION_PROGRESS request failed to send to plugin for transaction '%s'", transaction);
    }

    dictionary_acquired_item_release(dict, item);
    nrpc_transport_dispatcher_release(t);
}

// the handler this transport registers for every plugin-offered method:
// the nrpc_handler_cb_t of the pluginsd transport, invoked by nrpc_call()
int pluginsd_nrpc_handler(struct nrpc_request *req, void *data) {

    // IMPORTANT: this function MUST call the result_cb even on failures

    // `data` is the transport (UAF-C fix): the parser is dereferenced ONLY
    // under a dispatcher ref - once the parser starts tearing down the
    // acquire fails and the caller gets a clean 503 instead of a
    // check-then-use race on freed parser memory
    struct nrpc_transport *transport = data;

    if(!nrpc_transport_dispatcher_acquire(transport)) {
        int code = HTTP_RESP_SERVICE_UNAVAILABLE;
        nrpc_call_error(req->result.wb, "The plugin that offered this function is not available.", code);
        if(req->result.cb)
            req->result.cb(req->result.wb, code, req->result.data);
        return code;
    }

    PARSER *parser = transport->data;

    usec_t now_ut = now_monotonic_usec();

    int timeout_s = (int)((*req->stop_monotonic_ut - now_ut + USEC_PER_SEC / 2) / USEC_PER_SEC);

    struct pluginsd_call tmp = {
            .started_monotonic_ut = now_ut,
            .result_body_wb = req->result.wb,
            .timeout_s = timeout_s,
            .function = string_strdupz(req->function),
            .payload = buffer_dup(req->payload),
            .access = req->user_access,
            .source = req->source ? strdupz(req->source) : NULL,
            .parser = parser,

            .result = {
                    .cb = req->result.cb,
                    .data = req->result.data,
            },
            .progress = {
                    .cb = req->progress.cb,
                    .data = req->progress.data,
            },
    };
    uuid_copy(tmp.transaction, *req->call_id);

    char transaction_str[UUID_COMPACT_STR_LEN];
    uuid_unparse_lower_compact(tmp.transaction, transaction_str);

    dictionary_write_lock(parser->inflight.calls);

    // if there is any error, our dictionary callbacks will call the caller callback to notify
    // the caller about the error - no need for error handling here.
    struct pluginsd_call *t = dictionary_set(parser->inflight.calls, transaction_str, &tmp, sizeof(struct pluginsd_call));
    if(!t) {
        // dictionary_set() returns NULL when the dictionary is destroyed
        // (e.g., the plugin has exited). Clean up and notify the caller.
        dictionary_write_unlock(parser->inflight.calls);

        int code = HTTP_RESP_SERVICE_UNAVAILABLE;
        nrpc_call_error(req->result.wb, "The plugin is not available.", code);
        req->result.cb(req->result.wb, code, req->result.data);

        string_freez(tmp.function);
        buffer_free(tmp.payload);
        freez((void *)tmp.source);

        nrpc_transport_dispatcher_release(transport);
        return code;
    }

    if(!t->sent_successfully) {
        int code = t->code;
        dictionary_write_unlock(parser->inflight.calls);
        // the del delivers this record's error through the delete callback,
        // with no container locks held
        dictionary_del(parser->inflight.calls, transaction_str);
        // the GC now takes the container write lock itself, so its container
        // phase is locked on this path too (the old shape ran it unlocked
        // here, racing the locked readers of the timeout field)
        pluginsd_calls_garbage_collect(parser, now_ut);
        nrpc_transport_dispatcher_release(transport);
        return code;
    }
    else {
        if (req->register_cancel_hook.cb)
            req->register_cancel_hook.cb(req->register_cancel_hook.data, pluginsd_function_cancel_to_plugin, transport);

        if (req->register_progress_hook.cb &&
            (parser->repertoire == PARSER_INIT_PLUGINSD || (parser->repertoire == PARSER_INIT_STREAMING &&
                                                            stream_has_capability(&parser->user, STREAM_CAP_PROGRESS))))
            req->register_progress_hook.cb(req->register_progress_hook.data, pluginsd_function_progress_to_plugin, transport);

        // req->stop_monotonic_ut is valid for the duration of this handler
        // invocation only (the documented validity window) - the record itself
        // carries no deadline pointer, the GC reads deadlines via the in-flight table
        if (!parser->inflight.smaller_monotonic_timeout_ut ||
            nrpc_effective_deadline_ut(*req->stop_monotonic_ut) < parser->inflight.smaller_monotonic_timeout_ut)
            parser->inflight.smaller_monotonic_timeout_ut = nrpc_effective_deadline_ut(*req->stop_monotonic_ut);

        // snapshot the staleness verdict under the lock, but run the GC only
        // AFTER releasing it: the write lock is recursive, so calling the GC
        // while holding it would keep its delivery phase (the delete callbacks
        // that hand results to waiters) effectively under the items write
        // lock, re-arming the ABBA this step closes - the GC takes the lock
        // itself for its container phase
        bool gc_needed = (parser->inflight.smaller_monotonic_timeout_ut < now_ut);

        dictionary_write_unlock(parser->inflight.calls);

        if (gc_needed)
            pluginsd_calls_garbage_collect(parser, now_ut);

        nrpc_transport_dispatcher_release(transport);
        return HTTP_RESP_OK;
    }
}

PARSER_RC pluginsd_function(char **words, size_t num_words, PARSER *parser) {
    // a plugin or a child is registering a function

    bool global = false;
    size_t i = 1;
    if(num_words >= 2 && strcmp(get_word(words, num_words, 1), "GLOBAL") == 0) {
        i++;
        global = true;
    }

    char *name          = get_word(words, num_words, i++);
    char *timeout_str   = get_word(words, num_words, i++);
    char *help          = get_word(words, num_words, i++);
    char *tags          = get_word(words, num_words, i++);
    char *access_str    = get_word(words, num_words, i++);
    char *priority_str  = get_word(words, num_words, i++);
    char *version_str   = get_word(words, num_words, i++);

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_FUNCTION);
    if(!host) return PARSER_RC_ERROR;

    if (unlikely(!timeout_str || !name || !help)) {
        netdata_log_error("PLUGINSD: 'host:%s' got a FUNCTION, without providing the required data (global = '%s', name = '%s', timeout = '%s', priority = '%s', version = '%s', help = '%s'). Ignoring it.",
                          rrdhost_hostname(host),
                          global?"yes":"no",
                          name?name:"(unset)",
                          timeout_str ? timeout_str : "(unset)",
                          priority_str ? priority_str : "(unset)",
                          version_str ? version_str : "(unset)",
                          help?help:"(unset)"
        );
        return PARSER_RC_ERROR;
    }

    // chart-scoped functions no longer exist: a FUNCTION line without GLOBAL
    // registers host-wide. Inside an open chart scope that is a coercion worth
    // reporting once per line; without one it was always the effective behavior.
    if(!global) {
        RRDSET *st = pluginsd_get_scope_chart(parser);
        if(st)
            nd_log(NDLS_DAEMON, NDLP_NOTICE,
                   "PLUGINSD: 'host:%s' got a FUNCTION '%s' within chart '%s' scope - "
                   "chart-scoped functions are no longer supported, registering it host-wide",
                   rrdhost_hostname(host), name, rrdset_id(st));
    }

    // Reserved dynamic-configuration function names ("config", "config <id>")
    // are enforced by the registry itself on the sanitized key: a PLUGIN-source
    // registration of such a name is rejected by nrpc_method_register(), while a
    // STREAM-source one (a child's synthetic "config" proxy on its own host) is
    // accepted - see the reasoning at the enforcement site in nrpc-registry.c.
    bool from_streaming = (parser->repertoire & PARSER_INIT_STREAMING) != 0;

    int timeout_s = PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT;
    if (timeout_str && *timeout_str) {
        timeout_s = str2i(timeout_str);
        if (unlikely(timeout_s <= 0))
            timeout_s = PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT;
    }

    int priority = NRPC_PRIORITY_DEFAULT;
    if(priority_str && *priority_str) {
        priority = str2i(priority_str);
        if(priority <= 0)
            priority = NRPC_PRIORITY_DEFAULT;
    }

    uint32_t version = NRPC_VERSION_DEFAULT;
    if(version_str && *version_str)
        version = str2u(version_str);

    nrpc_method_register(&(struct nrpc_method_desc) {
        .owner = rrdhost_nrpc_owner(host),
        .name = name,
        .help = help,
        .tags = tags,
        .timeout_s = timeout_s,
        .priority = priority,
        .version = version,
        .access = http_access_from_hex_mapping_old_roles(access_str),
        .sync = false,
        .source = from_streaming ? NRPC_SOURCE_STREAM : NRPC_SOURCE_PLUGIN,
        .handler = pluginsd_nrpc_handler,
        .handler_data = parser->inflight.transport,
    });

    parser->user.data_collections_count++;

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_function_del(char **words, size_t num_words, PARSER *parser) {
    // FUNCTION_DEL is name-keyed: with chart-scoped functions gone, the
    // optional GLOBAL word only moves the name to the next slot
    size_t i = 1;
    if(num_words >= 2 && strcmp(get_word(words, num_words, 1), "GLOBAL") == 0)
        i++;

    char *name = get_word(words, num_words, i++);

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_FUNCTION_DEL);
    if(!host) return PARSER_RC_ERROR;

    if (unlikely(!name || !*name)) {
        netdata_log_error("PLUGINSD: 'host:%s' got a FUNCTION_DEL without a name. Ignoring it.",
                          rrdhost_hostname(host));
        return PARSER_RC_ERROR;
    }

    bool from_streaming = (parser->repertoire & PARSER_INIT_STREAMING) != 0;

    if(!nrpc_method_unregister(rrdhost_nrpc_owner(host), name,
                         from_streaming ? NRPC_SOURCE_STREAM : NRPC_SOURCE_PLUGIN)) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "PLUGINSD: 'host:%s' FUNCTION_DEL '%s' - function not found or ownership mismatch",
               rrdhost_hostname(host), name);
    }

    parser->user.data_collections_count++;

    return PARSER_RC_OK;
}

static void pluginsd_function_result_end(struct parser *parser, void *action_data) {
    STRING *key = action_data;

    // release -> del -> sweep: the INVERSE of the registry's del-then-release
    // idiom, and both inversions are load-bearing. This dictionary has no GC
    // linkage and traversals skip deleted items, so del-then-release would
    // DEFER the delete callback; release-then-del delivers it right here (the
    // zero-ref del is the only lock-free delivery path). And release-then-del
    // alone still does not deliver when a GC del landed mid-defer: that del
    // unlinked the item from the hashtable first, so our del is a miss and
    // our release only marks it pending - the sweep below delivers it. Net: a
    // GC del mid-stream delivers at result_end (504 + partial body) in ALL
    // interleavings.
    if(parser->defer.item) {
        dictionary_acquired_item_release(parser->inflight.calls, parser->defer.item);
        parser->defer.item = NULL;
    }

    if(key)
        dictionary_del(parser->inflight.calls, string2str(key));
    string_freez(key);

    // early-returns when nothing is pending, so the happy path pays nothing
    dictionary_garbage_collect_and_deliver(parser->inflight.calls);

    parser->user.data_collections_count++;
}

// UAF-B fix: the caller gets an ACQUIRED item (or NULL), so the record - and
// the result buffer the deferred body lines are appended to - cannot be freed
// by a concurrent GC delete while the caller uses it
static inline const DICTIONARY_ITEM *pluginsd_call_acquire(PARSER *parser, const char *transaction, const char *keyword) {
    const DICTIONARY_ITEM *item = NULL;

    if(transaction && *transaction)
        item = dictionary_get_and_acquire_item(parser->inflight.calls, transaction);

    if(!item)
        netdata_log_error("got a %s for transaction '%s', but the transaction is not found.",
                          keyword, transaction ? transaction : "(unset)");

    return item;
}

PARSER_RC pluginsd_function_result_begin(char **words, size_t num_words, PARSER *parser) {
    char *transaction = get_word(words, num_words, 1);
    char *status = get_word(words, num_words, 2);
    char *format = get_word(words, num_words, 3);
    char *expires = get_word(words, num_words, 4);

    if (unlikely(!transaction || !*transaction || !status || !*status || !format || !*format || !expires || !*expires)) {
        netdata_log_error("got a " PLUGINSD_KEYWORD_FUNCTION_RESULT_BEGIN " without providing the required data (key = '%s', status = '%s', format = '%s', expires = '%s')."
        , transaction ? transaction : "(unset)"
        , status ? status : "(unset)"
        , format ? format : "(unset)"
        , expires ? expires : "(unset)"
        );
    }

    int code = (status && *status) ? str2i(status) : 0;
    if (code <= 0)
        code = HTTP_RESP_BACKEND_RESPONSE_INVALID;

    time_t expiration = (expires && *expires) ? str2l(expires) : 0;

    const DICTIONARY_ITEM *item = pluginsd_call_acquire(parser, transaction, PLUGINSD_KEYWORD_FUNCTION_RESULT_BEGIN);
    struct pluginsd_call *pf = item ? dictionary_acquired_item_value(item) : NULL;
    if(pf) {
        if(format && *format)
            pf->result_body_wb->content_type = content_type_string2id(format);

        pf->code = code;

        pf->result_body_wb->expires = expiration;
        if(expiration <= now_realtime_sec())
            buffer_no_cacheable(pf->result_body_wb);
        else
            buffer_cacheable(pf->result_body_wb);
    }

    parser->defer.response = (pf) ? pf->result_body_wb : NULL;
    parser->defer.end_keyword = PLUGINSD_KEYWORD_FUNCTION_RESULT_END;
    parser->defer.action = pluginsd_function_result_end;
    parser->defer.action_data = string_strdupz(transaction); // it is ok is key is NULL
    parser->defer.item = item; // held across the RESULT_BEGIN..END span (UAF-B)
    parser->flags |= PARSER_DEFER_UNTIL_KEYWORD;

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_function_progress(char **words, size_t num_words, PARSER *parser) {
    size_t i = 1;

    char *transaction   = get_word(words, num_words, i++);
    char *done_str      = get_word(words, num_words, i++);
    char *all_str       = get_word(words, num_words, i++);

    const DICTIONARY_ITEM *item = pluginsd_call_acquire(parser, transaction, PLUGINSD_KEYWORD_FUNCTION_PROGRESS);
    if(item) {
        struct pluginsd_call *pf = dictionary_acquired_item_value(item);

        size_t done = done_str && *done_str ? str2u(done_str) : 0;
        size_t all = all_str && *all_str ? str2u(all_str) : 0;

        if(pf->progress.cb)
            pf->progress.cb(&pf->transaction, pf->progress.data, done, all);

        dictionary_acquired_item_release(parser->inflight.calls, item);
    }

    return PARSER_RC_OK;
}

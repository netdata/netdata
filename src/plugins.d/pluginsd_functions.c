// SPDX-License-Identifier: GPL-3.0-or-later

#include "pluginsd_functions.h"

#define LOG_FUNCTIONS false

// ----------------------------------------------------------------------------
// execution of functions

static void inflight_functions_insert_callback(const DICTIONARY_ITEM *item, void *func, void *parser_ptr) {
    struct inflight_function *pf = func;

    PARSER  *parser = parser_ptr;

    // leave this code as default, so that when the dictionary is destroyed this will be sent back to the caller
    pf->code = HTTP_RESP_SERVICE_UNAVAILABLE;

    const char *transaction = dictionary_acquired_item_name(item);

    int rc = uuid_parse_flexi(transaction, pf->transaction);
    if(rc != 0)
        netdata_log_error("FUNCTION: '%s': cannot parse transaction UUID", string2str(pf->function));

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
        netdata_log_error("FUNCTION '%s': failed to send it to the plugin, error %zd", string2str(pf->function), ret);
        rrd_call_function_error(pf->result_body_wb, "Failed to send this request to the plugin that offered it.", pf->code);
    }
    else {
        pf->sent_successfully = true;

        internal_error(LOG_FUNCTIONS,
                       "FUNCTION '%s' with transaction '%s' sent to collector (%zd bytes, in %"PRIu64" usec)",
                       string2str(pf->function), dictionary_acquired_item_name(item), ret,
                       pf->sent_monotonic_ut - pf->started_monotonic_ut);
    }
}

static bool inflight_functions_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *func __maybe_unused, void *new_func, void *parser_ptr __maybe_unused) {
    struct inflight_function *pf = new_func;

    netdata_log_error("PLUGINSD_PARSER: duplicate UUID on pending function '%s' detected. Ignoring the second one.", string2str(pf->function));
    pf->code = rrd_call_function_error(pf->result_body_wb, "This transaction is already in progress.", HTTP_RESP_BAD_REQUEST);
    pf->result.cb(pf->result_body_wb, pf->code, pf->result.data);
    string_freez(pf->function);

    return false;
}

static void inflight_functions_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *func, void *parser_ptr) {
    struct inflight_function *pf = func;
    struct parser *parser = (struct parser *)parser_ptr; (void)parser;

    internal_error(LOG_FUNCTIONS,
                   "FUNCTION '%s' result of transaction '%s' received from collector "
                   "(%zu bytes, request %"PRIu64" usec, response %"PRIu64" usec)",
                   string2str(pf->function), dictionary_acquired_item_name(item),
                   buffer_strlen(pf->result_body_wb),
                   pf->sent_monotonic_ut - pf->started_monotonic_ut, now_realtime_usec() - pf->sent_monotonic_ut);

    if(pf->code == HTTP_RESP_SERVICE_UNAVAILABLE && !buffer_strlen(pf->result_body_wb))
        rrd_call_function_error(pf->result_body_wb, "The plugin that was servicing this request, exited before responding.", pf->code);

    pf->result.cb(pf->result_body_wb, pf->code, pf->result.data);

    string_freez(pf->function);
    buffer_free((void *)pf->payload);
    freez((void *)pf->source);
}

void pluginsd_inflight_functions_init(PARSER *parser) {
    parser->inflight.functions = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE, &dictionary_stats_category_functions, 0);
    dictionary_register_insert_callback(parser->inflight.functions, inflight_functions_insert_callback, parser);
    dictionary_register_delete_callback(parser->inflight.functions, inflight_functions_delete_callback, parser);
    dictionary_register_conflict_callback(parser->inflight.functions, inflight_functions_conflict_callback, parser);
}

void pluginsd_inflight_functions_cleanup(PARSER *parser) {
    dictionary_destroy(parser->inflight.functions);
}

// ----------------------------------------------------------------------------

void pluginsd_inflight_functions_garbage_collect(PARSER  *parser, usec_t now_ut) {
    parser->inflight.smaller_monotonic_timeout_ut = 0;
    struct inflight_function *pf;
    dfe_start_write(parser->inflight.functions, pf) {
        if (*pf->stop_monotonic_ut + RRDFUNCTIONS_TIMEOUT_EXTENSION_UT < now_ut) {
            internal_error(true,
                           "FUNCTION '%s' removing expired transaction '%s', after %"PRIu64" usec.",
                           string2str(pf->function), pf_dfe.name, now_ut - pf->started_monotonic_ut);

            if(!buffer_strlen(pf->result_body_wb) || pf->code == HTTP_RESP_OK)
                pf->code = rrd_call_function_error(pf->result_body_wb,
                                                   "Timeout waiting for a response.",
                                                   HTTP_RESP_GATEWAY_TIMEOUT);

            dictionary_del(parser->inflight.functions, pf_dfe.name);
        }

        else if(!parser->inflight.smaller_monotonic_timeout_ut || *pf->stop_monotonic_ut + RRDFUNCTIONS_TIMEOUT_EXTENSION_UT < parser->inflight.smaller_monotonic_timeout_ut)
            parser->inflight.smaller_monotonic_timeout_ut = *pf->stop_monotonic_ut + RRDFUNCTIONS_TIMEOUT_EXTENSION_UT;
    }
    dfe_done(pf);
}

// ----------------------------------------------------------------------------

static void pluginsd_function_cancel(void *data) {
    struct inflight_function *look_for = data, *t;

    bool sent = false;
    dfe_start_read(look_for->parser->inflight.functions, t) {
        if(look_for == t) {
            const char *transaction = t_dfe.name;

            internal_error(true, "PLUGINSD: sending function cancellation to plugin for transaction '%s'", transaction);

            char buffer[2048];
            snprintfz(buffer, sizeof(buffer), PLUGINSD_CALL_FUNCTION_CANCEL " %s\n", transaction);

            // send the command to the plugin
            ssize_t ret = send_to_plugin(buffer, t->parser, STREAM_TRAFFIC_TYPE_FUNCTIONS);
            if(ret < 0)
                sent = true;

            break;
        }
    }
    dfe_done(t);

    if(sent <= 0)
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "PLUGINSD: FUNCTION_CANCEL request didn't match any pending function requests in pluginsd.d.");
}

static void pluginsd_function_progress_to_plugin(void *data) {
    struct inflight_function *look_for = data, *t;

    bool sent = false;
    dfe_start_read(look_for->parser->inflight.functions, t) {
        if(look_for == t) {
            const char *transaction = t_dfe.name;

            internal_error(true, "PLUGINSD: sending function progress to plugin for transaction '%s'", transaction);

            char buffer[2048];
            snprintfz(buffer, sizeof(buffer), PLUGINSD_CALL_FUNCTION_PROGRESS " %s\n", transaction);

            // send the command to the plugin
            ssize_t ret = send_to_plugin(buffer, t->parser, STREAM_TRAFFIC_TYPE_FUNCTIONS);
            if(ret < 0)
                sent = true;

            break;
        }
    }
    dfe_done(t);

    if(sent <= 0)
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
                "PLUGINSD: FUNCTION_PROGRESS request didn't match any pending function requests in pluginsd.d.");
}

// this is the function called from
// rrd_call_function_and_wait() and rrd_call_function_async()
int pluginsd_function_execute_cb(struct rrd_function_execute *rfe, void *data) {

    // IMPORTANT: this function MUST call the result_cb even on failures

    PARSER  *parser = data;

    usec_t now_ut = now_monotonic_usec();

    int timeout_s = (int)((*rfe->stop_monotonic_ut - now_ut + USEC_PER_SEC / 2) / USEC_PER_SEC);

    struct inflight_function tmp = {
            .started_monotonic_ut = now_ut,
            .stop_monotonic_ut = rfe->stop_monotonic_ut,
            .result_body_wb = rfe->result.wb,
            .timeout_s = timeout_s,
            .function = string_strdupz(rfe->function),
            .payload = buffer_dup(rfe->payload),
            .access = rfe->user_access,
            .source = rfe->source ? strdupz(rfe->source) : NULL,
            .parser = parser,

            .result = {
                    .cb = rfe->result.cb,
                    .data = rfe->result.data,
            },
            .progress = {
                    .cb = rfe->progress.cb,
                    .data = rfe->progress.data,
            },
    };
    uuid_copy(tmp.transaction, *rfe->transaction);

    char transaction_str[UUID_COMPACT_STR_LEN];
    uuid_unparse_lower_compact(tmp.transaction, transaction_str);

    dictionary_write_lock(parser->inflight.functions);

    // if there is any error, our dictionary callbacks will call the caller callback to notify
    // the caller about the error - no need for error handling here.
    struct inflight_function *t = dictionary_set(parser->inflight.functions, transaction_str, &tmp, sizeof(struct inflight_function));
    if(!t->sent_successfully) {
        int code = t->code;
        dictionary_write_unlock(parser->inflight.functions);
        dictionary_del(parser->inflight.functions, transaction_str);
        pluginsd_inflight_functions_garbage_collect(parser, now_ut);
        return code;
    }
    else {
        if (rfe->register_canceller.cb)
            rfe->register_canceller.cb(rfe->register_canceller.data, pluginsd_function_cancel, t);

        if (rfe->register_progresser.cb &&
            (parser->repertoire == PARSER_INIT_PLUGINSD || (parser->repertoire == PARSER_INIT_STREAMING &&
                                                            stream_has_capability(&parser->user, STREAM_CAP_PROGRESS))))
            rfe->register_progresser.cb(rfe->register_progresser.data, pluginsd_function_progress_to_plugin, t);

        if (!parser->inflight.smaller_monotonic_timeout_ut ||
            *tmp.stop_monotonic_ut + RRDFUNCTIONS_TIMEOUT_EXTENSION_UT < parser->inflight.smaller_monotonic_timeout_ut)
            parser->inflight.smaller_monotonic_timeout_ut = *tmp.stop_monotonic_ut + RRDFUNCTIONS_TIMEOUT_EXTENSION_UT;

        // garbage collect stale inflight functions
        if (parser->inflight.smaller_monotonic_timeout_ut < now_ut)
            pluginsd_inflight_functions_garbage_collect(parser, now_ut);

        dictionary_write_unlock(parser->inflight.functions);

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

    RRDSET *st = (global)? NULL: pluginsd_require_scope_chart(parser, PLUGINSD_KEYWORD_FUNCTION, PLUGINSD_KEYWORD_CHART);
    if(!st) global = true;

    if (unlikely(!timeout_str || !name || !help || (!global && !st))) {
        netdata_log_error("PLUGINSD: 'host:%s/chart:%s' got a FUNCTION, without providing the required data (global = '%s', name = '%s', timeout = '%s', priority = '%s', version = '%s', help = '%s'). Ignoring it.",
                          rrdhost_hostname(host),
                          st?rrdset_id(st):"(unset)",
                          global?"yes":"no",
                          name?name:"(unset)",
                          timeout_str ? timeout_str : "(unset)",
                          priority_str ? priority_str : "(unset)",
                          version_str ? version_str : "(unset)",
                          help?help:"(unset)"
        );
        return PARSER_RC_ERROR;
    }

    int timeout_s = PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT;
    if (timeout_str && *timeout_str) {
        timeout_s = str2i(timeout_str);
        if (unlikely(timeout_s <= 0))
            timeout_s = PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT;
    }

    int priority = RRDFUNCTIONS_PRIORITY_DEFAULT;
    if(priority_str && *priority_str) {
        priority = str2i(priority_str);
        if(priority <= 0)
            priority = RRDFUNCTIONS_PRIORITY_DEFAULT;
    }

    uint32_t version = RRDFUNCTIONS_VERSION_DEFAULT;
    if(version_str && *version_str)
        version = str2u(version_str);

    rrd_function_add(host, st, name, timeout_s, priority, version, help, tags,
                     http_access_from_hex_mapping_old_roles(access_str), false,
                     pluginsd_function_execute_cb, parser);

    parser->user.data_collections_count++;

    return PARSER_RC_OK;
}

static void pluginsd_function_result_end(struct parser *parser, void *action_data) {
    STRING *key = action_data;
    if(key)
        dictionary_del(parser->inflight.functions, string2str(key));
    string_freez(key);

    parser->user.data_collections_count++;
}

static inline struct inflight_function *inflight_function_find(PARSER *parser, const char *transaction) {
    struct inflight_function *pf = NULL;

    if(transaction && *transaction)
        pf = (struct inflight_function *)dictionary_get(parser->inflight.functions, transaction);

    if(!pf)
        netdata_log_error("got a " PLUGINSD_KEYWORD_FUNCTION_RESULT_BEGIN " for transaction '%s', but the transaction is not found.", transaction ? transaction : "(unset)");

    return pf;
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

    struct inflight_function *pf = inflight_function_find(parser, transaction);
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
    parser->flags |= PARSER_DEFER_UNTIL_KEYWORD;

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_function_progress(char **words, size_t num_words, PARSER *parser) {
    size_t i = 1;

    char *transaction   = get_word(words, num_words, i++);
    char *done_str      = get_word(words, num_words, i++);
    char *all_str       = get_word(words, num_words, i++);

    struct inflight_function *pf = inflight_function_find(parser, transaction);
    if(pf) {
        size_t done = done_str && *done_str ? str2u(done_str) : 0;
        size_t all = all_str && *all_str ? str2u(all_str) : 0;

        if(pf->progress.cb)
            pf->progress.cb(pf->progress.data, done, all);
    }

    return PARSER_RC_OK;
}

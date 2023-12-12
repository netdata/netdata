// SPDX-License-Identifier: GPL-3.0-or-later

#include "pluginsd_functions.h"

#define LOG_FUNCTIONS false

// ----------------------------------------------------------------------------
// execution of functions

static void inflight_functions_insert_callback(const DICTIONARY_ITEM *item, void *func, void *parser_ptr) {
    struct inflight_function *pf = func;

    PARSER  *parser = parser_ptr;

    // leave this code as default, so that when the dictionary is destroyed this will be sent back to the caller
    pf->code = HTTP_RESP_GATEWAY_TIMEOUT;

    const char *transaction = dictionary_acquired_item_name(item);

    char buffer[2048 + 1];
    snprintfz(buffer, sizeof(buffer) - 1, "%s %s %d \"%s\"\n",
              pf->payload ? PLUGINSD_KEYWORD_FUNCTION_PAYLOAD : PLUGINSD_KEYWORD_FUNCTION,
              transaction,
              pf->timeout,
              string2str(pf->function));

    // send the command to the plugin
    ssize_t ret = send_to_plugin(buffer, parser);

    pf->sent_ut = now_realtime_usec();

    if(ret < 0) {
        netdata_log_error("FUNCTION '%s': failed to send it to the plugin, error %zd", string2str(pf->function), ret);
        rrd_call_function_error(pf->result_body_wb, "Failed to communicate with collector", HTTP_RESP_SERVICE_UNAVAILABLE);
    }
    else {
        internal_error(LOG_FUNCTIONS,
                       "FUNCTION '%s' with transaction '%s' sent to collector (%zd bytes, in %"PRIu64" usec)",
                string2str(pf->function), dictionary_acquired_item_name(item), ret,
                pf->sent_ut - pf->started_ut);
    }

    if (!pf->payload)
        return;

    // send the payload to the plugin
    ret = send_to_plugin(pf->payload, parser);

    if(ret < 0) {
        netdata_log_error("FUNCTION_PAYLOAD '%s': failed to send function to plugin, error %zd", string2str(pf->function), ret);
        rrd_call_function_error(pf->result_body_wb, "Failed to communicate with collector", HTTP_RESP_SERVICE_UNAVAILABLE);
    }
    else {
        internal_error(LOG_FUNCTIONS,
                       "FUNCTION_PAYLOAD '%s' with transaction '%s' sent to collector (%zd bytes, in %"PRIu64" usec)",
                string2str(pf->function), dictionary_acquired_item_name(item), ret,
                pf->sent_ut - pf->started_ut);
    }

    send_to_plugin("\nFUNCTION_PAYLOAD_END\n", parser);
}

static bool inflight_functions_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *func __maybe_unused, void *new_func, void *parser_ptr __maybe_unused) {
    struct inflight_function *pf = new_func;

    netdata_log_error("PLUGINSD_PARSER: duplicate UUID on pending function '%s' detected. Ignoring the second one.", string2str(pf->function));
    pf->code = rrd_call_function_error(pf->result_body_wb, "This request is already in progress", HTTP_RESP_BAD_REQUEST);
    pf->result_cb(pf->result_body_wb, pf->code, pf->result_cb_data);
    string_freez(pf->function);

    return false;
}

static void delete_job_finalize(struct parser *parser __maybe_unused, struct configurable_plugin *plug, const char *fnc_sig, int code) {
    if (code != DYNCFG_VFNC_RET_CFG_ACCEPTED)
        return;

    char *params_local = strdupz(fnc_sig);
    char *words[DYNCFG_MAX_WORDS];
    size_t words_c = quoted_strings_splitter(params_local, words, DYNCFG_MAX_WORDS, isspace_map_pluginsd);

    if (words_c != 3) {
        netdata_log_error("PLUGINSD_PARSER: invalid number of parameters for delete_job");
        freez(params_local);
        return;
    }

    const char *module = words[1];
    const char *job = words[2];

    delete_job(plug, module, job);

    unlink_job(plug->name, module, job);

    rrdpush_send_job_deleted(localhost, plug->name, module, job);

    freez(params_local);
}

static void set_job_finalize(struct parser *parser __maybe_unused, struct configurable_plugin *plug __maybe_unused, const char *fnc_sig, int code) {
    if (code != DYNCFG_VFNC_RET_CFG_ACCEPTED)
        return;

    char *params_local = strdupz(fnc_sig);
    char *words[DYNCFG_MAX_WORDS];
    size_t words_c = quoted_strings_splitter(params_local, words, DYNCFG_MAX_WORDS, isspace_map_pluginsd);

    if (words_c != 3) {
        netdata_log_error("PLUGINSD_PARSER: invalid number of parameters for set_job_config");
        freez(params_local);
        return;
    }

    const char *module_name = get_word(words, words_c, 1);
    const char *job_name = get_word(words, words_c, 2);

    if (register_job(parser->user.host->configurable_plugins, parser->user.cd->configuration->name, module_name, job_name, JOB_TYPE_USER, JOB_FLG_USER_CREATED, 1)) {
        freez(params_local);
        return;
    }

    // only send this if it is not existing already (register_job cares for that)
    rrdpush_send_dyncfg_reg_job(localhost, parser->user.cd->configuration->name, module_name, job_name, JOB_TYPE_USER, JOB_FLG_USER_CREATED);

    freez(params_local);
}

static void inflight_functions_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *func, void *parser_ptr) {
    struct inflight_function *pf = func;
    struct parser *parser = (struct parser *)parser_ptr;

    internal_error(LOG_FUNCTIONS,
                   "FUNCTION '%s' result of transaction '%s' received from collector (%zu bytes, request %"PRIu64" usec, response %"PRIu64" usec)",
            string2str(pf->function), dictionary_acquired_item_name(item),
            buffer_strlen(pf->result_body_wb), pf->sent_ut - pf->started_ut, now_realtime_usec() - pf->sent_ut);

    if (pf->virtual && SERVING_PLUGINSD(parser)) {
        if (pf->payload) {
            if (strncmp(string2str(pf->function), FUNCTION_NAME_SET_JOB_CONFIG, strlen(FUNCTION_NAME_SET_JOB_CONFIG)) == 0)
                set_job_finalize(parser, parser->user.cd->configuration, string2str(pf->function), pf->code);
            dyn_conf_store_config(string2str(pf->function), pf->payload, parser->user.cd->configuration);
        } else if (strncmp(string2str(pf->function), FUNCTION_NAME_DELETE_JOB, strlen(FUNCTION_NAME_DELETE_JOB)) == 0) {
            delete_job_finalize(parser, parser->user.cd->configuration, string2str(pf->function), pf->code);
        }
    }

    pf->result_cb(pf->result_body_wb, pf->code, pf->result_cb_data);

    string_freez(pf->function);
    freez((void *)pf->payload);
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

void pluginsd_inflight_functions_garbage_collect(PARSER  *parser, usec_t now) {
    parser->inflight.smaller_timeout = 0;
    struct inflight_function *pf;
    dfe_start_write(parser->inflight.functions, pf) {
        if (pf->timeout_ut < now) {
            internal_error(true,
                           "FUNCTION '%s' removing expired transaction '%s', after %"PRIu64" usec.",
                    string2str(pf->function), pf_dfe.name, now - pf->started_ut);

            if(!buffer_strlen(pf->result_body_wb) || pf->code == HTTP_RESP_OK)
                pf->code = rrd_call_function_error(pf->result_body_wb,
                                                   "Timeout waiting for collector response.",
                                                   HTTP_RESP_GATEWAY_TIMEOUT);

            dictionary_del(parser->inflight.functions, pf_dfe.name);
        }

        else if(!parser->inflight.smaller_timeout || pf->timeout_ut < parser->inflight.smaller_timeout)
            parser->inflight.smaller_timeout = pf->timeout_ut;
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

            char buffer[2048 + 1];
            snprintfz(buffer, sizeof(buffer) - 1, "%s %s\n",
                      PLUGINSD_KEYWORD_FUNCTION_CANCEL,
                      transaction);

            // send the command to the plugin
            ssize_t ret = send_to_plugin(buffer, t->parser);
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

// this is the function called from
// rrd_call_function_and_wait() and rrd_call_function_async()
static int pluginsd_function_execute_cb(BUFFER *result_body_wb, int timeout, const char *function,
                                        void *execute_cb_data,
                                        rrd_function_result_callback_t result_cb, void *result_cb_data,
                                        rrd_function_is_cancelled_cb_t is_cancelled_cb __maybe_unused,
                                        void *is_cancelled_cb_data __maybe_unused,
                                        rrd_function_register_canceller_cb_t register_canceller_cb,
                                        void *register_canceller_db_data) {
    PARSER  *parser = execute_cb_data;

    usec_t now = now_realtime_usec();

    struct inflight_function tmp = {
            .started_ut = now,
            .timeout_ut = now + timeout * USEC_PER_SEC + RRDFUNCTIONS_TIMEOUT_EXTENSION_UT,
            .result_body_wb = result_body_wb,
            .timeout = timeout,
            .function = string_strdupz(function),
            .result_cb = result_cb,
            .result_cb_data = result_cb_data,
            .payload = NULL,
            .parser = parser,
    };

    uuid_t uuid;
    uuid_generate_random(uuid);

    char transaction[UUID_STR_LEN];
    uuid_unparse_lower(uuid, transaction);

    dictionary_write_lock(parser->inflight.functions);

    // if there is any error, our dictionary callbacks will call the caller callback to notify
    // the caller about the error - no need for error handling here.
    void *t = dictionary_set(parser->inflight.functions, transaction, &tmp, sizeof(struct inflight_function));
    if(register_canceller_cb)
        register_canceller_cb(register_canceller_db_data, pluginsd_function_cancel, t);

    if(!parser->inflight.smaller_timeout || tmp.timeout_ut < parser->inflight.smaller_timeout)
        parser->inflight.smaller_timeout = tmp.timeout_ut;

    // garbage collect stale inflight functions
    if(parser->inflight.smaller_timeout < now)
        pluginsd_inflight_functions_garbage_collect(parser, now);

    dictionary_write_unlock(parser->inflight.functions);

    return HTTP_RESP_OK;
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

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_FUNCTION);
    if(!host) return PARSER_RC_ERROR;

    RRDSET *st = (global)? NULL: pluginsd_require_scope_chart(parser, PLUGINSD_KEYWORD_FUNCTION, PLUGINSD_KEYWORD_CHART);
    if(!st) global = true;

    if (unlikely(!timeout_str || !name || !help || (!global && !st))) {
        netdata_log_error("PLUGINSD: 'host:%s/chart:%s' got a FUNCTION, without providing the required data (global = '%s', name = '%s', timeout = '%s', help = '%s'). Ignoring it.",
                          rrdhost_hostname(host),
                          st?rrdset_id(st):"(unset)",
                          global?"yes":"no",
                          name?name:"(unset)",
                timeout_str ? timeout_str : "(unset)",
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

    rrd_function_add(host, st, name, timeout_s, priority, help, tags,
                     http_access2id(access_str), false,
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

PARSER_RC pluginsd_function_result_begin(char **words, size_t num_words, PARSER *parser) {
    char *key = get_word(words, num_words, 1);
    char *status = get_word(words, num_words, 2);
    char *format = get_word(words, num_words, 3);
    char *expires = get_word(words, num_words, 4);

    if (unlikely(!key || !*key || !status || !*status || !format || !*format || !expires || !*expires)) {
        netdata_log_error("got a " PLUGINSD_KEYWORD_FUNCTION_RESULT_BEGIN " without providing the required data (key = '%s', status = '%s', format = '%s', expires = '%s')."
        , key ? key : "(unset)"
        , status ? status : "(unset)"
        , format ? format : "(unset)"
        , expires ? expires : "(unset)"
        );
    }

    int code = (status && *status) ? str2i(status) : 0;
    if (code <= 0)
        code = HTTP_RESP_BACKEND_RESPONSE_INVALID;

    time_t expiration = (expires && *expires) ? str2l(expires) : 0;

    struct inflight_function *pf = NULL;

    if(key && *key)
        pf = (struct inflight_function *)dictionary_get(parser->inflight.functions, key);

    if(!pf) {
        netdata_log_error("got a " PLUGINSD_KEYWORD_FUNCTION_RESULT_BEGIN " for transaction '%s', but the transaction is not found.", key?key:"(unset)");
    }
    else {
        if(format && *format)
            pf->result_body_wb->content_type = functions_format_to_content_type(format);

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
    parser->defer.action_data = string_strdupz(key); // it is ok is key is NULL
    parser->flags |= PARSER_DEFER_UNTIL_KEYWORD;

    return PARSER_RC_OK;
}

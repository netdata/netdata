// SPDX-License-Identifier: GPL-3.0-or-later

#include "pluginsd_parser.h"

#define LOG_FUNCTIONS false

static int send_to_plugin(const char *txt, void *data) {
    PARSER *parser = data;

    if(!txt || !*txt)
        return 0;

#ifdef ENABLE_HTTPS
    struct netdata_ssl *ssl = parser->ssl_output;
    if(ssl) {
        if(ssl->conn && ssl->flags == NETDATA_SSL_HANDSHAKE_COMPLETE) {
            size_t size = strlen(txt);
            return SSL_write(ssl->conn, txt, (int)size);
        }

        error("cannot write to SSL connection - connection is not ready.");
        return -1;
    }
#endif

    FILE *fp = parser->output;
    int ret = fprintf(fp, "%s", txt);
    fflush(fp);
    return ret;
}

PARSER_RC pluginsd_set(char **words, size_t num_words, void *user)
{
    char *dimension = get_word(words, num_words, 1);
    char *value = get_word(words, num_words, 2);

    RRDSET *st = ((PARSER_USER_OBJECT *) user)->st;
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;

    if (unlikely(!dimension || !*dimension)) {
        error("requested a SET on chart '%s' of host '%s', without a dimension. Disabling it.", rrdset_id(st), rrdhost_hostname(host));
        goto disable;
    }

    if (unlikely(!value || !*value))
        value = NULL;

    if (unlikely(!st)) {
        error(
            "requested a SET on dimension %s with value %s on host '%s', without a BEGIN. Disabling it.", dimension,
            value ? value : "<nothing>", rrdhost_hostname(host));
        goto disable;
    }

    if (unlikely(rrdset_flag_check(st, RRDSET_FLAG_DEBUG)))
        debug(D_PLUGINSD, "is setting dimension '%s'/'%s' to '%s'", rrdset_id(st), dimension, value ? value : "<nothing>");

    if (value) {
        RRDDIM_ACQUIRED *rda = rrddim_find_and_acquire(st, dimension);
        RRDDIM *rd = rrddim_acquired_to_rrddim(rda);
        if (unlikely(!rd)) {
            error( "requested a SET to dimension with id '%s' on stats '%s' (%s) on host '%s', which does not exist. Disabling it.",
                    dimension, rrdset_name(st), rrdset_id(st), rrdhost_hostname(st->rrdhost));
            goto disable;
        }
        rrddim_set_by_pointer(st, rd, strtoll(value, NULL, 0));
        rrddim_acquired_release(rda);
    }
    return PARSER_RC_OK;

disable:
    ((PARSER_USER_OBJECT *) user)->enabled = 0;
    return PARSER_RC_ERROR;
}

PARSER_RC pluginsd_begin(char **words, size_t num_words, void *user)
{
    char *id = get_word(words, num_words, 1);
    char *microseconds_txt = get_word(words, num_words, 2);

    RRDSET *st = NULL;
    RRDHOST *host = ((PARSER_USER_OBJECT *)user)->host;

    if (unlikely(!id)) {
        error("requested a BEGIN without a chart id for host '%s'. Disabling it.", rrdhost_hostname(host));
        goto disable;
    }

    st = rrdset_find(host, id);
    if (unlikely(!st)) {
        error("requested a BEGIN on chart '%s', which does not exist on host '%s'. Disabling it.", id, rrdhost_hostname(host));
        goto disable;
    }
    ((PARSER_USER_OBJECT *)user)->st = st;

    usec_t microseconds = 0;
    if (microseconds_txt && *microseconds_txt)
        microseconds = str2ull(microseconds_txt);

    if (likely(st->counter_done)) {
        if (likely(microseconds)) {
            if (((PARSER_USER_OBJECT *)user)->trust_durations)
                rrdset_next_usec_unfiltered(st, microseconds);
            else
                rrdset_next_usec(st, microseconds);
        } else
            rrdset_next(st);
    }
    return PARSER_RC_OK;
disable:
    ((PARSER_USER_OBJECT *)user)->enabled = 0;
    return PARSER_RC_ERROR;
}

PARSER_RC pluginsd_end(char **words, size_t num_words, void *user)
{
    UNUSED(words);
    UNUSED(num_words);

    RRDSET *st = ((PARSER_USER_OBJECT *) user)->st;
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;

    if (unlikely(!st)) {
        error("requested an END, without a BEGIN on host '%s'. Disabling it.", rrdhost_hostname(host));
        ((PARSER_USER_OBJECT *) user)->enabled = 0;
        return PARSER_RC_ERROR;
    }

    if (unlikely(rrdset_flag_check(st, RRDSET_FLAG_DEBUG)))
        debug(D_PLUGINSD, "requested an END on chart '%s'", rrdset_id(st));

    ((PARSER_USER_OBJECT *) user)->st = NULL;
    ((PARSER_USER_OBJECT *) user)->count++;
    rrdset_done(st);
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_chart(char **words, size_t num_words, void *user)
{
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;
    if (unlikely(!host && !((PARSER_USER_OBJECT *) user)->host_exists)) {
        debug(D_PLUGINSD, "Ignoring chart belonging to missing or ignored host.");
        return PARSER_RC_OK;
    }

    char *type = get_word(words, num_words, 1);
    char *name = get_word(words, num_words, 2);
    char *title = get_word(words, num_words, 3);
    char *units = get_word(words, num_words, 4);
    char *family = get_word(words, num_words, 5);
    char *context = get_word(words, num_words, 6);
    char *chart = get_word(words, num_words, 7);
    char *priority_s = get_word(words, num_words, 8);
    char *update_every_s = get_word(words, num_words, 9);
    char *options = get_word(words, num_words, 10);
    char *plugin = get_word(words, num_words, 11);
    char *module = get_word(words, num_words, 12);

    // parse the id from type
    char *id = NULL;
    if (likely(type && (id = strchr(type, '.')))) {
        *id = '\0';
        id++;
    }

    // make sure we have the required variables
    if (unlikely((!type || !*type || !id || !*id))) {
        if (likely(host))
            error("requested a CHART, without a type.id, on host '%s'. Disabling it.", rrdhost_hostname(host));
        else
            error("requested a CHART, without a type.id. Disabling it.");
        ((PARSER_USER_OBJECT *) user)->enabled = 0;
        return PARSER_RC_ERROR;
    }

    // parse the name, and make sure it does not include 'type.'
    if (unlikely(name && *name)) {
        // when data are streamed from child nodes
        // name will be type.name
        // so we have to remove 'type.' from name too
        size_t len = strlen(type);
        if (strncmp(type, name, len) == 0 && name[len] == '.')
            name = &name[len + 1];

        // if the name is the same with the id,
        // or is just 'NULL', clear it.
        if (unlikely(strcmp(name, id) == 0 || strcasecmp(name, "NULL") == 0 || strcasecmp(name, "(NULL)") == 0))
            name = NULL;
    }

    int priority = 1000;
    if (likely(priority_s && *priority_s))
        priority = str2i(priority_s);

    int update_every = ((PARSER_USER_OBJECT *) user)->cd->update_every;
    if (likely(update_every_s && *update_every_s))
        update_every = str2i(update_every_s);
    if (unlikely(!update_every))
        update_every = ((PARSER_USER_OBJECT *) user)->cd->update_every;

    RRDSET_TYPE chart_type = RRDSET_TYPE_LINE;
    if (unlikely(chart))
        chart_type = rrdset_type_id(chart);

    if (unlikely(name && !*name))
        name = NULL;
    if (unlikely(family && !*family))
        family = NULL;
    if (unlikely(context && !*context))
        context = NULL;
    if (unlikely(!title))
        title = "";
    if (unlikely(!units))
        units = "unknown";

    debug(
        D_PLUGINSD,
        "creating chart type='%s', id='%s', name='%s', family='%s', context='%s', chart='%s', priority=%d, update_every=%d",
        type, id, name ? name : "", family ? family : "", context ? context : "", rrdset_type_name(chart_type),
        priority, update_every);

    RRDSET *st = NULL;

    st = rrdset_create(
        host, type, id, name, family, context, title, units,
        (plugin && *plugin) ? plugin : ((PARSER_USER_OBJECT *)user)->cd->filename,
        module, priority, update_every,
        chart_type);

    if (likely(st)) {
        if (options && *options) {
            if (strstr(options, "obsolete"))
                rrdset_is_obsolete(st);
            else
                rrdset_isnot_obsolete(st);

            if (strstr(options, "detail"))
                rrdset_flag_set(st, RRDSET_FLAG_DETAIL);
            else
                rrdset_flag_clear(st, RRDSET_FLAG_DETAIL);

            if (strstr(options, "hidden"))
                rrdset_flag_set(st, RRDSET_FLAG_HIDDEN);
            else
                rrdset_flag_clear(st, RRDSET_FLAG_HIDDEN);

            if (strstr(options, "store_first"))
                rrdset_flag_set(st, RRDSET_FLAG_STORE_FIRST);
            else
                rrdset_flag_clear(st, RRDSET_FLAG_STORE_FIRST);
        } else {
            rrdset_isnot_obsolete(st);
            rrdset_flag_clear(st, RRDSET_FLAG_DETAIL);
            rrdset_flag_clear(st, RRDSET_FLAG_STORE_FIRST);
        }
    }
    ((PARSER_USER_OBJECT *)user)->st = st;

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_chart_definition_end(char **words, size_t num_words, void *user)
{
    const char *first_entry_txt = get_word(words, num_words, 1);
    const char *last_entry_txt = get_word(words, num_words, 2);

    if(unlikely(!first_entry_txt || !last_entry_txt)) {
        error("REPLAY: received " PLUGINSD_KEYWORD_CHART_DEFINITION_END " command without first or last entry. Disabling it.");
        return PARSER_RC_ERROR;
    }

    long first_entry_child = str2l(first_entry_txt);
    long last_entry_child = str2l(last_entry_txt);

    PARSER_USER_OBJECT *user_object = (PARSER_USER_OBJECT *) user;

    RRDHOST *host = user_object->host;
    RRDSET *st = user_object->st;
    if(unlikely(!host || !st)) {
        error("REPLAY: received " PLUGINSD_KEYWORD_CHART_DEFINITION_END " command without a chart. Disabling it.");
        return PARSER_RC_ERROR;
    }

    internal_error(
               (first_entry_child != 0 || last_entry_child != 0)
            && (first_entry_child == 0 || last_entry_child == 0),
            "REPLAY: received " PLUGINSD_KEYWORD_CHART_DEFINITION_END " with malformed timings (first time %llu, last time %llu).",
            (unsigned long long)first_entry_child, (unsigned long long)last_entry_child);

//    internal_error(
//            true,
//            "REPLAY host '%s', chart '%s': received " PLUGINSD_KEYWORD_CHART_DEFINITION_END " first time %llu, last time %llu.",
//            rrdhost_hostname(host), rrdset_id(st),
//            (unsigned long long)first_entry_child, (unsigned long long)last_entry_child);

    rrdset_flag_clear(st, RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED);

    bool ok = replicate_chart_request(send_to_plugin, user_object->parser, host, st, first_entry_child, last_entry_child, 0, 0);
    return ok ? PARSER_RC_OK : PARSER_RC_ERROR;
}

PARSER_RC pluginsd_dimension(char **words, size_t num_words, void *user)
{
    char *id = get_word(words, num_words, 1);
    char *name = get_word(words, num_words, 2);
    char *algorithm = get_word(words, num_words, 3);
    char *multiplier_s = get_word(words, num_words, 4);
    char *divisor_s = get_word(words, num_words, 5);
    char *options = get_word(words, num_words, 6);

    RRDSET *st = ((PARSER_USER_OBJECT *) user)->st;
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;
    if (unlikely(!host && !((PARSER_USER_OBJECT *) user)->host_exists)) {
        debug(D_PLUGINSD, "Ignoring dimension belonging to missing or ignored host.");
        return PARSER_RC_OK;
    }

    if (unlikely(!id)) {
        error(
            "requested a DIMENSION, without an id, host '%s' and chart '%s'. Disabling it.", rrdhost_hostname(host),
            st ? rrdset_id(st) : "UNSET");
        goto disable;
    }

    if (unlikely(!st && !((PARSER_USER_OBJECT *) user)->st_exists)) {
        error("requested a DIMENSION, without a CHART, on host '%s'. Disabling it.", rrdhost_hostname(host));
        goto disable;
    }

    long multiplier = 1;
    if (multiplier_s && *multiplier_s) {
        multiplier = strtol(multiplier_s, NULL, 0);
        if (unlikely(!multiplier))
            multiplier = 1;
    }

    long divisor = 1;
    if (likely(divisor_s && *divisor_s)) {
        divisor = strtol(divisor_s, NULL, 0);
        if (unlikely(!divisor))
            divisor = 1;
    }

    if (unlikely(!algorithm || !*algorithm))
        algorithm = "absolute";

    if (unlikely(st && rrdset_flag_check(st, RRDSET_FLAG_DEBUG)))
        debug(
            D_PLUGINSD,
            "creating dimension in chart %s, id='%s', name='%s', algorithm='%s', multiplier=%ld, divisor=%ld, hidden='%s'",
            rrdset_id(st), id, name ? name : "", rrd_algorithm_name(rrd_algorithm_id(algorithm)), multiplier, divisor,
            options ? options : "");

    RRDDIM *rd = rrddim_add(st, id, name, multiplier, divisor, rrd_algorithm_id(algorithm));
    int unhide_dimension = 1;

    rrddim_option_clear(rd, RRDDIM_OPTION_DONT_DETECT_RESETS_OR_OVERFLOWS);
    if (options && *options) {
        if (strstr(options, "obsolete") != NULL)
            rrddim_is_obsolete(st, rd);
        else
            rrddim_isnot_obsolete(st, rd);

        unhide_dimension = !strstr(options, "hidden");

        if (strstr(options, "noreset") != NULL)
            rrddim_option_set(rd, RRDDIM_OPTION_DONT_DETECT_RESETS_OR_OVERFLOWS);
        if (strstr(options, "nooverflow") != NULL)
            rrddim_option_set(rd, RRDDIM_OPTION_DONT_DETECT_RESETS_OR_OVERFLOWS);
    } else
        rrddim_isnot_obsolete(st, rd);

    if (likely(unhide_dimension)) {
        rrddim_option_clear(rd, RRDDIM_OPTION_HIDDEN);
        if (rrddim_flag_check(rd, RRDDIM_FLAG_META_HIDDEN)) {
            rrddim_flag_clear(rd, RRDDIM_FLAG_META_HIDDEN);
            metaqueue_dimension_update_flags(rd);
        }
    } else {
        rrddim_option_set(rd, RRDDIM_OPTION_HIDDEN);
        if (!rrddim_flag_check(rd, RRDDIM_FLAG_META_HIDDEN)) {
            rrddim_flag_set(rd, RRDDIM_FLAG_META_HIDDEN);
            metaqueue_dimension_update_flags(rd);
        }
    }

    return PARSER_RC_OK;
disable:
    ((PARSER_USER_OBJECT *)user)->enabled = 0;
    return PARSER_RC_ERROR;
}

// ----------------------------------------------------------------------------
// execution of functions

struct inflight_function {
    int code;
    int timeout;
    BUFFER *destination_wb;
    STRING *function;
    void (*callback)(BUFFER *wb, int code, void *callback_data);
    void *callback_data;
    usec_t timeout_ut;
    usec_t started_ut;
    usec_t sent_ut;
};

static void inflight_functions_insert_callback(const DICTIONARY_ITEM *item, void *func, void *parser_ptr) {
    struct inflight_function *pf = func;

    PARSER  *parser = parser_ptr;

    // leave this code as default, so that when the dictionary is destroyed this will be sent back to the caller
    pf->code = HTTP_RESP_GATEWAY_TIMEOUT;

    char buffer[2048 + 1];
    snprintfz(buffer, 2048, "FUNCTION %s %d \"%s\"\n",
                      dictionary_acquired_item_name(item),
                      pf->timeout,
                      string2str(pf->function));

    // send the command to the plugin
    int ret = send_to_plugin(buffer, parser);

    pf->sent_ut = now_realtime_usec();

    if(ret < 0) {
        error("FUNCTION: failed to send function to plugin, fprintf() returned error %d", ret);
        rrd_call_function_error(pf->destination_wb, "Failed to communicate with collector", HTTP_RESP_BACKEND_FETCH_FAILED);
    }
    else {
        internal_error(LOG_FUNCTIONS,
                       "FUNCTION '%s' with transaction '%s' sent to collector (%d bytes, in %llu usec)",
                       string2str(pf->function), dictionary_acquired_item_name(item), ret,
                       pf->sent_ut - pf->started_ut);
    }
}

static bool inflight_functions_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *func __maybe_unused, void *new_func, void *parser_ptr __maybe_unused) {
    struct inflight_function *pf = new_func;

    error("PLUGINSD_PARSER: duplicate UUID on pending function '%s' detected. Ignoring the second one.", string2str(pf->function));
    pf->code = rrd_call_function_error(pf->destination_wb, "This request is already in progress", HTTP_RESP_BAD_REQUEST);
    pf->callback(pf->destination_wb, pf->code, pf->callback_data);
    string_freez(pf->function);

    return false;
}
static void inflight_functions_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *func, void *parser_ptr __maybe_unused) {
    struct inflight_function *pf = func;

    internal_error(LOG_FUNCTIONS,
                   "FUNCTION '%s' result of transaction '%s' received from collector (%zu bytes, request %llu usec, response %llu usec)",
                   string2str(pf->function), dictionary_acquired_item_name(item),
                   buffer_strlen(pf->destination_wb), pf->sent_ut - pf->started_ut, now_realtime_usec() - pf->sent_ut);

    pf->callback(pf->destination_wb, pf->code, pf->callback_data);
    string_freez(pf->function);
}

void inflight_functions_init(PARSER *parser) {
    parser->inflight.functions = dictionary_create(DICT_OPTION_DONT_OVERWRITE_VALUE);
    dictionary_register_insert_callback(parser->inflight.functions, inflight_functions_insert_callback, parser);
    dictionary_register_delete_callback(parser->inflight.functions, inflight_functions_delete_callback, parser);
    dictionary_register_conflict_callback(parser->inflight.functions, inflight_functions_conflict_callback, parser);
}

static void inflight_functions_garbage_collect(PARSER  *parser, usec_t now) {
    parser->inflight.smaller_timeout = 0;
    struct inflight_function *pf;
    dfe_start_write(parser->inflight.functions, pf) {
        if (pf->timeout_ut < now) {
            internal_error(true,
                           "FUNCTION '%s' removing expired transaction '%s', after %llu usec.",
                           string2str(pf->function), pf_dfe.name, now - pf->started_ut);

            if(!buffer_strlen(pf->destination_wb) || pf->code == HTTP_RESP_OK)
                pf->code = rrd_call_function_error(pf->destination_wb,
                                                   "Timeout waiting for collector response.",
                                                   HTTP_RESP_GATEWAY_TIMEOUT);

            dictionary_del(parser->inflight.functions, pf_dfe.name);
        }

        else if(!parser->inflight.smaller_timeout || pf->timeout_ut < parser->inflight.smaller_timeout)
            parser->inflight.smaller_timeout = pf->timeout_ut;
    }
    dfe_done(pf);
}

// this is the function that is called from
// rrd_call_function_and_wait() and rrd_call_function_async()
static int pluginsd_execute_function_callback(BUFFER *destination_wb, int timeout, const char *function, void *collector_data, void (*callback)(BUFFER *wb, int code, void *callback_data), void *callback_data) {
    PARSER  *parser = collector_data;

    usec_t now = now_realtime_usec();

    struct inflight_function tmp = {
        .started_ut = now,
        .timeout_ut = now + timeout * USEC_PER_SEC,
        .destination_wb = destination_wb,
        .timeout = timeout,
        .function = string_strdupz(function),
        .callback = callback,
        .callback_data = callback_data,
    };

    uuid_t uuid;
    uuid_generate_time(uuid);

    char key[UUID_STR_LEN];
    uuid_unparse_lower(uuid, key);

    dictionary_write_lock(parser->inflight.functions);

    // if there is any error, our dictionary callbacks will call the caller callback to notify
    // the caller about the error - no need for error handling here.
    dictionary_set(parser->inflight.functions, key, &tmp, sizeof(struct inflight_function));

    if(!parser->inflight.smaller_timeout || tmp.timeout_ut < parser->inflight.smaller_timeout)
        parser->inflight.smaller_timeout = tmp.timeout_ut;

    // garbage collect stale inflight functions
    if(parser->inflight.smaller_timeout < now)
        inflight_functions_garbage_collect(parser, now);

    dictionary_write_unlock(parser->inflight.functions);

    return HTTP_RESP_OK;
}

PARSER_RC pluginsd_function(char **words, size_t num_words, void *user)
{
    bool global = false;
    size_t i = 1;
    if(num_words >= 2 && strcmp(get_word(words, num_words, 1), "GLOBAL") == 0) {
        i++;
        global = true;
    }

    char *name      = get_word(words, num_words, i++);
    char *timeout_s = get_word(words, num_words, i++);
    char *help      = get_word(words, num_words, i++);

    RRDSET *st = (global)?NULL:((PARSER_USER_OBJECT *) user)->st;
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;

    if (unlikely(!host || !timeout_s || !name || !help || (!global && !st))) {
        error("requested a FUNCTION, without providing the required data (global = '%s', name = '%s', timeout = '%s', help = '%s'), host '%s', chart '%s'. Ignoring it.",
              global?"yes":"no",
              name?name:"(unset)",
              timeout_s?timeout_s:"(unset)",
              help?help:"(unset)",
              host?rrdhost_hostname(host):"(unset)",
              st?rrdset_id(st):"(unset)");
        return PARSER_RC_OK;
    }

    int timeout = PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT;
    if (timeout_s && *timeout_s) {
        timeout = str2i(timeout_s);
        if (unlikely(timeout <= 0))
            timeout = PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT;
    }

    PARSER  *parser = ((PARSER_USER_OBJECT *) user)->parser;
    rrd_collector_add_function(host, st, name, timeout, help, false, pluginsd_execute_function_callback, parser);

    return PARSER_RC_OK;
}

static void pluginsd_function_result_end(struct parser *parser, void *action_data) {
    STRING *key = action_data;
    if(key)
        dictionary_del(parser->inflight.functions, string2str(key));
    string_freez(key);
}

PARSER_RC pluginsd_function_result_begin(char **words, size_t num_words, void *user)
{
    char *key = get_word(words, num_words, 1);
    char *status = get_word(words, num_words, 2);
    char *format = get_word(words, num_words, 3);
    char *expires = get_word(words, num_words, 4);

    if (unlikely(!key || !*key || !status || !*status || !format || !*format || !expires || !*expires)) {
        error("got a " PLUGINSD_KEYWORD_FUNCTION_RESULT_BEGIN " without providing the required data (key = '%s', status = '%s', format = '%s', expires = '%s')."
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

    PARSER  *parser = ((PARSER_USER_OBJECT *) user)->parser;

    struct inflight_function *pf = NULL;

    if(key && *key)
        pf = (struct inflight_function *)dictionary_get(parser->inflight.functions, key);

    if(!pf) {
        error("got a " PLUGINSD_KEYWORD_FUNCTION_RESULT_BEGIN " for transaction '%s', but the transaction is not found.", key?key:"(unset)");
    }
    else {
        if(format && *format)
            pf->destination_wb->contenttype = functions_format_to_content_type(format);

        pf->code = code;

        pf->destination_wb->expires = expiration;
        if(expiration <= now_realtime_sec())
            buffer_no_cacheable(pf->destination_wb);
        else
            buffer_cacheable(pf->destination_wb);
    }

    parser->defer.response = (pf) ? pf->destination_wb : NULL;
    parser->defer.end_keyword = PLUGINSD_KEYWORD_FUNCTION_RESULT_END;
    parser->defer.action = pluginsd_function_result_end;
    parser->defer.action_data = string_strdupz(key); // it is ok is key is NULL
    parser->flags |= PARSER_DEFER_UNTIL_KEYWORD;

    return PARSER_RC_OK;
}

// ----------------------------------------------------------------------------

PARSER_RC pluginsd_variable(char **words, size_t num_words, void *user)
{
    char *name = get_word(words, num_words, 1);
    char *value = get_word(words, num_words, 2);
    NETDATA_DOUBLE v;

    RRDSET *st = ((PARSER_USER_OBJECT *) user)->st;
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;

    int global = (st) ? 0 : 1;

    if (name && *name) {
        if ((strcmp(name, "GLOBAL") == 0 || strcmp(name, "HOST") == 0)) {
            global = 1;
            name = get_word(words, num_words, 2);
            value = get_word(words, num_words, 3);
        } else if ((strcmp(name, "LOCAL") == 0 || strcmp(name, "CHART") == 0)) {
            global = 0;
            name = get_word(words, num_words, 2);
            value = get_word(words, num_words, 3);
        }
    }

    if (unlikely(!name || !*name)) {
        error("requested a VARIABLE on host '%s', without a variable name. Disabling it.", rrdhost_hostname(host));
        ((PARSER_USER_OBJECT *)user)->enabled = 0;
        return PARSER_RC_ERROR;
    }

    if (unlikely(!value || !*value))
        value = NULL;

    if (unlikely(!value)) {
        error("cannot set %s VARIABLE '%s' on host '%s' to an empty value", (global) ? "HOST" : "CHART", name,
              rrdhost_hostname(host));
        return PARSER_RC_OK;
    }

    if (!global && !st) {
        error("cannot find/create CHART VARIABLE '%s' on host '%s' without a chart", name, rrdhost_hostname(host));
        return PARSER_RC_OK;
    }

    char *endptr = NULL;
    v = (NETDATA_DOUBLE)str2ndd(value, &endptr);
    if (unlikely(endptr && *endptr)) {
        if (endptr == value)
            error(
                "the value '%s' of VARIABLE '%s' on host '%s' cannot be parsed as a number", value, name,
                rrdhost_hostname(host));
        else
            error(
                "the value '%s' of VARIABLE '%s' on host '%s' has leftovers: '%s'", value, name, rrdhost_hostname(host),
                endptr);
    }

    if (global) {
        const RRDVAR_ACQUIRED *rva = rrdvar_custom_host_variable_add_and_acquire(host, name);
        if (rva) {
            rrdvar_custom_host_variable_set(host, rva, v);
            rrdvar_custom_host_variable_release(host, rva);
        }
        else
            error("cannot find/create HOST VARIABLE '%s' on host '%s'", name, rrdhost_hostname(host));
    } else {
        const RRDSETVAR_ACQUIRED *rsa = rrdsetvar_custom_chart_variable_add_and_acquire(st, name);
        if (rsa) {
            rrdsetvar_custom_chart_variable_set(st, rsa, v);
            rrdsetvar_custom_chart_variable_release(st, rsa);
        }
        else
            error("cannot find/create CHART VARIABLE '%s' on host '%s', chart '%s'", name, rrdhost_hostname(host), rrdset_id(st));
    }

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_flush(char **words __maybe_unused, size_t num_words __maybe_unused, void *user)
{
    debug(D_PLUGINSD, "requested a FLUSH");
    ((PARSER_USER_OBJECT *) user)->st = NULL;
    ((PARSER_USER_OBJECT *) user)->replay.start_time = 0;
    ((PARSER_USER_OBJECT *) user)->replay.end_time = 0;
    ((PARSER_USER_OBJECT *) user)->replay.start_time_ut = 0;
    ((PARSER_USER_OBJECT *) user)->replay.end_time_ut = 0;
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_disable(char **words __maybe_unused, size_t num_words __maybe_unused, void *user __maybe_unused)
{
    info("called DISABLE. Disabling it.");
    ((PARSER_USER_OBJECT *) user)->enabled = 0;
    return PARSER_RC_ERROR;
}

PARSER_RC pluginsd_label(char **words, size_t num_words, void *user)
{
    const char *name = get_word(words, num_words, 1);
    const char *label_source = get_word(words, num_words, 2);
    const char *value = get_word(words, num_words, 3);

    if (!name || !label_source || !value) {
        error("Ignoring malformed or empty LABEL command.");
        return PARSER_RC_OK;
    }

    char *store = (char *)value;
    bool allocated_store = false;

    if(unlikely(num_words > 4)) {
        allocated_store = true;
        store = mallocz(PLUGINSD_LINE_MAX + 1);
        size_t remaining = PLUGINSD_LINE_MAX;
        char *move = store;
        char *word;
        for(size_t i = 3; i < num_words && remaining > 2 && (word = get_word(words, num_words, i)) ;i++) {
            if(i > 3) {
                *move++ = ' ';
                *move = '\0';
                remaining--;
            }

            size_t length = strlen(word);
            if (length > remaining)
                length = remaining;

            remaining -= length;
            memcpy(move, word, length);
            move += length;
            *move = '\0';
        }
    }

    if(unlikely(!((PARSER_USER_OBJECT *) user)->new_host_labels))
        ((PARSER_USER_OBJECT *) user)->new_host_labels = rrdlabels_create();

    rrdlabels_add(((PARSER_USER_OBJECT *)user)->new_host_labels,
                  name,
                  store,
                  str2l(label_source));

    if (allocated_store)
        freez(store);

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_overwrite(char **words __maybe_unused, size_t num_words __maybe_unused, void *user)
{
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;
    debug(D_PLUGINSD, "requested to OVERWRITE host labels");

    if(!host->rrdlabels)
        host->rrdlabels = rrdlabels_create();

    rrdlabels_migrate_to_these(host->rrdlabels, (DICTIONARY *) (((PARSER_USER_OBJECT *)user)->new_host_labels));
    metaqueue_store_host_labels(host->machine_guid);

    rrdlabels_destroy(((PARSER_USER_OBJECT *)user)->new_host_labels);
    ((PARSER_USER_OBJECT *)user)->new_host_labels = NULL;
    return PARSER_RC_OK;
}


PARSER_RC pluginsd_clabel(char **words, size_t num_words, void *user)
{
    const char *name = get_word(words, num_words, 1);
    const char *value = get_word(words, num_words, 2);
    const char *label_source = get_word(words, num_words, 3);

    if (!name || !value || !*label_source) {
        error("Ignoring malformed or empty CHART LABEL command.");
        return PARSER_RC_OK;
    }

    if(unlikely(!((PARSER_USER_OBJECT *) user)->chart_rrdlabels_linked_temporarily)) {
        ((PARSER_USER_OBJECT *)user)->chart_rrdlabels_linked_temporarily = ((PARSER_USER_OBJECT *)user)->st->rrdlabels;
        rrdlabels_unmark_all(((PARSER_USER_OBJECT *)user)->chart_rrdlabels_linked_temporarily);
    }

    rrdlabels_add(((PARSER_USER_OBJECT *)user)->chart_rrdlabels_linked_temporarily,
                  name, value, str2l(label_source));

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_clabel_commit(char **words __maybe_unused, size_t num_words __maybe_unused, void *user)
{
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;
    RRDSET *st = ((PARSER_USER_OBJECT *)user)->st;

    if (unlikely(!st))
        return PARSER_RC_OK;

    debug(D_PLUGINSD, "requested to commit chart labels");

    if(!((PARSER_USER_OBJECT *)user)->chart_rrdlabels_linked_temporarily) {
        error("requested CLABEL_COMMIT on host '%s', without a BEGIN, ignoring it.", rrdhost_hostname(host));
        return PARSER_RC_OK;
    }

    rrdlabels_remove_all_unmarked(((PARSER_USER_OBJECT *)user)->chart_rrdlabels_linked_temporarily);

    rrdset_flag_set(st, RRDSET_FLAG_METADATA_UPDATE);
    rrdhost_flag_set(st->rrdhost, RRDHOST_FLAG_METADATA_UPDATE);

    ((PARSER_USER_OBJECT *)user)->chart_rrdlabels_linked_temporarily = NULL;
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_replay_rrdset_begin(char **words, size_t num_words, void *user)
{
    char *id = get_word(words, num_words, 1);
    char *start_time_str = get_word(words, num_words, 2);
    char *end_time_str = get_word(words, num_words, 3);

    RRDSET *st = ((PARSER_USER_OBJECT *) user)->st;
    RRDHOST *host = ((PARSER_USER_OBJECT *)user)->host;

    if (unlikely(!id || (!st && !*id))) {
        error("REPLAY: requested a " PLUGINSD_KEYWORD_REPLAY_BEGIN " without a chart id for host '%s'. Disabling it.", rrdhost_hostname(host));
        goto disable;
    }

    if(*id) {
        st = rrdset_find(host, id);
        if (unlikely(!st)) {
            error("requested a " PLUGINSD_KEYWORD_REPLAY_BEGIN " on chart '%s', which does not exist on host '%s'. Disabling it.",
                  id, rrdhost_hostname(host));
            goto disable;
        }

        ((PARSER_USER_OBJECT *) user)->st = st;
        ((PARSER_USER_OBJECT *) user)->replay.start_time = 0;
        ((PARSER_USER_OBJECT *) user)->replay.end_time = 0;
        ((PARSER_USER_OBJECT *) user)->replay.start_time_ut = 0;
        ((PARSER_USER_OBJECT *) user)->replay.end_time_ut = 0;
    }

    if(start_time_str && end_time_str) {
        time_t start_time = strtol(start_time_str, NULL, 0);
        time_t end_time = strtol(end_time_str, NULL, 0);

        if(start_time && end_time) {
            if (start_time > end_time) {
                error("REPLAY: requested a " PLUGINSD_KEYWORD_REPLAY_BEGIN " on chart '%s' ('%s') on host '%s', but timings are invalid (%ld to %ld). Disabling it.",
                      rrdset_name(st), rrdset_id(st), rrdhost_hostname(st->rrdhost), start_time, end_time);
                goto disable;
            }

            if (end_time - start_time != st->update_every)
                rrdset_set_update_every(st, end_time - start_time);

            st->last_collected_time.tv_sec = end_time;
            st->last_collected_time.tv_usec = 0;

            st->last_updated.tv_sec = end_time;
            st->last_updated.tv_usec = 0;

            ((PARSER_USER_OBJECT *) user)->replay.start_time = start_time;
            ((PARSER_USER_OBJECT *) user)->replay.end_time = end_time;
            ((PARSER_USER_OBJECT *) user)->replay.start_time_ut = (usec_t) start_time * USEC_PER_SEC;
            ((PARSER_USER_OBJECT *) user)->replay.end_time_ut = (usec_t) end_time * USEC_PER_SEC;

            st->counter++;
            st->counter_done++;

            // these are only needed for db mode RAM, SAVE, MAP, ALLOC
            st->current_entry++;
            if(st->current_entry >= st->entries)
                st->current_entry -= st->entries;
        }
    }

    return PARSER_RC_OK;

disable:
    ((PARSER_USER_OBJECT *)user)->enabled = 0;
    return PARSER_RC_ERROR;
}

PARSER_RC pluginsd_replay_set(char **words, size_t num_words, void *user)
{
    char *dimension = get_word(words, num_words, 1);
    char *value_str = get_word(words, num_words, 2);
    char *flags_str = get_word(words, num_words, 3);

    RRDSET *st = ((PARSER_USER_OBJECT *) user)->st;
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;

    if (unlikely(!st)) {
        error("REPLAY: requested a " PLUGINSD_KEYWORD_REPLAY_SET " on dimension '%s' on host '%s', without a " PLUGINSD_KEYWORD_REPLAY_BEGIN ". Disabling it.",
              dimension, rrdhost_hostname(host));
        goto disable;
    }

    if (unlikely(!dimension || !*dimension)) {
        error("REPLAY: requested a " PLUGINSD_KEYWORD_REPLAY_SET " on chart '%s' of host '%s', without a dimension. Disabling it.",
              rrdset_id(st), rrdhost_hostname(host));
        goto disable;
    }

    if (unlikely(!((PARSER_USER_OBJECT *) user)->replay.start_time || !((PARSER_USER_OBJECT *) user)->replay.end_time)) {
        error("REPLAY: requested a " PLUGINSD_KEYWORD_REPLAY_SET " on dimension '%s' on host '%s', without timings from a " PLUGINSD_KEYWORD_REPLAY_BEGIN ". Disabling it.",
              dimension, rrdhost_hostname(host));
        goto disable;
    }

    if (unlikely(!value_str || !*value_str))
        value_str = "nan";

    if(unlikely(!flags_str))
        flags_str = "";

    if (unlikely(rrdset_flag_check(st, RRDSET_FLAG_DEBUG)))
        debug(D_PLUGINSD, "REPLAY: is replaying dimension '%s'/'%s' to '%s'", rrdset_id(st), dimension, value_str);

    if (likely(value_str)) {
        RRDDIM_ACQUIRED *rda = rrddim_find_and_acquire(st, dimension);
        RRDDIM *rd = rrddim_acquired_to_rrddim(rda);
        if(unlikely(!rd)) {
            error("REPLAY: requested a " PLUGINSD_KEYWORD_REPLAY_SET " to dimension with id '%s' on chart '%s' ('%s') on host '%s', which does not exist. Disabling it.",
                  dimension, rrdset_name(st), rrdset_id(st), rrdhost_hostname(st->rrdhost));
            goto disable;
        }

        RRDDIM_FLAGS rd_flags = rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE | RRDDIM_FLAG_ARCHIVED);

        if(unlikely(rd_flags & RRDDIM_FLAG_OBSOLETE)) {
            error("Dimension %s in chart '%s' has the OBSOLETE flag set, but it is collected.", rrddim_name(rd), rrdset_id(st));
            rrddim_isnot_obsolete(st, rd);
        }

        if(!(rd_flags & RRDDIM_FLAG_ARCHIVED)) {
            NETDATA_DOUBLE value = strtondd(value_str, NULL);
            SN_FLAGS flags = SN_FLAG_NONE;

            char c;
            while ((c = *flags_str++)) {
                switch (c) {
                    case 'R':
                        flags |= SN_FLAG_RESET;
                        break;

                    case 'E':
                        flags |= SN_EMPTY_SLOT;
                        value = NAN;
                        break;

                    default:
                        error("unknown flag '%c'", c);
                        break;
                }
            }

            if (!netdata_double_isnumber(value)) {
                value = NAN;
                flags = SN_EMPTY_SLOT;
            }

            rrddim_store_metric(rd, ((PARSER_USER_OBJECT *) user)->replay.end_time_ut, value, flags);
            rd->last_collected_time.tv_sec = ((PARSER_USER_OBJECT *) user)->replay.end_time;
            rd->last_collected_time.tv_usec = 0;
            rd->collections_counter++;
        }
        else
            error("Dimension %s in chart '%s' has the ARCHIVED flag set, but it is collected. Ignoring data.", rrddim_name(rd), rrdset_id(st));

        rrddim_acquired_release(rda);
    }
    return PARSER_RC_OK;

disable:
    ((PARSER_USER_OBJECT *) user)->enabled = 0;
    return PARSER_RC_ERROR;
}

PARSER_RC pluginsd_replay_rrddim_collection_state(char **words, size_t num_words, void *user)
{
    char *dimension = get_word(words, num_words, 1);
    char *last_collected_ut_str = get_word(words, num_words, 2);
    char *last_collected_value_str = get_word(words, num_words, 3);
    char *last_calculated_value_str = get_word(words, num_words, 4);
    char *last_stored_value_str = get_word(words, num_words, 5);

    RRDSET *st = ((PARSER_USER_OBJECT *) user)->st;
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;

    if (unlikely(!st)) {
        error("REPLAY: requested a " PLUGINSD_KEYWORD_REPLAY_RRDDIM_STATE " on dimension '%s' on host '%s', without a " PLUGINSD_KEYWORD_REPLAY_BEGIN ". Disabling it.",
              dimension, rrdhost_hostname(host));
        goto disable;
    }

    if (unlikely(!dimension || !*dimension)) {
        error("REPLAY: requested a " PLUGINSD_KEYWORD_REPLAY_RRDDIM_STATE " on chart '%s' of host '%s', without a dimension. Disabling it.",
              rrdset_id(st), rrdhost_hostname(host));
        goto disable;
    }

    RRDDIM_ACQUIRED *rda = rrddim_find_and_acquire(st, dimension);
    RRDDIM *rd = rrddim_acquired_to_rrddim(rda);
    if(unlikely(!rd)) {
        error("REPLAY: requested a " PLUGINSD_KEYWORD_REPLAY_RRDDIM_STATE " to dimension with id '%s' on chart '%s' ('%s') on host '%s', which does not exist. Disabling it.",
              dimension, rrdset_name(st), rrdset_id(st), rrdhost_hostname(st->rrdhost));
        goto disable;
    }

    usec_t dim_last_collected_ut = (usec_t)rd->last_collected_time.tv_sec * USEC_PER_SEC + (usec_t)rd->last_collected_time.tv_usec;
    usec_t last_collected_ut = last_collected_ut_str ? str2ull(last_collected_ut_str) : 0;
    if(last_collected_ut > dim_last_collected_ut) {
        rd->last_collected_time.tv_sec = last_collected_ut / USEC_PER_SEC;
        rd->last_collected_time.tv_usec = last_collected_ut % USEC_PER_SEC;
    }

    rd->last_collected_value = last_collected_value_str ? str2ll(last_collected_value_str, NULL) : 0;
    rd->last_calculated_value = last_calculated_value_str ? str2ndd(last_calculated_value_str, NULL) : 0;
    rd->last_stored_value = last_stored_value_str ? str2ndd(last_stored_value_str, NULL) : 0.0;
    rrddim_acquired_release(rda);
    return PARSER_RC_OK;

disable:
    ((PARSER_USER_OBJECT *) user)->enabled = 0;
    return PARSER_RC_ERROR;
}

PARSER_RC pluginsd_replay_rrdset_collection_state(char **words, size_t num_words, void *user)
{
    char *last_collected_ut_str = get_word(words, num_words, 1);
    char *last_updated_ut_str = get_word(words, num_words, 2);

    RRDSET *st = ((PARSER_USER_OBJECT *) user)->st;
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;

    if (unlikely(!st)) {
        error("REPLAY: requested a " PLUGINSD_KEYWORD_REPLAY_RRDSET_STATE " on host '%s', without a " PLUGINSD_KEYWORD_REPLAY_BEGIN ". Disabling it.",
              rrdhost_hostname(host));
        goto disable;
    }

    usec_t chart_last_collected_ut = (usec_t)st->last_collected_time.tv_sec * USEC_PER_SEC + (usec_t)st->last_collected_time.tv_usec;
    usec_t last_collected_ut = last_collected_ut_str ? str2ull(last_collected_ut_str) : 0;
    if(last_collected_ut > chart_last_collected_ut) {
        st->last_collected_time.tv_sec = last_collected_ut / USEC_PER_SEC;
        st->last_collected_time.tv_usec = last_collected_ut % USEC_PER_SEC;
    }

    usec_t chart_last_updated_ut = (usec_t)st->last_updated.tv_sec * USEC_PER_SEC + (usec_t)st->last_updated.tv_usec;
    usec_t last_updated_ut = last_updated_ut_str ? str2ull(last_updated_ut_str) : 0;
    if(last_updated_ut > chart_last_updated_ut) {
        st->last_updated.tv_sec = last_updated_ut / USEC_PER_SEC;
        st->last_updated.tv_usec = last_updated_ut % USEC_PER_SEC;
    }

    st->counter++;
    st->counter_done++;

    return PARSER_RC_OK;

disable:
    ((PARSER_USER_OBJECT *) user)->enabled = 0;
    return PARSER_RC_ERROR;
}

PARSER_RC pluginsd_replay_end(char **words, size_t num_words, void *user)
{
    if (num_words < 7) {
        error("REPLAY: malformed " PLUGINSD_KEYWORD_REPLAY_END " command");
        return PARSER_RC_ERROR;
    }

    time_t update_every_child = str2l(get_word(words, num_words, 1));
    time_t first_entry_child = (time_t)str2ull(get_word(words, num_words, 2));
    time_t last_entry_child = (time_t)str2ull(get_word(words, num_words, 3));

    bool start_streaming = (strcmp(get_word(words, num_words, 4), "true") == 0);
    time_t first_entry_requested = (time_t)str2ull(get_word(words, num_words, 5));
    time_t last_entry_requested = (time_t)str2ull(get_word(words, num_words, 6));

    PARSER_USER_OBJECT *user_object = user;

    RRDSET *st = ((PARSER_USER_OBJECT *) user)->st;
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;

    if (unlikely(!st)) {
        error("REPLAY: requested a " PLUGINSD_KEYWORD_REPLAY_END " on host '%s', without a " PLUGINSD_KEYWORD_REPLAY_BEGIN ". Disabling it.",
              rrdhost_hostname(host));
        return PARSER_RC_ERROR;
    }

//    internal_error(true,
//                   "REPLAY: host '%s', chart '%s': received " PLUGINSD_KEYWORD_REPLAY_END " child first_t = %llu, last_t = %llu, start_streaming = %s, requested first_t = %llu, last_t = %llu",
//                   rrdhost_hostname(host), rrdset_id(st),
//                   (unsigned long long)first_entry_child, (unsigned long long)last_entry_child,
//                   start_streaming?"true":"false",
//                   (unsigned long long)first_entry_requested, (unsigned long long)last_entry_requested);

    ((PARSER_USER_OBJECT *) user)->st = NULL;
    ((PARSER_USER_OBJECT *) user)->count++;

    st->counter++;
    st->counter_done++;

    if (start_streaming) {
        if (st->update_every != update_every_child)
            rrdset_set_update_every(st, update_every_child);

        rrdset_flag_set(st, RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED);
        rrdset_flag_clear(st, RRDSET_FLAG_SYNC_CLOCK);
        return PARSER_RC_OK;
    }

    bool ok = replicate_chart_request(send_to_plugin, user_object->parser, host, st, first_entry_child, last_entry_child,
                                      first_entry_requested, last_entry_requested);
    return ok ? PARSER_RC_OK : PARSER_RC_ERROR;
}

static void pluginsd_process_thread_cleanup(void *ptr) {
    PARSER *parser = (PARSER *)ptr;
    rrd_collector_finished();
    parser_destroy(parser);
}

// New plugins.d parser

inline size_t pluginsd_process(RRDHOST *host, struct plugind *cd, FILE *fp_plugin_input, FILE *fp_plugin_output, int trust_durations)
{
    int enabled = cd->enabled;

    if (!fp_plugin_input || !fp_plugin_output || !enabled) {
        cd->enabled = 0;
        return 0;
    }

    if (unlikely(fileno(fp_plugin_input) == -1)) {
        error("input file descriptor given is not a valid stream");
        cd->serial_failures++;
        return 0;
    }

    if (unlikely(fileno(fp_plugin_output) == -1)) {
        error("output file descriptor given is not a valid stream");
        cd->serial_failures++;
        return 0;
    }

    clearerr(fp_plugin_input);
    clearerr(fp_plugin_output);

    PARSER_USER_OBJECT user = {
        .enabled = cd->enabled,
        .host = host,
        .cd = cd,
        .trust_durations = trust_durations
    };

    // fp_plugin_output = our input; fp_plugin_input = our output
    PARSER *parser = parser_init(host, &user, fp_plugin_output, fp_plugin_input, PARSER_INPUT_SPLIT, NULL);

    rrd_collector_started();

    // this keeps the parser with its current value
    // so, parser needs to be allocated before pushing it
    netdata_thread_cleanup_push(pluginsd_process_thread_cleanup, parser);

    user.parser = parser;

    while (likely(!parser_next(parser))) {
        if (unlikely(netdata_exit || parser_action(parser,  NULL)))
            break;
    }

    // free parser with the pop function
    netdata_thread_cleanup_pop(1);

    cd->enabled = user.enabled;
    size_t count = user.count;

    if (likely(count)) {
        cd->successful_collections += count;
        cd->serial_failures = 0;
    }
    else
        cd->serial_failures++;

    return count;
}

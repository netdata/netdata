// SPDX-License-Identifier: GPL-3.0-or-later

#include "pluginsd_parser.h"

#define LOG_FUNCTIONS false

/*
 * This is the action defined for the FLUSH command
 */
PARSER_RC pluginsd_set_action(void *user, RRDSET *st, RRDDIM *rd, long long int value)
{
    UNUSED(user);

    rrddim_set_by_pointer(st, rd, value);
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_flush_action(void *user, RRDSET *st)
{
    UNUSED(user);
    UNUSED(st);
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_begin_action(void *user, RRDSET *st, usec_t microseconds, int trust_durations)
{
    UNUSED(user);
    if (likely(st->counter_done)) {
        if (likely(microseconds)) {
            if (trust_durations)
                rrdset_next_usec_unfiltered(st, microseconds);
            else
                rrdset_next_usec(st, microseconds);
        } else
            rrdset_next(st);
    }
    return PARSER_RC_OK;
}


PARSER_RC pluginsd_end_action(void *user, RRDSET *st)
{
    UNUSED(user);

    rrdset_done(st);
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_chart_action(void *user, char *type, char *id, char *name, char *family, char *context, char *title, char *units, char *plugin,
           char *module, int priority, int update_every, RRDSET_TYPE chart_type, char *options)
{
    RRDSET *st = NULL;
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;

    st = rrdset_create(
        host, type, id, name, family, context, title, units,
        plugin, module, priority, update_every,
        chart_type);

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
    ((PARSER_USER_OBJECT *)user)->st = st;

    return PARSER_RC_OK;
}


PARSER_RC pluginsd_disable_action(void *user)
{
    UNUSED(user);

    info("called DISABLE. Disabling it.");
    ((PARSER_USER_OBJECT *) user)->enabled = 0;
    return PARSER_RC_ERROR;
}


PARSER_RC pluginsd_variable_action(void *user, RRDHOST *host, RRDSET *st, char *name, int global, NETDATA_DOUBLE value)
{
    UNUSED(user);

    if (global) {
        const RRDVAR_ACQUIRED *rva = rrdvar_custom_host_variable_add_and_acquire(host, name);
        if (rva) {
            rrdvar_custom_host_variable_set(host, rva, value);
            rrdvar_custom_host_variable_release(host, rva);
        }
        else
            error("cannot find/create HOST VARIABLE '%s' on host '%s'", name, rrdhost_hostname(host));
    } else {
        const RRDSETVAR_ACQUIRED *rsa = rrdsetvar_custom_chart_variable_add_and_acquire(st, name);
        if (rsa) {
            rrdsetvar_custom_chart_variable_set(st, rsa, value);
            rrdsetvar_custom_chart_variable_release(st, rsa);
        }
        else
            error("cannot find/create CHART VARIABLE '%s' on host '%s', chart '%s'", name, rrdhost_hostname(host), rrdset_id(st));
    }
    return PARSER_RC_OK;
}



PARSER_RC pluginsd_dimension_action(void *user, RRDSET *st, char *id, char *name, char *algorithm, long multiplier, long divisor, char *options,
                                    RRD_ALGORITHM algorithm_type)
{
    UNUSED(user);
    UNUSED(algorithm);

    RRDDIM *rd = rrddim_add(st, id, name, multiplier, divisor, algorithm_type);
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
            (void)sql_set_dimension_option(&rd->metric_uuid, NULL);
            rrddim_flag_clear(rd, RRDDIM_FLAG_META_HIDDEN);
        }
    } else {
        rrddim_option_set(rd, RRDDIM_OPTION_HIDDEN);
        if (!rrddim_flag_check(rd, RRDDIM_FLAG_META_HIDDEN)) {
           (void)sql_set_dimension_option(&rd->metric_uuid, "hidden");
            rrddim_flag_set(rd, RRDDIM_FLAG_META_HIDDEN);
        }
    }
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_label_action(void *user, char *key, char *value, RRDLABEL_SRC source)
{

    if(unlikely(!((PARSER_USER_OBJECT *) user)->new_host_labels))
        ((PARSER_USER_OBJECT *) user)->new_host_labels = rrdlabels_create();

    rrdlabels_add(((PARSER_USER_OBJECT *)user)->new_host_labels, key, value, source);

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_overwrite_action(void *user, RRDHOST *host, DICTIONARY *new_host_labels)
{
    UNUSED(user);

    if(!host->rrdlabels)
        host->rrdlabels = rrdlabels_create();

    rrdlabels_migrate_to_these(host->rrdlabels, new_host_labels);
    sql_store_host_labels(host);

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_set(char **words, void *user, PLUGINSD_ACTION  *plugins_action)
{
    char *dimension = words[1];
    char *value = words[2];

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
        RRDDIM *rd = rrddim_find(st, dimension);
        if (unlikely(!rd)) {
            error(
                "requested a SET to dimension with id '%s' on stats '%s' (%s) on host '%s', which does not exist. Disabling it.",
                dimension, rrdset_name(st), rrdset_id(st), rrdhost_hostname(st->rrdhost));
            goto disable;
        } else {
            if (plugins_action->set_action) {
                return plugins_action->set_action(
                    user, st, rd, strtoll(value, NULL, 0));
            }
        }
    }
    return PARSER_RC_OK;

disable:
    ((PARSER_USER_OBJECT *) user)->enabled = 0;
    return PARSER_RC_ERROR;
}

PARSER_RC pluginsd_begin(char **words, void *user, PLUGINSD_ACTION  *plugins_action)
{
    char *id = words[1];
    char *microseconds_txt = words[2];

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

    if (plugins_action->begin_action) {
        return plugins_action->begin_action(user, st, microseconds,
                                            ((PARSER_USER_OBJECT *)user)->trust_durations);
    }
    return PARSER_RC_OK;
disable:
    ((PARSER_USER_OBJECT *)user)->enabled = 0;
    return PARSER_RC_ERROR;
}

PARSER_RC pluginsd_end(char **words, void *user, PLUGINSD_ACTION  *plugins_action)
{
    UNUSED(words);
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
    if (plugins_action->end_action) {
        return plugins_action->end_action(user, st);
    }
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_chart(char **words, void *user, PLUGINSD_ACTION  *plugins_action)
{
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;
    if (unlikely(!host && !((PARSER_USER_OBJECT *) user)->host_exists)) {
        debug(D_PLUGINSD, "Ignoring chart belonging to missing or ignored host.");
        return PARSER_RC_OK;
    }

    char *type = words[1];
    char *name = words[2];
    char *title = words[3];
    char *units = words[4];
    char *family = words[5];
    char *context = words[6];
    char *chart = words[7];
    char *priority_s = words[8];
    char *update_every_s = words[9];
    char *options = words[10];
    char *plugin = words[11];
    char *module = words[12];

    int have_action = ((plugins_action->chart_action) != NULL);

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

    if (have_action) {
        return plugins_action->chart_action(
            user, type, id, name, family, context, title, units,
            (plugin && *plugin) ? plugin : ((PARSER_USER_OBJECT *)user)->cd->filename, module, priority, update_every,
            chart_type, options);
    }

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_dimension(char **words, void *user, PLUGINSD_ACTION  *plugins_action)
{
    char *id = words[1];
    char *name = words[2];
    char *algorithm = words[3];
    char *multiplier_s = words[4];
    char *divisor_s = words[5];
    char *options = words[6];

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

    if (plugins_action->dimension_action) {
        return plugins_action->dimension_action(
                user, st, id, name, algorithm,
            multiplier, divisor, (options && *options)?options:NULL, rrd_algorithm_id(algorithm));
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
    FILE *fp = parser->output;

    // leave this code as default, so that when the dictionary is destroyed this will be sent back to the caller
    pf->code = HTTP_RESP_GATEWAY_TIMEOUT;

    // send the command to the plugin
    int ret = fprintf(fp, "FUNCTION %s %d \"%s\"\n",
            dictionary_acquired_item_name(item),
            pf->timeout,
            string2str(pf->function));

    pf->sent_ut = now_realtime_usec();

    if(ret < 0) {
        error("FUNCTION: failed to send function to plugin, fprintf() returned error %d", ret);
        rrd_call_function_error(pf->destination_wb, "Failed to communicate with collector", HTTP_RESP_BACKEND_FETCH_FAILED);
    }
    else {
        fflush(fp);

        internal_error(LOG_FUNCTIONS,
                       "FUNCTION '%s' with transaction '%s' sent to collector (%d bytes, fd %d, in %llu usec)",
                       string2str(pf->function), dictionary_acquired_item_name(item), ret, fileno(fp),
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

PARSER_RC pluginsd_function(char **words, void *user, PLUGINSD_ACTION  *plugins_action __maybe_unused)
{
    bool global = false;
    int i = 1;
    if(strcmp(words[i], "GLOBAL") == 0) {
        i++;
        global = true;
    }

    char *name      = words[i++];
    char *timeout_s = words[i++];
    char *help      = words[i++];

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

PARSER_RC pluginsd_function_result_begin(char **words, void *user, PLUGINSD_ACTION  *plugins_action __maybe_unused)
{
    char *key = words[1];
    char *status = words[2];
    char *format = words[3];
    char *expires = words[4];

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

PARSER_RC pluginsd_variable(char **words, void *user, PLUGINSD_ACTION  *plugins_action)
{
    char *name = words[1];
    char *value = words[2];
    NETDATA_DOUBLE v;

    RRDSET *st = ((PARSER_USER_OBJECT *) user)->st;
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;

    int global = (st) ? 0 : 1;

    if (name && *name) {
        if ((strcmp(name, "GLOBAL") == 0 || strcmp(name, "HOST") == 0)) {
            global = 1;
            name = words[2];
            value = words[3];
        } else if ((strcmp(name, "LOCAL") == 0 || strcmp(name, "CHART") == 0)) {
            global = 0;
            name = words[2];
            value = words[3];
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

    if (plugins_action->variable_action) {
        return plugins_action->variable_action(user, host, st, name, global, v);
    }

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_flush(char **words, void *user, PLUGINSD_ACTION  *plugins_action)
{
    UNUSED(words);
    debug(D_PLUGINSD, "requested a FLUSH");
    RRDSET *st = ((PARSER_USER_OBJECT *) user)->st;
    ((PARSER_USER_OBJECT *) user)->st = NULL;
    if (plugins_action->flush_action) {
        return plugins_action->flush_action(user, st);
    }
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_disable(char **words, void *user, PLUGINSD_ACTION  *plugins_action)
{
    UNUSED(user);
    UNUSED(words);

    if (plugins_action->disable_action) {
        return plugins_action->disable_action(user);
    }
    return PARSER_RC_ERROR;
}

PARSER_RC pluginsd_label(char **words, void *user, PLUGINSD_ACTION  *plugins_action)
{
    char *store;

    if (!words[1] || !words[2] || !words[3]) {
        error("Ignoring malformed or empty LABEL command.");
        return PARSER_RC_OK;
    }
    if (!words[4])
        store = words[3];
    else {
        store = callocz(PLUGINSD_LINE_MAX + 1, sizeof(char));
        size_t remaining = PLUGINSD_LINE_MAX;
        char *move = store;
        int i = 3;
        while (i < PLUGINSD_MAX_WORDS) {
            size_t length = strlen(words[i]);
            if ((length + 1) >= remaining)
                break;

            remaining -= (length + 1);
            memcpy(move, words[i], length);
            move += length;
            *move++ = ' ';

            i++;
            if (!words[i])
                break;
        }
    }

    if (plugins_action->label_action) {
        PARSER_RC rc = plugins_action->label_action(user, words[1], store, strtol(words[2], NULL, 10));
        if (store != words[3])
            freez(store);
        return rc;
    }

    if (store != words[3])
        freez(store);
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_overwrite(char **words, void *user, PLUGINSD_ACTION  *plugins_action)
{
    UNUSED(words);

    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;
    debug(D_PLUGINSD, "requested to OVERWRITE host labels");

    PARSER_RC rc = PARSER_RC_OK;

    if (plugins_action->overwrite_action)
        rc = plugins_action->overwrite_action(user, host, ((PARSER_USER_OBJECT *)user)->new_host_labels);

    rrdlabels_destroy(((PARSER_USER_OBJECT *)user)->new_host_labels);
    ((PARSER_USER_OBJECT *)user)->new_host_labels = NULL;

    return rc;
}


PARSER_RC pluginsd_clabel(char **words, void *user, PLUGINSD_ACTION  *plugins_action __maybe_unused)
{
    if (!words[1] || !words[2] || !words[3]) {
        error("Ignoring malformed or empty CHART LABEL command.");
        return PARSER_RC_OK;
    }

    if(unlikely(!((PARSER_USER_OBJECT *) user)->chart_rrdlabels_linked_temporarily)) {
        ((PARSER_USER_OBJECT *)user)->chart_rrdlabels_linked_temporarily = ((PARSER_USER_OBJECT *)user)->st->rrdlabels;
        rrdlabels_unmark_all(((PARSER_USER_OBJECT *)user)->chart_rrdlabels_linked_temporarily);
    }

    rrdlabels_add(((PARSER_USER_OBJECT *)user)->chart_rrdlabels_linked_temporarily, words[1], words[2], strtol(words[3], NULL, 10));

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_clabel_commit(char **words, void *user, PLUGINSD_ACTION  *plugins_action __maybe_unused)
{
    UNUSED(words);

    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;
    debug(D_PLUGINSD, "requested to commit chart labels");

    if(!((PARSER_USER_OBJECT *)user)->chart_rrdlabels_linked_temporarily) {
        error("requested CLABEL_COMMIT on host '%s', without a BEGIN, ignoring it.", rrdhost_hostname(host));
        return PARSER_RC_OK;
    }

    rrdlabels_remove_all_unmarked(((PARSER_USER_OBJECT *)user)->chart_rrdlabels_linked_temporarily);

    ((PARSER_USER_OBJECT *)user)->chart_rrdlabels_linked_temporarily = NULL;
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_guid(char **words, void *user, PLUGINSD_ACTION *plugins_action)
{
    char *uuid_str = words[1];
    uuid_t uuid;

    if (unlikely(!uuid_str)) {
        error("requested a GUID, without a uuid.");
        return PARSER_RC_ERROR;
    }
    if (unlikely(strlen(uuid_str) != GUID_LEN || uuid_parse(uuid_str, uuid) == -1)) {
        error("requested a GUID, without a valid uuid string.");
        return PARSER_RC_ERROR;
    }

    debug(D_PLUGINSD, "Parsed uuid=%s", uuid_str);
    if (plugins_action->guid_action) {
        return plugins_action->guid_action(user, &uuid);
    }

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_context(char **words, void *user, PLUGINSD_ACTION *plugins_action)
{
    char *uuid_str = words[1];
    uuid_t uuid;

    if (unlikely(!uuid_str)) {
        error("requested a CONTEXT, without a uuid.");
        return PARSER_RC_ERROR;
    }
    if (unlikely(strlen(uuid_str) != GUID_LEN || uuid_parse(uuid_str, uuid) == -1)) {
        error("requested a CONTEXT, without a valid uuid string.");
        return PARSER_RC_ERROR;
    }

    debug(D_PLUGINSD, "Parsed uuid=%s", uuid_str);
    if (plugins_action->context_action) {
        return plugins_action->context_action(user, &uuid);
    }

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_tombstone(char **words, void *user, PLUGINSD_ACTION *plugins_action)
{
    char *uuid_str = words[1];
    uuid_t uuid;

    if (unlikely(!uuid_str)) {
        error("requested a TOMBSTONE, without a uuid.");
        return PARSER_RC_ERROR;
    }
    if (unlikely(strlen(uuid_str) != GUID_LEN || uuid_parse(uuid_str, uuid) == -1)) {
        error("requested a TOMBSTONE, without a valid uuid string.");
        return PARSER_RC_ERROR;
    }

    debug(D_PLUGINSD, "Parsed uuid=%s", uuid_str);
    if (plugins_action->tombstone_action) {
        return plugins_action->tombstone_action(user, &uuid);
    }

    return PARSER_RC_OK;
}

PARSER_RC metalog_pluginsd_host(char **words, void *user, PLUGINSD_ACTION  *plugins_action)
{
    char *machine_guid = words[1];
    char *hostname = words[2];
    char *registry_hostname = words[3];
    char *update_every_s = words[4];
    char *os = words[5];
    char *timezone = words[6];
    char *tags = words[7];

    int update_every = 1;
    if (likely(update_every_s && *update_every_s))
        update_every = str2i(update_every_s);
    if (unlikely(!update_every))
        update_every = 1;

    debug(D_PLUGINSD, "HOST PARSED: guid=%s, hostname=%s, reg_host=%s, update=%d, os=%s, timezone=%s, tags=%s",
         machine_guid, hostname, registry_hostname, update_every, os, timezone, tags);

    if (plugins_action->host_action) {
        return plugins_action->host_action(
            user, machine_guid, hostname, registry_hostname, update_every, os, timezone, tags);
    }

    return PARSER_RC_OK;
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
    PARSER *parser = parser_init(host, &user, fp_plugin_output, fp_plugin_input, PARSER_INPUT_SPLIT);

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

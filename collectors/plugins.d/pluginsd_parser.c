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
        if(ssl->conn && ssl->flags == NETDATA_SSL_HANDSHAKE_COMPLETE)
            return (int)netdata_ssl_write(ssl->conn, (void *)txt, strlen(txt));

        error("PLUGINSD: cannot send command (SSL)");
        return -1;
    }
#endif

    if(parser->fp_output) {
        int bytes = fprintf(parser->fp_output, "%s", txt);
        if(bytes <= 0) {
            error("PLUGINSD: cannot send command (FILE)");
            return -2;
        }
        fflush(parser->fp_output);
        return bytes;
    }

    if(parser->fd != -1) {
        size_t bytes = 0;
        size_t total = strlen(txt);
        ssize_t sent;

        do {
            sent = write(parser->fd, &txt[bytes], total - bytes);
            if(sent <= 0) {
                error("PLUGINSD: cannot send command (fd)");
                return -3;
            }
            bytes += sent;
        }
        while(bytes < total);

        return (int)bytes;
    }

    error("PLUGINSD: cannot send command (no output socket/pipe/file given to plugins.d parser)");
    return -4;
}

static inline RRDHOST *pluginsd_require_host_from_parent(void *user, const char *cmd) {
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;

    if(unlikely(!host))
        error("PLUGINSD: command %s requires a host, but is not set.", cmd);

    return host;
}

static inline RRDSET *pluginsd_require_chart_from_parent(void *user, const char *cmd, const char *parent_cmd) {
    RRDSET *st = ((PARSER_USER_OBJECT *) user)->st;

    if(unlikely(!st))
        error("PLUGINSD: command %s requires a chart defined via command %s, but is not set.", cmd, parent_cmd);

    return st;
}

static inline RRDDIM_ACQUIRED *pluginsd_acquire_dimension(RRDHOST *host, RRDSET *st, const char *dimension, const char *cmd) {
    if (unlikely(!dimension || !*dimension)) {
        error("PLUGINSD: 'host:%s/chart:%s' got a %s, without a dimension.",
              rrdhost_hostname(host), rrdset_id(st), cmd);
        return NULL;
    }

    RRDDIM_ACQUIRED *rda = rrddim_find_and_acquire(st, dimension);

    if (unlikely(!rda))
        error("PLUGINSD: 'host:%s/chart:%s/dim:%s' got a %s but dimension does not exist.",
              rrdhost_hostname(host), rrdset_id(st), dimension, cmd);

    return rda;
}

static inline RRDSET *pluginsd_find_chart(RRDHOST *host, const char *chart, const char *cmd) {
    if (unlikely(!chart || !*chart)) {
        error("PLUGINSD: 'host:%s' got a %s without a chart id.",
              rrdhost_hostname(host), cmd);
        return NULL;
    }

    RRDSET *st = rrdset_find(host, chart);
    if (unlikely(!st))
        error("PLUGINSD: 'host:%s/chart:%s' got a %s but chart does not exist.",
              rrdhost_hostname(host), chart, cmd);

    return st;
}

static inline PARSER_RC PLUGINSD_DISABLE_PLUGIN(void *user) {
    ((PARSER_USER_OBJECT *) user)->enabled = 0;
    return PARSER_RC_ERROR;
}

PARSER_RC pluginsd_set(char **words, size_t num_words, void *user)
{
    char *dimension = get_word(words, num_words, 1);
    char *value = get_word(words, num_words, 2);

    RRDHOST *host = pluginsd_require_host_from_parent(user, PLUGINSD_KEYWORD_SET);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(user);

    RRDSET *st = pluginsd_require_chart_from_parent(user, PLUGINSD_KEYWORD_SET, PLUGINSD_KEYWORD_CHART);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(user);

    RRDDIM_ACQUIRED *rda = pluginsd_acquire_dimension(host, st, dimension, PLUGINSD_KEYWORD_SET);
    if(!rda) return PLUGINSD_DISABLE_PLUGIN(user);

    RRDDIM *rd = rrddim_acquired_to_rrddim(rda);

    if (unlikely(rrdset_flag_check(st, RRDSET_FLAG_DEBUG)))
        debug(D_PLUGINSD, "PLUGINSD: 'host:%s/chart:%s/dim:%s' SET is setting value to '%s'",
              rrdhost_hostname(host), rrdset_id(st), dimension, value && *value ? value : "UNSET");

    if (value && *value)
        rrddim_set_by_pointer(st, rd, strtoll(value, NULL, 0));

    rrddim_acquired_release(rda);
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_begin(char **words, size_t num_words, void *user)
{
    char *id = get_word(words, num_words, 1);
    char *microseconds_txt = get_word(words, num_words, 2);

    RRDHOST *host = pluginsd_require_host_from_parent(user, PLUGINSD_KEYWORD_BEGIN);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(user);

    RRDSET *st = pluginsd_find_chart(host, id, PLUGINSD_KEYWORD_BEGIN);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(user);

    ((PARSER_USER_OBJECT *)user)->st = st;

    usec_t microseconds = 0;
    if (microseconds_txt && *microseconds_txt)
        microseconds = str2ull(microseconds_txt);

#ifdef NETDATA_LOG_REPLICATION_REQUESTS
    if(st->replay.log_next_data_collection) {
        st->replay.log_next_data_collection = false;

        internal_error(true,
                       "REPLAY: 'host:%s/chart:%s' first BEGIN after replication, last collected %llu, last updated %llu, microseconds %llu",
                       rrdhost_hostname(host), rrdset_id(st),
                       st->last_collected_time.tv_sec * USEC_PER_SEC + st->last_collected_time.tv_usec,
                       st->last_updated.tv_sec * USEC_PER_SEC + st->last_updated.tv_usec,
                       microseconds
                       );
    }
#endif

    if (likely(st->counter_done)) {
        if (likely(microseconds)) {
            if (((PARSER_USER_OBJECT *)user)->trust_durations)
                rrdset_next_usec_unfiltered(st, microseconds);
            else
                rrdset_next_usec(st, microseconds);
        }
        else
            rrdset_next(st);
    }
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_end(char **words, size_t num_words, void *user)
{
    UNUSED(words);
    UNUSED(num_words);

    RRDHOST *host = pluginsd_require_host_from_parent(user, PLUGINSD_KEYWORD_END);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(user);

    RRDSET *st = pluginsd_require_chart_from_parent(user, PLUGINSD_KEYWORD_END, PLUGINSD_KEYWORD_BEGIN);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(user);

    if (unlikely(rrdset_flag_check(st, RRDSET_FLAG_DEBUG)))
        debug(D_PLUGINSD, "requested an END on chart '%s'", rrdset_id(st));

    ((PARSER_USER_OBJECT *) user)->st = NULL;
    ((PARSER_USER_OBJECT *) user)->count++;

    struct timeval now;
    now_realtime_timeval(&now);
    rrdset_timed_done(st, now, /* pending_rrdset_next = */ false);

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_chart(char **words, size_t num_words, void *user)
{
    RRDHOST *host = pluginsd_require_host_from_parent(user, PLUGINSD_KEYWORD_CHART);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(user);

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
        error("PLUGINSD: 'host:%s' requested a CHART, without a type.id. Disabling it.",
              rrdhost_hostname(host));

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
    const char *world_time_txt = get_word(words, num_words, 3);

    RRDHOST *host = pluginsd_require_host_from_parent(user, PLUGINSD_KEYWORD_CHART_DEFINITION_END);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(user);

    RRDSET *st = pluginsd_require_chart_from_parent(user, PLUGINSD_KEYWORD_CHART_DEFINITION_END, PLUGINSD_KEYWORD_CHART);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(user);

    time_t first_entry_child = (first_entry_txt && *first_entry_txt) ? (time_t)str2ul(first_entry_txt) : 0;
    time_t last_entry_child = (last_entry_txt && *last_entry_txt) ? (time_t)str2ul(last_entry_txt) : 0;
    time_t child_world_time = (world_time_txt && *world_time_txt) ? (time_t)str2ul(world_time_txt) : now_realtime_sec();

    if((first_entry_child != 0 || last_entry_child != 0) && (first_entry_child == 0 || last_entry_child == 0))
        error("PLUGINSD REPLAY ERROR: 'host:%s/chart:%s' got a " PLUGINSD_KEYWORD_CHART_DEFINITION_END " with malformed timings (first time %ld, last time %ld, world time %ld).",
              rrdhost_hostname(host), rrdset_id(st),
              first_entry_child, last_entry_child, child_world_time);

    bool ok = true;
    if(!rrdset_flag_check(st, RRDSET_FLAG_RECEIVER_REPLICATION_IN_PROGRESS)) {

#ifdef NETDATA_LOG_REPLICATION_REQUESTS
        st->replay.start_streaming = false;
        st->replay.after = 0;
        st->replay.before = 0;
#endif

        rrdset_flag_set(st, RRDSET_FLAG_RECEIVER_REPLICATION_IN_PROGRESS);
        rrdset_flag_clear(st, RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED);
        rrdhost_receiver_replicating_charts_plus_one(st->rrdhost);

        PARSER *parser = ((PARSER_USER_OBJECT *)user)->parser;
        ok = replicate_chart_request(send_to_plugin, parser, host, st,
                                     first_entry_child, last_entry_child, child_world_time,
                                     0, 0);
    }
#ifdef NETDATA_LOG_REPLICATION_REQUESTS
    else {
        internal_error(true, "REPLAY: 'host:%s/chart:%s' not sending duplicate replication request",
                       rrdhost_hostname(st->rrdhost), rrdset_id(st));
    }
#endif

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

    RRDHOST *host = pluginsd_require_host_from_parent(user, PLUGINSD_KEYWORD_DIMENSION);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(user);

    RRDSET *st = pluginsd_require_chart_from_parent(user, PLUGINSD_KEYWORD_DIMENSION, PLUGINSD_KEYWORD_CHART);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(user);

    if (unlikely(!id)) {
        error("PLUGINSD: 'host:%s/chart:%s' got a DIMENSION, without an id. Disabling it.",
              rrdhost_hostname(host), st ? rrdset_id(st) : "UNSET");
        return PLUGINSD_DISABLE_PLUGIN(user);
    }

    if (unlikely(!st && !((PARSER_USER_OBJECT *) user)->st_exists)) {
        error("PLUGINSD: 'host:%s' got a DIMENSION, without a CHART. Disabling it.",
              rrdhost_hostname(host));
        return PLUGINSD_DISABLE_PLUGIN(user);
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
    }
    else {
        rrddim_option_set(rd, RRDDIM_OPTION_HIDDEN);
        if (!rrddim_flag_check(rd, RRDDIM_FLAG_META_HIDDEN)) {
            rrddim_flag_set(rd, RRDDIM_FLAG_META_HIDDEN);
            metaqueue_dimension_update_flags(rd);
        }
    }

    return PARSER_RC_OK;
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
        error("FUNCTION: failed to send function to plugin, error %d", ret);
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

    RRDHOST *host = pluginsd_require_host_from_parent(user, PLUGINSD_KEYWORD_FUNCTION);
    if(!host) return PARSER_RC_ERROR;

    RRDSET *st = (global)?NULL:pluginsd_require_chart_from_parent(user, PLUGINSD_KEYWORD_FUNCTION, PLUGINSD_KEYWORD_CHART);
    if(!st) global = true;

    if (unlikely(!timeout_s || !name || !help || (!global && !st))) {
        error("PLUGINSD: 'host:%s/chart:%s' got a FUNCTION, without providing the required data (global = '%s', name = '%s', timeout = '%s', help = '%s'). Ignoring it.",
              rrdhost_hostname(host),
              st?rrdset_id(st):"(unset)",
              global?"yes":"no",
              name?name:"(unset)",
              timeout_s?timeout_s:"(unset)",
              help?help:"(unset)"
              );
        return PARSER_RC_ERROR;
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

    RRDHOST *host = pluginsd_require_host_from_parent(user, PLUGINSD_KEYWORD_VARIABLE);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(user);

    RRDSET *st = ((PARSER_USER_OBJECT *) user)->st;

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
        error("PLUGINSD: 'host:%s/chart:%s' got a VARIABLE without a variable name. Disabling it.",
              rrdhost_hostname(host), st ? rrdset_id(st):"UNSET");

        ((PARSER_USER_OBJECT *)user)->enabled = 0;
        return PLUGINSD_DISABLE_PLUGIN(user);
    }

    if (unlikely(!value || !*value))
        value = NULL;

    if (unlikely(!value)) {
        error("PLUGINSD: 'host:%s/chart:%s' cannot set %s VARIABLE '%s' to an empty value",
              rrdhost_hostname(host),
              st ? rrdset_id(st):"UNSET",
              (global) ? "HOST" : "CHART",
              name);
        return PARSER_RC_OK;
    }

    if (!global && !st) {
        error("PLUGINSD: 'host:%s/chart:%s' cannot update CHART VARIABLE '%s' without a chart",
              rrdhost_hostname(host),
              st ? rrdset_id(st):"UNSET",
              name
              );
        return PLUGINSD_DISABLE_PLUGIN(user);
    }

    char *endptr = NULL;
    v = (NETDATA_DOUBLE)str2ndd(value, &endptr);
    if (unlikely(endptr && *endptr)) {
        if (endptr == value)
            error("PLUGINSD: 'host:%s/chart:%s' the value '%s' of VARIABLE '%s' cannot be parsed as a number",
                  rrdhost_hostname(host),
                  st ? rrdset_id(st):"UNSET",
                  value,
                  name);
        else
            error("PLUGINSD: 'host:%s/chart:%s' the value '%s' of VARIABLE '%s' has leftovers: '%s'",
                  rrdhost_hostname(host),
                  st ? rrdset_id(st):"UNSET",
                  value,
                  name,
                  endptr);
    }

    if (global) {
        const RRDVAR_ACQUIRED *rva = rrdvar_custom_host_variable_add_and_acquire(host, name);
        if (rva) {
            rrdvar_custom_host_variable_set(host, rva, v);
            rrdvar_custom_host_variable_release(host, rva);
        }
        else
            error("PLUGINSD: 'host:%s' cannot find/create HOST VARIABLE '%s'",
                  rrdhost_hostname(host),
                  name);
    } else {
        const RRDSETVAR_ACQUIRED *rsa = rrdsetvar_custom_chart_variable_add_and_acquire(st, name);
        if (rsa) {
            rrdsetvar_custom_chart_variable_set(st, rsa, v);
            rrdsetvar_custom_chart_variable_release(st, rsa);
        }
        else
            error("PLUGINSD: 'host:%s/chart:%s' cannot find/create CHART VARIABLE '%s'",
                  rrdhost_hostname(host), rrdset_id(st), name);
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
    info("PLUGINSD: plugin called DISABLE. Disabling it.");
    ((PARSER_USER_OBJECT *) user)->enabled = 0;
    return PARSER_RC_ERROR;
}

PARSER_RC pluginsd_label(char **words, size_t num_words, void *user)
{
    const char *name = get_word(words, num_words, 1);
    const char *label_source = get_word(words, num_words, 2);
    const char *value = get_word(words, num_words, 3);

    if (!name || !label_source || !value) {
        error("PLUGINSD: ignoring malformed or empty LABEL command.");
        return PLUGINSD_DISABLE_PLUGIN(user);
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
    RRDHOST *host = pluginsd_require_host_from_parent(user, PLUGINSD_KEYWORD_OVERWRITE);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(user);

    debug(D_PLUGINSD, "requested to OVERWRITE host labels");

    if(unlikely(!host->rrdlabels))
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
        return PLUGINSD_DISABLE_PLUGIN(user);
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
    RRDHOST *host = pluginsd_require_host_from_parent(user, PLUGINSD_KEYWORD_CLABEL_COMMIT);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(user);

    RRDSET *st = pluginsd_require_chart_from_parent(user, PLUGINSD_KEYWORD_CLABEL_COMMIT, PLUGINSD_KEYWORD_BEGIN);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(user);

    debug(D_PLUGINSD, "requested to commit chart labels");

    if(!((PARSER_USER_OBJECT *)user)->chart_rrdlabels_linked_temporarily) {
        error("PLUGINSD: 'host:%s' got CLABEL_COMMIT, without a CHART or BEGIN. Ignoring it.",
              rrdhost_hostname(host));
        return PLUGINSD_DISABLE_PLUGIN(user);
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
    char *child_now_str = get_word(words, num_words, 4);

    RRDHOST *host = pluginsd_require_host_from_parent(user, PLUGINSD_KEYWORD_REPLAY_BEGIN);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(user);

    RRDSET *st;
    if (likely(!id || !*id))
        st = pluginsd_require_chart_from_parent(user, PLUGINSD_KEYWORD_REPLAY_BEGIN, PLUGINSD_KEYWORD_REPLAY_BEGIN);
    else
        st = pluginsd_find_chart(host, id, PLUGINSD_KEYWORD_REPLAY_BEGIN);

    if(!st) return PLUGINSD_DISABLE_PLUGIN(user);
    ((PARSER_USER_OBJECT *) user)->st = st;

    if(start_time_str && end_time_str) {
        time_t start_time = (time_t)str2ul(start_time_str);
        time_t end_time = (time_t)str2ul(end_time_str);

        time_t wall_clock_time = 0, tolerance;
        bool wall_clock_comes_from_child; (void)wall_clock_comes_from_child;
        if(child_now_str) {
            wall_clock_time = (time_t)str2ul(child_now_str);
            tolerance = st->update_every + 1;
            wall_clock_comes_from_child = true;
        }

        if(wall_clock_time <= 0) {
            wall_clock_time = now_realtime_sec();
            tolerance = st->update_every + 5;
            wall_clock_comes_from_child = false;
        }

#ifdef NETDATA_LOG_REPLICATION_REQUESTS
        internal_error(
                (!st->replay.start_streaming && (end_time < st->replay.after || start_time > st->replay.before)),
                "REPLAY ERROR: 'host:%s/chart:%s' got a " PLUGINSD_KEYWORD_REPLAY_BEGIN " from %ld to %ld, which does not match our request (%ld to %ld).",
                rrdhost_hostname(st->rrdhost), rrdset_id(st), start_time, end_time, st->replay.after, st->replay.before);

        internal_error(
                true,
                "REPLAY: 'host:%s/chart:%s' got a " PLUGINSD_KEYWORD_REPLAY_BEGIN " from %ld to %ld, child wall clock is %ld (%s), had requested %ld to %ld",
                rrdhost_hostname(st->rrdhost), rrdset_id(st),
                start_time, end_time, wall_clock_time, wall_clock_comes_from_child ? "from child" : "parent time",
                st->replay.after, st->replay.before);
#endif

        if(start_time && end_time && start_time < wall_clock_time + tolerance && end_time < wall_clock_time + tolerance && start_time < end_time) {
            if (unlikely(end_time - start_time != st->update_every))
                rrdset_set_update_every(st, end_time - start_time);

            st->last_collected_time.tv_sec = end_time;
            st->last_collected_time.tv_usec = 0;

            st->last_updated.tv_sec = end_time;
            st->last_updated.tv_usec = 0;

            st->counter++;
            st->counter_done++;

            // these are only needed for db mode RAM, SAVE, MAP, ALLOC
            st->current_entry++;
            if(st->current_entry >= st->entries)
                st->current_entry -= st->entries;

            ((PARSER_USER_OBJECT *) user)->replay.start_time = start_time;
            ((PARSER_USER_OBJECT *) user)->replay.end_time = end_time;
            ((PARSER_USER_OBJECT *) user)->replay.start_time_ut = (usec_t) start_time * USEC_PER_SEC;
            ((PARSER_USER_OBJECT *) user)->replay.end_time_ut = (usec_t) end_time * USEC_PER_SEC;
            ((PARSER_USER_OBJECT *) user)->replay.wall_clock_time = wall_clock_time;
            ((PARSER_USER_OBJECT *) user)->replay.rset_enabled = true;

            return PARSER_RC_OK;
        }

        error("PLUGINSD REPLAY ERROR: 'host:%s/chart:%s' got a " PLUGINSD_KEYWORD_REPLAY_BEGIN " from %ld to %ld, but timestamps are invalid (now is %ld [%s], tolerance %ld). Ignoring " PLUGINSD_KEYWORD_REPLAY_SET,
              rrdhost_hostname(st->rrdhost), rrdset_id(st), start_time, end_time,
              wall_clock_time, wall_clock_comes_from_child ? "child wall clock" : "parent wall clock", tolerance);
    }

    // the child sends an RBEGIN without any parameters initially
    // setting rset_enabled to false, means the RSET should not store any metrics
    // to store metrics, the RBEGIN needs to have timestamps
    ((PARSER_USER_OBJECT *) user)->replay.start_time = 0;
    ((PARSER_USER_OBJECT *) user)->replay.end_time = 0;
    ((PARSER_USER_OBJECT *) user)->replay.start_time_ut = 0;
    ((PARSER_USER_OBJECT *) user)->replay.end_time_ut = 0;
    ((PARSER_USER_OBJECT *) user)->replay.wall_clock_time = 0;
    ((PARSER_USER_OBJECT *) user)->replay.rset_enabled = false;
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_replay_set(char **words, size_t num_words, void *user)
{
    char *dimension = get_word(words, num_words, 1);
    char *value_str = get_word(words, num_words, 2);
    char *flags_str = get_word(words, num_words, 3);

    RRDHOST *host = pluginsd_require_host_from_parent(user, PLUGINSD_KEYWORD_REPLAY_SET);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(user);

    RRDSET *st = pluginsd_require_chart_from_parent(user, PLUGINSD_KEYWORD_REPLAY_SET, PLUGINSD_KEYWORD_REPLAY_BEGIN);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(user);

    if(!((PARSER_USER_OBJECT *) user)->replay.rset_enabled) {
        error_limit_static_thread_var(erl, 1, 0);
        error_limit(&erl, "PLUGINSD: 'host:%s/chart:%s' got a " PLUGINSD_KEYWORD_REPLAY_SET " but it is disabled by " PLUGINSD_KEYWORD_REPLAY_BEGIN " errors",
                    rrdhost_hostname(host), rrdset_id(st));

        // we have to return OK here
        return PARSER_RC_OK;
    }

    RRDDIM_ACQUIRED *rda = pluginsd_acquire_dimension(host, st, dimension, PLUGINSD_KEYWORD_REPLAY_SET);
    if(!rda) return PLUGINSD_DISABLE_PLUGIN(user);

    if (unlikely(!((PARSER_USER_OBJECT *) user)->replay.start_time || !((PARSER_USER_OBJECT *) user)->replay.end_time)) {
        error("PLUGINSD: 'host:%s/chart:%s/dim:%s' got a " PLUGINSD_KEYWORD_REPLAY_SET " with invalid timestamps %ld to %ld from a " PLUGINSD_KEYWORD_REPLAY_BEGIN ". Disabling it.",
              rrdhost_hostname(host),
              rrdset_id(st),
              dimension,
              ((PARSER_USER_OBJECT *) user)->replay.start_time,
              ((PARSER_USER_OBJECT *) user)->replay.end_time);
        return PLUGINSD_DISABLE_PLUGIN(user);
    }

    if (unlikely(!value_str || !*value_str))
        value_str = "NAN";

    if(unlikely(!flags_str))
        flags_str = "";

    if (likely(value_str)) {
        RRDDIM *rd = rrddim_acquired_to_rrddim(rda);

        RRDDIM_FLAGS rd_flags = rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE | RRDDIM_FLAG_ARCHIVED);

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
        else {
            error_limit_static_global_var(erl, 1, 0);
            error_limit(&erl, "PLUGINSD: 'host:%s/chart:%s/dim:%s' has the ARCHIVED flag set, but it is replicated. Ignoring data.",
                        rrdhost_hostname(st->rrdhost), rrdset_id(st), rrddim_name(rd));
        }
    }

    rrddim_acquired_release(rda);
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_replay_rrddim_collection_state(char **words, size_t num_words, void *user)
{
    char *dimension = get_word(words, num_words, 1);
    char *last_collected_ut_str = get_word(words, num_words, 2);
    char *last_collected_value_str = get_word(words, num_words, 3);
    char *last_calculated_value_str = get_word(words, num_words, 4);
    char *last_stored_value_str = get_word(words, num_words, 5);

    RRDHOST *host = pluginsd_require_host_from_parent(user, PLUGINSD_KEYWORD_REPLAY_RRDDIM_STATE);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(user);

    RRDSET *st = pluginsd_require_chart_from_parent(user, PLUGINSD_KEYWORD_REPLAY_RRDDIM_STATE, PLUGINSD_KEYWORD_REPLAY_BEGIN);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(user);

    RRDDIM_ACQUIRED *rda = pluginsd_acquire_dimension(host, st, dimension, PLUGINSD_KEYWORD_REPLAY_RRDDIM_STATE);
    if(!rda) return PLUGINSD_DISABLE_PLUGIN(user);

    RRDDIM *rd = rrddim_acquired_to_rrddim(rda);
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
}

PARSER_RC pluginsd_replay_rrdset_collection_state(char **words, size_t num_words, void *user)
{
    char *last_collected_ut_str = get_word(words, num_words, 1);
    char *last_updated_ut_str = get_word(words, num_words, 2);

    RRDHOST *host = pluginsd_require_host_from_parent(user, PLUGINSD_KEYWORD_REPLAY_RRDSET_STATE);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(user);

    RRDSET *st = pluginsd_require_chart_from_parent(user, PLUGINSD_KEYWORD_REPLAY_RRDSET_STATE, PLUGINSD_KEYWORD_REPLAY_BEGIN);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(user);

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
}

PARSER_RC pluginsd_replay_end(char **words, size_t num_words, void *user)
{
    if (num_words < 7) { // accepts 7, but the 7th is optional
        error("REPLAY: malformed " PLUGINSD_KEYWORD_REPLAY_END " command");
        return PARSER_RC_ERROR;
    }

    const char *update_every_child_txt = get_word(words, num_words, 1);
    const char *first_entry_child_txt = get_word(words, num_words, 2);
    const char *last_entry_child_txt = get_word(words, num_words, 3);
    const char *start_streaming_txt = get_word(words, num_words, 4);
    const char *first_entry_requested_txt = get_word(words, num_words, 5);
    const char *last_entry_requested_txt = get_word(words, num_words, 6);
    const char *child_world_time_txt = get_word(words, num_words, 7); // optional

    time_t update_every_child = (time_t)str2ul(update_every_child_txt);
    time_t first_entry_child = (time_t)str2ul(first_entry_child_txt);
    time_t last_entry_child = (time_t)str2ul(last_entry_child_txt);

    bool start_streaming = (strcmp(start_streaming_txt, "true") == 0);
    time_t first_entry_requested = (time_t)str2ul(first_entry_requested_txt);
    time_t last_entry_requested = (time_t)str2ul(last_entry_requested_txt);

    // the optional child world time
    time_t child_world_time = (child_world_time_txt && *child_world_time_txt) ? (time_t)str2ul(child_world_time_txt) : now_realtime_sec();

    PARSER_USER_OBJECT *user_object = user;

    RRDHOST *host = pluginsd_require_host_from_parent(user, PLUGINSD_KEYWORD_REPLAY_END);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(user);

    RRDSET *st = pluginsd_require_chart_from_parent(user, PLUGINSD_KEYWORD_REPLAY_END, PLUGINSD_KEYWORD_REPLAY_BEGIN);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(user);

#ifdef NETDATA_LOG_REPLICATION_REQUESTS
    internal_error(true,
                   "PLUGINSD REPLAY: 'host:%s/chart:%s': got a " PLUGINSD_KEYWORD_REPLAY_END " child db from %llu to %llu, start_streaming %s, had requested from %llu to %llu, wall clock %llu",
                   rrdhost_hostname(host), rrdset_id(st),
                   (unsigned long long)first_entry_child, (unsigned long long)last_entry_child,
                   start_streaming?"true":"false",
                   (unsigned long long)first_entry_requested, (unsigned long long)last_entry_requested,
                   (unsigned long long)child_world_time
                   );
#endif

    ((PARSER_USER_OBJECT *) user)->st = NULL;
    ((PARSER_USER_OBJECT *) user)->count++;

    if(((PARSER_USER_OBJECT *) user)->replay.rset_enabled && st->rrdhost->receiver) {
        time_t now = now_realtime_sec();
        time_t started = st->rrdhost->receiver->replication_first_time_t;
        time_t current = ((PARSER_USER_OBJECT *) user)->replay.end_time;

        worker_set_metric(WORKER_RECEIVER_JOB_REPLICATION_COMPLETION,
                          (NETDATA_DOUBLE)(current - started) * 100.0 / (NETDATA_DOUBLE)(now - started));
    }

    ((PARSER_USER_OBJECT *) user)->replay.start_time = 0;
    ((PARSER_USER_OBJECT *) user)->replay.end_time = 0;
    ((PARSER_USER_OBJECT *) user)->replay.start_time_ut = 0;
    ((PARSER_USER_OBJECT *) user)->replay.end_time_ut = 0;
    ((PARSER_USER_OBJECT *) user)->replay.wall_clock_time = 0;
    ((PARSER_USER_OBJECT *) user)->replay.rset_enabled = false;

    st->counter++;
    st->counter_done++;

#ifdef NETDATA_LOG_REPLICATION_REQUESTS
    st->replay.start_streaming = false;
    st->replay.after = 0;
    st->replay.before = 0;
    if(start_streaming)
        st->replay.log_next_data_collection = true;
#endif

    if (start_streaming) {
        if (st->update_every != update_every_child)
            rrdset_set_update_every(st, update_every_child);

        if(rrdset_flag_check(st, RRDSET_FLAG_RECEIVER_REPLICATION_IN_PROGRESS)) {
            rrdset_flag_set(st, RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED);
            rrdset_flag_clear(st, RRDSET_FLAG_RECEIVER_REPLICATION_IN_PROGRESS);
            rrdset_flag_clear(st, RRDSET_FLAG_SYNC_CLOCK);
            rrdhost_receiver_replicating_charts_minus_one(st->rrdhost);
        }
#ifdef NETDATA_LOG_REPLICATION_REQUESTS
        else
            internal_error(true, "REPLAY ERROR: 'host:%s/chart:%s' got a " PLUGINSD_KEYWORD_REPLAY_END " with enable_streaming = true, but there is no replication in progress for this chart.",
                  rrdhost_hostname(host), rrdset_id(st));
#endif
        worker_set_metric(WORKER_RECEIVER_JOB_REPLICATION_COMPLETION, 100.0);

        return PARSER_RC_OK;
    }

    rrdcontext_updated_retention_rrdset(st);

    bool ok = replicate_chart_request(send_to_plugin, user_object->parser, host, st,
                                      first_entry_child, last_entry_child, child_world_time,
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
    PARSER *parser = parser_init(host, &user, fp_plugin_output, fp_plugin_input, -1, PARSER_INPUT_SPLIT, NULL);

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

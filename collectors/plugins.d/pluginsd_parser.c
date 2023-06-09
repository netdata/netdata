// SPDX-License-Identifier: GPL-3.0-or-later

#include "pluginsd_parser.h"

#define LOG_FUNCTIONS false

static int send_to_plugin(const char *txt, void *data) {
    PARSER *parser = data;

    if(!txt || !*txt)
        return 0;

#ifdef ENABLE_HTTPS
    NETDATA_SSL *ssl = parser->ssl_output;
    if(ssl) {
        if(SSL_connection(ssl))
            return (int)netdata_ssl_write(ssl, (void *)txt, strlen(txt));

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

static inline RRDSET *pluginsd_get_chart_from_parent(void *user) {
    return ((PARSER_USER_OBJECT *) user)->st;
}

static inline void pluginsd_lock_rrdset_data_collection(void *user) {
    PARSER_USER_OBJECT *u = (PARSER_USER_OBJECT *) user;
    if(u->st && !u->v2.locked_data_collection) {
        netdata_spinlock_lock(&u->st->data_collection_lock);
        u->v2.locked_data_collection = true;
    }
}

static inline bool pluginsd_unlock_rrdset_data_collection(void *user) {
    PARSER_USER_OBJECT *u = (PARSER_USER_OBJECT *) user;
    if(u->st && u->v2.locked_data_collection) {
        netdata_spinlock_unlock(&u->st->data_collection_lock);
        u->v2.locked_data_collection = false;
        return true;
    }

    return false;
}

void pluginsd_rrdset_cleanup(RRDSET *st) {
    for(size_t i = 0; i < st->pluginsd.used ; i++) {
        if (st->pluginsd.rda[i]) {
            rrddim_acquired_release(st->pluginsd.rda[i]);
            st->pluginsd.rda[i] = NULL;
        }
    }
    freez(st->pluginsd.rda);
    st->pluginsd.rda = NULL;
    st->pluginsd.size = 0;
    st->pluginsd.used = 0;
    st->pluginsd.pos = 0;
}

static inline void pluginsd_unlock_previous_chart(void *user, const char *keyword, bool stale) {
    PARSER_USER_OBJECT *u = (PARSER_USER_OBJECT *) user;

    if(unlikely(pluginsd_unlock_rrdset_data_collection(user))) {
        if(stale)
            error("PLUGINSD: 'host:%s/chart:%s/' stale data collection lock found during %s; it has been unlocked",
              rrdhost_hostname(u->st->rrdhost), rrdset_id(u->st), keyword);
    }

    if(unlikely(u->v2.ml_locked)) {
        ml_chart_update_end(u->st);
        u->v2.ml_locked = false;

        if(stale)
            error("PLUGINSD: 'host:%s/chart:%s/' stale ML lock found during %s, it has been unlocked",
              rrdhost_hostname(u->st->rrdhost), rrdset_id(u->st), keyword);
    }
}

static inline void pluginsd_set_chart_from_parent(void *user, RRDSET *st, const char *keyword) {
    PARSER_USER_OBJECT *u = (PARSER_USER_OBJECT *) user;

    pluginsd_unlock_previous_chart(user, keyword, true);

    if(st) {
        size_t dims = dictionary_entries(st->rrddim_root_index);
        if(unlikely(st->pluginsd.size < dims)) {
            st->pluginsd.rda = reallocz(st->pluginsd.rda, dims * sizeof(RRDDIM_ACQUIRED *));
            st->pluginsd.size = dims;
        }

        if(st->pluginsd.pos > st->pluginsd.used && st->pluginsd.pos <= st->pluginsd.size)
            st->pluginsd.used = st->pluginsd.pos;

        st->pluginsd.pos = 0;
    }

    u->st = st;
}

static inline RRDDIM *pluginsd_acquire_dimension(RRDHOST *host, RRDSET *st, const char *dimension, const char *cmd) {
    if (unlikely(!dimension || !*dimension)) {
        error("PLUGINSD: 'host:%s/chart:%s' got a %s, without a dimension.",
              rrdhost_hostname(host), rrdset_id(st), cmd);
        return NULL;
    }

    RRDDIM_ACQUIRED *rda;

    if(likely(st->pluginsd.pos < st->pluginsd.used)) {
        rda = st->pluginsd.rda[st->pluginsd.pos];
        RRDDIM *rd = rrddim_acquired_to_rrddim(rda);
        if (likely(rd && string_strcmp(rd->id, dimension) == 0)) {
            st->pluginsd.pos++;
            return rd;
        }
        else {
            rrddim_acquired_release(rda);
            st->pluginsd.rda[st->pluginsd.pos] = NULL;
        }
    }

    rda = rrddim_find_and_acquire(st, dimension);
    if (unlikely(!rda)) {
        error("PLUGINSD: 'host:%s/chart:%s/dim:%s' got a %s but dimension does not exist.",
              rrdhost_hostname(host), rrdset_id(st), dimension, cmd);

        return NULL;
    }

    if(likely(st->pluginsd.pos < st->pluginsd.size))
        st->pluginsd.rda[st->pluginsd.pos++] = rda;

    return rrddim_acquired_to_rrddim(rda);
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

static inline PARSER_RC PLUGINSD_DISABLE_PLUGIN(void *user, const char *keyword, const char *msg) {
    ((PARSER_USER_OBJECT *) user)->enabled = 0;

    if(keyword && msg) {
        error_limit_static_global_var(erl, 1, 0);
        error_limit(&erl, "PLUGINSD: keyword %s: %s", keyword, msg);
    }

    return PARSER_RC_ERROR;
}

PARSER_RC pluginsd_set(char **words, size_t num_words, void *user)
{
    char *dimension = get_word(words, num_words, 1);
    char *value = get_word(words, num_words, 2);

    RRDHOST *host = pluginsd_require_host_from_parent(user, PLUGINSD_KEYWORD_SET);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    RRDSET *st = pluginsd_require_chart_from_parent(user, PLUGINSD_KEYWORD_SET, PLUGINSD_KEYWORD_CHART);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    RRDDIM *rd = pluginsd_acquire_dimension(host, st, dimension, PLUGINSD_KEYWORD_SET);
    if(!rd) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    if (unlikely(rrdset_flag_check(st, RRDSET_FLAG_DEBUG)))
        debug(D_PLUGINSD, "PLUGINSD: 'host:%s/chart:%s/dim:%s' SET is setting value to '%s'",
              rrdhost_hostname(host), rrdset_id(st), dimension, value && *value ? value : "UNSET");

    if (value && *value)
        rrddim_set_by_pointer(st, rd, str2ll_encoded(value));

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_begin(char **words, size_t num_words, void *user)
{
    char *id = get_word(words, num_words, 1);
    char *microseconds_txt = get_word(words, num_words, 2);

    RRDHOST *host = pluginsd_require_host_from_parent(user, PLUGINSD_KEYWORD_BEGIN);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    RRDSET *st = pluginsd_find_chart(host, id, PLUGINSD_KEYWORD_BEGIN);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    pluginsd_set_chart_from_parent(user, st, PLUGINSD_KEYWORD_BEGIN);

    usec_t microseconds = 0;
    if (microseconds_txt && *microseconds_txt) {
        long long t = str2ll(microseconds_txt, NULL);
        if(t >= 0)
            microseconds = t;
    }

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
    if(!host) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    RRDSET *st = pluginsd_require_chart_from_parent(user, PLUGINSD_KEYWORD_END, PLUGINSD_KEYWORD_BEGIN);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    if (unlikely(rrdset_flag_check(st, RRDSET_FLAG_DEBUG)))
        debug(D_PLUGINSD, "requested an END on chart '%s'", rrdset_id(st));

    pluginsd_set_chart_from_parent(user, NULL, PLUGINSD_KEYWORD_END);
    ((PARSER_USER_OBJECT *) user)->data_collections_count++;

    struct timeval now;
    now_realtime_timeval(&now);
    rrdset_timed_done(st, now, /* pending_rrdset_next = */ false);

    return PARSER_RC_OK;
}

static void pluginsd_host_define_cleanup(void *user) {
    PARSER_USER_OBJECT *u = user;

    string_freez(u->host_define.hostname);
    dictionary_destroy(u->host_define.rrdlabels);

    u->host_define.hostname = NULL;
    u->host_define.rrdlabels = NULL;
    u->host_define.parsing_host = false;
}

static inline bool pluginsd_validate_machine_guid(const char *guid, uuid_t *uuid, char *output) {
    if(uuid_parse(guid, *uuid))
        return false;

    uuid_unparse_lower(*uuid, output);

    return true;
}

static PARSER_RC pluginsd_host_define(char **words, size_t num_words, void *user) {
    PARSER_USER_OBJECT *u = user;

    char *guid = get_word(words, num_words, 1);
    char *hostname = get_word(words, num_words, 2);

    if(unlikely(!guid || !*guid || !hostname || !*hostname))
        return PLUGINSD_DISABLE_PLUGIN(user, PLUGINSD_KEYWORD_HOST_DEFINE, "missing parameters");

    if(unlikely(u->host_define.parsing_host))
        return PLUGINSD_DISABLE_PLUGIN(user, PLUGINSD_KEYWORD_HOST_DEFINE,
            "another host definition is already open - did you send " PLUGINSD_KEYWORD_HOST_DEFINE_END "?");

    if(!pluginsd_validate_machine_guid(guid, &u->host_define.machine_guid, u->host_define.machine_guid_str))
        return PLUGINSD_DISABLE_PLUGIN(user, PLUGINSD_KEYWORD_HOST_DEFINE, "cannot parse MACHINE_GUID - is it a valid UUID?");

    u->host_define.hostname = string_strdupz(hostname);
    u->host_define.rrdlabels = rrdlabels_create();
    u->host_define.parsing_host = true;

    return PARSER_RC_OK;
}

static inline PARSER_RC pluginsd_host_dictionary(char **words, size_t num_words, void *user, DICTIONARY *dict, const char *keyword) {
    PARSER_USER_OBJECT *u = user;

    char *name = get_word(words, num_words, 1);
    char *value = get_word(words, num_words, 2);

    if(!name || !*name || !value)
        return PLUGINSD_DISABLE_PLUGIN(user, keyword, "missing parameters");

    if(!u->host_define.parsing_host || !dict)
        return PLUGINSD_DISABLE_PLUGIN(user, keyword, "host is not defined, send " PLUGINSD_KEYWORD_HOST_DEFINE " before this");

    rrdlabels_add(dict, name, value, RRDLABEL_SRC_CONFIG);

    return PARSER_RC_OK;
}

static PARSER_RC pluginsd_host_labels(char **words, size_t num_words, void *user) {
    PARSER_USER_OBJECT *u = user;
    return pluginsd_host_dictionary(words, num_words, user, u->host_define.rrdlabels, PLUGINSD_KEYWORD_HOST_LABEL);
}

static PARSER_RC pluginsd_host_define_end(char **words __maybe_unused, size_t num_words __maybe_unused, void *user) {
    PARSER_USER_OBJECT *u = user;

    if(!u->host_define.parsing_host)
        return PLUGINSD_DISABLE_PLUGIN(user, PLUGINSD_KEYWORD_HOST_DEFINE_END, "missing initialization, send " PLUGINSD_KEYWORD_HOST_DEFINE " before this");

    RRDHOST *host = rrdhost_find_or_create(
            string2str(u->host_define.hostname),
            string2str(u->host_define.hostname),
            u->host_define.machine_guid_str,
            "Netdata Virtual Host 1.0",
            netdata_configured_timezone,
            netdata_configured_abbrev_timezone,
            netdata_configured_utc_offset,
            NULL,
            program_name,
            program_version,
            default_rrd_update_every,
            default_rrd_history_entries,
            default_rrd_memory_mode,
            default_health_enabled,
            default_rrdpush_enabled,
            default_rrdpush_destination,
            default_rrdpush_api_key,
            default_rrdpush_send_charts_matching,
            default_rrdpush_enable_replication,
            default_rrdpush_seconds_to_replicate,
            default_rrdpush_replication_step,
            rrdhost_labels_to_system_info(u->host_define.rrdlabels),
            false
            );

    if(host->rrdlabels) {
        rrdlabels_migrate_to_these(host->rrdlabels, u->host_define.rrdlabels);
    }
    else {
        host->rrdlabels = u->host_define.rrdlabels;
        u->host_define.rrdlabels = NULL;
    }

    pluginsd_host_define_cleanup(user);

    u->host = host;
    pluginsd_set_chart_from_parent(user, NULL, PLUGINSD_KEYWORD_HOST_DEFINE_END);

    rrdhost_flag_clear(host, RRDHOST_FLAG_ORPHAN);
    rrdcontext_host_child_connected(host);
    schedule_node_info_update(host);

    return PARSER_RC_OK;
}

static PARSER_RC pluginsd_host(char **words, size_t num_words, void *user) {
    PARSER_USER_OBJECT *u = user;

    char *guid = get_word(words, num_words, 1);

    if(!guid || !*guid || strcmp(guid, "localhost") == 0) {
        u->host = localhost;
        return PARSER_RC_OK;
    }

    uuid_t uuid;
    char uuid_str[UUID_STR_LEN];
    if(!pluginsd_validate_machine_guid(guid, &uuid, uuid_str))
        return PLUGINSD_DISABLE_PLUGIN(user, PLUGINSD_KEYWORD_HOST, "cannot parse MACHINE_GUID - is it a valid UUID?");

    RRDHOST *host = rrdhost_find_by_guid(uuid_str);
    if(unlikely(!host))
        return PLUGINSD_DISABLE_PLUGIN(user, PLUGINSD_KEYWORD_HOST, "cannot find a host with this machine guid - have you created it?");

    u->host = host;

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_chart(char **words, size_t num_words, void *user)
{
    RRDHOST *host = pluginsd_require_host_from_parent(user, PLUGINSD_KEYWORD_CHART);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

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
    if (unlikely((!type || !*type || !id || !*id)))
        return PLUGINSD_DISABLE_PLUGIN(user, PLUGINSD_KEYWORD_CHART, "missing parameters");

    // parse the name, and make sure it does not include 'type.'
    if (unlikely(name && *name)) {
        // when data are streamed from child nodes
        // name will be type.name
        // so, we have to remove 'type.' from name too
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
    pluginsd_set_chart_from_parent(user, st, PLUGINSD_KEYWORD_CHART);

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_chart_definition_end(char **words, size_t num_words, void *user)
{
    const char *first_entry_txt = get_word(words, num_words, 1);
    const char *last_entry_txt = get_word(words, num_words, 2);
    const char *wall_clock_time_txt = get_word(words, num_words, 3);

    RRDHOST *host = pluginsd_require_host_from_parent(user, PLUGINSD_KEYWORD_CHART_DEFINITION_END);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    RRDSET *st = pluginsd_require_chart_from_parent(user, PLUGINSD_KEYWORD_CHART_DEFINITION_END, PLUGINSD_KEYWORD_CHART);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    time_t first_entry_child = (first_entry_txt && *first_entry_txt) ? (time_t)str2ul(first_entry_txt) : 0;
    time_t last_entry_child = (last_entry_txt && *last_entry_txt) ? (time_t)str2ul(last_entry_txt) : 0;
    time_t child_wall_clock_time = (wall_clock_time_txt && *wall_clock_time_txt) ? (time_t)str2ul(wall_clock_time_txt) : now_realtime_sec();

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
                                     first_entry_child, last_entry_child, child_wall_clock_time,
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
    if(!host) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    RRDSET *st = pluginsd_require_chart_from_parent(user, PLUGINSD_KEYWORD_DIMENSION, PLUGINSD_KEYWORD_CHART);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    if (unlikely(!id))
        return PLUGINSD_DISABLE_PLUGIN(user, PLUGINSD_KEYWORD_DIMENSION, "missing dimension id");

    long multiplier = 1;
    if (multiplier_s && *multiplier_s) {
        multiplier = str2ll_encoded(multiplier_s);
        if (unlikely(!multiplier))
            multiplier = 1;
    }

    long divisor = 1;
    if (likely(divisor_s && *divisor_s)) {
        divisor = str2ll_encoded(divisor_s);
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

    bool should_update_dimension = false;

    if (likely(unhide_dimension)) {
        rrddim_option_clear(rd, RRDDIM_OPTION_HIDDEN);
        should_update_dimension = rrddim_flag_check(rd, RRDDIM_FLAG_META_HIDDEN);
    }
    else {
        rrddim_option_set(rd, RRDDIM_OPTION_HIDDEN);
        should_update_dimension = !rrddim_flag_check(rd, RRDDIM_FLAG_META_HIDDEN);
    }

    if (should_update_dimension) {
        rrddim_flag_set(rd, RRDDIM_FLAG_METADATA_UPDATE);
        rrdhost_flag_set(rd->rrdset->rrdhost, RRDHOST_FLAG_METADATA_UPDATE);
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
    parser->inflight.functions = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE, &dictionary_stats_category_functions, 0);
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
            pf->destination_wb->content_type = functions_format_to_content_type(format);

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
    if(!host) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    RRDSET *st = pluginsd_get_chart_from_parent(user);

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

    if (unlikely(!name || !*name))
        return PLUGINSD_DISABLE_PLUGIN(user, PLUGINSD_KEYWORD_VARIABLE, "missing variable name");

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

    if (!global && !st)
        return PLUGINSD_DISABLE_PLUGIN(user, PLUGINSD_KEYWORD_VARIABLE, "no chart is defined and no GLOBAL is given");

    char *endptr = NULL;
    v = (NETDATA_DOUBLE) str2ndd_encoded(value, &endptr);
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
    debug(D_PLUGINSD, "requested a " PLUGINSD_KEYWORD_FLUSH);
    pluginsd_set_chart_from_parent(user, NULL, PLUGINSD_KEYWORD_FLUSH);
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
    return PARSER_RC_STOP;
}

PARSER_RC pluginsd_label(char **words, size_t num_words, void *user)
{
    const char *name = get_word(words, num_words, 1);
    const char *label_source = get_word(words, num_words, 2);
    const char *value = get_word(words, num_words, 3);

    if (!name || !label_source || !value)
        return PLUGINSD_DISABLE_PLUGIN(user, PLUGINSD_KEYWORD_LABEL, "missing parameters");

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
    if(!host) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    debug(D_PLUGINSD, "requested to OVERWRITE host labels");

    if(unlikely(!host->rrdlabels))
        host->rrdlabels = rrdlabels_create();

    rrdlabels_migrate_to_these(host->rrdlabels, (DICTIONARY *) (((PARSER_USER_OBJECT *)user)->new_host_labels));
    rrdhost_flag_set(host, RRDHOST_FLAG_METADATA_LABELS | RRDHOST_FLAG_METADATA_UPDATE);

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
        return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);
    }

    if(unlikely(!((PARSER_USER_OBJECT *) user)->chart_rrdlabels_linked_temporarily)) {
        RRDSET *st = pluginsd_get_chart_from_parent(user);
        ((PARSER_USER_OBJECT *)user)->chart_rrdlabels_linked_temporarily = st->rrdlabels;
        rrdlabels_unmark_all(((PARSER_USER_OBJECT *)user)->chart_rrdlabels_linked_temporarily);
    }

    rrdlabels_add(((PARSER_USER_OBJECT *)user)->chart_rrdlabels_linked_temporarily,
                  name, value, str2l(label_source));

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_clabel_commit(char **words __maybe_unused, size_t num_words __maybe_unused, void *user)
{
    RRDHOST *host = pluginsd_require_host_from_parent(user, PLUGINSD_KEYWORD_CLABEL_COMMIT);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    RRDSET *st = pluginsd_require_chart_from_parent(user, PLUGINSD_KEYWORD_CLABEL_COMMIT, PLUGINSD_KEYWORD_BEGIN);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    debug(D_PLUGINSD, "requested to commit chart labels");

    if(!((PARSER_USER_OBJECT *)user)->chart_rrdlabels_linked_temporarily) {
        error("PLUGINSD: 'host:%s' got CLABEL_COMMIT, without a CHART or BEGIN. Ignoring it.",
              rrdhost_hostname(host));
        return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);
    }

    rrdlabels_remove_all_unmarked(((PARSER_USER_OBJECT *)user)->chart_rrdlabels_linked_temporarily);

    rrdset_flag_set(st, RRDSET_FLAG_METADATA_UPDATE);
    rrdhost_flag_set(st->rrdhost, RRDHOST_FLAG_METADATA_UPDATE);

    ((PARSER_USER_OBJECT *)user)->chart_rrdlabels_linked_temporarily = NULL;
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_replay_begin(char **words, size_t num_words, void *user) {
    char *id = get_word(words, num_words, 1);
    char *start_time_str = get_word(words, num_words, 2);
    char *end_time_str = get_word(words, num_words, 3);
    char *child_now_str = get_word(words, num_words, 4);

    RRDHOST *host = pluginsd_require_host_from_parent(user, PLUGINSD_KEYWORD_REPLAY_BEGIN);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    RRDSET *st;
    if (likely(!id || !*id))
        st = pluginsd_require_chart_from_parent(user, PLUGINSD_KEYWORD_REPLAY_BEGIN, PLUGINSD_KEYWORD_REPLAY_BEGIN);
    else
        st = pluginsd_find_chart(host, id, PLUGINSD_KEYWORD_REPLAY_BEGIN);

    if(!st) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);
    pluginsd_set_chart_from_parent(user, st, PLUGINSD_KEYWORD_REPLAY_BEGIN);

    if(start_time_str && end_time_str) {
        time_t start_time = (time_t) str2ull_encoded(start_time_str);
        time_t end_time = (time_t) str2ull_encoded(end_time_str);

        time_t wall_clock_time = 0, tolerance;
        bool wall_clock_comes_from_child; (void)wall_clock_comes_from_child;
        if(child_now_str) {
            wall_clock_time = (time_t) str2ull_encoded(child_now_str);
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
                rrdset_set_update_every_s(st, end_time - start_time);

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

        error("PLUGINSD REPLAY ERROR: 'host:%s/chart:%s' got a " PLUGINSD_KEYWORD_REPLAY_BEGIN
              " from %ld to %ld, but timestamps are invalid "
              "(now is %ld [%s], tolerance %ld). Ignoring " PLUGINSD_KEYWORD_REPLAY_SET,
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

static inline SN_FLAGS pluginsd_parse_storage_number_flags(const char *flags_str) {
    SN_FLAGS flags = SN_FLAG_NONE;

    char c;
    while ((c = *flags_str++)) {
        switch (c) {
            case 'A':
                flags |= SN_FLAG_NOT_ANOMALOUS;
                break;

            case 'R':
                flags |= SN_FLAG_RESET;
                break;

            case 'E':
                flags = SN_EMPTY_SLOT;
                return flags;

            default:
                internal_error(true, "Unknown SN_FLAGS flag '%c'", c);
                break;
        }
    }

    return flags;
}

PARSER_RC pluginsd_replay_set(char **words, size_t num_words, void *user)
{
    char *dimension = get_word(words, num_words, 1);
    char *value_str = get_word(words, num_words, 2);
    char *flags_str = get_word(words, num_words, 3);

    RRDHOST *host = pluginsd_require_host_from_parent(user, PLUGINSD_KEYWORD_REPLAY_SET);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    RRDSET *st = pluginsd_require_chart_from_parent(user, PLUGINSD_KEYWORD_REPLAY_SET, PLUGINSD_KEYWORD_REPLAY_BEGIN);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    PARSER_USER_OBJECT *u = user;
    if(!u->replay.rset_enabled) {
        error_limit_static_thread_var(erl, 1, 0);
        error_limit(&erl, "PLUGINSD: 'host:%s/chart:%s' got a %s but it is disabled by %s errors",
                    rrdhost_hostname(host), rrdset_id(st), PLUGINSD_KEYWORD_REPLAY_SET, PLUGINSD_KEYWORD_REPLAY_BEGIN);

        // we have to return OK here
        return PARSER_RC_OK;
    }

    RRDDIM *rd = pluginsd_acquire_dimension(host, st, dimension, PLUGINSD_KEYWORD_REPLAY_SET);
    if(!rd) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    if (unlikely(!u->replay.start_time || !u->replay.end_time)) {
        error("PLUGINSD: 'host:%s/chart:%s/dim:%s' got a %s with invalid timestamps %ld to %ld from a %s. Disabling it.",
              rrdhost_hostname(host),
              rrdset_id(st),
              dimension,
              PLUGINSD_KEYWORD_REPLAY_SET,
              u->replay.start_time,
              u->replay.end_time,
              PLUGINSD_KEYWORD_REPLAY_BEGIN);
        return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);
    }

    if (unlikely(!value_str || !*value_str))
        value_str = "NAN";

    if(unlikely(!flags_str))
        flags_str = "";

    if (likely(value_str)) {
        RRDDIM_FLAGS rd_flags = rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE | RRDDIM_FLAG_ARCHIVED);

        if(!(rd_flags & RRDDIM_FLAG_ARCHIVED)) {
            NETDATA_DOUBLE value = str2ndd_encoded(value_str, NULL);
            SN_FLAGS flags = pluginsd_parse_storage_number_flags(flags_str);

            if (!netdata_double_isnumber(value) || (flags == SN_EMPTY_SLOT)) {
                value = NAN;
                flags = SN_EMPTY_SLOT;
            }

            rrddim_store_metric(rd, u->replay.end_time_ut, value, flags);
            rd->last_collected_time.tv_sec = u->replay.end_time;
            rd->last_collected_time.tv_usec = 0;
            rd->collections_counter++;
        }
        else {
            error_limit_static_global_var(erl, 1, 0);
            error_limit(&erl, "PLUGINSD: 'host:%s/chart:%s/dim:%s' has the ARCHIVED flag set, but it is replicated. Ignoring data.",
                        rrdhost_hostname(st->rrdhost), rrdset_id(st), rrddim_name(rd));
        }
    }

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_replay_rrddim_collection_state(char **words, size_t num_words, void *user)
{
    if(((PARSER_USER_OBJECT *) user)->replay.rset_enabled == false)
        return PARSER_RC_OK;

    char *dimension = get_word(words, num_words, 1);
    char *last_collected_ut_str = get_word(words, num_words, 2);
    char *last_collected_value_str = get_word(words, num_words, 3);
    char *last_calculated_value_str = get_word(words, num_words, 4);
    char *last_stored_value_str = get_word(words, num_words, 5);

    RRDHOST *host = pluginsd_require_host_from_parent(user, PLUGINSD_KEYWORD_REPLAY_RRDDIM_STATE);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    RRDSET *st = pluginsd_require_chart_from_parent(user, PLUGINSD_KEYWORD_REPLAY_RRDDIM_STATE, PLUGINSD_KEYWORD_REPLAY_BEGIN);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    RRDDIM *rd = pluginsd_acquire_dimension(host, st, dimension, PLUGINSD_KEYWORD_REPLAY_RRDDIM_STATE);
    if(!rd) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    usec_t dim_last_collected_ut = (usec_t)rd->last_collected_time.tv_sec * USEC_PER_SEC + (usec_t)rd->last_collected_time.tv_usec;
    usec_t last_collected_ut = last_collected_ut_str ? str2ull_encoded(last_collected_ut_str) : 0;
    if(last_collected_ut > dim_last_collected_ut) {
        rd->last_collected_time.tv_sec = (time_t)(last_collected_ut / USEC_PER_SEC);
        rd->last_collected_time.tv_usec = (last_collected_ut % USEC_PER_SEC);
    }

    rd->last_collected_value = last_collected_value_str ? str2ll_encoded(last_collected_value_str) : 0;
    rd->last_calculated_value = last_calculated_value_str ? str2ndd_encoded(last_calculated_value_str, NULL) : 0;
    rd->last_stored_value = last_stored_value_str ? str2ndd_encoded(last_stored_value_str, NULL) : 0.0;

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_replay_rrdset_collection_state(char **words, size_t num_words, void *user)
{
    if(((PARSER_USER_OBJECT *) user)->replay.rset_enabled == false)
        return PARSER_RC_OK;

    char *last_collected_ut_str = get_word(words, num_words, 1);
    char *last_updated_ut_str = get_word(words, num_words, 2);

    RRDHOST *host = pluginsd_require_host_from_parent(user, PLUGINSD_KEYWORD_REPLAY_RRDSET_STATE);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    RRDSET *st = pluginsd_require_chart_from_parent(user, PLUGINSD_KEYWORD_REPLAY_RRDSET_STATE, PLUGINSD_KEYWORD_REPLAY_BEGIN);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    usec_t chart_last_collected_ut = (usec_t)st->last_collected_time.tv_sec * USEC_PER_SEC + (usec_t)st->last_collected_time.tv_usec;
    usec_t last_collected_ut = last_collected_ut_str ? str2ull_encoded(last_collected_ut_str) : 0;
    if(last_collected_ut > chart_last_collected_ut) {
        st->last_collected_time.tv_sec = (time_t)(last_collected_ut / USEC_PER_SEC);
        st->last_collected_time.tv_usec = (last_collected_ut % USEC_PER_SEC);
    }

    usec_t chart_last_updated_ut = (usec_t)st->last_updated.tv_sec * USEC_PER_SEC + (usec_t)st->last_updated.tv_usec;
    usec_t last_updated_ut = last_updated_ut_str ? str2ull_encoded(last_updated_ut_str) : 0;
    if(last_updated_ut > chart_last_updated_ut) {
        st->last_updated.tv_sec = (time_t)(last_updated_ut / USEC_PER_SEC);
        st->last_updated.tv_usec = (last_updated_ut % USEC_PER_SEC);
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

    time_t update_every_child = (time_t) str2ull_encoded(update_every_child_txt);
    time_t first_entry_child = (time_t) str2ull_encoded(first_entry_child_txt);
    time_t last_entry_child = (time_t) str2ull_encoded(last_entry_child_txt);

    bool start_streaming = (strcmp(start_streaming_txt, "true") == 0);
    time_t first_entry_requested = (time_t) str2ull_encoded(first_entry_requested_txt);
    time_t last_entry_requested = (time_t) str2ull_encoded(last_entry_requested_txt);

    // the optional child world time
    time_t child_world_time = (child_world_time_txt && *child_world_time_txt) ? (time_t) str2ull_encoded(
            child_world_time_txt) : now_realtime_sec();

    PARSER_USER_OBJECT *user_object = user;

    RRDHOST *host = pluginsd_require_host_from_parent(user, PLUGINSD_KEYWORD_REPLAY_END);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    RRDSET *st = pluginsd_require_chart_from_parent(user, PLUGINSD_KEYWORD_REPLAY_END, PLUGINSD_KEYWORD_REPLAY_BEGIN);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

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

    ((PARSER_USER_OBJECT *) user)->data_collections_count++;

    if(((PARSER_USER_OBJECT *) user)->replay.rset_enabled && st->rrdhost->receiver) {
        time_t now = now_realtime_sec();
        time_t started = st->rrdhost->receiver->replication_first_time_t;
        time_t current = ((PARSER_USER_OBJECT *) user)->replay.end_time;

        if(started && current > started) {
            host->rrdpush_receiver_replication_percent = (NETDATA_DOUBLE) (current - started) * 100.0 / (NETDATA_DOUBLE) (now - started);
            worker_set_metric(WORKER_RECEIVER_JOB_REPLICATION_COMPLETION,
                              host->rrdpush_receiver_replication_percent);
        }
    }

    ((PARSER_USER_OBJECT *) user)->replay.start_time = 0;
    ((PARSER_USER_OBJECT *) user)->replay.end_time = 0;
    ((PARSER_USER_OBJECT *) user)->replay.start_time_ut = 0;
    ((PARSER_USER_OBJECT *) user)->replay.end_time_ut = 0;
    ((PARSER_USER_OBJECT *) user)->replay.wall_clock_time = 0;
    ((PARSER_USER_OBJECT *) user)->replay.rset_enabled = false;

    st->counter++;
    st->counter_done++;
    store_metric_collection_completed();

#ifdef NETDATA_LOG_REPLICATION_REQUESTS
    st->replay.start_streaming = false;
    st->replay.after = 0;
    st->replay.before = 0;
    if(start_streaming)
        st->replay.log_next_data_collection = true;
#endif

    if (start_streaming) {
        if (st->update_every != update_every_child)
            rrdset_set_update_every_s(st, update_every_child);

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

        pluginsd_set_chart_from_parent(user, NULL, PLUGINSD_KEYWORD_REPLAY_END);

        host->rrdpush_receiver_replication_percent = 100.0;
        worker_set_metric(WORKER_RECEIVER_JOB_REPLICATION_COMPLETION, host->rrdpush_receiver_replication_percent);

        return PARSER_RC_OK;
    }

    pluginsd_set_chart_from_parent(user, NULL, PLUGINSD_KEYWORD_REPLAY_END);

    rrdcontext_updated_retention_rrdset(st);

    bool ok = replicate_chart_request(send_to_plugin, user_object->parser, host, st,
                                      first_entry_child, last_entry_child, child_world_time,
                                      first_entry_requested, last_entry_requested);
    return ok ? PARSER_RC_OK : PARSER_RC_ERROR;
}

PARSER_RC pluginsd_begin_v2(char **words, size_t num_words, void *user) {
    timing_init();

    char *id = get_word(words, num_words, 1);
    char *update_every_str = get_word(words, num_words, 2);
    char *end_time_str = get_word(words, num_words, 3);
    char *wall_clock_time_str = get_word(words, num_words, 4);

    if(unlikely(!id || !update_every_str || !end_time_str || !wall_clock_time_str))
        return PLUGINSD_DISABLE_PLUGIN(user, PLUGINSD_KEYWORD_BEGIN_V2, "missing parameters");

    RRDHOST *host = pluginsd_require_host_from_parent(user, PLUGINSD_KEYWORD_BEGIN_V2);
    if(unlikely(!host)) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    timing_step(TIMING_STEP_BEGIN2_PREPARE);

    RRDSET *st = pluginsd_find_chart(host, id, PLUGINSD_KEYWORD_BEGIN_V2);
    if(unlikely(!st)) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    pluginsd_set_chart_from_parent(user, st, PLUGINSD_KEYWORD_BEGIN_V2);

    if(unlikely(rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE | RRDSET_FLAG_ARCHIVED)))
        rrdset_isnot_obsolete(st);

    timing_step(TIMING_STEP_BEGIN2_FIND_CHART);

    // ------------------------------------------------------------------------
    // parse the parameters

    time_t update_every = (time_t) str2ull_encoded(update_every_str);
    time_t end_time = (time_t) str2ull_encoded(end_time_str);

    time_t wall_clock_time;
    if(likely(*wall_clock_time_str == '#'))
        wall_clock_time = end_time;
    else
        wall_clock_time = (time_t) str2ull_encoded(wall_clock_time_str);

    if (unlikely(update_every != st->update_every))
        rrdset_set_update_every_s(st, update_every);

    timing_step(TIMING_STEP_BEGIN2_PARSE);

    // ------------------------------------------------------------------------
    // prepare our state

    pluginsd_lock_rrdset_data_collection(user);

    PARSER_USER_OBJECT *u = (PARSER_USER_OBJECT *) user;
    u->v2.update_every = update_every;
    u->v2.end_time = end_time;
    u->v2.wall_clock_time = wall_clock_time;
    u->v2.ml_locked = ml_chart_update_begin(st);

    timing_step(TIMING_STEP_BEGIN2_ML);

    // ------------------------------------------------------------------------
    // propagate it forward in v2

    if(!u->v2.stream_buffer.wb && rrdhost_has_rrdpush_sender_enabled(st->rrdhost))
        u->v2.stream_buffer = rrdset_push_metric_initialize(u->st, wall_clock_time);

    if(u->v2.stream_buffer.v2 && u->v2.stream_buffer.wb) {
        // check if receiver and sender have the same number parsing capabilities
        bool can_copy = stream_has_capability(u, STREAM_CAP_IEEE754) == stream_has_capability(&u->v2.stream_buffer, STREAM_CAP_IEEE754);
        NUMBER_ENCODING encoding = stream_has_capability(&u->v2.stream_buffer, STREAM_CAP_IEEE754) ? NUMBER_ENCODING_BASE64 : NUMBER_ENCODING_HEX;

        BUFFER *wb = u->v2.stream_buffer.wb;

        buffer_need_bytes(wb, 1024);

        if(unlikely(u->v2.stream_buffer.begin_v2_added))
            buffer_fast_strcat(wb, PLUGINSD_KEYWORD_END_V2 "\n", sizeof(PLUGINSD_KEYWORD_END_V2) - 1 + 1);

        buffer_fast_strcat(wb, PLUGINSD_KEYWORD_BEGIN_V2 " '", sizeof(PLUGINSD_KEYWORD_BEGIN_V2) - 1 + 2);
        buffer_fast_strcat(wb, rrdset_id(st), string_strlen(st->id));
        buffer_fast_strcat(wb, "' ", 2);

        if(can_copy)
            buffer_strcat(wb, update_every_str);
        else
            buffer_print_uint64_encoded(wb, encoding, update_every);

        buffer_fast_strcat(wb, " ", 1);

        if(can_copy)
            buffer_strcat(wb, end_time_str);
        else
            buffer_print_uint64_encoded(wb, encoding, end_time);

        buffer_fast_strcat(wb, " ", 1);

        if(can_copy)
            buffer_strcat(wb, wall_clock_time_str);
        else
            buffer_print_uint64_encoded(wb, encoding, wall_clock_time);

        buffer_fast_strcat(wb, "\n", 1);

        u->v2.stream_buffer.last_point_end_time_s = end_time;
        u->v2.stream_buffer.begin_v2_added = true;
    }

    timing_step(TIMING_STEP_BEGIN2_PROPAGATE);

    // ------------------------------------------------------------------------
    // store it

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

    timing_step(TIMING_STEP_BEGIN2_STORE);

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_set_v2(char **words, size_t num_words, void *user) {
    timing_init();

    char *dimension = get_word(words, num_words, 1);
    char *collected_str = get_word(words, num_words, 2);
    char *value_str = get_word(words, num_words, 3);
    char *flags_str = get_word(words, num_words, 4);

    if(unlikely(!dimension || !collected_str || !value_str || !flags_str))
        return PLUGINSD_DISABLE_PLUGIN(user, PLUGINSD_KEYWORD_SET_V2, "missing parameters");

    PARSER_USER_OBJECT *u = (PARSER_USER_OBJECT *) user;

    RRDHOST *host = pluginsd_require_host_from_parent(user, PLUGINSD_KEYWORD_SET_V2);
    if(unlikely(!host)) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    RRDSET *st = pluginsd_require_chart_from_parent(user, PLUGINSD_KEYWORD_SET_V2, PLUGINSD_KEYWORD_BEGIN_V2);
    if(unlikely(!st)) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    timing_step(TIMING_STEP_SET2_PREPARE);

    RRDDIM *rd = pluginsd_acquire_dimension(host, st, dimension, PLUGINSD_KEYWORD_SET_V2);
    if(unlikely(!rd)) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    if(unlikely(rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE | RRDDIM_FLAG_ARCHIVED)))
        rrddim_isnot_obsolete(st, rd);

    timing_step(TIMING_STEP_SET2_LOOKUP_DIMENSION);

    // ------------------------------------------------------------------------
    // parse the parameters

    collected_number collected_value = (collected_number) str2ll_encoded(collected_str);

    NETDATA_DOUBLE value;
    if(*value_str == '#')
        value = (NETDATA_DOUBLE)collected_value;
    else
        value = str2ndd_encoded(value_str, NULL);

    SN_FLAGS flags = pluginsd_parse_storage_number_flags(flags_str);

    timing_step(TIMING_STEP_SET2_PARSE);

    // ------------------------------------------------------------------------
    // check value and ML

    if (unlikely(!netdata_double_isnumber(value) || (flags == SN_EMPTY_SLOT))) {
        value = NAN;
        flags = SN_EMPTY_SLOT;

        if(u->v2.ml_locked)
            ml_dimension_is_anomalous(rd, u->v2.end_time, 0, false);
    }
    else if(u->v2.ml_locked) {
        if (ml_dimension_is_anomalous(rd, u->v2.end_time, value, true)) {
            // clear anomaly bit: 0 -> is anomalous, 1 -> not anomalous
            flags &= ~((storage_number) SN_FLAG_NOT_ANOMALOUS);
        }
        else
            flags |= SN_FLAG_NOT_ANOMALOUS;
    }

    timing_step(TIMING_STEP_SET2_ML);

    // ------------------------------------------------------------------------
    // propagate it forward in v2

    if(u->v2.stream_buffer.v2 && u->v2.stream_buffer.begin_v2_added && u->v2.stream_buffer.wb) {
        // check if receiver and sender have the same number parsing capabilities
        bool can_copy = stream_has_capability(u, STREAM_CAP_IEEE754) == stream_has_capability(&u->v2.stream_buffer, STREAM_CAP_IEEE754);
        NUMBER_ENCODING integer_encoding = stream_has_capability(&u->v2.stream_buffer, STREAM_CAP_IEEE754) ? NUMBER_ENCODING_BASE64 : NUMBER_ENCODING_HEX;
        NUMBER_ENCODING doubles_encoding = stream_has_capability(&u->v2.stream_buffer, STREAM_CAP_IEEE754) ? NUMBER_ENCODING_BASE64 : NUMBER_ENCODING_DECIMAL;

        BUFFER *wb = u->v2.stream_buffer.wb;
        buffer_need_bytes(wb, 1024);
        buffer_fast_strcat(wb, PLUGINSD_KEYWORD_SET_V2 " '", sizeof(PLUGINSD_KEYWORD_SET_V2) - 1 + 2);
        buffer_fast_strcat(wb, rrddim_id(rd), string_strlen(rd->id));
        buffer_fast_strcat(wb, "' ", 2);
        if(can_copy)
            buffer_strcat(wb, collected_str);
        else
            buffer_print_int64_encoded(wb, integer_encoding, collected_value); // original v2 had hex
        buffer_fast_strcat(wb, " ", 1);
        if(can_copy)
            buffer_strcat(wb, value_str);
        else
            buffer_print_netdata_double_encoded(wb, doubles_encoding, value); // original v2 had decimal
        buffer_fast_strcat(wb, " ", 1);
        buffer_print_sn_flags(wb, flags, true);
        buffer_fast_strcat(wb, "\n", 1);
    }

    timing_step(TIMING_STEP_SET2_PROPAGATE);

    // ------------------------------------------------------------------------
    // store it

    rrddim_store_metric(rd, u->v2.end_time * USEC_PER_SEC, value, flags);
    rd->last_collected_time.tv_sec = u->v2.end_time;
    rd->last_collected_time.tv_usec = 0;
    rd->last_collected_value = collected_value;
    rd->last_stored_value = value;
    rd->last_calculated_value = value;
    rd->collections_counter++;
    rd->updated = true;

    timing_step(TIMING_STEP_SET2_STORE);

    return PARSER_RC_OK;
}

void pluginsd_cleanup_v2(void *user) {
    // this is called when the thread is stopped while processing
    pluginsd_set_chart_from_parent(user, NULL, "THREAD CLEANUP");
}

PARSER_RC pluginsd_end_v2(char **words __maybe_unused, size_t num_words __maybe_unused, void *user) {
    timing_init();

    RRDHOST *host = pluginsd_require_host_from_parent(user, PLUGINSD_KEYWORD_END_V2);
    if(unlikely(!host)) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    RRDSET *st = pluginsd_require_chart_from_parent(user, PLUGINSD_KEYWORD_END_V2, PLUGINSD_KEYWORD_BEGIN_V2);
    if(unlikely(!st)) return PLUGINSD_DISABLE_PLUGIN(user, NULL, NULL);

    PARSER_USER_OBJECT *u = (PARSER_USER_OBJECT *) user;
    u->data_collections_count++;

    timing_step(TIMING_STEP_END2_PREPARE);

    // ------------------------------------------------------------------------
    // propagate the whole chart update in v1

    if(unlikely(!u->v2.stream_buffer.v2 && !u->v2.stream_buffer.begin_v2_added && u->v2.stream_buffer.wb))
        rrdset_push_metrics_v1(&u->v2.stream_buffer, st);

    timing_step(TIMING_STEP_END2_PUSH_V1);

    // ------------------------------------------------------------------------
    // unblock data collection

    pluginsd_unlock_previous_chart(user, PLUGINSD_KEYWORD_END_V2, false);
    rrdcontext_collected_rrdset(st);
    store_metric_collection_completed();

    timing_step(TIMING_STEP_END2_RRDSET);

    // ------------------------------------------------------------------------
    // propagate it forward

    rrdset_push_metrics_finished(&u->v2.stream_buffer, st);

    timing_step(TIMING_STEP_END2_PROPAGATE);

    // ------------------------------------------------------------------------
    // cleanup RRDSET / RRDDIM

    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
                rd->calculated_value = 0;
                rd->collected_value = 0;
                rd->updated = false;
            }
    rrddim_foreach_done(rd);

    // ------------------------------------------------------------------------
    // reset state

    u->v2 = (struct parser_user_object_v2){ 0 };

    timing_step(TIMING_STEP_END2_STORE);
    timing_report();

    return PARSER_RC_OK;
}

void pluginsd_process_thread_cleanup(void *ptr) {
    PARSER *parser = (PARSER *)ptr;

    pluginsd_cleanup_v2(parser->user);
    pluginsd_host_define_cleanup(parser->user);

    rrd_collector_finished();

    parser_destroy(parser);
}

// New plugins.d parser

inline size_t pluginsd_process(RRDHOST *host, struct plugind *cd, FILE *fp_plugin_input, FILE *fp_plugin_output, int trust_durations)
{
    int enabled = cd->unsafe.enabled;

    if (!fp_plugin_input || !fp_plugin_output || !enabled) {
        cd->unsafe.enabled = 0;
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
        .enabled = cd->unsafe.enabled,
        .host = host,
        .cd = cd,
        .trust_durations = trust_durations
    };

    // fp_plugin_output = our input; fp_plugin_input = our output
    PARSER *parser = parser_init(&user, fp_plugin_output, fp_plugin_input, -1,
                                 PARSER_INPUT_SPLIT, NULL);

    pluginsd_keywords_init(parser, PARSER_INIT_PLUGINSD);

    rrd_collector_started();

    // this keeps the parser with its current value
    // so, parser needs to be allocated before pushing it
    netdata_thread_cleanup_push(pluginsd_process_thread_cleanup, parser);

    user.parser = parser;
    char buffer[PLUGINSD_LINE_MAX + 1];

    while (likely(!parser_next(parser, buffer, PLUGINSD_LINE_MAX))) {
        if (unlikely(!service_running(SERVICE_COLLECTORS) || parser_action(parser,  buffer)))
            break;
    }

    // free parser with the pop function
    netdata_thread_cleanup_pop(1);

    cd->unsafe.enabled = user.enabled;
    size_t count = user.data_collections_count;

    if (likely(count)) {
        cd->successful_collections += count;
        cd->serial_failures = 0;
    }
    else
        cd->serial_failures++;

    return count;
}

PARSER_RC pluginsd_exit(char **words __maybe_unused, size_t num_words __maybe_unused, void *user __maybe_unused)
{
    info("PLUGINSD: plugin called EXIT.");
    return PARSER_RC_STOP;
}

static void pluginsd_keywords_init_internal(PARSER *parser, PLUGINSD_KEYWORDS types, void (*add_func)(PARSER *parser, char *keyword, keyword_function func)) {

    if (types & PARSER_INIT_PLUGINSD) {
        add_func(parser, PLUGINSD_KEYWORD_FLUSH, pluginsd_flush);
        add_func(parser, PLUGINSD_KEYWORD_DISABLE, pluginsd_disable);

        add_func(parser, PLUGINSD_KEYWORD_HOST_DEFINE, pluginsd_host_define);
        add_func(parser, PLUGINSD_KEYWORD_HOST_DEFINE_END, pluginsd_host_define_end);
        add_func(parser, PLUGINSD_KEYWORD_HOST_LABEL, pluginsd_host_labels);
        add_func(parser, PLUGINSD_KEYWORD_HOST, pluginsd_host);

        add_func(parser, PLUGINSD_KEYWORD_EXIT, pluginsd_exit);
    }

    if (types & (PARSER_INIT_PLUGINSD | PARSER_INIT_STREAMING)) {
        // plugins.d plugins and streaming
        add_func(parser, PLUGINSD_KEYWORD_CHART, pluginsd_chart);
        add_func(parser, PLUGINSD_KEYWORD_DIMENSION, pluginsd_dimension);
        add_func(parser, PLUGINSD_KEYWORD_VARIABLE, pluginsd_variable);
        add_func(parser, PLUGINSD_KEYWORD_LABEL, pluginsd_label);
        add_func(parser, PLUGINSD_KEYWORD_OVERWRITE, pluginsd_overwrite);
        add_func(parser, PLUGINSD_KEYWORD_CLABEL_COMMIT, pluginsd_clabel_commit);
        add_func(parser, PLUGINSD_KEYWORD_CLABEL, pluginsd_clabel);
        add_func(parser, PLUGINSD_KEYWORD_FUNCTION, pluginsd_function);
        add_func(parser, PLUGINSD_KEYWORD_FUNCTION_RESULT_BEGIN, pluginsd_function_result_begin);

        add_func(parser, PLUGINSD_KEYWORD_BEGIN, pluginsd_begin);
        add_func(parser, PLUGINSD_KEYWORD_SET, pluginsd_set);
        add_func(parser, PLUGINSD_KEYWORD_END, pluginsd_end);

        inflight_functions_init(parser);
    }

    if (types & PARSER_INIT_STREAMING) {
        add_func(parser, PLUGINSD_KEYWORD_CHART_DEFINITION_END, pluginsd_chart_definition_end);

        // replication
        add_func(parser, PLUGINSD_KEYWORD_REPLAY_BEGIN, pluginsd_replay_begin);
        add_func(parser, PLUGINSD_KEYWORD_REPLAY_SET, pluginsd_replay_set);
        add_func(parser, PLUGINSD_KEYWORD_REPLAY_RRDDIM_STATE, pluginsd_replay_rrddim_collection_state);
        add_func(parser, PLUGINSD_KEYWORD_REPLAY_RRDSET_STATE, pluginsd_replay_rrdset_collection_state);
        add_func(parser, PLUGINSD_KEYWORD_REPLAY_END, pluginsd_replay_end);

        // streaming metrics v2
        add_func(parser, PLUGINSD_KEYWORD_BEGIN_V2, pluginsd_begin_v2);
        add_func(parser, PLUGINSD_KEYWORD_SET_V2, pluginsd_set_v2);
        add_func(parser, PLUGINSD_KEYWORD_END_V2, pluginsd_end_v2);
    }
}

void pluginsd_keywords_init(PARSER *parser, PLUGINSD_KEYWORDS types) {
    pluginsd_keywords_init_internal(parser, types, parser_add_keyword);
}

struct pluginsd_user_unittest {
    size_t size;
    const char **hashtable;
    uint32_t (*hash)(const char *s);
    size_t collisions;
};

void pluginsd_keyword_collision_check(PARSER *parser, char *keyword, keyword_function func __maybe_unused) {
    struct pluginsd_user_unittest *u = parser->user;

    uint32_t hash = u->hash(keyword);
    uint32_t slot = hash % u->size;

    if(u->hashtable[slot])
        u->collisions++;

    u->hashtable[slot] = keyword;
}

static struct {
    const char *name;
    uint32_t (*hash)(const char *s);
    size_t slots_needed;
} hashers[] = {
    { .name = "djb2_hash32(s)",            djb2_hash32, .slots_needed = 0, },
    { .name = "fnv1_hash32(s)",            fnv1_hash32, .slots_needed = 0, },
    { .name = "fnv1a_hash32(s)",           fnv1a_hash32, .slots_needed = 0, },
    { .name = "larson_hash32(s)",          larson_hash32, .slots_needed = 0, },
    { .name = "pluginsd_parser_hash32(s)", pluginsd_parser_hash32, .slots_needed = 0, },

    // terminator
    { .name = NULL, NULL, .slots_needed = 0, },
};

int pluginsd_parser_unittest(void) {
    PARSER *p;
    size_t slots_to_check = 1000;
    size_t i, h;

    // check for hashtable collisions
    for(h = 0; hashers[h].name ;h++) {
        hashers[h].slots_needed = slots_to_check * 1000000;

        for (i = 10; i < slots_to_check; i++) {
            struct pluginsd_user_unittest user = {
                    .hash = hashers[h].hash,
                    .size = i,
                    .hashtable = callocz(i, sizeof(const char *)),
                    .collisions = 0,
            };

            p = parser_init(&user, NULL, NULL, -1, PARSER_INPUT_SPLIT, NULL);
            pluginsd_keywords_init_internal(p, PARSER_INIT_PLUGINSD | PARSER_INIT_STREAMING,
                                            pluginsd_keyword_collision_check);
            parser_destroy(p);

            freez(user.hashtable);

            if (!user.collisions) {
                hashers[h].slots_needed = i;
                break;
            }
        }
    }

    for(h = 0; hashers[h].name ;h++) {
        if(hashers[h].slots_needed > 1000)
            info("PARSER: hash function '%s' cannot be used without collisions under %zu slots", hashers[h].name, slots_to_check);
        else
            info("PARSER: hash function '%s' needs PARSER_KEYWORDS_HASHTABLE_SIZE (in parser.h) set to %zu", hashers[h].name, hashers[h].slots_needed);
    }

    p = parser_init(NULL, NULL, NULL, -1, PARSER_INPUT_SPLIT, NULL);
    pluginsd_keywords_init(p, PARSER_INIT_PLUGINSD | PARSER_INIT_STREAMING);
    parser_destroy(p);
    return 0;
}

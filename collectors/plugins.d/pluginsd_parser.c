// SPDX-License-Identifier: GPL-3.0-or-later

#include "pluginsd_parser.h"

#define LOG_FUNCTIONS false

#define SERVING_STREAMING(parser) (parser->repertoire == PARSER_INIT_STREAMING)
#define SERVING_PLUGINSD(parser) (parser->repertoire == PARSER_INIT_PLUGINSD)

static ssize_t send_to_plugin(const char *txt, void *data) {
    PARSER *parser = data;

    if(!txt || !*txt)
        return 0;

    errno = 0;
    spinlock_lock(&parser->writer.spinlock);
    ssize_t bytes = -1;

#ifdef ENABLE_HTTPS
    NETDATA_SSL *ssl = parser->ssl_output;
    if(ssl) {

        if(SSL_connection(ssl))
            bytes = netdata_ssl_write(ssl, (void *) txt, strlen(txt));

        else
            netdata_log_error("PLUGINSD: cannot send command (SSL)");

        spinlock_unlock(&parser->writer.spinlock);
        return bytes;
    }
#endif

    if(parser->fp_output) {

        bytes = fprintf(parser->fp_output, "%s", txt);
        if(bytes <= 0) {
            netdata_log_error("PLUGINSD: cannot send command (FILE)");
            bytes = -2;
        }
        else
            fflush(parser->fp_output);

        spinlock_unlock(&parser->writer.spinlock);
        return bytes;
    }

    if(parser->fd != -1) {
        bytes = 0;
        ssize_t total = (ssize_t)strlen(txt);
        ssize_t sent;

        do {
            sent = write(parser->fd, &txt[bytes], total - bytes);
            if(sent <= 0) {
                netdata_log_error("PLUGINSD: cannot send command (fd)");
                spinlock_unlock(&parser->writer.spinlock);
                return -3;
            }
            bytes += sent;
        }
        while(bytes < total);

        spinlock_unlock(&parser->writer.spinlock);
        return (int)bytes;
    }

    spinlock_unlock(&parser->writer.spinlock);
    netdata_log_error("PLUGINSD: cannot send command (no output socket/pipe/file given to plugins.d parser)");
    return -4;
}

static inline RRDHOST *pluginsd_require_scope_host(PARSER *parser, const char *cmd) {
    RRDHOST *host = parser->user.host;

    if(unlikely(!host))
        netdata_log_error("PLUGINSD: command %s requires a host, but is not set.", cmd);

    return host;
}

static inline RRDSET *pluginsd_require_scope_chart(PARSER *parser, const char *cmd, const char *parent_cmd) {
    RRDSET *st = parser->user.st;

    if(unlikely(!st))
        netdata_log_error("PLUGINSD: command %s requires a chart defined via command %s, but is not set.", cmd, parent_cmd);

    return st;
}

static inline RRDSET *pluginsd_get_scope_chart(PARSER *parser) {
    return parser->user.st;
}

static inline void pluginsd_lock_rrdset_data_collection(PARSER *parser) {
    if(parser->user.st && !parser->user.v2.locked_data_collection) {
        spinlock_lock(&parser->user.st->data_collection_lock);
        parser->user.v2.locked_data_collection = true;
    }
}

static inline bool pluginsd_unlock_rrdset_data_collection(PARSER *parser) {
    if(parser->user.st && parser->user.v2.locked_data_collection) {
        spinlock_unlock(&parser->user.st->data_collection_lock);
        parser->user.v2.locked_data_collection = false;
        return true;
    }

    return false;
}

void pluginsd_rrdset_cleanup(RRDSET *st) {
    spinlock_lock(&st->pluginsd.spinlock);

    for(size_t i = 0; i < st->pluginsd.size ; i++) {
        rrddim_acquired_release(st->pluginsd.rda[i]); // can be NULL
        st->pluginsd.rda[i] = NULL;
    }

    freez(st->pluginsd.rda);
    st->pluginsd.collector_tid = 0;
    st->pluginsd.rda = NULL;
    st->pluginsd.size = 0;
    st->pluginsd.pos = 0;

    spinlock_unlock(&st->pluginsd.spinlock);
}

static inline void pluginsd_unlock_previous_scope_chart(PARSER *parser, const char *keyword, bool stale) {
    if(unlikely(pluginsd_unlock_rrdset_data_collection(parser))) {
        if(stale)
            netdata_log_error("PLUGINSD: 'host:%s/chart:%s/' stale data collection lock found during %s; it has been unlocked",
                              rrdhost_hostname(parser->user.st->rrdhost),
                              rrdset_id(parser->user.st),
                              keyword);
    }

    if(unlikely(parser->user.v2.ml_locked)) {
        ml_chart_update_end(parser->user.st);
        parser->user.v2.ml_locked = false;

        if(stale)
            netdata_log_error("PLUGINSD: 'host:%s/chart:%s/' stale ML lock found during %s, it has been unlocked",
                              rrdhost_hostname(parser->user.st->rrdhost),
                              rrdset_id(parser->user.st),
                              keyword);
    }
}

static inline void pluginsd_clear_scope_chart(PARSER *parser, const char *keyword) {
    pluginsd_unlock_previous_scope_chart(parser, keyword, true);
    parser->user.st = NULL;
}

static inline bool pluginsd_set_scope_chart(PARSER *parser, RRDSET *st, const char *keyword) {
    RRDSET *old_st = parser->user.st;
    pid_t old_collector_tid = (old_st) ? old_st->pluginsd.collector_tid : 0;
    pid_t my_collector_tid = gettid();

    if(unlikely(old_collector_tid)) {
        if(old_collector_tid != my_collector_tid) {
            error_limit_static_global_var(erl, 1, 0);
            error_limit(&erl, "PLUGINSD: keyword %s: 'host:%s/chart:%s' is collected twice (my tid %d, other collector tid %d)",
                        keyword ? keyword : "UNKNOWN",
                        rrdhost_hostname(st->rrdhost), rrdset_id(st),
                        my_collector_tid, old_collector_tid);

            return false;
        }

        old_st->pluginsd.collector_tid = 0;
    }

    st->pluginsd.collector_tid = my_collector_tid;

    pluginsd_clear_scope_chart(parser, keyword);

    size_t dims = dictionary_entries(st->rrddim_root_index);
    if(unlikely(st->pluginsd.size < dims)) {
        st->pluginsd.rda = reallocz(st->pluginsd.rda, dims * sizeof(RRDDIM_ACQUIRED *));

        // initialize the empty slots
        for(ssize_t i = (ssize_t)dims - 1; i >= (ssize_t)st->pluginsd.size ;i--)
            st->pluginsd.rda[i] = NULL;

        st->pluginsd.size = dims;
    }

    st->pluginsd.pos = 0;
    parser->user.st = st;

    return true;
}

static inline RRDDIM *pluginsd_acquire_dimension(RRDHOST *host, RRDSET *st, const char *dimension, const char *cmd) {
    if (unlikely(!dimension || !*dimension)) {
        netdata_log_error("PLUGINSD: 'host:%s/chart:%s' got a %s, without a dimension.",
                          rrdhost_hostname(host), rrdset_id(st), cmd);
        return NULL;
    }

    if(unlikely(st->pluginsd.pos >= st->pluginsd.size))
        st->pluginsd.pos = 0;

    RRDDIM_ACQUIRED *rda = st->pluginsd.rda[st->pluginsd.pos];

    if(likely(rda)) {
        RRDDIM *rd = rrddim_acquired_to_rrddim(rda);
        if (likely(rd && string_strcmp(rd->id, dimension) == 0)) {
            // we found a cached RDA
            st->pluginsd.pos++;
            return rd;
        }
        else {
            // the collector is sending dimensions in a different order
            // release the previous one, to reuse this slot
            rrddim_acquired_release(rda);
            st->pluginsd.rda[st->pluginsd.pos] = NULL;
        }
    }

    rda = rrddim_find_and_acquire(st, dimension);
    if (unlikely(!rda)) {
        netdata_log_error("PLUGINSD: 'host:%s/chart:%s/dim:%s' got a %s but dimension does not exist.",
                          rrdhost_hostname(host), rrdset_id(st), dimension, cmd);

        return NULL;
    }

    st->pluginsd.rda[st->pluginsd.pos++] = rda;

    return rrddim_acquired_to_rrddim(rda);
}

static inline RRDSET *pluginsd_find_chart(RRDHOST *host, const char *chart, const char *cmd) {
    if (unlikely(!chart || !*chart)) {
        netdata_log_error("PLUGINSD: 'host:%s' got a %s without a chart id.",
                          rrdhost_hostname(host), cmd);
        return NULL;
    }

    RRDSET *st = rrdset_find(host, chart);
    if (unlikely(!st))
        netdata_log_error("PLUGINSD: 'host:%s/chart:%s' got a %s but chart does not exist.",
                          rrdhost_hostname(host), chart, cmd);

    return st;
}

static inline PARSER_RC PLUGINSD_DISABLE_PLUGIN(PARSER *parser, const char *keyword, const char *msg) {
    parser->user.enabled = 0;

    if(keyword && msg) {
        error_limit_static_global_var(erl, 1, 0);
        error_limit(&erl, "PLUGINSD: keyword %s: %s", keyword, msg);
    }

    return PARSER_RC_ERROR;
}

static inline PARSER_RC pluginsd_set(char **words, size_t num_words, PARSER *parser) {
    char *dimension = get_word(words, num_words, 1);
    char *value = get_word(words, num_words, 2);

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_SET);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    RRDSET *st = pluginsd_require_scope_chart(parser, PLUGINSD_KEYWORD_SET, PLUGINSD_KEYWORD_CHART);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    RRDDIM *rd = pluginsd_acquire_dimension(host, st, dimension, PLUGINSD_KEYWORD_SET);
    if(!rd) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    st->pluginsd.set = true;

    if (unlikely(rrdset_flag_check(st, RRDSET_FLAG_DEBUG)))
        netdata_log_debug(D_PLUGINSD, "PLUGINSD: 'host:%s/chart:%s/dim:%s' SET is setting value to '%s'",
              rrdhost_hostname(host), rrdset_id(st), dimension, value && *value ? value : "UNSET");

    if (value && *value)
        rrddim_set_by_pointer(st, rd, str2ll_encoded(value));

    return PARSER_RC_OK;
}

static inline PARSER_RC pluginsd_begin(char **words, size_t num_words, PARSER *parser) {
    char *id = get_word(words, num_words, 1);
    char *microseconds_txt = get_word(words, num_words, 2);

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_BEGIN);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    RRDSET *st = pluginsd_find_chart(host, id, PLUGINSD_KEYWORD_BEGIN);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    if(!pluginsd_set_scope_chart(parser, st, PLUGINSD_KEYWORD_BEGIN))
        return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

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
            if (parser->user.trust_durations)
                rrdset_next_usec_unfiltered(st, microseconds);
            else
                rrdset_next_usec(st, microseconds);
        }
        else
            rrdset_next(st);
    }
    return PARSER_RC_OK;
}

static inline PARSER_RC pluginsd_end(char **words, size_t num_words, PARSER *parser) {
    UNUSED(words);
    UNUSED(num_words);

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_END);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    RRDSET *st = pluginsd_require_scope_chart(parser, PLUGINSD_KEYWORD_END, PLUGINSD_KEYWORD_BEGIN);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    if (unlikely(rrdset_flag_check(st, RRDSET_FLAG_DEBUG)))
        netdata_log_debug(D_PLUGINSD, "requested an END on chart '%s'", rrdset_id(st));

    pluginsd_clear_scope_chart(parser, PLUGINSD_KEYWORD_END);
    parser->user.data_collections_count++;

    struct timeval now;
    now_realtime_timeval(&now);
    rrdset_timed_done(st, now, /* pending_rrdset_next = */ false);

    return PARSER_RC_OK;
}

static void pluginsd_host_define_cleanup(PARSER *parser) {
    string_freez(parser->user.host_define.hostname);
    rrdlabels_destroy(parser->user.host_define.rrdlabels);

    parser->user.host_define.hostname = NULL;
    parser->user.host_define.rrdlabels = NULL;
    parser->user.host_define.parsing_host = false;
}

static inline bool pluginsd_validate_machine_guid(const char *guid, uuid_t *uuid, char *output) {
    if(uuid_parse(guid, *uuid))
        return false;

    uuid_unparse_lower(*uuid, output);

    return true;
}

static inline PARSER_RC pluginsd_host_define(char **words, size_t num_words, PARSER *parser) {
    char *guid = get_word(words, num_words, 1);
    char *hostname = get_word(words, num_words, 2);

    if(unlikely(!guid || !*guid || !hostname || !*hostname))
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_HOST_DEFINE, "missing parameters");

    if(unlikely(parser->user.host_define.parsing_host))
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_HOST_DEFINE,
            "another host definition is already open - did you send " PLUGINSD_KEYWORD_HOST_DEFINE_END "?");

    if(!pluginsd_validate_machine_guid(guid, &parser->user.host_define.machine_guid, parser->user.host_define.machine_guid_str))
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_HOST_DEFINE, "cannot parse MACHINE_GUID - is it a valid UUID?");

    parser->user.host_define.hostname = string_strdupz(hostname);
    parser->user.host_define.rrdlabels = rrdlabels_create();
    parser->user.host_define.parsing_host = true;

    return PARSER_RC_OK;
}

static inline PARSER_RC pluginsd_host_dictionary(char **words, size_t num_words, PARSER *parser, RRDLABELS *labels, const char *keyword) {
    char *name = get_word(words, num_words, 1);
    char *value = get_word(words, num_words, 2);

    if(!name || !*name || !value)
        return PLUGINSD_DISABLE_PLUGIN(parser, keyword, "missing parameters");

    if(!parser->user.host_define.parsing_host || !labels)
        return PLUGINSD_DISABLE_PLUGIN(parser, keyword, "host is not defined, send " PLUGINSD_KEYWORD_HOST_DEFINE " before this");

    rrdlabels_add(labels, name, value, RRDLABEL_SRC_CONFIG);

    return PARSER_RC_OK;
}

static inline PARSER_RC pluginsd_host_labels(char **words, size_t num_words, PARSER *parser) {
    return pluginsd_host_dictionary(words, num_words, parser,
                                    parser->user.host_define.rrdlabels,
                                    PLUGINSD_KEYWORD_HOST_LABEL);
}

static inline PARSER_RC pluginsd_host_define_end(char **words __maybe_unused, size_t num_words __maybe_unused, PARSER *parser) {
    if(!parser->user.host_define.parsing_host)
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_HOST_DEFINE_END, "missing initialization, send " PLUGINSD_KEYWORD_HOST_DEFINE " before this");

    RRDHOST *host = rrdhost_find_or_create(
            string2str(parser->user.host_define.hostname),
            string2str(parser->user.host_define.hostname),
            parser->user.host_define.machine_guid_str,
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
            rrdhost_labels_to_system_info(parser->user.host_define.rrdlabels),
            false
            );

    rrdhost_option_set(host, RRDHOST_OPTION_VIRTUAL_HOST);

    if(host->rrdlabels) {
        rrdlabels_migrate_to_these(host->rrdlabels, parser->user.host_define.rrdlabels);
    }
    else {
        host->rrdlabels = parser->user.host_define.rrdlabels;
        parser->user.host_define.rrdlabels = NULL;
    }

    pluginsd_host_define_cleanup(parser);

    parser->user.host = host;
    pluginsd_clear_scope_chart(parser, PLUGINSD_KEYWORD_HOST_DEFINE_END);

    rrdhost_flag_clear(host, RRDHOST_FLAG_ORPHAN);
    rrdcontext_host_child_connected(host);
    schedule_node_info_update(host);

    return PARSER_RC_OK;
}

static inline PARSER_RC pluginsd_host(char **words, size_t num_words, PARSER *parser) {
    char *guid = get_word(words, num_words, 1);

    if(!guid || !*guid || strcmp(guid, "localhost") == 0) {
        parser->user.host = localhost;
        return PARSER_RC_OK;
    }

    uuid_t uuid;
    char uuid_str[UUID_STR_LEN];
    if(!pluginsd_validate_machine_guid(guid, &uuid, uuid_str))
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_HOST, "cannot parse MACHINE_GUID - is it a valid UUID?");

    RRDHOST *host = rrdhost_find_by_guid(uuid_str);
    if(unlikely(!host))
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_HOST, "cannot find a host with this machine guid - have you created it?");

    parser->user.host = host;

    return PARSER_RC_OK;
}

static inline PARSER_RC pluginsd_chart(char **words, size_t num_words, PARSER *parser) {
    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_CHART);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

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
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_CHART, "missing parameters");

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

    int update_every = parser->user.cd->update_every;
    if (likely(update_every_s && *update_every_s))
        update_every = str2i(update_every_s);
    if (unlikely(!update_every))
        update_every = parser->user.cd->update_every;

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

    netdata_log_debug(
        D_PLUGINSD,
        "creating chart type='%s', id='%s', name='%s', family='%s', context='%s', chart='%s', priority=%d, update_every=%d",
        type, id, name ? name : "", family ? family : "", context ? context : "", rrdset_type_name(chart_type),
        priority, update_every);

    RRDSET *st = NULL;

    st = rrdset_create(
        host, type, id, name, family, context, title, units,
        (plugin && *plugin) ? plugin : parser->user.cd->filename,
        module, priority, update_every,
        chart_type);

    if (likely(st)) {
        if (options && *options) {
            if (strstr(options, "obsolete")) {
                pluginsd_rrdset_cleanup(st);
                rrdset_is_obsolete(st);
            }
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
        }
        else {
            rrdset_isnot_obsolete(st);
            rrdset_flag_clear(st, RRDSET_FLAG_DETAIL);
            rrdset_flag_clear(st, RRDSET_FLAG_STORE_FIRST);
        }

        if(!pluginsd_set_scope_chart(parser, st, PLUGINSD_KEYWORD_CHART))
            return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);
    }
    else
        pluginsd_clear_scope_chart(parser, PLUGINSD_KEYWORD_CHART);

    return PARSER_RC_OK;
}

static inline PARSER_RC pluginsd_chart_definition_end(char **words, size_t num_words, PARSER *parser) {
    const char *first_entry_txt = get_word(words, num_words, 1);
    const char *last_entry_txt = get_word(words, num_words, 2);
    const char *wall_clock_time_txt = get_word(words, num_words, 3);

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_CHART_DEFINITION_END);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    RRDSET *st = pluginsd_require_scope_chart(parser, PLUGINSD_KEYWORD_CHART_DEFINITION_END, PLUGINSD_KEYWORD_CHART);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

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

static inline PARSER_RC pluginsd_dimension(char **words, size_t num_words, PARSER *parser) {
    char *id = get_word(words, num_words, 1);
    char *name = get_word(words, num_words, 2);
    char *algorithm = get_word(words, num_words, 3);
    char *multiplier_s = get_word(words, num_words, 4);
    char *divisor_s = get_word(words, num_words, 5);
    char *options = get_word(words, num_words, 6);

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_DIMENSION);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    RRDSET *st = pluginsd_require_scope_chart(parser, PLUGINSD_KEYWORD_DIMENSION, PLUGINSD_KEYWORD_CHART);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    if (unlikely(!id))
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_DIMENSION, "missing dimension id");

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
        netdata_log_debug(
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
    STRING *function;
    BUFFER *result_body_wb;
    rrd_function_result_callback_t result_cb;
    void *result_cb_data;
    usec_t timeout_ut;
    usec_t started_ut;
    usec_t sent_ut;
    const char *payload;
    PARSER *parser;
    bool virtual;
};

static void inflight_functions_insert_callback(const DICTIONARY_ITEM *item, void *func, void *parser_ptr) {
    struct inflight_function *pf = func;

    PARSER  *parser = parser_ptr;

    // leave this code as default, so that when the dictionary is destroyed this will be sent back to the caller
    pf->code = HTTP_RESP_GATEWAY_TIMEOUT;

    const char *transaction = dictionary_acquired_item_name(item);

    char buffer[2048 + 1];
    snprintfz(buffer, 2048, "%s %s %d \"%s\"\n",
                      pf->payload ? "FUNCTION_PAYLOAD" : "FUNCTION",
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

void delete_job_finalize(struct parser *parser __maybe_unused, struct configurable_plugin *plug, const char *fnc_sig, int code) {
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

void set_job_finalize(struct parser *parser __maybe_unused, struct configurable_plugin *plug __maybe_unused, const char *fnc_sig, int code) {
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

void pluginsd_function_cancel(void *data) {
    struct inflight_function *look_for = data, *t;

    bool sent = false;
    dfe_start_read(look_for->parser->inflight.functions, t) {
        if(look_for == t) {
            const char *transaction = t_dfe.name;

            internal_error(true, "PLUGINSD: sending function cancellation to plugin for transaction '%s'", transaction);

            char buffer[2048 + 1];
            snprintfz(buffer, 2048, "%s %s\n",
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
        netdata_log_error("PLUGINSD: FUNCTION_CANCEL request didn't match any pending function requests in pluginsd.d.");
}

// this is the function that is called from
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
        .timeout_ut = now + timeout * USEC_PER_SEC,
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
        inflight_functions_garbage_collect(parser, now);

    dictionary_write_unlock(parser->inflight.functions);

    return HTTP_RESP_OK;
}

static inline PARSER_RC pluginsd_function(char **words, size_t num_words, PARSER *parser) {
    // a plugin or a child is registering a function

    bool global = false;
    size_t i = 1;
    if(num_words >= 2 && strcmp(get_word(words, num_words, 1), "GLOBAL") == 0) {
        i++;
        global = true;
    }

    char *name      = get_word(words, num_words, i++);
    char *timeout_s = get_word(words, num_words, i++);
    char *help      = get_word(words, num_words, i++);

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_FUNCTION);
    if(!host) return PARSER_RC_ERROR;

    RRDSET *st = (global)? NULL: pluginsd_require_scope_chart(parser, PLUGINSD_KEYWORD_FUNCTION, PLUGINSD_KEYWORD_CHART);
    if(!st) global = true;

    if (unlikely(!timeout_s || !name || !help || (!global && !st))) {
        netdata_log_error("PLUGINSD: 'host:%s/chart:%s' got a FUNCTION, without providing the required data (global = '%s', name = '%s', timeout = '%s', help = '%s'). Ignoring it.",
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

    rrd_function_add(host, st, name, timeout, help, false, pluginsd_function_execute_cb, parser);

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

static inline PARSER_RC pluginsd_function_result_begin(char **words, size_t num_words, PARSER *parser) {
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

// ----------------------------------------------------------------------------

static inline PARSER_RC pluginsd_variable(char **words, size_t num_words, PARSER *parser) {
    char *name = get_word(words, num_words, 1);
    char *value = get_word(words, num_words, 2);
    NETDATA_DOUBLE v;

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_VARIABLE);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    RRDSET *st = pluginsd_get_scope_chart(parser);

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
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_VARIABLE, "missing variable name");

    if (unlikely(!value || !*value))
        value = NULL;

    if (unlikely(!value)) {
        netdata_log_error("PLUGINSD: 'host:%s/chart:%s' cannot set %s VARIABLE '%s' to an empty value",
                          rrdhost_hostname(host),
                          st ? rrdset_id(st):"UNSET",
                          (global) ? "HOST" : "CHART",
                          name);
        return PARSER_RC_OK;
    }

    if (!global && !st)
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_VARIABLE, "no chart is defined and no GLOBAL is given");

    char *endptr = NULL;
    v = (NETDATA_DOUBLE) str2ndd_encoded(value, &endptr);
    if (unlikely(endptr && *endptr)) {
        if (endptr == value)
            netdata_log_error("PLUGINSD: 'host:%s/chart:%s' the value '%s' of VARIABLE '%s' cannot be parsed as a number",
                              rrdhost_hostname(host),
                              st ? rrdset_id(st):"UNSET",
                              value,
                              name);
        else
            netdata_log_error("PLUGINSD: 'host:%s/chart:%s' the value '%s' of VARIABLE '%s' has leftovers: '%s'",
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
            netdata_log_error("PLUGINSD: 'host:%s' cannot find/create HOST VARIABLE '%s'",
                              rrdhost_hostname(host),
                              name);
    } else {
        const RRDSETVAR_ACQUIRED *rsa = rrdsetvar_custom_chart_variable_add_and_acquire(st, name);
        if (rsa) {
            rrdsetvar_custom_chart_variable_set(st, rsa, v);
            rrdsetvar_custom_chart_variable_release(st, rsa);
        }
        else
            netdata_log_error("PLUGINSD: 'host:%s/chart:%s' cannot find/create CHART VARIABLE '%s'",
                              rrdhost_hostname(host), rrdset_id(st), name);
    }

    return PARSER_RC_OK;
}

static inline PARSER_RC pluginsd_flush(char **words __maybe_unused, size_t num_words __maybe_unused, PARSER *parser) {
    netdata_log_debug(D_PLUGINSD, "requested a " PLUGINSD_KEYWORD_FLUSH);
    pluginsd_clear_scope_chart(parser, PLUGINSD_KEYWORD_FLUSH);
    parser->user.replay.start_time = 0;
    parser->user.replay.end_time = 0;
    parser->user.replay.start_time_ut = 0;
    parser->user.replay.end_time_ut = 0;
    return PARSER_RC_OK;
}

static inline PARSER_RC pluginsd_disable(char **words __maybe_unused, size_t num_words __maybe_unused, PARSER *parser) {
    netdata_log_info("PLUGINSD: plugin called DISABLE. Disabling it.");
    parser->user.enabled = 0;
    return PARSER_RC_STOP;
}

static inline PARSER_RC pluginsd_label(char **words, size_t num_words, PARSER *parser) {
    const char *name = get_word(words, num_words, 1);
    const char *label_source = get_word(words, num_words, 2);
    const char *value = get_word(words, num_words, 3);

    if (!name || !label_source || !value)
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_LABEL, "missing parameters");

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

    if(unlikely(!(parser->user.new_host_labels)))
        parser->user.new_host_labels = rrdlabels_create();

    rrdlabels_add(parser->user.new_host_labels, name, store, str2l(label_source));

    if (allocated_store)
        freez(store);

    return PARSER_RC_OK;
}

static inline PARSER_RC pluginsd_overwrite(char **words __maybe_unused, size_t num_words __maybe_unused, PARSER *parser) {
    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_OVERWRITE);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    netdata_log_debug(D_PLUGINSD, "requested to OVERWRITE host labels");

    if(unlikely(!host->rrdlabels))
        host->rrdlabels = rrdlabels_create();

    rrdlabels_migrate_to_these(host->rrdlabels, parser->user.new_host_labels);
    rrdhost_flag_set(host, RRDHOST_FLAG_METADATA_LABELS | RRDHOST_FLAG_METADATA_UPDATE);

    rrdlabels_destroy(parser->user.new_host_labels);
    parser->user.new_host_labels = NULL;
    return PARSER_RC_OK;
}

static inline PARSER_RC pluginsd_clabel(char **words, size_t num_words, PARSER *parser) {
    const char *name = get_word(words, num_words, 1);
    const char *value = get_word(words, num_words, 2);
    const char *label_source = get_word(words, num_words, 3);

    if (!name || !value || !label_source) {
        netdata_log_error("Ignoring malformed or empty CHART LABEL command.");
        return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);
    }

    if(unlikely(!parser->user.chart_rrdlabels_linked_temporarily)) {
        RRDSET *st = pluginsd_get_scope_chart(parser);
        parser->user.chart_rrdlabels_linked_temporarily = st->rrdlabels;
        rrdlabels_unmark_all(parser->user.chart_rrdlabels_linked_temporarily);
    }

    rrdlabels_add(parser->user.chart_rrdlabels_linked_temporarily, name, value, str2l(label_source));

    return PARSER_RC_OK;
}

static inline PARSER_RC pluginsd_clabel_commit(char **words __maybe_unused, size_t num_words __maybe_unused, PARSER *parser) {
    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_CLABEL_COMMIT);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    RRDSET *st = pluginsd_require_scope_chart(parser, PLUGINSD_KEYWORD_CLABEL_COMMIT, PLUGINSD_KEYWORD_BEGIN);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    netdata_log_debug(D_PLUGINSD, "requested to commit chart labels");

    if(!parser->user.chart_rrdlabels_linked_temporarily) {
        netdata_log_error("PLUGINSD: 'host:%s' got CLABEL_COMMIT, without a CHART or BEGIN. Ignoring it.", rrdhost_hostname(host));
        return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);
    }

    rrdlabels_remove_all_unmarked(parser->user.chart_rrdlabels_linked_temporarily);

    rrdset_flag_set(st, RRDSET_FLAG_METADATA_UPDATE);
    rrdhost_flag_set(st->rrdhost, RRDHOST_FLAG_METADATA_UPDATE);

    parser->user.chart_rrdlabels_linked_temporarily = NULL;
    return PARSER_RC_OK;
}

static inline PARSER_RC pluginsd_replay_begin(char **words, size_t num_words, PARSER *parser) {
    char *id = get_word(words, num_words, 1);
    char *start_time_str = get_word(words, num_words, 2);
    char *end_time_str = get_word(words, num_words, 3);
    char *child_now_str = get_word(words, num_words, 4);

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_REPLAY_BEGIN);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    RRDSET *st;
    if (likely(!id || !*id))
        st = pluginsd_require_scope_chart(parser, PLUGINSD_KEYWORD_REPLAY_BEGIN, PLUGINSD_KEYWORD_REPLAY_BEGIN);
    else
        st = pluginsd_find_chart(host, id, PLUGINSD_KEYWORD_REPLAY_BEGIN);

    if(!st) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    if(!pluginsd_set_scope_chart(parser, st, PLUGINSD_KEYWORD_REPLAY_BEGIN))
        return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

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
            st->db.current_entry++;
            if(st->db.current_entry >= st->db.entries)
                st->db.current_entry -= st->db.entries;

            parser->user.replay.start_time = start_time;
            parser->user.replay.end_time = end_time;
            parser->user.replay.start_time_ut = (usec_t) start_time * USEC_PER_SEC;
            parser->user.replay.end_time_ut = (usec_t) end_time * USEC_PER_SEC;
            parser->user.replay.wall_clock_time = wall_clock_time;
            parser->user.replay.rset_enabled = true;

            return PARSER_RC_OK;
        }

        netdata_log_error("PLUGINSD REPLAY ERROR: 'host:%s/chart:%s' got a " PLUGINSD_KEYWORD_REPLAY_BEGIN
                          " from %ld to %ld, but timestamps are invalid "
                          "(now is %ld [%s], tolerance %ld). Ignoring " PLUGINSD_KEYWORD_REPLAY_SET,
                          rrdhost_hostname(st->rrdhost), rrdset_id(st), start_time, end_time,
                          wall_clock_time, wall_clock_comes_from_child ? "child wall clock" : "parent wall clock",
                          tolerance);
    }

    // the child sends an RBEGIN without any parameters initially
    // setting rset_enabled to false, means the RSET should not store any metrics
    // to store metrics, the RBEGIN needs to have timestamps
    parser->user.replay.start_time = 0;
    parser->user.replay.end_time = 0;
    parser->user.replay.start_time_ut = 0;
    parser->user.replay.end_time_ut = 0;
    parser->user.replay.wall_clock_time = 0;
    parser->user.replay.rset_enabled = false;
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

static inline PARSER_RC pluginsd_replay_set(char **words, size_t num_words, PARSER *parser) {
    char *dimension = get_word(words, num_words, 1);
    char *value_str = get_word(words, num_words, 2);
    char *flags_str = get_word(words, num_words, 3);

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_REPLAY_SET);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    RRDSET *st = pluginsd_require_scope_chart(parser, PLUGINSD_KEYWORD_REPLAY_SET, PLUGINSD_KEYWORD_REPLAY_BEGIN);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    if(!parser->user.replay.rset_enabled) {
        error_limit_static_thread_var(erl, 1, 0);
        error_limit(&erl, "PLUGINSD: 'host:%s/chart:%s' got a %s but it is disabled by %s errors",
                    rrdhost_hostname(host), rrdset_id(st), PLUGINSD_KEYWORD_REPLAY_SET, PLUGINSD_KEYWORD_REPLAY_BEGIN);

        // we have to return OK here
        return PARSER_RC_OK;
    }

    RRDDIM *rd = pluginsd_acquire_dimension(host, st, dimension, PLUGINSD_KEYWORD_REPLAY_SET);
    if(!rd) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    st->pluginsd.set = true;

    if (unlikely(!parser->user.replay.start_time || !parser->user.replay.end_time)) {
        netdata_log_error("PLUGINSD: 'host:%s/chart:%s/dim:%s' got a %s with invalid timestamps %ld to %ld from a %s. Disabling it.",
              rrdhost_hostname(host),
              rrdset_id(st),
              dimension,
              PLUGINSD_KEYWORD_REPLAY_SET,
              parser->user.replay.start_time,
              parser->user.replay.end_time,
              PLUGINSD_KEYWORD_REPLAY_BEGIN);
        return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);
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

            rrddim_store_metric(rd, parser->user.replay.end_time_ut, value, flags);
            rd->collector.last_collected_time.tv_sec = parser->user.replay.end_time;
            rd->collector.last_collected_time.tv_usec = 0;
            rd->collector.counter++;
        }
        else {
            error_limit_static_global_var(erl, 1, 0);
            error_limit(&erl, "PLUGINSD: 'host:%s/chart:%s/dim:%s' has the ARCHIVED flag set, but it is replicated. Ignoring data.",
                        rrdhost_hostname(st->rrdhost), rrdset_id(st), rrddim_name(rd));
        }
    }

    return PARSER_RC_OK;
}

static inline PARSER_RC pluginsd_replay_rrddim_collection_state(char **words, size_t num_words, PARSER *parser) {
    if(parser->user.replay.rset_enabled == false)
        return PARSER_RC_OK;

    char *dimension = get_word(words, num_words, 1);
    char *last_collected_ut_str = get_word(words, num_words, 2);
    char *last_collected_value_str = get_word(words, num_words, 3);
    char *last_calculated_value_str = get_word(words, num_words, 4);
    char *last_stored_value_str = get_word(words, num_words, 5);

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_REPLAY_RRDDIM_STATE);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    RRDSET *st = pluginsd_require_scope_chart(parser, PLUGINSD_KEYWORD_REPLAY_RRDDIM_STATE, PLUGINSD_KEYWORD_REPLAY_BEGIN);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    if(st->pluginsd.set) {
        // reset pos to reuse the same RDAs
        st->pluginsd.pos = 0;
        st->pluginsd.set = false;
    }

    RRDDIM *rd = pluginsd_acquire_dimension(host, st, dimension, PLUGINSD_KEYWORD_REPLAY_RRDDIM_STATE);
    if(!rd) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    usec_t dim_last_collected_ut = (usec_t)rd->collector.last_collected_time.tv_sec * USEC_PER_SEC + (usec_t)rd->collector.last_collected_time.tv_usec;
    usec_t last_collected_ut = last_collected_ut_str ? str2ull_encoded(last_collected_ut_str) : 0;
    if(last_collected_ut > dim_last_collected_ut) {
        rd->collector.last_collected_time.tv_sec = (time_t)(last_collected_ut / USEC_PER_SEC);
        rd->collector.last_collected_time.tv_usec = (last_collected_ut % USEC_PER_SEC);
    }

    rd->collector.last_collected_value = last_collected_value_str ? str2ll_encoded(last_collected_value_str) : 0;
    rd->collector.last_calculated_value = last_calculated_value_str ? str2ndd_encoded(last_calculated_value_str, NULL) : 0;
    rd->collector.last_stored_value = last_stored_value_str ? str2ndd_encoded(last_stored_value_str, NULL) : 0.0;

    return PARSER_RC_OK;
}

static inline PARSER_RC pluginsd_replay_rrdset_collection_state(char **words, size_t num_words, PARSER *parser) {
    if(parser->user.replay.rset_enabled == false)
        return PARSER_RC_OK;

    char *last_collected_ut_str = get_word(words, num_words, 1);
    char *last_updated_ut_str = get_word(words, num_words, 2);

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_REPLAY_RRDSET_STATE);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    RRDSET *st = pluginsd_require_scope_chart(parser, PLUGINSD_KEYWORD_REPLAY_RRDSET_STATE,
                                              PLUGINSD_KEYWORD_REPLAY_BEGIN);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

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

static inline PARSER_RC pluginsd_replay_end(char **words, size_t num_words, PARSER *parser) {
    if (num_words < 7) { // accepts 7, but the 7th is optional
        netdata_log_error("REPLAY: malformed " PLUGINSD_KEYWORD_REPLAY_END " command");
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

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_REPLAY_END);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    RRDSET *st = pluginsd_require_scope_chart(parser, PLUGINSD_KEYWORD_REPLAY_END, PLUGINSD_KEYWORD_REPLAY_BEGIN);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

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

    parser->user.data_collections_count++;

    if(parser->user.replay.rset_enabled && st->rrdhost->receiver) {
        time_t now = now_realtime_sec();
        time_t started = st->rrdhost->receiver->replication_first_time_t;
        time_t current = parser->user.replay.end_time;

        if(started && current > started) {
            host->rrdpush_receiver_replication_percent = (NETDATA_DOUBLE) (current - started) * 100.0 / (NETDATA_DOUBLE) (now - started);
            worker_set_metric(WORKER_RECEIVER_JOB_REPLICATION_COMPLETION,
                              host->rrdpush_receiver_replication_percent);
        }
    }

    parser->user.replay.start_time = 0;
    parser->user.replay.end_time = 0;
    parser->user.replay.start_time_ut = 0;
    parser->user.replay.end_time_ut = 0;
    parser->user.replay.wall_clock_time = 0;
    parser->user.replay.rset_enabled = false;

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

        pluginsd_clear_scope_chart(parser, PLUGINSD_KEYWORD_REPLAY_END);

        host->rrdpush_receiver_replication_percent = 100.0;
        worker_set_metric(WORKER_RECEIVER_JOB_REPLICATION_COMPLETION, host->rrdpush_receiver_replication_percent);

        return PARSER_RC_OK;
    }

    pluginsd_clear_scope_chart(parser, PLUGINSD_KEYWORD_REPLAY_END);

    rrdcontext_updated_retention_rrdset(st);

    bool ok = replicate_chart_request(send_to_plugin, parser, host, st,
                                      first_entry_child, last_entry_child, child_world_time,
                                      first_entry_requested, last_entry_requested);
    return ok ? PARSER_RC_OK : PARSER_RC_ERROR;
}

static inline PARSER_RC pluginsd_begin_v2(char **words, size_t num_words, PARSER *parser) {
    timing_init();

    char *id = get_word(words, num_words, 1);
    char *update_every_str = get_word(words, num_words, 2);
    char *end_time_str = get_word(words, num_words, 3);
    char *wall_clock_time_str = get_word(words, num_words, 4);

    if(unlikely(!id || !update_every_str || !end_time_str || !wall_clock_time_str))
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_BEGIN_V2, "missing parameters");

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_BEGIN_V2);
    if(unlikely(!host)) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    timing_step(TIMING_STEP_BEGIN2_PREPARE);

    RRDSET *st = pluginsd_find_chart(host, id, PLUGINSD_KEYWORD_BEGIN_V2);
    if(unlikely(!st)) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    if(!pluginsd_set_scope_chart(parser, st, PLUGINSD_KEYWORD_BEGIN_V2))
        return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    if(unlikely(rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE)))
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

    pluginsd_lock_rrdset_data_collection(parser);

    parser->user.v2.update_every = update_every;
    parser->user.v2.end_time = end_time;
    parser->user.v2.wall_clock_time = wall_clock_time;
    parser->user.v2.ml_locked = ml_chart_update_begin(st);

    timing_step(TIMING_STEP_BEGIN2_ML);

    // ------------------------------------------------------------------------
    // propagate it forward in v2

    if(!parser->user.v2.stream_buffer.wb && rrdhost_has_rrdpush_sender_enabled(st->rrdhost))
        parser->user.v2.stream_buffer = rrdset_push_metric_initialize(parser->user.st, wall_clock_time);

    if(parser->user.v2.stream_buffer.v2 && parser->user.v2.stream_buffer.wb) {
        // check if receiver and sender have the same number parsing capabilities
        bool can_copy = stream_has_capability(&parser->user, STREAM_CAP_IEEE754) == stream_has_capability(&parser->user.v2.stream_buffer, STREAM_CAP_IEEE754);
        NUMBER_ENCODING encoding = stream_has_capability(&parser->user.v2.stream_buffer, STREAM_CAP_IEEE754) ? NUMBER_ENCODING_BASE64 : NUMBER_ENCODING_HEX;

        BUFFER *wb = parser->user.v2.stream_buffer.wb;

        buffer_need_bytes(wb, 1024);

        if(unlikely(parser->user.v2.stream_buffer.begin_v2_added))
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

        parser->user.v2.stream_buffer.last_point_end_time_s = end_time;
        parser->user.v2.stream_buffer.begin_v2_added = true;
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
    st->db.current_entry++;
    if(st->db.current_entry >= st->db.entries)
        st->db.current_entry -= st->db.entries;

    timing_step(TIMING_STEP_BEGIN2_STORE);

    return PARSER_RC_OK;
}

static inline PARSER_RC pluginsd_set_v2(char **words, size_t num_words, PARSER *parser) {
    timing_init();

    char *dimension = get_word(words, num_words, 1);
    char *collected_str = get_word(words, num_words, 2);
    char *value_str = get_word(words, num_words, 3);
    char *flags_str = get_word(words, num_words, 4);

    if(unlikely(!dimension || !collected_str || !value_str || !flags_str))
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_SET_V2, "missing parameters");

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_SET_V2);
    if(unlikely(!host)) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    RRDSET *st = pluginsd_require_scope_chart(parser, PLUGINSD_KEYWORD_SET_V2, PLUGINSD_KEYWORD_BEGIN_V2);
    if(unlikely(!st)) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    timing_step(TIMING_STEP_SET2_PREPARE);

    RRDDIM *rd = pluginsd_acquire_dimension(host, st, dimension, PLUGINSD_KEYWORD_SET_V2);
    if(unlikely(!rd)) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    st->pluginsd.set = true;

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

        if(parser->user.v2.ml_locked)
            ml_dimension_is_anomalous(rd, parser->user.v2.end_time, 0, false);
    }
    else if(parser->user.v2.ml_locked) {
        if (ml_dimension_is_anomalous(rd, parser->user.v2.end_time, value, true)) {
            // clear anomaly bit: 0 -> is anomalous, 1 -> not anomalous
            flags &= ~((storage_number) SN_FLAG_NOT_ANOMALOUS);
        }
        else
            flags |= SN_FLAG_NOT_ANOMALOUS;
    }

    timing_step(TIMING_STEP_SET2_ML);

    // ------------------------------------------------------------------------
    // propagate it forward in v2

    if(parser->user.v2.stream_buffer.v2 && parser->user.v2.stream_buffer.begin_v2_added && parser->user.v2.stream_buffer.wb) {
        // check if receiver and sender have the same number parsing capabilities
        bool can_copy = stream_has_capability(&parser->user, STREAM_CAP_IEEE754) == stream_has_capability(&parser->user.v2.stream_buffer, STREAM_CAP_IEEE754);
        NUMBER_ENCODING integer_encoding = stream_has_capability(&parser->user.v2.stream_buffer, STREAM_CAP_IEEE754) ? NUMBER_ENCODING_BASE64 : NUMBER_ENCODING_HEX;
        NUMBER_ENCODING doubles_encoding = stream_has_capability(&parser->user.v2.stream_buffer, STREAM_CAP_IEEE754) ? NUMBER_ENCODING_BASE64 : NUMBER_ENCODING_DECIMAL;

        BUFFER *wb = parser->user.v2.stream_buffer.wb;
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

    rrddim_store_metric(rd, parser->user.v2.end_time * USEC_PER_SEC, value, flags);
    rd->collector.last_collected_time.tv_sec = parser->user.v2.end_time;
    rd->collector.last_collected_time.tv_usec = 0;
    rd->collector.last_collected_value = collected_value;
    rd->collector.last_stored_value = value;
    rd->collector.last_calculated_value = value;
    rd->collector.counter++;
    rrddim_set_updated(rd);

    timing_step(TIMING_STEP_SET2_STORE);

    return PARSER_RC_OK;
}

void pluginsd_cleanup_v2(PARSER *parser) {
    // this is called when the thread is stopped while processing
    pluginsd_clear_scope_chart(parser, "THREAD CLEANUP");
}

static inline PARSER_RC pluginsd_end_v2(char **words __maybe_unused, size_t num_words __maybe_unused, PARSER *parser) {
    timing_init();

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_END_V2);
    if(unlikely(!host)) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    RRDSET *st = pluginsd_require_scope_chart(parser, PLUGINSD_KEYWORD_END_V2, PLUGINSD_KEYWORD_BEGIN_V2);
    if(unlikely(!st)) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    parser->user.data_collections_count++;

    timing_step(TIMING_STEP_END2_PREPARE);

    // ------------------------------------------------------------------------
    // propagate the whole chart update in v1

    if(unlikely(!parser->user.v2.stream_buffer.v2 && !parser->user.v2.stream_buffer.begin_v2_added && parser->user.v2.stream_buffer.wb))
        rrdset_push_metrics_v1(&parser->user.v2.stream_buffer, st);

    timing_step(TIMING_STEP_END2_PUSH_V1);

    // ------------------------------------------------------------------------
    // unblock data collection

    pluginsd_unlock_previous_scope_chart(parser, PLUGINSD_KEYWORD_END_V2, false);
    rrdcontext_collected_rrdset(st);
    store_metric_collection_completed();

    timing_step(TIMING_STEP_END2_RRDSET);

    // ------------------------------------------------------------------------
    // propagate it forward

    rrdset_push_metrics_finished(&parser->user.v2.stream_buffer, st);

    timing_step(TIMING_STEP_END2_PROPAGATE);

    // ------------------------------------------------------------------------
    // cleanup RRDSET / RRDDIM

    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        rd->collector.calculated_value = 0;
        rd->collector.collected_value = 0;
        rrddim_clear_updated(rd);
    }
    rrddim_foreach_done(rd);

    // ------------------------------------------------------------------------
    // reset state

    parser->user.v2 = (struct parser_user_object_v2){ 0 };

    timing_step(TIMING_STEP_END2_STORE);
    timing_report();

    return PARSER_RC_OK;
}

static inline PARSER_RC pluginsd_exit(char **words __maybe_unused, size_t num_words __maybe_unused, PARSER *parser __maybe_unused) {
    netdata_log_info("PLUGINSD: plugin called EXIT.");
    return PARSER_RC_STOP;
}

struct mutex_cond {
    pthread_mutex_t lock;
    pthread_cond_t cond;
    int rc;
};

static void virt_fnc_got_data_cb(BUFFER *wb __maybe_unused, int code, void *callback_data)
{
    struct mutex_cond *ctx = callback_data;
    pthread_mutex_lock(&ctx->lock);
    ctx->rc = code;
    pthread_cond_broadcast(&ctx->cond);
    pthread_mutex_unlock(&ctx->lock);
}

#define VIRT_FNC_TIMEOUT 1
#define VIRT_FNC_BUF_SIZE (4096)
void call_virtual_function_async(BUFFER *wb, RRDHOST *host, const char *name, const char *payload, rrd_function_result_callback_t callback, void *callback_data) {
    PARSER *parser = NULL;

    //TODO simplify (as we really need only first parameter to get plugin name maybe we can avoid parsing all)
    char *words[PLUGINSD_MAX_WORDS];
    char *function_with_params = strdupz(name);
    size_t num_words = quoted_strings_splitter(function_with_params, words, PLUGINSD_MAX_WORDS, isspace_map_pluginsd);

    if (num_words < 2) {
        netdata_log_error("PLUGINSD: virtual function name is empty.");
        freez(function_with_params);
        return;
    }

    const DICTIONARY_ITEM *cpi = dictionary_get_and_acquire_item(host->configurable_plugins, get_word(words, num_words, 1));
    if (unlikely(cpi == NULL)) {
        netdata_log_error("PLUGINSD: virtual function plugin '%s' not found.", name);
        freez(function_with_params);
        return;
    }
    struct configurable_plugin *cp = dictionary_acquired_item_value(cpi);
    parser = (PARSER *)cp->cb_usr_ctx;

    BUFFER *function_out = buffer_create(VIRT_FNC_BUF_SIZE, NULL);
    // if we are forwarding this to a plugin (as opposed to streaming/child) we have to remove the first parameter (plugin_name)
    buffer_strcat(function_out, get_word(words, num_words, 0));
    for (size_t i = 1; i < num_words; i++) {
        if (i == 1 && SERVING_PLUGINSD(parser))
            continue;
        buffer_sprintf(function_out, " %s", get_word(words, num_words, i));
    }
    freez(function_with_params);

    usec_t now = now_realtime_usec();

    struct inflight_function tmp = {
        .started_ut = now,
        .timeout_ut = now + VIRT_FNC_TIMEOUT + USEC_PER_SEC,
        .result_body_wb = wb,
        .timeout = VIRT_FNC_TIMEOUT * 10,
        .function = string_strdupz(buffer_tostring(function_out)),
        .result_cb = callback,
        .result_cb_data = callback_data,
        .payload = payload != NULL ? strdupz(payload) : NULL,
        .virtual = true,
    };
    buffer_free(function_out);

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
}


dyncfg_config_t call_virtual_function_blocking(PARSER *parser, const char *name, int *rc, const char *payload) {
    usec_t now = now_realtime_usec();
    BUFFER *wb = buffer_create(VIRT_FNC_BUF_SIZE, NULL);

    struct mutex_cond cond = {
        .lock = PTHREAD_MUTEX_INITIALIZER,
        .cond = PTHREAD_COND_INITIALIZER
    };

    struct inflight_function tmp = {
        .started_ut = now,
        .timeout_ut = now + VIRT_FNC_TIMEOUT + USEC_PER_SEC,
        .result_body_wb = wb,
        .timeout = VIRT_FNC_TIMEOUT,
        .function = string_strdupz(name),
        .result_cb = virt_fnc_got_data_cb,
        .result_cb_data = &cond,
        .payload = payload != NULL ? strdupz(payload) : NULL,
        .virtual = true,
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

    struct timespec tp;
    clock_gettime(CLOCK_REALTIME, &tp);
    tp.tv_sec += (time_t)VIRT_FNC_TIMEOUT;

    pthread_mutex_lock(&cond.lock);

    int ret = pthread_cond_timedwait(&cond.cond, &cond.lock, &tp);
    if (ret == ETIMEDOUT)
        netdata_log_error("PLUGINSD: DYNCFG virtual function %s timed out", name);

    pthread_mutex_unlock(&cond.lock);

    dyncfg_config_t cfg;
    cfg.data = strdupz(buffer_tostring(wb));
    cfg.data_size = buffer_strlen(wb);

    if (rc != NULL)
        *rc = cond.rc;

    buffer_free(wb);
    return cfg;
}

#define CVF_MAX_LEN (1024)
static dyncfg_config_t get_plugin_config_cb(void *usr_ctx, const char *plugin_name)
{
    PARSER *parser = usr_ctx;

    if (SERVING_STREAMING(parser)) {
        char buf[CVF_MAX_LEN + 1];
        snprintfz(buf, CVF_MAX_LEN, FUNCTION_NAME_GET_PLUGIN_CONFIG " %s", plugin_name);
        return call_virtual_function_blocking(parser, buf, NULL, NULL);
    }

    return call_virtual_function_blocking(parser, FUNCTION_NAME_GET_PLUGIN_CONFIG, NULL, NULL);
}

static dyncfg_config_t get_plugin_config_schema_cb(void *usr_ctx, const char *plugin_name)
{
    PARSER *parser = usr_ctx;

    if (SERVING_STREAMING(parser)) {
        char buf[CVF_MAX_LEN + 1];
        snprintfz(buf, CVF_MAX_LEN, FUNCTION_NAME_GET_PLUGIN_CONFIG_SCHEMA " %s", plugin_name);
        return call_virtual_function_blocking(parser, buf, NULL, NULL);
    }

    return call_virtual_function_blocking(parser, "get_plugin_config_schema", NULL, NULL);
}

static dyncfg_config_t get_module_config_cb(void *usr_ctx, const char *plugin_name, const char *module_name)
{
    PARSER *parser = usr_ctx;
    BUFFER *wb = buffer_create(CVF_MAX_LEN, NULL);

    buffer_strcat(wb, FUNCTION_NAME_GET_MODULE_CONFIG);
    if (SERVING_STREAMING(parser))
        buffer_sprintf(wb, " %s", plugin_name);
    
    buffer_sprintf(wb, " %s", module_name);

    dyncfg_config_t ret = call_virtual_function_blocking(parser, buffer_tostring(wb), NULL, NULL);

    buffer_free(wb);

    return ret;
}

static dyncfg_config_t get_module_config_schema_cb(void *usr_ctx, const char *plugin_name, const char *module_name)
{
    PARSER *parser = usr_ctx;
    BUFFER *wb = buffer_create(CVF_MAX_LEN, NULL);

    buffer_strcat(wb, FUNCTION_NAME_GET_MODULE_CONFIG_SCHEMA);
    if (SERVING_STREAMING(parser))
        buffer_sprintf(wb, " %s", plugin_name);

    buffer_sprintf(wb, " %s", module_name);

    dyncfg_config_t ret = call_virtual_function_blocking(parser, buffer_tostring(wb), NULL, NULL);

    buffer_free(wb);

    return ret;
}

static dyncfg_config_t get_job_config_schema_cb(void *usr_ctx, const char *plugin_name, const char *module_name)
{
    PARSER *parser = usr_ctx;
    BUFFER *wb = buffer_create(CVF_MAX_LEN, NULL);

    buffer_strcat(wb, FUNCTION_NAME_GET_JOB_CONFIG_SCHEMA);

    if (SERVING_STREAMING(parser))
        buffer_sprintf(wb, " %s", plugin_name);

    buffer_sprintf(wb, " %s", module_name);

    dyncfg_config_t ret = call_virtual_function_blocking(parser, buffer_tostring(wb), NULL, NULL);

    buffer_free(wb);

    return ret;
}

static dyncfg_config_t get_job_config_cb(void *usr_ctx, const char *plugin_name, const char *module_name, const char* job_name)
{
    PARSER *parser = usr_ctx;
    BUFFER *wb = buffer_create(CVF_MAX_LEN, NULL);

    buffer_strcat(wb, FUNCTION_NAME_GET_JOB_CONFIG);

    if (SERVING_STREAMING(parser))
        buffer_sprintf(wb, " %s", plugin_name);

    buffer_sprintf(wb, " %s %s", module_name, job_name);

    dyncfg_config_t ret = call_virtual_function_blocking(parser, buffer_tostring(wb), NULL, NULL);

    buffer_free(wb);

    return ret;
}

enum set_config_result set_plugin_config_cb(void *usr_ctx, const char *plugin_name, dyncfg_config_t *cfg)
{
    PARSER *parser = usr_ctx;
    BUFFER *wb = buffer_create(CVF_MAX_LEN, NULL);

    buffer_strcat(wb, FUNCTION_NAME_SET_PLUGIN_CONFIG);

    if (SERVING_STREAMING(parser))
        buffer_sprintf(wb, " %s", plugin_name);

    int rc;
    call_virtual_function_blocking(parser, buffer_tostring(wb), &rc, cfg->data);

    buffer_free(wb);
    if(rc != DYNCFG_VFNC_RET_CFG_ACCEPTED)
        return SET_CONFIG_REJECTED;
    return SET_CONFIG_ACCEPTED;
}

enum set_config_result set_module_config_cb(void *usr_ctx, const char *plugin_name, const char *module_name, dyncfg_config_t *cfg)
{
    PARSER *parser = usr_ctx;
    BUFFER *wb = buffer_create(CVF_MAX_LEN, NULL);

    buffer_strcat(wb, FUNCTION_NAME_SET_MODULE_CONFIG);

    if (SERVING_STREAMING(parser))
        buffer_sprintf(wb, " %s", plugin_name);

    buffer_sprintf(wb, " %s", module_name);

    int rc;
    call_virtual_function_blocking(parser, buffer_tostring(wb), &rc, cfg->data);

    buffer_free(wb);

    if(rc != DYNCFG_VFNC_RET_CFG_ACCEPTED)
        return SET_CONFIG_REJECTED;
    return SET_CONFIG_ACCEPTED;
}

enum set_config_result set_job_config_cb(void *usr_ctx, const char *plugin_name, const char *module_name, const char *job_name, dyncfg_config_t *cfg)
{
    PARSER *parser = usr_ctx;
    BUFFER *wb = buffer_create(CVF_MAX_LEN, NULL);

    buffer_strcat(wb, FUNCTION_NAME_SET_JOB_CONFIG);

    if (SERVING_STREAMING(parser))
        buffer_sprintf(wb, " %s", plugin_name);
    
    buffer_sprintf(wb, " %s %s", module_name, job_name);

    int rc;
    call_virtual_function_blocking(parser, buffer_tostring(wb), &rc, cfg->data);

    buffer_free(wb);

    if(rc != DYNCFG_VFNC_RET_CFG_ACCEPTED)
        return SET_CONFIG_REJECTED;
    return SET_CONFIG_ACCEPTED;
}

enum set_config_result delete_job_cb(void *usr_ctx, const char *plugin_name ,const char *module_name, const char *job_name)
{
    PARSER *parser = usr_ctx;
    BUFFER *wb = buffer_create(CVF_MAX_LEN, NULL);

    buffer_strcat(wb, FUNCTION_NAME_DELETE_JOB);

    if (SERVING_STREAMING(parser))
        buffer_sprintf(wb, " %s", plugin_name);

    buffer_sprintf(wb, " %s %s", module_name, job_name);

    int rc;
    call_virtual_function_blocking(parser, buffer_tostring(wb), &rc, NULL);

    buffer_free(wb);

    if(rc != DYNCFG_VFNC_RET_CFG_ACCEPTED)
        return SET_CONFIG_REJECTED;
    return SET_CONFIG_ACCEPTED;
}


static inline PARSER_RC pluginsd_register_plugin(char **words __maybe_unused, size_t num_words __maybe_unused, PARSER *parser __maybe_unused) {
    netdata_log_info("PLUGINSD: DYNCFG_ENABLE");

    if (unlikely (num_words != 2))
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_DYNCFG_ENABLE, "missing name parameter");

    struct configurable_plugin *cfg = callocz(1, sizeof(struct configurable_plugin));

    cfg->name = strdupz(words[1]);
    cfg->set_config_cb = set_plugin_config_cb;
    cfg->get_config_cb = get_plugin_config_cb;
    cfg->get_config_schema_cb = get_plugin_config_schema_cb;
    cfg->cb_usr_ctx = parser;

    const DICTIONARY_ITEM *di = register_plugin(parser->user.host->configurable_plugins, cfg, SERVING_PLUGINSD(parser));
    if (unlikely(di == NULL)) {
        freez(cfg->name);
        freez(cfg);
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_DYNCFG_ENABLE, "error registering plugin");
    }

    if (SERVING_PLUGINSD(parser)) {
        // this is optimization for pluginsd to avoid extra dictionary lookup
        // as we know which plugin is comunicating with us
        parser->user.cd->cfg_dict_item = di;
        parser->user.cd->configuration = cfg;
    } else {
        // register_plugin keeps the item acquired, so we need to release it
        dictionary_acquired_item_release(parser->user.host->configurable_plugins, di);
    }

    rrdpush_send_dyncfg_enable(parser->user.host, cfg->name);

    return PARSER_RC_OK;
}

#define LOG_MSG_SIZE (1024)
#define MODULE_NAME_IDX (SERVING_PLUGINSD(parser) ? 1 : 2)
#define MODULE_TYPE_IDX (SERVING_PLUGINSD(parser) ? 2 : 3)
static inline PARSER_RC pluginsd_register_module(char **words __maybe_unused, size_t num_words __maybe_unused, PARSER *parser __maybe_unused) {
    netdata_log_info("PLUGINSD: DYNCFG_REG_MODULE");

    size_t expected_num_words = SERVING_PLUGINSD(parser) ? 3 : 4;

    if (unlikely(num_words != expected_num_words)) {
        char log[LOG_MSG_SIZE + 1];
        snprintfz(log, LOG_MSG_SIZE, "expected %zu (got %zu) parameters: %smodule_name module_type", expected_num_words - 1, num_words - 1, SERVING_PLUGINSD(parser) ? "" : "plugin_name ");
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_DYNCFG_REGISTER_MODULE, log);
    }

    struct configurable_plugin *plug_cfg;
    const DICTIONARY_ITEM *di = NULL;
    if (SERVING_PLUGINSD(parser)) {
        plug_cfg = parser->user.cd->configuration;
        if (unlikely(plug_cfg == NULL))
            return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_DYNCFG_REGISTER_MODULE, "you have to enable dynamic configuration first using " PLUGINSD_KEYWORD_DYNCFG_ENABLE);
    } else {
        di = dictionary_get_and_acquire_item(parser->user.host->configurable_plugins, words[1]);
        if (unlikely(di == NULL))
            return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_DYNCFG_REGISTER_MODULE, "plugin not found");

        plug_cfg = (struct configurable_plugin *)dictionary_acquired_item_value(di);
    }

    struct module *mod = callocz(1, sizeof(struct module));

    mod->type = str2_module_type(words[MODULE_TYPE_IDX]);
    if (unlikely(mod->type == MOD_TYPE_UNKNOWN)) {
        freez(mod);
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_DYNCFG_REGISTER_MODULE, "unknown module type (allowed: job_array, single)");
    }

    mod->name = strdupz(words[MODULE_NAME_IDX]);

    mod->set_config_cb = set_module_config_cb;
    mod->get_config_cb = get_module_config_cb;
    mod->get_config_schema_cb = get_module_config_schema_cb;
    mod->config_cb_usr_ctx = parser;

    mod->get_job_config_cb = get_job_config_cb;
    mod->get_job_config_schema_cb = get_job_config_schema_cb;
    mod->set_job_config_cb = set_job_config_cb;
    mod->delete_job_cb = delete_job_cb;
    mod->job_config_cb_usr_ctx = parser;

    register_module(parser->user.host->configurable_plugins, plug_cfg, mod, SERVING_PLUGINSD(parser));

    if (di != NULL)
        dictionary_acquired_item_release(parser->user.host->configurable_plugins, di);

    rrdpush_send_dyncfg_reg_module(parser->user.host, plug_cfg->name, mod->name, mod->type);

    return PARSER_RC_OK;
}

static inline PARSER_RC pluginsd_register_job_common(char **words __maybe_unused, size_t num_words __maybe_unused, PARSER *parser __maybe_unused, const char *plugin_name) {
    if (atol(words[3]) < 0)
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_DYNCFG_REGISTER_JOB, "invalid flags");
    dyncfg_job_flg_t flags = atol(words[3]);
    if (SERVING_PLUGINSD(parser))
        flags |= JOB_FLG_PLUGIN_PUSHED;
    else
        flags |= JOB_FLG_STREAMING_PUSHED;

    enum job_type job_type = str2job_type(words[2]);
    if (job_type == JOB_TYPE_UNKNOWN)
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_DYNCFG_REGISTER_JOB, "unknown job type");
    if (SERVING_PLUGINSD(parser) && job_type == JOB_TYPE_USER)
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_DYNCFG_REGISTER_JOB, "plugins cannot push jobs of type \"user\" (this is allowed only in streaming)");

    if (register_job(parser->user.host->configurable_plugins, plugin_name, words[0], words[1], job_type, flags, 0)) // ignore existing is off as this is explicitly called register job
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_DYNCFG_REGISTER_JOB, "error registering job");

    rrdpush_send_dyncfg_reg_job(parser->user.host, plugin_name, words[0], words[1], job_type, flags);
    return PARSER_RC_OK;
}

static inline PARSER_RC pluginsd_register_job(char **words __maybe_unused, size_t num_words __maybe_unused, PARSER *parser __maybe_unused) {
    size_t expected_num_words = SERVING_PLUGINSD(parser) ? 5 : 6;

    if (unlikely(num_words != expected_num_words)) {
        char log[LOG_MSG_SIZE + 1];
        snprintfz(log, LOG_MSG_SIZE, "expected %zu (got %zu) parameters: %smodule_name job_name job_type", expected_num_words - 1, num_words - 1, SERVING_PLUGINSD(parser) ? "" : "plugin_name ");
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_DYNCFG_REGISTER_JOB, log);
    }

    if (SERVING_PLUGINSD(parser)) {
        return pluginsd_register_job_common(&words[1], num_words - 1, parser,  parser->user.cd->configuration->name);
    }
    return pluginsd_register_job_common(&words[2], num_words - 2, parser, words[1]);
}

static inline PARSER_RC pluginsd_job_status_common(char **words, size_t num_words, PARSER *parser, const char *plugin_name) {
    int state = str2i(words[3]);

    enum job_status status = str2job_state(words[2]);
    if (unlikely(SERVING_PLUGINSD(parser) && status == JOB_STATUS_UNKNOWN))
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_REPORT_JOB_STATUS, "unknown job status");

    char *message = NULL;
    if (num_words == 5)
        message = words[4];

    const DICTIONARY_ITEM *plugin_item;
    DICTIONARY *job_dict;
    const DICTIONARY_ITEM *job_item = report_job_status_acq_lock(parser->user.host->configurable_plugins, &plugin_item, &job_dict, plugin_name, words[0], words[1], status, state, message);

    if (job_item != NULL) {
        struct job *job = dictionary_acquired_item_value(job_item);
        rrdpush_send_job_status_update(parser->user.host, plugin_name, words[0], job);

        pthread_mutex_unlock(&job->lock);
        dictionary_acquired_item_release(job_dict, job_item);
        dictionary_acquired_item_release(parser->user.host->configurable_plugins, plugin_item);
    }

    return PARSER_RC_OK;
}

// job_status [plugin_name if streaming] <module_name> <job_name> <status_code> <state> [message]
static PARSER_RC pluginsd_job_status(char **words, size_t num_words, PARSER *parser) {
    if (SERVING_PLUGINSD(parser)) {
        if (unlikely(num_words != 5 && num_words != 6))
            return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_REPORT_JOB_STATUS, "expected 4 or 5 parameters: module_name, job_name, status_code, state, [optional: message]");
    } else {
        if (unlikely(num_words != 6 && num_words != 7))
            return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_REPORT_JOB_STATUS, "expected 5 or 6 parameters: plugin_name, module_name, job_name, status_code, state, [optional: message]");
    }

    if (SERVING_PLUGINSD(parser)) {
        return pluginsd_job_status_common(&words[1], num_words - 1, parser, parser->user.cd->configuration->name);
    }
    return pluginsd_job_status_common(&words[2], num_words - 2, parser, words[1]);
}

static PARSER_RC pluginsd_delete_job(char **words, size_t num_words, PARSER *parser) {
    // this can confuse a bit but there is a diference between KEYWORD_DELETE_JOB and actual delete_job function
    // they are of opossite direction
    if (num_words != 4)
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_DELETE_JOB, "expected 2 parameters: plugin_name, module_name, job_name");

    const char *plugin_name = get_word(words, num_words, 1);
    const char *module_name = get_word(words, num_words, 2);
    const char *job_name = get_word(words, num_words, 3);

    if (SERVING_STREAMING(parser))
        delete_job_pname(parser->user.host->configurable_plugins, plugin_name, module_name, job_name);

    // forward to parent if any
    rrdpush_send_job_deleted(parser->user.host, plugin_name, module_name, job_name);
    return PARSER_RC_OK;
}

static inline PARSER_RC streaming_claimed_id(char **words, size_t num_words, PARSER *parser)
{
    const char *host_uuid_str = get_word(words, num_words, 1);
    const char *claim_id_str = get_word(words, num_words, 2);

    if (!host_uuid_str || !claim_id_str) {
        netdata_log_error("Command CLAIMED_ID came malformed, uuid = '%s', claim_id = '%s'",
              host_uuid_str ? host_uuid_str : "[unset]",
              claim_id_str ? claim_id_str : "[unset]");
        return PARSER_RC_ERROR;
    }

    uuid_t uuid;
    RRDHOST *host = parser->user.host;

    // We don't need the parsed UUID
    // just do it to check the format
    if(uuid_parse(host_uuid_str, uuid)) {
        netdata_log_error("1st parameter (host GUID) to CLAIMED_ID command is not valid GUID. Received: \"%s\".", host_uuid_str);
        return PARSER_RC_ERROR;
    }
    if(uuid_parse(claim_id_str, uuid) && strcmp(claim_id_str, "NULL") != 0) {
        netdata_log_error("2nd parameter (Claim ID) to CLAIMED_ID command is not valid GUID. Received: \"%s\".", claim_id_str);
        return PARSER_RC_ERROR;
    }

    if(strcmp(host_uuid_str, host->machine_guid) != 0) {
        netdata_log_error("Claim ID is for host \"%s\" but it came over connection for \"%s\"", host_uuid_str, host->machine_guid);
        return PARSER_RC_OK; //the message is OK problem must be somewhere else
    }

    rrdhost_aclk_state_lock(host);

    if (host->aclk_state.claimed_id)
        freez(host->aclk_state.claimed_id);

    host->aclk_state.claimed_id = strcmp(claim_id_str, "NULL") ? strdupz(claim_id_str) : NULL;

    rrdhost_aclk_state_unlock(host);

    rrdhost_flag_set(host, RRDHOST_FLAG_METADATA_CLAIMID |RRDHOST_FLAG_METADATA_UPDATE);

    rrdpush_send_claimed_id(host);

    return PARSER_RC_OK;
}

// ----------------------------------------------------------------------------

static inline bool buffered_reader_read(struct buffered_reader *reader, int fd) {
#ifdef NETDATA_INTERNAL_CHECKS
    if(reader->read_buffer[reader->read_len] != '\0')
        fatal("%s(): read_buffer does not start with zero", __FUNCTION__ );
#endif

    ssize_t bytes_read = read(fd, reader->read_buffer + reader->read_len, sizeof(reader->read_buffer) - reader->read_len - 1);
    if(unlikely(bytes_read <= 0))
        return false;

    reader->read_len += bytes_read;
    reader->read_buffer[reader->read_len] = '\0';

    return true;
}

static inline bool buffered_reader_read_timeout(struct buffered_reader *reader, int fd, int timeout_ms) {
    errno = 0;
    struct pollfd fds[1];

    fds[0].fd = fd;
    fds[0].events = POLLIN;

    int ret = poll(fds, 1, timeout_ms);

    if (ret > 0) {
        /* There is data to read */
        if (fds[0].revents & POLLIN)
            return buffered_reader_read(reader, fd);

        else if(fds[0].revents & POLLERR) {
            netdata_log_error("PARSER: read failed: POLLERR.");
            return false;
        }
        else if(fds[0].revents & POLLHUP) {
            netdata_log_error("PARSER: read failed: POLLHUP.");
            return false;
        }
        else if(fds[0].revents & POLLNVAL) {
            netdata_log_error("PARSER: read failed: POLLNVAL.");
            return false;
        }

        netdata_log_error("PARSER: poll() returned positive number, but POLLIN|POLLERR|POLLHUP|POLLNVAL are not set.");
        return false;
    }
    else if (ret == 0) {
        netdata_log_error("PARSER: timeout while waiting for data.");
        return false;
    }

    netdata_log_error("PARSER: poll() failed with code %d.", ret);
    return false;
}

void pluginsd_process_thread_cleanup(void *ptr) {
    PARSER *parser = (PARSER *)ptr;

    pluginsd_cleanup_v2(parser);
    pluginsd_host_define_cleanup(parser);

    rrd_collector_finished();

    parser_destroy(parser);
}

inline size_t pluginsd_process(RRDHOST *host, struct plugind *cd, FILE *fp_plugin_input, FILE *fp_plugin_output, int trust_durations)
{
    int enabled = cd->unsafe.enabled;

    if (!fp_plugin_input || !fp_plugin_output || !enabled) {
        cd->unsafe.enabled = 0;
        return 0;
    }

    if (unlikely(fileno(fp_plugin_input) == -1)) {
        netdata_log_error("input file descriptor given is not a valid stream");
        cd->serial_failures++;
        return 0;
    }

    if (unlikely(fileno(fp_plugin_output) == -1)) {
        netdata_log_error("output file descriptor given is not a valid stream");
        cd->serial_failures++;
        return 0;
    }

    clearerr(fp_plugin_input);
    clearerr(fp_plugin_output);

    PARSER *parser;
    {
        PARSER_USER_OBJECT user = {
                .enabled = cd->unsafe.enabled,
                .host = host,
                .cd = cd,
                .trust_durations = trust_durations
        };

        // fp_plugin_output = our input; fp_plugin_input = our output
        parser = parser_init(&user, fp_plugin_output, fp_plugin_input, -1, PARSER_INPUT_SPLIT, NULL);
    }

    pluginsd_keywords_init(parser, PARSER_INIT_PLUGINSD);

    rrd_collector_started();

    size_t count = 0;

    // this keeps the parser with its current value
    // so, parser needs to be allocated before pushing it
    netdata_thread_cleanup_push(pluginsd_process_thread_cleanup, parser);

    buffered_reader_init(&parser->reader);
    BUFFER *buffer = buffer_create(sizeof(parser->reader.read_buffer) + 2, NULL);
    while(likely(service_running(SERVICE_COLLECTORS))) {
        if (unlikely(!buffered_reader_next_line(&parser->reader, buffer))) {
            if(unlikely(!buffered_reader_read_timeout(&parser->reader, fileno((FILE *)parser->fp_input), 2 * 60 * MSEC_PER_SEC)))
                break;

            continue;
        }

        if(unlikely(parser_action(parser,  buffer->buffer)))
            break;

        buffer->len = 0;
        buffer->buffer[0] = '\0';
    }
    buffer_free(buffer);

    cd->unsafe.enabled = parser->user.enabled;
    count = parser->user.data_collections_count;

    if (likely(count)) {
        cd->successful_collections += count;
        cd->serial_failures = 0;
    }
    else
        cd->serial_failures++;

    // free parser with the pop function
    netdata_thread_cleanup_pop(1);

    return count;
}

void pluginsd_keywords_init(PARSER *parser, PARSER_REPERTOIRE repertoire) {
    parser_init_repertoire(parser, repertoire);

    if (repertoire & (PARSER_INIT_PLUGINSD | PARSER_INIT_STREAMING))
        inflight_functions_init(parser);
}

PARSER *parser_init(struct parser_user_object *user, FILE *fp_input, FILE *fp_output, int fd,
                    PARSER_INPUT_TYPE flags, void *ssl __maybe_unused) {
    PARSER *parser;

    parser = callocz(1, sizeof(*parser));
    if(user)
        parser->user = *user;
    parser->fd = fd;
    parser->fp_input = fp_input;
    parser->fp_output = fp_output;
#ifdef ENABLE_HTTPS
    parser->ssl_output = ssl;
#endif
    parser->flags = flags;

    spinlock_init(&parser->writer.spinlock);
    return parser;
}

PARSER_RC parser_execute(PARSER *parser, PARSER_KEYWORD *keyword, char **words, size_t num_words) {
    switch(keyword->id) {
        case 1:
            return pluginsd_set_v2(words, num_words, parser);

        case 2:
            return pluginsd_begin_v2(words, num_words, parser);

        case 3:
            return pluginsd_end_v2(words, num_words, parser);

        case 11:
            return pluginsd_set(words, num_words, parser);

        case 12:
            return pluginsd_begin(words, num_words, parser);

        case 13:
            return pluginsd_end(words, num_words, parser);

        case 21:
            return pluginsd_replay_set(words, num_words, parser);

        case 22:
            return pluginsd_replay_begin(words, num_words, parser);

        case 23:
            return pluginsd_replay_rrddim_collection_state(words, num_words, parser);

        case 24:
            return pluginsd_replay_rrdset_collection_state(words, num_words, parser);

        case 25:
            return pluginsd_replay_end(words, num_words, parser);

        case 31:
            return pluginsd_dimension(words, num_words, parser);

        case 32:
            return pluginsd_chart(words, num_words, parser);

        case 33:
            return pluginsd_chart_definition_end(words, num_words, parser);

        case 34:
            return pluginsd_clabel(words, num_words, parser);

        case 35:
            return pluginsd_clabel_commit(words, num_words, parser);

        case 41:
            return pluginsd_function(words, num_words, parser);

        case 42:
            return pluginsd_function_result_begin(words, num_words, parser);

        case 51:
            return pluginsd_label(words, num_words, parser);

        case 52:
            return pluginsd_overwrite(words, num_words, parser);

        case 53:
            return pluginsd_variable(words, num_words, parser);

        case 61:
            return streaming_claimed_id(words, num_words, parser);

        case 71:
            return pluginsd_host(words, num_words, parser);

        case 72:
            return pluginsd_host_define(words, num_words, parser);

        case 73:
            return pluginsd_host_define_end(words, num_words, parser);

        case 74:
            return pluginsd_host_labels(words, num_words, parser);

        case 97:
            return pluginsd_flush(words, num_words, parser);

        case 98:
            return pluginsd_disable(words, num_words, parser);

        case 99:
            return pluginsd_exit(words, num_words, parser);

        case 101:
            return pluginsd_register_plugin(words, num_words, parser);

        case 102:
            return pluginsd_register_module(words, num_words, parser);

        case 103:
            return pluginsd_register_job(words, num_words, parser);

        case 110:
            return pluginsd_job_status(words, num_words, parser);
        
        case 111:
            return pluginsd_delete_job(words, num_words, parser);

        default:
            fatal("Unknown keyword '%s' with id %zu", keyword->keyword, keyword->id);
    }
}

#include "gperf-hashtable.h"

void parser_init_repertoire(PARSER *parser, PARSER_REPERTOIRE repertoire) {
    parser->repertoire = repertoire;

    for(size_t i = GPERF_PARSER_MIN_HASH_VALUE ; i <= GPERF_PARSER_MAX_HASH_VALUE ;i++) {
        if(gperf_keywords[i].keyword && *gperf_keywords[i].keyword && (parser->repertoire & gperf_keywords[i].repertoire))
            worker_register_job_name(gperf_keywords[i].worker_job_id, gperf_keywords[i].keyword);
    }
}

static void parser_destroy_dyncfg(PARSER *parser) {
    if (parser->user.cd != NULL && parser->user.cd->configuration != NULL) {
        unregister_plugin(parser->user.host->configurable_plugins, parser->user.cd->cfg_dict_item);
        parser->user.cd->configuration = NULL;
    } else if (parser->user.host != NULL && SERVING_STREAMING(parser) && parser->user.host != localhost){
        dictionary_flush(parser->user.host->configurable_plugins);
    }
}

void parser_destroy(PARSER *parser) {
    if (unlikely(!parser))
        return;

    parser_destroy_dyncfg(parser);

    dictionary_destroy(parser->inflight.functions);
    freez(parser);
}

int pluginsd_parser_unittest(void) {
    PARSER *p = parser_init(NULL, NULL, NULL, -1, PARSER_INPUT_SPLIT, NULL);
    pluginsd_keywords_init(p, PARSER_INIT_PLUGINSD | PARSER_INIT_STREAMING);

    char *lines[] = {
            "BEGIN2 abcdefghijklmnopqr 123",
            "SET2 abcdefg 0x12345678 0 0",
            "SET2 hijklmnoqr 0x12345678 0 0",
            "SET2 stuvwxyz 0x12345678 0 0",
            "END2",
            NULL,
    };

    char *words[PLUGINSD_MAX_WORDS];
    size_t iterations = 1000000;
    size_t count = 0;
    char input[PLUGINSD_LINE_MAX + 1];

    usec_t started = now_realtime_usec();
    while(--iterations) {
        for(size_t line = 0; lines[line] ;line++) {
            strncpyz(input, lines[line], PLUGINSD_LINE_MAX);
            size_t num_words = quoted_strings_splitter_pluginsd(input, words, PLUGINSD_MAX_WORDS);
            const char *command = get_word(words, num_words, 0);
            PARSER_KEYWORD *keyword = parser_find_keyword(p, command);
            if(unlikely(!keyword))
                fatal("Cannot parse the line '%s'", lines[line]);
            count++;
        }
    }
    usec_t ended = now_realtime_usec();

    netdata_log_info("Parsed %zu lines in %0.2f secs, %0.2f klines/sec", count,
         (double)(ended - started) / (double)USEC_PER_SEC,
         (double)count / ((double)(ended - started) / (double)USEC_PER_SEC) / 1000.0);

    parser_destroy(p);
    return 0;
}

// SPDX-License-Identifier: GPL-3.0-or-later

#include "pluginsd_internals.h"
#include "streaming/stream-replication-receiver.h"
#include "database/rrddim-collection.h"

static inline PARSER_RC pluginsd_set(char **words, size_t num_words, PARSER *parser) {
    int idx = 1;
    ssize_t slot = pluginsd_parse_rrd_slot(words, num_words);
    if(slot >= 0) idx++;

    char *dimension = get_word(words, num_words, idx++);
    char *value = get_word(words, num_words, idx++);

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_SET);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    RRDSET *st = pluginsd_require_scope_chart(parser, PLUGINSD_KEYWORD_SET, PLUGINSD_KEYWORD_CHART);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    RRDDIM *rd = pluginsd_acquire_dimension(host, st, dimension, slot, PLUGINSD_KEYWORD_SET);
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
    int idx = 1;
    ssize_t slot = pluginsd_parse_rrd_slot(words, num_words);
    if(slot >= 0) idx++;

    char *id = get_word(words, num_words, idx++);
    char *microseconds_txt = get_word(words, num_words, idx++);

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_BEGIN);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    RRDSET *st = pluginsd_rrdset_cache_get_from_slot(parser, host, id, slot, PLUGINSD_KEYWORD_BEGIN);
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
                       (long long unsigned)microseconds
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
    char *tv_sec = get_word(words, num_words, 1);
    char *tv_usec = get_word(words, num_words, 2);
    char *pending_rrdset_next = get_word(words, num_words, 3);

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_END);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    RRDSET *st = pluginsd_require_scope_chart(parser, PLUGINSD_KEYWORD_END, PLUGINSD_KEYWORD_BEGIN);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    if (unlikely(rrdset_flag_check(st, RRDSET_FLAG_DEBUG)))
        netdata_log_debug(D_PLUGINSD, "requested an END on chart '%s'", rrdset_id(st));

    pluginsd_clear_scope_chart(parser, PLUGINSD_KEYWORD_END);
    parser->user.data_collections_count++;

    struct timeval tv = {
        .tv_sec  = (tv_sec  && *tv_sec)  ? str2ll(tv_sec,  NULL) : 0,
        .tv_usec = (tv_usec && *tv_usec) ? str2ll(tv_usec, NULL) : 0
    };
    
    if(!tv.tv_sec)
        now_realtime_timeval(&tv);

    rrdset_timed_done(st, tv, pending_rrdset_next && *pending_rrdset_next ? true : false);

    return PARSER_RC_OK;
}

static void pluginsd_host_define_cleanup(PARSER *parser) {
    string_freez(parser->user.host_define.hostname);
    rrdlabels_destroy(parser->user.host_define.rrdlabels);

    parser->user.host_define.hostname = NULL;
    parser->user.host_define.rrdlabels = NULL;
    parser->user.host_define.parsing_host = false;
}

static inline bool pluginsd_validate_machine_guid(const char *guid, nd_uuid_t *uuid, char *output) {
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

static inline void pluginsd_update_host_ephemerality(RRDHOST *host) {
    char value[64];
    rrdlabels_get_value_strcpyz(host->rrdlabels, value, sizeof(value), HOST_LABEL_IS_EPHEMERAL);
    if(value[0] && inicfg_test_boolean_value(value)) {
        rrdhost_option_set(host, RRDHOST_OPTION_EPHEMERAL_HOST);
        strncpyz(value, "true", sizeof(value) - 1);
    }
    else {
        rrdhost_option_clear(host, RRDHOST_OPTION_EPHEMERAL_HOST);
        strncpyz(value, "false", sizeof(value) - 1);
    }

    if(!rrdlabels_exist(host->rrdlabels, HOST_LABEL_IS_EPHEMERAL))
        rrdlabels_add(host->rrdlabels, HOST_LABEL_IS_EPHEMERAL, value, RRDLABEL_SRC_CONFIG);
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
        program_name,
        NETDATA_VERSION,
        nd_profile.update_every,
        default_rrd_history_entries,
        default_rrd_memory_mode,
        health_plugin_enabled(),
        stream_send.enabled,
        stream_send.parents.destination,
        stream_send.api_key,
        stream_send.send_charts_matching,
        stream_receive.replication.enabled,
        stream_receive.replication.period,
        stream_receive.replication.step,
        rrdhost_system_info_from_host_labels(parser->user.host_define.rrdlabels),
        false);

    rrdhost_option_set(host, RRDHOST_OPTION_VIRTUAL_HOST);
    rrdhost_flag_set(host, RRDHOST_FLAG_COLLECTOR_ONLINE);
    object_state_activate_if_not_activated(&host->state_id);
    ml_host_start(host);
    dyncfg_host_init(host);
    pulse_host_status(host, 0, 0); // this will detect the receiver status

    if(host->rrdlabels) {
        rrdlabels_migrate_to_these(host->rrdlabels, parser->user.host_define.rrdlabels);
    }
    else {
        host->rrdlabels = parser->user.host_define.rrdlabels;
        parser->user.host_define.rrdlabels = NULL;
    }

    pluginsd_update_host_ephemerality(host);
    pluginsd_host_define_cleanup(parser);

    parser->user.host = host;
    pluginsd_clear_scope_chart(parser, PLUGINSD_KEYWORD_HOST_DEFINE_END);

    rrdhost_flag_clear(host, RRDHOST_FLAG_ORPHAN);
    rrdcontext_host_child_connected(host);
    if (host->aclk_config)
        aclk_queue_node_info(host, true);
    else
        schedule_node_state_update(host, 100);

    return PARSER_RC_OK;
}

static inline PARSER_RC pluginsd_host(char **words, size_t num_words, PARSER *parser) {
    char *guid = get_word(words, num_words, 1);

    if(!guid || !*guid || strcmp(guid, "localhost") == 0) {
        parser->user.host = localhost;
        return PARSER_RC_OK;
    }

    nd_uuid_t uuid;
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

    int idx = 1;
    ssize_t slot = pluginsd_parse_rrd_slot(words, num_words);
    if(slot >= 0) idx++;

    char *type = get_word(words, num_words, idx++);
    char *name = get_word(words, num_words, idx++);
    char *title = get_word(words, num_words, idx++);
    char *units = get_word(words, num_words, idx++);
    char *family = get_word(words, num_words, idx++);
    char *context = get_word(words, num_words, idx++);
    char *chart = get_word(words, num_words, idx++);
    char *priority_s = get_word(words, num_words, idx++);
    char *update_every_s = get_word(words, num_words, idx++);
    char *options = get_word(words, num_words, idx++);
    char *plugin = get_word(words, num_words, idx++);
    char *module = get_word(words, num_words, idx++);

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
        (plugin && *plugin) ? plugin : string2str(parser->user.cd->filename),
        module, priority, update_every,
        chart_type);

    bool obsolete = false;
    if (likely(st)) {
        if (options && *options) {
            if (strstr(options, "obsolete")) {
                rrdset_is_obsolete___safe_from_collector_thread(st);
                obsolete = true;
            }
            else
                rrdset_isnot_obsolete___safe_from_collector_thread(st);

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
            rrdset_isnot_obsolete___safe_from_collector_thread(st);
            rrdset_flag_clear(st, RRDSET_FLAG_STORE_FIRST);
        }

        if(!pluginsd_set_scope_chart(parser, st, PLUGINSD_KEYWORD_CHART))
            return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

        pluginsd_rrdset_cache_put_to_slot(parser, st, slot, obsolete);
    }
    else
        pluginsd_clear_scope_chart(parser, PLUGINSD_KEYWORD_CHART);

    return PARSER_RC_OK;
}

static inline PARSER_RC pluginsd_dimension(char **words, size_t num_words, PARSER *parser) {
    int idx = 1;
    ssize_t slot = pluginsd_parse_rrd_slot(words, num_words);
    if(slot >= 0) idx++;

    char *id = get_word(words, num_words, idx++);
    char *name = get_word(words, num_words, idx++);
    char *algorithm = get_word(words, num_words, idx++);
    char *multiplier_s = get_word(words, num_words, idx++);
    char *divisor_s = get_word(words, num_words, idx++);
    char *options = get_word(words, num_words, idx++);

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_DIMENSION);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    RRDSET *st = pluginsd_require_scope_chart(parser, PLUGINSD_KEYWORD_DIMENSION, PLUGINSD_KEYWORD_CHART);
    if(!st) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    if (unlikely(!id || !*id))
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
    if (unlikely(!rd))
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_DIMENSION, "failed to create dimension");

    int unhide_dimension = 1;

    rrddim_option_clear(rd, RRDDIM_OPTION_DONT_DETECT_RESETS_OR_OVERFLOWS);
    bool obsolete = false;
    if (options && *options) {
        if (strstr(options, "obsolete") != NULL) {
            obsolete = true;
            rrddim_is_obsolete___safe_from_collector_thread(st, rd);
        }
        else
            rrddim_isnot_obsolete___safe_from_collector_thread(st, rd);

        unhide_dimension = !strstr(options, "hidden");

        if (strstr(options, "noreset") != NULL)
            rrddim_option_set(rd, RRDDIM_OPTION_DONT_DETECT_RESETS_OR_OVERFLOWS);
        if (strstr(options, "nooverflow") != NULL)
            rrddim_option_set(rd, RRDDIM_OPTION_DONT_DETECT_RESETS_OR_OVERFLOWS);
    }
    else
        rrddim_isnot_obsolete___safe_from_collector_thread(st, rd);

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

    pluginsd_rrddim_put_to_slot(parser, st, rd, slot, obsolete);

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
        const RRDVAR_ACQUIRED *rva = rrdvar_host_variable_add_and_acquire(host, name);
        if (rva) {
            rrdvar_host_variable_set(host, rva, v);
            rrdvar_host_variable_release(host, rva);
        }
        else
            netdata_log_error("PLUGINSD: 'host:%s' cannot find/create HOST VARIABLE '%s'",
                              rrdhost_hostname(host),
                              name);
    } else {
        const RRDVAR_ACQUIRED *rsa = rrdvar_chart_variable_add_and_acquire(st, name);
        if (rsa) {
            rrdvar_chart_variable_set(st, rsa, v);
            rrdvar_chart_variable_release(st, rsa);
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
    pluginsd_update_host_ephemerality(host);

    if(!rrdlabels_exist(host->rrdlabels, "_os"))
        rrdlabels_add(host->rrdlabels, "_os", string2str(host->os), RRDLABEL_SRC_AUTO);

    if(!rrdlabels_exist(host->rrdlabels, "_hostname"))
        rrdlabels_add(host->rrdlabels, "_hostname", string2str(host->hostname), RRDLABEL_SRC_AUTO);

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
    rrdset_metadata_updated(st);

    parser->user.chart_rrdlabels_linked_temporarily = NULL;
    return PARSER_RC_OK;
}

static ALWAYS_INLINE PARSER_RC pluginsd_begin_v2(char **words, size_t num_words, PARSER *parser) {
    timing_init();

    int idx = 1;
    ssize_t slot = pluginsd_parse_rrd_slot(words, num_words);
    if(slot >= 0) idx++;

    char *id = get_word(words, num_words, idx++);
    char *update_every_str = get_word(words, num_words, idx++);
    char *end_time_str = get_word(words, num_words, idx++);
    char *wall_clock_time_str = get_word(words, num_words, idx++);

    if(unlikely(!id || !update_every_str || !end_time_str || !wall_clock_time_str))
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_BEGIN_V2, "missing parameters");

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_BEGIN_V2);
    if(unlikely(!host)) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    timing_step(TIMING_STEP_BEGIN2_PREPARE);

    RRDSET *st = pluginsd_rrdset_cache_get_from_slot(parser, host, id, slot, PLUGINSD_KEYWORD_BEGIN_V2);

    if(unlikely(!st)) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    if(!pluginsd_set_scope_chart(parser, st, PLUGINSD_KEYWORD_BEGIN_V2))
        return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    if(unlikely(rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE)))
        rrdset_isnot_obsolete___safe_from_collector_thread(st);

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

    rrdset_data_collection_lock(parser);

    parser->user.v2.update_every = update_every;
    parser->user.v2.end_time = end_time;
    parser->user.v2.wall_clock_time = wall_clock_time;
    parser->user.v2.ml_locked = ml_chart_update_begin(st);

    timing_step(TIMING_STEP_BEGIN2_ML);

    // ------------------------------------------------------------------------
    // propagate it forward in v2

    if(!parser->user.v2.stream_buffer.wb && rrdhost_has_stream_sender_enabled(st->rrdhost))
        parser->user.v2.stream_buffer = stream_send_metrics_init(parser->user.st, wall_clock_time);

    if(parser->user.v2.stream_buffer.v2 && parser->user.v2.stream_buffer.wb) {
        // check receiver capabilities
        bool can_copy = stream_has_capability(&parser->user, STREAM_CAP_IEEE754) == stream_has_capability(&parser->user.v2.stream_buffer, STREAM_CAP_IEEE754);

        // check sender capabilities
        bool with_slots = stream_has_capability(&parser->user.v2.stream_buffer, STREAM_CAP_SLOTS) ? true : false;
        NUMBER_ENCODING integer_encoding = stream_has_capability(&parser->user.v2.stream_buffer, STREAM_CAP_IEEE754) ? NUMBER_ENCODING_BASE64 : NUMBER_ENCODING_HEX;

        BUFFER *wb = parser->user.v2.stream_buffer.wb;

        buffer_need_bytes(wb, 1024);

        if(unlikely(parser->user.v2.stream_buffer.begin_v2_added))
            buffer_fast_strcat(wb, PLUGINSD_KEYWORD_END_V2 "\n", sizeof(PLUGINSD_KEYWORD_END_V2) - 1 + 1);

        buffer_fast_strcat(wb, PLUGINSD_KEYWORD_BEGIN_V2, sizeof(PLUGINSD_KEYWORD_BEGIN_V2) - 1);

        if(with_slots) {
            buffer_fast_strcat(wb, " "PLUGINSD_KEYWORD_SLOT":", sizeof(PLUGINSD_KEYWORD_SLOT) - 1 + 2);
            buffer_print_uint64_encoded(wb, integer_encoding, st->stream.snd.chart_slot);
        }

        buffer_fast_strcat(wb, " '", 2);
        buffer_fast_strcat(wb, rrdset_id(st), string_strlen(st->id));
        buffer_fast_strcat(wb, "' ", 2);

        if(can_copy)
            buffer_strcat(wb, update_every_str);
        else
            buffer_print_uint64_encoded(wb, integer_encoding, update_every);

        buffer_fast_strcat(wb, " ", 1);

        if(can_copy)
            buffer_strcat(wb, end_time_str);
        else
            buffer_print_uint64_encoded(wb, integer_encoding, end_time);

        buffer_fast_strcat(wb, " ", 1);

        if(can_copy)
            buffer_strcat(wb, wall_clock_time_str);
        else
            buffer_print_uint64_encoded(wb, integer_encoding, wall_clock_time);

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

    // these are only needed for db mode RAM, ALLOC
    st->db.current_entry++;
    if(st->db.current_entry >= st->db.entries)
        st->db.current_entry -= st->db.entries;

    timing_step(TIMING_STEP_BEGIN2_STORE);

    return PARSER_RC_OK;
}

static ALWAYS_INLINE PARSER_RC pluginsd_set_v2(char **words, size_t num_words, PARSER *parser) {
    timing_init();

    int idx = 1;
    ssize_t slot = pluginsd_parse_rrd_slot(words, num_words);
    if(slot >= 0) idx++;

    char *dimension = get_word(words, num_words, idx++);
    char *collected_str = get_word(words, num_words, idx++);
    char *value_str = get_word(words, num_words, idx++);
    char *flags_str = get_word(words, num_words, idx++);

    if(unlikely(!dimension || !collected_str || !value_str || !flags_str))
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_SET_V2, "missing parameters");

    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_SET_V2);
    if(unlikely(!host)) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    RRDSET *st = pluginsd_require_scope_chart(parser, PLUGINSD_KEYWORD_SET_V2, PLUGINSD_KEYWORD_BEGIN_V2);
    if(unlikely(!st)) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    timing_step(TIMING_STEP_SET2_PREPARE);

    RRDDIM *rd = pluginsd_acquire_dimension(host, st, dimension, slot, PLUGINSD_KEYWORD_SET_V2);
    if(unlikely(!rd)) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    st->pluginsd.set = true;

    if(unlikely(rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE | RRDDIM_FLAG_ARCHIVED)))
        rrddim_isnot_obsolete___safe_from_collector_thread(st, rd);

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

    if(stream_has_capability(&parser->user, STREAM_CAP_ML_MODELS)) {
        // we receive anomaly information, no need for prediction on this node
        if (unlikely(!netdata_double_isnumber(value) || (flags == SN_EMPTY_SLOT))) {
            value = NAN;
            flags = SN_EMPTY_SLOT;
        }

        if(parser->user.v2.ml_locked)
            ml_dimension_received_anomaly(rd, !(flags & SN_FLAG_NOT_ANOMALOUS));
    }
    else {
        // we don't receive anomaly information, we need to run prediction on this node
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
    }

    timing_step(TIMING_STEP_SET2_ML);

    // ------------------------------------------------------------------------
    // propagate it forward in v2

    if(parser->user.v2.stream_buffer.v2 && parser->user.v2.stream_buffer.begin_v2_added && parser->user.v2.stream_buffer.wb) {
        // check if receiver and sender have the same number parsing capabilities
        bool can_copy = stream_has_capability(&parser->user, STREAM_CAP_IEEE754) == stream_has_capability(&parser->user.v2.stream_buffer, STREAM_CAP_IEEE754);

        // check the sender capabilities
        bool with_slots = stream_has_capability(&parser->user.v2.stream_buffer, STREAM_CAP_SLOTS) ? true : false;
        NUMBER_ENCODING integer_encoding = stream_has_capability(&parser->user.v2.stream_buffer, STREAM_CAP_IEEE754) ? NUMBER_ENCODING_BASE64 : NUMBER_ENCODING_HEX;
        NUMBER_ENCODING doubles_encoding = stream_has_capability(&parser->user.v2.stream_buffer, STREAM_CAP_IEEE754) ? NUMBER_ENCODING_BASE64 : NUMBER_ENCODING_DECIMAL;

        BUFFER *wb = parser->user.v2.stream_buffer.wb;
        buffer_need_bytes(wb, 1024);
        buffer_fast_strcat(wb, PLUGINSD_KEYWORD_SET_V2, sizeof(PLUGINSD_KEYWORD_SET_V2) - 1);

        if(with_slots) {
            buffer_fast_strcat(wb, " "PLUGINSD_KEYWORD_SLOT":", sizeof(PLUGINSD_KEYWORD_SLOT) - 1 + 2);
            buffer_print_uint64_encoded(wb, integer_encoding, rd->stream.snd.dim_slot);
        }

        buffer_fast_strcat(wb, " '", 2);
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

static ALWAYS_INLINE PARSER_RC pluginsd_end_v2(char **words __maybe_unused, size_t num_words __maybe_unused, PARSER *parser) {
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
        stream_send_rrdset_metrics_v1(&parser->user.v2.stream_buffer, st);

    timing_step(TIMING_STEP_END2_PUSH_V1);

    // ------------------------------------------------------------------------
    // unblock data collection

    rrdset_previous_scope_chart_unlock(parser, PLUGINSD_KEYWORD_END_V2, false);
    rrdcontext_collected_rrdset(st);
    store_metric_collection_completed();

    timing_step(TIMING_STEP_END2_RRDSET);

    // ------------------------------------------------------------------------
    // propagate it forward

    stream_send_rrdset_metrics_finished(&parser->user.v2.stream_buffer, st);

    timing_step(TIMING_STEP_END2_PROPAGATE);

    // ------------------------------------------------------------------------
    // cleanup RRDSET / RRDDIM

    if(likely(st->pluginsd.dims_with_slots)) {
        for(size_t i = 0; i < st->pluginsd.size ;i++) {
            RRDDIM *rd = st->pluginsd.prd_array[i].rd;

            if(!rd)
                continue;

            rd->collector.calculated_value = 0;
            rd->collector.collected_value = 0;
            rrddim_clear_updated(rd);
        }
    }
    else {
        RRDDIM *rd;
        rrddim_foreach_read(rd, st){
            rd->collector.calculated_value = 0;
            rd->collector.collected_value = 0;
            rrddim_clear_updated(rd);
        }
        rrddim_foreach_done(rd);
    }

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

static void pluginsd_json_stream_paths(PARSER *parser, void *action_data __maybe_unused) {
    stream_path_set_from_json(parser->user.host, buffer_tostring(parser->defer.response), false);
    buffer_free(parser->defer.response);
}

static void pluginsd_json_ml_model(PARSER *parser, void *action_data __maybe_unused) {
    ml_model_received_from_child(parser->user.host, buffer_tostring(parser->defer.response));
    buffer_free(parser->defer.response);
}

static void pluginsd_json_dev_null(PARSER *parser, void *action_data __maybe_unused) {
    buffer_free(parser->defer.response);
}

static PARSER_RC pluginsd_json(char **words __maybe_unused, size_t num_words __maybe_unused, PARSER *parser) {
    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_JSON);
    if(!host) return PLUGINSD_DISABLE_PLUGIN(parser, NULL, NULL);

    char *keyword = get_word(words, num_words, 1);

    parser->defer.response = buffer_create(0, NULL);
    parser->defer.end_keyword = PLUGINSD_KEYWORD_JSON_END;
    parser->defer.action = pluginsd_json_dev_null;
    parser->defer.action_data = NULL;
    parser->flags |= PARSER_DEFER_UNTIL_KEYWORD;

    if(strcmp(keyword, PLUGINSD_KEYWORD_JSON_CMD_STREAM_PATH) == 0)
        parser->defer.action = pluginsd_json_stream_paths;
    else if(strcmp(keyword, PLUGINSD_KEYWORD_JSON_CMD_ML_MODEL) == 0)
        parser->defer.action = pluginsd_json_ml_model;
    else
        netdata_log_error("PLUGINSD: invalid JSON payload keyword '%s'", keyword);

    return PARSER_RC_OK;
}

PARSER_RC stream_receiver_pluginsd_claimed_id(char **words, size_t num_words, PARSER *parser);

// ----------------------------------------------------------------------------

void pluginsd_cleanup_v2(PARSER *parser) {
    // this is called when the thread is stopped while processing
    pluginsd_clear_scope_chart(parser, "THREAD CLEANUP");
}

void pluginsd_process_cleanup(PARSER *parser) {
    if(!parser) return;

    pluginsd_cleanup_v2(parser);
    pluginsd_host_define_cleanup(parser);

    parser_destroy(parser);
}

void pluginsd_process_thread_cleanup(void *pptr) {
    PARSER *parser = CLEANUP_FUNCTION_GET_PTR(pptr);
    pluginsd_process_cleanup(parser);
    rrd_collector_finished();
}

bool parser_reconstruct_node(BUFFER *wb, void *ptr) {
    PARSER *parser = ptr;
    if(!parser || !parser->user.host)
        return false;

    buffer_strcat(wb, rrdhost_hostname(parser->user.host));
    return true;
}

bool parser_reconstruct_instance(BUFFER *wb, void *ptr) {
    PARSER *parser = ptr;
    if(!parser || !parser->user.st)
        return false;

    buffer_strcat(wb, rrdset_name(parser->user.st));
    return true;
}

bool parser_reconstruct_context(BUFFER *wb, void *ptr) {
    PARSER *parser = ptr;
    if(!parser || !parser->user.st)
        return false;

    buffer_strcat(wb, string2str(parser->user.st->context));
    return true;
}

inline size_t pluginsd_process(RRDHOST *host, struct plugind *cd, int fd_input, int fd_output, int trust_durations)
{
    int enabled = cd->unsafe.enabled;

    if (fd_input == -1 || fd_output == -1 || !enabled) {
        cd->unsafe.enabled = 0;
        return 0;
    }

    PARSER *parser;
    {
        PARSER_USER_OBJECT user = {
                .enabled = cd->unsafe.enabled,
                .host = host,
                .cd = cd,
                .trust_durations = trust_durations
        };

        parser = parser_init(&user, fd_input, fd_output, PARSER_INPUT_SPLIT, NULL);
    }

    pluginsd_keywords_init(parser, PARSER_INIT_PLUGINSD);

    rrd_collector_started();

    size_t count = 0;

    ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_CB(NDF_REQUEST, line_splitter_reconstruct_line, &parser->line),
            ND_LOG_FIELD_CB(NDF_NIDL_NODE, parser_reconstruct_node, parser),
            ND_LOG_FIELD_CB(NDF_NIDL_INSTANCE, parser_reconstruct_instance, parser),
            ND_LOG_FIELD_CB(NDF_NIDL_CONTEXT, parser_reconstruct_context, parser),
            ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    CLEANUP_FUNCTION_REGISTER(pluginsd_process_thread_cleanup) cleanup_parser = parser;
    buffered_reader_init(&parser->reader);
    CLEAN_BUFFER *buffer = buffer_create(sizeof(parser->reader.read_buffer) + 2, NULL);
    bool send_quit = true;
    while(likely(service_running(SERVICE_COLLECTORS))) {

        if(unlikely(!buffered_reader_next_line(&parser->reader, buffer))) {
            buffered_reader_ret_t ret = buffered_reader_read_timeout(
                    &parser->reader, parser->fd_input,
                    2 * 60 * MSEC_PER_SEC, true);

            if(unlikely(ret != BUFFERED_READER_READ_OK)) {
                nd_log(NDLS_COLLECTORS, NDLP_INFO, "PLUGINSD: buffered reader not OK (%u)", (unsigned)ret);
                if(ret == BUFFERED_READER_READ_POLLERR || ret == BUFFERED_READER_READ_POLLHUP)
                    send_quit = false;
                break;
            }

            continue;
        }

        if(unlikely(parser_action(parser, buffer->buffer)))
            break;

        buffer->len = 0;
        buffer->buffer[0] = '\0';
    }

    if(send_quit) {
        nd_log(NDLS_COLLECTORS, NDLP_DEBUG,
               "PLUGINSD: sending '"PLUGINSD_CALL_QUIT"'  to plugin: %s",
               string2str(cd->filename));
        
        send_to_plugin(PLUGINSD_CALL_QUIT, parser, STREAM_TRAFFIC_TYPE_METADATA);
    }

    cd->unsafe.enabled = parser->user.enabled;
    count = parser->user.data_collections_count;

    if(likely(count)) {
        cd->successful_collections += count;
        cd->serial_failures = 0;
    }
    else
        cd->serial_failures++;

    return count;
}

#include "gperf-hashtable.h"

ALWAYS_INLINE PARSER_RC parser_execute(PARSER *parser, const PARSER_KEYWORD *keyword, char **words, size_t num_words) {
    // put all the keywords ordered by the frequency they are used

    switch(keyword->id) {
        case PLUGINSD_KEYWORD_ID_SET2:
            return pluginsd_set_v2(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_BEGIN2:
            return pluginsd_begin_v2(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_END2:
            return pluginsd_end_v2(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_SET:
            return pluginsd_set(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_BEGIN:
            return pluginsd_begin(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_END:
            return pluginsd_end(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_RSET:
            return pluginsd_replay_set(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_RBEGIN:
            return pluginsd_replay_begin(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_RDSTATE:
            return pluginsd_replay_rrddim_collection_state(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_RSSTATE:
            return pluginsd_replay_rrdset_collection_state(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_REND:
            return pluginsd_replay_end(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_DIMENSION:
            return pluginsd_dimension(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_CHART:
            return pluginsd_chart(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_CHART_DEFINITION_END:
            return pluginsd_chart_definition_end(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_CLABEL:
            return pluginsd_clabel(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_CLABEL_COMMIT:
            return pluginsd_clabel_commit(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_FUNCTION:
            return pluginsd_function(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_FUNCTION_RESULT_BEGIN:
            return pluginsd_function_result_begin(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_FUNCTION_PROGRESS:
            return pluginsd_function_progress(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_JSON:
            return pluginsd_json(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_LABEL:
            return pluginsd_label(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_OVERWRITE:
            return pluginsd_overwrite(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_VARIABLE:
            return pluginsd_variable(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_CLAIMED_ID:
            return stream_receiver_pluginsd_claimed_id(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_HOST:
            return pluginsd_host(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_HOST_DEFINE:
            return pluginsd_host_define(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_HOST_DEFINE_END:
            return pluginsd_host_define_end(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_HOST_LABEL:
            return pluginsd_host_labels(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_FLUSH:
            return pluginsd_flush(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_DISABLE:
            return pluginsd_disable(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_EXIT:
            return pluginsd_exit(words, num_words, parser);
        case PLUGINSD_KEYWORD_ID_CONFIG:
            return pluginsd_config(words, num_words, parser);

        case PLUGINSD_KEYWORD_ID_DYNCFG_ENABLE:
        case PLUGINSD_KEYWORD_ID_DYNCFG_REGISTER_MODULE:
        case PLUGINSD_KEYWORD_ID_DYNCFG_REGISTER_JOB:
        case PLUGINSD_KEYWORD_ID_DYNCFG_RESET:
        case PLUGINSD_KEYWORD_ID_REPORT_JOB_STATUS:
        case PLUGINSD_KEYWORD_ID_DELETE_JOB:
            return pluginsd_dyncfg_noop(words, num_words, parser);

        default:
            netdata_log_error("Unknown keyword '%s' with id %zu", keyword->keyword, keyword->id);
            return PARSER_RC_ERROR;;
    }
}

void parser_init_repertoire(PARSER *parser, PARSER_REPERTOIRE repertoire) {
    parser->repertoire = repertoire;

    for(size_t i = GPERF_PARSER_MIN_HASH_VALUE ; i <= GPERF_PARSER_MAX_HASH_VALUE ;i++) {
        if(gperf_keywords[i].keyword && *gperf_keywords[i].keyword && (parser->repertoire & gperf_keywords[i].repertoire))
            worker_register_job_name(gperf_keywords[i].worker_job_id, gperf_keywords[i].keyword);
    }
}

int pluginsd_parser_unittest(void) {
    PARSER *p = parser_init(NULL, -1, -1, PARSER_INPUT_SPLIT, NULL);
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
            const PARSER_KEYWORD *keyword = parser_find_keyword(p, command);
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

// SPDX-License-Identifier: GPL-3.0-or-later

#include "health_internals.h"
#include "health-alert-entry.h"

// ---------------------------------------------------------------------------------------------------------------------

static struct {
    ALERT_LOOKUP_DIMS_GROUPING group;
    const char *name;
} dims_grouping[] = {
    { .group = ALERT_LOOKUP_DIMS_SUM, .name = "sum" },
    { .group = ALERT_LOOKUP_DIMS_MIN, .name = "min" },
    { .group = ALERT_LOOKUP_DIMS_MAX, .name = "max" },
    { .group = ALERT_LOOKUP_DIMS_AVERAGE, .name = "average" },
    { .group = ALERT_LOOKUP_DIMS_MIN2MAX, .name = "min2max" },

    // terminator
    { .group = 0, .name = NULL },
};

ALERT_LOOKUP_DIMS_GROUPING alerts_dims_grouping2id(const char *group) {
    if(!group || !*group)
        return dims_grouping[0].group;

    for(size_t i = 0; dims_grouping[i].name ;i++) {
        if(strcmp(dims_grouping[i].name, group) == 0)
            return dims_grouping[i].group;
    }

    nd_log(NDLS_DAEMON, NDLP_WARNING, "Alert lookup dimensions grouping '%s' is not valid", group);
    return dims_grouping[0].group;
}

const char *alerts_dims_grouping_id2group(ALERT_LOOKUP_DIMS_GROUPING grouping) {
    for(size_t i = 0; dims_grouping[i].name ;i++) {
        if(grouping == dims_grouping[i].group)
            return dims_grouping[i].name;
    }

    nd_log(NDLS_DAEMON, NDLP_WARNING, "Alert lookup dimensions grouping %d is not valid", grouping);
    return dims_grouping[0].name;
}

// ---------------------------------------------------------------------------------------------------------------------

static struct {
    ALERT_LOOKUP_DATA_SOURCE source;
    const char *name;
} data_sources[] = {
    { .source = ALERT_LOOKUP_DATA_SOURCE_SAMPLES, .name = "samples" },
    { .source = ALERT_LOOKUP_DATA_SOURCE_PERCENTAGES, .name = "percentages" },
    { .source = ALERT_LOOKUP_DATA_SOURCE_ANOMALIES, .name = "anomalies" },

    // terminator
    { .source = 0, .name = NULL },
};

ALERT_LOOKUP_DATA_SOURCE alerts_data_sources2id(const char *source) {
    if(!source || !*source)
        return data_sources[0].source;

    for(size_t i = 0; data_sources[i].name ;i++) {
        if(strcmp(data_sources[i].name, source) == 0)
            return data_sources[i].source;
    }

    nd_log(NDLS_DAEMON, NDLP_WARNING, "Alert data source '%s' is not valid", source);
    return data_sources[0].source;
}

const char *alerts_data_source_id2source(ALERT_LOOKUP_DATA_SOURCE source) {
    for(size_t i = 0; data_sources[i].name ;i++) {
        if(source == data_sources[i].source)
            return data_sources[i].name;
    }

    nd_log(NDLS_DAEMON, NDLP_WARNING, "Alert data source %d is not valid", source);
    return data_sources[0].name;
}

// ---------------------------------------------------------------------------------------------------------------------

static struct {
    ALERT_LOOKUP_TIME_GROUP_CONDITION condition;
    const char *name;
} group_conditions[] = {
    { .condition = ALERT_LOOKUP_TIME_GROUP_CONDITION_EQUAL, .name = "=" },
    { .condition = ALERT_LOOKUP_TIME_GROUP_CONDITION_NOT_EQUAL, .name = "!=" },
    { .condition = ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER, .name = ">" },
    { .condition = ALERT_LOOKUP_TIME_GROUP_CONDITION_GREATER_EQUAL, .name = ">=" },
    { .condition = ALERT_LOOKUP_TIME_GROUP_CONDITION_LESS, .name = "<" },
    { .condition = ALERT_LOOKUP_TIME_GROUP_CONDITION_LESS_EQUAL, .name = "<=" },

    // terminator
    { .condition = 0, .name = NULL },
};

ALERT_LOOKUP_TIME_GROUP_CONDITION alerts_group_condition2id(const char *source) {
    if(!source || !*source)
        return group_conditions[0].condition;

    for(size_t i = 0; group_conditions[i].name ;i++) {
        if(strcmp(group_conditions[i].name, source) == 0)
            return group_conditions[i].condition;
    }

    nd_log(NDLS_DAEMON, NDLP_WARNING, "Alert data source '%s' is not valid", source);
    return group_conditions[0].condition;
}

const char *alerts_group_conditions_id2txt(ALERT_LOOKUP_TIME_GROUP_CONDITION source) {
    for(size_t i = 0; group_conditions[i].name ;i++) {
        if(source == group_conditions[i].condition)
            return group_conditions[i].name;
    }

    nd_log(NDLS_DAEMON, NDLP_WARNING, "Alert data source %d is not valid", source);
    return group_conditions[0].name;
}

// ---------------------------------------------------------------------------------------------------------------------

static struct {
    const char *name;
    uint32_t hash;
    ALERT_ACTION_OPTIONS value;
} alert_action_options[] = {
    {  "no-clear-notification", 0    , ALERT_ACTION_OPTION_NO_CLEAR_NOTIFICATION}

    // terminator
    , {NULL, 0, 0}
};

inline ALERT_ACTION_OPTIONS alert_action_options_parse_one(const char *o) {
    ALERT_ACTION_OPTIONS ret = 0;

    if(!o || !*o) return ret;

    uint32_t hash = simple_hash(o);
    int i;
    for(i = 0; alert_action_options[i].name ; i++) {
        if (unlikely(hash == alert_action_options[i].hash && !strcmp(o, alert_action_options[i].name))) {
            ret |= alert_action_options[i].value;
            break;
        }
    }

    return ret;
}

inline ALERT_ACTION_OPTIONS alert_action_options_parse(char *o) {
    ALERT_ACTION_OPTIONS ret = 0;
    char *tok;

    while(o && *o && (tok = strsep_skip_consecutive_separators(&o, ", |"))) {
        if(!*tok) continue;
        ret |= alert_action_options_parse_one(tok);
    }

    return ret;
}

void alert_action_options_to_buffer_json_array(BUFFER *wb, const char *key, ALERT_ACTION_OPTIONS options) {
    buffer_json_member_add_array(wb, key);

    RRDR_OPTIONS used = 0; // to prevent adding duplicates
    for(int i = 0; alert_action_options[i].name ; i++) {
        if (unlikely((alert_action_options[i].value & options) && !(alert_action_options[i].value & used))) {
            const char *name = alert_action_options[i].name;
            used |= alert_action_options[i].value;

            buffer_json_add_array_item_string(wb, name);
        }
    }

    buffer_json_array_close(wb);
}

void alert_action_options_to_buffer(BUFFER *wb, ALERT_ACTION_OPTIONS options) {
    RRDR_OPTIONS used = 0; // to prevent adding duplicates
    for(int i = 0; alert_action_options[i].name ; i++) {
        if (unlikely((alert_action_options[i].value & options) && !(alert_action_options[i].value & used))) {
            if(used != 0)
                buffer_strcat(wb, " ");

            const char *name = alert_action_options[i].name;
            used |= alert_action_options[i].value;

            buffer_strcat(wb, name);
        }
    }
}

static void alert_action_options_init(void) {
    for(int i = 0; alert_action_options[i].name ; i++)
        alert_action_options[i].hash = simple_hash(alert_action_options[i].name);
}


// ---------------------------------------------------------------------------------------------------------------------

static void health_prototype_cleanup_one_unsafe(RRD_ALERT_PROTOTYPE *ap) {
    rrd_alert_match_cleanup(&ap->match);
    rrd_alert_config_cleanup(&ap->config);
}

void health_prototype_cleanup(RRD_ALERT_PROTOTYPE *ap) {
    rw_spinlock_write_lock(&ap->_internal.rw_spinlock);

    while(ap->_internal.next) {
        RRD_ALERT_PROTOTYPE *t = ap->_internal.next;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(ap->_internal.next, t, _internal.prev, _internal.next);
        health_prototype_cleanup_one_unsafe(t);
        freez(t);
    }

    rw_spinlock_write_unlock(&ap->_internal.rw_spinlock);

    health_prototype_cleanup_one_unsafe(ap);
}

void health_prototype_free(RRD_ALERT_PROTOTYPE *ap) {
    if(!ap) return;
    health_prototype_cleanup(ap);
    freez(ap);
}

void health_prototype_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    RRD_ALERT_PROTOTYPE *ap = value;
    rw_spinlock_init(&ap->_internal.rw_spinlock);
    if(ap->config.source_type != DYNCFG_SOURCE_TYPE_DYNCFG)
        ap->_internal.is_on_disk = true;
}

bool health_prototype_conflict_cb(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data __maybe_unused) {
    RRD_ALERT_PROTOTYPE *ap = old_value;
    RRD_ALERT_PROTOTYPE *nap = new_value;

    bool replace = nap->config.source_type == DYNCFG_SOURCE_TYPE_DYNCFG;

    if(ap->config.source_type != DYNCFG_SOURCE_TYPE_DYNCFG || nap->config.source_type != DYNCFG_SOURCE_TYPE_DYNCFG)
        ap->_internal.is_on_disk = nap->_internal.is_on_disk = true;

    if(!replace) {
        if(ap->config.source_type == DYNCFG_SOURCE_TYPE_DYNCFG) {
            // the existing is a dyncfg and the new one is read from the config
            health_prototype_cleanup(nap);
            memset(nap, 0, sizeof(*nap));
        }
        else {
            // alerts with the same name are appended to the existing one
            nap = callocz(1, sizeof(*nap));
            memcpy(nap, new_value, sizeof(*nap));

            rw_spinlock_write_lock(&ap->_internal.rw_spinlock);
            DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(ap->_internal.next, nap, _internal.prev, _internal.next);
            rw_spinlock_write_unlock(&ap->_internal.rw_spinlock);

            if(nap->_internal.enabled)
                ap->_internal.enabled = true;
        }
    }
    else {
        // alerts with the same name replace the existing one
        rw_spinlock_init(&nap->_internal.rw_spinlock);

        rw_spinlock_write_lock(&nap->_internal.rw_spinlock);
        rw_spinlock_write_lock(&ap->_internal.rw_spinlock);
        SWAP(*ap, *nap);
        rw_spinlock_write_unlock(&ap->_internal.rw_spinlock);
        rw_spinlock_write_unlock(&nap->_internal.rw_spinlock);

        health_prototype_cleanup(nap);
        memset(nap, 0, sizeof(*nap));
    }

    return true;
}

void health_prototype_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    RRD_ALERT_PROTOTYPE *ap = value;
    health_prototype_cleanup(ap);
}

void health_init_prototypes(void) {
    if(health_globals.prototypes.dict)
        return;

    health_globals.prototypes.dict = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, &dictionary_stats_category_rrdhealth, sizeof(RRD_ALERT_PROTOTYPE));
    dictionary_register_insert_callback(health_globals.prototypes.dict, health_prototype_insert_cb, NULL);
    dictionary_register_conflict_callback(health_globals.prototypes.dict, health_prototype_conflict_cb, NULL);
    dictionary_register_delete_callback(health_globals.prototypes.dict, health_prototype_delete_cb, NULL);

    alert_action_options_init();
}

// ---------------------------------------------------------------------------------------------------------------------

static inline struct pattern_array *health_config_add_key_to_values(struct pattern_array *pa, const char *input_key, char *value)
{
    char key[HEALTH_CONF_MAX_LINE + 1];
    char data[HEALTH_CONF_MAX_LINE + 1];

    char *s = value;
    size_t i = 0;

    char pair[HEALTH_CONF_MAX_LINE + 1];
    if (input_key)
        strncpyz(key, input_key, HEALTH_CONF_MAX_LINE);
    else
        key[0] = '\0';

    while(*s) {
        if (*s == '=') {
            //hold the key
            data[i]='\0';
            strncpyz(key, data, HEALTH_CONF_MAX_LINE);
            i=0;
        } else if (*s == ' ') {
            data[i]='\0';
            if (data[0]=='!')
                snprintfz(pair, HEALTH_CONF_MAX_LINE, "!%s=%s ", key, data + 1);
            else
                snprintfz(pair, HEALTH_CONF_MAX_LINE, "%s=%s ", key, data);

            pa = pattern_array_add_key_simple_pattern(pa, key, simple_pattern_create(pair, NULL, SIMPLE_PATTERN_EXACT, true));
            i=0;
        } else {
            data[i++] = *s;
        }
        s++;
    }
    data[i]='\0';
    if (data[0]) {
        if (data[0]=='!')
            snprintfz(pair, HEALTH_CONF_MAX_LINE, "!%s=%s ", key, data + 1);
        else
            snprintfz(pair, HEALTH_CONF_MAX_LINE, "%s=%s ", key, data);

        pa = pattern_array_add_key_simple_pattern(pa, key, simple_pattern_create(pair, NULL, SIMPLE_PATTERN_EXACT, true));
    }

    return pa;
}

static char *simple_pattern_trim_around_equal(const char *src) {
    char *store = mallocz(strlen(src) + 1);

    char *dst = store;
    while (*src) {
        if (*src == '=') {
            if (*(dst -1) == ' ')
                dst--;

            *dst++ = *src++;
            if (*src == ' ')
                src++;
        }

        *dst++ = *src++;
    }
    *dst = 0x00;

    return store;
}

struct pattern_array *trim_and_add_key_to_values(struct pattern_array *pa, const char *key, STRING *input) {
    char *tmp = simple_pattern_trim_around_equal(string2str(input));
    pa = health_config_add_key_to_values(pa, key, tmp);
    freez(tmp);
    return pa;
}

static void health_prototype_activate_match_patterns(struct rrd_alert_match *am) {
    if(am->host_labels) {
        pattern_array_free(am->host_labels_pattern);
        am->host_labels_pattern = NULL;
        am->host_labels_pattern = trim_and_add_key_to_values(am->host_labels_pattern, NULL, am->host_labels);
    }

    if(am->chart_labels) {
        pattern_array_free(am->chart_labels_pattern);
        am->chart_labels_pattern = NULL;
        am->chart_labels_pattern = trim_and_add_key_to_values(am->chart_labels_pattern, NULL, am->chart_labels);
    }
}

void health_prototype_hash_id(RRD_ALERT_PROTOTYPE *ap) {
    CLEAN_BUFFER *wb = buffer_create(100, NULL);
    health_prototype_to_json(wb, ap, true);
    ND_UUID uuid = UUID_generate_from_hash(buffer_tostring(wb), buffer_strlen(wb));
    uuid_copy(ap->config.hash_id, uuid.uuid);

    sql_alert_store_config(ap);
}

bool health_prototype_add(RRD_ALERT_PROTOTYPE *ap, char **msg) {
    if(!ap->match.is_template) {
        if(!ap->match.on.chart) {
            netdata_log_error(
                "HEALTH: alert '%s' does not define a instance (parameter 'on'). Source: %s",
                string2str(ap->config.name), string2str(ap->config.source));
            if(msg)
                *msg = "missing match 'on' parameter for instance";
            return false;
        }
    }
    else {
        if(!ap->match.on.context) {
            netdata_log_error(
                "HEALTH: alert '%s' does not define a context (parameter 'on'). Source: %s",
                string2str(ap->config.name), string2str(ap->config.source));
            if(msg)
                *msg = "missing match 'on' parameter for context";
            return false;
        }
    }

    if(!ap->config.update_every) {
        netdata_log_error(
            "HEALTH: alert '%s' has no frequency (parameter 'every'). Source: %s",
            string2str(ap->config.name), string2str(ap->config.source));
        if(msg)
            *msg = "missing update frequency";
        return false;
    }

    if(!RRDCALC_HAS_DB_LOOKUP(ap) && !ap->config.calculation && !ap->config.warning && !ap->config.critical) {
        netdata_log_error(
            "HEALTH: alert '%s' is useless (no db lookup, no calculation, no warning and no critical expressions). Source: %s",
            string2str(ap->config.name), string2str(ap->config.source));
        if(msg)
            *msg = "no db lookup, calculation and warning/critical conditions";
        return false;
    }

    // activate the match patterns in it
    bool enabled = false;
    for(RRD_ALERT_PROTOTYPE *t = ap; t ;t = t->_internal.next) {
        // we need to generate config_hash_id for each instance included
        // so, let's break the linked list for this iteration

        RRD_ALERT_PROTOTYPE *prev = t->_internal.prev;
        RRD_ALERT_PROTOTYPE *next = t->_internal.next;
        t->_internal.prev = t;
        t->_internal.next = NULL;

        if(t->match.enabled)
            enabled = true;

        if(!t->config.name)
            t->config.name = string_dup(ap->config.name);

        health_prototype_hash_id(t);

        health_prototype_activate_match_patterns(&t->match);

        if (!t->config.exec)
            t->config.exec = string_dup(health_globals.config.default_exec);

        if (!t->config.recipient)
            t->config.recipient = string_dup(health_globals.config.default_recipient);

        // restore the linked list
        t->_internal.prev = prev;
        t->_internal.next = next;
    }
    ap->_internal.enabled = enabled;

    // add it to the prototypes
    dictionary_set_advanced(health_globals.prototypes.dict,
                            string2str(ap->config.name), string_strlen(ap->config.name),
                            ap, sizeof(RRD_ALERT_PROTOTYPE),
                            NULL);

    return true;
}

// ---------------------------------------------------------------------------------------------------------------------

void health_reload_prototypes(void) {
    // remove all dyncfg related to prototypes
    health_dyncfg_unregister_all_prototypes();

    // clear old prototypes from memory
    dictionary_flush(health_globals.prototypes.dict);

    // load the prototypes from disk
    recursive_config_double_dir_load(
        health_user_config_dir(),
        health_globals.config.stock_enabled ? health_stock_config_dir() : NULL,
        NULL,
        health_readfile,
        NULL, 0);

    // register all loaded prototypes
    health_dyncfg_register_all_prototypes();
}

// ---------------------------------------------------------------------------------------------------------------------

static bool prototype_matches_host(RRDHOST *host, RRD_ALERT_PROTOTYPE *ap) {
    if(health_globals.config.enabled_alerts &&
        !simple_pattern_matches(health_globals.config.enabled_alerts, string2str(ap->config.name)))
        return false;

    if (host->rrdlabels && ap->match.host_labels_pattern &&
        !pattern_array_label_match(ap->match.host_labels_pattern, host->rrdlabels, '=', NULL))
        return false;

    return true;
}

static bool prototype_matches_rrdset(RRDSET *st, RRD_ALERT_PROTOTYPE *ap) {
    // match the chart id
    if(!ap->match.is_template && ap->match.on.chart &&
        ap->match.on.chart != st->id && ap->match.on.chart != st->name)
        return false;

    // match the chart context
    if(ap->match.is_template && ap->match.on.context &&
        ap->match.on.context != st->context)
        return false;

    if (st->rrdlabels && ap->match.chart_labels_pattern &&
        !pattern_array_label_match(ap->match.chart_labels_pattern, st->rrdlabels, '=', NULL))
        return false;

    return true;
}

void health_prototype_copy_match_without_patterns(struct rrd_alert_match *dst, struct rrd_alert_match *src) {
    dst->enabled = src->enabled;
    dst->is_template = src->is_template;

    if(dst->is_template)
        dst->on.context = string_dup(src->on.context);
    else
        dst->on.chart = string_dup(src->on.chart);

    dst->host_labels = string_dup(src->host_labels);
    dst->chart_labels = string_dup(src->chart_labels);
}

void health_prototype_copy_config(struct rrd_alert_config *dst, struct rrd_alert_config *src) {
    uuid_copy(dst->hash_id, src->hash_id);

    dst->name = string_dup(src->name);

    dst->exec = string_dup(src->exec);
    dst->recipient = string_dup(src->recipient);

    dst->classification = string_dup(src->classification);
    dst->component = string_dup(src->component);
    dst->type = string_dup(src->type);

    dst->source_type = src->source_type;
    dst->source = string_dup(src->source);
    dst->units = string_dup(src->units);
    dst->summary = string_dup(src->summary);
    dst->info = string_dup(src->info);

    dst->update_every = src->update_every;

    dst->alert_action_options = src->alert_action_options;

    dst->dimensions = string_dup(src->dimensions);

    dst->time_group = src->time_group;
    dst->time_group_condition = src->time_group_condition;
    dst->time_group_value = src->time_group_value;
    dst->dims_group = src->dims_group;
    dst->data_source = src->data_source;
    dst->before = src->before;
    dst->after = src->after;
    dst->options = src->options;

    const char *failed_at = NULL;
    int error = 0;

    dst->calculation = expression_parse(expression_source(src->calculation), &failed_at, &error);
    dst->warning = expression_parse(expression_source(src->warning), &failed_at, &error);
    dst->critical = expression_parse(expression_source(src->critical), &failed_at, &error);

    dst->delay_up_duration = src->delay_up_duration;
    dst->delay_down_duration = src->delay_down_duration;
    dst->delay_max_duration = src->delay_max_duration;
    dst->delay_multiplier = src->delay_multiplier;

    dst->has_custom_repeat_config = src->has_custom_repeat_config;
    dst->warn_repeat_every = src->warn_repeat_every;
    dst->crit_repeat_every = src->crit_repeat_every;
}

static void health_prototype_apply_to_rrdset(RRDSET *st, RRD_ALERT_PROTOTYPE *ap) {
    if(!ap->_internal.enabled)
        return;

    rw_spinlock_read_lock(&ap->_internal.rw_spinlock);
    for(size_t template = 0; template < 2; template++) {
        bool want_template = template ? true : false;

        for (RRD_ALERT_PROTOTYPE *t = ap; t; t = t->_internal.next) {
            if (!t->match.enabled)
                continue;

            bool is_template = t->match.is_template ? true : false;

            if (is_template != want_template)
                continue;

            if (!prototype_matches_host(st->rrdhost, t))
                continue;

            if (!prototype_matches_rrdset(st, t))
                continue;

            rrdcalc_add_from_prototype(st->rrdhost, st, t);
        }
    }
    rw_spinlock_read_unlock(&ap->_internal.rw_spinlock);
}

void health_prototype_alerts_for_rrdset_incrementally(RRDSET *st) {
    RRD_ALERT_PROTOTYPE *ap;
    dfe_start_read(health_globals.prototypes.dict, ap) {
        health_prototype_apply_to_rrdset(st, ap);
    }
    dfe_done(ap);
}

void health_prototype_reset_alerts_for_rrdset(RRDSET *st) {
    rrdcalc_unlink_and_delete_all_rrdset_alerts(st);
    health_prototype_alerts_for_rrdset_incrementally(st);
}

// ---------------------------------------------------------------------------------------------------------------------

void health_apply_prototype_to_host(RRDHOST *host, RRD_ALERT_PROTOTYPE *ap) {
    if(!ap->_internal.enabled)
        return;

    if(unlikely(!host->health.enabled) && !rrdhost_flag_check(host, RRDHOST_FLAG_INITIALIZED_HEALTH))
        return;

    RRDSET *st;
    rrdset_foreach_read(st, host) {
        health_prototype_apply_to_rrdset(st, ap);
    }
    rrdset_foreach_done(st);
}

void health_prototype_apply_to_all_hosts(RRD_ALERT_PROTOTYPE *ap) {
    if(!ap->_internal.enabled)
        return;

    RRDHOST *host;
    dfe_start_reentrant(rrdhost_root_index, host){
        health_apply_prototype_to_host(host, ap);
    }
    dfe_done(host);
}

// ---------------------------------------------------------------------------------------------------------------------

void health_apply_prototypes_to_host(RRDHOST *host) {
    if(unlikely(!host->health.enabled) && !rrdhost_flag_check(host, RRDHOST_FLAG_INITIALIZED_HEALTH))
        return;

    // free all running alarms
    rrdcalc_delete_all(host);

    // invalidate all previous entries in the alarm log
    rw_spinlock_read_lock(&host->health_log.spinlock);
    ALARM_ENTRY *t;
    for(t = host->health_log.alarms ; t ; t = t->next) {
        if(t->new_status != RRDCALC_STATUS_REMOVED)
            t->flags |= HEALTH_ENTRY_FLAG_UPDATED;
    }
    rw_spinlock_read_unlock(&host->health_log.spinlock);

    // apply all the prototypes for the charts of the host
    RRDSET *st;
    rrdset_foreach_read(st, host) {
        health_prototype_reset_alerts_for_rrdset(st);
    }
    rrdset_foreach_done(st);
}

void health_apply_prototypes_to_all_hosts(void) {
    RRDHOST *host;
    dfe_start_reentrant(rrdhost_root_index, host){
        health_apply_prototypes_to_host(host);
    }
    dfe_done(host);
}

// ---------------------------------------------------------------------------------------------------------------------

void health_prototype_metadata_foreach(void *data, prototype_metadata_cb_t cb) {
    RRD_ALERT_PROTOTYPE *ap;
    dfe_start_read(health_globals.prototypes.dict, ap) {
        cb(data, ap->config.type, ap->config.component, ap->config.classification, ap->config.recipient);
    }
    dfe_done(ap);
}

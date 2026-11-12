// SPDX-License-Identifier: GPL-3.0-or-later

#include "health_internals.h"
#include "health-alert-entry.h"
#include "database/sqlite/sqlite_aclk_alert.h"

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
    memset(ap, 0, sizeof(*ap));
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
        if(ap->config.source_type != DYNCFG_SOURCE_TYPE_DYNCFG) {
            // alerts with the same name are appended to the existing one
            RRD_ALERT_PROTOTYPE *alloced = callocz(1, sizeof(*alloced));
            SWAP(*alloced, *nap);

            rw_spinlock_write_lock(&ap->_internal.rw_spinlock);
            DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(ap->_internal.next, alloced, _internal.prev, _internal.next);
            if(alloced->_internal.enabled)
                ap->_internal.enabled = true;
            rw_spinlock_write_unlock(&ap->_internal.rw_spinlock);
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
    }

    health_prototype_cleanup(nap);

    return true;
}

void health_prototype_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    RRD_ALERT_PROTOTYPE *ap = value;
    health_prototype_cleanup(ap);
}

void health_init_prototypes(void) {
    static bool reload_rwlock_initialized = false;
    if(!reload_rwlock_initialized) {
        netdata_rwlock_init(&health_globals.prototypes.reload_rwlock);
        reload_rwlock_initialized = true;
    }

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
    size_t value_len = strlen(value);
    size_t input_key_len = input_key ? strlen(input_key) : 0;
    size_t key_len = value_len > input_key_len ? value_len : input_key_len;
    size_t pair_len = key_len + value_len + 4;
    char *key = mallocz(key_len + 1);
    char *data = mallocz(value_len + 1);
    char *pair = mallocz(pair_len);

    char *s = value;
    size_t i = 0;

    if (input_key)
        strncpyz(key, input_key, key_len);
    else
        key[0] = '\0';

    while(*s) {
        if (*s == '=') {
            //hold the key
            data[i]='\0';
            strncpyz(key, data, key_len);
            i=0;
        } else if (*s == ' ') {
            data[i]='\0';
            if (data[0]=='!')
                snprintfz(pair, pair_len, "!%s=%s ", key, data + 1);
            else
                snprintfz(pair, pair_len, "%s=%s ", key, data);

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
            snprintfz(pair, pair_len, "!%s=%s ", key, data + 1);
        else
            snprintfz(pair, pair_len, "%s=%s ", key, data);

        pa = pattern_array_add_key_simple_pattern(pa, key, simple_pattern_create(pair, NULL, SIMPLE_PATTERN_EXACT, true));
    }

    freez(key);
    freez(data);
    freez(pair);

    return pa;
}

static char *simple_pattern_trim_around_equal(const char *src) {
    char *store = mallocz(strlen(src) + 1);

    char *dst = store;
    while (*src) {
        if (*src == '=') {
            if (dst > store && *(dst - 1) == ' ')
                dst--;

            *dst++ = *src++;
            if (*src == ' ')
                src++;

            if (!*src)
                break;
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

    if (ap->config.source_type == DYNCFG_SOURCE_TYPE_DYNCFG &&
        !__atomic_load_n(&health_globals.prototypes.registering, __ATOMIC_RELAXED) &&
        !alert_hash_has_transitioned(&ap->config.hash_id)) {
        char hash_id_str[UUID_STR_LEN];
        uuid_unparse_lower(ap->config.hash_id, hash_id_str);
        aclk_send_alert_configuration(hash_id_str);
    }
}

// Alerts sharing a name are kept as a chain of rules (one per OS, per label set, ...).
// Every rule has to stand on its own, because health_prototype_matches_rrdset() and the health
// loop only ever look at the single rule they are given. Validating just the head of the chain let
// a rule with no 'on' through, and such a rule then attached the alert to every chart of the host
// (netdata/netdata#23483).
static const char *health_prototype_rule_validate(RRD_ALERT_PROTOTYPE *t) {
    if(t->match.is_template) {
        if(!t->match.on.context)
            return "missing match 'on' parameter for context";
    }
    else if(!t->match.on.chart)
        return "missing match 'on' parameter for instance";

    if(t->config.update_every <= 0)
        return "missing update frequency";

    if(!RRDCALC_HAS_DB_LOOKUP(t) && !t->config.calculation && !t->config.warning && !t->config.critical)
        return "no db lookup, calculation and warning/critical conditions";

    if(!isfinite(t->config.delay_multiplier))
        return "non-finite delay multiplier";

    return NULL;
}

bool health_prototype_add(RRD_ALERT_PROTOTYPE *ap, const char **msg) {
    size_t rule = 0;
    for(RRD_ALERT_PROTOTYPE *t = ap; t; t = t->_internal.next, rule++) {
        const char *reason = health_prototype_rule_validate(t);
        if(!reason)
            continue;

        // config.source is set only for prototypes loaded from a health .conf file
        // (health_config.c); a dyncfg payload never carries one.
        netdata_log_error(
            "HEALTH: alert '%s' rule %zu is invalid: %s. Source: %s",
            string2str(ap->config.name), rule, reason,
            string2str(t->config.source));

        if(msg)
            *msg = reason;

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

    // clear old prototypes from memory and load the prototypes from disk; re-matches wait for the complete set
    netdata_rwlock_wrlock(&health_globals.prototypes.reload_rwlock);
    dictionary_flush(health_globals.prototypes.dict);
    recursive_config_double_dir_load(
        health_user_config_dir(),
        health_globals.config.stock_enabled ? health_stock_config_dir() : NULL,
        NULL,
        health_readfile,
        NULL, 0);
    netdata_rwlock_wrunlock(&health_globals.prototypes.reload_rwlock);

    // register all loaded prototypes
    __atomic_store_n(&health_globals.prototypes.registering, true, __ATOMIC_RELAXED);
    health_dyncfg_register_all_prototypes();
    __atomic_store_n(&health_globals.prototypes.registering, false, __ATOMIC_RELAXED);
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

bool health_prototype_matches_rrdset(RRDSET *st, RRD_ALERT_PROTOTYPE *ap) {
    // A template is defined by its context and an instance alert by its chart, so a rule without
    // one matches nothing. This must fail closed: treating a missing 'on' as "no constraint" turned
    // such a rule into a host-wide alert, attaching e.g. the system.cpu 10min_cpu_usage template to
    // every chart of the node (netdata/netdata#23483).
    if(ap->match.is_template) {
        if(!ap->match.on.context || ap->match.on.context != st->context)
            return false;
    }
    else if(!ap->match.on.chart ||
             (ap->match.on.chart != st->id && ap->match.on.chart != st->name))
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
    dst->time_group_options = string_dup(src->time_group_options);
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

// The rule of `ap` that attaches to `st`, or NULL: instance rules before templates, each in chain order, first match
// wins. Alerts are keyed by chart and alert name, so at most one rule of a chain attaches to a chart.
// The caller holds ap->_internal.rw_spinlock for reading.
static RRD_ALERT_PROTOTYPE *health_prototype_winner_for_rrdset_unsafe(RRDSET *st, RRD_ALERT_PROTOTYPE *ap) {
    if(!ap->_internal.enabled)
        return NULL;

    for(size_t template = 0; template < 2; template++) {
        bool want_template = template ? true : false;

        for (RRD_ALERT_PROTOTYPE *t = ap; t; t = t->_internal.next) {
            if (!t->match.enabled)
                continue;

            bool is_template = t->match.is_template ? true : false;

            if (is_template != want_template)
                continue;

            // the chart selector rejects almost every prototype with pointer compares, before any chart
            // label match, so it runs before the host checks, which pattern-match the name and, when
            // configured, the host labels
            if (!health_prototype_matches_rrdset(st, t))
                continue;

            if (!prototype_matches_host(st->rrdhost, t))
                continue;

            return t;
        }
    }

    return NULL;
}

static void health_prototype_apply_to_rrdset(RRDSET *st, RRD_ALERT_PROTOTYPE *ap) {
    rw_spinlock_read_lock(&ap->_internal.rw_spinlock);
    RRD_ALERT_PROTOTYPE *winner = health_prototype_winner_for_rrdset_unsafe(st, ap);
    if(winner)
        rrdcalc_add_from_prototype(st->rrdhost, st, winner);
    rw_spinlock_read_unlock(&ap->_internal.rw_spinlock);
}

// Re-matches the alert named by `ap` on `st`: keeps it when the rule that attaches now is the one it was created
// from, otherwise deletes it and creates the alert of the rule that attaches now, if any.
static void health_prototype_rematch_on_rrdset(RRDSET *st, RRD_ALERT_PROTOTYPE *ap, uint64_t seq) {
    rw_spinlock_read_lock(&ap->_internal.rw_spinlock);
    RRD_ALERT_PROTOTYPE *winner = health_prototype_winner_for_rrdset_unsafe(st, ap);

    // most rules match nothing on most charts: take the host alert dictionary write lock only when the chart has an
    // alert of this name to keep or delete; an alert another thread adds meanwhile is stamped for the sweep when linked
    bool kept = false;
    if(rrdcalc_rrdset_has_alert(st, ap->config.name))
        kept = rrdcalc_rematch_keep(st, ap->config.name, winner, seq);

    if(!kept && winner)
        rrdcalc_add_from_prototype(st->rrdhost, st, winner);
    rw_spinlock_read_unlock(&ap->_internal.rw_spinlock);
}

static uint64_t health_prototype_rematch_seq_next(void) {
    return __atomic_add_fetch(&health_globals.prototypes.rematch_seq, 1, __ATOMIC_SEQ_CST);
}

extern __thread bool is_health_thread;

void health_prototype_alerts_for_rrdset_incrementally(RRDSET *st) {
    // attaching from a set a reload is still loading would pick partial rule chains, which the re-match after the
    // reload would then replace
    netdata_rwlock_rdlock(&health_globals.prototypes.reload_rwlock);

    RRD_ALERT_PROTOTYPE *ap;
    dfe_start_read(health_globals.prototypes.dict, ap) {
        if (is_health_thread && !service_running(SERVICE_HEALTH))
            break;
        health_prototype_apply_to_rrdset(st, ap);
    }
    dfe_done(ap);

    netdata_rwlock_rdunlock(&health_globals.prototypes.reload_rwlock);
}

// Brings the alerts of `st` in line with the current labels and prototypes without touching the alerts whose rule
// did not change: a label change, a health reload or a re-registered prototype must not remove and re-raise them.
// Alerts whose rule changed are replaced, alerts whose rule is gone are removed, newly matching rules are added.
void health_prototype_rematch_rrdset(RRDSET *st) {
    // the whole walk sees one complete set of prototypes, not one a reload is still loading
    netdata_rwlock_rdlock(&health_globals.prototypes.reload_rwlock);

    uint64_t seq = health_prototype_rematch_seq_next();
    bool complete = true;

    RRD_ALERT_PROTOTYPE *ap;
    dfe_start_read(health_globals.prototypes.dict, ap) {
        if (is_health_thread && !service_running(SERVICE_HEALTH)) {
            complete = false;
            break;
        }
        health_prototype_rematch_on_rrdset(st, ap, seq);
    }
    dfe_done(ap);

    // an interrupted walk did not visit every prototype, so it cannot tell which alerts lost theirs
    if(complete)
        rrdcalc_rematch_sweep_rrdset(st, seq);

    netdata_rwlock_rdunlock(&health_globals.prototypes.reload_rwlock);
}

// ---------------------------------------------------------------------------------------------------------------------

// A prototype was added, changed, enabled or disabled: re-match its alert on every chart of every host. Alerts whose
// rule did not change are kept; a disabled prototype attaches nowhere, so its alerts are removed.
void health_prototype_apply_to_all_hosts(RRD_ALERT_PROTOTYPE *ap) {
    // like every re-match, wait for a reload that is still loading the rules
    netdata_rwlock_rdlock(&health_globals.prototypes.reload_rwlock);

    uint64_t seq = health_prototype_rematch_seq_next();

    RRDHOST *host;
    dfe_start_reentrant(rrdhost_root_index, host){
        if(unlikely(!host->health.enabled || !rrdhost_flag_check(host, RRDHOST_FLAG_INITIALIZED_HEALTH)))
            continue;

        RRDSET *st;
        rrdset_foreach_reentrant(st, host) {
            health_prototype_rematch_on_rrdset(st, ap, seq);
        }
        rrdset_foreach_done(st);
        dictionary_garbage_collect(host->rrdcalc_root_index);
    }
    dfe_done(host);

    netdata_rwlock_rdunlock(&health_globals.prototypes.reload_rwlock);
}

// ---------------------------------------------------------------------------------------------------------------------

// Re-matches every chart of the host against the current prototypes (health initialization and reload).
void health_apply_prototypes_to_host(RRDHOST *host) {
    if(unlikely(!host->health.enabled || !rrdhost_flag_check(host, RRDHOST_FLAG_INITIALIZED_HEALTH)))
        return;

    RRDSET *st;
    rrdset_foreach_reentrant(st, host) {
        if (is_health_thread && !service_running(SERVICE_HEALTH))
            break;
        health_prototype_rematch_rrdset(st);
    }
    rrdset_foreach_done(st);
    dictionary_garbage_collect(host->rrdcalc_root_index);
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

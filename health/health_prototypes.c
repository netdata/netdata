// SPDX-License-Identifier: GPL-3.0-or-later

#include "health_internals.h"

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
    spinlock_lock(&ap->_internal.spinlock);

    while(ap->_internal.next) {
        RRD_ALERT_PROTOTYPE *t = ap->_internal.next;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(ap->_internal.next, t, _internal.prev, _internal.next);
        health_prototype_cleanup_one_unsafe(t);
        freez(t);
    }

    spinlock_unlock(&ap->_internal.spinlock);

    health_prototype_cleanup_one_unsafe(ap);
}

void health_prototype_free(RRD_ALERT_PROTOTYPE *ap) {
    if(!ap) return;
    health_prototype_cleanup(ap);
    freez(ap);
}

void health_prototype_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    RRD_ALERT_PROTOTYPE *ap = value;
    spinlock_init(&ap->_internal.spinlock);
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

            spinlock_lock(&ap->_internal.spinlock);
            DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(ap->_internal.next, nap, _internal.prev, _internal.next);
            spinlock_unlock(&ap->_internal.spinlock);

            if(nap->_internal.enabled)
                ap->_internal.enabled = true;
        }
    }
    else {
        // alerts with the same name replace the existing one
        spinlock_init(&nap->_internal.spinlock);
        nap->_internal.uses = ap->_internal.uses;

        spinlock_lock(&nap->_internal.spinlock);
        spinlock_lock(&ap->_internal.spinlock);
        SWAP(*ap, *nap);
        spinlock_unlock(&ap->_internal.spinlock);
        spinlock_unlock(&nap->_internal.spinlock);

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

    health_globals.prototypes.dict = dictionary_create(DICT_OPTION_DONT_OVERWRITE_VALUE);
    dictionary_register_insert_callback(health_globals.prototypes.dict, health_prototype_insert_cb, NULL);
    dictionary_register_conflict_callback(health_globals.prototypes.dict, health_prototype_conflict_cb, NULL);
    dictionary_register_delete_callback(health_globals.prototypes.dict, health_prototype_delete_cb, NULL);

    alert_action_options_init();
}

// ---------------------------------------------------------------------------------------------------------------------

// If needed, add a prefix key to all possible values in the range
static inline char *health_config_add_key_to_values(char *value) {
    BUFFER *wb = buffer_create(HEALTH_CONF_MAX_LINE + 1, NULL);
    char key[HEALTH_CONF_MAX_LINE + 1];
    char data[HEALTH_CONF_MAX_LINE + 1];

    char *s = value;
    size_t i = 0;

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
                buffer_snprintf(wb, HEALTH_CONF_MAX_LINE, "!%s=%s ", key, data + 1);
            else
                buffer_snprintf(wb, HEALTH_CONF_MAX_LINE, "%s=%s ", key, data);
            i=0;
        } else {
            data[i++] = *s;
        }
        s++;
    }

    data[i]='\0';
    if (data[0]) {
        if (data[0]=='!')
            buffer_snprintf(wb, HEALTH_CONF_MAX_LINE, "!%s=%s ", key, data + 1);
        else
            buffer_snprintf(wb, HEALTH_CONF_MAX_LINE, "%s=%s ", key, data);
    }

    char *final = strdupz(buffer_tostring(wb));
    buffer_free(wb);

    return final;
}

static void health_prototype_activate_match_patterns(struct rrd_alert_match *am) {
    if(am->os) {
        simple_pattern_free(am->os_pattern);

        char *tmp = simple_pattern_trim_around_equal(string2str(am->os));
        am->os_pattern = simple_pattern_create(
            tmp, NULL, SIMPLE_PATTERN_EXACT, true);
        freez(tmp);
    }

    if(am->host) {
        simple_pattern_free(am->host_pattern);

        char *tmp = simple_pattern_trim_around_equal(string2str(am->host));
        am->host_pattern = simple_pattern_create(
            tmp, NULL, SIMPLE_PATTERN_EXACT, true);
        freez(tmp);
    }

    if(am->charts) {
        simple_pattern_free(am->charts_pattern);

        char *tmp = simple_pattern_trim_around_equal(string2str(am->charts));
        am->charts_pattern = simple_pattern_create(
            tmp, NULL, SIMPLE_PATTERN_EXACT, true);
        freez(tmp);
    }

    if(am->plugin) {
        simple_pattern_free(am->plugin_pattern);

        char *tmp = simple_pattern_trim_around_equal(string2str(am->plugin));
        am->plugin_pattern = simple_pattern_create(
            tmp, NULL, SIMPLE_PATTERN_EXACT, true);
        freez(tmp);
    }

    if(am->module) {
        simple_pattern_free(am->module_pattern);

        char *tmp = simple_pattern_trim_around_equal(string2str(am->module));
        am->module_pattern = simple_pattern_create(
            tmp, NULL, SIMPLE_PATTERN_EXACT, true);
        freez(tmp);
    }

    if(am->host_labels) {
        simple_pattern_free(am->host_labels_pattern);

        char *tmp = simple_pattern_trim_around_equal(string2str(am->host_labels));
        am->host_labels_pattern = simple_pattern_create(
            tmp, NULL, SIMPLE_PATTERN_EXACT, true);
        freez(tmp);
    }

    if(am->chart_labels) {
        simple_pattern_free(am->chart_labels_pattern);

        char *tmp = simple_pattern_trim_around_equal(string2str(am->chart_labels));
        char *tmp2 = health_config_add_key_to_values(tmp);
        am->chart_labels_pattern = simple_pattern_create(
            tmp2, NULL, SIMPLE_PATTERN_EXACT, true);
        freez(tmp2);
        freez(tmp);
    }
}

void health_prototype_hash_id(RRD_ALERT_PROTOTYPE *ap) {
    CLEAN_BUFFER *wb = buffer_create(100, NULL);
    health_prototype_to_json(wb, ap, true);
    UUID uuid = UUID_generate_from_hash(buffer_tostring(wb), buffer_strlen(wb));
    uuid_copy(ap->config.hash_id, uuid.uuid);

    (void) sql_alert_store_config(ap);
}

bool health_prototype_add(RRD_ALERT_PROTOTYPE *ap) {
    if(!ap->match.is_template) {
        if(!ap->match.on.chart) {
            netdata_log_error(
                "HEALTH: alert '%s' does not define a instance (parameter 'on'). Source: %s",
                string2str(ap->config.name), string2str(ap->config.source));
            return false;
        }
    }
    else {
        if(!ap->match.on.context) {
            netdata_log_error(
                "HEALTH: alert '%s' does not define a context (parameter 'on'). Source: %s",
                string2str(ap->config.name), string2str(ap->config.source));
            return false;
        }
    }

    if(!ap->config.update_every) {
        netdata_log_error(
            "HEALTH: alert '%s' has no frequency (parameter 'every'). Source: %s",
            string2str(ap->config.name), string2str(ap->config.source));
        return false;
    }

    if(!RRDCALC_HAS_DB_LOOKUP(ap) && !ap->config.calculation && !ap->config.warning && !ap->config.critical) {
        netdata_log_error(
            "HEALTH: alert '%s' is useless (no db lookup, no calculation, no warning and no critical expressions). Source: %s",
            string2str(ap->config.name), string2str(ap->config.source));
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
                            ap, sizeof(*ap),
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

    if(ap->match.os_pattern && !simple_pattern_matches_string(ap->match.os_pattern, host->os))
        return false;

    if(ap->match.host_pattern && !simple_pattern_matches_string(ap->match.host_pattern, host->hostname))
        return false;

    if(host->rrdlabels && ap->match.host_labels_pattern &&
        !rrdlabels_match_simple_pattern_parsed(
            host->rrdlabels, ap->match.host_labels_pattern, '=', NULL))
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

    // match the chart pattern
    if(ap->match.is_template && ap->match.charts && ap->match.charts_pattern &&
        !simple_pattern_matches_string(ap->match.charts_pattern, st->id) &&
        !simple_pattern_matches_string(ap->match.charts_pattern, st->name))
        return false;

    // match the plugin pattern
    if(ap->match.plugin && ap->match.plugin_pattern &&
        !simple_pattern_matches_string(ap->match.plugin_pattern, st->plugin_name))
        return false;

    // match the module pattern
    if(ap->match.module && ap->match.module_pattern &&
        !simple_pattern_matches_string(ap->match.module_pattern, st->module_name))
        return false;

    if (st->rrdlabels && ap->match.chart_labels && ap->match.chart_labels_pattern &&
        !rrdlabels_match_simple_pattern_parsed(
            st->rrdlabels, ap->match.chart_labels_pattern, '=', NULL))
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

    dst->os = string_dup(src->os);
    dst->host = string_dup(src->host);
    dst->charts = string_dup(src->charts);
    dst->plugin = string_dup(src->plugin);
    dst->module = string_dup(src->module);
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

    dst->green = src->green;
    dst->red = src->red;

    dst->dimensions = string_dup(src->dimensions);

    dst->group = src->group;
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

    spinlock_lock(&ap->_internal.spinlock);
    for(RRD_ALERT_PROTOTYPE *t = ap; t ; t = t->_internal.next) {
        if(!t->match.enabled)
            continue;

        if(!prototype_matches_host(st->rrdhost, t))
            continue;

        if(!prototype_matches_rrdset(st, t))
            continue;

        if(rrdcalc_add_from_prototype(st->rrdhost, st, t))
            ap->_internal.uses++;
    }
    spinlock_unlock(&ap->_internal.spinlock);
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

    if(unlikely(!host->health.health_enabled) && !rrdhost_flag_check(host, RRDHOST_FLAG_INITIALIZED_HEALTH))
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
    if(unlikely(!host->health.health_enabled) && !rrdhost_flag_check(host, RRDHOST_FLAG_INITIALIZED_HEALTH))
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

#ifdef ENABLE_ACLK
    if (netdata_cloud_enabled) {
        struct aclk_sync_cfg_t *wc = host->aclk_config;
        if (likely(wc)) {
            wc->alert_queue_removed = SEND_REMOVED_AFTER_HEALTH_LOOPS;
        }
    }
#endif
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

// SPDX-License-Identifier: GPL-3.0-or-later

#include "health_internals.h"

/*
 * [health]
 *    enabled = yes
 *    silencers file = /var/lib/netdata/health.silencers.json
 *    run at least every seconds = 10
 *    postpone alarms during hibernation for seconds = 60
 *    default repeat warning = never
 *    default repeat critical = never
 *    in memory max health log entries = 1000
 *    health log history = 432000
 *    enabled alarms = *
 *    script to execute on alarm = /usr/libexec/netdata/plugins.d/alarm-notify.sh
 *    use summary for notifications = yes
 *    enable stock health configuration = yes
 */

struct health_plugin_globals health_globals = {
    .sql_store_hashes = true,
    .config = {
        .enabled = true,
        .stock_enabled = true,
        .use_summary_for_notifications = true,

        .health_log_entries_max = HEALTH_LOG_ENTRIES_DEFAULT,
        .health_log_history = HEALTH_LOG_HISTORY_DEFAULT,

        .default_warn_repeat_every = 0,
        .default_crit_repeat_every = 0,

        .run_at_least_every_seconds = 10,
        .postpone_alarms_during_hibernation_for_seconds = 60,
    },
    .prototypes = {
        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
        .base = NULL,
    }
};

bool health_plugin_enabled(void) {
    return health_globals.config.enabled;
}

void health_plugin_disable(void) {
    health_globals.config.enabled = false;
}

void health_load_config_defaults(void) {
    char filename[FILENAME_MAX + 1];

    health_globals.config.enabled =
        config_get_boolean(CONFIG_SECTION_HEALTH,
                           "enabled",
                           health_globals.config.enabled);

    health_globals.config.stock_enabled =
        config_get_boolean(CONFIG_SECTION_HEALTH,
                           "enable stock health configuration",
                           health_globals.config.stock_enabled);

    health_globals.config.use_summary_for_notifications =
        config_get_boolean(CONFIG_SECTION_HEALTH,
                           "use summary for notifications",
                           health_globals.config.use_summary_for_notifications);

    health_globals.config.default_warn_repeat_every =
        config_get_duration(CONFIG_SECTION_HEALTH, "default repeat warning", "never");

    health_globals.config.default_crit_repeat_every =
        config_get_duration(CONFIG_SECTION_HEALTH, "default repeat critical", "never");

    health_globals.config.health_log_entries_max =
        config_get_number(CONFIG_SECTION_HEALTH, "in memory max health log entries",
                          health_globals.config.health_log_entries_max);

    health_globals.config.health_log_history =
        config_get_number(CONFIG_SECTION_HEALTH, "health log history", HEALTH_LOG_DEFAULT_HISTORY);

    snprintfz(filename, FILENAME_MAX, "%s/alarm-notify.sh", netdata_configured_primary_plugins_dir);
    health_globals.config.default_exec =
        string_strdupz(config_get(CONFIG_SECTION_HEALTH, "script to execute on alarm", filename));

    health_globals.config.enabled_alerts =
        simple_pattern_create(config_get(CONFIG_SECTION_HEALTH, "enabled alarms", "*"),
                              NULL, SIMPLE_PATTERN_EXACT, true);

    health_globals.config.run_at_least_every_seconds =
        (int)config_get_number(CONFIG_SECTION_HEALTH,
                               "run at least every seconds",
                               health_globals.config.run_at_least_every_seconds);

    health_globals.config.postpone_alarms_during_hibernation_for_seconds =
        config_get_number(CONFIG_SECTION_HEALTH,
                          "postpone alarms during hibernation for seconds",
                          health_globals.config.postpone_alarms_during_hibernation_for_seconds);

    health_globals.config.default_recipient =
        string_strdupz("root");

    // ------------------------------------------------------------------------
    // verify after loading

    if(health_globals.config.run_at_least_every_seconds < 1)
        health_globals.config.run_at_least_every_seconds = 1;

    if(health_globals.config.health_log_entries_max < HEALTH_LOG_ENTRIES_MIN) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "Health configuration has invalid max log entries %u, using minimum of %u",
               health_globals.config.health_log_entries_max,
            HEALTH_LOG_ENTRIES_MIN);

        health_globals.config.health_log_entries_max = HEALTH_LOG_ENTRIES_MIN;
        config_set_number(CONFIG_SECTION_HEALTH, "in memory max health log entries",
                          (long)health_globals.config.health_log_entries_max);
    }
    else if(health_globals.config.health_log_entries_max > HEALTH_LOG_ENTRIES_MAX) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "Health configuration has invalid max log entries %u, using maximum of %u",
               health_globals.config.health_log_entries_max,
            HEALTH_LOG_ENTRIES_MAX);

        health_globals.config.health_log_entries_max = HEALTH_LOG_ENTRIES_MAX;
        config_set_number(CONFIG_SECTION_HEALTH, "in memory max health log entries",
                          (long)health_globals.config.health_log_entries_max);
    }

    if (health_globals.config.health_log_history < HEALTH_LOG_MINIMUM_HISTORY) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "Health configuration has invalid health log history %u. Using minimum %d",
               health_globals.config.health_log_history, HEALTH_LOG_MINIMUM_HISTORY);

        health_globals.config.health_log_history = HEALTH_LOG_MINIMUM_HISTORY;
        config_set_number(CONFIG_SECTION_HEALTH, "health log history", health_globals.config.health_log_history);
    }

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "Health log history is set to %u seconds (%u days)",
           health_globals.config.health_log_history, health_globals.config.health_log_history / 86400);
}

static inline void buffer_append_prototype_key(BUFFER *wb, const char *key, const char *txt) {
    if(unlikely(!txt || !*txt || strcmp(txt, "*") == 0 || strcmp(txt, "!*") == 0 || strcmp(txt, "!* *") == 0)) return;

    if(key) {
        buffer_putc(wb, ',');
        buffer_strcat(wb, key);
        buffer_putc(wb, '[');
    }

    while(txt && *txt) {
        if(*txt <= ' ' || *txt == ':' || *txt == '"' || *txt == '\'') {
            buffer_putc(wb, ',');
            txt++;
            while(*txt && (*txt <= ' ' || *txt == ':' || *txt == '"' || *txt == '\'')) txt++;
        }
        else {
            buffer_putc(wb, *txt);
            txt++;
        }
    }
    if(key) {
        buffer_putc(wb, ']');
    }
}

STRING *health_alert_config_dyncfg_key(struct rrd_alert_match *am, const char *name, RRDHOST *host, RRDSET *st) {
    CLEAN_BUFFER *buffer = buffer_create(1024, NULL);

    if(!host && st)
        host = st->rrdhost;

    if(host && st) {
        buffer_sprintf(buffer, "health:alert:node[%s]:", rrdhost_hostname(host));
        buffer_append_prototype_key(buffer, NULL, name);
        buffer_append_prototype_key(buffer, "on", rrdset_name(st));
    }
    else if(host && !st) {
        if (am->is_template)
            buffer_sprintf(buffer, "health:alert:prototype:node[%s]:template:", rrdhost_hostname(host));
        else
            buffer_sprintf(buffer, "health:alert:prototype:node[%s]:alert:", rrdhost_hostname(host));

        buffer_append_prototype_key(buffer, NULL, name);

        if (am->is_template)
            buffer_append_prototype_key(buffer, "on", string2str(am->on.context));
        else
            buffer_append_prototype_key(buffer, "on", string2str(am->on.chart));

        if (am->plugin)
            buffer_append_prototype_key(buffer, "plugin", string2str(am->plugin));

        if (am->module)
            buffer_append_prototype_key(buffer, "module", string2str(am->module));

        if (am->chart_labels)
            buffer_append_prototype_key(buffer, "instance_labels", string2str(am->chart_labels));
    }
    else {
        // both rrdhost and rrdset are missing
        const char *type;
        if (am->is_template)
            type = "health:alert:prototype:global:template:";
        else
            type = "health:alert:prototype:global:alert:";

        buffer_strcat(buffer, type);
        buffer_append_prototype_key(buffer, NULL, name);

        if (am->is_template)
            buffer_append_prototype_key(buffer, "on", string2str(am->on.context));
        else
            buffer_append_prototype_key(buffer, "on", string2str(am->on.chart));

        if (am->host)
            buffer_append_prototype_key(buffer, "node", string2str(am->host));

        if (am->os)
            buffer_append_prototype_key(buffer, "os", string2str(am->os));

        if (am->host_labels)
            buffer_append_prototype_key(buffer, "node_labels", string2str(am->host_labels));

        if (am->plugin)
            buffer_append_prototype_key(buffer, "plugin", string2str(am->plugin));

        if (am->module)
            buffer_append_prototype_key(buffer, "module", string2str(am->module));

        if (am->chart_labels)
            buffer_append_prototype_key(buffer, "instance_labels", string2str(am->chart_labels));
    }

    const char *final = buffer_tostring(buffer);
    return string_strdupz(final);
}

void health_add_prototype_unsafe(RRD_ALERT_PROTOTYPE *ap) {
    ap->match.dyncfg_prototype = health_alert_config_dyncfg_key(&ap->match, string2str(ap->config.name), NULL, NULL);
    ap->config.dyncfg_key = string_dup(ap->match.dyncfg_prototype);

    sql_alert_hash_and_store_config(ap->config.hash_id, &ap->sql, health_globals.sql_store_hashes);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(health_globals.prototypes.base, ap, prev, next);
}

static void health_prototype_free(RRD_ALERT_PROTOTYPE *ap) {
    rrd_alert_match_free(&ap->match);
    rrd_alert_config_free(&ap->config);
    sql_alert_config_free(&ap->sql);
}

void health_reload_prototypes(void) {
    spinlock_lock(&health_globals.prototypes.spinlock);

    while(health_globals.prototypes.base) {
        RRD_ALERT_PROTOTYPE *ap = health_globals.prototypes.base;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(health_globals.prototypes.base, ap, prev, next);
        health_prototype_free(ap);
    }

    recursive_config_double_dir_load(
        health_user_config_dir(),
        health_globals.config.stock_enabled ? health_stock_config_dir() : NULL,
        NULL,
        health_readfile,
        NULL, 0);

    sql_alert_config_hashes_store_disable();

    spinlock_unlock(&health_globals.prototypes.spinlock);
}

static void health_copy_match_without_patterns(struct rrd_alert_match *dst, struct rrd_alert_match *src) {
    dst->dyncfg_prototype = string_dup(src->dyncfg_prototype);

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

static void health_copy_config(struct rrd_alert_config *dst, struct rrd_alert_config *src) {
    dst->dyncfg_key = string_dup(src->dyncfg_key);

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

    if(src->foreach_dimension) {
        dst->foreach_dimension = string_dup(src->foreach_dimension);
        dst->foreach_dimension_pattern =
            simple_pattern_create(string2str(dst->foreach_dimension),
                                  NULL, SIMPLE_PATTERN_EXACT, true);
    }

    dst->group = src->group;
    dst->before = src->before;
    dst->after = src->after;
    dst->options = src->options;

    const char *failed_at = NULL;
    int error = 0;

    if(src->calculation)
        dst->calculation = expression_parse(string2str(src->calculation->source), &failed_at, &error);

    if(src->warning)
        dst->warning = expression_parse(string2str(src->warning->source), &failed_at, &error);

    if(src->critical)
        dst->critical = expression_parse(string2str(src->critical->source), &failed_at, &error);


    dst->delay_up_duration = src->delay_up_duration;
    dst->delay_down_duration = src->delay_down_duration;
    dst->delay_max_duration = src->delay_max_duration;
    dst->delay_multiplier = src->delay_multiplier;

    dst->has_custom_repeat_config = src->has_custom_repeat_config;
    dst->warn_repeat_every = src->warn_repeat_every;
    dst->crit_repeat_every = src->crit_repeat_every;
}

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

void health_activate_match_patterns(struct rrd_alert_match *am) {
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

static RRDCALCTEMPLATE *health_rrdcalctemplate_from_prototype(RRDHOST *host, RRD_ALERT_PROTOTYPE *ap) {
    ap->uses++;

    RRDCALCTEMPLATE *rt = callocz(1, sizeof(RRDCALCTEMPLATE));

    health_copy_config(&rt->config, &ap->config);
    health_copy_match_without_patterns(&rt->match, &ap->match);
    health_activate_match_patterns(&rt->match);
    rt->context = ap->match.is_template ? ap->match.on.context : ap->match.on.chart;

    string_freez(rt->config.dyncfg_key);
    rt->config.dyncfg_key = health_alert_config_dyncfg_key(&ap->match, string2str(ap->config.name), host, NULL);

    if(!ap->config.has_custom_repeat_config) {
        rt->config.warn_repeat_every = host->health.health_default_warn_repeat_every;
        rt->config.crit_repeat_every = host->health.health_default_crit_repeat_every;
    }

    return rt;
}

static RRDCALC *health_rrdcalc_from_prototype(RRDHOST *host, RRD_ALERT_PROTOTYPE *ap) {
    ap->uses++;

    RRDCALC *rc = callocz(1, sizeof(RRDCALC));

    health_copy_config(&rc->config, &ap->config);
    health_copy_match_without_patterns(&rc->match, &ap->match);
    health_activate_match_patterns(&rc->match);
    rc->chart = ap->match.is_template ? ap->match.on.context : ap->match.on.chart;

    string_freez(rc->config.dyncfg_key);
    rc->config.dyncfg_key = health_alert_config_dyncfg_key(&ap->match, string2str(ap->config.name), host, NULL);

    if(!ap->config.has_custom_repeat_config) {
        rc->config.warn_repeat_every = host->health.health_default_warn_repeat_every;
        rc->config.crit_repeat_every = host->health.health_default_crit_repeat_every;
    }

    rc->next_event_id = 1;
    rc->value = NAN;
    rc->old_value = NAN;
    rc->old_status = RRDCALC_STATUS_REMOVED;

    return rc;
}

static bool prototype_matches_host(RRDHOST *host, struct rrd_alert_prototype *ap) {
    if(!simple_pattern_matches(health_globals.config.enabled_alerts, string2str(ap->config.name)))
        return false;

    if(ap->match.os_pattern && !simple_pattern_matches_string(ap->match.os_pattern, host->os))
        return false;

    if(host->rrdlabels && ap->match.host_labels_pattern &&
        !rrdlabels_match_simple_pattern_parsed(host->rrdlabels, ap->match.host_labels_pattern, '=', NULL))
        return false;

    return true;
}

void health_apply_prototypes_to_host(RRDHOST *host) {
    if(unlikely(!host->health.health_enabled) && !rrdhost_flag_check(host, RRDHOST_FLAG_INITIALIZED_HEALTH))
        return;

    // free all running alarms
    rrdcalc_delete_all(host);
    rrdcalctemplate_delete_all(host);

    // invalidate all previous entries in the alarm log
    rw_spinlock_read_lock(&host->health_log.spinlock);
    ALARM_ENTRY *t;
    for(t = host->health_log.alarms ; t ; t = t->next) {
        if(t->new_status != RRDCALC_STATUS_REMOVED)
            t->flags |= HEALTH_ENTRY_FLAG_UPDATED;
    }
    rw_spinlock_read_unlock(&host->health_log.spinlock);

    // reset all thresholds to all charts
    RRDSET *st;
    rrdset_foreach_read(st, host) {
        st->green = NAN;
        st->red = NAN;
    }
    rrdset_foreach_done(st);


    spinlock_lock(&health_globals.prototypes.spinlock);
    for(struct rrd_alert_prototype *ap = health_globals.prototypes.base; ap ;ap = ap->next) {
        if(!prototype_matches_host(host, ap))
            return;

        if(ap->match.is_template) {
            RRDCALCTEMPLATE *rt = health_rrdcalctemplate_from_prototype(host, ap);
            rrdcalctemplate_add_from_config(host, rt);
        }
        else {
            RRDCALC *rc = health_rrdcalc_from_prototype(host, ap);
            rrdcalc_add_from_config(host, rc);
        }
    }
    spinlock_unlock(&health_globals.prototypes.spinlock);

    // link the loaded alarms to their charts
    rrdset_foreach_write(st, host) {
        rrdcalc_link_matching_alerts_to_rrdset(st);
        rrdcalctemplate_link_matching_templates_to_rrdset(st);
    }
    rrdset_foreach_done(st);

    //Discard alarms with labels that do not apply to host
    rrdcalc_delete_alerts_not_matching_host_labels_from_this_host(host);

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


void sql_alert_config_hashes_store_enable(void) {
    health_globals.sql_store_hashes = true;
}

void sql_alert_config_hashes_store_disable(void) {
    health_globals.sql_store_hashes = false;
}

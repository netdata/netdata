// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd.h"

// ----------------------------------------------------------------------------
// RRDCALC helpers

inline const char *rrdcalc_status2string(RRDCALC_STATUS status) {
    switch(status) {
        case RRDCALC_STATUS_REMOVED:
            return "REMOVED";

        case RRDCALC_STATUS_UNDEFINED:
            return "UNDEFINED";

        case RRDCALC_STATUS_UNINITIALIZED:
            return "UNINITIALIZED";

        case RRDCALC_STATUS_CLEAR:
            return "CLEAR";

        case RRDCALC_STATUS_RAISED:
            return "RAISED";

        case RRDCALC_STATUS_WARNING:
            return "WARNING";

        case RRDCALC_STATUS_CRITICAL:
            return "CRITICAL";

        default:
            error("Unknown alarm status %d", status);
            return "UNKNOWN";
    }
}

uint32_t rrdcalc_get_unique_id(RRDHOST *host, STRING *chart, STRING *name, uint32_t *next_event_id) {
    netdata_rwlock_rdlock(&host->health_log.alarm_log_rwlock);

    // re-use old IDs, by looking them up in the alarm log
    ALARM_ENTRY *ae = NULL;
    for(ae = host->health_log.alarms; ae ;ae = ae->next) {
        if(unlikely(name == ae->name && chart == ae->chart)) {
            if(next_event_id) *next_event_id = ae->alarm_event_id + 1;
            break;
        }
    }

    uint32_t alarm_id;

    if(ae)
        alarm_id = ae->alarm_id;

    else {
        if (unlikely(!host->health_log.next_alarm_id))
            host->health_log.next_alarm_id = (uint32_t)now_realtime_sec();

        alarm_id = host->health_log.next_alarm_id++;
    }

    netdata_rwlock_unlock(&host->health_log.alarm_log_rwlock);
    return alarm_id;
}

// ----------------------------------------------------------------------------
// RRDCALC replacing info text variables with RRDSET labels

static STRING *rrdcalc_replace_variables_with_rrdset_labels(const char *line, RRDCALC *rc) {
    if (!line || !*line)
        return NULL;

    size_t pos = 0;
    char *temp = strdupz(line);
    char var[RRDCALC_VAR_MAX];
    char *m, *lbl_value = NULL;

    while ((m = strchr(temp + pos, '$')) && *(m+1) == '{') {
        int i = 0;
        char *e = m;
        while (*e) {
            var[i++] = *e;

            if (*e == '}' || i == RRDCALC_VAR_MAX - 1)
                break;

            e++;
        }

        var[i] = '\0';
        pos = m - temp + 1;

        if (!strcmp(var, RRDCALC_VAR_FAMILY)) {
            char *buf = find_and_replace(temp, var, (rc->rrdset && rc->rrdset->family) ? rrdset_family(rc->rrdset) : "", m);
            freez(temp);
            temp = buf;
        }
        else if (!strncmp(var, RRDCALC_VAR_LABEL, RRDCALC_VAR_LABEL_LEN)) {
            char label_val[RRDCALC_VAR_MAX + RRDCALC_VAR_LABEL_LEN + 1] = { 0 };
            strcpy(label_val, var+RRDCALC_VAR_LABEL_LEN);
            label_val[i - RRDCALC_VAR_LABEL_LEN - 1] = '\0';

            if(likely(rc->rrdset && rc->rrdset->rrdlabels)) {
                rrdlabels_get_value_strdup_or_null(rc->rrdset->rrdlabels, &lbl_value, label_val);
                if (lbl_value) {
                    char *buf = find_and_replace(temp, var, lbl_value, m);
                    freez(temp);
                    temp = buf;
                    freez(lbl_value);
                }
            }
        }
    }

    STRING *ret = string_strdupz(temp);
    freez(temp);

    return ret;
}

void rrdcalc_update_info_using_rrdset_labels(RRDCALC *rc) {
    if(!rc->rrdset || !rc->original_info || !rc->rrdset->rrdlabels) return;

    size_t labels_version = dictionary_version(rc->rrdset->rrdlabels);
    if(rc->labels_version != labels_version) {

        STRING *old = rc->info;
        rc->info = rrdcalc_replace_variables_with_rrdset_labels(rrdcalc_original_info(rc), rc);
        string_freez(old);

        rc->labels_version = labels_version;
    }
}

// ----------------------------------------------------------------------------
// RRDCALC index management for RRDSET

// the dictionary requires a unique key for every item
// we use {chart id}.{alert name} for both the RRDHOST and RRDSET alert indexes.

#define RRDCALC_MAX_KEY_SIZE 1024
static size_t rrdcalc_key(char *dst, size_t dst_len, const char *chart, const char *alert) {
    return snprintfz(dst, dst_len, "%s/%s", chart, alert);
}

const RRDCALC_ACQUIRED *rrdcalc_from_rrdset_get(RRDSET *st, const char *alert_name) {
    char key[RRDCALC_MAX_KEY_SIZE + 1];
    size_t key_len = rrdcalc_key(key, RRDCALC_MAX_KEY_SIZE, rrdset_id(st), alert_name);

    const RRDCALC_ACQUIRED *rca = (const RRDCALC_ACQUIRED *)dictionary_get_and_acquire_item_advanced(st->rrdhost->rrdcalc_root_index, key, (ssize_t)(key_len + 1));

    if(!rca) {
        key_len = rrdcalc_key(key, RRDCALC_MAX_KEY_SIZE, rrdset_name(st), alert_name);
        rca = (const RRDCALC_ACQUIRED *)dictionary_get_and_acquire_item_advanced(st->rrdhost->rrdcalc_root_index, key, (ssize_t)(key_len + 1));
    }

    return rca;
}

void rrdcalc_from_rrdset_release(RRDSET *st, const RRDCALC_ACQUIRED *rca) {
    if(!rca) return;

    dictionary_acquired_item_release(st->rrdhost->rrdcalc_root_index, (const DICTIONARY_ITEM *)rca);
}

RRDCALC *rrdcalc_acquired_to_rrdcalc(const RRDCALC_ACQUIRED *rca) {
    if(rca)
        return dictionary_acquired_item_value((const DICTIONARY_ITEM *)rca);

    return NULL;
}

// ----------------------------------------------------------------------------
// RRDCALC managing the linking with RRDSET

static void rrdcalc_link_to_rrdset(RRDSET *st, RRDCALC *rc) {
    RRDHOST *host = st->rrdhost;

    debug(D_HEALTH, "Health linking alarm '%s.%s' to chart '%s' of host '%s'", rrdcalc_chart_name(rc), rrdcalc_name(rc), rrdset_id(st), rrdhost_hostname(host));

    rc->last_status_change = now_realtime_sec();
    rc->rrdset = st;

    netdata_rwlock_wrlock(&st->alerts.rwlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(st->alerts.base, rc, prev, next);
    netdata_rwlock_unlock(&st->alerts.rwlock);

    if(rc->update_every < rc->rrdset->update_every) {
        error("Health alarm '%s.%s' has update every %d, less than chart update every %d. Setting alarm update frequency to %d.", rrdset_id(rc->rrdset), rrdcalc_name(rc), rc->update_every, rc->rrdset->update_every, rc->rrdset->update_every);
        rc->update_every = rc->rrdset->update_every;
    }

    if(!isnan(rc->green) && isnan(st->green)) {
        debug(D_HEALTH, "Health alarm '%s.%s' green threshold set from " NETDATA_DOUBLE_FORMAT_AUTO
                        " to " NETDATA_DOUBLE_FORMAT_AUTO ".", rrdset_id(rc->rrdset), rrdcalc_name(rc), rc->rrdset->green, rc->green);
        st->green = rc->green;
    }

    if(!isnan(rc->red) && isnan(st->red)) {
        debug(D_HEALTH, "Health alarm '%s.%s' red threshold set from " NETDATA_DOUBLE_FORMAT_AUTO " to " NETDATA_DOUBLE_FORMAT_AUTO
                        ".", rrdset_id(rc->rrdset), rrdcalc_name(rc), rc->rrdset->red, rc->red);
        st->red = rc->red;
    }

    char buf[RRDVAR_MAX_LENGTH + 1];
    snprintfz(buf, RRDVAR_MAX_LENGTH, "%s.%s", rrdset_name(st), rrdcalc_name(rc));
    STRING *rrdset_name_rrdcalc_name = string_strdupz(buf);
    snprintfz(buf, RRDVAR_MAX_LENGTH, "%s.%s", rrdset_id(st), rrdcalc_name(rc));
    STRING *rrdset_id_rrdcalc_name = string_strdupz(buf);

    rc->rrdvar_local = rrdvar_add_and_acquire(
        "local",
        st->rrdvars,
        rc->name,
        RRDVAR_TYPE_CALCULATED,
        RRDVAR_FLAG_RRDCALC_LOCAL_VAR,
        &rc->value);

    rc->rrdvar_family = rrdvar_add_and_acquire(
        "family",
        rrdfamily_rrdvars_dict(st->rrdfamily),
        rc->name,
        RRDVAR_TYPE_CALCULATED,
        RRDVAR_FLAG_RRDCALC_FAMILY_VAR,
        &rc->value);

    rc->rrdvar_host_chart_name = rrdvar_add_and_acquire(
        "host",
        host->rrdvars,
        rrdset_name_rrdcalc_name,
        RRDVAR_TYPE_CALCULATED,
        RRDVAR_FLAG_RRDCALC_HOST_CHARTNAME_VAR,
        &rc->value);

    rc->rrdvar_host_chart_id = rrdvar_add_and_acquire(
        "host",
        host->rrdvars,
        rrdset_id_rrdcalc_name,
        RRDVAR_TYPE_CALCULATED,
        RRDVAR_FLAG_RRDCALC_HOST_CHARTID_VAR | ((rc->rrdvar_host_chart_name) ? 0 : RRDVAR_FLAG_RRDCALC_HOST_CHARTNAME_VAR),
        &rc->value);

    string_freez(rrdset_id_rrdcalc_name);
    string_freez(rrdset_name_rrdcalc_name);

    if(!rc->units)
        rc->units = string_dup(st->units);

    rrdvar_store_for_chart(host, st);

    rrdcalc_update_info_using_rrdset_labels(rc);

    time_t now = now_realtime_sec();

    ALARM_ENTRY *ae = health_create_alarm_entry(
        host,
        rc->id,
        rc->next_event_id++,
        rc->config_hash_id,
        now,
        rc->name,
        rc->rrdset->id,
        rc->rrdset->context,
        rc->rrdset->family,
        rc->classification,
        rc->component,
        rc->type,
        rc->exec,
        rc->recipient,
        now - rc->last_status_change,
        rc->old_value,
        rc->value,
        rc->status,
        RRDCALC_STATUS_UNINITIALIZED,
        rc->source,
        rc->units,
        rc->info,
        0,
        rrdcalc_isrepeating(rc)?HEALTH_ENTRY_FLAG_IS_REPEATING:0);

    health_alarm_log_add_entry(host, ae);
}

static void rrdcalc_unlink_from_rrdset(RRDCALC *rc, bool having_ll_wrlock) {
    RRDSET *st = rc->rrdset;

    if(!st) {
        debug(D_HEALTH, "Requested to unlink RRDCALC '%s.%s' which is not linked to any RRDSET", rrdcalc_chart_name(rc), rrdcalc_name(rc));
        error("Requested to unlink RRDCALC '%s.%s' which is not linked to any RRDSET", rrdcalc_chart_name(rc), rrdcalc_name(rc));
        return;
    }

    RRDHOST *host = st->rrdhost;

    time_t now = now_realtime_sec();

    if (likely(rc->status != RRDCALC_STATUS_REMOVED)) {
        ALARM_ENTRY *ae = health_create_alarm_entry(
            host,
            rc->id,
            rc->next_event_id++,
            rc->config_hash_id,
            now,
            rc->name,
            rc->rrdset->id,
            rc->rrdset->context,
            rc->rrdset->family,
            rc->classification,
            rc->component,
            rc->type,
            rc->exec,
            rc->recipient,
            now - rc->last_status_change,
            rc->old_value,
            rc->value,
            rc->status,
            RRDCALC_STATUS_REMOVED,
            rc->source,
            rc->units,
            rc->info,
            0,
            0);

        health_alarm_log_add_entry(host, ae);
    }

    debug(D_HEALTH, "Health unlinking alarm '%s.%s' from chart '%s' of host '%s'", rrdcalc_chart_name(rc), rrdcalc_name(rc), rrdset_id(st), rrdhost_hostname(host));

    // unlink it

    if(!having_ll_wrlock)
        netdata_rwlock_wrlock(&st->alerts.rwlock);

    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(st->alerts.base, rc, prev, next);

    if(!having_ll_wrlock)
        netdata_rwlock_unlock(&st->alerts.rwlock);

    rc->rrdset = NULL;

    rrdvar_release_and_del(st->rrdvars, rc->rrdvar_local);
    rc->rrdvar_local = NULL;

    rrdvar_release_and_del(rrdfamily_rrdvars_dict(st->rrdfamily), rc->rrdvar_family);
    rc->rrdvar_family = NULL;

    rrdvar_release_and_del(host->rrdvars, rc->rrdvar_host_chart_id);
    rc->rrdvar_host_chart_id = NULL;

    rrdvar_release_and_del(host->rrdvars, rc->rrdvar_host_chart_name);
    rc->rrdvar_host_chart_name = NULL;

    // RRDCALC will remain in RRDHOST
    // so that if the matching chart is found in the future
    // it will be applied automatically
}

static inline bool rrdcalc_check_if_it_matches_rrdset(RRDCALC *rc, RRDSET *st) {
    if (   (rc->chart != st->id)
        && (rc->chart != st->name))
        return false;

    if (rc->module_pattern && !simple_pattern_matches_string(rc->module_pattern, st->module_name))
        return false;

    if (rc->plugin_pattern && !simple_pattern_matches_string(rc->plugin_pattern, st->module_name))
        return false;

    if (st->rrdhost->rrdlabels && rc->host_labels_pattern && !rrdlabels_match_simple_pattern_parsed(
            st->rrdhost->rrdlabels, rc->host_labels_pattern, '=', NULL))
        return false;

    if (st->rrdlabels && rc->chart_labels_pattern && !rrdlabels_match_simple_pattern_parsed(
            st->rrdlabels, rc->chart_labels_pattern, '=', NULL))
        return false;

    return true;
}

void rrdcalc_link_matching_alerts_to_rrdset(RRDSET *st) {
    RRDHOST *host = st->rrdhost;
    // debug(D_HEALTH, "find matching alarms for chart '%s'", st->id);

    RRDCALC *rc;
    foreach_rrdcalc_in_rrdhost_read(host, rc) {
        if(rc->rrdset)
            continue;

        if(unlikely(rrdcalc_check_if_it_matches_rrdset(rc, st)))
            rrdcalc_link_to_rrdset(st, rc);
    }
    foreach_rrdcalc_in_rrdhost_done(rc);
}

static inline int rrdcalc_check_and_link_rrdset_callback(RRDSET *st, void *rrdcalc) {
    RRDCALC *rc = rrdcalc;

    if(unlikely(rrdcalc_check_if_it_matches_rrdset(rc, st))) {
        rrdcalc_link_to_rrdset(st, rc);
        return -1;
    }

    return 0;
}

// ----------------------------------------------------------------------------
// RRDCALC rrdhost index management - constructor

struct rrdcalc_constructor {
    RRDHOST *rrdhost;                       // the host we operate upon
    RRDCALC *from_config;                   // points to the original RRDCALC, as loaded from the config
    RRDCALCTEMPLATE *from_rrdcalctemplate;  // the template this alert is generated from
    RRDSET *rrdset;                         // when this comes from rrdcalctemplate, we have a matching rrdset
    const char *overwrite_alert_name;       // when we have a dimension foreach, the alert is renamed
    const char *overwrite_dimensions;       // when we have a dimension foreach, the dimensions filter is renamed

    enum {
        RRDCALC_REACT_NONE,
        RRDCALC_REACT_NEW,
    } react_action;

    bool existing_from_template;
};

static void rrdcalc_rrdhost_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdcalc, void *constructor_data) {
    RRDCALC *rc = rrdcalc;
    struct rrdcalc_constructor *ctr = constructor_data;
    RRDHOST *host = ctr->rrdhost;

    rc->key = string_strdupz(dictionary_acquired_item_name(item));

    if(ctr->from_rrdcalctemplate) {
        rc->run_flags |= RRDCALC_FLAG_FROM_TEMPLATE;

        RRDCALCTEMPLATE *rt = ctr->from_rrdcalctemplate;
        RRDSET *st = ctr->rrdset;

        rc->next_event_id = 1;
        rc->name = (ctr->overwrite_alert_name) ? string_strdupz(ctr->overwrite_alert_name) : string_dup(rt->name);
        rc->chart = string_dup(st->id);
        uuid_copy(rc->config_hash_id, rt->config_hash_id);

        rc->dimensions = (ctr->overwrite_dimensions) ? string_strdupz(ctr->overwrite_dimensions) : string_dup(rt->dimensions);
        rc->foreach_dimension = NULL;
        rc->foreach_dimension_pattern = NULL;

        rc->green = rt->green;
        rc->red = rt->red;
        rc->value = NAN;
        rc->old_value = NAN;

        rc->delay_up_duration = rt->delay_up_duration;
        rc->delay_down_duration = rt->delay_down_duration;
        rc->delay_max_duration = rt->delay_max_duration;
        rc->delay_multiplier = rt->delay_multiplier;

        rc->last_repeat = 0;
        rc->times_repeat = 0;
        rc->warn_repeat_every = rt->warn_repeat_every;
        rc->crit_repeat_every = rt->crit_repeat_every;

        rc->group = rt->group;
        rc->after = rt->after;
        rc->before = rt->before;
        rc->update_every = rt->update_every;
        rc->options = rt->options;

        rc->exec = string_dup(rt->exec);
        rc->recipient = string_dup(rt->recipient);
        rc->source = string_dup(rt->source);
        rc->units = string_dup(rt->units);
        rc->info = string_dup(rt->info);
        rc->original_info = string_dup(rt->info);

        rc->classification = string_dup(rt->classification);
        rc->component = string_dup(rt->component);
        rc->type = string_dup(rt->type);

        if(rt->calculation) {
            rc->calculation = expression_parse(rt->calculation->source, NULL, NULL);
            if(!rc->calculation)
                error("Health alarm '%s.%s': failed to parse calculation expression '%s'", rrdset_id(st), rrdcalctemplate_name(rt), rt->calculation->source);
        }
        if(rt->warning) {
            rc->warning = expression_parse(rt->warning->source, NULL, NULL);
            if(!rc->warning)
                error("Health alarm '%s.%s': failed to re-parse warning expression '%s'", rrdset_id(st), rrdcalctemplate_name(rt), rt->warning->source);
        }
        if(rt->critical) {
            rc->critical = expression_parse(rt->critical->source, NULL, NULL);
            if(!rc->critical)
                error("Health alarm '%s.%s': failed to re-parse critical expression '%s'", rrdset_id(st), rrdcalctemplate_name(rt), rt->critical->source);
        }
    }
    else if(ctr->from_config) {
        // dictionary has already copied all the members values and pointers
        // no need for additional work in this case
        ;
    }

    rc->id = rrdcalc_get_unique_id(host, rc->chart, rc->name, &rc->next_event_id);

    if(rc->calculation) {
        rc->calculation->status = &rc->status;
        rc->calculation->myself = &rc->value;
        rc->calculation->after = &rc->db_after;
        rc->calculation->before = &rc->db_before;
        rc->calculation->rrdcalc = rc;
    }

    if(rc->warning) {
        rc->warning->status = &rc->status;
        rc->warning->myself = &rc->value;
        rc->warning->after = &rc->db_after;
        rc->warning->before = &rc->db_before;
        rc->warning->rrdcalc = rc;
    }

    if(rc->critical) {
        rc->critical->status = &rc->status;
        rc->critical->myself = &rc->value;
        rc->critical->after = &rc->db_after;
        rc->critical->before = &rc->db_before;
        rc->critical->rrdcalc = rc;
    }

    debug(D_HEALTH, "Health added alarm '%s.%s': exec '%s', recipient '%s', green " NETDATA_DOUBLE_FORMAT_AUTO
                    ", red " NETDATA_DOUBLE_FORMAT_AUTO
                    ", lookup: group %d, after %d, before %d, options %u, dimensions '%s', for each dimension '%s', update every %d, calculation '%s', warning '%s', critical '%s', source '%s', delay up %d, delay down %d, delay max %d, delay_multiplier %f, warn_repeat_every %u, crit_repeat_every %u",
          rrdcalc_chart_name(rc),
          rrdcalc_name(rc),
          (rc->exec)?rrdcalc_exec(rc):"DEFAULT",
          (rc->recipient)?rrdcalc_recipient(rc):"DEFAULT",
          rc->green,
          rc->red,
          (int)rc->group,
          rc->after,
          rc->before,
          rc->options,
          (rc->dimensions)?rrdcalc_dimensions(rc):"NONE",
          (rc->foreach_dimension)?rrdcalc_foreachdim(rc):"NONE",
          rc->update_every,
          (rc->calculation)?rc->calculation->parsed_as:"NONE",
          (rc->warning)?rc->warning->parsed_as:"NONE",
          (rc->critical)?rc->critical->parsed_as:"NONE",
          rrdcalc_source(rc),
          rc->delay_up_duration,
          rc->delay_down_duration,
          rc->delay_max_duration,
          rc->delay_multiplier,
          rc->warn_repeat_every,
          rc->crit_repeat_every
    );

    ctr->react_action = RRDCALC_REACT_NEW;
}

static bool rrdcalc_rrdhost_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdcalc, void *rrdcalc_new __maybe_unused, void *constructor_data ) {
    RRDCALC *rc = rrdcalc;
    struct rrdcalc_constructor *ctr = constructor_data;

    if(rc->run_flags & RRDCALC_FLAG_FROM_TEMPLATE)
        ctr->existing_from_template = true;
    else
        ctr->existing_from_template = false;

    ctr->react_action = RRDCALC_REACT_NONE;

    return false;
}

static void rrdcalc_rrdhost_react_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdcalc, void *constructor_data) {
    RRDCALC *rc = rrdcalc;
    struct rrdcalc_constructor *ctr = constructor_data;
    RRDHOST *host = ctr->rrdhost;

    if(ctr->react_action == RRDCALC_REACT_NEW) {
        if(ctr->rrdset)
            rrdcalc_link_to_rrdset(ctr->rrdset, rc);

        else if (ctr->from_rrdcalctemplate)
            rrdcontext_foreach_instance_with_rrdset_in_context(host, string2str(ctr->from_rrdcalctemplate->context), rrdcalc_check_and_link_rrdset_callback, rc);
    }
}

// ----------------------------------------------------------------------------
// RRDCALC rrdhost index management - destructor

static void rrdcalc_free_internals(RRDCALC *rc) {
    if(unlikely(!rc)) return;

    expression_free(rc->calculation);
    expression_free(rc->warning);
    expression_free(rc->critical);

    string_freez(rc->key);
    string_freez(rc->name);
    string_freez(rc->chart);
    string_freez(rc->dimensions);
    string_freez(rc->foreach_dimension);
    string_freez(rc->exec);
    string_freez(rc->recipient);
    string_freez(rc->source);
    string_freez(rc->units);
    string_freez(rc->info);
    string_freez(rc->original_info);
    string_freez(rc->classification);
    string_freez(rc->component);
    string_freez(rc->type);
    string_freez(rc->host_labels);
    string_freez(rc->module_match);
    string_freez(rc->plugin_match);
    string_freez(rc->chart_labels);

    simple_pattern_free(rc->foreach_dimension_pattern);
    simple_pattern_free(rc->host_labels_pattern);
    simple_pattern_free(rc->module_pattern);
    simple_pattern_free(rc->plugin_pattern);
    simple_pattern_free(rc->chart_labels_pattern);
}

static void rrdcalc_rrdhost_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdcalc, void *rrdhost __maybe_unused) {
    RRDCALC *rc = rrdcalc;
    //RRDHOST *host = rrdhost;

    if(unlikely(rc->rrdset))
        rrdcalc_unlink_from_rrdset(rc, false);

    // any destruction actions that require other locks
    // have to be placed in rrdcalc_del(), because the object is actually locked for deletion

    rrdcalc_free_internals(rc);
}

// ----------------------------------------------------------------------------
// RRDCALC rrdhost index management - index API

void rrdcalc_rrdhost_index_init(RRDHOST *host) {
    if(!host->rrdcalc_root_index) {
        host->rrdcalc_root_index = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                              &dictionary_stats_category_rrdhealth, sizeof(RRDCALC));

        dictionary_register_insert_callback(host->rrdcalc_root_index, rrdcalc_rrdhost_insert_callback, NULL);
        dictionary_register_conflict_callback(host->rrdcalc_root_index, rrdcalc_rrdhost_conflict_callback, NULL);
        dictionary_register_react_callback(host->rrdcalc_root_index, rrdcalc_rrdhost_react_callback, NULL);
        dictionary_register_delete_callback(host->rrdcalc_root_index, rrdcalc_rrdhost_delete_callback, host);
    }
}

void rrdcalc_rrdhost_index_destroy(RRDHOST *host) {
    dictionary_destroy(host->rrdcalc_root_index);
    host->rrdcalc_root_index = NULL;
}

void rrdcalc_add_from_rrdcalctemplate(RRDHOST *host, RRDCALCTEMPLATE *rt, RRDSET *st, const char *overwrite_alert_name, const char *overwrite_dimensions) {
    char key[RRDCALC_MAX_KEY_SIZE + 1];
    size_t key_len = rrdcalc_key(key, RRDCALC_MAX_KEY_SIZE, rrdset_id(st),
                                 overwrite_alert_name?overwrite_alert_name:string2str(rt->name));

    struct rrdcalc_constructor tmp = {
        .rrdhost = host,
        .from_config = NULL,
        .from_rrdcalctemplate = rt,
        .rrdset = st,
        .overwrite_alert_name = overwrite_alert_name,
        .overwrite_dimensions = overwrite_dimensions,
        .react_action = RRDCALC_REACT_NONE,
        .existing_from_template = false,
    };

    dictionary_set_advanced(host->rrdcalc_root_index, key, (ssize_t)(key_len + 1), NULL, sizeof(RRDCALC), &tmp);
    if(tmp.react_action != RRDCALC_REACT_NEW && tmp.existing_from_template == false)
        error("RRDCALC: from template '%s' on chart '%s' with key '%s', failed to be added to host '%s'. It is manually configured.",
              string2str(rt->name), rrdset_id(st), key, rrdhost_hostname(host));
}

int rrdcalc_add_from_config(RRDHOST *host, RRDCALC *rc) {
    if(!rc->chart) {
        error("Health configuration for alarm '%s' does not have a chart", rrdcalc_name(rc));
        return 0;
    }

    if(!rc->update_every) {
        error("Health configuration for alarm '%s.%s' has no frequency (parameter 'every'). Ignoring it.", rrdcalc_chart_name(rc), rrdcalc_name(rc));
        return 0;
    }

    if(!RRDCALC_HAS_DB_LOOKUP(rc) && !rc->calculation && !rc->warning && !rc->critical) {
        error("Health configuration for alarm '%s.%s' is useless (no db lookup, no calculation, no warning and no critical expressions)", rrdcalc_chart_name(rc), rrdcalc_name(rc));
        return 0;
    }

    char key[RRDCALC_MAX_KEY_SIZE + 1];
    size_t key_len = rrdcalc_key(key, RRDCALC_MAX_KEY_SIZE, string2str(rc->chart), string2str(rc->name));

    struct rrdcalc_constructor tmp = {
        .rrdhost = host,
        .from_config = rc,
        .from_rrdcalctemplate = NULL,
        .rrdset = NULL,
        .react_action = RRDCALC_REACT_NONE,
    };

    int ret = 1;
    RRDCALC *t = dictionary_set_advanced(host->rrdcalc_root_index, key, (ssize_t)(key_len + 1), rc, sizeof(RRDCALC), &tmp);
    if(tmp.react_action == RRDCALC_REACT_NEW) {
        // we copied rc into the dictionary, so we have to free the container here
        freez(rc);
        rc = t;

        // since we loaded this config from configuration, we need to check if we can link it to alarms
        RRDSET *st;
        rrdset_foreach_read(st, host) {
            if (unlikely(rrdcalc_check_and_link_rrdset_callback(st, rc) == -1))
                break;
        }
        rrdset_foreach_done(st);
    }
    else {
        error(
            "RRDCALC: from config '%s' on chart '%s' failed to be added to host '%s'. It already exists.",
            string2str(rc->name),
            string2str(rc->chart),
            rrdhost_hostname(host));

        ret = 0;

        // free all of it, internals and the container
        rrdcalc_free_unused_rrdcalc_loaded_from_config(rc);
    }

    return ret;
}

static void rrdcalc_unlink_and_delete(RRDHOST *host, RRDCALC *rc, bool having_ll_wrlock) {
    if(rc->rrdset)
        rrdcalc_unlink_from_rrdset(rc, having_ll_wrlock);

    dictionary_del_advanced(host->rrdcalc_root_index, string2str(rc->key), (ssize_t)string_strlen(rc->key) + 1);
}


// ----------------------------------------------------------------------------
// RRDCALC cleanup API functions

void rrdcalc_delete_alerts_not_matching_host_labels_from_this_host(RRDHOST *host) {
    RRDCALC *rc;
    foreach_rrdcalc_in_rrdhost_reentrant(host, rc) {
        if (!rc->host_labels)
            continue;

        if(!rrdlabels_match_simple_pattern_parsed(host->rrdlabels, rc->host_labels_pattern, '=', NULL)) {
            log_health("Health configuration for alarm '%s' cannot be applied, because the host %s does not have the label(s) '%s'",
                 rrdcalc_name(rc),
                 rrdhost_hostname(host),
                 rrdcalc_host_labels(rc));

            rrdcalc_unlink_and_delete(host, rc, false);
        }
    }
    foreach_rrdcalc_in_rrdhost_done(rc);
}

void rrdcalc_delete_alerts_not_matching_host_labels_from_all_hosts() {
    RRDHOST *host;
    dfe_start_reentrant(rrdhost_root_index, host) {
        if (unlikely(!host->health.health_enabled))
            continue;

        if (host->rrdlabels)
            rrdcalc_delete_alerts_not_matching_host_labels_from_this_host(host);
    }
    dfe_done(host);
}

void rrdcalc_unlink_all_rrdset_alerts(RRDSET *st) {
    RRDCALC *rc, *last = NULL;
    netdata_rwlock_wrlock(&st->alerts.rwlock);
    while((rc = st->alerts.base)) {
        if(last == rc) {
            error("RRDCALC: malformed list of alerts linked to chart - cannot cleanup - giving up.");
            break;
        }
        last = rc;

        if(rc->run_flags & RRDCALC_FLAG_FROM_TEMPLATE) {
            // if the alert comes from a template we can just delete it
            rrdcalc_unlink_and_delete(st->rrdhost, rc, true);
        }
        else {
            // this is a configuration for a specific chart
            // it should stay in the list
            rrdcalc_unlink_from_rrdset(rc, true);
        }

    }
    netdata_rwlock_unlock(&st->alerts.rwlock);
}

void rrdcalc_delete_all(RRDHOST *host) {
    dictionary_flush(host->rrdcalc_root_index);
}

void rrdcalc_free_unused_rrdcalc_loaded_from_config(RRDCALC *rc) {
    if(rc->rrdset)
        rrdcalc_unlink_from_rrdset(rc, false);

    rrdcalc_free_internals(rc);
    freez(rc);
}

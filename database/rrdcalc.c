// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd.h"

static void rrdcalc_del(RRDHOST *host, RRDCALC *rc);
static void rrdcalc_rrdset_index_add(RRDSET *st, RRDCALC *rc);
static void rrdcalc_rrdset_index_del(RRDSET *st, RRDCALC *rc);

// ----------------------------------------------------------------------------
// RRDCALC management

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

static STRING *rrdcalc_replace_variables(const char *line, RRDCALC *rc) {
    if (!line || !*line)
        return NULL;

    size_t pos = 0;
    char *temp = strdupz(line);
    char var[RRDCALC_VAR_MAX];
    char *m, *lbl_value = NULL;

    while ((m = strchr(temp + pos, '$'))) {
        int i = 0;
        char *e = m;
        while (*e) {

            if (*e == ' ' || i == RRDCALC_VAR_MAX - 1)
                break;
            else
                var[i] = *e;

            e++;
            i++;
        }

        var[i] = '\0';
        pos = m - temp + 1;

        if (!strcmp(var, RRDCALC_VAR_FAMILY)) {
            char *buf = find_and_replace(temp, var, (rc->rrdset && rc->rrdset->family) ? rrdset_family(rc->rrdset) : "", m);
            freez(temp);
            temp = buf;
        }
        else if (!strncmp(var, RRDCALC_VAR_LABEL, RRDCALC_VAR_LABEL_LEN)) {
            if(likely(rc->rrdset && rc->rrdset->rrdlabels)) {
                rrdlabels_get_value_to_char_or_null(rc->rrdset->rrdlabels, &lbl_value, var+RRDCALC_VAR_LABEL_LEN);
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

void rrdcalc_update_rrdlabels(RRDSET *st) {
    RRDCALC *rc;
    foreach_rrdcalc_in_rrdset_read(st, rc) {
        if(!rc->original_info) continue;

        STRING *old = rc->info;
        rc->info = rrdcalc_replace_variables(rrdcalc_original_info(rc), rc);
        string_freez(old);
    }
    foreach_rrdcalc_in_rrdset_done(rc);
}

static void rrdcalc_link_to_rrdset(RRDSET *st, RRDCALC *rc) {
    RRDHOST *host = st->rrdhost;

    debug(D_HEALTH, "Health linking alarm '%s.%s' to chart '%s' of host '%s'", rrdcalc_chart_name(rc), rrdcalc_name(rc), rrdset_id(st), rrdhost_hostname(host));

    rc->last_status_change = now_realtime_sec();
    rc->rrdset = st;

    rrdcalc_rrdset_index_add(st, rc);

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

    rc->rrdvar_local = rrdvar_add_and_acquire("local", st->rrdvars, rc->name, RRDVAR_TYPE_CALCULATED, RRDVAR_FLAG_RRDCALC_LOCAL_VAR, &rc->value);
    rc->rrdvar_family = rrdvar_add_and_acquire("family", rrdfamily_rrdvars_dict(st->rrdfamily), rc->name, RRDVAR_TYPE_CALCULATED, RRDVAR_FLAG_RRDCALC_FAMILY_VAR, &rc->value);

    char buf[RRDVAR_MAX_LENGTH + 1];

    snprintfz(buf, RRDVAR_MAX_LENGTH, "%s.%s", rrdset_name(st), rrdcalc_name(rc));
    STRING *rrdset_name_rrdcalc_name = string_strdupz(buf);
    rc->rrdvar_host_chart_name = rrdvar_add_and_acquire(
        "host",
        host->rrdvars,
        rrdset_name_rrdcalc_name,
        RRDVAR_TYPE_CALCULATED,
        RRDVAR_FLAG_RRDCALC_HOST_CHARTNAME_VAR,
        &rc->value);

    snprintfz(buf, RRDVAR_MAX_LENGTH, "%s.%s", rrdset_id(st), rrdcalc_name(rc));
    STRING *rrdset_id_rrdcalc_name = string_strdupz(buf);
    rc->rrdvar_host_chart_id = rrdvar_add_and_acquire(
        "host",
        host->rrdvars,
        rrdset_id_rrdcalc_name,
        RRDVAR_TYPE_CALCULATED,
        RRDVAR_FLAG_RRDCALC_HOST_CHARTID_VAR | ((rc->rrdvar_host_chart_name) ? 0 : RRDVAR_FLAG_RRDCALC_HOST_CHARTNAME_VAR),
        &rc->value);

    string_freez(rrdset_id_rrdcalc_name);
    string_freez(rrdset_name_rrdcalc_name);

    if(!rc->units) rc->units = string_dup(st->units);

    if (rc->original_info) {
        if (rc->info)
            string_freez(rc->info);

        rc->info = rrdcalc_replace_variables(rrdcalc_original_info(rc), rc);
    }

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

static inline int rrdcalc_is_matching_rrdset(RRDCALC *rc, RRDSET *st) {
    if (   (rc->chart != st->id)
        && (rc->chart != st->name))
        return 0;

    if (rc->module_pattern && !simple_pattern_matches(rc->module_pattern, rrdset_module_name(st)))
        return 0;

    if (rc->plugin_pattern && !simple_pattern_matches(rc->plugin_pattern, rrdset_plugin_name(st)))
        return 0;

    if (st->rrdhost->rrdlabels && rc->host_labels_pattern && !rrdlabels_match_simple_pattern_parsed(st->rrdhost->rrdlabels, rc->host_labels_pattern, '='))
        return 0;

    return 1;
}

inline void rrdcalc_link_matching_host_alarms_to_rrdset(RRDSET *st) {
    RRDHOST *host = st->rrdhost;
    // debug(D_HEALTH, "find matching alarms for chart '%s'", st->id);

    RRDCALC *rc;
    foreach_rrdcalc_in_rrdhost_read(host, rc) {
        if(rc->rrdset)
            continue;

        if(unlikely(rrdcalc_is_matching_rrdset(rc, st)))
            rrdcalc_link_to_rrdset(st, rc);
    }
    foreach_rrdcalc_in_rrdhost_done(rc);
}

// this has to be called while the RRDHOST is locked
static inline void rrdcalc_unlink_from_rrdset(RRDCALC *rc) {
    RRDSET *st = rc->rrdset;

    if(!st) {
        debug(D_HEALTH, "Requested to unlink RRDCALC '%s.%s' which is not linked to any RRDSET", rrdcalc_chart_name(rc), rrdcalc_name(rc));
        error("Requested to unlink RRDCALC '%s.%s' which is not linked to any RRDSET", rrdcalc_chart_name(rc), rrdcalc_name(rc));
        return;
    }

    RRDHOST *host = st->rrdhost;

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
        RRDCALC_STATUS_REMOVED,
        rc->source,
        rc->units,
        rc->info,
        0,
        0);

    health_alarm_log_add_entry(host, ae);

    debug(D_HEALTH, "Health unlinking alarm '%s.%s' from chart '%s' of host '%s'", rrdcalc_chart_name(rc), rrdcalc_name(rc), rrdset_id(st), rrdhost_hostname(host));

    // unlink it
    rrdcalc_rrdset_index_del(st, rc);

    rrdvar_release_and_del(st->rrdvars, rc->rrdvar_local);
    rc->rrdvar_local = NULL;

    rrdvar_release_and_del(rrdfamily_rrdvars_dict(st->rrdfamily), rc->rrdvar_family);
    rc->rrdvar_family = NULL;

    rrdvar_release_and_del(host->rrdvars, rc->rrdvar_host_chart_id);
    rc->rrdvar_host_chart_id = NULL;

    rrdvar_release_and_del(host->rrdvars, rc->rrdvar_host_chart_name);
    rc->rrdvar_host_chart_name = NULL;

    rc->rrdset = NULL;

    // RRDCALC will remain in RRDHOST
    // so that if the matching chart is found in the future
    // it will be applied automatically
}

RRDCALC *rrdcalc_find_in_rrdset_unsafe(RRDSET *st, const char *name) {
    RRDCALC *rc = NULL;

    STRING *name_string = string_strdupz(name);

    foreach_rrdcalc_in_rrdset_read(st, rc) {
        if(unlikely(rc->name == name_string))
            break;
    }
    foreach_rrdcalc_in_rrdset_done(rc);

    string_freez(name_string);

    return rc;
}

inline uint32_t rrdcalc_get_unique_id(RRDHOST *host, STRING *chart, STRING *name, uint32_t *next_event_id) {
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

/**
 * Alarm name with dimension
 *
 * Change the name of the current alarm appending a new diagram.
 *
 * @param name the alarm name
 * @param namelen is the length of the previous vector.
 * @param dim the dimension of the chart.
 * @param dimlen  is the length of the previous vector.
 *
 * @return It returns the new name on success and the old otherwise
 */
char *alarm_name_with_dim(const char *name, size_t namelen, const char *dim, size_t dimlen) {
    char *newname,*move;

    newname = mallocz(namelen + dimlen + 2);
    move = newname;
    memcpy(move, name, namelen);
    move += namelen;

    *move++ = '_';
    memcpy(move, dim, dimlen);
    move += dimlen;
    *move = '\0';

    return newname;
}

/**
 * Remove pipe comma
 *
 * Remove the pipes and commas converting to space.
 *
 * @param str the string to change.
 */
void dimension_remove_pipe_comma(char *str) {
    while(*str) {
        if(*str == '|' || *str == ',') *str = ' ';

        str++;
    }
}

void rrdcalc_free(RRDCALC *rc) {
    if(unlikely(!rc)) return;

    if(rc->rrdset) {
        rrdvar_release_and_del(rc->rrdset->rrdvars, rc->rrdvar_local);
        rrdvar_release_and_del(rrdfamily_rrdvars_dict(rc->rrdset->rrdfamily), rc->rrdvar_family);
        rrdvar_release_and_del(rc->rrdset->rrdhost->rrdvars, rc->rrdvar_host_chart_id);
        rrdvar_release_and_del(rc->rrdset->rrdhost->rrdvars, rc->rrdvar_host_chart_name);
        rc->rrdvar_local = rc->rrdvar_family = rc->rrdvar_host_chart_id = rc->rrdvar_host_chart_name = NULL;
    }

    expression_free(rc->calculation);
    expression_free(rc->warning);
    expression_free(rc->critical);

    string_freez(rc->key);
    string_freez(rc->name);
    string_freez(rc->chart);
    string_freez(rc->dimensions);
    string_freez(rc->foreachdim);
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

    simple_pattern_free(rc->spdim);
    simple_pattern_free(rc->host_labels_pattern);
    simple_pattern_free(rc->module_pattern);
    simple_pattern_free(rc->plugin_pattern);

    freez(rc);
}

void rrdcalc_unlink_and_free_all_rrdset_alarms(RRDHOST *host, RRDSET *st) {
    RRDCALC *rc;
    dfe_start_reentrant(st->rrdcalc_root_index, rc) {
        rrdcalc_del(host, rc);
    }
    dfe_done(rc);
}

void rrdcalc_unlink_and_free_all_rrdhost_alarms(RRDHOST *host) {
    RRDCALC *rc;
    dfe_start_reentrant(host->rrdcalc_root_index, rc) {
        rrdcalc_del(host, rc);
    }
    dfe_done(rc);
}

static void rrdcalc_labels_unlink_alarm_loop(RRDHOST *host) {
    RRDCALC *rc;
    foreach_rrdcalc_in_rrdhost_read(host, rc) {
        RRDCALC *rc_next = rc->next;

        if (!rc->host_labels) {
            rc = rc_next;
            continue;
        }

        if(!rrdlabels_match_simple_pattern_parsed(host->rrdlabels, rc->host_labels_pattern, '=')) {
            info("Health configuration for alarm '%s' cannot be applied, because the host %s does not have the label(s) '%s'",
                 rrdcalc_name(rc),
                 rrdhost_hostname(host),
                 rrdcalc_host_labels(rc));

            rrdcalc_del(host, rc);
        }
        rc = rc_next;
    }
    foreach_rrdcalc_in_rrdhost_done(rc);
}

void rrdcalc_labels_unlink_and_free_alarms_from_host(RRDHOST *host) {
    rrdcalc_labels_unlink_alarm_loop(host);
}

void rrdcalc_remove_alarms_not_matching_host_labels() {
    rrd_rdlock();

    RRDHOST *host;
    rrdhost_foreach_read(host) {
        if (unlikely(!host->health_enabled))
            continue;

        if (host->rrdlabels) {
            rrdhost_wrlock(host);
            rrdcalc_labels_unlink_and_free_alarms_from_host(host);
            rrdhost_unlock(host);
        }
    }

    rrd_unlock();
}

// ----------------------------------------------------------------------------
// RRDCALC rrdset index management

static void rrdcalc_rrdset_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdcalc __maybe_unused, void *rrdset __maybe_unused) {
//    RRDSET *st = rrdset;
//    dictionary_acquired_item_dup(st->rrdhost->rrdcalc_root_index, (const DICTIONARY_ITEM *)rrdcalc);
    ;
}

static void rrdcalc_rrdset_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdcalc __maybe_unused, void *rrdset __maybe_unused) {
//    RRDSET *st = rrdset;
//    dictionary_acquired_item_release(st->rrdhost->rrdcalc_root_index, (const DICTIONARY_ITEM *)rrdcalc);
    ;
}

void rrdcalc_rrdset_index_init(RRDSET *st) {
    if(!st->rrdcalc_root_index) {
        st->rrdcalc_root_index = dictionary_create(
             DICTIONARY_FLAG_NAME_LINK_DONT_CLONE
            |DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE
            |DICTIONARY_FLAG_DONT_OVERWRITE_VALUE);

        dictionary_register_insert_callback(st->rrdcalc_root_index, rrdcalc_rrdset_insert_callback, st);
        dictionary_register_delete_callback(st->rrdcalc_root_index, rrdcalc_rrdset_delete_callback, st);
    }
}

void rrdcalc_rrdset_index_destroy(RRDSET *st) {
    // destroy the id index last
    dictionary_destroy(st->rrdcalc_root_index);
    st->rrdcalc_root_index = NULL;
}

static void rrdcalc_rrdset_index_add(RRDSET *st, RRDCALC *rc) {
    dictionary_set_advanced(st->rrdcalc_root_index, string2str(rc->key), (ssize_t)string_strlen(rc->key) + 1, rc, sizeof(RRDCALC *), NULL);
}

static void rrdcalc_rrdset_index_del(RRDSET *st, RRDCALC *rc) {
    dictionary_del_advanced(st->rrdcalc_root_index, string2str(rc->key), (ssize_t)string_strlen(rc->key) + 1);
}

// ----------------------------------------------------------------------------
// RRDCALC rrdhost index management

static void rrdcalc_constructor_from_rrdcalctemplate(RRDCALC *rc, RRDCALCTEMPLATE *rt, RRDSET *st, const char *overwrite_alert_name, const char *overwrite_dimensions) {
    rc->next_event_id = 1;
    rc->name = overwrite_alert_name?string_strdupz(overwrite_alert_name):string_dup(rt->name);
    rc->chart = string_dup(st->id);
    uuid_copy(rc->config_hash_id, rt->config_hash_id);

    rc->dimensions = overwrite_dimensions?string_strdupz(overwrite_dimensions):string_dup(rt->dimensions);
    rc->foreachdim = NULL;
    rc->spdim = NULL;

    rc->foreachcounter = rt->foreachcounter;

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

struct rrdcalc_constructor {
    RRDHOST *rrdhost;
    RRDCALC *from_config;
    RRDCALCTEMPLATE *from_rrdcalctemplate;
    RRDSET *rrdset;
    const char *overwrite_alert_name;
    const char *overwrite_dimensions;

    enum {
        RRDCALC_REACT_NONE,
        RRDCALC_REACT_NEW,
    } react_action;
};

static void rrdcalc_rrdhost_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdcalc, void *constructor_data) {
    RRDCALC *rc = rrdcalc;
    struct rrdcalc_constructor *ctr = constructor_data;
    RRDHOST *host = ctr->rrdhost;

    rc->key = string_strdupz(dictionary_acquired_item_value(item));

    if(ctr->from_rrdcalctemplate) {
        rrdcalc_constructor_from_rrdcalctemplate(rc, ctr->from_rrdcalctemplate, ctr->rrdset, ctr->overwrite_alert_name, ctr->overwrite_dimensions);
    }
    else if(ctr->from_config) {
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
          (rc->foreachdim)?rrdcalc_foreachdim(rc):"NONE",
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

static inline int rrdcalc_check_and_link_rrdset_callback(RRDSET *st, void *rrdcalc) {
    RRDCALC *rc = rrdcalc;

    if(unlikely(rrdcalc_is_matching_rrdset(rc, st))) {
        rrdcalc_link_to_rrdset(st, rc);
        return -1;
    }

    return 0;
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

static void rrdcalc_rrdhost_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdcalc, void *rrdhost __maybe_unused) {
    RRDCALC *rc = rrdcalc;
    //RRDHOST *host = rrdhost;

    // any desctuction actions that require other locks
    // have to be placed in rrdcalc_del(), because the object is actually locked for deletion

    rrdcalc_free(rc);
}

void rrdcalc_rrdhost_index_init(RRDHOST *host) {
    if(!host->rrdcalc_root_index) {
        host->rrdcalc_root_index = dictionary_create(DICTIONARY_FLAG_DONT_OVERWRITE_VALUE);

        dictionary_register_insert_callback(host->rrdcalc_root_index, rrdcalc_rrdhost_insert_callback, NULL);
        dictionary_register_react_callback(host->rrdcalc_root_index, rrdcalc_rrdhost_react_callback, NULL);
        dictionary_register_delete_callback(host->rrdcalc_root_index, rrdcalc_rrdhost_delete_callback, host);
    }
}

void rrdcalc_rrdhost_index_destroy(RRDHOST *host) {
    // destroy the id index last
    dictionary_destroy(host->rrdcalc_root_index);
    host->rrdcalc_root_index = NULL;
}

static size_t rrdcalc_key(char *dst, size_t dst_len, const char *chart, const char *alert) {
    return snprintfz(dst, dst_len, "%s/%s", chart, alert);
}

void rrdcalc_add_from_rrdcalctemplate(RRDHOST *host, RRDCALCTEMPLATE *rt, RRDSET *st, const char *overwrite_alert_name, const char *overwrite_dimensions) {
    char key[1024 + 1];
    size_t key_len = rrdcalc_key(key, 1024, rrdset_id(st), overwrite_alert_name?overwrite_alert_name:string2str(rt->name));

    struct rrdcalc_constructor tmp = {
        .rrdhost = host,
        .from_config = NULL,
        .from_rrdcalctemplate = rt,
        .rrdset = st,
        .overwrite_alert_name = overwrite_alert_name,
        .overwrite_dimensions = overwrite_dimensions,
        .react_action = RRDCALC_REACT_NONE,
    };

    dictionary_set_advanced(host->rrdcalc_root_index, key, (ssize_t)(key_len + 1), NULL, sizeof(RRDCALC), &tmp);
    if(tmp.react_action != RRDCALC_REACT_NEW)
        error("RRDCALC: from template '%s' on chart '%s' with key '%s', failed to be added to host '%s'. It already exists.",
              string2str(rt->name), rrdset_id(st), key, rrdhost_hostname(host));
}

int rrdcalc_add_from_config_rrdcalc(RRDHOST *host, RRDCALC *rc) {
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

    char key[1024 + 1];
    size_t key_len = rrdcalc_key(key, 1024, string2str(rc->chart), string2str(rc->name));

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
        // we copied rc into the dictionary, so we have to free the base here
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
        rrdcalc_free(rc);
    }

    return ret;
}

static void rrdcalc_del(RRDHOST *host, RRDCALC *rc) {
    if(rc->rrdset)
        rrdcalc_unlink_from_rrdset(rc);

    dictionary_del_advanced(host->rrdcalc_root_index, string2str(rc->key), string_strlen(rc->key) + 1);
}

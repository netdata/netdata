// SPDX-License-Identifier: GPL-3.0-or-later

#define NETDATA_HEALTH_INTERNALS
#include "rrd.h"

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
            if(likely(rc->rrdset && rc->rrdset->state && rc->rrdset->state->chart_labels)) {
                rrdlabels_get_value_to_char_or_null(rc->rrdset->state->chart_labels, &lbl_value, var+RRDCALC_VAR_LABEL_LEN);
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
    for( rc = st->alarms; rc ; rc = rc->rrdset_next ) {
        if (rc->original_info) {
            if (rc->info)
                string_freez(rc->info);

            rc->info = rrdcalc_replace_variables(rrdcalc_original_info(rc), rc);
        }
    }
}

static void rrdsetcalc_link(RRDSET *st, RRDCALC *rc) {
    RRDHOST *host = st->rrdhost;

    debug(D_HEALTH, "Health linking alarm '%s.%s' to chart '%s' of host '%s'", rrdcalc_chart_name(rc), rrdcalc_name(rc), rrdset_id(st), rrdhost_hostname(host));

    rc->last_status_change = now_realtime_sec();
    rc->rrdset = st;

    rc->rrdset_next = st->alarms;
    rc->rrdset_prev = NULL;

    if(rc->rrdset_next)
        rc->rrdset_next->rrdset_prev = rc;

    st->alarms = rc;

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

    rc->local  = rrdvar_create_and_index("local",  &st->rrdvar_root_index, rc->name, RRDVAR_TYPE_CALCULATED, RRDVAR_OPTION_RRDCALC_LOCAL_VAR, &rc->value);
    rc->family = rrdvar_create_and_index("family", &st->rrdfamily->rrdvar_root_index, rc->name, RRDVAR_TYPE_CALCULATED, RRDVAR_OPTION_RRDCALC_FAMILY_VAR, &rc->value);

    char fullname[RRDVAR_MAX_LENGTH + 1];
    snprintfz(fullname, RRDVAR_MAX_LENGTH, "%s.%s", rrdset_id(st), rrdcalc_name(rc));
    STRING *fullname_string = string_strdupz(fullname);
    rc->hostid   = rrdvar_create_and_index("host", &host->rrdvar_root_index, fullname_string, RRDVAR_TYPE_CALCULATED, RRDVAR_OPTION_RRDCALC_HOST_CHARTID_VAR, &rc->value);

    snprintfz(fullname, RRDVAR_MAX_LENGTH, "%s.%s", rrdset_name(st), rrdcalc_name(rc));
    rc->hostname = rrdvar_create_and_index("host", &host->rrdvar_root_index, fullname_string, RRDVAR_TYPE_CALCULATED, RRDVAR_OPTION_RRDCALC_HOST_CHARTNAME_VAR, &rc->value);

    string_freez(fullname_string);

    if(rc->hostid && !rc->hostname)
        rc->hostid->options |= RRDVAR_OPTION_RRDCALC_HOST_CHARTNAME_VAR;

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
        0);

    health_alarm_log(host, ae);
}

static inline int rrdcalc_is_matching_rrdset(RRDCALC *rc, RRDSET *st) {
    if (   (rc->chart != st->id)
        && (rc->chart != st->name))
        return 0;

    if (rc->module_pattern && !simple_pattern_matches(rc->module_pattern, rrdset_module_name(st)))
        return 0;

    if (rc->plugin_pattern && !simple_pattern_matches(rc->plugin_pattern, rrdset_plugin_name(st)))
        return 0;

    if (st->rrdhost->host_labels && rc->host_labels_pattern && !rrdlabels_match_simple_pattern_parsed(st->rrdhost->host_labels, rc->host_labels_pattern, '='))
        return 0;

    return 1;
}

// this has to be called while the RRDHOST is locked
inline void rrdsetcalc_link_matching(RRDSET *st) {
    RRDHOST *host = st->rrdhost;
    // debug(D_HEALTH, "find matching alarms for chart '%s'", st->id);

    RRDCALC *rc;
    for(rc = host->alarms; rc ; rc = rc->next) {
        if(unlikely(rc->rrdset))
            continue;

        if(unlikely(rrdcalc_is_matching_rrdset(rc, st)))
            rrdsetcalc_link(st, rc);
    }
}

// this has to be called while the RRDHOST is locked
inline void rrdsetcalc_unlink(RRDCALC *rc) {
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

    health_alarm_log(host, ae);

    debug(D_HEALTH, "Health unlinking alarm '%s.%s' from chart '%s' of host '%s'", rrdcalc_chart_name(rc), rrdcalc_name(rc), rrdset_id(st), rrdhost_hostname(host));

    // unlink it
    if(rc->rrdset_prev)
        rc->rrdset_prev->rrdset_next = rc->rrdset_next;

    if(rc->rrdset_next)
        rc->rrdset_next->rrdset_prev = rc->rrdset_prev;

    if(st->alarms == rc)
        st->alarms = rc->rrdset_next;

    rc->rrdset_prev = rc->rrdset_next = NULL;

    rrdvar_free(host, &st->rrdvar_root_index, rc->local);
    rc->local = NULL;

    rrdvar_free(host, &st->rrdfamily->rrdvar_root_index, rc->family);
    rc->family = NULL;

    rrdvar_free(host, &host->rrdvar_root_index, rc->hostid);
    rc->hostid = NULL;

    rrdvar_free(host, &host->rrdvar_root_index, rc->hostname);
    rc->hostname = NULL;

    rc->rrdset = NULL;

    // RRDCALC will remain in RRDHOST
    // so that if the matching chart is found in the future
    // it will be applied automatically
}

RRDCALC *rrdcalc_find(RRDSET *st, const char *name) {
    RRDCALC *rc = NULL;

    STRING *name_string = string_strdupz(name);

    for( rc = st->alarms; rc ; rc = rc->rrdset_next ) {
        if(unlikely(rc->name == name_string))
            break;
    }

    string_freez(name_string);

    return rc;
}

inline int rrdcalc_exists(RRDHOST *host, const char *chart, const char *name) {
    RRDCALC *rc;

    if(unlikely(!chart)) {
        error("attempt to find RRDCALC '%s' without giving a chart name", name);
        return 1;
    }

    STRING *name_string = string_strdupz(name);
    STRING *chart_string = string_strdupz(chart);
    int ret = 0;

    // make sure it does not already exist
    for(rc = host->alarms; rc ; rc = rc->next) {
        if (unlikely(rc->chart == chart_string && rc->name == name_string)) {
            debug(D_HEALTH, "Health alarm '%s/%s' already exists in host '%s'.", chart, name, rrdhost_hostname(host));
            info("Health alarm '%s/%s' already exists in host '%s'.", chart, name, rrdhost_hostname(host));
            ret = 1;
            break;
        }
    }

    string_freez(name_string);
    string_freez(chart_string);

    return ret;
}

inline uint32_t rrdcalc_get_unique_id(RRDHOST *host, const char *chart, const char *name, uint32_t *next_event_id) {
    if(chart && name) {

        STRING *chart_string = string_strdupz(chart);
        STRING *name_string = string_strdupz(name);

        // re-use old IDs, by looking them up in the alarm log
        ALARM_ENTRY *ae = NULL;
        for(ae = host->health_log.alarms; ae ;ae = ae->next) {
            if(unlikely(name_string == ae->name && chart_string == ae->chart)) {
                if(next_event_id) *next_event_id = ae->alarm_event_id + 1;
                break;
            }
        }

        string_freez(chart_string);
        string_freez(name_string);

        if(ae)
            return ae->alarm_id;
    }

    if (unlikely(!host->health_log.next_alarm_id))
        host->health_log.next_alarm_id = (uint32_t)now_realtime_sec();

    return host->health_log.next_alarm_id++;
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

inline void rrdcalc_add_to_host(RRDHOST *host, RRDCALC *rc) {
    rrdhost_check_rdlock(host);

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

    if(!rc->foreachdim) {
        // link it to the host alarms list
        if(likely(host->alarms)) {
            // append it
            RRDCALC *t;
            for(t = host->alarms; t && t->next ; t = t->next) ;
            t->next = rc;
        }
        else {
            host->alarms = rc;
        }

        // link it to its chart
        RRDSET *st;
        rrdset_foreach_read(st, host) {
            if(rrdcalc_is_matching_rrdset(rc, st)) {
                rrdsetcalc_link(st, rc);
                break;
            }
        }
    } else {
        //link it case there is a foreach
        if(likely(host->alarms_with_foreach)) {
            // append it
            RRDCALC *t;
            for(t = host->alarms_with_foreach; t && t->next ; t = t->next) ;
            t->next = rc;
        }
        else {
            host->alarms_with_foreach = rc;
        }

        //I am not linking this alarm direct to the host here, this will be done when the children is created
    }
}

inline RRDCALC *rrdcalc_create_from_template(RRDHOST *host, RRDCALCTEMPLATE *rt, const char *chart) {
    debug(D_HEALTH, "Health creating dynamic alarm (from template) '%s.%s'", chart, rrdcalctemplate_name(rt));

    if(rrdcalc_exists(host, chart, rrdcalctemplate_name(rt)))
        return NULL;

    RRDCALC *rc = callocz(1, sizeof(RRDCALC));
    rc->next_event_id = 1;
    rc->name = string_dup(rt->name);
    rc->chart = string_strdupz(chart);
    uuid_copy(rc->config_hash_id, rt->config_hash_id);

    rc->id = rrdcalc_get_unique_id(host, rrdcalc_chart_name(rc), rrdcalc_name(rc), &rc->next_event_id);

    rc->dimensions = string_dup(rt->dimensions);
    rc->foreachdim = string_dup(rt->foreachdim);
    if(rt->foreachdim)
        rc->spdim = health_pattern_from_foreach(rrdcalc_foreachdim(rc));

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
            error("Health alarm '%s.%s': failed to parse calculation expression '%s'", chart, rrdcalctemplate_name(rt), rt->calculation->source);
    }
    if(rt->warning) {
        rc->warning = expression_parse(rt->warning->source, NULL, NULL);
        if(!rc->warning)
            error("Health alarm '%s.%s': failed to re-parse warning expression '%s'", chart, rrdcalctemplate_name(rt), rt->warning->source);
    }
    if(rt->critical) {
        rc->critical = expression_parse(rt->critical->source, NULL, NULL);
        if(!rc->critical)
            error("Health alarm '%s.%s': failed to re-parse critical expression '%s'", chart, rrdcalctemplate_name(rt), rt->critical->source);
    }

    debug(D_HEALTH, "Health runtime added alarm '%s.%s': exec '%s', recipient '%s', green " NETDATA_DOUBLE_FORMAT_AUTO
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

    rrdcalc_add_to_host(host, rc);
    if(!rt->foreachdim) {
        RRDCALC *rdcmp  = (RRDCALC *) avl_insert_lock(&(host)->alarms_idx_health_log,(avl_t *)rc);
        if (rdcmp != rc) {
            error("Cannot insert the alarm index ID %s",rrdcalc_name(rc));
        }
    }

    return rc;
}

/**
 *  Create from RRDCALC
 *
 *  Create a new alarm using another alarm as template.
 *
 * @param rc is the alarm that will be used as source
 * @param host is the host structure.
 * @param name is the newest chart name.
 * @param dimension is the current dimension
 * @param foreachdim the whole list of dimension
 *
 * @return it returns the new alarm changed.
 */
inline RRDCALC *rrdcalc_create_from_rrdcalc(RRDCALC *rc, RRDHOST *host, const char *name, const char *dimension) {
    RRDCALC *newrc = callocz(1, sizeof(RRDCALC));

    newrc->next_event_id = 1;
    newrc->id = rrdcalc_get_unique_id(host, rrdcalc_chart_name(rc), name, &rc->next_event_id);
    newrc->name = string_strdupz(name);
    newrc->chart = string_dup(rc->chart);
    uuid_copy(newrc->config_hash_id, *((uuid_t *) &rc->config_hash_id));

    newrc->dimensions = string_strdupz(dimension);
    newrc->foreachdim = NULL;
    rc->foreachcounter++;
    newrc->foreachcounter = rc->foreachcounter;

    newrc->green = rc->green;
    newrc->red = rc->red;
    newrc->value = NAN;
    newrc->old_value = NAN;

    newrc->delay_up_duration = rc->delay_up_duration;
    newrc->delay_down_duration = rc->delay_down_duration;
    newrc->delay_max_duration = rc->delay_max_duration;
    newrc->delay_multiplier = rc->delay_multiplier;

    newrc->last_repeat = 0;
    newrc->times_repeat = 0;
    newrc->warn_repeat_every = rc->warn_repeat_every;
    newrc->crit_repeat_every = rc->crit_repeat_every;

    newrc->group = rc->group;
    newrc->after = rc->after;
    newrc->before = rc->before;
    newrc->update_every = rc->update_every;
    newrc->options = rc->options;

    newrc->exec = string_dup(rc->exec);
    newrc->recipient = string_dup(rc->recipient);
    newrc->source = string_dup(rc->source);
    newrc->units = string_dup(rc->units);
    newrc->info = string_dup(rc->info);
    newrc->original_info = string_dup(rc->original_info);

    newrc->classification = string_dup(rc->classification);
    newrc->component = string_dup(rc->component);
    newrc->type = string_dup(rc->type);

    if(rc->calculation) {
        newrc->calculation = expression_parse(rc->calculation->source, NULL, NULL);
        if(!newrc->calculation)
            error("Health alarm '%s.%s': failed to parse calculation expression '%s'", rrdcalc_chart_name(rc), rrdcalc_name(rc), rc->calculation->source);
    }

    if(rc->warning) {
        newrc->warning = expression_parse(rc->warning->source, NULL, NULL);
        if(!newrc->warning)
            error("Health alarm '%s.%s': failed to re-parse warning expression '%s'", rrdcalc_chart_name(rc), rrdcalc_name(rc), rc->warning->source);
    }

    if(rc->critical) {
        newrc->critical = expression_parse(rc->critical->source, NULL, NULL);
        if(!newrc->critical)
            error("Health alarm '%s.%s': failed to re-parse critical expression '%s'", rrdcalc_chart_name(rc), rrdcalc_name(rc), rc->critical->source);
    }

    return newrc;
}

void rrdcalc_free(RRDCALC *rc) {
    if(unlikely(!rc)) return;

    expression_free(rc->calculation);
    expression_free(rc->warning);
    expression_free(rc->critical);

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

    freez(rc->family);
    freez(rc);
}

void rrdcalc_unlink_and_free(RRDHOST *host, RRDCALC *rc) {
    if(unlikely(!rc)) return;

    debug(D_HEALTH, "Health removing alarm '%s.%s' of host '%s'", rrdcalc_chart_name(rc), rrdcalc_name(rc), rrdhost_hostname(host));

    // unlink it from RRDSET
    if(rc->rrdset) rrdsetcalc_unlink(rc);

    // unlink it from RRDHOST
    if(unlikely(rc == host->alarms))
        host->alarms = rc->next;
    else {
        RRDCALC *t;
        for(t = host->alarms; t && t->next != rc; t = t->next) ;
        if(t) {
            t->next = rc->next;
            rc->next = NULL;
        }
        else
            error("Cannot unlink alarm '%s.%s' from host '%s': not found", rrdcalc_chart_name(rc), rrdcalc_name(rc), rrdhost_hostname(host));
    }

    RRDCALC *rdcmp = (RRDCALC *) avl_search_lock(&(host)->alarms_idx_health_log, (avl_t *)rc);
    if (rdcmp) {
        rdcmp = (RRDCALC *) avl_remove_lock(&(host)->alarms_idx_health_log, (avl_t *)rc);
        if (!rdcmp) {
            error("Cannot remove the health alarm index from health_log");
        }
    }

    rdcmp = (RRDCALC *) avl_search_lock(&(host)->alarms_idx_name, (avl_t *)rc);
    if (rdcmp) {
        rdcmp = (RRDCALC *) avl_remove_lock(&(host)->alarms_idx_name, (avl_t *)rc);
        if (!rdcmp) {
            error("Cannot remove the health alarm index from idx_name");
        }
    }

    rrdcalc_free(rc);
}

void rrdcalc_foreach_unlink_and_free(RRDHOST *host, RRDCALC *rc) {

    if(unlikely(rc == host->alarms_with_foreach))
        host->alarms_with_foreach = rc->next;
    else {
        RRDCALC *t;
        for(t = host->alarms_with_foreach; t && t->next != rc; t = t->next) ;
        if(t) {
            t->next = rc->next;
            rc->next = NULL;
        }
        else
            error("Cannot unlink alarm '%s.%s' from host '%s': not found", rrdcalc_chart_name(rc), rrdcalc_name(rc), rrdhost_hostname(host));
    }

    rrdcalc_free(rc);
}

static void rrdcalc_labels_unlink_alarm_loop(RRDHOST *host, RRDCALC *alarms) {
    for(RRDCALC *rc = alarms ; rc ; ) {
        RRDCALC *rc_next = rc->next;

        if (!rc->host_labels) {
            rc = rc_next;
            continue;
        }

        if(!rrdlabels_match_simple_pattern_parsed(host->host_labels, rc->host_labels_pattern, '=')) {
            info("Health configuration for alarm '%s' cannot be applied, because the host %s does not have the label(s) '%s'",
                 rrdcalc_name(rc),
                 rrdhost_hostname(host),
                 rrdcalc_host_labels(rc));

            if(host->alarms == alarms)
                rrdcalc_unlink_and_free(host, rc);
            else
                rrdcalc_foreach_unlink_and_free(host, rc);
        }
        rc = rc_next;
    }
}

void rrdcalc_labels_unlink_alarm_from_host(RRDHOST *host) {
    rrdcalc_labels_unlink_alarm_loop(host, host->alarms);
    rrdcalc_labels_unlink_alarm_loop(host, host->alarms_with_foreach);
}

void rrdcalc_labels_unlink() {
    rrd_rdlock();

    RRDHOST *host;
    rrdhost_foreach_read(host) {
        if (unlikely(!host->health_enabled))
            continue;

        if (host->host_labels) {
            rrdhost_wrlock(host);

            rrdcalc_labels_unlink_alarm_from_host(host);

            rrdhost_unlock(host);
        }
    }

    rrd_unlock();
}

// ----------------------------------------------------------------------------
// Alarm


/**
 * Alarm is repeating
 *
 * Is this alarm repeating ?
 *
 * @param host The structure that has the binary tree
 * @param alarm_id the id of the alarm to search
 *
 * @return It returns 1 case it is repeating and 0 otherwise
 */
int alarm_isrepeating(RRDHOST *host, uint32_t alarm_id) {
    RRDCALC findme;
    findme.id = alarm_id;
    RRDCALC *rc = (RRDCALC *)avl_search_lock(&host->alarms_idx_health_log, (avl_t *)&findme);
    if (!rc) {
        return 0;
    }
    return rrdcalc_isrepeating(rc);
}

/**
 * Entry is repeating
 *
 * Check whether the id of alarm entry is yet present in the host structure
 *
 * @param host The structure that has the binary tree
 * @param ae the alarm entry
 *
 * @return It returns 1 case it is repeating and 0 otherwise
 */
int alarm_entry_isrepeating(RRDHOST *host, ALARM_ENTRY *ae) {
    return alarm_isrepeating(host, ae->alarm_id);
}

/**
 * Max last repeat
 *
 * Check the maximum last_repeat for the alarms associated a host
 *
 * @param host The structure that has the binary tree
 *
 * @return It returns 1 case it is repeating and 0 otherwise
 */
RRDCALC *alarm_max_last_repeat(RRDHOST *host, char *alarm_name) {
    RRDCALC tmp = {
        .name = string_strdupz(alarm_name)
    };

    RRDCALC *rc = (RRDCALC *)avl_search_lock(&host->alarms_idx_name, (avl_t *)&tmp);

    string_freez(tmp.name);
    return rc;
}

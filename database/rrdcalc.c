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

static void rrdsetcalc_link(RRDSET *st, RRDCALC *rc) {
    RRDHOST *host = st->rrdhost;

    debug(D_HEALTH, "Health linking alarm '%s.%s' to chart '%s' of host '%s'", rc->chart?rc->chart:"NOCHART", rc->name, st->id, host->hostname);

    rc->last_status_change = now_realtime_sec();
    rc->rrdset = st;

    rc->rrdset_next = st->alarms;
    rc->rrdset_prev = NULL;

    if(rc->rrdset_next)
        rc->rrdset_next->rrdset_prev = rc;

    st->alarms = rc;

    if(rc->update_every < rc->rrdset->update_every) {
        error("Health alarm '%s.%s' has update every %d, less than chart update every %d. Setting alarm update frequency to %d.", rc->rrdset->id, rc->name, rc->update_every, rc->rrdset->update_every, rc->rrdset->update_every);
        rc->update_every = rc->rrdset->update_every;
    }

    if(!isnan(rc->green) && isnan(st->green)) {
        debug(D_HEALTH, "Health alarm '%s.%s' green threshold set from " CALCULATED_NUMBER_FORMAT_AUTO " to " CALCULATED_NUMBER_FORMAT_AUTO ".", rc->rrdset->id, rc->name, rc->rrdset->green, rc->green);
        st->green = rc->green;
    }

    if(!isnan(rc->red) && isnan(st->red)) {
        debug(D_HEALTH, "Health alarm '%s.%s' red threshold set from " CALCULATED_NUMBER_FORMAT_AUTO " to " CALCULATED_NUMBER_FORMAT_AUTO ".", rc->rrdset->id, rc->name, rc->rrdset->red, rc->red);
        st->red = rc->red;
    }

    rc->local  = rrdvar_create_and_index("local",  &st->rrdvar_root_index, rc->name, RRDVAR_TYPE_CALCULATED, RRDVAR_OPTION_RRDCALC_LOCAL_VAR, &rc->value);
    rc->family = rrdvar_create_and_index("family", &st->rrdfamily->rrdvar_root_index, rc->name, RRDVAR_TYPE_CALCULATED, RRDVAR_OPTION_RRDCALC_FAMILY_VAR, &rc->value);

    char fullname[RRDVAR_MAX_LENGTH + 1];
    snprintfz(fullname, RRDVAR_MAX_LENGTH, "%s.%s", st->id, rc->name);
    rc->hostid   = rrdvar_create_and_index("host", &host->rrdvar_root_index, fullname, RRDVAR_TYPE_CALCULATED, RRDVAR_OPTION_RRDCALC_HOST_CHARTID_VAR, &rc->value);

    snprintfz(fullname, RRDVAR_MAX_LENGTH, "%s.%s", st->name, rc->name);
    rc->hostname = rrdvar_create_and_index("host", &host->rrdvar_root_index, fullname, RRDVAR_TYPE_CALCULATED, RRDVAR_OPTION_RRDCALC_HOST_CHARTNAME_VAR, &rc->value);

    if(rc->hostid && !rc->hostname)
        rc->hostid->options |= RRDVAR_OPTION_RRDCALC_HOST_CHARTNAME_VAR;

    if(!rc->units) rc->units = strdupz(st->units);

    if(!rrdcalc_isrepeating(rc)) {
        time_t now = now_realtime_sec();
        ALARM_ENTRY *ae = health_create_alarm_entry(
                host,
                rc->id,
                rc->next_event_id++,
                now,
                rc->name,
                rc->rrdset->id,
                rc->rrdset->family,
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
                0
        );
        health_alarm_log(host, ae);
    }
}

static inline int rrdcalc_is_matching_this_rrdset(RRDCALC *rc, RRDSET *st) {
    if(     (rc->hash_chart == st->hash      && !strcmp(rc->chart, st->id)) ||
            (rc->hash_chart == st->hash_name && !strcmp(rc->chart, st->name)))
        return 1;

    return 0;
}

// this has to be called while the RRDHOST is locked
inline void rrdsetcalc_link_matching(RRDSET *st) {
    RRDHOST *host = st->rrdhost;
    // debug(D_HEALTH, "find matching alarms for chart '%s'", st->id);

    RRDCALC *rc;
    for(rc = host->alarms; rc ; rc = rc->next) {
        if(unlikely(rc->rrdset))
            continue;

        if(unlikely(rrdcalc_is_matching_this_rrdset(rc, st)))
            rrdsetcalc_link(st, rc);
    }
}

// this has to be called while the RRDHOST is locked
inline void rrdsetcalc_unlink(RRDCALC *rc) {
    RRDSET *st = rc->rrdset;

    if(!st) {
        debug(D_HEALTH, "Requested to unlink RRDCALC '%s.%s' which is not linked to any RRDSET", rc->chart?rc->chart:"NOCHART", rc->name);
        error("Requested to unlink RRDCALC '%s.%s' which is not linked to any RRDSET", rc->chart?rc->chart:"NOCHART", rc->name);
        return;
    }

    RRDHOST *host = st->rrdhost;

    if(!rrdcalc_isrepeating(rc)) {
        time_t now = now_realtime_sec();
        ALARM_ENTRY *ae = health_create_alarm_entry(
                host,
                rc->id,
                rc->next_event_id++,
                now,
                rc->name,
                rc->rrdset->id,
                rc->rrdset->family,
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
                0
        );
        health_alarm_log(host, ae);
    }

    debug(D_HEALTH, "Health unlinking alarm '%s.%s' from chart '%s' of host '%s'", rc->chart?rc->chart:"NOCHART", rc->name, st->id, host->hostname);

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
    RRDCALC *rc;
    uint32_t hash = simple_hash(name);

    for( rc = st->alarms; rc ; rc = rc->rrdset_next ) {
        if(unlikely(rc->hash == hash && !strcmp(rc->name, name)))
            return rc;
    }

    return NULL;
}

inline int rrdcalc_exists(RRDHOST *host, const char *chart, const char *name, uint32_t hash_chart, uint32_t hash_name) {
    RRDCALC *rc;

    if(unlikely(!chart)) {
        error("attempt to find RRDCALC '%s' without giving a chart name", name);
        return 1;
    }

    if(unlikely(!hash_chart)) hash_chart = simple_hash(chart);
    if(unlikely(!hash_name))  hash_name  = simple_hash(name);

    // make sure it does not already exist
    for(rc = host->alarms; rc ; rc = rc->next) {
        if (unlikely(rc->chart && rc->hash == hash_name && rc->hash_chart == hash_chart && !strcmp(name, rc->name) && !strcmp(chart, rc->chart))) {
            debug(D_HEALTH, "Health alarm '%s.%s' already exists in host '%s'.", chart, name, host->hostname);
            info("Health alarm '%s.%s' already exists in host '%s'.", chart, name, host->hostname);
            return 1;
        }
    }

    return 0;
}

inline uint32_t rrdcalc_get_unique_id(RRDHOST *host, const char *chart, const char *name, uint32_t *next_event_id) {
    if(chart && name) {
        uint32_t hash_chart = simple_hash(chart);
        uint32_t hash_name = simple_hash(name);

        // re-use old IDs, by looking them up in the alarm log
        ALARM_ENTRY *ae;
        for(ae = host->health_log.alarms; ae ;ae = ae->next) {
            if(unlikely(ae->hash_name == hash_name && ae->hash_chart == hash_chart && !strcmp(name, ae->name) && !strcmp(chart, ae->chart))) {
                if(next_event_id) *next_event_id = ae->alarm_event_id + 1;
                return ae->alarm_id;
            }
        }
    }

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
char *alarm_name_with_dim(char *name, size_t namelen, const char *dim, size_t dimlen) {
    char *newname,*move;

    newname = malloc(namelen + dimlen + 2);
    if(newname) {
        move = newname;
        memcpy(move, name, namelen);
        move += namelen;

        *move++ = '_';
        memcpy(move, dim, dimlen);
        move += dimlen;
        *move = '\0';
    } else {
        newname = name;
    }

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
        rc->calculation->this = &rc->value;
        rc->calculation->after = &rc->db_after;
        rc->calculation->before = &rc->db_before;
        rc->calculation->rrdcalc = rc;
    }

    if(rc->warning) {
        rc->warning->status = &rc->status;
        rc->warning->this = &rc->value;
        rc->warning->after = &rc->db_after;
        rc->warning->before = &rc->db_before;
        rc->warning->rrdcalc = rc;
    }

    if(rc->critical) {
        rc->critical->status = &rc->status;
        rc->critical->this = &rc->value;
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
            if(rrdcalc_is_matching_this_rrdset(rc, st)) {
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
    debug(D_HEALTH, "Health creating dynamic alarm (from template) '%s.%s'", chart, rt->name);

    if(rrdcalc_exists(host, chart, rt->name, 0, 0))
        return NULL;

    RRDCALC *rc = callocz(1, sizeof(RRDCALC));
    rc->next_event_id = 1;
    rc->name = strdupz(rt->name);
    rc->hash = simple_hash(rc->name);
    rc->chart = strdupz(chart);
    rc->hash_chart = simple_hash(rc->chart);

    rc->id = rrdcalc_get_unique_id(host, rc->chart, rc->name, &rc->next_event_id);

    if(rt->dimensions) rc->dimensions = strdupz(rt->dimensions);
    if(rt->foreachdim) {
        rc->foreachdim = strdupz(rt->foreachdim);
        rc->spdim = health_pattern_from_foreach(rc->foreachdim);
    }
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
    rc->warn_repeat_every = rt->warn_repeat_every;
    rc->crit_repeat_every = rt->crit_repeat_every;

    rc->group = rt->group;
    rc->after = rt->after;
    rc->before = rt->before;
    rc->update_every = rt->update_every;
    rc->options = rt->options;

    if(rt->exec) rc->exec = strdupz(rt->exec);
    if(rt->recipient) rc->recipient = strdupz(rt->recipient);
    if(rt->source) rc->source = strdupz(rt->source);
    if(rt->units) rc->units = strdupz(rt->units);
    if(rt->info) rc->info = strdupz(rt->info);

    if(rt->calculation) {
        rc->calculation = expression_parse(rt->calculation->source, NULL, NULL);
        if(!rc->calculation)
            error("Health alarm '%s.%s': failed to parse calculation expression '%s'", chart, rt->name, rt->calculation->source);
    }
    if(rt->warning) {
        rc->warning = expression_parse(rt->warning->source, NULL, NULL);
        if(!rc->warning)
            error("Health alarm '%s.%s': failed to re-parse warning expression '%s'", chart, rt->name, rt->warning->source);
    }
    if(rt->critical) {
        rc->critical = expression_parse(rt->critical->source, NULL, NULL);
        if(!rc->critical)
            error("Health alarm '%s.%s': failed to re-parse critical expression '%s'", chart, rt->name, rt->critical->source);
    }

    debug(D_HEALTH, "Health runtime added alarm '%s.%s': exec '%s', recipient '%s', green " CALCULATED_NUMBER_FORMAT_AUTO ", red " CALCULATED_NUMBER_FORMAT_AUTO ", lookup: group %d, after %d, before %d, options %u, dimensions '%s', for each dimension '%s', update every %d, calculation '%s', warning '%s', critical '%s', source '%s', delay up %d, delay down %d, delay max %d, delay_multiplier %f, warn_repeat_every %u, crit_repeat_every %u",
            (rc->chart)?rc->chart:"NOCHART",
            rc->name,
            (rc->exec)?rc->exec:"DEFAULT",
            (rc->recipient)?rc->recipient:"DEFAULT",
            rc->green,
            rc->red,
            (int)rc->group,
            rc->after,
            rc->before,
            rc->options,
            (rc->dimensions)?rc->dimensions:"NONE",
            (rc->foreachdim)?rc->foreachdim:"NONE",
            rc->update_every,
            (rc->calculation)?rc->calculation->parsed_as:"NONE",
            (rc->warning)?rc->warning->parsed_as:"NONE",
            (rc->critical)?rc->critical->parsed_as:"NONE",
            rc->source,
            rc->delay_up_duration,
            rc->delay_down_duration,
            rc->delay_max_duration,
            rc->delay_multiplier,
            rc->warn_repeat_every,
            rc->crit_repeat_every
    );

    rrdcalc_add_to_host(host, rc);
    if(!rt->foreachdim) {
        RRDCALC *rdcmp  = (RRDCALC *) avl_insert_lock(&(host)->alarms_idx_health_log,(avl *)rc);
        if (rdcmp != rc) {
            error("Cannot insert the alarm index ID %s",rc->name);
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
    newrc->id = rrdcalc_get_unique_id(host, rc->chart, name, &rc->next_event_id);
    newrc->name = (char *)name;
    newrc->hash = simple_hash(newrc->name);
    newrc->chart = strdupz(rc->chart);
    newrc->hash_chart = simple_hash(rc->chart);

    newrc->dimensions = strdupz(dimension);
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
    newrc->warn_repeat_every = rc->warn_repeat_every;
    newrc->crit_repeat_every = rc->crit_repeat_every;

    newrc->group = rc->group;
    newrc->after = rc->after;
    newrc->before = rc->before;
    newrc->update_every = rc->update_every;
    newrc->options = rc->options;

    if(rc->exec) newrc->exec = strdupz(rc->exec);
    if(rc->recipient) newrc->recipient = strdupz(rc->recipient);
    if(rc->source) newrc->source = strdupz(rc->source);
    if(rc->units) newrc->units = strdupz(rc->units);
    if(rc->info) newrc->info = strdupz(rc->info);

    if(rc->calculation) {
        newrc->calculation = expression_parse(rc->calculation->source, NULL, NULL);
        if(!newrc->calculation)
            error("Health alarm '%s.%s': failed to parse calculation expression '%s'", rc->chart, rc->name, rc->calculation->source);
    }

    if(rc->warning) {
        newrc->warning = expression_parse(rc->warning->source, NULL, NULL);
        if(!newrc->warning)
            error("Health alarm '%s.%s': failed to re-parse warning expression '%s'", rc->chart, rc->name, rc->warning->source);
    }

    if(rc->critical) {
        newrc->critical = expression_parse(rc->critical->source, NULL, NULL);
        if(!newrc->critical)
            error("Health alarm '%s.%s': failed to re-parse critical expression '%s'", rc->chart, rc->name, rc->critical->source);
    }

    return newrc;
}

void rrdcalc_free(RRDCALC *rc) {
    if(unlikely(!rc)) return;

    expression_free(rc->calculation);
    expression_free(rc->warning);
    expression_free(rc->critical);

    freez(rc->name);
    freez(rc->chart);
    freez(rc->family);
    freez(rc->dimensions);
    freez(rc->foreachdim);
    freez(rc->exec);
    freez(rc->recipient);
    freez(rc->source);
    freez(rc->units);
    freez(rc->info);
    simple_pattern_free(rc->spdim);
    freez(rc->labels);
    simple_pattern_free(rc->splabels);
    freez(rc);
}

void rrdcalc_unlink_and_free(RRDHOST *host, RRDCALC *rc) {
    if(unlikely(!rc)) return;

    debug(D_HEALTH, "Health removing alarm '%s.%s' of host '%s'", rc->chart?rc->chart:"NOCHART", rc->name, host->hostname);

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
            error("Cannot unlink alarm '%s.%s' from host '%s': not found", rc->chart?rc->chart:"NOCHART", rc->name, host->hostname);
    }

    RRDCALC *rdcmp = (RRDCALC *) avl_search_lock(&(host)->alarms_idx_health_log, (avl *)rc);
    if (rdcmp) {
        rdcmp = (RRDCALC *) avl_remove_lock(&(host)->alarms_idx_health_log, (avl *)rc);
        if (!rdcmp) {
            error("Cannot remove the health alarm index from health_log");
        }
    }

    rdcmp = (RRDCALC *) avl_search_lock(&(host)->alarms_idx_name, (avl *)rc);
    if (rdcmp) {
        rdcmp = (RRDCALC *) avl_remove_lock(&(host)->alarms_idx_name, (avl *)rc);
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
            error("Cannot unlink alarm '%s.%s' from host '%s': not found", rc->chart?rc->chart:"NOCHART", rc->name, host->hostname);
    }

    rrdcalc_free(rc);
}

static void rrdcalc_labels_unlink_alarm_loop(RRDHOST *host, RRDCALC *alarms) {
    RRDCALC *rc = alarms;
    while (rc) {
        if (!rc->labels) {
            rc = rc->next;
            continue;
        }

        char cmp[CONFIG_FILE_LINE_MAX+1];
        struct label *move = host->labels;
        while(move) {
            snprintf(cmp, CONFIG_FILE_LINE_MAX, "%s=%s", move->key, move->value);
            if (simple_pattern_matches(rc->splabels, move->key) ||
                simple_pattern_matches(rc->splabels, cmp)) {
                break;
            }

            move = move->next;
        }

        RRDCALC *next = rc->next;
        if(!move) {
            info("Health configuration for alarm '%s' cannot be applied, because the host %s does not have the label(s) '%s'",
                 rc->name,
                 host->hostname,
                 rc->labels);

            if(host->alarms == alarms) {
                rrdcalc_unlink_and_free(host, rc);
            } else
                rrdcalc_foreach_unlink_and_free(host, rc);

        }

        rc = next;
    }
}

void rrdcalc_labels_unlink_alarm_from_host(RRDHOST *host) {
    netdata_rwlock_rdlock(&host->labels_rwlock);

    rrdcalc_labels_unlink_alarm_loop(host, host->alarms);
    rrdcalc_labels_unlink_alarm_loop(host, host->alarms_with_foreach);

    netdata_rwlock_unlock(&host->labels_rwlock);
}

void rrdcalc_labels_unlink() {
    rrd_rdlock();

    RRDHOST *host;
    rrdhost_foreach_read(host) {
        if (unlikely(!host->health_enabled))
            continue;

        if (host->labels) {
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
    RRDCALC *rc = (RRDCALC *)avl_search_lock(&host->alarms_idx_health_log, (avl *)&findme);
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
RRDCALC *alarm_max_last_repeat(RRDHOST *host, char *alarm_name,uint32_t hash) {
    RRDCALC findme;
    findme.name = alarm_name;
    findme.hash = hash;
    RRDCALC *rc = (RRDCALC *)avl_search_lock(&host->alarms_idx_name, (avl *)&findme);

    return rc;
}

// SPDX-License-Identifier: GPL-3.0-or-later

#include "database/rrd.h"
#include "health_internals.h"
#include "health-alert-entry.h"

// ----------------------------------------------------------------------------
// RRDCALC helpers

void rrdcalc_flags_to_json_array(BUFFER *wb, const char *key, RRDCALC_FLAGS flags) {
    buffer_json_member_add_array(wb, key);

    if(flags & RRDCALC_FLAG_DB_ERROR)
        buffer_json_add_array_item_string(wb, "DB_ERROR");
    if(flags & RRDCALC_FLAG_DB_NAN)
        buffer_json_add_array_item_string(wb, "DB_NAN");
    if(flags & RRDCALC_FLAG_CALC_ERROR)
        buffer_json_add_array_item_string(wb, "CALC_ERROR");
    if(flags & RRDCALC_FLAG_WARN_ERROR)
        buffer_json_add_array_item_string(wb, "WARN_ERROR");
    if(flags & RRDCALC_FLAG_CRIT_ERROR)
        buffer_json_add_array_item_string(wb, "CRIT_ERROR");
    if(flags & RRDCALC_FLAG_RUNNABLE)
        buffer_json_add_array_item_string(wb, "RUNNABLE");
    if(flags & RRDCALC_FLAG_DISABLED)
        buffer_json_add_array_item_string(wb, "DISABLED");
    if(flags & RRDCALC_FLAG_SILENCED)
        buffer_json_add_array_item_string(wb, "SILENCED");
    if(flags & RRDCALC_FLAG_RUN_ONCE)
        buffer_json_add_array_item_string(wb, "RUN_ONCE");

    buffer_json_array_close(wb);
}

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
            netdata_log_error("Unknown alarm status %d", status);
            return "UNKNOWN";
    }
}

uint32_t rrdcalc_get_unique_id(RRDHOST *host, STRING *chart, STRING *name, uint32_t *next_event_id, nd_uuid_t *config_hash_id) {
    rw_spinlock_read_lock(&host->health_log.spinlock);

    // re-use old IDs, by looking them up in the alarm log
    ALARM_ENTRY *ae = NULL;
    for(ae = host->health_log.alarms; ae ;ae = ae->next) {
        if(unlikely(name == ae->name && chart == ae->chart && uuid_eq(ae->config_hash_id, *config_hash_id))) {
            if(next_event_id) *next_event_id = ae->alarm_event_id + 1;
            break;
        }
    }

    uint32_t alarm_id;

    if(ae)
        alarm_id = ae->alarm_id;
    else {
        alarm_id = sql_get_alarm_id(host, chart, name, next_event_id);
        if (!alarm_id) {
            if (unlikely(!host->health_log.next_alarm_id))
                host->health_log.next_alarm_id = get_uint32_id();
            alarm_id = host->health_log.next_alarm_id++;
        }
    }

    rw_spinlock_read_unlock(&host->health_log.spinlock);
    return alarm_id;
}

// ----------------------------------------------------------------------------
// RRDCALC replacing info/summary text variables with RRDSET labels

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
                lbl_value = NULL;
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
    if(rc->rrdset && rc->rrdset->rrdlabels) {
        size_t labels_version = rrdlabels_version(rc->rrdset->rrdlabels);
        if (rc->labels_version != labels_version) {
            STRING *old;

            old = rc->info;
            rc->info = rrdcalc_replace_variables_with_rrdset_labels(string2str(rc->config.info), rc);
            string_freez(old);

            old = rc->summary;
            rc->summary = rrdcalc_replace_variables_with_rrdset_labels(string2str(rc->config.summary), rc);
            string_freez(old);

            rc->labels_version = labels_version;
        }
    }

    if(!rc->summary)
        rc->summary = string_dup(rc->config.summary);

    if(!rc->info)
        rc->info = string_dup(rc->config.info);
}

// ----------------------------------------------------------------------------
// RRDCALC index management for RRDSET

// the dictionary requires a unique key for every item
// we use {chart id}.{alert name} for both the RRDHOST and RRDSET alert indexes.

#define RRDCALC_MAX_KEY_SIZE 1024
static size_t rrdcalc_key(char *dst, size_t dst_len, const char *chart, const char *alert) {
    return snprintfz(dst, dst_len, "%s,on[%s]", alert, chart);
}

const RRDCALC_ACQUIRED *rrdcalc_from_rrdset_get(RRDSET *st, const char *alert_name) {
    char key[RRDCALC_MAX_KEY_SIZE + 1];
    size_t key_len = rrdcalc_key(key, RRDCALC_MAX_KEY_SIZE, rrdset_id(st), alert_name);

    const RRDCALC_ACQUIRED *rca = (const RRDCALC_ACQUIRED *)dictionary_get_and_acquire_item_advanced(st->rrdhost->rrdcalc_root_index, key, (ssize_t)key_len);

    if(!rca) {
        key_len = rrdcalc_key(key, RRDCALC_MAX_KEY_SIZE, rrdset_name(st), alert_name);
        rca = (const RRDCALC_ACQUIRED *)dictionary_get_and_acquire_item_advanced(st->rrdhost->rrdcalc_root_index, key, (ssize_t)key_len);
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

static void rrdcalc_link_to_rrdset(RRDCALC *rc) {
    RRDSET *st = rc->rrdset;
    RRDHOST *host = st->rrdhost;

    rw_spinlock_write_lock(&st->alerts.spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(st->alerts.base, rc, prev, next);
    rw_spinlock_write_unlock(&st->alerts.spinlock);

    char buf[RRDVAR_MAX_LENGTH + 1];
    snprintfz(buf, RRDVAR_MAX_LENGTH, "%s.%s", rrdset_name(st), rrdcalc_name(rc));
    STRING *rrdset_name_rrdcalc_name = string_strdupz(buf);
    snprintfz(buf, RRDVAR_MAX_LENGTH, "%s.%s", rrdset_id(st), rrdcalc_name(rc));
    STRING *rrdset_id_rrdcalc_name = string_strdupz(buf);

    string_freez(rrdset_id_rrdcalc_name);
    string_freez(rrdset_name_rrdcalc_name);

    time_t now = now_realtime_sec();
    ALARM_ENTRY *ae = health_create_alarm_entry(
        host,
        rc,
        now,
        now - rc->last_status_change,
        rc->old_value,
        rc->value,
        RRDCALC_STATUS_REMOVED,
        rc->status,
        0,
        rrdcalc_isrepeating(rc)?HEALTH_ENTRY_FLAG_IS_REPEATING:0);

    health_log_alert(host, ae);
    health_alarm_log_add_entry(host, ae, false);
    rrdset_flag_set(st, RRDSET_FLAG_HAS_RRDCALC_LINKED);
}

static void rrdcalc_unlink_from_rrdset(RRDCALC *rc, bool having_ll_wrlock) {
    RRDSET *st = rc->rrdset;

    if(!st) {
        netdata_log_error(
            "Requested to unlink RRDCALC '%s.%s' which is not linked to any RRDSET",
            rrdcalc_chart_name(rc), rrdcalc_name(rc));
        return;
    }

    if (!exit_initiated) {
        RRDHOST *host = st->rrdhost;

        time_t now = now_realtime_sec();

        if (likely(rc->status != RRDCALC_STATUS_REMOVED)) {
            ALARM_ENTRY *ae = health_create_alarm_entry(
                host,
                rc,
                now,
                now - rc->last_status_change,
                rc->old_value,
                rc->value,
                rc->status,
                RRDCALC_STATUS_REMOVED,
                0,
                0);

            health_log_alert(host, ae);
            health_alarm_log_add_entry(host, ae, true);
        }
    }

    // unlink it

    if(!having_ll_wrlock)
        rw_spinlock_write_lock(&st->alerts.spinlock);

    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(st->alerts.base, rc, prev, next);

    if(!having_ll_wrlock)
        rw_spinlock_write_unlock(&st->alerts.spinlock);

    rc->rrdset = NULL;
}

// ----------------------------------------------------------------------------
// RRDCALC rrdhost index management - constructor

struct rrdcalc_constructor {
    RRDSET *rrdset;
    RRD_ALERT_PROTOTYPE *ap;

    enum {
        RRDCALC_REACT_NONE,
        RRDCALC_REACT_NEW,
    } react_action;
};

static void rrdcalc_rrdhost_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdcalc, void *constructor_data) {
    RRDCALC *rc = rrdcalc;
    struct rrdcalc_constructor *ctr = constructor_data;
    RRDSET *st = ctr->rrdset;
    RRDHOST *host = st->rrdhost;
    RRD_ALERT_PROTOTYPE *ap = ctr->ap;

    rc->key = string_strdupz(dictionary_acquired_item_name(item));
    rc->rrdset = st;
    rc->chart = string_dup(st->id);

    health_prototype_copy_config(&rc->config, &ap->config);
    health_prototype_copy_match_without_patterns(&rc->match, &ap->match);

    rc->next_event_id = 1;
    rc->value = NAN;
    rc->old_value = NAN;
    rc->last_repeat = 0;
    rc->times_repeat = 0;
    rc->last_status_change_value = rc->value;
    rc->last_status_change = now_realtime_sec();

    if(!rc->config.units)
        rc->config.units = string_dup(st->units);

    // the following interferes with replication, changing the alert frequency to unexpected values
    // let's respect user configuration, so we disable it
    
//    if(rc->config.update_every < rc->rrdset->update_every) {
//        netdata_log_info(
//            "HEALTH: alert '%s.%s' has update every %d, less than chart update every %d. "
//            "Setting alarm update frequency to %d.",
//            string2str(st->id), string2str(rc->config.name),
//            rc->config.update_every, rc->rrdset->update_every, rc->rrdset->update_every);
//
//        rc->config.update_every = st->update_every;
//    }

    rc->id = rrdcalc_get_unique_id(host, rc->chart, rc->config.name, &rc->next_event_id, &rc->config.hash_id);

    expression_set_variable_lookup_callback(rc->config.calculation, alert_variable_lookup, rc);
    expression_set_variable_lookup_callback(rc->config.warning, alert_variable_lookup, rc);
    expression_set_variable_lookup_callback(rc->config.critical, alert_variable_lookup, rc);

    rrdcalc_update_info_using_rrdset_labels(rc);

    ctr->react_action = RRDCALC_REACT_NEW;
}

static bool rrdcalc_rrdhost_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdcalc __maybe_unused, void *rrdcalc_new __maybe_unused, void *constructor_data) {
    struct rrdcalc_constructor *ctr = constructor_data;
    ctr->react_action = RRDCALC_REACT_NONE;
    return false;
}

static void rrdcalc_rrdhost_react_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdcalc, void *constructor_data) {
    RRDCALC *rc = rrdcalc;
    struct rrdcalc_constructor *ctr = constructor_data;

    if(ctr->react_action == RRDCALC_REACT_NEW)
        rrdcalc_link_to_rrdset(rc);
}

// ----------------------------------------------------------------------------
// RRDCALC rrdhost index management - destructor

static void rrdcalc_free_internals(RRDCALC *rc) {
    if(unlikely(!rc)) return;

    rrd_alert_match_cleanup(&rc->match);
    rrd_alert_config_cleanup(&rc->config);

    string_freez(rc->key);
    string_freez(rc->chart);

    string_freez(rc->info);
    string_freez(rc->summary);
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

bool rrdcalc_add_from_prototype(RRDHOST *host, RRDSET *st, RRD_ALERT_PROTOTYPE *ap) {
    char key[RRDCALC_MAX_KEY_SIZE + 1];
    size_t key_len = rrdcalc_key(key, RRDCALC_MAX_KEY_SIZE,
                                 string2str(st->id), string2str(ap->config.name));

    struct rrdcalc_constructor tmp = {
        .ap = ap,
        .rrdset = st,
        .react_action = RRDCALC_REACT_NONE,
    };

    bool ret = true;

    dictionary_set_advanced(host->rrdcalc_root_index, key, (ssize_t)key_len,
                            NULL, sizeof(RRDCALC), &tmp);

    if(tmp.react_action != RRDCALC_REACT_NEW)
        ret = false;

    return ret;
}

void rrdcalc_unlink_and_delete(RRDHOST *host, RRDCALC *rc, bool having_ll_wrlock) {
    if(rc->rrdset)
        rrdcalc_unlink_from_rrdset(rc, having_ll_wrlock);

    dictionary_del_advanced(host->rrdcalc_root_index, string2str(rc->key), (ssize_t)string_strlen(rc->key));
}


// ----------------------------------------------------------------------------
// RRDCALC cleanup API functions

void rrdcalc_unlink_and_delete_all_rrdset_alerts(RRDSET *st) {
    RRDCALC *rc, *last = NULL;
    rw_spinlock_write_lock(&st->alerts.spinlock);
    while((rc = st->alerts.base)) {
        if(last == rc) {
            netdata_log_error("RRDCALC: malformed list of alerts linked to chart - cannot cleanup - giving up.");
            break;
        }
        last = rc;

        rrdcalc_unlink_and_delete(st->rrdhost, rc, true);
    }
    rw_spinlock_write_unlock(&st->alerts.spinlock);
}

void rrdcalc_delete_all(RRDHOST *host) {
    dictionary_flush(host->rrdcalc_root_index);
}

void rrdcalc_child_disconnected(RRDHOST *host) {
    rrdcalc_delete_all(host);

    rrdhost_flag_clear(host, RRDHOST_FLAG_PENDING_HEALTH_INITIALIZATION);
    RRDSET *st;
    rrdset_foreach_read(st, host) {
        rrdset_flag_clear(st, RRDSET_FLAG_PENDING_HEALTH_INITIALIZATION);
    }
    rrdset_foreach_done(st);
}

void rrd_alert_match_cleanup(struct rrd_alert_match *am) {
    if(am->is_template)
        string_freez(am->on.context);
    else
        string_freez(am->on.chart);

    string_freez(am->host_labels);
    pattern_array_free(am->host_labels_pattern);

    string_freez(am->chart_labels);
    pattern_array_free(am->chart_labels_pattern);
}

void rrd_alert_config_cleanup(struct rrd_alert_config *ac) {
    string_freez(ac->name);

    string_freez(ac->exec);
    string_freez(ac->recipient);

    string_freez(ac->classification);
    string_freez(ac->component);
    string_freez(ac->type);

    string_freez(ac->source);
    string_freez(ac->units);
    string_freez(ac->summary);
    string_freez(ac->info);

    string_freez(ac->dimensions);

    expression_free(ac->calculation);
    expression_free(ac->warning);
    expression_free(ac->critical);
}

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

void rrdcalc_runtime_snapshot_publish(RRDCALC *rc, usec_t global_id, const nd_uuid_t *transition_id) {
    rw_spinlock_write_lock(&rc->runtime_snapshot.spinlock);

    RRDCALC_RUNTIME_SNAPSHOT *state = &rc->runtime_snapshot.state;
    state->status = rc->status;
    state->run_flags = rc->run_flags;
    state->value = rc->value;
    state->last_updated = rc->last_updated;
    state->last_status_change = rc->last_status_change;
    state->last_status_change_value = rc->last_status_change_value;
    state->next_update = rc->next_update;
    state->db_after = rc->db_after;
    state->db_before = rc->db_before;
    state->delay_up_to_timestamp = rc->delay_up_to_timestamp;
    state->last_repeat = rc->last_repeat;
    state->delay_last = rc->delay_last;
    state->times_repeat = rc->times_repeat;

    if(transition_id) {
        state->global_id = global_id;
        uuid_copy(state->last_transition_id, *transition_id);
    }

    rw_spinlock_write_unlock(&rc->runtime_snapshot.spinlock);
}

void rrdcalc_runtime_snapshot_publish_run_flags(RRDCALC *rc) {
    rw_spinlock_write_lock(&rc->runtime_snapshot.spinlock);
    rc->runtime_snapshot.state.run_flags = rc->run_flags;
    rw_spinlock_write_unlock(&rc->runtime_snapshot.spinlock);
}

void rrdcalc_runtime_snapshot_publish_repeat_state(RRDCALC *rc) {
    rw_spinlock_write_lock(&rc->runtime_snapshot.spinlock);
    rc->runtime_snapshot.state.run_flags = rc->run_flags;
    rc->runtime_snapshot.state.last_repeat = rc->last_repeat;
    rc->runtime_snapshot.state.times_repeat = rc->times_repeat;
    rw_spinlock_write_unlock(&rc->runtime_snapshot.spinlock);
}

void rrdcalc_runtime_snapshot_get(RRDCALC *rc, RRDCALC_RUNTIME_SNAPSHOT *snapshot) {
    rw_spinlock_read_lock(&rc->runtime_snapshot.spinlock);
    *snapshot = rc->runtime_snapshot.state;
    rw_spinlock_read_unlock(&rc->runtime_snapshot.spinlock);
}

void rrdcalc_runtime_strings_acquire(RRDCALC *rc, STRING **summary, STRING **info) {
    rw_spinlock_read_lock(&rc->runtime_snapshot.spinlock);

    if(summary)
        *summary = string_dup(rc->summary);
    if(info)
        *info = string_dup(rc->info);

    rw_spinlock_read_unlock(&rc->runtime_snapshot.spinlock);
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

static ALARM_ENTRY *rrdcalc_find_alarm_entry(RRDHOST *host, STRING *chart, STRING *name, nd_uuid_t *config_hash_id) {
    for(ALARM_ENTRY *ae = host->health_log.alarms; ae ;ae = ae->next) {
        if(unlikely(name == ae->name && chart == ae->chart && uuid_eq(ae->config_hash_id, *config_hash_id)))
            return ae;
    }

    return NULL;
}

uint32_t rrdcalc_get_unique_id(RRDHOST *host, STRING *chart, STRING *name, uint32_t *next_event_id, nd_uuid_t *config_hash_id) {
    rw_spinlock_read_lock(&host->health_log.spinlock);

    // re-use old IDs, by looking them up in the alarm log
    ALARM_ENTRY *ae = rrdcalc_find_alarm_entry(host, chart, name, config_hash_id);

    if(ae) {
        uint32_t alarm_id = ae->alarm_id;
        if(next_event_id) *next_event_id = ae->alarm_event_id + 1;
        rw_spinlock_read_unlock(&host->health_log.spinlock);
        return alarm_id;
    }

    rw_spinlock_read_unlock(&host->health_log.spinlock);

    uint32_t sql_next_event_id = next_event_id ? *next_event_id : 0;
    uint32_t sql_alarm_id = sql_get_alarm_id(host, chart, name, &sql_next_event_id);

    rw_spinlock_write_lock(&host->health_log.spinlock);

    ae = rrdcalc_find_alarm_entry(host, chart, name, config_hash_id);

    uint32_t alarm_id;
    if(ae) {
        alarm_id = ae->alarm_id;
        if(next_event_id) *next_event_id = ae->alarm_event_id + 1;
    }
    else if(sql_alarm_id) {
        alarm_id = sql_alarm_id;
        if(next_event_id) *next_event_id = sql_next_event_id;
    }
    else {
        if (unlikely(!host->health_log.next_alarm_id))
            host->health_log.next_alarm_id = get_uint32_id();
        alarm_id = host->health_log.next_alarm_id++;
    }

    rw_spinlock_write_unlock(&host->health_log.spinlock);
    return alarm_id;
}

// ----------------------------------------------------------------------------
// RRDCALC replacing info/summary text variables with RRDSET labels

static void rrdcalc_runtime_string_replace(RRDCALC *rc, STRING **field, STRING *replacement) {
    rw_spinlock_write_lock(&rc->runtime_snapshot.spinlock);
    STRING *old = *field;
    *field = replacement;
    rw_spinlock_write_unlock(&rc->runtime_snapshot.spinlock);

    string_freez(old);
}

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
        size_t match_pos = m - temp;
        pos = match_pos + 1;

        if (!strcmp(var, RRDCALC_VAR_FAMILY)) {
            const char *family = (rc->rrdset && rc->rrdset->family) ? rrdset_family(rc->rrdset) : "";
            char *buf = find_and_replace(temp, var, family, m);
            pos = match_pos + strlen(family);
            freez(temp);
            temp = buf;
        }
        else if (!strncmp(var, RRDCALC_VAR_LABEL, RRDCALC_VAR_LABEL_LEN) &&
                 i > (int)RRDCALC_VAR_LABEL_LEN && var[i - 1] == '}') {
            char label_val[RRDCALC_VAR_MAX + RRDCALC_VAR_LABEL_LEN + 1] = { 0 };
            strcpy(label_val, var+RRDCALC_VAR_LABEL_LEN);
            label_val[i - RRDCALC_VAR_LABEL_LEN - 1] = '\0';

            if(likely(rc->rrdset && rc->rrdset->rrdlabels)) {
                lbl_value = NULL;
                rrdlabels_get_value_strdup_or_null(rc->rrdset->rrdlabels, &lbl_value, label_val);
                if (lbl_value) {
                    char *buf = find_and_replace(temp, var, lbl_value, m);
                    pos = match_pos + strlen(lbl_value);
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
        uint32_t labels_version = rrdlabels_version(rc->rrdset->rrdlabels);
        if (rc->labels_version != labels_version) {
            STRING *info = rrdcalc_replace_variables_with_rrdset_labels(string2str(rc->config.info), rc);
            rrdcalc_runtime_string_replace(rc, &rc->info, info);

            STRING *summary = rrdcalc_replace_variables_with_rrdset_labels(string2str(rc->config.summary), rc);
            rrdcalc_runtime_string_replace(rc, &rc->summary, summary);

            rc->labels_version = labels_version;
        }
    }

    if(!rc->summary)
        rrdcalc_runtime_string_replace(rc, &rc->summary, string_dup(rc->config.summary));

    if(!rc->info)
        rrdcalc_runtime_string_replace(rc, &rc->info, string_dup(rc->config.info));
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
        nd_time_t_elapsed_saturating(now, rc->last_status_change),
        rc->old_value,
        rc->value,
        RRDCALC_STATUS_REMOVED,
        rc->status,
        0,
        rrdcalc_isrepeating(rc)?HEALTH_ENTRY_FLAG_IS_REPEATING:0);

    health_alarm_log_add_entry(host, ae, true);
    health_log_alert(host, ae);
    rrdcalc_runtime_snapshot_publish(rc, ae->global_id, &ae->transition_id);
    rrdset_flag_set(st, RRDSET_FLAG_HAS_RRDCALC_LINKED);
}

static void rrdcalc_unlink_from_rrdset(RRDCALC *rc, bool having_ll_wrlock) {
    RRDSET *st = rc->rrdset;
    if(!st) return;

    if (!exit_initiated_get()) {
        time_t now = now_realtime_sec();

        if (likely(rc->status != RRDCALC_STATUS_REMOVED)) {
            RRDHOST *host = st->rrdhost;
            ALARM_ENTRY *ae = health_create_alarm_entry(
                host,
                rc,
                now,
                nd_time_t_elapsed_saturating(now, rc->last_status_change),
                rc->old_value,
                rc->value,
                rc->status,
                RRDCALC_STATUS_REMOVED,
                    0,
                    0);

            health_alarm_log_add_entry(host, ae, true);
            health_log_alert(host, ae);
        }
    }

    if(!having_ll_wrlock)
        rw_spinlock_write_lock(&st->alerts.spinlock);

    if(rc->prev)
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(st->alerts.base, rc, prev, next);

    rc->rrdset = NULL;

    if(!having_ll_wrlock)
        rw_spinlock_write_unlock(&st->alerts.spinlock);
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

// defined further down, next to the rest of the name-index implementation
static void rrdcalc_name_index_add(RRDHOST *host, RRDCALC *rc, const DICTIONARY_ITEM *item);
static void rrdcalc_name_index_del(RRDHOST *host, RRDCALC *rc);

static void rrdcalc_rrdhost_insert_callback(const DICTIONARY_ITEM *item, void *rrdcalc, void *constructor_data) {
    RRDCALC *rc = rrdcalc;
    struct rrdcalc_constructor *ctr = constructor_data;
    RRDSET *st = ctr->rrdset;
    RRDHOST *host = st->rrdhost;
    RRD_ALERT_PROTOTYPE *ap = ctr->ap;

    rc->key = string_strdupz(dictionary_acquired_item_name(item));
    rc->rrdset = st;
    rc->chart = string_dup(st->id);

    health_prototype_copy_config(&rc->config, &ap->config);

    rc->next_event_id = 1;
    rc->value = NAN;
    rc->old_value = NAN;
    rc->last_repeat = 0;
    rc->times_repeat = 0;
    rc->last_status_change_value = rc->value;
    rc->last_status_change = now_realtime_sec();
    rw_spinlock_init(&rc->runtime_snapshot.spinlock);

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
    rrdcalc_runtime_snapshot_publish(rc, 0, NULL);

    rrdcalc_name_index_add(host, rc, item);

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

static __thread bool thread_having_ll_wrlock = false;

static void rrdcalc_rrdhost_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdcalc, void *rrdhost) {
    RRDCALC *rc = rrdcalc;
    RRDHOST *host = rrdhost;

    // safety net: rrdcalc_unlink_and_delete() already did this, but the
    // dictionary can reach the delete callback by other routes. Idempotent.
    if(likely(host))
        rrdcalc_name_index_del(host, rc);

    rrdcalc_unlink_from_rrdset(rc, thread_having_ll_wrlock);

    // any destruction actions that require other locks
    // have to be placed in rrdcalc_del(), because the object is actually locked for deletion

    rrd_alert_config_cleanup(&rc->config);

    string_freez(rc->key);
    string_freez(rc->chart);

    string_freez(rc->info);
    string_freez(rc->summary);

    memset(rc, 0, sizeof(*rc));
}

// ----------------------------------------------------------------------------
// RRDCALC rrdhost index management - secondary index by alert name
//
// Keyed by the interned STRING* of rc->config.name, which is assigned once in
// rrdcalc_rrdhost_insert_callback() and never mutated afterwards (the conflict
// callback returns false and the dictionary is DICT_OPTION_DONT_OVERWRITE_VALUE),
// so the key is stable for the lifetime of the RRDCALC.
//
// Locking: this index has its own spinlock rather than relying on the alert
// dictionary lock, because rrdcalc_unlink_and_delete() is reached with only the
// dictionary READ lock held from health_prototype_apply_to_all_hosts(), which
// iterates reentrantly.

static void rrdcalc_name_index_add(RRDHOST *host, RRDCALC *rc, const DICTIONARY_ITEM *item) {
    if(unlikely(!rc->config.name || !item))
        return;

    rw_spinlock_write_lock(&host->rrdcalc_by_name.spinlock);

    // Decide before touching the JudyL: inserting first would leave a
    // name -> NULL slot behind if we then declined to link.
    if(likely(rc->name_index_state != RRDCALC_NAME_INDEX_LINKED)) {
        JError_t J_Error;
        Pvoid_t *PValue = JudyLIns(&host->rrdcalc_by_name.JudyL, (Word_t)rc->config.name, &J_Error);

        // A failed insert cannot be recovered from and nothing retries it: the
        // alert would stay invisible to every name lookup, silently taking
        // dependent alerts to UNDEFINED. Same handling as the other structural
        // Judy indexes (see string.c and uuidmap.c).
        if(unlikely(!PValue || PValue == PJERR))
            fatal("HEALTH: cannot insert alert '%s' of chart '%s' on host '%s' into the alert name index, "
                  "JU_ERRNO_* == %u, ID == %d",
                  rrdcalc_name(rc), rrdcalc_chart_name(rc), rrdhost_hostname(host),
                  JU_ERRNO(&J_Error), JU_ERRID(&J_Error));

        RRDCALC *head = (RRDCALC *)*PValue;
        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(head, rc, name_prev, name_next);
        rc->name_item = item;
        rc->name_index_state = RRDCALC_NAME_INDEX_LINKED;
        *PValue = head;
    }

    rw_spinlock_write_unlock(&host->rrdcalc_by_name.spinlock);
}

// Removing takes the index lock while the caller may already hold
// st->alerts.spinlock (rrdcalc_unlink_and_delete_all_rrdset_alerts() holds it
// write across its loop). That fixes the writer order as
// alerts.spinlock -> name index, so readers must never take the name index and
// then a chart lock - see rrdcalc_by_name_snapshot().
static void rrdcalc_name_index_del(RRDHOST *host, RRDCALC *rc) {
    if(unlikely(!rc->config.name))
        return;

    rw_spinlock_write_lock(&host->rrdcalc_by_name.spinlock);

    // name_index_state makes this idempotent, which matters because both
    // rrdcalc_unlink_and_delete() and the dictionary delete callback call it.
    // It also avoids inferring membership from the list links, which would be
    // wrong: DOUBLE_LINKED_LIST keeps head->prev pointing at the tail, so a
    // single-item list has item->prev == item rather than NULL.
    Pvoid_t *PValue = JudyLGet(host->rrdcalc_by_name.JudyL, (Word_t)rc->config.name, PJE0);
    if(likely(PValue && PValue != PJERR && *PValue && rc->name_index_state == RRDCALC_NAME_INDEX_LINKED)) {
        RRDCALC *head = (RRDCALC *)*PValue;

        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(head, rc, name_prev, name_next);
        rc->name_index_state = RRDCALC_NAME_INDEX_UNLINKED;
        rc->name_prev = rc->name_next = NULL;
        rc->name_item = NULL;

        if(head)
            *PValue = head;
        else
            JudyLDel(&host->rrdcalc_by_name.JudyL, (Word_t)rc->config.name, PJE0);
    }

    rw_spinlock_write_unlock(&host->rrdcalc_by_name.spinlock);
}

size_t rrdcalc_by_name_snapshot(RRDHOST *host, STRING *name, const DICTIONARY_ITEM **dst, size_t dst_size) {
    size_t found = 0;

    // The index lock is what makes acquiring safe: every route that frees an
    // RRDCALC removes it from this index first (rrdcalc_unlink_and_delete(), or
    // the delete callback as a safety net), so an entry visible here still
    // points at live memory for as long as we hold the lock.
    rw_spinlock_read_lock(&host->rrdcalc_by_name.spinlock);

    Pvoid_t *PValue = JudyLGet(host->rrdcalc_by_name.JudyL, (Word_t)name, PJE0);
    if(PValue && PValue != PJERR) {
        // count first, so we never acquire references we would have to throw
        // away when the caller retries with a bigger buffer
        for(RRDCALC *rc = (RRDCALC *)*PValue; rc ; rc = rc->name_next)
            found++;

        if(likely(found <= dst_size)) {
            size_t used = 0;
            for(RRDCALC *rc = (RRDCALC *)*PValue; rc ; rc = rc->name_next) {
                const DICTIONARY_ITEM *item =
                    dictionary_item_acquire_if_not_deleted(host->rrdcalc_root_index, rc->name_item);

                if(likely(item))
                    dst[used++] = item;
            }

            // alerts being deleted were skipped
            found = used;
        }
    }

    rw_spinlock_read_unlock(&host->rrdcalc_by_name.spinlock);

    return found;
}

#ifdef NETDATA_INTERNAL_CHECKS
// A missing index entry does not crash: it silently makes the alert's name
// unresolvable as a variable and takes dependent alerts to UNDEFINED. Verify it
// loudly instead.
//
// This checks one alert against the index at one instant, under the index lock.
// It deliberately does NOT compare a name-index snapshot against a full
// dictionary traversal: nothing serializes those two views (the reapplication
// paths in health_prototypes.c and health_dyncfg.c delete alerts from reentrant
// traversals, which hold no dictionary lock in the loop body), so any difference
// between them is as likely to be a legitimate concurrent transition as a bug.
void rrdcalc_name_index_verify(RRDHOST *host, RRDCALC *rc) {
    if(unlikely(!rc->config.name))
        return;

    rw_spinlock_read_lock(&host->rrdcalc_by_name.spinlock);

    RRDCALC_NAME_INDEX_STATE state = rc->name_index_state;
    bool linked = false;

    if(state == RRDCALC_NAME_INDEX_LINKED) {
        Pvoid_t *PValue = JudyLGet(host->rrdcalc_by_name.JudyL, (Word_t)rc->config.name, PJE0);
        if(PValue && PValue != PJERR) {
            for(RRDCALC *t = (RRDCALC *)*PValue; t ; t = t->name_next) {
                if(t == rc) {
                    linked = true;
                    break;
                }
            }
        }
    }

    rw_spinlock_read_unlock(&host->rrdcalc_by_name.spinlock);

    // UNLINKED is legitimate here: rrdcalc_unlink_and_delete() removes the alert
    // from the index before it removes it from the dictionary, so a caller
    // holding a dictionary reference can legally see an alert in that window.
    if(unlikely(state == RRDCALC_NAME_INDEX_NEVER))
        fatal("HEALTH: alert '%s' of chart '%s' on host '%s' is in the alert dictionary "
              "but was never added to the alert name index",
              rrdcalc_name(rc), rrdcalc_chart_name(rc), rrdhost_hostname(host));

    if(unlikely(state == RRDCALC_NAME_INDEX_LINKED && !linked))
        fatal("HEALTH: alert '%s' of chart '%s' on host '%s' is flagged as indexed "
              "but is not in the alert name index",
              rrdcalc_name(rc), rrdcalc_chart_name(rc), rrdhost_hostname(host));
}
#endif

// ----------------------------------------------------------------------------
// RRDCALC rrdhost index management - index API

void rrdcalc_rrdhost_index_init(RRDHOST *host) {
    // called more than once per host (rrdhost.c:499 and :739), so everything
    // here must stay behind the same idempotency guard as the dictionary
    if(!host->rrdcalc_root_index) {
        rw_spinlock_init(&host->rrdcalc_by_name.spinlock);

        host->rrdcalc_root_index = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                              &dictionary_stats_category_rrdhealth, sizeof(RRDCALC));

        dictionary_register_insert_callback(host->rrdcalc_root_index, rrdcalc_rrdhost_insert_callback, NULL);
        dictionary_register_conflict_callback(host->rrdcalc_root_index, rrdcalc_rrdhost_conflict_callback, NULL);
        dictionary_register_react_callback(host->rrdcalc_root_index, rrdcalc_rrdhost_react_callback, NULL);
        dictionary_register_delete_callback(host->rrdcalc_root_index, rrdcalc_rrdhost_delete_callback, host);
    }
}

void rrdcalc_rrdhost_index_destroy(RRDHOST *host) {
    rrdcalc_delete_all(host);
    dictionary_destroy(host->rrdcalc_root_index);
    host->rrdcalc_root_index = NULL;

    // every alert is gone, so the name index must be empty; free whatever the
    // Judy array itself still holds and report residue, which would mean a
    // deletion path bypassed rrdcalc_name_index_del()
    rw_spinlock_write_lock(&host->rrdcalc_by_name.spinlock);
    Word_t idx = 0;
    JError_t J_Error;
    Pvoid_t *PValue = JudyLFirst(host->rrdcalc_by_name.JudyL, &idx, &J_Error);
    if(unlikely(PValue == PJERR))
        // a corrupted index is not residue - do not report it as such
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "HEALTH: cannot walk the alert name index of host '%s' at teardown, "
               "JU_ERRNO_* == %u, ID == %d",
               rrdhost_hostname(host), JU_ERRNO(&J_Error), JU_ERRID(&J_Error));
    else if(unlikely(PValue))
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "HEALTH: host '%s' still has entries in the alert name index at teardown",
               rrdhost_hostname(host));
    JudyLFreeArray(&host->rrdcalc_by_name.JudyL, PJE0);
    rw_spinlock_write_unlock(&host->rrdcalc_by_name.spinlock);
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
    // remove from the name index first, while rc and host are both in hand and
    // the caller's lock state is known; the delete callback repeats this
    // defensively for any path that does not come through here
    rrdcalc_name_index_del(host, rc);

    rrdcalc_unlink_from_rrdset(rc, having_ll_wrlock);

    thread_having_ll_wrlock = having_ll_wrlock;
    dictionary_del_advanced(host->rrdcalc_root_index, string2str(rc->key), (ssize_t)string_strlen(rc->key));
    thread_having_ll_wrlock = false;
}

// ----------------------------------------------------------------------------
// RRDCALC cleanup API functions

void rrdcalc_unlink_and_delete_all_rrdset_alerts(RRDSET *st) {
    RRDHOST *host = st->rrdhost;
    RRDCALC *rc, *last = NULL;

    // Acquire the host alert dictionary write lock to exclude the health
    // thread (which mutates rc fields under foreach_rrdcalc_in_rrdhost_read),
    // then walk only this chart's alert list to keep the work O(chart-alerts).
    // The lock is recursive, so dictionary_del_advanced() inside the loop
    // re-enters safely.
    dictionary_write_lock(host->rrdcalc_root_index);
    rw_spinlock_write_lock(&st->alerts.spinlock);

    while((rc = st->alerts.base)) {
        if(last == rc) {
            netdata_log_error("RRDCALC: malformed list of alerts linked to chart - cannot cleanup - giving up.");
            break;
        }
        last = rc;

        rrdcalc_unlink_and_delete(host, rc, true);
    }

    rw_spinlock_write_unlock(&st->alerts.spinlock);
    dictionary_write_unlock(host->rrdcalc_root_index);
}

void rrdcalc_delete_all(RRDHOST *host) {
    RRDCALC *rc;
    foreach_rrdcalc_in_rrdhost_write(host, rc) {
        rrdcalc_unlink_and_delete(host, rc, false);
    }
    foreach_rrdcalc_in_rrdhost_done(rc);
    dictionary_garbage_collect(host->rrdcalc_root_index);
}

void rrdcalc_child_disconnected(RRDHOST *host) {
    rrdcalc_delete_all(host);

    // We just deleted every alert of this host, so we must also ask health to re-create them when
    // the child comes back. We cannot rely on chart re-registration to do it: rrdset_conflict_callback()
    // only raises these flags when a chart definition actually changed, and a reconnecting child
    // normally re-sends identical definitions. RRDHOST_FLAG_INITIALIZED_HEALTH is never cleared, so
    // health_initialize_rrdhost() will not re-apply the prototypes either.
    //
    // This is the initialization flag rather than the label-recheck one on purpose: the alert lists
    // are now empty, so the incremental apply path is correct and cheaper than detach-and-reattach.
    //
    // The flags stay pending, inert, for as long as the child is away: our caller clears
    // RRDHOST_FLAG_COLLECTOR_ONLINE before calling us, so rrdhost_should_run_health() is already
    // false by the time we raise them and nothing consumes them (health.enabled = false, set later
    // in the same caller, only reinforces this). They are consumed on the first health pass after
    // the host is online again, whichever path brings it back.
    RRDSET *st;
    rrdset_foreach_read(st, host) {
        rrdset_flag_set(st, RRDSET_FLAG_PENDING_HEALTH_INITIALIZATION);
    }
    rrdset_foreach_done(st);

    // last: health_execute_delayed_initializations() consumes the host flag before walking the
    // charts, so raising it first would let a pass slip through with the chart flags still unset
    rrdhost_flag_set(host, RRDHOST_FLAG_PENDING_HEALTH_INITIALIZATION);
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

    memset(am, 0, sizeof(*am));
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

    memset(ac, 0, sizeof(*ac));
}

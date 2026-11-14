// SPDX-License-Identifier: GPL-3.0-or-later

#include "health.h"

// expression text, or null when there is none
static inline void health_json_member_add_expression_text(BUFFER *wb, const char *key, const char *value) {
    buffer_json_member_add_string(wb, key, (value && *value) ? value : NULL);
}

static inline void health_rrdcalc_values2json_nolock(
    RRDHOST *host, BUFFER *wb, RRDCALC *rc, const RRDCALC_RUNTIME_SNAPSHOT *snapshot) {
    (void)host;
    buffer_json_member_add_uint64(wb, "id", rc->id);
    buffer_json_member_add_double(wb, "value", snapshot->value);
    buffer_json_member_add_uint64(wb, "last_updated", (uint64_t)snapshot->last_updated);
    buffer_json_member_add_string(wb, "status", rrdcalc_status2string(snapshot->status));
}

static inline void health_rrdcalc2json_nolock(
    RRDHOST *host, BUFFER *wb, RRDCALC *rc, const RRDCALC_RUNTIME_SNAPSHOT *snapshot) {
    CLEAN_STRING *summary = NULL;
    CLEAN_STRING *info = NULL;
    rrdcalc_runtime_strings_acquire(rc, &summary, &info);

    char value_string[100 + 1];
    format_value_and_unit(value_string, 100, snapshot->value, rrdcalc_units(rc), -1);

    char hash_id[GUID_LEN + 1];
    uuid_unparse_lower(rc->config.hash_id, hash_id);

    buffer_json_member_add_uint64(wb, "id", rc->id);
    buffer_json_member_add_string(wb, "config_hash_id", hash_id);
    buffer_json_member_add_string(wb, "name", rrdcalc_name(rc));
    buffer_json_member_add_string(wb, "chart", rrdcalc_chart_name(rc));
    buffer_json_member_add_string(wb, "class", rc->config.classification ? rrdcalc_classification(rc) : "Unknown");
    buffer_json_member_add_string(wb, "component", rc->config.component ? rrdcalc_component(rc) : "Unknown");
    buffer_json_member_add_string(wb, "type", rc->config.type ? rrdcalc_type(rc) : "Unknown");
    buffer_json_member_add_boolean(wb, "active", rc->rrdset != NULL);
    buffer_json_member_add_boolean(wb, "disabled", snapshot->run_flags & RRDCALC_FLAG_DISABLED);
    buffer_json_member_add_boolean(wb, "silenced", snapshot->run_flags & RRDCALC_FLAG_SILENCED);
    buffer_json_member_add_string(wb, "exec",
        rc->config.exec ? rrdcalc_exec(rc) : string2str(host->health.default_exec));
    buffer_json_member_add_string(wb, "recipient",
        rc->config.recipient ? rrdcalc_recipient(rc) : string2str(host->health.default_recipient));
    buffer_json_member_add_string(wb, "source", rrdcalc_source(rc));
    buffer_json_member_add_string(wb, "units", rrdcalc_units(rc));
    buffer_json_member_add_string(wb, "summary", string2str(summary));
    buffer_json_member_add_string(wb, "info", string2str(info));
    buffer_json_member_add_string(wb, "status", rrdcalc_status2string(snapshot->status));
    buffer_json_member_add_uint64(wb, "last_status_change", (uint64_t)snapshot->last_status_change);
    buffer_json_member_add_uint64(wb, "last_updated", (uint64_t)snapshot->last_updated);
    buffer_json_member_add_uint64(wb, "next_update", (uint64_t)snapshot->next_update);
    buffer_json_member_add_int64(wb, "update_every", rc->config.update_every);
    buffer_json_member_add_int64(wb, "delay_up_duration", rc->config.delay_up_duration);
    buffer_json_member_add_int64(wb, "delay_down_duration", rc->config.delay_down_duration);
    buffer_json_member_add_int64(wb, "delay_max_duration", rc->config.delay_max_duration);
    // "%f" keeps it a JSON double (1.000000), as it has always been
    buffer_json_member_add_key_only(wb, "delay_multiplier");
    buffer_sprintf(wb, "%f", rc->config.delay_multiplier);
    buffer_json_member_add_int64(wb, "delay", snapshot->delay_last);
    buffer_json_member_add_uint64(wb, "delay_up_to_timestamp", (uint64_t)snapshot->delay_up_to_timestamp);

    // these three have always been strings
    buffer_json_member_add_sprintf(wb, "warn_repeat_every", "%u", rc->config.warn_repeat_every);
    buffer_json_member_add_sprintf(wb, "crit_repeat_every", "%u", rc->config.crit_repeat_every);
    buffer_json_member_add_string(wb, "value_string", value_string);
    buffer_json_member_add_sprintf(wb, "last_repeat", "%lu", (unsigned long)snapshot->last_repeat);

    buffer_json_member_add_uint64(wb, "times_repeat", snapshot->times_repeat);

    if(unlikely(rc->config.alert_action_options & ALERT_ACTION_OPTION_NO_CLEAR_NOTIFICATION))
        buffer_json_member_add_boolean(wb, "no_clear_notification", true);

    if(RRDCALC_HAS_DB_LOOKUP(rc)) {
        if(rc->config.dimensions)
            health_json_member_add_expression_text(wb, "lookup_dimensions", rrdcalc_dimensions(rc));

        buffer_json_member_add_uint64(wb, "db_after", (uint64_t)snapshot->db_after);
        buffer_json_member_add_uint64(wb, "db_before", (uint64_t)snapshot->db_before);
        buffer_json_member_add_string(wb, "lookup_method", time_grouping_id2txt(rc->config.time_group));
        buffer_json_member_add_int64(wb, "lookup_after", rc->config.after);
        buffer_json_member_add_int64(wb, "lookup_before", rc->config.before);

        // option names are fixed literals that need no escaping
        buffer_json_member_add_key_only(wb, "lookup_options");
        buffer_fast_strcat(wb, "\"", 1);
        rrdr_options_to_buffer(wb, rc->config.options);
        buffer_fast_strcat(wb, "\"", 1);
    }

    if(rc->config.calculation) {
        health_json_member_add_expression_text(wb, "calc", expression_source(rc->config.calculation));
        health_json_member_add_expression_text(wb, "calc_parsed", expression_parsed_as(rc->config.calculation));
    }

    if(rc->config.warning) {
        health_json_member_add_expression_text(wb, "warn", expression_source(rc->config.warning));
        health_json_member_add_expression_text(wb, "warn_parsed", expression_parsed_as(rc->config.warning));
    }

    if(rc->config.critical) {
        health_json_member_add_expression_text(wb, "crit", expression_source(rc->config.critical));
        health_json_member_add_expression_text(wb, "crit_parsed", expression_parsed_as(rc->config.critical));
    }

    buffer_json_member_add_double(wb, "green", NAN);
    buffer_json_member_add_double(wb, "red", NAN);
    buffer_json_member_add_double(wb, "value", snapshot->value);
}

void health_aggregate_alarms(RRDHOST *host, BUFFER *wb, BUFFER* contexts, RRDCALC_STATUS status) {
    RRDCALC *rc;
    int numberOfAlarms = 0;
    char *tok = NULL;
    char *p = NULL;

    if (contexts) {
        p = (char*)buffer_tostring(contexts);
        while(p && *p && (tok = strsep_skip_consecutive_separators(&p, ", |"))) {
            if(!*tok) continue;

            STRING *tok_string = string_strdupz(tok);

            foreach_rrdcalc_in_rrdhost_read(host, rc) {
                RRDSET *st = rrdcalc_rrdset_read_lock(rc);
                if(unlikely(!st))
                    continue;
                if(unlikely(!st->last_collected_time.tv_sec)) {
                    rrdcalc_rrdset_read_unlock(st);
                    continue;
                }
                if (unlikely(!rrdset_is_available_for_exporting_and_alarms(st))) {
                    rrdcalc_rrdset_read_unlock(st);
                    continue;
                }
                if(unlikely(st->context == tok_string)) {
                    RRDCALC_RUNTIME_SNAPSHOT snapshot;
                    rrdcalc_runtime_snapshot_get(rc, &snapshot);
                    if(unlikely((status == RRDCALC_STATUS_RAISED) ?
                                    (snapshot.status >= RRDCALC_STATUS_WARNING) : snapshot.status == status))
                        numberOfAlarms++;
                }
                rrdcalc_rrdset_read_unlock(st);
            }
            foreach_rrdcalc_in_rrdhost_done(rc);

            string_freez(tok_string);
        }
    }
    else {
        foreach_rrdcalc_in_rrdhost_read(host, rc) {
            RRDSET *st = rrdcalc_rrdset_read_lock(rc);
            if(unlikely(!st))
                continue;
            if(unlikely(!st->last_collected_time.tv_sec)) {
                rrdcalc_rrdset_read_unlock(st);
                continue;
            }
            if (unlikely(!rrdset_is_available_for_exporting_and_alarms(st))) {
                rrdcalc_rrdset_read_unlock(st);
                continue;
            }
            RRDCALC_RUNTIME_SNAPSHOT snapshot;
            rrdcalc_runtime_snapshot_get(rc, &snapshot);
            if(unlikely((status == RRDCALC_STATUS_RAISED) ?
                            (snapshot.status >= RRDCALC_STATUS_WARNING) : snapshot.status == status))
                numberOfAlarms++;
            rrdcalc_rrdset_read_unlock(st);
        }
        foreach_rrdcalc_in_rrdhost_done(rc);
    }

    buffer_sprintf(wb, "%d", numberOfAlarms);
}

// one "<chart>.<alert>" object per alert, filled by fp
static void health_alarms2json_fill_alarms(
    RRDHOST *host,
    BUFFER *wb,
    int all,
    void (*fp)(RRDHOST *, BUFFER *, RRDCALC *, const RRDCALC_RUNTIME_SNAPSHOT *)) {
    CLEAN_BUFFER *key = buffer_create(0, NULL);

    buffer_json_member_add_object(wb, "alarms");

    RRDCALC *rc;
    foreach_rrdcalc_in_rrdhost_read(host, rc) {
        RRDSET *st = rrdcalc_rrdset_read_lock(rc);
        if(unlikely(!st))
            continue;
        if(unlikely(!st->last_collected_time.tv_sec)) {
            rrdcalc_rrdset_read_unlock(st);
            continue;
        }

        if (unlikely(!rrdset_is_available_for_exporting_and_alarms(st))) {
            rrdcalc_rrdset_read_unlock(st);
            continue;
        }

        RRDCALC_RUNTIME_SNAPSHOT snapshot;
        rrdcalc_runtime_snapshot_get(rc, &snapshot);

        if(likely(!all && !(snapshot.status == RRDCALC_STATUS_WARNING || snapshot.status == RRDCALC_STATUS_CRITICAL))) {
            rrdcalc_rrdset_read_unlock(st);
            continue;
        }

        buffer_flush(key);
        buffer_sprintf(key, "%s.%s", rrdcalc_chart_name(rc), rrdcalc_name(rc));

        buffer_json_member_add_object(wb, buffer_tostring(key));
        fp(host, wb, rc, &snapshot);
        buffer_json_object_close(wb);

        rrdcalc_rrdset_read_unlock(st);
    }
    foreach_rrdcalc_in_rrdhost_done(rc);

    buffer_json_object_close(wb);
}

void health_alarms2json(RRDHOST *host, BUFFER *wb, int all) {
    rw_spinlock_read_lock(&host->health_log.spinlock);
    uint32_t latest_alarm_log_unique_id = (host->health_log.next_log_id > 0) ? (host->health_log.next_log_id - 1) : 0;
    rw_spinlock_read_unlock(&host->health_log.spinlock);

    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
    buffer_json_member_add_string(wb, "hostname", rrdhost_hostname(host));
    buffer_json_member_add_uint64(wb, "latest_alarm_log_unique_id", latest_alarm_log_unique_id);
    buffer_json_member_add_boolean(wb, "status", host->health.enabled);
    buffer_json_member_add_uint64(wb, "now", (uint64_t)now_realtime_sec());

    health_alarms2json_fill_alarms(host, wb, all, health_rrdcalc2json_nolock);

    buffer_json_finalize(wb);
}

void health_alarms_values2json(RRDHOST *host, BUFFER *wb, int all) {
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
    buffer_json_member_add_string(wb, "hostname", rrdhost_hostname(host));

    health_alarms2json_fill_alarms(host, wb, all, health_rrdcalc_values2json_nolock);

    buffer_json_finalize(wb);
}

void health_entry_flags_to_json_array(BUFFER *wb, const char *key, HEALTH_ENTRY_FLAGS flags) {
    buffer_json_member_add_array(wb, key);

    if(flags & HEALTH_ENTRY_FLAG_PROCESSED)
        buffer_json_add_array_item_string(wb, "PROCESSED");
    if(flags & HEALTH_ENTRY_FLAG_UPDATED)
        buffer_json_add_array_item_string(wb, "UPDATED");
    if(flags & HEALTH_ENTRY_FLAG_EXEC_RUN)
        buffer_json_add_array_item_string(wb, "EXEC_RUN");
    if(flags & HEALTH_ENTRY_FLAG_EXEC_FAILED)
        buffer_json_add_array_item_string(wb, "EXEC_FAILED");
    if(flags & HEALTH_ENTRY_FLAG_SILENCED)
        buffer_json_add_array_item_string(wb, "SILENCED");
    if(flags & HEALTH_ENTRY_RUN_ONCE)
        buffer_json_add_array_item_string(wb, "RUN_ONCE");
    if(flags & HEALTH_ENTRY_FLAG_EXEC_IN_PROGRESS)
        buffer_json_add_array_item_string(wb, "EXEC_IN_PROGRESS");
    if(flags & HEALTH_ENTRY_FLAG_IS_REPEATING)
        buffer_json_add_array_item_string(wb, "RECURRING");
    if(flags & HEALTH_ENTRY_FLAG_SAVED)
        buffer_json_add_array_item_string(wb, "SAVED");
    if(flags & HEALTH_ENTRY_FLAG_ACLK_QUEUED)
        buffer_json_add_array_item_string(wb, "ACLK_QUEUED");
    if(flags & HEALTH_ENTRY_FLAG_NO_CLEAR_NOTIFICATION)
        buffer_json_add_array_item_string(wb, "NO_CLEAR_NOTIFICATION");

    buffer_json_array_close(wb);
}

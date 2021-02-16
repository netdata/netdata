// SPDX-License-Identifier: GPL-3.0-or-later

#include "health.h"
#include "libnetdata/libnetdata.h"
#include <stdbool.h>

#define BOOL_STR(x)         (x) ? "true" : "false";
#define DEFAULT_STR(x, y)   (x) ? (x) : (y);

static void
health_kv_html_escape(BUFFER *wb, const char *key, const char *value,
               const char *prefix, const char *suffix)
{
    if (!value || !*value) {
        buffer_sprintf(wb, "%s\"%s\": null%s", prefix, key, suffix);
        return;
    }

    if (utf8_check(value)) {
        error("Found malformed utf8 string for key %s", key);
        value = "malformed_utf8_string";
    }

    buffer_sprintf(wb, "%s\"%s\": \"", prefix, key);
    buffer_strcat_htmlescape(wb, value);
    buffer_strcat(wb, "\"");
    buffer_strcat(wb, suffix);
}

static void
health_kv_str(BUFFER *wb, const char *key, const char *value,
       const char *prefix, const char *suffix) {
    if (utf8_check(value)) {
        error("Found malformed utf8 string for key %s", key);
        value = "malformed_utf8_string";
    }

    buffer_sprintf(wb, "%s\"%s\": \"", prefix, key);
    buffer_strcat(wb, value);
    buffer_strcat(wb, "\"");
    buffer_strcat(wb, suffix);
}

static void
health_kv_lu(BUFFER *wb, const char *key, long unsigned value,
      const char *prefix, const char *suffix) {
    buffer_sprintf(wb, "%s\"%s\": %lu%s", prefix, key, value, suffix);
}

static void
health_kv_ld(BUFFER *wb, const char *key, long int value,
      const char *prefix, const char *suffix) {
    buffer_sprintf(wb, "%s\"%s\": %ld%s", prefix, key, value, suffix);
}

static void
health_kv_float(BUFFER *wb, const char *key, float value,
         const char *prefix, const char *suffix) {
    buffer_sprintf(wb, "%s\"%s\": %f%s", prefix, key, value, suffix);
}

static void
health_kv_bool(BUFFER *wb, const char *key, bool value,
        const char *prefix, const char *suffix) {
    const char *s = value ? "true" : "false";
    buffer_sprintf(wb, "%s\"%s\": %s%s", prefix, key, s, suffix);
}

void
health_alarm_entry2json_nolock(BUFFER *wb, ALARM_ENTRY *ae, RRDHOST *host) {
    const char *processed = BOOL_STR(ae->flags & HEALTH_ENTRY_FLAG_PROCESSED);
    const char *updated = BOOL_STR(ae->flags & HEALTH_ENTRY_FLAG_UPDATED);

    const char *exec_failed = BOOL_STR(ae->flags & HEALTH_ENTRY_FLAG_EXEC_FAILED);
    const char *exec = DEFAULT_STR(ae->exec, host->health_default_exec);

    const char *recipient = DEFAULT_STR(ae->recipient, host->health_default_recipient);
    const char *units = DEFAULT_STR(ae->units, "");

    const char *status = rrdcalc_status2string(ae->new_status);
    const char *old_status = rrdcalc_status2string(ae->old_status);

    const char *silenced = BOOL_STR(ae->flags & HEALTH_ENTRY_FLAG_SILENCED);
    const char *info = DEFAULT_STR(ae->info, "");

    buffer_strcat(wb, "\n\t{\n");

    const char *prefix = "\t\t", *suffix = ",\n";
    health_kv_str(wb, "hostname", host->hostname, prefix, suffix);
    health_kv_lu(wb, "unique_id", ae->unique_id, prefix, suffix);
    health_kv_lu(wb, "alarm_id", ae->alarm_id, prefix, suffix);
    health_kv_lu(wb, "alarm_event_id", ae->alarm_event_id, prefix, suffix);
    health_kv_str(wb, "name", ae->name, prefix, suffix);
    health_kv_str(wb, "chart", ae->chart, prefix, suffix);
    health_kv_str(wb, "family", ae->family, prefix, suffix);
    health_kv_str(wb, "processed", processed, prefix, suffix);
    health_kv_str(wb, "updated", updated, prefix, suffix);
    health_kv_lu(wb, "exec_run", (unsigned long) ae->exec_run_timestamp, prefix, suffix);
    health_kv_str(wb, "exec_failed", exec_failed, prefix, suffix);
    health_kv_str(wb, "exec", exec, prefix, suffix);
    health_kv_str(wb, "recipient", recipient, prefix, suffix);
    health_kv_ld(wb, "exec_code", ae->exec_code, prefix, suffix);
    health_kv_str(wb, "source", ae->source, prefix, suffix);
    health_kv_str(wb, "units", units, prefix, suffix);
    health_kv_lu(wb, "when", (unsigned long) ae->when, prefix, suffix);
    health_kv_lu(wb, "duration", (unsigned long) ae->duration, prefix, suffix);
    health_kv_lu(wb, "non_clear_duration", (unsigned long) ae->non_clear_duration, prefix, suffix);
    health_kv_str(wb, "status", status, prefix, suffix);
    health_kv_str(wb, "old_status", old_status, prefix, suffix);
    health_kv_ld(wb, "delay", ae->delay, prefix, suffix);
    health_kv_lu(wb, "delay_up_to_timestamp", (unsigned long) ae->delay_up_to_timestamp, prefix, suffix);
    health_kv_lu(wb, "updated_by_id", ae->updated_by_id, prefix, suffix);
    health_kv_lu(wb, "updates_id", ae->updates_id, prefix, suffix);
    health_kv_str(wb, "value_string", ae->new_value_string, prefix, suffix);
    health_kv_str(wb, "old_value_string", ae->old_value_string, prefix, suffix);
    health_kv_lu(wb, "last_repeat", (unsigned long) ae->last_repeat, prefix, suffix);
    health_kv_str(wb, "silenced", silenced, prefix, suffix);
    health_kv_html_escape(wb, "info", info, prefix, suffix);

    if (ae->flags & HEALTH_ENTRY_FLAG_NO_CLEAR_NOTIFICATION)
        buffer_strcat(wb, "\t\t\"no_clear_notification\": true,\n");

    buffer_strcat(wb, "\t\t\"value\": ");
    buffer_rrd_value(wb, ae->new_value);
    buffer_strcat(wb, ",\n");

    buffer_strcat(wb, "\t\t\"old_value\": ");
    buffer_rrd_value(wb, ae->old_value);
    buffer_strcat(wb, "\n");

    buffer_strcat(wb, "\t}");
}

void health_alarm_log2json(RRDHOST *host, BUFFER *wb, uint32_t after) {
    netdata_rwlock_rdlock(&host->health_log.alarm_log_rwlock);

    buffer_strcat(wb, "[");

    unsigned int max = host->health_log.max;
    unsigned int count = 0;
    ALARM_ENTRY *ae;
    for(ae = host->health_log.alarms; ae && count < max ; count++, ae = ae->next) {
        if(ae->unique_id > after) {
            if(likely(count)) buffer_strcat(wb, ",");
            health_alarm_entry2json_nolock(wb, ae, host);
        }
    }

    buffer_strcat(wb, "\n]\n");

    netdata_rwlock_unlock(&host->health_log.alarm_log_rwlock);
}

static inline void health_rrdcalc_values2json_nolock(RRDHOST *host, BUFFER *wb, RRDCALC *rc) {
    (void) host;

    buffer_sprintf(wb, "\t\t\"%s.%s\": {\n", rc->chart, rc->name);

    const char *prefix="\t\t\t", *suffix = ",\n";
    health_kv_lu(wb, "id", rc->id, prefix, suffix);

    buffer_strcat(wb, "\t\t\t\"value\": ");
    buffer_rrd_value(wb, rc->value);
    buffer_strcat(wb, ",\n");

    health_kv_str(wb, "status", rrdcalc_status2string(rc->status), prefix, "\n");

    buffer_strcat(wb, "\t\t}");
}

static void
health_rrdcalc2json_nolock(RRDHOST *host, BUFFER *wb, RRDCALC *rc) {
    const char *family = (rc->rrdset && rc->rrdset->family) ? rc->rrdset->family : "";

    const char *exec = DEFAULT_STR(rc->exec, host->health_default_exec);
    const char *recipient = DEFAULT_STR(rc->recipient, host->health_default_recipient);

    const char *units = DEFAULT_STR(rc->units, "");
    const char *info = DEFAULT_STR(rc->info, "");

    char value_string[100 + 1];
    format_value_and_unit(value_string, 100, rc->value, rc->units, -1);

    buffer_sprintf(wb, "\t\t\"%s.%s\": {\n", rc->chart, rc->name);

    const char *prefix = "\t\t\t", *suffix = ",\n";
    health_kv_lu(wb, "id", rc->id, prefix, suffix);
    health_kv_str(wb, "name", rc->name, prefix, suffix);
    health_kv_str(wb, "chart", rc->chart, prefix, suffix);
    health_kv_str(wb, "family", family, prefix, suffix);
    health_kv_bool(wb, "active", rc->rrdset, prefix, suffix);
    health_kv_bool(wb, "disabled", rc->rrdcalc_flags & RRDCALC_FLAG_DISABLED, prefix, suffix);
    health_kv_bool(wb, "silenced", rc->rrdcalc_flags & RRDCALC_FLAG_SILENCED, prefix, suffix);
    health_kv_str(wb, "exec", exec, prefix, suffix);
    health_kv_str(wb, "recipient", recipient, prefix, suffix);
    health_kv_str(wb, "source", rc->source, prefix, suffix);
    health_kv_str(wb, "units", units, prefix, suffix);
    health_kv_str(wb, "info", info, prefix, suffix);
    health_kv_str(wb, "status", rrdcalc_status2string(rc->status), prefix, suffix);
    health_kv_lu(wb, "last_status_change", (unsigned long) rc->last_status_change, prefix, suffix);
    health_kv_lu(wb, "last_updated", (unsigned long) rc->last_updated, prefix, suffix);
    health_kv_lu(wb, "next_update", (unsigned long) rc->next_update, prefix, suffix);
    health_kv_ld(wb, "update_every", rc->update_every, prefix, suffix);
    health_kv_ld(wb, "delay_up_duration", rc->delay_up_duration, prefix, suffix);
    health_kv_ld(wb, "delay_down_duration", rc->delay_down_duration, prefix, suffix);
    health_kv_ld(wb, "delay_max_duration", rc->delay_down_duration, prefix, suffix);
    health_kv_float(wb, "delay_multiplier", rc->delay_multiplier, prefix, suffix);
    health_kv_ld(wb, "delay", rc->delay_last, prefix, suffix);
    health_kv_lu(wb, "delay_up_to_timestamp", (unsigned long) rc->delay_up_to_timestamp, prefix, suffix);
    health_kv_lu(wb, "warn_repeat_every", rc->warn_repeat_every, prefix, suffix);
    health_kv_lu(wb, "crit_repeat_every", rc->crit_repeat_every, prefix, suffix);
    health_kv_str(wb, "value_string", value_string, prefix, suffix);
    health_kv_lu(wb, "last_repeat", (unsigned long) rc->last_repeat, prefix, suffix);

    if (rc->options & RRDCALC_FLAG_NO_CLEAR_NOTIFICATION)
        buffer_strcat(wb, "\t\t\t\"no_clear_notification\": true,\n");

    if (RRDCALC_HAS_DB_LOOKUP(rc)) {
        if(rc->dimensions && *rc->dimensions)
            health_kv_html_escape(wb, "lookup_dimensions", rc->dimensions, prefix, suffix);

        health_kv_lu(wb, "db_after", (unsigned long) rc->db_after, prefix, suffix);
        health_kv_lu(wb, "db_before", (unsigned long) rc->db_before, prefix, suffix);
        health_kv_str(wb, "lookup_method", group_method2string(rc->group), prefix, suffix);
        health_kv_ld(wb, "lookup_after", rc->db_after, prefix, suffix);
        health_kv_ld(wb, "lookup_before", rc->db_before, prefix, suffix);

        buffer_strcat(wb, "\t\t\t\"lookup_options\": \"");
        buffer_data_options2string(wb, rc->options);
        buffer_strcat(wb, "\",\n");
    }

    if(rc->calculation) {
        health_kv_html_escape(wb, "calc", rc->calculation->source, prefix, suffix);
        health_kv_html_escape(wb, "calc_parsed", rc->calculation->parsed_as, prefix, suffix);
    }

    if(rc->warning) {
        health_kv_html_escape(wb, "warn", rc->warning->source, prefix, suffix);
        health_kv_html_escape(wb, "warn_parsed", rc->warning->parsed_as, prefix, suffix);
    }

    if(rc->critical) {
        health_kv_html_escape(wb, "crit", rc->critical->source, prefix, suffix);
        health_kv_html_escape(wb, "crit_parsed", rc->critical->parsed_as, prefix, suffix);
    }

    buffer_strcat(wb, "\t\t\t\"green\": ");
    buffer_rrd_value(wb, rc->green);
    buffer_strcat(wb, ",\n");

    buffer_strcat(wb, "\t\t\t\"red\": ");
    buffer_rrd_value(wb, rc->red);
    buffer_strcat(wb, ",\n");

    buffer_strcat(wb, "\t\t\t\"value\": ");
    buffer_rrd_value(wb, rc->value);
    buffer_strcat(wb, "\n");

    buffer_strcat(wb, "\t\t}");
}

void health_aggregate_alarms(RRDHOST *host, BUFFER *wb, BUFFER* contexts, RRDCALC_STATUS status) {
    RRDCALC *rc;
    int numberOfAlarms = 0;
    char *tok = NULL;
    char *p = NULL;

    rrdhost_rdlock(host);

    if (contexts) {
        p = (char*)buffer_tostring(contexts);
        while(p && *p && (tok = mystrsep(&p, ", |"))) {
            if(!*tok) continue;

            for(rc = host->alarms; rc ; rc = rc->next) {
                if(unlikely(!rc->rrdset || !rc->rrdset->last_collected_time.tv_sec))
                    continue;
                if(unlikely(rc->rrdset && rc->rrdset->hash_context == simple_hash(tok)
                            && !strcmp(rc->rrdset->context, tok)
                            && ((status==RRDCALC_STATUS_RAISED)?(rc->status >= RRDCALC_STATUS_WARNING):rc->status == status)))
                    numberOfAlarms++;
            }
        }
    }
    else {
        for(rc = host->alarms; rc ; rc = rc->next) {
            if(unlikely(!rc->rrdset || !rc->rrdset->last_collected_time.tv_sec))
                continue;

            if(unlikely((status==RRDCALC_STATUS_RAISED)?(rc->status >= RRDCALC_STATUS_WARNING):rc->status == status))
                numberOfAlarms++;
        }
    }

    buffer_sprintf(wb, "%d", numberOfAlarms);
    rrdhost_unlock(host);
}

static void health_alarms2json_fill_alarms(RRDHOST *host, BUFFER *wb, int all, void (*fp)(RRDHOST *, BUFFER *, RRDCALC *)) {
    RRDCALC *rc;
    int i;
    for(i = 0, rc = host->alarms; rc ; rc = rc->next) {
        if(unlikely(!rc->rrdset || !rc->rrdset->last_collected_time.tv_sec))
            continue;

        if(likely(!all && !(rc->status == RRDCALC_STATUS_WARNING || rc->status == RRDCALC_STATUS_CRITICAL)))
            continue;

        if(likely(i)) buffer_strcat(wb, ",\n");
        fp(host, wb, rc);
        i++;
    }
}

void health_alarms2json(RRDHOST *host, BUFFER *wb, int all) {
    rrdhost_rdlock(host);

    unsigned next_log_id = host->health_log.next_log_id;

    const char *prefix = "\t", *suffix = ",\n";
    buffer_strcat(wb, "{\n");

    health_kv_str(wb, "hostname", host->hostname, prefix, suffix);
    health_kv_lu(wb, "latest_alarm_log_unique_id", next_log_id ? next_log_id - 1 : 0, prefix, suffix);
    health_kv_bool(wb, "status", host->health_enabled, prefix, suffix);
    health_kv_lu(wb, "now", (unsigned long) now_realtime_sec(), prefix, suffix);

    buffer_strcat(wb, "\t\"alarms\": {\n");
    health_alarms2json_fill_alarms(host, wb, all,  health_rrdcalc2json_nolock);
    buffer_strcat(wb, "\n\t}\n}\n");

    rrdhost_unlock(host);
}

void health_alarms_values2json(RRDHOST *host, BUFFER *wb, int all) {
    rrdhost_rdlock(host);

    health_kv_str(wb, "hostname", host->hostname, "{\n\t", ",\n");

    buffer_strcat(wb, "\n\t\"alarms\": {\n");
    health_alarms2json_fill_alarms(host, wb, all,  health_rrdcalc_values2json_nolock);
    buffer_strcat(wb, "\n\t}\n}\n");

    rrdhost_unlock(host);
}

void health_active_log_alarms_2json(RRDHOST *host, BUFFER *wb) {
    netdata_rwlock_rdlock(&host->health_log.alarm_log_rwlock);

    buffer_sprintf(wb, "[\n");

    unsigned int max = host->health_log.max;
    unsigned int count = 0;
    ALARM_ENTRY *ae;
    for(ae = host->health_log.alarms; ae && count < max ; ae = ae->next) {
        if (!ae->updated_by_id &&
            ((ae->new_status == RRDCALC_STATUS_WARNING || ae->new_status == RRDCALC_STATUS_CRITICAL) ||
             ((ae->old_status == RRDCALC_STATUS_WARNING || ae->old_status == RRDCALC_STATUS_CRITICAL) &&
              ae->new_status == RRDCALC_STATUS_REMOVED))) {
            if (likely(count))
                buffer_strcat(wb, ",");
            health_alarm_entry2json_nolock(wb, ae, host);
            count++;
        }
    }
    buffer_strcat(wb, "]");

    netdata_rwlock_unlock(&host->health_log.alarm_log_rwlock);
}

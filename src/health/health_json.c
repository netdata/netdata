// SPDX-License-Identifier: GPL-3.0-or-later

#include "health.h"

void health_string2json(BUFFER *wb, const char *prefix, const char *label, const char *value, const char *suffix) {
    if(value && *value) {
        buffer_sprintf(wb, "%s\"%s\":\"", prefix, label);
        buffer_strcat_htmlescape(wb, value);
        buffer_strcat(wb, "\"");
        buffer_strcat(wb, suffix);
    }
    else
        buffer_sprintf(wb, "%s\"%s\":null%s", prefix, label, suffix);
}

static inline void health_rrdcalc_values2json_nolock(RRDHOST *host, BUFFER *wb, RRDCALC *rc) {
    (void)host;
    buffer_sprintf(wb,
                   "\t\t\"%s.%s\": {\n"
                   "\t\t\t\"id\": %lu,\n"
                   , rrdcalc_chart_name(rc), rrdcalc_name(rc)
                   , (unsigned long)rc->id);

    buffer_strcat(wb, "\t\t\t\"value\":");
    buffer_print_netdata_double(wb, rc->value);
    buffer_strcat(wb, ",\n");

    buffer_strcat(wb, "\t\t\t\"last_updated\":");
    buffer_sprintf(wb, "%lu", (unsigned long)rc->last_updated);
    buffer_strcat(wb, ",\n");

    buffer_sprintf(wb,
                   "\t\t\t\"status\": \"%s\"\n"
                   , rrdcalc_status2string(rc->status));

    buffer_strcat(wb, "\t\t}");
}

static inline void health_rrdcalc2json_nolock(RRDHOST *host, BUFFER *wb, RRDCALC *rc) {
    char value_string[100 + 1];
    format_value_and_unit(value_string, 100, rc->value, rrdcalc_units(rc), -1);

    char hash_id[GUID_LEN + 1];
    uuid_unparse_lower(rc->config.hash_id, hash_id);

    buffer_sprintf(wb,
            "\t\t\"%s.%s\": {\n"
                    "\t\t\t\"id\": %lu,\n"
                    "\t\t\t\"config_hash_id\": \"%s\",\n"
                    "\t\t\t\"name\": \"%s\",\n"
                    "\t\t\t\"chart\": \"%s\",\n"
                    "\t\t\t\"class\": \"%s\",\n"
                    "\t\t\t\"component\": \"%s\",\n"
                    "\t\t\t\"type\": \"%s\",\n"
                    "\t\t\t\"active\": %s,\n"
                    "\t\t\t\"disabled\": %s,\n"
                    "\t\t\t\"silenced\": %s,\n"
                    "\t\t\t\"exec\": \"%s\",\n"
                    "\t\t\t\"recipient\": \"%s\",\n"
                    "\t\t\t\"source\": \"%s\",\n"
                    "\t\t\t\"units\": \"%s\",\n"
                    "\t\t\t\"summary\": \"%s\",\n"
                    "\t\t\t\"info\": \"%s\",\n"
                    "\t\t\t\"status\": \"%s\",\n"
                    "\t\t\t\"last_status_change\": %lu,\n"
                    "\t\t\t\"last_updated\": %lu,\n"
                    "\t\t\t\"next_update\": %lu,\n"
                    "\t\t\t\"update_every\": %d,\n"
                    "\t\t\t\"delay_up_duration\": %d,\n"
                    "\t\t\t\"delay_down_duration\": %d,\n"
                    "\t\t\t\"delay_max_duration\": %d,\n"
                    "\t\t\t\"delay_multiplier\": %f,\n"
                    "\t\t\t\"delay\": %d,\n"
                    "\t\t\t\"delay_up_to_timestamp\": %lu,\n"
                    "\t\t\t\"warn_repeat_every\": \"%u\",\n"
                    "\t\t\t\"crit_repeat_every\": \"%u\",\n"
                    "\t\t\t\"value_string\": \"%s\",\n"
                    "\t\t\t\"last_repeat\": \"%lu\",\n"
                    "\t\t\t\"times_repeat\": %lu,\n"
                   , rrdcalc_chart_name(rc), rrdcalc_name(rc)
                   , (unsigned long)rc->id
                   , hash_id
                   , rrdcalc_name(rc)
                   , rrdcalc_chart_name(rc)
                   , rc->config.classification?rrdcalc_classification(rc):"Unknown"
                   , rc->config.component?rrdcalc_component(rc):"Unknown"
                   , rc->config.type?rrdcalc_type(rc):"Unknown"
                   , (rc->rrdset)?"true":"false"
                   , (rc->run_flags & RRDCALC_FLAG_DISABLED)?"true":"false"
                   , (rc->run_flags & RRDCALC_FLAG_SILENCED)?"true":"false"
                   , rc->config.exec?rrdcalc_exec(rc):string2str(host->health.default_exec)
                   , rc->config.recipient?rrdcalc_recipient(rc):string2str(host->health.default_recipient)
                   , rrdcalc_source(rc)
                   , rrdcalc_units(rc)
                   , string2str(rc->summary)
                   , string2str(rc->info)
                   , rrdcalc_status2string(rc->status)
                   , (unsigned long)rc->last_status_change
                   , (unsigned long)rc->last_updated
                   , (unsigned long)rc->next_update
                   , rc->config.update_every
                   , rc->config.delay_up_duration
                   , rc->config.delay_down_duration
                   , rc->config.delay_max_duration
                   , rc->config.delay_multiplier
                   , rc->delay_last
                   , (unsigned long)rc->delay_up_to_timestamp
                   , rc->config.warn_repeat_every
                   , rc->config.crit_repeat_every
                   , value_string
                   , (unsigned long)rc->last_repeat
                   , (unsigned long)rc->times_repeat
    );

    if(unlikely(rc->config.alert_action_options & ALERT_ACTION_OPTION_NO_CLEAR_NOTIFICATION)) {
        buffer_strcat(wb, "\t\t\t\"no_clear_notification\": true,\n");
    }

    if(RRDCALC_HAS_DB_LOOKUP(rc)) {
        if(rc->config.dimensions)
            health_string2json(wb, "\t\t\t", "lookup_dimensions", rrdcalc_dimensions(rc), ",\n");

        buffer_sprintf(wb,
                "\t\t\t\"db_after\": %lu,\n"
                        "\t\t\t\"db_before\": %lu,\n"
                        "\t\t\t\"lookup_method\": \"%s\",\n"
                        "\t\t\t\"lookup_after\": %d,\n"
                        "\t\t\t\"lookup_before\": %d,\n"
                        "\t\t\t\"lookup_options\": \"",
                       (unsigned long) rc->db_after,
                       (unsigned long) rc->db_before,
                       time_grouping_id2txt(rc->config.time_group),
                       rc->config.after,
                       rc->config.before
        );
        rrdr_options_to_buffer(wb, rc->config.options);
        buffer_strcat(wb, "\",\n");
    }

    if(rc->config.calculation) {
        health_string2json(wb, "\t\t\t", "calc", expression_source(rc->config.calculation), ",\n");
        health_string2json(wb, "\t\t\t", "calc_parsed", expression_parsed_as(rc->config.calculation), ",\n");
    }

    if(rc->config.warning) {
        health_string2json(wb, "\t\t\t", "warn", expression_source(rc->config.warning), ",\n");
        health_string2json(wb, "\t\t\t", "warn_parsed", expression_parsed_as(rc->config.warning), ",\n");
    }

    if(rc->config.critical) {
        health_string2json(wb, "\t\t\t", "crit", expression_source(rc->config.critical), ",\n");
        health_string2json(wb, "\t\t\t", "crit_parsed", expression_parsed_as(rc->config.critical), ",\n");
    }

    buffer_strcat(wb, "\t\t\t\"green\":");
    buffer_print_netdata_double(wb, NAN);
    buffer_strcat(wb, ",\n");

    buffer_strcat(wb, "\t\t\t\"red\":");
    buffer_print_netdata_double(wb, NAN);
    buffer_strcat(wb, ",\n");

    buffer_strcat(wb, "\t\t\t\"value\":");
    buffer_print_netdata_double(wb, rc->value);
    buffer_strcat(wb, "\n");

    buffer_strcat(wb, "\t\t}");
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
                if(unlikely(!rc->rrdset || !rc->rrdset->last_collected_time.tv_sec))
                    continue;
                if (unlikely(!rrdset_is_available_for_exporting_and_alarms(rc->rrdset)))
                    continue;
                if(unlikely(rc->rrdset
                             && rc->rrdset->context == tok_string
                             && ((status==RRDCALC_STATUS_RAISED)?(rc->status >= RRDCALC_STATUS_WARNING):rc->status == status)))
                    numberOfAlarms++;
            }
            foreach_rrdcalc_in_rrdhost_done(rc);

            string_freez(tok_string);
        }
    }
    else {
        foreach_rrdcalc_in_rrdhost_read(host, rc) {
            if(unlikely(!rc->rrdset || !rc->rrdset->last_collected_time.tv_sec))
                continue;
            if (unlikely(!rrdset_is_available_for_exporting_and_alarms(rc->rrdset)))
                continue;
            if(unlikely((status==RRDCALC_STATUS_RAISED)?(rc->status >= RRDCALC_STATUS_WARNING):rc->status == status))
                numberOfAlarms++;
        }
        foreach_rrdcalc_in_rrdhost_done(rc);
    }

    buffer_sprintf(wb, "%d", numberOfAlarms);
}

static void health_alarms2json_fill_alarms(RRDHOST *host, BUFFER *wb, int all, void (*fp)(RRDHOST *, BUFFER *, RRDCALC *)) {
    RRDCALC *rc;
    int i = 0;
    foreach_rrdcalc_in_rrdhost_read(host, rc) {
        if(unlikely(!rc->rrdset || !rc->rrdset->last_collected_time.tv_sec))
            continue;

        if (unlikely(!rrdset_is_available_for_exporting_and_alarms(rc->rrdset)))
            continue;

        if(likely(!all && !(rc->status == RRDCALC_STATUS_WARNING || rc->status == RRDCALC_STATUS_CRITICAL)))
            continue;

        if(likely(i)) buffer_strcat(wb, ",\n");
        fp(host, wb, rc);
        i++;
    }
    foreach_rrdcalc_in_rrdhost_done(rc);
}

void health_alarms2json(RRDHOST *host, BUFFER *wb, int all) {
    buffer_sprintf(wb, "{\n\t\"hostname\": \"%s\","
                    "\n\t\"latest_alarm_log_unique_id\": %u,"
                    "\n\t\"status\": %s,"
                    "\n\t\"now\": %lu,"
                    "\n\t\"alarms\": {\n",
                   rrdhost_hostname(host),
                   (host->health_log.next_log_id > 0)?(host->health_log.next_log_id - 1):0,
                   host->health.enabled ?"true":"false",
                   (unsigned long)now_realtime_sec());

    health_alarms2json_fill_alarms(host, wb, all,  health_rrdcalc2json_nolock);

    buffer_strcat(wb, "\n\t}\n}\n");
}

void health_alarms_values2json(RRDHOST *host, BUFFER *wb, int all) {
    buffer_sprintf(wb, "{\n\t\"hostname\": \"%s\","
                       "\n\t\"alarms\": {\n",
                   rrdhost_hostname(host));

    health_alarms2json_fill_alarms(host, wb, all,  health_rrdcalc_values2json_nolock);

    buffer_strcat(wb, "\n\t}\n}\n");
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

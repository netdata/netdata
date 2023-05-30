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
    uuid_unparse_lower(rc->config_hash_id, hash_id);

    buffer_sprintf(wb,
            "\t\t\"%s.%s\": {\n"
                    "\t\t\t\"id\": %lu,\n"
                    "\t\t\t\"config_hash_id\": \"%s\",\n"
                    "\t\t\t\"name\": \"%s\",\n"
                    "\t\t\t\"chart\": \"%s\",\n"
                    "\t\t\t\"family\": \"%s\",\n"
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
                   , (rc->rrdset)?rrdset_family(rc->rrdset):""
                   , rc->classification?rrdcalc_classification(rc):"Unknown"
                   , rc->component?rrdcalc_component(rc):"Unknown"
                   , rc->type?rrdcalc_type(rc):"Unknown"
                   , (rc->rrdset)?"true":"false"
                   , (rc->run_flags & RRDCALC_FLAG_DISABLED)?"true":"false"
                   , (rc->run_flags & RRDCALC_FLAG_SILENCED)?"true":"false"
                   , rc->exec?rrdcalc_exec(rc):string2str(host->health.health_default_exec)
                   , rc->recipient?rrdcalc_recipient(rc):string2str(host->health.health_default_recipient)
                   , rrdcalc_source(rc)
                   , rrdcalc_units(rc)
                   , rrdcalc_info(rc)
                   , rrdcalc_status2string(rc->status)
                   , (unsigned long)rc->last_status_change
                   , (unsigned long)rc->last_updated
                   , (unsigned long)rc->next_update
                   , rc->update_every
                   , rc->delay_up_duration
                   , rc->delay_down_duration
                   , rc->delay_max_duration
                   , rc->delay_multiplier
                   , rc->delay_last
                   , (unsigned long)rc->delay_up_to_timestamp
                   , rc->warn_repeat_every
                   , rc->crit_repeat_every
                   , value_string
                   , (unsigned long)rc->last_repeat
                   , (unsigned long)rc->times_repeat
    );

    if(unlikely(rc->options & RRDCALC_OPTION_NO_CLEAR_NOTIFICATION)) {
        buffer_strcat(wb, "\t\t\t\"no_clear_notification\": true,\n");
    }

    if(RRDCALC_HAS_DB_LOOKUP(rc)) {
        if(rc->dimensions)
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
                       time_grouping_method2string(rc->group),
                       rc->after,
                       rc->before
        );
        buffer_data_options2string(wb, rc->options);
        buffer_strcat(wb, "\",\n");
    }

    if(rc->calculation) {
        health_string2json(wb, "\t\t\t", "calc", rc->calculation->source, ",\n");
        health_string2json(wb, "\t\t\t", "calc_parsed", rc->calculation->parsed_as, ",\n");
    }

    if(rc->warning) {
        health_string2json(wb, "\t\t\t", "warn", rc->warning->source, ",\n");
        health_string2json(wb, "\t\t\t", "warn_parsed", rc->warning->parsed_as, ",\n");
    }

    if(rc->critical) {
        health_string2json(wb, "\t\t\t", "crit", rc->critical->source, ",\n");
        health_string2json(wb, "\t\t\t", "crit_parsed", rc->critical->parsed_as, ",\n");
    }

    buffer_strcat(wb, "\t\t\t\"green\":");
    buffer_print_netdata_double(wb, rc->green);
    buffer_strcat(wb, ",\n");

    buffer_strcat(wb, "\t\t\t\"red\":");
    buffer_print_netdata_double(wb, rc->red);
    buffer_strcat(wb, ",\n");

    buffer_strcat(wb, "\t\t\t\"value\":");
    buffer_print_netdata_double(wb, rc->value);
    buffer_strcat(wb, "\n");

    buffer_strcat(wb, "\t\t}");
}

//void health_rrdcalctemplate2json_nolock(BUFFER *wb, RRDCALCTEMPLATE *rt) {
//
//}

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
            host->health.health_enabled?"true":"false",
            (unsigned long)now_realtime_sec());

    health_alarms2json_fill_alarms(host, wb, all,  health_rrdcalc2json_nolock);

//    rrdhost_rdlock(host);
//    buffer_strcat(wb, "\n\t},\n\t\"templates\": {");
//    RRDCALCTEMPLATE *rt;
//    for(rt = host->templates; rt ; rt = rt->next)
//        health_rrdcalctemplate2json_nolock(wb, rt);
//    rrdhost_unlock(host);

    buffer_strcat(wb, "\n\t}\n}\n");
}

void health_alarms_values2json(RRDHOST *host, BUFFER *wb, int all) {
    buffer_sprintf(wb, "{\n\t\"hostname\": \"%s\","
                       "\n\t\"alarms\": {\n",
                   rrdhost_hostname(host));

    health_alarms2json_fill_alarms(host, wb, all,  health_rrdcalc_values2json_nolock);

    buffer_strcat(wb, "\n\t}\n}\n");
}


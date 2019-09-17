// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdset2json.h"

// generate JSON for the /api/v1/chart API call

void rrdset2json(RRDSET *st, BUFFER *wb, size_t *dimensions_count, size_t *memory_used) {
    rrdset_rdlock(st);

    time_t first_entry_t = rrdset_first_entry_t(st);
    time_t last_entry_t  = rrdset_last_entry_t(st);

    buffer_sprintf(wb,
            "\t\t{\n"
            "\t\t\t\"id\": \"%s\",\n"
            "\t\t\t\"name\": \"%s\",\n"
            "\t\t\t\"type\": \"%s\",\n"
            "\t\t\t\"family\": \"%s\",\n"
            "\t\t\t\"context\": \"%s\",\n"
            "\t\t\t\"title\": \"%s (%s)\",\n"
            "\t\t\t\"priority\": %ld,\n"
            "\t\t\t\"plugin\": \"%s\",\n"
            "\t\t\t\"module\": \"%s\",\n"
            "\t\t\t\"enabled\": %s,\n"
            "\t\t\t\"units\": \"%s\",\n"
            "\t\t\t\"data_url\": \"/api/v1/data?chart=%s\",\n"
            "\t\t\t\"chart_type\": \"%s\",\n"
            "\t\t\t\"duration\": %ld,\n"
            "\t\t\t\"first_entry\": %ld,\n"
            "\t\t\t\"last_entry\": %ld,\n"
            "\t\t\t\"update_every\": %d,\n"
            "\t\t\t\"dimensions\": {\n"
                   , st->id
                   , st->name
                   , st->type
                   , st->family
                   , st->context
                   , st->title, st->name
                   , st->priority
                   , st->plugin_name?st->plugin_name:""
                   , st->module_name?st->module_name:""
                   , rrdset_flag_check(st, RRDSET_FLAG_ENABLED)?"true":"false"
                   , st->units
                   , st->name
                   , rrdset_type_name(st->chart_type)
                   , last_entry_t - first_entry_t + st->update_every//st->entries * st->update_every
                   , first_entry_t//rrdset_first_entry_t(st)
                   , last_entry_t//rrdset_last_entry_t(st)
                   , st->update_every
    );

    unsigned long memory = st->memsize;

    size_t dimensions = 0;
    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        if(rrddim_flag_check(rd, RRDDIM_FLAG_HIDDEN) || rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE)) continue;

        memory += rd->memsize;

        buffer_sprintf(
                wb
                , "%s"
                  "\t\t\t\t\"%s\": { \"name\": \"%s\" }"
                , dimensions ? ",\n" : ""
                , rd->id
                , rd->name
        );

        dimensions++;
    }

    if(dimensions_count) *dimensions_count += dimensions;
    if(memory_used) *memory_used += memory;

    buffer_sprintf(wb, "\n\t\t\t},\n\t\t\t\"chart_variables\": ");
    health_api_v1_chart_custom_variables2json(st, wb);

    buffer_strcat(wb, ",\n\t\t\t\"green\": ");
    buffer_rrd_value(wb, st->green);
    buffer_strcat(wb, ",\n\t\t\t\"red\": ");
    buffer_rrd_value(wb, st->red);

    buffer_strcat(wb, ",\n\t\t\t\"alarms\": {\n");
    size_t alarms = 0;
    RRDCALC *rc;
    for(rc = st->alarms; rc ; rc = rc->rrdset_next) {

        buffer_sprintf(
                wb
                , "%s"
                  "\t\t\t\t\"%s\": {\n"
                  "\t\t\t\t\t\"id\": %u,\n"
                  "\t\t\t\t\t\"status\": \"%s\",\n"
                  "\t\t\t\t\t\"units\": \"%s\",\n"
                  "\t\t\t\t\t\"update_every\": %d\n"
                  "\t\t\t\t}"
                , (alarms) ? ",\n" : ""
                , rc->name
                , rc->id
                , rrdcalc_status2string(rc->status)
                , rc->units
                , rc->update_every
        );

        alarms++;
    }

    buffer_sprintf(wb,
            "\n\t\t\t}\n\t\t}"
    );

    rrdset_unlock(st);
}

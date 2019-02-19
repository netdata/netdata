// SPDX-License-Identifier: GPL-3.0-or-later

#include "allmetrics_shell.h"

// ----------------------------------------------------------------------------
// BASH
// /api/v1/allmetrics?format=bash

static inline size_t shell_name_copy(char *d, const char *s, size_t usable) {
    size_t n;

    for(n = 0; *s && n < usable ; d++, s++, n++) {
        register char c = *s;

        if(unlikely(!isalnum(c))) *d = '_';
        else *d = (char)toupper(c);
    }
    *d = '\0';

    return n;
}

#define SHELL_ELEMENT_MAX 100

void rrd_stats_api_v1_charts_allmetrics_shell(RRDHOST *host, BUFFER *wb) {
    rrdhost_rdlock(host);

    // for each chart
    RRDSET *st;
    rrdset_foreach_read(st, host) {
        calculated_number total = 0.0;
        char chart[SHELL_ELEMENT_MAX + 1];
        shell_name_copy(chart, st->name?st->name:st->id, SHELL_ELEMENT_MAX);

        buffer_sprintf(wb, "\n# chart: %s (name: %s)\n", st->id, st->name);
        if(rrdset_is_available_for_viewers(st)) {
            rrdset_rdlock(st);

            // for each dimension
            RRDDIM *rd;
            rrddim_foreach_read(rd, st) {
                if(rd->collections_counter && !rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE)) {
                    char dimension[SHELL_ELEMENT_MAX + 1];
                    shell_name_copy(dimension, rd->name?rd->name:rd->id, SHELL_ELEMENT_MAX);

                    calculated_number n = rd->last_stored_value;

                    if(isnan(n) || isinf(n))
                        buffer_sprintf(wb, "NETDATA_%s_%s=\"\"      # %s\n", chart, dimension, st->units);
                    else {
                        if(rd->multiplier < 0 || rd->divisor < 0) n = -n;
                        n = calculated_number_round(n);
                        if(!rrddim_flag_check(rd, RRDDIM_FLAG_HIDDEN)) total += n;
                        buffer_sprintf(wb, "NETDATA_%s_%s=\"" CALCULATED_NUMBER_FORMAT_ZERO "\"      # %s\n", chart, dimension, n, st->units);
                    }
                }
            }

            total = calculated_number_round(total);
            buffer_sprintf(wb, "NETDATA_%s_VISIBLETOTAL=\"" CALCULATED_NUMBER_FORMAT_ZERO "\"      # %s\n", chart, total, st->units);
            rrdset_unlock(st);
        }
    }

    buffer_strcat(wb, "\n# NETDATA ALARMS RUNNING\n");

    RRDCALC *rc;
    for(rc = host->alarms; rc ;rc = rc->next) {
        if(!rc->rrdset) continue;

        char chart[SHELL_ELEMENT_MAX + 1];
        shell_name_copy(chart, rc->rrdset->name?rc->rrdset->name:rc->rrdset->id, SHELL_ELEMENT_MAX);

        char alarm[SHELL_ELEMENT_MAX + 1];
        shell_name_copy(alarm, rc->name, SHELL_ELEMENT_MAX);

        calculated_number n = rc->value;

        if(isnan(n) || isinf(n))
            buffer_sprintf(wb, "NETDATA_ALARM_%s_%s_VALUE=\"\"      # %s\n", chart, alarm, rc->units);
        else {
            n = calculated_number_round(n);
            buffer_sprintf(wb, "NETDATA_ALARM_%s_%s_VALUE=\"" CALCULATED_NUMBER_FORMAT_ZERO "\"      # %s\n", chart, alarm, n, rc->units);
        }

        buffer_sprintf(wb, "NETDATA_ALARM_%s_%s_STATUS=\"%s\"\n", chart, alarm, rrdcalc_status2string(rc->status));
    }

    rrdhost_unlock(host);
}

// ----------------------------------------------------------------------------

void rrd_stats_api_v1_charts_allmetrics_json(RRDHOST *host, BUFFER *wb) {
    rrdhost_rdlock(host);

    buffer_strcat(wb, "{");

    size_t chart_counter = 0;
    size_t dimension_counter = 0;

    // for each chart
    RRDSET *st;
    rrdset_foreach_read(st, host) {
        if(rrdset_is_available_for_viewers(st)) {
            rrdset_rdlock(st);

            buffer_sprintf(wb, "%s\n"
                               "\t\"%s\": {\n"
                               "\t\t\"name\":\"%s\",\n"
                               "\t\t\"context\":\"%s\",\n"
                               "\t\t\"units\":\"%s\",\n"
                               "\t\t\"last_updated\": %ld,\n"
                               "\t\t\"dimensions\": {"
                           , chart_counter?",":""
                           , st->id
                           , st->name
                           , st->context
                           , st->units
                           , rrdset_last_entry_t(st)
            );

            chart_counter++;
            dimension_counter = 0;

            // for each dimension
            RRDDIM *rd;
            rrddim_foreach_read(rd, st) {
                if(rd->collections_counter && !rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE)) {

                    buffer_sprintf(wb, "%s\n"
                                       "\t\t\t\"%s\": {\n"
                                       "\t\t\t\t\"name\": \"%s\",\n"
                                       "\t\t\t\t\"value\": "
                                   , dimension_counter?",":""
                                   , rd->id
                                   , rd->name
                    );

                    if(isnan(rd->last_stored_value))
                        buffer_strcat(wb, "null");
                    else
                        buffer_sprintf(wb, CALCULATED_NUMBER_FORMAT, rd->last_stored_value);

                    buffer_strcat(wb, "\n\t\t\t}");

                    dimension_counter++;
                }
            }

            buffer_strcat(wb, "\n\t\t}\n\t}");
            rrdset_unlock(st);
        }
    }

    buffer_strcat(wb, "\n}");
    rrdhost_unlock(host);
}


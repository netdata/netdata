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

void rrd_stats_api_v1_charts_allmetrics_shell(RRDHOST *host, const char *filter_string, BUFFER *wb) {
    analytics_log_shell();
    SIMPLE_PATTERN *filter = simple_pattern_create(filter_string, NULL, SIMPLE_PATTERN_EXACT);
    rrdhost_rdlock(host);

    // for each chart
    RRDSET *st;
    rrdset_foreach_read(st, host) {
        if (filter && !simple_pattern_matches(filter, st->name))
            continue;

        NETDATA_DOUBLE total = 0.0;
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
                    shell_name_copy(dimension, rd->name?rrddim_name(rd):rrddim_id(rd), SHELL_ELEMENT_MAX);

                    NETDATA_DOUBLE n = rd->last_stored_value;

                    if(isnan(n) || isinf(n))
                        buffer_sprintf(wb, "NETDATA_%s_%s=\"\"      # %s\n", chart, dimension, rrdset_units(st));
                    else {
                        if(rd->multiplier < 0 || rd->divisor < 0) n = -n;
                        n = roundndd(n);
                        if(!rrddim_flag_check(rd, RRDDIM_FLAG_HIDDEN)) total += n;
                        buffer_sprintf(wb, "NETDATA_%s_%s=\"" NETDATA_DOUBLE_FORMAT_ZERO "\"      # %s\n", chart, dimension, n, rrdset_units(st));
                    }
                }
            }

            total = roundndd(total);
            buffer_sprintf(wb, "NETDATA_%s_VISIBLETOTAL=\"" NETDATA_DOUBLE_FORMAT_ZERO "\"      # %s\n", chart, total, rrdset_units(st));
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

        NETDATA_DOUBLE n = rc->value;

        if(isnan(n) || isinf(n))
            buffer_sprintf(wb, "NETDATA_ALARM_%s_%s_VALUE=\"\"      # %s\n", chart, alarm, rc->units);
        else {
            n = roundndd(n);
            buffer_sprintf(wb, "NETDATA_ALARM_%s_%s_VALUE=\"" NETDATA_DOUBLE_FORMAT_ZERO "\"      # %s\n", chart, alarm, n, rc->units);
        }

        buffer_sprintf(wb, "NETDATA_ALARM_%s_%s_STATUS=\"%s\"\n", chart, alarm, rrdcalc_status2string(rc->status));
    }

    rrdhost_unlock(host);
    simple_pattern_free(filter);
}

// ----------------------------------------------------------------------------

void rrd_stats_api_v1_charts_allmetrics_json(RRDHOST *host, const char *filter_string, BUFFER *wb) {
    analytics_log_json();
    SIMPLE_PATTERN *filter = simple_pattern_create(filter_string, NULL, SIMPLE_PATTERN_EXACT);
    rrdhost_rdlock(host);

    buffer_strcat(wb, "{");

    size_t chart_counter = 0;
    size_t dimension_counter = 0;

    // for each chart
    RRDSET *st;
    rrdset_foreach_read(st, host) {
        if (filter && !(simple_pattern_matches(filter, st->id) || simple_pattern_matches(filter, st->name)))
            continue;

        if(rrdset_is_available_for_viewers(st)) {
            rrdset_rdlock(st);

            buffer_sprintf(
                wb,
                "%s\n"
                "\t\"%s\": {\n"
                "\t\t\"name\":\"%s\",\n"
                "\t\t\"family\":\"%s\",\n"
                "\t\t\"context\":\"%s\",\n"
                "\t\t\"units\":\"%s\",\n"
                "\t\t\"last_updated\": %"PRId64",\n"
                "\t\t\"dimensions\": {",
                chart_counter ? "," : "",
                st->id,
                st->name,
                rrdset_family(st),
                st->context,
                rrdset_units(st),
                (int64_t)rrdset_last_entry_t_nolock(st));

            chart_counter++;
            dimension_counter = 0;

            // for each dimension
            RRDDIM *rd;
            rrddim_foreach_read(rd, st) {
                if(rd->collections_counter && !rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE)) {
                    buffer_sprintf(
                        wb,
                        "%s\n"
                        "\t\t\t\"%s\": {\n"
                        "\t\t\t\t\"name\": \"%s\",\n"
                        "\t\t\t\t\"value\": ",
                        dimension_counter ? "," : "",
                        rrddim_id(rd),
                        rrddim_name(rd));

                    if(isnan(rd->last_stored_value))
                        buffer_strcat(wb, "null");
                    else
                        buffer_sprintf(wb, NETDATA_DOUBLE_FORMAT, rd->last_stored_value);

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
    simple_pattern_free(filter);
}


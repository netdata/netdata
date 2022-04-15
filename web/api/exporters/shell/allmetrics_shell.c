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

/**
 * @brief Update simple pattern for chart filtering if there is a new filter
 * 
 * @param filter_sp simple pattern to update
 * @param prev_filter previously stored filter
 * @param filter a new filter
 * @return Returns 1 if the filter has changed, 0 otherwise 
 */
inline int update_filter(SIMPLE_PATTERN **filter_sp, char **prev_filter, const char *filter)
{
    int filter_changed = 0;

    if (*prev_filter && filter) {
        if (strcmp(*prev_filter, filter))
            filter_changed = 1;
    } else if (*prev_filter || filter) {
        filter_changed = 1;
    }

    if (filter_changed) {
        freez(*prev_filter);
        simple_pattern_free(*filter_sp);

        if (filter) {
            *prev_filter = strdupz(filter);
            *filter_sp = simple_pattern_create(filter, NULL, SIMPLE_PATTERN_EXACT);
        } else {
            *prev_filter = NULL;
            *filter_sp = NULL;
        }
    }

    return filter_changed;
}

int chart_is_filtered_out(SIMPLE_PATTERN *filter_sp, int filter_changed, int filter_type)
{
    if (filter_changed) {

/*
        if (unlikely(rrdset_flag_check(st, RRDSET_FLAG_EXPORTING_IGNORE)))
        return 0;

        if (unlikely(!rrdset_flag_check(st, RRDSET_FLAG_EXPORTING_SEND))) {
            // we have not checked this chart
            if (simple_pattern_matches(instance->config.charts_pattern, st->id) ||
                simple_pattern_matches(instance->config.charts_pattern, st->name))
                rrdset_flag_set(st, RRDSET_FLAG_EXPORTING_SEND);
            else {
                rrdset_flag_set(st, RRDSET_FLAG_EXPORTING_IGNORE);
                debug(
                    D_EXPORTING,
                    "EXPORTING: not sending chart '%s' of host '%s', because it is disabled for exporting.",
                    st->id,
                    host->hostname);
                return 0;
            }
        }
*/
    }

    return 0;
}

void rrd_stats_api_v1_charts_allmetrics_shell(RRDHOST *host, const char *filter, BUFFER *wb) {
    analytics_log_shell();
    rrdhost_rdlock(host);

    static char *prev_filter = NULL;
    static SIMPLE_PATTERN *filter_sp = NULL;
    static uv_mutex_t *filter_mutex = NULL;
    static uv_cond_t *filter_cond = NULL;
    static int request_number = 0;

    if (!filter_mutex) {
        filter_mutex = callocz(1, sizeof(uv_mutex_t));
        if (uv_mutex_init(filter_mutex)
            fatal("Cannot initialize mutex for allmetrics filter");

        filter_cond = callocz(1, sizeof(uv_cond_t));
        if (uv_cond_init(filter_cond)
            fatal("Cannot initialize conditional variable for allmetrics filter");
    }

    uv_mutex_lock(filter_mutex);
    request_number++;
    int filter_changed = update_filter(&filter_sp, &prev_filter, filter);
    if (filter_changed) {
        while (request_number > 1)
            uv_cond_wait(filter_cond, filter_mutex);
        
    } else {
        uv_mutex_unlock(filter_mutex);
    }

    // for each chart
    RRDSET *st;
    rrdset_foreach_read(st, host) {
        if (chart_is_filtered_out(filter_sp, filter_changed, RRDSET_API_FILTER_SHELL))
            continue;

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

void rrd_stats_api_v1_charts_allmetrics_json(RRDHOST *host, const char *filter, BUFFER *wb) {
    analytics_log_json();
    rrdhost_rdlock(host);

    buffer_strcat(wb, "{");

    size_t chart_counter = 0;
    size_t dimension_counter = 0;

    // for each chart
    RRDSET *st;
    rrdset_foreach_read(st, host) {
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
                st->family,
                st->context,
                st->units,
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
                        rd->id,
                        rd->name);

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


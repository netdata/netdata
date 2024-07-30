// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v1_calls.h"

#define ALLMETRICS_FORMAT_SHELL                 "shell"
#define ALLMETRICS_FORMAT_PROMETHEUS            "prometheus"
#define ALLMETRICS_FORMAT_PROMETHEUS_ALL_HOSTS  "prometheus_all_hosts"
#define ALLMETRICS_FORMAT_JSON                  "json"

#define ALLMETRICS_SHELL                        1
#define ALLMETRICS_PROMETHEUS                   2
#define ALLMETRICS_JSON                         3
#define ALLMETRICS_PROMETHEUS_ALL_HOSTS         4

struct prometheus_output_options {
    char *name;
    PROMETHEUS_OUTPUT_OPTIONS flag;
} prometheus_output_flags_root[] = {
    { "names",      PROMETHEUS_OUTPUT_NAMES      },
    { "timestamps", PROMETHEUS_OUTPUT_TIMESTAMPS },
    { "variables",  PROMETHEUS_OUTPUT_VARIABLES  },
    { "oldunits",   PROMETHEUS_OUTPUT_OLDUNITS   },
    { "hideunits",  PROMETHEUS_OUTPUT_HIDEUNITS  },
    // terminator
    { NULL, PROMETHEUS_OUTPUT_NONE },
};

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
    SIMPLE_PATTERN *filter = simple_pattern_create(filter_string, NULL, SIMPLE_PATTERN_EXACT, true);

    // for each chart
    RRDSET *st;
    rrdset_foreach_read(st, host) {
        if (filter && !simple_pattern_matches_string(filter, st->name))
            continue;
        if (rrdset_is_available_for_viewers(st)) {
            NETDATA_DOUBLE total = 0.0;

            char chart[SHELL_ELEMENT_MAX + 1];
            shell_name_copy(chart, st->name ? rrdset_name(st) : rrdset_id(st), SHELL_ELEMENT_MAX);

            buffer_sprintf(wb, "\n# chart: %s (name: %s)\n", rrdset_id(st), rrdset_name(st));

            // for each dimension
            RRDDIM *rd;
            rrddim_foreach_read(rd, st) {
                if(rd->collector.counter && !rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE)) {
                    char dimension[SHELL_ELEMENT_MAX + 1];
                    shell_name_copy(dimension, rd->name?rrddim_name(rd):rrddim_id(rd), SHELL_ELEMENT_MAX);

                    NETDATA_DOUBLE n = rd->collector.last_stored_value;

                    if(isnan(n) || isinf(n))
                        buffer_sprintf(wb, "NETDATA_%s_%s=\"\"      # %s\n", chart, dimension, rrdset_units(st));
                    else {
                        if(rd->multiplier < 0 || rd->divisor < 0) n = -n;
                        n = roundndd(n);
                        if(!rrddim_option_check(rd, RRDDIM_OPTION_HIDDEN)) total += n;
                        buffer_sprintf(wb, "NETDATA_%s_%s=\"" NETDATA_DOUBLE_FORMAT_ZERO "\"      # %s\n", chart, dimension, n, rrdset_units(st));
                    }
                }
            }
            rrddim_foreach_done(rd);

            total = roundndd(total);
            buffer_sprintf(wb, "NETDATA_%s_VISIBLETOTAL=\"" NETDATA_DOUBLE_FORMAT_ZERO "\"      # %s\n", chart, total, rrdset_units(st));
        }
    }
    rrdset_foreach_done(st);

    buffer_strcat(wb, "\n# NETDATA ALARMS RUNNING\n");

    RRDCALC *rc;
    foreach_rrdcalc_in_rrdhost_read(host, rc) {
        if(!rc->rrdset) continue;

        char chart[SHELL_ELEMENT_MAX + 1];
        shell_name_copy(chart, rc->rrdset->name?rrdset_name(rc->rrdset):rrdset_id(rc->rrdset), SHELL_ELEMENT_MAX);

        char alarm[SHELL_ELEMENT_MAX + 1];
        shell_name_copy(alarm, rrdcalc_name(rc), SHELL_ELEMENT_MAX);

        NETDATA_DOUBLE n = rc->value;

        if(isnan(n) || isinf(n))
            buffer_sprintf(wb, "NETDATA_ALARM_%s_%s_VALUE=\"\"      # %s\n", chart, alarm, rrdcalc_units(rc));
        else {
            n = roundndd(n);
            buffer_sprintf(wb, "NETDATA_ALARM_%s_%s_VALUE=\"" NETDATA_DOUBLE_FORMAT_ZERO "\"      # %s\n", chart, alarm, n, rrdcalc_units(rc));
        }

        buffer_sprintf(wb, "NETDATA_ALARM_%s_%s_STATUS=\"%s\"\n", chart, alarm, rrdcalc_status2string(rc->status));
    }
    foreach_rrdcalc_in_rrdhost_done(rc);

    simple_pattern_free(filter);
}

// ----------------------------------------------------------------------------

void rrd_stats_api_v1_charts_allmetrics_json(RRDHOST *host, const char *filter_string, BUFFER *wb) {
    analytics_log_json();
    SIMPLE_PATTERN *filter = simple_pattern_create(filter_string, NULL, SIMPLE_PATTERN_EXACT, true);

    buffer_strcat(wb, "{");

    size_t chart_counter = 0;
    size_t dimension_counter = 0;

    // for each chart
    RRDSET *st;
    rrdset_foreach_read(st, host) {
        if (filter && !(simple_pattern_matches_string(filter, st->id) || simple_pattern_matches_string(filter, st->name)))
            continue;

        if(rrdset_is_available_for_viewers(st)) {
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
                rrdset_id(st),
                rrdset_name(st),
                rrdset_family(st),
                rrdset_context(st),
                rrdset_units(st),
                (int64_t) rrdset_last_entry_s(st));

            chart_counter++;
            dimension_counter = 0;

            // for each dimension
            RRDDIM *rd;
            rrddim_foreach_read(rd, st) {
                if(rd->collector.counter && !rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE)) {
                    buffer_sprintf(
                        wb,
                        "%s\n"
                        "\t\t\t\"%s\": {\n"
                        "\t\t\t\t\"name\": \"%s\",\n"
                        "\t\t\t\t\"value\": ",
                        dimension_counter ? "," : "",
                        rrddim_id(rd),
                        rrddim_name(rd));

                    if(isnan(rd->collector.last_stored_value))
                        buffer_strcat(wb, "null");
                    else
                        buffer_sprintf(wb, NETDATA_DOUBLE_FORMAT, rd->collector.last_stored_value);

                    buffer_strcat(wb, "\n\t\t\t}");

                    dimension_counter++;
                }
            }
            rrddim_foreach_done(rd);

            buffer_strcat(wb, "\n\t\t}\n\t}");
        }
    }
    rrdset_foreach_done(st);

    buffer_strcat(wb, "\n}");
    simple_pattern_free(filter);
}

int api_v1_allmetrics(RRDHOST *host, struct web_client *w, char *url) {
    int format = ALLMETRICS_SHELL;
    const char *filter = NULL;
    const char *prometheus_server = w->client_ip;

    uint32_t prometheus_exporting_options;
    if (prometheus_exporter_instance)
        prometheus_exporting_options = prometheus_exporter_instance->config.options;
    else
        prometheus_exporting_options = global_exporting_options;

    PROMETHEUS_OUTPUT_OPTIONS prometheus_output_options =
        PROMETHEUS_OUTPUT_TIMESTAMPS |
        ((prometheus_exporting_options & EXPORTING_OPTION_SEND_NAMES) ? PROMETHEUS_OUTPUT_NAMES : 0);

    const char *prometheus_prefix;
    if (prometheus_exporter_instance)
        prometheus_prefix = prometheus_exporter_instance->config.prefix;
    else
        prometheus_prefix = global_exporting_prefix;

    while(url) {
        char *value = strsep_skip_consecutive_separators(&url, "&");
        if (!value || !*value) continue;

        char *name = strsep_skip_consecutive_separators(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        if(!strcmp(name, "format")) {
            if(!strcmp(value, ALLMETRICS_FORMAT_SHELL))
                format = ALLMETRICS_SHELL;
            else if(!strcmp(value, ALLMETRICS_FORMAT_PROMETHEUS))
                format = ALLMETRICS_PROMETHEUS;
            else if(!strcmp(value, ALLMETRICS_FORMAT_PROMETHEUS_ALL_HOSTS))
                format = ALLMETRICS_PROMETHEUS_ALL_HOSTS;
            else if(!strcmp(value, ALLMETRICS_FORMAT_JSON))
                format = ALLMETRICS_JSON;
            else
                format = 0;
        }
        else if(!strcmp(name, "filter")) {
            filter = value;
        }
        else if(!strcmp(name, "server")) {
            prometheus_server = value;
        }
        else if(!strcmp(name, "prefix")) {
            prometheus_prefix = value;
        }
        else if(!strcmp(name, "data") || !strcmp(name, "source") || !strcmp(name, "data source") || !strcmp(name, "data-source") || !strcmp(name, "data_source") || !strcmp(name, "datasource")) {
            prometheus_exporting_options = exporting_parse_data_source(value, prometheus_exporting_options);
        }
        else {
            int i;
            for(i = 0; prometheus_output_flags_root[i].name ; i++) {
                if(!strcmp(name, prometheus_output_flags_root[i].name)) {
                    if(!strcmp(value, "yes") || !strcmp(value, "1") || !strcmp(value, "true"))
                        prometheus_output_options |= prometheus_output_flags_root[i].flag;
                    else {
                        prometheus_output_options &= ~prometheus_output_flags_root[i].flag;
                    }

                    break;
                }
            }
        }
    }

    buffer_flush(w->response.data);
    buffer_no_cacheable(w->response.data);

    switch(format) {
        case ALLMETRICS_JSON:
            w->response.data->content_type = CT_APPLICATION_JSON;
            rrd_stats_api_v1_charts_allmetrics_json(host, filter, w->response.data);
            return HTTP_RESP_OK;

        case ALLMETRICS_SHELL:
            w->response.data->content_type = CT_TEXT_PLAIN;
            rrd_stats_api_v1_charts_allmetrics_shell(host, filter, w->response.data);
            return HTTP_RESP_OK;

        case ALLMETRICS_PROMETHEUS:
            w->response.data->content_type = CT_PROMETHEUS;
            rrd_stats_api_v1_charts_allmetrics_prometheus_single_host(
                host
                , filter
                , w->response.data
                , prometheus_server
                , prometheus_prefix
                , prometheus_exporting_options
                , prometheus_output_options
            );
            return HTTP_RESP_OK;

        case ALLMETRICS_PROMETHEUS_ALL_HOSTS:
            w->response.data->content_type = CT_PROMETHEUS;
            rrd_stats_api_v1_charts_allmetrics_prometheus_all_hosts(
                host
                , filter
                , w->response.data
                , prometheus_server
                , prometheus_prefix
                , prometheus_exporting_options
                , prometheus_output_options
            );
            return HTTP_RESP_OK;

        default:
            w->response.data->content_type = CT_TEXT_PLAIN;
            buffer_strcat(w->response.data, "Which format? '" ALLMETRICS_FORMAT_SHELL "', '" ALLMETRICS_FORMAT_PROMETHEUS "', '" ALLMETRICS_FORMAT_PROMETHEUS_ALL_HOSTS "' and '" ALLMETRICS_FORMAT_JSON "' are currently supported.");
            return HTTP_RESP_BAD_REQUEST;
    }
}

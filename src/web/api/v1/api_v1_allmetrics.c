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

        if(unlikely(!isalnum((uint8_t)c))) *d = '_';
        else *d = (char)toupper((uint8_t)c);
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
        RRDSET *alert_st = rc->rrdset;
        if(!alert_st) continue;

        rw_spinlock_read_lock(&alert_st->alerts.spinlock);
        if(unlikely(rc->rrdset != alert_st)) {
            rw_spinlock_read_unlock(&alert_st->alerts.spinlock);
            continue;
        }

        char chart[SHELL_ELEMENT_MAX + 1];
        shell_name_copy(chart, alert_st->name?rrdset_name(alert_st):rrdset_id(alert_st), SHELL_ELEMENT_MAX);

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
        rw_spinlock_read_unlock(&alert_st->alerts.spinlock);
    }
    foreach_rrdcalc_in_rrdhost_done(rc);

    simple_pattern_free(filter);
}

// ----------------------------------------------------------------------------

static void allmetrics_json_chart_begin(
    BUFFER *wb,
    bool prepend_comma,
    const char *id,
    const char *name,
    const char *family,
    const char *context,
    const char *units,
    int64_t last_updated) {
    buffer_strcat(wb, prepend_comma ? ",\n\t\"" : "\n\t\"");
    buffer_json_strcat(wb, id);
    buffer_strcat(wb, "\": {\n\t\t\"name\":\"");
    buffer_json_strcat(wb, name);
    buffer_strcat(wb, "\",\n\t\t\"family\":\"");
    buffer_json_strcat(wb, family);
    buffer_strcat(wb, "\",\n\t\t\"context\":\"");
    buffer_json_strcat(wb, context);
    buffer_strcat(wb, "\",\n\t\t\"units\":\"");
    buffer_json_strcat(wb, units);
    buffer_sprintf(
        wb,
        "\",\n"
        "\t\t\"last_updated\": %" PRId64 ",\n"
        "\t\t\"dimensions\": {",
        last_updated);
}

static void allmetrics_json_dimension_begin(
    BUFFER *wb, bool prepend_comma, const char *id, const char *name) {
    buffer_strcat(wb, prepend_comma ? ",\n\t\t\t\"" : "\n\t\t\t\"");
    buffer_json_strcat(wb, id);
    buffer_strcat(wb, "\": {\n\t\t\t\t\"name\": \"");
    buffer_json_strcat(wb, name);
    buffer_strcat(wb, "\",\n\t\t\t\t\"value\": ");
}

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
            allmetrics_json_chart_begin(
                wb,
                chart_counter != 0,
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
                    allmetrics_json_dimension_begin(
                        wb,
                        dimension_counter != 0,
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

static int allmetrics_json_unittest_case(
    const char *description,
    const char *chart_id,
    const char *chart_name,
    const char *family,
    const char *context,
    const char *units,
    const char *dimension_id,
    const char *dimension_name,
    const char *expected) {
    int errors = 0;
    BUFFER *wb = buffer_create(0, NULL);

    buffer_strcat(wb, "{");
    allmetrics_json_chart_begin(wb, false, chart_id, chart_name, family, context, units, 42);
    allmetrics_json_dimension_begin(wb, false, dimension_id, dimension_name);
    buffer_strcat(wb, "1.5\n\t\t\t}\n\t\t}\n\t}\n}");

    if(strcmp(buffer_tostring(wb), expected) != 0) {
        fprintf(stderr, "allmetrics JSON %s output mismatch\nexpected: %s\nactual:   %s\n",
                description, expected, buffer_tostring(wb));
        errors++;
    }

    json_object *root = json_tokener_parse(buffer_tostring(wb));
    json_object *chart = NULL, *member = NULL, *dimensions = NULL, *dimension = NULL;
    if(!root || !json_object_is_type(root, json_type_object) || json_object_object_length(root) != 1 ||
       !json_object_object_get_ex(root, chart_id, &chart) || !json_object_is_type(chart, json_type_object) ||
       json_object_object_length(chart) != 6 ||
       !json_object_object_get_ex(chart, "name", &member) || strcmp(json_object_get_string(member), chart_name) != 0 ||
       !json_object_object_get_ex(chart, "family", &member) || strcmp(json_object_get_string(member), family) != 0 ||
       !json_object_object_get_ex(chart, "context", &member) || strcmp(json_object_get_string(member), context) != 0 ||
       !json_object_object_get_ex(chart, "units", &member) || strcmp(json_object_get_string(member), units) != 0 ||
       !json_object_object_get_ex(chart, "last_updated", &member) || json_object_get_int64(member) != 42 ||
       !json_object_object_get_ex(chart, "dimensions", &dimensions) ||
       !json_object_is_type(dimensions, json_type_object) || json_object_object_length(dimensions) != 1 ||
       !json_object_object_get_ex(dimensions, dimension_id, &dimension) ||
       !json_object_is_type(dimension, json_type_object) || json_object_object_length(dimension) != 2 ||
       !json_object_object_get_ex(dimension, "name", &member) ||
       strcmp(json_object_get_string(member), dimension_name) != 0 ||
       !json_object_object_get_ex(dimension, "value", &member) || json_object_get_double(member) != 1.5) {
        fprintf(stderr, "allmetrics JSON %s schema or value mismatch\n", description);
        errors++;
    }

    if(root)
        json_object_put(root);
    buffer_free(wb);
    return errors;
}

int api_v1_allmetrics_json_unittest(void) {
    int errors = 0;

    errors += allmetrics_json_unittest_case(
        "ordinary metadata",
        "chart.id", "chart.name", "family", "context", "units", "dimension.id", "dimension.name",
        "{\n"
        "\t\"chart.id\": {\n"
        "\t\t\"name\":\"chart.name\",\n"
        "\t\t\"family\":\"family\",\n"
        "\t\t\"context\":\"context\",\n"
        "\t\t\"units\":\"units\",\n"
        "\t\t\"last_updated\": 42,\n"
        "\t\t\"dimensions\": {\n"
        "\t\t\t\"dimension.id\": {\n"
        "\t\t\t\t\"name\": \"dimension.name\",\n"
        "\t\t\t\t\"value\": 1.5\n"
        "\t\t\t}\n"
        "\t\t}\n"
        "\t}\n"
        "}");

    errors += allmetrics_json_unittest_case(
        "hostile metadata",
        "chart\"\\\x01 \xCE\xA9", "chart\"name", "family\\name", "context\nname", "units\tname",
        "dimension\rkey", "dimension\bname \xCE\xA9",
        "{\n"
        "\t\"chart\\\"\\\\\\u0001 \xCE\xA9\": {\n"
        "\t\t\"name\":\"chart\\\"name\",\n"
        "\t\t\"family\":\"family\\\\name\",\n"
        "\t\t\"context\":\"context\\nname\",\n"
        "\t\t\"units\":\"units\\tname\",\n"
        "\t\t\"last_updated\": 42,\n"
        "\t\t\"dimensions\": {\n"
        "\t\t\t\"dimension\\rkey\": {\n"
        "\t\t\t\t\"name\": \"dimension\\bname \xCE\xA9\",\n"
        "\t\t\t\t\"value\": 1.5\n"
        "\t\t\t}\n"
        "\t\t}\n"
        "\t}\n"
        "}");

    BUFFER *wb = buffer_create(0, NULL);
    allmetrics_json_chart_begin(wb, true, "chart", "name", "family", "context", "units", 42);
    if(strcmp(
           buffer_tostring(wb),
           ",\n\t\"chart\": {\n"
           "\t\t\"name\":\"name\",\n"
           "\t\t\"family\":\"family\",\n"
           "\t\t\"context\":\"context\",\n"
           "\t\t\"units\":\"units\",\n"
           "\t\t\"last_updated\": 42,\n"
           "\t\t\"dimensions\": {") != 0) {
        fprintf(stderr, "allmetrics JSON subsequent chart prefix mismatch\n");
        errors++;
    }

    buffer_flush(wb);
    allmetrics_json_dimension_begin(wb, true, "dimension", "name");
    if(strcmp(
           buffer_tostring(wb),
           ",\n\t\t\t\"dimension\": {\n"
           "\t\t\t\t\"name\": \"name\",\n"
           "\t\t\t\t\"value\": ") != 0) {
        fprintf(stderr, "allmetrics JSON subsequent dimension prefix mismatch\n");
        errors++;
    }
    buffer_free(wb);

    return errors;
}

int api_v1_allmetrics(RRDHOST *host, struct web_client *w, char *url) {
    int format = ALLMETRICS_SHELL;
    const char *filter = NULL;
    const char *prometheus_server = w->user_auth.client_ip;

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

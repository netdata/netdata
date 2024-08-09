// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v1_calls.h"

int api_v1_contexts(RRDHOST *host, struct web_client *w, char *url) {
    RRDCONTEXT_TO_JSON_OPTIONS options = RRDCONTEXT_OPTION_NONE;
    time_t after = 0, before = 0;
    const char *chart_label_key = NULL, *chart_labels_filter = NULL;
    BUFFER *dimensions = NULL;

    buffer_flush(w->response.data);

    while(url) {
        char *value = strsep_skip_consecutive_separators(&url, "&");
        if(!value || !*value) continue;

        char *name = strsep_skip_consecutive_separators(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        // name and value are now the parameters
        // they are not null and not empty

        if(!strcmp(name, "after")) after = str2l(value);
        else if(!strcmp(name, "before")) before = str2l(value);
        else if(!strcmp(name, "options")) options = rrdcontext_to_json_parse_options(value);
        else if(!strcmp(name, "chart_label_key")) chart_label_key = value;
        else if(!strcmp(name, "chart_labels_filter")) chart_labels_filter = value;
        else if(!strcmp(name, "dimension") || !strcmp(name, "dim") || !strcmp(name, "dimensions") || !strcmp(name, "dims")) {
            if(!dimensions) dimensions = buffer_create(100, &netdata_buffers_statistics.buffers_api);
            buffer_strcat(dimensions, "|");
            buffer_strcat(dimensions, value);
        }
    }

    SIMPLE_PATTERN *chart_label_key_pattern = NULL;
    SIMPLE_PATTERN *chart_labels_filter_pattern = NULL;
    SIMPLE_PATTERN *chart_dimensions_pattern = NULL;

    if(chart_label_key)
        chart_label_key_pattern = simple_pattern_create(chart_label_key, ",|\t\r\n\f\v", SIMPLE_PATTERN_EXACT, true);

    if(chart_labels_filter)
        chart_labels_filter_pattern = simple_pattern_create(chart_labels_filter, ",|\t\r\n\f\v", SIMPLE_PATTERN_EXACT,
                                                            true);

    if(dimensions) {
        chart_dimensions_pattern = simple_pattern_create(buffer_tostring(dimensions), ",|\t\r\n\f\v",
                                                         SIMPLE_PATTERN_EXACT, true);
        buffer_free(dimensions);
    }

    w->response.data->content_type = CT_APPLICATION_JSON;
    int ret = rrdcontexts_to_json(host, w->response.data, after, before, options, chart_label_key_pattern, chart_labels_filter_pattern, chart_dimensions_pattern);

    simple_pattern_free(chart_label_key_pattern);
    simple_pattern_free(chart_labels_filter_pattern);
    simple_pattern_free(chart_dimensions_pattern);

    return ret;
}

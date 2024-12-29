// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v2_calls.h"

int web_client_api_request_weights(RRDHOST *host, struct web_client *w, char *url, WEIGHTS_METHOD method, WEIGHTS_FORMAT format, size_t api_version) {
    if (!netdata_ready)
        return HTTP_RESP_SERVICE_UNAVAILABLE;

    time_t baseline_after = 0, baseline_before = 0, after = 0, before = 0;
    size_t points = 0;
    RRDR_OPTIONS options = 0;
    RRDR_TIME_GROUPING time_group_method = RRDR_GROUPING_AVERAGE;
    time_t timeout_ms = 0;
    size_t tier = 0;
    const char *time_group_options = NULL, *scope_contexts = NULL, *scope_nodes = NULL, *contexts = NULL, *nodes = NULL,
               *instances = NULL, *dimensions = NULL, *labels = NULL, *alerts = NULL;

    struct group_by_pass group_by = {
        .group_by = RRDR_GROUP_BY_NONE,
        .group_by_label = NULL,
        .aggregation = RRDR_GROUP_BY_FUNCTION_AVERAGE,
    };

    while (url) {
        char *value = strsep_skip_consecutive_separators(&url, "&");
        if (!value || !*value)
            continue;

        char *name = strsep_skip_consecutive_separators(&value, "=");
        if (!name || !*name)
            continue;
        if (!value || !*value)
            continue;

        if (!strcmp(name, "baseline_after"))
            baseline_after = str2l(value);

        else if (!strcmp(name, "baseline_before"))
            baseline_before = str2l(value);

        else if (!strcmp(name, "after") || !strcmp(name, "highlight_after"))
            after = str2l(value);

        else if (!strcmp(name, "before") || !strcmp(name, "highlight_before"))
            before = str2l(value);

        else if (!strcmp(name, "points") || !strcmp(name, "max_points"))
            points = str2ul(value);

        else if (!strcmp(name, "timeout"))
            timeout_ms = str2l(value);

        else if((api_version == 1 && !strcmp(name, "group")) || (api_version >= 2 && !strcmp(name, "time_group")))
            time_group_method = time_grouping_parse(value, RRDR_GROUPING_AVERAGE);

        else if((api_version == 1 && !strcmp(name, "group_options")) || (api_version >= 2 && !strcmp(name, "time_group_options")))
            time_group_options = value;

        else if(!strcmp(name, "options"))
            options |= rrdr_options_parse(value);

        else if(!strcmp(name, "method"))
            method = weights_string_to_method(value);

        else if(api_version == 1 && (!strcmp(name, "context") || !strcmp(name, "contexts")))
            scope_contexts = value;

        else if(api_version >= 2 && !strcmp(name, "scope_nodes")) scope_nodes = value;
        else if(api_version >= 2 && !strcmp(name, "scope_contexts")) scope_contexts = value;
        else if(api_version >= 2 && !strcmp(name, "nodes")) nodes = value;
        else if(api_version >= 2 && !strcmp(name, "contexts")) contexts = value;
        else if(api_version >= 2 && !strcmp(name, "instances")) instances = value;
        else if(api_version >= 2 && !strcmp(name, "dimensions")) dimensions = value;
        else if(api_version >= 2 && !strcmp(name, "labels")) labels = value;
        else if(api_version >= 2 && !strcmp(name, "alerts")) alerts = value;
        else if(api_version >= 2 && (!strcmp(name, "group_by") || !strcmp(name, "group_by[0]"))) {
            group_by.group_by = group_by_parse(value);
        }
        else if(api_version >= 2 && (!strcmp(name, "group_by_label") || !strcmp(name, "group_by_label[0]"))) {
            group_by.group_by_label = value;
        }
        else if(api_version >= 2 && (!strcmp(name, "aggregation") || !strcmp(name, "aggregation[0]"))) {
            group_by.aggregation = group_by_aggregate_function_parse(value);
        }

        else if(!strcmp(name, "tier")) {
            tier = str2ul(value);
            if(tier < nd_profile.storage_tiers)
                options |= RRDR_OPTION_SELECTED_TIER;
            else
                tier = 0;
        }
    }

    if(options == 0)
        // the user did not set any options
        options  = RRDR_OPTION_NOT_ALIGNED | RRDR_OPTION_NULL2ZERO | RRDR_OPTION_NONZERO;
    else
        // the user set some options, add also these
        options |= RRDR_OPTION_NOT_ALIGNED | RRDR_OPTION_NULL2ZERO;

    if(options & RRDR_OPTION_PERCENTAGE)
        options |= RRDR_OPTION_ABSOLUTE;

    if(options & RRDR_OPTION_DEBUG)
        options &= ~RRDR_OPTION_MINIFY;

    BUFFER *wb = w->response.data;
    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;

    QUERY_WEIGHTS_REQUEST qwr = {
        .version = api_version,
        .host = (api_version == 1) ? NULL : host,
        .scope_nodes = scope_nodes,
        .scope_contexts = scope_contexts,
        .nodes = nodes,
        .contexts = contexts,
        .instances = instances,
        .dimensions = dimensions,
        .labels = labels,
        .alerts = alerts,
        .group_by = {
            .group_by = group_by.group_by,
            .group_by_label = group_by.group_by_label,
            .aggregation = group_by.aggregation,
        },
        .method = method,
        .format = format,
        .time_group_method = time_group_method,
        .time_group_options = time_group_options,
        .baseline_after = baseline_after,
        .baseline_before = baseline_before,
        .after = after,
        .before = before,
        .points = points,
        .options = options,
        .tier = tier,
        .timeout_ms = timeout_ms,

        .interrupt_callback = web_client_interrupt_callback,
        .interrupt_callback_data = w,

        .transaction = &w->transaction,
    };

    return web_api_v12_weights(wb, &qwr);
}

int api_v2_weights(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    return web_client_api_request_weights(host, w, url, WEIGHTS_METHOD_VALUE, WEIGHTS_FORMAT_MULTINODE, 2);
}

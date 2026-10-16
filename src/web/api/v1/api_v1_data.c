// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v1_calls.h"

int api_v1_data(RRDHOST *host, struct web_client *w, char *url) {
    netdata_log_debug(D_WEB_CLIENT, "%llu: API v1 data with URL '%s'", w->id, url);

    int ret = HTTP_RESP_BAD_REQUEST;
    BUFFER *dimensions = NULL;

    buffer_flush(w->response.data);

    DATA_QUERY_JSONP jp = DATA_QUERY_JSONP_DEFAULTS;

    char *chart = NULL;
    char *before_str = NULL;
    char *after_str = NULL;
    char *group_time_str = NULL;
    char *points_str = NULL;
    char *timeout_str = NULL;
    char *context = NULL;
    char *chart_label_key = NULL;
    char *chart_labels_filter = NULL;
    char *group_options = NULL;
    char *cardinality_limit_str = NULL;
    char *limit_str = NULL;
    size_t tier = 0;
    size_t cardinality_limit = 0;
    RRDR_TIME_GROUPING group = RRDR_GROUPING_AVERAGE;
    DATASOURCE_FORMAT format = DATASOURCE_JSON;
    RRDR_OPTIONS options = 0;

    while(url) {
        char *value = strsep_skip_consecutive_separators(&url, "&");
        if(!value || !*value) continue;

        char *name = strsep_skip_consecutive_separators(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        netdata_log_debug(D_WEB_CLIENT, "%llu: API v1 data query param '%s' with value '%s'", w->id, name, value);

        // name and value are now the parameters
        // they are not null and not empty

        if(!strcmp(name, "context")) context = value;
        else if(!strcmp(name, "chart_label_key")) chart_label_key = value;
        else if(!strcmp(name, "chart_labels_filter")) chart_labels_filter = value;
        else if(!strcmp(name, "chart")) chart = value;
        else if(!strcmp(name, "dimension") || !strcmp(name, "dim") || !strcmp(name, "dimensions") || !strcmp(name, "dims")) {
            if(!dimensions) dimensions = buffer_create(100, &netdata_buffers_statistics.buffers_api);
            buffer_strcat(dimensions, "|");
            buffer_strcat(dimensions, value);
        }
        else if(!strcmp(name, "show_dimensions")) options |= RRDR_OPTION_ALL_DIMENSIONS;
        else if(!strcmp(name, "after")) after_str = value;
        else if(!strcmp(name, "before")) before_str = value;
        else if(!strcmp(name, "points")) points_str = value;
        else if(!strcmp(name, "timeout")) timeout_str = value;
        else if(!strcmp(name, "gtime")) group_time_str = value;
        else if(!strcmp(name, "group_options")) group_options = value;
        else if(!strcmp(name, "cardinality_limit")) cardinality_limit_str = value;
        else if(!strcmp(name, "limit")) limit_str = value;
        else if(!strcmp(name, "group")) {
            group = time_grouping_parse(value, RRDR_GROUPING_AVERAGE);
        }
        else if(!strcmp(name, "format")) {
            format = datasource_format_str_to_id(value);
        }
        else if(!strcmp(name, "options")) {
            options |= rrdr_options_parse(value);
        }
        else if(!strcmp(name, "callback")) {
            jp.responseHandler = value;
        }
        else if(!strcmp(name, "filename")) {
            jp.outFileName = value;
        }
        else if(!strcmp(name, "tqx")) data_query_jsonp_parse_tqx(&jp, value, &format);
        else if(!strcmp(name, "tier")) {
            tier = str2ul(value);
            if(tier < nd_profile.storage_tiers)
                options |= RRDR_OPTION_SELECTED_TIER;
            else
                tier = 0;
        }
    }

    // validate the google parameters given
    data_query_jsonp_sanitize(&jp);

    RRDSET *st = NULL;
    RRDSET_ACQUIRED *rsa = NULL;
    ONEWAYALLOC *owa = onewayalloc_create(0);
    QUERY_TARGET *qt = NULL;

    if(!is_valid_sp(chart) && !is_valid_sp(context)) {
        buffer_sprintf(w->response.data, "No chart or context is given.");
        goto cleanup;
    }

    if(chart && !context) {
        // check if this is a specific chart
        rsa = rrdset_find_and_acquire(host, chart, false);
        if (!rsa) rsa = rrdset_find_byname_and_acquire(host, chart);
        st = rrdset_acquired_to_rrdset(rsa);
    }

    long long before = (before_str && *before_str)?str2l(before_str):0;
    long long after  = (after_str  && *after_str) ?str2l(after_str):-600;
    int       points = (points_str && *points_str)?str2i(points_str):0;
    int       timeout = (timeout_str && *timeout_str)?str2i(timeout_str): 0;
    long      group_time = (group_time_str && *group_time_str)?str2l(group_time_str):0;
    
    if (cardinality_limit_str && *cardinality_limit_str)
        cardinality_limit = str2ul(cardinality_limit_str);
    else if (limit_str && *limit_str) {
        // limit=N: top-N dimensions plus the 'remaining' aggregate; ignored
        // by agents that predate the parameter (see api_v2_data.c)
        size_t limit = str2ul(limit_str);
        if (limit)
            cardinality_limit = (limit < SIZE_MAX) ? limit + 1 : limit;
    }

    QUERY_TARGET_REQUEST qtr = {
        .version = 1,
        .after = after,
        .before = before,
        .host = host,
        .st = st,
        .nodes = NULL,
        .contexts = context,
        .instances = chart,
        .dimensions = (dimensions)?buffer_tostring(dimensions):NULL,
        .timeout_ms = timeout,
        .points = points,
        .format = format,
        .options = options,
        .time_group_method = group,
        .time_group_options = group_options,
        .resampling_time = group_time,
        .tier = tier,
        .chart_label_key = chart_label_key,
        .labels = chart_labels_filter,
        .query_source = QUERY_SOURCE_API_DATA,
        .priority = STORAGE_PRIORITY_NORMAL,
        .cardinality_limit = cardinality_limit,
        .interrupt_callback = web_client_interrupt_callback,
        .interrupt_callback_data = w,
        .transaction = &w->transaction,
    };
    qt = query_target_create(&qtr);

    // checked before "no metrics": a cancelled build may have matched nothing yet
    int cancelled_code = query_target_cancelled_response(qt, w->response.data);
    if(cancelled_code) {
        ret = cancelled_code;
        goto cleanup;
    }

    if(!qt || !qt->query.used) {
        buffer_sprintf(w->response.data, "No metrics where matched to query.");
        ret = HTTP_RESP_NOT_FOUND;
        goto cleanup;
    }

    ret = data_query_response_begin(w, &jp, format, timeout, st ? st->last_updated.tv_sec : 0);
    if(ret)
        goto cleanup;

    ret = data_query_response_execute(owa, w, qt, &jp, format);

cleanup:
    query_target_release(qt);
    rrdset_acquired_release(rsa);
    onewayalloc_destroy(owa);
    buffer_free(dimensions);
    return ret;
}

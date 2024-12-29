// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v1_calls.h"

int api_v1_data(RRDHOST *host, struct web_client *w, char *url) {
    netdata_log_debug(D_WEB_CLIENT, "%llu: API v1 data with URL '%s'", w->id, url);

    int ret = HTTP_RESP_BAD_REQUEST;
    BUFFER *dimensions = NULL;

    buffer_flush(w->response.data);

    char    *google_version = "0.6",
         *google_reqId = "0",
         *google_sig = "0",
         *google_out = "json",
         *responseHandler = NULL,
         *outFileName = NULL;

    time_t last_timestamp_in_data = 0, google_timestamp = 0;

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
    size_t tier = 0;
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
            responseHandler = value;
        }
        else if(!strcmp(name, "filename")) {
            outFileName = value;
        }
        else if(!strcmp(name, "tqx")) {
            // parse Google Visualization API options
            // https://developers.google.com/chart/interactive/docs/dev/implementing_data_source
            char *tqx_name, *tqx_value;

            while(value) {
                tqx_value = strsep_skip_consecutive_separators(&value, ";");
                if(!tqx_value || !*tqx_value) continue;

                tqx_name = strsep_skip_consecutive_separators(&tqx_value, ":");
                if(!tqx_name || !*tqx_name) continue;
                if(!tqx_value || !*tqx_value) continue;

                if(!strcmp(tqx_name, "version"))
                    google_version = tqx_value;
                else if(!strcmp(tqx_name, "reqId"))
                    google_reqId = tqx_value;
                else if(!strcmp(tqx_name, "sig")) {
                    google_sig = tqx_value;
                    google_timestamp = strtoul(google_sig, NULL, 0);
                }
                else if(!strcmp(tqx_name, "out")) {
                    google_out = tqx_value;
                    format = google_data_format_str_to_id(google_out);
                }
                else if(!strcmp(tqx_name, "responseHandler"))
                    responseHandler = tqx_value;
                else if(!strcmp(tqx_name, "outFileName"))
                    outFileName = tqx_value;
            }
        }
        else if(!strcmp(name, "tier")) {
            tier = str2ul(value);
            if(tier < nd_profile.storage_tiers)
                options |= RRDR_OPTION_SELECTED_TIER;
            else
                tier = 0;
        }
    }

    // validate the google parameters given
    fix_google_param(google_out);
    fix_google_param(google_sig);
    fix_google_param(google_reqId);
    fix_google_param(google_version);
    fix_google_param(responseHandler);
    fix_google_param(outFileName);

    RRDSET *st = NULL;
    ONEWAYALLOC *owa = onewayalloc_create(0);
    QUERY_TARGET *qt = NULL;

    if(!is_valid_sp(chart) && !is_valid_sp(context)) {
        buffer_sprintf(w->response.data, "No chart or context is given.");
        goto cleanup;
    }

    if(chart && !context) {
        // check if this is a specific chart
        st = rrdset_find(host, chart);
        if (!st) st = rrdset_find_byname(host, chart);
    }

    long long before = (before_str && *before_str)?str2l(before_str):0;
    long long after  = (after_str  && *after_str) ?str2l(after_str):-600;
    int       points = (points_str && *points_str)?str2i(points_str):0;
    int       timeout = (timeout_str && *timeout_str)?str2i(timeout_str): 0;
    long      group_time = (group_time_str && *group_time_str)?str2l(group_time_str):0;

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
        .interrupt_callback = web_client_interrupt_callback,
        .interrupt_callback_data = w,
        .transaction = &w->transaction,
    };
    qt = query_target_create(&qtr);

    if(!qt || !qt->query.used) {
        buffer_sprintf(w->response.data, "No metrics where matched to query.");
        ret = HTTP_RESP_NOT_FOUND;
        goto cleanup;
    }

    web_client_timeout_checkpoint_set(w, timeout);
    if(web_client_timeout_checkpoint_and_check(w, NULL)) {
        ret = w->response.code;
        goto cleanup;
    }

    if(outFileName && *outFileName) {
        buffer_sprintf(w->response.header, "Content-Disposition: attachment; filename=\"%s\"\r\n", outFileName);
        netdata_log_debug(D_WEB_CLIENT, "%llu: generating outfilename header: '%s'", w->id, outFileName);
    }

    if(format == DATASOURCE_DATATABLE_JSONP) {
        if(responseHandler == NULL)
            responseHandler = "google.visualization.Query.setResponse";

        netdata_log_debug(D_WEB_CLIENT_ACCESS, "%llu: GOOGLE JSON/JSONP: version = '%s', reqId = '%s', sig = '%s', out = '%s', responseHandler = '%s', outFileName = '%s'",
                          w->id, google_version, google_reqId, google_sig, google_out, responseHandler, outFileName
        );

        buffer_sprintf(
            w->response.data,
            "%s({version:'%s',reqId:'%s',status:'ok',sig:'%"PRId64"',table:",
            responseHandler,
            google_version,
            google_reqId,
            (int64_t)(st ? st->last_updated.tv_sec : 0));
    }
    else if(format == DATASOURCE_JSONP) {
        if(responseHandler == NULL)
            responseHandler = "callback";

        buffer_strcat(w->response.data, responseHandler);
        buffer_strcat(w->response.data, "(");
    }

    ret = data_query_execute(owa, w->response.data, qt, &last_timestamp_in_data);

    if(format == DATASOURCE_DATATABLE_JSONP) {
        if(google_timestamp < last_timestamp_in_data)
            buffer_strcat(w->response.data, "});");

        else {
            // the client already has the latest data
            buffer_flush(w->response.data);
            buffer_sprintf(w->response.data,
                           "%s({version:'%s',reqId:'%s',status:'error',errors:[{reason:'not_modified',message:'Data not modified'}]});",
                           responseHandler, google_version, google_reqId);
        }
    }
    else if(format == DATASOURCE_JSONP)
        buffer_strcat(w->response.data, ");");

    if(qt->internal.relative)
        buffer_no_cacheable(w->response.data);
    else
        buffer_cacheable(w->response.data);

cleanup:
    query_target_release(qt);
    onewayalloc_destroy(owa);
    buffer_free(dimensions);
    return ret;
}

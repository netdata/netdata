// SPDX-License-Identifier: GPL-3.0-or-later

#include "web_api_v2.h"

static int web_client_api_request_v2_contexts_internal(RRDHOST *host __maybe_unused, struct web_client *w, char *url, CONTEXTS_V2_OPTIONS options) {
    struct api_v2_contexts_request req = { 0 };
    req.timings.received_ut = now_monotonic_usec();

    while(url) {
        char *value = mystrsep(&url, "&");
        if(!value || !*value) continue;

        char *name = mystrsep(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        // name and value are now the parameters
        // they are not null and not empty

        if(!strcmp(name, "scope_nodes")) req.scope_nodes = value;
        else if((options & (CONTEXTS_V2_NODES | CONTEXTS_V2_CONTEXTS)) && !strcmp(name, "nodes")) req.nodes = value;
        else if((options & CONTEXTS_V2_CONTEXTS) && !strcmp(name, "scope_contexts")) req.scope_contexts = value;
        else if((options & CONTEXTS_V2_CONTEXTS) && !strcmp(name, "contexts")) req.contexts = value;
        else if((options & CONTEXTS_V2_SEARCH) && !strcmp(name, "q")) req.q = value;
    }

    options |= CONTEXTS_V2_DEBUG;

    buffer_flush(w->response.data);
    buffer_no_cacheable(w->response.data);
    return rrdcontext_to_json_v2(w->response.data, &req, options);
}

static int web_client_api_request_v2_q(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    return web_client_api_request_v2_contexts_internal(host, w, url, CONTEXTS_V2_SEARCH | CONTEXTS_V2_CONTEXTS | CONTEXTS_V2_NODES);
}

static int web_client_api_request_v2_contexts(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    return web_client_api_request_v2_contexts_internal(host, w, url, CONTEXTS_V2_CONTEXTS);
}

static int web_client_api_request_v2_nodes(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    return web_client_api_request_v2_contexts_internal(host, w, url, CONTEXTS_V2_NODES);
}

static int web_client_api_request_v2_data(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    usec_t received_ut = now_monotonic_usec();

    int ret = HTTP_RESP_BAD_REQUEST;

    buffer_flush(w->response.data);

    char    *google_version = "0.6",
            *google_reqId = "0",
            *google_sig = "0",
            *google_out = "json",
            *responseHandler = NULL,
            *outFileName = NULL;

    time_t last_timestamp_in_data = 0, google_timestamp = 0;

    char *scope_nodes = NULL;
    char *scope_contexts = NULL;
    char *nodes = NULL;
    char *contexts = NULL;
    char *instances = NULL;
    char *dimensions = NULL;
    char *before_str = NULL;
    char *after_str = NULL;
    char *resampling_time_str = NULL;
    char *points_str = NULL;
    char *timeout_str = NULL;
    char *labels = NULL;
    char *alerts = NULL;
    char *time_group_options = NULL;
    char *tier_str = NULL;
    char *group_by_label = NULL;
    size_t tier = 0;
    RRDR_TIME_GROUPING time_group = RRDR_GROUPING_AVERAGE;
    RRDR_GROUP_BY group_by = RRDR_GROUP_BY_DIMENSION;
    RRDR_GROUP_BY_FUNCTION group_by_aggregate = RRDR_GROUP_BY_FUNCTION_AVERAGE;
    DATASOURCE_FORMAT format = DATASOURCE_JSON;
    RRDR_OPTIONS options = RRDR_OPTION_VIRTUAL_POINTS | RRDR_OPTION_JSON_WRAP | RRDR_OPTION_RETURN_JWAR;

    while(url) {
        char *value = mystrsep(&url, "&");
        if(!value || !*value) continue;

        char *name = mystrsep(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        // name and value are now the parameters
        // they are not null and not empty

        if(!strcmp(name, "scope_nodes")) scope_nodes = value;
        else if(!strcmp(name, "scope_contexts")) scope_contexts = value;
        else if(!strcmp(name, "nodes")) nodes = value;
        else if(!strcmp(name, "contexts")) contexts = value;
        else if(!strcmp(name, "instances")) instances = value;
        else if(!strcmp(name, "dimensions")) dimensions = value;
        else if(!strcmp(name, "labels")) labels = value;
        else if(!strcmp(name, "alerts")) alerts = value;
        else if(!strcmp(name, "after")) after_str = value;
        else if(!strcmp(name, "before")) before_str = value;
        else if(!strcmp(name, "points")) points_str = value;
        else if(!strcmp(name, "timeout")) timeout_str = value;
        else if(!strcmp(name, "group_by")) group_by = group_by_parse(value);
        else if(!strcmp(name, "group_by_label")) group_by_label = value;
        else if(!strcmp(name, "aggregation")) group_by_aggregate = group_by_aggregate_function_parse(value);
        else if(!strcmp(name, "format")) format = web_client_api_request_v1_data_format(value);
        else if(!strcmp(name, "options")) options |= web_client_api_request_v1_data_options(value);
        else if(!strcmp(name, "time_group")) time_group = time_grouping_parse(value, RRDR_GROUPING_AVERAGE);
        else if(!strcmp(name, "time_group_options")) time_group_options = value;
        else if(!strcmp(name, "time_resampling")) resampling_time_str = value;
        else if(!strcmp(name, "tier")) tier_str = value;
        else if(!strcmp(name, "callback")) responseHandler = value;
        else if(!strcmp(name, "filename")) outFileName = value;
        else if(!strcmp(name, "tqx")) {
            // parse Google Visualization API options
            // https://developers.google.com/chart/interactive/docs/dev/implementing_data_source
            char *tqx_name, *tqx_value;

            while(value) {
                tqx_value = mystrsep(&value, ";");
                if(!tqx_value || !*tqx_value) continue;

                tqx_name = mystrsep(&tqx_value, ":");
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
                    format = web_client_api_request_v1_data_google_format(google_out);
                }
                else if(!strcmp(tqx_name, "responseHandler"))
                    responseHandler = tqx_value;
                else if(!strcmp(tqx_name, "outFileName"))
                    outFileName = tqx_value;
            }
        }
    }

    // validate the google parameters given
    fix_google_param(google_out);
    fix_google_param(google_sig);
    fix_google_param(google_reqId);
    fix_google_param(google_version);
    fix_google_param(responseHandler);
    fix_google_param(outFileName);

    if(group_by_label && *group_by_label)
        group_by |= RRDR_GROUP_BY_LABEL;

    if(group_by == RRDR_GROUP_BY_NONE)
        group_by = RRDR_GROUP_BY_DIMENSION;

    if(group_by & RRDR_GROUP_BY_SELECTED)
        group_by = RRDR_GROUP_BY_SELECTED; // remove all other groupings

    if(group_by & ~(RRDR_GROUP_BY_DIMENSION))
        options |= RRDR_OPTION_ABSOLUTE;

    if(options & RRDR_OPTION_DEBUG)
        options &= ~RRDR_OPTION_MINIFY;

    if(tier_str && *tier_str) {
        tier = str2ul(tier_str);
        if(tier < storage_tiers)
            options |= RRDR_OPTION_SELECTED_TIER;
        else
            tier = 0;
    }

    long long before = (before_str && *before_str)?str2l(before_str):0;
    long long after  = (after_str  && *after_str) ?str2l(after_str):-600;
    int       points = (points_str && *points_str)?str2i(points_str):0;
    int       timeout = (timeout_str && *timeout_str)?str2i(timeout_str): 0;
    long      group_time = (resampling_time_str && *resampling_time_str) ? str2l(resampling_time_str) : 0;

    QUERY_TARGET_REQUEST qtr = {
            .version = 2,
            .scope_nodes = scope_nodes,
            .scope_contexts = scope_contexts,
            .after = after,
            .before = before,
            .host = NULL,
            .st = NULL,
            .nodes = nodes,
            .contexts = contexts,
            .instances = instances,
            .dimensions = dimensions,
            .alerts = alerts,
            .timeout = timeout,
            .points = points,
            .format = format,
            .options = options,
            .group_by = group_by,
            .group_by_label = group_by_label,
            .group_by_aggregate_function = group_by_aggregate,
            .time_group_method = time_group,
            .time_group_options = time_group_options,
            .resampling_time = group_time,
            .tier = tier,
            .chart_label_key = NULL,
            .labels = labels,
            .query_source = QUERY_SOURCE_API_DATA,
            .priority = STORAGE_PRIORITY_NORMAL,
            .received_ut = received_ut,
    };
    QUERY_TARGET *qt = query_target_create(&qtr);
    ONEWAYALLOC *owa = NULL;

    if(!qt) {
        buffer_sprintf(w->response.data, "Failed to prepare the query.");
        ret = HTTP_RESP_INTERNAL_SERVER_ERROR;
        goto cleanup;
    }

    if (timeout) {
        struct timeval now;
        now_realtime_timeval(&now);
        int inqueue = (int)dt_usec(&w->tv_in, &now) / 1000;
        timeout -= inqueue;
        if (timeout <= 0) {
            buffer_flush(w->response.data);
            buffer_strcat(w->response.data, "Query timeout exceeded");
            ret = HTTP_RESP_BACKEND_FETCH_FAILED;
            goto cleanup;
        }
    }

    if(outFileName && *outFileName) {
        buffer_sprintf(w->response.header, "Content-Disposition: attachment; filename=\"%s\"\r\n", outFileName);
        debug(D_WEB_CLIENT, "%llu: generating outfilename header: '%s'", w->id, outFileName);
    }

    if(format == DATASOURCE_DATATABLE_JSONP) {
        if(responseHandler == NULL)
            responseHandler = "google.visualization.Query.setResponse";

        debug(D_WEB_CLIENT_ACCESS, "%llu: GOOGLE JSON/JSONP: version = '%s', reqId = '%s', sig = '%s', out = '%s', responseHandler = '%s', outFileName = '%s'",
              w->id, google_version, google_reqId, google_sig, google_out, responseHandler, outFileName
        );

        buffer_sprintf(
                w->response.data,
                "%s({version:'%s',reqId:'%s',status:'ok',sig:'%"PRId64"',table:",
                responseHandler,
                google_version,
                google_reqId,
                (int64_t)now_realtime_sec());
    }
    else if(format == DATASOURCE_JSONP) {
        if(responseHandler == NULL)
            responseHandler = "callback";

        buffer_strcat(w->response.data, responseHandler);
        buffer_strcat(w->response.data, "(");
    }

    owa = onewayalloc_create(0);
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

cleanup:
    query_target_release(qt);
    onewayalloc_destroy(owa);
    return ret;
}



static struct web_api_command api_commands_v2[] = {
        {"data", 0, WEB_CLIENT_ACL_DASHBOARD | WEB_CLIENT_ACL_ACLK, web_client_api_request_v2_data},
        {"nodes", 0, WEB_CLIENT_ACL_DASHBOARD | WEB_CLIENT_ACL_ACLK, web_client_api_request_v2_nodes},
        {"contexts", 0, WEB_CLIENT_ACL_DASHBOARD | WEB_CLIENT_ACL_ACLK, web_client_api_request_v2_contexts},
        {"q", 0, WEB_CLIENT_ACL_DASHBOARD | WEB_CLIENT_ACL_ACLK, web_client_api_request_v2_q},

        // terminator
        {NULL, 0, WEB_CLIENT_ACL_NONE, NULL},
};

inline int web_client_api_request_v2(RRDHOST *host, struct web_client *w, char *url) {
    static int initialized = 0;

    if(unlikely(initialized == 0)) {
        initialized = 1;

        for(int i = 0; api_commands_v2[i].command ; i++)
            api_commands_v2[i].hash = simple_hash(api_commands_v2[i].command);
    }

    return web_client_api_request_vX(host, w, url, api_commands_v2);
}

// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v2_calls.h"

#define GROUP_BY_KEY_MAX_LENGTH 30
static struct {
    char group_by[GROUP_BY_KEY_MAX_LENGTH + 1];
    char aggregation[GROUP_BY_KEY_MAX_LENGTH + 1];
    char group_by_label[GROUP_BY_KEY_MAX_LENGTH + 1];
} group_by_keys[MAX_QUERY_GROUP_BY_PASSES];

__attribute__((constructor)) void initialize_group_by_keys(void) {
    for(size_t g = 0; g < MAX_QUERY_GROUP_BY_PASSES ;g++) {
        snprintfz(group_by_keys[g].group_by, GROUP_BY_KEY_MAX_LENGTH, "group_by[%zu]", g);
        snprintfz(group_by_keys[g].aggregation, GROUP_BY_KEY_MAX_LENGTH, "aggregation[%zu]", g);
        snprintfz(group_by_keys[g].group_by_label, GROUP_BY_KEY_MAX_LENGTH, "group_by_label[%zu]", g);
    }
}

int api_v2_data(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
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
    size_t tier = 0;
    RRDR_TIME_GROUPING time_group = RRDR_GROUPING_AVERAGE;
    DATASOURCE_FORMAT format = DATASOURCE_JSON2;
    RRDR_OPTIONS options = RRDR_OPTION_VIRTUAL_POINTS | RRDR_OPTION_JSON_WRAP | RRDR_OPTION_RETURN_JWAR;

    struct group_by_pass group_by[MAX_QUERY_GROUP_BY_PASSES] = {
        {
            .group_by = RRDR_GROUP_BY_DIMENSION,
            .group_by_label = NULL,
            .aggregation = RRDR_GROUP_BY_FUNCTION_AVERAGE,
        },
    };

    size_t group_by_idx = 0, group_by_label_idx = 0, aggregation_idx = 0;

    while(url) {
        char *value = strsep_skip_consecutive_separators(&url, "&");
        if(!value || !*value) continue;

        char *name = strsep_skip_consecutive_separators(&value, "=");
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
        else if(!strcmp(name, "group_by")) {
            group_by[group_by_idx++].group_by = group_by_parse(value);
            if(group_by_idx >= MAX_QUERY_GROUP_BY_PASSES)
                group_by_idx = MAX_QUERY_GROUP_BY_PASSES - 1;
        }
        else if(!strcmp(name, "group_by_label")) {
            group_by[group_by_label_idx++].group_by_label = value;
            if(group_by_label_idx >= MAX_QUERY_GROUP_BY_PASSES)
                group_by_label_idx = MAX_QUERY_GROUP_BY_PASSES - 1;
        }
        else if(!strcmp(name, "aggregation")) {
            group_by[aggregation_idx++].aggregation = group_by_aggregate_function_parse(value);
            if(aggregation_idx >= MAX_QUERY_GROUP_BY_PASSES)
                aggregation_idx = MAX_QUERY_GROUP_BY_PASSES - 1;
        }
        else if(!strcmp(name, "format")) format = datasource_format_str_to_id(value);
        else if(!strcmp(name, "options")) options |= rrdr_options_parse(value);
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
        else {
            for(size_t g = 0; g < MAX_QUERY_GROUP_BY_PASSES ;g++) {
                if(!strcmp(name, group_by_keys[g].group_by))
                    group_by[g].group_by = group_by_parse(value);
                else if(!strcmp(name, group_by_keys[g].group_by_label))
                    group_by[g].group_by_label = value;
                else if(!strcmp(name, group_by_keys[g].aggregation))
                    group_by[g].aggregation = group_by_aggregate_function_parse(value);
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

    for(size_t g = 0; g < MAX_QUERY_GROUP_BY_PASSES ;g++) {
        if (group_by[g].group_by_label && *group_by[g].group_by_label)
            group_by[g].group_by |= RRDR_GROUP_BY_LABEL;
    }

    if(group_by[0].group_by == RRDR_GROUP_BY_NONE)
        group_by[0].group_by = RRDR_GROUP_BY_DIMENSION;

    for(size_t g = 0; g < MAX_QUERY_GROUP_BY_PASSES ;g++) {
        if ((group_by[g].group_by != RRDR_GROUP_BY_NONE && !(group_by[g].group_by & RRDR_GROUP_BY_DIMENSION)) ||
            (options & RRDR_OPTION_PERCENTAGE)) {
            options |= RRDR_OPTION_ABSOLUTE;
            break;
        }
    }

    if(options & RRDR_OPTION_DEBUG)
        options &= ~RRDR_OPTION_MINIFY;

    if(tier_str && *tier_str) {
        tier = str2ul(tier_str);
        if(tier < nd_profile.storage_tiers)
            options |= RRDR_OPTION_SELECTED_TIER;
        else
            tier = 0;
    }

    time_t    before = (before_str && *before_str)?str2l(before_str):0;
    time_t    after  = (after_str  && *after_str) ?str2l(after_str):-600;
    size_t    points = (points_str && *points_str)?str2u(points_str):0;
    int       timeout = (timeout_str && *timeout_str)?str2i(timeout_str): 0;
    time_t    resampling_time = (resampling_time_str && *resampling_time_str) ? str2l(resampling_time_str) : 0;

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
        .timeout_ms = timeout,
        .points = points,
        .format = format,
        .options = options,
        .time_group_method = time_group,
        .time_group_options = time_group_options,
        .resampling_time = resampling_time,
        .tier = tier,
        .chart_label_key = NULL,
        .labels = labels,
        .query_source = QUERY_SOURCE_API_DATA,
        .priority = STORAGE_PRIORITY_NORMAL,
        .received_ut = received_ut,

        .interrupt_callback = web_client_interrupt_callback,
        .interrupt_callback_data = w,

        .transaction = &w->transaction,
    };

    for(size_t g = 0; g < MAX_QUERY_GROUP_BY_PASSES ;g++)
        qtr.group_by[g] = group_by[g];

    QUERY_TARGET *qt = query_target_create(&qtr);
    ONEWAYALLOC *owa = NULL;

    if(!qt) {
        buffer_sprintf(w->response.data, "Failed to prepare the query.");
        ret = HTTP_RESP_INTERNAL_SERVER_ERROR;
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

    if(qt->internal.relative)
        buffer_no_cacheable(w->response.data);
    else
        buffer_cacheable(w->response.data);

cleanup:
    query_target_release(qt);
    onewayalloc_destroy(owa);
    return ret;
}

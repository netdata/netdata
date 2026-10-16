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

static int api_v23_data_internal(RRDHOST *host __maybe_unused, struct web_client *w, char *url, size_t version) {
    usec_t received_ut = now_monotonic_usec();

    int ret = HTTP_RESP_BAD_REQUEST;

    buffer_flush(w->response.data);

    DATA_QUERY_JSONP jp = DATA_QUERY_JSONP_DEFAULTS;

    char *scope_nodes = NULL;
    char *scope_contexts = NULL;
    char *scope_instances = NULL;
    char *scope_labels = NULL;
    char *scope_dimensions = NULL;
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
    char *cardinality_limit_str = NULL;
    char *limit_str = NULL;
    size_t tier = 0;
    size_t cardinality_limit = 0;
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
        else if(!strcmp(name, "scope_instances")) scope_instances = value;
        else if(!strcmp(name, "scope_labels")) scope_labels = value;
        else if(!strcmp(name, "scope_dimensions")) scope_dimensions = value;
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
        else if(!strcmp(name, "cardinality_limit")) cardinality_limit_str = value;
        else if(!strcmp(name, "limit")) limit_str = value;
        else if(!strcmp(name, "callback")) jp.responseHandler = value;
        else if(!strcmp(name, "filename")) jp.outFileName = value;
        else if(!strcmp(name, "tqx")) data_query_jsonp_parse_tqx(&jp, value, &format);
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
    data_query_jsonp_sanitize(&jp);

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
    
    if(cardinality_limit_str && *cardinality_limit_str) {
        cardinality_limit = str2ul(cardinality_limit_str);
    }
    else if(limit_str && *limit_str) {
        // limit=N means "return at most N dimensions"; the engine reserves
        // one extra slot for the 'remaining N dimensions' aggregate, so the
        // response carries the top-N plus the folded tail. Agents without
        // this parameter ignore it, which lets clients use it without
        // version gating.
        size_t limit = str2ul(limit_str);
        if(limit)
            cardinality_limit = (limit < SIZE_MAX) ? limit + 1 : limit;
    }

    time_t    before = (before_str && *before_str)?str2l(before_str):0;
    time_t    after  = (after_str  && *after_str) ?str2l(after_str):-600;
    size_t    points = (points_str && *points_str)?str2u(points_str):0;
    int       timeout = (timeout_str && *timeout_str)?str2i(timeout_str): 0;
    time_t    resampling_time = (resampling_time_str && *resampling_time_str) ? str2l(resampling_time_str) : 0;

    QUERY_TARGET_REQUEST qtr = {
        .version = version,
        .scope_nodes = scope_nodes,
        .scope_contexts = scope_contexts,
        .scope_instances = scope_instances,
        .scope_labels = scope_labels,
        .scope_dimensions = scope_dimensions,
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
        .cardinality_limit = cardinality_limit,

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

    int cancelled_code = query_target_cancelled_response(qt, w->response.data);
    if(cancelled_code) {
        ret = cancelled_code;
        goto cleanup;
    }

    ret = data_query_response_begin(w, &jp, format, timeout, now_realtime_sec());
    if(ret)
        goto cleanup;

    owa = onewayalloc_create(0);
    ret = data_query_response_execute(owa, w, qt, &jp, format);

cleanup:
    query_target_release(qt);
    onewayalloc_destroy(owa);
    return ret;
}

int api_v2_data(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    return api_v23_data_internal(host, w, url, 2);
}

int api_v3_data(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    return api_v23_data_internal(host, w, url, 3);
}

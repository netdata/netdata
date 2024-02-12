// SPDX-License-Identifier: GPL-3.0-or-later

#include "web_api_v2.h"
#include "../rtc/webrtc.h"

static bool verify_agent_uuids(const char *machine_guid, const char *node_id, const char *claim_id) {
    if(!machine_guid || !node_id || !claim_id)
        return false;

    if(strcmp(machine_guid, localhost->machine_guid) != 0)
        return false;

    char *agent_claim_id = get_agent_claimid();

    bool not_verified = (!agent_claim_id || strcmp(claim_id, agent_claim_id) != 0);
    freez(agent_claim_id);

    if(not_verified || !localhost->node_id)
        return false;

    char buf[UUID_STR_LEN];
    uuid_unparse_lower(*localhost->node_id, buf);

    if(strcmp(node_id, buf) != 0)
        return false;

    return true;
}

int api_v2_bearer_protection(RRDHOST *host __maybe_unused, struct web_client *w __maybe_unused, char *url) {
    char *machine_guid = NULL;
    char *claim_id = NULL;
    char *node_id = NULL;
    bool protection = netdata_is_protected_by_bearer;

    while (url) {
        char *value = strsep_skip_consecutive_separators(&url, "&");
        if (!value || !*value) continue;

        char *name = strsep_skip_consecutive_separators(&value, "=");
        if (!name || !*name) continue;
        if (!value || !*value) continue;

        if(!strcmp(name, "bearer_protection")) {
            if(!strcmp(value, "on") || !strcmp(value, "true") || !strcmp(value, "yes"))
                protection = true;
            else
                protection = false;
        }
        else if(!strcmp(name, "machine_guid"))
            machine_guid = value;
        else if(!strcmp(name, "claim_id"))
            claim_id = value;
        else if(!strcmp(name, "node_id"))
            node_id = value;
    }

    if(!verify_agent_uuids(machine_guid, node_id, claim_id)) {
        buffer_flush(w->response.data);
        buffer_strcat(w->response.data, "The request is missing or not matching local UUIDs");
        return HTTP_RESP_BAD_REQUEST;
    }

    netdata_is_protected_by_bearer = protection;

    BUFFER *wb = w->response.data;
    buffer_flush(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
    buffer_json_member_add_boolean(wb, "bearer_protection", netdata_is_protected_by_bearer);
    buffer_json_finalize(wb);

    return HTTP_RESP_OK;
}

int api_v2_bearer_token(RRDHOST *host __maybe_unused, struct web_client *w __maybe_unused, char *url __maybe_unused) {
    char *machine_guid = NULL;
    char *claim_id = NULL;
    char *node_id = NULL;

    while(url) {
        char *value = strsep_skip_consecutive_separators(&url, "&");
        if (!value || !*value) continue;

        char *name = strsep_skip_consecutive_separators(&value, "=");
        if (!name || !*name) continue;
        if (!value || !*value) continue;

        if(!strcmp(name, "machine_guid"))
            machine_guid = value;
        else if(!strcmp(name, "claim_id"))
            claim_id = value;
        else if(!strcmp(name, "node_id"))
            node_id = value;
    }

    if(!verify_agent_uuids(machine_guid, node_id, claim_id)) {
        buffer_flush(w->response.data);
        buffer_strcat(w->response.data, "The request is missing or not matching local UUIDs");
        return HTTP_RESP_BAD_REQUEST;
    }

    uuid_t uuid;
    time_t expires_s = bearer_create_token(&uuid, w);

    BUFFER *wb = w->response.data;
    buffer_flush(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
    buffer_json_member_add_string(wb, "mg", localhost->machine_guid);
    buffer_json_member_add_boolean(wb, "bearer_protection", netdata_is_protected_by_bearer);
    buffer_json_member_add_uuid(wb, "token", &uuid);
    buffer_json_member_add_time_t(wb, "expiration", expires_s);
    buffer_json_finalize(wb);

    return HTTP_RESP_OK;
}

static int web_client_api_request_v2_contexts_internal(RRDHOST *host __maybe_unused, struct web_client *w, char *url, CONTEXTS_V2_MODE mode) {
    struct api_v2_contexts_request req = { 0 };

    while(url) {
        char *value = strsep_skip_consecutive_separators(&url, "&");
        if(!value || !*value) continue;

        char *name = strsep_skip_consecutive_separators(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        // name and value are now the parameters
        // they are not null and not empty

        if(!strcmp(name, "scope_nodes"))
            req.scope_nodes = value;
        else if(!strcmp(name, "nodes"))
            req.nodes = value;
        else if((mode & (CONTEXTS_V2_CONTEXTS | CONTEXTS_V2_SEARCH | CONTEXTS_V2_ALERTS | CONTEXTS_V2_ALERT_TRANSITIONS)) && !strcmp(name, "scope_contexts"))
            req.scope_contexts = value;
        else if((mode & (CONTEXTS_V2_CONTEXTS | CONTEXTS_V2_SEARCH | CONTEXTS_V2_ALERTS | CONTEXTS_V2_ALERT_TRANSITIONS)) && !strcmp(name, "contexts"))
            req.contexts = value;
        else if((mode & CONTEXTS_V2_SEARCH) && !strcmp(name, "q"))
            req.q = value;
        else if(!strcmp(name, "options"))
            req.options = web_client_api_request_v2_context_options(value);
        else if(!strcmp(name, "after"))
            req.after = str2l(value);
        else if(!strcmp(name, "before"))
            req.before = str2l(value);
        else if(!strcmp(name, "timeout"))
            req.timeout_ms = str2l(value);
        else if(mode & (CONTEXTS_V2_ALERTS | CONTEXTS_V2_ALERT_TRANSITIONS)) {
            if (!strcmp(name, "alert"))
                req.alerts.alert = value;
            else if (!strcmp(name, "transition"))
                req.alerts.transition = value;
            else if(mode & CONTEXTS_V2_ALERTS) {
                if (!strcmp(name, "status"))
                    req.alerts.status = web_client_api_request_v2_alert_status(value);
            }
            else if(mode & CONTEXTS_V2_ALERT_TRANSITIONS) {
                if (!strcmp(name, "last"))
                    req.alerts.last = strtoul(value, NULL, 0);
                else if(!strcmp(name, "context"))
                    req.contexts = value;
                else if (!strcmp(name, "anchor_gi")) {
                    req.alerts.global_id_anchor = str2ull(value, NULL);
                }
                else {
                    for(int i = 0; i < ATF_TOTAL_ENTRIES ;i++) {
                        if(!strcmp(name, alert_transition_facets[i].query_param))
                            req.alerts.facets[i] = value;
                    }
                }
            }
        }
    }

    if ((mode & CONTEXTS_V2_ALERT_TRANSITIONS) && !req.alerts.last)
        req.alerts.last = 1;

    buffer_flush(w->response.data);
    buffer_no_cacheable(w->response.data);
    return rrdcontext_to_json_v2(w->response.data, &req, mode);
}

static int web_client_api_request_v2_alert_transitions(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    return web_client_api_request_v2_contexts_internal(host, w, url, CONTEXTS_V2_ALERT_TRANSITIONS | CONTEXTS_V2_NODES);
}

static int web_client_api_request_v2_alerts(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    return web_client_api_request_v2_contexts_internal(host, w, url, CONTEXTS_V2_ALERTS | CONTEXTS_V2_NODES);
}

static int web_client_api_request_v2_functions(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    return web_client_api_request_v2_contexts_internal(host, w, url, CONTEXTS_V2_FUNCTIONS | CONTEXTS_V2_NODES | CONTEXTS_V2_AGENTS | CONTEXTS_V2_VERSIONS);
}

static int web_client_api_request_v2_versions(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    return web_client_api_request_v2_contexts_internal(host, w, url, CONTEXTS_V2_VERSIONS);
}

static int web_client_api_request_v2_q(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    return web_client_api_request_v2_contexts_internal(host, w, url, CONTEXTS_V2_SEARCH | CONTEXTS_V2_CONTEXTS | CONTEXTS_V2_NODES | CONTEXTS_V2_AGENTS | CONTEXTS_V2_VERSIONS);
}

static int web_client_api_request_v2_contexts(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    return web_client_api_request_v2_contexts_internal(host, w, url, CONTEXTS_V2_CONTEXTS | CONTEXTS_V2_NODES | CONTEXTS_V2_AGENTS | CONTEXTS_V2_VERSIONS);
}

static int web_client_api_request_v2_nodes(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    return web_client_api_request_v2_contexts_internal(host, w, url, CONTEXTS_V2_NODES | CONTEXTS_V2_NODES_INFO);
}

static int web_client_api_request_v2_info(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    return web_client_api_request_v2_contexts_internal(host, w, url, CONTEXTS_V2_AGENTS | CONTEXTS_V2_AGENTS_INFO);
}

static int web_client_api_request_v2_node_instances(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    return web_client_api_request_v2_contexts_internal(host, w, url, CONTEXTS_V2_NODES | CONTEXTS_V2_NODE_INSTANCES | CONTEXTS_V2_AGENTS | CONTEXTS_V2_AGENTS_INFO | CONTEXTS_V2_VERSIONS);
}

static int web_client_api_request_v2_weights(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    return web_client_api_request_weights(host, w, url, WEIGHTS_METHOD_VALUE, WEIGHTS_FORMAT_MULTINODE, 2);
}

static int web_client_api_request_v2_claim(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    return api_v2_claim(w, url);
}

static int web_client_api_request_v2_alert_config(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    const char *config = NULL;

    while(url) {
        char *value = strsep_skip_consecutive_separators(&url, "&");
        if(!value || !*value) continue;

        char *name = strsep_skip_consecutive_separators(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        // name and value are now the parameters
        // they are not null and not empty

        if(!strcmp(name, "config"))
            config = value;
    }

    buffer_flush(w->response.data);

    if(!config) {
        w->response.data->content_type = CT_TEXT_PLAIN;
        buffer_strcat(w->response.data, "A config hash ID is required. Add ?config=UUID query param");
        return HTTP_RESP_BAD_REQUEST;
    }

    return contexts_v2_alert_config_to_json(w, config);
}


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
        else if(!strcmp(name, "format")) format = web_client_api_request_v1_data_format(value);
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
                    format = web_client_api_request_v1_data_google_format(google_out);
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
        if ((group_by[g].group_by & ~(RRDR_GROUP_BY_DIMENSION)) || (options & RRDR_OPTION_PERCENTAGE)) {
            options |= RRDR_OPTION_ABSOLUTE;
            break;
        }
    }

    if(options & RRDR_OPTION_DEBUG)
        options &= ~RRDR_OPTION_MINIFY;

    if(tier_str && *tier_str) {
        tier = str2ul(tier_str);
        if(tier < storage_tiers)
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

static int web_client_api_request_v2_webrtc(RRDHOST *host __maybe_unused, struct web_client *w, char *url __maybe_unused) {
    return webrtc_new_connection(buffer_tostring(w->payload), w->response.data);
}

static int web_client_api_request_v2_progress(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    char *transaction = NULL;

    while(url) {
        char *value = strsep_skip_consecutive_separators(&url, "&");
        if(!value || !*value) continue;

        char *name = strsep_skip_consecutive_separators(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        // name and value are now the parameters
        // they are not null and not empty

        if(!strcmp(name, "transaction")) transaction = value;
    }

    uuid_t tr;
    uuid_parse_flexi(transaction, tr);

    rrd_function_call_progresser(&tr);

    return web_api_v2_report_progress(&tr, w->response.data);
}

static struct web_api_command api_commands_v2[] = {
    // time-series multi-node multi-instance data APIs
    {
        .api = "data",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v2_data,
        .allow_subpaths = 0
    },
    {
        .api = "weights",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v2_weights,
        .allow_subpaths = 0
    },

    // time-series multi-node multi-instance metadata APIs
    {
        .api = "contexts",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v2_contexts,
        .allow_subpaths = 0
    },
    {
        // full text search
        .api = "q",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v2_q,
        .allow_subpaths = 0
    },

    // multi-node multi-instance alerts APIs
    {
        .api = "alerts",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v2_alerts,
        .allow_subpaths = 0
    },
    {
        .api = "alert_transitions",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v2_alert_transitions,
        .allow_subpaths = 0
    },
    {
        .api = "alert_config",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v2_alert_config,
        .allow_subpaths = 0
    },

    // agent information APIs
    {
        .api = "info",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v2_info,
        .allow_subpaths = 0
    },
    {
        .api = "nodes",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v2_nodes,
        .allow_subpaths = 0
    },
        {
        .api = "node_instances",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v2_node_instances,
        .allow_subpaths = 0
    },
    {
        .api = "versions",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v2_versions,
        .allow_subpaths = 0
    },
    {
        .api = "progress",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v2_progress,
        .allow_subpaths = 0
    },

    // functions APIs
    {
        .api = "functions",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v2_functions,
        .allow_subpaths = 0
    },

    // WebRTC APIs
    {
        .api = "rtc_offer",
        .hash = 0,
        .acl = HTTP_ACL_ACLK | ACL_DEV_OPEN_ACCESS,
        .access = HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE,
        .callback = web_client_api_request_v2_webrtc,
        .allow_subpaths = 0
    },

    // management APIs
    {
        .api = "claim",
        .hash = 0,
        .acl = HTTP_ACL_NOCHECK,
        .access = HTTP_ACCESS_NONE,
        .callback = web_client_api_request_v2_claim,
        .allow_subpaths = 0
    },
    {
        .api = "bearer_protection",
        .hash = 0,
        .acl = HTTP_ACL_ACLK | ACL_DEV_OPEN_ACCESS,
        .access = HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_VIEW_AGENT_CONFIG | HTTP_ACCESS_EDIT_AGENT_CONFIG,
        .callback = api_v2_bearer_protection,
        .allow_subpaths = 0
    },
    {
        .api = "bearer_get_token",
        .hash = 0,
        .acl = HTTP_ACL_ACLK | ACL_DEV_OPEN_ACCESS,
        .access = HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE,
        .callback = api_v2_bearer_token,
        .allow_subpaths = 0
    },

    // Netdata branding APIs
    {
        .api = "ilove.svg",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v2_ilove,
        .allow_subpaths = 0
    },

    {
        // terminator
        .api = NULL,
        .hash = 0,
        .acl = HTTP_ACL_NONE,
        .access = HTTP_ACCESS_NONE,
        .callback = NULL,
        .allow_subpaths = 0
    },
};

inline int web_client_api_request_v2(RRDHOST *host, struct web_client *w, char *url_path_endpoint) {
    static int initialized = 0;

    if(unlikely(initialized == 0)) {
        initialized = 1;

        for(int i = 0; api_commands_v2[i].api ; i++)
            api_commands_v2[i].hash = simple_hash(api_commands_v2[i].api);
    }

    return web_client_api_request_vX(host, w, url_path_endpoint, api_commands_v2);
}

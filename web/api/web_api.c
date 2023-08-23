// SPDX-License-Identifier: GPL-3.0-or-later

#include "web_api.h"

bool netdata_is_protected_by_bearer = false; // this is controlled by cloud, at the point the agent logs in - this should also be saved to /var/lib/netdata
DICTIONARY *netdata_authorized_bearers = NULL;

static short int web_client_check_acl_and_bearer(struct web_client *w, WEB_CLIENT_ACL endpoint_acl) {
    if(endpoint_acl == WEB_CLIENT_ACL_NONE || (endpoint_acl & WEB_CLIENT_ACL_NOCHECK))
        // the endpoint is totally public
        return HTTP_RESP_OK;

    bool acl_allows = w->acl & endpoint_acl;
    if(!acl_allows)
        // the channel we received the request from (w->acl) is not compatible with the endpoint
        return HTTP_RESP_FORBIDDEN;

    if(!netdata_is_protected_by_bearer && !(endpoint_acl & WEB_CLIENT_ACL_BEARER_REQUIRED))
        // bearer protection is not enabled and is not required by the endpoint
        return HTTP_RESP_OK;

    if(!(endpoint_acl & (WEB_CLIENT_ACL_BEARER_REQUIRED|WEB_CLIENT_ACL_BEARER_OPTIONAL)))
        // endpoint does not require a bearer
        return HTTP_RESP_OK;

    if((w->acl & (WEB_CLIENT_ACL_ACLK|WEB_CLIENT_ACL_WEBRTC)))
        // the request is coming from ACLK or WEBRTC (authorized already),
        return HTTP_RESP_OK;

    // at this point we need a bearer to serve the request
    // either because:
    //
    // 1. WEB_CLIENT_ACL_BEARER_REQUIRED, or
    // 2. netdata_is_protected_by_bearer == true
    //

    BEARER_STATUS t = api_check_bearer_token(w);
    if(t == BEARER_STATUS_AVAILABLE_AND_VALIDATED)
        // we have a valid bearer on the request
        return HTTP_RESP_OK;

    netdata_log_info("BEARER: bearer is required for request: code %d", t);

    return HTTP_RESP_PRECOND_FAIL;
}

int web_client_api_request_vX(RRDHOST *host, struct web_client *w, char *url_path_endpoint, struct web_api_command *api_commands) {
    buffer_no_cacheable(w->response.data);

    if(unlikely(!url_path_endpoint || !*url_path_endpoint)) {
        buffer_flush(w->response.data);
        buffer_sprintf(w->response.data, "Which API command?");
        return HTTP_RESP_BAD_REQUEST;
    }

    char *api_command = strchr(url_path_endpoint, '/');
    if (likely(api_command == NULL)) // only config command supports subpaths for now
        api_command = url_path_endpoint;
    else {
        size_t api_command_len = api_command - url_path_endpoint;
        api_command = callocz(1, api_command_len + 1);
        memcpy(api_command, url_path_endpoint, api_command_len);
    }

    uint32_t hash = simple_hash(api_command);

    for(int i = 0; api_commands[i].command ; i++) {
        if(unlikely(hash == api_commands[i].hash && !strcmp(api_command, api_commands[i].command))) {
            if(unlikely(!api_commands[i].allow_subpaths && api_command != url_path_endpoint)) {
                buffer_flush(w->response.data);
                buffer_sprintf(w->response.data, "API command '%s' does not support subpaths.", api_command);
                freez(api_command);
                return HTTP_RESP_BAD_REQUEST;
            }

            if (api_command != url_path_endpoint)
                freez(api_command);

            short int code = web_client_check_acl_and_bearer(w, api_commands[i].acl);
            if(code != HTTP_RESP_OK) {
                if(code == HTTP_RESP_FORBIDDEN)
                    return web_client_permission_denied(w);

                if(code == HTTP_RESP_PRECOND_FAIL)
                    return web_client_bearer_required(w);

                buffer_flush(w->response.data);
                buffer_sprintf(w->response.data, "Failed with code %d", code);
                w->response.code = code;
                return code;
            }

            char *query_string = (char *)buffer_tostring(w->url_query_string_decoded);

            if(*query_string == '?')
                query_string = &query_string[1];

            return api_commands[i].callback(host, w, query_string);
        }
    }

    if (api_command != url_path_endpoint)
        freez(api_command);

    buffer_flush(w->response.data);
    buffer_strcat(w->response.data, "Unsupported API command: ");
    buffer_strcat_htmlescape(w->response.data, url_path_endpoint);
    return HTTP_RESP_NOT_FOUND;
}

RRDCONTEXT_TO_JSON_OPTIONS rrdcontext_to_json_parse_options(char *o) {
    RRDCONTEXT_TO_JSON_OPTIONS options = RRDCONTEXT_OPTION_NONE;
    char *tok;

    while(o && *o && (tok = strsep_skip_consecutive_separators(&o, ", |"))) {
        if(!*tok) continue;

        if(!strcmp(tok, "full") || !strcmp(tok, "all"))
            options |= RRDCONTEXT_OPTIONS_ALL;
        else if(!strcmp(tok, "charts") || !strcmp(tok, "instances"))
            options |= RRDCONTEXT_OPTION_SHOW_INSTANCES;
        else if(!strcmp(tok, "dimensions") || !strcmp(tok, "metrics"))
            options |= RRDCONTEXT_OPTION_SHOW_METRICS;
        else if(!strcmp(tok, "queue"))
            options |= RRDCONTEXT_OPTION_SHOW_QUEUED;
        else if(!strcmp(tok, "flags"))
            options |= RRDCONTEXT_OPTION_SHOW_FLAGS;
        else if(!strcmp(tok, "uuids"))
            options |= RRDCONTEXT_OPTION_SHOW_UUIDS;
        else if(!strcmp(tok, "deleted"))
            options |= RRDCONTEXT_OPTION_SHOW_DELETED;
        else if(!strcmp(tok, "labels"))
            options |= RRDCONTEXT_OPTION_SHOW_LABELS;
        else if(!strcmp(tok, "deepscan"))
            options |= RRDCONTEXT_OPTION_DEEPSCAN;
        else if(!strcmp(tok, "hidden"))
            options |= RRDCONTEXT_OPTION_SHOW_HIDDEN;
    }

    return options;
}

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
            options |= web_client_api_request_v1_data_options(value);

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
            if(tier < storage_tiers)
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
    };

    return web_api_v12_weights(wb, &qwr);
}

bool web_client_interrupt_callback(void *data) {
    struct web_client *w = data;

    if(w->interrupt.callback)
        return w->interrupt.callback(w, w->interrupt.callback_data);

    return sock_has_output_error(w->ofd);
}

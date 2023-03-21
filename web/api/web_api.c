// SPDX-License-Identifier: GPL-3.0-or-later

#include "web_api.h"

int web_client_api_request_vX(RRDHOST *host, struct web_client *w, char *url, struct web_api_command *api_commands) {
    if(unlikely(!url || !*url)) {
        buffer_flush(w->response.data);
        buffer_sprintf(w->response.data, "Which API command?");
        return HTTP_RESP_BAD_REQUEST;
    }

    uint32_t hash = simple_hash(url);

    for(int i = 0; api_commands[i].command ; i++) {
        if(unlikely(hash == api_commands[i].hash && !strcmp(url, api_commands[i].command))) {
            if(unlikely(api_commands[i].acl != WEB_CLIENT_ACL_NOCHECK) && !(w->acl & api_commands[i].acl))
                return web_client_permission_denied(w);

            return api_commands[i].callback(host, w, (w->decoded_query_string + 1));
        }
    }

    buffer_flush(w->response.data);
    buffer_strcat(w->response.data, "Unsupported API command: ");
    buffer_strcat_htmlescape(w->response.data, url);
    return HTTP_RESP_NOT_FOUND;
}

RRDCONTEXT_TO_JSON_OPTIONS rrdcontext_to_json_parse_options(char *o) {
    RRDCONTEXT_TO_JSON_OPTIONS options = RRDCONTEXT_OPTION_NONE;
    char *tok;

    while(o && *o && (tok = mystrsep(&o, ", |"))) {
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
        return HTTP_RESP_BACKEND_FETCH_FAILED;

    time_t baseline_after = 0, baseline_before = 0, after = 0, before = 0;
    size_t points = 0;
    RRDR_OPTIONS options = 0;
    RRDR_TIME_GROUPING time_group_method = RRDR_GROUPING_AVERAGE;
    time_t timeout_ms = 0;
    size_t tier = 0;
    const char *time_group_options = NULL, *scope_contexts = NULL, *scope_nodes = NULL, *contexts = NULL, *nodes = NULL,
        *instances = NULL, *dimensions = NULL, *labels = NULL, *alerts = NULL;

    while (url) {
        char *value = mystrsep(&url, "&");
        if (!value || !*value)
            continue;

        char *name = mystrsep(&value, "=");
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
    return sock_has_output_error(w->ofd);
}
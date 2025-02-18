// SPDX-License-Identifier: GPL-3.0-or-later

#include "web_api.h"

void host_labels2json(RRDHOST *host, BUFFER *wb, const char *key) {
    buffer_json_member_add_object(wb, key);
    rrdlabels_to_buffer_json_members(host->rrdlabels, wb);
    buffer_json_object_close(wb);
}

int web_client_api_request_vX(RRDHOST *host, struct web_client *w, char *url_path_endpoint, struct web_api_command *api_commands) {
    buffer_no_cacheable(w->response.data);

    internal_fatal(web_client_flags_check_auth(w) && !(w->access & HTTP_ACCESS_SIGNED_ID),
                   "signed-in permission should be set, but is missing");

    internal_fatal(!web_client_flags_check_auth(w) && (w->access & HTTP_ACCESS_SIGNED_ID),
                   "signed-in permission is set, but it shouldn't");

#ifdef NETDATA_GOD_MODE
    web_client_set_permissions(w, HTTP_ACCESS_ALL, HTTP_USER_ROLE_ADMIN, WEB_CLIENT_FLAG_AUTH_GOD);
#else
    if(!web_client_flags_check_auth(w)) {
        web_client_set_permissions(
            w,
            (netdata_is_protected_by_bearer) ? HTTP_ACCESS_NONE : HTTP_ACCESS_ANONYMOUS_DATA,
            (netdata_is_protected_by_bearer) ? HTTP_USER_ROLE_NONE : HTTP_USER_ROLE_ANY,
            0);
    }
#endif

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

    for(int i = 0; api_commands[i].api ; i++) {
        if(unlikely(hash == api_commands[i].hash && !strcmp(api_command, api_commands[i].api))) {
            if(unlikely(!api_commands[i].allow_subpaths && api_command != url_path_endpoint)) {
                buffer_flush(w->response.data);
                buffer_sprintf(w->response.data, "API command '%s' does not support subpaths.", api_command);
                freez(api_command);
                return HTTP_RESP_BAD_REQUEST;
            }

            if (api_command != url_path_endpoint)
                freez(api_command);

            bool acl_allows = ((w->acl & api_commands[i].acl) == api_commands[i].acl) || (api_commands[i].acl & HTTP_ACL_NOCHECK);
            if(!acl_allows)
                return web_client_permission_denied_acl(w);

            bool permissions_allows =
                http_access_user_has_enough_access_level_for_endpoint(w->access, api_commands[i].access);
            if(!permissions_allows)
                return web_client_permission_denied(w);

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


bool web_client_interrupt_callback(void *data) {
    struct web_client *w = data;

    bool ret;
    if(w->interrupt.callback)
        ret = w->interrupt.callback(w, w->interrupt.callback_data);
    else
        ret = is_socket_closed(w->fd);

    return ret;
}

void nd_web_api_init(void) {
    contexts_alert_statuses_init();
    rrdr_options_init();
    contexts_options_init();
    datasource_formats_init();
    time_grouping_init();
}

void web_client_progress_functions_update(void *data, size_t done, size_t all) {
    // handle progress updates from the plugin
    struct web_client *w = data;
    query_progress_functions_update(&w->transaction, done, all);
}


// SPDX-License-Identifier: GPL-3.0-or-later

#include "web_api.h"

void host_labels2json(RRDHOST *host, BUFFER *wb, const char *key) {
    buffer_json_member_add_object(wb, key);
    rrdlabels_to_buffer_json_members(host->rrdlabels, wb);
    buffer_json_object_close(wb);
}

void data_query_jsonp_parse_tqx(DATA_QUERY_JSONP *jp, char *value, DATASOURCE_FORMAT *format) {
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
            jp->google_version = tqx_value;
        else if(!strcmp(tqx_name, "reqId"))
            jp->google_reqId = tqx_value;
        else if(!strcmp(tqx_name, "sig")) {
            jp->google_sig = tqx_value;
            jp->google_timestamp = strtoul(jp->google_sig, NULL, 0);
        }
        else if(!strcmp(tqx_name, "out")) {
            jp->google_out = tqx_value;
            *format = google_data_format_str_to_id(jp->google_out);
        }
        else if(!strcmp(tqx_name, "responseHandler"))
            jp->responseHandler = tqx_value;
        else if(!strcmp(tqx_name, "outFileName"))
            jp->outFileName = tqx_value;
    }
}

static void fix_google_param(char *s) {
    if(unlikely(!s || !*s)) return;

    for( ; *s ;s++) {
        if(!isalnum((uint8_t)*s) && *s != '.' && *s != '_' && *s != '-')
            *s = '_';
    }
}

void data_query_jsonp_sanitize(DATA_QUERY_JSONP *jp) {
    fix_google_param(jp->google_out);
    fix_google_param(jp->google_sig);
    fix_google_param(jp->google_reqId);
    fix_google_param(jp->google_version);
    fix_google_param(jp->responseHandler);
    fix_google_param(jp->outFileName);
}

// checks the request deadline, then writes the attachment header and the JSONP preamble
// returns 0 to continue, or the HTTP code to respond with
int data_query_response_begin(struct web_client *w, DATA_QUERY_JSONP *jp, DATASOURCE_FORMAT format, int timeout, time_t sig) {
    web_client_timeout_checkpoint_set(w, timeout);
    if(web_client_timeout_checkpoint_and_check(w, NULL))
        return w->response.code;

    if(jp->outFileName && *jp->outFileName) {
        buffer_sprintf(w->response.header, "Content-Disposition: attachment; filename=\"%s\"\r\n", jp->outFileName);
        netdata_log_debug(D_WEB_CLIENT, "%llu: generating outfilename header: '%s'", w->id, jp->outFileName);
    }

    if(format == DATASOURCE_DATATABLE_JSONP) {
        if(jp->responseHandler == NULL)
            jp->responseHandler = "google.visualization.Query.setResponse";

        netdata_log_debug(D_WEB_CLIENT_ACCESS, "%llu: GOOGLE JSON/JSONP: version = '%s', reqId = '%s', sig = '%s', out = '%s', responseHandler = '%s', outFileName = '%s'",
                          w->id, jp->google_version, jp->google_reqId, jp->google_sig, jp->google_out, jp->responseHandler, jp->outFileName
        );

        buffer_sprintf(
            w->response.data,
            "%s({version:'%s',reqId:'%s',status:'ok',sig:'%"PRId64"',table:",
            jp->responseHandler,
            jp->google_version,
            jp->google_reqId,
            (int64_t)sig);
    }
    else if(format == DATASOURCE_JSONP) {
        if(jp->responseHandler == NULL)
            jp->responseHandler = "callback";

        buffer_strcat(w->response.data, jp->responseHandler);
        buffer_strcat(w->response.data, "(");
    }

    return 0;
}

// executes the query and closes the JSONP wrapper opened by data_query_response_begin()
int data_query_response_execute(ONEWAYALLOC *owa, struct web_client *w, QUERY_TARGET *qt, DATA_QUERY_JSONP *jp, DATASOURCE_FORMAT format) {
    time_t last_timestamp_in_data = 0;
    int ret = data_query_execute(owa, w->response.data, qt, &last_timestamp_in_data);
    if(ret == HTTP_RESP_GATEWAY_TIMEOUT || ret == HTTP_RESP_CLIENT_CLOSED_REQUEST)
        return ret; // a cancelled query has a plain-text error body: do not wrap it or mark it cacheable

    if(format == DATASOURCE_DATATABLE_JSONP) {
        if(jp->google_timestamp < last_timestamp_in_data)
            buffer_strcat(w->response.data, "});");

        else {
            // the client already has the latest data
            buffer_flush(w->response.data);
            buffer_sprintf(w->response.data,
                           "%s({version:'%s',reqId:'%s',status:'error',errors:[{reason:'not_modified',message:'Data not modified'}]});",
                           jp->responseHandler, jp->google_version, jp->google_reqId);
        }
    }
    else if(format == DATASOURCE_JSONP)
        buffer_strcat(w->response.data, ");");

    if(qt->internal.relative)
        buffer_no_cacheable(w->response.data);
    else
        buffer_cacheable(w->response.data);

    return ret;
}

void web_client_ensure_proper_authorization(struct web_client *w) {
    internal_fatal(w->user_auth.method != USER_AUTH_METHOD_NONE && !(w->user_auth.access & HTTP_ACCESS_SIGNED_ID),
                   "signed-in permission should be set, but is missing");

    internal_fatal(w->user_auth.method == USER_AUTH_METHOD_NONE && (w->user_auth.access & HTTP_ACCESS_SIGNED_ID),
                   "signed-in permission is set, but it shouldn't");

#ifdef NETDATA_GOD_MODE
    web_client_set_permissions(w, HTTP_ACCESS_ALL, HTTP_USER_ROLE_ADMIN, USER_AUTH_METHOD_GOD);
#else
    if(w->user_auth.method == USER_AUTH_METHOD_NONE) {
        bool bearer_protected = netdata_bearer_protection_is_enabled();
        web_client_set_permissions(
            w,
            bearer_protected ? HTTP_ACCESS_NONE : HTTP_ACCESS_ANONYMOUS_DATA,
            bearer_protected ? HTTP_USER_ROLE_NONE : HTTP_USER_ROLE_ANY,
            USER_AUTH_METHOD_NONE);
    }
#endif
}

void web_api_command_hashes_init(struct web_api_command *api_commands, SPINLOCK *spinlock, bool *initialized) {
    if(likely(__atomic_load_n(initialized, __ATOMIC_ACQUIRE)))
        return;

    spinlock_lock(spinlock);

    if(unlikely(!__atomic_load_n(initialized, __ATOMIC_ACQUIRE))) {
        for(int i = 0; api_commands[i].api ; i++)
            api_commands[i].hash = simple_hash(api_commands[i].api);

        __atomic_store_n(initialized, true, __ATOMIC_RELEASE);
    }

    spinlock_unlock(spinlock);
}

int web_client_api_request_vX(RRDHOST *host, struct web_client *w, char *url_path_endpoint, struct web_api_command *api_commands) {
    buffer_no_cacheable(w->response.data);

    web_client_ensure_proper_authorization(w);

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
                http_access_user_has_enough_access_level_for_endpoint(w->user_auth.access, api_commands[i].access);
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
        else if(!strcmp(tok, "rfc3339"))
            options |= RRDCONTEXT_OPTION_RFC3339;
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

void web_client_progress_functions_update(nd_uuid_t *transaction, void *data, size_t done, size_t all) {
    // handle progress updates from the plugin
    // data parameter is no longer used - transaction is provided directly
    (void)data;
    query_progress_functions_update(transaction, done, all);
}


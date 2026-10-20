// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v1_calls.h"

int api_v1_function(RRDHOST *host, struct web_client *w, char *url) {
    if (!netdata_ready_load())
        return HTTP_RESP_SERVICE_UNAVAILABLE;

    int timeout = 0;
    const char *function = NULL;

    while (url) {
        char *value = strsep_skip_consecutive_separators(&url, "&");
        if (!value || !*value)
            continue;

        char *name = strsep_skip_consecutive_separators(&value, "=");
        if (!name || !*name)
            continue;

        if (!strcmp(name, "function"))
            function = value;

        else if (!strcmp(name, "timeout"))
            timeout = (int) strtoul(value, NULL, 0);
    }

    BUFFER *wb = w->response.data;
    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_no_cacheable(wb);

    // a request without a function= parameter is malformed - answer 400 here
    // instead of forwarding a NULL cmd, which would fail the registry lookup
    // and answer a misleading 404 "not available on this host"
    if(!function || !*function)
        return nrpc_call_error(wb, "No function given to execute.", HTTP_RESP_BAD_REQUEST);

    char transaction[UUID_COMPACT_STR_LEN];
    uuid_unparse_lower_compact(w->transaction, transaction);

    CLEAN_BUFFER *source = buffer_create(100, NULL);
    user_auth_to_source_buffer(&w->user_auth, source);

    return nrpc_call(&(struct nrpc_call_spec) {
        .owner = host ? rrdhost_nrpc_owner(host) : NRPC_OWNER_NONE,
        .result_wb = wb,
        .cmd = function,
        .source = buffer_tostring(source),
        .user_access = w->user_auth.access,
        .timeout_s = timeout,
        .wait = true,
        .allow_restricted = false,
        .call_id = transaction,
        .payload = w->payload,
        .progress.cb = web_client_progress_functions_update,
        .is_cancelled.cb = web_client_interrupt_callback,
        .is_cancelled.data = w,
    });
}

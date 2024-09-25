// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v1_calls.h"

int api_v1_function(RRDHOST *host, struct web_client *w, char *url) {
    if (!netdata_ready)
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

    char transaction[UUID_COMPACT_STR_LEN];
    uuid_unparse_lower_compact(w->transaction, transaction);

    CLEAN_BUFFER *source = buffer_create(100, NULL);
    web_client_api_request_vX_source_to_buffer(w, source);

    return rrd_function_run(host, wb, timeout, w->access, function, true, transaction,
                            NULL, NULL,
                            web_client_progress_functions_update, w,
                            web_client_interrupt_callback, w, w->payload,
                            buffer_tostring(source), false);
}

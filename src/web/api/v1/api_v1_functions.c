// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v1_calls.h"

int api_v1_functions(RRDHOST *host, struct web_client *w, char *url __maybe_unused) {
    if (!netdata_ready)
        return HTTP_RESP_SERVICE_UNAVAILABLE;

    BUFFER *wb = w->response.data;
    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_no_cacheable(wb);

    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
    host_functions2json(host, wb);
    buffer_json_finalize(wb);

    return HTTP_RESP_OK;
}

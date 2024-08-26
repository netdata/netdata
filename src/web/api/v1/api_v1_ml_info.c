// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v1_calls.h"

int api_v1_ml_info(RRDHOST *host, struct web_client *w, char *url) {
    (void) url;
#if defined(ENABLE_ML)

    if (!netdata_ready)
        return HTTP_RESP_SERVICE_UNAVAILABLE;

    BUFFER *wb = w->response.data;
    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;

    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
    ml_host_get_detection_info(host, wb);
    buffer_json_finalize(wb);

    buffer_no_cacheable(wb);

    return HTTP_RESP_OK;
#else
    UNUSED(host);
    UNUSED(w);
    return HTTP_RESP_SERVICE_UNAVAILABLE;
#endif // ENABLE_ML
}

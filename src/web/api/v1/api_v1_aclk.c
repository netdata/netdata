// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v1_calls.h"

int api_v1_aclk(RRDHOST *host, struct web_client *w, char *url) {
    UNUSED(url);
    UNUSED(host);
    if (!netdata_ready) return HTTP_RESP_SERVICE_UNAVAILABLE;

    BUFFER *wb = w->response.data;
    buffer_flush(wb);
    char *str = aclk_state_json();
    buffer_strcat(wb, str);
    freez(str);

    wb->content_type = CT_APPLICATION_JSON;
    buffer_no_cacheable(wb);
    return HTTP_RESP_OK;
}


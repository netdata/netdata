// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v3_calls.h"
#include "../../rtc/webrtc.h"

int web_client_api_request_v3_webrtc(RRDHOST *host __maybe_unused, struct web_client *w, char *url __maybe_unused) {
    return webrtc_new_connection(buffer_tostring(w->payload), w->response.data);
}

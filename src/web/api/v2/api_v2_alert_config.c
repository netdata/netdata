// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v2_calls.h"

int api_v2_alert_config(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    const char *config = NULL;

    while(url) {
        char *value = strsep_skip_consecutive_separators(&url, "&");
        if(!value || !*value) continue;

        char *name = strsep_skip_consecutive_separators(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        // name and value are now the parameters
        // they are not null and not empty

        if(!strcmp(name, "config"))
            config = value;
    }

    buffer_flush(w->response.data);

    if(!config) {
        w->response.data->content_type = CT_TEXT_PLAIN;
        buffer_strcat(w->response.data, "A config hash ID is required. Add ?config=UUID query param");
        return HTTP_RESP_BAD_REQUEST;
    }

    return contexts_v2_alert_config_to_json(w, config);
}

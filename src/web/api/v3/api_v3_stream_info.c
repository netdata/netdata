// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v3_calls.h"

int api_v3_stream_info(RRDHOST *host __maybe_unused, struct web_client *w, char *url __maybe_unused) {
    const char *machine_guid = NULL;

    while(url) {
        char *value = strsep_skip_consecutive_separators(&url, "&");
        if(!value || !*value) continue;

        char *name = strsep_skip_consecutive_separators(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        // name and value are now the parameters
        // they are not null and not empty

        if(!strcmp(name, "machine_guid"))
            machine_guid = value;
    }

    return stream_info_to_json_v1(w->response.data, machine_guid);
}

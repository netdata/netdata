// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v2_calls.h"

int api_v2_progress(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    char *transaction = NULL;

    while(url) {
        char *value = strsep_skip_consecutive_separators(&url, "&");
        if(!value || !*value) continue;

        char *name = strsep_skip_consecutive_separators(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        // name and value are now the parameters
        // they are not null and not empty

        if(!strcmp(name, "transaction")) transaction = value;
    }

    nd_uuid_t tr;
    uuid_parse_flexi(transaction, tr);

    rrd_function_call_progresser(&tr);

    return web_api_v2_report_progress(&tr, w->response.data);
}

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

    BUFFER *wb = w->response.data;
    buffer_reset(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    RRDHOST_STATUS status = { 0 };
    int ret = HTTP_RESP_OK;
    if(!machine_guid || !(host = rrdhost_find_by_guid(machine_guid)))
        ret = HTTP_RESP_NOT_FOUND;
    else
        rrdhost_status(host, now_realtime_sec(), &status);

    buffer_json_member_add_uint64(wb, "status", ret);
    buffer_json_member_add_uint64(wb, "nodes", dictionary_entries(rrdhost_root_index));
    buffer_json_member_add_uint64(wb, "receivers", stream_currently_connected_receivers());

    if(ret == HTTP_RESP_OK) {
        buffer_json_member_add_string(wb, "db_status", rrdhost_db_status_to_string(status.db.status));
        buffer_json_member_add_string(wb, "db_liveness", rrdhost_db_liveness_to_string(status.db.liveness));
        buffer_json_member_add_string(wb, "ingest_type", rrdhost_ingest_type_to_string(status.ingest.type));
        buffer_json_member_add_string(wb, "ingest_status", rrdhost_ingest_status_to_string(status.ingest.status));
        buffer_json_member_add_uint64(wb, "first_time_s", status.db.first_time_s);
        buffer_json_member_add_uint64(wb, "last_time_s", status.db.last_time_s);
    }

    buffer_json_finalize(wb);
    return ret;
}

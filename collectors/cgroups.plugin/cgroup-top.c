// SPDX-License-Identifier: GPL-3.0-or-later

#include "cgroup-internals.h"

int cgroup_function_cgroup_top(BUFFER *wb, int timeout __maybe_unused, const char *function __maybe_unused,
        void *collector_data __maybe_unused,
        rrd_function_result_callback_t result_cb, void *result_cb_data,
        rrd_function_is_cancelled_cb_t is_cancelled_cb, void *is_cancelled_cb_data,
        rrd_function_register_canceller_cb_t register_canceller_cb __maybe_unused,
        void *register_canceller_cb_data __maybe_unused) {

    time_t now = now_realtime_sec();

    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    buffer_json_member_add_string(wb, "hostname", rrdhost_hostname(localhost));
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", 1);
    buffer_json_member_add_string(wb, "help", RRDFUNCTIONS_CGTOP_HELP);
    buffer_json_member_add_array(wb, "data");


    for(struct cgroup *cg = cgroup_root; cg ; cg = cg->next) {
        if(unlikely(!cg->enabled || cg->pending_renames))
            continue;

        buffer_json_add_array_item_array(wb);

        buffer_json_add_array_item_string(wb, cg->name); // Name

        if(is_cgroup_systemd_service(cg))
            buffer_json_add_array_item_string(wb, "systemd"); // Kind
        else if(k8s_is_kubepod(cg))
            buffer_json_add_array_item_string(wb, "kubernetes"); // Kind
        else
            buffer_json_add_array_item_string(wb, "cgroup"); // Kind

        buffer_json_array_close(wb);

    }
    buffer_json_array_close(wb); // data
    buffer_json_member_add_object(wb, "columns");
    {
        size_t field_id = 0;

        // Node
        buffer_rrdf_table_add_field(wb, field_id++, "Name", "CGROUP Name",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_STICKY,
                NULL);

        // Kind
        buffer_rrdf_table_add_field(wb, field_id++, "Kind", "CGROUP Kind",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);



    }
    buffer_json_object_close(wb); // columns
    buffer_json_member_add_string(wb, "default_sort_column", "Name");
//    buffer_json_member_add_object(wb, "charts");
//    {
//    }
//    buffer_json_object_close(wb); // charts
//
//    buffer_json_member_add_array(wb, "default_charts");
//    {
//    }
//    buffer_json_array_close(wb);
//
//    buffer_json_member_add_object(wb, "group_by");
//    {
//    }
//    buffer_json_object_close(wb); // group_by

    buffer_json_member_add_time_t(wb, "expires", now_realtime_sec() + 1);
    buffer_json_finalize(wb);

    int response = HTTP_RESP_OK;
    if(is_cancelled_cb && is_cancelled_cb(is_cancelled_cb_data)) {
        buffer_flush(wb);
        response = HTTP_RESP_CLIENT_CLOSED_REQUEST;
    }

    if(result_cb)
        result_cb(wb, response, result_cb_data);

    return response;
}

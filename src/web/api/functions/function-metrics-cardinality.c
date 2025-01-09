// SPDX-License-Identifier: GPL-3.0-or-later

#include "function-metrics-cardinality.h"
#include "database/contexts/internal.h"

struct context_counts {
    size_t nodes;
    size_t instances;
    size_t metrics;

    size_t online_nodes;
    size_t online_instances;
    size_t online_metrics;

    size_t offline_nodes;
    size_t offline_instances;
    size_t offline_metrics;
};

int function_metrics_cardinality(BUFFER *wb, const char *function __maybe_unused, BUFFER *payload __maybe_unused, const char *source __maybe_unused) {
    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    buffer_json_member_add_string(wb, "hostname", rrdhost_hostname(localhost));
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", 1);
    buffer_json_member_add_boolean(wb, "has_history", false);
    buffer_json_member_add_string(wb, "help", RRDFUNCTIONS_METRICS_CARDINALITY_HELP);
    buffer_json_member_add_array(wb, "data");

    DICTIONARY *contexts_dict = dictionary_create(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_DONT_OVERWRITE_VALUE);

    // Collect stats for each context across all nodes
    RRDHOST *host;
    dfe_start_read(rrdhost_root_index, host) {
        if(!host->rrdctx.contexts) continue;

        bool host_online = rrdhost_is_online(host);

        RRDCONTEXT *rc;
        dfe_start_read(host->rrdctx.contexts, rc) {
            const char *context_id = string2str(rc->id);

            struct context_counts tmp = { 0 };
            struct context_counts *cnt = dictionary_set(contexts_dict, context_id, &tmp, sizeof(tmp));

            cnt->nodes++;

            if(host_online)
                cnt->online_nodes++;
            else
                cnt->offline_nodes++;

            RRDINSTANCE *ri;
            dfe_start_read(rc->rrdinstances, ri) {
                cnt->instances++;

                if(rrd_flag_check(ri, RRD_FLAG_COLLECTED))
                    cnt->online_instances++;
                else
                    cnt->offline_instances++;

                RRDMETRIC *rm;
                dfe_start_read(ri->rrdmetrics, rm) {
                    cnt->metrics++;

                    if(rrd_flag_check(rm, RRD_FLAG_COLLECTED))
                        cnt->online_metrics++;
                    else
                        cnt->offline_metrics++;
                }
                dfe_done(rm);
            }
            dfe_done(ri);
        }
        dfe_done(rc);
    }
    dfe_done(host);

    struct context_counts max = { 0 };

    // Output collected stats
    struct context_counts *cnt;
    dfe_start_read(contexts_dict, cnt) {
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, cnt_dfe.name);
        buffer_json_add_array_item_uint64(wb, cnt->nodes);
        buffer_json_add_array_item_uint64(wb, cnt->instances);
        buffer_json_add_array_item_uint64(wb, cnt->metrics);
        buffer_json_add_array_item_uint64(wb, cnt->online_nodes);
        buffer_json_add_array_item_uint64(wb, cnt->online_instances);
        buffer_json_add_array_item_uint64(wb, cnt->online_metrics);
        buffer_json_add_array_item_uint64(wb, cnt->offline_nodes);
        buffer_json_add_array_item_uint64(wb, cnt->offline_instances);
        buffer_json_add_array_item_uint64(wb, cnt->offline_metrics);

        double ephemerality = (cnt->metrics > 0) ? ((double)cnt->offline_metrics * 100.0) / (double)cnt->metrics : 0.0;
        buffer_json_add_array_item_double(wb, ephemerality);

        buffer_json_array_close(wb);

        if(cnt->nodes > max.nodes)
            max.nodes = cnt->nodes;
        if(cnt->instances > max.instances)
            max.instances = cnt->instances;
        if(cnt->metrics > max.metrics)
            max.metrics = cnt->metrics;
        if(cnt->online_nodes > max.online_nodes)
            max.online_nodes = cnt->online_nodes;
        if(cnt->online_instances > max.online_instances)
            max.online_instances = cnt->online_instances;
        if(cnt->online_metrics > max.online_metrics)
            max.online_metrics = cnt->online_metrics;
        if(cnt->offline_nodes > max.offline_nodes)
            max.offline_nodes = cnt->offline_nodes;
        if(cnt->offline_instances > max.offline_instances)
            max.offline_instances = cnt->offline_instances;
        if(cnt->offline_metrics > max.offline_metrics)
            max.offline_metrics = cnt->offline_metrics;
    }
    dfe_done(cnt);

    buffer_json_array_close(wb); // data

    buffer_json_member_add_object(wb, "columns");
    {
        size_t field_id = 0;

        buffer_rrdf_table_add_field(wb, field_id++, "Context", "Context Name",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                                    RRDF_FIELD_OPTS_FULL_WIDTH | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_VISIBLE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "All Nodes", "Number of Nodes",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "nodes", (double)max.nodes, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "All Instances", "Total Number of Instances",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "instances", (double)max.instances, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "All Metrics", "Total Number of Metrics",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "metrics", (double)max.metrics, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Curr. Nodes", "Number of Online Nodes",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "nodes", (double)max.online_nodes, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Curr. Instances", "Total Number of Online Instances",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "instances", (double)max.online_instances, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Curr. Metrics", "Total Number of Online Metrics",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "metrics", (double)max.online_metrics, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Old Nodes", "Number of Offline Nodes",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "nodes", (double)max.offline_nodes, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Old Instances", "Total Number of Offline Instances",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "instances", (double)max.offline_instances, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Old Metrics", "Total Number of Offline Metrics",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "metrics", (double)max.offline_metrics, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Ephemerality", "Percentage of Metrics that are Offline",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                                    2, "%", 100.0, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE,
                                    NULL);
    }
    buffer_json_object_close(wb); // columns

    buffer_json_member_add_string(wb, "default_sort_column", "Old Metrics");

    buffer_json_member_add_time_t(wb, "expires", now_realtime_sec() + 1);
    buffer_json_finalize(wb);

    dictionary_destroy(contexts_dict);

    return HTTP_RESP_OK;
}

// SPDX-License-Identifier: GPL-3.0-or-later

#include "function-metrics-cardinality.h"
#include "database/contexts/internal.h"

struct counts {
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
    buffer_json_member_add_time_t(wb, "update_every", 10);
    buffer_json_member_add_boolean(wb, "has_history", false);
    buffer_json_member_add_string(wb, "help", RRDFUNCTIONS_METRICS_CARDINALITY_HELP);

    // Add parameter definition similar to network-viewer
    buffer_json_member_add_array(wb, "accepted_params");
    {
        buffer_json_add_array_item_string(wb, "group");
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_array(wb, "required_params");
    {
        buffer_json_add_array_item_object(wb);
        {
            buffer_json_member_add_string(wb, "id", "group");
            buffer_json_member_add_string(wb, "name", "Grouping");
            buffer_json_member_add_string(wb, "help", "Select how to group the metrics");
            buffer_json_member_add_boolean(wb, "unique_view", true);
            buffer_json_member_add_string(wb, "type", "select");
            buffer_json_member_add_array(wb, "options");
            {
                buffer_json_add_array_item_object(wb);
                {
                    buffer_json_member_add_string(wb, "id", "by-context");
                    buffer_json_member_add_string(wb, "name", "Group by Context");
                }
                buffer_json_object_close(wb);
                buffer_json_add_array_item_object(wb);
                {
                    buffer_json_member_add_string(wb, "id", "by-node");
                    buffer_json_member_add_string(wb, "name", "Group by Node");
                }
                buffer_json_object_close(wb);
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_array_close(wb);

    // Parse function parameters
    bool by_node = false;
    {
        char function_copy[strlen(function) + 1];
        memcpy(function_copy, function, sizeof(function_copy));
        char *words[1024];
        size_t num_words = quoted_strings_splitter_whitespace(function_copy, words, 1024);
        for (size_t i = 1; i < num_words; i++) {
            char *param = get_word(words, num_words, i);
            if (strcmp(param, "group:by-node") == 0) {
                by_node = true;
            } else if (strcmp(param, "group:by-context") == 0) {
                by_node = false;
            } else if (strcmp(param, "info") == 0) {
                buffer_json_finalize(wb);
                return HTTP_RESP_OK;
            }
        }
    }

    DICTIONARY *contexts_dict = dictionary_create(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_DONT_OVERWRITE_VALUE);

    buffer_json_member_add_array(wb, "data");

    struct counts all = { 0 };
    
    // Collect stats for each context across all nodes
    RRDHOST *host;
    dfe_start_read(rrdhost_root_index, host) {
        if(!host->rrdctx.contexts) continue;

        bool host_online = rrdhost_is_online(host);

        RRDCONTEXT *rc;
        dfe_start_read(host->rrdctx.contexts, rc) {
            const char *aggregation_key = by_node ? string2str(host->hostname) :string2str(rc->id);

            struct counts cnt = { 0 };

            cnt.nodes++;

            if (host_online)
                cnt.online_nodes++;
            else
                cnt.offline_nodes++;

            RRDINSTANCE *ri;
            dfe_start_read(rc->rrdinstances, ri) {
                cnt.instances++;

                if (rrd_flag_check(ri, RRD_FLAG_COLLECTED)) {
                    cnt.online_instances++;

                    RRDMETRIC *rm;
                    dfe_start_read(ri->rrdmetrics, rm) {
                        cnt.metrics++;

                        if (rrd_flag_check(rm, RRD_FLAG_COLLECTED))
                            cnt.online_metrics++;
                        else
                            cnt.offline_metrics++;
                    }
                    dfe_done(rm);
                }
                else {
                    cnt.offline_instances++;
                    size_t metrics = dictionary_entries(ri->rrdmetrics);
                    cnt.metrics += metrics;
                    cnt.offline_metrics += metrics;
                }
            }
            dfe_done(ri);

            struct counts *cc = dictionary_get(contexts_dict, aggregation_key);
            if (cc) {
                cc->nodes += cnt.nodes;
                cc->online_nodes += cnt.online_nodes;
                cc->offline_nodes += cnt.offline_nodes;

                cc->instances += cnt.instances;
                cc->online_instances += cnt.online_instances;
                cc->offline_instances += cnt.offline_instances;

                cc->metrics += cnt.metrics;
                cc->online_metrics += cnt.online_metrics;
                cc->offline_metrics += cnt.offline_metrics;
            }
            else
                dictionary_set(contexts_dict, aggregation_key, &cnt, sizeof(cnt));

            // keep track of the total
            all.nodes += cnt.nodes;
            all.online_nodes += cnt.online_nodes;
            all.offline_nodes += cnt.offline_nodes;
            
            all.instances += cnt.instances;
            all.online_instances += cnt.online_instances;
            all.offline_instances += cnt.offline_instances;

            all.metrics += cnt.metrics;
            all.online_metrics += cnt.online_metrics;
            all.offline_metrics += cnt.offline_metrics;
        }
        dfe_done(rc);
    }
    dfe_done(host);

    struct counts max = { 0 };

    // Output collected stats
    struct counts *cnt;
    dfe_start_read(contexts_dict, cnt) {
        buffer_json_add_array_item_array(wb);
        {
            buffer_json_add_array_item_string(wb, cnt_dfe.name);

            if(!by_node) {
                buffer_json_add_array_item_uint64(wb, cnt->nodes);
                buffer_json_add_array_item_uint64(wb, cnt->online_nodes);
                buffer_json_add_array_item_uint64(wb, cnt->offline_nodes);
            }

            buffer_json_add_array_item_uint64(wb, cnt->instances);
            buffer_json_add_array_item_uint64(wb, cnt->online_instances);
            buffer_json_add_array_item_uint64(wb, cnt->offline_instances);

            buffer_json_add_array_item_uint64(wb, cnt->metrics);
            buffer_json_add_array_item_uint64(wb, cnt->online_metrics);
            buffer_json_add_array_item_uint64(wb, cnt->offline_metrics);

            double instances_ephemerality = (cnt->instances > 0) ? ((double)cnt->offline_instances * 100.0) / (double)cnt->instances : 0.0;
            buffer_json_add_array_item_double(wb, instances_ephemerality);

            double metrics_ephemerality = (cnt->metrics > 0) ? ((double)cnt->offline_metrics * 100.0) / (double)cnt->metrics : 0.0;
            buffer_json_add_array_item_double(wb, metrics_ephemerality);
            
            double percentage_of_all_instances = (all.instances) ? ((double)cnt->instances * 100.0) / (double)all.instances : 0.0;
            buffer_json_add_array_item_double(wb, percentage_of_all_instances);

            double percentage_of_online_instances = (all.online_instances) ? ((double)cnt->online_instances * 100.0) / (double)all.online_instances : 0.0;
            buffer_json_add_array_item_double(wb, percentage_of_online_instances);

            double percentage_of_offline_instances = (all.offline_instances) ? ((double)cnt->offline_instances * 100.0) / (double)all.offline_instances : 0.0;
            buffer_json_add_array_item_double(wb, percentage_of_offline_instances);

            double percentage_of_all_metrics = (all.metrics) ? ((double)cnt->metrics * 100.0) / (double)all.metrics : 0.0;
            buffer_json_add_array_item_double(wb, percentage_of_all_metrics);
            
            double percentage_of_online_metrics = (all.online_metrics) ? ((double)cnt->online_metrics * 100.0) / (double)all.online_metrics : 0.0;
            buffer_json_add_array_item_double(wb, percentage_of_online_metrics);

            double percentage_of_offline_metrics = (all.offline_metrics) ? ((double)cnt->offline_metrics * 100.0) / (double)all.offline_metrics : 0.0;
            buffer_json_add_array_item_double(wb, percentage_of_offline_metrics);
        }
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

        if(by_node) {
            buffer_rrdf_table_add_field(wb, field_id++, "Hostname", "Hostname",
                                        RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                        0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                        RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                                        RRDF_FIELD_OPTS_FULL_WIDTH | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_VISIBLE,
                                        NULL);
        }
        else {
            buffer_rrdf_table_add_field(wb, field_id++, "Context", "Context Name",
                                        RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                        0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                        RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                                        RRDF_FIELD_OPTS_FULL_WIDTH | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_VISIBLE,
                                        NULL);
        }

        if(!by_node) {
            buffer_rrdf_table_add_field(wb, field_id++, "All Nodes", "Number of Nodes",
                                        RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                        0, "nodes", (double)max.nodes, RRDF_FIELD_SORT_DESCENDING, NULL,
                                        RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                        RRDF_FIELD_OPTS_NONE,
                                        NULL);

            buffer_rrdf_table_add_field(wb, field_id++, "Curr. Nodes", "Number of Online Nodes",
                                        RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                        0, "nodes", (double)max.online_nodes, RRDF_FIELD_SORT_DESCENDING, NULL,
                                        RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                        RRDF_FIELD_OPTS_VISIBLE,
                                        NULL);

            buffer_rrdf_table_add_field(wb, field_id++, "Old Nodes", "Number of Offline Nodes",
                                        RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                        0, "nodes", (double)max.offline_nodes, RRDF_FIELD_SORT_DESCENDING, NULL,
                                        RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                        RRDF_FIELD_OPTS_VISIBLE,
                                        NULL);
        }

        buffer_rrdf_table_add_field(wb, field_id++, "All Instances", "Total Number of Instances",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "instances", (double)max.instances, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Curr. Instances", "Total Number of Currently Collected Instances",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "instances", (double)max.online_instances, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Old Instances", "Total Number of Archived Instances",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "instances", (double)max.offline_instances, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "All Dimensions", "Total Number of Time-Series",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "metrics", (double)max.metrics, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Curr. Dimensions", "Total Number of Currently Collected Time-Series",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "metrics", (double)max.online_metrics, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Old Dimensions", "Total Number of Archived Time-Series",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "metrics", (double)max.offline_metrics, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Ephemeral Instances", "Percentage of Archived Instances vs All Instances of the row",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                                    2, "%", 100.0, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Ephemeral Dimensions", "Percentage of Archived Time-Series vs All Time-Series of the row",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                                    2, "%", 100.0, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "All Instances %", "Percentage of All Instances of row vs the sum of All Instances across all rows",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                                    2, "%", 100.0, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Curr. Instances %", "Percentage of Currently Collected Instances of row vs the sum of Currently Collected Instances across all rows",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                                    2, "%", 100.0, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Old Instances %", "Percentage of Old Instances of row vs the sum of Old Instances across all rows",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                                    2, "%", 100.0, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "All Dimensions %", "Percentage of All Time-Series of row vs the sum of All Time-Series across all rows",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                                    2, "%", 100.0, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);
        
        buffer_rrdf_table_add_field(wb, field_id++, "Curr. Dimensions %", "Percentage of Currently Collected Time-Series of row vs the sum of Currently Collected Time-Series across all rows",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                                    2, "%", 100.0, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Old Dimensions %", "Percentage of Archived Time-Series of row vs the sum of Archived Time-Series across all rows",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                                    2, "%", 100.0, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);
    }
    buffer_json_object_close(wb); // columns

    buffer_json_member_add_string(wb, "default_sort_column", "Old Instances");

    buffer_json_member_add_object(wb, "charts");
    {
        buffer_json_member_add_object(wb, "Instances Ephemerality");
        {
            buffer_json_member_add_array(wb, "columns");
            buffer_json_add_array_item_string(wb, "Curr. Instances");
            buffer_json_add_array_item_string(wb, "Old Instances");
            buffer_json_array_close(wb);

            buffer_json_member_add_string(wb, "name", "Instances Ephemerality");
            buffer_json_member_add_string(wb, "type", "doughnut");
            buffer_json_member_add_string(wb, "groupBy", "all");
            buffer_json_member_add_string(wb, "aggregation", "sum");
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "Dimensions Ephemerality");
        {
            buffer_json_member_add_array(wb, "columns");
            buffer_json_add_array_item_string(wb, "Curr. Dimensions");
            buffer_json_add_array_item_string(wb, "Old Dimensions");
            buffer_json_array_close(wb);

            buffer_json_member_add_string(wb, "name", "Dimensions Ephemerality");
            buffer_json_member_add_string(wb, "type", "doughnut");
            buffer_json_member_add_string(wb, "groupBy", "all");
            buffer_json_member_add_string(wb, "aggregation", "sum");
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "Instances Total");
        {
            buffer_json_member_add_array(wb, "columns");
            buffer_json_add_array_item_string(wb, "All Instances");
            buffer_json_array_close(wb);

            buffer_json_member_add_string(wb, "name", "Instances Total");
            buffer_json_member_add_string(wb, "type", "value");
            buffer_json_member_add_string(wb, "groupBy", "all");
            buffer_json_member_add_string(wb, "aggregation", "sum");
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "Dimensions Total");
        {
            buffer_json_member_add_array(wb, "columns");
            buffer_json_add_array_item_string(wb, "All Dimensions");
            buffer_json_array_close(wb);

            buffer_json_member_add_string(wb, "name", "Dimensions Total");
            buffer_json_member_add_string(wb, "type", "value");
            buffer_json_member_add_string(wb, "groupBy", "all");
            buffer_json_member_add_string(wb, "aggregation", "sum");
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // charts

    buffer_json_member_add_array(wb, "default_charts");
    {
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "Instances Ephemerality");
        buffer_json_array_close(wb);

        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "Dimensions Ephemerality");
        buffer_json_array_close(wb);
    }
    buffer_json_array_close(wb); // default_charts

    buffer_json_member_add_time_t(wb, "expires", now_realtime_sec() + 1);
    buffer_json_finalize(wb);

    dictionary_destroy(contexts_dict);

    return HTTP_RESP_OK;
}

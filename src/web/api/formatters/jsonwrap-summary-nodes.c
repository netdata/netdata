// SPDX-License-Identifier: GPL-3.0-or-later

#include "jsonwrap.h"
#include "jsonwrap-internal.h"

// Helper structure for sorting and limiting items based on their contribution
typedef struct {
    size_t index;                // Original index in the array
    NETDATA_DOUBLE contribution; // Sorting metric (usually volume contribution)
    const char *name;           // Name for display
} CARDINALITY_ITEM;

// Comparison function for sorting items by contribution
static int cardinality_item_compare(const void *a, const void *b) {
    const CARDINALITY_ITEM *item_a = (const CARDINALITY_ITEM *)a;
    const CARDINALITY_ITEM *item_b = (const CARDINALITY_ITEM *)b;

    // Sort by contribution (highest first)
    if (item_a->contribution > item_b->contribution) return -1;
    if (item_a->contribution < item_b->contribution) return 1;

    // If equal contribution, sort alphabetically by name
    if (item_a->name && item_b->name)
        return strcmp(item_a->name, item_b->name);

    return 0;
}

void query_target_summary_nodes_v2(BUFFER *wb, QUERY_TARGET *qt, const char *key, struct summary_total_counts *totals) {
    buffer_json_member_add_array(wb, key);
    size_t count = qt->nodes.used;
    size_t cardinality_limit = qt->request.cardinality_limit;

    bool show_node_status = !(qt->request.options & RRDR_OPTION_MINIMAL_STATS);

    // Check if we need to apply cardinality limiting
    if (cardinality_limit > 0 && count > cardinality_limit) {
        // We'll need to sort and limit the nodes
        CARDINALITY_ITEM *items = mallocz(sizeof(CARDINALITY_ITEM) * count);

        // Collect contribution data for each node
        for (size_t c = 0; c < count; c++) {
            QUERY_NODE *qn = query_node(qt, c);
            items[c].index = c;
            items[c].name = rrdhost_hostname(qn->rrdhost);

            // Use query points as the metric for contribution
            if (qt->query_points.sum > 0)
                items[c].contribution = qn->query_points.sum * 100.0 / qt->query_points.sum;
            else
                items[c].contribution = 0.0;
        }

        // Sort by contribution
        qsort(items, count, sizeof(CARDINALITY_ITEM), cardinality_item_compare);

        // First add the top (limit-1) nodes
        size_t nodes_to_show = cardinality_limit - 1;
        NETDATA_DOUBLE remaining_contribution = 0.0;
        size_t remaining_count = 0;
        QUERY_METRICS_COUNTS aggregated_metrics = {0};
        QUERY_INSTANCES_COUNTS aggregated_instances = {0};
        QUERY_ALERTS_COUNTS aggregated_alerts = {0};
        STORAGE_POINT aggregated_points = STORAGE_POINT_UNSET;

        for (size_t i = 0; i < count; i++) {
            if (i < nodes_to_show) {
                // Output this node normally
                QUERY_NODE *qn = query_node(qt, items[i].index);
                RRDHOST *host = qn->rrdhost;
                buffer_json_add_array_item_object(wb);
                buffer_json_node_add_v2(wb, host, qn->slot, qn->duration_ut, show_node_status);

                // Only include detailed statistics if MINIMAL_STATS option is not set
                if (!(qt->window.options & RRDR_OPTION_MINIMAL_STATS)) {
                    query_target_instance_counts(wb, &qn->instances);
                    query_target_metric_counts(wb, &qn->metrics);
                    query_target_alerts_counts(wb, &qn->alerts, NULL, false);
                }

                query_target_points_statistics(wb, qt, &qn->query_points);
                buffer_json_object_close(wb);

                aggregate_into_summary_totals(totals, &qn->metrics);
            } else {
                // Aggregate the remaining nodes
                QUERY_NODE *qn = query_node(qt, items[i].index);
                remaining_contribution += items[i].contribution;
                remaining_count++;

                // Aggregate metrics, instances, and alerts counts
                aggregate_metrics_counts(&aggregated_metrics, &qn->metrics);
                aggregate_instances_counts(&aggregated_instances, &qn->instances);
                aggregate_alerts_counts(&aggregated_alerts, &qn->alerts);

                // Aggregate points
                storage_point_merge_to(aggregated_points, qn->query_points);

                aggregate_into_summary_totals(totals, &qn->metrics);
            }
        }

        // Add the aggregated "remaining" node if there are any
        if (remaining_count > 0) {
            buffer_json_add_array_item_object(wb);

            // Add basic info for the aggregated node
            char remaining_label[50];
            snprintfz(remaining_label, sizeof(remaining_label), "remaining %zu nodes", remaining_count);

            buffer_json_member_add_string(wb, JSKEY(id), "__remaining_nodes__");
            buffer_json_member_add_string(wb, JSKEY(hostname), remaining_label);
            buffer_json_member_add_double(wb, JSKEY(contribution), remaining_contribution);

            // Only include detailed statistics if MINIMAL_STATS option is not set
            if (!(qt->window.options & RRDR_OPTION_MINIMAL_STATS)) {
                query_target_instance_counts(wb, &aggregated_instances);
                query_target_metric_counts(wb, &aggregated_metrics);
                query_target_alerts_counts(wb, &aggregated_alerts, NULL, false);
            }

            query_target_points_statistics(wb, qt, &aggregated_points);
            buffer_json_object_close(wb);
        }

        freez(items);
    } else {
        // No limiting needed, output all nodes
        for (size_t c = 0; c < count; c++) {
            QUERY_NODE *qn = query_node(qt, c);
            RRDHOST *host = qn->rrdhost;
            buffer_json_add_array_item_object(wb);
            buffer_json_node_add_v2(wb, host, qn->slot, qn->duration_ut, show_node_status);

            // Only include detailed statistics if MINIMAL_STATS option is not set
            if (!(qt->window.options & RRDR_OPTION_MINIMAL_STATS)) {
                query_target_instance_counts(wb, &qn->instances);
                query_target_metric_counts(wb, &qn->metrics);
                query_target_alerts_counts(wb, &qn->alerts, NULL, false);
            }

            query_target_points_statistics(wb, qt, &qn->query_points);
            buffer_json_object_close(wb);

            aggregate_into_summary_totals(totals, &qn->metrics);
        }
    }

    buffer_json_array_close(wb);
}

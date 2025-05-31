// SPDX-License-Identifier: GPL-3.0-or-later

#include "jsonwrap.h"
#include "jsonwrap-internal.h"

void query_target_summary_instances_v1(BUFFER *wb, QUERY_TARGET *qt, const char *key) {
    char name[RRD_ID_LENGTH_MAX * 2 + 2];

    buffer_json_member_add_array(wb, key);
    DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);
    for (long c = 0; c < (long) qt->instances.used; c++) {
        QUERY_INSTANCE *qi = query_instance(qt, c);

        snprintfz(name, RRD_ID_LENGTH_MAX * 2 + 1, "%s:%s",
                  rrdinstance_acquired_id(qi->ria),
                  rrdinstance_acquired_name(qi->ria));

        bool *set = dictionary_set(dict, name, NULL, sizeof(*set));
        if (!*set) {
            *set = true;
            buffer_json_add_array_item_array(wb);
            buffer_json_add_array_item_string(wb, rrdinstance_acquired_id(qi->ria));
            buffer_json_add_array_item_string(wb, rrdinstance_acquired_name(qi->ria));
            buffer_json_array_close(wb);
        }
    }
    dictionary_destroy(dict);
    buffer_json_array_close(wb);
}

// Helper structure for sorting and limiting instances based on their contribution
typedef struct {
    size_t index;                // Original index in the array
    NETDATA_DOUBLE contribution; // Sorting metric (usually volume contribution)
    const char *id;              // Instance ID
    const char *name;            // Instance name
} INSTANCE_CARDINALITY_ITEM;

// Comparison function for sorting instances by contribution
static int instance_cardinality_item_compare(const void *a, const void *b) {
    const INSTANCE_CARDINALITY_ITEM *item_a = (const INSTANCE_CARDINALITY_ITEM *)a;
    const INSTANCE_CARDINALITY_ITEM *item_b = (const INSTANCE_CARDINALITY_ITEM *)b;

    // Sort by contribution (highest first)
    if (item_a->contribution > item_b->contribution) return -1;
    if (item_a->contribution < item_b->contribution) return 1;

    // If equal contribution, sort alphabetically by id
    if (item_a->id && item_b->id)
        return strcmp(item_a->id, item_b->id);

    return 0;
}

void query_target_summary_instances_v2(BUFFER *wb, QUERY_TARGET *qt, const char *key, struct summary_total_counts *totals) {
    buffer_json_member_add_array(wb, key);
    long count = (long) qt->instances.used;
    size_t cardinality_limit = qt->request.cardinality_limit;
    
    // Check if we need to apply cardinality limiting
    if (cardinality_limit > 0 && count > (long)cardinality_limit) {
        // We'll need to sort and limit the instances
        INSTANCE_CARDINALITY_ITEM *items = mallocz(sizeof(INSTANCE_CARDINALITY_ITEM) * count);
        
        // Collect contribution data for each instance
        for (long c = 0; c < count; c++) {
            QUERY_INSTANCE *qi = query_instance(qt, c);
            items[c].index = c;
            items[c].id = rrdinstance_acquired_id(qi->ria);
            items[c].name = rrdinstance_acquired_name(qi->ria);
            
            // Use query points as the metric for contribution
            if (qt->query_points.sum > 0)
                items[c].contribution = qi->query_points.sum * 100.0 / qt->query_points.sum;
            else
                items[c].contribution = 0.0;
        }
        
        // Sort by contribution
        qsort(items, count, sizeof(INSTANCE_CARDINALITY_ITEM), instance_cardinality_item_compare);
        
        // First add the top (limit-1) instances
        size_t instances_to_show = cardinality_limit - 1;
        QUERY_METRICS_COUNTS aggregated_metrics = {0};
        QUERY_ALERTS_COUNTS aggregated_alerts = {0};
        STORAGE_POINT aggregated_points = STORAGE_POINT_UNSET;
        NETDATA_DOUBLE remaining_contribution = 0.0;
        size_t remaining_count = 0;
        
        // Output the top instances
        for (size_t i = 0; (long)i < count; i++) {
            if (i < instances_to_show) {
                // Output this instance normally
                QUERY_INSTANCE *qi = query_instance(qt, items[i].index);
                
                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_string(wb, "id", items[i].id);
                
                if(!rrdinstance_acquired_id_and_name_are_same(qi->ria))
                    buffer_json_member_add_string(wb, JSKEY(name), items[i].name);
                
                buffer_json_member_add_uint64(wb, JSKEY(node_index), qi->query_host_id);
                
                if (items[i].contribution > 0.0)
                    buffer_json_member_add_double(wb, JSKEY(contribution), items[i].contribution);
                
                // Only include detailed statistics if MINIMAL_STATS option is not set
                if (!(qt->window.options & RRDR_OPTION_MINIMAL_STATS)) {
                    query_target_metric_counts(wb, &qi->metrics);
                    query_target_alerts_counts(wb, &qi->alerts, NULL, false);
                }
                
                query_target_points_statistics(wb, qt, &qi->query_points);
                buffer_json_object_close(wb);
                
                aggregate_into_summary_totals(totals, &qi->metrics);
            } else {
                // Aggregate the remaining instances
                QUERY_INSTANCE *qi = query_instance(qt, items[i].index);
                remaining_contribution += items[i].contribution;
                remaining_count++;
                
                // Aggregate metrics and alerts counts
                aggregate_metrics_counts(&aggregated_metrics, &qi->metrics);
                aggregate_alerts_counts(&aggregated_alerts, &qi->alerts);
                
                // Aggregate points
                storage_point_merge_to(aggregated_points, qi->query_points);
                
                // Still add to summary totals
                aggregate_into_summary_totals(totals, &qi->metrics);
            }
        }
        
        // Add the aggregated "remaining" entry if there are any
        if (remaining_count > 0) {
            char remaining_instance[50];
            snprintfz(remaining_instance, sizeof(remaining_instance), "remaining %zu instances", remaining_count);
            
            buffer_json_add_array_item_object(wb);
            buffer_json_member_add_string(wb, "id", "__remaining_instances__");
            buffer_json_member_add_string(wb, JSKEY(name), remaining_instance);
            
            if (remaining_contribution > 0.0)
                buffer_json_member_add_double(wb, JSKEY(contribution), remaining_contribution);
            
            // Only include detailed statistics if MINIMAL_STATS option is not set
            if (!(qt->window.options & RRDR_OPTION_MINIMAL_STATS)) {
                query_target_metric_counts(wb, &aggregated_metrics);
                query_target_alerts_counts(wb, &aggregated_alerts, NULL, false);
            }
            
            query_target_points_statistics(wb, qt, &aggregated_points);
            buffer_json_object_close(wb);
        }
        
        freez(items);
    } else {
        // No limiting needed, output all instances
        for (long c = 0; c < count; c++) {
            QUERY_INSTANCE *qi = query_instance(qt, c);
            
            buffer_json_add_array_item_object(wb);
            buffer_json_member_add_string(wb, "id", rrdinstance_acquired_id(qi->ria));
            
            if(!rrdinstance_acquired_id_and_name_are_same(qi->ria))
                buffer_json_member_add_string(wb, JSKEY(name), rrdinstance_acquired_name(qi->ria));
            
            buffer_json_member_add_uint64(wb, JSKEY(node_index), qi->query_host_id);
            
            // Calculate contribution for this instance
            if (qt->query_points.sum > 0) {
                NETDATA_DOUBLE contribution = qi->query_points.sum * 100.0 / qt->query_points.sum;
                if (contribution > 0.0)
                    buffer_json_member_add_double(wb, JSKEY(contribution), contribution);
            }
            
            // Only include detailed statistics if MINIMAL_STATS option is not set
            if (!(qt->window.options & RRDR_OPTION_MINIMAL_STATS)) {
                query_target_metric_counts(wb, &qi->metrics);
                query_target_alerts_counts(wb, &qi->alerts, NULL, false);
            }
            
            query_target_points_statistics(wb, qt, &qi->query_points);
            buffer_json_object_close(wb);
            
            aggregate_into_summary_totals(totals, &qi->metrics);
        }
    }
    buffer_json_array_close(wb);
}

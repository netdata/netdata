// SPDX-License-Identifier: GPL-3.0-or-later

#include "jsonwrap.h"
#include "jsonwrap-internal.h"

size_t query_target_summary_contexts_v2(BUFFER *wb, QUERY_TARGET *qt, const char *key, struct summary_total_counts *totals) {
    buffer_json_member_add_array(wb, key);
    DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);

    struct {
        STORAGE_POINT query_points;
        QUERY_INSTANCES_COUNTS instances;
        QUERY_METRICS_COUNTS metrics;
        QUERY_ALERTS_COUNTS alerts;
    } *z;

    for (long c = 0; c < (long) qt->contexts.used; c++) {
        QUERY_CONTEXT *qc = query_context(qt, c);

        z = dictionary_set(dict, rrdcontext_acquired_id(qc->rca), NULL, sizeof(*z));

        z->instances.selected += qc->instances.selected;
        z->instances.excluded += qc->instances.selected;
        z->instances.queried += qc->instances.queried;
        z->instances.failed += qc->instances.failed;

        z->metrics.selected += qc->metrics.selected;
        z->metrics.excluded += qc->metrics.excluded;
        z->metrics.queried += qc->metrics.queried;
        z->metrics.failed += qc->metrics.failed;

        z->alerts.clear += qc->alerts.clear;
        z->alerts.warning += qc->alerts.warning;
        z->alerts.critical += qc->alerts.critical;

        storage_point_merge_to(z->query_points, qc->query_points);
    }

    size_t unique_contexts = dictionary_entries(dict);
    dfe_start_read(dict, z) {
        buffer_json_add_array_item_object(wb);
        buffer_json_member_add_string(wb, "id", z_dfe.name);

        // Only include detailed statistics if MINIMAL_STATS option is not set
        if (!(qt->window.options & RRDR_OPTION_MINIMAL_STATS)) {
            query_target_instance_counts(wb, &z->instances);
            query_target_metric_counts(wb, &z->metrics);
            query_target_alerts_counts(wb, &z->alerts, NULL, false);
        }

        query_target_points_statistics(wb, qt, &z->query_points);
        buffer_json_object_close(wb);

        aggregate_into_summary_totals(totals, &z->metrics);
    }
    dfe_done(z);
    buffer_json_array_close(wb);
    dictionary_destroy(dict);

    return unique_contexts;
}

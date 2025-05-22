// SPDX-License-Identifier: GPL-3.0-or-later

#include "jsonwrap.h"
#include "jsonwrap-internal.h"

struct dimensions_sorted_walkthrough_data {
    BUFFER *wb;
    struct summary_total_counts *totals;
    QUERY_TARGET *qt;
    size_t cardinality_limit;
    size_t count;
    QUERY_METRICS_COUNTS aggregated_metrics;
    STORAGE_POINT aggregated_points;
    NETDATA_DOUBLE remaining_contribution;
    size_t remaining_count;
};

struct dimensions_sorted_entry {
    const char *id;
    const char *name;
    STORAGE_POINT query_points;
    QUERY_METRICS_COUNTS metrics;
    uint32_t priority;
};

static int dimensions_sorted_walktrhough_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data) {
    struct dimensions_sorted_walkthrough_data *sdwd = data;
    BUFFER *wb = sdwd->wb;
    struct summary_total_counts *totals = sdwd->totals;
    QUERY_TARGET *qt = sdwd->qt;
    struct dimensions_sorted_entry *z = value;

    sdwd->count++;

    // Check if we need to apply cardinality limiting
    if (sdwd->cardinality_limit > 0 && sdwd->count > sdwd->cardinality_limit - 1) {
        // This dimension exceeds our limit - aggregate it

        // Increment our remaining count
        sdwd->remaining_count++;

        // Calculate contribution for this dimension
        if (qt->query_points.sum > 0) {
            NETDATA_DOUBLE contribution = z->query_points.sum * 100.0 / qt->query_points.sum;
            sdwd->remaining_contribution += contribution;
        }

        // Aggregate metrics counts
        aggregate_metrics_counts(&sdwd->aggregated_metrics, &z->metrics);

        // Aggregate points
        storage_point_merge_to(sdwd->aggregated_points, z->query_points);

        // Still add this to summary totals
        aggregate_into_summary_totals(totals, &z->metrics);

        return 1; // Continue processing next dimension
    }

    // Output this dimension normally
    buffer_json_add_array_item_object(wb);
    buffer_json_member_add_string(wb, "id", z->id);
    if (z->id != z->name && z->name)
        buffer_json_member_add_string(wb, "nm", z->name);

    // Only include detailed statistics if MINIMAL_STATS option is not set
    if (!(qt->window.options & RRDR_OPTION_MINIMAL_STATS)) {
        query_target_metric_counts(wb, &z->metrics);
    }

    query_target_points_statistics(wb, qt, &z->query_points);
    buffer_json_member_add_uint64(wb, "pri", z->priority);
    buffer_json_object_close(wb);

    aggregate_into_summary_totals(totals, &z->metrics);

    return 1;
}

// Standard sort by priority
static int dimensions_sorted_priority_compar(const DICTIONARY_ITEM **item1, const DICTIONARY_ITEM **item2) {
    struct dimensions_sorted_entry *z1 = dictionary_acquired_item_value(*item1);
    struct dimensions_sorted_entry *z2 = dictionary_acquired_item_value(*item2);

    if(z1->priority == z2->priority)
        return strcmp(dictionary_acquired_item_name(*item1), dictionary_acquired_item_name(*item2));
    else if(z1->priority < z2->priority)
        return -1;
    else
        return 1;
}

// Sort by sum value (highest first), then priority
static int dimensions_sorted_sum_compar(const DICTIONARY_ITEM **item1, const DICTIONARY_ITEM **item2) {
    struct dimensions_sorted_entry *z1 = dictionary_acquired_item_value(*item1);
    struct dimensions_sorted_entry *z2 = dictionary_acquired_item_value(*item2);

    // Sort by sum (highest first)
    if (z1->query_points.sum > z2->query_points.sum) return -1;
    if (z1->query_points.sum < z2->query_points.sum) return 1;

    // If equal sum, use priority as a secondary sort
    if (z1->priority != z2->priority) {
        if (z1->priority < z2->priority) return -1;
        else return 1;
    }

    // If still equal, sort alphabetically
    return strcmp(dictionary_acquired_item_name(*item1), dictionary_acquired_item_name(*item2));
}

void query_target_summary_dimensions_v12(BUFFER *wb, QUERY_TARGET *qt, const char *key, bool v2, struct summary_total_counts *totals) {
    char buf[RRD_ID_LENGTH_MAX * 2 + 2];

    buffer_json_member_add_array(wb, key);
    DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);
    struct dimensions_sorted_entry *z;
    size_t q = 0;
    for (long c = 0; c < (long) qt->dimensions.used; c++) {
        QUERY_DIMENSION * qd = query_dimension(qt, c);
        RRDMETRIC_ACQUIRED *rma = qd->rma;

        QUERY_METRIC *qm = NULL;
        for( ; q < qt->query.used ;q++) {
            QUERY_METRIC *tqm = query_metric(qt, q);
            QUERY_DIMENSION *tqd = query_dimension(qt, tqm->link.query_dimension_id);
            if(tqd->rma != rma) break;
            qm = tqm;
        }

        const char *k, *id, *name;

        if(v2) {
            k = rrdmetric_acquired_name(rma);
            id = k;
            name = k;
        }
        else {
            snprintfz(buf, RRD_ID_LENGTH_MAX * 2 + 1, "%s:%s",
                      rrdmetric_acquired_id(rma),
                      rrdmetric_acquired_name(rma));
            k = buf;
            id = rrdmetric_acquired_id(rma);
            name = rrdmetric_acquired_name(rma);
        }

        z = dictionary_set(dict, k, NULL, sizeof(*z));
        if(!z->id) {
            z->id = id;
            z->name = name;
            z->priority = qd->priority;
        }
        else {
            if(qd->priority < z->priority)
                z->priority = qd->priority;
        }

        if(qm) {
            z->metrics.selected += (qm->status & RRDR_DIMENSION_SELECTED) ? 1 : 0;
            z->metrics.failed += (qm->status & RRDR_DIMENSION_FAILED) ? 1 : 0;

            if(qm->status & RRDR_DIMENSION_QUERIED) {
                z->metrics.queried++;
                storage_point_merge_to(z->query_points, qm->query_points);
            }
        }
        else
            z->metrics.excluded++;
    }

    if(v2) {
        size_t cardinality_limit = qt->request.cardinality_limit;
        size_t dict_entries = dictionary_entries(dict);

        // Use the enhanced walkthrough with cardinality limiting
        struct dimensions_sorted_walkthrough_data t = {
            .wb = wb,
            .totals = totals,
            .qt = qt,
            .cardinality_limit = cardinality_limit,
            .count = 0,
            .remaining_count = 0,
            .remaining_contribution = 0.0,
            .aggregated_metrics = {0},
            .aggregated_points = STORAGE_POINT_UNSET
        };

        // Execute the sorted walkthrough which will either output dimensions directly
        // or aggregate them if they exceed the cardinality limit
        // Choose the appropriate comparison function based on cardinality limiting
        dict_item_comparator_t comparator = (cardinality_limit > 0 && dict_entries > cardinality_limit) ?
                                                dimensions_sorted_sum_compar :
                                                dimensions_sorted_priority_compar;

        dictionary_sorted_walkthrough_rw(dict, DICTIONARY_LOCK_READ, dimensions_sorted_walktrhough_cb,
                                         &t, comparator);

        // Add the aggregated "remaining" dimension if there are any
        if (t.remaining_count > 0) {
            buffer_json_add_array_item_object(wb);

            // Add basic info for the aggregated dimension
            char remaining_label[50];
            snprintfz(remaining_label, sizeof(remaining_label), "remaining %zu dimensions", t.remaining_count);

            buffer_json_member_add_string(wb, "id", "__remaining_dimensions__");
            buffer_json_member_add_string(wb, "nm", remaining_label);
            buffer_json_member_add_double(wb, "con", t.remaining_contribution);

            // Only include detailed statistics if MINIMAL_STATS option is not set
            if (!(qt->window.options & RRDR_OPTION_MINIMAL_STATS)) {
                query_target_metric_counts(wb, &t.aggregated_metrics);
            }

            query_target_points_statistics(wb, qt, &t.aggregated_points);
            buffer_json_object_close(wb);
        }
    }
    else {
        // v1
        dfe_start_read(dict, z) {
            buffer_json_add_array_item_array(wb);
            buffer_json_add_array_item_string(wb, z->id);
            buffer_json_add_array_item_string(wb, z->name);
            buffer_json_array_close(wb);
        }
        dfe_done(z);
    }
    dictionary_destroy(dict);
    buffer_json_array_close(wb);
}

// SPDX-License-Identifier: GPL-3.0-or-later

#include "jsonwrap.h"
#include "jsonwrap-internal.h"

struct rrdlabels_formatting_v2 {
    DICTIONARY *keys;
    QUERY_INSTANCE *qi;
    bool v2;
};

struct rrdlabels_keys_dict_entry {
    const char *name;
    DICTIONARY *values;
    STORAGE_POINT query_points;
    QUERY_METRICS_COUNTS metrics;
};

struct rrdlabels_key_value_dict_entry {
    const char *key;
    const char *value;
    STORAGE_POINT query_points;
    QUERY_METRICS_COUNTS metrics;
};

static int rrdlabels_formatting_v2(const char *name, const char *value, RRDLABEL_SRC ls __maybe_unused, void *data) {
    struct rrdlabels_formatting_v2 *t = data;

    struct rrdlabels_keys_dict_entry *d = dictionary_set(t->keys, name, NULL, sizeof(*d));
    if(!d->values) {
        d->name = name;
        d->values = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);
    }

    char n[RRD_ID_LENGTH_MAX * 2 + 2];
    snprintfz(n, RRD_ID_LENGTH_MAX * 2, "%s:%s", name, value);

    struct rrdlabels_key_value_dict_entry *z = dictionary_set(d->values, n, NULL, sizeof(*z));
    if(!z->key) {
        z->key = name;
        z->value = value;
    }

    if(t->v2) {
        QUERY_INSTANCE *qi = t->qi;

        z->metrics.selected += qi->metrics.selected;
        z->metrics.excluded += qi->metrics.excluded;
        z->metrics.queried += qi->metrics.queried;
        z->metrics.failed += qi->metrics.failed;

        d->metrics.selected += qi->metrics.selected;
        d->metrics.excluded += qi->metrics.excluded;
        d->metrics.queried += qi->metrics.queried;
        d->metrics.failed += qi->metrics.failed;

        storage_point_merge_to(z->query_points, qi->query_points);
        storage_point_merge_to(d->query_points, qi->query_points);
    }

    return 1;
}

// Sort label values by sum value (highest first)
int label_values_sorted_sum_compar(const DICTIONARY_ITEM **item1, const DICTIONARY_ITEM **item2) {
    struct rrdlabels_key_value_dict_entry *z1 = dictionary_acquired_item_value(*item1);
    struct rrdlabels_key_value_dict_entry *z2 = dictionary_acquired_item_value(*item2);

    // Sort by sum (highest first)
    if (z1->query_points.sum > z2->query_points.sum) return -1;
    if (z1->query_points.sum < z2->query_points.sum) return 1;

    // If equal sum, sort alphabetically
    return strcmp(dictionary_acquired_item_name(*item1), dictionary_acquired_item_name(*item2));
}

/**
 * Output a label value to the JSON buffer
 */
static inline void output_label_value(BUFFER *wb, QUERY_TARGET *qt, const char *id, const char *name,
                                      QUERY_METRICS_COUNTS *metrics, STORAGE_POINT *points) {
    buffer_json_add_array_item_object(wb);
    buffer_json_member_add_string(wb, JSKEY(id), id);

    if (name)
        buffer_json_member_add_string(wb, JSKEY(name), name);

    // Only include detailed statistics if MINIMAL_STATS option is not set
    if (!(qt->window.options & RRDR_OPTION_MINIMAL_STATS)) {
        query_target_metric_counts(wb, metrics);
    }

    query_target_points_statistics(wb, qt, points);
    buffer_json_object_close(wb);
}

// Callback for label values walkthrough
struct label_values_walkthrough_data {
    BUFFER *wb;
    struct summary_total_counts *totals;
    QUERY_TARGET *qt;
    size_t cardinality_limit;
    size_t count;

    // Aggregated "remaining" value data
    struct {
        QUERY_METRICS_COUNTS metrics;
        STORAGE_POINT points;
        size_t count;
    } remaining;
};

static int label_values_walkthrough_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data) {
    struct label_values_walkthrough_data *lwt = data;
    struct summary_total_counts *totals = lwt->totals;
    QUERY_TARGET *qt = lwt->qt;
    struct rrdlabels_key_value_dict_entry *z = value;

    lwt->count++;

    // Check if we need to apply cardinality limiting
    if (lwt->cardinality_limit > 0 && lwt->count > lwt->cardinality_limit - 1) {
        // This value exceeds our limit - aggregate it

        // Increment our remaining count
        lwt->remaining.count++;

        // Aggregate metrics counts
        aggregate_metrics_counts(&lwt->remaining.metrics, &z->metrics);

        // Aggregate points
        storage_point_merge_to(lwt->remaining.points, z->query_points);

        // Still add this to summary totals
        aggregate_into_summary_totals(totals, &z->metrics);

        return 1; // Continue processing next value
    }

    // Output this value normally using our helper function
    output_label_value(lwt->wb, qt, z->value, NULL, &z->metrics, &z->query_points);

    // Add to summary totals
    aggregate_into_summary_totals(totals, &z->metrics);

    return 1;
}

void query_target_summary_labels_v12(BUFFER *wb, QUERY_TARGET *qt, const char *key, bool v2, struct summary_total_counts *key_totals, struct summary_total_counts *value_totals) {
    buffer_json_member_add_array(wb, key);
    struct rrdlabels_formatting_v2 t = {
        .keys = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE),
        .v2 = v2,
    };
    for (long c = 0; c < (long) qt->instances.used; c++) {
        QUERY_INSTANCE *qi = query_instance(qt, c);
        RRDINSTANCE_ACQUIRED *ria = qi->ria;
        t.qi = qi;
        rrdlabels_walkthrough_read(rrdinstance_acquired_labels(ria), rrdlabels_formatting_v2, &t);
    }

    size_t cardinality_limit = qt->request.cardinality_limit;

    struct rrdlabels_keys_dict_entry *d;
    dfe_start_read(t.keys, d) {
        if(v2) {
            buffer_json_add_array_item_object(wb);
            buffer_json_member_add_string(wb, JSKEY(id), d_dfe.name);

            // Only include detailed statistics if MINIMAL_STATS option is not set
            if (!(qt->window.options & RRDR_OPTION_MINIMAL_STATS)) {
                query_target_metric_counts(wb, &d->metrics);
            }

            query_target_points_statistics(wb, qt, &d->query_points);
            aggregate_into_summary_totals(key_totals, &d->metrics);
            buffer_json_member_add_array(wb, JSKEY(label_values));

            // Apply cardinality limiting to label values
            size_t values_count = dictionary_entries(d->values);

            // Setup walkthrough data regardless of whether we'll apply cardinality limiting
            struct label_values_walkthrough_data vt = {
                .wb = wb,
                .totals = value_totals,
                .qt = qt,
                .cardinality_limit = (cardinality_limit > 0 && values_count > cardinality_limit) ? cardinality_limit : 0,
                .count = 0,
                .remaining = {
                    .metrics = {0},
                    .points = STORAGE_POINT_UNSET,
                    .count = 0
                }
            };

            // Choose appropriate comparison function based on whether we need cardinality limiting
            dict_item_comparator_t comparator = (vt.cardinality_limit > 0) ?
                                                    label_values_sorted_sum_compar :
                                                    NULL; // No specific ordering for normal case

            // Use sorted walkthrough for both cases
            dictionary_sorted_walkthrough_rw(d->values, DICTIONARY_LOCK_READ,
                                             label_values_walkthrough_cb,
                                             &vt, comparator);

            // Add the aggregated "remaining" value if there are any
            if (vt.remaining.count > 0) {
                char remaining_label[50];
                snprintfz(remaining_label, sizeof(remaining_label), "remaining %zu values", vt.remaining.count);

                // Use our helper function for consistency
                output_label_value(wb, qt, "__remaining_values__", remaining_label,
                                   &vt.remaining.metrics, &vt.remaining.points);
            }

            buffer_json_array_close(wb); // vl
            buffer_json_object_close(wb); // label key

        } else {
            // v1 format - no cardinality limiting here
            struct rrdlabels_key_value_dict_entry *z;
            dfe_start_read(d->values, z){
                buffer_json_add_array_item_array(wb);
                buffer_json_add_array_item_string(wb, z->key);
                buffer_json_add_array_item_string(wb, z->value);
                buffer_json_array_close(wb);
            }
            dfe_done(z);
        }

        dictionary_destroy(d->values);
    }
    dfe_done(d);
    dictionary_destroy(t.keys);
    buffer_json_array_close(wb);
}

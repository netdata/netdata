// SPDX-License-Identifier: GPL-3.0-or-later

#include "query-internal.h"

static int compare_contributions(const void *a, const void *b) {
    const struct { size_t dim_idx; NETDATA_DOUBLE contribution; const char *id; } *da = a;
    const struct { size_t dim_idx; NETDATA_DOUBLE contribution; const char *id; } *db = b;

    if (da->contribution > db->contribution) return -1;
    if (da->contribution < db->contribution) return 1;

    // deterministic tie-break by dimension id: qsort() is not stable, and
    // aggregators (Netdata Cloud) break equal-contribution ties by name -
    // without this, which of two tied dimensions survives the fold would be
    // unspecified and could differ from the merger's own choice
    return strcmp(da->id, db->id);
}

// merge the group-by labels of a folded dimension into the labels
// dictionary of the "remaining" aggregate dimension
static void group_by_labels_merge(DICTIONARY *dst, DICTIONARY *src) {
    struct group_by_label_key *k;
    dfe_start_read(src, k) {
        struct group_by_label_key *dk = dictionary_set(dst, k_dfe.name, NULL, sizeof(struct group_by_label_key));

        void *t;
        dfe_start_read(k->values, t) {
            dictionary_set(dk->values, t_dfe.name, NULL, 0);
        }
        dfe_done(t);
    }
    dfe_done(k);
}

RRDR *rrd2rrdr_cardinality_limit(RRDR *r) {
    QUERY_TARGET *qt = r->internal.qt;

    if(!qt || qt->request.cardinality_limit == 0 || r->d <= qt->request.cardinality_limit)
        return r;

    ONEWAYALLOC *owa = r->internal.owa;

    // Calculate contribution of each dimension using dview statistics (sum of values)
    NETDATA_DOUBLE *contributions = onewayalloc_mallocz(
        owa, onewayalloc_mul_or_fatal(r->d, sizeof(*contributions), "RRDR cardinality contributions"));

    // Count queried dimensions and get their contributions from dview
    size_t queried_count = 0;
    for (size_t d = 0; d < r->d; d++) {
        contributions[d] = 0.0;

        if (!(r->od[d] & RRDR_DIMENSION_QUERIED))
            continue;

        queried_count++;

        // Use the sum from dview if available, otherwise fall back to manual calculation
        if(r->dview && !isnan(r->dview[d].sum)) {
            contributions[d] = fabsndd(r->dview[d].sum);
        } else {
            // Fallback: calculate manually from values
            for(size_t i = 0; i < r->rows; i++) {
                size_t idx = i * r->d + d;

                if(r->o[idx] & RRDR_VALUE_EMPTY)
                    continue;

                NETDATA_DOUBLE value = r->v[idx];
                if(!isnan(value))
                    contributions[d] += fabsndd(value);
            }
        }
    }

    // If we don't need to reduce, return original
    if(queried_count <= qt->request.cardinality_limit) {
        onewayalloc_freez(owa, contributions);
        return r;
    }

    // Create array of dimension indices sorted by contribution (descending)
    struct {
        size_t dim_idx;
        NETDATA_DOUBLE contribution;
        const char *id;
    } *sorted_dims = onewayalloc_mallocz(
        owa, onewayalloc_mul_or_fatal(queried_count, sizeof(*sorted_dims), "RRDR cardinality dimensions"));

    size_t sorted_idx = 0;
    for (size_t d = 0; d < r->d; d++) {
        if (r->od[d] & RRDR_DIMENSION_QUERIED) {
            sorted_dims[sorted_idx].dim_idx = d;
            sorted_dims[sorted_idx].contribution = contributions[d];
            sorted_dims[sorted_idx].id = string2str(r->di[d]);
            sorted_idx++;
        }
    }

    // Sort by contribution (descending)
    qsort(sorted_dims, queried_count, sizeof(*sorted_dims), compare_contributions);

    // The reduced RRDR keeps the top (cardinality_limit - 1) dimensions and
    // folds the rest into one "remaining N dimensions" aggregate slot;
    // queried_count > cardinality_limit here, so at least 2 dimensions fold
    size_t new_d = qt->request.cardinality_limit;
    size_t kept_dimensions = new_d - 1;

    RRDR *new_r = rrdr_create(owa, qt, new_d, r->n);
    if (!new_r) {
        internal_error(true, "QUERY: cannot create cardinality limited RRDR");
        onewayalloc_freez(owa, contributions);
        onewayalloc_freez(owa, sorted_dims);
        return r;
    }

    // Copy basic metadata from original RRDR
    new_r->view = r->view;
    new_r->time_grouping = r->time_grouping;
    new_r->partial_data_trimming = r->partial_data_trimming;
    new_r->rows = r->rows;
    new_r->stats = r->stats;

    // Copy timestamps
    memcpy(new_r->t, r->t, r->n * sizeof(time_t));

    // Setup the arrays of the new RRDR, mirroring the arrays present in the
    // source RRDR: the formatters expect the reduced result to be
    // structurally identical to the original one - a missing array either
    // crashes them (gbc on the aggregatable path) or silently drops output
    // sections (dgbc, vh, dqp, dl); hgbc is intentionally NOT mirrored -
    // it is internal to the group-by passes and consumed (freed and NULLed)
    // by rrd2rrdr_group_by_finalize() before this reduction runs
    if(new_r->d) {
        if(r->dp)
            new_r->dp = onewayalloc_callocz(owa, new_r->d, sizeof(*new_r->dp));

        if(r->dview)
            new_r->dview = onewayalloc_callocz(owa, new_r->d, sizeof(*new_r->dview));

        if(r->dgbc)
            new_r->dgbc = onewayalloc_callocz(owa, new_r->d, sizeof(*new_r->dgbc));

        if(r->dqp)
            new_r->dqp = onewayalloc_callocz(owa, new_r->d, sizeof(STORAGE_POINT));

        if(r->dl) {
            new_r->dl = onewayalloc_callocz(owa, new_r->d, sizeof(DICTIONARY *));

            // take over the label keys index - the original RRDR is freed below
            new_r->label_keys = r->label_keys;
            r->label_keys = NULL;
        }

        if(new_r->n) {
            if(r->gbc)
                new_r->gbc = onewayalloc_callocz(
                    owa, onewayalloc_mul_or_fatal(new_r->n, new_r->d, "RRDR group-by counts"), sizeof(*new_r->gbc));

            if(r->vh)
                new_r->vh = onewayalloc_mallocz(
                    owa, onewayalloc_mul3_or_fatal(new_r->n, new_r->d, sizeof(*new_r->vh), "RRDR hidden values"));

            // Initialize all values as empty
            for (size_t i = 0; i < new_r->n; i++) {
                for (size_t d = 0; d < new_r->d; d++) {
                    size_t idx = i * new_r->d + d;
                    new_r->v[idx] = NAN;
                    new_r->ar[idx] = 0.0;
                    new_r->o[idx] = RRDR_VALUE_EMPTY;

                    if(new_r->vh)
                        new_r->vh[idx] = NAN;
                }
            }
        }
    }

    // Copy top dimensions
    for (size_t i = 0; i < kept_dimensions; i++) {
        size_t src_d = sorted_dims[i].dim_idx;

        // Copy metadata
        new_r->di[i] = string_dup(r->di[src_d]);
        new_r->dn[i] = string_dup(r->dn[src_d]);
        new_r->od[i] = r->od[src_d];
        new_r->du[i] = string_dup(r->du[src_d]);

        if(new_r->dp)
            new_r->dp[i] = r->dp[src_d];

        if(new_r->dgbc)
            new_r->dgbc[i] = r->dgbc[src_d];

        if(new_r->dqp)
            new_r->dqp[i] = r->dqp[src_d];

        if(new_r->dl) {
            // move the labels of this dimension - the original is freed below
            new_r->dl[i] = r->dl[src_d];
            r->dl[src_d] = NULL;
        }

        // Copy data
        for (size_t row = 0; row < r->rows; row++) {
            size_t src_idx = row * r->d + src_d;
            size_t dst_idx = row * new_r->d + i;

            new_r->v[dst_idx] = r->v[src_idx];
            new_r->ar[dst_idx] = r->ar[src_idx];
            new_r->o[dst_idx] = r->o[src_idx];

            if(new_r->gbc)
                new_r->gbc[dst_idx] = r->gbc[src_idx];

            if(new_r->vh)
                new_r->vh[dst_idx] = r->vh[src_idx];
        }

        // Copy dview stats
        if(new_r->dview)
            new_r->dview[i] = r->dview[src_d];
    }

    // Create the "remaining N dimensions" aggregate dimension
    {
        size_t remaining_idx = kept_dimensions;
        size_t remaining_count = queried_count - kept_dimensions;

        char remaining_name[256];
        snprintfz(remaining_name, sizeof(remaining_name), "remaining %zu dimensions", remaining_count);

        new_r->di[remaining_idx] = string_strdupz(remaining_name);
        new_r->dn[remaining_idx] = string_strdupz(remaining_name);
        new_r->od[remaining_idx] = RRDR_DIMENSION_QUERIED | RRDR_DIMENSION_NONZERO;

        // Use the units and priority from the first remaining dimension
        size_t first_remaining_d = sorted_dims[kept_dimensions].dim_idx;
        new_r->du[remaining_idx] = string_dup(r->du[first_remaining_d]);

        if(new_r->dp)
            new_r->dp[remaining_idx] = r->dp[first_remaining_d];

        if(new_r->dl) {
            new_r->dl[remaining_idx] = dictionary_create_advanced(
                DICT_OPTION_SINGLE_THREADED | DICT_OPTION_FIXED_SIZE | DICT_OPTION_DONT_OVERWRITE_VALUE,
                NULL, sizeof(struct group_by_label_key));
            dictionary_register_insert_callback(new_r->dl[remaining_idx], group_by_label_key_insert_cb, new_r->label_keys);
            dictionary_register_delete_callback(new_r->dl[remaining_idx], group_by_label_key_delete_cb, new_r->label_keys);
        }

        if(new_r->dqp)
            new_r->dqp[remaining_idx] = STORAGE_POINT_UNSET;

        // Aggregate the per-dimension metadata of the folded dimensions
        for (size_t i = kept_dimensions; i < queried_count; i++) {
            size_t src_d = sorted_dims[i].dim_idx;

            if(new_r->dgbc)
                new_r->dgbc[remaining_idx] += r->dgbc[src_d];

            if(new_r->dqp)
                storage_point_merge_to(new_r->dqp[remaining_idx], r->dqp[src_d]);

            if(new_r->dl && r->dl[src_d])
                group_by_labels_merge(new_r->dl[remaining_idx], r->dl[src_d]);
        }

        // Aggregate remaining dimensions
        NETDATA_DOUBLE sum = 0.0, min = NAN, max = NAN, ars = 0.0;
        size_t count = 0;

        for (size_t row = 0; row < r->rows; row++) {
            size_t dst_idx = row * new_r->d + remaining_idx;
            NETDATA_DOUBLE aggregated_value = 0.0;
            NETDATA_DOUBLE aggregated_ar = 0.0;
            NETDATA_DOUBLE aggregated_hidden = NAN;
            uint32_t aggregated_gbc = 0;
            RRDR_VALUE_FLAGS aggregated_flags = RRDR_VALUE_NOTHING;
            bool has_values = false;
            bool has_empty = false;

            for (size_t i = kept_dimensions; i < queried_count; i++) {
                size_t src_d = sorted_dims[i].dim_idx;
                size_t src_idx = row * r->d + src_d;

                // the hidden (percentage-of-group denominator) sum is
                // tracked independently of the visible value - a group may
                // have denominator data at a point where its visible value
                // is empty (only hidden dimensions collected there)
                if(r->vh) {
                    NETDATA_DOUBLE hidden = r->vh[src_idx];
                    if(!isnan(hidden))
                        aggregated_hidden = isnan(aggregated_hidden) ? hidden : aggregated_hidden + hidden;
                }

                if(!(r->o[src_idx] & RRDR_VALUE_EMPTY)) {
                    NETDATA_DOUBLE value = r->v[src_idx];
                    if(!isnan(value)) {
                        aggregated_value += value;
                        aggregated_ar += r->ar[src_idx];
                        aggregated_flags |= (r->o[src_idx] & (RRDR_VALUE_RESET | RRDR_VALUE_PARTIAL));

                        if(r->gbc)
                            aggregated_gbc += r->gbc[src_idx];

                        has_values = true;
                    }
                    else
                        has_empty = true;
                }
                else
                    has_empty = true;
            }

            // folding an empty point together with values makes the
            // aggregate partial - the same rule the mergers apply when they
            // fold dimensions themselves; without it the bucket looks
            // complete and live-edge trimming keeps incomplete rows
            if(has_values && has_empty)
                aggregated_flags |= RRDR_VALUE_PARTIAL;

            if(new_r->vh)
                new_r->vh[dst_idx] = aggregated_hidden;

            if(has_values) {
                new_r->v[dst_idx] = aggregated_value;
                new_r->ar[dst_idx] = aggregated_ar;
                new_r->o[dst_idx] = aggregated_flags & ~RRDR_VALUE_EMPTY;

                if(new_r->gbc)
                    new_r->gbc[dst_idx] = aggregated_gbc;

                // Update statistics for dview
                sum += aggregated_value;
                ars += aggregated_ar;
                if(count == 0) {
                    min = max = aggregated_value;
                } else {
                    if(aggregated_value < min) min = aggregated_value;
                    if(aggregated_value > max) max = aggregated_value;
                }
                count++;
            } else {
                new_r->v[dst_idx] = NAN;
                new_r->ar[dst_idx] = 0.0;
                new_r->o[dst_idx] = RRDR_VALUE_EMPTY;
            }
        }

        // Set dview for remaining dimension
        if(new_r->dview) {
            new_r->dview[remaining_idx] = (STORAGE_POINT) {
                .sum = sum,
                .count = count,
                .min = min,
                .max = max,
                .anomaly_count = (size_t)(ars * RRDR_DVIEW_ANOMALY_COUNT_MULTIPLIER / 100.0),
            };
        }
    }

    // Cleanup
    onewayalloc_freez(owa, contributions);
    onewayalloc_freez(owa, sorted_dims);

    // Free the original RRDR
    rrdr_free(owa, r);

    return new_r;
}

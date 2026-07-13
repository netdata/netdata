// SPDX-License-Identifier: GPL-3.0-or-later

#include "query-internal.h"

// hgbc packs, per point, the count of hidden (percentage denominator)
// contributions and, in the top bit, whether any of them was itself partial
#define HGBC_PARTIAL_FLAG (1U << 31)
#define HGBC_COUNT_MASK   (~HGBC_PARTIAL_FLAG)

void rrd2rrdr_group_by_add_metric(RRDR *r_dst, size_t d_dst, RRDR *r_tmp, size_t d_tmp,
                                         RRDR_GROUP_BY_FUNCTION group_by_aggregate_function,
                                         STORAGE_POINT *query_points, size_t pass __maybe_unused) {
    if(!r_tmp || r_dst == r_tmp || !(r_tmp->od[d_tmp] & RRDR_DIMENSION_QUERIED))
        return;

    internal_fatal(r_dst->n != r_tmp->n, "QUERY: group-by source and destination do not have the same number of rows");
    internal_fatal(d_dst >= r_dst->d, "QUERY: group-by destination dimension number exceeds destination RRDR size");
    internal_fatal(d_tmp >= r_tmp->d, "QUERY: group-by source dimension number exceeds source RRDR size");
    internal_fatal(!r_dst->dqp, "QUERY: group-by destination is not properly prepared (missing dqp array)");
    internal_fatal(!r_dst->gbc, "QUERY: group-by destination is not properly prepared (missing gbc array)");

    bool hidden_dimension_on_percentage_of_group = (r_tmp->od[d_tmp] & RRDR_DIMENSION_HIDDEN) && r_dst->vh;

    if(!hidden_dimension_on_percentage_of_group) {
        r_dst->od[d_dst] |= r_tmp->od[d_tmp];
        storage_point_merge_to(r_dst->dqp[d_dst], *query_points);
    }

    // do the group_by
    for(size_t i = 0; i != rrdr_rows(r_tmp) ; i++) {

        size_t idx_tmp = i * r_tmp->d + d_tmp;
        NETDATA_DOUBLE n_tmp = r_tmp->v[ idx_tmp ];
        RRDR_VALUE_FLAGS o_tmp = r_tmp->o[ idx_tmp ];
        NETDATA_DOUBLE ar_tmp = r_tmp->ar[ idx_tmp ];

        if(o_tmp & RRDR_VALUE_EMPTY)
            continue;

        size_t idx_dst = i * r_dst->d + d_dst;
        NETDATA_DOUBLE *cn = (hidden_dimension_on_percentage_of_group) ? &r_dst->vh[ idx_dst ] : &r_dst->v[ idx_dst ];
        RRDR_VALUE_FLAGS *co = &r_dst->o[ idx_dst ];
        NETDATA_DOUBLE *ar = &r_dst->ar[ idx_dst ];
        uint32_t *gbc = &r_dst->gbc[ idx_dst ];

        switch(group_by_aggregate_function) {
            default:
            case RRDR_GROUP_BY_FUNCTION_AVERAGE:
            case RRDR_GROUP_BY_FUNCTION_SUM:
            case RRDR_GROUP_BY_FUNCTION_PERCENTAGE:
                if(isnan(*cn))
                    *cn = n_tmp;
                else
                    *cn += n_tmp;
                break;

            case RRDR_GROUP_BY_FUNCTION_MIN:
                if(isnan(*cn) || n_tmp < *cn)
                    *cn = n_tmp;
                break;

            case RRDR_GROUP_BY_FUNCTION_MAX:
                if(isnan(*cn) || n_tmp > *cn)
                    *cn = n_tmp;
                break;

            case RRDR_GROUP_BY_FUNCTION_EXTREMES:
                // For extremes, we need to keep track of the value with the maximum absolute value
                if(isnan(*cn) || fabsndd(n_tmp) > fabsndd(*cn))
                    *cn = n_tmp;
                break;
        }

        if(!hidden_dimension_on_percentage_of_group) {
            *co &= ~RRDR_VALUE_EMPTY;
            *co |= (o_tmp & (RRDR_VALUE_RESET | RRDR_VALUE_PARTIAL));
            *ar += ar_tmp;
            (*gbc)++;
        }
        else if(r_dst->hgbc) {
            // count the hidden (denominator) contributions too, so that
            // an incomplete denominator can be detected as partial; a
            // denominator source that is itself partial (a shadow bucket
            // stamped at its own pass) taints the point through the top
            // bit - hgbc is internal to group-by and never leaves finalize
            r_dst->hgbc[idx_dst]++;
            if(o_tmp & RRDR_VALUE_PARTIAL)
                r_dst->hgbc[idx_dst] |= HGBC_PARTIAL_FLAG;
        }
    }
}

// stamp RRDR_VALUE_PARTIAL on every point that received fewer contributions
// than the sources mapped into its dimension - visible sources are counted
// by gbc, hidden (percentage denominator) sources by hgbc; this runs at
// EVERY group-by pass, because the next pass only sees whole groups: a group
// missing some of its own members would otherwise arrive complete-looking
// there; visible flags propagate upward with the values through
// rrd2rrdr_group_by_add_metric(), hidden flags through the hgbc top bit
static void rrd2rrdr_group_by_stamp_partial(RRDR *r, const uint32_t *expected_gbc, const uint32_t *expected_hgbc) {
    for(size_t i = 0; i < r->n ;i++) {
        for(size_t d = 0; d < r->d ;d++) {
            size_t idx = i * r->d + d;

            uint32_t gbc = r->gbc[idx];
            if(!gbc)
                // points without visible contributions are empty, not partial
                continue;

            uint32_t hgbc = r->hgbc ? r->hgbc[idx] : 0;
            if(gbc != expected_gbc[d] ||
               (hgbc & HGBC_COUNT_MASK) != expected_hgbc[d] ||
               (hgbc & HGBC_PARTIAL_FLAG))
                r->o[idx] |= RRDR_VALUE_PARTIAL;
        }
    }
}

void rrdr2rrdr_group_by_partial_trimming(RRDR *r) {
    time_t trimmable_after = r->partial_data_trimming.expected_after;

    // find the point just before the trimmable ones
    ssize_t i = (ssize_t)r->n - 1;
    for( ; i >= 0 ;i--) {
        if (r->t[i] < trimmable_after)
            break;
    }

    if(unlikely(i < 0))
        return;

    // internal_error(true, "Found trimmable index %zd (from 0 to %zu)", i, r->n - 1);

    size_t last_row_gbc = 0;
    for (; i < (ssize_t)r->n; i++) {
        size_t row_gbc = 0;
        for (size_t d = 0; d < r->d; d++) {
            if (unlikely(!(r->od[d] & RRDR_DIMENSION_QUERIED)))
                continue;

            row_gbc += r->gbc[ i * r->d + d ];
        }

        // internal_error(true, "GBC of index %zd is %zu", i, row_gbc);

        if (unlikely(r->t[i] >= trimmable_after && (row_gbc < last_row_gbc || !row_gbc))) {
            // discard the rest of the points
            // internal_error(true, "Discarding points %zd to %zu", i, r->n - 1);
            r->partial_data_trimming.trimmed_after = r->t[i];
            r->rows = i;
            break;
        }
        else
            last_row_gbc = row_gbc;
    }
}

void rrdr2rrdr_group_by_calculate_percentage_of_group(RRDR *r) {
    if(!r->vh)
        return;

    if(query_target_aggregatable(r->internal.qt) && query_has_group_by_aggregation_percentage(r->internal.qt))
        return;

    for(size_t i = 0; i < r->n ;i++) {
        NETDATA_DOUBLE *cn = &r->v[ i * r->d ];
        NETDATA_DOUBLE *ch = &r->vh[ i * r->d ];

        for(size_t d = 0; d < r->d ;d++) {
            NETDATA_DOUBLE n = cn[d];
            NETDATA_DOUBLE h = ch[d];

            if(isnan(n))
                cn[d] = 0.0;

            else if(isnan(h))
                cn[d] = 100.0;

            else {
                // all series collected zeros (or cancel out): report 0%, not a gap
                NETDATA_DOUBLE t = n + h;
                cn[d] = (t != 0.0) ? (n * 100.0 / t) : 0.0;
            }
        }
    }
}


void rrd2rrdr_convert_values_to_percentage_of_total(RRDR *r) {
    if(!(r->internal.qt->window.options & RRDR_OPTION_PERCENTAGE) || query_target_aggregatable(r->internal.qt))
        return;

    size_t global_min_max_values = 0;
    NETDATA_DOUBLE global_min = NAN, global_max = NAN;

    for(size_t i = 0; i != r->n ;i++) {
        NETDATA_DOUBLE *cn = &r->v[ i * r->d ];
        RRDR_VALUE_FLAGS *co = &r->o[ i * r->d ];

        NETDATA_DOUBLE total = 0;
        for (size_t d = 0; d < r->d; d++) {
            if (unlikely(!(r->od[d] & RRDR_DIMENSION_QUERIED)))
                continue;

            if(co[d] & RRDR_VALUE_EMPTY)
                continue;

            total += cn[d];
        }

        if(total == 0.0)
            total = 1.0;

        for (size_t d = 0; d < r->d; d++) {
            if (unlikely(!(r->od[d] & RRDR_DIMENSION_QUERIED)))
                continue;

            if(co[d] & RRDR_VALUE_EMPTY)
                continue;

            NETDATA_DOUBLE n = cn[d];
            n = cn[d] = n * 100.0 / total;

            if(unlikely(!global_min_max_values++))
                global_min = global_max = n;
            else {
                if(n < global_min)
                    global_min = n;
                if(n > global_max)
                    global_max = n;
            }
        }
    }

    r->view.min = global_min;
    r->view.max = global_max;

    if(!r->dview)
        // v1 query
        return;

    // v2 query

    for (size_t d = 0; d < r->d; d++) {
        if (unlikely(!(r->od[d] & RRDR_DIMENSION_QUERIED)))
            continue;

        size_t count = 0;
        NETDATA_DOUBLE min = 0.0, max = 0.0, sum = 0.0, ars = 0.0;
        for(size_t i = 0; i != r->rows ;i++) { // we use r->rows to respect trimming
            size_t idx = i * r->d + d;

            RRDR_VALUE_FLAGS o = r->o[ idx ];

            if (o & RRDR_VALUE_EMPTY)
                continue;

            NETDATA_DOUBLE ar = r->ar[ idx ];
            ars += ar;

            NETDATA_DOUBLE n = r->v[ idx ];
            sum += n;

            if(!count++)
                min = max = n;
            else {
                if(n < min)
                    min = n;
                if(n > max)
                    max = n;
            }
        }

        r->dview[d] = (STORAGE_POINT) {
            .sum = sum,
            .count = count,
            .min = min,
            .max = max,
            .anomaly_count = (size_t)(ars * (NETDATA_DOUBLE)count),
        };
    }
}

RRDR *rrd2rrdr_group_by_finalize(RRDR *r_tmp) {
    QUERY_TARGET *qt = r_tmp->internal.qt;

    if(!r_tmp->group_by.r) {
        // v1 query
        rrd2rrdr_convert_values_to_percentage_of_total(r_tmp);
        return r_tmp;
    }
    // v2 query

    // do the additional passes on RRDRs
    RRDR *last_r = r_tmp->group_by.r;

    // how many sources are expected to contribute to every point of each
    // dimension, in the units gbc counts at each pass: metrics at the first
    // pass, prior-pass groups at later passes; dgbc cannot be this comparand
    // - it counts metrics at every pass, including the hidden ones - so
    // complete data would flag PARTIAL; the split between expected_gbc and
    // expected_hgbc mirrors the routing rule of
    // rrd2rrdr_group_by_add_metric(): hidden sources feed the percentage
    // denominator (vh) and are counted by hgbc, never by gbc
    ONEWAYALLOC *owa = last_r->internal.owa;
    uint32_t *expected_gbc = onewayalloc_callocz(owa, last_r->d, sizeof(*expected_gbc));
    uint32_t *expected_hgbc = onewayalloc_callocz(owa, last_r->d, sizeof(*expected_hgbc));

    // the first pass received its contributions per metric during query
    // execution
    for(size_t m = 0; m < qt->query.used ;m++) {
        QUERY_METRIC *qm = query_metric(qt, m);
        if((qm->status & RRDR_DIMENSION_HIDDEN) && last_r->vh)
            expected_hgbc[qm->grouped_as.first_slot]++;
        else
            expected_gbc[qm->grouped_as.first_slot]++;
    }
    rrd2rrdr_group_by_stamp_partial(last_r, expected_gbc, expected_hgbc);

    rrdr2rrdr_group_by_calculate_percentage_of_group(last_r);

    RRDR *r = last_r->group_by.r;
    size_t pass = 0;
    while(r) {
        pass++;
        onewayalloc_freez(owa, expected_gbc);
        onewayalloc_freez(owa, expected_hgbc);
        expected_gbc = onewayalloc_callocz(owa, r->d, sizeof(*expected_gbc));
        expected_hgbc = onewayalloc_callocz(owa, r->d, sizeof(*expected_hgbc));

        for(size_t d = 0; d < last_r->d ;d++) {
            if((last_r->od[d] & RRDR_DIMENSION_HIDDEN) && r->vh)
                expected_hgbc[last_r->dgbs[d]]++;
            else
                expected_gbc[last_r->dgbs[d]]++;

            rrd2rrdr_group_by_add_metric(r, last_r->dgbs[d], last_r, d,
                                         qt->request.group_by[pass].aggregation,
                                         &last_r->dqp[d], pass);
        }
        rrd2rrdr_group_by_stamp_partial(r, expected_gbc, expected_hgbc);
        rrdr2rrdr_group_by_calculate_percentage_of_group(r);

        last_r = r;
        r = last_r->group_by.r;
    }

    onewayalloc_freez(owa, expected_gbc);
    onewayalloc_freez(owa, expected_hgbc);

    // the hidden contribution counters are internal to the group-by passes
    onewayalloc_freez(owa, last_r->hgbc);
    last_r->hgbc = NULL;

    // free all RRDRs except the last one
    r = r_tmp;
    while(r != last_r) {
        r_tmp = r->group_by.r;
        r->group_by.r = NULL;
        rrdr_free(r->internal.owa, r);
        r = r_tmp;
    }
    r = last_r;

    // find the final aggregation
    RRDR_GROUP_BY_FUNCTION aggregation = qt->request.group_by[0].aggregation;
    for(size_t g = 0; g < MAX_QUERY_GROUP_BY_PASSES ;g++)
        if(qt->request.group_by[g].group_by != RRDR_GROUP_BY_NONE)
            aggregation = qt->request.group_by[g].aggregation;

    if(!query_target_aggregatable(qt) && r->partial_data_trimming.expected_after < qt->window.before)
        rrdr2rrdr_group_by_partial_trimming(r);

    // for every aggregation except AVERAGE the plotted value is already the
    // final group aggregate (the percentage, the sum, the min, the max), so
    // the sts pair must average over the view rows to stay consistent with
    // min/max (row extremes), not over the per-point source contributions
    // (gbc); AVERAGE accumulates pre-division group sums, so its (sum, gbc)
    // pair is a correct weighted mean; in aggregatable (raw) mode the cloud
    // derives the statistics, so the (sum, gbc) pair is kept as-is
    bool stats_by_rows =
        aggregation != RRDR_GROUP_BY_FUNCTION_AVERAGE && !query_target_aggregatable(qt);

    // apply averaging, remove RRDR_VALUE_EMPTY, find the non-zero dimensions, min and max
    size_t global_min_max_values = 0;
    size_t dimensions_nonzero = 0;
    NETDATA_DOUBLE global_min = NAN, global_max = NAN;
    for (size_t d = 0; d < r->d; d++) {
        if (unlikely(!(r->od[d] & RRDR_DIMENSION_QUERIED)))
            continue;

        size_t points_nonzero = 0;
        NETDATA_DOUBLE min = 0, max = 0, sum = 0, ars = 0;
        size_t count = 0;

        for(size_t i = 0; i != r->n ;i++) {
            size_t idx = i * r->d + d;

            NETDATA_DOUBLE *cn = &r->v[ idx ];
            RRDR_VALUE_FLAGS *co = &r->o[ idx ];
            NETDATA_DOUBLE *ar = &r->ar[ idx ];
            uint32_t gbc = r->gbc[ idx ];

            if(likely(gbc)) {
                *co &= ~RRDR_VALUE_EMPTY;

                NETDATA_DOUBLE n;

                sum += *cn;

                // when the sts pair is per-row, the anomaly rate must also be
                // accumulated per-row (the row mean), not per-contribution
                ars += stats_by_rows ? (*ar / (NETDATA_DOUBLE)gbc) : *ar;

                if(aggregation == RRDR_GROUP_BY_FUNCTION_AVERAGE && !query_target_aggregatable(qt))
                    n = (*cn /= gbc);
                else
                    n = *cn;

                if(!query_target_aggregatable(qt))
                    *ar /= gbc;

                if(islessgreater(n, 0.0))
                    points_nonzero++;

                if(unlikely(!count))
                    min = max = n;
                else {
                    if(n < min)
                        min = n;

                    if(n > max)
                        max = n;
                }

                if(unlikely(!global_min_max_values++))
                    global_min = global_max = n;
                else {
                    if(n < global_min)
                        global_min = n;

                    if(n > global_max)
                        global_max = n;
                }

                count += stats_by_rows ? 1 : gbc;
            }
        }

        if(points_nonzero) {
            r->od[d] |= RRDR_DIMENSION_NONZERO;
            dimensions_nonzero++;
        }

        r->dview[d] = (STORAGE_POINT) {
            .sum = sum,
            .count = count,
            .min = min,
            .max = max,
            .anomaly_count = (size_t)(ars * RRDR_DVIEW_ANOMALY_COUNT_MULTIPLIER / 100.0),
        };
    }

    r->view.min = global_min;
    r->view.max = global_max;

    if(!dimensions_nonzero && (qt->window.options & RRDR_OPTION_NONZERO)) {
        // all dimensions are zero
        // remove the nonzero option
        qt->window.options &= ~RRDR_OPTION_NONZERO;
    }

    rrd2rrdr_convert_values_to_percentage_of_total(r);

    // update query instance counts in query host and query context
    {
        size_t h = 0, c = 0, i = 0;
        for(; h < qt->nodes.used ; h++) {
            QUERY_NODE *qn = &qt->nodes.array[h];

            for(; c < qt->contexts.used ;c++) {
                QUERY_CONTEXT *qc = &qt->contexts.array[c];

                if(!rrdcontext_acquired_belongs_to_host(qc->rca, qn->rrdhost))
                    break;

                for(; i < qt->instances.used ;i++) {
                    QUERY_INSTANCE *qi = &qt->instances.array[i];

                    if(!rrdinstance_acquired_belongs_to_context(qi->ria, qc->rca))
                        break;

                    if(qi->metrics.queried) {
                        qc->instances.queried++;
                        qn->instances.queried++;
                    }
                    else if(qi->metrics.failed) {
                        qc->instances.failed++;
                        qn->instances.failed++;
                    }
                }
            }
        }
    }

    return r;
}

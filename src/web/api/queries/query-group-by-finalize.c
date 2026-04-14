// SPDX-License-Identifier: GPL-3.0-or-later

#include "query-internal.h"

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

            else
                cn[d] = n * 100.0 / (n + h);
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
    rrdr2rrdr_group_by_calculate_percentage_of_group(last_r);

    RRDR *r = last_r->group_by.r;
    size_t pass = 0;
    while(r) {
        pass++;
        for(size_t d = 0; d < last_r->d ;d++) {
            rrd2rrdr_group_by_add_metric(r, last_r->dgbs[d], last_r, d,
                                         qt->request.group_by[pass].aggregation,
                                         &last_r->dqp[d], pass);
        }
        rrdr2rrdr_group_by_calculate_percentage_of_group(r);

        last_r = r;
        r = last_r->group_by.r;
    }

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

                if(gbc != r->dgbc[d])
                    *co |= RRDR_VALUE_PARTIAL;

                NETDATA_DOUBLE n;

                sum += *cn;
                ars += *ar;

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

                count += gbc;
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

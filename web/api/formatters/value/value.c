// SPDX-License-Identifier: GPL-3.0-or-later

#include "value.h"


inline NETDATA_DOUBLE rrdr2value(RRDR *r, long i, RRDR_OPTIONS options, int *all_values_are_null, NETDATA_DOUBLE *anomaly_rate) {
    size_t c;

    NETDATA_DOUBLE *cn = &r->v[ i * r->d ];
    RRDR_VALUE_FLAGS *co = &r->o[ i * r->d ];
    NETDATA_DOUBLE *ar = &r->ar[ i * r->d ];

    NETDATA_DOUBLE sum = 0, min = 0, max = 0, v;
    int all_null = 1, init = 1;

    NETDATA_DOUBLE total = 1;
    NETDATA_DOUBLE total_anomaly_rate = 0;

    int set_min_max = 0;
    if(unlikely(options & RRDR_OPTION_PERCENTAGE)) {
        total = 0;
        for (c = 0; c < r->d ; c++) {
            if(unlikely(!(r->od[c] & RRDR_DIMENSION_QUERIED))) continue;
            NETDATA_DOUBLE n = cn[c];

            if(likely((options & RRDR_OPTION_ABSOLUTE) && n < 0))
                n = -n;

            total += n;
        }
        // prevent a division by zero
        if(total == 0) total = 1;
        set_min_max = 1;
    }

    // for each dimension
    for (c = 0; c < r->d ; c++) {
        if(!rrdr_dimension_should_be_exposed(r->od[c], options))
            continue;

        NETDATA_DOUBLE n = cn[c];

        if(likely((options & RRDR_OPTION_ABSOLUTE) && n < 0))
            n = -n;

        if(unlikely(options & RRDR_OPTION_PERCENTAGE)) {
            n = n * 100 / total;

            if(unlikely(set_min_max)) {
                r->view.min = r->view.max = n;
                set_min_max = 0;
            }

            if(n < r->view.min) r->view.min = n;
            if(n > r->view.max) r->view.max = n;
        }

        if(unlikely(init)) {
            if(n > 0) {
                min = 0;
                max = n;
            }
            else {
                min = n;
                max = 0;
            }
            init = 0;
        }

        if(likely(!(co[c] & RRDR_VALUE_EMPTY))) {
            all_null = 0;
            sum += n;
        }

        if(n < min) min = n;
        if(n > max) max = n;

        total_anomaly_rate += ar[c];
    }

    if(anomaly_rate) {
        if(!r->d) *anomaly_rate = 0;
        else *anomaly_rate = total_anomaly_rate / (NETDATA_DOUBLE)r->d;
    }

    if(unlikely(all_null)) {
        if(likely(all_values_are_null))
            *all_values_are_null = 1;
        return 0;
    }
    else {
        if(likely(all_values_are_null))
            *all_values_are_null = 0;
    }

    if(options & RRDR_OPTION_MIN2MAX)
        v = max - min;
    else
        v = sum;

    return v;
}

QUERY_VALUE rrdmetric2value(RRDHOST *host,
                            struct rrdcontext_acquired *rca, struct rrdinstance_acquired *ria, struct rrdmetric_acquired *rma,
                            time_t after, time_t before,
                            RRDR_OPTIONS options, RRDR_TIME_GROUPING group_method, const char *group_options,
                            size_t tier, time_t timeout, QUERY_SOURCE query_source, STORAGE_PRIORITY priority
) {
    QUERY_TARGET_REQUEST qtr = {
            .version = 1,
            .host = host,
            .rca = rca,
            .ria = ria,
            .rma = rma,
            .after = after,
            .before = before,
            .points = 1,
            .options = options,
            .time_group_method = group_method,
            .time_group_options = group_options,
            .tier = tier,
            .timeout = timeout,
            .query_source = query_source,
            .priority = priority,
    };

    ONEWAYALLOC *owa = onewayalloc_create(16 * 1024);
    RRDR *r = rrd2rrdr(owa, query_target_create(&qtr));

    QUERY_VALUE qv;

    if(!r || rrdr_rows(r) == 0) {
        qv = (QUERY_VALUE) {
                .value = NAN,
                .anomaly_rate = NAN,
        };
    }
    else {
        qv = (QUERY_VALUE) {
                .after = r->view.after,
                .before = r->view.before,
                .points_read = r->stats.db_points_read,
                .result_points = r->stats.result_points_generated,
        };

        for(size_t t = 0; t < storage_tiers ;t++)
            qv.storage_points_per_tier[t] = r->internal.qt->db.tiers[t].points;

        long i = (!(options & RRDR_OPTION_REVERSED))?(long)rrdr_rows(r) - 1:0;
        int all_values_are_null = 0;
        qv.value = rrdr2value(r, i, options, &all_values_are_null, &qv.anomaly_rate);
        if(all_values_are_null) {
            qv.value = NAN;
            qv.anomaly_rate = NAN;
        }
    }

    rrdr_free(owa, r);
    onewayalloc_destroy(owa);

    return qv;
}

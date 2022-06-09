// SPDX-License-Identifier: GPL-3.0-or-later

#include "daemon/common.h"
#include "KolmogorovSmirnovDist.h"

#define MAX_POINTS 10000
int enable_metric_correlations = CONFIG_BOOLEAN_NO;
int metric_correlations_version = 1;

struct charts {
    RRDSET *st;
    struct charts *next;
};

struct per_dim {
    char *dimension;
    calculated_number baseline[MAX_POINTS];
    calculated_number highlight[MAX_POINTS];

    double baseline_diffs[MAX_POINTS];
    double highlight_diffs[MAX_POINTS];
};

static long int binarySearch(double arr[], long int left, long int right, double K) {
    while (left <= right) {
        long int m = left + (right - left) / 2;

        // Check if x is present at mid
        if (arr[m] == K)
            return m;

        // If x greater, ignore left half
        if (arr[m] < K)
            left = m + 1;

        // If x is smaller, ignore right half
        else
            right = m - 1;
    }

    // if we reach here, then element was not present
    return left;
}

int compare_doubles(const void *left, const void *right) {
    double lt = *(double *)left;
    double rt = *(double *)right;

    // https://stackoverflow.com/a/3886497/1114110
    return (lt > rt) - (lt < rt);
}

static double kstwo(double data1[], long int n1, double data2[], long int n2) {
    qsort(data1, n1, sizeof(double), compare_doubles);
    qsort(data2, n2, sizeof(double), compare_doubles);

    double en1 = (double)n1;
	double en2 = (double)n2;

    double min, max;

    // initialize min and max using the first number of data1
    min = max = 0 - (binarySearch(data2, 0, n2 - 1, data1[0]) / en2);

    for(int i = 1; i < n1 ; i++) {
        double K = data1[i];
        double cdf1, cdf2, cddiffs;

        cdf1 = i / en1;
        cdf2 = binarySearch(data2, 0, n2 - 1, K) / en2;

        cddiffs = cdf1 - cdf2;
        if(cddiffs < min) min = cddiffs;
        if(cddiffs > max) max = cddiffs;
    }

    for(int i = 0; i < n2 ; i++) {
        double K = data2[i];
        double cdf1, cdf2, cddiffs;

        cdf1 = binarySearch(data1, 0, n1 - 1, K) / en1;
        cdf2 = i / en2;

        cddiffs = cdf1 - cdf2;
        if(cddiffs < min) min = cddiffs;
        if(cddiffs > max) max = cddiffs;
    }

    // clip min
    if (fabs(min) > 1) min = 1;

    double d;
    if (fabs(min) < max) d = max;
    else d = fabs(min);

    if(isnan(d)) d = 0.0;

    double en = en1 * en2 / (en1 + en2);

    return KSfbar((int)round(en), d);
}

static void fill_nan (struct per_dim *d, long int hp, long int bp) {
    int k;

    for (k = 0; k < bp; k++) {
        if (isnan(d->baseline[k])) {
            d->baseline[k] = 0.0;
        }
    }

    for (k = 0; k < hp; k++) {
        if (isnan(d->highlight[k])) {
            d->highlight[k] = 0.0;
        }
    }
}

// TODO check counters
static void run_diffs_and_rev (struct per_dim *d, long int hp, long int bp)
{
    int k, j;

    for (k = 0, j = bp; k < bp - 1; k++, j--)
        d->baseline_diffs[k] = (double)d->baseline[j - 2] - (double)d->baseline[j - 1];

    for (k = 0, j = hp; k < hp - 1; k++, j--) {
        d->highlight_diffs[k] = (double)d->highlight[j - 2] - (double)d->highlight[j - 1];
    }
}

static int run_metric_correlations(BUFFER *wb, RRDSET *st, long long baseline_after, long long baseline_before, long long highlight_after, long long highlight_before, long long max_points) {
    uint32_t options = 0x00000000;
    int group_method = RRDR_GROUPING_AVERAGE;
    long group_time = 0;
    struct context_param  *context_param_list = NULL;
    long c;
    int i=0, j=0;
    int b_dims = 0;
    long int baseline_points = 0, highlight_points = 0;

    struct per_dim *pd = NULL;

    // TODO get everything in one go, when baseline is right before highlight

    // get baseline
    ONEWAYALLOC *owa = onewayalloc_create(0);
    RRDR *rb = rrd2rrdr(owa, st, max_points, baseline_after, baseline_before, group_method, group_time, options, NULL, context_param_list, 0);
    if(!rb) {
        info("Cannot generate metric correlations output with these parameters on this chart.");
        onewayalloc_destroy(owa);
        return 0;
    } else {
        baseline_points = rrdr_rows(rb);
        pd = mallocz(sizeof(struct per_dim) * rb->d);
        b_dims = rb->d;
        for (c = 0; c != rrdr_rows(rb) ; ++c) {
            RRDDIM *d;
            for (j = 0, d = rb->st->dimensions ; d && j < rb->d ; ++j, d = d->next) {
                calculated_number *cn = &rb->v[ c * rb->d ];
                if (!c) {
                    //TODO use points from query
                    pd[j].dimension = strdupz (d->name);
                    pd[j].baseline[c] = cn[j];
                } else {
                    pd[j].baseline[c] = cn[j];
                }
            }
        }
    }
    rrdr_free(owa, rb);
    onewayalloc_destroy(owa);
    if (!pd)
        return 0;

    //get highlight
    owa = onewayalloc_create(0);
    RRDR *rh = rrd2rrdr(owa, st, max_points, highlight_after, highlight_before, group_method, group_time, options, NULL, context_param_list, 0);
    if(!rh) {
        info("Cannot generate metric correlations output with these parameters on this chart.");
        freez(pd);
        onewayalloc_destroy(owa);
        return 0;
    } else {
        if (rh->d != b_dims) {
            //TODO handle different dims
            rrdr_free(owa, rh);
            onewayalloc_destroy(owa);
            freez(pd);
            return 0;
        }
        highlight_points = rrdr_rows(rh);
        for (c = 0; c != rrdr_rows(rh) ; ++c) {
            RRDDIM *d;
            for (j = 0, d = rh->st->dimensions ; d && j < rh->d ; ++j, d = d->next) {
                calculated_number *cn = &rh->v[ c * rh->d ];
                    pd[j].highlight[c] = cn[j];
            }
        }
    }
    rrdr_free(owa, rh);
    onewayalloc_destroy(owa);

    for(i = 0; i < b_dims ; i++)
        fill_nan(&pd[i], highlight_points, baseline_points);

    for(i = 0; i < b_dims ; i++)
        run_diffs_and_rev(&pd[i], highlight_points, baseline_points);

    for(i = 0 ; i < j ; i++) {
        if (baseline_points && highlight_points) {
            double prob = kstwo(pd[i].baseline_diffs, baseline_points-1, pd[i].highlight_diffs, highlight_points-1);

            buffer_sprintf(wb, "\t\t\t\t\"%s\": %f", pd[i].dimension, prob);
            if (i != j-1)
                buffer_sprintf(wb, ",\n");
            else
                buffer_sprintf(wb, "\n");
        }
    }

    freez(pd);
    return j;
}

void metric_correlations(RRDHOST *host, BUFFER *wb, long long baseline_after, long long baseline_before, long long highlight_after, long long highlight_before, long long max_points) {

    usec_t started_t = now_realtime_usec();

    info ("Running metric correlations, highlight_after: %lld, highlight_before: %lld, baseline_after: %lld, baseline_before: %lld, max_points: %lld", highlight_after, highlight_before, baseline_after, baseline_before, max_points);

    if (!enable_metric_correlations) {
        error("Metric correlations functionality is not enabled.");
        buffer_strcat(wb, "{\"error\": \"Metric correlations functionality is not enabled.\" }");
        return;
    }

    if (highlight_before <= highlight_after || baseline_before <= baseline_after) {
        error("Invalid baseline or highlight ranges.");
        buffer_strcat(wb, "{\"error\": \"Invalid baseline or highlight ranges.\" }");
        return;
    }

    long long dims = 0, total_dims = 0;
    RRDSET *st;
    size_t c = 0;
    BUFFER *wdims = buffer_create(1000);

    if (!max_points || max_points > MAX_POINTS)
        max_points = MAX_POINTS;

    //dont lock here and wait for results
    //get the charts and run mc after
    //should not be a problem for the query
    struct charts *charts = NULL;
    rrdhost_rdlock(host);
    rrdset_foreach_read(st, host) {
        if (rrdset_is_available_for_viewers(st)) {
            rrdset_rdlock(st);
            struct charts *chart = callocz(1, sizeof(struct charts));
            chart->st = st;
            chart->next = NULL;
            if (charts) {
                chart->next = charts;
            }
            charts = chart;
        }
    }
    rrdhost_unlock(host);

    buffer_strcat(wb, "{\n\t\"correlated_charts\": {");

    for (struct charts *ch = charts; ch; ch = ch->next) {
        buffer_flush(wdims);
        dims = run_metric_correlations(wdims, ch->st, baseline_after, baseline_before, highlight_after, highlight_before, max_points);
        if (dims) {
            if (c) buffer_strcat(wb, "\t\t},");

            buffer_strcat(wb, "\n\t\t\"");
            buffer_strcat(wb, ch->st->id);
            buffer_strcat(wb, "\": {\n");
            buffer_strcat(wb, "\t\t\t\"context\": \"");
            buffer_strcat(wb, ch->st->context);
            buffer_strcat(wb, "\",\n\t\t\t\"dimensions\": {\n");
            buffer_sprintf(wb, "%s", buffer_tostring(wdims));
            buffer_strcat(wb, "\t\t\t}\n");
            total_dims += dims;
            c++;
        }
    }
    buffer_strcat(wb, "\t\t}\n");
    buffer_sprintf(wb, "\t},\n\t\"total_dimensions_count\": %lld\n}", total_dims);

    if (!total_dims) {
        buffer_flush(wb);
        buffer_strcat(wb, "{\"error\": \"No results from metric correlations.\" }");
    }

    struct charts* ch;
    while(charts){
        ch = charts;
        charts = charts->next;
        rrdset_unlock(ch->st);
        free(ch);
    }

    buffer_free(wdims);

    usec_t ended_t = now_realtime_usec();

    info ("Done running metric correlations in %llu usec", ended_t -started_t);
}

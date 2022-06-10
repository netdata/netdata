// SPDX-License-Identifier: GPL-3.0-or-later

#include "daemon/common.h"
#include "KolmogorovSmirnovDist.h"

#define MAX_POINTS 10000
int enable_metric_correlations = CONFIG_BOOLEAN_YES;
int metric_correlations_version = 1;

typedef long long int DIFFS_NUMBERS;
#define DOUBLE_TO_INT_MULTIPLIER 1000000

struct charts {
    RRDSET *st;
    struct charts *next;
};

struct per_dim {
    char *dimension;
    calculated_number baseline[MAX_POINTS];
    calculated_number highlight[MAX_POINTS];

    DIFFS_NUMBERS baseline_diffs[MAX_POINTS];
    DIFFS_NUMBERS highlight_diffs[MAX_POINTS];
};

static inline int binary_search_bigger_than(const DIFFS_NUMBERS arr[], int left, int size, DIFFS_NUMBERS K) {
    // binary search to find the index the smallest index
    // of the first value in the array that is greater than K

    int right = size;
    while(left < right) {
        int middle = (int)(((unsigned int)(left + right)) >> 1);

        if(arr[middle] > K)
            right = middle;

        else
            left = middle + 1;
    }

    return left;
}

int compare_diffs(const void *left, const void *right) {
    DIFFS_NUMBERS lt = *(DIFFS_NUMBERS *)left;
    DIFFS_NUMBERS rt = *(DIFFS_NUMBERS *)right;

    // https://stackoverflow.com/a/3886497/1114110
    return (lt > rt) - (lt < rt);
}

static double kstwo(DIFFS_NUMBERS base[], int base_size, DIFFS_NUMBERS high[], int high_size, uint32_t base_shifts) {
    qsort(base, base_size, sizeof(DIFFS_NUMBERS), compare_diffs);
    qsort(high, high_size, sizeof(DIFFS_NUMBERS), compare_diffs);

    // initialize min and max using the first number of data1
    DIFFS_NUMBERS K = base[0];
    int cdf1 = binary_search_bigger_than(base, 1, base_size, K);
    int cdf2 = binary_search_bigger_than(high, 0, high_size, K);
    int delta = (cdf1 >> base_shifts) - cdf2;
    int min = delta, max = delta;
    int min1 = cdf1, min2 = cdf2;
    int max1 = cdf1, max2 = cdf2;

    // do the first set starting from 1 (we did position 0 above)
    for(int i = 1; i < base_size; i++) {
        K = base[i];
        cdf1 = binary_search_bigger_than(base, i + 1, base_size, K); // starting from i, since data1 is sorted
        cdf2 = binary_search_bigger_than(high, 0, high_size, K);

        delta = (cdf1 >> base_shifts) - cdf2;
        if(delta < min) {
            min = delta;
            min1 = cdf1;
            min2 = cdf2;
        }
        else if(delta > max) {
            max = delta;
            max1 = cdf1;
            max2 = cdf2;
        }
    }

    // do the second set
    for(int i = 0; i < high_size; i++) {
        K = high[i];
        cdf1 = binary_search_bigger_than(base, 0, base_size, K);
        cdf2 = binary_search_bigger_than(high, i + 1, high_size, K); // starting from i, since data2 is sorted

        delta = (cdf1 >> base_shifts) - cdf2;
        if(delta < min) {
            min = delta;
            min1 = cdf1;
            min2 = cdf2;
        }
        else if(delta > max) {
            max = delta;
            max1 = cdf1;
            max2 = cdf2;
        }
    }

    double en1 = (double)base_size;
    double en2 = (double)high_size;
    double emin = ((double)min1 / en1) - ((double)min2 / en2);
    double emax = ((double)max1 / en1) - ((double)max2 / en2);

    if (fabs(emin) > 1) emin = 1.0;

    double d;
    if (fabs(emin) < emax) d = emax;
    else d = fabs(emin);

    double en = en1 * en2 / (en1 + en2);
    return KSfbar((int)round(en), d);
}

static void calculate_pairs_diff(DIFFS_NUMBERS *diffs, calculated_number *arr, size_t size) {
    DIFFS_NUMBERS *diffs_end = &diffs[size - 1];
    arr = &arr[size - 1];

    while(diffs <= diffs_end) {
        calculated_number second = *arr--;
        calculated_number first  = *arr;
        *diffs++ = (DIFFS_NUMBERS)((first - second) * (calculated_number)DOUBLE_TO_INT_MULTIPLIER);
    }
}

static int run_metric_correlations(BUFFER *wb, RRDSET *st, long long baseline_after, long long baseline_before, long long highlight_after, long long highlight_before, long long max_points, uint32_t shifts) {
    RRDR_OPTIONS options = RRDR_OPTION_NULL2ZERO;
    int group_method = RRDR_GROUPING_AVERAGE;
    long group_time = 0;
    struct context_param  *context_param_list = NULL;
    long c;
    int i=0, j=0;
    int b_dims = 0;
    int baseline_points = 0, highlight_points = 0;

    struct per_dim *pd = NULL;

    // TODO get everything in one go, when baseline is right before highlight

    // fprintf(stderr, "Quering highlight for chart '%s'\n", st->name);

    // get first the highlight to find the number of points available
    ONEWAYALLOC *owa = onewayalloc_create(0);
    RRDR *rrdr = rrd2rrdr(owa, st, max_points, highlight_after, highlight_before, group_method, group_time, options, NULL, context_param_list, 0);
    if(!rrdr) {
        info("Cannot generate metric correlations output with these parameters on this chart.");
        onewayalloc_destroy(owa);
        return 0;
    }

    highlight_points = rrdr_rows(rrdr);
    b_dims = rrdr->d;
    pd = mallocz(sizeof(struct per_dim) * rrdr->d);
    for (c = 0; c != rrdr_rows(rrdr) ; ++c) {
        RRDDIM *d;
        for (j = 0, d = rrdr->st->dimensions ; d && j < rrdr->d ; ++j, d = d->next) {
            if(unlikely(!c)) {
                // TODO use points from query
                pd[j].dimension = strdupz(d->name);
            }

            calculated_number *cn = &rrdr->v[ c * rrdr->d ];
            pd[j].highlight[c] = cn[j];
        }
    }
    rrdr_free(owa, rrdr);
    onewayalloc_destroy(owa);
    if (!pd) return 0;

    // fprintf(stderr, "Quering baseline for chart '%s' for points %d\n", st->name, highlight_points);

    // get the baseline, requesting the same number of points as the highlight
    owa = onewayalloc_create(0);
    rrdr = rrd2rrdr(owa, st, highlight_points << shifts, baseline_after, baseline_before, group_method, group_time, options, NULL, context_param_list, 0);
    if(!rrdr) {
        info("Cannot generate metric correlations output with these parameters on this chart.");
        freez(pd);
        onewayalloc_destroy(owa);
        return 0;
    }
    if (rrdr->d != b_dims) {
        // TODO handle different dims
        info("Cannot generate metric correlations output when the baseline and the highlight have different number of dimensions.");
        rrdr_free(owa, rrdr);
        onewayalloc_destroy(owa);
        freez(pd);
        return 0;
    }

    baseline_points = rrdr_rows(rrdr);
    for (c = 0; c != rrdr_rows(rrdr) ; ++c) {
        RRDDIM *d;
        // TODO - the dimensions may have been returned in different order
        for (j = 0, d = rrdr->st->dimensions ; d && j < rrdr->d ; ++j, d = d->next) {
            calculated_number *cn = &rrdr->v[ c * rrdr->d ];
                pd[j].baseline[c] = cn[j];
        }
    }
    rrdr_free(owa, rrdr);
    onewayalloc_destroy(owa);

    for(i = 0; i < b_dims ; i++) {
        calculate_pairs_diff(pd[i].baseline_diffs, pd[i].baseline, baseline_points);
        calculate_pairs_diff(pd[i].highlight_diffs, pd[i].highlight, highlight_points);
    }

    for(i = 0 ; i < j ; i++) {
        if (baseline_points && highlight_points) {

            // calculate_pairs_diff() produces one point less than in the data series
            double prob = kstwo(pd[i].baseline_diffs, baseline_points - 1, pd[i].highlight_diffs, highlight_points - 1, shifts);

            // fprintf(stderr, "kstwo %d = %s:%s:%f\n", gettid(), st->name, pd[i].dimension, prob);

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

    if (!max_points || max_points > MAX_POINTS)
        max_points = MAX_POINTS;

    // baseline should be a power of two multiple of highlight
    uint32_t shifts = 0;
    {
        long long base_delta = baseline_before - baseline_after;
        long long high_delta = highlight_before - highlight_after;
        uint32_t multiplier = (uint32_t)round((double)base_delta / (double)high_delta);

        // check if the ratio is a power of two
        // https://stackoverflow.com/a/600306/1114110
        if((multiplier & (multiplier - 1)) != 0) {
            // it is not power of two
            // let's find the closest power of two
            // https://stackoverflow.com/a/466242/1114110
            multiplier--;
            multiplier |= multiplier >> 1;
            multiplier |= multiplier >> 2;
            multiplier |= multiplier >> 4;
            multiplier |= multiplier >> 8;
            multiplier |= multiplier >> 16;
            multiplier++;
        }

        while(multiplier > 1) {
            shifts++;
            multiplier = multiplier >> 1;
        }

        // if the baseline size will not fit in our buffers
        // lower the window of the baseline
        while(shifts && (max_points << shifts) > MAX_POINTS)
            shifts--;

        // if the baseline size still does not fit our buffer
        // lower the resolution of the highlight and the baseline
        while((max_points << shifts) > MAX_POINTS)
            max_points = max_points >> 1;

        // adjust the baseline to be multiplier times bigger than the highlight
        long long baseline_after_new = baseline_before - (high_delta << shifts);
        if(baseline_after_new != baseline_after)
            info("Metric correlations, adjusting baseline to be %d times bigger than highlight, from %lld to %lld", 1 << shifts, baseline_after, baseline_after_new);
        baseline_after = baseline_after_new;
    }

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
        dims = run_metric_correlations(wdims, ch->st, baseline_after, baseline_before, highlight_after, highlight_before, max_points, shifts);
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

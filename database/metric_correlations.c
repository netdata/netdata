// SPDX-License-Identifier: GPL-3.0-or-later

#include "daemon/common.h"
#include "KolmogorovSmirnovDist.h"

#define MAX_POINTS 10000
int enable_metric_correlations = CONFIG_BOOLEAN_YES;
int metric_correlations_version = 1;

typedef long int DIFFS_NUMBERS;
#define DOUBLE_TO_INT_MULTIPLIER 100000

struct charts {
    RRDSET *st;
    struct charts *next;
};

struct per_dim {
    char *dimension;
    calculated_number baseline[MAX_POINTS];
    calculated_number highlight[MAX_POINTS];
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

static size_t calculate_pairs_diff(DIFFS_NUMBERS *diffs, calculated_number *arr, size_t size) {
    DIFFS_NUMBERS *started_diffs = diffs;
    calculated_number *last = &arr[size - 1];

    while(last > arr) {
        calculated_number second = *last--;
        calculated_number first  = *last;
        *diffs++ = (DIFFS_NUMBERS)((first - second) * (calculated_number)DOUBLE_TO_INT_MULTIPLIER);
    }

    return (diffs - started_diffs) / sizeof(DIFFS_NUMBERS);
}

static double kstwo(calculated_number baseline[], int baseline_points, calculated_number highlight[], int highlight_points, uint32_t base_shifts) {

    DIFFS_NUMBERS baseline_diffs[baseline_points - 1];
    DIFFS_NUMBERS highlight_diffs[highlight_points - 1];

    int base_size = (int)calculate_pairs_diff(baseline_diffs, baseline, baseline_points);
    int high_size = (int)calculate_pairs_diff(highlight_diffs, highlight, highlight_points);

    qsort(baseline_diffs, base_size, sizeof(DIFFS_NUMBERS), compare_diffs);
    qsort(highlight_diffs, high_size, sizeof(DIFFS_NUMBERS), compare_diffs);

    // initialize min and max using the first number of data1
    DIFFS_NUMBERS K = baseline_diffs[0];
    int base_idx = binary_search_bigger_than(baseline_diffs, 1, base_size, K);
    int high_idx = binary_search_bigger_than(highlight_diffs, 0, high_size, K);
    int delta = (base_idx >> base_shifts) - high_idx;
    int min = delta, max = delta;
    int base_min_idx = base_idx;
    int base_max_idx = base_idx;
    int high_min_idx = high_idx;
    int high_max_idx = high_idx;

    // do the first set starting from 1 (we did position 0 above)
    for(int i = 1; i < base_size; i++) {
        K = baseline_diffs[i];
        base_idx = binary_search_bigger_than(baseline_diffs, i + 1, base_size, K); // starting from i, since data1 is sorted
        high_idx = binary_search_bigger_than(highlight_diffs, 0, high_size, K);

        delta = (base_idx >> base_shifts) - high_idx;
        if(delta < min) {
            min = delta;
            base_min_idx = base_idx;
            high_min_idx = high_idx;
        }
        else if(delta > max) {
            max = delta;
            base_max_idx = base_idx;
            high_max_idx = high_idx;
        }
    }

    // do the second set
    for(int i = 0; i < high_size; i++) {
        K = highlight_diffs[i];
        base_idx = binary_search_bigger_than(baseline_diffs, 0, base_size, K);
        high_idx = binary_search_bigger_than(highlight_diffs, i + 1, high_size, K); // starting from i, since data2 is sorted

        delta = (base_idx >> base_shifts) - high_idx;
        if(delta < min) {
            min = delta;
            base_min_idx = base_idx;
            high_min_idx = high_idx;
        }
        else if(delta > max) {
            max = delta;
            base_max_idx = base_idx;
            high_max_idx = high_idx;
        }
    }

    double base_length = (double)base_size;
    double high_length = (double)high_size;
    double dmin = ((double)base_min_idx / base_length) - ((double)high_min_idx / high_length);
    double dmax = ((double)base_max_idx / base_length) - ((double)high_max_idx / high_length);

    double d;

    if (fabs(dmin) > 1)
        dmin = 1.0;

    if (fabs(dmin) < dmax) d = dmax;
    else d = fabs(dmin);

    double en = base_length * high_length / (base_length + high_length);
    return KSfbar((int)round(en), d);
}

static int rrdset_metric_correlations(BUFFER *wb, RRDSET *st, long long baseline_after, long long baseline_before, long long highlight_after, long long highlight_before, long long max_points, uint32_t shifts) {
    RRDR_OPTIONS options = RRDR_OPTION_NULL2ZERO;
    int group_method = RRDR_GROUPING_AVERAGE;
    long group_time = 0;
    struct context_param  *context_param_list = NULL;
    long c;
    int i=0, j=0;
    int dim_count = 0;
    int baseline_points = 0, highlight_points = 0;

    struct per_dim *pd = NULL;

    // TODO get everything in one go, when baseline is right before highlight

    // fprintf(stderr, "Quering highlight for chart '%s'\n", st->name);

    // get first the highlight to find the number of points available
    ONEWAYALLOC *owa = onewayalloc_create(0);
    RRDR *rrdr = rrd2rrdr(owa, st, max_points, highlight_after, highlight_before, group_method, group_time, options, NULL, context_param_list, 0);
    if(!rrdr) {
        info("Metric correlations: rrd2rrdr() failed for the highlighted window on chart '%s'.", st->name);
        onewayalloc_destroy(owa);
        return 0;
    }

    // initialize the dataset for the dimensions we need
    dim_count = rrdr->d;
    if(!dim_count) {
        info("Metric correlations: rrd2rrdr() did not return any dimensions on chart '%s'.", st->name);
        rrdr_free(owa, rrdr);
        onewayalloc_destroy(owa);
        return 0;
    }

    pd = mallocz(sizeof(struct per_dim) * dim_count);
    RRDDIM *d;
    for (j = 0, d = rrdr->st->dimensions ; d && j < rrdr->d ; ++j, d = d->next)
        pd[j].dimension = strdupz(d->name);

    // copy the highlight points of all dimensions
    highlight_points = rrdr_rows(rrdr);
    for (c = 0; c != highlight_points ; c++) {
        calculated_number *cn = &rrdr->v[ c * rrdr->d ];
        for (j = 0, d = rrdr->st->dimensions ; d && j < rrdr->d ; ++j, d = d->next)
            pd[j].highlight[c] = cn[j];
    }

    rrdr_free(owa, rrdr);
    onewayalloc_destroy(owa);

    // fprintf(stderr, "Quering baseline for chart '%s' for points %d\n", st->name, highlight_points);

    // get the baseline, requesting the same number of points as the highlight
    owa = onewayalloc_create(0);
    rrdr = rrd2rrdr(owa, st, highlight_points << shifts, baseline_after, baseline_before, group_method, group_time, options, NULL, context_param_list, 0);
    if(!rrdr) {
        info("Metric correlations: rrd2rrdr() failed for the baseline window on chart '%s'.", st->name);
        freez(pd);
        onewayalloc_destroy(owa);
        return 0;
    }
    if (rrdr->d != dim_count) {
        // TODO handle different dims

        info("Cannot generate metric correlations for chart '%s' when the baseline and the highlight have different number of dimensions.", st->name);
        rrdr_free(owa, rrdr);
        onewayalloc_destroy(owa);
        freez(pd);
        return 0;
    }

    // copy the baseline points of all dimensions
    baseline_points = rrdr_rows(rrdr);
    for (c = 0; c < baseline_points ; ++c) {
        calculated_number *cn = &rrdr->v[ c * rrdr->d ];
        for (j = 0, d = rrdr->st->dimensions ; d && j < rrdr->d ; ++j, d = d->next)
            pd[j].baseline[c] = cn[j];
    }
    rrdr_free(owa, rrdr);
    onewayalloc_destroy(owa);

    // we need at least 2 points to do the job
    if(baseline_points > 2 && highlight_points > 2) {
        for(i = 0 ; i < dim_count; i++) {
            // calculate_pairs_diff() produces one point less than in the data series
            double prob = kstwo(pd[i].baseline, baseline_points, pd[i].highlight, highlight_points, shifts);

            // fprintf(stderr, "kstwo %d = %s:%s:%f\n", gettid(), st->name, pd[i].dimension, prob);

            buffer_sprintf(wb, "\t\t\t\t\"%s\": %f", pd[i].dimension, prob);
            if (i != j - 1)
                buffer_sprintf(wb, ",\n");
            else
                buffer_sprintf(wb, "\n");
        }
    }

    // free the dimension names
    for (j = 0; j < dim_count ; j++)
        freez(pd[j].dimension);

    // free the dimensions arrays
    freez(pd);
    return j;
}

int metric_correlations(RRDHOST *host, BUFFER *wb, long long baseline_after, long long baseline_before, long long highlight_after, long long highlight_before, long long max_points, long timeout) {

    if (enable_metric_correlations == CONFIG_BOOLEAN_NO) {
        error("Metric correlations: not enabled.");
        buffer_strcat(wb, "{\"error\": \"Metric correlations functionality is not enabled.\" }");
        return HTTP_RESP_BAD_REQUEST;
    }

    if (highlight_before <= highlight_after || baseline_before <= baseline_after) {
        error("Invalid baseline or highlight ranges.");
        buffer_strcat(wb, "{\"error\": \"Invalid baseline or highlight ranges.\" }");
        return HTTP_RESP_BAD_REQUEST;
    }

    // if the user didn't give a timeout
    // assume 20 seconds
    if(!timeout) timeout = 20 * MSEC_PER_SEC;

    // if the timeout is less than 1 second
    // make it at least 1 second
    if(timeout < (long)(1 * MSEC_PER_SEC))
        timeout = 1 * MSEC_PER_SEC;

    // if the number of points is less than 100
    // make them 100 points
    if(max_points < 100)
        max_points = 100;

    usec_t timeout_usec = timeout * USEC_PER_MS;
    usec_t started_t = now_realtime_usec();

    // baseline should be a power of two multiple of highlight
    uint32_t shifts = 0;
    {
        long long base_delta = baseline_before - baseline_after;
        long long high_delta = highlight_before - highlight_after;
        uint32_t multiplier = (uint32_t)round((double)base_delta / (double)high_delta);

        // check if the multiplier is a power of two
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

        // convert the multiplier to the number of shifts
        // we need to do, to divide baseline numbers to match
        // the highlight ones
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
            info("Metric correlations: adjusted baseline to be %d times bigger than highlight, from %lld to %lld", 1 << shifts, baseline_after, baseline_after_new);
        baseline_after = baseline_after_new;
    }

    info ("Running metric correlations, highlight_after: %lld, highlight_before: %lld, baseline_after: %lld, baseline_before: %lld, max_points: %lld, timeout: %ld ms, shifts %u", highlight_after, highlight_before, baseline_after, baseline_before, max_points, timeout, shifts);

    char *error = NULL;
    int resp = HTTP_RESP_OK;

    long long dims = 0, total_dims = 0;
    RRDSET *st;
    size_t c = 0;
    BUFFER *wdims = buffer_create(1000);

    // dont lock here and wait for results
    // get the charts and run mc after
    // should not be a problem for the query
    struct charts *charts = NULL;
    rrdhost_rdlock(host);
    rrdset_foreach_read(st, host) {
        if (rrdset_is_available_for_viewers(st)) {
            rrdset_rdlock(st);
            struct charts *ch = mallocz(sizeof(struct charts));
            ch->st = st;
            ch->next = charts;
            charts = ch;
        }
    }
    rrdhost_unlock(host);

    if(!charts) {
        error = "no charts to correlate";
        resp = HTTP_RESP_NOT_FOUND;
        goto cleanup;
    }

    buffer_strcat(wb, "{\n\t\"correlated_charts\": {");

    for (struct charts *ch = charts; ch; ch = ch->next) {
        if(now_realtime_usec() - started_t > timeout_usec) {
            error = "timed out";
            resp = HTTP_RESP_GATEWAY_TIMEOUT;
            goto cleanup;
        }

        buffer_flush(wdims);
        dims = rrdset_metric_correlations(wdims, ch->st, baseline_after, baseline_before, highlight_after, highlight_before, max_points, shifts);
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

        // unlock the chart as soon as possible
        rrdset_unlock(ch->st);
        ch->st = NULL; // mark it NULL, so that we will not try to unlock it later
    }
    buffer_strcat(wb, "\t\t}\n");
    buffer_sprintf(wb, "\t},\n\t\"total_dimensions_count\": %lld\n}", total_dims);

    if(!total_dims) {
        error = "no results produced from correlations";
        resp = HTTP_RESP_NOT_FOUND;
    }

cleanup:
    buffer_free(wdims);

    if(error) {
        buffer_flush(wb);
        buffer_sprintf(wb, "{\"error\": \"%s\" }", error);
    }

    struct charts* ch;
    while(charts){
        ch = charts;
        charts = charts->next;
        if(ch->st) rrdset_unlock(ch->st);
        free(ch);
    }

    usec_t ended_t = now_realtime_usec();
    info ("Done running metric correlations in %llu usec", ended_t -started_t);

    return resp;
}

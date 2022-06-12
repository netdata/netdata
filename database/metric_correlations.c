// SPDX-License-Identifier: GPL-3.0-or-later

#include "daemon/common.h"
#include "KolmogorovSmirnovDist.h"

#define MAX_POINTS 10000
int enable_metric_correlations = CONFIG_BOOLEAN_YES;
int metric_correlations_version = 1;

// ----------------------------------------------------------------------------
// The results per dimension are aggregated into a dictionary

struct register_result {
    RRDSET *st;
    const char *chart_id;
    const char *context;
    const char *dim_name;
    calculated_number value;
};

static void register_result_insert_callback(const char *name, void *value, void *data) {
    (void)name;
    (void)data;

    struct register_result *t = (struct register_result *)value;

    if(t->chart_id) t->chart_id = strdupz(t->chart_id);
    if(t->context) t->context = strdupz(t->context);
    if(t->dim_name) t->dim_name = strdupz(t->dim_name);
}

static int register_result_delete_callback(const char *name, void *value, void *data) {
    (void)name;
    (void)data;
    struct register_result *t = (struct register_result *)value;

    freez((void *)t->chart_id);
    freez((void *)t->context);
    freez((void *)t->dim_name);

    return 1;
}

static DICTIONARY *register_result_init() {
    DICTIONARY *results = dictionary_create(DICTIONARY_FLAG_SINGLE_THREADED);
    dictionary_register_insert_callback(results, register_result_insert_callback, results);
    // dictionary_register_delete_callback(results, register_result_delete_callback, results);
    return results;
}

static void register_result_destroy(DICTIONARY *results) {
    dictionary_walkthrough_write(results, register_result_delete_callback, results);
    dictionary_destroy(results);
}

static void register_result(DICTIONARY *results, RRDSET *st, RRDDIM *d, calculated_number value) {
    struct register_result t = {
        .st = st,
        .chart_id = st->id,
        .context = st->context,
        .dim_name = d->name,
        .value = value
    };

    char buf[5000 + 1];
    snprintfz(buf, 5000, "%s:%s", st->id, d->name);
    dictionary_set(results, buf, &t, sizeof(struct register_result));
}

// ----------------------------------------------------------------------------
// Generation of JSON output for the results

static size_t registered_results_to_json(DICTIONARY *results, BUFFER *wb) {
    buffer_strcat(wb, "{\n\t\"correlated_charts\": {\n");

    size_t charts = 0, chart_dims = 0, total_dimensions = 0;
    struct register_result *t;
    RRDSET *last_st = NULL; // never access this - we use it only for comparison
    dfe_start_read(results, t) {
        if(!last_st || t->st != last_st) {
            last_st = t->st;

            if(charts) buffer_strcat(wb, "\n\t\t\t}\n\t\t},\n");
            buffer_strcat(wb, "\t\t\"");
            buffer_strcat(wb, t->chart_id);
            buffer_strcat(wb, "\": {\n");
            buffer_strcat(wb, "\t\t\t\"context\": \"");
            buffer_strcat(wb, t->context);
            buffer_strcat(wb, "\",\n\t\t\t\"dimensions\": {\n");
            charts++;
            chart_dims = 0;
        }
        if (chart_dims) buffer_sprintf(wb, ",\n");
        buffer_sprintf(wb, "\t\t\t\t\"%s\": " CALCULATED_NUMBER_FORMAT, t->dim_name, t->value);
        chart_dims++;
        total_dimensions++;
    }
    dfe_done(t);

    // close dimensions and chart
    if (total_dimensions)
        buffer_strcat(wb, "\n\t\t\t}\n\t\t}\n");

    // close correlated_charts
    buffer_sprintf(wb, "\t},\n\t\"total_dimensions_count\": %zu\n}", total_dimensions);

    return total_dimensions;
}

// ----------------------------------------------------------------------------
// KS2 algorithm functions

typedef long int DIFFS_NUMBERS;
#define DOUBLE_TO_INT_MULTIPLIER 100000

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

    // -1 in size, since the calculate_pairs_diffs() returns one less point
    DIFFS_NUMBERS baseline_diffs[baseline_points - 1];
    DIFFS_NUMBERS highlight_diffs[highlight_points - 1];

    int base_size = (int)calculate_pairs_diff(baseline_diffs, baseline, baseline_points);
    int high_size = (int)calculate_pairs_diff(highlight_diffs, highlight, highlight_points);

    qsort(baseline_diffs, base_size, sizeof(DIFFS_NUMBERS), compare_diffs);
    qsort(highlight_diffs, high_size, sizeof(DIFFS_NUMBERS), compare_diffs);

    // Now we should be calculating this:
    //
    // For each number in the diffs arrays, we should find the index of the
    // number bigger than them in both arrays and calculate the % of this index
    // vs the total array size. Once we have the 2 percentages, we should find
    // the min and max across the delta of all of them.
    //
    // It should look like this:
    //
    // base_pcent = binary_search_bigger_than(...) / base_size;
    // high_pcent = binary_search_bigger_than(...) / high_size;
    // delta = base_pcent - high_pcent;
    // if(delta < min) min = delta;
    // if(delta > max) max = delta;
    //
    // This would require a lot of multiplications and divisions.
    //
    // To speed it up, we do the binary search to find the index of each number
    // but then we divide the base index by the power of two number (shifts) it
    // is bigger than high index. So the 2 indexes are now comparable.
    // We also keep track of the original indexes with min and max, to properly
    // calculate their percentages once the loops finish.


    // initialize min and max using the first number of baseline_diffs
    DIFFS_NUMBERS K = baseline_diffs[0];
    int base_idx = binary_search_bigger_than(baseline_diffs, 1, base_size, K);
    int high_idx = binary_search_bigger_than(highlight_diffs, 0, high_size, K);
    int delta = (base_idx >> base_shifts) - high_idx;
    int min = delta, max = delta;
    int base_min_idx = base_idx;
    int base_max_idx = base_idx;
    int high_min_idx = high_idx;
    int high_max_idx = high_idx;

    // do the baseline_diffs starting from 1 (we did position 0 above)
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

    // do the highlight_diffs starting from 0
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

    // now we have the min, max and their indexes
    // properly calculate min and max as dmin and dmax
    double dbase_size = (double)base_size;
    double dhigh_size = (double)high_size;
    double dmin = ((double)base_min_idx / dbase_size) - ((double)high_min_idx / dhigh_size);
    double dmax = ((double)base_max_idx / dbase_size) - ((double)high_max_idx / dhigh_size);

    double d;

    if (fabs(dmin) > 1)
        dmin = 1.0;

    if (fabs(dmin) < dmax) d = dmax;
    else d = fabs(dmin);

    double en = dbase_size * dhigh_size / (dbase_size + dhigh_size);
    return KSfbar((int)round(en), d);
}

static int rrdset_metric_correlations_ks2(RRDSET *st, DICTIONARY *results,
                                      long long baseline_after, long long baseline_before,
                                      long long highlight_after, long long highlight_before,
                                      long long max_points, uint32_t shifts, int timeout_ms) {
    RRDR_OPTIONS options = RRDR_OPTION_NULL2ZERO | RRDR_OPTION_NOT_ALIGNED | RRDR_OPTION_ALLOW_PAST | RRDR_OPTION_NONZERO;
    int group_method = RRDR_GROUPING_AVERAGE;
    long group_time = 0;
    struct context_param  *context_param_list = NULL;

    int correlated_dimensions = 0;

    RRDR *high_rrdr = NULL;
    RRDR *base_rrdr = NULL;

    // get first the highlight to find the number of points available
    usec_t started_usec = now_realtime_usec();
    ONEWAYALLOC *owa = onewayalloc_create(0);
    high_rrdr = rrd2rrdr(owa, st, max_points,
                          highlight_after, highlight_before, group_method,
                          group_time, options, NULL, context_param_list,
                          timeout_ms);
    if(!high_rrdr) {
        info("Metric correlations: rrd2rrdr() failed for the highlighted window on chart '%s'.", st->name);
        goto cleanup;
    }
    if(!high_rrdr->d) {
        info("Metric correlations: rrd2rrdr() did not return any dimensions on chart '%s'.", st->name);
        goto cleanup;
    }
    int high_points = rrdr_rows(high_rrdr);

    usec_t now_usec = now_realtime_usec();
    if(now_usec - started_usec > timeout_ms * USEC_PER_MS)
        goto cleanup;

    // get the baseline, requesting the same number of points as the highlight
    base_rrdr = rrd2rrdr(owa, st,high_points << shifts,
                    baseline_after, baseline_before, group_method,
                    group_time, options, NULL, context_param_list,
                    (int)(timeout_ms - ((now_usec - started_usec) / USEC_PER_MS)));
    if(!base_rrdr) {
        info("Metric correlations: rrd2rrdr() failed for the baseline window on chart '%s'.", st->name);
        goto cleanup;
    }
    if(!base_rrdr->d) {
        info("Metric correlations: rrd2rrdr() did not return any dimensions on chart '%s'.", st->name);
        goto cleanup;
    }
    if (base_rrdr->d != high_rrdr->d) {
        info("Cannot generate metric correlations for chart '%s' when the baseline and the highlight have different number of dimensions.", st->name);
        goto cleanup;
    }
    int base_points = rrdr_rows(base_rrdr);

    now_usec = now_realtime_usec();
    if(now_usec - started_usec > timeout_ms * USEC_PER_MS)
        goto cleanup;

    // we need at least 2 points to do the job
    if(base_points < 2 || high_points < 2)
        goto cleanup;

    // for each dimension
    RRDDIM *d;
    int i;
    for(i = 0, d = base_rrdr->st->dimensions ; d && i < base_rrdr->d; i++, d = d->next) {

        // skip the not evaluated ones
        if(unlikely(!(base_rrdr->od[i] & RRDR_DIMENSION_SELECTED) || !(high_rrdr->od[i] & RRDR_DIMENSION_SELECTED)))
            continue;

        // skip the dimensions that are just zero
        if(unlikely(!(base_rrdr->od[i] & RRDR_DIMENSION_NONZERO) && !(high_rrdr->od[i] & RRDR_DIMENSION_NONZERO)))
            continue;

        // copy the baseline points of the dimension to a contiguous array
        calculated_number baseline[base_points];
        for(int c = 0; c < base_points; c++)
            baseline[c] = base_rrdr->v[ c * base_rrdr->d + i ];

        // copy the highlight points of the dimension to a contiguous array
        calculated_number highlight[high_points];
        for(int c = 0; c < high_points; c++)
            highlight[c] = high_rrdr->v[ c * high_rrdr->d + i ];

        double prob = kstwo(baseline, base_points, highlight, high_points, shifts);
        register_result(results, base_rrdr->st, d, prob);
        correlated_dimensions++;
    }

cleanup:
    rrdr_free(owa, high_rrdr);
    rrdr_free(owa, base_rrdr);
    onewayalloc_destroy(owa);
    return correlated_dimensions;
}

// ----------------------------------------------------------------------------
// VOLUME algorithm functions

static int rrdset_metric_correlations_volume(RRDSET *st, DICTIONARY *results,
                                          long long baseline_after, long long baseline_before,
                                          long long highlight_after, long long highlight_before) {
    RRDR_OPTIONS options = RRDR_OPTION_NULL2ZERO | RRDR_OPTION_NOT_ALIGNED | RRDR_OPTION_ALLOW_PAST | RRDR_OPTION_NONZERO;
    int group_method = RRDR_GROUPING_AVERAGE;
    long group_time = 0;

    int correlated_dimensions = 0;
    int ret, value_is_null;

    RRDDIM *d;
    for(d = st->dimensions; d ; d = d->next) {

        calculated_number highlight_average = NAN;
        value_is_null = 1;
        ret = rrdset2value_api_v1(st, NULL, &highlight_average, d->id, 1, highlight_after, highlight_before,
                                  group_method, group_time, options, NULL, NULL, &value_is_null, 0);

        if(ret != HTTP_RESP_OK || value_is_null || !calculated_number_isnumber(highlight_average)) {
            // error("Metric correlations: cannot query highlight duration of dimension '%s' of chart '%s', %d %s %s %s", st->name, d->name, ret, (ret != HTTP_RESP_OK)?"response failed":"", (value_is_null)?"value is null":"", (!calculated_number_isnumber(highlight_average))?"result is NAN":"");
            // this means no data for the highlighted duration - so skip it
            continue;
        }

        calculated_number baseline_average = NAN;
        value_is_null = 1;
        ret = rrdset2value_api_v1(st, NULL, &baseline_average, d->id, 1, baseline_after, baseline_before,
                                  group_method, group_time, options, NULL, NULL, &value_is_null, 0);

        if(ret != HTTP_RESP_OK || value_is_null || !calculated_number_isnumber(baseline_average)) {
            // error("Metric correlations: cannot query baseline duration of dimension '%s' of chart '%s', %d %s %s %s", st->name, d->name, ret, (ret != HTTP_RESP_OK)?"response failed":"", (value_is_null)?"value is null":"", (!calculated_number_isnumber(baseline_average))?"result is NAN":"");
            // continue;
            // this means no data for the baseline window, but we have data for the highlighted one - assume zero
            baseline_average = 0.0;
        }

        calculated_number pcent = 0.0;
        if(baseline_average > 0.0)
            pcent = (highlight_average - baseline_average) / baseline_average;

        else if(highlight_average > baseline_average)
            pcent = 1000000000.0;

        pcent = calculated_number_fabs(pcent);

        register_result(results, st, d, pcent);
        correlated_dimensions++;
    }

    return correlated_dimensions;
}

int compare_calculated_numbers(const void *left, const void *right) {
    calculated_number lt = *(calculated_number *)left;
    calculated_number rt = *(calculated_number *)right;

    // https://stackoverflow.com/a/3886497/1114110
    return (lt > rt) - (lt < rt);
}

static inline int binary_search_bigger_than_calculated_number(const calculated_number arr[], int left, int size, calculated_number K) {
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

static size_t volume_post_processing(DICTIONARY *results) {
    struct register_result *t;

    // count the dimensions
    // TODO - remove this once rrdlabels is merged and use dictionary_entries()
    size_t dimensions = 0;
    dfe_start_read(results, t)
        dimensions++;
    dfe_done(t);

    if(!dimensions) return 0;

    // create an array of the right size and copy all the values in it
    calculated_number slots[dimensions];
    dimensions = 0;
    dfe_start_read(results, t) {
        slots[dimensions++] = t->value;
    }
    dfe_done(t);

    // sort the array with the values of all dimensions
    qsort(slots, dimensions, sizeof(calculated_number), compare_calculated_numbers);

    // skip the duplicates in the sorted array
    calculated_number last_value = NAN;
    size_t unique_values = 0;
    for(size_t i = 0; i < dimensions ;i++) {
        if(likely(slots[i] != last_value))
            slots[unique_values++] = last_value = slots[i];
    }

    // if we shortened the array, put an unrealistic value past the useful ones
    if(unique_values > 0 && unique_values < dimensions)
        slots[unique_values] = slots[unique_values - 1] * 2;

    // calculate the weight of each slot, using the number of unique values
    calculated_number slot_weight = 1.0 / (calculated_number)unique_values;

    dfe_start_read(results, t) {
        int slot = binary_search_bigger_than_calculated_number(slots, 0, (int)unique_values, t->value);
        calculated_number v = slot * slot_weight;
        if(unlikely(v > 1.0)) v = 1.0;
        v = 1.0 - v;
        t->value = v;
    }
    dfe_done(t);

    return dimensions;
}

// ----------------------------------------------------------------------------
// The main function

int metric_correlations(RRDHOST *host, BUFFER *wb, METRIC_CORRELATIONS_METHOD method,
                        long long baseline_after, long long baseline_before,
                        long long highlight_after, long long highlight_before,
                        long long max_points, int timeout_ms) {

    if (enable_metric_correlations == CONFIG_BOOLEAN_NO) {
        error("Metric correlations: not enabled.");
        buffer_strcat(wb, "{\"error\": \"Metric correlations functionality is not enabled.\" }");
        return HTTP_RESP_FORBIDDEN;
    }

    if (highlight_before <= highlight_after || baseline_before <= baseline_after) {
        error("Invalid baseline or highlight ranges.");
        buffer_strcat(wb, "{\"error\": \"Invalid baseline or highlight ranges.\" }");
        return HTTP_RESP_BAD_REQUEST;
    }

    // if the user didn't give a timeout
    // assume 60 seconds
    if(!timeout_ms)
        timeout_ms = 60 * MSEC_PER_SEC;

    // if the timeout is less than 1 second
    // make it at least 1 second
    if(timeout_ms < (long)(1 * MSEC_PER_SEC))
        timeout_ms = 1 * MSEC_PER_SEC;

    // if the number of points is less than 100
    // make them 100 points
    if(max_points < 100)
        max_points = 100;

    usec_t timeout_usec = timeout_ms * USEC_PER_MS;
    usec_t started_usec = now_realtime_usec();

    DICTIONARY *results = register_result_init();
    DICTIONARY *charts = dictionary_create(DICTIONARY_FLAG_SINGLE_THREADED|DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE);;

    char *error = NULL;
    int resp = HTTP_RESP_OK;

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

        // if the baseline size will not comply to MAX_POINTS
        // lower the window of the baseline
        while(shifts && (max_points << shifts) > MAX_POINTS)
            shifts--;

        // if the baseline size still does not comply to MAX_POINTS
        // lower the resolution of the highlight and the baseline
        while((max_points << shifts) > MAX_POINTS)
            max_points = max_points >> 1;

        if(max_points < 100) {
            error = "cannot comply to at least 100 points";
            resp = HTTP_RESP_BAD_REQUEST;
            goto cleanup;
        }

        // adjust the baseline to be multiplier times bigger than the highlight
        long long baseline_after_new = baseline_before - (high_delta << shifts);
        if(baseline_after_new != baseline_after)
            info("Metric correlations: adjusted baseline to be %d times bigger than highlight, baseline_after moved from %lld to %lld", 1 << shifts, baseline_after, baseline_after_new);
        baseline_after = baseline_after_new;
    }

    info ("Running metric correlations, method %u, highlight_after: %lld, highlight_before: %lld, baseline_after: %lld, baseline_before: %lld, max_points: %lld, timeout: %d ms, shifts %u", method, highlight_after, highlight_before, baseline_after, baseline_before, max_points, timeout_ms, shifts);

    // dont lock here and wait for results
    // get the charts and run mc after
    RRDSET *st;
    rrdhost_rdlock(host);
    rrdset_foreach_read(st, host) {
        if (rrdset_is_available_for_viewers(st))
            dictionary_set(charts, st->name, "", 1);
    }
    rrdhost_unlock(host);

    buffer_strcat(wb, "{\n\t\"correlated_charts\": {");

    size_t total_dims = 0;
    void *ptr;

    // for every chart in the dictionary
    dfe_start_read(charts, ptr) {
        usec_t now_usec = now_realtime_usec();
        if(now_usec - started_usec > timeout_usec) {
            error = "timed out";
            resp = HTTP_RESP_GATEWAY_TIMEOUT;
            goto cleanup;
        }

        st = rrdset_find_byname(host, ptr_name);
        if(!st) continue;

        rrdset_rdlock(st);

        switch(method) {
            case METRIC_CORRELATIONS_VOLUME:
                total_dims += rrdset_metric_correlations_volume(st, results,
                                                             baseline_after, baseline_before,
                                                             highlight_after, highlight_before);
                break;

            default:
            case METRIC_CORRELATIONS_KS2:
                total_dims += rrdset_metric_correlations_ks2(st, results,
                                                             baseline_after, baseline_before,
                                                             highlight_after, highlight_before,
                                                             max_points, shifts,
                                                             (int)(timeout_ms - ((now_usec - started_usec) / USEC_PER_MS)));
                break;
        }

        rrdset_unlock(st);
    }
    dfe_done(ptr);

    // post processing of registered results
    switch(method) {
        case METRIC_CORRELATIONS_VOLUME:
            volume_post_processing(results);
            break;

        default:
        case METRIC_CORRELATIONS_KS2:
            break;
    }

    // generate the json output we need
    buffer_flush(wb);
    total_dims = registered_results_to_json(results, wb);

    if(!total_dims) {
        error = "no results produced from correlations";
        resp = HTTP_RESP_NOT_FOUND;
    }

cleanup:
    if(charts) dictionary_destroy(charts);
    if(results) register_result_destroy(results);

    if(error) {
        buffer_flush(wb);
        buffer_sprintf(wb, "{\"error\": \"%s\" }", error);
    }

    usec_t ended_t = now_realtime_usec();
    info ("Done running metric correlations in %llu usec", ended_t - started_usec);

    return resp;
}

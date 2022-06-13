// SPDX-License-Identifier: GPL-3.0-or-later

#include "daemon/common.h"
#include "KolmogorovSmirnovDist.h"

#define MAX_POINTS 10000
int enable_metric_correlations = CONFIG_BOOLEAN_YES;
int metric_correlations_version = 1;

typedef struct mc_stats {
    size_t db_points;
    size_t result_points;
    size_t db_queries;
    size_t binary_searches;
} MC_STATS;

// ----------------------------------------------------------------------------
// parse and render metric correlations methods

static struct {
    const char *name;
    METRIC_CORRELATIONS_METHOD value;
} metric_correlations_methods[] = {
      { "ks2"         , METRIC_CORRELATIONS_KS2 }
    , { "volume"      , METRIC_CORRELATIONS_VOLUME }
    , { NULL          , 0 }
};

METRIC_CORRELATIONS_METHOD mc_string_to_method(const char *method) {
    for(int i = 0; metric_correlations_methods[i].name ;i++)
        if(strcmp(method, metric_correlations_methods[i].name) == 0)
            return metric_correlations_methods[i].value;

    return METRIC_CORRELATIONS_VOLUME;
}

const char *mc_method_to_string(METRIC_CORRELATIONS_METHOD method) {
    for(int i = 0; metric_correlations_methods[i].name ;i++)
        if(metric_correlations_methods[i].value == method)
            return metric_correlations_methods[i].name;

    return "unknown";
}

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

static size_t registered_results_to_json(DICTIONARY *results, BUFFER *wb,
                                         long long after, long long before,
                                         long long baseline_after, long long baseline_before,
                                         long points, METRIC_CORRELATIONS_METHOD method,
                                         RRDR_GROUPING group, RRDR_OPTIONS options, uint32_t shifts,
                                         size_t correlated_dimensions, usec_t duration, MC_STATS *stats) {

    buffer_sprintf(wb, "{\n"
                       "\t\"after\": %lld,\n"
                       "\t\"before\": %lld,\n"
                       "\t\"duration\": %lld,\n"
                       "\t\"points\": %ld,\n"
                       "\t\"baseline_after\": %lld,\n"
                       "\t\"baseline_before\": %lld,\n"
                       "\t\"baseline_duration\": %lld,\n"
                       "\t\"baseline_points\": %ld,\n"
                       "\t\"statistics\": {\n"
                       "\t\t\"query_time_ms\": %f,\n"
                       "\t\t\"db_queries\": %zu,\n"
                       "\t\t\"db_points_read\": %zu,\n"
                       "\t\t\"query_result_points\": %zu,\n"
                       "\t\t\"binary_searches\": %zu\n"
                       "\t},\n"
                       "\t\"group\": \"%s\",\n"
                       "\t\"method\": \"%s\",\n"
                       "\t\"options\": \"",
                   after,
                   before,
                   before - after,
                   points,
                   baseline_after,
                   baseline_before,
                   baseline_before - baseline_after,
                   points << shifts,
                   (double)duration / (double)USEC_PER_MS,
                   stats->db_queries,
                   stats->db_points,
                   stats->result_points,
                   stats->binary_searches,
                   web_client_api_request_v1_data_group_to_string(group),
                   mc_method_to_string(method));

    web_client_api_request_v1_data_options_to_string(wb, options);
    buffer_strcat(wb, "\",\n\t\"correlated_charts\": {\n");

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
    buffer_sprintf(wb, "\t},\n"
                       "\t\"correlated_dimensions\": %zu,\n"
                       "\t\"total_dimensions_count\": %zu\n"
                       "}\n",
                   total_dimensions,
                   correlated_dimensions // yes, we flip them
                   );

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
    calculated_number *last = &arr[size - 1];
    size_t added = 0;

    while(last > arr) {
        calculated_number second = *last--;
        calculated_number first  = *last;
        *diffs++ = (DIFFS_NUMBERS)((first - second) * (calculated_number)DOUBLE_TO_INT_MULTIPLIER);
        added++;
    }

    return added;
}

static double ks_2samp(DIFFS_NUMBERS baseline_diffs[], int base_size, DIFFS_NUMBERS highlight_diffs[], int high_size, uint32_t base_shifts) {

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
    int delta = base_idx - (high_idx << base_shifts);
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

        delta = base_idx - (high_idx << base_shifts);
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

        delta = base_idx - (high_idx << base_shifts);
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

    dmin = -dmin;
    if(islessequal(dmin, 0.0)) dmin = 0.0;
    else if(isgreaterequal(dmin, 1.0)) dmin = 1.0;

    double d;
    if(isgreaterequal(dmin, dmax)) d = dmin;
    else d = dmax;

    double en = round(dbase_size * dhigh_size / (dbase_size + dhigh_size));

    // under these conditions, KSfbar() crashes
    if(unlikely(isnan(en) || isinf(en) || en == 0.0 || isnan(d) || isinf(d)))
        return NAN;

    return KSfbar((int)en, d);
}

static double kstwo(calculated_number baseline[], int baseline_points, calculated_number highlight[], int highlight_points, uint32_t base_shifts) {
    // -1 in size, since the calculate_pairs_diffs() returns one less point
    DIFFS_NUMBERS baseline_diffs[baseline_points - 1];
    DIFFS_NUMBERS highlight_diffs[highlight_points - 1];

    int base_size = (int)calculate_pairs_diff(baseline_diffs, baseline, baseline_points);
    int high_size = (int)calculate_pairs_diff(highlight_diffs, highlight, highlight_points);

    if(unlikely(!base_size || !high_size))
        return NAN;

    if(unlikely(base_size != baseline_points - 1 || high_size != highlight_points - 1)) {
        error("Metric correlations: internal error - calculate_pairs_diff() returns the wrong number of entries");
        return NAN;
    }

    return ks_2samp(baseline_diffs, base_size, highlight_diffs, high_size, base_shifts);
}


static int rrdset_metric_correlations_ks2(RRDSET *st, DICTIONARY *results,
                                          long long baseline_after, long long baseline_before,
                                          long long after, long long before,
                                          long long points, RRDR_OPTIONS options, RRDR_GROUPING group,
                                          uint32_t shifts, int timeout, MC_STATS *stats) {
    long group_time = 0;
    struct context_param  *context_param_list = NULL;

    int correlated_dimensions = 0;

    RRDR *high_rrdr = NULL;
    RRDR *base_rrdr = NULL;

    // get first the highlight to find the number of points available
    stats->db_queries++;
    usec_t started_usec = now_realtime_usec();
    ONEWAYALLOC *owa = onewayalloc_create(0);
    high_rrdr = rrd2rrdr(owa, st, points,
                         after, before, group,
                         group_time, options, NULL, context_param_list, timeout);
    if(!high_rrdr) {
        info("Metric correlations: rrd2rrdr() failed for the highlighted window on chart '%s'.", st->name);
        goto cleanup;
    }
    stats->db_points     += high_rrdr->internal.db_points_read;
    stats->result_points += high_rrdr->internal.result_points_generated;
    if(!high_rrdr->d) {
        info("Metric correlations: rrd2rrdr() did not return any dimensions on chart '%s'.", st->name);
        goto cleanup;
    }
    if(high_rrdr->result_options & RRDR_RESULT_OPTION_CANCEL) {
        info("Metric correlations: rrd2rrdr() on highlighted window timed out '%s'.", st->name);
        goto cleanup;
    }
    int high_points = rrdr_rows(high_rrdr);

    usec_t now_usec = now_realtime_usec();
    if(now_usec - started_usec > timeout * USEC_PER_MS)
        goto cleanup;

    // get the baseline, requesting the same number of points as the highlight
    stats->db_queries++;
    base_rrdr = rrd2rrdr(owa, st,high_points << shifts,
                    baseline_after, baseline_before, group,
                    group_time, options, NULL, context_param_list,
                    (int)(timeout - ((now_usec - started_usec) / USEC_PER_MS)));
    if(!base_rrdr) {
        info("Metric correlations: rrd2rrdr() failed for the baseline window on chart '%s'.", st->name);
        goto cleanup;
    }
    stats->db_points     += base_rrdr->internal.db_points_read;
    stats->result_points += base_rrdr->internal.result_points_generated;
    if(!base_rrdr->d) {
        info("Metric correlations: rrd2rrdr() did not return any dimensions on chart '%s'.", st->name);
        goto cleanup;
    }
    if (base_rrdr->d != high_rrdr->d) {
        info("Cannot generate metric correlations for chart '%s' when the baseline and the highlight have different number of dimensions.", st->name);
        goto cleanup;
    }
    if(base_rrdr->result_options & RRDR_RESULT_OPTION_CANCEL) {
        info("Metric correlations: rrd2rrdr() on baseline window timed out '%s'.", st->name);
        goto cleanup;
    }
    int base_points = rrdr_rows(base_rrdr);

    now_usec = now_realtime_usec();
    if(now_usec - started_usec > timeout * USEC_PER_MS)
        goto cleanup;

    // we need at least 2 points to do the job
    if(base_points < 2 || high_points < 2)
        goto cleanup;

    // for each dimension
    RRDDIM *d;
    int i;
    for(i = 0, d = base_rrdr->st->dimensions ; d && i < base_rrdr->d; i++, d = d->next) {

        // skip the not evaluated ones
        if(unlikely(base_rrdr->od[i] & RRDR_DIMENSION_HIDDEN) || (high_rrdr->od[i] & RRDR_DIMENSION_HIDDEN))
            continue;

        correlated_dimensions++;

        // skip the dimensions that are just zero for both the baseline and the highlight
        if(unlikely(!(base_rrdr->od[i] & RRDR_DIMENSION_NONZERO) && !(high_rrdr->od[i] & RRDR_DIMENSION_NONZERO)))
            continue;

        // copy the baseline points of the dimension to a contiguous array
        // there is no need to check for empty values, since empty are already zero
        calculated_number baseline[base_points];
        for(int c = 0; c < base_points; c++)
            baseline[c] = base_rrdr->v[ c * base_rrdr->d + i ];

        // copy the highlight points of the dimension to a contiguous array
        // there is no need to check for empty values, since empty values are already zero
        // https://github.com/netdata/netdata/blob/6e3144683a73a2024d51425b20ecfd569034c858/web/api/queries/average/average.c#L41-L43
        calculated_number highlight[high_points];
        for(int c = 0; c < high_points; c++)
            highlight[c] = high_rrdr->v[ c * high_rrdr->d + i ];

        stats->binary_searches += 2 * (base_points - 1) + 2 * (high_points - 1);

        double prob = kstwo(baseline, base_points, highlight, high_points, shifts);
        if(!isnan(prob) && !isinf(prob)) {

            // these conditions should never happen, but still let's check
            if(unlikely(prob < 0.0)) {
                error("Metric correlations: kstwo() returned a negative number: %f", prob);
                prob = -prob;
            }
            if(unlikely(prob > 1.0)) {
                error("Metric correlations: kstwo() returned a number above 1.0: %f", prob);
                prob = 1.0;
            }

            // to spread the results evenly, 0.0 needs to be the less correlated and 1.0 the most correlated
            // so we flip the result of kstwo()
            register_result(results, base_rrdr->st, d, 1.0 - prob);
        }
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
                                             long long after, long long before,
                                             RRDR_OPTIONS options, RRDR_GROUPING group, int timeout, MC_STATS *stats) {
    options |= RRDR_OPTION_MATCH_IDS;
    long group_time = 0;

    int correlated_dimensions = 0;
    int ret, value_is_null;
    usec_t started_usec = now_realtime_usec();

    RRDDIM *d;
    for(d = st->dimensions; d ; d = d->next) {
        usec_t now_usec = now_realtime_usec();
        if(now_usec - started_usec > timeout * USEC_PER_MS)
            return correlated_dimensions;

        // we count how many metrics we evaluated
        correlated_dimensions++;

        // there is no point to pass a timeout to these queries
        // since the query engine checks for a timeout between
        // dimensions, and we query a single dimension at a time.

        stats->db_queries++;
        calculated_number highlight_average = NAN;
        value_is_null = 1;
        ret = rrdset2value_api_v1(st, NULL, &highlight_average, d->id, 1,
                                  after, before,
                                  group, group_time, options,
                                  NULL, NULL,
                                  &stats->db_points, &stats->result_points,
                                  &value_is_null, 0);

        if(ret != HTTP_RESP_OK || value_is_null || !calculated_number_isnumber(highlight_average)) {
            // error("Metric correlations: cannot query highlight duration of dimension '%s' of chart '%s', %d %s %s %s", st->name, d->name, ret, (ret != HTTP_RESP_OK)?"response failed":"", (value_is_null)?"value is null":"", (!calculated_number_isnumber(highlight_average))?"result is NAN":"");
            // this means no data for the highlighted duration - so skip it
            continue;
        }

        stats->db_queries++;
        calculated_number baseline_average = NAN;
        value_is_null = 1;
        ret = rrdset2value_api_v1(st, NULL, &baseline_average, d->id, 1,
                                  baseline_after, baseline_before,
                                  group, group_time, options,
                                  NULL, NULL,
                                  &stats->db_points, &stats->result_points,
                                  &value_is_null, 0);

        if(ret != HTTP_RESP_OK || value_is_null || !calculated_number_isnumber(baseline_average)) {
            // error("Metric correlations: cannot query baseline duration of dimension '%s' of chart '%s', %d %s %s %s", st->name, d->name, ret, (ret != HTTP_RESP_OK)?"response failed":"", (value_is_null)?"value is null":"", (!calculated_number_isnumber(baseline_average))?"result is NAN":"");
            // continue;
            // this means no data for the baseline window, but we have data for the highlighted one - assume zero
            baseline_average = 0.0;
        }

        calculated_number pcent = NAN;
        if(isgreater(baseline_average, 0.0) || isless(baseline_average, 0.0))
            pcent = (highlight_average - baseline_average) / baseline_average;

        else if(isgreater(highlight_average, 0.0) || isless(highlight_average, 0.0))
            pcent = highlight_average;

        if(!isnan(pcent))
            register_result(results, st, d, pcent);
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

// ----------------------------------------------------------------------------
// spread the results evenly according to their value

static size_t spread_results_evenly(DICTIONARY *results) {
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
        t->value = calculated_number_fabs(t->value);
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

int metric_correlations(RRDHOST *host, BUFFER *wb, METRIC_CORRELATIONS_METHOD method, RRDR_GROUPING group,
                        long long baseline_after, long long baseline_before,
                        long long after, long long before,
                        long long points, RRDR_OPTIONS options, int timeout) {

    // method = METRIC_CORRELATIONS_VOLUME;
    // options |= RRDR_OPTION_ANOMALY_BIT;

    MC_STATS stats = {};

    if (enable_metric_correlations == CONFIG_BOOLEAN_NO) {
        buffer_strcat(wb, "{\"error\": \"Metric correlations functionality is not enabled.\" }");
        return HTTP_RESP_FORBIDDEN;
    }

    // if the user didn't give a timeout
    // assume 60 seconds
    if(!timeout)
        timeout = 60 * MSEC_PER_SEC;

    // if the timeout is less than 1 second
    // make it at least 1 second
    if(timeout < (long)(1 * MSEC_PER_SEC))
        timeout = 1 * MSEC_PER_SEC;

    usec_t timeout_usec = timeout * USEC_PER_MS;
    usec_t started_usec = now_realtime_usec();

    if(!points) points = 500;

    rrdr_relative_window_to_absolute(&after, &before, default_rrd_update_every, points);

    if(baseline_before <= API_RELATIVE_TIME_MAX)
        baseline_before += after;

    rrdr_relative_window_to_absolute(&baseline_after, &baseline_before, default_rrd_update_every, points * 4);

    if (before <= after || baseline_before <= baseline_after) {
        buffer_strcat(wb, "{\"error\": \"Invalid baseline or highlight ranges.\" }");
        return HTTP_RESP_BAD_REQUEST;
    }

    DICTIONARY *results = register_result_init();
    DICTIONARY *charts = dictionary_create(DICTIONARY_FLAG_SINGLE_THREADED|DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE);;

    char *error = NULL;
    int resp = HTTP_RESP_OK;

    // baseline should be a power of two multiple of highlight
    uint32_t shifts = 0;
    {
        long long base_delta = baseline_before - baseline_after;
        long long high_delta = before - after;
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
        while(shifts && (points << shifts) > MAX_POINTS)
            shifts--;

        // if the baseline size still does not comply to MAX_POINTS
        // lower the resolution of the highlight and the baseline
        while((points << shifts) > MAX_POINTS)
            points = points >> 1;

        if(points < 100) {
            // error = "cannot comply to at least 100 points";
            resp = HTTP_RESP_BAD_REQUEST;
            goto cleanup;
        }

        // adjust the baseline to be multiplier times bigger than the highlight
        baseline_after = baseline_before - (high_delta << shifts);
    }

    // dont lock here and wait for results
    // get the charts and run mc after
    RRDSET *st;
    rrdhost_rdlock(host);
    rrdset_foreach_read(st, host) {
        if (rrdset_is_available_for_viewers(st))
            dictionary_set(charts, st->name, "", 1);
    }
    rrdhost_unlock(host);

    size_t correlated_dimensions = 0;
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
                correlated_dimensions += rrdset_metric_correlations_volume(st, results,
                                                                baseline_after, baseline_before,
                                                                after, before,
                                                                options, group,
                                                                (int)(timeout - ((now_usec - started_usec) / USEC_PER_MS)),
                                                                &stats);
                break;

            default:
            case METRIC_CORRELATIONS_KS2:
                correlated_dimensions += rrdset_metric_correlations_ks2(st, results,
                                                             baseline_after, baseline_before,
                                                             after, before,
                                                             points, options, group, shifts,
                                                             (int)(timeout - ((now_usec - started_usec) / USEC_PER_MS)),
                                                             &stats);
                break;
        }

        rrdset_unlock(st);
    }
    dfe_done(ptr);

    if(!(options & RRDR_OPTION_RETURN_RAW))
        spread_results_evenly(results);

    usec_t ended_usec = now_realtime_usec();

    // generate the json output we need
    buffer_flush(wb);
    size_t added_dimensions = registered_results_to_json(results, wb,
                                                         after, before,
                                                         baseline_after, baseline_before,
                                                         points, method, group, options, shifts, correlated_dimensions,
                                                         ended_usec - started_usec, &stats);

    if(!added_dimensions) {
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

    return resp;
}



// ----------------------------------------------------------------------------
// unittest

/*

Unit tests against the output of this:

https://github.com/scipy/scipy/blob/4cf21e753cf937d1c6c2d2a0e372fbc1dbbeea81/scipy/stats/_stats_py.py#L7275-L7449

import matplotlib.pyplot as plt
import pandas as pd
import numpy as np
import scipy as sp
from scipy import stats

data1 = np.array([ 1111, -2222, 33, 100, 100, 15555, -1, 19999, 888, 755, -1, -730 ])
data2 = np.array([365, -123, 0])
data1 = np.sort(data1)
data2 = np.sort(data2)
n1 = data1.shape[0]
n2 = data2.shape[0]
data_all = np.concatenate([data1, data2])
cdf1 = np.searchsorted(data1, data_all, side='right') / n1
cdf2 = np.searchsorted(data2, data_all, side='right') / n2
print(data_all)
print("\ndata1", data1, cdf1)
print("\ndata2", data2, cdf2)
cddiffs = cdf1 - cdf2
print("\ncddiffs", cddiffs)
minS = np.clip(-np.min(cddiffs), 0, 1)
maxS = np.max(cddiffs)
print("\nmin", minS)
print("max", maxS)
m, n = sorted([float(n1), float(n2)], reverse=True)
en = m * n / (m + n)
d = max(minS, maxS)
prob = stats.distributions.kstwo.sf(d, np.round(en))
print("\nprob", prob)

*/

static int double_expect(double v, const char *str, const char *descr) {
    char buf[100 + 1];
    snprintfz(buf, 100, "%0.6f", v);
    int ret = strcmp(buf, str) ? 1 : 0;

    fprintf(stderr, "%s %s, expected %s, got %s\n", ret?"FAILED":"OK", descr, str, buf);
    return ret;
}

static int mc_unittest1(void) {
    int bs = 3, hs = 3;
    DIFFS_NUMBERS base[3] = { 1, 2, 3 };
    DIFFS_NUMBERS high[3] = { 3, 4, 6 };

    double prob = ks_2samp(base, bs, high, hs, 0);
    return double_expect(prob, "0.222222", "3x3");
}

static int mc_unittest2(void) {
    int bs = 6, hs = 3;
    DIFFS_NUMBERS base[6] = { 1, 2, 3, 10, 10, 15 };
    DIFFS_NUMBERS high[3] = { 3, 4, 6 };

    double prob = ks_2samp(base, bs, high, hs, 1);
    return double_expect(prob, "0.500000", "6x3");
}

static int mc_unittest3(void) {
    int bs = 12, hs = 3;
    DIFFS_NUMBERS base[12] = { 1, 2, 3, 10, 10, 15, 111, 19999, 8, 55, -1, -73 };
    DIFFS_NUMBERS high[3] = { 3, 4, 6 };

    double prob = ks_2samp(base, bs, high, hs, 2);
    return double_expect(prob, "0.347222", "12x3");
}

static int mc_unittest4(void) {
    int bs = 12, hs = 3;
    DIFFS_NUMBERS base[12] = { 1111, -2222, 33, 100, 100, 15555, -1, 19999, 888, 755, -1, -730 };
    DIFFS_NUMBERS high[3] = { 365, -123, 0 };

    double prob = ks_2samp(base, bs, high, hs, 2);
    return double_expect(prob, "0.777778", "12x3");
}

int mc_unittest(void) {
    int errors = 0;

    errors += mc_unittest1();
    errors += mc_unittest2();
    errors += mc_unittest3();
    errors += mc_unittest4();

    return errors;
}


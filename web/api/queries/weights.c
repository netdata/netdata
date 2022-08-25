// SPDX-License-Identifier: GPL-3.0-or-later

#include "daemon/common.h"
#include "database/KolmogorovSmirnovDist.h"

#define MAX_POINTS 10000
int enable_metric_correlations = CONFIG_BOOLEAN_YES;
int metric_correlations_version = 1;
WEIGHTS_METHOD default_metric_correlations_method = WEIGHTS_METHOD_MC_KS2;

typedef struct weights_stats {
    NETDATA_DOUBLE max_base_high_ratio;
    size_t db_points;
    size_t result_points;
    size_t db_queries;
    size_t db_points_per_tier[RRD_STORAGE_TIERS];
    size_t binary_searches;
} WEIGHTS_STATS;

// ----------------------------------------------------------------------------
// parse and render metric correlations methods

static struct {
    const char *name;
    WEIGHTS_METHOD value;
} weights_methods[] = {
      { "ks2"          , WEIGHTS_METHOD_MC_KS2}
    , { "volume"       , WEIGHTS_METHOD_MC_VOLUME}
    , { "anomaly-rate" , WEIGHTS_METHOD_ANOMALY_RATE}
    , { NULL           , 0 }
};

WEIGHTS_METHOD weights_string_to_method(const char *method) {
    for(int i = 0; weights_methods[i].name ;i++)
        if(strcmp(method, weights_methods[i].name) == 0)
            return weights_methods[i].value;

    return default_metric_correlations_method;
}

const char *weights_method_to_string(WEIGHTS_METHOD method) {
    for(int i = 0; weights_methods[i].name ;i++)
        if(weights_methods[i].value == method)
            return weights_methods[i].name;

    return "unknown";
}

// ----------------------------------------------------------------------------
// The results per dimension are aggregated into a dictionary

typedef enum {
    RESULT_IS_BASE_HIGH_RATIO     = (1 << 0),
    RESULT_IS_PERCENTAGE_OF_TIME  = (1 << 1),
} RESULT_FLAGS;

struct register_result {
    RESULT_FLAGS flags;
    RRDSET *st;
    const char *chart_id;
    const char *context;
    const char *dim_name;
    NETDATA_DOUBLE value;

    struct register_result *next; // used to link contexts together
};

static void register_result_insert_callback(const char *name, void *value, void *data) {
    (void)name;
    (void)data;

    struct register_result *t = (struct register_result *)value;

    if(t->chart_id) t->chart_id = strdupz(t->chart_id);
    if(t->context)  t->context  = strdupz(t->context);
    if(t->dim_name) t->dim_name = strdupz(t->dim_name);
}

static void register_result_delete_callback(const char *name, void *value, void *data) {
    (void)name;
    (void)data;
    struct register_result *t = (struct register_result *)value;

    freez((void *)t->chart_id);
    freez((void *)t->context);
    freez((void *)t->dim_name);
}

static DICTIONARY *register_result_init() {
    DICTIONARY *results = dictionary_create(DICTIONARY_FLAG_SINGLE_THREADED);
    dictionary_register_insert_callback(results, register_result_insert_callback, results);
    dictionary_register_delete_callback(results, register_result_delete_callback, results);
    return results;
}

static void register_result_destroy(DICTIONARY *results) {
    dictionary_destroy(results);
}

static void register_result(DICTIONARY *results,
                            RRDSET *st,
                            RRDDIM *d,
                            NETDATA_DOUBLE value,
                            RESULT_FLAGS flags,
                            WEIGHTS_STATS *stats,
                            bool register_zero) {

    if(!netdata_double_isnumber(value)) return;

    // make it positive
    NETDATA_DOUBLE v = fabsndd(value);

    // no need to store zero scored values
    if(unlikely(fpclassify(v) == FP_ZERO && !register_zero))
        return;

    // keep track of the max of the baseline / highlight ratio
    if(flags & RESULT_IS_BASE_HIGH_RATIO && v > stats->max_base_high_ratio)
        stats->max_base_high_ratio = v;

    struct register_result t = {
        .flags = flags,
        .st = st,
        .chart_id = st->id,
        .context = rrdset_context(st),
        .dim_name = rrddim_name(d),
        .value = v
    };

    char buf[5000 + 1];
    snprintfz(buf, 5000, "%s:%s", st->id, rrddim_name(d));
    dictionary_set(results, buf, &t, sizeof(struct register_result));
}

// ----------------------------------------------------------------------------
// Generation of JSON output for the results

static void results_header_to_json(DICTIONARY *results __maybe_unused, BUFFER *wb,
                                   long long after, long long before,
                                   long long baseline_after, long long baseline_before,
                                   long points, WEIGHTS_METHOD method,
                                   RRDR_GROUPING group, RRDR_OPTIONS options, uint32_t shifts,
                                   size_t examined_dimensions __maybe_unused, usec_t duration,
                                   WEIGHTS_STATS *stats) {

    buffer_sprintf(wb, "{\n"
                       "\t\"after\": %lld,\n"
                       "\t\"before\": %lld,\n"
                       "\t\"duration\": %lld,\n"
                       "\t\"points\": %ld,\n",
                       after,
                       before,
                       before - after,
                       points
                       );

    if(method == WEIGHTS_METHOD_MC_KS2 || method == WEIGHTS_METHOD_MC_VOLUME)
        buffer_sprintf(wb, ""
                           "\t\"baseline_after\": %lld,\n"
                           "\t\"baseline_before\": %lld,\n"
                           "\t\"baseline_duration\": %lld,\n"
                           "\t\"baseline_points\": %ld,\n",
                           baseline_after,
                           baseline_before,
                           baseline_before - baseline_after,
                           points << shifts
                       );

    buffer_sprintf(wb, ""
                       "\t\"statistics\": {\n"
                       "\t\t\"query_time_ms\": %f,\n"
                       "\t\t\"db_queries\": %zu,\n"
                       "\t\t\"query_result_points\": %zu,\n"
                       "\t\t\"binary_searches\": %zu,\n"
                       "\t\t\"db_points_read\": %zu,\n"
                       "\t\t\"db_points_per_tier\": [ ",
                       (double)duration / (double)USEC_PER_MS,
                       stats->db_queries,
                       stats->result_points,
                       stats->binary_searches,
                       stats->db_points
                   );

    for(int tier = 0; tier < storage_tiers ;tier++)
        buffer_sprintf(wb, "%s%zu", tier?", ":"", stats->db_points_per_tier[tier]);

    buffer_sprintf(wb, " ]\n"
                       "\t},\n"
                       "\t\"group\": \"%s\",\n"
                       "\t\"method\": \"%s\",\n"
                       "\t\"options\": \"",
                       web_client_api_request_v1_data_group_to_string(group),
                       weights_method_to_string(method)
                   );

    web_client_api_request_v1_data_options_to_string(wb, options);
}

static size_t registered_results_to_json_charts(DICTIONARY *results, BUFFER *wb,
                                                long long after, long long before,
                                                long long baseline_after, long long baseline_before,
                                                long points, WEIGHTS_METHOD method,
                                                RRDR_GROUPING group, RRDR_OPTIONS options, uint32_t shifts,
                                                size_t examined_dimensions, usec_t duration,
                                                WEIGHTS_STATS *stats) {

    results_header_to_json(results, wb, after, before, baseline_after, baseline_before,
                           points, method, group, options, shifts, examined_dimensions, duration, stats);

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
        buffer_sprintf(wb, "\t\t\t\t\"%s\": " NETDATA_DOUBLE_FORMAT, t->dim_name, t->value);
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
                   examined_dimensions
                   );

    return total_dimensions;
}

static size_t registered_results_to_json_contexts(DICTIONARY *results, BUFFER *wb,
                                                  long long after, long long before,
                                                  long long baseline_after, long long baseline_before,
                                                  long points, WEIGHTS_METHOD method,
                                                  RRDR_GROUPING group, RRDR_OPTIONS options, uint32_t shifts,
                                                  size_t examined_dimensions, usec_t duration,
                                                  WEIGHTS_STATS *stats) {

    results_header_to_json(results, wb, after, before, baseline_after, baseline_before,
                           points, method, group, options, shifts, examined_dimensions, duration, stats);

    DICTIONARY *context_results = dictionary_create(
         DICTIONARY_FLAG_SINGLE_THREADED
        |DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE
        |DICTIONARY_FLAG_NAME_LINK_DONT_CLONE
        |DICTIONARY_FLAG_DONT_OVERWRITE_VALUE
        );

    struct register_result *t;
    dfe_start_read(results, t) {
        struct register_result *tc = dictionary_set(context_results, t->context, t, sizeof(*t));
        if(tc == t)
            t->next = NULL;
        else {
            t->next = tc->next;
            tc->next = t;
        }
    }
    dfe_done(t);

    buffer_strcat(wb, "\",\n\t\"contexts\": {\n");

    size_t contexts = 0, total_dimensions = 0, charts = 0, context_dims = 0, chart_dims = 0;
    NETDATA_DOUBLE contexts_total_weight = 0.0, charts_total_weight = 0.0;
    RRDSET *last_st = NULL; // never access this - we use it only for comparison
    dfe_start_read(context_results, t) {

        if(contexts)
            buffer_sprintf(wb, "\n\t\t\t\t\t},\n\t\t\t\t\t\"weight\":" NETDATA_DOUBLE_FORMAT "\n\t\t\t\t}\n\t\t\t},\n\t\t\t\"weight\":" NETDATA_DOUBLE_FORMAT "\n\t\t},\n", charts_total_weight / chart_dims, contexts_total_weight / context_dims);

        contexts++;
        context_dims = 0;
        contexts_total_weight = 0.0;

        buffer_strcat(wb, "\t\t\"");
        buffer_strcat(wb, t->context);
        buffer_strcat(wb, "\": {\n\t\t\t\"charts\":{\n");

        charts = 0;
        chart_dims = 0;
        struct register_result *tt;
        for(tt = t; tt ; tt = tt->next) {
            if(!last_st || tt->st != last_st) {
                last_st = tt->st;

                if(charts)
                    buffer_sprintf(wb, "\n\t\t\t\t\t},\n\t\t\t\t\t\"weight\":" NETDATA_DOUBLE_FORMAT "\n\t\t\t\t},\n", charts_total_weight / chart_dims);

                buffer_strcat(wb, "\t\t\t\t\"");
                buffer_strcat(wb, tt->chart_id);
                buffer_strcat(wb, "\": {\n");
                buffer_strcat(wb, "\t\t\t\t\t\"dimensions\": {\n");
                charts++;
                chart_dims = 0;
                charts_total_weight = 0.0;
            }

            if (chart_dims) buffer_sprintf(wb, ",\n");
            buffer_sprintf(wb, "\t\t\t\t\t\t\"%s\": " NETDATA_DOUBLE_FORMAT, tt->dim_name, tt->value);
            charts_total_weight += tt->value;
            contexts_total_weight += tt->value;
            chart_dims++;
            context_dims++;
            total_dimensions++;
        }
    }
    dfe_done(t);

    dictionary_destroy(context_results);

    // close dimensions and chart
    if (total_dimensions)
        buffer_sprintf(wb, "\n\t\t\t\t\t},\n\t\t\t\t\t\"weight\":" NETDATA_DOUBLE_FORMAT "\n\t\t\t\t}\n\t\t\t},\n\t\t\t\"weight\":" NETDATA_DOUBLE_FORMAT "\n\t\t}\n", charts_total_weight / chart_dims, contexts_total_weight / context_dims);

    // close correlated_charts
    buffer_sprintf(wb, "\t},\n"
                       "\t\"weighted_dimensions\": %zu,\n"
                       "\t\"total_dimensions_count\": %zu\n"
                       "}\n",
                   total_dimensions,
                   examined_dimensions
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

static size_t calculate_pairs_diff(DIFFS_NUMBERS *diffs, NETDATA_DOUBLE *arr, size_t size) {
    NETDATA_DOUBLE *last = &arr[size - 1];
    size_t added = 0;

    while(last > arr) {
        NETDATA_DOUBLE second = *last--;
        NETDATA_DOUBLE first  = *last;
        *diffs++ = (DIFFS_NUMBERS)((first - second) * (NETDATA_DOUBLE)DOUBLE_TO_INT_MULTIPLIER);
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

static double kstwo(
    NETDATA_DOUBLE baseline[], int baseline_points,
    NETDATA_DOUBLE highlight[], int highlight_points, uint32_t base_shifts) {
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
                                          long long points, RRDR_OPTIONS options,
                                          RRDR_GROUPING group, const char *group_options, int tier,
                                          uint32_t shifts, int timeout,
                                          WEIGHTS_STATS *stats, bool register_zero) {
    options |= RRDR_OPTION_NATURAL_POINTS;

    long group_time = 0;
    struct context_param  *context_param_list = NULL;

    int examined_dimensions = 0;

    RRDR *high_rrdr = NULL;
    RRDR *base_rrdr = NULL;

    // get first the highlight to find the number of points available
    stats->db_queries++;
    usec_t started_usec = now_realtime_usec();
    ONEWAYALLOC *owa = onewayalloc_create(0);
    high_rrdr = rrd2rrdr(owa, st, points,
                         after, before, group,
                         group_time, options, NULL, context_param_list, group_options,
                         timeout, tier);
    if(!high_rrdr) {
        info("Metric correlations: rrd2rrdr() failed for the highlighted window on chart '%s'.", st->name);
        goto cleanup;
    }

    for(int i = 0; i < storage_tiers ;i++)
        stats->db_points_per_tier[i] += high_rrdr->internal.tier_points_read[i];

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
                    group_time, options, NULL, context_param_list, group_options,
                    (int)(timeout - ((now_usec - started_usec) / USEC_PER_MS)), tier);
    if(!base_rrdr) {
        info("Metric correlations: rrd2rrdr() failed for the baseline window on chart '%s'.", st->name);
        goto cleanup;
    }

    for(int i = 0; i < storage_tiers ;i++)
        stats->db_points_per_tier[i] += base_rrdr->internal.tier_points_read[i];

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

        examined_dimensions++;

        // skip the dimensions that are just zero for both the baseline and the highlight
        if(unlikely(!(base_rrdr->od[i] & RRDR_DIMENSION_NONZERO) && !(high_rrdr->od[i] & RRDR_DIMENSION_NONZERO)))
            continue;

        // copy the baseline points of the dimension to a contiguous array
        // there is no need to check for empty values, since empty are already zero
        NETDATA_DOUBLE baseline[base_points];
        for(int c = 0; c < base_points; c++)
            baseline[c] = base_rrdr->v[ c * base_rrdr->d + i ];

        // copy the highlight points of the dimension to a contiguous array
        // there is no need to check for empty values, since empty values are already zero
        // https://github.com/netdata/netdata/blob/6e3144683a73a2024d51425b20ecfd569034c858/web/api/queries/average/average.c#L41-L43
        NETDATA_DOUBLE highlight[high_points];
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
            register_result(results, base_rrdr->st, d, 1.0 - prob, RESULT_IS_BASE_HIGH_RATIO, stats, register_zero);
        }
    }

cleanup:
    rrdr_free(owa, high_rrdr);
    rrdr_free(owa, base_rrdr);
    onewayalloc_destroy(owa);
    return examined_dimensions;
}

// ----------------------------------------------------------------------------
// VOLUME algorithm functions

static int rrdset_metric_correlations_volume(RRDSET *st, DICTIONARY *results,
                                             long long baseline_after, long long baseline_before,
                                             long long after, long long before,
                                             RRDR_OPTIONS options, RRDR_GROUPING group, const char *group_options,
                                             int tier, int timeout,
                                             WEIGHTS_STATS *stats, bool register_zero) {

    options |= RRDR_OPTION_MATCH_IDS | RRDR_OPTION_ABSOLUTE | RRDR_OPTION_NATURAL_POINTS;
    long group_time = 0;

    int examined_dimensions = 0;
    int ret, value_is_null;
    usec_t started_usec = now_realtime_usec();

    RRDDIM *d;
    for(d = st->dimensions; d ; d = d->next) {
        usec_t now_usec = now_realtime_usec();
        if(now_usec - started_usec > timeout * USEC_PER_MS)
            return examined_dimensions;

        // we count how many metrics we evaluated
        examined_dimensions++;

        // there is no point to pass a timeout to these queries
        // since the query engine checks for a timeout between
        // dimensions, and we query a single dimension at a time.

        stats->db_queries++;
        NETDATA_DOUBLE baseline_average = NAN;
        NETDATA_DOUBLE base_anomaly_rate = 0;
        value_is_null = 1;
        ret = rrdset2value_api_v1(st, NULL, &baseline_average, rrddim_id(d), 1,
                                  baseline_after, baseline_before,
                                  group, group_options, group_time, options,
                                  NULL, NULL,
                                  &stats->db_points, stats->db_points_per_tier,
                                  &stats->result_points,
                                  &value_is_null, &base_anomaly_rate, 0, tier);

        if(ret != HTTP_RESP_OK || value_is_null || !netdata_double_isnumber(baseline_average)) {
            // this means no data for the baseline window, but we may have data for the highlighted one - assume zero
            baseline_average = 0.0;
        }

        stats->db_queries++;
        NETDATA_DOUBLE highlight_average = NAN;
        NETDATA_DOUBLE high_anomaly_rate = 0;
        value_is_null = 1;
        ret = rrdset2value_api_v1(st, NULL, &highlight_average, rrddim_id(d), 1,
                                  after, before,
                                  group, group_options, group_time, options,
                                  NULL, NULL,
                                  &stats->db_points, stats->db_points_per_tier,
                                  &stats->result_points,
                                  &value_is_null, &high_anomaly_rate, 0, tier);

        if(ret != HTTP_RESP_OK || value_is_null || !netdata_double_isnumber(highlight_average)) {
            // this means no data for the highlighted duration - so skip it
            continue;
        }

        if(baseline_average == highlight_average) {
            // they are the same - let's move on
            continue;
        }

        stats->db_queries++;
        NETDATA_DOUBLE highlight_countif = NAN;
        value_is_null = 1;

        char highlighted_countif_options[50 + 1];
        snprintfz(highlighted_countif_options, 50, "%s" NETDATA_DOUBLE_FORMAT, highlight_average < baseline_average ? "<":">", baseline_average);

        ret = rrdset2value_api_v1(st, NULL, &highlight_countif, rrddim_id(d), 1,
                                  after, before,
                                  RRDR_GROUPING_COUNTIF,highlighted_countif_options,
                                  group_time, options,
                                  NULL, NULL,
                                  &stats->db_points, stats->db_points_per_tier,
                                  &stats->result_points,
                                  &value_is_null, NULL, 0, tier);

        if(ret != HTTP_RESP_OK || value_is_null || !netdata_double_isnumber(highlight_countif)) {
            info("MC: highlighted countif query failed, but highlighted average worked - strange...");
            continue;
        }

        // this represents the percentage of time
        // the highlighted window was above/below the baseline window
        // (above or below depending on their averages)
        highlight_countif = highlight_countif / 100.0; // countif returns 0 - 100.0

        RESULT_FLAGS flags;
        NETDATA_DOUBLE pcent = NAN;
        if(isgreater(baseline_average, 0.0) || isless(baseline_average, 0.0)) {
            flags = RESULT_IS_BASE_HIGH_RATIO;
            pcent = (highlight_average - baseline_average) / baseline_average * highlight_countif;
        }
        else {
            flags = RESULT_IS_PERCENTAGE_OF_TIME;
            pcent = highlight_countif;
        }

        register_result(results, st, d, pcent, flags, stats, register_zero);
    }

    return examined_dimensions;
}

// ----------------------------------------------------------------------------
// ANOMALY RATE algorithm functions

static int rrdset_weights_anomaly_rate(RRDSET *st, DICTIONARY *results,
                                       long long after, long long before,
                                       RRDR_OPTIONS options, RRDR_GROUPING group, const char *group_options,
                                       int tier, int timeout,
                                       WEIGHTS_STATS *stats, bool register_zero) {

    options |= RRDR_OPTION_MATCH_IDS | RRDR_OPTION_ANOMALY_BIT | RRDR_OPTION_NATURAL_POINTS;
    long group_time = 0;

    int examined_dimensions = 0;
    int ret, value_is_null;
    usec_t started_usec = now_realtime_usec();

    RRDDIM *d;
    for(d = st->dimensions; d ; d = d->next) {
        usec_t now_usec = now_realtime_usec();
        if(now_usec - started_usec > timeout * USEC_PER_MS)
            return examined_dimensions;

        // we count how many metrics we evaluated
        examined_dimensions++;

        // there is no point to pass a timeout to these queries
        // since the query engine checks for a timeout between
        // dimensions, and we query a single dimension at a time.

        stats->db_queries++;
        NETDATA_DOUBLE average = NAN;
        NETDATA_DOUBLE anomaly_rate = 0;
        value_is_null = 1;
        ret = rrdset2value_api_v1(st, NULL, &average, rrddim_id(d), 1,
                                  after, before,
                                  group, group_options, group_time, options,
                                  NULL, NULL,
                                  &stats->db_points, stats->db_points_per_tier,
                                  &stats->result_points,
                                  &value_is_null, &anomaly_rate, 0, tier);

        if(ret == HTTP_RESP_OK || !value_is_null || netdata_double_isnumber(average))
            register_result(results, st, d, average, 0, stats, register_zero);
    }

    return examined_dimensions;
}

// ----------------------------------------------------------------------------

int compare_netdata_doubles(const void *left, const void *right) {
    NETDATA_DOUBLE lt = *(NETDATA_DOUBLE *)left;
    NETDATA_DOUBLE rt = *(NETDATA_DOUBLE *)right;

    // https://stackoverflow.com/a/3886497/1114110
    return (lt > rt) - (lt < rt);
}

static inline int binary_search_bigger_than_netdata_double(const NETDATA_DOUBLE arr[], int left, int size, NETDATA_DOUBLE K) {
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

static size_t spread_results_evenly(DICTIONARY *results, WEIGHTS_STATS *stats) {
    struct register_result *t;

    // count the dimensions
    size_t dimensions = dictionary_stats_entries(results);
    if(!dimensions) return 0;

    if(stats->max_base_high_ratio == 0.0)
        stats->max_base_high_ratio = 1.0;

    // create an array of the right size and copy all the values in it
    NETDATA_DOUBLE slots[dimensions];
    dimensions = 0;
    dfe_start_read(results, t) {
        if(t->flags & (RESULT_IS_PERCENTAGE_OF_TIME))
            t->value = t->value * stats->max_base_high_ratio;

        slots[dimensions++] = t->value;
    }
    dfe_done(t);

    // sort the array with the values of all dimensions
    qsort(slots, dimensions, sizeof(NETDATA_DOUBLE), compare_netdata_doubles);

    // skip the duplicates in the sorted array
    NETDATA_DOUBLE last_value = NAN;
    size_t unique_values = 0;
    for(size_t i = 0; i < dimensions ;i++) {
        if(likely(slots[i] != last_value))
            slots[unique_values++] = last_value = slots[i];
    }

    // this cannot happen, but coverity thinks otherwise...
    if(!unique_values)
        unique_values = dimensions;

    // calculate the weight of each slot, using the number of unique values
    NETDATA_DOUBLE slot_weight = 1.0 / (NETDATA_DOUBLE)unique_values;

    dfe_start_read(results, t) {
        int slot = binary_search_bigger_than_netdata_double(slots, 0, (int)unique_values, t->value);
        NETDATA_DOUBLE v = slot * slot_weight;
        if(unlikely(v > 1.0)) v = 1.0;
        v = 1.0 - v;
        t->value = v;
    }
    dfe_done(t);

    return dimensions;
}

// ----------------------------------------------------------------------------
// The main function

int web_api_v1_weights(RRDHOST *host, BUFFER *wb, WEIGHTS_METHOD method, WEIGHTS_FORMAT format,
                        RRDR_GROUPING group, const char *group_options,
                        long long baseline_after, long long baseline_before,
                        long long after, long long before,
                        long long points, RRDR_OPTIONS options, SIMPLE_PATTERN *contexts, int tier, int timeout) {
    WEIGHTS_STATS stats = {};

    DICTIONARY *results = register_result_init();
    DICTIONARY *charts = dictionary_create(DICTIONARY_FLAG_SINGLE_THREADED|DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE);;
    char *error = NULL;
    int resp = HTTP_RESP_OK;

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

    if(!rrdr_relative_window_to_absolute(&after, &before))
        buffer_no_cacheable(wb);

    if (before <= after) {
        resp = HTTP_RESP_BAD_REQUEST;
        error = "Invalid selected time-range.";
        goto cleanup;
    }

    uint32_t shifts = 0;
    if(method == WEIGHTS_METHOD_MC_KS2 || method == WEIGHTS_METHOD_MC_VOLUME) {
        if(!points) points = 500;

        if(baseline_before <= API_RELATIVE_TIME_MAX)
            baseline_before += after;

        rrdr_relative_window_to_absolute(&baseline_after, &baseline_before);

        if (baseline_before <= baseline_after) {
            resp = HTTP_RESP_BAD_REQUEST;
            error = "Invalid baseline time-range.";
            goto cleanup;
        }

        // baseline should be a power of two multiple of highlight
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

        if(points < 15) {
            resp = HTTP_RESP_BAD_REQUEST;
            error = "Too few points available, at least 15 are needed.";
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
        if (rrdset_is_available_for_viewers(st)) {
            if(!contexts || simple_pattern_matches(contexts, rrdset_context(st)))
                dictionary_set(charts, st->name, NULL, 0);
        }
    }
    rrdhost_unlock(host);

    size_t examined_dimensions = 0;
    void *ptr;

    bool register_zero = true;
    if(options & RRDR_OPTION_NONZERO) {
        register_zero = false;
        options &= ~RRDR_OPTION_NONZERO;
    }

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
            case WEIGHTS_METHOD_ANOMALY_RATE:
                options |= RRDR_OPTION_ANOMALY_BIT;
                points = 1;
                examined_dimensions += rrdset_weights_anomaly_rate(st, results,
                                                                   after, before,
                                                                   options, group, group_options, tier,
                                                                   (int)(timeout - ((now_usec - started_usec) / USEC_PER_MS)),
                                                                   &stats, register_zero);
                break;

            case WEIGHTS_METHOD_MC_VOLUME:
                points = 1;
                examined_dimensions += rrdset_metric_correlations_volume(st, results,
                                                                baseline_after, baseline_before,
                                                                after, before,
                                                                options, group, group_options, tier,
                                                                (int)(timeout - ((now_usec - started_usec) / USEC_PER_MS)),
                                                                &stats, register_zero);
                break;

            default:
            case WEIGHTS_METHOD_MC_KS2:
                examined_dimensions += rrdset_metric_correlations_ks2(st, results,
                                                             baseline_after, baseline_before,
                                                             after, before,
                                                             points, options, group, group_options, tier, shifts,
                                                             (int)(timeout - ((now_usec - started_usec) / USEC_PER_MS)),
                                                             &stats, register_zero);
                break;
        }

        rrdset_unlock(st);
    }
    dfe_done(ptr);

    if(!register_zero)
        options |= RRDR_OPTION_NONZERO;

    if(!(options & RRDR_OPTION_RETURN_RAW))
        spread_results_evenly(results, &stats);

    usec_t ended_usec = now_realtime_usec();

    // generate the json output we need
    buffer_flush(wb);

    size_t added_dimensions = 0;
    switch(format) {
        case WEIGHTS_FORMAT_CHARTS:
            added_dimensions = registered_results_to_json_charts(results, wb,
                                                                 after, before,
                                                                 baseline_after, baseline_before,
                                                                 points, method, group, options, shifts,
                                                                 examined_dimensions,
                                                                 ended_usec - started_usec, &stats);
            break;

        default:
        case WEIGHTS_FORMAT_CONTEXTS:
            added_dimensions = registered_results_to_json_contexts(results, wb,
                                                                   after, before,
                                                                   baseline_after, baseline_before,
                                                                   points, method, group, options, shifts,
                                                                   examined_dimensions,
                                                                   ended_usec - started_usec, &stats);
            break;
    }

    if(!added_dimensions) {
        error = "no results produced.";
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


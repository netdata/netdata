// SPDX-License-Identifier: GPL-3.0-or-later

#include "database/rrd.h"
#include "KolmogorovSmirnovDist.h"

#define MAX_POINTS 10000
int metric_correlations_version = 1;

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
        , { "value"        , WEIGHTS_METHOD_VALUE}
        , { NULL           , 0 }
};

WEIGHTS_METHOD weights_string_to_method(const char *method) {
    for(int i = 0; weights_methods[i].name ;i++)
        if(strcmp(method, weights_methods[i].name) == 0)
            return weights_methods[i].value;

    return WEIGHTS_METHOD_MC_KS2;
}

const char *weights_method_to_string(WEIGHTS_METHOD method) {
    for(int i = 0; weights_methods[i].name ;i++)
        if(weights_methods[i].value == method)
            return weights_methods[i].name;

    return "ks2";
}

// ----------------------------------------------------------------------------
// The results per dimension are aggregated into a dictionary

typedef enum {
    RESULT_IS_BASE_HIGH_RATIO     = (1 << 0),
    RESULT_IS_PERCENTAGE_OF_TIME  = (1 << 1),
} RESULT_FLAGS;

struct register_result {
    RESULT_FLAGS flags;
    RRDHOST *host;
    RRDCONTEXT_ACQUIRED *rca;
    RRDINSTANCE_ACQUIRED *ria;
    RRDMETRIC_ACQUIRED *rma;
    NETDATA_DOUBLE value;
    STORAGE_POINT highlighted;
    STORAGE_POINT baseline;
    usec_t duration_ut;
};

static DICTIONARY *register_result_init() {
    DICTIONARY *results = dictionary_create_advanced(DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct register_result));
    return results;
}

static DICTIONARY *register_result_init_single_threaded() {
    DICTIONARY *results = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct register_result));
    return results;
}

static void register_result_destroy(DICTIONARY *results) {
    dictionary_destroy(results);
}

// Merge results from local dictionary into main dictionary
static void merge_results_dictionaries(DICTIONARY *main_results, DICTIONARY *local_results) {
    if (!local_results || !main_results)
        return;

    struct register_result *local_result;
    dfe_start_read(local_results, local_result) {
        // Try to get existing result in main dictionary
        struct register_result *main_result = dictionary_get(main_results, local_result_dfe.name);
        if (main_result) {
            // Merge the results - keep the higher weight
            if (local_result->value > main_result->value) {
                // Create a copy with the new values and replace the entire entry
                struct register_result merged_result = *local_result;
                dictionary_set(main_results, local_result_dfe.name, &merged_result, sizeof(struct register_result));
            }
            // If local value is not higher, keep the existing main result (do nothing)
        } else {
            // Insert new result - copy the entire structure
            dictionary_set(main_results, local_result_dfe.name, local_result, sizeof(struct register_result));
        }
    }
    dfe_done(local_result);
}

// Forward declarations
static ssize_t weights_do_node_callback(void *data, RRDHOST *host, bool queryable);
static ssize_t weights_do_context_callback(void *data, RRDCONTEXT_ACQUIRED *rca, bool queryable_context);

static void register_result(DICTIONARY *results, RRDHOST *host, RRDCONTEXT_ACQUIRED *rca, RRDINSTANCE_ACQUIRED *ria,
                            RRDMETRIC_ACQUIRED *rma, NETDATA_DOUBLE value, RESULT_FLAGS flags,
                            STORAGE_POINT *highlighted, STORAGE_POINT *baseline, WEIGHTS_STATS *stats,
                            bool register_zero, usec_t duration_ut) {

    if(!netdata_double_isnumber(value)) return;

    // make it positive
    NETDATA_DOUBLE v = fabsndd(value);

    // no need to store zero scored values
    if(unlikely(fpclassify(v) == FP_ZERO && !register_zero))
        return;

    // keep track of the max of the baseline / highlight ratio
    if((flags & RESULT_IS_BASE_HIGH_RATIO) && v > stats->max_base_high_ratio)
        stats->max_base_high_ratio = v;

    struct register_result t = {
        .flags = flags,
        .host = host,
        .rca = rca,
        .ria = ria,
        .rma = rma,
        .value = v,
        .duration_ut = duration_ut,
    };

    if(highlighted)
        t.highlighted = *highlighted;

    if(baseline)
        t.baseline = *baseline;

    // Use the original pointer address approach - revert the stable key change
    char buf[20 + 1];
    ssize_t len = snprintfz(buf, sizeof(buf) - 1, "%p", rma);
    dictionary_set_advanced(results, buf, len, &t, sizeof(struct register_result), NULL);
}

// ----------------------------------------------------------------------------
// Generation of JSON output for the results

static void results_header_to_json(DICTIONARY *results __maybe_unused, BUFFER *wb,
                                   time_t after, time_t before,
                                   time_t baseline_after, time_t baseline_before,
                                   size_t points, WEIGHTS_METHOD method,
                                   RRDR_TIME_GROUPING group, RRDR_OPTIONS options, uint32_t shifts,
                                   size_t examined_dimensions __maybe_unused, usec_t duration,
                                   WEIGHTS_STATS *stats) {

    buffer_json_member_add_time_t_formatted(wb, "after", after, options & RRDR_OPTION_RFC3339);
    buffer_json_member_add_time_t_formatted(wb, "before", before, options & RRDR_OPTION_RFC3339);
    buffer_json_member_add_time_t(wb, "duration", before - after);
    buffer_json_member_add_uint64(wb, "points", points);

    if(method == WEIGHTS_METHOD_MC_KS2 || method == WEIGHTS_METHOD_MC_VOLUME) {
        buffer_json_member_add_time_t_formatted(wb, "baseline_after", baseline_after, options & RRDR_OPTION_RFC3339);
        buffer_json_member_add_time_t_formatted(wb, "baseline_before", baseline_before, options & RRDR_OPTION_RFC3339);
        buffer_json_member_add_time_t(wb, "baseline_duration", baseline_before - baseline_after);
        buffer_json_member_add_uint64(wb, "baseline_points", points << shifts);
    }

    buffer_json_member_add_object(wb, "statistics");
    {
        buffer_json_member_add_double(wb, "query_time_ms", (double) duration / (double) USEC_PER_MS);
        buffer_json_member_add_uint64(wb, "db_queries", stats->db_queries);
        buffer_json_member_add_uint64(wb, "query_result_points", stats->result_points);
        buffer_json_member_add_uint64(wb, "binary_searches", stats->binary_searches);
        buffer_json_member_add_uint64(wb, "db_points_read", stats->db_points);

        buffer_json_member_add_array(wb, "db_points_per_tier");
        {
            for (size_t tier = 0; tier < nd_profile.storage_tiers; tier++)
                buffer_json_add_array_item_uint64(wb, stats->db_points_per_tier[tier]);
        }
        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);

    buffer_json_member_add_string(wb, "group", time_grouping_tostring(group));
    buffer_json_member_add_string(wb, "method", weights_method_to_string(method));
    rrdr_options_to_buffer_json_array(wb, "options", options);
}

static size_t registered_results_to_json_charts(DICTIONARY *results, BUFFER *wb,
                                                time_t after, time_t before,
                                                time_t baseline_after, time_t baseline_before,
                                                size_t points, WEIGHTS_METHOD method,
                                                RRDR_TIME_GROUPING group, RRDR_OPTIONS options, uint32_t shifts,
                                                size_t examined_dimensions, usec_t duration,
                                                WEIGHTS_STATS *stats) {

    buffer_json_initialize(wb, "\"", "\"", 0, true, (options & RRDR_OPTION_MINIFY) ? BUFFER_JSON_OPTIONS_MINIFY : BUFFER_JSON_OPTIONS_DEFAULT);

    results_header_to_json(results, wb, after, before, baseline_after, baseline_before,
                           points, method, group, options, shifts, examined_dimensions, duration, stats);

    buffer_json_member_add_object(wb, "correlated_charts");

    size_t charts = 0, total_dimensions = 0;
    struct register_result *t;
    RRDINSTANCE_ACQUIRED *last_ria = NULL; // never access this - we use it only for comparison
    dfe_start_read(results, t) {
        if(t->ria != last_ria) {
            last_ria = t->ria;

            if(charts) {
                buffer_json_object_close(wb); // dimensions
                buffer_json_object_close(wb); // chart:id
            }

            buffer_json_member_add_object(wb, rrdinstance_acquired_id(t->ria));
            buffer_json_member_add_string(wb, "context", rrdcontext_acquired_id(t->rca));
            buffer_json_member_add_object(wb, "dimensions");
            charts++;
        }
        buffer_json_member_add_double(wb, rrdmetric_acquired_name(t->rma), t->value);
        total_dimensions++;
    }
    dfe_done(t);

    // close dimensions and chart
    if (total_dimensions) {
        buffer_json_object_close(wb); // dimensions
        buffer_json_object_close(wb); // chart:id
    }

    buffer_json_object_close(wb);

    buffer_json_member_add_uint64(wb, "correlated_dimensions", total_dimensions);
    buffer_json_member_add_uint64(wb, "total_dimensions_count", examined_dimensions);
    buffer_json_finalize(wb);

    return total_dimensions;
}

static size_t registered_results_to_json_contexts(DICTIONARY *results, BUFFER *wb,
                                                  time_t after, time_t before,
                                                  time_t baseline_after, time_t baseline_before,
                                                  size_t points, WEIGHTS_METHOD method,
                                                  RRDR_TIME_GROUPING group, RRDR_OPTIONS options, uint32_t shifts,
                                                  size_t examined_dimensions, usec_t duration,
                                                  WEIGHTS_STATS *stats) {

    buffer_json_initialize(wb, "\"", "\"", 0, true, (options & RRDR_OPTION_MINIFY) ? BUFFER_JSON_OPTIONS_MINIFY : BUFFER_JSON_OPTIONS_DEFAULT);

    results_header_to_json(results, wb, after, before, baseline_after, baseline_before,
                           points, method, group, options, shifts, examined_dimensions, duration, stats);

    buffer_json_member_add_object(wb, "contexts");

    size_t contexts = 0, charts = 0, total_dimensions = 0, context_dims = 0, chart_dims = 0;
    NETDATA_DOUBLE contexts_total_weight = 0.0, charts_total_weight = 0.0;
    struct register_result *t;
    RRDCONTEXT_ACQUIRED *last_rca = NULL;
    RRDINSTANCE_ACQUIRED *last_ria = NULL;
    dfe_start_read(results, t) {

        if(t->rca != last_rca) {
            last_rca = t->rca;

            if(contexts) {
                buffer_json_object_close(wb); // dimensions
                buffer_json_member_add_double(wb, "weight", charts_total_weight / (double) chart_dims);
                buffer_json_object_close(wb); // chart:id
                buffer_json_object_close(wb); // charts
                buffer_json_member_add_double(wb, "weight", contexts_total_weight / (double) context_dims);
                buffer_json_object_close(wb); // context
            }

            buffer_json_member_add_object(wb, rrdcontext_acquired_id(t->rca));
            buffer_json_member_add_object(wb, "charts");

            contexts++;
            charts = 0;
            context_dims = 0;
            contexts_total_weight = 0.0;

            last_ria = NULL;
        }

        if(t->ria != last_ria) {
            last_ria = t->ria;

            if(charts) {
                buffer_json_object_close(wb); // dimensions
                buffer_json_member_add_double(wb, "weight", charts_total_weight / (double) chart_dims);
                buffer_json_object_close(wb); // chart:id
            }

            buffer_json_member_add_object(wb, rrdinstance_acquired_id(t->ria));
            buffer_json_member_add_object(wb, "dimensions");

            charts++;
            chart_dims = 0;
            charts_total_weight = 0.0;
        }

        buffer_json_member_add_double(wb, rrdmetric_acquired_name(t->rma), t->value);
        charts_total_weight += t->value;
        contexts_total_weight += t->value;
        chart_dims++;
        context_dims++;
        total_dimensions++;
    }
    dfe_done(t);

    // close dimensions and chart
    if (total_dimensions) {
        buffer_json_object_close(wb); // dimensions
        buffer_json_member_add_double(wb, "weight", charts_total_weight / (double) chart_dims);
        buffer_json_object_close(wb); // chart:id
        buffer_json_object_close(wb); // charts
        buffer_json_member_add_double(wb, "weight", contexts_total_weight / (double) context_dims);
        buffer_json_object_close(wb); // context
    }

    buffer_json_object_close(wb);

    buffer_json_member_add_uint64(wb, "correlated_dimensions", total_dimensions);
    buffer_json_member_add_uint64(wb, "total_dimensions_count", examined_dimensions);
    buffer_json_finalize(wb);

    return total_dimensions;
}

// Workload statistics for progress tracking and thread optimization
struct workload_stats {
    size_t nodes;
    size_t contexts;
    size_t metrics;
};

struct query_weights_data {
    QUERY_WEIGHTS_REQUEST *qwr;

    SIMPLE_PATTERN *scope_nodes_sp;
    SIMPLE_PATTERN *scope_contexts_sp;
    SIMPLE_PATTERN *scope_instances_sp;
    SIMPLE_PATTERN *scope_labels_sp;
    SIMPLE_PATTERN *scope_dimensions_sp;
    SIMPLE_PATTERN *nodes_sp;
    SIMPLE_PATTERN *contexts_sp;
    SIMPLE_PATTERN *instances_sp;
    SIMPLE_PATTERN *dimensions_sp;
    SIMPLE_PATTERN *labels_sp;
    SIMPLE_PATTERN *alerts_sp;

    struct pattern_array *scope_labels_pa;
    struct pattern_array *labels_pa;

    usec_t timeout_us;
    bool timed_out;
    bool interrupted;

    struct query_timings timings;

    size_t examined_dimensions;
    bool register_zero;

    DICTIONARY *results;
    WEIGHTS_STATS stats;
    RRDHOST **hosts_array;
    size_t total_hosts;
    size_t hosts_array_capacity;

    uint32_t shifts;

    struct query_versions versions;
    struct workload_stats total_workload; // Overall workload statistics for progress tracking
};

// Thread-local data for parallel processing
struct query_weights_thread_data {
    struct query_weights_data *main_qwd;
    DICTIONARY *local_results;
    WEIGHTS_STATS local_stats;
    size_t local_examined_dimensions;
    struct query_versions local_versions;
    RRDHOST **hosts;
    struct completion completion;
    size_t host_count;
    size_t thread_id;
};

// Worker thread function for parallel host processing
void query_weights_worker_thread(void *arg)
{
    struct query_weights_thread_data *thread_data = (struct query_weights_thread_data *)arg;
    struct query_weights_data *main_qwd = thread_data->main_qwd;

    // Initialize local statistics
    memset(&thread_data->local_stats, 0, sizeof(WEIGHTS_STATS));
    thread_data->local_examined_dimensions = 0;
    memset(&thread_data->local_versions, 0, sizeof(struct query_versions));

    // Process assigned hosts
    for (size_t i = 0; i < thread_data->host_count; i++) {
        RRDHOST *host = thread_data->hosts[i];
        if (!host) continue;

        // Check for timeout/interruption
        if (__atomic_load_n(&main_qwd->timed_out, __ATOMIC_RELAXED) ||
            __atomic_load_n(&main_qwd->interrupted, __ATOMIC_RELAXED)) {
            break;
        }

        // Check timeout
        if (now_monotonic_usec() > (main_qwd->timings.received_ut + main_qwd->timeout_us)) {
            __atomic_store_n(&main_qwd->timed_out, true, __ATOMIC_RELAXED);
            break;
        }

        // Check interruption callback
        if (main_qwd->qwr->interrupt_callback &&
            main_qwd->qwr->interrupt_callback(main_qwd->qwr->interrupt_callback_data)) {
            __atomic_store_n(&main_qwd->interrupted, true, __ATOMIC_RELAXED);
            break;
        }

        // Create a local query_weights_data for this thread
        struct query_weights_data local_qwd = *main_qwd;
        local_qwd.results = thread_data->local_results;
        local_qwd.stats = thread_data->local_stats;
        local_qwd.examined_dimensions = thread_data->local_examined_dimensions;
        local_qwd.versions = thread_data->local_versions;

        char uuid[UUID_STR_LEN];
        if(!UUIDiszero(host->node_id))
            uuid_unparse_lower(host->node_id.uuid, uuid);
        else
            uuid[0] = '\0';

        SIMPLE_PATTERN_RESULT match = SP_MATCHED_POSITIVE;
        if(main_qwd->scope_nodes_sp) {
            match = simple_pattern_matches_string_extract(main_qwd->scope_nodes_sp, host->hostname, NULL, 0);
            if(match == SP_NOT_MATCHED) {
                match = simple_pattern_matches_extract(main_qwd->scope_nodes_sp, host->machine_guid, NULL, 0);
                if(match == SP_NOT_MATCHED && *uuid)
                    match = simple_pattern_matches_extract(main_qwd->scope_nodes_sp, uuid, NULL, 0);
            }
        }

        if(match != SP_MATCHED_POSITIVE)
            continue;

        if(main_qwd->nodes_sp) {
            match = simple_pattern_matches_string_extract(main_qwd->nodes_sp, host->hostname, NULL, 0);
            if(match == SP_NOT_MATCHED) {
                match = simple_pattern_matches_extract(main_qwd->nodes_sp, host->machine_guid, NULL, 0);
                if(match == SP_NOT_MATCHED && *uuid)
                    match = simple_pattern_matches_extract(main_qwd->nodes_sp, uuid, NULL, 0);
            }
        }

        bool queryable_host = (match == SP_MATCHED_POSITIVE);

        // Update local version hashes
        thread_data->local_versions.contexts_hard_hash += dictionary_version(host->rrdctx.contexts);
        thread_data->local_versions.contexts_soft_hash += rrdcontext_queue_version(&host->rrdctx.hub_queue);
        thread_data->local_versions.alerts_hard_hash += dictionary_version(host->rrdcalc_root_index);
        thread_data->local_versions.alerts_soft_hash += __atomic_load_n(&host->health_transitions, __ATOMIC_RELAXED);

        // Process the host using the callback
        ssize_t ret = weights_do_node_callback(&local_qwd, host, queryable_host);
        if (ret < 0)
            break;

        // Update thread-local counters
        thread_data->local_examined_dimensions = local_qwd.examined_dimensions;
        thread_data->local_stats = local_qwd.stats;
    }
}

// Thread-safe statistics merging - use simple addition since we're in single-threaded merge
static void merge_weights_stats(WEIGHTS_STATS *dest, const WEIGHTS_STATS *src) {
    dest->db_queries += src->db_queries;
    dest->db_points += src->db_points;
    dest->result_points += src->result_points;
    dest->binary_searches += src->binary_searches;

    // Update max ratio if needed
    if (src->max_base_high_ratio > dest->max_base_high_ratio) {
        dest->max_base_high_ratio = src->max_base_high_ratio;
    }

    for(size_t tier = 0; tier < RRD_STORAGE_TIERS; tier++) {
        dest->db_points_per_tier[tier] += src->db_points_per_tier[tier];
    }
}

#define AGGREGATED_WEIGHT_EMPTY (struct aggregated_weight) {        \
    .min = NAN,                                                     \
    .max = NAN,                                                     \
    .sum = NAN,                                                     \
    .count = 0,                                                     \
    .hsp = STORAGE_POINT_UNSET,                                     \
    .bsp = STORAGE_POINT_UNSET,                                     \
}

#define merge_into_aw(aw, t) do {                                   \
        if(!(aw).count) {                                           \
            (aw).count = 1;                                         \
            (aw).min = (aw).max = (aw).sum = (t)->value;            \
            (aw).hsp = (t)->highlighted;                            \
            if(baseline)                                            \
                (aw).bsp = (t)->baseline;                           \
        }                                                           \
        else {                                                      \
            (aw).count++;                                           \
            (aw).sum += (t)->value;                                 \
            if((t)->value < (aw).min)                               \
                (aw).min = (t)->value;                              \
            if((t)->value > (aw).max)                               \
                (aw).max = (t)->value;                              \
            storage_point_merge_to((aw).hsp, (t)->highlighted);     \
            if(baseline)                                            \
                storage_point_merge_to((aw).bsp, (t)->baseline);    \
        }                                                           \
} while(0)

static void results_header_to_json_v2(DICTIONARY *results __maybe_unused, BUFFER *wb, struct query_weights_data *qwd,
                                   time_t after, time_t before,
                                   time_t baseline_after, time_t baseline_before,
                                   size_t points, WEIGHTS_METHOD method,
                                   RRDR_TIME_GROUPING group, RRDR_OPTIONS options, uint32_t shifts,
                                   size_t examined_dimensions __maybe_unused, usec_t duration __maybe_unused,
                                   WEIGHTS_STATS *stats, bool group_by) {

    buffer_json_member_add_object(wb, "request");
    buffer_json_member_add_string(wb, "method", weights_method_to_string(method));
    rrdr_options_to_buffer_json_array(wb, "options", options);

    buffer_json_member_add_object(wb, "scope");
    buffer_json_member_add_string(wb, "scope_nodes", qwd->qwr->scope_nodes ? qwd->qwr->scope_nodes : "*");
    buffer_json_member_add_string(wb, "scope_contexts", qwd->qwr->scope_contexts ? qwd->qwr->scope_contexts : "*");
    buffer_json_member_add_string(wb, "scope_instances", qwd->qwr->scope_instances ? qwd->qwr->scope_instances : "*");
    buffer_json_member_add_string(wb, "scope_labels", qwd->qwr->scope_labels ? qwd->qwr->scope_labels : "*");
    buffer_json_object_close(wb);

    buffer_json_member_add_object(wb, "selectors");
    buffer_json_member_add_string(wb, "nodes", qwd->qwr->nodes ? qwd->qwr->nodes : "*");
    buffer_json_member_add_string(wb, "contexts", qwd->qwr->contexts ? qwd->qwr->contexts : "*");
    buffer_json_member_add_string(wb, "instances", qwd->qwr->instances ? qwd->qwr->instances : "*");
    buffer_json_member_add_string(wb, "dimensions", qwd->qwr->dimensions ? qwd->qwr->dimensions : "*");
    buffer_json_member_add_string(wb, "labels", qwd->qwr->labels ? qwd->qwr->labels : "*");
    buffer_json_member_add_string(wb, "alerts", qwd->qwr->alerts ? qwd->qwr->alerts : "*");
    buffer_json_object_close(wb);

    buffer_json_member_add_object(wb, "window");
    buffer_json_member_add_time_t_formatted(wb, "after", qwd->qwr->after, options & RRDR_OPTION_RFC3339);
    buffer_json_member_add_time_t_formatted(wb, "before", qwd->qwr->before, options & RRDR_OPTION_RFC3339);
    buffer_json_member_add_uint64(wb, "points", qwd->qwr->points);
    if(qwd->qwr->options & RRDR_OPTION_SELECTED_TIER)
        buffer_json_member_add_uint64(wb, "tier", qwd->qwr->tier);
    else
        buffer_json_member_add_string(wb, "tier", NULL);
    buffer_json_object_close(wb);

    if(method == WEIGHTS_METHOD_MC_KS2 || method == WEIGHTS_METHOD_MC_VOLUME) {
        buffer_json_member_add_object(wb, "baseline");
        buffer_json_member_add_time_t_formatted(wb, "baseline_after", qwd->qwr->baseline_after, options & RRDR_OPTION_RFC3339);
        buffer_json_member_add_time_t_formatted(wb, "baseline_before", qwd->qwr->baseline_before, options & RRDR_OPTION_RFC3339);
        buffer_json_object_close(wb);
    }

    buffer_json_member_add_object(wb, "aggregations");
    buffer_json_member_add_object(wb, "time");
    buffer_json_member_add_string(wb, "time_group", time_grouping_tostring(qwd->qwr->time_group_method));
    buffer_json_member_add_string(wb, "time_group_options", qwd->qwr->time_group_options);
    buffer_json_object_close(wb); // time

    buffer_json_member_add_array(wb, "metrics");
    buffer_json_add_array_item_object(wb);
    {
        buffer_json_member_add_array(wb, "group_by");
        buffer_json_group_by_to_array(wb, qwd->qwr->group_by.group_by);
        buffer_json_array_close(wb);

//        buffer_json_member_add_array(wb, "group_by_label");
//        buffer_json_array_close(wb);

        buffer_json_member_add_string(wb, "aggregation", group_by_aggregate_function_to_string(qwd->qwr->group_by.aggregation));
    }
    buffer_json_object_close(wb); // 1st group by
    buffer_json_array_close(wb); // array
    buffer_json_object_close(wb); // aggregations

    buffer_json_member_add_uint64(wb, "timeout", qwd->qwr->timeout_ms);
    buffer_json_object_close(wb); // request

    buffer_json_member_add_object(wb, "view");
    buffer_json_member_add_string(wb, "format", (group_by)?"grouped":"full");
    buffer_json_member_add_string(wb, "time_group", time_grouping_tostring(group));

    buffer_json_member_add_object(wb, "window");
    buffer_json_member_add_time_t_formatted(wb, "after", after, options & RRDR_OPTION_RFC3339);
    buffer_json_member_add_time_t_formatted(wb, "before", before, options & RRDR_OPTION_RFC3339);
    buffer_json_member_add_time_t(wb, "duration", before - after);
    buffer_json_member_add_uint64(wb, "points", points);
    buffer_json_object_close(wb);

    if(method == WEIGHTS_METHOD_MC_KS2 || method == WEIGHTS_METHOD_MC_VOLUME) {
        buffer_json_member_add_object(wb, "baseline");
        buffer_json_member_add_time_t_formatted(wb, "after", baseline_after, options & RRDR_OPTION_RFC3339);
        buffer_json_member_add_time_t_formatted(wb, "before", baseline_before, options & RRDR_OPTION_RFC3339);
        buffer_json_member_add_time_t(wb, "duration", baseline_before - baseline_after);
        buffer_json_member_add_uint64(wb, "points", points << shifts);
        buffer_json_object_close(wb);
    }

    buffer_json_object_close(wb); // view

    buffer_json_member_add_object(wb, "db");
    {
        buffer_json_member_add_uint64(wb, "db_queries", stats->db_queries);
        buffer_json_member_add_uint64(wb, "query_result_points", stats->result_points);
        buffer_json_member_add_uint64(wb, "binary_searches", stats->binary_searches);
        buffer_json_member_add_uint64(wb, "db_points_read", stats->db_points);

        buffer_json_member_add_array(wb, "db_points_per_tier");
        {
            for (size_t tier = 0; tier < nd_profile.storage_tiers; tier++)
                buffer_json_add_array_item_uint64(wb, stats->db_points_per_tier[tier]);
        }
        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb); // db
}

typedef enum {
    WPT_DIMENSION = 0,
    WPT_INSTANCE = 1,
    WPT_CONTEXT = 2,
    WPT_NODE = 3,
    WPT_GROUP = 4,
} WEIGHTS_POINT_TYPE;

struct aggregated_weight {
    const char *name;
    NETDATA_DOUBLE min;
    NETDATA_DOUBLE max;
    NETDATA_DOUBLE sum;
    size_t count;
    STORAGE_POINT hsp;
    STORAGE_POINT bsp;
};

static inline void storage_point_to_json(BUFFER *wb, WEIGHTS_POINT_TYPE type, ssize_t di, ssize_t ii, ssize_t ci, ssize_t ni, struct aggregated_weight *aw, RRDR_OPTIONS options __maybe_unused, bool baseline) {
    if(type != WPT_GROUP) {
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_uint64(wb, type); // "type"
        buffer_json_add_array_item_int64(wb, ni);
        if (type != WPT_NODE) {
            buffer_json_add_array_item_int64(wb, ci);
            if (type != WPT_CONTEXT) {
                buffer_json_add_array_item_int64(wb, ii);
                if (type != WPT_INSTANCE)
                    buffer_json_add_array_item_int64(wb, di);
                else
                    buffer_json_add_array_item_string(wb, NULL);
            }
            else {
                buffer_json_add_array_item_string(wb, NULL);
                buffer_json_add_array_item_string(wb, NULL);
            }
        }
        else {
            buffer_json_add_array_item_string(wb, NULL);
            buffer_json_add_array_item_string(wb, NULL);
            buffer_json_add_array_item_string(wb, NULL);
        }
        buffer_json_add_array_item_double(wb, (aw->count) ? aw->sum / (NETDATA_DOUBLE)aw->count : 0.0); // "weight"
    }
    else {
        buffer_json_member_add_array(wb, "v");
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_double(wb, aw->min); // "min"
        buffer_json_add_array_item_double(wb, (aw->count) ? aw->sum / (NETDATA_DOUBLE)aw->count : 0.0); // "avg"
        buffer_json_add_array_item_double(wb, aw->max); // "max"
        buffer_json_add_array_item_double(wb, aw->sum); // "sum"
        buffer_json_add_array_item_uint64(wb, aw->count); // "count"
        buffer_json_array_close(wb);
    }

    buffer_json_add_array_item_array(wb);
    buffer_json_add_array_item_double(wb, aw->hsp.min); // "min"
    buffer_json_add_array_item_double(wb, (aw->hsp.count) ? aw->hsp.sum / (NETDATA_DOUBLE) aw->hsp.count : 0.0); // "avg"
    buffer_json_add_array_item_double(wb, aw->hsp.max); // "max"
    buffer_json_add_array_item_double(wb, aw->hsp.sum); // "sum"
    buffer_json_add_array_item_uint64(wb, aw->hsp.count); // "count"
    buffer_json_add_array_item_uint64(wb, aw->hsp.anomaly_count); // "anomaly_count"
    buffer_json_array_close(wb);

    if(baseline) {
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_double(wb, aw->bsp.min); // "min"
        buffer_json_add_array_item_double(wb, (aw->bsp.count) ? aw->bsp.sum / (NETDATA_DOUBLE) aw->bsp.count : 0.0); // "avg"
        buffer_json_add_array_item_double(wb, aw->bsp.max); // "max"
        buffer_json_add_array_item_double(wb, aw->bsp.sum); // "sum"
        buffer_json_add_array_item_uint64(wb, aw->bsp.count); // "count"
        buffer_json_add_array_item_uint64(wb, aw->bsp.anomaly_count); // "anomaly_count"
        buffer_json_array_close(wb);
    }

    buffer_json_array_close(wb);
}

static void multinode_data_schema(BUFFER *wb, RRDR_OPTIONS options __maybe_unused, const char *key, bool baseline, bool group_by) {
    buffer_json_member_add_object(wb, key); // schema

    buffer_json_member_add_string(wb, "type", "array");
    buffer_json_member_add_array(wb, "items");

    if(group_by) {
        buffer_json_add_array_item_object(wb);
        {
            buffer_json_member_add_string(wb, "name", "weight");
            buffer_json_member_add_string(wb, "type", "array");
            buffer_json_member_add_array(wb, "labels");
            {
                buffer_json_add_array_item_string(wb, "min");
                buffer_json_add_array_item_string(wb, "avg");
                buffer_json_add_array_item_string(wb, "max");
                buffer_json_add_array_item_string(wb, "sum");
                buffer_json_add_array_item_string(wb, "count");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    else {
        buffer_json_add_array_item_object(wb);
        buffer_json_member_add_string(wb, "name", "row_type");
        buffer_json_member_add_string(wb, "type", "integer");
        buffer_json_member_add_array(wb, "value");
        buffer_json_add_array_item_string(wb, "dimension");
        buffer_json_add_array_item_string(wb, "instance");
        buffer_json_add_array_item_string(wb, "context");
        buffer_json_add_array_item_string(wb, "node");
        buffer_json_array_close(wb);
        buffer_json_object_close(wb);

        buffer_json_add_array_item_object(wb);
        {
            buffer_json_member_add_string(wb, "name", "ni");
            buffer_json_member_add_string(wb, "type", "integer");
            buffer_json_member_add_string(wb, "dictionary", "nodes");
        }
        buffer_json_object_close(wb);

        buffer_json_add_array_item_object(wb);
        {
            buffer_json_member_add_string(wb, "name", "ci");
            buffer_json_member_add_string(wb, "type", "integer");
            buffer_json_member_add_string(wb, "dictionary", "contexts");
        }
        buffer_json_object_close(wb);

        buffer_json_add_array_item_object(wb);
        {
            buffer_json_member_add_string(wb, "name", "ii");
            buffer_json_member_add_string(wb, "type", "integer");
            buffer_json_member_add_string(wb, "dictionary", "instances");
        }
        buffer_json_object_close(wb);

        buffer_json_add_array_item_object(wb);
        {
            buffer_json_member_add_string(wb, "name", "di");
            buffer_json_member_add_string(wb, "type", "integer");
            buffer_json_member_add_string(wb, "dictionary", "dimensions");
        }
        buffer_json_object_close(wb);

        buffer_json_add_array_item_object(wb);
        {
            buffer_json_member_add_string(wb, "name", "weight");
            buffer_json_member_add_string(wb, "type", "number");
        }
        buffer_json_object_close(wb);
    }

    buffer_json_add_array_item_object(wb);
    {
        buffer_json_member_add_string(wb, "name", "timeframe");
        buffer_json_member_add_string(wb, "type", "array");
        buffer_json_member_add_array(wb, "labels");
        {
            buffer_json_add_array_item_string(wb, "min");
            buffer_json_add_array_item_string(wb, "avg");
            buffer_json_add_array_item_string(wb, "max");
            buffer_json_add_array_item_string(wb, "sum");
            buffer_json_add_array_item_string(wb, "count");
            buffer_json_add_array_item_string(wb, "anomaly_count");
        }
        buffer_json_array_close(wb);
        buffer_json_member_add_object(wb, "calculations");
        buffer_json_member_add_string(wb, "anomaly rate", "anomaly_count * 100 / count");
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb);

    if(baseline) {
        buffer_json_add_array_item_object(wb);
        {
            buffer_json_member_add_string(wb, "name", "baseline timeframe");
            buffer_json_member_add_string(wb, "type", "array");
            buffer_json_member_add_array(wb, "labels");
            {
                buffer_json_add_array_item_string(wb, "min");
                buffer_json_add_array_item_string(wb, "avg");
                buffer_json_add_array_item_string(wb, "max");
                buffer_json_add_array_item_string(wb, "sum");
                buffer_json_add_array_item_string(wb, "count");
                buffer_json_add_array_item_string(wb, "anomaly_count");
            }
            buffer_json_array_close(wb);
            buffer_json_member_add_object(wb, "calculations");
            buffer_json_member_add_string(wb, "anomaly rate", "anomaly_count * 100 / count");
            buffer_json_object_close(wb);
        }
        buffer_json_object_close(wb);
    }

    buffer_json_array_close(wb); // items
    buffer_json_object_close(wb); // schema
}

struct dict_unique_node {
    bool existing;
    bool exposed;
    uint32_t i;
    RRDHOST *host;
    usec_t duration_ut;
};

struct dict_unique_name_units {
    bool existing;
    bool exposed;
    uint32_t i;
    const char *units;
};

struct dict_unique_id_name {
    bool existing;
    bool exposed;
    uint32_t i;
    const char *id;
    const char *name;
};

static inline struct dict_unique_node *dict_unique_node_add(DICTIONARY *dict, RRDHOST *host, ssize_t *max_id) {
    struct dict_unique_node *dun = dictionary_set(dict, host->machine_guid, NULL, sizeof(struct dict_unique_node));
    if(!dun->existing) {
        dun->existing = true;
        dun->host = host;
        dun->i = *max_id;
        (*max_id)++;
    }

    return dun;
}

static inline struct dict_unique_name_units *dict_unique_name_units_add(DICTIONARY *dict, const char *name, const char *units, ssize_t *max_id) {
    struct dict_unique_name_units *dun = dictionary_set(dict, name, NULL, sizeof(struct dict_unique_name_units));
    if(!dun->existing) {
        dun->units = units;
        dun->existing = true;
        dun->i = *max_id;
        (*max_id)++;
    }

    return dun;
}

static inline struct dict_unique_id_name *dict_unique_id_name_add(DICTIONARY *dict, const char *id, const char *name, ssize_t *max_id) {
    char key[1024 + 1];
    snprintfz(key, sizeof(key) - 1, "%s:%s", id, name);
    struct dict_unique_id_name *dun = dictionary_set(dict, key, NULL, sizeof(struct dict_unique_id_name));
    if(!dun->existing) {
        dun->existing = true;
        dun->i = *max_id;
        (*max_id)++;
        dun->id = id;
        dun->name = name;
    }

    return dun;
}

static size_t registered_results_to_json_multinode_no_group_by(
        DICTIONARY *results, BUFFER *wb,
        time_t after, time_t before,
        time_t baseline_after, time_t baseline_before,
        size_t points, WEIGHTS_METHOD method,
        RRDR_TIME_GROUPING group, RRDR_OPTIONS options, uint32_t shifts,
        size_t examined_dimensions, struct query_weights_data *qwd,
        WEIGHTS_STATS *stats,
        struct query_versions *versions) {
    buffer_json_initialize(wb, "\"", "\"", 0, true, (options & RRDR_OPTION_MINIFY) ? BUFFER_JSON_OPTIONS_MINIFY : BUFFER_JSON_OPTIONS_DEFAULT);
    buffer_json_member_add_uint64(wb, "api", 2);

    results_header_to_json_v2(results, wb, qwd, after, before, baseline_after, baseline_before,
                           points, method, group, options, shifts, examined_dimensions,
                           qwd->timings.executed_ut - qwd->timings.received_ut, stats, false);

    version_hashes_api_v2(wb, versions);

    bool baseline = method == WEIGHTS_METHOD_MC_KS2 || method == WEIGHTS_METHOD_MC_VOLUME;
    multinode_data_schema(wb, options, "schema", baseline, false);

    DICTIONARY *dict_nodes = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct dict_unique_node));
    DICTIONARY *dict_contexts = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct dict_unique_name_units));
    DICTIONARY *dict_instances = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct dict_unique_id_name));
    DICTIONARY *dict_dimensions = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct dict_unique_id_name));

    buffer_json_member_add_array(wb, "result");

    struct aggregated_weight node_aw = AGGREGATED_WEIGHT_EMPTY, context_aw = AGGREGATED_WEIGHT_EMPTY, instance_aw = AGGREGATED_WEIGHT_EMPTY;
    struct register_result *t;
    RRDHOST *last_host = NULL;
    RRDCONTEXT_ACQUIRED *last_rca = NULL;
    RRDINSTANCE_ACQUIRED *last_ria = NULL;
    struct dict_unique_name_units *context_dun = NULL;
    struct dict_unique_node *node_dun = NULL;
    struct dict_unique_id_name *instance_dun = NULL;
    struct dict_unique_id_name *dimension_dun = NULL;
    ssize_t di = -1, ii = -1, ci = -1, ni = -1;
    ssize_t di_max = 0, ii_max = 0, ci_max = 0, ni_max = 0;
    size_t total_dimensions = 0;
    dfe_start_read(results, t) {

        // close instance
        if(t->ria != last_ria && last_ria) {
            storage_point_to_json(wb, WPT_INSTANCE, di, ii, ci, ni, &instance_aw, options, baseline);
            instance_dun->exposed = true;
            last_ria = NULL;
            instance_aw = AGGREGATED_WEIGHT_EMPTY;
        }

        // close context
        if(t->rca != last_rca && last_rca) {
            storage_point_to_json(wb, WPT_CONTEXT, di, ii, ci, ni, &context_aw, options, baseline);
            context_dun->exposed = true;
            last_rca = NULL;
            context_aw = AGGREGATED_WEIGHT_EMPTY;
        }

        // close node
        if(t->host != last_host && last_host) {
            storage_point_to_json(wb, WPT_NODE, di, ii, ci, ni, &node_aw, options, baseline);
            node_dun->exposed = true;
            last_host = NULL;
            node_aw = AGGREGATED_WEIGHT_EMPTY;
        }

        // open node
        if(t->host != last_host) {
            last_host = t->host;
            node_dun = dict_unique_node_add(dict_nodes, t->host, &ni_max);
            ni = node_dun->i;
        }

        // open context
        if(t->rca != last_rca) {
            last_rca = t->rca;
            context_dun = dict_unique_name_units_add(dict_contexts, rrdcontext_acquired_id(t->rca),
                                                     rrdcontext_acquired_units(t->rca), &ci_max);
            ci = context_dun->i;
        }

        // open instance
        if(t->ria != last_ria) {
            last_ria = t->ria;
            instance_dun = dict_unique_id_name_add(dict_instances, rrdinstance_acquired_id(t->ria), rrdinstance_acquired_name(t->ria), &ii_max);
            ii = instance_dun->i;
        }

        dimension_dun = dict_unique_id_name_add(dict_dimensions, rrdmetric_acquired_id(t->rma), rrdmetric_acquired_name(t->rma), &di_max);
        di = dimension_dun->i;

        struct aggregated_weight aw = {
                .min = t->value,
                .max = t->value,
                .sum = t->value,
                .count = 1,
                .hsp = t->highlighted,
                .bsp = t->baseline,
        };

        storage_point_to_json(wb, WPT_DIMENSION, di, ii, ci, ni, &aw, options, baseline);
        node_dun->exposed = true;
        context_dun->exposed = true;
        instance_dun->exposed = true;
        dimension_dun->exposed = true;

        merge_into_aw(instance_aw, t);
        merge_into_aw(context_aw, t);
        merge_into_aw(node_aw, t);

        node_dun->duration_ut += t->duration_ut;
        total_dimensions++;
    }
    dfe_done(t);

    // close instance
    if(last_ria) {
        storage_point_to_json(wb, WPT_INSTANCE, di, ii, ci, ni, &instance_aw, options, baseline);
        instance_dun->exposed = true;
    }

    // close context
    if(last_rca) {
        storage_point_to_json(wb, WPT_CONTEXT, di, ii, ci, ni, &context_aw, options, baseline);
        context_dun->exposed = true;
    }

    // close node
    if(last_host) {
        storage_point_to_json(wb, WPT_NODE, di, ii, ci, ni, &node_aw, options, baseline);
        node_dun->exposed = true;
    }

    buffer_json_array_close(wb); // points

    buffer_json_member_add_object(wb, "dictionaries");
    buffer_json_member_add_array(wb, "nodes");
    {
        struct dict_unique_node *dun;
        dfe_start_read(dict_nodes, dun) {
            if(!dun->exposed)
                continue;

            buffer_json_add_array_item_object(wb);
            buffer_json_node_add_v2(wb, dun->host, dun->i, dun->duration_ut, true);
            buffer_json_object_close(wb);
        }
        dfe_done(dun);
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_array(wb, "contexts");
    {
        struct dict_unique_name_units *dun;
        dfe_start_read(dict_contexts, dun) {
            if(!dun->exposed)
                continue;

            buffer_json_add_array_item_object(wb);
            buffer_json_member_add_string(wb, "id", dun_dfe.name);
            buffer_json_member_add_string(wb, "units", dun->units);
            buffer_json_member_add_int64(wb, "ci", dun->i);
            buffer_json_object_close(wb);
        }
        dfe_done(dun);
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_array(wb, "instances");
    {
        struct dict_unique_id_name *dun;
        dfe_start_read(dict_instances, dun) {
            if(!dun->exposed)
                continue;

            buffer_json_add_array_item_object(wb);
            buffer_json_member_add_string(wb, "id", dun->id);
            if(dun->id != dun->name)
                buffer_json_member_add_string(wb, "nm", dun->name);
            buffer_json_member_add_int64(wb, "ii", dun->i);
            buffer_json_object_close(wb);
        }
        dfe_done(dun);
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_array(wb, "dimensions");
    {
        struct dict_unique_id_name *dun;
        dfe_start_read(dict_dimensions, dun) {
            if(!dun->exposed)
                continue;

            buffer_json_add_array_item_object(wb);
            buffer_json_member_add_string(wb, "id", dun->id);
            if(dun->id != dun->name)
                buffer_json_member_add_string(wb, "nm", dun->name);
            buffer_json_member_add_int64(wb, "di", dun->i);
            buffer_json_object_close(wb);
        }
        dfe_done(dun);
    }
    buffer_json_array_close(wb);

    buffer_json_object_close(wb); //dictionaries

    buffer_json_agents_v2(wb, &qwd->timings, 0, false, true, rrdr_options_to_contexts_options(options));
    buffer_json_member_add_uint64(wb, "correlated_dimensions", total_dimensions);
    buffer_json_member_add_uint64(wb, "total_dimensions_count", examined_dimensions);
    buffer_json_finalize(wb);

    dictionary_destroy(dict_nodes);
    dictionary_destroy(dict_contexts);
    dictionary_destroy(dict_instances);
    dictionary_destroy(dict_dimensions);

    return total_dimensions;
}

static size_t registered_results_to_json_multinode_group_by(
        DICTIONARY *results, BUFFER *wb,
        time_t after, time_t before,
        time_t baseline_after, time_t baseline_before,
        size_t points, WEIGHTS_METHOD method,
        RRDR_TIME_GROUPING group, RRDR_OPTIONS options, uint32_t shifts,
        size_t examined_dimensions, struct query_weights_data *qwd,
        WEIGHTS_STATS *stats,
        struct query_versions *versions) {
    buffer_json_initialize(wb, "\"", "\"", 0, true, (options & RRDR_OPTION_MINIFY) ? BUFFER_JSON_OPTIONS_MINIFY : BUFFER_JSON_OPTIONS_DEFAULT);
    buffer_json_member_add_uint64(wb, "api", 2);

    results_header_to_json_v2(results, wb, qwd, after, before, baseline_after, baseline_before,
                           points, method, group, options, shifts, examined_dimensions,
                           qwd->timings.executed_ut - qwd->timings.received_ut, stats, true);

    version_hashes_api_v2(wb, versions);

    bool baseline = method == WEIGHTS_METHOD_MC_KS2 || method == WEIGHTS_METHOD_MC_VOLUME;
    multinode_data_schema(wb, options, "v_schema", baseline, true);

    DICTIONARY *group_by = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                      NULL, sizeof(struct aggregated_weight));

    struct register_result *t;
    size_t total_dimensions = 0;
    BUFFER *key = buffer_create(0, NULL);
    BUFFER *name = buffer_create(0, NULL);
    dfe_start_read(results, t) {
        char node_uuid[UUID_STR_LEN];

        if(UUIDiszero(t->host->node_id))
            uuid_unparse_lower(t->host->host_id.uuid, node_uuid);
        else
            uuid_unparse_lower(t->host->node_id.uuid, node_uuid);

        buffer_flush(key);
        buffer_flush(name);

        if(qwd->qwr->group_by.group_by & RRDR_GROUP_BY_DIMENSION) {
            buffer_strcat(key, rrdmetric_acquired_name(t->rma));
            buffer_strcat(name, rrdmetric_acquired_name(t->rma));
        }
        if(qwd->qwr->group_by.group_by & RRDR_GROUP_BY_INSTANCE) {
            if(buffer_strlen(key)) {
                buffer_fast_strcat(key, ",", 1);
                buffer_fast_strcat(name, ",", 1);
            }

            buffer_strcat(key, rrdinstance_acquired_id(t->ria));
            buffer_strcat(name, rrdinstance_acquired_name(t->ria));

            if(!(qwd->qwr->group_by.group_by & RRDR_GROUP_BY_NODE)) {
                buffer_fast_strcat(key, "@", 1);
                buffer_fast_strcat(name, "@", 1);
                buffer_strcat(key, node_uuid);
                buffer_strcat(name, rrdhost_hostname(t->host));
            }
        }
        if(qwd->qwr->group_by.group_by & RRDR_GROUP_BY_NODE) {
            if(buffer_strlen(key)) {
                buffer_fast_strcat(key, ",", 1);
                buffer_fast_strcat(name, ",", 1);
            }

            buffer_strcat(key, node_uuid);
            buffer_strcat(name, rrdhost_hostname(t->host));
        }
        if(qwd->qwr->group_by.group_by & RRDR_GROUP_BY_CONTEXT) {
            if(buffer_strlen(key)) {
                buffer_fast_strcat(key, ",", 1);
                buffer_fast_strcat(name, ",", 1);
            }

            buffer_strcat(key, rrdcontext_acquired_id(t->rca));
            buffer_strcat(name, rrdcontext_acquired_id(t->rca));
        }
        if(qwd->qwr->group_by.group_by & RRDR_GROUP_BY_UNITS) {
            if(buffer_strlen(key)) {
                buffer_fast_strcat(key, ",", 1);
                buffer_fast_strcat(name, ",", 1);
            }

            buffer_strcat(key, rrdcontext_acquired_units(t->rca));
            buffer_strcat(name, rrdcontext_acquired_units(t->rca));
        }

        struct aggregated_weight *aw = dictionary_set(group_by, buffer_tostring(key), NULL, sizeof(struct aggregated_weight));
        if(!aw->name) {
            aw->name = strdupz(buffer_tostring(name));
            aw->min = aw->max = aw->sum = t->value;
            aw->count = 1;
            aw->hsp = t->highlighted;
            aw->bsp = t->baseline;
        }
        else
            merge_into_aw(*aw, t);

        total_dimensions++;
    }
    dfe_done(t);
    buffer_free(key); key = NULL;
    buffer_free(name); name = NULL;

    struct aggregated_weight *aw;
    buffer_json_member_add_array(wb, "result");
    dfe_start_read(group_by, aw) {
        const char *k = aw_dfe.name;
        const char *n = aw->name;

        buffer_json_add_array_item_object(wb);
        buffer_json_member_add_string(wb, "id", k);

        if(strcmp(k, n) != 0)
            buffer_json_member_add_string(wb, "nm", n);

        storage_point_to_json(wb, WPT_GROUP, 0, 0, 0, 0, aw, options, baseline);
        buffer_json_object_close(wb);

        freez((void *)aw->name);
    }
    dfe_done(aw);
    buffer_json_array_close(wb); // result

    buffer_json_agents_v2(wb, &qwd->timings, 0, false, true, rrdr_options_to_contexts_options(options));
    buffer_json_member_add_uint64(wb, "correlated_dimensions", total_dimensions);
    buffer_json_member_add_uint64(wb, "total_dimensions_count", examined_dimensions);
    buffer_json_finalize(wb);

    dictionary_destroy(group_by);

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

static double ks_2samp(
        DIFFS_NUMBERS baseline_diffs[], int base_size,
        DIFFS_NUMBERS highlight_diffs[], int high_size,
        uint32_t base_shifts) {

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
    // but, then we divide the base index by the power of two number (shifts) it
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
    NETDATA_DOUBLE highlight[], int highlight_points,
    uint32_t base_shifts) {

    // -1 in size, since the calculate_pairs_diffs() returns one less point
    DIFFS_NUMBERS baseline_diffs[baseline_points - 1];
    DIFFS_NUMBERS highlight_diffs[highlight_points - 1];

    int base_size = (int)calculate_pairs_diff(baseline_diffs, baseline, baseline_points);
    int high_size = (int)calculate_pairs_diff(highlight_diffs, highlight, highlight_points);

    if(unlikely(!base_size || !high_size))
        return NAN;

    if(unlikely(base_size != baseline_points - 1 || high_size != highlight_points - 1)) {
        netdata_log_error("Metric correlations: internal error - calculate_pairs_diff() returns the wrong number of entries");
        return NAN;
    }

    return ks_2samp(baseline_diffs, base_size, highlight_diffs, high_size, base_shifts);
}

NETDATA_DOUBLE *rrd2rrdr_ks2(
        ONEWAYALLOC *owa, RRDHOST *host,
        RRDCONTEXT_ACQUIRED *rca, RRDINSTANCE_ACQUIRED *ria, RRDMETRIC_ACQUIRED *rma,
        time_t after, time_t before, size_t points, RRDR_OPTIONS options,
        RRDR_TIME_GROUPING time_group_method, const char *time_group_options, size_t tier,
        WEIGHTS_STATS *stats,
        size_t *entries,
        STORAGE_POINT *sp
        ) {

    NETDATA_DOUBLE *ret = NULL;

    QUERY_TARGET_REQUEST qtr = {
            .version = 1,
            .host = host,
            .rca = rca,
            .ria = ria,
            .rma = rma,
            .after = after,
            .before = before,
            .points = points,
            .options = options,
            .time_group_method = time_group_method,
            .time_group_options = time_group_options,
            .tier = tier,
            .query_source = QUERY_SOURCE_API_WEIGHTS,
            .priority = STORAGE_PRIORITY_SYNCHRONOUS_FIRST,
    };

    QUERY_TARGET *qt = query_target_create(&qtr);
    stream_control_user_weights_query_started();
    RRDR *r = rrd2rrdr(owa, qt);
    stream_control_user_weights_query_finished();

    if(!r)
        goto cleanup;

    stats->db_queries++;
    stats->result_points += r->stats.result_points_generated;
    stats->db_points += r->stats.db_points_read;
    for(size_t tr = 0; tr < nd_profile.storage_tiers; tr++)
        stats->db_points_per_tier[tr] += r->internal.qt->db.tiers[tr].points;

    if(!r->d || !r->internal.qt->query.used) {
        // the result is empty - no data to query for this metric
        goto cleanup;
    }
    
    if(r->d != 1 || r->internal.qt->query.used != 1) {
        netdata_log_error("WEIGHTS: on query '%s' expected 1 dimension in RRDR but got %zu r->d and %zu qt->query.used",
                          r->internal.qt->id, r->d, (size_t)r->internal.qt->query.used);
        goto cleanup;
    }

    if(unlikely(r->od[0] & RRDR_DIMENSION_HIDDEN))
        goto cleanup;

    if(unlikely(!(r->od[0] & RRDR_DIMENSION_QUERIED)))
        goto cleanup;

    if(unlikely(!(r->od[0] & RRDR_DIMENSION_NONZERO)))
        goto cleanup;

    if(rrdr_rows(r) < 2)
        goto cleanup;

    *entries = rrdr_rows(r);
    ret = onewayalloc_mallocz(owa, sizeof(NETDATA_DOUBLE) * rrdr_rows(r));

    if(sp)
        *sp = r->internal.qt->query.array[0].query_points;

    // copy the points of the dimension to a contiguous array
    // there is no need to check for empty values, since empty values are already zero
    // https://github.com/netdata/netdata/blob/6e3144683a73a2024d51425b20ecfd569034c858/web/api/queries/average/average.c#L41-L43
    memcpy(ret, r->v, rrdr_rows(r) * sizeof(NETDATA_DOUBLE));

cleanup:
    rrdr_free(owa, r);
    query_target_release(qt);
    return ret;
}

static void rrdset_metric_correlations_ks2(
        RRDHOST *host,
        RRDCONTEXT_ACQUIRED *rca, RRDINSTANCE_ACQUIRED *ria, RRDMETRIC_ACQUIRED *rma,
        DICTIONARY *results,
        time_t baseline_after, time_t baseline_before,
        time_t after, time_t before,
        size_t points, RRDR_OPTIONS options,
        RRDR_TIME_GROUPING time_group_method, const char *time_group_options, size_t tier,
        uint32_t shifts,
        WEIGHTS_STATS *stats, bool register_zero
        ) {

    options |= RRDR_OPTION_NATURAL_POINTS;

    usec_t started_ut = now_monotonic_usec();
    ONEWAYALLOC *owa = onewayalloc_create(16 * 1024);

    size_t high_points = 0;
    STORAGE_POINT highlighted_sp;
    NETDATA_DOUBLE *highlight = NULL, *baseline = NULL;

    highlight = rrd2rrdr_ks2(
            owa, host, rca, ria, rma, after, before, points,
            options, time_group_method, time_group_options, tier, stats, &high_points, &highlighted_sp);

    if(!highlight)
        goto cleanup;

    size_t base_points = 0;
    STORAGE_POINT baseline_sp;
    baseline = rrd2rrdr_ks2(
            owa, host, rca, ria, rma, baseline_after, baseline_before, high_points << shifts,
            options, time_group_method, time_group_options, tier, stats, &base_points, &baseline_sp);

    if(!baseline)
        goto cleanup;

    stats->binary_searches += 2 * (base_points - 1) + 2 * (high_points - 1);

    double prob = kstwo(baseline, (int)base_points, highlight, (int)high_points, shifts);
    if(!isnan(prob) && !isinf(prob)) {

        // these conditions should never happen, but still let's check
        if(unlikely(prob < 0.0)) {
            netdata_log_error("Metric correlations: kstwo() returned a negative number: %f", prob);
            prob = -prob;
        }
        if(unlikely(prob > 1.0)) {
            netdata_log_error("Metric correlations: kstwo() returned a number above 1.0: %f", prob);
            prob = 1.0;
        }

        usec_t ended_ut = now_monotonic_usec();

        // to spread the results evenly, 0.0 needs to be the less correlated and 1.0 the most correlated
        // so, we flip the result of kstwo()
        register_result(results, host, rca, ria, rma, 1.0 - prob, RESULT_IS_BASE_HIGH_RATIO, &highlighted_sp,
                        &baseline_sp, stats, register_zero, ended_ut - started_ut);
    }

cleanup:
    onewayalloc_freez(owa, highlight);
    onewayalloc_freez(owa, baseline);
    onewayalloc_destroy(owa);
}

// ----------------------------------------------------------------------------
// VOLUME algorithm functions

static void merge_query_value_to_stats(QUERY_VALUE *qv, WEIGHTS_STATS *stats, size_t queries) {
    stats->db_queries += queries;
    stats->result_points += qv->result_points;
    stats->db_points += qv->points_read;
    for(size_t tier = 0; tier < nd_profile.storage_tiers; tier++)
        stats->db_points_per_tier[tier] += qv->storage_points_per_tier[tier];
}

static void rrdset_metric_correlations_volume(
        RRDHOST *host,
        RRDCONTEXT_ACQUIRED *rca, RRDINSTANCE_ACQUIRED *ria, RRDMETRIC_ACQUIRED *rma,
        DICTIONARY *results,
        time_t baseline_after, time_t baseline_before,
        time_t after, time_t before,
        RRDR_OPTIONS options, RRDR_TIME_GROUPING time_group_method, const char *time_group_options,
        size_t tier,
        WEIGHTS_STATS *stats, bool register_zero) {

    options |= RRDR_OPTION_MATCH_IDS | RRDR_OPTION_ABSOLUTE | RRDR_OPTION_NATURAL_POINTS;

    QUERY_VALUE baseline_average = rrdmetric2value(host, rca, ria, rma, baseline_after, baseline_before,
                                                   options, time_group_method, time_group_options, tier, 0,
                                                   QUERY_SOURCE_API_WEIGHTS, STORAGE_PRIORITY_SYNCHRONOUS_FIRST);
    merge_query_value_to_stats(&baseline_average, stats, 1);

    if(!netdata_double_isnumber(baseline_average.value)) {
        // this means no data for the baseline window, but we may have data for the highlighted one - assume zero
        baseline_average.value = 0.0;
    }

    QUERY_VALUE highlight_average = rrdmetric2value(host, rca, ria, rma, after, before,
                                                    options, time_group_method, time_group_options, tier, 0,
                                                    QUERY_SOURCE_API_WEIGHTS, STORAGE_PRIORITY_SYNCHRONOUS_FIRST);
    merge_query_value_to_stats(&highlight_average, stats, 1);

    if(!netdata_double_isnumber(highlight_average.value))
        return;

    if(baseline_average.value == highlight_average.value) {
        // they are the same - let's move on
        return;
    }

    if((options & RRDR_OPTION_ANOMALY_BIT) && highlight_average.value < baseline_average.value) {
        // when working on anomaly bits, we are looking for an increase in the anomaly rate
        return;
    }

    char highlight_countif_options[50 + 1];
    snprintfz(highlight_countif_options, 50, "%s" NETDATA_DOUBLE_FORMAT, highlight_average.value < baseline_average.value ? "<" : ">", baseline_average.value);
    QUERY_VALUE highlight_countif = rrdmetric2value(host, rca, ria, rma, after, before,
                                                    options, RRDR_GROUPING_COUNTIF, highlight_countif_options, tier, 0,
                                                    QUERY_SOURCE_API_WEIGHTS, STORAGE_PRIORITY_SYNCHRONOUS_FIRST);
    merge_query_value_to_stats(&highlight_countif, stats, 1);

    if(!netdata_double_isnumber(highlight_countif.value)) {
        netdata_log_info("WEIGHTS: highlighted countif query failed, but highlighted average worked - strange...");
        return;
    }

    // this represents the percentage of time
    // the highlighted window was above/below the baseline window
    // (above or below depending on their averages)
    highlight_countif.value = highlight_countif.value / 100.0; // countif returns 0 - 100.0

    RESULT_FLAGS flags;
    NETDATA_DOUBLE pcent = NAN;
    if(isgreater(baseline_average.value, 0.0) || isless(baseline_average.value, 0.0)) {
        flags = RESULT_IS_BASE_HIGH_RATIO;
        pcent = (highlight_average.value - baseline_average.value) / baseline_average.value * highlight_countif.value;
    }
    else {
        flags = RESULT_IS_PERCENTAGE_OF_TIME;
        pcent = highlight_countif.value;
    }

    register_result(results, host, rca, ria, rma, pcent, flags, &highlight_average.sp, &baseline_average.sp, stats,
                    register_zero, baseline_average.duration_ut + highlight_average.duration_ut + highlight_countif.duration_ut);
}

// ----------------------------------------------------------------------------
// VALUE / ANOMALY RATE algorithm functions

static void rrdset_weights_value(
        RRDHOST *host,
        RRDCONTEXT_ACQUIRED *rca, RRDINSTANCE_ACQUIRED *ria, RRDMETRIC_ACQUIRED *rma,
        DICTIONARY *results,
        time_t after, time_t before,
        RRDR_OPTIONS options, RRDR_TIME_GROUPING time_group_method, const char *time_group_options,
        size_t tier,
        WEIGHTS_STATS *stats, bool register_zero) {

    options |= RRDR_OPTION_MATCH_IDS | RRDR_OPTION_NATURAL_POINTS;

    QUERY_VALUE qv = rrdmetric2value(host, rca, ria, rma, after, before,
                                     options, time_group_method, time_group_options, tier, 0,
                                     QUERY_SOURCE_API_WEIGHTS, STORAGE_PRIORITY_SYNCHRONOUS_FIRST);

    merge_query_value_to_stats(&qv, stats, 1);

    if(netdata_double_isnumber(qv.value))
        register_result(results, host, rca, ria, rma, qv.value, 0, &qv.sp, NULL, stats, register_zero, qv.duration_ut);
}

static void rrdset_weights_multi_dimensional_value(struct query_weights_data *qwd) {
    QUERY_TARGET_REQUEST qtr = {
            .version = 1,
            .scope_nodes = qwd->qwr->scope_nodes,
            .scope_contexts = qwd->qwr->scope_contexts,
            .scope_instances = qwd->qwr->scope_instances,
            .scope_labels = qwd->qwr->scope_labels,
            .scope_dimensions = qwd->qwr->scope_dimensions,
            .nodes = qwd->qwr->nodes,
            .contexts = qwd->qwr->contexts,
            .instances = qwd->qwr->instances,
            .dimensions = qwd->qwr->dimensions,
            .labels = qwd->qwr->labels,
            .alerts = qwd->qwr->alerts,
            .after = qwd->qwr->after,
            .before = qwd->qwr->before,
            .points = 1,
            .options = qwd->qwr->options | RRDR_OPTION_NATURAL_POINTS,
            .time_group_method = qwd->qwr->time_group_method,
            .time_group_options = qwd->qwr->time_group_options,
            .tier = qwd->qwr->tier,
            .timeout_ms = qwd->qwr->timeout_ms,
            .query_source = QUERY_SOURCE_API_WEIGHTS,
            .priority = STORAGE_PRIORITY_SYNCHRONOUS_FIRST,
    };

    ONEWAYALLOC *owa = onewayalloc_create(16 * 1024);
    QUERY_TARGET *qt = query_target_create(&qtr);
    stream_control_user_weights_query_started();
    RRDR *r = rrd2rrdr(owa, qt);
    stream_control_user_weights_query_finished();

    if(!r || rrdr_rows(r) != 1 || !r->d || r->d != r->internal.qt->query.used)
        goto cleanup;

    QUERY_VALUE qv = {
            .after = r->view.after,
            .before = r->view.before,
            .points_read = r->stats.db_points_read,
            .result_points = r->stats.result_points_generated,
    };

    size_t queries = 0;
    for(size_t d = 0; d < r->d ;d++) {
        qwd->examined_dimensions++;

        if(!rrdr_dimension_should_be_exposed(r->od[d], qwd->qwr->options))
            continue;

        long i = 0; // only one row
        NETDATA_DOUBLE *cn = &r->v[ i * r->d ];
        NETDATA_DOUBLE *ar = &r->ar[ i * r->d ];

        qv.value = cn[d];
        qv.anomaly_rate = ar[d];
        storage_point_merge_to(qv.sp, r->internal.qt->query.array[d].query_points);

        if(netdata_double_isnumber(qv.value)) {
            QUERY_METRIC *qm = query_metric(r->internal.qt, d);
            QUERY_DIMENSION *qd = query_dimension(r->internal.qt, qm->link.query_dimension_id);
            QUERY_INSTANCE *qi = query_instance(r->internal.qt, qm->link.query_instance_id);
            QUERY_CONTEXT *qc = query_context(r->internal.qt, qm->link.query_context_id);
            QUERY_NODE *qn = query_node(r->internal.qt, qm->link.query_node_id);

            register_result(qwd->results, qn->rrdhost, qc->rca, qi->ria, qd->rma, qv.value, 0,
                            &r->internal.qt->query.array[d].query_points, NULL,
                            &qwd->stats, qwd->register_zero, qm->duration_ut);
        }

        queries++;
    }

    merge_query_value_to_stats(&qv, &qwd->stats, queries);

cleanup:
    rrdr_free(owa, r);
    query_target_release(qt);
    onewayalloc_destroy(owa);
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
    size_t dimensions = dictionary_entries(results);
    if(!dimensions) return 0;

    if(stats->max_base_high_ratio == 0.0)
        stats->max_base_high_ratio = 1.0;

    // create an array of the right size and copy all the values in it
    NETDATA_DOUBLE slots[dimensions];
    dimensions = 0;
    dfe_start_read(results, t) {
        if(t->flags & RESULT_IS_PERCENTAGE_OF_TIME)
            t->value = t->value * stats->max_base_high_ratio;

        slots[dimensions++] = t->value;
    }
    dfe_done(t);

    if(!dimensions) return 0;   // Coverity fix

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
// MCP format output

// Comparator for sorting results by value (descending order - highest scores first)
static int registered_results_value_compare(const DICTIONARY_ITEM **item1, const DICTIONARY_ITEM **item2) {
    struct register_result *r1 = dictionary_acquired_item_value(*item1);
    struct register_result *r2 = dictionary_acquired_item_value(*item2);
    
    // Sort by value in descending order (highest first)
    if (r1->value < r2->value) return 1;
    if (r1->value > r2->value) return -1;
    return 0;
}

// Callback for sorted dictionary walkthrough
struct mcp_output_state {
    BUFFER *wb;
    WEIGHTS_METHOD method;
    size_t count;
    size_t limit;
};

static int registered_results_to_json_mcp_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data) {
    struct mcp_output_state *state = (struct mcp_output_state *)data;
    struct register_result *t = (struct register_result *)value;
    
    // Check if we've reached the cardinality limit
    if (state->count >= state->limit)
        return -1; // Stop iteration
        
    BUFFER *wb = state->wb;
    
    buffer_json_add_array_item_array(wb); // Start row array
    
    // Add score/value based on method
    switch(state->method) {
        case WEIGHTS_METHOD_MC_KS2:
        case WEIGHTS_METHOD_MC_VOLUME:
            buffer_json_add_array_item_double(wb, t->value);
            break;
            
        case WEIGHTS_METHOD_ANOMALY_RATE:
            // For anomaly rate, the value is already a percentage
            buffer_json_add_array_item_double(wb, t->value);
            break;
            
        case WEIGHTS_METHOD_VALUE:
            // For CV or other aggregations
            buffer_json_add_array_item_double(wb, t->value);
            break;
    }
    
    // Add the 5 statistical values
    // 1. Min
    if(storage_point_is_unset(t->highlighted) || storage_point_is_gap(t->highlighted))
        buffer_json_add_array_item_double(wb, NAN);
    else
        buffer_json_add_array_item_double(wb, t->highlighted.min);
    
    // 2. Max
    if(storage_point_is_unset(t->highlighted) || storage_point_is_gap(t->highlighted))
        buffer_json_add_array_item_double(wb, NAN);
    else
        buffer_json_add_array_item_double(wb, t->highlighted.max);
    
    // 3. Average
    if(storage_point_is_unset(t->highlighted) || storage_point_is_gap(t->highlighted) || t->highlighted.count == 0)
        buffer_json_add_array_item_double(wb, NAN);
    else
        buffer_json_add_array_item_double(wb, t->highlighted.sum / (NETDATA_DOUBLE)t->highlighted.count);
    
    // 4. Number of samples in window
    buffer_json_add_array_item_uint64(wb, t->highlighted.count);
    
    // 5. Number of anomalous samples in window
    buffer_json_add_array_item_double(wb, t->highlighted.anomaly_count);

    // Add metadata
    // Add node name
    buffer_json_add_array_item_string(wb, rrdhost_hostname(t->host));
    
    // Add context
    buffer_json_add_array_item_string(wb, rrdcontext_acquired_id(t->rca));
    
    // Add instance
    buffer_json_add_array_item_string(wb, rrdinstance_acquired_id(t->ria));
    
    // Add dimension
    buffer_json_add_array_item_string(wb, rrdmetric_acquired_name(t->rma));
    
    // Add labels (as object or null)
    RRDLABELS *labels = rrdinstance_acquired_labels(t->ria);
    if(labels && rrdlabels_entries(labels) > 0) {
        buffer_json_add_array_item_object(wb);
        rrdlabels_to_buffer_json_members(labels, wb);
        buffer_json_object_close(wb);
    }
    else {
        buffer_json_add_array_item_string(wb, NULL);
    }
    
    buffer_json_array_close(wb); // End row array
    
    state->count++;
    return 0; // Continue iteration
}

static size_t registered_results_to_json_mcp(
        DICTIONARY *results, BUFFER *wb,
        time_t after __maybe_unused, time_t before __maybe_unused,
        time_t baseline_after __maybe_unused, time_t baseline_before __maybe_unused,
        size_t points __maybe_unused, WEIGHTS_METHOD method,
        RRDR_TIME_GROUPING group __maybe_unused, RRDR_OPTIONS options, uint32_t shifts __maybe_unused,
        size_t examined_dimensions __maybe_unused, struct query_weights_data *qwd,
        WEIGHTS_STATS *stats __maybe_unused,
        struct query_versions *versions __maybe_unused) {
    
    buffer_json_initialize(wb, "\"", "\"", 0, true, (options & RRDR_OPTION_MINIFY) ? BUFFER_JSON_OPTIONS_MINIFY : BUFFER_JSON_OPTIONS_DEFAULT);
    
    // Add columns array based on method
    buffer_json_member_add_array(wb, "columns");
    
    switch(method) {
        case WEIGHTS_METHOD_MC_KS2:
            buffer_json_add_array_item_string(wb, "KS2 Score");
            break;

        case WEIGHTS_METHOD_MC_VOLUME:
            buffer_json_add_array_item_string(wb, "Volume Score");
            break;
            
        case WEIGHTS_METHOD_ANOMALY_RATE:
            buffer_json_add_array_item_string(wb, "Anomaly Rate");
            break;
            
        case WEIGHTS_METHOD_VALUE:
            buffer_json_add_array_item_string(wb, "Coefficient of Variation");
            break;
    }
    
    // Common statistical columns for all methods
    buffer_json_add_array_item_string(wb, "Minimum Sample Value");
    buffer_json_add_array_item_string(wb, "Maximum Sample Value");
    buffer_json_add_array_item_string(wb, "Average Sample Value");
    buffer_json_add_array_item_string(wb, "# of Samples in Window");
    buffer_json_add_array_item_string(wb, "# of Anomalous Samples in Window");

    // Metadata columns
    buffer_json_add_array_item_string(wb, "Hostname");
    buffer_json_add_array_item_string(wb, "Context / Metric Name");
    buffer_json_add_array_item_string(wb, "Metrics Instance");
    buffer_json_add_array_item_string(wb, "Dimension");
    buffer_json_add_array_item_string(wb, "Instance Labels");
    
    buffer_json_array_close(wb); // columns
    
    // Add results array
    buffer_json_member_add_array(wb, "results");
    
    // Get cardinality limit from query weights data
    size_t cardinality_limit = qwd && qwd->qwr ? qwd->qwr->cardinality_limit : 50;
    if (cardinality_limit < 30) cardinality_limit = 30;
    
    // Set up state for callback
    struct mcp_output_state state = {
        .wb = wb,
        .method = method,
        .count = 0,
        .limit = cardinality_limit
    };
    
    // Walk through dictionary in sorted order (by value descending)
    dictionary_sorted_walkthrough_rw(results, 'r', registered_results_to_json_mcp_callback, &state, registered_results_value_compare);
    
    buffer_json_array_close(wb); // results
    
    // Add metadata
    buffer_json_member_add_object(wb, "metadata");
    buffer_json_member_add_uint64(wb, "total_time_series_analyzed", examined_dimensions);
    buffer_json_member_add_uint64(wb, "total_time_series_returned", state.count);
    buffer_json_member_add_string(wb, "method", weights_method_to_string(method));
    if (state.count >= cardinality_limit) {
        buffer_json_member_add_uint64(wb, "cardinality_limit", cardinality_limit);
        buffer_json_member_add_boolean(wb, "truncated", true);
    }
    buffer_json_object_close(wb); // metadata
    
    buffer_json_finalize(wb);
    
    return state.count;
}

static ssize_t weights_count_for_rrdmetric(
    void *data,
    RRDHOST *host __maybe_unused,
    RRDCONTEXT_ACQUIRED *rca __maybe_unused,
    RRDINSTANCE_ACQUIRED *ria __maybe_unused,
    RRDMETRIC_ACQUIRED *rma __maybe_unused)
{
    struct query_weights_data *qwd = data;

    __atomic_fetch_add(&qwd->total_workload.metrics, 1, __ATOMIC_RELAXED);
    return 1;
}

// ----------------------------------------------------------------------------
// The main function

static ssize_t weights_for_rrdmetric(void *data, RRDHOST *host, RRDCONTEXT_ACQUIRED *rca, RRDINSTANCE_ACQUIRED *ria, RRDMETRIC_ACQUIRED *rma) {
    struct query_weights_data *qwd = data;
    QUERY_WEIGHTS_REQUEST *qwr = qwd->qwr;

    if(qwd->qwr->interrupt_callback && qwd->qwr->interrupt_callback(qwd->qwr->interrupt_callback_data)) {
        __atomic_store_n(&qwd->interrupted, true, __ATOMIC_RELAXED);
        return -1;
    }

    __atomic_fetch_add(&qwd->examined_dimensions, 1, __ATOMIC_RELAXED);

    switch(qwr->method) {
        case WEIGHTS_METHOD_VALUE:
            rrdset_weights_value(
                    host, rca, ria, rma,
                    qwd->results,
                    qwr->after, qwr->before,
                    qwr->options, qwr->time_group_method, qwr->time_group_options, qwr->tier,
                    &qwd->stats, qwd->register_zero
            );
            break;

        case WEIGHTS_METHOD_ANOMALY_RATE:
            qwr->options |= RRDR_OPTION_ANOMALY_BIT;
            rrdset_weights_value(
                    host, rca, ria, rma,
                    qwd->results,
                    qwr->after, qwr->before,
                    qwr->options, qwr->time_group_method, qwr->time_group_options, qwr->tier,
                    &qwd->stats, qwd->register_zero
            );
            break;

        case WEIGHTS_METHOD_MC_VOLUME:
            rrdset_metric_correlations_volume(
                    host, rca, ria, rma,
                    qwd->results,
                    qwr->baseline_after, qwr->baseline_before,
                    qwr->after, qwr->before,
                    qwr->options, qwr->time_group_method, qwr->time_group_options, qwr->tier,
                    &qwd->stats, qwd->register_zero
            );
            break;

        default:
        case WEIGHTS_METHOD_MC_KS2:
            rrdset_metric_correlations_ks2(
                    host, rca, ria, rma,
                    qwd->results,
                    qwr->baseline_after, qwr->baseline_before,
                    qwr->after, qwr->before, qwr->points,
                    qwr->options, qwr->time_group_method, qwr->time_group_options, qwr->tier, qwd->shifts,
                    &qwd->stats, qwd->register_zero
            );
            break;
    }

    qwd->timings.executed_ut = now_monotonic_usec();
    if(qwd->timings.executed_ut - qwd->timings.received_ut > qwd->timeout_us) {
        qwd->timed_out = true;
        return -1;
    }

    query_progress_done_step(qwr->transaction, 1);

    return 1;
}

static ssize_t weights_count_context_callback(void *data, RRDCONTEXT_ACQUIRED *rca, bool queryable_context) {
    if(!queryable_context)
        return false;

    struct query_weights_data *qwd = data;

    bool has_retention = false;
    switch(qwd->qwr->method) {
        case WEIGHTS_METHOD_VALUE:
        case WEIGHTS_METHOD_ANOMALY_RATE:
            has_retention = rrdcontext_retention_match(rca, qwd->qwr->after, qwd->qwr->before);
            break;

        case WEIGHTS_METHOD_MC_KS2:
        case WEIGHTS_METHOD_MC_VOLUME:
            has_retention = rrdcontext_retention_match(rca, qwd->qwr->after, qwd->qwr->before);
            if(has_retention)
                has_retention = rrdcontext_retention_match(rca, qwd->qwr->baseline_after, qwd->qwr->baseline_before);
            break;
    }

    if(!has_retention)
        return 0;

    __atomic_fetch_add(&qwd->total_workload.contexts, 1, __ATOMIC_RELAXED);
    ssize_t ret = weights_foreach_rrdmetric_in_context(rca,
                                            qwd->scope_instances_sp,
                                            qwd->scope_labels_pa,
                                            qwd->scope_dimensions_sp,
                                            qwd->instances_sp,
                                            NULL,
                                            qwd->labels_pa,
                                            qwd->alerts_sp,
                                            qwd->dimensions_sp,
                                            true, true, qwd->qwr->version,
                                            weights_count_for_rrdmetric, qwd);
    if (ret >= 1)
        return 1;
    else
        return 0;
}

static ssize_t weights_count_node_callback(void *data, RRDHOST *host, bool queryable) {
    if(!queryable)
        return 0;

    struct query_weights_data *qwd = data;
    if (qwd->total_hosts >= qwd->hosts_array_capacity) {
        qwd->hosts_array_capacity *= 2;
        qwd->hosts_array = reallocz(qwd->hosts_array, sizeof(RRDHOST *) * qwd->hosts_array_capacity);
    }
    qwd->hosts_array[qwd->total_hosts++] = host;

    __atomic_fetch_add(&qwd->total_workload.nodes, 1, __ATOMIC_RELAXED);
    ssize_t ret = query_scope_foreach_context(host, qwd->qwr->scope_contexts,
                                qwd->scope_contexts_sp, qwd->contexts_sp,
                                weights_count_context_callback, queryable, qwd);

    return ret;
}

static ssize_t weights_do_context_callback(void *data, RRDCONTEXT_ACQUIRED *rca, bool queryable_context) {
    if(!queryable_context)
        return false;

    struct query_weights_data *qwd = data;

    bool has_retention = false;
    switch(qwd->qwr->method) {
        case WEIGHTS_METHOD_VALUE:
        case WEIGHTS_METHOD_ANOMALY_RATE:
            has_retention = rrdcontext_retention_match(rca, qwd->qwr->after, qwd->qwr->before);
            break;

        case WEIGHTS_METHOD_MC_KS2:
        case WEIGHTS_METHOD_MC_VOLUME:
            has_retention = rrdcontext_retention_match(rca, qwd->qwr->after, qwd->qwr->before);
            if(has_retention)
                has_retention = rrdcontext_retention_match(rca, qwd->qwr->baseline_after, qwd->qwr->baseline_before);
            break;
    }

    if(!has_retention)
        return 0;

    ssize_t ret = weights_foreach_rrdmetric_in_context(rca,
                                            qwd->scope_instances_sp,
                                            qwd->scope_labels_pa,
                                            qwd->scope_dimensions_sp,
                                            qwd->instances_sp,
                                            NULL,
                                            qwd->labels_pa,
                                            qwd->alerts_sp,
                                            qwd->dimensions_sp,
                                            true, true, qwd->qwr->version,
                                            weights_for_rrdmetric, qwd);
    return ret;
}

// Parallel version of query_scope_foreach_host
static ssize_t query_scope_foreach_host_parallel(SIMPLE_PATTERN *scope_hosts_sp, SIMPLE_PATTERN *hosts_sp,
                                                  struct query_weights_data *qwd)
{
    size_t host_count = dictionary_entries(rrdhost_root_index);
    qwd->hosts_array = mallocz(sizeof(RRDHOST *) * host_count);
    qwd->hosts_array_capacity = host_count;
    qwd->total_hosts = 0;

    (void) query_scope_foreach_host(scope_hosts_sp, hosts_sp, weights_count_node_callback, qwd, &qwd->versions, NULL);

    size_t active_hosts = qwd->total_hosts;

    size_t num_threads = netdata_conf_cpus();
    if (num_threads < 1) num_threads = 1;

    // If we have fewer hosts than threads, reduce thread count
    if (active_hosts < num_threads) {
        num_threads = active_hosts;
    }

    if (num_threads <= 1 || active_hosts <= 1) {
        // Fall back to single-threaded processing
        freez(qwd->hosts_array);
        return query_scope_foreach_host(scope_hosts_sp, hosts_sp,
                                      weights_do_node_callback, qwd,
                                      &qwd->versions, NULL);
    }

    // Calculate hosts per thread
    size_t hosts_per_thread = active_hosts / num_threads;
    size_t remaining_hosts = active_hosts % num_threads;

    // Prepare thread data
    struct query_weights_thread_data *thread_data = mallocz(sizeof(struct query_weights_thread_data) * num_threads);
    ND_THREAD **threads = mallocz(sizeof(ND_THREAD *) * num_threads);

    size_t current_host_idx = 0;
    for (size_t i = 0; i < num_threads; i++) {
        thread_data[i].main_qwd = qwd;
        thread_data[i].local_results = register_result_init_single_threaded();
        thread_data[i].thread_id = i;
        thread_data[i].hosts = &qwd->hosts_array[current_host_idx];

        // Distribute hosts evenly, giving extra hosts to first threads
        thread_data[i].host_count = hosts_per_thread + (i < remaining_hosts ? 1 : 0);
        current_host_idx += thread_data[i].host_count;

        completion_init(&thread_data[i].completion);
        rrdeng_enq_cmd(NULL, RRDENG_OPCODE_PARALLEL_WEIGHT, &thread_data[i], &thread_data[i].completion, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);
    }

    // Wait for all threads to complete
    ssize_t total_added = 0;
    for (size_t i = 0; i < num_threads; i++) {
        completion_wait_for(&thread_data[i].completion);
        completion_destroy(&thread_data[i].completion);

        // Merge results from this thread
        merge_results_dictionaries(qwd->results, thread_data[i].local_results);
        merge_weights_stats(&qwd->stats, &thread_data[i].local_stats);

        // Accumulate examined dimensions
        __atomic_fetch_add(&qwd->examined_dimensions, thread_data[i].local_examined_dimensions, __ATOMIC_RELAXED);

        // Merge version hashes
        qwd->versions.contexts_hard_hash += thread_data[i].local_versions.contexts_hard_hash;
        qwd->versions.contexts_soft_hash += thread_data[i].local_versions.contexts_soft_hash;
        qwd->versions.alerts_hard_hash += thread_data[i].local_versions.alerts_hard_hash;
        qwd->versions.alerts_soft_hash += thread_data[i].local_versions.alerts_soft_hash;

        // Clean up thread data
        register_result_destroy(thread_data[i].local_results);
    }

    total_added = (ssize_t) dictionary_entries(qwd->results);

    // Cleanup
    freez(thread_data);
    freez(threads);
    freez(qwd->hosts_array);

    return total_added;
}

static ssize_t weights_do_node_callback(void *data, RRDHOST *host, bool queryable) {
    if(!queryable)
        return 0;

    struct query_weights_data *qwd = data;

    ssize_t ret = query_scope_foreach_context(host, qwd->qwr->scope_contexts,
                                qwd->scope_contexts_sp, qwd->contexts_sp,
                                weights_do_context_callback, queryable, qwd);

    return ret;
}

int web_api_v12_weights(BUFFER *wb, QUERY_WEIGHTS_REQUEST *qwr) {

    char *error = NULL;
    int resp = HTTP_RESP_OK;

    // if the user didn't give a timeout
    // assume 60 seconds
    if(!qwr->timeout_ms)
        qwr->timeout_ms = 5 * 60 * MSEC_PER_SEC;

    // if the timeout is less than 1 second
    // make it at least 1 second
    if(qwr->timeout_ms < (long)(1 * MSEC_PER_SEC))
        qwr->timeout_ms = 1 * MSEC_PER_SEC;

    struct query_weights_data qwd = {
            .qwr = qwr,

            .scope_nodes_sp = string_to_simple_pattern(qwr->scope_nodes),
            .scope_contexts_sp = string_to_simple_pattern(qwr->scope_contexts),
            .scope_instances_sp = string_to_simple_pattern(qwr->scope_instances),
            .scope_labels_sp = string_to_simple_pattern(qwr->scope_labels),
            .scope_dimensions_sp = string_to_simple_pattern(qwr->scope_dimensions),
            .nodes_sp = string_to_simple_pattern(qwr->nodes),
            .contexts_sp = string_to_simple_pattern(qwr->contexts),
            .instances_sp = string_to_simple_pattern(qwr->instances),
            .dimensions_sp = string_to_simple_pattern(qwr->dimensions),
            .labels_sp = string_to_simple_pattern(qwr->labels),
            .alerts_sp = string_to_simple_pattern(qwr->alerts),
            .scope_labels_pa = NULL,
            .labels_pa = NULL,
            .timeout_us = qwr->timeout_ms * USEC_PER_MS,
            .timed_out = false,
            .examined_dimensions = 0,
            .register_zero = true,
            .results = register_result_init(),
            .stats = {},
            .shifts = 0,
            .total_workload = {0}, // Initialize workload statistics
            .timings = {
                    .received_ut = now_monotonic_usec(),
            }
    };
    
    // Pre-compile pattern arrays for labels
    if(qwd.scope_labels_sp)
        qwd.scope_labels_pa = pattern_array_add_simple_pattern(NULL, qwd.scope_labels_sp, ':');
    if(qwd.labels_sp)
        qwd.labels_pa = pattern_array_add_simple_pattern(NULL, qwd.labels_sp, ':');

    if(!rrdr_relative_window_to_absolute_query(&qwr->after, &qwr->before, NULL, false))
        buffer_no_cacheable(wb);
    else
        buffer_cacheable(wb);

    if (qwr->before <= qwr->after) {
        resp = HTTP_RESP_BAD_REQUEST;
        error = "Invalid selected time-range.";
        goto cleanup;
    }

    if(qwr->method == WEIGHTS_METHOD_MC_KS2 || qwr->method == WEIGHTS_METHOD_MC_VOLUME) {
        if(!qwr->points) qwr->points = 500;

        if(qwr->baseline_before <= API_RELATIVE_TIME_MAX)
            qwr->baseline_before += qwr->after;

        rrdr_relative_window_to_absolute_query(&qwr->baseline_after, &qwr->baseline_before, NULL, false);

        if (qwr->baseline_before <= qwr->baseline_after) {
            resp = HTTP_RESP_BAD_REQUEST;
            error = "Invalid baseline time-range.";
            goto cleanup;
        }

        // baseline should be a power of two multiple of highlight
        long long base_delta = qwr->baseline_before - qwr->baseline_after;
        long long high_delta = qwr->before - qwr->after;
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
            qwd.shifts++;
            multiplier = multiplier >> 1;
        }

        // if the baseline size will not comply to MAX_POINTS
        // lower the window of the baseline
        while(qwd.shifts && (qwr->points << qwd.shifts) > MAX_POINTS)
            qwd.shifts--;

        // if the baseline size still does not comply to MAX_POINTS
        // lower the resolution of the highlight and the baseline
        while((qwr->points << qwd.shifts) > MAX_POINTS)
            qwr->points = qwr->points >> 1;

        if(qwr->points < 15) {
            resp = HTTP_RESP_BAD_REQUEST;
            error = "Too few points available, at least 15 are needed.";
            goto cleanup;
        }

        // adjust the baseline to be multiplier times bigger than the highlight
        qwr->baseline_after = qwr->baseline_before - (high_delta << qwd.shifts);
    }

    if(qwr->options & RRDR_OPTION_NONZERO) {
        qwd.register_zero = false;

        // remove it to run the queries without it
        qwr->options &= ~RRDR_OPTION_NONZERO;
    }

    if(qwr->host && qwr->version == 1)
        weights_do_node_callback(&qwd, qwr->host, true);
    else {
        if((qwd.qwr->method == WEIGHTS_METHOD_VALUE || qwd.qwr->method == WEIGHTS_METHOD_ANOMALY_RATE) && (qwd.contexts_sp || qwd.scope_contexts_sp)) {

            if(qwd.qwr->format == WEIGHTS_FORMAT_MCP && qwd.qwr->method == WEIGHTS_METHOD_ANOMALY_RATE)
                qwd.qwr->options |= RRDR_OPTION_ANOMALY_BIT;

            rrdset_weights_multi_dimensional_value(&qwd);
        }
        else {
            query_scope_foreach_host_parallel(qwd.scope_nodes_sp, qwd.nodes_sp, &qwd);
        }
    }

    if(!qwd.register_zero) {
        // put it back, to show it in the response
        qwr->options |= RRDR_OPTION_NONZERO;
    }

    if(__atomic_load_n(&qwd.timed_out, __ATOMIC_RELAXED)) {
        error = "timed out";
        resp = HTTP_RESP_GATEWAY_TIMEOUT;
        goto cleanup;
    }

    if(__atomic_load_n(&qwd.interrupted, __ATOMIC_RELAXED)) {
        error = "interrupted";
        resp = HTTP_RESP_CLIENT_CLOSED_REQUEST;
        goto cleanup;
    }

    if(!qwd.register_zero)
        qwr->options |= RRDR_OPTION_NONZERO;

    if(!(qwr->options & RRDR_OPTION_RETURN_RAW) &&
        qwr->method != WEIGHTS_METHOD_VALUE &&
        qwr->format != WEIGHTS_FORMAT_MCP)
        spread_results_evenly(qwd.results, &qwd.stats);

    usec_t ended_usec = qwd.timings.executed_ut = now_monotonic_usec();

    // generate the json output we need
    buffer_flush(wb);

    size_t added_dimensions = 0;
    switch(qwr->format) {
        case WEIGHTS_FORMAT_CHARTS:
            added_dimensions =
                    registered_results_to_json_charts(
                            qwd.results, wb,
                            qwr->after, qwr->before,
                            qwr->baseline_after, qwr->baseline_before,
                            qwr->points, qwr->method, qwr->time_group_method, qwr->options, qwd.shifts,
                            qwd.examined_dimensions,
                            ended_usec - qwd.timings.received_ut, &qwd.stats);
            break;

        case WEIGHTS_FORMAT_CONTEXTS:
            added_dimensions =
                    registered_results_to_json_contexts(
                            qwd.results, wb,
                            qwr->after, qwr->before,
                            qwr->baseline_after, qwr->baseline_before,
                            qwr->points, qwr->method, qwr->time_group_method, qwr->options, qwd.shifts,
                            qwd.examined_dimensions,
                            ended_usec - qwd.timings.received_ut, &qwd.stats);
            break;

        case WEIGHTS_FORMAT_MCP:
            added_dimensions =
                    registered_results_to_json_mcp(
                            qwd.results, wb,
                            qwr->after, qwr->before,
                            qwr->baseline_after, qwr->baseline_before,
                            qwr->points, qwr->method, qwr->time_group_method, qwr->options, qwd.shifts,
                            qwd.examined_dimensions,
                            &qwd, &qwd.stats, &qwd.versions);
            break;

        default:
        case WEIGHTS_FORMAT_MULTINODE:
            // we don't support these groupings in weights
            qwr->group_by.group_by &= ~(RRDR_GROUP_BY_LABEL|RRDR_GROUP_BY_SELECTED|RRDR_GROUP_BY_PERCENTAGE_OF_INSTANCE);
            if(qwr->group_by.group_by == RRDR_GROUP_BY_NONE) {
                added_dimensions =
                        registered_results_to_json_multinode_no_group_by(
                                qwd.results, wb,
                                qwr->after, qwr->before,
                                qwr->baseline_after, qwr->baseline_before,
                                qwr->points, qwr->method, qwr->time_group_method, qwr->options, qwd.shifts,
                                qwd.examined_dimensions,
                                &qwd, &qwd.stats, &qwd.versions);
            }
            else {
                added_dimensions =
                        registered_results_to_json_multinode_group_by(
                                qwd.results, wb,
                                qwr->after, qwr->before,
                                qwr->baseline_after, qwr->baseline_before,
                                qwr->points, qwr->method, qwr->time_group_method, qwr->options, qwd.shifts,
                                qwd.examined_dimensions,
                                &qwd, &qwd.stats, &qwd.versions);
            }
            break;
    }

    if(!added_dimensions && qwr->version < 2) {
        error = "no results produced.";
        resp = HTTP_RESP_NOT_FOUND;
    }

cleanup:
    simple_pattern_free(qwd.scope_nodes_sp);
    simple_pattern_free(qwd.scope_contexts_sp);
    simple_pattern_free(qwd.scope_instances_sp);
    simple_pattern_free(qwd.scope_labels_sp);
    simple_pattern_free(qwd.scope_dimensions_sp);
    simple_pattern_free(qwd.nodes_sp);
    simple_pattern_free(qwd.contexts_sp);
    simple_pattern_free(qwd.instances_sp);
    simple_pattern_free(qwd.dimensions_sp);
    simple_pattern_free(qwd.labels_sp);
    simple_pattern_free(qwd.alerts_sp);
    
    pattern_array_free(qwd.scope_labels_pa);
    pattern_array_free(qwd.labels_pa);

    register_result_destroy(qwd.results);

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
    snprintfz(buf, sizeof(buf) - 1, "%0.6f", v);
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


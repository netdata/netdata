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
        , { "value"        , WEIGHTS_METHOD_VALUE}
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
    RRDHOST *host;
    RRDCONTEXT_ACQUIRED *rca;
    RRDINSTANCE_ACQUIRED *ria;
    RRDMETRIC_ACQUIRED *rma;
    NETDATA_DOUBLE value;
    STORAGE_POINT highlighted;
    STORAGE_POINT baseline;
};

static DICTIONARY *register_result_init() {
    DICTIONARY *results = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct register_result));
    return results;
}

static void register_result_destroy(DICTIONARY *results) {
    dictionary_destroy(results);
}

static void register_result(DICTIONARY *results,
                            RRDHOST *host,
                            RRDCONTEXT_ACQUIRED *rca,
                            RRDINSTANCE_ACQUIRED *ria,
                            RRDMETRIC_ACQUIRED *rma,
                            NETDATA_DOUBLE value,
                            RESULT_FLAGS flags,
                            STORAGE_POINT *highlighted,
                            STORAGE_POINT *baseline,
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
        .host = host,
        .rca = rca,
        .ria = ria,
        .rma = rma,
        .value = v,
    };

    if(highlighted)
        t.highlighted = *highlighted;

    if(baseline)
        t.baseline = *baseline;

    // we can use the pointer address or RMA as a unique key for each metric
    char buf[20 + 1];
    ssize_t len = snprintfz(buf, 20, "%p", rma);
    dictionary_set_advanced(results, buf, len + 1, &t, sizeof(struct register_result), NULL);
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

    buffer_json_initialize(wb, "\"", "\"", 0, true, options & RRDR_OPTION_MINIFY);
    buffer_json_member_add_time_t(wb, "after", after);
    buffer_json_member_add_time_t(wb, "before", before);
    buffer_json_member_add_time_t(wb, "duration", before - after);
    buffer_json_member_add_uint64(wb, "points", points);

    if(method == WEIGHTS_METHOD_MC_KS2 || method == WEIGHTS_METHOD_MC_VOLUME) {
        buffer_json_member_add_time_t(wb, "baseline_after", baseline_after);
        buffer_json_member_add_time_t(wb, "baseline_before", baseline_before);
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
            for (size_t tier = 0; tier < storage_tiers; tier++)
                buffer_json_add_array_item_uint64(wb, stats->db_points_per_tier[tier]);
        }
        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);

    buffer_json_member_add_string(wb, "group", time_grouping_tostring(group));
    buffer_json_member_add_string(wb, "method", weights_method_to_string(method));
    web_client_api_request_v1_data_options_to_buffer_json_array(wb, "options", options);
}

static size_t registered_results_to_json_charts(DICTIONARY *results, BUFFER *wb,
                                                time_t after, time_t before,
                                                time_t baseline_after, time_t baseline_before,
                                                size_t points, WEIGHTS_METHOD method,
                                                RRDR_TIME_GROUPING group, RRDR_OPTIONS options, uint32_t shifts,
                                                size_t examined_dimensions, usec_t duration,
                                                WEIGHTS_STATS *stats) {

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

typedef enum {
    WPT_DIMENSION = 0,
    WPT_INSTANCE = 1,
    WPT_CONTEXT = 2,
    WPT_NODE = 3,
} WEIGHTS_POINT_TYPE;

static inline void storage_point_to_json(BUFFER *wb, WEIGHTS_POINT_TYPE type, ssize_t di, ssize_t ii, ssize_t ci, ssize_t ni, NETDATA_DOUBLE weight, STORAGE_POINT *highlighted_sp, STORAGE_POINT *baseline_sp, RRDR_OPTIONS options __maybe_unused, bool baseline) {
    buffer_json_add_array_item_array(wb);

    buffer_json_add_array_item_uint64(wb, type); // "type"
    buffer_json_add_array_item_array(wb);
    if(type == WPT_DIMENSION)
        buffer_json_add_array_item_int64(wb, di);
    if(type == WPT_DIMENSION || type == WPT_INSTANCE)
        buffer_json_add_array_item_int64(wb, ii);
    if(type == WPT_CONTEXT)
        buffer_json_add_array_item_int64(wb, ci);
    buffer_json_add_array_item_int64(wb, ni);
    buffer_json_array_close(wb);
    buffer_json_add_array_item_double(wb, weight); // "weight"

    buffer_json_add_array_item_array(wb);
    buffer_json_add_array_item_double(wb, highlighted_sp->min); // "min"
    buffer_json_add_array_item_double(wb, (highlighted_sp->count) ? highlighted_sp->sum / (NETDATA_DOUBLE) highlighted_sp->count : 0.0); // "avg"
    buffer_json_add_array_item_double(wb, highlighted_sp->max); // "max"
    buffer_json_add_array_item_double(wb, highlighted_sp->sum); // "sum"
    buffer_json_add_array_item_uint64(wb, highlighted_sp->count); // "count"
    buffer_json_add_array_item_uint64(wb, highlighted_sp->anomaly_count); // "anomaly_count"
    buffer_json_array_close(wb);

    if(baseline) {
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_double(wb, baseline_sp->min); // "min"
        buffer_json_add_array_item_double(wb, (baseline_sp->count) ? baseline_sp->sum / (NETDATA_DOUBLE) baseline_sp->count : 0.0); // "avg"
        buffer_json_add_array_item_double(wb, baseline_sp->max); // "max"
        buffer_json_add_array_item_double(wb, baseline_sp->sum); // "sum"
        buffer_json_add_array_item_uint64(wb, baseline_sp->count); // "count"
        buffer_json_add_array_item_uint64(wb, baseline_sp->anomaly_count); // "anomaly_count"
        buffer_json_array_close(wb);
    }

    buffer_json_array_close(wb); // key
}

static void multinode_data_schema(BUFFER *wb, RRDR_OPTIONS options __maybe_unused, const char *key, bool baseline) {
    size_t idx = 0;
    buffer_json_member_add_object(wb, key); // schema

    buffer_json_member_add_object(wb, "type");
    buffer_json_member_add_uint64(wb, "idx", idx++);
    buffer_json_object_close(wb); // type

    buffer_json_member_add_object(wb, "link");
    buffer_json_member_add_uint64(wb, "idx", idx++);
    buffer_json_member_add_object(wb, "dimension");
    {
        buffer_json_member_add_uint64(wb, "type", WPT_DIMENSION);
        size_t pidx = 0;
        buffer_json_member_add_uint64(wb, "di", pidx++);
        buffer_json_member_add_uint64(wb, "ii", pidx++);
        buffer_json_member_add_uint64(wb, "ni", pidx++);
    }
    buffer_json_object_close(wb); // dimension
    buffer_json_member_add_object(wb, "instance");
    {
        buffer_json_member_add_uint64(wb, "type", WPT_INSTANCE);
        size_t pidx = 0;
        buffer_json_member_add_uint64(wb, "ii", pidx++);
        buffer_json_member_add_uint64(wb, "ni", pidx++);
    }
    buffer_json_object_close(wb); // context
    buffer_json_member_add_object(wb, "context");
    {
        buffer_json_member_add_uint64(wb, "type", WPT_CONTEXT);
        size_t pidx = 0;
        buffer_json_member_add_uint64(wb, "ci", pidx++);
        buffer_json_member_add_uint64(wb, "ni", pidx++);
    }
    buffer_json_object_close(wb); // context
    buffer_json_member_add_object(wb, "node");
    {
        buffer_json_member_add_uint64(wb, "type", WPT_NODE);
        size_t pidx = 0;
        buffer_json_member_add_uint64(wb, "ni", pidx++);
    }
    buffer_json_object_close(wb); // node
    buffer_json_object_close(wb); // link

    buffer_json_member_add_object(wb, "weight");
    buffer_json_member_add_uint64(wb, "idx", idx++);
    buffer_json_object_close(wb); // weight

    for(size_t i = 0; i < ((baseline) ? 2 : 1) ; i++) {
        if(i == 0)
            buffer_json_member_add_object(wb, "highlighted");
        else
            buffer_json_member_add_object(wb, "baseline");

        buffer_json_member_add_uint64(wb, "idx", idx++);
        size_t pidx = 0;
        buffer_json_member_add_uint64(wb, "min", pidx++);
        buffer_json_member_add_uint64(wb, "avg", pidx++);
        buffer_json_member_add_uint64(wb, "max", pidx++);
        buffer_json_member_add_uint64(wb, "sum", pidx++);
        buffer_json_member_add_uint64(wb, "count", pidx++);
        buffer_json_member_add_uint64(wb, "anomaly_count", pidx++);
        buffer_json_object_close(wb); // point
    }

    buffer_json_object_close(wb); // schema
}

struct dict_unique_name {
    bool existing;
    uint32_t i;
};

struct dict_unique_id_name {
    bool existing;
    uint32_t i;
    const char *id;
    const char *name;
};

static inline ssize_t dict_unique_name_add(DICTIONARY *dict, const char *name, ssize_t *max_id) {
    struct dict_unique_name *dun = dictionary_set(dict, name, NULL, sizeof(struct dict_unique_name));
    if(!dun->existing) {
        dun->existing = true;
        dun->i = *max_id;
        (*max_id)++;
    }

    return (ssize_t)dun->i;
}

static inline ssize_t dict_unique_id_name_add(DICTIONARY *dict, const char *id, const char *name, ssize_t *max_id) {
    char key[1024 + 1];
    snprintfz(key, 1024, "%s:%s", id, name);
    struct dict_unique_id_name *dun = dictionary_set(dict, key, NULL, sizeof(struct dict_unique_id_name));
    if(!dun->existing) {
        dun->existing = true;
        dun->i = *max_id;
        (*max_id)++;
        dun->id = id;
        dun->name = name;
    }

    return (ssize_t)dun->i;
}

static size_t registered_results_to_json_multinode(DICTIONARY *results, BUFFER *wb,
                                                  time_t after, time_t before,
                                                  time_t baseline_after, time_t baseline_before,
                                                  size_t points, WEIGHTS_METHOD method,
                                                  RRDR_TIME_GROUPING group, RRDR_OPTIONS options, uint32_t shifts,
                                                  size_t examined_dimensions, usec_t duration,
                                                  WEIGHTS_STATS *stats,
                                                  struct query_versions *versions) {
    results_header_to_json(results, wb, after, before, baseline_after, baseline_before,
                           points, method, group, options, shifts, examined_dimensions, duration, stats);

    version_hashes_api_v2(wb, versions);

    bool baseline = method == WEIGHTS_METHOD_MC_KS2 || method == WEIGHTS_METHOD_MC_VOLUME;
    multinode_data_schema(wb, options, "schema", baseline);

    DICTIONARY *dict_nodes = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct dict_unique_name));
    DICTIONARY *dict_contexts = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct dict_unique_name));
    DICTIONARY *dict_instances = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct dict_unique_id_name));
    DICTIONARY *dict_dimensions = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct dict_unique_id_name));

    buffer_json_member_add_array(wb, "points");

    size_t total_dimensions = 0, node_dims = 0, context_dims = 0, instance_dims = 0;
    NETDATA_DOUBLE context_total_weight = 0.0, instance_total_weight = 0.0, node_total_weight = 0.0;
    STORAGE_POINT context_hsp = STORAGE_POINT_UNSET, instance_hsp = STORAGE_POINT_UNSET, node_hsp = STORAGE_POINT_UNSET;
    STORAGE_POINT context_bsp = STORAGE_POINT_UNSET, instance_bsp = STORAGE_POINT_UNSET, node_bsp = STORAGE_POINT_UNSET;
    struct register_result *t;
    RRDHOST *last_host = NULL;
    RRDCONTEXT_ACQUIRED *last_rca = NULL;
    RRDINSTANCE_ACQUIRED *last_ria = NULL;
    ssize_t di = -1, ii = -1, ci = -1, ni = -1;
    ssize_t di_max = 0, ii_max = 0, ci_max = 0, ni_max = 0;
    dfe_start_read(results, t) {

        // close instance
        if(t->ria != last_ria && last_ria) {
            storage_point_to_json(wb, WPT_INSTANCE, di, ii, ci, ni, instance_total_weight / (double) instance_dims, &instance_hsp, &instance_bsp, options, baseline);

            last_ria = NULL;
            instance_dims = 0;
            instance_total_weight = 0.0;
            instance_hsp = instance_bsp = STORAGE_POINT_UNSET;
        }

        // close context
        if(t->rca != last_rca && last_rca) {
            storage_point_to_json(wb, WPT_CONTEXT, di, ii, ci, ni, context_total_weight / (double) context_dims, &context_hsp, &instance_bsp, options, baseline);
            last_rca = NULL;
            context_dims = 0;
            context_total_weight = 0.0;
            context_hsp = context_bsp = STORAGE_POINT_UNSET;
        }

        // close node
        if(t->host != last_host && last_host) {
            storage_point_to_json(wb, WPT_NODE, di, ii, ci, ni, node_total_weight / (double) node_dims, &node_hsp, &node_bsp, options, baseline);
            last_host = NULL;
            node_dims = 0;
            node_total_weight = 0.0;
            node_hsp = node_bsp = STORAGE_POINT_UNSET;
        }

        // open node
        if(t->host != last_host) {
            last_host = t->host;
            ni = dict_unique_name_add(dict_nodes, t->host->machine_guid, &ni_max);
        }

        // open context
        if(t->rca != last_rca) {
            last_rca = t->rca;
            ci = dict_unique_name_add(dict_contexts, rrdcontext_acquired_id(t->rca), &ci_max);
        }

        // open instance
        if(t->ria != last_ria) {
            last_ria = t->ria;
            ii = dict_unique_id_name_add(dict_instances, rrdinstance_acquired_id(t->ria), rrdinstance_acquired_name(t->ria), &ii_max);
        }

        di = dict_unique_id_name_add(dict_dimensions, rrdmetric_acquired_id(t->rma), rrdmetric_acquired_name(t->rma), &di_max);
        storage_point_to_json(wb, WPT_DIMENSION, di, ii, ci, ni, t->value, &t->highlighted, &t->baseline, options, baseline);

        instance_total_weight += t->value;
        context_total_weight += t->value;
        node_total_weight += t->value;

        storage_point_merge_to(instance_hsp, t->highlighted);
        storage_point_merge_to(context_hsp, t->highlighted);
        storage_point_merge_to(node_hsp, t->highlighted);

        if(baseline) {
            storage_point_merge_to(instance_bsp, t->baseline);
            storage_point_merge_to(context_bsp, t->baseline);
            storage_point_merge_to(node_bsp, t->baseline);
        }

        instance_dims++;
        context_dims++;
        node_dims++;
        total_dimensions++;
    }
    dfe_done(t);

    // close instance
    if(last_ria)
        storage_point_to_json(wb, WPT_INSTANCE, di, ii, ci, ni, instance_total_weight / (double) instance_dims, &instance_hsp, &instance_bsp, options, baseline);

    // close context
    if(last_rca)
        storage_point_to_json(wb, WPT_CONTEXT, di, ii, ci, ni, context_total_weight / (double) context_dims, &context_hsp, &instance_bsp, options, baseline);

    // close node
    if(last_host)
        storage_point_to_json(wb, WPT_NODE, di, ii, ci, ni, node_total_weight / (double) node_dims, &node_hsp, &node_bsp, options, baseline);

    buffer_json_array_close(wb); // points

    buffer_json_member_add_array(wb, "nodes");
    {
        struct dict_unique_name *dun;
        dfe_start_read(dict_nodes, dun) {
                    buffer_json_add_array_item_object(wb);
                    buffer_json_member_add_string(wb, "mg", dun_dfe.name);
                    buffer_json_member_add_int64(wb, "ni", dun->i);
                    buffer_json_object_close(wb);
        }
        dfe_done(dun);
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_array(wb, "contexts");
    {
        struct dict_unique_name *dun;
        dfe_start_read(dict_contexts, dun) {
                    buffer_json_add_array_item_object(wb);
                    buffer_json_member_add_string(wb, "id", dun_dfe.name);
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

    buffer_json_member_add_uint64(wb, "correlated_dimensions", total_dimensions);
    buffer_json_member_add_uint64(wb, "total_dimensions_count", examined_dimensions);
    buffer_json_finalize(wb);

    dictionary_destroy(dict_nodes);
    dictionary_destroy(dict_contexts);
    dictionary_destroy(dict_instances);
    dictionary_destroy(dict_dimensions);

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
        error("Metric correlations: internal error - calculate_pairs_diff() returns the wrong number of entries");
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
            .priority = STORAGE_PRIORITY_SYNCHRONOUS,
    };

    RRDR *r = rrd2rrdr(owa, query_target_create(&qtr));
    if(!r)
        goto cleanup;

    stats->db_queries++;
    stats->result_points += r->stats.result_points_generated;
    stats->db_points += r->stats.db_points_read;
    for(size_t tr = 0; tr < storage_tiers ; tr++)
        stats->db_points_per_tier[tr] += r->internal.qt->db.tiers[tr].points;

    if(r->d != 1) {
        error("WEIGHTS: on query '%s' expected 1 dimension in RRDR but got %zu", r->internal.qt->id, r->d);
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
        *sp = r->drs[0];

    // copy the points of the dimension to a contiguous array
    // there is no need to check for empty values, since empty values are already zero
    // https://github.com/netdata/netdata/blob/6e3144683a73a2024d51425b20ecfd569034c858/web/api/queries/average/average.c#L41-L43
    memcpy(ret, r->v, rrdr_rows(r) * sizeof(NETDATA_DOUBLE));

cleanup:
    rrdr_free(owa, r);
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

    ONEWAYALLOC *owa = onewayalloc_create(16 * 1024);

    size_t high_points = 0;
    STORAGE_POINT highlighted_sp;
    NETDATA_DOUBLE *highlight = rrd2rrdr_ks2(
            owa, host, rca, ria, rma, after, before, points,
            options, time_group_method, time_group_options, tier, stats, &high_points, &highlighted_sp);

    if(!highlight)
        goto cleanup;

    size_t base_points = 0;
    STORAGE_POINT baseline_sp;
    NETDATA_DOUBLE *baseline = rrd2rrdr_ks2(
            owa, host, rca, ria, rma, baseline_after, baseline_before, high_points << shifts,
            options, time_group_method, time_group_options, tier, stats, &base_points, &baseline_sp);

    if(!baseline)
        goto cleanup;

    stats->binary_searches += 2 * (base_points - 1) + 2 * (high_points - 1);

    double prob = kstwo(baseline, (int)base_points, highlight, (int)high_points, shifts);
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
        // so, we flip the result of kstwo()
        register_result(results, host, rca, ria, rma, 1.0 - prob, RESULT_IS_BASE_HIGH_RATIO, &highlighted_sp, &baseline_sp, stats, register_zero);
    }

cleanup:
    onewayalloc_destroy(owa);
}

// ----------------------------------------------------------------------------
// VOLUME algorithm functions

static void merge_query_value_to_stats(QUERY_VALUE *qv, WEIGHTS_STATS *stats, size_t queries) {
    stats->db_queries += queries;
    stats->result_points += qv->result_points;
    stats->db_points += qv->points_read;
    for(size_t tier = 0; tier < storage_tiers ; tier++)
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
                                                   QUERY_SOURCE_API_WEIGHTS, STORAGE_PRIORITY_SYNCHRONOUS);
    merge_query_value_to_stats(&baseline_average, stats, 1);

    if(!netdata_double_isnumber(baseline_average.value)) {
        // this means no data for the baseline window, but we may have data for the highlighted one - assume zero
        baseline_average.value = 0.0;
    }

    QUERY_VALUE highlight_average = rrdmetric2value(host, rca, ria, rma, after, before,
                                                    options, time_group_method, time_group_options, tier, 0,
                                                    QUERY_SOURCE_API_WEIGHTS, STORAGE_PRIORITY_SYNCHRONOUS);
    merge_query_value_to_stats(&highlight_average, stats, 1);

    if(!netdata_double_isnumber(highlight_average.value))
        return;

    if(baseline_average.value == highlight_average.value) {
        // they are the same - let's move on
        return;
    }

    char highlight_countif_options[50 + 1];
    snprintfz(highlight_countif_options, 50, "%s" NETDATA_DOUBLE_FORMAT, highlight_average.value < baseline_average.value ? "<" : ">", baseline_average.value);
    QUERY_VALUE highlight_countif = rrdmetric2value(host, rca, ria, rma, after, before,
                                                    options, RRDR_GROUPING_COUNTIF, highlight_countif_options, tier, 0,
                                                    QUERY_SOURCE_API_WEIGHTS, STORAGE_PRIORITY_SYNCHRONOUS);
    merge_query_value_to_stats(&highlight_countif, stats, 1);

    if(!netdata_double_isnumber(highlight_countif.value)) {
        info("WEIGHTS: highlighted countif query failed, but highlighted average worked - strange...");
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

    register_result(results, host, rca, ria, rma, pcent, flags, &highlight_average.sp, &baseline_average.sp, stats, register_zero);
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
                                     QUERY_SOURCE_API_WEIGHTS, STORAGE_PRIORITY_SYNCHRONOUS);

    merge_query_value_to_stats(&qv, stats, 1);

    if(netdata_double_isnumber(qv.value))
        register_result(results, host, rca, ria, rma, qv.value, 0, &qv.sp, NULL, stats, register_zero);
}

struct query_weights_data {
    QUERY_WEIGHTS_REQUEST *qwr;

    SIMPLE_PATTERN *scope_nodes_sp;
    SIMPLE_PATTERN *scope_contexts_sp;
    SIMPLE_PATTERN *nodes_sp;
    SIMPLE_PATTERN *contexts_sp;
    SIMPLE_PATTERN *instances_sp;
    SIMPLE_PATTERN *dimensions_sp;
    SIMPLE_PATTERN *labels_sp;
    SIMPLE_PATTERN *alerts_sp;

    usec_t now_us;
    usec_t started_us;
    usec_t timeout_us;
    bool timed_out;
    bool interrupted;

    size_t examined_dimensions;
    bool register_zero;

    DICTIONARY *results;
    WEIGHTS_STATS stats;

    uint32_t shifts;

    struct query_versions versions;
};

static void rrdset_weights_multi_dimensional_value(struct query_weights_data *qwd) {
    QUERY_TARGET_REQUEST qtr = {
            .version = 1,
            .scope_nodes = qwd->qwr->scope_nodes,
            .scope_contexts = qwd->qwr->scope_contexts,
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
            .priority = STORAGE_PRIORITY_NORMAL,
    };

    ONEWAYALLOC *owa = onewayalloc_create(16 * 1024);
    RRDR *r = rrd2rrdr(owa, query_target_create(&qtr));

    if(rrdr_rows(r) != 1 || !r->d || r->d != r->internal.qt->query.used)
        goto cleanup;

    QUERY_VALUE qv = {
            .after = r->view.after,
            .before = r->view.before,
            .points_read = r->stats.db_points_read,
            .result_points = r->stats.result_points_generated,
    };

    size_t queries = 0;
    for(size_t d = 0; d < r->d ;d++) {
        if(!rrdr_dimension_should_be_exposed(r->od[d], qwd->qwr->options))
            continue;

        long i = 0; // only one row
        NETDATA_DOUBLE *cn = &r->v[ i * r->d ];
        NETDATA_DOUBLE *ar = &r->ar[ i * r->d ];

        qv.value = cn[d];
        qv.anomaly_rate = ar[d];
        qv.sp = *r->drs;

        if(netdata_double_isnumber(qv.value)) {
            QUERY_METRIC *qm = query_metric(r->internal.qt, d);
            QUERY_DIMENSION *qd = query_dimension(r->internal.qt, qm->link.query_dimension_id);
            QUERY_INSTANCE *qi = query_instance(r->internal.qt, qm->link.query_instance_id);
            QUERY_CONTEXT *qc = query_context(r->internal.qt, qm->link.query_context_id);
            QUERY_NODE *qn = query_node(r->internal.qt, qm->link.query_node_id);

            register_result(qwd->results, qn->rrdhost, qc->rca, qi->ria, qd->rma, qv.value, 0, &qv.sp,
                            NULL, &qwd->stats, qwd->register_zero);
        }

        queries++;
    }

    merge_query_value_to_stats(&qv, &qwd->stats, queries);

cleanup:
    rrdr_free(owa, r);
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
        if(t->flags & (RESULT_IS_PERCENTAGE_OF_TIME))
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
// The main function

static ssize_t weights_for_rrdmetric(void *data, RRDHOST *host, RRDCONTEXT_ACQUIRED *rca, RRDINSTANCE_ACQUIRED *ria, RRDMETRIC_ACQUIRED *rma) {
    struct query_weights_data *qwd = data;
    QUERY_WEIGHTS_REQUEST *qwr = qwd->qwr;

    qwd->now_us = now_realtime_usec();
    if(qwd->now_us - qwd->started_us > qwd->timeout_us) {
        qwd->timed_out = true;
        return -1;
    }

    if(qwd->qwr->interrupt_callback && qwd->qwr->interrupt_callback(qwd->qwr->interrupt_callback_data)) {
        qwd->interrupted = true;
        return -1;
    }

    qwd->examined_dimensions++;

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

    return 1;
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
                                            qwd->instances_sp,
                                            NULL,
                                            qwd->labels_sp,
                                            qwd->alerts_sp,
                                            qwd->dimensions_sp,
                                            true, true, qwd->qwr->version,
                                            weights_for_rrdmetric, qwd);
    return ret;
}

ssize_t weights_do_node_callback(void *data, RRDHOST *host, bool queryable) {
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
            .nodes_sp = string_to_simple_pattern(qwr->nodes),
            .contexts_sp = string_to_simple_pattern(qwr->contexts),
            .instances_sp = string_to_simple_pattern(qwr->instances),
            .dimensions_sp = string_to_simple_pattern(qwr->dimensions),
            .labels_sp = string_to_simple_pattern(qwr->labels),
            .alerts_sp = string_to_simple_pattern(qwr->alerts),
            .timeout_us = qwr->timeout_ms * USEC_PER_MS,
            .started_us = now_realtime_usec(),
            .timed_out = false,
            .examined_dimensions = 0,
            .register_zero = true,
            .results = register_result_init(),
            .stats = {},
            .shifts = 0,
    };

    if(!rrdr_relative_window_to_absolute(&qwr->after, &qwr->before, NULL))
        buffer_no_cacheable(wb);

    if (qwr->before <= qwr->after) {
        resp = HTTP_RESP_BAD_REQUEST;
        error = "Invalid selected time-range.";
        goto cleanup;
    }

    if(qwr->method == WEIGHTS_METHOD_MC_KS2 || qwr->method == WEIGHTS_METHOD_MC_VOLUME) {
        if(!qwr->points) qwr->points = 500;

        if(qwr->baseline_before <= API_RELATIVE_TIME_MAX)
            qwr->baseline_before += qwr->after;

        rrdr_relative_window_to_absolute(&qwr->baseline_after, &qwr->baseline_before, NULL);

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
            rrdset_weights_multi_dimensional_value(&qwd);
        }
        else {
            query_scope_foreach_host(qwd.scope_nodes_sp, qwd.nodes_sp,
                                     weights_do_node_callback, &qwd,
                                     &qwd.versions,
                                     NULL);
        }
    }

    if(!qwd.register_zero) {
        // put it back, to show it in the response
        qwr->options |= RRDR_OPTION_NONZERO;
    }

    if(qwd.timed_out) {
        error = "timed out";
        resp = HTTP_RESP_GATEWAY_TIMEOUT;
        goto cleanup;
    }

    if(qwd.interrupted) {
        error = "interrupted";
        resp = HTTP_RESP_BACKEND_FETCH_FAILED;
        goto cleanup;
    }

    if(!qwd.register_zero)
        qwr->options |= RRDR_OPTION_NONZERO;

    if(!(qwr->options & RRDR_OPTION_RETURN_RAW) && qwr->method != WEIGHTS_METHOD_VALUE)
        spread_results_evenly(qwd.results, &qwd.stats);

    usec_t ended_usec = now_realtime_usec();

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
                            ended_usec - qwd.started_us, &qwd.stats);
            break;

        case WEIGHTS_FORMAT_CONTEXTS:
            added_dimensions =
                    registered_results_to_json_contexts(
                            qwd.results, wb,
                            qwr->after, qwr->before,
                            qwr->baseline_after, qwr->baseline_before,
                            qwr->points, qwr->method, qwr->time_group_method, qwr->options, qwd.shifts,
                            qwd.examined_dimensions,
                            ended_usec - qwd.started_us, &qwd.stats);
            break;

        default:
        case WEIGHTS_FORMAT_MULTINODE:
            added_dimensions =
                    registered_results_to_json_multinode(
                            qwd.results, wb,
                            qwr->after, qwr->before,
                            qwr->baseline_after, qwr->baseline_before,
                            qwr->points, qwr->method, qwr->time_group_method, qwr->options, qwd.shifts,
                            qwd.examined_dimensions,
                            ended_usec - qwd.started_us, &qwd.stats, &qwd.versions);
            break;
    }

    if(!added_dimensions) {
        error = "no results produced.";
        resp = HTTP_RESP_NOT_FOUND;
    }

cleanup:
    simple_pattern_free(qwd.scope_nodes_sp);
    simple_pattern_free(qwd.scope_contexts_sp);
    simple_pattern_free(qwd.nodes_sp);
    simple_pattern_free(qwd.contexts_sp);
    simple_pattern_free(qwd.instances_sp);
    simple_pattern_free(qwd.dimensions_sp);
    simple_pattern_free(qwd.labels_sp);
    simple_pattern_free(qwd.alerts_sp);

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


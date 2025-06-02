// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_QUERY_INTERNAL_H
#define NETDATA_QUERY_INTERNAL_H

#include "query.h"
#include "web/api/formatters/rrd2json.h"
#include "rrdr.h"

#define QUERY_PLAN_MIN_POINTS 10
#define POINTS_TO_EXPAND_QUERY 5

typedef struct query_point {
    STORAGE_POINT sp;
    NETDATA_DOUBLE value;
    bool added;
#ifdef NETDATA_INTERNAL_CHECKS
    size_t id;
#endif
} QUERY_POINT;

#ifdef NETDATA_INTERNAL_CHECKS
#define QUERY_POINT_EMPTY (QUERY_POINT){ \
    .sp = STORAGE_POINT_UNSET, \
    .value = NAN, \
    .added = false, \
    .id = 0, \
}
#else
#define QUERY_POINT_EMPTY (QUERY_POINT){ \
    .sp = STORAGE_POINT_UNSET, \
    .value = NAN, \
    .added = false, \
}
#endif

#ifdef NETDATA_INTERNAL_CHECKS
#define query_point_set_id(point, point_id) (point).id = point_id
#else
#define query_point_set_id(point, point_id) debug_dummy()
#endif

typedef struct query_engine_ops {
    // configuration
    RRDR *r;
    QUERY_METRIC *qm;
    time_t view_update_every;
    time_t query_granularity;
    TIER_QUERY_FETCH tier_query_fetch;

    // query planer
    size_t current_plan;
    time_t current_plan_expire_time;
    time_t plan_expanded_after;
    time_t plan_expanded_before;

    // storage queries
    size_t tier;
    struct query_metric_tier *tier_ptr;
    struct storage_engine_query_handle *seqh;

    // aggregating points over time
    size_t group_points_non_zero;
    size_t group_points_added;
    STORAGE_POINT group_point;          // aggregates min, max, sum, count, anomaly count for each group point
    STORAGE_POINT query_point;          // aggregates min, max, sum, count, anomaly count across the whole query
    RRDR_VALUE_FLAGS group_value_flags;

    // statistics
    size_t db_total_points_read;
    size_t db_points_read_per_tier[RRD_STORAGE_TIERS];

    struct {
        time_t expanded_after;
        time_t expanded_before;
        struct storage_engine_query_handle handle;
        bool initialized;
        bool finalized;
    } plans[QUERY_PLANS_MAX];

    struct query_engine_ops *next;
} QUERY_ENGINE_OPS;

// query planner
#define query_plan_should_switch_plan(ops, now) ((now) >= (ops)->current_plan_expire_time)
bool query_planer_next_plan(QUERY_ENGINE_OPS *ops, time_t now, time_t last_point_end_time);
void query_planer_finalize_remaining_plans(QUERY_ENGINE_OPS *ops);
QUERY_ENGINE_OPS *rrd2rrdr_query_ops_prep(RRDR *r, size_t query_metric_id);
void rrd2rrdr_query_ops_release(QUERY_ENGINE_OPS *ops);
time_t rrdset_find_natural_update_every_for_timeframe(QUERY_TARGET *qt, time_t after_wanted, time_t before_wanted, size_t points_wanted, RRDR_OPTIONS options, size_t tier);
void rrd2rrdr_query_ops_freeall(RRDR *r);

// time aggregation
void time_grouping_add(RRDR *r, NETDATA_DOUBLE value, const RRDR_TIME_GROUPING add_flush);
NETDATA_DOUBLE time_grouping_flush(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr, const RRDR_TIME_GROUPING add_flush);
void rrdr_set_grouping_function(RRDR *r, RRDR_TIME_GROUPING group_method);

// group by
RRDR *rrd2rrdr_group_by_initialize(ONEWAYALLOC *owa, QUERY_TARGET *qt);
void rrdr2rrdr_group_by_calculate_percentage_of_group(RRDR *r);
void rrdr2rrdr_group_by_partial_trimming(RRDR *r);
void rrd2rrdr_group_by_add_metric(RRDR *r_dst, size_t d_dst, RRDR *r_tmp, size_t d_tmp,
                                  RRDR_GROUP_BY_FUNCTION group_by_aggregate_function,
                                  STORAGE_POINT *query_points, size_t pass);
void rrd2rrdr_convert_values_to_percentage_of_total(RRDR *r);
RRDR *rrd2rrdr_group_by_finalize(RRDR *r_tmp);
RRDR *rrd2rrdr_cardinality_limit(RRDR *r);

#endif //NETDATA_QUERY_INTERNAL_H

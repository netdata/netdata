// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_JSONWRAP_INTERNAL_H
#define NETDATA_JSONWRAP_INTERNAL_H

#include "jsonwrap.h"

struct summary_total_counts {
    size_t selected;
    size_t excluded;
    size_t queried;
    size_t failed;
};

// visualizers
void query_target_total_counts(BUFFER *wb, const char *key, struct summary_total_counts *totals);
void query_target_metric_counts(BUFFER *wb, QUERY_METRICS_COUNTS *metrics);
void query_target_instance_counts(BUFFER *wb, QUERY_INSTANCES_COUNTS *instances);
void query_target_alerts_counts(BUFFER *wb, QUERY_ALERTS_COUNTS *alerts, const char *name, bool array);
void query_target_points_statistics(BUFFER *wb, QUERY_TARGET *qt, STORAGE_POINT *sp);

// aggregators
void aggregate_metrics_counts(QUERY_METRICS_COUNTS *dst, const QUERY_METRICS_COUNTS *src);
void aggregate_instances_counts(QUERY_INSTANCES_COUNTS *dst, const QUERY_INSTANCES_COUNTS *src);
void aggregate_alerts_counts(QUERY_ALERTS_COUNTS *dst, const QUERY_ALERTS_COUNTS *src);
void aggregate_into_summary_totals(struct summary_total_counts *totals, QUERY_METRICS_COUNTS *metrics);


// common library calls
size_t rrdr_dimension_names(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options);
size_t rrdr_dimension_ids(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options);
void query_target_functions(BUFFER *wb, const char *key, RRDR *r);

// summary formatters
void query_target_summary_labels_v12(BUFFER *wb, QUERY_TARGET *qt, const char *key, bool v2, struct summary_total_counts *key_totals, struct summary_total_counts *value_totals);
void query_target_summary_nodes_v2(BUFFER *wb, QUERY_TARGET *qt, const char *key, struct summary_total_counts *totals);
void query_target_summary_instances_v2(BUFFER *wb, QUERY_TARGET *qt, const char *key, struct summary_total_counts *totals);
void query_target_summary_dimensions_v12(BUFFER *wb, QUERY_TARGET *qt, const char *key, bool v2, struct summary_total_counts *totals);
size_t query_target_summary_contexts_v2(BUFFER *wb, QUERY_TARGET *qt, const char *key, struct summary_total_counts *totals);
void query_target_summary_alerts_v2(BUFFER *wb, QUERY_TARGET *qt, const char *key);

void query_target_summary_instances_v1(BUFFER *wb, QUERY_TARGET *qt, const char *key);

void jsonwrap_query_plan(RRDR *r, BUFFER *wb);
void jsonwrap_query_metric_plan(BUFFER *wb, QUERY_METRIC *qm);

void query_target_detailed_objects_tree(BUFFER *wb, RRDR *r, RRDR_OPTIONS options);

#endif //NETDATA_JSONWRAP_INTERNAL_H

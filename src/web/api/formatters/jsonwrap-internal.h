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

// JSON key names struct for short vs long key support
struct jsonwrap_key_names {
    // Status and statistics
    const char *selected;           // "sl" / "selected"
    const char *excluded;           // "ex" / "excluded"
    const char *queried;            // "qr" / "queried"
    const char *failed;             // "fl" / "failed"
    
    // Object types
    const char *dimensions;         // "ds" / "dimensions"
    const char *instances;          // "is" / "instances"
    const char *alerts;             // "al" / "alerts"
    const char *statistics;         // "sts" / "statistics"
    
    // Common fields
    const char *name;               // "nm" / "name"
    const char *hostname;           // "nm" / "hostname"
    const char *id;                 // "id" / "id"
    const char *node_id;            // "nd" / "node_id"
    const char *time;               // "time" / "time"
    const char *value;              // "vl" / "value"
    const char *label_values;       // "lv" / "label_values"
    const char *machine_guid;       // "mg" / "machine_guid"
    const char *agent_index;        // "ai" / "agent_index"
    
    // Alert levels
    const char *clear;              // "cl" / "clear"
    const char *warning;            // "wr" / "warning"
    const char *critical;           // "cr" / "critical"
    const char *other;              // "ot" / "other"
    
    // Statistics fields
    const char *min;                // "min" / "min"
    const char *max;                // "max" / "max"
    const char *avg;                // "avg" / "average"
    const char *sum;                // "sum" / "sum"
    const char *count;              // "cnt" / "count"
    const char *volume;             // "vol" / "volume"
    const char *anomaly_rate;       // "arp" / "anomaly_rate"
    const char *anomaly_count;      // "arc" / "anomaly_count"
    const char *contribution;       // "con" / "contribution"
    const char *point_annotations;  // "pa" / "point_annotations"
    const char *point_schema;       // "point" / "point_schema"
    
    // Other fields
    const char *priority;           // "pri" / "priority"
    const char *update_every;       // "ue" / "update_every"
    const char *view;               // "view" / "view"
    const char *tier;               // "tier" / "tier"
    const char *tr;                 // "tr" / "tier"
    const char *after;              // "af" / "after"
    const char *before;             // "bf" / "before"
    const char *status;             // "st" / "status"
    const char *api;                // "api" / "api"
    const char *database;           // "db" / "database"
    const char *first_entry;        // "fe" / "first_entry"
    const char *last_entry;         // "le" / "last_entry"
    const char *node_index;         // "ni" / "node_index"
    const char *units;              // "un" / "units"
    const char *weight;             // "wg" / "weight"
    const char *as;                 // "as" / "as"
    const char *ids;                // "ids" / "ids"
    const char *info;               // "info" / "info"
};

// Thread-local pointer to the current key names
extern __thread const struct jsonwrap_key_names *jsonwrap_keys;

// Macro for clean key access
#define JSKEY(member) (jsonwrap_keys->member)

// Key name initialization
void jsonwrap_keys_init(RRDR_OPTIONS options);
void jsonwrap_keys_reset(void);

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
void query_target_info_nodes(BUFFER *wb, const char *key, RRDR_OPTIONS options);

// summary formatters
void query_target_summary_labels_v12(BUFFER *wb, QUERY_TARGET *qt, const char *key, bool v2, struct summary_total_counts *key_totals, struct summary_total_counts *value_totals);
void query_target_summary_nodes_v2(BUFFER *wb, QUERY_TARGET *qt, const char *key, struct summary_total_counts *totals);
void query_target_summary_instances_v2(BUFFER *wb, QUERY_TARGET *qt, const char *key, struct summary_total_counts *totals);
void query_target_summary_dimensions_v12(BUFFER *wb, QUERY_TARGET *qt, const char *key, bool v2, struct summary_total_counts *totals);
size_t query_target_summary_contexts_v2(BUFFER *wb, QUERY_TARGET *qt, const char *key, struct summary_total_counts *totals);
void query_target_summary_alerts_v2(BUFFER *wb, QUERY_TARGET *qt, const char *key);

void query_target_summary_instances_v1(BUFFER *wb, QUERY_TARGET *qt, const char *key);

void jsonwrap_query_plan(RRDR *r, BUFFER *wb, RRDR_OPTIONS options);
void jsonwrap_query_metric_plan(BUFFER *wb, QUERY_METRIC *qm, RRDR_OPTIONS options);

void query_target_detailed_objects_tree(BUFFER *wb, RRDR *r, RRDR_OPTIONS options);

#endif //NETDATA_JSONWRAP_INTERNAL_H

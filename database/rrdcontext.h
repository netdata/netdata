// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDCONTEXT_H
#define NETDATA_RRDCONTEXT_H 1

// ----------------------------------------------------------------------------
// RRDMETRIC

typedef struct rrdmetric_acquired RRDMETRIC_ACQUIRED;

// ----------------------------------------------------------------------------
// RRDINSTANCE

typedef struct rrdinstance_acquired RRDINSTANCE_ACQUIRED;

// ----------------------------------------------------------------------------
// RRDCONTEXT

typedef struct rrdcontexts_dictionary RRDCONTEXTS;
typedef struct rrdcontext_acquired RRDCONTEXT_ACQUIRED;

// ----------------------------------------------------------------------------

#include "rrd.h"

const char *rrdmetric_acquired_id(RRDMETRIC_ACQUIRED *rma);
const char *rrdmetric_acquired_name(RRDMETRIC_ACQUIRED *rma);
NETDATA_DOUBLE rrdmetric_acquired_last_stored_value(RRDMETRIC_ACQUIRED *rma);

const char *rrdinstance_acquired_id(RRDINSTANCE_ACQUIRED *ria);
const char *rrdinstance_acquired_name(RRDINSTANCE_ACQUIRED *ria);
DICTIONARY *rrdinstance_acquired_labels(RRDINSTANCE_ACQUIRED *ria);
DICTIONARY *rrdinstance_acquired_functions(RRDINSTANCE_ACQUIRED *ria);

// ----------------------------------------------------------------------------
// public API for rrdhost

void rrdhost_load_rrdcontext_data(RRDHOST *host);
void rrdhost_create_rrdcontexts(RRDHOST *host);
void rrdhost_destroy_rrdcontexts(RRDHOST *host);

void rrdcontext_host_child_connected(RRDHOST *host);
void rrdcontext_host_child_disconnected(RRDHOST *host);

int rrdcontext_foreach_instance_with_rrdset_in_context(RRDHOST *host, const char *context, int (*callback)(RRDSET *st, void *data), void *data);

typedef enum {
    RRDCONTEXT_OPTION_NONE               = 0,
    RRDCONTEXT_OPTION_SHOW_METRICS       = (1 << 0),
    RRDCONTEXT_OPTION_SHOW_INSTANCES     = (1 << 1),
    RRDCONTEXT_OPTION_SHOW_LABELS        = (1 << 2),
    RRDCONTEXT_OPTION_SHOW_QUEUED        = (1 << 3),
    RRDCONTEXT_OPTION_SHOW_FLAGS         = (1 << 4),
    RRDCONTEXT_OPTION_SHOW_DELETED       = (1 << 5),
    RRDCONTEXT_OPTION_DEEPSCAN           = (1 << 6),
    RRDCONTEXT_OPTION_SHOW_UUIDS         = (1 << 7),
    RRDCONTEXT_OPTION_SHOW_HIDDEN        = (1 << 8),
    RRDCONTEXT_OPTION_SKIP_ID            = (1 << 31), // internal use
} RRDCONTEXT_TO_JSON_OPTIONS;

#define RRDCONTEXT_OPTIONS_ALL (RRDCONTEXT_OPTION_SHOW_METRICS|RRDCONTEXT_OPTION_SHOW_INSTANCES|RRDCONTEXT_OPTION_SHOW_LABELS|RRDCONTEXT_OPTION_SHOW_QUEUED|RRDCONTEXT_OPTION_SHOW_FLAGS|RRDCONTEXT_OPTION_SHOW_DELETED|RRDCONTEXT_OPTION_SHOW_UUIDS|RRDCONTEXT_OPTION_SHOW_HIDDEN)

int rrdcontext_to_json(RRDHOST *host, BUFFER *wb, time_t after, time_t before, RRDCONTEXT_TO_JSON_OPTIONS options, const char *context, SIMPLE_PATTERN *chart_label_key, SIMPLE_PATTERN *chart_labels_filter, SIMPLE_PATTERN *chart_dimensions);
int rrdcontexts_to_json(RRDHOST *host, BUFFER *wb, time_t after, time_t before, RRDCONTEXT_TO_JSON_OPTIONS options, SIMPLE_PATTERN *chart_label_key, SIMPLE_PATTERN *chart_labels_filter, SIMPLE_PATTERN *chart_dimensions);

// ----------------------------------------------------------------------------
// public API for rrdcontexts

const char *rrdcontext_acquired_id(RRDCONTEXT_ACQUIRED *rca);

// ----------------------------------------------------------------------------
// public API for rrddims

void rrdcontext_updated_rrddim(RRDDIM *rd);
void rrdcontext_removed_rrddim(RRDDIM *rd);
void rrdcontext_updated_rrddim_algorithm(RRDDIM *rd);
void rrdcontext_updated_rrddim_multiplier(RRDDIM *rd);
void rrdcontext_updated_rrddim_divisor(RRDDIM *rd);
void rrdcontext_updated_rrddim_flags(RRDDIM *rd);
void rrdcontext_collected_rrddim(RRDDIM *rd);
int rrdcontext_find_dimension_uuid(RRDSET *st, const char *id, uuid_t *store_uuid);

// ----------------------------------------------------------------------------
// public API for rrdsets

void rrdcontext_updated_rrdset(RRDSET *st);
void rrdcontext_removed_rrdset(RRDSET *st);
void rrdcontext_updated_rrdset_name(RRDSET *st);
void rrdcontext_updated_rrdset_flags(RRDSET *st);
void rrdcontext_updated_retention_rrdset(RRDSET *st);
void rrdcontext_collected_rrdset(RRDSET *st);
int rrdcontext_find_chart_uuid(RRDSET *st, uuid_t *store_uuid);

// ----------------------------------------------------------------------------
// public API for ACLK

void rrdcontext_hub_checkpoint_command(void *cmd);
void rrdcontext_hub_stop_streaming_command(void *cmd);


// ----------------------------------------------------------------------------
// public API for threads

void rrdcontext_db_rotation(void);
void *rrdcontext_main(void *);

// ----------------------------------------------------------------------------
// public API for weights

struct metric_entry {
    RRDCONTEXT_ACQUIRED *rca;
    RRDINSTANCE_ACQUIRED *ria;
    RRDMETRIC_ACQUIRED *rma;
};

DICTIONARY *rrdcontext_all_metrics_to_dict(RRDHOST *host, SIMPLE_PATTERN *contexts);

// ----------------------------------------------------------------------------
// public API for queries

typedef struct query_plan_entry {
    size_t tier;
    time_t after;
    time_t before;
    time_t expanded_after;
    time_t expanded_before;
    struct storage_engine_query_handle handle;
    STORAGE_POINT (*next_metric)(struct storage_engine_query_handle *handle);
    int (*is_finished)(struct storage_engine_query_handle *handle);
    void (*finalize)(struct storage_engine_query_handle *handle);
    bool initialized;
    bool finalized;
} QUERY_PLAN_ENTRY;

#define QUERY_PLANS_MAX (RRD_STORAGE_TIERS * 2)

typedef struct query_metric {
    struct query_metric_tier {
        struct storage_engine *eng;
        STORAGE_METRIC_HANDLE *db_metric_handle;
        time_t db_first_time_s;         // the oldest timestamp available for this tier
        time_t db_last_time_s;          // the latest timestamp available for this tier
        time_t db_update_every_s;       // latest update every for this tier
        long weight;
    } tiers[RRD_STORAGE_TIERS];

    struct {
        size_t used;
        QUERY_PLAN_ENTRY array[QUERY_PLANS_MAX];
    } plan;

    struct {
        RRDHOST *host;
        RRDCONTEXT_ACQUIRED *rca;
        RRDINSTANCE_ACQUIRED *ria;
        RRDMETRIC_ACQUIRED *rma;
    } link;

    struct {
        STRING *id;
        STRING *name;
        RRDR_DIMENSION_FLAGS options;
    } dimension;

    struct {
        STRING *id;
        STRING *name;
    } chart;

} QUERY_METRIC;

#define MAX_QUERY_TARGET_ID_LENGTH 255

typedef struct query_target_request {
    // selecting / filtering metrics to be queried
    RRDHOST *host;                      // the host to be queried (can be NULL, hosts will be used)
    RRDCONTEXT_ACQUIRED *rca;           // the context to be queried (can be NULL)
    RRDINSTANCE_ACQUIRED *ria;          // the instance to be queried (can be NULL)
    RRDMETRIC_ACQUIRED *rma;            // the metric to be queried (can be NULL)
    RRDSET *st;                         // the chart to be queried (NULL, for context queries)
    const char *hosts;                  // hosts simple pattern
    const char *contexts;               // contexts simple pattern (context queries)
    const char *charts;                 // charts simple pattern (for context queries)
    const char *dimensions;             // dimensions simple pattern
    const char *chart_label_key;        // select only the chart having this label key
    const char *charts_labels_filter;   // select only the charts having this combo of label key:value

    time_t after;                       // the requested timeframe
    time_t before;                      // the requested timeframe
    size_t points;                      // the requested number of points to be returned

    uint32_t format;                    // DATASOURCE_FORMAT
    RRDR_OPTIONS options;
    time_t timeout;                     // the timeout of the query in seconds

    size_t tier;
    QUERY_SOURCE query_source;
    STORAGE_PRIORITY priority;

    // resampling metric values across time
    time_t resampling_time;

    // grouping metric values across time
    RRDR_TIME_GROUPING time_group_method;
    const char *time_group_options;

    // group by across multiple time-series
    RRDR_GROUP_BY group_by;
    const char *group_by_key;
    RRDR_GROUP_BY_FUNCTION group_by_function;

} QUERY_TARGET_REQUEST;

typedef struct query_target {
    char id[MAX_QUERY_TARGET_ID_LENGTH + 1]; // query identifier (for logging)
    QUERY_TARGET_REQUEST request;

    bool used;                              // when true, this query is currently being used
    size_t queries;                         // how many query we have done so far

    struct {
        bool relative;                      // true when the request made with relative timestamps, true if it was absolute
        bool aligned;
        time_t after;                       // the absolute timestamp this query is about
        time_t before;                      // the absolute timestamp this query is about
        time_t query_granularity;
        size_t points;                        // the number of points the query will return (maybe different from the request)
        size_t group;
        RRDR_TIME_GROUPING group_method;
        const char *group_options;
        size_t resampling_group;
        NETDATA_DOUBLE resampling_divisor;
        RRDR_OPTIONS options;
        size_t tier;
    } window;

    struct {
        time_t first_time_s;                  // the combined first_time_t of all metrics in the query, across all tiers
        time_t last_time_s;                   // the combined last_time_T of all metrics in the query, across all tiers
        time_t minimum_latest_update_every_s; // the min update every of the metrics in the query
    } db;

    struct {
        QUERY_METRIC *array;                // the metrics to be queried (all of them should be queried, no exceptions)
        uint32_t used;                      // how many items of the array are used
        uint32_t size;                      // the size of the array
        SIMPLE_PATTERN *pattern;
    } query;

    struct {
        RRDMETRIC_ACQUIRED **array;
        uint32_t used;                      // how many items of the array are used
        uint32_t size;                      // the size of the array
    } metrics;

    struct {
        RRDINSTANCE_ACQUIRED **array;
        uint32_t used;                      // how many items of the array are used
        uint32_t size;                      // the size of the array
        SIMPLE_PATTERN *pattern;
        SIMPLE_PATTERN *chart_label_key_pattern;
        SIMPLE_PATTERN *charts_labels_filter_pattern;
    } instances;

    struct {
        RRDCONTEXT_ACQUIRED **array;
        uint32_t used;                      // how many items of the array are used
        uint32_t size;                      // the size of the array
        SIMPLE_PATTERN *pattern;
    } contexts;

    struct {
        RRDHOST **array;
        uint32_t used;                      // how many items of the array are used
        uint32_t size;                      // the size of the array
        SIMPLE_PATTERN *pattern;
    } hosts;

} QUERY_TARGET;

void query_target_free(void);
void query_target_release(QUERY_TARGET *qt);

QUERY_TARGET *query_target_create(QUERY_TARGET_REQUEST *qtr);

#endif // NETDATA_RRDCONTEXT_H


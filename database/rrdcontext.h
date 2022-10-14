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
// public API for rrddims

void rrdcontext_updated_rrddim(RRDDIM *rd);
void rrdcontext_removed_rrddim(RRDDIM *rd);
void rrdcontext_updated_rrddim_algorithm(RRDDIM *rd);
void rrdcontext_updated_rrddim_multiplier(RRDDIM *rd);
void rrdcontext_updated_rrddim_divisor(RRDDIM *rd);
void rrdcontext_updated_rrddim_flags(RRDDIM *rd);
void rrdcontext_collected_rrddim(RRDDIM *rd);

// ----------------------------------------------------------------------------
// public API for rrdsets

void rrdcontext_updated_rrdset(RRDSET *st);
void rrdcontext_removed_rrdset(RRDSET *st);
void rrdcontext_updated_rrdset_name(RRDSET *st);
void rrdcontext_updated_rrdset_flags(RRDSET *st);
void rrdcontext_collected_rrdset(RRDSET *st);

// ----------------------------------------------------------------------------
// public API for ACLK

void rrdcontext_hub_checkpoint_command(void *cmd);
void rrdcontext_hub_stop_streaming_command(void *cmd);


// ----------------------------------------------------------------------------
// public API for threads

void rrdcontext_db_rotation(void);
void *rrdcontext_main(void *);

// ----------------------------------------------------------------------------
// public API for queries

typedef struct query_metric {
    uuid_t *metric_uuid;

    time_t first_time_t;
    time_t last_time_t;

    int update_every;

    struct {
        STORAGE_METRIC_HANDLE *metric_handle;
        time_t first_time_t;
        time_t last_time_t;
        time_t update_every;
    } tiers[RRD_STORAGE_TIERS];

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

#define MAX_QUERY_TARGET_ID_LENGTH 200

typedef struct query_target_request {
    RRDHOST *host;                      // the host to be queried (can be NULL, hosts will be used)
    RRDSET *st;                         // the chart to be queries (NULL, for context queries)
    const char *hosts;                  // hosts simple pattern
    const char *contexts;               // contexts simple pattern (context queries)
    const char *charts;                 // charts simple pattern (for context queries)
    const char *dimensions;             // dimensions simple pattern
    const char *chart_label_key;        // select only the chart having this label key
    const char *charts_labels_filter;   // select only the charts having this combo of label key:value
    long long after;                    // the requested timeframe
    long long before;                   // the requested timeframe
    int points;
    int timeout;                        // the timeout of the query
    int max_anomaly_rates;              // it only applies to anomaly rates chart - TODO - remove it
    uint32_t format;                    // DATASOURCE_FORMAT
    RRDR_OPTIONS options;
    RRDR_GROUPING group_method;
    const char *group_options;
    long resampling_time;
    int tier;
} QUERY_TARGET_REQUEST;

typedef struct query_target {
    bool used;
    bool multihost_query;
    bool context_query;
    usec_t start_ut;

    char id[MAX_QUERY_TARGET_ID_LENGTH + 1]; // query identifier (for logging)
    int update_every;                        // the min update every of the metrics in the query
    time_t first_time_t;                     // the combined first_time_t of all metrics in the query
    time_t last_time_t;                      // the combined last_time_T of all metrics in the query
    long points;                             // the number of points the query will return (maybe different from the request)

    QUERY_TARGET_REQUEST request;

    struct {
        bool absolute;  // true when the request made with absolute timestamps, false if it was relative
        size_t after;   // the absolute timestamp this query is about
        size_t before;  // the absolute timestamp this query is about
    } window;

    struct {
        QUERY_METRIC *array;
        uint32_t used;
        uint32_t size;
    } query;

    struct {
        RRDMETRIC_ACQUIRED **array;
        uint32_t used;
        uint32_t size;
    } metrics;

    struct {
        RRDINSTANCE_ACQUIRED **array;
        uint32_t used;
        uint32_t size;
    } instances;

    struct {
        RRDCONTEXT_ACQUIRED **array;
        uint32_t used;
        uint32_t size;
    } contexts;

    struct {
        RRDHOST **array;
        uint32_t used;
        uint32_t size;
    } hosts;

    DICTIONARY *rrdlabels;

} QUERY_TARGET;

void query_target_free(void);
void query_target_release(QUERY_TARGET *qt);

QUERY_TARGET *query_target_create(QUERY_TARGET_REQUEST qtr);

#endif // NETDATA_RRDCONTEXT_H


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

typedef struct rrdcontext_acquired RRDCONTEXT_ACQUIRED;

// ----------------------------------------------------------------------------

#include "../rrd.h"

bool rrdinstance_acquired_id_and_name_are_same(RRDINSTANCE_ACQUIRED *ria);
const char *rrdmetric_acquired_id(RRDMETRIC_ACQUIRED *rma);
const char *rrdmetric_acquired_name(RRDMETRIC_ACQUIRED *rma);
bool rrdmetric_acquired_has_name(RRDMETRIC_ACQUIRED *rma);

STRING *rrdmetric_acquired_id_dup(RRDMETRIC_ACQUIRED *rma);
STRING *rrdmetric_acquired_name_dup(RRDMETRIC_ACQUIRED *rma);

NETDATA_DOUBLE rrdmetric_acquired_last_stored_value(RRDMETRIC_ACQUIRED *rma);
time_t rrdmetric_acquired_first_entry(RRDMETRIC_ACQUIRED *rma);
time_t rrdmetric_acquired_last_entry(RRDMETRIC_ACQUIRED *rma);
bool rrdmetric_acquired_belongs_to_instance(RRDMETRIC_ACQUIRED *rma, RRDINSTANCE_ACQUIRED *ria);

const char *rrdinstance_acquired_id(RRDINSTANCE_ACQUIRED *ria);
const char *rrdinstance_acquired_name(RRDINSTANCE_ACQUIRED *ria);
bool rrdinstance_acquired_has_name(RRDINSTANCE_ACQUIRED *ria);
const char *rrdinstance_acquired_units(RRDINSTANCE_ACQUIRED *ria);
STRING *rrdinstance_acquired_units_dup(RRDINSTANCE_ACQUIRED *ria);
RRDLABELS *rrdinstance_acquired_labels(RRDINSTANCE_ACQUIRED *ria);
DICTIONARY *rrdinstance_acquired_functions(RRDINSTANCE_ACQUIRED *ria);
RRDHOST *rrdinstance_acquired_rrdhost(RRDINSTANCE_ACQUIRED *ria);
RRDSET *rrdinstance_acquired_rrdset(RRDINSTANCE_ACQUIRED *ria);

bool rrdinstance_acquired_belongs_to_context(RRDINSTANCE_ACQUIRED *ria, RRDCONTEXT_ACQUIRED *rca);
time_t rrdinstance_acquired_update_every(RRDINSTANCE_ACQUIRED *ria);

const char *rrdcontext_acquired_units(RRDCONTEXT_ACQUIRED *rca);
const char *rrdcontext_acquired_title(RRDCONTEXT_ACQUIRED *rca);
RRDSET_TYPE rrdcontext_acquired_chart_type(RRDCONTEXT_ACQUIRED *rca);

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
bool rrdcontext_acquired_belongs_to_host(RRDCONTEXT_ACQUIRED *rca, RRDHOST *host);

// ----------------------------------------------------------------------------
// public API for rrddims

void rrdcontext_updated_rrddim(RRDDIM *rd);
void rrdcontext_removed_rrddim(RRDDIM *rd);
void rrdcontext_updated_rrddim_algorithm(RRDDIM *rd);
void rrdcontext_updated_rrddim_multiplier(RRDDIM *rd);
void rrdcontext_updated_rrddim_divisor(RRDDIM *rd);
void rrdcontext_updated_rrddim_flags(RRDDIM *rd);
void rrdcontext_collected_rrddim(RRDDIM *rd);
int rrdcontext_find_dimension_uuid(RRDSET *st, const char *id, nd_uuid_t *store_uuid);

// ----------------------------------------------------------------------------
// public API for rrdsets

void rrdcontext_updated_rrdset(RRDSET *st);
void rrdcontext_removed_rrdset(RRDSET *st);
void rrdcontext_updated_rrdset_name(RRDSET *st);
void rrdcontext_updated_rrdset_flags(RRDSET *st);
void rrdcontext_updated_retention_rrdset(RRDSET *st);
void rrdcontext_collected_rrdset(RRDSET *st);
int rrdcontext_find_chart_uuid(RRDSET *st, nd_uuid_t *store_uuid);

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

typedef enum __attribute__ ((__packed__)) {
    QUERY_STATUS_NONE             = 0,
    QUERY_STATUS_QUERIED          = (1 << 0),
    QUERY_STATUS_DIMENSION_HIDDEN = (1 << 1),
    QUERY_STATUS_EXCLUDED         = (1 << 2),
    QUERY_STATUS_FAILED           = (1 << 3),
} QUERY_STATUS;

typedef struct query_plan_entry {
    size_t tier;
    time_t after;
    time_t before;
} QUERY_PLAN_ENTRY;

#define QUERY_PLANS_MAX (RRD_STORAGE_TIERS)

typedef struct query_metrics_counts {   // counts the number of metrics related to an object
    size_t selected;                    // selected to be queried
    size_t excluded;                    // not selected to be queried
    size_t queried;                     // successfully queried
    size_t failed;                      // failed to be queried
} QUERY_METRICS_COUNTS;

typedef struct query_instances_counts { // counts the number of instances related to an object
    size_t selected;                    // selected to be queried
    size_t excluded;                    // not selected to be queried
    size_t queried;                     // successfully queried
    size_t failed;                      // failed to be queried
} QUERY_INSTANCES_COUNTS;

typedef struct query_alerts_counts {    // counts the number of alerts related to an object
    size_t clear;                       // number of alerts in clear state
    size_t warning;                     // number of alerts in warning state
    size_t critical;                    // number of alerts in critical state
    size_t other;                       // number of alerts in any other state
} QUERY_ALERTS_COUNTS;

typedef struct _query_node {
    uint32_t slot;
    RRDHOST *rrdhost;
    char node_id[UUID_STR_LEN];
    usec_t duration_ut;

    STORAGE_POINT query_points;
    QUERY_INSTANCES_COUNTS instances;
    QUERY_METRICS_COUNTS metrics;
    QUERY_ALERTS_COUNTS alerts;
} QUERY_NODE;

typedef struct _query_context {
    uint32_t slot;
    RRDCONTEXT_ACQUIRED *rca;

    STORAGE_POINT query_points;
    QUERY_INSTANCES_COUNTS instances;
    QUERY_METRICS_COUNTS metrics;
    QUERY_ALERTS_COUNTS alerts;
} QUERY_CONTEXT;

typedef struct _query_instance {
    uint32_t slot;
    uint32_t query_host_id;
    RRDINSTANCE_ACQUIRED *ria;
    STRING *id_fqdn;        // never access this directly - it is created on demand via query_instance_id_fqdn()
    STRING *name_fqdn;      // never access this directly - it is created on demand via query_instance_name_fqdn()

    STORAGE_POINT query_points;
    QUERY_METRICS_COUNTS metrics;
    QUERY_ALERTS_COUNTS alerts;
} QUERY_INSTANCE;

typedef struct _query_dimension {
    uint32_t slot;
    uint32_t priority;
    RRDMETRIC_ACQUIRED *rma;
    QUERY_STATUS status;
} QUERY_DIMENSION;

typedef struct _query_metric {
    RRDR_DIMENSION_FLAGS status;

    struct query_metric_tier {
        STORAGE_METRIC_HANDLE *smh;
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
        uint32_t query_node_id;
        uint32_t query_context_id;
        uint32_t query_instance_id;
        uint32_t query_dimension_id;
    } link;

    STORAGE_POINT query_points;

    struct {
        uint32_t slot;
        uint32_t first_slot;
        STRING *id;
        STRING *name;
        STRING *units;
    } grouped_as;

    usec_t duration_ut;
} QUERY_METRIC;

#define MAX_QUERY_TARGET_ID_LENGTH 255
#define MAX_QUERY_GROUP_BY_PASSES 2

typedef bool (*qt_interrupt_callback_t)(void *data);

struct group_by_pass {
    RRDR_GROUP_BY group_by;
    char *group_by_label;
    RRDR_GROUP_BY_FUNCTION aggregation;
};

typedef struct query_target_request {
    size_t version;

    const char *scope_nodes;
    const char *scope_contexts;

    // selecting / filtering metrics to be queried
    RRDHOST *host;                      // the host to be queried (can be NULL, hosts will be used)
    RRDCONTEXT_ACQUIRED *rca;           // the context to be queried (can be NULL)
    RRDINSTANCE_ACQUIRED *ria;          // the instance to be queried (can be NULL)
    RRDMETRIC_ACQUIRED *rma;            // the metric to be queried (can be NULL)
    RRDSET *st;                         // the chart to be queried (NULL, for context queries)
    const char *nodes;                  // hosts simple pattern
    const char *contexts;               // contexts simple pattern (context queries)
    const char *instances;                 // charts simple pattern (for context queries)
    const char *dimensions;             // dimensions simple pattern
    const char *chart_label_key;        // select only the chart having this label key
    const char *labels;                 // select only the charts having this combo of label key:value
    const char *alerts;                 // select only the charts having this combo of alert name:status

    time_t after;                       // the requested timeframe
    time_t before;                      // the requested timeframe
    size_t points;                      // the requested number of points to be returned

    uint32_t format;                    // DATASOURCE_FORMAT
    RRDR_OPTIONS options;
    time_t timeout_ms;                     // the timeout of the query in milliseconds

    size_t tier;
    QUERY_SOURCE query_source;
    STORAGE_PRIORITY priority;

    // resampling metric values across time
    time_t resampling_time;

    // grouping metric values across time
    RRDR_TIME_GROUPING time_group_method;
    const char *time_group_options;

    // group by across multiple time-series
    struct group_by_pass group_by[MAX_QUERY_GROUP_BY_PASSES];

    usec_t received_ut;

    qt_interrupt_callback_t interrupt_callback;
    void *interrupt_callback_data;

    nd_uuid_t *transaction;
} QUERY_TARGET_REQUEST;

#define GROUP_BY_MAX_LABEL_KEYS 10

struct query_tier_statistics {
    size_t queries;
    size_t points;
    time_t update_every;
    struct {
        time_t first_time_s;
        time_t last_time_s;
    } retention;
};

struct query_versions {
    uint64_t contexts_hard_hash;
    uint64_t contexts_soft_hash;
    uint64_t alerts_hard_hash;
    uint64_t alerts_soft_hash;
};

struct query_timings {
    usec_t received_ut;
    usec_t preprocessed_ut;
    usec_t executed_ut;
    usec_t finished_ut;
};

#define query_view_update_every(qt) ((qt)->window.group * (qt)->window.query_granularity)

typedef struct query_target {
    char id[MAX_QUERY_TARGET_ID_LENGTH + 1]; // query identifier (for logging)
    QUERY_TARGET_REQUEST request;

    struct {
        time_t now;                         // the current timestamp, the absolute max for any query timestamp
        bool relative;                      // true when the request made with relative timestamps, true if it was absolute
        bool aligned;
        time_t after;                       // the absolute timestamp this query is about
        time_t before;                      // the absolute timestamp this query is about
        time_t query_granularity;
        size_t points;                      // the number of points the query will return (maybe different from the request)
        size_t group;
        RRDR_TIME_GROUPING time_group_method;
        const char *time_group_options;
        size_t resampling_group;
        NETDATA_DOUBLE resampling_divisor;
        RRDR_OPTIONS options;
        size_t tier;
    } window;

    struct {
        size_t queries[RRD_STORAGE_TIERS];
        time_t first_time_s;                  // the combined first_time_t of all metrics in the query, across all tiers
        time_t last_time_s;                   // the combined last_time_T of all metrics in the query, across all tiers
        time_t minimum_latest_update_every_s; // the min update every of the metrics in the query
        struct query_tier_statistics tiers[RRD_STORAGE_TIERS];
    } db;

    struct {
        QUERY_METRIC *array;                // the metrics to be queried (all of them should be queried, no exceptions)
        uint32_t used;                      // how many items of the array are used
        uint32_t size;                      // the size of the array
        SIMPLE_PATTERN *pattern;
    } query;

    struct {
        QUERY_DIMENSION *array;
        uint32_t used;                      // how many items of the array are used
        uint32_t size;                      // the size of the array
    } dimensions;

    struct {
        QUERY_INSTANCE *array;
        uint32_t used;                      // how many items of the array are used
        uint32_t size;                      // the size of the array
        SIMPLE_PATTERN *pattern;
        SIMPLE_PATTERN *labels_pattern;
        SIMPLE_PATTERN *alerts_pattern;
        SIMPLE_PATTERN *chart_label_key_pattern;
    } instances;

    struct {
        QUERY_CONTEXT *array;
        uint32_t used;                      // how many items of the array are used
        uint32_t size;                      // the size of the array
        SIMPLE_PATTERN *pattern;
        SIMPLE_PATTERN *scope_pattern;
    } contexts;

    struct {
        QUERY_NODE *array;
        uint32_t used;                      // how many items of the array are used
        uint32_t size;                      // the size of the array
        SIMPLE_PATTERN *pattern;
        SIMPLE_PATTERN *scope_pattern;
    } nodes;

    struct {
        size_t used;
        char *label_keys[GROUP_BY_MAX_LABEL_KEYS * MAX_QUERY_GROUP_BY_PASSES];
    } group_by[MAX_QUERY_GROUP_BY_PASSES];

    STORAGE_POINT query_points;
    struct query_versions versions;
    struct query_timings timings;

    struct {
        SPINLOCK spinlock;
        bool used;                              // when true, this query is currently being used
        bool relative;                          // when true, this query uses relative timestamps
        size_t queries;                         // how many query we have done so far with this QUERY_TARGET - not related to database queries
        struct query_target *prev;
        struct query_target *next;
    } internal;
} QUERY_TARGET;


struct sql_alert_transition_data {
    usec_t global_id;
    nd_uuid_t *transition_id;
    nd_uuid_t *host_id;
    nd_uuid_t *config_hash_id;
    uint32_t alarm_id;
    const char *alert_name;
    const char *chart;
    const char *chart_name;
    const char *chart_context;
    const char *family;
    const char *recipient;
    const char *units;
    const char *exec;
    const char *info;
    const char *summary;
    const char *classification;
    const char *type;
    const char *component;
    time_t when_key;
    time_t duration;
    time_t non_clear_duration;
    uint64_t flags;
    time_t delay_up_to_timestamp;
    time_t exec_run_timestamp;
    int exec_code;
    int new_status;
    int old_status;
    int delay;
    time_t last_repeat;
    NETDATA_DOUBLE new_value;
    NETDATA_DOUBLE old_value;
};

struct sql_alert_config_data {
    nd_uuid_t *config_hash_id;
    const char *name;

    struct {
        const char *on_template;
        const char *on_key;

        const char *families;
        const char *host_labels;
        const char *chart_labels;
    } selectors;

    const char *info;
    const char *classification;
    const char *component;
    const char *type;
    const char *summary;

    struct {
        struct {
            const char *dimensions;
            const char *method;
            ALERT_LOOKUP_TIME_GROUP_CONDITION time_group_condition;
            NETDATA_DOUBLE time_group_value;
            ALERT_LOOKUP_DIMS_GROUPING dims_group;
            ALERT_LOOKUP_DATA_SOURCE data_source;
            uint32_t options;

            int32_t after;
            int32_t before;

            const char *lookup;         // the lookup line, unparsed
        } db;

        const char *calc;               // the calculation expression, unparsed
        const char *units;

        int32_t update_every;           // the update frequency of the alert, in seconds
        const char *every;              // the every line, unparsed
    } value;

    struct {
        const char *green;              // the green threshold, unparsed
        const char *red;                // the red threshold, unparsed
        const char *warn;               // the warning expression, unparsed
        const char *crit;               // the critical expression, unparsed
    } status;

    struct {
        const char *exec;               // the script to execute, or NULL to execute the default script
        const char *to_key;             // the recipient, or NULL for the default recipient
        const char *delay;              // the delay line, unparsed
        const char *repeat;             // the repeat line, unparsed
        const char *options;            // FIXME what is this?
    } notification;

    const char *source;                 // the configuration file and line this alert came from
};

int contexts_v2_alert_config_to_json(struct web_client *w, const char *config_hash_id);

struct sql_alert_instance_v2_entry {
    RRDCALC *tmp;

    size_t ati;

    STRING *context;
    STRING *chart_id;
    STRING *chart_name;
    STRING *name;
    STRING *family;
    STRING *units;
    STRING *source;
    STRING *classification;
    STRING *type;
    STRING *component;
    STRING *recipient;
    RRDCALC_STATUS status;
    RRDCALC_FLAGS flags;
    STRING *info;
    STRING *summary;
    NETDATA_DOUBLE value;
    time_t last_updated;
    time_t last_status_change;
    NETDATA_DOUBLE last_status_change_value;
    nd_uuid_t config_hash_id;
    usec_t global_id;
    nd_uuid_t last_transition_id;
    uint32_t alarm_id;
    RRDHOST *host;
    size_t ni;
};

static inline NEVERNULL QUERY_NODE *query_node(QUERY_TARGET *qt, size_t id) {
    internal_fatal(id >= qt->nodes.used, "QUERY: invalid query host id");
    return &qt->nodes.array[id];
}

static inline NEVERNULL QUERY_CONTEXT *query_context(QUERY_TARGET *qt, size_t query_context_id) {
    internal_fatal(query_context_id >= qt->contexts.used, "QUERY: invalid query context id");
    return &qt->contexts.array[query_context_id];
}

static inline NEVERNULL QUERY_INSTANCE *query_instance(QUERY_TARGET *qt, size_t query_instance_id) {
    internal_fatal(query_instance_id >= qt->instances.used, "QUERY: invalid query instance id");
    return &qt->instances.array[query_instance_id];
}

static inline NEVERNULL QUERY_DIMENSION *query_dimension(QUERY_TARGET *qt, size_t query_dimension_id) {
    internal_fatal(query_dimension_id >= qt->dimensions.used, "QUERY: invalid query dimension id");
    return &qt->dimensions.array[query_dimension_id];
}

static inline NEVERNULL QUERY_METRIC *query_metric(QUERY_TARGET *qt, size_t id) {
    internal_fatal(id >= qt->query.used, "QUERY: invalid query metric id");
    return &qt->query.array[id];
}

static inline const char *query_metric_id(QUERY_TARGET *qt, QUERY_METRIC *qm) {
    QUERY_DIMENSION *qd = query_dimension(qt, qm->link.query_dimension_id);
    return rrdmetric_acquired_id(qd->rma);
}

static inline const char *query_metric_name(QUERY_TARGET *qt, QUERY_METRIC *qm) {
    QUERY_DIMENSION *qd = query_dimension(qt, qm->link.query_dimension_id);
    return rrdmetric_acquired_name(qd->rma);
}

struct storage *query_metric_storage_engine(QUERY_TARGET *qt, QUERY_METRIC *qm, size_t tier);

STRING *query_instance_id_fqdn(QUERY_INSTANCE *qi, size_t version);
STRING *query_instance_name_fqdn(QUERY_INSTANCE *qi, size_t version);

void query_target_free(void);
void query_target_release(QUERY_TARGET *qt);

QUERY_TARGET *query_target_create(QUERY_TARGET_REQUEST *qtr);

typedef enum __attribute__((packed)) {
    ATF_STATUS = 0,
    ATF_CLASS,
    ATF_TYPE,
    ATF_COMPONENT,
    ATF_ROLE,
    ATF_NODE,
    ATF_ALERT_NAME,
    ATF_CHART_NAME,
    ATF_CONTEXT,

    // total
    ATF_TOTAL_ENTRIES,
} ALERT_TRANSITION_FACET;

struct alert_transitions_facets {
    const char *name;
    const char *query_param;
    const char *id;
    size_t order;
};

extern struct alert_transitions_facets alert_transition_facets[];

struct api_v2_contexts_request {
    char *scope_nodes;
    char *scope_contexts;
    char *nodes;
    char *contexts;
    char *q;

    CONTEXTS_OPTIONS options;

    struct {
        CONTEXTS_ALERT_STATUS status;
        char *alert;
        char *transition;
        uint32_t last;

        const char *facets[ATF_TOTAL_ENTRIES];
        usec_t global_id_anchor;
    } alerts;

    time_t after;
    time_t before;
    time_t timeout_ms;

    qt_interrupt_callback_t interrupt_callback;
    void *interrupt_callback_data;
};

typedef enum __attribute__ ((__packed__)) {
    CONTEXTS_V2_SEARCH              = (1 << 1),
    CONTEXTS_V2_NODES               = (1 << 2),
    CONTEXTS_V2_NODES_INFO          = (1 << 3),
    CONTEXTS_V2_NODE_INSTANCES      = (1 << 4),
    CONTEXTS_V2_NODES_STREAM_PATH   = (1 << 5),
    CONTEXTS_V2_CONTEXTS            = (1 << 6),
    CONTEXTS_V2_AGENTS              = (1 << 7),
    CONTEXTS_V2_AGENTS_INFO         = (1 << 8),
    CONTEXTS_V2_VERSIONS            = (1 << 9),
    CONTEXTS_V2_FUNCTIONS           = (1 << 10),
    CONTEXTS_V2_ALERTS              = (1 << 11),
    CONTEXTS_V2_ALERT_TRANSITIONS   = (1 << 12),
} CONTEXTS_V2_MODE;

int rrdcontext_to_json_v2(BUFFER *wb, struct api_v2_contexts_request *req, CONTEXTS_V2_MODE mode);

RRDCONTEXT_TO_JSON_OPTIONS rrdcontext_to_json_parse_options(char *o);
void buffer_json_agents_v2(BUFFER *wb, struct query_timings *timings, time_t now_s, bool info, bool array);
void buffer_json_node_add_v2(BUFFER *wb, RRDHOST *host, size_t ni, usec_t duration_ut, bool status);
void buffer_json_query_timings(BUFFER *wb, const char *key, struct query_timings *timings);
void buffer_json_cloud_timings(BUFFER *wb, const char *key, struct query_timings *timings);

// ----------------------------------------------------------------------------
// scope

typedef ssize_t (*foreach_host_cb_t)(void *data, RRDHOST *host, bool queryable);
ssize_t query_scope_foreach_host(SIMPLE_PATTERN *scope_hosts_sp, SIMPLE_PATTERN *hosts_sp,
                                  foreach_host_cb_t cb, void *data,
                                  struct query_versions *versions,
                                  char *host_node_id_str);

typedef ssize_t (*foreach_context_cb_t)(void *data, RRDCONTEXT_ACQUIRED *rca, bool queryable_context);
ssize_t query_scope_foreach_context(RRDHOST *host, const char *scope_contexts, SIMPLE_PATTERN *scope_contexts_sp, SIMPLE_PATTERN *contexts_sp, foreach_context_cb_t cb, bool queryable_host, void *data);

// ----------------------------------------------------------------------------
// public API for weights

typedef ssize_t (*weights_add_metric_t)(void *data, RRDHOST *host, RRDCONTEXT_ACQUIRED *rca, RRDINSTANCE_ACQUIRED *ria, RRDMETRIC_ACQUIRED *rma);
ssize_t weights_foreach_rrdmetric_in_context(RRDCONTEXT_ACQUIRED *rca,
                                            SIMPLE_PATTERN *instances_sp,
                                            SIMPLE_PATTERN *chart_label_key_sp,
                                            SIMPLE_PATTERN *labels_sp,
                                            SIMPLE_PATTERN *alerts_sp,
                                            SIMPLE_PATTERN *dimensions_sp,
                                            bool match_ids, bool match_names,
                                            size_t version,
                                            weights_add_metric_t cb,
                                            void *data);

bool rrdcontext_retention_match(RRDCONTEXT_ACQUIRED *rca, time_t after, time_t before);

#define query_matches_retention(after, before, first_entry_s, last_entry_s, update_every_s) \
    (((first_entry_s) - ((update_every_s) * 2) <= (before)) &&                     \
     ((last_entry_s)  + ((update_every_s) * 2) >= (after)))

#define query_target_aggregatable(qt) ((qt)->window.options & RRDR_OPTION_RETURN_RAW)

static inline bool query_has_group_by_aggregation_percentage(QUERY_TARGET *qt) {

    // backwards compatibility
    // If the request was made with group_by = "percentage-of-instance"
    // we need to send back "raw" output with "count"
    // otherwise, we need to send back "raw" output with "hidden"

    bool last_is_percentage = false;

    for(int g = 0; g < MAX_QUERY_GROUP_BY_PASSES ;g++) {
        if(qt->request.group_by[g].group_by == RRDR_GROUP_BY_NONE)
            break;

        if(qt->request.group_by[g].group_by & RRDR_GROUP_BY_PERCENTAGE_OF_INSTANCE)
            // backwards compatibility
            return false;

        if(qt->request.group_by[g].aggregation == RRDR_GROUP_BY_FUNCTION_PERCENTAGE)
            last_is_percentage = true;

        else
            last_is_percentage = false;
    }

    return last_is_percentage;
}

static inline bool query_target_has_percentage_of_group(QUERY_TARGET *qt) {
    for(size_t g = 0; g < MAX_QUERY_GROUP_BY_PASSES ;g++) {
        if (qt->request.group_by[g].group_by & RRDR_GROUP_BY_PERCENTAGE_OF_INSTANCE)
            return true;

        if (qt->request.group_by[g].aggregation == RRDR_GROUP_BY_FUNCTION_PERCENTAGE)
            return true;
    }

    return false;
}

static inline bool query_target_needs_all_dimensions(QUERY_TARGET *qt) {
    if(qt->request.options & RRDR_OPTION_PERCENTAGE)
        return true;

    return query_target_has_percentage_of_group(qt);
}

static inline bool query_target_has_percentage_units(QUERY_TARGET *qt) {
    if(qt->window.time_group_method == RRDR_GROUPING_CV)
        return true;

    if((qt->request.options & RRDR_OPTION_PERCENTAGE) && !(qt->window.options & RRDR_OPTION_RETURN_RAW))
        return true;

    return query_target_has_percentage_of_group(qt);
}

#endif // NETDATA_RRDCONTEXT_H


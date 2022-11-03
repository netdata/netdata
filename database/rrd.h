// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRD_H
#define NETDATA_RRD_H 1

#ifdef __cplusplus
extern "C" {
#endif

// non-existing structs instead of voids
// to enable type checking at compile time
typedef struct storage_instance STORAGE_INSTANCE;
typedef struct storage_metric_handle STORAGE_METRIC_HANDLE;
typedef struct storage_alignment STORAGE_METRICS_GROUP;

// forward typedefs
typedef struct rrdhost RRDHOST;
typedef struct rrddim RRDDIM;
typedef struct rrdset RRDSET;
typedef struct rrdcalc RRDCALC;
typedef struct rrdcalctemplate RRDCALCTEMPLATE;
typedef struct alarm_entry ALARM_ENTRY;

typedef struct rrdfamily_acquired RRDFAMILY_ACQUIRED;
typedef struct rrdvar_acquired RRDVAR_ACQUIRED;
typedef struct rrdsetvar_acquired RRDSETVAR_ACQUIRED;
typedef struct rrdcalc_acquired RRDCALC_ACQUIRED;

typedef struct rrdhost_acquired RRDHOST_ACQUIRED;
typedef struct rrdset_acquired RRDSET_ACQUIRED;
typedef struct rrddim_acquired RRDDIM_ACQUIRED;

typedef void *ml_host_t;
typedef void *ml_dimension_t;

// forward declarations
struct rrddim_tier;

#ifdef ENABLE_DBENGINE
struct rrdeng_page_descr;
struct rrdengine_instance;
struct pg_cache_page_index;
#endif

#include "daemon/common.h"
#include "web/api/queries/query.h"
#include "web/api/queries/rrdr.h"
#include "rrdvar.h"
#include "rrdsetvar.h"
#include "rrddimvar.h"
#include "rrdcalc.h"
#include "rrdcalctemplate.h"
#include "streaming/rrdpush.h"
#include "aclk/aclk_rrdhost_state.h"
#include "sqlite/sqlite_health.h"
#include "rrdcontext.h"

extern bool dbengine_enabled;
extern size_t storage_tiers;
extern size_t storage_tiers_grouping_iterations[RRD_STORAGE_TIERS];

typedef enum {
    RRD_BACKFILL_NONE,
    RRD_BACKFILL_FULL,
    RRD_BACKFILL_NEW
} RRD_BACKFILL;

extern RRD_BACKFILL storage_tiers_backfill[RRD_STORAGE_TIERS];

enum {
    CONTEXT_FLAGS_ARCHIVE = 0x01,
    CONTEXT_FLAGS_CHART   = 0x02,
    CONTEXT_FLAGS_CONTEXT = 0x04
};

struct context_param {
    RRDDIM *rd;
    time_t first_entry_t;
    time_t last_entry_t;
    uint8_t flags;
};

#define UPDATE_EVERY 1
#define UPDATE_EVERY_MAX 3600

#define RRD_DEFAULT_HISTORY_ENTRIES 3600
#define RRD_HISTORY_ENTRIES_MAX (86400*365)

extern int default_rrd_update_every;
extern int default_rrd_history_entries;
extern int gap_when_lost_iterations_above;
extern time_t rrdset_free_obsolete_time;

#define RRD_ID_LENGTH_MAX 200

typedef long long total_number;
#define TOTAL_NUMBER_FORMAT "%lld"

// ----------------------------------------------------------------------------
// chart types

typedef enum rrdset_type {
    RRDSET_TYPE_LINE    = 0,
    RRDSET_TYPE_AREA    = 1,
    RRDSET_TYPE_STACKED = 2
} RRDSET_TYPE;

#define RRDSET_TYPE_LINE_NAME "line"
#define RRDSET_TYPE_AREA_NAME "area"
#define RRDSET_TYPE_STACKED_NAME "stacked"

RRDSET_TYPE rrdset_type_id(const char *name);
const char *rrdset_type_name(RRDSET_TYPE chart_type);


// ----------------------------------------------------------------------------
// memory mode

typedef enum rrd_memory_mode {
    RRD_MEMORY_MODE_NONE = 0,
    RRD_MEMORY_MODE_RAM  = 1,
    RRD_MEMORY_MODE_MAP  = 2,
    RRD_MEMORY_MODE_SAVE = 3,
    RRD_MEMORY_MODE_ALLOC = 4,
    RRD_MEMORY_MODE_DBENGINE = 5,

    // this is 8-bit
} RRD_MEMORY_MODE;

#define RRD_MEMORY_MODE_NONE_NAME "none"
#define RRD_MEMORY_MODE_RAM_NAME "ram"
#define RRD_MEMORY_MODE_MAP_NAME "map"
#define RRD_MEMORY_MODE_SAVE_NAME "save"
#define RRD_MEMORY_MODE_ALLOC_NAME "alloc"
#define RRD_MEMORY_MODE_DBENGINE_NAME "dbengine"

extern RRD_MEMORY_MODE default_rrd_memory_mode;

const char *rrd_memory_mode_name(RRD_MEMORY_MODE id);
RRD_MEMORY_MODE rrd_memory_mode_id(const char *name);


// ----------------------------------------------------------------------------
// algorithms types

typedef enum rrd_algorithm {
    RRD_ALGORITHM_ABSOLUTE              = 0,
    RRD_ALGORITHM_INCREMENTAL           = 1,
    RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL = 2,
    RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL  = 3,

    // this is 8-bit
} RRD_ALGORITHM;

#define RRD_ALGORITHM_ABSOLUTE_NAME                "absolute"
#define RRD_ALGORITHM_INCREMENTAL_NAME             "incremental"
#define RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL_NAME   "percentage-of-incremental-row"
#define RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL_NAME    "percentage-of-absolute-row"

RRD_ALGORITHM rrd_algorithm_id(const char *name);
const char *rrd_algorithm_name(RRD_ALGORITHM algorithm);

// ----------------------------------------------------------------------------
// RRD FAMILY

const RRDFAMILY_ACQUIRED *rrdfamily_add_and_acquire(RRDHOST *host, const char *id);
void rrdfamily_release(RRDHOST *host, const RRDFAMILY_ACQUIRED *rfa);
void rrdfamily_index_init(RRDHOST *host);
void rrdfamily_index_destroy(RRDHOST *host);
DICTIONARY *rrdfamily_rrdvars_dict(const RRDFAMILY_ACQUIRED *rf);


// ----------------------------------------------------------------------------
// flags & options

// options are permanent configuration options (no atomics to alter/access them)
typedef enum rrddim_options {
    RRDDIM_OPTION_NONE                              = 0,
    RRDDIM_OPTION_HIDDEN                            = (1 << 0), // this dimension will not be offered to callers
    RRDDIM_OPTION_DONT_DETECT_RESETS_OR_OVERFLOWS   = (1 << 1), // do not offer RESET or OVERFLOW info to callers
    RRDDIM_OPTION_BACKFILLED_HIGH_TIERS             = (1 << 2), // when set, we have backfilled higher tiers

    // this is 8-bit
} RRDDIM_OPTIONS;

#define rrddim_option_check(rd, option) ((rd)->options & (option))
#define rrddim_option_set(rd, option)   (rd)->options |= (option)
#define rrddim_option_clear(rd, option) (rd)->options &= ~(option)

// flags are runtime changing status flags (atomics are required to alter/access them)
typedef enum rrddim_flags {
    RRDDIM_FLAG_NONE                            = 0,
    RRDDIM_FLAG_PENDING_HEALTH_INITIALIZATION   = (1 << 0),

    RRDDIM_FLAG_OBSOLETE                        = (1 << 2),  // this is marked by the collector/module as obsolete
    // No new values have been collected for this dimension since agent start, or it was marked RRDDIM_FLAG_OBSOLETE at
    // least rrdset_free_obsolete_time seconds ago.
    RRDDIM_FLAG_ARCHIVED                        = (1 << 3),
    RRDDIM_FLAG_METADATA_UPDATE                 = (1 << 4),  // Metadata needs to go to the database

    RRDDIM_FLAG_META_HIDDEN                     = (1 << 6), // Status of hidden option in the metadata database

    // this is 8 bit
} RRDDIM_FLAGS;

#define rrddim_flag_check(rd, flag) (__atomic_load_n(&((rd)->flags), __ATOMIC_SEQ_CST) & (flag))
#define rrddim_flag_set(rd, flag)   __atomic_or_fetch(&((rd)->flags), (flag), __ATOMIC_SEQ_CST)
#define rrddim_flag_clear(rd, flag) __atomic_and_fetch(&((rd)->flags), ~(flag), __ATOMIC_SEQ_CST)

typedef enum rrdlabel_source {
    RRDLABEL_SRC_AUTO       = (1 << 0), // set when Netdata found the label by some automation
    RRDLABEL_SRC_CONFIG     = (1 << 1), // set when the user configured the label
    RRDLABEL_SRC_K8S        = (1 << 2), // set when this label is found from k8s (RRDLABEL_SRC_AUTO should also be set)
    RRDLABEL_SRC_ACLK       = (1 << 3), // set when this label is found from ACLK (RRDLABEL_SRC_AUTO should also be set)

    // more sources can be added here

    RRDLABEL_FLAG_PERMANENT = (1 << 29), // set when this label should never be removed (can be overwritten though)
    RRDLABEL_FLAG_OLD       = (1 << 30), // marks for rrdlabels internal use - they are not exposed outside rrdlabels
    RRDLABEL_FLAG_NEW       = (1 << 31)  // marks for rrdlabels internal use - they are not exposed outside rrdlabels
} RRDLABEL_SRC;

#define RRDLABEL_FLAG_INTERNAL (RRDLABEL_FLAG_OLD | RRDLABEL_FLAG_NEW | RRDLABEL_FLAG_PERMANENT)

size_t text_sanitize(unsigned char *dst, const unsigned char *src, size_t dst_size, unsigned char *char_map, bool utf, const char *empty, size_t *multibyte_length);

DICTIONARY *rrdlabels_create(void);
void rrdlabels_destroy(DICTIONARY *labels_dict);
void rrdlabels_add(DICTIONARY *dict, const char *name, const char *value, RRDLABEL_SRC ls);
void rrdlabels_add_pair(DICTIONARY *dict, const char *string, RRDLABEL_SRC ls);
void rrdlabels_get_value_to_buffer_or_null(DICTIONARY *labels, BUFFER *wb, const char *key, const char *quote, const char *null);
void rrdlabels_get_value_to_char_or_null(DICTIONARY *labels, char **value, const char *key);
void rrdlabels_flush(DICTIONARY *labels_dict);

void rrdlabels_unmark_all(DICTIONARY *labels);
void rrdlabels_remove_all_unmarked(DICTIONARY *labels);

int rrdlabels_walkthrough_read(DICTIONARY *labels, int (*callback)(const char *name, const char *value, RRDLABEL_SRC ls, void *data), void *data);
int rrdlabels_sorted_walkthrough_read(DICTIONARY *labels, int (*callback)(const char *name, const char *value, RRDLABEL_SRC ls, void *data), void *data);

void rrdlabels_log_to_buffer(DICTIONARY *labels, BUFFER *wb);
bool rrdlabels_match_simple_pattern(DICTIONARY *labels, const char *simple_pattern_txt);
bool rrdlabels_match_simple_pattern_parsed(DICTIONARY *labels, SIMPLE_PATTERN *pattern, char equal);
int rrdlabels_to_buffer(DICTIONARY *labels, BUFFER *wb, const char *before_each, const char *equal, const char *quote, const char *between_them, bool (*filter_callback)(const char *name, const char *value, RRDLABEL_SRC ls, void *data), void *filter_data, void (*name_sanitizer)(char *dst, const char *src, size_t dst_size), void (*value_sanitizer)(char *dst, const char *src, size_t dst_size));

void rrdlabels_migrate_to_these(DICTIONARY *dst, DICTIONARY *src);
void rrdlabels_copy(DICTIONARY *dst, DICTIONARY *src);

void reload_host_labels(void);
void rrdset_update_rrdlabels(RRDSET *st, DICTIONARY *new_rrdlabels);
void rrdset_save_rrdlabels_to_sql(RRDSET *st);
void rrdhost_set_is_parent_label(int count);
int rrdlabels_unittest(void);

// unfortunately this break when defined in exporting_engine.h
bool exporting_labels_filter_callback(const char *name, const char *value, RRDLABEL_SRC ls, void *data);

// ----------------------------------------------------------------------------
// RRD DIMENSION - this is a metric

struct rrddim {
    uuid_t metric_uuid;                             // global UUID for this metric (unique_across hosts)

    // ------------------------------------------------------------------------
    // dimension definition

    STRING *id;                                     // the id of this dimension (for internal identification)
    STRING *name;                                   // the name of this dimension (as presented to user)

    RRD_ALGORITHM algorithm:8;                      // the algorithm that is applied to add new collected values
    RRDDIM_OPTIONS options:8;                       // permanent configuration options
    RRD_MEMORY_MODE rrd_memory_mode:8;              // the memory mode for this dimension
    /*RRDDIM_FLAGS*/ uint8_t flags;                 // run time changing status flags

    bool updated;                                   // 1 when the dimension has been updated since the last processing
    bool exposed;                                   // 1 when set what have sent this dimension to the central netdata

    collected_number multiplier;                    // the multiplier of the collected values
    collected_number divisor;                       // the divider of the collected values

    int update_every;                               // every how many seconds is this updated
                                                    // TODO - remove update_every from rrddim
                                                    //        it is always the same in rrdset

    // ------------------------------------------------------------------------
    // operational state members

    ml_dimension_t ml_dimension;                    // machine learning data about this dimension

    // ------------------------------------------------------------------------
    // linking to siblings and parents

    struct rrdset *rrdset;

    RRDMETRIC_ACQUIRED *rrdmetric;                  // the rrdmetric of this dimension

    // ------------------------------------------------------------------------
    // data collection members

    struct rrddim_tier *tiers[RRD_STORAGE_TIERS];   // our tiers of databases

    struct timeval last_collected_time;             // when was this dimension last updated
                                                    // this is actual date time we updated the last_collected_value
                                                    // THIS IS DIFFERENT FROM THE SAME MEMBER OF RRDSET

    size_t collections_counter;                     // the number of times we added values to this rrddim
    collected_number collected_value_max;           // the absolute maximum of the collected value

    NETDATA_DOUBLE calculated_value;                // the current calculated value, after applying the algorithm - resets to zero after being used
    NETDATA_DOUBLE last_calculated_value;           // the last calculated value processed
    NETDATA_DOUBLE last_stored_value;               // the last value as stored in the database (after interpolation)

    collected_number collected_value;               // the current value, as collected - resets to 0 after being used
    collected_number last_collected_value;          // the last value that was collected, after being processed

    // ------------------------------------------------------------------------
    // db mode RAM, SAVE, MAP, ALLOC, NONE specifics
    // TODO - they should be managed by storage engine
    //        (RRDDIM_DB_STATE ptr to an undefined structure, and a call to clean this up during destruction)

    size_t memsize;                                 // the memory allocated for this dimension (without RRDDIM)
    void *rd_on_file;                               // pointer to the header written on disk
    storage_number *db;                             // the array of values
};

#define rrddim_id(rd) string2str((rd)->id)
#define rrddim_name(rd) string2str((rd) ->name)

// returns the RRDDIM cache filename, or NULL if it does not exist
const char *rrddim_cache_filename(RRDDIM *rd);

// updated the header with the latest RRDDIM value, for memory mode MAP and SAVE
void rrddim_memory_file_update(RRDDIM *rd);

// free the memory file structures for memory mode MAP and SAVE
void rrddim_memory_file_free(RRDDIM *rd);

bool rrddim_memory_load_or_create_map_save(RRDSET *st, RRDDIM *rd, RRD_MEMORY_MODE memory_mode);

// return the v019 header size of RRDDIM files
size_t rrddim_memory_file_header_size(void);

void rrddim_memory_file_save(RRDDIM *rd);

// ----------------------------------------------------------------------------

typedef struct storage_point {
    NETDATA_DOUBLE min;     // when count > 1, this is the minimum among them
    NETDATA_DOUBLE max;     // when count > 1, this is the maximum among them
    NETDATA_DOUBLE sum;     // the point sum - divided by count gives the average

    // end_time - start_time = point duration
    time_t start_time;      // the time the point starts
    time_t end_time;        // the time the point ends

    unsigned count;         // the number of original points aggregated
    unsigned anomaly_count; // the number of original points found anomalous

    SN_FLAGS flags;         // flags stored with the point
} STORAGE_POINT;

#define storage_point_unset(x)                     do { \
    (x).min = (x).max = (x).sum = NAN;                  \
    (x).count = 0;                                      \
    (x).anomaly_count = 0;                              \
    (x).flags = SN_FLAG_NONE;                           \
    (x).start_time = 0;                                 \
    (x).end_time = 0;                                   \
    } while(0)

#define storage_point_empty(x, start_t, end_t)     do { \
    (x).min = (x).max = (x).sum = NAN;                  \
    (x).count = 1;                                      \
    (x).anomaly_count = 0;                              \
    (x).flags = SN_FLAG_NONE;                           \
    (x).start_time = start_t;                           \
    (x).end_time = end_t;                               \
    } while(0)

#define storage_point_is_unset(x) (!(x).count)
#define storage_point_is_empty(x) (!netdata_double_isnumber((x).sum))

// ----------------------------------------------------------------------------
// engine-specific iterator state for dimension data collection
typedef struct storage_collect_handle STORAGE_COLLECT_HANDLE;

// ----------------------------------------------------------------------------
// engine-specific iterator state for dimension data queries
typedef struct storage_query_handle STORAGE_QUERY_HANDLE;

// ------------------------------------------------------------------------
// function pointers that handle data collection
struct storage_engine_collect_ops {
    // an initialization function to run before starting collection
    STORAGE_COLLECT_HANDLE *(*init)(STORAGE_METRIC_HANDLE *db_metric_handle, uint32_t update_every);

    // run this to store each metric into the database
    void (*store_metric)(STORAGE_COLLECT_HANDLE *collection_handle, usec_t point_in_time, NETDATA_DOUBLE number, NETDATA_DOUBLE min_value,
                         NETDATA_DOUBLE max_value, uint16_t count, uint16_t anomaly_count, SN_FLAGS flags);

    // run this to flush / reset the current data collection sequence
    void (*flush)(STORAGE_COLLECT_HANDLE *collection_handle);

    // a finalization function to run after collection is over
    // returns 1 if it's safe to delete the dimension
    int (*finalize)(STORAGE_COLLECT_HANDLE *collection_handle);

    void (*change_collection_frequency)(STORAGE_COLLECT_HANDLE *collection_handle, int update_every);

    STORAGE_METRICS_GROUP *(*metrics_group_get)(STORAGE_INSTANCE *db_instance, uuid_t *uuid);
    void (*metrics_group_release)(STORAGE_INSTANCE *db_instance, STORAGE_METRICS_GROUP *sa);
};

// ----------------------------------------------------------------------------
// iterator state for RRD dimension data queries
struct storage_engine_query_handle {
    RRDDIM *rd;
    time_t start_time_s;
    time_t end_time_s;
    STORAGE_QUERY_HANDLE* handle;
};

// function pointers that handle database queries
struct storage_engine_query_ops {
    // run this before starting a series of next_metric() database queries
    void (*init)(STORAGE_METRIC_HANDLE *db_metric_handle, struct storage_engine_query_handle *handle, time_t start_time, time_t end_time);

    // run this to load each metric number from the database
    STORAGE_POINT (*next_metric)(struct storage_engine_query_handle *handle);

    // run this to test if the series of next_metric() database queries is finished
    int (*is_finished)(struct storage_engine_query_handle *handle);

    // run this after finishing a series of load_metric() database queries
    void (*finalize)(struct storage_engine_query_handle *handle);

    // get the timestamp of the last entry of this metric
    time_t (*latest_time)(STORAGE_METRIC_HANDLE *db_metric_handle);

    // get the timestamp of the first entry of this metric
    time_t (*oldest_time)(STORAGE_METRIC_HANDLE *db_metric_handle);
};

typedef struct storage_engine STORAGE_ENGINE;

// ------------------------------------------------------------------------
// function pointers for all APIs provided by a storage engine
typedef struct storage_engine_api {
    // metric management
    STORAGE_METRIC_HANDLE *(*metric_get)(STORAGE_INSTANCE *instance, uuid_t *uuid, STORAGE_METRICS_GROUP *smg);
    STORAGE_METRIC_HANDLE *(*metric_get_or_create)(RRDDIM *rd, STORAGE_INSTANCE *instance, STORAGE_METRICS_GROUP *smg);
    void (*metric_release)(STORAGE_METRIC_HANDLE *);
    STORAGE_METRIC_HANDLE *(*metric_dup)(STORAGE_METRIC_HANDLE *);

    // operations
    struct storage_engine_collect_ops collect_ops;
    struct storage_engine_query_ops query_ops;
} STORAGE_ENGINE_API;

struct storage_engine {
    RRD_MEMORY_MODE id;
    const char* name;
    STORAGE_ENGINE_API api;
};

STORAGE_ENGINE* storage_engine_get(RRD_MEMORY_MODE mmode);
STORAGE_ENGINE* storage_engine_find(const char* name);

// ----------------------------------------------------------------------------
// Storage tier data for every dimension

struct rrddim_tier {
    size_t tier_grouping;
    STORAGE_METRIC_HANDLE *db_metric_handle;        // the metric handle inside the database
    STORAGE_COLLECT_HANDLE *db_collection_handle;   // the data collection handle
    STORAGE_POINT virtual_point;
    time_t next_point_time;
    struct storage_engine_collect_ops *collect_ops;
    struct storage_engine_query_ops *query_ops;
};

void rrdr_fill_tier_gap_from_smaller_tiers(RRDDIM *rd, size_t tier, time_t now);

// ----------------------------------------------------------------------------
// these loop macros make sure the linked list is accessed with the right lock

#define rrddim_foreach_read(rd, st) \
    dfe_start_read((st)->rrddim_root_index, rd)

#define rrddim_foreach_write(rd, st) \
    dfe_start_write((st)->rrddim_root_index, rd)

#define rrddim_foreach_reentrant(rd, st) \
    dfe_start_reentrant((st)->rrddim_root_index, rd)

#define rrddim_foreach_done(rd) \
    dfe_done(rd)

// ----------------------------------------------------------------------------
// RRDSET - this is a chart

// use this for configuration flags, not for state control
// flags are set/unset in a manner that is not thread safe
// and may lead to missing information.

typedef enum rrdset_flags {
    RRDSET_FLAG_DETAIL                  = (1 << 1),  // if set, the data set should be considered as a detail of another
                                                     // (the master data set should be the one that has the same family and is not detail)
    RRDSET_FLAG_DEBUG                   = (1 << 2),  // enables or disables debugging for a chart
    RRDSET_FLAG_OBSOLETE                = (1 << 3),  // this is marked by the collector/module as obsolete
    RRDSET_FLAG_EXPORTING_SEND          = (1 << 4),  // if set, this chart should be sent to Prometheus web API and external databases
    RRDSET_FLAG_EXPORTING_IGNORE        = (1 << 5),  // if set, this chart should not be sent to Prometheus web API and external databases

    RRDSET_FLAG_UPSTREAM_SEND           = (1 << 6),  // if set, this chart should be sent upstream (streaming)
    RRDSET_FLAG_UPSTREAM_IGNORE         = (1 << 7),  // if set, this chart should not be sent upstream (streaming)
    RRDSET_FLAG_UPSTREAM_EXPOSED        = (1 << 8),  // if set, we have sent this chart definition to netdata parent (streaming)

    RRDSET_FLAG_STORE_FIRST             = (1 << 9),  // if set, do not eliminate the first collection during interpolation
    RRDSET_FLAG_HETEROGENEOUS           = (1 << 10), // if set, the chart is not homogeneous (dimensions in it have multiple algorithms, multipliers or dividers)
    RRDSET_FLAG_HOMOGENEOUS_CHECK       = (1 << 11), // if set, the chart should be checked to determine if the dimensions are homogeneous
    RRDSET_FLAG_HIDDEN                  = (1 << 12), // if set, do not show this chart on the dashboard, but use it for exporting
    RRDSET_FLAG_SYNC_CLOCK              = (1 << 13), // if set, microseconds on next data collection will be ignored (the chart will be synced to now)
    RRDSET_FLAG_OBSOLETE_DIMENSIONS     = (1 << 14), // this is marked by the collector/module when a chart has obsolete dimensions
                                                     // No new values have been collected for this chart since agent start, or it was marked RRDSET_FLAG_OBSOLETE at
                                                     // least rrdset_free_obsolete_time seconds ago.
    RRDSET_FLAG_ARCHIVED                = (1 << 15),
    RRDSET_FLAG_METADATA_UPDATE         = (1 << 16), // Mark that metadata needs to be stored
    RRDSET_FLAG_ANOMALY_DETECTION       = (1 << 18), // flag to identify anomaly detection charts.
    RRDSET_FLAG_INDEXED_ID              = (1 << 19), // the rrdset is indexed by its id
    RRDSET_FLAG_INDEXED_NAME            = (1 << 20), // the rrdset is indexed by its name

    RRDSET_FLAG_ANOMALY_RATE_CHART      = (1 << 21), // the rrdset is for storing anomaly rates for all dimensions
    RRDSET_FLAG_PENDING_HEALTH_INITIALIZATION = (1 << 22),

    RRDSET_FLAG_SENDER_REPLICATION_FINISHED   = (1 << 23), // the sending side has completed replication
    RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED = (1 << 24), // the receiving side has completed replication

    RRDSET_FLAG_UPSTREAM_SEND_VARIABLES = (1 << 25), // a custom variable has been updated and needs to be exposed to parent
} RRDSET_FLAGS;

#define rrdset_flag_check(st, flag) (__atomic_load_n(&((st)->flags), __ATOMIC_SEQ_CST) & (flag))
#define rrdset_flag_set(st, flag)   __atomic_or_fetch(&((st)->flags), flag, __ATOMIC_SEQ_CST)
#define rrdset_flag_clear(st, flag) __atomic_and_fetch(&((st)->flags), ~(flag), __ATOMIC_SEQ_CST)

#define rrdset_is_ar_chart(st) rrdset_flag_check(st, RRDSET_FLAG_ANOMALY_RATE_CHART)

struct rrdset {
    uuid_t chart_uuid;                             // the global UUID for this chart

    // ------------------------------------------------------------------------
    // chart configuration

    struct {
        STRING *type;                               // the type of {type}.{id}
        STRING *id;                                 // the id of {type}.{id}
        STRING *name;                               // the name of {type}.{name}
    } parts;

    STRING *id;                                     // the unique ID of the rrdset as {type}.{id}
    STRING *name;                                   // the unique name of the rrdset as {type}.{name}
    STRING *family;                                 // grouping sets under the same family
    STRING *title;                                  // title shown to user
    STRING *units;                                  // units of measurement
    STRING *context;                                // the template of this data set
    STRING *plugin_name;                            // the name of the plugin that generated this
    STRING *module_name;                            // the name of the plugin module that generated this

    RRDSET_TYPE chart_type;                         // line, area, stacked

    long priority;                                  // the sorting priority of this chart

    int update_every;                               // data collection frequency

    DICTIONARY *rrdlabels;                          // chart labels
    DICTIONARY *rrdsetvar_root_index;               // chart variables
    DICTIONARY *rrddimvar_root_index;               // dimension variables
                                                    // we use this dictionary to manage their allocation

    // ------------------------------------------------------------------------
    // operational state members

    RRDSET_FLAGS flags;                             // flags
    RRD_MEMORY_MODE rrd_memory_mode;                // the db mode of this rrdset

    DICTIONARY *rrddim_root_index;                  // dimensions index

    int gap_when_lost_iterations_above;             // after how many lost iterations a gap should be stored
                                                    // netdata will interpolate values for gaps lower than this
                                                    // TODO - use the global - all charts have the same value

    STORAGE_METRICS_GROUP *storage_metrics_groups[RRD_STORAGE_TIERS];

    // ------------------------------------------------------------------------
    // linking to siblings and parents

    RRDHOST *rrdhost;                               // pointer to RRDHOST this chart belongs to

    RRDINSTANCE_ACQUIRED *rrdinstance;              // the rrdinstance of this chart
    RRDCONTEXT_ACQUIRED *rrdcontext;                // the rrdcontext this chart belongs to

    // ------------------------------------------------------------------------
    // data collection members

    size_t counter;                                 // the number of times we added values to this database
    size_t counter_done;                            // the number of times rrdset_done() has been called

    time_t last_accessed_time;                      // the last time this RRDSET has been accessed

    usec_t usec_since_last_update;                  // the time in microseconds since the last collection of data

    struct timeval last_updated;                    // when this data set was last updated (updated every time the rrd_stats_done() function)
    struct timeval last_collected_time;             // when did this data set last collected values

    size_t rrdlabels_last_saved_version;

    DICTIONARY *functions_view;                     // collector functions this rrdset supports, can be NULL

    // ------------------------------------------------------------------------
    // data collection - streaming to parents, temp variables

    time_t upstream_resync_time;                    // the timestamp up to which we should resync clock upstream

    // ------------------------------------------------------------------------
    // db mode SAVE, MAP specifics
    // TODO - they should be managed by storage engine
    //        (RRDSET_DB_STATE ptr to an undefined structure, and a call to clean this up during destruction)

    char *cache_dir;                                // the directory to store dimensions
    unsigned long memsize;                          // how much mem we have allocated for this (without dimensions)
    void *st_on_file;                               // compatibility with V019 RRDSET files

    // ------------------------------------------------------------------------
    // db mode RAM, SAVE, MAP, ALLOC, NONE specifics
    // TODO - they should be managed by storage engine
    //        (RRDSET_DB_STATE ptr to an undefined structure, and a call to clean this up during destruction)

    long entries;                                   // total number of entries in the data set

    long current_entry;                             // the entry that is currently being updated
                                                    // it goes around in a round-robin fashion

    // ------------------------------------------------------------------------
    // exporting to 3rd party time-series members
    // TODO - they should be managed by exporting engine
    //        (RRDSET_EXPORTING_STATE ptr to an undefined structure, and a call to clean this up during destruction)

    RRDSET_FLAGS *exporting_flags;                  // array of flags for exporting connector instances

    // ------------------------------------------------------------------------
    // health monitoring members
    // TODO - they should be managed by health
    //        (RRDSET_HEALTH_STATE ptr to an undefined structure, and a call to clean this up during destruction)

    NETDATA_DOUBLE green;                           // green threshold for this chart
    NETDATA_DOUBLE red;                             // red threshold for this chart

    DICTIONARY *rrdvars;                            // RRDVAR index for this chart
    const RRDFAMILY_ACQUIRED *rrdfamily;            // pointer to RRDFAMILY dictionary item, this chart belongs to

    struct {
        netdata_rwlock_t rwlock;                    // protection for RRDCALC *base
        RRDCALC *base;                              // double linked list of RRDCALC related to this RRDSET
    } alerts;
};

#define rrdset_plugin_name(st) string2str((st)->plugin_name)
#define rrdset_module_name(st) string2str((st)->module_name)
#define rrdset_units(st) string2str((st)->units)
#define rrdset_parts_type(st) string2str((st)->parts.type)
#define rrdset_family(st) string2str((st)->family)
#define rrdset_title(st) string2str((st)->title)
#define rrdset_context(st) string2str((st)->context)
#define rrdset_name(st) string2str((st)->name)
#define rrdset_id(st) string2str((st)->id)

STRING *rrd_string_strdupz(const char *s);

// ----------------------------------------------------------------------------
// these loop macros make sure the linked list is accessed with the right lock

#define rrdset_foreach_read(st, host) \
    dfe_start_read((host)->rrdset_root_index, st)

#define rrdset_foreach_write(st, host) \
    dfe_start_write((host)->rrdset_root_index, st)

#define rrdset_foreach_reentrant(st, host) \
    dfe_start_reentrant((host)->rrdset_root_index, st)

#define rrdset_foreach_done(st) \
    dfe_done(st)

#define rrdset_number_of_dimensions(st) \
    dictionary_entries((st)->rrddim_root_index)

void rrdset_memory_file_save(RRDSET *st);
void rrdset_memory_file_free(RRDSET *st);
void rrdset_memory_file_update(RRDSET *st);
const char *rrdset_cache_filename(RRDSET *st);
bool rrdset_memory_load_or_create_map_save(RRDSET *st_on_file, RRD_MEMORY_MODE memory_mode);

#include "rrdfunctions.h"

// ----------------------------------------------------------------------------
// RRDHOST flags
// use this for configuration flags, not for state control
// flags are set/unset in a manner that is not thread safe
// and may lead to missing information.

typedef enum rrdhost_flags {
    // Orphan, Archived and Obsolete flags
    RRDHOST_FLAG_ORPHAN                         = (1 << 10), // this host is orphan (not receiving data)
    RRDHOST_FLAG_ARCHIVED                       = (1 << 11), // The host is archived, no collected charts yet
    RRDHOST_FLAG_PENDING_OBSOLETE_CHARTS        = (1 << 12), // the host has pending chart obsoletions
    RRDHOST_FLAG_PENDING_OBSOLETE_DIMENSIONS    = (1 << 13), // the host has pending dimension obsoletions

    // Streaming sender
    RRDHOST_FLAG_RRDPUSH_SENDER_INITIALIZED     = (1 << 14), // the host has initialized rrdpush structures
    RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN           = (1 << 15), // When set, the sender thread is running
    RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED       = (1 << 16), // When set, the host is connected to a parent
    RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS = (1 << 17), // when set, rrdset_done() should push metrics to parent
    RRDHOST_FLAG_RRDPUSH_SENDER_LOGGED_STATUS   = (1 << 18), // when set, we have logged the status of metrics streaming
    RRDHOST_FLAG_RRDPUSH_SENDER_JOIN            = (1 << 19), // When set, we want to join the sender thread

    // Health
    RRDHOST_FLAG_PENDING_HEALTH_INITIALIZATION  = (1 << 20), // contains charts and dims with uninitialized variables
    RRDHOST_FLAG_INITIALIZED_HEALTH             = (1 << 21), // the host has initialized health structures

    // Exporting
    RRDHOST_FLAG_EXPORTING_SEND                 = (1 << 22), // send it to external databases
    RRDHOST_FLAG_EXPORTING_DONT_SEND            = (1 << 23), // don't send it to external databases

    // ACLK
    RRDHOST_FLAG_ACLK_STREAM_CONTEXTS           = (1 << 24), // when set, we should send ACLK stream context updates
    // Metadata
    RRDHOST_FLAG_METADATA_UPDATE                = (1 << 25), // metadata needs to be stored in the database
} RRDHOST_FLAGS;

#define rrdhost_flag_check(host, flag) (__atomic_load_n(&((host)->flags), __ATOMIC_SEQ_CST) & (flag))
#define rrdhost_flag_set(host, flag)   __atomic_or_fetch(&((host)->flags), flag, __ATOMIC_SEQ_CST)
#define rrdhost_flag_clear(host, flag) __atomic_and_fetch(&((host)->flags), ~(flag), __ATOMIC_SEQ_CST)

#ifdef NETDATA_INTERNAL_CHECKS
#define rrdset_debug(st, fmt, args...) do { if(unlikely(debug_flags & D_RRD_STATS && rrdset_flag_check(st, RRDSET_FLAG_DEBUG))) \
            debug_int(__FILE__, __FUNCTION__, __LINE__, "%s: " fmt, rrdset_name(st), ##args); } while(0)
#else
#define rrdset_debug(st, fmt, args...) debug_dummy()
#endif

typedef enum {
    // Indexing
    RRDHOST_OPTION_INDEXED_MACHINE_GUID     = (1 << 0), // when set, we have indexed its machine guid
    RRDHOST_OPTION_INDEXED_HOSTNAME         = (1 << 1), // when set, we have indexed its hostname

    // Streaming configuration
    RRDHOST_OPTION_SENDER_ENABLED           = (1 << 2), // set when the host is configured to send metrics to a parent

    // Configuration options
    RRDHOST_OPTION_DELETE_OBSOLETE_CHARTS   = (1 << 3), // delete files of obsolete charts
    RRDHOST_OPTION_DELETE_ORPHAN_HOST       = (1 << 4), // delete the entire host when orphan
} RRDHOST_OPTIONS;

#define rrdhost_option_check(host, flag) ((host)->options & (flag))
#define rrdhost_option_set(host, flag)   (host)->options |= flag
#define rrdhost_option_clear(host, flag) (host)->options &= ~(flag)

#define rrdhost_has_rrdpush_sender_enabled(host) (rrdhost_option_check(host, RRDHOST_OPTION_SENDER_ENABLED) && (host)->sender)

#define rrdhost_can_send_definitions_to_parent(host) (rrdhost_has_rrdpush_sender_enabled(host) && rrdhost_flag_check(host, RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED))

// ----------------------------------------------------------------------------
// Health data

struct alarm_entry {
    uint32_t unique_id;
    uint32_t alarm_id;
    uint32_t alarm_event_id;
    uuid_t config_hash_id;

    time_t when;
    time_t duration;
    time_t non_clear_duration;

    STRING *name;
    STRING *chart;
    STRING *chart_context;
    STRING *family;

    STRING *classification;
    STRING *component;
    STRING *type;

    STRING *exec;
    STRING *recipient;
    time_t exec_run_timestamp;
    int exec_code;
    uint64_t exec_spawn_serial;

    STRING *source;
    STRING *units;
    STRING *info;

    NETDATA_DOUBLE old_value;
    NETDATA_DOUBLE new_value;

    STRING *old_value_string;
    STRING *new_value_string;

    RRDCALC_STATUS old_status;
    RRDCALC_STATUS new_status;

    uint32_t flags;

    int delay;
    time_t delay_up_to_timestamp;

    uint32_t updated_by_id;
    uint32_t updates_id;

    time_t last_repeat;

    struct alarm_entry *next;
    struct alarm_entry *next_in_progress;
    struct alarm_entry *prev_in_progress;
};

#define ae_name(ae) string2str((ae)->name)
#define ae_chart_name(ae) string2str((ae)->chart)
#define ae_chart_context(ae) string2str((ae)->chart_context)
#define ae_family(ae) string2str((ae)->family)
#define ae_classification(ae) string2str((ae)->classification)
#define ae_component(ae) string2str((ae)->component)
#define ae_type(ae) string2str((ae)->type)
#define ae_exec(ae) string2str((ae)->exec)
#define ae_recipient(ae) string2str((ae)->recipient)
#define ae_source(ae) string2str((ae)->source)
#define ae_units(ae) string2str((ae)->units)
#define ae_info(ae) string2str((ae)->info)
#define ae_old_value_string(ae) string2str((ae)->old_value_string)
#define ae_new_value_string(ae) string2str((ae)->new_value_string)

typedef struct alarm_log {
    uint32_t next_log_id;
    uint32_t next_alarm_id;
    unsigned int count;
    unsigned int max;
    ALARM_ENTRY *alarms;
    netdata_rwlock_t alarm_log_rwlock;
} ALARM_LOG;


// ----------------------------------------------------------------------------
// RRD HOST

struct rrdhost_system_info {
    char *cloud_provider_type;
    char *cloud_instance_type;
    char *cloud_instance_region;

    char *host_os_name;
    char *host_os_id;
    char *host_os_id_like;
    char *host_os_version;
    char *host_os_version_id;
    char *host_os_detection;
    char *host_cores;
    char *host_cpu_freq;
    char *host_ram_total;
    char *host_disk_space;
    char *container_os_name;
    char *container_os_id;
    char *container_os_id_like;
    char *container_os_version;
    char *container_os_version_id;
    char *container_os_detection;
    char *kernel_name;
    char *kernel_version;
    char *architecture;
    char *virtualization;
    char *virt_detection;
    char *container;
    char *container_detection;
    char *is_k8s_node;
    uint16_t hops;
    bool ml_capable;
    bool ml_enabled;
    char *install_type;
    char *prebuilt_arch;
    char *prebuilt_dist;
    int mc_version;
};

struct rrdhost {
    char machine_guid[GUID_LEN + 1];                // the unique ID of this host

    // ------------------------------------------------------------------------
    // host information

    STRING *hostname;                               // the hostname of this host
    STRING *registry_hostname;                      // the registry hostname for this host
    STRING *os;                                     // the O/S type of the host
    STRING *tags;                                   // tags for this host
    STRING *timezone;                               // the timezone of the host
    STRING *abbrev_timezone;                        // the abbriviated timezone of the host
    STRING *program_name;                           // the program name that collects metrics for this host
    STRING *program_version;                        // the program version that collects metrics for this host

    int32_t utc_offset;                             // the offset in seconds from utc

    RRDHOST_OPTIONS options;                        // configuration option for this RRDHOST (no atomics on this)
    RRDHOST_FLAGS flags;                            // runtime flags about this RRDHOST (atomics on this)
    RRDHOST_FLAGS *exporting_flags;                 // array of flags for exporting connector instances

    int rrd_update_every;                           // the update frequency of the host
    long rrd_history_entries;                       // the number of history entries for the host's charts

    RRD_MEMORY_MODE rrd_memory_mode;                // the configured memory more for the charts of this host
                                                    // the actual per tier is at .db[tier].mode

    char *cache_dir;                                // the directory to save RRD cache files
    char *varlib_dir;                               // the directory to save health log

    struct {
        RRD_MEMORY_MODE mode;                       // the db mode for this tier
        STORAGE_ENGINE *eng;                        // the storage engine API for this tier
        STORAGE_INSTANCE *instance;                 // the db instance for this tier
        size_t tier_grouping;                       // tier 0 iterations aggregated on this tier
    } db[RRD_STORAGE_TIERS];

    struct rrdhost_system_info *system_info;        // information collected from the host environment

    // ------------------------------------------------------------------------
    // streaming of data to remote hosts - rrdpush sender

    char *rrdpush_send_destination;                 // where to send metrics to
    char *rrdpush_send_api_key;                     // the api key at the receiving netdata
    struct rrdpush_destinations *destinations;      // a linked list of possible destinations
    struct rrdpush_destinations *destination;       // the current destination from the above list
    SIMPLE_PATTERN *rrdpush_send_charts_matching;   // pattern to match the charts to be sent

    bool rrdpush_enable_replication;                // enable replication
    time_t rrdpush_seconds_to_replicate;            // max time we want to replicate from the child
    time_t rrdpush_replication_step;                // seconds per replication step

    // the following are state information for the threading
    // streaming metrics from this netdata to an upstream netdata
    struct sender_state *sender;
    netdata_thread_t rrdpush_sender_thread;         // the sender thread
    void *dbsync_worker;

    // ------------------------------------------------------------------------
    // streaming of data from remote hosts - rrdpush receiver

    time_t senders_connect_time;                    // the time the last sender was connected
    time_t senders_last_chart_command;              // the time of the last CHART streaming command
    time_t senders_disconnected_time;               // the time the last sender was disconnected
    int senders_count;                              // number of senders currently streaming

    struct receiver_state *receiver;
    netdata_mutex_t receiver_lock;
    int trigger_chart_obsoletion_check;             // set when child connects, will instruct parent to
                                                    // trigger a check for obsoleted charts since previous connect

    // ------------------------------------------------------------------------
    // health monitoring options

    unsigned int health_enabled;                   // 1 when this host has health enabled
    bool health_spawn;                             // true when health thread is running
    netdata_thread_t health_thread;                // the health thread
    unsigned int aclk_alert_reloaded;              // 1 on thread start and health reload, 0 after removed are sent
    time_t health_delay_up_to;                     // a timestamp to delay alarms processing up to
    STRING *health_default_exec;                   // the full path of the alarms notifications program
    STRING *health_default_recipient;              // the default recipient for all alarms
    char *health_log_filename;                     // the alarms event log filename
    size_t health_log_entries_written;             // the number of alarm events written to the alarms event log
    FILE *health_log_fp;                           // the FILE pointer to the open alarms event log file
    uint32_t health_default_warn_repeat_every;     // the default value for the interval between repeating warning notifications
    uint32_t health_default_crit_repeat_every;     // the default value for the interval between repeating critical notifications

    // all RRDCALCs are primarily allocated and linked here
    DICTIONARY *rrdcalc_root_index;

    // templates of alarms
    DICTIONARY *rrdcalctemplate_root_index;

    ALARM_LOG health_log;                           // alarms historical events (event log)
    uint32_t health_last_processed_id;              // the last processed health id from the log
    uint32_t health_max_unique_id;                  // the max alarm log unique id given for the host
    uint32_t health_max_alarm_id;                   // the max alarm id given for the host

    // ------------------------------------------------------------------------
    // locks

    netdata_rwlock_t rrdhost_rwlock;                // lock for this RRDHOST (protects rrdset_root linked list)

    // ------------------------------------------------------------------------
    // ML handle
    ml_host_t ml_host;

    // ------------------------------------------------------------------------
    // Support for host-level labels
    DICTIONARY *rrdlabels;

    // ------------------------------------------------------------------------
    // Support for functions
    DICTIONARY *functions;                          // collector functions this rrdset supports, can be NULL

    // ------------------------------------------------------------------------
    // indexes

    DICTIONARY *rrdset_root_index;                  // the host's charts index (by id)
    DICTIONARY *rrdset_root_index_name;             // the host's charts index (by name)

    DICTIONARY *rrdfamily_root_index;               // the host's chart families index
    DICTIONARY *rrdvars;                            // the host's chart variables index
                                                    // this includes custom host variables

    RRDCONTEXTS *rrdctx_hub_queue;
    RRDCONTEXTS *rrdctx_post_processing_queue;
    RRDCONTEXTS *rrdctx;

    uuid_t  host_uuid;                              // Global GUID for this host
    uuid_t  *node_id;                               // Cloud node_id

    netdata_mutex_t aclk_state_lock;
    aclk_rrdhost_state aclk_state;

    struct rrdhost *next;
    struct rrdhost *prev;
};
extern RRDHOST *localhost;

#define rrdhost_hostname(host) string2str((host)->hostname)
#define rrdhost_registry_hostname(host) string2str((host)->registry_hostname)
#define rrdhost_os(host) string2str((host)->os)
#define rrdhost_tags(host) string2str((host)->tags)
#define rrdhost_timezone(host) string2str((host)->timezone)
#define rrdhost_abbrev_timezone(host) string2str((host)->abbrev_timezone)
#define rrdhost_program_name(host) string2str((host)->program_name)
#define rrdhost_program_version(host) string2str((host)->program_version)

#define rrdhost_rdlock(host) netdata_rwlock_rdlock(&((host)->rrdhost_rwlock))
#define rrdhost_wrlock(host) netdata_rwlock_wrlock(&((host)->rrdhost_rwlock))
#define rrdhost_unlock(host) netdata_rwlock_unlock(&((host)->rrdhost_rwlock))

#define rrdhost_aclk_state_lock(host) netdata_mutex_lock(&((host)->aclk_state_lock))
#define rrdhost_aclk_state_unlock(host) netdata_mutex_unlock(&((host)->aclk_state_lock))

long rrdhost_hosts_available(void);

// ----------------------------------------------------------------------------
// these loop macros make sure the linked list is accessed with the right lock

#define rrdhost_foreach_read(var) \
    for((var) = localhost, rrd_check_rdlock(); var ; (var) = (var)->next)

#define rrdhost_foreach_write(var) \
    for((var) = localhost, rrd_check_wrlock(); var ; (var) = (var)->next)


// ----------------------------------------------------------------------------
// global lock for all RRDHOSTs

extern netdata_rwlock_t rrd_rwlock;

#define rrd_rdlock() netdata_rwlock_rdlock(&rrd_rwlock)
#define rrd_wrlock() netdata_rwlock_wrlock(&rrd_rwlock)
#define rrd_unlock() netdata_rwlock_unlock(&rrd_rwlock)

// ----------------------------------------------------------------------------

bool is_storage_engine_shared(STORAGE_INSTANCE *engine);
void rrdset_index_init(RRDHOST *host);
void rrdset_index_destroy(RRDHOST *host);

void rrddim_index_init(RRDSET *st);
void rrddim_index_destroy(RRDSET *st);

// ----------------------------------------------------------------------------

extern size_t rrd_hosts_available;
extern time_t rrdhost_free_orphan_time;

int rrd_init(char *hostname, struct rrdhost_system_info *system_info);

RRDHOST *rrdhost_find_by_hostname(const char *hostname);
RRDHOST *rrdhost_find_by_guid(const char *guid);

RRDHOST *rrdhost_find_or_create(
        const char *hostname
        , const char *registry_hostname
        , const char *guid
        , const char *os
        , const char *timezone
        , const char *abbrev_timezone
        , int32_t utc_offset
        , const char *tags
        , const char *program_name
        , const char *program_version
        , int update_every
        , long history
        , RRD_MEMORY_MODE mode
        , unsigned int health_enabled
        , unsigned int rrdpush_enabled
        , char *rrdpush_destination
        , char *rrdpush_api_key
        , char *rrdpush_send_charts_matching
        , bool rrdpush_enable_replication
        , time_t rrdpush_seconds_to_replicate
        , time_t rrdpush_replication_step
        , struct rrdhost_system_info *system_info
        , bool is_archived
);

void rrdhost_update(RRDHOST *host
    , const char *hostname
    , const char *registry_hostname
    , const char *guid
    , const char *os
    , const char *timezone
    , const char *abbrev_timezone
    , int32_t utc_offset
    , const char *tags
    , const char *program_name
    , const char *program_version
    , int update_every
    , long history
    , RRD_MEMORY_MODE mode
    , unsigned int health_enabled
    , unsigned int rrdpush_enabled
    , char *rrdpush_destination
    , char *rrdpush_api_key
    , char *rrdpush_send_charts_matching
    , bool rrdpush_enable_replication
    , time_t rrdpush_seconds_to_replicate
    , time_t rrdpush_replication_step
    , struct rrdhost_system_info *system_info
);

int rrdhost_set_system_info_variable(struct rrdhost_system_info *system_info, char *name, char *value);

#if defined(NETDATA_INTERNAL_CHECKS) && defined(NETDATA_VERIFY_LOCKS)
void __rrdhost_check_wrlock(RRDHOST *host, const char *file, const char *function, const unsigned long line);
void __rrdhost_check_rdlock(RRDHOST *host, const char *file, const char *function, const unsigned long line);
void __rrdset_check_rdlock(RRDSET *st, const char *file, const char *function, const unsigned long line);
void __rrdset_check_wrlock(RRDSET *st, const char *file, const char *function, const unsigned long line);
void __rrd_check_rdlock(const char *file, const char *function, const unsigned long line);
void __rrd_check_wrlock(const char *file, const char *function, const unsigned long line);

#define rrdhost_check_rdlock(host) __rrdhost_check_rdlock(host, __FILE__, __FUNCTION__, __LINE__)
#define rrdhost_check_wrlock(host) __rrdhost_check_wrlock(host, __FILE__, __FUNCTION__, __LINE__)
#define rrdset_check_rdlock(st) __rrdset_check_rdlock(st, __FILE__, __FUNCTION__, __LINE__)
#define rrdset_check_wrlock(st) __rrdset_check_wrlock(st, __FILE__, __FUNCTION__, __LINE__)
#define rrd_check_rdlock() __rrd_check_rdlock(__FILE__, __FUNCTION__, __LINE__)
#define rrd_check_wrlock() __rrd_check_wrlock(__FILE__, __FUNCTION__, __LINE__)

#else
#define rrdhost_check_rdlock(host) (void)0
#define rrdhost_check_wrlock(host) (void)0
#define rrdset_check_rdlock(st) (void)0
#define rrdset_check_wrlock(st) (void)0
#define rrd_check_rdlock() (void)0
#define rrd_check_wrlock() (void)0
#endif

// ----------------------------------------------------------------------------
// RRDSET functions

int rrdset_reset_name(RRDSET *st, const char *name);

RRDSET *rrdset_create_custom(RRDHOST *host
                             , const char *type
                             , const char *id
                             , const char *name
                             , const char *family
                             , const char *context
                             , const char *title
                             , const char *units
                             , const char *plugin
                             , const char *module
                             , long priority
                             , int update_every
                             , RRDSET_TYPE chart_type
                             , RRD_MEMORY_MODE memory_mode
                             , long history_entries);

#define rrdset_create(host, type, id, name, family, context, title, units, plugin, module, priority, update_every, chart_type) \
    rrdset_create_custom(host, type, id, name, family, context, title, units, plugin, module, priority, update_every, chart_type, (host)->rrd_memory_mode, (host)->rrd_history_entries)

#define rrdset_create_localhost(type, id, name, family, context, title, units, plugin, module, priority, update_every, chart_type) \
    rrdset_create(localhost, type, id, name, family, context, title, units, plugin, module, priority, update_every, chart_type)

void rrdhost_free_all(void);
void rrdhost_save_all(void);
void rrdhost_cleanup_all(void);

void rrdhost_system_info_free(struct rrdhost_system_info *system_info);
void rrdhost_free(RRDHOST *host, bool force);
void rrdhost_save_charts(RRDHOST *host);
void rrdhost_delete_charts(RRDHOST *host);

int rrdhost_should_be_removed(RRDHOST *host, RRDHOST *protected_host, time_t now);

void rrdset_update_heterogeneous_flag(RRDSET *st);

time_t rrdset_set_update_every(RRDSET *st, time_t update_every);

RRDSET *rrdset_find(RRDHOST *host, const char *id);
#define rrdset_find_localhost(id) rrdset_find(localhost, id)
/* This will not return charts that are archived */
static inline RRDSET *rrdset_find_active_localhost(const char *id)
{
    RRDSET *st = rrdset_find_localhost(id);
    if (unlikely(st && rrdset_flag_check(st, RRDSET_FLAG_ARCHIVED)))
        return NULL;
    return st;
}

RRDSET *rrdset_find_bytype(RRDHOST *host, const char *type, const char *id);
#define rrdset_find_bytype_localhost(type, id) rrdset_find_bytype(localhost, type, id)
/* This will not return charts that are archived */
static inline RRDSET *rrdset_find_active_bytype_localhost(const char *type, const char *id)
{
    RRDSET *st = rrdset_find_bytype_localhost(type, id);
    if (unlikely(st && rrdset_flag_check(st, RRDSET_FLAG_ARCHIVED)))
        return NULL;
    return st;
}

RRDSET *rrdset_find_byname(RRDHOST *host, const char *name);
#define rrdset_find_byname_localhost(name)  rrdset_find_byname(localhost, name)
/* This will not return charts that are archived */
static inline RRDSET *rrdset_find_active_byname_localhost(const char *name)
{
    RRDSET *st = rrdset_find_byname_localhost(name);
    if (unlikely(st && rrdset_flag_check(st, RRDSET_FLAG_ARCHIVED)))
        return NULL;
    return st;
}

void rrdset_next_usec_unfiltered(RRDSET *st, usec_t microseconds);
void rrdset_next_usec(RRDSET *st, usec_t microseconds);
void rrdset_timed_next(RRDSET *st, struct timeval now, usec_t microseconds);
#define rrdset_next(st) rrdset_next_usec(st, 0ULL)

void rrdset_timed_done(RRDSET *st, struct timeval now);
void rrdset_done(RRDSET *st);

void rrdset_is_obsolete(RRDSET *st);
void rrdset_isnot_obsolete(RRDSET *st);

// checks if the RRDSET should be offered to viewers
#define rrdset_is_available_for_viewers(st) (!rrdset_flag_check(st, RRDSET_FLAG_HIDDEN) && !rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE) && !rrdset_flag_check(st, RRDSET_FLAG_ARCHIVED) && rrdset_number_of_dimensions(st) && (st)->rrd_memory_mode != RRD_MEMORY_MODE_NONE)
#define rrdset_is_available_for_exporting_and_alarms(st) (!rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE) && !rrdset_flag_check(st, RRDSET_FLAG_ARCHIVED) && rrdset_number_of_dimensions(st))
#define rrdset_is_archived(st) (rrdset_flag_check(st, RRDSET_FLAG_ARCHIVED) && rrdset_number_of_dimensions(st))

time_t rrddim_first_entry_t(RRDDIM *rd);
time_t rrddim_last_entry_t(RRDDIM *rd);
time_t rrdset_last_entry_t(RRDSET *st);
time_t rrdset_first_entry_t(RRDSET *st);
time_t rrdhost_last_entry_t(RRDHOST *h);

// ----------------------------------------------------------------------------
// RRD DIMENSION functions

RRDDIM *rrddim_add_custom(RRDSET *st
                                 , const char *id
                                 , const char *name
                                 , collected_number multiplier
                                 , collected_number divisor
                                 , RRD_ALGORITHM algorithm
                                 , RRD_MEMORY_MODE memory_mode
                                 );

#define rrddim_add(st, id, name, multiplier, divisor, algorithm) \
    rrddim_add_custom(st, id, name, multiplier, divisor, algorithm, (st)->rrd_memory_mode)

int rrddim_reset_name(RRDSET *st, RRDDIM *rd, const char *name);
int rrddim_set_algorithm(RRDSET *st, RRDDIM *rd, RRD_ALGORITHM algorithm);
int rrddim_set_multiplier(RRDSET *st, RRDDIM *rd, collected_number multiplier);
int rrddim_set_divisor(RRDSET *st, RRDDIM *rd, collected_number divisor);

RRDDIM *rrddim_find(RRDSET *st, const char *id);
RRDDIM_ACQUIRED *rrddim_find_and_acquire(RRDSET *st, const char *id);
RRDDIM *rrddim_acquired_to_rrddim(RRDDIM_ACQUIRED *rda);
void rrddim_acquired_release(RRDDIM_ACQUIRED *rda);
RRDDIM *rrddim_find_active(RRDSET *st, const char *id);

int rrddim_hide(RRDSET *st, const char *id);
int rrddim_unhide(RRDSET *st, const char *id);

void rrddim_is_obsolete(RRDSET *st, RRDDIM *rd);
void rrddim_isnot_obsolete(RRDSET *st, RRDDIM *rd);

collected_number rrddim_timed_set_by_pointer(RRDSET *st, RRDDIM *rd, struct timeval collected_time, collected_number value);
collected_number rrddim_set_by_pointer(RRDSET *st, RRDDIM *rd, collected_number value);
collected_number rrddim_set(RRDSET *st, const char *id, collected_number value);

#ifdef ENABLE_ACLK
time_t calc_dimension_liveness(RRDDIM *rd, time_t now);
#endif
long align_entries_to_pagesize(RRD_MEMORY_MODE mode, long entries);

void rrddim_store_metric(RRDDIM *rd, usec_t point_end_time_ut, NETDATA_DOUBLE n, SN_FLAGS flags);

// ----------------------------------------------------------------------------
// Miscellaneous functions

char *rrdset_strncpyz_name(char *to, const char *from, size_t length);

// ----------------------------------------------------------------------------
// RRD internal functions

void rrdset_delete_files(RRDSET *st);
void rrdset_save(RRDSET *st);
void rrdset_free(RRDSET *st);

#ifdef NETDATA_RRD_INTERNALS

char *rrdset_cache_dir(RRDHOST *host, const char *id);

void rrddim_free(RRDSET *st, RRDDIM *rd);

void rrdset_reset(RRDSET *st);
void rrdset_delete_obsolete_dimensions(RRDSET *st);

RRDHOST *rrdhost_create(
    const char *hostname, const char *registry_hostname, const char *guid, const char *os, const char *timezone,
    const char *abbrev_timezone, int32_t utc_offset,const char *tags, const char *program_name, const char *program_version,
    int update_every, long entries, RRD_MEMORY_MODE memory_mode, unsigned int health_enabled, unsigned int rrdpush_enabled,
    char *rrdpush_destination, char *rrdpush_api_key, char *rrdpush_send_charts_matching,
    bool rrdpush_enable_replication, time_t rrdpush_seconds_to_replicate, time_t rrdpush_replication_step,
    struct rrdhost_system_info *system_info, int is_localhost, bool is_archived);

#endif /* NETDATA_RRD_INTERNALS */

void set_host_properties(
    RRDHOST *host, int update_every, RRD_MEMORY_MODE memory_mode, const char *registry_hostname,
    const char *os, const char *tags, const char *tzone, const char *abbrev_tzone, int32_t utc_offset,
    const char *program_name, const char *program_version);

size_t get_tier_grouping(size_t tier);

// ----------------------------------------------------------------------------
// RRD DB engine declarations

#ifdef ENABLE_DBENGINE
#include "database/engine/rrdengineapi.h"
#endif
#include "sqlite/sqlite_functions.h"
#include "sqlite/sqlite_context.h"
#include "sqlite/sqlite_metadata.h"
#include "sqlite/sqlite_aclk.h"
#include "sqlite/sqlite_aclk_alert.h"
#include "sqlite/sqlite_aclk_node.h"
#include "sqlite/sqlite_health.h"

#ifdef __cplusplus
}
#endif

#endif /* NETDATA_RRD_H */

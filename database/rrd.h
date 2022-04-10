// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRD_H
#define NETDATA_RRD_H 1

// forward typedefs
typedef struct rrdhost RRDHOST;
typedef struct rrddim RRDDIM;
typedef struct rrdset RRDSET;
typedef struct rrdvar RRDVAR;
typedef struct rrdsetvar RRDSETVAR;
typedef struct rrddimvar RRDDIMVAR;
typedef struct rrdcalc RRDCALC;
typedef struct rrdcalctemplate RRDCALCTEMPLATE;
typedef struct alarm_entry ALARM_ENTRY;
typedef struct context_param CONTEXT_PARAM;

typedef void *ml_host_t;
typedef void *ml_dimension_t;

// forward declarations
struct rrddim_volatile;
struct rrdset_volatile;
struct context_param;
struct label;
#ifdef ENABLE_DBENGINE
struct rrdeng_page_descr;
struct rrdengine_instance;
struct pg_cache_page_index;
#endif

#include "daemon/common.h"
#include "web/api/queries/query.h"
#include "rrdvar.h"
#include "rrdsetvar.h"
#include "rrddimvar.h"
#include "rrdcalc.h"
#include "rrdcalctemplate.h"
#include "streaming/rrdpush.h"
#include "aclk/aclk_rrdhost_state.h"
#include "sqlite/sqlite_health.h"

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

#define META_CHART_UPDATED 1
#define META_PLUGIN_UPDATED 2
#define META_MODULE_UPDATED 4
#define META_CHART_ACTIVATED 8

#define UPDATE_EVERY 1
#define UPDATE_EVERY_MAX 3600

#define RRD_DEFAULT_HISTORY_ENTRIES 3600
#define RRD_HISTORY_ENTRIES_MAX (86400*365)

extern int default_rrd_update_every;
extern int default_rrd_history_entries;
extern int gap_when_lost_iterations_above;
extern time_t rrdset_free_obsolete_time;

#define RRD_ID_LENGTH_MAX 200

#define RRDSET_MAGIC        "NETDATA RRD SET FILE V019"
#define RRDDIMENSION_MAGIC  "NETDATA RRD DIMENSION FILE V019"

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
    RRD_MEMORY_MODE_DBENGINE = 5
} RRD_MEMORY_MODE;

#define RRD_MEMORY_MODE_NONE_NAME "none"
#define RRD_MEMORY_MODE_RAM_NAME "ram"
#define RRD_MEMORY_MODE_MAP_NAME "map"
#define RRD_MEMORY_MODE_SAVE_NAME "save"
#define RRD_MEMORY_MODE_ALLOC_NAME "alloc"
#define RRD_MEMORY_MODE_DBENGINE_NAME "dbengine"

extern RRD_MEMORY_MODE default_rrd_memory_mode;

extern const char *rrd_memory_mode_name(RRD_MEMORY_MODE id);
extern RRD_MEMORY_MODE rrd_memory_mode_id(const char *name);


// ----------------------------------------------------------------------------
// algorithms types

typedef enum rrd_algorithm {
    RRD_ALGORITHM_ABSOLUTE              = 0,
    RRD_ALGORITHM_INCREMENTAL           = 1,
    RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL = 2,
    RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL  = 3
} RRD_ALGORITHM;

#define RRD_ALGORITHM_ABSOLUTE_NAME                "absolute"
#define RRD_ALGORITHM_INCREMENTAL_NAME             "incremental"
#define RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL_NAME   "percentage-of-incremental-row"
#define RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL_NAME    "percentage-of-absolute-row"

extern RRD_ALGORITHM rrd_algorithm_id(const char *name);
extern const char *rrd_algorithm_name(RRD_ALGORITHM algorithm);

// ----------------------------------------------------------------------------
// RRD FAMILY

struct rrdfamily {
    avl_t avl;

    const char *family;
    uint32_t hash_family;

    size_t use_count;

    avl_tree_lock rrdvar_root_index;
};
typedef struct rrdfamily RRDFAMILY;


// ----------------------------------------------------------------------------
// flags
// use this for configuration flags, not for state control
// flags are set/unset in a manner that is not thread safe
// and may lead to missing information.

typedef enum rrddim_flags {
    RRDDIM_FLAG_NONE                            = 0,
    RRDDIM_FLAG_HIDDEN                          = (1 << 0),  // this dimension will not be offered to callers
    RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS = (1 << 1),  // do not offer RESET or OVERFLOW info to callers
    RRDDIM_FLAG_OBSOLETE                        = (1 << 2),  // this is marked by the collector/module as obsolete
    // No new values have been collected for this dimension since agent start or it was marked RRDDIM_FLAG_OBSOLETE at
    // least rrdset_free_obsolete_time seconds ago.
    RRDDIM_FLAG_ARCHIVED                        = (1 << 3),
    RRDDIM_FLAG_ACLK                            = (1 << 4),

    RRDDIM_FLAG_PENDING_FOREACH_ALARM           = (1 << 5), // set when foreach alarm has not been initialized yet
} RRDDIM_FLAGS;

#ifdef HAVE_C___ATOMIC
#define rrddim_flag_check(rd, flag) (__atomic_load_n(&((rd)->flags), __ATOMIC_SEQ_CST) & (flag))
#define rrddim_flag_set(rd, flag)   __atomic_or_fetch(&((rd)->flags), (flag), __ATOMIC_SEQ_CST)
#define rrddim_flag_clear(rd, flag) __atomic_and_fetch(&((rd)->flags), ~(flag), __ATOMIC_SEQ_CST)
#else
#define rrddim_flag_check(rd, flag) ((rd)->flags & (flag))
#define rrddim_flag_set(rd, flag)   (rd)->flags |= (flag)
#define rrddim_flag_clear(rd, flag) (rd)->flags &= ~(flag)
#endif

typedef enum label_source {
    LABEL_SOURCE_AUTO             = 0,
    LABEL_SOURCE_NETDATA_CONF     = 1,
    LABEL_SOURCE_DOCKER           = 2,
    LABEL_SOURCE_ENVIRONMENT      = 3,
    LABEL_SOURCE_KUBERNETES       = 4
} LABEL_SOURCE;

#define LABEL_FLAG_UPDATE_STREAM 1
#define LABEL_FLAG_STOP_STREAM 2

struct label {
    char *key, *value;
    uint32_t key_hash;
    LABEL_SOURCE label_source;
    struct label *next;
};

struct label_index {
    struct label *head;                     // Label list
    netdata_rwlock_t labels_rwlock;         // lock for the label list
    uint32_t labels_flag;                   // Flags for labels
};

typedef enum strip_quotes {
    DO_NOT_STRIP_QUOTES,
    STRIP_QUOTES
} STRIP_QUOTES_OPTION;

typedef enum skip_escaped_characters {
    DO_NOT_SKIP_ESCAPED_CHARACTERS,
    SKIP_ESCAPED_CHARACTERS
} SKIP_ESCAPED_CHARACTERS_OPTION;

char *translate_label_source(LABEL_SOURCE l);
struct label *create_label(char *key, char *value, LABEL_SOURCE label_source);
extern struct label *add_label_to_list(struct label *l, char *key, char *value, LABEL_SOURCE label_source);
extern void update_label_list(struct label **labels, struct label *new_labels);
extern void replace_label_list(struct label_index *labels, struct label *new_labels);
extern int is_valid_label_value(char *value);
extern int is_valid_label_key(char *key);
extern void free_label_list(struct label *labels);
extern struct label *label_list_lookup_key(struct label *head, char *key, uint32_t key_hash);
extern struct label *label_list_lookup_keylist(struct label *head, char *keylist);
extern int label_list_contains_keylist(struct label *head, char *keylist);
extern int label_list_contains_key(struct label *head, char *key, uint32_t key_hash);
extern int label_list_contains(struct label *head, struct label *check);
extern struct label *merge_label_lists(struct label *lo_pri, struct label *hi_pri);
extern void strip_last_symbol(
    char *str,
    char symbol,
    SKIP_ESCAPED_CHARACTERS_OPTION skip_escaped_characters);
extern char *strip_double_quotes(char *str, SKIP_ESCAPED_CHARACTERS_OPTION skip_escaped_characters);
void reload_host_labels(void);
extern void rrdset_add_label_to_new_list(RRDSET *st, char *key, char *value, LABEL_SOURCE source);
extern void rrdset_finalize_labels(RRDSET *st);
extern void rrdset_update_labels(RRDSET *st, struct label *labels);
extern int rrdset_contains_label_keylist(RRDSET *st, char *key);
extern int rrdset_matches_label_keys(RRDSET *st, char *key, char *words[], uint32_t *hash_key_list, int *word_count, int size);
extern struct label *rrdset_lookup_label_key(RRDSET *st, char *key, uint32_t key_hash);

// ----------------------------------------------------------------------------
// RRD DIMENSION - this is a metric

struct rrddim {
    // ------------------------------------------------------------------------
    // binary indexing structures

    avl_t avl;                                      // the binary index - this has to be first member!

    // ------------------------------------------------------------------------
    // the dimension definition

    const char *id;                                 // the id of this dimension (for internal identification)
    const char *name;                               // the name of this dimension (as presented to user)
                                                    // this is a pointer to the config structure
                                                    // since the config always has a higher priority
                                                    // (the user overwrites the name of the charts)
                                                    // DO NOT FREE THIS - IT IS ALLOCATED IN CONFIG

    RRD_ALGORITHM algorithm;                        // the algorithm that is applied to add new collected values
    RRD_MEMORY_MODE rrd_memory_mode;                // the memory mode for this dimension

    collected_number multiplier;                    // the multiplier of the collected values
    collected_number divisor;                       // the divider of the collected values

    uint32_t flags;                                 // configuration flags for the dimension

    // ------------------------------------------------------------------------
    // members for temporary data we need for calculations

    uint32_t hash;                                  // a simple hash of the id, to speed up searching / indexing
                                                    // instead of strcmp() every item in the binary index
                                                    // we first compare the hashes

    uint32_t hash_name;                             // a simple hash of the name

    char *cache_filename;                           // the filename we load/save from/to this set

    size_t collections_counter;                     // the number of times we added values to this rrdim
    struct rrddim_volatile *state;                  // volatile state that is not persistently stored
    size_t unused[8];

    collected_number collected_value_max;           // the absolute maximum of the collected value

    unsigned int updated:1;                         // 1 when the dimension has been updated since the last processing
    unsigned int exposed:1;                         // 1 when set what have sent this dimension to the central netdata

    struct timeval last_collected_time;             // when was this dimension last updated
                                                    // this is actual date time we updated the last_collected_value
                                                    // THIS IS DIFFERENT FROM THE SAME MEMBER OF RRDSET

    calculated_number calculated_value;             // the current calculated value, after applying the algorithm - resets to zero after being used
    calculated_number last_calculated_value;        // the last calculated value processed

    calculated_number last_stored_value;            // the last value as stored in the database (after interpolation)

    collected_number collected_value;               // the current value, as collected - resets to 0 after being used
    collected_number last_collected_value;          // the last value that was collected, after being processed

    // the *_volume members are used to calculate the accuracy of the rounding done by the
    // storage number - they are printed to debug.log when debug is enabled for a set.
    calculated_number collected_volume;             // the sum of all collected values so far
    calculated_number stored_volume;                // the sum of all stored values so far

    struct rrddim *next;                            // linking of dimensions within the same data set
    struct rrdset *rrdset;

    // ------------------------------------------------------------------------
    // members for checking the data when loading from disk

    long entries;                                   // how many entries this dimension has in ram
                                                    // this is the same to the entries of the data set
                                                    // we set it here, to check the data when we load it from disk.

    int update_every;                               // every how many seconds is this updated

    size_t memsize;                                 // the memory allocated for this dimension

    char magic[sizeof(RRDDIMENSION_MAGIC) + 1];     // a string to be saved, used to identify our data file

    struct rrddimvar *variables;

    // ------------------------------------------------------------------------
    // the values stored in this dimension, using our floating point numbers

    storage_number values[];                        // the array of values - THIS HAS TO BE THE LAST MEMBER
};

// ----------------------------------------------------------------------------
// iterator state for RRD dimension data collection
union rrddim_collect_handle {
    struct {
        long slot;
        long entries;
    } slotted;                           // state the legacy code uses
#ifdef ENABLE_DBENGINE
    struct rrdeng_collect_handle {
        struct rrdeng_page_descr *descr, *prev_descr;
        unsigned long page_correlation_id;
        struct rrdengine_instance *ctx;
        // set to 1 when this dimension is not page aligned with the other dimensions in the chart
        uint8_t unaligned_page;
    } rrdeng; // state the database engine uses
#endif
};

// ----------------------------------------------------------------------------
// iterator state for RRD dimension data queries

#ifdef ENABLE_DBENGINE
struct rrdeng_query_handle {
    struct rrdeng_page_descr *descr;
    struct rrdengine_instance *ctx;
    struct pg_cache_page_index *page_index;
    time_t next_page_time;
    time_t now;
    unsigned position;
};
#endif

struct rrddim_query_handle {
    RRDDIM *rd;
    time_t start_time;
    time_t end_time;
    union {
        struct {
            long slot;
            long last_slot;
            uint8_t finished;
        } slotted;                         // state the legacy code uses
#ifdef ENABLE_DBENGINE
        struct rrdeng_query_handle rrdeng; // state the database engine uses
#endif
    };
};


// ----------------------------------------------------------------------------
// volatile state per RRD dimension
struct rrddim_volatile {
#ifdef ENABLE_DBENGINE
    uuid_t *rrdeng_uuid;                 // database engine metric UUID
    struct pg_cache_page_index *page_index;
#endif
#ifdef ENABLE_ACLK
    int aclk_live_status;
#endif
    uuid_t metric_uuid;                 // global UUID for this metric (unique_across hosts)
    union rrddim_collect_handle handle;
    // ------------------------------------------------------------------------
    // function pointers that handle data collection
    struct rrddim_collect_ops {
        // an initialization function to run before starting collection
        void (*init)(RRDDIM *rd);

        // run this to store each metric into the database
        void (*store_metric)(RRDDIM *rd, usec_t point_in_time, storage_number number);

        // an finalization function to run after collection is over
        // returns 1 if it's safe to delete the dimension
        int (*finalize)(RRDDIM *rd);
    } collect_ops;

    // function pointers that handle database queries
    struct rrddim_query_ops {
        // run this before starting a series of next_metric() database queries
        void (*init)(RRDDIM *rd, struct rrddim_query_handle *handle, time_t start_time, time_t end_time);

        // run this to load each metric number from the database
        storage_number (*next_metric)(struct rrddim_query_handle *handle, time_t *current_time);

        // run this to test if the series of next_metric() database queries is finished
        int (*is_finished)(struct rrddim_query_handle *handle);

        // run this after finishing a series of load_metric() database queries
        void (*finalize)(struct rrddim_query_handle *handle);

        // get the timestamp of the last entry of this metric
        time_t (*latest_time)(RRDDIM *rd);

        // get the timestamp of the first entry of this metric
        time_t (*oldest_time)(RRDDIM *rd);
    } query_ops;

    ml_dimension_t ml_dimension;
};

// ----------------------------------------------------------------------------
// volatile state per chart
struct rrdset_volatile {
    char *old_title;
    char *old_context;
    uuid_t hash_id;
    struct label *new_labels;
    struct label_index labels;
    bool is_ar_chart;
};

// ----------------------------------------------------------------------------
// these loop macros make sure the linked list is accessed with the right lock

#define rrddim_foreach_read(rd, st) \
    for((rd) = (st)->dimensions, rrdset_check_rdlock(st); (rd) ; (rd) = (rd)->next)

#define rrddim_foreach_write(rd, st) \
    for((rd) = (st)->dimensions, rrdset_check_wrlock(st); (rd) ; (rd) = (rd)->next)


// ----------------------------------------------------------------------------
// RRDSET - this is a chart

// use this for configuration flags, not for state control
// flags are set/unset in a manner that is not thread safe
// and may lead to missing information.

typedef enum rrdset_flags {
    RRDSET_FLAG_ENABLED             = 1 << 0, // enables or disables a chart
    RRDSET_FLAG_DETAIL              = 1 << 1, // if set, the data set should be considered as a detail of another
                                              // (the master data set should be the one that has the same family and is not detail)
    RRDSET_FLAG_DEBUG               = 1 << 2, // enables or disables debugging for a chart
    RRDSET_FLAG_OBSOLETE            = 1 << 3, // this is marked by the collector/module as obsolete
    RRDSET_FLAG_EXPORTING_SEND      = 1 << 4, // if set, this chart should be sent to Prometheus web API and external databases
    RRDSET_FLAG_EXPORTING_IGNORE    = 1 << 5, // if set, this chart should not be sent to Prometheus web API and external databases
    RRDSET_FLAG_UPSTREAM_SEND       = 1 << 6, // if set, this chart should be sent upstream (streaming)
    RRDSET_FLAG_UPSTREAM_IGNORE     = 1 << 7, // if set, this chart should not be sent upstream (streaming)
    RRDSET_FLAG_UPSTREAM_EXPOSED    = 1 << 8, // if set, we have sent this chart definition to netdata parent (streaming)
    RRDSET_FLAG_STORE_FIRST         = 1 << 9, // if set, do not eliminate the first collection during interpolation
    RRDSET_FLAG_HETEROGENEOUS       = 1 << 10, // if set, the chart is not homogeneous (dimensions in it have multiple algorithms, multipliers or dividers)
    RRDSET_FLAG_HOMOGENEOUS_CHECK   = 1 << 11, // if set, the chart should be checked to determine if the dimensions are homogeneous
    RRDSET_FLAG_HIDDEN              = 1 << 12, // if set, do not show this chart on the dashboard, but use it for exporting
    RRDSET_FLAG_SYNC_CLOCK          = 1 << 13, // if set, microseconds on next data collection will be ignored (the chart will be synced to now)
    RRDSET_FLAG_OBSOLETE_DIMENSIONS = 1 << 14, // this is marked by the collector/module when a chart has obsolete dimensions
    // No new values have been collected for this chart since agent start or it was marked RRDSET_FLAG_OBSOLETE at
    // least rrdset_free_obsolete_time seconds ago.
    RRDSET_FLAG_ARCHIVED            = 1 << 15,
    RRDSET_FLAG_ACLK                = 1 << 16,
    RRDSET_FLAG_PENDING_FOREACH_ALARMS = 1 << 17, // contains dims with uninitialized foreach alarms
    RRDSET_FLAG_ANOMALY_DETECTION   = 1 << 18 // flag to identify anomaly detection charts.
} RRDSET_FLAGS;

#ifdef HAVE_C___ATOMIC
#define rrdset_flag_check(st, flag) (__atomic_load_n(&((st)->flags), __ATOMIC_SEQ_CST) & (flag))
#define rrdset_flag_set(st, flag)   __atomic_or_fetch(&((st)->flags), flag, __ATOMIC_SEQ_CST)
#define rrdset_flag_clear(st, flag) __atomic_and_fetch(&((st)->flags), ~flag, __ATOMIC_SEQ_CST)
#else
#define rrdset_flag_check(st, flag) ((st)->flags & (flag))
#define rrdset_flag_set(st, flag)   (st)->flags |= (flag)
#define rrdset_flag_clear(st, flag) (st)->flags &= ~(flag)
#endif
#define rrdset_flag_check_noatomic(st, flag) ((st)->flags & (flag))

struct rrdset {
    // ------------------------------------------------------------------------
    // binary indexing structures

    avl_t avl;                                      // the index, with key the id - this has to be first!
    avl_t avlname;                                  // the index, with key the name

    // ------------------------------------------------------------------------
    // the set configuration

    char id[RRD_ID_LENGTH_MAX + 1];                 // id of the data set

    const char *name;                               // the name of this dimension (as presented to user)
                                                    // this is a pointer to the config structure
                                                    // since the config always has a higher priority
                                                    // (the user overwrites the name of the charts)

    char *config_section;                           // the config section for the chart

    char *type;                                     // the type of graph RRD_TYPE_* (a category, for determining graphing options)
    char *family;                                   // grouping sets under the same family
    char *title;                                    // title shown to user
    char *units;                                    // units of measurement

    char *context;                                  // the template of this data set
    uint32_t hash_context;                          // the hash of the chart's context

    RRDSET_TYPE chart_type;                         // line, area, stacked

    int update_every;                               // every how many seconds is this updated?

    long entries;                                   // total number of entries in the data set

    long current_entry;                             // the entry that is currently being updated
                                                    // it goes around in a round-robin fashion

    RRDSET_FLAGS flags;                             // configuration flags
    RRDSET_FLAGS *exporting_flags;                  // array of flags for exporting connector instances

    int gap_when_lost_iterations_above;             // after how many lost iterations a gap should be stored
                                                    // netdata will interpolate values for gaps lower than this

    long priority;                                  // the sorting priority of this chart


    // ------------------------------------------------------------------------
    // members for temporary data we need for calculations

    RRD_MEMORY_MODE rrd_memory_mode;                // if set to 1, this is memory mapped

    char *cache_dir;                                // the directory to store dimensions
    char cache_filename[FILENAME_MAX+1];            // the filename to store this set

    netdata_rwlock_t rrdset_rwlock;                 // protects dimensions linked list

    size_t counter;                                 // the number of times we added values to this database
    size_t counter_done;                            // the number of times rrdset_done() has been called

    union {
        time_t last_accessed_time;                  // the last time this RRDSET has been accessed
        time_t last_entry_t;                        // the last_entry_t computed for transient RRDSET
    };
    time_t upstream_resync_time;                    // the timestamp up to which we should resync clock upstream

    char *plugin_name;                              // the name of the plugin that generated this
    char *module_name;                              // the name of the plugin module that generated this
    uuid_t *chart_uuid;                             // Store the global GUID for this chart
                                                    // this object.
    struct rrdset_volatile *state;                  // volatile state that is not persistently stored
    size_t unused[3];

    size_t rrddim_page_alignment;                   // keeps metric pages in alignment when using dbengine

    uint32_t hash;                                  // a simple hash on the id, to speed up searching
                                                    // we first compare hashes, and only if the hashes are equal we do string comparisons

    uint32_t hash_name;                             // a simple hash on the name

    usec_t usec_since_last_update;                  // the time in microseconds since the last collection of data

    struct timeval last_updated;                    // when this data set was last updated (updated every time the rrd_stats_done() function)
    struct timeval last_collected_time;             // when did this data set last collected values

    total_number collected_total;                   // used internally to calculate percentages
    total_number last_collected_total;              // used internally to calculate percentages

    RRDFAMILY *rrdfamily;                           // pointer to RRDFAMILY this chart belongs to
    RRDHOST *rrdhost;                               // pointer to RRDHOST this chart belongs to

    struct rrdset *next;                            // linking of rrdsets

    // ------------------------------------------------------------------------
    // local variables

    calculated_number green;                        // green threshold for this chart
    calculated_number red;                          // red threshold for this chart

    avl_tree_lock rrdvar_root_index;                // RRDVAR index for this chart
    RRDSETVAR *variables;                           // RRDSETVAR linked list for this chart (one RRDSETVAR, many RRDVARs)
    RRDCALC *alarms;                                // RRDCALC linked list for this chart

    // ------------------------------------------------------------------------
    // members for checking the data when loading from disk

    unsigned long memsize;                          // how much mem we have allocated for this (without dimensions)

    char magic[sizeof(RRDSET_MAGIC) + 1];           // our magic

    // ------------------------------------------------------------------------
    // the dimensions

    avl_tree_lock dimensions_index;                 // the root of the dimensions index
    RRDDIM *dimensions;                             // the actual data for every dimension

};

#define rrdset_rdlock(st) netdata_rwlock_rdlock(&((st)->rrdset_rwlock))
#define rrdset_wrlock(st) netdata_rwlock_wrlock(&((st)->rrdset_rwlock))
#define rrdset_unlock(st) netdata_rwlock_unlock(&((st)->rrdset_rwlock))


// ----------------------------------------------------------------------------
// these loop macros make sure the linked list is accessed with the right lock

#define rrdset_foreach_read(st, host) \
    for((st) = (host)->rrdset_root, rrdhost_check_rdlock(host); st ; (st) = (st)->next)

#define rrdset_foreach_write(st, host) \
    for((st) = (host)->rrdset_root, rrdhost_check_wrlock(host); st ; (st) = (st)->next)


// ----------------------------------------------------------------------------
// RRDHOST flags
// use this for configuration flags, not for state control
// flags are set/unset in a manner that is not thread safe
// and may lead to missing information.

typedef enum rrdhost_flags {
    RRDHOST_FLAG_ORPHAN                 = 1 << 0, // this host is orphan (not receiving data)
    RRDHOST_FLAG_DELETE_OBSOLETE_CHARTS = 1 << 1, // delete files of obsolete charts
    RRDHOST_FLAG_DELETE_ORPHAN_HOST     = 1 << 2, // delete the entire host when orphan
    RRDHOST_FLAG_EXPORTING_SEND           = 1 << 3, // send it to external databases
    RRDHOST_FLAG_EXPORTING_DONT_SEND      = 1 << 4, // don't send it to external databases
    RRDHOST_FLAG_ARCHIVED               = 1 << 5, // The host is archived, no collected charts yet
    RRDHOST_FLAG_MULTIHOST              = 1 << 6, // Host belongs to localhost/megadb
    RRDHOST_FLAG_PENDING_FOREACH_ALARMS  = 1 << 7, // contains dims with uninitialized foreach alarms
} RRDHOST_FLAGS;

#ifdef HAVE_C___ATOMIC
#define rrdhost_flag_check(host, flag) (__atomic_load_n(&((host)->flags), __ATOMIC_SEQ_CST) & (flag))
#define rrdhost_flag_set(host, flag)   __atomic_or_fetch(&((host)->flags), flag, __ATOMIC_SEQ_CST)
#define rrdhost_flag_clear(host, flag) __atomic_and_fetch(&((host)->flags), ~flag, __ATOMIC_SEQ_CST)
#else
#define rrdhost_flag_check(host, flag) ((host)->flags & (flag))
#define rrdhost_flag_set(host, flag)   (host)->flags |= (flag)
#define rrdhost_flag_clear(host, flag) (host)->flags &= ~(flag)
#endif

#ifdef NETDATA_INTERNAL_CHECKS
#define rrdset_debug(st, fmt, args...) do { if(unlikely(debug_flags & D_RRD_STATS && rrdset_flag_check(st, RRDSET_FLAG_DEBUG))) \
            debug_int(__FILE__, __FUNCTION__, __LINE__, "%s: " fmt, st->name, ##args); } while(0)
#else
#define rrdset_debug(st, fmt, args...) debug_dummy()
#endif

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

    char *name;
    uint32_t hash_name;

    char *chart;
    uint32_t hash_chart;

    char *family;

    char *classification;
    char *component;
    char *type;

    char *exec;
    char *recipient;
    time_t exec_run_timestamp;
    int exec_code;
    uint64_t exec_spawn_serial;

    char *source;
    char *units;
    char *info;

    calculated_number old_value;
    calculated_number new_value;

    char *old_value_string;
    char *new_value_string;

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
};

struct rrdhost {
    avl_t avl;                                      // the index of hosts

    // ------------------------------------------------------------------------
    // host information

    char *hostname;                                 // the hostname of this host
    uint32_t hash_hostname;                         // the hostname hash

    char *registry_hostname;                        // the registry hostname for this host

    char machine_guid[GUID_LEN + 1];                // the unique ID of this host
    uint32_t hash_machine_guid;                     // the hash of the unique ID

    const char *os;                                 // the O/S type of the host
    const char *tags;                               // tags for this host
    const char *timezone;                           // the timezone of the host

#ifdef ENABLE_ACLK
    long    deleted_charts_count;
#endif

    const char *abbrev_timezone;                    // the abbriviated timezone of the host
    int32_t utc_offset;                             // the offset in seconds from utc

    RRDHOST_FLAGS flags;                            // flags about this RRDHOST
    RRDHOST_FLAGS *exporting_flags;                 // array of flags for exporting connector instances

    int rrd_update_every;                           // the update frequency of the host
    long rrd_history_entries;                       // the number of history entries for the host's charts
    RRD_MEMORY_MODE rrd_memory_mode;                // the memory more for the charts of this host

    char *cache_dir;                                // the directory to save RRD cache files
    char *varlib_dir;                               // the directory to save health log

    char *program_name;                             // the program name that collects metrics for this host
    char *program_version;                          // the program version that collects metrics for this host

    struct rrdhost_system_info *system_info;        // information collected from the host environment

    // ------------------------------------------------------------------------
    // streaming of data to remote hosts - rrdpush

    unsigned int rrdpush_send_enabled;            // 1 when this host sends metrics to another netdata
    char *rrdpush_send_destination;                 // where to send metrics to
    char *rrdpush_send_api_key;                     // the api key at the receiving netdata

    // the following are state information for the threading
    // streaming metrics from this netdata to an upstream netdata
    struct sender_state *sender;
    volatile unsigned int rrdpush_sender_spawn;   // 1 when the sender thread has been spawn
    netdata_thread_t rrdpush_sender_thread;         // the sender thread
    void *dbsync_worker;

    volatile unsigned int rrdpush_sender_connected; // 1 when the sender is ready to push metrics
    int rrdpush_sender_socket;                      // the fd of the socket to the remote host, or -1

    volatile unsigned int rrdpush_sender_error_shown; // 1 when we have logged a communication error
    volatile unsigned int rrdpush_sender_join;    // 1 when we have to join the sending thread

    SIMPLE_PATTERN *rrdpush_send_charts_matching;   // pattern to match the charts to be sent

    int rrdpush_sender_pipe[2];                     // collector to sender thread signaling
    //BUFFER *rrdpush_sender_buffer;                  // collector fills it, sender sends it

    //uint32_t stream_version;                             //Set the current version of the stream.

    // ------------------------------------------------------------------------
    // streaming of data from remote hosts - rrdpush

    volatile size_t connected_senders;              // when remote hosts are streaming to this
                                                    // host, this is the counter of connected clients

    time_t senders_disconnected_time;               // the time the last sender was disconnected

    struct receiver_state *receiver;
    netdata_mutex_t receiver_lock;

    // ------------------------------------------------------------------------
    // health monitoring options

    unsigned int health_enabled;                  // 1 when this host has health enabled
    time_t health_delay_up_to;                      // a timestamp to delay alarms processing up to
    char *health_default_exec;                      // the full path of the alarms notifications program
    char *health_default_recipient;                 // the default recipient for all alarms
    char *health_log_filename;                      // the alarms event log filename
    size_t health_log_entries_written;              // the number of alarm events written to the alarms event log
    FILE *health_log_fp;                            // the FILE pointer to the open alarms event log file
    uint32_t health_default_warn_repeat_every;      // the default value for the interval between repeating warning notifications
    uint32_t health_default_crit_repeat_every;      // the default value for the interval between repeating critical notifications


    // all RRDCALCs are primarily allocated and linked here
    // RRDCALCs may be linked to charts at any point
    // (charts may or may not exist when these are loaded)
    RRDCALC *alarms;
    RRDCALC *alarms_with_foreach;
    avl_tree_lock alarms_idx_health_log;
    avl_tree_lock alarms_idx_name;

    ALARM_LOG health_log;                           // alarms historical events (event log)
    uint32_t health_last_processed_id;              // the last processed health id from the log
    uint32_t health_max_unique_id;                  // the max alarm log unique id given for the host
    uint32_t health_max_alarm_id;                   // the max alarm id given for the host

    // templates of alarms
    // these are used to create alarms when charts
    // are created or renamed, that match them
    RRDCALCTEMPLATE *templates;
    RRDCALCTEMPLATE *alarms_template_with_foreach;


    // ------------------------------------------------------------------------
    // the charts of the host

    RRDSET *rrdset_root;                            // the host charts

    unsigned int obsolete_charts_count;


    // ------------------------------------------------------------------------
    // locks

    netdata_rwlock_t rrdhost_rwlock;                // lock for this RRDHOST (protects rrdset_root linked list)

    // ------------------------------------------------------------------------
    // ML handle
    ml_host_t ml_host;

    // ------------------------------------------------------------------------
    // Support for host-level labels
    struct label_index labels;

    // ------------------------------------------------------------------------
    // indexes

    avl_tree_lock rrdset_root_index;                // the host's charts index (by id)
    avl_tree_lock rrdset_root_index_name;           // the host's charts index (by name)

    avl_tree_lock rrdfamily_root_index;             // the host's chart families index
    avl_tree_lock rrdvar_root_index;                // the host's chart variables index

#ifdef ENABLE_DBENGINE
    struct rrdengine_instance *rrdeng_ctx;          // DB engine instance for this host
#endif
    uuid_t  host_uuid;                              // Global GUID for this host
    uuid_t  *node_id;                               // Cloud node_id

#ifdef ENABLE_HTTPS
    struct netdata_ssl ssl;                         //Structure used to encrypt the connection
    struct netdata_ssl stream_ssl;                         //Structure used to encrypt the stream
#endif

    netdata_mutex_t aclk_state_lock;
    aclk_rrdhost_state aclk_state;

    struct rrdhost *next;
};
extern RRDHOST *localhost;

#define rrdhost_rdlock(host) netdata_rwlock_rdlock(&((host)->rrdhost_rwlock))
#define rrdhost_wrlock(host) netdata_rwlock_wrlock(&((host)->rrdhost_rwlock))
#define rrdhost_unlock(host) netdata_rwlock_unlock(&((host)->rrdhost_rwlock))

#define rrdhost_aclk_state_lock(host) netdata_mutex_lock(&((host)->aclk_state_lock))
#define rrdhost_aclk_state_unlock(host) netdata_mutex_unlock(&((host)->aclk_state_lock))

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

extern size_t rrd_hosts_available;
extern time_t rrdhost_free_orphan_time;

extern int rrd_init(char *hostname, struct rrdhost_system_info *system_info);

extern RRDHOST *rrdhost_find_by_hostname(const char *hostname, uint32_t hash);
extern RRDHOST *rrdhost_find_by_guid(const char *guid, uint32_t hash);

extern RRDHOST *rrdhost_find_or_create(
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
        , struct rrdhost_system_info *system_info
);

extern void rrdhost_update(RRDHOST *host
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
    , struct rrdhost_system_info *system_info
);

extern int rrdhost_set_system_info_variable(struct rrdhost_system_info *system_info, char *name, char *value);

#if defined(NETDATA_INTERNAL_CHECKS) && defined(NETDATA_VERIFY_LOCKS)
extern void __rrdhost_check_wrlock(RRDHOST *host, const char *file, const char *function, const unsigned long line);
extern void __rrdhost_check_rdlock(RRDHOST *host, const char *file, const char *function, const unsigned long line);
extern void __rrdset_check_rdlock(RRDSET *st, const char *file, const char *function, const unsigned long line);
extern void __rrdset_check_wrlock(RRDSET *st, const char *file, const char *function, const unsigned long line);
extern void __rrd_check_rdlock(const char *file, const char *function, const unsigned long line);
extern void __rrd_check_wrlock(const char *file, const char *function, const unsigned long line);

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

extern int rrdset_set_name(RRDSET *st, const char *name);

extern RRDSET *rrdset_create_custom(RRDHOST *host
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

extern void rrdhost_free_all(void);
extern void rrdhost_save_all(void);
extern void rrdhost_cleanup_all(void);

extern void rrdhost_cleanup_orphan_hosts_nolock(RRDHOST *protected_host);
extern void rrdhost_system_info_free(struct rrdhost_system_info *system_info);
extern void rrdhost_free(RRDHOST *host);
extern void rrdhost_save_charts(RRDHOST *host);
extern void rrdhost_delete_charts(RRDHOST *host);
extern void rrd_cleanup_obsolete_charts();

extern int rrdhost_should_be_removed(RRDHOST *host, RRDHOST *protected_host, time_t now);

extern void rrdset_update_heterogeneous_flag(RRDSET *st);

extern RRDSET *rrdset_find(RRDHOST *host, const char *id);
#define rrdset_find_localhost(id) rrdset_find(localhost, id)
/* This will not return charts that are archived */
static inline RRDSET *rrdset_find_active_localhost(const char *id)
{
    RRDSET *st = rrdset_find_localhost(id);
    if (unlikely(st && rrdset_flag_check(st, RRDSET_FLAG_ARCHIVED)))
        return NULL;
    return st;
}

extern RRDSET *rrdset_find_bytype(RRDHOST *host, const char *type, const char *id);
#define rrdset_find_bytype_localhost(type, id) rrdset_find_bytype(localhost, type, id)
/* This will not return charts that are archived */
static inline RRDSET *rrdset_find_active_bytype_localhost(const char *type, const char *id)
{
    RRDSET *st = rrdset_find_bytype_localhost(type, id);
    if (unlikely(st && rrdset_flag_check(st, RRDSET_FLAG_ARCHIVED)))
        return NULL;
    return st;
}

extern RRDSET *rrdset_find_byname(RRDHOST *host, const char *name);
#define rrdset_find_byname_localhost(name)  rrdset_find_byname(localhost, name)
/* This will not return charts that are archived */
static inline RRDSET *rrdset_find_active_byname_localhost(const char *name)
{
    RRDSET *st = rrdset_find_byname_localhost(name);
    if (unlikely(st && rrdset_flag_check(st, RRDSET_FLAG_ARCHIVED)))
        return NULL;
    return st;
}

extern void rrdset_next_usec_unfiltered(RRDSET *st, usec_t microseconds);
extern void rrdset_next_usec(RRDSET *st, usec_t microseconds);
#define rrdset_next(st) rrdset_next_usec(st, 0ULL)

extern void rrdset_done(RRDSET *st);

extern void rrdset_is_obsolete(RRDSET *st);
extern void rrdset_isnot_obsolete(RRDSET *st);

// checks if the RRDSET should be offered to viewers
#define rrdset_is_available_for_viewers(st) (rrdset_flag_check(st, RRDSET_FLAG_ENABLED) && !rrdset_flag_check(st, RRDSET_FLAG_HIDDEN) && !rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE) && !rrdset_flag_check(st, RRDSET_FLAG_ARCHIVED) && (st)->dimensions && (st)->rrd_memory_mode != RRD_MEMORY_MODE_NONE)
#define rrdset_is_available_for_exporting_and_alarms(st) (rrdset_flag_check(st, RRDSET_FLAG_ENABLED) && !rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE) && !rrdset_flag_check(st, RRDSET_FLAG_ARCHIVED) && (st)->dimensions)
#define rrdset_is_archived(st) (rrdset_flag_check(st, RRDSET_FLAG_ARCHIVED) && (st)->dimensions)

// get the total duration in seconds of the round robin database
#define rrdset_duration(st) ((time_t)( (((st)->counter >= ((unsigned long)(st)->entries))?(unsigned long)(st)->entries:(st)->counter) * (st)->update_every ))

// get the timestamp of the last entry in the round robin database
static inline time_t rrdset_last_entry_t_nolock(RRDSET *st)
{
    if (st->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
        RRDDIM *rd;
        time_t last_entry_t  = 0;

        rrddim_foreach_read(rd, st) {
            last_entry_t = MAX(last_entry_t, rd->state->query_ops.latest_time(rd));
        }

        return last_entry_t;
    } else {
        return (time_t)st->last_updated.tv_sec;
    }
}

static inline time_t rrdset_last_entry_t(RRDSET *st)
{
    time_t last_entry_t;

    netdata_rwlock_rdlock(&st->rrdset_rwlock);
    last_entry_t = rrdset_last_entry_t_nolock(st);
    netdata_rwlock_unlock(&st->rrdset_rwlock);

    return last_entry_t;
}

// get the timestamp of first entry in the round robin database
static inline time_t rrdset_first_entry_t_nolock(RRDSET *st)
{
    if (st->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
        RRDDIM *rd;
        time_t first_entry_t = LONG_MAX;

        rrddim_foreach_read(rd, st) {
            first_entry_t =
                MIN(first_entry_t,
                    rd->state->query_ops.oldest_time(rd) > st->update_every ?
                        rd->state->query_ops.oldest_time(rd) - st->update_every : 0);
        }

        if (unlikely(LONG_MAX == first_entry_t)) return 0;
        return first_entry_t;
    } else {
        return (time_t)(rrdset_last_entry_t_nolock(st) - rrdset_duration(st));
    }
}

static inline time_t rrdset_first_entry_t(RRDSET *st)
{
    time_t first_entry_t;

    netdata_rwlock_rdlock(&st->rrdset_rwlock);
    first_entry_t = rrdset_first_entry_t_nolock(st);
    netdata_rwlock_unlock(&st->rrdset_rwlock);

    return first_entry_t;
}

// get the timestamp of the last entry in the round robin database
static inline time_t rrddim_last_entry_t(RRDDIM *rd) {
    if (rd->rrdset->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
        return rd->state->query_ops.latest_time(rd);
    return (time_t)rd->rrdset->last_updated.tv_sec;
}

static inline time_t rrddim_first_entry_t(RRDDIM *rd) {
    if (rd->rrdset->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
        return rd->state->query_ops.oldest_time(rd);
    return (time_t)(rd->rrdset->last_updated.tv_sec - rrdset_duration(rd->rrdset));
}

time_t rrdhost_last_entry_t(RRDHOST *h);

// get the last slot updated in the round robin database
#define rrdset_last_slot(st) ((size_t)(((st)->current_entry == 0) ? (st)->entries - 1 : (st)->current_entry - 1))

// get the first / oldest slot updated in the round robin database
// #define rrdset_first_slot(st) ((size_t)( (((st)->counter >= ((unsigned long)(st)->entries)) ? (unsigned long)( ((unsigned long)(st)->current_entry > 0) ? ((unsigned long)(st)->current_entry) : ((unsigned long)(st)->entries) ) - 1 : 0) ))

// return the slot that has the oldest value

static inline size_t rrdset_first_slot(RRDSET *st) {
    if(st->counter >= (size_t)st->entries) {
        // the database has been rotated at least once
        // the oldest entry is the one that will be next
        // overwritten by data collection
        return (size_t)st->current_entry;
    }

    // we do not have rotated the db yet
    // so 0 is the first entry
    return 0;
}

// get the slot of the round robin database, for the given timestamp (t)
// it always returns a valid slot, although may not be for the time requested if the time is outside the round robin database
// only valid when not using dbengine
static inline size_t rrdset_time2slot(RRDSET *st, time_t t) {
    size_t ret = 0;
    time_t last_entry_t = rrdset_last_entry_t_nolock(st);
    time_t first_entry_t = rrdset_first_entry_t_nolock(st);

    if(t >= last_entry_t) {
        // the requested time is after the last entry we have
        ret = rrdset_last_slot(st);
    }
    else {
        if(t <= first_entry_t) {
            // the requested time is before the first entry we have
            ret = rrdset_first_slot(st);
        }
        else {
            if(rrdset_last_slot(st) >= ((last_entry_t - t) / (size_t)(st->update_every)))
                ret = rrdset_last_slot(st) - ((last_entry_t - t) / (size_t)(st->update_every));
            else
                ret = rrdset_last_slot(st) - ((last_entry_t - t) / (size_t)(st->update_every)) + (unsigned long)st->entries;
        }
    }

    if(unlikely(ret >= (size_t)st->entries)) {
        error("INTERNAL ERROR: rrdset_time2slot() on %s returns values outside entries", st->name);
        ret = (size_t)(st->entries - 1);
    }

    return ret;
}

// get the timestamp of a specific slot in the round robin database
// only valid when not using dbengine
static inline time_t rrdset_slot2time(RRDSET *st, size_t slot) {
    time_t ret;
    time_t last_entry_t = rrdset_last_entry_t_nolock(st);
    time_t first_entry_t = rrdset_first_entry_t_nolock(st);

    if(slot >= (size_t)st->entries) {
        error("INTERNAL ERROR: caller of rrdset_slot2time() gives invalid slot %zu", slot);
        slot = (size_t)st->entries - 1;
    }

    if(slot > rrdset_last_slot(st)) {
        ret = last_entry_t - (size_t)st->update_every * (rrdset_last_slot(st) - slot + (size_t)st->entries);
    }
    else {
        ret = last_entry_t - (size_t)st->update_every;
    }

    if(unlikely(ret < first_entry_t)) {
        error("INTERNAL ERROR: rrdset_slot2time() on %s returns time too far in the past", st->name);
        ret = first_entry_t;
    }

    if(unlikely(ret > last_entry_t)) {
        error("INTERNAL ERROR: rrdset_slot2time() on %s returns time into the future", st->name);
        ret = last_entry_t;
    }

    return ret;
}

// ----------------------------------------------------------------------------
// RRD DIMENSION functions

extern void rrdcalc_link_to_rrddim(RRDDIM *rd, RRDSET *st, RRDHOST *host);
extern RRDDIM *rrddim_add_custom(RRDSET *st, const char *id, const char *name, collected_number multiplier,
                                 collected_number divisor, RRD_ALGORITHM algorithm, RRD_MEMORY_MODE memory_mode);//,
                                 //int is_archived, uuid_t *dim_uuid);
#define rrddim_add(st, id, name, multiplier, divisor, algorithm) rrddim_add_custom(st, id, name, multiplier, divisor, \
                                                                                   algorithm, (st)->rrd_memory_mode)//, 0, NULL)

extern int rrddim_set_name(RRDSET *st, RRDDIM *rd, const char *name);
extern int rrddim_set_algorithm(RRDSET *st, RRDDIM *rd, RRD_ALGORITHM algorithm);
extern int rrddim_set_multiplier(RRDSET *st, RRDDIM *rd, collected_number multiplier);
extern int rrddim_set_divisor(RRDSET *st, RRDDIM *rd, collected_number divisor);

extern RRDDIM *rrddim_find(RRDSET *st, const char *id);
/* This will not return dimensions that are archived */
static inline RRDDIM *rrddim_find_active(RRDSET *st, const char *id)
{
    RRDDIM *rd = rrddim_find(st, id);
    if (unlikely(rd && rrddim_flag_check(rd, RRDDIM_FLAG_ARCHIVED)))
        return NULL;
    return rd;
}


extern int rrddim_hide(RRDSET *st, const char *id);
extern int rrddim_unhide(RRDSET *st, const char *id);

extern void rrddim_is_obsolete(RRDSET *st, RRDDIM *rd);
extern void rrddim_isnot_obsolete(RRDSET *st, RRDDIM *rd);

extern collected_number rrddim_set_by_pointer(RRDSET *st, RRDDIM *rd, collected_number value);
extern collected_number rrddim_set(RRDSET *st, const char *id, collected_number value);

extern long align_entries_to_pagesize(RRD_MEMORY_MODE mode, long entries);

// ----------------------------------------------------------------------------
// Miscellaneous functions

extern int alarm_compare_id(void *a, void *b);
extern int alarm_compare_name(void *a, void *b);

// ----------------------------------------------------------------------------
// RRD internal functions

#ifdef NETDATA_RRD_INTERNALS

extern avl_tree_lock rrdhost_root_index;

extern char *rrdset_strncpyz_name(char *to, const char *from, size_t length);
extern char *rrdset_cache_dir(RRDHOST *host, const char *id, const char *config_section);

#define rrddim_free(st, rd) rrddim_free_custom(st, rd, 0)
extern void rrddim_free_custom(RRDSET *st, RRDDIM *rd, int db_rotated);

extern int rrddim_compare(void* a, void* b);
extern int rrdset_compare(void* a, void* b);
extern int rrdset_compare_name(void* a, void* b);
extern int rrdfamily_compare(void *a, void *b);

extern RRDFAMILY *rrdfamily_create(RRDHOST *host, const char *id);
extern void rrdfamily_free(RRDHOST *host, RRDFAMILY *rc);

#define rrdset_index_add(host, st) (RRDSET *)avl_insert_lock(&((host)->rrdset_root_index), (avl_t *)(st))
#define rrdset_index_del(host, st) (RRDSET *)avl_remove_lock(&((host)->rrdset_root_index), (avl_t *)(st))
extern RRDSET *rrdset_index_del_name(RRDHOST *host, RRDSET *st);

extern void rrdset_free(RRDSET *st);
extern void rrdset_reset(RRDSET *st);
extern void rrdset_save(RRDSET *st);
#define rrdset_delete(st) rrdset_delete_custom(st, 0)
extern void rrdset_delete_custom(RRDSET *st, int db_rotated);
extern void rrdset_delete_obsolete_dimensions(RRDSET *st);

extern RRDHOST *rrdhost_create(
    const char *hostname, const char *registry_hostname, const char *guid, const char *os, const char *timezone,
    const char *abbrev_timezone, int32_t utc_offset,const char *tags, const char *program_name, const char *program_version,
    int update_every, long entries, RRD_MEMORY_MODE memory_mode, unsigned int health_enabled, unsigned int rrdpush_enabled,
    char *rrdpush_destination, char *rrdpush_api_key, char *rrdpush_send_charts_matching, struct rrdhost_system_info *system_info,
    int is_localhost); //TODO: Remove , int is_archived);

#endif /* NETDATA_RRD_INTERNALS */

extern void set_host_properties(
    RRDHOST *host, int update_every, RRD_MEMORY_MODE memory_mode, const char *hostname, const char *registry_hostname,
    const char *guid, const char *os, const char *tags, const char *tzone, const char *abbrev_tzone, int32_t utc_offset,
    const char *program_name, const char *program_version);

// ----------------------------------------------------------------------------
// RRD DB engine declarations

#ifdef ENABLE_DBENGINE
#include "database/engine/rrdengineapi.h"
#endif
#include "sqlite/sqlite_functions.h"
#include "sqlite/sqlite_aclk.h"
#include "sqlite/sqlite_aclk_chart.h"
#include "sqlite/sqlite_aclk_alert.h"
#include "sqlite/sqlite_aclk_node.h"
#include "sqlite/sqlite_health.h"
#endif /* NETDATA_RRD_H */

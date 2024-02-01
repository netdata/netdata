// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRD_H
#define NETDATA_RRD_H 1

#ifdef __cplusplus
extern "C" {
#endif

#include "libnetdata/libnetdata.h"

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
typedef struct alarm_entry ALARM_ENTRY;

typedef struct rrdlabels RRDLABELS;

typedef struct rrdvar_acquired RRDVAR_ACQUIRED;
typedef struct rrdcalc_acquired RRDCALC_ACQUIRED;

typedef struct rrdhost_acquired RRDHOST_ACQUIRED;
typedef struct rrdset_acquired RRDSET_ACQUIRED;
typedef struct rrddim_acquired RRDDIM_ACQUIRED;

typedef struct ml_host rrd_ml_host_t;
typedef struct ml_chart rrd_ml_chart_t;
typedef struct ml_dimension rrd_ml_dimension_t;

typedef enum __attribute__ ((__packed__)) {
    QUERY_SOURCE_UNKNOWN = 0,
    QUERY_SOURCE_API_DATA,
    QUERY_SOURCE_API_BADGE,
    QUERY_SOURCE_API_WEIGHTS,
    QUERY_SOURCE_HEALTH,
    QUERY_SOURCE_ML,
    QUERY_SOURCE_UNITTEST,
} QUERY_SOURCE;

typedef enum __attribute__ ((__packed__)) storage_priority {
    STORAGE_PRIORITY_INTERNAL_DBENGINE = 0,
    STORAGE_PRIORITY_INTERNAL_QUERY_PREP,

    // query priorities
    STORAGE_PRIORITY_HIGH,
    STORAGE_PRIORITY_NORMAL,
    STORAGE_PRIORITY_LOW,
    STORAGE_PRIORITY_BEST_EFFORT,

    // synchronous query, not to be dispatched to workers or queued
    STORAGE_PRIORITY_SYNCHRONOUS,

    STORAGE_PRIORITY_INTERNAL_MAX_DONT_USE,
} STORAGE_PRIORITY;

// forward declarations
struct rrddim_tier;

#ifdef ENABLE_DBENGINE
struct rrdeng_page_descr;
struct rrdengine_instance;
struct pg_cache_page_index;
#endif

// ----------------------------------------------------------------------------
// memory mode

typedef enum __attribute__ ((__packed__)) rrd_memory_mode {
    RRD_MEMORY_MODE_NONE     = 0,
    RRD_MEMORY_MODE_RAM      = 1,
    RRD_MEMORY_MODE_ALLOC    = 4,
    RRD_MEMORY_MODE_DBENGINE = 5,

    // this is 8-bit
} RRD_MEMORY_MODE;

#define RRD_MEMORY_MODE_NONE_NAME "none"
#define RRD_MEMORY_MODE_RAM_NAME "ram"
#define RRD_MEMORY_MODE_ALLOC_NAME "alloc"
#define RRD_MEMORY_MODE_DBENGINE_NAME "dbengine"

extern RRD_MEMORY_MODE default_rrd_memory_mode;

const char *rrd_memory_mode_name(RRD_MEMORY_MODE id);
RRD_MEMORY_MODE rrd_memory_mode_id(const char *name);

struct ml_metrics_statistics {
    size_t anomalous;
    size_t normal;
    size_t trained;
    size_t pending;
    size_t silenced;
};

#include "daemon/common.h"
#include "web/api/queries/query.h"
#include "web/api/queries/rrdr.h"
#include "health/rrdvar.h"
#include "health/rrdcalc.h"
#include "rrdlabels.h"
#include "streaming/rrdpush.h"
#include "aclk/aclk_rrdhost_state.h"
#include "sqlite/sqlite_health.h"

typedef struct storage_query_handle STORAGE_QUERY_HANDLE;

typedef enum __attribute__ ((__packed__)) {
    STORAGE_ENGINE_BACKEND_RRDDIM = 1,
    STORAGE_ENGINE_BACKEND_DBENGINE = 2,
} STORAGE_ENGINE_BACKEND;

#define is_valid_backend(backend) ((backend) >= STORAGE_ENGINE_BACKEND_RRDDIM && (backend) <= STORAGE_ENGINE_BACKEND_DBENGINE)

// iterator state for RRD dimension data queries
struct storage_engine_query_handle {
    time_t start_time_s;
    time_t end_time_s;
    STORAGE_PRIORITY priority;
    STORAGE_ENGINE_BACKEND seb;
    STORAGE_QUERY_HANDLE *handle;
};

// ----------------------------------------------------------------------------
// chart types

typedef enum __attribute__ ((__packed__)) rrdset_type {
    RRDSET_TYPE_LINE    = 0,
    RRDSET_TYPE_AREA    = 1,
    RRDSET_TYPE_STACKED = 2,
} RRDSET_TYPE;

#define RRDSET_TYPE_LINE_NAME "line"
#define RRDSET_TYPE_AREA_NAME "area"
#define RRDSET_TYPE_STACKED_NAME "stacked"

RRDSET_TYPE rrdset_type_id(const char *name);
const char *rrdset_type_name(RRDSET_TYPE chart_type);

#include "contexts/rrdcontext.h"

extern bool unittest_running;
extern bool dbengine_enabled;
extern size_t storage_tiers;
extern bool use_direct_io;
extern size_t storage_tiers_grouping_iterations[RRD_STORAGE_TIERS];

typedef enum __attribute__ ((__packed__)) {
    RRD_BACKFILL_NONE = 0,
    RRD_BACKFILL_FULL,
    RRD_BACKFILL_NEW
} RRD_BACKFILL;

extern RRD_BACKFILL storage_tiers_backfill[RRD_STORAGE_TIERS];

#define UPDATE_EVERY 1
#define UPDATE_EVERY_MAX 3600

#define RRD_DEFAULT_HISTORY_ENTRIES 3600
#define RRD_HISTORY_ENTRIES_MAX (86400*365)

extern int default_rrd_update_every;
extern int default_rrd_history_entries;
extern int gap_when_lost_iterations_above;
extern time_t rrdset_free_obsolete_time_s;

#if defined(ENV32BIT)
#define MIN_LIBUV_WORKER_THREADS 8
#define MAX_LIBUV_WORKER_THREADS 128
#define RESERVED_LIBUV_WORKER_THREADS 3
#else
#define MIN_LIBUV_WORKER_THREADS 16
#define MAX_LIBUV_WORKER_THREADS 1024
#define RESERVED_LIBUV_WORKER_THREADS 6
#endif

extern int libuv_worker_threads;
extern bool ieee754_doubles;

#define RRD_ID_LENGTH_MAX 1000

typedef long long total_number;

// ----------------------------------------------------------------------------
// algorithms types

typedef enum __attribute__ ((__packed__)) rrd_algorithm {
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
// flags & options

// options are permanent configuration options (no atomics to alter/access them)
typedef enum __attribute__ ((__packed__)) rrddim_options {
    RRDDIM_OPTION_NONE                              = 0,
    RRDDIM_OPTION_HIDDEN                            = (1 << 0), // this dimension will not be offered to callers
    RRDDIM_OPTION_DONT_DETECT_RESETS_OR_OVERFLOWS   = (1 << 1), // do not offer RESET or OVERFLOW info to callers
    RRDDIM_OPTION_BACKFILLED_HIGH_TIERS             = (1 << 2), // when set, we have backfilled higher tiers
    RRDDIM_OPTION_UPDATED                           = (1 << 3), // single-threaded collector updated flag

    // this is 8-bit
} RRDDIM_OPTIONS;

#define rrddim_option_check(rd, option) ((rd)->collector.options & (option))
#define rrddim_option_set(rd, option)   (rd)->collector.options |= (option)
#define rrddim_option_clear(rd, option) (rd)->collector.options &= ~(option)

// flags are runtime changing status flags (atomics are required to alter/access them)
typedef enum __attribute__ ((__packed__)) rrddim_flags {
    RRDDIM_FLAG_NONE                            = 0,
    RRDDIM_FLAG_PENDING_HEALTH_INITIALIZATION   = (1 << 0),

    RRDDIM_FLAG_OBSOLETE                        = (1 << 1),  // this is marked by the collector/module as obsolete
    // No new values have been collected for this dimension since agent start, or it was marked RRDDIM_FLAG_OBSOLETE at
    // least rrdset_free_obsolete_time seconds ago.
    RRDDIM_FLAG_ARCHIVED                        = (1 << 2),
    RRDDIM_FLAG_METADATA_UPDATE                 = (1 << 3),  // Metadata needs to go to the database

    RRDDIM_FLAG_META_HIDDEN                     = (1 << 4),  // Status of hidden option in the metadata database
    RRDDIM_FLAG_ML_MODEL_LOAD                   = (1 << 5),  // Do ML LOAD for this dimension

    // this is 8 bit
} RRDDIM_FLAGS;

#define rrddim_flag_get(rd) __atomic_load_n(&((rd)->flags), __ATOMIC_ACQUIRE)
#define rrddim_flag_check(rd, flag) (__atomic_load_n(&((rd)->flags), __ATOMIC_ACQUIRE) & (flag))
#define rrddim_flag_set(rd, flag)   __atomic_or_fetch(&((rd)->flags), (flag), __ATOMIC_RELEASE)
#define rrddim_flag_clear(rd, flag) __atomic_and_fetch(&((rd)->flags), ~(flag), __ATOMIC_RELEASE)

// ----------------------------------------------------------------------------
// engine-specific iterator state for dimension data collection
typedef struct storage_collect_handle {
    STORAGE_ENGINE_BACKEND seb;
} STORAGE_COLLECT_HANDLE;

// ----------------------------------------------------------------------------
// Storage tier data for every dimension

struct rrddim_tier {
    STORAGE_POINT virtual_point;
    STORAGE_ENGINE_BACKEND seb;
    uint32_t tier_grouping;
    time_t next_point_end_time_s;
    STORAGE_METRIC_HANDLE *smh;                     // the metric handle inside the database
    STORAGE_COLLECT_HANDLE *sch;   // the data collection handle
};

void rrdr_fill_tier_gap_from_smaller_tiers(RRDDIM *rd, size_t tier, time_t now_s);

// ----------------------------------------------------------------------------
// RRD DIMENSION - this is a metric

struct rrddim {
    uuid_t metric_uuid;                             // global UUID for this metric (unique_across hosts)

    // ------------------------------------------------------------------------
    // dimension definition

    STRING *id;                                     // the id of this dimension (for internal identification)
    STRING *name;                                   // the name of this dimension (as presented to user)

    RRD_ALGORITHM algorithm;                        // the algorithm that is applied to add new collected values
    RRD_MEMORY_MODE rrd_memory_mode;                // the memory mode for this dimension
    RRDDIM_FLAGS flags;                             // run time changing status flags

    int32_t multiplier;                             // the multiplier of the collected values
    int32_t divisor;                                // the divider of the collected values

    // ------------------------------------------------------------------------
    // operational state members

    struct rrdset *rrdset;
    rrd_ml_dimension_t *ml_dimension;                   // machine learning data about this dimension

    struct {
        RRDMETRIC_ACQUIRED *rrdmetric;                  // the rrdmetric of this dimension
        bool collected;
    } rrdcontexts;

#ifdef NETDATA_LOG_COLLECTION_ERRORS
    usec_t rrddim_store_metric_last_ut;             // the timestamp we last called rrddim_store_metric()
    size_t rrddim_store_metric_count;               // the rrddim_store_metric() counter
    const char *rrddim_store_metric_last_caller;    // the name of the function that last called rrddim_store_metric()
#endif

    // ------------------------------------------------------------------------
    // db mode RAM, ALLOC, NONE specifics
    // TODO - they should be managed by storage engine
    //        (RRDDIM_DB_STATE ptr to an undefined structure, and a call to clean this up during destruction)

    struct {
        size_t memsize;                             // the memory allocated for this dimension (without RRDDIM)
        storage_number *data;                       // the array of values
    } db;

    // ------------------------------------------------------------------------
    // streaming

    struct {
        struct {
            uint32_t sent_version;
            uint32_t dim_slot;
        } sender;
    } rrdpush;

    // ------------------------------------------------------------------------
    // data collection members

    struct {
        RRDDIM_OPTIONS options;                         // permanent configuration options

        uint32_t counter;                               // the number of times we added values to this rrddim

        collected_number collected_value;               // the current value, as collected - resets to 0 after being used
        collected_number collected_value_max;           // the absolute maximum of the collected value
        collected_number last_collected_value;          // the last value that was collected, after being processed

        struct timeval last_collected_time;             // when was this dimension last updated
                                                        // this is actual date time we updated the last_collected_value
                                                        // THIS IS DIFFERENT FROM THE SAME MEMBER OF RRDSET

        NETDATA_DOUBLE calculated_value;                // the current calculated value, after applying the algorithm - resets to zero after being used
        NETDATA_DOUBLE last_calculated_value;           // the last calculated value processed

        NETDATA_DOUBLE last_stored_value;               // the last value as stored in the database (after interpolation)
    } collector;

    // ------------------------------------------------------------------------

    struct rrddim_tier tiers[];                     // our tiers of databases
};

size_t rrddim_size(void);

#define rrddim_id(rd) string2str((rd)->id)
#define rrddim_name(rd) string2str((rd) ->name)

#define rrddim_check_updated(rd) ((rd)->collector.options & RRDDIM_OPTION_UPDATED)
#define rrddim_set_updated(rd) (rd)->collector.options |= RRDDIM_OPTION_UPDATED
#define rrddim_clear_updated(rd) (rd)->collector.options &= ~RRDDIM_OPTION_UPDATED

// ------------------------------------------------------------------------
// DATA COLLECTION STORAGE OPS

STORAGE_METRICS_GROUP *rrdeng_metrics_group_get(STORAGE_INSTANCE *si, uuid_t *uuid);
STORAGE_METRICS_GROUP *rrddim_metrics_group_get(STORAGE_INSTANCE *si, uuid_t *uuid);
static inline STORAGE_METRICS_GROUP *storage_engine_metrics_group_get(STORAGE_ENGINE_BACKEND seb __maybe_unused, STORAGE_INSTANCE *si, uuid_t *uuid) {
    internal_fatal(!is_valid_backend(seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        return rrdeng_metrics_group_get(si, uuid);
#endif
    return rrddim_metrics_group_get(si, uuid);
}

void rrdeng_metrics_group_release(STORAGE_INSTANCE *si, STORAGE_METRICS_GROUP *smg);
void rrddim_metrics_group_release(STORAGE_INSTANCE *si, STORAGE_METRICS_GROUP *smg);
static inline void storage_engine_metrics_group_release(STORAGE_ENGINE_BACKEND seb __maybe_unused, STORAGE_INSTANCE *si, STORAGE_METRICS_GROUP *smg) {
    internal_fatal(!is_valid_backend(seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        rrdeng_metrics_group_release(si, smg);
    else
#endif
        rrddim_metrics_group_release(si, smg);
}

STORAGE_COLLECT_HANDLE *rrdeng_store_metric_init(STORAGE_METRIC_HANDLE *smh, uint32_t update_every, STORAGE_METRICS_GROUP *smg);
STORAGE_COLLECT_HANDLE *rrddim_collect_init(STORAGE_METRIC_HANDLE *smh, uint32_t update_every, STORAGE_METRICS_GROUP *smg);
static inline STORAGE_COLLECT_HANDLE *storage_metric_store_init(STORAGE_ENGINE_BACKEND seb __maybe_unused, STORAGE_METRIC_HANDLE *smh, uint32_t update_every, STORAGE_METRICS_GROUP *smg) {
    internal_fatal(!is_valid_backend(seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        return rrdeng_store_metric_init(smh, update_every, smg);
#endif
    return rrddim_collect_init(smh, update_every, smg);
}

void rrdeng_store_metric_next(
        STORAGE_COLLECT_HANDLE *sch, usec_t point_in_time_ut,
        NETDATA_DOUBLE n, NETDATA_DOUBLE min_value, NETDATA_DOUBLE max_value,
        uint16_t count, uint16_t anomaly_count, SN_FLAGS flags);

void rrddim_collect_store_metric(
        STORAGE_COLLECT_HANDLE *sch, usec_t point_in_time_ut,
        NETDATA_DOUBLE n, NETDATA_DOUBLE min_value, NETDATA_DOUBLE max_value,
        uint16_t count, uint16_t anomaly_count, SN_FLAGS flags);

static inline void storage_engine_store_metric(
        STORAGE_COLLECT_HANDLE *sch, usec_t point_in_time_ut,
        NETDATA_DOUBLE n, NETDATA_DOUBLE min_value, NETDATA_DOUBLE max_value,
        uint16_t count, uint16_t anomaly_count, SN_FLAGS flags) {
    internal_fatal(!is_valid_backend(sch->seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(sch->seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        return rrdeng_store_metric_next(sch, point_in_time_ut,
                                        n, min_value, max_value,
                                        count, anomaly_count, flags);
#endif
    return rrddim_collect_store_metric(sch, point_in_time_ut,
                                       n, min_value, max_value,
                                       count, anomaly_count, flags);
}

uint64_t rrdeng_disk_space_max(STORAGE_INSTANCE *si);
static inline uint64_t storage_engine_disk_space_max(STORAGE_ENGINE_BACKEND seb __maybe_unused, STORAGE_INSTANCE *si __maybe_unused) {
#ifdef ENABLE_DBENGINE
    if(likely(seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        return rrdeng_disk_space_max(si);
#endif

    return 0;
}

uint64_t rrdeng_disk_space_used(STORAGE_INSTANCE *si);
static inline uint64_t storage_engine_disk_space_used(STORAGE_ENGINE_BACKEND seb __maybe_unused, STORAGE_INSTANCE *si __maybe_unused) {
#ifdef ENABLE_DBENGINE
    if(likely(seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        return rrdeng_disk_space_used(si);
#endif

    // TODO - calculate the total host disk space for memory mode save and map
    return 0;
}

time_t rrdeng_global_first_time_s(STORAGE_INSTANCE *si);
static inline time_t storage_engine_global_first_time_s(STORAGE_ENGINE_BACKEND seb __maybe_unused, STORAGE_INSTANCE *si __maybe_unused) {
#ifdef ENABLE_DBENGINE
    if(likely(seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        return rrdeng_global_first_time_s(si);
#endif

    return now_realtime_sec() - (time_t)(default_rrd_history_entries * default_rrd_update_every);
}

size_t rrdeng_currently_collected_metrics(STORAGE_INSTANCE *si);
static inline size_t storage_engine_collected_metrics(STORAGE_ENGINE_BACKEND seb __maybe_unused, STORAGE_INSTANCE *si __maybe_unused) {
#ifdef ENABLE_DBENGINE
    if(likely(seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        return rrdeng_currently_collected_metrics(si);
#endif

    // TODO - calculate the total host disk space for memory mode save and map
    return 0;
}

void rrdeng_store_metric_flush_current_page(STORAGE_COLLECT_HANDLE *sch);
void rrddim_store_metric_flush(STORAGE_COLLECT_HANDLE *sch);
static inline void storage_engine_store_flush(STORAGE_COLLECT_HANDLE *sch) {
    if(unlikely(!sch))
        return;

    internal_fatal(!is_valid_backend(sch->seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(sch->seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        rrdeng_store_metric_flush_current_page(sch);
    else
#endif
        rrddim_store_metric_flush(sch);
}

int rrdeng_store_metric_finalize(STORAGE_COLLECT_HANDLE *sch);
int rrddim_collect_finalize(STORAGE_COLLECT_HANDLE *sch);
// a finalization function to run after collection is over
// returns 1 if it's safe to delete the dimension
static inline int storage_engine_store_finalize(STORAGE_COLLECT_HANDLE *sch) {
    internal_fatal(!is_valid_backend(sch->seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(sch->seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        return rrdeng_store_metric_finalize(sch);
#endif

    return rrddim_collect_finalize(sch);
}

void rrdeng_store_metric_change_collection_frequency(STORAGE_COLLECT_HANDLE *sch, int update_every);
void rrddim_store_metric_change_collection_frequency(STORAGE_COLLECT_HANDLE *sch, int update_every);
static inline void storage_engine_store_change_collection_frequency(STORAGE_COLLECT_HANDLE *sch, int update_every) {
    internal_fatal(!is_valid_backend(sch->seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(sch->seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        rrdeng_store_metric_change_collection_frequency(sch, update_every);
    else
#endif
        rrddim_store_metric_change_collection_frequency(sch, update_every);
}


// ----------------------------------------------------------------------------
// STORAGE ENGINE QUERY OPS

time_t rrdeng_metric_oldest_time(STORAGE_METRIC_HANDLE *smh);
time_t rrddim_query_oldest_time_s(STORAGE_METRIC_HANDLE *smh);
static inline time_t storage_engine_oldest_time_s(STORAGE_ENGINE_BACKEND seb  __maybe_unused, STORAGE_METRIC_HANDLE *smh) {
    internal_fatal(!is_valid_backend(seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        return rrdeng_metric_oldest_time(smh);
#endif
    return rrddim_query_oldest_time_s(smh);
}

time_t rrdeng_metric_latest_time(STORAGE_METRIC_HANDLE *smh);
time_t rrddim_query_latest_time_s(STORAGE_METRIC_HANDLE *smh);
static inline time_t storage_engine_latest_time_s(STORAGE_ENGINE_BACKEND seb __maybe_unused, STORAGE_METRIC_HANDLE *smh) {
    internal_fatal(!is_valid_backend(seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        return rrdeng_metric_latest_time(smh);
#endif
    return rrddim_query_latest_time_s(smh);
}

void rrdeng_load_metric_init(
        STORAGE_METRIC_HANDLE *smh, struct storage_engine_query_handle *seqh,
                time_t start_time_s, time_t end_time_s, STORAGE_PRIORITY priority);

void rrddim_query_init(
        STORAGE_METRIC_HANDLE *smh, struct storage_engine_query_handle *seqh,
                time_t start_time_s, time_t end_time_s, STORAGE_PRIORITY priority);

static inline void storage_engine_query_init(
        STORAGE_ENGINE_BACKEND seb __maybe_unused,
        STORAGE_METRIC_HANDLE *smh, struct storage_engine_query_handle *seqh,
                time_t start_time_s, time_t end_time_s, STORAGE_PRIORITY priority) {
    internal_fatal(!is_valid_backend(seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        rrdeng_load_metric_init(smh, seqh, start_time_s, end_time_s, priority);
    else
#endif
        rrddim_query_init(smh, seqh, start_time_s, end_time_s, priority);
}

STORAGE_POINT rrdeng_load_metric_next(struct storage_engine_query_handle *seqh);
STORAGE_POINT rrddim_query_next_metric(struct storage_engine_query_handle *seqh);
static inline STORAGE_POINT storage_engine_query_next_metric(struct storage_engine_query_handle *seqh) {
    internal_fatal(!is_valid_backend(seqh->seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(seqh->seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        return rrdeng_load_metric_next(seqh);
#endif
    return rrddim_query_next_metric(seqh);
}

int rrdeng_load_metric_is_finished(struct storage_engine_query_handle *seqh);
int rrddim_query_is_finished(struct storage_engine_query_handle *seqh);
static inline int storage_engine_query_is_finished(struct storage_engine_query_handle *seqh) {
    internal_fatal(!is_valid_backend(seqh->seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(seqh->seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        return rrdeng_load_metric_is_finished(seqh);
#endif
    return rrddim_query_is_finished(seqh);
}

void rrdeng_load_metric_finalize(struct storage_engine_query_handle *seqh);
void rrddim_query_finalize(struct storage_engine_query_handle *seqh);
static inline void storage_engine_query_finalize(struct storage_engine_query_handle *seqh) {
    internal_fatal(!is_valid_backend(seqh->seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(seqh->seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        rrdeng_load_metric_finalize(seqh);
    else
#endif
        rrddim_query_finalize(seqh);
}

time_t rrdeng_load_align_to_optimal_before(struct storage_engine_query_handle *seqh);
time_t rrddim_query_align_to_optimal_before(struct storage_engine_query_handle *seqh);
static inline time_t storage_engine_align_to_optimal_before(struct storage_engine_query_handle *seqh) {
    internal_fatal(!is_valid_backend(seqh->seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(seqh->seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        return rrdeng_load_align_to_optimal_before(seqh);
#endif
    return rrddim_query_align_to_optimal_before(seqh);
}

// ------------------------------------------------------------------------
// function pointers for all APIs provided by a storage engine
typedef struct storage_engine_api {
    // metric management
    STORAGE_METRIC_HANDLE *(*metric_get)(STORAGE_INSTANCE *si, uuid_t *uuid);
    STORAGE_METRIC_HANDLE *(*metric_get_or_create)(RRDDIM *rd, STORAGE_INSTANCE *si);
    void (*metric_release)(STORAGE_METRIC_HANDLE *);
    STORAGE_METRIC_HANDLE *(*metric_dup)(STORAGE_METRIC_HANDLE *);
    bool (*metric_retention_by_uuid)(STORAGE_INSTANCE *si, uuid_t *uuid, time_t *first_entry_s, time_t *last_entry_s);
} STORAGE_ENGINE_API;

typedef struct storage_engine {
    STORAGE_ENGINE_BACKEND seb;
    RRD_MEMORY_MODE id;
    const char* name;
    STORAGE_ENGINE_API api;
} STORAGE_ENGINE;

STORAGE_ENGINE* storage_engine_get(RRD_MEMORY_MODE mmode);
STORAGE_ENGINE* storage_engine_find(const char* name);

// ----------------------------------------------------------------------------
// these loop macros make sure the linked list is accessed with the right lock

#define rrddim_foreach_read(rd, st) \
    dfe_start_read((st)->rrddim_root_index, rd)
#define rrddim_foreach_done(rd) \
    dfe_done(rd)

// ----------------------------------------------------------------------------
// RRDSET - this is a chart

// use this for configuration flags, not for state control
// flags are set/unset in a manner that is not thread safe
// and may lead to missing information.

typedef enum __attribute__ ((__packed__)) rrdset_flags {
    RRDSET_FLAG_DETAIL                           = (1 << 1),  // if set, the data set should be considered as a detail of another
                                                              // (the master data set should be the one that has the same family and is not detail)
    RRDSET_FLAG_DEBUG                            = (1 << 2),  // enables or disables debugging for a chart
    RRDSET_FLAG_OBSOLETE                         = (1 << 3),  // this is marked by the collector/module as obsolete
    RRDSET_FLAG_EXPORTING_SEND                   = (1 << 4),  // if set, this chart should be sent to Prometheus web API and external databases
    RRDSET_FLAG_EXPORTING_IGNORE                 = (1 << 5),  // if set, this chart should not be sent to Prometheus web API and external databases

    RRDSET_FLAG_UPSTREAM_SEND                    = (1 << 6),  // if set, this chart should be sent upstream (streaming)
    RRDSET_FLAG_UPSTREAM_IGNORE                  = (1 << 7),  // if set, this chart should not be sent upstream (streaming)

    RRDSET_FLAG_STORE_FIRST                      = (1 << 8),  // if set, do not eliminate the first collection during interpolation
    RRDSET_FLAG_HETEROGENEOUS                    = (1 << 9),  // if set, the chart is not homogeneous (dimensions in it have multiple algorithms, multipliers or dividers)
    RRDSET_FLAG_HOMOGENEOUS_CHECK                = (1 << 10), // if set, the chart should be checked to determine if the dimensions are homogeneous
    RRDSET_FLAG_HIDDEN                           = (1 << 11), // if set, do not show this chart on the dashboard, but use it for exporting
    RRDSET_FLAG_SYNC_CLOCK                       = (1 << 12), // if set, microseconds on next data collection will be ignored (the chart will be synced to now)
    RRDSET_FLAG_OBSOLETE_DIMENSIONS              = (1 << 13), // this is marked by the collector/module when a chart has obsolete dimensions

    RRDSET_FLAG_METADATA_UPDATE                  = (1 << 14), // Mark that metadata needs to be stored
    RRDSET_FLAG_ANOMALY_DETECTION                = (1 << 15), // flag to identify anomaly detection charts.
    RRDSET_FLAG_INDEXED_ID                       = (1 << 16), // the rrdset is indexed by its id
    RRDSET_FLAG_INDEXED_NAME                     = (1 << 17), // the rrdset is indexed by its name

    RRDSET_FLAG_PENDING_HEALTH_INITIALIZATION    = (1 << 18),

    RRDSET_FLAG_SENDER_REPLICATION_IN_PROGRESS   = (1 << 19), // the sending side has replication in progress
    RRDSET_FLAG_SENDER_REPLICATION_FINISHED      = (1 << 20), // the sending side has completed replication
    RRDSET_FLAG_RECEIVER_REPLICATION_IN_PROGRESS = (1 << 21), // the receiving side has replication in progress
    RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED    = (1 << 22), // the receiving side has completed replication

    RRDSET_FLAG_UPSTREAM_SEND_VARIABLES          = (1 << 23), // a custom variable has been updated and needs to be exposed to parent

    RRDSET_FLAG_COLLECTION_FINISHED              = (1 << 24), // when set, data collection is not available for this chart

    RRDSET_FLAG_HAS_RRDCALC_LINKED               = (1 << 25), // this chart has at least one rrdcal linked
} RRDSET_FLAGS;

#define rrdset_flag_get(st) __atomic_load_n(&((st)->flags), __ATOMIC_ACQUIRE)
#define rrdset_flag_check(st, flag) (__atomic_load_n(&((st)->flags), __ATOMIC_ACQUIRE) & (flag))
#define rrdset_flag_set(st, flag)   __atomic_or_fetch(&((st)->flags), flag, __ATOMIC_RELEASE)
#define rrdset_flag_clear(st, flag) __atomic_and_fetch(&((st)->flags), ~(flag), __ATOMIC_RELEASE)

#define rrdset_is_replicating(st) (rrdset_flag_check(st, RRDSET_FLAG_SENDER_REPLICATION_IN_PROGRESS|RRDSET_FLAG_RECEIVER_REPLICATION_IN_PROGRESS) \
    && !rrdset_flag_check(st, RRDSET_FLAG_SENDER_REPLICATION_FINISHED|RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED))

struct pluginsd_rrddim {
    RRDDIM_ACQUIRED *rda;
    RRDDIM *rd;
    const char *id;
};

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

    int32_t priority;                               // the sorting priority of this chart
    int32_t update_every;                           // data collection frequency

    RRDLABELS *rrdlabels;                           // chart labels

    uint32_t version;                               // the metadata version (auto-increment)

    RRDSET_TYPE chart_type;                         // line, area, stacked

    // ------------------------------------------------------------------------
    // operational state members

    RRDSET_FLAGS flags;                             // flags
    RRD_MEMORY_MODE rrd_memory_mode;                // the db mode of this rrdset

    DICTIONARY *rrddim_root_index;                  // dimensions index

    rrd_ml_chart_t *ml_chart;

    STORAGE_METRICS_GROUP *smg[RRD_STORAGE_TIERS];

    // ------------------------------------------------------------------------
    // linking to siblings and parents

    RRDHOST *rrdhost;                               // pointer to RRDHOST this chart belongs to

    struct {
        RRDINSTANCE_ACQUIRED *rrdinstance;              // the rrdinstance of this chart
        RRDCONTEXT_ACQUIRED *rrdcontext;                // the rrdcontext this chart belongs to
        bool collected;
    } rrdcontexts;

    // ------------------------------------------------------------------------
    // data collection members

    SPINLOCK data_collection_lock;

    uint32_t counter;                               // the number of times we added values to this database
    uint32_t counter_done;                          // the number of times rrdset_done() has been called

    time_t last_accessed_time_s;                    // the last time this RRDSET has been accessed
    usec_t usec_since_last_update;                  // the time in microseconds since the last collection of data

    struct timeval last_updated;                    // when this data set was last updated (updated every time the rrd_stats_done() function)
    struct timeval last_collected_time;             // when did this data set last collected values

    size_t rrdlabels_last_saved_version;

    DICTIONARY *functions_view;                     // collector functions this rrdset supports, can be NULL

    // ------------------------------------------------------------------------
    // data collection - streaming to parents, temp variables

    struct {
        struct {
            uint32_t sent_version;
            uint32_t chart_slot;
            uint32_t dim_last_slot_used;

            time_t resync_time_s;                   // the timestamp up to which we should resync clock upstream
        } sender;
    } rrdpush;

    // ------------------------------------------------------------------------
    // db mode SAVE, MAP specifics
    // TODO - they should be managed by storage engine
    //        (RRDSET_DB_STATE ptr to an undefined structure, and a call to clean this up during destruction)

    struct {
        char *cache_dir;                                // the directory to store dimensions
        void *st_on_file;                               // compatibility with V019 RRDSET files

        int32_t entries;                                // total number of entries in the data set

        int32_t current_entry;                          // the entry that is currently being updated
        // it goes around in a round-robin fashion
    } db;

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

    struct {
        RW_SPINLOCK spinlock;                       // protection for RRDCALC *base
        RRDCALC *base;                              // double linked list of RRDCALC related to this RRDSET
    } alerts;

    struct {
        SPINLOCK spinlock; // used only for cleanup
        pid_t collector_tid;
        bool dims_with_slots;
        bool set;
        uint32_t pos;
        int32_t last_slot;
        uint32_t size;
        struct pluginsd_rrddim *prd_array;
    } pluginsd;

#ifdef NETDATA_LOG_REPLICATION_REQUESTS
    struct {
        bool log_next_data_collection;
        bool start_streaming;
        time_t after;
        time_t before;
    } replay;
#endif // NETDATA_LOG_REPLICATION_REQUESTS
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

static inline uint32_t rrdset_metadata_version(RRDSET *st) {
    return __atomic_load_n(&st->version, __ATOMIC_RELAXED);
}

static inline uint32_t rrdset_metadata_upstream_version(RRDSET *st) {
    return __atomic_load_n(&st->rrdpush.sender.sent_version, __ATOMIC_RELAXED);
}

void rrdset_metadata_updated(RRDSET *st);

static inline void rrdset_metadata_exposed_upstream(RRDSET *st, uint32_t version) {
    __atomic_store_n(&st->rrdpush.sender.sent_version, version, __ATOMIC_RELAXED);
}

static inline bool rrdset_check_upstream_exposed(RRDSET *st) {
    return rrdset_metadata_version(st) == rrdset_metadata_upstream_version(st);
}

static inline uint32_t rrddim_metadata_version(RRDDIM *rd) {
    // the metadata version of the dimension, is the version of the chart
    return rrdset_metadata_version(rd->rrdset);
}

static inline uint32_t rrddim_metadata_upstream_version(RRDDIM *rd) {
    return __atomic_load_n(&rd->rrdpush.sender.sent_version, __ATOMIC_RELAXED);
}

void rrddim_metadata_updated(RRDDIM *rd);

static inline void rrddim_metadata_exposed_upstream(RRDDIM *rd, uint32_t version) {
    __atomic_store_n(&rd->rrdpush.sender.sent_version, version, __ATOMIC_RELAXED);
}

static inline void rrddim_metadata_exposed_upstream_clear(RRDDIM *rd) {
    __atomic_store_n(&rd->rrdpush.sender.sent_version, 0, __ATOMIC_RELAXED);
}

static inline bool rrddim_check_upstream_exposed(RRDDIM *rd) {
    return rrddim_metadata_upstream_version(rd) != 0;
}

// the collector sets the exposed flag, but anyone can remove it
// still, it can be removed, after the collector has finished
// so, it is safe to check it without atomics
static inline bool rrddim_check_upstream_exposed_collector(RRDDIM *rd) {
    return rd->rrdset->version == rd->rrdpush.sender.sent_version;
}

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

#include "rrdcollector.h"
#include "rrdfunctions.h"

// ----------------------------------------------------------------------------
// RRDHOST flags
// use this for configuration flags, not for state control
// flags are set/unset in a manner that is not thread safe
// and may lead to missing information.

typedef enum __attribute__ ((__packed__)) rrdhost_flags {

    // Careful not to overlap with rrdhost_options to avoid bugs if
    // rrdhost_flags_xxx is used instead of rrdhost_option_xxx or vice-versa
    // Orphan, Archived and Obsolete flags
    RRDHOST_FLAG_ORPHAN                         = (1 << 8), // this host is orphan (not receiving data)
    RRDHOST_FLAG_ARCHIVED                       = (1 << 9), // The host is archived, no collected charts yet
    RRDHOST_FLAG_PENDING_OBSOLETE_CHARTS        = (1 << 10), // the host has pending chart obsoletions
    RRDHOST_FLAG_PENDING_OBSOLETE_DIMENSIONS    = (1 << 11), // the host has pending dimension obsoletions

    // Streaming sender
    RRDHOST_FLAG_RRDPUSH_SENDER_INITIALIZED     = (1 << 12), // the host has initialized rrdpush structures
    RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN           = (1 << 13), // When set, the sender thread is running
    RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED       = (1 << 14), // When set, the host is connected to a parent
    RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS = (1 << 15), // when set, rrdset_done() should push metrics to parent
    RRDHOST_FLAG_RRDPUSH_SENDER_LOGGED_STATUS   = (1 << 16), // when set, we have logged the status of metrics streaming

    // Health
    RRDHOST_FLAG_PENDING_HEALTH_INITIALIZATION  = (1 << 17), // contains charts and dims with uninitialized variables
    RRDHOST_FLAG_INITIALIZED_HEALTH             = (1 << 18), // the host has initialized health structures

    // Exporting
    RRDHOST_FLAG_EXPORTING_SEND                 = (1 << 19), // send it to external databases
    RRDHOST_FLAG_EXPORTING_DONT_SEND            = (1 << 20), // don't send it to external databases

    // ACLK
    RRDHOST_FLAG_ACLK_STREAM_CONTEXTS           = (1 << 21), // when set, we should send ACLK stream context updates
    RRDHOST_FLAG_ACLK_STREAM_ALERTS             = (1 << 22), // Host should stream alerts

    // Metadata
    RRDHOST_FLAG_METADATA_UPDATE                = (1 << 23), // metadata needs to be stored in the database
    RRDHOST_FLAG_METADATA_LABELS                = (1 << 24), // metadata needs to be stored in the database
    RRDHOST_FLAG_METADATA_INFO                  = (1 << 25), // metadata needs to be stored in the database
    RRDHOST_FLAG_PENDING_CONTEXT_LOAD           = (1 << 26), // Context needs to be loaded

    RRDHOST_FLAG_METADATA_CLAIMID               = (1 << 27), // metadata needs to be stored in the database
    RRDHOST_FLAG_RRDPUSH_RECEIVER_DISCONNECTED  = (1 << 28), // set when the receiver part is disconnected

    RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED       = (1 << 29), // set when the host has updated global functions
} RRDHOST_FLAGS;

#define rrdhost_flag_check(host, flag) (__atomic_load_n(&((host)->flags), __ATOMIC_SEQ_CST) & (flag))
#define rrdhost_flag_set(host, flag)   __atomic_or_fetch(&((host)->flags), flag, __ATOMIC_SEQ_CST)
#define rrdhost_flag_clear(host, flag) __atomic_and_fetch(&((host)->flags), ~(flag), __ATOMIC_SEQ_CST)

#ifdef NETDATA_INTERNAL_CHECKS
#define rrdset_debug(st, fmt, args...) do { if(unlikely(debug_flags & D_RRD_STATS && rrdset_flag_check(st, RRDSET_FLAG_DEBUG))) \
            netdata_logger(NDLS_DEBUG, NDLP_DEBUG, __FILE__, __FUNCTION__, __LINE__, "%s: " fmt, rrdset_name(st), ##args); } while(0)
#else
#define rrdset_debug(st, fmt, args...) debug_dummy()
#endif

typedef enum __attribute__ ((__packed__)) {
    // Indexing
    RRDHOST_OPTION_INDEXED_MACHINE_GUID     = (1 << 0), // when set, we have indexed its machine guid
    RRDHOST_OPTION_INDEXED_HOSTNAME         = (1 << 1), // when set, we have indexed its hostname

    // Streaming configuration
    RRDHOST_OPTION_SENDER_ENABLED           = (1 << 2), // set when the host is configured to send metrics to a parent

    // Configuration options
    RRDHOST_OPTION_DELETE_OBSOLETE_CHARTS   = (1 << 3), // delete files of obsolete charts
    RRDHOST_OPTION_DELETE_ORPHAN_HOST       = (1 << 4), // delete the entire host when orphan

    RRDHOST_OPTION_REPLICATION              = (1 << 5), // when set, we support replication for this host

    RRDHOST_OPTION_VIRTUAL_HOST             = (1 << 6), // when set, this host is a virtual one
    RRDHOST_OPTION_EPHEMERAL_HOST           = (1 << 7), // when set, this host is an ephemeral one
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
    usec_t global_id;
    uuid_t config_hash_id;
    uuid_t transition_id;

    time_t when;
    time_t duration;
    time_t non_clear_duration;

    STRING *name;
    STRING *chart;
    STRING *chart_context;
    STRING *chart_name;

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
    STRING *summary;
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
#define ae_chart_id(ae) string2str((ae)->chart)
#define ae_chart_name(ae) string2str((ae)->chart_name)
#define ae_chart_context(ae) string2str((ae)->chart_context)
#define ae_classification(ae) string2str((ae)->classification)
#define ae_exec(ae) string2str((ae)->exec)
#define ae_recipient(ae) string2str((ae)->recipient)
#define ae_source(ae) string2str((ae)->source)
#define ae_units(ae) string2str((ae)->units)
#define ae_summary(ae) string2str((ae)->summary)
#define ae_info(ae) string2str((ae)->info)
#define ae_old_value_string(ae) string2str((ae)->old_value_string)
#define ae_new_value_string(ae) string2str((ae)->new_value_string)

typedef struct alarm_log {
    uint32_t next_log_id;
    uint32_t next_alarm_id;
    unsigned int count;
    unsigned int max;
    uint32_t health_log_history;                   // the health log history in seconds to be kept in db
    ALARM_ENTRY *alarms;
    RW_SPINLOCK spinlock;
} ALARM_LOG;

typedef struct health {
    time_t health_delay_up_to;                     // a timestamp to delay alarms processing up to
    STRING *health_default_exec;                   // the full path of the alarms notifications program
    STRING *health_default_recipient;              // the default recipient for all alarms
    uint32_t health_default_warn_repeat_every;     // the default value for the interval between repeating warning notifications
    uint32_t health_default_crit_repeat_every;     // the default value for the interval between repeating critical notifications
    unsigned int health_enabled;                   // 1 when this host has health enabled
    bool use_summary_for_notifications;            // whether or not to use the summary field as a subject for notifications
} HEALTH;

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

struct rrdhost_system_info *rrdhost_labels_to_system_info(RRDLABELS *labels);

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

    int32_t rrd_update_every;                       // the update frequency of the host
    int32_t rrd_history_entries;                    // the number of history entries for the host's charts

    RRD_MEMORY_MODE rrd_memory_mode;                // the configured memory more for the charts of this host
                                                    // the actual per tier is at .db[tier].mode

    char *cache_dir;                                // the directory to save RRD cache files

    struct {
        RRD_MEMORY_MODE mode;                       // the db mode for this tier
        STORAGE_ENGINE *eng;                        // the storage engine API for this tier
        STORAGE_INSTANCE *si;                       // the db instance for this tier
        uint32_t tier_grouping;                     // tier 0 iterations aggregated on this tier
    } db[RRD_STORAGE_TIERS];

    struct rrdhost_system_info *system_info;        // information collected from the host environment

    // ------------------------------------------------------------------------
    // streaming of data to remote hosts - rrdpush sender

    struct {
        struct {
            struct {
                struct {
                    SPINLOCK spinlock;

                    bool ignore;                    // when set, freeing slots will not put them in the available
                    uint32_t used;
                    uint32_t size;
                    uint32_t *array;
                } available;                        // keep track of the available chart slots per host

                uint32_t last_used;                 // the last slot we used for a chart (increments only)
            } pluginsd_chart_slots;
        } send;

        struct {
            struct {
                SPINLOCK spinlock;                  // lock for the management of the allocation
                uint32_t size;
                RRDSET **array;
            } pluginsd_chart_slots;
        } receive;
    } rrdpush;

    char *rrdpush_send_destination;                 // where to send metrics to
    char *rrdpush_send_api_key;                     // the api key at the receiving netdata
    struct rrdpush_destinations *destinations;      // a linked list of possible destinations
    struct rrdpush_destinations *destination;       // the current destination from the above list
    SIMPLE_PATTERN *rrdpush_send_charts_matching;   // pattern to match the charts to be sent

    int32_t rrdpush_last_receiver_exit_reason;
    time_t rrdpush_seconds_to_replicate;            // max time we want to replicate from the child
    time_t rrdpush_replication_step;                // seconds per replication step
    size_t rrdpush_receiver_replicating_charts;     // the number of charts currently being replicated from a child
    NETDATA_DOUBLE rrdpush_receiver_replication_percent; // the % of replication completion

    // the following are state information for the threading
    // streaming metrics from this netdata to an upstream netdata
    struct sender_state *sender;
    netdata_thread_t rrdpush_sender_thread;         // the sender thread
    size_t rrdpush_sender_replicating_charts;       // the number of charts currently being replicated to a parent
    struct aclk_sync_cfg_t *aclk_config;

    uint32_t rrdpush_receiver_connection_counter;   // the number of times this receiver has connected
    uint32_t rrdpush_sender_connection_counter;     // the number of times this sender has connected

    // ------------------------------------------------------------------------
    // streaming of data from remote hosts - rrdpush receiver

    time_t last_connected;                          // last time child connected (stored in db)
    time_t child_connect_time;                      // the time the last sender was connected
    time_t child_last_chart_command;                // the time of the last CHART streaming command
    time_t child_disconnected_time;                 // the time the last sender was disconnected
    int connected_children_count;                   // number of senders currently streaming

    struct receiver_state *receiver;
    netdata_mutex_t receiver_lock;
    int trigger_chart_obsoletion_check;             // set when child connects, will instruct parent to
                                                    // trigger a check for obsoleted charts since previous connect

    // ------------------------------------------------------------------------
    // health monitoring options

    // health variables
    HEALTH health;

    // all RRDCALCs are primarily allocated and linked here
    DICTIONARY *rrdcalc_root_index;

    ALARM_LOG health_log;                           // alarms historical events (event log)
    uint32_t health_last_processed_id;              // the last processed health id from the log
    uint32_t health_max_unique_id;                  // the max alarm log unique id given for the host
    uint32_t health_max_alarm_id;                   // the max alarm id given for the host
    size_t health_transitions;                      // the number of times an alert changed state

    // ------------------------------------------------------------------------
    // locks

    SPINLOCK rrdhost_update_lock;

    // ------------------------------------------------------------------------
    // ML handle
    rrd_ml_host_t *ml_host;

    // ------------------------------------------------------------------------
    // Support for host-level labels
    RRDLABELS *rrdlabels;

    // ------------------------------------------------------------------------
    // Support for functions
    DICTIONARY *functions;                          // collector functions this rrdset supports, can be NULL

    // ------------------------------------------------------------------------
    // indexes

    DICTIONARY *rrdset_root_index;                  // the host's charts index (by id)
    DICTIONARY *rrdset_root_index_name;             // the host's charts index (by name)

    DICTIONARY *rrdvars;                            // the host's chart variables index
                                                    // this includes custom host variables

    struct {
        DICTIONARY *contexts;
        DICTIONARY *hub_queue;
        DICTIONARY *pp_queue;
        uint32_t metrics;
        uint32_t instances;
    } rrdctx;

    struct {
        SPINLOCK spinlock;
        time_t first_time_s;
        time_t last_time_s;
    } retention;

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

#define rrdhost_aclk_state_lock(host) netdata_mutex_lock(&((host)->aclk_state_lock))
#define rrdhost_aclk_state_unlock(host) netdata_mutex_unlock(&((host)->aclk_state_lock))

#define rrdhost_receiver_replicating_charts(host) (__atomic_load_n(&((host)->rrdpush_receiver_replicating_charts), __ATOMIC_RELAXED))
#define rrdhost_receiver_replicating_charts_plus_one(host) (__atomic_add_fetch(&((host)->rrdpush_receiver_replicating_charts), 1, __ATOMIC_RELAXED))
#define rrdhost_receiver_replicating_charts_minus_one(host) (__atomic_sub_fetch(&((host)->rrdpush_receiver_replicating_charts), 1, __ATOMIC_RELAXED))
#define rrdhost_receiver_replicating_charts_zero(host) (__atomic_store_n(&((host)->rrdpush_receiver_replicating_charts), 0, __ATOMIC_RELAXED))

#define rrdhost_sender_replicating_charts(host) (__atomic_load_n(&((host)->rrdpush_sender_replicating_charts), __ATOMIC_RELAXED))
#define rrdhost_sender_replicating_charts_plus_one(host) (__atomic_add_fetch(&((host)->rrdpush_sender_replicating_charts), 1, __ATOMIC_RELAXED))
#define rrdhost_sender_replicating_charts_minus_one(host) (__atomic_sub_fetch(&((host)->rrdpush_sender_replicating_charts), 1, __ATOMIC_RELAXED))
#define rrdhost_sender_replicating_charts_zero(host) (__atomic_store_n(&((host)->rrdpush_sender_replicating_charts), 0, __ATOMIC_RELAXED))

#define rrdhost_is_online(host) ((host) == localhost || rrdhost_option_check(host, RRDHOST_OPTION_VIRTUAL_HOST) || !rrdhost_flag_check(host, RRDHOST_FLAG_ORPHAN | RRDHOST_FLAG_RRDPUSH_RECEIVER_DISCONNECTED))
bool rrdhost_matches_window(RRDHOST *host, time_t after, time_t before, time_t now);

extern DICTIONARY *rrdhost_root_index;
size_t rrdhost_hosts_available(void);

RRDHOST_ACQUIRED *rrdhost_find_and_acquire(const char *machine_guid);
RRDHOST *rrdhost_acquired_to_rrdhost(RRDHOST_ACQUIRED *rha);
void rrdhost_acquired_release(RRDHOST_ACQUIRED *rha);

// ----------------------------------------------------------------------------

#define rrdhost_foreach_read(var) \
    for((var) = localhost; var ; (var) = (var)->next)

#define rrdhost_foreach_write(var) \
    for((var) = localhost; var ; (var) = (var)->next)


// ----------------------------------------------------------------------------
// global lock for all RRDHOSTs

extern netdata_rwlock_t rrd_rwlock;

#define rrd_rdlock() netdata_rwlock_rdlock(&rrd_rwlock)
#define rrd_wrlock() netdata_rwlock_wrlock(&rrd_rwlock)
#define rrd_unlock() netdata_rwlock_unlock(&rrd_rwlock)

// ----------------------------------------------------------------------------

bool is_storage_engine_shared(STORAGE_INSTANCE *si);
void rrdset_index_init(RRDHOST *host);
void rrdset_index_destroy(RRDHOST *host);

void rrddim_index_init(RRDSET *st);
void rrddim_index_destroy(RRDSET *st);

// ----------------------------------------------------------------------------

extern time_t rrdhost_free_orphan_time_s;
extern time_t rrdhost_free_ephemeral_time_s;

int rrd_init(char *hostname, struct rrdhost_system_info *system_info, bool unittest);

RRDHOST *rrdhost_find_by_hostname(const char *hostname);
RRDHOST *rrdhost_find_by_guid(const char *guid);
RRDHOST *find_host_by_node_id(char *node_id);

RRDHOST *rrdhost_find_or_create(
    const char *hostname,
    const char *registry_hostname,
    const char *guid,
    const char *os,
    const char *timezone,
    const char *abbrev_timezone,
    int32_t utc_offset,
    const char *tags,
    const char *prog_name,
    const char *prog_version,
    int update_every,
    long history,
    RRD_MEMORY_MODE mode,
    unsigned int health_enabled,
    unsigned int rrdpush_enabled,
    char *rrdpush_destination,
    char *rrdpush_api_key,
    char *rrdpush_send_charts_matching,
    bool rrdpush_enable_replication,
    time_t rrdpush_seconds_to_replicate,
    time_t rrdpush_replication_step,
    struct rrdhost_system_info *system_info,
    bool is_archived);

int rrdhost_set_system_info_variable(struct rrdhost_system_info *system_info, char *name, char *value);

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

void rrdhost_system_info_free(struct rrdhost_system_info *system_info);
void rrdhost_free___while_having_rrd_wrlock(RRDHOST *host, bool force);

int rrdhost_should_be_removed(RRDHOST *host, RRDHOST *protected_host, time_t now_s);

void rrdset_update_heterogeneous_flag(RRDSET *st);

time_t rrdset_set_update_every_s(RRDSET *st, time_t update_every_s);

RRDSET *rrdset_find(RRDHOST *host, const char *id);

RRDSET_ACQUIRED *rrdset_find_and_acquire(RRDHOST *host, const char *id);
RRDSET *rrdset_acquired_to_rrdset(RRDSET_ACQUIRED *rsa);
void rrdset_acquired_release(RRDSET_ACQUIRED *rsa);

#define rrdset_find_localhost(id) rrdset_find(localhost, id)
/* This will not return charts that are archived */
static inline RRDSET *rrdset_find_active_localhost(const char *id)
{
    RRDSET *st = rrdset_find_localhost(id);
    return st;
}

RRDSET *rrdset_find_bytype(RRDHOST *host, const char *type, const char *id);
#define rrdset_find_bytype_localhost(type, id) rrdset_find_bytype(localhost, type, id)
/* This will not return charts that are archived */
static inline RRDSET *rrdset_find_active_bytype_localhost(const char *type, const char *id)
{
    RRDSET *st = rrdset_find_bytype_localhost(type, id);
    return st;
}

RRDSET *rrdset_find_byname(RRDHOST *host, const char *name);
#define rrdset_find_byname_localhost(name)  rrdset_find_byname(localhost, name)
/* This will not return charts that are archived */
static inline RRDSET *rrdset_find_active_byname_localhost(const char *name)
{
    RRDSET *st = rrdset_find_byname_localhost(name);
    return st;
}

void rrdset_next_usec_unfiltered(RRDSET *st, usec_t microseconds);
void rrdset_next_usec(RRDSET *st, usec_t microseconds);
void rrdset_timed_next(RRDSET *st, struct timeval now, usec_t microseconds);
#define rrdset_next(st) rrdset_next_usec(st, 0ULL)

void rrdset_timed_done(RRDSET *st, struct timeval now, bool pending_rrdset_next);
void rrdset_done(RRDSET *st);

void rrdset_is_obsolete___safe_from_collector_thread(RRDSET *st);
void rrdset_isnot_obsolete___safe_from_collector_thread(RRDSET *st);

// checks if the RRDSET should be offered to viewers
#define rrdset_is_available_for_viewers(st) (!rrdset_flag_check(st, RRDSET_FLAG_HIDDEN) && !rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE) && rrdset_number_of_dimensions(st) && (st)->rrd_memory_mode != RRD_MEMORY_MODE_NONE)
#define rrdset_is_available_for_exporting_and_alarms(st) (!rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE) && rrdset_number_of_dimensions(st))

time_t rrddim_first_entry_s(RRDDIM *rd);
time_t rrddim_first_entry_s_of_tier(RRDDIM *rd, size_t tier);
time_t rrddim_last_entry_s(RRDDIM *rd);
time_t rrddim_last_entry_s_of_tier(RRDDIM *rd, size_t tier);

time_t rrdset_first_entry_s(RRDSET *st);
time_t rrdset_first_entry_s_of_tier(RRDSET *st, size_t tier);
time_t rrdset_last_entry_s(RRDSET *st);
time_t rrdset_last_entry_s_of_tier(RRDSET *st, size_t tier);

void rrdset_get_retention_of_tier_for_collected_chart(RRDSET *st, time_t *first_time_s, time_t *last_time_s, time_t now_s, size_t tier);

void rrdset_update_rrdlabels(RRDSET *st, RRDLABELS *new_rrdlabels);

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
int rrddim_set_multiplier(RRDSET *st, RRDDIM *rd, int32_t multiplier);
int rrddim_set_divisor(RRDSET *st, RRDDIM *rd, int32_t divisor);

RRDDIM *rrddim_find(RRDSET *st, const char *id);
RRDDIM_ACQUIRED *rrddim_find_and_acquire(RRDSET *st, const char *id);
RRDDIM *rrddim_acquired_to_rrddim(RRDDIM_ACQUIRED *rda);
void rrddim_acquired_release(RRDDIM_ACQUIRED *rda);
RRDDIM *rrddim_find_active(RRDSET *st, const char *id);

int rrddim_hide(RRDSET *st, const char *id);
int rrddim_unhide(RRDSET *st, const char *id);

void rrddim_is_obsolete___safe_from_collector_thread(RRDSET *st, RRDDIM *rd);
void rrddim_isnot_obsolete___safe_from_collector_thread(RRDSET *st, RRDDIM *rd);

collected_number rrddim_timed_set_by_pointer(RRDSET *st, RRDDIM *rd, struct timeval collected_time, collected_number value);
collected_number rrddim_set_by_pointer(RRDSET *st, RRDDIM *rd, collected_number value);
collected_number rrddim_set(RRDSET *st, const char *id, collected_number value);

bool rrddim_finalize_collection_and_check_retention(RRDDIM *rd);
void rrdset_finalize_collection(RRDSET *st, bool dimensions_too);
void rrdhost_finalize_collection(RRDHOST *host);
void rrd_finalize_collection_for_all_hosts(void);

long align_entries_to_pagesize(RRD_MEMORY_MODE mode, long entries);

#ifdef NETDATA_LOG_COLLECTION_ERRORS
#define rrddim_store_metric(rd, point_end_time_ut, n, flags) rrddim_store_metric_with_trace(rd, point_end_time_ut, n, flags, __FUNCTION__)
void rrddim_store_metric_with_trace(RRDDIM *rd, usec_t point_end_time_ut, NETDATA_DOUBLE n, SN_FLAGS flags, const char *function);
#else
void rrddim_store_metric(RRDDIM *rd, usec_t point_end_time_ut, NETDATA_DOUBLE n, SN_FLAGS flags);
#endif

// ----------------------------------------------------------------------------
// Miscellaneous functions

char *rrdset_strncpyz_name(char *to, const char *from, size_t length);
void reload_host_labels(void);
void rrdhost_set_is_parent_label(void);

// ----------------------------------------------------------------------------
// RRD internal functions

void rrdset_free(RRDSET *st);

void rrddim_free(RRDSET *st, RRDDIM *rd);

#ifdef NETDATA_RRD_INTERNALS

char *rrdhost_cache_dir_for_rrdset_alloc(RRDHOST *host, const char *id);

void rrdset_reset(RRDSET *st);

#endif /* NETDATA_RRD_INTERNALS */

void set_host_properties(
    RRDHOST *host, int update_every, RRD_MEMORY_MODE memory_mode, const char *registry_hostname,
    const char *os, const char *tags, const char *tzone, const char *abbrev_tzone, int32_t utc_offset,
    const char *prog_name, const char *prog_version);

size_t get_tier_grouping(size_t tier);
void store_metric_collection_completed(void);

static inline void rrdhost_retention(RRDHOST *host, time_t now, bool online, time_t *from, time_t *to) {
    time_t first_time_s = 0, last_time_s = 0;
    spinlock_lock(&host->retention.spinlock);
    first_time_s = host->retention.first_time_s;
    last_time_s = host->retention.last_time_s;
    spinlock_unlock(&host->retention.spinlock);

    if(from)
        *from = first_time_s;

    if(to)
        *to = online ? now : last_time_s;
}

void rrdhost_pluginsd_send_chart_slots_free(RRDHOST *host);
void rrdhost_pluginsd_receive_chart_slots_free(RRDHOST *host);
void rrdset_pluginsd_receive_unslot_and_cleanup(RRDSET *st);
void rrdset_pluginsd_receive_unslot(RRDSET *st);

// ----------------------------------------------------------------------------
static inline double rrddim_get_last_stored_value(RRDDIM *rd_dim, double *max_value, double div) {
    if (!rd_dim)
        return NAN;

    if (isnan(div) || div == 0.0)
        div = 1.0;

    double value = rd_dim->collector.last_stored_value / div;
    value = ABS(value);

    *max_value = MAX(*max_value, value);

    return value;
}

//
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

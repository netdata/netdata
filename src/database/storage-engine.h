// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STORAGEENGINEAPI_H
#define NETDATA_STORAGEENGINEAPI_H

#include "libnetdata/libnetdata.h"
#include "rrd-database-mode.h"

typedef struct rrddim RRDDIM;

typedef struct storage_query_handle STORAGE_QUERY_HANDLE;

typedef enum __attribute__ ((__packed__)) storage_priority {
    STORAGE_PRIORITY_INTERNAL_DBENGINE = 0,
    STORAGE_PRIORITY_INTERNAL_QUERY_PREP,

    // query priorities
    STORAGE_PRIORITY_HIGH,
    STORAGE_PRIORITY_NORMAL,
    STORAGE_PRIORITY_SYNCHRONOUS_FIRST,
    STORAGE_PRIORITY_LOW,
    STORAGE_PRIORITY_BEST_EFFORT,

    // synchronous query, not to be dispatched to workers or queued
    STORAGE_PRIORITY_SYNCHRONOUS,

    STORAGE_PRIORITY_INTERNAL_MAX_DONT_USE,
} STORAGE_PRIORITY;

typedef enum __attribute__ ((__packed__)) {
    STORAGE_ENGINE_BACKEND_RRDDIM = 1,      // the in-memory ring of src/database/ram/ (builds without dbengine only)
    STORAGE_ENGINE_BACKEND_DBENGINE = 2,
    STORAGE_ENGINE_BACKEND_NONE = 3,        // db = none: tier 0 keeps no data (storage-none.c)
} STORAGE_ENGINE_BACKEND;

#define is_valid_backend(backend) ((backend) >= STORAGE_ENGINE_BACKEND_RRDDIM && (backend) <= STORAGE_ENGINE_BACKEND_NONE)

// iterator state for RRD dimension data queries
struct storage_engine_query_handle {
    time_t start_time_s;
    time_t end_time_s;
    STORAGE_PRIORITY priority;
    STORAGE_ENGINE_BACKEND seb;
    STORAGE_QUERY_HANDLE *handle;
};

// non-existing structs instead of voids
// to enable type checking at compile time
typedef struct storage_instance STORAGE_INSTANCE;
typedef struct storage_metric_handle STORAGE_METRIC_HANDLE;
typedef struct storage_alignment STORAGE_METRICS_GROUP;

// --------------------------------------------------------------------------------------------------------------------
// engine-specific iterator state for dimension data collection

typedef struct storage_collect_handle {
    STORAGE_ENGINE_BACKEND seb;
} STORAGE_COLLECT_HANDLE;

// --------------------------------------------------------------------------------------------------------------------
// function pointers for all APIs provided by a storage engine

typedef struct storage_engine_api {
    // metric management
    STORAGE_METRIC_HANDLE *(*metric_get_by_id)(STORAGE_INSTANCE *si, UUIDMAP_ID id);
    STORAGE_METRIC_HANDLE *(*metric_get_by_uuid)(STORAGE_INSTANCE *si, nd_uuid_t *uuid);
    STORAGE_METRIC_HANDLE *(*metric_get_or_create)(RRDDIM *rd, STORAGE_INSTANCE *si);
    void (*metric_release)(STORAGE_METRIC_HANDLE *);
    STORAGE_METRIC_HANDLE *(*metric_dup)(STORAGE_METRIC_HANDLE *);
    bool (*metric_retention_by_id)(STORAGE_INSTANCE *si, UUIDMAP_ID id, time_t *first_entry_s, time_t *last_entry_s);
    bool (*metric_retention_by_uuid)(STORAGE_INSTANCE *si, nd_uuid_t *uuid, time_t *first_entry_s, time_t *last_entry_s);
    void (*metric_retention_delete_by_id)(STORAGE_INSTANCE *si, UUIDMAP_ID id);
} STORAGE_ENGINE_API;

typedef struct storage {
    STORAGE_ENGINE_BACKEND seb;
    RRD_DB_MODE id;
    const char* name;
    STORAGE_ENGINE_API api;
} STORAGE_ENGINE;

STORAGE_ENGINE* storage_engine_get(RRD_DB_MODE mmode);
STORAGE_ENGINE* storage_engine_find(const char* name);

// Iterator over existing engines
STORAGE_ENGINE* storage_engine_foreach_init();
STORAGE_ENGINE* storage_engine_foreach_next(STORAGE_ENGINE* it);

// --------------------------------------------------------------------------------------------------------------------
// Storage tier data for every dimension

struct rrddim_tier {
    STORAGE_POINT virtual_point;
    STORAGE_POINT last_completed_point;         // tier1/2 spread over time
    SPINLOCK spinlock;
    STORAGE_ENGINE_BACKEND seb;
    uint16_t last_completed_point_flush_modulo; // tier1/2 spread over time
    uint32_t tier_grouping;
    time_t next_point_end_time_s;
    STORAGE_METRIC_HANDLE *smh;    // the metric handle inside the database
    STORAGE_COLLECT_HANDLE *sch;   // the data collection handle
};

// --------------------------------------------------------------------------------------------------------------------

#include "daemon/config/netdata-conf-db.h"

// --------------------------------------------------------------------------------------------------------------------
// DATA COLLECTION STORAGE OPS
//
// Every op dispatches to dbengine, then (only in builds without dbengine) to the ring of src/database/ram/, and
// otherwise to the none backend.

// db = none (storage-none.c): tier 0 keeps no data, only the time of the last point collected, so a collected metric
// still has a retention (a metric without one is deleted by the contexts layer) but queries find no points.
STORAGE_METRIC_HANDLE *storage_none_metric_get_or_create(RRDDIM *rd, STORAGE_INSTANCE *si);
STORAGE_METRIC_HANDLE *storage_none_metric_get_by_id(STORAGE_INSTANCE *si, UUIDMAP_ID id);
STORAGE_METRIC_HANDLE *storage_none_metric_get_by_uuid(STORAGE_INSTANCE *si, nd_uuid_t *uuid);
STORAGE_METRIC_HANDLE *storage_none_metric_dup(STORAGE_METRIC_HANDLE *smh);
void storage_none_metric_release(STORAGE_METRIC_HANDLE *smh);
bool storage_none_metric_retention_by_id(STORAGE_INSTANCE *si, UUIDMAP_ID id, time_t *first_entry_s, time_t *last_entry_s);
bool storage_none_metric_retention_by_uuid(STORAGE_INSTANCE *si, nd_uuid_t *uuid, time_t *first_entry_s, time_t *last_entry_s);
void storage_none_metric_retention_delete_by_id(STORAGE_INSTANCE *si, UUIDMAP_ID id);
STORAGE_COLLECT_HANDLE *storage_none_collect_init(STORAGE_METRIC_HANDLE *smh);
void storage_none_store_metric(STORAGE_COLLECT_HANDLE *sch, usec_t point_in_time_ut);
int storage_none_collect_finalize(STORAGE_COLLECT_HANDLE *sch);
time_t storage_none_latest_time_s(STORAGE_METRIC_HANDLE *smh);

#ifndef ENABLE_DBENGINE
// the ring of src/database/ram/ (rrddim_mem.h), the ram/alloc engine of builds without dbengine
STORAGE_METRICS_GROUP *rrddim_metrics_group_get(STORAGE_INSTANCE *si, uint64_t seed);
void rrddim_metrics_group_release(STORAGE_INSTANCE *si, STORAGE_METRICS_GROUP *smg);
STORAGE_COLLECT_HANDLE *rrddim_collect_init(STORAGE_METRIC_HANDLE *smh, uint32_t update_every, STORAGE_METRICS_GROUP *smg);
void rrddim_collect_store_metric(
    STORAGE_COLLECT_HANDLE *sch, usec_t point_in_time_ut,
    NETDATA_DOUBLE n, NETDATA_DOUBLE min_value, NETDATA_DOUBLE max_value,
    uint16_t count, uint16_t anomaly_count, SN_FLAGS flags);
void rrddim_store_metric_flush(STORAGE_COLLECT_HANDLE *sch);
int rrddim_collect_finalize(STORAGE_COLLECT_HANDLE *sch);
void rrddim_store_metric_change_collection_frequency(STORAGE_COLLECT_HANDLE *sch, int update_every);
time_t rrddim_query_oldest_time_s(STORAGE_METRIC_HANDLE *smh);
time_t rrddim_query_latest_time_s(STORAGE_METRIC_HANDLE *smh);
void rrddim_query_init(
    STORAGE_METRIC_HANDLE *smh, struct storage_engine_query_handle *seqh,
    time_t start_time_s, time_t end_time_s, STORAGE_PRIORITY priority);
STORAGE_POINT rrddim_query_next_metric(struct storage_engine_query_handle *seqh);
int rrddim_query_is_finished(struct storage_engine_query_handle *seqh);
void rrddim_query_finalize(struct storage_engine_query_handle *seqh);
time_t rrddim_query_align_to_optimal_before(struct storage_engine_query_handle *seqh);
#endif

STORAGE_METRICS_GROUP *rrdeng_metrics_group_get(STORAGE_INSTANCE *si, uint64_t seed);

static inline STORAGE_METRICS_GROUP *storage_engine_metrics_group_get(STORAGE_ENGINE_BACKEND seb __maybe_unused, STORAGE_INSTANCE *si __maybe_unused, uint64_t seed __maybe_unused) {
    internal_fatal(!is_valid_backend(seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        return rrdeng_metrics_group_get(si, seed);
#else
    if(seb == STORAGE_ENGINE_BACKEND_RRDDIM)
        return rrddim_metrics_group_get(si, seed);
#endif
    return NULL;
}

// --------------------------------------------------------------------------------------------------------------------

void rrdeng_metrics_group_release(STORAGE_INSTANCE *si, STORAGE_METRICS_GROUP *smg);

static inline void storage_engine_metrics_group_release(STORAGE_ENGINE_BACKEND seb __maybe_unused, STORAGE_INSTANCE *si __maybe_unused, STORAGE_METRICS_GROUP *smg __maybe_unused) {
    internal_fatal(!is_valid_backend(seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        rrdeng_metrics_group_release(si, smg);
#else
    if(seb == STORAGE_ENGINE_BACKEND_RRDDIM)
        rrddim_metrics_group_release(si, smg);
#endif
}

// --------------------------------------------------------------------------------------------------------------------

STORAGE_COLLECT_HANDLE *rrdeng_store_metric_init(STORAGE_METRIC_HANDLE *smh, uint32_t update_every, STORAGE_METRICS_GROUP *smg);

static inline STORAGE_COLLECT_HANDLE *storage_metric_store_init(STORAGE_ENGINE_BACKEND seb __maybe_unused, STORAGE_METRIC_HANDLE *smh __maybe_unused, uint32_t update_every __maybe_unused, STORAGE_METRICS_GROUP *smg __maybe_unused) {
    internal_fatal(!is_valid_backend(seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        return rrdeng_store_metric_init(smh, update_every, smg);
#else
    if(seb == STORAGE_ENGINE_BACKEND_RRDDIM)
        return rrddim_collect_init(smh, update_every, smg);
#endif
    return storage_none_collect_init(smh);
}

// --------------------------------------------------------------------------------------------------------------------

void rrdeng_store_metric_next(
    STORAGE_COLLECT_HANDLE *sch, usec_t point_in_time_ut,
    NETDATA_DOUBLE n, NETDATA_DOUBLE min_value, NETDATA_DOUBLE max_value,
    uint16_t count, uint16_t anomaly_count, SN_FLAGS flags);

ALWAYS_INLINE_HOT_FLATTEN
static void storage_engine_store_metric(
    STORAGE_COLLECT_HANDLE *sch, usec_t point_in_time_ut __maybe_unused,
    NETDATA_DOUBLE n __maybe_unused, NETDATA_DOUBLE min_value __maybe_unused, NETDATA_DOUBLE max_value __maybe_unused,
    uint16_t count __maybe_unused, uint16_t anomaly_count __maybe_unused, SN_FLAGS flags __maybe_unused) {
    if(unlikely(!sch))
        return;

    internal_fatal(!is_valid_backend(sch->seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(sch->seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        rrdeng_store_metric_next(sch, point_in_time_ut,
                                 n, min_value, max_value,
                                 count, anomaly_count, flags);
    else
#else
    if(sch->seb == STORAGE_ENGINE_BACKEND_RRDDIM)
        rrddim_collect_store_metric(sch, point_in_time_ut,
                                    n, min_value, max_value,
                                    count, anomaly_count, flags);
    else
#endif
        storage_none_store_metric(sch, point_in_time_ut);
}

// --------------------------------------------------------------------------------------------------------------------

uint64_t rrdeng_disk_space_max(STORAGE_INSTANCE *si);

static inline uint64_t storage_engine_disk_space_max(STORAGE_ENGINE_BACKEND seb __maybe_unused, STORAGE_INSTANCE *si __maybe_unused) {
#ifdef ENABLE_DBENGINE
    if(likely(seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        return rrdeng_disk_space_max(si);
#endif

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------

uint64_t rrdeng_disk_space_used(STORAGE_INSTANCE *si);

static inline uint64_t storage_engine_disk_space_used(STORAGE_ENGINE_BACKEND seb __maybe_unused, STORAGE_INSTANCE *si __maybe_unused) {
#ifdef ENABLE_DBENGINE
    if(likely(seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        return rrdeng_disk_space_used(si);
#endif

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------

uint64_t rrdeng_metrics(STORAGE_INSTANCE *si);

static inline uint64_t storage_engine_metrics(STORAGE_ENGINE_BACKEND seb __maybe_unused, STORAGE_INSTANCE *si __maybe_unused) {
#ifdef ENABLE_DBENGINE
    if(likely(seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        return rrdeng_metrics(si);
#endif

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------

uint64_t rrdeng_samples(STORAGE_INSTANCE *si);

static inline uint64_t storage_engine_samples(STORAGE_ENGINE_BACKEND seb __maybe_unused, STORAGE_INSTANCE *si __maybe_unused) {
#ifdef ENABLE_DBENGINE
    if(likely(seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        return rrdeng_samples(si);
#endif
    return 0;
}

// --------------------------------------------------------------------------------------------------------------------

time_t rrdeng_global_first_time_s(STORAGE_INSTANCE *si);

// zero means unknown (no retention); see rrdstats_retention_collect()
static inline time_t storage_engine_global_first_time_s(STORAGE_ENGINE_BACKEND seb __maybe_unused, STORAGE_INSTANCE *si __maybe_unused) {
#ifdef ENABLE_DBENGINE
    if(likely(seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        return rrdeng_global_first_time_s(si);
#else
    if(seb == STORAGE_ENGINE_BACKEND_RRDDIM)
        return now_realtime_sec() - (time_t)(default_rrd_history_entries * nd_profile.update_every);
#endif

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------

size_t rrdeng_currently_collected_metrics(STORAGE_INSTANCE *si);

static inline size_t storage_engine_collected_metrics(STORAGE_ENGINE_BACKEND seb __maybe_unused, STORAGE_INSTANCE *si __maybe_unused) {
#ifdef ENABLE_DBENGINE
    if(likely(seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        return rrdeng_currently_collected_metrics(si);
#endif

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------

void rrdeng_store_metric_flush_current_page(STORAGE_COLLECT_HANDLE *sch);

static inline void storage_engine_store_flush(STORAGE_COLLECT_HANDLE *sch) {
    if(unlikely(!sch))
        return;

    internal_fatal(!is_valid_backend(sch->seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(sch->seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        rrdeng_store_metric_flush_current_page(sch);
#else
    if(sch->seb == STORAGE_ENGINE_BACKEND_RRDDIM)
        rrddim_store_metric_flush(sch);
#endif
}

// --------------------------------------------------------------------------------------------------------------------

time_t rrdeng_store_metric_page_time_to_close_s(STORAGE_COLLECT_HANDLE *sch);

// How many seconds of collected data the dimension's current tier page still needs before it is complete (the
// distance from its last stored point to the point that fills it and makes it flush, start + (capacity - 1) *
// update_every), or -1 when there is no open page or the backend has no pages. It is measured in the dimension's
// own timestamps, so it does not depend on how far a child's data lags the local clock: while the dimension keeps
// collecting, the page completes about that many seconds from now. A page left idle by a collection gap keeps the
// value it had when collection stopped. The caller MUST hold rd->tiers[tier].spinlock so the handle cannot be
// finalized under it; the fields themselves are read atomically, without the collector's cooperation, so a page
// change racing the read yields a value that is off by one page or -1 - acceptable for the ML training scheduler,
// its only user (see src/ml/ml_queue.h).
static inline time_t storage_engine_store_page_time_to_close_s(STORAGE_COLLECT_HANDLE *sch) {
    if(unlikely(!sch))
        return -1;

    internal_fatal(!is_valid_backend(sch->seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(sch->seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        return rrdeng_store_metric_page_time_to_close_s(sch);
#endif

    return -1;
}

// --------------------------------------------------------------------------------------------------------------------

int rrdeng_store_metric_finalize(STORAGE_COLLECT_HANDLE *sch);

// a finalization function to run after collection is over
// returns 1 if it's safe to delete the dimension (the db keeps no data of it)
static inline int storage_engine_store_finalize(STORAGE_COLLECT_HANDLE *sch) {
    if(unlikely(!sch))
        return 1;  // safe to delete if no handle

    internal_fatal(!is_valid_backend(sch->seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(sch->seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        return rrdeng_store_metric_finalize(sch);
#else
    if(sch->seb == STORAGE_ENGINE_BACKEND_RRDDIM)
        return rrddim_collect_finalize(sch);
#endif

    return storage_none_collect_finalize(sch);
}

// --------------------------------------------------------------------------------------------------------------------

void rrdeng_store_metric_change_collection_frequency(STORAGE_COLLECT_HANDLE *sch, int update_every);

static inline void storage_engine_store_change_collection_frequency(STORAGE_COLLECT_HANDLE *sch, int update_every __maybe_unused) {
    if(unlikely(!sch))
        return;

    internal_fatal(!is_valid_backend(sch->seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(sch->seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        rrdeng_store_metric_change_collection_frequency(sch, update_every);
#else
    if(sch->seb == STORAGE_ENGINE_BACKEND_RRDDIM)
        rrddim_store_metric_change_collection_frequency(sch, update_every);
#endif
}

// --------------------------------------------------------------------------------------------------------------------
// STORAGE ENGINE QUERY OPS

time_t rrdeng_metric_oldest_time(STORAGE_METRIC_HANDLE *smh);

ALWAYS_INLINE_HOT_FLATTEN
static time_t storage_engine_oldest_time_s(STORAGE_ENGINE_BACKEND seb  __maybe_unused, STORAGE_METRIC_HANDLE *smh __maybe_unused) {
    internal_fatal(!is_valid_backend(seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        return rrdeng_metric_oldest_time(smh);
#else
    if(seb == STORAGE_ENGINE_BACKEND_RRDDIM)
        return rrddim_query_oldest_time_s(smh);
#endif
    return storage_none_latest_time_s(smh);
}

// --------------------------------------------------------------------------------------------------------------------

time_t rrdeng_metric_latest_time(STORAGE_METRIC_HANDLE *smh);

ALWAYS_INLINE_HOT_FLATTEN
static time_t storage_engine_latest_time_s(STORAGE_ENGINE_BACKEND seb __maybe_unused, STORAGE_METRIC_HANDLE *smh __maybe_unused) {
    internal_fatal(!is_valid_backend(seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        return rrdeng_metric_latest_time(smh);
#else
    if(seb == STORAGE_ENGINE_BACKEND_RRDDIM)
        return rrddim_query_latest_time_s(smh);
#endif
    return storage_none_latest_time_s(smh);
}

// --------------------------------------------------------------------------------------------------------------------

void rrdeng_load_metric_init(
    STORAGE_METRIC_HANDLE *smh, struct storage_engine_query_handle *seqh,
    time_t start_time_s, time_t end_time_s, STORAGE_PRIORITY priority);

ALWAYS_INLINE_HOT_FLATTEN
static void storage_engine_query_init(
    STORAGE_ENGINE_BACKEND seb __maybe_unused,
    STORAGE_METRIC_HANDLE *smh __maybe_unused, struct storage_engine_query_handle *seqh,
    time_t start_time_s, time_t end_time_s, STORAGE_PRIORITY priority) {
    internal_fatal(!is_valid_backend(seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(seb == STORAGE_ENGINE_BACKEND_DBENGINE)) {
        rrdeng_load_metric_init(smh, seqh, start_time_s, end_time_s, priority);
        return;
    }
#else
    if(seb == STORAGE_ENGINE_BACKEND_RRDDIM) {
        rrddim_query_init(smh, seqh, start_time_s, end_time_s, priority);
        return;
    }
#endif

    // none: a query that is finished before it starts
    seqh->start_time_s = start_time_s;
    seqh->end_time_s = end_time_s;
    seqh->priority = priority;
    seqh->seb = STORAGE_ENGINE_BACKEND_NONE;
    seqh->handle = NULL;
}

// --------------------------------------------------------------------------------------------------------------------

STORAGE_POINT rrdeng_load_metric_next(struct storage_engine_query_handle *seqh);

ALWAYS_INLINE_HOT_FLATTEN
static STORAGE_POINT storage_engine_query_next_metric(struct storage_engine_query_handle *seqh) {
    internal_fatal(!is_valid_backend(seqh->seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(seqh->seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        return rrdeng_load_metric_next(seqh);
#else
    if(seqh->seb == STORAGE_ENGINE_BACKEND_RRDDIM)
        return rrddim_query_next_metric(seqh);
#endif

    // none: never called, the query is finished from the start
    STORAGE_POINT sp;
    storage_point_empty(sp, seqh->start_time_s, seqh->end_time_s);
    return sp;
}

// --------------------------------------------------------------------------------------------------------------------

int rrdeng_load_metric_is_finished(struct storage_engine_query_handle *seqh);

ALWAYS_INLINE_HOT_FLATTEN
static int storage_engine_query_is_finished(struct storage_engine_query_handle *seqh) {
    internal_fatal(!is_valid_backend(seqh->seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(seqh->seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        return rrdeng_load_metric_is_finished(seqh);
#else
    if(seqh->seb == STORAGE_ENGINE_BACKEND_RRDDIM)
        return rrddim_query_is_finished(seqh);
#endif
    return 1;
}

// --------------------------------------------------------------------------------------------------------------------

void rrdeng_load_metric_finalize(struct storage_engine_query_handle *seqh);

ALWAYS_INLINE_HOT_FLATTEN
static void storage_engine_query_finalize(struct storage_engine_query_handle *seqh) {
    internal_fatal(!is_valid_backend(seqh->seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(seqh->seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        rrdeng_load_metric_finalize(seqh);
#else
    if(seqh->seb == STORAGE_ENGINE_BACKEND_RRDDIM)
        rrddim_query_finalize(seqh);
#endif
}

// --------------------------------------------------------------------------------------------------------------------

time_t rrdeng_load_align_to_optimal_before(struct storage_engine_query_handle *seqh);

ALWAYS_INLINE_HOT_FLATTEN
static time_t storage_engine_align_to_optimal_before(struct storage_engine_query_handle *seqh) {
    internal_fatal(!is_valid_backend(seqh->seb), "STORAGE: invalid backend");

#ifdef ENABLE_DBENGINE
    if(likely(seqh->seb == STORAGE_ENGINE_BACKEND_DBENGINE))
        return rrdeng_load_align_to_optimal_before(seqh);
#else
    if(seqh->seb == STORAGE_ENGINE_BACKEND_RRDDIM)
        return rrddim_query_align_to_optimal_before(seqh);
#endif
    return seqh->end_time_s;
}

#endif

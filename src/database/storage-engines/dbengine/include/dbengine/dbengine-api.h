// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DBENGINE_API_H
#define NETDATA_DBENGINE_API_H

#include "libnetdata/libnetdata.h"
#include "database/storage-engines/storage-engine-types.h"
#include "database/storage-engines/dbengine/include/dbengine/dbengine-config.h"
#include "database/storage-engines/dbengine/include/dbengine/dbengine-stats.h"

#ifdef __cplusplus
extern "C" {
#endif

// The engine's public API. Everything the daemon needs from the engine is declared here or in the other public
// headers (dbengine-config.h, dbengine-stats.h, dbengine-workers.h, dbengine-tests.h); the rest of this directory
// is private and no header here includes it.
//
// The engine (DBENGINE_ENGINE, dbengine-config.h) is what the tiers share: its event loop, caches, metrics registry
// and configuration. The embedder makes it with dbengine_create() and holds it; every verb and getter that is about
// the engine rather than about a tier takes it, and accepts NULL (the embedder that never made one) as an engine
// with nothing in it: verbs do nothing, getters report false or zeros.
//
// A tier (struct dbengine_tier) is one tier of one database: a directory of datafiles and their journals. Outside
// the engine it is an opaque pointer: the storage vtable's STORAGE_INSTANCE is this very pointer, cast by the
// engine on either side, and the daemon only passes tiers around and indexes dbengine_multidb_tiers[], the static
// tiers of the daemon's multi-host database, which belong to the one engine that is up.
struct dbengine_tier;

extern DBENGINE_TIER *dbengine_multidb_tiers[RRD_STORAGE_TIERS];

// true when dbfiles_path holds at least one datafile named the way this engine names and scans
// them. Reads the directory only, touches no engine state, so it can be called before any tier
// is up (or on a build that never brings one up) to learn whether a previous run left data
// behind; a directory that cannot be opened holds none.
bool dbengine_dir_has_datafiles(const char *dbfiles_path);
STORAGE_METRIC_HANDLE *dbengine_metric_get_or_create_by_id(STORAGE_INSTANCE *si, UUIDMAP_ID id);
STORAGE_METRIC_HANDLE *dbengine_metric_get_by_id(STORAGE_INSTANCE *si, UUIDMAP_ID id);
STORAGE_METRIC_HANDLE *dbengine_metric_get_by_uuid(STORAGE_INSTANCE *si, nd_uuid_t *uuid);
void dbengine_metric_release(STORAGE_METRIC_HANDLE *smh);
STORAGE_METRIC_HANDLE *dbengine_metric_dup(STORAGE_METRIC_HANDLE *smh);

STORAGE_COLLECT_HANDLE *dbengine_store_init(STORAGE_METRIC_HANDLE *smh, uint32_t update_every, STORAGE_METRICS_GROUP *smg);
void dbengine_store_flush(STORAGE_COLLECT_HANDLE *sch);          // the collector's current page: hot -> dirty
void dbengine_store_change_collection_frequency(STORAGE_COLLECT_HANDLE *sch, int update_every);
void dbengine_store_next(STORAGE_COLLECT_HANDLE *sch, usec_t point_in_time_ut, NETDATA_DOUBLE n,
                         NETDATA_DOUBLE min_value,
                         NETDATA_DOUBLE max_value,
                         uint16_t count,
                         uint16_t anomaly_count,
                         SN_FLAGS flags);
int dbengine_store_finalize(STORAGE_COLLECT_HANDLE *sch);

void dbengine_query_init(STORAGE_METRIC_HANDLE *smh, struct storage_engine_query_handle *seqh,
                         time_t start_time_s, time_t end_time_s, STORAGE_PRIORITY priority);
STORAGE_POINT dbengine_query_next(struct storage_engine_query_handle *seqh);

int dbengine_query_is_finished(struct storage_engine_query_handle *seqh);
void dbengine_query_finalize(struct storage_engine_query_handle *seqh);
time_t dbengine_latest_time_s(STORAGE_METRIC_HANDLE *smh);
time_t dbengine_oldest_time_s(STORAGE_METRIC_HANDLE *smh);
time_t dbengine_query_align_to_optimal_before(struct storage_engine_query_handle *seqh);

// bring the static tier dbengine_multidb_tiers[tc->tier] up on a running engine, once per process: after
// dbengine_create() (fatal without the engine), before dbengine_shutdown() (UV_EIO after it), not while the tier is
// up (UV_EALREADY) and not after it came up and exited (UV_EIO: only dbengine_destroy() finalizes its datafiles and
// resets the slot). An invalid tc is fatal before any of those checks (a programming error, whatever the state).
// Those refusals leave the tier untouched; an init that fails opening the datafiles returns
// UV_EIO with the tier's configuration already written. Two inits of the same tier must not overlap, nothing
// serialises them. A tier init and the shutdown must not overlap either: the check is made when the tier starts,
// so an init that is still in flight when the shutdown begins would wait on a loop that is gone (the daemon joins
// its tier inits at startup, long before any shutdown)
int dbengine_tier_init(DBENGINE_ENGINE *engine, const struct dbengine_tier_config *tc);

void dbengine_readiness_wait(DBENGINE_TIER *tier);

int dbengine_tier_exit(DBENGINE_TIER *tier);

// a tier that came up and has not been shut down (dbengine_tier_exit() clears it first); the engine's
// periodic work covers exactly these tiers, and the embedder uses it to skip tiers that never
// started or already stopped
bool dbengine_tier_is_active(DBENGINE_TIER *tier);

// dbengine_create()'s counterpart: refuse embedder work, stop the engine's event loop and join its thread. After
// every tier's dbengine_tier_exit() (a tier exit needs the live loop), and never while a dbengine_tier_init() is
// in flight. Nothing may be enqueued to the engine afterwards, and no engine can be made again in this process
// (dbengine_create() returns NULL, dbengine_tier_init() fails). With no engine (NULL) it only records the stop,
// and a later dbengine_create() still returns NULL; a second call returns at once and does not wait for the first
// to finish
void dbengine_shutdown(DBENGINE_ENGINE *engine);
void dbengine_quiesce(DBENGINE_TIER *tier);
void dbengine_flush_dirty(DBENGINE_TIER *tier);
void dbengine_flush_all(DBENGINE_TIER *tier);

// Tear down the engine, for an embedder that checks for leaks at exit: the caches, the metrics registry, every
// static tier's datafiles, then the engine's own allocators and the engine, in the order their dependencies allow.
// Only after every tier's dbengine_tier_exit() and dbengine_shutdown(); never on an exit that skipped them (the
// loop is still running). A cache or the registry with live references stays allocated and reachable, and so does
// the engine they belong to; the embedder's handle is not valid after the call either way. Returns the registry
// metrics still referenced, 0 when there were none; 0 for NULL.
size_t dbengine_destroy(DBENGINE_ENGINE *engine);

// what the embedder reads about the tiers
time_t dbengine_max_retention_s(DBENGINE_TIER *tier);   // the tier's configured time limit; 0 = none
uint64_t dbengine_disk_space_max(STORAGE_INSTANCE *si);             // the tier's configured disk quota; 0 = none
uint64_t dbengine_disk_space_used(STORAGE_INSTANCE *si);
uint64_t dbengine_metrics(STORAGE_INSTANCE *si);
uint64_t dbengine_samples(STORAGE_INSTANCE *si);
time_t dbengine_global_first_time_s(STORAGE_INSTANCE *si);          // 0 while the tier holds no data
uint64_t dbengine_get_used_disk_space(DBENGINE_TIER *tier);
uint64_t dbengine_get_directory_free_bytes_space(DBENGINE_TIER *tier);

bool dbengine_metric_retention_by_id(STORAGE_INSTANCE *si, UUIDMAP_ID id, time_t *first_entry_s, time_t *last_entry_s);
bool dbengine_metric_retention_by_uuid(STORAGE_INSTANCE *si, nd_uuid_t *dim_uuid, time_t *first_entry_s, time_t *last_entry_s);
void dbengine_metric_retention_delete_by_id(STORAGE_INSTANCE *si, UUIDMAP_ID id);

extern STORAGE_METRICS_GROUP *dbengine_metrics_group_get(STORAGE_INSTANCE *si, nd_uuid_t *uuid);
extern void dbengine_metrics_group_release(STORAGE_INSTANCE *si, STORAGE_METRICS_GROUP *smg);

// work the embedder wants run on the engine's worker pool, next to the engine's own jobs. The caller owns the
// request and its completion (init before, destroy after); the engine runs fn(data) on a worker and marks the
// completion when it returns. fn is responsible for its own worker_is_busy() attribution, including registering
// the job names it reports: the pool threads register only the engine's own names.
//
// The engine takes work only while it is serving: from the moment dbengine_create() brought the event loop up until
// dbengine_shutdown() starts. Outside that window dbengine_enq_work() returns false having done
// nothing the caller can observe (no queueing, no wake-up, the completion is not touched), and the caller runs or
// drops the work itself; dbengine_work_available() answers the same question up front, for a caller that wants to
// plan a batch. Once the engine serves, the only later change is to stopped, so a request accepted is always
// completed. Only this entry point is gated; the engine's own commands are not.
struct dbengine_work_request {
    void (*fn)(void *data);
    void *data;
    struct completion completion;
};
bool dbengine_enq_work(DBENGINE_ENGINE *engine, struct dbengine_work_request *req);
bool dbengine_work_available(DBENGINE_ENGINE *engine);

size_t dbengine_collectors_running(DBENGINE_TIER *tier);

#ifdef __cplusplus
}
#endif

#endif /* NETDATA_DBENGINE_API_H */

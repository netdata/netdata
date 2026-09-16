// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDENGINEAPI_H
#define NETDATA_RRDENGINEAPI_H

#include "libnetdata/libnetdata.h"
#include "dbengine-config.h"
#include "dbengine-stats.h"

// The engine's public API. Everything the daemon needs from the engine is declared here or in the other public
// headers (dbengine-config.h, dbengine-stats.h, dbengine-workers.h, dbengine-tests.h); the rest of this directory
// is private and no header here includes it.
//
// An engine instance is one tier of one database. Outside the engine it is an opaque pointer: the storage vtable's
// STORAGE_INSTANCE is this very pointer, cast by the engine on either side, and the daemon only passes instances
// around and indexes multidb_ctx[], the static tiers of the daemon's multi-host database.
struct rrdengine_instance;

extern struct rrdengine_instance *multidb_ctx[RRD_STORAGE_TIERS];

// true when dbfiles_path holds at least one datafile named the way this engine names and scans
// them. Reads the directory only, touches no engine state, so it can be called before any tier
// is up (or on a build that never brings one up) to learn whether a previous run left data
// behind; a directory that cannot be opened holds none.
bool rrdeng_datafiles_present(const char *dbfiles_path);
STORAGE_METRIC_HANDLE *rrdeng_metric_get_or_create_by_id(STORAGE_INSTANCE *si, UUIDMAP_ID id);
STORAGE_METRIC_HANDLE *rrdeng_metric_get_by_id(STORAGE_INSTANCE *si, UUIDMAP_ID id);
STORAGE_METRIC_HANDLE *rrdeng_metric_get_by_uuid(STORAGE_INSTANCE *si, nd_uuid_t *uuid);
void rrdeng_metric_release(STORAGE_METRIC_HANDLE *smh);
STORAGE_METRIC_HANDLE *rrdeng_metric_dup(STORAGE_METRIC_HANDLE *smh);

STORAGE_COLLECT_HANDLE *rrdeng_store_metric_init(STORAGE_METRIC_HANDLE *smh, uint32_t update_every, STORAGE_METRICS_GROUP *smg);
void rrdeng_store_metric_flush_current_page(STORAGE_COLLECT_HANDLE *sch);
void rrdeng_store_metric_change_collection_frequency(STORAGE_COLLECT_HANDLE *sch, int update_every);
void rrdeng_store_metric_next(STORAGE_COLLECT_HANDLE *sch, usec_t point_in_time_ut, NETDATA_DOUBLE n,
                                     NETDATA_DOUBLE min_value,
                                     NETDATA_DOUBLE max_value,
                                     uint16_t count,
                                     uint16_t anomaly_count,
                                     SN_FLAGS flags);
int rrdeng_store_metric_finalize(STORAGE_COLLECT_HANDLE *sch);

void rrdeng_load_metric_init(STORAGE_METRIC_HANDLE *smh, struct storage_engine_query_handle *seqh,
                                    time_t start_time_s, time_t end_time_s, STORAGE_PRIORITY priority);
STORAGE_POINT rrdeng_load_metric_next(struct storage_engine_query_handle *seqh);

int rrdeng_load_metric_is_finished(struct storage_engine_query_handle *seqh);
void rrdeng_load_metric_finalize(struct storage_engine_query_handle *seqh);
time_t rrdeng_metric_latest_time(STORAGE_METRIC_HANDLE *smh);
time_t rrdeng_metric_oldest_time(STORAGE_METRIC_HANDLE *smh);
time_t rrdeng_load_align_to_optimal_before(struct storage_engine_query_handle *seqh);

/* must call once before using anything */
int rrdeng_init(struct rrdengine_instance **ctxp, const struct rrdeng_tier_config *tc);

void rrdeng_readiness_wait(struct rrdengine_instance *ctx);

int rrdeng_exit(struct rrdengine_instance *ctx);

// a tier that came up and has not been shut down (rrdeng_exit() clears it first); the engine's
// periodic work covers exactly these tiers, and the embedder uses it to skip tiers that never
// started or already stopped
bool rrdeng_ctx_is_active(struct rrdengine_instance *ctx);

// stop the engine's event loop and join its thread; after every tier's rrdeng_exit(). Nothing may
// be enqueued to the engine afterwards
void dbengine_shutdown(void);
void rrdeng_quiesce(struct rrdengine_instance *ctx);
void rrdeng_flush_dirty(struct rrdengine_instance *ctx);
void rrdeng_flush_all(struct rrdengine_instance *ctx);

// Tear down the engine's process-wide state, for an embedder that checks for leaks at exit: the caches, the
// metrics registry, then every static tier's datafiles, in the order their dependencies allow. Only after every
// tier's rrdeng_exit() and dbengine_shutdown(); never on an exit that skipped them (the loop is still running).
// A cache or the registry with live references stays allocated and reachable. Returns the registry metrics
// still referenced, 0 when everything was freed. Caller-allocated contexts are freed by rrdeng_exit() already.
size_t dbengine_destroy(void);

// what the embedder reads about the tiers
size_t rrdeng_active_tiers(void);
time_t rrdeng_max_retention_s(struct rrdengine_instance *ctx);   // the tier's configured time limit; 0 = none
uint64_t rrdeng_disk_space_max(STORAGE_INSTANCE *si);             // the tier's configured disk quota; 0 = none
uint64_t rrdeng_disk_space_used(STORAGE_INSTANCE *si);
uint64_t rrdeng_metrics(STORAGE_INSTANCE *si);
uint64_t rrdeng_samples(STORAGE_INSTANCE *si);
time_t rrdeng_global_first_time_s(STORAGE_INSTANCE *si);          // 0 while the tier holds no data
uint64_t rrdeng_get_used_disk_space(struct rrdengine_instance *ctx);
uint64_t rrdeng_get_directory_free_bytes_space(struct rrdengine_instance *ctx);

bool rrdeng_metric_retention_by_id(STORAGE_INSTANCE *si, UUIDMAP_ID id, time_t *first_entry_s, time_t *last_entry_s);
bool rrdeng_metric_retention_by_uuid(STORAGE_INSTANCE *si, nd_uuid_t *dim_uuid, time_t *first_entry_s, time_t *last_entry_s);
void rrdeng_metric_retention_delete_by_id(STORAGE_INSTANCE *si, UUIDMAP_ID id);

extern STORAGE_METRICS_GROUP *rrdeng_metrics_group_get(STORAGE_INSTANCE *si, nd_uuid_t *uuid);
extern void rrdeng_metrics_group_release(STORAGE_INSTANCE *si, STORAGE_METRICS_GROUP *smg);

// work the embedder wants run on the engine's worker pool, next to the engine's own jobs. The caller owns the
// request and its completion (init before, destroy after); the engine runs fn(data) on a worker and marks the
// completion when it returns. fn is responsible for its own worker_is_busy() attribution, including registering
// the job names it reports: the pool threads register only the engine's own names.
//
// The engine takes work only while it is serving: from the moment the first tier's rrdeng_init() spawned the
// event loop until dbengine_shutdown() starts. Outside that window rrdeng_enq_work() returns false having done
// nothing (no queueing, no wake-up, the completion is not touched), and the caller runs or drops the work itself;
// rrdeng_work_available() answers the same question up front, for a caller that wants to plan a batch. The
// answer can change between the two calls only in one direction, serving -> stopped, so a request accepted is
// always completed. Only this entry point is gated; the engine's own commands are not.
struct rrdeng_work_request {
    void (*fn)(void *data);
    void *data;
    struct completion completion;
};
bool rrdeng_enq_work(struct rrdeng_work_request *req);
bool rrdeng_work_available(void);

size_t rrdeng_collectors_running(struct rrdengine_instance *ctx);

#endif /* NETDATA_RRDENGINEAPI_H */

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDENGINEAPI_H
#define NETDATA_RRDENGINEAPI_H

#include "rrdengine.h"
#include "dbengine-stats.h"

#define RRDENG_MIN_PAGE_CACHE_SIZE_MB (8)
#define RRDENG_MIN_DISK_SPACE_MB (25)
#define RRDENG_DEFAULT_TIER_DISK_SPACE_MB (1024)

extern struct rrdengine_instance *multidb_ctx[RRD_STORAGE_TIERS];
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
void rrdeng_quiesce(struct rrdengine_instance *ctx);
void rrdeng_flush_dirty(struct rrdengine_instance *ctx);
void rrdeng_flush_all(struct rrdengine_instance *ctx);

// after rrdeng_exit(): close the tier's datafiles (the embedder's final teardown)
void finalize_rrd_files(struct rrdengine_instance *ctx);

// what the embedder reads about the tiers
size_t rrdeng_active_tiers(void);
uint64_t rrdeng_get_used_disk_space(struct rrdengine_instance *ctx, bool having_lock);
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
// Precondition: at least one tier has been brought up with rrdeng_init(). The command queue this call uses exists
// only once the engine has spawned; calling it on an agent without a dbengine tier dereferences NULL.
struct rrdeng_work_request {
    void (*fn)(void *data);
    void *data;
    struct completion completion;
};
void rrdeng_enq_work(struct rrdeng_work_request *req);

size_t rrdeng_collectors_running(struct rrdengine_instance *ctx);

#endif /* NETDATA_RRDENGINEAPI_H */

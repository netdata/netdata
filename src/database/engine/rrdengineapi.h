// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDENGINEAPI_H
#define NETDATA_RRDENGINEAPI_H

#include "rrdengine.h"

#define RRDENG_MIN_PAGE_CACHE_SIZE_MB (8)
#define RRDENG_MIN_DISK_SPACE_MB (25)
#define RRDENG_DEFAULT_TIER_DISK_SPACE_MB (1024)

#define RRDENG_NR_STATS (38)

#define RRDENG_FD_BUDGET_PER_INSTANCE (50)

extern uint64_t dbengine_out_of_memory_protection;
extern bool dbengine_use_all_ram_for_caches;

extern int default_rrdeng_page_cache_mb;
extern int default_rrdeng_extent_cache_mb;
extern int db_engine_journal_check;
extern int default_rrdeng_disk_quota_mb;
extern int default_multidb_disk_quota_mb;
extern bool new_dbengine_defaults;
extern bool legacy_multihost_db_space;
extern RRD_BACKFILL default_backfill;

extern struct rrdengine_instance *multidb_ctx[RRD_STORAGE_TIERS];
extern size_t page_type_size[];
extern size_t tier_page_size[];
extern size_t tier_quota_mb[];
extern uint8_t tier_page_type[];

#define CTX_POINT_SIZE_BYTES(ctx) page_type_size[(ctx)->config.page_type]

STORAGE_METRIC_HANDLE *rrdeng_metric_get_or_create(RRDDIM *rd, STORAGE_INSTANCE *si);
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

void rrdeng_get_37_statistics(struct rrdengine_instance *ctx, unsigned long long *array);

/* must call once before using anything */
int rrdeng_init(
    struct rrdengine_instance **ctxp,
    const char *dbfiles_path,
    unsigned disk_space_mb,
    size_t tier,
    time_t max_retention_s);

void rrdeng_readiness_wait(struct rrdengine_instance *ctx);

int rrdeng_exit(struct rrdengine_instance *ctx);
void rrdeng_quiesce(struct rrdengine_instance *ctx, bool dirty_only);

bool rrdeng_metric_retention_by_id(STORAGE_INSTANCE *si, UUIDMAP_ID id, time_t *first_entry_s, time_t *last_entry_s);
bool rrdeng_metric_retention_by_uuid(STORAGE_INSTANCE *si, nd_uuid_t *dim_uuid, time_t *first_entry_s, time_t *last_entry_s);
void rrdeng_metric_retention_delete_by_id(STORAGE_INSTANCE *si, UUIDMAP_ID id);

extern STORAGE_METRICS_GROUP *rrdeng_metrics_group_get(STORAGE_INSTANCE *si, nd_uuid_t *uuid);
extern void rrdeng_metrics_group_release(STORAGE_INSTANCE *si, STORAGE_METRICS_GROUP *smg);

typedef struct rrdengine_size_statistics {
    size_t default_granularity_secs;

    size_t sizeof_datafile;
    size_t sizeof_page_in_cache;
    size_t sizeof_point_data;
    size_t sizeof_page_data;

    size_t pages_per_extent;

    size_t datafiles;
    size_t extents;
    size_t extents_pages;
    size_t points;
    size_t metrics;
    size_t metrics_pages;

    size_t extents_compressed_bytes;
    size_t pages_uncompressed_bytes;
    time_t pages_duration_secs;

    struct {
        size_t pages;
        size_t pages_uncompressed_bytes;
        time_t pages_duration_secs;
        size_t points;
    } page_types[256];

    size_t single_point_pages;

    time_t first_time_s;
    time_t last_time_s;

    size_t currently_collected_metrics;
    size_t estimated_concurrently_collected_metrics;

    size_t disk_space;
    size_t max_disk_space;

    time_t database_retention_secs;
    double average_compression_savings;
    double average_point_duration_secs;
    double average_metric_retention_secs;

    double ephemeral_metrics_per_day_percent;

    double average_page_size_bytes;
} RRDENG_SIZE_STATS;

struct time_and_count {
    size_t count;
    usec_t usec;
};

static ALWAYS_INLINE void time_and_count_add(struct time_and_count *tc, usec_t dt) {
    __atomic_add_fetch(&tc->count, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&tc->usec, dt, __ATOMIC_RELAXED);
}

struct rrdeng_cache_efficiency_stats {
    PAD64(size_t) queries_planned_with_gaps;
    PAD64(size_t) queries_executed_with_gaps;

    PAD64(size_t) currently_running_queries;

    // query planner output of the queries
    PAD64(size_t) pages_total;
    PAD64(size_t) pages_to_load_from_disk;
    PAD64(size_t) extents_loaded_from_disk;

    // pages metadata sources
    PAD64(size_t) pages_meta_source_main_cache;
    PAD64(size_t) pages_meta_source_open_cache;
    PAD64(size_t) pages_meta_source_journal_v2;

    // preloading
    PAD64(size_t) page_next_wait_failed;
    PAD64(size_t) page_next_wait_loaded;
    PAD64(size_t) page_next_nowait_failed;
    PAD64(size_t) page_next_nowait_loaded;

    // pages data sources
    PAD64(size_t) pages_data_source_main_cache;
    PAD64(size_t) pages_data_source_main_cache_at_pass4;
    PAD64(size_t) pages_data_source_disk;
    PAD64(size_t) pages_data_source_extent_cache;              // loaded by a cached extent

    // cache hits at different points
    PAD64(size_t) pages_load_ok_loaded_but_cache_hit_while_inserting; // found in cache while inserting it (conflict)

    // loading
    PAD64(size_t) pages_load_extent_merged;
    PAD64(size_t) pages_load_ok_uncompressed;
    PAD64(size_t) pages_load_ok_compressed;
    PAD64(size_t) pages_load_fail_invalid_page_in_extent;
    PAD64(size_t) pages_load_fail_cant_mmap_extent;
    PAD64(size_t) pages_load_fail_datafile_not_available;
    PAD64(size_t) pages_load_fail_unroutable;
    PAD64(size_t) pages_load_fail_not_found;
    PAD64(size_t) pages_load_fail_invalid_extent;
    PAD64(size_t) pages_load_fail_cancelled;

    // count of queries and times spent in them
    PAD64(struct time_and_count) prep_time_to_route_sync;
    PAD64(struct time_and_count) prep_time_to_route_syncfirst;
    PAD64(struct time_and_count) prep_time_to_route_async;
    PAD64(struct time_and_count) prep_time_in_main_cache_lookup;
    PAD64(struct time_and_count) prep_time_in_open_cache_lookup;
    PAD64(struct time_and_count) prep_time_in_journal_v2_lookup;
    PAD64(struct time_and_count) prep_time_in_pass4_lookup;

    // timings the query thread experiences
    PAD64(struct time_and_count) query_time_init;
    PAD64(struct time_and_count) query_time_wait_for_prep;
    PAD64(struct time_and_count) query_time_to_slow_disk_next_page;
    PAD64(struct time_and_count) query_time_to_fast_disk_next_page;
    PAD64(struct time_and_count) query_time_to_slow_preload_next_page;
    PAD64(struct time_and_count) query_time_to_fast_preload_next_page;

    // query issues
    PAD64(size_t) pages_zero_time_skipped;
    PAD64(size_t) pages_past_time_skipped;
    PAD64(size_t) pages_overlapping_skipped;
    PAD64(size_t) pages_invalid_size_skipped;
    PAD64(size_t) pages_invalid_update_every_fixed;
    PAD64(size_t) pages_invalid_entries_fixed;

    // database events
    PAD64(size_t) journal_v2_mapped;
    PAD64(size_t) journal_v2_unmapped;
    PAD64(size_t) datafile_creation_started;
    PAD64(size_t) datafile_deletion_started;
    PAD64(size_t) datafile_deletion_spin;
    PAD64(size_t) journal_v2_indexing_started;
    PAD64(size_t) metrics_retention_started;
};

typedef enum rrdeng_mem {
    RRDENG_MEM_PGC = 0,
    RRDENG_MEM_PGD,
    RRDENG_MEM_MRG,
    RRDENG_MEM_OPCODES,
    RRDENG_MEM_HANDLES,
    RRDENG_MEM_DESCRIPTORS,
    RRDENG_MEM_WORKERS,
    RRDENG_MEM_PDC,
    RRDENG_MEM_XT_IO,
    RRDENG_MEM_EPDL,
    RRDENG_MEM_DEOL,
    RRDENG_MEM_PD,

    // terminator
    RRDENG_MEM_MAX,
} RRDENG_MEM;

struct rrdeng_buffer_sizes {
    struct aral_statistics *as[RRDENG_MEM_MAX];

    size_t wal;
    size_t xt_buf;
};

struct rrdeng_buffer_sizes rrdeng_pulse_memory_sizes(void);
struct rrdeng_cache_efficiency_stats rrdeng_get_cache_efficiency_stats(void);

RRDENG_SIZE_STATS rrdeng_size_statistics(struct rrdengine_instance *ctx);
size_t rrdeng_collectors_running(struct rrdengine_instance *ctx);

#endif /* NETDATA_RRDENGINEAPI_H */

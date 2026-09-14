// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DBENGINE_STATS_H
#define NETDATA_DBENGINE_STATS_H

#include "libnetdata/libnetdata.h"

// What the engine publishes about itself. The engine keeps these counters and sizes; whoever embeds
// it reads them through the getters below, as snapshots, whenever it wants to chart or report them.
// Nothing here is pushed: the engine has no idea who reads it.

struct rrdengine_instance;

// ---------------------------------------------------------------------------------------------------------------------
// per-tier size and shape statistics

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

RRDENG_SIZE_STATS rrdeng_size_statistics(struct rrdengine_instance *ctx);

// the legacy per-tier counters array (RRDENG_NR_STATS entries)
#define RRDENG_NR_STATS (38)
void rrdeng_get_37_statistics(struct rrdengine_instance *ctx, unsigned long long *array);

// ---------------------------------------------------------------------------------------------------------------------
// query / cache efficiency (process-wide, running totals)

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

struct rrdeng_cache_efficiency_stats rrdeng_get_cache_efficiency_stats(void);

// ---------------------------------------------------------------------------------------------------------------------
// memory: the engine's ARAL statistics, one per RRDENG_MEM slot, plus its non-ARAL buffers

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
    RRDENG_MEM_EPDL_EXTENT,

    // terminator
    RRDENG_MEM_MAX,
} RRDENG_MEM;

struct rrdeng_buffer_sizes {
    struct aral_statistics *as[RRDENG_MEM_MAX];

    size_t wal;
    size_t xt_buf;
};

struct rrdeng_buffer_sizes rrdeng_get_memory_sizes(void);
const char *rrdeng_mem_name(RRDENG_MEM idx);   // the chart name of each slot

// ---------------------------------------------------------------------------------------------------------------------
// tier-0 gorilla compression counters, kept by the engine since process start; a snapshot of the
// running totals (the daemon charts the deltas)
struct rrdeng_gorilla_stats {
    uint64_t hot_buffers_added;         // gorilla buffers allocated for pages being collected
    uint64_t tier0_disk_actual_bytes;   // bytes the flushed pages occupy on disk
    uint64_t tier0_disk_optimal_bytes;  // bytes they would occupy with perfectly sized buffers
    uint64_t tier0_disk_original_bytes; // bytes of the uncompressed samples they hold
};
struct rrdeng_gorilla_stats rrdeng_get_gorilla_stats(void);

#endif // NETDATA_DBENGINE_STATS_H

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DBENGINE_STATS_H
#define NETDATA_DBENGINE_STATS_H

#include "libnetdata/libnetdata.h"
#include "database/storage-engines/dbengine/include/dbengine/dbengine-config.h"

#ifdef __cplusplus
extern "C" {
#endif

// What the engine publishes about itself. The engine keeps these counters and sizes; whoever embeds
// it reads them through the getters below, as snapshots, whenever it wants to chart or report them.
// Nothing here is pushed: the engine has no idea who reads it.

// ---------------------------------------------------------------------------------------------------------------------
// per-tier size and shape statistics

struct dbengine_size_stats {
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
};

struct dbengine_size_stats dbengine_get_size_stats(DBENGINE_TIER *tier);

// the legacy per-tier counters array (DBENGINE_STATS_COUNT entries)
#define DBENGINE_STATS_COUNT (38)
void dbengine_get_stats(DBENGINE_TIER *tier, unsigned long long *array);

// ---------------------------------------------------------------------------------------------------------------------
// page cache statistics: the engine runs three caches (pages, open datafiles, extents), each reports this

// CACHE COMPILE TIME CONFIGURATION (the struct below depends on it, so it lives with the struct)
// #define PGC_COUNT_POINTS_COLLECTED 1

struct dbengine_cache_size_histogram_entry {
    size_t upto;
    size_t count;
};

#define DBENGINE_CACHE_SIZE_HISTOGRAM_ENTRIES 15
#define DBENGINE_CACHE_QUEUE_HOT   0
#define DBENGINE_CACHE_QUEUE_DIRTY 1
#define DBENGINE_CACHE_QUEUE_CLEAN 2

struct dbengine_cache_size_histogram {
    struct dbengine_cache_size_histogram_entry array[DBENGINE_CACHE_SIZE_HISTOGRAM_ENTRIES];
};

struct dbengine_cache_queue_stats {
    struct dbengine_cache_size_histogram size_histogram;

    PAD64(size_t) entries;
    PAD64(int64_t) size;

    PAD64(size_t) max_entries;
    PAD64(int64_t) max_size;

    PAD64(size_t) added_entries;
    PAD64(int64_t) added_size;

    PAD64(size_t) removed_entries;
    PAD64(int64_t) removed_size;
};

struct dbengine_cache_stats {
    PAD64(int64_t) wanted_cache_size;
    PAD64(int64_t) current_cache_size;

    // ----------------------------------------------------------------------------------------------------------------
    // volume

    PAD64(size_t) entries;                 // all the entries (includes clean, dirty, hot)
    PAD64(int64_t) size;                   // all the entries (includes clean, dirty, hot)

    PAD64(size_t) referenced_entries;      // all the entries currently referenced
    PAD64(int64_t) referenced_size;        // all the entries currently referenced

    PAD64(size_t) added_entries;
    PAD64(int64_t) added_size;

    PAD64(size_t) removed_entries;
    PAD64(int64_t) removed_size;

#ifdef PGC_COUNT_POINTS_COLLECTED
    PAD64(size_t) points_collected;
#endif

    // ----------------------------------------------------------------------------------------------------------------
    // migrations

    PAD64(size_t) evicting_entries;
    PAD64(int64_t) evicting_size;

    PAD64(size_t) flushing_entries;
    PAD64(int64_t) flushing_size;

    PAD64(size_t) hot2dirty_entries;
    PAD64(int64_t) hot2dirty_size;

    PAD64(size_t) hot_empty_pages_evicted_immediately;
    PAD64(size_t) hot_empty_pages_evicted_later;

    // ----------------------------------------------------------------------------------------------------------------
    // workload

    PAD64(size_t) acquires;
    PAD64(size_t) releases;

    PAD64(size_t) acquires_for_deletion;

    PAD64(size_t) searches_exact;
    PAD64(size_t) searches_exact_hits;
    PAD64(size_t) searches_exact_misses;

    PAD64(size_t) searches_closest;
    PAD64(size_t) searches_closest_hits;
    PAD64(size_t) searches_closest_misses;

    PAD64(size_t) flushes_completed;
    PAD64(int64_t) flushes_completed_size;
    PAD64(int64_t) flushes_cancelled_size;

    // ----------------------------------------------------------------------------------------------------------------
    // critical events

    PAD64(size_t) events_cache_under_severe_pressure;
    PAD64(size_t) events_cache_needs_space_aggressively;
    PAD64(size_t) events_flush_critical;

    // ----------------------------------------------------------------------------------------------------------------
    // worker threads

    PAD64(size_t) p2_workers_search;
    PAD64(size_t) p2_workers_add;
    PAD64(size_t) p0_workers_evict; // priority 0, we always need this when inline evictions are enabled
    PAD64(size_t) p2_workers_flush;
    PAD64(size_t) p2_workers_jv2_flush;
    PAD64(size_t) p2_workers_hot2dirty;

    // ----------------------------------------------------------------------------------------------------------------
    // waste events

    // waste events - spins
    PAD64(size_t) p2_waste_insert_spins;
    PAD64(size_t) p2_waste_evict_useless_spins;

    // waste events - eviction
    PAD64(size_t) p2_waste_evict_relocated;
    PAD64(size_t) p2_waste_evict_thread_signals;
    PAD64(size_t) p2_waste_evictions_inline_on_add;
    PAD64(size_t) p2_waste_evictions_inline_on_release;

    // waste events - flushing
    PAD64(size_t) p2_waste_flush_on_add;
    PAD64(size_t) p2_waste_flush_on_release;
    PAD64(size_t) p2_waste_flushes_cancelled;

    // ----------------------------------------------------------------------------------------------------------------
    // per queue statistics

    struct dbengine_cache_queue_stats queues[3];
};

typedef enum dbengine_cache {
    DBENGINE_CACHE_MAIN = 0,      // the pages
    DBENGINE_CACHE_OPEN,          // the open datafiles
    DBENGINE_CACHE_EXTENT,        // the compressed extents
} DBENGINE_CACHE;

// A snapshot of one cache; false, with *out zeroed, when that cache does not exist (no tier came up yet, or the
// caches were destroyed). The counters are copied as a whole, not under a lock and not atomically: a counter may
// be mid-update, and on a 32-bit target a 64-bit one may tear. Good enough for charts, not for accounting.
bool dbengine_get_cache_stats(DBENGINE_CACHE which, struct dbengine_cache_stats *out);

// pages of the main cache still to be written: hot (collected) plus dirty (waiting for a flush); 0 without a cache
size_t dbengine_pages_pending_flush(void);

// bytes lost to alignment inside page data allocations (process-wide)
size_t dbengine_page_padding_bytes(void);

// ---------------------------------------------------------------------------------------------------------------------
// metrics registry statistics

struct dbengine_metrics_registry_stats {
    // --- sampled lock-free by dbengine_get_metrics_registry_stats() ---
    // Writers use relaxed atomics. The padded fields below are updated on hotter reader/writer paths.

    size_t entries;
    int64_t size;    // total memory used, with indexing

    size_t additions;
    size_t additions_duplicate;

    size_t deletions;
    size_t delete_having_retention_or_referenced;
    size_t delete_misses;

    // --- hot counters --- multiple readers / writers

    PAD64(ssize_t) entries_acquired;
    PAD64(ssize_t) current_references;

    PAD64(size_t) search_hits;
    PAD64(size_t) search_misses;

    PAD64(size_t) writers;
    PAD64(size_t) writers_conflicts;
};

// A snapshot of the registry; false, with *out zeroed, when it does not exist
bool dbengine_get_metrics_registry_stats(struct dbengine_metrics_registry_stats *out);

// ---------------------------------------------------------------------------------------------------------------------
// query / cache efficiency (process-wide, running totals)

struct dbengine_time_and_count {
    size_t count;
    usec_t usec;
};

struct dbengine_cache_efficiency_stats {
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
    PAD64(struct dbengine_time_and_count) prep_time_to_route_sync;
    PAD64(struct dbengine_time_and_count) prep_time_to_route_syncfirst;
    PAD64(struct dbengine_time_and_count) prep_time_to_route_async;
    PAD64(struct dbengine_time_and_count) prep_time_in_main_cache_lookup;
    PAD64(struct dbengine_time_and_count) prep_time_in_open_cache_lookup;
    PAD64(struct dbengine_time_and_count) prep_time_in_journal_v2_lookup;
    PAD64(struct dbengine_time_and_count) prep_time_in_pass4_lookup;

    // timings the query thread experiences
    PAD64(struct dbengine_time_and_count) query_time_init;
    PAD64(struct dbengine_time_and_count) query_time_wait_for_prep;
    PAD64(struct dbengine_time_and_count) query_time_to_slow_disk_next_page;
    PAD64(struct dbengine_time_and_count) query_time_to_fast_disk_next_page;
    PAD64(struct dbengine_time_and_count) query_time_to_slow_preload_next_page;
    PAD64(struct dbengine_time_and_count) query_time_to_fast_preload_next_page;

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

struct dbengine_cache_efficiency_stats dbengine_get_cache_efficiency_stats(void);

// ---------------------------------------------------------------------------------------------------------------------
// memory: the engine's ARAL statistics, one per DBENGINE_MEM slot, plus its non-ARAL buffers

typedef enum dbengine_mem {
    DBENGINE_MEM_PGC = 0,
    DBENGINE_MEM_PGD,
    DBENGINE_MEM_MRG,
    DBENGINE_MEM_OPCODES,
    DBENGINE_MEM_HANDLES,
    DBENGINE_MEM_DESCRIPTORS,
    DBENGINE_MEM_WORKERS,
    DBENGINE_MEM_PDC,
    DBENGINE_MEM_XT_IO,
    DBENGINE_MEM_EPDL,
    DBENGINE_MEM_DEOL,
    DBENGINE_MEM_PD,
    DBENGINE_MEM_EPDL_EXTENT,

    // terminator
    DBENGINE_MEM_MAX,
} DBENGINE_MEM;

struct dbengine_buffer_sizes {
    struct aral_statistics *as[DBENGINE_MEM_MAX];

    size_t wal;
    size_t xt_buf;
};

struct dbengine_buffer_sizes dbengine_get_memory_sizes(void);
const char *dbengine_mem_name(DBENGINE_MEM idx);   // the chart name of each slot

// ---------------------------------------------------------------------------------------------------------------------
// tier-0 gorilla compression counters, kept by the engine while compression_statistics is set; a snapshot
// of the running totals (the daemon charts the buffer count incrementally and the byte totals as they are)
struct dbengine_gorilla_stats {
    uint64_t hot_buffers_added;         // gorilla buffers allocated for pages being collected
    uint64_t tier0_disk_actual_bytes;   // bytes the flushed pages occupy on disk
    uint64_t tier0_disk_optimal_bytes;  // bytes they would occupy with perfectly sized buffers
    uint64_t tier0_disk_original_bytes; // bytes of the uncompressed samples they hold
};
struct dbengine_gorilla_stats dbengine_get_gorilla_stats(void);

#ifdef __cplusplus
}
#endif

#endif // NETDATA_DBENGINE_STATS_H

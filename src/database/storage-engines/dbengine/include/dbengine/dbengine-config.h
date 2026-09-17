// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DBENGINE_CONFIG_H
#define NETDATA_DBENGINE_CONFIG_H

#include "libnetdata/libnetdata.h"

#ifdef __cplusplus
extern "C" {
#endif

struct rrdengine_instance;

// Receives one metric the embedder already knows, on the tier it belongs to; passed to preload_metrics() by
// the engine.
typedef void (*dbengine_preload_add_fn)(void *mrg, struct rrdengine_instance *ctx, nd_uuid_t *uuid);

// The storage engine's process-wide configuration.
//
// Whoever embeds the engine (the daemon; a test) fills one of these from its own sources and
// hands it to dbengine_init() exactly once, before the first dbengine_instance_init(). The engine keeps a
// private copy and reads nothing else afterwards. Per-tier settings (path, quota, retention,
// page type) travel with dbengine_instance_init() instead.
struct dbengine_config {
    // caches - read once, when the first tier brings up the caches shared by all tiers
    size_t page_cache_mb;                       // [db] dbengine page cache size
    size_t extent_cache_mb;                     // [db] dbengine extent cache size
    uint64_t out_of_memory_protection_bytes;    // [db] dbengine out of memory protection; 0 disables it
    bool use_all_ram_for_caches;                // [db] dbengine use all ram for caches

    bool cache_statistics;                      // keep per-cache statistics (the daemon's pulse setting)
    bool compression_statistics;                // count gorilla buffers and tier-0 compression bytes (pulse extended)

    // sizing of partitions, evictors, flushers and metric-registry loaders
    size_t cpus;                                // 0 = detect the system's cpus
    bool arals_for_large_pages;                 // keep ARALs for page sizes above 4KiB (a parent expects such pages)

    // files
    bool direct_io;                             // [db] dbengine use direct io
    unsigned pages_per_extent;                  // [db] dbengine pages per extent; 0 = default, > MAX_PAGES_PER_EXTENT is fatal
    bool journal_integrity_check;               // [db] dbengine enable journal integrity check
    time_t journal_v2_unmount_time_s;           // [db] dbengine journal v2 unmount time

    // runtime
    time_t default_update_every_s;              // used for a metric whose own update_every is unknown; 0 = 1, < 0 is fatal
    int libuv_worker_threads;                   // size of the libuv thread pool the engine dispatches work into;
                                                // 0 = the engine's default (16, or 8 on 32-bit)
    int reserved_libuv_worker_threads;          // pool threads the engine must leave free for the embedder's own work

    // services the embedder may provide; NULL = not provided
    void (*on_db_rotation)(void);               // a tier deleted its oldest datafile: retention just shrank
    size_t (*preload_metrics)(void *mrg, dbengine_preload_add_fn add);
                                                // called once, when the metrics registry is created and before any
                                                // tier loads its journals: feed every metric uuid the embedder already
                                                // knows through add(), so the registry is populated in one pass instead
                                                // of metric by metric as the journals are read; returns the count
};

#define DBENGINE_DEFAULT_PAGES_PER_EXTENT (109)

// Page types. The value is the page-type byte of the on-disk format (rrddiskprotocol.h), so an
// existing type is never renumbered; a new one takes the next value.
#define RRDENG_PAGE_TYPE_ARRAY_32BIT    (0)
#define RRDENG_PAGE_TYPE_ARRAY_TIER1    (1)
#define RRDENG_PAGE_TYPE_GORILLA_32BIT  (2)

// the floor the engine enforces on a tier's disk space (dbengine_instance_init() raises a smaller value), the floor
// the embedder is expected to keep the page cache above (the engine does not check it: below it the cache split
// underflows), and the disk-space default the engine leaves to the embedder
#define RRDENG_MIN_PAGE_CACHE_SIZE_MB (8)
#define RRDENG_MIN_DISK_SPACE_MB (25)
#define RRDENG_DEFAULT_TIER_DISK_SPACE_MB (1024)

#if defined(ENV32BIT)
#define DBENGINE_CONFIG_DEFAULT_PAGE_CACHE_MB (16)
#else
#define DBENGINE_CONFIG_DEFAULT_PAGE_CACHE_MB (32)
#endif

// The compiled defaults: the baseline a caller adjusts before dbengine_init(), which resolves the
// 0-means-default fields (cpus, libuv_worker_threads) to concrete values.
#define DBENGINE_CONFIG_DEFAULTS {                              \
    .page_cache_mb = DBENGINE_CONFIG_DEFAULT_PAGE_CACHE_MB,     \
    .extent_cache_mb = 0,                                       \
    .out_of_memory_protection_bytes = 0,                        \
    .use_all_ram_for_caches = false,                            \
    .cache_statistics = true,                                   \
    .compression_statistics = false,                            \
    .cpus = 0,                                                  \
    .arals_for_large_pages = false,                             \
    .direct_io = true,                                          \
    .pages_per_extent = DBENGINE_DEFAULT_PAGES_PER_EXTENT,      \
    .journal_integrity_check = false,                           \
    .journal_v2_unmount_time_s = 120,                           \
    .default_update_every_s = 1,                                \
    .libuv_worker_threads = 0,                                  \
    .reserved_libuv_worker_threads = 0,                         \
    .on_db_rotation = NULL,                                     \
    .preload_metrics = NULL,                                    \
}

// One tier's configuration, handed to dbengine_instance_init(); the engine copies what it needs.
struct rrdeng_tier_config {
    size_t tier;                                // 0 is the tier collectors write to; higher tiers aggregate the one below
    const char *dbfiles_path;                   // directory of this tier's datafiles and journals
    unsigned disk_space_mb;                     // 0 = no disk quota
    time_t max_retention_s;                     // 0 = no time limit
    uint8_t page_type;                          // tier 0: RRDENG_PAGE_TYPE_GORILLA_32BIT or RRDENG_PAGE_TYPE_ARRAY_32BIT;
                                                // higher tiers hold aggregates and must use RRDENG_PAGE_TYPE_ARRAY_TIER1
    size_t grouping;                            // points of tier 0 that make one point of this tier (1 for tier 0)
};

// Copy cfg into the engine, resolving the 0-means-default fields (cpus, default_update_every_s,
// pages_per_extent, libuv_worker_threads). Fatal when the libuv pool is not larger than the threads
// reserved for the embedder, when pages_per_extent exceeds what the extent format holds, or when
// default_update_every_s is negative. Call it once, from one thread, before the first dbengine_instance_init();
// a second call with an equal configuration is a no-op, with a different one it is fatal.
void dbengine_init(const struct dbengine_config *cfg);

// Release the references preload_metrics() left on the registry, once every tier has come up (after the last
// dbengine_readiness_wait()): until then they keep preloaded metrics from being evicted before their journals are
// read. A no-op when there is no registry.
void dbengine_preload_release(void);

#ifdef __cplusplus
}
#endif

#endif // NETDATA_DBENGINE_CONFIG_H

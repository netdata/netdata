// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdengine.h"

// Written once by dbengine_init(), before any tier exists; read-only afterwards.
struct dbengine_config dbengine_cfg = DBENGINE_CONFIG_DEFAULTS;
static bool dbengine_cfg_initialized = false;

static bool dbengine_config_equal(const struct dbengine_config *a, const struct dbengine_config *b) {
    return a->page_cache_mb == b->page_cache_mb &&
           a->extent_cache_mb == b->extent_cache_mb &&
           a->out_of_memory_protection_bytes == b->out_of_memory_protection_bytes &&
           a->use_all_ram_for_caches == b->use_all_ram_for_caches &&
           a->cache_statistics == b->cache_statistics &&
           a->compression_statistics == b->compression_statistics &&
           a->cpus == b->cpus &&
           a->arals_for_large_pages == b->arals_for_large_pages &&
           a->direct_io == b->direct_io &&
           a->pages_per_extent == b->pages_per_extent &&
           a->journal_integrity_check == b->journal_integrity_check &&
           a->journal_v2_unmount_time_s == b->journal_v2_unmount_time_s &&
           a->default_update_every_s == b->default_update_every_s &&
           a->libuv_worker_threads == b->libuv_worker_threads &&
           a->reserved_libuv_worker_threads == b->reserved_libuv_worker_threads &&
           a->on_db_rotation == b->on_db_rotation &&
           a->preload_metrics == b->preload_metrics;
}

// the 0-means-default fields become concrete values here, so the engine never has to re-check them
static void dbengine_config_resolve(struct dbengine_config *cfg) {
    if(!cfg->cpus)
        cfg->cpus = os_get_system_cpus();
    if(cfg->cpus < 1)
        cfg->cpus = 1;

    if(!cfg->default_update_every_s)
        cfg->default_update_every_s = 1;

    if(!cfg->libuv_worker_threads)
        cfg->libuv_worker_threads = DBENGINE_CONFIG_DEFAULT_WORKER_THREADS;

    // the dispatcher keeps the reserved threads free for the embedder; a pool that small cannot host the engine
    if(cfg->reserved_libuv_worker_threads < 0 || cfg->libuv_worker_threads <= cfg->reserved_libuv_worker_threads)
        fatal("DBENGINE: a libuv worker pool of %d threads is too small (%d are reserved)",
              cfg->libuv_worker_threads, cfg->reserved_libuv_worker_threads);
}

void dbengine_init(const struct dbengine_config *cfg) {
    if(!cfg)
        fatal("DBENGINE: dbengine_init() called without a configuration");

    struct dbengine_config resolved = *cfg;
    dbengine_config_resolve(&resolved);

    if(dbengine_cfg_initialized) {
        if(!dbengine_config_equal(&resolved, &dbengine_cfg))
            fatal("DBENGINE: dbengine_init() called again with a different configuration");
        return;
    }

    dbengine_cfg = resolved;
    dbengine_cfg_initialized = true;
}

bool dbengine_initialized(void) {
    return dbengine_cfg_initialized;
}

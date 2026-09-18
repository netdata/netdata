// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdengine.h"

// the smallest libuv pool the engine assumes when the embedder does not say
#if defined(ENV32BIT)
#define DBENGINE_CONFIG_DEFAULT_WORKER_THREADS (8)
#else
#define DBENGINE_CONFIG_DEFAULT_WORKER_THREADS (16)
#endif

// Written before the engine comes up, by dbengine_init() or (for the tests that need only the configuration) by
// dbengine_config_set(); read-only afterwards.
struct dbengine_config dbengine_cfg = DBENGINE_CONFIG_DEFAULTS;

// the 0-means-default fields become concrete values here, so the engine never has to re-check them
static void dbengine_config_resolve(struct dbengine_config *cfg) {
    if(!cfg->cpus)
        cfg->cpus = os_get_system_cpus();
    if(cfg->cpus < 1)
        cfg->cpus = 1;

    if(!cfg->default_update_every_s)
        cfg->default_update_every_s = 1;

    // the extent builder collects up to pages_per_extent descriptors into a MAX_PAGES_PER_EXTENT array
    if(!cfg->pages_per_extent)
        cfg->pages_per_extent = DBENGINE_DEFAULT_PAGES_PER_EXTENT;

    if(cfg->pages_per_extent > MAX_PAGES_PER_EXTENT)
        fatal("DBENGINE: %u pages per extent requested, the extent format allows at most %u",
              cfg->pages_per_extent, (unsigned)MAX_PAGES_PER_EXTENT);

    // a negative interval would turn into a huge unsigned granularity downstream
    if(cfg->default_update_every_s < 0)
        fatal("DBENGINE: a negative default update every (%lld s) makes no sense",
              (long long)cfg->default_update_every_s);

    if(!cfg->libuv_worker_threads)
        cfg->libuv_worker_threads = DBENGINE_CONFIG_DEFAULT_WORKER_THREADS;

    // the dispatcher keeps the reserved threads free for the embedder; a pool that small cannot host the engine
    if(cfg->reserved_libuv_worker_threads < 0 || cfg->libuv_worker_threads <= cfg->reserved_libuv_worker_threads)
        fatal("DBENGINE: a libuv worker pool of %d threads is too small (%d are reserved)",
              cfg->libuv_worker_threads, cfg->reserved_libuv_worker_threads);
}

void dbengine_config_set(const struct dbengine_config *cfg) {
    if(!cfg)
        fatal("DBENGINE: the engine was given no configuration");

    struct dbengine_config resolved = *cfg;
    dbengine_config_resolve(&resolved);

    dbengine_cfg = resolved;
}

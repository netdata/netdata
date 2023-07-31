// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrddim_eng.h"

struct dbengine_initialization {
    netdata_thread_t thread;
    char path[FILENAME_MAX + 1];
    int disk_space_mb;
    size_t tier;
    int ret;
};

static void *dbengine_tier_init(void *ptr) {
    struct dbengine_initialization *dbi = ptr;
    dbi->ret = rrdeng_tier_init(NULL, dbi->path, dbi->disk_space_mb, dbi->tier);
    return ptr;
}

bool dbengine_init(const char *hostname, dbengine_config_t *cfg) {
    struct dbengine_initialization tiers_init[STORAGE_ENGINE_TIERS] = {};

    for (size_t tier = 0; tier < cfg->storage_tiers ;tier++) {
        char dbenginepath[FILENAME_MAX + 1];

        if (tier == 0)
            snprintfz(dbenginepath, FILENAME_MAX, "%s/dbengine", cfg->base_path);
        else
            snprintfz(dbenginepath, FILENAME_MAX, "%s/dbengine-tier%zu", cfg->base_path, tier);

        int ret = mkdir(dbenginepath, 0775);
        if (ret != 0 && errno != EEXIST) {
            netdata_log_error("DBENGINE on '%s': cannot create directory '%s'", hostname, dbenginepath);
            break;
        }

        tiers_init[tier].disk_space_mb = cfg->multidb_disk_quota_mb[tier];
        tiers_init[tier].tier = tier;
        strncpyz(tiers_init[tier].path, dbenginepath, FILENAME_MAX);
        tiers_init[tier].ret = 0;

        if (cfg->parallel_initialization) {
            char tag[NETDATA_THREAD_TAG_MAX + 1];
            snprintfz(tag, NETDATA_THREAD_TAG_MAX, "DBENGINIT[%zu]", tier);
            netdata_thread_create(&tiers_init[tier].thread, tag, NETDATA_THREAD_OPTION_JOINABLE,
                                  dbengine_tier_init, &tiers_init[tier]);
        }
        else
            dbengine_tier_init(&tiers_init[tier]);
    }

    size_t created_tiers = 0;

    for (size_t tier = 0; tier < cfg->storage_tiers ;tier++) {
        void *ptr;

        if (cfg->parallel_initialization)
            netdata_thread_join(tiers_init[tier].thread, &ptr);

        if (tiers_init[tier].ret != 0) {
            netdata_log_error("DBENGINE on '%s': Failed to initialize multi-host database tier %zu on path '%s'",
                              hostname,
                              tiers_init[tier].tier,
                              tiers_init[tier].path);
        }
        else if (created_tiers == tier)
            created_tiers++;
    }

    if (created_tiers && created_tiers < cfg->storage_tiers) {
        netdata_log_error("DBENGINE on '%s': Managed to create %zu tiers instead of %zu. Continuing with %zu available.",
                          hostname,
                          created_tiers,
                          cfg->storage_tiers,
                          created_tiers);
        cfg->storage_tiers = created_tiers;
    }
    else if (!created_tiers)
        fatal("DBENGINE on '%s', failed to initialize databases at '%s'.", hostname, cfg->base_path);

    for (size_t tier = 0; tier < cfg->storage_tiers ;tier++)
        rrdeng_readiness_wait(cfg->multidb_ctx[tier]);

    return true;
}

dbengine_config_t dbengine_cfg = {
    .base_path = CACHE_DIR,

    .check_journal  = CONFIG_BOOLEAN_NO,
    .use_direct_io = true,
    .parallel_initialization = false,

    .disk_quota_mb = 256,

    #if defined(ENV32BIT)
        .page_cache_mb = 16,
        .extent_cache_mb = 0,
    #else
        .page_cache_mb = 32,
        .extent_cache_mb = 0,
    #endif

    .pages_per_extent = 64,

    .page_type_size = {
        sizeof(storage_number),
        sizeof(storage_number_tier1_t)
    },

    .storage_tiers = 3,

    .multidb_ctx = {
        NULL,
        NULL,
        NULL,
        NULL,
        NULL,
    },

    .multidb_disk_quota_mb = {
        256,
        128,
        64,
        32,
        16,
    },

    .storage_tiers_grouping_iterations = {
        1,
        60,
        60,
        60,
        60
    },

    .storage_tiers_backfill = {
        STORAGE_TIER_BACKFILL_NEW,
        STORAGE_TIER_BACKFILL_NEW,
        STORAGE_TIER_BACKFILL_NEW,
        STORAGE_TIER_BACKFILL_NEW,
        STORAGE_TIER_BACKFILL_NEW,
    },

#if defined(ENV32BIT)
    .tier_page_size = {
        2048,
        1024,
        192,
        192,
        192
    },
#else
    .tier_page_size = {
        4096,
        2048,
        384,
        384,
        384
    },
#endif
};

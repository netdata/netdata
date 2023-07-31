// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrddim_eng.h"

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

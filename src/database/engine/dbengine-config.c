// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdengine.h"

// Written once by dbengine_init(), before any tier exists; read-only afterwards.
struct dbengine_config dbengine_cfg = DBENGINE_CONFIG_DEFAULTS;
static bool dbengine_cfg_initialized = false;

void dbengine_config_defaults(struct dbengine_config *cfg) {
    *cfg = (struct dbengine_config)DBENGINE_CONFIG_DEFAULTS;
}

static bool dbengine_config_equal(const struct dbengine_config *a, const struct dbengine_config *b) {
    return a->page_cache_mb == b->page_cache_mb &&
           a->extent_cache_mb == b->extent_cache_mb &&
           a->out_of_memory_protection_bytes == b->out_of_memory_protection_bytes &&
           a->use_all_ram_for_caches == b->use_all_ram_for_caches &&
           a->pages_per_extent == b->pages_per_extent &&
           a->journal_integrity_check == b->journal_integrity_check &&
           a->journal_v2_unmount_time_s == b->journal_v2_unmount_time_s;
}

void dbengine_init(const struct dbengine_config *cfg) {
    if(!cfg)
        fatal("DBENGINE: dbengine_init() called without a configuration");

    if(dbengine_cfg_initialized) {
        if(!dbengine_config_equal(cfg, &dbengine_cfg))
            fatal("DBENGINE: dbengine_init() called again with a different configuration");
        return;
    }

    dbengine_cfg = *cfg;
    dbengine_cfg_initialized = true;
}

bool dbengine_initialized(void) {
    return dbengine_cfg_initialized;
}

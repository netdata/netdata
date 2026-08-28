// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DBENGINE_CONFIG_H
#define NETDATA_DBENGINE_CONFIG_H

#include "libnetdata/libnetdata.h"

// The storage engine's process-wide configuration.
//
// Whoever embeds the engine (the daemon; a test) fills one of these from its own sources and
// hands it to dbengine_init() exactly once, before the first rrdeng_init(). The engine keeps a
// private copy and reads nothing else afterwards. Per-tier settings (path, quota, retention,
// page type) travel with rrdeng_init() instead.
struct dbengine_config {
    // caches - read once, when the first tier brings up the caches shared by all tiers
    size_t page_cache_mb;                       // [db] dbengine page cache size
    size_t extent_cache_mb;                     // [db] dbengine extent cache size
    uint64_t out_of_memory_protection_bytes;    // [db] dbengine out of memory protection; 0 disables it
    bool use_all_ram_for_caches;                // [db] dbengine use all ram for caches

    // files
    unsigned pages_per_extent;                  // [db] dbengine pages per extent
    bool journal_integrity_check;               // [db] dbengine enable journal integrity check
    time_t journal_v2_unmount_time_s;           // [db] dbengine journal v2 unmount time
};

#define DEFAULT_PAGES_PER_EXTENT (109)

#if defined(ENV32BIT)
#define DBENGINE_CONFIG_DEFAULT_PAGE_CACHE_MB (16)
#else
#define DBENGINE_CONFIG_DEFAULT_PAGE_CACHE_MB (32)
#endif

// The compiled defaults: the baseline a caller adjusts before dbengine_init(), and what the
// engine runs with when nobody calls it (its own unit tests).
#define DBENGINE_CONFIG_DEFAULTS {                              \
    .page_cache_mb = DBENGINE_CONFIG_DEFAULT_PAGE_CACHE_MB,     \
    .extent_cache_mb = 0,                                       \
    .out_of_memory_protection_bytes = 0,                        \
    .use_all_ram_for_caches = false,                            \
    .pages_per_extent = DEFAULT_PAGES_PER_EXTENT,               \
    .journal_integrity_check = false,                           \
    .journal_v2_unmount_time_s = 120,                           \
}

void dbengine_config_defaults(struct dbengine_config *cfg);

// Copy cfg into the engine. Call it once, from one thread, before the first rrdeng_init();
// a second call with an equal configuration is a no-op, with a different one it is fatal.
void dbengine_init(const struct dbengine_config *cfg);

#endif // NETDATA_DBENGINE_CONFIG_H

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PAGECACHE_H
#define NETDATA_PAGECACHE_H

#include "rrdengine.h"

extern struct mrg *main_mrg;
extern struct pgc *main_cache;
extern struct pgc *open_cache;

/* Forward declarations */
struct rrdengine_instance;
struct extent_info;
struct rrdeng_page_descr;

#define INVALID_TIME (0)
#define MAX_PAGE_CACHE_FETCH_RETRIES (3)
#define PAGE_CACHE_FETCH_WAIT_TIMEOUT (3)

extern struct rrdeng_cache_efficiency_stats rrdeng_cache_efficiency_stats;

struct rrdeng_page_descr {
    uuid_t *id; /* never changes */
    usec_t start_time_ut;
    usec_t end_time_ut;
    uint32_t update_every_s:24;
    uint8_t type;
    uint32_t page_length;
    void *page;                 // Metrics are stored here
    uuid_t uuid;                // FIXME: Copy of uuid for extent flush
};

#define PAGE_INFO_SCRATCH_SZ (8)
struct rrdeng_page_info {
    uint8_t scratch[PAGE_INFO_SCRATCH_SZ]; /* scratch area to be used by page-cache users */

    usec_t start_time_ut;
    usec_t end_time_ut;
    uint32_t page_length;
};

struct pg_alignment {
    uint32_t page_length;
    uint32_t refcount;
};

struct rrdeng_query_handle;

time_t pg_cache_preload(struct rrdengine_instance *ctx, struct rrdeng_query_handle *handle, time_t start_time_t, time_t end_time_t);
struct pgc_page *pg_cache_lookup_next(struct rrdengine_instance *ctx, struct rrdeng_query_handle *handle, time_t start_time_t, time_t end_time_t, time_t *next_page_start_s);
void init_page_cache(void);

#endif /* NETDATA_PAGECACHE_H */

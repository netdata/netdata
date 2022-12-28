// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PAGECACHE_H
#define NETDATA_PAGECACHE_H

#include "rrdengine.h"

extern struct mrg *main_mrg;
extern struct pgc *main_cache;
extern struct pgc *open_cache;
extern struct pgc *extent_cache;
extern pthread_key_t query_key;

/* Forward declarations */
struct rrdengine_instance;
struct rrdeng_page_descr;

#define INVALID_TIME (0)
#define MAX_PAGE_CACHE_FETCH_RETRIES (3)
#define PAGE_CACHE_FETCH_WAIT_TIMEOUT (3)

extern struct rrdeng_cache_efficiency_stats rrdeng_cache_efficiency_stats;

struct rrdeng_page_descr {
    uuid_t *id; /* never changes */
    Word_t metric_id;
    usec_t start_time_ut;
    usec_t end_time_ut;
    uint8_t type;
    uint32_t update_every_s:24;
    uint32_t page_length;
    void *page;                 // Metrics are stored here
};

#define PAGE_INFO_SCRATCH_SZ (8)
struct rrdeng_page_info {
    uint8_t scratch[PAGE_INFO_SCRATCH_SZ]; /* scratch area to be used by page-cache users */

    usec_t start_time_ut;
    usec_t end_time_ut;
    uint32_t page_length;
};

struct pg_alignment {
    uint32_t page_position;
    uint32_t refcount;
};

struct rrdeng_query_handle;
struct page_details_control;

void pg_cache_preload(struct rrdengine_instance *ctx, struct rrdeng_query_handle *handle, time_t start_time_t, time_t end_time_t, int priority);
struct pgc_page *pg_cache_lookup_next(struct rrdengine_instance *ctx, struct page_details_control *pdc, time_t now_s, time_t last_update_every_s, size_t *entries);
void init_page_cache(void);

void pgc_open_add_hot_page(Word_t section, Word_t metric_id, time_t start_time_s, time_t end_time_s, time_t update_every_s, struct rrdengine_datafile *datafile, uint64_t extent_offset, unsigned extent_size, uint32_t page_length);

#endif /* NETDATA_PAGECACHE_H */

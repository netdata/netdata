// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PAGECACHE_H
#define NETDATA_PAGECACHE_H

#include "rrdengine.h"

struct pgc;

// the least the open and extent caches size themselves to, whatever the main cache reports, and what they
// settle on once the main cache is gone; the extent floor is also the least the engine reserves for it at start
#define OPEN_CACHE_MIN_SIZE   (2 * 1024 * 1024)
#define EXTENT_CACHE_MIN_SIZE (5 * 1024 * 1024)
// the share of the main cache's wanted size each of them follows
#define OPEN_CACHE_PERCENT    (5)
#define EXTENT_CACHE_PERCENT  (30)
int64_t dynamic_open_cache_size(struct pgc *cache);
int64_t dynamic_extent_cache_size(struct pgc *cache);

// what a cache that sizes itself from the main cache wants: `percent` of what the main cache wants plus the main
// cache's unused space, never below `floor_size`; `floor_size` alone when there is no main cache
int64_t dbengine_follower_cache_size(struct pgc *main_cache_or_null, int64_t percent, int64_t floor_size);

/* Forward declarations */
struct dbengine_tier;

#define INVALID_TIME (0)

struct page_descr_with_data {
    UUIDMAP_ID uuid_id;
    Word_t metric_id;
    usec_t start_time_ut;
    usec_t end_time_ut;
    uint8_t type;
    uint32_t update_every_s;
    uint32_t page_length;
    struct pgd *pgd;

    struct {
        struct page_descr_with_data *prev;
        struct page_descr_with_data *next;
    } link;
};

struct pg_alignment {
    uint32_t refcount;
};

struct dbengine_query_handle;
struct page_details_control;

void dbengine_prep_wait(struct page_details_control *pdc);
void dbengine_prep_query(struct page_details_control *pdc, bool worker);
void pg_cache_preload(struct dbengine_query_handle *handle);
struct pgc_page *pg_cache_lookup_next(struct dbengine_tier *ctx, struct page_details_control *pdc, time_t now_s, uint32_t last_update_every_s, size_t *entries);
void pgc_and_mrg_initialize(struct dbengine_engine *engine);

void pgc_open_add_hot_page(
    Word_t section,
    Word_t metric_id,
    UUIDMAP_ID uuid_id,
    time_t start_time_s,
    time_t end_time_s,
    uint32_t update_every_s,
    struct dbengine_datafile *datafile,
    uint64_t extent_offset,
    unsigned extent_size);

#endif /* NETDATA_PAGECACHE_H */

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PAGECACHE_H
#define NETDATA_PAGECACHE_H

#include "rrdengine.h"

extern struct mrg *main_mrg;
extern struct pgc *main_cache;
extern struct pgc *open_cache;
extern struct pgc *extent_cache;

/* Forward declarations */
struct rrdengine_instance;

#define INVALID_TIME (0)

extern struct rrdeng_cache_efficiency_stats rrdeng_cache_efficiency_stats;

struct page_descr_with_data {
    nd_uuid_t *id;
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

struct rrdeng_query_handle;
struct page_details_control;

void rrdeng_prep_wait(struct page_details_control *pdc);
void rrdeng_prep_query(struct page_details_control *pdc, bool worker);
void pg_cache_preload(struct rrdeng_query_handle *handle);
struct pgc_page *pg_cache_lookup_next(struct rrdengine_instance *ctx, struct page_details_control *pdc, time_t now_s, uint32_t last_update_every_s, size_t *entries);
void pgc_and_mrg_initialize(void);

void pgc_open_add_hot_page(Word_t section, Word_t metric_id, time_t start_time_s, time_t end_time_s, uint32_t update_every_s, struct rrdengine_datafile *datafile, uint64_t extent_offset, unsigned extent_size, uint32_t page_length);

#endif /* NETDATA_PAGECACHE_H */

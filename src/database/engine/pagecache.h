// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PAGECACHE_H
#define NETDATA_PAGECACHE_H

#include "rrdengine.h"

extern struct mrg *main_mrg;
extern struct pgc *main_cache;
extern struct pgc *open_cache;
extern struct pgc *extent_cache;
extern struct pgc *ram_cache;

/* Forward declarations */
struct rrdengine_instance;
struct pgc_page;

#define INVALID_TIME (0)

extern struct rrdeng_cache_efficiency_stats rrdeng_cache_efficiency_stats;

struct page_descr_with_data {
    UUIDMAP_ID uuid_id;
    Word_t metric_id;
    usec_t start_time_ut;
    usec_t end_time_ut;
    uint8_t type;
    uint32_t update_every_s;
    uint32_t page_length;
    uint32_t slots;             // stored slots of the page, set when its extent is written
    struct pgd *pgd;

    struct {
        struct page_descr_with_data *prev;
        struct page_descr_with_data *next;
    } link;
};

struct pg_alignment {
    uint32_t refcount;
    uint64_t seed;      // the chart's identity hash: picks the slot of absolute time its pages close at
};

struct rrdeng_query_handle;
struct page_details_control;

void rrdeng_prep_wait(struct page_details_control *pdc);
void rrdeng_prep_query(struct page_details_control *pdc, bool worker);
void pg_cache_preload(struct rrdeng_query_handle *handle);
struct pgc_page *pg_cache_lookup_next(struct rrdengine_instance *ctx, struct page_details_control *pdc, time_t now_s, uint32_t last_update_every_s, size_t *entries);
void pgc_and_mrg_initialize(bool in_memory_only);
void pgc_ram_cache_initialize(void);
void rrdeng_in_memory_enforce_retention(time_t now_s);
bool rrdeng_in_memory_take_lost_retention(void);

// the unit tests pause the retention decision of an in-memory page's free callback at these points (no hook otherwise)
struct pgc_entry;
typedef enum {
    RAM_CACHE_RETENTION_HOOK_ENTER,     // the page is out of the index; the callback has not waited for the others yet
    RAM_CACHE_RETENTION_HOOK_SCANNED,   // the callback scanned the pages that remain, and holds the decision lock
} RAM_CACHE_RETENTION_HOOK_POINT;
typedef void (*ram_cache_retention_hook_t)(const struct pgc_entry *entry, RAM_CACHE_RETENTION_HOOK_POINT point);
void rrdeng_in_memory_set_retention_test_hook(ram_cache_retention_hook_t hook);

void pgc_open_add_hot_page(
    Word_t section,
    Word_t metric_id,
    UUIDMAP_ID uuid_id,
    time_t start_time_s,
    time_t end_time_s,
    uint32_t update_every_s,
    struct rrdengine_datafile *datafile,
    uint64_t extent_offset,
    unsigned extent_size);

int pagecache_flush_drop_unittest(void);

#endif /* NETDATA_PAGECACHE_H */

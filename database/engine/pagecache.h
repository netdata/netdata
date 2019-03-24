// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PAGECACHE_H
#define NETDATA_PAGECACHE_H

#include "rrdengine.h"

/* Forward declerations */
struct rrdengine_instance;
struct extent_info;

#define INVALID_TIME (0)

/* Page flags */
#define RRD_PAGE_DIRTY          (1LU << 0)
#define RRD_PAGE_LOCKED         (1LU << 1) /* TODO: maybe remove? */
#define RRD_PAGE_READ_PENDING   (1LU << 2)
#define RRD_PAGE_WRITE_PENDING  (1LU << 3)
#define RRD_PAGE_POPULATED      (1LU << 4)

struct rrdeng_page_cache_descr {
    void *page;
    uint32_t page_length;
    usec_t start_time;
    usec_t end_time;
    uuid_t *id; /* never changes */
    struct extent_info *extent;
    unsigned long flags;
    void *private;
    struct rrdeng_page_cache_descr *prev;
    struct rrdeng_page_cache_descr *next;

    /* TODO: move waiter logic to concurrency table */
    unsigned refcnt;
    uv_mutex_t mutex; /* always take it after the page cache lock or after the commit lock */
    uv_cond_t cond;
    unsigned waiters;
    struct rrdeng_handle *handle; /* API user */
};

#define PAGE_CACHE_MAX_PAGES            (8192)
/* try to keep 5% of the page cache free */
#define PAGE_CACHE_LOW_WATERMARK        ((PAGE_CACHE_MAX_PAGES * 95LLU) / 100)

//#define PAGE_CACHE_MAX_COMMITED_PAGES   (2048) /* TODO: remove? */
#define PAGE_CACHE_MAX_PRELOAD_PAGES    (256)

/* maps time ranges to pages */
struct pg_cache_page_index {
    uuid_t id;
    Pvoid_t JudyL_array;
    uv_rwlock_t lock;
};

/* maps UUIDs to page indices */
struct pg_cache_metrics_index {
    uv_rwlock_t lock;
    Pvoid_t JudyHS_array;
};

/* gathers dirty pages to be written on disk */
struct pg_cache_commited_page_index {
    uv_rwlock_t lock;

    Pvoid_t JudyL_array;

    /*
     * Dirty page correlation ID is a hint. Dirty pages that are correlated should have
     * a small correlation ID difference. Dirty pages in memory should never have the
     * same ID at the same time for correctness.
     */
    Word_t latest_corr_id;

    unsigned nr_commited_pages;
};

/* gathers populated pages to be evicted */
struct pg_cache_replaceQ {
    uv_rwlock_t lock; /* LRU lock */

    struct rrdeng_page_cache_descr *head; /* LRU */
    struct rrdeng_page_cache_descr *tail; /* MRU */
};

struct page_cache { /* TODO: add statistics */
    uv_rwlock_t pg_cache_rwlock; /* page cache lock */

    struct pg_cache_metrics_index metrics_index;
    struct pg_cache_commited_page_index commited_page_index;
    struct pg_cache_replaceQ replaceQ;

    unsigned page_descriptors;
    unsigned populated_pages;
    /* Debug Statistics */
    unsigned consumers;
    unsigned producers;
};

extern void pg_cache_wake_up_waiters_unsafe(struct rrdeng_page_cache_descr *descr);
extern void pg_cache_wait_event_unsafe(struct rrdeng_page_cache_descr *descr);
extern unsigned long pg_cache_wait_event(struct rrdeng_page_cache_descr *descr);
extern void pg_cache_replaceQ_insert(struct rrdengine_instance *ctx,
                                     struct rrdeng_page_cache_descr *descr);
extern void pg_cache_replaceQ_delete(struct rrdengine_instance *ctx,
                                     struct rrdeng_page_cache_descr *descr);
extern void pg_cache_replaceQ_set_hot(struct rrdengine_instance *ctx,
                                      struct rrdeng_page_cache_descr *descr);
extern struct rrdeng_page_cache_descr *pg_cache_create_descr(void);
extern void pg_cache_put_unsafe(struct rrdeng_page_cache_descr *descr);
extern void pg_cache_put(struct rrdeng_page_cache_descr *descr);
extern void pg_cache_insert(struct rrdengine_instance *ctx, struct rrdeng_page_cache_descr *descr);
extern void pg_cache_punch_hole(struct rrdengine_instance *ctx, struct rrdeng_page_cache_descr *descr);
extern void pg_cache_preload(struct rrdengine_instance *ctx, uuid_t *id, usec_t start_time, usec_t end_time);
extern struct rrdeng_page_cache_descr *pg_cache_lookup(struct rrdengine_instance *ctx, uuid_t *id, usec_t point_in_time);
extern struct pg_cache_page_index *create_page_index(uuid_t *id);
extern void init_page_cache(struct rrdengine_instance *ctx);

#endif /* NETDATA_PAGECACHE_H */
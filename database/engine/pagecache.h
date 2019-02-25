// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PAGECACHE_H
#define NETDATA_PAGECACHE_H

#include "rrdengine.h"

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
    struct extent_info *extent; /* TODO: move this out of here */
    unsigned long flags;
    void *private;
    struct rrdeng_page_cache_descr *next;

    /* TODO: move waiter logic to concurrency table */
    unsigned refcnt;
    uv_mutex_t mutex; /* always take it after the page cache lock or after the commit lock */
    uv_cond_t cond;
    void **handle; /* API user */
};

struct extent_info {
    uint64_t offset;
    uint32_t size;
    uint8_t number_of_pages;
    struct rrdeng_page_cache_descr *pages[];
};


#define PAGE_CACHE_MAX_SIZE             (16384) /* TODO: Infinity? */
#define PAGE_CACHE_MAX_PAGES            (8192)
#define PAGE_CACHE_MAX_COMMITED_PAGES   (2048)
#define PAGE_CACHE_MAX_PRELOAD_PAGES    (256)

/*
 * Mock page cache
 */
struct page_cache { /* TODO: add statistics */
    /* page descriptor array TODO: tree */
    uv_rwlock_t pg_cache_rwlock; /* page cache lock */
    struct rrdeng_page_cache_descr *page_cache_array[PAGE_CACHE_MAX_SIZE];
    unsigned pages;
    unsigned populated_pages;

    uv_rwlock_t commited_pages_rwlock; /* commit lock */
    unsigned nr_commited_pages;
    struct rrdeng_page_cache_descr *commited_pages[PAGE_CACHE_MAX_COMMITED_PAGES];
};

extern struct page_cache pg_cache;

extern void pg_cache_put_unsafe(struct rrdeng_page_cache_descr *descr);
extern void pg_cache_put(struct rrdeng_page_cache_descr *descr);
extern void pg_cache_insert(struct rrdeng_page_cache_descr *descr);
void pg_cache_preload(uuid_t *id, usec_t start_time, usec_t end_time);
extern struct rrdeng_page_cache_descr *pg_cache_lookup(uuid_t *id, usec_t point_in_time);
extern void init_page_cache(void);

#endif /* NETDATA_PAGECACHE_H */
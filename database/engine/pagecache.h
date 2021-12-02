// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PAGECACHE_H
#define NETDATA_PAGECACHE_H

#include "rrdengine.h"

/* Forward declarations */
struct rrdengine_instance;
struct extent_info;
struct rrdeng_page_descr;

#define INVALID_TIME (0)

/* Page flags */
#define RRD_PAGE_DIRTY          (1LU << 0)
#define RRD_PAGE_LOCKED         (1LU << 1)
#define RRD_PAGE_READ_PENDING   (1LU << 2)
#define RRD_PAGE_WRITE_PENDING  (1LU << 3)
#define RRD_PAGE_POPULATED      (1LU << 4)

struct page_cache_descr {
    struct rrdeng_page_descr *descr; /* parent descriptor */
    void *page;
    unsigned long flags;
    struct page_cache_descr *prev; /* LRU */
    struct page_cache_descr *next; /* LRU */

    unsigned refcnt;
    uv_mutex_t mutex; /* always take it after the page cache lock or after the commit lock */
    uv_cond_t cond;
    unsigned waiters;
};

/* Page cache descriptor flags, state = 0 means no descriptor */
#define PG_CACHE_DESCR_ALLOCATED    (1LU << 0)
#define PG_CACHE_DESCR_DESTROY      (1LU << 1)
#define PG_CACHE_DESCR_LOCKED       (1LU << 2)
#define PG_CACHE_DESCR_SHIFT        (3)
#define PG_CACHE_DESCR_USERS_MASK   (((unsigned long)-1) << PG_CACHE_DESCR_SHIFT)
#define PG_CACHE_DESCR_FLAGS_MASK   (((unsigned long)-1) >> (BITS_PER_ULONG - PG_CACHE_DESCR_SHIFT))

/*
 * Page cache descriptor state bits (works for both 32-bit and 64-bit architectures):
 *
 * 63    ...     31   ...     3 |          2 |          1 |          0|
 * -----------------------------+------------+------------+-----------|
 * number of descriptor users   |    DESTROY |     LOCKED | ALLOCATED |
 */
struct rrdeng_page_descr {
    uuid_t *id; /* never changes */
    struct extent_info *extent;

    /* points to ephemeral page cache descriptor if the page resides in the cache */
    struct page_cache_descr *pg_cache_descr;

    /* Compare-And-Swap target for page cache descriptor allocation algorithm */
    volatile unsigned long pg_cache_descr_state;

    /* page information */
    usec_t start_time;
    usec_t end_time;
    uint32_t page_length;
};

#define PAGE_INFO_SCRATCH_SZ (8)
struct rrdeng_page_info {
    uint8_t scratch[PAGE_INFO_SCRATCH_SZ]; /* scratch area to be used by page-cache users */

    usec_t start_time;
    usec_t end_time;
    uint32_t page_length;
};

/* returns 1 for success, 0 for failure */
typedef int pg_cache_page_info_filter_t(struct rrdeng_page_descr *);

#define PAGE_CACHE_MAX_PRELOAD_PAGES    (256)

/* maps time ranges to pages */
struct pg_cache_page_index {
    uuid_t id;
    /*
     * care: JudyL_array indices are converted from useconds to seconds to fit in one word in 32-bit architectures
     * TODO: examine if we want to support better granularity than seconds
     */
    Pvoid_t JudyL_array;
    Word_t page_count;
    unsigned short writers;
    uv_rwlock_t lock;

    /*
     * Only one effective writer, data deletion workqueue.
     * It's also written during the DB loading phase.
     */
    usec_t oldest_time;

    /*
     * Only one effective writer, data collection thread.
     * It's also written by the data deletion workqueue when data collection is disabled for this metric.
     */
    usec_t latest_time;

    struct pg_cache_page_index *prev;
};

/* maps UUIDs to page indices */
struct pg_cache_metrics_index {
    uv_rwlock_t lock;
    Pvoid_t JudyHS_array;
    struct pg_cache_page_index *last_page_index;
};

/* gathers dirty pages to be written on disk */
struct pg_cache_committed_page_index {
    uv_rwlock_t lock;

    Pvoid_t JudyL_array;

    /*
     * Dirty page correlation ID is a hint. Dirty pages that are correlated should have
     * a small correlation ID difference. Dirty pages in memory should never have the
     * same ID at the same time for correctness.
     */
    Word_t latest_corr_id;

    unsigned nr_committed_pages;
};

/*
 * Gathers populated pages to be evicted.
 * Relies on page cache descriptors being there as it uses their memory.
 */
struct pg_cache_replaceQ {
    uv_rwlock_t lock; /* LRU lock */

    struct page_cache_descr *head; /* LRU */
    struct page_cache_descr *tail; /* MRU */
};

struct page_cache { /* TODO: add statistics */
    uv_rwlock_t pg_cache_rwlock; /* page cache lock */

    struct pg_cache_metrics_index metrics_index;
    struct pg_cache_committed_page_index committed_page_index;
    struct pg_cache_replaceQ replaceQ;

    unsigned page_descriptors;
    unsigned populated_pages;
};

extern void pg_cache_wake_up_waiters_unsafe(struct rrdeng_page_descr *descr);
extern void pg_cache_wake_up_waiters(struct rrdengine_instance *ctx, struct rrdeng_page_descr *descr);
extern void pg_cache_wait_event_unsafe(struct rrdeng_page_descr *descr);
extern unsigned long pg_cache_wait_event(struct rrdengine_instance *ctx, struct rrdeng_page_descr *descr);
extern void pg_cache_replaceQ_insert(struct rrdengine_instance *ctx,
                                     struct rrdeng_page_descr *descr);
extern void pg_cache_replaceQ_delete(struct rrdengine_instance *ctx,
                                     struct rrdeng_page_descr *descr);
extern void pg_cache_replaceQ_set_hot(struct rrdengine_instance *ctx,
                                      struct rrdeng_page_descr *descr);
extern struct rrdeng_page_descr *pg_cache_create_descr(void);
extern int pg_cache_try_get_unsafe(struct rrdeng_page_descr *descr, int exclusive_access);
extern void pg_cache_put_unsafe(struct rrdeng_page_descr *descr);
extern void pg_cache_put(struct rrdengine_instance *ctx, struct rrdeng_page_descr *descr);
extern void pg_cache_insert(struct rrdengine_instance *ctx, struct pg_cache_page_index *index,
                            struct rrdeng_page_descr *descr);
extern uint8_t pg_cache_punch_hole(struct rrdengine_instance *ctx, struct rrdeng_page_descr *descr,
                                   uint8_t remove_dirty, uint8_t is_exclusive_holder, uuid_t *metric_id);
extern usec_t pg_cache_oldest_time_in_range(struct rrdengine_instance *ctx, uuid_t *id,
                                            usec_t start_time, usec_t end_time);
extern void pg_cache_get_filtered_info_prev(struct rrdengine_instance *ctx, struct pg_cache_page_index *page_index,
                                            usec_t point_in_time, pg_cache_page_info_filter_t *filter,
                                            struct rrdeng_page_info *page_info);
extern struct rrdeng_page_descr *pg_cache_lookup_unpopulated_and_lock(struct rrdengine_instance *ctx, uuid_t *id,
                                                                      usec_t start_time);
extern unsigned
        pg_cache_preload(struct rrdengine_instance *ctx, uuid_t *id, usec_t start_time, usec_t end_time,
                         struct rrdeng_page_info **page_info_arrayp, struct pg_cache_page_index **ret_page_indexp);
extern struct rrdeng_page_descr *
        pg_cache_lookup(struct rrdengine_instance *ctx, struct pg_cache_page_index *index, uuid_t *id,
                        usec_t point_in_time);
extern struct rrdeng_page_descr *
        pg_cache_lookup_next(struct rrdengine_instance *ctx, struct pg_cache_page_index *index, uuid_t *id,
                     usec_t start_time, usec_t end_time);
extern struct pg_cache_page_index *create_page_index(uuid_t *id);
extern void init_page_cache(struct rrdengine_instance *ctx);
extern void free_page_cache(struct rrdengine_instance *ctx);
extern void pg_cache_add_new_metric_time(struct pg_cache_page_index *page_index, struct rrdeng_page_descr *descr);
extern void pg_cache_update_metric_times(struct pg_cache_page_index *page_index);
extern unsigned long pg_cache_hard_limit(struct rrdengine_instance *ctx);
extern unsigned long pg_cache_soft_limit(struct rrdengine_instance *ctx);
extern unsigned long pg_cache_committed_hard_limit(struct rrdengine_instance *ctx);

static inline void
    pg_cache_atomic_get_pg_info(struct rrdeng_page_descr *descr, usec_t *end_timep, uint32_t *page_lengthp)
{
    usec_t end_time, old_end_time;
    uint32_t page_length;

    if (NULL == descr->extent) {
        /* this page is currently being modified, get consistent info locklessly */
        do {
            end_time = descr->end_time;
            __sync_synchronize();
            old_end_time = end_time;
            page_length = descr->page_length;
            __sync_synchronize();
            end_time = descr->end_time;
            __sync_synchronize();
        } while ((end_time != old_end_time || (end_time & 1) != 0));

        *end_timep = end_time;
        *page_lengthp = page_length;
    } else {
        *end_timep = descr->end_time;
        *page_lengthp = descr->page_length;
    }
}

/* The caller must hold a reference to the page and must have already set the new data */
static inline void pg_cache_atomic_set_pg_info(struct rrdeng_page_descr *descr, usec_t end_time, uint32_t page_length)
{
    fatal_assert(!(end_time & 1));
    __sync_synchronize();
    descr->end_time |= 1; /* mark start of uncertainty period by adding 1 microsecond */
    __sync_synchronize();
    descr->page_length = page_length;
    __sync_synchronize();
    descr->end_time = end_time; /* mark end of uncertainty period */
}

#endif /* NETDATA_PAGECACHE_H */

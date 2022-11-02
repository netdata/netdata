// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PAGECACHE_H
#define NETDATA_PAGECACHE_H

#include "rrdengine.h"

/* Forward declarations */
struct rrdengine_instance;
struct extent_info;
struct rrdeng_page_descr;

#define INVALID_TIME (0)
#define MAX_PAGE_CACHE_FETCH_RETRIES (3)
#define PAGE_CACHE_FETCH_WAIT_TIMEOUT (3)

/* Page flags */
#define RRD_PAGE_DIRTY          (1LU << 0)
#define RRD_PAGE_LOCKED         (1LU << 1)
#define RRD_PAGE_READ_PENDING   (1LU << 2)
#define RRD_PAGE_WRITE_PENDING  (1LU << 3)
#define RRD_PAGE_POPULATED      (1LU << 4)
#define RRD_PAGE_INVALID        (1LU << 5)

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

//FIXME: To check if v2 add macros
// Is v2
// is v1
// is collected
struct rrdeng_page_descr {
    uuid_t *id; /* never changes */
    struct extent_info *extent;

    /* points to ephemeral page cache descriptor if the page resides in the cache */
    struct page_cache_descr *pg_cache_descr;

    /* Compare-And-Swap target for page cache descriptor allocation algorithm */
    volatile unsigned long pg_cache_descr_state;

    /* page information */
    usec_t start_time_ut;
    usec_t end_time_ut;
    uint32_t update_every_s:24;
    uint8_t type;
    uint32_t page_length;
    uv_file datafile_fd;
    void *extent_entry;
};

#define PAGE_INFO_SCRATCH_SZ (8)
struct rrdeng_page_info {
    uint8_t scratch[PAGE_INFO_SCRATCH_SZ]; /* scratch area to be used by page-cache users */

    usec_t start_time_ut;
    usec_t end_time_ut;
    uint32_t page_length;
};

/* returns 1 for success, 0 for failure */
typedef int pg_cache_page_info_filter_t(struct rrdeng_page_descr *);

#define PAGE_CACHE_MAX_PRELOAD_PAGES    (16)

struct pg_alignment {
    uint32_t page_length;
    uint32_t refcount;
};

/* maps time ranges to pages */
struct pg_cache_page_index {
    uuid_t id;
    /*
     * care: JudyL_array indices are converted from useconds to seconds to fit in one word in 32-bit architectures
     * TODO: examine if we want to support better granularity than seconds
     */
    Pvoid_t JudyL_array;
    Word_t page_count;
    unsigned short refcount;
    unsigned short writers;
    uv_rwlock_t lock;

    /*
     * Only one effective writer, data deletion workqueue.
     * It's also written during the DB loading phase.
     */
    usec_t oldest_time_ut;

    /*
     * Only one effective writer, data collection thread.
     * It's also written by the data deletion workqueue when data collection is disabled for this metric.
     */
    usec_t latest_time_ut;

    struct rrdengine_instance *ctx;
    struct pg_alignment *alignment;
    uint32_t latest_update_every_s;

    struct pg_cache_page_index *prev;
};

/* maps UUIDs to page indices */
struct pg_cache_metrics_index {
    uv_rwlock_t lock;
    Pvoid_t JudyHS_array;
    Pvoid_t migration_JudyHS_array;
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

struct pg_cache_dirty_descr_index {
    uv_rwlock_t lock;
    Pvoid_t JudyL_array;
    Pvoid_t JudyHS_array;
    unsigned count;
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
    struct pg_cache_dirty_descr_index dirty_descr_index;
    struct pg_cache_replaceQ replaceQ;

    unsigned page_descriptors;
    unsigned populated_pages;
};

void pg_cache_wake_up_waiters_unsafe(struct rrdeng_page_descr *descr);
void pg_cache_wake_up_waiters(struct rrdengine_instance *ctx, struct rrdeng_page_descr *descr);
void pg_cache_wait_event_unsafe(struct rrdeng_page_descr *descr);
unsigned long pg_cache_wait_event(struct rrdengine_instance *ctx, struct rrdeng_page_descr *descr);
void pg_cache_replaceQ_insert(struct rrdengine_instance *ctx,
                                     struct rrdeng_page_descr *descr);
void pg_cache_replaceQ_delete(struct rrdengine_instance *ctx,
                                     struct rrdeng_page_descr *descr);
void pg_cache_replaceQ_set_hot(struct rrdengine_instance *ctx,
                                      struct rrdeng_page_descr *descr);
struct rrdeng_page_descr *pg_cache_create_descr(void);
int pg_cache_try_get_unsafe(struct rrdeng_page_descr *descr, int exclusive_access);
void pg_cache_put_unsafe(struct rrdeng_page_descr *descr);
void pg_cache_put(struct rrdengine_instance *ctx, struct rrdeng_page_descr *descr);
struct rrdeng_page_descr *pg_cache_insert(
    struct rrdengine_instance *ctx,
    struct pg_cache_page_index *index,
    struct rrdeng_page_descr *descr,
    bool lock_page_index);
bool pg_cache_insert_unsafe(struct rrdengine_instance *ctx, struct pg_cache_page_index *index,
                     struct rrdeng_page_descr *descr);

uint8_t pg_cache_punch_hole(
    struct rrdengine_instance *ctx,
    struct rrdeng_page_descr *descr,
    uint8_t remove_dirty,
    uint8_t is_exclusive_holder,
    uuid_t(*metric_id),
    bool update_page_duration,
    bool spin);
usec_t pg_cache_oldest_time_in_range(struct rrdengine_instance *ctx, uuid_t *id,
                                            usec_t start_time_ut, usec_t end_time_ut);
void pg_cache_get_filtered_info_prev(struct rrdengine_instance *ctx, struct pg_cache_page_index *page_index,
                                            usec_t point_in_time_ut, pg_cache_page_info_filter_t *filter,
                                            struct rrdeng_page_info *page_info);
struct rrdeng_page_descr *pg_cache_lookup_unpopulated_and_lock(struct rrdengine_instance *ctx, uuid_t *id,
                                                                      usec_t start_time_ut);
unsigned
        pg_cache_preload(struct rrdengine_instance *ctx, uuid_t *id, usec_t start_time_ut, usec_t end_time_ut,
                         struct rrdeng_page_info **page_info_arrayp, struct pg_cache_page_index **ret_page_indexp);
struct rrdeng_page_descr *
        pg_cache_lookup_next(struct rrdengine_instance *ctx, struct pg_cache_page_index *index, uuid_t *id,
                     usec_t start_time_ut, usec_t end_time_ut);
struct pg_cache_page_index *create_page_index(uuid_t *id, struct rrdengine_instance *ctx);
void init_page_cache(struct rrdengine_instance *ctx);
void free_page_cache(struct rrdengine_instance *ctx);
void pg_cache_add_new_metric_time(struct pg_cache_page_index *page_index, struct rrdeng_page_descr *descr);
void pg_cache_update_metric_times(struct pg_cache_page_index *page_index);
unsigned long pg_cache_hard_limit(struct rrdengine_instance *ctx);
unsigned long pg_cache_soft_limit(struct rrdengine_instance *ctx);
unsigned long pg_cache_committed_hard_limit(struct rrdengine_instance *ctx);

void rrdeng_page_descr_aral_go_singlethreaded(void);
void rrdeng_page_descr_aral_go_multithreaded(void);
void rrdeng_page_descr_use_malloc(void);
void rrdeng_page_descr_use_mmap(void);
bool rrdeng_page_descr_is_mmap(void);
struct rrdeng_page_descr *rrdeng_page_descr_mallocz(void);
void rrdeng_page_descr_freez(struct rrdeng_page_descr *descr);

static inline void
    pg_cache_atomic_get_pg_info(struct rrdeng_page_descr *descr, usec_t *end_time_ut_p, uint32_t *page_lengthp)
{
    usec_t end_time_ut, old_end_time_ut;
    uint32_t page_length;

    if (NULL == descr->extent) {
        /* this page is currently being modified, get consistent info locklessly */
        do {
            end_time_ut = descr->end_time_ut;
            __sync_synchronize();
            old_end_time_ut = end_time_ut;
            page_length = descr->page_length;
            __sync_synchronize();
            end_time_ut = descr->end_time_ut;
            __sync_synchronize();
        } while ((end_time_ut != old_end_time_ut || (end_time_ut & 1) != 0));

        *end_time_ut_p = end_time_ut;
        *page_lengthp = page_length;
    } else {
        *end_time_ut_p = descr->end_time_ut;
        *page_lengthp = descr->page_length;
    }
}

/* The caller must hold a reference to the page and must have already set the new data */
static inline void pg_cache_atomic_set_pg_info(struct rrdeng_page_descr *descr, usec_t end_time_ut, uint32_t page_length)
{
    fatal_assert(!(end_time_ut & 1));
    __sync_synchronize();
    descr->end_time_ut |= 1; /* mark start of uncertainty period by adding 1 microsecond */
    __sync_synchronize();
    descr->page_length = page_length;
    __sync_synchronize();
    descr->end_time_ut = end_time_ut; /* mark end of uncertainty period */
}

#endif /* NETDATA_PAGECACHE_H */

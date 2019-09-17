// SPDX-License-Identifier: GPL-3.0-or-later
#define NETDATA_RRD_INTERNALS

#include "rrdengine.h"

/* Forward declerations */
static int pg_cache_try_evict_one_page_unsafe(struct rrdengine_instance *ctx);

/* always inserts into tail */
static inline void pg_cache_replaceQ_insert_unsafe(struct rrdengine_instance *ctx,
                                                   struct rrdeng_page_descr *descr)
{
    struct page_cache *pg_cache = &ctx->pg_cache;
    struct page_cache_descr *pg_cache_descr = descr->pg_cache_descr;

    if (likely(NULL != pg_cache->replaceQ.tail)) {
        pg_cache_descr->prev = pg_cache->replaceQ.tail;
        pg_cache->replaceQ.tail->next = pg_cache_descr;
    }
    if (unlikely(NULL == pg_cache->replaceQ.head)) {
        pg_cache->replaceQ.head = pg_cache_descr;
    }
    pg_cache->replaceQ.tail = pg_cache_descr;
}

static inline void pg_cache_replaceQ_delete_unsafe(struct rrdengine_instance *ctx,
                                                   struct rrdeng_page_descr *descr)
{
    struct page_cache *pg_cache = &ctx->pg_cache;
    struct page_cache_descr *pg_cache_descr = descr->pg_cache_descr, *prev, *next;

    prev = pg_cache_descr->prev;
    next = pg_cache_descr->next;

    if (likely(NULL != prev)) {
        prev->next = next;
    }
    if (likely(NULL != next)) {
        next->prev = prev;
    }
    if (unlikely(pg_cache_descr == pg_cache->replaceQ.head)) {
        pg_cache->replaceQ.head = next;
    }
    if (unlikely(pg_cache_descr == pg_cache->replaceQ.tail)) {
        pg_cache->replaceQ.tail = prev;
    }
    pg_cache_descr->prev = pg_cache_descr->next = NULL;
}

void pg_cache_replaceQ_insert(struct rrdengine_instance *ctx,
                              struct rrdeng_page_descr *descr)
{
    struct page_cache *pg_cache = &ctx->pg_cache;

    uv_rwlock_wrlock(&pg_cache->replaceQ.lock);
    pg_cache_replaceQ_insert_unsafe(ctx, descr);
    uv_rwlock_wrunlock(&pg_cache->replaceQ.lock);
}

void pg_cache_replaceQ_delete(struct rrdengine_instance *ctx,
                              struct rrdeng_page_descr *descr)
{
    struct page_cache *pg_cache = &ctx->pg_cache;

    uv_rwlock_wrlock(&pg_cache->replaceQ.lock);
    pg_cache_replaceQ_delete_unsafe(ctx, descr);
    uv_rwlock_wrunlock(&pg_cache->replaceQ.lock);
}
void pg_cache_replaceQ_set_hot(struct rrdengine_instance *ctx,
                               struct rrdeng_page_descr *descr)
{
    struct page_cache *pg_cache = &ctx->pg_cache;

    uv_rwlock_wrlock(&pg_cache->replaceQ.lock);
    pg_cache_replaceQ_delete_unsafe(ctx, descr);
    pg_cache_replaceQ_insert_unsafe(ctx, descr);
    uv_rwlock_wrunlock(&pg_cache->replaceQ.lock);
}

struct rrdeng_page_descr *pg_cache_create_descr(void)
{
    struct rrdeng_page_descr *descr;

    descr = mallocz(sizeof(*descr));
    descr->page_length = 0;
    descr->start_time = INVALID_TIME;
    descr->end_time = INVALID_TIME;
    descr->id = NULL;
    descr->extent = NULL;
    descr->pg_cache_descr_state = 0;
    descr->pg_cache_descr = NULL;

    return descr;
}

/* The caller must hold page descriptor lock. */
void pg_cache_wake_up_waiters_unsafe(struct rrdeng_page_descr *descr)
{
    struct page_cache_descr *pg_cache_descr = descr->pg_cache_descr;
    if (pg_cache_descr->waiters)
        uv_cond_broadcast(&pg_cache_descr->cond);
}

/*
 * The caller must hold page descriptor lock.
 * The lock will be released and re-acquired. The descriptor is not guaranteed
 * to exist after this function returns.
 */
void pg_cache_wait_event_unsafe(struct rrdeng_page_descr *descr)
{
    struct page_cache_descr *pg_cache_descr = descr->pg_cache_descr;

    ++pg_cache_descr->waiters;
    uv_cond_wait(&pg_cache_descr->cond, &pg_cache_descr->mutex);
    --pg_cache_descr->waiters;
}

/*
 * Returns page flags.
 * The lock will be released and re-acquired. The descriptor is not guaranteed
 * to exist after this function returns.
 */
unsigned long pg_cache_wait_event(struct rrdengine_instance *ctx, struct rrdeng_page_descr *descr)
{
    struct page_cache_descr *pg_cache_descr = descr->pg_cache_descr;
    unsigned long flags;

    rrdeng_page_descr_mutex_lock(ctx, descr);
    pg_cache_wait_event_unsafe(descr);
    flags = pg_cache_descr->flags;
    rrdeng_page_descr_mutex_unlock(ctx, descr);

    return flags;
}

/*
 * The caller must hold page descriptor lock.
 * Gets a reference to the page descriptor.
 * Returns 1 on success and 0 on failure.
 */
int pg_cache_try_get_unsafe(struct rrdeng_page_descr *descr, int exclusive_access)
{
    struct page_cache_descr *pg_cache_descr = descr->pg_cache_descr;

    if ((pg_cache_descr->flags & (RRD_PAGE_LOCKED | RRD_PAGE_READ_PENDING)) ||
        (exclusive_access && pg_cache_descr->refcnt)) {
        return 0;
    }
    if (exclusive_access)
        pg_cache_descr->flags |= RRD_PAGE_LOCKED;
    ++pg_cache_descr->refcnt;

    return 1;
}

/*
 * The caller must hold page descriptor lock.
 * Same return values as pg_cache_try_get_unsafe() without doing anything.
 */
int pg_cache_can_get_unsafe(struct rrdeng_page_descr *descr, int exclusive_access)
{
    struct page_cache_descr *pg_cache_descr = descr->pg_cache_descr;

    if ((pg_cache_descr->flags & (RRD_PAGE_LOCKED | RRD_PAGE_READ_PENDING)) ||
        (exclusive_access && pg_cache_descr->refcnt)) {
        return 0;
    }

    return 1;
}

/*
 * The caller must hold the page descriptor lock.
 * This function may block doing cleanup.
 */
void pg_cache_put_unsafe(struct rrdeng_page_descr *descr)
{
    struct page_cache_descr *pg_cache_descr = descr->pg_cache_descr;

    pg_cache_descr->flags &= ~RRD_PAGE_LOCKED;
    if (0 == --pg_cache_descr->refcnt) {
        pg_cache_wake_up_waiters_unsafe(descr);
    }
}

/*
 * This function may block doing cleanup.
 */
void pg_cache_put(struct rrdengine_instance *ctx, struct rrdeng_page_descr *descr)
{
    rrdeng_page_descr_mutex_lock(ctx, descr);
    pg_cache_put_unsafe(descr);
    rrdeng_page_descr_mutex_unlock(ctx, descr);
}

/* The caller must hold the page cache lock */
static void pg_cache_release_pages_unsafe(struct rrdengine_instance *ctx, unsigned number)
{
    struct page_cache *pg_cache = &ctx->pg_cache;

    pg_cache->populated_pages -= number;
}

static void pg_cache_release_pages(struct rrdengine_instance *ctx, unsigned number)
{
    struct page_cache *pg_cache = &ctx->pg_cache;

    uv_rwlock_wrlock(&pg_cache->pg_cache_rwlock);
    pg_cache_release_pages_unsafe(ctx, number);
    uv_rwlock_wrunlock(&pg_cache->pg_cache_rwlock);
}
/*
 * This function will block until it reserves #number populated pages.
 * It will trigger evictions or dirty page flushing if the ctx->max_cache_pages limit is hit.
 */
static void pg_cache_reserve_pages(struct rrdengine_instance *ctx, unsigned number)
{
    struct page_cache *pg_cache = &ctx->pg_cache;
    unsigned failures = 0;
    const unsigned FAILURES_CEILING = 10; /* truncates exponential backoff to (2^FAILURES_CEILING x slot) */
    unsigned long exp_backoff_slot_usec = USEC_PER_MS * 10;

    assert(number < ctx->max_cache_pages);

    uv_rwlock_wrlock(&pg_cache->pg_cache_rwlock);
    if (pg_cache->populated_pages + number >= ctx->max_cache_pages + 1)
        debug(D_RRDENGINE, "==Page cache full. Reserving %u pages.==",
                number);
    while (pg_cache->populated_pages + number >= ctx->max_cache_pages + 1) {

        if (!pg_cache_try_evict_one_page_unsafe(ctx)) {
            /* failed to evict */
            struct completion compl;
            struct rrdeng_cmd cmd;

            ++failures;
            uv_rwlock_wrunlock(&pg_cache->pg_cache_rwlock);

            init_completion(&compl);
            cmd.opcode = RRDENG_FLUSH_PAGES;
            cmd.completion = &compl;
            rrdeng_enq_cmd(&ctx->worker_config, &cmd);
            /* wait for some pages to be flushed */
            debug(D_RRDENGINE, "%s: waiting for pages to be written to disk before evicting.", __func__);
            wait_for_completion(&compl);
            destroy_completion(&compl);

            if (unlikely(failures > 1)) {
                unsigned long slots;
                /* exponential backoff */
                slots = random() % (2LU << MIN(failures, FAILURES_CEILING));
                (void)sleep_usec(slots * exp_backoff_slot_usec);
            }
            uv_rwlock_wrlock(&pg_cache->pg_cache_rwlock);
        }
    }
    pg_cache->populated_pages += number;
    uv_rwlock_wrunlock(&pg_cache->pg_cache_rwlock);
}

/*
 * This function will attempt to reserve #number populated pages.
 * It may trigger evictions if the ctx->cache_pages_low_watermark limit is hit.
 * Returns 0 on failure and 1 on success.
 */
static int pg_cache_try_reserve_pages(struct rrdengine_instance *ctx, unsigned number)
{
    struct page_cache *pg_cache = &ctx->pg_cache;
    unsigned count = 0;
    int ret = 0;

    assert(number < ctx->max_cache_pages);

    uv_rwlock_wrlock(&pg_cache->pg_cache_rwlock);
    if (pg_cache->populated_pages + number >= ctx->cache_pages_low_watermark + 1) {
        debug(D_RRDENGINE,
              "==Page cache full. Trying to reserve %u pages.==",
              number);
        do {
            if (!pg_cache_try_evict_one_page_unsafe(ctx))
                break;
            ++count;
        } while (pg_cache->populated_pages + number >= ctx->cache_pages_low_watermark + 1);
        debug(D_RRDENGINE, "Evicted %u pages.", count);
    }

    if (pg_cache->populated_pages + number < ctx->max_cache_pages + 1) {
        pg_cache->populated_pages += number;
        ret = 1; /* success */
    }
    uv_rwlock_wrunlock(&pg_cache->pg_cache_rwlock);

    return ret;
}

/* The caller must hold the page cache and the page descriptor locks in that order */
static void pg_cache_evict_unsafe(struct rrdengine_instance *ctx, struct rrdeng_page_descr *descr)
{
    struct page_cache_descr *pg_cache_descr = descr->pg_cache_descr;

    freez(pg_cache_descr->page);
    pg_cache_descr->page = NULL;
    pg_cache_descr->flags &= ~RRD_PAGE_POPULATED;
    pg_cache_release_pages_unsafe(ctx, 1);
    ++ctx->stats.pg_cache_evictions;
}

/*
 * The caller must hold the page cache lock.
 * Lock order: page cache -> replaceQ -> page descriptor
 * This function iterates all pages and tries to evict one.
 * If it fails it sets in_flight_descr to the oldest descriptor that has write-back in progress,
 * or it sets it to NULL if no write-back is in progress.
 *
 * Returns 1 on success and 0 on failure.
 */
static int pg_cache_try_evict_one_page_unsafe(struct rrdengine_instance *ctx)
{
    struct page_cache *pg_cache = &ctx->pg_cache;
    unsigned long old_flags;
    struct rrdeng_page_descr *descr;
    struct page_cache_descr *pg_cache_descr = NULL;

    uv_rwlock_wrlock(&pg_cache->replaceQ.lock);
    for (pg_cache_descr = pg_cache->replaceQ.head ; NULL != pg_cache_descr ; pg_cache_descr = pg_cache_descr->next) {
        descr = pg_cache_descr->descr;

        rrdeng_page_descr_mutex_lock(ctx, descr);
        old_flags = pg_cache_descr->flags;
        if ((old_flags & RRD_PAGE_POPULATED) && !(old_flags & RRD_PAGE_DIRTY) && pg_cache_try_get_unsafe(descr, 1)) {
            /* must evict */
            pg_cache_evict_unsafe(ctx, descr);
            pg_cache_put_unsafe(descr);
            pg_cache_replaceQ_delete_unsafe(ctx, descr);

            rrdeng_page_descr_mutex_unlock(ctx, descr);
            uv_rwlock_wrunlock(&pg_cache->replaceQ.lock);

            rrdeng_try_deallocate_pg_cache_descr(ctx, descr);

            return 1;
        }
        rrdeng_page_descr_mutex_unlock(ctx, descr);
    }
    uv_rwlock_wrunlock(&pg_cache->replaceQ.lock);

    /* failed to evict */
    return 0;
}

void pg_cache_punch_hole(struct rrdengine_instance *ctx, struct rrdeng_page_descr *descr, uint8_t remove_dirty)
{
    struct page_cache *pg_cache = &ctx->pg_cache;
    struct page_cache_descr *pg_cache_descr = NULL;
    Pvoid_t *PValue;
    struct pg_cache_page_index *page_index;
    int ret;

    uv_rwlock_rdlock(&pg_cache->metrics_index.lock);
    PValue = JudyHSGet(pg_cache->metrics_index.JudyHS_array, descr->id, sizeof(uuid_t));
    assert(NULL != PValue);
    page_index = *PValue;
    uv_rwlock_rdunlock(&pg_cache->metrics_index.lock);

    uv_rwlock_wrlock(&page_index->lock);
    ret = JudyLDel(&page_index->JudyL_array, (Word_t)(descr->start_time / USEC_PER_SEC), PJE0);
    uv_rwlock_wrunlock(&page_index->lock);
    if (unlikely(0 == ret)) {
        error("Page under deletion was not in index.");
        if (unlikely(debug_flags & D_RRDENGINE)) {
            print_page_descr(descr);
        }
        goto destroy;
    }
    assert(1 == ret);

    uv_rwlock_wrlock(&pg_cache->pg_cache_rwlock);
    ++ctx->stats.pg_cache_deletions;
    --pg_cache->page_descriptors;
    uv_rwlock_wrunlock(&pg_cache->pg_cache_rwlock);

    rrdeng_page_descr_mutex_lock(ctx, descr);
    pg_cache_descr = descr->pg_cache_descr;
    while (!pg_cache_try_get_unsafe(descr, 1)) {
        debug(D_RRDENGINE, "%s: Waiting for locked page:", __func__);
        if (unlikely(debug_flags & D_RRDENGINE))
            print_page_cache_descr(descr);
        pg_cache_wait_event_unsafe(descr);
    }
    if (!remove_dirty) {
        /* even a locked page could be dirty */
        while (unlikely(pg_cache_descr->flags & RRD_PAGE_DIRTY)) {
            debug(D_RRDENGINE, "%s: Found dirty page, waiting for it to be flushed:", __func__);
            if (unlikely(debug_flags & D_RRDENGINE))
                print_page_cache_descr(descr);
            pg_cache_wait_event_unsafe(descr);
        }
    }
    rrdeng_page_descr_mutex_unlock(ctx, descr);

    if (pg_cache_descr->flags & RRD_PAGE_POPULATED) {
        /* only after locking can it be safely deleted from LRU */
        pg_cache_replaceQ_delete(ctx, descr);

        uv_rwlock_wrlock(&pg_cache->pg_cache_rwlock);
        pg_cache_evict_unsafe(ctx, descr);
        uv_rwlock_wrunlock(&pg_cache->pg_cache_rwlock);
    }
    pg_cache_put(ctx, descr);

    rrdeng_destroy_pg_cache_descr(ctx, pg_cache_descr);
destroy:
    freez(descr);
    pg_cache_update_metric_times(page_index);
}

static inline int is_page_in_time_range(struct rrdeng_page_descr *descr, usec_t start_time, usec_t end_time)
{
    usec_t pg_start, pg_end;

    pg_start = descr->start_time;
    pg_end = descr->end_time;

    return (pg_start < start_time && pg_end >= start_time) ||
           (pg_start >= start_time && pg_start <= end_time);
}

static inline int is_point_in_time_in_page(struct rrdeng_page_descr *descr, usec_t point_in_time)
{
    return (point_in_time >= descr->start_time && point_in_time <= descr->end_time);
}

/* The caller must hold the page index lock */
static inline struct rrdeng_page_descr *
        find_first_page_in_time_range(struct pg_cache_page_index *page_index, usec_t start_time, usec_t end_time)
{
    struct rrdeng_page_descr *descr = NULL;
    Pvoid_t *PValue;
    Word_t Index;

    Index = (Word_t)(start_time / USEC_PER_SEC);
    PValue = JudyLLast(page_index->JudyL_array, &Index, PJE0);
    if (likely(NULL != PValue)) {
        descr = *PValue;
        if (is_page_in_time_range(descr, start_time, end_time)) {
            return descr;
        }
    }

    Index = (Word_t)(start_time / USEC_PER_SEC);
    PValue = JudyLFirst(page_index->JudyL_array, &Index, PJE0);
    if (likely(NULL != PValue)) {
        descr = *PValue;
        if (is_page_in_time_range(descr, start_time, end_time)) {
            return descr;
        }
    }

    return NULL;
}

/* Update metric oldest and latest timestamps efficiently when adding new values */
void pg_cache_add_new_metric_time(struct pg_cache_page_index *page_index, struct rrdeng_page_descr *descr)
{
    usec_t oldest_time = page_index->oldest_time;
    usec_t latest_time = page_index->latest_time;

    if (unlikely(oldest_time == INVALID_TIME || descr->start_time < oldest_time)) {
        page_index->oldest_time = descr->start_time;
    }
    if (likely(descr->end_time > latest_time || latest_time == INVALID_TIME)) {
        page_index->latest_time = descr->end_time;
    }
}

/* Update metric oldest and latest timestamps when removing old values */
void pg_cache_update_metric_times(struct pg_cache_page_index *page_index)
{
    Pvoid_t *firstPValue, *lastPValue;
    Word_t firstIndex, lastIndex;
    struct rrdeng_page_descr *descr;
    usec_t oldest_time = INVALID_TIME;
    usec_t latest_time = INVALID_TIME;

    uv_rwlock_rdlock(&page_index->lock);
    /* Find first page in range */
    firstIndex = (Word_t)0;
    firstPValue = JudyLFirst(page_index->JudyL_array, &firstIndex, PJE0);
    if (likely(NULL != firstPValue)) {
        descr = *firstPValue;
        oldest_time = descr->start_time;
    }
    lastIndex = (Word_t)-1;
    lastPValue = JudyLLast(page_index->JudyL_array, &lastIndex, PJE0);
    if (likely(NULL != lastPValue)) {
        descr = *lastPValue;
        latest_time = descr->end_time;
    }
    uv_rwlock_rdunlock(&page_index->lock);

    if (unlikely(NULL == firstPValue)) {
        assert(NULL == lastPValue);
        page_index->oldest_time = page_index->latest_time = INVALID_TIME;
        return;
    }
    page_index->oldest_time = oldest_time;
    page_index->latest_time = latest_time;
}

/* If index is NULL lookup by UUID (descr->id) */
void pg_cache_insert(struct rrdengine_instance *ctx, struct pg_cache_page_index *index,
                     struct rrdeng_page_descr *descr)
{
    struct page_cache *pg_cache = &ctx->pg_cache;
    Pvoid_t *PValue;
    struct pg_cache_page_index *page_index;
    unsigned long pg_cache_descr_state = descr->pg_cache_descr_state;

    if (0 != pg_cache_descr_state) {
        /* there is page cache descriptor pre-allocated state */
        struct page_cache_descr *pg_cache_descr = descr->pg_cache_descr;

        assert(pg_cache_descr_state & PG_CACHE_DESCR_ALLOCATED);
        if (pg_cache_descr->flags & RRD_PAGE_POPULATED) {
            pg_cache_reserve_pages(ctx, 1);
            if (!(pg_cache_descr->flags & RRD_PAGE_DIRTY))
                pg_cache_replaceQ_insert(ctx, descr);
        }
    }

    if (unlikely(NULL == index)) {
        uv_rwlock_rdlock(&pg_cache->metrics_index.lock);
        PValue = JudyHSGet(pg_cache->metrics_index.JudyHS_array, descr->id, sizeof(uuid_t));
        assert(NULL != PValue);
        page_index = *PValue;
        uv_rwlock_rdunlock(&pg_cache->metrics_index.lock);
    } else {
        page_index = index;
    }

    uv_rwlock_wrlock(&page_index->lock);
    PValue = JudyLIns(&page_index->JudyL_array, (Word_t)(descr->start_time / USEC_PER_SEC), PJE0);
    *PValue = descr;
    pg_cache_add_new_metric_time(page_index, descr);
    uv_rwlock_wrunlock(&page_index->lock);

    uv_rwlock_wrlock(&pg_cache->pg_cache_rwlock);
    ++ctx->stats.pg_cache_insertions;
    ++pg_cache->page_descriptors;
    uv_rwlock_wrunlock(&pg_cache->pg_cache_rwlock);
}

usec_t pg_cache_oldest_time_in_range(struct rrdengine_instance *ctx, uuid_t *id, usec_t start_time, usec_t end_time)
{
    struct page_cache *pg_cache = &ctx->pg_cache;
    struct rrdeng_page_descr *descr = NULL;
    Pvoid_t *PValue;
    struct pg_cache_page_index *page_index;

    uv_rwlock_rdlock(&pg_cache->metrics_index.lock);
    PValue = JudyHSGet(pg_cache->metrics_index.JudyHS_array, id, sizeof(uuid_t));
    if (likely(NULL != PValue)) {
        page_index = *PValue;
    }
    uv_rwlock_rdunlock(&pg_cache->metrics_index.lock);
    if (NULL == PValue) {
        return INVALID_TIME;
    }

    uv_rwlock_rdlock(&page_index->lock);
    descr = find_first_page_in_time_range(page_index, start_time, end_time);
    if (NULL == descr) {
        uv_rwlock_rdunlock(&page_index->lock);
        return INVALID_TIME;
    }
    uv_rwlock_rdunlock(&page_index->lock);
    return descr->start_time;
}

/**
 * Return page information for the first page before point_in_time that satisfies the filter.
 * @param ctx DB context
 * @param page_index page index of a metric
 * @param point_in_time the pages that are searched must be older than this timestamp
 * @param filter decides if the page satisfies the caller's criteria
 * @param page_info the result of the search is set in this pointer
 */
void pg_cache_get_filtered_info_prev(struct rrdengine_instance *ctx, struct pg_cache_page_index *page_index,
                                     usec_t point_in_time, pg_cache_page_info_filter_t *filter,
                                     struct rrdeng_page_info *page_info)
{
    struct page_cache *pg_cache = &ctx->pg_cache;
    struct rrdeng_page_descr *descr = NULL;
    Pvoid_t *PValue;
    Word_t Index;

    (void)pg_cache;
    assert(NULL != page_index);

    Index = (Word_t)(point_in_time / USEC_PER_SEC);
    uv_rwlock_rdlock(&page_index->lock);
    do {
        PValue = JudyLPrev(page_index->JudyL_array, &Index, PJE0);
        descr = unlikely(NULL == PValue) ? NULL : *PValue;
    } while (descr != NULL && !filter(descr));
    if (unlikely(NULL == descr)) {
        page_info->page_length = 0;
        page_info->start_time = INVALID_TIME;
        page_info->end_time = INVALID_TIME;
    } else {
        page_info->page_length = descr->page_length;
        page_info->start_time = descr->start_time;
        page_info->end_time = descr->end_time;
    }
    uv_rwlock_rdunlock(&page_index->lock);
}
/**
 * Searches for pages in a time range and triggers disk I/O if necessary and possible.
 * Does not get a reference.
 * @param ctx DB context
 * @param id UUID
 * @param start_time inclusive starting time in usec
 * @param end_time inclusive ending time in usec
 * @param page_info_arrayp It allocates (*page_arrayp) and populates it with information of pages that overlap
 *        with the time range [start_time,end_time]. The caller must free (*page_info_arrayp) with freez().
 *        If page_info_arrayp is set to NULL nothing was allocated.
 * @param ret_page_indexp Sets the page index pointer (*ret_page_indexp) for the given UUID.
 * @return the number of pages that overlap with the time range [start_time,end_time].
 */
unsigned pg_cache_preload(struct rrdengine_instance *ctx, uuid_t *id, usec_t start_time, usec_t end_time,
                          struct rrdeng_page_info **page_info_arrayp, struct pg_cache_page_index **ret_page_indexp)
{
    struct page_cache *pg_cache = &ctx->pg_cache;
    struct rrdeng_page_descr *descr = NULL, *preload_array[PAGE_CACHE_MAX_PRELOAD_PAGES];
    struct page_cache_descr *pg_cache_descr = NULL;
    unsigned i, j, k, preload_count, count, page_info_array_max_size;
    unsigned long flags;
    Pvoid_t *PValue;
    struct pg_cache_page_index *page_index;
    Word_t Index;
    uint8_t failed_to_reserve;

    assert(NULL != ret_page_indexp);

    uv_rwlock_rdlock(&pg_cache->metrics_index.lock);
    PValue = JudyHSGet(pg_cache->metrics_index.JudyHS_array, id, sizeof(uuid_t));
    if (likely(NULL != PValue)) {
        *ret_page_indexp = page_index = *PValue;
    }
    uv_rwlock_rdunlock(&pg_cache->metrics_index.lock);
    if (NULL == PValue) {
        debug(D_RRDENGINE, "%s: No page was found to attempt preload.", __func__);
        *ret_page_indexp = NULL;
        return 0;
    }

    uv_rwlock_rdlock(&page_index->lock);
    descr = find_first_page_in_time_range(page_index, start_time, end_time);
    if (NULL == descr) {
        uv_rwlock_rdunlock(&page_index->lock);
        debug(D_RRDENGINE, "%s: No page was found to attempt preload.", __func__);
        *ret_page_indexp = NULL;
        return 0;
    } else {
        Index = (Word_t)(descr->start_time / USEC_PER_SEC);
    }
    if (page_info_arrayp) {
        page_info_array_max_size = PAGE_CACHE_MAX_PRELOAD_PAGES * sizeof(struct rrdeng_page_info);
        *page_info_arrayp = mallocz(page_info_array_max_size);
    }

    for (count = 0, preload_count = 0 ;
         descr != NULL && is_page_in_time_range(descr, start_time, end_time) ;
         PValue = JudyLNext(page_index->JudyL_array, &Index, PJE0),
         descr = unlikely(NULL == PValue) ? NULL : *PValue) {
        /* Iterate all pages in range */

        if (unlikely(0 == descr->page_length))
            continue;
        if (page_info_arrayp) {
            if (unlikely(count >= page_info_array_max_size / sizeof(struct rrdeng_page_info))) {
                page_info_array_max_size += PAGE_CACHE_MAX_PRELOAD_PAGES * sizeof(struct rrdeng_page_info);
                *page_info_arrayp = reallocz(*page_info_arrayp, page_info_array_max_size);
            }
            (*page_info_arrayp)[count].start_time = descr->start_time;
            (*page_info_arrayp)[count].end_time = descr->end_time;
            (*page_info_arrayp)[count].page_length = descr->page_length;
        }
        ++count;

        rrdeng_page_descr_mutex_lock(ctx, descr);
        pg_cache_descr = descr->pg_cache_descr;
        flags = pg_cache_descr->flags;
        if (pg_cache_can_get_unsafe(descr, 0)) {
            if (flags & RRD_PAGE_POPULATED) {
                /* success */
                rrdeng_page_descr_mutex_unlock(ctx, descr);
                debug(D_RRDENGINE, "%s: Page was found in memory.", __func__);
                continue;
            }
        }
        if (!(flags & RRD_PAGE_POPULATED) && pg_cache_try_get_unsafe(descr, 1)) {
            preload_array[preload_count++] = descr;
            if (PAGE_CACHE_MAX_PRELOAD_PAGES == preload_count) {
                rrdeng_page_descr_mutex_unlock(ctx, descr);
                break;
            }
        }
        rrdeng_page_descr_mutex_unlock(ctx, descr);

    }
    uv_rwlock_rdunlock(&page_index->lock);

    failed_to_reserve = 0;
    for (i = 0 ; i < preload_count && !failed_to_reserve ; ++i) {
        struct rrdeng_cmd cmd;
        struct rrdeng_page_descr *next;

        descr = preload_array[i];
        if (NULL == descr) {
            continue;
        }
        if (!pg_cache_try_reserve_pages(ctx, 1)) {
            failed_to_reserve = 1;
            break;
        }
        cmd.opcode = RRDENG_READ_EXTENT;
        cmd.read_extent.page_cache_descr[0] = descr;
        /* don't use this page again */
        preload_array[i] = NULL;
        for (j = 0, k = 1 ; j < preload_count ; ++j) {
            next = preload_array[j];
            if (NULL == next) {
                continue;
            }
            if (descr->extent == next->extent) {
                /* same extent, consolidate */
                if (!pg_cache_try_reserve_pages(ctx, 1)) {
                    failed_to_reserve = 1;
                    break;
                }
                cmd.read_extent.page_cache_descr[k++] = next;
                /* don't use this page again */
                preload_array[j] = NULL;
            }
        }
        cmd.read_extent.page_count = k;
        rrdeng_enq_cmd(&ctx->worker_config, &cmd);
    }
    if (failed_to_reserve) {
        debug(D_RRDENGINE, "%s: Failed to reserve enough memory, canceling I/O.", __func__);
        for (i = 0 ; i < preload_count ; ++i) {
            descr = preload_array[i];
            if (NULL == descr) {
                continue;
            }
            pg_cache_put(ctx, descr);
        }
    }
    if (!preload_count) {
        /* no such page */
        debug(D_RRDENGINE, "%s: No page was eligible to attempt preload.", __func__);
    }
    if (unlikely(0 == count && page_info_arrayp)) {
        freez(*page_info_arrayp);
        *page_info_arrayp = NULL;
    }
    return count;
}

/*
 * Searches for a page and gets a reference.
 * When point_in_time is INVALID_TIME get any page.
 * If index is NULL lookup by UUID (id).
 */
struct rrdeng_page_descr *
        pg_cache_lookup(struct rrdengine_instance *ctx, struct pg_cache_page_index *index, uuid_t *id,
                        usec_t point_in_time)
{
    struct page_cache *pg_cache = &ctx->pg_cache;
    struct rrdeng_page_descr *descr = NULL;
    struct page_cache_descr *pg_cache_descr = NULL;
    unsigned long flags;
    Pvoid_t *PValue;
    struct pg_cache_page_index *page_index;
    Word_t Index;
    uint8_t page_not_in_cache;

    if (unlikely(NULL == index)) {
        uv_rwlock_rdlock(&pg_cache->metrics_index.lock);
        PValue = JudyHSGet(pg_cache->metrics_index.JudyHS_array, id, sizeof(uuid_t));
        if (likely(NULL != PValue)) {
            page_index = *PValue;
        }
        uv_rwlock_rdunlock(&pg_cache->metrics_index.lock);
        if (NULL == PValue) {
            return NULL;
        }
    } else {
        page_index = index;
    }
    pg_cache_reserve_pages(ctx, 1);

    page_not_in_cache = 0;
    uv_rwlock_rdlock(&page_index->lock);
    while (1) {
        Index = (Word_t)(point_in_time / USEC_PER_SEC);
        PValue = JudyLLast(page_index->JudyL_array, &Index, PJE0);
        if (likely(NULL != PValue)) {
            descr = *PValue;
        }
        if (NULL == PValue ||
            0 == descr->page_length ||
            (INVALID_TIME != point_in_time &&
             !is_point_in_time_in_page(descr, point_in_time))) {
            /* non-empty page not found */
            uv_rwlock_rdunlock(&page_index->lock);

            pg_cache_release_pages(ctx, 1);
            return NULL;
        }
        rrdeng_page_descr_mutex_lock(ctx, descr);
        pg_cache_descr = descr->pg_cache_descr;
        flags = pg_cache_descr->flags;
        if ((flags & RRD_PAGE_POPULATED) && pg_cache_try_get_unsafe(descr, 0)) {
            /* success */
            rrdeng_page_descr_mutex_unlock(ctx, descr);
            debug(D_RRDENGINE, "%s: Page was found in memory.", __func__);
            break;
        }
        if (!(flags & RRD_PAGE_POPULATED) && pg_cache_try_get_unsafe(descr, 1)) {
            struct rrdeng_cmd cmd;

            uv_rwlock_rdunlock(&page_index->lock);

            cmd.opcode = RRDENG_READ_PAGE;
            cmd.read_page.page_cache_descr = descr;
            rrdeng_enq_cmd(&ctx->worker_config, &cmd);

            debug(D_RRDENGINE, "%s: Waiting for page to be asynchronously read from disk:", __func__);
            if(unlikely(debug_flags & D_RRDENGINE))
                print_page_cache_descr(descr);
            while (!(pg_cache_descr->flags & RRD_PAGE_POPULATED)) {
                pg_cache_wait_event_unsafe(descr);
            }
            /* success */
            /* Downgrade exclusive reference to allow other readers */
            pg_cache_descr->flags &= ~RRD_PAGE_LOCKED;
            pg_cache_wake_up_waiters_unsafe(descr);
            rrdeng_page_descr_mutex_unlock(ctx, descr);
            rrd_stat_atomic_add(&ctx->stats.pg_cache_misses, 1);
            return descr;
        }
        uv_rwlock_rdunlock(&page_index->lock);
        debug(D_RRDENGINE, "%s: Waiting for page to be unlocked:", __func__);
        if(unlikely(debug_flags & D_RRDENGINE))
            print_page_cache_descr(descr);
        if (!(flags & RRD_PAGE_POPULATED))
            page_not_in_cache = 1;
        pg_cache_wait_event_unsafe(descr);
        rrdeng_page_descr_mutex_unlock(ctx, descr);

        /* reset scan to find again */
        uv_rwlock_rdlock(&page_index->lock);
    }
    uv_rwlock_rdunlock(&page_index->lock);

    if (!(flags & RRD_PAGE_DIRTY))
        pg_cache_replaceQ_set_hot(ctx, descr);
    pg_cache_release_pages(ctx, 1);
    if (page_not_in_cache)
        rrd_stat_atomic_add(&ctx->stats.pg_cache_misses, 1);
    else
        rrd_stat_atomic_add(&ctx->stats.pg_cache_hits, 1);
    return descr;
}

/*
 * Searches for the first page between start_time and end_time and gets a reference.
 * start_time and end_time are inclusive.
 * If index is NULL lookup by UUID (id).
 */
struct rrdeng_page_descr *
pg_cache_lookup_next(struct rrdengine_instance *ctx, struct pg_cache_page_index *index, uuid_t *id,
                     usec_t start_time, usec_t end_time)
{
    struct page_cache *pg_cache = &ctx->pg_cache;
    struct rrdeng_page_descr *descr = NULL;
    struct page_cache_descr *pg_cache_descr = NULL;
    unsigned long flags;
    Pvoid_t *PValue;
    struct pg_cache_page_index *page_index;
    uint8_t page_not_in_cache;

    if (unlikely(NULL == index)) {
        uv_rwlock_rdlock(&pg_cache->metrics_index.lock);
        PValue = JudyHSGet(pg_cache->metrics_index.JudyHS_array, id, sizeof(uuid_t));
        if (likely(NULL != PValue)) {
            page_index = *PValue;
        }
        uv_rwlock_rdunlock(&pg_cache->metrics_index.lock);
        if (NULL == PValue) {
            return NULL;
        }
    } else {
        page_index = index;
    }
    pg_cache_reserve_pages(ctx, 1);

    page_not_in_cache = 0;
    uv_rwlock_rdlock(&page_index->lock);
    while (1) {
        descr = find_first_page_in_time_range(page_index, start_time, end_time);
        if (NULL == descr || 0 == descr->page_length) {
            /* non-empty page not found */
            uv_rwlock_rdunlock(&page_index->lock);

            pg_cache_release_pages(ctx, 1);
            return NULL;
        }
        rrdeng_page_descr_mutex_lock(ctx, descr);
        pg_cache_descr = descr->pg_cache_descr;
        flags = pg_cache_descr->flags;
        if ((flags & RRD_PAGE_POPULATED) && pg_cache_try_get_unsafe(descr, 0)) {
            /* success */
            rrdeng_page_descr_mutex_unlock(ctx, descr);
            debug(D_RRDENGINE, "%s: Page was found in memory.", __func__);
            break;
        }
        if (!(flags & RRD_PAGE_POPULATED) && pg_cache_try_get_unsafe(descr, 1)) {
            struct rrdeng_cmd cmd;

            uv_rwlock_rdunlock(&page_index->lock);

            cmd.opcode = RRDENG_READ_PAGE;
            cmd.read_page.page_cache_descr = descr;
            rrdeng_enq_cmd(&ctx->worker_config, &cmd);

            debug(D_RRDENGINE, "%s: Waiting for page to be asynchronously read from disk:", __func__);
            if(unlikely(debug_flags & D_RRDENGINE))
                print_page_cache_descr(descr);
            while (!(pg_cache_descr->flags & RRD_PAGE_POPULATED)) {
                pg_cache_wait_event_unsafe(descr);
            }
            /* success */
            /* Downgrade exclusive reference to allow other readers */
            pg_cache_descr->flags &= ~RRD_PAGE_LOCKED;
            pg_cache_wake_up_waiters_unsafe(descr);
            rrdeng_page_descr_mutex_unlock(ctx, descr);
            rrd_stat_atomic_add(&ctx->stats.pg_cache_misses, 1);
            return descr;
        }
        uv_rwlock_rdunlock(&page_index->lock);
        debug(D_RRDENGINE, "%s: Waiting for page to be unlocked:", __func__);
        if(unlikely(debug_flags & D_RRDENGINE))
            print_page_cache_descr(descr);
        if (!(flags & RRD_PAGE_POPULATED))
            page_not_in_cache = 1;
        pg_cache_wait_event_unsafe(descr);
        rrdeng_page_descr_mutex_unlock(ctx, descr);

        /* reset scan to find again */
        uv_rwlock_rdlock(&page_index->lock);
    }
    uv_rwlock_rdunlock(&page_index->lock);

    if (!(flags & RRD_PAGE_DIRTY))
        pg_cache_replaceQ_set_hot(ctx, descr);
    pg_cache_release_pages(ctx, 1);
    if (page_not_in_cache)
        rrd_stat_atomic_add(&ctx->stats.pg_cache_misses, 1);
    else
        rrd_stat_atomic_add(&ctx->stats.pg_cache_hits, 1);
    return descr;
}

struct pg_cache_page_index *create_page_index(uuid_t *id)
{
    struct pg_cache_page_index *page_index;

    page_index = mallocz(sizeof(*page_index));
    page_index->JudyL_array = (Pvoid_t) NULL;
    uuid_copy(page_index->id, *id);
    assert(0 == uv_rwlock_init(&page_index->lock));
    page_index->oldest_time = INVALID_TIME;
    page_index->latest_time = INVALID_TIME;
    page_index->prev = NULL;

    return page_index;
}

static void init_metrics_index(struct rrdengine_instance *ctx)
{
    struct page_cache *pg_cache = &ctx->pg_cache;

    pg_cache->metrics_index.JudyHS_array = (Pvoid_t) NULL;
    pg_cache->metrics_index.last_page_index = NULL;
    assert(0 == uv_rwlock_init(&pg_cache->metrics_index.lock));
}

static void init_replaceQ(struct rrdengine_instance *ctx)
{
    struct page_cache *pg_cache = &ctx->pg_cache;

    pg_cache->replaceQ.head = NULL;
    pg_cache->replaceQ.tail = NULL;
    assert(0 == uv_rwlock_init(&pg_cache->replaceQ.lock));
}

static void init_commited_page_index(struct rrdengine_instance *ctx)
{
    struct page_cache *pg_cache = &ctx->pg_cache;

    pg_cache->commited_page_index.JudyL_array = (Pvoid_t) NULL;
    assert(0 == uv_rwlock_init(&pg_cache->commited_page_index.lock));
    pg_cache->commited_page_index.latest_corr_id = 0;
    pg_cache->commited_page_index.nr_commited_pages = 0;
}

void init_page_cache(struct rrdengine_instance *ctx)
{
    struct page_cache *pg_cache = &ctx->pg_cache;

    pg_cache->page_descriptors = 0;
    pg_cache->populated_pages = 0;
    assert(0 == uv_rwlock_init(&pg_cache->pg_cache_rwlock));

    init_metrics_index(ctx);
    init_replaceQ(ctx);
    init_commited_page_index(ctx);
}

void free_page_cache(struct rrdengine_instance *ctx)
{
    struct page_cache *pg_cache = &ctx->pg_cache;
    Word_t ret_Judy, bytes_freed = 0;
    Pvoid_t *PValue;
    struct pg_cache_page_index *page_index, *prev_page_index;
    Word_t Index;
    struct rrdeng_page_descr *descr;
    struct page_cache_descr *pg_cache_descr;

    /* Free commited page index */
    ret_Judy = JudyLFreeArray(&pg_cache->commited_page_index.JudyL_array, PJE0);
    assert(NULL == pg_cache->commited_page_index.JudyL_array);
    bytes_freed += ret_Judy;

    for (page_index = pg_cache->metrics_index.last_page_index ;
         page_index != NULL ;
         page_index = prev_page_index) {
        prev_page_index = page_index->prev;

        /* Find first page in range */
        Index = (Word_t) 0;
        PValue = JudyLFirst(page_index->JudyL_array, &Index, PJE0);
        descr = unlikely(NULL == PValue) ? NULL : *PValue;

        while (descr != NULL) {
            /* Iterate all page descriptors of this metric */

            if (descr->pg_cache_descr_state & PG_CACHE_DESCR_ALLOCATED) {
                /* Check rrdenglocking.c */
                pg_cache_descr = descr->pg_cache_descr;
                if (pg_cache_descr->flags & RRD_PAGE_POPULATED) {
                    freez(pg_cache_descr->page);
                    bytes_freed += RRDENG_BLOCK_SIZE;
                }
                rrdeng_destroy_pg_cache_descr(ctx, pg_cache_descr);
                bytes_freed += sizeof(*pg_cache_descr);
            }
            freez(descr);
            bytes_freed += sizeof(*descr);

            PValue = JudyLNext(page_index->JudyL_array, &Index, PJE0);
            descr = unlikely(NULL == PValue) ? NULL : *PValue;
        }

        /* Free page index */
        ret_Judy = JudyLFreeArray(&page_index->JudyL_array, PJE0);
        assert(NULL == page_index->JudyL_array);
        bytes_freed += ret_Judy;
        freez(page_index);
        bytes_freed += sizeof(*page_index);
    }
    /* Free metrics index */
    ret_Judy = JudyHSFreeArray(&pg_cache->metrics_index.JudyHS_array, PJE0);
    assert(NULL == pg_cache->metrics_index.JudyHS_array);
    bytes_freed += ret_Judy;

    info("Freed %lu bytes of memory from page cache.", bytes_freed);
}
// SPDX-License-Identifier: GPL-3.0-or-later
#define NETDATA_RRD_INTERNALS

#include "rrdengine.h"

ARAL page_descr_aral = {
    .requested_element_size = sizeof(struct rrdeng_page_descr),
    .initial_elements = 20000,
    .filename = "page_descriptors",
    .cache_dir = &netdata_configured_cache_dir,
    .use_mmap = false,
    .internal.initialized = false
};

void rrdeng_page_descr_aral_go_singlethreaded(void) {
    page_descr_aral.internal.lockless = true;
}
void rrdeng_page_descr_aral_go_multithreaded(void) {
    page_descr_aral.internal.lockless = false;
}

struct rrdeng_page_descr *rrdeng_page_descr_mallocz(void) {
    struct rrdeng_page_descr *descr;
    descr = arrayalloc_mallocz(&page_descr_aral);
    return descr;
}

void rrdeng_page_descr_freez(struct rrdeng_page_descr *descr) {
    arrayalloc_freez(&page_descr_aral, descr);
}

void rrdeng_page_descr_use_malloc(void) {
    if(page_descr_aral.internal.initialized)
        error("DBENGINE: cannot change ARAL allocation policy after it has been initialized.");
    else
        page_descr_aral.use_mmap = false;
}

void rrdeng_page_descr_use_mmap(void) {
    if(page_descr_aral.internal.initialized)
        error("DBENGINE: cannot change ARAL allocation policy after it has been initialized.");
    else
        page_descr_aral.use_mmap = true;
}

bool rrdeng_page_descr_is_mmap(void) {
    return page_descr_aral.use_mmap;
}

/* Forward declarations */
static struct rrdeng_page_descr *pg_cache_try_evict_one_page_unsafe(struct rrdengine_instance *ctx);

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

    descr = rrdeng_page_descr_mallocz();
    descr->page_length = 0;
    descr->start_time_ut = INVALID_TIME;
    descr->end_time_ut = INVALID_TIME;
    descr->id = NULL;
    descr->extent = NULL;
    descr->pg_cache_descr_state = 0;
    descr->pg_cache_descr = NULL;
    descr->update_every_s = 0;

    return descr;
}

/* The caller must hold page descriptor lock. */
void pg_cache_wake_up_waiters_unsafe(struct rrdeng_page_descr *descr)
{
    struct page_cache_descr *pg_cache_descr = descr->pg_cache_descr;
    if (pg_cache_descr->waiters)
        uv_cond_broadcast(&pg_cache_descr->cond);
}

void pg_cache_wake_up_waiters(struct rrdengine_instance *ctx, struct rrdeng_page_descr *descr)
{
    rrdeng_page_descr_mutex_lock(ctx, descr);
    pg_cache_wake_up_waiters_unsafe(descr);
    rrdeng_page_descr_mutex_unlock(ctx, descr);
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
 * The caller must hold page descriptor lock.
 * The lock will be released and re-acquired. The descriptor is not guaranteed
 * to exist after this function returns.
 * Returns UV_ETIMEDOUT if timeout_sec seconds pass.
 */
int pg_cache_timedwait_event_unsafe(struct rrdeng_page_descr *descr, uint64_t timeout_sec)
{
    int ret;
    struct page_cache_descr *pg_cache_descr = descr->pg_cache_descr;

    ++pg_cache_descr->waiters;
    ret = uv_cond_timedwait(&pg_cache_descr->cond, &pg_cache_descr->mutex, timeout_sec * NSEC_PER_SEC);
    --pg_cache_descr->waiters;

    return ret;
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
 * The caller must hold page descriptor lock.
 * Gets a reference to the page descriptor.
 * Returns 1 on success and 0 on failure.
 */
int pg_cache_try_get_unsafe(struct rrdeng_page_descr *descr, int exclusive_access)
{
    struct page_cache_descr *pg_cache_descr = descr->pg_cache_descr;

    if (!pg_cache_can_get_unsafe(descr, exclusive_access))
        return 0;

    if (exclusive_access)
        pg_cache_descr->flags |= RRD_PAGE_LOCKED;
    ++pg_cache_descr->refcnt;

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
 * This function returns the maximum number of pages allowed in the page cache.
 */
unsigned long pg_cache_hard_limit(struct rrdengine_instance *ctx)
{
    return ctx->max_cache_pages + (unsigned long)ctx->metric_API_max_producers;
}

/*
 * This function returns the low watermark number of pages in the page cache. The page cache should strive to keep the
 * number of pages below that number.
 */
unsigned long pg_cache_soft_limit(struct rrdengine_instance *ctx)
{
    return ctx->cache_pages_low_watermark + (unsigned long)ctx->metric_API_max_producers;
}

/*
 * This function returns the maximum number of dirty pages that are committed to be written to disk allowed in the page
 * cache.
 */
unsigned long pg_cache_committed_hard_limit(struct rrdengine_instance *ctx)
{
    /* We remove the active pages of the producers from the calculation and only allow the extra pinned pages */
    return ctx->cache_pages_low_watermark + (unsigned long)ctx->metric_API_max_producers;
}

/*
 * This function will block until it reserves #number populated pages.
 * It will trigger evictions or dirty page flushing if the pg_cache_hard_limit() limit is hit.
 */
static void pg_cache_reserve_pages(struct rrdengine_instance *ctx, unsigned number)
{
    struct page_cache *pg_cache = &ctx->pg_cache;
    unsigned failures = 0;
    const unsigned FAILURES_CEILING = 10; /* truncates exponential backoff to (2^FAILURES_CEILING x slot) */
    unsigned long exp_backoff_slot_usec = USEC_PER_MS * 10;

    assert(number < ctx->max_cache_pages);

    uv_rwlock_wrlock(&pg_cache->pg_cache_rwlock);
    if (pg_cache->populated_pages + number >= pg_cache_hard_limit(ctx) + 1)
        debug(D_RRDENGINE, "==Page cache full. Reserving %u pages.==",
                number);

    struct rrdeng_page_descr *list_to_destroy[number];
    struct rrdeng_page_descr *descr;
    unsigned deleted=0;
    while (pg_cache->populated_pages + number >= pg_cache_hard_limit(ctx) + 1) {

        // TODO: If it succeeds, return the descr and then we can punch hole to it
        if (!(descr = pg_cache_try_evict_one_page_unsafe(ctx))) {
            /* failed to evict */
            struct completion compl;
            struct rrdeng_cmd cmd;

            ++failures;
            uv_rwlock_wrunlock(&pg_cache->pg_cache_rwlock);

            completion_init(&compl);
            cmd.opcode = RRDENG_FLUSH_PAGES;
            cmd.completion = &compl;
            rrdeng_enq_cmd(&ctx->worker_config, &cmd);
            /* wait for some pages to be flushed */
            debug(D_RRDENGINE, "%s: waiting for pages to be written to disk before evicting.", __func__);
            completion_wait_for(&compl);
            completion_destroy(&compl);

            if (unlikely(failures > 1)) {
                unsigned long slots, usecs_to_sleep;
                /* exponential backoff */
                slots = random() % (2LU << MIN(failures, FAILURES_CEILING));
                usecs_to_sleep = slots * exp_backoff_slot_usec;

                if (usecs_to_sleep >= USEC_PER_SEC)
                    error("Page cache is full. Sleeping for %llu second(s).", usecs_to_sleep / USEC_PER_SEC);

                (void)sleep_usec(usecs_to_sleep);
            }
            uv_rwlock_wrlock(&pg_cache->pg_cache_rwlock);
        }
        else {
            if (deleted < number)
                list_to_destroy[deleted++] = descr;
        }
    }
    pg_cache->populated_pages += number;
    uv_rwlock_wrunlock(&pg_cache->pg_cache_rwlock);
    for (unsigned i=0; i < deleted; i++) {
        //FIXME: Use macro to decide if V2
        if (descr->extent == NULL) {
            char uuid_str[UUID_STR_LEN];
//            time_t index_time = list_to_destroy[i]->start_time_ut / USEC_PER_SEC;
            uuid_unparse_lower(*list_to_destroy[i]->id, uuid_str);
            pg_cache_punch_hole(ctx, list_to_destroy[i], 0, 0, NULL, false);
//            internal_error(true,"JOURNALV2: Removed %s @ %ld", uuid_str, index_time);
        }
    }
}

/*
 * This function will attempt to reserve #number populated pages.
 * It may trigger evictions if the pg_cache_soft_limit() limit is hit.
 * Returns 0 on failure and 1 on success.
 */
static int pg_cache_try_reserve_pages(struct rrdengine_instance *ctx, unsigned number)
{
    struct page_cache *pg_cache = &ctx->pg_cache;
    unsigned count = 0;
    int ret = 0;

    assert(number < ctx->max_cache_pages);

    // TODO: CHECK FOR EVICTION
//    struct rrdeng_page_descr *list_to_destroy[number];
    struct rrdeng_page_descr *descr;
//    unsigned deleted=0;

    uv_rwlock_wrlock(&pg_cache->pg_cache_rwlock);
    if (pg_cache->populated_pages + number >= pg_cache_soft_limit(ctx) + 1) {
        debug(D_RRDENGINE,
              "==Page cache full. Trying to reserve %u pages.==",
              number);
        do {
            if (!(descr = pg_cache_try_evict_one_page_unsafe(ctx)))
                break;
//            if (deleted < number)
//                list_to_destroy[deleted++] = descr;
            ++count;
        } while (pg_cache->populated_pages + number >= pg_cache_soft_limit(ctx) + 1);
        debug(D_RRDENGINE, "Evicted %u pages.", count);
    }

    if (pg_cache->populated_pages + number < pg_cache_hard_limit(ctx) + 1) {
        pg_cache->populated_pages += number;
        ret = 1; /* success */
    }
    uv_rwlock_wrunlock(&pg_cache->pg_cache_rwlock);

    //for (unsigned i=0; i < deleted; i++)
    //    pg_cache_punch_hole(ctx, list_to_destroy[i], 0, 1, NULL);

    return ret;
}

/* The caller must hold the page cache and the page descriptor locks in that order */
static void pg_cache_evict_unsafe(struct rrdengine_instance *ctx, struct rrdeng_page_descr *descr)
{
    struct page_cache_descr *pg_cache_descr = descr->pg_cache_descr;

    dbengine_page_free(pg_cache_descr->page);
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
 * Returns the evicted descr on success and NULL on failure.
 */
static struct rrdeng_page_descr *pg_cache_try_evict_one_page_unsafe(struct rrdengine_instance *ctx)
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

            return descr;
        }
        rrdeng_page_descr_mutex_unlock(ctx, descr);
    }
    uv_rwlock_wrunlock(&pg_cache->replaceQ.lock);

    /* failed to evict */
    return NULL;
}

/**
 * Deletes a page from the database.
 * Callers of this function need to make sure they're not deleting the same descriptor concurrently.
 * @param ctx is the database instance.
 * @param descr is the page descriptor.
 * @param remove_dirty must be non-zero if the page to be deleted is dirty.
 * @param is_exclusive_holder must be non-zero if the caller holds an exclusive page reference.
 * @param metric_id is set to the metric the page belongs to, if it's safe to delete the metric and metric_id is not
 *        NULL. Otherwise, metric_id is not set.
 * @return 1 if it's safe to delete the metric, 0 otherwise.
 */
uint8_t pg_cache_punch_hole(
    struct rrdengine_instance *ctx,
    struct rrdeng_page_descr *descr,
    uint8_t remove_dirty,
    uint8_t is_exclusive_holder,
    uuid_t(*metric_id),
    bool update_page_duration)
{
    struct page_cache *pg_cache = &ctx->pg_cache;
    struct page_cache_descr *pg_cache_descr = NULL;
    Pvoid_t *PValue;
    struct pg_cache_page_index *page_index = NULL;
    int ret;
    uint8_t can_delete_metric = 0;

    uv_rwlock_rdlock(&pg_cache->metrics_index.lock);
    PValue = JudyHSGet(pg_cache->metrics_index.JudyHS_array, descr->id, sizeof(uuid_t));
    fatal_assert(NULL != PValue);
    page_index = *PValue;
    uv_rwlock_rdunlock(&pg_cache->metrics_index.lock);

    uv_rwlock_wrlock(&page_index->lock);
    ret = JudyLDel(&page_index->JudyL_array, (Word_t)(descr->start_time_ut / USEC_PER_SEC), PJE0);
    if (unlikely(0 == ret)) {
        uv_rwlock_wrunlock(&page_index->lock);
        if (unlikely(debug_flags & D_RRDENGINE)) {
            print_page_descr(descr);
        }
        goto destroy;
    }
    --page_index->page_count;
    if (!page_index->writers && !page_index->page_count) {
        can_delete_metric = 1;
        if (metric_id) {
            memcpy(metric_id, page_index->id, sizeof(uuid_t));
        }
    }
    uv_rwlock_wrunlock(&page_index->lock);
    fatal_assert(1 == ret);

    uv_rwlock_wrlock(&pg_cache->pg_cache_rwlock);
    ++ctx->stats.pg_cache_deletions;
    --pg_cache->page_descriptors;
    uv_rwlock_wrunlock(&pg_cache->pg_cache_rwlock);

    rrdeng_page_descr_mutex_lock(ctx, descr);
    pg_cache_descr = descr->pg_cache_descr;
    if (!is_exclusive_holder) {
        /* If we don't hold an exclusive page reference get one */
        while (!pg_cache_try_get_unsafe(descr, 1)) {
            debug(D_RRDENGINE, "%s: Waiting for locked page:", __func__);
            if (unlikely(debug_flags & D_RRDENGINE))
                print_page_cache_descr(descr, "", true);
            pg_cache_wait_event_unsafe(descr);
        }
    }
    if (remove_dirty) {
        pg_cache_descr->flags &= ~(RRD_PAGE_DIRTY | RRD_PAGE_INVALID);
    } else {
        /* even a locked page could be dirty */
        while (unlikely(pg_cache_descr->flags & RRD_PAGE_DIRTY)) {
            debug(D_RRDENGINE, "%s: Found dirty page, waiting for it to be flushed:", __func__);
            if (unlikely(debug_flags & D_RRDENGINE))
                print_page_cache_descr(descr, "", true);
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
    rrdeng_try_deallocate_pg_cache_descr(ctx, descr);
    while (descr->pg_cache_descr_state & PG_CACHE_DESCR_ALLOCATED) {
        rrdeng_try_deallocate_pg_cache_descr(ctx, descr); /* spin */
        (void)sleep_usec(1000); /* 1 msec */
    }
destroy:
    rrdeng_page_descr_freez(descr);
    if (update_page_duration)
        pg_cache_update_metric_times(page_index);

    return can_delete_metric;
}

static inline int is_page_in_time_range(struct rrdeng_page_descr *descr, usec_t start_time, usec_t end_time)
{
    usec_t pg_start, pg_end;

    pg_start = descr->start_time_ut;
    pg_end = descr->end_time_ut;

    return (pg_start < start_time && pg_end >= start_time) ||
           (pg_start >= start_time && pg_start <= end_time);
}

static struct journal_page_list *find_matching_page_index(struct journal_page_header *page_list_header, uint32_t delta_start_time_s)
{
    struct journal_page_list *page_list = (struct journal_page_list *) ((void *) page_list_header + sizeof(*page_list_header));
    struct journal_page_list *page_entry;
    uint32_t middle_delta_start_s;
    uint32_t middle_delta_end_s;

    int left = 0;
    int right = (int)(page_list_header->entries);

    while (left < right) {
        int middle = (left + right) >> 1;

        internal_fatal(middle >= (int)(page_list_header->entries),
                       "DBENGINE: binary search invalid middle slot.");

        page_entry = &page_list[middle];
        middle_delta_start_s = page_entry->delta_start_s;
        middle_delta_end_s = page_entry->delta_end_s;

        if (delta_start_time_s < middle_delta_start_s)
            right = middle;
        else if(delta_start_time_s > middle_delta_end_s)
            left = middle + 1;
        else
            return page_entry;
    }

    int selected = right - 1;
    if(selected < 0)
        selected = 0;

    return &page_list[selected];
}


// Note we have read lock on page index
// We release and escalate to write lock
// Return to read lock when done
static struct rrdeng_page_descr *add_pages_from_timerange(struct journal_page_header *page_list_header, uint32_t delta_start_time_s, usec_t journal_start_time_ut,
                              struct pg_cache_page_index *page_index, struct journal_extent_list *extent_list, struct rrdengine_datafile *datafile)
{
    time_t journal_start_time_s = (time_t)(journal_start_time_ut / USEC_PER_SEC);

    struct rrdengine_instance *ctx = page_index->ctx;
    struct journal_page_list *page_entry = NULL;

    Pvoid_t *PValue;
//    info("JOURVALV2: Validating UUID PAGE LIST is %d", page_header_is_valid(page_list_header));

    page_entry = find_matching_page_index(page_list_header, delta_start_time_s);

    time_t page_entry_start_time_s = journal_start_time_s + page_entry->delta_start_s;
    Word_t Index = page_entry_start_time_s;

    struct rrdeng_page_descr *descr;

    PValue = JudyLFirst(page_index->JudyL_array, &Index, PJE0);
    descr = (PValue && *PValue) ? *PValue : NULL;

    if (unlikely(descr && (time_t)(descr->start_time_ut / USEC_PER_SEC) == page_entry_start_time_s)) {
        internal_error(true, "Page with start time %lu already exists; skipping", page_entry_start_time_s);
        return descr;
    }

    // FIXME: Safety of rd -> write lock
    uv_rwlock_rdunlock(&page_index->lock);
    uv_rwlock_wrlock(&page_index->lock);

    descr = pg_cache_create_descr();
    descr->page_length = page_entry->page_length;
    descr->start_time_ut = (journal_start_time_s + page_entry->delta_start_s) * USEC_PER_SEC;
    descr->end_time_ut = (journal_start_time_s + page_entry->delta_end_s) * USEC_PER_SEC;
    descr->id = &page_index->id;
    descr->extent = NULL;
    // FIXME: Check valid offset within file
    descr->extent_entry = &extent_list[page_entry->extent_index];
    descr->type = page_entry->type;
    descr->update_every_s = page_entry->update_every_s;
    descr->datafile_fd = datafile->file;

    struct rrdeng_page_descr *added_descr = pg_cache_insert(ctx, page_index, descr, false);
    if (unlikely(added_descr != descr))
        rrdeng_page_descr_freez(descr);

    uv_rwlock_wrunlock(&page_index->lock);
    uv_rwlock_rdlock(&page_index->lock);

    return added_descr;
};

static int journal_metric_uuid_compare(const void *key, const void *metric)
{
    return uuid_compare(*(uuid_t *) key, ((struct journal_metric_list *) metric)->uuid);
}

//
// Steps
// 1. Find which journal has the start time / end time range
// 2. Find the UUID in that journal
// 3. Find the array of times for that UUID (convert from the journal header to the offset needed)
// Note: We have page_index lock
static struct rrdeng_page_descr *populate_metric_time_from_journal(struct pg_cache_page_index *page_index, usec_t start_time_ut, usec_t end_time_ut)
{
    struct rrdengine_instance *ctx = page_index->ctx;
    struct rrdengine_datafile *datafile = ctx->datafiles.first;
    struct journal_metric_list *uuid_list = NULL;
    struct journal_page_header *page_list_header = NULL;
    struct journal_v2_header *journal_header = NULL;
    uint32_t delta_start_time = 0;

    while (datafile) {
        journal_header = (struct journal_v2_header *) datafile->journalfile->journal_data;
        if (!journal_header) {
            datafile = datafile->next;
            continue;
        }
        struct rrdengine_journalfile *journalfile = datafile->journalfile;
        if ((start_time_ut >= journal_header->start_time_ut && start_time_ut <= journal_header->end_time_ut)  ||
             (end_time_ut >= journal_header->start_time_ut && end_time_ut  <= journal_header->end_time_ut)) {

            // Calculate the time we will be looking for
            delta_start_time =  (start_time_ut - journal_header->start_time_ut) / USEC_PER_SEC;
            // FIXME: maybe add check
            uuid_list = (struct journal_metric_list *)(datafile->journalfile->journal_data + journal_header->metric_offset);
            uint32_t entries = journal_header->metric_count;

            struct journal_metric_list *uuid_entry = bsearch(
                &page_index->id,
                uuid_list,
                (size_t)entries,
                sizeof(struct journal_metric_list),
                journal_metric_uuid_compare);

            // FIXME: Check valid offset within file
            if (uuid_entry && delta_start_time >= uuid_entry->delta_start && delta_start_time <= uuid_entry->delta_end) {
                page_list_header = (struct journal_page_header *) (datafile->journalfile->journal_data + uuid_entry->page_offset);

                //info("JOURVALV2: Validating UUID PAGE LIST is %d", page_header_is_corrupted(journal_header, page_list_header));
                return add_pages_from_timerange(page_list_header, delta_start_time, journal_header->start_time_ut,
                        page_index, (void *) journalfile->journal_data + journal_header->extent_offset, datafile);
            }
        }
        datafile = datafile->next;
    }

    return NULL;
}

/* The caller must hold the page index lock */
static inline struct rrdeng_page_descr *
        find_first_page_in_time_range(struct pg_cache_page_index *page_index, usec_t start_time, usec_t end_time)
{

    struct rrdeng_page_descr *descr = populate_metric_time_from_journal(page_index, start_time, end_time);
//    if(descr)
//        return descr;

    Pvoid_t *PValue;
    Word_t Index;

    Index = (Word_t) (start_time / USEC_PER_SEC);
    PValue = JudyLLast(page_index->JudyL_array, &Index, PJE0);
    if (likely(NULL != PValue)) {
        descr = *PValue;
        if (is_page_in_time_range(descr, start_time, end_time)) {
            return descr;
        }
    }

    Index = (Word_t) (start_time / USEC_PER_SEC);
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
    usec_t oldest_time = page_index->oldest_time_ut;
    usec_t latest_time = page_index->latest_time_ut;

    if (unlikely(oldest_time == INVALID_TIME || descr->start_time_ut < oldest_time)) {
        page_index->oldest_time_ut = descr->start_time_ut;
    }
    if (likely(descr->end_time_ut > latest_time || latest_time == INVALID_TIME)) {
        page_index->latest_time_ut = descr->end_time_ut;
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
        oldest_time = descr->start_time_ut;
    }
    lastIndex = (Word_t)-1;
    lastPValue = JudyLLast(page_index->JudyL_array, &lastIndex, PJE0);
    if (likely(NULL != lastPValue)) {
        descr = *lastPValue;
        latest_time = descr->end_time_ut;
    }
    uv_rwlock_rdunlock(&page_index->lock);

    if (unlikely(NULL == firstPValue)) {
        fatal_assert(NULL == lastPValue);
        page_index->oldest_time_ut = page_index->latest_time_ut = INVALID_TIME;
        return;
    }
    page_index->oldest_time_ut = oldest_time;
    page_index->latest_time_ut = latest_time;
}


/* If index is NULL lookup by UUID (descr->id) */
struct rrdeng_page_descr *pg_cache_insert(struct rrdengine_instance *ctx, struct pg_cache_page_index *index, struct rrdeng_page_descr *descr, bool lock_page_index)
{
    struct page_cache *pg_cache = &ctx->pg_cache;
    Pvoid_t *PValue;
    struct pg_cache_page_index *page_index;
    unsigned long pg_cache_descr_state = descr->pg_cache_descr_state;

    if (0 != pg_cache_descr_state) {
        /* there is page cache descriptor pre-allocated state */
        struct page_cache_descr *pg_cache_descr = descr->pg_cache_descr;

        fatal_assert(pg_cache_descr_state & PG_CACHE_DESCR_ALLOCATED);
        if (pg_cache_descr->flags & RRD_PAGE_POPULATED) {
            pg_cache_reserve_pages(ctx, 1);
            if (!(pg_cache_descr->flags & RRD_PAGE_DIRTY))
                pg_cache_replaceQ_insert(ctx, descr);
        }
    }

    if (unlikely(NULL == index)) {
        uv_rwlock_rdlock(&pg_cache->metrics_index.lock);
        PValue = JudyHSGet(pg_cache->metrics_index.JudyHS_array, descr->id, sizeof(uuid_t));
        fatal_assert(NULL != PValue);
        page_index = *PValue;
        uv_rwlock_rdunlock(&pg_cache->metrics_index.lock);
    } else {
        page_index = index;
    }

    if (lock_page_index)
        uv_rwlock_wrlock(&page_index->lock);

    PValue = JudyLIns(&page_index->JudyL_array, (Word_t)(descr->start_time_ut / USEC_PER_SEC), PJE0);
    if (unlikely(*PValue)) {
        descr = *PValue;
        if (lock_page_index)
            uv_rwlock_wrunlock(&page_index->lock);
        return descr;
    }
    ++page_index->page_count;
    *PValue = descr;
    pg_cache_add_new_metric_time(page_index, descr);

    if (lock_page_index)
        uv_rwlock_wrunlock(&page_index->lock);

    uv_rwlock_wrlock(&pg_cache->pg_cache_rwlock);
    ++ctx->stats.pg_cache_insertions;
    ++pg_cache->page_descriptors;
    uv_rwlock_wrunlock(&pg_cache->pg_cache_rwlock);
    return descr;
}

/**
 * Return page information for the first page before point_in_time that satisfies the filter.
 * @param ctx DB context
 * @param page_index page index of a metric
 * @param point_in_time_ut the pages that are searched must be older than this timestamp
 * @param filter decides if the page satisfies the caller's criteria
 * @param page_info the result of the search is set in this pointer
 */
void pg_cache_get_filtered_info_prev(struct rrdengine_instance *ctx, struct pg_cache_page_index *page_index,
                                     usec_t point_in_time_ut, pg_cache_page_info_filter_t *filter,
                                     struct rrdeng_page_info *page_info)
{
    struct page_cache *pg_cache = &ctx->pg_cache;
    struct rrdeng_page_descr *descr = NULL;
    Pvoid_t *PValue;
    Word_t Index;

    (void)pg_cache;
    fatal_assert(NULL != page_index);

    Index = (Word_t)(point_in_time_ut / USEC_PER_SEC);
    uv_rwlock_rdlock(&page_index->lock);
    do {
        PValue = JudyLPrev(page_index->JudyL_array, &Index, PJE0);
        descr = unlikely(NULL == PValue) ? NULL : *PValue;
    } while (descr != NULL && !filter(descr));
    if (unlikely(NULL == descr)) {
        page_info->page_length = 0;
        page_info->start_time_ut = INVALID_TIME;
        page_info->end_time_ut = INVALID_TIME;
    } else {
        page_info->page_length = descr->page_length;
        page_info->start_time_ut = descr->start_time_ut;
        page_info->end_time_ut = descr->end_time_ut;
    }
    uv_rwlock_rdunlock(&page_index->lock);
}

/**
 * Searches for an unallocated page without triggering disk I/O. Attempts to reserve the page and get a reference.
 * @param ctx DB context
 * @param id lookup by UUID
 * @param start_time_ut exact starting time in usec
 * @param ret_page_indexp Sets the page index pointer (*ret_page_indexp) for the given UUID.
 * @return the page descriptor or NULL on failure. It can fail if:
 *         1. The page is already allocated to the page cache.
 *         2. It did not succeed to get a reference.
 *         3. It did not succeed to reserve a spot in the page cache.
 */
struct rrdeng_page_descr *pg_cache_lookup_unpopulated_and_lock(struct rrdengine_instance *ctx, uuid_t *id,
                                                               usec_t start_time_ut)
{
    struct page_cache *pg_cache = &ctx->pg_cache;
    struct rrdeng_page_descr *descr = NULL;
    struct page_cache_descr *pg_cache_descr = NULL;
    unsigned long flags;
    Pvoid_t *PValue;
    struct pg_cache_page_index *page_index = NULL;
    Word_t Index;

    uv_rwlock_rdlock(&pg_cache->metrics_index.lock);
    PValue = JudyHSGet(pg_cache->metrics_index.JudyHS_array, id, sizeof(uuid_t));
    if (likely(NULL != PValue)) {
        page_index = *PValue;
    }
    uv_rwlock_rdunlock(&pg_cache->metrics_index.lock);

    if ((NULL == PValue) || !pg_cache_try_reserve_pages(ctx, 1)) {
        /* Failed to find page or failed to reserve a spot in the cache */
        return NULL;
    }

    uv_rwlock_rdlock(&page_index->lock);
    Index = (Word_t)(start_time_ut / USEC_PER_SEC);
    PValue = JudyLGet(page_index->JudyL_array, Index, PJE0);
    if (likely(NULL != PValue)) {
        descr = *PValue;
    }
    if (NULL == PValue || 0 == descr->page_length) {
        /* Failed to find non-empty page */
        uv_rwlock_rdunlock(&page_index->lock);

        pg_cache_release_pages(ctx, 1);
        return NULL;
    }

    rrdeng_page_descr_mutex_lock(ctx, descr);
    pg_cache_descr = descr->pg_cache_descr;
    flags = pg_cache_descr->flags;
    uv_rwlock_rdunlock(&page_index->lock);

    if ((flags & RRD_PAGE_POPULATED) || !pg_cache_try_get_unsafe(descr, 1)) {
        /* Failed to get reference or page is already populated */
        rrdeng_page_descr_mutex_unlock(ctx, descr);

        pg_cache_release_pages(ctx, 1);
        return NULL;
    }
    /* success */
    rrdeng_page_descr_mutex_unlock(ctx, descr);
    rrd_stat_atomic_add(&ctx->stats.pg_cache_misses, 1);

    return descr;
}

/**
 * Searches for pages in a time range and triggers disk I/O if necessary and possible.
 * Does not get a reference.
 * @param ctx DB context
 * @param id UUID
 * @param start_time_ut inclusive starting time in usec
 * @param end_time_ut inclusive ending time in usec
 * @param page_info_arrayp It allocates (*page_arrayp) and populates it with information of pages that overlap
 *        with the time range [start_time,end_time]. The caller must free (*page_info_arrayp) with freez().
 *        If page_info_arrayp is set to NULL nothing was allocated.
 * @param ret_page_indexp Sets the page index pointer (*ret_page_indexp) for the given UUID.
 * @return the number of pages that overlap with the time range [start_time,end_time].
 */
unsigned pg_cache_preload(struct rrdengine_instance *ctx, uuid_t *id, usec_t start_time_ut, usec_t end_time_ut,
                          struct rrdeng_page_info **page_info_arrayp, struct pg_cache_page_index **ret_page_indexp)
{
    struct page_cache *pg_cache = &ctx->pg_cache;
    struct rrdeng_page_descr *descr = NULL, *preload_array[PAGE_CACHE_MAX_PRELOAD_PAGES];
    struct page_cache_descr *pg_cache_descr = NULL;
    unsigned i, j, k, preload_count, count, page_info_array_max_size;
    unsigned long flags;
    Pvoid_t *PValue;
    struct pg_cache_page_index *page_index = NULL;
    Word_t Index;
    Word_t IndexEnd;
    uint8_t failed_to_reserve;

    fatal_assert(NULL != ret_page_indexp);


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
    descr = find_first_page_in_time_range(page_index, start_time_ut, end_time_ut);
    if (NULL == descr) {
        uv_rwlock_rdunlock(&page_index->lock);
        debug(D_RRDENGINE, "%s: No page was found to attempt preload.", __func__);
        *ret_page_indexp = NULL;
        return 0;
    } else {
        Index = (Word_t)(descr->start_time_ut / USEC_PER_SEC);
    }
    if (page_info_arrayp) {
        page_info_array_max_size = PAGE_CACHE_MAX_PRELOAD_PAGES * sizeof(struct rrdeng_page_info);
        *page_info_arrayp = mallocz(page_info_array_max_size);
    }

    for (count = 0, preload_count = 0 ;
         descr != NULL && is_page_in_time_range(descr, start_time_ut, end_time_ut) ;
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
            (*page_info_arrayp)[count].start_time_ut = descr->start_time_ut;
            (*page_info_arrayp)[count].end_time_ut = descr->end_time_ut;
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
            if (descr->extent && descr->extent == next->extent) {
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
 * Searches for the first page between start_time and end_time and gets a reference.
 * start_time and end_time are inclusive.
 * If index is NULL lookup by UUID (id).
 */
struct rrdeng_page_descr *
pg_cache_lookup_next(struct rrdengine_instance *ctx, struct pg_cache_page_index *index, uuid_t *id,
                     usec_t start_time_ut, usec_t end_time_ut)
{
    struct page_cache *pg_cache = &ctx->pg_cache;
    struct rrdeng_page_descr *descr = NULL;
    struct page_cache_descr *pg_cache_descr = NULL;
    unsigned long flags;
    Pvoid_t *PValue;
    struct pg_cache_page_index *page_index = NULL;
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
        descr = find_first_page_in_time_range(page_index, start_time_ut, end_time_ut);
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
                print_page_cache_descr(descr, "", true);
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
            print_page_cache_descr(descr, "", true);
        if (!(flags & RRD_PAGE_POPULATED))
            page_not_in_cache = 1;

        pg_cache_wait_event_unsafe(descr);
        unsigned old_flags = pg_cache_descr->flags;
        rrdeng_page_descr_mutex_unlock(ctx, descr);
        if (old_flags & RRD_PAGE_INVALID) {
            info("DEBUG: Dropping invalid page descr=%lu - pg_cache=%lu - Ref=%u", descr->pg_cache_descr_state,
                 descr->pg_cache_descr->flags, descr->pg_cache_descr->refcnt);
            pg_cache_punch_hole(ctx, descr, 1, 1, NULL, true);
        }

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

struct pg_cache_page_index *create_page_index(uuid_t *id, struct rrdengine_instance *ctx)
{
    struct pg_cache_page_index *page_index;

    page_index = mallocz(sizeof(*page_index));
    fatal_assert(0 == uv_rwlock_init(&page_index->lock));
    page_index->JudyL_array = (Pvoid_t) NULL;
    uuid_copy(page_index->id, *id);
    page_index->oldest_time_ut = INVALID_TIME;
    page_index->latest_time_ut = INVALID_TIME;
    page_index->prev = NULL;
    page_index->page_count = 0;
    page_index->refcount = 0;
    page_index->writers = 0;
    page_index->ctx = ctx;
    page_index->alignment = NULL;
    page_index->latest_update_every_s = default_rrd_update_every;

    return page_index;
}

static void init_metrics_index(struct rrdengine_instance *ctx)
{
    struct page_cache *pg_cache = &ctx->pg_cache;

    pg_cache->metrics_index.JudyHS_array = (Pvoid_t) NULL;
    pg_cache->metrics_index.last_page_index = NULL;
    fatal_assert(0 == uv_rwlock_init(&pg_cache->metrics_index.lock));
}

static void init_replaceQ(struct rrdengine_instance *ctx)
{
    struct page_cache *pg_cache = &ctx->pg_cache;

    pg_cache->replaceQ.head = NULL;
    pg_cache->replaceQ.tail = NULL;
    fatal_assert(0 == uv_rwlock_init(&pg_cache->replaceQ.lock));
}

static void init_committed_page_index(struct rrdengine_instance *ctx)
{
    struct page_cache *pg_cache = &ctx->pg_cache;

    pg_cache->committed_page_index.JudyL_array = (Pvoid_t) NULL;
    fatal_assert(0 == uv_rwlock_init(&pg_cache->committed_page_index.lock));
    pg_cache->committed_page_index.latest_corr_id = 0;
    pg_cache->committed_page_index.nr_committed_pages = 0;
}

void init_page_cache(struct rrdengine_instance *ctx)
{
    struct page_cache *pg_cache = &ctx->pg_cache;

    pg_cache->page_descriptors = 0;
    pg_cache->populated_pages = 0;
    fatal_assert(0 == uv_rwlock_init(&pg_cache->pg_cache_rwlock));

    init_metrics_index(ctx);
    init_replaceQ(ctx);
    init_committed_page_index(ctx);
}

void free_page_cache(struct rrdengine_instance *ctx)
{
    struct page_cache *pg_cache = &ctx->pg_cache;
    Pvoid_t *PValue;
    struct pg_cache_page_index *page_index, *prev_page_index;
    Word_t Index;
    struct rrdeng_page_descr *descr;
    struct page_cache_descr *pg_cache_descr;

    // if we are exiting, the OS will recover all memory so do not slow down the shutdown process
    // Do the cleanup if we are compiling with NETDATA_INTERNAL_CHECKS
    // This affects the reporting of dbengine statistics which are available in real time
    // via the /api/v1/dbengine_stats endpoint
#ifndef NETDATA_DBENGINE_FREE
    if (netdata_exit)
        return;
#endif
    Word_t metrics_index_bytes = 0, pages_index_bytes = 0, pages_dirty_index_bytes = 0;

    /* Free committed page index */
    pages_dirty_index_bytes = JudyLFreeArray(&pg_cache->committed_page_index.JudyL_array, PJE0);
    fatal_assert(NULL == pg_cache->committed_page_index.JudyL_array);

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
                    dbengine_page_free(pg_cache_descr->page);
                }
                rrdeng_destroy_pg_cache_descr(ctx, pg_cache_descr);
            }
            rrdeng_page_descr_freez(descr);

            PValue = JudyLNext(page_index->JudyL_array, &Index, PJE0);
            descr = unlikely(NULL == PValue) ? NULL : *PValue;
        }

        /* Free page index */
        pages_index_bytes += JudyLFreeArray(&page_index->JudyL_array, PJE0);
        fatal_assert(NULL == page_index->JudyL_array);
        freez(page_index);
    }
    /* Free metrics index */
    metrics_index_bytes = JudyHSFreeArray(&pg_cache->metrics_index.JudyHS_array, PJE0);
    fatal_assert(NULL == pg_cache->metrics_index.JudyHS_array);
    info("Freed %lu bytes of memory from page cache.", pages_dirty_index_bytes + pages_index_bytes + metrics_index_bytes);
}

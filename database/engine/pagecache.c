// SPDX-License-Identifier: GPL-3.0-or-later
#define NETDATA_RRD_INTERNALS

#include "rrdengine.h"

void *main_mrg;
void *main_cache;
pthread_key_t query_key;

void query_key_release(void *data)
{
    Pvoid_t pl_judyL = data;
    Pvoid_t *PValue;
    info("Thread %d: Cleaning query data", gettid());
    Word_t time_index;
//    Word_t end_time_t;
    unsigned entries = 0;

    struct page_details *pd = NULL;
    if (pl_judyL) {
        for (PValue = JudyLFirst(pl_judyL, &time_index, PJE0),
            pd = unlikely(NULL == PValue) ? 0 : *PValue;
             pd != NULL;
             PValue = JudyLNext(pl_judyL, &time_index, PJE0),
            pd = unlikely(NULL == PValue) ? 0 : *PValue) {

            info("DEBUG: Page %u -- %lu - %lu (extent %lu, size %u at file %d) ", entries,  pd->start_time_t, pd->end_time_t, pd->pos, pd->size, pd->file);

            entries++;
        }
    }
    pthread_setspecific(query_key, NULL);
}

static void page_cache_free_clean_page_callback(PGC *cache __maybe_unused, PGC_ENTRY entry __maybe_unused) {
    info("FREE clean page section %lu, metric %lu, start_time %ld, end_time %ld", entry.section, entry.metric_id, entry.start_time_t, entry.end_time_t);
}
static void page_cache_save_dirty_page_callback(PGC *cache __maybe_unused, PGC_ENTRY *array __maybe_unused, size_t entries __maybe_unused) {
     info("SAVE %zu pages", entries);
//        if(!pgc_uts.stop) {
//            static const struct timespec work_duration = {.tv_sec = 0, .tv_nsec = 10000};
//            nanosleep(&work_duration, NULL);
//        }
}

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

// FIXME: This will not take descr
void pg_cache_replaceQ_insert(struct rrdengine_instance *ctx, struct rrdeng_page_descr *descr)
{
    // FIXME: DBENGINE2
    // Adding to the new page cache
    // Find metric id associated with uuid
    METRIC *this_metric = mrg_metric_get(main_mrg, descr->id, (Word_t) ctx);
    Word_t metric_id = mrg_metric_id(main_mrg, this_metric);

    // Get the data
    // We copy for now because we will add in both caches
//    void *data = dbengine_page_alloc();
//    memcpy(data, descr->page, RRDENG_BLOCK_SIZE);

    PGC_ENTRY page_entry = {
        .hot = false,
        .section = (Word_t) ctx,
        .metric_id = metric_id,
        .start_time_t = (time_t) (descr->start_time_ut / USEC_PER_SEC),
        .end_time_t =  (time_t) (descr->end_time_ut / USEC_PER_SEC),
        .update_every = descr->update_every_s,
        .size = RRDENG_BLOCK_SIZE,
        .data = descr->page
    };

    PGC_PAGE *page = pgc_page_add_and_acquire(main_cache, page_entry);
    pgc_page_release(main_cache, page);
    info("DEBUG: DBENGINE2 --> Adding new page in page cache");
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
    descr->page = NULL;
    descr->update_every_s = 0;
    descr->extent_entry = NULL;
    descr->type = 0;
    descr->file = -1;

    return descr;
}

static inline int is_page_in_time_range(struct rrdeng_page_descr *descr, usec_t start_time, usec_t end_time)
{
    usec_t pg_start, pg_end;

    pg_start = descr->start_time_ut;
    pg_end = descr->end_time_ut;

    return (pg_start < start_time && pg_end >= start_time) ||
           (pg_start >= start_time && pg_start <= end_time);
}

//static inline int is_point_in_time_in_page(struct rrdeng_page_descr *descr, usec_t point_in_time)
//{
//    return (point_in_time >= descr->start_time_ut && point_in_time <= descr->end_time_ut);
//}
//
//bool descr_exists_unsafe( struct pg_cache_page_index *page_index, time_t start_time_s)
//{
//    return (NULL != JudyLGet(page_index->JudyL_array, start_time_s, PJE0));
//}

static int journal_metric_uuid_compare(const void *key, const void *metric)
{
    return uuid_compare(*(uuid_t *) key, ((struct journal_metric_list *) metric)->uuid);
}

// Return a judyL will all pages that have start_time_ut and end_time_ut
// Pvalue of the judy will be the end time for that page
// DBENGINE2:
Pvoid_t *get_page_list(struct rrdengine_instance *ctx, uuid_t *uuid, usec_t start_time_ut, usec_t end_time_ut)
{
    Pvoid_t JudyL_page_array = (Pvoid_t) NULL;

    uv_rwlock_rdlock(&ctx->datafiles.rwlock);
    struct rrdengine_datafile *datafile = ctx->datafiles.first;
    bool lookup_continue = true;
    while (lookup_continue && datafile) {
        struct journal_v2_header *journal_header = (struct journal_v2_header *) datafile->journalfile->journal_data;
        if (!journal_header || !datafile->journalfile->is_valid) {
            datafile = datafile->next;
            continue;
        }

        if (start_time_ut >= journal_header->start_time_ut && start_time_ut <= journal_header->end_time_ut)  {
            size_t journal_metric_count = (size_t)journal_header->metric_count;
            struct journal_metric_list *uuid_list = (struct journal_metric_list *)((uint8_t *) journal_header + journal_header->metric_offset);
            struct journal_metric_list *uuid_entry = bsearch(uuid,uuid_list,journal_metric_count,sizeof(*uuid_list), journal_metric_uuid_compare);

            if (unlikely(!uuid_entry))
                    continue;

            uint32_t delta_start_time = (start_time_ut - journal_header->start_time_ut) / USEC_PER_SEC;
            uint32_t delta_end_time = (end_time_ut - journal_header->start_time_ut) / USEC_PER_SEC;
            Word_t journal_start_time_t = journal_header->start_time_ut / USEC_PER_SEC;

            struct journal_page_header *page_list_header = (struct journal_page_header *) ((uint8_t *) journal_header + uuid_entry->page_offset);
            struct journal_page_list *page_list = (struct journal_page_list *)((uint8_t *) page_list_header + sizeof(*page_list_header));

            struct journal_extent_list *extent_list = (void *)((uint8_t *)journal_header + journal_header->extent_offset);

            uint32_t entries = page_list_header->entries;

            for (uint32_t index = 0; index < entries; index++) {

                if (delta_start_time > page_list[index].delta_end_s)
                    continue;

                if (delta_end_time < page_list[index].delta_start_s) {
                    lookup_continue = false;
                    break;
                }

                Word_t Index = page_list[index].delta_start_s + journal_start_time_t;

                struct journal_page_list *page_entry = &page_list[index];

                Pvoid_t *PValue = JudyLIns(&JudyL_page_array, Index, PJE0);
                fatal_assert(NULL != PValue);
                struct page_details *pd = mallocz(sizeof(*pd));
                pd->pos = extent_list[page_entry->extent_index].datafile_offset;
                pd->size = extent_list[page_entry->extent_index].datafile_size;
                pd->file = datafile->file;
                pd->fileno = datafile->fileno;
                pd->start_time_t = Index;
                pd->end_time_t = page_list[index].delta_end_s + journal_start_time_t;
                pd->datafile = datafile;
                uuid_copy(pd->uuid, *uuid);
                *PValue = pd;
            }
        }
        datafile = datafile->next;
    }
    uv_rwlock_rdunlock(&ctx->datafiles.rwlock);

    // FIXME: Scan page index and add matching entries for the query TEMP

    struct pg_cache_page_index *page_index;
    struct page_cache *pg_cache = &ctx->pg_cache;

    uv_rwlock_rdlock(&pg_cache->metrics_index.lock);
    Pvoid_t *PValue = JudyHSGet(pg_cache->metrics_index.JudyHS_array, uuid, sizeof(uuid_t));
    page_index = (NULL == PValue || NULL == *PValue) ? NULL : *PValue;
    uv_rwlock_rdunlock(&pg_cache->metrics_index.lock);

    if (NULL == page_index)
        return JudyL_page_array;

    Word_t Index = (Word_t) (start_time_ut / USEC_PER_SEC);
    uv_rwlock_rdlock(&page_index->lock);
    PValue = JudyLLast(page_index->JudyL_array, &Index, PJE0);
    struct rrdeng_page_descr *descr = (NULL == PValue || NULL == *PValue) ? NULL : *PValue;
    while (descr && is_page_in_time_range(descr, start_time_ut, end_time_ut)) {
        PValue = JudyLIns(&JudyL_page_array, Index, PJE0);
        fatal_assert(NULL != PValue);
        struct page_details *pd = mallocz(sizeof(*pd));
        pd->pos = descr->extent->offset;
        pd->size = descr->extent->size;
        pd->file = descr->extent->datafile->file;
        pd->fileno = descr->extent->datafile->fileno;
        pd->start_time_t = Index;
        pd->end_time_t = descr->end_time_ut / USEC_PER_SEC;
        pd->datafile = datafile;
        uuid_copy(pd->uuid, *uuid);
        *PValue = pd;

        PValue = JudyLNext(page_index->JudyL_array, &Index, PJE0);
        descr = (NULL == PValue || NULL == *PValue) ? NULL : *PValue;
    }
    uv_rwlock_rdunlock(&page_index->lock);

    return JudyL_page_array;
}


/* The caller must hold the page index lock */
static inline struct rrdeng_page_descr *find_first_page_in_time_range(struct pg_cache_page_index *page_index, usec_t start_time, usec_t end_time)
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
    usec_t oldest_time = page_index->oldest_time_ut;
    usec_t latest_time = page_index->latest_time_ut;

    if (unlikely(oldest_time == INVALID_TIME || descr->start_time_ut < oldest_time)) {
        page_index->oldest_time_ut = descr->start_time_ut;
    }
    if (likely(descr->end_time_ut > latest_time || latest_time == INVALID_TIME)) {
        page_index->latest_time_ut = descr->end_time_ut;
    }

    // FIXME: DBENGINE2 rename and proper
    // This will update the metric
    update_uuid_last_time(page_index->ctx, &page_index->id, (time_t) (descr->end_time_ut / USEC_PER_SEC));
}

/* If index is NULL lookup by UUID (descr->id) */
void pg_cache_insert(struct rrdengine_instance *ctx, struct pg_cache_page_index *index, struct rrdeng_page_descr *descr)
{
    struct page_cache *pg_cache = &ctx->pg_cache;
    Pvoid_t *PValue;
    struct pg_cache_page_index *page_index;

    if (unlikely(NULL == index)) {
        uv_rwlock_rdlock(&pg_cache->metrics_index.lock);
        PValue = JudyHSGet(pg_cache->metrics_index.JudyHS_array, descr->id, sizeof(uuid_t));
        fatal_assert(NULL != PValue);
        page_index = *PValue;
        uv_rwlock_rdunlock(&pg_cache->metrics_index.lock);
    } else {
        page_index = index;
    }

    uv_rwlock_wrlock(&page_index->lock);
    PValue = JudyLIns(&page_index->JudyL_array, (Word_t)(descr->start_time_ut / USEC_PER_SEC), PJE0);
    *PValue = descr;
//    ++page_index->page_count;
//    pg_cache_add_new_metric_time(page_index, descr);
    uv_rwlock_wrunlock(&page_index->lock);

    uv_rwlock_wrlock(&pg_cache->pg_cache_rwlock);
    ++ctx->stats.pg_cache_insertions;
    ++pg_cache->page_descriptors;
    uv_rwlock_wrunlock(&pg_cache->pg_cache_rwlock);
}

//usec_t pg_cache_oldest_time_in_range(struct rrdengine_instance *ctx, uuid_t *id, usec_t start_time_ut, usec_t end_time_ut)
//{
//    struct page_cache *pg_cache = &ctx->pg_cache;
//    struct rrdeng_page_descr *descr = NULL;
//    Pvoid_t *PValue;
//    struct pg_cache_page_index *page_index = NULL;
//
//    uv_rwlock_rdlock(&pg_cache->metrics_index.lock);
//    PValue = JudyHSGet(pg_cache->metrics_index.JudyHS_array, id, sizeof(uuid_t));
//    if (likely(NULL != PValue)) {
//        page_index = *PValue;
//    }
//    uv_rwlock_rdunlock(&pg_cache->metrics_index.lock);
//    if (NULL == PValue) {
//        return INVALID_TIME;
//    }
//
//    uv_rwlock_rdlock(&page_index->lock);
//    descr = find_first_page_in_time_range(page_index, start_time_ut, end_time_ut);
//    if (NULL == descr) {
//        uv_rwlock_rdunlock(&page_index->lock);
//        return INVALID_TIME;
//    }
//    uv_rwlock_rdunlock(&page_index->lock);
//    return descr->start_time_ut;
//}

/**
 * Searches for pages in a time range and triggers disk I/O if necessary and possible.
 * Does not get a reference.
 * @param ctx DB context
 * @param page_index the page index that we are trying to preload
 * @param start_time_ut inclusive starting time in usec
 * @param end_time_ut inclusive ending time in usec
 * @return the number of pages that overlap with the time range [start_time,end_time].
 */
unsigned pg_cache_preload(struct rrdengine_instance *ctx, struct pg_cache_page_index *page_index, time_t start_time_t, time_t end_time_t)
{
//    struct rrdeng_page_descr *descr = NULL, *preload_array[PAGE_CACHE_MAX_PRELOAD_PAGES];
//    struct page_cache_descr *pg_cache_descr = NULL;
//    unsigned i, j, k, preload_count, count;
//    unsigned long flags;
    Pvoid_t *PValue;
//    Word_t Index;

    if (unlikely(NULL == page_index))
        return 0;

    // FIXME: Lets call DBENGINE2 and see whats happening
    METRIC *this_metric = mrg_metric_get(main_mrg, &page_index->id, (Word_t) ctx);
    char uuid_str[UUID_STR_LEN];
    uuid_unparse_lower(*mrg_metric_uuid(main_mrg, this_metric), uuid_str);
    info("DEBUG: Metric info %s : %ld - %ld", uuid_str,
         mrg_metric_get_first_time_t(main_mrg, this_metric),
         mrg_metric_get_latest_time_t(main_mrg, this_metric));

    info("DEBUG: Page info %s : %llu - %llu", uuid_str,
         page_index->oldest_time_ut / USEC_PER_SEC,
         page_index->latest_time_ut / USEC_PER_SEC);

    Pvoid_t  *pl_judyL = get_page_list(ctx, &page_index->id, start_time_t * USEC_PER_SEC, end_time_t * USEC_PER_SEC);
//    int ret = pthread_setspecific(query_key, pl_judyL);
//    fatal_assert(0 == ret);
    Word_t time_index;
    unsigned entries = 0;
    struct page_details *pd = NULL;
    if (pl_judyL) {
        for (PValue = JudyLFirst(pl_judyL, &time_index, PJE0),
            pd = unlikely(NULL == PValue) ? 0 : *PValue;
             pd != NULL;
             PValue = JudyLNext(pl_judyL, &time_index, PJE0),
            pd = unlikely(NULL == PValue) ? 0 : *PValue) {

            info("DEBUG: Page %d -- %ld - %ld (extent %llu, size %u at file %d) ", entries,  pd->start_time_t, pd->end_time_t, pd->pos, pd->size, pd->file);

            entries++;
        }
    }
    // Queue JudyL to the dbengine
    // FIXME: Add a free function for dbengine
    dbengine_load_page_list(ctx, pl_judyL);
//    return 1;

//    uv_rwlock_rdlock(&page_index->lock);
//    descr = find_first_page_in_time_range(page_index, start_time_ut, end_time_ut);
//    if (unlikely(NULL == descr)) {
//        uv_rwlock_rdunlock(&page_index->lock);
//        debug(D_RRDENGINE, "%s: No page was found to attempt preload.", __func__);
//        return 0;
//    }
//
//    Index = (Word_t)(descr->start_time_ut / USEC_PER_SEC);
//
//    netdata_thread_disable_cancelability();
//    struct rrdeng_page_descr *last_descr = NULL;
//    for (count = 0, preload_count = 0 ;
//         descr != NULL && is_page_in_time_range(descr, start_time_ut, end_time_ut) ;
//         PValue = JudyLNext(page_index->JudyL_array, &Index, PJE0),
//         descr = unlikely(NULL == PValue) ? NULL : *PValue) {
//        /* Iterate all pages in range */
//
//        if (last_descr == descr)
//            break;
//
//        last_descr = descr;
//
//        if (unlikely(0 == descr->page_length))
//            continue;
//        ++count;
//
//        rrdeng_page_descr_mutex_lock(ctx, descr);
//        pg_cache_descr = descr->pg_cache_descr;
//        flags = pg_cache_descr->flags;
//        if (pg_cache_can_get_unsafe(descr, 0)) {
//            if (flags & RRD_PAGE_POPULATED) {
//                /* success */
//                rrdeng_page_descr_mutex_unlock(ctx, descr);
//                debug(D_RRDENGINE, "%s: Page was found in memory.", __func__);
//                continue;
//            }
//        }
//        if (!(flags & RRD_PAGE_POPULATED) && pg_cache_try_get_unsafe(descr, 1)) {
//            preload_array[preload_count++] = descr;
//            if (PAGE_CACHE_MAX_PRELOAD_PAGES == preload_count) {
//                rrdeng_page_descr_mutex_unlock(ctx, descr);
//                break;
//            }
//        }
//        rrdeng_page_descr_mutex_unlock(ctx, descr);
//    }
//    uv_rwlock_rdunlock(&page_index->lock);
//
////    failed_to_reserve = 0;
//    k = 0;
//    for (i = 0 ; i < preload_count; ++i) {
//        struct rrdeng_cmd cmd;
//        struct rrdeng_page_descr *next;
//
//        descr = preload_array[i];
//        if (NULL == descr)
//            continue;
//
//        pg_cache_try_reserve_pages(ctx, 1);
//
//        cmd.opcode = RRDENG_READ_EXTENT;
//        cmd.read_extent.page_cache_descr[0] = descr;
//        /* don't use this page again */
//        preload_array[i] = NULL;
//        for (j = 0, k = 1 ; j < preload_count ; ++j) {
//            next = preload_array[j];
//            if (NULL == next)
//                continue;
//
//            if ((descr->extent && descr->extent == next->extent) ||
//                ((descr->extent_entry && descr->extent_entry == next->extent_entry))) {
//                /* same extent, consolidate */
//                (void ) pg_cache_try_reserve_pages(ctx, 1);
//                cmd.read_extent.page_cache_descr[k++] = next;
//                /* don't use this page again */
//                preload_array[j] = NULL;
//            }
//        }
//        cmd.read_extent.page_count = k;
//        rrdeng_enq_cmd(&ctx->worker_config, &cmd);
//    }
//    if (!preload_count) {
//        /* no such page */
//        debug(D_RRDENGINE, "%s: No page was eligible to attempt preload.", __func__);
//    }
////    rrd_stat_atomic_add(&ctx->stats.pg_cache_preload, preload_count);
//    netdata_thread_enable_cancelability();
//    return count;
}

/*
 * Searches for the first page between start_time and end_time and gets a reference.
 * start_time and end_time are inclusive.
 * If index is NULL lookup by UUID (id).
 */
void *pg_cache_lookup_next(struct rrdengine_instance *ctx __maybe_unused,
    struct pg_cache_page_index *page_index, time_t start_time_t __maybe_unused, time_t end_time_t __maybe_unused)
{
    struct rrdeng_page_descr *descr = NULL;
//    struct page_cache_descr *pg_cache_descr = NULL;
//    unsigned long flags;
//    uint8_t page_not_in_cache;
    PGC_PAGE *page_entry = NULL;

    if (unlikely(!page_index))
            return NULL;

    // FIXME: DBENGINE 2
    // Check local thread storage for info

//    pg_cache_reserve_pages(ctx, 1);
//
//    page_not_in_cache = 0;
//    uv_rwlock_rdlock(&page_index->lock);
//    int retry_count = 0;
//
//    netdata_thread_disable_cancelability();
//
//    while (1) {
//        descr = find_first_page_in_time_range(page_index, start_time_ut, end_time_ut);
//        if (NULL == descr || 0 == descr->page_length || retry_count == default_rrdeng_page_fetch_retries) {
//            /* non-empty page not found */
//            if (retry_count == default_rrdeng_page_fetch_retries)
//                error_report("Page cache timeout while waiting for page %p : returning FAIL", descr);
//            uv_rwlock_rdunlock(&page_index->lock);
//
//            pg_cache_release_pages(ctx, 1);
//            netdata_thread_enable_cancelability();
//            return NULL;
//        }
//        rrdeng_page_descr_mutex_lock(ctx, descr);
//        pg_cache_descr = descr->pg_cache_descr;
//        flags = pg_cache_descr->flags;
//
//        if ((flags & RRD_PAGE_POPULATED) && pg_cache_try_get_unsafe(descr, 0)) {
//            /* success */
//            rrdeng_page_descr_mutex_unlock(ctx, descr);
//            debug(D_RRDENGINE, "%s: Page was found in memory.", __func__);
//            break;
//        }
//        if (!(flags & RRD_PAGE_POPULATED) && pg_cache_try_get_unsafe(descr, 1)) {
//            struct rrdeng_cmd cmd;
//
//            uv_rwlock_rdunlock(&page_index->lock);
//            cmd.opcode = RRDENG_READ_PAGE;
//            cmd.read_page.page_cache_descr = descr;
//            rrdeng_enq_cmd(&ctx->worker_config, &cmd);
//
//            debug(D_RRDENGINE, "%s: Waiting for page to be asynchronously read from disk:", __func__);
//                if (unlikely(debug_flags & D_RRDENGINE))
//                print_page_cache_descr(descr, "", true);
//            while (!(pg_cache_descr->flags & RRD_PAGE_POPULATED)) {
//                pg_cache_wait_event_unsafe(descr);
//            }
//            /* success */
//            /* Downgrade exclusive reference to allow other readers */
//            pg_cache_descr->flags &= ~RRD_PAGE_LOCKED;
//            pg_cache_wake_up_waiters_unsafe(descr);
//            rrdeng_page_descr_mutex_unlock(ctx, descr);
//            rrd_stat_atomic_add(&ctx->stats.pg_cache_misses, 1);
//                netdata_thread_enable_cancelability();
//            return descr;
//        }
//
//        uv_rwlock_rdunlock(&page_index->lock);
//        debug(D_RRDENGINE, "%s: Waiting for page to be unlocked:", __func__);
//        if(unlikely(debug_flags & D_RRDENGINE))
//            print_page_cache_descr(descr, "", true);
//        if (!(flags & RRD_PAGE_POPULATED))
//            page_not_in_cache = 1;
//
//        if (pg_cache_timedwait_event_unsafe(descr, default_rrdeng_page_fetch_timeout) == UV_ETIMEDOUT) {
//            error_report("Page cache timeout while waiting for page %p : retry count = %d", descr, retry_count);
//            ++retry_count;
//        }
//        rrdeng_page_descr_mutex_unlock(ctx, descr);
//        /* reset scan to find again */
//        uv_rwlock_rdlock(&page_index->lock);
//    }
//    uv_rwlock_rdunlock(&page_index->lock);
//
//    if (!(flags & RRD_PAGE_DIRTY))
//        pg_cache_replaceQ_set_hot(ctx, descr);
//    pg_cache_release_pages(ctx, 1);
//    if (page_not_in_cache)
//        rrd_stat_atomic_add(&ctx->stats.pg_cache_misses, 1);
//    else {
//        rrd_stat_atomic_add(&ctx->stats.pg_cache_hits, 1);
//    }
//    netdata_thread_enable_cancelability();
////    (void) pg_cache_preload(ctx, page_index, end_time_ut, end_time_ut);
//    return descr;
    return page_entry;
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
    page_index->refcount = 0;
    page_index->ctx = ctx;
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

void init_page_cache(struct rrdengine_instance *ctx)
{
    static int mrg_init = 0;
    struct page_cache *pg_cache = &ctx->pg_cache;

    pg_cache->page_descriptors = 0;
    pg_cache->active_descriptors = 0;
    pg_cache->populated_pages = 0;
    fatal_assert(0 == uv_rwlock_init(&pg_cache->pg_cache_rwlock));

    if (!mrg_init) {
        (void)pthread_key_create(&query_key, NULL /*query_key_release*/);

        main_mrg = mrg_create();
        mrg_init = 1;

        main_cache = pgc_create(
            default_rrdeng_page_cache_mb * 1024 * 1024,
            page_cache_free_clean_page_callback,
            64,
            page_cache_save_dirty_page_callback,
            1000,
            10000,
            1,
            PGC_OPTIONS_NONE,
            16);
    }

    init_metrics_index(ctx);
//    init_replaceQ(ctx);
//    init_committed_page_index(ctx);

    fatal_assert(0 == uv_rwlock_init(&pg_cache->v2_lock));
}

//void free_page_cache(struct rrdengine_instance *ctx)
//{
//    struct page_cache *pg_cache = &ctx->pg_cache;
//    Pvoid_t *PValue;
//    struct pg_cache_page_index *page_index, *prev_page_index;
//    Word_t Index;
//    struct rrdeng_page_descr *descr;
//    struct page_cache_descr *pg_cache_descr;
//
//    // if we are exiting, the OS will recover all memory so do not slow down the shutdown process
//    // Do the cleanup if we are compiling with NETDATA_INTERNAL_CHECKS
//    // This affects the reporting of dbengine statistics which are available in real time
//    // via the /api/v1/dbengine_stats endpoint
//#ifndef NETDATA_DBENGINE_FREE
//    if (netdata_exit)
//        return;
//#endif
//    Word_t metrics_index_bytes = 0, pages_index_bytes = 0, pages_dirty_index_bytes = 0;
//
//    /* Free committed page index */
//    pages_dirty_index_bytes = JudyLFreeArray(&pg_cache->committed_page_index.JudyL_array, PJE0);
//    fatal_assert(NULL == pg_cache->committed_page_index.JudyL_array);
//
//    for (page_index = pg_cache->metrics_index.last_page_index ;
//         page_index != NULL ;
//         page_index = prev_page_index) {
//
//        prev_page_index = page_index->prev;
//
//        /* Find first page in range */
//        Index = (Word_t) 0;
//        PValue = JudyLFirst(page_index->JudyL_array, &Index, PJE0);
//        descr = unlikely(NULL == PValue) ? NULL : *PValue;
//
//        while (descr != NULL) {
//            /* Iterate all page descriptors of this metric */
//
//            if (descr->pg_cache_descr_state & PG_CACHE_DESCR_ALLOCATED) {
//                /* Check rrdenglocking.c */
//                pg_cache_descr = descr->pg_cache_descr;
//                if (pg_cache_descr->flags & RRD_PAGE_POPULATED) {
//                    dbengine_page_free(pg_cache_descr->page);
//                }
//                rrdeng_destroy_pg_cache_descr(ctx, pg_cache_descr);
//            }
//            rrdeng_page_descr_freez(descr);
//
//            PValue = JudyLNext(page_index->JudyL_array, &Index, PJE0);
//            descr = unlikely(NULL == PValue) ? NULL : *PValue;
//        }
//
//        /* Free page index */
//        pages_index_bytes += JudyLFreeArray(&page_index->JudyL_array, PJE0);
//        fatal_assert(NULL == page_index->JudyL_array);
//        freez(page_index);
//    }
//    /* Free metrics index */
//    metrics_index_bytes = JudyHSFreeArray(&pg_cache->metrics_index.JudyHS_array, PJE0);
//    fatal_assert(NULL == pg_cache->metrics_index.JudyHS_array);
//    info("Freed %lu bytes of memory from page cache.", pages_dirty_index_bytes + pages_index_bytes + metrics_index_bytes);
//}


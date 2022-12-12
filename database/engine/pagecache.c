// SPDX-License-Identifier: GPL-3.0-or-later
#define NETDATA_RRD_INTERNALS

#include "rrdengine.h"

struct mrg *main_mrg = NULL;
struct pgc *main_cache = NULL;
struct pgc *open_cache = NULL;
struct rrdeng_cache_efficiency_stats rrdeng_cache_efficiency_stats = {};

pthread_key_t query_key;


// FIXME: Check if it can of use to automatically release reserved pages
//void query_key_release(void *data)
//{
//    Pvoid_t pl_judyL = data;
//    Pvoid_t *PValue;
//    info("Thread %d: Cleaning query data", gettid());
//    Word_t time_index;
////    Word_t end_time_t;
//    unsigned entries = 0;
//
//    struct page_details *pd = NULL;
//    if (pl_judyL) {
//        for (PValue = JudyLFirst(pl_judyL, &time_index, PJE0),
//            pd = unlikely(NULL == PValue) ? 0 : *PValue;
//             pd != NULL;
//             PValue = JudyLNext(pl_judyL, &time_index, PJE0),
//            pd = unlikely(NULL == PValue) ? 0 : *PValue) {
//
//            info("DEBUG: Page %u -- %lu - %lu (extent %lu, size %u at file %d) ", entries,  pd->start_time_t, pd->end_time_t, pd->pos, pd->size, pd->file);
//            freez(pd);
//
//            entries++;
//        }
//    }
//    JudyLFreeArray(&pl_judyL, PJE0);
//    pthread_setspecific(query_key, NULL);
//}

void free_judyl_page_list(Pvoid_t pl_judyL)
{
    if (!pl_judyL)
        return;

    Pvoid_t *PValue;
    struct page_details *pd;
    Word_t time_index = 0;
    for (PValue = JudyLFirst(pl_judyL, &time_index, PJE0),
        pd = unlikely(NULL == PValue) ? 0 : *PValue;
         pd != NULL;
         PValue = JudyLNext(pl_judyL, &time_index, PJE0),
        pd = unlikely(NULL == PValue) ? 0 : *PValue) {

        freez(pd);
    }
    JudyLFreeArray(&pl_judyL, PJE0);
}

static void dbengine_clean_page_callback(PGC *cache __maybe_unused, PGC_ENTRY entry __maybe_unused)
{
    // Release storage associated with the page
    //info("FREE clean page section %lu, metric %lu, start_time %ld, end_time %ld", entry.section, entry.metric_id, entry.start_time_t, entry.end_time_t);
    freez(entry.data);
}

static void dbengine_flush_callback(PGC *cache __maybe_unused, PGC_ENTRY *array __maybe_unused, size_t entries __maybe_unused)
{
     struct completion queue_flush_command;
     completion_init(&queue_flush_command);

     Pvoid_t JudyL_flush = NULL;
     Pvoid_t *PValue;

     struct rrdengine_instance *ctx = (struct rrdengine_instance *) array[0].section;
     size_t bytes_per_point =  PAGE_POINT_CTX_SIZE_BYTES(ctx);

     for (size_t Index = 0 ; Index < entries; Index++) {
        time_t start_time_t = array[Index].start_time_t;
        time_t end_time_t = array[Index].end_time_t;
        struct rrdeng_page_descr *descr = rrdeng_page_descr_mallocz();

        uuid_copy(descr->uuid, *(mrg_metric_uuid(main_mrg, (METRIC *) array[Index].metric_id)));
        descr->start_time_ut = start_time_t * USEC_PER_SEC;
        descr->end_time_ut = end_time_t * USEC_PER_SEC;
        descr->update_every_s = array[Index].update_every;
        descr->type = ctx->page_type;
        descr->page_length = (end_time_t - start_time_t + 1) * bytes_per_point;
        descr->page = array[Index].data;
        PValue = JudyLIns(&JudyL_flush, (Word_t) Index, PJE0);
        fatal_assert( NULL != PValue);
        *PValue = descr;
     }

     struct rrdeng_cmd cmd;
     cmd.opcode = RRDENG_FLUSH_PAGES;
     cmd.data = JudyL_flush;
     cmd.completion = &queue_flush_command;
     rrdeng_enq_cmd(&ctx->worker_config, &cmd);

     completion_wait_for(&queue_flush_command);
     completion_destroy(&queue_flush_command);
}

static void open_cache_clean_page_callback(PGC *cache __maybe_unused, PGC_ENTRY entry __maybe_unused)
{
     // Release storage associated with the page
     //info("FREE clean page section %lu, metric %lu, start_time %ld, end_time %ld", entry.section, entry.metric_id, entry.start_time_t, entry.end_time_t);
     //freez(entry.data);
     ;
}

static void open_cache_flush_callback(PGC *cache __maybe_unused, PGC_ENTRY *array __maybe_unused, size_t entries __maybe_unused)
{
     info("Datafile flushing %zu pages", entries);
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
    descr->type = 0;
    return descr;
}

static inline bool is_page_in_time_range(time_t page_first_time_s, time_t page_last_time_s, time_t wanted_start_time_s, time_t wanted_end_time_s) {
    return page_first_time_s <= wanted_end_time_s && page_last_time_s >= wanted_start_time_s;
}

static int journal_metric_uuid_compare(const void *key, const void *metric)
{
    return uuid_compare(*(uuid_t *) key, ((struct journal_metric_list *) metric)->uuid);
}

static size_t get_page_list_from_pgc(PGC *cache, METRIC *metric, struct rrdengine_instance *ctx, time_t wanted_start_time_s, time_t wanted_end_time_s,
                                   Pvoid_t *JudyL_page_array, size_t *cache_gaps, time_t *first_page_starting_time_s, bool set_page) {

    size_t pages_found_in_cache = 0;
    uuid_t *uuid = mrg_metric_uuid(main_mrg, metric);
    Word_t metric_id = mrg_metric_id(main_mrg, metric);

    time_t current_start_time_s = wanted_start_time_s;
    time_t previous_page_update_every_s = mrg_metric_get_update_every(main_mrg, metric);

    if(!previous_page_update_every_s)
        previous_page_update_every_s = default_rrd_update_every;

    time_t previous_page_last_time_s = current_start_time_s - previous_page_update_every_s;

    do {
        PGC_PAGE *page = pgc_page_get_and_acquire(cache, (Word_t)ctx, (Word_t)metric_id, current_start_time_s, false);
        if(!page) {
            (*cache_gaps)++;
            break;
        }

        time_t page_first_time_s = pgc_page_start_time_t(page);
        time_t page_last_time_s = pgc_page_end_time_t(page);
        time_t page_update_every_s = pgc_page_update_every(page);

        if(!is_page_in_time_range(page_first_time_s, page_last_time_s, wanted_start_time_s, wanted_end_time_s)) {
            // not a useful page for this query
            pgc_page_release(cache, page);
            page = NULL;
            (*cache_gaps)++;
            break;
        }

        if (page_first_time_s - previous_page_last_time_s > previous_page_update_every_s)
            (*cache_gaps)++;

        if(*first_page_starting_time_s == INVALID_TIME || page_first_time_s < *first_page_starting_time_s)
            *first_page_starting_time_s = page_first_time_s;

        Pvoid_t *PValue = JudyLIns(JudyL_page_array, (Word_t) page_first_time_s, PJE0);
        if (!PValue || PValue == PJERR)
            fatal("DBENGINE: corrupted judy array in %s()", __FUNCTION__ );

        if (unlikely(*PValue)) {
            struct page_details *pd = *PValue;

            internal_fatal(
                    pd->first_time_s != page_first_time_s ||
                    pd->last_time_s != page_last_time_s ||
                    pd->update_every_s != page_update_every_s,
                    "DBENGINE: page is already in judy with different retention");

            pgc_page_release(cache, page);
        }
        else {
            struct page_details *pd = mallocz(sizeof(*pd));
            pd->pos = 0;
            pd->size = pgc_page_data_size(cache, page);
            pd->file = 0;
            pd->fileno = 0;
            pd->first_time_s = page_first_time_s;
            pd->last_time_s = page_last_time_s;
            pd->datafile = NULL;
            pd->page_length = pgc_page_data_size(cache, page);
            pd->update_every_s = pgc_page_update_every(page);
            pd->page = (set_page) ? page : NULL;
            pd->type = ctx->page_type;
            uuid_copy(pd->uuid, *uuid);
            *PValue = pd;

            pages_found_in_cache++;
        }

        // prepare for the next iteration
        previous_page_last_time_s = page_last_time_s;

        if(page_update_every_s > 0)
            previous_page_update_every_s = page_update_every_s;

        current_start_time_s = previous_page_last_time_s + previous_page_update_every_s;

    } while(current_start_time_s <= wanted_end_time_s);

    return pages_found_in_cache;
}

// Return a judyL will all pages that have start_time_ut and end_time_ut
// Pvalue of the judy will be the end time for that page
// DBENGINE2:
Pvoid_t get_page_list(struct rrdengine_instance *ctx, METRIC *metric, usec_t start_time_ut, usec_t end_time_ut, time_t *first_page_first_time_s, size_t *pages_to_load) {
    Pvoid_t JudyL_page_array = (Pvoid_t) NULL;

    uuid_t *uuid = mrg_metric_uuid(main_mrg, metric);
    Word_t metric_id = mrg_metric_id(main_mrg, metric);
    time_t wanted_start_time_s = (time_t)(start_time_ut / USEC_PER_SEC);
    time_t wanted_end_time_s = (time_t)(end_time_ut / USEC_PER_SEC);

    size_t pages_found_in_cache = 0, pages_found_in_journals_v2 = 0, pages_found_pass3 = 0, pages_pending = 0, pages_total = 0;
    size_t cache_gaps = 0;
    bool done_v2 = false;

    time_t first_page_starting_time_s = INVALID_TIME;

    // --------------------------------------------------------------
    // PASS 1: Check what the main page cache has available

    size_t pages_pass1 = get_page_list_from_pgc(main_cache, metric, ctx, wanted_start_time_s, wanted_end_time_s,
                                                &JudyL_page_array, &cache_gaps,
                                                &first_page_starting_time_s, true);
    pages_found_in_cache += pages_pass1;
    pages_total += pages_pass1;

    if(pages_found_in_cache && !cache_gaps)
        goto we_are_done;

    // --------------------------------------------------------------
    // PASS 2: Check what the open journal page cache has available

    size_t pages_pass2 = get_page_list_from_pgc(open_cache, metric, ctx, wanted_start_time_s, wanted_end_time_s,
                                                &JudyL_page_array, &cache_gaps,
                                                &first_page_starting_time_s, false);
    pages_found_in_cache += pages_pass2;
    pages_total += pages_pass2;

    // --------------------------------------------------------------
    // PASS 3: Check Journal v2 to fill the gaps

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

            if (unlikely(!uuid_entry)) {
                datafile = datafile->next;
                continue;
            }

            uint32_t delta_start_time = (start_time_ut - journal_header->start_time_ut) / USEC_PER_SEC;
            uint32_t delta_end_time = (end_time_ut - journal_header->start_time_ut) / USEC_PER_SEC;
            time_t journal_start_time_s = (time_t)(journal_header->start_time_ut / USEC_PER_SEC);

            struct journal_page_header *page_list_header = (struct journal_page_header *) ((uint8_t *) journal_header + uuid_entry->page_offset);
            struct journal_page_list *page_list = (struct journal_page_list *)((uint8_t *) page_list_header + sizeof(*page_list_header));

            struct journal_extent_list *extent_list = (void *)((uint8_t *)journal_header + journal_header->extent_offset);

            uint32_t entries = page_list_header->entries;

            for (uint32_t index = 0; index < entries; index++) {
                struct journal_page_list *page_entry_in_journal = &page_list[index];

                if (delta_start_time > page_entry_in_journal->delta_end_s)
                    continue;

                if (delta_end_time < page_entry_in_journal->delta_start_s) {
                    lookup_continue = false;
                    break;
                }

                time_t page_first_time_s = page_entry_in_journal->delta_start_s + journal_start_time_s;
                time_t page_last_time_s = page_entry_in_journal->delta_end_s + journal_start_time_s;
                time_t page_update_every_s = page_entry_in_journal->update_every_s;

                if(is_page_in_time_range(page_first_time_s, page_last_time_s, wanted_start_time_s, wanted_end_time_s)) {

                    if (first_page_starting_time_s == INVALID_TIME || page_first_time_s < first_page_starting_time_s)
                        first_page_starting_time_s = page_first_time_s;

                    Pvoid_t *PValue = JudyLIns(&JudyL_page_array, page_first_time_s, PJE0);
                    if (!PValue || PValue == PJERR)
                        fatal("DBENGINE: corrupted judy array");

                    if (unlikely(*PValue)) {
                        // it is already in the judy

                        struct page_details *pd = *PValue; (void)pd;
                        internal_fatal(
                                pd->first_time_s != page_first_time_s ||
                                pd->last_time_s != page_last_time_s,
                                "DBENGINE: page is already in judy with different retention");
                    }
                    else {
                        struct page_details *pd = mallocz(sizeof(*pd));
                        pd->pos = extent_list[page_entry_in_journal->extent_index].datafile_offset;
                        pd->size = extent_list[page_entry_in_journal->extent_index].datafile_size;
                        pd->file = datafile->file;
                        pd->fileno = datafile->fileno;
                        pd->first_time_s = page_first_time_s;
                        pd->last_time_s = page_last_time_s;
                        pd->datafile = datafile;
                        pd->page_length = page_entry_in_journal->page_length;
                        pd->update_every_s = page_update_every_s;
                        pd->type = page_entry_in_journal->type;
                        pd->page = NULL;
                        uuid_copy(pd->uuid, *uuid);
                        *PValue = pd;

                        pages_found_in_journals_v2++;
                        pages_total++;
                    }
                }
            }
        }

        datafile = datafile->next;
    }
    uv_rwlock_rdunlock(&ctx->datafiles.rwlock);
    done_v2 = true;

    // --------------------------------------------------------------
    // PASS 4: Check the cache again

    // Try To add additional pages in the list
    // Pages will be acquired -- should be ignored by the dbengine reader

    if(pages_found_in_journals_v2) {
        bool first = true;
        Word_t current_page_end_time = 0;
        Pvoid_t *PValue;

        first_page_starting_time_s = INVALID_TIME;
        pages_total = 0;
        while((PValue = JudyLFirstThenNext(JudyL_page_array, &current_page_end_time, &first))) {

            if(first_page_starting_time_s == INVALID_TIME || (time_t)current_page_end_time < first_page_starting_time_s)
                first_page_starting_time_s = (time_t)current_page_end_time;

            struct page_details *pd = *PValue;
            if(!pd->page) {
                pd->page = pgc_page_get_and_acquire(main_cache, (Word_t) ctx, (Word_t) metric_id, pd->first_time_s, true);
                if(pd->page)
                    pages_found_pass3++;
                else
                    pages_pending++;
            }

            pages_total++;
        }
    }

we_are_done:
    if(first_page_first_time_s)
        *first_page_first_time_s = first_page_starting_time_s;

    if(pages_to_load)
        *pages_to_load = pages_pending;

    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.queries, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.queries_journal_v2, done_v2 ? 1 : 0, __ATOMIC_RELAXED);
    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_total, pages_total, __ATOMIC_RELAXED);
    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_found_in_cache, pages_found_in_cache, __ATOMIC_RELAXED);
    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_loaded_from_journal_v2, pages_found_in_journals_v2, __ATOMIC_RELAXED);
    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_found_in_cache_at_pass3, pages_found_pass3, __ATOMIC_RELAXED);
    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_to_load_from_disk, pages_pending, __ATOMIC_RELAXED);

    return JudyL_page_array;
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
    update_metric_latest_time_by_uuid(page_index->ctx, &page_index->id, (time_t) (descr->end_time_ut / USEC_PER_SEC));
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
    uv_rwlock_wrunlock(&page_index->lock);
}

/**
 * Searches for pages in a time range and triggers disk I/O if necessary and possible.
 * @param ctx DB context
 * @param handle query handle as initialized
 * @param start_time_ut inclusive starting time in usec
 * @param end_time_ut inclusive ending time in usec
 * @return 1 / 0 (pages found or not found)
 */
time_t pg_cache_preload(struct rrdengine_instance *ctx, struct rrdeng_query_handle *handle, time_t start_time_t, time_t end_time_t) {
    if (unlikely(!handle || !handle->metric))
        return 0;

    time_t first_page_first_time_s = INVALID_TIME;
    size_t pages_to_load = 0;

    struct page_details_control *pdc = mallocz(sizeof(*pdc));
    completion_init(&pdc->completion);
    handle->pdc = pdc;

    handle->pdc->pl_JudyL = get_page_list(ctx, handle->metric,
                                     start_time_t * USEC_PER_SEC, end_time_t * USEC_PER_SEC,
                                     &first_page_first_time_s, &pages_to_load);

    if (pages_to_load && handle->pdc->pl_JudyL) {
        pdc->pl_JudyL = handle->pdc->pl_JudyL;
        pdc->reference_count = 1;
        pdc->jobs_started = 0;
        pdc->jobs_completed = 0;
        dbengine_load_page_list(ctx, pdc);
    }
    else
        completion_mark_complete(&handle->pdc->completion);

    return first_page_first_time_s;
}

/*
 * Searches for the first page between start_time and end_time and gets a reference.
 * start_time and end_time are inclusive.
 * If index is NULL lookup by UUID (id).
 */
struct pgc_page *pg_cache_lookup_next(struct rrdengine_instance *ctx __maybe_unused,  struct rrdeng_query_handle *handle,
        time_t start_time_t __maybe_unused, time_t end_time_t __maybe_unused, time_t *next_page_start_time_s)
{
    if (unlikely(!handle || !handle->pdc || !handle->pdc->pl_JudyL)) {

        if(next_page_start_time_s)
            *next_page_start_time_s = INVALID_TIME;

        return NULL;
    }

    completion_wait_for(&handle->pdc->completion);

    // Caller will request the next page which will be end_time + update_every so search inclusive from Index
    PGC_PAGE *page = NULL;
    struct page_details *pd;
    Word_t Index = start_time_t;
    Pvoid_t *PValue = JudyLFirst(handle->pdc->pl_JudyL, &Index, PJE0);
    if (!PValue || !*PValue) {

        if(next_page_start_time_s)
            *next_page_start_time_s = INVALID_TIME;

        return NULL;
    }
    pd = *PValue;
    page = pd->page;

    if(next_page_start_time_s) {
        Pvoid_t *PValue = JudyLNext(handle->pdc->pl_JudyL, &Index, PJE0);
        if(!PValue || !*PValue)
            *next_page_start_time_s = INVALID_TIME;
        else
            *next_page_start_time_s = (time_t)Index;
    }
    return page;
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
    fatal_assert(0 == uv_rwlock_init(&pg_cache->metrics_index.lock));
}

void init_page_cache(struct rrdengine_instance *ctx)
{
    static SPINLOCK spinlock = NETDATA_SPINLOCK_INITIALIZER;
    static bool initialized = false;

    struct page_cache *pg_cache = &ctx->pg_cache;

    pg_cache->page_descriptors = 0;
    pg_cache->active_descriptors = 0;
    pg_cache->populated_pages = 0;
    fatal_assert(0 == uv_rwlock_init(&pg_cache->pg_cache_rwlock));

    netdata_spinlock_lock(&spinlock);
    if (!initialized) {
        initialized = true;

        // FIXME: Check if it can be of use
        // (void)pthread_key_create(&query_key, NULL /*query_key_release*/);

        main_mrg = mrg_create();

        main_cache = pgc_create(
            default_rrdeng_page_cache_mb * 1024 * 1024,
            dbengine_clean_page_callback,
            rrdeng_pages_per_extent,
            dbengine_flush_callback,
            100,                                //
            1000,                           //
            1,                                          // don't delay too much other threads
            PGC_OPTIONS_DEFAULT,                                // AUTOSCALE = 2x max hot pages
            0,                                                 // 0 = as many as the system cpus
            0
            );

        open_cache = pgc_create(
            0,
            open_cache_clean_page_callback,
            1,
            open_cache_flush_callback,
            100,                                //
            1000,                           //
            1,                                          // don't delay too much other threads
            PGC_OPTIONS_DEFAULT,                                // AUTOSCALE = 2x max hot pages
            0,                                                 // 0 = as many as the system cpus
            0
            );
    }
    netdata_spinlock_unlock(&spinlock);

    init_metrics_index(ctx);
}

// FIXME: DBENGINE2 Release page cache properly
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


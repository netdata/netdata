// SPDX-License-Identifier: GPL-3.0-or-later
#define NETDATA_RRD_INTERNALS

#include "rrdengine.h"

MRG *main_mrg = NULL;
PGC *main_cache = NULL;
PGC *open_cache = NULL;
PGC *extent_cache = NULL;
struct rrdeng_cache_efficiency_stats rrdeng_cache_efficiency_stats = {};

static void main_cache_free_clean_page_callback(PGC *cache __maybe_unused, PGC_ENTRY entry __maybe_unused)
{
    // Release storage associated with the page
    //info("FREE clean page section %lu, metric %lu, start_time %ld, end_time %ld", entry.section, entry.metric_id, entry.start_time_t, entry.end_time_t);
    freez(entry.data);
}

static void main_cache_flush_dirty_page_callback(PGC *cache __maybe_unused, PGC_ENTRY *entries_array __maybe_unused, PGC_PAGE **pages_array __maybe_unused, size_t entries __maybe_unused)
{
     Pvoid_t JudyL_flush = NULL;
     Pvoid_t *PValue;

     struct rrdengine_instance *ctx = (struct rrdengine_instance *) entries_array[0].section;
     size_t bytes_per_point =  PAGE_POINT_CTX_SIZE_BYTES(ctx);

     for (size_t Index = 0 ; Index < entries; Index++) {
        time_t start_time_t = entries_array[Index].start_time_t;
        time_t end_time_t = entries_array[Index].end_time_t;
        struct rrdeng_page_descr *descr = callocz(1, sizeof(*descr));

        descr->id = mrg_metric_uuid(main_mrg, (METRIC *) entries_array[Index].metric_id);
        descr->metric_id = entries_array[Index].metric_id;
        descr->start_time_ut = start_time_t * USEC_PER_SEC;
        descr->end_time_ut = end_time_t * USEC_PER_SEC;
        descr->update_every_s = entries_array[Index].update_every;
        descr->type = ctx->page_type;

        descr->page_length = (end_time_t - (start_time_t - descr->update_every_s)) / descr->update_every_s * bytes_per_point;

        if(descr->page_length > entries_array[Index].size) {
            descr->page_length = entries_array[Index].size;

            error_limit_static_global_var(erl, 1, 0);
            error_limit(&erl, "DBENGINE: page exceeds the maximum size, adjusting it to max.");
        }

        descr->page = pgc_page_dup(main_cache, pages_array[Index]);
        PValue = JudyLIns(&JudyL_flush, (Word_t) Index, PJE0);
        fatal_assert( NULL != PValue);
        *PValue = descr;

        internal_fatal(descr->page_length > RRDENG_BLOCK_SIZE, "faulty page length calculation");
     }

     struct rrdeng_cmd cmd;
     cmd.opcode = RRDENG_FLUSH_PAGES;
     cmd.data = JudyL_flush;
     cmd.completion = NULL;
     rrdeng_enq_cmd(&ctx->worker_config, &cmd);
}

static void open_cache_free_clean_page_callback(PGC *cache __maybe_unused, PGC_ENTRY entry __maybe_unused)
{
    struct rrdengine_datafile *datafile = entry.data;
    datafile_release(datafile);
}

static void open_cache_flush_dirty_page_callback(PGC *cache __maybe_unused, PGC_ENTRY *entries_array __maybe_unused, PGC_PAGE **pages_array __maybe_unused, size_t entries __maybe_unused)
{
    ;
}

static void extent_cache_free_clean_page_callback(PGC *cache __maybe_unused, PGC_ENTRY entry __maybe_unused)
{
    freez(entry.data);
}

static void extent_cache_flush_dirty_page_callback(PGC *cache __maybe_unused, PGC_ENTRY *entries_array __maybe_unused, PGC_PAGE **pages_array __maybe_unused, size_t entries __maybe_unused)
{
    ;
}


static inline bool is_page_in_time_range(time_t page_first_time_s, time_t page_last_time_s, time_t wanted_start_time_s, time_t wanted_end_time_s) {
    return page_first_time_s <= wanted_end_time_s && page_last_time_s >= wanted_start_time_s;
}

static int journal_metric_uuid_compare(const void *key, const void *metric)
{
    return uuid_compare(*(uuid_t *) key, ((struct journal_metric_list *) metric)->uuid);
}

static size_t get_page_list_from_pgc(PGC *cache, METRIC *metric, struct rrdengine_instance *ctx,
        time_t wanted_start_time_s, time_t wanted_end_time_s,
        Pvoid_t *JudyL_page_array, size_t *cache_gaps, time_t *first_page_starting_time_s,
        bool open_cache_mode, PDC_PAGE_STATUS tags) {

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
        size_t page_length = pgc_page_data_size(cache, page);

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
            UNUSED(pd);

//            internal_error(
//                    pd->first_time_s != page_first_time_s ||
//                    pd->last_time_s != page_last_time_s ||
//                    pd->update_every_s != page_update_every_s,
//                    "DBENGINE: duplicate page with different retention in %s cache "
//                    "1st: %ld to %ld, ue %u, size %u "
//                    "2nd: %ld to %ld, ue %ld size %zu "
//                    "- ignoring the second",
//                    cache == open_cache ? "open" : "main",
//                    pd->first_time_s, pd->last_time_s, pd->update_every_s, pd->page_length,
//                    page_first_time_s, page_last_time_s, page_update_every_s, page_length);

            pgc_page_release(cache, page);
        }
        else {

            internal_fatal(pgc_page_metric(page) != metric_id, "Wrong metric id in page found in cache");
            internal_fatal(pgc_page_section(page) != (Word_t)ctx, "Wrong section in page found in cache");

            struct page_details *pd = callocz(1, sizeof(*pd));
            pd->metric_id = metric_id;
            pd->first_time_s = page_first_time_s;
            pd->last_time_s = page_last_time_s;
            pd->page_length = page_length;
            pd->update_every_s = page_update_every_s;
            pd->page = (open_cache_mode) ? NULL : page;
            pd->status |= ((pd->page) ? (PDC_PAGE_READY | PDC_PAGE_PRELOADED) : 0) | tags;
            pd->type = ctx->page_type;

            if(open_cache_mode) {
                struct extent_io_data *xio = (struct extent_io_data *)pgc_page_custom_data(cache, page);
                uuid_copy(pd->datafile.extent.page_uuid, *uuid);
                pd->datafile.ptr = pgc_page_data(page);
                pd->datafile.file = xio->file;
                pd->datafile.extent.pos = xio->pos;
                pd->datafile.extent.bytes = xio->bytes;
                pd->datafile.fileno = pd->datafile.ptr->fileno;
                datafile_acquire_dup(pd->datafile.ptr); // has to be done before releasing the page
                pd->status |= PDC_PAGE_DATAFILE_ACQUIRED;
                pgc_page_release(cache, page);
            }

            *PValue = pd;

            pages_found_in_cache++;
        }

        // prepare for the next iteration
        previous_page_last_time_s = page_last_time_s;

        if(page_update_every_s > 0)
            previous_page_update_every_s = page_update_every_s;

        current_start_time_s = previous_page_last_time_s + previous_page_update_every_s;

    } while(current_start_time_s <= wanted_end_time_s);

    if(previous_page_last_time_s < wanted_end_time_s)
        cache_gaps++;

    return pages_found_in_cache;
}

static size_t list_has_time_gaps(struct rrdengine_instance *ctx, METRIC *metric, Pvoid_t JudyL_page_array,
                                 time_t wanted_start_time_s, time_t wanted_end_time_s,
                                 time_t *first_page_starting_time_s,
                                 size_t *pages_total, size_t *pages_found_pass4, size_t *pages_pending,
                                 bool lookup_pending_in_open_cache) {

    Word_t metric_id = mrg_metric_id(main_mrg, metric);

    bool first = true;
    Word_t this_page_start_time = 0;
    Pvoid_t *PValue;
    size_t query_gaps = 0;

    *first_page_starting_time_s = INVALID_TIME;
    *pages_pending = 0;
    *pages_total = 0;

    time_t previous_page_update_every_s = mrg_metric_get_update_every(main_mrg, metric);

    if(!previous_page_update_every_s)
        previous_page_update_every_s = default_rrd_update_every;

    time_t previous_page_last_time_s = wanted_start_time_s - previous_page_update_every_s;

    while((PValue = JudyLFirstThenNext(JudyL_page_array, &this_page_start_time, &first))) {
        struct page_details *pd = *PValue;

        if(unlikely(*first_page_starting_time_s == INVALID_TIME || (time_t)this_page_start_time < *first_page_starting_time_s))
            *first_page_starting_time_s = (time_t)this_page_start_time;

        if(!pd->page) {
            if(lookup_pending_in_open_cache)
                pd->page = pgc_page_get_and_acquire(main_cache, (Word_t) ctx, (Word_t) metric_id, pd->first_time_s, true);

            if(pd->page) {
                (*pages_found_pass4)++;
                pd->status &= ~PDC_PAGE_DISK_PENDING;
                pd->status |= PDC_PAGE_READY | PDC_PAGE_PRELOADED | PDC_PAGE_PRELOADED_PASS4;
            }
            else {
                (*pages_pending)++;
                pd->status |= PDC_PAGE_DISK_PENDING;

                internal_fatal(!pd->datafile.ptr, "datafile is NULL");
                internal_fatal(!pd->datafile.extent.bytes, "datafile.extent.bytes zero");
                internal_fatal(!pd->datafile.extent.pos, "datafile.extent.pos is zero");
                internal_fatal(!pd->datafile.fileno, "datafile.fileno is zero");
            }

            internal_fatal(pd->metric_id != metric_id, "pd has wrong metric_id");
        }
        else {
            pd->status &= ~PDC_PAGE_DISK_PENDING;
            pd->status |= (PDC_PAGE_READY | PDC_PAGE_PRELOADED);
        }

        if(previous_page_last_time_s + previous_page_update_every_s < pd->first_time_s)
            query_gaps++;

        previous_page_last_time_s = pd->last_time_s;
        previous_page_update_every_s = (pd->update_every_s) ? pd->update_every_s : previous_page_update_every_s;

        (*pages_total)++;
    }

    if(previous_page_last_time_s < wanted_end_time_s)
        query_gaps++;

    return query_gaps;
}

#define time_delta(finish, pass) do { if(pass) { usec_t t = pass; (pass) = (finish) - (pass); (finish) = t; } } while(0)

// Return a judyL will all pages that have start_time_ut and end_time_ut
// Pvalue of the judy will be the end time for that page
// DBENGINE2:
Pvoid_t get_page_list(struct rrdengine_instance *ctx, METRIC *metric, usec_t start_time_ut, usec_t end_time_ut, time_t *first_page_first_time_s, size_t *pages_to_load) {
    Pvoid_t JudyL_page_array = (Pvoid_t) NULL;

    uuid_t *uuid = mrg_metric_uuid(main_mrg, metric);
    Word_t metric_id = mrg_metric_id(main_mrg, metric);
    time_t wanted_start_time_s = (time_t)(start_time_ut / USEC_PER_SEC);
    time_t wanted_end_time_s = (time_t)(end_time_ut / USEC_PER_SEC);

    size_t pages_found_in_cache = 0, pages_found_in_open = 0, pages_found_in_journals_v2 = 0, pages_found_pass4 = 0, pages_pending = 0, pages_total = 0;
    size_t cache_gaps = 0, query_gaps = 0;
    bool done_v2 = false, done_open = false;

    time_t first_page_starting_time_s = INVALID_TIME;

    usec_t pass1_ut = 0, pass2_ut = 0, pass3_ut = 0, pass4_ut = 0;

    // --------------------------------------------------------------
    // PASS 1: Check what the main page cache has available

    pass1_ut = now_monotonic_usec();
    size_t pages_pass1 = get_page_list_from_pgc(main_cache, metric, ctx, wanted_start_time_s, wanted_end_time_s,
                                                &JudyL_page_array, &cache_gaps,
                                                &first_page_starting_time_s, false,
                                                PDC_PAGE_PRELOADED_PASS1 | PDC_PAGE_SOURCE_MAIN_CACHE);
    query_gaps += cache_gaps;
    pages_found_in_cache += pages_pass1;
    pages_total += pages_pass1;

    if(pages_found_in_cache && !cache_gaps)
        goto we_are_done;


    // --------------------------------------------------------------
    // PASS 2: Check what the open journal page cache has available
    //         these will be loaded from disk

    pass2_ut = now_monotonic_usec();
    size_t pages_pass2 = get_page_list_from_pgc(open_cache, metric, ctx, wanted_start_time_s, wanted_end_time_s,
                                                &JudyL_page_array, &cache_gaps,
                                                &first_page_starting_time_s, true,
                                                PDC_PAGE_SOURCE_OPEN_CACHE);
    query_gaps += cache_gaps;
    pages_found_in_open += pages_pass2;
    pages_total += pages_pass2;
    done_open = true;

    // --------------------------------------------------------------
    // PASS 3: Check Journal v2 to fill the gaps

    pass3_ut = now_monotonic_usec();
    uv_rwlock_rdlock(&ctx->datafiles.rwlock);
    struct rrdengine_datafile *datafile = ctx->datafiles.first;
    bool lookup_continue = true;
    while (lookup_continue && datafile) {
        struct journal_v2_header *journal_header = (struct journal_v2_header *) GET_JOURNAL_DATA(datafile->journalfile);
        if (!journal_header || !datafile->users.available) {
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
                size_t page_length = page_entry_in_journal->page_length;

                if(is_page_in_time_range(page_first_time_s, page_last_time_s, wanted_start_time_s, wanted_end_time_s)) {

                    if (first_page_starting_time_s == INVALID_TIME || page_first_time_s < first_page_starting_time_s)
                        first_page_starting_time_s = page_first_time_s;

                    Pvoid_t *PValue = JudyLIns(&JudyL_page_array, page_first_time_s, PJE0);
                    if (!PValue || PValue == PJERR)
                        fatal("DBENGINE: corrupted judy array");

                    if (unlikely(*PValue)) {
                        // it is already in the judy

                        struct page_details *pd = *PValue; (void)pd;
//                        internal_error(
//                                pd->first_time_s != page_first_time_s ||
//                                pd->last_time_s != page_last_time_s ||
//                                pd->update_every_s != page_update_every_s,
//                                "DBENGINE: duplicate page with different retention in journal v2 "
//                                "1st: %ld to %ld, ue %u, size %u "
//                                "2nd: %ld to %ld, ue %ld size %zu "
//                                "- ignoring the second",
//                                pd->first_time_s, pd->last_time_s, pd->update_every_s, pd->page_length,
//                                page_first_time_s, page_last_time_s, page_update_every_s, page_length);
                    }
                    else {
                        if(datafile_acquire(datafile)) {
                            struct page_details *pd = callocz(1, sizeof(*pd));
                            pd->datafile.extent.pos = extent_list[page_entry_in_journal->extent_index].datafile_offset;
                            pd->datafile.extent.bytes = extent_list[page_entry_in_journal->extent_index].datafile_size;
                            pd->datafile.file = datafile->file;
                            pd->datafile.fileno = datafile->fileno;
                            pd->first_time_s = page_first_time_s;
                            pd->last_time_s = page_last_time_s;
                            pd->datafile.ptr = datafile;
                            pd->page_length = page_length;
                            pd->update_every_s = page_update_every_s;
                            pd->type = page_entry_in_journal->type;
                            pd->metric_id = metric_id;
                            pd->status |= PDC_PAGE_DISK_PENDING | PDC_PAGE_SOURCE_JOURNAL_V2 | PDC_PAGE_DATAFILE_ACQUIRED;
                            uuid_copy(pd->datafile.extent.page_uuid, *uuid);
                            *PValue = pd;

                            pages_found_in_journals_v2++;
                            pages_total++;
                        }
                        else {
                            // we cannot add it
                            if(!JudyLDel(&JudyL_page_array, page_first_time_s, PJE0))
                                fatal("DBENGINE: cannot delete page from routing JudyL");
                        }
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
    //         and calculate the time gaps in the query
    //         THIS IS REQUIRED AFTER JOURNAL V2 LOOKUP

    pass4_ut = now_monotonic_usec();
    query_gaps = list_has_time_gaps(ctx, metric, JudyL_page_array, wanted_start_time_s, wanted_end_time_s,
                                    &first_page_starting_time_s, &pages_total, &pages_found_pass4, &pages_pending,
                                    true);

we_are_done:

    if(first_page_first_time_s)
        *first_page_first_time_s = first_page_starting_time_s;

    if(pages_to_load)
        *pages_to_load = pages_pending;

    usec_t finish_ut = now_monotonic_usec();
    time_delta(finish_ut, pass4_ut);
    time_delta(finish_ut, pass3_ut);
    time_delta(finish_ut, pass2_ut);
    time_delta(finish_ut, pass1_ut);
    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.time_in_main_cache_lookup, pass1_ut, __ATOMIC_RELAXED);
    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.time_in_open_cache_lookup, pass2_ut, __ATOMIC_RELAXED);
    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.time_in_journal_v2_lookup, pass3_ut, __ATOMIC_RELAXED);
    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.time_in_pass4_lookup, pass4_ut, __ATOMIC_RELAXED);

    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.queries, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.queries_with_gaps, (query_gaps)?1:0, __ATOMIC_RELAXED);
    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.queries_open, done_open ? 1 : 0, __ATOMIC_RELAXED);
    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.queries_journal_v2, done_v2 ? 1 : 0, __ATOMIC_RELAXED);
    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_total, pages_total, __ATOMIC_RELAXED);
    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_found_in_cache, pages_found_in_cache, __ATOMIC_RELAXED);
    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_found_in_open, pages_found_in_open, __ATOMIC_RELAXED);
    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_found_in_jv2, pages_found_in_journals_v2, __ATOMIC_RELAXED);
    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_pending_found_in_cache_at_pass4, pages_found_pass4, __ATOMIC_RELAXED);
    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_to_load_from_disk, pages_pending, __ATOMIC_RELAXED);

    return JudyL_page_array;
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

    handle->pdc = callocz(1, sizeof(struct page_details_control));
    netdata_spinlock_init(&handle->pdc->refcount_spinlock);
    completion_init(&handle->pdc->completion);

    handle->pdc->page_list_JudyL = get_page_list(ctx, handle->metric,
                                     start_time_t * USEC_PER_SEC, end_time_t * USEC_PER_SEC,
                                                 &first_page_first_time_s, &pages_to_load);

    if (pages_to_load && handle->pdc->page_list_JudyL) {
        handle->pdc->refcount = 2; // we get 1 for us and 1 for the 1st worker in the chain: do_read_page_list_work()
        handle->pdc->preload_all_extent_pages = false;
        usec_t start_ut = now_monotonic_usec();
        dbengine_load_page_list(ctx, handle->pdc);
        //dbengine_load_page_list_directly(ctx, handle->pdc);
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.time_to_route, now_monotonic_usec() - start_ut, __ATOMIC_RELAXED);
    }
    else {
        handle->pdc->refcount = 1; // we are alone in this query - no need for any worker
        completion_mark_complete(&handle->pdc->completion);
    }

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
    if (unlikely(!handle || !handle->pdc || !handle->pdc->page_list_JudyL)) {

        if(next_page_start_time_s)
            *next_page_start_time_s = INVALID_TIME;

        return NULL;
    }

    usec_t start_ut = now_monotonic_usec();

    bool waited = false;

    // Caller will request the next page which will be end_time + update_every so search inclusive from Index
    PGC_PAGE *page = NULL;
    struct page_details *pd;
    Word_t Index = start_time_t;
    Pvoid_t *PValue = JudyLFirst(handle->pdc->page_list_JudyL, &Index, PJE0);
    if (!PValue || !*PValue) {

        if(next_page_start_time_s)
            *next_page_start_time_s = INVALID_TIME;

        return NULL;
    }
    pd = *PValue;

    bool done = (pd->page != NULL);
    while(!done && !pdc_page_status_check(pd, PDC_PAGE_READY | PDC_PAGE_FAILED)) {

        if(!completion_is_done(&handle->pdc->completion)) {
            handle->pdc->completed_jobs =
                    completion_wait_for_a_job(&handle->pdc->completion, handle->pdc->completed_jobs);

            waited = true;
        }
        else
            done = true;
    }

    page = pd->page;
    if(page) {
        // this is for pdc_destroy() to not release the page again
        pdc_page_status_set(pd, PDC_PAGE_RELEASED | PDC_PAGE_PROCESSED);

        if(waited)
            __atomic_add_fetch(&rrdeng_cache_efficiency_stats.page_next_wait_loaded, 1, __ATOMIC_RELAXED);
        else
            __atomic_add_fetch(&rrdeng_cache_efficiency_stats.page_next_nowait_loaded, 1, __ATOMIC_RELAXED);
    }
    else {
        if(waited)
            __atomic_add_fetch(&rrdeng_cache_efficiency_stats.page_next_wait_failed, 1, __ATOMIC_RELAXED);
        else
            __atomic_add_fetch(&rrdeng_cache_efficiency_stats.page_next_nowait_failed, 1, __ATOMIC_RELAXED);
    }

    if(next_page_start_time_s) {
        Pvoid_t *PValue = JudyLNext(handle->pdc->page_list_JudyL, &Index, PJE0);
        if(!PValue || !*PValue)
            *next_page_start_time_s = INVALID_TIME;
        else
            *next_page_start_time_s = (time_t)Index;
    }

    if(waited) {
        if(pd->status & PDC_PAGE_PRELOADED)
            __atomic_add_fetch(&rrdeng_cache_efficiency_stats.time_to_slow_preload_next_page, now_monotonic_usec() - start_ut, __ATOMIC_RELAXED);
        else
            __atomic_add_fetch(&rrdeng_cache_efficiency_stats.time_to_slow_disk_next_page, now_monotonic_usec() - start_ut, __ATOMIC_RELAXED);
    }
    else {
        if(pd->status & PDC_PAGE_PRELOADED)
            __atomic_add_fetch(&rrdeng_cache_efficiency_stats.time_to_fast_preload_next_page, now_monotonic_usec() - start_ut, __ATOMIC_RELAXED);
        else
            __atomic_add_fetch(&rrdeng_cache_efficiency_stats.time_to_fast_disk_next_page, now_monotonic_usec() - start_ut, __ATOMIC_RELAXED);
    }

    return page;
}

void pgc_open_add_hot_page(Word_t section, Word_t metric_id, time_t start_time_s, time_t end_time_s, time_t update_every_s, struct rrdengine_datafile *datafile, uint64_t extent_offset, unsigned extent_size) {

    if(!datafile_acquire(datafile))
        fatal("DBENGINE: cannot acquire datafile to put page in open cache");

    struct extent_io_data ext_io_data = {
            .file  = datafile->file,
            .fileno = datafile->fileno,
            .pos = extent_offset,
            .bytes = extent_size,
    };

    PGC_ENTRY page_entry = {
            .hot = true,
            .section = section,
            .metric_id = metric_id,
            .start_time_t = start_time_s,
            .end_time_t =  end_time_s,
            .update_every = update_every_s,
            .size = 0,
            .data = datafile,
            .custom_data = (uint8_t *) &ext_io_data,
    };

    internal_fatal(!datafile->fileno, "DBENGINE: datafile supplied does not have a number");

    bool added = true;
    PGC_PAGE *page = pgc_page_add_and_acquire(open_cache, page_entry, &added);
    int tries = 100;
    while(!added && page_entry.end_time_t > pgc_page_end_time_t(page) && tries--) {
        pgc_page_hot_to_clean_empty_and_release(open_cache, page);
        page = pgc_page_add_and_acquire(open_cache, page_entry, &added);
    }

    if(!added) {
        datafile_release(datafile);

        internal_fatal(page_entry.end_time_t > pgc_page_end_time_t(page),
                       "DBENGINE: cannot add longer page to open cache");
    }

    pgc_page_release(open_cache, (PGC_PAGE *)page);
}

void init_page_cache(void)
{
    static SPINLOCK spinlock = NETDATA_SPINLOCK_INITIALIZER;
    static bool initialized = false;

    netdata_spinlock_lock(&spinlock);
    if (!initialized) {
        initialized = true;

        main_mrg = mrg_create();

        main_cache = pgc_create(
                (size_t) default_rrdeng_page_cache_mb * 1024 * 1024,
                main_cache_free_clean_page_callback,
                (size_t) rrdeng_pages_per_extent,
                main_cache_flush_dirty_page_callback,
                2,                                 //
                1000,                           //
                1,                                          // don't delay too much other threads
                PGC_OPTIONS_AUTOSCALE,                               // AUTOSCALE = 2x max hot pages
                0,                                                 // 0 = as many as the system cpus
                0
        );

        open_cache = pgc_create(
                0,                                          // the default is 1MB
                open_cache_free_clean_page_callback,
                1,
                open_cache_flush_dirty_page_callback,
                2,                                //
                1000,                           //
                1,                                          // don't delay too much other threads
                PGC_OPTIONS_AUTOSCALE | PGC_OPTIONS_EVICT_PAGES_INLINE | PGC_OPTIONS_FLUSH_PAGES_INLINE,
                0,                                                 // 0 = as many as the system cpus
                sizeof(struct extent_io_data)
        );

        extent_cache = pgc_create(
                10 * 1024 * 1024,
                extent_cache_free_clean_page_callback,
                1,
                extent_cache_flush_dirty_page_callback,
                2,                                  //
                100,                            //
                1,                                          // don't delay too much other threads
                PGC_OPTIONS_AUTOSCALE | PGC_OPTIONS_EVICT_PAGES_INLINE | PGC_OPTIONS_FLUSH_PAGES_INLINE,
                0,                                                 // 0 = as many as the system cpus
                0
        );
    }
    netdata_spinlock_unlock(&spinlock);

}

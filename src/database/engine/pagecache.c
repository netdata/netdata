// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdengine.h"

MRG *main_mrg = NULL;
PGC *main_cache = NULL;
PGC *open_cache = NULL;
PGC *extent_cache = NULL;
struct rrdeng_cache_efficiency_stats rrdeng_cache_efficiency_stats = {};

static void main_cache_free_clean_page_callback(PGC *cache __maybe_unused, PGC_ENTRY entry __maybe_unused)
{
    // Release storage associated with the page
    pgd_free(entry.data);
}

static void main_cache_flush_dirty_page_init_callback(PGC *cache __maybe_unused, Word_t section) {
    struct rrdengine_instance *ctx = (struct rrdengine_instance *) section;

    // mark ctx as having flushing in progress
    __atomic_add_fetch(&ctx->atomic.extents_currently_being_flushed, 1, __ATOMIC_RELAXED);
}

static void main_cache_flush_dirty_page_callback(PGC *cache __maybe_unused, PGC_ENTRY *entries_array __maybe_unused, PGC_PAGE **pages_array __maybe_unused, size_t entries __maybe_unused)
{
    if(!entries)
        return;

     struct rrdengine_instance *ctx = (struct rrdengine_instance *) entries_array[0].section;

    struct page_descr_with_data *base = NULL;

    for (size_t Index = 0 ; Index < entries; Index++) {
        time_t start_time_s = entries_array[Index].start_time_s;
        time_t end_time_s = entries_array[Index].end_time_s;
        struct page_descr_with_data *descr = page_descriptor_get();

        descr->id = mrg_metric_uuid(main_mrg, (METRIC *) entries_array[Index].metric_id);
        descr->metric_id = entries_array[Index].metric_id;
        descr->start_time_ut = start_time_s * USEC_PER_SEC;
        descr->end_time_ut = end_time_s * USEC_PER_SEC;
        descr->update_every_s = entries_array[Index].update_every_s;

        descr->pgd = pgc_page_data(pages_array[Index]);
        descr->type = pgd_type(descr->pgd);
        descr->page_length = pgd_disk_footprint(descr->pgd);

        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(base, descr, link.prev, link.next);
    }

    struct completion completion;
    completion_init(&completion);
    rrdeng_enq_cmd(ctx, RRDENG_OPCODE_EXTENT_WRITE, base, &completion, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);
    completion_wait_for(&completion);
    completion_destroy(&completion);
}

static void open_cache_free_clean_page_callback(PGC *cache __maybe_unused, PGC_ENTRY entry __maybe_unused)
{
    struct rrdengine_datafile *datafile = entry.data;
    datafile_release(datafile, DATAFILE_ACQUIRE_OPEN_CACHE);
    timing_dbengine_evict_step(TIMING_STEP_DBENGINE_EVICT_FREE_OPEN);
}

static void open_cache_flush_dirty_page_callback(PGC *cache __maybe_unused, PGC_ENTRY *entries_array __maybe_unused, PGC_PAGE **pages_array __maybe_unused, size_t entries __maybe_unused)
{
    ;
}

static void extent_cache_free_clean_page_callback(PGC *cache __maybe_unused, PGC_ENTRY entry __maybe_unused)
{
    dbengine_extent_free(entry.data, entry.size);
    timing_dbengine_evict_step(TIMING_STEP_DBENGINE_EVICT_FREE_EXTENT);
}

static void extent_cache_flush_dirty_page_callback(PGC *cache __maybe_unused, PGC_ENTRY *entries_array __maybe_unused, PGC_PAGE **pages_array __maybe_unused, size_t entries __maybe_unused)
{
    ;
}

ALWAYS_INLINE_HOT TIME_RANGE_COMPARE is_page_in_time_range(time_t page_first_time_s, time_t page_last_time_s, time_t wanted_start_time_s, time_t wanted_end_time_s) {
    // page_first_time_s <= wanted_end_time_s && page_last_time_s >= wanted_start_time_s

    if(page_last_time_s < wanted_start_time_s)
        return PAGE_IS_IN_THE_PAST;

    if(page_first_time_s > wanted_end_time_s)
        return PAGE_IS_IN_THE_FUTURE;

    return PAGE_IS_IN_RANGE;
}

static ALWAYS_INLINE_HOT struct page_details *pdc_find_page_for_time(
        Pcvoid_t PArray,
        time_t wanted_time_s,
        size_t *gaps,
        PDC_PAGE_STATUS mode,
        PDC_PAGE_STATUS skip_list
) {
    Word_t PIndexF = wanted_time_s, PIndexL = wanted_time_s;
    Pvoid_t *PValueF, *PValueL;
    struct page_details *pdF = NULL, *pdL = NULL;
    bool firstF = true, firstL = true;

    PDC_PAGE_STATUS ignore_list = PDC_PAGE_QUERY_GLOBAL_SKIP_LIST | skip_list;

    while ((PValueF = PDCJudyLFirstThenNext(PArray, &PIndexF, &firstF))) {
        pdF = *PValueF;

        PDC_PAGE_STATUS status = __atomic_load_n(&pdF->status, __ATOMIC_ACQUIRE);
        if (!(status & (ignore_list | mode)))
            break;

        pdF = NULL;
    }

    while ((PValueL = PDCJudyLLastThenPrev(PArray, &PIndexL, &firstL))) {
        pdL = *PValueL;

        PDC_PAGE_STATUS status = __atomic_load_n(&pdL->status, __ATOMIC_ACQUIRE);
        if(status & mode) {
            // don't go all the way back to the beginning
            // stop at the last processed
            pdL = NULL;
            break;
        }

        if (!(status & ignore_list))
            break;

        pdL = NULL;
    }

    TIME_RANGE_COMPARE rcF = (pdF) ? is_page_in_time_range(pdF->first_time_s, pdF->last_time_s, wanted_time_s, wanted_time_s) : PAGE_IS_IN_THE_FUTURE;
    TIME_RANGE_COMPARE rcL = (pdL) ? is_page_in_time_range(pdL->first_time_s, pdL->last_time_s, wanted_time_s, wanted_time_s) : PAGE_IS_IN_THE_PAST;

    if (!pdF || pdF == pdL) {
        // F is missing, or they are the same
        // return L
        (*gaps) += (rcL == PAGE_IS_IN_RANGE) ? 0 : 1;
        return pdL;
    }

    if (!pdL) {
        // L is missing
        // return F
        (*gaps) += (rcF == PAGE_IS_IN_RANGE) ? 0 : 1;
        return pdF;
    }

    if (rcF == rcL) {
        // both are on the same side,
        // but they are different pages

        switch (rcF) {
            case PAGE_IS_IN_RANGE:
                // pick the higher resolution
                if (pdF->update_every_s && pdF->update_every_s < pdL->update_every_s)
                    return pdF;

                if (pdL->update_every_s && pdL->update_every_s < pdF->update_every_s)
                    return pdL;

                // same resolution - pick the one that starts earlier
                if (pdL->first_time_s < pdF->first_time_s)
                    return pdL;

                return pdF;
                break;

            case PAGE_IS_IN_THE_FUTURE:
                (*gaps)++;

                // pick the one that starts earlier
                if (pdL->first_time_s < pdF->first_time_s)
                    return pdL;

                return pdF;
                break;

            default:
            case PAGE_IS_IN_THE_PAST:
                (*gaps)++;
                return NULL;
                break;
        }
    }

    if(rcF == PAGE_IS_IN_RANGE) {
        // (*gaps) += 0;
        return pdF;
    }

    if(rcL == PAGE_IS_IN_RANGE) {
        // (*gaps) += 0;
        return pdL;
    }

    if(rcF == PAGE_IS_IN_THE_FUTURE) {
        (*gaps)++;
        return pdF;
    }

    if(rcL == PAGE_IS_IN_THE_FUTURE) {
        (*gaps)++;
        return pdL;
    }

    // impossible case
    (*gaps)++;
    return NULL;
}

static ALWAYS_INLINE_HOT size_t get_page_list_from_pgc(PGC *cache, METRIC *metric, struct rrdengine_instance *ctx,
        time_t wanted_start_time_s, time_t wanted_end_time_s,
        Pvoid_t *JudyL_page_array, size_t *cache_gaps,
        bool open_cache_mode, PDC_PAGE_STATUS tags) {

    size_t pages_found_in_cache = 0;
    Word_t metric_id = mrg_metric_id(main_mrg, metric);

    time_t now_s = wanted_start_time_s;
    uint32_t dt_s = mrg_metric_get_update_every_s(main_mrg, metric);

    if(!dt_s)
        dt_s = nd_profile.update_every;

    time_t previous_page_end_time_s = now_s - dt_s;
    bool first = true;

    do {
        PGC_PAGE *page = pgc_page_get_and_acquire(
                cache, (Word_t)ctx, (Word_t)metric_id, now_s,
                (first) ? PGC_SEARCH_CLOSEST : PGC_SEARCH_NEXT);

        first = false;

        if(!page) {
            if(previous_page_end_time_s < wanted_end_time_s)
                (*cache_gaps)++;

            break;
        }

        time_t page_start_time_s = pgc_page_start_time_s(page);
        time_t page_end_time_s = pgc_page_end_time_s(page);
        uint32_t page_update_every_s = pgc_page_update_every_s(page);

        if(!page_update_every_s)
            page_update_every_s = dt_s;

        if(is_page_in_time_range(page_start_time_s, page_end_time_s, wanted_start_time_s, wanted_end_time_s) != PAGE_IS_IN_RANGE) {
            // not a useful page for this query
            pgc_page_release(cache, page);
            page = NULL;

            if(previous_page_end_time_s < wanted_end_time_s)
                (*cache_gaps)++;

            break;
        }

        if (page_start_time_s - previous_page_end_time_s > (time_t)dt_s)
            (*cache_gaps)++;

        Pvoid_t *PValue = PDCJudyLIns(JudyL_page_array, (Word_t) page_start_time_s, PJE0);
        if (!PValue || PValue == PJERR)
            fatal("DBENGINE: corrupted judy array in %s()", __FUNCTION__ );

        if (unlikely(*PValue))
            // already exists in our list
            pgc_page_release(cache, page);

        else {

            internal_fatal(pgc_page_metric(page) != metric_id, "Wrong metric id in page found in cache");
            internal_fatal(pgc_page_section(page) != (Word_t)ctx, "Wrong section in page found in cache");

            struct page_details *pd = page_details_get();
            pd->metric_id = metric_id;
            pd->first_time_s = page_start_time_s;
            pd->last_time_s = page_end_time_s;
            pd->update_every_s = page_update_every_s;
            pd->page = (open_cache_mode) ? NULL : page;
            pd->status |= tags;

            if((pd->page)) {
                pd->status |= PDC_PAGE_READY | PDC_PAGE_PRELOADED;

                if(pgd_is_empty(pgc_page_data(page)))
                    pd->status |= PDC_PAGE_EMPTY;
            }

            if(open_cache_mode) {
                struct rrdengine_datafile *datafile = pgc_page_data(page);
                if(datafile_acquire(datafile, DATAFILE_ACQUIRE_PAGE_DETAILS)) { // for pd
                    struct extent_io_data *xio = (struct extent_io_data *) pgc_page_custom_data(cache, page);
                    pd->datafile.ptr = pgc_page_data(page);
                    pd->datafile.block = xio->block;
                    pd->datafile.bytes = xio->bytes;
                    pd->status |= PDC_PAGE_DATAFILE_ACQUIRED | PDC_PAGE_DISK_PENDING;
                }
                else {
                    pd->status |= PDC_PAGE_FAILED | PDC_PAGE_FAILED_TO_ACQUIRE_DATAFILE;
                }
                pgc_page_release(cache, page);
            }

            *PValue = pd;

            pages_found_in_cache++;
        }

        // prepare for the next iteration
        previous_page_end_time_s = page_end_time_s;

        if(page_update_every_s > 0)
            dt_s = page_update_every_s;

        // we are going to as for the NEXT page
        // so, set this to our first time
        now_s = page_start_time_s;

    } while(now_s <= wanted_end_time_s);

    return pages_found_in_cache;
}

static void pgc_inject_gap(struct rrdengine_instance *ctx, METRIC *metric, time_t start_time_s, time_t end_time_s) {

    time_t db_first_time_s, db_last_time_s;
    mrg_metric_get_retention(main_mrg, metric, &db_first_time_s, &db_last_time_s, NULL);

    if(is_page_in_time_range(start_time_s, end_time_s, db_first_time_s, db_last_time_s) != PAGE_IS_IN_RANGE)
        return;

    PGC_ENTRY page_entry = {
            .hot = false,
            .section = (Word_t)ctx,
            .metric_id = (Word_t)metric,
            .start_time_s = MAX(start_time_s, db_first_time_s),
            .end_time_s = MIN(end_time_s, db_last_time_s),
            .update_every_s = 0,
            .size = 0,
            .data = PGD_EMPTY,
    };

    if(page_entry.start_time_s >= page_entry.end_time_s)
        return;

    PGC_PAGE *page = pgc_page_add_and_acquire(main_cache, page_entry, NULL);
    pgc_page_release(main_cache, page);
}

static ALWAYS_INLINE_HOT size_t list_has_time_gaps(
        struct rrdengine_instance *ctx,
        METRIC *metric,
        Pvoid_t JudyL_page_array,
        time_t wanted_start_time_s,
        time_t wanted_end_time_s,
        size_t *pages_total,
        size_t *pages_found_pass4,
        size_t *pages_to_load_from_disk,
        size_t *pages_overlapping,
        time_t *optimal_end_time_s,
        bool populate_gaps,
        PDC_PAGE_STATUS *common_status
) {
    // we will recalculate these, so zero them
    *pages_to_load_from_disk = 0;
    *pages_overlapping = 0;
    *optimal_end_time_s = 0;
    *common_status = 0;

    bool first;
    Pvoid_t *PValue;
    Word_t this_page_start_time;
    struct page_details *pd;

    size_t gaps = 0;
    Word_t metric_id = mrg_metric_id(main_mrg, metric);

    // ------------------------------------------------------------------------
    // PASS 1: remove the preprocessing flags from the pages in PDC

    first = true;
    this_page_start_time = 0;
    while((PValue = PDCJudyLFirstThenNext(JudyL_page_array, &this_page_start_time, &first))) {
        pd = *PValue;
        pd->status &= ~(PDC_PAGE_SKIP|PDC_PAGE_PREPROCESSED);
    }

    // ------------------------------------------------------------------------
    // PASS 2: emulate processing to find the useful pages

    time_t now_s = wanted_start_time_s;
    time_t dt_s = mrg_metric_get_update_every_s(main_mrg, metric);
    if(!dt_s)
        dt_s = nd_profile.update_every;

    size_t pages_pass2 = 0, pages_pass3 = 0;
    while((pd = pdc_find_page_for_time(
            JudyL_page_array, now_s, &gaps,
            PDC_PAGE_PREPROCESSED, 0))) {

        pd->status |= PDC_PAGE_PREPROCESSED;
        pages_pass2++;

        if(pd->update_every_s)
            dt_s = pd->update_every_s;

        if(populate_gaps && pd->first_time_s > now_s)
            pgc_inject_gap(ctx, metric, now_s, pd->first_time_s);

        now_s = pd->last_time_s + dt_s;
        if(now_s > wanted_end_time_s) {
            *optimal_end_time_s = pd->last_time_s;
            break;
        }
    }

    if(populate_gaps && now_s < wanted_end_time_s)
        pgc_inject_gap(ctx, metric, now_s, wanted_end_time_s);

    // ------------------------------------------------------------------------
    // PASS 3: mark as skipped all the pages not useful

    first = true;
    this_page_start_time = 0;
    while((PValue = PDCJudyLFirstThenNext(JudyL_page_array, &this_page_start_time, &first))) {
        pd = *PValue;

        internal_fatal(pd->metric_id != metric_id, "pd has wrong metric_id");

        if(!(pd->status & PDC_PAGE_PREPROCESSED)) {
            (*pages_overlapping)++;
            pd->status |= PDC_PAGE_SKIP;
            pd->status &= ~(PDC_PAGE_READY | PDC_PAGE_DISK_PENDING);
            *common_status |= pd->status;
            continue;
        }

        pages_pass3++;

        if(!pd->page) {
            pd->page = pgc_page_get_and_acquire(main_cache, (Word_t) ctx, (Word_t) metric_id, pd->first_time_s, PGC_SEARCH_EXACT);

            if(pd->page) {
                (*pages_found_pass4)++;

                pd->status &= ~PDC_PAGE_DISK_PENDING;
                pd->status |= PDC_PAGE_READY | PDC_PAGE_PRELOADED | PDC_PAGE_PRELOADED_PASS4;

                if(pgd_is_empty(pgc_page_data(pd->page)))
                    pd->status |= PDC_PAGE_EMPTY;

            }
            else if(!(pd->status & PDC_PAGE_FAILED) && (pd->status & PDC_PAGE_DATAFILE_ACQUIRED)) {
                (*pages_to_load_from_disk)++;

                pd->status |= PDC_PAGE_DISK_PENDING;

                internal_fatal(pd->status & PDC_PAGE_SKIP, "page is disk pending and skipped");
                internal_fatal(!pd->datafile.ptr, "datafile is NULL");
                internal_fatal(!pd->datafile.extent.bytes, "datafile.extent.bytes zero");
                internal_fatal(!pd->datafile.extent.pos, "datafile.extent.pos is zero");
                internal_fatal(!pd->datafile.fileno, "datafile.fileno is zero");
            }
        }
        else {
            pd->status &= ~PDC_PAGE_DISK_PENDING;
            pd->status |= (PDC_PAGE_READY | PDC_PAGE_PRELOADED);
        }

        *common_status |= pd->status;
    }

    internal_fatal(pages_pass2 != pages_pass3,
                   "DBENGINE: page count does not match");

    *pages_total = pages_pass2;

    return gaps;
}

// ----------------------------------------------------------------------------

typedef void (*page_found_callback_t)(PGC_PAGE *page, void *data);
static NOT_INLINE_HOT size_t get_page_list_from_journal_v2(struct rrdengine_instance *ctx, METRIC *metric, usec_t start_time_ut, usec_t end_time_ut, page_found_callback_t callback, void *callback_data) {
    nd_uuid_t *uuid = mrg_metric_uuid(main_mrg, metric);
    Word_t metric_id = mrg_metric_id(main_mrg, metric);

    time_t wanted_start_time_s = (time_t)(start_time_ut / USEC_PER_SEC);
    time_t wanted_end_time_s = (time_t)(end_time_ut / USEC_PER_SEC);

    size_t pages_found = 0;

    NJFV2IDX_FIND_STATE state = {
            .init = false,
            .last = 0,
            .ctx = ctx,
            .wanted_start_time_s = wanted_start_time_s,
            .wanted_end_time_s = wanted_end_time_s,
            .j2_header_acquired = NULL,
    };

    struct rrdengine_datafile *datafile;
    while((datafile = njfv2idx_find_and_acquire_j2_header(&state))) {
        struct journal_v2_header *j2_header = state.j2_header_acquired;

        if (unlikely(!j2_header))
            continue;

        char file_path[RRDENG_PATH_MAX];
        journalfile_v2_generate_path(datafile, file_path, sizeof(file_path));
        PROTECTED_ACCESS_SETUP(datafile->journalfile->mmap.data, datafile->journalfile->mmap.size, file_path, "read");
        if(no_signal_received) {
            time_t journal_start_time_s = (time_t)(j2_header->start_time_ut / USEC_PER_SEC);
            size_t journal_v2_file_size = datafile->journalfile->mmap.size;

            // the datafile possibly contains useful data for this query

            size_t journal_metric_count = (size_t)j2_header->metric_count;
            struct journal_metric_list *uuid_list =
                (struct journal_metric_list *)((uint8_t *)j2_header + j2_header->metric_offset);
            size_t metric_offset = (uint8_t *)uuid_list - (uint8_t *)j2_header;
            if (metric_offset >= journal_v2_file_size) {
                nd_log_limit_static_thread_var(erl, 60, 0);
                nd_log_limit(&erl, NDLS_DAEMON, NDLP_ERR,
                             "DBENGINE: Invalid metric list header in journalfile %u of tier %u",
                             datafile->fileno, datafile->tier);
                journalfile_v2_data_release(datafile->journalfile);
                continue;
            }

            struct journal_metric_list *uuid_entry =
                bsearch(uuid, uuid_list, journal_metric_count, sizeof(*uuid_list), journal_metric_uuid_compare);

            if (unlikely(!uuid_entry)) {
                // our UUID is not in this datafile
                journalfile_v2_data_release(datafile->journalfile);
                continue;
            }

            struct journal_page_header *page_list_header =
                (struct journal_page_header *)((uint8_t *)j2_header + uuid_entry->page_offset);
            size_t page_offset = (uint8_t *)page_list_header - (uint8_t *)j2_header;
            if (page_offset >= journal_v2_file_size) {
                nd_log_limit_static_thread_var(erl, 60, 0);
                nd_log_limit(&erl, NDLS_DAEMON, NDLP_ERR,
                             "DBENGINE: Invalid page list header in journalfile %u of tier %u",
                             datafile->fileno, datafile->tier);
                journalfile_v2_data_release(datafile->journalfile);
                continue;
            }

            struct journal_page_list *page_list =
                (struct journal_page_list *)((uint8_t *)page_list_header + sizeof(*page_list_header));
            struct journal_extent_list *extent_list = (void *)((uint8_t *)j2_header + j2_header->extent_offset);
            uint32_t extent_entries = j2_header->extent_count;
            uint32_t uuid_page_entries = page_list_header->entries;

            for (uint32_t index = 0; index < uuid_page_entries; index++) {
                struct journal_page_list *page_entry_in_journal = &page_list[index];

                time_t page_first_time_s = page_entry_in_journal->delta_start_s + journal_start_time_s;
                time_t page_last_time_s = page_entry_in_journal->delta_end_s + journal_start_time_s;

                TIME_RANGE_COMPARE prc =
                    is_page_in_time_range(page_first_time_s, page_last_time_s, wanted_start_time_s, wanted_end_time_s);

                if (prc == PAGE_IS_IN_THE_PAST)
                    continue;

                if (prc == PAGE_IS_IN_THE_FUTURE)
                    break;

                // Make sure index is valid for this file
                if (page_entry_in_journal->extent_index > extent_entries) {
                    nd_log_limit_static_thread_var(erl, 60, 0);
                    nd_log_limit(&erl, NDLS_DAEMON, NDLP_ERR,
                                 "DBENGINE: Invalid extent index in journalfile %u",
                                 datafile->fileno);
                    break;
                }

                uint32_t page_update_every_s = page_entry_in_journal->update_every_s;

                if (datafile_acquire(datafile, DATAFILE_ACQUIRE_OPEN_CACHE)) {
                    //for open cache item
                    // add this page to open cache
                    bool added = false;
                    struct extent_io_data ei = {0};
                    ei.block = (extent_list[page_entry_in_journal->extent_index].datafile_offset) >> 12;
                    ei.bytes = extent_list[page_entry_in_journal->extent_index].datafile_size;
                    ei.fileno = datafile->fileno;

                    PGC_ENTRY e = {0};
                    e.hot = false;
                    e.section = (Word_t)ctx;
                    e.metric_id = metric_id;
                    e.start_time_s = page_first_time_s;
                    e.end_time_s = page_last_time_s;
                    e.update_every_s = page_update_every_s;
                    e.data = datafile;
                    e.size = 0;
                    e.custom_data = (uint8_t *)&ei;
                    PGC_PAGE *page = pgc_page_add_and_acquire(open_cache, e, &added);

                    if (!added)
                        datafile_release(datafile, DATAFILE_ACQUIRE_OPEN_CACHE);

                    callback(page, callback_data);

                    pgc_page_release(open_cache, page);

                    pages_found++;
                }
            }
        }
        else {
            nd_log_limit_static_thread_var(erl, 10, 0);
            nd_log_limit(&erl, NDLS_DAEMON, NDLP_ERR, "DBENGINE: failed to access journal file %u of tier %d (SIGBUS)",
                         datafile->fileno, datafile->ctx->config.tier);
        }

        journalfile_v2_data_release(datafile->journalfile);
    }

    return pages_found;
}

void add_page_details_from_journal_v2(PGC_PAGE *page, void *JudyL_pptr) {
    struct rrdengine_datafile *datafile = pgc_page_data(page);

    if(!datafile_acquire(datafile, DATAFILE_ACQUIRE_PAGE_DETAILS)) // for pd
        return;

    Pvoid_t *PValue = PDCJudyLIns(JudyL_pptr, pgc_page_start_time_s(page), PJE0);
    if (!PValue || PValue == PJERR)
        fatal("DBENGINE: corrupted judy array");

    if (unlikely(*PValue)) {
        datafile_release(datafile, DATAFILE_ACQUIRE_PAGE_DETAILS);
        return;
    }

    Word_t metric_id = pgc_page_metric(page);

    // let's add it to the judy
    struct extent_io_data *ei = pgc_page_custom_data(open_cache, page);
    struct page_details *pd = page_details_get();
    *PValue = pd;

    pd->datafile.block = ei->block;
    pd->datafile.bytes = ei->bytes;
    pd->first_time_s = pgc_page_start_time_s(page);
    pd->last_time_s = pgc_page_end_time_s(page);
    pd->datafile.ptr = datafile;
    pd->update_every_s = (uint32_t) pgc_page_update_every_s(page);
    pd->metric_id = metric_id;
    pd->status |= PDC_PAGE_DISK_PENDING | PDC_PAGE_SOURCE_JOURNAL_V2 | PDC_PAGE_DATAFILE_ACQUIRED;
}

// Return a judyL will all pages that have start_time_ut and end_time_ut
// Pvalue of the judy will be the end time for that page
// DBENGINE2:
#define time_delta(finish, pass) do { if(pass) { usec_t t = pass; (pass) = (finish) - (pass); (finish) = t; } } while(0)
static ALWAYS_INLINE_HOT Pvoid_t get_page_list(
        struct rrdengine_instance *ctx,
        METRIC *metric,
        usec_t start_time_ut,
        usec_t end_time_ut,
        time_t *optimal_end_time_s,
        size_t *pages_to_load_from_disk,
        PDC_PAGE_STATUS *common_status
) {
    *optimal_end_time_s = 0;
    *pages_to_load_from_disk = 0;
    *common_status = 0;

    Pvoid_t JudyL_page_array = (Pvoid_t) NULL;

    time_t wanted_start_time_s = (time_t)(start_time_ut / USEC_PER_SEC);
    time_t wanted_end_time_s = (time_t)(end_time_ut / USEC_PER_SEC);

    size_t  pages_found_in_main_cache = 0,
            pages_found_in_open_cache = 0,
            pages_found_in_journals_v2 = 0,
            pages_found_pass4 = 0,
            pages_overlapping = 0,
            pages_total = 0;

    size_t cache_gaps = 0, query_gaps = 0;
    bool done_v2 = false, done_open = false, done_pass4 = false;

    usec_t pass1_ut = 0, pass2_ut = 0, pass3_ut = 0, pass4_ut = 0, finish_ut = 0;

    // --------------------------------------------------------------
    // PASS 1: Check what the main page cache has available

    pass1_ut = now_monotonic_usec();
    size_t pages_pass1 = get_page_list_from_pgc(main_cache, metric, ctx, wanted_start_time_s, wanted_end_time_s,
                                                &JudyL_page_array, &cache_gaps,
                                                false, PDC_PAGE_SOURCE_MAIN_CACHE);
    query_gaps += cache_gaps;
    pages_found_in_main_cache += pages_pass1;
    pages_total += pages_pass1;

    if(pages_found_in_main_cache && !cache_gaps) {
        query_gaps = list_has_time_gaps(ctx, metric, JudyL_page_array, wanted_start_time_s, wanted_end_time_s,
                                        &pages_total, &pages_found_pass4, pages_to_load_from_disk, &pages_overlapping,
                                        optimal_end_time_s, false, common_status);

        if (pages_total && !query_gaps)
            goto we_are_done;
    }

    // --------------------------------------------------------------
    // PASS 2: Check what the open journal page cache has available
    //         these will be loaded from disk

    pass2_ut = now_monotonic_usec();
    size_t pages_pass2 = get_page_list_from_pgc(open_cache, metric, ctx, wanted_start_time_s, wanted_end_time_s,
                                                &JudyL_page_array, &cache_gaps,
                                                true, PDC_PAGE_SOURCE_OPEN_CACHE);
    query_gaps += cache_gaps;
    pages_found_in_open_cache += pages_pass2;
    pages_total += pages_pass2;
    done_open = true;

    if(pages_found_in_open_cache) {
        query_gaps = list_has_time_gaps(ctx, metric, JudyL_page_array, wanted_start_time_s, wanted_end_time_s,
                                        &pages_total, &pages_found_pass4, pages_to_load_from_disk, &pages_overlapping,
                                        optimal_end_time_s, false, common_status);

        if (pages_total && !query_gaps)
            goto we_are_done;
    }

    // --------------------------------------------------------------
    // PASS 3: Check Journal v2 to fill the gaps

    pass3_ut = now_monotonic_usec();
    size_t pages_pass3 = get_page_list_from_journal_v2(ctx, metric, start_time_ut, end_time_ut,
                                                       add_page_details_from_journal_v2, &JudyL_page_array);
    pages_found_in_journals_v2 += pages_pass3;
    pages_total += pages_pass3;
    done_v2 = true;

    // --------------------------------------------------------------
    // PASS 4: Check the cache again
    //         and calculate the time gaps in the query
    //         THIS IS REQUIRED AFTER JOURNAL V2 LOOKUP

    pass4_ut = now_monotonic_usec();
    query_gaps = list_has_time_gaps(ctx, metric, JudyL_page_array, wanted_start_time_s, wanted_end_time_s,
                                    &pages_total, &pages_found_pass4, pages_to_load_from_disk, &pages_overlapping,
                                    optimal_end_time_s, true, common_status);
    done_pass4 = true;

we_are_done:
    finish_ut = now_monotonic_usec();
    time_delta(finish_ut, pass4_ut); // do not change the order
    time_delta(finish_ut, pass3_ut); // do not change the order
    time_delta(finish_ut, pass2_ut); // do not change the order
    time_delta(finish_ut, pass1_ut); // do not change the order

    time_and_count_add(&rrdeng_cache_efficiency_stats.prep_time_in_main_cache_lookup, pass1_ut);

    if(done_open) {
        time_and_count_add(&rrdeng_cache_efficiency_stats.prep_time_in_open_cache_lookup, pass2_ut);
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_meta_source_open_cache, pages_found_in_open_cache, __ATOMIC_RELAXED);
    }

    if(done_v2) {
        time_and_count_add(&rrdeng_cache_efficiency_stats.prep_time_in_journal_v2_lookup, pass3_ut);
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_meta_source_journal_v2, pages_found_in_journals_v2, __ATOMIC_RELAXED);
    }

    if(done_pass4) {
        time_and_count_add(&rrdeng_cache_efficiency_stats.prep_time_in_pass4_lookup, pass4_ut);
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_data_source_main_cache_at_pass4, pages_found_pass4, __ATOMIC_RELAXED);
    }

    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.queries_planned_with_gaps, (query_gaps) ? 1 : 0, __ATOMIC_RELAXED);
    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_total, pages_total, __ATOMIC_RELAXED);
    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_meta_source_main_cache, pages_found_in_main_cache, __ATOMIC_RELAXED);
    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_data_source_main_cache, pages_found_in_main_cache, __ATOMIC_RELAXED);
    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_to_load_from_disk, *pages_to_load_from_disk, __ATOMIC_RELAXED);
    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_overlapping_skipped, pages_overlapping, __ATOMIC_RELAXED);

    return JudyL_page_array;
}

ALWAYS_INLINE void rrdeng_prep_wait(PDC *pdc) {
    if (unlikely(pdc && !pdc->prep_done)) {
        usec_t started_ut = now_monotonic_usec();
        completion_wait_for(&pdc->prep_completion);
        pdc->prep_done = true;
        time_and_count_add(&rrdeng_cache_efficiency_stats.query_time_wait_for_prep, now_monotonic_usec() - started_ut);
    }
}

ALWAYS_INLINE_HOT void rrdeng_prep_query(struct page_details_control *pdc, bool worker) {
    if(worker)
        worker_is_busy(UV_EVENT_DBENGINE_QUERY);

    pdc->page_list_JudyL = get_page_list(pdc->ctx, pdc->metric,
                                                 pdc->start_time_s * USEC_PER_SEC,
                                                 pdc->end_time_s * USEC_PER_SEC,
                                                 &pdc->optimal_end_time_s,
                                                 &pdc->pages_to_load_from_disk,
                                                 &pdc->common_status);

    internal_fatal(pdc->pages_to_load_from_disk && !(pdc->common_status & PDC_PAGE_DISK_PENDING),
                   "DBENGINE: PDC reports there are %zu pages to load from disk, "
                   "but none of the pages has the PDC_PAGE_DISK_PENDING flag",
                   pdc->pages_to_load_from_disk);

    internal_fatal(!pdc->pages_to_load_from_disk && (pdc->common_status & PDC_PAGE_DISK_PENDING),
                   "DBENGINE: PDC reports there are no pages to load from disk, "
                   "but one or more pages have the PDC_PAGE_DISK_PENDING flag");

    if (pdc->pages_to_load_from_disk && pdc->page_list_JudyL) {
        pdc_acquire(pdc); // we get 1 for the 1st worker in the chain: do_read_page_list_work()
        usec_t start_ut = now_monotonic_usec();
        if(likely(pdc->priority == STORAGE_PRIORITY_SYNCHRONOUS)) {
            pdc_route_synchronously(pdc->ctx, pdc);
            time_and_count_add(&rrdeng_cache_efficiency_stats.prep_time_to_route_sync, now_monotonic_usec() - start_ut);
        }
        else if(likely(pdc->priority == STORAGE_PRIORITY_SYNCHRONOUS_FIRST)) {
            pdc_route_synchronously_first(pdc->ctx, pdc);
            time_and_count_add(&rrdeng_cache_efficiency_stats.prep_time_to_route_syncfirst, now_monotonic_usec() - start_ut);
        }
        else {
            pdc_route_asynchronously(pdc->ctx, pdc);
            time_and_count_add(&rrdeng_cache_efficiency_stats.prep_time_to_route_async, now_monotonic_usec() - start_ut);
        }
    }
    else
        completion_mark_complete(&pdc->page_completion);

    completion_mark_complete(&pdc->prep_completion);

    pdc_release_and_destroy_if_unreferenced(pdc, true, true);

    if(worker)
        worker_is_idle();
}

/**
 * Searches for pages in a time range and triggers disk I/O if necessary and possible.
 * @param ctx DB context
 * @param handle query handle as initialized
 * @param start_time_ut inclusive starting time in usec
 * @param end_time_ut inclusive ending time in usec
 * @return 1 / 0 (pages found or not found)
 */
ALWAYS_INLINE_HOT void pg_cache_preload(struct rrdeng_query_handle *handle) {
    if (unlikely(!handle || !handle->metric))
        return;

    __atomic_add_fetch(&handle->ctx->atomic.inflight_queries, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.currently_running_queries, 1, __ATOMIC_RELAXED);
    handle->pdc = pdc_get();
    handle->pdc->metric = mrg_metric_dup(main_mrg, handle->metric);
    handle->pdc->start_time_s = handle->start_time_s;
    handle->pdc->end_time_s = handle->end_time_s;
    handle->pdc->priority = handle->priority;
    handle->pdc->optimal_end_time_s = handle->end_time_s;
    handle->pdc->ctx = handle->ctx;
    handle->pdc->refcount = 1;
    spinlock_init(&handle->pdc->refcount_spinlock);
    completion_init(&handle->pdc->prep_completion);
    completion_init(&handle->pdc->page_completion);

    if(ctx_is_available_for_queries(handle->ctx)) {
        handle->pdc->refcount++; // we get 1 for the query thread and 1 for the prep thread

        if(unlikely(handle->pdc->priority == STORAGE_PRIORITY_SYNCHRONOUS || handle->pdc->priority == STORAGE_PRIORITY_SYNCHRONOUS_FIRST))
            rrdeng_prep_query(handle->pdc, false);
        else
            rrdeng_enq_cmd(handle->ctx, RRDENG_OPCODE_QUERY, handle->pdc, NULL, handle->priority, NULL, NULL);
    }
    else {
        completion_mark_complete(&handle->pdc->prep_completion);
        completion_mark_complete(&handle->pdc->page_completion);
    }
}

/*
 * Searches for the first page between start_time and end_time and gets a reference.
 * start_time and end_time are inclusive.
 * If index is NULL lookup by UUID (id).
 */
struct pgc_page *pg_cache_lookup_next(
        struct rrdengine_instance *ctx,
        PDC *pdc,
        time_t now_s,
        uint32_t last_update_every_s,
        size_t *entries
) {
    if (unlikely(!pdc))
        return NULL;

    rrdeng_prep_wait(pdc);

    if (unlikely(!pdc->page_list_JudyL))
        return NULL;

    usec_t start_ut = now_monotonic_usec();
    size_t gaps = 0;
    bool waited = false, preloaded;
    PGC_PAGE *page = NULL;

    while(!page) {
        bool page_from_pd = false;
        preloaded = false;
        struct page_details *pd = pdc_find_page_for_time(
                pdc->page_list_JudyL, now_s, &gaps,
                PDC_PAGE_PROCESSED, PDC_PAGE_EMPTY);

        if (!pd)
            break;

        page = pd->page;
        page_from_pd = true;
        preloaded = pdc_page_status_check(pd, PDC_PAGE_PRELOADED);
        if(!page) {
            if(!completion_is_done(&pdc->page_completion)) {
                page = pgc_page_get_and_acquire(main_cache, (Word_t)ctx,
                                                pd->metric_id, pd->first_time_s, PGC_SEARCH_EXACT);
                page_from_pd = false;
                preloaded = pdc_page_status_check(pd, PDC_PAGE_PRELOADED);
            }

            if(!page) {
                pdc->completed_jobs =
                        completion_wait_for_a_job(&pdc->page_completion, pdc->completed_jobs);

                page = pd->page;
                page_from_pd = true;
                preloaded = pdc_page_status_check(pd, PDC_PAGE_PRELOADED);
                waited = true;
            }
        }

        if(page && pgd_is_empty(pgc_page_data(page)))
                pdc_page_status_set(pd, PDC_PAGE_EMPTY);

        if(!page || pdc_page_status_check(pd, PDC_PAGE_QUERY_GLOBAL_SKIP_LIST | PDC_PAGE_EMPTY)) {
            page = NULL;
            continue;
        }

        // we now have page and is not empty

        time_t page_start_time_s = pgc_page_start_time_s(page);
        time_t page_end_time_s = pgc_page_end_time_s(page);
        uint32_t page_update_every_s = pgc_page_update_every_s(page);

        if(unlikely(page_start_time_s == INVALID_TIME || page_end_time_s == INVALID_TIME)) {
            __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_zero_time_skipped, 1, __ATOMIC_RELAXED);
            pgc_page_to_clean_evict_or_release(main_cache, page);
            pdc_page_status_set(pd, PDC_PAGE_INVALID | PDC_PAGE_RELEASED);
            pd->page = page = NULL;
            continue;
        }
        else {
            if (unlikely(page_update_every_s <= 0 || page_update_every_s > 86400)) {
                __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_invalid_update_every_fixed, 1, __ATOMIC_RELAXED);
                page_update_every_s = pgc_page_fix_update_every(page, last_update_every_s);
                pd->update_every_s = page_update_every_s;
            }

            size_t entries_by_size = pgd_slots_used(pgc_page_data(page));
            size_t entries_by_time = page_entries_by_time(page_start_time_s, page_end_time_s, page_update_every_s);
            if(unlikely(entries_by_size < entries_by_time)) {
                time_t fixed_page_end_time_s = (time_t)(page_start_time_s + (entries_by_size - 1) * page_update_every_s);
                pd->last_time_s = page_end_time_s = pgc_page_fix_end_time_s(page, fixed_page_end_time_s);
                entries_by_time = (page_end_time_s - (page_start_time_s - page_update_every_s)) / page_update_every_s;

                internal_fatal(entries_by_size != entries_by_time, "DBENGINE: wrong entries by time again!");

                __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_invalid_entries_fixed, 1, __ATOMIC_RELAXED);
            }
            *entries = entries_by_time;
        }

        if(unlikely(page_end_time_s < now_s)) {
            __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_past_time_skipped, 1, __ATOMIC_RELAXED);
            pgc_page_release(main_cache, page);
            pdc_page_status_set(pd, PDC_PAGE_SKIP | PDC_PAGE_RELEASED);
            pd->page = page = NULL;
            continue;
        }

        if(page_from_pd)
            // PDC_PAGE_RELEASED is for pdc_destroy() to not release the page twice - the caller will release it
            pdc_page_status_set(pd, PDC_PAGE_RELEASED | PDC_PAGE_PROCESSED);
        else
            pdc_page_status_set(pd, PDC_PAGE_PROCESSED);
    }

    if(gaps && !pdc->executed_with_gaps)
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.queries_executed_with_gaps, 1, __ATOMIC_RELAXED);
    pdc->executed_with_gaps = +gaps;

    if(page) {
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

    if(waited) {
        if(preloaded)
            time_and_count_add(&rrdeng_cache_efficiency_stats.query_time_to_slow_preload_next_page, now_monotonic_usec() - start_ut);
        else
            time_and_count_add(&rrdeng_cache_efficiency_stats.query_time_to_slow_disk_next_page, now_monotonic_usec() - start_ut);
    }
    else {
        if(preloaded)
            time_and_count_add(&rrdeng_cache_efficiency_stats.query_time_to_fast_preload_next_page, now_monotonic_usec() - start_ut);
        else
            time_and_count_add(&rrdeng_cache_efficiency_stats.query_time_to_fast_disk_next_page, now_monotonic_usec() - start_ut);
    }

    return page;
}

void pgc_open_add_hot_page(
    Word_t section,
    Word_t metric_id,
    time_t start_time_s,
    time_t end_time_s,
    uint32_t update_every_s,
    struct rrdengine_datafile *datafile,
    uint64_t extent_offset,
    unsigned extent_size)
{

    if(!datafile_acquire(datafile, DATAFILE_ACQUIRE_OPEN_CACHE)) // for open cache item
        fatal("DBENGINE: cannot acquire datafile to put page in open cache");

    struct extent_io_data ext_io_data = {
            .fileno = datafile->fileno,
            .block = extent_offset >> 12,
            .bytes = extent_size,
    };

    PGC_ENTRY page_entry = {
            .hot = true,
            .section = section,
            .metric_id = metric_id,
            .start_time_s = start_time_s,
            .end_time_s =  end_time_s,
            .update_every_s = update_every_s,
            .size = 0,
            .data = datafile,
            .custom_data = (uint8_t *) &ext_io_data,
    };

    internal_fatal(!datafile->fileno, "DBENGINE: datafile supplied does not have a number");

    bool added = true;
    PGC_PAGE *page = pgc_page_add_and_acquire(open_cache, page_entry, &added);
    int tries = 100;
    while(!added && page_entry.end_time_s > pgc_page_end_time_s(page) && tries--) {
        pgc_page_to_clean_evict_or_release(open_cache, page);
        page = pgc_page_add_and_acquire(open_cache, page_entry, &added);
    }

    if(!added) {
        datafile_release(datafile, DATAFILE_ACQUIRE_OPEN_CACHE);

        internal_fatal(page_entry.end_time_s > pgc_page_end_time_s(page),
                       "DBENGINE: cannot add longer page to open cache");
    }

    pgc_page_release(open_cache, (PGC_PAGE *)page);
}

int64_t dynamic_open_cache_size(void) {
    int64_t main_wanted_cache_size = pgc_get_wanted_cache_size(main_cache);
    int64_t target_size = main_wanted_cache_size / 100 * 5;

    if(target_size < 2 * 1024 * 1024)
        target_size = 2 * 1024 * 1024;

    int64_t main_current_cache_size = pgc_get_current_cache_size(main_cache);

    int64_t main_free_cache_size = (main_wanted_cache_size > main_current_cache_size) ?
                                      main_wanted_cache_size - main_current_cache_size : 0;

    return target_size + main_free_cache_size;
}

int64_t dynamic_extent_cache_size(void) {
    int64_t main_wanted_cache_size = pgc_get_wanted_cache_size(main_cache);
    int64_t target_size = main_wanted_cache_size / 100 * 30;

    if(target_size < 5 * 1024 * 1024)
        target_size = 5 * 1024 * 1024;

    int64_t main_current_cache_size = pgc_get_current_cache_size(main_cache);

    int64_t main_free_cache_size = (main_wanted_cache_size > main_current_cache_size) ?
                                      main_wanted_cache_size - main_current_cache_size : 0;

    return target_size + main_free_cache_size;
}

size_t pgc_main_nominal_page_size(void *data) {
    return pgd_buffer_memory_footprint(data);
}

void pgc_and_mrg_initialize(void)
{
    main_mrg = mrg_create();

    size_t target_cache_size = (size_t)default_rrdeng_page_cache_mb * 1024ULL * 1024ULL;
    size_t main_cache_size = (target_cache_size / 100) * 70;
    size_t open_cache_size = 0;
    size_t extent_cache_size = (target_cache_size / 100) * 30;

    if(extent_cache_size < 5 * 1024 * 1024) {
        extent_cache_size = 5 * 1024 * 1024;
        main_cache_size = target_cache_size - extent_cache_size;
    }

    extent_cache_size += (size_t)(default_rrdeng_extent_cache_mb * 1024ULL * 1024ULL);

    main_cache = pgc_create(
            "MAIN_PGC",
            main_cache_size,
            main_cache_free_clean_page_callback,
            (size_t) rrdeng_pages_per_extent,
            main_cache_flush_dirty_page_init_callback,
            main_cache_flush_dirty_page_callback,
            2,
            pgc_max_evictors(),
            1000,
            1,
            PGC_OPTIONS_AUTOSCALE | PGC_OPTIONS_EVICT_PAGES_NO_INLINE,
            0,
            0
    );
    pgc_set_nominal_page_size_callback(main_cache, pgc_main_nominal_page_size);

    open_cache = pgc_create(
            "OPEN_PGC",
            open_cache_size,
            open_cache_free_clean_page_callback,
            2,
            NULL,
            open_cache_flush_dirty_page_callback,
            1,
            pgc_max_evictors(),
            1000,
            1,
            PGC_OPTIONS_AUTOSCALE | PGC_OPTIONS_FLUSH_PAGES_NO_INLINE | PGC_OPTIONS_EVICT_PAGES_NO_INLINE,
            0,
            sizeof(struct extent_io_data)
    );
    pgc_set_dynamic_target_cache_size_callback(open_cache, dynamic_open_cache_size);

    extent_cache = pgc_create(
            "EXTENT_PGC",
            extent_cache_size,
            extent_cache_free_clean_page_callback,
            2,
            NULL,
            extent_cache_flush_dirty_page_callback,
            1,
            pgc_max_evictors(),
            1000,
            1,
            PGC_OPTIONS_AUTOSCALE | PGC_OPTIONS_FLUSH_PAGES_NO_INLINE | PGC_OPTIONS_EVICT_PAGES_NO_INLINE, // no flushing needed
            0,
            0
    );
    pgc_set_dynamic_target_cache_size_callback(extent_cache, dynamic_extent_cache_size);
}

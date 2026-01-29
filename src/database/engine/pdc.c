// SPDX-License-Identifier: GPL-3.0-or-later

#include "pdc.h"
#include "dbengine-compression.h"

struct extent_page_details_list {
    uint32_t extent_block;
    uint32_t extent_size;
    Pvoid_t page_details_by_metric_id_JudyL;
    struct page_details_control *pdc;
    struct rrdengine_datafile *datafile;

    struct rrdeng_cmd *cmd;
    bool head_to_datafile_extent_queries_pending_for_extent;

    struct {
        struct extent_page_details_list *prev;
        struct extent_page_details_list *next;
    } query;
};

typedef struct datafile_extent_offset_list {
    uv_file file;
    unsigned fileno;
    Pvoid_t extent_pd_list_by_extent_offset_JudyL;
} DEOL;

// ----------------------------------------------------------------------------
// PDC cache

static struct {
    struct {
        ARAL *ar;
    } pdc;

    struct {
        ARAL *ar;
    } pd;

    struct {
        ARAL *ar;
    } epdl;

    struct {
        ARAL *ar;
    } deol;

    struct {
        ARAL *ar;
    } epdl_extent;
} pdc_globals = {};

void pdc_init(void) {
    pdc_globals.pdc.ar = aral_create(
            "dbengine-pdc",
            sizeof(PDC),
            0,
            0,
            NULL,
            NULL, NULL, false, false, true
            );

    pulse_aral_register(pdc_globals.pdc.ar, "pdc");
}

ALWAYS_INLINE PDC *pdc_get(void) {
    PDC *pdc = aral_mallocz(pdc_globals.pdc.ar);
    memset(pdc, 0, sizeof(PDC));
    return pdc;
}

static ALWAYS_INLINE void pdc_release(PDC *pdc) {
    aral_freez(pdc_globals.pdc.ar, pdc);
}

struct aral_statistics *pdc_aral_stats(void) {
    return aral_get_statistics(pdc_globals.pdc.ar);
}

// ----------------------------------------------------------------------------
// PD cache

void page_details_init(void) {
    pdc_globals.pd.ar = aral_create(
            "dbengine-pd",
            sizeof(struct page_details),
            0,
            0,
            NULL,
            NULL, NULL, false, false, true
    );
    pulse_aral_register(pdc_globals.pd.ar, "pd");
}

ALWAYS_INLINE struct page_details *page_details_get(void) {
    struct page_details *pd = aral_mallocz(pdc_globals.pd.ar);
    memset(pd, 0, sizeof(struct page_details));
    return pd;
}

static ALWAYS_INLINE void page_details_release(struct page_details *pd) {
    aral_freez(pdc_globals.pd.ar, pd);
}

struct aral_statistics *pd_aral_stats(void) {
    return aral_get_statistics(pdc_globals.pd.ar);
}

// ----------------------------------------------------------------------------
// epdl cache

void epdl_init(void) {
    pdc_globals.epdl.ar = aral_create(
            "dbengine-epdl",
            sizeof(EPDL),
            0,
            0,
            NULL,
            NULL, NULL, false, false, true
    );
    pulse_aral_register(pdc_globals.epdl.ar, "epdl");
}

static ALWAYS_INLINE EPDL *epdl_get(void) {
    EPDL *epdl = aral_mallocz(pdc_globals.epdl.ar);
    memset(epdl, 0, sizeof(EPDL));
    return epdl;
}

static ALWAYS_INLINE void epdl_release(EPDL *epdl) {
    aral_freez(pdc_globals.epdl.ar, epdl);
}

struct aral_statistics *epdl_aral_stats(void) {
    return aral_get_statistics(pdc_globals.epdl.ar);
}

// ----------------------------------------------------------------------------
// deol cache

void deol_init(void) {
    pdc_globals.deol.ar = aral_create(
            "dbengine-deol",
            sizeof(DEOL),
            0,
            0,
            NULL,
            NULL, NULL, false, false, true
    );

    pulse_aral_register(pdc_globals.deol.ar, "deol");
}

static ALWAYS_INLINE DEOL *deol_get(void) {
    DEOL *deol = aral_mallocz(pdc_globals.deol.ar);
    memset(deol, 0, sizeof(DEOL));
    return deol;
}

static ALWAYS_INLINE void deol_release(DEOL *deol) {
    aral_freez(pdc_globals.deol.ar, deol);
}

struct aral_statistics *deol_aral_stats(void) {
    return aral_get_statistics(pdc_globals.deol.ar);
}

// ----------------------------------------------------------------------------
// epdl_extent cache

void epdl_extent_init(void) {
    pdc_globals.epdl_extent.ar = aral_create(
            "dbengine-epdl-extent",
            sizeof(EPDL_EXTENT),
            0,
            0,
            NULL,
            NULL, NULL, false, false, true
    );

    pulse_aral_register(pdc_globals.epdl_extent.ar, "epdl_extent");
}

static ALWAYS_INLINE EPDL_EXTENT *epdl_extent_get(void) {
    EPDL_EXTENT *e = aral_mallocz(pdc_globals.epdl_extent.ar);
    memset(e, 0, sizeof(EPDL_EXTENT));
    return e;
}

ALWAYS_INLINE void epdl_extent_release(EPDL_EXTENT *e) {
    aral_freez(pdc_globals.epdl_extent.ar, e);
}

struct aral_statistics *epdl_extent_aral_stats(void) {
    return aral_get_statistics(pdc_globals.epdl_extent.ar);
}

// ----------------------------------------------------------------------------
// extent with buffer cache

static struct {
    struct {
        SPINLOCK spinlock;
        struct extent_buffer *available_items;
        size_t available;
    } protected;

    struct {
        size_t allocated;
        size_t allocated_bytes;
    } atomics;

    size_t max_size;

} extent_buffer_globals = {
        .protected = {
                .spinlock = SPINLOCK_INITIALIZER,
                .available_items = NULL,
                .available = 0,
        },
        .atomics = {
                .allocated = 0,
                .allocated_bytes = 0,
        },
        .max_size = MAX_EXTENT_UNCOMPRESSED_SIZE
};

void extent_buffer_init(void) {
    size_t max_extent_uncompressed = MAX_EXTENT_UNCOMPRESSED_SIZE;
    size_t max_size = (size_t)LZ4_compressBound(MAX_EXTENT_UNCOMPRESSED_SIZE);
    if(max_size < max_extent_uncompressed)
        max_size = max_extent_uncompressed;

    extent_buffer_globals.max_size = max_size;
}

void extent_buffer_cleanup1(void) {
    struct extent_buffer *item = NULL;

    if(!spinlock_trylock(&extent_buffer_globals.protected.spinlock))
        return;

    if(extent_buffer_globals.protected.available_items && extent_buffer_globals.protected.available > 1) {
        item = extent_buffer_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(extent_buffer_globals.protected.available_items, item, cache.prev, cache.next);
        extent_buffer_globals.protected.available--;
    }

    spinlock_unlock(&extent_buffer_globals.protected.spinlock);

    if(item) {
        size_t bytes = sizeof(struct extent_buffer) + item->bytes;
        freez(item);
        __atomic_sub_fetch(&extent_buffer_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
        __atomic_sub_fetch(&extent_buffer_globals.atomics.allocated_bytes, bytes, __ATOMIC_RELAXED);
    }
}

ALWAYS_INLINE struct extent_buffer *extent_buffer_get(size_t size) {
    internal_fatal(size > extent_buffer_globals.max_size, "DBENGINE: extent size is too big");

    struct extent_buffer *eb = NULL;

    if(size < extent_buffer_globals.max_size)
        size = extent_buffer_globals.max_size;

    spinlock_lock(&extent_buffer_globals.protected.spinlock);
    if(likely(extent_buffer_globals.protected.available_items)) {
        eb = extent_buffer_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(extent_buffer_globals.protected.available_items, eb, cache.prev, cache.next);
        extent_buffer_globals.protected.available--;
    }
    spinlock_unlock(&extent_buffer_globals.protected.spinlock);

    if(unlikely(eb && eb->bytes < size)) {
        size_t bytes = sizeof(struct extent_buffer) + eb->bytes;
        freez(eb);
        eb = NULL;
        __atomic_sub_fetch(&extent_buffer_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
        __atomic_sub_fetch(&extent_buffer_globals.atomics.allocated_bytes, bytes, __ATOMIC_RELAXED);
    }

    if(unlikely(!eb)) {
        size_t bytes = sizeof(struct extent_buffer) + size;
        eb = mallocz(bytes);
        eb->bytes = size;
        __atomic_add_fetch(&extent_buffer_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
        __atomic_add_fetch(&extent_buffer_globals.atomics.allocated_bytes, bytes, __ATOMIC_RELAXED);
    }

    return eb;
}

ALWAYS_INLINE void extent_buffer_release(struct extent_buffer *eb) {
    if(unlikely(!eb)) return;

    spinlock_lock(&extent_buffer_globals.protected.spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(extent_buffer_globals.protected.available_items, eb, cache.prev, cache.next);
    extent_buffer_globals.protected.available++;
    spinlock_unlock(&extent_buffer_globals.protected.spinlock);
}

size_t extent_buffer_cache_size(void) {
    return __atomic_load_n(&extent_buffer_globals.atomics.allocated_bytes, __ATOMIC_RELAXED);
}

// ----------------------------------------------------------------------------
// epdl logic

static ALWAYS_INLINE void epdl_destroy(EPDL *epdl)
{
    Pvoid_t *pd_by_start_time_s_JudyL;
    Word_t metric_id_index = 0;
    bool metric_id_first = true;
    while ((pd_by_start_time_s_JudyL = PDCJudyLFirstThenNext(
            epdl->page_details_by_metric_id_JudyL,
            &metric_id_index, &metric_id_first)))
        PDCJudyLFreeArray(pd_by_start_time_s_JudyL, PJE0);

    PDCJudyLFreeArray(&epdl->page_details_by_metric_id_JudyL, PJE0);
    epdl_release(epdl);
}

static ALWAYS_INLINE void epdl_mark_all_not_loaded_pages_as_failed(EPDL *epdl, PDC_PAGE_STATUS tags, size_t *statistics_counter)
{
    size_t pages_matched = 0;

    Word_t metric_id_index = 0;
    bool metric_id_first = true;
    Pvoid_t *pd_by_start_time_s_JudyL;
    while((pd_by_start_time_s_JudyL = PDCJudyLFirstThenNext(epdl->page_details_by_metric_id_JudyL, &metric_id_index, &metric_id_first))) {

        Word_t start_time_index = 0;
        bool start_time_first = true;
        Pvoid_t *PValue;
        while ((PValue = PDCJudyLFirstThenNext(*pd_by_start_time_s_JudyL, &start_time_index, &start_time_first))) {
            struct page_details *pd = *PValue;

            if(!pd->page && !pdc_page_status_check(pd, PDC_PAGE_FAILED|PDC_PAGE_READY)) {
                pdc_page_status_set(pd, PDC_PAGE_FAILED | tags);
                pages_matched++;
            }
        }
    }

    if(pages_matched && statistics_counter)
        __atomic_add_fetch(statistics_counter, pages_matched, __ATOMIC_RELAXED);
}
/*
static bool epdl_check_if_pages_are_already_in_cache(struct rrdengine_instance *ctx, EPDL *epdl, PDC_PAGE_STATUS tags)
{
    size_t count_remaining = 0;
    size_t found = 0;

    Word_t metric_id_index = 0;
    bool metric_id_first = true;
    Pvoid_t *pd_by_start_time_s_JudyL;
    while((pd_by_start_time_s_JudyL = PDCJudyLFirstThenNext(epdl->page_details_by_metric_id_JudyL, &metric_id_index, &metric_id_first))) {

        Word_t start_time_index = 0;
        bool start_time_first = true;
        Pvoid_t *PValue;
        while ((PValue = PDCJudyLFirstThenNext(*pd_by_start_time_s_JudyL, &start_time_index, &start_time_first))) {
            struct page_details *pd = *PValue;
            if (pd->page)
                continue;

            pd->page = pgc_page_get_and_acquire(main_cache, (Word_t) ctx, pd->metric_id, pd->first_time_s, PGC_SEARCH_EXACT);
            if (pd->page) {
                found++;
                pdc_page_status_set(pd, PDC_PAGE_READY | tags);
            }
            else
                count_remaining++;
        }
    }

    if(found) {
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_ok_preloaded, found, __ATOMIC_RELAXED);
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_data_source_main_cache, found, __ATOMIC_RELAXED);
    }

    return count_remaining == 0;
}
*/

// ----------------------------------------------------------------------------
// PDC logic

static ALWAYS_INLINE void pdc_destroy(PDC *pdc) {
    if(pdc->metric)
        mrg_metric_release(main_mrg, pdc->metric);

    completion_destroy(&pdc->prep_completion);
    completion_destroy(&pdc->page_completion);

    Pvoid_t *PValue;
    struct page_details *pd;
    Word_t time_index = 0;
    bool first_then_next = true;
    size_t unroutable = 0, cancelled = 0;
    while((PValue = PDCJudyLFirstThenNext(pdc->page_list_JudyL, &time_index, &first_then_next))) {
        pd = *PValue;

        // no need for atomics here - we are done...
        PDC_PAGE_STATUS status = pd->status;

        if(status & PDC_PAGE_DATAFILE_ACQUIRED) {
            datafile_release(pd->datafile.ptr, DATAFILE_ACQUIRE_PAGE_DETAILS);
            pd->datafile.ptr = NULL;
        }

        internal_fatal(pd->datafile.ptr, "DBENGINE: page details has a datafile.ptr that is not released.");

        if(!pd->page && !(status & (PDC_PAGE_READY | PDC_PAGE_FAILED | PDC_PAGE_RELEASED | PDC_PAGE_SKIP | PDC_PAGE_INVALID | PDC_PAGE_CANCELLED))) {
            // pdc_page_status_set(pd, PDC_PAGE_FAILED);
            unroutable++;
        }
        else if(!pd->page && (status & PDC_PAGE_CANCELLED))
            cancelled++;

        if(pd->page && !(status & PDC_PAGE_RELEASED)) {
            pgc_page_release(main_cache, pd->page);
            // pdc_page_status_set(pd, PDC_PAGE_RELEASED);
        }

        page_details_release(pd);
    }

    PDCJudyLFreeArray(&pdc->page_list_JudyL, PJE0);

    __atomic_sub_fetch(&rrdeng_cache_efficiency_stats.currently_running_queries, 1, __ATOMIC_RELAXED);
    __atomic_sub_fetch(&pdc->ctx->atomic.inflight_queries, 1, __ATOMIC_RELAXED);
    pdc_release(pdc);

    if(unroutable)
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_fail_unroutable, unroutable, __ATOMIC_RELAXED);

    if(cancelled)
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_fail_cancelled, cancelled, __ATOMIC_RELAXED);
}

ALWAYS_INLINE void pdc_acquire(PDC *pdc) {
    spinlock_lock(&pdc->refcount_spinlock);

    if(pdc->refcount < 1)
        fatal("DBENGINE: pdc is not referenced and cannot be acquired");

    pdc->refcount++;
    spinlock_unlock(&pdc->refcount_spinlock);
}

ALWAYS_INLINE bool pdc_release_and_destroy_if_unreferenced(PDC *pdc, bool worker, bool router __maybe_unused) {
    if(unlikely(!pdc))
        return true;

    spinlock_lock(&pdc->refcount_spinlock);

    if(pdc->refcount <= 0)
        fatal("DBENGINE: pdc is not referenced and cannot be released");

    pdc->refcount--;

    if (pdc->refcount <= 1 && worker) {
        // when 1 refcount is remaining, and we are a worker,
        // we can mark the job completed:
        // - if the remaining refcount is from the query caller, we will wake it up
        // - if the remaining refcount is from another worker, the query thread is already away
        completion_mark_complete(&pdc->page_completion);
    }

    if (pdc->refcount == 0) {
        spinlock_unlock(&pdc->refcount_spinlock);
        pdc_destroy(pdc);
        return true;
    }

    spinlock_unlock(&pdc->refcount_spinlock);
    return false;
}

ALWAYS_INLINE void epdl_cmd_queued(void *epdl_ptr, struct rrdeng_cmd *cmd) {
    EPDL *epdl = epdl_ptr;
    epdl->cmd = cmd;
}

ALWAYS_INLINE void epdl_cmd_dequeued(void *epdl_ptr) {
    EPDL *epdl = epdl_ptr;
    epdl->cmd = NULL;
}

static ALWAYS_INLINE struct rrdeng_cmd *epdl_get_cmd(void *epdl_ptr) {
    EPDL *epdl = epdl_ptr;
    return epdl->cmd;
}

static ALWAYS_INLINE EPDL_EXTENT *epdl_find_extent_base(EPDL *epdl) {
    EPDL_EXTENT *e = NULL;
    rw_spinlock_read_lock(&epdl->datafile->extent_epdl.spinlock);
    Pvoid_t *PValue = JudyLGet(epdl->datafile->extent_epdl.epdl_per_extent, epdl->extent_block, PJE0);
    internal_fatal(PValue == PJERR, "DBENGINE: corrupted pending extent judy");
    if(PValue)
        e = *PValue;
    rw_spinlock_read_unlock(&epdl->datafile->extent_epdl.spinlock);

    if(!e) {
        EPDL_EXTENT *e_to_free = NULL;
        e = epdl_extent_get();

        rw_spinlock_write_lock(&epdl->datafile->extent_epdl.spinlock);
        PValue = JudyLIns(&epdl->datafile->extent_epdl.epdl_per_extent, epdl->extent_block, PJE0);
        internal_fatal(!PValue || PValue == PJERR, "DBENGINE: corrupted pending extent judy");
        if(!*PValue) {
            *PValue = e;
            spinlock_init(&e->spinlock);
        }
        else {
            e_to_free = e;
            e = *PValue;
        }
        rw_spinlock_write_unlock(&epdl->datafile->extent_epdl.spinlock);

        epdl_extent_release(e_to_free);
    }

    return e;
}

static ALWAYS_INLINE bool epdl_pending_add(EPDL *epdl) {
    EPDL_EXTENT *e = epdl_find_extent_base(epdl);
    spinlock_lock(&e->spinlock);

    bool added_new;
    if(unlikely(e->base)) {
        added_new = false;
        epdl->head_to_datafile_extent_queries_pending_for_extent = false;

        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_extent_merged, 1, __ATOMIC_RELAXED);

//        if(e->base->pdc->priority > epdl->pdc->priority) {
//            e->base->pdc->priority = epdl->pdc->priority;
//            rrdeng_req_cmd(epdl_get_cmd, e->base, epdl->pdc->priority);
//        }
    }
    else {
        added_new = true;
        epdl->head_to_datafile_extent_queries_pending_for_extent = true;
    }

    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(e->base, epdl, query.prev, query.next);
    spinlock_unlock(&e->spinlock);

    return added_new;
}

static ALWAYS_INLINE void epdl_pending_del(EPDL *epdl) {
    EPDL_EXTENT *e = epdl_find_extent_base(epdl);
    spinlock_lock(&e->spinlock);
    e->base = NULL;
    spinlock_unlock(&e->spinlock);
}

ALWAYS_INLINE_HOT void pdc_to_epdl_router(struct rrdengine_instance *ctx, PDC *pdc, execute_extent_page_details_list_t exec_first_extent_list, execute_extent_page_details_list_t exec_rest_extent_list)
{
    Pvoid_t *PValue;
    Pvoid_t *PValue1;
    Pvoid_t *PValue2;
    Word_t time_index = 0;
    struct page_details *pd = NULL;

    // this is the entire page list
    // Lets do some deduplication
    // 1. Per datafile
    // 2. Per extent
    // 3. Pages per extent will be added to the cache either as acquired or not

    Pvoid_t JudyL_datafile_list = NULL;

    DEOL *deol;
    EPDL *epdl;

    if (pdc->page_list_JudyL) {
        bool first_then_next = true;
        while((PValue = PDCJudyLFirstThenNext(pdc->page_list_JudyL, &time_index, &first_then_next))) {
            pd = *PValue;

            internal_fatal(!pd,
                           "DBENGINE: pdc page list has an empty page details entry");

            if (!(pd->status & PDC_PAGE_DISK_PENDING))
                continue;

            internal_fatal(!(pd->status & PDC_PAGE_DATAFILE_ACQUIRED),
                           "DBENGINE: page details has not acquired the datafile");

            internal_fatal((pd->status & (PDC_PAGE_READY | PDC_PAGE_FAILED)),
                           "DBENGINE: page details has disk pending flag but it is ready/failed");

            internal_fatal(pd->page,
                           "DBENGINE: page details has a page linked to it, but it is marked for loading");

            PValue1 = PDCJudyLIns(&JudyL_datafile_list, pd->datafile.ptr->fileno, PJE0);
            if (PValue1 && !*PValue1) {
                *PValue1 = deol = deol_get();
                deol->extent_pd_list_by_extent_offset_JudyL = NULL;
                deol->fileno = pd->datafile.ptr->fileno;
            }
            else
                deol = *PValue1;

            PValue2 = PDCJudyLIns(&deol->extent_pd_list_by_extent_offset_JudyL, pd->datafile.block, PJE0);
            if (PValue2 && !*PValue2) {
                *PValue2 = epdl = epdl_get();
                epdl->page_details_by_metric_id_JudyL = NULL;
                epdl->extent_block = pd->datafile.block;
                epdl->extent_size = pd->datafile.bytes;
                epdl->datafile = pd->datafile.ptr;
            }
            else
                epdl = *PValue2;

            Pvoid_t *pd_by_first_time_s_judyL = PDCJudyLIns(&epdl->page_details_by_metric_id_JudyL, pd->metric_id, PJE0);
            Pvoid_t *pd_pptr = PDCJudyLIns(pd_by_first_time_s_judyL, pd->first_time_s, PJE0);
            *pd_pptr = pd;
        }

        size_t extent_list_no = 0;
        Word_t datafile_no = 0;
        first_then_next = true;
        while((PValue = PDCJudyLFirstThenNext(JudyL_datafile_list, &datafile_no, &first_then_next))) {
            deol = *PValue;

            bool first_then_next_extent = true;
            Word_t pos = 0;
            while ((PValue = PDCJudyLFirstThenNext(deol->extent_pd_list_by_extent_offset_JudyL, &pos, &first_then_next_extent))) {
                epdl = *PValue;
                internal_fatal(!epdl, "DBENGINE: extent_list is not populated properly");

                // The extent page list can be dispatched to a worker
                // It will need to populate the cache with "acquired" pages that are in the list (pd) only
                // the rest of the extent pages will be added to the cache butnot acquired

                pdc_acquire(pdc); // we do this for the next worker: do_read_extent_work()
                epdl->pdc = pdc;

                if(epdl_pending_add(epdl)) {
                    if (extent_list_no++ == 0)
                        exec_first_extent_list(ctx, epdl, pdc->priority);
                    else
                        exec_rest_extent_list(ctx, epdl, pdc->priority);
                }
            }
            PDCJudyLFreeArray(&deol->extent_pd_list_by_extent_offset_JudyL, PJE0);
            deol_release(deol);
        }
        PDCJudyLFreeArray(&JudyL_datafile_list, PJE0);
    }

    pdc_release_and_destroy_if_unreferenced(pdc, true, true);
}

void collect_page_flags_to_buffer(BUFFER *wb, RRDENG_COLLECT_PAGE_FLAGS flags) {
    if(flags & RRDENG_PAGE_PAST_COLLECTION)
        buffer_strcat(wb, "PAST_COLLECTION ");
    if(flags & RRDENG_PAGE_REPEATED_COLLECTION)
        buffer_strcat(wb, "REPEATED_COLLECTION ");
    if(flags & RRDENG_PAGE_BIG_GAP)
        buffer_strcat(wb, "BIG_GAP ");
    if(flags & RRDENG_PAGE_GAP)
        buffer_strcat(wb, "GAP ");
    if(flags & RRDENG_PAGE_FUTURE_POINT)
        buffer_strcat(wb, "FUTURE_POINT ");
    if(flags & RRDENG_PAGE_CREATED_IN_FUTURE)
        buffer_strcat(wb, "CREATED_IN_FUTURE ");
    if(flags & RRDENG_PAGE_COMPLETED_IN_FUTURE)
        buffer_strcat(wb, "COMPLETED_IN_FUTURE ");
    if(flags & RRDENG_PAGE_UNALIGNED)
        buffer_strcat(wb, "UNALIGNED ");
    if(flags & RRDENG_PAGE_CONFLICT)
        buffer_strcat(wb, "CONFLICT ");
    if(flags & RRDENG_PAGE_FULL)
        buffer_strcat(wb, "PAGE_FULL");
    if(flags & RRDENG_PAGE_COLLECT_FINALIZE)
        buffer_strcat(wb, "COLLECT_FINALIZE");
    if(flags & RRDENG_PAGE_UPDATE_EVERY_CHANGE)
        buffer_strcat(wb, "UPDATE_EVERY_CHANGE");
    if(flags & RRDENG_PAGE_STEP_TOO_SMALL)
        buffer_strcat(wb, "STEP_TOO_SMALL");
    if(flags & RRDENG_PAGE_STEP_UNALIGNED)
        buffer_strcat(wb, "STEP_UNALIGNED");
}

ALWAYS_INLINE VALIDATED_PAGE_DESCRIPTOR validate_extent_page_descr(const struct rrdeng_extent_page_descr *descr, time_t now_s, uint32_t overwrite_zero_update_every_s, bool have_read_error) {
    time_t start_time_s = (time_t) (descr->start_time_ut / USEC_PER_SEC);

    time_t end_time_s = 0;
    size_t entries = 0;

    switch (descr->type) {
        case RRDENG_PAGE_TYPE_ARRAY_32BIT:
        case RRDENG_PAGE_TYPE_ARRAY_TIER1:
            end_time_s = descr->end_time_ut / USEC_PER_SEC;
            entries = 0;
            break;
        case RRDENG_PAGE_TYPE_GORILLA_32BIT:
            end_time_s = start_time_s + descr->gorilla.delta_time_s;
            entries = descr->gorilla.entries;
            break;
        default:
            // Nothing to do. Validate page will notify the user.
            break;
    }

    return validate_page(
            (nd_uuid_t *)descr->uuid,
            start_time_s,
            end_time_s,
            0,
            descr->page_length,
            descr->type,
            entries,
            now_s,
            overwrite_zero_update_every_s,
            have_read_error,
            "loaded", 0);
}

static void validate_page_log(nd_uuid_t *uuid,
                              time_t start_time_s,
                              time_t end_time_s,
                              uint32_t update_every_s,
                              size_t page_length,
                              size_t entries,
                              time_t now_s,
                              const char *msg,
                              RRDENG_COLLECT_PAGE_FLAGS flags,
                              VALIDATED_PAGE_DESCRIPTOR vd) {
#ifndef NETDATA_INTERNAL_CHECKS
    nd_log_limit_static_global_var(erl, 1, 0);
#endif
    char uuid_str[UUID_STR_LEN + 1];
    uuid_unparse(*uuid, uuid_str);

    CLEAN_BUFFER *wb = NULL; // will be automatically freed on function exit

    if(flags) {
        wb = buffer_create(0, NULL);
        collect_page_flags_to_buffer(wb, flags);
    }

    if(!vd.is_valid) {
#ifdef NETDATA_INTERNAL_CHECKS
        internal_error(true,
#else
        nd_log_limit(&erl, NDLS_DAEMON, NDLP_ERR,
#endif
                       "DBENGINE: metric '%s' %s invalid page of type %u "
                       "from %ld to %ld (now %ld), update every %u, page length %zu, entries %zu (flags: %s)",
                       uuid_str, msg, (unsigned)vd.type,
                       (long)vd.start_time_s, (long)vd.end_time_s, (long)now_s, (unsigned)vd.update_every_s, (size_t)vd.page_length, (size_t)vd.entries, wb?buffer_tostring(wb):""
        );
    }
    else {
        CLEAN_BUFFER *log = buffer_create(0, NULL);

        buffer_strcat(log, "DBENGINE: metric '");
        buffer_strcat(log, uuid_str);
        buffer_strcat(log, "' ");
        buffer_strcat(log, msg ? msg : "");
        buffer_strcat(log, " page of type ");
        buffer_print_uint64(log, vd.type);
        buffer_strcat(log, " from ");
        buffer_print_int64(log, vd.start_time_s);
        buffer_strcat(log, " to ");
        buffer_print_int64(log, vd.end_time_s);
        buffer_strcat(log, " (now ");
        buffer_print_int64(log, now_s);
        buffer_strcat(log, "), update every ");
        buffer_print_uint64(log, vd.update_every_s);
        buffer_strcat(log, ", page length ");
        buffer_print_uint64(log, vd.page_length);
        buffer_strcat(log, ", entries ");
        buffer_print_uint64(log, vd.entries);
        buffer_strcat(log, " (flags: ");
        buffer_strcat(log, wb ? buffer_tostring(wb) : "");
        buffer_strcat(log, ")");
        buffer_strcat(log, "found inconsistent - the right is ");
        buffer_print_int64(log, vd.start_time_s);
        buffer_strcat(log, " to ");
        buffer_print_int64(log, vd.end_time_s);
        buffer_strcat(log, ", update every ");
        buffer_print_uint64(log, vd.update_every_s);
        buffer_strcat(log, ", page length ");
        buffer_print_uint64(log, vd.page_length);
        buffer_strcat(log, ", entries ");
        buffer_print_uint64(log, vd.entries);
        buffer_strcat(log, (vd.start_time_s == start_time_s) ? "" : "start time updated, ");
        buffer_strcat(log, (vd.end_time_s == end_time_s) ? "" : "end time updated, ");
        buffer_strcat(log, (vd.update_every_s == update_every_s) ? "" : "update every updated, ");
        buffer_strcat(log, (vd.page_length == page_length) ? "" : "page length updated, ");
        buffer_strcat(log, (vd.entries == entries) ? "" : "entries updated, ");
        buffer_strcat(log, (now_s && vd.end_time_s <= now_s) ? "" : "future end time, ");

#ifdef NETDATA_INTERNAL_CHECKS
        internal_error(true, "%s", buffer_tostring(log));
#else
        nd_log_limit(&erl, NDLS_DAEMON, NDLP_ERR, "%s", buffer_tostring(log));
#endif
    }
}

ALWAYS_INLINE
VALIDATED_PAGE_DESCRIPTOR validate_page(
        nd_uuid_t *uuid,
        time_t start_time_s,
        time_t end_time_s,
        uint32_t update_every_s,                // can be zero, if unknown
        size_t page_length,
        uint8_t page_type,
        size_t entries,                         // can be zero, if unknown
        time_t now_s,                           // can be zero, to disable future timestamp check
        uint32_t overwrite_zero_update_every_s,   // can be zero, if unknown
        bool have_read_error,
        const char *msg,
        RRDENG_COLLECT_PAGE_FLAGS flags)
{
    VALIDATED_PAGE_DESCRIPTOR vd = {
            .start_time_s = start_time_s,
            .end_time_s = end_time_s,
            .update_every_s = update_every_s,
            .page_length = page_length,
            .point_size = page_type_size[page_type],
            .type = page_type,
            .is_valid = true,
    };

    bool known_page_type = true;
    switch (page_type) {
        case RRDENG_PAGE_TYPE_ARRAY_32BIT:
        case RRDENG_PAGE_TYPE_ARRAY_TIER1:
            // always calculate entries by size
            vd.entries = page_entries_by_size(vd.page_length, vd.point_size);

            // allow to be called without entries (when loading pages from disk)
            if(!entries)
                entries = vd.entries;
            break;
        case RRDENG_PAGE_TYPE_GORILLA_32BIT:
            internal_fatal(entries == 0, "0 number of entries found on gorilla page");
            vd.entries = entries;
            break;
        default:
            known_page_type = false;
            break;
    }

    // allow to be called without update every (when loading pages from disk)
    if(!update_every_s) {
        vd.update_every_s = (vd.entries > 1) ? ((uint32_t)(vd.end_time_s - vd.start_time_s) / (vd.entries - 1))
                                             : overwrite_zero_update_every_s;

        update_every_s = vd.update_every_s;
    }

    // another such set of checks exists in
    // update_metric_retention_and_granularity_by_uuid()

    bool updated = false;

    size_t max_page_length = RRDENG_BLOCK_SIZE;

    // If gorilla can not compress the data we might end up needing slightly more
    // than 4KiB. However, gorilla pages extend the page length by increments of
    // 512 bytes.
    max_page_length += ((page_type == RRDENG_PAGE_TYPE_GORILLA_32BIT) * (2 * RRDENG_GORILLA_32BIT_BUFFER_SIZE));

    if (!known_page_type                                        ||
        have_read_error                                         ||
        vd.page_length == 0                                     ||
        vd.page_length > max_page_length                        ||
        vd.start_time_s > vd.end_time_s                         ||
        (now_s && vd.end_time_s > now_s)                        ||
        vd.start_time_s <= 0                                    ||
        vd.end_time_s <= 0                                      ||
        (vd.start_time_s == vd.end_time_s && vd.entries > 1)    ||
        (vd.update_every_s == 0 && vd.entries > 1))
    {
        vd.is_valid = false;
    }
    else {
        if(unlikely(vd.entries != entries || vd.update_every_s != update_every_s))
            updated = true;

        if (likely(vd.update_every_s)) {
            size_t entries_by_time = page_entries_by_time(vd.start_time_s, vd.end_time_s, vd.update_every_s);

            if (vd.entries != entries_by_time) {
                if (overwrite_zero_update_every_s < vd.update_every_s)
                    vd.update_every_s = overwrite_zero_update_every_s;

                time_t new_end_time_s = (time_t)(vd.start_time_s + (vd.entries - 1) * vd.update_every_s);

                if(new_end_time_s <= vd.end_time_s) {
                    // end time is wrong
                    vd.end_time_s = new_end_time_s;
                }
                else {
                    // update every is wrong
                    vd.update_every_s = overwrite_zero_update_every_s;
                    vd.end_time_s = (time_t)(vd.start_time_s + (vd.entries - 1) * vd.update_every_s);
                }

                updated = true;
            }
        }
        else if(overwrite_zero_update_every_s) {
            vd.update_every_s = overwrite_zero_update_every_s;
            updated = true;
        }
    }

    if(unlikely(!vd.is_valid || updated))
        validate_page_log(uuid, start_time_s, end_time_s, update_every_s, page_length, entries, now_s, msg, flags, vd);

    return vd;
}

static ALWAYS_INLINE struct page_details *epdl_get_pd_load_link_list_from_metric_start_time(EPDL *epdl, Word_t metric_id, time_t start_time_s) {

    if(unlikely(epdl->head_to_datafile_extent_queries_pending_for_extent))
        // stop appending more pages to this epdl
        epdl_pending_del(epdl);

    struct page_details *pd_list = NULL;

    for(EPDL *ep = epdl; ep ;ep = ep->query.next) {
        Pvoid_t *pd_by_start_time_s_judyL = PDCJudyLGet(ep->page_details_by_metric_id_JudyL, metric_id, PJE0);
        internal_fatal(pd_by_start_time_s_judyL == PJERR, "DBENGINE: corrupted extent metrics JudyL");

        if (unlikely(pd_by_start_time_s_judyL && *pd_by_start_time_s_judyL)) {
            Pvoid_t *pd_pptr = PDCJudyLGet(*pd_by_start_time_s_judyL, start_time_s, PJE0);
            internal_fatal(pd_pptr == PJERR, "DBENGINE: corrupted metric page details JudyHS");

            if(likely(pd_pptr && *pd_pptr)) {
                struct page_details *pd = *pd_pptr;
                internal_fatal(metric_id != pd->metric_id, "DBENGINE: metric ids do not match");

                if(likely(!pd->page)) {
                    if (unlikely(__atomic_load_n(&ep->pdc->workers_should_stop, __ATOMIC_RELAXED)))
                        pdc_page_status_set(pd, PDC_PAGE_FAILED | PDC_PAGE_CANCELLED);
                    else
                        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(pd_list, pd, load.prev, load.next);
                }
            }
        }
    }

    return pd_list;
}

static void epdl_extent_loading_error_log(struct rrdengine_instance *ctx, EPDL *epdl, struct rrdeng_extent_page_descr *descr, const char *msg, ND_LOG_FIELD_PRIORITY priority) {
    char uuid[UUID_STR_LEN] = "";
    time_t start_time_s = 0;
    time_t end_time_s = 0;
    bool used_epdl = false;
    bool used_descr = false;

    if (descr) {
        start_time_s = (time_t)(descr->start_time_ut / USEC_PER_SEC);
        switch (descr->type) {
            case RRDENG_PAGE_TYPE_ARRAY_32BIT:
            case RRDENG_PAGE_TYPE_ARRAY_TIER1:
                end_time_s = (time_t)(descr->end_time_ut / USEC_PER_SEC);
                break;
            case RRDENG_PAGE_TYPE_GORILLA_32BIT:
                end_time_s = (time_t) start_time_s + (descr->gorilla.delta_time_s);
                break;
        }
        uuid_unparse_lower(descr->uuid, uuid);
        used_descr = true;
    }
    else {
        struct page_details *pd = NULL;

        Word_t start = 0;
        Pvoid_t *pd_by_start_time_s_judyL = PDCJudyLFirst(epdl->page_details_by_metric_id_JudyL, &start, PJE0);
        if(pd_by_start_time_s_judyL) {
            start = 0;
            Pvoid_t *pd_pptr = PDCJudyLFirst(*pd_by_start_time_s_judyL, &start, PJE0);
            if(pd_pptr) {
                pd = *pd_pptr;
                start_time_s = pd->first_time_s;
                end_time_s = pd->last_time_s;
                METRIC *metric = (METRIC *)pd->metric_id;
                nd_uuid_t *u = mrg_metric_uuid(main_mrg, metric);
                uuid_unparse_lower(*u, uuid);
                used_epdl = true;
            }
        }
    }

    if(!used_epdl && !used_descr && epdl->pdc) {
        start_time_s = epdl->pdc->start_time_s;
        end_time_s = epdl->pdc->end_time_s;
    }

    char start_time_str[LOG_DATE_LENGTH + 1] = "";
    if(start_time_s)
        log_date(start_time_str, LOG_DATE_LENGTH, start_time_s);

    char end_time_str[LOG_DATE_LENGTH + 1] = "";
    if(end_time_s)
        log_date(end_time_str, LOG_DATE_LENGTH, end_time_s);

    nd_log_limit_static_global_var(erl, 1, 0);
    nd_log_limit(&erl, NDLS_DAEMON, priority,
                "DBENGINE: error while reading extent from datafile %u of tier %d, at offset %" PRIu64 " (%u bytes) "
                "%s from %ld (%s) to %ld (%s) %s%s: "
                "%s",
                epdl->datafile->fileno, ctx->config.tier,
                BLOCK_TO_OFFSET(epdl->extent_block), epdl->extent_size,
                used_epdl ? "to extract page (PD)" : used_descr ? "expected page (DESCR)" : "part of a query (PDC)",
                start_time_s, start_time_str, end_time_s, end_time_str,
                used_epdl || used_descr ? " of metric " : "",
                used_epdl || used_descr ? uuid : "",
                msg);
}

static bool epdl_populate_pages_from_extent_data(
        struct rrdengine_instance *ctx,
        void *data,
        size_t data_length,
        EPDL *epdl,
        bool worker,
        PDC_PAGE_STATUS tags,
        bool cached_extent)
{
    unsigned i, count;
    void *uncompressed_buf = NULL;
    uint64_t payload_length, payload_offset, trailer_offset;
    uint32_t uncompressed_payload_length = 0;
    bool have_read_error = false;
    /* persistent structures */
    struct rrdeng_df_extent_header *header;
    struct rrdeng_df_extent_trailer *trailer;
    struct extent_buffer *eb = NULL;
    uLong crc;

    bool can_use_data = true;
    if(data_length < sizeof(*header) + sizeof(header->descr[0]) + sizeof(*trailer)) {
        can_use_data = false;

        // added to satisfy the requirements of older compilers (prevent warnings)
        payload_length = 0;
        payload_offset = 0;
        trailer_offset = 0;
        count = 0;
        header = NULL;
        trailer = NULL;
    }
    else {
        header = data;
        payload_length = header->payload_length;
        count = header->number_of_pages;
        payload_offset = sizeof(*header) + sizeof(header->descr[0]) * count;
        trailer_offset = data_length - sizeof(*trailer);
        trailer = data + trailer_offset;
    }

    if( !can_use_data ||
        count < 1 ||
        count > MAX_PAGES_PER_EXTENT ||
        !dbengine_valid_compression_algorithm(header->compression_algorithm) ||
        (payload_length != trailer_offset - payload_offset) ||
        (data_length != payload_offset + payload_length + sizeof(*trailer))
        ) {
        epdl_extent_loading_error_log(ctx, epdl, NULL, "header is INVALID", NDLP_ERR);
        return false;
    }

    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, data, epdl->extent_size - sizeof(*trailer));
    if (unlikely(crc32cmp(trailer->checksum, crc))) {
        ctx_io_error(ctx);
        have_read_error = true;
        epdl_extent_loading_error_log(ctx, epdl, NULL, "CRC32 checksum FAILED", NDLP_ERR);
    }

    if(worker)
        worker_is_busy(UV_EVENT_DBENGINE_EXTENT_DECOMPRESSION);

    if (likely(!have_read_error && RRDENG_COMPRESSION_NONE != header->compression_algorithm)) {
        // find the uncompressed extent size
        uncompressed_payload_length = 0;
        for (i = 0; i < count; ++i) {
            size_t page_length = header->descr[i].page_length;
            if (page_length > RRDENG_BLOCK_SIZE &&
                (header->descr[i].type != RRDENG_PAGE_TYPE_GORILLA_32BIT ||
                 (header->descr[i].type == RRDENG_PAGE_TYPE_GORILLA_32BIT &&
                  (page_length - RRDENG_BLOCK_SIZE) % RRDENG_GORILLA_32BIT_BUFFER_SIZE))) {
                have_read_error = true;
                break;
            }

            uncompressed_payload_length += header->descr[i].page_length;
        }

        if(unlikely(uncompressed_payload_length > MAX_EXTENT_UNCOMPRESSED_SIZE))
            have_read_error = true;

        if(likely(!have_read_error)) {
            eb = extent_buffer_get(uncompressed_payload_length);
            uncompressed_buf = eb->data;

            size_t bytes = dbengine_decompress(uncompressed_buf, data + payload_offset,
                                               uncompressed_payload_length, payload_length,
                                               header->compression_algorithm);

            if(!bytes)
                have_read_error = true;
            else {
                __atomic_add_fetch(&ctx->stats.before_decompress_bytes, payload_length, __ATOMIC_RELAXED);
                __atomic_add_fetch(&ctx->stats.after_decompress_bytes, bytes, __ATOMIC_RELAXED);
            }
        }
    }

    if(worker)
        worker_is_busy(UV_EVENT_DBENGINE_EXTENT_PAGE_LOOKUP);

    size_t stats_data_from_main_cache = 0;
    size_t stats_data_from_extent = 0;
    size_t stats_load_compressed = 0;
    size_t stats_load_uncompressed = 0;
    size_t stats_load_invalid_page = 0;
    size_t stats_cache_hit_while_inserting = 0;

    uint32_t page_offset = 0, page_length;
    time_t now_s = max_acceptable_collected_time();
    for (i = 0; i < count; i++, page_offset += page_length) {
        page_length = header->descr[i].page_length;
        time_t start_time_s = (time_t) (header->descr[i].start_time_ut / USEC_PER_SEC);

        if(!page_length || !start_time_s) {
            char log[200 + 1];
            snprintfz(log, sizeof(log) - 1, "page %u (out of %u) is EMPTY", i, count);
            epdl_extent_loading_error_log(ctx, epdl, &header->descr[i], log, NDLP_ERR);
            continue;
        }

        METRIC *metric = mrg_metric_get_and_acquire_by_uuid(main_mrg, &header->descr[i].uuid, (Word_t)ctx);
        Word_t metric_id = (Word_t)metric;
        if(!metric) {
            char log[200 + 1];
            snprintfz(log, sizeof(log) - 1, "page %u (out of %u) has unknown UUID", i, count);
            epdl_extent_loading_error_log(ctx, epdl, &header->descr[i], log, NDLP_DEBUG);
            continue;
        }
        mrg_metric_release(main_mrg, metric);

        struct page_details *pd_list = epdl_get_pd_load_link_list_from_metric_start_time(epdl, metric_id, start_time_s);
        if(likely(!pd_list))
            continue;

        VALIDATED_PAGE_DESCRIPTOR vd = validate_extent_page_descr(
                &header->descr[i], now_s,
                (pd_list) ? pd_list->update_every_s : 0,
                have_read_error);

        if(worker)
            worker_is_busy(UV_EVENT_DBENGINE_EXTENT_PAGE_ALLOCATION);

        PGD *pgd;

        if (unlikely(!vd.is_valid)) {
            pgd = PGD_EMPTY;
            stats_load_invalid_page++;
        }
        else {
            if (RRDENG_COMPRESSION_NONE == header->compression_algorithm) {
                pgd = pgd_create_from_disk_data(header->descr[i].type,
                                                      data + payload_offset + page_offset,
                                                vd.page_length);
                stats_load_uncompressed++;
            }
            else {
                if (unlikely(page_offset + vd.page_length > uncompressed_payload_length)) {
                    char log[200 + 1];
                    snprintfz(log, sizeof(log) - 1, "page %u (out of %u) offset %u + page length %zu, "
                                        "exceeds the uncompressed buffer size %u",
                                        i, count, page_offset, vd.page_length, uncompressed_payload_length);
                    epdl_extent_loading_error_log(ctx, epdl, &header->descr[i], log, NDLP_ERR);

                    pgd = PGD_EMPTY;
                    stats_load_invalid_page++;
                }
                else {
                    pgd = pgd_create_from_disk_data(header->descr[i].type,
                                                    uncompressed_buf + page_offset,
                                                    vd.page_length);
                    stats_load_compressed++;
                }
            }
        }

        if(worker)
            worker_is_busy(UV_EVENT_DBENGINE_EXTENT_PAGE_POPULATION);

        PGC_ENTRY page_entry = {
                .hot = false,
                .section = (Word_t)ctx,
                .metric_id = metric_id,
                .start_time_s = vd.start_time_s,
                .end_time_s = vd.end_time_s,
                .update_every_s = (uint32_t) vd.update_every_s,
                .size = pgd_memory_footprint(pgd), // the footprint of the entire PGD, for accurate memory management
                .data = pgd,
        };

        bool added = true;
        PGC_PAGE *page = pgc_page_add_and_acquire(main_cache, page_entry, &added);
        if (false == added) {
            pgd_free(pgd);
            pgd = pgc_page_data(page);
            stats_cache_hit_while_inserting++;
            stats_data_from_main_cache++;
        }
        else
            stats_data_from_extent++;

        struct page_details *pd = pd_list;
        do {
            if(pd != pd_list)
                pgc_page_dup(main_cache, page);

            pd->page = page;
            pdc_page_status_set(pd, PDC_PAGE_READY | tags | (pgd_is_empty(pgd) ? PDC_PAGE_EMPTY : 0));

            pd = pd->load.next;
        } while(pd);

        if(worker)
            worker_is_busy(UV_EVENT_DBENGINE_EXTENT_PAGE_LOOKUP);
    }

    if(stats_data_from_main_cache)
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_data_source_main_cache, stats_data_from_main_cache, __ATOMIC_RELAXED);

    if(cached_extent)
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_data_source_extent_cache, stats_data_from_extent, __ATOMIC_RELAXED);
    else {
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_data_source_disk, stats_data_from_extent, __ATOMIC_RELAXED);
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.extents_loaded_from_disk, 1, __ATOMIC_RELAXED);
    }

    if(stats_cache_hit_while_inserting)
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_ok_loaded_but_cache_hit_while_inserting, stats_cache_hit_while_inserting, __ATOMIC_RELAXED);

    if(stats_load_compressed)
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_ok_compressed, stats_load_compressed, __ATOMIC_RELAXED);

    if(stats_load_uncompressed)
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_ok_uncompressed, stats_load_uncompressed, __ATOMIC_RELAXED);

    if(stats_load_invalid_page)
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_fail_invalid_page_in_extent, stats_load_invalid_page, __ATOMIC_RELAXED);

    if(worker)
        worker_is_idle();

    extent_buffer_release(eb);

    return true;
}

static inline void *datafile_extent_read(struct rrdengine_instance *ctx, uv_file file, uint32_t block, unsigned size_bytes)
{
    void *buffer = NULL;
    uv_fs_t request;

    unsigned real_io_size = ALIGN_BYTES_CEILING(size_bytes);
    (void)posix_memalignz(&buffer, RRDFILE_ALIGNMENT, real_io_size);

    uv_buf_t iov = uv_buf_init(buffer, real_io_size);
    int ret = uv_fs_read(NULL, &request, file, &iov, 1, (int64_t) BLOCK_TO_OFFSET(block), NULL);
    if (unlikely(-1 == ret)) {
        ctx_io_error(ctx);
        posix_memalign_freez(buffer);
        buffer = NULL;
    }
    else
        ctx_io_read_op_bytes(ctx, real_io_size);

    uv_fs_req_cleanup(&request);

    return buffer;
}

static inline void datafile_extent_read_free(void *buffer) {
    posix_memalign_freez(buffer);
}

NOT_INLINE_HOT void epdl_find_extent_and_populate_pages(struct rrdengine_instance *ctx, EPDL *epdl, bool worker) {
    if(worker)
        worker_is_busy(UV_EVENT_DBENGINE_EXTENT_CACHE_LOOKUP);

    size_t *statistics_counter = NULL;
    PDC_PAGE_STATUS not_loaded_pages_tag = 0, loaded_pages_tag = 0;

    bool should_stop = __atomic_load_n(&epdl->pdc->workers_should_stop, __ATOMIC_RELAXED);
    for(EPDL *ep = epdl->query.next; ep ;ep = ep->query.next) {
        internal_fatal(ep->datafile != epdl->datafile, "DBENGINE: datafiles do not match");
        internal_fatal(ep->extent_block != epdl->extent_block, "DBENGINE: extent blocks do not match");
        internal_fatal(ep->extent_size != epdl->extent_size, "DBENGINE: extent sizes do not match");

        if(!__atomic_load_n(&ep->pdc->workers_should_stop, __ATOMIC_RELAXED)) {
            should_stop = false;
            break;
        }
    }

    if(unlikely(should_stop)) {
        statistics_counter = &rrdeng_cache_efficiency_stats.pages_load_fail_cancelled;
        not_loaded_pages_tag = PDC_PAGE_CANCELLED;
        goto cleanup;
    }

    bool extent_found_in_cache = false;

    void *extent_compressed_data = NULL;
    PGC_PAGE *extent_cache_page = pgc_page_get_and_acquire(
            extent_cache, (Word_t)ctx,
            (Word_t)epdl->datafile->fileno, (time_t)epdl->extent_block,
            PGC_SEARCH_EXACT);

    if(extent_cache_page) {
        extent_compressed_data = pgc_page_data(extent_cache_page);
        internal_fatal(epdl->extent_size != pgc_page_data_size(extent_cache, extent_cache_page),
                       "DBENGINE: cache size does not match the expected size");

        loaded_pages_tag |= PDC_PAGE_EXTENT_FROM_CACHE;
        not_loaded_pages_tag |= PDC_PAGE_EXTENT_FROM_CACHE;
        extent_found_in_cache = true;
    }
    else {
        if(worker)
            worker_is_busy(UV_EVENT_DBENGINE_EXTENT_MMAP);

        void *extent_data = datafile_extent_read(ctx, epdl->datafile->file, epdl->extent_block, epdl->extent_size);
        if(extent_data != NULL) {

            void *tmp = dbengine_extent_alloc(epdl->extent_size);
            memcpy(tmp, extent_data, epdl->extent_size);
            datafile_extent_read_free(extent_data);
            extent_data = tmp;

            if(worker)
                worker_is_busy(UV_EVENT_DBENGINE_EXTENT_CACHE_LOOKUP);

            bool added = false;
            extent_cache_page = pgc_page_add_and_acquire(extent_cache, (PGC_ENTRY) {
                    .hot = false,
                    .section = (Word_t) ctx,
                    .metric_id = (Word_t) epdl->datafile->fileno,
                    .start_time_s = (time_t) epdl->extent_block,
                    .size = epdl->extent_size,
                    .end_time_s = 0,
                    .update_every_s = 0,
                    .data = extent_data,
            }, &added);

            if (!added) {
                dbengine_extent_free(extent_data, epdl->extent_size);
                internal_fatal(epdl->extent_size != pgc_page_data_size(extent_cache, extent_cache_page),
                               "DBENGINE: cache size does not match the expected size");
            }

            extent_compressed_data = pgc_page_data(extent_cache_page);

            loaded_pages_tag |= PDC_PAGE_EXTENT_FROM_DISK;
            not_loaded_pages_tag |= PDC_PAGE_EXTENT_FROM_DISK;
        }
    }

    if(extent_compressed_data) {
        // Need to decompress and then process the pagelist
        bool extent_used = epdl_populate_pages_from_extent_data(
                ctx, extent_compressed_data, epdl->extent_size,
                epdl, worker, loaded_pages_tag, extent_found_in_cache);

        if(extent_used) {
            // since the extent was used, all the pages that are not
            // loaded from this extent, were not found in the extent
            not_loaded_pages_tag |= PDC_PAGE_FAILED_NOT_IN_EXTENT;
            statistics_counter = &rrdeng_cache_efficiency_stats.pages_load_fail_not_found;
        }
        else {
            not_loaded_pages_tag |= PDC_PAGE_FAILED_INVALID_EXTENT;
            statistics_counter = &rrdeng_cache_efficiency_stats.pages_load_fail_invalid_extent;
        }
    }
    else {
        not_loaded_pages_tag |= PDC_PAGE_FAILED_TO_MAP_EXTENT;
        statistics_counter = &rrdeng_cache_efficiency_stats.pages_load_fail_cant_mmap_extent;
    }

    if(extent_cache_page)
        pgc_page_release(extent_cache, extent_cache_page);

cleanup:
    // remove it from the datafile extent_queries
    // this can be called multiple times safely
    epdl_pending_del(epdl);

    // mark all pending pages as failed
    for(EPDL *ep = epdl; ep ;ep = ep->query.next) {
        epdl_mark_all_not_loaded_pages_as_failed(
                ep, not_loaded_pages_tag, statistics_counter);
    }

    for(EPDL *ep = epdl, *next = NULL; ep ; ep = next) {
        next = ep->query.next;

        completion_mark_complete_a_job(&ep->pdc->page_completion);
        pdc_release_and_destroy_if_unreferenced(ep->pdc, true, false);

        // Free the Judy that holds the requested pagelist and the extents
        epdl_destroy(ep);
    }

    if(worker)
        worker_is_idle();
}

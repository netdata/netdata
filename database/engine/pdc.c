// SPDX-License-Identifier: GPL-3.0-or-later
#define NETDATA_RRD_INTERNALS
#include "pdc.h"

struct extent_page_details_list {
    uv_file file;
    uint64_t extent_offset;
    uint32_t extent_size;
    unsigned number_of_pages_in_JudyL;
    Pvoid_t page_details_by_metric_id_JudyL;
    struct page_details_control *pdc;
    struct rrdengine_datafile *datafile;

    struct rrdeng_cmd *cmd;
    bool head_to_datafile_extent_queries_pending_for_extent;

    struct {
        struct extent_page_details_list *prev;
        struct extent_page_details_list *next;
    } query;

    struct {
        struct extent_page_details_list *prev;
        struct extent_page_details_list *next;
    } cache;
};

typedef struct datafile_extent_offset_list {
    uv_file file;
    unsigned fileno;
    Pvoid_t extent_pd_list_by_extent_offset_JudyL;

    struct {
        struct datafile_extent_offset_list *prev;
        struct datafile_extent_offset_list *next;
    } cache;
} DEOL;

// ----------------------------------------------------------------------------
// PDC cache

static struct {
    struct {
        SPINLOCK spinlock;
        PDC *available_items;
        size_t available;
    } protected;

    struct {
        size_t allocated;
    } atomics;
} pdc_globals = {
        .protected = {
                .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                .available_items = NULL,
                .available = 0,
        },
        .atomics = {
                .allocated = 0,
        },
};

void pdc_cleanup1(void) {
    PDC *item = NULL;

    if(!netdata_spinlock_trylock(&pdc_globals.protected.spinlock))
        return;

    if(pdc_globals.protected.available_items && pdc_globals.protected.available > (size_t)libuv_worker_threads) {
        item = pdc_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(pdc_globals.protected.available_items, item, cache.prev, cache.next);
        pdc_globals.protected.available--;
    }

    netdata_spinlock_unlock(&pdc_globals.protected.spinlock);

    if(item) {
        freez(item);
        __atomic_sub_fetch(&pdc_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
    }
}

PDC *pdc_get(void) {
    PDC *pdc = NULL;

    netdata_spinlock_lock(&pdc_globals.protected.spinlock);

    if(likely(pdc_globals.protected.available_items)) {
        pdc = pdc_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(pdc_globals.protected.available_items, pdc, cache.prev, cache.next);
        pdc_globals.protected.available--;
    }

    netdata_spinlock_unlock(&pdc_globals.protected.spinlock);

    if(unlikely(!pdc)) {
        pdc = mallocz(sizeof(PDC));
        __atomic_add_fetch(&pdc_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
    }

    memset(pdc, 0, sizeof(PDC));
    return pdc;
}

static void pdc_release(PDC *pdc) {
    if(unlikely(!pdc)) return;

    netdata_spinlock_lock(&pdc_globals.protected.spinlock);
    DOUBLE_LINKED_LIST_APPEND_UNSAFE(pdc_globals.protected.available_items, pdc, cache.prev, cache.next);
    pdc_globals.protected.available++;
    netdata_spinlock_unlock(&pdc_globals.protected.spinlock);
}

size_t pdc_cache_size(void) {
    return __atomic_load_n(&pdc_globals.atomics.allocated, __ATOMIC_RELAXED) * sizeof(PDC);
}

// ----------------------------------------------------------------------------
// PD cache

static struct {
    struct {
        SPINLOCK spinlock;
        struct page_details *available_items;
        size_t available;
    } protected;

    struct {
        size_t allocated;
    } atomics;
} page_details_globals = {
        .protected = {
                .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                .available_items = NULL,
                .available = 0,
        },
        .atomics = {
                .allocated = 0,
        },
};

void page_details_cleanup1(void) {
    struct page_details *item = NULL;

    if(!netdata_spinlock_trylock(&page_details_globals.protected.spinlock))
        return;

    if(page_details_globals.protected.available_items && page_details_globals.protected.available > (size_t)libuv_worker_threads * 2) {
        item = page_details_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(page_details_globals.protected.available_items, item, cache.prev, cache.next);
        page_details_globals.protected.available--;
    }

    netdata_spinlock_unlock(&page_details_globals.protected.spinlock);

    if(item) {
        freez(item);
        __atomic_sub_fetch(&page_details_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
    }
}

struct page_details *page_details_get(void) {
    struct page_details *pd = NULL;

    netdata_spinlock_lock(&page_details_globals.protected.spinlock);

    if(likely(page_details_globals.protected.available_items)) {
        pd = page_details_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(page_details_globals.protected.available_items, pd, cache.prev, cache.next);
        page_details_globals.protected.available--;
    }

    netdata_spinlock_unlock(&page_details_globals.protected.spinlock);

    if(unlikely(!pd)) {
        pd = mallocz(sizeof(struct page_details));
        __atomic_add_fetch(&page_details_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
    }

    memset(pd, 0, sizeof(struct page_details));
    return pd;
}

static void page_details_release(struct page_details *pd) {
    if(unlikely(!pd)) return;

    netdata_spinlock_lock(&page_details_globals.protected.spinlock);
    DOUBLE_LINKED_LIST_APPEND_UNSAFE(page_details_globals.protected.available_items, pd, cache.prev, cache.next);
    page_details_globals.protected.available++;
    netdata_spinlock_unlock(&page_details_globals.protected.spinlock);
}

size_t pd_cache_size(void) {
    return __atomic_load_n(&page_details_globals.atomics.allocated, __ATOMIC_RELAXED) * sizeof(struct page_details);
}

// ----------------------------------------------------------------------------
// epdl cache

static struct {
    struct {
        SPINLOCK spinlock;
        EPDL *available_items;
        size_t available;
    } protected;

    struct {
        size_t allocated;
    } atomics;
} epdl_globals = {
        .protected = {
                .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                .available_items = NULL,
                .available = 0,
        },
        .atomics = {
                .allocated = 0,
        },
};

void epdl_cleanup1(void) {
    EPDL *item = NULL;

    if(!netdata_spinlock_trylock(&epdl_globals.protected.spinlock))
        return;

    if(epdl_globals.protected.available_items && epdl_globals.protected.available > 100) {
        item = epdl_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(epdl_globals.protected.available_items, item, cache.prev, cache.next);
        epdl_globals.protected.available--;
    }

    netdata_spinlock_unlock(&epdl_globals.protected.spinlock);

    if(item) {
        freez(item);
        __atomic_sub_fetch(&epdl_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
    }
}

static EPDL *epdl_get(void) {
    EPDL *epdl = NULL;

    netdata_spinlock_lock(&epdl_globals.protected.spinlock);

    if(likely(epdl_globals.protected.available_items)) {
        epdl = epdl_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(epdl_globals.protected.available_items, epdl, cache.prev, cache.next);
        epdl_globals.protected.available--;
    }

    netdata_spinlock_unlock(&epdl_globals.protected.spinlock);

    if(unlikely(!epdl)) {
        epdl = mallocz(sizeof(EPDL));
        __atomic_add_fetch(&epdl_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
    }

    memset(epdl, 0, sizeof(EPDL));
    return epdl;
}

static void epdl_release(EPDL *epdl) {
    if(unlikely(!epdl)) return;

    netdata_spinlock_lock(&epdl_globals.protected.spinlock);
    DOUBLE_LINKED_LIST_APPEND_UNSAFE(epdl_globals.protected.available_items, epdl, cache.prev, cache.next);
    epdl_globals.protected.available++;
    netdata_spinlock_unlock(&epdl_globals.protected.spinlock);
}

size_t epdl_cache_size(void) {
    return __atomic_load_n(&epdl_globals.atomics.allocated, __ATOMIC_RELAXED) * sizeof(EPDL);
}

// ----------------------------------------------------------------------------
// deol cache

static struct {
    struct {
        SPINLOCK spinlock;
        DEOL *available_items;
        size_t available;
    } protected;

    struct {
        size_t allocated;
    } atomics;
} deol_globals = {
        .protected = {
                .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                .available_items = NULL,
                .available = 0,
        },
        .atomics = {
                .allocated = 0,
        },
};

void deol_cleanup1(void) {
    DEOL *item = NULL;

    if(!netdata_spinlock_trylock(&deol_globals.protected.spinlock))
        return;

    if(deol_globals.protected.available_items && deol_globals.protected.available > 100) {
        item = deol_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(deol_globals.protected.available_items, item, cache.prev, cache.next);
        deol_globals.protected.available--;
    }

    netdata_spinlock_unlock(&deol_globals.protected.spinlock);

    if(item) {
        freez(item);
        __atomic_sub_fetch(&deol_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
    }
}

static DEOL *deol_get(void) {
    DEOL *deol = NULL;

    netdata_spinlock_lock(&deol_globals.protected.spinlock);

    if(likely(deol_globals.protected.available_items)) {
        deol = deol_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(deol_globals.protected.available_items, deol, cache.prev, cache.next);
        deol_globals.protected.available--;
    }

    netdata_spinlock_unlock(&deol_globals.protected.spinlock);

    if(unlikely(!deol)) {
        deol = mallocz(sizeof(DEOL));
        __atomic_add_fetch(&deol_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
    }

    memset(deol, 0, sizeof(DEOL));
    return deol;
}

static void deol_release(DEOL *deol) {
    if(unlikely(!deol)) return;

    netdata_spinlock_lock(&deol_globals.protected.spinlock);
    DOUBLE_LINKED_LIST_APPEND_UNSAFE(deol_globals.protected.available_items, deol, cache.prev, cache.next);
    deol_globals.protected.available++;
    netdata_spinlock_unlock(&deol_globals.protected.spinlock);
}

size_t deol_cache_size(void) {
    return __atomic_load_n(&deol_globals.atomics.allocated, __ATOMIC_RELAXED) * sizeof(DEOL);
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
                .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                .available_items = NULL,
                .available = 0,
        },
        .atomics = {
                .allocated = 0,
                .allocated_bytes = 0,
        },
        .max_size = MAX_PAGES_PER_EXTENT * RRDENG_BLOCK_SIZE,
};

void extent_buffer_init(void) {
    size_t max_extent_uncompressed = MAX_PAGES_PER_EXTENT * RRDENG_BLOCK_SIZE;
    size_t max_size = (size_t)LZ4_compressBound(MAX_PAGES_PER_EXTENT * RRDENG_BLOCK_SIZE);
    if(max_size < max_extent_uncompressed)
        max_size = max_extent_uncompressed;

    extent_buffer_globals.max_size = max_size;
}

void extent_buffer_cleanup1(void) {
    struct extent_buffer *item = NULL;

    if(!netdata_spinlock_trylock(&extent_buffer_globals.protected.spinlock))
        return;

    if(extent_buffer_globals.protected.available_items && extent_buffer_globals.protected.available > 1) {
        item = extent_buffer_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(extent_buffer_globals.protected.available_items, item, cache.prev, cache.next);
        extent_buffer_globals.protected.available--;
    }

    netdata_spinlock_unlock(&extent_buffer_globals.protected.spinlock);

    if(item) {
        size_t bytes = sizeof(struct extent_buffer) + item->bytes;
        freez(item);
        __atomic_sub_fetch(&extent_buffer_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
        __atomic_sub_fetch(&extent_buffer_globals.atomics.allocated_bytes, bytes, __ATOMIC_RELAXED);
    }
}

struct extent_buffer *extent_buffer_get(size_t size) {
    internal_fatal(size > extent_buffer_globals.max_size, "DBENGINE: extent size is too big");

    struct extent_buffer *eb = NULL;

    if(size < extent_buffer_globals.max_size)
        size = extent_buffer_globals.max_size;

    netdata_spinlock_lock(&extent_buffer_globals.protected.spinlock);
    if(likely(extent_buffer_globals.protected.available_items)) {
        eb = extent_buffer_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(extent_buffer_globals.protected.available_items, eb, cache.prev, cache.next);
        extent_buffer_globals.protected.available--;
    }
    netdata_spinlock_unlock(&extent_buffer_globals.protected.spinlock);

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

void extent_buffer_release(struct extent_buffer *eb) {
    if(unlikely(!eb)) return;

    netdata_spinlock_lock(&extent_buffer_globals.protected.spinlock);
    DOUBLE_LINKED_LIST_APPEND_UNSAFE(extent_buffer_globals.protected.available_items, eb, cache.prev, cache.next);
    extent_buffer_globals.protected.available++;
    netdata_spinlock_unlock(&extent_buffer_globals.protected.spinlock);
}

size_t extent_buffer_cache_size(void) {
    return __atomic_load_n(&extent_buffer_globals.atomics.allocated_bytes, __ATOMIC_RELAXED);
}

// ----------------------------------------------------------------------------
// epdl logic

static void epdl_destroy(EPDL *epdl)
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

static void epdl_mark_all_not_loaded_pages_as_failed(EPDL *epdl, PDC_PAGE_STATUS tags, size_t *statistics_counter)
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

static void pdc_destroy(PDC *pdc) {
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

void pdc_acquire(PDC *pdc) {
    netdata_spinlock_lock(&pdc->refcount_spinlock);

    if(pdc->refcount < 1)
        fatal("DBENGINE: pdc is not referenced and cannot be acquired");

    pdc->refcount++;
    netdata_spinlock_unlock(&pdc->refcount_spinlock);
}

bool pdc_release_and_destroy_if_unreferenced(PDC *pdc, bool worker, bool router __maybe_unused) {
    netdata_spinlock_lock(&pdc->refcount_spinlock);

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
        netdata_spinlock_unlock(&pdc->refcount_spinlock);
        pdc_destroy(pdc);
        return true;
    }

    netdata_spinlock_unlock(&pdc->refcount_spinlock);
    return false;
}

void epdl_cmd_queued(void *epdl_ptr, struct rrdeng_cmd *cmd) {
    EPDL *epdl = epdl_ptr;
    epdl->cmd = cmd;
}

void epdl_cmd_dequeued(void *epdl_ptr) {
    EPDL *epdl = epdl_ptr;
    epdl->cmd = NULL;
}

static struct rrdeng_cmd *epdl_get_cmd(void *epdl_ptr) {
    EPDL *epdl = epdl_ptr;
    return epdl->cmd;
}

static bool epdl_pending_add(EPDL *epdl) {
    bool added_new;

    netdata_spinlock_lock(&epdl->datafile->extent_queries.spinlock);
    Pvoid_t *PValue = JudyLIns(&epdl->datafile->extent_queries.pending_epdl_by_extent_offset_judyL, epdl->extent_offset, PJE0);
    internal_fatal(!PValue || PValue == PJERR, "DBENGINE: corrupted pending extent judy");

    EPDL *base = *PValue;

    if(!base) {
        added_new = true;
        epdl->head_to_datafile_extent_queries_pending_for_extent = true;
    }
    else {
        added_new = false;
        epdl->head_to_datafile_extent_queries_pending_for_extent = false;
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_extent_merged, 1, __ATOMIC_RELAXED);

        if(base->pdc->priority > epdl->pdc->priority)
            rrdeng_req_cmd(epdl_get_cmd, base, epdl->pdc->priority);
    }

    DOUBLE_LINKED_LIST_APPEND_UNSAFE(base, epdl, query.prev, query.next);
    *PValue = base;

    netdata_spinlock_unlock(&epdl->datafile->extent_queries.spinlock);

    return added_new;
}

static void epdl_pending_del(EPDL *epdl) {
    netdata_spinlock_lock(&epdl->datafile->extent_queries.spinlock);
    if(epdl->head_to_datafile_extent_queries_pending_for_extent) {
        epdl->head_to_datafile_extent_queries_pending_for_extent = false;
        int rc = JudyLDel(&epdl->datafile->extent_queries.pending_epdl_by_extent_offset_judyL, epdl->extent_offset, PJE0);
        (void) rc;
        internal_fatal(!rc, "DBENGINE: epdl not found in pending list");
    }
    netdata_spinlock_unlock(&epdl->datafile->extent_queries.spinlock);
}

void pdc_to_epdl_router(struct rrdengine_instance *ctx, PDC *pdc, execute_extent_page_details_list_t exec_first_extent_list, execute_extent_page_details_list_t exec_rest_extent_list)
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

            PValue1 = PDCJudyLIns(&JudyL_datafile_list, pd->datafile.fileno, PJE0);
            if (PValue1 && !*PValue1) {
                *PValue1 = deol = deol_get();
                deol->extent_pd_list_by_extent_offset_JudyL = NULL;
                deol->fileno = pd->datafile.fileno;
            }
            else
                deol = *PValue1;

            PValue2 = PDCJudyLIns(&deol->extent_pd_list_by_extent_offset_JudyL, pd->datafile.extent.pos, PJE0);
            if (PValue2 && !*PValue2) {
                *PValue2 = epdl = epdl_get();
                epdl->page_details_by_metric_id_JudyL = NULL;
                epdl->number_of_pages_in_JudyL = 0;
                epdl->file = pd->datafile.file;
                epdl->extent_offset = pd->datafile.extent.pos;
                epdl->extent_size = pd->datafile.extent.bytes;
                epdl->datafile = pd->datafile.ptr;
            }
            else
                epdl = *PValue2;

            epdl->number_of_pages_in_JudyL++;

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

static void fill_page_with_nulls(void *page, uint32_t page_length, uint8_t type) {
    switch(type) {
        case PAGE_METRICS: {
            storage_number n = pack_storage_number(NAN, SN_FLAG_NONE);
            storage_number *array = (storage_number *)page;
            size_t slots = page_length / sizeof(n);
            for(size_t i = 0; i < slots ; i++)
                array[i] = n;
        }
            break;

        case PAGE_TIER: {
            storage_number_tier1_t n = {
                    .min_value = NAN,
                    .max_value = NAN,
                    .sum_value = NAN,
                    .count = 1,
                    .anomaly_count = 0,
            };
            storage_number_tier1_t *array = (storage_number_tier1_t *)page;
            size_t slots = page_length / sizeof(n);
            for(size_t i = 0; i < slots ; i++)
                array[i] = n;
        }
            break;

        default: {
            static bool logged = false;
            if(!logged) {
                error("DBENGINE: cannot fill page with nulls on unknown page type id %d", type);
                logged = true;
            }
            memset(page, 0, page_length);
        }
    }
}

inline VALIDATED_PAGE_DESCRIPTOR validate_extent_page_descr(const struct rrdeng_extent_page_descr *descr, time_t now_s, time_t overwrite_zero_update_every_s, bool have_read_error) {
    VALIDATED_PAGE_DESCRIPTOR vd = {
            .start_time_s = (time_t) (descr->start_time_ut / USEC_PER_SEC),
            .end_time_s = (time_t) (descr->end_time_ut / USEC_PER_SEC),
            .page_length = descr->page_length,
            .type = descr->type,
    };
    vd.point_size = page_type_size[vd.type];
    vd.entries = page_entries_by_size(vd.page_length, vd.point_size);
    vd.update_every_s = (vd.entries > 1) ? ((vd.end_time_s - vd.start_time_s) / (time_t)(vd.entries - 1)) : overwrite_zero_update_every_s;

    bool is_valid = true;

    // another such set of checks exists in
    // update_metric_retention_and_granularity_by_uuid()

    if( have_read_error                                         ||
        vd.page_length == 0                                     ||
        vd.page_length > RRDENG_BLOCK_SIZE                      ||
        vd.start_time_s > vd.end_time_s                         ||
        vd.end_time_s > now_s                                   ||
        vd.start_time_s == 0                                    ||
        vd.end_time_s == 0                                      ||
        (vd.start_time_s == vd.end_time_s && vd.entries > 1)    ||
        (vd.update_every_s == 0 && vd.entries > 1)
            ) {
        is_valid = false;

        error_limit_static_global_var(erl, 1, 0);
        error_limit(&erl, "DBENGINE: ignoring invalid page of type %u from %ld to %ld (now %ld), update every %ld, page length %zu, point size %zu, entries %zu.",
                    vd.type, vd.start_time_s, vd.end_time_s, now_s, vd.update_every_s, vd.page_length, vd.point_size, vd.entries);
    }
    else {
        if (vd.update_every_s) {
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
            }
        }
        else
            vd.update_every_s = overwrite_zero_update_every_s;
    }

    if(!is_valid) {
        if(vd.start_time_s == vd.end_time_s) {
            vd.page_length = vd.point_size;
            vd.entries = 1;
        }
        else {
            vd.page_length = vd.point_size * 2;
            vd.update_every_s = vd.end_time_s - vd.start_time_s;
            vd.entries = 2;
        }
    }

    vd.data_on_disk_valid = is_valid;
    return vd;
}

static inline struct page_details *epdl_get_pd_load_link_list_from_metric_start_time(EPDL *epdl, Word_t metric_id, time_t start_time_s) {

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
                        DOUBLE_LINKED_LIST_APPEND_UNSAFE(pd_list, pd, load.prev, load.next);
                }
            }
        }
    }

    return pd_list;
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
    int ret;
    unsigned i, count;
    void *uncompressed_buf = NULL;
    uint32_t payload_length, payload_offset, trailer_offset, uncompressed_payload_length = 0;
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
        (header->compression_algorithm != RRD_NO_COMPRESSION && header->compression_algorithm != RRD_LZ4) ||
        (payload_length != trailer_offset - payload_offset) ||
        (data_length != payload_offset + payload_length + sizeof(*trailer))
            ) {

        error_limit_static_global_var(erl, 1, 0);
        error_limit(&erl, "%s: Extent at offset %"PRIu64" (%u bytes) was read from datafile %u, but header is INVALID", __func__,
                    epdl->extent_offset, epdl->extent_size, epdl->datafile->fileno);

        return false;
    }

    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, data, epdl->extent_size - sizeof(*trailer));
    ret = crc32cmp(trailer->checksum, crc);
    if (unlikely(ret)) {
        ++ctx->stats.io_errors;
        rrd_stat_atomic_add(&global_io_errors, 1);
        have_read_error = true;

        error_limit_static_global_var(erl, 1, 0);
        error_limit(&erl, "%s: Extent at offset %"PRIu64" (%u bytes) was read from datafile %u, but CRC32 check FAILED", __func__,
                    epdl->extent_offset, epdl->extent_size, epdl->datafile->fileno);
    }

    if(worker)
        worker_is_busy(UV_EVENT_EXT_DECOMPRESSION);

    if (likely(!have_read_error && RRD_NO_COMPRESSION != header->compression_algorithm)) {
        // find the uncompressed extent size
        uncompressed_payload_length = 0;
        for (i = 0; i < count; ++i) {
            size_t page_length = header->descr[i].page_length;
            if(page_length > RRDENG_BLOCK_SIZE) {
                have_read_error = true;
                break;
            }

            uncompressed_payload_length += header->descr[i].page_length;
        }

        if(unlikely(uncompressed_payload_length > MAX_PAGES_PER_EXTENT * RRDENG_BLOCK_SIZE))
            have_read_error = true;

        if(likely(!have_read_error)) {
            eb = extent_buffer_get(uncompressed_payload_length);
            uncompressed_buf = eb->data;

            ret = LZ4_decompress_safe(data + payload_offset, uncompressed_buf,
                                      (int) payload_length, (int) uncompressed_payload_length);
            ctx->stats.before_decompress_bytes += payload_length;
            ctx->stats.after_decompress_bytes += ret;
            debug(D_RRDENGINE, "LZ4 decompressed %u bytes to %d bytes.", payload_length, ret);
        }
    }

    if(worker)
        worker_is_busy(UV_EVENT_PAGE_LOOKUP);

    size_t stats_data_from_main_cache = 0;
    size_t stats_data_from_extent = 0;
    size_t stats_load_compressed = 0;
    size_t stats_load_uncompressed = 0;
    size_t stats_load_invalid_page = 0;
    size_t stats_cache_hit_while_inserting = 0;

    uint32_t page_offset = 0, page_length;
    time_t now_s = now_realtime_sec();
    for (i = 0; i < count; i++, page_offset += page_length) {
        page_length = header->descr[i].page_length;
        time_t start_time_s = (time_t) (header->descr[i].start_time_ut / USEC_PER_SEC);

        if(!page_length || !start_time_s) {
            error_limit_static_global_var(erl, 1, 0);
            error_limit(&erl, "%s: Extent at offset %"PRIu64" (%u bytes) was read from datafile %u, having page %u (out of %u) EMPTY",
                        __func__, epdl->extent_offset, epdl->extent_size, epdl->datafile->fileno, i, count);
            continue;
        }

        METRIC *metric = mrg_metric_get_and_acquire(main_mrg, &header->descr[i].uuid, (Word_t)ctx);
        Word_t metric_id = (Word_t)metric;
        if(!metric) {
            error_limit_static_global_var(erl, 1, 0);
            error_limit(&erl, "%s: Extent at offset %"PRIu64" (%u bytes) was read from datafile %u, having page %u (out of %u) for unknown UUID",
                        __func__, epdl->extent_offset, epdl->extent_size, epdl->datafile->fileno, i, count);
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
            worker_is_busy(UV_EVENT_PAGE_POPULATION);

        void *page_data = dbengine_page_alloc(vd.page_length);

        if (unlikely(!vd.data_on_disk_valid)) {
            fill_page_with_nulls(page_data, vd.page_length, vd.type);
            stats_load_invalid_page++;
        }

        else if (RRD_NO_COMPRESSION == header->compression_algorithm) {
            memcpy(page_data, data + payload_offset + page_offset, (size_t) vd.page_length);
            stats_load_uncompressed++;
        }

        else {
            if(unlikely(page_offset + vd.page_length > uncompressed_payload_length)) {
                error_limit_static_global_var(erl, 10, 0);
                error_limit(&erl,
                            "DBENGINE: page %u offset %u + page length %zu exceeds the uncompressed buffer size %u",
                            i, page_offset, vd.page_length, uncompressed_payload_length);

                fill_page_with_nulls(page_data, vd.page_length, vd.type);
                stats_load_invalid_page++;
            }
            else {
                memcpy(page_data, uncompressed_buf + page_offset, vd.page_length);
                stats_load_compressed++;
            }
        }

        PGC_ENTRY page_entry = {
                .hot = false,
                .section = (Word_t)ctx,
                .metric_id = metric_id,
                .start_time_s = vd.start_time_s,
                .end_time_s = vd.end_time_s,
                .update_every_s = vd.update_every_s,
                .size = (size_t) vd.page_length,
                .data = page_data
        };

        bool added = true;
        PGC_PAGE *page = pgc_page_add_and_acquire(main_cache, page_entry, &added);
        if (false == added) {
            dbengine_page_free(page_data, vd.page_length);
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
            pd->page_length = pgc_page_data_size(main_cache, page);
            pdc_page_status_set(pd, PDC_PAGE_READY | tags);

            pd = pd->load.next;
        } while(pd);

        if(worker)
            worker_is_busy(UV_EVENT_PAGE_LOOKUP);
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

void epdl_find_extent_and_populate_pages(struct rrdengine_instance *ctx, EPDL *epdl, bool worker) {
    size_t *statistics_counter = NULL;
    PDC_PAGE_STATUS not_loaded_pages_tag = 0, loaded_pages_tag = 0;

    bool should_stop = __atomic_load_n(&epdl->pdc->workers_should_stop, __ATOMIC_RELAXED);
    for(EPDL *ep = epdl->query.next; ep ;ep = ep->query.next) {
        internal_fatal(ep->datafile != epdl->datafile, "DBENGINE: datafiles do not match");
        internal_fatal(ep->extent_offset != epdl->extent_offset, "DBENGINE: extent offsets do not match");
        internal_fatal(ep->extent_size != epdl->extent_size, "DBENGINE: extent sizes do not match");
        internal_fatal(ep->file != epdl->file, "DBENGINE: files do not match");

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

    if(worker)
        worker_is_busy(UV_EVENT_EXTENT_CACHE);

    bool extent_found_in_cache = false;

    void *extent_compressed_data = NULL;
    PGC_PAGE *extent_cache_page = pgc_page_get_and_acquire(
            extent_cache, (Word_t)ctx,
            (Word_t)epdl->datafile->fileno, (time_t)epdl->extent_offset,
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
            worker_is_busy(UV_EVENT_EXTENT_MMAP);

        off_t map_start =  ALIGN_BYTES_FLOOR(epdl->extent_offset);
        size_t length = ALIGN_BYTES_CEILING(epdl->extent_offset + epdl->extent_size) - map_start;

        void *mmap_data = mmap(NULL, length, PROT_READ, MAP_SHARED, epdl->file, map_start);
        if(mmap_data != MAP_FAILED) {
            extent_compressed_data = mmap_data + (epdl->extent_offset - map_start);

            void *copied_extent_compressed_data = dbengine_extent_alloc(epdl->extent_size);
            memcpy(copied_extent_compressed_data, extent_compressed_data, epdl->extent_size);

            int ret = munmap(mmap_data, length);
            fatal_assert(0 == ret);

            if(worker)
                worker_is_busy(UV_EVENT_EXTENT_CACHE);

            bool added = false;
            extent_cache_page = pgc_page_add_and_acquire(extent_cache, (PGC_ENTRY) {
                    .hot = false,
                    .section = (Word_t) ctx,
                    .metric_id = (Word_t) epdl->datafile->fileno,
                    .start_time_s = (time_t) epdl->extent_offset,
                    .size = epdl->extent_size,
                    .end_time_s = 0,
                    .update_every_s = 0,
                    .data = copied_extent_compressed_data,
            }, &added);

            if (!added) {
                dbengine_extent_free(copied_extent_compressed_data, epdl->extent_size);
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

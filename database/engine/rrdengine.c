// SPDX-License-Identifier: GPL-3.0-or-later
#define NETDATA_RRD_INTERNALS

#include "rrdengine.h"

rrdeng_stats_t global_io_errors = 0;
rrdeng_stats_t global_fs_errors = 0;
rrdeng_stats_t rrdeng_reserved_file_descriptors = 0;
rrdeng_stats_t global_pg_cache_over_half_dirty_events = 0;
rrdeng_stats_t global_flushing_pressure_page_deletions = 0;

unsigned rrdeng_pages_per_extent = MAX_PAGES_PER_EXTENT;

struct rrdeng_main {
    uv_thread_t thread;
    uv_loop_t loop;
    uv_async_t async;
    uv_timer_t timer;
    pid_t tid;

    bool flush_running;
    bool evict_running;
} rrdeng_main = {
        .thread = 0,
        .loop = {},
        .async = {},
        .timer = {},
        .flush_running = false,
};

static void sanity_check(void)
{
    BUILD_BUG_ON(WORKER_UTILIZATION_MAX_JOB_TYPES < (RRDENG_OPCODE_MAX + 2));

    /* Magic numbers must fit in the super-blocks */
    BUILD_BUG_ON(strlen(RRDENG_DF_MAGIC) > RRDENG_MAGIC_SZ);
    BUILD_BUG_ON(strlen(RRDENG_JF_MAGIC) > RRDENG_MAGIC_SZ);

    /* Version strings must fit in the super-blocks */
    BUILD_BUG_ON(strlen(RRDENG_DF_VER) > RRDENG_VER_SZ);
    BUILD_BUG_ON(strlen(RRDENG_JF_VER) > RRDENG_VER_SZ);

    /* Data file super-block cannot be larger than RRDENG_BLOCK_SIZE */
    BUILD_BUG_ON(RRDENG_DF_SB_PADDING_SZ < 0);

    BUILD_BUG_ON(sizeof(uuid_t) != UUID_SZ); /* check UUID size */

    /* page count must fit in 8 bits */
    BUILD_BUG_ON(MAX_PAGES_PER_EXTENT > 255);

    /* extent cache count must fit in 32 bits */
//    BUILD_BUG_ON(MAX_CACHED_EXTENTS > 32);

    /* page info scratch space must be able to hold 2 32-bit integers */
    BUILD_BUG_ON(sizeof(((struct rrdeng_page_info *)0)->scratch) < 2 * sizeof(uint32_t));
}

typedef struct datafile_extent_list_s {
    uv_file file;
    unsigned fileno;
    Pvoid_t extent_pd_list_by_extent_offset_JudyL;
} DATAFILE_EXTENT_PD_LIST;

typedef struct extent_page_list_s {
    uv_file file;
    uint64_t extent_offset;
    uint32_t extent_size;
    unsigned number_of_pages_in_JudyL;
    Pvoid_t page_details_by_metric_id_JudyL;
    struct page_details_control *pdc;
    struct rrdengine_datafile *datafile;
} EXTENT_PD_LIST;

#if WORKER_UTILIZATION_MAX_JOB_TYPES < (RRDENG_OPCODE_MAX + 2)
#error Please increase WORKER_UTILIZATION_MAX_JOB_TYPES to at least (RRDENG_MAX_OPCODE + 2)
#endif


// ----------------------------------------------------------------------------
// extent page details list

void extent_list_free(EXTENT_PD_LIST *epdl)
{
    Pvoid_t *pd_by_start_time_s_JudyL;
    Word_t metric_id_index = 0;
    bool metric_id_first = true;
    while ((pd_by_start_time_s_JudyL = JudyLFirstThenNext(
            epdl->page_details_by_metric_id_JudyL,
            &metric_id_index, &metric_id_first)))
        JudyLFreeArray(pd_by_start_time_s_JudyL, PJE0);

    JudyLFreeArray(&epdl->page_details_by_metric_id_JudyL, PJE0);
    freez(epdl);
}

static void extent_list_mark_all_not_loaded_pages_as_failed(EXTENT_PD_LIST *epdl, PDC_PAGE_STATUS tags, size_t *statistics_counter)
{
    size_t pages_matched = 0;

    Word_t metric_id_index = 0;
    bool metric_id_first = true;
    Pvoid_t *pd_by_start_time_s_JudyL;
    while((pd_by_start_time_s_JudyL = JudyLFirstThenNext(epdl->page_details_by_metric_id_JudyL, &metric_id_index, &metric_id_first))) {

        Word_t start_time_index = 0;
        bool start_time_first = true;
        Pvoid_t *PValue;
        while ((PValue = JudyLFirstThenNext(*pd_by_start_time_s_JudyL, &start_time_index, &start_time_first))) {
            struct page_details *pd = *PValue;

            if(!pd->page) {
                pdc_page_status_set(pd, PDC_PAGE_FAILED | tags);
                pages_matched++;
            }
        }
    }

    if(pages_matched && statistics_counter)
        __atomic_add_fetch(statistics_counter, pages_matched, __ATOMIC_RELAXED);
}

static bool extent_list_check_if_pages_are_already_in_cache(struct rrdengine_instance *ctx, EXTENT_PD_LIST *epdl, PDC_PAGE_STATUS tags)
{
    size_t count_remaining = 0;
    size_t found = 0;

    Word_t metric_id_index = 0;
    bool metric_id_first = true;
    Pvoid_t *pd_by_start_time_s_JudyL;
    while((pd_by_start_time_s_JudyL = JudyLFirstThenNext(epdl->page_details_by_metric_id_JudyL, &metric_id_index, &metric_id_first))) {

        Word_t start_time_index = 0;
        bool start_time_first = true;
        Pvoid_t *PValue;
        while ((PValue = JudyLFirstThenNext(*pd_by_start_time_s_JudyL, &start_time_index, &start_time_first))) {
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

// ----------------------------------------------------------------------------
// page details list

void pdc_destroy(PDC *pdc) {
    mrg_metric_release(main_mrg, pdc->metric);
    completion_destroy(&pdc->prep_completion);
    completion_destroy(&pdc->page_completion);

    Pvoid_t *PValue;
    struct page_details *pd;
    Word_t time_index = 0;
    bool first_then_next = true;
    size_t unroutable = 0;
    while((PValue = JudyLFirstThenNext(pdc->page_list_JudyL, &time_index, &first_then_next))) {
        pd = *PValue;

        // no need for atomics here - we are done...
        PDC_PAGE_STATUS status = pd->status;

        if(status & PDC_PAGE_DATAFILE_ACQUIRED) {
            datafile_release(pd->datafile.ptr, DATAFILE_ACQUIRE_PAGE_DETAILS);
            pd->datafile.ptr = NULL;
        }

        internal_fatal(pd->datafile.ptr, "DBENGINE: page details has a datafile.ptr that is not released.");

        if(!pd->page && !(status & (PDC_PAGE_READY | PDC_PAGE_FAILED | PDC_PAGE_RELEASED | PDC_PAGE_SKIP | PDC_PAGE_INVALID))) {
            // pdc_page_status_set(pd, PDC_PAGE_FAILED);
            unroutable++;
        }

        if(pd->page && !(status & PDC_PAGE_RELEASED)) {
            pgc_page_release(main_cache, pd->page);
            // pdc_page_status_set(pd, PDC_PAGE_RELEASED);
        }

        page_details_release(pd);
    }

    JudyLFreeArray(&pdc->page_list_JudyL, PJE0);

    __atomic_sub_fetch(&rrdeng_cache_efficiency_stats.currently_running_queries, 1, __ATOMIC_RELAXED);
    __atomic_sub_fetch(&pdc->ctx->inflight_queries, 1, __ATOMIC_RELAXED);
    pdc_release(pdc);

    if(unroutable)
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_fail_unroutable, unroutable, __ATOMIC_RELAXED);
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


typedef void (*execute_extent_page_details_list_t)(struct rrdengine_instance *ctx, EXTENT_PD_LIST *extent_page_list, enum storage_priority priority);
static void pdc_to_extent_page_details_list(struct rrdengine_instance *ctx, struct page_details_control *pdc, execute_extent_page_details_list_t exec_first_extent_list, execute_extent_page_details_list_t exec_rest_extent_list)
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

    DATAFILE_EXTENT_PD_LIST *datafile_extent_list;
    EXTENT_PD_LIST *extent_page_list;

    if (pdc->page_list_JudyL) {
        bool first_then_next = true;
        while((PValue = JudyLFirstThenNext(pdc->page_list_JudyL, &time_index, &first_then_next))) {
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

            PValue1 = JudyLIns(&JudyL_datafile_list, pd->datafile.fileno, PJE0);
            if (PValue1 && !*PValue1) {
                *PValue1 = datafile_extent_list = mallocz(sizeof(*datafile_extent_list));
                datafile_extent_list->extent_pd_list_by_extent_offset_JudyL = NULL;
                datafile_extent_list->fileno = pd->datafile.fileno;
            }
            else
                datafile_extent_list = *PValue1;

            PValue2 = JudyLIns(&datafile_extent_list->extent_pd_list_by_extent_offset_JudyL, pd->datafile.extent.pos, PJE0);
            if (PValue2 && !*PValue2) {
                *PValue2 = extent_page_list = mallocz( sizeof(*extent_page_list));
                extent_page_list->page_details_by_metric_id_JudyL = NULL;
                extent_page_list->number_of_pages_in_JudyL = 0;
                extent_page_list->file = pd->datafile.file;
                extent_page_list->extent_offset = pd->datafile.extent.pos;
                extent_page_list->extent_size = pd->datafile.extent.bytes;
                extent_page_list->datafile = pd->datafile.ptr;
            }
            else
                extent_page_list = *PValue2;

            extent_page_list->number_of_pages_in_JudyL++;

            Pvoid_t *pd_by_first_time_s_judyL = JudyLIns(&extent_page_list->page_details_by_metric_id_JudyL, pd->metric_id, PJE0);
            Pvoid_t *pd_pptr = JudyLIns(pd_by_first_time_s_judyL, pd->first_time_s, PJE0);
            *pd_pptr = pd;
        }

        size_t extent_list_no = 0;
        Word_t datafile_no = 0;
        first_then_next = true;
        while((PValue = JudyLFirstThenNext(JudyL_datafile_list, &datafile_no, &first_then_next))) {
            datafile_extent_list = *PValue;

            bool first_then_next_extent = true;
            Word_t pos = 0;
            while ((PValue = JudyLFirstThenNext(datafile_extent_list->extent_pd_list_by_extent_offset_JudyL, &pos, &first_then_next_extent))) {
                extent_page_list = *PValue;
                internal_fatal(!extent_page_list, "DBENGINE: extent_list is not populated properly");

                // The extent page list can be dispatched to a worker
                // It will need to populate the cache with "acquired" pages that are in the list (pd) only
                // the rest of the extent pages will be added to the cache butnot acquired

                pdc_acquire(pdc); // we do this for the next worker: do_read_extent_work()
                extent_page_list->pdc = pdc;

                if(extent_list_no++ == 0)
                    exec_first_extent_list(ctx, extent_page_list, pdc->priority);
                else
                    exec_rest_extent_list(ctx, extent_page_list, pdc->priority);
            }
            JudyLFreeArray(&datafile_extent_list->extent_pd_list_by_extent_offset_JudyL, PJE0);
            freez(datafile_extent_list);
        }
        JudyLFreeArray(&JudyL_datafile_list, PJE0);
    }

    pdc_release_and_destroy_if_unreferenced(pdc, true, true);
}

// ----------------------------------------------------------------------------
// work request cache

typedef void (*work_cb)(struct rrdengine_instance *ctx, void *data, struct completion *completion, uv_work_t* req);
typedef void (*after_work_cb)(struct rrdengine_instance *ctx, void *data, struct completion *completion, uv_work_t* req, int status);

struct rrdeng_work {
    uv_work_t req;

    struct rrdengine_instance *ctx;
    void *data;
    struct completion *completion;

    work_cb work_cb;
    after_work_cb after_work_cb;
    enum rrdeng_opcode opcode;

    struct {
        struct rrdeng_work *prev;
        struct rrdeng_work *next;
    } cache;
};

static struct {
    struct {
        SPINLOCK spinlock;
        struct rrdeng_work *available_items;
        size_t available;
    } protected;

    struct {
        size_t allocated;
        size_t dispatched;
        size_t executing;
    } atomics;
} work_request_globals = {
        .protected = {
                .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                .available_items = NULL,
                .available = 0,
        },
        .atomics = {
                .allocated = 0,
                .dispatched = 0,
                .executing = 0,
        },
};

static inline bool work_request_full(void) {
    return __atomic_load_n(&work_request_globals.atomics.dispatched, __ATOMIC_RELAXED) >= (size_t)libuv_worker_threads;
}

static void work_request_cleanup(void) {
    netdata_spinlock_lock(&work_request_globals.protected.spinlock);
    while(work_request_globals.protected.available_items && work_request_globals.protected.available > (size_t)libuv_worker_threads) {
        struct rrdeng_work *item = work_request_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(work_request_globals.protected.available_items, item, cache.prev, cache.next);
        freez(item);
        work_request_globals.protected.available--;
        __atomic_add_fetch(&work_request_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
    }
    netdata_spinlock_unlock(&work_request_globals.protected.spinlock);
}

static inline void work_done(struct rrdeng_work *work_request) {
    netdata_spinlock_lock(&work_request_globals.protected.spinlock);
    DOUBLE_LINKED_LIST_APPEND_UNSAFE(work_request_globals.protected.available_items, work_request, cache.prev, cache.next);
    work_request_globals.protected.available++;
    netdata_spinlock_unlock(&work_request_globals.protected.spinlock);
}

void work_standard_worker(uv_work_t *req) {
    __atomic_add_fetch(&work_request_globals.atomics.executing, 1, __ATOMIC_RELAXED);
    register_libuv_worker_jobs();

    struct rrdeng_work *work_request = req->data;
    // worker_is_busy(work_request->opcode); // this is the wrong job id to the threadpool
    work_request->work_cb(work_request->ctx, work_request->data, work_request->completion, req);
    worker_is_idle();

    __atomic_sub_fetch(&work_request_globals.atomics.dispatched, 1, __ATOMIC_RELAXED);
    __atomic_sub_fetch(&work_request_globals.atomics.executing, 1, __ATOMIC_RELAXED);

    // signal the event loop a worker is available
    fatal_assert(0 == uv_async_send(&rrdeng_main.async));
}

void after_work_standard_callback(uv_work_t* req, int status) {
    struct rrdeng_work *work_request = req->data;

    worker_is_busy(RRDENG_OPCODE_MAX + work_request->opcode);

    if(work_request->after_work_cb)
        work_request->after_work_cb(work_request->ctx, work_request->data, work_request->completion, req, status);

    work_done(work_request);

    worker_is_idle();
}

static bool work_dispatch(struct rrdengine_instance *ctx, void *data, struct completion *completion, enum rrdeng_opcode opcode, work_cb work_cb, after_work_cb after_work_cb) {
    struct rrdeng_work *work_request = NULL;

    internal_fatal(rrdeng_main.tid != gettid(), "work_dispatch() can only be run from the event loop thread");

    netdata_spinlock_lock(&work_request_globals.protected.spinlock);

    if(likely(work_request_globals.protected.available_items)) {
        work_request = work_request_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(work_request_globals.protected.available_items, work_request, cache.prev, cache.next);
        work_request_globals.protected.available--;
    }

    netdata_spinlock_unlock(&work_request_globals.protected.spinlock);

    if(unlikely(!work_request)) {
        work_request = mallocz(sizeof(struct rrdeng_work));
        __atomic_add_fetch(&work_request_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
    }

    memset(work_request, 0, sizeof(struct rrdeng_work));
    work_request->req.data = work_request;
    work_request->ctx = ctx;
    work_request->data = data;
    work_request->completion = completion;
    work_request->work_cb = work_cb;
    work_request->after_work_cb = after_work_cb;
    work_request->opcode = opcode;

    if(uv_queue_work(&rrdeng_main.loop, &work_request->req, work_standard_worker, after_work_standard_callback)) {
        internal_fatal(true, "DBENGINE: cannot queue work");
        work_done(work_request);
        return false;
    }

    __atomic_add_fetch(&work_request_globals.atomics.dispatched, 1, __ATOMIC_RELAXED);

    return true;
}

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

static void pdc_cleanup(void) {
    netdata_spinlock_lock(&pdc_globals.protected.spinlock);

    while(pdc_globals.protected.available_items && pdc_globals.protected.available > (size_t)libuv_worker_threads) {
        PDC *item = pdc_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(pdc_globals.protected.available_items, item, cache.prev, cache.next);
        freez(item);
        pdc_globals.protected.available--;
        __atomic_sub_fetch(&pdc_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
    }

    netdata_spinlock_unlock(&pdc_globals.protected.spinlock);
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

void pdc_release(PDC *pdc) {
    if(unlikely(!pdc)) return;

    netdata_spinlock_lock(&pdc_globals.protected.spinlock);
    DOUBLE_LINKED_LIST_APPEND_UNSAFE(pdc_globals.protected.available_items, pdc, cache.prev, cache.next);
    pdc_globals.protected.available++;
    netdata_spinlock_unlock(&pdc_globals.protected.spinlock);
}

// ----------------------------------------------------------------------------
// WAL cache

static struct {
    struct {
        SPINLOCK spinlock;
        WAL *available_items;
        size_t available;
    } protected;

    struct {
        size_t allocated;
    } atomics;
} wal_globals = {
        .protected = {
                .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                .available_items = NULL,
                .available = 0,
        },
        .atomics = {
                .allocated = 0,
        },
};

static void wal_cleanup(void) {
    netdata_spinlock_lock(&wal_globals.protected.spinlock);

    while(wal_globals.protected.available_items && wal_globals.protected.available > storage_tiers) {
        WAL *wal = wal_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(wal_globals.protected.available_items, wal, cache.prev, cache.next);
        posix_memfree(wal->buf);
        freez(wal);
        wal_globals.protected.available--;
        __atomic_sub_fetch(&wal_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
    }

    netdata_spinlock_unlock(&wal_globals.protected.spinlock);
}

WAL *wal_get(struct rrdengine_instance *ctx, unsigned size) {
    if(!size || size > RRDENG_BLOCK_SIZE)
        fatal("DBENGINE: invalid WAL size requested");

    WAL *wal = NULL;

    netdata_spinlock_lock(&wal_globals.protected.spinlock);

    if(likely(wal_globals.protected.available_items)) {
        wal = wal_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(wal_globals.protected.available_items, wal, cache.prev, cache.next);
        wal_globals.protected.available--;
    }

    uint64_t transaction_id = ctx->commit_log.transaction_id++;
    netdata_spinlock_unlock(&wal_globals.protected.spinlock);

    if(unlikely(!wal)) {
        wal = mallocz(sizeof(WAL));
        wal->buf_size = RRDENG_BLOCK_SIZE;
        int ret = posix_memalign((void *)&wal->buf, RRDFILE_ALIGNMENT, wal->buf_size);
        if (unlikely(ret))
            fatal("DBENGINE: posix_memalign:%s", strerror(ret));
        __atomic_add_fetch(&wal_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
    }

    // these need to survive
    unsigned buf_size = wal->buf_size;
    void *buf = wal->buf;

    memset(wal, 0, sizeof(WAL));

    // put them back
    wal->buf_size = buf_size;
    wal->buf = buf;

    memset(wal->buf, 0, wal->buf_size);

    wal->transaction_id = transaction_id;
    wal->size = size;

    return wal;
}

void wal_release(WAL *wal) {
    if(unlikely(!wal)) return;

    netdata_spinlock_lock(&wal_globals.protected.spinlock);
    DOUBLE_LINKED_LIST_APPEND_UNSAFE(wal_globals.protected.available_items, wal, cache.prev, cache.next);
    wal_globals.protected.available++;
    netdata_spinlock_unlock(&wal_globals.protected.spinlock);
}

// ----------------------------------------------------------------------------
// PDC cache

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

static void page_details_cleanup(void) {
    netdata_spinlock_lock(&page_details_globals.protected.spinlock);

    while(page_details_globals.protected.available_items && page_details_globals.protected.available > (size_t)libuv_worker_threads * 2) {
        struct page_details *item = page_details_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(page_details_globals.protected.available_items, item, cache.prev, cache.next);
        freez(item);
        page_details_globals.protected.available--;
        __atomic_sub_fetch(&page_details_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
    }

    netdata_spinlock_unlock(&page_details_globals.protected.spinlock);
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

void page_details_release(struct page_details *pd) {
    if(unlikely(!pd)) return;

    netdata_spinlock_lock(&page_details_globals.protected.spinlock);
    DOUBLE_LINKED_LIST_APPEND_UNSAFE(page_details_globals.protected.available_items, pd, cache.prev, cache.next);
    page_details_globals.protected.available++;
    netdata_spinlock_unlock(&page_details_globals.protected.spinlock);
}

// ----------------------------------------------------------------------------
// page descriptor cache

static struct {
    struct {
        SPINLOCK spinlock;
        struct page_descr_with_data *available_items;
        size_t available;
    } protected;

    struct {
        size_t allocated;
    } atomics;
} page_descriptor_globals = {
        .protected = {
                .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                .available_items = NULL,
                .available = 0,
        },
        .atomics = {
                .allocated = 0,
        },
};

static void page_descriptor_cleanup(void) {
    netdata_spinlock_lock(&page_descriptor_globals.protected.spinlock);

    while(page_descriptor_globals.protected.available_items && page_descriptor_globals.protected.available > MAX_PAGES_PER_EXTENT) {
        struct page_descr_with_data *item = page_descriptor_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(page_descriptor_globals.protected.available_items, item, cache.prev, cache.next);
        freez(item);
        page_descriptor_globals.protected.available--;
        __atomic_sub_fetch(&page_descriptor_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
    }

    netdata_spinlock_unlock(&page_descriptor_globals.protected.spinlock);
}

struct page_descr_with_data *page_descriptor_get(void) {
    struct page_descr_with_data *descr = NULL;

    netdata_spinlock_lock(&page_descriptor_globals.protected.spinlock);

    if(likely(page_descriptor_globals.protected.available_items)) {
        descr = page_descriptor_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(page_descriptor_globals.protected.available_items, descr, cache.prev, cache.next);
        page_descriptor_globals.protected.available--;
    }

    netdata_spinlock_unlock(&page_descriptor_globals.protected.spinlock);

    if(unlikely(!descr)) {
        descr = mallocz(sizeof(struct page_descr_with_data));
        __atomic_add_fetch(&page_descriptor_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
    }

    memset(descr, 0, sizeof(struct page_descr_with_data));
    return descr;
}

static inline void page_descriptor_release(struct page_descr_with_data *descr) {
    if(unlikely(!descr)) return;

    netdata_spinlock_lock(&page_descriptor_globals.protected.spinlock);
    DOUBLE_LINKED_LIST_APPEND_UNSAFE(page_descriptor_globals.protected.available_items, descr, cache.prev, cache.next);
    page_descriptor_globals.protected.available++;
    netdata_spinlock_unlock(&page_descriptor_globals.protected.spinlock);
}

// ----------------------------------------------------------------------------
// extent io descriptor cache

static struct {
    struct {
        SPINLOCK spinlock;
        struct extent_io_descriptor *available_items;
        size_t available;
    } protected;

    struct {
        size_t allocated;
    } atomics;

} extent_io_descriptor_globals = {
        .protected = {
                .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                .available_items = NULL,
                .available = 0,
        },
        .atomics = {
                .allocated = 0,
        },
};

static void extent_io_descriptor_cleanup(void) {
    netdata_spinlock_lock(&extent_io_descriptor_globals.protected.spinlock);
    while(extent_io_descriptor_globals.protected.available_items && extent_io_descriptor_globals.protected.available > (size_t)libuv_worker_threads) {
        struct extent_io_descriptor *item = extent_io_descriptor_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(extent_io_descriptor_globals.protected.available_items, item, cache.prev, cache.next);
        freez(item);
        extent_io_descriptor_globals.protected.available--;
        __atomic_sub_fetch(&extent_io_descriptor_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
    }
    netdata_spinlock_unlock(&extent_io_descriptor_globals.protected.spinlock);
}

static struct extent_io_descriptor *extent_io_descriptor_get(void) {
    struct extent_io_descriptor *xt_io_descr = NULL;

    netdata_spinlock_lock(&extent_io_descriptor_globals.protected.spinlock);

    if(likely(extent_io_descriptor_globals.protected.available_items)) {
        xt_io_descr = extent_io_descriptor_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(extent_io_descriptor_globals.protected.available_items, xt_io_descr, cache.prev, cache.next);
        extent_io_descriptor_globals.protected.available--;
    }

    netdata_spinlock_unlock(&extent_io_descriptor_globals.protected.spinlock);

    if(unlikely(!xt_io_descr)) {
        xt_io_descr = mallocz(sizeof(struct extent_io_descriptor));
        __atomic_add_fetch(&extent_io_descriptor_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
    }

    memset(xt_io_descr, 0, sizeof(struct extent_io_descriptor));
    return xt_io_descr;
}

static inline void extent_io_descriptor_release(struct extent_io_descriptor *xt_io_descr) {
    if(unlikely(!xt_io_descr)) return;

    netdata_spinlock_lock(&extent_io_descriptor_globals.protected.spinlock);
    DOUBLE_LINKED_LIST_APPEND_UNSAFE(extent_io_descriptor_globals.protected.available_items, xt_io_descr, cache.prev, cache.next);
    extent_io_descriptor_globals.protected.available++;
    netdata_spinlock_unlock(&extent_io_descriptor_globals.protected.spinlock);
}

// ----------------------------------------------------------------------------
// extent with buffer cache

struct extent_buffer {
    size_t bytes;

    struct {
        struct extent_buffer *prev;
        struct extent_buffer *next;
    } cache;

    uint8_t data[];
};

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

static void extent_buffer_init(void) {
    size_t max_extent_uncompressed = MAX_PAGES_PER_EXTENT * RRDENG_BLOCK_SIZE;
    size_t max_size = (size_t)LZ4_compressBound(MAX_PAGES_PER_EXTENT * RRDENG_BLOCK_SIZE);
    if(max_size < max_extent_uncompressed)
        max_size = max_extent_uncompressed;

    extent_buffer_globals.max_size = max_size;
}

static void extent_buffer_cleanup(void) {
    netdata_spinlock_lock(&extent_buffer_globals.protected.spinlock);

    while(extent_buffer_globals.protected.available_items && extent_buffer_globals.protected.available > (size_t)libuv_worker_threads) {
        struct extent_buffer *item = extent_buffer_globals.protected.available_items;
        size_t bytes = sizeof(struct extent_buffer) + item->bytes;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(extent_buffer_globals.protected.available_items, item, cache.prev, cache.next);
        freez(item);
        extent_buffer_globals.protected.available--;
        __atomic_sub_fetch(&extent_buffer_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
        __atomic_sub_fetch(&extent_buffer_globals.atomics.allocated_bytes, bytes, __ATOMIC_RELAXED);
    }

    netdata_spinlock_unlock(&extent_buffer_globals.protected.spinlock);
}

static struct extent_buffer *extent_buffer_get(size_t size) {
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

static inline void extent_buffer_release(struct extent_buffer *eb) {
    if(unlikely(!eb)) return;

    netdata_spinlock_lock(&extent_buffer_globals.protected.spinlock);
    DOUBLE_LINKED_LIST_APPEND_UNSAFE(extent_buffer_globals.protected.available_items, eb, cache.prev, cache.next);
    extent_buffer_globals.protected.available++;
    netdata_spinlock_unlock(&extent_buffer_globals.protected.spinlock);
}

// ----------------------------------------------------------------------------
// query handle cache

static struct {
    struct {
        SPINLOCK spinlock;
        struct rrdeng_query_handle *available_items;
        size_t available;
    } protected;

    struct {
        size_t allocated;
    } atomics;
} rrdeng_query_handle_globals = {
        .protected = {
                .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                .available_items = NULL,
                .available = 0,
        },
        .atomics = {
                .allocated = 0,
        },
};

static void rrdeng_query_handle_cleanup(void) {
    netdata_spinlock_lock(&rrdeng_query_handle_globals.protected.spinlock);

    while(rrdeng_query_handle_globals.protected.available_items && rrdeng_query_handle_globals.protected.available > 10) {
        struct rrdeng_query_handle *item = rrdeng_query_handle_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(rrdeng_query_handle_globals.protected.available_items, item, cache.prev, cache.next);
        freez(item);
        rrdeng_query_handle_globals.protected.available--;
        __atomic_sub_fetch(&rrdeng_query_handle_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
    }

    netdata_spinlock_unlock(&rrdeng_query_handle_globals.protected.spinlock);
}

struct rrdeng_query_handle *rrdeng_query_handle_get(void) {
    struct rrdeng_query_handle *handle = NULL;

    netdata_spinlock_lock(&rrdeng_query_handle_globals.protected.spinlock);

    if(likely(rrdeng_query_handle_globals.protected.available_items)) {
        handle = rrdeng_query_handle_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(rrdeng_query_handle_globals.protected.available_items, handle, cache.prev, cache.next);
        rrdeng_query_handle_globals.protected.available--;
    }

    netdata_spinlock_unlock(&rrdeng_query_handle_globals.protected.spinlock);

    if(unlikely(!handle)) {
        handle = mallocz(sizeof(struct rrdeng_query_handle));
        __atomic_add_fetch(&rrdeng_query_handle_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
    }

    memset(handle, 0, sizeof(struct rrdeng_query_handle));
    return handle;
}

void rrdeng_query_handle_release(struct rrdeng_query_handle *handle) {
    if(unlikely(!handle)) return;

    netdata_spinlock_lock(&rrdeng_query_handle_globals.protected.spinlock);
    DOUBLE_LINKED_LIST_APPEND_UNSAFE(rrdeng_query_handle_globals.protected.available_items, handle, cache.prev, cache.next);
    rrdeng_query_handle_globals.protected.available++;
    netdata_spinlock_unlock(&rrdeng_query_handle_globals.protected.spinlock);
}

// ----------------------------------------------------------------------------
// command queue cache

struct rrdeng_cmd {
    struct rrdengine_instance *ctx;
    enum rrdeng_opcode opcode;
    void *data;
    struct completion *completion;
    enum storage_priority priority;

    struct {
        struct rrdeng_cmd *prev;
        struct rrdeng_cmd *next;
    } cache;
};

static struct {
    SPINLOCK spinlock;
    struct rrdeng_cmd *available_items;
    size_t allocated;
    size_t available;

    size_t waiting;
    struct rrdeng_cmd *waiting_items_by_priority[STORAGE_PRIO_MAX_DONT_USE];

} rrdeng_cmd_globals = {
        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
        .available_items = NULL,
        .allocated = 0,
        .available = 0,
        .waiting = 0,
};

static void rrdeng_cmd_cleanup(void) {
    netdata_spinlock_lock(&rrdeng_cmd_globals.spinlock);
    while(rrdeng_cmd_globals.available_items && rrdeng_cmd_globals.available > 100) {
        struct rrdeng_cmd *item = rrdeng_cmd_globals.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(rrdeng_cmd_globals.available_items, item, cache.prev, cache.next);
        freez(item);
        rrdeng_cmd_globals.allocated--;
        rrdeng_cmd_globals.available--;
    }
    netdata_spinlock_unlock(&rrdeng_cmd_globals.spinlock);
}

void rrdeng_enq_cmd(struct rrdengine_instance *ctx, enum rrdeng_opcode opcode, void *data, struct completion *completion, STORAGE_PRIORITY priority) {
    struct rrdeng_cmd *cmd;

    if(priority >= STORAGE_PRIO_MAX_DONT_USE)
        priority = STORAGE_PRIORITY_NORMAL;

    netdata_spinlock_lock(&rrdeng_cmd_globals.spinlock);

    if(unlikely(!rrdeng_cmd_globals.available_items)) {
        cmd = mallocz(sizeof(struct rrdeng_cmd));
        cmd->cache.prev = NULL;
        cmd->cache.next = NULL;
        rrdeng_cmd_globals.allocated++;
    }
    else {
        cmd = rrdeng_cmd_globals.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(rrdeng_cmd_globals.available_items, cmd, cache.prev, cache.next);
        rrdeng_cmd_globals.available--;
    }

    memset(cmd, 0, sizeof(struct rrdeng_cmd));
    cmd->ctx = ctx;
    cmd->opcode = opcode;
    cmd->data = data;
    cmd->completion = completion;
    cmd->priority = priority;

    DOUBLE_LINKED_LIST_APPEND_UNSAFE(rrdeng_cmd_globals.waiting_items_by_priority[priority], cmd, cache.prev, cache.next);
    rrdeng_cmd_globals.waiting++;

    netdata_spinlock_unlock(&rrdeng_cmd_globals.spinlock);

    fatal_assert(0 == uv_async_send(&rrdeng_main.async));
}

static inline struct rrdeng_cmd rrdeng_deq_cmd(void) {
    struct rrdeng_cmd ret = {
            .ctx = NULL,
            .opcode = RRDENG_OPCODE_NOOP,
            .priority = STORAGE_PRIORITY_BEST_EFFORT,
            .completion = NULL,
            .data = NULL,
    };

    STORAGE_PRIORITY max_priority = work_request_full() ? STORAGE_PRIORITY_CRITICAL : STORAGE_PRIORITY_BEST_EFFORT;

    netdata_spinlock_lock(&rrdeng_cmd_globals.spinlock);

    for(STORAGE_PRIORITY priority = STORAGE_PRIORITY_CRITICAL; priority <= max_priority ; priority++) {
        struct rrdeng_cmd *cmd = rrdeng_cmd_globals.waiting_items_by_priority[priority];
        if(cmd) {
            DOUBLE_LINKED_LIST_REMOVE_UNSAFE(rrdeng_cmd_globals.waiting_items_by_priority[priority], cmd, cache.prev, cache.next);
            ret = *cmd;
            DOUBLE_LINKED_LIST_APPEND_UNSAFE(rrdeng_cmd_globals.available_items, cmd, cache.prev, cache.next);
            rrdeng_cmd_globals.available++;
            rrdeng_cmd_globals.waiting--;
            break;
        }
    }

    netdata_spinlock_unlock(&rrdeng_cmd_globals.spinlock);

    return ret;
}


// ----------------------------------------------------------------------------

void *dbengine_page_alloc(struct rrdengine_instance *ctx __maybe_unused, size_t size) {
    void *page = mallocz(size);
    return page;
}

void dbengine_page_free(void *page) {
    freez(page);
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
    vd.entries = (size_t) vd.page_length / vd.point_size;
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
            size_t entries_by_time = (vd.start_time_s - (vd.start_time_s - vd.update_every_s)) / vd.update_every_s;

            if (vd.entries != entries_by_time)
                vd.end_time_s = (time_t) (vd.start_time_s + (vd.entries - 1) * vd.update_every_s);
        }
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

static bool extent_uncompress_and_populate_pages(
                struct rrdengine_instance *ctx,
                void *data,
                size_t data_length,
                EXTENT_PD_LIST *extent_page_list,
                bool preload_all_pages,
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
                    extent_page_list->extent_offset, extent_page_list->extent_size, extent_page_list->datafile->fileno);

        return false;
    }

    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, data, extent_page_list->extent_size - sizeof(*trailer));
    ret = crc32cmp(trailer->checksum, crc);
    if (unlikely(ret)) {
        ++ctx->stats.io_errors;
        rrd_stat_atomic_add(&global_io_errors, 1);
        have_read_error = true;

        error_limit_static_global_var(erl, 1, 0);
        error_limit(&erl, "%s: Extent at offset %"PRIu64" (%u bytes) was read from datafile %u, but CRC32 check FAILED", __func__,
                    extent_page_list->extent_offset, extent_page_list->extent_size, extent_page_list->datafile->fileno);
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

    size_t stats_data_from_main_cache = 0;
    size_t stats_data_from_extent = 0;
    size_t stats_load_compressed = 0;
    size_t stats_load_uncompressed = 0;
    size_t stats_load_invalid_page = 0;
    size_t stats_cache_hit_while_inserting = 0;
    size_t stats_cache_hit_before_allocation = 0;

    uint32_t page_offset = 0, page_length;
    time_t now_s = now_realtime_sec();
    for (i = 0; i < count; i++, page_offset += page_length) {
        page_length = header->descr[i].page_length;
        time_t start_time_s = (time_t) (header->descr[i].start_time_ut / USEC_PER_SEC);

        if(!page_length || !start_time_s) {
            error_limit_static_global_var(erl, 1, 0);
            error_limit(&erl, "%s: Extent at offset %"PRIu64" (%u bytes) was read from datafile %u, having page %u (out of %u) EMPTY",
                        __func__, extent_page_list->extent_offset, extent_page_list->extent_size, extent_page_list->datafile->fileno, i, count);
            continue;
        }

        if(worker)
            worker_is_busy(UV_EVENT_METRIC_LOOKUP);

        METRIC *metric = mrg_metric_get_and_acquire(main_mrg, &header->descr[i].uuid, (Word_t)ctx);
        Word_t metric_id = (Word_t)metric;
        if(!metric) {
            error_limit_static_global_var(erl, 1, 0);
            error_limit(&erl, "%s: Extent at offset %"PRIu64" (%u bytes) was read from datafile %u, having page %u (out of %u) for unknown UUID",
                        __func__, extent_page_list->extent_offset, extent_page_list->extent_size, extent_page_list->datafile->fileno, i, count);
            continue;
        }
        mrg_metric_release(main_mrg, metric);

        if(worker)
            worker_is_busy(UV_EVENT_PAGE_LOOKUP);

        struct page_details *pd = NULL;
        Pvoid_t *pd_by_start_time_s_judyL = JudyLGet(extent_page_list->page_details_by_metric_id_JudyL, metric_id, PJE0);
        internal_fatal(pd_by_start_time_s_judyL == PJERR, "DBENGINE: corrupted extent metrics JudyL");

        if(pd_by_start_time_s_judyL && *pd_by_start_time_s_judyL) {
            Pvoid_t *pd_pptr = JudyLGet(*pd_by_start_time_s_judyL, start_time_s, PJE0);
            internal_fatal(pd_pptr == PJERR, "DBENGINE: corrupted metric page details JudyHS");

            if(pd_pptr && *pd_pptr) {
                pd = *pd_pptr;
                internal_fatal(metric_id != pd->metric_id, "DBENGINE: metric ids do not match");
            }
        }

        if(!pd && !preload_all_pages)
            continue;

        VALIDATED_PAGE_DESCRIPTOR vd = validate_extent_page_descr(
                &header->descr[i], now_s,
                (pd) ? pd->update_every_s : 0,
                have_read_error);

        if(worker)
            worker_is_busy(UV_EVENT_PAGE_POPULATION);

        PGC_PAGE *page = pgc_page_get_and_acquire(main_cache, (Word_t)ctx, metric_id, start_time_s, PGC_SEARCH_EXACT);
        if (!page) {
            void *page_data = dbengine_page_alloc(ctx, vd.page_length);

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
                .start_time_t = vd.start_time_s,
                .end_time_t = vd.end_time_s,
                .update_every = vd.update_every_s,
                .size = (size_t) vd.page_length,
                .data = page_data
            };

            bool added = true;
            page = pgc_page_add_and_acquire(main_cache, page_entry, &added);
            if (false == added) {
                dbengine_page_free(page_data);
                stats_cache_hit_while_inserting++;
                stats_data_from_main_cache++;
            }
            else
                stats_data_from_extent++;
        }
        else {
            stats_cache_hit_before_allocation++;
            stats_data_from_main_cache++;
        }

        if (pd) {
            pd->page = page;
            pd->page_length = pgc_page_data_size(main_cache, page);
            pdc_page_status_set(pd, PDC_PAGE_READY | tags);
        }
        else
            pgc_page_release(main_cache, page);
    }

    if(stats_data_from_main_cache)
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_data_source_main_cache, stats_data_from_main_cache, __ATOMIC_RELAXED);

    if(cached_extent)
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_data_source_extent_cache, stats_data_from_extent, __ATOMIC_RELAXED);
    else
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_data_source_disk, stats_data_from_extent, __ATOMIC_RELAXED);

    if(stats_cache_hit_before_allocation)
        __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_ok_loaded_but_cache_hit_before_allocation, stats_cache_hit_before_allocation, __ATOMIC_RELAXED);

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

static void commit_data_extent(struct rrdengine_instance *ctx, struct extent_io_descriptor *xt_io_descr) {
    unsigned count, payload_length, descr_size, size_bytes;
    void *buf;
    /* persistent structures */
    struct rrdeng_df_extent_header *df_header;
    struct rrdeng_jf_transaction_header *jf_header;
    struct rrdeng_jf_store_data *jf_metric_data;
    struct rrdeng_jf_transaction_trailer *jf_trailer;
    uLong crc;

    df_header = xt_io_descr->buf;
    count = df_header->number_of_pages;
    descr_size = sizeof(*jf_metric_data->descr) * count;
    payload_length = sizeof(*jf_metric_data) + descr_size;
    size_bytes = sizeof(*jf_header) + payload_length + sizeof(*jf_trailer);

    xt_io_descr->wal = wal_get(ctx, size_bytes);
    buf = xt_io_descr->wal->buf;

    jf_header = buf;
    jf_header->type = STORE_DATA;
    jf_header->reserved = 0;
    jf_header->id = xt_io_descr->wal->transaction_id;
    jf_header->payload_length = payload_length;

    jf_metric_data = buf + sizeof(*jf_header);
    jf_metric_data->extent_offset = xt_io_descr->pos;
    jf_metric_data->extent_size = xt_io_descr->bytes;
    jf_metric_data->number_of_pages = count;
    memcpy(jf_metric_data->descr, df_header->descr, descr_size);

    jf_trailer = buf + sizeof(*jf_header) + payload_length;
    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, buf, sizeof(*jf_header) + payload_length);
    crc32set(jf_trailer->checksum, crc);
}

static void after_extent_flushed_to_open(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    ctx->worker_config.outstanding_flush_requests--;

    if(completion)
        completion_mark_complete(completion);

    rrdeng_enq_cmd(ctx, RRDENG_OPCODE_DATABASE_ROTATE, NULL, NULL, STORAGE_PRIORITY_CRITICAL);
}

static void extent_flushed_to_open_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    worker_is_busy(UV_EVENT_FLUSHED_TO_OPEN);

    uv_fs_t *uv_fs_request = data;
    struct extent_io_descriptor *xt_io_descr = uv_fs_request->data;
    struct page_descr_with_data *descr;
    struct rrdengine_datafile *datafile;
    unsigned i;

    if (uv_fs_request->result < 0) {
        __atomic_add_fetch(&ctx->stats.io_errors, 1, __ATOMIC_RELAXED);
        rrd_stat_atomic_add(&global_io_errors, 1);
        error("DBENGINE: %s: uv_fs_write: %s", __func__, uv_strerror((int)uv_fs_request->result));
    }
    datafile = xt_io_descr->datafile;

    // Descriptors need to be freed when migration to V2 happens

    for (i = 0 ; i < xt_io_descr->descr_count ; ++i) {
        descr = xt_io_descr->descr_array[i];

        pgc_open_add_hot_page(
                (Word_t)ctx, descr->metric_id,
                (time_t) (descr->start_time_ut / USEC_PER_SEC),
                (time_t) (descr->end_time_ut / USEC_PER_SEC),
                descr->update_every_s,
                datafile,
                xt_io_descr->pos, xt_io_descr->bytes, descr->page_length);

        page_descriptor_release(descr);
    }

    uv_fs_req_cleanup(uv_fs_request);
    posix_memfree(xt_io_descr->buf);
    extent_io_descriptor_release(xt_io_descr);

    netdata_spinlock_lock(&datafile->writers.spinlock);
    datafile->writers.flushed_to_open_running--;
    netdata_spinlock_unlock(&datafile->writers.spinlock);

    if(datafile->fileno != __atomic_load_n(&ctx->last_fileno, __ATOMIC_RELAXED))
        // we just finished a flushing on a datafile that is not the active one
        rrdeng_enq_cmd(ctx, RRDENG_OPCODE_JOURNAL_FILE_INDEX, datafile, NULL, STORAGE_PRIORITY_CRITICAL);
}

// Main event loop callback
static void extent_flush_io_callback(uv_fs_t *uv_fs_request) {
    worker_is_busy(RRDENG_OPCODE_MAX + RRDENG_OPCODE_FLUSH_PAGES);
    struct extent_io_descriptor *xt_io_descr = uv_fs_request->data;
    struct rrdengine_datafile *datafile = xt_io_descr->datafile;
    struct rrdengine_instance *ctx = datafile->ctx;

    wal_flush_transaction_buffer(ctx, xt_io_descr->datafile, xt_io_descr->wal, &rrdeng_main.loop);

    netdata_spinlock_lock(&datafile->writers.spinlock);
    datafile->writers.running--;

    datafile->writers.flushed_to_open_running++;
    rrdeng_enq_cmd(xt_io_descr->ctx, RRDENG_OPCODE_FLUSHED_TO_OPEN, uv_fs_request, xt_io_descr->completion, STORAGE_PRIORITY_CRITICAL);

    netdata_spinlock_unlock(&datafile->writers.spinlock);

    worker_is_idle();
}

/*
 * Take a page list in a judy array and write them
 */
static unsigned do_flush_extent(struct rrdengine_instance *ctx, struct page_descr_with_data *base, struct completion *completion) {
    int ret;
    int compressed_size, max_compressed_size = 0;
    unsigned i, count, size_bytes, pos, real_io_size;
    uint32_t uncompressed_payload_length, payload_offset;
    struct page_descr_with_data *descr, *eligible_pages[MAX_PAGES_PER_EXTENT];
    struct extent_io_descriptor *xt_io_descr;
    struct extent_buffer *eb = NULL;
    void *compressed_buf = NULL;
    Word_t Index;
    uint8_t compression_algorithm = ctx->global_compress_alg;
    struct rrdengine_datafile *datafile;
    /* persistent structures */
    struct rrdeng_df_extent_header *header;
    struct rrdeng_df_extent_trailer *trailer;
    uLong crc;

    for(descr = base, Index = 0, count = 0, uncompressed_payload_length = 0;
        descr && count != rrdeng_pages_per_extent;
        descr = descr->link.next, Index++) {

        uncompressed_payload_length += descr->page_length;
        eligible_pages[count++] = descr;

    }

    if (!count) {
        if (completion)
            completion_mark_complete(completion);
        return 0;
    }

    xt_io_descr = extent_io_descriptor_get();
    xt_io_descr->ctx = ctx;
    payload_offset = sizeof(*header) + count * sizeof(header->descr[0]);
    switch (compression_algorithm) {
        case RRD_NO_COMPRESSION:
            size_bytes = payload_offset + uncompressed_payload_length + sizeof(*trailer);
            break;

        default: /* Compress */
            fatal_assert(uncompressed_payload_length < LZ4_MAX_INPUT_SIZE);
            max_compressed_size = LZ4_compressBound(uncompressed_payload_length);
            eb = extent_buffer_get(max_compressed_size);
            compressed_buf = eb->data;
            size_bytes = payload_offset + MAX(uncompressed_payload_length, (unsigned)max_compressed_size) + sizeof(*trailer);
            break;
    }

    ret = posix_memalign((void *)&xt_io_descr->buf, RRDFILE_ALIGNMENT, ALIGN_BYTES_CEILING(size_bytes));
    if (unlikely(ret)) {
        fatal("DBENGINE: posix_memalign:%s", strerror(ret));
        /* freez(xt_io_descr);*/
    }
    memset(xt_io_descr->buf, 0, ALIGN_BYTES_CEILING(size_bytes));
    (void) memcpy(xt_io_descr->descr_array, eligible_pages, sizeof(struct page_descr_with_data *) * count);
    xt_io_descr->descr_count = count;

    pos = 0;
    header = xt_io_descr->buf;
    header->compression_algorithm = compression_algorithm;
    header->number_of_pages = count;
    pos += sizeof(*header);

    for (i = 0 ; i < count ; ++i) {
        descr = xt_io_descr->descr_array[i];
        header->descr[i].type = descr->type;
        uuid_copy(*(uuid_t *)header->descr[i].uuid, *descr->id);
        header->descr[i].page_length = descr->page_length;
        header->descr[i].start_time_ut = descr->start_time_ut;
        header->descr[i].end_time_ut = descr->end_time_ut;
        pos += sizeof(header->descr[i]);
    }
    for (i = 0 ; i < count ; ++i) {
        descr = xt_io_descr->descr_array[i];
        (void) memcpy(xt_io_descr->buf + pos, descr->page, descr->page_length);
        pos += descr->page_length;
    }

    switch (compression_algorithm) {
        case RRD_NO_COMPRESSION:
            header->payload_length = uncompressed_payload_length;
            break;
        default: /* Compress */
            compressed_size = LZ4_compress_default(xt_io_descr->buf + payload_offset, compressed_buf,
                                               uncompressed_payload_length, max_compressed_size);
            ctx->stats.before_compress_bytes += uncompressed_payload_length;
            ctx->stats.after_compress_bytes += compressed_size;
            debug(D_RRDENGINE, "LZ4 compressed %"PRIu32" bytes to %d bytes.", uncompressed_payload_length, compressed_size);
            (void) memcpy(xt_io_descr->buf + payload_offset, compressed_buf, compressed_size);
            extent_buffer_release(eb);
            size_bytes = payload_offset + compressed_size + sizeof(*trailer);
            header->payload_length = compressed_size;
        break;
    }

    datafile = ctx->datafiles.first->prev;
    if(datafile->pos > rrdeng_target_data_file_size(ctx)) {
        static SPINLOCK sp = NETDATA_SPINLOCK_INITIALIZER;
        netdata_spinlock_lock(&sp);
        if(create_new_datafile_pair(ctx) == 0)
            rrdeng_enq_cmd(ctx, RRDENG_OPCODE_JOURNAL_FILE_INDEX, datafile, NULL, STORAGE_PRIORITY_CRITICAL);
        netdata_spinlock_unlock(&sp);
    }

    datafile = ctx->datafiles.first->prev;
    netdata_spinlock_lock(&datafile->writers.spinlock);

    datafile->writers.running++;

    xt_io_descr->datafile = datafile;
    xt_io_descr->bytes = size_bytes;
    xt_io_descr->pos = datafile->pos;
    xt_io_descr->uv_fs_request.data = xt_io_descr;
    xt_io_descr->completion = completion;

    trailer = xt_io_descr->buf + size_bytes - sizeof(*trailer);
    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, xt_io_descr->buf, size_bytes - sizeof(*trailer));
    crc32set(trailer->checksum, crc);

    real_io_size = ALIGN_BYTES_CEILING(size_bytes);
    xt_io_descr->iov = uv_buf_init((void *)xt_io_descr->buf, real_io_size);

    ctx->stats.io_write_bytes += real_io_size;
    ++ctx->stats.io_write_requests;
    ctx->stats.io_write_extent_bytes += real_io_size;
    ++ctx->stats.io_write_extents;
    commit_data_extent(ctx, xt_io_descr);
    datafile->pos += real_io_size;
    ctx->disk_space += real_io_size;
    ctx->last_flush_fileno = datafile->fileno;

    ret = uv_fs_write(&rrdeng_main.loop, &xt_io_descr->uv_fs_request, datafile->file, &xt_io_descr->iov,
                      1, xt_io_descr->pos, extent_flush_io_callback);

    fatal_assert(-1 != ret);

    netdata_spinlock_unlock(&datafile->writers.spinlock);

    return real_io_size;
}

static void after_database_rotate(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    ctx->worker_config.now_deleting_files = false;
}

struct uuid_first_time_s {
    uuid_t *uuid;
    time_t first_time_t;
    time_t last_time_t;
    METRIC *metric;
};


static int journal_metric_uuid_compare(const void *key, const void *metric)
{
    return uuid_compare(*(uuid_t *) key, ((struct journal_metric_list *) metric)->uuid);
}

void find_uuid_first_time(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile, Pvoid_t metric_first_time_JudyL)
{
    if (unlikely(!datafile))
        return;

    unsigned v2_count = 0;
    unsigned journalfile_count = 0;
    uv_rwlock_rdlock(&ctx->datafiles.rwlock);
    while (datafile) {
        struct journal_v2_header *journal_header = (struct journal_v2_header *) GET_JOURNAL_DATA(datafile->journalfile);
        if (!journal_header || !datafile->users.available) {
            datafile = datafile->next;
            continue;
        }

        time_t journal_start_time_t = (time_t) (journal_header->start_time_ut / USEC_PER_SEC);
        size_t journal_metric_count = (size_t)journal_header->metric_count;
        struct journal_metric_list *uuid_list = (struct journal_metric_list *)((uint8_t *) journal_header + journal_header->metric_offset);

        Word_t index = 0;
        bool first_then_next = true;
        Pvoid_t *PValue;
        while ((PValue = JudyLFirstThenNext(metric_first_time_JudyL, &index, &first_then_next))) {
            struct uuid_first_time_s *uuid_first_t_entry = *PValue;

            struct journal_metric_list *uuid_entry = bsearch(uuid_first_t_entry->uuid,uuid_list,journal_metric_count,sizeof(*uuid_list), journal_metric_uuid_compare);

            if (unlikely(!uuid_entry))
                continue;

            time_t first_time_t = uuid_entry->delta_start + journal_start_time_t;
            time_t last_time_t = uuid_entry->delta_end + journal_start_time_t;
            uuid_first_t_entry->first_time_t = MIN(uuid_first_t_entry->first_time_t , first_time_t);
            uuid_first_t_entry->last_time_t = MAX(uuid_first_t_entry->last_time_t , last_time_t);
            v2_count++;
        }
        journalfile_count++;
        datafile = datafile->next;
    }
    uv_rwlock_rdunlock(&ctx->datafiles.rwlock);

    // Let's scan the open cache for almost exact match
    bool first_then_next = true;
    Pvoid_t *PValue;
    Word_t index = 0;
    unsigned open_cache_count = 0;
    while ((PValue = JudyLFirstThenNext(metric_first_time_JudyL, &index, &first_then_next))) {
        struct uuid_first_time_s *uuid_first_t_entry = *PValue;

        PGC_PAGE *page = pgc_page_get_and_acquire(
                open_cache, (Word_t)ctx,
                (Word_t)uuid_first_t_entry->metric, uuid_first_t_entry->last_time_t,
                PGC_SEARCH_CLOSEST);

        if (page) {
            time_t first_time_t = pgc_page_start_time_t(page);
            time_t last_time_t = pgc_page_end_time_t(page);
            uuid_first_t_entry->first_time_t = MIN(uuid_first_t_entry->first_time_t, first_time_t);
            uuid_first_t_entry->last_time_t = MAX(uuid_first_t_entry->last_time_t, last_time_t);
            pgc_page_release(open_cache, page);
            open_cache_count++;
        }
    }
    info("DBENGINE: processed %u journalfiles and matched %u metrics in v2 files and %u in open cache", journalfile_count,
        v2_count, open_cache_count);
}

static void update_metrics_first_time_t(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile_to_delete, struct rrdengine_datafile *first_datafile_remaining, bool worker) {
    if(worker)
        worker_is_busy(UV_EVENT_ANALYZE_V2);

    struct rrdengine_journalfile *journal_file = datafile_to_delete->journalfile;
    struct journal_v2_header *journal_header = (struct journal_v2_header *)GET_JOURNAL_DATA(journal_file);
    struct journal_metric_list *uuid_list = (struct journal_metric_list *)((uint8_t *) journal_header + journal_header->metric_offset);

    Pvoid_t metric_first_time_JudyL = (Pvoid_t) NULL;
    Pvoid_t *PValue;

    unsigned count = 0;
    struct uuid_first_time_s *uuid_first_t_entry;
    for (uint32_t index = 0; index < journal_header->metric_count; ++index) {
        METRIC *metric = mrg_metric_get_and_acquire(main_mrg, &uuid_list[index].uuid, (Word_t) ctx);
        if (!metric)
            continue;

        PValue = JudyLIns(&metric_first_time_JudyL, (Word_t) index, PJE0);
        fatal_assert(NULL != PValue);
        if (!*PValue) {
            uuid_first_t_entry = mallocz(sizeof(*uuid_first_t_entry));
            uuid_first_t_entry->metric = metric;
            uuid_first_t_entry->first_time_t = mrg_metric_get_first_time_t(main_mrg, metric);
            uuid_first_t_entry->last_time_t = mrg_metric_get_latest_time_t(main_mrg, metric);
            uuid_first_t_entry->uuid = mrg_metric_uuid(main_mrg, metric);
            *PValue = uuid_first_t_entry;
            count++;
        }
    }

    info("DBENGINE: recalculating retention for %u metrics", count);

    // Update the first time / last time for all metrics we plan to delete

    if(worker)
        worker_is_busy(UV_EVENT_RETENTION_V2);

    find_uuid_first_time(ctx, first_datafile_remaining, metric_first_time_JudyL);

    if(worker)
        worker_is_busy(UV_EVENT_RETENTION_UPDATE);

    info("DBENGINE: updating metric registry retention for %u metrics", count);

    Word_t index = 0;
    bool first_then_next = true;
    while ((PValue = JudyLFirstThenNext(metric_first_time_JudyL, &index, &first_then_next))) {
        uuid_first_t_entry = *PValue;
        mrg_metric_set_first_time_t(main_mrg, uuid_first_t_entry->metric, uuid_first_t_entry->first_time_t);
        mrg_metric_release(main_mrg, uuid_first_t_entry->metric);
        freez(uuid_first_t_entry);
    }

    JudyLFreeArray(&metric_first_time_JudyL, PJE0);

    if(worker)
        worker_is_idle();
}

static void datafile_delete(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile, bool worker) {
    if(worker)
        worker_is_busy(UV_EVENT_DATAFILE_ACQUIRE);

    bool datafile_got_for_deletion = datafile_acquire_for_deletion(datafile);

    update_metrics_first_time_t(ctx, datafile, datafile->next, worker);

    while (!datafile_got_for_deletion) {
        if(worker)
            worker_is_busy(UV_EVENT_DATAFILE_ACQUIRE);

        datafile_got_for_deletion = datafile_acquire_for_deletion(datafile);

        if (!datafile_got_for_deletion) {
            info("DBENGINE: waiting for data file '%s/"
                         DATAFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL DATAFILE_EXTENSION
                         "' to be available for deletion, "
                         "it is in use currently by %u users.",
                 ctx->dbfiles_path, ctx->datafiles.first->tier, ctx->datafiles.first->fileno, datafile->users.lockers);

            sleep_usec(1 * USEC_PER_SEC);
        }
    }

    info("DBENGINE: deleting data file '%s/"
         DATAFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL DATAFILE_EXTENSION
         "'.",
         ctx->dbfiles_path, ctx->datafiles.first->tier, ctx->datafiles.first->fileno);

    if(worker)
        worker_is_busy(UV_EVENT_DATAFILE_DELETE);

    struct rrdengine_journalfile *journal_file;
    unsigned deleted_bytes, journal_file_bytes, datafile_bytes;
    int ret;
    char path[RRDENG_PATH_MAX];

    uv_rwlock_wrlock(&ctx->datafiles.rwlock);

    journal_file = datafile->journalfile;
    datafile_bytes = datafile->pos;
    journal_file_bytes = journal_file->pos;
    deleted_bytes = GET_JOURNAL_DATA_SIZE(journal_file);

    info("DBENGINE: deleting data and journal files to maintain disk quota");
    datafile_list_delete_unsafe(ctx, datafile);
    ret = destroy_journal_file_unsafe(journal_file, datafile);
    if (!ret) {
        generate_journalfilepath(datafile, path, sizeof(path));
        info("DBENGINE: deleted journal file \"%s\".", path);
        generate_journalfilepath_v2(datafile, path, sizeof(path));
        info("DBENGINE: deleted journal file \"%s\".", path);
        deleted_bytes += journal_file_bytes;
    }
    ret = destroy_data_file_unsafe(datafile);
    if (!ret) {
        generate_datafilepath(datafile, path, sizeof(path));
        info("DBENGINE: deleted data file \"%s\".", path);
        deleted_bytes += datafile_bytes;
    }
    freez(journal_file);
    freez(datafile);

    ctx->disk_space -= deleted_bytes;
    info("DBENGINE: reclaimed %u bytes of disk space.", deleted_bytes);
    uv_rwlock_wrunlock(&ctx->datafiles.rwlock);

    rrdcontext_db_rotation();
}

static void database_rotate_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    datafile_delete(ctx, ctx->datafiles.first, true);
}

static void cache_flush_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    if (!main_cache)
        return;

    worker_is_busy(UV_EVENT_FLUSH_MAIN);
    pgc_flush_pages(main_cache, 0);
}

static void cache_evict_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *req __maybe_unused) {
    if (!main_cache)
        return;

    worker_is_busy(UV_EVENT_EVICT_MAIN);
    pgc_evict_pages(main_cache, 0, 0);
}

static void after_prep_query(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    ;
}

static void query_prep_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *req __maybe_unused) {
    worker_is_busy(UV_EVENT_PREP_QUERY);
    PDC *pdc = data;
    rrdeng_prep_query(pdc);
}

unsigned rrdeng_target_data_file_size(struct rrdengine_instance *ctx) {
    unsigned target_size = ctx->max_disk_space / TARGET_DATAFILES;
    target_size = MIN(target_size, MAX_DATAFILE_SIZE);
    target_size = MAX(target_size, MIN_DATAFILE_SIZE);
    return target_size;
}

/* return 0 on success */
int init_rrd_files(struct rrdengine_instance *ctx)
{
    return init_data_files(ctx);
}

void finalize_rrd_files(struct rrdengine_instance *ctx)
{
    return finalize_data_files(ctx);
}

void async_cb(uv_async_t *handle)
{
    uv_stop(handle->loop);
    uv_update_time(handle->loop);
    debug(D_RRDENGINE, "%s called, active=%d.", __func__, uv_is_active((uv_handle_t *)handle));
}

#define TIMER_PERIOD_MS (1000)

static bool datafile_get_exclusive_access_to_extent(EXTENT_PD_LIST *extent_page_list) {
    struct rrdengine_datafile *df = extent_page_list->datafile;
    bool is_it_mine = false;

    while(!is_it_mine) {
        netdata_spinlock_lock(&df->extent_exclusive_access.spinlock);
        if(!df->users.available) {
            netdata_spinlock_unlock(&df->extent_exclusive_access.spinlock);
            return false;
        }
        Pvoid_t *PValue = JudyLIns(&df->extent_exclusive_access.extents_JudyL, extent_page_list->extent_offset, PJE0);
        if (!*PValue) {
            *(Word_t *) PValue = gettid();
            df->extent_exclusive_access.lockers++;
            is_it_mine = true;
        }
        netdata_spinlock_unlock(&df->extent_exclusive_access.spinlock);

        if(!is_it_mine) {
            static const struct timespec ns = { .tv_sec = 0, .tv_nsec = 1 };
            nanosleep(&ns, NULL);
        }
    }
    return true;
}

static void datafile_release_exclusive_access_to_extent(EXTENT_PD_LIST *extent_page_list) {
    struct rrdengine_datafile *df = extent_page_list->datafile;

    netdata_spinlock_lock(&df->extent_exclusive_access.spinlock);

#ifdef NETDATA_INTERNAL_CHECKS
    Pvoid_t *PValue = JudyLGet(df->extent_exclusive_access.extents_JudyL, extent_page_list->extent_offset, PJE0);
    if (*(Word_t *) PValue != (Word_t)gettid())
        fatal("DBENGINE: exclusive extent access is not mine");
#endif

    int rc = JudyLDel(&df->extent_exclusive_access.extents_JudyL, extent_page_list->extent_offset, PJE0);
    if (!rc)
        fatal("DBENGINE: cannot find my exclusive access");

    df->extent_exclusive_access.lockers--;
    netdata_spinlock_unlock(&df->extent_exclusive_access.spinlock);
}

static void load_pages_from_an_extent_list(struct rrdengine_instance *ctx, EXTENT_PD_LIST *extent_page_list, bool worker) {
    struct page_details_control *pdc = extent_page_list->pdc;

    bool extent_exclusive = false;

    if(pdc->preload_all_extent_pages) {
        if (!datafile_get_exclusive_access_to_extent(extent_page_list)) {
            __atomic_add_fetch(&rrdeng_cache_efficiency_stats.pages_load_fail_datafile_not_available, 1, __ATOMIC_RELAXED);
            goto cleanup;
        }
        extent_exclusive = true;
    }

    if (extent_list_check_if_pages_are_already_in_cache(ctx, extent_page_list, PDC_PAGE_PRELOADED_WORKER))
        goto cleanup;

    if(__atomic_load_n(&pdc->workers_should_stop, __ATOMIC_RELAXED))
        goto cleanup;

    if(worker)
        worker_is_busy(UV_EVENT_EXTENT_CACHE);

    PDC_PAGE_STATUS not_loaded_pages_tag = 0, loaded_pages_tag = 0;
    bool extent_found_in_cache = false;

    void *extent_compressed_data = NULL;
    PGC_PAGE *extent_cache_page = pgc_page_get_and_acquire(
            extent_cache, (Word_t)ctx,
            (Word_t)extent_page_list->datafile->fileno, (time_t)extent_page_list->extent_offset,
            PGC_SEARCH_EXACT);

    if(extent_cache_page) {
        extent_compressed_data = pgc_page_data(extent_cache_page);
        internal_fatal(extent_page_list->extent_size != pgc_page_data_size(extent_cache, extent_cache_page),
                       "DBENGINE: cache size does not match the expected size");

        loaded_pages_tag |= PDC_PAGE_LOADED_FROM_EXTENT_CACHE;
        not_loaded_pages_tag |= PDC_PAGE_LOADED_FROM_EXTENT_CACHE;
        extent_found_in_cache = true;
    }
    else {
        if(worker)
            worker_is_busy(UV_EVENT_EXTENT_MMAP);

        off_t map_start =  ALIGN_BYTES_FLOOR(extent_page_list->extent_offset);
        size_t length = ALIGN_BYTES_CEILING(extent_page_list->extent_offset + extent_page_list->extent_size) - map_start;

        void *mmap_data = mmap(NULL, length, PROT_READ, MAP_SHARED, extent_page_list->file, map_start);
        if(mmap_data != MAP_FAILED) {
            extent_compressed_data = mmap_data + (extent_page_list->extent_offset - map_start);

            void *copied_extent_compressed_data = mallocz(extent_page_list->extent_size);
            memcpy(copied_extent_compressed_data, extent_compressed_data, extent_page_list->extent_size);

            int ret = munmap(mmap_data, length);
            fatal_assert(0 == ret);

            if(worker)
                worker_is_busy(UV_EVENT_EXTENT_CACHE);

            bool added = false;
            extent_cache_page = pgc_page_add_and_acquire(extent_cache, (PGC_ENTRY) {
                    .hot = false,
                    .section = (Word_t) ctx,
                    .metric_id = (Word_t) extent_page_list->datafile->fileno,
                    .start_time_t = (time_t) extent_page_list->extent_offset,
                    .size = extent_page_list->extent_size,
                    .end_time_t = 0,
                    .update_every = 0,
                    .data = copied_extent_compressed_data,
            }, &added);

            if (!added) {
                freez(copied_extent_compressed_data);
                internal_fatal(extent_page_list->extent_size != pgc_page_data_size(extent_cache, extent_cache_page),
                               "DBENGINE: cache size does not match the expected size");
            }

            extent_compressed_data = pgc_page_data(extent_cache_page);

            loaded_pages_tag |= PDC_PAGE_LOADED_FROM_DISK;
            not_loaded_pages_tag |= PDC_PAGE_LOADED_FROM_DISK;
        }
    }

    if(extent_compressed_data) {
        // Need to decompress and then process the pagelist
        bool extent_used = extent_uncompress_and_populate_pages(
                ctx, extent_compressed_data, extent_page_list->extent_size,
                extent_page_list, pdc->preload_all_extent_pages,
                worker, loaded_pages_tag, extent_found_in_cache);

        if(extent_used) {
            // since the extent was used, all the pages that are not
            // loaded from this extent, were not found in the extent
            not_loaded_pages_tag |= PDC_PAGE_FAILED_UUID_NOT_IN_EXTENT;
        }
        else
            not_loaded_pages_tag |= PDC_PAGE_FAILED_INVALID_EXTENT;
    }
    else
        not_loaded_pages_tag |= PDC_PAGE_FAILED_TO_MAP_EXTENT;


    // mark all pending pages as failed
    extent_list_mark_all_not_loaded_pages_as_failed(
            extent_page_list, not_loaded_pages_tag,
            &rrdeng_cache_efficiency_stats.pages_load_fail_cant_mmap_extent);

    if(extent_cache_page)
        pgc_page_release(extent_cache, extent_cache_page);

cleanup:
    if(extent_exclusive)
        datafile_release_exclusive_access_to_extent(extent_page_list);

    completion_mark_complete_a_job(&extent_page_list->pdc->page_completion);
    pdc_release_and_destroy_if_unreferenced(pdc, true, false);

    // Free the Judy that holds the requested pagelist and the extents
    extent_list_free(extent_page_list);

    if(worker)
        worker_is_idle();
}

static void extent_read_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    EXTENT_PD_LIST *extent_page_list = data;
    load_pages_from_an_extent_list(ctx, extent_page_list, true);
}

static void queue_extent_list(struct rrdengine_instance *ctx, EXTENT_PD_LIST *extent_page_list, STORAGE_PRIORITY priority) {
    rrdeng_enq_cmd(ctx, RRDENG_OPCODE_EXTENT_READ, extent_page_list, NULL, priority);
}

void dbengine_load_page_list(struct rrdengine_instance *ctx, struct page_details_control *pdc) {
    pdc_to_extent_page_details_list(ctx, pdc, queue_extent_list, queue_extent_list);
}

void load_pages_from_an_extent_list_directly(struct rrdengine_instance *ctx, EXTENT_PD_LIST *extent_list, enum storage_priority priority __maybe_unused) {
    load_pages_from_an_extent_list(ctx, extent_list, false);
}

void dbengine_load_page_list_directly(struct rrdengine_instance *ctx, struct page_details_control *pdc) {
    pdc_to_extent_page_details_list(ctx, pdc, load_pages_from_an_extent_list_directly, load_pages_from_an_extent_list_directly);
}

#define MAX_RETRIES_TO_START_INDEX (100)
static void journal_v2_indexing_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    unsigned count = 0;
    worker_is_busy(UV_EVENT_JOURNAL_INDEX_WAIT);

    while (ctx->worker_config.now_deleting_files && count++ < MAX_RETRIES_TO_START_INDEX)
        sleep_usec(100 * USEC_PER_MS);

    if (count == MAX_RETRIES_TO_START_INDEX) {
        worker_is_idle();
        return;
    }

    struct rrdengine_datafile *datafile = ctx->datafiles.first;
    worker_is_busy(UV_EVENT_JOURNAL_INDEX);
    count = 0;
    while (datafile && datafile->fileno != ctx->last_fileno && datafile->fileno != ctx->last_flush_fileno) {

        netdata_spinlock_lock(&datafile->writers.spinlock);
        bool available = (datafile->writers.running || datafile->writers.flushed_to_open_running) ? false : true;
        netdata_spinlock_unlock(&datafile->writers.spinlock);

        if(!available)
            continue;

        if (unlikely(!GET_JOURNAL_DATA(datafile->journalfile))) {
            info("DBENGINE: journal file %u is ready to be indexed", datafile->fileno);
            pgc_open_cache_to_journal_v2(open_cache, (Word_t) ctx, (int) datafile->fileno, ctx->page_type, do_migrate_to_v2_callback, (void *) datafile->journalfile);
            count++;
        }

        datafile = datafile->next;
        if (unlikely(NO_QUIESCE != ctx->quiesce))
            break;
    }

    errno = 0;
    internal_error(count, "DBENGINE: journal indexing done; %u files processed", count);

    worker_is_idle();
}

static void after_do_cache_flush(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    rrdeng_main.flush_running = false;
}

static void after_do_cache_evict(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    rrdeng_main.evict_running = false;
}

static void after_extent_read(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    ;
}

static void after_journal_v2_indexing(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    ctx->worker_config.migration_to_v2_running = false;
    rrdeng_enq_cmd(ctx, RRDENG_OPCODE_DATABASE_ROTATE, NULL, NULL, STORAGE_PRIORITY_CRITICAL);
}

struct rrdeng_buffer_sizes rrdeng_get_buffer_sizes(void) {
    return (struct rrdeng_buffer_sizes) {
            .opcodes     = rrdeng_cmd_globals.allocated * sizeof(struct rrdeng_cmd),
            .handles     = __atomic_load_n(&rrdeng_query_handle_globals.atomics.allocated, __ATOMIC_RELAXED) * sizeof(struct rrdeng_query_handle),
            .descriptors = __atomic_load_n(&page_descriptor_globals.atomics.allocated, __ATOMIC_RELAXED) * sizeof(struct page_descr_with_data),
            .wal         = __atomic_load_n(&wal_globals.atomics.allocated, __ATOMIC_RELAXED) * (sizeof(WAL) + RRDENG_BLOCK_SIZE),
            .workers     = __atomic_load_n(&work_request_globals.atomics.allocated, __ATOMIC_RELAXED) * sizeof(struct rrdeng_work),
        .pdc       = __atomic_load_n(&pdc_globals.atomics.allocated, __ATOMIC_RELAXED) * sizeof(PDC),
        .xt_io     = __atomic_load_n(&extent_io_descriptor_globals.atomics.allocated, __ATOMIC_RELAXED) * sizeof(struct extent_io_descriptor),
        .xt_buf    = __atomic_load_n(&extent_buffer_globals.atomics.allocated_bytes, __ATOMIC_RELAXED),
    };
}

void timer_cb(uv_timer_t* handle) {
    worker_is_busy(RRDENG_TIMER_CB);
    uv_stop(handle->loop);
    uv_update_time(handle->loop);

    worker_set_metric(RRDENG_OPCODES_WAITING, (NETDATA_DOUBLE)rrdeng_cmd_globals.waiting);
    worker_set_metric(RRDENG_WORKS_DISPATCHED, (NETDATA_DOUBLE)__atomic_load_n(&work_request_globals.atomics.dispatched, __ATOMIC_RELAXED));
    worker_set_metric(RRDENG_WORKS_EXECUTING, (NETDATA_DOUBLE)__atomic_load_n(&work_request_globals.atomics.executing, __ATOMIC_RELAXED));

    rrdeng_enq_cmd(NULL, RRDENG_OPCODE_FLUSH_INIT, NULL, NULL, STORAGE_PRIORITY_CRITICAL);
    rrdeng_enq_cmd(NULL, RRDENG_OPCODE_EVICT_INIT, NULL, NULL, STORAGE_PRIORITY_CRITICAL);

    if(now_monotonic_sec() % 600 == 0) {
        work_request_cleanup();
        page_descriptor_cleanup();
        extent_io_descriptor_cleanup();
        rrdeng_cmd_cleanup();
        pdc_cleanup();
        page_details_cleanup();
        rrdeng_query_handle_cleanup();
        wal_cleanup();
        extent_buffer_cleanup();
    }

    worker_is_idle();
}

bool rrdeng_dbengine_spawn(struct rrdengine_instance *ctx) {
    static bool spawned = false;

    if(!spawned) {
        int ret;

        ret = uv_loop_init(&rrdeng_main.loop);
        if (ret) {
            error("DBENGINE: uv_loop_init(): %s", uv_strerror(ret));
            return false;
        }
        rrdeng_main.loop.data = &rrdeng_main;

        ret = uv_async_init(&rrdeng_main.loop, &rrdeng_main.async, async_cb);
        if (ret) {
            error("DBENGINE: uv_async_init(): %s", uv_strerror(ret));
            fatal_assert(0 == uv_loop_close(&rrdeng_main.loop));
            return false;
        }
        rrdeng_main.async.data = &rrdeng_main;

        ret = uv_timer_init(&rrdeng_main.loop, &rrdeng_main.timer);
        if (ret) {
            error("DBENGINE: uv_timer_init(): %s", uv_strerror(ret));
            uv_close((uv_handle_t *)&rrdeng_main.async, NULL);
            fatal_assert(0 == uv_loop_close(&rrdeng_main.loop));
            return false;
        }
        rrdeng_main.timer.data = &rrdeng_main;

        fatal_assert(0 == uv_thread_create(&rrdeng_main.thread, rrdeng_worker, &rrdeng_main));
        spawned = true;
    }

    ctx->worker_config.now_deleting_files = false;
    ctx->worker_config.migration_to_v2_running = false;
    ctx->worker_config.outstanding_flush_requests = 0;

    return true;
}

void rrdeng_worker(void* arg) {
    sanity_check();
    uv_thread_set_name_np(pthread_self(), "DBENGINE");

    worker_register("DBENGINE");

    // opcode jobs
    worker_register_job_name(RRDENG_OPCODE_NOOP,                                     "noop");

    worker_register_job_name(RRDENG_OPCODE_EXTENT_READ,                              "extent read");
    worker_register_job_name(RRDENG_OPCODE_PREP_QUERY,                               "prep query");
    worker_register_job_name(RRDENG_OPCODE_FLUSH_PAGES,                              "flush pages");
    worker_register_job_name(RRDENG_OPCODE_FLUSHED_TO_OPEN,                          "flushed to open");
    worker_register_job_name(RRDENG_OPCODE_FLUSH_INIT,                               "flush init");
    worker_register_job_name(RRDENG_OPCODE_EVICT_INIT,                               "evict init");
    //worker_register_job_name(RRDENG_OPCODE_DATAFILE_CREATE,                          "datafile create");
    worker_register_job_name(RRDENG_OPCODE_JOURNAL_FILE_INDEX,                       "journal file index");
    worker_register_job_name(RRDENG_OPCODE_DATABASE_ROTATE,                          "db rotate");
    worker_register_job_name(RRDENG_OPCODE_CTX_SHUTDOWN,                             "ctx shutdown");
    worker_register_job_name(RRDENG_OPCODE_CTX_QUIESCE,                              "ctx quiesce");

    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_EXTENT_READ,          "extent read cb");
    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_PREP_QUERY,           "prep query cb");
    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_FLUSH_PAGES,          "flush pages cb");
    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_FLUSHED_TO_OPEN,      "flushed to open cb");
    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_FLUSH_INIT,           "flush init cb");
    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_EVICT_INIT,           "evict init cb");
    //worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_DATAFILE_CREATE,      "datafile create cb");
    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_JOURNAL_FILE_INDEX,   "journal file index cb");
    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_DATABASE_ROTATE,      "db rotate cb");
    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_CTX_SHUTDOWN,         "ctx shutdown cb");
    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_CTX_QUIESCE,          "ctx quiesce cb");

    // special jobs
    worker_register_job_name(RRDENG_TIMER_CB,                                        "timer");
    worker_register_job_name(RRDENG_FLUSH_TRANSACTION_BUFFER_CB,                     "transaction buffer flush cb");

    worker_register_job_custom_metric(RRDENG_OPCODES_WAITING,  "opcodes waiting",  "opcodes", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(RRDENG_WORKS_DISPATCHED, "works dispatched", "works",   WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(RRDENG_WORKS_EXECUTING,  "works executing",  "works",   WORKER_METRIC_ABSOLUTE);

    extent_buffer_init();

    struct rrdeng_main *main = arg;
    enum rrdeng_opcode opcode;
    struct rrdeng_cmd cmd;
    main->tid = gettid();

    fatal_assert(0 == uv_timer_start(&main->timer, timer_cb, TIMER_PERIOD_MS, TIMER_PERIOD_MS));

    bool shutdown = false;
    while (likely(!shutdown)) {
        worker_is_idle();
        uv_run(&main->loop, UV_RUN_DEFAULT);

        /* wait for commands */
        do {
            cmd = rrdeng_deq_cmd();
            opcode = cmd.opcode;

            worker_is_busy(opcode);

            switch (opcode) {
                case RRDENG_OPCODE_EXTENT_READ: {
                    struct rrdengine_instance *ctx = cmd.ctx;
                    EXTENT_PD_LIST *extent_page_list = cmd.data;
                    work_dispatch(ctx, extent_page_list, NULL, opcode, extent_read_tp_worker, after_extent_read);
                    break;
                }

                case RRDENG_OPCODE_PREP_QUERY: {
                    struct rrdengine_instance *ctx = cmd.ctx;
                    PDC *pdc = cmd.data;
                    work_dispatch(ctx, pdc, NULL, opcode, query_prep_tp_worker, after_prep_query);
                    break;
                }

                case RRDENG_OPCODE_FLUSH_PAGES: {
                    struct rrdengine_instance *ctx = cmd.ctx;
                    struct page_descr_with_data *base = cmd.data;
                    struct completion *completion = cmd.completion; // optional
                    cmd.ctx->worker_config.outstanding_flush_requests++;
                    do_flush_extent(ctx, base, completion);
                    break;
                }

                case RRDENG_OPCODE_FLUSHED_TO_OPEN: {
                    struct rrdengine_instance *ctx = cmd.ctx;
                    uv_fs_t *uv_fs_request = cmd.data;
                    struct extent_io_descriptor *xt_io_descr = uv_fs_request->data;
                    struct completion *completion = xt_io_descr->completion;
                    work_dispatch(ctx, uv_fs_request, completion, opcode, extent_flushed_to_open_tp_worker, after_extent_flushed_to_open);
                    break;
                }

                case RRDENG_OPCODE_FLUSH_INIT: {
                    if(!rrdeng_main.flush_running) {

                        rrdeng_main.flush_running = true;
                        if(!work_dispatch(NULL, NULL, NULL, opcode, cache_flush_tp_worker, after_do_cache_flush))
                            rrdeng_main.flush_running = false;

                    }
                    break;
                }

                case RRDENG_OPCODE_EVICT_INIT: {
                    if(!rrdeng_main.evict_running) {

                        rrdeng_main.evict_running = true;
                        if (!work_dispatch(NULL, NULL, NULL, opcode, cache_evict_tp_worker, after_do_cache_evict))
                            rrdeng_main.evict_running = false;

                    }
                    break;
                }

//                case RRDENG_OPCODE_DATAFILE_CREATE: {
//                    struct rrdengine_instance *ctx = cmd.ctx;
//                    struct rrdengine_datafile *datafile = ctx->datafiles.first->prev;
//                    if(datafile->pos > rrdeng_target_data_file_size(ctx) &&
//                       create_new_datafile_pair(ctx, 1, ctx->last_fileno + 1) == 0) {
//                        ++ctx->last_fileno;
//                        rrdeng_enq_cmd(ctx, RRDENG_OPCODE_JOURNAL_FILE_INDEX, datafile, NULL, STORAGE_PRIORITY_CRITICAL);
//                    }
//                    break;
//                }

                case RRDENG_OPCODE_JOURNAL_FILE_INDEX: {
                    struct rrdengine_instance *ctx = cmd.ctx;
                    struct rrdengine_datafile *datafile = cmd.data;
                    if(!ctx->worker_config.migration_to_v2_running) {

                        ctx->worker_config.migration_to_v2_running = true;
                        if (!work_dispatch(ctx, datafile, NULL, opcode, journal_v2_indexing_tp_worker, after_journal_v2_indexing))
                            ctx->worker_config.migration_to_v2_running = false;

                    }
                    break;
                }

                case RRDENG_OPCODE_DATABASE_ROTATE: {
                    struct rrdengine_instance *ctx = cmd.ctx;
                    if (!ctx->worker_config.now_deleting_files &&
                         ctx->datafiles.first->next != NULL &&
                         ctx->datafiles.first->next->next != NULL &&
                         ctx->disk_space > MAX(ctx->max_disk_space, 2 * ctx->metric_API_max_producers * RRDENG_BLOCK_SIZE)) {

                        ctx->worker_config.now_deleting_files = true;
                        if(!work_dispatch(ctx, NULL, NULL, opcode, database_rotate_tp_worker, after_database_rotate))
                            ctx->worker_config.now_deleting_files = false;

                    }
                    break;
                }

                case RRDENG_OPCODE_CTX_QUIESCE: {
                    // a ctx will shutdown shortly
                    struct rrdengine_instance *ctx = cmd.ctx; (void)ctx;
                    // FIXME - ktsaou - flush all pages of this ctx (section)
                    break;
                }

                case RRDENG_OPCODE_CTX_SHUTDOWN: {
                    // a ctx is shutting down
                    struct rrdengine_instance *ctx = cmd.ctx;
                    struct completion *completion = cmd.completion;

                    // FIXME - ktsaou - make sure we cleanup all requests for this ctx

                    if(ctx->worker_config.outstanding_flush_requests)
                        // spin
                        rrdeng_enq_cmd(ctx, opcode, NULL, completion, STORAGE_PRIORITY_BEST_EFFORT);
                    else
                        // done
                        completion_mark_complete(completion);

                    break;
                }

                case RRDENG_OPCODE_NOOP: {
                    /* the command queue was empty, do nothing */
                    break;
                }

                // not opcodes
                case RRDENG_OPCODE_MAX:
                default: {
                    internal_fatal(true, "DBENGINE: unknown opcode");
                    break;
                }
            }

        } while (opcode != RRDENG_OPCODE_NOOP);
    }

    /* cleanup operations of the event loop */
    info("DBENGINE: shutting down dbengine thread");

    /*
     * uv_async_send after uv_close does not seem to crash in linux at the moment,
     * it is however undocumented behaviour and we need to be aware if this becomes
     * an issue in the future.
     */
    uv_close((uv_handle_t *)&main->async, NULL);
    uv_timer_stop(&main->timer);
    uv_close((uv_handle_t *)&main->timer, NULL);
    uv_run(&main->loop, UV_RUN_DEFAULT);
    uv_loop_close(&main->loop);
    worker_unregister();
}

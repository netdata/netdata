// SPDX-License-Identifier: GPL-3.0-or-later
#define NETDATA_RRD_INTERNALS

#include "rrdengine.h"
#include "pdc.h"

rrdeng_stats_t global_io_errors = 0;
rrdeng_stats_t global_fs_errors = 0;
rrdeng_stats_t rrdeng_reserved_file_descriptors = 0;
rrdeng_stats_t global_pg_cache_over_half_dirty_events = 0;
rrdeng_stats_t global_flushing_pressure_page_deletions = 0;

unsigned rrdeng_pages_per_extent = MAX_PAGES_PER_EXTENT;

#if WORKER_UTILIZATION_MAX_JOB_TYPES < (RRDENG_OPCODE_MAX + 2)
#error Please increase WORKER_UTILIZATION_MAX_JOB_TYPES to at least (RRDENG_MAX_OPCODE + 2)
#endif

struct rrdeng_cmd {
    struct rrdengine_instance *ctx;
    enum rrdeng_opcode opcode;
    void *data;
    struct completion *completion;
    enum storage_priority priority;
    dequeue_callback_t dequeue_cb;

    struct {
        struct rrdeng_cmd *prev;
        struct rrdeng_cmd *next;
    } queue;
};

static inline struct rrdeng_cmd rrdeng_deq_cmd(bool from_worker);
static inline void worker_dispatch_extent_read(struct rrdeng_cmd cmd, bool from_worker);
static inline void worker_dispatch_query_prep(struct rrdeng_cmd cmd, bool from_worker);

struct rrdeng_main {
    uv_thread_t thread;
    uv_loop_t loop;
    uv_async_t async;
    uv_timer_t timer;
    pid_t tid;

    size_t flushes_running;
    size_t evictions_running;
    size_t cleanup_running;

    struct {
        ARAL *ar;

        struct {
            SPINLOCK spinlock;

            size_t waiting;
            struct rrdeng_cmd *waiting_items_by_priority[STORAGE_PRIORITY_INTERNAL_MAX_DONT_USE];
            size_t executed_by_priority[STORAGE_PRIORITY_INTERNAL_MAX_DONT_USE];
        } unsafe;
    } cmd_queue;

    struct {
        ARAL *ar;

        struct {
            size_t dispatched;
            size_t executing;
        } atomics;
    } work_cmd;

    struct {
        ARAL *ar;
    } handles;

    struct {
        ARAL *ar;
    } descriptors;

    struct {
        ARAL *ar;
    } xt_io_descr;

} rrdeng_main = {
        .thread = 0,
        .loop = {},
        .async = {},
        .timer = {},
        .flushes_running = 0,
        .evictions_running = 0,
        .cleanup_running = 0,

        .cmd_queue = {
                .unsafe = {
                        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                },
        }
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

// ----------------------------------------------------------------------------
// work request cache

typedef void *(*work_cb)(struct rrdengine_instance *ctx, void *data, struct completion *completion, uv_work_t* req);
typedef void (*after_work_cb)(struct rrdengine_instance *ctx, void *data, struct completion *completion, uv_work_t* req, int status);

struct rrdeng_work {
    uv_work_t req;

    struct rrdengine_instance *ctx;
    void *data;
    struct completion *completion;

    work_cb work_cb;
    after_work_cb after_work_cb;
    enum rrdeng_opcode opcode;
};

static void work_request_init(void) {
    rrdeng_main.work_cmd.ar = aral_create(
            "dbengine-work-cmd",
            sizeof(struct rrdeng_work),
            0,
            65536, NULL,
            NULL, NULL, false, false
    );
}

enum LIBUV_WORKERS_STATUS {
    LIBUV_WORKERS_RELAXED,
    LIBUV_WORKERS_STRESSED,
    LIBUV_WORKERS_CRITICAL,
};

static inline enum LIBUV_WORKERS_STATUS work_request_full(void) {
    size_t dispatched = __atomic_load_n(&rrdeng_main.work_cmd.atomics.dispatched, __ATOMIC_RELAXED);

    if(dispatched >= (size_t)(libuv_worker_threads))
        return LIBUV_WORKERS_CRITICAL;

    else if(dispatched >= (size_t)(libuv_worker_threads - RESERVED_LIBUV_WORKER_THREADS))
        return LIBUV_WORKERS_STRESSED;

    return LIBUV_WORKERS_RELAXED;
}

static inline void work_done(struct rrdeng_work *work_request) {
    aral_freez(rrdeng_main.work_cmd.ar, work_request);
}

static void work_standard_worker(uv_work_t *req) {
    __atomic_add_fetch(&rrdeng_main.work_cmd.atomics.executing, 1, __ATOMIC_RELAXED);

    register_libuv_worker_jobs();
    worker_is_busy(UV_EVENT_WORKER_INIT);

    struct rrdeng_work *work_request = req->data;

    work_request->data = work_request->work_cb(work_request->ctx, work_request->data, work_request->completion, req);
    worker_is_idle();

    if(work_request->opcode == RRDENG_OPCODE_EXTENT_READ || work_request->opcode == RRDENG_OPCODE_QUERY) {
        internal_fatal(work_request->after_work_cb != NULL, "DBENGINE: opcodes with a callback should not boosted");

        while(1) {
            struct rrdeng_cmd cmd = rrdeng_deq_cmd(true);
            if (cmd.opcode == RRDENG_OPCODE_NOOP)
                break;

            worker_is_busy(UV_EVENT_WORKER_INIT);
            switch (cmd.opcode) {
                case RRDENG_OPCODE_EXTENT_READ:
                    worker_dispatch_extent_read(cmd, true);
                    break;

                case RRDENG_OPCODE_QUERY:
                    worker_dispatch_query_prep(cmd, true);
                    break;

                default:
                    fatal("DBENGINE: Opcode should not be executed synchronously");
                    break;
            }
            worker_is_idle();
        }
    }

    __atomic_sub_fetch(&rrdeng_main.work_cmd.atomics.dispatched, 1, __ATOMIC_RELAXED);
    __atomic_sub_fetch(&rrdeng_main.work_cmd.atomics.executing, 1, __ATOMIC_RELAXED);

    // signal the event loop a worker is available
    fatal_assert(0 == uv_async_send(&rrdeng_main.async));
}

static void after_work_standard_callback(uv_work_t* req, int status) {
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

    work_request = aral_mallocz(rrdeng_main.work_cmd.ar);
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

    __atomic_add_fetch(&rrdeng_main.work_cmd.atomics.dispatched, 1, __ATOMIC_RELAXED);

    return true;
}

// ----------------------------------------------------------------------------
// page descriptor cache

void page_descriptors_init(void) {
    rrdeng_main.descriptors.ar = aral_create(
            "dbengine-descriptors",
            sizeof(struct page_descr_with_data),
            0,
            65536 * 4,
            NULL,
            NULL, NULL, false, false);
}

struct page_descr_with_data *page_descriptor_get(void) {
    struct page_descr_with_data *descr = aral_mallocz(rrdeng_main.descriptors.ar);
    memset(descr, 0, sizeof(struct page_descr_with_data));
    return descr;
}

static inline void page_descriptor_release(struct page_descr_with_data *descr) {
    aral_freez(rrdeng_main.descriptors.ar, descr);
}

// ----------------------------------------------------------------------------
// extent io descriptor cache

static void extent_io_descriptor_init(void) {
    rrdeng_main.xt_io_descr.ar = aral_create(
            "dbengine-extent-io",
            sizeof(struct extent_io_descriptor),
            0,
            65536,
            NULL,
            NULL, NULL, false, false
            );
}

static struct extent_io_descriptor *extent_io_descriptor_get(void) {
    struct extent_io_descriptor *xt_io_descr = aral_mallocz(rrdeng_main.xt_io_descr.ar);
    memset(xt_io_descr, 0, sizeof(struct extent_io_descriptor));
    return xt_io_descr;
}

static inline void extent_io_descriptor_release(struct extent_io_descriptor *xt_io_descr) {
    aral_freez(rrdeng_main.xt_io_descr.ar, xt_io_descr);
}

// ----------------------------------------------------------------------------
// query handle cache

void rrdeng_query_handle_init(void) {
    rrdeng_main.handles.ar = aral_create(
            "dbengine-query-handles",
            sizeof(struct rrdeng_query_handle),
            0,
            65536,
            NULL,
            NULL, NULL, false, false);
}

struct rrdeng_query_handle *rrdeng_query_handle_get(void) {
    struct rrdeng_query_handle *handle = aral_mallocz(rrdeng_main.handles.ar);
    memset(handle, 0, sizeof(struct rrdeng_query_handle));
    return handle;
}

void rrdeng_query_handle_release(struct rrdeng_query_handle *handle) {
    aral_freez(rrdeng_main.handles.ar, handle);
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

static void wal_cleanup1(void) {
    WAL *wal = NULL;

    if(!netdata_spinlock_trylock(&wal_globals.protected.spinlock))
        return;

    if(wal_globals.protected.available_items && wal_globals.protected.available > storage_tiers) {
        wal = wal_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(wal_globals.protected.available_items, wal, cache.prev, cache.next);
        wal_globals.protected.available--;
    }

    netdata_spinlock_unlock(&wal_globals.protected.spinlock);

    if(wal) {
        posix_memfree(wal->buf);
        freez(wal);
        __atomic_sub_fetch(&wal_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
    }
}

WAL *wal_get(struct rrdengine_instance *ctx, unsigned size) {
    if(!size || size > RRDENG_BLOCK_SIZE)
        fatal("DBENGINE: invalid WAL size requested");

    WAL *wal = NULL;

    netdata_spinlock_lock(&wal_globals.protected.spinlock);

    if(likely(wal_globals.protected.available_items)) {
        wal = wal_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(wal_globals.protected.available_items, wal, cache.prev, cache.next);
        wal_globals.protected.available--;
    }

    uint64_t transaction_id = __atomic_fetch_add(&ctx->atomic.transaction_id, 1, __ATOMIC_RELAXED);
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
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(wal_globals.protected.available_items, wal, cache.prev, cache.next);
    wal_globals.protected.available++;
    netdata_spinlock_unlock(&wal_globals.protected.spinlock);
}

// ----------------------------------------------------------------------------
// command queue cache

static void rrdeng_cmd_queue_init(void) {
    rrdeng_main.cmd_queue.ar = aral_create("dbengine-opcodes",
                                           sizeof(struct rrdeng_cmd),
                                           0,
                                           65536,
                                           NULL,
                                           NULL, NULL, false, false);
}

static inline STORAGE_PRIORITY rrdeng_enq_cmd_map_opcode_to_priority(enum rrdeng_opcode opcode, STORAGE_PRIORITY priority) {
    if(unlikely(priority >= STORAGE_PRIORITY_INTERNAL_MAX_DONT_USE))
        priority = STORAGE_PRIORITY_BEST_EFFORT;

    switch(opcode) {
        case RRDENG_OPCODE_QUERY:
            priority = STORAGE_PRIORITY_INTERNAL_QUERY_PREP;
            break;

        default:
            break;
    }

    return priority;
}

void rrdeng_enqueue_epdl_cmd(struct rrdeng_cmd *cmd) {
    epdl_cmd_queued(cmd->data, cmd);
}

void rrdeng_dequeue_epdl_cmd(struct rrdeng_cmd *cmd) {
    epdl_cmd_dequeued(cmd->data);
}

void rrdeng_req_cmd(requeue_callback_t get_cmd_cb, void *data, STORAGE_PRIORITY priority) {
    netdata_spinlock_lock(&rrdeng_main.cmd_queue.unsafe.spinlock);

    struct rrdeng_cmd *cmd = get_cmd_cb(data);
    if(cmd) {
        priority = rrdeng_enq_cmd_map_opcode_to_priority(cmd->opcode, priority);

        if (cmd->priority > priority) {
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(rrdeng_main.cmd_queue.unsafe.waiting_items_by_priority[cmd->priority], cmd, queue.prev, queue.next);
            DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(rrdeng_main.cmd_queue.unsafe.waiting_items_by_priority[priority], cmd, queue.prev, queue.next);
            cmd->priority = priority;
        }
    }

    netdata_spinlock_unlock(&rrdeng_main.cmd_queue.unsafe.spinlock);
}

void rrdeng_enq_cmd(struct rrdengine_instance *ctx, enum rrdeng_opcode opcode, void *data, struct completion *completion,
               enum storage_priority priority, enqueue_callback_t enqueue_cb, dequeue_callback_t dequeue_cb) {

    priority = rrdeng_enq_cmd_map_opcode_to_priority(opcode, priority);

    struct rrdeng_cmd *cmd = aral_mallocz(rrdeng_main.cmd_queue.ar);
    memset(cmd, 0, sizeof(struct rrdeng_cmd));
    cmd->ctx = ctx;
    cmd->opcode = opcode;
    cmd->data = data;
    cmd->completion = completion;
    cmd->priority = priority;
    cmd->dequeue_cb = dequeue_cb;

    netdata_spinlock_lock(&rrdeng_main.cmd_queue.unsafe.spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(rrdeng_main.cmd_queue.unsafe.waiting_items_by_priority[priority], cmd, queue.prev, queue.next);
    rrdeng_main.cmd_queue.unsafe.waiting++;
    if(enqueue_cb)
        enqueue_cb(cmd);
    netdata_spinlock_unlock(&rrdeng_main.cmd_queue.unsafe.spinlock);

    fatal_assert(0 == uv_async_send(&rrdeng_main.async));
}

static inline bool rrdeng_cmd_has_waiting_opcodes_in_lower_priorities(STORAGE_PRIORITY priority, STORAGE_PRIORITY max_priority) {
    for(; priority <= max_priority ; priority++)
        if(rrdeng_main.cmd_queue.unsafe.waiting_items_by_priority[priority])
            return true;

    return false;
}

#define opcode_empty (struct rrdeng_cmd) {      \
    .ctx = NULL,                                \
    .opcode = RRDENG_OPCODE_NOOP,               \
    .priority = STORAGE_PRIORITY_BEST_EFFORT,   \
    .completion = NULL,                         \
    .data = NULL,                               \
}

static inline struct rrdeng_cmd rrdeng_deq_cmd(bool from_worker) {
    struct rrdeng_cmd *cmd = NULL;
    enum LIBUV_WORKERS_STATUS status = work_request_full();

    STORAGE_PRIORITY min_priority, max_priority;
    min_priority = STORAGE_PRIORITY_INTERNAL_DBENGINE;
    max_priority = (status != LIBUV_WORKERS_RELAXED) ? STORAGE_PRIORITY_INTERNAL_DBENGINE : STORAGE_PRIORITY_INTERNAL_MAX_DONT_USE - 1;

    if(from_worker) {
        if(status == LIBUV_WORKERS_CRITICAL)
            return opcode_empty;

        min_priority = STORAGE_PRIORITY_INTERNAL_QUERY_PREP;
        max_priority = STORAGE_PRIORITY_BEST_EFFORT;
    }

    // find an opcode to execute from the queue
    netdata_spinlock_lock(&rrdeng_main.cmd_queue.unsafe.spinlock);
    for(STORAGE_PRIORITY priority = min_priority; priority <= max_priority ; priority++) {
        cmd = rrdeng_main.cmd_queue.unsafe.waiting_items_by_priority[priority];
        if(cmd) {

            // avoid starvation of lower priorities
            if(unlikely(priority >= STORAGE_PRIORITY_HIGH &&
                        priority < STORAGE_PRIORITY_BEST_EFFORT &&
                        ++rrdeng_main.cmd_queue.unsafe.executed_by_priority[priority] % 50 == 0 &&
                        rrdeng_cmd_has_waiting_opcodes_in_lower_priorities(priority + 1, max_priority))) {
                // let the others run 2% of the requests
                cmd = NULL;
                continue;
            }

            // remove it from the queue
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(rrdeng_main.cmd_queue.unsafe.waiting_items_by_priority[priority], cmd, queue.prev, queue.next);
            rrdeng_main.cmd_queue.unsafe.waiting--;
            break;
        }
    }

    if(cmd && cmd->dequeue_cb) {
        cmd->dequeue_cb(cmd);
        cmd->dequeue_cb = NULL;
    }

    netdata_spinlock_unlock(&rrdeng_main.cmd_queue.unsafe.spinlock);

    struct rrdeng_cmd ret;
    if(cmd) {
        // copy it, to return it
        ret = *cmd;

        aral_freez(rrdeng_main.cmd_queue.ar, cmd);
    }
    else
        ret = opcode_empty;

    return ret;
}


// ----------------------------------------------------------------------------

struct {
    ARAL *aral[RRD_STORAGE_TIERS];
} dbengine_page_alloc_globals = {};

static inline ARAL *page_size_lookup(size_t size) {
    for(size_t tier = 0; tier < storage_tiers ;tier++)
        if(size == tier_page_size[tier])
            return dbengine_page_alloc_globals.aral[tier];

    return NULL;
}

static void dbengine_page_alloc_init(void) {
    for(size_t i = storage_tiers; i > 0 ;i--) {
        size_t tier = storage_tiers - i;

        char buf[20 + 1];
        snprintfz(buf, 20, "tier%zu-pages", tier);

        dbengine_page_alloc_globals.aral[tier] = aral_create(
                buf,
                tier_page_size[tier],
                64,
                512 * tier_page_size[tier],
                pgc_aral_statistics(),
                NULL, NULL, false, false);
    }
}

void *dbengine_page_alloc(size_t size) {
    ARAL *ar = page_size_lookup(size);
    if(ar) return aral_mallocz(ar);

    return mallocz(size);
}

void dbengine_page_free(void *page, size_t size __maybe_unused) {
    if(unlikely(!page || page == DBENGINE_EMPTY_PAGE))
        return;

    ARAL *ar = page_size_lookup(size);
    if(ar)
        aral_freez(ar, page);
    else
        freez(page);
}

// ----------------------------------------------------------------------------

void *dbengine_extent_alloc(size_t size) {
    void *extent = mallocz(size);
    return extent;
}

void dbengine_extent_free(void *extent, size_t size __maybe_unused) {
    freez(extent);
}

static void journalfile_extent_build(struct rrdengine_instance *ctx, struct extent_io_descriptor *xt_io_descr) {
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
    if(completion)
        completion_mark_complete(completion);

    if(ctx_is_available_for_queries(ctx))
        rrdeng_enq_cmd(ctx, RRDENG_OPCODE_DATABASE_ROTATE, NULL, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);
}

static void *extent_flushed_to_open_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    worker_is_busy(UV_EVENT_DBENGINE_FLUSHED_TO_OPEN);

    uv_fs_t *uv_fs_request = data;
    struct extent_io_descriptor *xt_io_descr = uv_fs_request->data;
    struct page_descr_with_data *descr;
    struct rrdengine_datafile *datafile;
    unsigned i;

    datafile = xt_io_descr->datafile;

    bool still_running = ctx_is_available_for_queries(ctx);

    for (i = 0 ; i < xt_io_descr->descr_count ; ++i) {
        descr = xt_io_descr->descr_array[i];

        if (likely(still_running))
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

    if(datafile->fileno != ctx_last_fileno_get(ctx) && still_running)
        // we just finished a flushing on a datafile that is not the active one
        rrdeng_enq_cmd(ctx, RRDENG_OPCODE_JOURNAL_INDEX, datafile, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);

    return data;
}

// Main event loop callback
static void after_extent_write_datafile_io(uv_fs_t *uv_fs_request) {
    worker_is_busy(RRDENG_OPCODE_MAX + RRDENG_OPCODE_EXTENT_WRITE);

    struct extent_io_descriptor *xt_io_descr = uv_fs_request->data;
    struct rrdengine_datafile *datafile = xt_io_descr->datafile;
    struct rrdengine_instance *ctx = datafile->ctx;

    if (uv_fs_request->result < 0) {
        ctx_io_error(ctx);
        error("DBENGINE: %s: uv_fs_write(): %s", __func__, uv_strerror((int)uv_fs_request->result));
    }

    journalfile_v1_extent_write(ctx, xt_io_descr->datafile, xt_io_descr->wal, &rrdeng_main.loop);

    netdata_spinlock_lock(&datafile->writers.spinlock);
    datafile->writers.running--;
    datafile->writers.flushed_to_open_running++;
    netdata_spinlock_unlock(&datafile->writers.spinlock);

    rrdeng_enq_cmd(xt_io_descr->ctx,
                   RRDENG_OPCODE_FLUSHED_TO_OPEN,
                   uv_fs_request,
                   xt_io_descr->completion,
                   STORAGE_PRIORITY_INTERNAL_DBENGINE,
                   NULL,
                   NULL);

    worker_is_idle();
}

static bool datafile_is_full(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile) {
    bool ret = false;
    netdata_spinlock_lock(&datafile->writers.spinlock);

    if(ctx_is_available_for_queries(ctx) && datafile->pos > rrdeng_target_data_file_size(ctx))
        ret = true;

    netdata_spinlock_unlock(&datafile->writers.spinlock);

    return ret;
}

static struct rrdengine_datafile *get_datafile_to_write_extent(struct rrdengine_instance *ctx) {
    struct rrdengine_datafile *datafile;

    // get the latest datafile
    uv_rwlock_rdlock(&ctx->datafiles.rwlock);
    datafile = ctx->datafiles.first->prev;
    // become a writer on this datafile, to prevent it from vanishing
    netdata_spinlock_lock(&datafile->writers.spinlock);
    datafile->writers.running++;
    netdata_spinlock_unlock(&datafile->writers.spinlock);
    uv_rwlock_rdunlock(&ctx->datafiles.rwlock);

    if(datafile_is_full(ctx, datafile)) {
        // remember the datafile we have become writers to
        struct rrdengine_datafile *old_datafile = datafile;

        // only 1 datafile creation at a time
        static netdata_mutex_t mutex = NETDATA_MUTEX_INITIALIZER;
        netdata_mutex_lock(&mutex);

        // take the latest datafile again - without this, multiple threads may create multiple files
        uv_rwlock_rdlock(&ctx->datafiles.rwlock);
        datafile = ctx->datafiles.first->prev;
        uv_rwlock_rdunlock(&ctx->datafiles.rwlock);

        if(datafile_is_full(ctx, datafile) && create_new_datafile_pair(ctx) == 0)
            rrdeng_enq_cmd(ctx, RRDENG_OPCODE_JOURNAL_INDEX, datafile, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL,
                           NULL);

        netdata_mutex_unlock(&mutex);

        // get the new latest datafile again, like above
        uv_rwlock_rdlock(&ctx->datafiles.rwlock);
        datafile = ctx->datafiles.first->prev;
        // become a writer on this datafile, to prevent it from vanishing
        netdata_spinlock_lock(&datafile->writers.spinlock);
        datafile->writers.running++;
        netdata_spinlock_unlock(&datafile->writers.spinlock);
        uv_rwlock_rdunlock(&ctx->datafiles.rwlock);

        // release the writers on the old datafile
        netdata_spinlock_lock(&old_datafile->writers.spinlock);
        old_datafile->writers.running--;
        netdata_spinlock_unlock(&old_datafile->writers.spinlock);
    }

    return datafile;
}

/*
 * Take a page list in a judy array and write them
 */
static struct extent_io_descriptor *datafile_extent_build(struct rrdengine_instance *ctx, struct page_descr_with_data *base, struct completion *completion) {
    int ret;
    int compressed_size, max_compressed_size = 0;
    unsigned i, count, size_bytes, pos, real_io_size;
    uint32_t uncompressed_payload_length, payload_offset;
    struct page_descr_with_data *descr, *eligible_pages[MAX_PAGES_PER_EXTENT];
    struct extent_io_descriptor *xt_io_descr;
    struct extent_buffer *eb = NULL;
    void *compressed_buf = NULL;
    Word_t Index;
    uint8_t compression_algorithm = ctx->config.global_compress_alg;
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

        __atomic_sub_fetch(&ctx->atomic.extents_currently_being_flushed, 1, __ATOMIC_RELAXED);
        return NULL;
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

    if(likely(compression_algorithm == RRD_LZ4)) {
        compressed_size = LZ4_compress_default(
                xt_io_descr->buf + payload_offset,
                compressed_buf,
                (int)uncompressed_payload_length,
                max_compressed_size);

        __atomic_add_fetch(&ctx->stats.before_compress_bytes, uncompressed_payload_length, __ATOMIC_RELAXED);
        __atomic_add_fetch(&ctx->stats.after_compress_bytes, compressed_size, __ATOMIC_RELAXED);

        (void) memcpy(xt_io_descr->buf + payload_offset, compressed_buf, compressed_size);
        extent_buffer_release(eb);
        size_bytes = payload_offset + compressed_size + sizeof(*trailer);
        header->payload_length = compressed_size;
    }
    else { // RRD_NO_COMPRESSION
        header->payload_length = uncompressed_payload_length;
    }

    real_io_size = ALIGN_BYTES_CEILING(size_bytes);

    datafile = get_datafile_to_write_extent(ctx);
    netdata_spinlock_lock(&datafile->writers.spinlock);
    xt_io_descr->datafile = datafile;
    xt_io_descr->pos = datafile->pos;
    datafile->pos += real_io_size;
    netdata_spinlock_unlock(&datafile->writers.spinlock);

    xt_io_descr->bytes = size_bytes;
    xt_io_descr->uv_fs_request.data = xt_io_descr;
    xt_io_descr->completion = completion;

    trailer = xt_io_descr->buf + size_bytes - sizeof(*trailer);
    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, xt_io_descr->buf, size_bytes - sizeof(*trailer));
    crc32set(trailer->checksum, crc);

    xt_io_descr->iov = uv_buf_init((void *)xt_io_descr->buf, real_io_size);
    journalfile_extent_build(ctx, xt_io_descr);

    ctx_last_flush_fileno_set(ctx, datafile->fileno);
    ctx_current_disk_space_increase(ctx, real_io_size);
    ctx_io_write_op_bytes(ctx, real_io_size);

    return xt_io_descr;
}

static void after_extent_write(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* uv_work_req __maybe_unused, int status __maybe_unused) {
    struct extent_io_descriptor *xt_io_descr = data;

    if(xt_io_descr) {
        int ret = uv_fs_write(&rrdeng_main.loop,
                              &xt_io_descr->uv_fs_request,
                              xt_io_descr->datafile->file,
                              &xt_io_descr->iov,
                              1,
                              (int64_t) xt_io_descr->pos,
                              after_extent_write_datafile_io);

        fatal_assert(-1 != ret);
    }
}

static void *extent_write_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    worker_is_busy(UV_EVENT_DBENGINE_EXTENT_WRITE);
    struct page_descr_with_data *base = data;
    struct extent_io_descriptor *xt_io_descr = datafile_extent_build(ctx, base, completion);
    return xt_io_descr;
}

static void after_database_rotate(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    __atomic_store_n(&ctx->atomic.now_deleting_files, false, __ATOMIC_RELAXED);
}

struct uuid_first_time_s {
    uuid_t *uuid;
    time_t first_time_s;
    METRIC *metric;
    size_t pages_found;
    size_t df_matched;
    size_t df_index_oldest;
};

struct rrdengine_datafile *datafile_release_and_acquire_next_for_retention(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile) {

    uv_rwlock_rdlock(&ctx->datafiles.rwlock);

    struct rrdengine_datafile *next_datafile = datafile->next;

    while(next_datafile && !datafile_acquire(next_datafile, DATAFILE_ACQUIRE_RETENTION))
        next_datafile = next_datafile->next;

    uv_rwlock_rdunlock(&ctx->datafiles.rwlock);

    datafile_release(datafile, DATAFILE_ACQUIRE_RETENTION);

    return next_datafile;
}

void find_uuid_first_time(
    struct rrdengine_instance *ctx,
    struct rrdengine_datafile *datafile,
    struct uuid_first_time_s *uuid_first_entry_list,
    size_t count)
{
    // acquire the datafile to work with it
    uv_rwlock_rdlock(&ctx->datafiles.rwlock);
    while(datafile && !datafile_acquire(datafile, DATAFILE_ACQUIRE_RETENTION))
        datafile = datafile->next;
    uv_rwlock_rdunlock(&ctx->datafiles.rwlock);

    if (unlikely(!datafile))
        return;

    unsigned journalfile_count = 0;
    size_t binary_match = 0;
    size_t not_matching_bsearches = 0;

    while (datafile) {
        struct journal_v2_header *j2_header = journalfile_v2_data_acquire(datafile->journalfile, NULL, 0, 0);
        if (!j2_header) {
            datafile = datafile_release_and_acquire_next_for_retention(ctx, datafile);
            continue;
        }

        time_t journal_start_time_s = (time_t) (j2_header->start_time_ut / USEC_PER_SEC);
        struct journal_metric_list *uuid_list = (struct journal_metric_list *)((uint8_t *) j2_header + j2_header->metric_offset);
        struct uuid_first_time_s *uuid_original_entry;

        size_t journal_metric_count = j2_header->metric_count;

        for (size_t index = 0; index < count; ++index) {
            uuid_original_entry = &uuid_first_entry_list[index];

            // Check here if we should skip this
            if (uuid_original_entry->df_matched > 3 || uuid_original_entry->pages_found > 5)
                continue;

            struct journal_metric_list *live_entry =
                    bsearch(uuid_original_entry->uuid,uuid_list,journal_metric_count,
                            sizeof(*uuid_list), journal_metric_uuid_compare);

            if (!live_entry) {
                // Not found in this journal
                not_matching_bsearches++;
                continue;
            }

            uuid_original_entry->pages_found += live_entry->entries;
            uuid_original_entry->df_matched++;

            time_t old_first_time_s = uuid_original_entry->first_time_s;

            // Calculate first / last for this match
            time_t first_time_s = live_entry->delta_start_s + journal_start_time_s;
            uuid_original_entry->first_time_s = MIN(uuid_original_entry->first_time_s, first_time_s);

            if (uuid_original_entry->first_time_s != old_first_time_s)
                uuid_original_entry->df_index_oldest = uuid_original_entry->df_matched;

            binary_match++;
        }

        journalfile_count++;
        journalfile_v2_data_release(datafile->journalfile);
        datafile = datafile_release_and_acquire_next_for_retention(ctx, datafile);
    }

    // Let's scan the open cache for almost exact match
    size_t open_cache_count = 0;

    size_t df_index[10] = { 0 };
    size_t without_metric = 0;
    size_t open_cache_gave_first_time_s = 0;
    size_t metric_count = 0;
    size_t without_retention = 0;
    size_t not_needed_bsearches = 0;

    for (size_t index = 0; index < count; ++index) {
        struct uuid_first_time_s *uuid_first_t_entry = &uuid_first_entry_list[index];

        metric_count++;

        size_t idx = uuid_first_t_entry->df_index_oldest;
        if(idx >= 10)
            idx = 9;

        df_index[idx]++;

        not_needed_bsearches += uuid_first_t_entry->df_matched - uuid_first_t_entry->df_index_oldest;

        if (unlikely(!uuid_first_t_entry->metric)) {
            without_metric++;
            continue;
        }

        PGC_PAGE *page = pgc_page_get_and_acquire(
                open_cache, (Word_t)ctx,
                (Word_t)uuid_first_t_entry->metric, 0,
                PGC_SEARCH_FIRST);

        if (page) {
            time_t old_first_time_s = uuid_first_t_entry->first_time_s;

            time_t first_time_s = pgc_page_start_time_s(page);
            uuid_first_t_entry->first_time_s = MIN(uuid_first_t_entry->first_time_s, first_time_s);
            pgc_page_release(open_cache, page);
            open_cache_count++;

            if(uuid_first_t_entry->first_time_s != old_first_time_s) {
                open_cache_gave_first_time_s++;
            }
        }
        else {
            if(!uuid_first_t_entry->df_index_oldest)
                without_retention++;
        }
    }
    internal_error(true,
         "DBENGINE: analyzed the retention of %zu rotated metrics of tier %d, "
         "did %zu jv2 matching binary searches (%zu not matching, %zu overflown) in %u journal files, "
         "%zu metrics with entries in open cache, "
         "metrics first time found per datafile index ([not in jv2]:%zu, [1]:%zu, [2]:%zu, [3]:%zu, [4]:%zu, [5]:%zu, [6]:%zu, [7]:%zu, [8]:%zu, [bigger]: %zu), "
         "open cache found first time %zu, "
         "metrics without any remaining retention %zu, "
         "metrics not in MRG %zu",
         metric_count,
         ctx->config.tier,
         binary_match,
         not_matching_bsearches,
         not_needed_bsearches,
         journalfile_count,
         open_cache_count,
         df_index[0], df_index[1], df_index[2], df_index[3], df_index[4], df_index[5], df_index[6], df_index[7], df_index[8], df_index[9],
         open_cache_gave_first_time_s,
         without_retention,
         without_metric
    );
}

static void update_metrics_first_time_s(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile_to_delete, struct rrdengine_datafile *first_datafile_remaining, bool worker) {
    if(worker)
        worker_is_busy(UV_EVENT_DBENGINE_FIND_ROTATED_METRICS);

    struct rrdengine_journalfile *journalfile = datafile_to_delete->journalfile;
    struct journal_v2_header *j2_header = journalfile_v2_data_acquire(journalfile, NULL, 0, 0);

    if (unlikely(!j2_header)) {
        if (worker)
            worker_is_idle();
        return;
    }

    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.metrics_retention_started, 1, __ATOMIC_RELAXED);

    struct journal_metric_list *uuid_list = (struct journal_metric_list *)((uint8_t *) j2_header + j2_header->metric_offset);

    size_t count = j2_header->metric_count;
    struct uuid_first_time_s *uuid_first_t_entry;
    struct uuid_first_time_s *uuid_first_entry_list = callocz(count, sizeof(struct uuid_first_time_s));

    size_t added = 0;
    for (size_t index = 0; index < count; ++index) {
        METRIC *metric = mrg_metric_get_and_acquire(main_mrg, &uuid_list[index].uuid, (Word_t) ctx);
        if (!metric)
            continue;

        uuid_first_entry_list[added].metric = metric;
        uuid_first_entry_list[added].first_time_s = LONG_MAX;
        uuid_first_entry_list[added].df_matched = 0;
        uuid_first_entry_list[added].df_index_oldest = 0;
        uuid_first_entry_list[added].uuid = mrg_metric_uuid(main_mrg, metric);
        added++;
    }

    info("DBENGINE: recalculating tier %d retention for %zu metrics starting with datafile %u",
         ctx->config.tier, count, first_datafile_remaining->fileno);

    journalfile_v2_data_release(journalfile);

    // Update the first time / last time for all metrics we plan to delete

    if(worker)
        worker_is_busy(UV_EVENT_DBENGINE_FIND_REMAINING_RETENTION);

    find_uuid_first_time(ctx, first_datafile_remaining, uuid_first_entry_list, added);

    if(worker)
        worker_is_busy(UV_EVENT_DBENGINE_POPULATE_MRG);

    info("DBENGINE: updating tier %d metrics registry retention for %zu metrics",
         ctx->config.tier, added);

    size_t deleted_metrics = 0, zero_retention_referenced = 0, zero_disk_retention = 0, zero_disk_but_live = 0;
    for (size_t index = 0; index < added; ++index) {
        uuid_first_t_entry = &uuid_first_entry_list[index];
        if (likely(uuid_first_t_entry->first_time_s != LONG_MAX)) {
            mrg_metric_set_first_time_s_if_bigger(main_mrg, uuid_first_t_entry->metric, uuid_first_t_entry->first_time_s);
            mrg_metric_release(main_mrg, uuid_first_t_entry->metric);
        }
        else {
            zero_disk_retention++;

            // there is no retention for this metric
            bool has_retention = mrg_metric_zero_disk_retention(main_mrg, uuid_first_t_entry->metric);
            if (!has_retention) {
                bool deleted = mrg_metric_release_and_delete(main_mrg, uuid_first_t_entry->metric);
                if(deleted)
                    deleted_metrics++;
                else
                    zero_retention_referenced++;
            }
            else {
                zero_disk_but_live++;
                mrg_metric_release(main_mrg, uuid_first_t_entry->metric);
            }
        }
    }
    freez(uuid_first_entry_list);

    internal_error(zero_disk_retention,
                   "DBENGINE: deleted %zu metrics, zero retention but referenced %zu (out of %zu total, of which %zu have main cache retention) zero on-disk retention tier %d metrics from metrics registry",
                   deleted_metrics, zero_retention_referenced, zero_disk_retention, zero_disk_but_live, ctx->config.tier);

    if(worker)
        worker_is_idle();
}

void datafile_delete(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile, bool update_retention, bool worker) {
    if(worker)
        worker_is_busy(UV_EVENT_DBENGINE_DATAFILE_DELETE_WAIT);

    bool datafile_got_for_deletion = datafile_acquire_for_deletion(datafile);

    if (update_retention)
        update_metrics_first_time_s(ctx, datafile, datafile->next, worker);

    while (!datafile_got_for_deletion) {
        if(worker)
            worker_is_busy(UV_EVENT_DBENGINE_DATAFILE_DELETE_WAIT);

        datafile_got_for_deletion = datafile_acquire_for_deletion(datafile);

        if (!datafile_got_for_deletion) {
            info("DBENGINE: waiting for data file '%s/"
                         DATAFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL DATAFILE_EXTENSION
                         "' to be available for deletion, "
                         "it is in use currently by %u users.",
                 ctx->config.dbfiles_path, ctx->datafiles.first->tier, ctx->datafiles.first->fileno, datafile->users.lockers);

            __atomic_add_fetch(&rrdeng_cache_efficiency_stats.datafile_deletion_spin, 1, __ATOMIC_RELAXED);
            sleep_usec(1 * USEC_PER_SEC);
        }
    }

    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.datafile_deletion_started, 1, __ATOMIC_RELAXED);
    info("DBENGINE: deleting data file '%s/"
         DATAFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL DATAFILE_EXTENSION
         "'.",
         ctx->config.dbfiles_path, ctx->datafiles.first->tier, ctx->datafiles.first->fileno);

    if(worker)
        worker_is_busy(UV_EVENT_DBENGINE_DATAFILE_DELETE);

    struct rrdengine_journalfile *journal_file;
    unsigned deleted_bytes, journal_file_bytes, datafile_bytes;
    int ret;
    char path[RRDENG_PATH_MAX];

    uv_rwlock_wrlock(&ctx->datafiles.rwlock);
    datafile_list_delete_unsafe(ctx, datafile);
    uv_rwlock_wrunlock(&ctx->datafiles.rwlock);

    journal_file = datafile->journalfile;
    datafile_bytes = datafile->pos;
    journal_file_bytes = journalfile_current_size(journal_file);
    deleted_bytes = journalfile_v2_data_size_get(journal_file);

    info("DBENGINE: deleting data and journal files to maintain disk quota");
    ret = journalfile_destroy_unsafe(journal_file, datafile);
    if (!ret) {
        journalfile_v1_generate_path(datafile, path, sizeof(path));
        info("DBENGINE: deleted journal file \"%s\".", path);
        journalfile_v2_generate_path(datafile, path, sizeof(path));
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

    ctx_current_disk_space_decrease(ctx, deleted_bytes);
    info("DBENGINE: reclaimed %u bytes of disk space.", deleted_bytes);
}

static void *database_rotate_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    datafile_delete(ctx, ctx->datafiles.first, ctx_is_available_for_queries(ctx), true);

    if (rrdeng_ctx_exceeded_disk_quota(ctx))
        rrdeng_enq_cmd(ctx, RRDENG_OPCODE_DATABASE_ROTATE, NULL, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);

    rrdcontext_db_rotation();

    return data;
}

static void after_flush_all_hot_and_dirty_pages_of_section(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    ;
}

static void *flush_all_hot_and_dirty_pages_of_section_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    worker_is_busy(UV_EVENT_DBENGINE_QUIESCE);
    pgc_flush_all_hot_and_dirty_pages(main_cache, (Word_t)ctx);
    completion_mark_complete(&ctx->quiesce.completion);
    return data;
}

static void after_populate_mrg(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    ;
}

static void *populate_mrg_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    worker_is_busy(UV_EVENT_DBENGINE_POPULATE_MRG);

    do {
        struct rrdengine_datafile *datafile = NULL;

        // find a datafile to work
        uv_rwlock_rdlock(&ctx->datafiles.rwlock);
        for(datafile = ctx->datafiles.first; datafile ; datafile = datafile->next) {
            if(!netdata_spinlock_trylock(&datafile->populate_mrg.spinlock))
                continue;

            if(datafile->populate_mrg.populated) {
                netdata_spinlock_unlock(&datafile->populate_mrg.spinlock);
                continue;
            }

            // we have the spinlock and it is not populated
            break;
        }
        uv_rwlock_rdunlock(&ctx->datafiles.rwlock);

        if(!datafile)
            break;

        journalfile_v2_populate_retention_to_mrg(ctx, datafile->journalfile);
        datafile->populate_mrg.populated = true;
        netdata_spinlock_unlock(&datafile->populate_mrg.spinlock);

    } while(1);

    completion_mark_complete(completion);

    return data;
}

static void after_ctx_shutdown(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    ;
}

static void *ctx_shutdown_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    worker_is_busy(UV_EVENT_DBENGINE_SHUTDOWN);

    completion_wait_for(&ctx->quiesce.completion);
    completion_destroy(&ctx->quiesce.completion);

    bool logged = false;
    while(__atomic_load_n(&ctx->atomic.extents_currently_being_flushed, __ATOMIC_RELAXED) ||
            __atomic_load_n(&ctx->atomic.inflight_queries, __ATOMIC_RELAXED)) {
        if(!logged) {
            logged = true;
            info("DBENGINE: waiting for %zu inflight queries to finish to shutdown tier %d...",
                 __atomic_load_n(&ctx->atomic.inflight_queries, __ATOMIC_RELAXED),
                 (ctx->config.legacy) ? -1 : ctx->config.tier);
        }
        sleep_usec(1 * USEC_PER_MS);
    }

    completion_mark_complete(completion);

    return data;
}

static void *cache_flush_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    if (!main_cache)
        return data;

    worker_is_busy(UV_EVENT_DBENGINE_FLUSH_MAIN_CACHE);
    pgc_flush_pages(main_cache, 0);

    return data;
}

static void *cache_evict_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *req __maybe_unused) {
    if (!main_cache)
        return data;

    worker_is_busy(UV_EVENT_DBENGINE_EVICT_MAIN_CACHE);
    pgc_evict_pages(main_cache, 0, 0);

    return data;
}

static void *query_prep_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *req __maybe_unused) {
    PDC *pdc = data;
    rrdeng_prep_query(pdc, true);
    return data;
}

unsigned rrdeng_target_data_file_size(struct rrdengine_instance *ctx) {
    unsigned target_size = ctx->config.max_disk_space / TARGET_DATAFILES;
    target_size = MIN(target_size, MAX_DATAFILE_SIZE);
    target_size = MAX(target_size, MIN_DATAFILE_SIZE);
    return target_size;
}

bool rrdeng_ctx_exceeded_disk_quota(struct rrdengine_instance *ctx)
{
    uint64_t estimated_disk_space = ctx_current_disk_space_get(ctx) + rrdeng_target_data_file_size(ctx) -
                                    (ctx->datafiles.first->prev ? ctx->datafiles.first->prev->pos : 0);

    return estimated_disk_space > ctx->config.max_disk_space;
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


static void *extent_read_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    EPDL *epdl = data;
    epdl_find_extent_and_populate_pages(ctx, epdl, true);
    return data;
}

static void epdl_populate_pages_asynchronously(struct rrdengine_instance *ctx, EPDL *epdl, STORAGE_PRIORITY priority) {
    rrdeng_enq_cmd(ctx, RRDENG_OPCODE_EXTENT_READ, epdl, NULL, priority,
                   rrdeng_enqueue_epdl_cmd, rrdeng_dequeue_epdl_cmd);
}

void pdc_route_asynchronously(struct rrdengine_instance *ctx, struct page_details_control *pdc) {
    pdc_to_epdl_router(ctx, pdc, epdl_populate_pages_asynchronously, epdl_populate_pages_asynchronously);
}

void epdl_populate_pages_synchronously(struct rrdengine_instance *ctx, EPDL *epdl, enum storage_priority priority __maybe_unused) {
    epdl_find_extent_and_populate_pages(ctx, epdl, false);
}

void pdc_route_synchronously(struct rrdengine_instance *ctx, struct page_details_control *pdc) {
    pdc_to_epdl_router(ctx, pdc, epdl_populate_pages_synchronously, epdl_populate_pages_synchronously);
}

#define MAX_RETRIES_TO_START_INDEX (100)
static void *journal_v2_indexing_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    unsigned count = 0;
    worker_is_busy(UV_EVENT_DBENGINE_JOURNAL_INDEX_WAIT);

    while (__atomic_load_n(&ctx->atomic.now_deleting_files, __ATOMIC_RELAXED) && count++ < MAX_RETRIES_TO_START_INDEX)
        sleep_usec(100 * USEC_PER_MS);

    if (count == MAX_RETRIES_TO_START_INDEX) {
        worker_is_idle();
        return data;
    }

    struct rrdengine_datafile *datafile = ctx->datafiles.first;
    worker_is_busy(UV_EVENT_DBENGINE_JOURNAL_INDEX);
    count = 0;
    while (datafile && datafile->fileno != ctx_last_fileno_get(ctx) && datafile->fileno != ctx_last_flush_fileno_get(ctx)) {
        if(journalfile_v2_data_available(datafile->journalfile)) {
            // journal file v2 is already there for this datafile
            datafile = datafile->next;
            continue;
        }

        netdata_spinlock_lock(&datafile->writers.spinlock);
        bool available = (datafile->writers.running || datafile->writers.flushed_to_open_running) ? false : true;
        netdata_spinlock_unlock(&datafile->writers.spinlock);

        if(!available) {
            info("DBENGINE: journal file %u needs to be indexed, but it has writers working on it - skipping it for now", datafile->fileno);
            datafile = datafile->next;
            continue;
        }

        info("DBENGINE: journal file %u is ready to be indexed", datafile->fileno);
        pgc_open_cache_to_journal_v2(open_cache, (Word_t) ctx, (int) datafile->fileno, ctx->config.page_type,
                                     journalfile_migrate_to_v2_callback, (void *) datafile->journalfile);

        count++;

        datafile = datafile->next;

        if (unlikely(!ctx_is_available_for_queries(ctx)))
            break;
    }

    errno = 0;
    internal_error(count, "DBENGINE: journal indexing done; %u files processed", count);

    worker_is_idle();

    return data;
}

static void after_do_cache_flush(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    rrdeng_main.flushes_running--;
}

static void after_do_cache_evict(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    rrdeng_main.evictions_running--;
}

static void after_journal_v2_indexing(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    __atomic_store_n(&ctx->atomic.migration_to_v2_running, false, __ATOMIC_RELAXED);
    rrdeng_enq_cmd(ctx, RRDENG_OPCODE_DATABASE_ROTATE, NULL, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);
}

struct rrdeng_buffer_sizes rrdeng_get_buffer_sizes(void) {
    return (struct rrdeng_buffer_sizes) {
            .pgc         = pgc_aral_overhead() + pgc_aral_structures(),
            .mrg         = mrg_aral_overhead() + mrg_aral_structures(),
            .opcodes     = aral_overhead(rrdeng_main.cmd_queue.ar) + aral_structures(rrdeng_main.cmd_queue.ar),
            .handles     = aral_overhead(rrdeng_main.handles.ar) + aral_structures(rrdeng_main.handles.ar),
            .descriptors = aral_overhead(rrdeng_main.descriptors.ar) + aral_structures(rrdeng_main.descriptors.ar),
            .wal         = __atomic_load_n(&wal_globals.atomics.allocated, __ATOMIC_RELAXED) * (sizeof(WAL) + RRDENG_BLOCK_SIZE),
            .workers     = aral_overhead(rrdeng_main.work_cmd.ar),
            .pdc         = pdc_cache_size(),
            .xt_io       = aral_overhead(rrdeng_main.xt_io_descr.ar) + aral_structures(rrdeng_main.xt_io_descr.ar),
            .xt_buf      = extent_buffer_cache_size(),
            .epdl        = epdl_cache_size(),
            .deol        = deol_cache_size(),
            .pd          = pd_cache_size(),

#ifdef PDC_USE_JULYL
            .julyl       = julyl_cache_size(),
#endif
    };
}

static void after_cleanup(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    rrdeng_main.cleanup_running--;
}

static void *cleanup_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    worker_is_busy(UV_EVENT_DBENGINE_BUFFERS_CLEANUP);

    wal_cleanup1();
    extent_buffer_cleanup1();

    {
        static time_t last_run_s = 0;
        time_t now_s = now_monotonic_sec();
        if(now_s - last_run_s >= 10) {
            last_run_s = now_s;
            journalfile_v2_data_unmount_cleanup(now_s);
        }
    }

#ifdef PDC_USE_JULYL
    julyl_cleanup1();
#endif

    return data;
}

void timer_cb(uv_timer_t* handle) {
    worker_is_busy(RRDENG_TIMER_CB);
    uv_stop(handle->loop);
    uv_update_time(handle->loop);

    worker_set_metric(RRDENG_OPCODES_WAITING, (NETDATA_DOUBLE)rrdeng_main.cmd_queue.unsafe.waiting);
    worker_set_metric(RRDENG_WORKS_DISPATCHED, (NETDATA_DOUBLE)__atomic_load_n(&rrdeng_main.work_cmd.atomics.dispatched, __ATOMIC_RELAXED));
    worker_set_metric(RRDENG_WORKS_EXECUTING, (NETDATA_DOUBLE)__atomic_load_n(&rrdeng_main.work_cmd.atomics.executing, __ATOMIC_RELAXED));

    rrdeng_enq_cmd(NULL, RRDENG_OPCODE_FLUSH_INIT, NULL, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);
    rrdeng_enq_cmd(NULL, RRDENG_OPCODE_EVICT_INIT, NULL, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);
    rrdeng_enq_cmd(NULL, RRDENG_OPCODE_CLEANUP, NULL, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);

    worker_is_idle();
}

static void dbengine_initialize_structures(void) {
    pgc_and_mrg_initialize();

    pdc_init();
    page_details_init();
    epdl_init();
    deol_init();
    rrdeng_cmd_queue_init();
    work_request_init();
    rrdeng_query_handle_init();
    page_descriptors_init();
    extent_buffer_init();
    dbengine_page_alloc_init();
    extent_io_descriptor_init();
}

bool rrdeng_dbengine_spawn(struct rrdengine_instance *ctx __maybe_unused) {
    static bool spawned = false;
    static SPINLOCK spinlock = NETDATA_SPINLOCK_INITIALIZER;

    netdata_spinlock_lock(&spinlock);

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

        dbengine_initialize_structures();

        fatal_assert(0 == uv_thread_create(&rrdeng_main.thread, dbengine_event_loop, &rrdeng_main));
        spawned = true;
    }

    netdata_spinlock_unlock(&spinlock);
    return true;
}

static inline void worker_dispatch_extent_read(struct rrdeng_cmd cmd, bool from_worker) {
    struct rrdengine_instance *ctx = cmd.ctx;
    EPDL *epdl = cmd.data;

    if(from_worker)
        epdl_find_extent_and_populate_pages(ctx, epdl, true);
    else
        work_dispatch(ctx, epdl, NULL, cmd.opcode, extent_read_tp_worker, NULL);
}

static inline void worker_dispatch_query_prep(struct rrdeng_cmd cmd, bool from_worker) {
    struct rrdengine_instance *ctx = cmd.ctx;
    PDC *pdc = cmd.data;

    if(from_worker)
        rrdeng_prep_query(pdc, true);
    else
        work_dispatch(ctx, pdc, NULL, cmd.opcode, query_prep_tp_worker, NULL);
}

void dbengine_event_loop(void* arg) {
    sanity_check();
    uv_thread_set_name_np(pthread_self(), "DBENGINE");
    service_register(SERVICE_THREAD_TYPE_EVENT_LOOP, NULL, NULL, NULL, true);

    worker_register("DBENGINE");

    // opcode jobs
    worker_register_job_name(RRDENG_OPCODE_NOOP,                                     "noop");

    worker_register_job_name(RRDENG_OPCODE_QUERY,                                    "query");
    worker_register_job_name(RRDENG_OPCODE_EXTENT_WRITE,                             "extent write");
    worker_register_job_name(RRDENG_OPCODE_EXTENT_READ,                              "extent read");
    worker_register_job_name(RRDENG_OPCODE_FLUSHED_TO_OPEN,                          "flushed to open");
    worker_register_job_name(RRDENG_OPCODE_DATABASE_ROTATE,                          "db rotate");
    worker_register_job_name(RRDENG_OPCODE_JOURNAL_INDEX,                            "journal index");
    worker_register_job_name(RRDENG_OPCODE_FLUSH_INIT,                               "flush init");
    worker_register_job_name(RRDENG_OPCODE_EVICT_INIT,                               "evict init");
    worker_register_job_name(RRDENG_OPCODE_CTX_SHUTDOWN,                             "ctx shutdown");
    worker_register_job_name(RRDENG_OPCODE_CTX_QUIESCE,                              "ctx quiesce");

    worker_register_job_name(RRDENG_OPCODE_MAX,                                      "get opcode");

    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_QUERY,                "query cb");
    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_EXTENT_WRITE,         "extent write cb");
    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_EXTENT_READ,          "extent read cb");
    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_FLUSHED_TO_OPEN,      "flushed to open cb");
    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_DATABASE_ROTATE,      "db rotate cb");
    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_JOURNAL_INDEX,        "journal index cb");
    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_FLUSH_INIT,           "flush init cb");
    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_EVICT_INIT,           "evict init cb");
    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_CTX_SHUTDOWN,         "ctx shutdown cb");
    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_CTX_QUIESCE,          "ctx quiesce cb");

    // special jobs
    worker_register_job_name(RRDENG_TIMER_CB,                                        "timer");
    worker_register_job_name(RRDENG_FLUSH_TRANSACTION_BUFFER_CB,                     "transaction buffer flush cb");

    worker_register_job_custom_metric(RRDENG_OPCODES_WAITING,  "opcodes waiting",  "opcodes", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(RRDENG_WORKS_DISPATCHED, "works dispatched", "works",   WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(RRDENG_WORKS_EXECUTING,  "works executing",  "works",   WORKER_METRIC_ABSOLUTE);

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
            worker_is_busy(RRDENG_OPCODE_MAX);
            cmd = rrdeng_deq_cmd(RRDENG_OPCODE_NOOP);
            opcode = cmd.opcode;

            worker_is_busy(opcode);

            switch (opcode) {
                case RRDENG_OPCODE_EXTENT_READ:
                    worker_dispatch_extent_read(cmd, false);
                    break;

                case RRDENG_OPCODE_QUERY:
                    worker_dispatch_query_prep(cmd, false);
                    break;

                case RRDENG_OPCODE_EXTENT_WRITE: {
                    struct rrdengine_instance *ctx = cmd.ctx;
                    struct page_descr_with_data *base = cmd.data;
                    struct completion *completion = cmd.completion; // optional
                    work_dispatch(ctx, base, completion, opcode, extent_write_tp_worker, after_extent_write);
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
                    if(rrdeng_main.flushes_running < (size_t)(libuv_worker_threads / 4)) {
                        rrdeng_main.flushes_running++;
                        work_dispatch(NULL, NULL, NULL, opcode, cache_flush_tp_worker, after_do_cache_flush);
                    }
                    break;
                }

                case RRDENG_OPCODE_EVICT_INIT: {
                    if(!rrdeng_main.evictions_running) {
                        rrdeng_main.evictions_running++;
                        work_dispatch(NULL, NULL, NULL, opcode, cache_evict_tp_worker, after_do_cache_evict);
                    }
                    break;
                }

                case RRDENG_OPCODE_CLEANUP: {
                    if(!rrdeng_main.cleanup_running) {
                        rrdeng_main.cleanup_running++;
                        work_dispatch(NULL, NULL, NULL, opcode, cleanup_tp_worker, after_cleanup);
                    }
                    break;
                }

                case RRDENG_OPCODE_JOURNAL_INDEX: {
                    struct rrdengine_instance *ctx = cmd.ctx;
                    struct rrdengine_datafile *datafile = cmd.data;
                    if(!__atomic_load_n(&ctx->atomic.migration_to_v2_running, __ATOMIC_RELAXED)) {

                        __atomic_store_n(&ctx->atomic.migration_to_v2_running, true, __ATOMIC_RELAXED);
                        work_dispatch(ctx, datafile, NULL, opcode, journal_v2_indexing_tp_worker, after_journal_v2_indexing);
                    }
                    break;
                }

                case RRDENG_OPCODE_DATABASE_ROTATE: {
                    struct rrdengine_instance *ctx = cmd.ctx;
                    if (!__atomic_load_n(&ctx->atomic.now_deleting_files, __ATOMIC_RELAXED) &&
                         ctx->datafiles.first->next != NULL &&
                         ctx->datafiles.first->next->next != NULL &&
                         rrdeng_ctx_exceeded_disk_quota(ctx)) {

                        __atomic_store_n(&ctx->atomic.now_deleting_files, true, __ATOMIC_RELAXED);
                        work_dispatch(ctx, NULL, NULL, opcode, database_rotate_tp_worker, after_database_rotate);
                    }
                    break;
                }

                case RRDENG_OPCODE_CTX_POPULATE_MRG: {
                    struct rrdengine_instance *ctx = cmd.ctx;
                    struct completion *completion = cmd.completion;
                    work_dispatch(ctx, NULL, completion, opcode, populate_mrg_tp_worker, after_populate_mrg);
                    break;
                }

                case RRDENG_OPCODE_CTX_QUIESCE: {
                    // a ctx will shutdown shortly
                    struct rrdengine_instance *ctx = cmd.ctx;
                    __atomic_store_n(&ctx->quiesce.enabled, true, __ATOMIC_RELEASE);
                    work_dispatch(ctx, NULL, NULL, opcode,
                                      flush_all_hot_and_dirty_pages_of_section_tp_worker,
                                      after_flush_all_hot_and_dirty_pages_of_section);
                    break;
                }

                case RRDENG_OPCODE_CTX_SHUTDOWN: {
                    // a ctx is shutting down
                    struct rrdengine_instance *ctx = cmd.ctx;
                    struct completion *completion = cmd.completion;
                    work_dispatch(ctx, NULL, completion, opcode, ctx_shutdown_tp_worker, after_ctx_shutdown);
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

// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdengine.h"
#include "pdc.h"
#include "dbengine-compression.h"

struct rrdeng_global_stats global_stats = { 0 };

unsigned rrdeng_pages_per_extent = DEFAULT_PAGES_PER_EXTENT;

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
    ND_THREAD *thread;
    uv_loop_t loop;
    uv_async_t async;
    uv_timer_t timer;
    uv_timer_t retention_timer;
    pid_t tid;
    bool shutdown;

    size_t flushes_running;
    size_t evict_main_running;
    size_t evict_open_running;
    size_t evict_extent_running;
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
        .retention_timer = {},
        .flushes_running = 0,
        .evict_main_running = 0,
        .cleanup_running = 0,

        .cmd_queue = {
                .unsafe = {
                        .spinlock = SPINLOCK_INITIALIZER,
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

    BUILD_BUG_ON(sizeof(nd_uuid_t) != UUID_SZ); /* check UUID size */

    /* page count must fit in 8 bits */
    BUILD_BUG_ON(MAX_PAGES_PER_EXTENT > 255);
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
        0,
        NULL,
        NULL, NULL, false, false, true
    );

    pulse_aral_register(rrdeng_main.work_cmd.ar, "workers");
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

// This needs to be called from event loop thread only (callback)
static inline void check_and_schedule_db_rotation(struct rrdengine_instance *ctx)
{
    internal_fatal(rrdeng_main.tid != gettid_cached(), "check_and_schedule_db_rotation() can only be run from the event loop thread");

    if (__atomic_load_n(&ctx->atomic.needs_indexing, __ATOMIC_RELAXED)) {
        if (ctx->datafiles.pending_index == false) {
            ctx->datafiles.pending_index = true;
            rrdeng_enq_cmd(ctx, RRDENG_OPCODE_JOURNAL_INDEX, NULL, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);
        }
    }

    if (ctx->datafiles.pending_rotate) {
        nd_log_daemon(NDLP_DEBUG, "DBENGINE: tier %d is already pending rotation", ctx->config.tier);
        return;
    }

    if(rrdeng_ctx_tier_cap_exceeded(ctx)) {
        ctx->datafiles.pending_rotate = true;
        rrdeng_enq_cmd(ctx, RRDENG_OPCODE_DATABASE_ROTATE, NULL, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);
    }
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

static bool work_dispatch(struct rrdengine_instance *ctx, void *data, struct completion *completion, enum rrdeng_opcode opcode, work_cb do_work_cb, after_work_cb do_after_work_cb) {
    struct rrdeng_work *work_request = NULL;

    internal_fatal(rrdeng_main.tid != gettid_cached(), "work_dispatch() can only be run from the event loop thread");

    work_request = aral_mallocz(rrdeng_main.work_cmd.ar);
    memset(work_request, 0, sizeof(struct rrdeng_work));
    work_request->req.data = work_request;
    work_request->ctx = ctx;
    work_request->data = data;
    work_request->completion = completion;
    work_request->work_cb = do_work_cb;
    work_request->after_work_cb = do_after_work_cb;
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
            0,
            NULL,
            NULL, NULL, false, false, true);

    pulse_aral_register(rrdeng_main.descriptors.ar, "descriptors");
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
            0,
            NULL,
            NULL, NULL, false, false, true
            );

    pulse_aral_register(rrdeng_main.xt_io_descr.ar, "extent io");
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
            0,
            NULL,
            NULL, NULL, false, false, true);

    pulse_aral_register(rrdeng_main.handles.ar, "query handles");
}

ALWAYS_INLINE struct rrdeng_query_handle *rrdeng_query_handle_get(void) {
    struct rrdeng_query_handle *handle = aral_mallocz(rrdeng_main.handles.ar);
    memset(handle, 0, sizeof(struct rrdeng_query_handle));
    return handle;
}

ALWAYS_INLINE void rrdeng_query_handle_release(struct rrdeng_query_handle *handle) {
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
                .spinlock = SPINLOCK_INITIALIZER,
                .available_items = NULL,
                .available = 0,
        },
        .atomics = {
                .allocated = 0,
        },
};

static void wal_cleanup1(void) {
    WAL *wal = NULL;

    if(!spinlock_trylock(&wal_globals.protected.spinlock))
        return;

    if(wal_globals.protected.available_items && wal_globals.protected.available > nd_profile.storage_tiers) {
        wal = wal_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(wal_globals.protected.available_items, wal, cache.prev, cache.next);
        wal_globals.protected.available--;
    }

    spinlock_unlock(&wal_globals.protected.spinlock);

    if(wal) {
        posix_memalign_freez(wal->buf);
        freez(wal);
        __atomic_sub_fetch(&wal_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
    }
}

WAL *wal_get(struct rrdengine_instance *ctx, unsigned size) {
    if(!size || size > RRDENG_BLOCK_SIZE)
        fatal("DBENGINE: invalid WAL size requested");

    WAL *wal = NULL;

    spinlock_lock(&wal_globals.protected.spinlock);

    if(likely(wal_globals.protected.available_items)) {
        wal = wal_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(wal_globals.protected.available_items, wal, cache.prev, cache.next);
        wal_globals.protected.available--;
    }

    uint64_t transaction_id = __atomic_fetch_add(&ctx->atomic.transaction_id, 1, __ATOMIC_RELAXED);
    spinlock_unlock(&wal_globals.protected.spinlock);

    if(unlikely(!wal)) {
        wal = mallocz(sizeof(WAL));
        wal->buf_size = RRDENG_BLOCK_SIZE;
        (void)posix_memalignz((void *)&wal->buf, RRDFILE_ALIGNMENT, wal->buf_size);
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

    spinlock_lock(&wal_globals.protected.spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(wal_globals.protected.available_items, wal, cache.prev, cache.next);
    wal_globals.protected.available++;
    spinlock_unlock(&wal_globals.protected.spinlock);
}

// ----------------------------------------------------------------------------
// command queue cache

static void rrdeng_cmd_queue_init(void) {
    rrdeng_main.cmd_queue.ar = aral_create("dbengine-opcodes",
                                           sizeof(struct rrdeng_cmd),
                                           0,
                                           0,
                                           NULL,
                                           NULL, NULL, false, false, true);

    pulse_aral_register(rrdeng_main.cmd_queue.ar, "opcodes");
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

ALWAYS_INLINE void rrdeng_enqueue_epdl_cmd(struct rrdeng_cmd *cmd) {
    epdl_cmd_queued(cmd->data, cmd);
}

ALWAYS_INLINE void rrdeng_dequeue_epdl_cmd(struct rrdeng_cmd *cmd) {
    epdl_cmd_dequeued(cmd->data);
}

ALWAYS_INLINE void rrdeng_req_cmd(requeue_callback_t get_cmd_cb, void *data, STORAGE_PRIORITY priority) {
    spinlock_lock(&rrdeng_main.cmd_queue.unsafe.spinlock);

    struct rrdeng_cmd *cmd = get_cmd_cb(data);
    if(cmd) {
        priority = rrdeng_enq_cmd_map_opcode_to_priority(cmd->opcode, priority);

        if (cmd->priority > priority) {
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(rrdeng_main.cmd_queue.unsafe.waiting_items_by_priority[cmd->priority], cmd, queue.prev, queue.next);
            DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(rrdeng_main.cmd_queue.unsafe.waiting_items_by_priority[priority], cmd, queue.prev, queue.next);
            cmd->priority = priority;
        }
    }

    spinlock_unlock(&rrdeng_main.cmd_queue.unsafe.spinlock);
}

ALWAYS_INLINE void rrdeng_enq_cmd(struct rrdengine_instance *ctx, enum rrdeng_opcode opcode, void *data, struct completion *completion,
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

    spinlock_lock(&rrdeng_main.cmd_queue.unsafe.spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(rrdeng_main.cmd_queue.unsafe.waiting_items_by_priority[priority], cmd, queue.prev, queue.next);
    rrdeng_main.cmd_queue.unsafe.waiting++;
    if(enqueue_cb)
        enqueue_cb(cmd);
    spinlock_unlock(&rrdeng_main.cmd_queue.unsafe.spinlock);

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

    if(unlikely(from_worker)) {
        if(status == LIBUV_WORKERS_CRITICAL)
            return opcode_empty;

        min_priority = STORAGE_PRIORITY_INTERNAL_QUERY_PREP;
        max_priority = STORAGE_PRIORITY_BEST_EFFORT;
    }
    else {
        min_priority = STORAGE_PRIORITY_INTERNAL_DBENGINE;
        max_priority = (status != LIBUV_WORKERS_RELAXED) ? STORAGE_PRIORITY_INTERNAL_DBENGINE : STORAGE_PRIORITY_INTERNAL_MAX_DONT_USE - 1;
    }

    // find an opcode to execute from the queue
    spinlock_lock(&rrdeng_main.cmd_queue.unsafe.spinlock);
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

    spinlock_unlock(&rrdeng_main.cmd_queue.unsafe.spinlock);

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
    fatal_assert(count <= MAX_PAGES_PER_EXTENT);
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

static void
extent_flush_to_open(struct rrdengine_instance *ctx, struct extent_io_descriptor *xt_io_descr, bool have_error)
{
    worker_is_busy(UV_EVENT_DBENGINE_FLUSHED_TO_OPEN);

    struct page_descr_with_data *descr;
    struct rrdengine_datafile *datafile;
    unsigned i;

    datafile = xt_io_descr->datafile;

    bool still_running = ctx_is_available_for_queries(ctx);

    for (i = 0 ; i < xt_io_descr->descr_count ; ++i) {
        descr = xt_io_descr->descr_array[i];

        if (likely(still_running && !have_error))
            pgc_open_add_hot_page(
                    (Word_t)ctx, descr->metric_id,
                    (time_t) (descr->start_time_ut / USEC_PER_SEC),
                    (time_t) (descr->end_time_ut / USEC_PER_SEC),
                    descr->update_every_s,
                    datafile,
                    xt_io_descr->pos, xt_io_descr->bytes, descr->page_length);

        page_descriptor_release(descr);
    }

    posix_memalign_freez(xt_io_descr->buf);
    extent_io_descriptor_release(xt_io_descr);

    spinlock_lock(&datafile->writers.spinlock);
    datafile->writers.flushed_to_open_running--;
    spinlock_unlock(&datafile->writers.spinlock);

    if(datafile->fileno != ctx_last_fileno_get(ctx) && still_running)
        __atomic_store_n(&ctx->atomic.needs_indexing, true, __ATOMIC_RELAXED);

    worker_is_idle();
}

// Main event loop callback

static bool datafile_is_full(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile) {
    bool ret = false;
    spinlock_lock(&datafile->writers.spinlock);

    if(datafile->pos > rrdeng_target_data_file_size(ctx))
        ret = true;

    spinlock_unlock(&datafile->writers.spinlock);

    return ret;
}

size_t datafile_count(struct rrdengine_instance *ctx, bool with_lock)
{
    size_t count = 0;

    if (!with_lock)
        uv_rwlock_rdlock(&ctx->datafiles.rwlock);

    count = JudyLCount(ctx->datafiles.JudyL, 0, -1, PJE0);

    if (!with_lock)
        uv_rwlock_rdunlock(&ctx->datafiles.rwlock);

    return count;
}

struct rrdengine_datafile *
get_next_datafile(struct rrdengine_datafile *this_datafile, struct rrdengine_instance *ctx, bool with_lock)
{
    struct rrdengine_datafile *datafile = NULL;

    ctx = this_datafile ? this_datafile->ctx : ctx;
    if (!ctx)
        return NULL;

    if (!with_lock)
        uv_rwlock_rdlock(&ctx->datafiles.rwlock);

    Word_t Index = this_datafile ? this_datafile->fileno : 0;
    Pvoid_t *Pvalue;

    Pvalue = JudyLNext(ctx->datafiles.JudyL, &Index, PJE0);

    if (Pvalue)
        datafile = *Pvalue;

    if (!with_lock)
        uv_rwlock_rdunlock(&ctx->datafiles.rwlock);

    return datafile;
}

static struct rrdengine_datafile *get_ctx_datafile_first_or_last(struct rrdengine_instance *ctx, bool first, bool with_lock)
{
    struct rrdengine_datafile *datafile = NULL;

    if (!with_lock)
        uv_rwlock_rdlock(&ctx->datafiles.rwlock);

    Word_t Index = 0;
    Pvoid_t *Pvalue;

    if (first)
        Pvalue = JudyLFirst(ctx->datafiles.JudyL, &Index, PJE0);
    else {
        Index = -1;
        Pvalue = JudyLLast(ctx->datafiles.JudyL, &Index, PJE0);
    }

    if (Pvalue)
        datafile = *Pvalue;

    if (!with_lock)
        uv_rwlock_rdunlock(&ctx->datafiles.rwlock);

    return datafile;
}

struct rrdengine_datafile *get_first_ctx_datafile(struct rrdengine_instance *ctx, bool with_lock) {
    return get_ctx_datafile_first_or_last(ctx, true, with_lock);
}

struct rrdengine_datafile *get_last_ctx_datafile(struct rrdengine_instance *ctx, bool with_lock) {
    return get_ctx_datafile_first_or_last(ctx, false, with_lock);
}


static struct rrdengine_datafile *get_datafile_to_write_extent(struct rrdengine_instance *ctx) {
    struct rrdengine_datafile *datafile;

    // get the latest datafile
    uv_rwlock_rdlock(&ctx->datafiles.rwlock);

    datafile = get_last_ctx_datafile(ctx, true);
    // become a writer on this datafile, to prevent it from vanishing
    spinlock_lock(&datafile->writers.spinlock);
    datafile->writers.running++;
    spinlock_unlock(&datafile->writers.spinlock);
    uv_rwlock_rdunlock(&ctx->datafiles.rwlock);

    if(datafile_is_full(ctx, datafile)) {
        // remember the datafile we have become writers to
        struct rrdengine_datafile *old_datafile = datafile;

        // only 1 datafile creation at a time
        static netdata_mutex_t mutex = NETDATA_MUTEX_INITIALIZER;
        netdata_mutex_lock(&mutex);

        // take the latest datafile again - without this, multiple threads may create multiple files
        datafile = get_last_ctx_datafile(ctx, false);

        if(datafile_is_full(ctx, datafile) && create_new_datafile_pair(ctx) == 0)
            __atomic_store_n(&ctx->atomic.needs_indexing, true, __ATOMIC_RELAXED);

        netdata_mutex_unlock(&mutex);

        // get the new latest datafile again, like above
        uv_rwlock_rdlock(&ctx->datafiles.rwlock);
        datafile = get_last_ctx_datafile(ctx, true);
        // become a writer on this datafile, to prevent it from vanishing
        spinlock_lock(&datafile->writers.spinlock);
        datafile->writers.running++;
        spinlock_unlock(&datafile->writers.spinlock);
        uv_rwlock_rdunlock(&ctx->datafiles.rwlock);

        // release the writers on the old datafile
        spinlock_lock(&old_datafile->writers.spinlock);
        old_datafile->writers.running--;
        spinlock_unlock(&old_datafile->writers.spinlock);
    }

    return datafile;
}

/*
 * Take a page list in a judy array and write them
 */
static struct extent_io_descriptor *
datafile_extent_build(struct rrdengine_instance *ctx, struct page_descr_with_data *base, uv_buf_t *iov)
{
    unsigned i;
    uint32_t real_io_size, size_bytes, count, pos;
    uint32_t uncompressed_payload_length, max_compressed_size, payload_offset;
    struct page_descr_with_data *descr, *eligible_pages[MAX_PAGES_PER_EXTENT];
    struct extent_io_descriptor *xt_io_descr;
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
        __atomic_sub_fetch(&ctx->atomic.extents_currently_being_flushed, 1, __ATOMIC_RELAXED);
        return NULL;
    }

    xt_io_descr = extent_io_descriptor_get();
    payload_offset = sizeof(*header) + count * sizeof(header->descr[0]);
    max_compressed_size = dbengine_max_compressed_size(uncompressed_payload_length, compression_algorithm);
    size_bytes = payload_offset + MAX(uncompressed_payload_length, max_compressed_size) + sizeof(*trailer);
    (void)posix_memalignz((void *)&xt_io_descr->buf, RRDFILE_ALIGNMENT, ALIGN_BYTES_CEILING(size_bytes));
    memset(xt_io_descr->buf, 0, ALIGN_BYTES_CEILING(size_bytes));
    (void) memcpy(xt_io_descr->descr_array, eligible_pages, sizeof(struct page_descr_with_data *) * count);
    xt_io_descr->descr_count = count;

    pos = 0;
    header = xt_io_descr->buf;
    header->number_of_pages = count;
    pos += sizeof(*header);

    for (i = 0 ; i < count ; ++i) {
        descr = xt_io_descr->descr_array[i];
        header->descr[i].type = descr->type;
        uuid_copy(*(nd_uuid_t *)header->descr[i].uuid, *descr->id);
        header->descr[i].page_length = descr->page_length;
        header->descr[i].start_time_ut = descr->start_time_ut;

        switch (descr->type) {
            case RRDENG_PAGE_TYPE_ARRAY_32BIT:
            case RRDENG_PAGE_TYPE_ARRAY_TIER1:
                header->descr[i].end_time_ut = descr->end_time_ut;
                break;
            case RRDENG_PAGE_TYPE_GORILLA_32BIT:
                header->descr[i].gorilla.delta_time_s = (uint32_t) ((descr->end_time_ut - descr->start_time_ut) / USEC_PER_SEC);
                header->descr[i].gorilla.entries = pgd_slots_used(descr->pgd);
                break;
            default:
                fatal("Unknown page type: %uc", descr->type);
        }

        pos += sizeof(header->descr[i]);
    }

    // build the extent payload
    for (i = 0 ; i < count ; ++i) {
        descr = xt_io_descr->descr_array[i];
        pgd_copy_to_extent(descr->pgd, xt_io_descr->buf + pos, descr->page_length);
        pos += descr->page_length;
    }

    // compress the payload
    size_t compressed_size =
        (int)dbengine_compress(xt_io_descr->buf + payload_offset,
                               uncompressed_payload_length,
                               compression_algorithm);

    internal_fatal(compressed_size > max_compressed_size, "DBENGINE: compression returned more data than the max allowed");
    internal_fatal(compressed_size > uncompressed_payload_length, "DBENGINE: compression returned more data than the uncompressed extent");

    if(compressed_size) {
        header->compression_algorithm = compression_algorithm;
        header->payload_length = compressed_size;
    }
    else {
       // compression failed, or generated bigger pages
       // so it didn't touch our uncompressed buffer
       header->compression_algorithm = RRDENG_COMPRESSION_NONE;
       header->payload_length = compressed_size = uncompressed_payload_length;
    }

    // set the correct size
    size_bytes = payload_offset + compressed_size + sizeof(*trailer);

    if(compression_algorithm != RRDENG_COMPRESSION_NONE) {
        __atomic_add_fetch(&ctx->stats.before_compress_bytes, uncompressed_payload_length, __ATOMIC_RELAXED);
        __atomic_add_fetch(&ctx->stats.after_compress_bytes, compressed_size, __ATOMIC_RELAXED);
    }

    real_io_size = ALIGN_BYTES_CEILING(size_bytes);

    datafile = get_datafile_to_write_extent(ctx);
    spinlock_lock(&datafile->writers.spinlock);
    xt_io_descr->datafile = datafile;
    xt_io_descr->pos = datafile->pos;
    datafile->pos += real_io_size;
    spinlock_unlock(&datafile->writers.spinlock);

    xt_io_descr->bytes = size_bytes;

    trailer = xt_io_descr->buf + size_bytes - sizeof(*trailer);
    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, xt_io_descr->buf, size_bytes - sizeof(*trailer));
    crc32set(trailer->checksum, crc);

    *iov = uv_buf_init((void *)xt_io_descr->buf, real_io_size);
    journalfile_extent_build(ctx, xt_io_descr);

    ctx_last_flush_fileno_set(ctx, datafile->fileno);
    xt_io_descr->real_io_size = real_io_size;

    return xt_io_descr;
}

static void after_extent_write(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* uv_work_req __maybe_unused, int status __maybe_unused)
{
    check_and_schedule_db_rotation(ctx);
}

static void *extent_write_tp_worker(
    struct rrdengine_instance *ctx,
    void *data,
    struct completion *completion __maybe_unused,
    uv_work_t *req __maybe_unused)
{
    worker_is_busy(UV_EVENT_DBENGINE_EXTENT_WRITE);
    uv_buf_t iov;
    struct page_descr_with_data *base = data;
    struct extent_io_descriptor *xt_io_descr = datafile_extent_build(ctx, base, &iov);

    if (!xt_io_descr)
        goto done;

    struct rrdengine_datafile *datafile = xt_io_descr->datafile;
    uv_fs_t request;

    int retries = 10;
    int ret = -1;
    while (ret < 0 && --retries) {
        ret = uv_fs_write(NULL, &request, datafile->file, &iov, 1, (int64_t)xt_io_descr->pos, NULL);
        uv_fs_req_cleanup(&request);
        if (ret < 0) {
            if (ret == -ENOSPC || ret == -EBADF || ret == -EACCES || ret == -EROFS || ret == -EINVAL)
                break;
            sleep_usec(300 * USEC_PER_MS);
        }
    }

    if (unlikely(ret < 0))
        ctx_io_error(ctx);
    else {
        ctx_current_disk_space_increase(ctx, xt_io_descr->real_io_size);
        ctx_io_write_op_bytes(ctx, xt_io_descr->real_io_size);
        ret = journalfile_v1_extent_write(ctx, datafile, xt_io_descr->wal);
    }

    if (ret < 0) {
        nd_log_limit_static_global_var(dbengine_erl, 10, 0);
        nd_log_limit(&dbengine_erl, NDLS_DAEMON, NDLP_ERR, "DBENGINE: Tier %d, %s", ctx->config.tier, uv_strerror(ret));
    }

    spinlock_lock(&datafile->writers.spinlock);
    datafile->writers.running--;
    datafile->writers.flushed_to_open_running++;
    spinlock_unlock(&datafile->writers.spinlock);

    extent_flush_to_open(ctx, xt_io_descr, ret < 0);

done:
    __atomic_sub_fetch(&ctx->atomic.extents_currently_being_flushed, 1, __ATOMIC_RELAXED);
    completion_mark_complete(completion);
    worker_is_idle();
    return NULL;
}

static void after_database_rotate(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    __atomic_store_n(&ctx->atomic.now_deleting_files, false, __ATOMIC_RELAXED);

    check_and_schedule_db_rotation(ctx);
}

struct uuid_first_time_s {
    nd_uuid_t *uuid;
    time_t first_time_s;
    METRIC *metric;
    size_t pages_found;
    size_t df_matched;
    size_t df_index_oldest;
};

struct rrdengine_datafile *datafile_release_and_acquire_next_for_retention(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile) {

    uv_rwlock_rdlock(&ctx->datafiles.rwlock);

    struct rrdengine_datafile *next_datafile = get_next_datafile(datafile, NULL, true);

    while(next_datafile && !datafile_acquire(next_datafile, DATAFILE_ACQUIRE_RETENTION))
        next_datafile = get_next_datafile(next_datafile, NULL, true);

    uv_rwlock_rdunlock(&ctx->datafiles.rwlock);

    datafile_release(datafile, DATAFILE_ACQUIRE_RETENTION);

    return next_datafile;
}

static time_t find_uuid_first_time(
    struct rrdengine_instance *ctx,
    struct rrdengine_datafile *datafile,
    struct uuid_first_time_s *uuid_first_entry_list,
    size_t count)
{
    time_t global_first_time_s = LONG_MAX;

    // acquire the datafile to work with it
    uv_rwlock_rdlock(&ctx->datafiles.rwlock);
    while(datafile && !datafile_acquire(datafile, DATAFILE_ACQUIRE_RETENTION))
        datafile = get_next_datafile(datafile, NULL, true);

    uv_rwlock_rdunlock(&ctx->datafiles.rwlock);

    if (unlikely(!datafile))
        return global_first_time_s;

    unsigned journalfile_count = 0;
    size_t binary_match = 0;
    size_t not_matching_bsearches = 0;

    bool agent_shutdown = false;
    while (datafile) {
        struct journal_v2_header *j2_header = journalfile_v2_data_acquire(datafile->journalfile, NULL, 0, 0);
        if (!j2_header) {
            datafile = datafile_release_and_acquire_next_for_retention(ctx, datafile);
            continue;
        }

        bool any_matching = false;

        time_t journal_start_time_s = (time_t)(j2_header->start_time_ut / USEC_PER_SEC);

        if (journal_start_time_s < global_first_time_s)
            global_first_time_s = journal_start_time_s;

        struct journal_metric_list *uuid_list =
            (struct journal_metric_list *)((uint8_t *)j2_header + j2_header->metric_offset);
        struct uuid_first_time_s *uuid_original_entry;

        size_t journal_metric_count = j2_header->metric_count;
        char file_path[RRDENG_PATH_MAX];
        journalfile_v2_generate_path(datafile, file_path, sizeof(file_path));
        PROTECTED_ACCESS_SETUP(datafile->journalfile->mmap.data, datafile->journalfile->mmap.size, file_path, "read");
        if (no_signal_received) {
            size_t journal_search_start = 0; // Start of remaining search space
            any_matching = false;
            for (size_t index = 0; index < count; ++index) {
                uuid_original_entry = &uuid_first_entry_list[index];

                if (uuid_original_entry->df_matched > 3 || uuid_original_entry->pages_found > 5)
                    continue;

                any_matching = true;
                struct journal_metric_list *live_entry = &uuid_list[journal_search_start];
                // Check if we avoid bsearch
                if (journal_metric_uuid_compare(uuid_original_entry->uuid, live_entry->uuid) != 0) {
                    live_entry = bsearch(
                        uuid_original_entry->uuid,
                        uuid_list + journal_search_start,
                        journal_metric_count - journal_search_start,
                        sizeof(*uuid_list),
                        journal_metric_uuid_compare);

                    if (!live_entry) {
                        not_matching_bsearches++;
                        continue;
                    }
                }

                size_t found_index = live_entry - uuid_list;
                journal_search_start = found_index + 1; // Next search starts after this match

                if (journal_search_start >= journal_metric_count) {
                    not_matching_bsearches += (count - index - 1);
                    break;
                }

                uuid_original_entry->pages_found += live_entry->entries;
                uuid_original_entry->df_matched++;

                time_t old_first_time_s = uuid_original_entry->first_time_s;
                time_t first_time_s = live_entry->delta_start_s + journal_start_time_s;
                uuid_original_entry->first_time_s = MIN(uuid_original_entry->first_time_s, first_time_s);

                if (uuid_original_entry->first_time_s != old_first_time_s)
                    uuid_original_entry->df_index_oldest = uuid_original_entry->df_matched;

                binary_match++;

                if (unlikely(!ctx_is_available_for_queries(ctx))) {
                    agent_shutdown = true;
                    break;
                }
            }
        } else {
            nd_log_daemon(NDLP_ERR, "DBENGINE: journalfile \"%s\" is corrupted, skipping it", file_path);
        }
        journalfile_v2_data_release(datafile->journalfile);

        if (agent_shutdown) {
            datafile_release(datafile, DATAFILE_ACQUIRE_RETENTION);
            break;
        }

        journalfile_count++;
        datafile = datafile_release_and_acquire_next_for_retention(ctx, datafile);
        if (!any_matching) {
            if (datafile)
                datafile_release(datafile, DATAFILE_ACQUIRE_RETENTION);
            break;
        }
    }

    if (agent_shutdown)
        return global_first_time_s;

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

    return global_first_time_s;
}

static void update_metrics_first_time_s(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile_to_delete, struct rrdengine_datafile *first_datafile_remaining, bool worker) {
    time_t global_first_time_s = LONG_MAX;

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
        METRIC *metric = mrg_metric_get_and_acquire_by_uuid(main_mrg, &uuid_list[index].uuid, (Word_t)ctx);
        if (!metric)
            continue;

        uuid_first_entry_list[added].metric = metric;
        uuid_first_entry_list[added].first_time_s = LONG_MAX;
        uuid_first_entry_list[added].df_matched = 0;
        uuid_first_entry_list[added].df_index_oldest = 0;
        uuid_first_entry_list[added].uuid = mrg_metric_uuid(main_mrg, metric);
        added++;
    }

    netdata_log_info("DBENGINE: recalculating tier %d retention for %zu metrics starting with datafile %u",
         ctx->config.tier, count, first_datafile_remaining->fileno);

    journalfile_v2_data_release(journalfile);

    // Update the first time / last time for all metrics we plan to delete

    if(worker)
        worker_is_busy(UV_EVENT_DBENGINE_FIND_REMAINING_RETENTION);

    global_first_time_s = find_uuid_first_time(ctx, first_datafile_remaining, uuid_first_entry_list, added);

    if (!ctx_is_available_for_queries(ctx)) {
        for (size_t index = 0; index < added; ++index) {
            uuid_first_t_entry = &uuid_first_entry_list[index];
            mrg_metric_release(main_mrg, uuid_first_t_entry->metric);
        }
        goto done;
    }

    if(worker)
        worker_is_busy(UV_EVENT_DBENGINE_POPULATE_MRG);

    netdata_log_info("DBENGINE: updating tier %d metrics registry retention for %zu metrics", ctx->config.tier, added);

    size_t deleted_metrics = 0, zero_retention_referenced = 0, zero_disk_retention = 0, zero_disk_but_live = 0;
    for (size_t index = 0; index < added; ++index) {
        uuid_first_t_entry = &uuid_first_entry_list[index];

        if (!ctx_is_available_for_queries(ctx)) {
            mrg_metric_release(main_mrg, uuid_first_t_entry->metric);
            continue;
        }

        if (likely(uuid_first_t_entry->first_time_s != LONG_MAX)) {

            time_t old_first_time_s = mrg_metric_get_first_time_s(main_mrg, uuid_first_t_entry->metric);

            bool changed = mrg_metric_set_first_time_s_if_bigger(main_mrg, uuid_first_t_entry->metric, uuid_first_t_entry->first_time_s);
            if (changed) {
                uint32_t update_every_s = mrg_metric_get_update_every_s(main_mrg, uuid_first_t_entry->metric);
                if (update_every_s && old_first_time_s && uuid_first_t_entry->first_time_s > old_first_time_s) {
                    uint64_t remove_samples = (uuid_first_t_entry->first_time_s - old_first_time_s) / update_every_s;
                    __atomic_sub_fetch(&ctx->atomic.samples, remove_samples, __ATOMIC_RELAXED);
                }
            }
            mrg_metric_release(main_mrg, uuid_first_t_entry->metric);
        }
        else {
            zero_disk_retention++;

            // there is no retention for this metric
            bool has_retention = mrg_metric_has_zero_disk_retention(main_mrg, uuid_first_t_entry->metric);
            if (!has_retention) {
                time_t first_time_s = mrg_metric_get_first_time_s(main_mrg, uuid_first_t_entry->metric);
                time_t last_time_s = mrg_metric_get_latest_time_s(main_mrg, uuid_first_t_entry->metric);
                time_t update_every_s = mrg_metric_get_update_every_s(main_mrg, uuid_first_t_entry->metric);
                if (update_every_s && first_time_s && last_time_s) {
                    uint64_t remove_samples = (first_time_s - last_time_s) / update_every_s;
                    __atomic_sub_fetch(&ctx->atomic.samples, remove_samples, __ATOMIC_RELAXED);
                }

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

    if (!ctx_is_available_for_queries(ctx))
        goto done;

    internal_error(zero_disk_retention,
                   "DBENGINE: deleted %zu metrics, zero retention but referenced %zu (out of %zu total, of which %zu have main cache retention) zero on-disk retention tier %d metrics from metrics registry",
                   deleted_metrics, zero_retention_referenced, zero_disk_retention, zero_disk_but_live, ctx->config.tier);

    if(global_first_time_s != LONG_MAX)
        __atomic_store_n(&ctx->atomic.first_time_s, global_first_time_s, __ATOMIC_RELAXED);

done:
    freez(uuid_first_entry_list);

    if(worker)
        worker_is_idle();
}

void datafile_delete(
    struct rrdengine_instance *ctx,
    struct rrdengine_datafile *datafile,
    bool update_retention,
    bool disk_time,
    bool worker)
{
    if(worker)
        worker_is_busy(UV_EVENT_DBENGINE_DATAFILE_DELETE_WAIT);

    bool datafile_got_for_deletion = datafile_acquire_for_deletion(datafile, false);

    while (!datafile_got_for_deletion) {
        if(worker)
            worker_is_busy(UV_EVENT_DBENGINE_DATAFILE_DELETE_WAIT);

        datafile_got_for_deletion = datafile_acquire_for_deletion(datafile, false);

        if (!datafile_got_for_deletion) {
            netdata_log_info("DBENGINE: waiting for data file '%s/"
                         DATAFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL DATAFILE_EXTENSION
                         "' to be available for deletion, "
                         "it is in use currently by %u users.",
                 ctx->config.dbfiles_path, datafile->tier, datafile->fileno, datafile->users.lockers);

            __atomic_add_fetch(&rrdeng_cache_efficiency_stats.datafile_deletion_spin, 1, __ATOMIC_RELAXED);
            sleep_usec(1 * USEC_PER_SEC);
        }
    }

    netdata_log_info("DBENGINE: acquired data file \"%s/"
                     DATAFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL DATAFILE_EXTENSION
                     "\" for deletion.",
                     ctx->config.dbfiles_path, datafile->tier, datafile->fileno);

    if (update_retention)
        update_metrics_first_time_s(ctx, datafile, get_next_datafile(datafile, NULL, true), worker);

//    if (!ctx_is_available_for_queries(ctx)) {
//        // agent is shutting down, we cannot continue
//        if(worker)
//            worker_is_idle();
//        return;
//    }

    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.datafile_deletion_started, 1, __ATOMIC_RELAXED);
    netdata_log_info("DBENGINE: deleting data file \"%s/"
         DATAFILE_PREFIX RRDENG_FILE_NUMBER_PRINT_TMPL DATAFILE_EXTENSION
         "\".",
         ctx->config.dbfiles_path, datafile->tier, datafile->fileno);

    if(worker)
        worker_is_busy(UV_EVENT_DBENGINE_DATAFILE_DELETE);

    struct rrdengine_journalfile *journal_file;
    size_t deleted_bytes, journal_file_bytes, datafile_bytes;
    int ret;
    char path[RRDENG_PATH_MAX];

    uv_rwlock_wrlock(&ctx->datafiles.rwlock);
    datafile_list_delete_unsafe(ctx, datafile);
    uv_rwlock_wrunlock(&ctx->datafiles.rwlock);

    journal_file = datafile->journalfile;
    datafile_bytes = datafile->pos;
    journal_file_bytes = journalfile_current_size(journal_file);
    deleted_bytes = journalfile_v2_data_size_get(journal_file);

    netdata_log_info("DBENGINE: deleting data and journal files to maintain %s", disk_time ? "disk quota" : "time retention");
    // This will delete journalfile_v2 and journalfile_v1
    ret = journalfile_destroy_unsafe(journal_file, datafile);
    if (!ret) {
        journalfile_v1_generate_path(datafile, path, sizeof(path));
        netdata_log_info("DBENGINE: deleted journal file \"%s\".", path);
        journalfile_v2_generate_path(datafile, path, sizeof(path));
        netdata_log_info("DBENGINE: deleted journal file \"%s\".", path);
        deleted_bytes += journal_file_bytes;
    }
    // This will delete the datafile
    ret = destroy_data_file_unsafe(datafile);
    if (!ret) {
        generate_datafilepath(datafile, path, sizeof(path));
        netdata_log_info("DBENGINE: deleted data file \"%s\".", path);
        deleted_bytes += datafile_bytes;
    }

    cleanup_datafile_epdl_structures(datafile);

    memset(journal_file, 0, sizeof(*journal_file));
    memset(datafile, 0, sizeof(*datafile));

    freez(journal_file);
    freez(datafile);

    ctx_current_disk_space_decrease(ctx, deleted_bytes);
    char size_for_humans[128];
    size_snprintf(size_for_humans, sizeof(size_for_humans), deleted_bytes, "B", false);
    netdata_log_info("DBENGINE: reclaimed %zu bytes (%s) of disk space.", deleted_bytes, size_for_humans);
}

static void *database_rotate_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {

    struct rrdengine_datafile *datafile = get_first_ctx_datafile(ctx, false);
    datafile_delete(ctx, datafile, ctx_is_available_for_queries(ctx), true, true);

    rrdcontext_db_rotation();

    return data;
}

static void after_flush_all_hot_and_dirty_pages_of_section(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    ;
}

static void *flush_all_hot_and_dirty_pages_of_section_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    worker_is_busy(UV_EVENT_DBENGINE_QUIESCE);
    pgc_flush_all_hot_and_dirty_pages(main_cache, (Word_t)ctx);

    for(size_t i = 0; i < pgc_max_flushers() ; i++)
        rrdeng_enq_cmd(NULL, RRDENG_OPCODE_FLUSH_MAIN, NULL, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);

    return data;
}

static void after_flush_dirty_pages_of_section(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    ;
}

static void *flush_dirty_pages_of_section_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    worker_is_busy(UV_EVENT_DBENGINE_FLUSH_DIRTY);
    pgc_flush_dirty_pages(main_cache, (Word_t)ctx);

    for(size_t i = 0; i < pgc_max_flushers() ; i++)
        rrdeng_enq_cmd(NULL, RRDENG_OPCODE_FLUSH_MAIN, NULL, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);

    return data;
}

struct mrg_load_thread {
    int max_threads;
    ND_THREAD *thread;
    uv_sem_t *sem;
    int tier;
    struct rrdengine_datafile *datafile;
    bool busy;
    bool finished;
};

size_t max_running_threads = 0;
size_t running_threads = 0;

void *journalfile_v2_populate_retention_to_mrg_worker(void *arg)
{
    struct mrg_load_thread *mlt = arg;
    uv_sem_wait(mlt->sem);

    struct rrdengine_instance *ctx = mlt->datafile->ctx;

    size_t current_threads = __atomic_add_fetch(&running_threads, 1, __ATOMIC_RELAXED);
    size_t prev_max;
    do {
        prev_max = __atomic_load_n(&max_running_threads, __ATOMIC_RELAXED);
        if (current_threads <= prev_max) {
            break;
        }
    } while (!__atomic_compare_exchange_n(
        &max_running_threads, &prev_max, current_threads, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED));

    journalfile_v2_populate_retention_to_mrg(ctx, mlt->datafile->journalfile);

    __atomic_sub_fetch(&running_threads, 1, __ATOMIC_RELAXED);
    uv_sem_post(mlt->sem);

    // Signal completion - this needs to be last
    __atomic_store_n(&mlt->finished, true, __ATOMIC_RELEASE);
    return NULL;
}

static void after_populate_mrg(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    if (completion)
        completion_mark_complete(completion);
}

static void *populate_mrg_tp_worker(
    struct rrdengine_instance *ctx,
    void *data,
    struct completion *completion __maybe_unused,
    uv_work_t *uv_work_req __maybe_unused)
{
    worker_is_busy(UV_EVENT_DBENGINE_POPULATE_MRG);

    struct mrg_load_thread *mlt = data;
    size_t max_threads = mlt->max_threads;
    int tier = ctx->config.tier;

    size_t thread_index = 0;
    int rc;

    uv_rwlock_rdlock(&ctx->datafiles.rwlock);

    size_t total_datafiles = 0;
    size_t populated_datafiles = 0;
    struct rrdengine_datafile *df = NULL;
    while ((df = get_next_datafile(df, ctx, true))) {
        total_datafiles++;
        if (df->populate_mrg.populated)
            populated_datafiles++;
    }
    uv_rwlock_rdunlock(&ctx->datafiles.rwlock);

    if (total_datafiles == 0) {
        nd_log_daemon(NDLP_WARNING, "DBENGINE: No datafiles to populate MRG");
        worker_is_idle();
        return data;
    }

    do {
        struct rrdengine_datafile *datafile = NULL;

        // find a datafile to work on
        uv_rwlock_rdlock(&ctx->datafiles.rwlock);
        bool first_then_next = true;
        Pvoid_t *Pvalue =  NULL;
        Word_t Index = 0;
        while((Pvalue = JudyLFirstThenNext(ctx->datafiles.JudyL, &Index, &first_then_next))) {
            datafile = *Pvalue;
            if(!spinlock_trylock(&datafile->populate_mrg.spinlock)) {
                datafile = NULL;
                continue;
            }

            if(datafile->populate_mrg.populated) {
                spinlock_unlock(&datafile->populate_mrg.spinlock);
                datafile = NULL;
                continue;
            }
            break;
        }
        uv_rwlock_rdunlock(&ctx->datafiles.rwlock);

        if(!datafile)
            break;

        // Datafile populate mrg spinlock is acquired
        // Find an available thread slot or join finished threads
        bool thread_slot_found = false;

        while (!thread_slot_found) {
            // First, check for any finished threads to clean up
            for (size_t index = 0; index < max_threads; index++) {
                if (__atomic_load_n(&mlt[index].finished, __ATOMIC_RELAXED) &&
                    __atomic_load_n(&mlt[index].tier, __ATOMIC_ACQUIRE) == tier) {

                    rc = nd_thread_join(mlt[index].thread);
                    if (rc)
                        nd_log_daemon(NDLP_WARNING, "Failed to join thread, rc = %d", rc);

                    __atomic_store_n(&mlt[index].busy, false, __ATOMIC_RELEASE);
                    __atomic_store_n(&mlt[index].finished, false, __ATOMIC_RELEASE);
                    mlt[index].datafile->populate_mrg.populated = true;
                    populated_datafiles++;
                    spinlock_unlock(&mlt[index].datafile->populate_mrg.spinlock);

                    // We've cleaned up a thread slot, but we'll still look for a free one
                }
            }

            // Look for a free thread slot
            for (size_t index = 0; index < max_threads; index++) {
                bool expected = false;
                if (__atomic_compare_exchange_n(&(mlt[index].busy), &expected, true, false,
                                                __ATOMIC_ACQUIRE, __ATOMIC_RELAXED)) {
                    thread_index = index;
                    thread_slot_found = true;
                    break;
                }
            }

            if (!thread_slot_found) {
                // If we couldn't find a free slot after cleanup, wait a bit and try again
                sleep_usec(10 * USEC_PER_MS);
            }
        }

        // We have a thread slot (thread_index) and a datafile to process
        __atomic_store_n(&mlt[thread_index].tier, tier, __ATOMIC_RELAXED);
        mlt[thread_index].datafile = datafile;

        mlt[thread_index].thread = nd_thread_create("MRGLOAD", NETDATA_THREAD_OPTION_DEFAULT, journalfile_v2_populate_retention_to_mrg_worker,
                                                    &mlt[thread_index]);

        if (!mlt[thread_index].thread) {
            nd_log_daemon(NDLP_WARNING, "Failed to create thread for MRG population");
            __atomic_store_n(&mlt[thread_index].busy, false, __ATOMIC_RELEASE);
            spinlock_unlock(&datafile->populate_mrg.spinlock);
        }
        nd_log_limit_static_thread_var(erl, 10, 0);
        nd_log_limit(&erl, NDLS_DAEMON, NDLP_INFO, "DBENGINE: Tier %d MRG population completed: %.2f%% (%zu/%zu)", tier, (populated_datafiles * 100.0) / total_datafiles,
                     populated_datafiles, total_datafiles);
    } while(1);

    // We've processed all datafiles. Now wait for all our threads to complete
    bool threads_still_running;
    do {
        threads_still_running = false;

        for (size_t index = 0; index < max_threads; index++) {
            if (__atomic_load_n(&mlt[index].busy, __ATOMIC_ACQUIRE) &&
                __atomic_load_n(&mlt[index].tier, __ATOMIC_ACQUIRE) == tier) {

                if (__atomic_load_n(&mlt[index].finished, __ATOMIC_RELAXED)) {
                    // Thread is finished, join it
                    rc = nd_thread_join((mlt[index].thread));
                    if (rc)
                        nd_log_daemon(NDLP_WARNING, "Failed to join thread, rc = %d", rc);

                    __atomic_store_n(&mlt[index].busy, false, __ATOMIC_RELEASE);
                    __atomic_store_n(&mlt[index].finished, false, __ATOMIC_RELEASE);
                    mlt[index].datafile->populate_mrg.populated = true;
                    spinlock_unlock(&mlt[index].datafile->populate_mrg.spinlock);
                } else {
                    // Thread is still running
                    threads_still_running = true;
                }
            }
        }

        if (threads_still_running) {
            // Wait a bit before checking again
            sleep_usec(10 * USEC_PER_MS);
        }

    } while (threads_still_running);

    worker_is_idle();
    return data;
}

static void after_ctx_shutdown(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    ;
}

static void *ctx_shutdown_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    worker_is_busy(UV_EVENT_DBENGINE_SHUTDOWN);

    bool logged = false;
    while(__atomic_load_n(&ctx->atomic.extents_currently_being_flushed, __ATOMIC_RELAXED) ||
            __atomic_load_n(&ctx->atomic.inflight_queries, __ATOMIC_RELAXED)) {
        if(!logged) {
            logged = true;
            netdata_log_info("DBENGINE: waiting for %zu inflight queries to finish to shutdown tier %d...",
                 __atomic_load_n(&ctx->atomic.inflight_queries, __ATOMIC_RELAXED), ctx->config.tier);
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
    while (pgc_flush_pages(main_cache))
        yield_the_processor();

    return data;
}

static void *cache_evict_main_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *req __maybe_unused) {
    if (!main_cache)
        return data;

    worker_is_busy(UV_EVENT_DBENGINE_EVICT_MAIN_CACHE);
    while (pgc_evict_pages(main_cache, 0, 0))
        yield_the_processor();

    return data;
}

static void *cache_evict_open_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *req __maybe_unused) {
    if (!open_cache)
        return data;

    worker_is_busy(UV_EVENT_DBENGINE_EVICT_OPEN_CACHE);
    while (pgc_evict_pages(open_cache, 0, 0))
        yield_the_processor();

    return data;
}

static void *cache_evict_extent_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *req __maybe_unused) {
    if (!extent_cache)
        return data;

    worker_is_busy(UV_EVENT_DBENGINE_EVICT_EXTENT_CACHE);
    while (pgc_evict_pages(extent_cache, 0, 0))
        yield_the_processor();

    return data;
}

static void *query_prep_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *req __maybe_unused) {
    PDC *pdc = data;
    rrdeng_prep_query(pdc, true);
    return data;
}

uint64_t rrdeng_target_data_file_size(struct rrdengine_instance *ctx) {
    uint64_t target_size = ctx->config.max_disk_space ? ctx->config.max_disk_space / TARGET_DATAFILES : MAX_DATAFILE_SIZE;
    target_size = MIN(target_size, MAX_DATAFILE_SIZE);
    target_size = MAX(target_size, MIN_DATAFILE_SIZE);
    return target_size;
}

time_t get_datafile_end_time(struct rrdengine_instance *ctx)
{
    time_t last_time_s = 0;

    uv_rwlock_rdlock(&ctx->datafiles.rwlock);
    struct rrdengine_datafile *datafile = get_last_ctx_datafile(ctx, true);

    if (datafile) {
        last_time_s = datafile->journalfile->v2.last_time_s;
        if (!last_time_s)
            last_time_s = datafile->journalfile->v2.first_time_s;
    }

    uv_rwlock_rdunlock(&ctx->datafiles.rwlock);
    return last_time_s;
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
    netdata_log_debug(D_RRDENGINE, "%s called, active=%d.", __func__, uv_is_active((uv_handle_t *)handle));
}

#define TIMER_PERIOD_MS (1000)


static void *extent_read_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    EPDL *epdl = data;
    epdl_find_extent_and_populate_pages(ctx, epdl, true);
    return data;
}

static NOT_INLINE_HOT void epdl_populate_pages_asynchronously(struct rrdengine_instance *ctx, EPDL *epdl, STORAGE_PRIORITY priority) {
    rrdeng_enq_cmd(ctx, RRDENG_OPCODE_EXTENT_READ, epdl, NULL, priority,
                   rrdeng_enqueue_epdl_cmd, rrdeng_dequeue_epdl_cmd);
}

NOT_INLINE_HOT void pdc_route_asynchronously(struct rrdengine_instance *ctx, struct page_details_control *pdc) {
    pdc_to_epdl_router(ctx, pdc, epdl_populate_pages_asynchronously, epdl_populate_pages_asynchronously);
}

NOT_INLINE_HOT void epdl_populate_pages_synchronously(struct rrdengine_instance *ctx, EPDL *epdl, enum storage_priority priority __maybe_unused) {
    epdl_find_extent_and_populate_pages(ctx, epdl, false);
}

NOT_INLINE_HOT void pdc_route_synchronously(struct rrdengine_instance *ctx, struct page_details_control *pdc) {
    pdc_to_epdl_router(ctx, pdc, epdl_populate_pages_synchronously, epdl_populate_pages_synchronously);
}

NOT_INLINE_HOT void pdc_route_synchronously_first(struct rrdengine_instance *ctx, struct page_details_control *pdc) {
    pdc_to_epdl_router(ctx, pdc, epdl_populate_pages_synchronously, epdl_populate_pages_asynchronously);
}

static struct rrdengine_datafile *release_and_aquire_next_datafile_for_indexing(struct rrdengine_instance *ctx, struct rrdengine_datafile *release_datafile)
{
    struct rrdengine_datafile *datafile = NULL;

    uv_rwlock_rdlock(&ctx->datafiles.rwlock);
    if (release_datafile) {
        datafile = get_next_datafile(release_datafile, NULL, true);
        datafile_release(release_datafile, DATAFILE_ACQUIRE_INDEXING);
    }
    else
        datafile = get_first_ctx_datafile(ctx, true);

    while (datafile && datafile->fileno != ctx_last_fileno_get(ctx) && datafile->fileno != ctx_last_flush_fileno_get(ctx)) {
        if(journalfile_v2_data_available(datafile->journalfile)) {
            datafile = get_next_datafile(datafile, NULL, true);
            continue;
        }

        int retries = 5;
        bool locked = false;
        while (retries-- > 0) {
            locked = datafile_acquire(datafile, DATAFILE_ACQUIRE_INDEXING);
            if (locked)
                break;
            sleep_usec(200 * USEC_PER_MS);
        }
        if (locked) {
            uv_rwlock_rdunlock(&ctx->datafiles.rwlock);
            return datafile;
        }
        nd_log_daemon(NDLP_INFO, "DBENGINE: Datafile %u CANNOT be locked for indexing after retries; skipping", datafile->fileno);
        datafile = get_next_datafile(datafile, NULL, true);
    }
    uv_rwlock_rdunlock(&ctx->datafiles.rwlock);
    return NULL;
}


static void *journal_v2_indexing_tp_worker(struct rrdengine_instance *ctx, void *data, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    unsigned count = 0;

    if (unlikely(!ctx_is_available_for_queries(ctx)))
        return data;

    worker_is_busy(UV_EVENT_DBENGINE_JOURNAL_INDEX);
    struct rrdengine_datafile *datafile = NULL;
    char path[RRDENG_PATH_MAX];

    bool index_once = false;
    while ((datafile = release_and_aquire_next_datafile_for_indexing(ctx, datafile))) {

        spinlock_lock(&datafile->writers.spinlock);
        bool available = (datafile->writers.running || datafile->writers.flushed_to_open_running) ? false : true;
        spinlock_unlock(&datafile->writers.spinlock);

        journalfile_v1_generate_path(datafile, path, sizeof(path));

        if(!available) {
            nd_log_daemon(NDLP_NOTICE,
                   "DBENGINE: journal file \"%s\" needs to be indexed, but it has writers working on it - "
                   "skipping it for now",
                   path);
            continue;
        }

        if (index_once && unlikely(rrdeng_ctx_tier_cap_exceeded(ctx))) {
            nd_log_daemon(
                NDLP_INFO, "DBENGINE: tier %d reached quota limit, stopping journal indexing", ctx->config.tier);
            __atomic_store_n(&ctx->atomic.needs_indexing, true, __ATOMIC_RELAXED);
            datafile_release(datafile, DATAFILE_ACQUIRE_INDEXING);
            break;
        }
        nd_log_daemon(NDLP_INFO, "DBENGINE: journal file \"%s\" is ready to be indexed", path);

        pgc_open_cache_to_journal_v2(
            open_cache,
            (Word_t)ctx,
            (int)datafile->fileno,
            ctx->config.page_type,
            journalfile_migrate_to_v2_callback,
            (void *)datafile->journalfile,
            false);

        index_once = true;

        count++;

        // check if we are shutting down
        if (unlikely(!ctx_is_available_for_queries(ctx))) {
            datafile_release(datafile, DATAFILE_ACQUIRE_INDEXING);
            break;
        }
    }

    errno_clear();
    if(count)
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "DBENGINE: journal indexing done; %u files processed",
               count);

    worker_is_idle();

    return data;
}

static void after_do_cache_flush(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    rrdeng_main.flushes_running--;
}

static void after_do_main_cache_evict(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    rrdeng_main.evict_main_running--;
}

static void after_do_open_cache_evict(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    rrdeng_main.evict_open_running--;
}

static void after_do_extent_cache_evict(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    rrdeng_main.evict_extent_running--;
}

static void after_journal_v2_indexing(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    __atomic_store_n(&ctx->atomic.migration_to_v2_running, false, __ATOMIC_RELAXED);

    check_and_schedule_db_rotation(ctx);
}

struct rrdeng_buffer_sizes rrdeng_pulse_memory_sizes(void) {
    return (struct rrdeng_buffer_sizes) {
        .as = {
            [RRDENG_MEM_PGC]            = pgc_aral_stats(),
            [RRDENG_MEM_PGD]            = pgd_aral_stats(),
            [RRDENG_MEM_MRG]            = mrg_aral_stats(),
            [RRDENG_MEM_PDC]            = pdc_aral_stats(),
            [RRDENG_MEM_EPDL]           = epdl_aral_stats(),
            [RRDENG_MEM_DEOL]           = deol_aral_stats(),
            [RRDENG_MEM_PD]             = pd_aral_stats(),
            [RRDENG_MEM_EPDL_EXTENT]    = epdl_extent_aral_stats(),
            [RRDENG_MEM_OPCODES]        = aral_get_statistics(rrdeng_main.cmd_queue.ar),
            [RRDENG_MEM_HANDLES]        = aral_get_statistics(rrdeng_main.handles.ar),
            [RRDENG_MEM_DESCRIPTORS]    = aral_get_statistics(rrdeng_main.descriptors.ar),
            [RRDENG_MEM_WORKERS]        = aral_get_statistics(rrdeng_main.work_cmd.ar),
            [RRDENG_MEM_XT_IO]          = aral_get_statistics(rrdeng_main.xt_io_descr.ar),
        },
        .wal    = __atomic_load_n(&wal_globals.atomics.allocated, __ATOMIC_RELAXED) * (sizeof(WAL) + RRDENG_BLOCK_SIZE),
        .xt_buf = extent_buffer_cache_size(),
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

    return data;
}

uint64_t rrdeng_get_used_disk_space(struct rrdengine_instance *ctx, bool having_lock)
{
    uint64_t active_space = 0;

    if (!having_lock)
        uv_rwlock_rdlock(&ctx->datafiles.rwlock);

    struct rrdengine_datafile *first_datafile = get_first_ctx_datafile(ctx, true);
    struct rrdengine_datafile *last_datafile = get_last_ctx_datafile(ctx, true);

    if (first_datafile && last_datafile)
        active_space = last_datafile->pos;

    if (!having_lock)
        uv_rwlock_rdunlock(&ctx->datafiles.rwlock);

    // calculate the estimated disk space based on the expected final size of the datafile
    // We cant know the final v1/v2 journal size -- we let the current v1 size be part of the calculation by not
    // including it in the active_space
    uint64_t estimated_disk_space = ctx_current_disk_space_get(ctx) + rrdeng_target_data_file_size(ctx) - active_space;

    uint64_t database_space = get_total_database_space();
    uint64_t adjusted_database_space =  database_space * ctx->config.disk_percentage / 100 ;
    estimated_disk_space += adjusted_database_space;

    return estimated_disk_space;
}

static time_t get_tier_retention(struct rrdengine_instance *ctx)
{
    time_t retention = 0;
    if (localhost) {
        STORAGE_ENGINE *eng = localhost->db[ctx->config.tier].eng;
        if (eng) {
            time_t first_time_s = get_datafile_end_time(ctx);
            if (first_time_s)
                retention = now_realtime_sec() - first_time_s;
        }
    }
    return retention;
}

// Check if disk or retention time cap reached
bool rrdeng_ctx_tier_cap_exceeded(struct rrdengine_instance *ctx)
{
    uv_rwlock_rdlock(&ctx->datafiles.rwlock);
    struct rrdengine_datafile *first_datafile = get_first_ctx_datafile(ctx, true);

    if (!first_datafile || datafile_count(ctx, true) < 2) {
        uv_rwlock_rdunlock(&ctx->datafiles.rwlock);
        return false;
    }

    uint64_t estimated_disk_space = rrdeng_get_used_disk_space(ctx, true);

    uv_rwlock_rdunlock(&ctx->datafiles.rwlock);

    if (ctx->config.max_retention_s) {
        time_t retention = get_tier_retention(ctx);
        if (retention > ctx->config.max_retention_s) {
            __atomic_store_n(&ctx->datafiles.disk_time, false, __ATOMIC_RELAXED);
            return true;
        }
    }

    if (ctx->config.max_disk_space && estimated_disk_space > ctx->config.max_disk_space) {
        __atomic_store_n(&ctx->datafiles.disk_time, true, __ATOMIC_RELAXED);
        return true;
    }

    return false;
}

static void retention_timer_cb(uv_timer_t *handle) {
    if (!localhost)
        return;

    worker_is_busy(RRDENG_RETENTION_TIMER_CB);
    uv_stop(handle->loop);
    uv_update_time(handle->loop);

    for (size_t tier = 0; tier < nd_profile.storage_tiers; tier++) {
        STORAGE_ENGINE *eng = localhost->db[tier].eng;
        if (!eng || eng->seb != STORAGE_ENGINE_BACKEND_DBENGINE)
            continue;
        check_and_schedule_db_rotation(multidb_ctx[tier]);
    }

    worker_is_idle();
}

static void timer_per_sec_cb(uv_timer_t* handle) {
    worker_is_busy(RRDENG_TIMER_CB);
    uv_stop(handle->loop);
    uv_update_time(handle->loop);

    worker_set_metric(RRDENG_OPCODES_WAITING, (NETDATA_DOUBLE)rrdeng_main.cmd_queue.unsafe.waiting);
    worker_set_metric(RRDENG_WORKS_DISPATCHED, (NETDATA_DOUBLE)__atomic_load_n(&rrdeng_main.work_cmd.atomics.dispatched, __ATOMIC_RELAXED));
    worker_set_metric(RRDENG_WORKS_EXECUTING, (NETDATA_DOUBLE)__atomic_load_n(&rrdeng_main.work_cmd.atomics.executing, __ATOMIC_RELAXED));

    // rrdeng_enq_cmd(NULL, RRDENG_OPCODE_EVICT_MAIN, NULL, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);
    // rrdeng_enq_cmd(NULL, RRDENG_OPCODE_EVICT_OPEN, NULL, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);
    // rrdeng_enq_cmd(NULL, RRDENG_OPCODE_EVICT_EXTENT, NULL, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);
    rrdeng_enq_cmd(NULL, RRDENG_OPCODE_FLUSH_MAIN, NULL, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);
    rrdeng_enq_cmd(NULL, RRDENG_OPCODE_CLEANUP, NULL, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);

    worker_is_idle();
}

static void dbengine_initialize_structures(void) {
    pgd_init_arals();
    pgc_and_mrg_initialize();

    pdc_init();
    page_details_init();
    epdl_init();
    deol_init();
    epdl_extent_init();
    rrdeng_cmd_queue_init();
    work_request_init();
    rrdeng_query_handle_init();
    page_descriptors_init();
    extent_buffer_init();
    extent_io_descriptor_init();
}

bool rrdeng_dbengine_spawn(struct rrdengine_instance *ctx __maybe_unused) {
    static bool spawned = false;
    static SPINLOCK spinlock = SPINLOCK_INITIALIZER;

    spinlock_lock(&spinlock);

    if(!spawned) {
        int ret;

        ret = uv_loop_init(&rrdeng_main.loop);
        if (ret) {
            netdata_log_error("DBENGINE: uv_loop_init(): %s", uv_strerror(ret));
            return false;
        }
        rrdeng_main.loop.data = &rrdeng_main;

        ret = uv_async_init(&rrdeng_main.loop, &rrdeng_main.async, async_cb);
        if (ret) {
            netdata_log_error("DBENGINE: uv_async_init(): %s", uv_strerror(ret));
            fatal_assert(0 == uv_loop_close(&rrdeng_main.loop));
            return false;
        }
        rrdeng_main.async.data = &rrdeng_main;

        ret = uv_timer_init(&rrdeng_main.loop, &rrdeng_main.timer);
        if (ret) {
            netdata_log_error("DBENGINE: uv_timer_init(): %s", uv_strerror(ret));
            uv_close((uv_handle_t *)&rrdeng_main.async, NULL);
            fatal_assert(0 == uv_loop_close(&rrdeng_main.loop));
            return false;
        }

        ret = uv_timer_init(&rrdeng_main.loop, &rrdeng_main.retention_timer);
        if (ret) {
            netdata_log_error("DBENGINE: uv_timer_init(): %s", uv_strerror(ret));
            uv_close((uv_handle_t *)&rrdeng_main.async, NULL);
            fatal_assert(0 == uv_loop_close(&rrdeng_main.loop));
            return false;
        }

        rrdeng_main.timer.data = &rrdeng_main;
        rrdeng_main.retention_timer.data = &rrdeng_main;

        dbengine_initialize_structures();

        int retries = 0;
        rrdeng_main.thread = nd_thread_create("DBEV", NETDATA_THREAD_OPTION_DEFAULT, dbengine_event_loop, &rrdeng_main);

        fatal_assert(0 != rrdeng_main.thread);

        if (retries)
            nd_log_daemon(NDLP_WARNING, "DBENGINE thread was created after %d attempts", retries);

        spawned = true;
    }

    spinlock_unlock(&spinlock);
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

uint64_t rrdeng_get_directory_free_bytes_space(struct rrdengine_instance *ctx)
{
    uint64_t free_bytes = 0;
    struct statvfs buff_statvfs;
    if (statvfs(ctx->config.dbfiles_path, &buff_statvfs) == 0)
        free_bytes = buff_statvfs.f_bavail * buff_statvfs.f_bsize;

    return (free_bytes - (free_bytes * 5 / 100));
}

void rrdeng_calculate_tier_disk_space_percentage(void)
{
    uint64_t tier_space[RRD_STORAGE_TIERS];

    if (!localhost)
        return;

    uint64_t total_diskspace = 0;
    for(size_t tier = 0; tier < nd_profile.storage_tiers;tier++) {
        STORAGE_ENGINE *eng = localhost->db[tier].eng;
        if (!eng || eng->seb != STORAGE_ENGINE_BACKEND_DBENGINE) {
            tier_space[tier] = 0;
            continue;
        }
        uint64_t tier_disk_space = multidb_ctx[tier]->config.max_disk_space ?
                                       multidb_ctx[tier]->config.max_disk_space :
                                       rrdeng_get_directory_free_bytes_space(multidb_ctx[tier]);
        total_diskspace += tier_disk_space;
        tier_space[tier] = tier_disk_space;
    }

    if (total_diskspace) {
        for (size_t tier = 0; tier < nd_profile.storage_tiers; tier++) {
            multidb_ctx[tier]->config.disk_percentage = (100 * tier_space[tier] / total_diskspace);
        }
    }
}

#define NOT_DELETING_FILES(ctx)                                                                                        \
     (!__atomic_load_n(&(ctx)->atomic.now_deleting_files, __ATOMIC_RELAXED))

#define NOT_INDEXING_FILES(ctx)                                                                                        \
    (!__atomic_load_n(&(ctx)->atomic.migration_to_v2_running, __ATOMIC_RELAXED))

void *dbengine_event_loop(void* arg) {
    sanity_check();
    uv_thread_set_name_np("DBENGINE");
    service_register(NULL, NULL, NULL);

    worker_register("DBENGINE");

    // opcode jobs
    worker_register_job_name(RRDENG_OPCODE_NOOP,                                     "noop");

    worker_register_job_name(RRDENG_OPCODE_QUERY,                                    "query");
    worker_register_job_name(RRDENG_OPCODE_EXTENT_WRITE,                             "extent write");
    worker_register_job_name(RRDENG_OPCODE_EXTENT_READ,                              "extent read");
    worker_register_job_name(RRDENG_OPCODE_DATABASE_ROTATE,                          "db rotate");
    worker_register_job_name(RRDENG_OPCODE_JOURNAL_INDEX,                            "journal index");
    worker_register_job_name(RRDENG_OPCODE_FLUSH_MAIN,                               "flush init");
    worker_register_job_name(RRDENG_OPCODE_EVICT_MAIN,                               "evict init");
    worker_register_job_name(RRDENG_OPCODE_CTX_SHUTDOWN,                             "ctx shutdown");
    worker_register_job_name(RRDENG_OPCODE_CTX_FLUSH_DIRTY,                          "ctx flush dirty");
    worker_register_job_name(RRDENG_OPCODE_CTX_FLUSH_HOT_DIRTY,                      "ctx flush all");
    worker_register_job_name(RRDENG_OPCODE_CTX_QUIESCE,                              "ctx quiesce");
    worker_register_job_name(RRDENG_OPCODE_SHUTDOWN_EVLOOP,                          "dbengine shutdown");

    worker_register_job_name(RRDENG_OPCODE_MAX,                                      "get opcode");

    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_QUERY,                "query cb");
    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_EXTENT_WRITE,         "extent write cb");
    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_EXTENT_READ,          "extent read cb");
    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_DATABASE_ROTATE,      "db rotate cb");
    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_JOURNAL_INDEX,        "journal index cb");
    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_FLUSH_MAIN,           "flush init cb");
    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_EVICT_MAIN,           "evict init cb");
    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_CTX_SHUTDOWN,         "ctx shutdown cb");
    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_CTX_FLUSH_DIRTY,      "ctx flush dirty cb");
    worker_register_job_name(RRDENG_OPCODE_MAX + RRDENG_OPCODE_CTX_QUIESCE,          "ctx quiesce cb");

    // special jobs
    worker_register_job_name(RRDENG_RETENTION_TIMER_CB,                              "retention timer");
    worker_register_job_name(RRDENG_TIMER_CB,                                        "timer");

    worker_register_job_custom_metric(RRDENG_OPCODES_WAITING,  "opcodes waiting",  "opcodes", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(RRDENG_WORKS_DISPATCHED, "works dispatched", "works",   WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(RRDENG_WORKS_EXECUTING,  "works executing",  "works",   WORKER_METRIC_ABSOLUTE);

    struct rrdeng_main *main = arg;
    enum rrdeng_opcode opcode;
    struct rrdeng_cmd cmd;
    main->tid = gettid_cached();

    fatal_assert(0 == uv_timer_start(&main->timer, timer_per_sec_cb, TIMER_PERIOD_MS, TIMER_PERIOD_MS));
    fatal_assert(0 == uv_timer_start(&main->retention_timer, retention_timer_cb, TIMER_PERIOD_MS * 60, TIMER_PERIOD_MS * 60));

    bool shutdown = false;
    size_t cpus = netdata_conf_cpus();
    uv_sem_t sem;
    uv_sem_init(&sem, (unsigned int) cpus);
    struct mrg_load_thread *mlt = callocz(cpus, sizeof(*mlt));
    for (size_t i = 0; i < cpus; i++) {
        mlt[i].sem = &sem;
        mlt[i].max_threads = cpus;
        mlt[i].busy = false;
        mlt[i].finished = false;
    }

    while (likely(!shutdown)) {
        worker_is_idle();
        uv_run(&main->loop, UV_RUN_DEFAULT);

        /* wait for commands */
        size_t count = 0;
        do {
            count++;

            if(count % 100 == 0) {
                worker_is_idle();
                uv_run(&main->loop, UV_RUN_NOWAIT);
            }

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

                case RRDENG_OPCODE_FLUSH_MAIN: {
                    if(rrdeng_main.flushes_running < pgc_max_flushers()) {
                        rrdeng_main.flushes_running++;
                        work_dispatch(NULL, NULL, NULL, opcode, cache_flush_tp_worker, after_do_cache_flush);
                    }
                    break;
                }

                case RRDENG_OPCODE_EVICT_MAIN: {
                    if(rrdeng_main.evict_main_running < pgc_max_evictors()) {
                        rrdeng_main.evict_main_running++;
                        work_dispatch(NULL, NULL, NULL, opcode, cache_evict_main_tp_worker, after_do_main_cache_evict);
                    }
                    break;
                }

                case RRDENG_OPCODE_EVICT_OPEN: {
                    if(rrdeng_main.evict_open_running < pgc_max_evictors()) {
                        rrdeng_main.evict_open_running++;
                        work_dispatch(NULL, NULL, NULL, opcode, cache_evict_open_tp_worker, after_do_open_cache_evict);
                    }
                    break;
                }

                case RRDENG_OPCODE_EVICT_EXTENT: {
                    if(rrdeng_main.evict_extent_running < pgc_max_evictors()) {
                        rrdeng_main.evict_extent_running++;
                        work_dispatch(NULL, NULL, NULL, opcode, cache_evict_extent_tp_worker, after_do_extent_cache_evict);
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
                    // We no longer have an indexing command pending
                    ctx->datafiles.pending_index = false;
                    if (NOT_INDEXING_FILES(ctx) && ctx_is_available_for_queries(ctx)) {
                        __atomic_store_n(&ctx->atomic.migration_to_v2_running, true, __ATOMIC_RELAXED);
                        __atomic_store_n(&ctx->atomic.needs_indexing, false, __ATOMIC_RELAXED);
                        work_dispatch(ctx, datafile, NULL, opcode, journal_v2_indexing_tp_worker, after_journal_v2_indexing);
                    }
                    break;
                }

                case RRDENG_OPCODE_DATABASE_ROTATE: {
                    struct rrdengine_instance *ctx = cmd.ctx;
                    ctx->datafiles.pending_rotate = false;
                    if (NOT_DELETING_FILES(ctx) && datafile_count(ctx, false) > 2 &&
                        rrdeng_ctx_tier_cap_exceeded(ctx)) {
                        __atomic_store_n(&ctx->atomic.now_deleting_files, true, __ATOMIC_RELAXED);
                        work_dispatch(ctx, NULL, NULL, opcode, database_rotate_tp_worker, after_database_rotate);
                    }
                    break;
                }

                case RRDENG_OPCODE_CTX_POPULATE_MRG: {
                    struct rrdengine_instance *ctx = cmd.ctx;
                    struct completion *completion = cmd.completion;
                    work_dispatch(ctx, mlt, completion, opcode, populate_mrg_tp_worker, after_populate_mrg);
                    break;
                }

                case RRDENG_OPCODE_CTX_FLUSH_DIRTY: {
                    struct rrdengine_instance *ctx = cmd.ctx;
                    work_dispatch(ctx, NULL, NULL, opcode,
                                  flush_dirty_pages_of_section_tp_worker,
                                  after_flush_dirty_pages_of_section);
                    break;
                }

                case RRDENG_OPCODE_CTX_FLUSH_HOT_DIRTY: {
                    struct rrdengine_instance *ctx = cmd.ctx;
                    work_dispatch(ctx, NULL, NULL, opcode,
                                  flush_all_hot_and_dirty_pages_of_section_tp_worker,
                                  after_flush_all_hot_and_dirty_pages_of_section);
                    break;
                }

                case RRDENG_OPCODE_CTX_QUIESCE: {
                    // a ctx will shutdown shortly
                    struct rrdengine_instance *ctx = cmd.ctx;
                    nd_log_daemon(NDLP_INFO, "DBENGINE: Tier %d is shutting down — query processing disabled", ctx->config.tier);
                    __atomic_store_n(&ctx->quiesce.enabled, true, __ATOMIC_RELEASE);
                    break;
                }

                case RRDENG_OPCODE_CTX_SHUTDOWN: {
                    // a ctx is shutting down
                    struct rrdengine_instance *ctx = cmd.ctx;
                    struct completion *completion = cmd.completion;
                    work_dispatch(ctx, NULL, completion, opcode, ctx_shutdown_tp_worker, after_ctx_shutdown);
                    break;
                }

                case RRDENG_OPCODE_SHUTDOWN_EVLOOP: {
                    uv_close((uv_handle_t *)&main->async, NULL);

                    (void) uv_timer_stop(&main->timer);
                    uv_close((uv_handle_t *)&main->timer, NULL);

                    (void) uv_timer_stop(&main->retention_timer);
                    uv_close((uv_handle_t *)&main->retention_timer, NULL);
                    shutdown = true;
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
    freez(mlt);
    uv_sem_destroy(&sem);

    nd_log(NDLS_DAEMON, NDLP_DEBUG, "Shutting down dbengine thread");
    (void) uv_loop_close(&main->loop);
    worker_unregister();
    return NULL;
}

void dbengine_shutdown()
{
    rrdeng_enq_cmd(NULL, RRDENG_OPCODE_SHUTDOWN_EVLOOP, NULL, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);

    int rc = nd_thread_join(rrdeng_main.thread);
    if (rc)
        nd_log_daemon(NDLP_ERR, "DBENGINE: Failed to join thread, error %s", uv_err_name(rc));
    else
        nd_log_daemon(NDLP_INFO, "DBENGINE: thread shutdown completed");
}

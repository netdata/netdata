// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdengine.h"
#include "pdc.h"
#include "dbengine-compression.h"

#if WORKER_UTILIZATION_MAX_JOB_TYPES < (DBENGINE_OPCODE_MAX + 2)
#error Please increase WORKER_UTILIZATION_MAX_JOB_TYPES to at least (DBENGINE_MAX_OPCODE + 2)
#endif

struct dbengine_cmd {
    struct dbengine_tier *ctx;
    enum dbengine_opcode opcode;
    void *data;
    struct completion *completion;
    enum storage_priority priority;
    dequeue_callback_t dequeue_cb;

    struct {
        struct dbengine_cmd *prev;
        struct dbengine_cmd *next;
    } queue;
};

static inline struct dbengine_cmd dbengine_deq_cmd(struct dbengine_engine *engine, bool from_worker);
static inline void worker_dispatch_extent_read(struct dbengine_engine *engine, struct dbengine_cmd cmd, bool from_worker);
static inline void worker_dispatch_query_prep(struct dbengine_engine *engine, struct dbengine_cmd cmd, bool from_worker);

// serialises the creation of engines: the claim on the daemon's static tiers is made under it. The release, in
// dbengine_destroy(), relies on the embedder owning the engine from one thread, as the daemon does
static SPINLOCK dbengine_create_spinlock = SPINLOCK_INITIALIZER;

DBENGINE_LIFECYCLE_STATE dbengine_engine_lifecycle_state(struct dbengine_engine *engine) {
    spinlock_lock(&engine->lifecycle.spinlock);
    DBENGINE_LIFECYCLE_STATE state = engine->lifecycle.stopped ? DBENGINE_LIFECYCLE_STOPPED :
                                     engine->lifecycle.spawned ? DBENGINE_LIFECYCLE_RUNNING :
                                                                 DBENGINE_LIFECYCLE_DOWN;
    spinlock_unlock(&engine->lifecycle.spinlock);
    return state;
}

#if defined(OS_WINDOWS)
netdata_mutex_t dbengine_async_mutex;

__attribute__((constructor)) void initialize_dbengine_async_mutex(void)
{
    netdata_mutex_init(&dbengine_async_mutex);
}

__attribute__((destructor)) void destroy_dbengine_async_mutex(void)
{
    netdata_mutex_destroy(&dbengine_async_mutex);
}

void dbengine_async_wakeup(struct dbengine_engine *engine)
{

    if (__atomic_load_n(&engine->async_ready, __ATOMIC_RELAXED)) {
        netdata_mutex_lock(&dbengine_async_mutex);
        int rc = uv_async_send(&engine->async);
        if (rc)
            nd_log_daemon(NDLP_ERR,"DBENGINE: wakeup async error = %d", rc);

        netdata_mutex_unlock(&dbengine_async_mutex);
    } else {
        nd_log_daemon(NDLP_WARNING,"DBENGINE: wakeup async handler is being reset");
    }
}
#else
void dbengine_async_wakeup(struct dbengine_engine *engine)
{
    int rc = uv_async_send(&engine->async);
    if (rc)
        nd_log_daemon(NDLP_ERR,"DBENGINE: wakeup async error = %d", rc);
}
#endif

static void sanity_check(void)
{
    BUILD_BUG_ON(WORKER_UTILIZATION_MAX_JOB_TYPES < (DBENGINE_OPCODE_MAX + 2));

    /* Magic numbers must fit in the super-blocks */
    BUILD_BUG_ON(sizeof(DBENGINE_DF_MAGIC) - 1 > DBENGINE_MAGIC_SZ);
    BUILD_BUG_ON(sizeof(DBENGINE_JF_MAGIC) - 1 > DBENGINE_MAGIC_SZ);

    /* Version strings must fit in the super-blocks */
    BUILD_BUG_ON(sizeof(DBENGINE_DF_VER) - 1 > DBENGINE_VER_SZ);
    BUILD_BUG_ON(sizeof(DBENGINE_JF_VER) - 1 > DBENGINE_VER_SZ);

    /* Data file super-block cannot be larger than DBENGINE_BLOCK_SIZE */
    BUILD_BUG_ON(DBENGINE_DF_SB_PADDING_SZ < 0);

    BUILD_BUG_ON(sizeof(nd_uuid_t) != UUID_SZ); /* check UUID size */

    /* page count must fit in 8 bits */
    BUILD_BUG_ON(MAX_PAGES_PER_EXTENT > 255);
}

// ----------------------------------------------------------------------------
// work request cache

typedef void *(*work_cb)(struct dbengine_engine *engine, struct dbengine_tier *ctx, void *data, struct completion *completion, uv_work_t* req);
typedef void (*after_work_cb)(struct dbengine_engine *engine, struct dbengine_tier *ctx, void *data, struct completion *completion, uv_work_t* req, int status);

struct dbengine_work {
    uv_work_t req;

    struct dbengine_engine *engine;
    struct dbengine_tier *ctx;
    void *data;
    struct completion *completion;

    work_cb work_cb;
    after_work_cb after_work_cb;
    enum dbengine_opcode opcode;
};

static void work_request_init(struct dbengine_engine *engine) {
    engine->work_cmd.ar = aral_create(
        "dbengine-work-cmd",
        sizeof(struct dbengine_work),
        0,
        0,
        NULL,
        NULL, NULL, false, false, true
    );

}

enum LIBUV_WORKERS_STATUS {
    LIBUV_WORKERS_RELAXED,
    LIBUV_WORKERS_STRESSED,
    LIBUV_WORKERS_CRITICAL,
};

static inline enum LIBUV_WORKERS_STATUS work_request_full(struct dbengine_engine *engine) {
    size_t dispatched = __atomic_load_n(&engine->work_cmd.atomics.dispatched, __ATOMIC_RELAXED);

    if(dispatched >= (size_t)(engine->cfg.libuv_worker_threads))
        return LIBUV_WORKERS_CRITICAL;

    else if(dispatched >= (size_t)(engine->cfg.libuv_worker_threads - engine->cfg.reserved_libuv_worker_threads))
        return LIBUV_WORKERS_STRESSED;

    return LIBUV_WORKERS_RELAXED;
}

// This needs to be called from event loop thread only (callback)
static inline void check_and_schedule_db_rotation(struct dbengine_tier *ctx)
{
    struct dbengine_engine *engine = ctx->engine;

    internal_fatal(engine->tid != gettid_cached(), "check_and_schedule_db_rotation() can only be run from the event loop thread");

    // neither indexing nor rotation before the registry has been loaded from every journal,
    // nor once dbengine_tier_exit() has started taking the tier down
    if (!dbengine_tier_is_active(ctx) || !dbengine_ctx_is_mrg_populated(ctx))
        return;

    if (__atomic_load_n(&ctx->atomic.needs_indexing, __ATOMIC_RELAXED)) {
        if (ctx->datafiles.pending_index == false) {
            ctx->datafiles.pending_index = true;
            dbengine_enq_cmd(engine, ctx, DBENGINE_OPCODE_JOURNAL_INDEX, NULL, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);
        }
    }

    if (ctx->datafiles.pending_rotate) {
        nd_log_daemon(NDLP_DEBUG, "DBENGINE: tier %d is already pending rotation", ctx->config.tier);
        return;
    }

    if(dbengine_ctx_tier_cap_exceeded(ctx)) {
        ctx->datafiles.pending_rotate = true;
        dbengine_enq_cmd(engine, ctx, DBENGINE_OPCODE_DATABASE_ROTATE, NULL, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);
    }
}

// Prepares the calling pool thread for engine work: pool-thread setup (once per thread) and the
// engine's job names. Idempotent per thread; called at the start of every work item.
static void dbengine_worker_jobs_register(void) {
    static __thread bool registered = false;
    if(likely(registered))
        return;
    registered = true;

    libuv_worker_thread_init();

    worker_register_job_name(DBENGINE_WORKER_JOB_INIT, "worker init");

    // query related
    worker_register_job_name(DBENGINE_WORKER_JOB_QUERY, "query");
    worker_register_job_name(DBENGINE_WORKER_JOB_EXTENT_CACHE_LOOKUP, "extent cache");
    worker_register_job_name(DBENGINE_WORKER_JOB_EXTENT_MMAP, "extent mmap");
    worker_register_job_name(DBENGINE_WORKER_JOB_EXTENT_DECOMPRESSION, "extent decompression");
    worker_register_job_name(DBENGINE_WORKER_JOB_EXTENT_PAGE_LOOKUP, "page lookup");
    worker_register_job_name(DBENGINE_WORKER_JOB_EXTENT_PAGE_POPULATION, "page populate");
    worker_register_job_name(DBENGINE_WORKER_JOB_EXTENT_PAGE_ALLOCATION, "page allocate");

    // flushing related
    worker_register_job_name(DBENGINE_WORKER_JOB_FLUSH_MAIN_CACHE, "flush main");
    worker_register_job_name(DBENGINE_WORKER_JOB_EXTENT_WRITE, "extent write");
    worker_register_job_name(DBENGINE_WORKER_JOB_FLUSHED_TO_OPEN, "flushed to open");

    // datafile full
    worker_register_job_name(DBENGINE_WORKER_JOB_JOURNAL_INDEX, "jv2 indexing");

    // db rotation related
    worker_register_job_name(DBENGINE_WORKER_JOB_DATAFILE_DELETE_WAIT, "datafile delete wait");
    worker_register_job_name(DBENGINE_WORKER_JOB_DATAFILE_DELETE, "datafile deletion");
    worker_register_job_name(DBENGINE_WORKER_JOB_FIND_ROTATED_METRICS, "find rotated metrics");
    worker_register_job_name(DBENGINE_WORKER_JOB_FIND_REMAINING_RETENTION, "find remaining retention");
    worker_register_job_name(DBENGINE_WORKER_JOB_POPULATE_MRG, "update retention");

    // other
    worker_register_job_name(DBENGINE_WORKER_JOB_EVICT_MAIN_CACHE, "evict main");
    worker_register_job_name(DBENGINE_WORKER_JOB_EVICT_OPEN_CACHE, "evict open");
    worker_register_job_name(DBENGINE_WORKER_JOB_EVICT_EXTENT_CACHE, "evict extent");
    worker_register_job_name(DBENGINE_WORKER_JOB_BUFFERS_CLEANUP, "dbengine buffers cleanup");
    worker_register_job_name(DBENGINE_WORKER_JOB_FLUSH_DIRTY, "dbengine flush dirty");
    worker_register_job_name(DBENGINE_WORKER_JOB_QUIESCE, "dbengine quiesce");
    worker_register_job_name(DBENGINE_WORKER_JOB_SHUTDOWN, "dbengine shutdown");
    worker_register_job_name(DBENGINE_WORKER_JOB_MRG_LOAD, "jv2 mrg load");
}

static inline void work_done(struct dbengine_work *work_request) {
    aral_freez(work_request->engine->work_cmd.ar, work_request);
}

static void work_standard_worker(uv_work_t *req) {
    struct dbengine_work *work_request = req->data;
    struct dbengine_engine *engine = work_request->engine;

    __atomic_add_fetch(&engine->work_cmd.atomics.executing, 1, __ATOMIC_RELAXED);

    dbengine_worker_jobs_register();
    worker_is_busy(DBENGINE_WORKER_JOB_INIT);

    work_request->data = work_request->work_cb(engine, work_request->ctx, work_request->data, work_request->completion, req);
    worker_is_idle();

    if(work_request->opcode == DBENGINE_OPCODE_EXTENT_READ || work_request->opcode == DBENGINE_OPCODE_QUERY) {
        internal_fatal(work_request->after_work_cb != NULL, "DBENGINE: opcodes with a callback should not boosted");

        while(1) {
            struct dbengine_cmd cmd = dbengine_deq_cmd(engine, true);
            if (cmd.opcode == DBENGINE_OPCODE_NOOP)
                break;

            worker_is_busy(DBENGINE_WORKER_JOB_INIT);
            switch (cmd.opcode) {
                case DBENGINE_OPCODE_EXTENT_READ:
                    worker_dispatch_extent_read(engine, cmd, true);
                    break;

                case DBENGINE_OPCODE_QUERY:
                    worker_dispatch_query_prep(engine, cmd, true);
                    break;

                default:
                    fatal("DBENGINE: Opcode should not be executed synchronously");
                    break;
            }
            worker_is_idle();
        }
    }

    __atomic_sub_fetch(&engine->work_cmd.atomics.dispatched, 1, __ATOMIC_RELAXED);
    __atomic_sub_fetch(&engine->work_cmd.atomics.executing, 1, __ATOMIC_RELAXED);
}

static void after_work_standard_callback(uv_work_t* req, int status) {
    struct dbengine_work *work_request = req->data;

    worker_is_busy(DBENGINE_OPCODE_MAX + work_request->opcode);

    if(work_request->after_work_cb)
        work_request->after_work_cb(work_request->engine, work_request->ctx, work_request->data, work_request->completion, req, status);

    work_done(work_request);

    worker_is_idle();
}

static bool work_dispatch(struct dbengine_engine *engine, struct dbengine_tier *ctx, void *data, struct completion *completion, enum dbengine_opcode opcode, work_cb do_work_cb, after_work_cb do_after_work_cb) {
    struct dbengine_work *work_request = NULL;

    internal_fatal(engine->tid != gettid_cached(), "work_dispatch() can only be run from the event loop thread");

    work_request = aral_mallocz(engine->work_cmd.ar);
    memset(work_request, 0, sizeof(struct dbengine_work));
    work_request->req.data = work_request;
    work_request->engine = engine;
    work_request->ctx = ctx;
    work_request->data = data;
    work_request->completion = completion;
    work_request->work_cb = do_work_cb;
    work_request->after_work_cb = do_after_work_cb;
    work_request->opcode = opcode;

    if(uv_queue_work(&engine->loop, &work_request->req, work_standard_worker, after_work_standard_callback)) {
        internal_fatal(true, "DBENGINE: cannot queue work");
        // whoever waits on this must not wait forever
        if(completion)
            completion_mark_complete(completion);
        work_done(work_request);
        return false;
    }

    __atomic_add_fetch(&engine->work_cmd.atomics.dispatched, 1, __ATOMIC_RELAXED);

    return true;
}

// ----------------------------------------------------------------------------
// page descriptor cache

void page_descriptors_init(struct dbengine_engine *engine) {
    engine->descriptors.ar = aral_create(
            "dbengine-descriptors",
            sizeof(struct page_descr_with_data),
            0,
            0,
            NULL,
            NULL, NULL, false, false, true);

}

struct page_descr_with_data *page_descriptor_get(struct dbengine_engine *engine) {
    struct page_descr_with_data *descr = aral_mallocz(engine->descriptors.ar);
    memset(descr, 0, sizeof(struct page_descr_with_data));
    return descr;
}

static inline void page_descriptor_release(struct dbengine_engine *engine, struct page_descr_with_data *descr) {
    uuidmap_free(descr->uuid_id);
    aral_freez(engine->descriptors.ar, descr);
}

// ----------------------------------------------------------------------------
// extent io descriptor cache

static void extent_io_descriptor_init(struct dbengine_engine *engine) {
    engine->xt_io_descr.ar = aral_create(
            "dbengine-extent-io",
            sizeof(struct extent_io_descriptor),
            0,
            0,
            NULL,
            NULL, NULL, false, false, true
            );

}

static struct extent_io_descriptor *extent_io_descriptor_get(struct dbengine_engine *engine) {
    struct extent_io_descriptor *xt_io_descr = aral_mallocz(engine->xt_io_descr.ar);
    memset(xt_io_descr, 0, sizeof(struct extent_io_descriptor));
    return xt_io_descr;
}

static inline void extent_io_descriptor_release(struct dbengine_engine *engine, struct extent_io_descriptor *xt_io_descr) {
    aral_freez(engine->xt_io_descr.ar, xt_io_descr);
}

// ----------------------------------------------------------------------------
// query handle cache

void dbengine_query_handle_init(struct dbengine_engine *engine) {
    engine->handles.ar = aral_create(
            "dbengine-query-handles",
            sizeof(struct dbengine_query_handle),
            0,
            0,
            NULL,
            NULL, NULL, false, false, true);

}

ALWAYS_INLINE struct dbengine_query_handle *dbengine_query_handle_get(struct dbengine_engine *engine) {
    struct dbengine_query_handle *handle = aral_mallocz(engine->handles.ar);
    memset(handle, 0, sizeof(struct dbengine_query_handle));
    return handle;
}

ALWAYS_INLINE void dbengine_query_handle_release(struct dbengine_engine *engine, struct dbengine_query_handle *handle) {
    aral_freez(engine->handles.ar, handle);
}

// ----------------------------------------------------------------------------
// WAL cache

static size_t dbengine_active_tiers(void) {
    size_t active = 0;
    for(size_t tier = 0; tier < RRD_STORAGE_TIERS; tier++)
        if(dbengine_tier_is_active(dbengine_multidb_tiers[tier]))
            active++;
    return active;
}

static void wal_cleanup1(struct dbengine_engine *engine) {
    WAL *wal = NULL;

    if(!spinlock_trylock(&engine->wal.guarded.spinlock))
        return;

    if(engine->wal.guarded.available_items && engine->wal.guarded.available > dbengine_active_tiers()) {
        wal = engine->wal.guarded.available_items;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(engine->wal.guarded.available_items, wal, cache.prev, cache.next);
        engine->wal.guarded.available--;
    }

    spinlock_unlock(&engine->wal.guarded.spinlock);

    if(wal) {
        posix_memalign_freez(wal->buf);
        freez(wal);
        __atomic_sub_fetch(&engine->wal.atomics.allocated, 1, __ATOMIC_RELAXED);
    }
}

WAL *wal_get(struct dbengine_tier *ctx, unsigned size) {
    if(!size || size > DBENGINE_BLOCK_SIZE)
        fatal("DBENGINE: invalid WAL size requested");

    struct dbengine_engine *engine = ctx->engine;
    WAL *wal = NULL;

    spinlock_lock(&engine->wal.guarded.spinlock);

    if(likely(engine->wal.guarded.available_items)) {
        wal = engine->wal.guarded.available_items;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(engine->wal.guarded.available_items, wal, cache.prev, cache.next);
        engine->wal.guarded.available--;
    }

    uint64_t transaction_id = __atomic_fetch_add(&ctx->atomic.transaction_id, 1, __ATOMIC_RELAXED);
    spinlock_unlock(&engine->wal.guarded.spinlock);

    if(unlikely(!wal)) {
        wal = mallocz(sizeof(WAL));
        wal->buf_size = DBENGINE_BLOCK_SIZE;
        (void)posix_memalignz((void *)&wal->buf, RRDFILE_ALIGNMENT, wal->buf_size);
        __atomic_add_fetch(&engine->wal.atomics.allocated, 1, __ATOMIC_RELAXED);
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

void wal_release(struct dbengine_engine *engine, WAL *wal) {
    if(unlikely(!wal)) return;

    spinlock_lock(&engine->wal.guarded.spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(engine->wal.guarded.available_items, wal, cache.prev, cache.next);
    engine->wal.guarded.available++;
    spinlock_unlock(&engine->wal.guarded.spinlock);
}

// ----------------------------------------------------------------------------
// command queue cache

static void dbengine_cmd_queue_init(struct dbengine_engine *engine) {
    engine->cmd_queue.ar = aral_create("dbengine-opcodes",
                                           sizeof(struct dbengine_cmd),
                                           0,
                                           0,
                                           NULL,
                                           NULL, NULL, false, false, true);

}

static inline STORAGE_PRIORITY dbengine_enq_cmd_map_opcode_to_priority(enum dbengine_opcode opcode, STORAGE_PRIORITY priority) {
    if(unlikely(priority >= STORAGE_PRIORITY_INTERNAL_MAX_DONT_USE))
        priority = STORAGE_PRIORITY_BEST_EFFORT;

    switch(opcode) {
        case DBENGINE_OPCODE_QUERY:
            priority = STORAGE_PRIORITY_INTERNAL_QUERY_PREP;
            break;

        default:
            break;
    }

    return priority;
}

ALWAYS_INLINE void dbengine_enqueue_epdl_cmd(struct dbengine_cmd *cmd) {
    epdl_cmd_queued(cmd->data, cmd);
}

ALWAYS_INLINE void dbengine_dequeue_epdl_cmd(struct dbengine_cmd *cmd) {
    epdl_cmd_dequeued(cmd->data);
}

ALWAYS_INLINE void dbengine_req_cmd(struct dbengine_engine *engine, requeue_callback_t get_cmd_cb, void *data, STORAGE_PRIORITY priority) {
    spinlock_lock(&engine->cmd_queue.unsafe.spinlock);

    struct dbengine_cmd *cmd = get_cmd_cb(data);
    if(cmd) {
        priority = dbengine_enq_cmd_map_opcode_to_priority(cmd->opcode, priority);

        if (cmd->priority > priority) {
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(engine->cmd_queue.unsafe.waiting_items_by_priority[cmd->priority], cmd, queue.prev, queue.next);
            DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(engine->cmd_queue.unsafe.waiting_items_by_priority[priority], cmd, queue.prev, queue.next);
            cmd->priority = priority;
        }
    }

    spinlock_unlock(&engine->cmd_queue.unsafe.spinlock);
}

static ALWAYS_INLINE struct dbengine_cmd *dbengine_cmd_alloc(struct dbengine_engine *engine, struct dbengine_tier *ctx, enum dbengine_opcode opcode, void *data,
                                                         struct completion *completion, enum storage_priority priority,
                                                         dequeue_callback_t dequeue_cb) {
    struct dbengine_cmd *cmd = aral_mallocz(engine->cmd_queue.ar);
    memset(cmd, 0, sizeof(struct dbengine_cmd));
    cmd->ctx = ctx;
    cmd->opcode = opcode;
    cmd->data = data;
    cmd->completion = completion;
    cmd->priority = dbengine_enq_cmd_map_opcode_to_priority(opcode, priority);
    cmd->dequeue_cb = dequeue_cb;
    return cmd;
}

// Appends cmd and wakes the loop. With only_if_accepting, the acceptance check and the append happen under the
// same lock, so a refusal is final: dbengine_shutdown() cannot slip in between them. Returns false on refusal,
// leaving cmd untouched for the caller to free.
static ALWAYS_INLINE bool dbengine_cmd_append(struct dbengine_engine *engine, struct dbengine_cmd *cmd, enqueue_callback_t enqueue_cb, bool only_if_accepting) {
    spinlock_lock(&engine->cmd_queue.unsafe.spinlock);
    if(only_if_accepting && !engine->cmd_queue.unsafe.accepting) {
        spinlock_unlock(&engine->cmd_queue.unsafe.spinlock);
        return false;
    }
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(engine->cmd_queue.unsafe.waiting_items_by_priority[cmd->priority], cmd, queue.prev, queue.next);
    engine->cmd_queue.unsafe.waiting++;
    if(enqueue_cb)
        enqueue_cb(cmd);
    spinlock_unlock(&engine->cmd_queue.unsafe.spinlock);

    dbengine_async_wakeup(engine);
    return true;
}

ALWAYS_INLINE void dbengine_enq_cmd(struct dbengine_engine *engine, struct dbengine_tier *ctx, enum dbengine_opcode opcode, void *data,
               struct completion *completion, enum storage_priority priority, enqueue_callback_t enqueue_cb, dequeue_callback_t dequeue_cb) {
    struct dbengine_cmd *cmd = dbengine_cmd_alloc(engine, ctx, opcode, data, completion, priority, dequeue_cb);
    dbengine_cmd_append(engine, cmd, enqueue_cb, false);
}

static void dbengine_cmd_queue_set_accepting(struct dbengine_engine *engine, bool accepting) {
    spinlock_lock(&engine->cmd_queue.unsafe.spinlock);
    engine->cmd_queue.unsafe.accepting = accepting;
    spinlock_unlock(&engine->cmd_queue.unsafe.spinlock);
}

bool dbengine_work_available(struct dbengine_engine *engine) {
    if(!engine)
        return false;

    spinlock_lock(&engine->cmd_queue.unsafe.spinlock);
    bool accepting = engine->cmd_queue.unsafe.accepting;
    spinlock_unlock(&engine->cmd_queue.unsafe.spinlock);
    return accepting;
}

static inline bool dbengine_cmd_has_waiting_opcodes_in_lower_priorities(struct dbengine_engine *engine, STORAGE_PRIORITY priority, STORAGE_PRIORITY max_priority) {
    for(; priority <= max_priority ; priority++)
        if(engine->cmd_queue.unsafe.waiting_items_by_priority[priority])
            return true;

    return false;
}

#define opcode_empty (struct dbengine_cmd) {      \
    .ctx = NULL,                                \
    .opcode = DBENGINE_OPCODE_NOOP,               \
    .priority = STORAGE_PRIORITY_BEST_EFFORT,   \
    .completion = NULL,                         \
    .data = NULL,                               \
}

static inline struct dbengine_cmd dbengine_deq_cmd(struct dbengine_engine *engine, bool from_worker) {
    struct dbengine_cmd *cmd = NULL;
    enum LIBUV_WORKERS_STATUS status = work_request_full(engine);
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
    spinlock_lock(&engine->cmd_queue.unsafe.spinlock);
    for(STORAGE_PRIORITY priority = min_priority; priority <= max_priority ; priority++) {
        cmd = engine->cmd_queue.unsafe.waiting_items_by_priority[priority];
        if(cmd) {

            // avoid starvation of lower priorities
            if(unlikely(priority >= STORAGE_PRIORITY_HIGH &&
                        priority < STORAGE_PRIORITY_BEST_EFFORT &&
                        ++engine->cmd_queue.unsafe.executed_by_priority[priority] % 50 == 0 &&
                        dbengine_cmd_has_waiting_opcodes_in_lower_priorities(engine, priority + 1, max_priority))) {
                // let the others run 2% of the requests
                cmd = NULL;
                continue;
            }

            // remove it from the queue
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(engine->cmd_queue.unsafe.waiting_items_by_priority[priority], cmd, queue.prev, queue.next);
            engine->cmd_queue.unsafe.waiting--;
            break;
        }
    }

    if(cmd && cmd->dequeue_cb) {
        cmd->dequeue_cb(cmd);
        cmd->dequeue_cb = NULL;
    }

    spinlock_unlock(&engine->cmd_queue.unsafe.spinlock);

    struct dbengine_cmd ret;
    if(cmd) {
        // copy it, to return it
        ret = *cmd;

        aral_freez(engine->cmd_queue.ar, cmd);
    }
    else
        ret = opcode_empty;

    return ret;
}


// ----------------------------------------------------------------------------

static void journalfile_extent_build(struct dbengine_tier *ctx, struct extent_io_descriptor *xt_io_descr) {
    unsigned count, payload_length, descr_size, size_bytes;
    void *buf;
    /* persistent structures */
    struct dbengine_df_extent_header *df_header;
    struct dbengine_jf_transaction_header *jf_header;
    struct dbengine_jf_store_data *jf_metric_data;
    struct dbengine_jf_transaction_trailer *jf_trailer;
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
extent_flush_to_open(struct dbengine_tier *ctx, struct extent_io_descriptor *xt_io_descr, bool have_error)
{
    worker_is_busy(DBENGINE_WORKER_JOB_FLUSHED_TO_OPEN);

    struct page_descr_with_data *descr;
    struct dbengine_datafile *datafile;
    unsigned i;

    datafile = xt_io_descr->datafile;

    bool still_running = ctx_is_available_for_queries(ctx);
    if (likely(still_running && !have_error))
        internal_fatal(!dbengine_valid_extent_disk_size(xt_io_descr->bytes),
                       "DBENGINE: flushed extent has invalid size %u",
                       xt_io_descr->bytes);

    usec_t max_end_time_ut = 0;
    for (i = 0 ; i < xt_io_descr->descr_count ; ++i) {
        descr = xt_io_descr->descr_array[i];

        if (descr->end_time_ut > max_end_time_ut)
            max_end_time_ut = descr->end_time_ut;

        if (likely(still_running && !have_error)) {
            pgc_open_add_hot_page(
                (Word_t)ctx,
                descr->metric_id,
                descr->uuid_id,
                (time_t)(descr->start_time_ut / USEC_PER_SEC),
                (time_t)(descr->end_time_ut / USEC_PER_SEC),
                descr->update_every_s,
                datafile,
                xt_io_descr->pos,
                xt_io_descr->bytes);
        }

        page_descriptor_release(ctx->engine, descr);
    }

    if (!have_error) {
        if (max_end_time_ut > 0) {
            time_t new_last_time_s = (time_t)(max_end_time_ut / USEC_PER_SEC);

            // Atomically update to keep the maximum
            spinlock_tracked_lock(&datafile->journalfile->data_spinlock);
            if (new_last_time_s > datafile->journalfile->v2.last_time_s)
                datafile->journalfile->v2.last_time_s = new_last_time_s;
            spinlock_tracked_unlock(&datafile->journalfile->data_spinlock);
        }
    }

    posix_memalign_freez(xt_io_descr->buf);
    extent_io_descriptor_release(ctx->engine, xt_io_descr);

    spinlock_lock(&datafile->writers.spinlock);
    datafile->writers.flushed_to_open_running--;
    spinlock_unlock(&datafile->writers.spinlock);

    if(datafile->fileno != ctx_last_fileno_get(ctx) && still_running)
        __atomic_store_n(&ctx->atomic.needs_indexing, true, __ATOMIC_RELAXED);

    worker_is_idle();
}

// Main event loop callback

static bool datafile_is_full(struct dbengine_tier *ctx, struct dbengine_datafile *datafile, uint64_t extent_size) {
    bool ret = false;

    spinlock_lock(&datafile->writers.spinlock);

    if(unlikely(datafile->writers.failed)) {
        // this datafile cannot accept any more extents
        spinlock_unlock(&datafile->writers.spinlock);
        return true;
    }

#ifdef OS_WINDOWS
    time_t now = now_realtime_sec();
    if (now - datafile->writers.last_sync_time > 60) {
        sync_uv_file_data(datafile->file);
        sync_uv_file_data(datafile->journalfile->file);
        datafile->writers.last_sync_time = now_realtime_sec();
    }
#endif

    // Check if adding this extent would exceed the target size
    if(datafile->pos + extent_size > dbengine_target_data_file_size(ctx))
        ret = true;

    spinlock_unlock(&datafile->writers.spinlock);

    return ret;
}

size_t datafile_count(struct dbengine_tier *ctx, bool with_lock)
{
    size_t count = 0;

    if (!with_lock)
        netdata_rwlock_rdlock(&ctx->datafiles.rwlock);

    count = JudyLCount(ctx->datafiles.JudyL, 0, -1, PJE0);

    if (!with_lock)
        netdata_rwlock_rdunlock(&ctx->datafiles.rwlock);

    return count;
}

struct dbengine_datafile *
get_next_datafile(struct dbengine_datafile *this_datafile, struct dbengine_tier *ctx, bool with_lock)
{
    struct dbengine_datafile *datafile = NULL;

    ctx = this_datafile ? this_datafile->ctx : ctx;
    if (!ctx)
        return NULL;

    if (!with_lock)
        netdata_rwlock_rdlock(&ctx->datafiles.rwlock);

    Word_t Index = this_datafile ? this_datafile->fileno : 0;
    Pvoid_t *Pvalue;

    Pvalue = JudyLNext(ctx->datafiles.JudyL, &Index, PJE0);

    if (Pvalue)
        datafile = *Pvalue;

    if (!with_lock)
        netdata_rwlock_rdunlock(&ctx->datafiles.rwlock);

    return datafile;
}

static struct dbengine_datafile *get_ctx_datafile_first_or_last(struct dbengine_tier *ctx, bool first, bool with_lock)
{
    struct dbengine_datafile *datafile = NULL;

    if (!with_lock)
        netdata_rwlock_rdlock(&ctx->datafiles.rwlock);

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
        netdata_rwlock_rdunlock(&ctx->datafiles.rwlock);

    return datafile;
}

struct dbengine_datafile *get_first_ctx_datafile(struct dbengine_tier *ctx, bool with_lock) {
    return get_ctx_datafile_first_or_last(ctx, true, with_lock);
}

struct dbengine_datafile *get_last_ctx_datafile(struct dbengine_tier *ctx, bool with_lock) {
    return get_ctx_datafile_first_or_last(ctx, false, with_lock);
}

static struct dbengine_datafile *get_datafile_to_write_extent(struct dbengine_tier *ctx, uint64_t extent_size) {
    struct dbengine_datafile *datafile;

    // Acquire the mutex at the beginning to make the entire check-and-act atomic
    // This prevents the race condition where multiple threads pass the "is full" check
    // before any of them increments the position, causing files to grow beyond limits
    netdata_mutex_lock(&ctx->engine->datafile_write_mutex);

    // get the latest datafile
    netdata_rwlock_rdlock(&ctx->datafiles.rwlock);

    datafile = get_last_ctx_datafile(ctx, true);
    // become a writer on this datafile, to prevent it from vanishing
    spinlock_lock(&datafile->writers.spinlock);
    datafile->writers.running++;
    spinlock_unlock(&datafile->writers.spinlock);
    netdata_rwlock_rdunlock(&ctx->datafiles.rwlock);

    if(datafile_is_full(ctx, datafile, extent_size)) {
        // remember the datafile we have become writers to
        struct dbengine_datafile *old_datafile = datafile;

        // Create a new datafile - since we hold the mutex, no other thread can interfere
        if(create_new_datafile_pair(ctx) == 0)
            __atomic_store_n(&ctx->atomic.needs_indexing, true, __ATOMIC_RELAXED);

        // get the new datafile
        netdata_rwlock_rdlock(&ctx->datafiles.rwlock);
        datafile = get_last_ctx_datafile(ctx, true);
        // become a writer on this datafile, to prevent it from vanishing
        spinlock_lock(&datafile->writers.spinlock);
        datafile->writers.running++;
        spinlock_unlock(&datafile->writers.spinlock);
        netdata_rwlock_rdunlock(&ctx->datafiles.rwlock);

        // release the writers on the old datafile
        spinlock_lock(&old_datafile->writers.spinlock);
        old_datafile->writers.running--;
#ifdef OS_WINDOWS
        sync_uv_file_data(old_datafile->file);
        sync_uv_file_data(old_datafile->journalfile->file);
        datafile->writers.last_sync_time = now_realtime_sec();
        old_datafile->writers.last_sync_time = now_realtime_sec();
#endif
        spinlock_unlock(&old_datafile->writers.spinlock);
    }

    netdata_mutex_unlock(&ctx->engine->datafile_write_mutex);

    return datafile;
}

/*
 * Take a page list in a judy array and write them
 */
static struct extent_io_descriptor *
datafile_extent_build(struct dbengine_tier *ctx, struct page_descr_with_data *base, uv_buf_t *iov)
{
    struct dbengine_engine *engine = ctx->engine;
    unsigned i;
    uint32_t real_io_size, size_bytes, count, pos;
    uint32_t uncompressed_payload_length, max_compressed_size, payload_offset;
    struct page_descr_with_data *descr, *eligible_pages[MAX_PAGES_PER_EXTENT];
    struct extent_io_descriptor *xt_io_descr;
    Word_t Index;
    uint8_t compression_algorithm = ctx->config.global_compress_alg;
    struct dbengine_datafile *datafile;
    /* persistent structures */
    struct dbengine_df_extent_header *header;
    struct dbengine_df_extent_trailer *trailer;
    uLong crc;

    for(descr = base, Index = 0, count = 0, uncompressed_payload_length = 0;
        descr && count != engine->cfg.pages_per_extent;
        descr = descr->link.next, Index++) {

        uncompressed_payload_length += descr->page_length;
        eligible_pages[count++] = descr;

    }

    if (!count) {
        // No decrement here. The only caller does `if (!xt_io_descr) goto done;` and `done:`
        // decrements extents_currently_being_flushed, so decrementing here too would take the
        // unsigned counter below zero and wrap it, and both waiters on it - shutdown's
        // ctx_shutdown_tp_worker() (rrdengine.c) and finalize_data_files() (datafile.c) - would
        // spin forever. Not reachable today (the only EXTENT_WRITE producer returns early on an
        // empty batch), but the accounting must not depend on that.
        return NULL;
    }

    xt_io_descr = extent_io_descriptor_get(ctx->engine);
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
        uuid_copy(*(nd_uuid_t *)header->descr[i].uuid, *uuidmap_uuid_ptr(descr->uuid_id));
        header->descr[i].page_length = descr->page_length;
        header->descr[i].start_time_ut = descr->start_time_ut;

        switch (descr->type) {
            case DBENGINE_PAGE_TYPE_ARRAY_32BIT:
            case DBENGINE_PAGE_TYPE_ARRAY_TIER1:
                header->descr[i].end_time_ut = descr->end_time_ut;
                break;
            case DBENGINE_PAGE_TYPE_GORILLA_32BIT:
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
        dbengine_compress(xt_io_descr->buf + payload_offset,
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
       header->compression_algorithm = DBENGINE_COMPRESSION_NONE;
       header->payload_length = compressed_size = uncompressed_payload_length;
    }

    // set the correct size
    size_bytes = payload_offset + compressed_size + sizeof(*trailer);

    if(compression_algorithm != DBENGINE_COMPRESSION_NONE) {
        __atomic_add_fetch(&ctx->stats.before_compress_bytes, uncompressed_payload_length, __ATOMIC_RELAXED);
        __atomic_add_fetch(&ctx->stats.after_compress_bytes, compressed_size, __ATOMIC_RELAXED);
    }

    real_io_size = ALIGN_BYTES_CEILING(size_bytes);

    // Pass the extent size so the check can determine if this extent will fit
    datafile = get_datafile_to_write_extent(ctx, real_io_size);
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


static void after_external_work(struct dbengine_engine *engine __maybe_unused, struct dbengine_tier *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* uv_work_req __maybe_unused, int status __maybe_unused)
{
    ;
}

static void *external_work_worker(
    struct dbengine_engine *engine __maybe_unused,
    struct dbengine_tier *ctx __maybe_unused,
    void *data,
    struct completion *completion,
    uv_work_t *req __maybe_unused)
{
    struct dbengine_work_request *work = data;
    work->fn(work->data);
    completion_mark_complete(completion);
    worker_is_idle();
    return NULL;
}

bool dbengine_enq_work(struct dbengine_engine *engine, struct dbengine_work_request *req) {
    if(!dbengine_work_available(engine))
        return false;   // no engine, never spawned, or already shut down: the queue's allocator may not even exist

    struct dbengine_cmd *cmd = dbengine_cmd_alloc(engine, NULL, DBENGINE_OPCODE_EXTERNAL_WORK, req, &req->completion, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL);
    if(!dbengine_cmd_append(engine, cmd, NULL, true)) {
        aral_freez(engine->cmd_queue.ar, cmd);
        return false;
    }
    return true;
}

static void after_extent_write(struct dbengine_engine *engine __maybe_unused, struct dbengine_tier *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* uv_work_req __maybe_unused, int status __maybe_unused)
{
    check_and_schedule_db_rotation(ctx);
}

static void datafile_mark_failed(struct dbengine_datafile *datafile) {
    spinlock_lock(&datafile->writers.spinlock);
    datafile->writers.failed = true;
    spinlock_unlock(&datafile->writers.spinlock);
}

// The extent buffer is position independent - only its WAL transaction references
// the reserved datafile offset. So, an extent that failed to be written can be
// retargeted to another datafile by reserving space on it and rebuilding the WAL.
// The caller must have marked the current datafile failed, so that rotation is forced.
static bool extent_move_to_new_datafile(struct dbengine_tier *ctx, struct extent_io_descriptor *xt_io_descr) {
    struct dbengine_datafile *old_datafile = xt_io_descr->datafile;

    // the WAL references the offset reserved on the failed datafile
    wal_release(ctx->engine, xt_io_descr->wal);
    xt_io_descr->wal = NULL;

    // release our writer slot on the failed datafile
    spinlock_lock(&old_datafile->writers.spinlock);
    old_datafile->writers.running--;
    spinlock_unlock(&old_datafile->writers.spinlock);

    // the failed datafile is reported full, so this rotates to a new datafile pair
    struct dbengine_datafile *datafile = get_datafile_to_write_extent(ctx, xt_io_descr->real_io_size);
    xt_io_descr->datafile = datafile;

    if(datafile == old_datafile)
        // rotation failed - a new datafile pair could not be created
        return false;

    spinlock_lock(&datafile->writers.spinlock);
    xt_io_descr->pos = datafile->pos;
    datafile->pos += xt_io_descr->real_io_size;
    spinlock_unlock(&datafile->writers.spinlock);

    journalfile_extent_build(ctx, xt_io_descr);
    ctx_last_flush_fileno_set(ctx, datafile->fileno);

    return true;
}

static int extent_write_to_datafile(struct dbengine_datafile *datafile, uv_buf_t *iov, uint64_t pos) {
    uv_fs_t request;

    int retries = 10;
    int ret = -1;
    while (ret < 0 && --retries) {
        ret = uv_fs_write(NULL, &request, datafile->file, iov, 1, (int64_t)pos, NULL);
        uv_fs_req_cleanup(&request);
        if (ret < 0) {
            if (ret == -ENOSPC || ret == -EBADF || ret == -EACCES || ret == -EROFS || ret == -EINVAL)
                break;
            sleep_usec(300 * USEC_PER_MS);
        }
    }

    return ret;
}

static void *extent_write_tp_worker(
    struct dbengine_engine *engine __maybe_unused,
    struct dbengine_tier *ctx,
    void *data,
    struct completion *completion __maybe_unused,
    uv_work_t *req __maybe_unused)
{
    worker_is_busy(DBENGINE_WORKER_JOB_EXTENT_WRITE);
    uv_buf_t iov;
    struct page_descr_with_data *base = data;
    struct extent_io_descriptor *xt_io_descr = datafile_extent_build(ctx, base, &iov);

    if (!xt_io_descr)
        goto done;

    int ret = -1;
    for (size_t attempt = 0; attempt < 2 ; attempt++) {
        worker_is_busy(DBENGINE_WORKER_JOB_EXTENT_WRITE);
        struct dbengine_datafile *datafile = xt_io_descr->datafile;

        ret = extent_write_to_datafile(datafile, &iov, xt_io_descr->pos);
        if (likely(ret >= 0)) {
            ctx_current_disk_space_increase(ctx, xt_io_descr->real_io_size);
            ctx_io_write_op_bytes(ctx, xt_io_descr->real_io_size);

            // journalfile_v1_extent_write() always releases the WAL
            ret = journalfile_v1_extent_write(ctx, datafile, xt_io_descr->wal);
            xt_io_descr->wal = NULL;

            if (likely(ret >= 0))
                break;
        }

        ctx_io_error(ctx);

        // this datafile pair dropped a write - no more extents should be directed to it
        datafile_mark_failed(datafile);

        if (attempt == 0) {
            nd_log_limit_static_global_var(dbengine_rotate_erl, 10, 0);
            nd_log_limit(&dbengine_rotate_erl, NDLS_DAEMON, NDLP_ERR,
                         "DBENGINE: tier %d datafile %u write failed (%s) - "
                         "rotating to a new datafile and retrying the extent, to prevent data loss",
                         ctx->config.tier, datafile->fileno, uv_strerror(ret));

            if (extent_move_to_new_datafile(ctx, xt_io_descr))
                continue;
        }

        break;
    }

    if (unlikely(ret < 0)) {
        // recovery failed - the pages of this extent are lost
        wal_release(ctx->engine, xt_io_descr->wal);
        xt_io_descr->wal = NULL;

        nd_log_limit_static_global_var(dbengine_erl, 10, 0);
        nd_log_limit(&dbengine_erl, NDLS_DAEMON, NDLP_ERR,
                     "DBENGINE: tier %d datafile %u write failed (%s) - the extent is lost",
                     ctx->config.tier, xt_io_descr->datafile->fileno, uv_strerror(ret));
    }

    struct dbengine_datafile *datafile = xt_io_descr->datafile;
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

static void after_database_rotate(struct dbengine_engine *engine __maybe_unused, struct dbengine_tier *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
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

struct dbengine_datafile *datafile_release_and_acquire_next_for_retention(struct dbengine_tier *ctx, struct dbengine_datafile *datafile) {

    netdata_rwlock_rdlock(&ctx->datafiles.rwlock);

    struct dbengine_datafile *next_datafile = get_next_datafile(datafile, NULL, true);

    while(next_datafile && !datafile_acquire(next_datafile, DATAFILE_ACQUIRE_RETENTION))
        next_datafile = get_next_datafile(next_datafile, NULL, true);

    netdata_rwlock_rdunlock(&ctx->datafiles.rwlock);

    datafile_release(datafile, DATAFILE_ACQUIRE_RETENTION);

    return next_datafile;
}

static time_t find_uuid_first_time(
    struct dbengine_tier *ctx,
    struct dbengine_datafile *datafile,
    struct uuid_first_time_s *uuid_first_entry_list,
    size_t count)
{
    struct dbengine_engine *engine = ctx->engine;
    volatile time_t global_first_time_s = LONG_MAX;

    // acquire the datafile to work with it
    netdata_rwlock_rdlock(&ctx->datafiles.rwlock);
    while(datafile && !datafile_acquire(datafile, DATAFILE_ACQUIRE_RETENTION))
        datafile = get_next_datafile(datafile, NULL, true);

    netdata_rwlock_rdunlock(&ctx->datafiles.rwlock);

    if (unlikely(!datafile))
        return global_first_time_s;

    unsigned journalfile_count = 0;
    volatile size_t binary_match = 0;
    volatile size_t not_matching_bsearches = 0;

    bool agent_shutdown = false;
    while (datafile) {
        size_t journal_v2_file_size = 0;
        struct journal_v2_header *j2_header = journalfile_v2_data_acquire_with_hint(
            datafile->journalfile, &journal_v2_file_size, 0, 0, JOURNALFILE_V2_ACCESS_RANDOM);
        if (!j2_header) {
            datafile = datafile_release_and_acquire_next_for_retention(ctx, datafile);
            continue;
        }

        bool any_matching = false;
        bool journal_access_failed = false;

        char file_path[DBENGINE_PATH_MAX];
        journalfile_v2_generate_path(datafile, file_path, sizeof(file_path));
        PROTECTED_ACCESS_SETUP(datafile->journalfile->mmap.data, datafile->journalfile->mmap.size, file_path, "read");
        if (no_signal_received) {
            time_t journal_start_time_s = (time_t)(j2_header->start_time_ut / USEC_PER_SEC);

            // same as journalfile_v2_populate_retention_to_mrg(): a zero header
            // start time means the journal has no metrics, not that its
            // retention starts at the epoch
            if (journal_start_time_s > 0 && journal_start_time_s < global_first_time_s)
                global_first_time_s = journal_start_time_s;

            size_t metric_offset = j2_header->metric_offset;
            size_t journal_metric_count = j2_header->metric_count;
            size_t metric_list_size;
            if (__builtin_mul_overflow(journal_metric_count, sizeof(struct journal_metric_list), &metric_list_size)) {
                nd_log_daemon(NDLP_ERR,
                              "DBENGINE: metric list size overflow in journalfile \"%s\" "
                              "(metric_count=%zu, entry_size=%zu), skipping it",
                              file_path, journal_metric_count, sizeof(struct journal_metric_list));
                journal_access_failed = true;
                goto release_journal;
            }
            if (metric_offset > journal_v2_file_size ||
                metric_list_size > journal_v2_file_size - metric_offset) {
                nd_log_daemon(NDLP_ERR,
                              "DBENGINE: metric list exceeds journal file size in journalfile \"%s\" "
                              "(metric_offset=%zu, list_size=%zu, file_size=%zu), skipping it",
                              file_path, metric_offset, metric_list_size, journal_v2_file_size);
                journal_access_failed = true;
                goto release_journal;
            }

            struct journal_metric_list *uuid_list =
                (struct journal_metric_list *)((uint8_t *)j2_header + metric_offset);
            struct uuid_first_time_s *uuid_original_entry;

            size_t journal_search_start = 0; // Start of remaining search space
            any_matching = false;
            for (size_t index = 0; index < count; ++index) {
                uuid_original_entry = &uuid_first_entry_list[index];

                if (uuid_original_entry->df_matched > 3 || uuid_original_entry->pages_found > 5)
                    continue;

                any_matching = true;
                if (journal_search_start >= journal_metric_count)
                    break;

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

                if (journal_search_start >= journal_metric_count)
                    break;

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
            journal_access_failed = true;
        }

release_journal:
        journalfile_v2_data_release(datafile->journalfile);

        if (agent_shutdown) {
            datafile_release(datafile, DATAFILE_ACQUIRE_RETENTION);
            break;
        }

        journalfile_count++;
        datafile = datafile_release_and_acquire_next_for_retention(ctx, datafile);
        if (journal_access_failed)
            continue;

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
                engine->open_cache, (Word_t)ctx,
                (Word_t)uuid_first_t_entry->metric, 0,
                PGC_SEARCH_FIRST);

        if (page) {
            time_t old_first_time_s = uuid_first_t_entry->first_time_s;

            time_t first_time_s = pgc_page_start_time_s(page);
            uuid_first_t_entry->first_time_s = MIN(uuid_first_t_entry->first_time_s, first_time_s);
            pgc_page_release(engine->open_cache, page);
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

static void update_metrics_first_time_s(struct dbengine_tier *ctx, struct dbengine_datafile *datafile_to_delete, struct dbengine_datafile *first_datafile_remaining, bool worker) {
    struct dbengine_engine *engine = ctx->engine;
    time_t global_first_time_s = LONG_MAX;

    if(worker)
        worker_is_busy(DBENGINE_WORKER_JOB_FIND_ROTATED_METRICS);

    struct dbengine_journalfile *journalfile = datafile_to_delete->journalfile;
    struct journal_v2_header *j2_header = journalfile_v2_data_acquire_with_hint(
        journalfile, NULL, 0, 0, JOURNALFILE_V2_ACCESS_SEQUENTIAL_DIRECTORY);

    if (unlikely(!j2_header)) {
        if (worker)
            worker_is_idle();
        return;
    }

    __atomic_add_fetch(&engine->cache_efficiency_stats.metrics_retention_started, 1, __ATOMIC_RELAXED);

    char file_path[DBENGINE_PATH_MAX];
    journalfile_v2_generate_path(datafile_to_delete, file_path, sizeof(file_path));

    struct uuid_first_time_s *uuid_first_t_entry;
    // PROTECTED_ACCESS_SETUP below uses sigsetjmp/siglongjmp (see
    // src/libnetdata/protected-access/protected-access.h). Per C11 7.13.2.1, non-volatile locals
    // that are modified between setjmp and longjmp have indeterminate values
    // on the recovery path. uuid_first_entry_list / count / added are all
    // mutated inside the protected region and then read afterwards (the
    // unconditional log line and the journal_access_failed cleanup loop), so
    // they must be volatile to keep the recovery path well-defined.
    // journal_access_failed is also marked volatile defensively: although it
    // is only assigned on a path that does not subsequently SIGBUS, future
    // edits could break that invariant, and the bool is read once on cleanup.
    struct uuid_first_time_s * volatile uuid_first_entry_list = NULL;
    volatile size_t count = 0;
    volatile size_t added = 0;
    volatile bool journal_access_failed = false;

    // Protect the mmap walk: reading j2_header->metric_offset, metric_count,
    // and the per-metric uuid_list[] entries can SIGBUS if the underlying v2
    // file has any unreadable page (truncated, sparse hole, transient I/O
    // error). Without a protected region active the process aborts. Same
    // pattern as find_uuid_first_time() added by commit 26b26ac25a (#22310);
    // this caller was missed at the time.
    // Scope the protected frame tightly to the mmap walk only. The
    // PROTECTED_ACCESS_AUTO_CLEANUP() guard inside PROTECTED_ACCESS_SETUP
    // declares a __attribute__((cleanup)) local; when this inner block exits
    // (normally or via the SIGBUS recovery else-branch falling through), the
    // cleanup runs and the protected-access depth drops back to its prior
    // value. Without this scoping, the frame would stay live through the
    // post-walk log, the data_release, the cleanup loop, the nested
    // find_uuid_first_time() (which registers its own frame), and the final
    // cleanup -- masking unrelated faults that might land in the mmap range
    // and inflating nesting depth unnecessarily.
    {
        PROTECTED_ACCESS_SETUP(journalfile->mmap.data, journalfile->mmap.size, file_path, "mrg-retention");
        if(no_signal_received) {
            size_t journal_v2_file_size = journalfile->mmap.size;
            size_t metric_offset = j2_header->metric_offset;
            count = j2_header->metric_count;
            size_t metric_list_size;
            size_t entry_list_size;
            // Also check count * sizeof(uuid_first_time_s) -- the allocation below
            // sizes the working array by count, and on 32-bit builds count can
            // pass the metric_list_size bound while still overflowing the entry
            // list multiplication (struct uuid_first_time_s is larger than
            // struct journal_metric_list).
            if (__builtin_mul_overflow(count, sizeof(struct journal_metric_list), &metric_list_size) ||
                __builtin_mul_overflow(count, sizeof(struct uuid_first_time_s), &entry_list_size) ||
                metric_offset > journal_v2_file_size ||
                metric_list_size > journal_v2_file_size - metric_offset) {
                nd_log_daemon(NDLP_ERR,
                              "DBENGINE: metric list exceeds journal file size in journalfile \"%s\" "
                              "(metric_offset=%zu, list_size=%zu, file_size=%zu), skipping retention update",
                              file_path, metric_offset, metric_list_size, journal_v2_file_size);
                journal_access_failed = true;
            }
            else {
                struct journal_metric_list *uuid_list = (struct journal_metric_list *)((uint8_t *) j2_header + metric_offset);
                uuid_first_entry_list = callocz(count, sizeof(struct uuid_first_time_s));

                for (size_t index = 0; index < count; ++index) {
                    // Copy uuid out of the mmap onto the stack BEFORE calling mrg.
                    // If a backing page is unreadable, uuid_copy SIGBUSes here and
                    // the protected region recovers cleanly; the mrg call then
                    // never executes. If we passed &uuid_list[index].uuid into
                    // mrg, a SIGBUS could fire INSIDE mrg while it holds internal
                    // locks, and siglongjmp would skip mrg's unlock paths.
                    nd_uuid_t local_uuid;
                    uuid_copy(local_uuid, uuid_list[index].uuid);

                    METRIC *metric = mrg_metric_get_and_acquire_by_uuid(engine->main_mrg, &local_uuid, (Word_t)ctx);
                    if (!metric)
                        continue;

                    uuid_first_entry_list[added].metric = metric;
                    uuid_first_entry_list[added].first_time_s = LONG_MAX;
                    uuid_first_entry_list[added].df_matched = 0;
                    uuid_first_entry_list[added].df_index_oldest = 0;
                    uuid_first_entry_list[added].uuid = mrg_metric_uuid(engine->main_mrg, metric);
                    added++;
                }
            }
        }
        else {
            // SIGBUS/SIGSEGV inside the mmap walk -- bail cleanly. The
            // PROTECTED_ACCESS_SETUP macro already rate-limits the error log.
            journal_access_failed = true;
        }
    }

    netdata_log_info(
        "DBENGINE: tier %d: recalculating retention for %zu metrics starting with datafile %u",
        ctx->config.tier,
        count,
        first_datafile_remaining ? first_datafile_remaining->fileno : 0);

    journalfile_v2_data_release(journalfile);

    if (unlikely(journal_access_failed)) {
        // Release any partially-acquired metrics; uuid_first_entry_list may
        // be NULL (signal received before callocz).
        for (size_t index = 0; index < added; ++index)
            mrg_metric_release(engine->main_mrg, uuid_first_entry_list[index].metric);
        goto done;
    }

    // Update the first time / last time for all metrics we plan to delete

    if(worker)
        worker_is_busy(DBENGINE_WORKER_JOB_FIND_REMAINING_RETENTION);

    global_first_time_s = find_uuid_first_time(ctx, first_datafile_remaining, uuid_first_entry_list, added);

    if (!ctx_is_available_for_queries(ctx)) {
        for (size_t index = 0; index < added; ++index) {
            uuid_first_t_entry = &uuid_first_entry_list[index];
            mrg_metric_release(engine->main_mrg, uuid_first_t_entry->metric);
        }
        goto done;
    }

    if(worker)
        worker_is_busy(DBENGINE_WORKER_JOB_POPULATE_MRG);

    netdata_log_info("DBENGINE: tier %d: updating metrics registry retention for %zu metrics", ctx->config.tier, added);

    size_t deleted_metrics = 0, zero_retention_referenced = 0, zero_disk_retention = 0, zero_disk_but_live = 0;
    for (size_t index = 0; index < added; ++index) {
        uuid_first_t_entry = &uuid_first_entry_list[index];

        if (!ctx_is_available_for_queries(ctx)) {
            mrg_metric_release(engine->main_mrg, uuid_first_t_entry->metric);
            continue;
        }

        if (likely(uuid_first_t_entry->first_time_s != LONG_MAX)) {

            time_t old_first_time_s = mrg_metric_get_first_time_s(engine->main_mrg, uuid_first_t_entry->metric);

            bool changed = mrg_metric_set_first_time_s_if_bigger(engine->main_mrg, uuid_first_t_entry->metric, uuid_first_t_entry->first_time_s);
            if (changed) {
                uint32_t update_every_s = mrg_metric_get_update_every_s(engine->main_mrg, uuid_first_t_entry->metric);
                uint64_t remove_samples;
                if (dbengine_retention_samples_delta(
                        ctx,
                        old_first_time_s,
                        uuid_first_t_entry->first_time_s,
                        update_every_s,
                        "advancing metric first retention time",
                        &remove_samples))
                    dbengine_atomic_uint64_sub_saturating(
                        ctx,
                        &ctx->atomic.samples,
                        remove_samples,
                        "samples",
                        "advancing metric first retention time");
            }
            mrg_metric_release(engine->main_mrg, uuid_first_t_entry->metric);
        }
        else {
            zero_disk_retention++;

            // there is no retention for this metric
            bool has_retention = mrg_metric_has_zero_disk_retention(engine->main_mrg, uuid_first_t_entry->metric);
            if (!has_retention) {
                time_t first_time_s = mrg_metric_get_first_time_s(engine->main_mrg, uuid_first_t_entry->metric);
                time_t last_time_s = mrg_metric_get_latest_time_s(engine->main_mrg, uuid_first_t_entry->metric);
                uint32_t update_every_s = mrg_metric_get_update_every_s(engine->main_mrg, uuid_first_t_entry->metric);
                uint64_t remove_samples;
                if (dbengine_retention_samples_delta(
                        ctx,
                        first_time_s,
                        last_time_s,
                        update_every_s,
                        "deleting a metric with zero disk retention",
                        &remove_samples))
                    dbengine_atomic_uint64_sub_saturating(
                        ctx,
                        &ctx->atomic.samples,
                        remove_samples,
                        "samples",
                        "deleting a metric with zero disk retention");

                bool deleted = mrg_metric_release_and_delete(engine->main_mrg, uuid_first_t_entry->metric);
                if(deleted)
                    deleted_metrics++;
                else
                    zero_retention_referenced++;
            }
            else {
                zero_disk_but_live++;
                mrg_metric_release(engine->main_mrg, uuid_first_t_entry->metric);
            }
        }
    }

    if (!ctx_is_available_for_queries(ctx))
        goto done;

    internal_error(zero_disk_retention,
                   "DBENGINE: tier %d: deleted %zu metrics from metrics registry; %zu still had zero retention but were referenced "
                   "(out of %zu total zero on-disk retention metrics, of which %zu have main cache retention)",
                   ctx->config.tier, deleted_metrics, zero_retention_referenced, zero_disk_retention, zero_disk_but_live);

    if(global_first_time_s != LONG_MAX)
        __atomic_store_n(&ctx->atomic.first_time_s, global_first_time_s, __ATOMIC_RELAXED);

done:
    freez(uuid_first_entry_list);

    if(worker)
        worker_is_idle();
}

void datafile_delete(
    struct dbengine_tier *ctx,
    struct dbengine_datafile *datafile,
    bool update_retention,
    bool disk_time,
    bool worker)
{
    struct dbengine_engine *engine = ctx->engine;
    unsigned tier = ctx->config.tier;
    unsigned fileno = datafile->fileno;

    if(worker)
        worker_is_busy(DBENGINE_WORKER_JOB_DATAFILE_DELETE_WAIT);

    bool datafile_got_for_deletion = datafile_acquire_for_deletion(datafile);
    size_t attempts = 0;

    while (!datafile_got_for_deletion) {
        if(worker)
            worker_is_busy(DBENGINE_WORKER_JOB_DATAFILE_DELETE_WAIT);

        datafile_got_for_deletion = datafile_acquire_for_deletion(datafile);

        if (!datafile_got_for_deletion) {
            if(++attempts >= 30) {
                // pending_deletion is already set, blocking new acquires.
                // Bail out and let the next rotation cycle retry - lockers
                // will drain over time since no new ones can be added.
                netdata_log_error("DBENGINE: tier %u: " DATAFILE_PREFIX DBENGINE_FILE_NUMBER_PRINT_TMPL
                                  " could not be acquired for deletion after %zu attempts (%u lockers remain)"
                                  " - will retry on next rotation",
                                  tier, datafile->tier, fileno, attempts, datafile->users.lockers);

                if(worker)
                    worker_is_idle();

                return;
            }

            netdata_log_info("DBENGINE: tier %u: waiting for " DATAFILE_PREFIX DBENGINE_FILE_NUMBER_PRINT_TMPL
                         " to be available for deletion, in use by %u users.",
                 tier, datafile->tier, fileno, datafile->users.lockers);

            __atomic_add_fetch(&engine->cache_efficiency_stats.datafile_deletion_spin, 1, __ATOMIC_RELAXED);
            sleep_usec(1 * USEC_PER_SEC);
        }
    }

    if (update_retention)
        update_metrics_first_time_s(ctx, datafile, get_next_datafile(datafile, NULL, false), worker);

//    if (!ctx_is_available_for_queries(ctx)) {
//        // agent is shutting down, we cannot continue
//        if(worker)
//            worker_is_idle();
//        return;
//    }

    __atomic_add_fetch(&engine->cache_efficiency_stats.datafile_deletion_started, 1, __ATOMIC_RELAXED);
    netdata_log_info("DBENGINE: tier %u: deleting " DATAFILE_PREFIX DBENGINE_FILE_NUMBER_PRINT_TMPL " to maintain %s.",
                     tier, datafile->tier, fileno, disk_time ? "disk quota" : "time retention");

    if(worker)
        worker_is_busy(DBENGINE_WORKER_JOB_DATAFILE_DELETE);

    struct dbengine_journalfile *journal_file;
    size_t deleted_bytes, journal_file_bytes, datafile_bytes;
    uint8_t deleted_journal_files = 0;
    uint8_t expected_journal_files = JOURNALFILE_DELETED_V1;
    bool deleted_datafile = false;
    unsigned datafile_tier = datafile->tier;
    int ret;

    netdata_rwlock_wrlock(&ctx->datafiles.rwlock);
    datafile_list_delete_unsafe(ctx, datafile);
    netdata_rwlock_wrunlock(&ctx->datafiles.rwlock);

    journal_file = datafile->journalfile;
    datafile_bytes = datafile->pos;
    journal_file_bytes = journalfile_current_size(journal_file);
    size_t journal_v2_bytes = journalfile_v2_data_size_get(journal_file);
    if (journalfile_v2_data_available(journal_file))
        expected_journal_files |= JOURNALFILE_DELETED_V2;
    deleted_bytes = 0;

    // This will delete journalfile_v2 and journalfile_v1 (returns bitmask of JOURNALFILE_DELETED_V1/V2)
    deleted_journal_files = journalfile_destroy_unsafe(journal_file, datafile);
    if (deleted_journal_files & JOURNALFILE_DELETED_V1)
        deleted_bytes += journal_file_bytes;
    if (deleted_journal_files & JOURNALFILE_DELETED_V2)
        deleted_bytes += journal_v2_bytes;
    // This will delete the datafile
    ret = destroy_data_file_unsafe(datafile);
    if (!ret) {
        deleted_datafile = true;
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

    bool del_ndf = deleted_datafile;
    bool del_njf = deleted_journal_files & JOURNALFILE_DELETED_V1;
    bool del_njfv2 = deleted_journal_files & JOURNALFILE_DELETED_V2;
    bool exp_njf = expected_journal_files & JOURNALFILE_DELETED_V1;
    bool exp_njfv2 = expected_journal_files & JOURNALFILE_DELETED_V2;

    if (del_ndf && del_njf && del_njfv2)
        netdata_log_info("DBENGINE: tier %u: deleted " DATAFILE_PREFIX DBENGINE_FILE_NUMBER_PRINT_TMPL " (.ndf, .njf, .njfv2), reclaimed %s.",
                         tier, datafile_tier, fileno, size_for_humans);
    else if (del_ndf && del_njf && !exp_njfv2)
        netdata_log_info("DBENGINE: tier %u: deleted " DATAFILE_PREFIX DBENGINE_FILE_NUMBER_PRINT_TMPL " (.ndf, .njf), reclaimed %s.",
                         tier, datafile_tier, fileno, size_for_humans);
    else if (del_ndf || del_njf || del_njfv2) {
        BUFFER *removed = buffer_create(0, NULL);
        BUFFER *failed = buffer_create(0, NULL);
        const char *sep;

        sep = "";
        if (del_ndf)   { buffer_strcat(removed, sep); buffer_strcat(removed, ".ndf");   sep = ", "; }
        if (del_njf)   { buffer_strcat(removed, sep); buffer_strcat(removed, ".njf");   sep = ", "; }
        if (del_njfv2) { buffer_strcat(removed, sep); buffer_strcat(removed, ".njfv2"); }

        sep = "";
        if (!del_ndf)   { buffer_strcat(failed, sep); buffer_strcat(failed, ".ndf");   sep = ", "; }
        if (exp_njf && !del_njf)   { buffer_strcat(failed, sep); buffer_strcat(failed, ".njf");   sep = ", "; }
        if (exp_njfv2 && !del_njfv2) { buffer_strcat(failed, sep); buffer_strcat(failed, ".njfv2"); }

        if(buffer_strlen(failed))
            netdata_log_error("DBENGINE: tier %u: partial delete of " DATAFILE_PREFIX DBENGINE_FILE_NUMBER_PRINT_TMPL
                              " - removed: %s, failed: %s, reclaimed %s.",
                              tier, datafile_tier, fileno,
                              buffer_tostring(removed), buffer_tostring(failed), size_for_humans);
        else
            netdata_log_info("DBENGINE: tier %u: deleted " DATAFILE_PREFIX DBENGINE_FILE_NUMBER_PRINT_TMPL " (%s), reclaimed %s.",
                             tier, datafile_tier, fileno, buffer_tostring(removed), size_for_humans);
        buffer_free(removed);
        buffer_free(failed);
    }
    else
        netdata_log_error("DBENGINE: tier %u: failed to delete " DATAFILE_PREFIX DBENGINE_FILE_NUMBER_PRINT_TMPL " to maintain %s.",
                          tier, datafile_tier, fileno, disk_time ? "disk quota" : "time retention");
}

static void *database_rotate_tp_worker(struct dbengine_engine *engine, struct dbengine_tier *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {

    struct dbengine_datafile *datafile = get_first_ctx_datafile(ctx, false);
    datafile_delete(ctx, datafile, ctx_is_available_for_queries(ctx), true, true);

    if(engine->cfg.on_db_rotation)
        engine->cfg.on_db_rotation();

    return data;
}

static void after_flush_all_hot_and_dirty_pages_of_section(struct dbengine_engine *engine __maybe_unused, struct dbengine_tier *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    ;
}

static void *flush_all_hot_and_dirty_pages_of_section_tp_worker(struct dbengine_engine *engine, struct dbengine_tier *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    worker_is_busy(DBENGINE_WORKER_JOB_QUIESCE);
    pgc_flush_all_hot_and_dirty_pages(engine->main_cache, (Word_t)ctx);

    for(size_t i = 0; i < pgc_max_flushers(engine->main_cache) ; i++)
        dbengine_enq_cmd(engine, NULL, DBENGINE_OPCODE_FLUSH_MAIN, NULL, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);

    return data;
}

static void after_flush_dirty_pages_of_section(struct dbengine_engine *engine __maybe_unused, struct dbengine_tier *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    ;
}

static void *flush_dirty_pages_of_section_tp_worker(struct dbengine_engine *engine, struct dbengine_tier *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    worker_is_busy(DBENGINE_WORKER_JOB_FLUSH_DIRTY);
    pgc_flush_dirty_pages(engine->main_cache, (Word_t)ctx);

    for(size_t i = 0; i < pgc_max_flushers(engine->main_cache) ; i++)
        dbengine_enq_cmd(engine, NULL, DBENGINE_OPCODE_FLUSH_MAIN, NULL, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);

    return data;
}

struct mrg_load_thread {
    uv_sem_t *sem;
    struct dbengine_datafile *datafile;
    size_t *total;
    size_t *populated_datafiles;
};

void journalfile_v2_populate_retention_to_mrg_worker(void *arg)
{
    struct mrg_load_thread *mlt = arg;
    struct dbengine_tier *ctx = mlt->datafile->ctx;

    journalfile_v2_populate_retention_to_mrg(ctx, mlt->datafile->journalfile);

    uv_sem_post(mlt->sem);
}

static void *tier_mrg_load(
    struct dbengine_engine *engine __maybe_unused,
    struct dbengine_tier *ctx __maybe_unused,
    void *data,
    struct completion *completion __maybe_unused,
    uv_work_t *req __maybe_unused)
{
    worker_is_busy(DBENGINE_WORKER_JOB_MRG_LOAD);
    struct mrg_load_thread *mlt = data;
    journalfile_v2_populate_retention_to_mrg_worker(mlt);
    mlt->datafile->populate_mrg.populated = true;
    spinlock_unlock(&mlt->datafile->populate_mrg.spinlock);

    // last touch of the datafile: a pending deletion may free it right after this
    datafile_release(mlt->datafile, DATAFILE_ACQUIRE_MRG_LOAD);

    __atomic_add_fetch(mlt->populated_datafiles, 1, __ATOMIC_RELAXED);
    __atomic_sub_fetch(mlt->total, 1, __ATOMIC_RELEASE);
    freez(mlt);
    worker_is_idle();
    return NULL;
}


static void after_populate_mrg(struct dbengine_engine *engine __maybe_unused, struct dbengine_tier *ctx, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    __atomic_store_n(&ctx->atomic.mrg_populated, true, __ATOMIC_RELEASE);

    if (completion)
        completion_mark_complete(completion);
}

static void *populate_mrg_tp_worker(
    struct dbengine_engine *engine __maybe_unused,
    struct dbengine_tier *ctx,
    void *data,
    struct completion *completion __maybe_unused,
    uv_work_t *uv_work_req __maybe_unused)
{
    worker_is_busy(DBENGINE_WORKER_JOB_POPULATE_MRG);

    struct mrg_load_thread *mlt = data;
    int tier = ctx->config.tier;

    netdata_rwlock_rdlock(&ctx->datafiles.rwlock);

    size_t total_datafiles = 0;
    size_t populated_datafiles = 0;
    struct dbengine_datafile *df = NULL;
    while ((df = get_next_datafile(df, ctx, true))) {
        total_datafiles++;
        if (df->populate_mrg.populated)
            populated_datafiles++;
    }
    netdata_rwlock_rdunlock(&ctx->datafiles.rwlock);

    if (total_datafiles == 0) {
        nd_log_daemon(NDLP_WARNING, "DBENGINE: tier %d: no datafiles to populate MRG", tier);
        worker_is_idle();
        return data;
    }

    size_t total = 0;
    Word_t last_index = 0;
    bool resume_scan = false;
    do {
        struct dbengine_datafile *datafile = NULL;

        // find a datafile to work on
        netdata_rwlock_rdlock(&ctx->datafiles.rwlock);
        Pvoid_t *Pvalue = NULL;
        Word_t Index = resume_scan ? last_index : 0;
        bool first_then_next = !resume_scan;
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

            // hold the datafile until its journal has been loaded; datafile_delete() waits for this
            // reference like it does for queries. The acquire fails only for a datafile already pending
            // deletion, whose retention is going away with it.
            if(!datafile_acquire(datafile, DATAFILE_ACQUIRE_MRG_LOAD)) {
                // mark it done, so the rescan below does not count it again
                datafile->populate_mrg.populated = true;
                spinlock_unlock(&datafile->populate_mrg.spinlock);
                nd_log_daemon(NDLP_INFO, "DBENGINE: tier %d: skipping " DATAFILE_PREFIX DBENGINE_FILE_NUMBER_PRINT_TMPL
                                         " for MRG population, it is pending deletion",
                              tier, datafile->tier, datafile->fileno);
                total_datafiles--;
                datafile = NULL;
                continue;
            }
            break;
        }
        netdata_rwlock_rdunlock(&ctx->datafiles.rwlock);

        if(!datafile)
            break;

        // resume next scan from current position
        last_index = Index;
        resume_scan = true;

        uv_sem_wait(mlt->sem);
        struct mrg_load_thread *local_mlt = callocz(1, sizeof(struct mrg_load_thread));
        local_mlt->datafile = datafile;
        local_mlt->sem = mlt->sem;
        local_mlt->total = &total;
        local_mlt->populated_datafiles = &populated_datafiles;
        __atomic_add_fetch(local_mlt->total, 1, __ATOMIC_RELAXED);
        dbengine_enq_cmd(ctx->engine, ctx, DBENGINE_OPCODE_MRG_LOAD, local_mlt, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);
        {
            nd_log_limit_static_thread_var(erl, 10, 0);
            size_t completed = __atomic_load_n(&populated_datafiles, __ATOMIC_RELAXED);
            nd_log_limit(&erl, NDLS_DAEMON, NDLP_INFO,
                "DBENGINE: tier %d: MRG population completed: %.2f%% (%zu/%zu)",
                tier, (completed * 100.0) / total_datafiles, completed, total_datafiles);
        }
    } while(1);

    // We've queued all datafiles. Now wait for all worker threads to complete.
    size_t pending;
    do {
        pending = __atomic_load_n(&total, __ATOMIC_ACQUIRE);
        if (pending) {
            nd_log_limit_static_thread_var(erl, 10, 0);
            size_t completed = __atomic_load_n(&populated_datafiles, __ATOMIC_RELAXED);
            nd_log_limit(&erl, NDLS_DAEMON, NDLP_INFO,
                "DBENGINE: tier %d: MRG population completed: %.2f%% (%zu/%zu), waiting for %zu workers",
                tier, (completed * 100.0) / total_datafiles, completed, total_datafiles, pending);
            sleep_usec(10 * USEC_PER_MS);
        }
    } while (pending > 0);

    worker_is_idle();
    return data;
}

static void after_ctx_shutdown(struct dbengine_engine *engine __maybe_unused, struct dbengine_tier *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    ;
}

static void *ctx_shutdown_tp_worker(struct dbengine_engine *engine __maybe_unused, struct dbengine_tier *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    worker_is_busy(DBENGINE_WORKER_JOB_SHUTDOWN);

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

static void *cache_flush_tp_worker(struct dbengine_engine *engine, struct dbengine_tier *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    if (!engine->main_cache)
        return data;

    worker_is_busy(DBENGINE_WORKER_JOB_FLUSH_MAIN_CACHE);
    while (pgc_flush_pages(engine->main_cache))
        yield_the_processor();

    return data;
}

static void *cache_evict_main_tp_worker(struct dbengine_engine *engine, struct dbengine_tier *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *req __maybe_unused) {
    if (!engine->main_cache)
        return data;

    worker_is_busy(DBENGINE_WORKER_JOB_EVICT_MAIN_CACHE);
    while (pgc_evict_pages(engine->main_cache, 0, 0))
        yield_the_processor();

    return data;
}

static void *cache_evict_open_tp_worker(struct dbengine_engine *engine, struct dbengine_tier *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *req __maybe_unused) {
    if (!engine->open_cache)
        return data;

    worker_is_busy(DBENGINE_WORKER_JOB_EVICT_OPEN_CACHE);
    while (pgc_evict_pages(engine->open_cache, 0, 0))
        yield_the_processor();

    return data;
}

static void *cache_evict_extent_tp_worker(struct dbengine_engine *engine, struct dbengine_tier *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *req __maybe_unused) {
    if (!engine->extent_cache)
        return data;

    worker_is_busy(DBENGINE_WORKER_JOB_EVICT_EXTENT_CACHE);
    while (pgc_evict_pages(engine->extent_cache, 0, 0))
        yield_the_processor();

    return data;
}

static void *query_prep_tp_worker(struct dbengine_engine *engine __maybe_unused, struct dbengine_tier *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *req __maybe_unused) {
    PDC *pdc = data;
    dbengine_prep_query(pdc, true);
    return data;
}

uint64_t dbengine_target_data_file_size(struct dbengine_tier *ctx) {
    uint64_t target_size = ctx->config.max_disk_space ? ctx->config.max_disk_space / TARGET_DATAFILES : MAX_DATAFILE_SIZE;
    target_size = MIN(target_size, MAX_DATAFILE_SIZE);
    target_size = MAX(target_size, MIN_DATAFILE_SIZE);
    return target_size;
}

/* return 0 on success */
int init_rrd_files(struct dbengine_tier *ctx)
{
    return init_data_files(ctx);
}

void finalize_rrd_files(struct dbengine_tier *ctx)
{
    return finalize_data_files(ctx);
}

#if defined(OS_WINDOWS)
void async_cb(uv_async_t *handle)
{
    struct dbengine_engine *engine = handle->data;
    engine->last_async_callback = uv_hrtime();

    netdata_log_debug(D_RRDENGINE, "%s called, active=%d.", __func__, uv_is_active((uv_handle_t *)handle));
}

static void async_closed_cb(uv_handle_t *handle)
{
    struct dbengine_engine *engine = handle->data;

    int ret = uv_async_init(handle->loop, &engine->async, async_cb);
    if (ret)
        netdata_log_error("DBENGINE: reinitializing uv_async_init(): %s", uv_strerror(ret));
    __atomic_store_n(&engine->async_ready, true, __ATOMIC_RELEASE);
}
#else
void async_cb(uv_async_t *handle __maybe_unused)
{
    netdata_log_debug(D_RRDENGINE, "%s called, active=%d.", __func__, uv_is_active((uv_handle_t *)handle));
}
#endif

#define TIMER_PERIOD_MS (1000)

static void *extent_read_tp_worker(struct dbengine_engine *engine __maybe_unused, struct dbengine_tier *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    EPDL *epdl = data;
    epdl_find_extent_and_populate_pages(ctx, epdl, true);
    return data;
}

static NOT_INLINE_HOT void epdl_populate_pages_asynchronously(struct dbengine_tier *ctx, EPDL *epdl, STORAGE_PRIORITY priority) {
    dbengine_enq_cmd(ctx->engine, ctx, DBENGINE_OPCODE_EXTENT_READ, epdl, NULL, priority,
                   dbengine_enqueue_epdl_cmd, dbengine_dequeue_epdl_cmd);
}

NOT_INLINE_HOT void pdc_route_asynchronously(struct dbengine_tier *ctx, struct page_details_control *pdc) {
    pdc_to_epdl_router(ctx, pdc, epdl_populate_pages_asynchronously, epdl_populate_pages_asynchronously);
}

NOT_INLINE_HOT void epdl_populate_pages_synchronously(struct dbengine_tier *ctx, EPDL *epdl, enum storage_priority priority __maybe_unused) {
    epdl_find_extent_and_populate_pages(ctx, epdl, false);
}

NOT_INLINE_HOT void pdc_route_synchronously(struct dbengine_tier *ctx, struct page_details_control *pdc) {
    pdc_to_epdl_router(ctx, pdc, epdl_populate_pages_synchronously, epdl_populate_pages_synchronously);
}

NOT_INLINE_HOT void pdc_route_synchronously_first(struct dbengine_tier *ctx, struct page_details_control *pdc) {
    pdc_to_epdl_router(ctx, pdc, epdl_populate_pages_synchronously, epdl_populate_pages_asynchronously);
}

static struct dbengine_datafile *release_and_aquire_next_datafile_for_indexing(struct dbengine_tier *ctx, struct dbengine_datafile *release_datafile)
{
    struct dbengine_datafile *datafile = NULL;

    netdata_rwlock_rdlock(&ctx->datafiles.rwlock);
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
            netdata_rwlock_rdunlock(&ctx->datafiles.rwlock);
            return datafile;
        }
        nd_log_daemon(NDLP_INFO, "DBENGINE: tier %d: " DATAFILE_PREFIX DBENGINE_FILE_NUMBER_PRINT_TMPL " cannot be locked for indexing after retries; skipping",
                      ctx->config.tier, datafile->tier, datafile->fileno);
        datafile = get_next_datafile(datafile, NULL, true);
    }
    netdata_rwlock_rdunlock(&ctx->datafiles.rwlock);
    return NULL;
}


static void *journal_v2_indexing_tp_worker(struct dbengine_engine *engine, struct dbengine_tier *ctx, void *data, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    unsigned count = 0;

    if (unlikely(!ctx_is_available_for_queries(ctx)))
        return data;

    worker_is_busy(DBENGINE_WORKER_JOB_JOURNAL_INDEX);
    struct dbengine_datafile *datafile = NULL;

    bool index_once = false;
    while ((datafile = release_and_aquire_next_datafile_for_indexing(ctx, datafile))) {

        spinlock_lock(&datafile->writers.spinlock);
        bool available = (datafile->writers.running || datafile->writers.flushed_to_open_running) ? false : true;
        spinlock_unlock(&datafile->writers.spinlock);

        if(!available) {
            nd_log_daemon(NDLP_NOTICE,
                   "DBENGINE: tier %d: " DATAFILE_PREFIX DBENGINE_FILE_NUMBER_PRINT_TMPL
                   " needs to be indexed, but it has writers working on it - skipping it for now",
                   ctx->config.tier, datafile->tier, datafile->fileno);
            continue;
        }

        if (index_once && unlikely(dbengine_ctx_tier_cap_exceeded(ctx))) {
            nd_log_daemon(
                NDLP_INFO, "DBENGINE: tier %d: reached quota limit, stopping journal indexing", ctx->config.tier);
            __atomic_store_n(&ctx->atomic.needs_indexing, true, __ATOMIC_RELAXED);
            datafile_release(datafile, DATAFILE_ACQUIRE_INDEXING);
            break;
        }
        nd_log_daemon(NDLP_INFO, "DBENGINE: tier %d: " DATAFILE_PREFIX DBENGINE_FILE_NUMBER_PRINT_TMPL " is ready to be indexed",
                      ctx->config.tier, datafile->tier, datafile->fileno);

        pgc_open_cache_to_journal_v2(
            engine->open_cache,
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
               "DBENGINE: tier %d: journal indexing done; %u files processed",
               ctx->config.tier, count);

    worker_is_idle();

    return data;
}

static void after_do_cache_flush(struct dbengine_engine *engine, struct dbengine_tier *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    engine->flushes_running--;
}

static void after_do_main_cache_evict(struct dbengine_engine *engine, struct dbengine_tier *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    engine->evict_main_running--;
}

static void after_do_open_cache_evict(struct dbengine_engine *engine, struct dbengine_tier *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    engine->evict_open_running--;
}

static void after_do_extent_cache_evict(struct dbengine_engine *engine, struct dbengine_tier *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    engine->evict_extent_running--;
}

static void after_journal_v2_indexing(struct dbengine_engine *engine __maybe_unused, struct dbengine_tier *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    __atomic_store_n(&ctx->atomic.migration_to_v2_running, false, __ATOMIC_RELAXED);

    check_and_schedule_db_rotation(ctx);
}

// the name each DBENGINE_MEM slot is charted under; the daemon's pulse reads these together with the
// statistics below instead of the engine registering them one by one
const char *dbengine_mem_name(DBENGINE_MEM idx) {
    static const char *const names[DBENGINE_MEM_MAX] = {
        [DBENGINE_MEM_PGC]            = "pgc",
        [DBENGINE_MEM_PGD]            = "pgd",
        [DBENGINE_MEM_MRG]            = "mrg",
        [DBENGINE_MEM_OPCODES]        = "opcodes",
        [DBENGINE_MEM_HANDLES]        = "query handles",
        [DBENGINE_MEM_DESCRIPTORS]    = "descriptors",
        [DBENGINE_MEM_WORKERS]        = "workers",
        [DBENGINE_MEM_PDC]            = "pdc",
        [DBENGINE_MEM_XT_IO]          = "extent io",
        [DBENGINE_MEM_EPDL]           = "epdl",
        [DBENGINE_MEM_DEOL]           = "deol",
        [DBENGINE_MEM_PD]             = "pd",
        [DBENGINE_MEM_EPDL_EXTENT]    = "epdl_extent",
    };
    return idx < DBENGINE_MEM_MAX ? names[idx] : NULL;
}

struct dbengine_buffer_sizes dbengine_get_memory_sizes(struct dbengine_engine *engine) {
    struct dbengine_buffer_sizes sizes = {
        .as = {
            [DBENGINE_MEM_PGC]            = pgc_aral_stats(),
            [DBENGINE_MEM_PGD]            = pgd_aral_stats(),
            [DBENGINE_MEM_MRG]            = mrg_aral_stats(),
            [DBENGINE_MEM_PDC]            = pdc_aral_stats(),
            [DBENGINE_MEM_EPDL]           = epdl_aral_stats(),
            [DBENGINE_MEM_DEOL]           = deol_aral_stats(),
            [DBENGINE_MEM_PD]             = pd_aral_stats(),
            [DBENGINE_MEM_EPDL_EXTENT]    = epdl_extent_aral_stats(),
        },
        .xt_buf = extent_buffer_cache_size(),
    };

    // the engine's own allocators exist only while it does
    if(engine) {
        sizes.as[DBENGINE_MEM_OPCODES]     = aral_get_statistics(engine->cmd_queue.ar);
        sizes.as[DBENGINE_MEM_HANDLES]     = aral_get_statistics(engine->handles.ar);
        sizes.as[DBENGINE_MEM_DESCRIPTORS] = aral_get_statistics(engine->descriptors.ar);
        sizes.as[DBENGINE_MEM_WORKERS]     = aral_get_statistics(engine->work_cmd.ar);
        sizes.as[DBENGINE_MEM_XT_IO]       = aral_get_statistics(engine->xt_io_descr.ar);
        sizes.wal = __atomic_load_n(&engine->wal.atomics.allocated, __ATOMIC_RELAXED) * (sizeof(WAL) + DBENGINE_BLOCK_SIZE);
    }
    return sizes;
}

static void after_cleanup(struct dbengine_engine *engine, struct dbengine_tier *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    engine->cleanup_running--;
}

static void *cleanup_tp_worker(struct dbengine_engine *engine, struct dbengine_tier *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    worker_is_busy(DBENGINE_WORKER_JOB_BUFFERS_CLEANUP);

    wal_cleanup1(engine);
    extent_buffer_cleanup1();

    {
        time_t now_s = now_monotonic_sec();
        if(now_s - engine->cleanup_last_run_s >= 10) {
            engine->cleanup_last_run_s = now_s;
            journalfile_v2_data_unmount_cleanup(engine, now_s);
        }
    }

    return data;
}

uint64_t dbengine_get_used_disk_space_unsafe(struct dbengine_tier *ctx)
{
    uint64_t active_space = 0;

    struct dbengine_datafile *first_datafile = get_first_ctx_datafile(ctx, true);
    struct dbengine_datafile *last_datafile = get_last_ctx_datafile(ctx, true);

    if (first_datafile && last_datafile)
        active_space = last_datafile->pos;

    // calculate the estimated disk space based on the expected final size of the datafile
    // We cant know the final v1/v2 journal size -- we let the current v1 size be part of the calculation by not
    // including it in the active_space
    uint64_t estimated_disk_space = ctx_current_disk_space_get(ctx) + dbengine_target_data_file_size(ctx) - active_space;

    return estimated_disk_space;
}

uint64_t dbengine_get_used_disk_space(struct dbengine_tier *ctx)
{
    netdata_rwlock_rdlock(&ctx->datafiles.rwlock);
    uint64_t estimated_disk_space = dbengine_get_used_disk_space_unsafe(ctx);
    netdata_rwlock_rdunlock(&ctx->datafiles.rwlock);
    return estimated_disk_space;
}

// Check if disk or retention time cap reached
bool dbengine_ctx_tier_cap_exceeded(struct dbengine_tier *ctx)
{
    bool trigger_time_retention = false;
    uint64_t estimated_disk_space = 0;

    netdata_rwlock_rdlock(&ctx->datafiles.rwlock);
    struct dbengine_datafile *first_datafile = get_first_ctx_datafile(ctx, true);

    if (!first_datafile || datafile_count(ctx, true) < 2) {
        netdata_rwlock_rdunlock(&ctx->datafiles.rwlock);
        return false;
    }

    if (ctx->config.max_retention_s) {
        time_t last_time_s = first_datafile->journalfile->v2.last_time_s;
        if (!last_time_s)
            last_time_s = first_datafile->journalfile->v2.first_time_s;

        time_t cutoff_before_time_s = now_realtime_sec() - ctx->config.max_retention_s;
        trigger_time_retention = (last_time_s && last_time_s <= cutoff_before_time_s);
    }

    // avoid disk calculation if we will trigger time retention
    // calculate estimated disk space only if we have a disk cap
    if (false == trigger_time_retention && ctx->config.max_disk_space)
        estimated_disk_space = dbengine_get_used_disk_space_unsafe(ctx);

    netdata_rwlock_rdunlock(&ctx->datafiles.rwlock);

    if (trigger_time_retention) {
        __atomic_store_n(&ctx->datafiles.disk_time, false, __ATOMIC_RELAXED);
        return true;
    }

    if (ctx->config.max_disk_space && estimated_disk_space > ctx->config.max_disk_space) {
        __atomic_store_n(&ctx->datafiles.disk_time, true, __ATOMIC_RELAXED);
        return true;
    }

    return false;
}

static void retention_timer_cb(uv_timer_t *handle __maybe_unused)
{
    worker_is_busy(DBENGINE_RETENTION_TIMER_CB);

    for (size_t tier = 0; tier < RRD_STORAGE_TIERS; tier++) {
        struct dbengine_tier *ctx = dbengine_multidb_tiers[tier];
        if (!dbengine_tier_is_active(ctx))
            continue;
        check_and_schedule_db_rotation(ctx);
    }

    worker_is_idle();
}

static void timer_per_sec_cb(uv_timer_t *handle)
{
    struct dbengine_engine *engine = handle->data;

    worker_is_busy(DBENGINE_TIMER_CB);

    worker_set_metric(DBENGINE_OPCODES_WAITING, (NETDATA_DOUBLE)engine->cmd_queue.unsafe.waiting);
    worker_set_metric(DBENGINE_WORKS_DISPATCHED, (NETDATA_DOUBLE)__atomic_load_n(&engine->work_cmd.atomics.dispatched, __ATOMIC_RELAXED));
    worker_set_metric(DBENGINE_WORKS_EXECUTING, (NETDATA_DOUBLE)__atomic_load_n(&engine->work_cmd.atomics.executing, __ATOMIC_RELAXED));

    dbengine_enq_cmd(engine, NULL, DBENGINE_OPCODE_FLUSH_MAIN, NULL, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);
    dbengine_enq_cmd(engine, NULL, DBENGINE_OPCODE_CLEANUP, NULL, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);

    worker_is_idle();
}

static void dbengine_initialize_structures(struct dbengine_engine *engine) {
    pgd_init_arals(&engine->cfg.allocator);
    pgc_and_mrg_initialize(engine);

    pdc_init();
    page_details_init();
    epdl_init();
    deol_init();
    epdl_extent_init();
    dbengine_cmd_queue_init(engine);
    work_request_init(engine);
    dbengine_query_handle_init(engine);
    page_descriptors_init(engine);
    extent_buffer_init();
    extent_io_descriptor_init(engine);
}

// a spawn that failed after handles were opened on the loop: a closed handle leaves the loop only when a loop
// iteration runs its close callback, and the loop cannot be closed before that
static void dbengine_spawn_unwind(struct dbengine_engine *engine, bool timer_opened) {
#if defined(OS_WINDOWS)
    __atomic_store_n(&engine->async_ready, false, __ATOMIC_RELEASE);
#endif
    if(timer_opened)
        uv_close((uv_handle_t *)&engine->timer, NULL);
    uv_close((uv_handle_t *)&engine->async, NULL);
    uv_run(&engine->loop, UV_RUN_DEFAULT);
    fatal_assert(0 == uv_loop_close(&engine->loop));
    engine->loop_open = false;
}

// the loop, its handles, the structures and the thread; with the process lock held. A failed step closes what
// came before it and leaves the engine not spawned, so dbengine_create() reports the error and frees it
static int dbengine_spawn(struct dbengine_engine *engine) {
    int ret;

    ret = uv_loop_init(&engine->loop);
    if (ret) {
        netdata_log_error("DBENGINE: uv_loop_init(): %s", uv_strerror(ret));
        return ret;
    }
    engine->loop_open = true;
    engine->loop.data = engine;

    ret = uv_async_init(&engine->loop, &engine->async, async_cb);
    if (ret) {
        netdata_log_error("DBENGINE: uv_async_init(): %s", uv_strerror(ret));
        fatal_assert(0 == uv_loop_close(&engine->loop));
        engine->loop_open = false;
        return ret;
    }
    engine->async.data = engine;
#if defined(OS_WINDOWS)
    engine->async_ready = true;
#endif

    ret = uv_timer_init(&engine->loop, &engine->timer);
    if (ret) {
        netdata_log_error("DBENGINE: uv_timer_init(): %s", uv_strerror(ret));
        dbengine_spawn_unwind(engine, false);
        return ret;
    }

    ret = uv_timer_init(&engine->loop, &engine->retention_timer);
    if (ret) {
        netdata_log_error("DBENGINE: uv_timer_init(): %s", uv_strerror(ret));
        dbengine_spawn_unwind(engine, true);
        return ret;
    }

    engine->timer.data = engine;
    engine->retention_timer.data = engine;

    dbengine_initialize_structures(engine);

    engine->thread = nd_thread_create("DBEV", NETDATA_THREAD_OPTION_DEFAULT, dbengine_event_loop, engine);
    fatal_assert(0 != engine->thread);

    dbengine_cmd_queue_set_accepting(engine, true);

    spinlock_lock(&engine->lifecycle.spinlock);
    engine->lifecycle.spawned = true;
    spinlock_unlock(&engine->lifecycle.spinlock);
    return 0;
}

struct dbengine_engine *dbengine_engine_alloc(const struct dbengine_config *cfg) {
    if(!cfg)
        fatal("DBENGINE: the engine was given no configuration");

    struct dbengine_engine *engine = callocz(1, sizeof(*engine));

    engine->cfg = *cfg;
    dbengine_config_resolve(&engine->cfg);

    spinlock_init(&engine->lifecycle.spinlock);
    spinlock_init(&engine->cmd_queue.unsafe.spinlock);
    spinlock_init(&engine->wal.guarded.spinlock);
    netdata_mutex_init(&engine->datafile_write_mutex);

    return engine;
}

void dbengine_engine_free(struct dbengine_engine *engine) {
    if(!engine)
        return;

    // the loop is closed by the thread on its way out or by dbengine_shutdown() after it; an engine that spawned
    // is freed only after that
    internal_fatal(engine->loop_open, "DBENGINE: the engine is freed while its loop is open");

    // what the registry preload acquired holds references on metrics, so a registry that was freed was released
    // of it first
    internal_fatal(engine->preload.acquired.judyl, "DBENGINE: the engine is freed while its preload set still holds metrics");

    if(engine->cmd_queue.ar) aral_destroy(engine->cmd_queue.ar);
    if(engine->work_cmd.ar) aral_destroy(engine->work_cmd.ar);
    if(engine->handles.ar) aral_destroy(engine->handles.ar);
    if(engine->descriptors.ar) aral_destroy(engine->descriptors.ar);
    if(engine->xt_io_descr.ar) aral_destroy(engine->xt_io_descr.ar);

    WAL *wal;
    while((wal = engine->wal.guarded.available_items)) {
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(engine->wal.guarded.available_items, wal, cache.prev, cache.next);
        posix_memalign_freez(wal->buf);
        freez(wal);
    }

    netdata_mutex_destroy(&engine->datafile_write_mutex);
    freez(engine);
}

struct dbengine_engine *dbengine_create(const struct dbengine_config *cfg) {
    if(!cfg)
        fatal("DBENGINE: dbengine_create() called without a configuration");

    spinlock_lock(&dbengine_create_spinlock);

    struct dbengine_engine *engine = NULL;

    // the daemon's static tiers are the tier list of the engine that owns them (until the list moves into the
    // engine); one that a live engine, a stopped one or a retained one still points at is not free to be claimed:
    // dbengine_destroy() is what releases them, and a new engine can be made after it
    bool tiers_free = true;
    for(size_t tier = 0; tier < RRD_STORAGE_TIERS; tier++)
        if(dbengine_multidb_tiers[tier]->engine)
            tiers_free = false;

    if(!tiers_free)
        netdata_log_error("DBENGINE: the static tiers already belong to an engine, a second one cannot be made");

    else {
        // the configuration is resolved into the engine first: the caches and the allocators read it as they come
        // up. Every tier knows its engine before the spawn, which loads the registry through them
        engine = dbengine_engine_alloc(cfg);
        for(size_t tier = 0; tier < RRD_STORAGE_TIERS; tier++)
            dbengine_multidb_tiers[tier]->engine = engine;

        if(dbengine_spawn(engine)) {
            for(size_t tier = 0; tier < RRD_STORAGE_TIERS; tier++)
                dbengine_multidb_tiers[tier]->engine = NULL;

            // the spawn failed before its caches existed: nothing but the object and its locks
            dbengine_engine_free(engine);
            engine = NULL;
        }
    }

    spinlock_unlock(&dbengine_create_spinlock);
    return engine;
}

static inline void worker_dispatch_extent_read(struct dbengine_engine *engine, struct dbengine_cmd cmd, bool from_worker) {
    struct dbengine_tier *ctx = cmd.ctx;
    EPDL *epdl = cmd.data;

    if(from_worker)
        epdl_find_extent_and_populate_pages(ctx, epdl, true);
    else
        work_dispatch(engine, ctx, epdl, NULL, cmd.opcode, extent_read_tp_worker, NULL);
}

static inline void worker_dispatch_query_prep(struct dbengine_engine *engine, struct dbengine_cmd cmd, bool from_worker) {
    struct dbengine_tier *ctx = cmd.ctx;
    PDC *pdc = cmd.data;

    if(from_worker)
        dbengine_prep_query(pdc, true);
    else
        work_dispatch(engine, ctx, pdc, NULL, cmd.opcode, query_prep_tp_worker, NULL);
}

uint64_t dbengine_get_directory_free_bytes_space(struct dbengine_tier *ctx)
{
    uint64_t free_bytes = 0;
    OS_SYSTEM_DISK_SPACE space = os_disk_space(ctx->config.dbfiles_path);
    free_bytes = OS_SYSTEM_DISK_SPACE_OK(space) ? space.free_bytes : 0;
    return (free_bytes - (free_bytes * 5 / 100));
}

#define NOT_DELETING_FILES(ctx)                                                                                        \
     (!__atomic_load_n(&(ctx)->atomic.now_deleting_files, __ATOMIC_RELAXED))

#define NOT_INDEXING_FILES(ctx)                                                                                        \
    (!__atomic_load_n(&(ctx)->atomic.migration_to_v2_running, __ATOMIC_RELAXED))

void dbengine_event_loop(void* arg) {
    sanity_check();
    uv_thread_set_name_np("DBENGINE");

    worker_register("DBENGINE");

    // opcode jobs
    worker_register_job_name(DBENGINE_OPCODE_NOOP,                                     "noop");

    worker_register_job_name(DBENGINE_OPCODE_QUERY,                                    "query");
    worker_register_job_name(DBENGINE_OPCODE_EXTENT_WRITE,                             "extent write");
    worker_register_job_name(DBENGINE_OPCODE_EXTENT_READ,                              "extent read");
    worker_register_job_name(DBENGINE_OPCODE_DATABASE_ROTATE,                          "db rotate");
    worker_register_job_name(DBENGINE_OPCODE_JOURNAL_INDEX,                            "journal index");
    worker_register_job_name(DBENGINE_OPCODE_FLUSH_MAIN,                               "flush init");
    worker_register_job_name(DBENGINE_OPCODE_EVICT_MAIN,                               "evict init");
    worker_register_job_name(DBENGINE_OPCODE_CTX_SHUTDOWN,                             "ctx shutdown");
    worker_register_job_name(DBENGINE_OPCODE_CTX_FLUSH_DIRTY,                          "ctx flush dirty");
    worker_register_job_name(DBENGINE_OPCODE_CTX_FLUSH_HOT_DIRTY,                      "ctx flush all");
    worker_register_job_name(DBENGINE_OPCODE_CTX_QUIESCE,                              "ctx quiesce");
    worker_register_job_name(DBENGINE_OPCODE_SHUTDOWN_EVLOOP,                          "dbengine shutdown");
    worker_register_job_name(DBENGINE_OPCODE_EXTERNAL_WORK,                            "external work");
    worker_register_job_name(DBENGINE_OPCODE_MRG_LOAD,                                 "mrg tier load");


    worker_register_job_name(DBENGINE_OPCODE_MAX,                                      "get opcode");

    worker_register_job_name(DBENGINE_OPCODE_MAX + DBENGINE_OPCODE_QUERY,                "query cb");
    worker_register_job_name(DBENGINE_OPCODE_MAX + DBENGINE_OPCODE_EXTENT_WRITE,         "extent write cb");
    worker_register_job_name(DBENGINE_OPCODE_MAX + DBENGINE_OPCODE_EXTENT_READ,          "extent read cb");
    worker_register_job_name(DBENGINE_OPCODE_MAX + DBENGINE_OPCODE_DATABASE_ROTATE,      "db rotate cb");
    worker_register_job_name(DBENGINE_OPCODE_MAX + DBENGINE_OPCODE_JOURNAL_INDEX,        "journal index cb");
    worker_register_job_name(DBENGINE_OPCODE_MAX + DBENGINE_OPCODE_FLUSH_MAIN,           "flush init cb");
    worker_register_job_name(DBENGINE_OPCODE_MAX + DBENGINE_OPCODE_EVICT_MAIN,           "evict init cb");
    worker_register_job_name(DBENGINE_OPCODE_MAX + DBENGINE_OPCODE_CTX_SHUTDOWN,         "ctx shutdown cb");
    worker_register_job_name(DBENGINE_OPCODE_MAX + DBENGINE_OPCODE_CTX_FLUSH_DIRTY,      "ctx flush dirty cb");
    worker_register_job_name(DBENGINE_OPCODE_MAX + DBENGINE_OPCODE_CTX_QUIESCE,          "ctx quiesce cb");
    worker_register_job_name(DBENGINE_OPCODE_MAX + DBENGINE_OPCODE_EXTERNAL_WORK,        "external work cb");
    worker_register_job_name(DBENGINE_OPCODE_MAX + DBENGINE_OPCODE_MRG_LOAD,             "mrg tier load cb");

    // special jobs
    worker_register_job_name(DBENGINE_RETENTION_TIMER_CB,                              "retention timer");
    worker_register_job_name(DBENGINE_TIMER_CB,                                        "timer");

    worker_register_job_custom_metric(DBENGINE_OPCODES_WAITING,  "opcodes waiting",  "opcodes", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(DBENGINE_WORKS_DISPATCHED, "works dispatched", "works",   WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(DBENGINE_WORKS_EXECUTING,  "works executing",  "works",   WORKER_METRIC_ABSOLUTE);

    struct dbengine_engine *engine = arg;
    enum dbengine_opcode opcode;
    struct dbengine_cmd cmd;
    engine->tid = gettid_cached();

    fatal_assert(0 == uv_timer_start(&engine->timer, timer_per_sec_cb, TIMER_PERIOD_MS, TIMER_PERIOD_MS));
    fatal_assert(0 == uv_timer_start(&engine->retention_timer, retention_timer_cb, TIMER_PERIOD_MS * 60, TIMER_PERIOD_MS * 60));

    bool shutdown = false;
    size_t cpus = engine->cfg.cpus;
    uv_sem_t sem;
    uv_sem_init(&sem, (unsigned int) cpus);
    struct mrg_load_thread *mlt = callocz(cpus, sizeof(*mlt));
    for (size_t i = 0; i < cpus; i++) {
        mlt[i].sem = &sem;
    }

#if defined(OS_WINDOWS)
    engine->last_async_callback = uv_hrtime();
#endif

    while (likely(!shutdown)) {
        worker_is_idle();
        uv_run(&engine->loop, UV_RUN_ONCE);

        do {
            worker_is_busy(DBENGINE_OPCODE_MAX);
            cmd = dbengine_deq_cmd(engine, false);
            opcode = cmd.opcode;

            worker_is_busy(opcode);

            switch (opcode) {
                case DBENGINE_OPCODE_MRG_LOAD:
                    work_dispatch(engine, NULL, cmd.data, cmd.completion, cmd.opcode, tier_mrg_load, NULL);
                    break;

                case DBENGINE_OPCODE_EXTERNAL_WORK:
                    work_dispatch(engine, NULL, cmd.data, cmd.completion, cmd.opcode, external_work_worker, after_external_work);
                    break;

                case DBENGINE_OPCODE_EXTENT_READ:
                    worker_dispatch_extent_read(engine, cmd, false);
                    break;

                case DBENGINE_OPCODE_QUERY:;
#if defined(OS_WINDOWS)
                    if (uv_hrtime() - engine->last_async_callback > 1000UL * NSEC_PER_MSEC) {
                        if (++engine->async_timeout_count > 30) {
                            netdata_log_error("DBENGINE: async callback timeout detected, re-initializing the async handle");
                            __atomic_store_n(&engine->async_ready, false, __ATOMIC_RELEASE);
                            uv_close((uv_handle_t *)&engine->async, async_closed_cb);
                            engine->async_timeout_count = 0;
                        } else
                            netdata_log_error("DBENGINE: async callback timeout detected count = %d", engine->async_timeout_count);
                    }
#endif
                    worker_dispatch_query_prep(engine, cmd, false);
                    break;

                case DBENGINE_OPCODE_EXTENT_WRITE: {
                    struct dbengine_tier *ctx = cmd.ctx;
                    struct page_descr_with_data *base = cmd.data;
                    struct completion *completion = cmd.completion; // optional
                    work_dispatch(engine, ctx, base, completion, opcode, extent_write_tp_worker, after_extent_write);
                    break;
                }

                case DBENGINE_OPCODE_FLUSH_MAIN: {
                    if(engine->flushes_running < pgc_max_flushers(engine->main_cache)) {
                        engine->flushes_running++;
                        work_dispatch(engine, NULL, NULL, NULL, opcode, cache_flush_tp_worker, after_do_cache_flush);
                    }
                    break;
                }

                case DBENGINE_OPCODE_EVICT_MAIN: {
                    if(engine->evict_main_running < pgc_max_evictors(engine->main_cache)) {
                        engine->evict_main_running++;
                        work_dispatch(engine, NULL, NULL, NULL, opcode, cache_evict_main_tp_worker, after_do_main_cache_evict);
                    }
                    break;
                }

                case DBENGINE_OPCODE_EVICT_OPEN: {
                    if(engine->evict_open_running < pgc_max_evictors(engine->main_cache)) {
                        engine->evict_open_running++;
                        work_dispatch(engine, NULL, NULL, NULL, opcode, cache_evict_open_tp_worker, after_do_open_cache_evict);
                    }
                    break;
                }

                case DBENGINE_OPCODE_EVICT_EXTENT: {
                    if(engine->evict_extent_running < pgc_max_evictors(engine->main_cache)) {
                        engine->evict_extent_running++;
                        work_dispatch(engine, NULL, NULL, NULL, opcode, cache_evict_extent_tp_worker, after_do_extent_cache_evict);
                    }
                    break;
                }

                case DBENGINE_OPCODE_CLEANUP: {
                    if(!engine->cleanup_running) {
                        engine->cleanup_running++;
                        work_dispatch(engine, NULL, NULL, NULL, opcode, cleanup_tp_worker, after_cleanup);
                    }
                    break;
                }

                case DBENGINE_OPCODE_JOURNAL_INDEX: {
                    struct dbengine_tier *ctx = cmd.ctx;
                    struct dbengine_datafile *datafile = cmd.data;
                    // We no longer have an indexing command pending
                    ctx->datafiles.pending_index = false;
                    if (NOT_INDEXING_FILES(ctx) && ctx_is_available_for_queries(ctx)) {
                        __atomic_store_n(&ctx->atomic.migration_to_v2_running, true, __ATOMIC_RELAXED);
                        __atomic_store_n(&ctx->atomic.needs_indexing, false, __ATOMIC_RELAXED);
                        work_dispatch(engine, ctx, datafile, NULL, opcode, journal_v2_indexing_tp_worker, after_journal_v2_indexing);
                    }
                    break;
                }

                case DBENGINE_OPCODE_DATABASE_ROTATE: {
                    struct dbengine_tier *ctx = cmd.ctx;
                    ctx->datafiles.pending_rotate = false;
                    if (NOT_DELETING_FILES(ctx) && datafile_count(ctx, false) > 2 &&
                        dbengine_ctx_tier_cap_exceeded(ctx)) {
                        __atomic_store_n(&ctx->atomic.now_deleting_files, true, __ATOMIC_RELAXED);
                        work_dispatch(engine, ctx, NULL, NULL, opcode, database_rotate_tp_worker, after_database_rotate);
                    }
                    break;
                }

                case DBENGINE_OPCODE_CTX_POPULATE_MRG: {
                    struct dbengine_tier *ctx = cmd.ctx;
                    struct completion *completion = cmd.completion;
                    work_dispatch(engine, ctx, mlt, completion, opcode, populate_mrg_tp_worker, after_populate_mrg);
                    break;
                }

                case DBENGINE_OPCODE_CTX_FLUSH_DIRTY: {
                    struct dbengine_tier *ctx = cmd.ctx;
                    work_dispatch(engine, ctx, NULL, NULL, opcode,
                                  flush_dirty_pages_of_section_tp_worker,
                                  after_flush_dirty_pages_of_section);
                    break;
                }

                case DBENGINE_OPCODE_CTX_FLUSH_HOT_DIRTY: {
                    struct dbengine_tier *ctx = cmd.ctx;
                    work_dispatch(engine, ctx, NULL, NULL, opcode,
                                  flush_all_hot_and_dirty_pages_of_section_tp_worker,
                                  after_flush_all_hot_and_dirty_pages_of_section);
                    break;
                }

                case DBENGINE_OPCODE_CTX_QUIESCE: {
                    // a ctx will shutdown shortly
                    struct dbengine_tier *ctx = cmd.ctx;
                    nd_log_daemon(NDLP_INFO, "DBENGINE: Tier %d is shutting down — query processing disabled", ctx->config.tier);
                    __atomic_store_n(&ctx->quiesce.enabled, true, __ATOMIC_RELEASE);
                    break;
                }

                case DBENGINE_OPCODE_CTX_SHUTDOWN: {
                    // a ctx is shutting down
                    struct dbengine_tier *ctx = cmd.ctx;
                    struct completion *completion = cmd.completion;
                    work_dispatch(engine, ctx, NULL, completion, opcode, ctx_shutdown_tp_worker, after_ctx_shutdown);
                    break;
                }

                case DBENGINE_OPCODE_SHUTDOWN_EVLOOP: {
                    uv_close((uv_handle_t *)&engine->async, NULL);

                    (void) uv_timer_stop(&engine->timer);
                    uv_close((uv_handle_t *)&engine->timer, NULL);

                    (void) uv_timer_stop(&engine->retention_timer);
                    uv_close((uv_handle_t *)&engine->retention_timer, NULL);
                    shutdown = true;
                    break;
                }

                case DBENGINE_OPCODE_NOOP: {
                    /* the command queue was empty, do nothing */
                    break;
                }

                // not opcodes
                case DBENGINE_OPCODE_MAX:
                default: {
                    internal_fatal(true, "DBENGINE: unknown opcode");
                    break;
                }
            }
            if (opcode != DBENGINE_OPCODE_NOOP)
                uv_run(&engine->loop, UV_RUN_NOWAIT);

        } while (opcode != DBENGINE_OPCODE_NOOP);
    }
    freez(mlt);
    uv_sem_destroy(&sem);

    nd_log(NDLS_DAEMON, NDLP_DEBUG, "Shutting down dbengine thread");
    // what is left open (a handle still closing, a work request still on the thread pool) is finished by
    // dbengine_shutdown() once this thread was joined
    engine->loop_open = (0 != uv_loop_close(&engine->loop));
    worker_unregister();
}

void dbengine_shutdown(struct dbengine_engine *engine)
{
    if(!engine)
        return;

    // no tier may come up from here on: it would find no loop to serve it. A second call has nothing to stop,
    // and neither has a call on an engine that never spawned: its queue and thread do not exist.
    spinlock_lock(&engine->lifecycle.spinlock);
    bool nothing_to_stop = engine->lifecycle.stopped || !engine->lifecycle.spawned;
    engine->lifecycle.stopped = true;
    spinlock_unlock(&engine->lifecycle.spinlock);
    if(nothing_to_stop)
        return;

    // refuse embedder work next, so no request can be queued behind the loop's exit and left unanswered
    dbengine_cmd_queue_set_accepting(engine, false);
    dbengine_enq_cmd(engine, NULL, DBENGINE_OPCODE_SHUTDOWN_EVLOOP, NULL, NULL, STORAGE_PRIORITY_INTERNAL_DBENGINE, NULL, NULL);

    int rc = nd_thread_join(engine->thread);
    if (rc)
        nd_log_daemon(NDLP_ERR, "DBENGINE: Failed to join thread, error %s", uv_err_name(rc));
    else
        nd_log_daemon(NDLP_INFO, "DBENGINE: thread shutdown completed");

    // a loop its thread could not close on the way out still has a handle closing, or a work request on the thread
    // pool (a flush or a cleanup the last timer tick dispatched). Running it here finishes them while everything they
    // touch is still allocated: this returns with nothing of the engine executing any more, which is what
    // dbengine_destroy() relies on
    if(engine->loop_open) {
        uv_run(&engine->loop, UV_RUN_DEFAULT);
        fatal_assert(0 == uv_loop_close(&engine->loop));
        engine->loop_open = false;
    }
}

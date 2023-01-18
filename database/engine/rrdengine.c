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

struct rrdeng_main {
    uv_thread_t thread;
    uv_loop_t loop;
    uv_async_t async;
    uv_timer_t timer;
    pid_t tid;

    size_t flushes_running;
    size_t evictions_running;
} rrdeng_main = {
        .thread = 0,
        .loop = {},
        .async = {},
        .timer = {},
        .flushes_running = 0,
        .evictions_running = 0,
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
        size_t pending_cb;
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
    return __atomic_load_n(&work_request_globals.atomics.dispatched, __ATOMIC_RELAXED) >= (size_t)(libuv_worker_threads - RESERVED_LIBUV_WORKER_THREADS);
}

static void work_request_cleanup1(void) {
    struct rrdeng_work *item = NULL;

    if(!netdata_spinlock_trylock(&work_request_globals.protected.spinlock))
        return;

    if(work_request_globals.protected.available_items && work_request_globals.protected.available > (size_t)libuv_worker_threads) {
        item = work_request_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(work_request_globals.protected.available_items, item, cache.prev, cache.next);
        work_request_globals.protected.available--;
    }
    netdata_spinlock_unlock(&work_request_globals.protected.spinlock);

    if(item) {
        freez(item);
        __atomic_sub_fetch(&work_request_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
    }
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
    worker_is_busy(UV_EVENT_WORKER_INIT);

    struct rrdeng_work *work_request = req->data;
    work_request->work_cb(work_request->ctx, work_request->data, work_request->completion, req);
    worker_is_idle();

    __atomic_sub_fetch(&work_request_globals.atomics.dispatched, 1, __ATOMIC_RELAXED);
    __atomic_sub_fetch(&work_request_globals.atomics.executing, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&work_request_globals.atomics.pending_cb, 1, __ATOMIC_RELAXED);

    // signal the event loop a worker is available
    fatal_assert(0 == uv_async_send(&rrdeng_main.async));
}

void after_work_standard_callback(uv_work_t* req, int status) {
    struct rrdeng_work *work_request = req->data;

    worker_is_busy(RRDENG_OPCODE_MAX + work_request->opcode);

    if(work_request->after_work_cb)
        work_request->after_work_cb(work_request->ctx, work_request->data, work_request->completion, req, status);

    work_done(work_request);
    __atomic_sub_fetch(&work_request_globals.atomics.pending_cb, 1, __ATOMIC_RELAXED);

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

static void page_descriptor_cleanup1(void) {
    struct page_descr_with_data *item = NULL;

    if(!netdata_spinlock_trylock(&page_descriptor_globals.protected.spinlock))
        return;

    if(page_descriptor_globals.protected.available_items && page_descriptor_globals.protected.available > MAX_PAGES_PER_EXTENT) {
        item = page_descriptor_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(page_descriptor_globals.protected.available_items, item, cache.prev, cache.next);
        page_descriptor_globals.protected.available--;
    }

    netdata_spinlock_unlock(&page_descriptor_globals.protected.spinlock);

    if(item) {
        freez(item);
        __atomic_sub_fetch(&page_descriptor_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
    }
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

static void extent_io_descriptor_cleanup1(void) {
    struct extent_io_descriptor *item = NULL;

    if(!netdata_spinlock_trylock(&extent_io_descriptor_globals.protected.spinlock))
        return;

    if(extent_io_descriptor_globals.protected.available_items && extent_io_descriptor_globals.protected.available > (size_t)libuv_worker_threads) {
        item = extent_io_descriptor_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(extent_io_descriptor_globals.protected.available_items, item, cache.prev, cache.next);
        extent_io_descriptor_globals.protected.available--;
    }
    netdata_spinlock_unlock(&extent_io_descriptor_globals.protected.spinlock);

    if(item) {
        freez(item);
        __atomic_sub_fetch(&extent_io_descriptor_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
    }
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

static void rrdeng_query_handle_cleanup1(void) {
    struct rrdeng_query_handle *item = NULL;

    if(!netdata_spinlock_trylock(&rrdeng_query_handle_globals.protected.spinlock))
        return;

    if(rrdeng_query_handle_globals.protected.available_items && rrdeng_query_handle_globals.protected.available > 10) {
        item = rrdeng_query_handle_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(rrdeng_query_handle_globals.protected.available_items, item, cache.prev, cache.next);
        rrdeng_query_handle_globals.protected.available--;
    }

    netdata_spinlock_unlock(&rrdeng_query_handle_globals.protected.spinlock);

    if(item) {
        freez(item);
        __atomic_sub_fetch(&rrdeng_query_handle_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
    }
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
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(wal_globals.protected.available_items, wal, cache.prev, cache.next);
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
// command queue cache

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
    } cache;
};

static struct {
    struct {
        SPINLOCK spinlock;
        struct rrdeng_cmd *available_items;
        size_t available;

        struct {
            size_t allocated;
        } atomics;
    } cache;

    struct {
        SPINLOCK spinlock;
        size_t waiting;
        struct rrdeng_cmd *waiting_items_by_priority[STORAGE_PRIO_MAX_DONT_USE];
        size_t executed_by_priority[STORAGE_PRIO_MAX_DONT_USE];
    } queue;


} rrdeng_cmd_globals = {
        .cache = {
                .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                .available_items = NULL,
                .available = 0,
                .atomics = {
                        .allocated = 0,
                },
        },
        .queue = {
                .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                .waiting = 0,
        },
};

static void rrdeng_cmd_cleanup1(void) {
    struct rrdeng_cmd *item = NULL;

    if(!netdata_spinlock_trylock(&rrdeng_cmd_globals.cache.spinlock))
        return;

    if(rrdeng_cmd_globals.cache.available_items && rrdeng_cmd_globals.cache.available > 100) {
        item = rrdeng_cmd_globals.cache.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(rrdeng_cmd_globals.cache.available_items, item, cache.prev, cache.next);
        rrdeng_cmd_globals.cache.available--;
    }
    netdata_spinlock_unlock(&rrdeng_cmd_globals.cache.spinlock);

    if(item) {
        freez(item);
        __atomic_sub_fetch(&rrdeng_cmd_globals.cache.atomics.allocated, 1, __ATOMIC_RELAXED);
    }
}

void rrdeng_enqueue_epdl_cmd(struct rrdeng_cmd *cmd) {
    epdl_cmd_queued(cmd->data, cmd);
}

void rrdeng_dequeue_epdl_cmd(struct rrdeng_cmd *cmd) {
    epdl_cmd_dequeued(cmd->data);
}

void rrdeng_req_cmd(requeue_callback_t get_cmd_cb, void *data, STORAGE_PRIORITY priority) {
    netdata_spinlock_lock(&rrdeng_cmd_globals.queue.spinlock);

    struct rrdeng_cmd *cmd = get_cmd_cb(data);
    if(cmd && cmd->priority > priority) {
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(rrdeng_cmd_globals.queue.waiting_items_by_priority[cmd->priority], cmd, cache.prev, cache.next);
        DOUBLE_LINKED_LIST_APPEND_UNSAFE(rrdeng_cmd_globals.queue.waiting_items_by_priority[priority], cmd, cache.prev, cache.next);
        cmd->priority = priority;
    }

    netdata_spinlock_unlock(&rrdeng_cmd_globals.queue.spinlock);
}

void rrdeng_enq_cmd(struct rrdengine_instance *ctx, enum rrdeng_opcode opcode, void *data, struct completion *completion,
               enum storage_priority priority, enqueue_callback_t enqueue_cb, dequeue_callback_t dequeue_cb) {
    struct rrdeng_cmd *cmd = NULL;

    if(unlikely(priority >= STORAGE_PRIO_MAX_DONT_USE))
        priority = STORAGE_PRIORITY_NORMAL;

    netdata_spinlock_lock(&rrdeng_cmd_globals.cache.spinlock);
    if(likely(rrdeng_cmd_globals.cache.available_items)) {
        cmd = rrdeng_cmd_globals.cache.available_items;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(rrdeng_cmd_globals.cache.available_items, cmd, cache.prev, cache.next);
        rrdeng_cmd_globals.cache.available--;
    }
    netdata_spinlock_unlock(&rrdeng_cmd_globals.cache.spinlock);

    if(unlikely(!cmd)) {
        cmd = mallocz(sizeof(struct rrdeng_cmd));
        __atomic_add_fetch(&rrdeng_cmd_globals.cache.atomics.allocated, 1, __ATOMIC_RELAXED);
    }

    memset(cmd, 0, sizeof(struct rrdeng_cmd));
    cmd->ctx = ctx;
    cmd->opcode = opcode;
    cmd->data = data;
    cmd->completion = completion;
    cmd->priority = priority;
    cmd->dequeue_cb = dequeue_cb;

    netdata_spinlock_lock(&rrdeng_cmd_globals.queue.spinlock);
    DOUBLE_LINKED_LIST_APPEND_UNSAFE(rrdeng_cmd_globals.queue.waiting_items_by_priority[priority], cmd, cache.prev, cache.next);
    rrdeng_cmd_globals.queue.waiting++;
    if(enqueue_cb)
        enqueue_cb(cmd);
    netdata_spinlock_unlock(&rrdeng_cmd_globals.queue.spinlock);

    fatal_assert(0 == uv_async_send(&rrdeng_main.async));
}

static inline bool rrdeng_cmd_has_waiting_opcodes_in_lower_priorities(STORAGE_PRIORITY priority, STORAGE_PRIORITY max_priority) {
    for(; priority <= max_priority ; priority++)
        if(rrdeng_cmd_globals.queue.waiting_items_by_priority[priority])
            return true;

    return false;
}

static inline struct rrdeng_cmd rrdeng_deq_cmd(void) {
    struct rrdeng_cmd *cmd = NULL;

    STORAGE_PRIORITY max_priority = work_request_full() ? STORAGE_PRIORITY_CRITICAL : STORAGE_PRIORITY_BEST_EFFORT;

    // find an opcode to execute from the queue
    netdata_spinlock_lock(&rrdeng_cmd_globals.queue.spinlock);
    for(STORAGE_PRIORITY priority = STORAGE_PRIORITY_CRITICAL; priority <= max_priority ; priority++) {
        cmd = rrdeng_cmd_globals.queue.waiting_items_by_priority[priority];
        if(cmd) {

            // avoid starvation of lower priorities
            if(unlikely(priority > STORAGE_PRIORITY_CRITICAL &&
                        priority < STORAGE_PRIORITY_BEST_EFFORT &&
                        ++rrdeng_cmd_globals.queue.executed_by_priority[priority] % 50 == 0 &&
                        rrdeng_cmd_has_waiting_opcodes_in_lower_priorities(priority + 1, max_priority))) {
                // let the others run 2% of the requests
                cmd = NULL;
                continue;
            }

            // remove it from the queue
            DOUBLE_LINKED_LIST_REMOVE_UNSAFE(rrdeng_cmd_globals.queue.waiting_items_by_priority[priority], cmd, cache.prev, cache.next);
            rrdeng_cmd_globals.queue.waiting--;
            break;
        }
    }

    if(cmd && cmd->dequeue_cb) {
        cmd->dequeue_cb(cmd);
        cmd->dequeue_cb = NULL;
    }

    netdata_spinlock_unlock(&rrdeng_cmd_globals.queue.spinlock);

    struct rrdeng_cmd ret;
    if(cmd) {
        // copy it, to return it
        ret = *cmd;

        // put it in the cache
        netdata_spinlock_lock(&rrdeng_cmd_globals.cache.spinlock);
        DOUBLE_LINKED_LIST_APPEND_UNSAFE(rrdeng_cmd_globals.cache.available_items, cmd, cache.prev, cache.next);
        rrdeng_cmd_globals.cache.available++;
        netdata_spinlock_unlock(&rrdeng_cmd_globals.cache.spinlock);
    }
    else
        ret = (struct rrdeng_cmd) {
                .ctx = NULL,
                .opcode = RRDENG_OPCODE_NOOP,
                .priority = STORAGE_PRIORITY_BEST_EFFORT,
                .completion = NULL,
                .data = NULL,
        };

    return ret;
}


// ----------------------------------------------------------------------------

void *dbengine_page_alloc(size_t size) {
    void *page = mallocz(size);
    return page;
}

void dbengine_page_free(void *page, size_t size __maybe_unused) {
    freez(page);
}

void *dbengine_extent_alloc(size_t size) {
    void *extent = mallocz(size);
    return extent;
}

void dbengine_extent_free(void *extent, size_t size __maybe_unused) {
    freez(extent);
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
    if(completion)
        completion_mark_complete(completion);

    if(ctx_is_available_for_queries(ctx))
        rrdeng_enq_cmd(ctx, RRDENG_OPCODE_DATABASE_ROTATE, NULL, NULL, STORAGE_PRIORITY_CRITICAL, NULL, NULL);
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

    if(datafile->fileno != __atomic_load_n(&ctx->last_fileno, __ATOMIC_RELAXED) && still_running)
        // we just finished a flushing on a datafile that is not the active one
        rrdeng_enq_cmd(ctx, RRDENG_OPCODE_JOURNAL_FILE_INDEX, datafile, NULL, STORAGE_PRIORITY_CRITICAL, NULL, NULL);
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
    rrdeng_enq_cmd(xt_io_descr->ctx, RRDENG_OPCODE_FLUSHED_TO_OPEN, uv_fs_request, xt_io_descr->completion,
                   STORAGE_PRIORITY_CRITICAL, NULL, NULL);

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

        __atomic_sub_fetch(&ctx->worker_config.atomics.extents_currently_being_flushed, 1, __ATOMIC_RELAXED);
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

    // get the latest datafile
    uv_rwlock_rdlock(&ctx->datafiles.rwlock);
    datafile = ctx->datafiles.first->prev;
    netdata_spinlock_lock(&datafile->writers.spinlock);
    uv_rwlock_rdunlock(&ctx->datafiles.rwlock);

    if(ctx_is_available_for_queries(ctx) && datafile->pos > rrdeng_target_data_file_size(ctx)) {
        static SPINLOCK sp = NETDATA_SPINLOCK_INITIALIZER;
        netdata_spinlock_lock(&sp);
        if(create_new_datafile_pair(ctx) == 0)
            rrdeng_enq_cmd(ctx, RRDENG_OPCODE_JOURNAL_FILE_INDEX, datafile, NULL, STORAGE_PRIORITY_CRITICAL, NULL,
                           NULL);
        netdata_spinlock_unlock(&sp);

        // unlock the old datafile
        netdata_spinlock_unlock(&datafile->writers.spinlock);

        // get the new datafile
        uv_rwlock_rdlock(&ctx->datafiles.rwlock);
        datafile = ctx->datafiles.first->prev;
        netdata_spinlock_lock(&datafile->writers.spinlock);
        uv_rwlock_rdunlock(&ctx->datafiles.rwlock);
    }

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
    time_t first_time_s;
    time_t last_time_s;
    METRIC *metric;
};

static int journal_metric_uuid_compare(const void *key, const void *metric)
{
    return uuid_compare(*(uuid_t *) key, ((struct journal_metric_list *) metric)->uuid);
}

struct rrdengine_datafile *datafile_release_and_acquire_next_for_retention(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile) {

    uv_rwlock_rdlock(&ctx->datafiles.rwlock);

    struct rrdengine_datafile *next_datafile = datafile->next;

    while(next_datafile && !datafile_acquire(next_datafile, DATAFILE_ACQUIRE_RETENTION))
        next_datafile = next_datafile->next;

    uv_rwlock_rdunlock(&ctx->datafiles.rwlock);

    datafile_release(datafile, DATAFILE_ACQUIRE_RETENTION);

    return next_datafile;
}

void find_uuid_first_time(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile, Pvoid_t metric_first_time_JudyL) {
    // acquire the datafile to work with it
    uv_rwlock_rdlock(&ctx->datafiles.rwlock);
    while(datafile && !datafile_acquire(datafile, DATAFILE_ACQUIRE_RETENTION))
        datafile = datafile->next;
    uv_rwlock_rdunlock(&ctx->datafiles.rwlock);

    if (unlikely(!datafile))
        return;

    unsigned v2_count = 0;
    unsigned journalfile_count = 0;
    while (datafile) {
        struct journal_v2_header *j2_header = journalfile_v2_data_acquire(datafile->journalfile, NULL, 0, 0);
        if (!j2_header) {
            datafile = datafile_release_and_acquire_next_for_retention(ctx, datafile);
            continue;
        }

        time_t journal_start_time_s = (time_t) (j2_header->start_time_ut / USEC_PER_SEC);
        size_t journal_metric_count = (size_t)j2_header->metric_count;
        struct journal_metric_list *uuid_list = (struct journal_metric_list *)((uint8_t *) j2_header + j2_header->metric_offset);

        Word_t index = 0;
        bool first_then_next = true;
        Pvoid_t *PValue;
        while ((PValue = JudyLFirstThenNext(metric_first_time_JudyL, &index, &first_then_next))) {
            struct uuid_first_time_s *uuid_first_t_entry = *PValue;

            struct journal_metric_list *uuid_entry = bsearch(uuid_first_t_entry->uuid,uuid_list,journal_metric_count,sizeof(*uuid_list), journal_metric_uuid_compare);

            if (unlikely(!uuid_entry))
                continue;

            time_t first_time_s = uuid_entry->delta_start_s + journal_start_time_s;
            time_t last_time_s = uuid_entry->delta_end_s + journal_start_time_s;
            uuid_first_t_entry->first_time_s = MIN(uuid_first_t_entry->first_time_s , first_time_s);
            uuid_first_t_entry->last_time_s = MAX(uuid_first_t_entry->last_time_s , last_time_s);
            v2_count++;
        }
        journalfile_count++;
        journalfile_v2_data_release(datafile->journalfile);
        datafile = datafile_release_and_acquire_next_for_retention(ctx, datafile);
    }

    // Let's scan the open cache for almost exact match
    bool first_then_next = true;
    Pvoid_t *PValue;
    Word_t index = 0;
    unsigned open_cache_count = 0;
    while ((PValue = JudyLFirstThenNext(metric_first_time_JudyL, &index, &first_then_next))) {
        struct uuid_first_time_s *uuid_first_t_entry = *PValue;

        PGC_PAGE *page = pgc_page_get_and_acquire(
                open_cache, (Word_t)ctx,
                (Word_t)uuid_first_t_entry->metric, uuid_first_t_entry->last_time_s,
                PGC_SEARCH_CLOSEST);

        if (page) {
            time_t first_time_s = pgc_page_start_time_s(page);
            time_t last_time_s = pgc_page_end_time_s(page);
            uuid_first_t_entry->first_time_s = MIN(uuid_first_t_entry->first_time_s, first_time_s);
            uuid_first_t_entry->last_time_s = MAX(uuid_first_t_entry->last_time_s, last_time_s);
            pgc_page_release(open_cache, page);
            open_cache_count++;
        }
    }
    info("DBENGINE: processed %u journalfiles and matched %u metric pages in v2 files and %u in open cache", journalfile_count,
        v2_count, open_cache_count);
}

static void update_metrics_first_time_s(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile_to_delete, struct rrdengine_datafile *first_datafile_remaining, bool worker) {
    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.metrics_retention_started, 1, __ATOMIC_RELAXED);

    if(worker)
        worker_is_busy(UV_EVENT_ANALYZE_V2);

    struct rrdengine_journalfile *journalfile = datafile_to_delete->journalfile;
    struct journal_v2_header *j2_header = journalfile_v2_data_acquire(journalfile, NULL, 0, 0);
    struct journal_metric_list *uuid_list = (struct journal_metric_list *)((uint8_t *) j2_header + j2_header->metric_offset);

    Pvoid_t metric_first_time_JudyL = (Pvoid_t) NULL;
    Pvoid_t *PValue;

    unsigned count = 0;
    struct uuid_first_time_s *uuid_first_t_entry;
    for (uint32_t index = 0; index < j2_header->metric_count; ++index) {
        METRIC *metric = mrg_metric_get_and_acquire(main_mrg, &uuid_list[index].uuid, (Word_t) ctx);
        if (!metric)
            continue;

        PValue = JudyLIns(&metric_first_time_JudyL, (Word_t) index, PJE0);
        fatal_assert(NULL != PValue);
        if (!*PValue) {
            uuid_first_t_entry = mallocz(sizeof(*uuid_first_t_entry));
            uuid_first_t_entry->metric = metric;
            uuid_first_t_entry->first_time_s = mrg_metric_get_first_time_s(main_mrg, metric);
            uuid_first_t_entry->last_time_s = mrg_metric_get_latest_time_s(main_mrg, metric);
            uuid_first_t_entry->uuid = mrg_metric_uuid(main_mrg, metric);
            *PValue = uuid_first_t_entry;
            count++;
        }
    }
    journalfile_v2_data_release(journalfile);

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
        mrg_metric_set_first_time_s(main_mrg, uuid_first_t_entry->metric, uuid_first_t_entry->first_time_s);
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

    if (ctx_is_available_for_queries(ctx))
        update_metrics_first_time_s(ctx, datafile, datafile->next, worker);

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

            __atomic_add_fetch(&rrdeng_cache_efficiency_stats.datafile_deletion_spin, 1, __ATOMIC_RELAXED);
            sleep_usec(1 * USEC_PER_SEC);
        }
    }

    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.datafile_deletion_started, 1, __ATOMIC_RELAXED);
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
    datafile_list_delete_unsafe(ctx, datafile);
    uv_rwlock_wrunlock(&ctx->datafiles.rwlock);

    journal_file = datafile->journalfile;
    datafile_bytes = datafile->pos;
    journal_file_bytes = journal_file->pos;
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

    ctx->disk_space -= deleted_bytes;
    info("DBENGINE: reclaimed %u bytes of disk space.", deleted_bytes);

    if (rrdeng_ctx_exceeded_disk_quota(ctx))
        rrdeng_enq_cmd(ctx, RRDENG_OPCODE_DATABASE_ROTATE, NULL, NULL, STORAGE_PRIORITY_CRITICAL, NULL, NULL);

    rrdcontext_db_rotation();
}

static void database_rotate_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    datafile_delete(ctx, ctx->datafiles.first, true);
}

static void after_flush_all_hot_and_dirty_pages_of_section(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    ;
}

static void flush_all_hot_and_dirty_pages_of_section_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    pgc_flush_all_hot_and_dirty_pages(main_cache, (Word_t)ctx);
    completion_mark_complete(&ctx->quiesce_completion);
}

static void after_ctx_shutdown(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    ;
}

static void ctx_shutdown_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    completion_wait_for(&ctx->quiesce_completion);
    completion_destroy(&ctx->quiesce_completion);

    while(__atomic_load_n(&ctx->worker_config.atomics.extents_currently_being_flushed, __ATOMIC_RELAXED) ||
            __atomic_load_n(&ctx->inflight_queries, __ATOMIC_RELAXED))
        sleep_usec(1 * USEC_PER_MS);

    completion_mark_complete(completion);
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

bool rrdeng_ctx_exceeded_disk_quota(struct rrdengine_instance *ctx)
{
    uint64_t estimated_disk_space = ctx->disk_space + rrdeng_target_data_file_size(ctx) -
                                    (ctx->datafiles.first->prev ? ctx->datafiles.first->prev->pos : 0);

    return estimated_disk_space > ctx->max_disk_space;
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


static void extent_read_tp_worker(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t *uv_work_req __maybe_unused) {
    EPDL *epdl = data;
    epdl_find_extent_and_populate_pages(ctx, epdl, true);
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

        if (unlikely(!journalfile_v2_data_available(datafile->journalfile))) {
            info("DBENGINE: journal file %u is ready to be indexed", datafile->fileno);
            pgc_open_cache_to_journal_v2(open_cache, (Word_t) ctx, (int) datafile->fileno, ctx->page_type,
                                         journalfile_migrate_to_v2_callback, (void *) datafile->journalfile);
            count++;
        }

        datafile = datafile->next;

        if (unlikely(!ctx_is_available_for_queries(ctx)))
            break;
    }

    errno = 0;
    internal_error(count, "DBENGINE: journal indexing done; %u files processed", count);

    worker_is_idle();
}

static void after_do_cache_flush(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    rrdeng_main.flushes_running--;
}

static void after_do_cache_evict(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    rrdeng_main.evictions_running--;
}

static void after_extent_read(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    ;
}

static void after_journal_v2_indexing(struct rrdengine_instance *ctx __maybe_unused, void *data __maybe_unused, struct completion *completion __maybe_unused, uv_work_t* req __maybe_unused, int status __maybe_unused) {
    ctx->worker_config.migration_to_v2_running = false;
    rrdeng_enq_cmd(ctx, RRDENG_OPCODE_DATABASE_ROTATE, NULL, NULL, STORAGE_PRIORITY_CRITICAL, NULL, NULL);
}

struct rrdeng_buffer_sizes rrdeng_get_buffer_sizes(void) {
    return (struct rrdeng_buffer_sizes) {
            .opcodes     = __atomic_load_n(&rrdeng_cmd_globals.cache.atomics.allocated, __ATOMIC_RELAXED) * sizeof(struct rrdeng_cmd),
            .handles     = __atomic_load_n(&rrdeng_query_handle_globals.atomics.allocated, __ATOMIC_RELAXED) * sizeof(struct rrdeng_query_handle),
            .descriptors = __atomic_load_n(&page_descriptor_globals.atomics.allocated, __ATOMIC_RELAXED) * sizeof(struct page_descr_with_data),
            .wal         = __atomic_load_n(&wal_globals.atomics.allocated, __ATOMIC_RELAXED) * (sizeof(WAL) + RRDENG_BLOCK_SIZE),
            .workers     = __atomic_load_n(&work_request_globals.atomics.allocated, __ATOMIC_RELAXED) * sizeof(struct rrdeng_work),
            .pdc         = pdc_cache_size(),
            .xt_io       = __atomic_load_n(&extent_io_descriptor_globals.atomics.allocated, __ATOMIC_RELAXED) * sizeof(struct extent_io_descriptor),
            .xt_buf      = extent_buffer_cache_size(),
            .epdl        = epdl_cache_size(),
            .deol        = deol_cache_size(),
            .pd          = pd_cache_size(),
#ifdef PDC_USE_JULYL
            .julyl       = julyl_cache_size(),
#endif
    };
}

void timer_cb(uv_timer_t* handle) {
    worker_is_busy(RRDENG_TIMER_CB);
    uv_stop(handle->loop);
    uv_update_time(handle->loop);

    worker_set_metric(RRDENG_OPCODES_WAITING, (NETDATA_DOUBLE)rrdeng_cmd_globals.queue.waiting);
    worker_set_metric(RRDENG_WORKS_DISPATCHED, (NETDATA_DOUBLE)__atomic_load_n(&work_request_globals.atomics.dispatched, __ATOMIC_RELAXED));
    worker_set_metric(RRDENG_WORKS_EXECUTING, (NETDATA_DOUBLE)__atomic_load_n(&work_request_globals.atomics.executing, __ATOMIC_RELAXED));

    rrdeng_enq_cmd(NULL, RRDENG_OPCODE_FLUSH_INIT, NULL, NULL, STORAGE_PRIORITY_CRITICAL, NULL, NULL);
    rrdeng_enq_cmd(NULL, RRDENG_OPCODE_EVICT_INIT, NULL, NULL, STORAGE_PRIORITY_CRITICAL, NULL, NULL);

    rrdeng_cmd_cleanup1();
    work_request_cleanup1();
    page_descriptor_cleanup1();
    extent_io_descriptor_cleanup1();
    pdc_cleanup1();
    page_details_cleanup1();
    rrdeng_query_handle_cleanup1();
    wal_cleanup1();
    extent_buffer_cleanup1();
    epdl_cleanup1();
    deol_cleanup1();

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

        fatal_assert(0 == uv_thread_create(&rrdeng_main.thread, dbengine_event_loop, &rrdeng_main));
        spawned = true;
    }

    ctx->worker_config.now_deleting_files = false;
    ctx->worker_config.migration_to_v2_running = false;
    ctx->worker_config.atomics.extents_currently_being_flushed = 0;

    return true;
}

void dbengine_event_loop(void* arg) {
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

    worker_register_job_name(RRDENG_OPCODE_MAX,                                      "get opcode");

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
            worker_is_busy(RRDENG_OPCODE_MAX);
            cmd = rrdeng_deq_cmd();
            opcode = cmd.opcode;

            worker_is_busy(opcode);

            switch (opcode) {
                case RRDENG_OPCODE_EXTENT_READ: {
                    struct rrdengine_instance *ctx = cmd.ctx;
                    EPDL *epdl = cmd.data;
                    work_dispatch(ctx, epdl, NULL, opcode, extent_read_tp_worker, after_extent_read);
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
                    // for the datafile and the journalfile
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
                         rrdeng_ctx_exceeded_disk_quota(ctx)) {

                        ctx->worker_config.now_deleting_files = true;
                        if(!work_dispatch(ctx, NULL, NULL, opcode, database_rotate_tp_worker, after_database_rotate))
                            ctx->worker_config.now_deleting_files = false;

                    }
                    break;
                }

                case RRDENG_OPCODE_CTX_QUIESCE: {
                    // a ctx will shutdown shortly
                    struct rrdengine_instance *ctx = cmd.ctx;
                    __atomic_store_n(&ctx->quiesce, SET_QUIESCE, __ATOMIC_RELEASE);
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

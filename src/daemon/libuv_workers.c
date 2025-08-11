// SPDX-License-Identifier: GPL-3.0-or-later

#include <daemon/main.h>
#include "libuv_workers.h"

static void register_libuv_worker_jobs_internal(void) {
    signals_block_all_except_deadly();

    worker_register("LIBUV");

    // generic
    worker_register_job_name(UV_EVENT_WORKER_INIT, "worker init");

    // query related
    worker_register_job_name(UV_EVENT_DBENGINE_QUERY, "query");
    worker_register_job_name(UV_EVENT_DBENGINE_EXTENT_CACHE_LOOKUP, "extent cache");
    worker_register_job_name(UV_EVENT_DBENGINE_EXTENT_MMAP, "extent mmap");
    worker_register_job_name(UV_EVENT_DBENGINE_EXTENT_DECOMPRESSION, "extent decompression");
    worker_register_job_name(UV_EVENT_DBENGINE_EXTENT_PAGE_LOOKUP, "page lookup");
    worker_register_job_name(UV_EVENT_DBENGINE_EXTENT_PAGE_POPULATION, "page populate");
    worker_register_job_name(UV_EVENT_DBENGINE_EXTENT_PAGE_ALLOCATION, "page allocate");

    // flushing related
    worker_register_job_name(UV_EVENT_DBENGINE_FLUSH_MAIN_CACHE, "flush main");
    worker_register_job_name(UV_EVENT_DBENGINE_EXTENT_WRITE, "extent write");
    worker_register_job_name(UV_EVENT_DBENGINE_FLUSHED_TO_OPEN, "flushed to open");

    // datafile full
    worker_register_job_name(UV_EVENT_DBENGINE_JOURNAL_INDEX, "jv2 indexing");

    // db rotation related
    worker_register_job_name(UV_EVENT_DBENGINE_DATAFILE_DELETE_WAIT, "datafile delete wait");
    worker_register_job_name(UV_EVENT_DBENGINE_DATAFILE_DELETE, "datafile deletion");
    worker_register_job_name(UV_EVENT_DBENGINE_FIND_ROTATED_METRICS, "find rotated metrics");
    worker_register_job_name(UV_EVENT_DBENGINE_FIND_REMAINING_RETENTION, "find remaining retention");
    worker_register_job_name(UV_EVENT_DBENGINE_POPULATE_MRG, "update retention");

    // other dbengine events
    worker_register_job_name(UV_EVENT_DBENGINE_EVICT_MAIN_CACHE, "evict main");
    worker_register_job_name(UV_EVENT_DBENGINE_EVICT_OPEN_CACHE, "evict open");
    worker_register_job_name(UV_EVENT_DBENGINE_EVICT_EXTENT_CACHE, "evict extent");
    worker_register_job_name(UV_EVENT_DBENGINE_BUFFERS_CLEANUP, "dbengine buffers cleanup");
    worker_register_job_name(UV_EVENT_DBENGINE_FLUSH_DIRTY, "dbengine flush dirty");
    worker_register_job_name(UV_EVENT_DBENGINE_QUIESCE, "dbengine quiesce");
    worker_register_job_name(UV_EVENT_DBENGINE_SHUTDOWN, "dbengine shutdown");

    // metadata
    worker_register_job_name(UV_EVENT_HOST_CONTEXT_LOAD, "metadata load host context");
    worker_register_job_name(UV_EVENT_METADATA_STORE, "metadata store host");
    worker_register_job_name(UV_EVENT_METADATA_CLEANUP, "metadata cleanup");
    worker_register_job_name(UV_EVENT_METADATA_ML_LOAD, "metadata load ml models");
    worker_register_job_name(UV_EVENT_CTX_CLEANUP_SCHEDULE, "metadata ctx cleanup schedule");
    worker_register_job_name(UV_EVENT_CTX_CLEANUP, "metadata ctx cleanup");
    worker_register_job_name(UV_EVENT_STORE_ALERT_TRANSITIONS, "metadata store alert transitions");
    worker_register_job_name(UV_EVENT_STORE_SQL_STATEMENTS, "metadata store sql statements");
    worker_register_job_name(UV_EVENT_CHART_LABEL_CLEANUP, "metadata chart label cleanup");
    worker_register_job_name(UV_EVENT_HEALTH_LOG_CLEANUP, "alert transitions cleanup");
    worker_register_job_name(UV_EVENT_UUID_DELETION, "metadata dimension deletion");
    worker_register_job_name(UV_EVENT_DIMENSION_CLEANUP, "metadata dimension cleanup");
    worker_register_job_name(UV_EVENT_CHART_CLEANUP, "metadata chart cleanup");
    worker_register_job_name(UV_EVENT_STORE_HOST, "metadata store host");
    worker_register_job_name(UV_EVENT_STORE_CHART, "metadata store chart");
    worker_register_job_name(UV_EVENT_STORE_DIMENSION, "metadata store dimension");

    // aclk_sync
    worker_register_job_name(UV_EVENT_ACLK_NODE_INFO, "aclk host node info");
    worker_register_job_name(UV_EVENT_ACLK_ALERT_PUSH, "aclk alert push");
    worker_register_job_name(UV_EVENT_ACLK_QUERY_EXECUTE, "aclk query execute");
    // aclk
    worker_register_job_name(UV_EVENT_CTX_STOP_STREAMING, "ctx stop streaming");
    worker_register_job_name(UV_EVENT_CTX_CHECKPOINT, "ctx version check");
    worker_register_job_name(UV_EVENT_ALARM_PROVIDE_CFG, "send alarm config");
    worker_register_job_name(UV_EVENT_ALARM_SNAPSHOT, "alert snapshot");
    worker_register_job_name(UV_EVENT_REGISTER_NODE, "register node");
    worker_register_job_name(UV_EVENT_UPDATE_NODE_COLLECTORS, "update collectors");
    worker_register_job_name(UV_EVENT_UPDATE_NODE_INFO, "send node info");
    worker_register_job_name(UV_EVENT_CTX_SEND_SNAPSHOT, "ctx send snapshot");
    worker_register_job_name(UV_EVENT_CTX_SEND_SNAPSHOT_UPD, "ctx send update");
    worker_register_job_name(UV_EVENT_NODE_STATE_UPDATE, "node state update");
    worker_register_job_name(UV_EVENT_SEND_NODE_INSTANCES, "send node instances");
    worker_register_job_name(UV_EVENT_ALERT_START_STREAMING, "alert start streaming");
    worker_register_job_name(UV_EVENT_ALERT_CHECKPOINT, "alert checkpoint");
    worker_register_job_name(UV_EVENT_CREATE_NODE_INSTANCE, "create node instance");
    worker_register_job_name(UV_EVENT_UNREGISTER_NODE, "unregister node locally");

    // netdatacli
    worker_register_job_name(UV_EVENT_SCHEDULE_CMD, "schedule command");

    // make sure we have the right thread id
    gettid_uncached();

    static int workers = 0;
    int worker_id = __atomic_add_fetch(&workers, 1, __ATOMIC_RELAXED);

    char buf[NETDATA_THREAD_TAG_MAX + 1];
    snprintfz(buf, NETDATA_THREAD_TAG_MAX, "UV_WORKER[%d]", worker_id);
    uv_thread_set_name_np(buf);
}

// Register workers
ALWAYS_INLINE
void register_libuv_worker_jobs() {
    static __thread bool registered = false;

    if(likely(registered))
        return;

    registered = true;
    register_libuv_worker_jobs_internal();
}

void libuv_close_callback(uv_handle_t *handle, void *data __maybe_unused)
{
    // Only close handles that aren't already closing
    if (!uv_is_closing(handle)) {
        if (handle->type == UV_TIMER) {
            uv_timer_stop((uv_timer_t *)handle);
        }
        uv_close(handle, NULL);
    }
}

// Initialize the worker pool
void init_worker_pool(WorkerPool *pool) {
    for (int i = 0; i < MAX_ACTIVE_WORKERS; i++) {
        pool->workers[i].allocated = false;
        pool->free_stack[i] = i;  // Fill the stack with indices
    }
    pool->top = MAX_ACTIVE_WORKERS;  // All workers are initially free
}

// Get a worker from the pool
// Needs to be called from the uv event loop thread
worker_data_t *get_worker(WorkerPool *pool) {
    worker_data_t *worker;
    if (pool->top == 0) {
        worker = callocz(1, sizeof(worker_data_t));
        worker->allocated = true;  // Mark as allocated
    } else {
        int index = pool->free_stack[--pool->top]; // Pop from stack
        worker = &pool->workers[index];
    }
    worker->request.data = worker;
    return worker;
}

// Return a worker for reuse
void return_worker(WorkerPool *pool, worker_data_t *worker) {
    if (unlikely(worker->allocated)) {
        freez(worker);
        return;
    }

    int index = (int) (worker - pool->workers);
    if (index < 0 || index >= MAX_ACTIVE_WORKERS) {
        return;  // Invalid worker (should not happen)
    }
    pool->free_stack[pool->top++] = index;  // Push index back to stack
}

// Initialize the command pool
void init_cmd_pool(CmdPool *pool, int size) {
    pool->buffer = mallocz(sizeof(cmd_data_t) * size);

    pool->size = size;
    pool->head = 0;
    pool->tail = 0;
    pool->count = 0;

    fatal_assert(0 == uv_mutex_init(&pool->lock));
    fatal_assert(0 == uv_cond_init(&pool->not_full));
}

bool push_cmd(CmdPool *pool, const cmd_data_t *cmd, bool wait_on_full)
{
    uv_mutex_lock(&pool->lock);

    while (pool->count == pool->size) {
        if (wait_on_full)
            uv_cond_wait(&pool->not_full, &pool->lock);
        else {
            uv_mutex_unlock(&pool->lock); // No space, return
            return false;
        }
    }

    pool->buffer[pool->tail] = *cmd;
    pool->tail = (pool->tail + 1) % pool->size;
    pool->count++;

    uv_mutex_unlock(&pool->lock);
    return true;
}

bool pop_cmd(CmdPool *pool, cmd_data_t *out_cmd) {
    uv_mutex_lock(&pool->lock);
    if (pool->count == 0) {
        uv_mutex_unlock(&pool->lock); // No commands to pop
        return false;
    }
    *out_cmd = pool->buffer[pool->head];
    pool->head = (pool->head + 1) % pool->size;
    pool->count--;

    uv_cond_signal(&pool->not_full);
    uv_mutex_unlock(&pool->lock);
    return true;
}

void release_cmd_pool(CmdPool *pool) {
    if (pool->buffer) {
        free(pool->buffer);
        pool->buffer = NULL;
    }
    uv_mutex_destroy(&pool->lock);
    uv_cond_destroy(&pool->not_full);
}

/// Test

typedef struct {
    CmdPool *pool;
    int total;
    int failed;
} ThreadArgs;

void push_thread(void *arg) {
    ThreadArgs *args = (ThreadArgs *)arg;
    CmdPool *pool = args->pool;
    for (int i = 0; i < args->total; ++i) {
        cmd_data_t cmd = { 0 };
        snprintf(cmd.data, sizeof(cmd.data), "cmd-%d", i);
        push_cmd(pool, &cmd, true);
    }
    fprintf(stderr, "PUSHED: %d commands\n", args->total);
}

void pop_thread(void *arg) {
    ThreadArgs *args = (ThreadArgs *)arg;
    CmdPool *pool = args->pool;

    cmd_data_t cmd;
    for (int i = 0; i < args->total; ) {
        bool got = pop_cmd(pool, &cmd);
        if (got) {
            char expected[64];
            snprintf(expected, sizeof(expected), "cmd-%d", i);
            if (strcmp(cmd.data, expected) != 0) {
                fprintf(stderr, "POPPED: %s --- EXPECTED %s FAILED\n", cmd.data, expected);
                args->failed++;
            }
            i++;
        } else {
            uv_sleep(1);  // avoid busy spin
        }
    }
    fprintf(stderr, "POPPED: %d commands\n", args->total);
}

int test_cmd_pool_fifo()
{
    CmdPool pool;

    int pool_sizes[] = {32, 64, 128, 256};

    for (size_t i = 0; i < sizeof(pool_sizes) / sizeof(pool_sizes[0]); ++i) {
        int pool_size = pool_sizes[i];
        init_cmd_pool(&pool, pool_size);

        ThreadArgs args = {.pool = &pool, .total = 1000, .failed = 0};
        uv_thread_t producer, consumer;
        fprintf(stderr, "Testing pool size %d\n", pool_size);

        uv_thread_create(&producer, push_thread, &args);
        uv_thread_create(&consumer, pop_thread, &args);

        uv_thread_join(&producer);
        uv_thread_join(&consumer);

        release_cmd_pool(&pool);
        if (args.failed) {
            fprintf(stderr, "Multithreaded FIFO test failed with %d errors.\n", args.failed);
            return 1;
        }
    }
    fprintf(stderr, "Multithreaded FIFO test passed.\n");
    return 0;
}

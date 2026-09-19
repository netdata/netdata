// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EVENT_LOOP_H
#define NETDATA_EVENT_LOOP_H

#include "libnetdata/libnetdata.h"

// job ids are indices into per-thread worker utilization tables that every user of the libuv threadpool
// shares; the storage engine owns the first block, the daemon's own jobs follow it
#ifdef ENABLE_DBENGINE
#include "database/storage-engines/dbengine/include/dbengine/dbengine-workers.h"
#define UV_EVENT_JOB_FIRST DBENGINE_WORKER_JOB_MAX
#else
#define UV_EVENT_JOB_FIRST 1
#endif

enum event_loop_job {
    UV_EVENT_JOB_NONE = 0,

    // Metrics calculation
    UV_EVENT_WEIGHTS_CALCULATION = UV_EVENT_JOB_FIRST,

    // metadata
    UV_EVENT_HOST_CONTEXT_LOAD,
    UV_EVENT_METADATA_STORE,
    UV_EVENT_METADATA_CLEANUP,
    UV_EVENT_METADATA_ML_LOAD,
    UV_EVENT_CTX_CLEANUP_SCHEDULE,
    UV_EVENT_CTX_CLEANUP,
    UV_EVENT_STORE_HOST,
    UV_EVENT_STORE_CHART,
    UV_EVENT_STORE_DIMENSION,
    UV_EVENT_STORE_ALERT_TRANSITIONS,
    UV_EVENT_STORE_SQL_STATEMENTS,
    UV_EVENT_HEALTH_LOG_CLEANUP,
    UV_EVENT_CHART_LABEL_CLEANUP,
    UV_EVENT_UUID_DELETION,
    UV_EVENT_DIMENSION_CLEANUP,
    UV_EVENT_CHART_CLEANUP,

    // aclk_sync
    UV_EVENT_ACLK_NODE_INFO,
    UV_EVENT_ACLK_ALERT_PUSH,
    UV_EVENT_ACLK_QUERY_EXECUTE,

    //
    UV_EVENT_CTX_STOP_STREAMING,
    UV_EVENT_CTX_CHECKPOINT,
    UV_EVENT_ALARM_PROVIDE_CFG,
    UV_EVENT_ALARM_SNAPSHOT,
    UV_EVENT_REGISTER_NODE,
    UV_EVENT_UPDATE_NODE_COLLECTORS,
    UV_EVENT_UPDATE_NODE_INFO,
    UV_EVENT_UPDATE_NODE_MANIFEST,
    UV_EVENT_CTX_SEND_SNAPSHOT,
    UV_EVENT_CTX_SEND_SNAPSHOT_UPD,
    UV_EVENT_NODE_STATE_UPDATE,
    UV_EVENT_SEND_NODE_INSTANCES,
    UV_EVENT_ALERT_START_STREAMING,
    UV_EVENT_ALERT_CHECKPOINT,
    UV_EVENT_CREATE_NODE_INSTANCE,
    UV_EVENT_UNREGISTER_NODE,

    // maintenance
    UV_EVENT_CLEANUP_OBSOLETE_CHARTS,
    UV_EVENT_ARCHIVE_CHART_DIMENSIONS,
    UV_EVENT_ARCHIVE_DIMENSION,
    UV_EVENT_CLEANUP_ORPHAN_HOSTS,
    UV_EVENT_CLEANUP_OBSOLETE_CHARTS_ON_HOSTS,
    UV_EVENT_FREE_HOST,
    UV_EVENT_FREE_CHART,
    UV_EVENT_FREE_DIMENSION,

    // netdatacli
    UV_EVENT_SCHEDULE_CMD,

    // terminator
    UV_EVENT_JOB_MAX,
};

// the engine's block and the daemon's block share one per-thread table; an id added to either shifts the daemon's
static_assert(UV_EVENT_JOB_MAX <= WORKER_UTILIZATION_MAX_JOB_TYPES, "libuv worker job ids exceed the worker utilization table");

#define MAX_ACTIVE_WORKERS (256)

typedef struct worker_data {
    uv_work_t request;
    void *config;
    void *pending_alert_list;
    void *pending_ctx_cleanup_list;
    void *pending_uuid_deletion;
    void *pending_sql_statement;
    union {
        void *payload;
        void *work_buffer;
    };
    bool allocated;
} worker_data_t;

typedef struct {
    worker_data_t workers[MAX_ACTIVE_WORKERS];  // Preallocated worker data pool
    int free_stack[MAX_ACTIVE_WORKERS];  // Stack of available worker data indices
    int top;  // Stack pointer
} WorkerPool;

typedef struct {
    uint8_t opcode;
    uint8_t padding[sizeof(void *) - sizeof(uint8_t)]; // Padding to align the union
    union {
        void *param[2];
        char data[sizeof(void *) * 2];
    };
} cmd_data_t;

typedef struct {
    bool closed;                // set by close_cmd_pool(): refuse pushes, wake anyone waiting
    int producers;              // producers currently inside the pool; close_cmd_pool() waits for 0
    cmd_data_t *buffer;
    int size;
    int head;
    int tail;
    int count;

    netdata_mutex_t lock;
    netdata_cond_t not_full;
    netdata_cond_t no_producers;   // signalled when the last producer leaves a closed pool
} CmdPool;


void register_libuv_worker_jobs();
void libuv_close_callback(uv_handle_t *handle, void *data __maybe_unused);

void init_worker_pool(WorkerPool *pool);
worker_data_t *get_worker(WorkerPool *pool);
void return_worker(WorkerPool *pool, worker_data_t *worker);

void init_cmd_pool(CmdPool *pool, int size);

// Stops the pool accepting commands, atomically with respect to push_cmd(): after this returns, no
// push can succeed and none is still waiting for space. A consumer that is shutting down calls this
// before draining, so a producer either got its command in - and the consumer will see it - or was
// refused and knows not to wait for it.
void close_cmd_pool(CmdPool *pool);

// Extends a producer's "inside the pool" window past push_cmd(), for a caller that must finish
// something else before the owner may tear the pool down - notifying the consumer, typically.
// enter() fails if the pool is already closed; on success the caller MUST pair it with leave().
bool cmd_pool_producer_enter(CmdPool *pool);
void cmd_pool_producer_leave(CmdPool *pool);

bool push_cmd(CmdPool *pool, const cmd_data_t *cmd, bool wait_on_full);
bool pop_cmd(CmdPool *pool, cmd_data_t *out_cmd);
void release_cmd_pool(CmdPool *pool);

// Destroys the pool's lock and conditions. release_cmd_pool() deliberately does not, because it
// cannot rule out a producer that is still blocked acquiring the lock. Call this ONLY when that is
// provable - every producer thread joined - and only if the pool's storage is to be reused.
void destroy_cmd_pool(CmdPool *pool);
int test_cmd_pool_fifo();

// size of the libuv worker thread pool, resolved from netdata.conf at startup
#if defined(ENV32BIT)
#define MIN_LIBUV_WORKER_THREADS 8
#define MAX_LIBUV_WORKER_THREADS 128
#define RESERVED_LIBUV_WORKER_THREADS 3
#else
#define MIN_LIBUV_WORKER_THREADS 16
#define MAX_LIBUV_WORKER_THREADS 1024
#define RESERVED_LIBUV_WORKER_THREADS 6
#endif

extern int libuv_worker_threads;

#endif //NETDATA_EVENT_LOOP_H

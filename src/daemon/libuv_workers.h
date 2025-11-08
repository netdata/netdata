// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EVENT_LOOP_H
#define NETDATA_EVENT_LOOP_H

enum event_loop_job {
    UV_EVENT_JOB_NONE = 0,

    // generic
    UV_EVENT_WORKER_INIT,

    // query related
    UV_EVENT_DBENGINE_QUERY,
    UV_EVENT_DBENGINE_EXTENT_CACHE_LOOKUP,
    UV_EVENT_DBENGINE_EXTENT_MMAP,
    UV_EVENT_DBENGINE_EXTENT_DECOMPRESSION,
    UV_EVENT_DBENGINE_EXTENT_PAGE_LOOKUP,
    UV_EVENT_DBENGINE_EXTENT_PAGE_POPULATION,
    UV_EVENT_DBENGINE_EXTENT_PAGE_ALLOCATION,
    // Metrics calculation
    UV_EVENT_WEIGHTS_CALCULATION,

    // flushing related
    UV_EVENT_DBENGINE_FLUSH_MAIN_CACHE,
    UV_EVENT_DBENGINE_EXTENT_WRITE,
    UV_EVENT_DBENGINE_FLUSHED_TO_OPEN,

    // datafile full
    UV_EVENT_DBENGINE_JOURNAL_INDEX,

    // db rotation related
    UV_EVENT_DBENGINE_DATAFILE_DELETE_WAIT,
    UV_EVENT_DBENGINE_DATAFILE_DELETE,
    UV_EVENT_DBENGINE_FIND_ROTATED_METRICS, // find the metrics that are rotated
    UV_EVENT_DBENGINE_FIND_REMAINING_RETENTION, // find their remaining retention
    UV_EVENT_DBENGINE_POPULATE_MRG, // update mrg

    // other dbengine events
    UV_EVENT_DBENGINE_EVICT_MAIN_CACHE,
    UV_EVENT_DBENGINE_EVICT_OPEN_CACHE,
    UV_EVENT_DBENGINE_EVICT_EXTENT_CACHE,
    UV_EVENT_DBENGINE_BUFFERS_CLEANUP,
    UV_EVENT_DBENGINE_FLUSH_DIRTY,
    UV_EVENT_DBENGINE_QUIESCE,
    UV_EVENT_DBENGINE_MRG_LOAD,
    UV_EVENT_DBENGINE_SHUTDOWN,

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
};

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
    cmd_data_t *buffer;
    int size;
    int head;
    int tail;
    int count;

    netdata_mutex_t lock;
    netdata_cond_t not_full;
} CmdPool;


void register_libuv_worker_jobs();
void libuv_close_callback(uv_handle_t *handle, void *data __maybe_unused);

void init_worker_pool(WorkerPool *pool);
worker_data_t *get_worker(WorkerPool *pool);
void return_worker(WorkerPool *pool, worker_data_t *worker);

void init_cmd_pool(CmdPool *pool, int size);
bool push_cmd(CmdPool *pool, const cmd_data_t *cmd, bool wait_on_full);
bool pop_cmd(CmdPool *pool, cmd_data_t *out_cmd);
void release_cmd_pool(CmdPool *pool);
int test_cmd_pool_fifo();

#endif //NETDATA_EVENT_LOOP_H

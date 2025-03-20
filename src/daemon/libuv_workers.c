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
    worker_register_job_name(UV_EVENT_DBENGINE_JOURNAL_INDEX_WAIT, "jv2 index wait");
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

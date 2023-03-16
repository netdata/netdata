// SPDX-License-Identifier: GPL-3.0-or-later

#include <daemon/main.h>
#include "event_loop.h"

// Register workers
void register_libuv_worker_jobs() {
    static __thread bool registered = false;

    if(likely(registered))
        return;

    registered = true;

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
    worker_register_job_name(UV_EVENT_DBENGINE_BUFFERS_CLEANUP, "dbengine buffers cleanup");
    worker_register_job_name(UV_EVENT_DBENGINE_QUIESCE, "dbengine quiesce");
    worker_register_job_name(UV_EVENT_DBENGINE_SHUTDOWN, "dbengine shutdown");

    // metadata
    worker_register_job_name(UV_EVENT_HOST_CONTEXT_LOAD, "metadata load host context");
    worker_register_job_name(UV_EVENT_METADATA_STORE, "metadata store host");
    worker_register_job_name(UV_EVENT_METADATA_CLEANUP, "metadata cleanup");

    // netdatacli
    worker_register_job_name(UV_EVENT_SCHEDULE_CMD, "schedule command");

    uv_thread_set_name_np(pthread_self(), "LIBUV_WORKER");
}

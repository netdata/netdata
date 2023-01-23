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
    worker_register_job_name(UV_EVENT_PAGE_LOOKUP, "page lookup");
    worker_register_job_name(UV_EVENT_PAGE_POPULATION, "populate page");
    worker_register_job_name(UV_EVENT_EXT_DECOMPRESSION, "extent decompression");
    worker_register_job_name(UV_EVENT_METADATA_STORE, "store host metadata");
    worker_register_job_name(UV_EVENT_JOURNAL_INDEX_WAIT, "journal v2 wait");
    worker_register_job_name(UV_EVENT_JOURNAL_INDEX, "journal v2 indexing");
    worker_register_job_name(UV_EVENT_SCHEDULE_CMD, "schedule command");
    worker_register_job_name(UV_EVENT_METADATA_CLEANUP, "metadata cleanup");
    worker_register_job_name(UV_EVENT_EXTENT_CACHE, "extent cache");
    worker_register_job_name(UV_EVENT_EXTENT_MMAP, "extent mmap");
    worker_register_job_name(UV_EVENT_FLUSH_MAIN, "flush main");
    worker_register_job_name(UV_EVENT_EVICT_MAIN, "evict main");
    worker_register_job_name(UV_EVENT_ANALYZE_V2, "analyze journalfile");
    worker_register_job_name(UV_EVENT_RETENTION_V2, "calculate retention");
    worker_register_job_name(UV_EVENT_RETENTION_UPDATE, "update retention");
    worker_register_job_name(UV_EVENT_DATAFILE_ACQUIRE, "datafile acquire");
    worker_register_job_name(UV_EVENT_DATAFILE_DELETE, "datafile deletion");
    worker_register_job_name(UV_EVENT_FLUSHED_TO_OPEN, "flushed to open");
    worker_register_job_name(UV_EVENT_PREP_QUERY, "prep query");
    worker_register_job_name(UV_EVENT_WORKER_INIT, "worker init");
    worker_register_job_name(UV_EVENT_FLUSH_PAGES, "flush pages");
    worker_register_job_name(UV_EVENT_BUFFERS_CLEANUP, "buffers cleanup");
    worker_register_job_name(UV_EVENT_QUIESCE, "quiesce");
    worker_register_job_name(UV_EVENT_POPULATE_MRG, "populate mrg");
    worker_register_job_name(UV_EVENT_SHUTDOWN, "shutdown");

    uv_thread_set_name_np(pthread_self(), "LIBUV_WORKER");
}

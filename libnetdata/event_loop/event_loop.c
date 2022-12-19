// SPDX-License-Identifier: GPL-3.0-or-later

#include <daemon/main.h>
#include "event_loop.h"

// Register workers
void register_libuv_worker_jobs()
{
    worker_register("LIBUV");
    worker_register_job_name(UV_EVENT_READ_PAGE_CB, "read page cb");
    worker_register_job_name(UV_EVENT_READ_EXTENT_CB, "read extent cb");
    worker_register_job_name(UV_EVENT_COMMIT_PAGE_CB, "commit cb");
    worker_register_job_name(UV_EVENT_FLUSH_PAGES_CB, "flush cb");
    worker_register_job_name(UV_EVENT_PAGE_LOOKUP, "page lookup");
    worker_register_job_name(UV_EVENT_METRIC_LOOKUP, "metric lookup");
    worker_register_job_name(UV_EVENT_PAGE_POPULATION, "populate page");
    worker_register_job_name(UV_EVENT_EXT_DECOMPRESSION, "extent decompression");
    worker_register_job_name(UV_EVENT_READ_MMAP_EXTENT, "read extent (mmap)");
    worker_register_job_name(UV_EVENT_EXTENT_PROCESSING, "extent processing");
    worker_register_job_name(UV_EVENT_METADATA_STORE, "store host metadata");
    worker_register_job_name(UV_EVENT_JOURNAL_INDEX_WAIT, "journal v2 wait");
    worker_register_job_name(UV_EVENT_JOURNAL_INDEX, "journal v2 indexing");
    worker_register_job_name(UV_EVENT_SCHEDULE_CMD, "schedule command");
    worker_register_job_name(UV_EVENT_METADATA_CLEANUP, "metadata cleanup");
    worker_register_job_name(UV_EVENT_EXTENT_CACHE, "extent cache");
    worker_register_job_name(UV_EVENT_EXTENT_MMAP, "extent mmap");
    worker_register_job_name(UV_EVENT_PAGE_DISPATCH, "dispatch page list");
    worker_register_job_name(UV_EVENT_FLUSH_CALLBACK, "flush callback");
    worker_register_job_name(UV_EVENT_FLUSH_MAIN, "flush main");
    worker_register_job_name(UV_EVENT_FLUSH_OPEN, "flush open");
    worker_register_job_name(UV_EVENT_EVICT_MAIN, "evict open");
    worker_register_job_name(UV_EVENT_EVICT_OPEN, "evict main");
}

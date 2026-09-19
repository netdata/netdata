// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DBENGINE_WORKERS_H
#define NETDATA_DBENGINE_WORKERS_H

#include "libnetdata/libnetdata.h"

#ifdef __cplusplus
extern "C" {
#endif

// The engine runs its work on the process-wide libuv thread pool, which it shares with whoever else
// embeds it in the same process. Worker utilization is charted per job id, and every user of the pool
// registers its own job names on each pool thread it runs on; the ids therefore live in one shared
// space. The engine's block starts at 1; an embedder numbers its own jobs from DBENGINE_WORKER_JOB_MAX.
enum {
    DBENGINE_WORKER_JOB_NONE = 0,

    DBENGINE_WORKER_JOB_INIT,

    // query related
    DBENGINE_WORKER_JOB_QUERY,
    DBENGINE_WORKER_JOB_EXTENT_CACHE_LOOKUP,
    DBENGINE_WORKER_JOB_EXTENT_MMAP,
    DBENGINE_WORKER_JOB_EXTENT_DECOMPRESSION,
    DBENGINE_WORKER_JOB_EXTENT_PAGE_LOOKUP,
    DBENGINE_WORKER_JOB_EXTENT_PAGE_POPULATION,
    DBENGINE_WORKER_JOB_EXTENT_PAGE_ALLOCATION,

    // flushing related
    DBENGINE_WORKER_JOB_FLUSH_MAIN_CACHE,
    DBENGINE_WORKER_JOB_EXTENT_WRITE,
    DBENGINE_WORKER_JOB_FLUSHED_TO_OPEN,

    // datafile full
    DBENGINE_WORKER_JOB_JOURNAL_INDEX,

    // db rotation related
    DBENGINE_WORKER_JOB_DATAFILE_DELETE_WAIT,
    DBENGINE_WORKER_JOB_DATAFILE_DELETE,
    DBENGINE_WORKER_JOB_FIND_ROTATED_METRICS,     // find the metrics that are rotated
    DBENGINE_WORKER_JOB_FIND_REMAINING_RETENTION, // find their remaining retention
    DBENGINE_WORKER_JOB_POPULATE_MRG,             // update mrg

    // other
    DBENGINE_WORKER_JOB_EVICT_MAIN_CACHE,
    DBENGINE_WORKER_JOB_EVICT_OPEN_CACHE,
    DBENGINE_WORKER_JOB_EVICT_EXTENT_CACHE,
    DBENGINE_WORKER_JOB_BUFFERS_CLEANUP,
    DBENGINE_WORKER_JOB_FLUSH_DIRTY,
    DBENGINE_WORKER_JOB_QUIESCE,
    DBENGINE_WORKER_JOB_MRG_LOAD,
    DBENGINE_WORKER_JOB_SHUTDOWN,

    // terminator: the first id available to the embedder
    DBENGINE_WORKER_JOB_MAX,
};

#ifdef __cplusplus
}
#endif

#endif // NETDATA_DBENGINE_WORKERS_H

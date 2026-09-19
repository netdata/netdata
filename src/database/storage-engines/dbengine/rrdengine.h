// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDENGINE_H
#define NETDATA_RRDENGINE_H

// START HERE
//
// dbengine is netdata's tiered time-series store. Vocabulary used throughout this directory:
//
//   ctx (struct dbengine_tier)        one tier of one database: a directory of datafiles and their journals;
//                                     the daemon's tiers are dbengine_multidb_tiers[]
//   datafile / extent / page          on-disk container / a compressed group of pages / one metric's samples
//   journalfile                       the per-datafile index of extents and metrics (v1 while writing, v2 when sealed)
//   the engine (struct dbengine_engine) what the tiers share: the event loop, the caches, the metric registry, the
//                                     configuration, the counters
//   MRG, the metric registry          the engine's: every metric's uuid, section (its ctx) and retention (mrg.h)
//   PGC, the page cache               the engine's caches, shared by its ctxs (cache.h, pagecache.h)
//   PDC, the page details control     the plan of one query: which pages, from cache or disk, in what order (pdc.h)
//   the event loop                    the engine's single libuv thread that owns datafile I/O, flushing and rotation
//
// The daemon drives the engine through dbengine-api.h behind the storage-engine vtable, hands it its
// configuration and optional services through dbengine-config.h, and reads what the engine publishes
// (statistics, worker job ids) through the engine's own headers. Those public headers include nothing
// of this file: this header and everything it pulls in are the engine's private side. The engine, in
// turn, includes nothing of the daemon.

#include <fcntl.h>
#include <lz4.h>
#include <Judy.h>
#include <openssl/sha.h>
#include <openssl/evp.h>
#include "database/storage-engines/dbengine/include/dbengine/dbengine-config.h"
#include "database/storage-engines/dbengine/include/dbengine/dbengine-workers.h"

// the 0-means-default fields of *cfg become concrete values, so the engine never has to re-check them
void dbengine_config_resolve(struct dbengine_config *cfg);

struct dbengine_engine;

// where an engine is in its life: made and not spawned, running, or shut down (then only destroyed)
typedef enum {
    DBENGINE_LIFECYCLE_DOWN,
    DBENGINE_LIFECYCLE_RUNNING,
    DBENGINE_LIFECYCLE_STOPPED,
} DBENGINE_LIFECYCLE_STATE;
DBENGINE_LIFECYCLE_STATE dbengine_engine_lifecycle_state(struct dbengine_engine *engine);

// a configured engine with nothing running: the object and its locks, no loop, no caches; cfg is copied and resolved.
// The tests that need an engine without an event loop use it directly
struct dbengine_engine *dbengine_engine_alloc(const struct dbengine_config *cfg);

// release what the engine owns (its allocators, free lists, preload set, the loop's closes) and free the object.
// Only when no cache, registry or event loop of it exists any more
void dbengine_engine_free(struct dbengine_engine *engine);

#define DBENGINE_FD_BUDGET_PER_TIER (50)

#define DBENGINE_PAGE_TYPE_MAX (2) // Maximum page type (inclusive)

extern size_t page_type_size[];
extern size_t tier_page_size[];
#define CTX_POINT_SIZE_BYTES(ctx) page_type_size[(ctx)->config.page_type]

#include "rrddiskprotocol.h"
#include "rrdenginelib.h"
#include "datafile.h"
#include "journalfile.h"
#include "database/storage-engines/dbengine/include/dbengine/dbengine-api.h"
#include "pagecache.h"
#include "mrg.h"
#include "cache.h"
#include "pdc.h"
#include "page.h"

static ALWAYS_INLINE void time_and_count_add(struct dbengine_time_and_count *tc, usec_t dt) {
    __atomic_add_fetch(&tc->count, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&tc->usec, dt, __ATOMIC_RELAXED);
}

#define BLOCK_TO_OFFSET(block) ((uint64_t)(block) << 12)
#define OFFSET_TO_BLOCK(ofs) ((uint64_t)(ofs) >> 12)

#define UNLINK_FILE(ctx, path, ret_var)                                                                                \
    do {                                                                                                               \
        uv_fs_t _req;                                                                                                  \
        (ret_var) = uv_fs_unlink(NULL, &(_req), (path), NULL);                                                         \
        if ((ret_var) < 0) {                                                                                           \
            netdata_log_error("DBENGINE: uv_fs_unlink(\"%s\"): %s", (path), uv_strerror(ret_var));                     \
            ctx_fs_error(ctx);                                                                                         \
        }                                                                                                              \
        uv_fs_req_cleanup(&(_req));                                                                                   \
    } while (0)

#define CLOSE_FILE(ctx, path, file, ret_var)                                                                           \
    do {                                                                                                               \
        uv_fs_t _req;                                                                                                  \
        (ret_var) = uv_fs_close(NULL, &(_req), (file), NULL);                                                          \
        if ((ret_var) < 0) {                                                                                           \
            netdata_log_error("DBENGINE: uv_fs_close(\"%s\"): %s", (path), uv_strerror(ret_var));                      \
            ctx_fs_error(ctx);                                                                                         \
        }                                                                                                              \
        uv_fs_req_cleanup(&(_req));                                                                                    \
    } while (0)

/* Forward declarations */
struct dbengine_tier;
struct dbengine_cmd;
struct dbengine_engine;

#define MAX_PAGES_PER_EXTENT (109) /* TODO: can go higher only when journal supports bigger than 4KiB transactions */

#define MAX_EXTENT_UNCOMPRESSED_SIZE (MAX_PAGES_PER_EXTENT * (DBENGINE_BLOCK_SIZE + RRDENG_GORILLA_32BIT_BUFFER_SIZE))

static inline size_t dbengine_min_extent_disk_size(void) {
    return sizeof(struct dbengine_df_extent_header) +
           sizeof(struct dbengine_extent_page_descr) +
           sizeof(struct dbengine_df_extent_trailer);
}

static inline size_t dbengine_max_extent_disk_size(void) {
    return sizeof(struct dbengine_df_extent_header) +
           sizeof(struct dbengine_extent_page_descr) * MAX_PAGES_PER_EXTENT +
           MAX_EXTENT_UNCOMPRESSED_SIZE +
           sizeof(struct dbengine_df_extent_trailer);
}

static inline bool dbengine_valid_extent_disk_size(size_t size) {
    return size >= dbengine_min_extent_disk_size() && size <= dbengine_max_extent_disk_size();
}


#define DBENGINE_FILE_NUMBER_SCAN_TMPL "%1u-%10u"
#define DBENGINE_FILE_NUMBER_PRINT_TMPL "%1.1u-%10.10u"

typedef enum __attribute__ ((__packed__)) {
    // final status for all pages
    // if a page does not have one of these, it is considered unroutable
    PDC_PAGE_READY     = (1 << 0),                  // ready to be processed (pd->page is not null)
    PDC_PAGE_FAILED    = (1 << 1),                  // failed to be loaded (pd->page is null)
    PDC_PAGE_SKIP      = (1 << 2),                  // don't use this page, it is not good for us
    PDC_PAGE_INVALID   = (1 << 3),                  // don't use this page, it is invalid
    PDC_PAGE_EMPTY     = (1 << 4),                  // the page is empty, does not have any data

    // other statuses for tracking issues
    PDC_PAGE_PREPROCESSED              = (1 << 5),  // used during preprocessing
    PDC_PAGE_PROCESSED                 = (1 << 6),  // processed by the query caller
    PDC_PAGE_RELEASED                  = (1 << 7),  // already released

    // data found in cache (preloaded) or on disk?
    PDC_PAGE_PRELOADED                 = (1 << 8),  // data found in memory
    PDC_PAGE_DISK_PENDING              = (1 << 9),  // data need to be loaded from disk

    // worker related statuses
    PDC_PAGE_FAILED_INVALID_EXTENT     = (1 << 10),
    PDC_PAGE_FAILED_NOT_IN_EXTENT      = (1 << 11),
    PDC_PAGE_FAILED_TO_MAP_EXTENT      = (1 << 12),
    PDC_PAGE_FAILED_TO_ACQUIRE_DATAFILE= (1 << 13),

    PDC_PAGE_EXTENT_FROM_CACHE         = (1 << 14),
    PDC_PAGE_EXTENT_FROM_DISK          = (1 << 15),

    PDC_PAGE_CANCELLED                 = (1 << 16), // the query thread had left when we try to load the page

    PDC_PAGE_SOURCE_MAIN_CACHE         = (1 << 17),
    PDC_PAGE_SOURCE_OPEN_CACHE         = (1 << 18),
    PDC_PAGE_SOURCE_JOURNAL_V2         = (1 << 19),
    PDC_PAGE_PRELOADED_PASS4           = (1 << 20),

    // datafile acquired
    PDC_PAGE_DATAFILE_ACQUIRED         = (1 << 30),
} PDC_PAGE_STATUS;

#define PDC_PAGE_QUERY_GLOBAL_SKIP_LIST (PDC_PAGE_FAILED | PDC_PAGE_SKIP | PDC_PAGE_INVALID | PDC_PAGE_RELEASED)

typedef struct page_details_control {
    struct dbengine_tier *ctx;
    struct metric *metric;

    struct completion prep_completion;
    struct completion page_completion;   // sync between the query thread and the workers

    Pvoid_t page_list_JudyL;        // the list of page details
    unsigned completed_jobs;        // the number of jobs completed last time the query thread checked
    bool workers_should_stop;       // true when the query thread left and the workers should stop
    bool prep_done;

    PDC_PAGE_STATUS common_status;
    size_t pages_to_load_from_disk;

    SPINLOCK refcount_spinlock;     // spinlock to protect refcount
    int32_t refcount;               // the number of workers currently working on this request + 1 for the query thread
    size_t executed_with_gaps;

    time_t start_time_s;
    time_t end_time_s;
    STORAGE_PRIORITY priority;

    time_t optimal_end_time_s;
} PDC;

PDC *pdc_get(void);

struct page_details {
    struct {
        struct dbengine_datafile *ptr;
        uint32_t block;     // the block in the datafile. Offset in the datafile is block * DBENGINE_BLOCK_SIZE
        uint32_t bytes;
    } datafile;

    struct pgc_page *page;
    Word_t metric_id;
    time_t first_time_s;
    time_t last_time_s;
    uint32_t update_every_s;
    PDC_PAGE_STATUS status;

    struct {
        struct page_details *prev;
        struct page_details *next;
    } load;
};

struct page_details *page_details_get(void);

#define pdc_page_status_check(pd, flag) (__atomic_load_n(&((pd)->status), __ATOMIC_ACQUIRE) & (flag))
#define pdc_page_status_set(pd, flag)   __atomic_or_fetch(&((pd)->status), flag, __ATOMIC_RELEASE)
#define pdc_page_status_clear(pd, flag) __atomic_and_fetch(&((pd)->status), ~(flag), __ATOMIC_RELEASE)

struct jv2_extents_info {
    uint32_t index;
    uint32_t block;
    unsigned bytes;
    uint32_t number_of_pages;
};

struct jv2_metrics_info {
    nd_uuid_t *uuid;
    void *metric;
    uint32_t page_list_header;
    uint32_t number_of_pages;
    time_t first_time_s;
    time_t last_time_s;
    Pvoid_t JudyL_pages_by_start_time;
};

struct jv2_page_info {
    time_t start_time_s;
    time_t end_time_s;
    uint32_t update_every_s;
    uint32_t extent_index;
    size_t page_length;
    void *custom_data;

    // private
    struct pgc_page *page;
    struct jv2_extents_info *ei;    // the extent this page was counted into
};

typedef enum __attribute__ ((__packed__)) {
    DBENGINE_COLLECT_HANDLE_OPTION_NONE   = 0,

#ifdef NETDATA_INTERNAL_CHECKS
    DBENGINE_1ST_METRIC_WRITER            = (1 << 0),
#endif
} DBENGINE_COLLECT_HANDLE_OPTIONS;

typedef enum __attribute__ ((__packed__)) {
    DBENGINE_PAGE_PAST_COLLECTION       = (1 << 0),
    DBENGINE_PAGE_REPEATED_COLLECTION   = (1 << 1),
    DBENGINE_PAGE_BIG_GAP               = (1 << 2),
    DBENGINE_PAGE_GAP                   = (1 << 3),
    DBENGINE_PAGE_FUTURE_POINT          = (1 << 4),
    DBENGINE_PAGE_CREATED_IN_FUTURE     = (1 << 5),
    DBENGINE_PAGE_COMPLETED_IN_FUTURE   = (1 << 6),
    DBENGINE_PAGE_UNALIGNED             = (1 << 7),
    DBENGINE_PAGE_CONFLICT              = (1 << 8),
    DBENGINE_PAGE_FULL                  = (1 << 9),
    DBENGINE_PAGE_COLLECT_FINALIZE      = (1 << 10),
    DBENGINE_PAGE_UPDATE_EVERY_CHANGE   = (1 << 11),
    DBENGINE_PAGE_STEP_TOO_SMALL        = (1 << 12),
    DBENGINE_PAGE_STEP_UNALIGNED        = (1 << 13),
    DBENGINE_PAGE_RETENTION_RECORDED    = (1 << 14),
} DBENGINE_COLLECT_PAGE_FLAGS;

struct dbengine_collect_handle {
    struct storage_collect_handle common; // has to be first item

    DBENGINE_COLLECT_PAGE_FLAGS page_flags;
    DBENGINE_COLLECT_HANDLE_OPTIONS options;
    uint8_t type;

    struct dbengine_tier *ctx;
    struct metric *metric;
    struct pgc_page *pgc_page;
    struct pgd *page_data;
    struct pg_alignment *alignment;
    uint32_t page_entries_max;
    uint32_t page_position;                   // keep track of the current page size, to make sure we don't exceed it
    usec_t page_start_time_ut;
    usec_t page_end_time_ut;
    usec_t update_every_ut;
};

struct dbengine_query_handle {
    struct metric *metric;
    struct pgc_page *page;
    struct dbengine_tier *ctx;
    struct pgd_cursor pgdc;
    struct page_details_control *pdc;

    // the request
    time_t start_time_s;
    time_t end_time_s;
    STORAGE_PRIORITY priority;

    // internal data
    time_t now_s;
    uint32_t dt_s;

    unsigned position;
    unsigned entries;

#ifdef NETDATA_INTERNAL_CHECKS
    usec_t started_time_s;
    pid_t query_pid;
    struct dbengine_query_handle *prev, *next;
#endif
};

struct dbengine_query_handle *dbengine_query_handle_get(struct dbengine_engine *engine);
void dbengine_query_handle_release(struct dbengine_engine *engine, struct dbengine_query_handle *handle);

enum dbengine_opcode {
    /* can be used to return empty status or flush the command queue */
    DBENGINE_OPCODE_NOOP = 0,

    DBENGINE_OPCODE_QUERY,
    DBENGINE_OPCODE_EXTENT_WRITE,
    DBENGINE_OPCODE_EXTENT_READ,
    DBENGINE_OPCODE_DATABASE_ROTATE,
    DBENGINE_OPCODE_JOURNAL_INDEX,
    DBENGINE_OPCODE_FLUSH_MAIN,
    DBENGINE_OPCODE_EVICT_MAIN,
    DBENGINE_OPCODE_EVICT_OPEN,
    DBENGINE_OPCODE_EVICT_EXTENT,
    DBENGINE_OPCODE_CTX_SHUTDOWN,
    DBENGINE_OPCODE_CTX_FLUSH_DIRTY,
    DBENGINE_OPCODE_CTX_FLUSH_HOT_DIRTY,
    DBENGINE_OPCODE_CTX_QUIESCE,
    DBENGINE_OPCODE_CTX_POPULATE_MRG,
    DBENGINE_OPCODE_SHUTDOWN_EVLOOP,
    DBENGINE_OPCODE_EXTERNAL_WORK,
    DBENGINE_OPCODE_MRG_LOAD,
    DBENGINE_OPCODE_CLEANUP,

    DBENGINE_OPCODE_MAX
};

// WORKERS IDS:
// DBENGINE_MAX_OPCODE                     : reserved for the cleanup
// DBENGINE_MAX_OPCODE + opcode            : reserved for the callbacks of each opcode
// DBENGINE_MAX_OPCODE + DBENGINE_MAX_OPCODE : reserved for the timer
#define DBENGINE_TIMER_CB (DBENGINE_OPCODE_MAX + DBENGINE_OPCODE_MAX)
#define DBENGINE_OPCODES_WAITING             (DBENGINE_TIMER_CB + 1)
#define DBENGINE_WORKS_DISPATCHED            (DBENGINE_TIMER_CB + 2)
#define DBENGINE_WORKS_EXECUTING             (DBENGINE_TIMER_CB + 3)
#define DBENGINE_RETENTION_TIMER_CB          (DBENGINE_TIMER_CB + 4)

struct extent_io_data {
    unsigned fileno;
    uint32_t block;
    unsigned bytes;

    // The metric this page belongs to, as a uuidmap id.
    //
    // This exists so journal-v2 indexing never has to dereference the page's
    // metric_id, which is a bare METRIC pointer with no reference behind it and
    // can therefore be stale. Resolving the id through the MRG index is an
    // indexed lookup that cannot touch freed memory; a dead metric simply
    // misses. NOT a uuidmap reference - see mrg_metric_uuidmap_id().
    UUIDMAP_ID uuid_id;
};

struct extent_io_descriptor {
    struct dbengine_tier *ctx;
    void *buf;
    uint64_t pos;
    uint32_t descr_count;
    uint32_t bytes;
    uint32_t real_io_size;
    struct wal *wal;
    uv_file file;
    struct page_descr_with_data *descr_array[MAX_PAGES_PER_EXTENT];
    struct dbengine_datafile *datafile;
};

typedef struct wal {
    uint64_t transaction_id;
    void *buf;
    size_t size;
    size_t buf_size;

    struct {
        struct wal *prev;
        struct wal *next;
    } cache;
} WAL;

WAL *wal_get(struct dbengine_tier *ctx, unsigned size);
void wal_release(struct dbengine_engine *engine, WAL *wal);

/*
 * Debug statistics not used by code logic.
 * They only describe operations since the tier was loaded.
 */
struct dbengine_statistics {
    PAD64(dbengine_stats_t) before_decompress_bytes;
    PAD64(dbengine_stats_t) after_decompress_bytes;
    PAD64(dbengine_stats_t) before_compress_bytes;
    PAD64(dbengine_stats_t) after_compress_bytes;

    PAD64(dbengine_stats_t) io_write_bytes;
    PAD64(dbengine_stats_t) io_write_requests;
    PAD64(dbengine_stats_t) io_read_bytes;
    PAD64(dbengine_stats_t) io_read_requests;

    PAD64(dbengine_stats_t) datafile_creations;
    PAD64(dbengine_stats_t) datafile_deletions;
    PAD64(dbengine_stats_t) journalfile_creations;
    PAD64(dbengine_stats_t) journalfile_deletions;

    PAD64(dbengine_stats_t) io_errors;
    PAD64(dbengine_stats_t) fs_errors;
};

struct dbengine_global_stats {
    /* I/O errors global counter */
    PAD64(dbengine_stats_t) global_io_errors;

    /* File-System errors global counter */
    PAD64(dbengine_stats_t) global_fs_errors;

    /* number of File-Descriptors that have been reserved by dbengine */
    PAD64(dbengine_stats_t) dbengine_reserved_file_descriptors;

    /* inability to flush global counters */
    PAD64(dbengine_stats_t) global_pg_cache_over_half_dirty_events;
    PAD64(dbengine_stats_t) global_flushing_pressure_page_deletions; /* number of deleted pages */
};

typedef struct tier_config_prototype {
    int tier;                                   // the tier of this ctx
    uint8_t page_type;                          // default page type for this context
    size_t grouping;                            // points of tier 0 per point of this tier
    uint64_t max_disk_space;                    // the max disk space this ctx is allowed to use
    time_t max_retention_s;                     // The max retention in seconds
    uint8_t global_compress_alg;                // the wanted compression algorithm
    char dbfiles_path[FILENAME_MAX + 1];
} TIER_CONFIG_PROTOTYPE;

struct dbengine_tier {
    struct dbengine_engine *engine;                     // the engine this tier belongs to, set before the engine spawns

    TIER_CONFIG_PROTOTYPE config;

    struct {
        netdata_rwlock_t rwlock;                         // the JudyL of datafiles is protected by this lock
        bool disk_time;                             // true: delete for disk quota, false: delete for retention
        bool pending_rotate;                        // Change from event loop
        bool pending_index;                         // Change from event loop
        Pvoid_t JudyL;                              // the datafiles, indexed by fileno
    } datafiles;

    struct {
        RW_SPINLOCK spinlock;
        Pvoid_t JudyL;
    } njfv2idx;

    struct {
        PAD64(unsigned) last_fileno;                       // newest index of datafile and journalfile
        PAD64(unsigned) last_flush_fileno;                 // newest index of datafile received data

        PAD64(size_t) collectors_running;
        PAD64(size_t) collectors_running_duplicate;
        PAD64(size_t) inflight_queries;                    // the number of queries currently running
        PAD64(uint64_t) current_disk_space;                // the current disk space size used

        PAD64(uint64_t) transaction_id;                    // the transaction id of the next extent flushing

        PAD64(bool) active;                                // set by a successful dbengine_tier_init(), cleared by dbengine_tier_exit()
        PAD64(bool) came_up;                               // set on a successful init, just before active; never cleared by
                                                           // dbengine_tier_exit(): a tier that
                                                           // exited keeps its datafiles attached until dbengine_destroy() finalizes
                                                           // them and initialize_tier() resets the slot, so it cannot come up again
        PAD64(bool) mrg_populated;                         // set when the metrics registry has been loaded from every journal
        PAD64(bool) migration_to_v2_running;
        PAD64(bool) now_deleting_files;
        PAD64(bool) needs_indexing;
        PAD64(unsigned) extents_currently_being_flushed;   // non-zero until we commit data to disk (both datafile and journal file)

        PAD64(time_t) first_time_s;
        PAD64(uint64_t) metrics;
        PAD64(uint64_t) samples;
    } atomic;

    struct {
        bool exit_mode;
        bool enabled;                               // when set (before shutdown), queries are prohibited
    } quiesce;

    struct {
        struct completion load_mrg;
        bool create_new_datafile_pair;
    } loading;

    struct dbengine_statistics stats;
};

// What the tiers of one database share. Everything but the tiers themselves (still the static
// dbengine_multidb_tiers[]) hangs here: the event loop and its queues, the caches, the metric registry, the
// configuration, the counters. The page allocators stay process-wide (dbengine-config.h: allocator).
struct dbengine_engine {
    // the event loop
    ND_THREAD *thread;
    uv_loop_t loop;
    bool loop_open;                     // uv_loop_init() succeeded and the loop is not closed yet: a loop the thread
                                        // could not close on its way out is run to completion and closed by
                                        // dbengine_shutdown() once the thread was joined
    uv_async_t async;
#if defined(OS_WINDOWS)
    bool async_ready;
    uint64_t last_async_callback;
    int async_timeout_count;            // consecutive query rounds without an async callback; the loop re-creates
                                        // the async handle when it passes its limit
#endif
    uv_timer_t timer;
    uv_timer_t retention_timer;
    pid_t tid;

    size_t flushes_running;
    size_t evict_main_running;
    size_t evict_open_running;
    size_t evict_extent_running;
    size_t cleanup_running;
    time_t cleanup_last_run_s;          // when the journal unmount sweep last ran, in the cleanup worker

    // where the engine is in its life
    struct {
        SPINLOCK spinlock;
        bool spawned;
        bool stopped;
    } lifecycle;

    // the configuration, resolved: the copy of what dbengine_create() was given
    struct dbengine_config cfg;

    // the caches and the registry the tiers share
    struct mrg *main_mrg;
    struct pgc *main_cache;
    struct pgc *open_cache;
    struct pgc *extent_cache;

    struct dbengine_cache_efficiency_stats cache_efficiency_stats;
    struct dbengine_global_stats global_stats;

    // the metrics the registry preload acquired, until dbengine_preload_release() lets them go
    struct {
        METRIC_JudyLSet acquired;
        size_t counter;
        size_t deleted;
    } preload;

    // the write-ahead buffers idle between extents
    struct {
        struct {
            SPINLOCK spinlock;
            WAL *available_items;
            size_t available;
        } guarded;

        struct {
            size_t allocated;
        } atomics;
    } wal;

    netdata_mutex_t datafile_write_mutex;   // serialises the choice of the datafile an extent is written to

    struct {
        ARAL *ar;

        struct {
            SPINLOCK spinlock;

            bool accepting;                     // embedder work is taken; set when the event loop is spawned,
                                                // cleared by dbengine_shutdown() before it queues the loop's exit
            size_t waiting;
            struct dbengine_cmd *waiting_items_by_priority[STORAGE_PRIORITY_INTERNAL_MAX_DONT_USE];
            size_t executed_by_priority[STORAGE_PRIORITY_INTERNAL_MAX_DONT_USE];
        } unsafe;
    } cmd_queue;

    struct {
        ARAL *ar;

        struct {
            size_t dispatched;
            size_t executing;
        } atomics;
    } work_cmd;

    struct {
        ARAL *ar;
    } handles;

    struct {
        ARAL *ar;
    } descriptors;

    struct {
        ARAL *ar;
    } xt_io_descr;
};

// Retention and indexing work wait until the registry has been loaded from every journal: rotating a
// datafile before then would drop metrics the registry has not learned yet. The loader's own datafile references only
// protect the files it is reading at that moment; this flag keeps rotation from starting at all.
static inline bool dbengine_ctx_is_mrg_populated(struct dbengine_tier *ctx) {
    return __atomic_load_n(&ctx->atomic.mrg_populated, __ATOMIC_ACQUIRE);
}


#define ctx_current_disk_space_get(ctx) __atomic_load_n(&(ctx)->atomic.current_disk_space, __ATOMIC_RELAXED)
#define ctx_current_disk_space_increase(ctx, size) __atomic_add_fetch(&(ctx)->atomic.current_disk_space, size, __ATOMIC_RELAXED)
#define ctx_current_disk_space_decrease(ctx, size) __atomic_sub_fetch(&(ctx)->atomic.current_disk_space, size, __ATOMIC_RELAXED)

static inline void ctx_io_read_op_bytes(struct dbengine_tier *ctx, size_t bytes) {
    __atomic_add_fetch(&ctx->stats.io_read_bytes, bytes, __ATOMIC_RELAXED);
    __atomic_add_fetch(&ctx->stats.io_read_requests, 1, __ATOMIC_RELAXED);
}

static inline void ctx_io_write_op_bytes(struct dbengine_tier *ctx, size_t bytes) {
    __atomic_add_fetch(&ctx->stats.io_write_bytes, bytes, __ATOMIC_RELAXED);
    __atomic_add_fetch(&ctx->stats.io_write_requests, 1, __ATOMIC_RELAXED);
}

static inline void ctx_io_error(struct dbengine_tier *ctx) {
    __atomic_add_fetch(&ctx->stats.io_errors, 1, __ATOMIC_RELAXED);
    rrd_stat_atomic_add(&ctx->engine->global_stats.global_io_errors, 1);
}

static inline void ctx_fs_error(struct dbengine_tier *ctx) {
    __atomic_add_fetch(&ctx->stats.fs_errors, 1, __ATOMIC_RELAXED);
    rrd_stat_atomic_add(&ctx->engine->global_stats.global_fs_errors, 1);
}

static inline bool dbengine_retention_samples_delta(
    struct dbengine_tier *ctx,
    time_t first_time_s,
    time_t last_time_s,
    uint32_t update_every_s,
    const char *reason,
    uint64_t *samples)
{
    *samples = 0;

    if(!update_every_s || !first_time_s || !last_time_s || first_time_s == last_time_s)
        return false;

    if(unlikely(first_time_s > last_time_s)) {
        int tier = ctx ? ctx->config.tier : -1;

        internal_fatal(
            true,
            "DBENGINE: tier %d: invalid retention interval while %s (first=%ld, last=%ld, update_every=%u)",
            tier,
            reason,
            (long)first_time_s,
            (long)last_time_s,
            update_every_s);

        nd_log_limit_static_global_var(erl, 60, 0);
        nd_log_limit(
            &erl,
            NDLS_DAEMON,
            NDLP_ERR,
            "DBENGINE: tier %d: invalid retention interval while %s (first=%ld, last=%ld, update_every=%u); not updating sample counter",
            tier,
            reason,
            (long)first_time_s,
            (long)last_time_s,
            update_every_s);

        return false;
    }

    *samples = (last_time_s - first_time_s) / update_every_s;
    return *samples > 0;
}

static inline bool dbengine_atomic_uint64_sub_saturating(
    struct dbengine_tier *ctx,
    uint64_t *counter,
    uint64_t value,
    const char *counter_name,
    const char *reason)
{
    if(!value)
        return true;

    uint64_t old = __atomic_load_n(counter, __ATOMIC_RELAXED);
    while(true) {
        if(unlikely(old < value)) {
            int tier = ctx ? ctx->config.tier : -1;

            internal_fatal(
                true,
                "DBENGINE: tier %d: %s counter underflow while %s (current=%" PRIu64 ", subtract=%" PRIu64 ")",
                tier,
                counter_name,
                reason,
                old,
                value);

            nd_log_limit_static_global_var(erl, 60, 0);
            nd_log_limit(
                &erl,
                NDLS_DAEMON,
                NDLP_ERR,
                "DBENGINE: tier %d: %s counter underflow while %s (current=%" PRIu64 ", subtract=%" PRIu64 "); saturating to zero",
                tier,
                counter_name,
                reason,
                old,
                value);

            if(__atomic_compare_exchange_n(counter, &old, 0, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED))
                return false;

            continue;
        }

        uint64_t wanted = old - value;
        if(__atomic_compare_exchange_n(counter, &old, wanted, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED))
            return true;
    }
}

#define ctx_last_fileno_get(ctx) __atomic_load_n(&(ctx)->atomic.last_fileno, __ATOMIC_RELAXED)
#define ctx_last_fileno_increment(ctx) __atomic_add_fetch(&(ctx)->atomic.last_fileno, 1, __ATOMIC_RELAXED)

#define ctx_last_flush_fileno_get(ctx) __atomic_load_n(&(ctx)->atomic.last_flush_fileno, __ATOMIC_RELAXED)
static inline void ctx_last_flush_fileno_set(struct dbengine_tier *ctx, unsigned fileno) {
    unsigned old_fileno = ctx_last_flush_fileno_get(ctx);

    do {
        if(old_fileno >= fileno)
            return;

    } while(!__atomic_compare_exchange_n(&ctx->atomic.last_flush_fileno, &old_fileno, fileno, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED));
}

#define ctx_is_available_for_queries(ctx) (__atomic_load_n(&(ctx)->quiesce.enabled, __ATOMIC_RELAXED) == false && __atomic_load_n(&(ctx)->quiesce.exit_mode, __ATOMIC_RELAXED) == false)

bool dbengine_ctx_tier_cap_exceeded(struct dbengine_tier *ctx);
int init_rrd_files(struct dbengine_tier *ctx);
void dbengine_event_loop(void *arg);

typedef void (*enqueue_callback_t)(struct dbengine_cmd *cmd);
typedef void (*dequeue_callback_t)(struct dbengine_cmd *cmd);

void dbengine_enqueue_epdl_cmd(struct dbengine_cmd *cmd);
void dbengine_dequeue_epdl_cmd(struct dbengine_cmd *cmd);

typedef struct dbengine_cmd *(*requeue_callback_t)(void *data);
void dbengine_req_cmd(struct dbengine_engine *engine, requeue_callback_t get_cmd_cb, void *data, STORAGE_PRIORITY priority);

void dbengine_enq_cmd(struct dbengine_engine *engine, struct dbengine_tier *ctx, enum dbengine_opcode opcode, void *data,
                struct completion *completion, enum storage_priority priority,
                enqueue_callback_t enqueue_cb, dequeue_callback_t dequeue_cb);

void pdc_route_asynchronously(struct dbengine_tier *ctx, struct page_details_control *pdc);
void pdc_route_synchronously(struct dbengine_tier *ctx, struct page_details_control *pdc);
void pdc_route_synchronously_first(struct dbengine_tier *ctx, struct page_details_control *pdc);

void pdc_acquire(PDC *pdc);
bool pdc_release_and_destroy_if_unreferenced(PDC *pdc, bool worker, bool router);

uint64_t dbengine_target_data_file_size(struct dbengine_tier *ctx);

struct page_descr_with_data *page_descriptor_get(struct dbengine_engine *engine);

typedef struct validated_page_descriptor {
    time_t start_time_s;
    time_t end_time_s;
    uint32_t update_every_s;
    size_t page_length;
    size_t point_size;
    size_t entries;
    uint8_t type;
    bool is_valid;
} VALIDATED_PAGE_DESCRIPTOR;

#define page_entries_by_time(start_time_s, end_time_s, update_every_s) \
        ((update_every_s) ? (((end_time_s) - ((start_time_s) - (update_every_s))) / (update_every_s)) : 1)

#define page_entries_by_size(page_length_in_bytes, point_size_in_bytes) \
        ((page_length_in_bytes) / (point_size_in_bytes))

VALIDATED_PAGE_DESCRIPTOR validate_page(nd_uuid_t *uuid,
                                        time_t start_time_s,
                                        time_t end_time_s,
                                        uint32_t update_every_s,
                                        size_t page_length,
                                        uint8_t page_type,
                                        size_t entries,
                                        time_t now_s,
                                        uint32_t overwrite_zero_update_every_s,
                                        bool have_read_error,
                                        const char *msg,
                                        DBENGINE_COLLECT_PAGE_FLAGS flags);
VALIDATED_PAGE_DESCRIPTOR validate_extent_page_descr(const struct dbengine_extent_page_descr *descr, time_t now_s, uint32_t overwrite_zero_update_every_s, bool have_read_error);
void collect_page_flags_to_buffer(BUFFER *wb, DBENGINE_COLLECT_PAGE_FLAGS flags);

typedef enum {
    PAGE_IS_IN_THE_PAST   = -1,
    PAGE_IS_IN_RANGE      =  0,
    PAGE_IS_IN_THE_FUTURE =  1,
} TIME_RANGE_COMPARE;

TIME_RANGE_COMPARE is_page_in_time_range(time_t page_first_time_s, time_t page_last_time_s, time_t wanted_start_time_s, time_t wanted_end_time_s);

static inline time_t max_acceptable_collected_time(void) {
    return now_realtime_sec() + 1;
}

void datafile_delete(
    struct dbengine_tier *ctx,
    struct dbengine_datafile *datafile,
    bool update_retention,
    bool disk_time,
    bool worker);

// --------------------------------------------------------------------------------------------------------------------
// the following functions are used to sort UUIDs in the journal files
// DO NOT CHANGE, as this will break backwards compatibility with the data files users have.

static inline int journal_uuid_memcmp(const nd_uuid_t *uu1, const nd_uuid_t *uu2) {
    return memcmp(uu1, uu2, sizeof(nd_uuid_t));
}

static inline int journal_metric_uuid_compare(const void *key, const void *metric) {
    return journal_uuid_memcmp((const nd_uuid_t *)key, (const nd_uuid_t *)&(((struct journal_metric_list *) metric)->uuid));
}

// --------------------------------------------------------------------------------------------------------------------
// dbengine_get_used_disk_space() for a caller that already holds ctx->datafiles.rwlock
uint64_t dbengine_get_used_disk_space_unsafe(struct dbengine_tier *ctx);
// after dbengine_tier_exit(), on a static multidb tier only: close its datafiles
void finalize_rrd_files(struct dbengine_tier *ctx);
size_t datafile_count(struct dbengine_tier *ctx, bool with_lock);
struct dbengine_datafile *get_first_ctx_datafile(struct dbengine_tier *ctx, bool with_lock);
struct dbengine_datafile *get_last_ctx_datafile(struct dbengine_tier *ctx, bool with_lock);
struct dbengine_datafile *
get_next_datafile(struct dbengine_datafile *this_datafile, struct dbengine_tier *ctx, bool with_lock);


#endif /* NETDATA_RRDENGINE_H */

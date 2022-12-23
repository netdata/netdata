// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDENGINE_H
#define NETDATA_RRDENGINE_H

#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif
#include <fcntl.h>
#include <lz4.h>
#include <Judy.h>
#include <openssl/sha.h>
#include <openssl/evp.h>
#include "daemon/common.h"
#include "../rrd.h"
#include "rrddiskprotocol.h"
#include "rrdenginelib.h"
#include "datafile.h"
#include "journalfile.h"
#include "rrdengineapi.h"
#include "pagecache.h"
#include "../engine2/metric.h"
#include "../engine2/cache.h"

extern unsigned rrdeng_pages_per_extent;

/* Forward declarations */
struct rrdengine_instance;

#define MAX_PAGES_PER_EXTENT (64) /* TODO: can go higher only when journal supports bigger than 4KiB transactions */

#define GET_JOURNAL_DATA(x) __atomic_load_n(&(x)->journal_data, __ATOMIC_ACQUIRE)
#define GET_JOURNAL_DATA_SIZE(x) __atomic_load_n(&(x)->journal_data_size, __ATOMIC_ACQUIRE)
#define SET_JOURNAL_DATA(x, y) __atomic_store_n(&(x)->journal_data, (y), __ATOMIC_RELEASE)
#define SET_JOURNAL_DATA_SIZE(x, y) __atomic_store_n(&(x)->journal_data_size, (y), __ATOMIC_RELEASE)

#define RRDENG_FILE_NUMBER_SCAN_TMPL "%1u-%10u"
#define RRDENG_FILE_NUMBER_PRINT_TMPL "%1.1u-%10.10u"

typedef struct page_details_control {
    struct rrdengine_instance *ctx;

    struct completion completion;   // sync between the query thread and the workers

    Pvoid_t page_list_JudyL;        // the list of page details
    unsigned completed_jobs;        // the number of jobs completed last time the query thread checked
    bool preload_all_extent_pages;  // true to preload all the pages on each extent involved in the query
    bool workers_should_stop;       // true when the query thread left and the workers should stop

    SPINLOCK refcount_spinlock;     // spinlock to protect refcount
    int32_t refcount;               // the number of workers currently working on this request + 1 for the query thread
} PDC;

typedef enum __attribute__ ((__packed__)) {
    // all pages must pass through these states
    PDC_PAGE_READY     = (1 << 0),                  // ready to be processed (pd->page is not null)
    PDC_PAGE_FAILED    = (1 << 1),                  // failed to be loaded (pd->page is null)
    PDC_PAGE_PROCESSED = (1 << 2),                  // processed by the query caller
    PDC_PAGE_RELEASED  = (1 << 3),                  // already released

    // other statuses for tracking issues

    // data found in cache (preloaded) or on disk?
    PDC_PAGE_PRELOADED                 = (1 << 4),  // data found in memory
    PDC_PAGE_DISK_PENDING              = (1 << 5),  // data need to be loaded from disk

    // worker related statuses
    PDC_PAGE_FAILED_INVALID_EXTENT     = (1 << 6),
    PDC_PAGE_FAILED_UUID_NOT_IN_EXTENT = (1 << 7),
    PDC_PAGE_FAILED_TO_MAP_EXTENT      = (1 << 8),
    PDC_PAGE_FAILED_TO_ACQUIRE_DATAFILE= (1 << 9),

    PDC_PAGE_LOADED_FROM_EXTENT_CACHE  = (1 << 10),
    PDC_PAGE_LOADED_FROM_DISK          = (1 << 11),

    PDC_PAGE_PRELOADED_PASS1           = (1 << 12),
    PDC_PAGE_PRELOADED_PASS4           = (1 << 13),
    PDC_PAGE_PRELOADED_WORKER          = (1 << 14),

    PDC_PAGE_SOURCE_MAIN_CACHE         = (1 << 15),
    PDC_PAGE_SOURCE_OPEN_CACHE         = (1 << 16),
    PDC_PAGE_SOURCE_JOURNAL_V2         = (1 << 17),

    // datafile acquired
    PDC_PAGE_DATAFILE_ACQUIRED         = (1 << 30),
} PDC_PAGE_STATUS;

struct page_details {
    struct {
        struct rrdengine_datafile *ptr;
        uv_file file;
        unsigned fileno;

        struct {
            uint64_t pos;
            uint32_t bytes;
        } extent;
    } datafile;

    struct pgc_page *page;
    Word_t metric_id;
    time_t first_time_s;
    time_t last_time_s;
    uint32_t update_every_s;
    uint16_t page_length;
    uint8_t type;
    PDC_PAGE_STATUS status;
};

#define pdc_page_status_check(pd, flag) (__atomic_load_n(&((pd)->status), __ATOMIC_ACQUIRE) & (flag))
#define pdc_page_status_set(pd, flag)   __atomic_or_fetch(&((pd)->status), flag, __ATOMIC_RELEASE)
#define pdc_page_status_clear(pd, flag) __atomic_and_fetch(&((od)->status), ~(flag), __ATOMIC_RELEASE)

struct jv2_extents_info {
    size_t index;
    uint64_t pos;
    unsigned bytes;
    size_t number_of_pages;
};

struct jv2_metrics_info {
    uuid_t *uuid;
    uint32_t page_list_header;
    time_t first_time_t;
    time_t last_time_t;
    size_t number_of_pages;
    Pvoid_t JudyL_pages_by_start_time;
};

struct jv2_page_info {
    time_t start_time_t;
    time_t end_time_t;
    time_t update_every;
    size_t page_length;
    uint32_t extent_index;
    void *custom_data;

    // private
    struct pgc_page *page;
};

typedef enum __attribute__ ((__packed__)) {
    RRDENG_CHO_UNALIGNED        = (1 << 0), // set when this metric is not page aligned according to page alignment
    RRDENG_CHO_SET_FIRST_TIME_T = (1 << 1), // set when this metric has unset first_time_t and needs to be updated on the first data collection
    RRDENG_FIRST_PAGE_ALLOCATED = (1 << 2), // set when this metric has allocated its first page
} RRDENG_COLLECT_HANDLE_OPTIONS;

struct rrdeng_collect_handle {
    struct metric *metric;
    struct pgc_page *page;
    struct pg_alignment *alignment;
    RRDENG_COLLECT_HANDLE_OPTIONS options;
    uint8_t type;
    // 2 bytes remaining here for future use
    uint32_t page_entries_max;
    uint32_t page_position;                   // keep track of the current page size, to make sure we don't exceed it
    usec_t page_end_time_ut;
    usec_t update_every_ut;
};

struct rrdeng_query_handle {
    struct metric *metric;
    struct pgc_page *page;
    struct rrdengine_instance *ctx;
    storage_number *metric_data;
    struct page_details_control *pdc;

    time_t wanted_next_page_start_time_s;
    time_t now_s;
    time_t dt_s;

    unsigned position;
    unsigned entries;
};

enum rrdeng_opcode {
    /* can be used to return empty status or flush the command queue */
    RRDENG_NOOP = 0,

    RRDENG_READ_EXTENT,
    RRDENG_COMMIT_PAGE,
    RRDENG_FLUSH_PAGES,
    RRDENG_SHUTDOWN,
    RRDENG_QUIESCE,
    RRDENG_READ_PAGE_LIST,

    RRDENG_MAX_OPCODE
};

struct rrdeng_cmd {
    enum rrdeng_opcode opcode;
    void *data;
    struct completion *completion;
};

#define RRDENG_CMD_Q_MAX_SIZE (8192)

struct rrdeng_work {
    uv_work_t req;
    struct rrdengine_worker_config *wc;
    void *data;
    uint32_t count;
    bool rerun;
    struct completion *completion;
};

struct rrdeng_cmdqueue {
    unsigned head, tail;
    struct rrdeng_cmd cmd_array[RRDENG_CMD_Q_MAX_SIZE];
};

struct extent_io_data {
    unsigned fileno;
    uv_file file;
    uint64_t pos;
    unsigned bytes;
    uint16_t page_length;
};

struct extent_io_descriptor {
    uv_fs_t req;
    uv_buf_t iov;
    uv_file file;
    void *buf;
    uint64_t pos;
    unsigned bytes;
    struct completion *completion;
    unsigned descr_count;
    struct rrdeng_page_descr *descr_array[MAX_PAGES_PER_EXTENT];
    struct rrdengine_datafile *datafile;
    Word_t descr_commit_idx_array[MAX_PAGES_PER_EXTENT];
    struct extent_io_descriptor *next; /* multiple requests to be served by the same cached extent */
};

struct generic_io_descriptor {
    uv_fs_t req;
    uv_buf_t iov;
    void *buf;
    void *data;
    uint64_t pos;
    unsigned bytes;
    struct completion *completion;
};

struct rrdengine_worker_config {
    struct rrdengine_instance *ctx;

    uv_thread_t thread;
    uv_loop_t *loop;
    uv_async_t async;

    unsigned long now_deleting_files;

    unsigned long running_journal_migration;
    unsigned long running_cache_flush_evictions;
    unsigned outstanding_flush_requests;
    bool run_indexing;
    bool cache_flush_can_run;

    /* FIFO command queue */
    uv_mutex_t cmd_mutex;
    uv_cond_t cmd_cond;
    volatile unsigned queue_size;
    struct rrdeng_cmdqueue cmd_queue;
    int error;
};

/*
 * Debug statistics not used by code logic.
 * They only describe operations since DB engine instance load time.
 */
struct rrdengine_statistics {
    rrdeng_stats_t metric_API_producers;
    rrdeng_stats_t metric_API_consumers;
    rrdeng_stats_t pg_cache_insertions;
    rrdeng_stats_t pg_cache_deletions;
    rrdeng_stats_t pg_cache_hits;
    rrdeng_stats_t pg_cache_misses;
    rrdeng_stats_t pg_cache_backfills;
    rrdeng_stats_t pg_cache_evictions;
    rrdeng_stats_t before_decompress_bytes;
    rrdeng_stats_t after_decompress_bytes;
    rrdeng_stats_t before_compress_bytes;
    rrdeng_stats_t after_compress_bytes;
    rrdeng_stats_t io_write_bytes;
    rrdeng_stats_t io_write_requests;
    rrdeng_stats_t io_read_bytes;
    rrdeng_stats_t io_read_requests;
    rrdeng_stats_t io_write_extent_bytes;
    rrdeng_stats_t io_write_extents;
    rrdeng_stats_t io_read_extent_bytes;
    rrdeng_stats_t io_read_extents;
    rrdeng_stats_t datafile_creations;
    rrdeng_stats_t datafile_deletions;
    rrdeng_stats_t journalfile_creations;
    rrdeng_stats_t journalfile_deletions;
    rrdeng_stats_t page_cache_descriptors;
    rrdeng_stats_t io_errors;
    rrdeng_stats_t fs_errors;
    rrdeng_stats_t pg_cache_over_half_dirty_events;
    rrdeng_stats_t flushing_pressure_page_deletions;
};

/* I/O errors global counter */
extern rrdeng_stats_t global_io_errors;
/* File-System errors global counter */
extern rrdeng_stats_t global_fs_errors;
/* number of File-Descriptors that have been reserved by dbengine */
extern rrdeng_stats_t rrdeng_reserved_file_descriptors;
/* inability to flush global counters */
extern rrdeng_stats_t global_pg_cache_over_half_dirty_events;
extern rrdeng_stats_t global_flushing_pressure_page_deletions; /* number of deleted pages */

#define NO_QUIESCE  (0) /* initial state when all operations function normally */
#define SET_QUIESCE (1) /* set it before shutting down the instance, quiesce long running operations */
#define QUIESCED    (2) /* is set after all threads have finished running */

struct rrdengine_instance {
    struct rrdengine_worker_config worker_config;
    struct completion rrdengine_completion;
    bool journal_initialization;
    uint8_t global_compress_alg;
    struct transaction_commit_log commit_log;
    struct rrdengine_datafile_list datafiles;
    RRDHOST *host; /* the legacy host, or NULL for multi-host DB */
    char dbfiles_path[FILENAME_MAX + 1];
    char machine_guid[GUID_LEN + 1]; /* the unique ID of the corresponding host, or localhost for multihost DB */
    uint64_t disk_space;
    uint64_t max_disk_space;
    int tier;
    unsigned last_fileno; /* newest index of datafile and journalfile */
    unsigned long metric_API_max_producers;

    bool create_new_datafile_pair;
    uint8_t quiesce;   /* set to SET_QUIESCE before shutdown of the engine */
    uint8_t page_type; /* Default page type for this context */

    size_t inflight_queries;
    struct rrdengine_statistics stats;
};

void *dbengine_page_alloc(struct rrdengine_instance *ctx, size_t size);
void dbengine_page_free(void *page);

int init_rrd_files(struct rrdengine_instance *ctx);
void finalize_rrd_files(struct rrdengine_instance *ctx);
void rrdeng_worker(void *arg);
void rrdeng_enq_cmd(struct rrdengine_worker_config *wc, struct rrdeng_cmd *cmd);
struct rrdeng_cmd rrdeng_deq_cmd(struct rrdengine_worker_config *wc);
void after_journal_indexing(uv_work_t *req, int status);
void start_journal_indexing(uv_work_t *req);
void pdc_destroy(PDC *pdc);

void dbengine_load_page_list(struct rrdengine_instance *ctx, struct page_details_control *pdc);
void dbengine_load_page_list_directly(struct rrdengine_instance *ctx, struct page_details_control *pdc);

bool pdc_release_and_destroy_if_unreferenced(PDC *pdc, bool worker, bool router);

unsigned rrdeng_target_data_file_size(struct rrdengine_instance *ctx);

typedef struct validated_page_descriptor {
    time_t start_time_s;
    time_t end_time_s;
    time_t update_every_s;
    size_t page_length;
    size_t point_size;
    size_t entries;
    uint8_t type;
    bool data_on_disk_valid;
} VALIDATED_PAGE_DESCRIPTOR;

VALIDATED_PAGE_DESCRIPTOR validate_extent_page_descr(const struct rrdeng_extent_page_descr *descr, time_t now_s, time_t overwrite_zero_update_every_s, bool have_read_error);

#endif /* NETDATA_RRDENGINE_H */

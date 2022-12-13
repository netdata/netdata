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

#define RRDENG_FILE_NUMBER_SCAN_TMPL "%1u-%10u"
#define RRDENG_FILE_NUMBER_PRINT_TMPL "%1.1u-%10.10u"

struct page_details_control {
  unsigned reference_count;
  unsigned jobs_started;
  unsigned jobs_completed;
  unsigned completion_jobs_completed;
  struct completion completion;
  Pvoid_t pl_JudyL;
};

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

    uuid_t uuid;
    time_t first_time_s;
    time_t last_time_s;
    uint32_t update_every_s;
    uint16_t page_length;
    uint8_t type;
    bool page_failed_to_load;
    bool page_is_loaded;
    bool page_is_released;
    struct pgc_page *page;
};

typedef enum __attribute__ ((__packed__)) {
    RRDENG_CHO_UNALIGNED        = (1 << 0), // set when this metric is not page aligned according to page alignment
    RRDENG_CHO_SET_FIRST_TIME_T = (1 << 1), // set when this metric has unset first_time_t and needs to be updated on the first data collection
} RRDENG_COLLECT_HANDLE_OPTIONS;

struct rrdeng_collect_handle {
    struct metric *metric;
    struct pgc_page *page;
    struct pg_alignment *alignment;
    RRDENG_COLLECT_HANDLE_OPTIONS options;
    uint8_t type;

    uint32_t page_length;                   // keep track of the current page size, to make sure we don't exceed it
    usec_t start_time_ut;
    usec_t end_time_ut;
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

typedef enum {
    RRDENGINE_STATUS_UNINITIALIZED = 0,
    RRDENGINE_STATUS_INITIALIZING,
    RRDENGINE_STATUS_INITIALIZED
} rrdengine_state_t;

enum rrdeng_opcode {
    /* can be used to return empty status or flush the command queue */
    RRDENG_NOOP = 0,

    RRDENG_READ_EXTENT,
    RRDENG_COMMIT_PAGE,
    RRDENG_FLUSH_PAGES,
    RRDENG_SHUTDOWN,
    RRDENG_QUIESCE,
    RRDENG_READ_DF_EXTENT_LIST,
    RRDENG_READ_PAGE_LIST,

    RRDENG_MAX_OPCODE
};

struct rrdeng_read_page {
    struct rrdeng_page_descr *page_cache_descr;
};

struct uuid_list_s {
    uuid_t uuid;
    time_t start_time_t;
};

struct rrdeng_read_extent {
    size_t entries;
    uv_file file;
    uint64_t pos;
    uint64_t size;
//    struct uuid_list_s uuid_list[MAX_PAGES_PER_EXTENT];
//    struct rrdeng_page_descr *page_cache_descr[MAX_PAGES_PER_EXTENT];
    struct completion *completion;
//    int page_count;
};

struct rrdeng_cmd {
    enum rrdeng_opcode opcode;
    void *data;
    union {
        struct rrdeng_read_page read_page;
        struct rrdeng_read_extent read_extent;
        struct completion *completion;
    };
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
    int fileno;
    uv_file file;
    uint64_t pos;
    unsigned bytes;
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
//    struct rrdeng_page_descr descr_read_array[MAX_PAGES_PER_EXTENT];
//    struct uuid_list_s uuid_list[MAX_PAGES_PER_EXTENT];
//    BITMAP256 descr_array_wakeup;
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

    /* file deletion thread */
    uv_thread_t *now_deleting_files;
    unsigned long cleanup_thread_deleting_files; /* set to 0 when now_deleting_files is still running */

    unsigned long running_journal_migration;
    bool run_indexing;

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

typedef enum {
    LOAD_ERRORS_PAGE_FLIPPED_TIME = 0,
    LOAD_ERRORS_PAGE_EQUAL_TIME = 1,
    LOAD_ERRORS_PAGE_ZERO_ENTRIES = 2,
    LOAD_ERRORS_PAGE_UPDATE_ZERO = 3,
    LOAD_ERRORS_PAGE_FLEXY_TIME = 4,
    LOAD_ERRORS_DROPPED_EXTENT = 5,
    LOAD_ERRORS_PAGE_FUTURE_TIME = 6,
} INVALID_PAGE_ID;

struct rrdengine_instance {
    struct rrdengine_worker_config worker_config;
    struct completion rrdengine_completion;
    struct page_cache pg_cache;
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
    unsigned long max_cache_pages;
    unsigned long metric_API_max_producers;

    uint8_t quiesce;   /* set to SET_QUIESCE before shutdown of the engine */
    uint8_t page_type; /* Default page type for this context */

    struct rrdengine_statistics stats;

    struct {
        size_t counter;
        usec_t latest_end_time_ut;
    } load_errors[7];
};

void *dbengine_page_alloc(void);
void dbengine_page_free(void *page);

int init_rrd_files(struct rrdengine_instance *ctx);
void finalize_rrd_files(struct rrdengine_instance *ctx);
void rrdeng_test_quota(struct rrdengine_worker_config *wc);
void rrdeng_worker(void *arg);
void rrdeng_enq_cmd(struct rrdengine_worker_config *wc, struct rrdeng_cmd *cmd);
struct rrdeng_cmd rrdeng_deq_cmd(struct rrdengine_worker_config *wc);
void after_journal_indexing(uv_work_t *req, int status);
void start_journal_indexing(uv_work_t *req);
struct pg_cache_page_index *get_page_index(struct page_cache *pg_cache, uuid_t *uuid);
struct rrdeng_page_descr *get_descriptor(struct pg_cache_page_index *page_index, time_t start_time_s);
void dbengine_load_page_list(struct rrdengine_instance *ctx, struct page_details_control *pdc);

bool page_details_release_and_destroy_if_unreferenced(struct page_details_control *pdc);

#endif /* NETDATA_RRDENGINE_H */

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDENGINE_H
#define NETDATA_RRDENGINE_H

#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif
#include <fcntl.h>
#include <aio.h>
#include <uv.h>
#include <assert.h>
#include <lz4.h>
#include <Judy.h>
#include <openssl/sha.h>
#include <openssl/evp.h>
#include <stdint.h>
#include "../rrd.h"
#include "rrddiskprotocol.h"
#include "rrdenginelib.h"
#include "datafile.h"
#include "journalfile.h"
#include "rrdengineapi.h"
#include "pagecache.h"
#include "rrdenglocking.h"

#ifdef NETDATA_RRD_INTERNALS

#endif /* NETDATA_RRD_INTERNALS */

/* Forward declerations */
struct rrdengine_instance;

#define MAX_PAGES_PER_EXTENT (64) /* TODO: can go higher only when journal supports bigger than 4KiB transactions */

#define RRDENG_FILE_NUMBER_SCAN_TMPL "%1u-%10u"
#define RRDENG_FILE_NUMBER_PRINT_TMPL "%1.1u-%10.10u"


typedef enum {
    RRDENGINE_STATUS_UNINITIALIZED = 0,
    RRDENGINE_STATUS_INITIALIZING,
    RRDENGINE_STATUS_INITIALIZED
} rrdengine_state_t;

enum rrdeng_opcode {
    /* can be used to return empty status or flush the command queue */
    RRDENG_NOOP = 0,

    RRDENG_READ_PAGE,
    RRDENG_READ_EXTENT,
    RRDENG_COMMIT_PAGE,
    RRDENG_FLUSH_PAGES,
    RRDENG_SHUTDOWN,

    RRDENG_MAX_OPCODE
};

struct rrdeng_cmd {
    enum rrdeng_opcode opcode;
    union {
        struct rrdeng_read_page {
            struct rrdeng_page_descr *page_cache_descr;
        } read_page;
        struct rrdeng_read_extent {
            struct rrdeng_page_descr *page_cache_descr[MAX_PAGES_PER_EXTENT];
            int page_count;
        } read_extent;
        struct completion *completion;
    };
};

#define RRDENG_CMD_Q_MAX_SIZE (2048)

struct rrdeng_cmdqueue {
    unsigned head, tail;
    struct rrdeng_cmd cmd_array[RRDENG_CMD_Q_MAX_SIZE];
};

struct extent_io_descriptor {
    uv_fs_t req;
    uv_buf_t iov;
    void *buf;
    uint64_t pos;
    unsigned bytes;
    struct completion *completion;
    unsigned descr_count;
    int release_descr;
    struct rrdeng_page_descr *descr_array[MAX_PAGES_PER_EXTENT];
    Word_t descr_commit_idx_array[MAX_PAGES_PER_EXTENT];
};

struct generic_io_descriptor {
    uv_fs_t req;
    uv_buf_t iov;
    void *buf;
    uint64_t pos;
    unsigned bytes;
    struct completion *completion;
};

struct rrdengine_worker_config {
    struct rrdengine_instance *ctx;

    uv_thread_t thread;
    uv_loop_t* loop;
    uv_async_t async;
    uv_work_t now_deleting;

    /* FIFO command queue */
    uv_mutex_t cmd_mutex;
    uv_cond_t cmd_cond;
    volatile unsigned queue_size;
    struct rrdeng_cmdqueue cmd_queue;
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
};

struct rrdengine_instance {
    rrdengine_state_t rrdengine_state;
    struct rrdengine_worker_config worker_config;
    struct completion rrdengine_completion;
    struct page_cache pg_cache;
    uint8_t global_compress_alg;
    struct transaction_commit_log commit_log;
    struct rrdengine_datafile_list datafiles;
    char dbfiles_path[FILENAME_MAX+1];
    uint64_t disk_space;
    uint64_t max_disk_space;
    unsigned long max_cache_pages;
    unsigned long cache_pages_low_watermark;

    struct rrdengine_statistics stats;
};

extern void sanity_check(void);
extern int init_rrd_files(struct rrdengine_instance *ctx);
extern void rrdeng_test_quota(struct rrdengine_worker_config* wc);
extern void rrdeng_worker(void* arg);
extern void rrdeng_enq_cmd(struct rrdengine_worker_config* wc, struct rrdeng_cmd *cmd);
extern struct rrdeng_cmd rrdeng_deq_cmd(struct rrdengine_worker_config* wc);

#endif /* NETDATA_RRDENGINE_H */
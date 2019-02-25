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
#include "../rrd.h"
#include "rrddiskprotocol.h"
#include "rrdenginelib.h"
#include "datafile.h"
#include "journalfile.h"
#include "rrdengineapi.h"
#include "pagecache.h"

#ifdef NETDATA_RRD_INTERNALS

#endif /* NETDATA_RRD_INTERNALS */

#define MAX_PAGES_PER_EXTENT (64) /* TODO: can go higher only when journal supports bigger than 4KiB transactions */

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
            struct rrdeng_page_cache_descr *page_cache_descr;
        } read_page;
        struct rrdeng_read_extent {
            struct rrdeng_page_cache_descr *page_cache_descr[MAX_PAGES_PER_EXTENT];
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
    struct rrdeng_page_cache_descr *descr_array[MAX_PAGES_PER_EXTENT];
    unsigned descr_commit_idx_array[MAX_PAGES_PER_EXTENT];
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
    uv_thread_t thread;
    uv_loop_t* loop;
    uv_async_t async;

    /* FIFO command queue */
    uv_mutex_t cmd_mutex;
    uv_cond_t cmd_cond;
    volatile unsigned queue_size;
    struct rrdeng_cmdqueue cmd_queue;
};

extern rrdengine_state_t rrdengine_state;
extern struct rrdengine_worker_config worker_config;
extern struct completion rrdengine_completion;

extern void sanity_check(void);
extern int init_rrd_files(void);
extern void rrdeng_worker(void* arg);
extern void rrdeng_enq_cmd(struct rrdengine_worker_config* wc, struct rrdeng_cmd *cmd);
extern struct rrdeng_cmd rrdeng_deq_cmd(struct rrdengine_worker_config* wc);

#endif /* NETDATA_RRDENGINE_H */
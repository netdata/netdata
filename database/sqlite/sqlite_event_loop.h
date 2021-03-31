// SPDX-License-Identifier: GPL-3.0-or-later


#ifndef NETDATA_SQLITE_EVENT_LOOP_H
#define NETDATA_SQLITE_EVENT_LOOP_H
#include <uv.h>
#include "sqlite_functions.h"

enum sqlite_opcode {
    /* can be used to return empty status or flush the command queue */
    SQLITEOP_NOOP = 0,
    SQLITEOP_CLEANUP,
    SQLITEOP_UPD_CHART,
    SQLITEOP_UPD_ALERT,
    SQLITEOP_SHUTDOWN,
    SQLITEOP_MAX_OPCODE
};

struct sqlite_cmd {
    enum sqlite_opcode opcode;
    union {
//        struct rrdeng_read_page {
//            struct rrdeng_page_descr *page_cache_descr;
//        } read_page;
//        struct rrdeng_read_extent {
//            struct rrdeng_page_descr *page_cache_descr[MAX_PAGES_PER_EXTENT];
//            int page_count;
//        } read_extent;
        struct completion *completion;
    };
};

#define SQLITE_CMD_Q_MAX_SIZE (2048)

struct sqlite_cmdqueue {
    unsigned head, tail;
    struct sqlite_cmd cmd_array[SQLITE_CMD_Q_MAX_SIZE];
};

struct sqlite_worker_config {
    //struct rrdengine_instance *ctx;

    uv_thread_t thread;
    uv_loop_t* loop;
    uv_async_t async;

    /* file deletion thread */
    //uv_thread_t *now_deleting_files;
    //unsigned long cleanup_thread_deleting_files; /* set to 0 when now_deleting_files is still running */

    /* dirty page deletion thread */
    //uv_thread_t *now_invalidating_dirty_pages;
    /* set to 0 when now_invalidating_dirty_pages is still running */
    //unsigned long cleanup_thread_invalidating_dirty_pages;
    //unsigned inflight_dirty_pages;

    /* FIFO command queue */
    uv_mutex_t cmd_mutex;
    uv_cond_t cmd_cond;
    volatile unsigned queue_size;
    struct sqlite_cmdqueue cmd_queue;

    //struct extent_cache xt_cache;

    int error;
};

extern void sqlite_worker(void* arg);
extern void sqlite_enq_cmd(struct sqlite_worker_config *wc, struct sqlite_cmd *cmd);
#endif //NETDATA_SQLITE_EVENT_LOOP_H

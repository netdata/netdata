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
    uv_thread_t thread;
    char uuid_str[GUID_LEN + 1];
    uv_loop_t *loop;
    RRDHOST *host;
    uv_async_t async;
    /* FIFO command queue */
    uv_mutex_t cmd_mutex;
    uv_cond_t cmd_cond;
    volatile unsigned queue_size;
    struct sqlite_cmdqueue cmd_queue;
    int error;
};

extern void sqlite_worker(void* arg);
extern void sqlite_enq_cmd(struct sqlite_worker_config *wc, struct sqlite_cmd *cmd);
#endif //NETDATA_SQLITE_EVENT_LOOP_H

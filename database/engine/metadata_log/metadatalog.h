// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_METADATALOG_H
#define NETDATA_METADATALOG_H

#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif
#include "../rrdengine.h"
#include "metadatalogprotocol.h"
#include "logfile.h"
#include "metadatalogapi.h"

/* Forward declerations */
struct metalog_instance;

#define MAX_PAGES_PER_EXTENT (64) /* TODO: can go higher only when journal supports bigger than 4KiB transactions */

#define METALOG_FILE_NUMBER_SCAN_TMPL "%5u-%5u"
#define METALOG_FILE_NUMBER_PRINT_TMPL "%5.5u-%5.5u"


typedef enum {
    METALOG_STATUS_UNINITIALIZED = 0,
    METALOG_STATUS_INITIALIZING,
    METALOG_STATUS_INITIALIZED
} metalog_state_t;

struct metalog_record_io_descr {
    uv_fs_t req;
    uv_buf_t iov;
    BUFFER *buffer;
    uint64_t pos;
    unsigned bytes;
    struct completion *completion;
};

enum metalog_opcode {
    /* can be used to return empty status or flush the command queue */
    METALOG_NOOP = 0,

    METALOG_SHUTDOWN,
    METALOG_COMMIT_CREATION_RECORD,
    METALOG_COMMIT_DELETION_RECORD,

    METALOG_MAX_OPCODE
};

struct metalog_cmd {
    enum metalog_opcode opcode;
    struct metalog_record_io_descr record_io_descr;
};

#define METALOG_CMD_Q_MAX_SIZE (2048)

struct metalog_cmdqueue {
    unsigned head, tail;
    struct metalog_cmd cmd_array[METALOG_CMD_Q_MAX_SIZE];
};

struct metalog_worker_config {
    struct metalog_instance *ctx;

    uv_thread_t thread;
    uv_loop_t *loop;
    uv_async_t async;

    /* metadata log file comapaction thread */
    uv_thread_t *now_compacting_files;
    unsigned long cleanup_thread_compacting_files; /* set to 0 when now_compacting_files is still running */

    /* FIFO command queue */
    uv_mutex_t cmd_mutex;
    uv_cond_t cmd_cond;
    volatile unsigned queue_size;
    struct metalog_cmdqueue cmd_queue;

    int error;
};

/*
 * Debug statistics not used by code logic.
 * They only describe operations since DB engine instance load time.
 */
struct metalog_statistics {
    rrdeng_stats_t io_write_bytes;
    rrdeng_stats_t io_write_requests;
    rrdeng_stats_t io_read_bytes;
    rrdeng_stats_t io_read_requests;
    rrdeng_stats_t io_write_record_bytes;
    rrdeng_stats_t io_write_records;
    rrdeng_stats_t io_read_record_bytes;
    rrdeng_stats_t io_read_records;
    rrdeng_stats_t metadata_logfile_creations;
    rrdeng_stats_t metadata_logfile_deletions;
    rrdeng_stats_t io_errors;
    rrdeng_stats_t fs_errors;
};

struct metalog_instance {
    struct rrdengine_instance *rrdeng_ctx;
    struct metalog_worker_config worker_config;
    struct completion metalog_completion;
    struct metadata_record_commit_log records_log;
    struct metadata_logfile_list metadata_logfiles;
    uint64_t disk_space;
    uint64_t max_disk_space;
    unsigned last_fileno; /* newest index of metadata log file */

    struct metalog_statistics stats;
};

extern int init_metadata_logfiles(struct metalog_instance *ctx);
extern void finalize_metadata_logfiles(struct metalog_instance *ctx);
extern void metalog_test_quota(struct metalog_worker_config *wc);
extern void metalog_worker(void* arg);
extern void metalog_enq_cmd(struct metalog_worker_config *wc, struct metalog_cmd *cmd);
extern struct metalog_cmd metalog_deq_cmd(struct metalog_worker_config *wc);

#endif /* NETDATA_METADATALOG_H */

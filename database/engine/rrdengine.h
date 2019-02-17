// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDENGINE_H
#define NETDATA_RRDENGINE_H

#define _GNU_SOURCE
#include <fcntl.h>
#include <aio.h>
#include <uv.h>
#include <assert.h>
#include <lz4.h>
#include "../rrd.h"

#define STR_HELPER(x) #x
#define STR(x) STR_HELPER(x)

/* Taken from linux kernel */
#define BUILD_BUG_ON(condition) ((void)sizeof(char[1 - 2*!!(condition)]))

#ifdef NETDATA_RRD_INTERNALS

#endif /* NETDATA_RRD_INTERNALS */

#define RRDENG_BLOCK_SIZE (4096)
#define RRDFILE_ALIGNMENT RRDENG_BLOCK_SIZE
#define ALIGN_BYTES_FLOOR(x) ((x / RRDENG_BLOCK_SIZE) * RRDENG_BLOCK_SIZE)
#define ALIGN_BYTES_CEILING(x) (((x + RRDENG_BLOCK_SIZE - 1) / RRDENG_BLOCK_SIZE) * RRDENG_BLOCK_SIZE)

#define RRDENG_MAGIC_SZ (32)
#define RRDENG_DF_MAGIC "netdata-data-file"
#define RRDENG_JF_MAGIC "netdata-journal-file"

#define RRDENG_VER_SZ (16)
#define RRDENG_DF_VER "1.0"
#define RRDENG_JF_VER "1.0"

#define UUID_SZ (16)
#define CHECKSUM_SZ (4) /* CRC32 */

#define RRD_NO_COMPRESSION (0)
#define RRD_LZ4 (1)

#define RRDENG_DF_SB_PADDING_SZ (RRDENG_BLOCK_SIZE - (RRDENG_MAGIC_SZ + RRDENG_VER_SZ + sizeof(uint8_t)))
/*
 * Data file persistent super-block
 */
struct rrdeng_df_sb {
    char magic_number[RRDENG_MAGIC_SZ];
    char version[RRDENG_VER_SZ];
    uint8_t tier;
    uint8_t padding[RRDENG_DF_SB_PADDING_SZ];
} __attribute__ ((packed));

/*
 * Data file page descriptor
 */
struct rrdeng_extent_page_descr {
    uint8_t uuid[UUID_SZ];
    uint32_t page_length;
    uint64_t start_time;
    uint64_t end_time;
} __attribute__ ((packed));

/*
 * Data file extent header
 */
struct rrdeng_df_extent_header {
    uint8_t compression_algorithm;
    uint32_t payload_length;
    uint8_t number_of_pages;
    /* #number_of_pages page descriptors follow */
    struct rrdeng_extent_page_descr descr[];
} __attribute__ ((packed));

/*
 * Data file extent trailer
 */
struct rrdeng_df_extent_trailer {
    uint8_t checksum[CHECKSUM_SZ]; /* CRC32 */
} __attribute__ ((packed));

#define RRDENG_JF_SB_PADDING_SZ (RRDENG_BLOCK_SIZE - (RRDENG_MAGIC_SZ + RRDENG_VER_SZ))
/*
 * Journal file super-block
 */
struct rrdeng_jf_sb {
    char magic_number[RRDENG_MAGIC_SZ];
    char version[RRDENG_VER_SZ];
    uint8_t padding[RRDENG_JF_SB_PADDING_SZ];
} __attribute__ ((packed));

/*
 * Transaction record types
 */
#define STORE_METRIC_DATA   (1)
#define STORE_LOGS          (2) /* reserved */

/*
 * Journal file transaction record header
 */
struct rrdeng_jf_transaction_header {
    uint32_t id;
    uint16_t payload_length;
    uint32_t type       : 4;
    uint32_t reserved   : 28;
} __attribute__ ((packed));

/*
 * Journal file transaction record trailer
 */
struct rrdeng_jf_transaction_trailer {
    uint8_t checksum[CHECKSUM_SZ]; /* CRC32 */
} __attribute__ ((packed));

/*
 * Journal file STORE_METRIC_DATA action
 */
struct rrdeng_jf_store_metric_data {
    uint8_t number_of_pages;
    /* #number_of_pages page descriptors follow */
    struct rrdeng_extent_page_descr descr[];
} __attribute__ ((packed));

/* Page flags */
#define RRD_PAGE_DIRTY          (1LU << 0)
#define RRD_PAGE_LOCKED         (1LU << 1)
#define RRD_PAGE_READ_PENDING   (1LU << 2)
#define RRD_PAGE_WRITE_PENDING  (1LU << 3)
#define RRD_PAGE_POPULATED      (1LU << 4)

#define INVALID_EXTENT_OFFSET (-1)

struct extent_info {
    uint64_t offset;
    uint32_t size;
};

#define INVALID_TIME (0)

struct rrdeng_page_cache_descr {
    void *page;
    uint32_t page_length;
    usec_t start_time;
    usec_t end_time;
    uuid_t *metric; /* never changes */
    struct extent_info extent; /* TODO: move this out of here */
    unsigned long flags;
    void *private;
    struct rrdeng_page_cache_descr *next;

    /* TODO: move waiter logic to concurrency table */
    unsigned refcnt;
    uv_mutex_t mutex; /* always take it after the page cache lock or after the commit lock */
    uv_cond_t cond;
};

#define PAGE_CACHE_MAX_SIZE             (64) /* TODO: Infinity? */
#define PAGE_CACHE_MAX_PAGES            (16)
#define PAGE_CACHE_MAX_COMMITED_PAGES   (16)

/*
 * Mock page cache
 */
struct page_cache {
    /* page descriptor array TODO: tree */
    uv_rwlock_t pg_cache_rwlock; /* page cache lock */
    struct rrdeng_page_cache_descr *page_cache_array[PAGE_CACHE_MAX_SIZE];
    unsigned populated_pages;

    uv_rwlock_t commited_pages_rwlock; /* commit lock */
    unsigned nr_commited_pages;
    struct rrdeng_page_cache_descr *commited_pages[PAGE_CACHE_MAX_COMMITED_PAGES];
};

enum rrdeng_opcode {
    /* can be used to return empty status or flush the command queue */
    RRDENG_NOOP = 0,

    RRDENG_READ_PAGE,
    RRDENG_COMMIT_PAGE,
    RRDENG_FLUSH_PAGES,
    RRDENG_SHUTDOWN,

    RRDENG_MAX_OPCODE
};

struct completion {
    uv_mutex_t mutex;
    uv_cond_t cond;
    volatile unsigned completed;
};

static inline void init_completion(struct completion *p)
{
    p->completed = 0;
    assert(0 == uv_cond_init(&p->cond));
    assert(0 == uv_mutex_init(&p->mutex));
}

static inline void destroy_completion(struct completion *p)
{
    uv_cond_destroy(&p->cond);
    uv_mutex_destroy(&p->mutex);
}

static inline void wait_for_completion(struct completion *p)
{
    uv_mutex_lock(&p->mutex);
    while (0 == p->completed) {
        uv_cond_wait(&p->cond, &p->mutex);
    }
    assert(1 == p->completed);
    uv_mutex_unlock(&p->mutex);
}

static inline void complete(struct completion *p)
{
    uv_mutex_lock(&p->mutex);
    p->completed = 1;
    uv_mutex_unlock(&p->mutex);
    uv_cond_broadcast(&p->cond);
}

struct rrdeng_cmd {
    int opcode;
    union {
        struct rrdeng_page_cache_descr *page_cache_descr;
        struct completion *completion;
    };
};

#define RRDENG_CMD_Q_MAX_SIZE (64)

struct rrdeng_cmdqueue {
    unsigned head, tail;
    struct rrdeng_cmd cmd_array[RRDENG_CMD_Q_MAX_SIZE];
};

#define MAX_PAGES_PER_EXTENT (4)

struct extent_io_descriptor {
    uv_fs_t req;
    uv_buf_t iov;
    void *buf;
    int64_t pos;
    unsigned bytes;
    struct completion *completion;
    unsigned descr_count;
    struct rrdeng_page_cache_descr *descr_array[MAX_PAGES_PER_EXTENT];
    unsigned descr_commit_idx_array[MAX_PAGES_PER_EXTENT];
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

struct rrdengine_datafile {
    uv_fs_t req;
    uv_file file;
    int fd;
    int64_t pos;
};

/* must call once before using anything */
int rrdeng_init(void);

int rrdeng_exit(void);


#endif /* NETDATA_RRDENGINE_H */
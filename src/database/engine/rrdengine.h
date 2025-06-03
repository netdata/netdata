// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDENGINE_H
#define NETDATA_RRDENGINE_H

#include <fcntl.h>
#include <lz4.h>
#include <Judy.h>
#include <openssl/sha.h>
#include <openssl/evp.h>
#include "../rrd.h"
#include "rrddiskprotocol.h"
#include "rrdenginelib.h"
#include "datafile.h"
#include "journalfile.h"
#include "rrdengineapi.h"
#include "pagecache.h"
#include "mrg.h"
#include "cache.h"
#include "pdc.h"
#include "page.h"

#include "daemon/protected-access.h"

extern unsigned rrdeng_pages_per_extent;

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
struct rrdengine_instance;
struct rrdeng_cmd;

#define MAX_PAGES_PER_EXTENT (109) /* TODO: can go higher only when journal supports bigger than 4KiB transactions */
#define DEFAULT_PAGES_PER_EXTENT (109)

#define MAX_EXTENT_UNCOMPRESSED_SIZE (MAX_PAGES_PER_EXTENT * (RRDENG_BLOCK_SIZE + RRDENG_GORILLA_32BIT_BUFFER_SIZE))


#define RRDENG_FILE_NUMBER_SCAN_TMPL "%1u-%10u"
#define RRDENG_FILE_NUMBER_PRINT_TMPL "%1.1u-%10.10u"

typedef struct dbengine_tier_stats  {
    RRDSET *st;
    RRDDIM *rd_space;
    RRDDIM *rd_time;
} DBENGINE_TIER_STATS;

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
    struct rrdengine_instance *ctx;
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
        struct rrdengine_datafile *ptr;
        uint32_t block;     // the block in the datafile. Offset in the datafile is block * RRDENG_BLOCK_SIZE
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
#define pdc_page_status_clear(pd, flag) __atomic_and_fetch(&((od)->status), ~(flag), __ATOMIC_RELEASE)

struct jv2_extents_info {
    uint32_t index;
    uint32_t block;
    unsigned bytes;
    uint32_t number_of_pages;
};

struct jv2_metrics_info {
    nd_uuid_t *uuid;
    uint32_t page_list_header;
    time_t first_time_s;
    time_t last_time_s;
    size_t number_of_pages;
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
};

typedef enum __attribute__ ((__packed__)) {
    RRDENG_COLLECT_HANDLE_OPTION_NONE   = 0,

#ifdef NETDATA_INTERNAL_CHECKS
    RRDENG_1ST_METRIC_WRITER            = (1 << 0),
#endif
} RRDENG_COLLECT_HANDLE_OPTIONS;

typedef enum __attribute__ ((__packed__)) {
    RRDENG_PAGE_PAST_COLLECTION       = (1 << 0),
    RRDENG_PAGE_REPEATED_COLLECTION   = (1 << 1),
    RRDENG_PAGE_BIG_GAP               = (1 << 2),
    RRDENG_PAGE_GAP                   = (1 << 3),
    RRDENG_PAGE_FUTURE_POINT          = (1 << 4),
    RRDENG_PAGE_CREATED_IN_FUTURE     = (1 << 5),
    RRDENG_PAGE_COMPLETED_IN_FUTURE   = (1 << 6),
    RRDENG_PAGE_UNALIGNED             = (1 << 7),
    RRDENG_PAGE_CONFLICT              = (1 << 8),
    RRDENG_PAGE_FULL                  = (1 << 9),
    RRDENG_PAGE_COLLECT_FINALIZE      = (1 << 10),
    RRDENG_PAGE_UPDATE_EVERY_CHANGE   = (1 << 11),
    RRDENG_PAGE_STEP_TOO_SMALL        = (1 << 12),
    RRDENG_PAGE_STEP_UNALIGNED        = (1 << 13),
} RRDENG_COLLECT_PAGE_FLAGS;

struct rrdeng_collect_handle {
    struct storage_collect_handle common; // has to be first item

    RRDENG_COLLECT_PAGE_FLAGS page_flags;
    RRDENG_COLLECT_HANDLE_OPTIONS options;
    uint8_t type;

    struct rrdengine_instance *ctx;
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

struct rrdeng_query_handle {
    struct metric *metric;
    struct pgc_page *page;
    struct rrdengine_instance *ctx;
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
    struct rrdeng_query_handle *prev, *next;
#endif
};

struct rrdeng_query_handle *rrdeng_query_handle_get(void);
void rrdeng_query_handle_release(struct rrdeng_query_handle *handle);

enum rrdeng_opcode {
    /* can be used to return empty status or flush the command queue */
    RRDENG_OPCODE_NOOP = 0,

    RRDENG_OPCODE_QUERY,
    RRDENG_OPCODE_EXTENT_WRITE,
    RRDENG_OPCODE_EXTENT_READ,
    RRDENG_OPCODE_DATABASE_ROTATE,
    RRDENG_OPCODE_JOURNAL_INDEX,
    RRDENG_OPCODE_FLUSH_MAIN,
    RRDENG_OPCODE_EVICT_MAIN,
    RRDENG_OPCODE_EVICT_OPEN,
    RRDENG_OPCODE_EVICT_EXTENT,
    RRDENG_OPCODE_CTX_SHUTDOWN,
    RRDENG_OPCODE_CTX_FLUSH_DIRTY,
    RRDENG_OPCODE_CTX_FLUSH_HOT_DIRTY,
    RRDENG_OPCODE_CTX_QUIESCE,
    RRDENG_OPCODE_CTX_POPULATE_MRG,
    RRDENG_OPCODE_SHUTDOWN_EVLOOP,
    RRDENG_OPCODE_CLEANUP,

    RRDENG_OPCODE_MAX
};

// WORKERS IDS:
// RRDENG_MAX_OPCODE                     : reserved for the cleanup
// RRDENG_MAX_OPCODE + opcode            : reserved for the callbacks of each opcode
// RRDENG_MAX_OPCODE + RRDENG_MAX_OPCODE : reserved for the timer
#define RRDENG_TIMER_CB (RRDENG_OPCODE_MAX + RRDENG_OPCODE_MAX)
#define RRDENG_OPCODES_WAITING             (RRDENG_TIMER_CB + 1)
#define RRDENG_WORKS_DISPATCHED            (RRDENG_TIMER_CB + 2)
#define RRDENG_WORKS_EXECUTING             (RRDENG_TIMER_CB + 3)
#define RRDENG_RETENTION_TIMER_CB          (RRDENG_TIMER_CB + 4)

struct extent_io_data {
    unsigned fileno;
    uint32_t block;
    unsigned bytes;
};

struct extent_io_descriptor {
    struct rrdengine_instance *ctx;
    void *buf;
    uint64_t pos;
    uint32_t descr_count;
    uint32_t bytes;
    uint32_t real_io_size;
    struct wal *wal;
    uv_file file;
    struct page_descr_with_data *descr_array[MAX_PAGES_PER_EXTENT];
    struct rrdengine_datafile *datafile;
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

WAL *wal_get(struct rrdengine_instance *ctx, unsigned size);
void wal_release(WAL *wal);

/*
 * Debug statistics not used by code logic.
 * They only describe operations since DB engine instance load time.
 */
struct rrdengine_statistics {
    PAD64(rrdeng_stats_t) before_decompress_bytes;
    PAD64(rrdeng_stats_t) after_decompress_bytes;
    PAD64(rrdeng_stats_t) before_compress_bytes;
    PAD64(rrdeng_stats_t) after_compress_bytes;

    PAD64(rrdeng_stats_t) io_write_bytes;
    PAD64(rrdeng_stats_t) io_write_requests;
    PAD64(rrdeng_stats_t) io_read_bytes;
    PAD64(rrdeng_stats_t) io_read_requests;

    PAD64(rrdeng_stats_t) datafile_creations;
    PAD64(rrdeng_stats_t) datafile_deletions;
    PAD64(rrdeng_stats_t) journalfile_creations;
    PAD64(rrdeng_stats_t) journalfile_deletions;

    PAD64(rrdeng_stats_t) io_errors;
    PAD64(rrdeng_stats_t) fs_errors;
};

struct rrdeng_global_stats {
    /* I/O errors global counter */
    PAD64(rrdeng_stats_t) global_io_errors;

    /* File-System errors global counter */
    PAD64(rrdeng_stats_t) global_fs_errors;

    /* number of File-Descriptors that have been reserved by dbengine */
    PAD64(rrdeng_stats_t) rrdeng_reserved_file_descriptors;

    /* inability to flush global counters */
    PAD64(rrdeng_stats_t) global_pg_cache_over_half_dirty_events;
    PAD64(rrdeng_stats_t) global_flushing_pressure_page_deletions; /* number of deleted pages */
};

extern struct rrdeng_global_stats global_stats;

typedef struct tier_config_prototype {
    int tier;                                   // the tier of this ctx
    uint8_t page_type;                          // default page type for this context
    uint64_t max_disk_space;                    // the max disk space this ctx is allowed to use
    time_t max_retention_s;                     // The max retention in seconds
    uint8_t disk_percentage;                    // percentage of metadata that contribute towards tier space used
    uint8_t global_compress_alg;                // the wanted compression algorithm
    char dbfiles_path[FILENAME_MAX + 1];

    struct {
        uint32_t uses;
        bool enabled;
        bool is_on_disk;
        SPINLOCK spinlock;
    } _internal;
} TIER_CONFIG_PROTOTYPE;

struct rrdengine_instance {
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

    struct rrdengine_statistics stats;
};

#define ctx_current_disk_space_get(ctx) __atomic_load_n(&(ctx)->atomic.current_disk_space, __ATOMIC_RELAXED)
#define ctx_current_disk_space_increase(ctx, size) __atomic_add_fetch(&(ctx)->atomic.current_disk_space, size, __ATOMIC_RELAXED)
#define ctx_current_disk_space_decrease(ctx, size) __atomic_sub_fetch(&(ctx)->atomic.current_disk_space, size, __ATOMIC_RELAXED)

static inline void ctx_io_read_op_bytes(struct rrdengine_instance *ctx, size_t bytes) {
    __atomic_add_fetch(&ctx->stats.io_read_bytes, bytes, __ATOMIC_RELAXED);
    __atomic_add_fetch(&ctx->stats.io_read_requests, 1, __ATOMIC_RELAXED);
}

static inline void ctx_io_write_op_bytes(struct rrdengine_instance *ctx, size_t bytes) {
    __atomic_add_fetch(&ctx->stats.io_write_bytes, bytes, __ATOMIC_RELAXED);
    __atomic_add_fetch(&ctx->stats.io_write_requests, 1, __ATOMIC_RELAXED);
}

static inline void ctx_io_error(struct rrdengine_instance *ctx) {
    __atomic_add_fetch(&ctx->stats.io_errors, 1, __ATOMIC_RELAXED);
    rrd_stat_atomic_add(&global_stats.global_io_errors, 1);
}

static inline void ctx_fs_error(struct rrdengine_instance *ctx) {
    __atomic_add_fetch(&ctx->stats.fs_errors, 1, __ATOMIC_RELAXED);
    rrd_stat_atomic_add(&global_stats.global_fs_errors, 1);
}

#define ctx_last_fileno_get(ctx) __atomic_load_n(&(ctx)->atomic.last_fileno, __ATOMIC_RELAXED)
#define ctx_last_fileno_increment(ctx) __atomic_add_fetch(&(ctx)->atomic.last_fileno, 1, __ATOMIC_RELAXED)

#define ctx_last_flush_fileno_get(ctx) __atomic_load_n(&(ctx)->atomic.last_flush_fileno, __ATOMIC_RELAXED)
static inline void ctx_last_flush_fileno_set(struct rrdengine_instance *ctx, unsigned fileno) {
    unsigned old_fileno = ctx_last_flush_fileno_get(ctx);

    do {
        if(old_fileno >= fileno)
            return;

    } while(!__atomic_compare_exchange_n(&ctx->atomic.last_flush_fileno, &old_fileno, fileno, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED));
}

#define ctx_is_available_for_queries(ctx) (__atomic_load_n(&(ctx)->quiesce.enabled, __ATOMIC_RELAXED) == false && __atomic_load_n(&(ctx)->quiesce.exit_mode, __ATOMIC_RELAXED) == false)

bool rrdeng_ctx_tier_cap_exceeded(struct rrdengine_instance *ctx);
int init_rrd_files(struct rrdengine_instance *ctx);
void finalize_rrd_files(struct rrdengine_instance *ctx);
bool rrdeng_dbengine_spawn(struct rrdengine_instance *ctx);
void dbengine_event_loop(void *arg);

typedef void (*enqueue_callback_t)(struct rrdeng_cmd *cmd);
typedef void (*dequeue_callback_t)(struct rrdeng_cmd *cmd);

void rrdeng_enqueue_epdl_cmd(struct rrdeng_cmd *cmd);
void rrdeng_dequeue_epdl_cmd(struct rrdeng_cmd *cmd);

typedef struct rrdeng_cmd *(*requeue_callback_t)(void *data);
void rrdeng_req_cmd(requeue_callback_t get_cmd_cb, void *data, STORAGE_PRIORITY priority);

void rrdeng_enq_cmd(struct rrdengine_instance *ctx, enum rrdeng_opcode opcode, void *data,
                struct completion *completion, enum storage_priority priority,
                enqueue_callback_t enqueue_cb, dequeue_callback_t dequeue_cb);

void pdc_route_asynchronously(struct rrdengine_instance *ctx, struct page_details_control *pdc);
void pdc_route_synchronously(struct rrdengine_instance *ctx, struct page_details_control *pdc);
void pdc_route_synchronously_first(struct rrdengine_instance *ctx, struct page_details_control *pdc);

void pdc_acquire(PDC *pdc);
bool pdc_release_and_destroy_if_unreferenced(PDC *pdc, bool worker, bool router);

uint64_t rrdeng_target_data_file_size(struct rrdengine_instance *ctx);

struct page_descr_with_data *page_descriptor_get(void);

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
                                        RRDENG_COLLECT_PAGE_FLAGS flags);
VALIDATED_PAGE_DESCRIPTOR validate_extent_page_descr(const struct rrdeng_extent_page_descr *descr, time_t now_s, uint32_t overwrite_zero_update_every_s, bool have_read_error);
void collect_page_flags_to_buffer(BUFFER *wb, RRDENG_COLLECT_PAGE_FLAGS flags);

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
    struct rrdengine_instance *ctx,
    struct rrdengine_datafile *datafile,
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
uint64_t rrdeng_get_used_disk_space(struct rrdengine_instance *ctx, bool having_lock);
void rrdeng_calculate_tier_disk_space_percentage(void);
uint64_t rrdeng_get_directory_free_bytes_space(struct rrdengine_instance *ctx);
void dbengine_shutdown();
size_t datafile_count(struct rrdengine_instance *ctx, bool with_lock);
struct rrdengine_datafile *get_first_ctx_datafile(struct rrdengine_instance *ctx, bool with_lock);
struct rrdengine_datafile *get_last_ctx_datafile(struct rrdengine_instance *ctx, bool with_lock);
struct rrdengine_datafile *
get_next_datafile(struct rrdengine_datafile *this_datafile, struct rrdengine_instance *ctx, bool with_lock);


#endif /* NETDATA_RRDENGINE_H */

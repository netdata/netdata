// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_METADATA_H
#define NETDATA_SQLITE_METADATA_H

#include "sqlite3.h"
#include "sqlite_functions.h"

#define METADATA_CMD_Q_MAX_SIZE (16384)            // Max queue size; callers will block until there is room
#define METADATA_MAINTENANCE_FIRST_CHECK (1800)     // Maintenance first run after agent startup in seconds
#define METADATA_MAINTENANCE_RETRY (60)             // Retry run if already running Or last run did actual work
#define METADATA_MAINTENANCE_INTERVAL (3600)        // Repeat maintenance after latest successful

#define METADATA_HOST_CHECK_FIRST_CHECK (30)        // Check host for pending metadata within 60 seconds
#define METADATA_HOST_CHECK_INTERVAL (30)           // Repeat host check every 60 seconds
#define METADATA_HOST_CHECK_IMMEDIATE (5)           // Repeat immediate run because we have more metadata to write

#define MAX_METADATA_CLEANUP (500)                  // Maximum metadata write operations (e.g  deletes before retrying)
#define METADATA_MAX_BATCH_SIZE (512)               // Maximum commands to execute before running the event loop
#define METADATA_MAX_TRANSACTION_BATCH (128)        // Maximum commands to add in a transaction

enum metadata_opcode {
    METADATA_DATABASE_NOOP = 0,
    METADATA_DATABASE_TIMER,
    METADATA_ADD_CHART,
    METADATA_ADD_CHART_LABEL,
    METADATA_ADD_DIMENSION,
    METADATA_DEL_DIMENSION,
    METADATA_ADD_DIMENSION_OPTION,
    METADATA_ADD_HOST_SYSTEM_INFO,
    METADATA_ADD_HOST_INFO,
    METADATA_STORE_CLAIM_ID,
    METADATA_STORE_HOST_LABELS,
    METADATA_STORE_BUFFER,

    METADATA_SKIP_TRANSACTION,                      // Dummy -- OPCODES less than this one can be in a tranasction

    METADATA_SCAN_HOSTS,
    METADATA_MAINTENANCE,
    METADATA_SYNC_SHUTDOWN,
    METADATA_UNITTEST,
    // leave this last
    // we need it to check for worker utilization
    METADATA_MAX_ENUMERATIONS_DEFINED
};

#define MAX_PARAM_LIST  (2)
struct metadata_cmd {
    enum metadata_opcode opcode;
    struct completion *completion;
    const void *param[MAX_PARAM_LIST];
};

struct metadata_database_cmdqueue {
    unsigned head, tail;
    struct metadata_cmd cmd_array[METADATA_CMD_Q_MAX_SIZE];
};

typedef enum {
    METADATA_FLAG_CLEANUP           = (1 << 0), // Cleanup is running
    METADATA_FLAG_SCANNING_HOSTS    = (1 << 1), // Scanning of hosts in worker thread
    METADATA_FLAG_SHUTDOWN          = (1 << 2), // Shutting down
} METADATA_FLAG;

#define METADATA_WORKER_BUSY    (METADATA_FLAG_CLEANUP | METADATA_FLAG_SCANNING_HOSTS)

#define metadata_flag_check(target_flags, flag) (__atomic_load_n(&((target_flags)->flags), __ATOMIC_SEQ_CST) & (flag))
#define metadata_flag_set(target_flags, flag)   __atomic_or_fetch(&((target_flags)->flags), (flag), __ATOMIC_SEQ_CST)
#define metadata_flag_clear(target_flags, flag) __atomic_and_fetch(&((target_flags)->flags), ~(flag), __ATOMIC_SEQ_CST)

struct metadata_queue {
    volatile unsigned queue_size;
    struct metadata_database_cmdqueue cmd_queue;
    struct metadata_queue *prev;
    struct metadata_queue *next;
};

struct metadata_wc {
    uv_thread_t thread;
    time_t check_metadata_after;
    time_t check_hosts_after;
    volatile unsigned queue_size;
    uv_loop_t *loop;
    uv_async_t async;
    METADATA_FLAG flags;
    uint64_t row_id;
    uv_timer_t timer_req;
    struct completion init_complete;
    /* FIFO command queue */
    uv_mutex_t cmd_mutex;
    uv_cond_t cmd_cond;
    struct metadata_database_cmdqueue cmd_queue;
};

// To initialize and shutdown
void metadata_sync_init(void);
void metadata_sync_shutdown(void);
void metadata_sync_shutdown_prepare(void);

void metaqueue_dimension_update(RRDDIM *rd);
void metaqueue_chart_update(RRDSET *st);
void metaqueue_dimension_update_flags(RRDDIM *rd);
void metaqueue_host_update_system_info(const char *machine_guid);
void metaqueue_host_update_info(const char *machine_guid);
void metaqueue_delete_dimension_uuid(uuid_t *uuid);
void metaqueue_store_claim_id(uuid_t *host_uuid, uuid_t *claim_uuid);
void metaqueue_store_host_labels(const char *machine_guid);
void metaqueue_chart_labels(RRDSET *st);
void migrate_localhost(uuid_t *host_uuid);
void metaqueue_buffer(BUFFER *buffer);

// UNIT TEST
int metadata_unittest(int);
#endif //NETDATA_SQLITE_METADATA_H

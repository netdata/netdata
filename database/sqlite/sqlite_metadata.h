// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_METADATA_H
#define NETDATA_SQLITE_METADATA_H

#include "sqlite3.h"
#include "sqlite_functions.h"

// SQL statements

#define SQL_STORE_CLAIM_ID  "insert into node_instance " \
    "(host_id, claim_id, date_created) values (@host_id, @claim_id, unixepoch()) " \
    "on conflict(host_id) do update set claim_id = excluded.claim_id;"

#define SQL_DELETE_HOST_LABELS  "DELETE FROM host_label WHERE host_id = @uuid;"

#define STORE_HOST_LABEL                                                                                               \
    "INSERT OR REPLACE INTO host_label (host_id, source_type, label_key, label_value, date_created) VALUES "

#define STORE_CHART_LABEL                                                                                              \
    "INSERT OR REPLACE INTO chart_label (chart_id, source_type, label_key, label_value, date_created) VALUES "

#define STORE_HOST_OR_CHART_LABEL_VALUE "(u2h('%s'), %d,'%s','%s', unixepoch())"

#define DELETE_DIMENSION_UUID   "DELETE FROM dimension WHERE dim_id = @uuid;"

#define SQL_STORE_HOST_INFO "INSERT OR REPLACE INTO host " \
        "(host_id, hostname, registry_hostname, update_every, os, timezone," \
        "tags, hops, memory_mode, abbrev_timezone, utc_offset, program_name, program_version," \
        "entries, health_enabled) " \
        "values (@host_id, @hostname, @registry_hostname, @update_every, @os, @timezone, @tags, @hops, @memory_mode, " \
        "@abbrev_timezone, @utc_offset, @program_name, @program_version, " \
        "@entries, @health_enabled);"

#define SQL_STORE_CHART "insert or replace into chart (chart_id, host_id, type, id, " \
    "name, family, context, title, unit, plugin, module, priority, update_every , chart_type , memory_mode , " \
    "history_entries) values (?1,?2,?3,?4,?5,?6,?7,?8,?9,?10,?11,?12,?13,?14,?15,?16);"

#define SQL_STORE_DIMENSION "INSERT OR REPLACE INTO dimension (dim_id, chart_id, id, name, multiplier, divisor , algorithm) " \
        "VALUES (@dim_id, @chart_id, @id, @name, @multiplier, @divisor, @algorithm);"

#define SELECT_DIMENSION_LIST "SELECT dim_id, rowid FROM dimension WHERE rowid > @row_id"

#define STORE_HOST_INFO "INSERT OR REPLACE INTO host_info (host_id, system_key, system_value, date_created) VALUES "
#define STORE_HOST_INFO_VALUES "(u2h('%s'), '%s','%s', unixepoch())"

#define MIGRATE_LOCALHOST_TO_NEW_MACHINE_GUID                                                                          \
    "UPDATE chart SET host_id = @host_id WHERE host_id in (SELECT host_id FROM host where host_id <> @host_id and hops = 0);"
#define DELETE_NON_EXISTING_LOCALHOST "DELETE FROM host WHERE hops = 0 AND host_id <> @host_id;"
#define DELETE_MISSING_NODE_INSTANCES "DELETE FROM node_instance WHERE host_id NOT IN (SELECT host_id FROM host);"



#define METADATA_CMD_Q_MAX_SIZE (1024)              // Max queue size; callers will block until there is room
#define METADATA_MAINTENANCE_FIRST_CHECK (1800)     // Maintenance first run after agent startup in seconds
#define METADATA_MAINTENANCE_RETRY (60)             // Retry run if already running or last run did actual work
#define METADATA_MAINTENANCE_INTERVAL (3600)        // Repeat maintenance after latest successful

#define METADATA_HOST_CHECK_FIRST_CHECK (5)         // First check for pending metadata
#define METADATA_HOST_CHECK_INTERVAL (30)           // Repeat check for pending metadata
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
void metaqueue_host_update_system_info(RRDHOST *host);
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

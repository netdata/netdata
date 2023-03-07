// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_ACLK_H
#define NETDATA_SQLITE_ACLK_H

#include "sqlite3.h"


#ifndef ACLK_MAX_CHART_BATCH
#define ACLK_MAX_CHART_BATCH    (200)
#endif
#ifndef ACLK_MAX_CHART_BATCH_COUNT
#define ACLK_MAX_CHART_BATCH_COUNT (10)
#endif
#define ACLK_MAX_ALERT_UPDATES  (5)
#define ACLK_DATABASE_CLEANUP_FIRST  (1200)
#define ACLK_DATABASE_CLEANUP_INTERVAL (3600)
#define ACLK_DELETE_ACK_ALERTS_INTERNAL (86400)
#define ACLK_SYNC_QUERY_SIZE 512

struct aclk_completion {
    uv_mutex_t mutex;
    uv_cond_t cond;
    volatile unsigned completed;
};

static inline void init_aclk_completion(struct aclk_completion *p)
{
    p->completed = 0;
    fatal_assert(0 == uv_cond_init(&p->cond));
    fatal_assert(0 == uv_mutex_init(&p->mutex));
}

static inline void destroy_aclk_completion(struct aclk_completion *p)
{
    uv_cond_destroy(&p->cond);
    uv_mutex_destroy(&p->mutex);
}

static inline void wait_for_aclk_completion(struct aclk_completion *p)
{
    uv_mutex_lock(&p->mutex);
    while (0 == p->completed) {
        uv_cond_wait(&p->cond, &p->mutex);
    }
    fatal_assert(1 == p->completed);
    uv_mutex_unlock(&p->mutex);
}

static inline void aclk_complete(struct aclk_completion *p)
{
    uv_mutex_lock(&p->mutex);
    p->completed = 1;
    uv_mutex_unlock(&p->mutex);
    uv_cond_broadcast(&p->cond);
}

extern uv_mutex_t aclk_async_lock;

static inline void uuid_unparse_lower_fix(uuid_t *uuid, char *out)
{
    uuid_unparse_lower(*uuid, out);
    out[8] = '_';
    out[13] = '_';
    out[18] = '_';
    out[23] = '_';
}

#define TABLE_ACLK_ALERT "CREATE TABLE IF NOT EXISTS aclk_alert_%s (sequence_id INTEGER PRIMARY KEY, " \
        "alert_unique_id, date_created, date_submitted, date_cloud_ack, filtered_alert_unique_id NOT NULL, " \
        "unique(alert_unique_id));"

#define INDEX_ACLK_ALERT "CREATE INDEX IF NOT EXISTS aclk_alert_index_%s ON aclk_alert_%s (alert_unique_id);"
enum aclk_database_opcode {
    ACLK_DATABASE_NOOP = 0,

    ACLK_DATABASE_ORPHAN_HOST,
    ACLK_DATABASE_ALARM_HEALTH_LOG,
    ACLK_DATABASE_CLEANUP,
    ACLK_DATABASE_DELETE_HOST,
    ACLK_DATABASE_NODE_INFO,
    ACLK_DATABASE_NODE_STATE,
    ACLK_DATABASE_PUSH_ALERT,
    ACLK_DATABASE_PUSH_ALERT_CONFIG,
    ACLK_DATABASE_PUSH_ALERT_SNAPSHOT,
    ACLK_DATABASE_QUEUE_REMOVED_ALERTS,
    ACLK_DATABASE_NODE_COLLECTORS,
    ACLK_DATABASE_TIMER,

    // leave this last
    // we need it to check for worker utilization
    ACLK_MAX_ENUMERATIONS_DEFINED
};

struct aclk_database_cmd {
    enum aclk_database_opcode opcode;
    void *data;
    void *data_param;
    int count;
    struct aclk_completion *completion;
};

#define ACLK_DATABASE_CMD_Q_MAX_SIZE (1024)

struct aclk_database_cmdqueue {
    unsigned head, tail;
    struct aclk_database_cmd cmd_array[ACLK_DATABASE_CMD_Q_MAX_SIZE];
};

struct aclk_database_worker_config {
    uv_thread_t thread;
    char uuid_str[GUID_LEN + 1];
    char node_id[GUID_LEN + 1];
    char host_guid[GUID_LEN + 1];
    char *hostname;                 // hostname to avoid constant lookups
    time_t cleanup_after;           // Start a cleanup after this timestamp
    time_t startup_time;           // When the sync thread started
    uint64_t alerts_batch_id; // batch id for alerts to use
    uint64_t alerts_start_seq_id; // cloud has asked to start streaming from
    uint64_t alert_sequence_id; // last alert sequence_id
    int pause_alert_updates;
    uint32_t chart_payload_count;
    uint64_t alerts_snapshot_id; //will contain the snapshot_id value if snapshot was requested
    uint64_t alerts_ack_sequence_id; //last sequence_id ack'ed from cloud via sendsnapshot message
    uv_loop_t *loop;
    RRDHOST *host;
    uv_async_t async;
    /* FIFO command queue */
    uv_mutex_t cmd_mutex;
    uv_cond_t cmd_cond;
    volatile unsigned queue_size;
    struct aclk_database_cmdqueue cmd_queue;
    int alert_updates;
    int node_info_send;
    time_t node_collectors_send;
    volatile unsigned is_shutting_down;
    volatile unsigned is_orphan;
    struct aclk_database_worker_config  *next;
};

static inline RRDHOST *find_host_by_node_id(char *node_id)
{
    uuid_t node_uuid;
    if (unlikely(!node_id))
        return NULL;

    if (uuid_parse(node_id, node_uuid))
        return NULL;

    rrd_rdlock();
    RRDHOST *host, *ret = NULL;
    rrdhost_foreach_read(host) {
        if (host->node_id && !(uuid_compare(*host->node_id, node_uuid))) {
            ret = host;
            break;
        }
    }
    rrd_unlock();

    return ret;
}


extern sqlite3 *db_meta;

int aclk_database_enq_cmd_noblock(struct aclk_database_worker_config *wc, struct aclk_database_cmd *cmd);
void aclk_database_enq_cmd(struct aclk_database_worker_config *wc, struct aclk_database_cmd *cmd);
void sql_create_aclk_table(RRDHOST *host, uuid_t *host_uuid, uuid_t *node_id);
void sql_aclk_sync_init(void);
int claimed();
void aclk_sync_exit_all();
void schedule_node_info_update(RRDHOST *host);
struct aclk_database_worker_config *find_inactive_wc_by_node_id(char *node_id);
#endif //NETDATA_SQLITE_ACLK_H

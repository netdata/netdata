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

static inline void uuid_unparse_lower_fix(uuid_t *uuid, char *out)
{
    uuid_unparse_lower(*uuid, out);
    out[8] = '_';
    out[13] = '_';
    out[18] = '_';
    out[23] = '_';
}

static inline int claimed()
{
    return localhost->aclk_state.claimed_id != NULL;
}


#define TABLE_ACLK_ALERT "CREATE TABLE IF NOT EXISTS aclk_alert_%s (sequence_id INTEGER PRIMARY KEY, " \
        "alert_unique_id, date_created, date_submitted, date_cloud_ack, filtered_alert_unique_id NOT NULL, " \
        "unique(alert_unique_id));"

#define INDEX_ACLK_ALERT "CREATE INDEX IF NOT EXISTS aclk_alert_index_%s ON aclk_alert_%s (alert_unique_id);"
enum aclk_database_opcode {
    ACLK_DATABASE_NOOP = 0,

    ACLK_DATABASE_CLEANUP,
    ACLK_DATABASE_DELETE_HOST,
    ACLK_DATABASE_NODE_STATE,
    ACLK_DATABASE_PUSH_ALERT,
    ACLK_DATABASE_PUSH_ALERT_CONFIG,
    ACLK_DATABASE_PUSH_ALERT_SNAPSHOT,
    ACLK_DATABASE_PUSH_ALERT_CHECKPOINT,
    ACLK_DATABASE_QUEUE_REMOVED_ALERTS,
    ACLK_DATABASE_TIMER,

    // leave this last
    // we need it to check for worker utilization
    ACLK_MAX_ENUMERATIONS_DEFINED
};

struct aclk_database_cmd {
    enum aclk_database_opcode opcode;
    void *param[2];
    struct completion *completion;
};

#define ACLK_DATABASE_CMD_Q_MAX_SIZE (1024)

struct aclk_database_cmdqueue {
    unsigned head, tail;
    struct aclk_database_cmd cmd_array[ACLK_DATABASE_CMD_Q_MAX_SIZE];
};

struct aclk_sync_host_config {
    RRDHOST *host;
    int alert_updates;
    int alert_checkpoint_req;
    int alert_queue_removed;
    time_t node_info_send_time;
    time_t node_collectors_send;
    char uuid_str[UUID_STR_LEN];
    char node_id[UUID_STR_LEN];
    char *alerts_snapshot_uuid;        // will contain the snapshot_uuid value if snapshot was requested
};

extern sqlite3 *db_meta;

int aclk_database_enq_cmd_noblock(struct aclk_database_cmd *cmd);
void sql_create_aclk_table(RRDHOST *host, uuid_t *host_uuid, uuid_t *node_id);
void sql_aclk_sync_init(void);
void aclk_push_alert_config(const char *node_id, const char *config_hash);
void aclk_push_node_alert_snapshot(const char *node_id);
void aclk_push_node_health_log(const char *node_id);
void aclk_push_node_removed_alerts(const char *node_id);
void schedule_node_info_update(RRDHOST *host);

#endif //NETDATA_SQLITE_ACLK_H

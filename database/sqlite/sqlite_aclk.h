// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_ACLK_H
#define NETDATA_SQLITE_ACLK_H

#include "sqlite3.h"

#include "../../aclk/schema-wrappers/chart_stream.h"

#define ACLK_MAX_CHART_UPDATES  (5)

static inline void uuid_unparse_lower_fix(uuid_t *uuid, char *out)
{
    uuid_unparse_lower(*uuid, out);
    out[8] = '_';
    out[13] = '_';
    out[18] = '_';
    out[23] = '_';
}

static inline char *get_str_from_uuid(uuid_t *uuid)
{
    char uuid_str[GUID_LEN + 1];
    if (unlikely(!uuid)) {
        uuid_t zero_uuid;
        uuid_clear(zero_uuid);
        uuid_unparse_lower(zero_uuid, uuid_str);
    }
    else
        uuid_unparse_lower(*uuid, uuid_str);
    return strdupz(uuid_str);
}

#define TABLE_ACLK_CHART "CREATE TABLE IF NOT EXISTS aclk_chart_%s (sequence_id INTEGER PRIMARY KEY, " \
        "date_created, date_updated, date_submitted, status, uuid, type, unique_id, " \
        "update_count default 1, unique(uuid, status));"

#define TABLE_ACLK_CHART_PAYLOAD "CREATE TABLE IF NOT EXISTS aclk_chart_payload_%s (unique_id BLOB PRIMARY KEY, " \
        "uuid, claim_id, type, date_created, payload);"

#define TRIGGER_ACLK_CHART_PAYLOAD "CREATE TRIGGER IF NOT EXISTS aclk_tr_chart_payload_%s " \
        "after insert on aclk_chart_payload_%s " \
        "begin insert into aclk_chart_%s (uuid, unique_id, type, status, date_created) values " \
        " (new.uuid, new.unique_id, new.type, 'pending', strftime('%%s')) on conflict(uuid, status) " \
        " do update set unique_id = new.unique_id, update_count = update_count + 1; " \
        "end;"

#define TABLE_ACLK_ALERT "CREATE TABLE IF NOT EXISTS aclk_alert_%s (sequence_id INTEGER PRIMARY KEY, " \
        "alert_unique_id, date_created, date_submitted); "

enum aclk_database_opcode {
    ACLK_DATABASE_NOOP = 0,
    ACLK_DATABASE_CLEANUP,
    ACLK_DATABASE_TIMER,
    ACLK_DATABASE_ADD_CHART,
    ACLK_DATABASE_ADD_DIMENSION,
    ACLK_DATABASE_PUSH_CHART,
    ACLK_DATABASE_PUSH_CHART_CONFIG,
    ACLK_DATABASE_CHART_ACK,
    ACLK_DATABASE_RESET_CHART,
    ACLK_DATABASE_STATUS_CHART,
    ACLK_DATABASE_RESET_NODE,
    ACLK_DATABASE_SHUTDOWN,
    ACLK_DATABASE_ADD_ALERT,
    ACLK_DATABASE_NODE_INFO,
    ACLK_DATABASE_DEDUP_CHART,
    ACLK_DATABASE_UPD_STATS,
    ACLK_DATABASE_SYNC_CHART_SEQ,
    ACLK_DATABASE_MAX_OPCODE
};

struct aclk_chart_payload_t {
    long sequence_id;
    long last_sequence_id;
    char *payload;
    struct aclk_chart_payload_t *next;
};


struct aclk_database_cmd {
    enum aclk_database_opcode opcode;
    void *data;
    void *data_param;
    int count;
    uint64_t param1;
    struct completion *completion;
};

#define ACLK_DATABASE_CMD_Q_MAX_SIZE (2048)

struct aclk_database_cmdqueue {
    unsigned head, tail;
    struct aclk_database_cmd cmd_array[ACLK_DATABASE_CMD_Q_MAX_SIZE];
};

struct aclk_database_worker_config {
    uv_thread_t thread;
    char uuid_str[GUID_LEN + 1];
    char host_guid[GUID_LEN + 1];
    uint64_t chart_sequence_id;     // last chart_sequence_id
    time_t chart_timestamp;         // last chart timestamp
    uint64_t batch_id;    // batch id to use
    uv_loop_t *loop;
    RRDHOST *host;
    uv_async_t async;
    /* FIFO command queue */
    uv_mutex_t cmd_mutex;
    uv_cond_t cmd_cond;
    volatile unsigned queue_size;
    struct aclk_database_cmdqueue cmd_queue;
    int error;
    int chart_updates;
    int alert_updates;
};

static inline RRDHOST *find_host_by_node_id(char *node_id)
{
    uuid_t node_uuid;
    if (unlikely(!node_id))
        return NULL;

    uuid_parse(node_id, node_uuid);

    RRDHOST *host = localhost;
    while(host) {
        if (host->node_id && !(uuid_compare(*host->node_id, node_uuid)))
            return host;
        host = host->next;
    }
    return NULL;
}


//extern void sqlite_worker(void* arg);
extern void aclk_database_enq_cmd(struct aclk_database_worker_config *wc, struct aclk_database_cmd *cmd);

extern void sql_queue_chart_to_aclk(RRDSET *st, int cmd);
extern void sql_queue_dimension_to_aclk(RRDDIM *rd, int cmd);
extern void sql_queue_alarm_to_aclk(RRDHOST *host, ALARM_ENTRY *ae);
extern sqlite3 *db_meta;
extern void sql_create_aclk_table(RRDHOST *host, uuid_t *host_uuid);
int aclk_add_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
int aclk_add_dimension_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
int aclk_push_chart_config_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
//int aclk_add_alert_event(RRDHOST *host, ALARM_ENTRY *ae, struct completion *completion);
int aclk_add_alert_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
//void aclk_fetch_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void sql_reset_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void sql_build_node_info(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void aclk_reset_chart_event(char *node_id, uint64_t last_sequence_id);
void aclk_status_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void aclk_reset_node_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void aclk_push_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void sql_drop_host_aclk_table_list(uuid_t *host_uuid);
void aclk_ack_chart_sequence_id(char *node_id, uint64_t last_sequence_id);
void aclk_get_chart_config(char **hash_id_list);
void aclk_start_streaming(char *node_id, uint64_t seq_id, time_t created_at, uint64_t batch_id);
void aclk_start_alert_streaming(char *node_id);
void sql_aclk_drop_all_table_list();
void sql_set_chart_ack(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void aclk_submit_param_command(char *node_id, enum aclk_database_opcode aclk_command, uint64_t param);
extern void aclk_set_architecture(int mode);
char **build_dimension_payload_list(RRDSET *st, size_t **payload_list_size, size_t  *total);
void sql_chart_deduplicate(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void sql_get_last_chart_sequence(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void sql_update_metric_statistics(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
#endif //NETDATA_SQLITE_ACLK_H

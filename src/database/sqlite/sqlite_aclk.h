// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_ACLK_H
#define NETDATA_SQLITE_ACLK_H

#define ACLK_MAX_ALERT_UPDATES  "50"
#define ACLK_SYNC_QUERY_SIZE 512

static inline int uuid_parse_fix(char *in, nd_uuid_t uuid)
{
    in[8] = '-';
    in[13] = '-';
    in[18] = '-';
    in[23] = '-';
    return uuid_parse(in, uuid);
}

enum aclk_database_opcode {
    ACLK_DATABASE_NOOP = 0,
    ACLK_DATABASE_NODE_STATE,
    ACLK_DATABASE_PUSH_ALERT,
    ACLK_DATABASE_PUSH_ALERT_CONFIG,
    ACLK_DATABASE_NODE_UNREGISTER,
    ACLK_MQTT_WSS_CLIENT,
    ACLK_QUERY_EXECUTE,
    ACLK_QUERY_EXECUTE_SYNC,
    ACLK_QUERY_BATCH_ADD,
    ACLK_QUERY_BATCH_EXECUTE,

    // leave this last
    // we need it to check for worker utilization
    ACLK_MAX_ENUMERATIONS_DEFINED
};

struct aclk_database_cmd {
    enum aclk_database_opcode opcode;
    void *param[2];
    struct aclk_database_cmd *prev, *next;
};

typedef struct aclk_sync_cfg_t {
    RRDHOST *host;
    uv_timer_t timer;
    bool timer_initialized;
    int8_t send_snapshot;
    bool stream_alerts;
    int alert_count;
    int snapshot_count;
    int checkpoint_count;
    time_t node_info_send_time;
    time_t node_collectors_send;
    char node_id[UUID_STR_LEN];
} aclk_sync_cfg_t;

void create_aclk_config(RRDHOST *host, nd_uuid_t *host_uuid, nd_uuid_t *node_id);
void destroy_aclk_config(RRDHOST *host);
void sql_aclk_sync_init(void);
void aclk_push_alert_config(const char *node_id, const char *config_hash);
void schedule_node_state_update(RRDHOST *host, uint64_t delay);
void unregister_node(const char *machine_guid);

#endif //NETDATA_SQLITE_ACLK_H

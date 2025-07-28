// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef ACLK_H
#define ACLK_H

#include "database/rrd.h"

#include "aclk_util.h"
//#include "aclk_rrdhost_state.h"

#include "https_client.h"

// How many MQTT PUBACKs we need to get to consider connection
// stable for the purposes of TBEB (truncated binary exponential backoff)
#define ACLK_PUBACKS_CONN_STABLE 3

typedef enum {
    ACLK_NO_DISCONNECT = 0,
    ACLK_CLOUD_DISCONNECT = 1,
    ACLK_RELOAD_CONF = 2,
    ACLK_PING_TIMEOUT = 3
} ACLK_DISCONNECT_ACTION;

typedef enum {
    ACLK_STATUS_CONNECTED = 0,

    // ND_SOCK_ERR_XXX is included here
    // HTTPS_CLIENT_RESP_XXX is included here

    ACLK_STATUS_OFFLINE                   = HTTPS_CLIENT_RESP_MAX,
    ACLK_STATUS_DISABLED,
    ACLK_STATUS_CANT_CONNECT_NO_CLOUD_URL,
    ACLK_STATUS_CANT_CONNECT_INVALID_CLOUD_URL,
    ACLK_STATUS_BLOCKED,
    ACLK_STATUS_NO_OLD_PROTOCOL,
    ACLK_STATUS_NO_PROTOCOL_CAPABILITY,
    ACLK_STATUS_INVALID_ENV_AUTH_URL,
    ACLK_STATUS_INVALID_ENV_TRANSPORT_IDX,
    ACLK_STATUS_INVALID_ENV_TRANSPORT_URL,
    ACLK_STATUS_NO_LWT_TOPIC,

    // disconnection reasons
    ACLK_STATUS_OFFLINE_CLOUD_REQUESTED_DISCONNECT,
    ACLK_STATUS_OFFLINE_PING_TIMEOUT,
    ACLK_STATUS_OFFLINE_RELOADING_CONFIG,
    ACLK_STATUS_OFFLINE_POLL_ERROR,
    ACLK_STATUS_OFFLINE_CLOSED_BY_REMOTE,
    ACLK_STATUS_OFFLINE_SOCKET_ERROR,
    ACLK_STATUS_OFFLINE_MQTT_PROTOCOL_ERROR,
    ACLK_STATUS_OFFLINE_WS_PROTOCOL_ERROR,
    ACLK_STATUS_OFFLINE_MESSAGE_TOO_BIG,

} ACLK_STATUS;

extern ACLK_STATUS aclk_status;
extern const char *aclk_cloud_base_url;
const char *aclk_status_to_string(void);

extern int aclk_ctx_based;
extern int aclk_disable_runtime;
//extern int aclk_stats_enabled;
extern int aclk_kill_link;

bool aclk_online(void);
bool aclk_online_for_contexts(void);
bool aclk_online_for_alerts(void);
bool aclk_online_for_nodes(void);

void aclk_config_get_query_scope(void);
bool aclk_query_scope_has(HTTP_ACL acl);

extern time_t last_conn_time_mqtt;
extern time_t last_conn_time_appl;
extern time_t last_disconnect_time;
extern time_t next_connection_attempt;
extern float last_backoff_value;

extern usec_t aclk_session_us;
extern time_t aclk_session_sec;

extern time_t aclk_block_until;

extern int aclk_connection_counter;
extern ACLK_DISCONNECT_ACTION disconnect_req;

extern struct aclk_shared_state {
    // To wait for `disconnect` message PUBACK
    // when shutting down
    // at the same time if > 0 we know link is
    // shutting down
    int mqtt_shutdown_msg_id;
    int mqtt_shutdown_msg_rcvd;
} aclk_shared_state;

void aclk_host_state_update(RRDHOST *host, int live, int queryable);
bool aclk_host_state_update_auto(RRDHOST *host);

void aclk_send_node_instances();

void aclk_send_bin_msg(char *msg, size_t msg_len, enum aclk_topics subtopic, const char *msgname);

char *aclk_state(void);
char *aclk_state_json(void);
void add_aclk_host_labels(void);

struct mqtt_wss_stats aclk_statistics(void);
void aclk_status_set(ACLK_STATUS status);

#endif /* ACLK_H */

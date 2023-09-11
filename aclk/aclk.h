// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef ACLK_H
#define ACLK_H

#include "daemon/common.h"

#ifdef ENABLE_ACLK
#include "aclk_util.h"
#include "aclk_rrdhost_state.h"

// How many MQTT PUBACKs we need to get to consider connection
// stable for the purposes of TBEB (truncated binary exponential backoff)
#define ACLK_PUBACKS_CONN_STABLE 3
#endif /* ENABLE_ACLK */

typedef enum __attribute__((packed)) {
    ACLK_STATUS_CONNECTED = 0,
    ACLK_STATUS_NONE,
    ACLK_STATUS_DISABLED,
    ACLK_STATUS_NO_CLOUD_URL,
    ACLK_STATUS_INVALID_CLOUD_URL,
    ACLK_STATUS_NOT_CLAIMED,
    ACLK_STATUS_ENV_ENDPOINT_UNREACHABLE,
    ACLK_STATUS_ENV_RESPONSE_NOT_200,
    ACLK_STATUS_ENV_RESPONSE_EMPTY,
    ACLK_STATUS_ENV_RESPONSE_NOT_JSON,
    ACLK_STATUS_ENV_FAILED,
    ACLK_STATUS_BLOCKED,
    ACLK_STATUS_NO_OLD_PROTOCOL,
    ACLK_STATUS_NO_PROTOCOL_CAPABILITY,
    ACLK_STATUS_INVALID_ENV_AUTH_URL,
    ACLK_STATUS_INVALID_ENV_TRANSPORT_IDX,
    ACLK_STATUS_INVALID_ENV_TRANSPORT_URL,
    ACLK_STATUS_INVALID_OTP,
    ACLK_STATUS_NO_LWT_TOPIC,
} ACLK_STATUS;

extern ACLK_STATUS aclk_status;
extern const char *aclk_cloud_base_url;
const char *aclk_status_to_string(void);

extern int aclk_connected;
extern int aclk_ctx_based;
extern int aclk_disable_runtime;
extern int aclk_stats_enabled;
extern int aclk_kill_link;

extern time_t last_conn_time_mqtt;
extern time_t last_conn_time_appl;
extern time_t last_disconnect_time;
extern time_t next_connection_attempt;
extern float last_backoff_value;

extern usec_t aclk_session_us;
extern time_t aclk_session_sec;

extern time_t aclk_block_until;

extern int aclk_connection_counter;
extern int disconnect_req;

#ifdef ENABLE_ACLK
void *aclk_main(void *ptr);

extern netdata_mutex_t aclk_shared_state_mutex;
#define ACLK_SHARED_STATE_LOCK netdata_mutex_lock(&aclk_shared_state_mutex)
#define ACLK_SHARED_STATE_UNLOCK netdata_mutex_unlock(&aclk_shared_state_mutex)

extern struct aclk_shared_state {
    // To wait for `disconnect` message PUBACK
    // when shutting down
    // at the same time if > 0 we know link is
    // shutting down
    int mqtt_shutdown_msg_id;
    int mqtt_shutdown_msg_rcvd;
} aclk_shared_state;

void aclk_host_state_update(RRDHOST *host, int cmd);
void aclk_send_node_instances(void);

void aclk_send_bin_msg(char *msg, size_t msg_len, enum aclk_topics subtopic, const char *msgname);

#endif /* ENABLE_ACLK */

char *aclk_state(void);
char *aclk_state_json(void);
void add_aclk_host_labels(void);
void aclk_queue_node_info(RRDHOST *host, bool immediate);

#endif /* ACLK_H */

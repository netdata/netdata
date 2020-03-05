// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_AGENT_CLOUD_LINK_H
#define NETDATA_AGENT_CLOUD_LINK_H

#include "../daemon/common.h"
#include "mqtt.h"

#define ACLK_VERSION 1
#define ACLK_THREAD_NAME "ACLK_Query"
#define ACLK_CHART_TOPIC "chart"
#define ACLK_ALARMS_TOPIC "alarms"
#define ACLK_METADATA_TOPIC "meta"
#define ACLK_COMMAND_TOPIC "cmd"
#define ACLK_TOPIC_STRUCTURE "/agent/%s"

#define ACLK_MAX_BACKOFF_DELAY 1024 // maximum backoff delay in seconds

#define ACLK_INITIALIZATION_WAIT 60      // Wait for link to initialize in seconds (per msg)
#define ACLK_INITIALIZATION_SLEEP_WAIT 1 // Wait time @ spin lock for MQTT initialization in seconds
#define ACLK_QOS 1
#define ACLK_PING_INTERVAL 60
#define ACLK_LOOP_TIMEOUT 5 // seconds to wait for operations in the library loop

#define ACLK_MAX_TOPIC 255

#define ACLK_RECONNECT_DELAY 1 // reconnect delay -- with backoff stragegy fow now
#define ACLK_STABLE_TIMEOUT 10 // Minimum delay to mark AGENT as stable
#define ACLK_DEFAULT_PORT 9002
#define ACLK_DEFAULT_HOST "localhost"

struct aclk_request {
    char *type_id;
    char *msg_id;
    char *callback_topic;
    char *payload;
    int version;
};

typedef enum aclk_cmd {
    ACLK_CMD_CLOUD,
    ACLK_CMD_ONCONNECT,
    ACLK_CMD_INFO,
    ACLK_CMD_CHART,
    ACLK_CMD_CHARTDEL,
    ACLK_CMD_ALARM,
    ACLK_CMD_ALARMS,
    ACLK_CMD_MAX
} ACLK_CMD;

typedef enum aclk_metadata_state {
    ACLK_METADATA_REQUIRED,
    ACLK_METADATA_CMD_QUEUED,
    ACLK_METADATA_SENT
} ACLK_METADATA_STATE;

typedef enum agent_state {
    AGENT_INITIALIZING,
    AGENT_STABLE
} AGENT_STATE;

typedef enum aclk_init_action { ACLK_INIT, ACLK_REINIT } ACLK_INIT_ACTION;

void *aclk_main(void *ptr);

#define NETDATA_ACLK_HOOK                                                                                              \
    { .name = "ACLK_Main",                                                                                             \
      .config_section = NULL,                                                                                          \
      .config_name = NULL,                                                                                             \
      .enabled = 1,                                                                                                    \
      .thread = NULL,                                                                                                  \
      .init_routine = NULL,                                                                                            \
      .start_routine = aclk_main },

extern int aclk_send_message(char *sub_topic, char *message, char *msg_id);

//int     aclk_init();
//char    *get_base_topic();

extern char *is_agent_claimed(void);
char *create_uuid();

// callbacks for agent cloud link
int aclk_subscribe(char *topic, int qos);
void aclk_shutdown();
int cloud_to_agent_parse(JSON_ENTRY *e);
void aclk_disconnect();
void aclk_connect();
int aclk_send_metadata();
int aclk_send_info_metadata();
int aclk_wait_for_initialization();
char *create_publish_base_topic();

int aclk_send_single_chart(char *host, char *chart);
int aclk_queue_query(char *token, char *data, char *msg_type, char *query, int run_after, int internal, ACLK_CMD cmd);
struct aclk_query *
aclk_query_find(char *token, char *data, char *msg_id, char *query, ACLK_CMD cmd, struct aclk_query **last_query);
int aclk_update_chart(RRDHOST *host, char *chart_name, ACLK_CMD aclk_cmd);
int aclk_update_alarm(RRDHOST *host, ALARM_ENTRY *ae);
void aclk_create_header(BUFFER *dest, char *type, char *msg_id);
int aclk_handle_cloud_request(char *payload);
int aclk_submit_request(struct aclk_request *);
void aclk_add_collector(const char *hostname, const char *plugin_name, const char *module_name);
void aclk_del_collector(const char *hostname, const char *plugin_name, const char *module_name);
void aclk_alarm_reload();
void aclk_send_alarm_metadata();
int aclk_execute_query(struct aclk_query *query);
BUFFER *aclk_encode_response(BUFFER *contents);
unsigned long int aclk_reconnect_delay(int mode);
extern void health_alarm_entry2json_nolock(BUFFER *wb, ALARM_ENTRY *ae, RRDHOST *host);
void aclk_single_update_enable();
void aclk_single_update_disable();

#endif //NETDATA_AGENT_CLOUD_LINK_H

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_AGENT_CLOUD_LINK_H
#define NETDATA_AGENT_CLOUD_LINK_H

#include "../daemon/common.h"
#include "mqtt.h"

#define ACLK_JSON_IN_MSGID "msg-id"
#define ACLK_JSON_IN_TYPE "type"
#define ACLK_JSON_IN_VERSION "version"
#define ACLK_JSON_IN_TOPIC "callback-topic"
#define ACLK_JSON_IN_URL "payload"


#define ACLK_MSG_TYPE_CHART "chart"
#define ACLK_ALARMS_TOPIC "alarms"
#define ACLK_METADATA_TOPIC "meta"
#define ACLK_COMMAND_TOPIC "cmd"
#define ACLK_TOPIC_STRUCTURE "/agent/%s"

#define ACLK_STARTUP_WAIT 30             // Seconds to wait before establishing initialization process
#define ACLK_INITIALIZATION_WAIT 60      // Wait for link to initialize in seconds (per msg)
#define ACLK_INITIALIZATION_SLEEP_WAIT 1 // Wait time @ spin lock for MQTT initialization in seconds
#define ACLK_QOS 1
#define ACLK_PING_INTERVAL 60
#define ACLK_LOOP_TIMEOUT 5        // seconds to wait for operations in the library loop

#define ACLK_MAX_TOPIC  255

#define ACLK_RECONNECT_DELAY 1          // reconnect delay -- with backoff stragegy fow now
#define ACLK_MAX_RECONNECT_DELAY 120
#define ACLK_VERSION "1"

#define CONFIG_SECTION_ACLK "agent_cloud_link"

struct aclk_request {
    char    *type_id;
    char    *msg_id;
    char    *topic;
    char    *url;
    int     version;
};


typedef enum publish_topic_action {
    PUBLICH_TOPIC_GET,
    PUBLICH_TOPIC_FREE,
    PUBLICH_TOPIC_REBUILD
} PUBLISH_TOPIC_ACTION;

typedef enum aclk_init_action {
    ACLK_INIT,
    ACLK_REINIT
} ACLK_INIT_ACTION;


#define GET_PUBLISH_BASE_TOPIC get_publish_base_topic(0)
#define FREE_PUBLISH_BASE_TOPIC get_publish_base_topic(1)
#define REBUILD_PUBLISH_BASE_TOPIC get_publish_base_topic(2)

void *aclk_main(void *ptr);

#define NETDATA_ACLK_HOOK \
    { \
        .name = "ACLK_Main", \
        .config_section = NULL, \
        .config_name = NULL, \
        .enabled = 1, \
        .thread = NULL, \
        .init_routine = NULL, \
        .start_routine = aclk_main \
    },

extern int aclk_send_message(char *sub_topic, char *message);

int     aclk_init();
char    *get_base_topic();

extern char *is_agent_claimed(void);

// callbacks for agent cloud link
int aclk_subscribe(char  *topic, int qos);
void aclk_shutdown();
//void aclk_message_callback(struct mosquitto *moqs, void *obj, const struct mosquitto_message *msg);

int cloud_to_agent_parse(JSON_ENTRY *e);
void aclk_disconnect(void *conn);
void aclk_connect(void *conn);
void aclk_create_metadata_message(BUFFER *dest, char *type, char *msg_id, BUFFER *contents);
int aclk_send_metadata();
int aclk_wait_for_initialization();
//int aclk_send_charts(RRDHOST *host, BUFFER *wb);
int aclk_send_single_chart(char *host, char *chart);
int aclk_queue_query(char *token, char *data, char *msg_type, char *query, int run_after, int internal);
struct aclk_query  *aclk_query_find(char *token, char *data, char *msg_id, char *query);
//void aclk_rrdset2json(RRDSET *st, BUFFER *wb, char *hostname, int is_slave);
int aclk_update_chart(RRDHOST *host, char *chart_name);
int aclk_update_alarm(RRDHOST *host, char *alarm_name);
void aclk_create_header(BUFFER *dest, char *type, char *msg_id);
int aclk_handle_cloud_request(char *payload);
int aclk_submit_request(struct aclk_request *);
#endif //NETDATA_AGENT_CLOUD_LINK_H

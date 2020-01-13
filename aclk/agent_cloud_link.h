// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_AGENT_CLOUD_LINK_H
#define NETDATA_AGENT_CLOUD_LINK_H

#include "mqtt.h"

#define ACLK_METADATA_TOPIC "meta"
#define ACLK_COMMAND_TOPIC "cmd"
#define ACLK_TOPIC_STRUCTURE "/agent/%s"

#define ACLK_INITIALIZATION_WAIT 60        // Wait for link to initialize in seconds (per msg)
#define ACLK_INITIALIZATION_SLEEP_WAIT 1  // Wait time @ spin lock for MQTT initialization in seconds
#define ACLK_QOS 1
#define ACLK_PING_INTERVAL 60
#define ACLK_LOOP_TIMEOUT  5           // seconds to wait for operations in the library loop
#define ACLK_HEARTBEAT_INTERVAL 60      // Send heart beat interval (in seconds)

#define ACLK_MAX_TOPIC  255

#define ACLK_RECONNECT_DELAY 1          // reconnect delay -- with backoff stragegy fow now
#define ACLK_MAX_RECONNECT_DELAY 120

#define CONFIG_SECTION_ACLK "agent_cloud_link"

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
        .name = "AgentCloudLink", \
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
void aclk_message_callback(struct mosquitto *moqs, void *obj, const struct mosquitto_message *msg);

void aclk_disconnect(void *conn);
void aclk_connect(void *conn);
int aclk_heartbeat();
void aclk_create_metadata_message(BUFFER *dest, char *type, BUFFER *contents);
int aclk_send_metadata_info();
int aclk_wait_for_initialization();



#endif //NETDATA_AGENT_CLOUD_LINK_H

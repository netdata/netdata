// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MQTT_H

#define NETDATA_MQTT_H

#include <mosquitto.h>

#define NETDATA_MQTT_INITIALIZATION_WAIT 60     // Wait for MQTT to initialize in seconds (per msg)
#define NETDATA_MQTT_INITIALIZATION_SLEEP_WAIT 1000  // Wait time @ spin lock for MQTT initialization in ms
#define NETDATA_MQTT_QOS 1
#define NETDATA_MQTT_PING_INTERVAL 60
#define NETDATA_MQTT_LOOP_TIMEOUT  60           // seconds to wait for operations in the library loop


#define NETDATA_MQTT_MAX_TOPIC  255

#define NETDATA_MQTT_RECONNECT_DELAY 1          // reconnect delay -- with backoff stragegy fow now
#define NETDATA_MQTT_MAX_RECONNECT_DELAY 120

#define CONFIG_SECTION_MQTT "mqtt"

typedef enum publish_topic_action {
    PUBLICH_TOPIC_GET,
    PUBLICH_TOPIC_FREE,
    PUBLICH_TOPIC_REBUILD
} PUBLISH_TOPIC_ACTION;

typedef enum mqtt_init_action {
    MQTT_INIT,
    MQTT_REINIT
} MQTT_INIT_ACTION;


#define GET_PUBLISH_BASE_TOPIC get_publish_base_topic(0)
#define FREE_PUBLISH_BASE_TOPIC get_publish_base_topic(1)
#define REBUILD_PUBLISH_BASE_TOPIC get_publish_base_topic(2)


void *mqtt_main(void *ptr);

#define NETDATA_MQTT_HOOK \
    { \
        .name = "MQTT", \
        .config_section = NULL, \
        .config_name = NULL, \
        .enabled = 1, \
        .thread = NULL, \
        .init_routine = NULL, \
        .start_routine = mqtt_main \
    },

extern int mqtt_send(char *base_topic, char *sub_topic, char *message);

int     mqtt_init();
char    *get_base_topic();

// callbacks for agent cloud link
int mqtt_subscribe(char  *topic);
void mqtt_message_callback(
    struct mosquitto *moqs, void *obj, const struct mosquitto_message *msg, const mosquitto_property *props);

#endif //NETDATA_MQTT_H

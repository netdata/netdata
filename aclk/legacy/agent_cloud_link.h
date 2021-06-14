// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_AGENT_CLOUD_LINK_H
#define NETDATA_AGENT_CLOUD_LINK_H

#include "daemon/common.h"
#include "mqtt.h"
#include "aclk_common.h"

#define ACLK_CHART_TOPIC "outbound/meta"
#define ACLK_ALARMS_TOPIC "outbound/alarms"
#define ACLK_METADATA_TOPIC "outbound/meta"
#define ACLK_COMMAND_TOPIC "inbound/cmd"
#define ACLK_TOPIC_STRUCTURE "/agent/%s"

#define ACLK_MAX_BACKOFF_DELAY 1024 // maximum backoff delay in seconds

#define ACLK_INITIALIZATION_WAIT 60      // Wait for link to initialize in seconds (per msg)
#define ACLK_INITIALIZATION_SLEEP_WAIT 1 // Wait time @ spin lock for MQTT initialization in seconds
#define ACLK_PING_INTERVAL 60
#define ACLK_LOOP_TIMEOUT 5 // seconds to wait for operations in the library loop

#define ACLK_MAX_TOPIC 255

#define ACLK_RECONNECT_DELAY 1 // reconnect delay -- with backoff strategy for now
#define ACLK_DEFAULT_PORT 9002
#define ACLK_DEFAULT_HOST "localhost"

#define ACLK_V2_PAYLOAD_SEPARATOR "\x0D\x0A\x0D\x0A"

struct aclk_request {
    char *type_id;
    char *msg_id;
    char *callback_topic;
    char *payload;
    int version;
    int min_version;
    int max_version;
};

typedef enum aclk_init_action { ACLK_INIT, ACLK_REINIT } ACLK_INIT_ACTION;

void *legacy_aclk_main(void *ptr);

extern int aclk_send_message(char *sub_topic, char *message, char *msg_id);
extern int aclk_send_message_bin(char *sub_topic, const void *message, size_t len, char *msg_id);

extern char *is_agent_claimed(void);
extern void aclk_lws_wss_mqtt_layer_disconnect_notif();
char *create_uuid();

// callbacks for agent cloud link
int aclk_subscribe(char *topic, int qos);
int legacy_cloud_to_agent_parse(JSON_ENTRY *e);
void aclk_disconnect();
void aclk_connect();

int aclk_send_metadata(ACLK_METADATA_STATE state, RRDHOST *host);
int legacy_aclk_send_info_metadata(ACLK_METADATA_STATE metadata_submitted, RRDHOST *host);
void legacy_aclk_send_alarm_metadata(ACLK_METADATA_STATE metadata_submitted);

int aclk_wait_for_initialization();
char *create_publish_base_topic();

int aclk_send_single_chart(RRDHOST *host, char *chart);
int legacy_aclk_update_chart(RRDHOST *host, char *chart_name, int create);
int legacy_aclk_update_alarm(RRDHOST *host, ALARM_ENTRY *ae);
void aclk_create_header(BUFFER *dest, char *type, char *msg_id, time_t ts_secs, usec_t ts_us, int version);
int legacy_aclk_handle_cloud_message(char *payload);
void legacy_aclk_add_collector(RRDHOST *host, const char *plugin_name, const char *module_name);
void legacy_aclk_del_collector(RRDHOST *host, const char *plugin_name, const char *module_name);
void legacy_aclk_alarm_reload(void);
unsigned long int aclk_reconnect_delay(int mode);
extern void health_alarm_entry2json_nolock(BUFFER *wb, ALARM_ENTRY *ae, RRDHOST *host);

void aclk_host_state_update(RRDHOST *host, ACLK_CMD cmd);
int aclk_send_info_child_connection(RRDHOST *host, ACLK_CMD cmd);
void aclk_update_next_child_to_popcorn(void);

#endif //NETDATA_AGENT_CLOUD_LINK_H

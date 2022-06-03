// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef ACLK_TX_MSGS_H
#define ACLK_TX_MSGS_H

#include <json-c/json.h>
#include "libnetdata/libnetdata.h"
#include "daemon/common.h"
#include "mqtt_wss_client.h"
#include "schema-wrappers/schema_wrappers.h"
#include "aclk_util.h"

uint16_t aclk_send_bin_message_subtopic_pid(mqtt_wss_client client, char *msg, size_t msg_len, enum aclk_topics subtopic, const char *msgname);

void aclk_send_info_metadata(mqtt_wss_client client, int metadata_submitted, RRDHOST *host);
void aclk_send_alarm_metadata(mqtt_wss_client client, int metadata_submitted);

void aclk_http_msg_v2_err(mqtt_wss_client client, const char *topic, const char *msg_id, int http_code, int ec, const char* emsg, const char *payload, size_t payload_len);
void aclk_http_msg_v2(mqtt_wss_client client, const char *topic, const char *msg_id, usec_t t_exec, usec_t created, int http_code, const char *payload, size_t payload_len);

void aclk_chart_msg(mqtt_wss_client client, RRDHOST *host, const char *chart);

void aclk_alarm_state_msg(mqtt_wss_client client, json_object *msg);

json_object *aclk_generate_disconnect(const char *message);
int aclk_send_app_layer_disconnect(mqtt_wss_client client, const char *message);

#ifdef ENABLE_NEW_CLOUD_PROTOCOL
// new protobuf msgs
uint16_t aclk_send_agent_connection_update(mqtt_wss_client client, int reachable);
char *aclk_generate_lwt(size_t *size);
#endif

#endif

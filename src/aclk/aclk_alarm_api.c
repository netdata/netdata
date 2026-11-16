// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_alarm_api.h"

#include "aclk_query_queue.h"

#include "aclk_util.h"

#include "aclk.h"

#include "mqtt_websockets/mqtt_wss_client.h"

ACLK_ALARM_LOG_SEND_RESULT aclk_send_alarm_log_entry(struct alarm_log_entry *log_entry)
{
    size_t payload_size;
    char *payload = generate_alarm_log_entry(&payload_size, log_entry);

    if (!payload) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "Failed to generate payload");
        return ACLK_ALARM_LOG_DROPPED;
    }

    int rc = aclk_send_bin_msg(payload, payload_size, ACLK_TOPICID_ALARM_LOG, "AlarmLogEntry");
    if (rc == MQTT_WSS_ERR_MSG_TOO_BIG) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "Alert log entry of %zu bytes is too big for the server; dropping it", payload_size);
        return ACLK_ALARM_LOG_DROPPED;
    }
    if (rc)
        return ACLK_ALARM_LOG_SEND_FAILED;

    return ACLK_ALARM_LOG_SENT;
}

void aclk_send_provide_alarm_cfg(struct provide_alarm_configuration *cfg)
{
    aclk_query_t *query = aclk_query_new(ALARM_PROVIDE_CFG);
    query->data.bin_payload.payload = generate_provide_alarm_configuration(&query->data.bin_payload.size, cfg);
    query->data.bin_payload.topic = ACLK_TOPICID_ALARM_CONFIG;
    query->data.bin_payload.msg_name = "ProvideAlarmConfiguration";
    QUEUE_IF_PAYLOAD_PRESENT(query);
}

void aclk_send_alarm_snapshot(alarm_snapshot_proto_ptr_t snapshot)
{
    aclk_query_t *query = aclk_query_new(ALARM_SNAPSHOT);
    query->data.bin_payload.payload = generate_alarm_snapshot_bin(&query->data.bin_payload.size, snapshot);
    query->data.bin_payload.topic = ACLK_TOPICID_ALARM_SNAPSHOT;
    query->data.bin_payload.msg_name = "AlarmSnapshot";
    QUEUE_IF_PAYLOAD_PRESENT(query);
}

// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_alarm_api.h"

#include "aclk_query_queue.h"

#include "aclk_util.h"

void aclk_send_alarm_log_health(struct alarm_log_health *log_health)
{
    aclk_query_t query = aclk_query_new(ALARM_LOG_HEALTH);
    query->data.bin_payload.payload = generate_alarm_log_health(&query->data.bin_payload.size, log_health);
    query->data.bin_payload.topic = ACLK_TOPICID_ALARM_HEALTH;
    query->data.bin_payload.msg_name = "AlarmLogHealth";
    if (query->data.bin_payload.payload)
        aclk_queue_query(query);
}

void aclk_send_alarm_log_entry(struct alarm_log_entry *log_entry)
{
    aclk_query_t query = aclk_query_new(ALARM_LOG_ENTRY);

    query->data.bin_payload.payload = generate_alarm_log_entry(&query->data.bin_payload.size, log_entry);
    query->data.bin_payload.topic = ACLK_TOPICID_ALARM_LOG;
    query->data.bin_payload.msg_name = "AlarmLogEntry";
    if (query->data.bin_payload.payload)
        aclk_queue_query(query);
}

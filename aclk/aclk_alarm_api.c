// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_alarm_api.h"

#include "aclk_query_queue.h"

#include "aclk_util.h"

#include "aclk.h"

void aclk_send_alarm_log_health(struct alarm_log_health *log_health)
{
    GENERATE_AND_SEND_PAYLOAD(ACLK_TOPICID_ALARM_HEALTH, "AlarmLogHealth", generate_alarm_log_health, log_health);
}

void aclk_send_alarm_log_entry(struct alarm_log_entry *log_entry)
{
    GENERATE_AND_SEND_PAYLOAD(ACLK_TOPICID_ALARM_LOG, "AlarmLogEntry", generate_alarm_log_entry, log_entry);
}

void aclk_send_provide_alarm_cfg(struct provide_alarm_configuration *cfg)
{
    GENERATE_AND_SEND_PAYLOAD(ACLK_TOPICID_ALARM_CONFIG, "ProvideAlarmConfiguration", generate_provide_alarm_configuration, cfg);
}

void aclk_send_alarm_snapshot(alarm_snapshot_proto_ptr_t snapshot)
{
    GENERATE_AND_SEND_PAYLOAD(ACLK_TOPICID_ALARM_SNAPSHOT, "AlarmSnapshot", generate_alarm_snapshot_bin, snapshot);
}

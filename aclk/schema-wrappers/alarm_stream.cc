// SPDX-License-Identifier: GPL-3.0-or-later

#include "alarm_stream.h"

#include "proto/alarm/v1/stream.pb.h"

#include "libnetdata/libnetdata.h"

#include "schema_wrapper_utils.h"

struct start_alarm_streaming parse_start_alarm_streaming(const char *data, size_t len)
{
    struct start_alarm_streaming ret;
    memset(&ret, 0, sizeof(ret));

    alarmstream::v1::StartAlarmStreaming msg;

    if (!msg.ParseFromArray(data, len))
        return ret;

    ret.node_id = strdupz(msg.node_id().c_str());
    ret.batch_id = msg.batch_id();
    ret.start_seq_id = msg.start_sequnce_id();

    return ret;
}

char *parse_send_alarm_log_health(const char *data, size_t len)
{
    alarmstream::v1::SendAlarmLogHealth msg;
    if (!msg.ParseFromArray(data, len))
        return NULL;
    return strdupz(msg.node_id().c_str());
}

char *generate_alarm_log_health(size_t *len, struct alarm_log_health *data)
{
    alarmstream::v1::AlarmLogHealth msg;
    alarmstream::v1::LogEntries *entries;

    msg.set_claim_id(data->claim_id);
    msg.set_node_id(data->node_id);
    msg.set_enabled(data->enabled);

    switch (data->status) {
        case ALARM_LOG_STATUS_IDLE:
            msg.set_status(alarmstream::v1::ALARM_LOG_STATUS_IDLE);
            break;
        case ALARM_LOG_STATUS_RUNNING:
            msg.set_status(alarmstream::v1::ALARM_LOG_STATUS_RUNNING);
            break;
        case ALARM_LOG_STATUS_UNSPECIFIED:
            msg.set_status(alarmstream::v1::ALARM_LOG_STATUS_UNSPECIFIED);
            break;
        default:
            error("Unknown status of AlarmLogHealth LogEntry");
            return NULL;
    }

    entries = msg.mutable_log_entries();
    entries->set_first_sequence_id(data->log_entries.first_seq_id);
    entries->set_last_sequence_id(data->log_entries.last_seq_id);

    set_google_timestamp_from_timeval(data->log_entries.first_when, entries->mutable_first_when());
    set_google_timestamp_from_timeval(data->log_entries.last_when, entries->mutable_last_when());

    *len = PROTO_COMPAT_MSG_SIZE(msg);
    char *bin = (char*)mallocz(*len);
    if (!msg.SerializeToArray(bin, *len))
        return NULL;

    return bin;
}

static alarmstream::v1::AlarmStatus aclk_alarm_status_to_proto(enum aclk_alarm_status status)
{
    switch (status) {
        case ALARM_STATUS_NULL:
            return alarmstream::v1::ALARM_STATUS_NULL;
        case ALARM_STATUS_UNKNOWN:
            return alarmstream::v1::ALARM_STATUS_UNKNOWN;
        case ALARM_STATUS_REMOVED:
            return alarmstream::v1::ALARM_STATUS_REMOVED;
        case ALARM_STATUS_NOT_A_NUMBER:
            return alarmstream::v1::ALARM_STATUS_NOT_A_NUMBER;
        case ALARM_STATUS_CLEAR:
            return alarmstream::v1::ALARM_STATUS_CLEAR;
        case ALARM_STATUS_WARNING:
            return alarmstream::v1::ALARM_STATUS_WARNING;
        case ALARM_STATUS_CRITICAL:
            return alarmstream::v1::ALARM_STATUS_CRITICAL;
        default:
            error("Unknown alarm status");
            return alarmstream::v1::ALARM_STATUS_UNKNOWN;
    }
}

void destroy_alarm_log_entry(struct alarm_log_entry *entry)
{
    freez(entry->node_id);
    freez(entry->claim_id);

    freez(entry->chart);
    freez(entry->name);
    freez(entry->family);

    freez(entry->config_hash);

    freez(entry->timezone);

    freez(entry->exec_path);
    freez(entry->conf_source);
    freez(entry->command);

    freez(entry->value_string);
    freez(entry->old_value_string);

    freez(entry->rendered_info);
}

char *generate_alarm_log_entry(size_t *len, struct alarm_log_entry *data)
{
    alarmstream::v1::AlarmLogEntry le;

    le.set_node_id(data->node_id);
    le.set_claim_id(data->claim_id);

    le.set_chart(data->chart);
    le.set_name(data->name);
    if (data->family)
        le.set_family(data->family);

    le.set_batch_id(data->batch_id);
    le.set_sequence_id(data->sequence_id);
    le.set_when(data->when);

    le.set_config_hash(data->config_hash);

    le.set_utc_offset(data->utc_offset);
    le.set_timezone(data->timezone);

    le.set_exec_path(data->exec_path);
    le.set_conf_source(data->conf_source);
    le.set_command(data->command);

    le.set_duration(data->duration);
    le.set_non_clear_duration(data->non_clear_duration);


    le.set_status(aclk_alarm_status_to_proto(data->status));
    le.set_old_status(aclk_alarm_status_to_proto(data->old_status));
    le.set_delay(data->delay);
    le.set_delay_up_to_timestamp(data->delay_up_to_timestamp);

    le.set_last_repeat(data->last_repeat);
    le.set_silenced(data->silenced);

    if (data->value_string)
        le.set_value_string(data->value_string);
    if (data->old_value_string)
        le.set_old_value_string(data->old_value_string);

    le.set_value(data->value);
    le.set_old_value(data->old_value);

    le.set_updated(data->updated);

    le.set_rendered_info(data->rendered_info);

    *len = PROTO_COMPAT_MSG_SIZE(le);
    char *bin = (char*)mallocz(*len);
    if (!le.SerializeToArray(bin, *len))
        return NULL;

    return bin;
}

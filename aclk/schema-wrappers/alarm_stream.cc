// SPDX-License-Identifier: GPL-3.0-or-later

#include "alarm_stream.h"

#include "proto/alarm/v1/stream.pb.h"

#include "libnetdata/libnetdata.h"

#include "schema_wrapper_utils.h"

using namespace alarms::v1;

struct start_alarm_streaming parse_start_alarm_streaming(const char *data, size_t len)
{
    struct start_alarm_streaming ret;
    memset(&ret, 0, sizeof(ret));

    StartAlarmStreaming msg;

    if (!msg.ParseFromArray(data, len))
        return ret;

    ret.node_id = strdupz(msg.node_id().c_str());
    ret.batch_id = msg.batch_id();
    ret.start_seq_id = msg.start_sequnce_id();

    return ret;
}

char *parse_send_alarm_log_health(const char *data, size_t len)
{
    SendAlarmLogHealth msg;
    if (!msg.ParseFromArray(data, len))
        return NULL;
    return strdupz(msg.node_id().c_str());
}

char *generate_alarm_log_health(size_t *len, struct alarm_log_health *data)
{
    AlarmLogHealth msg;
    LogEntries *entries;

    msg.set_claim_id(data->claim_id);
    msg.set_node_id(data->node_id);
    msg.set_enabled(data->enabled);

    switch (data->status) {
        case alarm_log_status_aclk::ALARM_LOG_STATUS_IDLE:
            msg.set_status(alarms::v1::ALARM_LOG_STATUS_IDLE);
            break;
        case alarm_log_status_aclk::ALARM_LOG_STATUS_RUNNING:
            msg.set_status(alarms::v1::ALARM_LOG_STATUS_RUNNING);
            break;
        case alarm_log_status_aclk::ALARM_LOG_STATUS_UNSPECIFIED:
            msg.set_status(alarms::v1::ALARM_LOG_STATUS_UNSPECIFIED);
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

static alarms::v1::AlarmStatus aclk_alarm_status_to_proto(enum aclk_alarm_status status)
{
    switch (status) {
        case aclk_alarm_status::ALARM_STATUS_NULL:
            return alarms::v1::ALARM_STATUS_NULL;
        case aclk_alarm_status::ALARM_STATUS_UNKNOWN:
            return alarms::v1::ALARM_STATUS_UNKNOWN;
        case aclk_alarm_status::ALARM_STATUS_REMOVED:
            return alarms::v1::ALARM_STATUS_REMOVED;
        case aclk_alarm_status::ALARM_STATUS_NOT_A_NUMBER:
            return alarms::v1::ALARM_STATUS_NOT_A_NUMBER;
        case aclk_alarm_status::ALARM_STATUS_CLEAR:
            return alarms::v1::ALARM_STATUS_CLEAR;
        case aclk_alarm_status::ALARM_STATUS_WARNING:
            return alarms::v1::ALARM_STATUS_WARNING;
        case aclk_alarm_status::ALARM_STATUS_CRITICAL:
            return alarms::v1::ALARM_STATUS_CRITICAL;
        default:
            error("Unknown alarm status");
            return alarms::v1::ALARM_STATUS_UNKNOWN;
    }
}

void destroy_alarm_log_entry(struct alarm_log_entry *entry)
{
    //freez(entry->node_id);
    //freez(entry->claim_id);

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

static void fill_alarm_log_entry(struct alarm_log_entry *data, AlarmLogEntry *proto)
{
    proto->set_node_id(data->node_id);
    proto->set_claim_id(data->claim_id);

    proto->set_chart(data->chart);
    proto->set_name(data->name);
    if (data->family)
        proto->set_family(data->family);

    proto->set_batch_id(data->batch_id);
    proto->set_sequence_id(data->sequence_id);
    proto->set_when(data->when);

    proto->set_config_hash(data->config_hash);

    proto->set_utc_offset(data->utc_offset);
    proto->set_timezone(data->timezone);

    proto->set_exec_path(data->exec_path);
    proto->set_conf_source(data->conf_source);
    proto->set_command(data->command);

    proto->set_duration(data->duration);
    proto->set_non_clear_duration(data->non_clear_duration);


    proto->set_status(aclk_alarm_status_to_proto(data->status));
    proto->set_old_status(aclk_alarm_status_to_proto(data->old_status));
    proto->set_delay(data->delay);
    proto->set_delay_up_to_timestamp(data->delay_up_to_timestamp);

    proto->set_last_repeat(data->last_repeat);
    proto->set_silenced(data->silenced);

    if (data->value_string)
        proto->set_value_string(data->value_string);
    if (data->old_value_string)
        proto->set_old_value_string(data->old_value_string);

    proto->set_value(data->value);
    proto->set_old_value(data->old_value);

    proto->set_updated(data->updated);

    proto->set_rendered_info(data->rendered_info);
}

char *generate_alarm_log_entry(size_t *len, struct alarm_log_entry *data)
{
    AlarmLogEntry le;

    fill_alarm_log_entry(data, &le);

    *len = PROTO_COMPAT_MSG_SIZE(le);
    char *bin = (char*)mallocz(*len);
    if (!le.SerializeToArray(bin, *len)) {
        freez(bin);
        return NULL;
    }

    return bin;
}

struct send_alarm_snapshot *parse_send_alarm_snapshot(const char *data, size_t len)
{
    SendAlarmSnapshot msg;
    if (!msg.ParseFromArray(data, len))
        return NULL;

    struct send_alarm_snapshot *ret = (struct send_alarm_snapshot*)callocz(1, sizeof(struct send_alarm_snapshot));
    if (msg.claim_id().c_str())
        ret->claim_id = strdupz(msg.claim_id().c_str());
    if (msg.node_id().c_str())
        ret->node_id = strdupz(msg.node_id().c_str());
    ret->snapshot_id = msg.snapshot_id();
    ret->sequence_id = msg.sequence_id();
    
    return ret;
}

void destroy_send_alarm_snapshot(struct send_alarm_snapshot *ptr)
{
    freez(ptr->claim_id);
    freez(ptr->node_id);
    freez(ptr);
}

alarm_snapshot_proto_ptr_t generate_alarm_snapshot_proto(struct alarm_snapshot *data)
{
    AlarmSnapshot *msg = new AlarmSnapshot;
    if (unlikely(!msg)) fatal("Cannot allocate memory for AlarmSnapshot");

    msg->set_node_id(data->node_id);
    msg->set_claim_id(data->claim_id);
    msg->set_snapshot_id(data->snapshot_id);
    msg->set_chunks(data->chunks);
    msg->set_chunk(data->chunk);

    // this is handled automatically by add_alarm_log_entry2snapshot function
    msg->set_chunk_size(0);

    return msg;
}

void add_alarm_log_entry2snapshot(alarm_snapshot_proto_ptr_t snapshot, struct alarm_log_entry *data)
{
    AlarmSnapshot *alarm_snapshot = (AlarmSnapshot *)snapshot;
    AlarmLogEntry *alarm_log_entry = alarm_snapshot->add_alarms();

    fill_alarm_log_entry(data, alarm_log_entry);

    alarm_snapshot->set_chunk_size(alarm_snapshot->chunk_size() + 1);
}

char *generate_alarm_snapshot_bin(size_t *len, alarm_snapshot_proto_ptr_t snapshot)
{
    AlarmSnapshot *alarm_snapshot = (AlarmSnapshot *)snapshot;
    *len = PROTO_COMPAT_MSG_SIZE_PTR(alarm_snapshot);
    char *bin = (char*)mallocz(*len);
    if (!alarm_snapshot->SerializeToArray(bin, *len)) {
        delete alarm_snapshot;
        return NULL;
    }

    delete alarm_snapshot;
    return bin;
}

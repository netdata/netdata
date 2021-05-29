// SPDX-License-Identifier: GPL-3.0-or-later

#include "alarm_stream.h"

#include "proto/alarm/v1/stream.pb.h"

#include "libnetdata/libnetdata.h"

#include "common.h"

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
    entries->set_first_sequence_id(data->first_seq_id);
    entries->set_last_sequence_id(data->last_seq_id);

    set_google_timestamp_from_timeval(data->first_when, entries->mutable_first_when());
    set_google_timestamp_from_timeval(data->last_when, entries->mutable_last_when());

    *len = msg.ByteSizeLong();
    char *bin = (char*)mallocz(*len);
    if (!msg.SerializeToArray(bin, *len))
        return NULL;

    return bin;
}

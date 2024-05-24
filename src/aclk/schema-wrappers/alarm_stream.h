// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_SCHEMA_WRAPPER_ALARM_STREAM_H
#define ACLK_SCHEMA_WRAPPER_ALARM_STREAM_H

#include <stdlib.h>

#include "database/rrd.h"

#ifdef __cplusplus
extern "C" {
#endif

struct start_alarm_streaming {
    char *node_id;
    uint64_t version;
    bool resets;
};

struct start_alarm_streaming parse_start_alarm_streaming(const char *data, size_t len);

enum aclk_alarm_status {
    ALARM_STATUS_NULL = 0,
    ALARM_STATUS_UNKNOWN = 1,
    ALARM_STATUS_REMOVED = 2,
    ALARM_STATUS_NOT_A_NUMBER = 3,
    ALARM_STATUS_CLEAR = 4,
    ALARM_STATUS_WARNING = 5,
    ALARM_STATUS_CRITICAL = 6
};

struct alarm_log_entry {
    char *node_id;
    char *claim_id;

    char *chart;
    char *name;
    char *family;

    uint64_t when;

    char *config_hash;

    int32_t utc_offset;
    char *timezone;

    char *exec_path;
    char *conf_source;
    char *command;

    uint32_t duration;
    uint32_t non_clear_duration;

    enum aclk_alarm_status status;
    enum aclk_alarm_status old_status;
    uint64_t delay;
    uint64_t delay_up_to_timestamp;

    uint64_t last_repeat;
    int silenced;

    char *value_string;
    char *old_value_string;

    double value;
    double old_value;

    // updated alarm entry, when the status of the alarm has been updated by a later entry
    int updated;

    // rendered_info 
    char *rendered_info;

    char *chart_context;
    char *chart_name;

    uint64_t event_id;
    uint64_t version;
    char *transition_id;
    char *summary;

    // local book keeping
    int64_t health_log_id;
    int64_t alarm_id;
    int64_t unique_id;
    int64_t sequence_id;
};

struct send_alarm_checkpoint {
    char *node_id;
    char *claim_id;
    uint64_t version;
    uint64_t when_end;
};

struct alarm_checkpoint {
    char *node_id;
    char *claim_id;
    char *checksum;
};

struct send_alarm_snapshot {
    char *node_id;
    char *claim_id;
    char *snapshot_uuid;
};

struct alarm_snapshot {
    char *node_id;
    char *claim_id;
    char *snapshot_uuid;
    uint32_t chunks;
    uint32_t chunk;
};

typedef void* alarm_snapshot_proto_ptr_t;

void destroy_alarm_log_entry(struct alarm_log_entry *entry);

char *generate_alarm_log_entry(size_t *len, struct alarm_log_entry *data);

struct send_alarm_snapshot *parse_send_alarm_snapshot(const char *data, size_t len);
void destroy_send_alarm_snapshot(struct send_alarm_snapshot *ptr);

struct send_alarm_checkpoint parse_send_alarm_checkpoint(const char *data, size_t len);
char *generate_alarm_checkpoint(size_t *len, struct alarm_checkpoint *data);

alarm_snapshot_proto_ptr_t generate_alarm_snapshot_proto(struct alarm_snapshot *data);
void add_alarm_log_entry2snapshot(alarm_snapshot_proto_ptr_t snapshot, struct alarm_log_entry *data);
char *generate_alarm_snapshot_bin(size_t *len, alarm_snapshot_proto_ptr_t snapshot);

#ifdef __cplusplus
}
#endif

#endif /* ACLK_SCHEMA_WRAPPER_ALARM_STREAM_H */

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_SCHEMA_WRAPPER_ALARM_STREAM_H
#define ACLK_SCHEMA_WRAPPER_ALARM_STREAM_H

#include <stdlib.h>

#include "database/rrd.h"

#ifdef __cplusplus
extern "C" {
#endif

enum alarm_log_status_aclk {
    ALARM_LOG_STATUS_UNSPECIFIED = 0,
    ALARM_LOG_STATUS_RUNNING = 1,
    ALARM_LOG_STATUS_IDLE = 2
};

struct alarm_log_entries {
    int64_t first_seq_id;
    struct timeval first_when;

    int64_t last_seq_id;
    struct timeval last_when;
};

struct alarm_log_health {
    char *claim_id;
    char *node_id;
    int enabled;
    enum alarm_log_status_aclk status;
    struct alarm_log_entries log_entries;
};

struct start_alarm_streaming {
    char *node_id;
    uint64_t batch_id;
    uint64_t start_seq_id;
};

struct start_alarm_streaming parse_start_alarm_streaming(const char *data, size_t len);
char *parse_send_alarm_log_health(const char *data, size_t len);

char *generate_alarm_log_health(size_t *len, struct alarm_log_health *data);

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

    uint64_t batch_id;
    uint64_t sequence_id;
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
};

struct send_alarm_snapshot {
    char *node_id;
    char *claim_id;
    uint64_t snapshot_id;
    uint64_t sequence_id;
};

struct alarm_snapshot {
    char *node_id;
    char *claim_id;
    uint64_t snapshot_id;
    uint32_t chunks;
    uint32_t chunk;
};

typedef void* alarm_snapshot_proto_ptr_t;

void destroy_alarm_log_entry(struct alarm_log_entry *entry);

char *generate_alarm_log_entry(size_t *len, struct alarm_log_entry *data);

struct send_alarm_snapshot *parse_send_alarm_snapshot(const char *data, size_t len);
void destroy_send_alarm_snapshot(struct send_alarm_snapshot *ptr);

alarm_snapshot_proto_ptr_t generate_alarm_snapshot_proto(struct alarm_snapshot *data);
void add_alarm_log_entry2snapshot(alarm_snapshot_proto_ptr_t snapshot, struct alarm_log_entry *data);
char *generate_alarm_snapshot_bin(size_t *len, alarm_snapshot_proto_ptr_t snapshot);

#ifdef __cplusplus
}
#endif

#endif /* ACLK_SCHEMA_WRAPPER_ALARM_STREAM_H */

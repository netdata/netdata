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
    uint64_t batch_id;
    uint64_t start_seq_id;
};

struct start_alarm_streaming parse_start_alarm_streaming(const char *data, size_t len);
char *parse_send_alarm_log_health(const char *data, size_t len);

enum alarm_log_status_aclk {
    ALARM_LOG_STATUS_UNSPECIFIED = 0,
    ALARM_LOG_STATUS_RUNNING = 1,
    ALARM_LOG_STATUS_IDLE = 2
};
struct alarm_log_health {
    char *claim_id;
    char *node_id;
    int enabled;
    enum alarm_log_status_aclk status;
    int64_t first_seq_id;
    struct timeval first_when;
    int64_t last_seq_id;
    struct timeval last_when;
};

char *generate_alarm_log_health(size_t *len, struct alarm_log_health *data);

#ifdef __cplusplus
}
#endif

#endif /* ACLK_SCHEMA_WRAPPER_ALARM_STREAM_H */

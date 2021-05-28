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

#ifdef __cplusplus
}
#endif

#endif /* ACLK_SCHEMA_WRAPPER_ALARM_STREAM_H */

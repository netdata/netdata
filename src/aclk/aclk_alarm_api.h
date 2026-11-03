// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_ALARM_API_H
#define ACLK_ALARM_API_H

#include "database/rrd.h"
#include "schema-wrappers/schema_wrappers.h"

typedef enum {
    ACLK_ALARM_LOG_SENT = 0,            // handed to the mqtt layer
    ACLK_ALARM_LOG_SEND_FAILED,         // not accepted by the mqtt layer right now (offline, buffer full) - retry later
    ACLK_ALARM_LOG_DROPPED,             // cannot be built, or too big for the server - retrying would never succeed
} ACLK_ALARM_LOG_SEND_RESULT;

ACLK_ALARM_LOG_SEND_RESULT aclk_send_alarm_log_entry(struct alarm_log_entry *log_entry);
void aclk_send_provide_alarm_cfg(struct provide_alarm_configuration *cfg);
void aclk_send_alarm_snapshot(alarm_snapshot_proto_ptr_t snapshot);

#endif /* ACLK_ALARM_API_H */

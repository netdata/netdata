// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_ALARM_API_H
#define ACLK_ALARM_API_H

#include "../daemon/common.h"
#include "schema-wrappers/schema_wrappers.h"

void aclk_send_alarm_log_health(struct alarm_log_health *log_health, char *node_id);
void aclk_send_alarm_log_entry(struct alarm_log_entry *log_entry, const char *node_id, const char *context);
void aclk_send_provide_alarm_cfg(struct provide_alarm_configuration *cfg);
void aclk_send_alarm_snapshot(alarm_snapshot_proto_ptr_t snapshot);

#endif /* ACLK_ALARM_API_H */

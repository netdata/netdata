// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_ALARM_API_H
#define ACLK_ALARM_API_H

#include "../daemon/common.h"
#include "schema-wrappers/schema_wrappers.h"

void aclk_send_alarm_log_health(struct alarm_log_health *log_health);

#endif /* ACLK_ALARM_API_H */

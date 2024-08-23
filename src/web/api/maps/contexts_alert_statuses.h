// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CONTEXTS_ALERT_STATUSES_H
#define NETDATA_CONTEXTS_ALERT_STATUSES_H

#include "libnetdata/libnetdata.h"

typedef enum contexts_alert_status {
    CONTEXT_ALERT_UNINITIALIZED = (1 << 6), // include UNINITIALIZED alerts
    CONTEXT_ALERT_UNDEFINED     = (1 << 7), // include UNDEFINED alerts
    CONTEXT_ALERT_CLEAR         = (1 << 8), // include CLEAR alerts
    CONTEXT_ALERT_RAISED        = (1 << 9), // include WARNING & CRITICAL alerts
    CONTEXT_ALERT_WARNING       = (1 << 10), // include WARNING alerts
    CONTEXT_ALERT_CRITICAL      = (1 << 11), // include CRITICAL alerts
} CONTEXTS_ALERT_STATUS;

#define CONTEXTS_ALERT_STATUSES (CONTEXT_ALERT_UNINITIALIZED | CONTEXT_ALERT_UNDEFINED | CONTEXT_ALERT_CLEAR | \
                                 CONTEXT_ALERT_RAISED | CONTEXT_ALERT_WARNING | CONTEXT_ALERT_CRITICAL)

CONTEXTS_ALERT_STATUS contexts_alert_status_str_to_id(char *o);
void contexts_alerts_status_to_buffer_json_array(BUFFER *wb, const char *key,
    CONTEXTS_ALERT_STATUS options);

void contexts_alert_statuses_init(void);

#endif //NETDATA_CONTEXTS_ALERT_STATUSES_H

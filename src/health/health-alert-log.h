// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_HEALTH_ALERT_LOG_H
#define NETDATA_HEALTH_ALERT_LOG_H

#include "libnetdata/libnetdata.h"

typedef struct alarm_log {
    uint32_t next_log_id;
    uint32_t next_alarm_id;
    unsigned int count;
    unsigned int max;
    uint32_t health_log_retention_s;                   // the health log retention in seconds to be kept in db
    struct alarm_entry *alarms;
    RW_SPINLOCK spinlock;
} ALARM_LOG;

typedef struct health {
    time_t delay_up_to;                             // a timestamp to delay alarms processing up to
    STRING *default_exec;                           // the full path of the alarms notifications program
    STRING *default_recipient;                      // the default recipient for all alarms
    bool enabled;                                   // 1 when this host has health enabled
    bool use_summary_for_notifications;             // whether to use the summary field as a subject for notifications
    int32_t pending_transitions;                    // pending alert transitions to store
    uint64_t evloop_iteration;                      // the last health iteration that evaluated this host
} HEALTH;

#endif //NETDATA_HEALTH_ALERT_LOG_H

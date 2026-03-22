// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_HEALTH_ALERT_LOG_H
#define NETDATA_HEALTH_ALERT_LOG_H

#include "libnetdata/libnetdata.h"

typedef struct alarm_log {
    uint32_t next_log_id;
    uint32_t next_alarm_id;
    unsigned int max;
    uint32_t health_log_retention_s;                   // the health log retention in seconds to be kept in db
    struct alarm_entry *alarms;
    RW_SPINLOCK spinlock;
} ALARM_LOG;

struct health_alert_status_counts {
    uint32_t clear;
    uint32_t warning;
    uint32_t critical;
    uint32_t undefined;
    uint32_t uninitialized;
};

typedef struct health {
    // Recomputed on each host health evaluation and read by alerts APIs as a host-level prefilter.
    struct {
        uint64_t generation;                         // seqlock-style generation for consistent readers
        struct health_alert_status_counts counts;
        uint8_t valid;                               // 1 when snapshot is fully published
    } alert_status_snapshot;

    time_t delay_up_to;                             // a timestamp to delay alarms processing up to
    STRING *default_exec;                           // the full path of the alarms notifications program
    STRING *default_recipient;                      // the default recipient for all alarms
    bool enabled;                                   // 1 when this host has health enabled
    bool use_summary_for_notifications;             // whether to use the summary field as a subject for notifications
    int32_t pending_transitions;                    // pending alert transitions to store
    uint64_t evloop_iteration;                      // the last health iteration that evaluated this host

    // Per-host scheduling for parallel health processing
    time_t next_run;                                // when this host should next be processed
    bool processing;                                // true while health work is queued/running for this host
} HEALTH;

#endif //NETDATA_HEALTH_ALERT_LOG_H

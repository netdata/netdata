// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ND_LOG_LIMIT_H
#define NETDATA_ND_LOG_LIMIT_H

#include "../libnetdata.h"

struct nd_log_source;
bool nd_log_limit_reached(struct nd_log_source *source);

struct nd_log_limit {
    SPINLOCK spinlock;
    usec_t started_monotonic_ut;
    uint32_t counter;
    uint32_t prevented;

    uint32_t throttle_period;
    uint32_t logs_per_period;
    uint32_t logs_per_period_backup;
};

#define ND_LOG_LIMITS_DEFAULT (struct nd_log_limit){ \
    .spinlock = SPINLOCK_INITIALIZER, \
    .logs_per_period = ND_LOG_DEFAULT_THROTTLE_LOGS, \
    .logs_per_period_backup = ND_LOG_DEFAULT_THROTTLE_LOGS, \
    .throttle_period = ND_LOG_DEFAULT_THROTTLE_PERIOD, \
}

#define ND_LOG_LIMITS_UNLIMITED (struct nd_log_limit){ \
    .spinlock = SPINLOCK_INITIALIZER, \
    .logs_per_period = 0, \
    .logs_per_period_backup = 0, \
    .throttle_period = 0, \
}

#include "nd_log-internals.h"

#endif //NETDATA_ND_LOG_LIMIT_H

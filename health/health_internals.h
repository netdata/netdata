// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_HEALTH_INTERNALS_H
#define NETDATA_HEALTH_INTERNALS_H

#include "health.h"

#define HEALTH_LOG_ENTRIES_DEFAULT 1000U
#define HEALTH_LOG_ENTRIES_MAX 100000U
#define HEALTH_LOG_ENTRIES_MIN 10U

#define HEALTH_LOG_HISTORY_DEFAULT (5 * 86400)

struct health_plugin_globals {
    struct {
        bool enabled;
        bool stock_enabled;
        bool use_summary_for_notifications;

        unsigned int health_log_entries_max;
        uint32_t health_log_history;                   // the health log history in seconds to be kept in db

        STRING *silencers_filename;
        STRING *default_exec;
        STRING *default_recipient;

        SIMPLE_PATTERN *enabled_alerts;

        uint32_t default_warn_repeat_every;     // the default value for the interval between repeating warning notifications
        uint32_t default_crit_repeat_every;     // the default value for the interval between repeating critical notifications
    } config;

    struct {
        SPINLOCK spinlock;
        RRD_ALERT_PROTOTYPE *base;
    } prototypes;

    DICTIONARY *rrdvars;
};

extern struct health_plugin_globals health_globals;

int health_readfile(const char *filename, void *data __maybe_unused, bool stock_config __maybe_unused);

#endif //NETDATA_HEALTH_INTERNALS_H

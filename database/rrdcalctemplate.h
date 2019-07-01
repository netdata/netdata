// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDCALCTEMPLATE_H
#define NETDATA_RRDCALCTEMPLATE_H 1

#include "rrd.h"

// RRDCALCTEMPLATE
// these are to be applied to charts found dynamically
// based on their context.
struct rrdcalctemplate {
    char *name;
    uint32_t hash_name;

    char *exec;
    char *recipient;

    char *context;
    uint32_t hash_context;

    char *family_match;
    SIMPLE_PATTERN *family_pattern;

    char *source;                   // the source of this alarm
    char *units;                    // the units of the alarm
    char *info;                     // a short description of the alarm

    int update_every;               // update frequency for the alarm

    // the red and green threshold of this alarm (to be set to the chart)
    calculated_number green;
    calculated_number red;

    // ------------------------------------------------------------------------
    // database lookup settings

    char *dimensions;               // the chart dimensions
    RRDR_GROUPING group;               // grouping method: average, max, etc.
    int before;                     // ending point in time-series
    int after;                      // starting point in time-series
    uint32_t options;               // calculation options

    // ------------------------------------------------------------------------
    // notification delay settings

    int delay_up_duration;         // duration to delay notifications when alarm raises
    int delay_down_duration;       // duration to delay notifications when alarm lowers
    int delay_max_duration;        // the absolute max delay to apply to this alarm
    float delay_multiplier;        // multiplier for all delays when alarms switch status

    // ------------------------------------------------------------------------
    // notification repeat settings

    uint32_t warn_repeat_every;    // interval between repeating warning notifications
    uint32_t crit_repeat_every; // interval between repeating critical notifications

    // ------------------------------------------------------------------------
    // expressions related to the alarm

    EVAL_EXPRESSION *calculation;
    EVAL_EXPRESSION *warning;
    EVAL_EXPRESSION *critical;

    struct rrdcalctemplate *next;
};

#define RRDCALCTEMPLATE_HAS_DB_LOOKUP(rt) ((rt)->after)

extern void rrdcalctemplate_link_matching(RRDSET *st);

extern void rrdcalctemplate_free(RRDCALCTEMPLATE *rt);
extern void rrdcalctemplate_unlink_and_free(RRDHOST *host, RRDCALCTEMPLATE *rt);

#endif //NETDATA_RRDCALCTEMPLATE_H

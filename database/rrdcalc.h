// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd.h"

#ifndef NETDATA_RRDCALC_H
#define NETDATA_RRDCALC_H 1

// calculated variables (defined in health configuration)
// These aggregate time-series data at fixed intervals
// (defined in their update_every member below)
// They increase the overhead of netdata.
//
// These calculations are allocated and linked (->next)
// under RRDHOST.
// Then are also linked to RRDSET (of course only when the
// chart is found, via ->rrdset_next and ->rrdset_prev).
// This double-linked list is maintained sorted at all times
// having as RRDSET.calculations the RRDCALC to be processed
// next.

#define RRDCALC_FLAG_DB_ERROR              0x00000001
#define RRDCALC_FLAG_DB_NAN                0x00000002
/* #define RRDCALC_FLAG_DB_STALE           0x00000004 */
#define RRDCALC_FLAG_CALC_ERROR            0x00000008
#define RRDCALC_FLAG_WARN_ERROR            0x00000010
#define RRDCALC_FLAG_CRIT_ERROR            0x00000020
#define RRDCALC_FLAG_RUNNABLE              0x00000040
#define RRDCALC_FLAG_NO_CLEAR_NOTIFICATION 0x80000000

struct rrdcalc {
    uint32_t id;                    // the unique id of this alarm
    uint32_t next_event_id;         // the next event id that will be used for this alarm

    char *name;                     // the name of this alarm
    uint32_t hash;

    char *exec;                     // the command to execute when this alarm switches state
    char *recipient;                // the recipient of the alarm (the first parameter to exec)

    char *chart;                    // the chart id this should be linked to
    uint32_t hash_chart;

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
    // expressions related to the alarm

    EVAL_EXPRESSION *calculation;   // expression to calculate the value of the alarm
    EVAL_EXPRESSION *warning;       // expression to check the warning condition
    EVAL_EXPRESSION *critical;      // expression to check the critical condition

    // ------------------------------------------------------------------------
    // notification delay settings

    int delay_up_duration;         // duration to delay notifications when alarm raises
    int delay_down_duration;       // duration to delay notifications when alarm lowers
    int delay_max_duration;        // the absolute max delay to apply to this alarm
    float delay_multiplier;        // multiplier for all delays when alarms switch status
    // while now < delay_up_to

    // ------------------------------------------------------------------------
    // runtime information

    RRDCALC_STATUS status;          // the current status of the alarm

    calculated_number value;        // the current value of the alarm
    calculated_number old_value;    // the previous value of the alarm

    uint32_t rrdcalc_flags;         // check RRDCALC_FLAG_*

    time_t last_updated;            // the last update timestamp of the alarm
    time_t next_update;             // the next update timestamp of the alarm
    time_t last_status_change;      // the timestamp of the last time this alarm changed status

    time_t db_after;                // the first timestamp evaluated by the db lookup
    time_t db_before;               // the last timestamp evaluated by the db lookup

    time_t delay_up_to_timestamp;   // the timestamp up to which we should delay notifications
    int delay_up_current;           // the current up notification delay duration
    int delay_down_current;         // the current down notification delay duration
    int delay_last;                 // the last delay we used

    // ------------------------------------------------------------------------
    // variables this alarm exposes to the rest of the alarms

    RRDVAR *local;
    RRDVAR *family;
    RRDVAR *hostid;
    RRDVAR *hostname;

    // ------------------------------------------------------------------------
    // the chart this alarm it is linked to

    struct rrdset *rrdset;

    // linking of this alarm on its chart
    struct rrdcalc *rrdset_next;
    struct rrdcalc *rrdset_prev;

    struct rrdcalc *next;
};

#define RRDCALC_HAS_DB_LOOKUP(rc) ((rc)->after)

extern void rrdsetcalc_link_matching(RRDSET *st);
extern void rrdsetcalc_unlink(RRDCALC *rc);
extern RRDCALC *rrdcalc_find(RRDSET *st, const char *name);

extern const char *rrdcalc_status2string(RRDCALC_STATUS status);

extern void rrdcalc_free(RRDCALC *rc);
extern void rrdcalc_unlink_and_free(RRDHOST *host, RRDCALC *rc);

extern int rrdcalc_exists(RRDHOST *host, const char *chart, const char *name, uint32_t hash_chart, uint32_t hash_name);
extern uint32_t rrdcalc_get_unique_id(RRDHOST *host, const char *chart, const char *name, uint32_t *next_event_id);
extern RRDCALC *rrdcalc_create(RRDHOST *host, RRDCALCTEMPLATE *rt, const char *chart);
extern void rrdcalc_create_part2(RRDHOST *host, RRDCALC *rc);

#endif //NETDATA_RRDCALC_H

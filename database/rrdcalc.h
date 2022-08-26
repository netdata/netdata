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
#define RRDCALC_FLAG_DISABLED              0x00000080
#define RRDCALC_FLAG_SILENCED              0x00000100
#define RRDCALC_FLAG_RUN_ONCE              0x00000200
#define RRDCALC_FLAG_NO_CLEAR_NOTIFICATION 0x80000000


struct rrdcalc {
    avl_t avl;                      // the index, with key the id - this has to be first!
    uint32_t id;                    // the unique id of this alarm
    uint32_t next_event_id;         // the next event id that will be used for this alarm

    char *name;                     // the name of this alarm
    uint32_t hash;                  // the hash of the alarm name
    uuid_t config_hash_id;          // a predictable hash_id based on specific alert configuration

    char *exec;                     // the command to execute when this alarm switches state
    char *recipient;                // the recipient of the alarm (the first parameter to exec)

    char *classification;           // the class that this alarm belongs
    char *component;                // the component that this alarm refers to
    char *type;                     // type of the alarm

    STRING *chart;                  // the chart id this should be linked to
    uint32_t hash_chart;

    char *plugin_match;             //the plugin name that should be linked to
    SIMPLE_PATTERN *plugin_pattern;

    char *module_match;             //the module name that should be linked to
    SIMPLE_PATTERN *module_pattern;

    char *source;                   // the source of this alarm
    char *units;                    // the units of the alarm
    char *original_info;            // the original info field before any variable replacement
    char *info;                     // a short description of the alarm

    int update_every;               // update frequency for the alarm

    // the red and green threshold of this alarm (to be set to the chart)
    NETDATA_DOUBLE green;
    NETDATA_DOUBLE red;

    // ------------------------------------------------------------------------
    // database lookup settings

    char *dimensions;               // the chart dimensions
    char *foreachdim;               // the group of dimensions that the `foreach` will be applied.
    SIMPLE_PATTERN *spdim;          // used if and only if there is a simple pattern for the chart.
    int foreachcounter;             // the number of alarms created with foreachdim, this also works as an id of the
                                    // children
    RRDR_GROUPING group;            // grouping method: average, max, etc.
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
    // notification repeat settings

    uint32_t warn_repeat_every;     // interval between repeating warning notifications
    uint32_t crit_repeat_every; // interval between repeating critical notifications

    // ------------------------------------------------------------------------
    // Labels settings
    char *host_labels;                   // the label read from an alarm file
    SIMPLE_PATTERN *host_labels_pattern; // the simple pattern of labels

    // ------------------------------------------------------------------------
    // runtime information

    RRDCALC_STATUS old_status; // the old status of the alarm
    RRDCALC_STATUS status;          // the current status of the alarm

    NETDATA_DOUBLE value;        // the current value of the alarm
    NETDATA_DOUBLE old_value;    // the previous value of the alarm

    uint32_t rrdcalc_flags;         // check RRDCALC_FLAG_*

    time_t last_updated;            // the last update timestamp of the alarm
    time_t next_update;             // the next update timestamp of the alarm
    time_t last_status_change;      // the timestamp of the last time this alarm changed status
    time_t last_repeat; // the last time the alarm got repeated
    uint32_t times_repeat;          // number of times the alarm got repeated

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

#define rrdcalc_chart_name(rc) string2str((rc)->chart)

struct alert_config {
    char *alarm;
    char *template_key;
    char *os;
    char *host;
    char *on;
    char *families;
    char *plugin;
    char *module;
    char *charts;
    char *lookup;
    char *calc;
    char *warn;
    char *crit;
    char *every;
    char *green;
    char *red;
    char *exec;
    char *to;
    char *units;
    char *info;
    char *classification;
    char *component;
    char *type;
    char *delay;
    char *options;
    char *repeat;
    char *host_labels;

    char *p_db_lookup_dimensions;
    char *p_db_lookup_method;
    uint32_t p_db_lookup_options;
    int32_t p_db_lookup_after;
    int32_t p_db_lookup_before;
    int32_t p_update_every;
};

extern int alarm_isrepeating(RRDHOST *host, uint32_t alarm_id);
extern int alarm_entry_isrepeating(RRDHOST *host, ALARM_ENTRY *ae);
extern RRDCALC *alarm_max_last_repeat(RRDHOST *host, char *alarm_name, uint32_t hash);

#define RRDCALC_HAS_DB_LOOKUP(rc) ((rc)->after)

extern void rrdsetcalc_link_matching(RRDSET *st);
extern void rrdsetcalc_unlink(RRDCALC *rc);
extern RRDCALC *rrdcalc_find(RRDSET *st, const char *name);

extern const char *rrdcalc_status2string(RRDCALC_STATUS status);

extern void rrdcalc_free(RRDCALC *rc);
extern void rrdcalc_unlink_and_free(RRDHOST *host, RRDCALC *rc);

extern int rrdcalc_exists(RRDHOST *host, const char *chart, const char *name, uint32_t hash_chart, uint32_t hash_name);
extern uint32_t rrdcalc_get_unique_id(RRDHOST *host, const char *chart, const char *name, uint32_t *next_event_id);
extern RRDCALC *rrdcalc_create_from_template(RRDHOST *host, RRDCALCTEMPLATE *rt, const char *chart);
extern RRDCALC *rrdcalc_create_from_rrdcalc(RRDCALC *rc, RRDHOST *host, const char *name, const char *dimension);
extern void rrdcalc_add_to_host(RRDHOST *host, RRDCALC *rc);
extern void dimension_remove_pipe_comma(char *str);
extern char *alarm_name_with_dim(char *name, size_t namelen, const char *dim, size_t dimlen);
extern void rrdcalc_update_rrdlabels(RRDSET *st);

extern void rrdcalc_labels_unlink();
extern void rrdcalc_labels_unlink_alarm_from_host(RRDHOST *host);

static inline int rrdcalc_isrepeating(RRDCALC *rc) {
    if (unlikely(rc->warn_repeat_every > 0 || rc->crit_repeat_every > 0)) {
        return 1;
    }
    return 0;
}

#define RRDCALC_VAR_MAX 100
#define RRDCALC_VAR_FAMILY "$family"
#define RRDCALC_VAR_LABEL "$label:"
#define RRDCALC_VAR_LABEL_LEN (sizeof(RRDCALC_VAR_LABEL)-1)

#endif //NETDATA_RRDCALC_H

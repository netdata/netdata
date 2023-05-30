// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd.h"

#ifndef NETDATA_RRDCALC_H
#define NETDATA_RRDCALC_H 1

// calculated variables (defined in health configuration)
// These aggregate time-series data at fixed intervals
// (defined in their update_every member below)
// They increase the overhead of netdata.
//
// These calculations are stored under RRDHOST.
// Then are also linked to RRDSET (of course only when a
// matching chart is found).

typedef enum {
    RRDCALC_FLAG_DB_ERROR                   = (1 << 0),
    RRDCALC_FLAG_DB_NAN                     = (1 << 1),
    // RRDCALC_FLAG_DB_STALE                   = (1 << 2),
    RRDCALC_FLAG_CALC_ERROR                 = (1 << 3),
    RRDCALC_FLAG_WARN_ERROR                 = (1 << 4),
    RRDCALC_FLAG_CRIT_ERROR                 = (1 << 5),
    RRDCALC_FLAG_RUNNABLE                   = (1 << 6),
    RRDCALC_FLAG_DISABLED                   = (1 << 7),
    RRDCALC_FLAG_SILENCED                   = (1 << 8),
    RRDCALC_FLAG_RUN_ONCE                   = (1 << 9),
    RRDCALC_FLAG_FROM_TEMPLATE              = (1 << 10), // the rrdcalc has been created from a template
} RRDCALC_FLAGS;

typedef enum {
    // This list uses several other options from RRDR_OPTIONS for db lookups.
    // To add an item here, you need to reserve a bit in RRDR_OPTIONS.
    RRDCALC_OPTION_NO_CLEAR_NOTIFICATION    = RRDR_OPTION_HEALTH_RSRVD1,
} RRDCALC_OPTIONS;

#define RRDCALC_ALL_OPTIONS_EXCLUDING_THE_RRDR_ONES (RRDCALC_OPTION_NO_CLEAR_NOTIFICATION)

struct rrdcalc {
    STRING *key;                    // the unique key in the host's rrdcalc_root_index

    uint32_t id;                    // the unique id of this alarm
    uint32_t next_event_id;         // the next event id that will be used for this alarm

    uuid_t config_hash_id;          // a predictable hash_id based on specific alert configuration

    STRING *name;                   // the name of this alarm
    STRING *chart;                  // the chart id this should be linked to

    STRING *exec;                   // the command to execute when this alarm switches state
    STRING *recipient;              // the recipient of the alarm (the first parameter to exec)

    STRING *classification;         // the class that this alarm belongs
    STRING *component;              // the component that this alarm refers to
    STRING *type;                   // type of the alarm

    STRING *plugin_match;           // the plugin name that should be linked to
    SIMPLE_PATTERN *plugin_pattern;

    STRING *module_match;           // the module name that should be linked to
    SIMPLE_PATTERN *module_pattern;

    STRING *source;                 // the source of this alarm
    STRING *units;                  // the units of the alarm
    STRING *original_info;          // the original info field before any variable replacement
    STRING *info;                   // a short description of the alarm

    int update_every;               // update frequency for the alarm

    // the red and green threshold of this alarm (to be set to the chart)
    NETDATA_DOUBLE green;
    NETDATA_DOUBLE red;

    // ------------------------------------------------------------------------
    // database lookup settings

    STRING *dimensions;             // the chart dimensions
    STRING *foreach_dimension;      // the group of dimensions that the `foreach` will be applied.
    SIMPLE_PATTERN *foreach_dimension_pattern; // used if and only if there is a simple pattern for the chart.
    RRDR_TIME_GROUPING group;            // grouping method: average, max, etc.
    int before;                     // ending point in time-series
    int after;                      // starting point in time-series
    RRDCALC_OPTIONS options;        // configuration options

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

    uint32_t warn_repeat_every;    // interval between repeating warning notifications
    uint32_t crit_repeat_every;    // interval between repeating critical notifications

    // ------------------------------------------------------------------------
    // Labels settings
    STRING *host_labels;                 // the label read from an alarm file
    SIMPLE_PATTERN *host_labels_pattern; // the simple pattern of labels

    STRING *chart_labels;                 // the chart label read from an alarm file
    SIMPLE_PATTERN *chart_labels_pattern; // the simple pattern of chart labels

    // ------------------------------------------------------------------------
    // runtime information

    RRDCALC_STATUS old_status;      // the old status of the alarm
    RRDCALC_STATUS status;          // the current status of the alarm

    NETDATA_DOUBLE value;           // the current value of the alarm
    NETDATA_DOUBLE old_value;       // the previous value of the alarm

    RRDCALC_FLAGS run_flags;        // check RRDCALC_FLAG_*

    time_t last_updated;            // the last update timestamp of the alarm
    time_t next_update;             // the next update timestamp of the alarm
    time_t last_status_change;      // the timestamp of the last time this alarm changed status
    time_t last_repeat;             // the last time the alarm got repeated
    uint32_t times_repeat;          // number of times the alarm got repeated

    time_t db_after;                // the first timestamp evaluated by the db lookup
    time_t db_before;               // the last timestamp evaluated by the db lookup

    time_t delay_up_to_timestamp;   // the timestamp up to which we should delay notifications
    int delay_up_current;           // the current up notification delay duration
    int delay_down_current;         // the current down notification delay duration
    int delay_last;                 // the last delay we used

    // ------------------------------------------------------------------------
    // variables this alarm exposes to the rest of the alarms

    const RRDVAR_ACQUIRED *rrdvar_local;
    const RRDVAR_ACQUIRED *rrdvar_family;
    const RRDVAR_ACQUIRED *rrdvar_host_chart_id;
    const RRDVAR_ACQUIRED *rrdvar_host_chart_name;

    // ------------------------------------------------------------------------
    // the chart this alarm it is linked to

    size_t labels_version;
    struct rrdset *rrdset;

    struct rrdcalc *next;
    struct rrdcalc *prev;
};

#define rrdcalc_name(rc) string2str((rc)->name)
#define rrdcalc_chart_name(rc) string2str((rc)->chart)
#define rrdcalc_exec(rc) string2str((rc)->exec)
#define rrdcalc_recipient(rc) string2str((rc)->recipient)
#define rrdcalc_classification(rc) string2str((rc)->classification)
#define rrdcalc_component(rc) string2str((rc)->component)
#define rrdcalc_type(rc) string2str((rc)->type)
#define rrdcalc_plugin_match(rc) string2str((rc)->plugin_match)
#define rrdcalc_module_match(rc) string2str((rc)->module_match)
#define rrdcalc_source(rc) string2str((rc)->source)
#define rrdcalc_units(rc) string2str((rc)->units)
#define rrdcalc_original_info(rc) string2str((rc)->original_info)
#define rrdcalc_info(rc) string2str((rc)->info)
#define rrdcalc_dimensions(rc) string2str((rc)->dimensions)
#define rrdcalc_foreachdim(rc) string2str((rc)->foreach_dimension)
#define rrdcalc_host_labels(rc) string2str((rc)->host_labels)
#define rrdcalc_chart_labels(rc) string2str((rc)->chart_labels)

#define foreach_rrdcalc_in_rrdhost_read(host, rc) \
    dfe_start_read((host)->rrdcalc_root_index, rc) \

#define foreach_rrdcalc_in_rrdhost_reentrant(host, rc) \
    dfe_start_reentrant((host)->rrdcalc_root_index, rc)

#define foreach_rrdcalc_in_rrdhost_done(rc) \
    dfe_done(rc)

struct alert_config {
    STRING *alarm;
    STRING *template_key;
    STRING *os;
    STRING *host;
    STRING *on;
    STRING *families;
    STRING *plugin;
    STRING *module;
    STRING *charts;
    STRING *lookup;
    STRING *calc;
    STRING *warn;
    STRING *crit;
    STRING *every;
    STRING *green;
    STRING *red;
    STRING *exec;
    STRING *to;
    STRING *units;
    STRING *info;
    STRING *classification;
    STRING *component;
    STRING *type;
    STRING *delay;
    STRING *options;
    STRING *repeat;
    STRING *host_labels;
    STRING *chart_labels;

    STRING *p_db_lookup_dimensions;
    STRING *p_db_lookup_method;

    uint32_t p_db_lookup_options;
    int32_t p_db_lookup_after;
    int32_t p_db_lookup_before;
    int32_t p_update_every;
};

#define RRDCALC_HAS_DB_LOOKUP(rc) ((rc)->after)

void rrdcalc_update_info_using_rrdset_labels(RRDCALC *rc);

void rrdcalc_link_matching_alerts_to_rrdset(RRDSET *st);

const RRDCALC_ACQUIRED *rrdcalc_from_rrdset_get(RRDSET *st, const char *alert_name);
void rrdcalc_from_rrdset_release(RRDSET *st, const RRDCALC_ACQUIRED *rca);
RRDCALC *rrdcalc_acquired_to_rrdcalc(const RRDCALC_ACQUIRED *rca);

const char *rrdcalc_status2string(RRDCALC_STATUS status);

void rrdcalc_free_unused_rrdcalc_loaded_from_config(RRDCALC *rc);

uint32_t rrdcalc_get_unique_id(RRDHOST *host, STRING *chart, STRING *name, uint32_t *next_event_id);
void rrdcalc_add_from_rrdcalctemplate(RRDHOST *host, RRDCALCTEMPLATE *rt, RRDSET *st, const char *overwrite_alert_name, const char *overwrite_dimensions);
int rrdcalc_add_from_config(RRDHOST *host, RRDCALC *rc);

void rrdcalc_delete_alerts_not_matching_host_labels_from_all_hosts();
void rrdcalc_delete_alerts_not_matching_host_labels_from_this_host(RRDHOST *host);

static inline int rrdcalc_isrepeating(RRDCALC *rc) {
    if (unlikely(rc->warn_repeat_every > 0 || rc->crit_repeat_every > 0)) {
        return 1;
    }
    return 0;
}

void rrdcalc_unlink_all_rrdset_alerts(RRDSET *st);
void rrdcalc_delete_all(RRDHOST *host);

void rrdcalc_rrdhost_index_init(RRDHOST *host);
void rrdcalc_rrdhost_index_destroy(RRDHOST *host);

#define RRDCALC_VAR_MAX 100
#define RRDCALC_VAR_FAMILY "${family}"
#define RRDCALC_VAR_LABEL "${label:"
#define RRDCALC_VAR_LABEL_LEN (sizeof(RRDCALC_VAR_LABEL)-1)

#endif //NETDATA_RRDCALC_H

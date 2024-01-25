// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_HEALTH_PROTOTYPES_H
#define NETDATA_HEALTH_PROTOTYPES_H

#include "../web/api/queries/rrdr.h"

typedef enum __attribute__((packed)) {
    ALERT_ACTION_OPTION_NONE = 0,
    ALERT_ACTION_OPTION_NO_CLEAR_NOTIFICATION = (1 << 0),
} ALERT_ACTION_OPTIONS;

struct rrd_alert_match {
    bool enabled;

    bool is_template;
    union {
        STRING *chart;
        STRING *context;
    } on;

    STRING *os;
    STRING *host;
    STRING *charts;                         // the charts that should be linked to (for templates)
    STRING *plugin;                         // the plugin name that should be linked to
    STRING *module;                         // the module name that should be linked to
    STRING *host_labels;                    // the label read from an alarm file
    STRING *chart_labels;                   // the chart label read from an alarm file

    SIMPLE_PATTERN *os_pattern;
    SIMPLE_PATTERN *host_pattern;
    SIMPLE_PATTERN *charts_pattern;         // the simple pattern of charts
    SIMPLE_PATTERN *plugin_pattern;         // the simple pattern of plugin
    SIMPLE_PATTERN *module_pattern;         // the simple pattern of module
    SIMPLE_PATTERN *host_labels_pattern;    // the simple pattern of labels
    SIMPLE_PATTERN *chart_labels_pattern;   // the simple pattern of chart labels
};
void rrd_alert_match_cleanup(struct rrd_alert_match *am);

struct rrd_alert_config {
    uuid_t hash_id;

    STRING *name;                   // the name of this alarm

    STRING *exec;                   // the command to execute when this alarm switches state
    STRING *recipient;              // the recipient of the alarm (the first parameter to exec)

    STRING *classification;         // the class that this alarm belongs
    STRING *component;              // the component that this alarm refers to
    STRING *type;                   // type of the alarm

    DYNCFG_SOURCE_TYPE source_type;
    STRING *source;                 // the source of this alarm
    STRING *units;                  // the units of the alarm
    STRING *summary;                // a short alert summary
    STRING *info;                   // a description of the alarm
    STRING *lookup;                 // the lookup field

    int update_every;               // update frequency for the alarm

    ALERT_ACTION_OPTIONS alert_action_options;

    // the red and green threshold of this alarm (to be set to the chart)
    NETDATA_DOUBLE green;
    NETDATA_DOUBLE red;

    // ------------------------------------------------------------------------
    // database lookup settings

    STRING *dimensions;             // the chart dimensions
    RRDR_TIME_GROUPING group;       // grouping method: average, max, etc.
    int before;                     // ending point in time-series
    int after;                      // starting point in time-series
    RRDR_OPTIONS options;           // configuration options

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

    bool has_custom_repeat_config;
    uint32_t warn_repeat_every;    // interval between repeating warning notifications
    uint32_t crit_repeat_every;    // interval between repeating critical notifications
};
void rrd_alert_config_cleanup(struct rrd_alert_config *ac);

#include "health.h"

void health_init_prototypes(void);

bool health_plugin_enabled(void);
void health_plugin_disable(void);

void health_reload_prototypes(void);
void health_apply_prototypes_to_host(RRDHOST *host);
void health_apply_prototypes_to_all_hosts(void);

void health_prototype_alerts_for_rrdset_incrementally(RRDSET *st);

struct rrd_alert_config;
struct rrd_alert_match;
void health_prototype_copy_config(struct rrd_alert_config *dst, struct rrd_alert_config *src);
void health_prototype_copy_match_without_patterns(struct rrd_alert_match *dst, struct rrd_alert_match *src);
void health_prototype_reset_alerts_for_rrdset(RRDSET *st);

#endif //NETDATA_HEALTH_PROTOTYPES_H

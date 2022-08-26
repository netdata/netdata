// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDCALCTEMPLATE_H
#define NETDATA_RRDCALCTEMPLATE_H 1

#include "rrd.h"

// RRDCALCTEMPLATE
// these are to be applied to charts found dynamically
// based on their context.
struct rrdcalctemplate {
    uuid_t config_hash_id;

    STRING *name;

    STRING *exec;
    STRING *recipient;

    STRING *classification;
    STRING *component;
    STRING *type;

    STRING *context;

    STRING *family_match;
    SIMPLE_PATTERN *family_pattern;

    STRING *plugin_match;
    SIMPLE_PATTERN *plugin_pattern;

    STRING *module_match;
    SIMPLE_PATTERN *module_pattern;

    STRING *charts_match;
    SIMPLE_PATTERN *charts_pattern;

    STRING *source;                 // the source of this alarm
    STRING *units;                  // the units of the alarm
    STRING *info;                   // a short description of the alarm

    int update_every;               // update frequency for the alarm

    // the red and green threshold of this alarm (to be set to the chart)
    NETDATA_DOUBLE green;
    NETDATA_DOUBLE red;

    // ------------------------------------------------------------------------
    // database lookup settings

    STRING *dimensions;             // the chart dimensions
    STRING *foreachdim;             // the group of dimensions that the lookup will be applied.
    SIMPLE_PATTERN *spdim;          // used if and only if there is a simple pattern for the chart.
    int foreachcounter;             // the number of alarms created with foreachdim, this also works as an id of the
                                    // children
    RRDR_GROUPING group;            // grouping method: average, max, etc.
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
    // Labels settings
    STRING *host_labels;                 // the label read from an alarm file
    SIMPLE_PATTERN *host_labels_pattern; // the simple pattern of labels

    // ------------------------------------------------------------------------
    // expressions related to the alarm

    EVAL_EXPRESSION *calculation;
    EVAL_EXPRESSION *warning;
    EVAL_EXPRESSION *critical;

    struct rrdcalctemplate *next;
};

#define rrdcalctemplate_name(rt) string2str((rt)->name)
#define rrdcalctemplate_exec(rt) string2str((rt)->exec)
#define rrdcalctemplate_recipient(rt) string2str((rt)->recipient)
#define rrdcalctemplate_classification(rt) string2str((rt)->classification)
#define rrdcalctemplate_component(rt) string2str((rt)->component)
#define rrdcalctemplate_type(rt) string2str((rt)->type)
#define rrdcalctemplate_family_match(rt) string2str((rt)->family_match)
#define rrdcalctemplate_plugin_match(rt) string2str((rt)->plugin_match)
#define rrdcalctemplate_module_match(rt) string2str((rt)->module_match)
#define rrdcalctemplate_charts_match(rt) string2str((rt)->charts_match)
#define rrdcalctemplate_units(rt) string2str((rt)->units)
#define rrdcalctemplate_info(rt) string2str((rt)->info)
#define rrdcalctemplate_source(rt) string2str((rt)->source)
#define rrdcalctemplate_dimensions(rt) string2str((rt)->dimensions)
#define rrdcalctemplate_foreachdim(rt) string2str((rt)->foreachdim)
#define rrdcalctemplate_host_labels(rt) string2str((rt)->host_labels)

#define RRDCALCTEMPLATE_HAS_DB_LOOKUP(rt) ((rt)->after)

extern void rrdcalctemplate_link_matching(RRDSET *st);

extern void rrdcalctemplate_free(RRDCALCTEMPLATE *rt);
extern void rrdcalctemplate_unlink_and_free(RRDHOST *host, RRDCALCTEMPLATE *rt);
extern void rrdcalctemplate_create_alarms(RRDHOST *host, RRDCALCTEMPLATE *rt, RRDSET *st);
#endif //NETDATA_RRDCALCTEMPLATE_H

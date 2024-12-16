// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_HEALTH_ALERT_ENTRY_H
#define NETDATA_HEALTH_ALERT_ENTRY_H

#include "libnetdata/libnetdata.h"
#include "rrdcalc.h"

struct alarm_entry {
    uint32_t unique_id;
    uint32_t alarm_id;
    uint32_t alarm_event_id;
    usec_t global_id;
    nd_uuid_t config_hash_id;
    nd_uuid_t transition_id;

    time_t when;
    time_t duration;
    time_t non_clear_duration;

    STRING *name;
    STRING *chart;
    STRING *chart_context;
    STRING *chart_name;

    STRING *classification;
    STRING *component;
    STRING *type;

    STRING *exec;
    STRING *recipient;
    time_t exec_run_timestamp;
    int exec_code;

    STRING *source;
    STRING *units;
    STRING *summary;
    STRING *info;

    NETDATA_DOUBLE old_value;
    NETDATA_DOUBLE new_value;

    STRING *old_value_string;
    STRING *new_value_string;

    RRDCALC_STATUS old_status;
    RRDCALC_STATUS new_status;

    uint32_t flags;
    int32_t pending_save_count;

    int delay;
    time_t delay_up_to_timestamp;

    uint32_t updated_by_id;
    uint32_t updates_id;

    time_t last_repeat;

    POPEN_INSTANCE *popen_instance;

    struct alarm_entry *next;
    struct alarm_entry *next_in_progress;
    struct alarm_entry *prev_in_progress;
};


#define ae_name(ae) string2str((ae)->name)
#define ae_chart_id(ae) string2str((ae)->chart)
#define ae_chart_name(ae) string2str((ae)->chart_name)
#define ae_chart_context(ae) string2str((ae)->chart_context)
#define ae_classification(ae) string2str((ae)->classification)
#define ae_exec(ae) string2str((ae)->exec)
#define ae_recipient(ae) string2str((ae)->recipient)
#define ae_source(ae) string2str((ae)->source)
#define ae_units(ae) string2str((ae)->units)
#define ae_summary(ae) string2str((ae)->summary)
#define ae_info(ae) string2str((ae)->info)
#define ae_old_value_string(ae) string2str((ae)->old_value_string)
#define ae_new_value_string(ae) string2str((ae)->new_value_string)

#endif //NETDATA_HEALTH_ALERT_ENTRY_H

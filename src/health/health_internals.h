// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_HEALTH_INTERNALS_H
#define NETDATA_HEALTH_INTERNALS_H

#include "health.h"

#define HEALTH_LOG_ENTRIES_DEFAULT 1000U
#define HEALTH_LOG_ENTRIES_MAX 100000U
#define HEALTH_LOG_ENTRIES_MIN 10U

#define HEALTH_LOG_RETENTION_DEFAULT (5 * 86400)

#define HEALTH_CONF_MAX_LINE 4096

#define HEALTH_ALARM_KEY "alarm"
#define HEALTH_TEMPLATE_KEY "template"
#define HEALTH_CHART_KEY "chart"
#define HEALTH_CONTEXT_KEY "context"
#define HEALTH_ON_KEY "on"
#define HEALTH_HOST_KEY "hosts"
#define HEALTH_OS_KEY "os"
#define HEALTH_PLUGIN_KEY "plugin"
#define HEALTH_MODULE_KEY "module"
#define HEALTH_LOOKUP_KEY "lookup"
#define HEALTH_CALC_KEY "calc"
#define HEALTH_EVERY_KEY "every"
#define HEALTH_GREEN_KEY "green"
#define HEALTH_RED_KEY "red"
#define HEALTH_WARN_KEY "warn"
#define HEALTH_CRIT_KEY "crit"
#define HEALTH_EXEC_KEY "exec"
#define HEALTH_RECIPIENT_KEY "to"
#define HEALTH_UNITS_KEY "units"
#define HEALTH_SUMMARY_KEY "summary"
#define HEALTH_INFO_KEY "info"
#define HEALTH_CLASS_KEY "class"
#define HEALTH_COMPONENT_KEY "component"
#define HEALTH_TYPE_KEY "type"
#define HEALTH_DELAY_KEY "delay"
#define HEALTH_OPTIONS_KEY "options"
#define HEALTH_REPEAT_KEY "repeat"
#define HEALTH_HOST_LABEL_KEY "host labels"
#define HEALTH_CHART_LABEL_KEY "chart labels"

void alert_action_options_to_buffer_json_array(BUFFER *wb, const char *key, ALERT_ACTION_OPTIONS options);
void alert_action_options_to_buffer(BUFFER *wb, ALERT_ACTION_OPTIONS options);
ALERT_ACTION_OPTIONS alert_action_options_parse(char *o);
ALERT_ACTION_OPTIONS alert_action_options_parse_one(const char *o);

typedef struct rrd_alert_prototype {
    struct rrd_alert_match match;
    struct rrd_alert_config config;

    struct {
        bool enabled;
        bool is_on_disk;
        RW_SPINLOCK rw_spinlock;
        struct rrd_alert_prototype *prev, *next;
    } _internal;
} RRD_ALERT_PROTOTYPE;
bool health_prototype_add(RRD_ALERT_PROTOTYPE *ap, char **msg);
void health_prototype_cleanup(RRD_ALERT_PROTOTYPE *ap);
void health_prototype_free(RRD_ALERT_PROTOTYPE *ap);

struct health_plugin_globals {
    struct {
        SPINLOCK spinlock;
        bool done;
    } initialization;

    struct {
        bool enabled;
        bool stock_enabled;
        bool use_summary_for_notifications;

        unsigned int health_log_entries_max;
        uint32_t health_log_retention_s;        // the health log retention in seconds to be kept in db

        STRING *silencers_filename;
        STRING *default_exec;
        STRING *default_recipient;

        SIMPLE_PATTERN *enabled_alerts;

        uint32_t default_warn_repeat_every;     // the default value for the interval between repeating warning notifications
        uint32_t default_crit_repeat_every;     // the default value for the interval between repeating critical notifications

        int32_t run_at_least_every_seconds;
        int32_t postpone_alarms_during_hibernation_for_seconds;
    } config;

    struct {
        DICTIONARY *dict;
    } prototypes;
};

extern struct health_plugin_globals health_globals;

int health_readfile(const char *filename, void *data, bool stock_config);

// for unit testing
int health_parse_db_lookup(size_t line, const char *filename, char *string, struct rrd_alert_config *ac);
void unlink_alarm_notify_in_progress(ALARM_ENTRY *ae);
void wait_for_all_notifications_to_finish_before_allowing_health_to_be_cleaned_up(void);

void health_alarm_wait_for_execution(ALARM_ENTRY *ae);

bool rrdcalc_add_from_prototype(RRDHOST *host, RRDSET *st, RRD_ALERT_PROTOTYPE *ap);

int dyncfg_health_cb(const char *transaction, const char *id, DYNCFG_CMDS cmd, const char *add_name,
                     BUFFER *payload, usec_t *stop_monotonic_ut, bool *cancelled,
                     BUFFER *result, HTTP_ACCESS access, const char *source, void *data);

void health_dyncfg_unregister_all_prototypes(void);
void health_dyncfg_register_all_prototypes(void);
void health_prototype_to_json(BUFFER *wb, RRD_ALERT_PROTOTYPE *ap, bool for_hashing);

bool alert_variable_lookup(STRING *variable, void *data, NETDATA_DOUBLE *result);

struct health_raised_summary;
struct health_raised_summary *alerts_raised_summary_create(RRDHOST *host);
void alerts_raised_summary_populate(struct health_raised_summary *hrm);
void alerts_raised_summary_free(struct health_raised_summary *hrm);
void health_send_notification(RRDHOST *host, ALARM_ENTRY *ae, struct health_raised_summary *hrm);
void health_alarm_log_process_to_send_notifications(RRDHOST *host, struct health_raised_summary *hrm);

void health_apply_prototype_to_host(RRDHOST *host, RRD_ALERT_PROTOTYPE *ap);
void health_prototype_apply_to_all_hosts(RRD_ALERT_PROTOTYPE *ap);

#endif //NETDATA_HEALTH_INTERNALS_H

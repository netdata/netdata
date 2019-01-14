// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_HEALTH_H
#define NETDATA_HEALTH_H 1

#include "../daemon/common.h"

#define NETDATA_PLUGIN_HOOK_HEALTH \
    { \
        .name = "HEALTH", \
        .config_section = NULL, \
        .config_name = NULL, \
        .enabled = 1, \
        .thread = NULL, \
        .init_routine = NULL, \
        .start_routine = health_main \
    },

extern unsigned int default_health_enabled;

#define HEALTH_ENTRY_FLAG_PROCESSED             0x00000001
#define HEALTH_ENTRY_FLAG_UPDATED               0x00000002
#define HEALTH_ENTRY_FLAG_EXEC_RUN              0x00000004
#define HEALTH_ENTRY_FLAG_EXEC_FAILED           0x00000008
#define HEALTH_ENTRY_FLAG_SILENCED              0x00000008

#define HEALTH_ENTRY_FLAG_SAVED                 0x10000000
#define HEALTH_ENTRY_FLAG_NO_CLEAR_NOTIFICATION 0x80000000

#ifndef HEALTH_LISTEN_PORT
#define HEALTH_LISTEN_PORT 19998
#endif

#ifndef HEALTH_LISTEN_BACKLOG
#define HEALTH_LISTEN_BACKLOG 4096
#endif

#define HEALTH_ALARM_KEY "alarm"
#define HEALTH_TEMPLATE_KEY "template"
#define HEALTH_ON_KEY "on"
#define HEALTH_CONTEXT_KEY "context"
#define HEALTH_CHART_KEY "chart"
#define HEALTH_HOST_KEY "hosts"
#define HEALTH_OS_KEY "os"
#define HEALTH_FAMILIES_KEY "families"
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
#define HEALTH_INFO_KEY "info"
#define HEALTH_DELAY_KEY "delay"
#define HEALTH_OPTIONS_KEY "options"

typedef struct silencer {
    char *alarms;
    SIMPLE_PATTERN *alarms_pattern;

    char *hosts;
    SIMPLE_PATTERN *hosts_pattern;

    char *contexts;
    SIMPLE_PATTERN *contexts_pattern;

    char *charts;
    SIMPLE_PATTERN *charts_pattern;

    char *families;
    SIMPLE_PATTERN *families_pattern;

    struct silencer *next;
} SILENCER;

typedef enum silence_type {
    STYPE_NONE,
    STYPE_DISABLE_ALARMS,
    STYPE_SILENCE_NOTIFICATIONS
} SILENCE_TYPE;

typedef struct silencers {
    int all_alarms;
    SILENCE_TYPE stype;
    SILENCER *silencers;
} SILENCERS;

SILENCERS *silencers;

extern void health_init(void);
extern void *health_main(void *ptr);

extern void health_reload(void);

extern int health_variable_lookup(const char *variable, uint32_t hash, RRDCALC *rc, calculated_number *result);
extern void health_alarms2json(RRDHOST *host, BUFFER *wb, int all);
extern void health_alarm_log2json(RRDHOST *host, BUFFER *wb, uint32_t after);

void health_api_v1_chart_variables2json(RRDSET *st, BUFFER *buf);

extern int health_alarm_log_open(RRDHOST *host);
extern void health_alarm_log_close(RRDHOST *host);
extern void health_log_rotate(RRDHOST *host);
extern void health_alarm_log_save(RRDHOST *host, ALARM_ENTRY *ae);
extern ssize_t health_alarm_log_read(RRDHOST *host, FILE *fp, const char *filename);
extern void health_alarm_log_load(RRDHOST *host);

extern void health_alarm_log(
        RRDHOST *host,
        uint32_t alarm_id,
        uint32_t alarm_event_id,
        time_t when,
        const char *name,
        const char *chart,
        const char *family,
        const char *exec,
        const char *recipient,
        time_t duration,
        calculated_number old_value,
        calculated_number new_value,
        RRDCALC_STATUS old_status,
        RRDCALC_STATUS new_status,
        const char *source,
        const char *units,
        const char *info,
        int delay,
        uint32_t flags);

extern void health_readdir(RRDHOST *host, const char *user_path, const char *stock_path, const char *subpath);
extern char *health_user_config_dir(void);
extern char *health_stock_config_dir(void);
extern void health_reload_host(RRDHOST *host);
extern void health_alarm_log_free(RRDHOST *host);

extern void health_alarm_log_free_one_nochecks_nounlink(ALARM_ENTRY *ae);

extern void *health_cmdapi_thread(void *ptr);

#endif //NETDATA_HEALTH_H

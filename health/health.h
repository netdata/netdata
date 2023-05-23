// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_HEALTH_H
#define NETDATA_HEALTH_H 1

#include "daemon/common.h"

extern unsigned int default_health_enabled;

#define HEALTH_ENTRY_FLAG_PROCESSED             0x00000001
#define HEALTH_ENTRY_FLAG_UPDATED               0x00000002
#define HEALTH_ENTRY_FLAG_EXEC_RUN              0x00000004
#define HEALTH_ENTRY_FLAG_EXEC_FAILED           0x00000008
#define HEALTH_ENTRY_FLAG_SILENCED              0x00000010
#define HEALTH_ENTRY_RUN_ONCE                   0x00000020
#define HEALTH_ENTRY_FLAG_EXEC_IN_PROGRESS      0x00000040
#define HEALTH_ENTRY_FLAG_IS_REPEATING          0x00000080

#define HEALTH_ENTRY_FLAG_SAVED                 0x10000000
#define HEALTH_ENTRY_FLAG_ACLK_QUEUED           0x20000000
#define HEALTH_ENTRY_FLAG_NO_CLEAR_NOTIFICATION 0x80000000

#ifndef HEALTH_LISTEN_PORT
#define HEALTH_LISTEN_PORT 19998
#endif

#ifndef HEALTH_LISTEN_BACKLOG
#define HEALTH_LISTEN_BACKLOG 4096
#endif

#define HEALTH_SILENCERS_MAX_FILE_LEN 10000

extern char *silencers_filename;
extern SIMPLE_PATTERN *conf_enabled_alarms;
extern DICTIONARY *health_rrdvars;

void health_init(void);

void health_reload(void);

void health_aggregate_alarms(RRDHOST *host, BUFFER *wb, BUFFER* context, RRDCALC_STATUS status);
void health_alarms2json(RRDHOST *host, BUFFER *wb, int all);
void health_alarms_values2json(RRDHOST *host, BUFFER *wb, int all);

void health_api_v1_chart_variables2json(RRDSET *st, BUFFER *buf);
void health_api_v1_chart_custom_variables2json(RRDSET *st, BUFFER *buf);

int health_alarm_log_open(RRDHOST *host);
void health_alarm_log_save(RRDHOST *host, ALARM_ENTRY *ae);
void health_alarm_log_load(RRDHOST *host);

ALARM_ENTRY* health_create_alarm_entry(
    RRDHOST *host,
    uint32_t alarm_id,
    uint32_t alarm_event_id,
    const uuid_t config_hash_id,
    time_t when,
    STRING *name,
    STRING *chart,
    STRING *chart_context,
    STRING *family,
    STRING *classification,
    STRING *component,
    STRING *type,
    STRING *exec,
    STRING *recipient,
    time_t duration,
    NETDATA_DOUBLE old_value,
    NETDATA_DOUBLE new_value,
    RRDCALC_STATUS old_status,
    RRDCALC_STATUS new_status,
    STRING *source,
    STRING *units,
    STRING *info,
    int delay,
    uint32_t flags);

void health_alarm_log_add_entry(RRDHOST *host, ALARM_ENTRY *ae);

void health_readdir(RRDHOST *host, const char *user_path, const char *stock_path, const char *subpath);
char *health_user_config_dir(void);
char *health_stock_config_dir(void);
void health_alarm_log_free(RRDHOST *host);

void health_alarm_log_free_one_nochecks_nounlink(ALARM_ENTRY *ae);

void *health_cmdapi_thread(void *ptr);

char *health_edit_command_from_source(const char *source);
void sql_refresh_hashes(void);

void health_add_host_labels(void);
void health_string2json(BUFFER *wb, const char *prefix, const char *label, const char *value, const char *suffix);

#endif //NETDATA_HEALTH_H

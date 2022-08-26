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

extern void health_init(void);

extern void health_reload(void);

extern int health_variable_lookup(const char *variable, uint32_t hash, RRDCALC *rc, NETDATA_DOUBLE *result);
extern void health_aggregate_alarms(RRDHOST *host, BUFFER *wb, BUFFER* context, RRDCALC_STATUS status);
extern void health_alarms2json(RRDHOST *host, BUFFER *wb, int all);
extern void health_alarms_values2json(RRDHOST *host, BUFFER *wb, int all);
extern void health_alarm_log2json(RRDHOST *host, BUFFER *wb, uint32_t after, char *chart);

void health_api_v1_chart_variables2json(RRDSET *st, BUFFER *buf);
void health_api_v1_chart_custom_variables2json(RRDSET *st, BUFFER *buf);

extern int health_alarm_log_open(RRDHOST *host);
extern void health_alarm_log_save(RRDHOST *host, ALARM_ENTRY *ae);
extern void health_alarm_log_load(RRDHOST *host);

extern ALARM_ENTRY* health_create_alarm_entry(
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

extern void health_alarm_log(RRDHOST *host, ALARM_ENTRY *ae);

extern void health_readdir(RRDHOST *host, const char *user_path, const char *stock_path, const char *subpath);
extern char *health_user_config_dir(void);
extern char *health_stock_config_dir(void);
extern void health_alarm_log_free(RRDHOST *host);

extern void health_alarm_log_free_one_nochecks_nounlink(ALARM_ENTRY *ae);

extern void *health_cmdapi_thread(void *ptr);

extern void health_label_log_save(RRDHOST *host);

extern char *health_edit_command_from_source(const char *source);
extern void sql_refresh_hashes(void);

extern SIMPLE_PATTERN *health_pattern_from_foreach(const char *s);

#endif //NETDATA_HEALTH_H

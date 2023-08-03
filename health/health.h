// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_HEALTH_H
#define NETDATA_HEALTH_H 1

#include "daemon/common.h"

extern unsigned int default_health_enabled;

typedef enum __attribute__((packed)) {
    HEALTH_ENTRY_FLAG_PROCESSED             = 0x00000001, // notifications engine has processed this
    HEALTH_ENTRY_FLAG_UPDATED               = 0x00000002, // there is a more recent update about this transition
    HEALTH_ENTRY_FLAG_EXEC_RUN              = 0x00000004, // notification script has been run (this is the intent, not the result)
    HEALTH_ENTRY_FLAG_EXEC_FAILED           = 0x00000008, // notification script couldn't be run
    HEALTH_ENTRY_FLAG_SILENCED              = 0x00000010,
    HEALTH_ENTRY_RUN_ONCE                   = 0x00000020,
    HEALTH_ENTRY_FLAG_EXEC_IN_PROGRESS      = 0x00000040,
    HEALTH_ENTRY_FLAG_IS_REPEATING          = 0x00000080,
    HEALTH_ENTRY_FLAG_SAVED                 = 0x10000000, // Saved to SQL
    HEALTH_ENTRY_FLAG_ACLK_QUEUED           = 0x20000000, // Sent to Netdata Cloud
    HEALTH_ENTRY_FLAG_NO_CLEAR_NOTIFICATION = 0x80000000,
} HEALTH_ENTRY_FLAGS;

void health_entry_flags_to_json_array(BUFFER *wb, const char *key, HEALTH_ENTRY_FLAGS flags);

#ifndef HEALTH_LISTEN_PORT
#define HEALTH_LISTEN_PORT 19998
#endif

#ifndef HEALTH_LISTEN_BACKLOG
#define HEALTH_LISTEN_BACKLOG 4096
#endif

#ifndef HEALTH_LOG_DEFAULT_HISTORY
#define HEALTH_LOG_DEFAULT_HISTORY 432000
#endif

#ifndef HEALTH_LOG_MINIMUM_HISTORY
#define HEALTH_LOG_MINIMUM_HISTORY 86400
#endif

#define HEALTH_SILENCERS_MAX_FILE_LEN 10000

extern char *silencers_filename;
extern SIMPLE_PATTERN *conf_enabled_alarms;
extern DICTIONARY *health_rrdvars;

void health_init(void);

void health_reload(void);

void health_aggregate_alarms(RRDHOST *host, BUFFER *wb, BUFFER* context, RRDCALC_STATUS status);
void health_alarms2json(RRDHOST *host, BUFFER *wb, int all);
void health_alert2json_conf(RRDHOST *host, BUFFER *wb, CONTEXTS_V2_OPTIONS all);
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
    STRING *chart_id,
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
    HEALTH_ENTRY_FLAGS flags);

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

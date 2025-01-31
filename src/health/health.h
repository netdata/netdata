// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_HEALTH_H
#define NETDATA_HEALTH_H 1

#include "database/rrd.h"
#include "rrdcalc.h"

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

#define RRDR_OPTIONS_DATA_SOURCES (RRDR_OPTION_PERCENTAGE|RRDR_OPTION_ANOMALY_BIT)
#define RRDR_OPTIONS_DIMS_AGGREGATION (RRDR_OPTION_DIMS_MIN|RRDR_OPTION_DIMS_MAX|RRDR_OPTION_DIMS_AVERAGE|RRDR_OPTION_DIMS_MIN2MAX)
#define RRDR_OPTIONS_REMOVE_OVERLAPPING(options) ((options) & ~(RRDR_OPTIONS_DIMS_AGGREGATION|RRDR_OPTIONS_DATA_SOURCES))

void health_entry_flags_to_json_array(BUFFER *wb, const char *key, HEALTH_ENTRY_FLAGS flags);

#ifndef HEALTH_LISTEN_PORT
#define HEALTH_LISTEN_PORT 19998
#endif

#ifndef HEALTH_LISTEN_BACKLOG
#define HEALTH_LISTEN_BACKLOG 4096
#endif

#ifndef HEALTH_LOG_RETENTION_DEFAULT
#define HEALTH_LOG_RETENTION_DEFAULT (5 * 86400)
#endif

#ifndef HEALTH_LOG_MINIMUM_HISTORY
#define HEALTH_LOG_MINIMUM_HISTORY 86400
#endif

#define HEALTH_SILENCERS_MAX_FILE_LEN 10000

void health_plugin_init(void);
void health_plugin_destroy(void);

void health_plugin_reload(void);

void health_aggregate_alarms(RRDHOST *host, BUFFER *wb, BUFFER* context, RRDCALC_STATUS status);
void health_alarms2json(RRDHOST *host, BUFFER *wb, int all);
void health_alert2json_conf(RRDHOST *host, BUFFER *wb, CONTEXTS_OPTIONS all);
void health_alarms_values2json(RRDHOST *host, BUFFER *wb, int all);

void health_api_v1_chart_variables2json(RRDSET *st, BUFFER *wb);
void health_api_v1_chart_custom_variables2json(RRDSET *st, BUFFER *buf);

int health_alarm_log_open(RRDHOST *host);
void health_alarm_log_save(RRDHOST *host, ALARM_ENTRY *ae, bool async);
void health_alarm_log_load(RRDHOST *host);

ALARM_ENTRY* health_create_alarm_entry(
    RRDHOST *host,
    RRDCALC *rc,
    time_t when,
    time_t duration,
    NETDATA_DOUBLE old_value,
    NETDATA_DOUBLE new_value,
    RRDCALC_STATUS old_status,
    RRDCALC_STATUS new_status,
    int delay,
    HEALTH_ENTRY_FLAGS flags);

void health_alarm_log_add_entry(RRDHOST *host, ALARM_ENTRY *ae, bool async);

const char *health_user_config_dir(void);
const char *health_stock_config_dir(void);
void health_alarm_log_free(RRDHOST *host);

void health_alarm_log_free_one_nochecks_nounlink(ALARM_ENTRY *ae);

void *health_cmdapi_thread(void *ptr);

char *health_edit_command_from_source(const char *source);

void health_string2json(BUFFER *wb, const char *prefix, const char *label, const char *value, const char *suffix);

void health_log_alert_transition_with_trace(RRDHOST *host, ALARM_ENTRY *ae, int line, const char *file, const char *function);
#define health_log_alert(host, ae) health_log_alert_transition_with_trace(host, ae, __LINE__, __FILE__, __FUNCTION__)
bool health_alarm_log_get_global_id_and_transition_id_for_rrdcalc(RRDCALC *rc, usec_t *global_id, nd_uuid_t *transitions_id);

int alert_variable_lookup_trace(RRDHOST *host, RRDSET *st, const char *variable, BUFFER *wb);

#include "health_prototypes.h"
#include "health_silencers.h"

typedef void (*prototype_metadata_cb_t)(void *data, STRING *type, STRING *component, STRING *classification, STRING *recipient);
void health_prototype_metadata_foreach(void *data, prototype_metadata_cb_t cb);

uint64_t health_evloop_current_iteration(void);
void rrdhost_set_health_evloop_iteration(RRDHOST *host);
uint64_t rrdhost_health_evloop_last_iteration(RRDHOST *host);

#endif //NETDATA_HEALTH_H

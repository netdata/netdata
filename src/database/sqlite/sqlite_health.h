// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_HEALTH_H
#define NETDATA_SQLITE_HEALTH_H

#include "database/rrd.h"
#include "database/sqlite/vendored/sqlite3.h"

#define ALERT_TRANSITION_DELAY_LONG  (600)
#define ALERT_TRANSITION_DELAY_SHORT (10)
#define ALERT_TRANSITION_DELAY_NONE  (0)

struct sql_alert_transition_data;
struct sql_alert_config_data;
struct rrd_alert_prototype;
void sql_health_alarm_log_load(RRDHOST *host);
void sql_health_alarm_log_save(RRDHOST *host, ALARM_ENTRY *ae);
void sql_health_alarm_log_cleanup(RRDHOST *host);
void sql_alert_store_config(struct rrd_alert_prototype *ap);
int sql_health_get_last_executed_event(RRDHOST *host, ALARM_ENTRY *ae, RRDCALC_STATUS *last_executed_status);
void sql_health_alarm_log2json(RRDHOST *host, BUFFER *wb, time_t after, const char *chart);
int health_migrate_old_health_log_table(char *table);
uint32_t sql_get_alarm_id(RRDHOST *host, STRING *chart, STRING *name, uint32_t *next_event_id);
void sql_alert_transitions(
    DICTIONARY *nodes,
    time_t after,
    time_t before,
    const char *context,
    const char *alert_name,
    const char *transition,
    void (*cb)(struct sql_alert_transition_data *, void *),
    void *data,
    bool debug);

int sql_get_alert_configuration(
    DICTIONARY *configs,
    void (*cb)(struct sql_alert_config_data *, void *),
    void *data,
    bool debug __maybe_unused);

bool sql_find_alert_transition(const char *transition, void (*cb)(const char *machine_guid, const char *context, time_t alert_id, void *data), void *data);
void sql_alert_cleanup(bool cli);
#endif //NETDATA_SQLITE_HEALTH_H

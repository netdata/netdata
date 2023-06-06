// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_HEALTH_H
#define NETDATA_SQLITE_HEALTH_H
#include "../../daemon/common.h"
#include "sqlite3.h"

extern sqlite3 *db_meta;
void sql_health_alarm_log_load(RRDHOST *host);
int sql_create_health_log_table(RRDHOST *host);
void sql_health_alarm_log_update(RRDHOST *host, ALARM_ENTRY *ae);
void sql_health_alarm_log_insert(RRDHOST *host, ALARM_ENTRY *ae);
void sql_health_alarm_log_save(RRDHOST *host, ALARM_ENTRY *ae);
void sql_health_alarm_log_cleanup(RRDHOST *host);
int alert_hash_and_store_config(uuid_t hash_id, struct alert_config *cfg, int store_hash);
void sql_aclk_alert_clean_dead_entries(RRDHOST *host);
int sql_health_get_last_executed_event(RRDHOST *host, ALARM_ENTRY *ae, RRDCALC_STATUS *last_executed_status);
void sql_health_alarm_log2json(RRDHOST *host, BUFFER *wb, uint32_t after, char *chart);
#endif //NETDATA_SQLITE_HEALTH_H

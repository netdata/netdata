// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_HEALTH_H
#define NETDATA_SQLITE_HEALTH_H
#include "../../daemon/common.h"
#include "sqlite3.h"

extern sqlite3 *db_meta;
extern void sql_health_alarm_log_load(RRDHOST *host);
extern int sql_create_health_log_table(RRDHOST *host);
extern void sql_health_alarm_log_update(RRDHOST *host, ALARM_ENTRY *ae);
extern void sql_health_alarm_log_insert(RRDHOST *host, ALARM_ENTRY *ae);
extern void sql_health_alarm_log_save(RRDHOST *host, ALARM_ENTRY *ae);
extern void sql_health_alarm_log_cleanup(RRDHOST *host);
extern int alert_hash_and_store_config(uuid_t hash_id, struct alert_config *cfg);
extern void sql_aclk_alert_clean_dead_entries(RRDHOST *host);
#endif //NETDATA_SQLITE_HEALTH_H

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_ACLK_ALERT_H
#define NETDATA_SQLITE_ACLK_ALERT_H

extern sqlite3 *db_meta;

int aclk_add_alert_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void aclk_push_alert_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void aclk_send_alarm_health_log(char *node_id);
void aclk_push_alarm_health_log(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void aclk_send_alarm_configuration (char *config_hash);
int aclk_push_alert_config_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void aclk_start_alert_streaming(char *node_id, uint64_t batch_id, uint64_t start_seq_id);
int sql_queue_removed_alerts_to_aclk(RRDHOST *host);

#endif //NETDATA_SQLITE_ACLK_ALERT_H

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_ACLK_ALERT_H
#define NETDATA_SQLITE_ACLK_ALERT_H

extern sqlite3 *db_meta;

struct proto_alert_status {
    int alert_updates;
    uint64_t alerts_batch_id;
    uint64_t last_acked_sequence_id;
    uint64_t pending_min_sequence_id;
    uint64_t pending_max_sequence_id;
};

int aclk_add_alert_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void aclk_push_alert_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void aclk_send_alarm_health_log(char *node_id);
void aclk_push_alarm_health_log(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void aclk_send_alarm_configuration (char *config_hash);
int aclk_push_alert_config_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void aclk_start_alert_streaming(char *node_id, uint64_t batch_id, uint64_t start_seq_id);
void sql_queue_removed_alerts_to_aclk(RRDHOST *host);
void sql_process_queue_removed_alerts_to_aclk(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void aclk_push_alert_snapshot_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void aclk_process_send_alarm_snapshot(char *node_id, char *claim_id, uint64_t snapshot_id, uint64_t sequence_id);

#endif //NETDATA_SQLITE_ACLK_ALERT_H

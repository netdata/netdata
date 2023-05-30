// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_ACLK_ALERT_H
#define NETDATA_SQLITE_ACLK_ALERT_H

extern sqlite3 *db_meta;

#define SEND_REMOVED_AFTER_HEALTH_LOOPS 3
#define SEND_CHECKPOINT_AFTER_HEALTH_LOOPS 4

struct proto_alert_status {
    int alert_updates;
    uint64_t pending_min_sequence_id;
    uint64_t pending_max_sequence_id;
    uint64_t last_submitted_sequence_id;
};

int aclk_add_alert_event(struct aclk_sync_host_config *wc, struct aclk_database_cmd cmd);
void aclk_push_alert_event(struct aclk_sync_host_config *wc);
void aclk_send_alarm_configuration (char *config_hash);
int aclk_push_alert_config_event(char *node_id, char *config_hash);
void aclk_start_alert_streaming(char *node_id, bool resets);
void sql_queue_removed_alerts_to_aclk(RRDHOST *host);
void sql_process_queue_removed_alerts_to_aclk(char *node_id);
void aclk_send_alarm_checkpoint(char *node_id, char *claim_id);
void aclk_push_alarm_checkpoint(RRDHOST *host);

void aclk_push_alert_snapshot_event(char *node_id);
void aclk_process_send_alarm_snapshot(char *node_id, char *claim_id, char *snapshot_uuid);
int get_proto_alert_status(RRDHOST *host, struct proto_alert_status *proto_alert_status);
int sql_queue_alarm_to_aclk(RRDHOST *host, ALARM_ENTRY *ae, int skip_filter);
void aclk_push_alert_events_for_all_hosts(void);


#endif //NETDATA_SQLITE_ACLK_ALERT_H

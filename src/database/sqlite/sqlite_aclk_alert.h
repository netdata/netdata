// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_ACLK_ALERT_H
#define NETDATA_SQLITE_ACLK_ALERT_H

extern sqlite3 *db_meta;

struct proto_alert_status {
    int alert_updates;
    uint64_t pending_min_sequence_id;
    uint64_t pending_max_sequence_id;
    uint64_t last_submitted_sequence_id;
};

void aclk_send_alert_configuration(char *config_hash);
void aclk_push_alert_config_event(char *node_id, char *config_hash);
void aclk_start_alert_streaming(char *node_id, uint64_t cloud_version);
void aclk_alert_version_check(char *node_id, char *claim_id, uint64_t cloud_version);

void send_alert_snapshot_to_cloud(RRDHOST *host __maybe_unused);
void aclk_process_send_alarm_snapshot(char *node_id, char *claim_id, char *snapshot_uuid);
int get_proto_alert_status(RRDHOST *host, struct proto_alert_status *proto_alert_status);
bool process_alert_pending_queue(RRDHOST *host);
void aclk_push_alert_events_for_all_hosts(void);

#endif //NETDATA_SQLITE_ACLK_ALERT_H

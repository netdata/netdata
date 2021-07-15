// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_ACLK_CHART_H
#define NETDATA_SQLITE_ACLK_CHART_H


extern sqlite3 *db_meta;

extern int sql_queue_chart_to_aclk(RRDSET *st);
extern int sql_queue_dimension_to_aclk(RRDDIM *rd);
extern void sql_queue_alarm_to_aclk(RRDHOST *host, ALARM_ENTRY *ae);
extern void sql_create_aclk_table(RRDHOST *host, uuid_t *host_uuid, uuid_t *node_id);
int aclk_add_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
int aclk_add_dimension_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
int aclk_push_chart_config_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void sql_reset_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void aclk_reset_chart_event(char *node_id, uint64_t last_sequence_id);
void aclk_status_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void aclk_push_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void aclk_ack_chart_sequence_id(char *node_id, uint64_t last_sequence_id);
void aclk_get_chart_config(char **hash_id_list);
void aclk_start_streaming(char *node_id, uint64_t seq_id, time_t created_at, uint64_t batch_id);
void sql_set_chart_ack(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void sql_chart_deduplicate(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void sql_get_last_chart_sequence(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void sql_update_metric_statistics(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void sql_check_dimension_state(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void sql_check_rotation_state(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
int aclk_add_offline_dimension_event(struct aclk_database_worker_config *wc, char *node_id, char *chart_name, uuid_t *dim_uuid, char *rd_id, char *rd_name, time_t first_entry_t, time_t last_entry_t);
#endif //NETDATA_SQLITE_ACLK_CHART_H

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_ACLK_CHART_H
#define NETDATA_SQLITE_ACLK_CHART_H


typedef enum payload_type {
    ACLK_PAYLOAD_CHART,
    ACLK_PAYLOAD_DIMENSION,
    ACLK_PAYLOAD_DIMENSION_ROTATED
} ACLK_PAYLOAD_TYPE;

extern sqlite3 *db_meta;

#ifndef RRDSET_MINIMUM_DIM_LIVE_MULTIPLIER
#define RRDSET_MINIMUM_DIM_LIVE_MULTIPLIER   (3)
#endif

#ifndef ACLK_MAX_DIMENSION_CLEANUP
#define ACLK_MAX_DIMENSION_CLEANUP (500)
#endif

struct aclk_chart_sync_stats {
    int        updates;
    uint64_t   batch_id;
    uint64_t   min_seqid;
    uint64_t   max_seqid;
    uint64_t   min_seqid_pend;
    uint64_t   max_seqid_pend;
    uint64_t   min_seqid_sent;
    uint64_t   max_seqid_sent;
    uint64_t   min_seqid_ack;
    uint64_t   max_seqid_ack;
    time_t     max_date_created;
    time_t     max_date_submitted;
    time_t     max_date_ack;
};

extern int queue_chart_to_aclk(RRDSET *st);
extern void queue_dimension_to_aclk(RRDDIM *rd);
extern void sql_create_aclk_table(RRDHOST *host, uuid_t *host_uuid, uuid_t *node_id);
int aclk_add_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
int aclk_add_dimension_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
int aclk_send_chart_config(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void aclk_ack_chart_sequence_id(char *node_id, uint64_t last_sequence_id);
void aclk_get_chart_config(char **hash_id_list);
void aclk_send_chart_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void aclk_start_streaming(char *node_id, uint64_t seq_id, time_t created_at, uint64_t batch_id);
void sql_chart_deduplicate(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void sql_check_rotation_state(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void sql_get_last_chart_sequence(struct aclk_database_worker_config *wc);
void aclk_receive_chart_reset(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void aclk_receive_chart_ack(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
void aclk_process_dimension_deletion(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd);
uint32_t sql_get_pending_count(struct aclk_database_worker_config *wc);
void aclk_send_dimension_update(RRDDIM *rd);
struct aclk_chart_sync_stats *aclk_get_chart_sync_stats(RRDHOST *host);
void sql_check_chart_liveness(RRDSET *st);
#endif //NETDATA_SQLITE_ACLK_CHART_H

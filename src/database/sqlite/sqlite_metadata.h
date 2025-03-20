// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_METADATA_H
#define NETDATA_SQLITE_METADATA_H

#include "database/sqlite/vendored/sqlite3.h"
#include "sqlite_functions.h"

typedef enum event_log_type {
    EVENT_AGENT_START_TIME  = 1,
    EVENT_AGENT_SHUTDOWN_TIME,

    // terminator
    EVENT_AGENT_MAX,
} event_log_type_t;
void get_agent_event_time_median_init(void);

typedef enum db_check_action_type {
    DB_CHECK_NONE          = (1 << 0),
    DB_CHECK_RECLAIM_SPACE = (1 << 1),
    DB_CHECK_ANALYZE       = (1 << 2),
    DB_CHECK_CONT          = (1 << 3),
    DB_CHECK_RECOVER       = (1 << 4),
} db_check_action_type_t;

// To initialize and shutdown
void metadata_sync_init(void);
void metadata_sync_shutdown(void);
void metadata_sync_shutdown_prepare(void);

void metaqueue_delete_dimension_uuid(nd_uuid_t *uuid);
void metaqueue_store_claim_id(nd_uuid_t *host_uuid, nd_uuid_t *claim_uuid);
void metaqueue_ml_load_models(RRDDIM *rd);
void detect_machine_guid_change(nd_uuid_t *host_uuid);
void metadata_queue_load_host_context(RRDHOST *host);
void vacuum_database(sqlite3 *database, const char *db_alias, int threshold, int vacuum_pc);

int sql_metadata_cache_stats(int op);

int get_node_id(nd_uuid_t *host_id, nd_uuid_t *node_id);
void sql_update_node_id(nd_uuid_t *host_id, nd_uuid_t *node_id);
void sql_load_node_id(RRDHOST *host);

// Help build archived hosts in memory when agent starts
void sql_build_host_system_info(nd_uuid_t *host_id, struct rrdhost_system_info *system_info);
void invalidate_node_instances(nd_uuid_t *host_id, nd_uuid_t *claim_id);
RRDLABELS *sql_load_host_labels(nd_uuid_t *host_id);
bool sql_set_host_label(nd_uuid_t *host_id, const char *label_key, const char *label_value);

uint64_t sqlite_get_meta_space(void);
int sql_init_meta_database(db_check_action_type_t rebuild, int memory);

void cleanup_agent_event_log(void);
void add_agent_event(event_log_type_t event_id, int64_t value);
usec_t get_agent_event_time_median(event_log_type_t event_id);
void metadata_queue_ae_save(RRDHOST *host, ALARM_ENTRY *ae);
void metadata_queue_ae_deletion(ALARM_ENTRY *ae);
void commit_alert_transitions(RRDHOST *host);

void metadata_sync_shutdown_background(void);
void metadata_sync_shutdown_background_wait(void);
void metadata_queue_ctx_host_cleanup(nd_uuid_t *host_uuid, const char *context);
void store_host_info_and_metadata(RRDHOST *host, BUFFER *work_buffer, size_t *query_counter);
void metadata_execute_store_statement(sqlite3_stmt *stmt);

// UNIT TEST
int metadata_unittest(void);
#endif //NETDATA_SQLITE_METADATA_H

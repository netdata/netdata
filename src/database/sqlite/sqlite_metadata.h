// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_METADATA_H
#define NETDATA_SQLITE_METADATA_H

#include "sqlite3.h"
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

void metaqueue_delete_dimension_uuid(nd_uuid_t *uuid);

// Dimensions of one host whose data expired from every tier, found by the contexts garbage collection.
// The metadata thread deletes their rows, unless a live dimension uses the uuid again.
struct metadata_expired_dimension {
    nd_uuid_t uuid;
    STRING *chart_id;
    STRING *dim_id;
};

typedef struct metadata_expired_dimensions {
    char machine_guid[GUID_LEN + 1];
    size_t used;    // records added
    size_t done;    // records the metadata thread has processed
    size_t size;    // records allocated
    uint8_t delete_failures;    // consecutive failed deletes of the next record
    struct metadata_expired_dimension *items;
    struct metadata_expired_dimensions *prev, *next;
} METADATA_EXPIRED_DIMENSIONS;

METADATA_EXPIRED_DIMENSIONS *metadata_expired_dimensions_create(const char *machine_guid);
void metadata_expired_dimensions_add(METADATA_EXPIRED_DIMENSIONS *batch, UUIDMAP_ID uuid, STRING *chart_id, STRING *dim_id);
void metadata_expired_dimensions_free(METADATA_EXPIRED_DIMENSIONS *batch);
void metaqueue_delete_expired_dimensions(METADATA_EXPIRED_DIMENSIONS *batch);
void metaqueue_store_claim_id(nd_uuid_t *host_uuid, nd_uuid_t *claim_uuid);
void metaqueue_ml_load_models(RRDDIM *rd);
void detect_machine_guid_change(nd_uuid_t *host_uuid);
bool metadata_queue_load_host_context();
void reset_host_context_load_flag();
void vacuum_database(sqlite3 *database, const char *db_alias, int threshold, int vacuum_pc, time_t *next_run);

int sql_metadata_cache_stats(int op);

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
bool metadata_queue_ae_save(RRDHOST *host, ALARM_ENTRY *ae);
void metadata_queue_ae_deletion(ALARM_ENTRY *ae);
void commit_alert_transitions(RRDHOST *host);

//void metadata_sync_shutdown_background(void);
//void metadata_sync_shutdown_background_wait(void);
void metadata_queue_ctx_host_cleanup(nd_uuid_t *host_uuid, const char *context);
void store_host_info_and_metadata(RRDHOST *host);
void metadata_execute_store_statement(sqlite3_stmt *stmt);
size_t populate_metrics_from_database(void *mrg, void (*populate_cb)(void *mrg, Word_t section, nd_uuid_t *uuid));

// UNIT TEST
int metadata_unittest(void);
int metadata_expired_dimensions_unittest(void);
#endif //NETDATA_SQLITE_METADATA_H

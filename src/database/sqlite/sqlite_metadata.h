// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_METADATA_H
#define NETDATA_SQLITE_METADATA_H

#include "sqlite3.h"
#include "sqlite_functions.h"

// return a node list
struct node_instance_list {
    uuid_t  node_id;
    uuid_t  host_id;
    char *hostname;
    int live;
    int queryable;
    int hops;
};

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

void metaqueue_delete_dimension_uuid(uuid_t *uuid);
void metaqueue_store_claim_id(uuid_t *host_uuid, uuid_t *claim_uuid);
void metaqueue_host_update_info(RRDHOST *host);
void metaqueue_ml_load_models(RRDDIM *rd);
void detect_machine_guid_change(uuid_t *host_uuid);
void metadata_queue_load_host_context(RRDHOST *host);
void metadata_delete_host_chart_labels(char *machine_guid);
void vacuum_database(sqlite3 *database, const char *db_alias, int threshold, int vacuum_pc);

int sql_metadata_cache_stats(int op);

int get_node_id(uuid_t *host_id, uuid_t *node_id);
int update_node_id(uuid_t *host_id, uuid_t *node_id);
struct node_instance_list *get_node_list(void);
void sql_load_node_id(RRDHOST *host);

// Help build archived hosts in memory when agent starts
void sql_build_host_system_info(uuid_t *host_id, struct rrdhost_system_info *system_info);
void invalidate_node_instances(uuid_t *host_id, uuid_t *claim_id);
RRDLABELS *sql_load_host_labels(uuid_t *host_id);

uint64_t sqlite_get_meta_space(void);
int sql_init_meta_database(db_check_action_type_t rebuild, int memory);
void sql_close_meta_database(void);

// UNIT TEST
int metadata_unittest(void);
#endif //NETDATA_SQLITE_METADATA_H

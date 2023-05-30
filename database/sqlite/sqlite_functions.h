// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_FUNCTIONS_H
#define NETDATA_SQLITE_FUNCTIONS_H

#include "daemon/common.h"
#include "sqlite3.h"

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
    DB_CHECK_NONE  = 0x0000,
    DB_CHECK_INTEGRITY  = 0x0001,
    DB_CHECK_FIX_DB = 0x0002,
    DB_CHECK_RECLAIM_SPACE = 0x0004,
    DB_CHECK_CONT = 0x00008
} db_check_action_type_t;

#define SQL_MAX_RETRY (100)
#define SQLITE_INSERT_DELAY (10)        // Insert delay in case of lock

#define CHECK_SQLITE_CONNECTION(db_meta)                                                                               \
    if (unlikely(!db_meta)) {                                                                                          \
        if (default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE) {                                                     \
            return 1;                                                                                                  \
        }                                                                                                              \
        error_report("Database has not been initialized");                                                             \
        return 1;                                                                                                      \
    }

SQLITE_API int sqlite3_step_monitored(sqlite3_stmt *stmt);
SQLITE_API int sqlite3_exec_monitored(
    sqlite3 *db,                               /* An open database */
    const char *sql,                           /* SQL to be evaluated */
    int (*callback)(void*,int,char**,char**),  /* Callback function */
    void *data,                                /* 1st argument to callback */
    char **errmsg                              /* Error msg written here */
    );

// Initialization and shutdown
int init_database_batch(sqlite3 *database, int rebuild, int init_type, const char *batch[]);
int sql_init_database(db_check_action_type_t rebuild, int memory);
void sql_close_database(void);

// Helpers
int bind_text_null(sqlite3_stmt *res, int position, const char *text, bool can_be_null);
int prepare_statement(sqlite3 *database, const char *query, sqlite3_stmt **statement);
int execute_insert(sqlite3_stmt *res);
int exec_statement_with_uuid(const char *sql, uuid_t *uuid);
int db_execute(sqlite3 *database, const char *cmd);
void initialize_thread_key_pool(void);

// Look up functions
int get_node_id(uuid_t *host_id, uuid_t *node_id);
int get_host_id(uuid_t *node_id, uuid_t *host_id);
struct node_instance_list *get_node_list(void);
void sql_load_node_id(RRDHOST *host);
char *get_hostname_by_node_id(char *node_id);

// Help build archived hosts in memory when agent starts
void sql_build_host_system_info(uuid_t *host_id, struct rrdhost_system_info *system_info);
DICTIONARY *sql_load_host_labels(uuid_t *host_id);

// TODO: move to metadata
int update_node_id(uuid_t *host_id, uuid_t *node_id);

void invalidate_node_instances(uuid_t *host_id, uuid_t *claim_id);

// Provide statistics
int sql_metadata_cache_stats(int op);

#endif //NETDATA_SQLITE_FUNCTIONS_H

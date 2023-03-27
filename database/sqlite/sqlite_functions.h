// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_FUNCTIONS_H
#define NETDATA_SQLITE_FUNCTIONS_H

#include "daemon/common.h"
#include "sqlite3.h"

typedef enum db_check_action_type {
    DB_CHECK_NONE  = 0x0000,
    DB_CHECK_INTEGRITY  = 0x0001,
    DB_CHECK_FIX_DB = 0x0002,
    DB_CHECK_RECLAIM_SPACE = 0x0004,
    DB_CHECK_CONT = 0x00008
} db_check_action_type_t;

#define SQL_MAX_RETRY (100)
#define SQLITE_INSERT_DELAY (10)        // Insert delay in case of lock

/*
#define CHECK_SQLITE_CONNECTION(db_meta)                                                                               \
    if (unlikely(!db_meta)) {                                                                                          \
        if (default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE) {                                                     \
            return 1;                                                                                                  \
        }                                                                                                              \
        error_report("Database has not been initialized");                                                             \
        return 1;                                                                                                      \
    }
*/

#define SQLITE_LOOKASIDE_SLOTS 192
#define SQLITE_LOOKASIDE_SLOT_SIZE 8192

SQLITE_API int sqlite3_step_monitored(sqlite3_stmt *stmt);
SQLITE_API int sqlite3_exec_monitored(
    sqlite3 *db,                               /* An open database */
    const char *sql,                           /* SQL to be evaluated */
    int (*callback)(void*,int,char**,char**),  /* Callback function */
    void *data,                                /* 1st argument to callback */
    char **errmsg                              /* Error msg written here */
    );

// Initialization and shutdown
int sqlite_library_init(void);
void sqlite_library_shutdown(void);
int sqlite_init_databases(db_check_action_type_t mode, bool memory_mode);
void sqlite_close_databases(void);

int init_database_batch(sqlite3 *database, const char *batch[]);
int sql_init_metadata_database(db_check_action_type_t rebuild, int memory);

// Helpers
int bind_text_null(sqlite3_stmt *res, int position, const char *text, bool can_be_null);
int prepare_statement(sqlite3 *database, const char *query, sqlite3_stmt **statement);
int execute_insert(sqlite3_stmt *res);
int exec_statement_with_uuid(sqlite3 *database, const char *sql, uuid_t(*uuid));
int db_execute(sqlite3 *database, const char *cmd);
int configure_database_params(sqlite3 *database, int target_version);
int database_set_version(sqlite3 *database, int target_version);
int attach_database(sqlite3 *database, const char *database_path, const char *alias);
void add_stmt_to_list(sqlite3_stmt *res);
void init_sqlite_thread_key_pool(void);
int check_table_integrity(sqlite3 *database, char *table);

void invalidate_node_instances(uuid_t *host_id, uuid_t *claim_id);

#endif //NETDATA_SQLITE_FUNCTIONS_H

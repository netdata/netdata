// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_FUNCTIONS_H
#define NETDATA_SQLITE_FUNCTIONS_H

#include "daemon/common.h"
#include "sqlite3.h"

void analytics_set_data_str(char **name, const char *value);

#define SQLITE_BIND_FAIL(label, rc)                                                                                    \
    do {                                                                                                               \
        if ((rc) != SQLITE_OK)                                                                                         \
            goto label;                                                                                                \
    } while (0)

#define REPORT_BIND_FAIL(res, param)                                                                                   \
    do {                                                                                                               \
        if (unlikely((param))) {                                                                                       \
            const char *failed_param = sqlite3_bind_parameter_name((res), (param));                                    \
            nd_log(                                                                                                    \
                NDLS_DAEMON,                                                                                           \
                NDLP_ERR,                                                                                              \
                "Failed to bind parameter %d (%s) in %s",                                                              \
                (param),                                                                                               \
                failed_param ? failed_param : "?",                                                                     \
                __FUNCTION__);                                                                                         \
        }                                                                                                              \
    } while (0)

#define SQLITE_FINALIZE(res)                                                                                           \
    do {                                                                                                               \
        if ((res)) {                                                                                                   \
            int _rc = sqlite3_finalize((res));                                                                         \
            if (_rc != SQLITE_OK) {                                                                                    \
                nd_log(NDLS_DAEMON, NDLP_ERR, "Failed to finalize statement rc=%d in %s", _rc, __FUNCTION__);          \
            }                                                                                                          \
        }                                                                                                              \
    } while (0)

#define SQLITE_RESET(res)                                                                                              \
    do {                                                                                                               \
        if ((res)) {                                                                                                   \
            int _rc = sqlite3_reset((res));                                                                            \
            if (_rc != SQLITE_OK) {                                                                                    \
                nd_log(NDLS_DAEMON, NDLP_ERR, "Failed to reset statement rc=%d in %s", _rc, __FUNCTION__);             \
            }                                                                                                          \
        }                                                                                                              \
    } while (0)

#define SQL_MAX_RETRY (100)
#define SQLITE_INSERT_DELAY (10)        // Insert delay in case of lock

SQLITE_API int sqlite3_step_monitored(sqlite3_stmt *stmt);
SQLITE_API int sqlite3_exec_monitored(
    sqlite3 *db,                               /* An open database */
    const char *sql,                           /* SQL to be evaluated */
    int (*callback)(void*,int,char**,char**),  /* Callback function */
    void *data,                                /* 1st argument to callback */
    char **errmsg                              /* Error msg written here */
    );

// Initialization and shutdown
int init_database_batch(sqlite3 *database, const char *batch[], const char *description);
int configure_sqlite_database(sqlite3 *database, int target_version, const char *description);

// Helpers
int bind_text_null(sqlite3_stmt *res, int position, const char *text, bool can_be_null);
int prepare_statement(sqlite3 *database, const char *query, sqlite3_stmt **statement);
int execute_insert(sqlite3_stmt *res);
int db_execute(sqlite3 *database, const char *cmd);
char *get_database_extented_error(sqlite3 *database, int i, const char *description);

void sql_drop_table(const char *table);
void sqlite_now_usec(sqlite3_context *context, int argc, sqlite3_value **argv);

uint64_t sqlite_get_db_space(sqlite3 *db);

int get_free_page_count(sqlite3 *database);
int get_database_page_count(sqlite3 *database);

int sqlite_library_init(void);
void sqlite_library_shutdown(void);

void sql_close_database(sqlite3 *database, const char *database_name);
void sqlite_close_databases(void);
#endif //NETDATA_SQLITE_FUNCTIONS_H

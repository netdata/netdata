// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"

#define MAX_PREPARED_STATEMENTS (32)
pthread_key_t key_pool[MAX_PREPARED_STATEMENTS];

SQLITE_API int sqlite3_exec_monitored(
    sqlite3 *db,                               /* An open database */
    const char *sql,                           /* SQL to be evaluated */
    int (*callback)(void*,int,char**,char**),  /* Callback function */
    void *data,                                /* 1st argument to callback */
    char **errmsg                              /* Error msg written here */
) {
    int rc = sqlite3_exec(db, sql, callback, data, errmsg);
    global_statistics_sqlite3_query_completed(rc == SQLITE_OK, rc == SQLITE_BUSY, rc == SQLITE_LOCKED);
    return rc;
}

SQLITE_API int sqlite3_step_monitored(sqlite3_stmt *stmt) {
    int rc;
    int cnt = 0;

    while (cnt++ < SQL_MAX_RETRY) {
        rc = sqlite3_step(stmt);
        switch (rc) {
            case SQLITE_DONE:
                global_statistics_sqlite3_query_completed(1, 0, 0);
                break;
            case SQLITE_ROW:
                global_statistics_sqlite3_row_completed();
                break;
            case SQLITE_BUSY:
            case SQLITE_LOCKED:
                global_statistics_sqlite3_query_completed(rc == SQLITE_DONE, rc == SQLITE_BUSY, rc == SQLITE_LOCKED);
                usleep(SQLITE_INSERT_DELAY * USEC_PER_MS);
                continue;
            default:
                break;
        }
        break;
    }
    return rc;
}

int execute_insert(sqlite3_stmt *res)
{
    int rc;
    int cnt = 0;
    while ((rc = sqlite3_step_monitored(res)) != SQLITE_DONE && ++cnt < SQL_MAX_RETRY && likely(!netdata_exit)) {
        if (likely(rc == SQLITE_BUSY || rc == SQLITE_LOCKED)) {
            usleep(SQLITE_INSERT_DELAY * USEC_PER_MS);
            error_report("Failed to insert/update, rc = %d -- attempt %d", rc, cnt);
        }
        else {
            error_report("SQLite error %d", rc);
            break;
        }
    }

    return rc;
}

#define MAX_OPEN_STATEMENTS (512)

void add_stmt_to_list(sqlite3_stmt *res)
{
    static int idx = 0;
    static sqlite3_stmt *statements[MAX_OPEN_STATEMENTS];

    if (unlikely(!res)) {
        if (idx)
            info("SQLITE: Finilizing %d statements", idx);
        else
            info("SQLITE: No statements pending to finalize");
        while (idx > 0) {
            int rc;
            rc = sqlite3_finalize(statements[--idx]);
            if (unlikely(rc != SQLITE_OK))
                error_report("Failed to finalize statement during shutdown, rc = %d", rc);
        }
        return;
    }

    if (unlikely(idx == MAX_OPEN_STATEMENTS))
        return;
}

static void release_statement(void *statement)
{
    int rc;
    if (unlikely(rc = sqlite3_finalize((sqlite3_stmt *) statement) != SQLITE_OK))
        error_report("Failed to finalize statement, rc = %d", rc);
}

void init_sqlite_thread_key_pool(void)
{
    for (int i = 0; i < MAX_PREPARED_STATEMENTS; i++)
        (void)pthread_key_create(&key_pool[i], release_statement);
}

int prepare_statement(sqlite3 *database, const char *query, sqlite3_stmt **statement)
{
    static __thread uint32_t keys_used = 0;

    pthread_key_t *key = NULL;
    int ret = 1;

    if (likely(keys_used < MAX_PREPARED_STATEMENTS))
        key = &key_pool[keys_used++];

    int rc = sqlite3_prepare_v2(database, query, -1, statement, 0);
    if (likely(rc == SQLITE_OK)) {
        if (likely(key))
            ret = pthread_setspecific(*key, *statement);
        if (ret)
            add_stmt_to_list(*statement);
    }
    return rc;
}

static int check_table_integrity_cb(void *data, int argc, char **argv, char **column)
{
    int *status = data;
    UNUSED(argc);
    UNUSED(column);
    info("---> %s", argv[0]);
    *status = (strcmp(argv[0], "ok") != 0);
    return 0;
}

int check_table_integrity(sqlite3 *database, char *table)
{
    int status = 0;
    char *err_msg = NULL;
    char wstr[255];

    if (table) {
        info("Checking table %s", table);
        snprintfz(wstr, 254, "PRAGMA integrity_check(%s);", table);
    }
    else {
        info("Checking entire database");
        strcpy(wstr,"PRAGMA integrity_check;");
    }

    int rc = sqlite3_exec_monitored(database, wstr, check_table_integrity_cb, (void *)&status, &err_msg);
    if (rc != SQLITE_OK) {
        error_report("SQLite error during database integrity check for %s, rc = %d (%s)",
                     table ? table : "the entire database", rc, err_msg);
        sqlite3_free(err_msg);
    }

    return status;
}

int attach_database(sqlite3 *database, const char *database_path, const char *alias)
{
    char buf[FILENAME_MAX + 129] = "";
    const char *list[2] = { buf, NULL };

    if (likely(database_path))
        snprintfz(buf, FILENAME_MAX + 128, "ATTACH DATABASE \"%s/%s\" AS %s", netdata_configured_cache_dir, database_path, alias);
    else
        snprintfz(buf, FILENAME_MAX + 128, "ATTACH DATABASE ':memory:' AS %s", alias);

    return init_database_batch(database, list);
}

int configure_database_params(sqlite3 *database, int target_version)
{
    char buf[1024 + 1] = "";
    const char *list[2] = { buf, NULL };

    // https://www.sqlite.org/pragma.html#pragma_auto_vacuum
    // PRAGMA schema.auto_vacuum = 0 | NONE | 1 | FULL | 2 | INCREMENTAL;
    snprintfz(buf, 1024, "PRAGMA auto_vacuum=%s;", config_get(CONFIG_SECTION_SQLITE, "auto vacuum", "INCREMENTAL"));
    if(init_database_batch(database, list)) return 1;

    // https://www.sqlite.org/pragma.html#pragma_synchronous
    // PRAGMA schema.synchronous = 0 | OFF | 1 | NORMAL | 2 | FULL | 3 | EXTRA;
    snprintfz(buf, 1024, "PRAGMA synchronous=%s;", config_get(CONFIG_SECTION_SQLITE, "synchronous", "NORMAL"));
    if(init_database_batch(database, list)) return 1;

    // https://www.sqlite.org/pragma.html#pragma_journal_mode
    // PRAGMA schema.journal_mode = DELETE | TRUNCATE | PERSIST | MEMORY | WAL | OFF
    snprintfz(buf, 1024, "PRAGMA journal_mode=%s;", config_get(CONFIG_SECTION_SQLITE, "journal mode", "WAL"));
    if(init_database_batch(database, list)) return 1;

    // https://www.sqlite.org/pragma.html#pragma_temp_store
    // PRAGMA temp_store = 0 | DEFAULT | 1 | FILE | 2 | MEMORY;
    snprintfz(buf, 1024, "PRAGMA temp_store=%s;", config_get(CONFIG_SECTION_SQLITE, "temp store", "MEMORY"));
    if(init_database_batch(database, list)) return 1;

    // https://www.sqlite.org/pragma.html#pragma_journal_size_limit
    // PRAGMA schema.journal_size_limit = N ;
    snprintfz(buf, 1024, "PRAGMA journal_size_limit=%lld;", config_get_number(CONFIG_SECTION_SQLITE, "journal size limit", 16777216));
    if(init_database_batch(database, list)) return 1;

    // https://www.sqlite.org/pragma.html#pragma_cache_size
    // PRAGMA schema.cache_size = pages;
    // PRAGMA schema.cache_size = -kibibytes;
    snprintfz(buf, 1024, "PRAGMA cache_size=%lld;", config_get_number(CONFIG_SECTION_SQLITE, "cache size", -2000));
    if(init_database_batch(database, list)) return 1;

    return 0;
}

int database_set_version(sqlite3 *database, int target_version)
{
    char buf[1024 + 1] = "";
    const char *list[2] = { buf, NULL };

    snprintfz(buf, 1024, "PRAGMA user_version=%d;", target_version);
    if(init_database_batch(database, list)) return 1;

    return 0;
}

int exec_statement_with_uuid(sqlite3 *database, const char *sql, uuid_t(*uuid))
{
    int rc, result = 1;
    sqlite3_stmt *res = NULL;

    rc = sqlite3_prepare_v2(database, sql, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement %s, rc = %d", sql, rc);
        return 1;
    }

    rc = sqlite3_bind_blob(res, 1, uuid, sizeof(*uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host parameter to %s, rc = %d", sql, rc);
        goto skip;
    }

    rc = execute_insert(res);
    if (likely(rc == SQLITE_DONE))
        result = SQLITE_OK;
    else
        error_report("Failed to execute %s, rc = %d", sql, rc);

skip:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement %s, rc = %d", sql, rc);
    return result;
}

// Return 0 OK
// Return 1 Failed
int db_execute(sqlite3 *database, const char *cmd)
{
    int rc;
    int cnt = 0;
    while (cnt < SQL_MAX_RETRY) {
        char *err_msg;
        rc = sqlite3_exec_monitored(database, cmd, 0, 0, &err_msg);
        if (rc != SQLITE_OK) {
            error_report("Failed to execute '%s', rc = %d (%s) -- attempt %d", cmd, rc, err_msg, cnt);
            sqlite3_free(err_msg);
            if (likely(rc == SQLITE_BUSY || rc == SQLITE_LOCKED)) {
                usleep(SQLITE_INSERT_DELAY * USEC_PER_MS);
            }
            else
                break;
        }
        else
            break;

        ++cnt;
    }
    return (rc != SQLITE_OK);
}

// Utils
int bind_text_null(sqlite3_stmt *res, int position, const char *text, bool can_be_null)
{
    if (likely(text))
        return sqlite3_bind_text(res, position, text, -1, SQLITE_STATIC);
    if (!can_be_null)
        return 1;
    return sqlite3_bind_null(res, position);
}

void sql_close_database(sqlite3 *database, const char *database_name)
{
    int rc;
    if (unlikely(!database))
        return;

    info("%s: Closing sqlite database", database_name);

    int t_count_used,t_count_hit,t_count_miss,t_count_full, dummy;
    (void) sqlite3_db_status(database, SQLITE_DBSTATUS_LOOKASIDE_USED, &dummy, &t_count_used, 0);
    (void) sqlite3_db_status(database, SQLITE_DBSTATUS_LOOKASIDE_HIT, &dummy,&t_count_hit, 0);
    (void) sqlite3_db_status(database, SQLITE_DBSTATUS_LOOKASIDE_MISS_SIZE, &dummy,&t_count_miss, 0);
    (void) sqlite3_db_status(database, SQLITE_DBSTATUS_LOOKASIDE_MISS_FULL, &dummy,&t_count_full, 0);

    info("%s: Database lookaside allocation statistics: Used slots %d, Hit %d, Misses due to small slot size %d, Misses due to slots full %d", database_name,
         t_count_used,t_count_hit, t_count_miss, t_count_full);

#ifdef NETDATA_DEV_MODE
    (void) sqlite3_db_release_memory(database);
#endif

    rc = sqlite3_close_v2(database);
    if (unlikely(rc != SQLITE_OK))
        error_report("%s: Error while closing the sqlite database: rc %d, error \"%s\"", database_name, rc, sqlite3_errstr(rc));
}

int sqlite_init_databases(db_check_action_type_t mode, bool memory_mode)
{
    if (unlikely(sql_init_metadata_database(mode, memory_mode))) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            fatal("Failed to initialize metadata database");
        info("Skipping SQLITE metadata initialization since memory mode is not dbengine");
    }

    if (unlikely(sql_init_health_database(memory_mode))) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            fatal("Failed to initialize health database");
        info("Skipping SQLITE health initialization since memory mode is not dbengine");
    }

    if (unlikely(sql_init_aclk_database(memory_mode))) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            fatal("Failed to initialize aclk database");
        info("Skipping SQLITE aclk initialization since memory mode is not dbengine");
    }

    if (unlikely(sql_init_context_database(memory_mode))) {
        error_report("Failed to initialize context metadata database");
    }

    return 0;
}

void sqlite_close_databases(void)
{
    add_stmt_to_list(NULL);

    sql_close_database(db_context_meta, "CONTEXT");
    sql_close_database(db_health,"HEALTH");
    sql_close_database(db_aclk, "ACLK");
    sql_close_database(db_meta,"METADATA");
}



// Initialize sqlite library
int sqlite_library_init(void)
{
    int lookaside_slots = config_get_number(CONFIG_SECTION_SQLITE, "look aside buffer slots", SQLITE_LOOKASIDE_SLOTS);
    int lookaside_slot_size = config_get_number(CONFIG_SECTION_SQLITE, "look aside buffer slot size", SQLITE_LOOKASIDE_SLOT_SIZE);

    (void) sqlite3_config(SQLITE_CONFIG_LOOKASIDE,  lookaside_slot_size, lookaside_slots);

    int rc = sqlite3_initialize();

    if (SQLITE_OK != rc)
        return 1;

    return 0;
}

void sqlite_library_shutdown(void)
{
    (void) sqlite3_shutdown();
}

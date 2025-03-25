// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"

#define MAX_PREPARED_STATEMENTS (32)
pthread_key_t key_pool[MAX_PREPARED_STATEMENTS];

long long def_journal_size_limit = 16777216;

SQLITE_API int sqlite3_exec_monitored(
    sqlite3 *db,                               /* An open database */
    const char *sql,                           /* SQL to be evaluated */
    int (*callback)(void*,int,char**,char**),  /* Callback function */
    void *data,                                /* 1st argument to callback */
    char **errmsg                              /* Error msg written here */
) {
    internal_fatal(!nd_thread_runs_sql(), "THIS THREAD CANNOT RUN SQL");

    int rc = sqlite3_exec(db, sql, callback, data, errmsg);
    pulse_sqlite3_query_completed(rc == SQLITE_OK, rc == SQLITE_BUSY, rc == SQLITE_LOCKED);
    return rc;
}

SQLITE_API int sqlite3_step_monitored(sqlite3_stmt *stmt) {
    internal_fatal(!nd_thread_runs_sql(), "THIS THREAD CANNOT RUN SQL");

    int rc;
    int cnt = 0;

    while (cnt++ < SQL_MAX_RETRY) {
        rc = sqlite3_step(stmt);
        switch (rc) {
            case SQLITE_DONE:
                pulse_sqlite3_query_completed(1, 0, 0);
                break;
            case SQLITE_ROW:
                pulse_sqlite3_row_completed();
                break;
            case SQLITE_BUSY:
            case SQLITE_LOCKED:
                pulse_sqlite3_query_completed(false, rc == SQLITE_BUSY, rc == SQLITE_LOCKED);
                usleep(SQLITE_INSERT_DELAY * USEC_PER_MS);
                continue;
            default:
                break;
        }
        break;
    }
    return rc;
}

static bool mark_database_to_recover(sqlite3_stmt *res, sqlite3 *database, int rc)
{

    if (!res && !database)
        return false;

    if (!database)
        database = sqlite3_db_handle(res);

    if (db_meta == database) {
        char recover_file[FILENAME_MAX + 1];
        snprintfz(recover_file, FILENAME_MAX, "%s/.netdata-meta.db.%s", netdata_configured_cache_dir, SQLITE_CORRUPT == rc ? "recover" : "delete" );
        int fd = open(recover_file, O_WRONLY | O_CREAT | O_TRUNC | O_CLOEXEC, 444);
        if (fd >= 0) {
            close(fd);
            return true;
        }
    }
    return false;
}

int execute_insert(sqlite3_stmt *res) {
    int rc;
    rc =  sqlite3_step_monitored(res);
    if (rc == SQLITE_CORRUPT) {
        (void)mark_database_to_recover(res, NULL, rc);
        error_report("SQLite error %d", rc);
    }
    return rc;
}

int configure_sqlite_database(sqlite3 *database, int target_version, const char *description)
{
    char buf[1024 + 1] = "";
    const char *list[2] = { buf, NULL };

    const char *def_auto_vacuum = "INCREMENTAL";
    const char *def_synchronous = "NORMAL";
    const char *def_journal_mode = "WAL";
    const char *def_temp_store = "MEMORY";
    long long def_cache_size = -2000;

    // https://www.sqlite.org/pragma.html#pragma_auto_vacuum
    // PRAGMA schema.auto_vacuum = 0 | NONE | 1 | FULL | 2 | INCREMENTAL;
    snprintfz(buf, sizeof(buf) - 1, "PRAGMA auto_vacuum=%s", def_auto_vacuum);
    if (inicfg_exists(&netdata_config, CONFIG_SECTION_SQLITE, "auto vacuum"))
        snprintfz(buf, sizeof(buf) - 1, "PRAGMA auto_vacuum=%s",inicfg_get(&netdata_config, CONFIG_SECTION_SQLITE, "auto vacuum", def_auto_vacuum));
    if (init_database_batch(database, list, description))
        return 1;

    // https://www.sqlite.org/pragma.html#pragma_synchronous
    // PRAGMA schema.synchronous = 0 | OFF | 1 | NORMAL | 2 | FULL | 3 | EXTRA;
    snprintfz(buf, sizeof(buf) - 1, "PRAGMA synchronous=%s", def_synchronous);
    if (inicfg_exists(&netdata_config, CONFIG_SECTION_SQLITE, "synchronous"))
        snprintfz(buf, sizeof(buf) - 1, "PRAGMA synchronous=%s", inicfg_get(&netdata_config, CONFIG_SECTION_SQLITE, "synchronous", def_synchronous));
    if (init_database_batch(database, list, description))
        return 1;

    // https://www.sqlite.org/pragma.html#pragma_journal_mode
    // PRAGMA schema.journal_mode = DELETE | TRUNCATE | PERSIST | MEMORY | WAL | OFF
    snprintfz(buf, sizeof(buf) - 1, "PRAGMA journal_mode=%s", def_journal_mode);
    if (inicfg_exists(&netdata_config, CONFIG_SECTION_SQLITE, "journal mode"))
        snprintfz(buf, sizeof(buf) - 1, "PRAGMA journal_mode=%s", inicfg_get(&netdata_config, CONFIG_SECTION_SQLITE, "journal mode", def_journal_mode));
    if (init_database_batch(database, list, description))
        return 1;

    // https://www.sqlite.org/pragma.html#pragma_temp_store
    // PRAGMA temp_store = 0 | DEFAULT | 1 | FILE | 2 | MEMORY;
    snprintfz(buf, sizeof(buf) - 1, "PRAGMA temp_store=%s", def_temp_store);
    if (inicfg_exists(&netdata_config, CONFIG_SECTION_SQLITE, "temp store"))
        snprintfz(buf, sizeof(buf) - 1, "PRAGMA temp_store=%s", inicfg_get(&netdata_config, CONFIG_SECTION_SQLITE, "temp store", def_temp_store));
    if (init_database_batch(database, list, description))
        return 1;

    // https://www.sqlite.org/pragma.html#pragma_journal_size_limit
    // PRAGMA schema.journal_size_limit = N ;
    snprintfz(buf, sizeof(buf) - 1, "PRAGMA journal_size_limit=%lld", def_journal_size_limit);
    if (inicfg_exists(&netdata_config, CONFIG_SECTION_SQLITE, "journal size limit")) {
        def_journal_size_limit = inicfg_get_number(&netdata_config, CONFIG_SECTION_SQLITE, "journal size limit", def_journal_size_limit);
        snprintfz(buf, sizeof(buf) - 1, "PRAGMA journal_size_limit=%lld", def_journal_size_limit);
    }
    if (init_database_batch(database, list, description))
        return 1;

    // https://www.sqlite.org/pragma.html#pragma_cache_size
    // PRAGMA schema.cache_size = pages;
    // PRAGMA schema.cache_size = -kibibytes;
    snprintfz(buf, sizeof(buf) - 1, "PRAGMA cache_size=%lld", def_cache_size);
    if (inicfg_exists(&netdata_config, CONFIG_SECTION_SQLITE, "cache size"))
        snprintfz(buf, sizeof(buf) - 1, "PRAGMA cache_size=%lld", inicfg_get_number(&netdata_config, CONFIG_SECTION_SQLITE, "cache size", def_cache_size));
    if (init_database_batch(database, list, description))
        return 1;

    snprintfz(buf, sizeof(buf) - 1, "PRAGMA user_version=%d", target_version);
    if (init_database_batch(database, list, description))
        return 1;

    snprintfz(buf, sizeof(buf) - 1, "PRAGMA optimize=0x10002");
    if (init_database_batch(database, list, description))
        return 1;

    return 0;
}

#define MAX_OPEN_STATEMENTS (512)

static void add_stmt_to_list(sqlite3_stmt *res)
{
    static int idx = 0;
    static sqlite3_stmt *statements[MAX_OPEN_STATEMENTS];

    if (unlikely(!res)) {
        if (idx)
            netdata_log_info("Finilizing %d statements", idx);
        else
            netdata_log_info("No statements pending to finalize");
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

static void initialize_thread_key_pool(void)
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
    if (rc == SQLITE_OK) {
        if (key)
            ret = pthread_setspecific(*key, *statement);
        if (ret)
            add_stmt_to_list(*statement);
    }
    return rc;
}

char *get_database_extented_error(sqlite3 *database, int i, const char *description)
{
    const char *err = sqlite3_errstr(sqlite3_extended_errcode(database));

    if (!err)
        return NULL;

    size_t len = strlen(err)+ strlen(description) + 32;
    char *full_err = mallocz(len);

    snprintfz(full_err, len - 1, "%s: %d: %s", description, i,  err);
    return full_err;
}

int init_database_batch(sqlite3 *database, const char *batch[], const char *description)
{
    int rc;
    char *err_msg = NULL;
    for (int i = 0; batch[i]; i++) {
        rc = sqlite3_exec_monitored(database, batch[i], 0, 0, &err_msg);
        if (rc != SQLITE_OK) {
            error_report("SQLite error during database initialization, rc = %d (%s)", rc, err_msg);
            error_report("SQLite failed statement %s", batch[i]);
            char *error_str = get_database_extented_error(database, i, description);
            if (error_str)
                analytics_set_data_str(&analytics_data.netdata_fail_reason, error_str);
            sqlite3_free(err_msg);
            freez(error_str);
            if (SQLITE_CORRUPT == rc || SQLITE_NOTADB == rc) {
                if (mark_database_to_recover(NULL, database, rc))
                    error_report("Database is corrupted will attempt to fix");
                return SQLITE_CORRUPT;
            }
            return 1;
        }
    }
    return 0;
}

// Return 0 OK
// Return 1 Failed
int db_execute(sqlite3 *db, const char *cmd)
{
    int rc;
    int cnt = 0;

    while (cnt < SQL_MAX_RETRY) {
        char *err_msg = NULL;
        rc = sqlite3_exec_monitored(db, cmd, 0, 0, &err_msg);
        if (likely(rc == SQLITE_OK))
            break;

        ++cnt;
        error_report("Failed to execute '%s', rc = %d (%s) -- attempt %d", cmd, rc, err_msg ? err_msg : "unknown", cnt);
        if (err_msg) {
            sqlite3_free(err_msg);
        }

        if (likely(rc == SQLITE_BUSY || rc == SQLITE_LOCKED)) {
            usleep(SQLITE_INSERT_DELAY * USEC_PER_MS);
            continue;
        }

        if (rc == SQLITE_CORRUPT)
            mark_database_to_recover(NULL, db, rc);
        break;
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

#define SQL_DROP_TABLE "DROP table %s"

void sql_drop_table(const char *table)
{
    if (!table)
        return;

    char wstr[255];
    snprintfz(wstr, sizeof(wstr) - 1, SQL_DROP_TABLE, table);

    int rc = sqlite3_exec_monitored(db_meta, wstr, 0, 0, NULL);
    if (rc != SQLITE_OK) {
        error_report("DES SQLite error during drop table operation for %s, rc = %d", table, rc);
    }
}

static int get_pragma_value(sqlite3 *database, const char *sql)
{
    sqlite3_stmt *res = NULL;
    int result = -1;
    if (PREPARE_STATEMENT(database, sql, &res)) {
        if (likely(sqlite3_step_monitored(res) == SQLITE_ROW))
            result = sqlite3_column_int(res, 0);
        SQLITE_FINALIZE(res);
    }
    return result;
}

int get_free_page_count(sqlite3 *database)
{
    return get_pragma_value(database, "PRAGMA freelist_count");
}

int get_database_page_count(sqlite3 *database)
{
    return get_pragma_value(database, "PRAGMA page_count");
}

uint64_t sqlite_get_db_space(sqlite3 *db)
{
    if (!db)
        return 0;

    uint64_t page_size = (uint64_t) get_pragma_value(db, "PRAGMA page_size");
    uint64_t page_count = (uint64_t) get_pragma_value(db, "PRAGMA page_count");

    return page_size * page_count;
}

/*
 * Close the sqlite database
 */

void sql_close_database(sqlite3 *database, const char *database_name)
{
    int rc;
    if (unlikely(!database))
        return;

    (void) db_execute(database, "PRAGMA optimize");

    netdata_log_info("%s: Closing sqlite database", database_name);

#ifdef NETDATA_DEV_MODE
    int t_count_used,t_count_hit,t_count_miss,t_count_full, dummy;
    (void) sqlite3_db_status(database, SQLITE_DBSTATUS_LOOKASIDE_USED, &dummy, &t_count_used, 0);
    (void) sqlite3_db_status(database, SQLITE_DBSTATUS_LOOKASIDE_HIT, &dummy,&t_count_hit, 0);
    (void) sqlite3_db_status(database, SQLITE_DBSTATUS_LOOKASIDE_MISS_SIZE, &dummy,&t_count_miss, 0);
    (void) sqlite3_db_status(database, SQLITE_DBSTATUS_LOOKASIDE_MISS_FULL, &dummy,&t_count_full, 0);

    netdata_log_info("%s: Database lookaside allocation statistics: Used slots %d, Hit %d, Misses due to small slot size %d, Misses due to slots full %d", database_name,
                     t_count_used,t_count_hit, t_count_miss, t_count_full);

    (void) sqlite3_db_release_memory(database);
#endif

    rc = sqlite3_close_v2(database);
    if (unlikely(rc != SQLITE_OK))
        error_report("%s: Error while closing the sqlite database: rc %d, error \"%s\"", database_name, rc, sqlite3_errstr(rc));
    database = NULL;
}

extern sqlite3 *db_context_meta;

void sqlite_close_databases(void)
{
    add_stmt_to_list(NULL);

    sql_close_database(db_context_meta, "CONTEXT");
    sql_close_database(db_meta, "METADATA");
}

uint64_t get_total_database_space(void)
{
    return 0;

/*
    if (!new_dbengine_defaults)
        return 0;

    uint64_t database_space = sqlite_get_meta_space() + sqlite_get_context_space();
#ifdef ENABLE_ML
    database_space +=  sqlite_get_ml_space();
#endif
    return database_space;
*/
}

#define SQLITE_HEAP_HARD_LIMIT (256 * 1024 * 1024)
#define SQLITE_HEAP_SOFT_LIMIT (32 * 1024 * 1024)

int sqlite_library_init(void)
{
    initialize_thread_key_pool();

    int rc = sqlite3_initialize();
    if (rc == SQLITE_OK) {

        (void )sqlite3_hard_heap_limit64(SQLITE_HEAP_HARD_LIMIT);
        int64_t hard_limit_bytes = sqlite3_hard_heap_limit64(-1);

        (void) sqlite3_soft_heap_limit64(SQLITE_HEAP_SOFT_LIMIT);
        int64_t soft_limit_bytes = sqlite3_soft_heap_limit64(-1);

        const char sqlite_hard_limit_mb[32];
        size_snprintf_bytes((char *)sqlite_hard_limit_mb, sizeof(sqlite_hard_limit_mb), hard_limit_bytes);

        const char sqlite_soft_limit_mb[32];
        size_snprintf_bytes((char *)sqlite_soft_limit_mb, sizeof(sqlite_soft_limit_mb), soft_limit_bytes);

        nd_log_daemon(
            NDLP_INFO, "SQLITE: heap memory hard limit %s, soft limit %s", sqlite_hard_limit_mb, sqlite_soft_limit_mb);
    }

    return (SQLITE_OK != rc);
}

SPINLOCK sqlite_spinlock = SPINLOCK_INITIALIZER;

int sqlite_release_memory(int bytes)
{
    return sqlite3_release_memory(bytes);
}

void sqlite_library_shutdown(void)
{
#ifdef NETDATA_INTERNAL_CHECKS
    int bytes;
    do {
        bytes = sqlite_release_memory(1024 * 1024);
        netdata_log_info("SQLITE: Released %d bytes of memory", bytes);
    } while (bytes);
#endif
    spinlock_lock(&sqlite_spinlock);
    (void) sqlite3_shutdown();
    spinlock_unlock(&sqlite_spinlock);
}

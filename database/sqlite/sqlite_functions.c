// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite3recover.h"
#include "sqlite_db_migration.h"

#define DB_METADATA_VERSION 16

const char *database_config[] = {
    "CREATE TABLE IF NOT EXISTS host(host_id BLOB PRIMARY KEY, hostname TEXT NOT NULL, "
    "registry_hostname TEXT NOT NULL default 'unknown', update_every INT NOT NULL default 1, "
    "os TEXT NOT NULL default 'unknown', timezone TEXT NOT NULL default 'unknown', tags TEXT NOT NULL default '',"
    "hops INT NOT NULL DEFAULT 0,"
    "memory_mode INT DEFAULT 0, abbrev_timezone TEXT DEFAULT '', utc_offset INT NOT NULL DEFAULT 0,"
    "program_name TEXT NOT NULL DEFAULT 'unknown', program_version TEXT NOT NULL DEFAULT 'unknown', "
    "entries INT NOT NULL DEFAULT 0,"
    "health_enabled INT NOT NULL DEFAULT 0, last_connected INT NOT NULL DEFAULT 0)",

    "CREATE TABLE IF NOT EXISTS chart(chart_id blob PRIMARY KEY, host_id blob, type text, id text, name text, "
    "family text, context text, title text, unit text, plugin text, module text, priority int, update_every int, "
    "chart_type int, memory_mode int, history_entries)",

    "CREATE TABLE IF NOT EXISTS dimension(dim_id blob PRIMARY KEY, chart_id blob, id text, name text, "
    "multiplier int, divisor int , algorithm int, options text)",

    "CREATE TABLE IF NOT EXISTS metadata_migration(filename text, file_size, date_created int)",

    "CREATE INDEX IF NOT EXISTS ind_d2 on dimension (chart_id)",

    "CREATE INDEX IF NOT EXISTS ind_c3 on chart (host_id)",

    "CREATE TABLE IF NOT EXISTS chart_label(chart_id blob, source_type int, label_key text, "
    "label_value text, date_created int, PRIMARY KEY (chart_id, label_key))",

    "CREATE TABLE IF NOT EXISTS node_instance (host_id blob PRIMARY KEY, claim_id, node_id, date_created)",

    "CREATE TABLE IF NOT EXISTS alert_hash(hash_id blob PRIMARY KEY, date_updated int, alarm text, template text, "
    "on_key text, class text, component text, type text, os text, hosts text, lookup text, "
    "every text, units text, calc text, families text, plugin text, module text, charts text, green text, "
    "red text, warn text, crit text, exec text, to_key text, info text, delay text, options text, "
    "repeat text, host_labels text, p_db_lookup_dimensions text, p_db_lookup_method text, p_db_lookup_options int, "
    "p_db_lookup_after int, p_db_lookup_before int, p_update_every int, source text, chart_labels text, summary text)",

    "CREATE TABLE IF NOT EXISTS host_info(host_id blob, system_key text NOT NULL, system_value text NOT NULL, "
    "date_created INT, PRIMARY KEY(host_id, system_key))",

    "CREATE TABLE IF NOT EXISTS host_label(host_id blob, source_type int, label_key text NOT NULL, "
    "label_value text NOT NULL, date_created INT, PRIMARY KEY (host_id, label_key))",

    "CREATE TRIGGER IF NOT EXISTS ins_host AFTER INSERT ON host BEGIN INSERT INTO node_instance (host_id, date_created)"
      " SELECT new.host_id, unixepoch() WHERE new.host_id NOT IN (SELECT host_id FROM node_instance); END",

    "CREATE TABLE IF NOT EXISTS health_log (health_log_id INTEGER PRIMARY KEY, host_id blob, alarm_id int, "
    "config_hash_id blob, name text, chart text, family text, recipient text, units text, exec text, "
    "chart_context text, last_transition_id blob, chart_name text, UNIQUE (host_id, alarm_id))",

    "CREATE INDEX IF NOT EXISTS health_log_ind_1 ON health_log (host_id)",

    "CREATE TABLE IF NOT EXISTS health_log_detail (health_log_id int, unique_id int, alarm_id int, alarm_event_id int, "
    "updated_by_id int, updates_id int, when_key int, duration int, non_clear_duration int, "
    "flags int, exec_run_timestamp int, delay_up_to_timestamp int, "
    "info text, exec_code int, new_status real, old_status real, delay int, "
    "new_value double, old_value double, last_repeat int, transition_id blob, global_id int, summary text)",

    "CREATE INDEX IF NOT EXISTS health_log_d_ind_2 ON health_log_detail (global_id)",
    "CREATE INDEX IF NOT EXISTS health_log_d_ind_3 ON health_log_detail (transition_id)",
    "CREATE INDEX IF NOT EXISTS health_log_d_ind_9 ON health_log_detail (unique_id DESC, health_log_id)",
    "CREATE INDEX IF NOT EXISTS health_log_d_ind_6 on health_log_detail (health_log_id, when_key)",
    "CREATE INDEX IF NOT EXISTS health_log_d_ind_7 on health_log_detail (alarm_id)",
    "CREATE INDEX IF NOT EXISTS health_log_d_ind_8 on health_log_detail (new_status, updated_by_id)",

    NULL
};

const char *database_cleanup[] = {
    "DELETE FROM host WHERE host_id NOT IN (SELECT host_id FROM chart)",
    "DELETE FROM node_instance WHERE host_id NOT IN (SELECT host_id FROM host)",
    "DELETE FROM host_info WHERE host_id NOT IN (SELECT host_id FROM host)",
    "DELETE FROM host_label WHERE host_id NOT IN (SELECT host_id FROM host)",
    "DROP TRIGGER IF EXISTS tr_dim_del",
    "DROP INDEX IF EXISTS ind_d1",
    "DROP INDEX IF EXISTS ind_c1",
    "DROP INDEX IF EXISTS ind_c2",
    "DROP INDEX IF EXISTS alert_hash_index",
    "DROP INDEX IF EXISTS health_log_d_ind_4",
    "DROP INDEX IF EXISTS health_log_d_ind_1",
    "DROP INDEX IF EXISTS health_log_d_ind_5",
    NULL
};

sqlite3 *db_meta = NULL;

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
                global_statistics_sqlite3_query_completed(false, rc == SQLITE_BUSY, rc == SQLITE_LOCKED);
                usleep(SQLITE_INSERT_DELAY * USEC_PER_MS);
                continue;
            default:
                break;
        }
        break;
    }
    return rc;
}

static bool mark_database_to_recover(sqlite3_stmt *res, sqlite3 *database)
{

    if (!res && !database)
        return false;

    if (!database)
        database = sqlite3_db_handle(res);

    if (db_meta == database) {
        char recover_file[FILENAME_MAX + 1];
        snprintfz(recover_file, FILENAME_MAX, "%s/.netdata-meta.db.recover", netdata_configured_cache_dir);
        int fd = open(recover_file, O_WRONLY | O_CREAT | O_TRUNC, 444);
        if (fd >= 0) {
            close(fd);
            return true;
        }
    }
    return false;
}

static void recover_database(const char *sqlite_database, const char *new_sqlite_database)
{
    sqlite3 *database;
    int rc = sqlite3_open(sqlite_database, &database);
    if (rc != SQLITE_OK)
        return;

    netdata_log_info("Recover %s", sqlite_database);
    netdata_log_info("     to %s", new_sqlite_database);

    // This will remove the -shm and -wal files when we close the database
    (void) db_execute(database, "select count(*) from sqlite_master limit 0");

    sqlite3_recover *recover = sqlite3_recover_init(database, "main", new_sqlite_database);
    if (recover) {

        rc = sqlite3_recover_run(recover);

        if (rc == SQLITE_OK)
            netdata_log_info("Recover complete");
        else
            netdata_log_info("Recover encountered an error but the database may be usable");

        rc = sqlite3_recover_finish(recover);

        (void) sqlite3_close(database);

        if (rc == SQLITE_OK) {
            rc = rename(new_sqlite_database, sqlite_database);
            if (rc == 0) {
                netdata_log_info("Renamed %s", new_sqlite_database);
                netdata_log_info("     to %s", sqlite_database);
            }
        }
    }
    else
        (void) sqlite3_close(database);
}

int execute_insert(sqlite3_stmt *res)
{
    int rc;
    rc =  sqlite3_step_monitored(res);
    if (rc == SQLITE_CORRUPT) {
        (void)mark_database_to_recover(res, NULL);
        error_report("SQLite error %d", rc);
    }
    return rc;
}

int configure_sqlite_database(sqlite3 *database, int target_version, const char *description)
{
    char buf[1024 + 1] = "";
    const char *list[2] = { buf, NULL };

    // https://www.sqlite.org/pragma.html#pragma_auto_vacuum
    // PRAGMA schema.auto_vacuum = 0 | NONE | 1 | FULL | 2 | INCREMENTAL;
    snprintfz(buf, sizeof(buf) - 1, "PRAGMA auto_vacuum=%s", config_get(CONFIG_SECTION_SQLITE, "auto vacuum", "INCREMENTAL"));
    if (init_database_batch(database, list, description))
        return 1;

    // https://www.sqlite.org/pragma.html#pragma_synchronous
    // PRAGMA schema.synchronous = 0 | OFF | 1 | NORMAL | 2 | FULL | 3 | EXTRA;
    snprintfz(buf, sizeof(buf) - 1, "PRAGMA synchronous=%s", config_get(CONFIG_SECTION_SQLITE, "synchronous", "NORMAL"));
    if (init_database_batch(database, list, description))
        return 1;

    // https://www.sqlite.org/pragma.html#pragma_journal_mode
    // PRAGMA schema.journal_mode = DELETE | TRUNCATE | PERSIST | MEMORY | WAL | OFF
    snprintfz(buf, sizeof(buf) - 1, "PRAGMA journal_mode=%s", config_get(CONFIG_SECTION_SQLITE, "journal mode", "WAL"));
    if (init_database_batch(database, list, description))
        return 1;

    // https://www.sqlite.org/pragma.html#pragma_temp_store
    // PRAGMA temp_store = 0 | DEFAULT | 1 | FILE | 2 | MEMORY;
    snprintfz(buf, sizeof(buf) - 1, "PRAGMA temp_store=%s", config_get(CONFIG_SECTION_SQLITE, "temp store", "MEMORY"));
    if (init_database_batch(database, list, description))
        return 1;

    // https://www.sqlite.org/pragma.html#pragma_journal_size_limit
    // PRAGMA schema.journal_size_limit = N ;
    snprintfz(buf, sizeof(buf) - 1, "PRAGMA journal_size_limit=%lld", config_get_number(CONFIG_SECTION_SQLITE, "journal size limit", 16777216));
    if (init_database_batch(database, list, description))
        return 1;

    // https://www.sqlite.org/pragma.html#pragma_cache_size
    // PRAGMA schema.cache_size = pages;
    // PRAGMA schema.cache_size = -kibibytes;
    snprintfz(buf, sizeof(buf) - 1, "PRAGMA cache_size=%lld", config_get_number(CONFIG_SECTION_SQLITE, "cache size", -2000));
    if (init_database_batch(database, list, description))
        return 1;

    snprintfz(buf, sizeof(buf) - 1, "PRAGMA user_version=%d", target_version);
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
#ifdef NETDATA_DEV_MODE
    netdata_log_info("Thread %d: Cleaning prepared statement on %p", gettid(), statement);
#endif
    if (unlikely(rc = sqlite3_finalize((sqlite3_stmt *) statement) != SQLITE_OK))
        error_report("Failed to finalize statement, rc = %d", rc);
}

void initialize_thread_key_pool(void)
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
        if (likely(key)) {
            ret = pthread_setspecific(*key, *statement);
#ifdef NETDATA_DEV_MODE
            netdata_log_info("Thread %d: Using key %u on statement %p", gettid(), keys_used, *statement);
#endif
        }
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
            if (SQLITE_CORRUPT == rc) {
                if (mark_database_to_recover(NULL, database))
                    error_report("Database is corrupted will attempt to fix");
                return SQLITE_CORRUPT;
            }
            return 1;
        }
    }
    return 0;
}

static void sqlite_uuid_parse(sqlite3_context *context, int argc, sqlite3_value **argv)
{
    uuid_t  uuid;

    if ( argc != 1 ){
        sqlite3_result_null(context);
        return ;
    }
    int rc = uuid_parse((const char *) sqlite3_value_text(argv[0]), uuid);
    if (rc == -1)  {
        sqlite3_result_null(context);
        return ;
    }

    sqlite3_result_blob(context, &uuid, sizeof(uuid_t), SQLITE_TRANSIENT);
}

void sqlite_now_usec(sqlite3_context *context, int argc, sqlite3_value **argv)
{
    if (argc != 1 ){
        sqlite3_result_null(context);
        return ;
    }

    if (sqlite3_value_int(argv[0]) != 0) {
        struct timespec req = {.tv_sec = 0, .tv_nsec = 1};
        nanosleep(&req, NULL);
    }

    sqlite3_result_int64(context, (sqlite_int64) now_realtime_usec());
}

void sqlite_uuid_random(sqlite3_context *context, int argc, sqlite3_value **argv)
{
    (void)argc;
    (void)argv;

    uuid_t uuid;
    uuid_generate_random(uuid);
    sqlite3_result_blob(context, &uuid, sizeof(uuid_t), SQLITE_TRANSIENT);
}

/*
 * Initialize the SQLite database
 * Return 0 on success
 */
int sql_init_database(db_check_action_type_t rebuild, int memory)
{
    char *err_msg = NULL;
    char sqlite_database[FILENAME_MAX + 1];
    int rc;

    if (likely(!memory)) {
        snprintfz(sqlite_database, sizeof(sqlite_database) - 1, "%s/.netdata-meta.db.recover", netdata_configured_cache_dir);
        rc = unlink(sqlite_database);
        snprintfz(sqlite_database, FILENAME_MAX, "%s/netdata-meta.db", netdata_configured_cache_dir);

        if (rc == 0 || (rebuild & DB_CHECK_RECOVER)) {
            char new_sqlite_database[FILENAME_MAX + 1];
            snprintfz(new_sqlite_database, sizeof(new_sqlite_database) - 1, "%s/netdata-meta-recover.db", netdata_configured_cache_dir);
            recover_database(sqlite_database, new_sqlite_database);
            if (rebuild & DB_CHECK_RECOVER)
                return 0;
        }
    }
    else
        strcpy(sqlite_database, ":memory:");

    rc = sqlite3_open(sqlite_database, &db_meta);
    if (rc != SQLITE_OK) {
        error_report("Failed to initialize database at %s, due to \"%s\"", sqlite_database, sqlite3_errstr(rc));
        char *error_str = get_database_extented_error(db_meta, 0, "meta_open");
        if (error_str)
            analytics_set_data_str(&analytics_data.netdata_fail_reason, error_str);
        freez(error_str);
        sqlite3_close(db_meta);
        db_meta = NULL;
        return 1;
    }

    if (rebuild & DB_CHECK_RECLAIM_SPACE) {
        netdata_log_info("Reclaiming space of %s", sqlite_database);
        rc = sqlite3_exec_monitored(db_meta, "VACUUM", 0, 0, &err_msg);
        if (rc != SQLITE_OK) {
            error_report("Failed to execute VACUUM rc = %d (%s)", rc, err_msg);
            sqlite3_free(err_msg);
        }
        else {
            (void) db_execute(db_meta, "select count(*) from sqlite_master limit 0");
            (void) sqlite3_close(db_meta);
        }
        return 1;
    }

    if (rebuild & DB_CHECK_ANALYZE) {
        netdata_log_info("Running ANALYZE on %s", sqlite_database);
        rc = sqlite3_exec_monitored(db_meta, "ANALYZE", 0, 0, &err_msg);
        if (rc != SQLITE_OK) {
            error_report("Failed to execute ANALYZE rc = %d (%s)", rc, err_msg);
            sqlite3_free(err_msg);
        }
        else {
            (void) db_execute(db_meta, "select count(*) from sqlite_master limit 0");
            (void) sqlite3_close(db_meta);
        }
        return 1;
    }

    netdata_log_info("SQLite database %s initialization", sqlite_database);

    rc = sqlite3_create_function(db_meta, "u2h", 1, SQLITE_ANY | SQLITE_DETERMINISTIC, 0, sqlite_uuid_parse, 0, 0);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to register internal u2h function");

    rc = sqlite3_create_function(db_meta, "now_usec", 1, SQLITE_ANY, 0, sqlite_now_usec, 0, 0);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to register internal now_usec function");

    rc = sqlite3_create_function(db_meta, "uuid_random", 0, SQLITE_ANY, 0, sqlite_uuid_random, 0, 0);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to register internal uuid_random function");

    int target_version = DB_METADATA_VERSION;

    if (likely(!memory))
        target_version = perform_database_migration(db_meta, DB_METADATA_VERSION);

   if (configure_sqlite_database(db_meta, target_version, "meta_config"))
        return 1;

    if (init_database_batch(db_meta, &database_config[0], "meta_init"))
        return 1;

    if (init_database_batch(db_meta, &database_cleanup[0], "meta_cleanup"))
        return 1;

    netdata_log_info("SQLite database initialization completed");

    initialize_thread_key_pool();

    return 0;
}

/*
 * Close the sqlite database
 */

void sql_close_database(void)
{
    int rc;
    if (unlikely(!db_meta))
        return;

    netdata_log_info("Closing SQLite database");

    add_stmt_to_list(NULL);

    (void) db_execute(db_meta, "PRAGMA analysis_limit=10000");
    (void) db_execute(db_meta, "PRAGMA optimize");

    rc = sqlite3_close_v2(db_meta);
    if (unlikely(rc != SQLITE_OK))
        error_report("Error %d while closing the SQLite database, %s", rc, sqlite3_errstr(rc));
}

int exec_statement_with_uuid(const char *sql, uuid_t *uuid)
{
    int rc, result = 1;
    sqlite3_stmt *res = NULL;

    rc = sqlite3_prepare_v2(db_meta, sql, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement %s, rc = %d", sql, rc);
        return 1;
    }

    rc = sqlite3_bind_blob(res, 1, uuid, sizeof(*uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind UUID parameter to %s, rc = %d", sql, rc);
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
int db_execute(sqlite3 *db, const char *cmd)
{
    int rc;
    int cnt = 0;

    while (cnt < SQL_MAX_RETRY) {
        char *err_msg;
        rc = sqlite3_exec_monitored(db, cmd, 0, 0, &err_msg);
        if (likely(rc == SQLITE_OK))
            break;

        ++cnt;
        error_report("Failed to execute '%s', rc = %d (%s) -- attempt %d", cmd, rc, err_msg, cnt);
        sqlite3_free(err_msg);

        if (likely(rc == SQLITE_BUSY || rc == SQLITE_LOCKED)) {
            usleep(SQLITE_INSERT_DELAY * USEC_PER_MS);
            continue;
        }

        if (rc == SQLITE_CORRUPT)
            mark_database_to_recover(NULL, db);
        break;
    }
    return (rc != SQLITE_OK);
}

static inline void set_host_node_id(RRDHOST *host, uuid_t *node_id)
{
    if (unlikely(!host))
        return;

    if (unlikely(!node_id)) {
        freez(host->node_id);
        __atomic_store_n(&host->node_id, NULL, __ATOMIC_RELAXED);
        return;
    }

    struct aclk_sync_cfg_t  *wc = host->aclk_config;

    if (unlikely(!host->node_id)) {
        uuid_t *t = mallocz(sizeof(*host->node_id));
        uuid_copy(*t, *node_id);
        __atomic_store_n(&host->node_id, t, __ATOMIC_RELAXED);
    }
    else {
        uuid_copy(*(host->node_id), *node_id);
    }

    if (unlikely(!wc))
        sql_create_aclk_table(host, &host->host_uuid, node_id);
    else
        uuid_unparse_lower(*node_id, wc->node_id);
}

#define SQL_UPDATE_NODE_ID  "UPDATE node_instance SET node_id = @node_id WHERE host_id = @host_id"

int update_node_id(uuid_t *host_id, uuid_t *node_id)
{
    sqlite3_stmt *res = NULL;
    RRDHOST *host = NULL;
    int rc = 2;

    char host_guid[GUID_LEN + 1];
    uuid_unparse_lower(*host_id, host_guid);
    rrd_wrlock();
    host = rrdhost_find_by_guid(host_guid);
    if (likely(host))
        set_host_node_id(host, node_id);
    rrd_unlock();

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return 1;
    }

    rc = sqlite3_prepare_v2(db_meta, SQL_UPDATE_NODE_ID, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to store node instance information");
        return 1;
    }

    rc = sqlite3_bind_blob(res, 1, node_id, sizeof(*node_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter to store node instance information");
        goto failed;
    }

    rc = sqlite3_bind_blob(res, 2, host_id, sizeof(*host_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter to store node instance information");
        goto failed;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to store node instance information, rc = %d", rc);
    rc = sqlite3_changes(db_meta);

failed:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when storing node instance information");

    return rc - 1;
}

#define SQL_SELECT_NODE_ID  "SELECT node_id FROM node_instance WHERE host_id = @host_id AND node_id IS NOT NULL"

int get_node_id(uuid_t *host_id, uuid_t *node_id)
{
    sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return 1;
    }

    rc = sqlite3_prepare_v2(db_meta, SQL_SELECT_NODE_ID, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to select node instance information for a host");
        return 1;
    }

    rc = sqlite3_bind_blob(res, 1, host_id, sizeof(*host_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter to select node instance information");
        goto failed;
    }

    rc = sqlite3_step_monitored(res);
    if (likely(rc == SQLITE_ROW && node_id))
        uuid_copy(*node_id, *((uuid_t *) sqlite3_column_blob(res, 0)));

failed:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when selecting node instance information");

    return (rc == SQLITE_ROW) ? 0 : -1;
}

#define SQL_INVALIDATE_NODE_INSTANCES                                                                                  \
    "UPDATE node_instance SET node_id = NULL WHERE EXISTS "                                                            \
    "(SELECT host_id FROM node_instance WHERE host_id = @host_id AND (@claim_id IS NULL OR claim_id <> @claim_id))"

void invalidate_node_instances(uuid_t *host_id, uuid_t *claim_id)
{
    sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return;
    }

    rc = sqlite3_prepare_v2(db_meta, SQL_INVALIDATE_NODE_INSTANCES, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to invalidate node instance ids");
        return;
    }

    rc = sqlite3_bind_blob(res, 1, host_id, sizeof(*host_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter to invalidate node instance information");
        goto failed;
    }

    if (claim_id)
        rc = sqlite3_bind_blob(res, 2, claim_id, sizeof(*claim_id), SQLITE_STATIC);
    else
        rc = sqlite3_bind_null(res, 2);

    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind claim_id parameter to invalidate node instance information");
        goto failed;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to invalidate node instance information, rc = %d", rc);

failed:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when invalidating node instance information");
}

#define SQL_GET_NODE_INSTANCE_LIST                                                                                     \
    "SELECT ni.node_id, ni.host_id, h.hostname "                                                                       \
    "FROM node_instance ni, host h WHERE ni.host_id = h.host_id AND h.hops >=0"

struct node_instance_list *get_node_list(void)
{
    struct node_instance_list *node_list = NULL;
    sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return NULL;
    }

    rc = sqlite3_prepare_v2(db_meta, SQL_GET_NODE_INSTANCE_LIST, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to get node instance information");
        return NULL;
    }

    int row = 0;
    char host_guid[37];
    while (sqlite3_step_monitored(res) == SQLITE_ROW)
        row++;

    if (sqlite3_reset(res) != SQLITE_OK) {
        error_report("Failed to reset the prepared statement while fetching node instance information");
        goto failed;
    }
    node_list = callocz(row + 1, sizeof(*node_list));
    int max_rows = row;
    row = 0;
    // TODO: Check to remove lock
    rrd_rdlock();
    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        if (sqlite3_column_bytes(res, 0) == sizeof(uuid_t))
            uuid_copy(node_list[row].node_id, *((uuid_t *)sqlite3_column_blob(res, 0)));
        if (sqlite3_column_bytes(res, 1) == sizeof(uuid_t)) {
            uuid_t *host_id = (uuid_t *)sqlite3_column_blob(res, 1);
            uuid_unparse_lower(*host_id, host_guid);
            RRDHOST *host = rrdhost_find_by_guid(host_guid);
            if (!host)
                continue;
            if (rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD)) {
                netdata_log_info("ACLK: 'host:%s' skipping get node list because context is initializing", rrdhost_hostname(host));
                continue;
            }
            uuid_copy(node_list[row].host_id, *host_id);
            node_list[row].queryable = 1;
            node_list[row].live = (host && (host == localhost || host->receiver
                                            || !(rrdhost_flag_check(host, RRDHOST_FLAG_ORPHAN)))) ? 1 : 0;
            node_list[row].hops = (host && host->system_info) ? host->system_info->hops :
                                  uuid_memcmp(host_id, &localhost->host_uuid) ? 1 : 0;
            node_list[row].hostname =
                sqlite3_column_bytes(res, 2) ? strdupz((char *)sqlite3_column_text(res, 2)) : NULL;
        }
        row++;
        if (row == max_rows)
            break;
    }
    rrd_unlock();

failed:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when fetching node instance information");

    return node_list;
}

#define SQL_GET_HOST_NODE_ID "SELECT node_id FROM node_instance WHERE host_id = @host_id"

void sql_load_node_id(RRDHOST *host)
{
    sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return;
    }

    rc = sqlite3_prepare_v2(db_meta, SQL_GET_HOST_NODE_ID, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to fetch node id");
        return;
    }

    rc = sqlite3_bind_blob(res, 1, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter to load node instance information");
        goto failed;
    }

    rc = sqlite3_step_monitored(res);
    if (likely(rc == SQLITE_ROW)) {
        if (likely(sqlite3_column_bytes(res, 0) == sizeof(uuid_t)))
            set_host_node_id(host, (uuid_t *)sqlite3_column_blob(res, 0));
        else
            set_host_node_id(host, NULL);
    }

failed:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when loading node instance information");
}

#define SELECT_HOST_INFO "SELECT system_key, system_value FROM host_info WHERE host_id = @host_id"

void sql_build_host_system_info(uuid_t *host_id, struct rrdhost_system_info *system_info)
{
    int rc;

    sqlite3_stmt *res = NULL;

    rc = sqlite3_prepare_v2(db_meta, SELECT_HOST_INFO, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to read host information");
        return;
    }

    rc = sqlite3_bind_blob(res, 1, host_id, sizeof(*host_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host parameter host information");
        goto skip;
    }

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        rrdhost_set_system_info_variable(system_info, (char *) sqlite3_column_text(res, 0),
                                         (char *) sqlite3_column_text(res, 1));
    }

skip:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when reading host information");
}

#define SELECT_HOST_LABELS "SELECT label_key, label_value, source_type FROM host_label WHERE host_id = @host_id " \
    "AND label_key IS NOT NULL AND label_value IS NOT NULL"

RRDLABELS *sql_load_host_labels(uuid_t *host_id)
{
    int rc;

    RRDLABELS *labels = NULL;
    sqlite3_stmt *res = NULL;

    rc = sqlite3_prepare_v2(db_meta, SELECT_HOST_LABELS, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to read host information");
        return NULL;
    }

    rc = sqlite3_bind_blob(res, 1, host_id, sizeof(*host_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host parameter host information");
        goto skip;
    }

    labels = rrdlabels_create();

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        rrdlabels_add(labels, (const char *)sqlite3_column_text(res, 0), (const char *)sqlite3_column_text(res, 1), sqlite3_column_int(res, 2));
    }

skip:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when reading host information");
    return labels;
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

int sql_metadata_cache_stats(int op)
{
    int count, dummy;

    if (unlikely(!db_meta))
        return 0;

    netdata_thread_disable_cancelability();
    sqlite3_db_status(db_meta, op, &count, &dummy, 0);
    netdata_thread_enable_cancelability();
    return count;
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

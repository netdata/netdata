// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_db_migration.h"

#define DB_METADATA_VERSION 4

const char *database_config[] = {
    "CREATE TABLE IF NOT EXISTS host(host_id BLOB PRIMARY KEY, hostname TEXT NOT NULL, "
    "registry_hostname TEXT NOT NULL default 'unknown', update_every INT NOT NULL default 1, "
    "os TEXT NOT NULL default 'unknown', timezone TEXT NOT NULL default 'unknown', tags TEXT NOT NULL default '',"
    "hops INT NOT NULL DEFAULT 0,"
    "memory_mode INT DEFAULT 0, abbrev_timezone TEXT DEFAULT '', utc_offset INT NOT NULL DEFAULT 0,"
    "program_name TEXT NOT NULL DEFAULT 'unknown', program_version TEXT NOT NULL DEFAULT 'unknown', "
    "entries INT NOT NULL DEFAULT 0,"
    "health_enabled INT NOT NULL DEFAULT 0);",

    "CREATE TABLE IF NOT EXISTS chart(chart_id blob PRIMARY KEY, host_id blob, type text, id text, name text, "
    "family text, context text, title text, unit text, plugin text, module text, priority int, update_every int, "
    "chart_type int, memory_mode int, history_entries);",
    "CREATE TABLE IF NOT EXISTS dimension(dim_id blob PRIMARY KEY, chart_id blob, id text, name text, "
    "multiplier int, divisor int , algorithm int, options text);",

    "DROP TABLE IF EXISTS chart_active;",
    "DROP TABLE IF EXISTS dimension_active;",

    "CREATE TABLE IF NOT EXISTS chart_active(chart_id blob PRIMARY KEY, date_created int);",
    "CREATE TABLE IF NOT EXISTS dimension_active(dim_id blob primary key, date_created int);",
    "CREATE TABLE IF NOT EXISTS metadata_migration(filename text, file_size, date_created int);",
    "CREATE INDEX IF NOT EXISTS ind_d1 on dimension (chart_id, id, name);",
    "CREATE INDEX IF NOT EXISTS ind_c1 on chart (host_id, id, type, name);",
    "CREATE TABLE IF NOT EXISTS chart_label(chart_id blob, source_type int, label_key text, "
    "label_value text, date_created int, PRIMARY KEY (chart_id, label_key));",
    "CREATE TABLE IF NOT EXISTS node_instance (host_id blob PRIMARY KEY, claim_id, node_id, date_created);",
    "CREATE TABLE IF NOT EXISTS alert_hash(hash_id blob PRIMARY KEY, date_updated int, alarm text, template text, "
    "on_key text, class text, component text, type text, os text, hosts text, lookup text, "
    "every text, units text, calc text, families text, plugin text, module text, charts text, green text, "
    "red text, warn text, crit text, exec text, to_key text, info text, delay text, options text, "
    "repeat text, host_labels text, p_db_lookup_dimensions text, p_db_lookup_method text, p_db_lookup_options int, "
    "p_db_lookup_after int, p_db_lookup_before int, p_update_every int);",

    "CREATE TABLE IF NOT EXISTS host_info(host_id blob, system_key text NOT NULL, system_value text NOT NULL, "
    "date_created INT, PRIMARY KEY(host_id, system_key));",

    "CREATE TABLE IF NOT EXISTS host_label(host_id blob, source_type int, label_key text NOT NULL, "
    "label_value text NOT NULL, date_created INT, PRIMARY KEY (host_id, label_key));",

    "CREATE TABLE IF NOT EXISTS chart_hash_map(chart_id blob , hash_id blob, UNIQUE (chart_id, hash_id));",

    "CREATE TABLE IF NOT EXISTS chart_hash(hash_id blob PRIMARY KEY,type text, id text, name text, "
    "family text, context text, title text, unit text, plugin text, "
    "module text, priority integer, chart_type, last_used);",

    "CREATE VIEW IF NOT EXISTS v_chart_hash as SELECT ch.*, chm.chart_id FROM chart_hash ch, chart_hash_map chm "
    "WHERE ch.hash_id = chm.hash_id;",

    "CREATE TRIGGER IF NOT EXISTS ins_host AFTER INSERT ON host BEGIN INSERT INTO node_instance (host_id, date_created)"
      " SELECT new.host_id, unixepoch() WHERE new.host_id NOT IN (SELECT host_id FROM node_instance); END;",

    "CREATE TRIGGER IF NOT EXISTS tr_v_chart_hash INSTEAD OF INSERT on v_chart_hash BEGIN "
    "INSERT INTO chart_hash (hash_id, type, id, name, family, context, title, unit, plugin, "
    "module, priority, chart_type, last_used) "
    "values (new.hash_id, new.type, new.id, new.name, new.family, new.context, new.title, new.unit, new.plugin, "
    "new.module, new.priority, new.chart_type, unixepoch()) "
    "ON CONFLICT (hash_id) DO UPDATE SET last_used = unixepoch(); "
    "INSERT INTO chart_hash_map (chart_id, hash_id) values (new.chart_id, new.hash_id) "
    "on conflict (chart_id, hash_id) do nothing; END; ",

    NULL
};

const char *database_cleanup[] = {
    "delete from chart where chart_id not in (select chart_id from dimension);",
    "delete from host where host_id not in (select host_id from chart);",
    "delete from chart_label where chart_id not in (select chart_id from chart);",
    "DELETE FROM chart_hash_map WHERE chart_id NOT IN (SELECT chart_id FROM chart);",
    "DELETE FROM chart_hash WHERE hash_id NOT IN (SELECT hash_id FROM chart_hash_map);",
    "DELETE FROM node_instance WHERE host_id NOT IN (SELECT host_id FROM host);",
    "DELETE FROM host_info WHERE host_id NOT IN (SELECT host_id FROM host);",
    "DELETE FROM host_label WHERE host_id NOT IN (SELECT host_id FROM host);",
    NULL
};

sqlite3 *db_meta = NULL;

#define MAX_PREPARED_STATEMENTS (32)
pthread_key_t key_pool[MAX_PREPARED_STATEMENTS];

static uv_mutex_t sqlite_transaction_lock;


SQLITE_API int sqlite3_exec_monitored(
    sqlite3 *db,                               /* An open database */
    const char *sql,                           /* SQL to be evaluated */
    int (*callback)(void*,int,char**,char**),  /* Callback function */
    void *data,                                /* 1st argument to callback */
    char **errmsg                              /* Error msg written here */
) {
    int rc = sqlite3_exec(db, sql, callback, data, errmsg);
    sqlite3_query_completed(rc == SQLITE_OK, rc == SQLITE_BUSY, rc == SQLITE_LOCKED);
    return rc;
}

SQLITE_API int sqlite3_step_monitored(sqlite3_stmt *stmt) {
    int rc = sqlite3_step(stmt);

    if(likely(rc == SQLITE_ROW))
        sqlite3_row_completed();
    else
        sqlite3_query_completed(rc == SQLITE_DONE, rc == SQLITE_BUSY, rc == SQLITE_LOCKED);

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

static void add_stmt_to_list(sqlite3_stmt *res)
{
    static int idx = 0;
    static sqlite3_stmt *statements[MAX_OPEN_STATEMENTS];

    if (unlikely(!res)) {
        if (idx)
            info("Finilizing %d statements", idx);
        else
            info("No statements pending to finalize");
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
#ifdef NETDATA_INTERNAL_CHECKS
    info("Thread %d: Cleaning prepared statement on %p", gettid(), statement);
#endif
    if (unlikely(rc = sqlite3_finalize((sqlite3_stmt *) statement) != SQLITE_OK))
        error_report("Failed to finalize statement, rc = %d", rc);
}

int prepare_statement(sqlite3 *database, char *query, sqlite3_stmt **statement)
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
#ifdef NETDATA_INTERNAL_CHECKS
            info("Thread %d: Using key %u on statement %p", gettid(), keys_used, *statement);
#endif
        }
        if (ret)
            add_stmt_to_list(*statement);
    }
    return rc;
}

/*
 * Store a chart or dimension UUID in  chart_active or dimension_active
 * The statement that will be prepared determines that
 */

static int store_active_uuid_object(sqlite3_stmt **res, char *statement, uuid_t *uuid)
{
    int rc;

    // Check if we should need to prepare the statement
    if (!*res) {
        rc = prepare_statement(db_meta, statement, res);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to store active object, rc = %d", rc);
            return rc;
        }
    }

    rc = sqlite3_bind_blob(*res, 1, uuid, sizeof(*uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to bind input parameter to store active object, rc = %d", rc);
    else
        rc = execute_insert(*res);
    return rc;
}

/*
 * Marks a chart with UUID as active
 * Input: UUID
 */
void store_active_chart(uuid_t *chart_uuid)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return;
    }

    if (unlikely(!chart_uuid))
        return;

    rc = store_active_uuid_object(&res, SQL_STORE_ACTIVE_CHART, chart_uuid);
    if (rc != SQLITE_DONE)
        error_report("Failed to store active chart, rc = %d", rc);

    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement in store active chart, rc = %d", rc);
    return;
}

/*
 * Marks a dimension with UUID as active
 * Input: UUID
 */
void store_active_dimension(uuid_t *dimension_uuid)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return;
    }

    if (unlikely(!dimension_uuid))
        return;

    rc = store_active_uuid_object(&res, SQL_STORE_ACTIVE_DIMENSION, dimension_uuid);
    if (rc != SQLITE_DONE)
        error_report("Failed to store active dimension, rc = %d", rc);

    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement in store active dimension, rc = %d", rc);
    return;
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


static int check_table_integrity(char *table)
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

    int rc = sqlite3_exec_monitored(db_meta, wstr, check_table_integrity_cb, (void *) &status, &err_msg);
    if (rc != SQLITE_OK) {
        error_report("SQLite error during database integrity check for %s, rc = %d (%s)",
                     table ? table : "the entire database", rc, err_msg);
        sqlite3_free(err_msg);
    }

    return status;
}

const char *rebuild_chart_commands[] = {
    "BEGIN TRANSACTION; ",
    "DROP INDEX IF EXISTS ind_c1;" ,
    "DROP TABLE IF EXISTS chart_backup; " ,
    "CREATE TABLE chart_backup AS SELECT * FROM chart; " ,
    "DROP TABLE chart;  ",
    "CREATE TABLE IF NOT EXISTS chart(chart_id blob PRIMARY KEY, host_id blob, type text, id text, "
       "name text, family text, context text, title text, unit text, plugin text, "
       "module text, priority int, update_every int, chart_type int, memory_mode int, history_entries); ",
    "INSERT INTO chart SELECT DISTINCT * FROM chart_backup; ",
    "DROP TABLE chart_backup;  " ,
    "CREATE INDEX IF NOT EXISTS ind_c1 on chart (host_id, id, type, name);",
    "COMMIT TRANSACTION;",
    NULL
};

static void rebuild_chart()
{
    int rc;
    char *err_msg = NULL;
    info("Rebuilding chart table");
    for (int i = 0; rebuild_chart_commands[i]; i++) {
        info("Executing %s", rebuild_chart_commands[i]);
        rc = sqlite3_exec_monitored(db_meta, rebuild_chart_commands[i], 0, 0, &err_msg);
        if (rc != SQLITE_OK) {
            error_report("SQLite error during database setup, rc = %d (%s)", rc, err_msg);
            error_report("SQLite failed statement %s", rebuild_chart_commands[i]);
            sqlite3_free(err_msg);
        }
    }
    return;
}

const char *rebuild_dimension_commands[] = {
    "BEGIN TRANSACTION; ",
    "DROP INDEX IF EXISTS ind_d1;" ,
    "DROP TABLE IF EXISTS dimension_backup; " ,
    "CREATE TABLE dimension_backup AS SELECT * FROM dimension; " ,
    "DROP TABLE dimension; " ,
    "CREATE TABLE IF NOT EXISTS dimension(dim_id blob PRIMARY KEY, chart_id blob, id text, name text, "
        "multiplier int, divisor int , algorithm int, options text);" ,
    "INSERT INTO dimension SELECT distinct * FROM dimension_backup; " ,
    "DROP TABLE dimension_backup;  " ,
    "CREATE INDEX IF NOT EXISTS ind_d1 on dimension (chart_id, id, name);",
    "COMMIT TRANSACTION;",
    NULL
};

void rebuild_dimension()
{
    int rc;
    char *err_msg = NULL;

    info("Rebuilding dimension table");
    for (int i = 0; rebuild_dimension_commands[i]; i++) {
        info("Executing %s", rebuild_dimension_commands[i]);
        rc = sqlite3_exec_monitored(db_meta, rebuild_dimension_commands[i], 0, 0, &err_msg);
        if (rc != SQLITE_OK) {
            error_report("SQLite error during database setup, rc = %d (%s)", rc, err_msg);
            error_report("SQLite failed statement %s", rebuild_dimension_commands[i]);
            sqlite3_free(err_msg);
        }
    }
    return;
}

static int attempt_database_fix()
{
    info("Closing database and attempting to fix it");
    int rc = sqlite3_close(db_meta);
    if (rc != SQLITE_OK)
        error_report("Failed to close database, rc = %d", rc);
    info("Attempting to fix database");
    db_meta = NULL;
    return sql_init_database(DB_CHECK_FIX_DB | DB_CHECK_CONT, 0);
}

int init_database_batch(sqlite3 *database, int rebuild, int init_type, const char *batch[])
{
    int rc;
    char *err_msg = NULL;
    for (int i = 0; batch[i]; i++) {
        debug(D_METADATALOG, "Executing %s", batch[i]);
        rc = sqlite3_exec_monitored(database, batch[i], 0, 0, &err_msg);
        if (rc != SQLITE_OK) {
            error_report("SQLite error during database %s, rc = %d (%s)", init_type ? "cleanup" : "setup", rc, err_msg);
            error_report("SQLite failed statement %s", batch[i]);
            sqlite3_free(err_msg);
            if (SQLITE_CORRUPT == rc) {
                if (!rebuild)
                    return attempt_database_fix();
                rc = check_table_integrity(NULL);
                if (rc)
                    error_report("Databse integrity errors reported");
            }
            return 1;
        }
    }
    return 0;
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

    if (likely(!memory))
        snprintfz(sqlite_database, FILENAME_MAX, "%s/netdata-meta.db", netdata_configured_cache_dir);
    else
        strcpy(sqlite_database, ":memory:");

    rc = sqlite3_open(sqlite_database, &db_meta);
    if (rc != SQLITE_OK) {
        error_report("Failed to initialize database at %s, due to \"%s\"", sqlite_database, sqlite3_errstr(rc));
        sqlite3_close(db_meta);
        db_meta = NULL;
        return 1;
    }

    if (rebuild & (DB_CHECK_INTEGRITY | DB_CHECK_FIX_DB)) {
        int errors_detected = 0;
        if (!(rebuild & DB_CHECK_CONT))
            info("Running database check on %s", sqlite_database);

        if (check_table_integrity("chart")) {
            errors_detected++;
            if (rebuild & DB_CHECK_FIX_DB)
                rebuild_chart();
            else
                error_report("Errors reported -- run with -W sqlite-fix");
        }

        if (check_table_integrity("dimension")) {
            errors_detected++;
            if (rebuild & DB_CHECK_FIX_DB)
                rebuild_dimension();
            else
                error_report("Errors reported -- run with -W sqlite-fix");
        }

        if (!errors_detected) {
            if (check_table_integrity(NULL))
                error_report("Errors reported");
        }
    }

    if (rebuild & DB_CHECK_RECLAIM_SPACE) {
        if (!(rebuild & DB_CHECK_CONT))
            info("Reclaiming space of %s", sqlite_database);
        rc = sqlite3_exec_monitored(db_meta, "VACUUM;", 0, 0, &err_msg);
        if (rc != SQLITE_OK) {
            error_report("Failed to execute VACUUM rc = %d (%s)", rc, err_msg);
            sqlite3_free(err_msg);
        }
    }

    if (rebuild && !(rebuild & DB_CHECK_CONT))
        return 1;

    info("SQLite database %s initialization", sqlite_database);

    char buf[1024 + 1] = "";
    const char *list[2] = { buf, NULL };

    int target_version = DB_METADATA_VERSION;

    if (likely(!memory))
        target_version = perform_database_migration(db_meta, DB_METADATA_VERSION);

    // https://www.sqlite.org/pragma.html#pragma_auto_vacuum
    // PRAGMA schema.auto_vacuum = 0 | NONE | 1 | FULL | 2 | INCREMENTAL;
    snprintfz(buf, 1024, "PRAGMA auto_vacuum=%s;", config_get(CONFIG_SECTION_SQLITE, "auto vacuum", "INCREMENTAL"));
    if(init_database_batch(db_meta, rebuild, 0, list)) return 1;

    // https://www.sqlite.org/pragma.html#pragma_synchronous
    // PRAGMA schema.synchronous = 0 | OFF | 1 | NORMAL | 2 | FULL | 3 | EXTRA;
    snprintfz(buf, 1024, "PRAGMA synchronous=%s;", config_get(CONFIG_SECTION_SQLITE, "synchronous", "NORMAL"));
    if(init_database_batch(db_meta, rebuild, 0, list)) return 1;

    // https://www.sqlite.org/pragma.html#pragma_journal_mode
    // PRAGMA schema.journal_mode = DELETE | TRUNCATE | PERSIST | MEMORY | WAL | OFF
    snprintfz(buf, 1024, "PRAGMA journal_mode=%s;", config_get(CONFIG_SECTION_SQLITE, "journal mode", "WAL"));
    if(init_database_batch(db_meta, rebuild, 0, list)) return 1;

    // https://www.sqlite.org/pragma.html#pragma_temp_store
    // PRAGMA temp_store = 0 | DEFAULT | 1 | FILE | 2 | MEMORY;
    snprintfz(buf, 1024, "PRAGMA temp_store=%s;", config_get(CONFIG_SECTION_SQLITE, "temp store", "MEMORY"));
    if(init_database_batch(db_meta, rebuild, 0, list)) return 1;

    // https://www.sqlite.org/pragma.html#pragma_journal_size_limit
    // PRAGMA schema.journal_size_limit = N ;
    snprintfz(buf, 1024, "PRAGMA journal_size_limit=%lld;", config_get_number(CONFIG_SECTION_SQLITE, "journal size limit", 16777216));
    if(init_database_batch(db_meta, rebuild, 0, list)) return 1;

    // https://www.sqlite.org/pragma.html#pragma_cache_size
    // PRAGMA schema.cache_size = pages;
    // PRAGMA schema.cache_size = -kibibytes;
    snprintfz(buf, 1024, "PRAGMA cache_size=%lld;", config_get_number(CONFIG_SECTION_SQLITE, "cache size", -2000));
    if(init_database_batch(db_meta, rebuild, 0, list)) return 1;

    snprintfz(buf, 1024, "PRAGMA user_version=%d;", target_version);
    if(init_database_batch(db_meta, rebuild, 0, list)) return 1;

    if (init_database_batch(db_meta, rebuild, 0, &database_config[0]))
        return 1;

    if (init_database_batch(db_meta, rebuild, 0, &database_cleanup[0]))
        return 1;

    fatal_assert(0 == uv_mutex_init(&sqlite_transaction_lock));
    info("SQLite database initialization completed");

    for (int i = 0; i < MAX_PREPARED_STATEMENTS; i++)
        (void)pthread_key_create(&key_pool[i], release_statement);

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

    info("Closing SQLite database");

    add_stmt_to_list(NULL);

    rc = sqlite3_close_v2(db_meta);
    if (unlikely(rc != SQLITE_OK))
        error_report("Error %d while closing the SQLite database, %s", rc, sqlite3_errstr(rc));
    return;
}

#define FIND_UUID_TYPE  "select 1 from host where host_id = @uuid union select 2 from chart where chart_id = @uuid union select 3 from dimension where dim_id = @uuid;"

int find_uuid_type(uuid_t *uuid)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc;
    int uuid_type = 3;

    if (unlikely(!res)) {
        rc = prepare_statement(db_meta, FIND_UUID_TYPE, &res);
        if (rc != SQLITE_OK) {
            error_report("Failed to bind prepare statement to find UUID type in the database");
            return 0;
        }
    }

    rc = sqlite3_bind_blob(res, 1, uuid, sizeof(*uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_step_monitored(res);
    if (likely(rc == SQLITE_ROW))
        uuid_type = sqlite3_column_int(res, 0);

    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement during find uuid type, rc = %d", rc);

    return uuid_type;

bind_fail:
    return 0;
}

int find_dimension_uuid(RRDSET *st, RRDDIM *rd, uuid_t *store_uuid)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc;
    int status = 1;

    if (unlikely(!db_meta) && default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE)
        return 1;

    if (unlikely(!res)) {
        rc = prepare_statement(db_meta, SQL_FIND_DIMENSION_UUID, &res);
        if (rc != SQLITE_OK) {
            error_report("Failed to bind prepare statement to lookup dimension UUID in the database");
            return 1;
        }
    }

    rc = sqlite3_bind_blob(res, 1, st->chart_uuid, sizeof(*st->chart_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res, 2, rrddim_id(rd), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res, 3, rrddim_name(rd), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_step_monitored(res);
    if (likely(rc == SQLITE_ROW)) {
        uuid_copy(*store_uuid, *((uuid_t *) sqlite3_column_blob(res, 0)));
        status = 0;
    }
    else {
        uuid_generate(*store_uuid);
        status = sql_store_dimension(store_uuid, st->chart_uuid, rrddim_id(rd), rrddim_name(rd), rd->multiplier, rd->divisor, rd->algorithm);
        if (unlikely(status))
            error_report("Failed to store dimension metadata in the database");
    }

    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement find dimension uuid, rc = %d", rc);
    return status;

bind_fail:
    error_report("Failed to bind input parameter to perform dimension UUID database lookup, rc = %d", rc);
    return 1;
}

#define DELETE_DIMENSION_UUID   "delete from dimension where dim_id = @uuid;"

void delete_dimension_uuid(uuid_t *dimension_uuid)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc;

#ifdef NETDATA_INTERNAL_CHECKS
    char uuid_str[GUID_LEN + 1];
    uuid_unparse_lower(*dimension_uuid, uuid_str);
    debug(D_METADATALOG,"Deleting dimension uuid %s", uuid_str);
#endif

    if (unlikely(!res)) {
        rc = prepare_statement(db_meta, DELETE_DIMENSION_UUID, &res);
        if (rc != SQLITE_OK) {
            error_report("Failed to prepare statement to delete a dimension uuid");
            return;
        }
    }

    rc = sqlite3_bind_blob(res, 1, dimension_uuid,  sizeof(*dimension_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_step_monitored(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to delete dimension uuid, rc = %d", rc);

bind_fail:
    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when deleting dimension UUID, rc = %d", rc);
    return;
}

/*
 * Do a database lookup to find the UUID of a chart
 *
 */
uuid_t *find_chart_uuid(RRDHOST *host, const char *type, const char *id, const char *name)
{
    static __thread sqlite3_stmt *res = NULL;
    uuid_t *uuid = NULL;
    int rc;

    if (unlikely(!db_meta) && default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE)
        return NULL;

    if (unlikely(!res)) {
        rc = prepare_statement(db_meta, SQL_FIND_CHART_UUID, &res);
        if (rc != SQLITE_OK) {
            error_report("Failed to prepare statement to lookup chart UUID in the database");
            return NULL;
        }
    }

    rc = sqlite3_bind_blob(res, 1, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
            goto bind_fail;

    rc = sqlite3_bind_text(res, 2, type, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res, 3, id, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res, 4, name ? name : id, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_step_monitored(res);
    if (likely(rc == SQLITE_ROW)) {
        uuid = mallocz(sizeof(uuid_t));
        uuid_copy(*uuid, sqlite3_column_blob(res, 0));
    }

    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when searching for a chart UUID, rc = %d", rc);

#ifdef NETDATA_INTERNAL_CHECKS
    char  uuid_str[GUID_LEN + 1];
    if (likely(uuid)) {
        uuid_unparse_lower(*uuid, uuid_str);
        debug(D_METADATALOG, "Found UUID %s for chart %s.%s", uuid_str, type, name ? name : id);
    }
    else
        debug(D_METADATALOG, "UUID not found for chart %s.%s", type, name ? name : id);
#endif
    return uuid;

bind_fail:
    error_report("Failed to bind input parameter to perform chart UUID database lookup, rc = %d", rc);
    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when searching for a chart UUID, rc = %d", rc);
    return NULL;
}

int update_chart_metadata(uuid_t *chart_uuid, RRDSET *st, const char *id, const char *name)
{
    int rc;

    if (unlikely(!db_meta) && default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE)
        return 0;

    rc = sql_store_chart(
        chart_uuid, &st->rrdhost->host_uuid, rrdset_type(st), id, name,
        rrdset_family(st), rrdset_context(st), rrdset_title(st), rrdset_units(st),
        rrdset_plugin_name(st), rrdset_module_name(st),
        st->priority, st->update_every, st->chart_type,
        st->rrd_memory_mode, st->entries);

    return rc;
}

uuid_t *create_chart_uuid(RRDSET *st, const char *id, const char *name)
{
    uuid_t *uuid = NULL;
    int rc;

    uuid = mallocz(sizeof(uuid_t));
    uuid_generate(*uuid);

#ifdef NETDATA_INTERNAL_CHECKS
    char uuid_str[GUID_LEN + 1];
    uuid_unparse_lower(*uuid, uuid_str);
    debug(D_METADATALOG,"Generating uuid [%s] for chart %s under host %s", uuid_str, rrdset_id(st), rrdhost_hostname(st->rrdhost));
#endif

    rc = update_chart_metadata(uuid, st, id, name);

    if (unlikely(rc))
        error_report("Failed to store chart metadata in the database");

    return uuid;
}

static int exec_statement_with_uuid(const char *sql, uuid_t *uuid)
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
        error_report("Failed to bind host parameter to %s, rc = %d", sql, rc);
        goto failed;
    }

    rc = execute_insert(res);
    if (likely(rc == SQLITE_DONE))
        result = SQLITE_OK;
    else
        error_report("Failed to execute %s, rc = %d", sql, rc);

failed:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement %s, rc = %d", sql, rc);
    return result;
}


// Migrate all hosts with hops zero to this host_uuid
void migrate_localhost(uuid_t *host_uuid)
{
    int rc;

    rc = exec_statement_with_uuid("UPDATE chart SET host_id = @host_id WHERE host_id in (SELECT host_id FROM host where host_id <> @host_id and hops = 0); ", host_uuid);
    if (!rc)
        rc = exec_statement_with_uuid("DELETE FROM host WHERE hops = 0 AND host_id <> @host_id; ", host_uuid);
    if (!rc)
        db_execute("DELETE FROM node_instance WHERE host_id NOT IN (SELECT host_id FROM host);");

}

int sql_store_host(
    uuid_t *host_uuid, const char *hostname, const char *registry_hostname, int update_every, const char *os,
    const char *tzone, const char *tags, int hops)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE)
            return 0;
        error_report("Database has not been initialized");
        return 1;
    }

    if (unlikely((!res))) {
        rc = prepare_statement(db_meta, SQL_STORE_HOST, &res);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to store host, rc = %d", rc);
            return 1;
        }
    }

    rc = sqlite3_bind_blob(res, 1, host_uuid, sizeof(*host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res, 2, hostname, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res, 3, registry_hostname, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, 4, update_every);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res, 5, os, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res, 6, tzone, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res, 7, tags, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, 8, hops);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    int store_rc = sqlite3_step_monitored(res);
    if (unlikely(store_rc != SQLITE_DONE))
        error_report("Failed to store host %s, rc = %d", hostname, rc);

    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement to store host %s, rc = %d", hostname, rc);

    return !(store_rc == SQLITE_DONE);
bind_fail:
    error_report("Failed to bind parameter to store host %s, rc = %d", hostname, rc);
    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement to store host %s, rc = %d", hostname, rc);
    return 1;
}

//
// Store host and host system info information in the database
#define SQL_STORE_HOST_INFO "INSERT OR REPLACE INTO host " \
        "(host_id, hostname, registry_hostname, update_every, os, timezone," \
        "tags, hops, memory_mode, abbrev_timezone, utc_offset, program_name, program_version," \
        "entries, health_enabled) " \
        "values (@host_id, @hostname, @registry_hostname, @update_every, @os, @timezone, @tags, @hops, @memory_mode, " \
        "@abbrev_timezone, @utc_offset, @program_name, @program_version, " \
        "@entries, @health_enabled);"

int sql_store_host_info(RRDHOST *host)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE)
            return 0;
        error_report("Database has not been initialized");
        return 1;
    }

    if (unlikely((!res))) {
        rc = prepare_statement(db_meta, SQL_STORE_HOST_INFO, &res);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to store host, rc = %d", rc);
            return 1;
        }
    }

    rc = sqlite3_bind_blob(res, 1, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = bind_text_null(res, 2, rrdhost_hostname(host), 0);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = bind_text_null(res, 3, rrdhost_registry_hostname(host), 1);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, 4, host->rrd_update_every);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = bind_text_null(res, 5, rrdhost_os(host), 1);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = bind_text_null(res, 6, rrdhost_timezone(host), 1);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = bind_text_null(res, 7, rrdhost_tags(host), 1);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, 8, host->system_info ? host->system_info->hops : 0);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, 9, host->rrd_memory_mode);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = bind_text_null(res, 10, rrdhost_abbrev_timezone(host), 1);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, 11, host->utc_offset);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = bind_text_null(res, 12, rrdhost_program_name(host), 1);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = bind_text_null(res, 13, rrdhost_program_version(host), 1);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int64(res, 14, host->rrd_history_entries);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, 15, host->health_enabled);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    int store_rc = sqlite3_step_monitored(res);
    if (unlikely(store_rc != SQLITE_DONE))
        error_report("Failed to store host %s, rc = %d", rrdhost_hostname(host), rc);

    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement to store host %s, rc = %d", rrdhost_hostname(host), rc);

    return !(store_rc == SQLITE_DONE);
bind_fail:
    error_report("Failed to bind parameter to store host %s, rc = %d", rrdhost_hostname(host), rc);
    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement to store host %s, rc = %d", rrdhost_hostname(host), rc);
    return 1;
}

/*
 * Store a chart in the database
 */

int sql_store_chart(
    uuid_t *chart_uuid, uuid_t *host_uuid, const char *type, const char *id, const char *name, const char *family,
    const char *context, const char *title, const char *units, const char *plugin, const char *module, long priority,
    int update_every, int chart_type, int memory_mode, long history_entries)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc, param = 0;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE)
            return 0;
        error_report("Database has not been initialized");
        return 1;
    }

    if (unlikely(!res)) {
        rc = prepare_statement(db_meta, SQL_STORE_CHART, &res);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to store chart, rc = %d", rc);
            return 1;
        }
    }

    param++;
    rc = sqlite3_bind_blob(res, 1, chart_uuid, sizeof(*chart_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_blob(res, 2, host_uuid, sizeof(*host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 3, type, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 4, id, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    if (name && *name)
        rc = sqlite3_bind_text(res, 5, name, -1, SQLITE_STATIC);
    else
        rc = sqlite3_bind_null(res, 5);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 6, family, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 7, context, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 8, title, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 9, units, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 10, plugin, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 11, module, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_int(res, 12, priority);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_int(res, 13, update_every);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_int(res, 14, chart_type);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_int(res, 15, memory_mode);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_int(res, 16, history_entries);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to store chart, rc = %d", rc);

    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement in chart store function, rc = %d", rc);

    return 0;

bind_fail:
    error_report("Failed to bind parameter %d to store chart, rc = %d", param, rc);
    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement in chart store function, rc = %d", rc);
    return 1;
}

/*
 * Store a dimension
 */
int sql_store_dimension(
    uuid_t *dim_uuid, uuid_t *chart_uuid, const char *id, const char *name, collected_number multiplier,
    collected_number divisor, int algorithm)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE)
            return 0;
        error_report("Database has not been initialized");
        return 1;
    }

    if (unlikely(!res)) {
        rc = prepare_statement(db_meta, SQL_STORE_DIMENSION, &res);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to store dimension, rc = %d", rc);
            return 1;
        }
    }

    rc = sqlite3_bind_blob(res, 1, dim_uuid, sizeof(*dim_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_blob(res, 2, chart_uuid, sizeof(*chart_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res, 3, id, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res, 4, name, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, 5, multiplier);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, 6, divisor);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, 7, algorithm);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to store dimension, rc = %d", rc);

    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement in store dimension, rc = %d", rc);
    return 0;

bind_fail:
    error_report("Failed to bind parameter to store dimension, rc = %d", rc);
    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement in store dimension, rc = %d", rc);
    return 1;
}

/*
 * Store set option for a dimension
 */
int sql_set_dimension_option(uuid_t *dim_uuid, char *option)
{
    sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE)
            return 0;
        error_report("Database has not been initialized");
        return 1;
    }

    rc = sqlite3_prepare_v2(db_meta, "UPDATE dimension SET options = @options WHERE dim_id = @dim_id", -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to update dimension options");
        return 0;
    };

    rc = sqlite3_bind_blob(res, 2, dim_uuid, sizeof(*dim_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    if (!option || !strcmp(option,"unhide"))
        rc = sqlite3_bind_null(res, 1);
    else
        rc = sqlite3_bind_text(res, 1, option, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to update dimension option, rc = %d", rc);

bind_fail:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement in update dimension options, rc = %d", rc);
    return 0;
}


//
// Support for archived charts
//
#define SELECT_DIMENSION "select d.id, d.name from dimension d where d.chart_id = @chart_uuid;"

void sql_rrdim2json(sqlite3_stmt *res_dim, uuid_t *chart_uuid, BUFFER *wb, size_t *dimensions_count)
{
    int rc;

    rc = sqlite3_bind_blob(res_dim, 1, chart_uuid, sizeof(*chart_uuid), SQLITE_STATIC);
    if (rc != SQLITE_OK)
        return;

    int dimensions = 0;
    buffer_sprintf(wb, "\t\t\t\"dimensions\": {\n");

    while (sqlite3_step_monitored(res_dim) == SQLITE_ROW) {
        if (dimensions)
            buffer_strcat(wb, ",\n\t\t\t\t\"");
        else
            buffer_strcat(wb, "\t\t\t\t\"");
        buffer_strcat_jsonescape(wb, (const char *) sqlite3_column_text(res_dim, 0));
        buffer_strcat(wb, "\": { \"name\": \"");
        buffer_strcat_jsonescape(wb, (const char *) sqlite3_column_text(res_dim, 1));
        buffer_strcat(wb, "\" }");
        dimensions++;
    }
    *dimensions_count += dimensions;
    buffer_sprintf(wb, "\n\t\t\t}");
}

#define SELECT_CHART "select chart_id, id, name, type, family, context, title, priority, plugin, " \
    "module, unit, chart_type, update_every from chart " \
    "where host_id = @host_uuid and chart_id not in (select chart_id from chart_active) order by chart_id asc;"

void sql_rrdset2json(RRDHOST *host, BUFFER *wb)
{
    //    time_t first_entry_t = 0; //= rrdset_first_entry_t(st);
    //   time_t last_entry_t = 0; //rrdset_last_entry_t(st);
    static char *custom_dashboard_info_js_filename = NULL;
    int rc;

    sqlite3_stmt *res_chart = NULL;
    sqlite3_stmt *res_dim = NULL;
    time_t now = now_realtime_sec();

    rc = sqlite3_prepare_v2(db_meta, SELECT_CHART, -1, &res_chart, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to fetch host archived charts");
        return;
    }

    rc = sqlite3_bind_blob(res_chart, 1, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host parameter to fetch archived charts");
        goto failed;
    }

    rc = sqlite3_prepare_v2(db_meta, SELECT_DIMENSION, -1, &res_dim, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to fetch chart archived dimensions");
        goto failed;
    };

    if(unlikely(!custom_dashboard_info_js_filename))
        custom_dashboard_info_js_filename = config_get(CONFIG_SECTION_WEB, "custom dashboard_info.js", "");

    buffer_sprintf(wb, "{\n"
                       "\t\"hostname\": \"%s\""
                       ",\n\t\"version\": \"%s\""
                       ",\n\t\"release_channel\": \"%s\""
                       ",\n\t\"os\": \"%s\""
                       ",\n\t\"timezone\": \"%s\""
                       ",\n\t\"update_every\": %d"
                       ",\n\t\"history\": %ld"
                       ",\n\t\"memory_mode\": \"%s\""
                       ",\n\t\"custom_info\": \"%s\""
                       ",\n\t\"charts\": {"
        , rrdhost_hostname(host)
        , rrdhost_program_version(host)
        , get_release_channel()
        , rrdhost_os(host)
        , rrdhost_timezone(host)
        , host->rrd_update_every
        , host->rrd_history_entries
        , rrd_memory_mode_name(host->rrd_memory_mode)
        , custom_dashboard_info_js_filename
    );

    size_t c = 0;
    size_t dimensions = 0;

    while (sqlite3_step_monitored(res_chart) == SQLITE_ROW) {
        char id[512];
        sprintf(id, "%s.%s", sqlite3_column_text(res_chart, 3), sqlite3_column_text(res_chart, 1));
        RRDSET *st = rrdset_find(host, id);
        if (st && !rrdset_flag_check(st, RRDSET_FLAG_ARCHIVED))
            continue;

        if (c)
            buffer_strcat(wb, ",\n\t\t\"");
        else
            buffer_strcat(wb, "\n\t\t\"");
        c++;

        buffer_strcat(wb, id);
        buffer_strcat(wb, "\": ");

        buffer_sprintf(
            wb,
            "\t\t{\n"
            "\t\t\t\"id\": \"%s\",\n"
            "\t\t\t\"name\": \"%s\",\n"
            "\t\t\t\"type\": \"%s\",\n"
            "\t\t\t\"family\": \"%s\",\n"
            "\t\t\t\"context\": \"%s\",\n"
            "\t\t\t\"title\": \"%s (%s)\",\n"
            "\t\t\t\"priority\": %ld,\n"
            "\t\t\t\"plugin\": \"%s\",\n"
            "\t\t\t\"module\": \"%s\",\n"
            "\t\t\t\"enabled\": %s,\n"
            "\t\t\t\"units\": \"%s\",\n"
            "\t\t\t\"data_url\": \"/api/v1/data?chart=%s\",\n"
            "\t\t\t\"chart_type\": \"%s\",\n",
            id //sqlite3_column_text(res_chart, 1)
            ,
            id // sqlite3_column_text(res_chart, 2)
            ,
            sqlite3_column_text(res_chart, 3), sqlite3_column_text(res_chart, 4), sqlite3_column_text(res_chart, 5),
            sqlite3_column_text(res_chart, 6), id //sqlite3_column_text(res_chart, 2)
            ,
            (long ) sqlite3_column_int(res_chart, 7),
            (const char *) sqlite3_column_text(res_chart, 8) ? (const char *) sqlite3_column_text(res_chart, 8) : (char *) "",
            (const char *) sqlite3_column_text(res_chart, 9) ? (const char *) sqlite3_column_text(res_chart, 9) : (char *) "", (char *) "false",
            (const char *) sqlite3_column_text(res_chart, 10), id //sqlite3_column_text(res_chart, 2)
            ,
            rrdset_type_name(sqlite3_column_int(res_chart, 11)));

        sql_rrdim2json(res_dim, (uuid_t *) sqlite3_column_blob(res_chart, 0), wb, &dimensions);

        rc = sqlite3_reset(res_dim);
        if (unlikely(rc != SQLITE_OK))
            error_report("Failed to reset the prepared statement when reading archived chart dimensions");
        buffer_strcat(wb, "\n\t\t}");
    }

    buffer_sprintf(wb
        , "\n\t}"
          ",\n\t\"charts_count\": %zu"
          ",\n\t\"dimensions_count\": %zu"
          ",\n\t\"alarms_count\": %zu"
          ",\n\t\"rrd_memory_bytes\": %zu"
          ",\n\t\"hosts_count\": %zu"
          ",\n\t\"hosts\": ["
        , c
        , dimensions
        , (size_t) 0
        , (size_t) 0
        , rrd_hosts_available
    );

    if(unlikely(rrd_hosts_available > 1)) {
        rrd_rdlock();

        size_t found = 0;
        RRDHOST *h;
        rrdhost_foreach_read(h) {
            if(!rrdhost_should_be_removed(h, host, now) && !rrdhost_flag_check(h, RRDHOST_FLAG_ARCHIVED)) {
                buffer_sprintf(wb
                    , "%s\n\t\t{"
                      "\n\t\t\t\"hostname\": \"%s\""
                      "\n\t\t}"
                    , (found > 0) ? "," : ""
                    , rrdhost_hostname(h)
                );

                found++;
            }
        }

        rrd_unlock();
    }
    else {
        buffer_sprintf(wb
            , "\n\t\t{"
              "\n\t\t\t\"hostname\": \"%s\""
              "\n\t\t}"
            , rrdhost_hostname(host)
        );
    }

    buffer_sprintf(wb, "\n\t]\n}\n");

    rc = sqlite3_finalize(res_dim);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when reading archived chart dimensions");

failed:
    rc = sqlite3_finalize(res_chart);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when reading archived charts");

    return;
}

void free_temporary_host(RRDHOST *host)
{
    if (host) {
        string_freez(host->hostname);
        string_freez(host->os);
        string_freez(host->tags);
        string_freez(host->timezone);
        string_freez(host->program_name);
        string_freez(host->program_version);
        string_freez(host->registry_hostname);
        freez(host->system_info);
        freez(host);
    }
}

#define SELECT_HOST "select host_id, registry_hostname, update_every, os, timezone, tags from host where hostname = @hostname order by rowid desc;"
#define SELECT_HOST_BY_UUID "select h.host_id, h.registry_hostname, h.update_every, h.os, h.timezone, h.tags from host h, node_instance ni " \
    "where (ni.host_id = @host_id or ni.node_id = @host_id) AND ni.host_id = h.host_id;"

RRDHOST *sql_create_host_by_uuid(char *hostname)
{
    int rc;
    RRDHOST *host = NULL;
    uuid_t host_uuid;

    sqlite3_stmt *res = NULL;

    rc = uuid_parse(hostname, host_uuid);
    if (!rc) {
        rc = sqlite3_prepare_v2(db_meta, SELECT_HOST_BY_UUID, -1, &res, 0);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to fetch host by uuid");
            return NULL;
        }
        rc = sqlite3_bind_blob(res, 1, &host_uuid, sizeof(host_uuid), SQLITE_STATIC);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to bind host_id parameter to fetch host information");
            goto failed;
        }
    }
    else {
        rc = sqlite3_prepare_v2(db_meta, SELECT_HOST, -1, &res, 0);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to fetch host by hostname");
            return NULL;
        }
        rc = sqlite3_bind_text(res, 1, hostname, -1, SQLITE_STATIC);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to bind hostname parameter to fetch host information");
            goto failed;
        }
    }

    rc = sqlite3_step_monitored(res);
    if (unlikely(rc != SQLITE_ROW)) {
        error_report("Failed to find hostname %s", hostname);
        goto failed;
    }

    char uuid_str[GUID_LEN + 1];
    uuid_unparse_lower(*((uuid_t *) sqlite3_column_blob(res, 0)), uuid_str);

    host = callocz(1, sizeof(RRDHOST));

    set_host_properties(host, sqlite3_column_int(res, 2), RRD_MEMORY_MODE_DBENGINE, hostname,
                        (char *) sqlite3_column_text(res, 1), (const char *) uuid_str,
                        (char *) sqlite3_column_text(res, 3), (char *) sqlite3_column_text(res, 5),
                        (char *) sqlite3_column_text(res, 4), NULL, 0, NULL, NULL);

    uuid_copy(host->host_uuid, *((uuid_t *) sqlite3_column_blob(res, 0)));

    host->system_info = callocz(1, sizeof(*host->system_info));;
    rrdhost_flag_set(host, RRDHOST_FLAG_ARCHIVED);

#ifdef ENABLE_DBENGINE
    for(int tier = 0; tier < storage_tiers ; tier++)
        host->storage_instance[tier] = (STORAGE_INSTANCE *)multidb_ctx[tier];
#endif

failed:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when reading host information");

    return host;
}

void db_execute(const char *cmd)
{
    int rc;
    int cnt = 0;
    while (cnt < SQL_MAX_RETRY) {
        char *err_msg;
        rc = sqlite3_exec_monitored(db_meta, cmd, 0, 0, &err_msg);
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
}

void db_lock(void)
{
    uv_mutex_lock(&sqlite_transaction_lock);
    return;
}

void db_unlock(void)
{
    uv_mutex_unlock(&sqlite_transaction_lock);
    return;
}


#define SELECT_MIGRATED_FILE    "select 1 from metadata_migration where filename = @path;"

int file_is_migrated(char *path)
{
    sqlite3_stmt *res = NULL;
    int rc;

    rc = sqlite3_prepare_v2(db_meta, SELECT_MIGRATED_FILE, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to fetch host");
        return 0;
    }

    rc = sqlite3_bind_text(res, 1, path, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind filename parameter to check migration");
        return 0;
    }

    rc = sqlite3_step_monitored(res);

    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when checking if metadata file is migrated");

    return (rc == SQLITE_ROW);
}

#define STORE_MIGRATED_FILE    "insert or replace into metadata_migration (filename, file_size, date_created) " \
                                "values (@file, @size, unixepoch());"

void add_migrated_file(char *path, uint64_t file_size)
{
    sqlite3_stmt *res = NULL;
    int rc;

    rc = sqlite3_prepare_v2(db_meta, STORE_MIGRATED_FILE, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to fetch host");
        return;
    }

    rc = sqlite3_bind_text(res, 1, path, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind filename parameter to store migration information");
        return;
    }

    rc = sqlite3_bind_int64(res, 2, file_size);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind size parameter to store migration information");
        return;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to store migrated file, rc = %d", rc);

    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when checking if metadata file is migrated");

    return;
}

static int sql_store_label(sqlite3_stmt *res, uuid_t *uuid, int source_type, const char *label, const char *value)
{
    int rc;

    rc = sqlite3_bind_blob(res, 1, uuid, sizeof(*uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind UUID parameter to store label information");
        goto skip_store;
    }

    rc = sqlite3_bind_int(res, 2, source_type);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind type parameter to store label information");
        goto skip_store;
    }

    rc = sqlite3_bind_text(res, 3, label, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind label parameter to store label information");
        goto skip_store;
    }

    rc = sqlite3_bind_text(res, 4, value, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind value parameter to store label information");
        goto skip_store;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to store label entry, rc = %d", rc);

skip_store:
    if (unlikely(sqlite3_reset(res) != SQLITE_OK))
        error_report("Failed to reset the prepared statement when storing label information");

    return rc != SQLITE_DONE;
}

#define SQL_INS_CHART_LABEL "insert or replace into chart_label " \
    "(chart_id, source_type, label_key, label_value, date_created) " \
    "values (@chart, @source, @label, @value, unixepoch());"

void sql_store_chart_label(uuid_t *chart_uuid, int source_type, char *label, char *value)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return;
    }

    if (unlikely(!res)) {
        rc = prepare_statement(db_meta, SQL_INS_CHART_LABEL, &res);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement store chart labels");
            return;
        }
    }

    sql_store_label(res, chart_uuid, source_type, label, value);

    return;
}

#define SQL_INS_HOST_LABEL "INSERT OR REPLACE INTO host_label " \
    "(host_id, source_type, label_key, label_value, date_created) " \
    "values (@chart, @source, @label, @value, unixepoch());"

static void sql_store_host_label(uuid_t *host_uuid, int source_type, const char *label, const char *value)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return;
    }

    if (unlikely(!res)) {
        rc = prepare_statement(db_meta, SQL_INS_HOST_LABEL, &res);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement store chart labels");
            return;
        }
    }

    (void) sql_store_label(res, host_uuid, source_type, label, value);
}

int find_dimension_first_last_t(char *machine_guid, char *chart_id, char *dim_id,
                                uuid_t *uuid, time_t *first_entry_t, time_t *last_entry_t, uuid_t *rrdeng_uuid, int tier)
{
#ifdef ENABLE_DBENGINE
    int rc;
    uuid_t  legacy_uuid;
    uuid_t  multihost_legacy_uuid;
    time_t dim_first_entry_t, dim_last_entry_t;

    rc = rrdeng_metric_latest_time_by_uuid(uuid, &dim_first_entry_t, &dim_last_entry_t, tier);
    if (unlikely(rc)) {
        rrdeng_generate_legacy_uuid(dim_id, chart_id, &legacy_uuid);
        rc = rrdeng_metric_latest_time_by_uuid(&legacy_uuid, &dim_first_entry_t, &dim_last_entry_t, tier);
        if (likely(rc)) {
            rrdeng_convert_legacy_uuid_to_multihost(machine_guid, &legacy_uuid, &multihost_legacy_uuid);
            rc = rrdeng_metric_latest_time_by_uuid(&multihost_legacy_uuid, &dim_first_entry_t, &dim_last_entry_t, tier);
            if (likely(!rc))
                uuid_copy(*rrdeng_uuid, multihost_legacy_uuid);
        }
        else
            uuid_copy(*rrdeng_uuid, legacy_uuid);
    }
    else
        uuid_copy(*rrdeng_uuid, *uuid);

    if (likely(!rc)) {
        *first_entry_t = MIN(*first_entry_t, dim_first_entry_t);
        *last_entry_t = MAX(*last_entry_t, dim_last_entry_t);
    }
    return rc;
#else
    UNUSED(machine_guid);
    UNUSED(chart_id);
    UNUSED(dim_id);
    UNUSED(uuid);
    UNUSED(first_entry_t);
    UNUSED(last_entry_t);
    UNUSED(rrdeng_uuid);
    return 1;
#endif
}
#include "../storage_engine.h"
#ifdef ENABLE_DBENGINE
static RRDDIM *create_rrdim_entry(ONEWAYALLOC *owa, RRDSET *st, char *id, char *name, uuid_t *metric_uuid)
{
    STORAGE_ENGINE *eng = storage_engine_get(RRD_MEMORY_MODE_DBENGINE);

    if (unlikely(!eng))
        return NULL;

    RRDDIM *rd = onewayalloc_callocz(owa, 1, sizeof(*rd));
    rd->rrdset = st;
    rd->update_every = st->update_every;
    rd->last_stored_value = NAN;
    rrddim_flag_set(rd, RRDDIM_FLAG_NONE);

    uuid_copy(rd->metric_uuid, *metric_uuid);
    rd->id = string_strdupz(id);
    rd->name = string_strdupz(name);

    for(int tier = 0; tier < storage_tiers ;tier++) {
        rd->tiers[tier] = onewayalloc_callocz(owa, 1, sizeof(*rd->tiers[tier]));
        rd->rrd_memory_mode = RRD_MEMORY_MODE_DBENGINE;
        rd->tiers[tier]->tier_grouping = get_tier_grouping(tier);
        rd->tiers[tier]->mode = RRD_MEMORY_MODE_DBENGINE;
        rd->tiers[tier]->query_ops.init = rrdeng_load_metric_init;
        rd->tiers[tier]->query_ops.next_metric = rrdeng_load_metric_next;
        rd->tiers[tier]->query_ops.is_finished = rrdeng_load_metric_is_finished;
        rd->tiers[tier]->query_ops.finalize = rrdeng_load_metric_finalize;
        rd->tiers[tier]->query_ops.latest_time = rrdeng_metric_latest_time;
        rd->tiers[tier]->query_ops.oldest_time = rrdeng_metric_oldest_time;
        rd->tiers[tier]->db_metric_handle = eng->api.init(rd, st->rrdhost->storage_instance[tier]);
    }

    return rd;
}
#endif

#define SELECT_CHART_CONTEXT  "select d.dim_id, d.id, d.name, c.id, c.type, c.name, c.update_every, c.chart_id, " \
    "c.context, CASE WHEN d.options = 'hidden' THEN 1 else 0 END from chart c, " \
    "dimension d, host h " \
    "where d.chart_id = c.chart_id and c.host_id = h.host_id and c.host_id = @host_id and c.context = @context " \
    "order by c.chart_id asc, c.type||c.id desc;"

#define SELECT_CHART_SINGLE  "select d.dim_id, d.id, d.name, c.id, c.type, c.name, c.update_every, c.chart_id, " \
    "c.context, CASE WHEN d.options = 'hidden' THEN 1 else 0 END from chart c, " \
    "dimension d, host h " \
    "where d.chart_id = c.chart_id and c.host_id = h.host_id and c.host_id = @host_id and c.type||'.'||c.id = @chart " \
    "order by c.chart_id asc, c.type||'.'||c.id desc;"

void sql_build_context_param_list(ONEWAYALLOC  *owa, struct context_param **param_list, RRDHOST *host, char *context, char *chart)
{
#ifdef ENABLE_DBENGINE
    int rc;

    if (unlikely(!param_list) || host->rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE)
        return;

    if (unlikely(!(*param_list))) {
        *param_list = onewayalloc_mallocz(owa, sizeof(struct context_param));
        (*param_list)->first_entry_t = LONG_MAX;
        (*param_list)->last_entry_t = 0;
        (*param_list)->rd = NULL;
        (*param_list)->flags = CONTEXT_FLAGS_ARCHIVE;
        if (chart)
            (*param_list)->flags |= CONTEXT_FLAGS_CHART;
        else
            (*param_list)->flags |= CONTEXT_FLAGS_CONTEXT;
    }

    sqlite3_stmt *res = NULL;

    if (context)
        rc = sqlite3_prepare_v2(db_meta, SELECT_CHART_CONTEXT, -1, &res, 0);
    else
        rc = sqlite3_prepare_v2(db_meta, SELECT_CHART_SINGLE, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to fetch host archived charts");
        return;
    }

    rc = sqlite3_bind_blob(res, 1, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host parameter to fetch archived charts");
        goto failed;
    }

    if (context)
        rc = sqlite3_bind_text(res, 2, context, -1, SQLITE_STATIC);
    else
        rc = sqlite3_bind_text(res, 2, chart, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host parameter to fetch archived charts");
        goto failed;
    }

    RRDSET *st = NULL;
    char machine_guid[GUID_LEN + 1];
    uuid_unparse_lower(host->host_uuid, machine_guid);
    uuid_t rrdeng_uuid;
    uuid_t chart_id;

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        char id[512];
        sprintf(id, "%s.%s", sqlite3_column_text(res, 3), sqlite3_column_text(res, 1));

        if (!st || uuid_compare(*(uuid_t *)sqlite3_column_blob(res, 7), chart_id)) {
            if (unlikely(st && !st->counter)) {
                string_freez(st->context);
                string_freez(st->name);
                string_freez(st->id);
                onewayalloc_freez(owa, st);
            }
            st = onewayalloc_callocz(owa, 1, sizeof(*st));
            char n[RRD_ID_LENGTH_MAX + 1];

            snprintfz(
                n, RRD_ID_LENGTH_MAX, "%s.%s", (char *)sqlite3_column_text(res, 4),
                (char *)sqlite3_column_text(res, 3));
            st->name = string_strdupz(n);
            st->update_every = sqlite3_column_int(res, 6);
            st->counter = 0;
            if (chart) {
                st->context = string_strdupz((char *)sqlite3_column_text(res, 8));
                st->id = string_strdupz(chart);
            }

            // TODO: @stelfrag, what will be the st->id if chart == NULL ?

            uuid_copy(chart_id, *(uuid_t *)sqlite3_column_blob(res, 7));
            st->last_entry_t = 0;
            st->rrdhost = host;
        }

        if (unlikely(find_dimension_first_last_t(machine_guid, (char *)rrdset_name(st), (char *)sqlite3_column_text(res, 1),
                (uuid_t *)sqlite3_column_blob(res, 0), &(*param_list)->first_entry_t, &(*param_list)->last_entry_t,
                &rrdeng_uuid, 0)))
            continue;

        st->counter++;
        st->last_entry_t = MAX(st->last_entry_t, (*param_list)->last_entry_t);

        RRDDIM *rd = create_rrdim_entry(owa, st, (char *)sqlite3_column_text(res, 1), (char *)sqlite3_column_text(res, 2), &rrdeng_uuid);
        if (unlikely(!rd))
            continue;
        if (sqlite3_column_int(res, 9) == 1)
            rrddim_flag_set(rd, RRDDIM_FLAG_HIDDEN);
        rd->next = (*param_list)->rd;
        (*param_list)->rd = rd;
    }
    if (st) {
        if (!st->counter) {
            string_freez(st->context);
            string_freez(st->name);
            string_freez(st->id);
            onewayalloc_freez(owa,st);
        }
        else
            if (!st->context && context)
                st->context = string_strdupz(context);
    }

failed:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when reading archived charts");
#else
    UNUSED(param_list);
    UNUSED(host);
    UNUSED(context);
    UNUSED(chart);
#endif
    return;
}


/*
 * Store a chart hash in the database
 */

#define SQL_STORE_CHART_HASH "insert into v_chart_hash (hash_id, type, id, " \
    "name, family, context, title, unit, plugin, module, priority, chart_type, last_used, chart_id) " \
    "values (?1,?2,?3,?4,?5,?6,?7,?8,?9,?10,?11, ?12, unixepoch(), ?13);"

int sql_store_chart_hash(
    uuid_t *hash_id, uuid_t *chart_id, const char *type, const char *id, const char *name, const char *family,
    const char *context, const char *title, const char *units, const char *plugin, const char *module, long priority,
    RRDSET_TYPE chart_type)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc, param = 0;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE)
            return 0;
        error_report("Database has not been initialized");
        return 1;
    }

    if (unlikely(!res)) {
        rc = prepare_statement(db_meta, SQL_STORE_CHART_HASH, &res);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to store chart, rc = %d", rc);
            return 1;
        }
    }

    param++;
    rc = sqlite3_bind_blob(res, 1, hash_id, sizeof(*hash_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 2, type, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 3, id, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    if (name && *name)
        rc = sqlite3_bind_text(res, 4, name, -1, SQLITE_STATIC);
    else
        rc = sqlite3_bind_null(res, 4);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 5, family, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 6, context, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 7, title, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 8, units, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 9, plugin, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 10, module, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_int(res, 11, (int) priority);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_int(res, 12, chart_type);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_blob(res, 13, chart_id, sizeof(*chart_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to store chart hash_id, rc = %d", rc);

    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement in chart hash_id store function, rc = %d", rc);

    return 0;

    bind_fail:
    error_report("Failed to bind parameter %d to store chart hash_id, rc = %d", param, rc);
    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement in chart hash_id store function, rc = %d", rc);
    return 1;
}

/*
  chart hashes are used for cloud communication.
  if cloud is disabled or openssl is not available (which will prevent cloud connectivity)
  skip hash calculations
*/
void compute_chart_hash(RRDSET *st)
{
#if !defined DISABLE_CLOUD && defined ENABLE_HTTPS
    EVP_MD_CTX *evpctx;
    unsigned char hash_value[EVP_MAX_MD_SIZE];
    unsigned int hash_len;
    char  priority_str[32];

    if (rrdhost_flag_check(st->rrdhost, RRDHOST_FLAG_ACLK_STREAM_CONTEXTS)) {
        internal_error(true, "Skipping compute_chart_hash for host %s because context streaming is enabled", rrdhost_hostname(st->rrdhost));
        return;
    }

    sprintf(priority_str, "%ld", st->priority);

    evpctx = EVP_MD_CTX_create();
    EVP_DigestInit_ex(evpctx, EVP_sha256(), NULL);
    //EVP_DigestUpdate(evpctx, st->type, strlen(st->type));
    EVP_DigestUpdate(evpctx, rrdset_id(st), string_length(st->id));
    EVP_DigestUpdate(evpctx, rrdset_name(st), string_length(st->name));
    EVP_DigestUpdate(evpctx, rrdset_family(st), string_length(st->family));
    EVP_DigestUpdate(evpctx, rrdset_context(st), string_length(st->context));
    EVP_DigestUpdate(evpctx, rrdset_title(st), string_length(st->title));
    EVP_DigestUpdate(evpctx, rrdset_units(st), string_length(st->units));
    EVP_DigestUpdate(evpctx, rrdset_plugin_name(st), string_length(st->plugin_name));
    EVP_DigestUpdate(evpctx, rrdset_module_name(st), string_length(st->module_name));
//    EVP_DigestUpdate(evpctx, priority_str, strlen(priority_str));
    EVP_DigestUpdate(evpctx, &st->priority, sizeof(st->priority));
    EVP_DigestUpdate(evpctx, &st->chart_type, sizeof(st->chart_type));
    EVP_DigestFinal_ex(evpctx, hash_value, &hash_len);
    EVP_MD_CTX_destroy(evpctx);
    fatal_assert(hash_len > sizeof(uuid_t));

    char uuid_str[GUID_LEN + 1];
    uuid_unparse_lower(*((uuid_t *) &hash_value), uuid_str);
    //info("Calculating HASH %s for chart %s", uuid_str, st->name);
    uuid_copy(st->state->hash_id, *((uuid_t *) &hash_value));

    (void)sql_store_chart_hash(
        (uuid_t *)&hash_value,
        st->chart_uuid,
        rrdset_type(st),
        rrdset_id(st),
        rrdset_name(st),
        rrdset_family(st),
        rrdset_context(st),
        rrdset_title(st),
        rrdset_units(st),
        rrdset_plugin_name(st),
        rrdset_module_name(st),
        st->priority,
        st->chart_type);
#else
    UNUSED(st);
#endif
    return;
}

#define SQL_STORE_CLAIM_ID  "insert into node_instance " \
    "(host_id, claim_id, date_created) values (@host_id, @claim_id, unixepoch()) " \
    "on conflict(host_id) do update set claim_id = excluded.claim_id;"

void store_claim_id(uuid_t *host_id, uuid_t *claim_id)
{
    sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return;
    }

    rc = sqlite3_prepare_v2(db_meta, SQL_STORE_CLAIM_ID, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement store chart labels");
        return;
    }

    rc = sqlite3_bind_blob(res, 1, host_id, sizeof(*host_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter to store node instance information");
        goto failed;
    }

    if (claim_id)
        rc = sqlite3_bind_blob(res, 2, claim_id, sizeof(*claim_id), SQLITE_STATIC);
    else
        rc = sqlite3_bind_null(res, 2);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind claim_id parameter to store node instance information");
        goto failed;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to store node instance information, rc = %d", rc);

failed:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when storing node instance information");

    return;
}

static inline void set_host_node_id(RRDHOST *host, uuid_t *node_id)
{
    if (unlikely(!host))
        return;

    if (unlikely(!node_id)) {
        freez(host->node_id);
        host->node_id = NULL;
        return;
    }

    struct aclk_database_worker_config *wc = host->dbsync_worker;

    if (unlikely(!host->node_id))
        host->node_id = mallocz(sizeof(*host->node_id));
    uuid_copy(*(host->node_id), *node_id);

    if (unlikely(!wc))
        sql_create_aclk_table(host, &host->host_uuid, node_id);
    else
        uuid_unparse_lower(*node_id, wc->node_id);
    return;
}

#define SQL_UPDATE_NODE_ID  "update node_instance set node_id = @node_id where host_id = @host_id;"

int update_node_id(uuid_t *host_id, uuid_t *node_id)
{
    sqlite3_stmt *res = NULL;
    RRDHOST *host = NULL;
    int rc = 2;

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

    char host_guid[GUID_LEN + 1];
    uuid_unparse_lower(*host_id, host_guid);
    rrd_wrlock();
    host = rrdhost_find_by_guid(host_guid);
    if (likely(host))
            set_host_node_id(host, node_id);
    rrd_unlock();

failed:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when storing node instance information");

    return rc - 1;
}

#define SQL_SELECT_HOSTNAME_BY_NODE_ID  "SELECT h.hostname FROM node_instance ni, " \
"host h WHERE ni.host_id = h.host_id AND ni.node_id = @node_id;"

char *get_hostname_by_node_id(char *node)
{
    sqlite3_stmt *res = NULL;
    char  *hostname = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return NULL;
    }

    uuid_t node_id;
    if (uuid_parse(node, node_id))
        return NULL;

    rc = sqlite3_prepare_v2(db_meta, SQL_SELECT_HOSTNAME_BY_NODE_ID, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to fetch hostname by node id");
        return NULL;
    }

    rc = sqlite3_bind_blob(res, 1, &node_id, sizeof(node_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter to select node instance information");
        goto failed;
    }

    rc = sqlite3_step_monitored(res);
    if (likely(rc == SQLITE_ROW))
        hostname = strdupz((char *)sqlite3_column_text(res, 0));

failed:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when search for hostname by node id");

    return hostname;
}

#define SQL_SELECT_HOST_BY_NODE_ID  "select host_id from node_instance where node_id = @node_id;"

int get_host_id(uuid_t *node_id, uuid_t *host_id)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return 1;
    }

    if (unlikely(!res)) {
        rc = prepare_statement(db_meta, SQL_SELECT_HOST_BY_NODE_ID, &res);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to select node instance information for a node");
            return 1;
        }
    }

    rc = sqlite3_bind_blob(res, 1, node_id, sizeof(*node_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter to select node instance information");
        goto failed;
    }

    rc = sqlite3_step_monitored(res);
    if (likely(rc == SQLITE_ROW && host_id))
        uuid_copy(*host_id, *((uuid_t *) sqlite3_column_blob(res, 0)));

failed:
    if (unlikely(sqlite3_reset(res) != SQLITE_OK))
        error_report("Failed to reset the prepared statement when selecting node instance information");

    return (rc == SQLITE_ROW) ? 0 : -1;
}

#define SQL_SELECT_NODE_ID  "select node_id from node_instance where host_id = @host_id and node_id not null;"

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

#define SQL_INVALIDATE_NODE_INSTANCES "update node_instance set node_id = NULL where exists " \
    "(select host_id from node_instance where host_id = @host_id and (@claim_id is null or claim_id <> @claim_id));"

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

#define SQL_GET_NODE_INSTANCE_LIST "select ni.node_id, ni.host_id, h.hostname " \
    "from node_instance ni, host h where ni.host_id = h.host_id;"

struct  node_instance_list *get_node_list(void)
{
    struct  node_instance_list *node_list = NULL;
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
    };

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
    rrd_rdlock();
    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        if (sqlite3_column_bytes(res, 0) == sizeof(uuid_t))
            uuid_copy(node_list[row].node_id, *((uuid_t *)sqlite3_column_blob(res, 0)));
        if (sqlite3_column_bytes(res, 1) == sizeof(uuid_t)) {
            uuid_t *host_id = (uuid_t *)sqlite3_column_blob(res, 1);
            uuid_copy(node_list[row].host_id, *host_id);
            node_list[row].queryable = 1;
            uuid_unparse_lower(*host_id, host_guid);
            RRDHOST *host = rrdhost_find_by_guid(host_guid);
            node_list[row].live = host && (host == localhost || host->receiver) ? 1 : 0;
            node_list[row].hops = (host && host->system_info) ? host->system_info->hops :
                                  uuid_compare(*host_id, localhost->host_uuid) ? 1 : 0;
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
};

#define SQL_GET_HOST_NODE_ID "select node_id from node_instance where host_id = @host_id;"

void sql_load_node_id(RRDHOST *host)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return;
    }

    if (unlikely(!res)) {
        rc = prepare_statement(db_meta, SQL_GET_HOST_NODE_ID, &res);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to fetch node id");
            return;
        };
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
    if (unlikely(sqlite3_reset(res) != SQLITE_OK))
        error_report("Failed to reset the prepared statement when loading node instance information");

    return;
};


#define SELECT_HOST_INFO "SELECT system_key, system_value FROM host_info WHERE host_id = @host_id;"

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
        goto skip_loading;
    }

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        rrdhost_set_system_info_variable(system_info, (char *) sqlite3_column_text(res, 0),
                                         (char *) sqlite3_column_text(res, 1));
    }

skip_loading:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when reading host information");
    return;
}


#define SQL_INS_HOST_SYSTEM_INFO "INSERT OR REPLACE INTO host_info " \
    "(host_id, system_key, system_value, date_created) " \
    "VALUES (@host, @key, @value, unixepoch());"

void sql_store_host_system_info_key_value(uuid_t *host_id, const char *name, const char *value)
{
    sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return;
    }

    rc = sqlite3_prepare_v2(db_meta, SQL_INS_HOST_SYSTEM_INFO, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to store system info");
        return;
    }

    rc = sqlite3_bind_blob(res, 1, host_id, sizeof(*host_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host parameter to store system information");
        goto skip_store;
    }

    rc = sqlite3_bind_text(res, 2, name, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind label parameter to store name information");
        goto skip_store;
    }

    rc = sqlite3_bind_text(res, 3, value, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind value parameter to store value information");
        goto skip_store;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to store host system info, rc = %d", rc);

skip_store:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when storing  host system information");

    return;
}


void sql_store_host_system_info(uuid_t *host_id, const struct rrdhost_system_info *system_info)
{
    if (unlikely(!system_info))
        return;

    if (system_info->container_os_name)
        sql_store_host_system_info_key_value(host_id, "NETDATA_CONTAINER_OS_NAME", system_info->container_os_name);

    if (system_info->container_os_id)
        sql_store_host_system_info_key_value(host_id, "NETDATA_CONTAINER_OS_ID", system_info->container_os_id);

    if (system_info->container_os_id_like)
        sql_store_host_system_info_key_value(host_id, "NETDATA_CONTAINER_OS_ID_LIKE", system_info->container_os_id_like);

    if (system_info->container_os_version)
        sql_store_host_system_info_key_value(host_id, "NETDATA_CONTAINER_OS_VERSION", system_info->container_os_version);

    if (system_info->container_os_version_id)
        sql_store_host_system_info_key_value(host_id, "NETDATA_CONTAINER_OS_VERSION_ID", system_info->container_os_version_id);

    if (system_info->host_os_detection)
        sql_store_host_system_info_key_value(host_id, "NETDATA_CONTAINER_OS_DETECTION", system_info->host_os_detection);

    if (system_info->host_os_name)
        sql_store_host_system_info_key_value(host_id, "NETDATA_HOST_OS_NAME", system_info->host_os_name);

    if (system_info->host_os_id)
        sql_store_host_system_info_key_value(host_id, "NETDATA_HOST_OS_ID", system_info->host_os_id);

    if (system_info->host_os_id_like)
        sql_store_host_system_info_key_value(host_id, "NETDATA_HOST_OS_ID_LIKE", system_info->host_os_id_like);

    if (system_info->host_os_version)
        sql_store_host_system_info_key_value(host_id, "NETDATA_HOST_OS_VERSION", system_info->host_os_version);

    if (system_info->host_os_version_id)
        sql_store_host_system_info_key_value(host_id, "NETDATA_HOST_OS_VERSION_ID", system_info->host_os_version_id);

    if (system_info->host_os_detection)
        sql_store_host_system_info_key_value(host_id, "NETDATA_HOST_OS_DETECTION", system_info->host_os_detection);

    if (system_info->kernel_name)
        sql_store_host_system_info_key_value(host_id, "NETDATA_SYSTEM_KERNEL_NAME", system_info->kernel_name);

    if (system_info->host_cores)
        sql_store_host_system_info_key_value(host_id, "NETDATA_SYSTEM_CPU_LOGICAL_CPU_COUNT", system_info->host_cores);

    if (system_info->host_cpu_freq)
        sql_store_host_system_info_key_value(host_id, "NETDATA_SYSTEM_CPU_FREQ", system_info->host_cpu_freq);

    if (system_info->host_ram_total)
        sql_store_host_system_info_key_value(host_id, "NETDATA_SYSTEM_TOTAL_RAM", system_info->host_ram_total);

    if (system_info->host_disk_space)
        sql_store_host_system_info_key_value(host_id, "NETDATA_SYSTEM_TOTAL_DISK_SIZE", system_info->host_disk_space);

    if (system_info->kernel_version)
        sql_store_host_system_info_key_value(host_id, "NETDATA_SYSTEM_KERNEL_VERSION", system_info->kernel_version);

    if (system_info->architecture)
        sql_store_host_system_info_key_value(host_id, "NETDATA_SYSTEM_ARCHITECTURE", system_info->architecture);

    if (system_info->virtualization)
        sql_store_host_system_info_key_value(host_id, "NETDATA_SYSTEM_VIRTUALIZATION", system_info->virtualization);

    if (system_info->virt_detection)
        sql_store_host_system_info_key_value(host_id, "NETDATA_SYSTEM_VIRT_DETECTION", system_info->virt_detection);

    if (system_info->container)
        sql_store_host_system_info_key_value(host_id, "NETDATA_SYSTEM_CONTAINER", system_info->container);

    if (system_info->container_detection)
        sql_store_host_system_info_key_value(host_id, "NETDATA_SYSTEM_CONTAINER_DETECTION", system_info->container_detection);

    if (system_info->is_k8s_node)
        sql_store_host_system_info_key_value(host_id, "NETDATA_HOST_IS_K8S_NODE", system_info->is_k8s_node);

    return;
}

static int save_host_label_callback(const char *name, const char *value, RRDLABEL_SRC label_source, void *data)
{
    RRDHOST *host = (RRDHOST *)data;
    sql_store_host_label(&host->host_uuid, (int)label_source & ~(RRDLABEL_FLAG_INTERNAL), name, value);
    return 0;
}

#define SQL_DELETE_HOST_LABELS  "DELETE FROM host_label WHERE host_id = @uuid;"
void sql_store_host_labels(RRDHOST *host)
{
    int rc = exec_statement_with_uuid(SQL_DELETE_HOST_LABELS, &host->host_uuid);
    if (rc != SQLITE_OK)
        error_report("Failed to remove old host labels for host %s", rrdhost_hostname(host));

    rrdlabels_walkthrough_read(host->host_labels, save_host_label_callback, host);
}

#define SELECT_HOST_LABELS "SELECT label_key, label_value, source_type FROM host_label WHERE host_id = @host_id " \
    "AND label_key IS NOT NULL AND label_value IS NOT NULL;"

DICTIONARY *sql_load_host_labels(uuid_t *host_id)
{
    int rc;

    DICTIONARY *labels = NULL;
    sqlite3_stmt *res = NULL;

    rc = sqlite3_prepare_v2(db_meta, SELECT_HOST_LABELS, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to read host information");
        return NULL;
    }

    rc = sqlite3_bind_blob(res, 1, host_id, sizeof(*host_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host parameter host information");
        goto skip_loading;
    }

    labels = rrdlabels_create();

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        rrdlabels_add(
            labels,
            (const char *)sqlite3_column_text(res, 0),
            (const char *)sqlite3_column_text(res, 1),
            sqlite3_column_int(res, 2));
    }

skip_loading:
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

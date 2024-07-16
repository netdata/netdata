// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_db_migration.h"

static int return_int_cb(void *data, int argc, char **argv, char **column)
{
    int *status = data;
    UNUSED(argc);
    UNUSED(column);
    *status = (int) str2uint32_t(argv[0], NULL);
    return 0;
}

static int get_auto_vaccum(sqlite3 *database)
{
    char *err_msg = NULL;
    char sql[128];

    int exists = 0;

    snprintf(sql, sizeof(sql) - 1, "PRAGMA auto_vacuum");

    int rc = sqlite3_exec_monitored(database, sql, return_int_cb, (void *) &exists, &err_msg);
    if (rc != SQLITE_OK) {
        netdata_log_info("Error checking database auto vacuum setting; %s", err_msg);
        sqlite3_free(err_msg);
    }

    return exists;
}

int db_table_count(sqlite3 *database)
{
    char *err_msg = NULL;
    char sql[128];

    int count = 0;
    snprintf(sql, sizeof(sql) - 1, "select count(1) from sqlite_schema where type = 'table'");
    int rc = sqlite3_exec_monitored(database, sql, return_int_cb, (void *) &count, &err_msg);
    if (rc != SQLITE_OK) {
        netdata_log_info("Error checking database table count; %s", err_msg);
        sqlite3_free(err_msg);
    }
    return count;
}

int table_exists_in_database(sqlite3 *database, const char *table)
{
    char *err_msg = NULL;
    char sql[128];

    int exists = 0;

    snprintf(sql, sizeof(sql) - 1, "select 1 from sqlite_schema where type = 'table' and name = '%s'", table);

    int rc = sqlite3_exec_monitored(database, sql, return_int_cb, (void *) &exists, &err_msg);
    if (rc != SQLITE_OK) {
        netdata_log_info("Error checking table existence; %s", err_msg);
        sqlite3_free(err_msg);
    }

    return exists;
}

static int column_exists_in_table(sqlite3 *database, const char *table, const char *column)
{
    char *err_msg = NULL;
    char sql[128];

    int exists = 0;

    snprintf(sql, sizeof(sql) - 1, "SELECT 1 FROM pragma_table_info('%s') where name = '%s'", table, column);

    int rc = sqlite3_exec_monitored(database, sql, return_int_cb, (void *) &exists, &err_msg);
    if (rc != SQLITE_OK) {
        netdata_log_info("Error checking column existence; %s", err_msg);
        sqlite3_free(err_msg);
    }

    return exists;
}

static int get_database_user_version(sqlite3 *database)
{
    int user_version = 0;

    int rc = sqlite3_exec_monitored(database, "PRAGMA user_version", return_int_cb, (void *)&user_version, NULL);
    if (rc != SQLITE_OK)
        netdata_log_error("Failed to get user version for database");

    return user_version;
}

const char *database_migrate_v1_v2[] = {
    "ALTER TABLE host ADD hops INTEGER NOT NULL DEFAULT 0",
    NULL
};

const char *database_migrate_v2_v3[] = {
    "ALTER TABLE host ADD memory_mode INT NOT NULL DEFAULT 0",
    "ALTER TABLE host ADD abbrev_timezone TEXT NOT NULL DEFAULT ''",
    "ALTER TABLE host ADD utc_offset INT NOT NULL DEFAULT 0",
    "ALTER TABLE host ADD program_name TEXT NOT NULL DEFAULT 'unknown'",
    "ALTER TABLE host ADD program_version TEXT NOT NULL DEFAULT 'unknown'",
    "ALTER TABLE host ADD entries INT NOT NULL DEFAULT 0",
    "ALTER TABLE host ADD health_enabled INT NOT NULL DEFAULT 0",
    NULL
};

const char *database_migrate_v4_v5[] = {
    "DROP TABLE IF EXISTS chart_active",
    "DROP TABLE IF EXISTS dimension_active",
    "DROP TABLE IF EXISTS chart_hash",
    "DROP TABLE IF EXISTS chart_hash_map",
    "DROP VIEW IF EXISTS v_chart_hash",
    NULL
};

const char *database_migrate_v5_v6[] = {
    "DROP TRIGGER IF EXISTS tr_dim_del",
    "DROP TABLE IF EXISTS dimension_delete",
    NULL
};

const char *database_migrate_v9_v10[] = {
    "ALTER TABLE alert_hash ADD chart_labels TEXT",
    NULL
};

const char *database_migrate_v10_v11[] = {
    "ALTER TABLE health_log ADD chart_name TEXT",
    NULL
};

const char *database_migrate_v11_v12[] = {
    "ALTER TABLE health_log_detail ADD summary TEXT",
    "ALTER TABLE alert_hash ADD summary TEXT",
    NULL
};

const char *database_migrate_v12_v13_detail[] = {
    "ALTER TABLE health_log_detail ADD summary TEXT",
    NULL
};

const char *database_migrate_v12_v13_hash[] = {
    "ALTER TABLE alert_hash ADD summary TEXT",
    NULL
};

const char *database_migrate_v13_v14[] = {
    "ALTER TABLE host ADD last_connected INT NOT NULL DEFAULT 0",
    NULL
};

const char *database_migrate_v16_v17[] = {
    "ALTER TABLE alert_hash ADD time_group_condition INT",
    "ALTER TABLE alert_hash ADD time_group_value DOUBLE",
    "ALTER TABLE alert_hash ADD dims_group INT",
    "ALTER TABLE alert_hash ADD data_source INT",
    NULL
};

// Note: Same as database_migrate_v16_v17. This is not wrong
//       Do additional migration to handle agents that created wrong alert_hash table
const char *database_migrate_v17_v18[] = {
    "ALTER TABLE alert_hash ADD time_group_condition INT",
    "ALTER TABLE alert_hash ADD time_group_value DOUBLE",
    "ALTER TABLE alert_hash ADD dims_group INT",
    "ALTER TABLE alert_hash ADD data_source INT",
    NULL
};


static int do_migration_v1_v2(sqlite3 *database)
{
    if (table_exists_in_database(database, "host") && !column_exists_in_table(database, "host", "hops"))
        return init_database_batch(database, &database_migrate_v1_v2[0], "meta_migrate");
    return 0;
}

static int do_migration_v2_v3(sqlite3 *database)
{
    if (table_exists_in_database(database, "host") && !column_exists_in_table(database, "host", "memory_mode"))
        return init_database_batch(database, &database_migrate_v2_v3[0], "meta_migrate");
    return 0;
}

static int do_migration_v3_v4(sqlite3 *database)
{
    char sql[256];

    int rc;
    sqlite3_stmt *res = NULL;
    snprintfz(sql, sizeof(sql) - 1, "SELECT name FROM sqlite_schema WHERE type ='table' AND name LIKE 'health_log_%%'");
    rc = sqlite3_prepare_v2(database, sql, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to alter health_log tables");
        return 1;
    }

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
         char *table = strdupz((char *) sqlite3_column_text(res, 0));
         if (!column_exists_in_table(database, table, "chart_context")) {
             snprintfz(sql, sizeof(sql) - 1, "ALTER TABLE %s ADD chart_context text", table);
             sqlite3_exec_monitored(database, sql, 0, 0, NULL);
         }
         freez(table);
    }

    SQLITE_FINALIZE(res);

    return 0;
}

static int do_migration_v4_v5(sqlite3 *database)
{
    return init_database_batch(database, &database_migrate_v4_v5[0], "meta_migrate");
}

static int do_migration_v5_v6(sqlite3 *database)
{
    return init_database_batch(database, &database_migrate_v5_v6[0], "meta_migrate");
}

static int do_migration_v6_v7(sqlite3 *database)
{
    char sql[256];

    int rc;
    sqlite3_stmt *res = NULL;
    snprintfz(sql, sizeof(sql) - 1, "SELECT name FROM sqlite_schema WHERE type ='table' AND name LIKE 'aclk_alert_%%'");
    rc = sqlite3_prepare_v2(database, sql, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to alter aclk_alert tables");
        return 1;
    }

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
         char *table = strdupz((char *) sqlite3_column_text(res, 0));
         if (!column_exists_in_table(database, table, "filtered_alert_unique_id")) {
             snprintfz(sql, sizeof(sql) - 1, "ALTER TABLE %s ADD filtered_alert_unique_id", table);
             sqlite3_exec_monitored(database, sql, 0, 0, NULL);
             snprintfz(sql, sizeof(sql) - 1, "UPDATE %s SET filtered_alert_unique_id = alert_unique_id", table);
             sqlite3_exec_monitored(database, sql, 0, 0, NULL);
         }
         freez(table);
    }

    SQLITE_FINALIZE(res);

    return 0;
}

static int do_migration_v7_v8(sqlite3 *database)
{
    char sql[256];

    int rc;
    sqlite3_stmt *res = NULL;
    snprintfz(sql, sizeof(sql) - 1, "SELECT name FROM sqlite_schema WHERE type ='table' AND name LIKE 'health_log_%%'");
    rc = sqlite3_prepare_v2(database, sql, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to alter health_log tables");
        return 1;
    }

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
         char *table = strdupz((char *) sqlite3_column_text(res, 0));
         if (!column_exists_in_table(database, table, "transition_id")) {
             snprintfz(sql, sizeof(sql) - 1, "ALTER TABLE %s ADD transition_id blob", table);
             sqlite3_exec_monitored(database, sql, 0, 0, NULL);
         }
         freez(table);
    }

    SQLITE_FINALIZE(res);

    return 0;
}

static int do_migration_v8_v9(sqlite3 *database)
{
    char sql[2048];
    int rc;
    sqlite3_stmt *res = NULL;

    //create the health_log table and it's index
    snprintfz(sql, sizeof(sql) - 1, "CREATE TABLE IF NOT EXISTS health_log (health_log_id INTEGER PRIMARY KEY, host_id blob, alarm_id int, " \
              "config_hash_id blob, name text, chart text, family text, recipient text, units text, exec text, " \
              "chart_context text, last_transition_id blob, UNIQUE (host_id, alarm_id))");
    sqlite3_exec_monitored(database, sql, 0, 0, NULL);

    //TODO indexes
    snprintfz(sql, sizeof(sql) - 1, "CREATE INDEX IF NOT EXISTS health_log_ind_1 ON health_log (host_id)");
    sqlite3_exec_monitored(database, sql, 0, 0, NULL);

    snprintfz(sql, sizeof(sql) - 1, "CREATE TABLE IF NOT EXISTS health_log_detail (health_log_id int, unique_id int, alarm_id int, alarm_event_id int, " \
              "updated_by_id int, updates_id int, when_key int, duration int, non_clear_duration int, " \
              "flags int, exec_run_timestamp int, delay_up_to_timestamp int, " \
              "info text, exec_code int, new_status real, old_status real, delay int, " \
              "new_value double, old_value double, last_repeat int, transition_id blob, global_id int, summary text, host_id blob)");
    sqlite3_exec_monitored(database, sql, 0, 0, NULL);

    snprintfz(sql, sizeof(sql) - 1, "CREATE INDEX IF NOT EXISTS health_log_d_ind_1 ON health_log_detail (unique_id)");
    sqlite3_exec_monitored(database, sql, 0, 0, NULL);
    snprintfz(sql, sizeof(sql) - 1, "CREATE INDEX IF NOT EXISTS health_log_d_ind_2 ON health_log_detail (global_id)");
    sqlite3_exec_monitored(database, sql, 0, 0, NULL);
    snprintfz(sql, sizeof(sql) - 1, "CREATE INDEX IF NOT EXISTS health_log_d_ind_3 ON health_log_detail (transition_id)");
    sqlite3_exec_monitored(database, sql, 0, 0, NULL);
    snprintfz(sql, sizeof(sql) - 1, "CREATE INDEX IF NOT EXISTS health_log_d_ind_4 ON health_log_detail (health_log_id)");
    sqlite3_exec_monitored(database, sql, 0, 0, NULL);

    snprintfz(sql, sizeof(sql) - 1, "ALTER TABLE alert_hash ADD source text");
    sqlite3_exec_monitored(database, sql, 0, 0, NULL);

    snprintfz(sql, sizeof(sql) - 1, "CREATE INDEX IF NOT EXISTS alert_hash_index ON alert_hash (hash_id)");
    sqlite3_exec_monitored(database, sql, 0, 0, NULL);

    snprintfz(sql, sizeof(sql) - 1, "SELECT name FROM sqlite_schema WHERE type ='table' AND name LIKE 'health_log_%%' AND name <> 'health_log_detail'");
    rc = sqlite3_prepare_v2(database, sql, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to alter health_log tables");
        return 1;
    }

    DICTIONARY *dict_tables = dictionary_create(DICT_OPTION_NONE);

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        char *table = strdupz((char *) sqlite3_column_text(res, 0));
        if (health_migrate_old_health_log_table(table)) {
            dictionary_set(dict_tables, table, NULL, 0);
        }
        freez(table);
    }

    SQLITE_FINALIZE(res);

    char *table = NULL;
    dfe_start_read(dict_tables, table) {
        sql_drop_table(table_dfe.name);
    }
    dfe_done(table);
    dictionary_destroy(dict_tables);

    snprintfz(sql, sizeof(sql) - 1, "ALTER TABLE health_log_detail DROP COLUMN host_id");
    sqlite3_exec_monitored(database, sql, 0, 0, NULL);

    return 0;
}

static int do_migration_v9_v10(sqlite3 *database)
{
    if (table_exists_in_database(database, "alert_hash") && !column_exists_in_table(database, "alert_hash", "chart_labels"))
        return init_database_batch(database, &database_migrate_v9_v10[0], "meta_migrate");
    return 0;
}

static int do_migration_v10_v11(sqlite3 *database)
{
    if (table_exists_in_database(database, "health_log") && !column_exists_in_table(database, "health_log", "chart_name"))
        return init_database_batch(database, &database_migrate_v10_v11[0], "meta_migrate");

    return 0;
}

#define MIGR_11_12_UPD_HEALTH_LOG_DETAIL "UPDATE health_log_detail SET summary = (select name from health_log where health_log_id = health_log_detail.health_log_id)"
static int do_migration_v11_v12(sqlite3 *database)
{
    int rc = 0;

    if (table_exists_in_database(database, "health_log_detail") && !column_exists_in_table(database, "health_log_detail", "summary") &&
        table_exists_in_database(database, "alert_hash") && !column_exists_in_table(database, "alert_hash", "summary"))
        rc = init_database_batch(database, &database_migrate_v11_v12[0], "meta_migrate");

    if (!rc)
        sqlite3_exec_monitored(database, MIGR_11_12_UPD_HEALTH_LOG_DETAIL, 0, 0, NULL);

    return rc;
}

static int do_migration_v14_v15(sqlite3 *database)
{
    char sql[256];

    int rc;
    sqlite3_stmt *res = NULL;
    snprintfz(sql, sizeof(sql) - 1, "SELECT name FROM sqlite_schema WHERE type = \"index\" AND name LIKE \"aclk_alert_index@_%%\" ESCAPE \"@\"");
    rc = sqlite3_prepare_v2(database, sql, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to drop unused indices");
        return 1;
    }

    BUFFER *wb = buffer_create(128, NULL);
    size_t count = 0;
    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        buffer_sprintf(wb, "DROP INDEX IF EXISTS %s; ", (char *)sqlite3_column_text(res, 0));
        count++;
    }

    SQLITE_FINALIZE(res);

    if (count)
        (void) db_execute(database, buffer_tostring(wb));

    buffer_free(wb);
    return 0;
}

static int do_migration_v15_v16(sqlite3 *database)
{
    char sql[256];

    int rc;
    sqlite3_stmt *res = NULL;
    snprintfz(sql, sizeof(sql) - 1, "SELECT name FROM sqlite_schema WHERE type = \"table\" AND name LIKE \"aclk_alert_%%\"");
    rc = sqlite3_prepare_v2(database, sql, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to drop unused indices");
        return 1;
    }

    BUFFER *wb = buffer_create(128, NULL);
    size_t count = 0;
    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        buffer_sprintf(wb, "ANALYZE %s ; ", (char *)sqlite3_column_text(res, 0));
        count++;
    }

    SQLITE_FINALIZE(res);

    if (count)
        (void) db_execute(database, buffer_tostring(wb));

    buffer_free(wb);
    return 0;
}

static int do_migration_v16_v17(sqlite3 *database)
{
    if (table_exists_in_database(database, "alert_hash") && !column_exists_in_table(database, "alert_hash", "time_group_condition"))
        return init_database_batch(database, &database_migrate_v16_v17[0], "meta_migrate");

    return 0;
}

static int do_migration_v17_v18(sqlite3 *database)
{
    if (table_exists_in_database(database, "alert_hash") && !column_exists_in_table(database, "alert_hash", "time_group_condition"))
        return init_database_batch(database, &database_migrate_v17_v18[0], "meta_migrate");

    return 0;
}


static int do_migration_v12_v13(sqlite3 *database)
{
    int rc = 0;

    if (table_exists_in_database(database, "health_log_detail") && !column_exists_in_table(database, "health_log_detail", "summary")) {
        rc = init_database_batch(database, &database_migrate_v12_v13_detail[0], "meta_migrate");
        sqlite3_exec_monitored(database, MIGR_11_12_UPD_HEALTH_LOG_DETAIL, 0, 0, NULL);
    }

    if (table_exists_in_database(database, "alert_hash") && !column_exists_in_table(database, "alert_hash", "summary"))
        rc = init_database_batch(database, &database_migrate_v12_v13_hash[0], "meta_migrate");

    return rc;
}

static int do_migration_v13_v14(sqlite3 *database)
{
    if (table_exists_in_database(database, "host") && !column_exists_in_table(database, "host", "last_connected"))
        return init_database_batch(database, &database_migrate_v13_v14[0], "meta_migrate");

    return 0;
}


// Actions for ML migration
const char *database_ml_migrate_v1_v2[] = {
    "PRAGMA journal_mode=delete",
    "PRAGMA journal_mode=WAL",
    "PRAGMA auto_vacuum=2",
    "VACUUM",
    NULL
};

static int do_ml_migration_v1_v2(sqlite3 *database)
{
    if (get_auto_vaccum(database) != 2)
        return init_database_batch(database, &database_ml_migrate_v1_v2[0], "ml_migrate");
    return 0;
}

static int do_migration_noop(sqlite3 *database)
{
    UNUSED(database);
    return 0;
}

typedef struct database_func_migration_list {
    char *name;
    int (*func)(sqlite3 *database);
} DATABASE_FUNC_MIGRATION_LIST;


static int migrate_database(sqlite3 *database, int target_version, char *db_name, DATABASE_FUNC_MIGRATION_LIST *migration_list)
{
    int user_version = 0;
    char *err_msg = NULL;

    int rc = sqlite3_exec_monitored(database, "PRAGMA user_version", return_int_cb, (void *) &user_version, &err_msg);
    if (rc != SQLITE_OK) {
        netdata_log_info("Error checking the %s database version; %s", db_name, err_msg);
        sqlite3_free(err_msg);
    }

    if (likely(user_version == target_version)) {
        errno_clear();
        netdata_log_info("%s database version is %d (no migration needed)", db_name, target_version);
        return target_version;
    }

    netdata_log_info("Database version is %d, current version is %d. Running migration for %s ...", user_version, target_version, db_name);
    for (int i = user_version; i < target_version && migration_list[i].func; i++) {
        netdata_log_info("Running database \"%s\" migration %s", db_name, migration_list[i].name);
        rc = (migration_list[i].func)(database);
        if (unlikely(rc)) {
            error_report("Database %s migration from version %d to version %d failed", db_name, i, i + 1);
            return i;
        }
    }
    return target_version;
}

DATABASE_FUNC_MIGRATION_LIST migration_action[] = {
    {.name = "v0 to v1",  .func = do_migration_noop},
    {.name = "v1 to v2",  .func = do_migration_v1_v2},
    {.name = "v2 to v3",  .func = do_migration_v2_v3},
    {.name = "v3 to v4",  .func = do_migration_v3_v4},
    {.name = "v4 to v5",  .func = do_migration_v4_v5},
    {.name = "v5 to v6",  .func = do_migration_v5_v6},
    {.name = "v6 to v7",  .func = do_migration_v6_v7},
    {.name = "v7 to v8",  .func = do_migration_v7_v8},
    {.name = "v8 to v9",  .func = do_migration_v8_v9},
    {.name = "v9 to v10",  .func = do_migration_v9_v10},
    {.name = "v10 to v11",  .func = do_migration_v10_v11},
    {.name = "v11 to v12",  .func = do_migration_v11_v12},
    {.name = "v12 to v13",  .func = do_migration_v12_v13},
    {.name = "v13 to v14",  .func = do_migration_v13_v14},
    {.name = "v14 to v15",  .func = do_migration_v14_v15},
    {.name = "v15 to v16",  .func = do_migration_v15_v16},
    {.name = "v16 to v17",  .func = do_migration_v16_v17},
    {.name = "v17 to v18",  .func = do_migration_v17_v18},
    // the terminator of this array
    {.name = NULL, .func = NULL}
};

DATABASE_FUNC_MIGRATION_LIST context_migration_action[] = {
    {.name = "v0 to v1",  .func = do_migration_noop},
    // the terminator of this array
    {.name = NULL, .func = NULL}
};

DATABASE_FUNC_MIGRATION_LIST ml_migration_action[] = {
    {.name = "v0 to v1",  .func = do_migration_noop},
    {.name = "v1 to v2",  .func = do_ml_migration_v1_v2},
    // the terminator of this array
    {.name = NULL, .func = NULL}
};

int perform_database_migration(sqlite3 *database, int target_version)
{
    int user_version = get_database_user_version(database);

    if (!user_version && !db_table_count(database))
        return target_version;

    return migrate_database(database, target_version, "metadata", migration_action);
}

int perform_context_database_migration(sqlite3 *database, int target_version)
{
    int user_version = get_database_user_version(database);

    if (!user_version && !table_exists_in_database(database, "context"))
        return target_version;

    return migrate_database(database, target_version, "context", context_migration_action);
}

int perform_ml_database_migration(sqlite3 *database, int target_version)
{
    return migrate_database(database, target_version, "ml", ml_migration_action);
}

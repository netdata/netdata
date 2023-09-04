// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_db_migration.h"

static int return_int_cb(void *data, int argc, char **argv, char **column)
{
    int *status = data;
    UNUSED(argc);
    UNUSED(column);
    *status = str2uint32_t(argv[0], NULL);
    return 0;
}

int table_exists_in_database(const char *table)
{
    char *err_msg = NULL;
    char sql[128];

    int exists = 0;

    snprintf(sql, 127, "select 1 from sqlite_schema where type = 'table' and name = '%s';", table);

    int rc = sqlite3_exec_monitored(db_meta, sql, return_int_cb, (void *) &exists, &err_msg);
    if (rc != SQLITE_OK) {
        netdata_log_info("Error checking table existence; %s", err_msg);
        sqlite3_free(err_msg);
    }

    return exists;
}

static int column_exists_in_table(const char *table, const char *column)
{
    char *err_msg = NULL;
    char sql[128];

    int exists = 0;

    snprintf(sql, 127, "SELECT 1 FROM pragma_table_info('%s') where name = '%s';", table, column);

    int rc = sqlite3_exec_monitored(db_meta, sql, return_int_cb, (void *) &exists, &err_msg);
    if (rc != SQLITE_OK) {
        netdata_log_info("Error checking column existence; %s", err_msg);
        sqlite3_free(err_msg);
    }

    return exists;
}

const char *database_migrate_v1_v2[] = {
    "ALTER TABLE host ADD hops INTEGER NOT NULL DEFAULT 0;",
    NULL
};

const char *database_migrate_v2_v3[] = {
    "ALTER TABLE host ADD memory_mode INT NOT NULL DEFAULT 0;",
    "ALTER TABLE host ADD abbrev_timezone TEXT NOT NULL DEFAULT '';",
    "ALTER TABLE host ADD utc_offset INT NOT NULL DEFAULT 0;",
    "ALTER TABLE host ADD program_name TEXT NOT NULL DEFAULT 'unknown';",
    "ALTER TABLE host ADD program_version TEXT NOT NULL DEFAULT 'unknown';",
    "ALTER TABLE host ADD entries INT NOT NULL DEFAULT 0;",
    "ALTER TABLE host ADD health_enabled INT NOT NULL DEFAULT 0;",
    NULL
};

const char *database_migrate_v4_v5[] = {
    "DROP TABLE IF EXISTS chart_active;",
    "DROP TABLE IF EXISTS dimension_active;",
    "DROP TABLE IF EXISTS chart_hash;",
    "DROP TABLE IF EXISTS chart_hash_map;",
    "DROP VIEW IF EXISTS v_chart_hash;",
    NULL
};

const char *database_migrate_v5_v6[] = {
    "DROP TRIGGER IF EXISTS tr_dim_del;",
    "DROP TABLE IF EXISTS dimension_delete;",
    NULL
};

const char *database_migrate_v9_v10[] = {
    "ALTER TABLE alert_hash ADD chart_labels TEXT;",
    NULL
};

const char *database_migrate_v10_v11[] = {
    "ALTER TABLE health_log ADD chart_name TEXT;",
    NULL
};

static int do_migration_v1_v2(sqlite3 *database, const char *name)
{
    UNUSED(name);
    netdata_log_info("Running \"%s\" database migration", name);

    if (table_exists_in_database("host") && !column_exists_in_table("host", "hops"))
        return init_database_batch(database, &database_migrate_v1_v2[0]);
    return 0;
}

static int do_migration_v2_v3(sqlite3 *database, const char *name)
{
    UNUSED(name);
    netdata_log_info("Running \"%s\" database migration", name);

    if (table_exists_in_database("host") && !column_exists_in_table("host", "memory_mode"))
        return init_database_batch(database, &database_migrate_v2_v3[0]);
    return 0;
}

static int do_migration_v3_v4(sqlite3 *database, const char *name)
{
    UNUSED(name);
    netdata_log_info("Running database migration %s", name);

    char sql[256];

    int rc;
    sqlite3_stmt *res = NULL;
    snprintfz(sql, 255, "SELECT name FROM sqlite_schema WHERE type ='table' AND name LIKE 'health_log_%%';");
    rc = sqlite3_prepare_v2(database, sql, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to alter health_log tables");
        return 1;
    }

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
         char *table = strdupz((char *) sqlite3_column_text(res, 0));
         if (!column_exists_in_table(table, "chart_context")) {
             snprintfz(sql, 255, "ALTER TABLE %s ADD chart_context text", table);
             sqlite3_exec_monitored(database, sql, 0, 0, NULL);
         }
         freez(table);
    }

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement when altering health_log tables, rc = %d", rc);

    return 0;
}

static int do_migration_v4_v5(sqlite3 *database, const char *name)
{
    UNUSED(name);
    netdata_log_info("Running \"%s\" database migration", name);

    return init_database_batch(database, &database_migrate_v4_v5[0]);
}

static int do_migration_v5_v6(sqlite3 *database, const char *name)
{
    UNUSED(name);
    netdata_log_info("Running \"%s\" database migration", name);

    return init_database_batch(database, &database_migrate_v5_v6[0]);
}

static int do_migration_v6_v7(sqlite3 *database, const char *name)
{
    UNUSED(name);
    netdata_log_info("Running \"%s\" database migration", name);

    char sql[256];

    int rc;
    sqlite3_stmt *res = NULL;
    snprintfz(sql, 255, "SELECT name FROM sqlite_schema WHERE type ='table' AND name LIKE 'aclk_alert_%%';");
    rc = sqlite3_prepare_v2(database, sql, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to alter aclk_alert tables");
        return 1;
    }

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
         char *table = strdupz((char *) sqlite3_column_text(res, 0));
         if (!column_exists_in_table(table, "filtered_alert_unique_id")) {
             snprintfz(sql, 255, "ALTER TABLE %s ADD filtered_alert_unique_id", table);
             sqlite3_exec_monitored(database, sql, 0, 0, NULL);
             snprintfz(sql, 255, "UPDATE %s SET filtered_alert_unique_id = alert_unique_id", table);
             sqlite3_exec_monitored(database, sql, 0, 0, NULL);
         }
         freez(table);
    }

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement when altering aclk_alert tables, rc = %d", rc);

    return 0;
}

static int do_migration_v7_v8(sqlite3 *database, const char *name)
{
    UNUSED(name);
    netdata_log_info("Running database migration %s", name);

    char sql[256];

    int rc;
    sqlite3_stmt *res = NULL;
    snprintfz(sql, 255, "SELECT name FROM sqlite_schema WHERE type ='table' AND name LIKE 'health_log_%%';");
    rc = sqlite3_prepare_v2(database, sql, -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to alter health_log tables");
        return 1;
    }

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
         char *table = strdupz((char *) sqlite3_column_text(res, 0));
         if (!column_exists_in_table(table, "transition_id")) {
             snprintfz(sql, 255, "ALTER TABLE %s ADD transition_id blob", table);
             sqlite3_exec_monitored(database, sql, 0, 0, NULL);
         }
         freez(table);
    }

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement when altering health_log tables, rc = %d", rc);

    return 0;
}

static int do_migration_v8_v9(sqlite3 *database, const char *name)
{
    netdata_log_info("Running database migration %s", name);

    char sql[2048];
    int rc;
    sqlite3_stmt *res = NULL;

    //create the health_log table and it's index
    snprintfz(sql, 2047, "CREATE TABLE IF NOT EXISTS health_log (health_log_id INTEGER PRIMARY KEY, host_id blob, alarm_id int, " \
              "config_hash_id blob, name text, chart text, family text, recipient text, units text, exec text, " \
              "chart_context text, last_transition_id blob, UNIQUE (host_id, alarm_id)) ;");
    sqlite3_exec_monitored(database, sql, 0, 0, NULL);

    //TODO indexes
    snprintfz(sql, 2047, "CREATE INDEX IF NOT EXISTS health_log_ind_1 ON health_log (host_id);");
    sqlite3_exec_monitored(database, sql, 0, 0, NULL);

    snprintfz(sql, 2047, "CREATE TABLE IF NOT EXISTS health_log_detail (health_log_id int, unique_id int, alarm_id int, alarm_event_id int, " \
              "updated_by_id int, updates_id int, when_key int, duration int, non_clear_duration int, " \
              "flags int, exec_run_timestamp int, delay_up_to_timestamp int, " \
              "info text, exec_code int, new_status real, old_status real, delay int, " \
              "new_value double, old_value double, last_repeat int, transition_id blob, global_id int, host_id blob);");
    sqlite3_exec_monitored(database, sql, 0, 0, NULL);

    snprintfz(sql, 2047, "CREATE INDEX IF NOT EXISTS health_log_d_ind_1 ON health_log_detail (unique_id);");
    sqlite3_exec_monitored(database, sql, 0, 0, NULL);
    snprintfz(sql, 2047, "CREATE INDEX IF NOT EXISTS health_log_d_ind_2 ON health_log_detail (global_id);");
    sqlite3_exec_monitored(database, sql, 0, 0, NULL);
    snprintfz(sql, 2047, "CREATE INDEX IF NOT EXISTS health_log_d_ind_3 ON health_log_detail (transition_id);");
    sqlite3_exec_monitored(database, sql, 0, 0, NULL);
    snprintfz(sql, 2047, "CREATE INDEX IF NOT EXISTS health_log_d_ind_4 ON health_log_detail (health_log_id);");
    sqlite3_exec_monitored(database, sql, 0, 0, NULL);

    snprintfz(sql, 2047, "ALTER TABLE alert_hash ADD source text;");
    sqlite3_exec_monitored(database, sql, 0, 0, NULL);

    snprintfz(sql, 2047, "CREATE INDEX IF NOT EXISTS alert_hash_index ON alert_hash (hash_id);");
    sqlite3_exec_monitored(database, sql, 0, 0, NULL);

    snprintfz(sql, 2047, "SELECT name FROM sqlite_schema WHERE type ='table' AND name LIKE 'health_log_%%' AND name <> 'health_log_detail';");
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

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement when copying health_log tables, rc = %d", rc);

    char *table = NULL;
    dfe_start_read(dict_tables, table) {
        sql_drop_table(table_dfe.name);
    }
    dfe_done(table);
    dictionary_destroy(dict_tables);

    snprintfz(sql, 2047, "ALTER TABLE health_log_detail DROP COLUMN host_id;");
    sqlite3_exec_monitored(database, sql, 0, 0, NULL);

    return 0;
}

static int do_migration_v9_v10(sqlite3 *database, const char *name)
{
    netdata_log_info("Running \"%s\" database migration", name);

    if (table_exists_in_database("alert_hash") && !column_exists_in_table("alert_hash", "chart_labels"))
        return init_database_batch(database, &database_migrate_v9_v10[0]);
    return 0;
}

static int do_migration_v10_v11(sqlite3 *database, const char *name)
{
    netdata_log_info("Running \"%s\" database migration", name);

    if (table_exists_in_database("health_log") && !column_exists_in_table("health_log", "chart_name"))
        return init_database_batch(database, &database_migrate_v10_v11[0]);

    return 0;
}

static int do_migration_noop(sqlite3 *database, const char *name)
{
    UNUSED(database);
    UNUSED(name);
    netdata_log_info("Running database migration %s", name);
    return 0;
}

typedef struct database_func_migration_list {
    char *name;
    int (*func)(sqlite3 *database, const char *name);
} DATABASE_FUNC_MIGRATION_LIST;


static int migrate_database(sqlite3 *database, int target_version, char *db_name, DATABASE_FUNC_MIGRATION_LIST *migration_list)
{
    int user_version = 0;
    char *err_msg = NULL;

    int rc = sqlite3_exec_monitored(database, "PRAGMA user_version;", return_int_cb, (void *) &user_version, &err_msg);
    if (rc != SQLITE_OK) {
        netdata_log_info("Error checking the %s database version; %s", db_name, err_msg);
        sqlite3_free(err_msg);
    }

    if (likely(user_version == target_version)) {
        netdata_log_info("%s database version is %d (no migration needed)", db_name, target_version);
        return target_version;
    }

    netdata_log_info("Database version is %d, current version is %d. Running migration for %s ...", user_version, target_version, db_name);
    for (int i = user_version; i < target_version && migration_list[i].func; i++) {
        rc = (migration_list[i].func)(database, migration_list[i].name);
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
    // the terminator of this array
    {.name = NULL, .func = NULL}
};

DATABASE_FUNC_MIGRATION_LIST context_migration_action[] = {
    {.name = "v0 to v1",  .func = do_migration_noop},
    // the terminator of this array
    {.name = NULL, .func = NULL}
};


int perform_database_migration(sqlite3 *database, int target_version)
{
    return migrate_database(database, target_version, "metadata", migration_action);
}

int perform_context_database_migration(sqlite3 *database, int target_version)
{
    return migrate_database(database, target_version, "context", context_migration_action);
}

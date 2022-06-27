// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_db_migration.h"

static int return_int_cb(void *data, int argc, char **argv, char **column)
{
    int *status = data;
    UNUSED(argc);
    UNUSED(column);
    *status = str2uint32_t(argv[0]);
    return 0;
}

static int column_exists_in_table(const char *table, const char *column)
{
    char *err_msg = NULL;
    char sql[128];

    int exists = 0;

    snprintf(sql, 127, "SELECT 1 FROM pragma_table_info('%s') where name = '%s';", table, column);

    int rc = sqlite3_exec(db_meta, sql, return_int_cb, (void *) &exists, &err_msg);
    if (rc != SQLITE_OK) {
        info("Error checking column existence; %s", err_msg);
        sqlite3_free(err_msg);
    }

    return exists;
}

static int do_migration_noop(sqlite3 *database, const char *name)
{
    UNUSED(database);
    UNUSED(name);
    info("Running database migration %s", name);
    return 0;
}

static struct database_func_migration_list {
    char *name;
    int (*func)(sqlite3 *database, const char *name);
} migration_action[] = {
    {.name = "v0 to v1",  .func = do_migration_noop},
    // the terminator of this array
    {.name = NULL, .func = NULL}
};


static int perform_database_migration_cb(void *data, int argc, char **argv, char **column)
{
    int *status = data;
    UNUSED(argc);
    UNUSED(column);
    *status = str2uint32_t(argv[0]);
    return 0;
}

int perform_database_migration(sqlite3 *database, int target_version)
{
    int user_version = 0;
    char *err_msg = NULL;

    int rc = sqlite3_exec(database, "PRAGMA user_version;", perform_database_migration_cb, (void *) &user_version, &err_msg);
    if (rc != SQLITE_OK) {
        info("Error checking the database version; %s", err_msg);
        sqlite3_free(err_msg);
    }

    if (likely(user_version == target_version)) {
        info("Metadata database version is %d", target_version);
        return target_version;
    }

    info("Database version is %d, current version is %d. Running migration ...", user_version, target_version);
    for (int i = user_version; migration_action[i].func && i < target_version; i++) {
        rc = (migration_action[i].func)(database, migration_action[i].name);
        if (unlikely(rc)) {
            error_report("Database migration from version %d to version %d failed", i, i + 1);
            return i;
        }
    }
    return target_version;
}

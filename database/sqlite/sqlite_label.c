// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_label.h"
#include "sqlite_db_migration.h"

#define DB_LABEL_METADATA_VERSION 2

const char *database_label_config[] = {
    "CREATE TABLE IF NOT EXISTS chart_label(chart_id blob, source_type int, label_key text, "
    "label_value text, date_created int, PRIMARY KEY (chart_id, label_key));",

    "CREATE TABLE IF NOT EXISTS host_label(host_id blob, source_type int, label_key text NOT NULL, "
    "label_value text NOT NULL, date_created INT, PRIMARY KEY (host_id, label_key));",

    NULL
};

const char *database_label_cleanup[] = {
    "VACUUM;",
    NULL
};

sqlite3 *db_label_meta = NULL;
/*
 * Initialize the SQLite database
 * Return 0 on success
 */
int sql_init_label_database(int memory)
{
    char sqlite_database[FILENAME_MAX + 1];
    int rc;

    if (likely(!memory))
        snprintfz(sqlite_database, FILENAME_MAX, "%s/netdata-label.db", netdata_configured_cache_dir);
    else
        strcpy(sqlite_database, ":memory:");

    rc = sqlite3_open(sqlite_database, &db_label_meta);
    if (rc != SQLITE_OK) {
        error_report("Failed to initialize database at %s, due to \"%s\"", sqlite_database, sqlite3_errstr(rc));
        sqlite3_close(db_label_meta);
        db_label_meta = NULL;
        return 1;
    }

    info("SQLite database %s initialization", sqlite_database);

    if (configure_database_params(db_label_meta))
        return 1;

    if (init_database_batch(db_label_meta, &database_label_config[0]))
        return 1;

    if (attach_database(db_label_meta, memory ? NULL : "netdata-meta.db", "meta"))
        return 1;

    int target_version = DB_LABEL_METADATA_VERSION;
    if (likely(!memory))
        target_version = perform_label_database_migration(db_label_meta, DB_LABEL_METADATA_VERSION);

    if (database_set_version(db_label_meta, target_version))
        return 1;

    if (init_database_batch(db_label_meta, &database_label_cleanup[0]))
        return 1;

    rc = sqlite3_create_function(db_label_meta, "u2h", 1, SQLITE_ANY | SQLITE_DETERMINISTIC, 0, sqlite_uuid_parse, 0, 0);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to register internal u2h function");

    info("SQLite database %s initialization completed", sqlite_database);
    return 0;
}

int sql_label_cache_stats(int op)
{
    int count, dummy;

    if (unlikely(!db_label_meta))
        return 0;

    netdata_thread_disable_cancelability();
    int rc = sqlite3_db_status(db_label_meta, op, &count, &dummy, 0);
    netdata_thread_enable_cancelability();

    if (SQLITE_OK == rc)
        return count;
    error_report("METADATA: SQLITE statistics failed with rc = %d, %s", rc, sqlite3_errstr(rc));
    return 0;
}


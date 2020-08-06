// SPDX-License-Identifier: GPL-3.0-or-later

#include <sqlite3.h>
#include "sqlite_functions.h"

sqlite3 *db = NULL;

int sql_init_database()
{
    char *err_msg = NULL;

    int rc = sqlite3_open("/tmp/database", &db);
    //int rc = sqlite3_open(NULL, &db);

    info("SQLITE Database initialized (rc = %d)", rc);

    char *sql = "PRAGMA synchronous=0 ; CREATE TABLE IF NOT EXISTS dimension(dim_uuid text PRIMARY KEY, chart_uuid text, id text, name text, archived int);";

    rc = sqlite3_exec(db, sql, 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
        sqlite3_close(db);
    }

    return rc;
}

int sql_close_database()
{
    if (db)
        sqlite3_close(db);
    return 0;
}


int sql_store_dimension(uuid_t *dim_uuid, uuid_t *chart_uuid, const char *id, const char *name, collected_number multiplier,
                         collected_number divisor, int algorithm)
{
    char *err_msg = NULL;
    char  sql[1024];
    char  dim_str[37], chart_str[37];
    int rc;

    if (!db) {
        sql_init_database();
    }

    uuid_unparse_lower(dim_uuid, dim_str);
    uuid_unparse_lower(chart_uuid, chart_str);

    //info("SQLITE: Adding dimension [%s] [%s]", id, name);

    sprintf(sql, "INSERT OR REPLACE INTO dimension (dim_uuid, chart_uuid, id, name, archived) values ('%s','%s','%s','%s', 1) ;",
            dim_str, chart_str, id, name);
    //info("%s", sql);

    rc = sqlite3_exec(db, sql, 0, 0, &err_msg);
    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    return  0;
}
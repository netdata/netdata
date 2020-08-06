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

    uuid_unparse_lower(*dim_uuid, dim_str);
    uuid_unparse_lower(*chart_uuid, chart_str);

    sprintf(sql, "INSERT into dimension (dim_uuid, chart_uuid, id, name, archived) values ('%s','%s','%s','%s', 1) ;",
            dim_str, chart_str, id, name);
    //info("%s", sql);

    rc = sqlite3_exec(db, sql, 0, 0, &err_msg);
    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    return  0;
}

int sql_dimension_archive(uuid_t *dim_uuid, int archive)
{
    char *err_msg = NULL;
    char  sql[1024];
    char  dim_str[37];
    int rc;

    if (!db) {
        sql_init_database();
    }

    uuid_unparse_lower(*dim_uuid, dim_str);

    sprintf(sql, "update dimension set archived = %d where dim_uuid = '%s';", archive, dim_str);

    rc = sqlite3_exec(db, sql, 0, 0, &err_msg);
    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    return  0;
}

int dim_callback(void *dim_ptr, int argc, char **argv, char **azColName)
{
    //struct dimension *dimension_list = (struct dimension *) dim_ptr;
    struct dimension *dimension_result = mallocz(sizeof(struct dimension));
    for (int i = 0; i < argc; i++) {
        if (i == 0) {
            uuid_parse(argv[i], ((DIMENSION *)dimension_result)->dim_uuid);
            strcpy(((DIMENSION *)dimension_result)->dim_str, argv[i]);
        }
        if (i == 1)
            ((DIMENSION *)dimension_result)->id = strdupz(argv[i]);
        if (i == 2)
            ((DIMENSION *)dimension_result)->name = strdupz(argv[i]);
    }
    info("STEL: [%s] [%s] [%s]", ((DIMENSION *)dimension_result)->dim_str, ((DIMENSION *)dimension_result)->id, ((DIMENSION *)dimension_result)->name);
    struct dimension **dimension_root  = (void *) dim_ptr;
    dimension_result->next = *dimension_root;
    *dimension_root = dimension_result;
    return 0;
}


int sql_select_dimension(uuid_t *chart_uuid, struct dimension **dimension_list)
{
    char *err_msg = NULL;
    char  sql[1024];
    char  chart_str[37];
    int rc;
    //DIMENSION *dim = callocz(1, sizeof(*dim));

    if (!db) {
        sql_init_database();
    }

    uuid_unparse_lower(*chart_uuid, chart_str);

    sprintf(sql, "select dim_uuid, id, name from dimension where chart_uuid = '%s' and archived = 1;", chart_str);
    //info("%s", sql);

    rc = sqlite3_exec(db, sql, dim_callback, dimension_list, &err_msg);
    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }
    //*id = dim->id;
    //*name = dim->name;
    //uuid_copy(*dim_uuid, dim->dim_uuid);
    return  0;
}
    // SPDX-License-Identifier: GPL-3.0-or-later

#include <sqlite3.h>
#include "sqlite_functions.h"

struct dimension_list *global_dimensions = NULL;

sqlite3 *db = NULL;
sqlite3 *dbmem = NULL;

static void _uuid_parse(sqlite3_context *context, int argc, sqlite3_value **argv)
{
    uuid_t uuid;

    if (argc != 1) {
        sqlite3_result_null(context);
        return;
    }
    int rc = uuid_parse((const char *) sqlite3_value_text(argv[0]), uuid);
    if (rc == -1) {
        sqlite3_result_null(context);
        return;
    }

    sqlite3_result_blob(context, &uuid, sizeof(uuid_t), SQLITE_TRANSIENT);
}

static void _uuid_unparse(sqlite3_context *context, int argc, sqlite3_value **argv)
{
    char uuid_str[37];
    if (argc != 1 || sqlite3_value_blob(argv[0]) == NULL) {
        sqlite3_result_null(context);
        return;
    }
    uuid_unparse_lower(sqlite3_value_blob(argv[0]), uuid_str);
    sqlite3_result_text(context, uuid_str, 36, SQLITE_TRANSIENT);
}

int dim_callback(void *dim_ptr, int argc, char **argv, char **azColName)
{
    UNUSED(azColName);

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
    //info("[%s] [%s] [%s]", ((DIMENSION *)dimension_result)->dim_str, ((DIMENSION *)dimension_result)->id,
     //   ((DIMENSION *)dimension_result)->name);
    struct dimension **dimension_root = (void *)dim_ptr;
    dimension_result->next = *dimension_root;
    *dimension_root = dimension_result;
    return 0;
}

/*
 * Initialize a database
 */

int sql_init_database()
{
    char *err_msg = NULL;

    int rc = sqlite3_open("/tmp/database", &db);
    info("SQLite Database initialized (rc = %d)", rc);

    char *sql = "PRAGMA synchronous=0 ; CREATE TABLE IF NOT EXISTS dimension(dim_uuid blob PRIMARY KEY, chart_uuid blob, id text, name text, multiplier int, divisor int , algorithm int, archived int, options text);";

    rc = sqlite3_exec(db, sql, 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
        sqlite3_close(db);
    }

    rc = sqlite3_exec(db, "create index if not exists ind_chart_uuid on dimension (chart_uuid);", 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    rc = sqlite3_exec(db, "ATTACH ':memory:' as ram;", 0, 0, &err_msg);
    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    rc = sqlite3_exec(db, "CREATE TABLE IF NOT EXISTS ram.chart_dim(chart_uuid blob, dim_uuid blob, id text, name text); create index ram.ind_dim_uuid on chart_dim (dim_uuid); create index if not exists ram.ind_chart_uuid on chart_dim (chart_uuid);", 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    rc = sqlite3_exec(db, "CREATE TABLE IF NOT EXISTS ram.chart_stat(chart_uuid blob, min_row int, max_row int); create index ram.ind_chart_uuid1 on chart_stat (chart_uuid);", 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    sqlite3_create_function(db, "u2h", 1, SQLITE_ANY | SQLITE_DETERMINISTIC , 0, _uuid_parse, 0, 0);
    sqlite3_create_function(db, "h2u", 1, SQLITE_ANY | SQLITE_DETERMINISTIC , 0, _uuid_unparse, 0, 0);

    return rc;
}

int sql_close_database()
{
    if (db)
        sqlite3_close(db);
    return 0;
}

void sql_compact_database()
{
    char *err_msg = NULL;

    int rc = sqlite3_exec(db, "VACUUM;", 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }
}

void sql_backup_database()
{
    char *err_msg = NULL;

    char sql[512];

    sprintf(sql,"VACUUM into '/tmp/database.%u'", time(NULL));

    int rc = sqlite3_exec(db, sql, 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }
}

int sql_store_dimension(uuid_t *dim_uuid, uuid_t *chart_uuid, const char *id, const char *name, collected_number multiplier,
                         collected_number divisor, int algorithm)
{
    char *err_msg = NULL;
    char  sql[1024];
    char  dim_str[37], chart_str[37];
    int rc;

    if (!db)
        return 1;

    // FIRST WAY TO DO IT
//    uuid_unparse_lower(*dim_uuid, dim_str);
//    uuid_unparse_lower(*chart_uuid, chart_str);
//
//    sprintf(sql, "INSERT OR REPLACE into dimension (dim_uuid, chart_uuid, id, name, multiplier, divisor , algorithm, archived) values (u2h('%s'),u2h('%s'),'%s','%s', %lld, %lld, %d, 1) ;",
//            dim_str, chart_str, id, name, multiplier, divisor, algorithm);
//
//    rc = sqlite3_exec(db, sql, 0, 0, &err_msg);
//    if (rc != SQLITE_OK) {
//        error("SQL error: %s", err_msg);
//        sqlite3_free(err_msg);
//    }
    // FIRST DONE

    // SECOND WAY TO DO IT
    sqlite3_stmt *res;
#define SQL_INSERT_DIMENSION "INSERT OR REPLACE into dimension (dim_uuid, chart_uuid, id, name, multiplier, divisor , algorithm, archived) values (?0001,?0002,?0003,?0004, ?0005, ?0006, ?0007, 1) ;"
    rc = sqlite3_prepare_v2(db, SQL_INSERT_DIMENSION, -1, &res, 0);
    if (rc != SQLITE_OK)
        return 1;

    int param = sqlite3_bind_parameter_index(res, "@dim");
    rc = sqlite3_bind_blob(res, 1, dim_uuid, 16, SQLITE_STATIC);
    rc = sqlite3_bind_blob(res, 2, chart_uuid, 16, SQLITE_STATIC);
    rc = sqlite3_bind_text(res, 3, id, -1, SQLITE_STATIC);
    rc = sqlite3_bind_text(res, 4, name, -1, SQLITE_STATIC);
    rc = sqlite3_bind_int(res, 5, multiplier);
    rc = sqlite3_bind_int(res, 6, divisor);
    rc = sqlite3_bind_int(res, 7, algorithm);
    // Omit checks

    rc = sqlite3_step(res);
    sqlite3_finalize(res);
    return (rc != SQLITE_ROW);
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

    sprintf(sql, "update dimension set archived = %d where dim_uuid = u2h('%s');", archive, dim_str);

    rc = sqlite3_exec(db, sql, 0, 0, &err_msg);
    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    return  0;
}

int sql_dimension_options(uuid_t *dim_uuid, char *options)
{
    char *err_msg = NULL;
    char sql[1024];
    char dim_str[37];
    int rc;

    if (!db)
        return 1;

    if (!(options && *options))
        return 1;

    uuid_unparse_lower(*dim_uuid, dim_str);

    sprintf(sql, "update dimension set options = '%s' where dim_uuid = u2h('%s');", options, dim_str);

    rc = sqlite3_exec(db, sql, 0, 0, &err_msg);
    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    return 0;
}

/*
 * This will load and initialize a dimension under a chart
 *
 */

#define SQL_SELECT_DIMENSION    "select id, name, multiplier, divisor , algorithm, options from dimension where dim_uuid = @dim and archived = 1;"

int sql_create_dimension(char *dim_str, RRDSET *st)
{
    char sql[1024];
    uuid_t  dim_uuid;
    sqlite3_stmt *res;
    int rc;

    if (!db)
        return 1;

    uuid_parse(dim_str, dim_uuid);

    //sprintf(sql, "select id, name, multiplier, divisor , algorithm, options from dimension where dim_uuid = u2h('%s') and archived = 1;", dim_str);
    rc = sqlite3_prepare_v2(db, SQL_SELECT_DIMENSION, -1, &res, 0);
    if (rc != SQLITE_OK)
        return 1;

    int param = sqlite3_bind_parameter_index(res, "@dim");

    rc = sqlite3_bind_blob(res, param, dim_uuid, 16, SQLITE_STATIC);
    if (rc != SQLITE_OK) // Release the RES
        return 1;

    rc = sqlite3_step(res);

    if (rc == SQLITE_ROW) {

        RRDDIM *rd = rrddim_add_custom(
            st, (const char *) sqlite3_column_text(res, 0), (const char *)  sqlite3_column_text(res, 1), sqlite3_column_int(res, 2),
            sqlite3_column_int(res, 3), sqlite3_column_int(res, 4), st->rrd_memory_mode, 0, &dim_uuid);

        rrddim_flag_clear(rd, RRDDIM_FLAG_HIDDEN);
        rrddim_flag_clear(rd, RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS);
        rrddim_isnot_obsolete(st, rd); /* archived dimensions cannot be obsolete */
        const char *option = (const char *) sqlite3_column_text(res, 5);
        if (option && *option) {
            if (strstr(option, "hidden") != NULL)
                rrddim_flag_set(rd, RRDDIM_FLAG_HIDDEN);
            if (strstr(option, "noreset") != NULL)
                rrddim_flag_set(rd, RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS);
            if (strstr(option, "nooverflow") != NULL)
                rrddim_flag_set(rd, RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS);
        }
    }

    sqlite3_finalize(res);

    return 0;
}

/*
 * Load a charts dimensions and create them under RRDSET
 */
int sql_load_chart_dimensions(RRDSET *st, char *dimensions)
{
    UNUSED(dimensions);

    if (!db)
        return 1;

    struct dimension *dimension_list = NULL, *tmp_dimension_list;
    sql_load_one_chart_dimension(st->chart_uuid, &dimension_list);

    // loop through all the dimensions and create under the chart
    while(dimension_list) {

        sql_create_dimension(dimension_list->dim_str, st);

        tmp_dimension_list = dimension_list->next;
        freez(dimension_list->id);
        freez(dimension_list->name);
        freez(dimension_list);
        dimension_list = tmp_dimension_list;
    }

    return 0;
}

int sql_load_one_chart_dimension(uuid_t *chart_uuid, struct dimension **dimension_list)
{
    char *err_msg = NULL;
    char sql[1024];
    char chart_str[37];
    int rc;

    if (!db)
        return 1;

    uuid_unparse_lower(*chart_uuid, chart_str);

    sprintf(sql, "select h2u(dim_uuid), id, name, chart_uuid from dimension where chart_uuid = u2h('%s') and archived = 1;", chart_str);

    rc = sqlite3_exec(db, sql, dim_callback, dimension_list, &err_msg);
    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }
    return 0;
}


/*
 * Load all chart dimensions and return an array
 */

//#define SQL_GET_DIMLIST "select h2u(dim_uuid), id, name, chart_uuid, rowid from ram.chart_dim where chart_uuid = @chart;"
#define SQL_GET_DIMLIST "select h2u(dim_uuid), id, name, chart_uuid, rowid from ram.chart_dim order by chart_uuid;"
int sql_select_dimension(uuid_t *chart_uuid, struct dimension_list **dimension_list, int *from, int *to)
{
    char *err_msg = NULL;
    char chart_str[37];
    int rc;
    sqlite3_stmt *res;

    if (!db)
        return 1;

    //uuid_unparse_lower(*chart_uuid, chart_str);

//    while (sqlite3_step(res) == SQLITE_ROW) {
//            info("Reading chart data (%s) --> [%s] [%s] [%s]", chart_str, sqlite3_column_text(res, 0), sqlite3_column_text(res, 1),
//                 sqlite3_column_text(res, 2));
//            rows++;
//    }
//
//    sqlite3_reset(res);
    if (!global_dimensions) {
        rc = sqlite3_prepare_v2(db, SQL_GET_DIMLIST, -1, &res, 0);
        if (rc != SQLITE_OK)
            return 1;

//        int param = sqlite3_bind_parameter_index(res, "@chart");
//
//        rc = sqlite3_bind_blob(res, param, chart_uuid, 16, SQLITE_STATIC);
//        if (rc != SQLITE_OK) { // Release the RES
//            info("Failed to bind");
//            return 1;
//        }

        int rows = 100000; // assume max of 100 dimensions

        info("Allocating dimensions");
        global_dimensions = callocz(rows + 1, sizeof(**dimension_list));

        int i = 0;
        while (sqlite3_step(res) == SQLITE_ROW) {
            uuid_parse(sqlite3_column_text(res, 0), global_dimensions[i].dim_uuid);
            strcpy(global_dimensions[i].dim_str, sqlite3_column_text(res, 0));
            //strcpy(global_dimensions[i].id, sqlite3_column_text(res, 1));
            //strcpy(global_dimensions[i].name, sqlite3_column_text(res, 2));
            global_dimensions[i].id = strdupz(sqlite3_column_text(res, 1));
            global_dimensions[i].name = strdupz(sqlite3_column_text(res, 2));
            i++;
        }

        info("Initialized dimensions %d", i);
        sqlite3_finalize(res);
    }

    if (from && to) {
        rc = sqlite3_prepare_v2(
            db, "select min_row, max_row from ram.chart_stat where chart_uuid = @chart;", -1, &res, 0);
        int param = sqlite3_bind_parameter_index(res, "@chart");
        rc = sqlite3_bind_blob(res, param, chart_uuid, 16, SQLITE_STATIC);
        if (rc != SQLITE_OK) {
            error("Failed to bind to get chart range");
        }
        while (sqlite3_step(res) == SQLITE_ROW) {
            *from = sqlite3_column_int(res, 0) - 1;
            *to = sqlite3_column_int(res, 1);
        }
        sqlite3_finalize(res);
    }
    *dimension_list = global_dimensions;

    return 0;
}

void sql_sync_ram_db()
{

    sqlite3_exec(db, "delete from ram.chart_dim; insert into ram.chart_dim select chart_uuid,dim_uuid,id, name from dimension order by chart_uuid;", 0, 0, NULL);

    sqlite3_exec(db, "delete from ram.chart_stat ; insert into ram.chart_stat select chart_uuid, min(rowid), max(rowid) from ram.chart_dim group by chart_uuid;", 0, 0, NULL);

}

void  sql_add_metric(uuid_t *dim_uuid, usec_t point_in_time, storage_number number)
{
    char *err_msg = NULL;
    char  sql[1024];
    char  dim_str[37];
    int rc;

    if (!dbmem) {
        int rc = sqlite3_open(":memory:", &dbmem);
        if (rc != SQLITE_OK) {
            error("SQL error: %s", err_msg);
            sqlite3_free(err_msg);
            return;
        }
        info("SQLite in memory initialized");

        char *sql = "PRAGMA synchronous=0 ; CREATE TABLE IF NOT EXISTS metric(dim_uuid text, date_created int, value int);";
        rc = sqlite3_exec(dbmem, sql, 0, 0, &err_msg);
        if (rc != SQLITE_OK) {
            error("SQL error: %s", err_msg);
            sqlite3_free(err_msg);
            return;
        }
    }

    uuid_unparse_lower(*dim_uuid, dim_str);

    sprintf(sql, "INSERT into metric (dim_uuid, date_created, value) values ('%s', %llu, %u);",
            dim_str, point_in_time, number);

    rc = sqlite3_exec(dbmem, sql, 0, 0, &err_msg);
    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }
}

void sql_add_metric_page(uuid_t *dim_uuid, struct rrdeng_page_descr *descr)
{
    char *err_msg = NULL;
    char  sql[1024];
    char  dim_str[37];
    int rc;
    BUFFER *wb;

    if (!descr->page_length)
        return;

    wb =buffer_create(300);

    uuid_unparse_lower(*dim_uuid, dim_str);
    int entries =  descr->page_length / sizeof(storage_number);
    int *metric = descr->pg_cache_descr->page;
    int dt = 0;
    time_t start_time = descr->start_time/ USEC_PER_SEC;
    if (entries > 1)
        dt = ((descr->end_time - descr->start_time) / USEC_PER_SEC) / (entries - 1);
    for (int i=0; i < entries; i++, start_time += dt) {
        snprintf(sql, 256, "insert into metric (dim_uuid, date_created, value) values ('%s', %ld, %d);", dim_str,
                 start_time, metric[i]);
        buffer_strcat(wb, sql);
    }
    info("SQL STORE: %s entries %lu from (%llu to %llu)", dim_str, descr->page_length / sizeof(storage_number), descr->start_time / USEC_PER_SEC, descr->end_time / USEC_PER_SEC);
    info("SQL DETAIL: %s", buffer_tostring(wb));
    rc = sqlite3_exec(db, buffer_tostring(wb), 0, 0, &err_msg);
    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }
    buffer_free(wb);
    return;
}
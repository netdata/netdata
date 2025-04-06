// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_context.h"
#include "sqlite_db_migration.h"
#include "database/contexts/internal.h"

#define DB_CONTEXT_METADATA_VERSION 1

const char *database_context_config[] = {
    "CREATE TABLE IF NOT EXISTS context (host_id BLOB, id TEXT NOT NULL, version INT NOT NULL, title TEXT NOT NULL, "
    "chart_type TEXT NOT NULL, unit TEXT NOT NULL, priority INT NOT NULL, first_time_t INT NOT NULL, "
    "last_time_t INT NOT NULL, deleted INT NOT NULL, "
    "family TEXT, PRIMARY KEY (host_id, id))",

    NULL
};

const char *database_context_cleanup[] = {
    "DROP TRIGGER IF EXISTS del_context1",
    "DROP TABLE IF EXISTS context_metadata_cleanup",
    "VACUUM",
    NULL
};

sqlite3 *db_context_meta = NULL;
/*
 * Initialize the SQLite database
 * Return 0 on success
 */
int sql_init_context_database(int memory)
{
    char sqlite_database[FILENAME_MAX + 1];
    int rc;

    if (likely(!memory))
        snprintfz(sqlite_database, sizeof(sqlite_database) - 1, "%s/context-meta.db", netdata_configured_cache_dir);
    else
        strcpy(sqlite_database, ":memory:");

    rc = sqlite3_open(sqlite_database, &db_context_meta);
    if (rc != SQLITE_OK) {
        error_report("Failed to initialize database at %s, due to \"%s\"", sqlite_database, sqlite3_errstr(rc));
        sqlite3_close(db_context_meta);
        db_context_meta = NULL;
        return 1;
    }

    errno_clear();
    netdata_log_info("SQLite database %s initialization", sqlite_database);

    char buf[1024 + 1] = "";
    const char *list[2] = { buf, NULL };

    int target_version = DB_CONTEXT_METADATA_VERSION;
    if (likely(!memory))
        target_version = perform_context_database_migration(db_context_meta, DB_CONTEXT_METADATA_VERSION);

    if (configure_sqlite_database(db_context_meta, target_version, "context_config"))
        return 1;

    if (likely(!memory))
        snprintfz(buf, sizeof(buf) - 1, "ATTACH DATABASE \"%s/netdata-meta.db\" as meta", netdata_configured_cache_dir);
    else
        snprintfz(buf, sizeof(buf) - 1, "ATTACH DATABASE ':memory:' as meta");

    if(init_database_batch(db_context_meta, list, "context")) return 1;

    if (init_database_batch(db_context_meta, &database_context_config[0], "context_init"))
        return 1;

    if (init_database_batch(db_context_meta, &database_context_cleanup[0], "context_cleanup"))
        return 1;

    return 0;
}

extern __thread sqlite3 *db_meta_thread;
extern __thread sqlite3 *db_context_thread;
//
// Fetching data
//
#define CTX_GET_CHART_LIST  "SELECT c.chart_id, c.type||'.'||c.id, c.name, c.context, c.title, c.unit, c.priority, " \
    "c.update_every, c.chart_type, c.family FROM chart c WHERE c.host_id = @host_id AND c.chart_id IS NOT NULL"

void ctx_get_chart_list(nd_uuid_t *host_uuid, void (*dict_cb)(SQL_CHART_DATA *, void *), void *data)
{
    sqlite3_stmt *res = NULL;

    if (unlikely(!host_uuid)) {
       internal_error(true, "Requesting context chart list without host_id");
       return;
    }

    if (!PREPARE_STATEMENT(db_meta_thread ? db_meta_thread : db_meta, CTX_GET_CHART_LIST, &res))
        return;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, host_uuid, sizeof(*host_uuid), SQLITE_STATIC));

    param = 0;
    SQL_CHART_DATA chart_data = { 0 };
    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        uuid_copy(chart_data.chart_id, *((nd_uuid_t *)sqlite3_column_blob(res, 0)));
        chart_data.id = (char *) sqlite3_column_text(res, 1);
        chart_data.name = (char *) sqlite3_column_text(res, 2);
        chart_data.context = (char *) sqlite3_column_text(res, 3);
        chart_data.title = (char *) sqlite3_column_text(res, 4);
        chart_data.units = (char *) sqlite3_column_text(res, 5);
        chart_data.priority = sqlite3_column_int(res, 6);
        chart_data.update_every = sqlite3_column_int(res, 7);
        chart_data.chart_type = sqlite3_column_int(res, 8);
        chart_data.family = (char *) sqlite3_column_text(res, 9);
        dict_cb(&chart_data, data);
    }

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
}

// Dimension list
#define CTX_GET_DIMENSION_LIST  "SELECT d.dim_id, d.id, d.name, CASE WHEN INSTR(d.options,\"hidden\") > 0 THEN 1 ELSE 0 END, c.type||'.'||c.id, c.context " \
    "FROM dimension d, chart c WHERE c.host_id = @host_id AND d.chart_id = c.chart_id AND d.dim_id IS NOT NULL ORDER BY d.rowid ASC"
void ctx_get_dimension_list(nd_uuid_t *host_uuid, void (*dict_cb)(SQL_DIMENSION_DATA *, void *), void *data)
{
    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_meta_thread ? db_meta_thread : db_meta, CTX_GET_DIMENSION_LIST, &res))
        return;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, host_uuid, sizeof(*host_uuid), SQLITE_STATIC));

    SQL_DIMENSION_DATA dimension_data;

    param = 0;
    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        uuid_copy(dimension_data.dim_id, *((nd_uuid_t *)sqlite3_column_blob(res, 0)));
        dimension_data.id = (char *) sqlite3_column_text(res, 1);
        dimension_data.name = (char *) sqlite3_column_text(res, 2);
        dimension_data.hidden = sqlite3_column_int(res, 3);
        dimension_data.chart_id = (char *) sqlite3_column_text(res, 4);
        dimension_data.context = (char *) sqlite3_column_text(res, 5);
        dict_cb(&dimension_data, data);
    }

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
}

// LABEL LIST
#define CTX_GET_LABEL_LIST  "SELECT l.label_key, l.label_value, l.source_type FROM meta.chart_label l WHERE l.chart_id = @id"

void ctx_get_label_list(nd_uuid_t *chart_uuid, void (*dict_cb)(SQL_CLABEL_DATA *, void *), void *data)
{
    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_context_meta, CTX_GET_LABEL_LIST, &res))
        return;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, chart_uuid, sizeof(*chart_uuid), SQLITE_STATIC));

    param = 0;
    SQL_CLABEL_DATA label_data;

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        label_data.label_key = (char *) sqlite3_column_text(res, 0);
        label_data.label_value = (char *) sqlite3_column_text(res, 1);
        label_data.label_source = sqlite3_column_int(res, 2);
        dict_cb(&label_data, data);
    }

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
}

// CONTEXT LIST
#define CTX_GET_CONTEXT_LIST  "SELECT id, version, title, chart_type, unit, priority, first_time_t, " \
    "last_time_t, deleted, family FROM context c WHERE c.host_id = @host_id"

void ctx_get_context_list(nd_uuid_t *host_uuid, void (*dict_cb)(VERSIONED_CONTEXT_DATA *, void *), void *data)
{
    if (unlikely(!host_uuid))
        return;

    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_context_thread ? db_context_thread : db_context_meta, CTX_GET_CONTEXT_LIST, &res))
        return;

    VERSIONED_CONTEXT_DATA context_data = {0};

    int param = 0;
    SQLITE_BIND_FAIL(done,  sqlite3_bind_blob(res, ++param, host_uuid, sizeof(*host_uuid), SQLITE_STATIC));
    param = 0;

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        context_data.id = (char *) sqlite3_column_text(res, 0);
        context_data.version = sqlite3_column_int64(res, 1);
        context_data.title = (char *) sqlite3_column_text(res, 2);
        context_data.chart_type = (char *) sqlite3_column_text(res, 3);
        context_data.units = (char *) sqlite3_column_text(res, 4);
        context_data.priority = sqlite3_column_int64(res, 5);
        context_data.first_time_s = sqlite3_column_int64(res, 6);
        context_data.last_time_s = sqlite3_column_int64(res, 7);
        context_data.deleted = sqlite3_column_int(res, 8);
        context_data.family = (char *) sqlite3_column_text(res, 9);
        dict_cb(&context_data, data);
    }

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
}


//
// Storing Data
//
#define CTX_STORE_CONTEXT                                                                                              \
    "INSERT OR REPLACE INTO context "                                                                                  \
    "(host_id, id, version, title, chart_type, unit, priority, first_time_t, last_time_t, deleted, family) "           \
    "VALUES (@host_id, @context, @version, @title, @chart_type, @unit, @priority, @first_t, @last_t, @delete, @family)"

int ctx_store_context(nd_uuid_t *host_uuid, VERSIONED_CONTEXT_DATA *context_data)
{
    int rc_stored = 1;
    sqlite3_stmt *res = NULL;

    if (unlikely(!host_uuid || !context_data || !context_data->id))
        return 0;

    if (!PREPARE_STATEMENT(db_context_meta ? db_context_meta : db_meta, CTX_STORE_CONTEXT, &res))
        return 1;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, host_uuid, sizeof(*host_uuid), SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, bind_text_null(res, ++param, context_data->id, 0));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(res, ++param, (time_t) context_data->version));
    SQLITE_BIND_FAIL(done, bind_text_null(res, ++param, context_data->title, 0));
    SQLITE_BIND_FAIL(done, bind_text_null(res, ++param, context_data->chart_type, 0));
    SQLITE_BIND_FAIL(done, bind_text_null(res, ++param, context_data->units, 0));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(res, ++param, (time_t) context_data->priority));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(res, ++param, (time_t) context_data->first_time_s));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(res, ++param, (time_t) context_data->last_time_s));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int(res, ++param, context_data->deleted));
    SQLITE_BIND_FAIL(done, bind_text_null(res, ++param, context_data->family, 1));

    param = 0;
    rc_stored = execute_insert(res);

    if (rc_stored != SQLITE_DONE)
        error_report("Failed store context details for context %s, rc = %d", context_data->id, rc_stored);

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
    return (rc_stored != SQLITE_DONE);
}

// Delete a context
#define CTX_DELETE_CONTEXT "DELETE FROM context WHERE host_id = @host_id AND id = @context"
int ctx_delete_context(nd_uuid_t *host_uuid, VERSIONED_CONTEXT_DATA *context_data)
{
    int rc_stored = 1;
    sqlite3_stmt *res = NULL;

    if (unlikely(!context_data || !context_data->id))
        return 0;

    if (!PREPARE_STATEMENT(db_context_meta, CTX_DELETE_CONTEXT, &res))
        return 1;

    metadata_queue_ctx_host_cleanup(host_uuid, context_data->id);

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, host_uuid, sizeof(*host_uuid), SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_text(res, ++param, context_data->id, -1, SQLITE_STATIC));

    param = 0;
    rc_stored = execute_insert(res);

    if (rc_stored != SQLITE_DONE)
        error_report("Failed to delete context %s, rc = %d", context_data->id, rc_stored);

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
    return (rc_stored != SQLITE_DONE);
}

int sql_context_cache_stats(int op)
{
    int count, dummy;

    if (unlikely(!db_context_meta))
        return 0;

    sqlite3_db_status(db_context_meta, op, &count, &dummy, 0);
    return count;
}


uint64_t sqlite_get_context_space(void)
{
    return sqlite_get_db_space(db_context_meta);
}

//
// TESTING FUNCTIONS
//

static void dict_ctx_get_context_list_cb(VERSIONED_CONTEXT_DATA *context_data, void *data)
{
    (void)data;
    netdata_log_info("   Context id = %s "
         "version = %"PRIu64" "
         "title = %s "
         "chart_type = %s "
         "units = %s "
         "priority = %"PRIu64" "
         "first time = %"PRIu64" "
         "last time = %"PRIu64" "
         "deleted = %d "
         "family = %s",
         context_data->id,
         context_data->version,
         context_data->title,
         context_data->chart_type,
         context_data->units,
         context_data->priority,
         context_data->first_time_s,
         context_data->last_time_s,
         context_data->deleted,
         context_data->family);
}

int ctx_unittest(void)
{
    nd_uuid_t host_uuid;
    uuid_generate(host_uuid);

    if (sqlite_library_init())
        return 1;

    int rc = sql_init_context_database(1);

    if (rc != SQLITE_OK)
        return 1;

    // Store a context
    VERSIONED_CONTEXT_DATA context_data;

    context_data.id = strdupz("cpu.cpu");
    context_data.title = strdupz("TestContextTitle");
    context_data.units= strdupz("TestContextUnits");
    context_data.chart_type = strdupz("TestContextChartType");
    context_data.family = strdupz("TestContextFamily");
    context_data.priority = 50000;
    context_data.deleted = 0;
    context_data.first_time_s = 1657781000;
    context_data.last_time_s  = 1657781100;
    context_data.version  = now_realtime_usec();

    if (likely(!ctx_store_context(&host_uuid, &context_data)))
        netdata_log_info("Entry %s inserted", context_data.id);
    else
        netdata_log_info("Entry %s not inserted", context_data.id);

    if (likely(!ctx_store_context(&host_uuid, &context_data)))
        netdata_log_info("Entry %s inserted", context_data.id);
    else
        netdata_log_info("Entry %s not inserted", context_data.id);

    // This will change end time
    context_data.first_time_s = 1657781000;
    context_data.last_time_s  = 1657782001;
    if (likely(!ctx_update_context(&host_uuid, &context_data)))
        netdata_log_info("Entry %s updated", context_data.id);
    else
        netdata_log_info("Entry %s not updated", context_data.id);
    netdata_log_info("List context start after insert");
    ctx_get_context_list(&host_uuid, dict_ctx_get_context_list_cb, NULL);
    netdata_log_info("List context end after insert");

    // This will change start time
    context_data.first_time_s = 1657782000;
    context_data.last_time_s  = 1657782001;
    if (likely(!ctx_update_context(&host_uuid, &context_data)))
        netdata_log_info("Entry %s updated", context_data.id);
    else
        netdata_log_info("Entry %s not updated", context_data.id);

    // This will list one entry
    netdata_log_info("List context start after insert");
    ctx_get_context_list(&host_uuid, dict_ctx_get_context_list_cb, NULL);
    netdata_log_info("List context end after insert");

    netdata_log_info("List context start after insert");
    ctx_get_context_list(&host_uuid, dict_ctx_get_context_list_cb, NULL);
    netdata_log_info("List context end after insert");

    // This will delete the entry
    if (likely(!ctx_delete_context(&host_uuid, &context_data)))
        netdata_log_info("Entry %s deleted", context_data.id);
    else
        netdata_log_info("Entry %s not deleted", context_data.id);

    freez((void *)context_data.id);
    freez((void *)context_data.title);
    freez((void *)context_data.chart_type);
    freez((void *)context_data.family);
    freez((void *)context_data.units);

    // The list should be empty
    netdata_log_info("List context start after delete");
    ctx_get_context_list(&host_uuid, dict_ctx_get_context_list_cb, NULL);
    netdata_log_info("List context end after delete");

    sql_close_database(db_context_meta, "CONTEXT");
    sqlite_library_shutdown();

    return 0;
}


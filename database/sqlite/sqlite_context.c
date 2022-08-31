// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_context.h"
#include "sqlite_db_migration.h"

#define DB_CONTEXT_METADATA_VERSION 1

const char *database_context_config[] = {
    "CREATE TABLE IF NOT EXISTS context (host_id BLOB, id TEXT NOT NULL, version INT NOT NULL, title TEXT NOT NULL, " \
    "chart_type TEXT NOT NULL, unit TEXT NOT NULL, priority INT NOT NULL, first_time_t INT NOT NULL, "
    "last_time_t INT NOT NULL, deleted INT NOT NULL, "
    "family TEXT, PRIMARY KEY (host_id, id));",

    NULL
};

const char *database_context_cleanup[] = {
    "VACUUM;",
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
        snprintfz(sqlite_database, FILENAME_MAX, "%s/context-meta.db", netdata_configured_cache_dir);
    else
        strcpy(sqlite_database, ":memory:");

    rc = sqlite3_open(sqlite_database, &db_context_meta);
    if (rc != SQLITE_OK) {
        error_report("Failed to initialize database at %s, due to \"%s\"", sqlite_database, sqlite3_errstr(rc));
        sqlite3_close(db_context_meta);
        db_context_meta = NULL;
        return 1;
    }

    info("SQLite database %s initialization", sqlite_database);

    char buf[1024 + 1] = "";
    const char *list[2] = { buf, NULL };

    int target_version = DB_CONTEXT_METADATA_VERSION;
    if (likely(!memory))
        target_version = perform_context_database_migration(db_context_meta, DB_CONTEXT_METADATA_VERSION);

    // https://www.sqlite.org/pragma.html#pragma_auto_vacuum
    // PRAGMA schema.auto_vacuum = 0 | NONE | 1 | FULL | 2 | INCREMENTAL;
    snprintfz(buf, 1024, "PRAGMA auto_vacuum=%s;", config_get(CONFIG_SECTION_SQLITE, "auto vacuum", "INCREMENTAL"));
    if(init_database_batch(db_context_meta, DB_CHECK_NONE, 0, list)) return 1;

    // https://www.sqlite.org/pragma.html#pragma_synchronous
    // PRAGMA schema.synchronous = 0 | OFF | 1 | NORMAL | 2 | FULL | 3 | EXTRA;
    snprintfz(buf, 1024, "PRAGMA synchronous=%s;", config_get(CONFIG_SECTION_SQLITE, "synchronous", "NORMAL"));
    if(init_database_batch(db_context_meta, DB_CHECK_NONE, 0, list))  return 1;

    // https://www.sqlite.org/pragma.html#pragma_journal_mode
    // PRAGMA schema.journal_mode = DELETE | TRUNCATE | PERSIST | MEMORY | WAL | OFF
    snprintfz(buf, 1024, "PRAGMA journal_mode=%s;", config_get(CONFIG_SECTION_SQLITE, "journal mode", "WAL"));
    if(init_database_batch(db_context_meta, DB_CHECK_NONE, 0, list)) return 1;

    // https://www.sqlite.org/pragma.html#pragma_temp_store
    // PRAGMA temp_store = 0 | DEFAULT | 1 | FILE | 2 | MEMORY;
    snprintfz(buf, 1024, "PRAGMA temp_store=%s;", config_get(CONFIG_SECTION_SQLITE, "temp store", "MEMORY"));
    if(init_database_batch(db_context_meta, DB_CHECK_NONE, 0, list)) return 1;
    
    // https://www.sqlite.org/pragma.html#pragma_journal_size_limit
    // PRAGMA schema.journal_size_limit = N ;
    snprintfz(buf, 1024, "PRAGMA journal_size_limit=%lld;", config_get_number(CONFIG_SECTION_SQLITE, "journal size limit", 16777216));
    if(init_database_batch(db_context_meta, DB_CHECK_NONE, 0, list)) return 1;

    // https://www.sqlite.org/pragma.html#pragma_cache_size
    // PRAGMA schema.cache_size = pages;
    // PRAGMA schema.cache_size = -kibibytes;
    snprintfz(buf, 1024, "PRAGMA cache_size=%lld;", config_get_number(CONFIG_SECTION_SQLITE, "cache size", -2000));
    if(init_database_batch(db_context_meta, DB_CHECK_NONE, 0, list)) return 1;

    snprintfz(buf, 1024, "PRAGMA user_version=%d;", target_version);
    if(init_database_batch(db_context_meta, DB_CHECK_NONE, 0, list)) return 1;

    if (likely(!memory))
        snprintfz(buf, 1024, "ATTACH DATABASE \"%s/netdata-meta.db\" as meta;", netdata_configured_cache_dir);
    else
        snprintfz(buf, 1024, "ATTACH DATABASE ':memory:' as meta;");

    if(init_database_batch(db_context_meta, DB_CHECK_NONE, 0, list)) return 1;

    if (init_database_batch(db_context_meta, DB_CHECK_NONE, 0, &database_context_config[0]))
        return 1;

    if (init_database_batch(db_context_meta, DB_CHECK_NONE, 0, &database_context_cleanup[0]))
        return 1;

    return 0;
}

/*
 * Close the sqlite database
 */

void sql_close_context_database(void)
{
    int rc;
    if (unlikely(!db_context_meta))
        return;

    info("Closing context SQLite database");

    rc = sqlite3_close_v2(db_context_meta);
    if (unlikely(rc != SQLITE_OK))
        error_report("Error %d while closing the context SQLite database, %s", rc, sqlite3_errstr(rc));
    return;
}

//
// Fetching data
//
#define CTX_GET_CHART_LIST  "SELECT c.chart_id, c.type||'.'||c.id, c.name, c.context, c.title, c.unit, c.priority, " \
        "c.update_every, c.chart_type, c.family FROM meta.chart c WHERE c.host_id = @host_id; "

void ctx_get_chart_list(uuid_t *host_uuid, void (*dict_cb)(SQL_CHART_DATA *, void *), void *data)
{
    int rc;
    sqlite3_stmt *res = NULL;

    if (unlikely(!host_uuid)) {
       internal_error(true, "Requesting context chart list without host_id");
       return;
    }

    rc = sqlite3_prepare_v2(db_context_meta, CTX_GET_CHART_LIST, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to fetch chart list");
        return;
    }

    rc = sqlite3_bind_blob(res, 1, host_uuid, sizeof(*host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id to fetch the chart list");
        goto skip_load;
    }

    SQL_CHART_DATA chart_data = { 0 };
    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        uuid_copy(chart_data.chart_id, *((uuid_t *)sqlite3_column_blob(res, 0)));
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

skip_load:
    rc = sqlite3_finalize(res);
    if (rc != SQLITE_OK)
        error_report("Failed to finalize statement that fetches chart label data, rc = %d", rc);
}

// Dimension list
#define CTX_GET_DIMENSION_LIST  "SELECT d.dim_id, d.id, d.name FROM meta.dimension d WHERE d.chart_id = @id;"
void ctx_get_dimension_list(uuid_t *chart_uuid, void (*dict_cb)(SQL_DIMENSION_DATA *, void *), void *data)
{
    int rc;
    sqlite3_stmt *res = NULL;

    rc = sqlite3_prepare_v2(db_context_meta, CTX_GET_DIMENSION_LIST, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to fetch chart dimension data");
        return;
    }

    rc = sqlite3_bind_blob(res, 1, chart_uuid, sizeof(*chart_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind chart_id to fetch dimension list");
        goto failed;
    }

    SQL_DIMENSION_DATA dimension_data;

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        uuid_copy(dimension_data.dim_id, *((uuid_t *)sqlite3_column_blob(res, 0)));
        dimension_data.id = (char *) sqlite3_column_text(res, 1);
        dimension_data.name = (char *) sqlite3_column_text(res, 2);
        dict_cb(&dimension_data, data);
    }

failed:
    rc = sqlite3_finalize(res);
    if (rc != SQLITE_OK)
        error_report("Failed to finalize statement that fetches the chart dimension list, rc = %d", rc);
}

// LABEL LIST
#define CTX_GET_LABEL_LIST  "SELECT l.label_key, l.label_value, l.source_type FROM meta.chart_label l WHERE l.chart_id = @id;"
void ctx_get_label_list(uuid_t *chart_uuid, void (*dict_cb)(SQL_CLABEL_DATA *, void *), void *data)
{
    int rc;
    sqlite3_stmt *res = NULL;

    rc = sqlite3_prepare_v2(db_context_meta, CTX_GET_LABEL_LIST, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to fetch chart lanbels");
        return;
    }

    rc = sqlite3_bind_blob(res, 1, chart_uuid, sizeof(*chart_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind chart_id to fetch chart labels");
        goto failed;
    }

    SQL_CLABEL_DATA label_data;

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        label_data.label_key = (char *) sqlite3_column_text(res, 0);
        label_data.label_value = (char *) sqlite3_column_text(res, 1);
        label_data.label_source = sqlite3_column_int(res, 2);
        dict_cb(&label_data, data);
    }

failed:
    rc = sqlite3_finalize(res);
    if (rc != SQLITE_OK)
        error_report("Failed to finalize statement that fetches chart label data, rc = %d", rc);

    return;
}

// CONTEXT LIST
#define CTX_GET_CONTEXT_LIST  "SELECT id, version, title, chart_type, unit, priority, first_time_t, " \
            "last_time_t, deleted, family FROM context c WHERE c.host_id = @host_id;"
void ctx_get_context_list(uuid_t *host_uuid, void (*dict_cb)(VERSIONED_CONTEXT_DATA *, void *), void *data)
{

    if (unlikely(!host_uuid))
        return;

    int rc;
    sqlite3_stmt *res = NULL;

    rc = sqlite3_prepare_v2(db_context_meta, CTX_GET_CONTEXT_LIST, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to fetch stored context list");
        return;
    }

    VERSIONED_CONTEXT_DATA context_data = {0};

    rc = sqlite3_bind_blob(res, 1, host_uuid, sizeof(*host_uuid), SQLITE_STATIC);

    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id to fetch versioned context data");
        goto failed;
    }

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        context_data.id = (char *) sqlite3_column_text(res, 0);
        context_data.version = sqlite3_column_int64(res, 1);
        context_data.title = (char *) sqlite3_column_text(res, 2);
        context_data.chart_type = (char *) sqlite3_column_text(res, 3);
        context_data.units = (char *) sqlite3_column_text(res, 4);
        context_data.priority = sqlite3_column_int64(res, 5);
        context_data.first_time_t = sqlite3_column_int64(res, 6);
        context_data.last_time_t = sqlite3_column_int64(res, 7);
        context_data.deleted = sqlite3_column_int(res, 8);
        context_data.family = (char *) sqlite3_column_text(res, 9);
        dict_cb(&context_data, data);
    }

failed:
    rc = sqlite3_finalize(res);
    if (rc != SQLITE_OK)
        error_report("Failed to finalize statement that fetches stored context versioned data, rc = %d", rc);
}


//
// Storing Data
//
#define CTX_STORE_CONTEXT "INSERT OR REPLACE INTO context " \
    "(host_id, id, version, title, chart_type, unit, priority, first_time_t, last_time_t, deleted, family) " \
    "VALUES (@host_id, @context, @version, @title, @chart_type, @unit, @priority, @first_time_t, @last_time_t, @deleted, @family);"

int ctx_store_context(uuid_t *host_uuid, VERSIONED_CONTEXT_DATA *context_data)
{
    int rc, rc_stored = 1;
    sqlite3_stmt *res = NULL;

    if (unlikely(!host_uuid || !context_data || !context_data->id))
        return 0;

    rc = sqlite3_prepare_v2(db_context_meta, CTX_STORE_CONTEXT, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to store context");
        return 1;
    }

    rc = sqlite3_bind_blob(res, 1, host_uuid, sizeof(*host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_uuid to store context details");
        goto skip_store;
    }

    rc = bind_text_null(res, 2, context_data->id, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind context to store details");
        goto skip_store;
    }

    rc = sqlite3_bind_int64(res, 3, (time_t) context_data->version);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind first_time_t to store context details");
        goto skip_store;
    }

    rc = bind_text_null(res, 4, context_data->title, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind context to store details");
        goto skip_store;
    }

    rc = bind_text_null(res, 5, context_data->chart_type, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind context to store details");
        goto skip_store;
    }

    rc = bind_text_null(res, 6, context_data->units, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind context to store details");
        goto skip_store;
    }

    rc = sqlite3_bind_int64(res, 7, (time_t) context_data->priority);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind first_time_t to store context details");
        goto skip_store;
    }

    rc = sqlite3_bind_int64(res, 8, (time_t) context_data->first_time_t);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind first_time_t to store context details");
        goto skip_store;
    }

    rc = sqlite3_bind_int64(res, 9, (time_t) context_data->last_time_t);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind last_time_t to store context details");
        goto skip_store;
    }

    rc = sqlite3_bind_int(res, 10, (time_t) context_data->deleted);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind last_time_t to store context details");
        goto skip_store;
    }

    rc = bind_text_null(res, 11, context_data->family, 1);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind context to store details");
        goto skip_store;
    }

    rc_stored = execute_insert(res);

    if (rc_stored != SQLITE_DONE)
        error_report("Failed store context details for context %s, rc = %d", context_data->id, rc_stored);

skip_store:
    rc = sqlite3_finalize(res);
    if (rc != SQLITE_OK)
        error_report("Failed to finalize statement that stores context details, rc = %d", rc);

    return (rc_stored != SQLITE_DONE);
}

// Delete a context

#define CTX_DELETE_CONTEXT "DELETE FROM context WHERE host_id = @host_id AND id = @context;"
int ctx_delete_context(uuid_t *host_uuid, VERSIONED_CONTEXT_DATA *context_data)
{
    int rc, rc_stored = 1;
    sqlite3_stmt *res = NULL;

    if (unlikely(!context_data || !context_data->id))
        return 0;

    rc = sqlite3_prepare_v2(db_context_meta, CTX_DELETE_CONTEXT, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to delete context");
        return 1;
    }

	rc = sqlite3_bind_blob(res, 1, host_uuid, sizeof(*host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id to delete context data");
        goto skip_delete;
    }

    rc = sqlite3_bind_text(res, 2, context_data->id, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind context id for data deletion");
        goto skip_delete;
    }

    rc_stored = execute_insert(res);

    if (rc_stored != SQLITE_DONE)
        error_report("Failed to delete context %s, rc = %d", context_data->id, rc_stored);
#ifdef NETDATA_INTERNAL_CHECKS
     else {
         char host_uuid_str[UUID_STR_LEN];
         uuid_unparse_lower(*host_uuid, host_uuid_str);
         info("%s: Deleted context %s under host %s", __FUNCTION__ , context_data->id, host_uuid_str);
     }
#endif

skip_delete:
    rc = sqlite3_finalize(res);
    if (rc != SQLITE_OK)
        error_report("Failed to finalize statement where deleting a context, rc = %d", rc);

    return (rc_stored != SQLITE_DONE);
}

//
// TESTING FUNCTIONS
//

static void dict_ctx_get_context_list_cb(VERSIONED_CONTEXT_DATA *context_data, void *data)
{
    (void)data;
    info("   Context id = %s "
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
         context_data->first_time_t,
         context_data->last_time_t,
         context_data->deleted,
         context_data->family);
}

int ctx_unittest(void)
{
    uuid_t host_uuid;
    uuid_generate(host_uuid);

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
    context_data.first_time_t = 1657781000;
    context_data.last_time_t  = 1657781100;
    context_data.version  = now_realtime_usec();

    if (likely(!ctx_store_context(&host_uuid, &context_data)))
        info("Entry %s inserted", context_data.id);
    else
        info("Entry %s not inserted", context_data.id);

    if (likely(!ctx_store_context(&host_uuid, &context_data)))
        info("Entry %s inserted", context_data.id);
    else
        info("Entry %s not inserted", context_data.id);

    // This will change end time
    context_data.first_time_t = 1657781000;
    context_data.last_time_t  = 1657782001;
    if (likely(!ctx_update_context(&host_uuid, &context_data)))
        info("Entry %s updated", context_data.id);
    else
        info("Entry %s not updated", context_data.id);
    info("List context start after insert");
    ctx_get_context_list(&host_uuid, dict_ctx_get_context_list_cb, NULL);
    info("List context end after insert");

    // This will change start time
    context_data.first_time_t = 1657782000;
    context_data.last_time_t  = 1657782001;
    if (likely(!ctx_update_context(&host_uuid, &context_data)))
        info("Entry %s updated", context_data.id);
    else
        info("Entry %s not updated", context_data.id);

    // This will list one entry
    info("List context start after insert");
    ctx_get_context_list(&host_uuid, dict_ctx_get_context_list_cb, NULL);
    info("List context end after insert");

    info("List context start after insert");
    ctx_get_context_list(&host_uuid, dict_ctx_get_context_list_cb, NULL);
    info("List context end after insert");

    // This will delete the entry
    if (likely(!ctx_delete_context(&host_uuid, &context_data)))
        info("Entry %s deleted", context_data.id);
    else
        info("Entry %s not deleted", context_data.id);

    freez((void *)context_data.id);
    freez((void *)context_data.title);
    freez((void *)context_data.chart_type);
    freez((void *)context_data.family);

    // The list should be empty
    info("List context start after delete");
    ctx_get_context_list(&host_uuid, dict_ctx_get_context_list_cb, NULL);
    info("List context end after delete");

    sql_close_context_database();

    return 0;
}


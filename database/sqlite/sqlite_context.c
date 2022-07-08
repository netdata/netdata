// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_context.h"

#define DB_CONTEXT_METADATA_VERSION 1

const char *database_context_config[] = {
    "CREATE TABLE IF NOT EXISTS context (context_name text PRIMARY KEY, first_time_t integer, last_time_t integer)",
    NULL
};

const char *database_context_cleanup[] = {

    NULL
};

sqlite3 *db_context_meta = NULL;

int init_context_database_batch(const char *batch[])
{
    int rc;
    char *err_msg = NULL;
    for (int i = 0; batch[i]; i++) {
        debug(D_METADATALOG, "Executing %s", batch[i]);
        rc = sqlite3_exec(db_context_meta, batch[i], 0, 0, &err_msg);
        if (rc != SQLITE_OK) {
            error_report("SQLite error during database setup, rc = %d (%s)", rc, err_msg);
            error_report("SQLite failed statement %s", batch[i]);
            sqlite3_free(err_msg);
            if (SQLITE_CORRUPT == rc)
                error_report("Databse integrity errors reported");
            return 1;
        }
    }
    return 0;
}

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


    // TODO: Cleanup this

    int target_version = DB_CONTEXT_METADATA_VERSION;

    // https://www.sqlite.org/pragma.html#pragma_auto_vacuum
    // PRAGMA schema.auto_vacuum = 0 | NONE | 1 | FULL | 2 | INCREMENTAL;
    snprintfz(buf, 1024, "PRAGMA auto_vacuum=%s;", config_get(CONFIG_SECTION_SQLITE, "auto vacuum", "INCREMENTAL"));
    if(init_context_database_batch(list)) return 1;

    // https://www.sqlite.org/pragma.html#pragma_synchronous
    // PRAGMA schema.synchronous = 0 | OFF | 1 | NORMAL | 2 | FULL | 3 | EXTRA;
    snprintfz(buf, 1024, "PRAGMA synchronous=%s;", config_get(CONFIG_SECTION_SQLITE, "synchronous", "NORMAL"));
    if(init_context_database_batch(list))  return 1;

    // https://www.sqlite.org/pragma.html#pragma_journal_mode
    // PRAGMA schema.journal_mode = DELETE | TRUNCATE | PERSIST | MEMORY | WAL | OFF
    snprintfz(buf, 1024, "PRAGMA journal_mode=%s;", config_get(CONFIG_SECTION_SQLITE, "journal mode", "WAL"));
    if(init_context_database_batch(list)) return 1;

    // https://www.sqlite.org/pragma.html#pragma_temp_store
    // PRAGMA temp_store = 0 | DEFAULT | 1 | FILE | 2 | MEMORY;
    snprintfz(buf, 1024, "PRAGMA temp_store=%s;", config_get(CONFIG_SECTION_SQLITE, "temp store", "MEMORY"));
    if(init_context_database_batch(list)) return 1;
    
    // https://www.sqlite.org/pragma.html#pragma_journal_size_limit
    // PRAGMA schema.journal_size_limit = N ;
    snprintfz(buf, 1024, "PRAGMA journal_size_limit=%lld;", config_get_number(CONFIG_SECTION_SQLITE, "journal size limit", 16777216));
    if(init_context_database_batch(list)) return 1;

    // https://www.sqlite.org/pragma.html#pragma_cache_size
    // PRAGMA schema.cache_size = pages;
    // PRAGMA schema.cache_size = -kibibytes;
    snprintfz(buf, 1024, "PRAGMA cache_size=%lld;", config_get_number(CONFIG_SECTION_SQLITE, "cache size", -2000));
    if(init_context_database_batch(list)) return 1;

    snprintfz(buf, 1024, "PRAGMA user_version=%d;", target_version);
    if(init_context_database_batch(list)) return 1;

    snprintfz(buf, 1024, "ATTACH DATABASE \"%s/netdata-meta.db\" as meta;", netdata_configured_cache_dir);
    if(init_context_database_batch(list)) return 1;

    if (init_context_database_batch(&database_context_config[0]))
        return 1;

    if (init_context_database_batch(&database_context_cleanup[0]))
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

    info("Closing SQLite database");

    rc = sqlite3_close_v2(db_context_meta);
    if (unlikely(rc != SQLITE_OK))
        error_report("Error %d while closing the SQLite database, %s", rc, sqlite3_errstr(rc));
    return;
}

//
// Fetching data
//
#define CTX_GET_CHART_LIST  "SELECT c.chart_id, c.type||'.'||c.id, c.context, c.update_every FROM meta.chart c " \
        "WHERE c.host_id IN (SELECT h.host_id FROM meta.host h " \
        "WHERE UNLIKELY((h.hops = 0 AND @host_id IS NULL)) OR LIKELY((h.host_id = @host_id)));"

void ctx_get_chart_list(uuid_t *host_id, void (*dict_cb)(void *))
{
    int rc;
    sqlite3_stmt *res = NULL;


    rc = sqlite3_prepare_v2(db_context_meta, CTX_GET_CHART_LIST, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to fetch chart lanbels");
        return;
    }

    if (likely(host_id))
        rc = sqlite3_bind_blob(res, 1, *host_id, sizeof(*host_id), SQLITE_STATIC);
    else
        rc = sqlite3_bind_null(res, 1);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id to fetch the chart list");
        goto failed;
    }

    ctx_chart_t chart_data;
    memset(&chart_data, 0, sizeof(chart_data));

    while (sqlite3_step(res) == SQLITE_ROW) {
        uuid_copy(chart_data.chart_id, *((uuid_t *)sqlite3_column_blob(res, 0)));
        chart_data.id = (char *) sqlite3_column_text(res, 1);
        chart_data.context = (char *) sqlite3_column_text(res, 2);
        chart_data.update_every = sqlite3_column_int(res, 3);
        dict_cb(&chart_data);
    }

failed:
    rc = sqlite3_finalize(res);
    if (rc != SQLITE_OK)
        error_report("Failed to finalize statement that fetches chart label data, rc = %d", rc);

    return;
}

// Dimension list
#define CTX_GET_DIMENSION_LIST  "SELECT d.dim_id, d.id FROM meta.dimension d where d.chart_id = @id;"
void ctx_get_dimension_list(uuid_t *chart_id, void (*dict_cb)(void *))
{
    int rc;
    sqlite3_stmt *res = NULL;

    rc = sqlite3_prepare_v2(db_context_meta, CTX_GET_DIMENSION_LIST, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to fetch chart dimension data");
        return;
    }

    rc = sqlite3_bind_blob(res, 1, *chart_id, sizeof(*chart_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind chart_id to fetch dimension list");
        goto failed;
    }

    ctx_dimension_t dimension_data;

    while (sqlite3_step(res) == SQLITE_ROW) {
        uuid_copy(dimension_data.dim_id, *((uuid_t *)sqlite3_column_blob(res, 0)));
        dimension_data.id = (char *) sqlite3_column_text(res, 1);
        dict_cb(&dimension_data);
    }

failed:
    rc = sqlite3_finalize(res);
    if (rc != SQLITE_OK)
        error_report("Failed to finalize statement that fetches the chart dimension list, rc = %d", rc);

    return;
}

// LABEL LIST
#define CTX_GET_LABEL_LIST  "SELECT l.label_key, l.label_value, l.source_type FROM meta.chart_label l where l.chart_id = @id;"
void ctx_get_label_list(uuid_t *chart_id, void (*dict_cb)(void *))
{
    int rc;
    sqlite3_stmt *res = NULL;

    rc = sqlite3_prepare_v2(db_context_meta, CTX_GET_LABEL_LIST, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to fetch chart lanbels");
        return;
    }

    rc = sqlite3_bind_blob(res, 1, *chart_id, sizeof(*chart_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind chart_id to fetch chart labels");
        goto failed;
    }

    ctx_label_t label_data;

    while (sqlite3_step(res) == SQLITE_ROW) {
        label_data.label_key = (char *) sqlite3_column_text(res, 0);
        label_data.label_value = (char *) sqlite3_column_text(res, 1);
        label_data.label_source = sqlite3_column_int(res, 2);
        dict_cb(&label_data);
    }

failed:
    rc = sqlite3_finalize(res);
    if (rc != SQLITE_OK)
        error_report("Failed to finalize statement that fetches chart label data, rc = %d", rc);

    return;
}

// CONTEXT LIST
#define CTX_GET_CONTEXT_LIST  "SELECT context_name, first_time_t, last_time_t FROM context;"
void ctx_get_context_list(void (*dict_cb)(void *))
{
    int rc;
    sqlite3_stmt *res = NULL;

    rc = sqlite3_prepare_v2(db_context_meta, CTX_GET_CONTEXT_LIST, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to fetch stored context list");
        return;
    }

    ctx_context_t context_data;

    while (sqlite3_step(res) == SQLITE_ROW) {
        context_data.context = (char *) sqlite3_column_text(res, 0);
        context_data.first_time_t = (time_t) sqlite3_column_int64(res, 1);
        context_data.last_time_t = (time_t) sqlite3_column_int64(res, 2);
        dict_cb(&context_data);
    }

    rc = sqlite3_finalize(res);
    if (rc != SQLITE_OK)
        error_report("Failed to finalize statement that fetches stored context list, rc = %d", rc);

    return;
}


//
// Storing Data
//
#define CTX_STORE_CONTEXT "INSERT OR REPLACE INTO context (context_name, first_time_t, last_time_t) VALUES (@context, @first_time_t, @last_time_t);"
int ctx_store_context(ctx_context_t *context_data)
{
    int rc, rc_stored = 1;
    sqlite3_stmt *res = NULL;

    if (unlikely(!context_data || !context_data->context))
        return 0;

    rc = sqlite3_prepare_v2(db_context_meta, CTX_STORE_CONTEXT, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to store context");
        return 1;
    }

    rc = sqlite3_bind_text(res, 1, context_data->context, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind context to store details");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 2, (time_t) context_data->first_time_t);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind first_time_t to store context details");
        goto failed;
    }

    rc = sqlite3_bind_int64(res, 3, (time_t) context_data->last_time_t);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind last_time_t to store context details");
        goto failed;
    }

    rc_stored = execute_insert(res);

    if (rc_stored != SQLITE_DONE)
        error_report("Failed store context details for context %s, rc = %d", context_data->context, rc_stored);

failed:
    rc = sqlite3_finalize(res);
    if (rc != SQLITE_OK)
        error_report("Failed to finalize statement that stores context details, rc = %d", rc);

    return (rc_stored != SQLITE_DONE);
}

// Delete a context

#define CTX_DELETE_CONTEXT "DELETE FROM context WHERE context_name = @context;"
int ctx_delete_context(ctx_context_t *context_data)
{
    int rc, rc_stored = 1;
    sqlite3_stmt *res = NULL;

    if (unlikely(!context_data || !context_data->context))
        return 0;

    rc = sqlite3_prepare_v2(db_context_meta, CTX_DELETE_CONTEXT, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to delete context");
        return 1;
    }

    rc = sqlite3_bind_text(res, 1, context_data->context, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind context for data deletion");
        goto failed;
    }

    rc_stored = execute_insert(res);

    if (rc_stored != SQLITE_DONE)
        error_report("Failed to delete context %s, rc = %d", context_data->context, rc_stored);

failed:
    rc = sqlite3_finalize(res);
    if (rc != SQLITE_OK)
        error_report("Failed to finalize statement where deleting a context, rc = %d", rc);

    return (rc_stored != SQLITE_DONE);
}

//
// TESTING FUNCTIONS
//
static void dict_ctx_get_label_list_cb(void *data)
{
    ctx_label_t *label_data = data;

    info(" LABEL %d %s = %s", label_data->label_source, label_data->label_key, label_data->label_value);
}

static void dict_ctx_get_dimension_list_cb(void *data)
{
    ctx_dimension_t *dimension_data = data;

    char uuid_str[GUID_LEN + 1];
    uuid_unparse_lower(dimension_data->dim_id, uuid_str);

    info(" Dimension %s = %s", uuid_str, dimension_data->id);
}


static void dict_ctx_get_chart_list_cb(void *data)
{
    ctx_chart_t *chart_data = data;

    char uuid_str[GUID_LEN + 1];
    uuid_unparse_lower(chart_data->chart_id, uuid_str);
    info("OK GOT %s ID = %s, context = %s", uuid_str, chart_data->id, chart_data->context);
    ctx_get_label_list(&chart_data->chart_id, dict_ctx_get_label_list_cb);
    ctx_get_dimension_list(&chart_data->chart_id, dict_ctx_get_dimension_list_cb);
}

static void dict_ctx_get_context_list_cb(void *data)
{
    ctx_context_t *context_data = data;

    info("   Context %s from %ld to %ld", context_data->context, context_data->first_time_t, context_data->last_time_t);
}


int ctx_unittest(void)
{
    sql_init_context_database(1);
    ctx_get_chart_list(NULL, dict_ctx_get_chart_list_cb);

    // Store a context
    ctx_context_t context_data;

    context_data.context = strdupz("TestContext1");
    context_data.first_time_t = 1000;
    context_data.last_time_t  = 1001;
    if (likely(!ctx_store_context(&context_data)))
        info("Entry %s inserted", context_data.context);
    else
        info("Entry %s not inserted", context_data.context);

    // This will change end time
    context_data.first_time_t = 1000;
    context_data.last_time_t  = 2001;
    if (likely(!ctx_update_context(&context_data)))
        info("Entry %s updated", context_data.context);
    else
        info("Entry %s not updated", context_data.context);

    // This will change start time
    context_data.first_time_t = 2000;
    context_data.last_time_t  = 2001;
    if (likely(!ctx_update_context(&context_data)))
        info("Entry %s updated", context_data.context);
    else
        info("Entry %s not updated", context_data.context);

    // This will list one entry
    info("List context start after insert");
    ctx_get_context_list(dict_ctx_get_context_list_cb);
    info("List context end after insert");

    // This will delete the entry
    if (likely(!ctx_delete_context(&context_data)))
        info("Entry %s deleted", context_data.context);
    else
        info("Entry %s not deleted", context_data.context);

    freez(context_data.context);

    // The list should be empty
    info("List context start after delete");
    ctx_get_context_list(dict_ctx_get_context_list_cb);
    info("List context end after delete");

    sql_close_context_database();

    return 0;
}

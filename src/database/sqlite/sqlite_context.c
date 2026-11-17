// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_context.h"
#include "sqlite_db_migration.h"
#include "database/contexts/rrdcontext-internal.h"

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

    char host_guid[UUID_STR_LEN];
    uuid_unparse_lower(*host_uuid, host_guid);

    param = 0;
    SQL_CHART_DATA chart_data = { 0 };
    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        if (unlikely(exit_initiated_get()))
            break;

        if (unlikely(!sqlite3_column_uuid_copy(res, 0, chart_data.chart_id))) {
            error_report("CTX [%s]: Got invalid chart id in column 0. Ignoring it.", host_guid);
            continue;
        }

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
#define CTX_GET_DIMENSION_LIST  "SELECT d.dim_id, d.id, d.name, CASE WHEN INSTR(d.options,\"hidden\") > 0 THEN 1 ELSE 0 END, c.type||'.'||c.id, c.context, d.algorithm " \
    "FROM dimension d, chart c WHERE c.host_id = @host_id AND d.chart_id = c.chart_id AND d.dim_id IS NOT NULL ORDER BY d.rowid ASC"
void ctx_get_dimension_list(nd_uuid_t *host_uuid, void (*dict_cb)(SQL_DIMENSION_DATA *, void *), void *data)
{
    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_meta_thread ? db_meta_thread : db_meta, CTX_GET_DIMENSION_LIST, &res))
        return;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, host_uuid, sizeof(*host_uuid), SQLITE_STATIC));

    char host_guid[UUID_STR_LEN];
    uuid_unparse_lower(*host_uuid, host_guid);

    SQL_DIMENSION_DATA dimension_data;

    param = 0;
    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        if (unlikely(exit_initiated_get()))
            break;

        if (unlikely(!sqlite3_column_uuid_copy(res, 0, dimension_data.dim_id))) {
            error_report("CTX [%s]: Got invalid dimension id in column 0. Ignoring it.", host_guid);
            continue;
        }

        dimension_data.id = (char *) sqlite3_column_text(res, 1);
        dimension_data.name = (char *) sqlite3_column_text(res, 2);
        dimension_data.hidden = sqlite3_column_int(res, 3);
        dimension_data.chart_id = (char *) sqlite3_column_text(res, 4);
        dimension_data.context = (char *) sqlite3_column_text(res, 5);
        dimension_data.algorithm = sqlite3_column_int(res, 6);
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
        if (unlikely(exit_initiated_get()))
            break;

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
static int ctx_delete_context_row(nd_uuid_t *host_uuid, VERSIONED_CONTEXT_DATA *context_data)
{
    int rc_stored = 1;
    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_context_meta, CTX_DELETE_CONTEXT, &res))
        return 1;

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

int ctx_delete_context(nd_uuid_t *host_uuid, VERSIONED_CONTEXT_DATA *context_data)
{
    if (unlikely(!context_data || !context_data->id))
        return 0;

    metadata_queue_ctx_host_cleanup(host_uuid, context_data->id);
    return ctx_delete_context_row(host_uuid, context_data);
}

// ----------------------------------------------------------------------------
// Context write queue
//
// rrdcontext decides context writes while it holds rrdcontext_lock(rc), where SQLite must not run: a slow or stuck
// database would keep the context locked and, through rrdcontext_conflict_callback(), stall every collector that
// sets a context of the host. Writes are queued with copies of their data and executed by ctx_sql_flush(): every
// metasync cycle, before a host loads its contexts from the table, and once more at shutdown.
//
// Only the newest write of a (host, context) pair is kept, since both writes replace the whole row. A store that
// replaces a pending delete keeps the metadata cleanup the delete would have scheduled.

typedef enum __attribute__((packed)) {
    CTX_SQL_STORE,
    CTX_SQL_DELETE,
} CTX_SQL_OP;

typedef struct ctx_sql_write {
    CTX_SQL_OP op;
    bool cleanup;                   // a store that replaced a pending delete
    nd_uuid_t host_uuid;
    VERSIONED_CONTEXT_DATA data;    // owns its strings
    struct ctx_sql_write *prev, *next;
} CTX_SQL_WRITE;

static struct {
    SPINLOCK spinlock;              // protects the fields below; never held across SQLite
    Pvoid_t JudyHS;                 // host uuid + context id -> its pending CTX_SQL_WRITE
    CTX_SQL_WRITE *head;            // pending writes, oldest first
    bool closed;                    // set by the final flush; later writes are dropped
    size_t dropped;
    bool flush_mutex_initialized;

    netdata_mutex_t flush_mutex;    // one flusher at a time, so batches execute in queueing order
} ctx_sql_queue = {
    .spinlock = SPINLOCK_INITIALIZER,
};

static const char *ctx_sql_strdupz_or_null(const char *s)
{
    return s ? strdupz(s) : NULL;
}

static void ctx_sql_write_free(CTX_SQL_WRITE *w)
{
    freez((void *)w->data.id);
    freez((void *)w->data.title);
    freez((void *)w->data.chart_type);
    freez((void *)w->data.units);
    freez((void *)w->data.family);
    freez(w);
}

static void ctx_sql_queue_write(CTX_SQL_OP op, nd_uuid_t *host_uuid, VERSIONED_CONTEXT_DATA *context_data)
{
    if (unlikely(!host_uuid || !context_data || !context_data->id))
        return;

    CTX_SQL_WRITE *w = callocz(1, sizeof(*w));
    w->op = op;
    uuid_copy(w->host_uuid, *host_uuid);
    w->data = *context_data;
    w->data.id = strdupz(context_data->id);
    w->data.title = ctx_sql_strdupz_or_null(context_data->title);
    w->data.chart_type = ctx_sql_strdupz_or_null(context_data->chart_type);
    w->data.units = ctx_sql_strdupz_or_null(context_data->units);
    w->data.family = ctx_sql_strdupz_or_null(context_data->family);

    size_t id_len = strlen(w->data.id);
    size_t key_len = sizeof(nd_uuid_t) + id_len;
    char *key = mallocz(key_len);
    memcpy(key, w->host_uuid, sizeof(nd_uuid_t));
    memcpy(key + sizeof(nd_uuid_t), w->data.id, id_len);

    CTX_SQL_WRITE *replaced = NULL;
    size_t dropped = 0;

    spinlock_lock(&ctx_sql_queue.spinlock);
    if (unlikely(ctx_sql_queue.closed)) {
        dropped = ++ctx_sql_queue.dropped;
        replaced = w;
    }
    else {
        Pvoid_t *PValue = JudyHSIns(&ctx_sql_queue.JudyHS, key, key_len, PJE0);
        if (unlikely(PValue == PJERR))
            fatal("CONTEXT SQL: failed to index a queued context write");

        replaced = *PValue;
        if (replaced) {
            w->cleanup = (replaced->op == CTX_SQL_DELETE || replaced->cleanup);
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(ctx_sql_queue.head, replaced, prev, next);
        }

        *PValue = w;
        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(ctx_sql_queue.head, w, prev, next);
    }
    spinlock_unlock(&ctx_sql_queue.spinlock);

    freez(key);

    if (replaced)
        ctx_sql_write_free(replaced);

    if (unlikely(dropped == 1))
        nd_log_daemon(NDLP_WARNING, "CONTEXT SQL: dropping context writes queued after the final flush at shutdown");
}

void ctx_sql_queue_store(nd_uuid_t *host_uuid, VERSIONED_CONTEXT_DATA *context_data)
{
    ctx_sql_queue_write(CTX_SQL_STORE, host_uuid, context_data);
}

void ctx_sql_queue_delete(nd_uuid_t *host_uuid, VERSIONED_CONTEXT_DATA *context_data)
{
    ctx_sql_queue_write(CTX_SQL_DELETE, host_uuid, context_data);
}

#define CTX_SQL_FINAL_FLUSH_WAIT_MS 3000

// Executes the pending context writes, in queueing order. final = metasync's last flush at shutdown: it closes the
// queue, and waits at most CTX_SQL_FINAL_FLUSH_WAIT_MS for another flusher, which may be stuck in SQLite after
// outliving metasync's shutdown wait; past that, the pending writes are dropped.
void ctx_sql_flush(bool final)
{
    // netdata_mutex_t has no static initializer
    spinlock_lock(&ctx_sql_queue.spinlock);
    if (unlikely(!ctx_sql_queue.flush_mutex_initialized)) {
        netdata_mutex_init(&ctx_sql_queue.flush_mutex);
        ctx_sql_queue.flush_mutex_initialized = true;
    }
    spinlock_unlock(&ctx_sql_queue.spinlock);

    bool locked;
    if (final) {
        locked = (netdata_mutex_trylock(&ctx_sql_queue.flush_mutex) == 0);
        for (int waited_ms = 0; !locked && waited_ms < CTX_SQL_FINAL_FLUSH_WAIT_MS; waited_ms += 10) {
            sleep_usec(10 * USEC_PER_MS);
            locked = (netdata_mutex_trylock(&ctx_sql_queue.flush_mutex) == 0);
        }
    }
    else {
        netdata_mutex_lock(&ctx_sql_queue.flush_mutex);
        locked = true;
    }

    spinlock_lock(&ctx_sql_queue.spinlock);
    CTX_SQL_WRITE *batch = ctx_sql_queue.head;
    ctx_sql_queue.head = NULL;
    (void)JudyHSFreeArray(&ctx_sql_queue.JudyHS, PJE0);
    if (final)
        ctx_sql_queue.closed = true;
    spinlock_unlock(&ctx_sql_queue.spinlock);

    size_t dropped = 0;
    while (batch) {
        CTX_SQL_WRITE *w = batch;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(batch, w, prev, next);

        if (unlikely(!locked))
            dropped++;
        else {
            // flushers run where SQLite may run: schedule the cleanup directly, never through metasync's queue,
            // which a flusher on metasync's worker could wait on while metasync is shutting down
            if (w->op == CTX_SQL_DELETE || w->cleanup)
                metadata_schedule_ctx_cleanup_now(&w->host_uuid, w->data.id);

            if (w->op == CTX_SQL_DELETE)
                (void)ctx_delete_context_row(&w->host_uuid, &w->data);
            else
                (void)ctx_store_context(&w->host_uuid, &w->data);
        }

        ctx_sql_write_free(w);
    }

    if (locked)
        netdata_mutex_unlock(&ctx_sql_queue.flush_mutex);
    else if (dropped)
        nd_log_daemon(NDLP_WARNING,
                      "CONTEXT SQL: dropped %zu context writes at shutdown, another flush is still running after %d ms",
                      dropped, CTX_SQL_FINAL_FLUSH_WAIT_MS);
}

// returns -1 when the value is unavailable, which pulse turns into UINT64_MAX and skips
int sql_context_cache_stats(int op)
{
    int count = 0, dummy;

    if (unlikely(!db_context_meta))
        return -1;

    if (sqlite3_db_status(db_context_meta, op, &count, &dummy, 0) != SQLITE_OK)
        return -1;

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

struct ctx_sql_queue_unittest_row {
    const char *id;
    size_t found;
    uint64_t version;
};

static void ctx_sql_queue_unittest_cb(VERSIONED_CONTEXT_DATA *context_data, void *data)
{
    struct ctx_sql_queue_unittest_row *row = data;
    if (context_data->id && strcmp(context_data->id, row->id) == 0) {
        row->found++;
        row->version = context_data->version;
    }
}

// the stored version of a context, 0 when it has no row
static uint64_t ctx_sql_queue_unittest_version(nd_uuid_t *host_uuid, const char *id, size_t *found)
{
    struct ctx_sql_queue_unittest_row row = { .id = id };
    ctx_get_context_list(host_uuid, ctx_sql_queue_unittest_cb, &row);
    *found = row.found;
    return row.version;
}

static void ctx_sql_queue_unittest_final_flush(void *arg)
{
    bool *started = arg;
    __atomic_store_n(started, true, __ATOMIC_RELEASE);
    ctx_sql_flush(true);
}

static int ctx_sql_queue_unittest(nd_uuid_t *host_uuid)
{
    VERSIONED_CONTEXT_DATA d = {
        .title = "title", .chart_type = "line", .units = "units", .family = "family",
        .priority = 1000, .first_time_s = 1657781000, .last_time_s = 1657781100,
    };
    size_t found;
    int errors = 0;

#define CTX_SQL_QUEUE_CHECK(id, exp_found, exp_version) do {                                                          \
        uint64_t _v = ctx_sql_queue_unittest_version(host_uuid, (id), &found);                                         \
        if (found != (exp_found) || (found && _v != (exp_version))) {                                                  \
            netdata_log_error("CONTEXT SQL QUEUE TEST: '%s' expected %zu row(s) version %" PRIu64                     \
                              ", found %zu version %" PRIu64, (id), (size_t)(exp_found), (uint64_t)(exp_version),      \
                              found, _v);                                                                              \
            errors++;                                                                                                  \
        }                                                                                                              \
    } while (0)

    // nothing is written before a flush; the newest of two stores wins
    d.id = "queue.store"; d.version = 1; ctx_sql_queue_store(host_uuid, &d);
    d.version = 2; ctx_sql_queue_store(host_uuid, &d);
    CTX_SQL_QUEUE_CHECK("queue.store", 0, 0);
    ctx_sql_flush(false);
    CTX_SQL_QUEUE_CHECK("queue.store", 1, 2);

    // a delete then a store: one pending store that keeps the delete's cleanup, and the store wins
    d.id = "queue.store"; ctx_sql_queue_delete(host_uuid, &d);
    d.version = 3; ctx_sql_queue_store(host_uuid, &d);
    spinlock_lock(&ctx_sql_queue.spinlock);
    CTX_SQL_WRITE *pending = ctx_sql_queue.head;
    bool coalesced = pending && !pending->next && pending->op == CTX_SQL_STORE && pending->cleanup;
    spinlock_unlock(&ctx_sql_queue.spinlock);
    if (!coalesced) {
        netdata_log_error("CONTEXT SQL QUEUE TEST: a delete then a store did not coalesce into one store with cleanup");
        errors++;
    }
    ctx_sql_flush(false);
    CTX_SQL_QUEUE_CHECK("queue.store", 1, 3);

    // a store then a delete: the row is gone
    ctx_sql_queue_delete(host_uuid, &d);
    d.id = "queue.other"; d.version = 7; ctx_sql_queue_store(host_uuid, &d);
    ctx_sql_flush(false);
    CTX_SQL_QUEUE_CHECK("queue.store", 0, 0);
    CTX_SQL_QUEUE_CHECK("queue.other", 1, 7);

    // the final flush waits for a flusher that is still running, executes what was queued before it, and closes the
    // queue: later writes are dropped
    d.id = "queue.final"; d.version = 8; ctx_sql_queue_store(host_uuid, &d);
    bool final_flusher_started = false;
    netdata_mutex_lock(&ctx_sql_queue.flush_mutex);
    ND_THREAD *final_flusher = nd_thread_create(
        "CTXSQL_TEST", NETDATA_THREAD_OPTION_DONT_LOG, ctx_sql_queue_unittest_final_flush, &final_flusher_started);
    while (!__atomic_load_n(&final_flusher_started, __ATOMIC_ACQUIRE))
        sleep_usec(USEC_PER_MS);
    // hold the mutex well past the helper's first trylock
    sleep_usec(200 * USEC_PER_MS);
    netdata_mutex_unlock(&ctx_sql_queue.flush_mutex);
    nd_thread_join(final_flusher);
    CTX_SQL_QUEUE_CHECK("queue.final", 1, 8);
    d.id = "queue.late"; d.version = 9; ctx_sql_queue_store(host_uuid, &d);
    ctx_sql_flush(false);
    CTX_SQL_QUEUE_CHECK("queue.late", 0, 0);

    // reopen the queue for the rest of this process
    spinlock_lock(&ctx_sql_queue.spinlock);
    ctx_sql_queue.closed = false;
    ctx_sql_queue.dropped = 0;
    spinlock_unlock(&ctx_sql_queue.spinlock);

#undef CTX_SQL_QUEUE_CHECK

    if (errors)
        netdata_log_error("CONTEXT SQL QUEUE TEST: %d check(s) failed", errors);
    else
        netdata_log_info("CONTEXT SQL QUEUE TEST: passed");

    return errors ? 1 : 0;
}

int ctx_unittest(void)
{
    nd_uuid_t host_uuid;
    uuid_generate(host_uuid);

    sqlite3 *saved_db_context_meta = db_context_meta;
    bool owns_sqlite_lifecycle = !saved_db_context_meta;

    if (sqlite_library_init())
        return 1;

    int rc = sql_init_context_database(1);

    if (rc != SQLITE_OK) {
        sql_close_database(db_context_meta, "CONTEXT");
        db_context_meta = saved_db_context_meta;
        if (owns_sqlite_lifecycle)
            sqlite_library_shutdown();
        return 1;
    }

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

    int queue_errors = ctx_sql_queue_unittest(&host_uuid);

    sql_close_database(db_context_meta, "CONTEXT");
    db_context_meta = saved_db_context_meta;
    if (owns_sqlite_lifecycle)
        sqlite_library_shutdown();

    return queue_errors;
}

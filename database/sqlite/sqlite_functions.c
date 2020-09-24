// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include <lz4.h>

struct dimension_list *global_dimensions = NULL;

#define ENABLE_CACHE_DIMENSIONS
#define ENABLE_CACHE_CHARTS

sqlite3 *db = NULL;
sqlite3 *db_page = NULL;
sqlite3 *dbmem = NULL;

int rotation = 0;

#define SQLITE_MAINT_LOOP_DELAY     1000    // ms

int metrics_read=0, metrics_write = 0, in_memory_metrics_read = 0;


static int items_to_commit = 0;
static sqlite3_stmt *row_res = NULL;
static sqlite3_stmt *stmt_metric_page = NULL;
static sqlite3_stmt *stmt_metric_page_rotation = NULL;
static sqlite3_stmt *res = NULL;
static int db_initialized = 0;

static uv_mutex_t sqlite_lookup;
static uv_mutex_t sqlite_flush;
static uv_mutex_t sqlite_add_page;
static uint32_t pending_page_inserts = 0;
static uint32_t database_flush_transaction_count;
static uint32_t database_size;
static uint32_t page_size;
static uint32_t page_count;
static uint32_t free_page_count;
static uint32_t delete_rows;


/*
 * Database parameters
 */
uint32_t sqlite_disk_quota_mb;      // quota specified in the database
uint32_t desired_pages = 0;
uint32_t page_cache_mb;
uint32_t db_wal_size;
int sqlite_disk_mode;

struct sqlite_page_flush {
    uint32_t    page_count;               // Number of pages to flush
    struct rrddim_metric_page *head;
    struct rrddim_metric_page *tail;    // Add head
} sqlite_page_flush_list = { .page_count=0, .head=NULL, .tail=NULL};

/*
 * Add a UUID in memory for fast lookup
 *
 * Caller should lock the uuid_cache
 *   Under host holds charts
 *   Under chart holds dimensions
 *
 */

static inline void add_in_uuid_cache(struct uuid_cache **uuid_cache, uuid_t *uuid, const char *type, const char *id, const char *name)
{
    struct uuid_cache *item = mallocz(sizeof(*item));
    uuid_copy(item->uuid, *uuid);
    item->id = id ? strdupz(id) : NULL;
    item->type = type ? strdupz(type) : NULL;
    item->name = name ? strdupz(name) : NULL;
    item->next = *uuid_cache;
    *uuid_cache = item;
    return;
}

/*
 * Destroy the uuid cache used during startup
 * Normally it should be empty if all charts and dimensions are active
 * TODO: Cleanup the cache after the agent starts to save memory
 * charts / dimensions that will be activated will just query the database
 */
void free_uuid_cache(struct uuid_cache **uuid_cache)
{
    struct uuid_cache *item;
    while ((item = *uuid_cache)) {
        *uuid_cache = item->next;
        freez(item->id);
        freez(item->type);
        freez(item->name);
        freez(item);
    }
    return;
}


/*
 * Find a type,id,name in cache -- remove if found
 * Caller must lock the uuid_cache
 */
uuid_t *find_in_uuid_cache(struct uuid_cache **uuid_cache, const char *type, const char *id, const char *name)
{
    //struct uuid_cache **temp_uuid_cache = uuid_cache;
    struct uuid_cache *item;
    uuid_t  *uuid = NULL;

    while ((item = *uuid_cache)) {
        if ((!strcmp(item->id, id)) && (!type || (!strcmp(item->type, type))) && ((!name && !item->name) || (!strcmp(item->name, name)))) {
            uuid = mallocz(sizeof(uuid_t));
            uuid_copy(*uuid, item->uuid);
            *uuid_cache = item->next;
            freez(item->id);
            freez(item->type);
            freez(item->name);
            freez(item);
            break;
        }
        uuid_cache = &(item)->next;
    }
    return uuid;
}

/*
 * Flush a single page (lock is needed)
 * Wont unlink the page
 */
void sqlite_flush_page_single(struct rrddim_metric_page *metric_page) {
    RRDDIM *rd;
    if (unlikely(!metric_page || metric_page->stored & 1))
        return;

    rd = metric_page->rd;
    sql_add_metric_page_nolock(
            rd->state->metric_uuid, metric_page->values, metric_page->active_count, metric_page->first_entry_t,
            metric_page->last_entry_t);

    //sqlite_page_flush_list.page_count--;
    metric_page->stored = 1;
    rd->state->db_last_entry_t = metric_page->last_entry_t;
//    sqlite_page_flush_list.head = sqlite_page_flush_list.head->next;
    return;
}



static void sqlite_flush_page(uint32_t count, struct rrddim_metric_page *target_metric_page) {
    RRDDIM *rd;
    struct rrddim_metric_page *metric_page;

    if (unlikely(target_metric_page)) {
        uv_mutex_lock(&sqlite_flush);
        uv_mutex_lock(&sqlite_add_page);
        metric_page = target_metric_page;
        while (metric_page) {
            sqlite_flush_page_single(metric_page);
            metric_page = metric_page->prev;
        }
        uv_mutex_unlock(&sqlite_add_page);
        uv_mutex_unlock(&sqlite_flush);
        return;
    }

    if (!sqlite_page_flush_list.page_count)
        return;

    if (count == 0) {
        // Special case -- we need the lock
        //uv_rwlock_wrlock(&sqlite_add_page);
        while (sqlite_page_flush_list.page_count > 0) {
            if (sqlite_page_flush_list.page_count < database_flush_transaction_count)
                sqlite_flush_page(sqlite_page_flush_list.page_count, NULL);
            else {
                sqlite_flush_page(database_flush_transaction_count, NULL);
            }
        }
        //uv_rwlock_wrunlock(&sqlite_add_page);
        return;
    }

    uv_mutex_lock(&sqlite_add_page);
    uv_mutex_lock(&sqlite_flush);

//    int rc;
    int added = 0;
    while (count && (metric_page = sqlite_page_flush_list.head)) {
        struct rrddim_metric_page *next_metric_page =  sqlite_page_flush_list.head->next;
        if (likely(!(metric_page->stored & 1))) {
            rd = metric_page->rd;
            sql_add_metric_page_nolock(rd->state->metric_uuid, metric_page->values, metric_page->active_count, metric_page->first_entry_t,
                metric_page->last_entry_t);
            // TODO: Check if we need this
            //rd->state->db_last_entry_t = metric_page->last_entry_t;
            metric_page->stored = 3;        // Stored and can be removed
            count--;
            added++;
        }
        sqlite_page_flush_list.page_count--;
        sqlite_page_flush_list.head = next_metric_page;
        if (added % 10 == 0 && next_metric_page) {
            uv_mutex_unlock(&sqlite_flush);
            uv_mutex_unlock(&sqlite_flush);
        }
    }
    if (!sqlite_page_flush_list.head)
        sqlite_page_flush_list.tail = NULL;


    uv_mutex_unlock(&sqlite_flush);
    uv_mutex_unlock(&sqlite_add_page);
}

void sqlite_queue_page_to_flush(struct rrddim_metric_page *metric_page)
{
    uv_mutex_lock(&sqlite_flush);

    metric_page->next = NULL;
    if (sqlite_page_flush_list.tail)
        sqlite_page_flush_list.tail->next = metric_page;
    else
        sqlite_page_flush_list.head = metric_page;
    sqlite_page_flush_list.tail = metric_page;
    sqlite_page_flush_list.page_count++;

    uv_mutex_unlock(&sqlite_flush);
    return;
}

struct rrddim_metric_page *rrddim_init_metric_page(RRDDIM *rd)
{
    struct rrddim_metric_page *metric_page = mallocz(sizeof(*metric_page));

    metric_page->active_count = 0;
    metric_page->stored = 0;
    metric_page->entries = rd->rrdset->entries;     // Initial entries from the CHART
    metric_page->last_entry_t = 0;
    metric_page->first_entry_t = LONG_MAX;
    metric_page->next = NULL;
    metric_page->prev = NULL;
    metric_page->rd = rd;
    metric_page->values = callocz(metric_page->entries, sizeof(storage_number));
    return metric_page;
}

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

static void _uncompress(sqlite3_context *context, int argc, sqlite3_value **argv)
{
    uint32_t uncompressed_buf[4096];
    if ( argc != 1 || sqlite3_value_blob(argv[0]) == NULL) {
        sqlite3_result_null(context);
        return ;
    }
    int ret = LZ4_decompress_safe((char *) sqlite3_value_blob(argv[0]), (char *) &uncompressed_buf,
                                  sqlite3_value_bytes(argv[0]), 4096);
    sqlite3_result_blob(context, uncompressed_buf, ret, SQLITE_TRANSIENT);
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
#define HOST_DEF "CREATE TABLE IF NOT EXISTS host (host_uuid blob PRIMARY KEY, hostname text, registry_hostname text, update_every int, os text, timezone text, tags text);"
#define CHART_DEF "CREATE TABLE IF NOT EXISTS chart (chart_uuid blob PRIMARY KEY, host_uuid blob, type text, id text, name text, family text, context text, title text, unit text, plugin text, module text, priority int, update_every int, chart_type int, memory_mode int, history_entries);"
#define DIM_DEF "CREATE TABLE IF NOT EXISTS dimension(dim_uuid blob PRIMARY KEY, chart_uuid blob, id text, name text, multiplier int, divisor int , algorithm int, options text);"

#define CHART_RAM_DEF "CREATE TABLE IF NOT EXISTS ram.chart (chart_uuid blob PRIMARY KEY, host_uuid blob, type text, id text, name text, family text, context text, title text, unit text, plugin text, module text, priority int, update_every int, chart_type int, memory_mode int, history_entries);"
#define DIM_RAM_DEF "CREATE TABLE IF NOT EXISTS ram.dimension(dim_uuid blob PRIMARY KEY, chart_uuid blob, id text, name text, multiplier int, divisor int , algorithm int, options text);"

#define DIM_TRIGGER "CREATE TEMPORARY TRIGGER tr_dim after delete on ram.dimension begin insert into dimension (dim_uuid, chart_uuid, id, name, multiplier, divisor, algorithm, options) values (old.dim_uuid, old.chart_uuid, old.id, old.name, old.multiplier, old.divisor, old.algorithm, old.options);  end;"

#define CHART_TRIGGER "CREATE TEMPORARY TRIGGER tr_chart after delete on ram.chart begin insert into chart (chart_uuid,host_uuid,type,id,name,family,context,title,unit,plugin,module,priority,update_every,chart_type,memory_mode,history_entries) values (old.chart_uuid,old.host_uuid,old.type,old.id,old.name,old.family,old.context,old.title,old.unit,old.plugin,old.module,old.priority,old.update_every,old.chart_type,old.memory_mode,old.history_entries); end;"

int sql_init_database()
{
    char *err_msg = NULL;
    char sqlite_database[FILENAME_MAX+1];
    char wstr[512];

    fatal_assert(0 == uv_mutex_init(&sqlite_flush));
    fatal_assert(0 == uv_mutex_init(&sqlite_lookup));
    fatal_assert(0 == uv_mutex_init(&sqlite_add_page));

    sqlite_disk_quota_mb =  (uint32_t) config_get_number(CONFIG_SECTION_GLOBAL, "database disk space", 64);
    page_cache_mb =  (uint32_t) config_get_number(CONFIG_SECTION_GLOBAL, "database page cache", 2000);
    db_wal_size =  (uint32_t) config_get_number(CONFIG_SECTION_GLOBAL, "database wal size", 16 * 1024 * 1024);

    sqlite_disk_mode = config_get_boolean(CONFIG_SECTION_GLOBAL, "sqlite mode disk", 1);
    database_flush_transaction_count = (uint32_t) config_get_number(CONFIG_SECTION_GLOBAL, "database flush transaction count", 128);
    delete_rows = (uint32_t) config_get_number(CONFIG_SECTION_GLOBAL, "database delete row count", 10000);

    snprintfz(sqlite_database, FILENAME_MAX, "%s/sqlite", netdata_configured_cache_dir);

    int rc;

    if (sqlite_disk_mode)
        rc = sqlite3_open(sqlite_database, &db);
    else
        rc =  sqlite3_open(":memory:", &db);

    if (rc != SQLITE_OK) {
        errno = 0;
        error("Failed to initialize database");
        return 1;
    }

    info("SQLite Database initialized at %s (rc = %d), database quota set to %u MiB, WAL file size set to %u bytes", sqlite_database, rc, sqlite_disk_quota_mb,  db_wal_size);

    if (sqlite_disk_mode)
        sprintf(wstr, "PRAGMA auto_vacuum=incremental; PRAGMA synchronous=1 ; PRAGMA journal_mode=WAL; PRAGMA journal_size_limit=%u; PRAGMA cache_size=-%u; PRAGMA temp_store=MEMORY;", db_wal_size, page_cache_mb);
    else
        sprintf(wstr, "PRAGMA synchronous=0 ; PRAGMA journal_mode=memory; PRAGMA journal_size_limit=%u; PRAGMA cache_size=-%u; PRAGMA temp_store=MEMORY;", db_wal_size, page_cache_mb);

    rc = sqlite3_exec(db, wstr, 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }


    // TODO: Open a second connection to handle transactions (needs to open in shared mode)
    // Keep the db_page handle the same as db for now
    //rc = sqlite3_open(sqlite_database, &db_page);
    db_page = db;

    rc = sqlite3_exec(db, "create table if not exists agent_configuration (key text primary key, value);", 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    sprintf(wstr,"insert or replace into agent_configuration (key, value) values ('max_sqlite_space', %u);", sqlite_disk_quota_mb);

    rc = sqlite3_exec(db, wstr, 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    rc = sqlite3_exec(db, HOST_DEF, 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    rc = sqlite3_exec(db, CHART_DEF, 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    rc = sqlite3_exec(db, DIM_DEF, 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    rc = sqlite3_exec(db, "create index if not exists ind_chart_uuid on dimension (chart_uuid);", 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    rc = sqlite3_exec(db, "create index if not exists ind_dim1 on dimension (chart_uuid, id, name);", 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    rc = sqlite3_exec(db, "create index if not exists ind_cha1 on chart (host_uuid, id, name);", 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    rc = sqlite3_exec(db, "create index if not exists ind_host_uuid on chart (host_uuid);", 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    rc = sqlite3_exec(db, "CREATE TABLE IF NOT EXISTS datafile (fileno integer primary key, path text, file_size int); delete from datafile;", 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    rc = sqlite3_exec(db, "CREATE TABLE IF NOT EXISTS page (dim_uuid blob, page int , page_size int, start_date int, end_date int, fileno int, offset int, size int); --delete from page;", 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    rc = sqlite3_exec(db, "create unique index if not exists ind_page on page (dim_uuid, start_date);", 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    rc = sqlite3_exec(db, "create table if not exists chart_active (chart_uuid blob primary key, date_created int); delete from chart_active;", 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    rc = sqlite3_exec(db, "create table if not exists dimension_active (dim_uuid blob primary key, date_created int); delete from dimension_active;", 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    // EXAMPLE -- Creating RAM database and attaching
    rc = sqlite3_exec(db, "ATTACH ':memory:' as ram;", 0, 0, &err_msg);
    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    rc = sqlite3_exec(db, CHART_RAM_DEF, 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    rc = sqlite3_exec(db, DIM_RAM_DEF, 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    rc = sqlite3_exec(db, CHART_TRIGGER, 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    rc = sqlite3_exec(db, DIM_TRIGGER, 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    rc = sqlite3_exec(db, "CREATE TABLE IF NOT EXISTS metric_update(dim_uuid blob primary key, date_created int);", 0, 0, &err_msg);
    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    rc = sqlite3_exec(db, "CREATE TABLE IF NOT EXISTS metric_page(dim_uuid blob, entries int, start_date int, end_date int, metric blob);", 0, 0, &err_msg);
    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    rc = sqlite3_exec(db, "create unique index if not exists ind_metric_page on metric_page (dim_uuid, start_date);", 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    rc = sqlite3_exec(db, "create index if not exists ind_start on metric_page (start_date);", 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    rc = sqlite3_exec(db, "create index if not exists ind_se on metric_page (dim_uuid, start_date, end_date);", 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    rc = sqlite3_exec(db, "CREATE TABLE IF NOT EXISTS metric_migrate(dim_uuid blob, entries int, start_date int, end_date int, metric blob);", 0, 0, &err_msg);
    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    rc = sqlite3_exec(db, "create unique index if not exists ind_metric_migrate on metric_migrate (dim_uuid, start_date);", 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    sqlite3_create_function(db, "u2h", 1, SQLITE_ANY | SQLITE_DETERMINISTIC , 0, _uuid_parse, 0, 0);
    sqlite3_create_function(db, "h2u", 1, SQLITE_ANY | SQLITE_DETERMINISTIC , 0, _uuid_unparse, 0, 0);
    sqlite3_create_function(db, "uncompress", 1, SQLITE_ANY , 0, _uncompress, 0, 0);


    // Determine the database page seq storage fraction
    sqlite3_stmt *res;
    int seqpc = 0;
    rc = sqlite3_exec(db, SQLITE_GET_PAGE_SEQFRACTION1, 0, 0, &err_msg);
    if (rc != SQLITE_OK) {
        error("SQL error during database rotation %s", err_msg);
        sqlite3_free(err_msg);
    }
    rc = sqlite3_prepare_v2(db, SQLITE_GET_PAGE_SEQFRACTION2, -1, &res, (const char **) &err_msg);
    if (rc != SQLITE_OK) {
        error("SQL error during database rotation %s", err_msg);
        sqlite3_free(err_msg);
    }
    if (rc == SQLITE_OK) {
        while (sqlite3_step(res) == SQLITE_ROW)
            seqpc = sqlite3_column_int(res, 0);
        info("Database sequential page storage is at %d%%", seqpc);
        // TODO: If percentage is low, consider VACUUM to defragment
    }
    sqlite3_finalize(res);
    rc = sqlite3_exec(db, "DROP TABLE s;", 0, 0, NULL);

    db_initialized = 1;

    return rc;
}

int sql_close_database()
{
    char *err_msg = NULL;
    info("SQLITE: Closing database");
    if (db) {
        info("Writing %u dirty pages to the database due to shutdown", sqlite_page_flush_list.page_count);
        while (sqlite_page_flush_list.page_count) {
            sqlite_flush_page(database_flush_transaction_count, NULL);
        }
        uv_mutex_unlock(&sqlite_add_page);
        if (pending_page_inserts) {
            info("Writing final transactions %u", pending_page_inserts);
            sqlite3_exec(db_page, "COMMIT TRANSACTION;", 0, 0, &err_msg);
            pending_page_inserts = 0;
        }
        sqlite3_finalize(stmt_metric_page);
        sqlite3_finalize(stmt_metric_page_rotation);
        sqlite3_finalize(row_res);
        sqlite3_finalize(res);
        uv_mutex_unlock(&sqlite_add_page);
        sqlite3_close(db);
        if (db != db_page)
            sqlite3_close(db_page);
    }
    return 0;
}

/*
 * Return the database size in MiB
 *
 */
int sql_database_size()
{
    sqlite3_stmt *res;
    int rc;

    rc = sqlite3_prepare_v2(db, "pragma page_count;", -1, &res, 0);
    if (rc != SQLITE_OK)
        return 0;

    if (sqlite3_step(res) == SQLITE_ROW)
        page_count = sqlite3_column_int(res, 0);

    sqlite3_finalize(res);

    rc = sqlite3_prepare_v2(db, "pragma freelist_count;", -1, &res, 0);
    if (rc != SQLITE_OK)
        return 0;

    if (sqlite3_step(res) == SQLITE_ROW)
        free_page_count = sqlite3_column_int(res, 0);

    sqlite3_finalize(res);

    if (unlikely(!page_size)) {
        rc = sqlite3_prepare_v2(db, "pragma page_size;", -1, &res, 0);
        if (rc != SQLITE_OK)
            return 0;

        if (sqlite3_step(res) == SQLITE_ROW)
            page_size = (uint32_t) sqlite3_column_int(res, 0);

        sqlite3_finalize(res);
        desired_pages = (sqlite_disk_quota_mb * 0.95) * (1024 * 1024 / page_size);
        info("Database desired size is %u pages (page size is %u bytes). Current size is %u pages (includes %u free pages)", desired_pages, page_size, page_count, free_page_count);
    }

    return ((page_count - free_page_count) / 1024) * (page_size / 1024);
}

/*
 * Do database compaction / rotation
 *
 * First call will try to bring database below the quota
 *
 */

void sql_compact_database(uint32_t rows)
{
    char *err_msg = NULL;
    char sql[512];
    int rc;
    static int report_free = 0;
    static int init = 0;

    uint32_t quota = (uint32_t)(sqlite_disk_quota_mb * 0.95);

    uint32_t pages_to_vacuum;
    uint32_t rows_to_delete;

    if (unlikely(!init)) {
        init = 0;
        pages_to_vacuum = free_page_count;
        rows_to_delete = 0;
        if (page_count > desired_pages) {
            info(
                "Required size = %d -- current size = %d (free pages = %d -- gap = %d)", desired_pages, page_count,
                free_page_count, page_count - desired_pages - free_page_count);
            if (free_page_count >= (page_count - desired_pages)) {
                pages_to_vacuum = page_count - desired_pages;
                rows_to_delete = 0;
            } else {
                pages_to_vacuum = free_page_count;
                rows_to_delete = (page_count - desired_pages - free_page_count) * 6;
            }
            rotation = 1;
        }
        else
            rotation = 0;

        if (rows_to_delete) {
            info("Deleting %u rows from metrics", rows_to_delete);
            sprintf(sql, "delete from metric_page order by start_date limit %u", rows_to_delete);
            rc = sqlite3_exec(db_page, sql, 0, 0, &err_msg);
            if (rc != SQLITE_OK) {
                error("SQL error during database rotation %s", err_msg);
                sqlite3_free(err_msg);
            }
        }
        if (pages_to_vacuum) {
            info("VACUUM incremental %u pages from metrics", pages_to_vacuum);
            sprintf(sql, "pragma incremental_vacuum(%u)", pages_to_vacuum);
            rc = sqlite3_exec(db_page, sql, 0, 0, &err_msg);
            if (rc != SQLITE_OK) {
                error("SQL error during database rotation %s", err_msg);
                sqlite3_free(err_msg);
            }
        }
    }

    return;

    // Occupied size is within limit?
    if (database_size <= quota) {
        // Check if our actual size is over limit and trim if necessary
        // Compute quota pages
        //uint32_t desired_pages = quota * (1024 * 1024 / page_size);
        if (page_count > desired_pages && free_page_count) {
            if (!report_free) {
                report_free = 1;
                info(
                    "Database free page count %u, starting incremental vacuum to shrink database by %u pages",
                    free_page_count, page_count - desired_pages);
            }
            rc = sqlite3_exec(db_page, "pragma incremental_vacuum(100);", 0, 0, &err_msg);
        }
        rotation = 0;
        return;
    }
    report_free = 0;
    rotation = 1;

    return;

    info(
        "Database size = %u MiB, limit is %u (total pages %u, free pages %u)", database_size, sqlite_disk_quota_mb,
        page_count, free_page_count);

    if (sqlite_disk_mode) {
        sprintf(sql, "delete from metric_page order by start_date limit %u;", rows);
        rc = sqlite3_exec(db_page, sql, 0, 0, &err_msg);
        if (rc == SQLITE_OK) {
            uint32_t new_database_size = sql_database_size();
            // TODO: maybe calculate rate to achieve end goal of database quota in 1/10 of the cache size duration
            info("Database rotation, deleting %u rows, freed %u MiB", rows, database_size - new_database_size);
        }
    } else
        rc = sqlite3_exec(db_page, "delete from metric_page order by start_date limit 100;", 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error during database rotation %s", err_msg);
        sqlite3_free(err_msg);
    }
}

void sql_backup_database()
{
    char *err_msg = NULL;

    char sql[512];

    sprintf(sql,"VACUUM into '/tmp/database.%ld'", time(NULL));

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

    if (!db) {
        errno = 0;
        error("Database has not been initialized");
        return 1;
    }
    // FIRST WAY TO DO IT
    uuid_unparse_lower(*dim_uuid, dim_str);
    uuid_unparse_lower(*chart_uuid, chart_str);

    sprintf(sql, "INSERT OR REPLACE into ram.dimension (dim_uuid, chart_uuid, id, name, multiplier, divisor , algorithm) values (u2h('%s'),u2h('%s'),'%s','%s', %lld, %lld, %d) ;",
            dim_str, chart_str, id, name, multiplier, divisor, algorithm);
    //unsigned long long start = now_realtime_usec();
    rc = sqlite3_exec(db, sql, 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }
    //unsigned long long end = now_realtime_usec();
    //info("SQLITE: Query insert in %llu usec", end - start);
    // FIRST DONE

    // SECOND WAY TO DO IT
//    sqlite3_stmt *res;
//#define SQL_INSERT_DIMENSION "INSERT OR REPLACE into dimension (dim_uuid, chart_uuid, id, name, multiplier, divisor , algorithm, archived) values (?0001,?0002,?0003,?0004, ?0005, ?0006, ?0007, 1) ;"
//    rc = sqlite3_prepare_v2(db, SQL_INSERT_DIMENSION, -1, &res, 0);
//    if (rc != SQLITE_OK)
//        return 1;
//
//    int param = sqlite3_bind_parameter_index(res, "@dim");
//    rc = sqlite3_bind_blob(res, 1, dim_uuid, 16, SQLITE_STATIC);
//    rc = sqlite3_bind_blob(res, 2, chart_uuid, 16, SQLITE_STATIC);
//    rc = sqlite3_bind_text(res, 3, id, -1, SQLITE_STATIC);
//    rc = sqlite3_bind_text(res, 4, name, -1, SQLITE_STATIC);
//    rc = sqlite3_bind_int(res, 5, multiplier);
//    rc = sqlite3_bind_int(res, 6, divisor);
//    rc = sqlite3_bind_int(res, 7, algorithm);
//    // Omit checks
//    rc = sqlite3_step(res);
//    sqlite3_finalize(res);
    items_to_commit = 1;
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

#define SQL_SELECT_DIMENSION    "select id, name, multiplier, divisor , algorithm, options from dimension where dim_uuid = @dim;"

RRDDIM *sql_create_dimension(char *dim_str, RRDSET *st, int temp)
{
    //char sql[1024];
    uuid_t  dim_uuid;
    sqlite3_stmt *res;
    int rc;

    if (!db)
        return NULL;

    uuid_parse(dim_str, dim_uuid);

    //sprintf(sql, "select id, name, multiplier, divisor , algorithm, o ptions from dimension where dim_uuid = u2h('%s') and archived = 1;", dim_str);
    rc = sqlite3_prepare_v2(db, SQL_SELECT_DIMENSION, -1, &res, 0);
    if (rc != SQLITE_OK)
        return NULL;

    int param = sqlite3_bind_parameter_index(res, "@dim");

    rc = sqlite3_bind_blob(res, param, dim_uuid, 16, SQLITE_STATIC);
    if (rc != SQLITE_OK) // Release the RES
        return NULL;

    rc = sqlite3_step(res);

    RRDDIM *rd = NULL;
    if (rc == SQLITE_ROW) {
        rd = rrddim_add_custom(
            st, (const char *)sqlite3_column_text(res, 0), (const char *)sqlite3_column_text(res, 1),
            sqlite3_column_int(res, 2), sqlite3_column_int(res, 3), sqlite3_column_int(res, 4), st->rrd_memory_mode,
            temp);

        if (temp != 1) {
            rrddim_flag_clear(rd, RRDDIM_FLAG_HIDDEN);
            rrddim_flag_clear(rd, RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS);
            rrddim_isnot_obsolete(st, rd); /* archived dimensions cannot be obsolete */
            const char *option = (const char *)sqlite3_column_text(res, 5);
            if (option && *option) {
                if (strstr(option, "hidden") != NULL)
                    rrddim_flag_set(rd, RRDDIM_FLAG_HIDDEN);
                if (strstr(option, "noreset") != NULL)
                    rrddim_flag_set(rd, RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS);
                if (strstr(option, "nooverflow") != NULL)
                    rrddim_flag_set(rd, RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS);
            }
        }
    }

    sqlite3_finalize(res);

    return rd;
}

RRDHOST *sql_create_host_by_name(char *hostname)
{
    sqlite3_stmt *res = NULL;
    int rc;

    rc = sqlite3_prepare_v2(db, "select h2u(host_uuid), registry_hostname, update_every, os, timezone, tags from host where hostname = @host", -1, &res, 0);
    if (rc != SQLITE_OK)
        return NULL;

    rc = sqlite3_bind_text(res, 1, hostname, -1, SQLITE_TRANSIENT);

    RRDHOST *host = NULL;

//    char *registry_hostname;
    if (sqlite3_step(res) == SQLITE_ROW) {
        host = rrdhost_create(
            hostname, (const char *) sqlite3_column_text(res, 1), (const char *) sqlite3_column_text(res, 0), (const char *) sqlite3_column_text(res, 3),
            (const char *) sqlite3_column_text(res, 4), (const char *) sqlite3_column_text(res, 5), NULL, NULL, sqlite3_column_int(res, 2), 600,
            RRD_MEMORY_MODE_SQLITE, 0, 0, NULL, NULL, NULL, NULL, 0, 1);
    }

    rc = sqlite3_finalize(res);
    return host;
}

RRDSET *sql_create_chart_by_name(RRDHOST *host, char *chart)
{
    sqlite3_stmt *res = NULL;
    int rc;

    rc = sqlite3_prepare_v2(db, "select type, id, name, family, context, title, unit, plugin, module, priority, update_every, chart_type from chart where type||\".\"||id = @chart and host_uuid = @host;", -1, &res, 0);
    if (rc != SQLITE_OK) {
        info("sql_create_chart_by_name: prepare failed");
        return NULL;
    }

    rc = sqlite3_bind_text(res, 1, chart, -1, SQLITE_TRANSIENT);
    rc = sqlite3_bind_blob(res, 2, &host->host_uuid, 16, SQLITE_TRANSIENT);

    RRDSET *st = NULL;

    if (sqlite3_step(res) == SQLITE_ROW) {
        info("CREATING new chart %s", sqlite3_column_text(res, 1));
        st = rrdset_create_custom(
            host,
            (const char *) sqlite3_column_text(res, 0),
            (const char *) sqlite3_column_text(res, 1),
            (const char *) sqlite3_column_text(res, 2),
            (const char *) sqlite3_column_text(res, 3),
            (const char *) sqlite3_column_text(res, 4),
            (const char *) sqlite3_column_text(res, 5),
            (const char *) sqlite3_column_text(res, 6),
            (const char *) sqlite3_column_text(res, 7),
            (const char *) sqlite3_column_text(res, 8),
            sqlite3_column_int(res, 9),
            sqlite3_column_int(res, 10),
            sqlite3_column_int(res, 11),
            host->rrd_memory_mode, host->rrd_history_entries, 1);
    }
    rc = sqlite3_finalize(res);
    return st;
}

#define HOST_DEF "CREATE TABLE IF NOT EXISTS host (host_uuid blob PRIMARY KEY, hostname text, registry_hostname text, update_every int, os text, timezone text, tags text);"

#define INSERT_HOST "insert or replace into host (host_uuid,hostname,registry_hostname,update_every,os,timezone,tags) values (?1,?2,?3,?4,?5,?6,?7);"
int sql_store_host(
    const char *guid, const char *hostname, const char *registry_hostname, int update_every, const char *os, const char *tzone, const char *tags)
{
    sqlite3_stmt *res;
    int rc;

    rc = sqlite3_prepare_v2(db, INSERT_HOST, -1, &res, 0);
    if (rc != SQLITE_OK)
        return 1;

    uuid_t  host_uuid;
    uuid_parse(guid, host_uuid);

    rc = sqlite3_bind_blob(res, 1, host_uuid, 16, SQLITE_TRANSIENT);
    rc = sqlite3_bind_text(res, 2, hostname, -1, SQLITE_TRANSIENT);
    rc = sqlite3_bind_text(res, 3, registry_hostname, -1, SQLITE_TRANSIENT);
    rc = sqlite3_bind_int(res, 4, update_every);
    rc = sqlite3_bind_text(res, 5, os, -1, SQLITE_TRANSIENT);
    rc = sqlite3_bind_text(res, 6, tzone, -1, SQLITE_TRANSIENT);
    rc = sqlite3_bind_text(res, 7, tags, -1, SQLITE_TRANSIENT);

    rc = sqlite3_step(res);

    rc = sqlite3_finalize(res);

    return 0;
}

#define INSERT_CHART "insert or replace into ram.chart (chart_uuid, host_uuid, type, id, name, family, context, title, unit, plugin, module, priority, update_every , chart_type , memory_mode , history_entries) values (?1,?2,?3,?4,?5,?6,?7,?8,?9,?10,?11,?12,?13,?14,?15,?16);"
int sql_store_chart(
    uuid_t *chart_uuid, uuid_t *host_uuid, const char *type, const char *id, const char *name, const char *family,
    const char *context, const char *title, const char *units, const char *plugin, const char *module, long priority,
    int update_every, int chart_type, int memory_mode, long history_entries)
{
    sqlite3_stmt *res;
    int rc;

    rc = sqlite3_prepare_v2(db, INSERT_CHART, -1, &res, 0);
    if (rc != SQLITE_OK)
        return 1;

    rc = sqlite3_bind_blob(res, 1, chart_uuid, 16, SQLITE_TRANSIENT);
    rc = sqlite3_bind_blob(res, 2, host_uuid, 16, SQLITE_TRANSIENT);
    rc = sqlite3_bind_text(res, 3, type, -1, SQLITE_TRANSIENT);
    rc = sqlite3_bind_text(res, 4, id, -1, SQLITE_TRANSIENT);
    if (name)
        rc = sqlite3_bind_text(res, 5, name, -1, SQLITE_TRANSIENT);
    //else
    //   rc = sqlite3_bind_text(res, 5, id, -1, SQLITE_TRANSIENT);
    rc = sqlite3_bind_text(res, 6, family, -1, SQLITE_TRANSIENT);
    rc = sqlite3_bind_text(res, 7, context, -1, SQLITE_TRANSIENT);
    rc = sqlite3_bind_text(res, 8, title, -1, SQLITE_TRANSIENT);
    rc = sqlite3_bind_text(res, 9, units, -1, SQLITE_TRANSIENT);
    rc = sqlite3_bind_text(res, 10, plugin, -1, SQLITE_TRANSIENT);
    rc = sqlite3_bind_text(res, 11, module, -1, SQLITE_TRANSIENT);
    rc = sqlite3_bind_int(res, 12, priority);
    rc = sqlite3_bind_int(res, 13, update_every);
    rc = sqlite3_bind_int(res, 14, chart_type);
    rc = sqlite3_bind_int(res, 15, memory_mode);
    rc = sqlite3_bind_int(res, 15, history_entries);

    rc = sqlite3_step(res);

    rc = sqlite3_finalize(res);

    //info("SQLITE: Will create chart %s", id);
    items_to_commit = 1;
    return 0;
}

/*
 * Load a charts dimensions and create them under RRDSET
 */
RRDDIM *sql_load_chart_dimensions(RRDSET *st, int temp)
{
    char sql[1024];
    char chart_str[37];
    int rc;
    char *err_msg = NULL;

    if (!db)
        return NULL;

    struct dimension *dimension_list = NULL, *tmp_dimension_list;

    uuid_unparse_lower(*st->chart_uuid, chart_str);
    sprintf(sql, "select h2u(dim_uuid), id, name from dimension where chart_uuid = u2h('%s');", chart_str);

    rc = sqlite3_exec(db, sql, dim_callback, &dimension_list, &err_msg);
    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }

    //sql_load_one_chart_dimension(st->chart_uuid, &dimension_list);

    // loop through all the dimensions and create under the chart
//    RRDDIM *rd = NULL;
    while(dimension_list) {

        //RRDDIM *temp_rd = sql_create_dimension(dimension_list->dim_str, st, temp);
        sql_create_dimension(dimension_list->dim_str, st, temp);

        tmp_dimension_list = dimension_list->next;
        freez(dimension_list->id);
        freez(dimension_list->name);
        freez(dimension_list);
        dimension_list = tmp_dimension_list;
        //temp_rd->next = rd;
        //rd = temp_rd;
    }

    return st->dimensions;
}

int sql_load_one_chart_dimension(uuid_t *chart_uuid, BUFFER *wb, int *dimensions)
{
//    char *err_msg = NULL;
    char sql[1024];
    char chart_str[37];
    int rc;
    sqlite3_stmt *res = NULL;

    if (!db)
        return 1;

    uuid_unparse_lower(*chart_uuid, chart_str);

    if (!res) {
        sprintf(sql, "select h2u(dim_uuid), id, name from dimension where chart_uuid = @chart;");
        rc = sqlite3_prepare_v2(db, sql, -1, &res, 0);
        if (rc != SQLITE_OK) {
            error("Failed to prepare statement");
            return 1;
        }
    }

    rc = sqlite3_bind_blob(res, 1, chart_uuid, 16, SQLITE_TRANSIENT);

    //unsigned long long start = now_realtime_usec();
    while (sqlite3_step(res) == SQLITE_ROW) {
        if (*dimensions)
            buffer_strcat(wb, ",\n\t\t\t\t\"");
        else
            buffer_strcat(wb, "\t\t\t\t\"");
        buffer_strcat_jsonescape(wb, (char *) sqlite3_column_text(res, 1));
        buffer_strcat(wb, "\": { \"name\": \"");
        buffer_strcat_jsonescape(wb, (char *) sqlite3_column_text(res, 2));
        buffer_strcat(wb, " (");
        buffer_strcat(wb, (char *) sqlite3_column_text(res, 0));
        buffer_strcat(wb, ")");
        buffer_strcat(wb, "\" }");
        (*dimensions)++;
    }
    //unsigned long long end = now_realtime_usec();
    //info("SQLITE: Chart %s generated in %llu usec", chart_str, end - start);
    sqlite3_finalize(res);
    return 0;
}


/*
 * Load all chart dimensions and return an array
 */

//#define SQL_GET_DIMLIST "select h2u(dim_uuid), id, name, chart_uuid, rowid from ram.chart_dim where chart_uuid = @chart;"
#define SQL_GET_DIMLIST "select h2u(dim_uuid), id, name, chart_uuid, rowid from ram.chart_dim order by chart_uuid;"
int sql_select_dimension(uuid_t *chart_uuid, struct dimension_list **dimension_list, int *from, int *to)
{
//    char *err_msg = NULL;
//    char chart_str[37];
    int rc;
    sqlite3_stmt *res;
//    static sqlite3_stmt *row_res = NULL;

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

    if (global_dimensions) {
        for(int i=0; global_dimensions[i].id; i++) {
            freez(global_dimensions[i].id);
            freez(global_dimensions[i].name);
        }
        freez(global_dimensions);
        global_dimensions = NULL;
    }

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
            uuid_parse((const char *) sqlite3_column_text(res, 0), global_dimensions[i].dim_uuid);
            strcpy(global_dimensions[i].dim_str, (const char *) sqlite3_column_text(res, 0));
            //strcpy(global_dimensions[i].id, sqlite3_column_text(res, 1));
            //strcpy(global_dimensions[i].name, sqlite3_column_text(res, 2));
            global_dimensions[i].id = strdupz((const char *) sqlite3_column_text(res, 1));
            global_dimensions[i].name = strdupz((const char *) sqlite3_column_text(res, 2));
            i++;
        }

        info("Initialized dimensions %d", i);
        sqlite3_finalize(res);
    }

    if (from && to) {
        if (!row_res)
            rc = sqlite3_prepare_v2(
                db, "select min_row, max_row from ram.chart_stat where chart_uuid = @chart;", -1, &row_res, 0);
        int param = sqlite3_bind_parameter_index(row_res, "@chart");
        rc = sqlite3_bind_blob(row_res, param, chart_uuid, 16, SQLITE_STATIC);
        if (rc != SQLITE_OK) {
            error("Failed to bind to get chart range");
        }
        while (sqlite3_step(row_res) == SQLITE_ROW) {
            *from = sqlite3_column_int(row_res, 0) - 1;
            *to = sqlite3_column_int(row_res, 1);
        }
        //sqlite3_finalize(row_res);
        sqlite3_reset(row_res);
    }
    *dimension_list = global_dimensions;

    return 0;
}

uuid_t *sql_find_dim_uuid(RRDSET *st, RRDDIM *rd)  //, char *id, char *name, collected_number multiplier, collected_number divisor, int algorithm)
{
    sqlite3_stmt *res = NULL;
//    sqlite3_stmt *res1 = NULL;
    uuid_t *uuid = NULL;
    int rc;

    //info("LOOKUP Dim %s in chart %s", rd->id, st->id);
    uuid = find_in_uuid_cache(&st->state->uuid_cache, NULL, rd->id, rd->name);

    if (uuid)
        goto found;

    //netdata_mutex_lock(&sqlite_find_uuid);
    if (!res) {
        rc = sqlite3_prepare_v2(
            db, "select dim_uuid from dimension where chart_uuid = @chart and id = @id and name = @name;", -1, &res, 0);
        if (rc != SQLITE_OK) {
            info("SQLITE: failed to prepare statement to lookup dimension GUID");
            //netdata_mutex_unlock(&sqlite_find_uuid);
            return NULL;
        }
    }

    int dim_id = sqlite3_bind_parameter_index(res, "@chart");
    int id_id = sqlite3_bind_parameter_index(res, "@id");
    int name_id = sqlite3_bind_parameter_index(res, "@name");

    rc = sqlite3_bind_blob(res, dim_id, st->chart_uuid, 16, SQLITE_TRANSIENT);
    rc = sqlite3_bind_text(res, id_id, rd->id, -1, SQLITE_TRANSIENT);
    rc = sqlite3_bind_text(res, name_id, rd->name, -1, SQLITE_TRANSIENT);

    while (sqlite3_step(res) == SQLITE_ROW) {
        uuid = mallocz(sizeof(uuid_t));
        uuid_copy(*uuid, sqlite3_column_blob(res, 0));
        break;
    }

    // Dimension is not known, create it
    if (uuid == NULL) {
        uuid = mallocz(sizeof(uuid_t));
        uuid_generate(*uuid);
        sql_store_dimension(uuid, st->chart_uuid, rd->id, rd->name, rd->multiplier, rd->divisor, rd->algorithm);
    }
    sqlite3_reset(res);
    sqlite3_finalize(res);

    found:

//    if (uuid) {
//        rc = sqlite3_prepare_v2(
//            db, "insert or replace into dimension_active (dim_uuid, date_created) values (@id, strftime('%s'));", -1,
//            &res1, 0);
//        if (rc != SQLITE_OK) {
//            info("SQLITE: failed to bind to update dimension active");
//            return NULL;
//        }
//        rc = sqlite3_bind_blob(res1, 1, uuid, 16, SQLITE_TRANSIENT);
//        // TODO: check return code etc
//        while (rc = sqlite3_step(res1) != SQLITE_DONE) {
//            if (rc != SQLITE_BUSY)
//                break;
//            info("Busy detected on DIM set to active");
//        }
//        sqlite3_reset(res1);
//        sqlite3_finalize(res1);
//    }
    // netdata_mutex_unlock(&sqlite_find_uuid);

    return uuid;
}

uuid_t *sql_find_chart_uuid(RRDHOST *host, RRDSET *st, const char *type, const char *id, const char *name) // char *id, char *name, const char *type, const char *family,
//const char *context, const char *title, const char *units, const char *plugin, const char *module, long priority,
//int update_every, int chart_type, int memory_mode, long history_entries)
{
    sqlite3_stmt *res = NULL;
    sqlite3_stmt *res1 = NULL;
    uuid_t *uuid = NULL;
    int rc;

    //Check in the CHART cache first
//    struct uuid_cache *temp_uuid_cache = host->uuid_cache;

    uuid = find_in_uuid_cache(&host->uuid_cache, type, id, name);

    if (uuid)
        goto found;

    //netdata_mutex_lock(&sqlite_find_uuid);

    if (!res) {
        rc = sqlite3_prepare_v2(
            db, "select chart_uuid from chart where host_uuid = @host and id = @id and name = @name;", -1, &res, 0);
        if (rc != SQLITE_OK) {
            info("SQLITE: failed to bind to find GUID");
            //netdata_mutex_unlock(&sqlite_find_uuid);
            return NULL;
        }
    }

    int dim_id = sqlite3_bind_parameter_index(res, "@host");
    int id_id = sqlite3_bind_parameter_index(res, "@id");
    int name_id = sqlite3_bind_parameter_index(res, "@name");

    rc = sqlite3_bind_blob(res, dim_id, &host->host_uuid, 16, SQLITE_TRANSIENT);
    rc = sqlite3_bind_text(res, id_id, st->id, -1, SQLITE_TRANSIENT);
    rc = sqlite3_bind_text(res, name_id, st->name, -1, SQLITE_TRANSIENT);

    while (sqlite3_step(res) == SQLITE_ROW) {
        uuid = mallocz(sizeof(uuid_t));
        uuid_copy(*uuid, sqlite3_column_blob(res, 0));
        break;
    }

    if (uuid == NULL) {
        uuid = mallocz(sizeof(uuid_t));
        uuid_generate(*uuid);
        char uuid_str[37];
        uuid_unparse_lower(*uuid, uuid_str);
        info("SQLite: Generating uuid [%s] for chart %s under host %s", uuid_str, st->id, host->hostname);
        sql_store_chart(
            uuid, &host->host_uuid, st->type, id, name, st->family, st->context, st->title, st->units, st->plugin_name, st->module_name, st->priority,
            st->update_every, st->chart_type, st->rrd_memory_mode, st->entries);
    }
    //char uuid_str[37];
    //uuid_unparse_lower(*uuid, uuid_str);
    //info("SQLite: Returning uuid [%s] for chart %s under host %s", uuid_str, id, host->hostname);
    sqlite3_reset(res);
    sqlite3_finalize(res);

    found:

    if (uuid && !rrdset_flag_check(st, RRDSET_FLAG_ARCHIVED)) {
        rc = sqlite3_prepare_v2(
            db, "insert or replace into chart_active (chart_uuid, date_created) values (@id, strftime('%s'));",
            -1, &res1, 0);
        if (rc != SQLITE_OK) {
            info("SQLITE: failed to bind to update charts");
            return NULL;
        }
        rc = sqlite3_bind_blob(res1, 1, uuid, 16, SQLITE_TRANSIENT);
        // TODO: check return code etc
        rc = sqlite3_step(res1);
        sqlite3_reset(res1);
        sqlite3_finalize(res1);
    }
    else
    if (uuid) {
        info("Not setting chart %s to active", id);
    }
    //netdata_mutex_unlock(&sqlite_find_uuid);
    return uuid;
}

void  sql_add_metric(uuid_t *dim_uuid, usec_t point_in_time, storage_number number)
{
    char *err_msg = NULL;
    char  sql[1024];
    char  dim_str[37];
    int rc;

    if (!dbmem) {
        rc = sqlite3_open(":memory:", &dbmem);
        if (rc != SQLITE_OK) {
            error("SQL error: %s", err_msg);
            sqlite3_free(err_msg);
            return;
        }
        info("SQLite in memory initialized");

        rc = sqlite3_exec(dbmem, "PRAGMA synchronous=0 ; CREATE TABLE IF NOT EXISTS metric(dim_uuid text, date_created int, value int);", 0, 0, &err_msg);
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

/*
 * Store a page of metrics to the database
 * This will be an array of values size * storage_number
 */

void sql_add_metric_page_nolock(uuid_t *dim_uuid, storage_number *metric, size_t entries, time_t start_time, time_t end_time)
{
    char *err_msg = NULL;
    int rc;
    char compressed_buf[32768];
    int compressed_size = 0;
    int max_compressed_size = sizeof(compressed_buf);

    if (unlikely(start_time == LONG_MAX))
        return;

    if (!stmt_metric_page) {
        rc = sqlite3_prepare_v2(db_page, "insert into metric_page (entries, dim_uuid, start_date, end_date, metric) values (@entries, @dim, @start_date, @end_date, @page);", -1, &stmt_metric_page, 0);
        if (rc != SQLITE_OK) {
            info("SQLITE: Failed to prepare statement for metric page");
            return;
        }
    }

    if (!stmt_metric_page_rotation) {
        rc = sqlite3_prepare_v2(db_page, "delete from metric_page order by start_date limit 1;", -1, &stmt_metric_page_rotation, 0);
        if (rc != SQLITE_OK) {
            info("SQLITE: Failed to prepare statement for metric page");
            return;
        }
    }

    if (unlikely(!pending_page_inserts)) {
        //info("Starting METRIC transaction");
        rc = sqlite3_exec(db_page, "BEGIN TRANSACTION;", 0, 0, &err_msg);
    }

    if (entries)
        compressed_size = LZ4_compress_default((const char *) metric, compressed_buf, entries * sizeof(storage_number), max_compressed_size);

    sqlite3_clear_bindings(stmt_metric_page);
    rc = sqlite3_bind_int(stmt_metric_page, 1, entries);
    rc = sqlite3_bind_blob(stmt_metric_page, 2, dim_uuid, 16, SQLITE_TRANSIENT);
    rc = sqlite3_bind_int64(stmt_metric_page, 3, start_time);
    rc = sqlite3_bind_int64(stmt_metric_page, 4, end_time);
    if (entries)
        rc = sqlite3_bind_blob(stmt_metric_page, 5, compressed_buf, compressed_size, SQLITE_TRANSIENT);


    while ((rc=sqlite3_step(stmt_metric_page)) != SQLITE_DONE) {
        if (rc == SQLITE_BUSY) {
            info("SQLITE: Reports busy on metric page insert");
            usleep(50 * USEC_PER_MS);
        }
        else {
            char dim_str[37];
            uuid_unparse_lower(*dim_uuid, dim_str);
            info("SQLITE: Error on adding metric page nolock %d -- adding (%s, %ld)", rc, dim_str, start_time);
            break;
        }
    }

    while (rotation && (rc=sqlite3_step(stmt_metric_page_rotation)) != SQLITE_DONE) {
        if (rc == SQLITE_BUSY) {
            info("SQLITE: Reports busy on metric page rotation");
            usleep(50 * USEC_PER_MS);
        }
        else {
            char dim_str[37];
            uuid_unparse_lower(*dim_uuid, dim_str);
            info("SQLITE: Error on adding metric page nolock %d -- adding (%s, %ld)", rc, dim_str, start_time);
            break;
        }
        info("Page deleted");
    }

    pending_page_inserts++;
    sqlite3_reset(stmt_metric_page);
    sqlite3_reset(stmt_metric_page_rotation);

    if (pending_page_inserts == database_flush_transaction_count) {
        //info("Ending METRIC transaction %d pages", pending_page_inserts);
        rc = sqlite3_exec(db_page, "COMMIT TRANSACTION;", 0, 0, &err_msg);
        pending_page_inserts = 0;
    }
    return;
}



void sql_add_metric_page(uuid_t *dim_uuid, storage_number *metric, size_t entries, time_t start_time, time_t end_time)
{
    char *err_msg = NULL;
    //char  dim_str[37];
    int rc;
    char compressed_buf[32768];
    int compressed_size = 0;
    int max_compressed_size = sizeof(compressed_buf);

    if (unlikely(start_time > end_time))
        return;

//    if (!res) {
//        rc = sqlite3_prepare_v2(db, "insert into metric_update (dim_uuid, date_created) values (@dim_uuid, @date) on conflict(dim_uuid) DO update set date_created=excluded.date_created;", -1, &res, 0);
//        if (rc != SQLITE_OK) {
//            info("SQLITE: Failed to prepare statement");
//            return;
//        }
//        dim_id = sqlite3_bind_parameter_index(res, "@dim_uuid");
//        date_id = sqlite3_bind_parameter_index(res, "@date");
//    }
    //info("GET SQLITE_PAGE_LOCK");
    uv_mutex_lock(&sqlite_add_page);

    if (!stmt_metric_page) {
        rc = sqlite3_prepare_v2(db_page, "insert into metric_page (entries, dim_uuid, start_date, end_date, metric) values (@entries, @dim, @start_date, @end_date, @page);", -1, &stmt_metric_page, 0);
        if (rc != SQLITE_OK) {
            info("SQLITE: Failed to prepare statement for metric page");
            uv_mutex_unlock(&sqlite_add_page);
            return;
        }
    }

    if (unlikely(!pending_page_inserts)) {
        //info("Starting METRIC transaction");
        rc = sqlite3_exec(db_page, "BEGIN TRANSACTION;", 0, 0, &err_msg);
    }

    if (entries)
        compressed_size = LZ4_compress_default((const char *) metric, compressed_buf, entries * sizeof(storage_number), max_compressed_size);

    sqlite3_clear_bindings(stmt_metric_page);
    rc = sqlite3_bind_int(stmt_metric_page, 1, entries);
    rc = sqlite3_bind_blob(stmt_metric_page, 2, dim_uuid, 16, SQLITE_TRANSIENT);
    rc = sqlite3_bind_int64(stmt_metric_page, 3, start_time);
    rc = sqlite3_bind_int64(stmt_metric_page, 4, end_time);
    if (entries)
        rc = sqlite3_bind_blob(stmt_metric_page, 5, compressed_buf, compressed_size, SQLITE_TRANSIENT);

    //unsigned long long start = now_realtime_usec();
    //sqlite3_step(res);
//    while ((rc=sqlite3_step(res)) != SQLITE_DONE) {
//        if (rc == SQLITE_BUSY)
//            info("SQLITE: Reports busy on metric update");
//        else {
//            info("SQLITE: Error %d", rc);
//            break;
//        }
//    }
//    sqlite3_reset(res);
    while ((rc=sqlite3_step(stmt_metric_page)) != SQLITE_DONE) {
        if (rc == SQLITE_BUSY) {
            info("SQLITE: Reports busy on metric page insert");
            usleep(50 * USEC_PER_MS);
        }
        else {
            info("SQLITE: Error on addding metric page %d", rc);
            break;
        }
    }
    pending_page_inserts++;
    sqlite3_reset(stmt_metric_page);

    //info("Pages pending %d",pending_page_inserts);

    if (pending_page_inserts == database_flush_transaction_count) {
        //info("Ending METRIC transaction %d pages", pending_page_inserts);
        rc = sqlite3_exec(db_page, "COMMIT TRANSACTION;", 0, 0, &err_msg);
        pending_page_inserts = 0;
    }

    uv_mutex_unlock(&sqlite_add_page);
    //info("REL SQLITE_PAGE_LOCK");

    //sqlite3_finalize(res);
    //sqlite3_finalize(res_page);
    //unsigned long long end = now_realtime_usec();
    //info("SQLITE: PAGE %s in %llu usec (%d -> %d bytes) entries=%d (%d - %d)", dim_str, end-start, page_length, compressed_size, entries, start_time, end_time);
    //freez(compressed_buf); // May be null
    return;
}

//void sql_add_metric_page_from_extent(struct rrdeng_page_descr *descr)
//{
//    char *err_msg = NULL;
//    char dim_str[37];
//    int rc;
//    static sqlite3_stmt *res = NULL;
//    static sqlite3_stmt *res_page = NULL;
//    static int dim_id, date_id;
//    static int metric_id;
//    static int level = 0;
//
//    if (!descr->page_length) {
//        info("SQLITE: Empty page");
//        return;
//    }
//    level++;
//
//    uuid_unparse_lower(descr->id, dim_str);
//    uint32_t entries = descr->page_length / sizeof(storage_number);
//    uint32_t *metric = descr->pg_cache_descr->page;
//
//    if (!res_page) {
//        rc = sqlite3_prepare_v2(
//            db,
//            "insert or replace into metric_migrate (entries, dim_uuid, start_date, end_date, metric) values (@entries, @dim, @start_date, @end_date, @page);",
//            -1, &res_page, 0);
//        if (rc != SQLITE_OK) {
//            info("SQLITE: Failed to prepare statement for metric page");
//            return;
//        }
//        metric_id = sqlite3_bind_parameter_index(res_page, "@page");
//    }
//
//    rc = sqlite3_bind_blob(res, dim_id, descr->id, 16, SQLITE_TRANSIENT);
//    rc = sqlite3_bind_int(res, date_id, descr->end_time / USEC_PER_SEC);
//
//    void *compressed_buf = NULL;
//    int max_compressed_size = LZ4_compressBound(descr->page_length);
//    compressed_buf = mallocz(max_compressed_size);
//
//    int compressed_size = LZ4_compress_default(metric, compressed_buf, descr->page_length, max_compressed_size);
//
//    rc = sqlite3_bind_int(res_page, 1, entries);
//    rc = sqlite3_bind_int64(res_page, 3, descr->start_time);
//    rc = sqlite3_bind_int64(res_page, 4, descr->end_time);
//    rc = sqlite3_bind_blob(res_page, metric_id, compressed_buf, compressed_size, SQLITE_TRANSIENT);
//    rc = sqlite3_bind_blob(res_page, 2, descr->id, 16, SQLITE_TRANSIENT);
//
//    freez(compressed_buf);
//
//    unsigned long long start = now_realtime_usec();
//    sqlite3_step(res);
//    sqlite3_reset(res);
//    sqlite3_step(res_page);
//    sqlite3_reset(res_page);
//    unsigned long long end = now_realtime_usec();
//    info(
//        "SQLITE: PAGE in  %llu usec (%d -> %d bytes) (max computed %d) entries=%d (level - %d)", end - start,
//        descr->page_length, compressed_size, max_compressed_size, entries, level);
//    level--;
//    return;
//}

void sql_store_datafile_info(char *path, int fileno, size_t file_size)
{
    char sql[512];
    char *err_msg = NULL;
    sprintf(sql, "INSERT OR REPLACE into datafile (fileno, path , file_size ) values (%d, '%s', %lu);",
            fileno, path, file_size);
    int rc = sqlite3_exec(db, sql, 0, 0, &err_msg);

    if (rc != SQLITE_OK) {
        error("SQL error: %s", err_msg);
        sqlite3_free(err_msg);
    }
    return;
}

sqlite3_blob *sql_open_metric_blob(uuid_t *dim_uuid)
{
    sqlite3_blob *blob;
    static sqlite3_stmt *res = NULL;
    int rc;

    if (!db)
        return NULL;

    if (!res) {
        rc = sqlite3_prepare_v2(db, "select rowid from metric_update where dim_uuid = @dim;", -1, &res, 0);
        if (rc != SQLITE_OK)
            return NULL;
    }


    rc = sqlite3_bind_blob(res, 1, dim_uuid, 16, SQLITE_TRANSIENT);
    if (rc != SQLITE_OK) // Release the RES
        return NULL;

    sqlite3_int64 row = 0;
    if ((rc = sqlite3_step(res)) == SQLITE_ROW)
       row = sqlite3_column_int64(res, 0);
    else
        info("BLOB execution find row failed %d", rc);

//    if (row != 2681)
//        return NULL;

    rc = sqlite3_blob_open(db, "main", "metric_update", "metric", row, 1, &blob);
    if (rc != SQLITE_OK)
        info("BLOB open failed");

    char dim_str[37];
    uuid_unparse_lower(*dim_uuid, dim_str);
    info("BLOB open for %s on line %lld", dim_str, row);
    sqlite3_reset(res);
    return blob;
}

//void sql_store_page_info(uuid_t dim_uuid, int valid_page, int page_length, usec_t  start_time, usec_t end_time, int fileno, size_t offset, size_t size)
//{
//    char sql[512];
//    char *err_msg = NULL;
//    static sqlite3_stmt *res = NULL;
//    static int last_fileno = 0;
//    static int last_offset = 0;
//    static char *buf = NULL;
//    static char *uncompressed_buf = NULL;
//    static void *compressed_buf = NULL;
//    static int max_compressed_size = 0;
//
//    //return;
//
//    if (!res) {
//        int rc = sqlite3_prepare_v2(
//            db,
//            "INSERT OR REPLACE into page (dim_uuid, page , page_size, start_date, end_date, fileno, offset, size) values (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8);",
//            -1, &res, 0);
//        if (rc != SQLITE_OK) {
//            info("SQLITE: Failed to prepare statement for metric page");
//            return;
//        }
//    }
//
////    if (last_fileno != fileno || last_offset != offset) {
////        freez(buf);
////        freez(uncompressed_buf);
////        freez(compressed_buf);
////        //buf = mallocz(size);
////        size_t old_pos = lseek(fileno, 0, SEEK_CUR);
////        int new_pos = lseek(fileno, offset, SEEK_SET);
////        posix_memalign((void *)&buf, RRDFILE_ALIGNMENT, ALIGN_BYTES_CEILING(size));
////        int rc = read(fileno, buf, size);
////        if (rc < 0)
////                error("Cant ready the extent");
////        lseek(fileno, old_pos, SEEK_SET);
////        uncompressed_buf = mallocz(64 * page_length + 128);
////        int ret = LZ4_decompress_safe((char *) buf, (char *) uncompressed_buf, size, 64 * page_length);
////        info("Read %d bytes -- Uncompressed extent, new size = %d (old file pos %llu , new file pos %llu)", rc, ret, old_pos, new_pos);
////        max_compressed_size = LZ4_compressBound(page_length);
////        compressed_buf = mallocz(max_compressed_size);
////        last_fileno = fileno;
////        last_offset = offset;
////    }
////    // Uncompress it
////
////    int compressed_size = LZ4_compress_default(uncompressed_buf+valid_page * page_length, compressed_buf, page_length, max_compressed_size);
////    info("Compressed size for page %d = %d", valid_page, compressed_size);
//
//    int rc = sqlite3_bind_blob(res, 1  , dim_uuid, 16, SQLITE_TRANSIENT);
//    rc = sqlite3_bind_int(res, 2, valid_page);
//    rc = sqlite3_bind_int(res, 3, page_length);
//    rc = sqlite3_bind_int64(res, 4, start_time);
//    rc = sqlite3_bind_int64(res, 5, end_time);
//    rc = sqlite3_bind_int(res, 6, fileno);
//    rc = sqlite3_bind_int64(res, 7, offset);
//    rc = sqlite3_bind_int64(res, 8, size);
//    //rc = sqlite3_bind_blob(res, 9, compressed_buf, compressed_size, SQLITE_TRANSIENT);
//
//    //free(compressed_buf);
//
//    sqlite3_step(res);
//    sqlite3_reset(res);
//    return;
//}
//
//    sql_store_page_info(temp_id, valid_page, descr->page_length, descr->start_time, descr->end_time, extent->datafile->file, extent->offset);

time_t sql_rrdeng_metric_latest_time(RRDDIM *rd)
{
    //sqlite3_blob *blob;
    static sqlite3_stmt *res = NULL;
    int rc;
    time_t  tim;

    if (!db)
        return 0;

    if (!res) {
        rc = sqlite3_prepare_v2(db, "select cast(max(end_date)/1E6 as \"int\") from page where dim_uuid = @dim;", -1, &res, 0);
        if (rc != SQLITE_OK)
            return 0;
    }


    rc = sqlite3_bind_blob(res, 1, rd->state->metric_uuid, 16, SQLITE_TRANSIENT);
    if (rc != SQLITE_OK) // Release the RES
        return 0;

    unsigned long long start = now_realtime_usec();
    if ((rc = sqlite3_step(res)) == SQLITE_ROW)
        tim = sqlite3_column_int(res, 0);
    unsigned long long end = now_realtime_usec();
    info("SQLITE: MAX in %llu usec (value = %ld)", end - start, tim);

    sqlite3_reset(res);
    return tim;
}

time_t sql_rrdeng_metric_oldest_time(RRDDIM *rd)
{
//    sqlite3_blob *blob;
    static sqlite3_stmt *res = NULL;
    int rc;
    time_t tim;

    if (!db)
        return 0;

    if (!res) {
        rc = sqlite3_prepare_v2(
            db, "select cast(min(start_date)/1E6 as \"int\") from page where dim_uuid = @dim;", -1, &res, 0);
        if (rc != SQLITE_OK)
            return 0;
    }

    rc = sqlite3_bind_blob(res, 1, rd->state->metric_uuid, 16, SQLITE_TRANSIENT);
    if (rc != SQLITE_OK) // Release the RES
        return 0;

    unsigned long long start = now_realtime_usec();
    if ((rc = sqlite3_step(res)) == SQLITE_ROW)
        tim = sqlite3_column_int(res, 0);
    unsigned long long end = now_realtime_usec();
    info("SQLITE: MIN in %llu usec (value = %ld)", end - start, tim);

    sqlite3_reset(res);
    return tim;
}

void sql_rrdset_first_entry_t(RRDSET *st, time_t *first, time_t *last)
{
  //  sqlite3_blob *blob;
    sqlite3_stmt *res = NULL;
    int rc;
//    time_t tim;

    if (!db)
        return;

    if (!res) {
        rc = sqlite3_prepare_v2(
            db, "select min(m.start_date), max(m.end_date) from metric_page m, chart c, dimension d where c.chart_uuid = @chart_uuid and d.chart_uuid = c.chart_uuid and d.dim_uuid = m.dim_uuid;", -1, &res, 0);
        if (rc != SQLITE_OK)
            return;
    }

    rc = sqlite3_bind_blob(res, 1, st->chart_uuid, 16, SQLITE_TRANSIENT);
    if (rc != SQLITE_OK) // Release the RES
        return;

    //tim = LONG_MAX;
//    unsigned long long start = now_realtime_usec();
    rc = sqlite3_step(res);
    if (rc == SQLITE_ROW) {
        *first = sqlite3_column_int(res, 0);
        *last = sqlite3_column_int(res, 1);
    }

    if (unlikely(!*first))
        *first = LONG_MAX;

    //unsigned long long end = now_realtime_usec();
    //info("SQLITE: MIN/MAX %s in %llu usec (value = %ld  - %ld)", st->id, end - start, *first, *last);

    sqlite3_reset(res);
    sqlite3_finalize(res);
    return;
}

void sql_rrddim_first_last_entry_t(RRDDIM *rd, time_t *first, time_t *last)
{
  //  sqlite3_blob *blob;
    int rc;
//    time_t tim;

    if (!db)
        return;

    uv_mutex_lock(&sqlite_lookup);
    if (!res) {
        rc = sqlite3_prepare_v2(
            db, "select min(m.start_date), max(m.end_date) from metric_page m where m.dim_uuid = @dim_uuid;", -1, &res,
            0);
        if (rc != SQLITE_OK) {
            uv_mutex_unlock(&sqlite_lookup);
            return;
        }
    }

    rc = sqlite3_bind_blob(res, 1, rd->state->metric_uuid, 16, SQLITE_TRANSIENT);
    if (rc != SQLITE_OK) {
        //sqlite3_finalize(res);
        sqlite3_reset(res);
        uv_mutex_unlock(&sqlite_lookup);
        return;
    }

//    tim = LONG_MAX;
//    unsigned long long start = now_realtime_usec();
    if ((rc = sqlite3_step(res)) == SQLITE_ROW) {
        *first = sqlite3_column_int(res, 0);
        *last = sqlite3_column_int(res, 1);
    }
//    unsigned long long end = now_realtime_usec();
//    char dim_str[37];
//    uuid_unparse_lower(rd->state->metric_uuid, dim_str);
//    info("SQLITE: Fetch RD %s (%s) MIN/MAX in %llu usec (value = %ld  - %ld)", rd->id, dim_str, end - start, *first, *last);
    sqlite3_reset(res);
    //sqlite3_finalize(res);
    uv_mutex_unlock(&sqlite_lookup);
    return;
}

time_t sql_rrdset_last_entry_t(RRDSET *st)
{
//    sqlite3_blob *blob;
    static sqlite3_stmt *res = NULL;
    int rc;
    time_t tim;

    if (!db)
        return 0;

    //    if (st->state->first_entry_t != LONG_MAX) {
    //        info("SQLITE: MAX (value = %ld)", st->state->first_entry_t);
    //        return st->state->first_entry_t;
    //    }

    if (!res) {
        rc = sqlite3_prepare_v2(
            db,
            "select max(m.end_date) from metric_page m, chart c, dimension d where c.chart_uuid = @chart_uuid and d.chart_uuid = c.chart_uuid and d.dim_uuid = m.dim_uuid;",
            -1, &res, 0);
        if (rc != SQLITE_OK)
            return 0;
    }

    rc = sqlite3_bind_blob(res, 1, st->chart_uuid, 16, SQLITE_TRANSIENT);
    if (rc != SQLITE_OK) {
        sqlite3_finalize(res);
        return 0;
    }

    tim = 0;
  //  unsigned long long start = now_realtime_usec();
    if ((rc = sqlite3_step(res)) == SQLITE_ROW)
        tim = sqlite3_column_int(res, 0);
//    unsigned long long end = now_realtime_usec();
    //info("SQLITE: MAX in %llu usec (value = %ld)", end - start, tim);

    sqlite3_reset(res);
    return tim;
}

#ifdef ENABLE_DBENGINE
GUID_TYPE sql_find_object_by_guid(uuid_t *uuid, char *object, int max_size)
{
    static sqlite3_stmt *res = NULL;
    int rc;

    if (!db)
        return 0;

    int guid_type = GUID_TYPE_NOTFOUND;

    if (!res) {
        rc = sqlite3_prepare_v2(
                db, "select 1 from host where host_uuid=@guid union select 2 from chart where chart_uuid=@guid union select 3 from dimension where dim_uuid =@guid;", -1, &res, 0);
        if (rc != SQLITE_OK)
            return 0;
    }

    rc = sqlite3_bind_blob(res, 1, uuid, 16, SQLITE_TRANSIENT);
    if (rc != SQLITE_OK) // Release the RES
        return guid_type;

    unsigned long long start = now_realtime_usec();
    if ((rc = sqlite3_step(res)) == SQLITE_ROW)
        guid_type = sqlite3_column_int(res, 0);
    unsigned long long end = now_realtime_usec();
    //char dim_str[37];
    //uuid_unparse_lower(uuid, dim_str);
    //info("SQLITE: sql_find_object_by_guid [%s] in %llu usec (value = %ld)", dim_str,    end - start, guid_type);

    sqlite3_reset(res);
    return guid_type;
}

GUID_TYPE sql_add_dimension_guid(uuid_t *uuid, uuid_t *chart)
{
    static sqlite3_stmt *res = NULL;
    int rc;

    if (!db)
        return 0;

    int guid_type = GUID_TYPE_NOTFOUND;

    if (!res) {
        rc = sqlite3_prepare_v2(
            db,
            "select 1 from host where host_uuid=@guid union select 2 from chart where chart_uuid=@guid union select 3 from dimension where dim_uuid =@guid;",
            -1, &res, 0);
        if (rc != SQLITE_OK)
            return 0;
    }

    rc = sqlite3_bind_blob(res, 1, uuid, 16, SQLITE_TRANSIENT);
    if (rc != SQLITE_OK) // Release the RES
        return guid_type;

    unsigned long long start = now_realtime_usec();
    if ((rc = sqlite3_step(res)) == SQLITE_ROW)
        guid_type = sqlite3_column_int(res, 0);
    unsigned long long end = now_realtime_usec();
    //char dim_str[37];
    //uuid_unparse_lower(uuid, dim_str);
    //info("SQLITE: sql_find_object_by_guid [%s] in %llu usec (value = %ld)", dim_str, end - start, guid_type);

    sqlite3_reset(res);
    return guid_type;
}
#endif

//#define SELECT_DIMENSION "select d.chart_uuid, d.id, d.name, d.multiplier, d.divisor, d.algorithm from chart c, dimension d where c.host_uuid = @host_uuid and c.chart_uuid = d.chart_uuid order by c.chart_uuid asc;"
#define SELECT_DIMENSION "select d.id, d.name from dimension d where d.chart_uuid = @chart_uuid;"

void sql_rrdim2json(uuid_t *chart_uuid, BUFFER *wb, size_t *dimensions_count, size_t *memory_used)
{
    UNUSED(dimensions_count);
    UNUSED(memory_used);

    int rc;
    sqlite3_stmt *res_dim = NULL;

    rc = sqlite3_prepare_v2(db, SELECT_DIMENSION, -1, &res_dim, 0);
    if (rc != SQLITE_OK)
        return;

    rc = sqlite3_bind_blob(res_dim, 1, chart_uuid, 16, SQLITE_TRANSIENT);
    if (rc != SQLITE_OK)
        return;

    //uuid_t dim_chart_uuid;
//    char *dim_id = NULL;
//    char *dim_name = NULL;
//    int dim_multiplier = 0;
//    int dim_divisor = 0;
//    int dim_algorithm = 0;

    int dimensions = 0;
    buffer_sprintf(wb, "\t\t\t\"dimensions\": {\n");
    while (sqlite3_step(res_dim) == SQLITE_ROW) {
        if (dimensions)
            buffer_strcat(wb, ",\n\t\t\t\t\"");
        else
            buffer_strcat(wb, "\t\t\t\t\"");
        buffer_strcat_jsonescape(wb, (const char *) sqlite3_column_text(res_dim, 0));
        buffer_strcat(wb, "\": { \"name\": \"");
        buffer_strcat_jsonescape(wb, (const char *) sqlite3_column_text(res_dim, 1));
        buffer_strcat(wb, "\" }");
        dimensions++;

    }
    buffer_sprintf(wb, "\n\t\t\t}");
}

#define SELECT_CHART "select chart_uuid, id, name, type, family, context, title, priority, plugin, module, unit, chart_type, update_every from chart where host_uuid = @host_uuid and chart_uuid not in (select chart_uuid from chart_active) order by chart_uuid asc;"

void sql_rrdset2json(RRDHOST *host, BUFFER *wb, size_t *dimensions_count, size_t *memory_used)
{
//    time_t first_entry_t = 0; //= rrdset_first_entry_t(st);
 //   time_t last_entry_t = 0; //rrdset_last_entry_t(st);
    int rc;

    sqlite3_stmt *res_chart = NULL;

    rc = sqlite3_prepare_v2(db, SELECT_CHART, -1, &res_chart, 0);
    if (rc != SQLITE_OK) {
        error("Failed to prepare query to get charts. Wrong schema?");
        return;
    }
    rc = sqlite3_bind_blob(res_chart, 1, &host->host_uuid, 16, SQLITE_TRANSIENT);

//    uuid_t  chart_uuid;
//    char   *chart_id = NULL;
//    char   *chart_name = NULL;
//    char   *chart_type = NULL;
//    char   *chart_family = NULL;
//    char   *chart_context = NULL;
//    char   *chart_title = NULL;
//    int    chart_priority = 0;
//    char   *chart_plugin_name = NULL;
//    char   *chart_module_name = NULL;
//    char   *chart_units = NULL;
//    int    chart_type_id = 0;
//    int    chart_update_every = 0;

//    int dimensions = 0;
    int c = 0;
    while (sqlite3_step(res_chart) == SQLITE_ROW) {
        char id[512];
        sprintf(id, "%s.%s", sqlite3_column_text(res_chart, 3), sqlite3_column_text(res_chart, 1));
        RRDSET *st = rrdset_find(host, id);
        if (st && !rrdset_flag_check(st, RRDSET_FLAG_ARCHIVED))
            continue;

        if (c)
            buffer_strcat(wb, ",\n\t\t\"");
        else
            buffer_strcat(wb, "\n\t\t\"");
        c++;

        buffer_strcat(wb, id);
        buffer_strcat(wb, "\": ");

        buffer_sprintf(
            wb,
            "\t\t{\n"
            "\t\t\t\"id\": \"%s\",\n"
            "\t\t\t\"name\": \"%s\",\n"
            "\t\t\t\"type\": \"%s\",\n"
            "\t\t\t\"family\": \"%s\",\n"
            "\t\t\t\"context\": \"%s\",\n"
            "\t\t\t\"title\": \"%s (%s)\",\n"
            "\t\t\t\"priority\": %ld,\n"
            "\t\t\t\"plugin\": \"%s\",\n"
            "\t\t\t\"module\": \"%s\",\n"
            "\t\t\t\"enabled\": %s,\n"
            "\t\t\t\"units\": \"%s\",\n"
            "\t\t\t\"data_url\": \"/api/v1/data?chart=%s\",\n"
            "\t\t\t\"chart_type\": \"%s\",\n",
            id //sqlite3_column_text(res_chart, 1)
            ,
            id // sqlite3_column_text(res_chart, 2)
            ,
            sqlite3_column_text(res_chart, 3), sqlite3_column_text(res_chart, 4), sqlite3_column_text(res_chart, 5),
            sqlite3_column_text(res_chart, 6), id //sqlite3_column_text(res_chart, 2)
            ,
            (long ) sqlite3_column_int(res_chart, 7),
            (const char *) sqlite3_column_text(res_chart, 8) ? (const char *) sqlite3_column_text(res_chart, 8) : (char *) "",
            (const char *) sqlite3_column_text(res_chart, 9) ? (const char *) sqlite3_column_text(res_chart, 9) : (char *) "", (char *) "false",
            (const char *) sqlite3_column_text(res_chart, 10), id //sqlite3_column_text(res_chart, 2)
            ,
            rrdset_type_name(sqlite3_column_int(res_chart, 11)));

        sql_rrdim2json((uuid_t *) sqlite3_column_blob(res_chart, 0), wb, dimensions_count, memory_used);
        //if (dimensions)
        //    buffer_strcat(wb, ",\n\t\t\t\t\"");
        //else
        buffer_strcat(wb, "\n\t\t}");
    }

    //buffer_sprintf(wb, "\n\t\t}");

    // Final cleanup;

    return;
}

// ----------------------------------------------------------------------------
// SQLite functions based on the RRDDIM legacy ones

void rrddim_sql_collect_init(RRDDIM *rd)
{
    //info("Init collect on %s", rd->id);
    rd->state->db_first_entry_t = LONG_MAX;
    rd->state->db_last_entry_t = 0;
    sql_rrddim_first_last_entry_t(rd, &rd->state->db_first_entry_t, &rd->state->db_last_entry_t);
    if (rrddim_flag_check(rd, RRDDIM_FLAG_ARCHIVED)) {
       info("SQLITE: Fetch ARCHIVED RD %s (value = %ld  - %ld)", rd->id,  rd->state->db_first_entry_t, rd->state->db_last_entry_t);
       rd->state->metric_page->first_entry_t = rd->state->db_last_entry_t;
       rd->state->metric_page->last_entry_t = rd->state->db_last_entry_t;
    }
    rd->state->metric_page->values[rd->rrdset->current_entry] = SN_EMPTY_SLOT; // pack_storage_number(0, SN_NOT_EXISTS);
}

void rrddim_sql_collect_store_metric(RRDDIM *rd, usec_t point_in_time, storage_number number)
{
    (void)point_in_time;

    metrics_write++;

    //info("Store: %llu %s %d", point_in_time, rd->id, number);
    rd->state->metric_page->values[rd->rrdset->current_entry] = number;
    rd->state->metric_page->last_entry_t = point_in_time / USEC_PER_SEC;

    if (unlikely(!rd->rrdset->current_entry)) {
        struct rrddim_metric_page *old_metric_page = rd->state->metric_page_last;
        struct rrddim_metric_page *tmp_metric_page;

        while (old_metric_page && (old_metric_page->stored & 2) && old_metric_page->prev) {
            tmp_metric_page = old_metric_page->prev;
            freez(old_metric_page->values);
            freez(old_metric_page);
            old_metric_page = tmp_metric_page;
            rd->state->metric_page_last = tmp_metric_page;
        }

        rd->state->metric_page->first_entry_t = rd->state->metric_page->last_entry_t;
        rd->state->metric_page->active_count = 1;
        //info("Setting first time for RD %s to %d", rd->id, rd->state->first_entry_t);
        if (unlikely(rd->rrdset->state->first_entry_t == LONG_MAX))
            rd->rrdset->state->first_entry_t = rd->state->metric_page->first_entry_t;
    }
    else
        rd->state->metric_page->active_count++;

    if (unlikely(!rd->state->gap_checked)) {
        //rd->state->gap_checked = 1;
        //info("Checking %s for gap filling", rd->id);
        if (rd->state->metric_page->first_entry_t != LONG_MAX) {
            if (rd->state->db_last_entry_t && rd->state->db_last_entry_t < rd->state->metric_page->first_entry_t) {
                //info("Adding gap for (%s) %d - %d",  rd->id, rd->state->db_last_entry_t + 1, rd->state->first_entry_t - 1);
                sql_add_metric_page(
                    rd->state->metric_uuid, NULL, 0, rd->state->db_last_entry_t + 1, rd->state->metric_page->first_entry_t - 1);
                rd->state->db_last_entry_t = rd->state->metric_page->first_entry_t - 1;
                rd->state->gap_checked = 1;
            }
        }
    }
}

void rrddim_sql_flush_metrics(RRDDIM *rd)
{
    //info("Flush metrics on rotation %s %s", rd->rrdset->id, rd->id);
    if (rd->state->metric_page->active_count) {
        // Get a new metric page to be the current one
        struct rrddim_metric_page *metric_page = rrddim_init_metric_page(rd);
        struct rrddim_metric_page *old_metric_page = rd->state->metric_page;
        // TODO: Check for locking
        metric_page->next = rd->state->metric_page;
        rd->state->metric_page->prev = metric_page;
        rd->state->metric_page = metric_page;
        sqlite_queue_page_to_flush(old_metric_page);
    }
    return;
}


int rrddim_sql_collect_finalize(RRDDIM *rd)
{
    (void)rd;

    sqlite_flush_page(0, rd->state->metric_page_last);
    return 0;
}


void rrddim_sql_query_init(RRDDIM *rd, struct rrddim_query_handle *handle, time_t start_time, time_t end_time)
{
    handle->rd = rd;

    start_time = MAX(start_time, MIN(rd->state->db_first_entry_t, rd->state->metric_page->first_entry_t));

    handle->start_time = start_time;
    handle->end_time = end_time;

    //start_time = MAX(start_time, MIN(rd->state->db_first_entry_t, rd->state->metric_page->first_entry_t));

    struct rrddim_metric_page *metric_page = rd->state->metric_page_last;

    // Remove this
//    if (end_time >= rd->state->metric_page->first_entry_t) {
//        if (start_time <= rd->state->metric_page->first_entry_t)
//            handle->slotted.slot = 0;
//        else {
//            handle->slotted.slot = ((uint64_t)(start_time - rd->state->metric_page->first_entry_t)) * (rd->state->metric_page->active_count - 1) /
//                                   (rd->state->metric_page->last_entry_t - rd->state->metric_page->first_entry_t);
//        }
//    }
//    else
//        handle->slotted.slot = LONG_MAX;

    handle->slotted.finished = 0;
    handle->slotted.init = 0;
//    info("Request (%d - %d) - BUFFER has (%d - %d) SQLite (%d to %d)", start_time, end_time,
 //        metric_page->first_entry_t, metric_page->last_entry_t, rd->state->db_first_entry_t, rd->state->db_last_entry_t);

    if (start_time < metric_page->first_entry_t) {
          sqlite3_stmt **query = (sqlite3_stmt **) &(handle->slotted.query);
//        info(
//            "Request (%d - %d) - BUFFER has (%d - %d) Request needs to go to SQLite as well", start_time, end_time,
//            metric_page->first_entry_t, metric_page->last_entry_t);

//        if (start_time < rd->state->db_last_entry_t || end_time < rd->state->db_last_entry_t) {
//            start_time = MAX(start_time, rd->state->db_first_entry_t);
            end_time = MIN(end_time, metric_page->first_entry_t);
            //info("SQLite will fetch data (%d to %d)", start_time, end_time);
//            int rc = sqlite3_prepare_v2(
//                db,
//                "select start_date, end_date, entries, uncompress(metric) from metric_page where dim_uuid = @dim_uuid and (@start_date between start_date and end_date or @end_date between start_date and end_date or (@start_date <= start_date and @end_date >= end_date)) and entries > 0 order by start_date asc;",
//                -1, &(handle->slotted.query), 0);

              int rc = sqlite3_prepare_v2(
                db,
                "select start_date, end_date, entries, uncompress(metric) from metric_page where dim_uuid = @dim_uuid and (@start_date between start_date and end_date or @end_date between start_date and end_date or (@start_date <= start_date and @end_date >= end_date)) and entries > 0 order by start_date asc;",
                -1, query, 0);
            if (rc == SQLITE_OK) {
                rc = sqlite3_bind_blob(*query, 1, rd->state->metric_uuid, 16, SQLITE_TRANSIENT);
                rc = sqlite3_bind_int(*query, 2, start_time);
                rc = sqlite3_bind_int(*query, 3, end_time);
                handle->slotted.local_end_time = 0;
                handle->slotted.local_start_time = 0;
                handle->slotted.init = 1;
            } else
                info("SQLITE: Query statement failed to prepare");
    }
}

storage_number rrddim_sql_query_next_metric(struct rrddim_query_handle *handle, time_t *current_time)
{
    RRDDIM *rd = handle->rd;
    //long entries = rd->rrdset->entries;
    //long slot = handle->slotted.slot;
    (void)current_time;
    storage_number ret;

    //info("Query next for %s %d slots (%d - %d)", rd->id, *current_time, handle->slotted.slot, handle->slotted.last_slot);

    if (handle->slotted.init) {
        sqlite3_stmt *query = (sqlite3_stmt *) handle->slotted.query;
        while (1) {
            int have_valid_entry = 0;
            if (*current_time < handle->slotted.local_start_time) {
                ret = SN_EMPTY_SLOT;
                //info("SQLITE: Return EMPTY for %d (GAP)", *current_time);
                have_valid_entry = 1;
            }
            //if (/**current_time >= handle->slotted.local_start_time && */ *current_time <= handle->slotted.local_end_time) {
            else if (*current_time <= handle->slotted.local_end_time) {
                have_valid_entry = 1;
                if (sqlite3_column_bytes(query, 3) == 0) {
                   // info("SQLITE: Return EMPTY for %d (FILLED)", *current_time);
                    ret = SN_EMPTY_SLOT;
                } else {
                    //size_t entries = sqlite3_column_bytes(query, 3) / sizeof(storage_number);
                    uint32_t *metric = (uint32_t *) sqlite3_column_blob(query, 3);
                    int index;
                    if (unlikely(handle->slotted.local_end_time == handle->slotted.local_start_time))
                        index = 0;
                    else
                        index = ((uint64_t)(*current_time - handle->slotted.local_start_time)) *
                                (handle->slotted.entries - 1) /
                                (handle->slotted.local_end_time - handle->slotted.local_start_time);
                    ret = metric[index];
                    //info("SQLITE: Return valid for %d", *current_time);
                }
            }
            if (have_valid_entry) {
                if (unlikely(handle->end_time == *current_time)) {
                    //info("SQLITE: Completed at %d", *current_time);
                    handle->slotted.finished = 1;
                    handle->slotted.init = 0;
                    sqlite3_finalize(query);
                }
                metrics_read++;
                return ret;
            }
//            unsigned long long start = now_realtime_usec();
            int rc = sqlite3_step(query);
//            unsigned long long end = now_realtime_usec();
            //info("SQLITE: Fetch next metric entry in %llu usec (rc = %d)", end - start, rc);
            if (rc == SQLITE_ROW) {
                handle->slotted.local_start_time = sqlite3_column_int(query, 0);
                handle->slotted.local_end_time = sqlite3_column_int(query, 1);
                handle->slotted.entries = sqlite3_column_int(query, 2);
                //info("SQLITE: %d start %d - %d (%d)", *current_time, handle->slotted.local_start_time, handle->slotted.local_end_time, handle->slotted.entries);
                continue;
            }
            // No valid entry, no more DB results lets go to the hot page
            sqlite3_finalize(query);
            handle->slotted.query = NULL;
            handle->slotted.init = 0;
            handle->slotted.local_start_time = 0;
            handle->slotted.local_end_time = 0;
            break;
        }
    }

    int index = -1;

    struct rrddim_metric_page *metric_page = rd->state->metric_page_last; // Find the value in the metric page

    // If they ask for a value less that the first entry then return EMPTY
    // It probably went to the database and it didn't find anything anyway
    if (*current_time < metric_page->first_entry_t) {
        //info("Active request time %d -- will serve EMPTY due to gap", *current_time);
        return SN_EMPTY_SLOT;
    }

    while (metric_page) {
        if (*current_time >= metric_page->first_entry_t && *current_time <= metric_page->last_entry_t) {
            if (unlikely(metric_page->first_entry_t == metric_page->last_entry_t))
                index = 0;
            else
                index = (int)((*current_time - metric_page->first_entry_t) * (metric_page->active_count - 1) /
                        (metric_page->last_entry_t - metric_page->first_entry_t));
            break;
        }
        else {
            // TODO: Can throw away the entry
        }
//        info("SQLITE: %d not found in %p %d - %d (stored = %d) Next active buffer",
//             *current_time, metric_page, metric_page->first_entry_t, metric_page->last_entry_t, metric_page->stored);
        metric_page = metric_page->prev;
    }
    if (unlikely(index == -1)) {
        handle->slotted.finished = 1;
        return SN_EMPTY_SLOT;
    }

//    if (*current_time >= rd->state->metric_page->first_entry_t && *current_time <= rd->state->metric_page->last_entry_t) {
//
//        if (unlikely(rd->state->metric_page->first_entry_t == rd->state->metric_page->first_entry_t))
//            index = 0;
//        else
//            index = ((uint64_t)(*current_time - rd->state->metric_page->first_entry_t)) *
//                    (rd->state->metric_page->active_count - 1) / (rd->state->metric_page->first_entry_t - rd->state->metric_page->first_entry_t);
//    }

    //    size_t position;
//    if (unlikely(rd->state->active_count == 1))
//        position = 0;
//    else
//        position = ((uint64_t)(*current_time - rd->state->first_entry_t)) * (rd->state->active_count - 1) /
//               (rd->state->last_entry_t - rd->state->first_entry_t);

    //info("Active request time %d -- will serve %d", *current_time, metric_page->first_entry_t + (index * rd->update_every));
//    if (unlikely(*current_time == rd->state->first_entry_t + (slot * rd->update_every)))
//        handle->slotted.finished = 1;

    if (unlikely(*current_time == handle->end_time)) {
        //info("Active request time %d (DONE)", *current_time);
        handle->slotted.finished = 1;
    }

    //storage_number n = rd->state->metric_page->values[slot++];

    storage_number n = metric_page->values[index];

//    if (unlikely(handle->slotted.slot == LONG_MAX))
//        handle->slotted.finished = 1;
//
//    if (unlikely(handle->slotted.slot == handle->slotted.last_slot))
//        handle->slotted.finished = 1;
//    storage_number n = rd->values[slot++];
//    info("Return %d from buffer (%d)", *current_time, n);
//
//    if (unlikely(slot >= entries))
//        slot = 0;
    //handle->slotted.slot = slot;
    in_memory_metrics_read++;
    return n;
}

int rrddim_sql_query_is_finished(struct rrddim_query_handle *handle)
{
    return handle->slotted.finished;
}

void rrddim_sql_query_finalize(struct rrddim_query_handle *handle)
{
    (void)handle;

    if (handle->slotted.init) {
        error("Query has not been finalized");
    }
    return;
}

time_t rrddim_sql_query_latest_time(RRDDIM *rd)
{
    //return rrdset_last_entry_t(rd->rrdset);
    return rd->state->metric_page->last_entry_t;
}

time_t rrddim_sql_query_oldest_time(RRDDIM *rd)
{
    //return sql_rrdset_first_entry_t(rd->rrdset);
    return rd->state->metric_page->first_entry_t;
}


int sql_cache_host_charts(RRDHOST *host)
{
#ifdef ENABLE_CACHE_CHARTS
    int rc;
    sqlite3_stmt *res = NULL;

    if (!db)
        return 0;

    if (!res) {
        rc = sqlite3_prepare_v2(db, "select chart_uuid, type, id, name from chart where host_uuid = @host;", -1, &res, 0);
        if (rc != SQLITE_OK)
            return 0;
    }

    rc = sqlite3_bind_blob(res, 1, &host->host_uuid, 16, SQLITE_TRANSIENT);
    int count = 0;
    while (sqlite3_step(res) == SQLITE_ROW) {
        add_in_uuid_cache(
            &host->uuid_cache, (uuid_t *)sqlite3_column_blob(res, 0), (const char *)sqlite3_column_text(res, 1),
            (const char *)sqlite3_column_text(res, 2), (const char *)sqlite3_column_text(res, 3));
        count++;
    }
    sqlite3_finalize(res);
    return count;
#else
    return 0;
#endif
}

int sql_cache_chart_dimensions(RRDSET *st)
{
#ifdef ENABLE_CACHE_DIMENSIONS
    int rc;
    sqlite3_stmt *res = NULL;
    if (!db) {
        errno = 0;
        error("Database has not been initialized");
        return 0;
    }

    if (!res) {
        rc = sqlite3_prepare_v2(db, "select dim_uuid, id, name from dimension where chart_uuid = @chart;", -1, &res, 0);
        if (rc != SQLITE_OK) {
            errno = 0;
            error("Failed to prepare statement to find chart dimensions");
            return 0;
        }
    }

    rc = sqlite3_bind_blob(res, 1, st->chart_uuid, 16, SQLITE_TRANSIENT);
    if (rc != SQLITE_OK) {
        errno = 0;
        error("Failed to bind chart_uuid to find chart dimensions");
        sqlite3_finalize(res);
        return 0;
    }
    int count = 0;
    while (sqlite3_step(res) == SQLITE_ROW) {
        add_in_uuid_cache(&st->state->uuid_cache, (uuid_t *) sqlite3_column_blob(res, 0), NULL, (const char *) sqlite3_column_text(res,1),(const char *)  sqlite3_column_text(res,2));
        count++;
    }
    sqlite3_finalize(res);
    return count;
#else
    return 0;
#endif
}


// Thead to give statistics
static void sqlite_stats_main_cleanup(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    info("cleaning up...");

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

#define SQLITE_STATS_UPDATE_EVERY 5

void *sqlite_stats_main(void *ptr) {
    netdata_thread_cleanup_push(sqlite_stats_main_cleanup, ptr);

    RRDSET *st = rrdset_create_localhost(
        "netdata", "SQLiteStats", NULL, "SQLite", NULL, "SQLite statistics1", "Cache Statistics", "sqlite.plugin",
        NULL, 20000, SQLITE_STATS_UPDATE_EVERY, RRDSET_TYPE_LINE);
    RRDDIM *rd_hit   = rrddim_add(st, "hit", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *rd_miss  = rrddim_add(st, "miss", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *rd_write = rrddim_add(st, "write", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

    RRDSET *st1 = rrdset_create_localhost(
        "netdata", "SQLiteQueries", NULL, "SQLite", NULL, "SQLite metrics R/W", "Metrics", "sqlite.plugin", NULL,
        20001, SQLITE_STATS_UPDATE_EVERY, RRDSET_TYPE_LINE);
    RRDDIM *rd_metrics_write = rrddim_add(st1, "generated", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_metrics_read = rrddim_add(st1, "read db", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_in_memory_metrics_read = rrddim_add(st1, "read mem", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

    RRDSET *st2 = rrdset_create_localhost(
        "netdata", "SQLitePages", NULL, "SQLite", NULL, "SQLite pages", "DirtyPages", "sqlite.plugin", NULL,
        20002, SQLITE_STATS_UPDATE_EVERY, RRDSET_TYPE_LINE);
    RRDDIM *rd_pages_to_write = rrddim_add(st2, "pending", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

    RRDSET *st3 = rrdset_create_localhost(
        "netdata", "SQLiteDiskPages", NULL, "SQLite", NULL, "SQLite pages", "Database (MiB)", "sqlite.plugin", NULL,
        20003, SQLITE_STATS_UPDATE_EVERY, RRDSET_TYPE_LINE);
    RRDDIM *rd_db_size = rrddim_add(st3, "size", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

    heartbeat_t hb;
    heartbeat_init(&hb);
    usec_t step_ut = SQLITE_STATS_UPDATE_EVERY * USEC_PER_SEC;

//    uint32_t count = 0;
    while (1) {
        heartbeat_next(&hb, step_ut);
        if (netdata_exit)
            break;

        rrdset_next(st);
        rrdset_next(st1);
        rrdset_next(st2);
        rrdset_next(st3);

        int hit, miss, write, dummy;

        sqlite3_db_status(db,SQLITE_DBSTATUS_CACHE_HIT , &hit, &dummy, 1);
        sqlite3_db_status(db,SQLITE_DBSTATUS_CACHE_MISS , &miss, &dummy, 1);
        sqlite3_db_status(db,SQLITE_DBSTATUS_CACHE_WRITE , &write, &dummy, 1);

        rrddim_set_by_pointer(st, rd_hit, hit);
        rrddim_set_by_pointer(st, rd_miss, miss);
        rrddim_set_by_pointer(st, rd_write, write);

        rrddim_set_by_pointer(st1, rd_metrics_read, metrics_read);
        rrddim_set_by_pointer(st1, rd_in_memory_metrics_read, in_memory_metrics_read);

        //metrics_read = 0;
        rrddim_set_by_pointer(st1, rd_metrics_write, metrics_write);
        //metrics_write = 0;
        //rrddim_set_by_pointer(st, rd_avg, error_total / iterations);
        rrddim_set_by_pointer(st2, rd_pages_to_write, sqlite_page_flush_list.page_count);

        rrddim_set_by_pointer(st3, rd_db_size, database_size);

        rrdset_done(st);
        rrdset_done(st1);
        rrdset_done(st2);
        rrdset_done(st3);
    }
    netdata_thread_cleanup_pop(1);
    return NULL;
}

static void sqlite_rotation_main_cleanup(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    info("cleaning up rotation thread");

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void *sqlite_rotation_main(void *ptr)
{
    netdata_thread_cleanup_push(sqlite_rotation_main_cleanup, ptr);

    char *err_msg = NULL;

    uint32_t count = 0;

    uint32_t last_pending_page_inserts = 0;
    int ram_vacuumed = 0;
    uint32_t flush_count = 1;
    netdata_thread_disable_cancelability();
    time_t start_flushing = 0;
    while (1) {
        usleep(SQLITE_MAINT_LOOP_DELAY * USEC_PER_MS);
        if (netdata_exit)
            break;

        if (!db_initialized)
            continue;

//        uv_rwlock_wrlock(&sqlite_flush);
        if (items_to_commit) {
            ram_vacuumed = 0;
            int rc = sqlite3_exec(db, "delete from ram.chart limit 5000;", 0, 0, &err_msg);
            if (rc == SQLITE_OK) {
                items_to_commit = sqlite3_changes(db);
                info("Committed %d charts", items_to_commit);
                if (!items_to_commit) {
                    rc = sqlite3_exec(db, "delete from ram.dimension limit 35000;", 0, 0, &err_msg);
                    if (rc == SQLITE_OK) {
                        items_to_commit = sqlite3_changes(db);
                        info("Committed %d dimensions", items_to_commit);
                    }
                    else {
                        errno = 0;
                        error("SQL error during ram.dimension cleanup : %s", err_msg);
                        sqlite3_free(err_msg);
                    }
                }
            } else {
                errno = 0;
                error("SQL error during ram.chart cleanup : %s", err_msg);
                sqlite3_free(err_msg);
            }
        } else {
            if (!ram_vacuumed) {
                ram_vacuumed = 1;
                if (sqlite3_exec(db, "VACUUM ram;", 0, 0, NULL) != SQLITE_OK)
                    ram_vacuumed = 0;
                else
                    info("No items to commit; RAM VACUUM");
            }
        }

//        if (last_page_count <= sqlite_page_flush_list.page_count)
//            flush_count++;
//        else
//            flush_count = flush_count > 2 ? flush_count - 1 : flush_count;

        if (sqlite_page_flush_list.page_count) {
            if (!start_flushing)
                start_flushing = now_realtime_sec() + 5;
        }
        else
            start_flushing = 0;

        if (start_flushing && start_flushing < time(NULL)) {
            flush_count = sqlite_page_flush_list.page_count / 60;
            if (flush_count < database_flush_transaction_count)
                flush_count = database_flush_transaction_count;
            sqlite_flush_page(flush_count, NULL);
        }

        database_size = sql_database_size();

        uv_mutex_lock(&sqlite_add_page);
        if (database_size > (uint32_t) (sqlite_disk_quota_mb * 0.95))
            last_pending_page_inserts = pending_page_inserts;

        if (last_pending_page_inserts && last_pending_page_inserts == pending_page_inserts) {
            //info("Ending METRIC transaction (last count = %d same as current)", last_pending_page_inserts);
            sqlite3_exec(db_page, "COMMIT TRANSACTION;", 0, 0, &err_msg);
            pending_page_inserts = 0;
        }
        last_pending_page_inserts = pending_page_inserts;

        count++;
        if (count % 3 == 0) // && !pending_page_inserts)
            sql_compact_database(delete_rows);

        uv_mutex_unlock(&sqlite_add_page);
    }
//    info("Writing %u dirty pages to the database due to shutdown", sqlite_page_flush_list.page_count);
//    while (sqlite_page_flush_list.page_count) {
//        sqlite_flush_page(database_flush_transaction_count, NULL);
//    }
//    if (pending_page_inserts) {
//        info("Writing final transactions %u", pending_page_inserts);
//        uv_rwlock_wrlock(&sqlite_add_page);
//        sqlite3_exec(db_page, "COMMIT TRANSACTION;", 0, 0, &err_msg);
//        uv_rwlock_wrunlock(&sqlite_add_page);
//        pending_page_inserts = 0;
//    }
//    info("Done");

    netdata_thread_cleanup_pop(1);
    return NULL;
}

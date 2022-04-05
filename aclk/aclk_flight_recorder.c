// SPDX-License-Identifier: GPL-3.0-or-later

#include <stdarg.h>

#include "database/sqlite/sqlite3.h"

#include "daemon/common.h"

#include "aclk_flight_recorder.h"

sqlite3 *aclk_fl_db = NULL;

#define CONNECTION_HISTORY_COUNT_DEFAULT 10

// to make INSERT and subsequent sqlite3_last_insert_rowid() not race
// across threads, while aclk_new_connection_log is expected to be called
// only from ACLK_Main any Netdata thread can add an event (creating INSERT)
static netdata_mutex_t aclk_fl_db_write_lock = NETDATA_MUTEX_INITIALIZER;
#define DB_MUTEX_LOCK()   netdata_mutex_lock(&aclk_fl_db_write_lock);
#define DB_MUTEX_UNLOCK() netdata_mutex_unlock(&aclk_fl_db_write_lock);

//#ifndef HAVE_C___ATOMIC
static netdata_mutex_t aclk_fl_lock = NETDATA_MUTEX_INITIALIZER;
#define FL_LOCK()   netdata_mutex_lock(&aclk_fl_lock);
#define FL_UNLOCK() netdata_mutex_unlock(&aclk_fl_lock);
//#endif

static const char *aclk_fl_db_init_v1[] = {
    "PRAGMA foreign_keys = ON;"
    "CREATE TABLE IF NOT EXISTS connection(id INTEGER PRIMARY KEY AUTOINCREMENT, uuid TEXT NOT NULL UNIQUE);"
// This is important for how auto cleanup (history) works. Quoting SQLITE3 docs:
// "If the AUTOINCREMENT keyword appears after INTEGER PRIMARY KEY,
// that changes the automatic ROWID assignment algorithm to prevent
// the reuse of ROWIDs over the lifetime of the database. In other words,
// the purpose of AUTOINCREMENT is to prevent the reuse of ROWIDs from previously deleted rows."
    "CREATE TABLE IF NOT EXISTS connection_log(connection_id INTEGER REFERENCES connection(id) ON DELETE CASCADE, log TEXT NOT NULL, time int, event_id int, severity int);"
    "PRAGMA user_version=1;",
    NULL
};

struct aclk_fl {
    uint64_t id;
    char *uuid;
};

struct aclk_fl * volatile aclk_fl_current = NULL;
struct aclk_fl * volatile aclk_fl_previous = NULL;

static int fl_enabled = 0;

static int execute_db_batch(const char *batch[])
{
    int rc;
    char *err_msg = NULL;
    for (int i = 0; batch[i]; i++) {
        debug(D_METADATALOG, "Executing %s", batch[i]);
        rc = sqlite3_exec(aclk_fl_db, batch[i], 0, 0, &err_msg);
        if (rc != SQLITE_OK) {
            error_report("SQLite error during database init, rc = %d (%s)", rc, err_msg);
            error_report("SQLite failed statement %s", batch[i]);
            sqlite3_free(err_msg);
            if (SQLITE_CORRUPT == rc) {
                error_report("SQLITE_CORRUPT");
            }
            return 1;
        }
    }
    return 0;
}

#define STMT_GET_MAX_IDX "select MAX(id) from connection;"
static int get_max_id(void)
{
    sqlite3_stmt *res = NULL;
    int max_id = -1;

    int rc = sqlite3_prepare_v2(aclk_fl_db, STMT_GET_MAX_IDX, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to get MAX(id) of connection record");
        return -1;
    }

    while (sqlite3_step(res) == SQLITE_ROW)
        max_id = (uint32_t) sqlite3_column_int(res, 0);

    if (max_id < 0)
        error_report("Failed to get MAX(id) of connection record");

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement trying to fetch MAX(id) of connection record, rc = %d", rc);

    return max_id;
}

#define STMT_DELETE_OLD_RECORDS "delete from connection where id<@id;"
static void delete_old_records(int id_less_than)
{
    sqlite3_stmt *res = NULL;

    int rc = sqlite3_prepare_v2(aclk_fl_db, STMT_DELETE_OLD_RECORDS, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to delete old connection records");
        return;
    }
    rc = sqlite3_bind_int(res, 1, id_less_than);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind id to statement to delete old connection records");
        goto fin;
    }

    // do we need to retry? we will just clean up next time
    rc = sqlite3_step(res);
    if (rc != SQLITE_DONE)
        error_report("Failed to execute statement to delete old connection records");

fin:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement trying to delete old connection records, rc = %d", rc);
}

static void aclk_flight_recorder_cleanup(void)
{
    int max_id = get_max_id();

    if (max_id <= 0)
        return;

    int keep_history = config_get_number(CONFIG_SECTION_CLOUD, "connection recorder history", CONNECTION_HISTORY_COUNT_DEFAULT);
    if (max_id - keep_history <= 0)
        return;

    delete_old_records(max_id - keep_history + 1);
}

int aclk_flight_recorder_init()
{
    char sqlite_database[FILENAME_MAX + 1];
    int rc;

    fl_enabled = config_get_boolean(CONFIG_SECTION_CLOUD, "connection recorder enabled", CONFIG_BOOLEAN_NO);
    if (!fl_enabled)
        return 0;

    snprintfz(sqlite_database, FILENAME_MAX, "%s/aclk-flight-recorder.db", netdata_configured_cache_dir);
    info("SQLite database %s initialization", sqlite_database);

    rc = sqlite3_open(sqlite_database, &aclk_fl_db);
    if (rc != SQLITE_OK) {
        error_report("Failed to initialize database at %s, due to \"%s\"", sqlite_database, sqlite3_errstr(rc));
        sqlite3_close(aclk_fl_db);
        aclk_fl_db = NULL;
        return 1;
    }

    execute_db_batch(aclk_fl_db_init_v1);

    info("SQLite database %s cleanup on startup", sqlite_database);

    aclk_flight_recorder_cleanup();

    return 0;
}

#define STMT_NEW_CONN_INSERT "INSERT INTO connection (uuid) values (@uuid);"
void aclk_new_connection_log()
{
    int rc;
    uuid_t uuid;
    sqlite3_stmt *res = NULL;
    struct aclk_fl *fl;

    if (!fl_enabled)
        return;

    // do this before creating new record
    // as we want to keep configured history + active connection
    aclk_flight_recorder_cleanup();

    FL_LOCK();
    freez(aclk_fl_previous);
    aclk_fl_previous = aclk_fl_current;
    aclk_fl_current = NULL;
    FL_UNLOCK();

    fl = callocz(1, sizeof(struct aclk_fl) + UUID_STR_LEN);
    fl->uuid = ((char*)fl) + sizeof(struct aclk_fl);

    uuid_generate(uuid);
    uuid_unparse_lower(uuid, fl->uuid);

    DB_MUTEX_LOCK();
    rc = sqlite3_prepare_v2(aclk_fl_db, STMT_NEW_CONN_INSERT, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement create new ACLK flight recorder");
        goto fail;
    }

    rc = sqlite3_bind_text(res, 1, fl->uuid, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind uuid parameter to store ACLK flight recorder UUID");
        goto fail_bind;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE)) {
        error_report("Failed to create new aclk_connection record in DB, rc = %d", rc);
        goto fail_bind;
    }

    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when creating flight recorder");

    fl->id = sqlite3_last_insert_rowid(aclk_fl_db);
    DB_MUTEX_UNLOCK();

    FL_LOCK();
    aclk_fl_current = fl;
    FL_UNLOCK();

    return;

fail_bind:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when creating flight recorder");
fail:
    DB_MUTEX_UNLOCK();
    freez(fl);
}

#define STMT_LOG_EVENT "INSERT INTO connection_log (connection_id, event_id, severity, log, time) values (@fk_id, @event_id, @severity, @log, strftime('%s','now'));"
void aclk_store_event(aclk_event_log_t event_id, int severity, const char *log)
{
    int rc;
    sqlite3_stmt *res = NULL;

    if (!fl_enabled)
        return;

    FL_LOCK();
    if (unlikely(aclk_fl_current == NULL)) {
        FL_UNLOCK();
        error_report("Failed to log event. No connection context");
        return;
    }
    uint64_t conn_id = aclk_fl_current->id;
    FL_UNLOCK();

    DB_MUTEX_LOCK();
    rc = sqlite3_prepare_v2(aclk_fl_db, STMT_LOG_EVENT, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement create new ACLK event");
        return;
    }

    rc = sqlite3_bind_int64(res, 1, conn_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind conn_id parameter to store ACLK event");
        goto finalize;
    }

    rc = sqlite3_bind_int64(res, 2, event_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind event_id parameter to store ACLK event");
        goto finalize;
    }

    rc = sqlite3_bind_int64(res, 3, severity);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind severity parameter to store ACLK event");
        goto finalize;
    }

    rc = sqlite3_bind_text(res, 4, log, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind log parameter to store ACLK event");
        goto finalize;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE)) {
        error_report("Failed to insert new ACLK event record in DB, rc = %d", rc);
    }

finalize:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize the prepared statement for ACLK event");

    DB_MUTEX_UNLOCK();
}

void aclk_log_info(aclk_event_log_t event_id, const char *file, const char *function, const unsigned long line, const char *fmt, ...)
{
    va_list args;
    BUFFER *buf = buffer_create(1024);

    va_start( args, fmt );
    buffer_vsprintf(buf, fmt, args);
    va_end( args );

    info_int(file, function, line, "%s", buffer_tostring(buf));
    aclk_store_event(event_id, 0, buffer_tostring(buf));
}

void aclk_log_error(aclk_event_log_t event_id, const char *file, const char *function, const unsigned long line, const char *fmt, ...)
{
    va_list args;
    BUFFER *buf = buffer_create(1024);

    va_start( args, fmt );
    buffer_vsprintf(buf, fmt, args);
    va_end( args );

    errno = 0;
    error_int("ERROR", file, function, line, "%s", buffer_tostring(buf));
    aclk_store_event(event_id, 5, buffer_tostring(buf));
}

void aclk_log(aclk_event_log_t event_id, const char *file, const char *function, const unsigned long line, const char *fmt, ...)
{
    va_list args;
    BUFFER *buf = buffer_create(1024);

    va_start( args, fmt );
    buffer_vsprintf(buf, fmt, args);
    va_end ( args );

    int sev;
    if (aclk_evt_is_error(event_id)) {
        sev = 5;
        errno = 0;
        error_int("ERROR", file, function, line, "%s", buffer_tostring(buf));
    } else {
        sev = 0;
        info_int(file, function, line, "%s", buffer_tostring(buf));
    }

    aclk_store_event(event_id, sev, buffer_tostring(buf));
}

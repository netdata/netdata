// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_metadata.h"
#include "database/sqlite/vendored/sqlite3recover.h"
#include "health/health-alert-entry.h"

#include "sqlite_db_migration.h"

#define DB_METADATA_VERSION 18

#define COMPUTE_DURATION(var_name, unit, start, end)      \
    char var_name[64];                                    \
    duration_snprintf(var_name, sizeof(var_name),         \
                      (int64_t)((end) - (start)), unit, true)

#define SHUTDOWN_REQUESTED(config) (__atomic_load_n(&(config)->shutdown_requested, __ATOMIC_RELAXED))

extern long long def_journal_size_limit;

const char *database_config[] = {
    "CREATE TABLE IF NOT EXISTS host(host_id BLOB PRIMARY KEY, hostname TEXT NOT NULL, "
    "registry_hostname TEXT NOT NULL default 'unknown', update_every INT NOT NULL default 1, "
    "os TEXT NOT NULL default 'unknown', timezone TEXT NOT NULL default 'unknown', tags TEXT NOT NULL default '',"
    "hops INT NOT NULL DEFAULT 0,"
    "memory_mode INT DEFAULT 0, abbrev_timezone TEXT DEFAULT '', utc_offset INT NOT NULL DEFAULT 0,"
    "program_name TEXT NOT NULL DEFAULT 'unknown', program_version TEXT NOT NULL DEFAULT 'unknown', "
    "entries INT NOT NULL DEFAULT 0,"
    "health_enabled INT NOT NULL DEFAULT 0, last_connected INT NOT NULL DEFAULT 0)",

    "CREATE TABLE IF NOT EXISTS chart(chart_id blob PRIMARY KEY, host_id blob, type text, id text, name text, "
    "family text, context text, title text, unit text, plugin text, module text, priority int, update_every int, "
    "chart_type int, memory_mode int, history_entries)",

    "CREATE TABLE IF NOT EXISTS dimension(dim_id blob PRIMARY KEY, chart_id blob, id text, name text, "
    "multiplier int, divisor int , algorithm int, options text)",

    "CREATE TABLE IF NOT EXISTS metadata_migration(filename text, file_size, date_created int)",

    "CREATE TABLE IF NOT EXISTS chart_label(chart_id blob, source_type int, label_key text, "
    "label_value text, date_created int, PRIMARY KEY (chart_id, label_key))",

    "CREATE TRIGGER IF NOT EXISTS del_chart_label AFTER DELETE ON chart "
    "BEGIN DELETE FROM chart_label WHERE chart_id = old.chart_id; END",

    "CREATE TRIGGER IF NOT EXISTS del_chart "
    "AFTER DELETE ON dimension "
    "FOR EACH ROW "
    "BEGIN"
    "  DELETE FROM chart WHERE chart_id = OLD.chart_id "
    "  AND NOT EXISTS (SELECT 1 FROM dimension WHERE chart_id = OLD.chart_id);"
    "END",

    "CREATE TABLE IF NOT EXISTS node_instance (host_id blob PRIMARY KEY, claim_id, node_id, date_created)",

    "CREATE TABLE IF NOT EXISTS alert_hash(hash_id blob PRIMARY KEY, date_updated int, alarm text, template text, "
    "on_key text, class text, component text, type text, os text, hosts text, lookup text, "
    "every text, units text, calc text, families text, plugin text, module text, charts text, green text, "
    "red text, warn text, crit text, exec text, to_key text, info text, delay text, options text, "
    "repeat text, host_labels text, p_db_lookup_dimensions text, p_db_lookup_method text, p_db_lookup_options int, "
    "p_db_lookup_after int, p_db_lookup_before int, p_update_every int, source text, chart_labels text, "
    "summary text, time_group_condition INT, time_group_value DOUBLE, dims_group INT, data_source INT)",

    "CREATE TABLE IF NOT EXISTS host_info(host_id blob, system_key text NOT NULL, system_value text NOT NULL, "
    "date_created INT, PRIMARY KEY(host_id, system_key))",

    "CREATE TABLE IF NOT EXISTS host_label(host_id blob, source_type int, label_key text NOT NULL, "
    "label_value text NOT NULL, date_created INT, PRIMARY KEY (host_id, label_key))",

    "CREATE TRIGGER IF NOT EXISTS ins_host AFTER INSERT ON host BEGIN INSERT INTO node_instance (host_id, date_created)"
    " SELECT new.host_id, unixepoch() WHERE new.host_id NOT IN (SELECT host_id FROM node_instance); END",

    "CREATE TABLE IF NOT EXISTS health_log (health_log_id INTEGER PRIMARY KEY, host_id blob, alarm_id int, "
    "config_hash_id blob, name text, chart text, family text, recipient text, units text, exec text, "
    "chart_context text, last_transition_id blob, chart_name text, UNIQUE (host_id, alarm_id))",

    "CREATE TABLE IF NOT EXISTS health_log_detail (health_log_id int, unique_id int, alarm_id int, alarm_event_id int, "
    "updated_by_id int, updates_id int, when_key int, duration int, non_clear_duration int, "
    "flags int, exec_run_timestamp int, delay_up_to_timestamp int, "
    "info text, exec_code int, new_status real, old_status real, delay int, "
    "new_value double, old_value double, last_repeat int, transition_id blob, global_id int, summary text)",

    "CREATE INDEX IF NOT EXISTS ind_d2 on dimension (chart_id)",
    "CREATE INDEX IF NOT EXISTS ind_c3 on chart (host_id)",
    "CREATE INDEX IF NOT EXISTS health_log_ind_1 ON health_log (host_id)",
    "CREATE INDEX IF NOT EXISTS health_log_d_ind_2 ON health_log_detail (global_id)",
    "CREATE INDEX IF NOT EXISTS health_log_d_ind_3 ON health_log_detail (transition_id)",
    "CREATE INDEX IF NOT EXISTS health_log_d_ind_9 ON health_log_detail (unique_id DESC, health_log_id)",
    "CREATE INDEX IF NOT EXISTS health_log_d_ind_6 on health_log_detail (health_log_id, when_key)",
    "CREATE INDEX IF NOT EXISTS health_log_d_ind_7 on health_log_detail (alarm_id)",
    "CREATE INDEX IF NOT EXISTS health_log_d_ind_8 on health_log_detail (new_status, updated_by_id)",

    "CREATE TABLE IF NOT EXISTS agent_event_log (id INTEGER PRIMARY KEY, version TEXT, event_type INT, value, date_created INT)",
    "CREATE INDEX IF NOT EXISTS idx_agent_event_log1 on agent_event_log (event_type)",

    "CREATE TABLE IF NOT EXISTS alert_queue "
    " (host_id BLOB, health_log_id INT, unique_id INT, alarm_id INT, status INT, date_scheduled INT, "
    " UNIQUE(host_id, health_log_id, alarm_id))",

    "CREATE INDEX IF NOT EXISTS ind_alert_queue1 ON alert_queue(host_id, date_scheduled)",

    "CREATE TABLE IF NOT EXISTS alert_version (health_log_id INTEGER PRIMARY KEY, unique_id INT, status INT, "
    "version INT, date_submitted INT)",

    "CREATE TABLE IF NOT EXISTS aclk_queue (sequence_id INTEGER PRIMARY KEY, host_id blob, health_log_id INT, "
    "unique_id INT, date_created INT,  UNIQUE(host_id, health_log_id))",

    "CREATE TABLE IF NOT EXISTS ctx_metadata_cleanup (id INTEGER PRIMARY KEY, host_id BLOB, context TEXT NOT NULL, date_created INT NOT NULL, "
    "UNIQUE (host_id, context))",

    NULL
};

const char *database_cleanup[] = {
    "DELETE FROM host WHERE host_id NOT IN (SELECT host_id FROM chart)",
    "DELETE FROM node_instance WHERE host_id NOT IN (SELECT host_id FROM host)",
    "DELETE FROM host_info WHERE host_id NOT IN (SELECT host_id FROM host)",
    "DELETE FROM host_label WHERE host_id NOT IN (SELECT host_id FROM host)",
    "DELETE FROM ctx_metadata_cleanup WHERE host_id NOT IN (SELECT host_id FROM host)",
    "DROP TRIGGER IF EXISTS tr_dim_del",
    "DROP INDEX IF EXISTS ind_d1",
    "DROP INDEX IF EXISTS ind_c1",
    "DROP INDEX IF EXISTS ind_c2",
    "DROP INDEX IF EXISTS alert_hash_index",
    "DROP INDEX IF EXISTS health_log_d_ind_4",
    "DROP INDEX IF EXISTS health_log_d_ind_1",
    "DROP INDEX IF EXISTS health_log_d_ind_5",
    NULL
};

sqlite3 *db_meta = NULL;

// SQL statements

#define SQL_CLEANUP_AGENT_EVENT_LOG "DELETE FROM agent_event_log WHERE date_created < UNIXEPOCH() - 30 * 86400"

#define SQL_DELETE_ORPHAN_HEALTH_LOG "DELETE FROM health_log WHERE host_id NOT IN (SELECT host_id FROM host)"

#define SQL_DELETE_ORPHAN_HEALTH_LOG_DETAIL                                                                            \
    "DELETE FROM health_log_detail WHERE health_log_id NOT IN (SELECT health_log_id FROM health_log)"

#define SQL_DELETE_ORPHAN_ALERT_VERSION                                                                                \
    "DELETE FROM alert_version WHERE health_log_id NOT IN (SELECT health_log_id FROM health_log)"

#define SQL_STORE_CLAIM_ID                                                                                             \
    "INSERT INTO node_instance "                                                                                       \
    "(host_id, claim_id, date_created) VALUES (@host_id, @claim_id, UNIXEPOCH()) "                                     \
    "ON CONFLICT(host_id) DO UPDATE SET claim_id = excluded.claim_id"

#define SQL_DELETE_HOST_LABELS  "DELETE FROM host_label WHERE host_id = @uuid"

#define STORE_HOST_LABEL                                                                                               \
    "INSERT INTO host_label (host_id, source_type, label_key, label_value, date_created) VALUES "

#define STORE_CHART_LABEL                                                                                              \
    "INSERT INTO chart_label (chart_id, source_type, label_key, label_value, date_created) VALUES "

#define STORE_HOST_OR_CHART_LABEL_VALUE "(u2h('%s'), %d,'%s','%s', unixepoch())"

#define DELETE_DIMENSION_UUID   "DELETE FROM dimension WHERE dim_id = @uuid"

#define SQL_STORE_HOST_INFO                                                                                              \
    "INSERT OR REPLACE INTO host (host_id, hostname, registry_hostname, update_every, os, timezone, tags, hops, "        \
    "memory_mode, abbrev_timezone, utc_offset, program_name, program_version, entries, health_enabled, last_connected) " \
    "VALUES (@host_id, @hostname, @registry_hostname, @update_every, @os, @timezone, @tags, @hops, "                     \
    "@memory_mode, @abbrev_tz, @utc_offset, @prog_name, @prog_version, @entries, @health_enabled, @last_connected)"

#define SQL_STORE_CHART                                                                                                \
    "INSERT INTO chart (chart_id, host_id, type, id, name, family, context, title, unit, plugin, module, priority, "   \
    "update_every, chart_type, memory_mode, history_entries) "                                                         \
    "values (@chart_id, @host_id, @type, @id, @name, @family, @context, @title, @unit, @plugin, @module, @priority, "  \
    "@update_every, @chart_type, @memory_mode, @history_entries) "                                                     \
    "ON CONFLICT(chart_id) DO UPDATE SET type=excluded.type, id=excluded.id, name=excluded.name, "                     \
    "family=excluded.family, context=excluded.context, title=excluded.title, unit=excluded.unit, "                     \
    "plugin=excluded.plugin, module=excluded.module, priority=excluded.priority, update_every=excluded.update_every, " \
    "chart_type=excluded.chart_type, memory_mode = excluded.memory_mode, history_entries = excluded.history_entries"

#define SQL_STORE_DIMENSION                                                                                            \
    "INSERT INTO dimension (dim_id, chart_id, id, name, multiplier, divisor , algorithm, options) "                    \
    "VALUES (@dim_id, @chart_id, @id, @name, @multiplier, @divisor, @algorithm, @options) "                            \
    "ON CONFLICT(dim_id) DO UPDATE SET id=excluded.id, name=excluded.name, multiplier=excluded.multiplier, "           \
    "divisor=excluded.divisor, algorithm=excluded.algorithm, options=excluded.options"

#define SELECT_DIMENSION_LIST "SELECT dim_id, rowid FROM dimension WHERE rowid > @row_id"
#define SELECT_CHART_LIST "SELECT chart_id, rowid FROM chart WHERE rowid > @row_id"
#define SELECT_CHART_LABEL_LIST "SELECT chart_id, rowid FROM chart_label WHERE rowid > @row_id"

#define SQL_STORE_HOST_SYSTEM_INFO_VALUES                                                                              \
    "INSERT OR REPLACE INTO host_info (host_id, system_key, system_value, date_created) VALUES "                       \
    "(@uuid, @name, @value, UNIXEPOCH())"

#define CONVERT_EXISTING_LOCALHOST "UPDATE host SET hops = 1 WHERE hops = 0 AND host_id <> @host_id"
#define DELETE_MISSING_NODE_INSTANCES "DELETE FROM node_instance WHERE host_id NOT IN (SELECT host_id FROM host)"

#define METADATA_MAINTENANCE_FIRST_CHECK (1800)     // Maintenance first run after agent startup in seconds
#define METADATA_MAINTENANCE_REPEAT (60)            // Repeat if last run for dimensions, charts, labels needs more work
#define METADATA_MAINTENANCE_CTX_CLEAN_REPEAT (300) // Repeat if last run for dimensions, charts, labels needs more work
#define METADATA_HEALTH_LOG_INTERVAL (3600)         // Repeat maintenance for health
#define METADATA_LABEL_CHECK_INTERVAL (3600)        // Repeat maintenance for labels
#define METADATA_RUNTIME_THRESHOLD (5)              // Run time threshold for cleanup task

#define METADATA_HOST_CHECK_FIRST_CHECK (5)         // First check for pending metadata
#define METADATA_HOST_CHECK_INTERVAL (5)            // Repeat check for pending metadata
#define METADATA_MAX_BATCH_SIZE (64)                // Maximum commands to execute before running the event loop

#define DATABASE_VACUUM_FREQUENCY_SECONDS (60)
#define DATABASE_FREE_PAGES_THRESHOLD_PC (5)        // Percentage of free pages to trigger vacuum
#define DATABASE_FREE_PAGES_VACUUM_PC (10)          // Percentage of free pages to vacuum

enum metadata_opcode {
    METADATA_DATABASE_NOOP = 0,
    METADATA_DEL_DIMENSION,
    METADATA_STORE_CLAIM_ID,
    METADATA_STORE,
    METADATA_LOAD_HOST_CONTEXT,
    METADATA_ADD_HOST_AE,
    METADATA_DEL_HOST_AE,
    METADATA_ADD_CTX_CLEANUP,
    METADATA_EXECUTE_STORE_STATEMENT,
    METADATA_SYNC_SHUTDOWN,
    METADATA_UNITTEST,
    // leave this last
    // we need it to check for worker utilization
    METADATA_MAX_ENUMERATIONS_DEFINED
};

struct meta_config_s {
    ND_THREAD *thread;
    uv_loop_t loop;
    uv_async_t async;
    uv_timer_t timer_req;
    time_t metadata_check_after;
    Pvoid_t ae_DelJudyL;
    bool initialized;
    bool ctx_load_running;
    bool metadata_running;
    bool store_metadata;
    bool shutdown_requested;
    struct completion start_stop_complete;
    CmdPool cmd_pool;
    WorkerPool worker_pool;
} meta_config;

//
// For unittest
//
struct thread_unittest {
    int join;
    unsigned added;
    unsigned processed;
    unsigned *done;
};

int sql_metadata_cache_stats(int op)
{
    int count, dummy;

    sqlite3_db_status(db_meta, op, &count, &dummy, 0);
    return count;
}

static inline void set_host_node_id(RRDHOST *host, nd_uuid_t *node_id)
{
    if (unlikely(!host))
        return;

    if (unlikely(!node_id)) {
        host->node_id = UUID_ZERO;
        return;
    }

    struct aclk_sync_cfg_t *aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_RELAXED);

    uuid_copy(host->node_id.uuid, *node_id);

    if (unlikely(!aclk_host_config))
        create_aclk_config(host, &host->host_id.uuid, node_id);
    else
        uuid_unparse_lower(*node_id, aclk_host_config->node_id);

    stream_receiver_send_node_and_claim_id_to_child(host);
    stream_path_node_id_updated(host);
}

struct host_ctx_cleanup_s {
    nd_uuid_t host_uuid;
    STRING *context;
};

#define CTX_DELETE_CONTEXT_META_CLEANUP_ITEM "DELETE FROM ctx_metadata_cleanup WHERE host_id = @host_id AND context = @context"

static void ctx_delete_metadata_cleanup_context(sqlite3_stmt **res, nd_uuid_t *host_uuid, const char *context)
{
    if (!*res) {
        if (!PREPARE_STATEMENT(db_meta, CTX_DELETE_CONTEXT_META_CLEANUP_ITEM, res))
            return;
    }

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(*res, ++param, host_uuid, sizeof(*host_uuid), SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_text(*res, ++param, context, -1, SQLITE_STATIC));

    param = 0;
    int rc = sqlite3_step_monitored(*res);
    if (rc != SQLITE_DONE)
        error_report("Failed to delete context check entry, rc = %d", rc);

done:
    REPORT_BIND_FAIL(*res, param);
    SQLITE_RESET(*res);
}

#define CTX_GET_CONTEXT_META_CLEANUP_LIST "SELECT context FROM ctx_metadata_cleanup WHERE host_id = @host_id"

static void ctx_get_context_list_to_cleanup(nd_uuid_t *host_uuid, void (*cleanup_cb)(Pvoid_t JudyL, void *data), void *data)
{
    if (unlikely(!host_uuid))
        return;

    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_meta, CTX_GET_CONTEXT_META_CLEANUP_LIST, &res))
        return;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, host_uuid, sizeof(*host_uuid), SQLITE_STATIC));
    param = 0;

    const char *context;
    Pvoid_t CTX_JudyL = NULL;
    Pvoid_t *Pvalue;
    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        context = (char *) sqlite3_column_text(res, 0);
        STRING *ctx = string_strdupz(context);
        Pvalue = JudyLIns(&CTX_JudyL, (Word_t) ctx, PJE0);
        if (*Pvalue)
            string_freez(ctx);
        else
            *(int *)Pvalue = 1;
    }

    if (CTX_JudyL) {
        cleanup_cb(CTX_JudyL, data);

        bool first = true;
        Word_t Index = 0;
        while ((Pvalue = JudyLFirstThenNext(CTX_JudyL, &Index, &first))) {
            STRING *ctx = (STRING *) Index;
            string_freez(ctx);
        }
    }
    (void)JudyLFreeArray(&CTX_JudyL, PJE0);

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
}

#define SQL_SCHEDULE_HOST_CTX_CLEANUP                                                                                  \
    "INSERT INTO ctx_metadata_cleanup (host_id, context, date_created) "                                               \
    "VALUES (@host_id, @context, UNIXEPOCH()) ON CONFLICT DO UPDATE SET date_created = excluded.date_created; END"

// Schedule context cleanup for host
static void sql_schedule_host_ctx_cleanup(sqlite3_stmt **res, nd_uuid_t *host_id, const char *context)
{
    if (!*res) {
        if (!PREPARE_STATEMENT(db_meta, SQL_SCHEDULE_HOST_CTX_CLEANUP, res))
            return;
    }

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(*res, ++param, host_id, sizeof(*host_id), SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_text(*res, ++param, context, -1, SQLITE_STATIC));

    param = 0;
    int rc = execute_insert(*res);
    if (rc != SQLITE_DONE)
        error_report("Failed to host context check data, rc = %d", rc);
done:
    REPORT_BIND_FAIL(*res, param);
    SQLITE_RESET(*res);
}

#define SQL_SET_HOST_LABEL                                                                                             \
    "INSERT INTO host_label (host_id, source_type, label_key, label_value, date_created) "                             \
    "VALUES (@host_id, @source_type, @label_key, @label_value, UNIXEPOCH()) ON CONFLICT (host_id, label_key) "         \
    " DO UPDATE SET source_type = excluded.source_type, label_value=excluded.label_value, date_created=UNIXEPOCH()"

bool sql_set_host_label(nd_uuid_t *host_id, const char *label_key, const char *label_value)
{
    sqlite3_stmt *res = NULL;
    bool status = false;

    if (!label_key || !label_value || !host_id)
        return false;

    if (!PREPARE_STATEMENT(db_meta, SQL_SET_HOST_LABEL, &res))
        return 1;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, host_id, sizeof(*host_id), SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int(res, ++param, RRDLABEL_SRC_AUTO));
    SQLITE_BIND_FAIL(done, sqlite3_bind_text(res, ++param, label_key, -1, SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_text(res, ++param, label_value, -1, SQLITE_STATIC));

    param = 0;
    int rc = sqlite3_step_monitored(res);
    status = (rc == SQLITE_DONE);
    if (false == status)
        error_report("Failed to store node instance information, rc = %d", rc);
done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
    return status;
}

#define SQL_UPDATE_NODE_ID  "UPDATE node_instance SET node_id = @node_id WHERE host_id = @host_id"

void sql_update_node_id(nd_uuid_t *host_id, nd_uuid_t *node_id)
{
    sqlite3_stmt *res = NULL;
    RRDHOST *host = NULL;

    char host_guid[GUID_LEN + 1];
    uuid_unparse_lower(*host_id, host_guid);
    rrd_wrlock();
    host = rrdhost_find_by_guid(host_guid);
    if (likely(host))
        set_host_node_id(host, node_id);
    rrd_wrunlock();

    if (!PREPARE_STATEMENT(db_meta, SQL_UPDATE_NODE_ID, &res))
        return;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, node_id, sizeof(*node_id), SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, host_id, sizeof(*host_id), SQLITE_STATIC));

    param = 0;
    int rc = sqlite3_step_monitored(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to store node instance information, rc = %d", rc);

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
}

#define SQL_INVALIDATE_NODE_INSTANCES                                                                                  \
    "UPDATE node_instance SET node_id = NULL WHERE EXISTS "                                                            \
    "(SELECT host_id FROM node_instance WHERE host_id = @host_id AND (@claim_id IS NULL OR claim_id <> @claim_id))"

void invalidate_node_instances(nd_uuid_t *host_id, nd_uuid_t *claim_id)
{
    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_meta, SQL_INVALIDATE_NODE_INSTANCES, &res))
        return;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, host_id, sizeof(*host_id), SQLITE_STATIC));

    if (claim_id)
        SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, claim_id, sizeof(*claim_id), SQLITE_STATIC));
    else
        SQLITE_BIND_FAIL(done, sqlite3_bind_null(res, ++param));

    param = 0;
    int rc = sqlite3_step_monitored(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to invalidate node instance information, rc = %d", rc);

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
}

#define SQL_GET_HOST_NODE_ID "SELECT node_id FROM node_instance WHERE host_id = @host_id"

void sql_load_node_id(RRDHOST *host)
{
    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_meta, SQL_GET_HOST_NODE_ID, &res))
        return;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, &host->host_id.uuid, sizeof(host->host_id.uuid), SQLITE_STATIC));

    param = 0;
    int rc = sqlite3_step_monitored(res);
    if (likely(rc == SQLITE_ROW)) {
        if (likely(sqlite3_column_bytes(res, 0) == sizeof(nd_uuid_t)))
            set_host_node_id(host, (nd_uuid_t *)sqlite3_column_blob(res, 0));
        else
            set_host_node_id(host, NULL);
    }

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
}

#define SELECT_HOST_INFO "SELECT system_key, system_value FROM host_info WHERE host_id = @host_id"

void sql_build_host_system_info(nd_uuid_t *host_id, struct rrdhost_system_info *system_info)
{
    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_meta, SELECT_HOST_INFO, &res))
        return;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, host_id, sizeof(*host_id), SQLITE_STATIC));

    param = 0;
    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        rrdhost_system_info_set_by_name(
            system_info, (char *)sqlite3_column_text(res, 0), (char *)sqlite3_column_text(res, 1));
    }

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
}

#define SELECT_HOST_LABELS "SELECT label_key, label_value, source_type FROM host_label WHERE host_id = @host_id " \
    "AND label_key IS NOT NULL AND label_value IS NOT NULL"

RRDLABELS *sql_load_host_labels(nd_uuid_t *host_id)
{
    RRDLABELS *labels = NULL;
    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_meta, SELECT_HOST_LABELS, &res))
        return NULL;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, host_id, sizeof(*host_id), SQLITE_STATIC));

    param = 0;
    labels = rrdlabels_create();

    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        rrdlabels_add(
            labels,
            (const char *)sqlite3_column_text(res, 0),
            (const char *)sqlite3_column_text(res, 1),
            sqlite3_column_int(res, 2));
    }

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
    return labels;
}

static int exec_statement_with_uuid(const char *sql, nd_uuid_t *uuid)
{
    int result = 1;
    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_meta, sql, &res)) {
        error_report("Failed to prepare statement %s", sql);
        return 1;
    }

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, uuid, sizeof(*uuid), SQLITE_STATIC));

    param = 0;
    int rc = sqlite3_step_monitored(res);
    if (likely(rc == SQLITE_DONE))
        result = SQLITE_OK;
    else
        error_report("Failed to execute %s, rc = %d", sql, rc);

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
    return result;
}

static void recover_database(const char *sqlite_database, const char *new_sqlite_database)
{
    sqlite3 *database;
    int rc = sqlite3_open(sqlite_database, &database);
    if (rc != SQLITE_OK)
        return;

    netdata_log_info("Recover %s", sqlite_database);
    netdata_log_info("     to %s", new_sqlite_database);

    // This will remove the -shm and -wal files when we close the database
    (void)db_execute(database, "select count(*) from sqlite_master limit 0", NULL);

    sqlite3_recover *recover = sqlite3_recover_init(database, "main", new_sqlite_database);
    if (recover) {

        rc = sqlite3_recover_run(recover);

        if (rc == SQLITE_OK)
            netdata_log_info("Recover complete");
        else
            netdata_log_error("Recover encountered an error but the database may be usable");

        rc = sqlite3_recover_finish(recover);

        (void) sqlite3_close(database);

        if (rc == SQLITE_OK) {
            rc = rename(new_sqlite_database, sqlite_database);
            if (rc == 0) {
                netdata_log_info("Renamed %s", new_sqlite_database);
                netdata_log_info("     to %s", sqlite_database);
            }
        }
        else
            netdata_log_error("Recover failed to free resources");
    }
    else
        (void) sqlite3_close(database);
}


static void sqlite_uuid_parse(sqlite3_context *context, int argc, sqlite3_value **argv)
{
    nd_uuid_t  uuid;

    if ( argc != 1 ){
        sqlite3_result_null(context);
        return ;
    }
    int rc = uuid_parse((const char *) sqlite3_value_text(argv[0]), uuid);
    if (rc == -1)  {
        sqlite3_result_null(context);
        return ;
    }

    sqlite3_result_blob(context, &uuid, sizeof(nd_uuid_t), SQLITE_TRANSIENT);
}

void sqlite_now_usec(sqlite3_context *context, int argc, sqlite3_value **argv)
{
    if (argc != 1 ){
        sqlite3_result_null(context);
        return ;
    }

    if (sqlite3_value_int(argv[0]) != 0) {
        struct timespec req = {.tv_sec = 0, .tv_nsec = 1};
        nanosleep(&req, NULL);
    }

    sqlite3_result_int64(context, (sqlite_int64) now_realtime_usec());
}

void sqlite_uuid_random(sqlite3_context *context, int argc, sqlite3_value **argv)
{
    (void)argc;
    (void)argv;

    nd_uuid_t uuid;
    uuid_generate_random(uuid);
    sqlite3_result_blob(context, &uuid, sizeof(nd_uuid_t), SQLITE_TRANSIENT);
}

static int64_t sql_get_wal_size(const char *database_file)
{
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, sizeof(filename) - 1, "%s/%s-wal", netdata_configured_cache_dir, database_file);

    uv_fs_t req;
    int result = uv_fs_stat(NULL, &req, filename, NULL);
    int64_t file_size = result >= 0 ? (int64_t) req.statbuf.st_size : -1;

    uv_fs_req_cleanup(&req);
    return file_size;
}

#define SQLITE_METADATA_WAL_LIMIT_X (10)

bool sql_metadata_wal_size_acceptable()
{
    int64_t wal_size = sql_get_wal_size("netdata-meta.db");

    if (wal_size > SQLITE_METADATA_WAL_LIMIT_X * def_journal_size_limit)
        return false;

    return true;
}

// Init
/*
 * Initialize the SQLite database
 * Return 0 on success
 */
int sql_init_meta_database(db_check_action_type_t rebuild, int memory)
{
    char *err_msg = NULL;
    char sqlite_database[FILENAME_MAX + 1];
    int rc;

    if (likely(!memory)) {
        snprintfz(sqlite_database, sizeof(sqlite_database) - 1, "%s/.netdata-meta.db.recover", netdata_configured_cache_dir);
        rc = unlink(sqlite_database);
        snprintfz(sqlite_database, FILENAME_MAX, "%s/netdata-meta.db", netdata_configured_cache_dir);

        if (rc == 0 || (rebuild & DB_CHECK_RECOVER)) {
            char new_sqlite_database[FILENAME_MAX + 1];
            snprintfz(new_sqlite_database, sizeof(new_sqlite_database) - 1, "%s/netdata-meta-recover.db", netdata_configured_cache_dir);
            recover_database(sqlite_database, new_sqlite_database);
            if (rebuild & DB_CHECK_RECOVER)
                return 0;
        }

        snprintfz(sqlite_database, sizeof(sqlite_database) - 1, "%s/.netdata-meta.db.delete", netdata_configured_cache_dir);
        rc = unlink(sqlite_database);
        snprintfz(sqlite_database, FILENAME_MAX, "%s/netdata-meta.db", netdata_configured_cache_dir);
        if (rc == 0) {
            char new_sqlite_database[FILENAME_MAX + 1];
            snprintfz(new_sqlite_database, sizeof(new_sqlite_database) - 1, "%s/netdata-meta.bad", netdata_configured_cache_dir);
            rc = rename(sqlite_database, new_sqlite_database);
            if (rc)
                error_report("Failed to rename %s to %s", sqlite_database, new_sqlite_database);
        }
        // note: sqlite_database contains the right name
    }
    else
        strncpyz(sqlite_database, ":memory:", sizeof(sqlite_database) - 1);

    rc = sqlite3_open(sqlite_database, &db_meta);
    if (rc != SQLITE_OK) {
        error_report("Failed to initialize database at %s, due to \"%s\"", sqlite_database, sqlite3_errstr(rc));
        char *error_str = get_database_extented_error(db_meta, 0, "meta_open");
        if (error_str)
            analytics_set_data_str(&analytics_data.netdata_fail_reason, error_str);
        freez(error_str);
        goto close_database;
    }

    if (rebuild & DB_CHECK_RECLAIM_SPACE) {
        netdata_log_info("Reclaiming space of %s", sqlite_database);
        rc = sqlite3_exec_monitored(db_meta, "VACUUM", 0, 0, &err_msg);
        if (rc != SQLITE_OK) {
            error_report("Failed to execute VACUUM rc = %d (%s)", rc, err_msg);
            sqlite3_free(err_msg);
        }
        else {
            (void)db_execute(db_meta, "select count(*) from sqlite_master limit 0", NULL);
            (void) sqlite3_close(db_meta);
        }
        return 1;
    }

    if (rebuild & DB_CHECK_ANALYZE) {
        errno_clear();
        netdata_log_info("Running ANALYZE on %s", sqlite_database);
        rc = sqlite3_exec_monitored(db_meta, "ANALYZE", 0, 0, &err_msg);
        if (rc != SQLITE_OK) {
            error_report("Failed to execute ANALYZE rc = %d (%s)", rc, err_msg);
            sqlite3_free(err_msg);
        }
        else {
            (void)db_execute(db_meta, "select count(*) from sqlite_master limit 0", NULL);
            (void) sqlite3_close(db_meta);
        }
        return 1;
    }

    errno_clear();
    netdata_log_info("SQLite database %s initialization", sqlite_database);

    rc = sqlite3_create_function(db_meta, "u2h", 1, SQLITE_ANY | SQLITE_DETERMINISTIC, 0, sqlite_uuid_parse, 0, 0);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to register internal u2h function");

    rc = sqlite3_create_function(db_meta, "now_usec", 1, SQLITE_ANY, 0, sqlite_now_usec, 0, 0);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to register internal now_usec function");

    rc = sqlite3_create_function(db_meta, "uuid_random", 0, SQLITE_ANY, 0, sqlite_uuid_random, 0, 0);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to register internal uuid_random function");

    int target_version = DB_METADATA_VERSION;

    if (likely(!memory))
        target_version = perform_database_migration(db_meta, DB_METADATA_VERSION);

    if (configure_sqlite_database(db_meta, target_version, "meta_config"))
        goto close_database;

    if (init_database_batch(db_meta, &database_config[0], "meta_init"))
        goto close_database;

    if (init_database_batch(db_meta, &database_cleanup[0], "meta_cleanup"))
        goto close_database;

    netdata_log_info("SQLite database initialization completed");
    if (sqlite3_busy_timeout(db_meta, SQLITE_BUSY_DELAY_MS) != SQLITE_OK)
        nd_log_daemon(NDLP_WARNING, "SQLITE: Failed to set busy timeout to %d ms", SQLITE_BUSY_DELAY_MS);

    return 0;

close_database:
    sqlite3_close(db_meta);
    db_meta = NULL;
    return 1;
}

// Metadata functions

struct query_build {
    BUFFER *sql;
    int count;
    char uuid_str[UUID_STR_LEN];
};

static int host_label_store_to_sql_callback(const char *name, const char *value, RRDLABEL_SRC ls, void *data) {
    struct query_build *lb = data;
    if (unlikely(!lb->count))
        buffer_sprintf(lb->sql, STORE_HOST_LABEL);
    else
        buffer_strcat(lb->sql, ", ");
    buffer_sprintf(lb->sql, STORE_HOST_OR_CHART_LABEL_VALUE, lb->uuid_str, (int) (ls & ~(RRDLABEL_FLAG_INTERNAL)), name, value);
    lb->count++;
    return 1;
}

static int chart_label_store_to_sql_callback(const char *name, const char *value, RRDLABEL_SRC ls, void *data) {
    struct query_build *lb = data;
    if (unlikely(!lb->count))
        buffer_sprintf(lb->sql, STORE_CHART_LABEL);
    else
        buffer_strcat(lb->sql, ", ");
    buffer_sprintf(lb->sql, STORE_HOST_OR_CHART_LABEL_VALUE, lb->uuid_str, (int) (ls & ~(RRDLABEL_FLAG_INTERNAL)), name, value);
    lb->count++;
    return 1;
}

static int check_and_update_chart_labels(RRDSET *st, BUFFER *work_buffer)
{
    uint32_t old_version = st->rrdlabels_last_saved_version;
    uint32_t new_version = rrdlabels_version(st->rrdlabels);

    if (new_version == old_version)
        return 0;

    struct query_build tmp = {.sql = work_buffer, .count = 0};
    uuid_unparse_lower(st->chart_uuid, tmp.uuid_str);
    rrdlabels_walkthrough_read(st->rrdlabels, chart_label_store_to_sql_callback, &tmp);
    buffer_strcat(work_buffer, " ON CONFLICT (chart_id, label_key) DO UPDATE SET source_type = excluded.source_type, label_value=excluded.label_value, date_created=UNIXEPOCH()");
    int rc = db_execute(db_meta, buffer_tostring(work_buffer), NULL);
    if (likely(!rc))
        st->rrdlabels_last_saved_version = new_version;

    return rc;
}

// If the machine guid has changed, then existing one with hops 0 will be marked as hops 1 (child)
void detect_machine_guid_change(nd_uuid_t *host_uuid)
{
    int rc;

    rc = exec_statement_with_uuid(CONVERT_EXISTING_LOCALHOST, host_uuid);
    if (!rc) {
        if (unlikely(db_execute(db_meta, DELETE_MISSING_NODE_INSTANCES, NULL)))
            error_report("Failed to remove deleted hosts from node instances");
    }
}

static int store_claim_id(nd_uuid_t *host_id, nd_uuid_t *claim_id)
{
    sqlite3_stmt *res = NULL;
    int rc = 0;

    if (!PREPARE_STATEMENT(db_meta, SQL_STORE_CLAIM_ID, &res))
        return 1;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, host_id, sizeof(*host_id), SQLITE_STATIC));

    if (claim_id)
        SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param,claim_id, sizeof(*claim_id), SQLITE_STATIC));
    else
        SQLITE_BIND_FAIL(done, sqlite3_bind_null(res, ++param));

    param = 0;
    rc = sqlite3_step_monitored(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to store host claim id rc = %d", rc);

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
    return rc != SQLITE_DONE;
}

#define SQL_DELETE_DIMENSION_BY_ID   "DELETE FROM dimension WHERE rowid = @dimension_row AND dim_id = @uuid"

static void delete_dimension_by_rowid(sqlite3_stmt **res, int64_t dimension_id, nd_uuid_t *dim_uuid)
{
    if (!*res) {
        if (!PREPARE_STATEMENT(db_meta, SQL_DELETE_DIMENSION_BY_ID, res))
            return;
    }

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(*res, ++param, dimension_id));
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(*res, ++param, dim_uuid, sizeof(*dim_uuid), SQLITE_STATIC));

    param = 0;
    int rc = sqlite3_step_monitored(*res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to delete dimension id, rc = %d", rc);

done:
    REPORT_BIND_FAIL(*res, param);
    SQLITE_RESET(*res);
}

static void delete_dimension_uuid(nd_uuid_t *dimension_uuid, sqlite3_stmt **action_res __maybe_unused, bool flag __maybe_unused)
{
    if(!dimension_uuid)
        return;

    sqlite3_stmt *res = NULL;
    int rc;

    if (!PREPARE_STATEMENT(db_meta, DELETE_DIMENSION_UUID, &res))
        return;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, dimension_uuid, sizeof(*dimension_uuid), SQLITE_STATIC));

    param = 0;
    rc = sqlite3_step_monitored(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to delete dimension uuid, rc = %d", rc);

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
}

//
// Store host and host system info information in the database
static int store_host_metadata(RRDHOST *host)
{
    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_meta, SQL_STORE_HOST_INFO, &res))
        return false;

    int param = 0;
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_blob(res, ++param, &host->host_id.uuid, sizeof(host->host_id.uuid), SQLITE_STATIC));
    SQLITE_BIND_FAIL(bind_fail, bind_text_null(res, ++param, rrdhost_hostname(host), 0));
    SQLITE_BIND_FAIL(bind_fail, bind_text_null(res, ++param, rrdhost_registry_hostname(host), 1));
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_int(res, ++param, host->rrd_update_every));
    SQLITE_BIND_FAIL(bind_fail, bind_text_null(res, ++param, rrdhost_os(host), 1));
    SQLITE_BIND_FAIL(bind_fail, bind_text_null(res, ++param, rrdhost_timezone(host), 1));
    SQLITE_BIND_FAIL(bind_fail, bind_text_null(res, ++param, "", 1));
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_int(res, ++param, rrdhost_ingestion_hops(host)));
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_int(res, ++param, host->rrd_memory_mode));
    SQLITE_BIND_FAIL(bind_fail, bind_text_null(res, ++param, rrdhost_abbrev_timezone(host), 1));
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_int(res, ++param, host->utc_offset));
    SQLITE_BIND_FAIL(bind_fail, bind_text_null(res, ++param, rrdhost_program_name(host), 1));
    SQLITE_BIND_FAIL(bind_fail, bind_text_null(res, ++param, rrdhost_program_version(host), 1));
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_int64(res, ++param, host->rrd_history_entries));
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_int(res, ++param, (int)host->health.enabled));
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_int64(res, ++param, (sqlite3_int64) host->stream.snd.status.last_connected));

    int store_rc = sqlite3_step_monitored(res);

    if (unlikely(store_rc != SQLITE_DONE))
        error_report("Failed to store host %s, rc = %d", rrdhost_hostname(host), store_rc);

    SQLITE_FINALIZE(res);

    return store_rc != SQLITE_DONE;

bind_fail:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
    return 1;
}

static int add_host_sysinfo_key_value(const char *name, const char *value, nd_uuid_t *uuid)
{
    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_meta, SQL_STORE_HOST_SYSTEM_INFO_VALUES, &res))
        return 0;

    int param = 0;
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_blob(res, ++param, uuid, sizeof(*uuid), SQLITE_STATIC));
    SQLITE_BIND_FAIL(bind_fail, bind_text_null(res, ++param, name, 0));
    SQLITE_BIND_FAIL(bind_fail, bind_text_null(res, ++param, value ? value : "unknown", 0));

    int store_rc = sqlite3_step_monitored(res);
    if (unlikely(store_rc != SQLITE_DONE))
        error_report("Failed to store host info value %s, rc = %d", name, store_rc);

    SQLITE_FINALIZE(res);

    return store_rc == SQLITE_DONE;

bind_fail:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
    return 0;
}

static bool store_host_systeminfo(RRDHOST *host)
{
    struct rrdhost_system_info *system_info = host->system_info;

    if (unlikely(!system_info))
        return false;

    return (27 != rrdhost_system_info_foreach(system_info, add_host_sysinfo_key_value, &host->host_id.uuid));
}


/*
 * Store a chart in the database
 */

static int store_chart_metadata(RRDSET *st, sqlite3_stmt **res)
{
    if (!*res) {
        if (!PREPARE_STATEMENT(db_meta, SQL_STORE_CHART, res))
            return 1;
    }

    int rc = 1;
    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(*res, ++param, &st->chart_uuid, sizeof(st->chart_uuid), SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(*res, ++param, &st->rrdhost->host_id.uuid, sizeof(st->rrdhost->host_id.uuid), SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_text(*res, ++param, string2str(st->parts.type), -1, SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_text(*res, ++param, string2str(st->parts.id), -1, SQLITE_STATIC));

    const char *name = string2str(st->parts.name);
    if (name && *name)
        SQLITE_BIND_FAIL(done, sqlite3_bind_text(*res, ++param, name, -1, SQLITE_STATIC));
    else
        SQLITE_BIND_FAIL(done, sqlite3_bind_null(*res, ++param));

    SQLITE_BIND_FAIL(done, sqlite3_bind_text(*res, ++param, rrdset_family(st), -1, SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_text(*res, ++param, rrdset_context(st), -1, SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_text(*res, ++param, rrdset_title(st), -1, SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_text(*res, ++param, rrdset_units(st), -1, SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_text(*res, ++param, rrdset_plugin_name(st), -1, SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_text(*res, ++param, rrdset_module_name(st), -1, SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int(*res, ++param, (int) st->priority));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int(*res, ++param, st->update_every));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int(*res, ++param, st->chart_type));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int(*res, ++param, st->rrd_memory_mode));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int(*res, ++param, (int) st->db.entries));

    param = 0;
    rc = sqlite3_step_monitored(*res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to store chart, rc = %d", rc);

done:
    REPORT_BIND_FAIL(*res, param);
    SQLITE_RESET(*res);
    return rc != SQLITE_DONE;
}

static bool store_dimension_metadata(RRDDIM *rd, sqlite3_stmt **res)
{
    if (!*res) {
        if (!PREPARE_STATEMENT(db_meta, SQL_STORE_DIMENSION, res))
            return 1;
    }

    int rc = 1;
    int param = 0;

    nd_uuid_t *rd_uuid = uuidmap_uuid_ptr(rd->uuid);
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(*res, ++param, rd_uuid, sizeof(*rd_uuid), SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(*res, ++param, &rd->rrdset->chart_uuid, sizeof(rd->rrdset->chart_uuid), SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_text(*res, ++param, string2str(rd->id), -1, SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_text(*res, ++param, string2str(rd->name), -1, SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int(*res, ++param, (int) rd->multiplier));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int(*res, ++param, (int ) rd->divisor));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int(*res, ++param, rd->algorithm));
    if (rrddim_option_check(rd, RRDDIM_OPTION_HIDDEN))
        SQLITE_BIND_FAIL(done, sqlite3_bind_text(*res, ++param, "hidden", -1, SQLITE_STATIC));
    else
        SQLITE_BIND_FAIL(done, sqlite3_bind_null(*res, ++param));

    param = 0;
    rc = sqlite3_step_monitored(*res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to store dimension, rc = %d", rc);

done:
    REPORT_BIND_FAIL(*res, param);
    SQLITE_RESET(*res);
    return (rc != SQLITE_DONE);
}

static bool dimension_can_be_deleted(nd_uuid_t *dim_uuid __maybe_unused, sqlite3_stmt **res __maybe_unused, bool flag __maybe_unused)
{
#ifdef ENABLE_DBENGINE
    if(dbengine_enabled && dim_uuid) {
        bool no_retention = true;
        for (size_t tier = 0; tier < nd_profile.storage_tiers; tier++) {
            if (!multidb_ctx[tier])
                continue;
            time_t first_time_t = 0, last_time_t = 0;
            if (rrdeng_metric_retention_by_uuid((void *) multidb_ctx[tier], dim_uuid, &first_time_t, &last_time_t)) {
                if (first_time_t > 0) {
                    no_retention = false;
                    break;
                }
            }
        }
        return no_retention;
    }
    else
        return false;
#else
    return false;
#endif
}

static bool run_cleanup_loop(
    sqlite3_stmt *res,
    struct meta_config_s *config,
    bool (*check_cb)(nd_uuid_t *, sqlite3_stmt **, bool),
    void (*action_cb)(nd_uuid_t *, sqlite3_stmt **, bool),
    uint32_t *total_checked,
    uint32_t *total_deleted,
    uint64_t *row_id,
    sqlite3_stmt **check_stmt,
    sqlite3_stmt **action_stmt,
    bool check_flag,
    bool action_flag)
{
    if (unlikely(SHUTDOWN_REQUESTED(config)))
        return true;

    int rc = sqlite3_bind_int64(res, 1, (sqlite3_int64) *row_id);
    if (unlikely(rc != SQLITE_OK))
        return true;

    time_t start_running = now_monotonic_sec();
    bool time_expired = false;

    uint32_t l_checked = 0;
    uint32_t l_deleted = 0;
    while (!time_expired && sqlite3_step_monitored(res) == SQLITE_ROW) {
        if (unlikely(SHUTDOWN_REQUESTED(config)))
            break;

        *row_id = sqlite3_column_int64(res, 1);
        rc = check_cb((nd_uuid_t *)sqlite3_column_blob(res, 0), check_stmt, check_flag);

        if (rc == true) {
            action_cb((nd_uuid_t *)sqlite3_column_blob(res, 0), action_stmt, action_flag);
            l_deleted++;
//            if (false == sql_metadata_wal_size_acceptable())
//                (void) sqlite3_wal_checkpoint(db_meta, NULL);
        }

        l_checked++;
        time_expired = ((now_monotonic_sec() - start_running) > METADATA_RUNTIME_THRESHOLD);
    }

    (*total_checked) += l_checked;
    (*total_deleted) += l_deleted;
    return time_expired;
}


#define SQL_CHECK_CHART_EXISTENCE_IN_DIMENSION "SELECT count(1) FROM dimension WHERE chart_id = @chart_id"
#define SQL_CHECK_CHART_EXISTENCE_IN_CHART "SELECT count(1) FROM chart WHERE chart_id = @chart_id"

static bool chart_can_be_deleted(nd_uuid_t *chart_uuid, sqlite3_stmt **check_res, bool check_in_dimension)
{
    int rc, result = 1;
    sqlite3_stmt *res = check_res ? *check_res : NULL;

    if (!res) {
        if (!PREPARE_STATEMENT(
                db_meta,
                check_in_dimension ? SQL_CHECK_CHART_EXISTENCE_IN_DIMENSION : SQL_CHECK_CHART_EXISTENCE_IN_CHART,
                &res))
            return 0;

        if (check_res)
            *check_res = res;
    }

    rc = sqlite3_bind_blob(res, 1, chart_uuid, sizeof(*chart_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind chart uuid parameter, rc = %d", rc);
        goto skip;
    }

    rc = sqlite3_step_monitored(res);
    if (likely(rc == SQLITE_ROW))
        result = sqlite3_column_int(res, 0);

skip:
    if (check_res)
        SQLITE_RESET(res);
    else
        SQLITE_FINALIZE(res);

    return result == 0;
}

#define SQL_DELETE_CHART_BY_UUID        "DELETE FROM chart WHERE chart_id = @chart_id"
#define SQL_DELETE_CHART_LABEL_BY_UUID  "DELETE FROM chart_label WHERE chart_id = @chart_id"

static void delete_chart_uuid(nd_uuid_t *chart_uuid, sqlite3_stmt **action_res, bool label_only)
{
    int rc;
    sqlite3_stmt *res = action_res ? *action_res : NULL;

    if (!res) {
        if (!PREPARE_STATEMENT(db_meta, label_only ? SQL_DELETE_CHART_LABEL_BY_UUID : SQL_DELETE_CHART_BY_UUID, &res))
            return;
        if (action_res)
            *action_res = res;
    }

    rc = sqlite3_bind_blob(res, 1, chart_uuid, sizeof(*chart_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind chart uuid parameter, rc = %d", rc);
        goto skip;
    }

    rc = sqlite3_step_monitored(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to delete a chart uuid from the %s table, rc = %d", label_only ? "labels" : "chart", rc);

skip:
    if (action_res)
        SQLITE_RESET(res);
    else
        SQLITE_FINALIZE(res);
}

static uint64_t get_rowid_from_statement(const char *sql)
{
    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_meta, sql, &res))
        return 0;

    uint64_t rowid = 0;

    if (sqlite3_step_monitored(res) == SQLITE_ROW) {
        rowid = sqlite3_column_int64(res, 0);
    }

    SQLITE_FINALIZE(res);
    return rowid;
}


#define SQL_GET_MAX_DIM_ROW_ID "SELECT MAX(rowid) FROM dimension"

static bool check_dimension_metadata(struct meta_config_s *wc)
{
    static time_t next_execution_t = 0;
    static uint64_t last_row_id = 0;
    static uint64_t max_row_id = 0;

    time_t now = now_realtime_sec();

    if (!next_execution_t) {
        next_execution_t = now + METADATA_MAINTENANCE_FIRST_CHECK;
        max_row_id = get_rowid_from_statement(SQL_GET_MAX_DIM_ROW_ID);
        nd_log(NDLS_DAEMON, NDLP_INFO, "Dimension metadata check has been scheduled to run (max id = %lu)", max_row_id);
    }

    if (next_execution_t && next_execution_t > now)
        return true;

    if (max_row_id && last_row_id >= max_row_id) {
        nd_log_daemon(NDLP_INFO, "Dimension metadata check completed");
        // For long running agents, check in a week
        next_execution_t = now + 604800;
        return true;
    }

    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_meta, SELECT_DIMENSION_LIST, &res))
        return true;

    uint32_t total_checked = 0;
    uint32_t total_deleted = 0;

    nd_log(NDLS_DAEMON, NDLP_DEBUG, "Checking dimensions starting after row %" PRIu64, last_row_id);

    worker_is_busy(UV_EVENT_DIMENSION_CLEANUP);

    (void) run_cleanup_loop(
        res,
        wc,
        dimension_can_be_deleted,
        delete_dimension_uuid,
        &total_checked,
        &total_deleted,
        &last_row_id,
        NULL,
        NULL,
        false,
        false);

    now = now_realtime_sec();
    next_execution_t = now + METADATA_MAINTENANCE_REPEAT;
    nd_log_daemon(
        NDLP_DEBUG,
        "Dimensions checked %u, deleted %u. Checks will resume in %d seconds",
        total_checked,
        total_deleted,
        METADATA_MAINTENANCE_REPEAT);

    SQLITE_FINALIZE(res);

    worker_is_idle();
    return false;
}

#define SQL_GET_MAX_CHART_ROW_ID "SELECT MAX(rowid) FROM chart"

static bool check_chart_metadata(struct meta_config_s *wc)
{
    static time_t next_execution_t = 0;
    static uint64_t last_row_id = 0;
    static uint64_t max_row_id = 0;
    static bool check_completed = false;

    if (check_completed)
        return true;

    time_t now = now_realtime_sec();

    if (!next_execution_t) {
        next_execution_t = now + METADATA_MAINTENANCE_FIRST_CHECK;
        max_row_id = get_rowid_from_statement(SQL_GET_MAX_CHART_ROW_ID);
        nd_log(NDLS_DAEMON, NDLP_INFO, "Chart metadata check has been scheduled to run (max id = %lu)", max_row_id);
    }

    if (next_execution_t && next_execution_t > now)
        return true;

    if (max_row_id && last_row_id >= max_row_id) {
        nd_log(NDLS_DAEMON, NDLP_INFO, "Chart metadata check completed");
        check_completed = true;
        return true;
    }

    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_meta, SELECT_CHART_LIST, &res))
        return true;

    uint32_t total_checked = 0;
    uint32_t total_deleted = 0;

    nd_log(NDLS_DAEMON, NDLP_DEBUG, "Checking charts starting after row %" PRIu64, last_row_id);

    worker_is_busy(UV_EVENT_CHART_CLEANUP);
    sqlite3_stmt *check_res = NULL;
    sqlite3_stmt *action_res = NULL;
    (void)run_cleanup_loop(
        res,
        wc,
        chart_can_be_deleted,
        delete_chart_uuid,
        &total_checked,
        &total_deleted,
        &last_row_id,
        &check_res,
        &action_res,
        true,
        false);

    SQLITE_FINALIZE(check_res);
    SQLITE_FINALIZE(action_res);

    now = now_realtime_sec();
    next_execution_t = now + METADATA_MAINTENANCE_REPEAT;
    nd_log_daemon(
        NDLP_DEBUG,
        "Charts checked %u, deleted %u. Checks will resume in %d seconds",
        total_checked,
        total_deleted,
        METADATA_MAINTENANCE_REPEAT);

    SQLITE_FINALIZE(res);
    worker_is_idle();
    return false;
}

#define SQL_GET_MAX_CHART_LABEL_ROW_ID "SELECT MAX(rowid) FROM chart_label"

static bool check_label_metadata(struct meta_config_s *wc)
{
    static time_t next_execution_t = 0;
    static uint64_t last_row_id = 0;
    static uint64_t max_row_id = 0;
    static bool check_completed = false;

    if (check_completed)
        return true;

    time_t now = now_realtime_sec();

    if (!next_execution_t) {
        next_execution_t = now + METADATA_MAINTENANCE_FIRST_CHECK;
        max_row_id = get_rowid_from_statement(SQL_GET_MAX_CHART_LABEL_ROW_ID);
        nd_log(NDLS_DAEMON, NDLP_INFO, "Chart label metadata check has been scheduled to run (max id = %lu)", max_row_id);
    }

    if (next_execution_t && next_execution_t > now)
        return true;

    if (max_row_id && last_row_id >= max_row_id) {
        nd_log(NDLS_DAEMON, NDLP_INFO, "Chart label metadata check completed");
        check_completed = true;
        return true;
    }

    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_meta, SELECT_CHART_LABEL_LIST, &res))
        return true;

    uint32_t total_checked = 0;
    uint32_t total_deleted = 0;

    nd_log(NDLS_DAEMON, NDLP_DEBUG, "Checking charts labels starting after row %" PRIu64, last_row_id);

    sqlite3_stmt *check_res = NULL;
    sqlite3_stmt *action_res = NULL;

    worker_is_busy(UV_EVENT_CHART_LABEL_CLEANUP);

    (void )run_cleanup_loop(
        res,
        wc,
        chart_can_be_deleted,
        delete_chart_uuid,
        &total_checked,
        &total_deleted,
        &last_row_id,
        &check_res,
        &action_res,
        false,
        true);

    SQLITE_FINALIZE(check_res);
    SQLITE_FINALIZE(action_res);

    now = now_realtime_sec();
    next_execution_t = now + METADATA_LABEL_CHECK_INTERVAL;

    nd_log_daemon(
        NDLP_DEBUG,
        "Chart labels checked %u, deleted %u. Checks will resume in %d seconds",
        total_checked,
        total_deleted,
        METADATA_LABEL_CHECK_INTERVAL);

    SQLITE_FINALIZE(res);

    worker_is_idle();
    return false;
}

static void cleanup_health_log(struct meta_config_s *config)
{
    static time_t next_execution_t = 0;

    time_t now = now_realtime_sec();

    if (!next_execution_t)
        next_execution_t = now + METADATA_MAINTENANCE_FIRST_CHECK;

    if (next_execution_t && next_execution_t > now)
        return;

    next_execution_t = now + METADATA_HEALTH_LOG_INTERVAL;

    RRDHOST *host;
    worker_is_busy(UV_EVENT_HEALTH_LOG_CLEANUP);

    dfe_start_reentrant(rrdhost_root_index, host)
    {
        sql_health_alarm_log_cleanup(host);
        if (unlikely(SHUTDOWN_REQUESTED(config)))
            break;
    }
    dfe_done(host);

    if (unlikely(SHUTDOWN_REQUESTED(config))) {
        worker_is_idle();
        return;
    }

    (void) db_execute(db_meta, SQL_DELETE_ORPHAN_HEALTH_LOG, NULL);
    (void) db_execute(db_meta, SQL_DELETE_ORPHAN_HEALTH_LOG_DETAIL, NULL);
    (void) db_execute(db_meta, SQL_DELETE_ORPHAN_ALERT_VERSION, NULL);
    worker_is_idle();
}

//
// EVENT LOOP STARTS HERE
//


static bool metadata_enq_cmd(cmd_data_t *cmd, bool wait_on_full)
{
    if(unlikely(!__atomic_load_n(&meta_config.initialized, __ATOMIC_RELAXED)))
        return false;

    bool added = push_cmd(&meta_config.cmd_pool, (void *)cmd, wait_on_full);
    if (added)
        (void) uv_async_send(&meta_config.async);
    return added;
}

static cmd_data_t metadata_deq_cmd()
{
    cmd_data_t ret;
    ret.opcode = METADATA_DATABASE_NOOP;
    (void) pop_cmd(&meta_config.cmd_pool, (cmd_data_t *) &ret);
    return ret;
}

static void async_cb(uv_async_t *handle)
{
    uv_stop(handle->loop);
    uv_update_time(handle->loop);
}

#define TIMER_INITIAL_PERIOD_MS (1000)
#define TIMER_REPEAT_PERIOD_MS (1000)

static void timer_cb(uv_timer_t *handle)
{
    uv_stop(handle->loop);
    uv_update_time(handle->loop);

   struct meta_config_s *config = handle->data;
   if (config->metadata_check_after <  now_realtime_sec())
       config->store_metadata = true;
}

void vacuum_database(sqlite3 *database, const char *db_alias, int threshold, int vacuum_pc)
{
    static time_t next_run = 0;

    time_t now = now_realtime_sec();
    if (next_run > now)
        return;

    next_run = now + DATABASE_VACUUM_FREQUENCY_SECONDS;

    int free_pages = get_free_page_count(database);
    int total_pages = get_database_page_count(database);

    if (!threshold)
        threshold = DATABASE_FREE_PAGES_THRESHOLD_PC;

    if (!vacuum_pc)
        vacuum_pc = DATABASE_FREE_PAGES_VACUUM_PC;

    if (free_pages > (total_pages * threshold / 100)) {
        int do_free_pages = (int)(free_pages * vacuum_pc / 100);
        nd_log(NDLS_DAEMON, NDLP_DEBUG, "%s: Freeing %d database pages", db_alias, do_free_pages);

        char sql[128];
        snprintfz(sql, sizeof(sql) - 1, "PRAGMA incremental_vacuum(%d)", do_free_pages);
        (void)db_execute(database, sql, NULL);
    }
}

#define SQL_SELECT_HOST_CTX_CHART_DIM_LIST                                                                             \
    "SELECT d.dim_id, d.rowid FROM chart c, dimension d WHERE c.chart_id = d.chart_id AND c.rowid = @rowid"

static bool clean_host_chart_dimensions(sqlite3_stmt **res, int64_t chart_row_id, size_t *checked, size_t *deleted)
{
    struct meta_config_s *config = &meta_config;

    bool can_continue = false;

    if (!*res) {
        if (!PREPARE_STATEMENT(db_meta, SQL_SELECT_HOST_CTX_CHART_DIM_LIST, res))
            return false;
    }
    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(*res, ++param, chart_row_id));
    param = 0;

    sqlite3_stmt *dim_del_stmt = NULL;

    can_continue = true;
    while (can_continue && sqlite3_step_monitored(*res) == SQLITE_ROW) {
        if (sqlite3_column_bytes(*res, 0) != sizeof(nd_uuid_t))
            continue;

        nd_uuid_t *dim_uuid = (nd_uuid_t *)sqlite3_column_blob(*res, 0);
        int64_t dimension_id = sqlite3_column_int64(*res, 1);

        if (dimension_can_be_deleted(dim_uuid, NULL, false)) {
            delete_dimension_by_rowid(&dim_del_stmt, dimension_id, dim_uuid);
            (*deleted)++;
        }
        (*checked)++;
        can_continue = (!SHUTDOWN_REQUESTED(config)) && sql_metadata_wal_size_acceptable();
    }
    SQLITE_FINALIZE(dim_del_stmt);

done:
    REPORT_BIND_FAIL(*res, param);
    SQLITE_RESET(*res);
    return can_continue;
}

#define SQL_SELECT_HOST_CTX_CHART_LIST "SELECT rowid, context FROM chart WHERE host_id = @host"

static void cleanup_host_context_metadata(Pvoid_t CTX_JudyL, void *data)
{
    if (!CTX_JudyL || !data)
        return;

    struct meta_config_s *config = &meta_config;

    RRDHOST *host = data;

    sqlite3_stmt *res = NULL;
    sqlite3_stmt *dimension_res = NULL;
    sqlite3_stmt *context_res = NULL;

    if (!PREPARE_STATEMENT(db_meta, SQL_SELECT_HOST_CTX_CHART_LIST, &res))
        return;

    Word_t num_of_contexts = JudyLCount(CTX_JudyL, 0, -1, PJE0);

    nd_log_daemon(NDLP_DEBUG, "Verifying the retention of %zu contexts for host %s", num_of_contexts, rrdhost_hostname(host));

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, &host->host_id.uuid, sizeof(host->host_id.uuid), SQLITE_STATIC));

    param = 0;
    Pvoid_t *Pvalue;
    int64_t chart_row_id;

    size_t deleted = 0;
    size_t checked = 0;

    bool can_continue = true;
    while (can_continue && sqlite3_step_monitored(res) == SQLITE_ROW) {
        chart_row_id = sqlite3_column_int64(res, 0);
        const char *context = (char *)sqlite3_column_text(res, 1);
        STRING *ctx = string_strdupz(context);
        Pvalue = JudyLGet(CTX_JudyL, (Word_t)ctx, PJE0);
        if (Pvalue) {
            can_continue = clean_host_chart_dimensions(&dimension_res, chart_row_id, &checked, &deleted);
            ctx_delete_metadata_cleanup_context(&context_res, &host->host_id.uuid, context);
        }
        string_freez(ctx);
        can_continue = can_continue && (!SHUTDOWN_REQUESTED(config)) && sql_metadata_wal_size_acceptable();
    }
    SQLITE_FINALIZE(dimension_res);
    SQLITE_FINALIZE(context_res);

    nd_log_daemon(
        NDLP_DEBUG,
        "Verified the contexts of host %s (Checked %zu metrics and removed %zu)",
        rrdhost_hostname(host),
        checked,
        deleted);

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
}

void run_metadata_cleanup(struct meta_config_s *config)
{
    static time_t next_context_list_cleanup = 0;

    time_t now = now_realtime_sec();

    if (!next_context_list_cleanup)
        next_context_list_cleanup = now + 5;

    if (next_context_list_cleanup < now && sql_metadata_wal_size_acceptable()) {
        RRDHOST *host;
        worker_is_busy(UV_EVENT_CTX_CLEANUP);
        dfe_start_reentrant(rrdhost_root_index, host) {
            ctx_get_context_list_to_cleanup(&host->host_id.uuid, cleanup_host_context_metadata, host);
            if (SHUTDOWN_REQUESTED(config) || false == sql_metadata_wal_size_acceptable())
                break;
        }
        dfe_done(host);
        worker_is_idle();
        next_context_list_cleanup = now_realtime_sec() + METADATA_MAINTENANCE_CTX_CLEAN_REPEAT;
    }

    if (unlikely(SHUTDOWN_REQUESTED(config)))
        return;

    if (check_dimension_metadata(config))
        if (check_chart_metadata(config))
            check_label_metadata(config);

    cleanup_health_log(config);

    if (unlikely(SHUTDOWN_REQUESTED(config)))
        return;

    vacuum_database(db_meta, "METADATA", DATABASE_FREE_PAGES_THRESHOLD_PC, DATABASE_FREE_PAGES_VACUUM_PC);

    (void) sqlite3_wal_checkpoint(db_meta, NULL);
}

struct host_context_load_thread {
    ND_THREAD *thread;
    RRDHOST *host;
    sqlite3 *db_meta_thread;
    sqlite3 *db_context_thread;
    bool busy;
    bool finished;
};

__thread sqlite3 *db_meta_thread = NULL;
__thread sqlite3 *db_context_thread = NULL;
__thread bool main_context_thread = false;

extern uv_sem_t ctx_sem;
static void restore_host_context(void *arg)
{
    struct host_context_load_thread *hclt = arg;
    RRDHOST *host = hclt->host;

    if (!host)
        return;

    if (!db_meta_thread) {
        if (hclt->db_meta_thread) {
            db_meta_thread = hclt->db_meta_thread;
            db_context_thread = hclt->db_context_thread;
        } else {
            char sqlite_database[FILENAME_MAX + 1];
            snprintfz(sqlite_database, sizeof(sqlite_database) - 1, "%s/netdata-meta.db", netdata_configured_cache_dir);
            int rc = sqlite3_open_v2(sqlite_database, &db_meta_thread, SQLITE_OPEN_READONLY | SQLITE_OPEN_NOMUTEX, NULL);
            if (rc != SQLITE_OK) {
                sqlite3_close(db_meta_thread);
                db_meta_thread = NULL;
            }

            snprintfz(sqlite_database, sizeof(sqlite_database) - 1, "%s/context-meta.db", netdata_configured_cache_dir);
            rc = sqlite3_open_v2(sqlite_database, &db_context_thread, SQLITE_OPEN_READONLY | SQLITE_OPEN_NOMUTEX, NULL);
            if (rc != SQLITE_OK) {
                sqlite3_close(db_context_thread);
                db_context_thread = NULL;
            }

            hclt->db_meta_thread = db_meta_thread;
            hclt->db_context_thread = db_context_thread;
        }
    }

    usec_t started_ut = now_monotonic_usec(); (void)started_ut;
    rrdhost_load_rrdcontext_data(host);
    usec_t ended_ut = now_monotonic_usec(); (void)ended_ut;

    char load_duration[64];
    duration_snprintf(load_duration, sizeof(load_duration), (int64_t)(ended_ut - started_ut), "us", true);
    nd_log_daemon(NDLP_DEBUG, "Contexts for host %s loaded in %s", rrdhost_hostname(host), load_duration);

    rrdhost_flag_clear(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD);
    pulse_host_status(host, 0, 0); // this will detect the receiver status

    aclk_queue_node_info(host, false);

    if (IS_VIRTUAL_HOST_OS(host)) {
        uv_sem_post(&ctx_sem);
    }

    // Check and clear the thread local variables
    if (!main_context_thread) {
        db_meta_thread = NULL;
        db_context_thread = NULL;
    }

    __atomic_store_n(&hclt->finished, true, __ATOMIC_RELEASE);
}

// Callback after scan of hosts is done
static void after_ctx_hosts_load(uv_work_t *req, int status __maybe_unused)
{
    worker_data_t *worker = req->data;
    struct meta_config_s *config = worker->config;
    config->ctx_load_running = false;
    return_worker(&config->worker_pool, worker);
}

static bool cleanup_finished_threads(struct host_context_load_thread *hclt, size_t max_thread_slots, bool wait, size_t *free_slot)
{
    if (!hclt)
        return false;

    bool found_slot = false;

    size_t loop_count = 20;
    while (loop_count--) {
        for (size_t index = 0; index < max_thread_slots; index++) {
            if (free_slot && false == __atomic_load_n(&(hclt[index].busy), __ATOMIC_ACQUIRE)) {
                found_slot = true;
                *free_slot = index;
                break;
            }
            if (__atomic_load_n(&(hclt[index].finished), __ATOMIC_RELAXED) ||
                (wait && __atomic_load_n(&(hclt[index].busy), __ATOMIC_ACQUIRE))) {

                int rc = nd_thread_join(hclt[index].thread);
                if (rc)
                    nd_log_daemon(NDLP_WARNING, "Failed to join thread, rc = %d", rc);
                __atomic_store_n(&(hclt[index].busy), false, __ATOMIC_RELEASE);
                __atomic_store_n(&(hclt[index].finished), false, __ATOMIC_RELEASE);
                found_slot = true;
                if (free_slot) {
                    *free_slot = index;
                    break;
                }
            }
        }
        if (found_slot || wait)
            break;
        sleep_usec(10 * USEC_PER_MS);
    }
    return found_slot || wait;
}

void reset_host_context_load_flag()
{
    RRDHOST *host;
    dfe_start_reentrant(rrdhost_root_index, host)
    {
        rrdhost_flag_clear(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD);
    }
    dfe_done(host);
}

static void ctx_hosts_load(uv_work_t *req)
{
    register_libuv_worker_jobs();

    worker_data_t *worker = req->data;
    struct meta_config_s *config = worker->config;

    worker_is_busy(UV_EVENT_HOST_CONTEXT_LOAD);
    usec_t started_ut = now_monotonic_usec(); (void)started_ut;

    RRDHOST *host;

    size_t max_threads = netdata_conf_cpus();
    if (max_threads < 1)
        max_threads = 1;

    nd_log(NDLS_DAEMON, NDLP_DEBUG, "Using %zu threads for context loading", max_threads);
    struct host_context_load_thread *hclt = max_threads > 1 ? callocz(max_threads, sizeof(*hclt)) : NULL;

    size_t thread_index = 0;
    main_context_thread = true;
    size_t host_count = 0;
    size_t sync_exec = 0;
    size_t async_exec = 0;

    for (int pass=0 ; pass < 2 ; pass++) {
        dfe_start_reentrant(rrdhost_root_index, host) {
            // pass 0 will do vnodes (skip the rest)
            // pass 1 will do the rest (skip vnodes)
            if (pass == IS_VIRTUAL_HOST_OS(host))
                continue;

            if (!rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD))
                continue;

            if (unlikely(SHUTDOWN_REQUESTED(config)))
                break;

            nd_log_daemon(NDLP_DEBUG, "Loading context for host %s", rrdhost_hostname(host));

            int rc = 0;
            bool thread_found = cleanup_finished_threads(hclt, max_threads, false, &thread_index);
            if (thread_found) {
                __atomic_store_n(&hclt[thread_index].busy, true, __ATOMIC_RELAXED);
                hclt[thread_index].host = host;
                hclt[thread_index].thread = nd_thread_create("CTXLOAD", NETDATA_THREAD_OPTION_DEFAULT, restore_host_context, &hclt[thread_index]);
                rc = (hclt[thread_index].thread == NULL);
                async_exec += (rc == 0);
                // if it failed, mark the thread slot as free
                if (rc)
                    __atomic_store_n(&hclt[thread_index].busy, false, __ATOMIC_RELAXED);
            }
            // if single thread, thread creation failure or failure tofind slot
            if (rc || !thread_found) {
                sync_exec++;
                struct host_context_load_thread hclt_sync = {.host = host};
                restore_host_context(&hclt_sync);
            }
            host_count++;
        }
        dfe_done(host);
    }

    bool should_clean_threads = cleanup_finished_threads(hclt, max_threads, true, NULL);

    if (should_clean_threads) {
        for (size_t index = 0; index < max_threads; index++) {
            if (hclt[index].db_meta_thread)
                sqlite3_close_v2(hclt[index].db_meta_thread);

            if (hclt[index].db_context_thread)
                sqlite3_close_v2(hclt[index].db_context_thread);
        }
    }

    usec_t ended_ut = now_monotonic_usec(); (void)ended_ut;
    char load_duration[64];
    duration_snprintf(load_duration, sizeof(load_duration), (int64_t)(ended_ut - started_ut), "us", true);

    nd_log_daemon(
        NDLP_INFO,
        "Contexts for %zu hosts loaded: %zu delegated to %zu threads, %zu handled directly, in %s.",
        host_count,
        async_exec,
        max_threads,
        sync_exec,
        load_duration);

    if (db_meta_thread) {
        sqlite3_close_v2(db_meta_thread);
        sqlite3_close_v2(db_context_thread);
        db_meta_thread = NULL;
        db_context_thread = NULL;
    }
    freez(hclt);
    worker_is_idle();
}

// Callback after scan of hosts is done
static void after_metadata_hosts(uv_work_t *req, int status __maybe_unused)
{
    worker_data_t *worker = req->data;
    struct meta_config_s *config = worker->config;

    bool first = true;
    Word_t Index = 0;
    Pvoid_t *Pvalue;
    while ((Pvalue = JudyLFirstThenNext(config->ae_DelJudyL, &Index, &first))) {
        ALARM_ENTRY *ae = (ALARM_ENTRY *) Index;
        if(!__atomic_load_n(&ae->pending_save_count, __ATOMIC_RELAXED)) {
            health_alarm_log_free_one_nochecks_nounlink(ae);
            (void) JudyLDel(&config->ae_DelJudyL, Index, PJE0);
            first = true;
            Index = 0;
        }
    }

    config->metadata_running = false;
    return_worker(&config->worker_pool, worker);
}

#ifdef ENABLE_DBENGINE
#define GET_UUID_LIST  "SELECT dim_id FROM dimension"
size_t populate_metrics_from_database(void *mrg, void (*populate_cb)(void *mrg, Word_t section, nd_uuid_t *uuid))
{
    sqlite3_stmt *res = NULL;
    sqlite3 *local_meta_db = NULL;

    char sqlite_database[FILENAME_MAX + 1];
    snprintfz(sqlite_database, sizeof(sqlite_database) - 1, "%s/netdata-meta.db", netdata_configured_cache_dir);
    int rc = sqlite3_open_v2(sqlite_database, &local_meta_db, SQLITE_OPEN_READONLY | SQLITE_OPEN_NOMUTEX, NULL);
    if (rc != SQLITE_OK) {
        sqlite3_close(local_meta_db);
        local_meta_db = NULL;
    }

    if (local_meta_db)
        (void)db_execute(local_meta_db, "PRAGMA cache_size=10000", NULL);

    if (!PREPARE_STATEMENT(local_meta_db ? local_meta_db : db_meta, GET_UUID_LIST, &res)) {
        sqlite3_close(local_meta_db);
        return 0;
    }

    size_t count = 0;

    usec_t started_ut = now_monotonic_usec();
    while (sqlite3_step(res) == SQLITE_ROW) {
        nd_uuid_t *uuid = (nd_uuid_t *)sqlite3_column_blob(res, 0);
        if (!uuid || sqlite3_column_bytes(res, 0) != sizeof(nd_uuid_t))
            continue;

        for (size_t tier = 0; tier < nd_profile.storage_tiers ; tier++) {
            if (unlikely(!multidb_ctx[tier]))
                continue;

            populate_cb(mrg, (Word_t)multidb_ctx[tier], uuid);
        }
        count++;
    }

    SQLITE_FINALIZE(res);
    sqlite3_close(local_meta_db);
    COMPUTE_DURATION(report_duration, "us", started_ut, now_monotonic_usec());
    nd_log_daemon(NDLP_INFO, "MRG: Loaded %zu metrics from database in %s", count, report_duration);
    return count;
}
#endif

static void metadata_scan_host(struct meta_config_s *config, RRDHOST *host, BUFFER *work_buffer, bool is_worker)
{
    static bool skip_models = false;
    RRDSET *st;
    int rc;

    sqlite3_stmt *ml_load_stmt = NULL;
    sqlite3_stmt *store_dimension = NULL;
    sqlite3_stmt *store_chart = NULL;

    bool load_ml_models = is_worker;

    bool host_need_recheck = false;
    (void)db_execute(db_meta, "BEGIN TRANSACTION", NULL);

    rrdset_foreach_reentrant(st, host) {

        if (SHUTDOWN_REQUESTED(config))
            break;

        if(rrdset_flag_check(st, RRDSET_FLAG_METADATA_UPDATE)) {

            rrdset_flag_clear(st, RRDSET_FLAG_METADATA_UPDATE);

            buffer_flush(work_buffer);

            if (is_worker)
                worker_is_busy(UV_EVENT_STORE_CHART);

            rc = check_and_update_chart_labels(st, work_buffer);
            if (unlikely(rc))
                error_report("METADATA: 'host:%s': Failed to update labels for chart %s", rrdhost_hostname(host), rrdset_name(st));

            rc = store_chart_metadata(st, &store_chart);
            if (unlikely(rc)) {
                host_need_recheck = true;
                rrdset_flag_set(st, RRDSET_FLAG_METADATA_UPDATE);
                error_report(
                    "METADATA: 'host:%s': Failed to store metadata for chart %s",
                    rrdhost_hostname(host),
                    rrdset_name(st));
            }
            if (is_worker)
                worker_is_idle();
        }

        RRDDIM *rd;
        rrddim_foreach_read(rd, st) {
            if (load_ml_models) {
                if (rrddim_flag_check(rd, RRDDIM_FLAG_ML_MODEL_LOAD)) {
                    rrddim_flag_clear(rd, RRDDIM_FLAG_ML_MODEL_LOAD);
                    if (likely(!skip_models)) {
                        if (is_worker)
                            worker_is_busy(UV_EVENT_METADATA_ML_LOAD);

                        skip_models = ml_dimension_load_models(rd, &ml_load_stmt);

                        if (is_worker)
                            worker_is_idle();
                    }
                }
            }

            if(likely(!rrddim_flag_check(rd, RRDDIM_FLAG_METADATA_UPDATE)))
                continue;

            rrddim_flag_clear(rd, RRDDIM_FLAG_METADATA_UPDATE);

            if (rrddim_option_check(rd, RRDDIM_OPTION_HIDDEN))
                rrddim_flag_set(rd, RRDDIM_FLAG_META_HIDDEN);
            else
                rrddim_flag_clear(rd, RRDDIM_FLAG_META_HIDDEN);

            if (is_worker)
                worker_is_busy(UV_EVENT_STORE_DIMENSION);

            rc = store_dimension_metadata(rd, &store_dimension);
            if (unlikely(rc)) {
                host_need_recheck = true;
                rrddim_flag_set(rd, RRDDIM_FLAG_METADATA_UPDATE);
                error_report(
                    "METADATA: 'host:%s': Failed to store dimension metadata for chart %s. dimension %s",
                    rrdhost_hostname(host),
                    rrdset_name(st),
                    rrddim_name(rd));
            }

            if (is_worker)
                worker_is_idle();
        }
        rrddim_foreach_done(rd);
    }
    rrdset_foreach_done(st);

    (void)db_execute(db_meta, "COMMIT TRANSACTION", NULL);
    if (host_need_recheck)
        rrdhost_flag_set(host,RRDHOST_FLAG_METADATA_UPDATE);

    SQLITE_FINALIZE(ml_load_stmt);
    SQLITE_FINALIZE(store_dimension);
    SQLITE_FINALIZE(store_chart);
}


static void store_host_and_system_info(RRDHOST *host)
{
    rrdhost_flag_clear(host, RRDHOST_FLAG_METADATA_INFO);

    if (unlikely(store_host_systeminfo(host))) {
        error_report("METADATA: 'host:%s': Failed to store host updated system information in the database", rrdhost_hostname(host));
        rrdhost_flag_set(host, RRDHOST_FLAG_METADATA_INFO | RRDHOST_FLAG_METADATA_UPDATE);
    }

    if (unlikely(store_host_metadata(host))) {
        error_report("METADATA: 'host:%s': Failed to store host info in the database", rrdhost_hostname(host));
        rrdhost_flag_set(host, RRDHOST_FLAG_METADATA_INFO | RRDHOST_FLAG_METADATA_UPDATE);
    }
}

struct judy_list_t {
    Pvoid_t JudyL;
    Word_t count;
};

static void do_pending_uuid_deletion(struct meta_config_s *config, struct judy_list_t *pending_uuid_deletion)
{
    if (!pending_uuid_deletion)
        return;

    worker_is_busy(UV_EVENT_UUID_DELETION);

    usec_t started_ut = now_monotonic_usec(); (void)started_ut;

    size_t entries = pending_uuid_deletion->count;
    Word_t Index = 0;
    bool first = true;
    Pvoid_t *Pvalue;
    while ((Pvalue = JudyLFirstThenNext(pending_uuid_deletion->JudyL, &Index, &first))) {
        if (!*Pvalue)
            continue;

        nd_uuid_t *uuid = *Pvalue;
        if (likely(!SHUTDOWN_REQUESTED(config))) {
            if (dimension_can_be_deleted(uuid, NULL, false))
                delete_dimension_uuid(uuid, NULL, false);
        }

        freez(uuid);
    }
    (void) JudyLFreeArray(&pending_uuid_deletion->JudyL, PJE0);
    freez(pending_uuid_deletion);

    usec_t ended_ut = now_monotonic_usec(); (void)ended_ut;
    nd_log_daemon(
        NDLP_DEBUG,
        "Processed %zu dimension delete items in %0.2f ms",
        entries,
        (double)(ended_ut - started_ut) / USEC_PER_MS);

    worker_is_idle();
}

static void store_ctx_cleanup_list(struct meta_config_s *config, struct judy_list_t *pending_ctx_cleanup_list)
{
    if (!pending_ctx_cleanup_list)
        return;

    worker_is_busy(UV_EVENT_CTX_CLEANUP_SCHEDULE);

    usec_t started_ut = now_monotonic_usec(); (void)started_ut;

    size_t entries = pending_ctx_cleanup_list->count;
    Word_t Index = 0;
    bool first = true;
    Pvoid_t *Pvalue;
    sqlite3_stmt *res = NULL;
    while ((Pvalue = JudyLFirstThenNext(pending_ctx_cleanup_list->JudyL, &Index, &first))) {
        if (!*Pvalue)
            continue;

        struct host_ctx_cleanup_s *ctx_cleanup = *Pvalue;

        if (likely(!SHUTDOWN_REQUESTED(config)))
            sql_schedule_host_ctx_cleanup(&res, &ctx_cleanup->host_uuid, string2str(ctx_cleanup->context));

        string_freez(ctx_cleanup->context);
        freez(ctx_cleanup);
    }
    (void) JudyLFreeArray(&pending_ctx_cleanup_list->JudyL, PJE0);
    freez(pending_ctx_cleanup_list);
    SQLITE_FINALIZE(res);

    usec_t ended_ut = now_monotonic_usec(); (void)ended_ut;
    nd_log_daemon(
        NDLP_DEBUG,
        "Stored %zu host context cleanup items in %0.2f ms",
        entries,
        (double)(ended_ut - started_ut) / USEC_PER_MS);

    worker_is_idle();
}

static void store_alert_transitions(struct judy_list_t *pending_alert_list, bool is_worker, bool cleanup_only)
{
    if (!pending_alert_list)
        return;

    if (cleanup_only)
        goto done;

    if (is_worker)
        worker_is_busy(UV_EVENT_STORE_ALERT_TRANSITIONS);

    usec_t started_ut = now_monotonic_usec(); (void)started_ut;

    size_t entries = pending_alert_list->count;
    Word_t Index = 0;
    bool first = true;
    Pvoid_t *Pvalue;
    while ((Pvalue = JudyLFirstThenNext(pending_alert_list->JudyL, &Index, &first))) {
        RRDHOST *host = *Pvalue;

        Pvalue = JudyLGet(pending_alert_list->JudyL, ++Index, PJE0);
        ALARM_ENTRY *ae = *Pvalue;

        sql_health_alarm_log_save(host, ae);

        __atomic_add_fetch(&ae->pending_save_count, -1, __ATOMIC_RELAXED);
        __atomic_add_fetch(&host->health.pending_transitions, -1, __ATOMIC_RELAXED);
    }

    usec_t ended_ut = now_monotonic_usec(); (void)ended_ut;
    nd_log(
        NDLS_DAEMON,
        NDLP_DEBUG,
        "Stored and processed %zu alert transitions in %0.2f ms",
        entries,
        (double)(ended_ut - started_ut) / USEC_PER_MS);

    if (is_worker)
        worker_is_idle();

done:
    (void) JudyLFreeArray(&pending_alert_list->JudyL, PJE0);
    freez(pending_alert_list);
}

static int execute_statement(sqlite3_stmt *stmt, bool only_finalize)
{
    if (!stmt)
        return SQLITE_OK;

    int rc = SQLITE_OK;

    if (unlikely(only_finalize))
        goto done;

    rc = sqlite3_step_monitored(stmt);
    if (unlikely(rc != SQLITE_DONE))
        nd_log_daemon(NDLP_ERR, "Failed to execute sql statement, rc = %d", rc);

done:
    SQLITE_FINALIZE(stmt);

    return rc == SQLITE_DONE ? SQLITE_OK : rc;
}

static void store_sql_statements(struct judy_list_t *pending_sql_statement, bool is_worker, bool only_finalize)
{
    if (!pending_sql_statement)
        return;

    if (is_worker)
        worker_is_busy(METADATA_EXECUTE_STORE_STATEMENT);

    usec_t started_ut = now_monotonic_usec();

    size_t entries = pending_sql_statement->count;
    Word_t Index = 0;
    bool first = true;
    Pvoid_t *Pvalue;
    while ((Pvalue = JudyLFirstThenNext(pending_sql_statement->JudyL, &Index, &first))) {
        sqlite3_stmt *stmt = *Pvalue;
        execute_statement(stmt, only_finalize);
    }
    (void) JudyLFreeArray(&pending_sql_statement->JudyL, PJE0);
    freez(pending_sql_statement);

    COMPUTE_DURATION(report_duration, "us", started_ut, now_monotonic_usec());
    nd_log_daemon(NDLP_DEBUG, "Stored and processed %zu sql statements in %s", entries, report_duration);

    if (is_worker)
        worker_is_idle();
}

static void meta_store_host_labels(RRDHOST *host, BUFFER *work_buffer)
{
    rrdhost_flag_clear(host, RRDHOST_FLAG_METADATA_LABELS);

    int rc = exec_statement_with_uuid(SQL_DELETE_HOST_LABELS, &host->host_id.uuid);
    if (unlikely(rc)) {
        error_report("METADATA: 'host:%s': failed to delete old host labels", rrdhost_hostname(host));
        rrdhost_flag_set(host, RRDHOST_FLAG_METADATA_LABELS | RRDHOST_FLAG_METADATA_UPDATE);
        return;
    }

    buffer_flush(work_buffer);

    struct query_build tmp = {.sql = work_buffer, .count = 0};
    uuid_unparse_lower(host->host_id.uuid, tmp.uuid_str);
    rrdlabels_walkthrough_read(host->rrdlabels, host_label_store_to_sql_callback, &tmp);
    buffer_strcat(
        work_buffer,
        " ON CONFLICT (host_id, label_key) DO UPDATE SET source_type = excluded.source_type, label_value=excluded.label_value, date_created=UNIXEPOCH()");
    rc = db_execute(db_meta, buffer_tostring(work_buffer), NULL);

    if (unlikely(rc)) {
        error_report("METADATA: 'host:%s': failed to update metadata host labels", rrdhost_hostname(host));
        rrdhost_flag_set(host, RRDHOST_FLAG_METADATA_LABELS | RRDHOST_FLAG_METADATA_UPDATE);
    }
}

static void store_host_claim_id(RRDHOST *host)
{
    rrdhost_flag_clear(host, RRDHOST_FLAG_METADATA_CLAIMID);
    int rc;
    ND_UUID uuid = claim_id_get_uuid();
    if (!UUIDiszero(uuid))
        rc = store_claim_id(&host->host_id.uuid, &uuid.uuid);
    else
        rc = store_claim_id(&host->host_id.uuid, NULL);

    if (unlikely(rc))
        rrdhost_flag_set(host, RRDHOST_FLAG_METADATA_CLAIMID | RRDHOST_FLAG_METADATA_UPDATE);
}

void store_host_info_and_metadata(RRDHOST *host, BUFFER *work_buffer)
{
    // Store labels (if needed)
    if (unlikely(rrdhost_flag_check(host, RRDHOST_FLAG_METADATA_LABELS)))
        meta_store_host_labels(host, work_buffer);

    // Store claim id (if needed)
    if (unlikely(rrdhost_flag_check(host, RRDHOST_FLAG_METADATA_CLAIMID)))
        store_host_claim_id(host);

    // Store host and system info (if needed);
    if (rrdhost_flag_check(host, RRDHOST_FLAG_METADATA_INFO))
        store_host_and_system_info(host);
}

static void store_hosts_metadata(struct meta_config_s *config, BUFFER *work_buffer, bool is_worker)
{
    RRDHOST *host;
    size_t host_count = 0;
    usec_t started_ut = now_monotonic_usec();
    if (!is_worker) {
        dfe_start_reentrant(rrdhost_root_index, host) {
            host_count++;
        }
        dfe_done(host);
        if (!host_count)
            host_count = 1; // avoid division by zero
    }

    size_t count = 0;
    dfe_start_reentrant(rrdhost_root_index, host)
    {
        count++;
        if (rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED) || !rrdhost_flag_check(host, RRDHOST_FLAG_METADATA_UPDATE))
            continue;

        rrdhost_flag_clear(host, RRDHOST_FLAG_METADATA_UPDATE);

        if (SHUTDOWN_REQUESTED(config))
            break;

        if (is_worker)
            worker_is_busy(UV_EVENT_STORE_HOST);

        // store labels, claim_id, host and system info (if needed)
        store_host_info_and_metadata(host, work_buffer);

        if (is_worker)
            worker_is_idle();

        metadata_scan_host(config, host, work_buffer, is_worker);

        if (!is_worker)
            nd_log_daemon(NDLP_INFO, "METADATA: Progress of metadata storage: %6.2f%% completed", (100.0 * count / host_count));
    }
    dfe_done(host);

    if (!is_worker) {
        COMPUTE_DURATION(report_duration, "us", started_ut, now_monotonic_usec());
        nd_log_daemon(
            NDLP_INFO,
            "METADATA: Progress of metadata storage: %6.2f%% completed in %s",
            (100.0 * count / host_count),
            report_duration);
    }
}

// Worker thread to scan hosts for pending metadata to store
static void start_metadata_hosts(uv_work_t *req)
{
    register_libuv_worker_jobs();

    worker_data_t *worker = req->data;
    struct meta_config_s *config = worker->config;

    BUFFER *work_buffer = worker->work_buffer;
    usec_t all_started_ut = now_monotonic_usec();

    store_sql_statements((struct judy_list_t *)worker->pending_sql_statement, true, false);

    store_alert_transitions((struct judy_list_t *)worker->pending_alert_list, true, false);

    if (!SHUTDOWN_REQUESTED(config))
        store_ctx_cleanup_list(config, (struct judy_list_t *)worker->pending_ctx_cleanup_list);

    worker_is_busy(UV_EVENT_METADATA_STORE);

    store_hosts_metadata(config, work_buffer, true);

    COMPUTE_DURATION(report_duration, "us", all_started_ut, now_monotonic_usec());
    nd_log_daemon(NDLP_DEBUG, "Checking all hosts completed in %s", report_duration);

    if (!SHUTDOWN_REQUESTED(config)) {
        do_pending_uuid_deletion(config, (struct judy_list_t *)worker->pending_uuid_deletion);
        run_metadata_cleanup(config);
    }

    config->metadata_check_after = now_realtime_sec() + METADATA_HOST_CHECK_INTERVAL;
    worker_is_idle();
}

#define EVENT_LOOP_NAME "METASYNC"

#define MAX_SHUTDOWN_TIMEOUT_SECONDS (15)
#define SHUTDOWN_SLEEP_INTERVAL_MS (100)
#define CMD_POOL_SIZE (32768)

static void metadata_event_loop(void *arg)
{
    struct meta_config_s *config = arg;
    uv_thread_set_name_np(EVENT_LOOP_NAME);
    service_register(NULL, NULL, NULL);
    worker_register(EVENT_LOOP_NAME);

    init_cmd_pool(&config->cmd_pool, CMD_POOL_SIZE);

    worker_register_job_name(METADATA_DATABASE_NOOP, "noop");
    worker_register_job_name(METADATA_DEL_DIMENSION, "delete dimension");
    worker_register_job_name(METADATA_STORE_CLAIM_ID, "add claim id");
    worker_register_job_name(METADATA_ADD_CTX_CLEANUP, "host ctx cleanup");
    worker_register_job_name(METADATA_STORE, "host metadata store");
    worker_register_job_name(METADATA_LOAD_HOST_CONTEXT, "host load context");
    worker_register_job_name(METADATA_ADD_HOST_AE, "add host alert entry");
    worker_register_job_name(METADATA_DEL_HOST_AE, "delete host alert entry");
    worker_register_job_name(METADATA_EXECUTE_STORE_STATEMENT, "add sql statement");

    uv_loop_t *loop = &config->loop;
    fatal_assert(0 == uv_loop_init(loop));
    fatal_assert(0 == uv_async_init(loop, &config->async, async_cb));
    fatal_assert(0 == uv_timer_init(loop, &config->timer_req));
    fatal_assert(0 == uv_timer_start(&config->timer_req, timer_cb, TIMER_INITIAL_PERIOD_MS, TIMER_REPEAT_PERIOD_MS));
    loop->data = config;
    config->async.data = config;
    config->timer_req.data = config;

    nd_log(NDLS_DAEMON, NDLP_DEBUG, "Starting metadata sync thread");
    config->metadata_check_after = now_realtime_sec() + METADATA_HOST_CHECK_FIRST_CHECK;

    BUFFER *work_buffer = buffer_create(1024, &netdata_buffers_statistics.buffers_sqlite);
    worker_data_t *worker;
    Pvoid_t *Pvalue;
    struct judy_list_t *pending_alert_list = NULL;
    struct judy_list_t *pending_ctx_cleanup_list = NULL;
    struct judy_list_t *pending_uuid_deletion = NULL;
    struct judy_list_t *pending_sql_statement = NULL;

    config->initialized = true;
    __atomic_store_n(&config->shutdown_requested, false, __ATOMIC_RELAXED);
    nd_log_daemon(NDLP_INFO, "METADATA: Synchronization thread is up and running");
    completion_mark_complete(&config->start_stop_complete);

    while (likely(__atomic_load_n(&config->shutdown_requested, __ATOMIC_RELAXED) == false))  {
        nd_uuid_t *uuid;
        RRDHOST *host = NULL;
        ALARM_ENTRY *ae = NULL;
        sqlite3_stmt *stmt;
        enum metadata_opcode opcode;

        worker_is_idle();
        uv_run(loop, UV_RUN_DEFAULT);

        /* wait for commands */
        unsigned cmd_batch_size = 0;
        do {
            if (unlikely(++cmd_batch_size >= METADATA_MAX_BATCH_SIZE))
                break;

            cmd_data_t cmd;
            if (config->store_metadata && !config->metadata_running) {
                config->store_metadata = false;
                opcode = METADATA_STORE;
            }
            else {
                cmd = metadata_deq_cmd();
                opcode = cmd.opcode;
            }

            if (likely(opcode != METADATA_DATABASE_NOOP))
                worker_is_busy(opcode);

            switch (opcode) {
                case METADATA_DATABASE_NOOP:
                    break;
                case METADATA_DEL_DIMENSION:
                    uuid = (nd_uuid_t *)cmd.param[0];
                    if (!pending_uuid_deletion)
                        pending_uuid_deletion = callocz(1, sizeof(*pending_uuid_deletion));

                    Pvalue = JudyLIns(&pending_uuid_deletion->JudyL, ++pending_uuid_deletion->count, PJE0);
                    if (Pvalue != PJERR)
                        *Pvalue = uuid;
                    else {
                        // Failure in Judy, attempt to continue running anyway
                        // ignore uuid, global cleanup will take care of it
                        freez(uuid);
                    }
                    break;
                case METADATA_STORE_CLAIM_ID:
                    store_claim_id((nd_uuid_t *)cmd.param[0], (nd_uuid_t *)cmd.param[1]);
                    freez((void *)cmd.param[0]);
                    freez((void *)cmd.param[1]);
                    break;

                case METADATA_ADD_CTX_CLEANUP:
                    if (!pending_ctx_cleanup_list)
                        pending_ctx_cleanup_list = callocz(1, sizeof(*pending_ctx_cleanup_list));

                    struct host_ctx_cleanup_s *ctx_cleanup = (struct host_ctx_cleanup_s *)cmd.param[0];
                    Pvalue = JudyLIns(&pending_ctx_cleanup_list->JudyL, ++pending_ctx_cleanup_list->count, PJE0);
                    if (Pvalue && Pvalue != PJERR)
                        *Pvalue = ctx_cleanup;
                    else {
                        // Failure in Judy, attempt to continue running anyway
                        // Cleanup structure
                        string_freez(ctx_cleanup->context);
                        freez(ctx_cleanup);
                    }
                    break;
                case METADATA_STORE:
                    if (config->metadata_running || unittest_running)
                        break;

                    worker = get_worker(&config->worker_pool);
                    worker->config = config;
                    worker->pending_alert_list = pending_alert_list;
                    worker->pending_ctx_cleanup_list = pending_ctx_cleanup_list;
                    worker->pending_uuid_deletion = pending_uuid_deletion;
                    worker->pending_sql_statement = pending_sql_statement;

                    worker->work_buffer = work_buffer;
                    pending_alert_list = NULL;
                    pending_ctx_cleanup_list = NULL;
                    pending_uuid_deletion = NULL;
                    pending_sql_statement = NULL;
                    config->metadata_running = true;
                    if (uv_queue_work(loop, &worker->request, start_metadata_hosts, after_metadata_hosts)) {
                        pending_alert_list = worker->pending_alert_list;
                        pending_ctx_cleanup_list = worker->pending_ctx_cleanup_list;
                        pending_uuid_deletion = worker->pending_uuid_deletion;
                        pending_sql_statement = worker->pending_sql_statement;
                        config->metadata_running = false;
                        return_worker(&config->worker_pool, worker);
                    }
                    break;
                case METADATA_LOAD_HOST_CONTEXT:
                    if (config->ctx_load_running || unittest_running)
                        break;

                    worker = get_worker(&config->worker_pool);
                    config->ctx_load_running = true;
                    worker->config = config;
                    if (uv_queue_work(loop, &worker->request, ctx_hosts_load, after_ctx_hosts_load)) {
                        config->ctx_load_running = false;
                        // Fallback reset context so hosts will load on demand
                        reset_host_context_load_flag();
                        return_worker(&config->worker_pool, worker);
                    }
                    break;
                case METADATA_ADD_HOST_AE:
                    host = (RRDHOST *)cmd.param[0];
                    ae = (ALARM_ENTRY *)cmd.param[1];

                    if (!pending_alert_list)
                        pending_alert_list = callocz(1, sizeof(*pending_alert_list));

                    Pvalue = JudyLIns(&pending_alert_list->JudyL, ++pending_alert_list->count, PJE0);
                    if (!Pvalue || Pvalue == PJERR)
                        fatal("METASYNC: Corrupted pending_alert_list Judy array");
                    *Pvalue = (void *)host;

                    Pvalue = JudyLIns(&pending_alert_list->JudyL, ++pending_alert_list->count, PJE0);
                    if (!Pvalue || Pvalue == PJERR)
                        fatal("METASYNC: Corrupted pending_alert_list Judy array");
                    *Pvalue = (void *)ae;
                    break;
                case METADATA_DEL_HOST_AE:
                    (void)JudyLIns(&config->ae_DelJudyL, (Word_t)(void *)cmd.param[0], PJE0);
                    break;
                case METADATA_EXECUTE_STORE_STATEMENT:
                    stmt = (sqlite3_stmt *)cmd.param[0];
                    if (!pending_sql_statement)
                        pending_sql_statement = callocz(1, sizeof(*pending_sql_statement));

                    Pvalue = JudyLIns(&pending_sql_statement->JudyL, ++pending_sql_statement->count, PJE0);
                    if (Pvalue && Pvalue != PJERR)
                        *Pvalue = (void *)stmt;
                    else {
                        // Fallback execute immediately
                        execute_statement(stmt, false);
                    }
                    break;
                case METADATA_SYNC_SHUTDOWN:
                    __atomic_store_n(&config->shutdown_requested, true, __ATOMIC_RELAXED);
                    break;
                case METADATA_UNITTEST:;
                    struct thread_unittest *tu = (struct thread_unittest *)cmd.param[0];
                    sleep_usec(1000); // processing takes 1ms
                    __atomic_fetch_add(&tu->processed, 1, __ATOMIC_SEQ_CST);
                    break;
                default:
                    break;
            }
        } while (opcode != METADATA_DATABASE_NOOP);
    }
    config->initialized = false;

    if (!uv_timer_stop(&config->timer_req))
        uv_close((uv_handle_t *)&config->timer_req, NULL);

    uv_close((uv_handle_t *)&config->async, NULL);
    uv_walk(loop, libuv_close_callback, NULL);

    size_t loop_count = (MAX_SHUTDOWN_TIMEOUT_SECONDS * MSEC_PER_SEC) / SHUTDOWN_SLEEP_INTERVAL_MS;

    // are we waiting for callbacks?
    bool callbacks_pending = (config->metadata_running || config->ctx_load_running);

    while ((config->metadata_running || config->ctx_load_running) && loop_count > 0) {
        callbacks_pending = uv_run(loop, UV_RUN_NOWAIT);
        if (!callbacks_pending)
            break;  // No pending callbacks
        sleep_usec(SHUTDOWN_SLEEP_INTERVAL_MS * USEC_PER_MS);
        loop_count--;
    }

    (void)uv_loop_close(loop);

    store_alert_transitions(pending_alert_list, false, true);
    store_sql_statements(pending_sql_statement, false, true);

    if (pending_ctx_cleanup_list) {
        Word_t Index = 0;
        bool first = true;
        while ((Pvalue = JudyLFirstThenNext(pending_ctx_cleanup_list->JudyL, &Index, &first))) {
            if (!*Pvalue)
                continue;
            struct host_ctx_cleanup_s *ctx_cleanup = *Pvalue;
            string_freez(ctx_cleanup->context);
            freez(ctx_cleanup);
        }
        (void)JudyLFreeArray(&pending_ctx_cleanup_list->JudyL, PJE0);
        freez(pending_ctx_cleanup_list);
    }

    if (pending_uuid_deletion) {
        Word_t Index = 0;
        bool first = true;
        Pvoid_t *Pvalue;
        while ((Pvalue = JudyLFirstThenNext(pending_uuid_deletion->JudyL, &Index, &first))) {
            if (!*Pvalue)
                continue;
            nd_uuid_t *uuid = *Pvalue;
            freez(uuid);
        }
        (void)JudyLFreeArray(&pending_uuid_deletion->JudyL, PJE0);
        freez(pending_uuid_deletion);
    }

    buffer_free(work_buffer);
    release_cmd_pool(&config->cmd_pool);
    worker_unregister();
    service_exits();
    completion_mark_complete(&config->start_stop_complete);
}

void metadata_sync_shutdown(void)
{
    cmd_data_t cmd;
    memset(&cmd, 0, sizeof(cmd));
    cmd.opcode = METADATA_SYNC_SHUTDOWN;

    // if we can't sent command return
    // This should not happen but if we wait we may not get a completion
    // and shutdown will timeout
    if (!metadata_enq_cmd(&cmd, true)) {
        nd_log_daemon(NDLP_WARNING, "METADATA: Failed to send a shutdown command");
        return;
    }
    nd_log_daemon(NDLP_INFO, "METADATA: Submitted shutdown command, waiting for ACK");

    completion_wait_for(&meta_config.start_stop_complete);
    completion_destroy(&meta_config.start_stop_complete);

    int rc = nd_thread_join(meta_config.thread);
    if (rc)
        nd_log_daemon(NDLP_ERR, "METADATA: Failed to join synchronization thread");
    else
        nd_log_daemon(NDLP_INFO, "METADATA: synchronization thread shutdown completed");
}

// -------------------------------------------------------------
// Init function called on agent startup

void metadata_sync_init(void)
{
    memset(&meta_config, 0, sizeof(meta_config));
    completion_init(&meta_config.start_stop_complete);

    init_worker_pool(&meta_config.worker_pool);
    meta_config.thread = nd_thread_create("METASYNC", NETDATA_THREAD_OPTION_DEFAULT, metadata_event_loop, &meta_config);
    fatal_assert(NULL != meta_config.thread);

    // Wait for initialization
    completion_wait_for(&meta_config.start_stop_complete);

    // Reset the completion, we will use it again during shutdown
    completion_reset(&meta_config.start_stop_complete);
}

//  Helpers

static inline bool queue_metadata_cmd(enum metadata_opcode opcode, void *param0, void *param1)
{
    cmd_data_t cmd;
    cmd.opcode = opcode;
    cmd.param[0] = param0;
    cmd.param[1] = param1;
    return metadata_enq_cmd(&cmd, true);
}

// Public
void metaqueue_delete_dimension_uuid(nd_uuid_t *uuid)
{
    if (unlikely(!uuid))
        return;

    nd_uuid_t *use_uuid = mallocz(sizeof(*uuid));
    uuid_copy(*use_uuid, *uuid);
    if (!queue_metadata_cmd(METADATA_DEL_DIMENSION, use_uuid, NULL))
        freez(use_uuid);
}

void metaqueue_store_claim_id(nd_uuid_t *host_uuid, nd_uuid_t *claim_uuid)
{
    if (unlikely(!host_uuid))
        return;

    nd_uuid_t *local_host_uuid = mallocz(sizeof(*host_uuid));
    nd_uuid_t *local_claim_uuid = NULL;

    uuid_copy(*local_host_uuid, *host_uuid);
    if (likely(claim_uuid)) {
        local_claim_uuid = mallocz(sizeof(*claim_uuid));
        uuid_copy(*local_claim_uuid, *claim_uuid);
    }
    if (unlikely(!queue_metadata_cmd(METADATA_STORE_CLAIM_ID, local_host_uuid, local_claim_uuid))) {
        freez(local_host_uuid);
        freez(local_claim_uuid);
    }
}

void metaqueue_ml_load_models(RRDDIM *rd)
{
    rrddim_flag_set(rd, RRDDIM_FLAG_ML_MODEL_LOAD);
}

bool metadata_queue_load_host_context()
{
    return queue_metadata_cmd(METADATA_LOAD_HOST_CONTEXT, NULL, NULL);
}

void metadata_queue_ctx_host_cleanup(nd_uuid_t *host_uuid, const char *context)
{
    if (unlikely(!host_uuid || !context))
        return;

    struct host_ctx_cleanup_s *ctx_cleanup = mallocz(sizeof(*ctx_cleanup));

    uuid_copy(ctx_cleanup->host_uuid, *host_uuid);
    ctx_cleanup->context = string_strdupz(context);

    if (unlikely(!queue_metadata_cmd(METADATA_ADD_CTX_CLEANUP, ctx_cleanup, NULL))) {
        string_freez(ctx_cleanup->context);
        freez(ctx_cleanup);
    }
}

bool metadata_queue_ae_save(RRDHOST *host, ALARM_ENTRY *ae)
{
    if (unlikely(!host || !ae))
        return true;

    __atomic_add_fetch(&host->health.pending_transitions, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&ae->pending_save_count, 1, __ATOMIC_RELAXED);

    if (unlikely(!queue_metadata_cmd(METADATA_ADD_HOST_AE, host, ae))) {
        // Failed to queue, reset counters
        __atomic_sub_fetch(&host->health.pending_transitions, 1, __ATOMIC_RELAXED);
        __atomic_sub_fetch(&ae->pending_save_count, 1, __ATOMIC_RELAXED);
        return false;
    }
    return true;
}

void metadata_queue_ae_deletion(ALARM_ENTRY *ae)
{
    if (unlikely(!ae))
        return;

    (void) queue_metadata_cmd(METADATA_DEL_HOST_AE, ae, NULL);
}

void metadata_execute_store_statement(sqlite3_stmt *stmt)
{
    if (unlikely(!stmt))
        return;

    (void) queue_metadata_cmd(METADATA_EXECUTE_STORE_STATEMENT, stmt, NULL);
}

void commit_alert_transitions(RRDHOST *host __maybe_unused)
{
    (void) queue_metadata_cmd(METADATA_STORE, NULL, NULL);
}

uint64_t sqlite_get_meta_space(void)
{
    return sqlite_get_db_space(db_meta);
}

#define SQL_ADD_AGENT_EVENT_LOG      \
    "INSERT INTO agent_event_log (event_type, version, value, date_created) VALUES " \
    " (@event_type, @version, @value, UNIXEPOCH())"

void add_agent_event(event_log_type_t event_id, int64_t value)
{
    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_meta, SQL_ADD_AGENT_EVENT_LOG, &res))
        return;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_int(res, ++param, event_id));
    SQLITE_BIND_FAIL(done, sqlite3_bind_text(res, ++param, NETDATA_VERSION, -1, SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_int64(res, ++param, value));

    param = 0;
    int rc = execute_insert(res);
    if (rc != SQLITE_DONE)
        error_report("Failed to store agent event information, rc = %d", rc);
done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
}

void cleanup_agent_event_log(void)
{
    (void) db_execute(db_meta, SQL_CLEANUP_AGENT_EVENT_LOG, NULL);
}

#define SQL_GET_AGENT_EVENT_TYPE_MEDIAN                                                                                \
    "SELECT AVG(value) AS median FROM "                                                                                \
    "(SELECT value FROM agent_event_log WHERE event_type = @event ORDER BY value "                                     \
    " LIMIT 2 - (SELECT COUNT(*) FROM agent_event_log WHERE event_type = @event) % 2 "                                 \
    "OFFSET(SELECT(COUNT(*) - 1) / 2 FROM agent_event_log WHERE event_type = @event)) "

usec_t get_agent_event_time_median(event_log_type_t event_id)
{
    static bool initialized[EVENT_AGENT_MAX] = { 0 };
    static usec_t median[EVENT_AGENT_MAX] = { 0 };

    if(event_id >= EVENT_AGENT_MAX)
        return 0;

    if(initialized[event_id])
        return median[event_id];

    sqlite3_stmt *res = NULL;
    if (!PREPARE_STATEMENT(db_meta, SQL_GET_AGENT_EVENT_TYPE_MEDIAN, &res))
        return 0;

    usec_t avg_time = 0;
    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_int(res, ++param, event_id));

    param = 0;
    if (sqlite3_step_monitored(res) == SQLITE_ROW)
        avg_time = sqlite3_column_int64(res, 0);

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);

    median[event_id] = avg_time;
    initialized[event_id] = true;
    return avg_time;
}

void get_agent_event_time_median_init(void) {
    for(event_log_type_t event_id = 1; event_id < EVENT_AGENT_MAX; event_id++)
        get_agent_event_time_median(event_id);
}

//
// unitests
//

static void unittest_queue_metadata(void *arg) {
    struct thread_unittest *tu = arg;

    cmd_data_t cmd;
    cmd.opcode = METADATA_UNITTEST;
    cmd.param[0] = tu;
    cmd.param[1] = NULL;
    metadata_enq_cmd(&cmd, true);

    do {
        __atomic_fetch_add(&tu->added, 1, __ATOMIC_SEQ_CST);
        metadata_enq_cmd(&cmd, true);
        sleep_usec(10000);
    } while (!__atomic_load_n(&tu->join, __ATOMIC_RELAXED));
}

static void *metadata_unittest_threads(void)
{

    unsigned done;

    struct thread_unittest tu = {
        .join = 0,
        .added = 0,
        .processed = 0,
        .done = &done,
    };

    // Queue messages / Time it
    time_t seconds_to_run = 5;
    int threads_to_create = 4;
    fprintf(
        stderr,
        "\nChecking metadata queue using %d threads for %lld seconds...\n",
        threads_to_create,
        (long long)seconds_to_run);

    ND_THREAD *threads[threads_to_create];
    tu.join = 0;
    for (int i = 0; i < threads_to_create; i++) {
        char buf[100 + 1];
        snprintf(buf, sizeof(buf) - 1, "META[%d]", i);
        threads[i] = nd_thread_create(buf, NETDATA_THREAD_OPTION_DONT_LOG, unittest_queue_metadata, &tu);
    }
    (void) uv_async_send(&meta_config.async);
    sleep_usec(seconds_to_run * USEC_PER_SEC);

    __atomic_store_n(&tu.join, 1, __ATOMIC_RELAXED);
    for (int i = 0; i < threads_to_create; i++) {
        nd_thread_join(threads[i]);
    }
    sleep_usec(5 * USEC_PER_SEC);

    fprintf(stderr, "Added %u elements, processed %u\n", tu.added, tu.processed);

    return 0;
}

int metadata_unittest(void)
{
    metadata_sync_init();

    // Queue items for a specific period of time
    metadata_unittest_threads();

    metadata_sync_shutdown();

    return 0;
}

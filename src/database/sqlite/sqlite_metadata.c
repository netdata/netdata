// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_metadata.h"
#include "sqlite3recover.h"
//#include "sqlite_db_migration.h"

#define DB_METADATA_VERSION 18

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

    "CREATE TABLE IF NOT EXISTS alert_version (health_log_id INTEGER PRIMARY KEY, unique_id INT, status INT, "
    "version INT, date_submitted INT)",

    "CREATE TABLE IF NOT EXISTS aclk_queue (sequence_id INTEGER PRIMARY KEY, host_id blob, health_log_id INT, "
    "unique_id INT, date_created INT,  UNIQUE(host_id, health_log_id))",

    NULL
};

const char *database_cleanup[] = {
    "DELETE FROM host WHERE host_id NOT IN (SELECT host_id FROM chart)",
    "DELETE FROM node_instance WHERE host_id NOT IN (SELECT host_id FROM host)",
    "DELETE FROM host_info WHERE host_id NOT IN (SELECT host_id FROM host)",
    "DELETE FROM host_label WHERE host_id NOT IN (SELECT host_id FROM host)",
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
#define METADATA_HEALTH_LOG_INTERVAL (3600)         // Repeat maintenance for health
#define METADATA_DIM_CHECK_INTERVAL (3600)          // Repeat maintenance for dimensions
#define METADATA_CHART_CHECK_INTERVAL (3600)        // Repeat maintenance for charts
#define METADATA_LABEL_CHECK_INTERVAL (3600)        // Repeat maintenance for labels
#define METADATA_RUNTIME_THRESHOLD (5)              // Run time threshold for cleanup task

#define METADATA_HOST_CHECK_FIRST_CHECK (5)         // First check for pending metadata
#define METADATA_HOST_CHECK_INTERVAL (5)            // Repeat check for pending metadata
#define MAX_METADATA_CLEANUP (500)                  // Maximum metadata write operations (e.g  deletes before retrying)
#define METADATA_MAX_BATCH_SIZE (512)               // Maximum commands to execute before running the event loop

#define DATABASE_FREE_PAGES_THRESHOLD_PC (5)        // Percentage of free pages to trigger vacuum
#define DATABASE_FREE_PAGES_VACUUM_PC (10)          // Percentage of free pages to vacuum

enum metadata_opcode {
    METADATA_DATABASE_NOOP = 0,
    METADATA_DATABASE_TIMER,
    METADATA_DEL_DIMENSION,
    METADATA_STORE_CLAIM_ID,
    METADATA_ADD_HOST_INFO,
    METADATA_SCAN_HOSTS,
    METADATA_LOAD_HOST_CONTEXT,
    METADATA_DELETE_HOST_CHART_LABELS,
    METADATA_ADD_HOST_AE,
    METADATA_DEL_HOST_AE,
    METADATA_MAINTENANCE,
    METADATA_SYNC_SHUTDOWN,
    METADATA_UNITTEST,
    // leave this last
    // we need it to check for worker utilization
    METADATA_MAX_ENUMERATIONS_DEFINED
};

#define MAX_PARAM_LIST  (2)
struct metadata_cmd {
    enum metadata_opcode opcode;
    struct completion *completion;
    const void *param[MAX_PARAM_LIST];
    struct metadata_cmd *prev, *next;
};

typedef enum {
    METADATA_FLAG_PROCESSING    = (1 << 0), // store or cleanup
    METADATA_FLAG_SHUTDOWN      = (1 << 1), // Shutting down
} METADATA_FLAG;

struct metadata_wc {
    uv_thread_t thread;
    uv_loop_t *loop;
    uv_async_t async;
    uv_timer_t timer_req;
    time_t metadata_check_after;
    Pvoid_t ae_DelJudyL;
    METADATA_FLAG flags;
    struct completion start_stop_complete;
    struct completion *scan_complete;
    /* FIFO command queue */
    SPINLOCK cmd_queue_lock;
    struct metadata_cmd *cmd_base;
};

#define metadata_flag_check(target_flags, flag) (__atomic_load_n(&((target_flags)->flags), __ATOMIC_SEQ_CST) & (flag))
#define metadata_flag_set(target_flags, flag)   __atomic_or_fetch(&((target_flags)->flags), (flag), __ATOMIC_SEQ_CST)
#define metadata_flag_clear(target_flags, flag) __atomic_and_fetch(&((target_flags)->flags), ~(flag), __ATOMIC_SEQ_CST)

struct metadata_wc metasync_worker = {.loop = NULL};

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

    if (!REQUIRE_DB(db_meta))
        return 0;

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

    struct aclk_sync_cfg_t  *wc = host->aclk_config;

    uuid_copy(host->node_id.uuid, *node_id);

    if (unlikely(!wc))
        create_aclk_config(host, &host->host_id.uuid, node_id);
    else
        uuid_unparse_lower(*node_id, wc->node_id);

    stream_receiver_send_node_and_claim_id_to_child(host);
    stream_path_node_id_updated(host);
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
    int rc = execute_insert(res);
    status = (rc == SQLITE_DONE);
    if (false == status)
        error_report("Failed to store node instance information, rc = %d", rc);
done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
    return status;
}


#define SQL_UPDATE_NODE_ID  "UPDATE node_instance SET node_id = @node_id WHERE host_id = @host_id"

int sql_update_node_id(nd_uuid_t *host_id, nd_uuid_t *node_id)
{
    sqlite3_stmt *res = NULL;
    RRDHOST *host = NULL;
    int rc = 2;

    char host_guid[GUID_LEN + 1];
    uuid_unparse_lower(*host_id, host_guid);
    rrd_wrlock();
    host = rrdhost_find_by_guid(host_guid);
    if (likely(host))
        set_host_node_id(host, node_id);
    rrd_wrunlock();

    if (!REQUIRE_DB(db_meta))
        return 1;

    if (!PREPARE_STATEMENT(db_meta, SQL_UPDATE_NODE_ID, &res))
        return 1;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, node_id, sizeof(*node_id), SQLITE_STATIC));
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, host_id, sizeof(*host_id), SQLITE_STATIC));

    param = 0;
    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to store node instance information, rc = %d", rc);
    rc = sqlite3_changes(db_meta);

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
    return rc - 1;
}

#define SQL_SELECT_NODE_ID  "SELECT node_id FROM node_instance WHERE host_id = @host_id AND node_id IS NOT NULL"

int get_node_id(nd_uuid_t *host_id, nd_uuid_t *node_id)
{
    sqlite3_stmt *res = NULL;

    if (!REQUIRE_DB(db_meta))
        return 1;

    if (!PREPARE_STATEMENT(db_meta, SQL_SELECT_NODE_ID, &res))
        return 1;

    int param = 0, rc = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, host_id, sizeof(*host_id), SQLITE_STATIC));

    param = 0;
    rc = sqlite3_step_monitored(res);
    if (likely(rc == SQLITE_ROW && node_id))
        uuid_copy(*node_id, *((nd_uuid_t *) sqlite3_column_blob(res, 0)));

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
    return (rc == SQLITE_ROW) ? 0 : -1;
}

#define SQL_INVALIDATE_NODE_INSTANCES                                                                                  \
    "UPDATE node_instance SET node_id = NULL WHERE EXISTS "                                                            \
    "(SELECT host_id FROM node_instance WHERE host_id = @host_id AND (@claim_id IS NULL OR claim_id <> @claim_id))"

void invalidate_node_instances(nd_uuid_t *host_id, nd_uuid_t *claim_id)
{
    sqlite3_stmt *res = NULL;

    if (!REQUIRE_DB(db_meta))
        return;

    if (!PREPARE_STATEMENT(db_meta, SQL_INVALIDATE_NODE_INSTANCES, &res))
        return;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, host_id, sizeof(*host_id), SQLITE_STATIC));

    if (claim_id)
        SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, claim_id, sizeof(*claim_id), SQLITE_STATIC));
    else
        SQLITE_BIND_FAIL(done, sqlite3_bind_null(res, ++param));

    param = 0;
    int rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to invalidate node instance information, rc = %d", rc);

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
}

#define SQL_GET_NODE_INSTANCE_LIST                                                                                     \
    "SELECT ni.node_id, ni.host_id, h.hostname "                                                                       \
    "FROM node_instance ni, host h WHERE ni.host_id = h.host_id AND h.hops >=0"

struct node_instance_list *get_node_list(void)
{
    struct node_instance_list *node_list = NULL;
    sqlite3_stmt *res = NULL;

    if (!REQUIRE_DB(db_meta))
        return NULL;

    if (!PREPARE_STATEMENT(db_meta, SQL_GET_NODE_INSTANCE_LIST, &res))
        return NULL;

    int row = 0;
    char host_guid[UUID_STR_LEN];
    while (sqlite3_step_monitored(res) == SQLITE_ROW)
        row++;

    if (sqlite3_reset(res) != SQLITE_OK) {
        error_report("Failed to reset the prepared statement while fetching node instance information");
        goto failed;
    }
    node_list = callocz(row + 1, sizeof(*node_list));
    int max_rows = row;
    row = 0;
    // TODO: Check to remove lock
    rrd_rdlock();
    while (sqlite3_step_monitored(res) == SQLITE_ROW) {
        if (sqlite3_column_bytes(res, 0) == sizeof(nd_uuid_t))
            uuid_copy(node_list[row].node_id, *((nd_uuid_t *)sqlite3_column_blob(res, 0)));
        if (sqlite3_column_bytes(res, 1) == sizeof(nd_uuid_t)) {
            nd_uuid_t *host_id = (nd_uuid_t *)sqlite3_column_blob(res, 1);
            uuid_unparse_lower(*host_id, host_guid);
            RRDHOST *host = rrdhost_find_by_guid(host_guid);
            if (!host)
                continue;

            if (rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD)) {
                netdata_log_info(
                    "ACLK: 'host:%s' skipping get node list because context is initializing", rrdhost_hostname(host));
                continue;
            }

            uuid_copy(node_list[row].host_id, *host_id);
            node_list[row].queryable = 1;
            node_list[row].live =
                (host == localhost || host->receiver || !(rrdhost_flag_check(host, RRDHOST_FLAG_ORPHAN))) ? 1 : 0;
            node_list[row].hops = host->system_info ? host->system_info->hops :
                                  uuid_eq(*host_id, localhost->host_id.uuid) ? 0 : 1;
            node_list[row].hostname =
                sqlite3_column_bytes(res, 2) ? strdupz((char *)sqlite3_column_text(res, 2)) : NULL;
        }
        row++;
        if (row == max_rows)
            break;
    }
    rrd_rdunlock();

failed:
    SQLITE_FINALIZE(res);

    return node_list;
}

#define SQL_GET_HOST_NODE_ID "SELECT node_id FROM node_instance WHERE host_id = @host_id"

void sql_load_node_id(RRDHOST *host)
{
    sqlite3_stmt *res = NULL;

    if (!REQUIRE_DB(db_meta))
        return;

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
        rrdhost_set_system_info_variable(
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
    int rc = execute_insert(res);
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
    (void) db_execute(database, "select count(*) from sqlite_master limit 0");

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
        sqlite3_close(db_meta);
        db_meta = NULL;
        return 1;
    }

    if (rebuild & DB_CHECK_RECLAIM_SPACE) {
        netdata_log_info("Reclaiming space of %s", sqlite_database);
        rc = sqlite3_exec_monitored(db_meta, "VACUUM", 0, 0, &err_msg);
        if (rc != SQLITE_OK) {
            error_report("Failed to execute VACUUM rc = %d (%s)", rc, err_msg);
            sqlite3_free(err_msg);
        }
        else {
            (void) db_execute(db_meta, "select count(*) from sqlite_master limit 0");
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
            (void) db_execute(db_meta, "select count(*) from sqlite_master limit 0");
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
        return 1;

    if (init_database_batch(db_meta, &database_config[0], "meta_init"))
        return 1;

    if (init_database_batch(db_meta, &database_cleanup[0], "meta_cleanup"))
        return 1;

    netdata_log_info("SQLite database initialization completed");

    return 0;
}

// Metadata functions

struct query_build {
    BUFFER *sql;
    int count;
    char uuid_str[UUID_STR_LEN];
};

#define SQL_DELETE_CHART_LABELS_BY_HOST                                                                                \
    "DELETE FROM chart_label WHERE chart_id in (SELECT chart_id FROM chart WHERE host_id = @host_id)"

static void delete_host_chart_labels(nd_uuid_t *host_uuid)
{
    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_meta, SQL_DELETE_CHART_LABELS_BY_HOST, &res))
        return;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, host_uuid, sizeof(*host_uuid), SQLITE_STATIC));

    param = 0;
    int rc = sqlite3_step_monitored(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to execute command to remove chart labels, rc = %d", rc);

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
}

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

#define SQL_DELETE_CHART_LABEL "DELETE FROM chart_label WHERE chart_id = @chart_id"
#define SQL_DELETE_CHART_LABEL_HISTORY "DELETE FROM chart_label WHERE date_created < %ld AND chart_id = @chart_id"

static void clean_old_chart_labels(RRDSET *st)
{
    char sql[512];
    time_t first_time_s = rrdset_first_entry_s(st);

    if (unlikely(!first_time_s))
        snprintfz(sql, sizeof(sql) - 1, SQL_DELETE_CHART_LABEL);
    else
        snprintfz(sql, sizeof(sql) - 1, SQL_DELETE_CHART_LABEL_HISTORY, first_time_s);

    int rc = exec_statement_with_uuid(sql, &st->chart_uuid);
    if (unlikely(rc))
        error_report("METADATA: 'host:%s' Failed to clean old labels for chart %s", rrdhost_hostname(st->rrdhost), rrdset_name(st));
}

static int check_and_update_chart_labels(RRDSET *st, BUFFER *work_buffer, size_t *query_counter)
{
    size_t old_version = st->rrdlabels_last_saved_version;
    size_t new_version = rrdlabels_version(st->rrdlabels);

    if (new_version == old_version)
        return 0;

    struct query_build tmp = {.sql = work_buffer, .count = 0};
    uuid_unparse_lower(st->chart_uuid, tmp.uuid_str);
    rrdlabels_walkthrough_read(st->rrdlabels, chart_label_store_to_sql_callback, &tmp);
    buffer_strcat(work_buffer, " ON CONFLICT (chart_id, label_key) DO UPDATE SET source_type = excluded.source_type, label_value=excluded.label_value, date_created=UNIXEPOCH()");
    int rc = db_execute(db_meta, buffer_tostring(work_buffer));
    if (likely(!rc)) {
        st->rrdlabels_last_saved_version = new_version;
        (*query_counter)++;
    }

    clean_old_chart_labels(st);
    return rc;
}

// If the machine guid has changed, then existing one with hops 0 will be marked as hops 1 (child)
void detect_machine_guid_change(nd_uuid_t *host_uuid)
{
    int rc;

    rc = exec_statement_with_uuid(CONVERT_EXISTING_LOCALHOST, host_uuid);
    if (!rc) {
        if (unlikely(db_execute(db_meta, DELETE_MISSING_NODE_INSTANCES)))
            error_report("Failed to remove deleted hosts from node instances");
    }
}

static int store_claim_id(nd_uuid_t *host_id, nd_uuid_t *claim_id)
{
    sqlite3_stmt *res = NULL;
    int rc = 0;

    if (!REQUIRE_DB(db_meta))
        return 1;

    if (!PREPARE_STATEMENT(db_meta, SQL_STORE_CLAIM_ID, &res))
        return 1;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, host_id, sizeof(*host_id), SQLITE_STATIC));

    if (claim_id)
        SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param,claim_id, sizeof(*claim_id), SQLITE_STATIC));
    else
        SQLITE_BIND_FAIL(done, sqlite3_bind_null(res, ++param));

    param = 0;
    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to store host claim id rc = %d", rc);

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_FINALIZE(res);
    return rc != SQLITE_DONE;
}

static void delete_dimension_uuid(nd_uuid_t *dimension_uuid, sqlite3_stmt **action_res __maybe_unused, bool flag __maybe_unused)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc;

    if (!PREPARE_COMPILED_STATEMENT(db_meta, DELETE_DIMENSION_UUID, &res))
        return;

    int param = 0;
    SQLITE_BIND_FAIL(done, sqlite3_bind_blob(res, ++param, dimension_uuid, sizeof(*dimension_uuid), SQLITE_STATIC));

    param = 0;
    rc = sqlite3_step_monitored(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to delete dimension uuid, rc = %d", rc);

done:
    REPORT_BIND_FAIL(res, param);
    SQLITE_RESET(res);
}

//
// Store host and host system info information in the database
static int store_host_metadata(RRDHOST *host)
{
    static __thread sqlite3_stmt *res = NULL;

    if (!PREPARE_COMPILED_STATEMENT(db_meta, SQL_STORE_HOST_INFO, &res))
        return false;

    int param = 0;
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_blob(res, ++param, &host->host_id.uuid, sizeof(host->host_id.uuid), SQLITE_STATIC));
    SQLITE_BIND_FAIL(bind_fail, bind_text_null(res, ++param, rrdhost_hostname(host), 0));
    SQLITE_BIND_FAIL(bind_fail, bind_text_null(res, ++param, rrdhost_registry_hostname(host), 1));
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_int(res, ++param, host->rrd_update_every));
    SQLITE_BIND_FAIL(bind_fail, bind_text_null(res, ++param, rrdhost_os(host), 1));
    SQLITE_BIND_FAIL(bind_fail, bind_text_null(res, ++param, rrdhost_timezone(host), 1));
    SQLITE_BIND_FAIL(bind_fail, bind_text_null(res, ++param, "", 1));
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_int(res, ++param, host->system_info ? host->system_info->hops : 0));
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

    SQLITE_RESET(res);

    return store_rc != SQLITE_DONE;

bind_fail:
    REPORT_BIND_FAIL(res, param);
    SQLITE_RESET(res);
    return 1;
}

static int add_host_sysinfo_key_value(const char *name, const char *value, nd_uuid_t *uuid)
{
    static __thread sqlite3_stmt *res = NULL;

    if (!REQUIRE_DB(db_meta))
        return 0;

    if (!PREPARE_COMPILED_STATEMENT(db_meta, SQL_STORE_HOST_SYSTEM_INFO_VALUES, &res))
        return 0;

    int param = 0;
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_blob(res, ++param, uuid, sizeof(*uuid), SQLITE_STATIC));
    SQLITE_BIND_FAIL(bind_fail, bind_text_null(res, ++param, name, 0));
    SQLITE_BIND_FAIL(bind_fail, bind_text_null(res, ++param, value ? value : "unknown", 0));

    int store_rc = sqlite3_step_monitored(res);
    if (unlikely(store_rc != SQLITE_DONE))
        error_report("Failed to store host info value %s, rc = %d", name, store_rc);

    SQLITE_RESET(res);

    return store_rc == SQLITE_DONE;

bind_fail:
    REPORT_BIND_FAIL(res, param);
    SQLITE_RESET(res);
    return 0;
}

static bool store_host_systeminfo(RRDHOST *host)
{
    struct rrdhost_system_info *system_info = host->system_info;

    if (unlikely(!system_info))
        return false;

    int ret = 0;

    ret += add_host_sysinfo_key_value("NETDATA_CONTAINER_OS_NAME", system_info->container_os_name, &host->host_id.uuid);
    ret += add_host_sysinfo_key_value("NETDATA_CONTAINER_OS_ID", system_info->container_os_id, &host->host_id.uuid);
    ret += add_host_sysinfo_key_value("NETDATA_CONTAINER_OS_ID_LIKE", system_info->container_os_id_like, &host->host_id.uuid);
    ret += add_host_sysinfo_key_value("NETDATA_CONTAINER_OS_VERSION", system_info->container_os_version, &host->host_id.uuid);
    ret += add_host_sysinfo_key_value("NETDATA_CONTAINER_OS_VERSION_ID", system_info->container_os_version_id, &host->host_id.uuid);
    ret += add_host_sysinfo_key_value("NETDATA_CONTAINER_OS_DETECTION", system_info->host_os_detection, &host->host_id.uuid);
    ret += add_host_sysinfo_key_value("NETDATA_HOST_OS_NAME", system_info->host_os_name, &host->host_id.uuid);
    ret += add_host_sysinfo_key_value("NETDATA_HOST_OS_ID", system_info->host_os_id, &host->host_id.uuid);
    ret += add_host_sysinfo_key_value("NETDATA_HOST_OS_ID_LIKE", system_info->host_os_id_like, &host->host_id.uuid);
    ret += add_host_sysinfo_key_value("NETDATA_HOST_OS_VERSION", system_info->host_os_version, &host->host_id.uuid);
    ret += add_host_sysinfo_key_value("NETDATA_HOST_OS_VERSION_ID", system_info->host_os_version_id, &host->host_id.uuid);
    ret += add_host_sysinfo_key_value("NETDATA_HOST_OS_DETECTION", system_info->host_os_detection, &host->host_id.uuid);
    ret += add_host_sysinfo_key_value("NETDATA_SYSTEM_KERNEL_NAME", system_info->kernel_name, &host->host_id.uuid);
    ret += add_host_sysinfo_key_value("NETDATA_SYSTEM_CPU_LOGICAL_CPU_COUNT", system_info->host_cores, &host->host_id.uuid);
    ret += add_host_sysinfo_key_value("NETDATA_SYSTEM_CPU_FREQ", system_info->host_cpu_freq, &host->host_id.uuid);
    ret += add_host_sysinfo_key_value("NETDATA_SYSTEM_TOTAL_RAM", system_info->host_ram_total, &host->host_id.uuid);
    ret += add_host_sysinfo_key_value("NETDATA_SYSTEM_TOTAL_DISK_SIZE", system_info->host_disk_space, &host->host_id.uuid);
    ret += add_host_sysinfo_key_value("NETDATA_SYSTEM_KERNEL_VERSION", system_info->kernel_version, &host->host_id.uuid);
    ret += add_host_sysinfo_key_value("NETDATA_SYSTEM_ARCHITECTURE", system_info->architecture, &host->host_id.uuid);
    ret += add_host_sysinfo_key_value("NETDATA_SYSTEM_VIRTUALIZATION", system_info->virtualization, &host->host_id.uuid);
    ret += add_host_sysinfo_key_value("NETDATA_SYSTEM_VIRT_DETECTION", system_info->virt_detection, &host->host_id.uuid);
    ret += add_host_sysinfo_key_value("NETDATA_SYSTEM_CONTAINER", system_info->container, &host->host_id.uuid);
    ret += add_host_sysinfo_key_value("NETDATA_SYSTEM_CONTAINER_DETECTION", system_info->container_detection, &host->host_id.uuid);
    ret += add_host_sysinfo_key_value("NETDATA_HOST_IS_K8S_NODE", system_info->is_k8s_node, &host->host_id.uuid);

    return !(24 == ret);
}


/*
 * Store a chart in the database
 */

static int store_chart_metadata(RRDSET *st)
{
    static __thread sqlite3_stmt *res = NULL;

    if (!PREPARE_COMPILED_STATEMENT(db_meta, SQL_STORE_CHART, &res))
        return 1;

    int param = 0;
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_blob(res, ++param, &st->chart_uuid, sizeof(st->chart_uuid), SQLITE_STATIC));
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_blob(res, ++param, &st->rrdhost->host_id.uuid, sizeof(st->rrdhost->host_id.uuid), SQLITE_STATIC));
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_text(res, ++param, string2str(st->parts.type), -1, SQLITE_STATIC));
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_text(res, ++param, string2str(st->parts.id), -1, SQLITE_STATIC));

    const char *name = string2str(st->parts.name);
    if (name && *name)
        SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_text(res, ++param, name, -1, SQLITE_STATIC));
    else
        SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_null(res, ++param));

    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_text(res, ++param, rrdset_family(st), -1, SQLITE_STATIC));
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_text(res, ++param, rrdset_context(st), -1, SQLITE_STATIC));
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_text(res, ++param, rrdset_title(st), -1, SQLITE_STATIC));
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_text(res, ++param, rrdset_units(st), -1, SQLITE_STATIC));
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_text(res, ++param, rrdset_plugin_name(st), -1, SQLITE_STATIC));
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_text(res, ++param, rrdset_module_name(st), -1, SQLITE_STATIC));
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_int(res, ++param, (int) st->priority));
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_int(res, ++param, st->update_every));
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_int(res, ++param, st->chart_type));
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_int(res, ++param, st->rrd_memory_mode));
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_int(res, ++param, (int) st->db.entries));

    int store_rc = execute_insert(res);
    if (unlikely(store_rc != SQLITE_DONE))
        error_report("Failed to store chart, rc = %d", store_rc);

    SQLITE_RESET(res);

    return store_rc != SQLITE_DONE;

bind_fail:
    REPORT_BIND_FAIL(res, param);
    SQLITE_RESET(res);
    return 1;
}

/*
 * Store a dimension
 */
static int store_dimension_metadata(RRDDIM *rd)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc, param = 0;

    if (!PREPARE_COMPILED_STATEMENT(db_meta, SQL_STORE_DIMENSION, &res))
        return 1;

    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_blob(res, ++param, &rd->metric_uuid, sizeof(rd->metric_uuid), SQLITE_STATIC));
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_blob(res, ++param, &rd->rrdset->chart_uuid, sizeof(rd->rrdset->chart_uuid), SQLITE_STATIC));
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_text(res, ++param, string2str(rd->id), -1, SQLITE_STATIC));
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_text(res, ++param, string2str(rd->name), -1, SQLITE_STATIC));
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_int(res, ++param, (int) rd->multiplier));
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_int(res, ++param, (int ) rd->divisor));
    SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_int(res, ++param, rd->algorithm));
    if (rrddim_option_check(rd, RRDDIM_OPTION_HIDDEN))
        SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_text(res, ++param, "hidden", -1, SQLITE_STATIC));
    else
        SQLITE_BIND_FAIL(bind_fail, sqlite3_bind_null(res, ++param));

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to store dimension, rc = %d", rc);

    SQLITE_RESET(res);
    return 0;

bind_fail:
    REPORT_BIND_FAIL(res, param);
    SQLITE_RESET(res);
    return 1;
}

static bool dimension_can_be_deleted(nd_uuid_t *dim_uuid __maybe_unused, sqlite3_stmt **res __maybe_unused, bool flag __maybe_unused)
{
#ifdef ENABLE_DBENGINE
    if(dbengine_enabled) {
        bool no_retention = true;
        for (size_t tier = 0; tier < storage_tiers; tier++) {
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
    struct metadata_wc *wc,
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
    if (unlikely(metadata_flag_check(wc, METADATA_FLAG_SHUTDOWN)))
        return true;

    int rc = sqlite3_bind_int64(res, 1, (sqlite3_int64) *row_id);
    if (unlikely(rc != SQLITE_OK))
        return true;

    time_t start_running = now_monotonic_sec();
    bool time_expired = false;
    while (!time_expired && sqlite3_step_monitored(res) == SQLITE_ROW &&
           (*total_deleted < MAX_METADATA_CLEANUP && *total_checked < MAX_METADATA_CLEANUP)) {
        if (unlikely(metadata_flag_check(wc, METADATA_FLAG_SHUTDOWN)))
            break;

        *row_id = sqlite3_column_int64(res, 1);
        rc = check_cb((nd_uuid_t *)sqlite3_column_blob(res, 0), check_stmt, check_flag);

        if (rc == true) {
            action_cb((nd_uuid_t *)sqlite3_column_blob(res, 0), action_stmt, action_flag);
            (*total_deleted)++;
        }

        (*total_checked)++;
        time_expired = ((now_monotonic_sec() - start_running) > METADATA_RUNTIME_THRESHOLD);
    }
    return time_expired || (*total_checked == MAX_METADATA_CLEANUP) || (*total_deleted == MAX_METADATA_CLEANUP);
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

static void check_dimension_metadata(struct metadata_wc *wc)
{
    static time_t next_execution_t = 0;
    static uint64_t last_row_id = 0;

    time_t now = now_realtime_sec();

    if (!next_execution_t)
        next_execution_t = now + METADATA_MAINTENANCE_FIRST_CHECK;

    if (next_execution_t && next_execution_t > now)
        return;

    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_meta, SELECT_DIMENSION_LIST, &res))
        return;

    uint32_t total_checked = 0;
    uint32_t total_deleted = 0;

    nd_log(NDLS_DAEMON, NDLP_DEBUG, "Checking dimensions starting after row %" PRIu64, last_row_id);

    bool more_to_do = run_cleanup_loop(
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
    if (more_to_do)
        next_execution_t = now + METADATA_MAINTENANCE_REPEAT;
    else {
        last_row_id = 0;
        next_execution_t = now + METADATA_DIM_CHECK_INTERVAL;
    }

    nd_log(
        NDLS_DAEMON,
        NDLP_DEBUG,
        "Dimensions checked %u, deleted %u. Checks will %s in %lld seconds",
        total_checked,
        total_deleted,
        last_row_id ? "resume" : "restart",
        (long long)(next_execution_t - now));

    SQLITE_FINALIZE(res);
}

static void check_chart_metadata(struct metadata_wc *wc)
{
    static time_t next_execution_t = 0;
    static uint64_t last_row_id = 0;

    time_t now = now_realtime_sec();

    if (!next_execution_t)
        next_execution_t = now + METADATA_MAINTENANCE_FIRST_CHECK;

    if (next_execution_t && next_execution_t > now)
        return;

    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_meta, SELECT_CHART_LIST, &res))
        return;

    uint32_t total_checked = 0;
    uint32_t total_deleted = 0;

    nd_log(NDLS_DAEMON, NDLP_DEBUG, "Checking charts starting after row %" PRIu64, last_row_id);

    sqlite3_stmt *check_res = NULL;
    sqlite3_stmt *action_res = NULL;
    bool more_to_do = run_cleanup_loop(
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
    if (more_to_do)
        next_execution_t = now + METADATA_MAINTENANCE_REPEAT;
    else {
        last_row_id = 0;
        next_execution_t = now + METADATA_CHART_CHECK_INTERVAL;
    }

    nd_log(
        NDLS_DAEMON,
        NDLP_DEBUG,
        "Charts checked %u, deleted %u. Checks will %s in %lld seconds",
        total_checked,
        total_deleted,
        last_row_id ? "resume" : "restart",
        (long long)(next_execution_t - now));

    SQLITE_FINALIZE(res);
}

static void check_label_metadata(struct metadata_wc *wc)
{
    static time_t next_execution_t = 0;
    static uint64_t last_row_id = 0;

    time_t now = now_realtime_sec();

    if (!next_execution_t)
        next_execution_t = now + METADATA_MAINTENANCE_FIRST_CHECK;

    if (next_execution_t && next_execution_t > now)
        return;

    sqlite3_stmt *res = NULL;

    if (!PREPARE_STATEMENT(db_meta, SELECT_CHART_LABEL_LIST, &res))
        return;

    uint32_t total_checked = 0;
    uint32_t total_deleted = 0;

    nd_log(NDLS_DAEMON, NDLP_DEBUG, "Checking charts labels starting after row %" PRIu64, last_row_id);

    sqlite3_stmt *check_res = NULL;
    sqlite3_stmt *action_res = NULL;

    bool more_to_do = run_cleanup_loop(
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
    if (more_to_do)
        next_execution_t = now + METADATA_MAINTENANCE_REPEAT;
    else {
        last_row_id = 0;
        next_execution_t = now + METADATA_LABEL_CHECK_INTERVAL;
    }

    nd_log(
        NDLS_DAEMON,
        NDLP_DEBUG,
        "Chart labels checked %u, deleted %u. Checks will %s in %lld seconds",
        total_checked,
        total_deleted,
        last_row_id ? "resume" : "restart",
        (long long)(next_execution_t - now));

    SQLITE_FINALIZE(res);
}


static void cleanup_health_log(struct metadata_wc *wc)
{
    static time_t next_execution_t = 0;

    time_t now = now_realtime_sec();

    if (!next_execution_t)
        next_execution_t = now + METADATA_MAINTENANCE_FIRST_CHECK;

    if (next_execution_t && next_execution_t > now)
        return;

    next_execution_t = now + METADATA_HEALTH_LOG_INTERVAL;

    RRDHOST *host;

    dfe_start_reentrant(rrdhost_root_index, host){
        if (rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED))
            continue;
        sql_health_alarm_log_cleanup(host);
        if (unlikely(metadata_flag_check(wc, METADATA_FLAG_SHUTDOWN)))
            break;
    }
    dfe_done(host);

    if (unlikely(metadata_flag_check(wc, METADATA_FLAG_SHUTDOWN)))
        return;

    (void) db_execute(db_meta,"DELETE FROM health_log WHERE host_id NOT IN (SELECT host_id FROM host)");
    (void) db_execute(db_meta,"DELETE FROM health_log_detail WHERE health_log_id NOT IN (SELECT health_log_id FROM health_log)");
    (void) db_execute(db_meta,"DELETE FROM alert_version WHERE health_log_id NOT IN (SELECT health_log_id FROM health_log)");
}

//
// EVENT LOOP STARTS HERE
//

static void metadata_free_cmd_queue(struct metadata_wc *wc)
{
    spinlock_lock(&wc->cmd_queue_lock);
    while(wc->cmd_base) {
        struct metadata_cmd *t = wc->cmd_base;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(wc->cmd_base, t, prev, next);
        freez(t);
    }
    spinlock_unlock(&wc->cmd_queue_lock);
}

static void metadata_enq_cmd(struct metadata_wc *wc, struct metadata_cmd *cmd)
{
    if (cmd->opcode == METADATA_SYNC_SHUTDOWN) {
        metadata_flag_set(wc, METADATA_FLAG_SHUTDOWN);
        goto wakeup_event_loop;
    }

    if (unlikely(metadata_flag_check(wc, METADATA_FLAG_SHUTDOWN)))
        goto wakeup_event_loop;

    struct metadata_cmd *t = mallocz(sizeof(*t));
    *t = *cmd;
    t->prev = t->next = NULL;

    spinlock_lock(&wc->cmd_queue_lock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(wc->cmd_base, t, prev, next);
    spinlock_unlock(&wc->cmd_queue_lock);

wakeup_event_loop:
    (void) uv_async_send(&wc->async);
}

static struct metadata_cmd metadata_deq_cmd(struct metadata_wc *wc)
{
    struct metadata_cmd ret;

    spinlock_lock(&wc->cmd_queue_lock);
    if(wc->cmd_base) {
        struct metadata_cmd *t = wc->cmd_base;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(wc->cmd_base, t, prev, next);
        ret = *t;
        freez(t);
    }
    else {
        ret.opcode = METADATA_DATABASE_NOOP;
        ret.completion = NULL;
    }
    spinlock_unlock(&wc->cmd_queue_lock);

    return ret;
}

static void async_cb(uv_async_t *handle)
{
    uv_stop(handle->loop);
    uv_update_time(handle->loop);
}

#define TIMER_INITIAL_PERIOD_MS (1000)
#define TIMER_REPEAT_PERIOD_MS (1000)

static void timer_cb(uv_timer_t* handle)
{
    uv_stop(handle->loop);
    uv_update_time(handle->loop);

   struct metadata_wc *wc = handle->data;
   struct metadata_cmd cmd;
   memset(&cmd, 0, sizeof(cmd));

   if (wc->metadata_check_after <  now_realtime_sec()) {
       cmd.opcode = METADATA_SCAN_HOSTS;
       metadata_enq_cmd(wc, &cmd);
   }
}

void vacuum_database(sqlite3 *database, const char *db_alias, int threshold, int vacuum_pc)
{
   int free_pages = get_free_page_count(database);
   int total_pages = get_database_page_count(database);

   if (!threshold)
       threshold = DATABASE_FREE_PAGES_THRESHOLD_PC;

   if (!vacuum_pc)
       vacuum_pc = DATABASE_FREE_PAGES_VACUUM_PC;

   if (free_pages > (total_pages * threshold / 100)) {

       int do_free_pages = (int) (free_pages * vacuum_pc / 100);
       nd_log(NDLS_DAEMON, NDLP_DEBUG, "%s: Freeing %d database pages", db_alias, do_free_pages);

       char sql[128];
       snprintfz(sql, sizeof(sql) - 1, "PRAGMA incremental_vacuum(%d)", do_free_pages);
       (void) db_execute(database, sql);
   }
}

void run_metadata_cleanup(struct metadata_wc *wc)
{
    if (unlikely(metadata_flag_check(wc, METADATA_FLAG_SHUTDOWN)))
       return;

    check_dimension_metadata(wc);
    check_chart_metadata(wc);
    check_label_metadata(wc);
    cleanup_health_log(wc);

    if (unlikely(metadata_flag_check(wc, METADATA_FLAG_SHUTDOWN)))
       return;

    vacuum_database(db_meta, "METADATA", DATABASE_FREE_PAGES_THRESHOLD_PC, DATABASE_FREE_PAGES_VACUUM_PC);

    (void) sqlite3_wal_checkpoint(db_meta, NULL);
}

struct scan_metadata_payload {
    uv_work_t request;
    struct metadata_wc *wc;
    void *chart_label_cleanup;
    void *pending_alert_list;
    BUFFER *work_buffer;
    uint32_t max_count;
};

struct host_context_load_thread {
    uv_thread_t thread;
    RRDHOST *host;
    bool busy;
    bool finished;
};

static void restore_host_context(void *arg)
{
    struct host_context_load_thread *hclt = arg;
    RRDHOST *host = hclt->host;

    usec_t started_ut = now_monotonic_usec(); (void)started_ut;
    rrdhost_load_rrdcontext_data(host);
    usec_t ended_ut = now_monotonic_usec(); (void)ended_ut;

    rrdhost_flag_clear(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD);

    aclk_queue_node_info(host, false);

    nd_log(
        NDLS_DAEMON,
        NDLP_DEBUG,
        "Contexts for host %s loaded in %0.2f ms",
        rrdhost_hostname(host),
        (double)(ended_ut - started_ut) / USEC_PER_MS);

    __atomic_store_n(&hclt->finished, true, __ATOMIC_RELEASE);
}

// Callback after scan of hosts is done
static void after_start_host_load_context(uv_work_t *req, int status __maybe_unused)
{
    struct scan_metadata_payload *data = req->data;
    freez(data);
}

#define MAX_FIND_THREAD_RETRIES (10)

static void cleanup_finished_threads(struct host_context_load_thread *hclt, size_t max_thread_slots, bool wait)
{
    if (!hclt)
        return;

    for (size_t index = 0; index < max_thread_slots; index++) {
       if (__atomic_load_n(&(hclt[index].finished), __ATOMIC_RELAXED)
           || (wait && __atomic_load_n(&(hclt[index].busy), __ATOMIC_ACQUIRE))) {
           int rc = uv_thread_join(&(hclt[index].thread));
           if (rc)
               nd_log(NDLS_DAEMON, NDLP_WARNING, "Failed to join thread, rc = %d", rc);
           __atomic_store_n(&(hclt[index].busy), false, __ATOMIC_RELEASE);
           __atomic_store_n(&(hclt[index].finished), false, __ATOMIC_RELEASE);
       }
    }
}

static size_t find_available_thread_slot(struct host_context_load_thread *hclt, size_t max_thread_slots, size_t *found_index)
{
    size_t retries = MAX_FIND_THREAD_RETRIES;
    while (retries--) {
       size_t index = 0;
       while (index < max_thread_slots) {
           if (false == __atomic_load_n(&(hclt[index].busy), __ATOMIC_ACQUIRE)) {
                *found_index = index;
                return true;
           }
           index++;
       }
       sleep_usec(10 * USEC_PER_MS);
    }
    return false;
}

static void start_all_host_load_context(uv_work_t *req __maybe_unused)
{
    register_libuv_worker_jobs();

    struct scan_metadata_payload *data = req->data;
    struct metadata_wc *wc = data->wc;

    worker_is_busy(UV_EVENT_HOST_CONTEXT_LOAD);
    usec_t started_ut = now_monotonic_usec(); (void)started_ut;

    RRDHOST *host;

    size_t max_threads = MIN(get_netdata_cpus() / 2, 6);
    if (max_threads < 1)
        max_threads = 1;

    nd_log(NDLS_DAEMON, NDLP_DEBUG, "Using %zu threads for context loading", max_threads);
    struct host_context_load_thread *hclt = max_threads > 1 ? callocz(max_threads, sizeof(*hclt)) : NULL;

    size_t thread_index = 0;
    dfe_start_reentrant(rrdhost_root_index, host) {
       if (!rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD))
           continue;

       nd_log(NDLS_DAEMON, NDLP_DEBUG, "Loading context for host %s", rrdhost_hostname(host));

       int rc = 0;
       if (hclt) {
           bool found_slot = false;
           do {
               if (metadata_flag_check(wc, METADATA_FLAG_SHUTDOWN))
                   break;

               cleanup_finished_threads(hclt, max_threads, false);
               found_slot = find_available_thread_slot(hclt, max_threads, &thread_index);
           } while (!found_slot);

           if (metadata_flag_check(wc, METADATA_FLAG_SHUTDOWN))
               break;

           __atomic_store_n(&hclt[thread_index].busy, true, __ATOMIC_RELAXED);
           hclt[thread_index].host = host;
           rc = uv_thread_create(&hclt[thread_index].thread, restore_host_context, &hclt[thread_index]);
       }
       // if single thread or thread creation failed
       if (rc || !hclt) {
           struct host_context_load_thread hclt_sync = {.host = host};
           restore_host_context(&hclt_sync);

           if (metadata_flag_check(wc, METADATA_FLAG_SHUTDOWN))
               break;
       }
    }
    dfe_done(host);

    cleanup_finished_threads(hclt, max_threads, true);
    freez(hclt);
    usec_t ended_ut = now_monotonic_usec(); (void)ended_ut;
    nd_log(NDLS_DAEMON, NDLP_DEBUG, "Host contexts loaded in %0.2f ms", (double)(ended_ut - started_ut) / USEC_PER_MS);

    worker_is_idle();
}

// Callback after scan of hosts is done
static void after_metadata_hosts(uv_work_t *req, int status __maybe_unused)
{
    struct scan_metadata_payload *data = req->data;
    struct metadata_wc *wc = data->wc;

    bool first = false;
    Word_t Index = 0;
    Pvoid_t *Pvalue;
    while ((Pvalue = JudyLFirstThenNext(wc->ae_DelJudyL, &Index, &first))) {
        ALARM_ENTRY *ae = (ALARM_ENTRY *) Index;
        if(!__atomic_load_n(&ae->pending_save_count, __ATOMIC_RELAXED)) {
            health_alarm_log_free_one_nochecks_nounlink(ae);
            (void) JudyLDel(&wc->ae_DelJudyL, Index, PJE0);
            first = false;
            Index = 0;
        }
    }

    metadata_flag_clear(wc, METADATA_FLAG_PROCESSING);

    if (unlikely(wc->scan_complete))
        completion_mark_complete(wc->scan_complete);

    freez(data);
}

static bool metadata_scan_host(RRDHOST *host, uint32_t max_count, bool use_transaction, BUFFER *work_buffer, size_t *query_counter) {
    RRDSET *st;
    int rc;

    bool more_to_do = false;
    uint32_t scan_count = 1;

    sqlite3_stmt *ml_load_stmt = NULL;

    bool load_ml_models = max_count;

    if (use_transaction)
        (void)db_execute(db_meta, "BEGIN TRANSACTION");

    rrdset_foreach_reentrant(st, host) {
        if (scan_count == max_count) {
            more_to_do = true;
            break;
        }
        if(rrdset_flag_check(st, RRDSET_FLAG_METADATA_UPDATE)) {
            (*query_counter)++;

            rrdset_flag_clear(st, RRDSET_FLAG_METADATA_UPDATE);
            scan_count++;

            buffer_flush(work_buffer);
            rc = check_and_update_chart_labels(st, work_buffer, query_counter);
            if (unlikely(rc))
                error_report("METADATA: 'host:%s': Failed to update labels for chart %s", rrdhost_hostname(host), rrdset_name(st));
            else
                (*query_counter)++;

            rc = store_chart_metadata(st);
            if (unlikely(rc))
               error_report("METADATA: 'host:%s': Failed to store metadata for chart %s", rrdhost_hostname(host), rrdset_name(st));
        }

        RRDDIM *rd;
        rrddim_foreach_read(rd, st) {
            if(rrddim_flag_check(rd, RRDDIM_FLAG_METADATA_UPDATE)) {
                (*query_counter)++;

                rrddim_flag_clear(rd, RRDDIM_FLAG_METADATA_UPDATE);

                if (rrddim_option_check(rd, RRDDIM_OPTION_HIDDEN))
                    rrddim_flag_set(rd, RRDDIM_FLAG_META_HIDDEN);
                else
                    rrddim_flag_clear(rd, RRDDIM_FLAG_META_HIDDEN);

                rc = store_dimension_metadata(rd);
                if (unlikely(rc))
                    error_report("METADATA: 'host:%s': Failed to dimension metadata for chart %s. dimension %s",
                                 rrdhost_hostname(host), rrdset_name(st),
                                 rrddim_name(rd));
            }

            if(rrddim_flag_check(rd, RRDDIM_FLAG_ML_MODEL_LOAD)) {
                rrddim_flag_clear(rd, RRDDIM_FLAG_ML_MODEL_LOAD);
                if (likely(load_ml_models))
                    (void) ml_dimension_load_models(rd, &ml_load_stmt);
            }

            worker_is_idle();
        }
        rrddim_foreach_done(rd);
    }
    rrdset_foreach_done(st);

    if (use_transaction)
        (void)db_execute(db_meta, "COMMIT TRANSACTION");

    SQLITE_FINALIZE(ml_load_stmt);
    ml_load_stmt = NULL;

    return more_to_do;
}

static void store_host_and_system_info(RRDHOST *host, size_t *query_counter)
{
    if (unlikely(store_host_systeminfo(host))) {
        error_report("METADATA: 'host:%s': Failed to store host updated system information in the database", rrdhost_hostname(host));
        rrdhost_flag_set(host, RRDHOST_FLAG_METADATA_INFO | RRDHOST_FLAG_METADATA_UPDATE);
    }
    else {
        if (likely(query_counter))
            (*query_counter)++;
    }

    if (unlikely(store_host_metadata(host))) {
        error_report("METADATA: 'host:%s': Failed to store host info in the database", rrdhost_hostname(host));
        rrdhost_flag_set(host, RRDHOST_FLAG_METADATA_INFO | RRDHOST_FLAG_METADATA_UPDATE);
    }
    else {
        if (likely(query_counter))
            (*query_counter)++;
    }
}

struct judy_list_t {
    Pvoid_t JudyL;
    Word_t count;
};

static void store_alert_transitions(struct judy_list_t *pending_alert_list)
{
    if (!pending_alert_list)
        return;

    usec_t started_ut = now_monotonic_usec(); (void)started_ut;

    size_t entries = pending_alert_list->count;
    Word_t Index = 0;
    bool first = true;
    Pvoid_t *PValue;
    while ((PValue = JudyLFirstThenNext(pending_alert_list->JudyL, &Index, &first))) {
        RRDHOST *host = *PValue;

        PValue = JudyLGet(pending_alert_list->JudyL, ++Index, PJE0);
        ALARM_ENTRY *ae = *PValue;

        sql_health_alarm_log_save(host, ae);

        __atomic_add_fetch(&ae->pending_save_count, -1, __ATOMIC_RELAXED);
        __atomic_add_fetch(&host->health.pending_transitions, -1, __ATOMIC_RELAXED);
    }
    (void) JudyLFreeArray(&pending_alert_list->JudyL, PJE0);
    freez(pending_alert_list);

    usec_t ended_ut = now_monotonic_usec(); (void)ended_ut;
    nd_log(
        NDLS_DAEMON,
        NDLP_DEBUG,
        "Stored and processed %zu alert transitions in %0.2f ms",
        entries,
        (double)(ended_ut - started_ut) / USEC_PER_MS);
}

static void do_chart_label_cleanup(struct judy_list_t *cl_cleanup_data)
{
    if (!cl_cleanup_data)
        return;

    Word_t Index = 0;
    bool first = true;
    Pvoid_t *PValue;
    while ((PValue = JudyLFirstThenNext(cl_cleanup_data->JudyL, &Index, &first))) {
        char *machine_guid = *PValue;

        RRDHOST *host = rrdhost_find_by_guid(machine_guid);
        if (likely(!host)) {
            nd_uuid_t host_uuid;
            if (!uuid_parse(machine_guid, host_uuid))
                delete_host_chart_labels(&host_uuid);
        }

        freez(machine_guid);
    }
    JudyLFreeArray(&cl_cleanup_data->JudyL, PJE0);
    freez(cl_cleanup_data);
}

// Worker thread to scan hosts for pending metadata to store
static void start_metadata_hosts(uv_work_t *req __maybe_unused)
{
    register_libuv_worker_jobs();

    RRDHOST *host;
    int transaction_started = 0;

    struct scan_metadata_payload *data = req->data;
    struct metadata_wc *wc = data->wc;

    BUFFER *work_buffer = data->work_buffer;
    usec_t all_started_ut = now_monotonic_usec(); (void)all_started_ut;
    nd_log(NDLS_DAEMON, NDLP_DEBUG, "Checking all hosts started");
    usec_t started_ut = now_monotonic_usec(); (void)started_ut;

    store_alert_transitions((struct judy_list_t *)data->pending_alert_list);
    do_chart_label_cleanup((struct judy_list_t *)data->chart_label_cleanup);

    bool run_again = false;
    worker_is_busy(UV_EVENT_METADATA_STORE);

    if (!data->max_count)
        transaction_started = !db_execute(db_meta, "BEGIN TRANSACTION");

    dfe_start_reentrant(rrdhost_root_index, host) {

        if (rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED) || !rrdhost_flag_check(host, RRDHOST_FLAG_METADATA_UPDATE))
            continue;

        size_t query_counter = 0; (void)query_counter;

        rrdhost_flag_clear(host,RRDHOST_FLAG_METADATA_UPDATE);

        if (unlikely(rrdhost_flag_check(host, RRDHOST_FLAG_METADATA_LABELS))) {
            rrdhost_flag_clear(host, RRDHOST_FLAG_METADATA_LABELS);

            int rc = exec_statement_with_uuid(SQL_DELETE_HOST_LABELS, &host->host_id.uuid);
            if (likely(!rc)) {
                query_counter++;

                buffer_flush(work_buffer);
                struct query_build tmp = {.sql = work_buffer, .count = 0};
                uuid_unparse_lower(host->host_id.uuid, tmp.uuid_str);
                rrdlabels_walkthrough_read(host->rrdlabels, host_label_store_to_sql_callback, &tmp);
                buffer_strcat(work_buffer, " ON CONFLICT (host_id, label_key) DO UPDATE SET source_type = excluded.source_type, label_value=excluded.label_value, date_created=UNIXEPOCH()");
                rc = db_execute(db_meta, buffer_tostring(work_buffer));

                if (unlikely(rc)) {
                    error_report("METADATA: 'host:%s': failed to update metadata host labels", rrdhost_hostname(host));
                    rrdhost_flag_set(host, RRDHOST_FLAG_METADATA_LABELS | RRDHOST_FLAG_METADATA_UPDATE);
                }
                else
                    query_counter++;
            } else {
                error_report("METADATA: 'host:%s': failed to delete old host labels", rrdhost_hostname(host));
                rrdhost_flag_set(host, RRDHOST_FLAG_METADATA_LABELS | RRDHOST_FLAG_METADATA_UPDATE);
            }
        }

        if (unlikely(rrdhost_flag_check(host, RRDHOST_FLAG_METADATA_CLAIMID))) {
            rrdhost_flag_clear(host, RRDHOST_FLAG_METADATA_CLAIMID);
            int rc;
            ND_UUID uuid = claim_id_get_uuid();
            if(!UUIDiszero(uuid))
                rc = store_claim_id(&host->host_id.uuid, &uuid.uuid);
            else
                rc = store_claim_id(&host->host_id.uuid, NULL);

            if (unlikely(rc))
                rrdhost_flag_set(host, RRDHOST_FLAG_METADATA_CLAIMID | RRDHOST_FLAG_METADATA_UPDATE);
            else
                query_counter++;
        }
        if (unlikely(rrdhost_flag_check(host, RRDHOST_FLAG_METADATA_INFO))) {
            rrdhost_flag_clear(host, RRDHOST_FLAG_METADATA_INFO);
            store_host_and_system_info(host, &query_counter);
        }

        // For clarity
        bool use_transaction = data->max_count;
        if (unlikely(metadata_scan_host(host, data->max_count, use_transaction, work_buffer, &query_counter))) {
            run_again = true;
            rrdhost_flag_set(host,RRDHOST_FLAG_METADATA_UPDATE);
        }
        usec_t ended_ut = now_monotonic_usec(); (void)ended_ut;
        nd_log(
            NDLS_DAEMON,
            NDLP_DEBUG,
            "Host %s saved metadata with %zu SQL statements, in %0.2f ms",
            rrdhost_hostname(host),
            query_counter,
            (double)(ended_ut - started_ut) / USEC_PER_MS);
    }
    dfe_done(host);

    if (!data->max_count && transaction_started)
        transaction_started = db_execute(db_meta, "COMMIT TRANSACTION");

    usec_t all_ended_ut = now_monotonic_usec(); (void)all_ended_ut;
    nd_log(
        NDLS_DAEMON,
        NDLP_DEBUG,
        "Checking all hosts completed in %0.2f ms",
        (double)(all_ended_ut - all_started_ut) / USEC_PER_MS);

    if (likely(!run_again))
        run_metadata_cleanup(wc);

    wc->metadata_check_after = now_realtime_sec() + METADATA_HOST_CHECK_INTERVAL;
    worker_is_idle();
}

static void metadata_event_loop(void *arg)
{
    worker_register("METASYNC");
    worker_register_job_name(METADATA_DATABASE_NOOP,        "noop");
    worker_register_job_name(METADATA_DATABASE_TIMER,       "timer");
    worker_register_job_name(METADATA_DEL_DIMENSION,        "delete dimension");
    worker_register_job_name(METADATA_STORE_CLAIM_ID,       "add claim id");
    worker_register_job_name(METADATA_ADD_HOST_INFO,        "add host info");
    worker_register_job_name(METADATA_MAINTENANCE,          "maintenance");

    int ret;
    uv_loop_t *loop;
    unsigned cmd_batch_size;
    struct metadata_wc *wc = arg;
    enum metadata_opcode opcode;

    uv_thread_set_name_np("METASYNC");
    loop = wc->loop = mallocz(sizeof(uv_loop_t));
    ret = uv_loop_init(loop);
    if (ret) {
        netdata_log_error("uv_loop_init(): %s", uv_strerror(ret));
        goto error_after_loop_init;
    }
    loop->data = wc;

    ret = uv_async_init(wc->loop, &wc->async, async_cb);
    if (ret) {
        netdata_log_error("uv_async_init(): %s", uv_strerror(ret));
        goto error_after_async_init;
    }
    wc->async.data = wc;

    ret = uv_timer_init(loop, &wc->timer_req);
    if (ret) {
        netdata_log_error("uv_timer_init(): %s", uv_strerror(ret));
        goto error_after_timer_init;
    }
    wc->timer_req.data = wc;
    fatal_assert(0 == uv_timer_start(&wc->timer_req, timer_cb, TIMER_INITIAL_PERIOD_MS, TIMER_REPEAT_PERIOD_MS));

    nd_log(NDLS_DAEMON, NDLP_DEBUG, "Starting metadata sync thread");

    struct metadata_cmd cmd;
    memset(&cmd, 0, sizeof(cmd));
    metadata_flag_clear(wc, METADATA_FLAG_PROCESSING);

    wc->metadata_check_after = now_realtime_sec() + METADATA_HOST_CHECK_FIRST_CHECK;

    int shutdown = 0;
    completion_mark_complete(&wc->start_stop_complete);
    BUFFER *work_buffer = buffer_create(1024, &netdata_buffers_statistics.buffers_sqlite);
    struct scan_metadata_payload *data;
    struct judy_list_t *cl_cleanup_data = NULL;
    Pvoid_t *PValue;
    struct judy_list_t *pending_ae_list = NULL;

    while (shutdown == 0 || (wc->flags & METADATA_FLAG_PROCESSING)) {
        nd_uuid_t  *uuid;
        RRDHOST *host = NULL;
        ALARM_ENTRY *ae = NULL;
//        struct aclk_sync_cfg_t *host_aclk_sync;

        worker_is_idle();
        uv_run(loop, UV_RUN_DEFAULT);

        /* wait for commands */
        cmd_batch_size = 0;
        do {
            if (unlikely(cmd_batch_size >= METADATA_MAX_BATCH_SIZE))
                break;

            cmd = metadata_deq_cmd(wc);
            opcode = cmd.opcode;

            if (unlikely(opcode == METADATA_DATABASE_NOOP && metadata_flag_check(wc, METADATA_FLAG_SHUTDOWN))) {
                shutdown = 1;
                continue;
            }

            ++cmd_batch_size;

            if (likely(opcode != METADATA_DATABASE_NOOP))
                worker_is_busy(opcode);

            switch (opcode) {
                case METADATA_DATABASE_NOOP:
                case METADATA_DATABASE_TIMER:
                    break;
                case METADATA_DEL_DIMENSION:
                    uuid = (nd_uuid_t *) cmd.param[0];
                    if (likely(dimension_can_be_deleted(uuid, NULL, false)))
                        delete_dimension_uuid(uuid, NULL, false);
                    freez(uuid);
                    break;
                case METADATA_STORE_CLAIM_ID:
                    store_claim_id((nd_uuid_t *) cmd.param[0], (nd_uuid_t *) cmd.param[1]);
                    freez((void *) cmd.param[0]);
                    freez((void *) cmd.param[1]);
                    break;
                case METADATA_ADD_HOST_INFO:
                    host = (RRDHOST *) cmd.param[0];
                    store_host_and_system_info(host, NULL);
                    break;
                case METADATA_SCAN_HOSTS:
                    if (unlikely(metadata_flag_check(wc, METADATA_FLAG_PROCESSING)))
                        break;

                    if (unittest_running)
                        break;

                    data = mallocz(sizeof(*data));
                    data->request.data = data;
                    data->wc = wc;
                    data->chart_label_cleanup = cl_cleanup_data;
                    data->pending_alert_list = pending_ae_list;
                    data->work_buffer = work_buffer;
                    cl_cleanup_data = NULL;
                    pending_ae_list = NULL;

                    if (unlikely(cmd.completion)) {
                        data->max_count = 0;            // 0 will process all pending updates
                        cmd.completion = NULL;          // Do not complete after launching worker (worker will do)
                    }
                    else
                        data->max_count = 5000;

                    metadata_flag_set(wc, METADATA_FLAG_PROCESSING);
                    if (uv_queue_work(loop, &data->request, start_metadata_hosts, after_metadata_hosts)) {
                        // Failed to launch worker -- let the event loop handle completion
                        cmd.completion = wc->scan_complete;
                        cl_cleanup_data = data->chart_label_cleanup;
                        pending_ae_list = data->pending_alert_list;
                        freez(data);
                        metadata_flag_clear(wc, METADATA_FLAG_PROCESSING);
                    }
                    break;
                case METADATA_LOAD_HOST_CONTEXT:;
                    if (unittest_running)
                        break;

                    data = callocz(1,sizeof(*data));
                    data->request.data = data;
                    data->wc = wc;
                    if (uv_queue_work(loop, &data->request, start_all_host_load_context, after_start_host_load_context)) {
                        freez(data);
                    }
                    break;
                case METADATA_DELETE_HOST_CHART_LABELS:;
                    if (!cl_cleanup_data)
                        cl_cleanup_data = callocz(1,sizeof(*cl_cleanup_data));

                    PValue = JudyLIns(&cl_cleanup_data->JudyL, (Word_t) ++cl_cleanup_data->count, PJE0);
                    if (PValue)
                        *PValue = (void *) cmd.param[0];

                    break;
                case METADATA_ADD_HOST_AE:
                    host = (RRDHOST *) cmd.param[0];
                    ae = (ALARM_ENTRY *) cmd.param[1];

                    if (!pending_ae_list)
                        pending_ae_list = callocz(1, sizeof(*pending_ae_list));

                    PValue = JudyLIns(&pending_ae_list->JudyL, ++pending_ae_list->count, PJE0);
                    if (PValue)
                        *PValue = (void *)host;

                    PValue = JudyLIns(&pending_ae_list->JudyL, ++pending_ae_list->count, PJE0);
                    if (PValue)
                        *PValue = (void *)ae;
                    break;
                case METADATA_DEL_HOST_AE:;
                    (void) JudyLIns(&wc->ae_DelJudyL, (Word_t) (void *) cmd.param[0], PJE0);
                    break;
                case METADATA_UNITTEST:;
                    struct thread_unittest *tu = (struct thread_unittest *) cmd.param[0];
                    sleep_usec(1000); // processing takes 1ms
                    __atomic_fetch_add(&tu->processed, 1, __ATOMIC_SEQ_CST);
                    break;
                default:
                    break;
            }

            if (cmd.completion)
                completion_mark_complete(cmd.completion);
        } while (opcode != METADATA_DATABASE_NOOP);
    }

    if (!uv_timer_stop(&wc->timer_req))
        uv_close((uv_handle_t *)&wc->timer_req, NULL);

    uv_close((uv_handle_t *)&wc->async, NULL);
    int rc;
    do {
        rc = uv_loop_close(loop);
    } while (rc != UV_EBUSY);

    buffer_free(work_buffer);
    freez(loop);
    worker_unregister();

    nd_log(NDLS_DAEMON, NDLP_DEBUG, "Shutting down metadata thread");
    completion_mark_complete(&wc->start_stop_complete);
    if (wc->scan_complete) {
        completion_destroy(wc->scan_complete);
        freez(wc->scan_complete);
    }
    metadata_free_cmd_queue(wc);
    return;

error_after_timer_init:
    uv_close((uv_handle_t *)&wc->async, NULL);
error_after_async_init:
    fatal_assert(0 == uv_loop_close(loop));
error_after_loop_init:
    freez(loop);
    worker_unregister();
}

void metadata_sync_shutdown(void)
{
    completion_init(&metasync_worker.start_stop_complete);

    struct metadata_cmd cmd;
    memset(&cmd, 0, sizeof(cmd));
    nd_log(NDLS_DAEMON, NDLP_DEBUG, "METADATA: Sending a shutdown command");
    cmd.opcode = METADATA_SYNC_SHUTDOWN;
    metadata_enq_cmd(&metasync_worker, &cmd);

    /* wait for metadata thread to shut down */
    nd_log(NDLS_DAEMON, NDLP_DEBUG, "METADATA: Waiting for shutdown ACK");
    completion_wait_for(&metasync_worker.start_stop_complete);
    completion_destroy(&metasync_worker.start_stop_complete);
    nd_log(NDLS_DAEMON, NDLP_DEBUG, "METADATA: Shutdown complete");
}

void metadata_sync_shutdown_prepare(void)
{
    static bool running = false;
    if (unlikely(!metasync_worker.loop || running))
        return;

    running = true;

    struct metadata_cmd cmd;
    memset(&cmd, 0, sizeof(cmd));

    struct metadata_wc *wc = &metasync_worker;

    struct completion *compl = mallocz(sizeof(*compl));
    completion_init(compl);
    __atomic_store_n(&wc->scan_complete, compl, __ATOMIC_RELAXED);

    nd_log(NDLS_DAEMON, NDLP_DEBUG, "METADATA: Sending a scan host command");
    uint32_t max_wait_iterations = 2000;
    while (unlikely(metadata_flag_check(&metasync_worker, METADATA_FLAG_PROCESSING)) && max_wait_iterations--) {
        if (max_wait_iterations == 1999)
            nd_log(NDLS_DAEMON, NDLP_DEBUG, "METADATA: Current worker is running; waiting to finish");
        sleep_usec(1000);
    }

    cmd.opcode = METADATA_SCAN_HOSTS;
    metadata_enq_cmd(&metasync_worker, &cmd);

    nd_log(NDLS_DAEMON, NDLP_DEBUG, "METADATA: Waiting for host scan completion");
    completion_wait_for(wc->scan_complete);
    nd_log(NDLS_DAEMON, NDLP_DEBUG, "METADATA: Host scan complete; can continue with shutdown");
}

// -------------------------------------------------------------
// Init function called on agent startup

void metadata_sync_init(void)
{
    struct metadata_wc *wc = &metasync_worker;

    memset(wc, 0, sizeof(*wc));
    completion_init(&wc->start_stop_complete);

    fatal_assert(0 == uv_thread_create(&(wc->thread), metadata_event_loop, wc));

    completion_wait_for(&wc->start_stop_complete);
    completion_destroy(&wc->start_stop_complete);

    nd_log(NDLS_DAEMON, NDLP_DEBUG, "SQLite metadata sync initialization complete");
}


//  Helpers

static inline void queue_metadata_cmd(enum metadata_opcode opcode, const void *param0, const void *param1)
{
    struct metadata_cmd cmd;
    cmd.opcode = opcode;
    cmd.param[0] = param0;
    cmd.param[1] = param1;
    cmd.completion = NULL;
    metadata_enq_cmd(&metasync_worker, &cmd);
}

// Public
void metaqueue_delete_dimension_uuid(nd_uuid_t *uuid)
{
    if (unlikely(!metasync_worker.loop))
        return;
    nd_uuid_t *use_uuid = mallocz(sizeof(*uuid));
    uuid_copy(*use_uuid, *uuid);
    queue_metadata_cmd(METADATA_DEL_DIMENSION, use_uuid, NULL);
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
    queue_metadata_cmd(METADATA_STORE_CLAIM_ID, local_host_uuid, local_claim_uuid);
}

void metaqueue_host_update_info(RRDHOST *host)
{
    if (unlikely(!metasync_worker.loop))
        return;
    queue_metadata_cmd(METADATA_ADD_HOST_INFO, host, NULL);
}

void metaqueue_ml_load_models(RRDDIM *rd)
{
    rrddim_flag_set(rd, RRDDIM_FLAG_ML_MODEL_LOAD);
}

void metadata_queue_load_host_context(RRDHOST *host)
{
    if (unlikely(!metasync_worker.loop))
        return;
    queue_metadata_cmd(METADATA_LOAD_HOST_CONTEXT, host, NULL);
    nd_log(NDLS_DAEMON, NDLP_DEBUG, "Queued command to load host contexts");
}

void metadata_delete_host_chart_labels(char *machine_guid)
{
    if (unlikely(!metasync_worker.loop)) {
        freez(machine_guid);
        return;
    }

    // Node machine guid is already strdup-ed
    queue_metadata_cmd(METADATA_DELETE_HOST_CHART_LABELS, machine_guid, NULL);
    nd_log(NDLS_DAEMON, NDLP_DEBUG, "Queued command delete chart labels for host %s", machine_guid);
}

void metadata_queue_ae_save(RRDHOST *host, ALARM_ENTRY *ae)
{
    if (unlikely(!metasync_worker.loop))
        return;
    __atomic_add_fetch(&host->health.pending_transitions, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&ae->pending_save_count, 1, __ATOMIC_RELAXED);
    queue_metadata_cmd(METADATA_ADD_HOST_AE, host, ae);
}

void metadata_queue_ae_deletion(ALARM_ENTRY *ae)
{
    if (unlikely(!metasync_worker.loop))
        return;

    queue_metadata_cmd(METADATA_DEL_HOST_AE, ae, NULL);
}

void commit_alert_transitions(RRDHOST *host __maybe_unused)
{
    if (unlikely(!metasync_worker.loop))
        return;

    queue_metadata_cmd(METADATA_SCAN_HOSTS, NULL, NULL);
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
    (void) db_execute(db_meta, "DELETE FROM agent_event_log WHERE date_created < UNIXEPOCH() - 30 * 86400");
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

static void *unittest_queue_metadata(void *arg) {
    struct thread_unittest *tu = arg;

    struct metadata_cmd cmd;
    cmd.opcode = METADATA_UNITTEST;
    cmd.param[0] = tu;
    cmd.param[1] = NULL;
    cmd.completion = NULL;
    metadata_enq_cmd(&metasync_worker, &cmd);

    do {
        __atomic_fetch_add(&tu->added, 1, __ATOMIC_SEQ_CST);
        metadata_enq_cmd(&metasync_worker, &cmd);
        sleep_usec(10000);
    } while (!__atomic_load_n(&tu->join, __ATOMIC_RELAXED));
    return arg;
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
        threads[i] = nd_thread_create(
            buf,
            NETDATA_THREAD_OPTION_DONT_LOG | NETDATA_THREAD_OPTION_JOINABLE,
            unittest_queue_metadata,
            &tu);
    }
    (void) uv_async_send(&metasync_worker.async);
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

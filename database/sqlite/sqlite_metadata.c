// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_metadata.h"

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

#define MIGRATE_LOCALHOST_TO_NEW_MACHINE_GUID                                                                          \
    "UPDATE chart SET host_id = @host_id WHERE host_id in (SELECT host_id FROM host where host_id <> @host_id and hops = 0)"
#define DELETE_NON_EXISTING_LOCALHOST "DELETE FROM host WHERE hops = 0 AND host_id <> @host_id"
#define DELETE_MISSING_NODE_INSTANCES "DELETE FROM node_instance WHERE host_id NOT IN (SELECT host_id FROM host)"

#define METADATA_MAINTENANCE_FIRST_CHECK (1800)     // Maintenance first run after agent startup in seconds
#define METADATA_MAINTENANCE_REPEAT (60)            // Repeat if last run for dimensions, charts, labels needs more work
#define METADATA_HEALTH_LOG_INTERVAL (3600)         // Repeat maintenance for health
#define METADATA_DIM_CHECK_INTERVAL (3600)          // Repeat maintenance for dimensions
#define METADATA_CHART_CHECK_INTERVAL (3600)        // Repeat maintenance for charts
#define METADATA_LABEL_CHECK_INTERVAL (3600)        // Repeat maintenance for labels
#define METADATA_RUNTIME_THRESHOLD (5)              // Run time threshold for cleanup task

#define METADATA_HOST_CHECK_FIRST_CHECK (5)         // First check for pending metadata
#define METADATA_HOST_CHECK_INTERVAL (30)           // Repeat check for pending metadata
#define METADATA_HOST_CHECK_IMMEDIATE (5)           // Repeat immediate run because we have more metadata to write
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


// Metadata functions

struct query_build {
    BUFFER *sql;
    int count;
    char uuid_str[UUID_STR_LEN];
};

#define SQL_DELETE_CHART_LABELS_BY_HOST                                                                                \
    "DELETE FROM chart_label WHERE chart_id in (SELECT chart_id FROM chart WHERE host_id = @host_id)"

static void delete_host_chart_labels(uuid_t *host_uuid)
{
    sqlite3_stmt *res = NULL;

    int rc = sqlite3_prepare_v2(db_meta, SQL_DELETE_CHART_LABELS_BY_HOST, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to delete chart labels by host");
        return;
    }

    rc = sqlite3_bind_blob(res, 1, host_uuid, sizeof(*host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter to host chart labels");
        goto failed;
    }
    rc = sqlite3_step_monitored(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to execute command to remove host chart labels");

failed:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize statement to remove host chart labels");
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

// Migrate all hosts with hops zero to this host_uuid
void migrate_localhost(uuid_t *host_uuid)
{
    int rc;

    rc = exec_statement_with_uuid(MIGRATE_LOCALHOST_TO_NEW_MACHINE_GUID, host_uuid);
    if (!rc)
        rc = exec_statement_with_uuid(DELETE_NON_EXISTING_LOCALHOST, host_uuid);
    if (!rc) {
        if (unlikely(db_execute(db_meta, DELETE_MISSING_NODE_INSTANCES)))
            error_report("Failed to remove deleted hosts from node instances");
    }
}

static int store_claim_id(uuid_t *host_id, uuid_t *claim_id)
{
    sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return 1;
    }

    rc = sqlite3_prepare_v2(db_meta, SQL_STORE_CLAIM_ID, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to store host claim id");
        return 1;
    }

    rc = sqlite3_bind_blob(res, 1, host_id, sizeof(*host_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter to store claim id");
        goto failed;
    }

    if (claim_id)
        rc = sqlite3_bind_blob(res, 2, claim_id, sizeof(*claim_id), SQLITE_STATIC);
    else
        rc = sqlite3_bind_null(res, 2);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind claim_id parameter to host claim id");
        goto failed;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to store host claim id rc = %d", rc);

failed:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when storing a host claim id");

    return rc != SQLITE_DONE;
}

static void delete_dimension_uuid(uuid_t *dimension_uuid, sqlite3_stmt **action_res __maybe_unused, bool flag __maybe_unused)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!res)) {
        rc = prepare_statement(db_meta, DELETE_DIMENSION_UUID, &res);
        if (rc != SQLITE_OK) {
            error_report("Failed to prepare statement to delete a dimension uuid");
            return;
        }
    }

    rc = sqlite3_bind_blob(res, 1, dimension_uuid, sizeof(*dimension_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto skip_execution;

    rc = sqlite3_step_monitored(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to delete dimension uuid, rc = %d", rc);

skip_execution:
    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when deleting dimension UUID, rc = %d", rc);
}

//
// Store host and host system info information in the database
static int store_host_metadata(RRDHOST *host)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc, param = 0;

    if (unlikely((!res))) {
        rc = prepare_statement(db_meta, SQL_STORE_HOST_INFO, &res);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to store host, rc = %d", rc);
            return 1;
        }
    }

    rc = sqlite3_bind_blob(res, ++param, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = bind_text_null(res, ++param, rrdhost_hostname(host), 0);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = bind_text_null(res, ++param, rrdhost_registry_hostname(host), 1);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, ++param, host->rrd_update_every);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = bind_text_null(res, ++param, rrdhost_os(host), 1);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = bind_text_null(res, ++param, rrdhost_timezone(host), 1);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = bind_text_null(res, ++param, rrdhost_tags(host), 1);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, ++param, host->system_info ? host->system_info->hops : 0);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, ++param, host->rrd_memory_mode);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = bind_text_null(res, ++param, rrdhost_abbrev_timezone(host), 1);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, ++param, host->utc_offset);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = bind_text_null(res, ++param, rrdhost_program_name(host), 1);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = bind_text_null(res, ++param, rrdhost_program_version(host), 1);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int64(res, ++param, host->rrd_history_entries);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, ++param, (int ) host->health.health_enabled);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int64(res, ++param, (sqlite3_int64) host->last_connected);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    int store_rc = sqlite3_step_monitored(res);
    if (unlikely(store_rc != SQLITE_DONE))
        error_report("Failed to store host %s, rc = %d", rrdhost_hostname(host), rc);

    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement to store host %s, rc = %d", rrdhost_hostname(host), rc);

    return store_rc != SQLITE_DONE;
bind_fail:
    error_report("Failed to bind %d parameter to store host %s, rc = %d", param, rrdhost_hostname(host), rc);
    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement to store host %s, rc = %d", rrdhost_hostname(host), rc);
    return 1;
}

static int add_host_sysinfo_key_value(const char *name, const char *value, uuid_t *uuid)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc, param = 0;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE)
            return 0;
        error_report("Database has not been initialized");
        return 0;
    }

    if (unlikely((!res))) {
        rc = prepare_statement(db_meta, SQL_STORE_HOST_SYSTEM_INFO_VALUES, &res);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to store host info values, rc = %d", rc);
            return 0;
        }
    }

    rc = sqlite3_bind_blob(res, ++param, uuid, sizeof(*uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = bind_text_null(res, ++param, name, 0);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = bind_text_null(res, ++param, value ? value : "unknown", 0);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    int store_rc = sqlite3_step_monitored(res);
    if (unlikely(store_rc != SQLITE_DONE))
        error_report("Failed to store host info value %s, rc = %d", name, rc);

    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement to store host info value %s, rc = %d", name, rc);

    return store_rc == SQLITE_DONE;
bind_fail:
    error_report("Failed to bind %d parameter to store host info values %s, rc = %d", param, name, rc);
    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement to store host info values %s, rc = %d", name, rc);
    return 0;
}

static bool store_host_systeminfo(RRDHOST *host)
{
    struct rrdhost_system_info *system_info = host->system_info;

    if (unlikely(!system_info))
        return false;

    int ret = 0;

    ret += add_host_sysinfo_key_value("NETDATA_CONTAINER_OS_NAME", system_info->container_os_name, &host->host_uuid);
    ret += add_host_sysinfo_key_value("NETDATA_CONTAINER_OS_ID", system_info->container_os_id, &host->host_uuid);
    ret += add_host_sysinfo_key_value("NETDATA_CONTAINER_OS_ID_LIKE", system_info->container_os_id_like, &host->host_uuid);
    ret += add_host_sysinfo_key_value("NETDATA_CONTAINER_OS_VERSION", system_info->container_os_version, &host->host_uuid);
    ret += add_host_sysinfo_key_value("NETDATA_CONTAINER_OS_VERSION_ID", system_info->container_os_version_id, &host->host_uuid);
    ret += add_host_sysinfo_key_value("NETDATA_CONTAINER_OS_DETECTION", system_info->host_os_detection, &host->host_uuid);
    ret += add_host_sysinfo_key_value("NETDATA_HOST_OS_NAME", system_info->host_os_name, &host->host_uuid);
    ret += add_host_sysinfo_key_value("NETDATA_HOST_OS_ID", system_info->host_os_id, &host->host_uuid);
    ret += add_host_sysinfo_key_value("NETDATA_HOST_OS_ID_LIKE", system_info->host_os_id_like, &host->host_uuid);
    ret += add_host_sysinfo_key_value("NETDATA_HOST_OS_VERSION", system_info->host_os_version, &host->host_uuid);
    ret += add_host_sysinfo_key_value("NETDATA_HOST_OS_VERSION_ID", system_info->host_os_version_id, &host->host_uuid);
    ret += add_host_sysinfo_key_value("NETDATA_HOST_OS_DETECTION", system_info->host_os_detection, &host->host_uuid);
    ret += add_host_sysinfo_key_value("NETDATA_SYSTEM_KERNEL_NAME", system_info->kernel_name, &host->host_uuid);
    ret += add_host_sysinfo_key_value("NETDATA_SYSTEM_CPU_LOGICAL_CPU_COUNT", system_info->host_cores, &host->host_uuid);
    ret += add_host_sysinfo_key_value("NETDATA_SYSTEM_CPU_FREQ", system_info->host_cpu_freq, &host->host_uuid);
    ret += add_host_sysinfo_key_value("NETDATA_SYSTEM_TOTAL_RAM", system_info->host_ram_total, &host->host_uuid);
    ret += add_host_sysinfo_key_value("NETDATA_SYSTEM_TOTAL_DISK_SIZE", system_info->host_disk_space, &host->host_uuid);
    ret += add_host_sysinfo_key_value("NETDATA_SYSTEM_KERNEL_VERSION", system_info->kernel_version, &host->host_uuid);
    ret += add_host_sysinfo_key_value("NETDATA_SYSTEM_ARCHITECTURE", system_info->architecture, &host->host_uuid);
    ret += add_host_sysinfo_key_value("NETDATA_SYSTEM_VIRTUALIZATION", system_info->virtualization, &host->host_uuid);
    ret += add_host_sysinfo_key_value("NETDATA_SYSTEM_VIRT_DETECTION", system_info->virt_detection, &host->host_uuid);
    ret += add_host_sysinfo_key_value("NETDATA_SYSTEM_CONTAINER", system_info->container, &host->host_uuid);
    ret += add_host_sysinfo_key_value("NETDATA_SYSTEM_CONTAINER_DETECTION", system_info->container_detection, &host->host_uuid);
    ret += add_host_sysinfo_key_value("NETDATA_HOST_IS_K8S_NODE", system_info->is_k8s_node, &host->host_uuid);

    return !(24 == ret);
}


/*
 * Store a chart in the database
 */

static int store_chart_metadata(RRDSET *st)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc, param = 0, store_rc = 0;

    if (unlikely(!res)) {
        rc = prepare_statement(db_meta, SQL_STORE_CHART, &res);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to store chart, rc = %d", rc);
            return 1;
        }
    }

    rc = sqlite3_bind_blob(res, ++param, &st->chart_uuid, sizeof(st->chart_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_blob(res, ++param, &st->rrdhost->host_uuid, sizeof(st->rrdhost->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res, ++param, string2str(st->parts.type), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res, ++param, string2str(st->parts.id), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    const char *name = string2str(st->parts.name);
    if (name && *name)
        rc = sqlite3_bind_text(res, ++param, name, -1, SQLITE_STATIC);
    else
        rc = sqlite3_bind_null(res, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res, ++param, rrdset_family(st), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res, ++param, rrdset_context(st), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res, ++param, rrdset_title(st), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res, ++param, rrdset_units(st), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res, ++param, rrdset_plugin_name(st), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res, ++param, rrdset_module_name(st), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, ++param, (int) st->priority);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, ++param, st->update_every);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, ++param, st->chart_type);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, ++param, st->rrd_memory_mode);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, ++param, (int) st->db.entries);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    store_rc = execute_insert(res);
    if (unlikely(store_rc != SQLITE_DONE))
        error_report("Failed to store chart, rc = %d", store_rc);

    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement in chart store function, rc = %d", rc);

    return store_rc != SQLITE_DONE;

bind_fail:
    error_report("Failed to bind parameter %d to store chart, rc = %d", param, rc);
    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement in chart store function, rc = %d", rc);
    return 1;
}

/*
 * Store a dimension
 */
static int store_dimension_metadata(RRDDIM *rd)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc, param = 0;

    if (unlikely(!res)) {
        rc = prepare_statement(db_meta, SQL_STORE_DIMENSION, &res);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to store dimension, rc = %d", rc);
            return 1;
        }
    }

    rc = sqlite3_bind_blob(res, ++param, &rd->metric_uuid, sizeof(rd->metric_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_blob(res, ++param, &rd->rrdset->chart_uuid, sizeof(rd->rrdset->chart_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res, ++param, string2str(rd->id), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res, ++param, string2str(rd->name), -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, ++param, (int) rd->multiplier);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, ++param, (int ) rd->divisor);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, ++param, rd->algorithm);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    if (rrddim_option_check(rd, RRDDIM_OPTION_HIDDEN))
        rc = sqlite3_bind_text(res, ++param, "hidden", -1, SQLITE_STATIC);
    else
        rc = sqlite3_bind_null(res, ++param);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to store dimension, rc = %d", rc);

    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement in store dimension, rc = %d", rc);
    return 0;

bind_fail:
    error_report("Failed to bind parameter %d to store dimension, rc = %d", param, rc);
    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement in store dimension, rc = %d", rc);
    return 1;
}

static bool dimension_can_be_deleted(uuid_t *dim_uuid __maybe_unused, sqlite3_stmt **res __maybe_unused, bool flag __maybe_unused)
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

int get_pragma_value(sqlite3 *database, const char *sql)
{
    sqlite3_stmt *res = NULL;
    int rc = sqlite3_prepare_v2(database, sql, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK))
        return -1;

    int result = -1;
    rc = sqlite3_step_monitored(res);
    if (likely(rc == SQLITE_ROW))
        result = sqlite3_column_int(res, 0);

    rc = sqlite3_finalize(res);
    (void) rc;

    return result;
}


int get_free_page_count(sqlite3 *database)
{
    return get_pragma_value(database, "PRAGMA freelist_count");
}

int get_database_page_count(sqlite3 *database)
{
    return get_pragma_value(database, "PRAGMA page_count");
}

static bool run_cleanup_loop(
    sqlite3_stmt *res,
    struct metadata_wc *wc,
    bool (*check_cb)(uuid_t *, sqlite3_stmt **, bool),
    void (*action_cb)(uuid_t *, sqlite3_stmt **, bool),
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
        rc = check_cb((uuid_t *)sqlite3_column_blob(res, 0), check_stmt, check_flag);

        if (rc == true) {
            action_cb((uuid_t *)sqlite3_column_blob(res, 0), action_stmt, action_flag);
            (*total_deleted)++;
        }

        (*total_checked)++;
        time_expired = ((now_monotonic_sec() - start_running) > METADATA_RUNTIME_THRESHOLD);
    }
    return time_expired || (*total_checked == MAX_METADATA_CLEANUP) || (*total_deleted == MAX_METADATA_CLEANUP);
}


#define SQL_CHECK_CHART_EXISTENCE_IN_DIMENSION "SELECT count(1) FROM dimension WHERE chart_id = @chart_id"
#define SQL_CHECK_CHART_EXISTENCE_IN_CHART "SELECT count(1) FROM chart WHERE chart_id = @chart_id"

static bool chart_can_be_deleted(uuid_t *chart_uuid, sqlite3_stmt **check_res, bool check_in_dimension)
{
    int rc, result = 1;
    sqlite3_stmt *res = check_res ? *check_res : NULL;

    if (!res) {
        if (check_in_dimension)
            rc = sqlite3_prepare_v2(db_meta, SQL_CHECK_CHART_EXISTENCE_IN_DIMENSION, -1, &res, 0);
        else
            rc = sqlite3_prepare_v2(db_meta, SQL_CHECK_CHART_EXISTENCE_IN_CHART, -1, &res, 0);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to check for chart existence, rc = %d", rc);
            return 0;
        }
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
        rc = sqlite3_reset(res);
    else
        rc = sqlite3_finalize(res);

    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to %s statement that checks chart uuid existence rc = %d", check_res ? "reset" : "finalize", rc);
    return result == 0;
}

#define SQL_DELETE_CHART_BY_UUID        "DELETE FROM chart WHERE chart_id = @chart_id"
#define SQL_DELETE_CHART_LABEL_BY_UUID  "DELETE FROM chart_label WHERE chart_id = @chart_id"

static void delete_chart_uuid(uuid_t *chart_uuid, sqlite3_stmt **action_res, bool label_only)
{
    int rc;
    sqlite3_stmt *res = action_res ? *action_res : NULL;

    if (!res) {
        if (label_only)
            rc = sqlite3_prepare_v2(db_meta, SQL_DELETE_CHART_LABEL_BY_UUID, -1, &res, 0);
        else
            rc = sqlite3_prepare_v2(db_meta, SQL_DELETE_CHART_BY_UUID, -1, &res, 0);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to check for chart existence, rc = %d", rc);
            return;
        }
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
        rc = sqlite3_reset(res);
    else
        rc = sqlite3_finalize(res);

    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to %s statement that deletes a chart uuid rc = %d", action_res ? "reset" : "finalize", rc);
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

    int rc;
    sqlite3_stmt *res = NULL;

    rc = sqlite3_prepare_v2(db_meta, SELECT_DIMENSION_LIST, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to fetch host dimensions");
        return;
    }

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

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize the prepared statement to check dimensions");
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

    int rc = sqlite3_prepare_v2(db_meta, SELECT_CHART_LIST, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to fetch charts");
        return;
    }

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

    if (check_res)
        sqlite3_finalize(check_res);

    if (action_res)
        sqlite3_finalize(action_res);

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

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when reading charts");
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

    int rc;
    sqlite3_stmt *res = NULL;

    rc = sqlite3_prepare_v2(db_meta, SELECT_CHART_LABEL_LIST, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to fetch charts");
        return;
    }

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

    if (check_res)
        sqlite3_finalize(check_res);

    if (action_res)
        sqlite3_finalize(action_res);

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

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when checking charts");
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

    bool is_claimed = claimed();
    dfe_start_reentrant(rrdhost_root_index, host){
        if (rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED))
            continue;
        sql_health_alarm_log_cleanup(host, is_claimed);
        if (unlikely(metadata_flag_check(wc, METADATA_FLAG_SHUTDOWN)))
            break;
    }
    dfe_done(host);

    if (unlikely(metadata_flag_check(wc, METADATA_FLAG_SHUTDOWN)))
        return;

    (void) db_execute(db_meta,"DELETE FROM health_log WHERE host_id NOT IN (SELECT host_id FROM host)");
    (void) db_execute(db_meta,"DELETE FROM health_log_detail WHERE health_log_id NOT IN (SELECT health_log_id FROM health_log)");
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
    void *data;
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

#ifdef ENABLE_ACLK
    aclk_queue_node_info(host, false);
#endif

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

    if (ml_load_stmt) {
        sqlite3_finalize(ml_load_stmt);
        ml_load_stmt = NULL;
    }

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

struct host_chart_label_cleanup {
    Pvoid_t JudyL;
    Word_t count;
};

static void do_chart_label_cleanup(struct host_chart_label_cleanup *cl_cleanup_data)
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
            uuid_t host_uuid;
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

    do_chart_label_cleanup((struct host_chart_label_cleanup *) data->data);

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

            int rc = exec_statement_with_uuid(SQL_DELETE_HOST_LABELS, &host->host_uuid);
            if (likely(!rc)) {
                query_counter++;

                buffer_flush(work_buffer);
                struct query_build tmp = {.sql = work_buffer, .count = 0};
                uuid_unparse_lower(host->host_uuid, tmp.uuid_str);
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
            uuid_t uuid;
            int rc;
            if (likely(host->aclk_state.claimed_id && !uuid_parse(host->aclk_state.claimed_id, uuid)))
                rc = store_claim_id(&host->host_uuid, &uuid);
            else
                rc = store_claim_id(&host->host_uuid, NULL);

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

    if (unlikely(run_again))
        wc->metadata_check_after = now_realtime_sec() + METADATA_HOST_CHECK_IMMEDIATE;
    else {
        wc->metadata_check_after = now_realtime_sec() + METADATA_HOST_CHECK_INTERVAL;
        run_metadata_cleanup(wc);
    }
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

    uv_thread_set_name_np(wc->thread, "METASYNC");
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
    struct host_chart_label_cleanup *cl_cleanup_data = NULL;

    while (shutdown == 0 || (wc->flags & METADATA_FLAG_PROCESSING)) {
        uuid_t  *uuid;
        RRDHOST *host = NULL;

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
                    uuid = (uuid_t *) cmd.param[0];
                    if (likely(dimension_can_be_deleted(uuid, NULL, false)))
                        delete_dimension_uuid(uuid, NULL, false);
                    freez(uuid);
                    break;
                case METADATA_STORE_CLAIM_ID:
                    store_claim_id((uuid_t *) cmd.param[0], (uuid_t *) cmd.param[1]);
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
                    data->data = cl_cleanup_data;
                    data->work_buffer = work_buffer;
                    cl_cleanup_data = NULL;

                    if (unlikely(cmd.completion)) {
                        data->max_count = 0;            // 0 will process all pending updates
                        cmd.completion = NULL;          // Do not complete after launching worker (worker will do)
                    }
                    else
                        data->max_count = 5000;

                    metadata_flag_set(wc, METADATA_FLAG_PROCESSING);
                    if (unlikely(
                            uv_queue_work(loop,&data->request,
                                          start_metadata_hosts,
                                          after_metadata_hosts))) {
                        // Failed to launch worker -- let the event loop handle completion
                        cmd.completion = wc->scan_complete;
                        cl_cleanup_data = data->data;
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
                    if (unlikely(
                            uv_queue_work(loop,&data->request, start_all_host_load_context,
                                          after_start_host_load_context))) {
                        freez(data);
                    }
                    break;
                case METADATA_DELETE_HOST_CHART_LABELS:;
                    if (!cl_cleanup_data)
                        cl_cleanup_data = callocz(1,sizeof(*cl_cleanup_data));

                    Pvoid_t *PValue = JudyLIns(&cl_cleanup_data->JudyL, (Word_t) ++cl_cleanup_data->count, PJE0);
                    if (PValue)
                        *PValue = (void *) cmd.param[0];

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
    if (unlikely(!metasync_worker.loop))
        return;

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
void metaqueue_delete_dimension_uuid(uuid_t *uuid)
{
    if (unlikely(!metasync_worker.loop))
        return;
    uuid_t *use_uuid = mallocz(sizeof(*uuid));
    uuid_copy(*use_uuid, *uuid);
    queue_metadata_cmd(METADATA_DEL_DIMENSION, use_uuid, NULL);
}

void metaqueue_store_claim_id(uuid_t *host_uuid, uuid_t *claim_uuid)
{
    if (unlikely(!host_uuid))
        return;

    uuid_t *local_host_uuid = mallocz(sizeof(*host_uuid));
    uuid_t *local_claim_uuid = NULL;

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

    netdata_thread_t threads[threads_to_create];
    tu.join = 0;
    for (int i = 0; i < threads_to_create; i++) {
        char buf[100 + 1];
        snprintf(buf, sizeof(buf) - 1, "META[%d]", i);
        netdata_thread_create(
            &threads[i],
            buf,
            NETDATA_THREAD_OPTION_DONT_LOG | NETDATA_THREAD_OPTION_JOINABLE,
            unittest_queue_metadata,
            &tu);
    }
    (void) uv_async_send(&metasync_worker.async);
    sleep_usec(seconds_to_run * USEC_PER_SEC);

    __atomic_store_n(&tu.join, 1, __ATOMIC_RELAXED);
    for (int i = 0; i < threads_to_create; i++) {
        void *retval;
        netdata_thread_join(threads[i], &retval);
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

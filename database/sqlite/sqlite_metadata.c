// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_metadata.h"

extern DICTIONARY *rrdhost_root_index;

// Metadata functions
#define SQL_DELETE_HOST_LABELS  "DELETE FROM host_label WHERE host_id = @uuid;"

struct label_str {
    BUFFER *sql;
    int count;
    char uuid_str[UUID_STR_LEN];
};

static int host_label_store_to_sql_callback(const char *name, const char *value, RRDLABEL_SRC ls, void *data) {
    struct label_str *lb = data;
    if (unlikely(!lb->count))
        buffer_sprintf(lb->sql, "INSERT OR REPLACE INTO host_label (host_id, source_type, label_key, label_value, date_created) VALUES ");
    else
        buffer_strcat(lb->sql, ", ");
    buffer_sprintf(lb->sql, "(u2h('%s'), %d,'%s','%s', unixepoch())", lb->uuid_str, (int)ls & ~(RRDLABEL_FLAG_INTERNAL), name, value);
    lb->count++;
    return 1;
}

static int chart_label_store_to_sql_callback(const char *name, const char *value, RRDLABEL_SRC ls, void *data) {
    struct label_str *lb = data;
    if (unlikely(!lb->count))
        buffer_sprintf(lb->sql, "INSERT OR REPLACE INTO chart_label (chart_id, source_type, label_key, label_value, date_created) VALUES ");
    else
        buffer_strcat(lb->sql, ", ");
    buffer_sprintf(lb->sql, "(u2h('%s'), %d,'%s','%s', unixepoch())", lb->uuid_str, ls, name, value);
    lb->count++;
    return 1;
}

// Migrate all hosts with hops zero to this host_uuid
void migrate_localhost(uuid_t *host_uuid)
{
    int rc;

    rc = exec_statement_with_uuid("UPDATE chart SET host_id = @host_id WHERE host_id in (SELECT host_id FROM host where host_id <> @host_id and hops = 0); ", host_uuid);
    if (!rc)
        rc = exec_statement_with_uuid("DELETE FROM host WHERE hops = 0 AND host_id <> @host_id; ", host_uuid);
    if (!rc)
        db_execute("DELETE FROM node_instance WHERE host_id NOT IN (SELECT host_id FROM host);");

}

#define SQL_STORE_CLAIM_ID  "insert into node_instance " \
    "(host_id, claim_id, date_created) values (@host_id, @claim_id, unixepoch()) " \
    "on conflict(host_id) do update set claim_id = excluded.claim_id;"

static void store_claim_id(uuid_t *host_id, uuid_t *claim_id)
{
    sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return;
    }

    rc = sqlite3_prepare_v2(db_meta, SQL_STORE_CLAIM_ID, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement store chart labels");
        return;
    }

    rc = sqlite3_bind_blob(res, 1, host_id, sizeof(*host_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host_id parameter to store node instance information");
        goto failed;
    }

    if (claim_id)
        rc = sqlite3_bind_blob(res, 2, claim_id, sizeof(*claim_id), SQLITE_STATIC);
    else
        rc = sqlite3_bind_null(res, 2);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind claim_id parameter to store node instance information");
        goto failed;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to store node instance information, rc = %d", rc);

failed:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when storing node instance information");
}

#define DELETE_DIMENSION_UUID   "DELETE FROM dimension WHERE dim_id = @uuid;"
static void delete_dimension_uuid(uuid_t *dimension_uuid)
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

    rc = sqlite3_bind_blob(res, 1, dimension_uuid,  sizeof(*dimension_uuid), SQLITE_STATIC);
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
#define SQL_STORE_HOST_INFO "INSERT OR REPLACE INTO host " \
        "(host_id, hostname, registry_hostname, update_every, os, timezone," \
        "tags, hops, memory_mode, abbrev_timezone, utc_offset, program_name, program_version," \
        "entries, health_enabled) " \
        "values (@host_id, @hostname, @registry_hostname, @update_every, @os, @timezone, @tags, @hops, @memory_mode, " \
        "@abbrev_timezone, @utc_offset, @program_name, @program_version, " \
        "@entries, @health_enabled);"

static int sql_store_host_info(RRDHOST *host)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE)
            return 0;
        error_report("Database has not been initialized");
        return 1;
    }

    if (unlikely((!res))) {
        rc = prepare_statement(db_meta, SQL_STORE_HOST_INFO, &res);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to store host, rc = %d", rc);
            return 1;
        }
    }

    rc = sqlite3_bind_blob(res, 1, &host->host_uuid, sizeof(host->host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = bind_text_null(res, 2, rrdhost_hostname(host), 0);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = bind_text_null(res, 3, rrdhost_registry_hostname(host), 1);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, 4, host->rrd_update_every);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = bind_text_null(res, 5, rrdhost_os(host), 1);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = bind_text_null(res, 6, rrdhost_timezone(host), 1);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = bind_text_null(res, 7, rrdhost_tags(host), 1);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, 8, host->system_info ? host->system_info->hops : 0);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, 9, host->rrd_memory_mode);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = bind_text_null(res, 10, rrdhost_abbrev_timezone(host), 1);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, 11, host->utc_offset);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = bind_text_null(res, 12, rrdhost_program_name(host), 1);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = bind_text_null(res, 13, rrdhost_program_version(host), 1);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int64(res, 14, host->rrd_history_entries);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, 15, (int ) host->health_enabled);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    int store_rc = sqlite3_step_monitored(res);
    if (unlikely(store_rc != SQLITE_DONE))
        error_report("Failed to store host %s, rc = %d", rrdhost_hostname(host), rc);

    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement to store host %s, rc = %d", rrdhost_hostname(host), rc);

    return !(store_rc == SQLITE_DONE);
bind_fail:
    error_report("Failed to bind parameter to store host %s, rc = %d", rrdhost_hostname(host), rc);
    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement to store host %s, rc = %d", rrdhost_hostname(host), rc);
    return 1;
}

#define SQL_INS_HOST_SYSTEM_INFO "INSERT OR REPLACE INTO host_info " \
    "(host_id, system_key, system_value, date_created) " \
    "VALUES (@host, @key, @value, unixepoch());"

static void sql_store_host_system_info_key_value(uuid_t *host_id, const char *name, const char *value)
{
    sqlite3_stmt *res = NULL;
    int rc;

    rc = sqlite3_prepare_v2(db_meta, SQL_INS_HOST_SYSTEM_INFO, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to store system info");
        return;
    }

    rc = sqlite3_bind_blob(res, 1, host_id, sizeof(*host_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host parameter to store system information");
        goto skip_store;
    }

    rc = sqlite3_bind_text(res, 2, name, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind label parameter to store name information");
        goto skip_store;
    }

    rc = sqlite3_bind_text(res, 3, value, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind value parameter to store value information");
        goto skip_store;
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to store host system info, rc = %d", rc);

skip_store:
    if (unlikely(sqlite3_finalize(res) != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when storing  host system information");
}


static void sql_store_host_system_info(uuid_t *host_id, const struct rrdhost_system_info *system_info)
{
    if (unlikely(!system_info))
        return;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            error_report("Database has not been initialized");
        return;
    }

    if (system_info->container_os_name)
        sql_store_host_system_info_key_value(host_id, "NETDATA_CONTAINER_OS_NAME", system_info->container_os_name);

    if (system_info->container_os_id)
        sql_store_host_system_info_key_value(host_id, "NETDATA_CONTAINER_OS_ID", system_info->container_os_id);

    if (system_info->container_os_id_like)
        sql_store_host_system_info_key_value(host_id, "NETDATA_CONTAINER_OS_ID_LIKE", system_info->container_os_id_like);

    if (system_info->container_os_version)
        sql_store_host_system_info_key_value(host_id, "NETDATA_CONTAINER_OS_VERSION", system_info->container_os_version);

    if (system_info->container_os_version_id)
        sql_store_host_system_info_key_value(host_id, "NETDATA_CONTAINER_OS_VERSION_ID", system_info->container_os_version_id);

    if (system_info->host_os_detection)
        sql_store_host_system_info_key_value(host_id, "NETDATA_CONTAINER_OS_DETECTION", system_info->host_os_detection);

    if (system_info->host_os_name)
        sql_store_host_system_info_key_value(host_id, "NETDATA_HOST_OS_NAME", system_info->host_os_name);

    if (system_info->host_os_id)
        sql_store_host_system_info_key_value(host_id, "NETDATA_HOST_OS_ID", system_info->host_os_id);

    if (system_info->host_os_id_like)
        sql_store_host_system_info_key_value(host_id, "NETDATA_HOST_OS_ID_LIKE", system_info->host_os_id_like);

    if (system_info->host_os_version)
        sql_store_host_system_info_key_value(host_id, "NETDATA_HOST_OS_VERSION", system_info->host_os_version);

    if (system_info->host_os_version_id)
        sql_store_host_system_info_key_value(host_id, "NETDATA_HOST_OS_VERSION_ID", system_info->host_os_version_id);

    if (system_info->host_os_detection)
        sql_store_host_system_info_key_value(host_id, "NETDATA_HOST_OS_DETECTION", system_info->host_os_detection);

    if (system_info->kernel_name)
        sql_store_host_system_info_key_value(host_id, "NETDATA_SYSTEM_KERNEL_NAME", system_info->kernel_name);

    if (system_info->host_cores)
        sql_store_host_system_info_key_value(host_id, "NETDATA_SYSTEM_CPU_LOGICAL_CPU_COUNT", system_info->host_cores);

    if (system_info->host_cpu_freq)
        sql_store_host_system_info_key_value(host_id, "NETDATA_SYSTEM_CPU_FREQ", system_info->host_cpu_freq);

    if (system_info->host_ram_total)
        sql_store_host_system_info_key_value(host_id, "NETDATA_SYSTEM_TOTAL_RAM", system_info->host_ram_total);

    if (system_info->host_disk_space)
        sql_store_host_system_info_key_value(host_id, "NETDATA_SYSTEM_TOTAL_DISK_SIZE", system_info->host_disk_space);

    if (system_info->kernel_version)
        sql_store_host_system_info_key_value(host_id, "NETDATA_SYSTEM_KERNEL_VERSION", system_info->kernel_version);

    if (system_info->architecture)
        sql_store_host_system_info_key_value(host_id, "NETDATA_SYSTEM_ARCHITECTURE", system_info->architecture);

    if (system_info->virtualization)
        sql_store_host_system_info_key_value(host_id, "NETDATA_SYSTEM_VIRTUALIZATION", system_info->virtualization);

    if (system_info->virt_detection)
        sql_store_host_system_info_key_value(host_id, "NETDATA_SYSTEM_VIRT_DETECTION", system_info->virt_detection);

    if (system_info->container)
        sql_store_host_system_info_key_value(host_id, "NETDATA_SYSTEM_CONTAINER", system_info->container);

    if (system_info->container_detection)
        sql_store_host_system_info_key_value(host_id, "NETDATA_SYSTEM_CONTAINER_DETECTION", system_info->container_detection);

    if (system_info->is_k8s_node)
        sql_store_host_system_info_key_value(host_id, "NETDATA_HOST_IS_K8S_NODE", system_info->is_k8s_node);
}


/*
 * Store set option for a dimension
 */
static int sql_set_dimension_option(uuid_t *dim_uuid, char *option)
{
    sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE)
            return 0;
        error_report("Database has not been initialized");
        return 1;
    }

    rc = sqlite3_prepare_v2(db_meta, "UPDATE dimension SET options = @options WHERE dim_id = @dim_id", -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to update dimension options");
        return 0;
    };

    rc = sqlite3_bind_blob(res, 2, dim_uuid, sizeof(*dim_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    if (!option || !strcmp(option,"unhide"))
        rc = sqlite3_bind_null(res, 1);
    else
        rc = sqlite3_bind_text(res, 1, option, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to update dimension option, rc = %d", rc);

bind_fail:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement in update dimension options, rc = %d", rc);
    return 0;
}

/*
 * Store a chart in the database
 */

#define SQL_STORE_CHART "insert or replace into chart (chart_id, host_id, type, id, " \
    "name, family, context, title, unit, plugin, module, priority, update_every , chart_type , memory_mode , " \
    "history_entries) values (?1,?2,?3,?4,?5,?6,?7,?8,?9,?10,?11,?12,?13,?14,?15,?16);"

static int sql_store_chart(
    uuid_t *chart_uuid, uuid_t *host_uuid, const char *type, const char *id, const char *name, const char *family,
    const char *context, const char *title, const char *units, const char *plugin, const char *module, long priority,
    int update_every, int chart_type, int memory_mode, long history_entries)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc, param = 0;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE)
            return 0;
        error_report("Database has not been initialized");
        return 1;
    }

    if (unlikely(!res)) {
        rc = prepare_statement(db_meta, SQL_STORE_CHART, &res);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to store chart, rc = %d", rc);
            return 1;
        }
    }

    param++;
    rc = sqlite3_bind_blob(res, 1, chart_uuid, sizeof(*chart_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_blob(res, 2, host_uuid, sizeof(*host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 3, type, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 4, id, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    if (name && *name)
        rc = sqlite3_bind_text(res, 5, name, -1, SQLITE_STATIC);
    else
        rc = sqlite3_bind_null(res, 5);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 6, family, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 7, context, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 8, title, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 9, units, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 10, plugin, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_text(res, 11, module, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_int(res, 12, (int) priority);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_int(res, 13, update_every);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_int(res, 14, chart_type);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_int(res, 15, memory_mode);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    param++;
    rc = sqlite3_bind_int(res, 16, (int) history_entries);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to store chart, rc = %d", rc);

    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement in chart store function, rc = %d", rc);

    return 0;

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

#define SQL_STORE_DIMENSION "INSERT OR REPLACE INTO dimension (dim_id, chart_id, id, name, multiplier, divisor , algorithm) " \
        "VALUES (@dim_id, @chart_id, @id, @name, @multiplier, @divisor, @algorithm);"
static int sql_store_dimension(
    uuid_t *dim_uuid, uuid_t *chart_uuid, const char *id, const char *name, collected_number multiplier,
    collected_number divisor, int algorithm)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc;

    if (unlikely(!db_meta)) {
        if (default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE)
            return 0;
        error_report("Database has not been initialized");
        return 1;
    }

    if (unlikely(!res)) {
        rc = prepare_statement(db_meta, SQL_STORE_DIMENSION, &res);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to store dimension, rc = %d", rc);
            return 1;
        }
    }

    rc = sqlite3_bind_blob(res, 1, dim_uuid, sizeof(*dim_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_blob(res, 2, chart_uuid, sizeof(*chart_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res, 3, id, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_text(res, 4, name, -1, SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, 5, (int) multiplier);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, 6, (int ) divisor);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, 7, algorithm);
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
    error_report("Failed to bind parameter to store dimension, rc = %d", rc);
    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement in store dimension, rc = %d", rc);
    return 1;
}

static bool dimension_can_be_deleted(uuid_t *dim_uuid)
{
#ifdef ENABLE_DBENGINE
    bool no_retention = true;
    for (int tier = 0; tier < storage_tiers; tier++) {
        if (!multidb_ctx[tier])
            continue;
        time_t first_time_t = 0, last_time_t = 0;
        if (rrdeng_metric_retention_by_uuid((void *) multidb_ctx[tier], dim_uuid, &first_time_t, &last_time_t) == 0) {
            if (first_time_t > 0) {
                no_retention = false;
                break;
            }
        }
    }
    return no_retention;
#else
    return false;
#endif
}

#define SELECT_DIMENSION_LIST "SELECT dim_id, rowid FROM dimension WHERE rowid > @row_id"

static void check_dimension_metadata(struct metadata_wc *wc)
{
    int rc;
    sqlite3_stmt *res = NULL;

    rc = sqlite3_prepare_v2(db_meta, SELECT_DIMENSION_LIST, -1, &res, 0);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to fetch host dimensions");
        return;
    }

    rc = sqlite3_bind_int64(res, 1,  (sqlite3_int64) wc->row_id);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to row parameter");
        goto skip_run;
    }

    uint32_t total_checked = 0;
    uint32_t total_deleted= 0;
    uint64_t last_row_id = wc->row_id;

    info("METADATA: Checking dimensions starting after row %lu", wc->row_id);

    while (sqlite3_step_monitored(res) == SQLITE_ROW && total_deleted < MAX_METADATA_CLEANUP) {
        if (unlikely(metadata_flag_check(wc, METADATA_FLAG_SHUTDOWN)))
            break;

        last_row_id = sqlite3_column_int64(res, 1);
        rc = dimension_can_be_deleted((uuid_t *)sqlite3_column_blob(res, 0));
        if (rc == true) {
            delete_dimension_uuid((uuid_t *)sqlite3_column_blob(res, 0));
            total_deleted++;
        }
        total_checked++;
    }
    wc->row_id = last_row_id;
    int64_t now = now_realtime_sec();
    if (total_deleted > 0) {
        wc->check_metadata_after = now + METADATA_MAINTENANCE_RETRY;
    } else
        wc->row_id = 0;
    info("METADATA: Checked %u, deleted %u -- will resume after row %lu in %ld seconds", total_checked, total_deleted, wc->row_id,
         wc->check_metadata_after - now);

skip_run:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when reading dimensions");
}


//
// EVENT LOOP STARTS HERE
//
static uv_mutex_t metadata_async_lock;

static void metadata_database_init_cmd_queue(struct metadata_wc *wc)
{
    wc->cmd_queue.head = wc->cmd_queue.tail = 0;
    wc->queue_size = 0;
    fatal_assert(0 == uv_cond_init(&wc->cmd_cond));
    fatal_assert(0 == uv_mutex_init(&wc->cmd_mutex));
}

int metadata_database_enq_cmd_noblock(struct metadata_wc *wc, struct metadata_cmd *cmd)
{
    unsigned queue_size;

    /* wait for free space in queue */
    uv_mutex_lock(&wc->cmd_mutex);

    if (cmd->opcode == METADATA_SYNC_SHUTDOWN) {
        metadata_flag_set(wc, METADATA_FLAG_SHUTDOWN);
        uv_mutex_unlock(&wc->cmd_mutex);
        return 0;
    }

    if (unlikely((queue_size = wc->queue_size) == METADATA_CMD_Q_MAX_SIZE ||
        metadata_flag_check(wc, METADATA_FLAG_SHUTDOWN))) {
        uv_mutex_unlock(&wc->cmd_mutex);
        return 1;
    }

    fatal_assert(queue_size < METADATA_CMD_Q_MAX_SIZE);
    /* enqueue command */
    wc->cmd_queue.cmd_array[wc->cmd_queue.tail] = *cmd;
    wc->cmd_queue.tail = wc->cmd_queue.tail != METADATA_CMD_Q_MAX_SIZE - 1 ?
                             wc->cmd_queue.tail + 1 : 0;
    wc->queue_size = queue_size + 1;
    uv_mutex_unlock(&wc->cmd_mutex);
    return 0;
}

static void metadata_enq_cmd(struct metadata_wc *wc, struct metadata_cmd *cmd)
{
    unsigned queue_size;

    /* wait for free space in queue */
    uv_mutex_lock(&wc->cmd_mutex);
    if (unlikely(metadata_flag_check(wc, METADATA_FLAG_SHUTDOWN))) {
        uv_mutex_unlock(&wc->cmd_mutex);
        (void) uv_async_send(&wc->async);
        return;
    }

    if (cmd->opcode == METADATA_SYNC_SHUTDOWN) {
        metadata_flag_set(wc, METADATA_FLAG_SHUTDOWN);
        uv_mutex_unlock(&wc->cmd_mutex);
        (void) uv_async_send(&wc->async);
        return;
    }

    while ((queue_size = wc->queue_size) == METADATA_CMD_Q_MAX_SIZE) {
        if (unlikely(metadata_flag_check(wc, METADATA_FLAG_SHUTDOWN))) {
            uv_mutex_unlock(&wc->cmd_mutex);
            return;
        }
        uv_cond_wait(&wc->cmd_cond, &wc->cmd_mutex);
    }
    fatal_assert(queue_size < METADATA_CMD_Q_MAX_SIZE);
    /* enqueue command */
    wc->cmd_queue.cmd_array[wc->cmd_queue.tail] = *cmd;
    wc->cmd_queue.tail = wc->cmd_queue.tail != METADATA_CMD_Q_MAX_SIZE - 1 ?
                             wc->cmd_queue.tail + 1 : 0;
    wc->queue_size = queue_size + 1;
    uv_mutex_unlock(&wc->cmd_mutex);

    /* wake up event loop */
    (void) uv_async_send(&wc->async);
}

static struct metadata_cmd metadata_deq_cmd(struct metadata_wc *wc, enum metadata_opcode *next_opcode)
{
    struct metadata_cmd ret;
    unsigned queue_size;

    uv_mutex_lock(&wc->cmd_mutex);
    queue_size = wc->queue_size;
    if (queue_size == 0) {
        memset(&ret, 0, sizeof(ret));
        ret.opcode = METADATA_DATABASE_NOOP;
        ret.completion = NULL;
        *next_opcode = METADATA_DATABASE_NOOP;
    } else {
        /* dequeue command */
        ret = wc->cmd_queue.cmd_array[wc->cmd_queue.head];

        if (queue_size == 1) {
            wc->cmd_queue.head = wc->cmd_queue.tail = 0;
        } else {
            wc->cmd_queue.head = wc->cmd_queue.head != METADATA_CMD_Q_MAX_SIZE - 1 ?
                                     wc->cmd_queue.head + 1 : 0;
        }
        wc->queue_size = queue_size - 1;
        if (wc->queue_size > 0)
            *next_opcode = wc->cmd_queue.cmd_array[wc->cmd_queue.head].opcode;
        else
            *next_opcode = METADATA_DATABASE_NOOP;
        /* wake up producers */
        uv_cond_signal(&wc->cmd_cond);
    }
    uv_mutex_unlock(&wc->cmd_mutex);

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

   time_t now = now_realtime_sec();

   if (wc->check_metadata_after && wc->check_metadata_after < now) {
       cmd.opcode = METADATA_MAINTENANCE;
       if (!metadata_database_enq_cmd_noblock(wc, &cmd))
           wc->check_metadata_after = now + METADATA_MAINTENANCE_INTERVAL;
   }

   if (wc->check_hosts_after && wc->check_hosts_after < now) {
       cmd.opcode = METADATA_SCAN_HOSTS;
       if (!metadata_database_enq_cmd_noblock(wc, &cmd))
           wc->check_hosts_after = now + METADATA_HOST_CHECK_INTERVAL;
   }
}

static void after_metadata_cleanup(uv_work_t *req, int status)
{
    UNUSED(status);

    struct metadata_wc *wc = req->data;
    metadata_flag_clear(wc, METADATA_FLAG_CLEANUP);
}
static void start_metadata_cleanup(uv_work_t *req)
{
    struct metadata_wc *wc = req->data;
    check_dimension_metadata(wc);
}

struct scan_metadata_payload {
    struct metadata_wc *wc;
    uint32_t max_count;
};

// Callback after scan of hosts is done
static void after_metadata_hosts(uv_work_t *req, int status __maybe_unused)
{
    struct scan_metadata_payload *data = req->data;
    struct metadata_wc *wc = data->wc;

    metadata_flag_clear(wc, METADATA_FLAG_SCANNING_HOSTS);
    internal_error(true, "METADATA: scanning hosts complete");
    freez(data);
}

static bool metadata_scan_host(RRDHOST *host, uint32_t max_count) {
    RRDSET *st;
    int rc;

    bool more_to_do = false;
    uint32_t scan_count = 0;

    rrdset_foreach_reentrant(st, host) {
        if (scan_count == max_count) {
            more_to_do = true;
            break;
        }
        if(rrdset_flag_check(st, RRDSET_FLAG_METADATA_UPDATE)) {
            rrdset_flag_clear(st, RRDSET_FLAG_METADATA_UPDATE);
            scan_count++;
            rc = sql_store_chart(
                &st->chart_uuid,
                &st->rrdhost->host_uuid,
                string2str(st->parts.type),
                string2str(st->parts.id),
                string2str(st->parts.name),
                rrdset_family(st),
                rrdset_context(st),
                rrdset_title(st),
                rrdset_units(st),
                rrdset_plugin_name(st),
                rrdset_module_name(st),
                st->priority,
                st->update_every,
                st->chart_type,
                st->rrd_memory_mode,
                st->entries);
            if (unlikely(rc))
                internal_error(true, "METADATA: Failed to store chart metadata %s", string2str(st->id));
        }

        RRDDIM *rd;
        rrddim_foreach_read(rd, st) {
            if(rrddim_flag_check(rd, RRDDIM_FLAG_METADATA_UPDATE)) {
                rrddim_flag_clear(rd, RRDDIM_FLAG_METADATA_UPDATE);

                rc = sql_store_dimension(
                    &rd->metric_uuid,
                    &rd->rrdset->chart_uuid,
                    string2str(rd->id),
                    string2str(rd->name),
                    rd->multiplier,
                    rd->divisor,
                    rd->algorithm);

                if (unlikely(rc))
                    error_report("METADATA: Failed to store dimension %s", string2str(rd->id));
            }
        }
        rrddim_foreach_done(rd);
    }
    rrdset_foreach_done(st);
    return more_to_do;
}

// Worker thread to scan hosts for pending metadata to store
static void start_metadata_hosts(uv_work_t *req __maybe_unused)
{
    RRDHOST *host;

    struct scan_metadata_payload *data = req->data;
    struct metadata_wc *wc = data->wc;

    bool run_again = false;
    dfe_start_reentrant(rrdhost_root_index, host) {
        if (rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED) && !rrdhost_flag_check(host, RRDHOST_FLAG_METADATA_UPDATE))
            continue;
        internal_error(true, "METADATA: %d Scanning host %s", gettid(), rrdhost_hostname(host));
        rrdhost_flag_clear(host,RRDHOST_FLAG_METADATA_UPDATE);
        if (unlikely(metadata_scan_host(host, data->max_count))) {
            run_again = true;
            rrdhost_flag_set(host,RRDHOST_FLAG_METADATA_UPDATE);
            info("METADATA: Rescheduling host %s to run; more charts to store", rrdhost_hostname(host));
        }
    }
    dfe_done(host);
    if (unlikely(run_again))
        wc->check_hosts_after = now_realtime_sec() + METADATA_HOST_CHECK_INTERVAL;
    else
        wc->check_hosts_after = now_realtime_sec() + METADATA_HOST_CHECK_IMMEDIATE;
}

static void metadata_event_loop(void *arg)
{
    worker_register("METASYNC");
    worker_register_job_name(METADATA_DATABASE_NOOP,        "noop");
    worker_register_job_name(METADATA_DATABASE_TIMER,       "timer");
    worker_register_job_name(METADATA_ADD_CHART,            "add chart");
    worker_register_job_name(METADATA_ADD_CHART_LABEL,      "add chart label");
    worker_register_job_name(METADATA_ADD_DIMENSION,        "add dimension");
    worker_register_job_name(METADATA_DEL_DIMENSION,        "delete dimension");
    worker_register_job_name(METADATA_ADD_DIMENSION_OPTION, "dimension option");
    worker_register_job_name(METADATA_ADD_HOST_SYSTEM_INFO, "host system info");
    worker_register_job_name(METADATA_ADD_HOST_INFO,        "host info");
    worker_register_job_name(METADATA_STORE_CLAIM_ID,       "add claim id");
    worker_register_job_name(METADATA_STORE_HOST_LABELS,    "host labels");
    worker_register_job_name(METADATA_MAINTENANCE,          "maintenance");


    int ret;
    uv_loop_t *loop;
    unsigned cmd_batch_size;
    struct metadata_wc *wc = arg;
    enum metadata_opcode opcode, next_opcode;
    uv_work_t metadata_cleanup_worker;
    uv_work_t metadata_scan_hosts_worker;

    uv_thread_set_name_np(wc->thread, "METASYNC");
    loop = wc->loop = mallocz(sizeof(uv_loop_t));
    ret = uv_loop_init(loop);
    if (ret) {
        error("uv_loop_init(): %s", uv_strerror(ret));
        goto error_after_loop_init;
    }
    loop->data = wc;

    ret = uv_async_init(wc->loop, &wc->async, async_cb);
    if (ret) {
        error("uv_async_init(): %s", uv_strerror(ret));
        goto error_after_async_init;
    }
    wc->async.data = wc;

    ret = uv_timer_init(loop, &wc->timer_req);
    if (ret) {
        error("uv_timer_init(): %s", uv_strerror(ret));
        goto error_after_timer_init;
    }
    wc->timer_req.data = wc;
    fatal_assert(0 == uv_timer_start(&wc->timer_req, timer_cb, TIMER_INITIAL_PERIOD_MS, TIMER_REPEAT_PERIOD_MS));

    info("Starting metadata sync thread -- scratch area %d entries, %lu bytes", METADATA_CMD_Q_MAX_SIZE, sizeof(*wc));

    struct metadata_cmd cmd;
    memset(&cmd, 0, sizeof(cmd));
    metadata_flag_clear(wc, METADATA_FLAG_CLEANUP);
    metadata_flag_clear(wc, METADATA_FLAG_SCANNING_HOSTS);

    wc->check_metadata_after = now_realtime_sec() + METADATA_MAINTENANCE_FIRST_CHECK;
    wc->check_hosts_after    = now_realtime_sec() + METADATA_HOST_CHECK_FIRST_CHECK;

    int shutdown = 0;
    int in_transaction = 0;
    int commands_in_transaction = 0;
    BUFFER *work_buffer = buffer_create(1024);
    wc->row_id = 0;
    completion_mark_complete(&wc->init_complete);

    while (shutdown == 0 || (wc->flags & METADATA_WORKER_BUSY)) {
        RRDDIM *rd;
        RRDSET *st;
        RRDHOST *host;
        uuid_t  *uuid;
        int rc;
        DICTIONARY_ITEM *dict_item;
        BUFFER *buffer;

        worker_is_idle();
        uv_run(loop, UV_RUN_DEFAULT);

        /* wait for commands */
        cmd_batch_size = 0;
        do {
            if (unlikely(cmd_batch_size >= METADATA_MAX_BATCH_SIZE))
                break;

            cmd = metadata_deq_cmd(wc, &next_opcode);
            opcode = cmd.opcode;

            if (unlikely(opcode == METADATA_DATABASE_NOOP && metadata_flag_check(wc, METADATA_FLAG_SHUTDOWN))) {
                shutdown = 1;
                continue;
            }

            ++cmd_batch_size;

            // If we are not in transaction and this command is the same with the next ; start a transaction
            if (!in_transaction && opcode < METADATA_SKIP_TRANSACTION && opcode == next_opcode) {
                if (opcode != METADATA_DATABASE_NOOP) {
                    in_transaction = 1;
                    db_execute("BEGIN TRANSACTION;");
                }
            }

            if (likely(in_transaction)) {
                commands_in_transaction++;
            }

            if (likely(opcode != METADATA_DATABASE_NOOP))
                    worker_is_busy(opcode);

            switch (opcode) {
                case METADATA_DATABASE_NOOP:
                case METADATA_DATABASE_TIMER:
                    break;
                case METADATA_ADD_CHART:
                    dict_item = (DICTIONARY_ITEM * ) cmd.param[0];
                    st = (RRDSET *) dictionary_acquired_item_value(dict_item);

                    rc = sql_store_chart(
                        &st->chart_uuid,
                        &st->rrdhost->host_uuid,
                        string2str(st->parts.type),
                        string2str(st->parts.id),
                        string2str(st->parts.name),
                        rrdset_family(st),
                        rrdset_context(st),
                        rrdset_title(st),
                        rrdset_units(st),
                        rrdset_plugin_name(st),
                        rrdset_module_name(st),
                        st->priority,
                        st->update_every,
                        st->chart_type,
                        st->rrd_memory_mode,
                        st->entries);

                    if (unlikely(rc))
                        error_report("Failed to store chart %s", string2str(rd->id));

                    dictionary_acquired_item_release(st->rrdhost->rrdset_root_index, dict_item);
                    break;
                case METADATA_ADD_CHART_LABEL:
                    dict_item = (DICTIONARY_ITEM * ) cmd.param[0];
                    st = (RRDSET *) dictionary_acquired_item_value(dict_item);

                    struct label_str {
                        BUFFER  *sql;
                        int count;
                        char uuid_str[UUID_STR_LEN];
                    };

                    size_t old_version = st->rrdlabels_last_saved_version;
                    size_t new_version = dictionary_version(st->rrdlabels);

                    if(new_version != old_version) {
                        buffer_flush(work_buffer);
                        struct label_str tmp = {.sql = work_buffer, .count = 0};
                        uuid_unparse_lower(st->chart_uuid, tmp.uuid_str);
                        rrdlabels_walkthrough_read(st->rrdlabels, chart_label_store_to_sql_callback, &tmp);
                        st->rrdlabels_last_saved_version = new_version;
                        db_execute(buffer_tostring(work_buffer));
                    }

                    dictionary_acquired_item_release(st->rrdhost->rrdset_root_index, dict_item);
                    break;
                case METADATA_ADD_DIMENSION:
                    dict_item = (DICTIONARY_ITEM * ) cmd.param[0];
                    rd = (RRDDIM *) dictionary_acquired_item_value(dict_item);
//                    info("METADATA: Storing DIM %s (chart %s) (host %s)",
//                         string2str(rd->id), string2str(rd->rrdset->id), string2str(rd->rrdset->rrdhost->hostname));

                    rc = sql_store_dimension(
                        &rd->metric_uuid,
                        &rd->rrdset->chart_uuid,
                        string2str(rd->id),
                        string2str(rd->name),
                        rd->multiplier,
                        rd->divisor,
                        rd->algorithm);

                    if (unlikely(rc))
                        error_report("Failed to store dimension %s", string2str(rd->id));
                    dictionary_acquired_item_release(rd->rrdset->rrddim_root_index, dict_item);
                    break;
                case METADATA_DEL_DIMENSION:
                    uuid = (uuid_t *) cmd.param[0];
                    if (likely(dimension_can_be_deleted(uuid)))
                        delete_dimension_uuid(uuid);
                    freez(uuid);
                    break;
                case METADATA_ADD_DIMENSION_OPTION:
                    dict_item = (DICTIONARY_ITEM * ) cmd.param[0];
                    rd = (RRDDIM *) dictionary_acquired_item_value(dict_item);
//                    info("METADATA: Storing DIM OPTION %s", string2str(rd->id));
                    rc = sql_set_dimension_option(
                        &rd->metric_uuid, rrddim_flag_check(rd, RRDDIM_FLAG_META_HIDDEN) ? "hidden" : NULL);
                    if (unlikely(rc))
                        error_report("Failed to store dimension option for %s", string2str(rd->id));
                    dictionary_acquired_item_release(rd->rrdset->rrddim_root_index, dict_item);
                    break;
                case METADATA_ADD_HOST_SYSTEM_INFO:
                    dict_item = (DICTIONARY_ITEM * ) cmd.param[0];
                    host = (RRDHOST *) dictionary_acquired_item_value(dict_item);
//                    info("METADATA: Storing HOST SYSTEM INFO %s", string2str(host->hostname));
                    // TODO: Resolve race condition -- system info may be changed
                    sql_store_host_system_info(&host->host_uuid, host->system_info);
                    dictionary_acquired_item_release(rrdhost_root_index, dict_item);
                    break;
                case METADATA_ADD_HOST_INFO:
                    dict_item = (DICTIONARY_ITEM * ) cmd.param[0];
                    host = (RRDHOST *) dictionary_acquired_item_value(dict_item);
//                    info("METADATA: Storing HOST SYSTEM INFO %s", string2str(host->hostname));
                    rc = sql_store_host_info(host);
                    if (unlikely(rc))
                        error_report("Failed to store host info in the database for %s", string2str(host->hostname));
                    dictionary_acquired_item_release(rrdhost_root_index, dict_item);
                    break;
                case METADATA_STORE_CLAIM_ID:
//                    info("METADATA: Storing CLAIM ID FOR HOST");
                    store_claim_id((uuid_t *) cmd.param[0], (uuid_t *) cmd.param[1]);
                    freez((void *) cmd.param[0]);
                    freez((void *) cmd.param[1]);
                    break;
                case METADATA_STORE_HOST_LABELS:
                    dict_item = (DICTIONARY_ITEM * ) cmd.param[0];
                    host = (RRDHOST *) dictionary_acquired_item_value(dict_item);
//                    info("METADATA: Storing HOST LABELS %s", string2str(host->hostname));

                    rc = exec_statement_with_uuid(SQL_DELETE_HOST_LABELS, &host->host_uuid);
                    if (likely(rc == SQLITE_OK)) {
                        buffer_flush(work_buffer);
                        struct label_str tmp = {.sql = work_buffer, .count = 0};
                        uuid_unparse_lower(host->host_uuid, tmp.uuid_str);
                        rrdlabels_walkthrough_read(host->rrdlabels, host_label_store_to_sql_callback, &tmp);
                        db_execute(buffer_tostring(work_buffer));
                    }

                    dictionary_acquired_item_release(rrdhost_root_index, dict_item);
                    break;

                case METADATA_SCAN_HOSTS:
                    if (unlikely(metadata_flag_check(wc, METADATA_FLAG_SCANNING_HOSTS)))
                        break;

                    struct scan_metadata_payload *data = mallocz(sizeof(*data));
                    data->wc = wc;
                    data->max_count = 1000;

                    metadata_scan_hosts_worker.data = data;
                    metadata_flag_set(wc, METADATA_FLAG_SCANNING_HOSTS);
                    internal_error(true, "Starting metadata host check in a worker thread");
                    if (unlikely(
                            uv_queue_work(loop, &metadata_scan_hosts_worker, start_metadata_hosts, after_metadata_hosts))) {
                        metadata_flag_clear(wc, METADATA_FLAG_SCANNING_HOSTS);
                    }
                    break;
                case METADATA_STORE_BUFFER:
                    buffer = (BUFFER *) cmd.param[0];

                    /// TODO: Error checking
                    db_execute(buffer_tostring(buffer));

                    buffer_free(buffer);
                    break;
                case METADATA_MAINTENANCE:
                    if (unlikely(metadata_flag_check(wc, METADATA_FLAG_CLEANUP)))
                        break;

                    metadata_cleanup_worker.data = wc;
                    metadata_flag_set(wc, METADATA_FLAG_CLEANUP);
                    if (unlikely(
                            uv_queue_work(loop, &metadata_cleanup_worker, start_metadata_cleanup, after_metadata_cleanup))) {
                        metadata_flag_clear(wc, METADATA_FLAG_CLEANUP);
                    }
                    break;
                case METADATA_UNITTEST:
                    ;
                    struct thread_unittest {
                        int join;
                        unsigned added;
                        unsigned processed;
                        unsigned *done;
                    };

                    struct thread_unittest *tu = (struct thread_unittest *) cmd.param[0];
                    usleep(1000); // processing takes 1ms
                    __atomic_fetch_add(&tu->processed, 1, __ATOMIC_SEQ_CST);
                    break;
                default:
                    break;
            }
            if (in_transaction && (commands_in_transaction >= METADATA_MAX_TRANSACTION_BATCH || opcode != next_opcode)) {
                in_transaction = 0;
                db_execute("COMMIT TRANSACTION;");
                //info("METADATA: Ending transaction commands %d", commands_in_transaction);
                commands_in_transaction = 0;
            }

            if (cmd.completion)
                completion_mark_complete(cmd.completion);
        } while (opcode != METADATA_DATABASE_NOOP);
    }

    if (!uv_timer_stop(&wc->timer_req))
        uv_close((uv_handle_t *)&wc->timer_req, NULL);

    /*
     * uv_async_send after uv_close does not seem to crash in linux at the moment,
     * it is however undocumented behaviour we need to be aware if this becomes
     * an issue in the future.
     */
    uv_close((uv_handle_t *)&wc->async, NULL);
    uv_run(loop, UV_RUN_DEFAULT);

    /* TODO: don't let the API block by waiting to enqueue commands */
    uv_cond_destroy(&wc->cmd_cond);
    /*  uv_mutex_destroy(&wc->cmd_mutex); */
    //fatal_assert(0 == uv_loop_close(loop));
    int rc;

    do {
        rc = uv_loop_close(loop);
    } while (rc != UV_EBUSY);

    freez(loop);
    worker_unregister();

    buffer_free(work_buffer);
    info("METADATA: Shutting down event loop");
    completion_mark_complete(&wc->init_complete);
    return;

error_after_timer_init:
    uv_close((uv_handle_t *)&wc->async, NULL);
error_after_async_init:
    fatal_assert(0 == uv_loop_close(loop));
error_after_loop_init:
    freez(loop);
    worker_unregister();
}

struct metadata_wc metasync_worker;

void metadata_sync_shutdown(void)
{
    completion_init(&metasync_worker.init_complete);

    info("METADATA: Starting shutdown...");
    struct metadata_cmd cmd;
    memset(&cmd, 0, sizeof(cmd));

    info("METADATA: Sending a shutdown command");
    cmd.opcode = METADATA_SYNC_SHUTDOWN;
    metadata_enq_cmd(&metasync_worker, &cmd);

    /* wait for metadata thread to shut down */
    info("METADATA: Waiting for shutdown ACK");
    completion_wait_for(&metasync_worker.init_complete);
    completion_destroy(&metasync_worker.init_complete);
    info("METADATA: Shutdown complete");
}

void metadata_sync_shutdown_prepare(void)
{
    struct metadata_cmd cmd;
    memset(&cmd, 0, sizeof(cmd));

    struct completion compl;
    completion_init(&compl);

    info("METADATA: Sending a scan host command");
    cmd.opcode = METADATA_SCAN_HOSTS;
    cmd.completion = &compl;
    metadata_enq_cmd(&metasync_worker, &cmd);

    info("METADATA: Waiting for host scan completion");
    completion_wait_for(&compl);
    completion_destroy(&compl);
    info("METADATA: Host scan complete; can continue with shutdown");
}

// -------------------------------------------------------------
// Init function called on agent startup

void metadata_sync_init(void)
{
    struct metadata_wc *wc = &metasync_worker;

    fatal_assert(0 == uv_mutex_init(&metadata_async_lock));

    memset(wc, 0, sizeof(*wc));
    metadata_database_init_cmd_queue(wc);
    completion_init(&wc->init_complete);

    fatal_assert(0 == uv_thread_create(&(wc->thread), metadata_event_loop, wc));

    completion_wait_for(&wc->init_complete);
    completion_destroy(&wc->init_complete);

    info("SQLite metadata sync initialization complete");
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
void metaqueue_chart_update(RRDSET *st)
{
    const DICTIONARY_ITEM *acquired_st = dictionary_get_and_acquire_item(st->rrdhost->rrdset_root_index, string2str(st->id));
    queue_metadata_cmd(METADATA_ADD_CHART, acquired_st, NULL);
}

//
// RD may not be collected, so we may store it needlessly
void metaqueue_dimension_update(RRDDIM *rd)
{
    const DICTIONARY_ITEM *acquired_rd =
        dictionary_get_and_acquire_item(rd->rrdset->rrddim_root_index, string2str(rd->id));

    if (unlikely(rrdset_flag_check(rd->rrdset, RRDSET_FLAG_METADATA_UPDATE))) {
        metaqueue_chart_update(rd->rrdset);
        rrdset_flag_clear(rd->rrdset, RRDSET_FLAG_METADATA_UPDATE);
    }

    queue_metadata_cmd(METADATA_ADD_DIMENSION, acquired_rd, NULL);
}

void metaqueue_dimension_update_flags(RRDDIM *rd)
{
    const DICTIONARY_ITEM *acquired_rd =
        dictionary_get_and_acquire_item(rd->rrdset->rrddim_root_index, string2str(rd->id));
    queue_metadata_cmd(METADATA_ADD_DIMENSION_OPTION, acquired_rd, NULL);
}

void metaqueue_host_update_system_info(const char *machine_guid)
{
    const DICTIONARY_ITEM *acquired_host = dictionary_get_and_acquire_item(rrdhost_root_index, machine_guid);
    queue_metadata_cmd(METADATA_ADD_HOST_SYSTEM_INFO, acquired_host, NULL);
}

void metaqueue_host_update_info(const char *machine_guid)
{
    const DICTIONARY_ITEM *acquired_host = dictionary_get_and_acquire_item(rrdhost_root_index, machine_guid);
    queue_metadata_cmd(METADATA_ADD_HOST_INFO, acquired_host, NULL);
}

void metaqueue_delete_dimension_uuid(uuid_t *uuid)
{
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

void metaqueue_store_host_labels(const char *machine_guid)
{
    const DICTIONARY_ITEM *acquired_host = dictionary_get_and_acquire_item(rrdhost_root_index, machine_guid);
    queue_metadata_cmd(METADATA_STORE_HOST_LABELS, acquired_host, NULL);
}

void metaqueue_buffer(BUFFER *buffer)
{
    queue_metadata_cmd(METADATA_STORE_BUFFER, buffer, NULL);
}

void metaqueue_chart_labels(RRDSET *st)
{
    const DICTIONARY_ITEM *acquired_st = dictionary_get_and_acquire_item(st->rrdhost->rrdset_root_index, string2str(st->id));
    queue_metadata_cmd(METADATA_ADD_CHART_LABEL, acquired_st, NULL);
}


//
// unitests
//

struct thread_unittest {
    int join;
    unsigned added;
    unsigned processed;
    unsigned *done;
};

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
        if (metadata_database_enq_cmd_noblock(&metasync_worker, &cmd))
            break;
        usleep(10000);
    } while (!__atomic_load_n(&tu->join, __ATOMIC_RELAXED));
    return arg;
}

static void *metadata_unittest_threads(int items __maybe_unused)
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
        "\nChecking metadata queue using %d threads for %ld seconds...\n",
        threads_to_create,
        seconds_to_run);

    netdata_thread_t threads[threads_to_create];
    tu.join = 0;
    for (int i = 0; i < threads_to_create; i++) {
        char buf[100 + 1];
        snprintf(buf, 100, "meta%d", i);
        netdata_thread_create(
            &threads[i],
            buf,
            NETDATA_THREAD_OPTION_DONT_LOG | NETDATA_THREAD_OPTION_JOINABLE,
            unittest_queue_metadata,
            &tu);
    }
    uv_async_send(&metasync_worker.async);
    sleep_usec(seconds_to_run * USEC_PER_SEC);

    __atomic_store_n(&tu.join, 1, __ATOMIC_RELAXED);
    for (int i = 0; i < threads_to_create; i++) {
        void *retval;
        netdata_thread_join(threads[i], &retval);
    }
    fprintf(stderr, "Added %u elements, processed %u\n", tu.added, tu.processed);

    uv_async_send(&metasync_worker.async);

    sleep_usec(5 * USEC_PER_SEC);

    fprintf(stderr, "Added %u elements, processed %u\n", tu.added, tu.processed);

    return 0;
}

int metadata_unittest(int items)
{
    metadata_sync_init();

    // Queue items
    metadata_unittest_threads(items);

    fprintf(stderr, "Items still in queue %u\n", metasync_worker.queue_size);
    metadata_sync_shutdown();

    return 0;
}

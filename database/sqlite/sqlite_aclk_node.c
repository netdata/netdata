// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_aclk_node.h"

#ifndef ACLK_NG
#include "../../aclk/legacy/agent_cloud_link.h"
#else
#include "../../aclk/aclk.h"
#include "../../aclk/aclk_charts_api.h"
#include "../../aclk/aclk_alarm_api.h"
#endif


void aclk_reset_node_event(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    int rc;
    uuid_t  host_id;

    UNUSED(wc);

    rc = uuid_parse((char *) cmd.data, host_id);
    if (unlikely(rc))
        goto fail1;

    BUFFER *sql = buffer_create(1024);

    buffer_sprintf(sql, "UPDATE node_instance set node_id = null WHERE host_id = @host_id;");

    sqlite3_stmt *res = NULL;
    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to reset node id in the database");
        goto fail;
    }

    rc = sqlite3_bind_blob(res, 1, &host_id , sizeof(host_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to reset the node id of host %s, rc = %d", (char *) cmd.data, rc);

    bind_fail:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement when doing a node reset, rc = %d", rc);

    fail:
    buffer_free(sql);

    fail1:
    return;
}


void sql_drop_host_aclk_table_list(uuid_t *host_uuid)
{
    int rc;
    char uuid_str[GUID_LEN + 1];

    uuid_unparse_lower_fix(host_uuid, uuid_str);

    BUFFER *sql = buffer_create(1024);
    buffer_sprintf(
        sql,"SELECT 'drop '||type||' IF EXISTS '||name||';' FROM sqlite_schema " \
        "WHERE name LIKE 'aclk_%%_%s' AND type IN ('table', 'trigger', 'index');", uuid_str);

    sqlite3_stmt *res = NULL;

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to clean up aclk tables");
        goto fail;
    }
    buffer_flush(sql);

    while (sqlite3_step(res) == SQLITE_ROW)
        buffer_strcat(sql, (char *) sqlite3_column_text(res, 0));

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement to clean up aclk tables, rc = %d", rc);

    db_execute(buffer_tostring(sql));

    fail:
    buffer_free(sql);
}

void sql_aclk_drop_all_table_list()
{
    int rc;

    BUFFER *sql = buffer_create(1024);
    buffer_strcat(sql, "SELECT host_id FROM host;");
    sqlite3_stmt *res = NULL;

    rc = sqlite3_prepare_v2(db_meta, buffer_tostring(sql), -1, &res, 0);
    if (rc != SQLITE_OK) {
        error_report("Failed to prepare statement to clean up aclk tables");
        goto fail;
    }
    while (sqlite3_step(res) == SQLITE_ROW) {
        sql_drop_host_aclk_table_list((uuid_t *)sqlite3_column_blob(res, 0));
    }

    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize statement to clean up aclk tables, rc = %d", rc);

    fail:
    buffer_free(sql);
    return;
}



void sql_build_node_info(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    UNUSED(cmd);

    struct update_node_info node_info;

    rrd_wrlock();
    node_info.node_id = get_str_from_uuid(wc->host->node_id);
    node_info.claim_id = is_agent_claimed();
    node_info.machine_guid = strdupz(wc->host_guid);
    node_info.child = (wc->host != localhost);
    now_realtime_timeval(&node_info.updated_at);

    RRDHOST *host = wc->host;

    node_info.data.name = strdupz(host->hostname);
    node_info.data.os = strdupz(host->os);
    node_info.data.os_name = strdupz(host->system_info->host_os_name);
    node_info.data.os_version = strdupz(host->system_info->host_os_version);
    node_info.data.kernel_name = strdupz(host->system_info->kernel_name);
    node_info.data.kernel_version = strdupz(host->system_info->kernel_version);
    node_info.data.architecture = strdupz(host->system_info->architecture);
    node_info.data.cpus = str2uint32_t(host->system_info->host_cores);
    node_info.data.cpu_frequency = strdupz(host->system_info->host_cpu_freq);
    node_info.data.memory = strdupz(host->system_info->host_ram_total);
    node_info.data.disk_space = strdupz(host->system_info->host_disk_space);
    node_info.data.version = strdupz(VERSION);
    node_info.data.release_channel = strdupz("nightly");
    node_info.data.timezone = strdupz("UTC");
    node_info.data.virtualization_type = strdupz(host->system_info->virtualization);
    node_info.data.container_type = strdupz(host->system_info->container);
    node_info.data.custom_info = config_get(CONFIG_SECTION_WEB, "custom dashboard_info.js", "");
    node_info.data.services = NULL;   // char **
    node_info.data.service_count = 0;
    node_info.data.machine_guid = strdupz(wc->host_guid);
    node_info.data.host_labels_head = NULL;     //struct label *host_labels_head;

    rrd_unlock();
    aclk_update_node_info(&node_info);
    return;
}


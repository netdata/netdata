// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_aclk_node.h"

#ifdef ENABLE_NEW_CLOUD_PROTOCOL
#include "../../aclk/aclk_charts_api.h"
#include "../../aclk/aclk.h"
#endif

void sql_build_node_info(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    UNUSED(cmd);

#ifdef ENABLE_NEW_CLOUD_PROTOCOL
    struct update_node_info node_info;

    if (!wc->host)
        return;

    rrd_wrlock();
    node_info.node_id = wc->node_id;
    node_info.claim_id = is_agent_claimed();
    node_info.machine_guid = wc->host_guid;
    node_info.child = (wc->host != localhost);
    now_realtime_timeval(&node_info.updated_at);

    RRDHOST *host = wc->host;

    node_info.data.name = host->hostname;
    node_info.data.os = (char *) host->os;
    node_info.data.os_name = host->system_info->host_os_name;
    node_info.data.os_version = host->system_info->host_os_version;
    node_info.data.kernel_name = host->system_info->kernel_name;
    node_info.data.kernel_version = host->system_info->kernel_version;
    node_info.data.architecture = host->system_info->architecture;
    node_info.data.cpus = str2uint32_t(host->system_info->host_cores);
    node_info.data.cpu_frequency = host->system_info->host_cpu_freq;
    node_info.data.memory = host->system_info->host_ram_total;
    node_info.data.disk_space = host->system_info->host_disk_space;
    node_info.data.version = VERSION;
    node_info.data.release_channel = "nightly";
    node_info.data.timezone = (char *) host->abbrev_timezone;
    node_info.data.virtualization_type = host->system_info->virtualization;
    node_info.data.container_type = host->system_info->container;
    node_info.data.custom_info = config_get(CONFIG_SECTION_WEB, "custom dashboard_info.js", "");
    node_info.data.services = NULL;   // char **
    node_info.data.service_count = 0;
    node_info.data.machine_guid = wc->host_guid;

    struct label_index *labels = &host->labels;
    netdata_rwlock_wrlock(&labels->labels_rwlock);
    node_info.data.host_labels_head = labels->head;

    aclk_update_node_info(&node_info);

    netdata_rwlock_unlock(&labels->labels_rwlock);
    rrd_unlock();
    freez(node_info.claim_id);
#else
    UNUSED(wc);
#endif

    return;
}
#define SQL_SELECT_HOST_MEMORY_MODE "select memory_mode from chart where host_id = @host_id limit 1;"
static RRD_MEMORY_MODE sql_get_host_memory_mode(uuid_t *host_id)
{
    int rc;

    RRD_MEMORY_MODE memory_mode = RRD_MEMORY_MODE_RAM;
    sqlite3_stmt *res = NULL;

    rc = sqlite3_prepare_v2(db_meta, SQL_SELECT_HOST_MEMORY_MODE, -1, &res, 0);

    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to read host memory mode");
        return memory_mode;
    }

    rc = sqlite3_bind_blob(res, 1, host_id, sizeof(*host_id), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host parameter to fetch host memory mode");
        goto failed;
    }

    while (sqlite3_step(res) == SQLITE_ROW) {
        memory_mode = (RRD_MEMORY_MODE) sqlite3_column_int(res, 0);
    }

failed:
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when reading host memory mode");
    return memory_mode;
}

#define SELECT_HOST_DIMENSION_LIST  "SELECT d.dim_id, c.update_every, c.type||'.'||c.id FROM chart c, dimension d, host h " \
        "WHERE d.chart_id = c.chart_id AND c.host_id = h.host_id AND c.host_id = @host_id ORDER BY c.update_every ASC;"

#define SELECT_HOST_CHART_LIST  "SELECT distinct h.host_id, c.update_every, c.type||'.'||c.id FROM chart c, host h " \
        "WHERE c.host_id = h.host_id AND c.host_id = @host_id ORDER BY c.update_every ASC;"

void aclk_update_retention(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    UNUSED(cmd);
#ifdef ENABLE_NEW_CLOUD_PROTOCOL
    int rc;

    if (!aclk_use_new_cloud_arch || !aclk_connected)
        return;

    char *claim_id = is_agent_claimed();
    if (unlikely(!claim_id))
        return;

    sqlite3_stmt *res = NULL;
    RRD_MEMORY_MODE memory_mode;

    uuid_t host_uuid;
    rc = uuid_parse(wc->host_guid, host_uuid);
    if (unlikely(rc)) {
        freez(claim_id);
        return;
    }

    if (wc->host)
        memory_mode = wc->host->rrd_memory_mode;
    else
        memory_mode = sql_get_host_memory_mode(&host_uuid);

    if (memory_mode == RRD_MEMORY_MODE_DBENGINE)
        rc = sqlite3_prepare_v2(db_meta, SELECT_HOST_DIMENSION_LIST, -1, &res, 0);
    else
        rc = sqlite3_prepare_v2(db_meta, SELECT_HOST_CHART_LIST, -1, &res, 0);

    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to prepare statement to fetch host dimensions");
        freez(claim_id);
        return;
    }

    rc = sqlite3_bind_blob(res, 1, &host_uuid, sizeof(host_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to bind host parameter to fetch host dimensions");
        goto failed;
    }

    time_t  start_time = LONG_MAX;
    time_t  first_entry_t;
    uint32_t update_every = 0;

    struct retention_updated rotate_data;

    memset(&rotate_data, 0, sizeof(rotate_data));

    int max_intervals = 32;

    rotate_data.interval_duration_count = 0;
    rotate_data.interval_durations = callocz(max_intervals, sizeof(*rotate_data.interval_durations));

    now_realtime_timeval(&rotate_data.rotation_timestamp);
    rotate_data.memory_mode = memory_mode;
    rotate_data.claim_id = claim_id;
    rotate_data.node_id = strdupz(wc->node_id);

    while (sqlite3_step(res) == SQLITE_ROW) {
        if (!update_every || update_every != (uint32_t) sqlite3_column_int(res, 1)) {
            if (update_every) {
                debug(D_ACLK_SYNC,"Update %s for %u oldest time = %ld", wc->host_guid, update_every, start_time);
                rotate_data.interval_durations[rotate_data.interval_duration_count].retention = rotate_data.rotation_timestamp.tv_sec - start_time;
                rotate_data.interval_duration_count++;
            }
            update_every = (uint32_t) sqlite3_column_int(res, 1);
            rotate_data.interval_durations[rotate_data.interval_duration_count].update_every = update_every;
            start_time = LONG_MAX;
        }
#ifdef ENABLE_DBENGINE
        time_t  last_entry_t;
        if (memory_mode == RRD_MEMORY_MODE_DBENGINE)
            rc = rrdeng_metric_latest_time_by_uuid((uuid_t *)sqlite3_column_blob(res, 0), &first_entry_t, &last_entry_t);
        else
#endif
        {
            if (wc->host) {
                RRDSET *st = NULL;
                rc = (st = rrdset_find(wc->host, (const char *)sqlite3_column_text(res, 2))) ? 0 : 1;
                if (!rc)
                    first_entry_t = rrdset_first_entry_t(st);
            }
            else {
                 rc = 0;
                 first_entry_t = rotate_data.rotation_timestamp.tv_sec;
            }
        }

        if (likely(!rc && first_entry_t))
            start_time = MIN(start_time, first_entry_t);
    }
    if (update_every) {
        debug(D_ACLK_SYNC, "Update %s for %u oldest time = %ld", wc->host_guid, update_every, start_time);
        rotate_data.interval_durations[rotate_data.interval_duration_count].retention = rotate_data.rotation_timestamp.tv_sec - start_time;
        rotate_data.interval_duration_count++;
    }

    for (int i = 0; i < rotate_data.interval_duration_count; ++i) {
        debug(D_ACLK_SYNC,"%d --> Update %s for %u  Retention = %u", i, wc->host_guid,
             rotate_data.interval_durations[i].update_every, rotate_data.interval_durations[i].retention);
    };
    aclk_retention_updated(&rotate_data);
    freez(rotate_data.node_id);
    freez(rotate_data.interval_durations);

failed:
    freez(claim_id);
    rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to finalize the prepared statement when reading host dimensions");
#else
    UNUSED(wc);
#endif
    return;
}
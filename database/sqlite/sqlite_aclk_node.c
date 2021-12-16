// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_aclk_node.h"

#ifdef ENABLE_NEW_CLOUD_PROTOCOL
#include "../../aclk/aclk_charts_api.h"
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
    node_info.ml_on_parent = (wc->host != localhost) && (wc->host->ml_host != NULL);
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
    node_info.data.ml_capable = host->system_info->ml_capable;
    node_info.data.ml_enabled = host->system_info->ml_enabled;

    struct label_index *labels = &host->labels;
    netdata_rwlock_wrlock(&labels->labels_rwlock);
    node_info.data.host_labels_head = labels->head;

    aclk_update_node_info(&node_info);
    log_access("OG [%s (%s)]: Sending node info for guid [%s] (%s).", wc->node_id, wc->host->hostname, wc->host_guid, wc->host == localhost ? "parent" : "child");

    netdata_rwlock_unlock(&labels->labels_rwlock);
    rrd_unlock();
    freez(node_info.claim_id);
#else
    UNUSED(wc);
#endif

    return;
}

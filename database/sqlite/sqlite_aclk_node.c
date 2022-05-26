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

    rrd_rdlock();
    node_info.node_id = wc->node_id;
    node_info.claim_id = is_agent_claimed();
    node_info.machine_guid = wc->host_guid;
    node_info.child = (wc->host != localhost);
    node_info.ml_info.ml_capable = ml_capable(localhost);
    node_info.ml_info.ml_enabled = ml_enabled(wc->host);

    struct capability instance_caps[] = {
        { .name = "proto", .version = 1,                     .enabled = 1 },
        { .name = "ml",    .version = ml_capable(localhost), .enabled = ml_enabled(wc->host) },
        { .name = NULL,    .version = 0,                     .enabled = 0 }
    };
    node_info.node_instance_capabilities = instance_caps;

    now_realtime_timeval(&node_info.updated_at);

    RRDHOST *host = wc->host;
    char *host_version = NULL;
    if (host != localhost) {
        netdata_mutex_lock(&host->receiver_lock);
        host_version =
            strdupz(host->receiver && host->receiver->program_version ? host->receiver->program_version : "unknown");
        netdata_mutex_unlock(&host->receiver_lock);
    }

    node_info.data.name = host->hostname;
    node_info.data.os = (char *) host->os;
    node_info.data.os_name = host->system_info->host_os_name;
    node_info.data.os_version = host->system_info->host_os_version;
    node_info.data.kernel_name = host->system_info->kernel_name;
    node_info.data.kernel_version = host->system_info->kernel_version;
    node_info.data.architecture = host->system_info->architecture;
    node_info.data.cpus = host->system_info->host_cores ? str2uint32_t(host->system_info->host_cores) : 0;
    node_info.data.cpu_frequency = host->system_info->host_cpu_freq ? host->system_info->host_cpu_freq : "0";
    node_info.data.memory = host->system_info->host_ram_total ? host->system_info->host_ram_total : "0";
    node_info.data.disk_space = host->system_info->host_disk_space ? host->system_info->host_disk_space : "0";
    node_info.data.version = host_version ? host_version : VERSION;
    node_info.data.release_channel = (char *) get_release_channel();
    node_info.data.timezone = (char *) host->abbrev_timezone;
    node_info.data.virtualization_type = host->system_info->virtualization ? host->system_info->virtualization : "unknown";
    node_info.data.container_type = host->system_info->container ? host->system_info->container : "unknown";
    node_info.data.custom_info = config_get(CONFIG_SECTION_WEB, "custom dashboard_info.js", "");
    node_info.data.collectors = NULL;   // char **
    node_info.data.collector_count = 0;
    node_info.data.machine_guid = wc->host_guid;

    struct capability node_caps[] = {
        { .name = "ml", .version = host->system_info->ml_capable, .enabled = host->system_info->ml_enabled },
        { .name = NULL, .version = 0, .enabled = 0 }
    };
    node_info.node_capabilities = node_caps;

    node_info.data.ml_info.ml_capable = host->system_info->ml_capable;
    node_info.data.ml_info.ml_enabled = host->system_info->ml_enabled;

    struct label_index *labels = &host->labels;
    netdata_rwlock_wrlock(&labels->labels_rwlock);
    node_info.data.host_labels_head = labels->head;

    aclk_update_node_info(&node_info);
    log_access("ACLK RES [%s (%s)]: NODE INFO SENT for guid [%s] (%s)", wc->node_id, wc->host->hostname, wc->host_guid, wc->host == localhost ? "parent" : "child");

    netdata_rwlock_unlock(&labels->labels_rwlock);
    rrd_unlock();
    freez(node_info.claim_id);
    freez(host_version);
#else
    UNUSED(wc);
#endif

    return;
}

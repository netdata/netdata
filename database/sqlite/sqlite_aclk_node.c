// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_aclk_node.h"

#ifdef ENABLE_ACLK
#include "../../aclk/aclk_charts_api.h"
#endif

void sql_build_node_info(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    UNUSED(cmd);

#ifdef ENABLE_ACLK
    struct update_node_info node_info;

    rrd_wrlock();
    node_info.node_id = strdupz(wc->node_id);
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
    node_info.data.timezone = strdupz(host->abbrev_timezone);
    node_info.data.virtualization_type = strdupz(host->system_info->virtualization);
    node_info.data.container_type = strdupz(host->system_info->container);
    node_info.data.custom_info = config_get(CONFIG_SECTION_WEB, "custom dashboard_info.js", "");
    node_info.data.services = NULL;   // char **
    node_info.data.service_count = 0;
    node_info.data.machine_guid = strdupz(wc->host_guid);

    struct label_index *labels = &host->labels;
    netdata_rwlock_wrlock(&labels->labels_rwlock);
    struct label *label_list = labels->head;
    struct label *aclk_label = NULL;
    while (label_list) {
        aclk_label = add_label_to_list(aclk_label, label_list->key, label_list->value, label_list->label_source);
        label_list = label_list->next;
    }

    netdata_rwlock_unlock(&labels->labels_rwlock);
    node_info.data.host_labels_head = aclk_label;
    rrd_unlock();

    aclk_update_node_info(&node_info);

    free_label_list(aclk_label);
    freez(node_info.node_id);
    freez(node_info.claim_id);
    freez(node_info.machine_guid);
    freez(node_info.data.name);
    freez(node_info.data.os);
    freez(node_info.data.os_name);
    freez(node_info.data.os_version);
    freez(node_info.data.kernel_name);
    freez(node_info.data.kernel_version);
    freez(node_info.data.architecture);
    freez(node_info.data.cpu_frequency);
    freez(node_info.data.memory);
    freez(node_info.data.disk_space);
    freez(node_info.data.version);
    freez(node_info.data.release_channel);
    freez(node_info.data.timezone);
    freez(node_info.data.virtualization_type);
    freez(node_info.data.container_type);
    freez(node_info.data.machine_guid);
#else
    UNUSED(wc);
#endif

    return;
}

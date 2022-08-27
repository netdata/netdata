// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_aclk_node.h"

#ifdef ENABLE_ACLK
#include "../../aclk/aclk_charts_api.h"
#endif

#ifdef ENABLE_ACLK
DICTIONARY *collectors_from_charts(RRDHOST *host, DICTIONARY *dict) {
    RRDSET *st;
    char name[500];

    rrdhost_rdlock(host);
    rrdset_foreach_read(st, host) {
        if (rrdset_is_available_for_viewers(st)) {
            struct collector_info col = {
                    .plugin = rrdset_plugin_name(st),
                    .module = rrdset_module_name(st)
            };
            snprintfz(name, 499, "%s:%s", col.plugin, col.module);
            dictionary_set(dict, name, &col, sizeof(struct collector_info));
        }
    }
    rrdhost_unlock(host);

    return dict;
}
#endif

void sql_build_node_collectors(struct aclk_database_worker_config *wc)
{
#ifdef ENABLE_ACLK
    if (!wc->host)
        return;

    struct update_node_collectors upd_node_collectors;
    DICTIONARY *dict = dictionary_create(DICTIONARY_FLAG_SINGLE_THREADED);

    upd_node_collectors.node_id = wc->node_id;
    upd_node_collectors.claim_id = get_agent_claimid();

    upd_node_collectors.node_collectors = collectors_from_charts(wc->host, dict);
    aclk_update_node_collectors(&upd_node_collectors);

    dictionary_destroy(dict);
    freez(upd_node_collectors.claim_id);

    log_access("ACLK RES [%s (%s)]: NODE COLLECTORS SENT", wc->node_id, rrdhost_hostname(wc->host));
#else
    UNUSED(wc);
#endif
    return;
}

void sql_build_node_info(struct aclk_database_worker_config *wc, struct aclk_database_cmd cmd)
{
    UNUSED(cmd);

#ifdef ENABLE_ACLK
    struct update_node_info node_info;

    if (!wc->host) {
        wc->node_info_send = 1;
        return;
    }

    rrd_rdlock();
    node_info.node_id = wc->node_id;
    node_info.claim_id = get_agent_claimid();
    node_info.machine_guid = wc->host_guid;
    node_info.child = (wc->host != localhost);
    node_info.ml_info.ml_capable = ml_capable(localhost);
    node_info.ml_info.ml_enabled = ml_enabled(wc->host);

    struct capability instance_caps[] = {
        { .name = "proto", .version = 1,                     .enabled = 1 },
        { .name = "ml",    .version = ml_capable(localhost), .enabled = ml_enabled(wc->host) },
        { .name = "mc",    .version = enable_metric_correlations ? metric_correlations_version : 0, .enabled = enable_metric_correlations },
        { .name = "ctx",   .version = 1,                     .enabled = rrdcontext_enabled},
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

    node_info.data.name = (char *)rrdhost_hostname(host);
    node_info.data.os = (char *)rrdhost_os(host);
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
    node_info.data.machine_guid = wc->host_guid;

    struct capability node_caps[] = {
        { .name = "ml", .version = host->system_info->ml_capable, .enabled = host->system_info->ml_enabled },
        { .name = "mc", .version = host->system_info->mc_version ? host->system_info->mc_version : 0, .enabled = host->system_info->mc_version ? 1 : 0 },
        { .name = NULL, .version = 0, .enabled = 0 }
    };
    node_info.node_capabilities = node_caps;

    node_info.data.ml_info.ml_capable = host->system_info->ml_capable;
    node_info.data.ml_info.ml_enabled = host->system_info->ml_enabled;

    node_info.data.host_labels_ptr = host->host_labels;

    aclk_update_node_info(&node_info);
    log_access("ACLK RES [%s (%s)]: NODE INFO SENT for guid [%s] (%s)", wc->node_id, rrdhost_hostname(wc->host), wc->host_guid, wc->host == localhost ? "parent" : "child");

    rrd_unlock();
    freez(node_info.claim_id);
    freez(host_version);

    wc->node_collectors_send = now_realtime_sec();
#else
    UNUSED(wc);
#endif

    return;
}

// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_aclk_node.h"

#include "../../aclk/aclk_contexts_api.h"
#include "../../aclk/aclk_capas.h"

#ifdef ENABLE_ACLK
DICTIONARY *collectors_from_charts(RRDHOST *host, DICTIONARY *dict) {
    RRDSET *st;
    char name[500];

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
    rrdset_foreach_done(st);

    return dict;
}

static void build_node_collectors(char *node_id __maybe_unused)
{

    RRDHOST *host = find_host_by_node_id(node_id);

    if (unlikely(!host))
        return;

    struct aclk_sync_host_config *wc = (struct aclk_sync_host_config *) host->aclk_sync_host_config;
    if (unlikely(!wc))
        return;

    struct update_node_collectors upd_node_collectors;
    DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED);

    upd_node_collectors.node_id = wc->node_id;
    upd_node_collectors.claim_id = get_agent_claimid();

    upd_node_collectors.node_collectors = collectors_from_charts(host, dict);
    aclk_update_node_collectors(&upd_node_collectors);

    dictionary_destroy(dict);
    freez(upd_node_collectors.claim_id);

    log_access("ACLK RES [%s (%s)]: NODE COLLECTORS SENT", node_id, rrdhost_hostname(host));

    freez(node_id);
}

static void build_node_info(char *node_id __maybe_unused)
{
    struct update_node_info node_info;

    RRDHOST *host = find_host_by_node_id(node_id);

    if (unlikely((!host))) {
        freez(node_id);
        return;
    }

    struct aclk_sync_host_config *wc = (struct aclk_sync_host_config *) host->aclk_sync_host_config;

    if (unlikely(!wc)) {
        freez(node_id);
        return;
    }

    rrd_rdlock();
    node_info.node_id = wc->node_id;
    node_info.claim_id = get_agent_claimid();
    node_info.machine_guid = host->machine_guid;
    node_info.child = (wc->host != localhost);
    node_info.ml_info.ml_capable = ml_capable();
    node_info.ml_info.ml_enabled = ml_enabled(wc->host);

    node_info.node_instance_capabilities = aclk_get_node_instance_capas(wc->host);

    now_realtime_timeval(&node_info.updated_at);

    char *host_version = NULL;
    if (host != localhost) {
        netdata_mutex_lock(&host->receiver_lock);
        host_version = strdupz(host->receiver && host->receiver->program_version ? host->receiver->program_version : rrdhost_program_version(host));
        netdata_mutex_unlock(&host->receiver_lock);
    }

    node_info.data.name = rrdhost_hostname(host);
    node_info.data.os = rrdhost_os(host);
    node_info.data.os_name = host->system_info->host_os_name;
    node_info.data.os_version = host->system_info->host_os_version;
    node_info.data.kernel_name = host->system_info->kernel_name;
    node_info.data.kernel_version = host->system_info->kernel_version;
    node_info.data.architecture = host->system_info->architecture;
    node_info.data.cpus = host->system_info->host_cores ? str2uint32_t(host->system_info->host_cores, NULL) : 0;
    node_info.data.cpu_frequency = host->system_info->host_cpu_freq ? host->system_info->host_cpu_freq : "0";
    node_info.data.memory = host->system_info->host_ram_total ? host->system_info->host_ram_total : "0";
    node_info.data.disk_space = host->system_info->host_disk_space ? host->system_info->host_disk_space : "0";
    node_info.data.version = host_version ? host_version : VERSION;
    node_info.data.release_channel = get_release_channel();
    node_info.data.timezone = rrdhost_abbrev_timezone(host);
    node_info.data.virtualization_type = host->system_info->virtualization ? host->system_info->virtualization : "unknown";
    node_info.data.container_type = host->system_info->container ? host->system_info->container : "unknown";
    node_info.data.custom_info = config_get(CONFIG_SECTION_WEB, "custom dashboard_info.js", "");
    node_info.data.machine_guid = host->machine_guid;

    struct capability node_caps[] = {
        { .name = "ml", .version = host->system_info->ml_capable, .enabled = host->system_info->ml_enabled },
        { .name = "mc", .version = host->system_info->mc_version ? host->system_info->mc_version : 0, .enabled = host->system_info->mc_version ? 1 : 0 },
        { .name = NULL, .version = 0, .enabled = 0 }
    };
    node_info.node_capabilities = node_caps;

    node_info.data.ml_info.ml_capable = host->system_info->ml_capable;
    node_info.data.ml_info.ml_enabled = host->system_info->ml_enabled;

    node_info.data.host_labels_ptr = host->rrdlabels;

    aclk_update_node_info(&node_info);
    log_access("ACLK RES [%s (%s)]: NODE INFO SENT for guid [%s] (%s)", wc->node_id, rrdhost_hostname(wc->host), host->machine_guid, wc->host == localhost ? "parent" : "child");

    rrd_unlock();
    freez(node_info.claim_id);
    freez(node_info.node_instance_capabilities);
    freez(host_version);

    wc->node_collectors_send = now_realtime_sec();
    freez(node_id);

}


void aclk_check_node_info_and_collectors(void)
{
    RRDHOST *host;

    if (unlikely(!aclk_connected))
        return;

    size_t pending = 0;
    dfe_start_reentrant(rrdhost_root_index, host) {

        struct aclk_sync_host_config *wc = host->aclk_sync_host_config;
        if (unlikely(!wc))
            continue;

        if (unlikely(rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD))) {
            internal_error(true, "ACLK SYNC: Context still pending for %s", rrdhost_hostname(host));
            pending++;
            continue;
        }

        if (wc->node_info_send_time && wc->node_info_send_time + 30 < now_realtime_sec()) {
            wc->node_info_send_time = 0;
            build_node_info(strdupz(wc->node_id));
            internal_error(true, "ACLK SYNC: Sending node info for %s", rrdhost_hostname(host));
        }

        if (wc->node_collectors_send && wc->node_collectors_send + 30 < now_realtime_sec()) {
            build_node_collectors(strdupz(wc->node_id));
            internal_error(true, "ACLK SYNC: Sending collectors for %s", rrdhost_hostname(host));
            wc->node_collectors_send = 0;
        }
    }
    dfe_done(host);

    if(pending)
        info("ACLK: %zu nodes are pending for contexts to load, skipped sending node info for them", pending);
}

#endif

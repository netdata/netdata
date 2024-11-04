// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_aclk_node.h"

#include "../../aclk/aclk_contexts_api.h"
#include "../../aclk/aclk_capas.h"

DICTIONARY *collectors_from_charts(RRDHOST *host, DICTIONARY *dict) {
    RRDSET *st;
    char name[500];

    rrdset_foreach_read(st, host)
    {
        if (rrdset_is_available_for_viewers(st)) {
            struct collector_info col = {.plugin = rrdset_plugin_name(st), .module = rrdset_module_name(st)};
            snprintfz(name, sizeof(name) - 1, "%s:%s", col.plugin, col.module);
            dictionary_set(dict, name, &col, sizeof(struct collector_info));
        }
    }
    rrdset_foreach_done(st);

    return dict;
}

static void build_node_collectors(RRDHOST *host)
{
    struct aclk_sync_cfg_t *wc = host->aclk_config;

    struct update_node_collectors upd_node_collectors;
    DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED);

    CLAIM_ID claim_id = claim_id_get();
    upd_node_collectors.node_id = wc->node_id;
    upd_node_collectors.claim_id = claim_id_is_set(claim_id) ? claim_id.str : NULL;

    upd_node_collectors.node_collectors = collectors_from_charts(host, dict);
    aclk_update_node_collectors(&upd_node_collectors);

    dictionary_destroy(dict);

    nd_log(NDLS_ACCESS, NDLP_DEBUG,
           "ACLK RES [%s (%s)]: NODE COLLECTORS SENT",
           wc->node_id, rrdhost_hostname(host));
}

static void build_node_info(RRDHOST *host)
{
    struct update_node_info node_info;

    struct aclk_sync_cfg_t *wc = host->aclk_config;

    CLAIM_ID claim_id = claim_id_get();

    rrd_rdlock();
    node_info.node_id = wc->node_id;
    node_info.claim_id = claim_id_is_set(claim_id) ? claim_id.str : NULL;
    node_info.machine_guid = host->machine_guid;
    node_info.child = (host != localhost);
    node_info.ml_info.ml_capable = ml_capable();
    node_info.ml_info.ml_enabled = ml_enabled(host);

    node_info.node_instance_capabilities = aclk_get_node_instance_capas(host);

    now_realtime_timeval(&node_info.updated_at);

    char *host_version = NULL;
    if (host != localhost) {
        spinlock_lock(&host->receiver_lock);
        host_version = strdupz(
            host->receiver && host->receiver->program_version ? host->receiver->program_version :
                                                                rrdhost_program_version(host));
        spinlock_unlock(&host->receiver_lock);
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
    node_info.data.version = host_version ? host_version : NETDATA_VERSION;
    node_info.data.release_channel = get_release_channel();
    node_info.data.timezone = rrdhost_abbrev_timezone(host);
    node_info.data.virtualization_type = host->system_info->virtualization ? host->system_info->virtualization : "unknown";
    node_info.data.container_type = host->system_info->container ? host->system_info->container : "unknown";
    node_info.data.custom_info = config_get(CONFIG_SECTION_WEB, "custom dashboard_info.js", "");
    node_info.data.machine_guid = host->machine_guid;

    node_info.node_capabilities = (struct capability *)aclk_get_agent_capas();

    node_info.data.ml_info.ml_capable = host->system_info->ml_capable;
    node_info.data.ml_info.ml_enabled = host->system_info->ml_enabled;

    node_info.data.host_labels_ptr = host->rrdlabels;

    aclk_update_node_info(&node_info);
    nd_log(
        NDLS_ACCESS,
        NDLP_DEBUG,
        "ACLK RES [%s (%s)]: NODE INFO SENT for guid [%s] (%s)",
        wc->node_id,
        rrdhost_hostname(host),
        host->machine_guid,
        host == localhost ? "parent" : "child");

    rrd_rdunlock();
    freez(node_info.node_instance_capabilities);
    freez(host_version);

    wc->node_collectors_send = now_realtime_sec();
}

static bool host_is_replicating(RRDHOST *host)
{
    bool replicating = false;
    RRDSET *st;
    rrdset_foreach_reentrant(st, host) {
        if (rrdset_is_replicating(st)) {
            replicating = true;
            break;
        }
    }
    rrdset_foreach_done(st);
    return replicating;
}

void aclk_check_node_info_and_collectors(void)
{
    RRDHOST *host;

    if (unlikely(!aclk_online_for_nodes()))
        return;

    size_t context_loading = 0;
    size_t replicating = 0;
    size_t context_pp = 0;

    time_t now = now_realtime_sec();
    dfe_start_reentrant(rrdhost_root_index, host)
    {
        struct aclk_sync_cfg_t *wc = host->aclk_config;
        if (unlikely(!wc))
            continue;

        if (unlikely(rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD))) {
            internal_error(true, "ACLK SYNC: Context still pending for %s", rrdhost_hostname(host));
            context_loading++;
            continue;
        }

        if (!wc->node_info_send_time && !wc->node_collectors_send)
            continue;

        if (unlikely(host_is_replicating(host))) {
            internal_error(true, "ACLK SYNC: Host %s is still replicating", rrdhost_hostname(host));
            replicating++;
            continue;
        }

        bool pp_queue_empty = !(host->rrdctx.pp_queue && dictionary_entries(host->rrdctx.pp_queue));

        if (!pp_queue_empty && (wc->node_info_send_time || wc->node_collectors_send))
            context_pp++;

        if (pp_queue_empty && wc->node_info_send_time && wc->node_info_send_time + 30 < now) {
            wc->node_info_send_time = 0;
            build_node_info(host);
            schedule_node_state_update(host, 10000);
            internal_error(true, "ACLK SYNC: Sending node info for %s", rrdhost_hostname(host));
        }

        if (pp_queue_empty && wc->node_collectors_send && wc->node_collectors_send + 30 < now) {
            build_node_collectors(host);
            internal_error(true, "ACLK SYNC: Sending collectors for %s", rrdhost_hostname(host));
            wc->node_collectors_send = 0;
        }
    }
    dfe_done(host);

    if (context_loading || replicating || context_pp) {
        nd_log_limit_static_thread_var(erl, 10, 100 * USEC_PER_MS);
        nd_log_limit(
            &erl,
            NDLS_DAEMON,
            NDLP_INFO,
            "%zu nodes loading contexts, %zu replicating data, %zu pending context post processing",
            context_loading,
            replicating,
            context_pp);
    }
}

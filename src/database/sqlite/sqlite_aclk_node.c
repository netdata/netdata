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
    struct aclk_sync_cfg_t *aclk_host_config = host->aclk_host_config;

    struct update_node_collectors upd_node_collectors;
    DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED);

    CLAIM_ID claim_id = claim_id_get();
    upd_node_collectors.node_id = aclk_host_config->node_id;
    upd_node_collectors.claim_id = claim_id_is_set(claim_id) ? claim_id.str : NULL;

    upd_node_collectors.node_collectors = collectors_from_charts(host, dict);
    aclk_update_node_collectors(&upd_node_collectors);

    dictionary_destroy(dict);

    nd_log(NDLS_ACCESS, NDLP_DEBUG,
           "ACLK RES [%s (%s)]: NODE COLLECTORS SENT",
        aclk_host_config->node_id, rrdhost_hostname(host));
}

static void build_node_info(RRDHOST *host)
{
    struct update_node_info node_info;

    struct aclk_sync_cfg_t *aclk_host_config = host->aclk_host_config;

    CLAIM_ID claim_id = claim_id_get();

    rrd_rdlock();
    node_info.node_id = aclk_host_config->node_id;
    node_info.claim_id = claim_id_is_set(claim_id) ? claim_id.str : NULL;
    node_info.machine_guid = host->machine_guid;
    node_info.child = (host != localhost);
    node_info.ml_info.ml_capable = ml_capable();
    node_info.ml_info.ml_enabled = ml_enabled(host);

    node_info.node_instance_capabilities = aclk_get_node_instance_capas(host);

    now_realtime_timeval(&node_info.updated_at);

    char *host_version = NULL;
    bool is_virtual_host = (rrdhost_option_check(host, RRDHOST_OPTION_VIRTUAL_HOST) || IS_VIRTUAL_HOST_OS(host));

    if (host != localhost && !is_virtual_host)
        host_version = stream_receiver_program_version_strdupz(host);

    node_info.data.name = rrdhost_hostname(host);
    node_info.data.os = rrdhost_os(host);
    node_info.data.version = host_version ? host_version : NETDATA_VERSION;
    node_info.data.release_channel = get_release_channel();
    node_info.data.timezone = rrdhost_abbrev_timezone(host);
    node_info.data.custom_info = inicfg_get(&netdata_config, CONFIG_SECTION_WEB, "custom dashboard_info.js", "");
    node_info.data.machine_guid = host->machine_guid;
    node_info.node_capabilities = (struct capability *)aclk_get_agent_capas();
    node_info.data.host_labels_ptr = host->rrdlabels;

    rrdhost_system_info_to_node_info(host->system_info, &node_info);

    aclk_update_node_info(&node_info);
    nd_log(
        NDLS_ACCESS,
        NDLP_DEBUG,
        "ACLK RES [%s (%s)]: NODE INFO SENT for guid [%s] (%s)",
        aclk_host_config->node_id,
        rrdhost_hostname(host),
        host->machine_guid,
        host == localhost ? "parent" : "child");

    rrd_rdunlock();
    freez(node_info.node_instance_capabilities);
    freez(host_version);

    aclk_host_config->node_collectors_send = now_realtime_sec();
}

void aclk_check_node_info_and_collectors(void)
{
    RRDHOST *host;

    if (unlikely(!aclk_online_for_nodes()))
        return;

    size_t context_loading = 0;
    size_t replicating_rcv = 0;
    size_t replicating_snd = 0;
    size_t context_pp = 0;

    STRING *context_loading_host = NULL;
    STRING *replicating_rcv_host = NULL;
    STRING *replicating_snd_host = NULL;
    STRING *context_pp_host = NULL;

#ifdef REPLICATION_TRACKING
    struct replay_who_counters replay_counters = { 0 };
#endif

    time_t now = now_realtime_sec();
    dfe_start_reentrant(rrdhost_root_index, host)
    {
        struct aclk_sync_cfg_t *aclk_host_config = host->aclk_host_config;
        if (unlikely(!aclk_host_config))
            continue;

        if (unlikely(rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD))) {
            internal_error(true, "ACLK SYNC: Context still pending for %s", rrdhost_hostname(host));
            context_loading++;
            context_loading_host = host->hostname;
            continue;
        }

        if (!aclk_host_config->node_info_send_time && !aclk_host_config->node_collectors_send)
            continue;

        if (unlikely(rrdhost_receiver_replicating_charts(host))) {
            internal_error(true, "ACLK SYNC: Host %s is still replicating in", rrdhost_hostname(host));
            replicating_rcv++;
            replicating_rcv_host = host->hostname;
        }

        if (unlikely(rrdhost_sender_replicating_charts(host))) {
            internal_error(true, "ACLK SYNC: Host %s is still replicating out", rrdhost_hostname(host));
            replicating_snd++;
            replicating_snd_host = host->hostname;
        }

#ifdef REPLICATION_TRACKING
        replication_tracking_counters(host, &replay_counters);
#endif

        if(replicating_rcv)
            continue;

        bool pp_queue_empty = !rrdcontext_queue_entries(&host->rrdctx.pp_queue);

        if (!pp_queue_empty && (aclk_host_config->node_info_send_time || aclk_host_config->node_collectors_send)) {
            context_pp++;
            context_pp_host = host->hostname;
        }

        if (pp_queue_empty && aclk_host_config->node_info_send_time &&
            aclk_host_config->node_info_send_time + 30 < now) {
            aclk_host_config->node_info_send_time = 0;
            build_node_info(host);
            schedule_node_state_update(host, 10000);
            internal_error(true, "ACLK SYNC: Sending node info for %s", rrdhost_hostname(host));
        }

        if (pp_queue_empty && aclk_host_config->node_collectors_send &&
            aclk_host_config->node_collectors_send + 30 < now) {
            build_node_collectors(host);
            internal_error(true, "ACLK SYNC: Sending collectors for %s", rrdhost_hostname(host));
            aclk_host_config->node_collectors_send = 0;
        }
    }
    dfe_done(host);

    if (context_loading || replicating_rcv || replicating_snd || context_pp) {
#ifdef REPLICATION_TRACKING
        char replay_counters_txt[1024];
        snprintfz(replay_counters_txt, sizeof(replay_counters_txt),
            " - REPLAY WHO RCV { %zu unknown, %zu me, %zu them, %zu finished } - "
            "REPLAY WHO SND { %zu unknown, %zu me, %zu them, %zu finished }",
                  replay_counters.rcv[REPLAY_WHO_UNKNOWN], replay_counters.rcv[REPLAY_WHO_ME], replay_counters.rcv[REPLAY_WHO_THEM], replay_counters.rcv[REPLAY_WHO_FINISHED],
                  replay_counters.snd[REPLAY_WHO_UNKNOWN], replay_counters.snd[REPLAY_WHO_ME], replay_counters.snd[REPLAY_WHO_THEM], replay_counters.snd[REPLAY_WHO_FINISHED]
        );
#else
        char *replay_counters_txt = "";
#endif

        const char *context_loading_pre = "", *context_loading_body = "", *context_loading_post = "";
        if(context_loading == 1) {
            context_loading_pre = " (host '";
            context_loading_body = string2str(context_loading_host);
            context_loading_post = "')";
        }

        const char *replicating_rcv_pre = "", *replicating_rcv_body = "", *replicating_rcv_post = "";
        if(replicating_rcv == 1) {
            replicating_rcv_pre = " (host '";
            replicating_rcv_body = string2str(replicating_rcv_host);
            replicating_rcv_post = "')";
        }

        const char *replicating_snd_pre = "", *replicating_snd_body = "", *replicating_snd_post = "";
        if(replicating_snd == 1) {
            replicating_snd_pre = " (host '";
            replicating_snd_body = string2str(replicating_snd_host);
            replicating_snd_post = "')";
        }

        const char *context_pp_pre = "", *context_pp_body = "", *context_pp_post = "";
        if(context_pp == 1) {
            context_pp_pre = " (host '";
            context_pp_body = string2str(context_pp_host);
            context_pp_post = "')";
        }

        nd_log_limit_static_global_var(erl, 10, 100 * USEC_PER_MS);
        nd_log_limit(&erl, NDLS_DAEMON, NDLP_INFO,
            "NODES INFO: %zu nodes loading contexts%s%s%s, %zu receiving replication%s%s%s, %zu sending replication%s%s%s, %zu pending context post processing%s%s%s%s",
                     context_loading, context_loading_pre, context_loading_body, context_loading_post,
                     replicating_rcv, replicating_rcv_pre, replicating_rcv_body, replicating_rcv_post,
                     replicating_snd, replicating_snd_pre, replicating_snd_body, replicating_snd_post,
                     context_pp, context_pp_pre, context_pp_body, context_pp_post,
                     replay_counters_txt
                     );
    }
}

// SPDX-License-Identifier: GPL-3.0-or-later

#include "sqlite_functions.h"
#include "sqlite_aclk_node.h"

#include "../../aclk/aclk_contexts_api.h"
#include "../../aclk/aclk_capas.h"
#include "../../aclk/aclk_query_queue.h"

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
    struct aclk_sync_cfg_t *aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE);

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

// Copies aclk_host_config->node_id into dst and reports whether it is usable.
//
// The copy is not optional. set_host_node_id() fills that buffer with a plain 37-byte
// uuid_unparse_lower() and no release fence, so validating the shared buffer and then transmitting
// it would validate different bytes than it sends. Callers validate once and send the snapshot.
//
// Usable means it parses AND is not the all-zero UUID: zero means the cloud has not registered this
// node, and UpdateNodeInstanceManifest carries no machine_guid to fall back on, so such a message is
// unattributable. Note node_id[0] alone is not evidence of a usable id - for the zero UUID it is '0'.
static bool aclk_node_id_snapshot(const struct aclk_sync_cfg_t *aclk_host_config, char dst[UUID_STR_LEN])
{
    memcpy(dst, aclk_host_config->node_id, UUID_STR_LEN);
    dst[UUID_STR_LEN - 1] = '\0';

    nd_uuid_t parsed;
    return uuid_parse(dst, parsed) == 0 && !uuid_is_null(parsed);
}

// node_id is the caller's validated snapshot, not the shared buffer - see aclk_node_id_snapshot().
static void build_node_manifest(RRDHOST *host, char *node_id)
{
    struct update_node_instance_manifest manifest;

    CLAIM_ID claim_id = claim_id_get();
    manifest.node_id = node_id;
    manifest.claim_id = claim_id_is_set(claim_id) ? claim_id.str : NULL;
    now_realtime_timeval(&manifest.updated_at);

    // rrd_rdlock() for the same reason build_node_info() takes it: an archived host keeps its
    // aclk config, so it is still reached by this loop, and rrdhost_cleanup_data_collection_and_health()
    // destroys host->functions. This excludes the orphan-cleanup path, which does that under
    // rrd_wrlock() (service.c). It does NOT cover
    // rrdhost_free___consume_metadata_lifetime_writelock(), which drops rrd_wrlock() before
    // rrdhost_free_unlinked() - that exposure is pre-existing and identical for build_node_info()
    // and build_node_collectors(), which dereference the host in this same loop.
    rrd_rdlock();
    manifest.functions = host_functions_to_manifest_dict(host);
    rrd_rdunlock();

    aclk_update_node_instance_manifest(&manifest);

    dictionary_destroy(manifest.functions);

    nd_log(NDLS_ACCESS, NDLP_DEBUG,
           "ACLK RES [%s (%s)]: NODE MANIFEST SENT",
        node_id, rrdhost_hostname(host));
}

static void build_node_info(RRDHOST *host, struct aclk_sync_completion *sync_completion)
{
    struct update_node_info node_info;

    struct aclk_sync_cfg_t *aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE);

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
    bool is_virtual_host = (rrdhost_flag_check(host, RRDHOST_FLAG_VIRTUAL_HOST) || IS_VIRTUAL_HOST_OS(host));

    if (host != localhost && !is_virtual_host)
        host_version = stream_receiver_program_version_strdupz(host);

    RRDHOST_TZ host_tz = rrdhost_tz_get(host);
    RRDHOST_METADATA_IDENTITY identity = rrdhost_metadata_identity_acquire(host);

    node_info.data.name = string2str(identity.common.hostname);
    node_info.data.os = string2str(identity.os);
    node_info.data.version = host_version ? host_version : NETDATA_VERSION;
    node_info.data.release_channel = get_release_channel();
    node_info.data.timezone = host_tz.abbrev_timezone;
    node_info.data.custom_info = inicfg_get(&netdata_config, CONFIG_SECTION_WEB, "custom dashboard_info.js", "");
    node_info.data.machine_guid = host->machine_guid;
    node_info.node_capabilities = (struct capability *)aclk_get_agent_capas();
    node_info.data.host_labels_ptr = host->rrdlabels;

    spinlock_lock(&host->rrdhost_update_lock);
    struct rrdhost_system_info *system_info = rrdhost_system_info_dup(host->system_info);
    spinlock_unlock(&host->rrdhost_update_lock);
    rrdhost_system_info_to_node_info(system_info, &node_info);

    aclk_update_node_info(&node_info, sync_completion);
    rrdhost_system_info_free(system_info);
    nd_log(
        NDLS_ACCESS,
        NDLP_DEBUG,
        "ACLK RES [%s (%s)]: NODE INFO SENT for guid [%s] (%s)",
        aclk_host_config->node_id,
        string2str(identity.common.hostname),
        host->machine_guid,
        host == localhost ? "parent" : "child");

    rrdhost_metadata_identity_release(&identity);
    rrd_rdunlock();
    rrdhost_tz_free(&host_tz);
    freez(node_info.node_instance_capabilities);
    freez(host_version);

    aclk_send_timestamp_set(&aclk_host_config->node_collectors_send, now_realtime_sec());

    // Every node info send also re-arms the function manifest, the same way it arms the
    // collectors above. Arming (not setting) keeps an older pending deadline intact.
    aclk_send_timestamp_arm(&aclk_host_config->node_manifest_send_time, now_realtime_sec());
}

void send_node_info_with_wait(RRDHOST *host)
{
    if (unlikely(!host || !__atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE)))
        return;

    // No node_id means cloud doesn't know about this node - nothing to update
    if (uuid_is_null(host->node_id.uuid))
        return;

    if (!aclk_online())
        return;

    struct aclk_sync_completion *sc = aclk_sync_completion_create();

    build_node_info(host, sc);

    bool success = aclk_sync_completion_timedwait(sc, 30);
    if (!success) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "Timed out waiting for node info update for host '%s'",
               rrdhost_hostname(host));
    }
    // sc is automatically freed when both waiter and query release their references
}

void send_node_update_with_wait(RRDHOST *host, int live, int queryable)
{
    if (unlikely(!host || !__atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE)))
        return;

    // No node_id means cloud doesn't know about this node - nothing to update
    if (uuid_is_null(host->node_id.uuid))
        return;

    if (!aclk_online())
        return;

    struct aclk_sync_completion *sc = aclk_sync_completion_create();

    aclk_host_state_update(host, live, queryable, sc);

    bool success = aclk_sync_completion_timedwait(sc, 30);
    if (!success) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "Timed out waiting for node state update for host '%s'",
               rrdhost_hostname(host));
    }
    // sc is automatically freed when both waiter and query release their references
}

static inline void hostname_snapshot_update(STRING **snapshot, RRDHOST *host)
{
    spinlock_lock(&host->rrdhost_update_lock);
    STRING *hostname = string_dup(host->hostname);
    spinlock_unlock(&host->rrdhost_update_lock);

    string_freez(*snapshot);
    *snapshot = hostname;
}

void aclk_check_node_info_collectors_and_manifest(void)
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

    // The node manifest is optional: older clouds do not advertise its topic. Checked once
    // here, before anything claims a pending send, so a pending manifest survives until a
    // cloud that supports it is connected.
    bool manifest_topic_available = aclk_topic_available(ACLK_TOPICID_NODE_MANIFEST);

    time_t now = now_realtime_sec();
    dfe_start_reentrant(rrdhost_root_index, host)
    {
        struct aclk_sync_cfg_t *aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE);
        if (unlikely(!aclk_host_config))
            continue;

        if (unlikely(rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD))) {
            internal_error(true, "ACLK SYNC: Context still pending for %s", rrdhost_hostname(host));
            context_loading++;
            hostname_snapshot_update(&context_loading_host, host);
            continue;
        }

        time_t node_info_send_time = aclk_send_timestamp_get(&aclk_host_config->node_info_send_time);
        time_t node_collectors_send = aclk_send_timestamp_get(&aclk_host_config->node_collectors_send);
        time_t node_manifest_send_time = aclk_send_timestamp_get(&aclk_host_config->node_manifest_send_time);

        // Whether a pending manifest can actually be sent. Unlike the other two send times, this
        // one can be blocked by a condition that never clears (an old cloud that does not advertise
        // the topic, a node the cloud never registered), so it must NOT count as pending for the
        // early exit below - otherwise those hosts stay on the per-second path forever, redoing the
        // replication and context checks and re-emitting the NODES INFO line. The request itself is
        // deliberately left armed, so it goes out as soon as the condition clears.
        //
        // The node_id checked is the one build_node_manifest() will send, snapshotted here so the
        // validated bytes are the transmitted bytes. Do NOT test host->node_id.uuid instead: a child
        // can inherit its node_id from its parent (stream_sender_get_node_and_claim_id_from_parent()
        // assigns host->node_id directly) without it ever reaching the aclk config.
        //
        // An unusable node_id must leave manifest_pending false rather than being rejected later in
        // build_node_manifest(): the request stays armed either way, but reporting it as pending here
        // would keep this host on the per-second path indefinitely, which is exactly what the
        // early exit below exists to avoid.
        char manifest_node_id[UUID_STR_LEN];
        bool manifest_pending = node_manifest_send_time && manifest_topic_available &&
                                aclk_node_id_snapshot(aclk_host_config, manifest_node_id);

        if (!node_info_send_time && !node_collectors_send && !manifest_pending)
            continue;

        // read once: this gates THIS host below, while replicating_rcv accumulates across
        // all hosts for the summary log at the end of the function
        bool host_replicating_rcv = rrdhost_receiver_replicating_charts(host) != 0;
        if (unlikely(host_replicating_rcv)) {
            internal_error(true, "ACLK SYNC: Host %s is still replicating in", rrdhost_hostname(host));
            replicating_rcv++;
            hostname_snapshot_update(&replicating_rcv_host, host);
        }

        if (unlikely(rrdhost_sender_replicating_charts(host))) {
            internal_error(true, "ACLK SYNC: Host %s is still replicating out", rrdhost_hostname(host));
            replicating_snd++;
            hostname_snapshot_update(&replicating_snd_host, host);
        }

#ifdef REPLICATION_TRACKING
        replication_tracking_counters(host, &replay_counters);
#endif

        // defer this host only: a host replicating in has an incomplete chart/function set,
        // so anything built from it now would be wrong. Testing the fleet-wide replicating_rcv
        // counter here would defer every host that follows a replicating one in index order.
        if(host_replicating_rcv)
            continue;

        bool pp_queue_empty = !rrdcontext_queue_entries(&host->rrdctx.pp_queue);

        if (!pp_queue_empty && (node_info_send_time || node_collectors_send || manifest_pending)) {
            context_pp++;
            hostname_snapshot_update(&context_pp_host, host);
        }

        node_info_send_time = aclk_send_timestamp_get(&aclk_host_config->node_info_send_time);
        if (pp_queue_empty && node_info_send_time &&
            nd_time_t_add_compare(node_info_send_time, 30, now) < 0 &&
            aclk_send_timestamp_claim(&aclk_host_config->node_info_send_time, node_info_send_time)) {
            build_node_info(host, NULL);
            schedule_node_state_update(host, 10000);
            internal_error(true, "ACLK SYNC: Sending node info for %s", rrdhost_hostname(host));
        }

        node_collectors_send = aclk_send_timestamp_get(&aclk_host_config->node_collectors_send);
        if (pp_queue_empty && node_collectors_send &&
            nd_time_t_add_compare(node_collectors_send, 30, now) < 0 &&
            aclk_send_timestamp_claim(&aclk_host_config->node_collectors_send, node_collectors_send)) {
            build_node_collectors(host);
            internal_error(true, "ACLK SYNC: Sending collectors for %s", rrdhost_hostname(host));
        }

        node_manifest_send_time = aclk_send_timestamp_get(&aclk_host_config->node_manifest_send_time);
        if (manifest_pending && pp_queue_empty && node_manifest_send_time &&
            nd_time_t_add_compare(node_manifest_send_time, 30, now) < 0 &&
            aclk_send_timestamp_claim(&aclk_host_config->node_manifest_send_time, node_manifest_send_time)) {
            build_node_manifest(host, manifest_node_id);
            internal_error(true, "ACLK SYNC: Sending node manifest for %s", rrdhost_hostname(host));
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

    string_freez(context_loading_host);
    string_freez(replicating_rcv_host);
    string_freez(replicating_snd_host);
    string_freez(context_pp_host);
}

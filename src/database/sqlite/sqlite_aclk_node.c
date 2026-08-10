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
    char node_id[UUID_STR_LEN];
    aclk_node_id_copy(aclk_host_config, node_id);
    upd_node_collectors.node_id = node_id;
    upd_node_collectors.claim_id = claim_id_is_set(claim_id) ? claim_id.str : NULL;

    upd_node_collectors.node_collectors = collectors_from_charts(host, dict);
    aclk_update_node_collectors(&upd_node_collectors);

    dictionary_destroy(dict);

    nd_log(NDLS_ACCESS, NDLP_DEBUG,
           "ACLK RES [%s (%s)]: NODE COLLECTORS SENT",
        node_id, rrdhost_hostname(host));
}

// Usable means it parses AND is not the all-zero UUID: zero means the cloud has not registered this
// node, and UpdateNodeInstanceManifest carries no machine_guid to fall back on, so such a message is
// unattributable. Note node_id[0] alone is not evidence of a usable id - for the zero UUID it is '0'.
static bool aclk_node_id_snapshot(aclk_sync_cfg_t *aclk_host_config, char dst[UUID_STR_LEN])
{
    aclk_node_id_copy(aclk_host_config, dst);

    nd_uuid_t parsed;
    return uuid_parse(dst, parsed) == 0 && !uuid_is_null(parsed);
}

// Hands out the per-enqueue token that identifies one manifest publication. Monotonic and never
// reused, so it identifies an ENQUEUE, which the content key cannot: identical content produces an
// identical key, and content that is published, changed, and changed back records the same key twice.
// Never returns 0, which node_manifest_sent_token uses to mean "nothing outstanding".
static uint64_t manifest_publication_token_next(void)
{
    static uint64_t seq = 0;
    return __atomic_add_fetch(&seq, 1, __ATOMIC_RELAXED);
}

// Undoes the record build_node_manifest() made for a manifest that never went out. Runs under the
// rrd read lock - see rrdhost_apply_by_machine_guid() - so everything it does is one atomic CAS and
// one atomic arm, which is all it may do while that lock is held.
static void manifest_publication_apply(RRDHOST *host, void *data)
{
    const struct aclk_manifest_publication *publication = data;

    aclk_sync_cfg_t *aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE);
    if (!aclk_host_config)
        return;

    // Invalidate the record only while it is still THIS enqueue's, which is what the token is for:
    // the CAS fails if a later manifest has been enqueued since, whatever its content, so publications
    // for one host can be in flight together and complete in any order without a later one losing its
    // record to an earlier drop - including when the later one carries identical content and therefore
    // an identical key.
    //
    // Clearing the token, not the key, is what makes this a single atomic operation. The key stays as
    // it was; a 0 token is what stops it from suppressing anything (see build_node_manifest()), so the
    // pair never has to be written together and cannot be observed half-updated.
    uint64_t expected = publication->token;
    if (!__atomic_compare_exchange_n(
            &aclk_host_config->node_manifest_sent_token, &expected, 0, false, __ATOMIC_RELEASE, __ATOMIC_RELAXED))
        return; // a later manifest owns the record - it is the one the cloud needs, and it will report itself

    // Re-arm with now, not with the deadline this send was claimed from: a failure that repeats for
    // the same payload (a manifest the server refuses as too big) would otherwise come due on every
    // pass and take a pacer slot each time, while arming with now retries one coalescing window
    // later. aclk_send_timestamp_arm() only ever lowers a pending deadline, so a request armed
    // meanwhile keeps its earlier one.
    aclk_send_timestamp_arm(&aclk_host_config->node_manifest_send_time, now_realtime_sec());
}

// Applies the outcome of a publication. Runs on the ACLK query worker that executed the send, or on
// whichever thread dropped the query before it got that far.
//
// Only a DROP does anything. A publication that reached the mqtt layer is already accounted for: the
// key was recorded when the manifest was enqueued, which is what the suppression in
// build_node_manifest() must compare against. Recording it here instead would compare builds against
// the last CONFIRMED manifest rather than the last SENT one, and those differ for as long as a
// publication is in flight - long enough for a content revert inside that window to be suppressed
// against a key the in-flight message is about to replace, leaving the cloud with content the node no
// longer has and nothing armed to correct it.
//
// The host is resolved rather than carried as a pointer, because the host and its config can be
// freed between the enqueue and the outcome; and it is resolved through
// rrdhost_apply_by_machine_guid() rather than any find-then-dereference helper, because only
// the former holds the host allocated while the callback runs.
//
// Whether it may wait for the read lock depends on the thread, so it is decided per call. Every
// mqtt-level drop - an offline or disconnecting client, a full mqtt buffer, a payload the server
// refuses - is reported from an ACLK query worker, and a payload that fails to generate is reported
// from the alert-push worker; both may block, so for them the clear below always happens. Only the
// event loop must not wait: a host teardown can be blocked on it inside destroy_aclk_config() while
// holding rrd_wrlock(), so waiting there would wedge the agent. It reaches this path when it drops a
// query it could not park (a Judy allocation failure) or when it drains queries at shutdown.
//
// On the event loop, then, a lost race leaves the dropped manifest's key recorded as if it had been
// sent, so that content is not re-sent until it changes again or the ACLK session does. That is the
// original defect of this SOW, reduced to: an allocation failure that drops a manifest, on the one
// thread that cannot wait, while a teardown happens to hold rrd_wrlock(). At shutdown it is moot -
// the next start is a new session, and every key carries the session.
//
// Resolved by machine_guid, NOT by the node_id this manifest was keyed under at the cloud, even
// though the node_id is what identifies the message. host->node_id and the ACLK config's node_id are
// allowed to disagree - command-nodeid.c assigns host->node_id from the parent without touching the
// config, and set_host_node_id() documents the divergence - so a node_id lookup can miss the host
// whose config must be written, or find a different one. Missing it would leave a dropped manifest
// recorded as sent; finding the wrong host would invalidate a record that host is still relying on -
// harmless beyond a redundant publish, since only its token can be cleared and only when the CAS
// matches. machine_guid is immutable for the host's lifetime, and is also rrdhost_root_index's key,
// so the lookup is exact.
//
// The two drop sites that run before the query is handed off - a payload that failed to generate,
// and a sync queue that is not accepting - reach this from inside
// aclk_check_node_info_collectors_and_manifest()'s walk, which holds no rrd lock in its body:
// build_node_manifest() releases rrd_rdlock() before enqueueing.
void aclk_node_manifest_publish_result(const struct aclk_manifest_publication *publication)
{
    if (publication->published)
        return;

    (void)rrdhost_apply_by_machine_guid(
        publication->machine_guid, manifest_publication_apply, (void *)publication,
        !aclk_sync_on_event_loop_thread());
}

// node_id is the caller's validated snapshot, not the shared buffer - see aclk_node_id_snapshot().
// aclk_host_config is the caller's, already loaded and non-NULL for this host.
//
// Returns whether a manifest was enqueued for publication, so the caller's per-scan budget is spent
// on messages rather than on suppressed builds - see MAX_NODE_MANIFESTS_PER_SCAN.
//
// The budget counts what this pass ENQUEUES, not what reaches the mqtt layer, because the publication
// completes asynchronously and the pass cannot wait for it. A message dropped between here and the
// mqtt layer therefore still spends a slot. That is deliberate: the alternative is reporting the
// queue-accept status back through aclk_execute_query() to every caller of QUEUE_IF_PAYLOAD_PRESENT,
// and the only paths it would recover a slot on are ACLK sync not accepting work and a payload that
// failed to generate - both of which cost at most this pass's remaining slots, and both of which
// re-arm, so the effect is under-scheduling by one window, never a lost manifest.
static bool build_node_manifest(RRDHOST *host, aclk_sync_cfg_t *aclk_host_config, char *node_id)
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

    // Suppress publishing a manifest identical to the one already published in this ACLK session.
    // Arming is event-driven and content-blind - a no-op function re-registration (plugin restart,
    // child reconnect), every node-info send and every cloud node-id reply all arm a send - so this
    // chokepoint compares content instead. The hash covers everything generate_..._message()
    // transmits, including the node_id it is keyed under at the cloud, but not updated_at.
    //
    // The comparison is against the newest manifest SENT, not the newest confirmed delivered, which
    // is why the key is recorded here rather than by the publication. A publication is asynchronous,
    // so "confirmed" lags "sent" by however long the query takes; comparing against it would let a
    // content revert inside that window be suppressed against a key the in-flight message is about to
    // replace, leaving the cloud with content the node no longer has. A message that is then dropped
    // clears this record instead (aclk_node_manifest_publish_result()), so nothing stays suppressed on
    // the strength of a manifest that never went out.
    //
    // A 0 token means nothing is outstanding for this config - it has published nothing yet, or its
    // last manifest was dropped and invalidated - so the key beside it must not suppress anything.
    //
    // Deliberately NOT suppressed: the first manifest of a config, the first manifest of every ACLK
    // session (the session is folded into the key, because a publication is still never acked), and
    // empty manifests - an empty list is meaningful to the cloud ("this node has no functions"),
    // not an absence of information.
    uint64_t key = manifest_publication_key(
        manifest_dict_hash(manifest.functions, node_id, manifest.claim_id), aclk_session_load());
    bool suppressed = __atomic_load_n(&aclk_host_config->node_manifest_sent_token, __ATOMIC_ACQUIRE) != 0 &&
                      __atomic_load_n(&aclk_host_config->node_manifest_sent_key, __ATOMIC_ACQUIRE) == key;

    if (!suppressed) {
        // Record BEFORE enqueueing, never after: a query can be dropped SYNCHRONOUSLY inside the call
        // below (a payload that fails to generate), and its invalidating CAS would then run against a
        // token this build has not stored yet - it would fail, and the store that followed would leave
        // the dropped manifest recorded as outstanding.
        //
        // Key first, then token: the token is what licenses the key to suppress, so publishing it last
        // means no reader can act on a key that is not yet in place. Both are written only here, by the
        // one serialized scan pass; the drop path only ever clears the token.
        uint64_t token = manifest_publication_token_next();
        __atomic_store_n(&aclk_host_config->node_manifest_sent_key, key, __ATOMIC_RELEASE);
        __atomic_store_n(&aclk_host_config->node_manifest_sent_token, token, __ATOMIC_RELEASE);
        aclk_update_node_instance_manifest(&manifest, host->machine_guid, key, token);
    }

    dictionary_destroy(manifest.functions);

    nd_log(NDLS_ACCESS, NDLP_DEBUG,
           "ACLK RES [%s (%s)]: NODE MANIFEST %s",
        node_id, rrdhost_hostname(host), suppressed ? "SUPPRESSED (unchanged)" : "SENT");

    return !suppressed;
}

static void build_node_info(RRDHOST *host, struct aclk_sync_completion *sync_completion)
{
    struct update_node_info node_info;

    struct aclk_sync_cfg_t *aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE);

    CLAIM_ID claim_id = claim_id_get();
    char node_id[UUID_STR_LEN];
    aclk_node_id_copy(aclk_host_config, node_id);

    rrd_rdlock();
    node_info.node_id = node_id;
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
        node_id,
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

// The manifest pacing state (budget + earliest-deadline-first cutoff) lives in
// sqlite_aclk_node.h so it can be unit tested; see MANIFEST_PACER there for why it exists.
//
// Only the alert-push worker reaches this, serialized by alert_push_running, and libuv's threadpool
// hand-off gives each pass a happens-before edge on the previous one, so the pacer itself needs no
// synchronization. The publication record does: this worker stores both node_manifest_sent_key and
// node_manifest_sent_token, and the token alone is cleared - by whichever thread reports that a
// manifest was dropped, which is an ACLK query worker for anything the mqtt layer rejected, this
// same worker for a drop inside the enqueue call, or the ACLK sync event loop for a query it could
// not park and for the shutdown drain.
static MANIFEST_PACER manifest_pacer = { 0 };

void aclk_check_node_info_collectors_and_manifest(void)
{
    RRDHOST *host;

    if (unlikely(!aclk_online_for_nodes()))
        return;

    manifest_pacer_begin(&manifest_pacer);

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
        char manifest_node_id[UUID_STR_LEN] = "";
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
        bool manifest_due = manifest_pending && pp_queue_empty && node_manifest_send_time &&
                            nd_time_t_add_compare(node_manifest_send_time, NODE_MANIFEST_WINDOW_S, now) < 0;

        // The budget and the cutoff are both tested before the claim, so a host denied a slot keeps
        // its armed request instead of having it consumed and dropped. The budget is charged only
        // when a message was really enqueued, so a run of suppressed (unchanged) manifests cannot
        // exhaust it for the hosts behind them - but a suppressed host still consumed its claim, so
        // it counts as served and is not carried into the cutoff below. A manifest dropped after
        // this point still spent its slot: the budget paces what this pass hands to the mqtt layer,
        // and the retry comes back through a re-arm on a later pass.
        bool manifest_served = false;
        if (manifest_due && manifest_pacer_admit(&manifest_pacer, node_manifest_send_time) &&
            aclk_send_timestamp_claim(&aclk_host_config->node_manifest_send_time, node_manifest_send_time)) {
            manifest_served = true;

            if (build_node_manifest(host, aclk_host_config, manifest_node_id)) {
                manifest_pacer_published(&manifest_pacer);
                internal_error(true, "ACLK SYNC: Sending node manifest for %s", rrdhost_hostname(host));
            }
        }

        if (manifest_due && !manifest_served)
            manifest_pacer_defer(&manifest_pacer, node_manifest_send_time);
    }
    dfe_done(host);

    manifest_pacer_end(&manifest_pacer);

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

// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdcontext-internal.h"
#include "../sqlite/sqlite_aclk.h"

// ----------------------------------------------------------------------------
// visualizing flags

struct rrdcontext_reason rrdcontext_reasons[] = {
        // context related
        {RRD_FLAG_UPDATE_REASON_TRIGGERED,               "triggered transition", 65 * USEC_PER_SEC },
        {RRD_FLAG_UPDATE_REASON_NEW_OBJECT,              "object created",       65 * USEC_PER_SEC },
        {RRD_FLAG_UPDATE_REASON_UPDATED_OBJECT,          "object updated",       65 * USEC_PER_SEC },
        {RRD_FLAG_UPDATE_REASON_LOAD_SQL,                "loaded from sql",      65 * USEC_PER_SEC },
        {RRD_FLAG_UPDATE_REASON_CHANGED_METADATA,        "changed metadata",     65 * USEC_PER_SEC },
        {RRD_FLAG_UPDATE_REASON_ZERO_RETENTION,          "has no retention",     65 * USEC_PER_SEC },
        {RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T,    "updated first_time_t", 65 * USEC_PER_SEC },
        {RRD_FLAG_UPDATE_REASON_CHANGED_LAST_TIME_T,     "updated last_time_t",  65 * USEC_PER_SEC },
        {RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED, "stopped collected",    65 * USEC_PER_SEC },
        {RRD_FLAG_UPDATE_REASON_STARTED_BEING_COLLECTED, "started collected",    5 * USEC_PER_SEC },
        {RRD_FLAG_UPDATE_REASON_UNUSED,                  "unused",               5 * USEC_PER_SEC },

        // not context related
        {RRD_FLAG_UPDATE_REASON_CHANGED_LINKING,         "changed rrd link",     65 * USEC_PER_SEC },
        {RRD_FLAG_UPDATE_REASON_DISCONNECTED_CHILD,      "child disconnected",   65 * USEC_PER_SEC },
        {RRD_FLAG_UPDATE_REASON_DB_ROTATION,             "db rotation",          65 * USEC_PER_SEC },
        {RRD_FLAG_UPDATE_REASON_UPDATE_RETENTION,        "updated retention",    65 * USEC_PER_SEC },

        // terminator
        {0, NULL,                                                                0 },
};

void rrd_reasons_to_buffer_json_array_items(RRD_FLAGS flags, BUFFER *wb) {
    for(int i = 0, added = 0; rrdcontext_reasons[i].name ; i++) {
        if (flags & rrdcontext_reasons[i].flag) {
            buffer_json_add_array_item_string(wb, rrdcontext_reasons[i].name);
            added++;
        }
    }
}
// ----------------------------------------------------------------------------
// public API

ALWAYS_INLINE void rrdcontext_updated_rrddim(RRDDIM *rd) {
    rrdmetric_from_rrddim(rd);
}

ALWAYS_INLINE void rrdcontext_removed_rrddim(RRDDIM *rd) {
    rrdmetric_rrddim_is_freed(rd);
}

ALWAYS_INLINE void rrdcontext_updated_rrddim_algorithm(RRDDIM *rd) {
    rrdmetric_updated_rrddim_flags(rd);
}

ALWAYS_INLINE void rrdcontext_updated_rrddim_multiplier(RRDDIM *rd) {
    rrdmetric_updated_rrddim_flags(rd);
}

ALWAYS_INLINE void rrdcontext_updated_rrddim_divisor(RRDDIM *rd) {
    rrdmetric_updated_rrddim_flags(rd);
}

ALWAYS_INLINE void rrdcontext_updated_rrddim_flags(RRDDIM *rd) {
    rrdmetric_updated_rrddim_flags(rd);
}

ALWAYS_INLINE void rrdcontext_collected_rrddim(RRDDIM *rd) {
    rrdmetric_collected_rrddim(rd);
}

ALWAYS_INLINE void rrdcontext_updated_rrdset(RRDSET *st) {
    rrdinstance_from_rrdset(st);
}

ALWAYS_INLINE void rrdcontext_removed_rrdset(RRDSET *st) {
    rrdinstance_rrdset_is_freed(st);
}

ALWAYS_INLINE void rrdcontext_updated_retention_rrdset(RRDSET *st) {
    rrdinstance_rrdset_has_updated_retention(st);
}

ALWAYS_INLINE void rrdcontext_updated_rrdset_name(RRDSET *st) {
    rrdinstance_updated_rrdset_name(st);
}

ALWAYS_INLINE void rrdcontext_updated_rrdset_flags(RRDSET *st) {
    rrdinstance_updated_rrdset_flags(st);
}

ALWAYS_INLINE void rrdcontext_collected_rrdset(RRDSET *st) {
    rrdinstance_collected_rrdset(st);
}

ALWAYS_INLINE void rrdcontext_host_child_disconnected(RRDHOST *host) {
    rrdhost_flag_set(host, RRDHOST_FLAG_RRDCONTEXT_GET_RETENTION);
}

ALWAYS_INLINE void rrdcontext_host_child_connected(RRDHOST *host) {
    // clear the rrdcontexts status cache inside RRDSET and RRDDIM
    RRDSET *st;
    rrdset_foreach_read(st, host) {
        rrdinstance_rrdset_not_collected(st);
    }
    rrdset_foreach_done(st);
}

usec_t rrdcontext_next_db_rotation_ut = 0;
ALWAYS_INLINE void rrdcontext_db_rotation(void) {
    // called when the db rotates its database
    rrdcontext_next_db_rotation_ut = now_realtime_usec() + FULL_RETENTION_SCAN_DELAY_AFTER_DB_ROTATION_SECS * USEC_PER_SEC;
}

int rrdcontext_find_dimension_uuid(RRDSET *st, const char *id, nd_uuid_t *store_uuid) {
    if(!st->rrdhost) return 1;
    if(!st->context) return 2;

    RRDCONTEXT_ACQUIRED *rca = (RRDCONTEXT_ACQUIRED *)dictionary_get_and_acquire_item(st->rrdhost->rrdctx.contexts, string2str(st->context));
    if(!rca) return 3;

    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);

    RRDINSTANCE_ACQUIRED *ria = (RRDINSTANCE_ACQUIRED *)dictionary_get_and_acquire_item(rc->rrdinstances, string2str(st->id));
    if(!ria) {
        rrdcontext_release(rca);
        return 4;
    }

    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);

    RRDMETRIC_ACQUIRED *rma = (RRDMETRIC_ACQUIRED *)dictionary_get_and_acquire_item(ri->rrdmetrics, id);
    if(!rma) {
        rrdinstance_release(ria);
        rrdcontext_release(rca);
        return 5;
    }

    RRDMETRIC *rm = rrdmetric_acquired_value(rma);

    uuidmap_uuid(rm->uuid, *store_uuid);

    rrdmetric_release(rma);
    rrdinstance_release(ria);
    rrdcontext_release(rca);
    return 0;
}

int rrdcontext_find_chart_uuid(RRDSET *st, nd_uuid_t *store_uuid) {
    if(!st->rrdhost) return 1;
    if(!st->context) return 2;

    RRDCONTEXT_ACQUIRED *rca = (RRDCONTEXT_ACQUIRED *)dictionary_get_and_acquire_item(st->rrdhost->rrdctx.contexts, string2str(st->context));
    if(!rca) return 3;

    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);

    RRDINSTANCE_ACQUIRED *ria = (RRDINSTANCE_ACQUIRED *)dictionary_get_and_acquire_item(rc->rrdinstances, string2str(st->id));
    if(!ria) {
        rrdcontext_release(rca);
        return 4;
    }

    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    uuidmap_uuid(ri->uuid, *store_uuid);

    rrdinstance_release(ria);
    rrdcontext_release(rca);
    return 0;
}

int rrdcontext_foreach_instance_with_rrdset_in_context(RRDHOST *host, const char *context, int (*callback)(RRDSET *st, void *data), void *data) {
    if(unlikely(!host || !context || !*context || !callback))
        return -1;

    RRDCONTEXT_ACQUIRED *rca = (RRDCONTEXT_ACQUIRED *)dictionary_get_and_acquire_item(host->rrdctx.contexts, context);
    if(unlikely(!rca)) return -1;

    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);
    if(unlikely(!rc)) return -1;

    int ret = 0;
    RRDINSTANCE *ri;
    dfe_start_read(rc->rrdinstances, ri) {
        if(ri->rrdset) {
            int r = callback(ri->rrdset, data);
            if(r >= 0) ret += r;
            else {
                ret = r;
                break;
            }
        }
    }
    dfe_done(ri);

    rrdcontext_release(rca);

    return ret;
}

// ----------------------------------------------------------------------------
// ACLK interface

static void rrdcontext_checkpoint_clear_pending_unsafe(struct aclk_sync_cfg_t *aclk_host_config) {
    freez(aclk_host_config->pending_ctx_claim_id);
    freez(aclk_host_config->pending_ctx_node_id);
    aclk_host_config->pending_ctx_claim_id = NULL;
    aclk_host_config->pending_ctx_node_id = NULL;
    aclk_host_config->pending_ctx_version_hash = 0;
    aclk_host_config->pending_ctx_saved_monotonic_s = 0;
    __atomic_store_n(&aclk_host_config->pending_ctx_checkpoint, false, __ATOMIC_RELEASE);
}

static uint64_t rrdcontext_checkpoint_invalidate_pending(RRDHOST *host) {
    struct aclk_sync_cfg_t *aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE);
    if(!aclk_host_config)
        return 0;

    spinlock_lock(&aclk_host_config->pending_ctx_spinlock);
    uint64_t generation = __atomic_add_fetch(&aclk_host_config->pending_ctx_generation, 1, __ATOMIC_RELEASE);
    rrdcontext_checkpoint_clear_pending_unsafe(aclk_host_config);
    spinlock_unlock(&aclk_host_config->pending_ctx_spinlock);

    return generation;
}

static bool rrdcontext_checkpoint_generation_is_current(RRDHOST *host, const char *claim_id, const char *node_id, uint64_t generation) {
    if(!generation)
        return true;

    struct aclk_sync_cfg_t *aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE);
    uint64_t current_generation = aclk_host_config ?
                                  __atomic_load_n(&aclk_host_config->pending_ctx_generation, __ATOMIC_ACQUIRE) :
                                  0;

    if(likely(aclk_host_config && current_generation == generation))
        return true;

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "RRDCONTEXT: skipping stale checkpoint for host '%s', claim id '%s', node id '%s' "
           "(generation %"PRIu64", current %"PRIu64").",
           rrdhost_hostname(host), claim_id, node_id, generation, current_generation);
    return false;
}

// Save a pending checkpoint to be replayed when context processing completes.
// Returns true if saved successfully, false if save failed (caller should execute immediately).
static bool rrdcontext_checkpoint_save_pending(RRDHOST *host, struct ctxs_checkpoint *cmd) {
    struct aclk_sync_cfg_t *aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE);
    if(!aclk_host_config)
        return false;

    spinlock_lock(&aclk_host_config->pending_ctx_spinlock);

    // Pause incremental context streaming until the deferred checkpoint is replayed.
    rrdhost_flag_clear(host, RRDHOST_FLAG_ACLK_STREAM_CONTEXTS);

    __atomic_add_fetch(&aclk_host_config->pending_ctx_generation, 1, __ATOMIC_RELEASE);
    rrdcontext_checkpoint_clear_pending_unsafe(aclk_host_config);
    aclk_host_config->pending_ctx_claim_id = strdupz(cmd->claim_id);
    aclk_host_config->pending_ctx_node_id = strdupz(cmd->node_id);
    aclk_host_config->pending_ctx_version_hash = cmd->version_hash;
    aclk_host_config->pending_ctx_saved_monotonic_s = now_monotonic_sec();
    __atomic_store_n(&aclk_host_config->pending_ctx_checkpoint, true, __ATOMIC_RELEASE);
    spinlock_unlock(&aclk_host_config->pending_ctx_spinlock);
    return true;
}

// Execute the checkpoint: compare version hash, send snapshot if needed, enable streaming.
static void rrdcontext_checkpoint_execute(RRDHOST *host, const char *claim_id, const char *node_id, uint64_t version_hash, uint64_t generation) {
    if(!rrdcontext_checkpoint_generation_is_current(host, claim_id, node_id, generation))
        return;

    if(rrdhost_flag_check(host, RRDHOST_FLAG_ACLK_STREAM_CONTEXTS)) {
        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "RRDCONTEXT: checkpoint for claim id '%s', node id '%s', "
               "while node '%s' has an active context streaming.",
               claim_id, node_id, rrdhost_hostname(host));

        // disable it temporarily, so that our worker will not attempt to send messages in parallel
        rrdhost_flag_clear(host, RRDHOST_FLAG_ACLK_STREAM_CONTEXTS);
    }

    uint64_t our_version_hash = rrdcontext_version_hash(host);

    if(version_hash != our_version_hash) {
        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "RRDCONTEXT: received version hash %"PRIu64" for host '%s', does not match our version hash %"PRIu64". "
               "Sending snapshot of all contexts.",
               version_hash, rrdhost_hostname(host), our_version_hash);

        // prepare the snapshot
        char uuid_str[UUID_STR_LEN];
        uuid_unparse_lower(host->node_id.uuid, uuid_str);
        contexts_snapshot_t bundle = contexts_snapshot_new(claim_id, uuid_str, our_version_hash);

        // do a deep scan on every metric of the host to make sure all our data are updated
        rrdcontext_recalculate_host_retention(host, RRD_FLAG_NONE, false);

        // calculate version hash and pack all the messages together in one go
        our_version_hash = rrdcontext_version_hash_with_callback(host, rrdcontext_message_send_unsafe, true, bundle);

        if(!rrdcontext_checkpoint_generation_is_current(host, claim_id, node_id, generation)) {
            contexts_snapshot_delete(bundle);
            return;
        }

        // update the version
        contexts_snapshot_set_version(bundle, our_version_hash);

        // send it
        aclk_send_contexts_snapshot(bundle);
    }

    if(!rrdcontext_checkpoint_generation_is_current(host, claim_id, node_id, generation))
        return;

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "RRDCONTEXT: host '%s' enabling streaming of contexts",
           rrdhost_hostname(host));

    struct aclk_sync_cfg_t *aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE);
    if(aclk_host_config) {
        spinlock_lock(&aclk_host_config->pending_ctx_spinlock);

        bool can_enable =
            __atomic_load_n(&aclk_host_config->pending_ctx_generation, __ATOMIC_ACQUIRE) == generation &&
            !__atomic_load_n(&aclk_host_config->pending_ctx_checkpoint, __ATOMIC_ACQUIRE);

        if(can_enable)
            rrdhost_flag_set(host, RRDHOST_FLAG_ACLK_STREAM_CONTEXTS);

        spinlock_unlock(&aclk_host_config->pending_ctx_spinlock);

        if(!can_enable)
            return;
    }
    else
        rrdhost_flag_set(host, RRDHOST_FLAG_ACLK_STREAM_CONTEXTS);

    char node_str[UUID_STR_LEN];
    uuid_unparse_lower(host->node_id.uuid, node_str);
    nd_log(NDLS_ACCESS, NDLP_DEBUG,
           "ACLK REQ [%s (%s)]: STREAM CONTEXTS ENABLED",
           node_str, rrdhost_hostname(host));
}

void rrdcontext_hub_checkpoint_command(void *ptr) {
    struct ctxs_checkpoint *cmd = ptr;

    if(!claim_id_matches(cmd->claim_id)) {
        CLAIM_ID claim_id = claim_id_get();
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "RRDCONTEXT: received checkpoint command for claim_id '%s', node id '%s', "
               "but this is not our claim id. Ours '%s', received '%s'. Ignoring command.",
               cmd->claim_id, cmd->node_id,
               claim_id.str, cmd->claim_id);

        return;
    }

    RRDHOST *host = rrdhost_find_by_node_id(cmd->node_id);
    if(!host) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "RRDCONTEXT: received checkpoint command for claim id '%s', node id '%s', "
               "but there is no node with such node id here. Ignoring command.",
               cmd->claim_id, cmd->node_id);

        return;
    }

    if(rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD) ||
       rrdcontext_queue_entries(&host->rrdctx.pp_queue) > 0) {
        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "RRDCONTEXT: received checkpoint command for claim id '%s', node id '%s', "
               "but host '%s' has pending context work. Saving checkpoint for replay after processing completes.",
               cmd->claim_id, cmd->node_id, rrdhost_hostname(host));

        if(rrdcontext_checkpoint_save_pending(host, cmd))
            return;

        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "RRDCONTEXT: failed to save pending checkpoint for host '%s' (no aclk config). Executing immediately.",
               rrdhost_hostname(host));
    }

    uint64_t generation = rrdcontext_checkpoint_invalidate_pending(host);
    rrdcontext_checkpoint_execute(host, cmd->claim_id, cmd->node_id, cmd->version_hash, generation);
}

void rrdcontext_hub_stop_streaming_command(void *ptr) {
    struct stop_streaming_ctxs *cmd = ptr;

    if(!claim_id_matches(cmd->claim_id)) {
        CLAIM_ID claim_id = claim_id_get();
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "RRDCONTEXT: received stop streaming command for claim_id '%s', node id '%s', "
               "but this is not our claim id. Ours '%s', received '%s'. Ignoring command.",
               cmd->claim_id, cmd->node_id,
               claim_id.str, cmd->claim_id);

        return;
    }

    RRDHOST *host = rrdhost_find_by_node_id(cmd->node_id);
    if(!host) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "RRDCONTEXT: received stop streaming command for claim id '%s', node id '%s', "
               "but there is no node with such node id here. Ignoring command.",
               cmd->claim_id, cmd->node_id);

        return;
    }

    bool had_pending_checkpoint = false;
    struct aclk_sync_cfg_t *aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE);
    if(aclk_host_config)
        had_pending_checkpoint = __atomic_load_n(&aclk_host_config->pending_ctx_checkpoint, __ATOMIC_ACQUIRE);

    rrdcontext_checkpoint_invalidate_pending(host);

    if(!rrdhost_flag_check(host, RRDHOST_FLAG_ACLK_STREAM_CONTEXTS)) {
        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "RRDCONTEXT: received stop streaming command for claim id '%s', node id '%s', "
               "but node '%s' does not have active context streaming%s.",
               cmd->claim_id, cmd->node_id, rrdhost_hostname(host),
               had_pending_checkpoint ? "; invalidated deferred checkpoint" : "");

        return;
    }

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "RRDCONTEXT: host '%s' disabling streaming of contexts",
           rrdhost_hostname(host));

    rrdhost_flag_clear(host, RRDHOST_FLAG_ACLK_STREAM_CONTEXTS);
}

#define PENDING_CTX_CHECKPOINT_MAX_AGE_S 300

void rrdcontext_hub_pending_checkpoint_replay(RRDHOST *host) {
    struct aclk_sync_cfg_t *aclk_host_config = __atomic_load_n(&host->aclk_host_config, __ATOMIC_ACQUIRE);
    if(!aclk_host_config || !__atomic_load_n(&aclk_host_config->pending_ctx_checkpoint, __ATOMIC_ACQUIRE))
        return;

    bool pending_context_load = rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD);
    bool pp_queue_empty = rrdcontext_queue_entries(&host->rrdctx.pp_queue) <= 0;

    spinlock_lock(&aclk_host_config->pending_ctx_spinlock);
    if(!__atomic_load_n(&aclk_host_config->pending_ctx_checkpoint, __ATOMIC_RELAXED)) {
        spinlock_unlock(&aclk_host_config->pending_ctx_spinlock);
        return;
    }

    // replay when pp_queue is empty, or force replay on timeout
    time_t now_s = now_monotonic_sec();
    bool timed_out = (now_s - aclk_host_config->pending_ctx_saved_monotonic_s >= PENDING_CTX_CHECKPOINT_MAX_AGE_S);
    if((pending_context_load || !pp_queue_empty) && !timed_out) {
        spinlock_unlock(&aclk_host_config->pending_ctx_spinlock);
        return;
    }

    char *claim_id = aclk_host_config->pending_ctx_claim_id;
    char *node_id = aclk_host_config->pending_ctx_node_id;
    uint64_t version_hash = aclk_host_config->pending_ctx_version_hash;
    uint64_t generation = __atomic_load_n(&aclk_host_config->pending_ctx_generation, __ATOMIC_RELAXED);

    aclk_host_config->pending_ctx_claim_id = NULL;
    aclk_host_config->pending_ctx_node_id = NULL;
    aclk_host_config->pending_ctx_version_hash = 0;
    aclk_host_config->pending_ctx_saved_monotonic_s = 0;
    __atomic_store_n(&aclk_host_config->pending_ctx_checkpoint, false, __ATOMIC_RELEASE);
    spinlock_unlock(&aclk_host_config->pending_ctx_spinlock);

    if(timed_out && (pending_context_load || !pp_queue_empty))
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "RRDCONTEXT: pending checkpoint for host '%s' timed out after %d sec with context work still active. Forcing replay.",
               rrdhost_hostname(host), PENDING_CTX_CHECKPOINT_MAX_AGE_S);

    if(!claim_id || !node_id) {
        freez(claim_id);
        freez(node_id);
        return;
    }

    // verify claim id is still valid
    if(!claim_id_matches(claim_id)) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "RRDCONTEXT: pending checkpoint for host '%s' has stale claim id '%s'. Discarding.",
               rrdhost_hostname(host), claim_id);
        freez(claim_id);
        freez(node_id);
        return;
    }

    nd_log(NDLS_DAEMON, NDLP_NOTICE,
           "RRDCONTEXT: replaying deferred checkpoint for host '%s', claim id '%s', node id '%s'.",
           rrdhost_hostname(host), claim_id, node_id);

    rrdcontext_checkpoint_execute(host, claim_id, node_id, version_hash, generation);

    freez(claim_id);
    freez(node_id);
}

ALWAYS_INLINE
bool rrdcontext_retention_match(RRDCONTEXT_ACQUIRED *rca, time_t after, time_t before) {
    if(unlikely(!rca)) return false;

    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);

    if(rrd_flag_is_collected(rc))
        return query_matches_retention(after, before, rc->first_time_s, before > rc->last_time_s ? before : rc->last_time_s, 1);
    else
        return query_matches_retention(after, before, rc->first_time_s, rc->last_time_s, 1);
}

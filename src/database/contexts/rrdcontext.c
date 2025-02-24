// SPDX-License-Identifier: GPL-3.0-or-later

#include "internal.h"

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

    RRDHOST *host = find_host_by_node_id(cmd->node_id);
    if(!host) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "RRDCONTEXT: received checkpoint command for claim id '%s', node id '%s', "
               "but there is no node with such node id here. Ignoring command.",
               cmd->claim_id, cmd->node_id);

        return;
    }

    if(rrdhost_flag_check(host, RRDHOST_FLAG_ACLK_STREAM_CONTEXTS)) {
        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "RRDCONTEXT: received checkpoint command for claim id '%s', node id '%s', "
               "while node '%s' has an active context streaming.",
               cmd->claim_id, cmd->node_id, rrdhost_hostname(host));

        // disable it temporarily, so that our worker will not attempt to send messages in parallel
        rrdhost_flag_clear(host, RRDHOST_FLAG_ACLK_STREAM_CONTEXTS);
    }

    uint64_t our_version_hash = rrdcontext_version_hash(host);

    if(cmd->version_hash != our_version_hash) {
        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "RRDCONTEXT: received version hash %"PRIu64" for host '%s', does not match our version hash %"PRIu64". "
               "Sending snapshot of all contexts.",
               cmd->version_hash, rrdhost_hostname(host), our_version_hash);

        // prepare the snapshot
        char uuid_str[UUID_STR_LEN];
        uuid_unparse_lower(host->node_id.uuid, uuid_str);
        contexts_snapshot_t bundle = contexts_snapshot_new(cmd->claim_id, uuid_str, our_version_hash);

        // do a deep scan on every metric of the host to make sure all our data are updated
        rrdcontext_recalculate_host_retention(host, RRD_FLAG_NONE, false);

        // calculate version hash and pack all the messages together in one go
        our_version_hash = rrdcontext_version_hash_with_callback(host, rrdcontext_message_send_unsafe, true, bundle);

        // update the version
        contexts_snapshot_set_version(bundle, our_version_hash);

        // send it
        aclk_send_contexts_snapshot(bundle);
    }

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "RRDCONTEXT: host '%s' enabling streaming of contexts",
           rrdhost_hostname(host));

    rrdhost_flag_set(host, RRDHOST_FLAG_ACLK_STREAM_CONTEXTS);
    char node_str[UUID_STR_LEN];
    uuid_unparse_lower(host->node_id.uuid, node_str);
    nd_log(NDLS_ACCESS, NDLP_DEBUG,
           "ACLK REQ [%s (%s)]: STREAM CONTEXTS ENABLED",
           node_str, rrdhost_hostname(host));
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

    RRDHOST *host = find_host_by_node_id(cmd->node_id);
    if(!host) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "RRDCONTEXT: received stop streaming command for claim id '%s', node id '%s', "
               "but there is no node with such node id here. Ignoring command.",
               cmd->claim_id, cmd->node_id);

        return;
    }

    if(!rrdhost_flag_check(host, RRDHOST_FLAG_ACLK_STREAM_CONTEXTS)) {
        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "RRDCONTEXT: received stop streaming command for claim id '%s', node id '%s', "
               "but node '%s' does not have active context streaming. Ignoring command.",
               cmd->claim_id, cmd->node_id, rrdhost_hostname(host));

        return;
    }

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "RRDCONTEXT: host '%s' disabling streaming of contexts",
           rrdhost_hostname(host));

    rrdhost_flag_clear(host, RRDHOST_FLAG_ACLK_STREAM_CONTEXTS);
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

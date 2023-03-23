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

void rrdcontext_updated_rrddim(RRDDIM *rd) {
    rrdmetric_from_rrddim(rd);
}

void rrdcontext_removed_rrddim(RRDDIM *rd) {
    rrdmetric_rrddim_is_freed(rd);
}

void rrdcontext_updated_rrddim_algorithm(RRDDIM *rd) {
    rrdmetric_updated_rrddim_flags(rd);
}

void rrdcontext_updated_rrddim_multiplier(RRDDIM *rd) {
    rrdmetric_updated_rrddim_flags(rd);
}

void rrdcontext_updated_rrddim_divisor(RRDDIM *rd) {
    rrdmetric_updated_rrddim_flags(rd);
}

void rrdcontext_updated_rrddim_flags(RRDDIM *rd) {
    rrdmetric_updated_rrddim_flags(rd);
}

void rrdcontext_collected_rrddim(RRDDIM *rd) {
    rrdmetric_collected_rrddim(rd);
}

void rrdcontext_updated_rrdset(RRDSET *st) {
    rrdinstance_from_rrdset(st);
}

void rrdcontext_removed_rrdset(RRDSET *st) {
    rrdinstance_rrdset_is_freed(st);
}

void rrdcontext_updated_retention_rrdset(RRDSET *st) {
    rrdinstance_rrdset_has_updated_retention(st);
}

void rrdcontext_updated_rrdset_name(RRDSET *st) {
    rrdinstance_updated_rrdset_name(st);
}

void rrdcontext_updated_rrdset_flags(RRDSET *st) {
    rrdinstance_updated_rrdset_flags(st);
}

void rrdcontext_collected_rrdset(RRDSET *st) {
    rrdinstance_collected_rrdset(st);
}

void rrdcontext_host_child_connected(RRDHOST *host) {
    (void)host;

    // no need to do anything here
    ;
}

usec_t rrdcontext_next_db_rotation_ut = 0;
void rrdcontext_db_rotation(void) {
    // called when the db rotates its database
    rrdcontext_next_db_rotation_ut = now_realtime_usec() + FULL_RETENTION_SCAN_DELAY_AFTER_DB_ROTATION_SECS * USEC_PER_SEC;
}

int rrdcontext_find_dimension_uuid(RRDSET *st, const char *id, uuid_t *store_uuid) {
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

    uuid_copy(*store_uuid, rm->uuid);

    rrdmetric_release(rma);
    rrdinstance_release(ria);
    rrdcontext_release(rca);
    return 0;
}

int rrdcontext_find_chart_uuid(RRDSET *st, uuid_t *store_uuid) {
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
    uuid_copy(*store_uuid, ri->uuid);

    rrdinstance_release(ria);
    rrdcontext_release(rca);
    return 0;
}

void rrdcontext_host_child_disconnected(RRDHOST *host) {
    rrdcontext_recalculate_host_retention(host, RRD_FLAG_UPDATE_REASON_DISCONNECTED_CHILD, false);
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

static bool rrdhost_check_our_claim_id(const char *claim_id) {
    if(!localhost->aclk_state.claimed_id) return false;
    return (strcasecmp(claim_id, localhost->aclk_state.claimed_id) == 0) ? true : false;
}

static RRDHOST *rrdhost_find_by_node_id(const char *node_id) {
    uuid_t uuid;
    if (uuid_parse(node_id, uuid))
        return NULL;

    RRDHOST *host = NULL;
    dfe_start_read(rrdhost_root_index, host) {
        if(!host->node_id) continue;

        if(uuid_compare(uuid, *host->node_id) == 0)
            break;
    }
    dfe_done(host);

    return host;
}

void rrdcontext_hub_checkpoint_command(void *ptr) {
    struct ctxs_checkpoint *cmd = ptr;

    if(!rrdhost_check_our_claim_id(cmd->claim_id)) {
        error("RRDCONTEXT: received checkpoint command for claim_id '%s', node id '%s', but this is not our claim id. Ours '%s', received '%s'. Ignoring command.",
              cmd->claim_id, cmd->node_id,
              localhost->aclk_state.claimed_id?localhost->aclk_state.claimed_id:"NOT SET",
              cmd->claim_id);

        return;
    }

    RRDHOST *host = rrdhost_find_by_node_id(cmd->node_id);
    if(!host) {
        error("RRDCONTEXT: received checkpoint command for claim id '%s', node id '%s', but there is no node with such node id here. Ignoring command.",
              cmd->claim_id, cmd->node_id);

        return;
    }

    if(rrdhost_flag_check(host, RRDHOST_FLAG_ACLK_STREAM_CONTEXTS)) {
        info("RRDCONTEXT: received checkpoint command for claim id '%s', node id '%s', while node '%s' has an active context streaming.",
              cmd->claim_id, cmd->node_id, rrdhost_hostname(host));

        // disable it temporarily, so that our worker will not attempt to send messages in parallel
        rrdhost_flag_clear(host, RRDHOST_FLAG_ACLK_STREAM_CONTEXTS);
    }

    uint64_t our_version_hash = rrdcontext_version_hash(host);

    if(cmd->version_hash != our_version_hash) {
        error("RRDCONTEXT: received version hash %"PRIu64" for host '%s', does not match our version hash %"PRIu64". Sending snapshot of all contexts.",
              cmd->version_hash, rrdhost_hostname(host), our_version_hash);

#ifdef ENABLE_ACLK
        // prepare the snapshot
        char uuid[UUID_STR_LEN];
        uuid_unparse_lower(*host->node_id, uuid);
        contexts_snapshot_t bundle = contexts_snapshot_new(cmd->claim_id, uuid, our_version_hash);

        // do a deep scan on every metric of the host to make sure all our data are updated
        rrdcontext_recalculate_host_retention(host, RRD_FLAG_NONE, false);

        // calculate version hash and pack all the messages together in one go
        our_version_hash = rrdcontext_version_hash_with_callback(host, rrdcontext_message_send_unsafe, true, bundle);

        // update the version
        contexts_snapshot_set_version(bundle, our_version_hash);

        // send it
        aclk_send_contexts_snapshot(bundle);
#endif
    }

    internal_error(true, "RRDCONTEXT: host '%s' enabling streaming of contexts", rrdhost_hostname(host));
    rrdhost_flag_set(host, RRDHOST_FLAG_ACLK_STREAM_CONTEXTS);
    char node_str[UUID_STR_LEN];
    uuid_unparse_lower(*host->node_id, node_str);
    log_access("ACLK REQ [%s (%s)]: STREAM CONTEXTS ENABLED", node_str, rrdhost_hostname(host));
}

void rrdcontext_hub_stop_streaming_command(void *ptr) {
    struct stop_streaming_ctxs *cmd = ptr;

    if(!rrdhost_check_our_claim_id(cmd->claim_id)) {
        error("RRDCONTEXT: received stop streaming command for claim_id '%s', node id '%s', but this is not our claim id. Ours '%s', received '%s'. Ignoring command.",
              cmd->claim_id, cmd->node_id,
              localhost->aclk_state.claimed_id?localhost->aclk_state.claimed_id:"NOT SET",
              cmd->claim_id);

        return;
    }

    RRDHOST *host = rrdhost_find_by_node_id(cmd->node_id);
    if(!host) {
        error("RRDCONTEXT: received stop streaming command for claim id '%s', node id '%s', but there is no node with such node id here. Ignoring command.",
              cmd->claim_id, cmd->node_id);

        return;
    }

    if(!rrdhost_flag_check(host, RRDHOST_FLAG_ACLK_STREAM_CONTEXTS)) {
        error("RRDCONTEXT: received stop streaming command for claim id '%s', node id '%s', but node '%s' does not have active context streaming. Ignoring command.",
              cmd->claim_id, cmd->node_id, rrdhost_hostname(host));

        return;
    }

    internal_error(true, "RRDCONTEXT: host '%s' disabling streaming of contexts", rrdhost_hostname(host));
    rrdhost_flag_clear(host, RRDHOST_FLAG_ACLK_STREAM_CONTEXTS);
}

// ----------------------------------------------------------------------------
// weights API

static void metric_entry_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct metric_entry *t = value;
    t->rca = rrdcontext_acquired_dup(t->rca);
    t->ria = rrdinstance_acquired_dup(t->ria);
    t->rma = rrdmetric_acquired_dup(t->rma);
}
static void metric_entry_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct metric_entry *t = value;
    rrdcontext_release(t->rca);
    rrdinstance_release(t->ria);
    rrdmetric_release(t->rma);
}
static bool metric_entry_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value __maybe_unused, void *new_value __maybe_unused, void *data __maybe_unused) {
    internal_fatal("RRDCONTEXT: %s() detected a conflict on a metric pointer!", __FUNCTION__);
    return false;
}

DICTIONARY *rrdcontext_all_metrics_to_dict(RRDHOST *host, SIMPLE_PATTERN *contexts) {
    if(!host || !host->rrdctx.contexts)
        return NULL;

    DICTIONARY *dict = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_DONT_OVERWRITE_VALUE, &dictionary_stats_category_rrdcontext, 0);
    dictionary_register_insert_callback(dict, metric_entry_insert_callback, NULL);
    dictionary_register_delete_callback(dict, metric_entry_delete_callback, NULL);
    dictionary_register_conflict_callback(dict, metric_entry_conflict_callback, NULL);

    RRDCONTEXT *rc;
    dfe_start_reentrant(host->rrdctx.contexts, rc) {
        if(rrd_flag_is_deleted(rc))
            continue;

        if(contexts && !simple_pattern_matches_string(contexts, rc->id))
            continue;

        RRDINSTANCE *ri;
        dfe_start_read(rc->rrdinstances, ri) {
            if(rrd_flag_is_deleted(ri))
                continue;

            RRDMETRIC *rm;
            dfe_start_read(ri->rrdmetrics, rm) {
                if(rrd_flag_is_deleted(rm))
                    continue;

                struct metric_entry tmp = {
                    .rca = (RRDCONTEXT_ACQUIRED *)rc_dfe.item,
                    .ria = (RRDINSTANCE_ACQUIRED *)ri_dfe.item,
                    .rma = (RRDMETRIC_ACQUIRED *)rm_dfe.item,
                };

                char buffer[20 + 1];
                ssize_t len = snprintfz(buffer, 20, "%p", rm);
                dictionary_set_advanced(dict, buffer, len + 1, &tmp, sizeof(struct metric_entry), NULL);
            }
            dfe_done(rm);
        }
        dfe_done(ri);
    }
    dfe_done(rc);

    return dict;
}

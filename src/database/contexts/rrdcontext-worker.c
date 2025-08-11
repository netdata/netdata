// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdcontext-internal.h"

static struct {
    bool enabled;
    size_t db_rotations;
    size_t instances_count;
    size_t active_vs_archived_percentage;
} extreme_cardinality = {
    .enabled = true, // this value is ignored - there is a dynamic condition to enable it
    .db_rotations = 0,
    .instances_count = 1000,
    .active_vs_archived_percentage = 50,
};

static uint64_t rrdcontext_get_next_version(RRDCONTEXT *rc);

static void rrdcontext_garbage_collect_for_all_hosts(void);

extern usec_t rrdcontext_next_db_rotation_ut;

// ----------------------------------------------------------------------------
// version hash calculation

uint64_t rrdcontext_version_hash_with_callback(
        RRDHOST *host,
        void (*callback)(RRDCONTEXT *, bool, void *),
        bool snapshot,
        void *bundle) {

    if(unlikely(!host || !host->rrdctx.contexts)) return 0;

    RRDCONTEXT *rc;
    uint64_t hash = 0;

    // loop through all contexts of the host
    dfe_start_read(host->rrdctx.contexts, rc) {

                rrdcontext_lock(rc);

                if(unlikely(rrd_flag_check(rc, RRD_FLAG_HIDDEN))) {
                    rrdcontext_unlock(rc);
                    continue;
                }

                if(unlikely(callback))
                    callback(rc, snapshot, bundle);

                // skip any deleted contexts
                if(unlikely(rrd_flag_is_deleted(rc))) {
                    rrdcontext_unlock(rc);
                    continue;
                }

                // we use rc->hub.* which has the latest
                // metadata we have sent to the hub

                // if a context is currently queued, rc->hub.* does NOT
                // reflect the queued changes. rc->hub.* is updated with
                // their metadata, after messages are dispatched to hub.

                // when the context is being collected,
                // rc->hub.last_time_t is already zero

                hash += rc->hub.version + rc->hub.last_time_s - rc->hub.first_time_s;

                rrdcontext_unlock(rc);

            }
    dfe_done(rc);

    return hash;
}

// ----------------------------------------------------------------------------
// retention recalculation

static void rrdhost_update_cached_retention(RRDHOST *host, time_t first_time_s, time_t last_time_s, bool global) {
    if(unlikely(!host))
        return;

    spinlock_lock(&host->retention.spinlock);

    time_t old_first_time_s = host->retention.first_time_s;

    if(global) {
        host->retention.first_time_s = first_time_s;
        host->retention.last_time_s = last_time_s;
    }
    else {
        if(!host->retention.first_time_s || (first_time_s && first_time_s < host->retention.first_time_s))
            host->retention.first_time_s = first_time_s;

        if(!host->retention.last_time_s || last_time_s > host->retention.last_time_s)
            host->retention.last_time_s = last_time_s;
    }

    bool stream_path_update_required = old_first_time_s != host->retention.first_time_s;

    spinlock_unlock(&host->retention.spinlock);

    if(stream_path_update_required)
        stream_path_retention_updated(host);
}

void rrdcontext_recalculate_context_retention(RRDCONTEXT *rc, RRD_FLAGS reason, bool worker_jobs) {
    bool forcefully_removed_instances = false;
    do {
        forcefully_removed_instances = rrdcontext_post_process_updates(rc, true, reason, worker_jobs);
    } while(forcefully_removed_instances);
}

void rrdcontext_recalculate_host_retention(RRDHOST *host, RRD_FLAGS reason, bool worker_jobs) {
    if(unlikely(!host || !host->rrdctx.contexts)) return;

    time_t first_time_s = 0;
    time_t last_time_s = 0;

    RRDCONTEXT *rc;
    dfe_start_read(host->rrdctx.contexts, rc) {
        rrdcontext_recalculate_context_retention(rc, reason, worker_jobs);

        if(!first_time_s || (rc->first_time_s && rc->first_time_s < first_time_s))
            first_time_s = rc->first_time_s;

        if(!last_time_s || rc->last_time_s > last_time_s)
            last_time_s = rc->last_time_s;
    }
    dfe_done(rc);

    rrdhost_update_cached_retention(host, first_time_s, last_time_s, true);
}

static void rrdcontext_recalculate_retention_all_hosts(void) {
    rrdcontext_next_db_rotation_ut = 0;
    RRDHOST *host;
    dfe_start_reentrant(rrdhost_root_index, host) {
        worker_is_busy(WORKER_JOB_RETENTION);
        rrdcontext_recalculate_host_retention(host, RRD_FLAG_UPDATE_REASON_DB_ROTATION, true);
    }
    dfe_done(host);
}

// ----------------------------------------------------------------------------
// garbage collector

void get_metric_retention_by_id(RRDHOST *host, UUIDMAP_ID id, time_t *min_first_time_t, time_t *max_last_time_t, bool *tier0_retention) {
    *min_first_time_t = LONG_MAX;
    *max_last_time_t = 0;

    for (size_t tier = 0; tier < nd_profile.storage_tiers; tier++) {
        STORAGE_ENGINE *eng = host->db[tier].eng;

        time_t first_time_t = 0, last_time_t = 0;
        if (eng->api.metric_retention_by_id(host->db[tier].si, id, &first_time_t, &last_time_t)) {
            if (first_time_t > 0 && first_time_t < *min_first_time_t)
                *min_first_time_t = first_time_t;

            if (last_time_t > *max_last_time_t)
                *max_last_time_t = last_time_t;
        }

        if(tier == 0 && tier0_retention)
            *tier0_retention = first_time_t || last_time_t;
    }
}

bool rrdmetric_update_retention(RRDMETRIC *rm) {
    time_t min_first_time_t = LONG_MAX, max_last_time_t = 0;

    if(rm->rrddim) {
        min_first_time_t = rrddim_first_entry_s(rm->rrddim);
        max_last_time_t = rrddim_last_entry_s(rm->rrddim);
        rrd_flag_clear(rm, RRD_FLAG_NO_TIER0_RETENTION);
    }
    else {
        bool tier0_retention;
        get_metric_retention_by_id(rm->ri->rc->rrdhost, rm->uuid, &min_first_time_t, &max_last_time_t, &tier0_retention);

        if(tier0_retention)
            rrd_flag_clear(rm, RRD_FLAG_NO_TIER0_RETENTION);
        else
            rrd_flag_set(rm, RRD_FLAG_NO_TIER0_RETENTION);
    }

    if(min_first_time_t == LONG_MAX)
        min_first_time_t = 0;

    if(min_first_time_t > max_last_time_t) {
        internal_error(true, "RRDMETRIC: retention of '%s' is flipped, first_time_t = %ld, last_time_t = %ld", string2str(rm->id), min_first_time_t, max_last_time_t);
        SWAP(min_first_time_t, max_last_time_t);
    }

    // check if retention changed

    if (min_first_time_t != rm->first_time_s) {
        rm->first_time_s = min_first_time_t;
        rrd_flag_set_updated(rm, RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T);
    }

    if (max_last_time_t != rm->last_time_s) {
        rm->last_time_s = max_last_time_t;
        rrd_flag_set_updated(rm, RRD_FLAG_UPDATE_REASON_CHANGED_LAST_TIME_T);
    }

    if(unlikely(!rm->first_time_s && !rm->last_time_s))
        rrdmetric_set_deleted(rm, RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);

    rrd_flag_set(rm, RRD_FLAG_LIVE_RETENTION);

    return true;
}

static inline bool rrdmetric_should_be_deleted(RRDMETRIC *rm) {
    if(likely(!rrd_flag_check(rm, RRD_FLAGS_REQUIRED_FOR_DELETIONS)))
        return false;

    if(likely(rrd_flag_check(rm, RRD_FLAGS_PREVENTING_DELETIONS)))
        return false;

    if(likely(rm->rrddim))
        return false;

    rrdmetric_update_retention(rm);
    if(rm->first_time_s || rm->last_time_s)
        return false;

    return true;
}

static inline bool rrdinstance_should_be_deleted(RRDINSTANCE *ri) {
    if(likely(!rrd_flag_check(ri, RRD_FLAGS_REQUIRED_FOR_DELETIONS)))
        return false;

    if(likely(rrd_flag_check(ri, RRD_FLAGS_PREVENTING_DELETIONS)))
        return false;

    if(likely(ri->rrdset))
        return false;

    if(unlikely(dictionary_referenced_items(ri->rrdmetrics) != 0))
        return false;

    if(unlikely(dictionary_entries(ri->rrdmetrics) != 0))
        return false;

    if(ri->first_time_s || ri->last_time_s)
        return false;

    return true;
}

bool rrdcontext_should_be_deleted(RRDCONTEXT *rc) {
    if(likely(!rrd_flag_check(rc, RRD_FLAGS_REQUIRED_FOR_DELETIONS)))
        return false;

    if(likely(rrd_flag_check(rc, RRD_FLAGS_PREVENTING_DELETIONS)))
        return false;

    if(unlikely(dictionary_referenced_items(rc->rrdinstances) != 0))
        return false;

    if(unlikely(dictionary_entries(rc->rrdinstances) != 0))
        return false;

    if(unlikely(rc->first_time_s || rc->last_time_s))
        return false;

    return true;
}

void rrdcontext_delete_from_sql_unsafe(RRDCONTEXT *rc) {
    // we need to refresh the string pointers in rc->hub
    // in case the context changed values
    rc->hub.id = string2str(rc->id);
    rc->hub.title = string2str(rc->title);
    rc->hub.units = string2str(rc->units);
    rc->hub.family = string2str(rc->family);

    if (rc->rrdhost->rrd_memory_mode != RRD_DB_MODE_DBENGINE)
        return;

    // delete it from SQL
    if(ctx_delete_context(&rc->rrdhost->host_id.uuid, &rc->hub) != 0)
        netdata_log_error("RRDCONTEXT: failed to delete context '%s' version %"PRIu64" from SQL.",
                          rc->hub.id, rc->hub.version);
}

void rrdcontext_garbage_collect_single_host(RRDHOST *host, bool worker_jobs) {

    internal_error(true, "RRDCONTEXT: garbage collecting context structures of host '%s'", rrdhost_hostname(host));

    RRDCONTEXT *rc;
    dfe_start_reentrant(host->rrdctx.contexts, rc) {
                if(unlikely(worker_jobs && !service_running(SERVICE_CONTEXT))) break;

                if(worker_jobs) worker_is_busy(WORKER_JOB_CLEANUP);

                rrdcontext_lock(rc);

                RRDINSTANCE *ri;
                dfe_start_reentrant(rc->rrdinstances, ri) {
                            if(unlikely(worker_jobs && !service_running(SERVICE_CONTEXT))) break;

                            RRDMETRIC *rm;
                            dfe_start_write(ri->rrdmetrics, rm) {
                                        if(rrdmetric_should_be_deleted(rm)) {
                                            if(worker_jobs) worker_is_busy(WORKER_JOB_CLEANUP_DELETE);
                                            if(!dictionary_del(ri->rrdmetrics, string2str(rm->id)))
                                                netdata_log_error("RRDCONTEXT: metric '%s' of instance '%s' of context '%s' of host '%s', failed to be deleted from rrdmetrics dictionary.",
                                                                  string2str(rm->id),
                                                                  string2str(ri->id),
                                                                  string2str(rc->id),
                                                                  rrdhost_hostname(host));
                                            else
                                                internal_error(
                                                        true,
                                                        "RRDCONTEXT: metric '%s' of instance '%s' of context '%s' of host '%s', deleted from rrdmetrics dictionary.",
                                                        string2str(rm->id),
                                                        string2str(ri->id),
                                                        string2str(rc->id),
                                                        rrdhost_hostname(host));
                                        }
                                    }
                            dfe_done(rm);

                            if(rrdinstance_should_be_deleted(ri)) {
                                if(worker_jobs) worker_is_busy(WORKER_JOB_CLEANUP_DELETE);
                                if(!dictionary_del(rc->rrdinstances, string2str(ri->id)))
                                    netdata_log_error("RRDCONTEXT: instance '%s' of context '%s' of host '%s', failed to be deleted from rrdmetrics dictionary.",
                                                      string2str(ri->id),
                                                      string2str(rc->id),
                                                      rrdhost_hostname(host));
                                else
                                    internal_error(
                                            true,
                                            "RRDCONTEXT: instance '%s' of context '%s' of host '%s', deleted from rrdmetrics dictionary.",
                                            string2str(ri->id),
                                            string2str(rc->id),
                                            rrdhost_hostname(host));
                            }

                            dictionary_garbage_collect(ri->rrdmetrics);
                        }
                dfe_done(ri);
                dictionary_garbage_collect(rc->rrdinstances);

                if(unlikely(rrdcontext_should_be_deleted(rc))) {
                    if(worker_jobs) worker_is_busy(WORKER_JOB_CLEANUP_DELETE);
                    rrdcontext_dequeue_from_post_processing(rc);
                    rrdcontext_delete_from_sql_unsafe(rc);

                    if(!dictionary_del(host->rrdctx.contexts, string2str(rc->id)))
                        netdata_log_error("RRDCONTEXT: context '%s' of host '%s', failed to be deleted from rrdmetrics dictionary.",
                              string2str(rc->id),
                              rrdhost_hostname(host));
                    else
                        internal_error(
                                true,
                                "RRDCONTEXT: context '%s' of host '%s', deleted from rrdmetrics dictionary.",
                                string2str(rc->id),
                                rrdhost_hostname(host));
                }

                // the item is referenced in the dictionary
                // so, it is still here to unlock, even if we have deleted it
                rrdcontext_unlock(rc);
            }
    dfe_done(rc);

    dictionary_garbage_collect(host->rrdctx.contexts);
}

static void rrdcontext_garbage_collect_for_all_hosts(void) {
    RRDHOST *host;
    dfe_start_reentrant(rrdhost_root_index, host) {
        rrdcontext_garbage_collect_single_host(host, true);
    }
    dfe_done(host);
}

// ----------------------------------------------------------------------------
// post processing

static void rrdmetric_process_updates(RRDMETRIC *rm, bool force, RRD_FLAGS reason, bool worker_jobs) {
    if(reason != RRD_FLAG_NONE)
        rrd_flag_set_updated(rm, reason);

    if(!force && !rrd_flag_is_updated(rm) && rrd_flag_check(rm, RRD_FLAG_LIVE_RETENTION) && !rrd_flag_check(rm, RRD_FLAG_UPDATE_REASON_UPDATE_RETENTION))
        return;

    if(worker_jobs)
        worker_is_busy(WORKER_JOB_PP_METRIC);

    if(reason & RRD_FLAG_UPDATE_REASON_DISCONNECTED_CHILD) {
        rrdmetric_set_archived(rm);
        rrd_flag_set(rm, RRD_FLAG_UPDATE_REASON_DISCONNECTED_CHILD);
    }
    if(rrd_flag_is_deleted(rm) && (reason & RRD_FLAG_UPDATE_REASON_UPDATE_RETENTION))
        rrdmetric_set_archived(rm);

    rrdmetric_update_retention(rm);

    rrd_flag_unset_updated(rm);
}

static void rrdinstance_post_process_updates(RRDINSTANCE *ri, bool force, RRD_FLAGS reason, bool worker_jobs) {
    if(reason != RRD_FLAG_NONE)
        rrd_flag_set_updated(ri, reason);

    if(!force && !rrd_flag_is_updated(ri) && rrd_flag_check(ri, RRD_FLAG_LIVE_RETENTION))
        return;

    if(worker_jobs)
        worker_is_busy(WORKER_JOB_PP_INSTANCE);

    time_t min_first_time_t = LONG_MAX, max_last_time_t = 0;
    size_t metrics_active = 0, metrics_deleted = 0, metrics_no_tier0 = 0;
    bool live_retention = true, currently_collected = false;
    if(dictionary_entries(ri->rrdmetrics) > 0) {
        RRDMETRIC *rm;
        dfe_start_read((DICTIONARY *)ri->rrdmetrics, rm) {
                    if(unlikely(worker_jobs && !service_running(SERVICE_CONTEXT))) break;

                    RRD_FLAGS reason_to_pass = reason;
                    if(rrd_flag_check(ri, RRD_FLAG_UPDATE_REASON_UPDATE_RETENTION))
                        reason_to_pass |= RRD_FLAG_UPDATE_REASON_UPDATE_RETENTION;

                    rrdmetric_process_updates(rm, force, reason_to_pass, worker_jobs);

                    if(unlikely(!rrd_flag_check(rm, RRD_FLAG_LIVE_RETENTION)))
                        live_retention = false;

                    if(unlikely(rrd_flag_check(rm, RRD_FLAG_NO_TIER0_RETENTION)))
                        metrics_no_tier0++;

                    if (unlikely((rrdmetric_should_be_deleted(rm)))) {
                        metrics_deleted++;
                        continue;
                    }

                    if(!currently_collected && rrd_flag_is_collected(rm) && rm->first_time_s)
                        currently_collected = true;

                    metrics_active++;

                    if (rm->first_time_s && rm->first_time_s < min_first_time_t)
                        min_first_time_t = rm->first_time_s;

                    if (rm->last_time_s && rm->last_time_s > max_last_time_t)
                        max_last_time_t = rm->last_time_s;
                }
        dfe_done(rm);
    }

    if(metrics_no_tier0 && metrics_no_tier0 == metrics_active)
        rrd_flag_set(ri, RRD_FLAG_NO_TIER0_RETENTION);
    else
        rrd_flag_clear(ri, RRD_FLAG_NO_TIER0_RETENTION);

    if(unlikely(live_retention && !rrd_flag_check(ri, RRD_FLAG_LIVE_RETENTION)))
        rrd_flag_set(ri, RRD_FLAG_LIVE_RETENTION);
    else if(unlikely(!live_retention && rrd_flag_check(ri, RRD_FLAG_LIVE_RETENTION)))
        rrd_flag_clear(ri, RRD_FLAG_LIVE_RETENTION);

    if(unlikely(!metrics_active)) {
        // no metrics available

        if(ri->first_time_s) {
            ri->first_time_s = 0;
            rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T);
        }

        if(ri->last_time_s) {
            ri->last_time_s = 0;
            rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_LAST_TIME_T);
        }

        rrdinstance_set_deleted(ri, RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);
    }
    else {
        // we have active metrics...

        if (unlikely(min_first_time_t == LONG_MAX))
            min_first_time_t = 0;

        if (unlikely(min_first_time_t == 0 || max_last_time_t == 0)) {
            if(ri->first_time_s) {
                ri->first_time_s = 0;
                rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T);
            }

            if(ri->last_time_s) {
                ri->last_time_s = 0;
                rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_LAST_TIME_T);
            }

            if(likely(live_retention))
                rrdinstance_set_deleted(ri, RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);
        }
        else {
            rrd_flag_clear(ri, RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);

            if (unlikely(ri->first_time_s != min_first_time_t)) {
                ri->first_time_s = min_first_time_t;
                rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T);
            }

            if (unlikely(ri->last_time_s != max_last_time_t)) {
                ri->last_time_s = max_last_time_t;
                rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_LAST_TIME_T);
            }

            if(likely(currently_collected))
                rrdinstance_set_collected(ri);
            else
                rrdinstance_set_archived(ri);
        }
    }

    rrd_flag_unset_updated(ri);
}

static bool rrdinstance_forcefully_clear_retention(RRDCONTEXT *rc, size_t count, const char *descr) {
    if(!count) return false;

    RRDHOST *host = rc->rrdhost;

    time_t from_s = LONG_MAX;
    time_t to_s = 0;

    size_t instances_deleted = 0;
    size_t metrics_deleted = 0;
    RRDINSTANCE *ri;
    dfe_start_read(rc->rrdinstances, ri) {
        if(!rrd_flag_check(ri, RRD_FLAG_NO_TIER0_RETENTION) || rrd_flag_is_collected(ri) || ri->rrdset)
            continue;

        size_t metrics_cleared = 0;
        RRDMETRIC *rm;
        dfe_start_read(ri->rrdmetrics, rm) {
            if(!rrd_flag_check(rm, RRD_FLAG_NO_TIER0_RETENTION) || rrd_flag_is_collected(rm) || rm->rrddim)
                continue;

            rrdmetric_update_retention(rm);

            if(rm->first_time_s < from_s)
                from_s = rm->first_time_s;

            if(rm->last_time_s > to_s)
                to_s = rm->last_time_s;

            for (size_t tier = 0; tier < nd_profile.storage_tiers; tier++) {
                STORAGE_ENGINE *eng = host->db[tier].eng;
                eng->api.metric_retention_delete_by_id(host->db[tier].si, rm->uuid);
            }

            metrics_cleared++;
            metrics_deleted++;
            rrdmetric_update_retention(rm);
            rrdmetric_trigger_updates(rm, __FUNCTION__ );
        }
        dfe_done(rm);

        if(metrics_cleared) {
            rrdinstance_trigger_updates(ri, __FUNCTION__ );
            instances_deleted++;

            if(--count == 0)
                break;
        }
    }
    dfe_done(ri);

    if(metrics_deleted) {
        char from_txt[128], to_txt[128];

        if(!from_s || from_s == LONG_MAX)
            snprintfz(from_txt, sizeof(from_txt), "%s", "NONE");
        else
            rfc3339_datetime_ut(from_txt, sizeof(from_txt), from_s * USEC_PER_SEC, 0, true);

        if(!to_s)
            snprintfz(to_txt, sizeof(to_txt), "%s", "NONE");
        else
            rfc3339_datetime_ut(to_txt, sizeof(to_txt), to_s * USEC_PER_SEC, 0, true);

        ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_TXT(NDF_MODULE, "extreme cardinality protection"),
            ND_LOG_FIELD_STR(NDF_NIDL_NODE, rc->rrdhost->hostname),
            ND_LOG_FIELD_STR(NDF_NIDL_CONTEXT, rc->id),
            ND_LOG_FIELD_UUID(NDF_MESSAGE_ID, &extreme_cardinality_msgid),
            ND_LOG_FIELD_END(),
        };
        ND_LOG_STACK_PUSH(lgs);

        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "EXTREME CARDINALITY PROTECTION: host '%s', context '%s', %s: "
               "forcefully cleared the retention of %zu metrics and %zu instances, "
               "having non-tier0 retention from %s to %s.",
               rrdhost_hostname(rc->rrdhost),
               string2str(rc->id),
               descr,
               metrics_deleted, instances_deleted,
               from_txt, to_txt);

        return true;
    }

    return false;
}

bool rrdcontext_post_process_updates(RRDCONTEXT *rc, bool force, RRD_FLAGS reason, bool worker_jobs) {
    bool ret = false;

    if(reason != RRD_FLAG_NONE)
        rrd_flag_set_updated(rc, reason);

    if(worker_jobs)
        worker_is_busy(WORKER_JOB_PP_CONTEXT);

    size_t min_priority_collected = LONG_MAX;
    size_t min_priority_not_collected = LONG_MAX;
    size_t min_priority = LONG_MAX;
    time_t min_first_time_t = LONG_MAX, max_last_time_t = 0;
    size_t instances_active = 0, instances_deleted = 0, instances_no_tier0 = 0;
    bool live_retention = true, currently_collected = false, hidden = true;
    if(dictionary_entries(rc->rrdinstances) > 0) {
        RRDINSTANCE *ri;
        dfe_start_reentrant(rc->rrdinstances, ri) {
                    if(unlikely(worker_jobs && !service_running(SERVICE_CONTEXT))) break;

                    RRD_FLAGS reason_to_pass = reason;
                    if(rrd_flag_check(rc, RRD_FLAG_UPDATE_REASON_UPDATE_RETENTION))
                        reason_to_pass |= RRD_FLAG_UPDATE_REASON_UPDATE_RETENTION;

                    rrdinstance_post_process_updates(ri, force, reason_to_pass, worker_jobs);

                    if(unlikely(hidden && !rrd_flag_check(ri, RRD_FLAG_HIDDEN)))
                        hidden = false;

                    if(unlikely(live_retention && !rrd_flag_check(ri, RRD_FLAG_LIVE_RETENTION)))
                        live_retention = false;

                    if (unlikely(rrdinstance_should_be_deleted(ri))) {
                        instances_deleted++;
                        continue;
                    }

                    if(unlikely(rrd_flag_check(ri, RRD_FLAG_NO_TIER0_RETENTION)))
                        instances_no_tier0++;

                    bool ri_collected = rrd_flag_is_collected(ri);

                    if(ri_collected && !rrd_flag_check(ri, RRD_FLAG_MERGED_COLLECTED_RI_TO_RC)) {
                        rrdcontext_update_from_collected_rrdinstance(ri);
                        rrd_flag_set(ri, RRD_FLAG_MERGED_COLLECTED_RI_TO_RC);
                    }

                    if(unlikely(!currently_collected && rrd_flag_is_collected(ri) && ri->first_time_s))
                        currently_collected = true;

                    internal_error(rc->units != ri->units,
                                   "RRDCONTEXT: '%s' rrdinstance '%s' has different units, context '%s', instance '%s'",
                                   string2str(rc->id), string2str(ri->id),
                                   string2str(rc->units), string2str(ri->units));

                    instances_active++;

                    if (ri->priority >= RRDCONTEXT_MINIMUM_ALLOWED_PRIORITY) {
                        if(rrd_flag_is_collected(ri)) {
                            if(ri->priority < min_priority_collected)
                                min_priority_collected = ri->priority;
                        }
                        else {
                            if(ri->priority < min_priority_not_collected)
                                min_priority_not_collected = ri->priority;
                        }
                    }

                    if (ri->first_time_s && ri->first_time_s < min_first_time_t)
                        min_first_time_t = ri->first_time_s;

                    if (ri->last_time_s && ri->last_time_s > max_last_time_t)
                        max_last_time_t = ri->last_time_s;
                }
        dfe_done(ri);

        if(extreme_cardinality.enabled &&
            extreme_cardinality.db_rotations &&
            instances_active &&
            instances_no_tier0 >= extreme_cardinality.instances_count) {
            size_t percent = (100 * instances_no_tier0 / instances_active);
            if(percent >= extreme_cardinality.active_vs_archived_percentage) {
                size_t to_keep = extreme_cardinality.active_vs_archived_percentage * instances_active / 100;
                to_keep = MAX(to_keep, extreme_cardinality.instances_count);
                size_t to_remove = instances_no_tier0 > to_keep ? instances_no_tier0 - to_keep : 0;

                if(to_remove) {
                    char buf[256];
                    snprintfz(buf, sizeof(buf),
                              "total active instances %zu, not in tier0 %zu, ephemerality %zu%%",
                              instances_active, instances_no_tier0, percent);
                    ret = rrdinstance_forcefully_clear_retention(rc, to_remove, buf);
                }
            }
        }

        if(min_priority_collected != LONG_MAX)
            // use the collected priority
            min_priority = min_priority_collected;
        else
            // use the non-collected priority
            min_priority = min_priority_not_collected;
    }

    {
        bool previous_hidden = rrd_flag_check(rc, RRD_FLAG_HIDDEN);
        if (hidden != previous_hidden) {
            if (hidden && !rrd_flag_check(rc, RRD_FLAG_HIDDEN))
                rrd_flag_set(rc, RRD_FLAG_HIDDEN);
            else if (!hidden && rrd_flag_check(rc, RRD_FLAG_HIDDEN))
                rrd_flag_clear(rc, RRD_FLAG_HIDDEN);
        }

        bool previous_live_retention = rrd_flag_check(rc, RRD_FLAG_LIVE_RETENTION);
        if (live_retention != previous_live_retention) {
            if (live_retention && !rrd_flag_check(rc, RRD_FLAG_LIVE_RETENTION))
                rrd_flag_set(rc, RRD_FLAG_LIVE_RETENTION);
            else if (!live_retention && rrd_flag_check(rc, RRD_FLAG_LIVE_RETENTION))
                rrd_flag_clear(rc, RRD_FLAG_LIVE_RETENTION);
        }
    }

    rrdcontext_lock(rc);
    rc->pp.executions++;

    if(unlikely(!instances_active)) {
        // we had some instances, but they are gone now...

        if(rc->first_time_s) {
            rc->first_time_s = 0;
            rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T);
        }

        if(rc->last_time_s) {
            rc->last_time_s = 0;
            rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_LAST_TIME_T);
        }

        rrdcontext_set_deleted(rc, RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);
    }
    else {
        // we have some active instances...

        if (unlikely(min_first_time_t == LONG_MAX))
            min_first_time_t = 0;

        if (unlikely(min_first_time_t == 0 && max_last_time_t == 0)) {
            if(rc->first_time_s) {
                rc->first_time_s = 0;
                rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T);
            }

            if(rc->last_time_s) {
                rc->last_time_s = 0;
                rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_LAST_TIME_T);
            }

            rrdcontext_set_deleted(rc, RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);
        }
        else {
            rrd_flag_clear(rc, RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);

            if (unlikely(rc->first_time_s != min_first_time_t)) {
                rc->first_time_s = min_first_time_t;
                rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T);
            }

            if (rc->last_time_s != max_last_time_t) {
                rc->last_time_s = max_last_time_t;
                rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_LAST_TIME_T);
            }

            if(likely(currently_collected))
                rrdcontext_set_collected(rc);
            else
                rrdcontext_set_archived(rc);
        }

        if (min_priority != LONG_MAX && rc->priority != min_priority) {
            rc->priority = min_priority;
            rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
        }
    }

    if(unlikely(rrd_flag_is_updated(rc))) {
        if(check_if_cloud_version_changed_unsafe(rc, false)) {
            rc->version = rrdcontext_get_next_version(rc);
            rrdcontext_add_to_hub_queue(rc);
        }
    }

    rrd_flag_unset_updated(rc);
    rrdcontext_unlock(rc);

    return ret;
}

void rrdcontext_queue_for_post_processing(RRDCONTEXT *rc, const char *function __maybe_unused, RRD_FLAGS flags __maybe_unused) {
#if 0
    if(string_strcmp(rc->id, "system.cpu") == 0) {
        CLEAN_BUFFER *wb = buffer_create(0, NULL);
        buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
        buffer_json_member_add_array(wb, "flags");
        rrd_flags_to_buffer_json_array_items(rc->flags, wb);
        buffer_json_array_close(wb);
        buffer_json_member_add_array(wb, "reasons");
        rrd_reasons_to_buffer_json_array_items(rc->flags, wb);
        buffer_json_array_close(wb);
        buffer_json_finalize(wb);
        nd_log(NDLS_DAEMON, NDLP_EMERG, "%s() context '%s', triggered: %s",
               function, string2str(rc->id), buffer_tostring(wb));
    }
#endif

    rrdcontext_add_to_pp_queue(rc);
}

void rrdcontext_initial_processing_after_loading(RRDCONTEXT *rc) {
    rrdcontext_dequeue_from_post_processing(rc);
    rrdcontext_post_process_updates(rc, false, RRD_FLAG_NONE, false);
}

void rrdcontext_delete_after_loading(RRDHOST *host, RRDCONTEXT *rc) {
    rrdcontext_del_from_hub_queue(rc, false);
    rrdcontext_dequeue_from_post_processing(rc);
    dictionary_del(host->rrdctx.contexts, string2str(rc->id));
}

// ----------------------------------------------------------------------------
// dispatching contexts to cloud

static uint64_t rrdcontext_get_next_version(RRDCONTEXT *rc) {
    time_t now = now_realtime_sec();
    uint64_t version = MAX(rc->version, rc->hub.version);
    version = MAX((uint64_t)now, version);
    version++;
    return version;
}

void rrdcontext_message_send_unsafe(RRDCONTEXT *rc, bool snapshot __maybe_unused, void *bundle __maybe_unused) {

    // save it, so that we know the last version we sent to hub
    rc->version = rc->hub.version = rrdcontext_get_next_version(rc);
    rc->hub.id = string2str(rc->id);
    rc->hub.title = string2str(rc->title);
    rc->hub.units = string2str(rc->units);
    rc->hub.family = string2str(rc->family);
    rc->hub.chart_type = rrdset_type_name(rc->chart_type);
    rc->hub.priority = rc->priority;
    rc->hub.first_time_s = rc->first_time_s;
    rc->hub.last_time_s = rrd_flag_is_collected(rc) ? 0 : rc->last_time_s;
    rc->hub.deleted = rrd_flag_is_deleted(rc) ? true : false;

    struct context_updated message = {
            .id = rc->hub.id,
            .version = rc->hub.version,
            .title = rc->hub.title,
            .units = rc->hub.units,
            .family = rc->hub.family,
            .chart_type = rc->hub.chart_type,
            .priority = rc->hub.priority,
            .first_entry = rc->hub.first_time_s,
            .last_entry = rc->hub.last_time_s,
            .deleted = rc->hub.deleted,
    };

    if(likely(!rrd_flag_check(rc, RRD_FLAG_HIDDEN))) {
        if (snapshot) {
            if (!rc->hub.deleted)
                contexts_snapshot_add_ctx_update(bundle, &message);
        }
        else
            contexts_updated_add_ctx_update(bundle, &message);
    }

    // store it to SQL

    if(rrd_flag_is_deleted(rc))
        rrdcontext_delete_from_sql_unsafe(rc);

    else {
        if (rc->rrdhost->rrd_memory_mode != RRD_DB_MODE_DBENGINE)
            return;
        if (ctx_store_context(&rc->rrdhost->host_id.uuid, &rc->hub) != 0)
            netdata_log_error(
                "RRDCONTEXT: failed to save context '%s' version %" PRIu64 " to SQL.", rc->hub.id, rc->hub.version);
    }
}

bool check_if_cloud_version_changed_unsafe(RRDCONTEXT *rc, bool sending __maybe_unused) {
    bool id_changed = false,
            title_changed = false,
            units_changed = false,
            family_changed = false,
            chart_type_changed = false,
            priority_changed = false,
            first_time_changed = false,
            last_time_changed = false,
            deleted_changed = false;

    RRD_FLAGS flags = rrd_flags_get(rc);

    if(unlikely(string2str(rc->id) != rc->hub.id))
        id_changed = true;

    if(unlikely(string2str(rc->title) != rc->hub.title))
        title_changed = true;

    if(unlikely(string2str(rc->units) != rc->hub.units))
        units_changed = true;

    if(unlikely(string2str(rc->family) != rc->hub.family))
        family_changed = true;

    if(unlikely(rrdset_type_name(rc->chart_type) != rc->hub.chart_type))
        chart_type_changed = true;

    if(unlikely(rc->priority != rc->hub.priority))
        priority_changed = true;

    if(unlikely((uint64_t)rc->first_time_s != rc->hub.first_time_s))
        first_time_changed = true;

    if(unlikely((uint64_t)((flags & RRD_FLAG_COLLECTED) ? 0 : rc->last_time_s) != rc->hub.last_time_s))
        last_time_changed = true;

    if(unlikely(((flags & RRD_FLAG_DELETED) ? true : false) != rc->hub.deleted))
        deleted_changed = true;

    if(unlikely(id_changed || title_changed || units_changed || family_changed || chart_type_changed || priority_changed || first_time_changed || last_time_changed || deleted_changed)) {

        internal_error(LOG_TRANSITIONS,
                       "RRDCONTEXT: %s NEW VERSION '%s'%s of host '%s', version %"PRIu64", title '%s'%s, units '%s'%s, family '%s'%s, chart type '%s'%s, priority %u%s, first_time_t %ld%s, last_time_t %ld%s, deleted '%s'%s, (queued for %llu ms, expected %llu ms)",
                       sending?"SENDING":"QUEUE",
                       string2str(rc->id), id_changed ? " (CHANGED)" : "",
                       rrdhost_hostname(rc->rrdhost),
                       rc->version,
                       string2str(rc->title), title_changed ? " (CHANGED)" : "",
                       string2str(rc->units), units_changed ? " (CHANGED)" : "",
                       string2str(rc->family), family_changed ? " (CHANGED)" : "",
                       rrdset_type_name(rc->chart_type), chart_type_changed ? " (CHANGED)" : "",
                       rc->priority, priority_changed ? " (CHANGED)" : "",
                       rc->first_time_s, first_time_changed ? " (CHANGED)" : "",
                       (flags & RRD_FLAG_COLLECTED) ? 0 : rc->last_time_s, last_time_changed ? " (CHANGED)" : "",
                       (flags & RRD_FLAG_DELETED) ? "true" : "false", deleted_changed ? " (CHANGED)" : "",
                       sending ? (now_realtime_usec() - rc->queue.queued_ut) / USEC_PER_MS : 0,
                       sending ? (rc->queue.scheduled_dispatch_ut - rc->queue.queued_ut) / USEC_PER_MS : 0
        );

        rrdhost_update_cached_retention(rc->rrdhost, rc->first_time_s, rc->last_time_s, false);

        return true;
    }

    if(!(flags & RRD_FLAG_COLLECTED))
        rrdhost_update_cached_retention(rc->rrdhost, rc->first_time_s, rc->last_time_s, false);

    return false;
}

usec_t rrdcontext_calculate_queued_dispatch_time_ut(RRDCONTEXT *rc, usec_t now_ut) {

    if(likely(rc->queue.delay_calc_ut >= rc->queue.queued_ut))
        return rc->queue.scheduled_dispatch_ut;

    RRD_FLAGS flags = rc->queue.queued_flags;

    usec_t delay = LONG_MAX;
    int i;
    struct rrdcontext_reason *reason;
    for(i = 0, reason = &rrdcontext_reasons[i]; reason->name ; reason = &rrdcontext_reasons[++i]) {
        if(unlikely(flags & reason->flag)) {
            if(reason->delay_ut < delay)
                delay = reason->delay_ut;
        }
    }

    if(unlikely(delay == LONG_MAX)) {
        internal_error(true, "RRDCONTEXT: '%s', cannot find minimum delay of flags %x", string2str(rc->id), (unsigned int)flags);
        delay = 60 * USEC_PER_SEC;
    }

    rc->queue.delay_calc_ut = now_ut;
    usec_t dispatch_ut = rc->queue.scheduled_dispatch_ut = rc->queue.queued_ut + delay;
    return dispatch_ut;
}

// ----------------------------------------------------------------------------
// worker thread

static void rrdcontext_main_cleanup(void *pptr) {
    struct netdata_static_thread *static_thread = CLEANUP_FUNCTION_GET_PTR(pptr);
    if(!static_thread) return;

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    // custom code
    worker_unregister();

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void rrdcontext_main(void *ptr) {
    CLEANUP_FUNCTION_REGISTER(rrdcontext_main_cleanup) cleanup_ptr = ptr;

    worker_register("RRDCONTEXT");
    worker_register_job_name(WORKER_JOB_HOSTS, "hosts");
    worker_register_job_name(WORKER_JOB_CHECK, "dedup checks");
    worker_register_job_name(WORKER_JOB_SEND, "sent contexts");
    worker_register_job_name(WORKER_JOB_DEQUEUE, "deduplicated contexts");
    worker_register_job_name(WORKER_JOB_RETENTION, "metrics retention");
    worker_register_job_name(WORKER_JOB_QUEUED, "queued contexts");
    worker_register_job_name(WORKER_JOB_CLEANUP, "cleanups");
    worker_register_job_name(WORKER_JOB_CLEANUP_DELETE, "deletes");
    worker_register_job_name(WORKER_JOB_PP_METRIC, "check metrics");
    worker_register_job_name(WORKER_JOB_PP_INSTANCE, "check instances");
    worker_register_job_name(WORKER_JOB_PP_CONTEXT, "check contexts");

    worker_register_job_custom_metric(WORKER_JOB_HUB_QUEUE_SIZE, "hub queue size", "contexts", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(WORKER_JOB_PP_QUEUE_SIZE, "post processing queue size", "contexts", WORKER_METRIC_ABSOLUTE);

    heartbeat_t hb;
    heartbeat_init(&hb, RRDCONTEXT_WORKER_THREAD_HEARTBEAT_USEC);

    extreme_cardinality.enabled = inicfg_get_boolean(
        &netdata_config, CONFIG_SECTION_DB, "extreme cardinality protection",
        nd_profile.storage_tiers > 1 && default_rrd_memory_mode == RRD_DB_MODE_DBENGINE
    );

    extreme_cardinality.instances_count = inicfg_get_number_range(
        &netdata_config, CONFIG_SECTION_DB, "extreme cardinality keep instances",
        (long long)extreme_cardinality.instances_count, 1, 1000000);

    extreme_cardinality.active_vs_archived_percentage = inicfg_get_number_range(
        &netdata_config, CONFIG_SECTION_DB, "extreme cardinality min ephemerality",
        (long long)extreme_cardinality.active_vs_archived_percentage, 0, 100);

    while (service_running(SERVICE_CONTEXT)) {
        worker_is_idle();
        heartbeat_next(&hb);

        if(unlikely(!service_running(SERVICE_CONTEXT))) break;

        usec_t now_ut = now_realtime_usec();

        if(rrdcontext_next_db_rotation_ut && now_ut > rrdcontext_next_db_rotation_ut) {
            extreme_cardinality.db_rotations++;
            rrdcontext_recalculate_retention_all_hosts();
            rrdcontext_garbage_collect_for_all_hosts();
            rrdcontext_next_db_rotation_ut = 0;
        }

        size_t hub_queued_contexts_for_all_hosts = 0;
        size_t pp_queued_contexts_for_all_hosts = 0;

        RRDHOST *host;
        dfe_start_reentrant(rrdhost_root_index, host) {
            if(unlikely(!service_running(SERVICE_CONTEXT))) break;

            if(rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD))
                continue;

            if(rrdhost_flag_check(host, RRDHOST_FLAG_RRDCONTEXT_GET_RETENTION)) {
                rrdcontext_recalculate_host_retention(host, RRD_FLAG_UPDATE_REASON_DISCONNECTED_CHILD, false);
                rrdhost_flag_clear(host, RRDHOST_FLAG_RRDCONTEXT_GET_RETENTION);
            }

            worker_is_busy(WORKER_JOB_HOSTS);

            pp_queued_contexts_for_all_hosts += rrdcontext_queue_entries(&host->rrdctx.pp_queue);
            rrdcontext_post_process_queued_contexts(host);

            hub_queued_contexts_for_all_hosts += rrdcontext_queue_entries(&host->rrdctx.hub_queue);
            rrdcontext_dispatch_queued_contexts_to_hub(host, now_ut);

            if (host->rrdctx.contexts)
                dictionary_garbage_collect(host->rrdctx.contexts);
        }
        dfe_done(host);

        worker_set_metric(WORKER_JOB_HUB_QUEUE_SIZE, (NETDATA_DOUBLE)hub_queued_contexts_for_all_hosts);
        worker_set_metric(WORKER_JOB_PP_QUEUE_SIZE, (NETDATA_DOUBLE)pp_queued_contexts_for_all_hosts);
    }
}

// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdcontext-internal.h"

#define RRDCONTEXT_HUB_QUEUE_MAX_OFFLINE_AGE_UT   (15 * 60 * USEC_PER_SEC)
#define RRDCONTEXT_HUB_QUEUE_MAX_OFFLINE_ENTRIES  10000

typedef enum {
    RRDCONTEXT_QUEUE_INVALID = 0,
    RRDCONTEXT_QUEUE_ADDED,
    RRDCONTEXT_QUEUE_FOUND,
} RRDCONTEXT_QUEUE_STATUS;

static inline RRDCONTEXT_QUEUE_STATUS rrdcontext_queue_add(RRDCONTEXT_QUEUE_JudyLSet *queue, RRDCONTEXT *rc, Word_t *idx, bool having_lock) {
    RRDCONTEXT_QUEUE_STATUS ret = RRDCONTEXT_QUEUE_INVALID;
    if(!queue || !rc || !idx) return ret;

    if(!having_lock)
        spinlock_lock(&queue->spinlock);

    if(*idx) {
        fatal_assert(RRDCONTEXT_QUEUE_GET(queue, *idx) == rc);
        ret = RRDCONTEXT_QUEUE_FOUND;
    }
    else {
        *idx = queue->id++;
        RRDCONTEXT_QUEUE_SET(queue, *idx, rc);
        __atomic_add_fetch(&queue->version, 1, __ATOMIC_RELAXED);
        __atomic_add_fetch(&queue->entries, 1, __ATOMIC_RELAXED);
        ret = RRDCONTEXT_QUEUE_ADDED;
    }

    if(!having_lock)
        spinlock_unlock(&queue->spinlock);

    return ret;
}

void rrdcontext_add_to_hub_queue(RRDCONTEXT *rc) {
    if(!rc || !rc->rrdhost) return;

    CLAIM_ID claim_id = claim_id_get();
    if(unlikely(!claim_id_is_set(claim_id)))
        return;

    spinlock_lock(&rc->rrdhost->rrdctx.hub_queue.spinlock);

    RRDCONTEXT_QUEUE_STATUS ret = rrdcontext_queue_add(&rc->rrdhost->rrdctx.hub_queue, rc, &rc->queue.idx, true);

    if(ret == RRDCONTEXT_QUEUE_ADDED) {
        rrd_flag_set(rc, RRD_FLAG_QUEUED_FOR_HUB);
        rc->queue.queued_ut = now_realtime_usec();
        rc->queue.queued_flags = rrd_flags_get(rc);
    }
    else if(ret == RRDCONTEXT_QUEUE_FOUND) {
        rrd_flag_set(rc, RRD_FLAG_QUEUED_FOR_HUB);
        rc->queue.queued_ut = now_realtime_usec();
        rc->queue.queued_flags |= rrd_flags_get(rc);
    }

    spinlock_unlock(&rc->rrdhost->rrdctx.hub_queue.spinlock);
}

static void rrdcontext_prune_hub_queue(RRDHOST *host, usec_t now_ut, bool force_drop_all, bool apply_bounds) {
    if(!rrdcontext_queue_entries(&host->rrdctx.hub_queue))
        return;

    size_t dropped = 0;
    int32_t queued = rrdcontext_queue_entries(&host->rrdctx.hub_queue);

    spinlock_lock(&host->rrdctx.hub_queue.spinlock);
    Word_t idx = 0;
    for(RRDCONTEXT *rc = RRDCONTEXT_QUEUE_FIRST(&host->rrdctx.hub_queue, &idx);
        rc;
        rc = RRDCONTEXT_QUEUE_NEXT(&host->rrdctx.hub_queue, &idx)) {
        if(unlikely(!service_running(SERVICE_CONTEXT)))
            break;

        const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(host->rrdctx.contexts, string2str(rc->id));
        bool context_matches = item && (dictionary_acquired_item_value(item) == rc);
        bool stale = !item || !context_matches;
        bool do_it = context_matches;

        spinlock_unlock(&host->rrdctx.hub_queue.spinlock);

        if(context_matches) {
            bool drop = force_drop_all;
            if(!drop && apply_bounds) {
                bool queue_is_over_limit = queued > RRDCONTEXT_HUB_QUEUE_MAX_OFFLINE_ENTRIES;
                bool queue_item_expired = (now_ut > rc->queue.queued_ut) &&
                                          ((now_ut - rc->queue.queued_ut) > RRDCONTEXT_HUB_QUEUE_MAX_OFFLINE_AGE_UT);
                drop = queue_is_over_limit || queue_item_expired;
            }

            do_it = drop;
        }

        if(item)
            dictionary_acquired_item_release(host->rrdctx.contexts, item);

        spinlock_lock(&host->rrdctx.hub_queue.spinlock);
        if(unlikely(!service_running(SERVICE_CONTEXT)))
            break;

        if(stale) {
            // Revalidate after re-lock: queue may have changed while lock was dropped.
            RRDCONTEXT *rc_at_idx = RRDCONTEXT_QUEUE_GET(&host->rrdctx.hub_queue, idx);
            if(rc_at_idx == rc) {
                RRDCONTEXT_QUEUE_DEL(&host->rrdctx.hub_queue, idx);
                __atomic_add_fetch(&host->rrdctx.hub_queue.version, 1, __ATOMIC_RELAXED);
                __atomic_sub_fetch(&host->rrdctx.hub_queue.entries, 1, __ATOMIC_RELAXED);

                rc->queue.idx = 0;
                rrd_flag_clear(rc, RRD_FLAG_QUEUED_FOR_HUB);

                dropped++;
                if(queued > 0)
                    queued--;
            }
        }
        else if(do_it) {
            rrdcontext_del_from_hub_queue(rc, true);
            dropped++;
            if(queued > 0)
                queued--;
        }
    }
    spinlock_unlock(&host->rrdctx.hub_queue.spinlock);

    if(unlikely(dropped)) {
        nd_log_limit_static_global_var(erl, 1, 0);
        nd_log_limit(&erl, NDLS_DAEMON, NDLP_NOTICE,
                     "RRDCONTEXT: host '%s' pruned %zu queued context updates (force_drop_all=%s, bounded=%s)",
                     rrdhost_hostname(host), dropped,
                     force_drop_all ? "true" : "false",
                     apply_bounds ? "true" : "false");
    }
}

void rrdcontext_add_to_pp_queue(RRDCONTEXT *rc) {
    if(!rc || !rc->rrdhost) return;

    spinlock_lock(&rc->rrdhost->rrdctx.pp_queue.spinlock);

    RRDCONTEXT_QUEUE_STATUS ret = rrdcontext_queue_add(&rc->rrdhost->rrdctx.pp_queue, rc, &rc->pp.idx, true);

    if(ret == RRDCONTEXT_QUEUE_ADDED) {
        rrd_flag_set(rc, RRD_FLAG_QUEUED_FOR_PP);
        rc->pp.queued_flags = rc->flags;
        rc->pp.queued_ut = now_realtime_usec();
    }
    else if(ret == RRDCONTEXT_QUEUE_FOUND) {
        rrd_flag_set(rc, RRD_FLAG_QUEUED_FOR_PP);
        rc->pp.queued_flags |= rc->flags;
    }

    spinlock_unlock(&rc->rrdhost->rrdctx.pp_queue.spinlock);
}

static inline bool rrdcontext_queue_del(RRDCONTEXT_QUEUE_JudyLSet *queue, RRDCONTEXT *rc, Word_t *idx, bool having_lock) {
    bool ret = false;
    if(!queue || !rc || !idx) return ret;

    if(!having_lock)
        spinlock_lock(&queue->spinlock);

    RRDCONTEXT *rc_found = RRDCONTEXT_QUEUE_GET(queue, *idx);

    if(rc_found == rc) {
        RRDCONTEXT_QUEUE_DEL(queue, *idx);
        __atomic_add_fetch(&queue->version, 1, __ATOMIC_RELAXED);
        __atomic_sub_fetch(&queue->entries, 1, __ATOMIC_RELAXED);
        ret = true;
    }
    *idx = 0;

    if(!having_lock)
        spinlock_unlock(&queue->spinlock);

    return ret;
}

void rrdcontext_del_from_hub_queue(RRDCONTEXT *rc, bool having_lock) {
    if(!rc || !rc->rrdhost) return;
    if(!having_lock)
        spinlock_lock(&rc->rrdhost->rrdctx.hub_queue.spinlock);

    if(rrdcontext_queue_del(&rc->rrdhost->rrdctx.hub_queue, rc, &rc->queue.idx, true)) {
        rrd_flag_clear(rc, RRD_FLAG_QUEUED_FOR_HUB);
    }

    if(!having_lock)
        spinlock_unlock(&rc->rrdhost->rrdctx.hub_queue.spinlock);
}

void rrdcontext_del_from_pp_queue(RRDCONTEXT *rc, bool having_lock) {
    if(!rc || !rc->rrdhost) return;

    if(!having_lock)
        spinlock_lock(&rc->rrdhost->rrdctx.pp_queue.spinlock);

    if(rrdcontext_queue_del(&rc->rrdhost->rrdctx.pp_queue, rc, &rc->pp.idx, true)) {
        rrd_flag_clear(rc, RRD_FLAG_QUEUED_FOR_PP);
        rc->pp.dequeued_ut = now_realtime_usec();
    }

    if(!having_lock)
        spinlock_unlock(&rc->rrdhost->rrdctx.pp_queue.spinlock);
}


uint32_t rrdcontext_queue_version(RRDCONTEXT_QUEUE_JudyLSet *queue) {
    return __atomic_load_n(&queue->version, __ATOMIC_RELAXED);
}

int32_t rrdcontext_queue_entries(RRDCONTEXT_QUEUE_JudyLSet *queue) {
    return __atomic_load_n(&queue->entries, __ATOMIC_RELAXED);
}

void rrdcontext_post_process_queued_contexts(RRDHOST *host) {

    spinlock_lock(&host->rrdctx.pp_queue.spinlock);
    Word_t idx = 0;
    for(RRDCONTEXT *rc = RRDCONTEXT_QUEUE_FIRST(&host->rrdctx.pp_queue, &idx);
         rc;
         rc = RRDCONTEXT_QUEUE_NEXT(&host->rrdctx.pp_queue, &idx)) {
        if(unlikely(!service_running(SERVICE_CONTEXT))) break;

        const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(host->rrdctx.contexts, string2str(rc->id));
        bool do_it = dictionary_acquired_item_value(item) == rc;

        if(do_it)
            rrdcontext_del_from_pp_queue(rc, true);

        spinlock_unlock(&host->rrdctx.pp_queue.spinlock);

        if(item) {
            if (do_it)
                rrdcontext_post_process_updates(rc, false, RRD_FLAG_NONE, true);

            dictionary_acquired_item_release(host->rrdctx.contexts, item);
        }

        spinlock_lock(&host->rrdctx.pp_queue.spinlock);
    }

    spinlock_unlock(&host->rrdctx.pp_queue.spinlock);
}

void rrdcontext_dispatch_queued_contexts_to_hub(RRDHOST *host, usec_t now_ut) {
    CLAIM_ID claim_id = claim_id_get();

    if(unlikely(!claim_id_is_set(claim_id))) {
        // unclaimed agents should not retain hub-queue state, otherwise queued flags block local GC indefinitely
        rrdcontext_prune_hub_queue(host, now_ut, true, false);
        return;
    }

    // check if we have received a streaming command for this host
    if(UUIDiszero(host->node_id) || !rrdhost_flag_check(host, RRDHOST_FLAG_ACLK_STREAM_CONTEXTS) || !aclk_online_for_contexts())
    {
        // claimed but currently not dispatching: keep queue bounded to avoid indefinite retention
        rrdcontext_prune_hub_queue(host, now_ut, false, true);
        return;
    }

    // check if there are queued items to send
    if(!rrdcontext_queue_entries(&host->rrdctx.hub_queue))
        return;

    size_t messages_added = 0;
    contexts_updated_t bundle = NULL;

    spinlock_lock(&host->rrdctx.hub_queue.spinlock);
    Word_t idx = 0;
    for(RRDCONTEXT *rc = RRDCONTEXT_QUEUE_FIRST(&host->rrdctx.hub_queue, &idx);
         rc;
         rc = RRDCONTEXT_QUEUE_NEXT(&host->rrdctx.hub_queue, &idx)) {
        if(unlikely(!service_running(SERVICE_CONTEXT))) break;

        if(unlikely(messages_added >= MESSAGES_PER_BUNDLE_TO_SEND_TO_HUB_PER_HOST))
            break;

        const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(host->rrdctx.contexts, string2str(rc->id));
        bool do_it = dictionary_acquired_item_value(item) == rc;

        spinlock_unlock(&host->rrdctx.hub_queue.spinlock);

        if(item) {
            if (do_it) {
                worker_is_busy(WORKER_JOB_QUEUED);
                usec_t dispatch_ut = rrdcontext_calculate_queued_dispatch_time_ut(rc, now_ut);
                if(unlikely(now_ut >= dispatch_ut) && claim_id_is_set(claim_id)) {
                    worker_is_busy(WORKER_JOB_CHECK);

                    rrdcontext_lock(rc);

                    if(check_if_cloud_version_changed_unsafe(rc, true)) {
                        worker_is_busy(WORKER_JOB_SEND);

                        if(!bundle) {
                            // prepare the bundle to send the messages
                            char uuid_str[UUID_STR_LEN];
                            uuid_unparse_lower(host->node_id.uuid, uuid_str);

                            bundle = contexts_updated_new(claim_id.str, uuid_str, 0, now_ut);
                        }
                        // update the hub data of the context, give a new version, pack the message
                        // and save an update to SQL
                        rrdcontext_message_send_unsafe(rc, false, bundle);
                        messages_added++;

                        rc->queue.dispatches++;
                        rc->queue.dequeued_ut = now_ut;
                    }
                    else
                        rc->version = rc->hub.version;

                    if(unlikely(rrdcontext_should_be_deleted(rc))) {
                        // this is a deleted context - delete it forever...

                        worker_is_busy(WORKER_JOB_CLEANUP_DELETE);

                        rrdcontext_dequeue_from_post_processing(rc);
                        rrdcontext_delete_from_sql_unsafe(rc);

                        STRING *id = string_dup(rc->id);
                        rrdcontext_unlock(rc);

                        // delete it from the master dictionary
                        if(!dictionary_del(host->rrdctx.contexts, string2str(rc->id)))
                            netdata_log_error("RRDCONTEXT: '%s' of host '%s' failed to be deleted from rrdcontext dictionary.",
                                              string2str(id), rrdhost_hostname(host));

                        string_freez(id);
                    }
                    else
                        rrdcontext_unlock(rc);
                }
                else
                    do_it = false;
            }

            dictionary_acquired_item_release(host->rrdctx.contexts, item);
        }
        spinlock_lock(&host->rrdctx.hub_queue.spinlock);

        if(do_it) {
            worker_is_busy(WORKER_JOB_DEQUEUE);
            rrdcontext_del_from_hub_queue(rc, true);
        }
    }
    spinlock_unlock(&host->rrdctx.hub_queue.spinlock);

    if(service_running(SERVICE_CONTEXT) && bundle) {
        // we have a bundle to send messages

        // update the version hash
        contexts_updated_update_version_hash(bundle, rrdcontext_version_hash(host));

        // send it
        aclk_send_contexts_updated(bundle);
    }
    else if(bundle)
        contexts_updated_delete(bundle);
}

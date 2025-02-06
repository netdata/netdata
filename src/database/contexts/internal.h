// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDCONTEXT_INTERNAL_H
#define NETDATA_RRDCONTEXT_INTERNAL_H 1

#include "rrdcontext.h"
#include "../sqlite/sqlite_context.h"
#include "../../aclk/schema-wrappers/context.h"
#include "../../aclk/aclk_contexts_api.h"
#include "../../aclk/aclk.h"
#include "../storage-engine.h"

#define MESSAGES_PER_BUNDLE_TO_SEND_TO_HUB_PER_HOST         5000
#define FULL_RETENTION_SCAN_DELAY_AFTER_DB_ROTATION_SECS    120
#define RRDCONTEXT_WORKER_THREAD_HEARTBEAT_USEC             (1000 * USEC_PER_MS)
#define RRDCONTEXT_MINIMUM_ALLOWED_PRIORITY                 10

#define LOG_TRANSITIONS false

#define WORKER_JOB_HOSTS                 1
#define WORKER_JOB_CHECK                 2
#define WORKER_JOB_SEND                  3
#define WORKER_JOB_DEQUEUE               4
#define WORKER_JOB_RETENTION             5
#define WORKER_JOB_QUEUED                6
#define WORKER_JOB_CLEANUP               7
#define WORKER_JOB_CLEANUP_DELETE        8
#define WORKER_JOB_PP_METRIC             9 // post-processing metrics
#define WORKER_JOB_PP_INSTANCE          10 // post-processing instances
#define WORKER_JOB_PP_CONTEXT           11 // post-processing contexts
#define WORKER_JOB_HUB_QUEUE_SIZE       12
#define WORKER_JOB_PP_QUEUE_SIZE        13


typedef enum __attribute__ ((__packed__)) {
    RRD_FLAG_NONE           = 0,
    RRD_FLAG_DELETED        = (1 << 0), // this is a deleted object (metrics, instances, contexts)
    RRD_FLAG_COLLECTED      = (1 << 1), // this object is currently being collected
    RRD_FLAG_UPDATED        = (1 << 2), // this object has updates to propagate
    RRD_FLAG_ARCHIVED       = (1 << 3), // this object is not currently being collected
    RRD_FLAG_OWN_LABELS     = (1 << 4), // this instance has its own labels - not linked to an RRDSET
    RRD_FLAG_DEMAND_LABELS  = (1 << 5), // this instance should load labels on demand
    RRD_FLAG_LIVE_RETENTION = (1 << 6), // we have got live retention from the database
    RRD_FLAG_QUEUED_FOR_HUB = (1 << 7), // this context is currently queued to be dispatched to hub
    RRD_FLAG_QUEUED_FOR_PP  = (1 << 8), // this context is currently queued to be post-processed
    RRD_FLAG_HIDDEN         = (1 << 9), // don't expose this to the hub or the API

    RRD_FLAG_UPDATE_REASON_TRIGGERED               = (1 << 10), // the update was triggered by the child object
    RRD_FLAG_UPDATE_REASON_LOAD_SQL                = (1 << 11), // this object has just been loaded from SQL
    RRD_FLAG_UPDATE_REASON_NEW_OBJECT              = (1 << 12), // this object has just been created
    RRD_FLAG_UPDATE_REASON_UPDATED_OBJECT          = (1 << 13), // we received an update on this object
    RRD_FLAG_UPDATE_REASON_CHANGED_LINKING         = (1 << 14), // an instance or a metric switched RRDSET or RRDDIM
    RRD_FLAG_UPDATE_REASON_CHANGED_METADATA        = (1 << 15), // this context or instance changed uuid, name, units, title, family, chart type, priority, update every, rrd changed flags
    RRD_FLAG_UPDATE_REASON_ZERO_RETENTION          = (1 << 16), // this object has no retention
    RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T    = (1 << 17), // this object changed its oldest time in the db
    RRD_FLAG_UPDATE_REASON_CHANGED_LAST_TIME_T     = (1 << 18), // this object change its latest time in the db
    RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED = (1 << 19), // this object has stopped being collected
    RRD_FLAG_UPDATE_REASON_STARTED_BEING_COLLECTED = (1 << 20), // this object has started being collected
    RRD_FLAG_UPDATE_REASON_DISCONNECTED_CHILD      = (1 << 21), // this context belongs to a host that just disconnected
    RRD_FLAG_UPDATE_REASON_UNUSED                  = (1 << 22), // this context is not used anymore
    RRD_FLAG_UPDATE_REASON_DB_ROTATION             = (1 << 23), // this context changed because of a db rotation

    RRD_FLAG_NO_TIER0_RETENTION                    = (1 << 28),
    RRD_FLAG_MERGED_COLLECTED_RI_TO_RC             = (1 << 29),

    // action to perform on an object
    RRD_FLAG_UPDATE_REASON_UPDATE_RETENTION        = (1 << 30), // this object has to update its retention from the db
} RRD_FLAGS;

struct rrdcontext_reason {
    RRD_FLAGS flag;
    const char *name;
    usec_t delay_ut;
};

extern struct rrdcontext_reason rrdcontext_reasons[];

#define RRD_FLAG_ALL_UPDATE_REASONS                   ( \
     RRD_FLAG_UPDATE_REASON_TRIGGERED                   \
    |RRD_FLAG_UPDATE_REASON_LOAD_SQL                    \
    |RRD_FLAG_UPDATE_REASON_NEW_OBJECT                  \
    |RRD_FLAG_UPDATE_REASON_UPDATED_OBJECT              \
    |RRD_FLAG_UPDATE_REASON_CHANGED_LINKING             \
    |RRD_FLAG_UPDATE_REASON_CHANGED_METADATA            \
    |RRD_FLAG_UPDATE_REASON_ZERO_RETENTION              \
    |RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T        \
    |RRD_FLAG_UPDATE_REASON_CHANGED_LAST_TIME_T         \
    |RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED     \
    |RRD_FLAG_UPDATE_REASON_STARTED_BEING_COLLECTED     \
    |RRD_FLAG_UPDATE_REASON_DISCONNECTED_CHILD          \
    |RRD_FLAG_UPDATE_REASON_DB_ROTATION                 \
    |RRD_FLAG_UPDATE_REASON_UNUSED                      \
 )

#define RRD_FLAGS_ALLOWED_EXTERNALLY_ON_NEW_OBJECTS   ( \
     RRD_FLAG_ARCHIVED                                  \
    |RRD_FLAG_HIDDEN                                    \
    |RRD_FLAG_ALL_UPDATE_REASONS                        \
 )

#define RRD_FLAGS_REQUIRED_FOR_DELETIONS              ( \
     RRD_FLAG_DELETED                                   \
    |RRD_FLAG_LIVE_RETENTION                            \
)

#define RRD_FLAGS_PREVENTING_DELETIONS                ( \
     RRD_FLAG_QUEUED_FOR_HUB                            \
    |RRD_FLAG_COLLECTED                                 \
    |RRD_FLAG_QUEUED_FOR_PP                             \
)

// get all the flags of an object
#define rrd_flags_get(obj) __atomic_load_n(&((obj)->flags), __ATOMIC_SEQ_CST)

// check if ANY of the given flags (bits) is set
#define rrd_flag_check(obj, flag) (rrd_flags_get(obj) & (flag))

// check if ALL the given flags (bits) are set
#define rrd_flag_check_all(obj, flag) (rrd_flag_check(obj, flag) == (flag))

// set one or more flags (bits)
// NEVER alter RRD_FLAG_COLLECTED, RRD_FLAG_ARCHIVED, RRD_FLAG_DELETED with this
#define rrd_flag_set(obj, flag)   __atomic_or_fetch(&((obj)->flags), flag, __ATOMIC_SEQ_CST)

// clear one or more flags (bits)
// NEVER alter RRD_FLAG_COLLECTED, RRD_FLAG_ARCHIVED, RRD_FLAG_DELETED with this
#define rrd_flag_clear(obj, flag) __atomic_and_fetch(&((obj)->flags), ~(flag), __ATOMIC_SEQ_CST)

static ALWAYS_INLINE RRD_FLAGS
rrd_flag_add_remove_atomic(RRD_FLAGS *flags, RRD_FLAGS check, RRD_FLAGS conditionally_add, RRD_FLAGS always_remove) {
    RRD_FLAGS expected, desired;

    do {
        expected = *flags;

        desired = expected;
        desired &= ~(always_remove);

        if(!(expected & check))
            desired |= (check | conditionally_add);

    } while(!__atomic_compare_exchange_n(flags, &expected, desired, false, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST));

    return expected;
}

static ALWAYS_INLINE RRD_FLAGS
rrd_flags_replace_atomic(RRD_FLAGS *flags, RRD_FLAGS desired) {
    RRD_FLAGS expected;
    
    do {
        expected = *flags;
    } while(!__atomic_compare_exchange_n(flags, &expected, desired, false, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST));

    return expected;
}

#define rrd_flag_set_collected(obj)                                                                             \
        rrd_flag_add_remove_atomic(&((obj)->flags)                                                              \
                               /* check this flag */                                                            \
                               , RRD_FLAG_COLLECTED                                                             \
                                                                                                                \
                               /* add these flags together with the above, if the above is not already set */   \
                               , RRD_FLAG_UPDATE_REASON_STARTED_BEING_COLLECTED | RRD_FLAG_UPDATED              \
                                                                                                                \
                               /* always remove these flags */                                                  \
                               , RRD_FLAG_ARCHIVED                                                              \
                               | RRD_FLAG_DELETED                                                               \
                               | RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED                                 \
                               | RRD_FLAG_UPDATE_REASON_ZERO_RETENTION                                          \
                               | RRD_FLAG_UPDATE_REASON_DISCONNECTED_CHILD                                      \
        )

#define rrd_flag_set_archived(obj)                                                                              \
        rrd_flag_add_remove_atomic(&((obj)->flags)                                                              \
                               /* check this flag */                                                            \
                               , RRD_FLAG_ARCHIVED                                                              \
                                                                                                                \
                               /* add these flags together with the above, if the above is not already set */   \
                               , RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED | RRD_FLAG_UPDATED              \
                                                                                                                \
                               /* always remove these flags */                                                  \
                               , RRD_FLAG_COLLECTED                                                             \
                               | RRD_FLAG_DELETED                                                               \
                               | RRD_FLAG_UPDATE_REASON_STARTED_BEING_COLLECTED                                 \
                               | RRD_FLAG_UPDATE_REASON_ZERO_RETENTION                                          \
        )

#define rrd_flag_set_deleted(obj, reason)                                                                       \
        rrd_flag_add_remove_atomic(&((obj)->flags)                                                              \
                               /* check this flag */                                                            \
                               , RRD_FLAG_DELETED                                                               \
                                                                                                                \
                               /* add these flags together with the above, if the above is not already set */   \
                               , RRD_FLAG_UPDATE_REASON_ZERO_RETENTION | RRD_FLAG_UPDATED | (reason)            \
                                                                                                                \
                               /* always remove these flags */                                                  \
                               , RRD_FLAG_ARCHIVED                                                              \
                               | RRD_FLAG_COLLECTED                                                             \
        )

#define rrd_flag_is_collected(obj) rrd_flag_check(obj, RRD_FLAG_COLLECTED)
#define rrd_flag_is_archived(obj) rrd_flag_check(obj, RRD_FLAG_ARCHIVED)
#define rrd_flag_is_deleted(obj) rrd_flag_check(obj, RRD_FLAG_DELETED)
#define rrd_flag_is_updated(obj) rrd_flag_check(obj, RRD_FLAG_UPDATED)

// mark an object as updated, providing reasons (additional bits)
#define rrd_flag_set_updated(obj, reason) rrd_flag_set(obj,  RRD_FLAG_UPDATED | (reason))

// clear an object as being updated, clearing also all the reasons
#define rrd_flag_unset_updated(obj)       rrd_flag_clear(obj, RRD_FLAG_UPDATED | RRD_FLAG_ALL_UPDATE_REASONS)


typedef struct rrdmetric {
    UUIDMAP_ID uuid;
    RRD_FLAGS flags;

    STRING *id;
    STRING *name;

    RRDDIM *rrddim;

    time_t first_time_s;
    time_t last_time_s;

    struct rrdinstance *ri;
} RRDMETRIC;

typedef struct rrdinstance {
    UUIDMAP_ID uuid;
    int update_every_s;                 // data collection frequency

    RRD_FLAGS flags;                    // flags related to this instance
    uint32_t priority:24;
    RRDSET_TYPE chart_type;

    STRING *id;
    STRING *name;
    STRING *title;
    STRING *units;
    STRING *family;

    time_t first_time_s;
    time_t last_time_s;

    RRDSET *rrdset;                     // pointer to RRDSET when collected, or NULL

    RRDLABELS *rrdlabels;               // linked to RRDSET->chart_labels or own version

    struct rrdcontext *rc;
    DICTIONARY *rrdmetrics;

    struct {
        uint32_t collected_metrics_count;   // a temporary variable to detect BEGIN/END without SET
        // don't use it for other purposes
        // it goes up and then resets to zero, on every iteration
    } internal;
} RRDINSTANCE;

typedef struct rrdcontext {
    uint64_t version;

    STRING *id;
    STRING *title;
    STRING *units;
    STRING *family;
    uint32_t priority;
    RRDSET_TYPE chart_type;

    SPINLOCK spinlock;

    RRD_FLAGS flags;
    time_t first_time_s;
    time_t last_time_s;

    VERSIONED_CONTEXT_DATA hub;

    DICTIONARY *rrdinstances;
    RRDHOST *rrdhost;

    struct {
        RRD_FLAGS queued_flags;         // the last flags that triggered the post-processing
        size_t executions;              // how many times this context has been processed
        usec_t queued_ut;               // the last time this was queued
        usec_t dequeued_ut;             // the last time we sent (or deduplicated) this context
    } pp;

    struct {
        RRD_FLAGS queued_flags;         // the last flags that triggered the queueing
        size_t dispatches;              // the number of times this has been dispatched to hub
        usec_t queued_ut;               // the last time this was queued
        usec_t delay_calc_ut;           // the last time we calculated the scheduled_dispatched_ut
        usec_t scheduled_dispatch_ut;   // the time it was/is scheduled to be sent
        usec_t dequeued_ut;             // the last time we sent (or deduplicated) this context
    } queue;
} RRDCONTEXT;

// ----------------------------------------------------------------------------
// helpers for counting collected metrics, instances and contexts

static ALWAYS_INLINE void rrdmetric_set_collected(RRDMETRIC *rm) {
    RRD_FLAGS old = rrd_flag_set_collected(rm);
    if(!(old & RRD_FLAG_COLLECTED))
        __atomic_add_fetch(&rm->ri->rc->rrdhost->collected.metrics_count, 1, __ATOMIC_RELAXED);

    rm->rrddim->rrdcontexts.collected = true;
}

static ALWAYS_INLINE void rrdmetric_set_archived(RRDMETRIC *rm) {
    RRD_FLAGS old = rrd_flag_set_archived(rm);
    if(old & RRD_FLAG_COLLECTED)
        __atomic_sub_fetch(&rm->ri->rc->rrdhost->collected.metrics_count, 1, __ATOMIC_RELAXED);
}

static ALWAYS_INLINE void rrdmetric_set_deleted(RRDMETRIC *rm, RRD_FLAGS reason) {
    RRD_FLAGS old = rrd_flag_set_deleted(rm, reason);
    if(old & RRD_FLAG_COLLECTED)
        __atomic_sub_fetch(&rm->ri->rc->rrdhost->collected.metrics_count, 1, __ATOMIC_RELAXED);
}

static ALWAYS_INLINE void rrdmetric_set_deleted_overwrite(RRDMETRIC *rm, RRD_FLAGS replacement) {
    replacement &= ~(RRD_FLAG_COLLECTED|RRD_FLAG_ARCHIVED|RRD_FLAG_DELETED);
    replacement |= RRD_FLAG_DELETED;
    RRD_FLAGS old = rrd_flags_replace_atomic(&rm->flags, replacement);
    if(old & RRD_FLAG_COLLECTED)
        __atomic_sub_fetch(&rm->ri->rc->rrdhost->collected.metrics_count, 1, __ATOMIC_RELAXED);
}

static ALWAYS_INLINE void rrdinstance_set_collected(RRDINSTANCE *ri) {
    RRD_FLAGS old = rrd_flag_set_collected(ri);
    if(!(old & RRD_FLAG_COLLECTED))
        __atomic_add_fetch(&ri->rc->rrdhost->collected.instances_count, 1, __ATOMIC_RELAXED);
}

static ALWAYS_INLINE void rrdinstance_set_archived(RRDINSTANCE *ri) {
    RRD_FLAGS old = rrd_flag_set_archived(ri);
    if(old & RRD_FLAG_COLLECTED)
        __atomic_sub_fetch(&ri->rc->rrdhost->collected.instances_count, 1, __ATOMIC_RELAXED);
}

static ALWAYS_INLINE void rrdinstance_set_deleted(RRDINSTANCE *ri, RRD_FLAGS reason) {
    RRD_FLAGS old = rrd_flag_set_deleted(ri, reason);
    if(old & RRD_FLAG_COLLECTED)
        __atomic_sub_fetch(&ri->rc->rrdhost->collected.instances_count, 1, __ATOMIC_RELAXED);
}

static ALWAYS_INLINE void rrdinstance_set_deleted_overwrite(RRDINSTANCE *ri, RRD_FLAGS replacement) {
    replacement &= ~(RRD_FLAG_COLLECTED|RRD_FLAG_ARCHIVED|RRD_FLAG_DELETED);
    replacement |= RRD_FLAG_DELETED;
    RRD_FLAGS old = rrd_flags_replace_atomic(&ri->flags, replacement);
    if(old & RRD_FLAG_COLLECTED)
        __atomic_sub_fetch(&ri->rc->rrdhost->collected.instances_count, 1, __ATOMIC_RELAXED);
}

static ALWAYS_INLINE void rrdcontext_set_collected(RRDCONTEXT *rc) {
    RRD_FLAGS old = rrd_flag_set_collected(rc);
    if(!(old & RRD_FLAG_COLLECTED))
        __atomic_add_fetch(&rc->rrdhost->collected.contexts_count, 1, __ATOMIC_RELAXED);
}

static ALWAYS_INLINE void rrdcontext_set_archived(RRDCONTEXT *rc) {
    RRD_FLAGS old = rrd_flag_set_archived(rc);
    if(old & RRD_FLAG_COLLECTED)
        __atomic_sub_fetch(&rc->rrdhost->collected.contexts_count, 1, __ATOMIC_RELAXED);
}

static ALWAYS_INLINE void rrdcontext_set_deleted(RRDCONTEXT *rc, RRD_FLAGS reason) {
    RRD_FLAGS old = rrd_flag_set_deleted(rc, reason);
    if(old & RRD_FLAG_COLLECTED)
        __atomic_sub_fetch(&rc->rrdhost->collected.contexts_count, 1, __ATOMIC_RELAXED);
}

// ----------------------------------------------------------------------------
// helper one-liners for RRDMETRIC

bool rrdmetric_update_retention(RRDMETRIC *rm);

static ALWAYS_INLINE RRDMETRIC *rrdmetric_acquired_value(RRDMETRIC_ACQUIRED *rma) {
    return dictionary_acquired_item_value((DICTIONARY_ITEM *)rma);
}

static ALWAYS_INLINE RRDMETRIC_ACQUIRED *rrdmetric_acquired_dup(RRDMETRIC_ACQUIRED *rma) {
    RRDMETRIC *rm = rrdmetric_acquired_value(rma);
    return (RRDMETRIC_ACQUIRED *)dictionary_acquired_item_dup(rm->ri->rrdmetrics, (DICTIONARY_ITEM *)rma);
}

static ALWAYS_INLINE void rrdmetric_release(RRDMETRIC_ACQUIRED *rma) {
    RRDMETRIC *rm = rrdmetric_acquired_value(rma);
    dictionary_acquired_item_release(rm->ri->rrdmetrics, (DICTIONARY_ITEM *)rma);
}

void rrdmetric_rrddim_is_freed(RRDDIM *rd);
void rrdmetric_updated_rrddim_flags(RRDDIM *rd);
void rrdmetric_collected_rrddim(RRDDIM *rd);
void rrdmetric_not_collected_rrddim(RRDDIM *rd);

// ----------------------------------------------------------------------------
// helper one-liners for RRDINSTANCE

static ALWAYS_INLINE RRDINSTANCE *rrdinstance_acquired_value(RRDINSTANCE_ACQUIRED *ria) {
    return dictionary_acquired_item_value((DICTIONARY_ITEM *)ria);
}

static ALWAYS_INLINE RRDINSTANCE_ACQUIRED *rrdinstance_acquired_dup(RRDINSTANCE_ACQUIRED *ria) {
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    return (RRDINSTANCE_ACQUIRED *)dictionary_acquired_item_dup(ri->rc->rrdinstances, (DICTIONARY_ITEM *)ria);
}

static ALWAYS_INLINE void rrdinstance_release(RRDINSTANCE_ACQUIRED *ria) {
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    dictionary_acquired_item_release(ri->rc->rrdinstances, (DICTIONARY_ITEM *)ria);
}

void rrdinstance_from_rrdset(RRDSET *st);
void rrdinstance_rrdset_is_freed(RRDSET *st);
void rrdinstance_rrdset_has_updated_retention(RRDSET *st);
void rrdinstance_updated_rrdset_name(RRDSET *st);
void rrdinstance_updated_rrdset_flags_no_action(RRDINSTANCE *ri, RRDSET *st);
void rrdinstance_updated_rrdset_flags(RRDSET *st);
void rrdinstance_collected_rrdset(RRDSET *st);
void rrdinstance_rrdset_not_collected(RRDSET *st);

void rrdcontext_queue_for_post_processing(RRDCONTEXT *rc, const char *function, RRD_FLAGS flags);

// ----------------------------------------------------------------------------
// helper one-liners for RRDCONTEXT

static ALWAYS_INLINE RRDCONTEXT *rrdcontext_acquired_value(RRDCONTEXT_ACQUIRED *rca) {
    return dictionary_acquired_item_value((DICTIONARY_ITEM *)rca);
}

static ALWAYS_INLINE RRDCONTEXT_ACQUIRED *rrdcontext_acquired_dup(RRDCONTEXT_ACQUIRED *rca) {
    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);
    return (RRDCONTEXT_ACQUIRED *)dictionary_acquired_item_dup(rc->rrdhost->rrdctx.contexts, (DICTIONARY_ITEM *)rca);
}

static ALWAYS_INLINE void rrdcontext_release(RRDCONTEXT_ACQUIRED *rca) {
    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);
    dictionary_acquired_item_release(rc->rrdhost->rrdctx.contexts, (DICTIONARY_ITEM *)rca);
}

// ----------------------------------------------------------------------------
// Forward definitions
void load_instance_labels_on_demand(nd_uuid_t *uuid, void *data);

void rrdcontext_recalculate_context_retention(RRDCONTEXT *rc, RRD_FLAGS reason, bool worker_jobs);
void rrdcontext_recalculate_host_retention(RRDHOST *host, RRD_FLAGS reason, bool worker_jobs);

#define rrdcontext_lock(rc) spinlock_lock(&((rc)->spinlock))
#define rrdcontext_unlock(rc) spinlock_unlock(&((rc)->spinlock))

void rrdmetric_trigger_updates(RRDMETRIC *rm, const char *function);
void rrdinstance_trigger_updates(RRDINSTANCE *ri, const char *function);
void rrdcontext_trigger_updates(RRDCONTEXT *rc, const char *function);

void rrdinstances_create_in_rrdcontext(RRDCONTEXT *rc);
void rrdinstances_destroy_from_rrdcontext(RRDCONTEXT *rc);

void rrdmetrics_destroy_from_rrdinstance(RRDINSTANCE *ri);
void rrdmetrics_create_in_rrdinstance(RRDINSTANCE *ri);

void rrdmetric_from_rrddim(RRDDIM *rd);

void rrd_reasons_to_buffer_json_array_items(RRD_FLAGS flags, BUFFER *wb);

#define rrdcontext_version_hash(host) rrdcontext_version_hash_with_callback(host, NULL, false, NULL)
uint64_t rrdcontext_version_hash_with_callback(
        RRDHOST *host,
        void (*callback)(RRDCONTEXT *, bool, void *),
        bool snapshot,
        void *bundle);

void rrdcontext_message_send_unsafe(RRDCONTEXT *rc, bool snapshot __maybe_unused, void *bundle __maybe_unused);

void rrdcontext_update_from_collected_rrdinstance(RRDINSTANCE *ri);

void rrdcontext_garbage_collect_single_host(RRDHOST *host, bool worker_jobs);

void get_metric_retention_by_id(RRDHOST *host, UUIDMAP_ID id, time_t *min_first_time_t, time_t *max_last_time_t, bool *tier0_retention);

void rrdcontext_delete_after_loading(RRDHOST *host, RRDCONTEXT *rc);
void rrdcontext_initial_processing_after_loading(RRDCONTEXT *rc);

#endif //NETDATA_RRDCONTEXT_INTERNAL_H

// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdcontext.h"
#include "sqlite/sqlite_context.h"
#include "aclk/schema-wrappers/context.h"
#include "aclk/aclk_contexts_api.h"
#include "aclk/aclk.h"
#include "storage_engine.h"

#define MESSAGES_PER_BUNDLE_TO_SEND_TO_HUB_PER_HOST         5000
#define FULL_RETENTION_SCAN_DELAY_AFTER_DB_ROTATION_SECS    120
#define RRDCONTEXT_WORKER_THREAD_HEARTBEAT_USEC             (1000 * USEC_PER_MS)
#define RRDCONTEXT_MINIMUM_ALLOWED_PRIORITY                 10

#define LOG_TRANSITIONS false

#define WORKER_JOB_HOSTS            1
#define WORKER_JOB_CHECK            2
#define WORKER_JOB_SEND             3
#define WORKER_JOB_DEQUEUE          4
#define WORKER_JOB_RETENTION        5
#define WORKER_JOB_QUEUED           6
#define WORKER_JOB_CLEANUP          7
#define WORKER_JOB_CLEANUP_DELETE   8
#define WORKER_JOB_PP_METRIC        9 // post-processing metrics
#define WORKER_JOB_PP_INSTANCE     10 // post-processing instances
#define WORKER_JOB_PP_CONTEXT      11 // post-processing contexts
#define WORKER_JOB_HUB_QUEUE_SIZE  12
#define WORKER_JOB_PP_QUEUE_SIZE   13


typedef enum __attribute__ ((__packed__)) {
    RRD_FLAG_NONE           = 0,
    RRD_FLAG_DELETED        = (1 << 0), // this is a deleted object (metrics, instances, contexts)
    RRD_FLAG_COLLECTED      = (1 << 1), // this object is currently being collected
    RRD_FLAG_UPDATED        = (1 << 2), // this object has updates to propagate
    RRD_FLAG_ARCHIVED       = (1 << 3), // this object is not currently being collected
    RRD_FLAG_OWN_LABELS     = (1 << 4), // this instance has its own labels - not linked to an RRDSET
    RRD_FLAG_LIVE_RETENTION = (1 << 5), // we have got live retention from the database
    RRD_FLAG_QUEUED_FOR_HUB = (1 << 6), // this context is currently queued to be dispatched to hub
    RRD_FLAG_QUEUED_FOR_PP  = (1 << 7), // this context is currently queued to be post-processed
    RRD_FLAG_HIDDEN         = (1 << 8), // don't expose this to the hub or the API

    RRD_FLAG_UPDATE_REASON_TRIGGERED               = (1 << 9),  // the update was triggered by the child object
    RRD_FLAG_UPDATE_REASON_LOAD_SQL                = (1 << 10), // this object has just been loaded from SQL
    RRD_FLAG_UPDATE_REASON_NEW_OBJECT              = (1 << 11), // this object has just been created
    RRD_FLAG_UPDATE_REASON_UPDATED_OBJECT          = (1 << 12), // we received an update on this object
    RRD_FLAG_UPDATE_REASON_CHANGED_LINKING         = (1 << 13), // an instance or a metric switched RRDSET or RRDDIM
    RRD_FLAG_UPDATE_REASON_CHANGED_METADATA        = (1 << 14), // this context or instance changed uuid, name, units, title, family, chart type, priority, update every, rrd changed flags
    RRD_FLAG_UPDATE_REASON_ZERO_RETENTION          = (1 << 15), // this object has no retention
    RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T    = (1 << 16), // this object changed its oldest time in the db
    RRD_FLAG_UPDATE_REASON_CHANGED_LAST_TIME_T     = (1 << 17), // this object change its latest time in the db
    RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED = (1 << 18), // this object has stopped being collected
    RRD_FLAG_UPDATE_REASON_STARTED_BEING_COLLECTED = (1 << 19), // this object has started being collected
    RRD_FLAG_UPDATE_REASON_DISCONNECTED_CHILD      = (1 << 20), // this context belongs to a host that just disconnected
    RRD_FLAG_UPDATE_REASON_UNUSED                  = (1 << 21), // this context is not used anymore
    RRD_FLAG_UPDATE_REASON_DB_ROTATION             = (1 << 22), // this context changed because of a db rotation

    // action to perform on an object
    RRD_FLAG_UPDATE_REASON_UPDATE_RETENTION        = (1 << 30), // this object has to update its retention from the db
} RRD_FLAGS;

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
#define rrd_flag_set(obj, flag)   __atomic_or_fetch(&((obj)->flags), flag, __ATOMIC_SEQ_CST)

// clear one or more flags (bits)
#define rrd_flag_clear(obj, flag) __atomic_and_fetch(&((obj)->flags), ~(flag), __ATOMIC_SEQ_CST)

// replace the flags of an object, with the supplied ones
#define rrd_flags_replace(obj, all_flags) __atomic_store_n(&((obj)->flags), all_flags, __ATOMIC_SEQ_CST)

static inline void
rrd_flag_add_remove_atomic(RRD_FLAGS *flags, RRD_FLAGS check, RRD_FLAGS conditionally_add, RRD_FLAGS always_remove) {
    RRD_FLAGS expected, desired;

    do {
        expected = *flags;

        desired = expected;
        desired &= ~(always_remove);

        if(!(expected & check))
            desired |= (check | conditionally_add);

    } while(!__atomic_compare_exchange_n(flags, &expected, desired, false, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST));
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


static struct rrdcontext_reason {
    RRD_FLAGS flag;
    const char *name;
    usec_t delay_ut;
} rrdcontext_reasons[] = {
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


typedef struct rrdmetric {
    uuid_t uuid;

    STRING *id;
    STRING *name;

    RRDDIM *rrddim;

    time_t first_time_s;
    time_t last_time_s;
    RRD_FLAGS flags;

    struct rrdinstance *ri;
} RRDMETRIC;

typedef struct rrdinstance {
    uuid_t uuid;

    STRING *id;
    STRING *name;
    STRING *title;
    STRING *units;
    STRING *family;
    uint32_t priority;
    RRDSET_TYPE chart_type;

    RRD_FLAGS flags;                    // flags related to this instance
    time_t first_time_s;
    time_t last_time_s;

    time_t update_every_s;              // data collection frequency
    RRDSET *rrdset;                     // pointer to RRDSET when collected, or NULL

    DICTIONARY *rrdlabels;              // linked to RRDSET->chart_labels or own version

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

    RRD_FLAGS flags;
    time_t first_time_s;
    time_t last_time_s;

    VERSIONED_CONTEXT_DATA hub;

    DICTIONARY *rrdinstances;
    RRDHOST *rrdhost;

    struct {
        RRD_FLAGS queued_flags;         // the last flags that triggered the post-processing
        usec_t queued_ut;               // the last time this was queued
        usec_t dequeued_ut;             // the last time we sent (or deduplicated) this context
        size_t executions;              // how many times this context has been processed
    } pp;

    struct {
        RRD_FLAGS queued_flags;         // the last flags that triggered the queueing
        usec_t queued_ut;               // the last time this was queued
        usec_t delay_calc_ut;           // the last time we calculated the scheduled_dispatched_ut
        usec_t scheduled_dispatch_ut;   // the time it was/is scheduled to be sent
        usec_t dequeued_ut;             // the last time we sent (or deduplicated) this context
        size_t dispatches;              // the number of times this has been dispatched to hub
    } queue;

    netdata_mutex_t mutex;
} RRDCONTEXT;

// ----------------------------------------------------------------------------
// helper one-liners for RRDMETRIC

static bool rrdmetric_update_retention(RRDMETRIC *rm);

static inline RRDMETRIC *rrdmetric_acquired_value(RRDMETRIC_ACQUIRED *rma) {
    return dictionary_acquired_item_value((DICTIONARY_ITEM *)rma);
}

static inline RRDMETRIC_ACQUIRED *rrdmetric_acquired_dup(RRDMETRIC_ACQUIRED *rma) {
    RRDMETRIC *rm = rrdmetric_acquired_value(rma);
    return (RRDMETRIC_ACQUIRED *)dictionary_acquired_item_dup(rm->ri->rrdmetrics, (DICTIONARY_ITEM *)rma);
}

static inline void rrdmetric_release(RRDMETRIC_ACQUIRED *rma) {
    RRDMETRIC *rm = rrdmetric_acquired_value(rma);
    dictionary_acquired_item_release(rm->ri->rrdmetrics, (DICTIONARY_ITEM *)rma);
}

const char *rrdmetric_acquired_id(RRDMETRIC_ACQUIRED *rma) {
    RRDMETRIC *rm = rrdmetric_acquired_value(rma);
    return string2str(rm->id);
}

const char *rrdmetric_acquired_name(RRDMETRIC_ACQUIRED *rma) {
    RRDMETRIC *rm = rrdmetric_acquired_value(rma);
    return string2str(rm->name);
}

NETDATA_DOUBLE rrdmetric_acquired_last_stored_value(RRDMETRIC_ACQUIRED *rma) {
    RRDMETRIC *rm = rrdmetric_acquired_value(rma);

    if(rm->rrddim)
        return rm->rrddim->last_stored_value;

    return NAN;
}

// ----------------------------------------------------------------------------
// helper one-liners for RRDINSTANCE

static inline RRDINSTANCE *rrdinstance_acquired_value(RRDINSTANCE_ACQUIRED *ria) {
    return dictionary_acquired_item_value((DICTIONARY_ITEM *)ria);
}

static inline RRDINSTANCE_ACQUIRED *rrdinstance_acquired_dup(RRDINSTANCE_ACQUIRED *ria) {
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    return (RRDINSTANCE_ACQUIRED *)dictionary_acquired_item_dup(ri->rc->rrdinstances, (DICTIONARY_ITEM *)ria);
}

static inline void rrdinstance_release(RRDINSTANCE_ACQUIRED *ria) {
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    dictionary_acquired_item_release(ri->rc->rrdinstances, (DICTIONARY_ITEM *)ria);
}

const char *rrdinstance_acquired_id(RRDINSTANCE_ACQUIRED *ria) {
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    return string2str(ri->id);
}

const char *rrdinstance_acquired_name(RRDINSTANCE_ACQUIRED *ria) {
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    return string2str(ri->name);
}

DICTIONARY *rrdinstance_acquired_labels(RRDINSTANCE_ACQUIRED *ria) {
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    return ri->rrdlabels;
}

DICTIONARY *rrdinstance_acquired_functions(RRDINSTANCE_ACQUIRED *ria) {
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    if(!ri->rrdset) return NULL;
    return ri->rrdset->functions_view;
}

// ----------------------------------------------------------------------------
// helper one-liners for RRDCONTEXT

static inline RRDCONTEXT *rrdcontext_acquired_value(RRDCONTEXT_ACQUIRED *rca) {
    return dictionary_acquired_item_value((DICTIONARY_ITEM *)rca);
}

const char *rrdcontext_acquired_id(RRDCONTEXT_ACQUIRED *rca) {
    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);
    return string2str(rc->id);
}

static inline RRDCONTEXT_ACQUIRED *rrdcontext_acquired_dup(RRDCONTEXT_ACQUIRED *rca) {
    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);
    return (RRDCONTEXT_ACQUIRED *)dictionary_acquired_item_dup((DICTIONARY *)rc->rrdhost->rrdctx, (DICTIONARY_ITEM *)rca);
}

static inline void rrdcontext_release(RRDCONTEXT_ACQUIRED *rca) {
    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);
    dictionary_acquired_item_release((DICTIONARY *)rc->rrdhost->rrdctx, (DICTIONARY_ITEM *)rca);
}

static void rrdcontext_recalculate_context_retention(RRDCONTEXT *rc, RRD_FLAGS reason, bool worker_jobs);
static void rrdcontext_recalculate_host_retention(RRDHOST *host, RRD_FLAGS reason, bool worker_jobs);

#define rrdcontext_version_hash(host) rrdcontext_version_hash_with_callback(host, NULL, false, NULL)
static uint64_t rrdcontext_version_hash_with_callback(RRDHOST *host, void (*callback)(RRDCONTEXT *, bool, void *), bool snapshot, void *bundle);

static void rrdcontext_garbage_collect_single_host(RRDHOST *host, bool worker_jobs);
static void rrdcontext_garbage_collect_for_all_hosts(void);

#define rrdcontext_lock(rc) netdata_mutex_lock(&((rc)->mutex))
#define rrdcontext_unlock(rc) netdata_mutex_unlock(&((rc)->mutex))

// ----------------------------------------------------------------------------
// Forward definitions

static uint64_t rrdcontext_get_next_version(RRDCONTEXT *rc);
static bool check_if_cloud_version_changed_unsafe(RRDCONTEXT *rc, bool sending __maybe_unused);
static void rrdcontext_message_send_unsafe(RRDCONTEXT *rc, bool snapshot __maybe_unused, void *bundle __maybe_unused);

static void rrdcontext_delete_from_sql_unsafe(RRDCONTEXT *rc);

static void rrdcontext_dequeue_from_post_processing(RRDCONTEXT *rc);
static void rrdcontext_queue_for_post_processing(RRDCONTEXT *rc, const char *function, RRD_FLAGS flags);
static void rrdcontext_post_process_updates(RRDCONTEXT *rc, bool force, RRD_FLAGS reason, bool worker_jobs);

static void rrdmetric_trigger_updates(RRDMETRIC *rm, const char *function);
static void rrdinstance_trigger_updates(RRDINSTANCE *ri, const char *function);
static void rrdcontext_trigger_updates(RRDCONTEXT *rc, const char *function);

// ----------------------------------------------------------------------------
// visualizing flags

static void rrd_flags_to_buffer_json_array_items(RRD_FLAGS flags, BUFFER *wb) {
    if(flags & RRD_FLAG_QUEUED_FOR_HUB)
        buffer_json_add_array_item_string(wb, "QUEUED");

    if(flags & RRD_FLAG_DELETED)
        buffer_json_add_array_item_string(wb, "DELETED");

    if(flags & RRD_FLAG_COLLECTED)
        buffer_json_add_array_item_string(wb, "COLLECTED");

    if(flags & RRD_FLAG_UPDATED)
        buffer_json_add_array_item_string(wb, "UPDATED");

    if(flags & RRD_FLAG_ARCHIVED)
        buffer_json_add_array_item_string(wb, "ARCHIVED");

    if(flags & RRD_FLAG_OWN_LABELS)
        buffer_json_add_array_item_string(wb, "OWN_LABELS");

    if(flags & RRD_FLAG_LIVE_RETENTION)
        buffer_json_add_array_item_string(wb, "LIVE_RETENTION");

    if(flags & RRD_FLAG_HIDDEN)
        buffer_json_add_array_item_string(wb, "HIDDEN");

    if(flags & RRD_FLAG_QUEUED_FOR_PP)
        buffer_json_add_array_item_string(wb, "PENDING_UPDATES");
}

static void rrd_reasons_to_buffer_json_array_items(RRD_FLAGS flags, BUFFER *wb) {
    for(int i = 0, added = 0; rrdcontext_reasons[i].name ; i++) {
        if (flags & rrdcontext_reasons[i].flag) {
            buffer_json_add_array_item_string(wb, rrdcontext_reasons[i].name);
            added++;
        }
    }
}

// ----------------------------------------------------------------------------
// RRDMETRIC

// free the contents of RRDMETRIC.
// RRDMETRIC itself is managed by DICTIONARY - no need to free it here.
static void rrdmetric_free(RRDMETRIC *rm) {
    string_freez(rm->id);
    string_freez(rm->name);

    rm->id = NULL;
    rm->name = NULL;
    rm->ri = NULL;
}

// called when this rrdmetric is inserted to the rrdmetrics dictionary of a rrdinstance
// the constructor of the rrdmetric object
static void rrdmetric_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *rrdinstance) {
    RRDMETRIC *rm = value;

    // link it to its parent
    rm->ri = rrdinstance;

    // remove flags that we need to figure out at runtime
    rm->flags = rm->flags & RRD_FLAGS_ALLOWED_EXTERNALLY_ON_NEW_OBJECTS; // no need for atomics

    // signal the react callback to do the job
    rrd_flag_set_updated(rm, RRD_FLAG_UPDATE_REASON_NEW_OBJECT);
}

// called when this rrdmetric is deleted from the rrdmetrics dictionary of a rrdinstance
// the destructor of the rrdmetric object
static void rrdmetric_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *rrdinstance __maybe_unused) {
    RRDMETRIC *rm = value;

    internal_error(rm->rrddim, "RRDMETRIC: '%s' is freed but there is a RRDDIM linked to it.", string2str(rm->id));

    // free the resources
    rrdmetric_free(rm);
}

// called when the same rrdmetric is inserted again to the rrdmetrics dictionary of a rrdinstance
// while this is called, the dictionary is write locked, but there may be other users of the object
static bool rrdmetric_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *rrdinstance __maybe_unused) {
    RRDMETRIC *rm     = old_value;
    RRDMETRIC *rm_new = new_value;

    internal_error(rm->id != rm_new->id,
                   "RRDMETRIC: '%s' cannot change id to '%s'",
                   string2str(rm->id), string2str(rm_new->id));

    if(uuid_compare(rm->uuid, rm_new->uuid) != 0) {
#ifdef NETDATA_INTERNAL_CHECKS
        char uuid1[UUID_STR_LEN], uuid2[UUID_STR_LEN];
        uuid_unparse(rm->uuid, uuid1);
        uuid_unparse(rm_new->uuid, uuid2);

        time_t old_first_time_s = 0;
        time_t old_last_time_s = 0;
        if(rrdmetric_update_retention(rm)) {
            old_first_time_s = rm->first_time_s;
            old_last_time_s = rm->last_time_s;
        }

        uuid_copy(rm->uuid, rm_new->uuid);

        time_t new_first_time_s = 0;
        time_t new_last_time_s = 0;
        if(rrdmetric_update_retention(rm)) {
            new_first_time_s = rm->first_time_s;
            new_last_time_s = rm->last_time_s;
        }

        internal_error(true,
                       "RRDMETRIC: '%s' of instance '%s' of host '%s' changed UUID from '%s' (retention %ld to %ld, %ld secs) to '%s' (retention %ld to %ld, %ld secs)"
                       , string2str(rm->id)
                       , string2str(rm->ri->id)
                       , rrdhost_hostname(rm->ri->rc->rrdhost)
                       , uuid1, old_first_time_s, old_last_time_s, old_last_time_s - old_first_time_s
                       , uuid2, new_first_time_s, new_last_time_s, new_last_time_s - new_first_time_s
                       );
#else
        uuid_copy(rm->uuid, rm_new->uuid);
#endif
        rrd_flag_set_updated(rm, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
    }

    if(rm->rrddim && rm_new->rrddim && rm->rrddim != rm_new->rrddim) {
        rm->rrddim = rm_new->rrddim;
        rrd_flag_set_updated(rm, RRD_FLAG_UPDATE_REASON_CHANGED_LINKING);
    }

#ifdef NETDATA_INTERNAL_CHECKS
    if(rm->rrddim && uuid_compare(rm->uuid, rm->rrddim->metric_uuid) != 0) {
        char uuid1[UUID_STR_LEN], uuid2[UUID_STR_LEN];
        uuid_unparse(rm->uuid, uuid1);
        uuid_unparse(rm_new->uuid, uuid2);
        internal_error(true, "RRDMETRIC: '%s' is linked to RRDDIM '%s' but they have different UUIDs. RRDMETRIC has '%s', RRDDIM has '%s'", string2str(rm->id), rrddim_id(rm->rrddim), uuid1, uuid2);
    }
#endif

    if(rm->rrddim != rm_new->rrddim)
        rm->rrddim = rm_new->rrddim;

    if(rm->name != rm_new->name) {
        STRING *old = rm->name;
        rm->name = string_dup(rm_new->name);
        string_freez(old);
        rrd_flag_set_updated(rm, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
    }

    if(!rm->first_time_s || (rm_new->first_time_s && rm_new->first_time_s < rm->first_time_s)) {
        rm->first_time_s = rm_new->first_time_s;
        rrd_flag_set_updated(rm, RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T);
    }

    if(!rm->last_time_s || (rm_new->last_time_s && rm_new->last_time_s > rm->last_time_s)) {
        rm->last_time_s = rm_new->last_time_s;
        rrd_flag_set_updated(rm, RRD_FLAG_UPDATE_REASON_CHANGED_LAST_TIME_T);
    }

    rrd_flag_set(rm, rm_new->flags & RRD_FLAGS_ALLOWED_EXTERNALLY_ON_NEW_OBJECTS); // no needs for atomics on rm_new

    if(rrd_flag_is_collected(rm) && rrd_flag_is_archived(rm))
        rrd_flag_set_collected(rm);

    if(rrd_flag_check(rm, RRD_FLAG_UPDATED))
        rrd_flag_set(rm, RRD_FLAG_UPDATE_REASON_UPDATED_OBJECT);

    rrdmetric_free(rm_new);

    // the react callback will continue from here
    return rrd_flag_is_updated(rm);
}

// this is called after the insert or the conflict callbacks,
// but the dictionary is now unlocked
static void rrdmetric_react_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *rrdinstance __maybe_unused) {
    RRDMETRIC *rm = value;
    rrdmetric_trigger_updates(rm, __FUNCTION__ );
}

static void rrdmetrics_create_in_rrdinstance(RRDINSTANCE *ri) {
    if(unlikely(!ri)) return;
    if(likely(ri->rrdmetrics)) return;

    ri->rrdmetrics = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                &dictionary_stats_category_rrdcontext, sizeof(RRDMETRIC));

    dictionary_register_insert_callback(ri->rrdmetrics, rrdmetric_insert_callback, ri);
    dictionary_register_delete_callback(ri->rrdmetrics, rrdmetric_delete_callback, ri);
    dictionary_register_conflict_callback(ri->rrdmetrics, rrdmetric_conflict_callback, ri);
    dictionary_register_react_callback(ri->rrdmetrics, rrdmetric_react_callback, ri);
}

static void rrdmetrics_destroy_from_rrdinstance(RRDINSTANCE *ri) {
    if(unlikely(!ri || !ri->rrdmetrics)) return;
    dictionary_destroy(ri->rrdmetrics);
    ri->rrdmetrics = NULL;
}

// trigger post-processing of the rrdmetric, escalating changes to the rrdinstance it belongs
static void rrdmetric_trigger_updates(RRDMETRIC *rm, const char *function) {
    if(unlikely(rrd_flag_is_collected(rm)) && (!rm->rrddim || rrd_flag_check(rm, RRD_FLAG_UPDATE_REASON_DISCONNECTED_CHILD)))
            rrd_flag_set_archived(rm);

    if(rrd_flag_is_updated(rm) || !rrd_flag_check(rm, RRD_FLAG_LIVE_RETENTION)) {
        rrd_flag_set_updated(rm->ri, RRD_FLAG_UPDATE_REASON_TRIGGERED);
        rrdcontext_queue_for_post_processing(rm->ri->rc, function, rm->flags);
    }
}

// ----------------------------------------------------------------------------
// RRDMETRIC HOOKS ON RRDDIM

static inline void rrdmetric_from_rrddim(RRDDIM *rd) {
    if(unlikely(!rd->rrdset))
        fatal("RRDMETRIC: rrddim '%s' does not have a rrdset.", rrddim_id(rd));

    if(unlikely(!rd->rrdset->rrdhost))
        fatal("RRDMETRIC: rrdset '%s' does not have a rrdhost", rrdset_id(rd->rrdset));

    if(unlikely(!rd->rrdset->rrdinstance))
        fatal("RRDMETRIC: rrdset '%s' does not have a rrdinstance", rrdset_id(rd->rrdset));

    RRDINSTANCE *ri = rrdinstance_acquired_value(rd->rrdset->rrdinstance);

    RRDMETRIC trm = {
        .id = string_dup(rd->id),
        .name = string_dup(rd->name),
        .flags = RRD_FLAG_NONE, // no need for atomics
        .rrddim = rd,
    };
    uuid_copy(trm.uuid, rd->metric_uuid);

    RRDMETRIC_ACQUIRED *rma = (RRDMETRIC_ACQUIRED *)dictionary_set_and_acquire_item(ri->rrdmetrics, string2str(trm.id), &trm, sizeof(trm));

    if(rd->rrdmetric)
        rrdmetric_release(rd->rrdmetric);

    rd->rrdmetric = rma;
}

#define rrddim_get_rrdmetric(rd) rrddim_get_rrdmetric_with_trace(rd, __FUNCTION__)
static inline RRDMETRIC *rrddim_get_rrdmetric_with_trace(RRDDIM *rd, const char *function) {
    if(unlikely(!rd->rrdmetric)) {
        error("RRDMETRIC: RRDDIM '%s' is not linked to an RRDMETRIC at %s()", rrddim_id(rd), function);
        return NULL;
    }

    RRDMETRIC *rm = rrdmetric_acquired_value(rd->rrdmetric);
    if(unlikely(!rm)) {
        error("RRDMETRIC: RRDDIM '%s' lost the link to its RRDMETRIC at %s()", rrddim_id(rd), function);
        return NULL;
    }

    if(unlikely(rm->rrddim != rd))
        fatal("RRDMETRIC: '%s' is not linked to RRDDIM '%s' at %s()", string2str(rm->id), rrddim_id(rd), function);

    return rm;
}

static inline void rrdmetric_rrddim_is_freed(RRDDIM *rd) {
    RRDMETRIC *rm = rrddim_get_rrdmetric(rd);
    if(unlikely(!rm)) return;

    if(unlikely(rrd_flag_is_collected(rm)))
        rrd_flag_set_archived(rm);

    rm->rrddim = NULL;
    rrdmetric_trigger_updates(rm, __FUNCTION__ );
    rrdmetric_release(rd->rrdmetric);
    rd->rrdmetric = NULL;
}

static inline void rrdmetric_updated_rrddim_flags(RRDDIM *rd) {
    RRDMETRIC *rm = rrddim_get_rrdmetric(rd);
    if(unlikely(!rm)) return;

    if(unlikely(rrddim_flag_check(rd, RRDDIM_FLAG_ARCHIVED|RRDDIM_FLAG_OBSOLETE))) {
        if(unlikely(rrd_flag_is_collected(rm)))
            rrd_flag_set_archived(rm);
    }

    rrdmetric_trigger_updates(rm, __FUNCTION__ );
}

static inline void rrdmetric_collected_rrddim(RRDDIM *rd) {
    RRDMETRIC *rm = rrddim_get_rrdmetric(rd);
    if(unlikely(!rm)) return;

    if(unlikely(!rrd_flag_is_collected(rm)))
        rrd_flag_set_collected(rm);

    // we use this variable to detect BEGIN/END without SET
    rm->ri->internal.collected_metrics_count++;

    rrdmetric_trigger_updates(rm, __FUNCTION__ );
}

// ----------------------------------------------------------------------------
// RRDINSTANCE

static void rrdinstance_free(RRDINSTANCE *ri) {

    if(rrd_flag_check(ri, RRD_FLAG_OWN_LABELS))
        dictionary_destroy(ri->rrdlabels);

    rrdmetrics_destroy_from_rrdinstance(ri);
    string_freez(ri->id);
    string_freez(ri->name);
    string_freez(ri->title);
    string_freez(ri->units);
    string_freez(ri->family);

    ri->id = NULL;
    ri->name = NULL;
    ri->title = NULL;
    ri->units = NULL;
    ri->family = NULL;
    ri->rc = NULL;
    ri->rrdlabels = NULL;
    ri->rrdmetrics = NULL;
    ri->rrdset = NULL;
}

static void rrdinstance_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *rrdcontext) {
    RRDINSTANCE *ri = value;

    // link it to its parent
    ri->rc = rrdcontext;

    ri->flags = ri->flags & RRD_FLAGS_ALLOWED_EXTERNALLY_ON_NEW_OBJECTS; // no need for atomics

    if(!ri->name)
        ri->name = string_dup(ri->id);

    if(ri->rrdset) {
        ri->rrdlabels = ri->rrdset->rrdlabels;
        ri->flags &= ~RRD_FLAG_OWN_LABELS; // no need of atomics at the constructor
    }
    else {
        ri->rrdlabels = rrdlabels_create();
        ri->flags |= RRD_FLAG_OWN_LABELS; // no need of atomics at the constructor
    }

    if(ri->rrdset) {
        if(unlikely(rrdset_flag_check(ri->rrdset, RRDSET_FLAG_HIDDEN)))
            ri->flags |= RRD_FLAG_HIDDEN; // no need of atomics at the constructor
        else
            ri->flags &= ~RRD_FLAG_HIDDEN; // no need of atomics at the constructor
    }

    rrdmetrics_create_in_rrdinstance(ri);

    // signal the react callback to do the job
    rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_NEW_OBJECT);
}

static void rrdinstance_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *rrdcontext __maybe_unused) {
    RRDINSTANCE *ri = (RRDINSTANCE *)value;

    internal_error(ri->rrdset, "RRDINSTANCE: '%s' is freed but there is a RRDSET linked to it.", string2str(ri->id));

    rrdinstance_free(ri);
}

static bool rrdinstance_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *rrdcontext __maybe_unused) {
    RRDINSTANCE *ri     = (RRDINSTANCE *)old_value;
    RRDINSTANCE *ri_new = (RRDINSTANCE *)new_value;

    internal_error(ri->id != ri_new->id,
                   "RRDINSTANCE: '%s' cannot change id to '%s'",
                   string2str(ri->id), string2str(ri_new->id));

    if(uuid_compare(ri->uuid, ri_new->uuid) != 0) {
#ifdef NETDATA_INTERNAL_CHECKS
        char uuid1[UUID_STR_LEN], uuid2[UUID_STR_LEN];
        uuid_unparse(ri->uuid, uuid1);
        uuid_unparse(ri_new->uuid, uuid2);
        internal_error(true, "RRDINSTANCE: '%s' of host '%s' changed UUID from '%s' to '%s'",
                       string2str(ri->id), rrdhost_hostname(ri->rc->rrdhost), uuid1, uuid2);
#endif

        uuid_copy(ri->uuid, ri_new->uuid);
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
    }

    if(ri->rrdset && ri_new->rrdset && ri->rrdset != ri_new->rrdset) {
        ri->rrdset = ri_new->rrdset;
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_LINKING);
    }

#ifdef NETDATA_INTERNAL_CHECKS
    if(ri->rrdset && uuid_compare(ri->uuid, ri->rrdset->chart_uuid) != 0) {
        char uuid1[UUID_STR_LEN], uuid2[UUID_STR_LEN];
        uuid_unparse(ri->uuid, uuid1);
        uuid_unparse(ri->rrdset->chart_uuid, uuid2);
        internal_error(true, "RRDINSTANCE: '%s' is linked to RRDSET '%s' but they have different UUIDs. RRDINSTANCE has '%s', RRDSET has '%s'", string2str(ri->id), rrdset_id(ri->rrdset), uuid1, uuid2);
    }
#endif

    if(ri->name != ri_new->name) {
        STRING *old = ri->name;
        ri->name = string_dup(ri_new->name);
        string_freez(old);
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
    }

    if(ri->title != ri_new->title) {
        STRING *old = ri->title;
        ri->title = string_dup(ri_new->title);
        string_freez(old);
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
    }

    if(ri->units != ri_new->units) {
        STRING *old = ri->units;
        ri->units = string_dup(ri_new->units);
        string_freez(old);
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
    }

    if(ri->family != ri_new->family) {
        STRING *old = ri->family;
        ri->family = string_dup(ri_new->family);
        string_freez(old);
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
    }

    if(ri->chart_type != ri_new->chart_type) {
        ri->chart_type = ri_new->chart_type;
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
    }

    if(ri->priority != ri_new->priority) {
        ri->priority = ri_new->priority;
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
    }

    if(ri->update_every_s != ri_new->update_every_s) {
        ri->update_every_s = ri_new->update_every_s;
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
    }

    if(ri->rrdset != ri_new->rrdset) {
        ri->rrdset = ri_new->rrdset;

        if(ri->rrdset && rrd_flag_check(ri, RRD_FLAG_OWN_LABELS)) {
            DICTIONARY *old = ri->rrdlabels;
            ri->rrdlabels = ri->rrdset->rrdlabels;
            rrd_flag_clear(ri, RRD_FLAG_OWN_LABELS);
            rrdlabels_destroy(old);
        }
        else if(!ri->rrdset && !rrd_flag_check(ri, RRD_FLAG_OWN_LABELS)) {
            ri->rrdlabels = rrdlabels_create();
            rrd_flag_set(ri, RRD_FLAG_OWN_LABELS);
        }
    }

    if(ri->rrdset) {
        if(unlikely(rrdset_flag_check(ri->rrdset, RRDSET_FLAG_HIDDEN)))
            rrd_flag_set(ri, RRD_FLAG_HIDDEN);
        else
            rrd_flag_clear(ri, RRD_FLAG_HIDDEN);
    }

    rrd_flag_set(ri, ri_new->flags & RRD_FLAGS_ALLOWED_EXTERNALLY_ON_NEW_OBJECTS); // no need for atomics on ri_new

    if(rrd_flag_is_collected(ri) && rrd_flag_is_archived(ri))
        rrd_flag_set_collected(ri);

    if(rrd_flag_is_updated(ri))
        rrd_flag_set(ri, RRD_FLAG_UPDATE_REASON_UPDATED_OBJECT);

    // free the new one
    rrdinstance_free(ri_new);

    // the react callback will continue from here
    return rrd_flag_is_updated(ri);
}

static void rrdinstance_react_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *rrdcontext __maybe_unused) {
    RRDINSTANCE *ri = value;

    rrdinstance_trigger_updates(ri, __FUNCTION__ );
}

void rrdinstances_create_in_rrdcontext(RRDCONTEXT *rc) {
    if(unlikely(!rc || rc->rrdinstances)) return;

    rc->rrdinstances = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                  &dictionary_stats_category_rrdcontext, sizeof(RRDINSTANCE));

    dictionary_register_insert_callback(rc->rrdinstances, rrdinstance_insert_callback, rc);
    dictionary_register_delete_callback(rc->rrdinstances, rrdinstance_delete_callback, rc);
    dictionary_register_conflict_callback(rc->rrdinstances, rrdinstance_conflict_callback, rc);
    dictionary_register_react_callback(rc->rrdinstances, rrdinstance_react_callback, rc);
}

void rrdinstances_destroy_from_rrdcontext(RRDCONTEXT *rc) {
    if(unlikely(!rc || !rc->rrdinstances)) return;

    dictionary_destroy(rc->rrdinstances);
    rc->rrdinstances = NULL;
}

static void rrdinstance_trigger_updates(RRDINSTANCE *ri, const char *function) {
    RRDSET *st = ri->rrdset;

    if(likely(st)) {
        if(unlikely((unsigned int) st->priority != ri->priority)) {
            ri->priority = st->priority;
            rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
        }
        if(unlikely(st->update_every != ri->update_every_s)) {
            ri->update_every_s = st->update_every;
            rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
        }
    }
    else if(unlikely(rrd_flag_is_collected(ri))) {
        // there is no rrdset, but we have it as collected!

        rrd_flag_set_archived(ri);
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_LINKING);
    }

    if(rrd_flag_is_updated(ri) || !rrd_flag_check(ri, RRD_FLAG_LIVE_RETENTION)) {
        rrd_flag_set_updated(ri->rc, RRD_FLAG_UPDATE_REASON_TRIGGERED);
        rrdcontext_queue_for_post_processing(ri->rc, function, ri->flags);
    }
}

// ----------------------------------------------------------------------------
// RRDINSTANCE HOOKS ON RRDSET

static inline void rrdinstance_from_rrdset(RRDSET *st) {
    RRDCONTEXT trc = {
        .id = string_dup(st->context),
        .title = string_dup(st->title),
        .units = string_dup(st->units),
        .family = string_dup(st->family),
        .priority = st->priority,
        .chart_type = st->chart_type,
        .flags = RRD_FLAG_NONE, // no need for atomics
        .rrdhost = st->rrdhost,
    };

    RRDCONTEXT_ACQUIRED *rca = (RRDCONTEXT_ACQUIRED *)dictionary_set_and_acquire_item((DICTIONARY *)st->rrdhost->rrdctx, string2str(trc.id), &trc, sizeof(trc));
    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);

    RRDINSTANCE tri = {
        .id = string_dup(st->id),
        .name = string_dup(st->name),
        .units = string_dup(st->units),
        .family = string_dup(st->family),
        .title = string_dup(st->title),
        .chart_type = st->chart_type,
        .priority = st->priority,
        .update_every_s = st->update_every,
        .flags = RRD_FLAG_NONE, // no need for atomics
        .rrdset = st,
    };
    uuid_copy(tri.uuid, st->chart_uuid);

    RRDINSTANCE_ACQUIRED *ria = (RRDINSTANCE_ACQUIRED *)dictionary_set_and_acquire_item(rc->rrdinstances, string2str(tri.id), &tri, sizeof(tri));

    RRDCONTEXT_ACQUIRED *rca_old = st->rrdcontext;
    RRDINSTANCE_ACQUIRED *ria_old = st->rrdinstance;

    st->rrdcontext = rca;
    st->rrdinstance = ria;

    if(rca == rca_old) {
        rrdcontext_release(rca_old);
        rca_old = NULL;
    }

    if(ria == ria_old) {
        rrdinstance_release(ria_old);
        ria_old = NULL;
    }

    if(rca_old && ria_old) {
        // Oops! The chart changed context!

        // RRDCONTEXT *rc_old = rrdcontext_acquired_value(rca_old);
        RRDINSTANCE *ri_old = rrdinstance_acquired_value(ria_old);

        // migrate all dimensions to the new metrics
        RRDDIM *rd;
        rrddim_foreach_read(rd, st) {
            if (!rd->rrdmetric) continue;

            RRDMETRIC *rm_old = rrdmetric_acquired_value(rd->rrdmetric);
            rrd_flags_replace(rm_old, RRD_FLAG_DELETED|RRD_FLAG_UPDATED|RRD_FLAG_LIVE_RETENTION|RRD_FLAG_UPDATE_REASON_UNUSED|RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);
            rm_old->rrddim = NULL;
            rm_old->first_time_s = 0;
            rm_old->last_time_s = 0;

            rrdmetric_release(rd->rrdmetric);
            rd->rrdmetric = NULL;

            rrdmetric_from_rrddim(rd);
        }
        rrddim_foreach_done(rd);

        // mark the old instance, ready to be deleted
        if(!rrd_flag_check(ri_old, RRD_FLAG_OWN_LABELS))
            ri_old->rrdlabels = rrdlabels_create();

        rrd_flags_replace(ri_old, RRD_FLAG_OWN_LABELS|RRD_FLAG_DELETED|RRD_FLAG_UPDATED|RRD_FLAG_LIVE_RETENTION|RRD_FLAG_UPDATE_REASON_UNUSED|RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);
        ri_old->rrdset = NULL;
        ri_old->first_time_s = 0;
        ri_old->last_time_s = 0;

        rrdinstance_trigger_updates(ri_old, __FUNCTION__ );
        rrdinstance_release(ria_old);

        /*
        // trigger updates on the old context
        if(!dictionary_entries(rc_old->rrdinstances) && !dictionary_stats_referenced_items(rc_old->rrdinstances)) {
            rrdcontext_lock(rc_old);
            rc_old->flags = ((rc_old->flags & RRD_FLAG_QUEUED)?RRD_FLAG_QUEUED:RRD_FLAG_NONE)|RRD_FLAG_DELETED|RRD_FLAG_UPDATED|RRD_FLAG_LIVE_RETENTION|RRD_FLAG_UPDATE_REASON_UNUSED|RRD_FLAG_UPDATE_REASON_ZERO_RETENTION;
            rc_old->first_time_s = 0;
            rc_old->last_time_s = 0;
            rrdcontext_unlock(rc_old);
            rrdcontext_trigger_updates(rc_old, __FUNCTION__ );
        }
        else
            rrdcontext_trigger_updates(rc_old, __FUNCTION__ );
        */

        rrdcontext_release(rca_old);
        rca_old = NULL;
        ria_old = NULL;
    }

    if(rca_old || ria_old)
        fatal("RRDCONTEXT: cannot switch rrdcontext without switching rrdinstance too");
}

#define rrdset_get_rrdinstance(st) rrdset_get_rrdinstance_with_trace(st, __FUNCTION__);
static inline RRDINSTANCE *rrdset_get_rrdinstance_with_trace(RRDSET *st, const char *function) {
    if(unlikely(!st->rrdinstance)) {
        error("RRDINSTANCE: RRDSET '%s' is not linked to an RRDINSTANCE at %s()", rrdset_id(st), function);
        return NULL;
    }

    RRDINSTANCE *ri = rrdinstance_acquired_value(st->rrdinstance);
    if(unlikely(!ri)) {
        error("RRDINSTANCE: RRDSET '%s' lost its link to an RRDINSTANCE at %s()", rrdset_id(st), function);
        return NULL;
    }

    if(unlikely(ri->rrdset != st))
        fatal("RRDINSTANCE: '%s' is not linked to RRDSET '%s' at %s()", string2str(ri->id), rrdset_id(st), function);

    return ri;
}

static inline void rrdinstance_rrdset_is_freed(RRDSET *st) {
    RRDINSTANCE *ri = rrdset_get_rrdinstance(st);
    if(unlikely(!ri)) return;

    rrd_flag_set_archived(ri);

    if(!rrd_flag_check(ri, RRD_FLAG_OWN_LABELS)) {
        ri->rrdlabels = rrdlabels_create();
        rrdlabels_copy(ri->rrdlabels, st->rrdlabels);
        rrd_flag_set(ri, RRD_FLAG_OWN_LABELS);
    }

    ri->rrdset = NULL;

    rrdinstance_trigger_updates(ri, __FUNCTION__ );

    rrdinstance_release(st->rrdinstance);
    st->rrdinstance = NULL;

    rrdcontext_release(st->rrdcontext);
    st->rrdcontext = NULL;
}

static inline void rrdinstance_rrdset_has_updated_retention(RRDSET *st) {
    RRDINSTANCE *ri = rrdset_get_rrdinstance(st);
    if(unlikely(!ri)) return;

    rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_UPDATE_RETENTION);
    rrdinstance_trigger_updates(ri, __FUNCTION__ );
}

static inline void rrdinstance_updated_rrdset_name(RRDSET *st) {
    // the chart may not be initialized when this is called
    if(unlikely(!st->rrdinstance)) return;

    RRDINSTANCE *ri = rrdset_get_rrdinstance(st);
    if(unlikely(!ri)) return;

    if(st->name != ri->name) {
        STRING *old = ri->name;
        ri->name = string_dup(st->name);
        string_freez(old);

        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
        rrdinstance_trigger_updates(ri, __FUNCTION__ );
    }
}

static inline void rrdinstance_updated_rrdset_flags_no_action(RRDINSTANCE *ri, RRDSET *st) {
    if(unlikely(ri->rrdset != st))
        fatal("RRDCONTEXT: instance '%s' is not linked to chart '%s' on host '%s'",
              string2str(ri->id), rrdset_id(st), rrdhost_hostname(st->rrdhost));

    bool st_is_hidden = rrdset_flag_check(st, RRDSET_FLAG_HIDDEN);
    bool ri_is_hidden = rrd_flag_check(ri, RRD_FLAG_HIDDEN);

    if(unlikely(st_is_hidden != ri_is_hidden)) {
        if (unlikely(st_is_hidden && !ri_is_hidden))
            rrd_flag_set_updated(ri, RRD_FLAG_HIDDEN | RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);

        else if (unlikely(!st_is_hidden && ri_is_hidden)) {
            rrd_flag_clear(ri, RRD_FLAG_HIDDEN);
            rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
        }
    }
}

static inline void rrdinstance_updated_rrdset_flags(RRDSET *st) {
    RRDINSTANCE *ri = rrdset_get_rrdinstance(st);
    if(unlikely(!ri)) return;

    if(unlikely(rrdset_flag_check(st, RRDSET_FLAG_ARCHIVED|RRDSET_FLAG_OBSOLETE)))
        rrd_flag_set_archived(ri);

    rrdinstance_updated_rrdset_flags_no_action(ri, st);

    rrdinstance_trigger_updates(ri, __FUNCTION__ );
}

static inline void rrdinstance_collected_rrdset(RRDSET *st) {
    RRDINSTANCE *ri = rrdset_get_rrdinstance(st);
    if(unlikely(!ri)) return;

    rrdinstance_updated_rrdset_flags_no_action(ri, st);

    if(unlikely(ri->internal.collected_metrics_count && !rrd_flag_is_collected(ri)))
        rrd_flag_set_collected(ri);

    // we use this variable to detect BEGIN/END without SET
    ri->internal.collected_metrics_count = 0;

    rrdinstance_trigger_updates(ri, __FUNCTION__ );
}

// ----------------------------------------------------------------------------
// RRDCONTEXT

static void rrdcontext_freez(RRDCONTEXT *rc) {
    string_freez(rc->id);
    string_freez(rc->title);
    string_freez(rc->units);
    string_freez(rc->family);
}

static void rrdcontext_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *rrdhost) {
    RRDHOST *host = (RRDHOST *)rrdhost;
    RRDCONTEXT *rc = (RRDCONTEXT *)value;

    rc->rrdhost = host;
    rc->flags = rc->flags & RRD_FLAGS_ALLOWED_EXTERNALLY_ON_NEW_OBJECTS; // no need for atomics at constructor

    if(rc->hub.version) {
        // we are loading data from the SQL database

        if(rc->version)
            error("RRDCONTEXT: context '%s' is already initialized with version %"PRIu64", but it is loaded again from SQL with version %"PRIu64"", string2str(rc->id), rc->version, rc->hub.version);

        // IMPORTANT
        // replace all string pointers in rc->hub with our own versions
        // the originals are coming from a tmp allocation of sqlite

        string_freez(rc->id);
        rc->id = string_strdupz(rc->hub.id);
        rc->hub.id = string2str(rc->id);

        string_freez(rc->title);
        rc->title = string_strdupz(rc->hub.title);
        rc->hub.title = string2str(rc->title);

        string_freez(rc->units);
        rc->units = string_strdupz(rc->hub.units);
        rc->hub.units = string2str(rc->units);

        string_freez(rc->family);
        rc->family = string_strdupz(rc->hub.family);
        rc->hub.family = string2str(rc->family);

        rc->chart_type = rrdset_type_id(rc->hub.chart_type);
        rc->hub.chart_type = rrdset_type_name(rc->chart_type);

        rc->version      = rc->hub.version;
        rc->priority     = rc->hub.priority;
        rc->first_time_s = (time_t)rc->hub.first_time_s;
        rc->last_time_s  = (time_t)rc->hub.last_time_s;

        if(rc->hub.deleted || !rc->hub.first_time_s)
            rrd_flag_set_deleted(rc, RRD_FLAG_NONE);
        else {
            if (rc->last_time_s == 0)
                rrd_flag_set_collected(rc);
            else
                rrd_flag_set_archived(rc);
        }

        rc->flags |= RRD_FLAG_UPDATE_REASON_LOAD_SQL; // no need for atomics at constructor
    }
    else {
        // we are adding this context now for the first time
        rc->version = now_realtime_sec();
    }

    rrdinstances_create_in_rrdcontext(rc);
    netdata_mutex_init(&rc->mutex);

    // signal the react callback to do the job
    rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_NEW_OBJECT);
}

static void rrdcontext_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *rrdhost __maybe_unused) {

    RRDCONTEXT *rc = (RRDCONTEXT *)value;

    rrdinstances_destroy_from_rrdcontext(rc);
    netdata_mutex_destroy(&rc->mutex);
    rrdcontext_freez(rc);
}

static bool rrdcontext_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *rrdhost __maybe_unused) {
    RRDCONTEXT *rc = (RRDCONTEXT *)old_value;
    RRDCONTEXT *rc_new = (RRDCONTEXT *)new_value;

    //current rc is not archived, new_rc is archived, don't merge
    if (!rrd_flag_is_archived(rc) && rrd_flag_is_archived(rc_new)) {
        rrdcontext_freez(rc_new);
        return false;
    }

    rrdcontext_lock(rc);

    if(rc->title != rc_new->title) {
        STRING *old_title = rc->title;
        if (rrd_flag_is_archived(rc) && !rrd_flag_is_archived(rc_new))
            rc->title = string_dup(rc_new->title);
        else
            rc->title = string_2way_merge(rc->title, rc_new->title);
        string_freez(old_title);
        rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
    }

    if(rc->units != rc_new->units) {
        STRING *old_units = rc->units;
        rc->units = string_dup(rc_new->units);
        string_freez(old_units);
        rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
    }

    if(rc->family != rc_new->family) {
        STRING *old_family = rc->family;
        if (rrd_flag_is_archived(rc) && !rrd_flag_is_archived(rc_new))
            rc->family = string_dup(rc_new->family);
        else
            rc->family = string_2way_merge(rc->family, rc_new->family);
        string_freez(old_family);
        rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
    }

    if(rc->chart_type != rc_new->chart_type) {
        rc->chart_type = rc_new->chart_type;
        rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
    }

    if(rc->priority != rc_new->priority) {
        rc->priority = rc_new->priority;
        rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
    }

    rrd_flag_set(rc, rc_new->flags & RRD_FLAGS_ALLOWED_EXTERNALLY_ON_NEW_OBJECTS); // no need for atomics on rc_new

    if(rrd_flag_is_collected(rc) && rrd_flag_is_archived(rc))
        rrd_flag_set_collected(rc);

    if(rrd_flag_is_updated(rc))
        rrd_flag_set(rc, RRD_FLAG_UPDATE_REASON_UPDATED_OBJECT);

    rrdcontext_unlock(rc);

    // free the resources of the new one
    rrdcontext_freez(rc_new);

    // the react callback will continue from here
    return rrd_flag_is_updated(rc);
}

static void rrdcontext_react_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *rrdhost __maybe_unused) {
    RRDCONTEXT *rc = (RRDCONTEXT *)value;
    rrdcontext_trigger_updates(rc, __FUNCTION__ );
}

static void rrdcontext_trigger_updates(RRDCONTEXT *rc, const char *function) {
    if(rrd_flag_is_updated(rc) || !rrd_flag_check(rc, RRD_FLAG_LIVE_RETENTION))
        rrdcontext_queue_for_post_processing(rc, function, rc->flags);
}

static void rrdcontext_hub_queue_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *context, void *nothing __maybe_unused) {
    RRDCONTEXT *rc = context;
    rrd_flag_set(rc, RRD_FLAG_QUEUED_FOR_HUB);
    rc->queue.queued_ut = now_realtime_usec();
    rc->queue.queued_flags = rrd_flags_get(rc);
}

static void rrdcontext_hub_queue_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *context, void *nothing __maybe_unused) {
    RRDCONTEXT *rc = context;
    rrd_flag_clear(rc, RRD_FLAG_QUEUED_FOR_HUB);
}

static bool rrdcontext_hub_queue_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *context, void *new_context __maybe_unused, void *nothing __maybe_unused) {
    // context and new_context are the same
    // we just need to update the timings
    RRDCONTEXT *rc = context;
    rrd_flag_set(rc, RRD_FLAG_QUEUED_FOR_HUB);
    rc->queue.queued_ut = now_realtime_usec();
    rc->queue.queued_flags |= rrd_flags_get(rc);

    return true;
}

static void rrdcontext_post_processing_queue_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *context, void *nothing __maybe_unused) {
    RRDCONTEXT *rc = context;
    rrd_flag_set(rc, RRD_FLAG_QUEUED_FOR_PP);
    rc->pp.queued_flags = rc->flags;
    rc->pp.queued_ut = now_realtime_usec();
}

static void rrdcontext_post_processing_queue_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *context, void *nothing __maybe_unused) {
    RRDCONTEXT *rc = context;
    rrd_flag_clear(rc, RRD_FLAG_QUEUED_FOR_PP);
    rc->pp.dequeued_ut = now_realtime_usec();
}

static bool rrdcontext_post_processing_queue_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *context, void *new_context __maybe_unused, void *nothing __maybe_unused) {
    RRDCONTEXT *rc = context;
    bool changed = false;

    if(!(rc->flags & RRD_FLAG_QUEUED_FOR_PP)) {
        rrd_flag_set(rc, RRD_FLAG_QUEUED_FOR_PP);
        changed = true;
    }

    if(rc->pp.queued_flags != rc->flags) {
        rc->pp.queued_flags |= rc->flags;
        changed = true;
    }

    return changed;
}

void rrdhost_create_rrdcontexts(RRDHOST *host) {
    if(unlikely(!host)) return;
    if(likely(host->rrdctx)) return;

    host->rrdctx = (RRDCONTEXTS *)dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                             &dictionary_stats_category_rrdcontext, sizeof(RRDCONTEXT));

    dictionary_register_insert_callback((DICTIONARY *)host->rrdctx, rrdcontext_insert_callback, host);
    dictionary_register_delete_callback((DICTIONARY *)host->rrdctx, rrdcontext_delete_callback, host);
    dictionary_register_conflict_callback((DICTIONARY *)host->rrdctx, rrdcontext_conflict_callback, host);
    dictionary_register_react_callback((DICTIONARY *)host->rrdctx, rrdcontext_react_callback, host);

    host->rrdctx_hub_queue = (RRDCONTEXTS *)dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_VALUE_LINK_DONT_CLONE, &dictionary_stats_category_rrdcontext, 0);
    dictionary_register_insert_callback((DICTIONARY *)host->rrdctx_hub_queue, rrdcontext_hub_queue_insert_callback, NULL);
    dictionary_register_delete_callback((DICTIONARY *)host->rrdctx_hub_queue, rrdcontext_hub_queue_delete_callback, NULL);
    dictionary_register_conflict_callback((DICTIONARY *)host->rrdctx_hub_queue, rrdcontext_hub_queue_conflict_callback, NULL);

    host->rrdctx_post_processing_queue = (RRDCONTEXTS *)dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_VALUE_LINK_DONT_CLONE, &dictionary_stats_category_rrdcontext, 0);
    dictionary_register_insert_callback((DICTIONARY *)host->rrdctx_post_processing_queue, rrdcontext_post_processing_queue_insert_callback, NULL);
    dictionary_register_delete_callback((DICTIONARY *)host->rrdctx_post_processing_queue, rrdcontext_post_processing_queue_delete_callback, NULL);
    dictionary_register_conflict_callback((DICTIONARY *)host->rrdctx_post_processing_queue, rrdcontext_post_processing_queue_conflict_callback, NULL);
}

void rrdhost_destroy_rrdcontexts(RRDHOST *host) {
    if(unlikely(!host)) return;
    if(unlikely(!host->rrdctx)) return;

    DICTIONARY *old;

    if(host->rrdctx_hub_queue) {
        old = (DICTIONARY *)host->rrdctx_hub_queue;
        host->rrdctx_hub_queue = NULL;

        RRDCONTEXT *rc;
        dfe_start_write(old, rc) {
            dictionary_del(old, string2str(rc->id));
        }
        dfe_done(rc);
        dictionary_destroy(old);
    }

    if(host->rrdctx_post_processing_queue) {
        old = (DICTIONARY *)host->rrdctx_post_processing_queue;
        host->rrdctx_post_processing_queue = NULL;

        RRDCONTEXT *rc;
        dfe_start_write(old, rc) {
            dictionary_del(old, string2str(rc->id));
        }
        dfe_done(rc);
        dictionary_destroy(old);
    }

    old = (DICTIONARY *)host->rrdctx;
    host->rrdctx = NULL;
    dictionary_destroy(old);
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

int rrdcontext_find_dimension_uuid(RRDSET *st, const char *id, uuid_t *store_uuid) {
    if(!st->rrdhost) return 1;
    if(!st->context) return 2;

    RRDCONTEXT_ACQUIRED *rca = (RRDCONTEXT_ACQUIRED *)dictionary_get_and_acquire_item((DICTIONARY *)st->rrdhost->rrdctx, string2str(st->context));
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

    RRDCONTEXT_ACQUIRED *rca = (RRDCONTEXT_ACQUIRED *)dictionary_get_and_acquire_item((DICTIONARY *)st->rrdhost->rrdctx, string2str(st->context));
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

static usec_t rrdcontext_next_db_rotation_ut = 0;
void rrdcontext_db_rotation(void) {
    // called when the db rotates its database
    rrdcontext_next_db_rotation_ut = now_realtime_usec() + FULL_RETENTION_SCAN_DELAY_AFTER_DB_ROTATION_SECS * USEC_PER_SEC;
}

int rrdcontext_foreach_instance_with_rrdset_in_context(RRDHOST *host, const char *context, int (*callback)(RRDSET *st, void *data), void *data) {
    if(unlikely(!host || !context || !*context || !callback))
        return -1;

    RRDCONTEXT_ACQUIRED *rca = (RRDCONTEXT_ACQUIRED *)dictionary_get_and_acquire_item((DICTIONARY *)host->rrdctx, context);
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

    rrd_rdlock();
    rrdhost_foreach_read(host) {
        if(!host->node_id) continue;

        if(uuid_compare(uuid, *host->node_id) == 0)
            break;
    }
    rrd_unlock();

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
// web API

struct rrdcontext_to_json {
    BUFFER *wb;
    RRDCONTEXT_TO_JSON_OPTIONS options;
    time_t after;
    time_t before;
    SIMPLE_PATTERN *chart_label_key;
    SIMPLE_PATTERN *chart_labels_filter;
    SIMPLE_PATTERN *chart_dimensions;
    size_t written;
    time_t now;
    time_t combined_first_time_s;
    time_t combined_last_time_s;
    RRD_FLAGS combined_flags;
};

static inline int rrdmetric_to_json_callback(const DICTIONARY_ITEM *item, void *value, void *data) {
    const char *id = dictionary_acquired_item_name(item);
    struct rrdcontext_to_json * t = data;
    RRDMETRIC *rm = value;
    BUFFER *wb = t->wb;
    RRDCONTEXT_TO_JSON_OPTIONS options = t->options;
    time_t after = t->after;
    time_t before = t->before;

    if(unlikely(rrd_flag_is_deleted(rm) && !(options & RRDCONTEXT_OPTION_SHOW_DELETED)))
        return 0;

    if(after && (!rm->last_time_s || after > rm->last_time_s))
        return 0;

    if(before && (!rm->first_time_s || before < rm->first_time_s))
        return 0;

    if(t->chart_dimensions
        && !simple_pattern_matches(t->chart_dimensions, string2str(rm->id))
        && !simple_pattern_matches(t->chart_dimensions, string2str(rm->name)))
        return 0;

    if(t->written) {
        t->combined_first_time_s = MIN(t->combined_first_time_s, rm->first_time_s);
        t->combined_last_time_s = MAX(t->combined_last_time_s, rm->last_time_s);
        t->combined_flags |= rrd_flags_get(rm);
    }
    else {
        t->combined_first_time_s = rm->first_time_s;
        t->combined_last_time_s = rm->last_time_s;
        t->combined_flags = rrd_flags_get(rm);
    }

    buffer_json_member_add_object(wb, id);

    if(options & RRDCONTEXT_OPTION_SHOW_UUIDS) {
        char uuid[UUID_STR_LEN];
        uuid_unparse(rm->uuid, uuid);
        buffer_json_member_add_string(wb, "uuid", uuid);
    }

    buffer_json_member_add_string(wb, "name", string2str(rm->name));
    buffer_json_member_add_time_t(wb, "first_time_t", rm->first_time_s);
    buffer_json_member_add_time_t(wb, "last_time_t", rrd_flag_is_collected(rm) ? (long long)t->now : (long long)rm->last_time_s);
    buffer_json_member_add_boolean(wb, "collected", rrd_flag_is_collected(rm));

    if(options & RRDCONTEXT_OPTION_SHOW_DELETED)
        buffer_json_member_add_boolean(wb, "deleted", rrd_flag_is_deleted(rm));

    if(options & RRDCONTEXT_OPTION_SHOW_FLAGS) {
        buffer_json_member_add_array(wb, "flags");
        rrd_flags_to_buffer_json_array_items(rrd_flags_get(rm), wb);
        buffer_json_array_close(wb);
    }

    buffer_json_object_close(wb);
    t->written++;
    return 1;
}

static inline int rrdinstance_to_json_callback(const DICTIONARY_ITEM *item, void *value, void *data) {
    const char *id = dictionary_acquired_item_name(item);

    struct rrdcontext_to_json *t_parent = data;
    RRDINSTANCE *ri = value;
    BUFFER *wb = t_parent->wb;
    RRDCONTEXT_TO_JSON_OPTIONS options = t_parent->options;
    time_t after = t_parent->after;
    time_t before = t_parent->before;
    bool has_filter = t_parent->chart_label_key || t_parent->chart_labels_filter || t_parent->chart_dimensions;

    if(unlikely(rrd_flag_is_deleted(ri) && !(options & RRDCONTEXT_OPTION_SHOW_DELETED)))
        return 0;

    if(after && (!ri->last_time_s || after > ri->last_time_s))
        return 0;

    if(before && (!ri->first_time_s || before < ri->first_time_s))
        return 0;

    if(t_parent->chart_label_key && !rrdlabels_match_simple_pattern_parsed(ri->rrdlabels, t_parent->chart_label_key, '\0'))
        return 0;

    if(t_parent->chart_labels_filter && !rrdlabels_match_simple_pattern_parsed(ri->rrdlabels, t_parent->chart_labels_filter, ':'))
        return 0;

    time_t first_time_s = ri->first_time_s;
    time_t last_time_s = ri->last_time_s;
    RRD_FLAGS flags = rrd_flags_get(ri);

    BUFFER *wb_metrics = NULL;
    if(options & RRDCONTEXT_OPTION_SHOW_METRICS || t_parent->chart_dimensions) {

        wb_metrics = buffer_create(4096, &netdata_buffers_statistics.buffers_api);
        buffer_json_initialize(wb_metrics, "\"", "\"", wb->json.depth + 2, false);

        struct rrdcontext_to_json t_metrics = {
            .wb = wb_metrics,
            .options = options,
            .chart_label_key = t_parent->chart_label_key,
            .chart_labels_filter = t_parent->chart_labels_filter,
            .chart_dimensions = t_parent->chart_dimensions,
            .after = after,
            .before = before,
            .written = 0,
            .now = t_parent->now,
        };
        dictionary_walkthrough_read(ri->rrdmetrics, rrdmetric_to_json_callback, &t_metrics);

        if(has_filter && !t_metrics.written) {
            buffer_free(wb_metrics);
            return 0;
        }

        first_time_s = t_metrics.combined_first_time_s;
        last_time_s = t_metrics.combined_last_time_s;
        flags = t_metrics.combined_flags;
    }

    if(t_parent->written) {
        t_parent->combined_first_time_s = MIN(t_parent->combined_first_time_s, first_time_s);
        t_parent->combined_last_time_s = MAX(t_parent->combined_last_time_s, last_time_s);
        t_parent->combined_flags |= flags;
    }
    else {
        t_parent->combined_first_time_s = first_time_s;
        t_parent->combined_last_time_s = last_time_s;
        t_parent->combined_flags = flags;
    }

    buffer_json_member_add_object(wb, id);

    if(options & RRDCONTEXT_OPTION_SHOW_UUIDS) {
        char uuid[UUID_STR_LEN];
        uuid_unparse(ri->uuid, uuid);
        buffer_json_member_add_string(wb, "uuid", uuid);
    }

    buffer_json_member_add_string(wb, "name", string2str(ri->name));
    buffer_json_member_add_string(wb, "context", string2str(ri->rc->id));
    buffer_json_member_add_string(wb, "title", string2str(ri->title));
    buffer_json_member_add_string(wb, "units", string2str(ri->units));
    buffer_json_member_add_string(wb, "family", string2str(ri->family));
    buffer_json_member_add_string(wb, "chart_type", rrdset_type_name(ri->chart_type));
    buffer_json_member_add_uint64(wb, "priority", ri->priority);
    buffer_json_member_add_time_t(wb, "update_every", ri->update_every_s);
    buffer_json_member_add_time_t(wb, "first_time_t", first_time_s);
    buffer_json_member_add_time_t(wb, "last_time_t", (flags & RRD_FLAG_COLLECTED) ? (long long)t_parent->now : (long long)last_time_s);
    buffer_json_member_add_boolean(wb, "collected", flags & RRD_FLAG_COLLECTED);

    if(options & RRDCONTEXT_OPTION_SHOW_DELETED)
        buffer_json_member_add_boolean(wb, "deleted", rrd_flag_is_deleted(ri));

    if(options & RRDCONTEXT_OPTION_SHOW_FLAGS) {
        buffer_json_member_add_array(wb, "flags");
        rrd_flags_to_buffer_json_array_items(rrd_flags_get(ri), wb);
        buffer_json_array_close(wb);
    }

    if(options & RRDCONTEXT_OPTION_SHOW_LABELS && ri->rrdlabels && dictionary_entries(ri->rrdlabels)) {
        buffer_json_member_add_object(wb, "labels");
        rrdlabels_to_buffer_json_members(ri->rrdlabels, wb);
        buffer_json_object_close(wb);
    }

    if(wb_metrics) {
        buffer_json_member_add_object(wb, "dimensions");
        buffer_fast_strcat(wb, buffer_tostring(wb_metrics), buffer_strlen(wb_metrics));
        buffer_json_object_close(wb);

        buffer_free(wb_metrics);
    }

    buffer_json_object_close(wb);
    t_parent->written++;
    return 1;
}

static inline int rrdcontext_to_json_callback(const DICTIONARY_ITEM *item, void *value, void *data) {
    const char *id = dictionary_acquired_item_name(item);
    struct rrdcontext_to_json *t_parent = data;
    RRDCONTEXT *rc = value;
    BUFFER *wb = t_parent->wb;
    RRDCONTEXT_TO_JSON_OPTIONS options = t_parent->options;
    time_t after = t_parent->after;
    time_t before = t_parent->before;
    bool has_filter = t_parent->chart_label_key || t_parent->chart_labels_filter || t_parent->chart_dimensions;

    if(unlikely(rrd_flag_check(rc, RRD_FLAG_HIDDEN) && !(options & RRDCONTEXT_OPTION_SHOW_HIDDEN)))
        return 0;

    if(unlikely(rrd_flag_is_deleted(rc) && !(options & RRDCONTEXT_OPTION_SHOW_DELETED)))
        return 0;

    if(options & RRDCONTEXT_OPTION_DEEPSCAN)
        rrdcontext_recalculate_context_retention(rc, RRD_FLAG_NONE, false);

    if(after && (!rc->last_time_s || after > rc->last_time_s))
        return 0;

    if(before && (!rc->first_time_s || before < rc->first_time_s))
        return 0;

    time_t first_time_s = rc->first_time_s;
    time_t last_time_s = rc->last_time_s;
    RRD_FLAGS flags = rrd_flags_get(rc);

    BUFFER *wb_instances = NULL;
    if((options & (RRDCONTEXT_OPTION_SHOW_LABELS|RRDCONTEXT_OPTION_SHOW_INSTANCES|RRDCONTEXT_OPTION_SHOW_METRICS))
        || t_parent->chart_label_key
        || t_parent->chart_labels_filter
        || t_parent->chart_dimensions) {

        wb_instances = buffer_create(4096, &netdata_buffers_statistics.buffers_api);
        buffer_json_initialize(wb_instances, "\"", "\"", wb->json.depth + 2, false);

        struct rrdcontext_to_json t_instances = {
            .wb = wb_instances,
            .options = options,
            .chart_label_key = t_parent->chart_label_key,
            .chart_labels_filter = t_parent->chart_labels_filter,
            .chart_dimensions = t_parent->chart_dimensions,
            .after = after,
            .before = before,
            .written = 0,
            .now = t_parent->now,
        };
        dictionary_walkthrough_read(rc->rrdinstances, rrdinstance_to_json_callback, &t_instances);

        if(has_filter && !t_instances.written) {
            buffer_free(wb_instances);
            return 0;
        }

        first_time_s = t_instances.combined_first_time_s;
        last_time_s = t_instances.combined_last_time_s;
        flags = t_instances.combined_flags;
    }

    if(!(options & RRDCONTEXT_OPTION_SKIP_ID))
        buffer_json_member_add_object(wb, id);

    rrdcontext_lock(rc);

    buffer_json_member_add_string(wb, "title", string2str(rc->title));
    buffer_json_member_add_string(wb, "units", string2str(rc->units));
    buffer_json_member_add_string(wb, "family", string2str(rc->family));
    buffer_json_member_add_string(wb, "chart_type", rrdset_type_name(rc->chart_type));
    buffer_json_member_add_uint64(wb, "priority", rc->priority);
    buffer_json_member_add_time_t(wb, "first_time_t", first_time_s);
    buffer_json_member_add_time_t(wb, "last_time_t", (flags & RRD_FLAG_COLLECTED) ? (long long)t_parent->now : (long long)last_time_s);
    buffer_json_member_add_boolean(wb, "collected", (flags & RRD_FLAG_COLLECTED));

    if(options & RRDCONTEXT_OPTION_SHOW_DELETED)
        buffer_json_member_add_boolean(wb, "deleted", rrd_flag_is_deleted(rc));

    if(options & RRDCONTEXT_OPTION_SHOW_FLAGS) {
        buffer_json_member_add_array(wb, "flags");
        rrd_flags_to_buffer_json_array_items(rrd_flags_get(rc), wb);
        buffer_json_array_close(wb);
    }

    if(options & RRDCONTEXT_OPTION_SHOW_QUEUED) {
        buffer_json_member_add_array(wb, "queued_reasons");
        rrd_reasons_to_buffer_json_array_items(rc->queue.queued_flags, wb);
        buffer_json_array_close(wb);

        buffer_json_member_add_time_t(wb, "last_queued", (time_t)(rc->queue.queued_ut / USEC_PER_SEC));
        buffer_json_member_add_time_t(wb, "scheduled_dispatch", (time_t)(rc->queue.scheduled_dispatch_ut / USEC_PER_SEC));
        buffer_json_member_add_time_t(wb, "last_dequeued", (time_t)(rc->queue.dequeued_ut / USEC_PER_SEC));
        buffer_json_member_add_uint64(wb, "dispatches", rc->queue.dispatches);
        buffer_json_member_add_uint64(wb, "hub_version", rc->hub.version);
        buffer_json_member_add_uint64(wb, "version", rc->version);

        buffer_json_member_add_array(wb, "pp_reasons");
        rrd_reasons_to_buffer_json_array_items(rc->pp.queued_flags, wb);
        buffer_json_array_close(wb);

        buffer_json_member_add_time_t(wb, "pp_last_queued", (time_t)(rc->pp.queued_ut / USEC_PER_SEC));
        buffer_json_member_add_time_t(wb, "pp_last_dequeued", (time_t)(rc->pp.dequeued_ut / USEC_PER_SEC));
        buffer_json_member_add_uint64(wb, "pp_executed", rc->pp.executions);
    }

    rrdcontext_unlock(rc);

    if(wb_instances) {
        buffer_json_member_add_object(wb, "charts");
        buffer_fast_strcat(wb, buffer_tostring(wb_instances), buffer_strlen(wb_instances));
        buffer_json_object_close(wb);

        buffer_free(wb_instances);
    }

    if(!(options & RRDCONTEXT_OPTION_SKIP_ID))
        buffer_json_object_close(wb);

    t_parent->written++;
    return 1;
}

int rrdcontext_to_json(RRDHOST *host, BUFFER *wb, time_t after, time_t before, RRDCONTEXT_TO_JSON_OPTIONS options, const char *context, SIMPLE_PATTERN *chart_label_key, SIMPLE_PATTERN *chart_labels_filter, SIMPLE_PATTERN *chart_dimensions) {
    if(!host->rrdctx) {
        error("%s(): request for host '%s' that does not have rrdcontexts initialized.", __FUNCTION__, rrdhost_hostname(host));
        return HTTP_RESP_NOT_FOUND;
    }

    RRDCONTEXT_ACQUIRED *rca = (RRDCONTEXT_ACQUIRED *)dictionary_get_and_acquire_item((DICTIONARY *)host->rrdctx, context);
    if(!rca) return HTTP_RESP_NOT_FOUND;

    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);

    if(after != 0 && before != 0)
        rrdr_relative_window_to_absolute(&after, &before);

    buffer_json_initialize(wb, "\"", "\"", 0, true);
    struct rrdcontext_to_json t_contexts = {
        .wb = wb,
        .options = options|RRDCONTEXT_OPTION_SKIP_ID,
        .chart_label_key = chart_label_key,
        .chart_labels_filter = chart_labels_filter,
        .chart_dimensions = chart_dimensions,
        .after = after,
        .before = before,
        .written = 0,
        .now = now_realtime_sec(),
    };
    rrdcontext_to_json_callback((DICTIONARY_ITEM *)rca, rc, &t_contexts);
    buffer_json_finalize(wb);

    rrdcontext_release(rca);

    if(!t_contexts.written)
        return HTTP_RESP_NOT_FOUND;

    return HTTP_RESP_OK;
}

int rrdcontexts_to_json(RRDHOST *host, BUFFER *wb, time_t after, time_t before, RRDCONTEXT_TO_JSON_OPTIONS options, SIMPLE_PATTERN *chart_label_key, SIMPLE_PATTERN *chart_labels_filter, SIMPLE_PATTERN *chart_dimensions) {
    if(!host->rrdctx) {
        error("%s(): request for host '%s' that does not have rrdcontexts initialized.", __FUNCTION__, rrdhost_hostname(host));
        return HTTP_RESP_NOT_FOUND;
    }

    char node_uuid[UUID_STR_LEN] = "";

    if(host->node_id)
        uuid_unparse(*host->node_id, node_uuid);

    if(after != 0 && before != 0)
        rrdr_relative_window_to_absolute(&after, &before);

    buffer_json_initialize(wb, "\"", "\"", 0, true);
    buffer_json_member_add_string(wb, "hostname", rrdhost_hostname(host));
    buffer_json_member_add_string(wb, "machine_guid", host->machine_guid);
    buffer_json_member_add_string(wb, "node_id", node_uuid);
    buffer_json_member_add_string(wb, "claim_id", host->aclk_state.claimed_id ? host->aclk_state.claimed_id : "");

    if(options & RRDCONTEXT_OPTION_SHOW_LABELS) {
        buffer_json_member_add_object(wb, "host_labels");
        rrdlabels_to_buffer_json_members(host->rrdlabels, wb);
        buffer_json_object_close(wb);
    }

    buffer_json_member_add_object(wb, "contexts");
    struct rrdcontext_to_json t_contexts = {
        .wb = wb,
        .options = options,
        .chart_label_key = chart_label_key,
        .chart_labels_filter = chart_labels_filter,
        .chart_dimensions = chart_dimensions,
        .after = after,
        .before = before,
        .written = 0,
        .now = now_realtime_sec(),
    };
    dictionary_walkthrough_read((DICTIONARY *)host->rrdctx, rrdcontext_to_json_callback, &t_contexts);
    buffer_json_object_close(wb);

    buffer_json_finalize(wb);

    return HTTP_RESP_OK;
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
    if(!host || !host->rrdctx)
        return NULL;

    DICTIONARY *dict = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_DONT_OVERWRITE_VALUE, &dictionary_stats_category_rrdcontext, 0);
    dictionary_register_insert_callback(dict, metric_entry_insert_callback, NULL);
    dictionary_register_delete_callback(dict, metric_entry_delete_callback, NULL);
    dictionary_register_conflict_callback(dict, metric_entry_conflict_callback, NULL);

    RRDCONTEXT *rc;
    dfe_start_reentrant((DICTIONARY *)host->rrdctx, rc) {
        if(rrd_flag_is_deleted(rc))
            continue;

        if(contexts && !simple_pattern_matches(contexts, string2str(rc->id)))
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

// ----------------------------------------------------------------------------
// query API

typedef struct query_target_locals {
    time_t start_s;

    QUERY_TARGET *qt;

    RRDSET *st;

    const char *hosts;
    const char *contexts;
    const char *charts;
    const char *dimensions;
    const char *chart_label_key;
    const char *charts_labels_filter;

    long long after;
    long long before;
    bool match_ids;
    bool match_names;

    RRDHOST *host;
    RRDCONTEXT_ACQUIRED *rca;
    RRDINSTANCE_ACQUIRED *ria;

    size_t metrics_skipped_due_to_not_matching_timeframe;
} QUERY_TARGET_LOCALS;

static __thread QUERY_TARGET thread_query_target = {};
void query_target_release(QUERY_TARGET *qt) {
    if(unlikely(!qt)) return;
    if(unlikely(!qt->used)) return;

    simple_pattern_free(qt->hosts.pattern);
    qt->hosts.pattern = NULL;

    simple_pattern_free(qt->contexts.pattern);
    qt->contexts.pattern = NULL;

    simple_pattern_free(qt->instances.pattern);
    qt->instances.pattern = NULL;

    simple_pattern_free(qt->instances.chart_label_key_pattern);
    qt->instances.chart_label_key_pattern = NULL;

    simple_pattern_free(qt->instances.charts_labels_filter_pattern);
    qt->instances.charts_labels_filter_pattern = NULL;

    simple_pattern_free(qt->query.pattern);
    qt->query.pattern = NULL;

    // release the query
    for(size_t i = 0, used = qt->query.used; i < used ;i++) {
        string_freez(qt->query.array[i].dimension.id);
        qt->query.array[i].dimension.id = NULL;

        string_freez(qt->query.array[i].dimension.name);
        qt->query.array[i].dimension.name = NULL;

        string_freez(qt->query.array[i].chart.id);
        qt->query.array[i].chart.id = NULL;

        string_freez(qt->query.array[i].chart.name);
        qt->query.array[i].chart.name = NULL;

        // reset the plans
        for(size_t p = 0; p < qt->query.array[i].plan.used; p++) {
            internal_fatal(qt->query.array[i].plan.array[p].initialized &&
                            !qt->query.array[i].plan.array[p].finalized,
                           "QUERY: left-over initialized plan");

            qt->query.array[i].plan.array[p].initialized = false;
            qt->query.array[i].plan.array[p].finalized = false;
        }
        qt->query.array[i].plan.used = 0;

        // reset the tiers
        for(size_t tier = 0; tier < storage_tiers ;tier++) {
            if(qt->query.array[i].tiers[tier].db_metric_handle) {
                STORAGE_ENGINE *eng = qt->query.array[i].tiers[tier].eng;
                eng->api.metric_release(qt->query.array[i].tiers[tier].db_metric_handle);
                qt->query.array[i].tiers[tier].db_metric_handle = NULL;
                qt->query.array[i].tiers[tier].weight = 0;
                qt->query.array[i].tiers[tier].eng = NULL;
            }
        }
    }

    // release the metrics
    for(size_t i = 0, used = qt->metrics.used; i < used ;i++) {
        rrdmetric_release(qt->metrics.array[i]);
        qt->metrics.array[i] = NULL;
    }

    // release the instances
    for(size_t i = 0, used = qt->instances.used; i < used ;i++) {
        rrdinstance_release(qt->instances.array[i]);
        qt->instances.array[i] = NULL;
    }

    // release the contexts
    for(size_t i = 0, used = qt->contexts.used; i < used ;i++) {
        rrdcontext_release(qt->contexts.array[i]);
        qt->contexts.array[i] = NULL;
    }

    // release the hosts
    for(size_t i = 0, used = qt->hosts.used; i < used ;i++) {
        qt->hosts.array[i] = NULL;
    }

    qt->query.used = 0;
    qt->metrics.used = 0;
    qt->instances.used = 0;
    qt->contexts.used = 0;
    qt->hosts.used = 0;

    qt->db.minimum_latest_update_every_s = 0;
    qt->db.first_time_s = 0;
    qt->db.last_time_s = 0;

    qt->id[0] = '\0';

    qt->used = false;
}
void query_target_free(void) {
    QUERY_TARGET *qt = &thread_query_target;

    if(qt->used)
        query_target_release(qt);

    __atomic_sub_fetch(&netdata_buffers_statistics.query_targets_size, qt->query.size * sizeof(QUERY_METRIC), __ATOMIC_RELAXED);
    freez(qt->query.array);
    qt->query.array = NULL;
    qt->query.size = 0;

    __atomic_sub_fetch(&netdata_buffers_statistics.query_targets_size, qt->metrics.size * sizeof(RRDMETRIC_ACQUIRED *), __ATOMIC_RELAXED);
    freez(qt->metrics.array);
    qt->metrics.array = NULL;
    qt->metrics.size = 0;

    __atomic_sub_fetch(&netdata_buffers_statistics.query_targets_size, qt->instances.size * sizeof(RRDINSTANCE_ACQUIRED *), __ATOMIC_RELAXED);
    freez(qt->instances.array);
    qt->instances.array = NULL;
    qt->instances.size = 0;

    __atomic_sub_fetch(&netdata_buffers_statistics.query_targets_size, qt->contexts.size * sizeof(RRDCONTEXT_ACQUIRED *), __ATOMIC_RELAXED);
    freez(qt->contexts.array);
    qt->contexts.array = NULL;
    qt->contexts.size = 0;

    __atomic_sub_fetch(&netdata_buffers_statistics.query_targets_size, qt->hosts.size * sizeof(RRDHOST *), __ATOMIC_RELAXED);
    freez(qt->hosts.array);
    qt->hosts.array = NULL;
    qt->hosts.size = 0;
}

static void query_target_add_metric(QUERY_TARGET_LOCALS *qtl, RRDMETRIC_ACQUIRED *rma, RRDINSTANCE *ri,
                                    bool queryable_instance) {
    QUERY_TARGET *qt = qtl->qt;

    RRDMETRIC *rm = rrdmetric_acquired_value(rma);
    if(rrd_flag_is_deleted(rm))
        return;

    if(qt->metrics.used == qt->metrics.size) {
        size_t old_mem = qt->metrics.size * sizeof(RRDMETRIC_ACQUIRED *);
        qt->metrics.size = (qt->metrics.size) ? qt->metrics.size * 2 : 1;
        size_t new_mem = qt->metrics.size * sizeof(RRDMETRIC_ACQUIRED *);
        qt->metrics.array = reallocz(qt->metrics.array, new_mem);

        __atomic_add_fetch(&netdata_buffers_statistics.query_targets_size, new_mem - old_mem, __ATOMIC_RELAXED);
    }
    qt->metrics.array[qt->metrics.used++] = rrdmetric_acquired_dup(rma);

    if(!queryable_instance)
        return;

    time_t common_first_time_s = 0;
    time_t common_last_time_s = 0;
    time_t common_update_every_s = 0;
    size_t tiers_added = 0;
    struct {
        STORAGE_ENGINE *eng;
        STORAGE_METRIC_HANDLE *db_metric_handle;
        time_t db_first_time_s;
        time_t db_last_time_s;
        time_t db_update_every_s;
    } tier_retention[storage_tiers];

    for (size_t tier = 0; tier < storage_tiers; tier++) {
        STORAGE_ENGINE *eng = qtl->host->db[tier].eng;
        tier_retention[tier].eng = eng;
        tier_retention[tier].db_update_every_s = (time_t) (qtl->host->db[tier].tier_grouping * ri->update_every_s);

        if(rm->rrddim && rm->rrddim->tiers[tier].db_metric_handle)
            tier_retention[tier].db_metric_handle = eng->api.metric_dup(rm->rrddim->tiers[tier].db_metric_handle);
        else
            tier_retention[tier].db_metric_handle = eng->api.metric_get(qtl->host->db[tier].instance, &rm->uuid);

        if(tier_retention[tier].db_metric_handle) {
            tier_retention[tier].db_first_time_s = tier_retention[tier].eng->api.query_ops.oldest_time_s(tier_retention[tier].db_metric_handle);
            tier_retention[tier].db_last_time_s = tier_retention[tier].eng->api.query_ops.latest_time_s(tier_retention[tier].db_metric_handle);

            if(!common_first_time_s)
                common_first_time_s = tier_retention[tier].db_first_time_s;
            else if(tier_retention[tier].db_first_time_s)
                common_first_time_s = MIN(common_first_time_s, tier_retention[tier].db_first_time_s);

            if(!common_last_time_s)
                common_last_time_s = tier_retention[tier].db_last_time_s;
            else
                common_last_time_s = MAX(common_last_time_s, tier_retention[tier].db_last_time_s);

            if(!common_update_every_s)
                common_update_every_s = tier_retention[tier].db_update_every_s;
            else if(tier_retention[tier].db_update_every_s)
                common_update_every_s = MIN(common_update_every_s, tier_retention[tier].db_update_every_s);

            tiers_added++;
        }
        else {
            tier_retention[tier].db_first_time_s = 0;
            tier_retention[tier].db_last_time_s = 0;
            tier_retention[tier].db_update_every_s = 0;
        }
    }

    bool release_retention = true;
    bool timeframe_matches =
            (tiers_added
            && (common_first_time_s - common_update_every_s * 2) <= qt->window.before
            && (common_last_time_s + common_update_every_s * 2) >= qt->window.after
            ) ? true : false;

    if(timeframe_matches) {
        RRDR_DIMENSION_FLAGS options = RRDR_DIMENSION_DEFAULT;

        if (rrd_flag_check(rm, RRD_FLAG_HIDDEN)
            || (rm->rrddim && rrddim_option_check(rm->rrddim, RRDDIM_OPTION_HIDDEN))) {
            options |= RRDR_DIMENSION_HIDDEN;
            options &= ~RRDR_DIMENSION_QUERIED;
        }

        if (qt->query.pattern) {
            // we have a dimensions pattern
            // lets see if this dimension is selected

            if ((qtl->match_ids   && simple_pattern_matches(qt->query.pattern, string2str(rm->id)))
             || (qtl->match_names && simple_pattern_matches(qt->query.pattern, string2str(rm->name)))
                    ) {
                // it matches the pattern
                options |= (RRDR_DIMENSION_QUERIED | RRDR_DIMENSION_NONZERO);
                options &= ~RRDR_DIMENSION_HIDDEN;
            }
            else {
                // it does not match the pattern
                options |= RRDR_DIMENSION_HIDDEN;
                options &= ~RRDR_DIMENSION_QUERIED;
            }
        }
        else {
            // we don't have a dimensions pattern
            // so this is a selected dimension
            // if it is not hidden
            if(!(options & RRDR_DIMENSION_HIDDEN))
                options |= RRDR_DIMENSION_QUERIED;
        }

        if((options & RRDR_DIMENSION_HIDDEN) && (options & RRDR_DIMENSION_QUERIED))
            options &= ~RRDR_DIMENSION_HIDDEN;

        if(!(options & RRDR_DIMENSION_HIDDEN) || (qt->request.options & RRDR_OPTION_PERCENTAGE)) {
            // we have a non-hidden dimension
            // let's add it to the query metrics

            if(ri->rrdset)
                ri->rrdset->last_accessed_time_s = qtl->start_s;

            if (qt->query.used == qt->query.size) {
                size_t old_mem = qt->query.size * sizeof(QUERY_METRIC);
                qt->query.size = (qt->query.size) ? qt->query.size * 2 : 1;
                size_t new_mem = qt->query.size * sizeof(QUERY_METRIC);
                qt->query.array = reallocz(qt->query.array, new_mem);

                __atomic_add_fetch(&netdata_buffers_statistics.query_targets_size, new_mem - old_mem, __ATOMIC_RELAXED);
            }
            QUERY_METRIC *qm = &qt->query.array[qt->query.used++];

            qm->plan.used = 0;
            qm->dimension.options = options;

            qm->link.host = qtl->host;
            qm->link.rca = qtl->rca;
            qm->link.ria = qtl->ria;
            qm->link.rma = rma;

            qm->chart.id = string_dup(ri->id);
            qm->chart.name = string_dup(ri->name);

            qm->dimension.id = string_dup(rm->id);
            qm->dimension.name = string_dup(rm->name);

            if (!qt->db.first_time_s || common_first_time_s < qt->db.first_time_s)
                qt->db.first_time_s = common_first_time_s;

            if (!qt->db.last_time_s || common_last_time_s > qt->db.last_time_s)
                qt->db.last_time_s = common_last_time_s;

            for (size_t tier = 0; tier < storage_tiers; tier++) {
                qm->tiers[tier].eng = tier_retention[tier].eng;
                qm->tiers[tier].db_metric_handle = tier_retention[tier].db_metric_handle;
                qm->tiers[tier].db_first_time_s = tier_retention[tier].db_first_time_s;
                qm->tiers[tier].db_last_time_s = tier_retention[tier].db_last_time_s;
                qm->tiers[tier].db_update_every_s = tier_retention[tier].db_update_every_s;
            }
            release_retention = false;
        }
    }
    else
        qtl->metrics_skipped_due_to_not_matching_timeframe++;

    if(release_retention) {
        // cleanup anything we allocated to the retention we will not use
        for(size_t tier = 0; tier < storage_tiers ;tier++) {
            if (tier_retention[tier].db_metric_handle)
                tier_retention[tier].eng->api.metric_release(tier_retention[tier].db_metric_handle);
        }
    }
}

static void query_target_add_instance(QUERY_TARGET_LOCALS *qtl, RRDINSTANCE_ACQUIRED *ria, bool queryable_instance) {
    QUERY_TARGET *qt = qtl->qt;

    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    if(rrd_flag_is_deleted(ri))
        return;

    if(qt->instances.used == qt->instances.size) {
        size_t old_mem = qt->instances.size * sizeof(RRDINSTANCE_ACQUIRED *);
        qt->instances.size = (qt->instances.size) ? qt->instances.size * 2 : 1;
        size_t new_mem = qt->instances.size * sizeof(RRDINSTANCE_ACQUIRED *);
        qt->instances.array = reallocz(qt->instances.array, new_mem);

        __atomic_add_fetch(&netdata_buffers_statistics.query_targets_size, new_mem - old_mem, __ATOMIC_RELAXED);
    }

    qtl->ria = qt->instances.array[qt->instances.used++] = rrdinstance_acquired_dup(ria);

    if(qt->db.minimum_latest_update_every_s == 0 || ri->update_every_s < qt->db.minimum_latest_update_every_s)
        qt->db.minimum_latest_update_every_s = ri->update_every_s;

    if(queryable_instance) {
        if ((qt->instances.chart_label_key_pattern && !rrdlabels_match_simple_pattern_parsed(ri->rrdlabels, qt->instances.chart_label_key_pattern, ':')) ||
            (qt->instances.charts_labels_filter_pattern && !rrdlabels_match_simple_pattern_parsed(ri->rrdlabels, qt->instances.charts_labels_filter_pattern, ':')))
            queryable_instance = false;
    }

    size_t added = 0;

    if(unlikely(qt->request.rma)) {
        query_target_add_metric(qtl, qt->request.rma, ri, queryable_instance);
        added++;
    }
    else {
        RRDMETRIC *rm;
        dfe_start_read(ri->rrdmetrics, rm) {
            query_target_add_metric(qtl, (RRDMETRIC_ACQUIRED *) rm_dfe.item, ri, queryable_instance);
            added++;
        }
        dfe_done(rm);
    }

    if(!added) {
        qt->instances.used--;
        rrdinstance_release(ria);
    }
}

static void query_target_add_context(QUERY_TARGET_LOCALS *qtl, RRDCONTEXT_ACQUIRED *rca) {
    QUERY_TARGET *qt = qtl->qt;

    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);
    if(rrd_flag_is_deleted(rc))
        return;

    if(qt->contexts.used == qt->contexts.size) {
        size_t old_mem = qt->contexts.size * sizeof(RRDCONTEXT_ACQUIRED *);
        qt->contexts.size = (qt->contexts.size) ? qt->contexts.size * 2 : 1;
        size_t new_mem = qt->contexts.size * sizeof(RRDCONTEXT_ACQUIRED *);
        qt->contexts.array = reallocz(qt->contexts.array, new_mem);

        __atomic_add_fetch(&netdata_buffers_statistics.query_targets_size, new_mem - old_mem, __ATOMIC_RELAXED);
    }
    qtl->rca = qt->contexts.array[qt->contexts.used++] = rrdcontext_acquired_dup(rca);

    size_t added = 0;
    if(unlikely(qt->request.ria)) {
        query_target_add_instance(qtl, qt->request.ria, true);
        added++;
    }
    else if(unlikely(qtl->st && qtl->st->rrdcontext == rca && qtl->st->rrdinstance)) {
        query_target_add_instance(qtl, qtl->st->rrdinstance, true);
        added++;
    }
    else {
        RRDINSTANCE *ri;
        dfe_start_read(rc->rrdinstances, ri) {
            bool queryable_instance = false;
            if(!qt->instances.pattern
                || (qtl->match_ids   && simple_pattern_matches(qt->instances.pattern, string2str(ri->id)))
                || (qtl->match_names && simple_pattern_matches(qt->instances.pattern, string2str(ri->name)))
                )
                queryable_instance = true;

            query_target_add_instance(qtl, (RRDINSTANCE_ACQUIRED *)ri_dfe.item, queryable_instance);
            added++;
        }
        dfe_done(ri);
    }

    if(!added) {
        qt->contexts.used--;
        rrdcontext_release(rca);
    }
}

static void query_target_add_host(QUERY_TARGET_LOCALS *qtl, RRDHOST *host) {
    QUERY_TARGET *qt = qtl->qt;

    if(qt->hosts.used == qt->hosts.size) {
        size_t old_mem = qt->hosts.size * sizeof(RRDHOST *);
        qt->hosts.size = (qt->hosts.size) ? qt->hosts.size * 2 : 1;
        size_t new_mem = qt->hosts.size * sizeof(RRDHOST *);
        qt->hosts.array = reallocz(qt->hosts.array, new_mem);

        __atomic_add_fetch(&netdata_buffers_statistics.query_targets_size, new_mem - old_mem, __ATOMIC_RELAXED);
    }
    qtl->host = qt->hosts.array[qt->hosts.used++] = host;

    // is the chart given valid?
    if(unlikely(qtl->st && (!qtl->st->rrdinstance || !qtl->st->rrdcontext))) {
        error("QUERY TARGET: RRDSET '%s' given, because it is not linked to rrdcontext structures. Switching to context query.", rrdset_name(qtl->st));

        if(!is_valid_sp(qtl->charts))
            qtl->charts = rrdset_name(qtl->st);

        qtl->st = NULL;
    }

    size_t added = 0;
    if(unlikely(qt->request.rca)) {
        query_target_add_context(qtl, qt->request.rca);
        added++;
    }
    else if(unlikely(qtl->st)) {
        // single chart data queries
        query_target_add_context(qtl, qtl->st->rrdcontext);
        added++;
    }
    else {
        // context pattern queries
        RRDCONTEXT_ACQUIRED *rca = (RRDCONTEXT_ACQUIRED *)dictionary_get_and_acquire_item((DICTIONARY *)qtl->host->rrdctx, qtl->contexts);
        if(likely(rca)) {
            // we found it!
            query_target_add_context(qtl, rca);
            rrdcontext_release(rca);
            added++;
        }
        else {
            // Probably it is a pattern, we need to search for it...
            RRDCONTEXT *rc;
            dfe_start_read((DICTIONARY *)qtl->host->rrdctx, rc) {
                if(qt->contexts.pattern && !simple_pattern_matches(qt->contexts.pattern, string2str(rc->id)))
                    continue;

                query_target_add_context(qtl, (RRDCONTEXT_ACQUIRED *)rc_dfe.item);
                added++;
            }
            dfe_done(rc);
        }
    }

    if(!added) {
        qt->hosts.used--;
    }
}

void query_target_generate_name(QUERY_TARGET *qt) {
    char options_buffer[100 + 1];
    web_client_api_request_v1_data_options_to_string(options_buffer, 100, qt->request.options);

    char resampling_buffer[20 + 1] = "";
    if(qt->request.resampling_time > 1)
        snprintfz(resampling_buffer, 20, "/resampling:%lld", (long long)qt->request.resampling_time);

    char tier_buffer[20 + 1] = "";
    if(qt->request.options & RRDR_OPTION_SELECTED_TIER)
        snprintfz(tier_buffer, 20, "/tier:%zu", qt->request.tier);

    if(qt->request.st)
        snprintfz(qt->id, MAX_QUERY_TARGET_ID_LENGTH, "chart://host:%s/instance:%s/dimensions:%s/after:%lld/before:%lld/points:%zu/group:%s%s/options:%s%s%s"
                  , rrdhost_hostname(qt->request.st->rrdhost)
                  , rrdset_name(qt->request.st)
                  , (qt->request.dimensions) ? qt->request.dimensions : "*"
                  , (long long)qt->request.after
                  , (long long)qt->request.before
                  , qt->request.points
                  , time_grouping_tostring(qt->request.time_group_method)
                  , qt->request.time_group_options ? qt->request.time_group_options : ""
                  , options_buffer
                  , resampling_buffer
                  , tier_buffer
                  );
    else if(qt->request.host && qt->request.rca && qt->request.ria && qt->request.rma)
        snprintfz(qt->id, MAX_QUERY_TARGET_ID_LENGTH, "metric://host:%s/context:%s/instance:%s/dimension:%s/after:%lld/before:%lld/points:%zu/group:%s%s/options:%s%s%s"
                , rrdhost_hostname(qt->request.host)
                , rrdcontext_acquired_id(qt->request.rca)
                , rrdinstance_acquired_id(qt->request.ria)
                , rrdmetric_acquired_id(qt->request.rma)
                , (long long)qt->request.after
                , (long long)qt->request.before
                , qt->request.points
                , time_grouping_tostring(qt->request.time_group_method)
                , qt->request.time_group_options ? qt->request.time_group_options : ""
                , options_buffer
                , resampling_buffer
                , tier_buffer
                );
    else
        snprintfz(qt->id, MAX_QUERY_TARGET_ID_LENGTH, "context://host:%s/contexts:%s/instances:%s/dimensions:%s/after:%lld/before:%lld/points:%zu/group:%s%s/options:%s%s%s"
                , (qt->request.host) ? rrdhost_hostname(qt->request.host) : ((qt->request.hosts) ? qt->request.hosts : "*")
                , (qt->request.contexts) ? qt->request.contexts : "*"
                , (qt->request.charts) ? qt->request.charts : "*"
                , (qt->request.dimensions) ? qt->request.dimensions : "*"
                , (long long)qt->request.after
                , (long long)qt->request.before
                , qt->request.points
                , time_grouping_tostring(qt->request.time_group_method)
                , qt->request.time_group_options ? qt->request.time_group_options : ""
                , options_buffer
                , resampling_buffer
                , tier_buffer
                );

    json_fix_string(qt->id);
}

QUERY_TARGET *query_target_create(QUERY_TARGET_REQUEST *qtr) {
    if(!service_running(ABILITY_DATA_QUERIES))
        return NULL;

    QUERY_TARGET *qt = &thread_query_target;

    if(qt->used)
        fatal("QUERY TARGET: this query target is already used (%zu queries made with this QUERY_TARGET so far).", qt->queries);

    qt->used = true;
    qt->queries++;

    // copy the request into query_thread_target
    qt->request = *qtr;

    query_target_generate_name(qt);
    qt->window.after = qt->request.after;
    qt->window.before = qt->request.before;
    rrdr_relative_window_to_absolute(&qt->window.after, &qt->window.before);

    // prepare our local variables - we need these across all these functions
    QUERY_TARGET_LOCALS qtl = {
        .qt = qt,
        .start_s = now_realtime_sec(),
        .host = qt->request.host,
        .st = qt->request.st,
        .hosts = qt->request.hosts,
        .contexts = qt->request.contexts,
        .charts = qt->request.charts,
        .dimensions = qt->request.dimensions,
        .chart_label_key = qt->request.chart_label_key,
        .charts_labels_filter = qt->request.charts_labels_filter,
    };

    qt->db.minimum_latest_update_every_s = 0; // it will be updated by query_target_add_query()

    // prepare all the patterns
    qt->hosts.pattern = is_valid_sp(qtl.hosts) ? simple_pattern_create(qtl.hosts, ",|\t\r\n\f\v", SIMPLE_PATTERN_EXACT) : NULL;
    qt->contexts.pattern = is_valid_sp(qtl.contexts) ? simple_pattern_create(qtl.contexts, ",|\t\r\n\f\v", SIMPLE_PATTERN_EXACT) : NULL;
    qt->instances.pattern = is_valid_sp(qtl.charts) ? simple_pattern_create(qtl.charts, ",|\t\r\n\f\v", SIMPLE_PATTERN_EXACT) : NULL;
    qt->query.pattern = is_valid_sp(qtl.dimensions) ? simple_pattern_create(qtl.dimensions, ",|\t\r\n\f\v", SIMPLE_PATTERN_EXACT) : NULL;
    qt->instances.chart_label_key_pattern = is_valid_sp(qtl.chart_label_key) ? simple_pattern_create(qtl.chart_label_key, ",|\t\r\n\f\v", SIMPLE_PATTERN_EXACT) : NULL;
    qt->instances.charts_labels_filter_pattern = is_valid_sp(qtl.charts_labels_filter) ? simple_pattern_create(qtl.charts_labels_filter, ",|\t\r\n\f\v", SIMPLE_PATTERN_EXACT) : NULL;

    qtl.match_ids = qt->request.options & RRDR_OPTION_MATCH_IDS;
    qtl.match_names = qt->request.options & RRDR_OPTION_MATCH_NAMES;
    if(likely(!qtl.match_ids && !qtl.match_names))
        qtl.match_ids = qtl.match_names = true;

    // verify that the chart belongs to the host we are interested
    if(qtl.st) {
        if (!qtl.host) {
            // It is NULL, set it ourselves.
            qtl.host = qtl.st->rrdhost;
        }
        else if (unlikely(qtl.host != qtl.st->rrdhost)) {
            // Oops! A different host!
            error("QUERY TARGET: RRDSET '%s' given does not belong to host '%s'. Switching query host to '%s'",
                  rrdset_name(qtl.st), rrdhost_hostname(qtl.host), rrdhost_hostname(qtl.st->rrdhost));
            qtl.host = qtl.st->rrdhost;
        }
    }

    if(qtl.host) {
        // single host query
        query_target_add_host(&qtl, qtl.host);
        qtl.hosts = rrdhost_hostname(qtl.host);
    }
    else {
        // multi host query
        rrd_rdlock();
        rrdhost_foreach_read(qtl.host) {
            if(!qt->hosts.pattern || simple_pattern_matches(qt->hosts.pattern, rrdhost_hostname(qtl.host)))
                query_target_add_host(&qtl, qtl.host);
        }
        rrd_unlock();
    }

    // make sure everything is good
    if(!qt->query.used || !qt->metrics.used || !qt->instances.used || !qt->contexts.used || !qt->hosts.used) {
        internal_error(
                true
                , "QUERY TARGET: query '%s' does not have all the data required. "
                  "Matched %u hosts, %u contexts, %u instances, %u dimensions, %u metrics to query, "
                  "%zu metrics skipped because they don't have data in the desired time-frame. "
                  "Aborting it."
                , qt->id
                , qt->hosts.used
                , qt->contexts.used
                , qt->instances.used
                , qt->metrics.used
                , qt->query.used
                , qtl.metrics_skipped_due_to_not_matching_timeframe
                );

        query_target_release(qt);
        return NULL;
    }

    if(!query_target_calculate_window(qt)) {
        query_target_release(qt);
        return NULL;
    }

    return qt;
}


// ----------------------------------------------------------------------------
// load from SQL

static void rrdinstance_load_clabel(SQL_CLABEL_DATA *sld, void *data) {
    RRDINSTANCE *ri = data;
    rrdlabels_add(ri->rrdlabels, sld->label_key, sld->label_value, sld->label_source);
}

static void rrdinstance_load_dimension(SQL_DIMENSION_DATA *sd, void *data) {
    RRDINSTANCE *ri = data;

    RRDMETRIC trm = {
        .id = string_strdupz(sd->id),
        .name = string_strdupz(sd->name),
        .flags = RRD_FLAG_ARCHIVED | RRD_FLAG_UPDATE_REASON_LOAD_SQL, // no need for atomic
    };
    if(sd->hidden) trm.flags |= RRD_FLAG_HIDDEN;

    uuid_copy(trm.uuid, sd->dim_id);

    dictionary_set(ri->rrdmetrics, string2str(trm.id), &trm, sizeof(trm));
}

static void rrdinstance_load_chart_callback(SQL_CHART_DATA *sc, void *data) {
    RRDHOST *host = data;

    RRDCONTEXT tc = {
        .id = string_strdupz(sc->context),
        .title = string_strdupz(sc->title),
        .units = string_strdupz(sc->units),
        .family = string_strdupz(sc->family),
        .priority = sc->priority,
        .chart_type = sc->chart_type,
        .flags = RRD_FLAG_ARCHIVED | RRD_FLAG_UPDATE_REASON_LOAD_SQL, // no need for atomics
        .rrdhost = host,
    };

    RRDCONTEXT_ACQUIRED *rca = (RRDCONTEXT_ACQUIRED *)dictionary_set_and_acquire_item((DICTIONARY *)host->rrdctx, string2str(tc.id), &tc, sizeof(tc));
    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);

    RRDINSTANCE tri = {
        .id = string_strdupz(sc->id),
        .name = string_strdupz(sc->name),
        .title = string_strdupz(sc->title),
        .units = string_strdupz(sc->units),
        .family = string_strdupz(sc->family),
        .chart_type = sc->chart_type,
        .priority = sc->priority,
        .update_every_s = sc->update_every,
        .flags = RRD_FLAG_ARCHIVED | RRD_FLAG_UPDATE_REASON_LOAD_SQL, // no need for atomics
    };
    uuid_copy(tri.uuid, sc->chart_id);

    RRDINSTANCE_ACQUIRED *ria = (RRDINSTANCE_ACQUIRED *)dictionary_set_and_acquire_item(rc->rrdinstances, sc->id, &tri, sizeof(tri));
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);

    ctx_get_dimension_list(&ri->uuid, rrdinstance_load_dimension, ri);
    ctx_get_label_list(&ri->uuid, rrdinstance_load_clabel, ri);
    rrdinstance_trigger_updates(ri, __FUNCTION__ );
    rrdinstance_release(ria);
    rrdcontext_release(rca);
}

static void rrdcontext_load_context_callback(VERSIONED_CONTEXT_DATA *ctx_data, void *data) {
    RRDHOST *host = data;
    (void)host;

    RRDCONTEXT trc = {
        .id = string_strdupz(ctx_data->id),
        .flags = RRD_FLAG_ARCHIVED | RRD_FLAG_UPDATE_REASON_LOAD_SQL, // no need for atomics

        // no need to set more data here
        // we only need the hub data

        .hub = *ctx_data,
    };
    dictionary_set((DICTIONARY *)host->rrdctx, string2str(trc.id), &trc, sizeof(trc));
}

void rrdhost_load_rrdcontext_data(RRDHOST *host) {
    if(host->rrdctx) return;

    rrdhost_create_rrdcontexts(host);
    ctx_get_context_list(&host->host_uuid, rrdcontext_load_context_callback, host);
    ctx_get_chart_list(&host->host_uuid, rrdinstance_load_chart_callback, host);

    RRDCONTEXT *rc;
    dfe_start_read((DICTIONARY *)host->rrdctx, rc) {
        rrdcontext_trigger_updates(rc, __FUNCTION__ );
    }
    dfe_done(rc);

    rrdcontext_garbage_collect_single_host(host, false);
}

// ----------------------------------------------------------------------------
// version hash calculation

static uint64_t rrdcontext_version_hash_with_callback(
    RRDHOST *host,
    void (*callback)(RRDCONTEXT *, bool, void *),
    bool snapshot,
    void *bundle) {

    if(unlikely(!host || !host->rrdctx)) return 0;

    RRDCONTEXT *rc;
    uint64_t hash = 0;

    // loop through all contexts of the host
    dfe_start_read((DICTIONARY *)host->rrdctx, rc) {

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

static void rrdcontext_recalculate_context_retention(RRDCONTEXT *rc, RRD_FLAGS reason, bool worker_jobs) {
    rrdcontext_post_process_updates(rc, true, reason, worker_jobs);
}

static void rrdcontext_recalculate_host_retention(RRDHOST *host, RRD_FLAGS reason, bool worker_jobs) {
    if(unlikely(!host || !host->rrdctx)) return;

    RRDCONTEXT *rc;
    dfe_start_read((DICTIONARY *)host->rrdctx, rc) {
        rrdcontext_recalculate_context_retention(rc, reason, worker_jobs);
    }
    dfe_done(rc);
}

static void rrdcontext_recalculate_retention_all_hosts(void) {
    rrdcontext_next_db_rotation_ut = 0;
    rrd_rdlock();
    RRDHOST *host;
    rrdhost_foreach_read(host) {
        worker_is_busy(WORKER_JOB_RETENTION);
        rrdcontext_recalculate_host_retention(host, RRD_FLAG_UPDATE_REASON_DB_ROTATION, true);
    }
    rrd_unlock();
}

// ----------------------------------------------------------------------------
// garbage collector

static bool rrdmetric_update_retention(RRDMETRIC *rm) {
    time_t min_first_time_t = LONG_MAX, max_last_time_t = 0;

    if(rm->rrddim) {
        min_first_time_t = rrddim_first_entry_s(rm->rrddim);
        max_last_time_t = rrddim_last_entry_s(rm->rrddim);
    }
    else {
        RRDHOST *rrdhost = rm->ri->rc->rrdhost;
        for (size_t tier = 0; tier < storage_tiers; tier++) {
            STORAGE_ENGINE *eng = rrdhost->db[tier].eng;

            time_t first_time_t, last_time_t;
            if (eng->api.metric_retention_by_uuid(rrdhost->db[tier].instance, &rm->uuid, &first_time_t, &last_time_t)) {
                if (first_time_t < min_first_time_t)
                    min_first_time_t = first_time_t;

                if (last_time_t > max_last_time_t)
                    max_last_time_t = last_time_t;
            }
        }
    }

    if((min_first_time_t == LONG_MAX || min_first_time_t == 0) && max_last_time_t == 0)
        return false;

    if(min_first_time_t == LONG_MAX)
        min_first_time_t = 0;

    if(min_first_time_t > max_last_time_t) {
        internal_error(true, "RRDMETRIC: retention of '%s' is flipped, first_time_t = %ld, last_time_t = %ld", string2str(rm->id), min_first_time_t, max_last_time_t);
        time_t tmp = min_first_time_t;
        min_first_time_t = max_last_time_t;
        max_last_time_t = tmp;
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
        rrd_flag_set_deleted(rm, RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);

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

static inline bool rrdcontext_should_be_deleted(RRDCONTEXT *rc) {
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

    // delete it from SQL
    if(ctx_delete_context(&rc->rrdhost->host_uuid, &rc->hub) != 0)
        error("RRDCONTEXT: failed to delete context '%s' version %"PRIu64" from SQL.", rc->hub.id, rc->hub.version);
}

static void rrdcontext_garbage_collect_single_host(RRDHOST *host, bool worker_jobs) {

    internal_error(true, "RRDCONTEXT: garbage collecting context structures of host '%s'", rrdhost_hostname(host));

    RRDCONTEXT *rc;
    dfe_start_reentrant((DICTIONARY *)host->rrdctx, rc) {
        if(unlikely(!service_running(SERVICE_CONTEXT))) break;

        if(worker_jobs) worker_is_busy(WORKER_JOB_CLEANUP);

        rrdcontext_lock(rc);

        RRDINSTANCE *ri;
        dfe_start_reentrant(rc->rrdinstances, ri) {
            if(unlikely(!service_running(SERVICE_CONTEXT))) break;

            RRDMETRIC *rm;
            dfe_start_write(ri->rrdmetrics, rm) {
                if(rrdmetric_should_be_deleted(rm)) {
                    if(worker_jobs) worker_is_busy(WORKER_JOB_CLEANUP_DELETE);
                    if(!dictionary_del(ri->rrdmetrics, string2str(rm->id)))
                        error("RRDCONTEXT: metric '%s' of instance '%s' of context '%s' of host '%s', failed to be deleted from rrdmetrics dictionary.",
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
                    error("RRDCONTEXT: instance '%s' of context '%s' of host '%s', failed to be deleted from rrdmetrics dictionary.",
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
        }
        dfe_done(ri);

        if(unlikely(rrdcontext_should_be_deleted(rc))) {
            if(worker_jobs) worker_is_busy(WORKER_JOB_CLEANUP_DELETE);
            rrdcontext_dequeue_from_post_processing(rc);
            rrdcontext_delete_from_sql_unsafe(rc);

            if(!dictionary_del((DICTIONARY *)host->rrdctx, string2str(rc->id)))
                error("RRDCONTEXT: context '%s' of host '%s', failed to be deleted from rrdmetrics dictionary.",
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
}

static void rrdcontext_garbage_collect_for_all_hosts(void) {
    rrd_rdlock();
    RRDHOST *host;
    rrdhost_foreach_read(host) {
        rrdcontext_garbage_collect_single_host(host, true);
    }
    rrd_unlock();
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
        rrd_flag_set_archived(rm);
        rrd_flag_set(rm, RRD_FLAG_UPDATE_REASON_DISCONNECTED_CHILD);
    }
    if(rrd_flag_is_deleted(rm) && (reason & RRD_FLAG_UPDATE_REASON_UPDATE_RETENTION))
        rrd_flag_set_archived(rm);

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
    size_t metrics_active = 0, metrics_deleted = 0;
    bool live_retention = true, currently_collected = false;
    if(dictionary_entries(ri->rrdmetrics) > 0) {
        RRDMETRIC *rm;
        dfe_start_read((DICTIONARY *)ri->rrdmetrics, rm) {
            if(unlikely(!service_running(SERVICE_CONTEXT))) break;

            RRD_FLAGS reason_to_pass = reason;
            if(rrd_flag_check(ri, RRD_FLAG_UPDATE_REASON_UPDATE_RETENTION))
                reason_to_pass |= RRD_FLAG_UPDATE_REASON_UPDATE_RETENTION;

            rrdmetric_process_updates(rm, force, reason_to_pass, worker_jobs);

            if(unlikely(!rrd_flag_check(rm, RRD_FLAG_LIVE_RETENTION)))
                live_retention = false;

            if (unlikely((rrdmetric_should_be_deleted(rm)))) {
                metrics_deleted++;
                continue;
            }

            if(!currently_collected && rrd_flag_check(rm, RRD_FLAG_COLLECTED) && rm->first_time_s)
                currently_collected = true;

            metrics_active++;

            if (rm->first_time_s && rm->first_time_s < min_first_time_t)
                min_first_time_t = rm->first_time_s;

            if (rm->last_time_s && rm->last_time_s > max_last_time_t)
                max_last_time_t = rm->last_time_s;
        }
        dfe_done(rm);
    }

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

        rrd_flag_set_deleted(ri, RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);
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
                rrd_flag_set_deleted(ri, RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);
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
                rrd_flag_set_collected(ri);
            else
                rrd_flag_set_archived(ri);
        }
    }

    rrd_flag_unset_updated(ri);
}

static void rrdcontext_post_process_updates(RRDCONTEXT *rc, bool force, RRD_FLAGS reason, bool worker_jobs) {
    if(reason != RRD_FLAG_NONE)
        rrd_flag_set_updated(rc, reason);

    if(worker_jobs)
        worker_is_busy(WORKER_JOB_PP_CONTEXT);

    size_t min_priority_collected = LONG_MAX;
    size_t min_priority_not_collected = LONG_MAX;
    size_t min_priority = LONG_MAX;
    time_t min_first_time_t = LONG_MAX, max_last_time_t = 0;
    size_t instances_active = 0, instances_deleted = 0;
    bool live_retention = true, currently_collected = false, hidden = true;
    if(dictionary_entries(rc->rrdinstances) > 0) {
        RRDINSTANCE *ri;
        dfe_start_reentrant(rc->rrdinstances, ri) {
            if(unlikely(!service_running(SERVICE_CONTEXT))) break;

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

            if(unlikely(!currently_collected && rrd_flag_is_collected(ri) && ri->first_time_s))
                currently_collected = true;

            internal_error(rc->units != ri->units,
                           "RRDCONTEXT: '%s' rrdinstance '%s' has different units, context '%s', instance '%s'",
                           string2str(rc->id), string2str(ri->id),
                           string2str(rc->units), string2str(ri->units));

            instances_active++;

            if (ri->priority >= RRDCONTEXT_MINIMUM_ALLOWED_PRIORITY) {
                if(rrd_flag_check(ri, RRD_FLAG_COLLECTED)) {
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

        rrd_flag_set_deleted(rc, RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);
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

            rrd_flag_set_deleted(rc, RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);
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
                rrd_flag_set_collected(rc);
            else
                rrd_flag_set_archived(rc);
        }

        if (min_priority != LONG_MAX && rc->priority != min_priority) {
            rc->priority = min_priority;
            rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
        }
    }

    if(unlikely(rrd_flag_is_updated(rc) && rc->rrdhost->rrdctx_hub_queue)) {
        if(check_if_cloud_version_changed_unsafe(rc, false)) {
            rc->version = rrdcontext_get_next_version(rc);
            dictionary_set((DICTIONARY *)rc->rrdhost->rrdctx_hub_queue,
                           string2str(rc->id), rc, sizeof(*rc));
        }
    }

    rrd_flag_unset_updated(rc);
    rrdcontext_unlock(rc);
}

static void rrdcontext_queue_for_post_processing(RRDCONTEXT *rc, const char *function __maybe_unused, RRD_FLAGS flags __maybe_unused) {
    if(unlikely(!rc->rrdhost->rrdctx_post_processing_queue)) return;

    if(!rrd_flag_check(rc, RRD_FLAG_QUEUED_FOR_PP)) {
        dictionary_set((DICTIONARY *)rc->rrdhost->rrdctx_post_processing_queue,
                       string2str(rc->id),
                       rc,
                       sizeof(*rc));

#if(defined(NETDATA_INTERNAL_CHECKS) && defined(LOG_POST_PROCESSING_QUEUE_INSERTIONS))
        {
            BUFFER *wb_flags = buffer_create(1000);
            rrd_flags_to_buffer(flags, wb_flags);

            BUFFER *wb_reasons = buffer_create(1000);
            rrd_reasons_to_buffer(flags, wb_reasons);

            internal_error(true, "RRDCONTEXT: '%s' update triggered by function %s(), due to flags: %s, reasons: %s",
                           string2str(rc->id), function,
                           buffer_tostring(wb_flags),
                           buffer_tostring(wb_reasons));

            buffer_free(wb_reasons);
            buffer_free(wb_flags);
        }
#endif
    }
}

static void rrdcontext_dequeue_from_post_processing(RRDCONTEXT *rc) {
    if(unlikely(!rc->rrdhost->rrdctx_post_processing_queue)) return;
    dictionary_del((DICTIONARY *)rc->rrdhost->rrdctx_post_processing_queue, string2str(rc->id));
}

static void rrdcontext_post_process_queued_contexts(RRDHOST *host) {
    if(unlikely(!host->rrdctx_post_processing_queue)) return;

    RRDCONTEXT *rc;
    dfe_start_reentrant((DICTIONARY *)host->rrdctx_post_processing_queue, rc) {
        if(unlikely(!service_running(SERVICE_CONTEXT))) break;

        rrdcontext_dequeue_from_post_processing(rc);
        rrdcontext_post_process_updates(rc, false, RRD_FLAG_NONE, true);
    }
    dfe_done(rc);
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

static void rrdcontext_message_send_unsafe(RRDCONTEXT *rc, bool snapshot __maybe_unused, void *bundle __maybe_unused) {

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

#ifdef ENABLE_ACLK
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
#endif

    // store it to SQL

    if(rrd_flag_is_deleted(rc))
        rrdcontext_delete_from_sql_unsafe(rc);

    else if (ctx_store_context(&rc->rrdhost->host_uuid, &rc->hub) != 0)
        error("RRDCONTEXT: failed to save context '%s' version %"PRIu64" to SQL.", rc->hub.id, rc->hub.version);
}

static bool check_if_cloud_version_changed_unsafe(RRDCONTEXT *rc, bool sending __maybe_unused) {
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

        return true;
    }

    return false;
}

static inline usec_t rrdcontext_calculate_queued_dispatch_time_ut(RRDCONTEXT *rc, usec_t now_ut) {

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

static void rrdcontext_dequeue_from_hub_queue(RRDCONTEXT *rc) {
    dictionary_del((DICTIONARY *)rc->rrdhost->rrdctx_hub_queue, string2str(rc->id));
}

static void rrdcontext_dispatch_queued_contexts_to_hub(RRDHOST *host, usec_t now_ut) {

    // check if we have received a streaming command for this host
    if(!rrdhost_flag_check(host, RRDHOST_FLAG_ACLK_STREAM_CONTEXTS) || !aclk_connected || !host->rrdctx_hub_queue)
        return;

    // check if there are queued items to send
    if(!dictionary_entries((DICTIONARY *)host->rrdctx_hub_queue))
        return;

    if(!host->node_id)
        return;

    size_t messages_added = 0;
    contexts_updated_t bundle = NULL;

    RRDCONTEXT *rc;
    dfe_start_reentrant((DICTIONARY *)host->rrdctx_hub_queue, rc) {
        if(unlikely(!service_running(SERVICE_CONTEXT))) break;

        if(unlikely(messages_added >= MESSAGES_PER_BUNDLE_TO_SEND_TO_HUB_PER_HOST))
            break;

        worker_is_busy(WORKER_JOB_QUEUED);
        usec_t dispatch_ut = rrdcontext_calculate_queued_dispatch_time_ut(rc, now_ut);
        char *claim_id = get_agent_claimid();

        if(unlikely(now_ut >= dispatch_ut) && claim_id) {
            worker_is_busy(WORKER_JOB_CHECK);

            rrdcontext_lock(rc);

            if(check_if_cloud_version_changed_unsafe(rc, true)) {
                worker_is_busy(WORKER_JOB_SEND);

#ifdef ENABLE_ACLK
                if(!bundle) {
                    // prepare the bundle to send the messages
                    char uuid[UUID_STR_LEN];
                    uuid_unparse_lower(*host->node_id, uuid);

                    bundle = contexts_updated_new(claim_id, uuid, 0, now_ut);
                }
#endif
                // update the hub data of the context, give a new version, pack the message
                // and save an update to SQL
                rrdcontext_message_send_unsafe(rc, false, bundle);
                messages_added++;

                rc->queue.dispatches++;
                rc->queue.dequeued_ut = now_ut;
            }
            else
                rc->version = rc->hub.version;

            // remove it from the queue
            worker_is_busy(WORKER_JOB_DEQUEUE);
            rrdcontext_dequeue_from_hub_queue(rc);

            if(unlikely(rrdcontext_should_be_deleted(rc))) {
                // this is a deleted context - delete it forever...

                worker_is_busy(WORKER_JOB_CLEANUP_DELETE);

                rrdcontext_dequeue_from_post_processing(rc);
                rrdcontext_delete_from_sql_unsafe(rc);

                STRING *id = string_dup(rc->id);
                rrdcontext_unlock(rc);

                // delete it from the master dictionary
                if(!dictionary_del((DICTIONARY *)host->rrdctx, string2str(rc->id)))
                    error("RRDCONTEXT: '%s' of host '%s' failed to be deleted from rrdcontext dictionary.",
                          string2str(id), rrdhost_hostname(host));

                string_freez(id);
            }
            else
                rrdcontext_unlock(rc);
        }
        freez(claim_id);
    }
    dfe_done(rc);

#ifdef ENABLE_ACLK
    if(service_running(SERVICE_CONTEXT) && bundle) {
        // we have a bundle to send messages

        // update the version hash
        contexts_updated_update_version_hash(bundle, rrdcontext_version_hash(host));

        // send it
        aclk_send_contexts_updated(bundle);
    }
    else if(bundle)
        contexts_updated_delete(bundle);
#endif

}

// ----------------------------------------------------------------------------
// worker thread

static void rrdcontext_main_cleanup(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    // custom code
    worker_unregister();

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void *rrdcontext_main(void *ptr) {
    netdata_thread_cleanup_push(rrdcontext_main_cleanup, ptr);

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
    heartbeat_init(&hb);
    usec_t step = RRDCONTEXT_WORKER_THREAD_HEARTBEAT_USEC;

    while (service_running(SERVICE_CONTEXT)) {
        worker_is_idle();
        heartbeat_next(&hb, step);

        if(unlikely(!service_running(SERVICE_CONTEXT))) break;

        usec_t now_ut = now_realtime_usec();

        if(rrdcontext_next_db_rotation_ut && now_ut > rrdcontext_next_db_rotation_ut) {
            rrdcontext_recalculate_retention_all_hosts();
            rrdcontext_garbage_collect_for_all_hosts();
            rrdcontext_next_db_rotation_ut = 0;
        }

        size_t hub_queued_contexts_for_all_hosts = 0;
        size_t pp_queued_contexts_for_all_hosts = 0;

        rrd_rdlock();
        RRDHOST *host;
        rrdhost_foreach_read(host) {
            if(unlikely(!service_running(SERVICE_CONTEXT))) break;

            worker_is_busy(WORKER_JOB_HOSTS);

            if(host->rrdctx_post_processing_queue) {
                pp_queued_contexts_for_all_hosts +=
                    dictionary_entries((DICTIONARY *)host->rrdctx_post_processing_queue);
                rrdcontext_post_process_queued_contexts(host);
            }

            if(host->rrdctx_hub_queue) {
                hub_queued_contexts_for_all_hosts += dictionary_entries((DICTIONARY *)host->rrdctx_hub_queue);
                rrdcontext_dispatch_queued_contexts_to_hub(host, now_ut);
            }
        }
        rrd_unlock();

        worker_set_metric(WORKER_JOB_HUB_QUEUE_SIZE, (NETDATA_DOUBLE)hub_queued_contexts_for_all_hosts);
        worker_set_metric(WORKER_JOB_PP_QUEUE_SIZE, (NETDATA_DOUBLE)pp_queued_contexts_for_all_hosts);
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}

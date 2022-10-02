// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdcontext.h"
#include "sqlite/sqlite_context.h"
#include "aclk/schema-wrappers/context.h"
#include "aclk/aclk_contexts_api.h"
#include "aclk/aclk.h"

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


typedef enum {
    RRD_FLAG_NONE           = 0,
    RRD_FLAG_DELETED        = (1 << 0), // this is a deleted object (metrics, instances, contexts)
    RRD_FLAG_COLLECTED      = (1 << 1), // this object is currently being collected
    RRD_FLAG_UPDATED        = (1 << 2), // this object has updates to propagate
    RRD_FLAG_ARCHIVED       = (1 << 3), // this object is not currently being collected
    RRD_FLAG_OWN_LABELS     = (1 << 4), // this instance has its own labels - not linked to an RRDSET
    RRD_FLAG_LIVE_RETENTION = (1 << 5), // we have got live retention from the database
    RRD_FLAG_QUEUED_FOR_HUB = (1 << 6), // this context is currently queued to be dispatched to hub
    RRD_FLAG_QUEUED_FOR_POST_PROCESSING = (1 << 7), // this context is currently queued to be post-processed
    RRD_FLAG_HIDDEN         = (1 << 8), // don't expose this to the hub or the API

    RRD_FLAG_UPDATE_REASON_TRIGGERED               = (1 << 9),  // the update was triggered by the child object
    RRD_FLAG_UPDATE_REASON_LOAD_SQL                = (1 << 10), // this object has just been loaded from SQL
    RRD_FLAG_UPDATE_REASON_NEW_OBJECT              = (1 << 11), // this object has just been created
    RRD_FLAG_UPDATE_REASON_UPDATED_OBJECT          = (1 << 12), // we received an update on this object
    RRD_FLAG_UPDATE_REASON_CHANGED_LINKING         = (1 << 13), // an instance or a metric switched RRDSET or RRDDIM
    RRD_FLAG_UPDATE_REASON_CHANGED_UUID            = (1 << 14), // an instance or a metric changed UUID
    RRD_FLAG_UPDATE_REASON_CHANGED_NAME            = (1 << 15), // an instance or a metric changed name
    RRD_FLAG_UPDATE_REASON_CHANGED_UNITS           = (1 << 16), // this context or instance changed units
    RRD_FLAG_UPDATE_REASON_CHANGED_TITLE           = (1 << 17), // this context or instance changed title
    RRD_FLAG_UPDATE_REASON_CHANGED_FAMILY          = (1 << 18), // the context or the instance changed family
    RRD_FLAG_UPDATE_REASON_CHANGED_CHART_TYPE      = (1 << 19), // this context or instance changed chart type
    RRD_FLAG_UPDATE_REASON_CHANGED_PRIORITY        = (1 << 20), // this context or instance changed its priority
    RRD_FLAG_UPDATE_REASON_CHANGED_UPDATE_EVERY    = (1 << 21), // the instance or the metric changed update frequency
    RRD_FLAG_UPDATE_REASON_ZERO_RETENTION          = (1 << 22), // this object has not retention
    RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T    = (1 << 23), // this object changed its oldest time in the db
    RRD_FLAG_UPDATE_REASON_CHANGED_LAST_TIME_T     = (1 << 24), // this object change its latest time in the db
    RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED = (1 << 25), // this object has stopped being collected
    RRD_FLAG_UPDATE_REASON_STARTED_BEING_COLLECTED = (1 << 26), // this object has started being collected
    RRD_FLAG_UPDATE_REASON_DISCONNECTED_CHILD      = (1 << 27), // this context belongs to a host that just disconnected
    RRD_FLAG_UPDATE_REASON_DB_ROTATION             = (1 << 28), // this context changed because of a db rotation
    RRD_FLAG_UPDATE_REASON_UNUSED                  = (1 << 29), // this context is not used anymore
    RRD_FLAG_UPDATE_REASON_CHANGED_FLAGS           = (1 << 30), // this context is not used anymore

    // DO NOT ADD (1 << 31) or bigger!
    // runtime error: left shift of 1 by 31 places cannot be represented in type 'int'
} RRD_FLAGS;

#define RRD_FLAG_ALL_UPDATE_REASONS                   ( \
     RRD_FLAG_UPDATE_REASON_TRIGGERED                   \
    |RRD_FLAG_UPDATE_REASON_LOAD_SQL                    \
    |RRD_FLAG_UPDATE_REASON_NEW_OBJECT                  \
    |RRD_FLAG_UPDATE_REASON_UPDATED_OBJECT              \
    |RRD_FLAG_UPDATE_REASON_CHANGED_LINKING             \
    |RRD_FLAG_UPDATE_REASON_CHANGED_UUID                \
    |RRD_FLAG_UPDATE_REASON_CHANGED_NAME                \
    |RRD_FLAG_UPDATE_REASON_CHANGED_UNITS               \
    |RRD_FLAG_UPDATE_REASON_CHANGED_TITLE               \
    |RRD_FLAG_UPDATE_REASON_CHANGED_FAMILY              \
    |RRD_FLAG_UPDATE_REASON_CHANGED_CHART_TYPE          \
    |RRD_FLAG_UPDATE_REASON_CHANGED_PRIORITY            \
    |RRD_FLAG_UPDATE_REASON_CHANGED_UPDATE_EVERY        \
    |RRD_FLAG_UPDATE_REASON_ZERO_RETENTION              \
    |RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T        \
    |RRD_FLAG_UPDATE_REASON_CHANGED_LAST_TIME_T         \
    |RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED     \
    |RRD_FLAG_UPDATE_REASON_STARTED_BEING_COLLECTED     \
    |RRD_FLAG_UPDATE_REASON_DISCONNECTED_CHILD          \
    |RRD_FLAG_UPDATE_REASON_DB_ROTATION                 \
    |RRD_FLAG_UPDATE_REASON_UNUSED                      \
    |RRD_FLAG_UPDATE_REASON_CHANGED_FLAGS               \
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
    |RRD_FLAG_QUEUED_FOR_POST_PROCESSING                \
)

// get all the flags of an object
#define rrd_flags_get(obj) __atomic_load_n(&((obj)->flags), __ATOMIC_SEQ_CST)

// check if ANY of the given flags (bits) is set
#define rrd_flag_check(obj, flag) (rrd_flags_get(obj) & (flag))

// check if ALL of the given flags (bits) are set
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
    { RRD_FLAG_UPDATE_REASON_TRIGGERED,               "triggered transition", 60 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_NEW_OBJECT,              "object created",       60 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_UPDATED_OBJECT,          "object updated",       60 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_LOAD_SQL,                "loaded from sql",      60 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_CHANGED_TITLE,           "changed title",        30 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_CHANGED_UNITS,           "changed units",        30 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_CHANGED_FAMILY,          "changed family",       30 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_CHANGED_PRIORITY,        "changed priority",     30 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_ZERO_RETENTION,          "has no retention",     60 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T,    "updated first_time_t", 30 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_CHANGED_LAST_TIME_T,     "updated last_time_t",  60 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_CHANGED_CHART_TYPE,      "changed chart type",   30 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED, "stopped collected",    60 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_STARTED_BEING_COLLECTED, "started collected",    0 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_UNUSED,                  "unused",               0 * USEC_PER_SEC },

    // not context related
    { RRD_FLAG_UPDATE_REASON_CHANGED_UUID,            "changed uuid",         60 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_CHANGED_UPDATE_EVERY,    "changed updated every",60 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_CHANGED_LINKING,         "changed rrd link",     60 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_CHANGED_NAME,            "changed name",         60 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_DISCONNECTED_CHILD,      "child disconnected",   30 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_DB_ROTATION,             "db rotation",          60 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_CHANGED_FLAGS,           "changed flags",        60 * USEC_PER_SEC },

    // terminator
    { 0, NULL, 0 },
};


typedef struct rrdmetric {
    uuid_t uuid;

    STRING *id;
    STRING *name;

    RRDDIM *rrddim;

    time_t first_time_t;
    time_t last_time_t;
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
    time_t first_time_t;
    time_t last_time_t;

    int update_every;                   // data collection frequency
    RRDSET *rrdset;                     // pointer to RRDSET when collected, or NULL

    DICTIONARY *rrdlabels;              // linked to RRDSET->chart_labels or own version

    struct rrdcontext *rc;
    DICTIONARY *rrdmetrics;

    struct {
        uint32_t collected_metrics;     // a temporary variable to detect BEGIN/END without SET
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
    time_t first_time_t;
    time_t last_time_t;

    VERSIONED_CONTEXT_DATA hub;

    DICTIONARY *rrdinstances;
    RRDHOST *rrdhost;

    struct {
        RRD_FLAGS queued_flags;         // the last flags that triggered the queueing
        usec_t queued_ut;               // the last time this was queued
        usec_t delay_calc_ut;           // the last time we calculated the scheduled_dispatched_ut
        usec_t scheduled_dispatch_ut;   // the time it was/is scheduled to be sent
        usec_t dequeued_ut;             // the last time we sent (or deduped) this context
    } queue;

    netdata_mutex_t mutex;
} RRDCONTEXT;

// ----------------------------------------------------------------------------
// helper one-liners for RRDMETRIC

static inline RRDMETRIC *rrdmetric_acquired_value(RRDMETRIC_ACQUIRED *rma) {
    return dictionary_acquired_item_value((DICTIONARY_ITEM *)rma);
}

static inline void rrdmetric_release(RRDMETRIC_ACQUIRED *rma) {
    RRDMETRIC *rm = rrdmetric_acquired_value(rma);
    dictionary_acquired_item_release(rm->ri->rrdmetrics, (DICTIONARY_ITEM *)rma);
}

// ----------------------------------------------------------------------------
// helper one-liners for RRDINSTANCE

static inline RRDINSTANCE *rrdinstance_acquired_value(RRDINSTANCE_ACQUIRED *ria) {
    return dictionary_acquired_item_value((DICTIONARY_ITEM *)ria);
}

static inline void rrdinstance_release(RRDINSTANCE_ACQUIRED *ria) {
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    dictionary_acquired_item_release(ri->rc->rrdinstances, (DICTIONARY_ITEM *)ria);
}

// ----------------------------------------------------------------------------
// helper one-liners for RRDCONTEXT

static inline RRDCONTEXT *rrdcontext_acquired_value(RRDCONTEXT_ACQUIRED *rca) {
    return dictionary_acquired_item_value((DICTIONARY_ITEM *)rca);
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

static void rrd_flags_to_buffer(RRD_FLAGS flags, BUFFER *wb) {
    if(flags & RRD_FLAG_QUEUED_FOR_HUB)
        buffer_strcat(wb, "QUEUED ");

    if(flags & RRD_FLAG_DELETED)
        buffer_strcat(wb, "DELETED ");

    if(flags & RRD_FLAG_COLLECTED)
        buffer_strcat(wb, "COLLECTED ");

    if(flags & RRD_FLAG_UPDATED)
        buffer_strcat(wb, "UPDATED ");

    if(flags & RRD_FLAG_ARCHIVED)
        buffer_strcat(wb, "ARCHIVED ");

    if(flags & RRD_FLAG_OWN_LABELS)
        buffer_strcat(wb, "OWN_LABELS ");

    if(flags & RRD_FLAG_LIVE_RETENTION)
        buffer_strcat(wb, "LIVE_RETENTION ");

    if(flags & RRD_FLAG_HIDDEN)
        buffer_strcat(wb, "HIDDEN ");

    if(flags & RRD_FLAG_QUEUED_FOR_POST_PROCESSING)
        buffer_strcat(wb, "PENDING_UPDATES ");
}

static void rrd_reasons_to_buffer(RRD_FLAGS flags, BUFFER *wb) {
    for(int i = 0, added = 0; rrdcontext_reasons[i].name ; i++) {
        if (flags & rrdcontext_reasons[i].flag) {
            if (added)
                buffer_strcat(wb, ", ");
            buffer_strcat(wb, rrdcontext_reasons[i].name);
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
static bool rrdmetric_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *oldv, void *newv, void *rrdinstance __maybe_unused) {
    RRDMETRIC *rm     = oldv;
    RRDMETRIC *rm_new = newv;

    internal_error(rm->id != rm_new->id,
                   "RRDMETRIC: '%s' cannot change id to '%s'",
                   string2str(rm->id), string2str(rm_new->id));

    if(uuid_compare(rm->uuid, rm_new->uuid) != 0) {
        char uuid1[UUID_STR_LEN], uuid2[UUID_STR_LEN];
        uuid_unparse(rm->uuid, uuid1);
        uuid_unparse(rm_new->uuid, uuid2);
        internal_error(true, "RRDMETRIC: '%s' of instance '%s' changed uuid from '%s' to '%s'", string2str(rm->id), string2str(rm->ri->id), uuid1, uuid2);
        uuid_copy(rm->uuid, rm_new->uuid);
        rrd_flag_set_updated(rm, RRD_FLAG_UPDATE_REASON_CHANGED_UUID);
    }

    if(rm->rrddim && rm_new->rrddim && rm->rrddim != rm_new->rrddim) {
        rm->rrddim = rm_new->rrddim;
        rrd_flag_set_updated(rm, RRD_FLAG_UPDATE_REASON_CHANGED_LINKING);
    }

    if(rm->rrddim && uuid_compare(rm->uuid, rm->rrddim->metric_uuid) != 0) {
        char uuid1[UUID_STR_LEN], uuid2[UUID_STR_LEN];
        uuid_unparse(rm->uuid, uuid1);
        uuid_unparse(rm_new->uuid, uuid2);
        internal_error(true, "RRDMETRIC: '%s' is linked to RRDDIM '%s' but they have different UUIDs. RRDMETRIC has '%s', RRDDIM has '%s'", string2str(rm->id), rrddim_id(rm->rrddim), uuid1, uuid2);
    }

    if(rm->rrddim != rm_new->rrddim)
        rm->rrddim = rm_new->rrddim;

    if(rm->name != rm_new->name) {
        STRING *old = rm->name;
        rm->name = string_dup(rm_new->name);
        string_freez(old);
        rrd_flag_set_updated(rm, RRD_FLAG_UPDATE_REASON_CHANGED_NAME);
    }

    if(!rm->first_time_t || (rm_new->first_time_t && rm_new->first_time_t < rm->first_time_t)) {
        rm->first_time_t = rm_new->first_time_t;
        rrd_flag_set_updated(rm, RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T);
    }

    if(!rm->last_time_t || (rm_new->last_time_t && rm_new->last_time_t > rm->last_time_t)) {
        rm->last_time_t = rm_new->last_time_t;
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

    ri->rrdmetrics = dictionary_create(DICT_OPTION_DONT_OVERWRITE_VALUE);
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
    rm->ri->internal.collected_metrics++;

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
    static STRING *ml_anomaly_rates_id = NULL;

    if(unlikely(!ml_anomaly_rates_id))
        ml_anomaly_rates_id = string_strdupz(ML_ANOMALY_RATES_CHART_ID);

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
        if(unlikely((rrdset_flag_check(ri->rrdset, RRDSET_FLAG_HIDDEN)) || rrdset_is_ar_chart(ri->rrdset)))
            ri->flags |= RRD_FLAG_HIDDEN; // no need of atomics at the constructor
        else
            ri->flags &= ~RRD_FLAG_HIDDEN; // no need of atomics at the constructor
    }

    // we need this when loading from SQL
    if(unlikely(ri->id == ml_anomaly_rates_id))
        ri->flags |= RRD_FLAG_HIDDEN; // no need of atomics at the constructor

    rrdmetrics_create_in_rrdinstance(ri);

    // signal the react callback to do the job
    rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_NEW_OBJECT);
}

static void rrdinstance_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *rrdcontext __maybe_unused) {
    RRDINSTANCE *ri = (RRDINSTANCE *)value;

    internal_error(ri->rrdset, "RRDINSTANCE: '%s' is freed but there is a RRDSET linked to it.", string2str(ri->id));

    rrdinstance_free(ri);
}

static bool rrdinstance_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *oldv, void *newv, void *rrdcontext __maybe_unused) {
    RRDINSTANCE *ri     = (RRDINSTANCE *)oldv;
    RRDINSTANCE *ri_new = (RRDINSTANCE *)newv;

    internal_error(ri->id != ri_new->id,
                   "RRDINSTANCE: '%s' cannot change id to '%s'",
                   string2str(ri->id), string2str(ri_new->id));

    if(uuid_compare(ri->uuid, ri_new->uuid) != 0) {
        uuid_copy(ri->uuid, ri_new->uuid);
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_UUID);
    }

    if(ri->rrdset && ri_new->rrdset && ri->rrdset != ri_new->rrdset) {
        ri->rrdset = ri_new->rrdset;
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_LINKING);
    }

    if(ri->rrdset && uuid_compare(ri->uuid, ri->rrdset->chart_uuid) != 0) {
        char uuid1[UUID_STR_LEN], uuid2[UUID_STR_LEN];
        uuid_unparse(ri->uuid, uuid1);
        uuid_unparse(ri->rrdset->chart_uuid, uuid2);
        internal_error(true, "RRDINSTANCE: '%s' is linked to RRDSET '%s' but they have different UUIDs. RRDINSTANCE has '%s', RRDSET has '%s'", string2str(ri->id), rrdset_id(ri->rrdset), uuid1, uuid2);
    }

    if(ri->name != ri_new->name) {
        STRING *old = ri->name;
        ri->name = string_dup(ri_new->name);
        string_freez(old);
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_NAME);
    }

    if(ri->title != ri_new->title) {
        STRING *old = ri->title;
        ri->title = string_dup(ri_new->title);
        string_freez(old);
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_TITLE);
    }

    if(ri->units != ri_new->units) {
        STRING *old = ri->units;
        ri->units = string_dup(ri_new->units);
        string_freez(old);
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_UNITS);
    }

    if(ri->family != ri_new->family) {
        STRING *old = ri->family;
        ri->family = string_dup(ri_new->family);
        string_freez(old);
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_FAMILY);
    }

    if(ri->chart_type != ri_new->chart_type) {
        ri->chart_type = ri_new->chart_type;
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_CHART_TYPE);
    }

    if(ri->priority != ri_new->priority) {
        ri->priority = ri_new->priority;
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_PRIORITY);
    }

    if(ri->update_every != ri_new->update_every) {
        ri->update_every = ri_new->update_every;
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_UPDATE_EVERY);
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
        if(unlikely((rrdset_flag_check(ri->rrdset, RRDSET_FLAG_HIDDEN)) || rrdset_is_ar_chart(ri->rrdset)))
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

    rc->rrdinstances = dictionary_create(DICT_OPTION_DONT_OVERWRITE_VALUE);
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
            rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_PRIORITY);
        }
        if(unlikely(st->update_every != ri->update_every)) {
            ri->update_every = st->update_every;
            rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_UPDATE_EVERY);
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
        .update_every = st->update_every,
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
        // Ooops! The chart changed context!

        // RRDCONTEXT *rc_old = rrdcontext_acquired_value(rca_old);
        RRDINSTANCE *ri_old = rrdinstance_acquired_value(ria_old);

        // migrate all dimensions to the new metrics
        RRDDIM *rd;
        rrddim_foreach_read(rd, st) {
            if (!rd->rrdmetric) continue;

            RRDMETRIC *rm_old = rrdmetric_acquired_value(rd->rrdmetric);
            rrd_flags_replace(rm_old, RRD_FLAG_DELETED|RRD_FLAG_UPDATED|RRD_FLAG_LIVE_RETENTION|RRD_FLAG_UPDATE_REASON_UNUSED|RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);
            rm_old->rrddim = NULL;
            rm_old->first_time_t = 0;
            rm_old->last_time_t = 0;

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
        ri_old->first_time_t = 0;
        ri_old->last_time_t = 0;

        rrdinstance_trigger_updates(ri_old, __FUNCTION__ );
        rrdinstance_release(ria_old);

        /*
        // trigger updates on the old context
        if(!dictionary_entries(rc_old->rrdinstances) && !dictionary_stats_referenced_items(rc_old->rrdinstances)) {
            rrdcontext_lock(rc_old);
            rc_old->flags = ((rc_old->flags & RRD_FLAG_QUEUED)?RRD_FLAG_QUEUED:RRD_FLAG_NONE)|RRD_FLAG_DELETED|RRD_FLAG_UPDATED|RRD_FLAG_LIVE_RETENTION|RRD_FLAG_UPDATE_REASON_UNUSED|RRD_FLAG_UPDATE_REASON_ZERO_RETENTION;
            rc_old->first_time_t = 0;
            rc_old->last_time_t = 0;
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

static inline void rrdinstance_updated_rrdset_name(RRDSET *st) {
    // the chart may not be initialized when this is called
    if(unlikely(!st->rrdinstance)) return;

    RRDINSTANCE *ri = rrdset_get_rrdinstance(st);
    if(unlikely(!ri)) return;

    if(st->name != ri->name) {
        STRING *old = ri->name;
        ri->name = string_dup(st->name);
        string_freez(old);

        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_NAME);
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
            rrd_flag_set_updated(ri, RRD_FLAG_HIDDEN | RRD_FLAG_UPDATE_REASON_CHANGED_FLAGS);

        else if (unlikely(!st_is_hidden && ri_is_hidden)) {
            rrd_flag_clear(ri, RRD_FLAG_HIDDEN);
            rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_FLAGS);
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

    if(unlikely(ri->internal.collected_metrics && !rrd_flag_is_collected(ri)))
        rrd_flag_set_collected(ri);

    // we use this variable to detect BEGIN/END without SET
    ri->internal.collected_metrics = 0;

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
        rc->first_time_t = rc->hub.first_time_t;
        rc->last_time_t  = rc->hub.last_time_t;

        if(rc->hub.deleted || !rc->hub.first_time_t)
            rrd_flag_set_deleted(rc, RRD_FLAG_NONE);
        else {
            if (rc->last_time_t == 0)
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

static bool rrdcontext_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *oldv, void *newv, void *rrdhost __maybe_unused) {
    RRDCONTEXT *rc = (RRDCONTEXT *)oldv;
    RRDCONTEXT *rc_new = (RRDCONTEXT *)newv;

    //current rc is not archived, new_rc is archived, dont merge
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
        rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_TITLE);
    }

    if(rc->units != rc_new->units) {
        STRING *old_units = rc->units;
        rc->units = string_dup(rc_new->units);
        string_freez(old_units);
        rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_UNITS);
    }

    if(rc->family != rc_new->family) {
        STRING *old_family = rc->family;
        if (rrd_flag_is_archived(rc) && !rrd_flag_is_archived(rc_new))
            rc->family = string_dup(rc_new->family);
        else
            rc->family = string_2way_merge(rc->family, rc_new->family);
        string_freez(old_family);
        rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_FAMILY);
    }

    if(rc->chart_type != rc_new->chart_type) {
        rc->chart_type = rc_new->chart_type;
        rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_CHART_TYPE);
    }

    if(rc->priority != rc_new->priority) {
        rc->priority = rc_new->priority;
        rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_PRIORITY);
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
    rrd_flag_set(rc, RRD_FLAG_QUEUED_FOR_POST_PROCESSING);
}

static void rrdcontext_post_processing_queue_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *context, void *nothing __maybe_unused) {
    RRDCONTEXT *rc = context;
    rrd_flag_clear(rc, RRD_FLAG_QUEUED_FOR_POST_PROCESSING);
}

void rrdhost_create_rrdcontexts(RRDHOST *host) {
    if(unlikely(!host)) return;
    if(likely(host->rrdctx)) return;

    host->rrdctx = (RRDCONTEXTS *)dictionary_create(DICT_OPTION_DONT_OVERWRITE_VALUE);
    dictionary_register_insert_callback((DICTIONARY *)host->rrdctx, rrdcontext_insert_callback, host);
    dictionary_register_delete_callback((DICTIONARY *)host->rrdctx, rrdcontext_delete_callback, host);
    dictionary_register_conflict_callback((DICTIONARY *)host->rrdctx, rrdcontext_conflict_callback, host);
    dictionary_register_react_callback((DICTIONARY *)host->rrdctx, rrdcontext_react_callback, host);

    host->rrdctx_hub_queue = (RRDCONTEXTS *)dictionary_create(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_VALUE_LINK_DONT_CLONE);

    dictionary_register_insert_callback((DICTIONARY *)host->rrdctx_hub_queue, rrdcontext_hub_queue_insert_callback, NULL);
    dictionary_register_delete_callback((DICTIONARY *)host->rrdctx_hub_queue, rrdcontext_hub_queue_delete_callback, NULL);
    dictionary_register_conflict_callback((DICTIONARY *)host->rrdctx_hub_queue, rrdcontext_hub_queue_conflict_callback, NULL);

    host->rrdctx_post_processing_queue = (RRDCONTEXTS *)dictionary_create(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_VALUE_LINK_DONT_CLONE);

    dictionary_register_insert_callback((DICTIONARY *)host->rrdctx_hub_queue, rrdcontext_post_processing_queue_insert_callback, NULL);
    dictionary_register_delete_callback((DICTIONARY *)host->rrdctx_hub_queue, rrdcontext_post_processing_queue_delete_callback, NULL);
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
    time_t combined_first_time_t;
    time_t combined_last_time_t;
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

    if(after && (!rm->last_time_t || after > rm->last_time_t))
        return 0;

    if(before && (!rm->first_time_t || before < rm->first_time_t))
        return 0;

    if(t->chart_dimensions
        && !simple_pattern_matches(t->chart_dimensions, string2str(rm->id))
        && !simple_pattern_matches(t->chart_dimensions, string2str(rm->name)))
        return 0;

    if(t->written) {
        buffer_strcat(wb, ",\n");
        t->combined_first_time_t = MIN(t->combined_first_time_t, rm->first_time_t);
        t->combined_last_time_t = MAX(t->combined_last_time_t, rm->last_time_t);
        t->combined_flags |= rrd_flags_get(rm);
    }
    else {
        buffer_strcat(wb, "\n");
        t->combined_first_time_t = rm->first_time_t;
        t->combined_last_time_t = rm->last_time_t;
        t->combined_flags = rrd_flags_get(rm);
    }

    buffer_sprintf(wb, "\t\t\t\t\t\t\"%s\": {", id);

    if(options & RRDCONTEXT_OPTION_SHOW_UUIDS) {
        char uuid[UUID_STR_LEN];
        uuid_unparse(rm->uuid, uuid);
        buffer_sprintf(wb, "\n\t\t\t\t\t\t\t\"uuid\":\"%s\",", uuid);
    }

    buffer_sprintf(wb,
                   "\n\t\t\t\t\t\t\t\"name\":\"%s\""
                   ",\n\t\t\t\t\t\t\t\"first_time_t\":%ld"
                   ",\n\t\t\t\t\t\t\t\"last_time_t\":%ld"
                   ",\n\t\t\t\t\t\t\t\"collected\":%s"
                   , string2str(rm->name)
                   , rm->first_time_t
                   , rrd_flag_is_collected(rm) ? t->now : rm->last_time_t
                   , rrd_flag_is_collected(rm) ? "true" : "false"
                   );

    if(options & RRDCONTEXT_OPTION_SHOW_DELETED) {
        buffer_sprintf(wb,
                       ",\n\t\t\t\t\t\t\t\"deleted\":%s"
                       , rrd_flag_is_deleted(rm) ? "true" : "false"
        );
    }

    if(options & RRDCONTEXT_OPTION_SHOW_FLAGS) {
        buffer_strcat(wb, ",\n\t\t\t\t\t\t\t\"flags\":\"");
        rrd_flags_to_buffer(rrd_flags_get(rm), wb);
        buffer_strcat(wb, "\"");
    }

    buffer_strcat(wb, "\n\t\t\t\t\t\t}");
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

    if(after && (!ri->last_time_t || after > ri->last_time_t))
        return 0;

    if(before && (!ri->first_time_t || before < ri->first_time_t))
        return 0;

    if(t_parent->chart_label_key && !rrdlabels_match_simple_pattern_parsed(ri->rrdlabels, t_parent->chart_label_key, '\0'))
        return 0;

    if(t_parent->chart_labels_filter && !rrdlabels_match_simple_pattern_parsed(ri->rrdlabels, t_parent->chart_labels_filter, ':'))
        return 0;

    time_t first_time_t = ri->first_time_t;
    time_t last_time_t = ri->last_time_t;
    RRD_FLAGS flags = rrd_flags_get(ri);

    BUFFER *wb_metrics = NULL;
    if(options & RRDCONTEXT_OPTION_SHOW_METRICS || t_parent->chart_dimensions) {

        wb_metrics = buffer_create(4096);

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

        first_time_t = t_metrics.combined_first_time_t;
        last_time_t = t_metrics.combined_last_time_t;
        flags = t_metrics.combined_flags;
    }

    if(t_parent->written) {
        buffer_strcat(wb, ",\n");
        t_parent->combined_first_time_t = MIN(t_parent->combined_first_time_t, first_time_t);
        t_parent->combined_last_time_t = MAX(t_parent->combined_last_time_t, last_time_t);
        t_parent->combined_flags |= flags;
    }
    else {
        buffer_strcat(wb, "\n");
        t_parent->combined_first_time_t = first_time_t;
        t_parent->combined_last_time_t = last_time_t;
        t_parent->combined_flags = flags;
    }

    buffer_sprintf(wb, "\t\t\t\t\"%s\": {", id);

    if(options & RRDCONTEXT_OPTION_SHOW_UUIDS) {
        char uuid[UUID_STR_LEN];
        uuid_unparse(ri->uuid, uuid);
        buffer_sprintf(wb,"\n\t\t\t\t\t\"uuid\":\"%s\",", uuid);
    }

    buffer_sprintf(wb,
                   "\n\t\t\t\t\t\"name\":\"%s\""
                   ",\n\t\t\t\t\t\"context\":\"%s\""
                   ",\n\t\t\t\t\t\"title\":\"%s\""
                   ",\n\t\t\t\t\t\"units\":\"%s\""
                   ",\n\t\t\t\t\t\"family\":\"%s\""
                   ",\n\t\t\t\t\t\"chart_type\":\"%s\""
                   ",\n\t\t\t\t\t\"priority\":%u"
                   ",\n\t\t\t\t\t\"update_every\":%d"
                   ",\n\t\t\t\t\t\"first_time_t\":%ld"
                   ",\n\t\t\t\t\t\"last_time_t\":%ld"
                   ",\n\t\t\t\t\t\"collected\":%s"
                   , string2str(ri->name)
                   , string2str(ri->rc->id)
                   , string2str(ri->title)
                   , string2str(ri->units)
                   , string2str(ri->family)
                   , rrdset_type_name(ri->chart_type)
                   , ri->priority
                   , ri->update_every
                   , first_time_t
                   , (flags & RRD_FLAG_COLLECTED) ? t_parent->now : last_time_t
                   , (flags & RRD_FLAG_COLLECTED) ? "true" : "false"
    );

    if(options & RRDCONTEXT_OPTION_SHOW_DELETED) {
        buffer_sprintf(wb,
                       ",\n\t\t\t\t\t\"deleted\":%s"
                       , rrd_flag_is_deleted(ri) ? "true" : "false"
        );
    }

    if(options & RRDCONTEXT_OPTION_SHOW_FLAGS) {
        buffer_strcat(wb, ",\n\t\t\t\t\t\"flags\":\"");
        rrd_flags_to_buffer(rrd_flags_get(ri), wb);
        buffer_strcat(wb, "\"");
    }

    if(options & RRDCONTEXT_OPTION_SHOW_LABELS && ri->rrdlabels && dictionary_entries(ri->rrdlabels)) {
        buffer_sprintf(wb, ",\n\t\t\t\t\t\"labels\": {\n");
        rrdlabels_to_buffer(ri->rrdlabels, wb, "\t\t\t\t\t\t", ":", "\"", ",\n", NULL, NULL, NULL, NULL);
        buffer_strcat(wb, "\n\t\t\t\t\t}");
    }

    if(wb_metrics) {
        buffer_sprintf(wb, ",\n\t\t\t\t\t\"dimensions\": {");
        buffer_fast_strcat(wb, buffer_tostring(wb_metrics), buffer_strlen(wb_metrics));
        buffer_strcat(wb, "\n\t\t\t\t\t}");

        buffer_free(wb_metrics);
    }

    buffer_strcat(wb, "\n\t\t\t\t}");
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

    if(after && (!rc->last_time_t || after > rc->last_time_t))
        return 0;

    if(before && (!rc->first_time_t || before < rc->first_time_t))
        return 0;

    time_t first_time_t = rc->first_time_t;
    time_t last_time_t = rc->last_time_t;
    RRD_FLAGS flags = rrd_flags_get(rc);

    BUFFER *wb_instances = NULL;
    if((options & (RRDCONTEXT_OPTION_SHOW_LABELS|RRDCONTEXT_OPTION_SHOW_INSTANCES|RRDCONTEXT_OPTION_SHOW_METRICS))
        || t_parent->chart_label_key
        || t_parent->chart_labels_filter
        || t_parent->chart_dimensions) {

        wb_instances = buffer_create(4096);

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

        first_time_t = t_instances.combined_first_time_t;
        last_time_t = t_instances.combined_last_time_t;
        flags = t_instances.combined_flags;
    }

    if(t_parent->written)
        buffer_strcat(wb, ",\n");
    else
        buffer_strcat(wb, "\n");

    if(options & RRDCONTEXT_OPTION_SKIP_ID)
        buffer_sprintf(wb, "\t\t\{");
    else
        buffer_sprintf(wb, "\t\t\"%s\": {", id);

    rrdcontext_lock(rc);

    buffer_sprintf(wb,
                   "\n\t\t\t\"title\":\"%s\""
                   ",\n\t\t\t\"units\":\"%s\""
                   ",\n\t\t\t\"family\":\"%s\""
                   ",\n\t\t\t\"chart_type\":\"%s\""
                   ",\n\t\t\t\"priority\":%u"
                   ",\n\t\t\t\"first_time_t\":%ld"
                   ",\n\t\t\t\"last_time_t\":%ld"
                   ",\n\t\t\t\"collected\":%s"
                   , string2str(rc->title)
                   , string2str(rc->units)
                   , string2str(rc->family)
                   , rrdset_type_name(rc->chart_type)
                   , rc->priority
                   , first_time_t
                   , (flags & RRD_FLAG_COLLECTED) ? t_parent->now : last_time_t
                   , (flags & RRD_FLAG_COLLECTED) ? "true" : "false"
                   );

    if(options & RRDCONTEXT_OPTION_SHOW_DELETED) {
        buffer_sprintf(wb,
                       ",\n\t\t\t\"deleted\":%s"
                       , rrd_flag_is_deleted(rc) ? "true" : "false"
        );
    }

    if(options & RRDCONTEXT_OPTION_SHOW_FLAGS) {
        buffer_strcat(wb, ",\n\t\t\t\"flags\":\"");
        rrd_flags_to_buffer(rrd_flags_get(rc), wb);
        buffer_strcat(wb, "\"");
    }

    if(options & RRDCONTEXT_OPTION_SHOW_QUEUED) {
        buffer_strcat(wb, ",\n\t\t\t\"queued_reasons\":\"");
        rrd_reasons_to_buffer(rc->queue.queued_flags, wb);
        buffer_strcat(wb, "\"");

        buffer_sprintf(wb,
                       ",\n\t\t\t\"last_queued\":%llu"
                       ",\n\t\t\t\"scheduled_dispatch\":%llu"
                       ",\n\t\t\t\"last_dequeued\":%llu"
                       ",\n\t\t\t\"hub_version\":%"PRIu64""
                       ",\n\t\t\t\"version\":%"PRIu64""
                       , rc->queue.queued_ut / USEC_PER_SEC
                       , rc->queue.scheduled_dispatch_ut / USEC_PER_SEC
                       , rc->queue.dequeued_ut / USEC_PER_SEC
                       , rc->hub.version
                       , rc->version
                       );
    }

    rrdcontext_unlock(rc);

    if(wb_instances) {
        buffer_sprintf(wb, ",\n\t\t\t\"charts\": {");
        buffer_fast_strcat(wb, buffer_tostring(wb_instances), buffer_strlen(wb_instances));
        buffer_strcat(wb, "\n\t\t\t}");

        buffer_free(wb_instances);
    }

    buffer_strcat(wb, "\n\t\t}");
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

    if(after != 0 && before != 0) {
        long long after_wanted = after;
        long long before_wanted = before;
        rrdr_relative_window_to_absolute(&after_wanted, &before_wanted);
        after = after_wanted;
        before = before_wanted;
    }

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

    if(after != 0 && before != 0) {
        long long after_wanted = after;
        long long before_wanted = before;
        rrdr_relative_window_to_absolute(&after_wanted, &before_wanted);
        after = after_wanted;
        before = before_wanted;
    }

    buffer_sprintf(wb, "{\n"
                          "\t\"hostname\": \"%s\""
                       ",\n\t\"machine_guid\": \"%s\""
                       ",\n\t\"node_id\": \"%s\""
                       ",\n\t\"claim_id\": \"%s\""
                   , rrdhost_hostname(host)
                   , host->machine_guid
                   , node_uuid
                   , host->aclk_state.claimed_id ? host->aclk_state.claimed_id : ""
                   );

    if(options & RRDCONTEXT_OPTION_SHOW_LABELS) {
        buffer_sprintf(wb, ",\n\t\"host_labels\": {\n");
        rrdlabels_to_buffer(host->rrdlabels, wb, "\t\t", ":", "\"", ",\n", NULL, NULL, NULL, NULL);
        buffer_strcat(wb, "\n\t}");
    }

    buffer_sprintf(wb, ",\n\t\"contexts\": {");
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

    // close contexts, close main
    buffer_strcat(wb, "\n\t}\n}");

    return HTTP_RESP_OK;
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
        .update_every = sc->update_every,
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

        hash += rc->hub.version + rc->hub.last_time_t - rc->hub.first_time_t;

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

static void rrdmetric_update_retention(RRDMETRIC *rm) {
    time_t min_first_time_t = LONG_MAX, max_last_time_t = 0;

    if(rm->rrddim) {
        min_first_time_t = rrddim_first_entry_t(rm->rrddim);
        max_last_time_t = rrddim_last_entry_t(rm->rrddim);
    }
#ifdef ENABLE_DBENGINE
    else {
        RRDHOST *rrdhost = rm->ri->rc->rrdhost;
        for (int tier = 0; tier < storage_tiers; tier++) {
            if(!rrdhost->storage_instance[tier]) continue;

            time_t first_time_t, last_time_t;
            if (rrdeng_metric_retention_by_uuid(rrdhost->storage_instance[tier], &rm->uuid, &first_time_t, &last_time_t) == 0) {
                if (first_time_t < min_first_time_t)
                    min_first_time_t = first_time_t;

                if (last_time_t > max_last_time_t)
                    max_last_time_t = last_time_t;
            }
        }
    }
#endif

    if(min_first_time_t == LONG_MAX)
        min_first_time_t = 0;

    if(min_first_time_t > max_last_time_t) {
        internal_error(true, "RRDMETRIC: retention of '%s' is flipped", string2str(rm->id));
        time_t tmp = min_first_time_t;
        min_first_time_t = max_last_time_t;
        max_last_time_t = tmp;
    }

    // check if retention changed

    if (min_first_time_t != rm->first_time_t) {
        rm->first_time_t = min_first_time_t;
        rrd_flag_set_updated(rm, RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T);
    }

    if (max_last_time_t != rm->last_time_t) {
        rm->last_time_t = max_last_time_t;
        rrd_flag_set_updated(rm, RRD_FLAG_UPDATE_REASON_CHANGED_LAST_TIME_T);
    }

    if(unlikely(!rm->first_time_t && !rm->last_time_t))
        rrd_flag_set_deleted(rm, RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);

    rrd_flag_set(rm, RRD_FLAG_LIVE_RETENTION);
}

static inline bool rrdmetric_should_be_deleted(RRDMETRIC *rm) {
    if(likely(!rrd_flag_check(rm, RRD_FLAGS_REQUIRED_FOR_DELETIONS)))
        return false;

    if(likely(rrd_flag_check(rm, RRD_FLAGS_PREVENTING_DELETIONS)))
        return false;

    if(likely(rm->rrddim))
        return false;

    rrdmetric_update_retention(rm);
    if(rm->first_time_t || rm->last_time_t)
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

    if(ri->first_time_t || ri->last_time_t)
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

    if(unlikely(rc->first_time_t || rc->last_time_t))
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
        if(unlikely(netdata_exit)) break;

        if(worker_jobs) worker_is_busy(WORKER_JOB_CLEANUP);

        rrdcontext_lock(rc);

        RRDINSTANCE *ri;
        dfe_start_reentrant(rc->rrdinstances, ri) {
            if(unlikely(netdata_exit)) break;

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

            fprintf(stderr, "RRDCONTEXT: deleted context '%s'", string2str(rc->id));
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

    if(!force && !rrd_flag_is_updated(rm) && rrd_flag_check(rm, RRD_FLAG_LIVE_RETENTION))
        return;

    if(worker_jobs)
        worker_is_busy(WORKER_JOB_PP_METRIC);

    if(reason == RRD_FLAG_UPDATE_REASON_DISCONNECTED_CHILD) {
        rrd_flag_set_archived(rm);
        rrd_flag_set(rm, RRD_FLAG_UPDATE_REASON_DISCONNECTED_CHILD);
    }

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
            if(unlikely(netdata_exit)) break;

            rrdmetric_process_updates(rm, force, reason, worker_jobs);

            if(unlikely(!rrd_flag_check(rm, RRD_FLAG_LIVE_RETENTION)))
                live_retention = false;

            if (unlikely((rrdmetric_should_be_deleted(rm)))) {
                metrics_deleted++;
                continue;
            }

            if(!currently_collected && rrd_flag_check(rm, RRD_FLAG_COLLECTED) && rm->first_time_t)
                currently_collected = true;

            metrics_active++;

            if (rm->first_time_t && rm->first_time_t < min_first_time_t)
                min_first_time_t = rm->first_time_t;

            if (rm->last_time_t && rm->last_time_t > max_last_time_t)
                max_last_time_t = rm->last_time_t;
        }
        dfe_done(rm);
    }

    if(unlikely(live_retention && !rrd_flag_check(ri, RRD_FLAG_LIVE_RETENTION)))
        rrd_flag_set(ri, RRD_FLAG_LIVE_RETENTION);
    else if(unlikely(!live_retention && rrd_flag_check(ri, RRD_FLAG_LIVE_RETENTION)))
        rrd_flag_clear(ri, RRD_FLAG_LIVE_RETENTION);

    if(unlikely(!metrics_active)) {
        // no metrics available

        if(ri->first_time_t) {
            ri->first_time_t = 0;
            rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T);
        }

        if(ri->last_time_t) {
            ri->last_time_t = 0;
            rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_LAST_TIME_T);
        }

        rrd_flag_set_deleted(ri, RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);
    }
    else {
        // we have active metrics...

        if (unlikely(min_first_time_t == LONG_MAX))
            min_first_time_t = 0;

        if (unlikely(min_first_time_t == 0 || max_last_time_t == 0)) {
            if(ri->first_time_t) {
                ri->first_time_t = 0;
                rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T);
            }

            if(ri->last_time_t) {
                ri->last_time_t = 0;
                rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_LAST_TIME_T);
            }

            if(likely(live_retention))
                rrd_flag_set_deleted(ri, RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);
        }
        else {
            rrd_flag_clear(ri, RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);

            if (unlikely(ri->first_time_t != min_first_time_t)) {
                ri->first_time_t = min_first_time_t;
                rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T);
            }

            if (unlikely(ri->last_time_t != max_last_time_t)) {
                ri->last_time_t = max_last_time_t;
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

    size_t min_priority = LONG_MAX;
    time_t min_first_time_t = LONG_MAX, max_last_time_t = 0;
    size_t instances_active = 0, instances_deleted = 0;
    bool live_retention = true, currently_collected = false, hidden = true;
    if(dictionary_entries(rc->rrdinstances) > 0) {
        RRDINSTANCE *ri;
        dfe_start_reentrant(rc->rrdinstances, ri) {
            if(unlikely(netdata_exit)) break;

            rrdinstance_post_process_updates(ri, force, reason, worker_jobs);

            if(unlikely(hidden && !rrd_flag_check(ri, RRD_FLAG_HIDDEN)))
                hidden = false;

            if(unlikely(live_retention && !rrd_flag_check(ri, RRD_FLAG_LIVE_RETENTION)))
                live_retention = false;

            if (unlikely(rrdinstance_should_be_deleted(ri))) {
                instances_deleted++;
                continue;
            }

            if(unlikely(!currently_collected && rrd_flag_is_collected(ri) && ri->first_time_t))
                currently_collected = true;

            internal_error(rc->units != ri->units,
                           "RRDCONTEXT: '%s' rrdinstance '%s' has different units, context '%s', instance '%s'",
                           string2str(rc->id), string2str(ri->id),
                           string2str(rc->units), string2str(ri->units));

            instances_active++;

            if (ri->priority >= RRDCONTEXT_MINIMUM_ALLOWED_PRIORITY && ri->priority < min_priority)
                min_priority = ri->priority;

            if (ri->first_time_t && ri->first_time_t < min_first_time_t)
                min_first_time_t = ri->first_time_t;

            if (ri->last_time_t && ri->last_time_t > max_last_time_t)
                max_last_time_t = ri->last_time_t;
        }
        dfe_done(ri);
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

    if(unlikely(!instances_active)) {
        // we had some instances, but they are gone now...

        if(rc->first_time_t) {
            rc->first_time_t = 0;
            rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T);
        }

        if(rc->last_time_t) {
            rc->last_time_t = 0;
            rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_LAST_TIME_T);
        }

        rrd_flag_set_deleted(rc, RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);
    }
    else {
        // we have some active instances...

        if (unlikely(min_first_time_t == LONG_MAX))
            min_first_time_t = 0;

        if (unlikely(min_first_time_t == 0 && max_last_time_t == 0)) {
            if(rc->first_time_t) {
                rc->first_time_t = 0;
                rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T);
            }

            if(rc->last_time_t) {
                rc->last_time_t = 0;
                rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_LAST_TIME_T);
            }

            rrd_flag_set_deleted(rc, RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);
        }
        else {
            rrd_flag_clear(rc, RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);

            if (unlikely(rc->first_time_t != min_first_time_t)) {
                rc->first_time_t = min_first_time_t;
                rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T);
            }

            if (rc->last_time_t != max_last_time_t) {
                rc->last_time_t = max_last_time_t;
                rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_LAST_TIME_T);
            }

            if(likely(currently_collected))
                rrd_flag_set_collected(rc);
            else
                rrd_flag_set_archived(rc);
        }

        if (min_priority != LONG_MAX && rc->priority != min_priority) {
            rc->priority = min_priority;
            rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_PRIORITY);
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

    if(!rrd_flag_check(rc, RRD_FLAG_QUEUED_FOR_POST_PROCESSING)) {
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
        if(unlikely(netdata_exit)) break;

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
    rc->hub.first_time_t = rc->first_time_t;
    rc->hub.last_time_t = rrd_flag_is_collected(rc) ? 0 : rc->last_time_t;
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
        .first_entry = rc->hub.first_time_t,
        .last_entry = rc->hub.last_time_t,
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

    if(unlikely((uint64_t)rc->first_time_t != rc->hub.first_time_t))
        first_time_changed = true;

    if(unlikely((uint64_t)((flags & RRD_FLAG_COLLECTED) ? 0 : rc->last_time_t) != rc->hub.last_time_t))
        last_time_changed = true;

    if(unlikely(((flags & RRD_FLAG_DELETED) ? true : false) != rc->hub.deleted))
        deleted_changed = true;

    if(unlikely(id_changed || title_changed || units_changed || family_changed || chart_type_changed || priority_changed || first_time_changed || last_time_changed || deleted_changed)) {

        internal_error(LOG_TRANSITIONS,
                       "RRDCONTEXT: %s NEW VERSION '%s'%s, version %"PRIu64", title '%s'%s, units '%s'%s, family '%s'%s, chart type '%s'%s, priority %u%s, first_time_t %ld%s, last_time_t %ld%s, deleted '%s'%s, (queued for %llu ms, expected %llu ms)",
                       sending?"SENDING":"QUEUE",
                       string2str(rc->id), id_changed ? " (CHANGED)" : "",
                       rc->version,
                       string2str(rc->title), title_changed ? " (CHANGED)" : "",
                       string2str(rc->units), units_changed ? " (CHANGED)" : "",
                       string2str(rc->family), family_changed ? " (CHANGED)" : "",
                       rrdset_type_name(rc->chart_type), chart_type_changed ? " (CHANGED)" : "",
                       rc->priority, priority_changed ? " (CHANGED)" : "",
                       rc->first_time_t, first_time_changed ? " (CHANGED)" : "",
                       (flags & RRD_FLAG_COLLECTED) ? 0 : rc->last_time_t, last_time_changed ? " (CHANGED)" : "",
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
        if(unlikely(netdata_exit)) break;

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
    if(!netdata_exit && bundle) {
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
    worker_register_job_name(WORKER_JOB_DEQUEUE, "deduped contexts");
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

    while (!netdata_exit) {
        worker_is_idle();
        heartbeat_next(&hb, step);

        if(unlikely(netdata_exit)) break;

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
            if(unlikely(netdata_exit)) break;

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

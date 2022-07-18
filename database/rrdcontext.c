// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdcontext.h"
#include "sqlite/sqlite_context.h"
#include "aclk/schema-wrappers/context.h"
#include "aclk/aclk_contexts_api.h"
#include "aclk/aclk_api.h"

int rrdcontext_enabled = CONFIG_BOOLEAN_YES;

#define MESSAGES_PER_BUNDLE_TO_SEND_TO_HUB_PER_HOST         5000
#define FULL_RETENTION_SCAN_DELAY_AFTER_DB_ROTATION_SECS    120
#define RRDCONTEXT_WORKER_THREAD_HEARTBEAT_SECS             1

// #define LOG_TRANSITIONS 1
// #define LOG_RRDINSTANCES 1

typedef enum {
    RRD_FLAG_NONE           = 0,
    RRD_FLAG_DELETED        = (1 << 0), // this is a deleted object (metrics, instances, contexts)
    RRD_FLAG_COLLECTED      = (1 << 1), // this object is currently being collected
    RRD_FLAG_UPDATED        = (1 << 2), // this object has updates to propagate
    RRD_FLAG_ARCHIVED       = (1 << 3), // this object is not currently being collected
    RRD_FLAG_OWN_LABELS     = (1 << 4), // this instance has its own labels - not linked to an RRDSET
    RRD_FLAG_LIVE_RETENTION = (1 << 5), // we have got live retention from the database
    RRD_FLAG_QUEUED         = (1 << 6), // this context is currently queued to be dispatched to hub

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
} RRD_FLAGS;

#define RRD_FLAG_ALL_UPDATE_REASONS                   ( \
     RRD_FLAG_UPDATE_REASON_LOAD_SQL                    \
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
)

#define RRD_FLAGS_PROPAGATED_UPSTREAM                 ( \
     RRD_FLAG_COLLECTED                                 \
    |RRD_FLAG_DELETED                                   \
    |RRD_FLAG_LIVE_RETENTION                            \
    |RRD_FLAG_ALL_UPDATE_REASONS                        \
 )

#define RRD_FLAGS_ALLOWED_EXTERNALLY_ON_NEW_OBJECTS   ( \
     RRD_FLAG_ARCHIVED                                  \
    |RRD_FLAG_ALL_UPDATE_REASONS                        \
 )

#define RRD_FLAGS_PREVENTING_DELETIONS                ( \
     RRD_FLAG_QUEUED                                    \
    |RRD_FLAG_COLLECTED                                 \
    |RRD_FLAG_UPDATE_REASON_LOAD_SQL                    \
    |RRD_FLAG_UPDATE_REASON_NEW_OBJECT                  \
    |RRD_FLAG_UPDATE_REASON_UPDATED_OBJECT              \
    |RRD_FLAG_UPDATE_REASON_CHANGED_LINKING             \
)

#define rrd_flag_set_updated(obj, reason) (obj)->flags |=  (RRD_FLAG_UPDATED | (reason))
#define rrd_flag_unset_updated(obj)       (obj)->flags &= ~(RRD_FLAG_UPDATED | RRD_FLAG_ALL_UPDATE_REASONS)

#define rrd_flag_set_collected(obj)                                                                        do { \
        if(likely(!((obj)->flags &    RRD_FLAG_COLLECTED)))                                                     \
                    (obj)->flags |=  (RRD_FLAG_COLLECTED | RRD_FLAG_UPDATE_REASON_STARTED_BEING_COLLECTED | RRD_FLAG_UPDATED); \
        if(likely( ((obj)->flags &   (RRD_FLAG_ARCHIVED  | RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED))))   \
                    (obj)->flags &= ~(RRD_FLAG_ARCHIVED  | RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED); \
} while(0)

#define rrd_flag_set_archived(obj)                                                                         do { \
        if(likely(!((obj)->flags &    RRD_FLAG_ARCHIVED)))                                                      \
                    (obj)->flags |=  (RRD_FLAG_ARCHIVED  | RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED | RRD_FLAG_UPDATED); \
        if(likely( ((obj)->flags &   (RRD_FLAG_COLLECTED | RRD_FLAG_UPDATE_REASON_STARTED_BEING_COLLECTED))))   \
                    (obj)->flags &= ~(RRD_FLAG_COLLECTED | RRD_FLAG_UPDATE_REASON_STARTED_BEING_COLLECTED); \
} while(0)

#define rrd_flag_is_collected(obj) ((obj)->flags & RRD_FLAG_COLLECTED)
#define rrd_flag_is_archived(obj) ((obj)->flags & RRD_FLAG_ARCHIVED)

static struct rrdcontext_reason {
    RRD_FLAGS flag;
    const char *name;
    usec_t delay_ut;
} rrdcontext_reasons[] = {
    // context related
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

    // not context related
    { RRD_FLAG_UPDATE_REASON_CHANGED_UUID,            "changed uuid",         60 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_CHANGED_UPDATE_EVERY,    "changed updated every",60 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_CHANGED_LINKING,         "changed rrd link",     60 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_CHANGED_NAME,            "changed name",         60 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_DISCONNECTED_CHILD,      "child disconnected",   30 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_DB_ROTATION,             "db rotation",          60 * USEC_PER_SEC },

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

    usec_t created_ut;                  // the time this object was created
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

    DICTIONARY *rrdlabels;              // linked to RRDSET->state->chart_labels or own version

    struct rrdcontext *rc;
    DICTIONARY *rrdmetrics;
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

static inline RRDINSTANCE_ACQUIRED *rrdinstance_dup(RRDINSTANCE_ACQUIRED *ria) {
    return (RRDINSTANCE_ACQUIRED *)dictionary_acquired_item_dup((DICTIONARY_ITEM *)ria);
}

static inline RRDINSTANCE *rrdinstance_acquired_value(RRDINSTANCE_ACQUIRED *ria) {
    return dictionary_acquired_item_value((DICTIONARY_ITEM *)ria);
}

static inline const char *rrdinstance_acquired_name(RRDINSTANCE_ACQUIRED *ria) {
    return dictionary_acquired_item_name((DICTIONARY_ITEM *)ria);
}

static inline void rrdinstance_release(RRDINSTANCE_ACQUIRED *ria) {
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    dictionary_acquired_item_release(ri->rc->rrdinstances, (DICTIONARY_ITEM *)ria);
}

// ----------------------------------------------------------------------------
// helper one-liners for RRDCONTEXT

static inline RRDCONTEXT_ACQUIRED *rrdcontext_dup(RRDCONTEXT_ACQUIRED *rca) {
    return (RRDCONTEXT_ACQUIRED *)dictionary_acquired_item_dup((DICTIONARY_ITEM *)rca);
}

static inline const char *rrdcontext_acquired_name(RRDCONTEXT_ACQUIRED *rca) {
    return dictionary_acquired_item_name((DICTIONARY_ITEM *)rca);
}

static inline RRDCONTEXT *rrdcontext_acquired_value(RRDCONTEXT_ACQUIRED *rca) {
    return dictionary_acquired_item_value((DICTIONARY_ITEM *)rca);
}

static inline RRDCONTEXT_ACQUIRED *rrdcontext_acquire(RRDHOST *host, const char *name) {
    return (RRDCONTEXT_ACQUIRED *)dictionary_get_and_acquire_item((DICTIONARY *)host->rrdctx, name);
}

static inline void rrdcontext_release(RRDCONTEXT_ACQUIRED *rca) {
    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);
    dictionary_acquired_item_release((DICTIONARY *)rc->rrdhost->rrdctx, (DICTIONARY_ITEM *)rca);
}

static void rrdcontext_recalculate_context_retention(RRDCONTEXT *rc, RRD_FLAGS reason, int job_id);
static void rrdcontext_recalculate_host_retention(RRDHOST *host, RRD_FLAGS reason, int job_id);

#define rrdcontext_version_hash(host) rrdcontext_version_hash_with_callback(host, NULL, false, NULL)
static uint64_t rrdcontext_version_hash_with_callback(RRDHOST *host, void (*callback)(RRDCONTEXT *, bool, void *), bool snapshot, void *bundle);

void rrdcontext_delete_from_sql_unsafe(RRDCONTEXT *rc);

#define rrdcontext_lock(rc) netdata_mutex_lock(&((rc)->mutex))
#define rrdcontext_unlock(rc) netdata_mutex_unlock(&((rc)->mutex))

// ----------------------------------------------------------------------------
// Updates triggers

static void rrdmetric_trigger_updates(RRDMETRIC *rm, bool force, bool escalate);
static void rrdinstance_trigger_updates(RRDINSTANCE *ri, bool force, bool escalate);
static void rrdcontext_trigger_updates(RRDCONTEXT *rc, bool force, RRD_FLAGS reason);

// ----------------------------------------------------------------------------
// visualizing flags

static void rrd_flags_to_buffer(RRD_FLAGS flags, BUFFER *wb) {
    if(flags & RRD_FLAG_QUEUED)
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
// logging of all data collected

#ifdef LOG_TRANSITIONS
static void log_transition(STRING *metric, STRING *instance, STRING *context, RRD_FLAGS flags, const char *msg) {
    BUFFER *wb = buffer_create(1000);

    buffer_sprintf(wb, "RRD TRANSITION: context '%s'", string2str(context));

    if(instance)
        buffer_sprintf(wb, ", instance '%s'", string2str(instance));

    if(metric)
        buffer_sprintf(wb, ", metric '%s'", string2str(metric));

    buffer_sprintf(wb, ", triggered by %s: ", msg);

    rrd_flags_to_buffer(flags, wb);

    buffer_strcat(wb, ", reasons: ");

    rrd_reasons_to_buffer(flags, wb);

    internal_error(true, "%s", buffer_tostring(wb));
    buffer_free(wb);
}
#else
#define log_transition(metric, instance, context, flags, msg) debug_dummy()
#endif

#ifdef LOG_RRDINSTANCES
static void rrdinstance_log(RRDINSTANCE *ri, const char *msg) {
    char uuid[UUID_STR_LEN];

    uuid_unparse(ri->uuid, uuid);

    BUFFER *wb = buffer_create(1000);

    buffer_sprintf(wb,
                   "RRDINSTANCE: %s id '%s' (host '%s'), uuid '%s', name '%s', context '%s', title '%s', units '%s', family '%s', priority %zu, chart type '%s', update every %d, rrdset '%s', flags %s%s%s%s%s%s%s, first_time_t %ld, last_time_t %ld",
                   msg,
                   string2str(ri->id),
                   ri->rc->rrdhost->hostname,
                   uuid,
                   string2str(ri->name),
                   string2str(ri->rc->id),
                   string2str(ri->title),
                   string2str(ri->units),
                   string2str(ri->family),
                   ri->priority,
                   rrdset_type_name(ri->chart_type),
                   ri->update_every,
                   ri->rrdset?ri->rrdset->id:"NONE",
                   ri->flags & RRD_FLAG_DELETED ?"DELETED ":"",
                   ri->flags & RRD_FLAG_UPDATED ?"UPDATED ":"",
                   rrd_flag_is_collected(ri) ?"COLLECTED ":"",
                   rrd_flag_is_archived(ri) ?"ARCHIVED ":"",
                   ri->flags & RRD_FLAG_OWNLABELS ?"OWNLABELS ":"",
                   ri->flags & RRD_FLAG_LIVE_RETENTION ?"LIVE ":"",
                   ri->flags & RRD_FLAG_QUEUED ?"QUEUED ":"",
                   ri->first_time_t,
                   ri->last_time_t
                   );

    buffer_strcat(wb, ", update reasons: { ");
    for(int i = 0, added = 0; rrdcontext_reasons[i].name ;i++)
        if(ri->flags & rrdcontext_reasons[i].flag) {
            if(added) buffer_strcat(wb, ", ");
            buffer_strcat(wb, rrdcontext_reasons[i].name);
            added++;
        }
    buffer_strcat(wb, " }");

    buffer_strcat(wb, ", labels: { ");
    if(ri->rrdlabels) {
        if(!rrdlabels_to_buffer(ri->rrdlabels, wb, "", "=", "'", ", ", NULL, NULL, NULL, NULL))
            buffer_strcat(wb, "EMPTY }");
        else
            buffer_strcat(wb, " }");
    }
    else
        buffer_strcat(wb, "NONE }");

    buffer_strcat(wb, ", metrics: { ");
    if(ri->rrdmetrics) {
        RRDMETRIC *v;
        int i = 0;
        dfe_start_read((DICTIONARY *)ri->rrdmetrics, v) {
            buffer_sprintf(wb, "%s%s", i?",":"", v_name);
            i++;
        }
        dfe_done(v);

        if(!i)
            buffer_strcat(wb, "EMPTY }");
        else
            buffer_strcat(wb, " }");
    }
    else
        buffer_strcat(wb, "NONE }");

    internal_error(true, "%s", buffer_tostring(wb));
    buffer_free(wb);
}
#else
#define rrdinstance_log(ir, msg) debug_dummy()
#endif

// ----------------------------------------------------------------------------
// RRDMETRIC

static void rrdmetric_free(RRDMETRIC *rm) {
    string_freez(rm->id);
    string_freez(rm->name);

    rm->id = NULL;
    rm->name = NULL;
    rm->ri = NULL;
}

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

    if(rm->first_time_t == 0 && rm->last_time_t == 0 && (!(rm->flags & RRD_FLAG_DELETED))) {
        rm->flags |= RRD_FLAG_DELETED;
        rrd_flag_set_updated(rm, RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);
    }

    rm->flags |= RRD_FLAG_LIVE_RETENTION;
}

// called when this rrdmetric is inserted to the rrdmetrics dictionary of a rrdinstance
static void rrdmetric_insert_callback(const char *id __maybe_unused, void *value, void *data) {
    RRDMETRIC *rm = value;

    // link it to its parent
    rm->ri = data;

    // remove flags that we need to figure out at runtime
    rm->flags = rm->flags & RRD_FLAGS_ALLOWED_EXTERNALLY_ON_NEW_OBJECTS;

    rm->created_ut = now_realtime_usec();

    // signal the react callback to do the job
    rrd_flag_set_updated(rm, RRD_FLAG_UPDATE_REASON_NEW_OBJECT);
}

// called when this rrdmetric is deleted from the rrdmetrics dictionary of a rrdinstance
static void rrdmetric_delete_callback(const char *id __maybe_unused, void *value, void *data __maybe_unused) {
    RRDMETRIC *rm = value;

    if(rm->rrddim)
        fatal("RRDMETRIC: '%s' is freed but there is a RRDDIM linked to it.", string2str(rm->id));

    // free the resources
    rrdmetric_free(rm);
}

// called when the same rrdmetric is inserted again to the rrdmetrics dictionary of a rrdinstance
static void rrdmetric_conflict_callback(const char *id __maybe_unused, void *oldv, void *newv, void *data __maybe_unused) {
    RRDMETRIC *rm     = oldv;
    RRDMETRIC *rm_new = newv;

    if(rm->id != rm_new->id)
        fatal("RRDMETRIC: '%s' cannot change id to '%s'", string2str(rm->id), string2str(rm_new->id));

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
        fatal("RRDMETRIC: '%s' is linked to RRDDIM '%s' but they have different UUIDs. RRDMETRIC has '%s', RRDDIM has '%s'", string2str(rm->id), rm->rrddim->id, uuid1, uuid2);
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

    rm->flags |= (rm_new->flags & RRD_FLAGS_ALLOWED_EXTERNALLY_ON_NEW_OBJECTS);

    if(rrd_flag_is_collected(rm) && rrd_flag_is_archived(rm))
        rrd_flag_set_collected(rm);

    if(rm->flags & RRD_FLAG_UPDATED)
        rm->flags |= RRD_FLAG_UPDATE_REASON_UPDATED_OBJECT;

    rrdmetric_free(rm_new);

    // the react callback will continue from here
}

static void rrdmetric_react_callback(const char *id __maybe_unused, void *value, void *data __maybe_unused) {
    RRDMETRIC *rm = value;

    rrdmetric_trigger_updates(rm, false, true);
}

static void rrdmetrics_create(RRDINSTANCE *ri) {
    if(unlikely(!ri)) return;
    if(likely(ri->rrdmetrics)) return;

    ri->rrdmetrics = dictionary_create(DICTIONARY_FLAG_DONT_OVERWRITE_VALUE);
    dictionary_register_insert_callback(ri->rrdmetrics, rrdmetric_insert_callback, (void *)ri);
    dictionary_register_delete_callback(ri->rrdmetrics, rrdmetric_delete_callback, (void *)ri);
    dictionary_register_conflict_callback(ri->rrdmetrics, rrdmetric_conflict_callback, (void *)ri);
    dictionary_register_react_callback(ri->rrdmetrics, rrdmetric_react_callback, (void *)ri);
}

static void rrdmetrics_destroy(RRDINSTANCE *ri) {
    if(unlikely(!ri || !ri->rrdmetrics)) return;
    dictionary_destroy(ri->rrdmetrics);
    ri->rrdmetrics = NULL;
}

static inline bool rrdmetric_should_be_deleted(RRDMETRIC *rm) {
    if(likely(!(rm->flags & RRD_FLAG_DELETED)))
        return false;

    if(likely(!(rm->flags & RRD_FLAG_LIVE_RETENTION)))
        return false;

    if(unlikely(rm->flags & RRD_FLAGS_PREVENTING_DELETIONS))
        return false;

    if(likely(rm->rrddim))
        return false;

    if((now_realtime_usec() - rm->created_ut) < 600 * USEC_PER_SEC)
        return false;

    rrdmetric_update_retention(rm);
    if(rm->first_time_t || rm->last_time_t)
        return false;

    return true;
}

static void rrdmetric_trigger_updates(RRDMETRIC *rm, bool force, bool escalate) {
    if(likely(!force && !(rm->flags & RRD_FLAG_UPDATED))) return;

    if(unlikely((rm->flags & RRD_FLAG_UPDATE_REASON_DISCONNECTED_CHILD) && rrd_flag_is_collected(rm)))
        rrd_flag_set_archived(rm);

    rrdmetric_update_retention(rm);

    if(unlikely(escalate && rm->flags & RRD_FLAG_UPDATED)) {
        log_transition(rm->id, rm->ri->id, rm->ri->rc->id, rm->flags, "RRDMETRIC");
        rrdinstance_trigger_updates(rm->ri, true, true);
        rrd_flag_unset_updated(rm);
    }
}

static inline void rrdmetric_from_rrddim(RRDDIM *rd) {
    if(unlikely(!rd->rrdset))
        fatal("RRDMETRIC: rrddim '%s' does not have a rrdset.", rd->id);

    if(unlikely(!rd->rrdset->rrdhost))
        fatal("RRDMETRIC: rrdset '%s' does not have a rrdhost", rd->rrdset->id);

    if(unlikely(!rd->rrdset->rrdinstance))
        fatal("RRDMETRIC: rrdset '%s' does not have a rrdinstance", rd->rrdset->id);

    RRDINSTANCE *ri = rrdinstance_acquired_value(rd->rrdset->rrdinstance);

    if(unlikely(!ri->rrdmetrics))
        fatal("RRDMETRIC: rrdinstance '%s' does not have a rrdmetrics dictionary", string2str(ri->id));

    RRDMETRIC trm = {
        .id = string_strdupz(rd->id),
        .name = string_strdupz(rd->name),
        .flags = RRD_FLAG_NONE,
        .rrddim = rd,
    };
    uuid_copy(trm.uuid, rd->metric_uuid);

    RRDMETRIC_ACQUIRED *rma = (RRDMETRIC_ACQUIRED *)dictionary_set_and_acquire_item(ri->rrdmetrics, string2str(trm.id), &trm, sizeof(trm));

    if(rd->rrdmetric && rd->rrdmetric != rma)
        fatal("RRDMETRIC: dimension '%s' of chart '%s' changed rrdmetric!", rd->id, rd->rrdset->id);
    else if(!rd->rrdmetric)
        rd->rrdmetric = rma;
}

#define rrddim_get_rrdmetric(rd) rrddim_get_rrdmetric_with_trace(rd, __FUNCTION__)
static inline RRDMETRIC *rrddim_get_rrdmetric_with_trace(RRDDIM *rd, const char *function) {
    if(unlikely(!rd->rrdmetric))
        fatal("RRDMETRIC: RRDDIM '%s' is not linked to an RRDMETRIC at %s()", rd->id, function);

    RRDMETRIC *rm = rrdmetric_acquired_value(rd->rrdmetric);

    if(unlikely(rm->rrddim != rd))
        fatal("RRDMETRIC: '%s' is not linked to RRDDIM '%s' at %s()", string2str(rm->id), rd->id, function);

    return rm;
}

static inline void rrdmetric_rrddim_is_freed(RRDDIM *rd) {
    RRDMETRIC *rm = rrddim_get_rrdmetric(rd);

    if(unlikely(rrd_flag_is_collected(rm)))
        rrd_flag_set_archived(rm);

    rm->rrddim = NULL;
    rrdmetric_trigger_updates(rm, false, true);
    rrdmetric_release(rd->rrdmetric);
    rd->rrdmetric = NULL;
}

static inline void rrdmetric_updated_rrddim_flags(RRDDIM *rd) {
    RRDMETRIC *rm = rrddim_get_rrdmetric(rd);

    if(unlikely(rd->flags & (RRDDIM_FLAG_ARCHIVED | RRDDIM_FLAG_OBSOLETE))) {
        if(unlikely(rrd_flag_is_collected(rm)))
            rrd_flag_set_archived(rm);
    }

    rrdmetric_trigger_updates(rm, false, true);
}

static inline void rrdmetric_collected_rrddim(RRDDIM *rd) {
    RRDMETRIC *rm = rrddim_get_rrdmetric(rd);

    if(unlikely(!rrd_flag_is_collected(rm)))
        rrd_flag_set_collected(rm);

    rrdmetric_trigger_updates(rm, false, true);
}

// ----------------------------------------------------------------------------
// RRDINSTANCE

static void rrdinstance_check(RRDINSTANCE *ri) {
    if(unlikely(!ri->id))
        fatal("RRDINSTANCE: created without an id");

    if(unlikely(!ri->name))
        fatal("RRDINSTANCE: '%s' created without a name", string2str(ri->id));

    if(unlikely(!ri->title))
        fatal("RRDINSTANCE: '%s' created without a title", string2str(ri->id));

    if(unlikely(!ri->units))
        fatal("RRDINSTANCE: '%s' created without units", string2str(ri->id));

    if(unlikely(!ri->priority))
        fatal("RRDINSTANCE: '%s' created without a priority", string2str(ri->id));

    if(unlikely(!ri->update_every))
        fatal("RRDINSTANCE: '%s' created without an update_every", string2str(ri->id));
}

static void rrdinstance_free(RRDINSTANCE *ri) {

    if(ri->flags & RRD_FLAG_OWN_LABELS)
        dictionary_destroy(ri->rrdlabels);

    rrdmetrics_destroy(ri);
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

static void rrdinstance_insert_callback(const char *id __maybe_unused, void *value, void *data) {
    RRDINSTANCE *ri = value;

    // link it to its parent
    ri->rc = data;

    if(!ri->name)
        ri->name = string_dup(ri->id);

    rrdinstance_check(ri);

    if(ri->rrdset && ri->rrdset->state) {
        ri->rrdlabels = ri->rrdset->state->chart_labels;
        if(ri->flags & RRD_FLAG_OWN_LABELS)
            ri->flags &= ~RRD_FLAG_OWN_LABELS;
    }
    else {
        ri->rrdlabels = rrdlabels_create();
        ri->flags |= RRD_FLAG_OWN_LABELS;
    }

    rrdmetrics_create(ri);

    rrdinstance_log(ri, "INSERT");

    // signal the react callback to do the job
    rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_NEW_OBJECT);
}

static void rrdinstance_delete_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDCONTEXT *rc = data; (void)rc;
    RRDINSTANCE *ri = (RRDINSTANCE *)value;

    rrdinstance_log(ri, "DELETE");

    if(ri->rrdset)
        fatal("RRDINSTANCE: '%s' is freed but there is a RRDSET linked to it.", string2str(ri->id));

    rrdinstance_free(ri);
}

static void rrdinstance_conflict_callback(const char *id __maybe_unused, void *oldv, void *newv, void *data __maybe_unused) {
    RRDINSTANCE *ri     = (RRDINSTANCE *)oldv;
    RRDINSTANCE *ri_new = (RRDINSTANCE *)newv;

    rrdinstance_check(ri_new);

    if(ri->id != ri_new->id)
        fatal("RRDINSTANCE: '%s' cannot change id to '%s'", string2str(ri->id), string2str(ri_new->id));

    if(uuid_compare(ri->uuid, ri_new->uuid) != 0) {
        uuid_copy(ri->uuid, ri_new->uuid);
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_UUID);
    }

    if(ri->rrdset && ri_new->rrdset && ri->rrdset != ri_new->rrdset) {
        ri->rrdset = ri_new->rrdset;
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_LINKING);
    }

    if(ri->rrdset && ri->rrdset->chart_uuid && uuid_compare(ri->uuid, *ri->rrdset->chart_uuid) != 0) {
        char uuid1[UUID_STR_LEN], uuid2[UUID_STR_LEN];
        uuid_unparse(ri->uuid, uuid1);
        uuid_unparse(*ri->rrdset->chart_uuid, uuid2);
        fatal("RRDINSTANCE: '%s' is linked to RRDSET '%s' but they have different UUIDs. RRDINSTANCE has '%s', RRDSET has '%s'", string2str(ri->id), ri->rrdset->id, uuid1, uuid2);
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

        if(ri->flags & RRD_FLAG_OWN_LABELS) {
            DICTIONARY *old = ri->rrdlabels;
            ri->rrdlabels = ri->rrdset->state->chart_labels;
            ri->flags &= ~RRD_FLAG_OWN_LABELS;
            rrdlabels_destroy(old);
        }
    }

    ri->flags |= (ri_new->flags & RRD_FLAGS_ALLOWED_EXTERNALLY_ON_NEW_OBJECTS);

    if(rrd_flag_is_collected(ri) && rrd_flag_is_archived(ri))
        rrd_flag_set_collected(ri);

    if(ri->flags & RRD_FLAG_UPDATED)
        ri->flags |= RRD_FLAG_UPDATE_REASON_UPDATED_OBJECT;

    rrdinstance_log(ri, "CONFLICT");

    // free the new one
    rrdinstance_free(ri_new);

    // the react callback will continue from here
}

static void rrdinstance_react_callback(const char *id __maybe_unused, void *value, void *data __maybe_unused) {
    RRDINSTANCE *ri = value;

    rrdinstance_trigger_updates(ri, false, true);
}

void rrdinstances_create(RRDCONTEXT *rc) {
    if(unlikely(rrdcontext_enabled == CONFIG_BOOLEAN_NO))
        return;

    if(unlikely(!rc || rc->rrdinstances)) return;

    rc->rrdinstances = dictionary_create(DICTIONARY_FLAG_DONT_OVERWRITE_VALUE);
    dictionary_register_insert_callback(rc->rrdinstances, rrdinstance_insert_callback, (void *)rc);
    dictionary_register_delete_callback(rc->rrdinstances, rrdinstance_delete_callback, (void *)rc);
    dictionary_register_conflict_callback(rc->rrdinstances, rrdinstance_conflict_callback, (void *)rc);
    dictionary_register_react_callback(rc->rrdinstances, rrdinstance_react_callback, (void *)rc);
}

void rrdinstances_destroy(RRDCONTEXT *rc) {
    if(unlikely(!rc || !rc->rrdinstances)) return;

    dictionary_destroy(rc->rrdinstances);
    rc->rrdinstances = NULL;
}

static inline bool rrdinstance_should_be_deleted(RRDINSTANCE *ri) {
    if(likely(!(ri->flags & RRD_FLAG_DELETED)))
        return false;

    if(likely(!(ri->flags & RRD_FLAG_LIVE_RETENTION)))
        return false;

    if(unlikely(ri->flags & RRD_FLAGS_PREVENTING_DELETIONS))
        return false;

    if(likely(ri->rrdset))
        return false;

    if(unlikely(dictionary_stats_referenced_items(ri->rrdmetrics) != 0))
        return false;

    if(unlikely(dictionary_stats_entries(ri->rrdmetrics) != 0))
        return false;

    if(ri->first_time_t || ri->last_time_t)
        return false;

    return true;
}

static void rrdinstance_trigger_updates(RRDINSTANCE *ri, bool force, bool escalate) {
    if(unlikely(!force && !(ri->flags & RRD_FLAG_UPDATED))) return;
    rrd_flag_unset_updated(ri);

    RRD_FLAGS combined_metrics_flags = RRD_FLAG_NONE;
    time_t min_first_time_t = LONG_MAX, max_last_time_t = 0;
    size_t metrics_active = 0, metrics_deleted = 0;
    {
        RRDMETRIC *rm;
        dfe_start_read((DICTIONARY *)ri->rrdmetrics, rm) {
            // find the combined flags of all the metrics
            combined_metrics_flags |= rm->flags & RRD_FLAGS_PROPAGATED_UPSTREAM;

            if (unlikely((rrdmetric_should_be_deleted(rm)))) {
                metrics_deleted++;
                continue;
            }

            metrics_active++;

            if (rm->first_time_t && rm->first_time_t < min_first_time_t)
                min_first_time_t = rm->first_time_t;

            if (rm->last_time_t && rm->last_time_t > max_last_time_t)
                max_last_time_t = rm->last_time_t;
        }
        dfe_done(rm);
    }

    // remove the deleted flag - we will recalculate it below
    ri->flags &= ~RRD_FLAG_DELETED;

    if(unlikely(!metrics_active && metrics_deleted)) {
        // we had some metrics, but there are gone now...

        ri->flags |= RRD_FLAG_DELETED;
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);
    }
    else if(metrics_active) {
        // we have active metrics...

        if (unlikely(min_first_time_t == LONG_MAX))
            min_first_time_t = 0;

        if (unlikely(min_first_time_t == 0 || max_last_time_t == 0)) {
            ri->first_time_t = 0;
            ri->last_time_t = 0;

            if(unlikely(combined_metrics_flags & RRD_FLAG_LIVE_RETENTION)) {
                ri->flags |= RRD_FLAG_DELETED;
                rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);
            }
        }
        else {
            if (unlikely(ri->first_time_t != min_first_time_t)) {
                ri->first_time_t = min_first_time_t;
                rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T);
            }

            if (unlikely(ri->last_time_t != max_last_time_t)) {
                ri->last_time_t = max_last_time_t;
                rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_LAST_TIME_T);
            }
        }

        if(combined_metrics_flags & RRD_FLAG_LIVE_RETENTION)
            ri->flags |= RRD_FLAG_LIVE_RETENTION;
        else
            ri->flags &= ~RRD_FLAG_LIVE_RETENTION;

        if(likely(combined_metrics_flags & RRD_FLAG_COLLECTED))
            rrd_flag_set_collected(ri);
        else
            rrd_flag_set_archived(ri);
    }
    else {
        // no deleted metrics, no active metrics
        // just hanging there...

        rrd_flag_set_archived(ri);
    }

    if(unlikely(escalate && ri->flags & RRD_FLAG_UPDATED)) {
        log_transition(NULL, ri->id, ri->rc->id, ri->flags, "RRDINSTANCE");
        rrdcontext_trigger_updates(ri->rc, true, RRD_FLAG_NONE);
        rrd_flag_unset_updated(ri);
    }
}

static inline void rrdinstance_from_rrdset(RRDSET *st) {
    RRDCONTEXT tc = {
        .id = string_strdupz(st->context),
        .title = string_strdupz(st->title),
        .units = string_strdupz(st->units),
        .family = string_strdupz(st->family),
        .priority = st->priority,
        .chart_type = st->chart_type,
        .flags = RRD_FLAG_NONE,
        .rrdhost = st->rrdhost,
    };

    RRDCONTEXT_ACQUIRED *rca = (RRDCONTEXT_ACQUIRED *)dictionary_set_and_acquire_item((DICTIONARY *)st->rrdhost->rrdctx, string2str(tc.id), &tc, sizeof(tc));
    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);

    RRDINSTANCE tri = {
        .id = string_strdupz(st->id),
        .name = string_strdupz(st->name),
        .units = string_strdupz(st->units),
        .family = string_strdupz(st->family),
        .title = string_strdupz(st->title),
        .chart_type = st->chart_type,
        .priority = st->priority,
        .update_every = st->update_every,
        .flags = RRD_FLAG_NONE,
        .rrdset = st,
    };
    uuid_copy(tri.uuid, *st->chart_uuid);

    RRDINSTANCE_ACQUIRED *ria = (RRDINSTANCE_ACQUIRED *)dictionary_set_and_acquire_item(rc->rrdinstances, string2str(tri.id), &tri, sizeof(tri));

    if(st->rrdinstance && st->rrdinstance != ria)
        fatal("RRDINSTANCE: chart '%s' changed rrdinstance.", st->id);

    st->rrdinstance = ria;

    if(st->rrdcontext && st->rrdcontext != rca) {
        // the chart changed context
        RRDCONTEXT *rc_old = rrdcontext_acquired_value(st->rrdcontext);
        dictionary_del(rc_old->rrdinstances, st->id);
        rrdcontext_trigger_updates(rc_old, true, RRD_FLAG_UPDATE_REASON_CHANGED_LINKING);
        rrdcontext_release(st->rrdcontext);
    }

    st->rrdcontext = rca;
}

#define rrdset_get_rrdinstance(st) rrdset_get_rrdinstance_with_trace(st, __FUNCTION__);
static inline RRDINSTANCE *rrdset_get_rrdinstance_with_trace(RRDSET *st, const char *function) {
    if(unlikely(!st->rrdinstance))
        fatal("RRDINSTANCE: RRDSET '%s' is not linked to an RRDINSTANCE at %s()", st->id, function);

    RRDINSTANCE *ri = rrdinstance_acquired_value(st->rrdinstance);

    if(unlikely(ri->rrdset != st))
        fatal("RRDINSTANCE: '%s' is not linked to RRDSET '%s' at %s()", string2str(ri->id), st->id, function);

    return ri;
}

static inline void rrdinstance_rrdset_is_freed(RRDSET *st) {
    RRDINSTANCE *ri = rrdset_get_rrdinstance(st);

    rrd_flag_set_archived(ri);

    if(!(ri->flags & RRD_FLAG_OWN_LABELS)) {
        ri->flags |= RRD_FLAG_OWN_LABELS;
        ri->rrdlabels = rrdlabels_create();
        rrdlabels_copy(ri->rrdlabels, st->state->chart_labels);
    }

    ri->rrdset = NULL;
    rrdinstance_trigger_updates(ri, false, true);
    rrdinstance_release(st->rrdinstance);
    st->rrdinstance = NULL;

    rrdcontext_release(st->rrdcontext);
    st->rrdcontext = NULL;
}

static inline void rrdinstance_updated_rrdset_name(RRDSET *st) {
    // the chart may not be initialized when this is called
    if(unlikely(!st->rrdinstance)) return;

    RRDINSTANCE *ri = rrdset_get_rrdinstance(st);

    STRING *old = ri->name;
    ri->name = string_strdupz(st->name);

    if(ri->name != old)
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_NAME);

    string_freez(old);

    rrdinstance_trigger_updates(ri, false, true);
}

static inline void rrdinstance_updated_rrdset_flags(RRDSET *st) {
    RRDINSTANCE *ri = rrdset_get_rrdinstance(st);

    if(unlikely(st->flags & (RRDSET_FLAG_ARCHIVED | RRDSET_FLAG_OBSOLETE)))
        rrd_flag_set_archived(ri);

    rrdinstance_trigger_updates(ri, false, true);
}

static inline void rrdinstance_collected_rrdset(RRDSET *st) {
    RRDINSTANCE *ri = rrdset_get_rrdinstance(st);

    if(unlikely(!rrd_flag_is_collected(ri)))
        rrd_flag_set_collected(ri);

    rrdinstance_trigger_updates(ri, false, true);
}

// ----------------------------------------------------------------------------
// RRDCONTEXT

static void rrdcontext_freez(RRDCONTEXT *rc) {
    string_freez(rc->id);
    string_freez(rc->title);
    string_freez(rc->units);
    string_freez(rc->family);
}

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
    rc->hub.deleted = (rc->flags & RRD_FLAG_DELETED) ? true : false;

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

#ifdef ENABLE_ACLK
    if(snapshot)
        contexts_snapshot_add_ctx_update(bundle, &message);
    else
        contexts_updated_add_ctx_update(bundle, &message);
#endif

    // store it to SQL

    if(rc->flags & RRD_FLAG_DELETED) {
        rrdcontext_delete_from_sql_unsafe(rc);
    }
    else {
        if (ctx_store_context(&rc->rrdhost->host_uuid, &rc->hub) != 0)
            error("RRDCONTEXT: failed to save context '%s' version %lu to SQL.", rc->hub.id, rc->hub.version);
    }
}

static bool check_if_cloud_version_changed_unsafe(RRDCONTEXT *rc, bool sending) {
    bool id_changed = false,
         title_changed = false,
         units_changed = false,
         family_changed = false,
         chart_type_changed = false,
         priority_changed = false,
         first_time_changed = false,
         last_time_changed = false,
         deleted_changed = false;

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

    if(unlikely((uint64_t)(rrd_flag_is_collected(rc) ? 0 : rc->last_time_t) != rc->hub.last_time_t))
        last_time_changed = true;

    if(unlikely(((rc->flags & RRD_FLAG_DELETED) ? true : false) != rc->hub.deleted))
        deleted_changed = true;

    if(unlikely(id_changed || title_changed || units_changed || family_changed || chart_type_changed || priority_changed || first_time_changed || last_time_changed || deleted_changed)) {

        internal_error(true, "RRDCONTEXT: %s NEW VERSION '%s'%s, version %zu, title '%s'%s, units '%s'%s, family '%s'%s, chart type '%s'%s, priority %u%s, first_time_t %ld%s, last_time_t %ld%s, deleted '%s'%s, (queued for %llu ms, expected %llu ms)",
                       sending?"SENDING":"QUEUE",
                       string2str(rc->id), id_changed ? " (CHANGED)" : "",
                       rc->version,
                       string2str(rc->title), title_changed ? " (CHANGED)" : "",
                       string2str(rc->units), units_changed ? " (CHANGED)" : "",
                       string2str(rc->family), family_changed ? " (CHANGED)" : "",
                       rrdset_type_name(rc->chart_type), chart_type_changed ? " (CHANGED)" : "",
                       rc->priority, priority_changed ? " (CHANGED)" : "",
                       rc->first_time_t, first_time_changed ? " (CHANGED)" : "",
                       rrd_flag_is_collected(rc) ? 0 : rc->last_time_t, last_time_changed ? " (CHANGED)" : "",
                       (rc->flags & RRD_FLAG_DELETED) ? "true" : "false", deleted_changed ? " (CHANGED)" : "",
                       sending ? (now_realtime_usec() - rc->queue.queued_ut) / USEC_PER_MS : 0,
                       sending ? (rc->queue.scheduled_dispatch_ut - rc->queue.queued_ut) / USEC_PER_SEC : 0
                       );
        return true;
    }

    return false;
}

static void rrdcontext_insert_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDHOST *host = (RRDHOST *)data;
    RRDCONTEXT *rc = (RRDCONTEXT *)value;

    rc->rrdhost = host;

    if(rc->hub.version) {
        // we are loading data from the SQL database

        if(rc->version)
            error("RRDCONTEXT: context '%s' is already initialized with version %lu, but it is loaded again from SQL with version %lu", string2str(rc->id), rc->version, rc->hub.version);

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

        if(rc->hub.deleted)
            rc->flags |= RRD_FLAG_DELETED;
        else {
            if (rc->last_time_t == 0)
                rrd_flag_set_collected(rc);
            else
                rrd_flag_set_archived(rc);
        }
    }
    else {
        // we are adding this context now for the first time
        rc->version = now_realtime_sec();
        rc->flags = rc->flags & RRD_FLAGS_ALLOWED_EXTERNALLY_ON_NEW_OBJECTS;
    }

    rrdinstances_create(rc);
    netdata_mutex_init(&rc->mutex);

    // signal the react callback to do the job
    rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_NEW_OBJECT);
}

static void rrdcontext_delete_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDHOST *host = (RRDHOST *)data;
    (void)host;

    RRDCONTEXT *rc = (RRDCONTEXT *)value;

    rrdinstances_destroy(rc);
    netdata_mutex_destroy(&rc->mutex);
    rrdcontext_freez(rc);
}

static STRING *string_2way_merge(RRDCONTEXT *rc __maybe_unused, STRING *a, STRING *b) {
    static STRING *X = NULL;

    if(unlikely(!X)) {
        // initialization
        X = string_strdupz("[x]");
    }

    if(unlikely(a == b)) return string_dup(a);
    if(unlikely(a == X)) return string_dup(a);
    if(unlikely(b == X)) return string_dup(b);
    if(unlikely(!a)) return string_dup(X);
    if(unlikely(!b)) return string_dup(X);

    size_t alen = string_length(a);
    size_t blen = string_length(b);
    size_t length = alen + blen + string_length(X) + 1;
    char buf1[length + 1], buf2[length + 1], *dst1, *dst2;
    const char *s1, *s2;

    s1 = string2str(a);
    s2 = string2str(b);
    dst1 = buf1;
    for( ; *s1 && *s2 && *s1 == *s2 ;s1++, s2++)
        *dst1++ = *s1;

    *dst1 = '\0';

    if(*s1 != '\0' || *s2 != '\0') {
        *dst1++ = '[';
        *dst1++ = 'x';
        *dst1++ = ']';

        s1 = &(string2str(a))[alen - 1];
        s2 = &(string2str(b))[blen - 1];
        dst2 = &buf2[length];
        *dst2 = '\0';
        for (; *s1 && *s2 && *s1 == *s2; s1--, s2--)
            *(--dst2) = *s1;

        strcpy(dst1, dst2);
    }

    return string_strdupz(buf1);
}

static void rrdcontext_conflict_callback(const char *id, void *oldv, void *newv, void *data) {
    (void)id;
    RRDHOST *host = (RRDHOST *)data;
    (void)host;

    RRDCONTEXT *rc = (RRDCONTEXT *)oldv;
    RRDCONTEXT *rc_new = (RRDCONTEXT *)newv;

    rrdcontext_lock(rc);

    if(rc->title != rc_new->title) {
        STRING *old_title = rc->title;
        rc->title = string_2way_merge(rc, rc->title, rc_new->title);
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
        rc->family = string_2way_merge(rc, rc->family, rc_new->family);
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

    rc->flags |= (rc_new->flags & RRD_FLAGS_ALLOWED_EXTERNALLY_ON_NEW_OBJECTS);

    if(rrd_flag_is_collected(rc) && rrd_flag_is_archived(rc))
        rrd_flag_set_collected(rc);

    if(rc->flags & RRD_FLAG_UPDATED)
        rc->flags |= RRD_FLAG_UPDATE_REASON_UPDATED_OBJECT;

    rrdcontext_unlock(rc);

    // free the resources of the new one
    rrdcontext_freez(rc_new);

    // the react callback will continue from here
}

static void rrdcontext_react_callback(const char *id __maybe_unused, void *value, void *data __maybe_unused) {
    RRDCONTEXT *rc = (RRDCONTEXT *)value;

    rrdcontext_trigger_updates(rc, false, RRD_FLAG_NONE);
}

void rrdhost_create_rrdcontexts(RRDHOST *host) {
    if(unlikely(rrdcontext_enabled == CONFIG_BOOLEAN_NO))
        return;

    if(unlikely(!host)) return;
    if(likely(host->rrdctx)) return;

    host->rrdctx = (RRDCONTEXTS *)dictionary_create(DICTIONARY_FLAG_DONT_OVERWRITE_VALUE);
    dictionary_register_insert_callback((DICTIONARY *)host->rrdctx, rrdcontext_insert_callback, (void *)host);
    dictionary_register_delete_callback((DICTIONARY *)host->rrdctx, rrdcontext_delete_callback, (void *)host);
    dictionary_register_conflict_callback((DICTIONARY *)host->rrdctx, rrdcontext_conflict_callback, (void *)host);
    dictionary_register_react_callback((DICTIONARY *)host->rrdctx, rrdcontext_react_callback, (void *)host);

    host->rrdctx_queue = (RRDCONTEXTS *)dictionary_create(DICTIONARY_FLAG_DONT_OVERWRITE_VALUE | DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE);
}

void rrdhost_destroy_rrdcontexts(RRDHOST *host) {
    if(unlikely(!host)) return;
    if(unlikely(!host->rrdctx)) return;

    if(host->rrdctx_queue) {
        dictionary_destroy((DICTIONARY *)host->rrdctx_queue);
        host->rrdctx_queue = NULL;
    }

    dictionary_destroy((DICTIONARY *)host->rrdctx);
    host->rrdctx = NULL;
}

static inline bool rrdcontext_should_be_deleted(RRDCONTEXT *rc) {
    if(likely(!(rc->flags & RRD_FLAG_DELETED)))
        return false;

    if(likely(!(rc->flags & RRD_FLAG_LIVE_RETENTION)))
        return false;

    if(unlikely(rc->flags & RRD_FLAGS_PREVENTING_DELETIONS))
        return false;

    if(unlikely(dictionary_stats_referenced_items(rc->rrdinstances) != 0))
        return false;

    if(unlikely(dictionary_stats_entries(rc->rrdinstances) != 0))
        return false;

    if(unlikely(rc->first_time_t || rc->last_time_t))
        return false;

    return true;
}

static void rrdcontext_trigger_updates(RRDCONTEXT *rc, bool force, RRD_FLAGS reason) {
    if(unlikely(!force && !(rc->flags & RRD_FLAG_UPDATED))) return;

    rrdcontext_lock(rc);
    rc->flags |= reason;
    rrd_flag_unset_updated(rc);

    size_t min_priority = LONG_MAX;
    RRD_FLAGS combined_instances_flags = RRD_FLAG_NONE;
    time_t min_first_time_t = LONG_MAX, max_last_time_t = 0;
    size_t instances_active = 0, instances_deleted = 0;
    {
        RRDINSTANCE *ri;
        dfe_start_read(rc->rrdinstances, ri) {
            // find the combined flags of all the metrics
            combined_instances_flags |= ri->flags & RRD_FLAGS_PROPAGATED_UPSTREAM;

            if (unlikely(rrdinstance_should_be_deleted(ri))) {
                instances_deleted++;
                continue;
            }

            instances_active++;

            if (ri->priority > 0 && ri->priority < min_priority)
                min_priority = ri->priority;

            if (ri->first_time_t && ri->first_time_t < min_first_time_t)
                min_first_time_t = ri->first_time_t;

            if (ri->last_time_t && ri->last_time_t > max_last_time_t)
                max_last_time_t = ri->last_time_t;
        }
        dfe_done(ri);
    }

    rc->flags &= ~RRD_FLAG_DELETED;

    if(unlikely(!instances_active && instances_deleted)) {
        // we had some instances, but they are gone now...

        rc->flags |= RRD_FLAG_DELETED;
        rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);
    }
    else if(instances_active) {
        // we have some active instances...

        if (unlikely(min_first_time_t == LONG_MAX))
            min_first_time_t = 0;

        if (unlikely(min_first_time_t == 0 && max_last_time_t == 0)) {
            rc->first_time_t = 0;
            rc->last_time_t = 0;

            if(unlikely(combined_instances_flags & RRD_FLAG_LIVE_RETENTION)) {
                rc->flags |= RRD_FLAG_DELETED;
                rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);
            }
        }
        else {
            if (unlikely(rc->first_time_t != min_first_time_t)) {
                rc->first_time_t = min_first_time_t;
                rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T);
            }

            if (rc->last_time_t != max_last_time_t) {
                rc->last_time_t = max_last_time_t;
                rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_LAST_TIME_T);
            }
        }

        if(combined_instances_flags & RRD_FLAG_LIVE_RETENTION)
            rc->flags |= RRD_FLAG_LIVE_RETENTION;
        else
            rc->flags &= ~RRD_FLAG_LIVE_RETENTION;

        if(likely(combined_instances_flags & RRD_FLAG_COLLECTED))
            rrd_flag_set_collected(rc);
        else
            rrd_flag_set_archived(rc);

        if (min_priority != LONG_MAX && rc->priority != min_priority) {
            rc->priority = min_priority;
            rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_PRIORITY);
        }
    }
    else {
        // no deleted instances, no active instances
        // just hanging there...

        rrd_flag_set_archived(rc);
    }

    if(unlikely(rc->flags & RRD_FLAG_UPDATED)) {
        log_transition(NULL, NULL, rc->id, rc->flags, "RRDCONTEXT");

        if(check_if_cloud_version_changed_unsafe(rc, false)) {
            rc->version = rrdcontext_get_next_version(rc);
            rc->queue.queued_ut = now_realtime_usec();
            rc->queue.queued_flags |= rc->flags;
            if(!(rc->flags & RRD_FLAG_QUEUED)) {
                rc->flags |= RRD_FLAG_QUEUED;
                dictionary_set((DICTIONARY *)rc->rrdhost->rrdctx_queue, string2str(rc->id), rc, sizeof(*rc));
            }
        }

        rrd_flag_unset_updated(rc);
    }

    rrdcontext_unlock(rc);
}

// ----------------------------------------------------------------------------
// public API

void rrdcontext_updated_rrddim(RRDDIM *rd) {
    if(unlikely(rrdcontext_enabled == CONFIG_BOOLEAN_NO))
        return;

    rrdmetric_from_rrddim(rd);
}

void rrdcontext_removed_rrddim(RRDDIM *rd) {
    if(unlikely(rrdcontext_enabled == CONFIG_BOOLEAN_NO))
        return;

    rrdmetric_rrddim_is_freed(rd);
}

void rrdcontext_updated_rrddim_algorithm(RRDDIM *rd) {
    if(unlikely(rrdcontext_enabled == CONFIG_BOOLEAN_NO))
        return;

    rrdmetric_updated_rrddim_flags(rd);
}

void rrdcontext_updated_rrddim_multiplier(RRDDIM *rd) {
    if(unlikely(rrdcontext_enabled == CONFIG_BOOLEAN_NO))
        return;

    rrdmetric_updated_rrddim_flags(rd);
}

void rrdcontext_updated_rrddim_divisor(RRDDIM *rd) {
    if(unlikely(rrdcontext_enabled == CONFIG_BOOLEAN_NO))
        return;

    rrdmetric_updated_rrddim_flags(rd);
}

void rrdcontext_updated_rrddim_flags(RRDDIM *rd) {
    if(unlikely(rrdcontext_enabled == CONFIG_BOOLEAN_NO))
        return;

    rrdmetric_updated_rrddim_flags(rd);
}

void rrdcontext_collected_rrddim(RRDDIM *rd) {
    if(unlikely(rrdcontext_enabled == CONFIG_BOOLEAN_NO))
        return;

    rrdmetric_collected_rrddim(rd);
}

void rrdcontext_updated_rrdset(RRDSET *st) {
    if(unlikely(rrdcontext_enabled == CONFIG_BOOLEAN_NO))
        return;

    rrdinstance_from_rrdset(st);
}

void rrdcontext_removed_rrdset(RRDSET *st) {
    if(unlikely(rrdcontext_enabled == CONFIG_BOOLEAN_NO))
        return;

    rrdinstance_rrdset_is_freed(st);
}

void rrdcontext_updated_rrdset_name(RRDSET *st) {
    if(unlikely(rrdcontext_enabled == CONFIG_BOOLEAN_NO))
        return;

    rrdinstance_updated_rrdset_name(st);
}

void rrdcontext_updated_rrdset_flags(RRDSET *st) {
    if(unlikely(rrdcontext_enabled == CONFIG_BOOLEAN_NO))
        return;

    rrdinstance_updated_rrdset_flags(st);
}

void rrdcontext_collected_rrdset(RRDSET *st) {
    if(unlikely(rrdcontext_enabled == CONFIG_BOOLEAN_NO))
        return;

    rrdinstance_collected_rrdset(st);
}

void rrdcontext_host_child_connected(RRDHOST *host) {
    (void)host;

    if(unlikely(rrdcontext_enabled == CONFIG_BOOLEAN_NO))
        return;

    // no need to do anything here
    ;
}

void rrdcontext_host_child_disconnected(RRDHOST *host) {
    if(unlikely(rrdcontext_enabled == CONFIG_BOOLEAN_NO))
        return;

    rrdcontext_recalculate_host_retention(host, RRD_FLAG_UPDATE_REASON_DISCONNECTED_CHILD, -1);
}

// ----------------------------------------------------------------------------
// ACLK interface

static bool rrdhost_check_our_claim_id(const char *claim_id) {
    if(!localhost->aclk_state.claimed_id) return false;
    return (strcasecmp(claim_id, localhost->aclk_state.claimed_id) == 0) ? true : false;
}

static RRDHOST *rrdhost_find_by_node_id(const char *node_id) {
    uuid_t uuid;
    uuid_parse(node_id, uuid);

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
              cmd->claim_id, cmd->node_id, host->hostname);

        // disable it temporarily, so that our worker will not attempt to send messages in parallel
        rrdhost_flag_clear(host, RRDHOST_FLAG_ACLK_STREAM_CONTEXTS);
    }

    uint64_t our_version_hash = rrdcontext_version_hash(host);

    if(cmd->version_hash != our_version_hash) {
        error("RRDCONTEXT: received version hash %lu for host '%s', does not match our version hash %lu. Sending snapshot of all contexts.",
              cmd->version_hash, host->hostname, our_version_hash);

#ifdef ENABLE_ACLK
        // prepare the snapshot
        char uuid[UUID_STR_LEN];
        uuid_unparse_lower(*host->node_id, uuid);
        contexts_snapshot_t bundle = contexts_snapshot_new(cmd->claim_id, uuid, our_version_hash);

        // do a deep scan on every metric of the host to make sure all our data are updated
        rrdcontext_recalculate_host_retention(host, RRD_FLAG_NONE, -1);

        // calculate version hash and pack all the messages together in one go
        our_version_hash = rrdcontext_version_hash_with_callback(host, rrdcontext_message_send_unsafe, true, bundle);

        // update the version
        contexts_snapshot_set_version(bundle, our_version_hash);

        // send it
        aclk_send_contexts_snapshot(bundle);
#endif
    }

    internal_error(true, "RRDCONTEXT: host '%s' enabling streaming of contexts", host->hostname);
    rrdhost_flag_set(host, RRDHOST_FLAG_ACLK_STREAM_CONTEXTS);
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
              cmd->claim_id, cmd->node_id, host->hostname);

        return;
    }

    internal_error(true, "RRDCONTEXT: host '%s' disabling streaming of contexts", host->hostname);
    rrdhost_flag_clear(host, RRDHOST_FLAG_ACLK_STREAM_CONTEXTS);
}

// ----------------------------------------------------------------------------
// web API

struct rrdcontext_to_json {
    BUFFER *wb;
    RRDCONTEXT_TO_JSON_OPTIONS options;
    time_t after;
    time_t before;
    size_t written;
    time_t now;
};

static inline int rrdmetric_to_json_callback(const char *id, void *value, void *data) {
    struct rrdcontext_to_json * t = data;
    RRDMETRIC *rm = value;
    BUFFER *wb = t->wb;
    RRDCONTEXT_TO_JSON_OPTIONS options = t->options;
    time_t after = t->after;
    time_t before = t->before;

    if((rm->flags & RRD_FLAG_DELETED) && !(options & RRDCONTEXT_OPTION_SHOW_DELETED))
        return 0;

    if(after && (!rm->last_time_t || after > rm->last_time_t))
        return 0;

    if(before && (!rm->first_time_t || before < rm->first_time_t))
        return 0;

    if(t->written)
        buffer_strcat(wb, ",\n");
    else
        buffer_strcat(wb, "\n");

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
                   , rm->flags & RRD_FLAG_COLLECTED ? "true" : "false"
                   );

    if(options & RRDCONTEXT_OPTION_SHOW_DELETED) {
        buffer_sprintf(wb,
                       ",\n\t\t\t\t\t\t\t\"deleted\":%s"
                       , rm->flags & RRD_FLAG_DELETED ? "true" : "false"
        );
    }

    if(options & RRDCONTEXT_OPTION_SHOW_FLAGS) {
        buffer_strcat(wb, ",\n\t\t\t\t\t\t\t\"flags\":\"");
        rrd_flags_to_buffer(rm->flags, wb);
        buffer_strcat(wb, "\"");
    }

    buffer_strcat(wb, "\n\t\t\t\t\t\t}");
    t->written++;
    return 1;
}

static inline int rrdinstance_to_json_callback(const char *id, void *value, void *data) {
    struct rrdcontext_to_json * t = data;
    RRDINSTANCE *ri = value;
    BUFFER *wb = t->wb;
    RRDCONTEXT_TO_JSON_OPTIONS options = t->options;
    time_t after = t->after;
    time_t before = t->before;

    if((ri->flags & RRD_FLAG_DELETED) && !(options & RRDCONTEXT_OPTION_SHOW_DELETED))
        return 0;

    if(after && (!ri->last_time_t || after > ri->last_time_t))
        return 0;

    if(before && (!ri->first_time_t || before < ri->first_time_t))
        return 0;

    if(t->written)
        buffer_strcat(wb, ",\n");
    else
        buffer_strcat(wb, "\n");

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
                   , ri->first_time_t
                   , rrd_flag_is_collected(ri) ? t->now : ri->last_time_t
                   , ri->flags & RRD_FLAG_COLLECTED ? "true" : "false"
    );

    if(options & RRDCONTEXT_OPTION_SHOW_DELETED) {
        buffer_sprintf(wb,
                       ",\n\t\t\t\t\t\"deleted\":%s"
                       , ri->flags & RRD_FLAG_DELETED ? "true" : "false"
        );
    }

    if(options & RRDCONTEXT_OPTION_SHOW_FLAGS) {
        buffer_strcat(wb, ",\n\t\t\t\t\t\"flags\":\"");
        rrd_flags_to_buffer(ri->flags, wb);
        buffer_strcat(wb, "\"");
    }

    if(options & RRDCONTEXT_OPTION_SHOW_LABELS && ri->rrdlabels && dictionary_stats_entries(ri->rrdlabels)) {
        buffer_sprintf(wb, ",\n\t\t\t\t\t\"labels\": {\n");
        rrdlabels_to_buffer(ri->rrdlabels, wb, "\t\t\t\t\t\t", ":", "\"", ",\n", NULL, NULL, NULL, NULL);
        buffer_strcat(wb, "\n\t\t\t\t\t}");
    }

    if(options & RRDCONTEXT_OPTION_SHOW_METRICS) {
        buffer_sprintf(wb, ",\n\t\t\t\t\t\"dimensions\": {");
        struct rrdcontext_to_json tt = {
            .wb = wb,
            .options = options,
            .after = after,
            .before = before,
            .written = 0,
            .now = t->now,
        };
        dictionary_walkthrough_read(ri->rrdmetrics, rrdmetric_to_json_callback, &tt);
        buffer_strcat(wb, "\n\t\t\t\t\t}");
    }

    buffer_strcat(wb, "\n\t\t\t\t}");
    t->written++;
    return 1;
}

static inline int rrdcontext_to_json_callback(const char *id, void *value, void *data) {
    struct rrdcontext_to_json * t = data;
    RRDCONTEXT *rc = value;
    BUFFER *wb = t->wb;
    RRDCONTEXT_TO_JSON_OPTIONS options = t->options;
    time_t after = t->after;
    time_t before = t->before;

    if((rc->flags & RRD_FLAG_DELETED) && !(options & RRDCONTEXT_OPTION_SHOW_DELETED))
        return 0;

    if(after && (!rc->last_time_t || after > rc->last_time_t))
        return 0;

    if(before && (!rc->first_time_t || before < rc->first_time_t))
        return 0;

    if(t->written)
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
                   , rc->first_time_t
                   , rrd_flag_is_collected(rc) ? t->now : rc->last_time_t
                   , rc->flags & RRD_FLAG_COLLECTED ? "true" : "false"
                   );

    if(options & RRDCONTEXT_OPTION_SHOW_DELETED) {
        buffer_sprintf(wb,
                       ",\n\t\t\t\"deleted\":%s"
                       , rc->flags & RRD_FLAG_DELETED ? "true" : "false"
        );
    }

    if(options & RRDCONTEXT_OPTION_SHOW_FLAGS) {
        buffer_strcat(wb, ",\n\t\t\t\"flags\":\"");
        rrd_flags_to_buffer(rc->flags, wb);
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
                       ",\n\t\t\t\"hub_version\":%lu"
                       ",\n\t\t\t\"version\":%lu"
                       , rc->queue.queued_ut / USEC_PER_SEC
                       , rc->queue.scheduled_dispatch_ut / USEC_PER_SEC
                       , rc->queue.dequeued_ut / USEC_PER_SEC
                       , rc->hub.version
                       , rc->version
                       );
    }

    rrdcontext_unlock(rc);

    if(options & (RRDCONTEXT_OPTION_SHOW_INSTANCES|RRDCONTEXT_OPTION_SHOW_METRICS)) {
        buffer_sprintf(wb, ",\n\t\t\t\"charts\": {");
        struct rrdcontext_to_json tt = {
            .wb = wb,
            .options = options,
            .after = after,
            .before = before,
            .written = 0,
            .now = t->now,
        };
        dictionary_walkthrough_read(rc->rrdinstances, rrdinstance_to_json_callback, &tt);
        buffer_strcat(wb, "\n\t\t\t}");
    }

    buffer_strcat(wb, "\n\t\t}");
    t->written++;
    return 1;
}

int rrdcontext_to_json(RRDHOST *host, BUFFER *wb, time_t after, time_t before, RRDCONTEXT_TO_JSON_OPTIONS options, const char *context) {
    RRDCONTEXT_ACQUIRED *rca = (RRDCONTEXT_ACQUIRED *)dictionary_get_and_acquire_item((DICTIONARY *)host->rrdctx, context);
    if(!rca) return HTTP_RESP_NOT_FOUND;

    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);

    if(options & RRDCONTEXT_OPTION_DEEPSCAN)
        rrdcontext_recalculate_context_retention(rc, RRD_FLAG_NONE, -1);

    if(after != 0 && before != 0) {
        long long after_wanted = after;
        long long before_wanted = before;
        rrdr_relative_window_to_absolute(&after_wanted, &before_wanted);
        after = after_wanted;
        before = before_wanted;
    }

    struct rrdcontext_to_json t = {
        .wb = wb,
        .options = options|RRDCONTEXT_OPTION_SKIP_ID,
        .after = after,
        .before = before,
        .written = 0,
        .now = now_realtime_sec(),
    };
    rrdcontext_to_json_callback(context, rc, &t);

    rrdcontext_release(rca);

    if(!t.written)
        return HTTP_RESP_NOT_FOUND;

    return HTTP_RESP_OK;
}

int rrdcontexts_to_json(RRDHOST *host, BUFFER *wb, time_t after, time_t before, RRDCONTEXT_TO_JSON_OPTIONS options) {
    char node_uuid[UUID_STR_LEN] = "";

    if(host->node_id)
        uuid_unparse(*host->node_id, node_uuid);

    if(options & RRDCONTEXT_OPTION_DEEPSCAN)
        rrdcontext_recalculate_host_retention(host, RRD_FLAG_NONE, -1);

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
                   , host->hostname
                   , host->machine_guid
                   , node_uuid
                   , host->aclk_state.claimed_id ? host->aclk_state.claimed_id : ""
                   );

    if(options & RRDCONTEXT_OPTION_SHOW_LABELS) {
        buffer_sprintf(wb, ",\n\t\"host_labels\": {\n");
        rrdlabels_to_buffer(host->host_labels, wb, "\t\t", ":", "\"", ",\n", NULL, NULL, NULL, NULL);
        buffer_strcat(wb, "\n\t}");
    }

    buffer_sprintf(wb, ",\n\t\"contexts\": {");
    struct rrdcontext_to_json t = {
        .wb = wb,
        .options = options,
        .after = after,
        .before = before,
        .written = 0,
        .now = now_realtime_sec(),
    };
    dictionary_walkthrough_read((DICTIONARY *)host->rrdctx, rrdcontext_to_json_callback, &t);

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
        .flags = RRD_FLAG_ARCHIVED | RRD_FLAG_UPDATE_REASON_LOAD_SQL,
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
        .flags = RRD_FLAG_ARCHIVED | RRD_FLAG_UPDATE_REASON_LOAD_SQL,
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
        .flags = RRD_FLAG_ARCHIVED | RRD_FLAG_UPDATE_REASON_LOAD_SQL,
    };
    uuid_copy(tri.uuid, sc->chart_id);

    RRDINSTANCE_ACQUIRED *ria = (RRDINSTANCE_ACQUIRED *)dictionary_set_and_acquire_item(rc->rrdinstances, sc->id, &tri, sizeof(tri));
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);

    ctx_get_dimension_list(&ri->uuid, rrdinstance_load_dimension, ri);
    ctx_get_label_list(&ri->uuid, rrdinstance_load_clabel, ri);

    rrdinstance_release(ria);
    rrdcontext_release(rca);
}

static void rrdcontext_load_context_callback(VERSIONED_CONTEXT_DATA *ctx_data, void *data) {
    RRDHOST *host = data;
    (void)host;

    RRDCONTEXT tmp = {
        .id = string_strdupz(ctx_data->id),

        // no need to set more data here
        // we only need the hub data

        .hub = *ctx_data,
    };
    dictionary_set((DICTIONARY *)host->rrdctx, string2str(tmp.id), &tmp, sizeof(tmp));
}

void rrdhost_load_rrdcontext_data(RRDHOST *host) {
    if(unlikely(rrdcontext_enabled == CONFIG_BOOLEAN_NO))
        return;

    if(host->rrdctx) return;

    rrdhost_create_rrdcontexts(host);
    ctx_get_context_list(&host->host_uuid, rrdcontext_load_context_callback, host);
    ctx_get_chart_list(&host->host_uuid, rrdinstance_load_chart_callback, host);
}

// ----------------------------------------------------------------------------
// the worker thread

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

#define WORKER_JOB_HOSTS            1
#define WORKER_JOB_CHECK            2
#define WORKER_JOB_SEND             3
#define WORKER_JOB_DEQUEUE          4
#define WORKER_JOB_RETENTION        5
#define WORKER_JOB_QUEUED           6
#define WORKER_JOB_CLEANUP          7
#define WORKER_JOB_CLEANUP_DELETE   8

static usec_t rrdcontext_next_db_rotation_ut = 0;
void rrdcontext_db_rotation(void) {
    // called when the db rotates its database
    rrdcontext_next_db_rotation_ut = now_realtime_usec() + FULL_RETENTION_SCAN_DELAY_AFTER_DB_ROTATION_SECS * USEC_PER_SEC;
}

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

        // skip any deleted contexts
        if(unlikely(rc->flags & RRD_FLAG_DELETED)) {
            rrdcontext_unlock(rc);
            continue;
        }

        if(unlikely(callback))
            callback(rc, snapshot, bundle);

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

static void rrdcontext_recalculate_context_retention(RRDCONTEXT *rc, RRD_FLAGS reason, int job_id) {
    RRDINSTANCE *ri;
    dfe_start_read(rc->rrdinstances, ri) {
        RRDMETRIC *rm;
        dfe_start_read(ri->rrdmetrics, rm) {

            if(job_id >= 0)
                worker_is_busy(job_id);

            rrd_flag_set_updated(rm, reason);

            rrdmetric_trigger_updates(rm, true, false);
        }
        dfe_done(rm);

        rrdinstance_trigger_updates(ri, true, false);
    }
    dfe_done(ri);

    rrdcontext_trigger_updates(rc, true, RRD_FLAG_NONE);
}

static void rrdcontext_recalculate_host_retention(RRDHOST *host, RRD_FLAGS reason, int job_id) {
    if(unlikely(!host || !host->rrdctx)) return;

    RRDCONTEXT *rc;
    dfe_start_read((DICTIONARY *)host->rrdctx, rc) {
        rrdcontext_recalculate_context_retention(rc, reason, job_id);
    }
    dfe_done(rc);
}

static void rrdcontext_recalculate_retention(int job_id) {
    rrdcontext_next_db_rotation_ut = 0;
    rrd_rdlock();
    RRDHOST *host;
    rrdhost_foreach_read(host) {
        rrdcontext_recalculate_host_retention(host, RRD_FLAG_UPDATE_REASON_DB_ROTATION, job_id);
    }
    rrd_unlock();
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
        error("RRDCONTEXT: failed to delete context '%s' version %lu from SQL.", rc->hub.id, rc->hub.version);
}

static void rrdcontext_garbage_collect(void) {
    rrd_rdlock();
    RRDHOST *host;
    rrdhost_foreach_read(host) {
        RRDCONTEXT *rc;
        dfe_start_write((DICTIONARY *)host->rrdctx, rc) {
            worker_is_busy(WORKER_JOB_CLEANUP);

            rrdcontext_lock(rc);

            if(unlikely(rrdcontext_should_be_deleted(rc))) {
                worker_is_busy(WORKER_JOB_CLEANUP_DELETE);
                rrdcontext_delete_from_sql_unsafe(rc);

                if(dictionary_del_having_write_lock((DICTIONARY *)host->rrdctx, string2str(rc->id)) != 0)
                    error("RRDCONTEXT: '%s' of host '%s' failed to be deleted from rrdcontext dictionary.",
                          string2str(rc->id), host->hostname);
            }
            else {
                RRDINSTANCE *ri;
                dfe_start_write(rc->rrdinstances, ri) {
                    if(rrdinstance_should_be_deleted(ri)) {
                        worker_is_busy(WORKER_JOB_CLEANUP_DELETE);
                        dictionary_del_having_write_lock(rc->rrdinstances, string2str(ri->id));
                    }
                    else {
                        RRDMETRIC *rm;
                        dfe_start_write(ri->rrdmetrics, rm) {
                            if(rrdmetric_should_be_deleted(rm)) {
                                worker_is_busy(WORKER_JOB_CLEANUP_DELETE);
                                dictionary_del_having_write_lock(ri->rrdmetrics, string2str(rm->id));
                            }
                        }
                        dfe_done(rm);
                    }
                }
                dfe_done(ri);
            }

            // the item is referenced in the dictionary
            // so, it is still here to unlock, even if we have deleted it
            rrdcontext_unlock(rc);
        }
        dfe_done(rc);
    }
    rrd_unlock();
}

static void rrdcontext_main_cleanup(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;
    // custom code
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void *rrdcontext_main(void *ptr) {
    if(unlikely(rrdcontext_enabled == CONFIG_BOOLEAN_NO))
        return NULL;

    worker_register("RRDCONTEXT");
    worker_register_job_name(WORKER_JOB_HOSTS, "hosts");
    worker_register_job_name(WORKER_JOB_CHECK, "dedup checks");
    worker_register_job_name(WORKER_JOB_SEND, "sent contexts");
    worker_register_job_name(WORKER_JOB_DEQUEUE, "deduped contexts");
    worker_register_job_name(WORKER_JOB_RETENTION, "metrics retention");
    worker_register_job_name(WORKER_JOB_QUEUED, "queued contexts");
    worker_register_job_name(WORKER_JOB_CLEANUP, "cleanups");
    worker_register_job_name(WORKER_JOB_CLEANUP_DELETE, "deletes");

    netdata_thread_cleanup_push(rrdcontext_main_cleanup, ptr);
    heartbeat_t hb;
    heartbeat_init(&hb);
    usec_t step = USEC_PER_SEC * RRDCONTEXT_WORKER_THREAD_HEARTBEAT_SECS;

    while (!netdata_exit) {
        worker_is_idle();
        heartbeat_next(&hb, step);

        if(!aclk_connected) continue;

        usec_t now_ut = now_realtime_usec();

        if(rrdcontext_next_db_rotation_ut && now_ut > rrdcontext_next_db_rotation_ut) {
            rrdcontext_recalculate_retention(WORKER_JOB_RETENTION);
            rrdcontext_garbage_collect();
            rrdcontext_next_db_rotation_ut = 0;
        }

        rrd_rdlock();
        RRDHOST *host;
        rrdhost_foreach_read(host) {
            worker_is_busy(WORKER_JOB_HOSTS);

            // check if we have received a streaming command for this host
            if(!rrdhost_flag_check(host, RRDHOST_FLAG_ACLK_STREAM_CONTEXTS))
                continue;

            // check if there are queued items to send
            if(!dictionary_stats_entries((DICTIONARY *)host->rrdctx_queue))
                continue;

            if(!host->node_id)
                continue;

            size_t messages_added = 0;
            contexts_updated_t bundle = NULL;

            RRDCONTEXT *rc;
            dfe_start_write((DICTIONARY *)host->rrdctx_queue, rc) {

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
                    
                    // remove the queued flag, so that it can be queued again
                    rc->flags &= ~RRD_FLAG_QUEUED;

                    // remove it from the queue
                    worker_is_busy(WORKER_JOB_DEQUEUE);
                    dictionary_del_having_write_lock((DICTIONARY *)host->rrdctx_queue, string2str(rc->id));

                    if(unlikely(rrdcontext_should_be_deleted(rc))) {
                        // this is a deleted context - delete it forever...

                        worker_is_busy(WORKER_JOB_CLEANUP_DELETE);
                        rrdcontext_delete_from_sql_unsafe(rc);

                        STRING *id = string_dup(rc->id);
                        rrdcontext_unlock(rc);

                        // delete it from the master dictionary
                        if(dictionary_del((DICTIONARY *)host->rrdctx, string2str(rc->id)) != 0)
                            error("RRDCONTEXT: '%s' of host '%s' failed to be deleted from rrdcontext dictionary.",
                                  string2str(id), host->hostname);

                        string_freez(id);
                    }
                    else
                        rrdcontext_unlock(rc);
                }
                freez(claim_id);
            }
            dfe_done(rc);

#ifdef ENABLE_ACLK
            if(bundle) {
                // we have a bundle to send messages

                // update the version hash
                contexts_updated_update_version_hash(bundle, rrdcontext_version_hash(host));

                // send it
                aclk_send_contexts_updated(bundle);
            }
#endif
        }
        rrd_unlock();

    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}

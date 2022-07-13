// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdcontext.h"
#include "sqlite/sqlite_context.h"

int rrdcontext_enabled = CONFIG_BOOLEAN_NO;

#define LOG_TRANSITIONS 1
// #define LOG_RRDINSTANCES 1

typedef enum {
    RRD_FLAG_NONE           = 0,
    RRD_FLAG_DELETED        = (1 << 0), // this is a deleted object (metrics, instances, contexts)
    RRD_FLAG_COLLECTED      = (1 << 1), // this object is currently being collected
    RRD_FLAG_UPDATED        = (1 << 2), // this object has updates to propagate
    RRD_FLAG_ARCHIVED       = (1 << 3), // this object is not currently being collected
    RRD_FLAG_OWNLABELS      = (1 << 4), // this instance has its own labels - not linked to an RRDSET
    RRD_FLAG_LIVE_RETENTION = (1 << 5), // we have got live retention from the database
    RRD_FLAG_QUEUED         = (1 << 6), // this context is currently queued to be dispatched to hub

    RRD_FLAG_UPDATE_REASON_CHANGED_UPDATE_EVERY    = (1 << 14),
    RRD_FLAG_UPDATE_REASON_CHANGED_LINKING         = (1 << 15), // an instance or a metric switched RRDSET or RRDDIM
    RRD_FLAG_UPDATE_REASON_CHANGED_NAME            = (1 << 16), // an instance or a metric changed name
    RRD_FLAG_UPDATE_REASON_CHANGED_UUID            = (1 << 17), // an instance or a metric changed UUID
    RRD_FLAG_UPDATE_REASON_NEW_OBJECT              = (1 << 18), // this object has just been created
    RRD_FLAG_UPDATE_REASON_ZERO_RETENTION          = (1 << 19), // this object has not retention
    RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T    = (1 << 20),
    RRD_FLAG_UPDATE_REASON_CHANGED_LAST_TIME_T     = (1 << 21),
    RRD_FLAG_UPDATE_REASON_CHANGED_CHART_TYPE      = (1 << 22),
    RRD_FLAG_UPDATE_REASON_CHANGED_PRIORITY        = (1 << 23),
    RRD_FLAG_UPDATE_REASON_CHANGED_UNITS           = (1 << 24),
    RRD_FLAG_UPDATE_REASON_CHANGED_TITLE           = (1 << 25),
    RRD_FLAG_UPDATE_REASON_CONNECTED_CHILD         = (1 << 26),
    RRD_FLAG_UPDATE_REASON_DISCONNECTED_CHILD      = (1 << 27),
    RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED = (1 << 28),
    RRD_FLAG_UPDATE_REASON_STARTED_BEING_COLLECTED = (1 << 29),
    RRD_FLAG_UPDATE_REASON_DB_ROTATION             = (1 << 30),
    RRD_FLAG_UPDATE_REASON_LOAD_SQL                = (1 << 31),
} RRD_FLAGS;

#define RRD_FLAG_UPDATE_REASONS                       ( \
     RRD_FLAG_UPDATE_REASON_CHANGED_UPDATE_EVERY        \
    |RRD_FLAG_UPDATE_REASON_CHANGED_LINKING             \
    |RRD_FLAG_UPDATE_REASON_CHANGED_NAME                \
    |RRD_FLAG_UPDATE_REASON_CHANGED_UUID                \
    |RRD_FLAG_UPDATE_REASON_NEW_OBJECT                  \
    |RRD_FLAG_UPDATE_REASON_ZERO_RETENTION              \
    |RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T        \
    |RRD_FLAG_UPDATE_REASON_CHANGED_LAST_TIME_T         \
    |RRD_FLAG_UPDATE_REASON_CHANGED_CHART_TYPE          \
    |RRD_FLAG_UPDATE_REASON_CHANGED_PRIORITY            \
    |RRD_FLAG_UPDATE_REASON_CHANGED_UNITS               \
    |RRD_FLAG_UPDATE_REASON_CHANGED_TITLE               \
    |RRD_FLAG_UPDATE_REASON_CONNECTED_CHILD             \
    |RRD_FLAG_UPDATE_REASON_DISCONNECTED_CHILD          \
    |RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED     \
    |RRD_FLAG_UPDATE_REASON_STARTED_BEING_COLLECTED     \
    |RRD_FLAG_UPDATE_REASON_DB_ROTATION                 \
    |RRD_FLAG_UPDATE_REASON_LOAD_SQL                    \
)

#define RRD_FLAGS_PROPAGATED_UPSTREAM                 ( \
     RRD_FLAG_COLLECTED                                 \
    |RRD_FLAG_DELETED                                   \
    |RRD_FLAG_LIVE_RETENTION                            \
    |RRD_FLAG_UPDATE_REASONS                            \
    )

#define RRD_FLAGS_ALLOWED_EXTERNALLY_ON_NEW_OBJECTS   ( \
     RRD_FLAG_ARCHIVED                                  \
    |RRD_FLAG_UPDATE_REASONS                            \
)

#define rrd_flag_set_updated(obj, reason) (obj)->flags |= (RRD_FLAG_UPDATED | (reason))
#define rrd_flag_unset_updated(obj) (obj)->flags &= ~(RRD_FLAG_UPDATED|RRD_FLAG_UPDATE_REASONS)

#define rrd_flag_set_collected(obj) do { \
    (obj)->flags |= RRD_FLAG_COLLECTED;  \
    (obj)->flags &= ~RRD_FLAG_ARCHIVED;  \
} while(0)

#define rrd_flag_set_archived(obj) do { \
    (obj)->flags |= RRD_FLAG_ARCHIVED;  \
    (obj)->flags &= ~RRD_FLAG_COLLECTED;\
} while(0)

#define rrd_flag_is_collected(obj) ((obj)->flags & RRD_FLAG_COLLECTED)
#define rrd_flag_is_archived(obj) ((obj)->flags & RRD_FLAG_ARCHIVED)

static struct rrdcontext_reason {
    RRD_FLAGS flag;
    const char *name;
    usec_t delay_ut;
} rrdcontext_reasons[] = {
    // context related
    { RRD_FLAG_UPDATE_REASON_NEW_OBJECT,              "object created",       0 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_LOAD_SQL,                "loaded from sql",      60 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_CHANGED_TITLE,           "changed title",        30 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_CHANGED_UNITS,           "changed units",        30 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_CHANGED_PRIORITY,        "changed priority",     30 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_ZERO_RETENTION,          "has no retention",     0 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T,    "updated first_time_t", 0 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_CHANGED_LAST_TIME_T,     "updated last_time_t",  60 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_CHANGED_CHART_TYPE,      "changed chart type",   30 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED, "stopped collected",    60 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_STARTED_BEING_COLLECTED, "started collected",    0 * USEC_PER_SEC },

    // not context related
    { RRD_FLAG_UPDATE_REASON_CHANGED_UUID,            "changed uuid",         60 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_CHANGED_UPDATE_EVERY,    "changed updated every",60 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_CHANGED_LINKING,         "changed rrd link",     60 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_CHANGED_NAME,            "changed name",         60 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_CONNECTED_CHILD,         "child connected",      0 * USEC_PER_SEC },
    { RRD_FLAG_UPDATE_REASON_DISCONNECTED_CHILD,      "child disconnected",   60 * USEC_PER_SEC },
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
} RRDMETRIC;

typedef struct rrdinstance {
    uuid_t uuid;

    STRING *id;
    STRING *name;
    STRING *title;
    STRING *units;
    size_t priority;
    RRDSET_TYPE chart_type;

    int update_every;                   // data collection frequency
    RRDSET *rrdset;                     // pointer to RRDSET when collected, or NULL

    time_t first_time_t;
    time_t last_time_t;
    RRD_FLAGS flags;                    // flags related to this instance

    DICTIONARY *rrdlabels;              // linked to RRDSET->state->chart_labels or own version

    struct rrdcontext *rc;
    DICTIONARY *rrdmetrics;
} RRDINSTANCE;

typedef struct rrdcontext {
    uint64_t version;

    STRING *id;
    STRING *title;
    STRING *units;
    RRDSET_TYPE chart_type;

    size_t priority;

    time_t first_time_t;
    time_t last_time_t;
    RRD_FLAGS flags;

    VERSIONED_CONTEXT_DATA hub;

    DICTIONARY *rrdinstances;
    RRDHOST *rrdhost;

    RRD_FLAGS last_queued_flags;
    usec_t last_queued_ut;
    usec_t last_delay_calc_ut;
    usec_t scheduled_dispatch_ut;

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

static void rrdcontext_recalculate_host_retention(RRDHOST *host, RRD_FLAGS reason, int job_id);

#define rrdcontext_lock(rc) netdata_mutex_lock(&((rc)->mutex))
#define rrdcontext_unlock(rc) netdata_mutex_unlock(&((rc)->mutex))

// ----------------------------------------------------------------------------
// Updates triggers

static void rrdmetric_trigger_updates(RRDMETRIC *rm);
static void rrdinstance_trigger_updates(RRDINSTANCE *ri);
static void rrdcontext_trigger_updates(RRDCONTEXT *rc, bool force, RRD_FLAGS reason);

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

    size_t added = 0;
    for(int i = 0; rrdcontext_reasons[i].name ;i++) {
        if(flags & rrdcontext_reasons[i].flag) {
            if(added++) buffer_strcat(wb, ", ");
            buffer_strcat(wb, rrdcontext_reasons[i].name);
        }
    }
    if(!added)
        buffer_strcat(wb, "NONE");

    internal_error(true, "%s", buffer_tostring(wb));
    buffer_free(wb);
}
#else
#define log_transition(metric, instance, context, flags, msg) debug_dummy()
#endif

#ifdef LOG_RRDINSTANCES
static void rrdinstance_log(RRDINSTANCE *ri, const char *msg, bool rrdmetrics_is_write_locked) {
    char uuid[UUID_STR_LEN + 1];

    uuid_unparse(ri->uuid, uuid);

    BUFFER *wb = buffer_create(1000);

    buffer_sprintf(wb,
                   "RRDINSTANCE: %s id '%s' (host '%s'), uuid '%s', name '%s', context '%s', title '%s', units '%s', priority %zu, chart type '%s', update every %d, rrdset '%s', flags %s%s%s%s%s, first_time_t %ld, last_time_t %ld",
                   msg,
                   string2str(ri->id),
                   ri->rc->rrdhost->hostname,
                   uuid,
                   string2str(ri->name),
                   string2str(ri->rc->id),
                   string2str(ri->title),
                   string2str(ri->units),
                   ri->priority,
                   rrdset_type_name(ri->chart_type),
                   ri->update_every,
                   ri->rrdset?ri->rrdset->id:"NONE",
                   ri->flags & RRD_FLAG_DELETED ?"DELETED ":"",
                   ri->flags & RRD_FLAG_UPDATED ?"UPDATED ":"",
                   rrd_flag_is_collected(ri) ?"COLLECTED ":"",
                   rrd_flag_is_archived(ri) ?"ARCHIVED ":"",
                   ri->flags & RRD_FLAG_OWNLABELS ?"OWNLABELS ":"",
                   ri->first_time_t,
                   ri->last_time_t
                   );

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
        dfe_start_rw((DICTIONARY *)ri->rrdmetrics, v, (rrdmetrics_is_write_locked)?'u':'r') {
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

    info("%s", buffer_tostring(wb));
    buffer_free(wb);
}
#else
#define rrdinstance_log(ir, msg, lock) debug_dummy()
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

    rm->flags |= rm_new->flags & RRD_FLAGS_ALLOWED_EXTERNALLY_ON_NEW_OBJECTS;

    if(rrd_flag_is_collected(rm) && rrd_flag_is_archived(rm))
        rrd_flag_set_collected(rm);

    rrdmetric_free(rm_new);

    // the react callback will continue from here
}

static void rrdmetric_react_callback(const char *id __maybe_unused, void *value, void *data __maybe_unused) {
    RRDMETRIC *rm = value;

    rrdmetric_trigger_updates(rm);
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

static void rrdmetric_trigger_updates(RRDMETRIC *rm) {
    if(likely(!(rm->flags & RRD_FLAG_UPDATED))) return;

    rrdmetric_update_retention(rm);

    if(unlikely((rm->flags & RRD_FLAG_DELETED) && rm->rrddim)) {
        rm->flags &= ~RRD_FLAG_DELETED;
    }

    if(unlikely(rm->flags & RRD_FLAG_UPDATED)) {
        rm->ri->flags |= RRD_FLAG_UPDATED;
        log_transition(rm->id, rm->ri->id, rm->ri->rc->id, rm->flags, "RRDMETRIC");
        rrdinstance_trigger_updates(rm->ri);
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

    if(unlikely(rrd_flag_is_collected(rm))) {
        rrd_flag_set_archived(rm);
        rrd_flag_set_updated(rm, RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED);
    }

    rm->rrddim = NULL;
    rrdmetric_trigger_updates(rm);
    rrdmetric_release(rd->rrdmetric);
    rd->rrdmetric = NULL;
}

static inline void rrdmetric_updated_rrddim_flags(RRDDIM *rd) {
    RRDMETRIC *rm = rrddim_get_rrdmetric(rd);

    if(unlikely(rd->flags & (RRDDIM_FLAG_ARCHIVED | RRDDIM_FLAG_OBSOLETE))) {
        if(unlikely(rrd_flag_is_collected(rm))) {
            rrd_flag_set_archived(rm);
            rrd_flag_set_updated(rm, RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED);
        }
    }

    rrdmetric_trigger_updates(rm);
}

static inline void rrdmetric_collected_rrddim(RRDDIM *rd) {
    RRDMETRIC *rm = rrddim_get_rrdmetric(rd);

    if(unlikely(!rrd_flag_is_collected(rm))) {
        rrd_flag_set_collected(rm);
        rrd_flag_set_updated(rm, RRD_FLAG_UPDATE_REASON_STARTED_BEING_COLLECTED);
    }

    rrdmetric_trigger_updates(rm);
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

    if(ri->flags & RRD_FLAG_OWNLABELS)
        dictionary_destroy(ri->rrdlabels);

    rrdmetrics_destroy(ri);
    string_freez(ri->id);
    string_freez(ri->name);
    string_freez(ri->title);
    string_freez(ri->units);

    ri->id = NULL;
    ri->name = NULL;
    ri->title = NULL;
    ri->units = NULL;
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
        if(ri->flags & RRD_FLAG_OWNLABELS)
            ri->flags &= ~RRD_FLAG_OWNLABELS;
    }
    else {
        ri->rrdlabels = rrdlabels_create();
        ri->flags |= RRD_FLAG_OWNLABELS;
    }

    rrdmetrics_create(ri);

    rrdinstance_log(ri, "INSERT", false);

    // signal the react callback to do the job
    rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_NEW_OBJECT);
}

static void rrdinstance_delete_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDCONTEXT *rc = data; (void)rc;
    RRDINSTANCE *ri = (RRDINSTANCE *)value;

    rrdinstance_log(ri, "DELETE", false);

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

        if(ri->flags & RRD_FLAG_OWNLABELS) {
            DICTIONARY *old = ri->rrdlabels;
            ri->rrdlabels = ri->rrdset->state->chart_labels;
            ri->flags &= ~RRD_FLAG_OWNLABELS;
            rrdlabels_destroy(old);
        }
    }

    ri->flags |= ri_new->flags & RRD_FLAGS_ALLOWED_EXTERNALLY_ON_NEW_OBJECTS;
    if(rrd_flag_is_collected(ri) && rrd_flag_is_archived(ri))
        rrd_flag_set_collected(ri);

    rrdinstance_log(ri, "CONFLICT", false);

    // free the new one
    rrdinstance_free(ri_new);

    // the react callback will continue from here
}

static void rrdinstance_react_callback(const char *id __maybe_unused, void *value, void *data __maybe_unused) {
    RRDINSTANCE *ri = value;

    rrdinstance_trigger_updates(ri);
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

static void rrdinstance_trigger_updates(RRDINSTANCE *ri) {
    if(unlikely(!(ri->flags & RRD_FLAG_UPDATED))) return;
    rrd_flag_unset_updated(ri);

    RRD_FLAGS combined_metrics_flags = RRD_FLAG_NONE;
    time_t min_first_time_t = LONG_MAX, max_last_time_t = 0;
    size_t metrics_active = 0, metrics_deleted = 0;
    {
        RRDMETRIC *rm;
        dfe_start_write((DICTIONARY *)ri->rrdmetrics, rm) {
            // find the combined flags of all the metrics
            combined_metrics_flags |= rm->flags & RRD_FLAGS_PROPAGATED_UPSTREAM;

            if (unlikely((rm->flags & RRD_FLAG_DELETED) && !rm->rrddim)) {
                if(unlikely(dictionary_del_unsafe(ri->rrdmetrics, string2str(rm->id)) != 0))
                    error("RRDINSTANCE: '%s' failed to delete rrdmetric", string2str(ri->id));

                metrics_deleted++;
                continue;
            }

            metrics_active++;

            if (rm->first_time_t == 0 || rm->last_time_t == 0)
                continue;

            if (rm->first_time_t < min_first_time_t)
                min_first_time_t = rm->first_time_t;

            if (rm->last_time_t > max_last_time_t)
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

        if(likely(combined_metrics_flags & RRD_FLAG_COLLECTED)) {
            if(unlikely(!rrd_flag_is_collected(ri))) {
                rrd_flag_set_collected(ri);
                rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_STARTED_BEING_COLLECTED);
            }
        }
        else {
            if(unlikely(!rrd_flag_is_archived(ri))) {
                rrd_flag_set_archived(ri);
                rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED);
            }
        }
    }
    else {
        // no deleted metrics, no active metrics
        // just hanging there...

        if(unlikely(rrd_flag_is_collected(ri))) {
            rrd_flag_set_archived(ri);
            rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED);
        }
    }

    if(unlikely((ri->flags & RRD_FLAG_DELETED) && ri->rrdset)) {
        ri->flags &= ~RRD_FLAG_DELETED;
    }

    if(unlikely(ri->flags & RRD_FLAG_UPDATED)) {
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

    if(unlikely(rrd_flag_is_collected(ri))) {
        rrd_flag_set_archived(ri);
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED);
    }

    if(!(ri->flags & RRD_FLAG_OWNLABELS)) {
        ri->flags &= ~RRD_FLAG_OWNLABELS;
        ri->rrdlabels = rrdlabels_create();
        rrdlabels_copy(ri->rrdlabels, st->state->chart_labels);
    }

    ri->rrdset = NULL;
    rrdinstance_trigger_updates(ri);
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

    rrdinstance_trigger_updates(ri);
}

static inline void rrdinstance_updated_rrdset_flags(RRDSET *st) {
    RRDINSTANCE *ri = rrdset_get_rrdinstance(st);

    if(unlikely(st->flags & (RRDSET_FLAG_ARCHIVED | RRDSET_FLAG_OBSOLETE))) {
        rrd_flag_set_archived(ri);
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED);
    }

    rrdinstance_trigger_updates(ri);
}

static inline void rrdinstance_collected_rrdset(RRDSET *st) {
    RRDINSTANCE *ri = rrdset_get_rrdinstance(st);

    if(unlikely(!rrd_flag_is_collected(ri))) {
        rrd_flag_set_collected(ri);
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_STARTED_BEING_COLLECTED);
    }

    rrdinstance_trigger_updates(ri);
}

// ----------------------------------------------------------------------------
// RRDCONTEXT

static void rrdcontext_freez(RRDCONTEXT *rc) {
    string_freez(rc->id);
    string_freez(rc->title);
    string_freez(rc->units);
}

static uint64_t rrdcontext_get_next_version(RRDCONTEXT *rc) {
    time_t now = now_realtime_sec();
    uint64_t version = MAX(rc->version, rc->hub.version);
    version = MAX((uint64_t)now, version);
    version++;
    return version;
}

static void rrdcontext_message_send_unsafe(RRDCONTEXT *rc) {

    // save it, so that we know the last version we sent to hub
    rc->version = rc->hub.version = rrdcontext_get_next_version(rc);
    rc->hub.id = string2str(rc->id);
    rc->hub.title = string2str(rc->title);
    rc->hub.units = string2str(rc->units);
    rc->hub.chart_type = rrdset_type_name(rc->chart_type);
    rc->hub.priority = rc->priority;
    rc->hub.first_time_t = rc->first_time_t;
    rc->hub.last_time_t = rrd_flag_is_collected(rc) ? 0 : rc->last_time_t;
    rc->hub.deleted = (rc->flags & RRD_FLAG_DELETED) ? true : false;

    // TODO call the ACLK function to send this message


    // store it to SQL
    if(ctx_store_context(&rc->rrdhost->host_uuid, &rc->hub) != 0)
        error("RRDCONTEXT: failed to save context '%s' version %lu to SQL.", rc->hub.id, rc->hub.version);
}

static bool check_if_cloud_version_changed_unsafe(RRDCONTEXT *rc, const char *msg) {
    bool id_changed = false,
         title_changed = false,
         units_changed = false,
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

    if(unlikely(id_changed || title_changed || units_changed || chart_type_changed || priority_changed || first_time_changed || last_time_changed || deleted_changed)) {

        internal_error(true, "RRDCONTEXT: %s NEW VERSION '%s'%s, version %zu, title '%s'%s, units '%s'%s, chart type '%s'%s, priority %lu%s, first_time_t %ld%s, last_time_t %ld%s, deleted '%s'%s",
                       msg,
                       string2str(rc->id), id_changed ? " (CHANGED)" : "",
                       rc->version,
                       string2str(rc->title), title_changed ? " (CHANGED)" : "",
                       string2str(rc->units), units_changed ? " (CHANGED)" : "",
                       rrdset_type_name(rc->chart_type), chart_type_changed ? " (CHANGED)" : "",
                       rc->priority, priority_changed ? " (CHANGED)" : "",
                       rc->first_time_t, first_time_changed ? " (CHANGED)" : "",
                       rc->last_time_t, last_time_changed ? " (CHANGED)" : "",
                       (rc->flags & RRD_FLAG_DELETED) ? "true" : "false", deleted_changed ? " (CHANGED)" : ""
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

    //internal_error(true, "RRDCONTEXT: INSERT '%s' on host '%s', version %lu, title '%s', units '%s', chart type '%s', priority %zu, first_time_t %ld, last_time_t %ld, options %s%s%s",
    //               string2str(rc->id),
    //               host->hostname,
    //               rc->version,
    //               string2str(rc->title),
    //               string2str(rc->units),
    //               rrdset_type_name(rc->chart_type),
    //               rc->priority,
    //               rc->first_time_t,
    //               rc->last_time_t,
    //               rc->flags & RRD_FLAG_DELETED ? "DELETED ":"",
    //               rrd_flag_is_collected(rc) ? "COLLECTED ":"",
    //               rc->flags & RRD_FLAG_UPDATED ? "UPDATED ": "");

    // signal the react callback to do the job
    rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_NEW_OBJECT);
}

static void rrdcontext_delete_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDHOST *host = (RRDHOST *)data;
    (void)host;

    RRDCONTEXT *rc = (RRDCONTEXT *)value;

    //internal_error(true, "RRDCONTEXT: DELETE '%s' on host '%s', version %lu, title '%s', units '%s', chart type '%s', priority %zu, first_time_t %ld, last_time_t %ld, options %s%s%s",
    //               string2str(rc->id),
    //               host->hostname,
    //               rc->version,
    //               string2str(rc->title),
    //               string2str(rc->units),
    //               rrdset_type_name(rc->chart_type),
    //               rc->priority,
    //               rc->first_time_t,
    //               rc->last_time_t,
    //               rc->flags & RRD_FLAG_DELETED ? "DELETED ":"",
    //               rrd_flag_is_collected(rc) ? "COLLECTED ":"",
    //               rc->flags & RRD_FLAG_UPDATED ? "UPDATED ": "");

    rrdinstances_destroy(rc);
    netdata_mutex_destroy(&rc->mutex);
    rrdcontext_freez(rc);
}

static STRING *merge_titles(RRDCONTEXT *rc, STRING *a, STRING *b) {
    if(strcmp(string2str(a), "X") == 0)
        return string_dup(b);

    size_t alen = string_length(a);
    size_t blen = string_length(b);
    size_t length = MAX(alen, blen);
    char buf1[length + 1], buf2[length + 1], *dst1, *dst2;
    const char *s1, *s2;

    s1 = string2str(a);
    s2 = string2str(b);
    dst1 = buf1;
    for( ; *s1 && *s2 && *s1 == *s2 ;s1++, s2++)
        *dst1++ = *s1;

    *dst1 = '\0';

    if(*s1 != '\0' || *s2 != '\0') {
        *dst1++ = 'X';

        s1 = &(string2str(a))[alen - 1];
        s2 = &(string2str(b))[blen - 1];
        dst2 = &buf2[length];
        *dst2 = '\0';
        for (; *s1 && *s2 && *s1 == *s2; s1--, s2--)
            *(--dst2) = *s1;

        strcpy(dst1, dst2);
    }

    if(strcmp(buf1, "X") == 0)
        return string_dup(a);

    internal_error(true, "RRDCONTEXT: '%s' merged title '%s' and title '%s' as '%s'", string2str(rc->id), string2str(a), string2str(b), buf1);
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
        rc->title = merge_titles(rc, rc->title, rc_new->title);
        string_freez(old_title);
        rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_TITLE);
    }

    if(rc->units != rc_new->units) {
        STRING *old_units = rc->units;
        rc->units = string_dup(rc_new->units);
        string_freez(old_units);
        rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_UNITS);
    }

    if(rc->chart_type != rc_new->chart_type) {
        rc->chart_type = rc_new->chart_type;
        rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_CHART_TYPE);
    }

    if(rc->priority != rc_new->priority) {
        rc->priority = rc_new->priority;
        rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_PRIORITY);
    }

    rc->flags |= rc_new->flags & RRD_FLAGS_ALLOWED_EXTERNALLY_ON_NEW_OBJECTS;
    if(rrd_flag_is_collected(rc) && rrd_flag_is_archived(rc))
        rrd_flag_set_collected(rc);

    //internal_error(true, "RRDCONTEXT: CONFLICT '%s' on host '%s', version %lu, title '%s', units '%s', chart type '%s', priority %zu, first_time_t %ld, last_time_t %ld, options %s%s%s",
    //               string2str(rc->id),
    //               host->hostname,
    //               rc->version,
    //               string2str(rc->title),
    //               string2str(rc->units),
    //               rrdset_type_name(rc->chart_type),
    //               rc->priority,
    //               rc->first_time_t,
    //               rc->last_time_t,
    //               rc->flags & RRD_FLAG_DELETED ? "DELETED ":"",
    //               rrd_flag_is_collected(rc) ? "COLLECTED ":"",
    //               rc->flags & RRD_FLAG_UPDATED ? "UPDATED ": "");

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
        host->rrdctx = NULL;
    }

    dictionary_destroy((DICTIONARY *)host->rrdctx);
    host->rrdctx = NULL;
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
        dfe_start_write(rc->rrdinstances, ri) {
            // find the combined flags of all the metrics
            combined_instances_flags |= ri->flags & RRD_FLAGS_PROPAGATED_UPSTREAM;

            if (unlikely((ri->flags & RRD_FLAG_DELETED) && !ri->rrdset)) {
                if(unlikely(dictionary_del_unsafe(rc->rrdinstances, string2str(ri->id)) != 0))
                    error("RRDCONTEXT: '%s' failed to delete rrdinstance", string2str(rc->id));

                instances_deleted++;
                continue;
            }

            instances_active++;

            if (ri->priority > 0 && ri->priority < min_priority)
                min_priority = ri->priority;

            if (!ri->first_time_t || !ri->last_time_t)
                continue;

            if (ri->first_time_t < min_first_time_t)
                min_first_time_t = ri->first_time_t;

            if (ri->last_time_t > max_last_time_t)
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

        if(likely(combined_instances_flags & RRD_FLAG_COLLECTED)) {
            if(unlikely(!rrd_flag_is_collected(rc))) {
                rrd_flag_set_collected(rc);
                rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_STARTED_BEING_COLLECTED);
            }
        }
        else {
            if(unlikely(!rrd_flag_is_archived(rc))) {
                rrd_flag_set_archived(rc);
                rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED);
            }
        }

        if (min_priority != LONG_MAX && rc->priority != min_priority) {
            rc->priority = min_priority;
            rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_PRIORITY);
        }
    }
    else {
        // no deleted instances, no active instances
        // just hanging there...

        if(unlikely(rrd_flag_is_collected(rc))) {
            rrd_flag_set_archived(rc);
            rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED);
        }
    }

    if(unlikely(rc->flags & RRD_FLAG_UPDATED)) {
        log_transition(NULL, NULL, rc->id, rc->flags, "RRDCONTEXT");

        if(check_if_cloud_version_changed_unsafe(rc, "QUEUE")) {
            rc->version = rrdcontext_get_next_version(rc);
            rc->last_queued_ut = now_realtime_usec();
            rc->last_queued_flags |= rc->flags;
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

// TODO - cleanup contexts that no longer have any retention
//

static inline usec_t rrdcontext_queued_dispatch_ut(RRDCONTEXT *rc, usec_t now_ut) {

    if(likely(rc->last_delay_calc_ut >= rc->last_queued_ut))
        return rc->scheduled_dispatch_ut;

    RRD_FLAGS flags = rc->last_queued_flags;

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

    rc->last_delay_calc_ut = now_ut;
    usec_t dispatch_ut = rc->scheduled_dispatch_ut = rc->last_queued_ut + delay;
    return dispatch_ut;
}

#define RRDCONTEXT_DELAY_AFTER_DB_ROTATION_SECS 120
// #define RRDCONTEXT_CLEANUP_DELETED_EVERY_SECS 3600 // not needed, now it is done at dequeue
#define RRDCONTEXT_HEARTBEAT_SECS 1

#define WORKER_JOB_HOSTS            1
#define WORKER_JOB_CHECK            2
#define WORKER_JOB_SEND             3
#define WORKER_JOB_DEQUEUE          4
#define WORKER_JOB_RETENTION        5
#define WORKER_JOB_QUEUED           6
#define WORKER_JOB_CLEANUP          7
#define WORKER_JOB_CLEANUP_DELETE   8

usec_t rrdcontext_last_cleanup_ut = 0;
usec_t rrdcontext_last_db_rotation_ut = 0;

void rrdcontext_db_rotation(void) {
    rrdcontext_last_db_rotation_ut = now_realtime_usec();
}

static uint64_t rrdcontext_version_hash(RRDHOST *host) {
    if(unlikely(!host || !host->rrdctx)) return 0;

    RRDCONTEXT *rc;
    uint64_t hash = 0;

    // loop through all contexts of the host
    dfe_start_read((DICTIONARY *)host->rrdctx, rc) {

        // skip any deleted contexts
        if(unlikely(rc->flags & RRD_FLAG_DELETED))
            continue;

        // we use rc->hub.* which has the latest
        // metadata we have sent to the hub

        // if a context is currently queued, rc->hub.* does NOT
        // reflect the queued changes. rc->hub.* is updated with
        // their metadata, after messages are dispatched to hub.

        // when the context is being collected,
        // rc->hub.last_time_t is already zero

        hash += rc->hub.version + rc->hub.last_time_t - rc->hub.first_time_t;

    }
    dfe_done(rc);

    return hash;
}

static void rrdcontext_recalculate_host_retention(RRDHOST *host, RRD_FLAGS reason, int job_id) {
    if(unlikely(!host || !host->rrdctx)) return;

    RRDCONTEXT *rc;
    dfe_start_read((DICTIONARY *)host->rrdctx, rc) {
        RRDINSTANCE *ri;
        dfe_start_read(rc->rrdinstances, ri) {
            RRDMETRIC *rm;
            dfe_start_read(ri->rrdmetrics, rm) {

                if(job_id >= 0)
                    worker_is_busy(job_id);

                rrd_flag_set_updated(rm, reason);
                rrdmetric_trigger_updates(rm);
            }
            dfe_done(rm);
        }
        dfe_done(ri);
    }
    dfe_done(rc);
}

static void rrdcontext_recalculate_retention(int job_id) {
    rrdcontext_last_db_rotation_ut = 0;
    rrd_rdlock();
    RRDHOST *host;
    rrdhost_foreach_read(host) {
        rrdcontext_recalculate_host_retention(host, RRD_FLAG_UPDATE_REASON_DB_ROTATION, job_id);
    }
    rrd_unlock();
}

/*
static void rrdcontext_cleanup_deleted_unqueued_contexts(void) {
    rrd_rdlock();
    RRDHOST *host;
    rrdhost_foreach_read(host) {
        RRDCONTEXT *rc;
        dfe_start_write((DICTIONARY *)host->rrdctx, rc) {
            worker_is_busy(WORKER_JOB_CLEANUP);
            if(unlikely((rc->flags & RRD_FLAG_DELETED) && !(rc->flags & RRD_FLAG_QUEUED))) {
                worker_is_busy(WORKER_JOB_CLEANUP_DELETE);

                // we need to refresh the string pointers in rc->hub
                // in case the context changed values
                rc->hub.id = string2str(rc->id);
                rc->hub.title = string2str(rc->title);
                rc->hub.units = string2str(rc->units);

                // delete it from SQL
                ctx_delete_context(&host->host_uuid, &rc->hub);

                // delete it from the dictionary
                if(dictionary_del_unsafe((DICTIONARY *)host->rrdctx, string2str(rc->id)) != 0)
                    error("RRDCONTEXT: '%s' of host '%s' failed to be deleted from rrdcontext dictionary.", string2str(rc->id), host->hostname);
            }
        }
        dfe_done(rc);
    }
    rrd_unlock();
}
*/

static void rrdcontext_main_cleanup(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;
    // custom code
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void *rrdcontext_main(void *ptr) {
    if(unlikely(rrdcontext_enabled == CONFIG_BOOLEAN_NO))
        return NULL;

    rrdcontext_last_cleanup_ut = now_realtime_usec();
    
    worker_register("RRDCONTEXT");
    worker_register_job_name(WORKER_JOB_HOSTS, "hosts");
    worker_register_job_name(WORKER_JOB_CHECK, "dedup checks");
    worker_register_job_name(WORKER_JOB_SEND, "sent contexts");
    worker_register_job_name(WORKER_JOB_DEQUEUE, "deduped contexts");
    worker_register_job_name(WORKER_JOB_RETENTION, "metrics retention");
    worker_register_job_name(WORKER_JOB_QUEUED, "queued contexts");
    worker_register_job_name(WORKER_JOB_CLEANUP, "cleanup contexts");
    worker_register_job_name(WORKER_JOB_CLEANUP_DELETE, "deleted contexts");

    netdata_thread_cleanup_push(rrdcontext_main_cleanup, ptr);
    heartbeat_t hb;
    heartbeat_init(&hb);
    usec_t step = USEC_PER_SEC * RRDCONTEXT_HEARTBEAT_SECS;

    while (!netdata_exit) {
        worker_is_idle();
        heartbeat_next(&hb, step);

        usec_t now_ut = now_realtime_usec();

        if(now_ut < rrdcontext_last_db_rotation_ut + RRDCONTEXT_DELAY_AFTER_DB_ROTATION_SECS * USEC_PER_SEC) {
            rrdcontext_recalculate_retention(WORKER_JOB_RETENTION);
        }

        // cleanup is not needed like this - it is done during dequeue
        //if(now_ut < rrdcontext_last_cleanup_ut + RRDCONTEXT_CLEANUP_DELETED_EVERY_SECS * USEC_PER_SEC) {
        //    rrdcontext_cleanup_deleted_unqueued_contexts();
        //}

        rrd_rdlock();
        RRDHOST *host;
        rrdhost_foreach_read(host) {
            worker_is_busy(WORKER_JOB_HOSTS);

            if(!dictionary_stats_entries((DICTIONARY *)host->rrdctx_queue)) continue;

            RRDCONTEXT *rc;
            dfe_start_write((DICTIONARY *)host->rrdctx_queue, rc) {
                worker_is_busy(WORKER_JOB_QUEUED);
                usec_t dispatch_ut = rrdcontext_queued_dispatch_ut(rc, now_ut);

                if(unlikely(now_ut >= dispatch_ut)) {
                    worker_is_busy(WORKER_JOB_CHECK);

                    rrdcontext_lock(rc);

                    if(check_if_cloud_version_changed_unsafe(rc, "SENDING")) {
                        worker_is_busy(WORKER_JOB_SEND);
                        rrdcontext_message_send_unsafe(rc);
                    }
                    else
                        rc->version = rc->hub.version;
                    
                    // remove the queued flag, so that it can be queued again
                    rc->flags &= ~RRD_FLAG_QUEUED;

                    // remove it from the queue
                    worker_is_busy(WORKER_JOB_DEQUEUE);
                    dictionary_del_unsafe((DICTIONARY *)host->rrdctx_queue, string2str(rc->id));

                    if(unlikely(rc->flags & RRD_FLAG_DELETED)) {
                        // this is a deleted context - delete it forever...

                        worker_is_busy(WORKER_JOB_CLEANUP_DELETE);

                        // we need to refresh the string pointers in rc->hub
                        // in case the context changed values
                        rc->hub.id = string2str(rc->id);
                        rc->hub.title = string2str(rc->title);
                        rc->hub.units = string2str(rc->units);

                        // delete it from SQL
                        ctx_delete_context(&host->host_uuid, &rc->hub);

                        STRING *id = string_dup(rc->id);

                        rrdcontext_unlock(rc);

                        // delete it from the master dictionary
                        if(dictionary_del((DICTIONARY *)host->rrdctx, string2str(rc->id)) != 0)
                            error("RRDCONTEXT: '%s' of host '%s' failed to be deleted from rrdcontext dictionary.", string2str(id), host->hostname);

                        string_freez(id);
                    }
                    else
                        rrdcontext_unlock(rc);
                }
            }
            dfe_done(rc);
        }
        rrd_unlock();
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}

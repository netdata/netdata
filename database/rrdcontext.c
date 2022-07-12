// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdcontext.h"
#include "sqlite/sqlite_context.h"

#define LOG_TRANSITIONS 1
// #define LOG_RRDINSTANCES 1

static int log_calls = 1;

typedef enum {
    RRD_FLAG_NONE           = 0,
    RRD_FLAG_DELETED        = (1 << 0), // this is a deleted object, we will immediately remove
    RRD_FLAG_COLLECTED      = (1 << 1),
    RRD_FLAG_UPDATED        = (1 << 2),
    RRD_FLAG_ARCHIVED       = (1 << 3),
    RRD_FLAG_OWNLABELS      = (1 << 4),
    RRD_FLAG_LIVE_RETENTION = (1 << 5),

    RRD_FLAG_UPDATE_REASON_CHANGED_UPDATE_EVERY    = (1 << 14),
    RRD_FLAG_UPDATE_REASON_CHANGED_LINKING         = (1 << 15),
    RRD_FLAG_UPDATE_REASON_CHANGED_NAME            = (1 << 16),
    RRD_FLAG_UPDATE_REASON_CHANGED_UUID            = (1 << 17),
    RRD_FLAG_UPDATE_REASON_NEW_OBJECT              = (1 << 18),
    RRD_FLAG_UPDATE_REASON_ZERO_RETENTION          = (1 << 19),
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
    RRD_FLAG_UPDATE_REASON_NETDATA_EXIT            = (1 << 30),
    RRD_FLAG_UPDATE_REASON_LOAD_SQL                = (1 << 31),
} RRD_FLAGS;

#define RRD_FLAG_UPDATE_REASONS ( \
     RRD_FLAG_UPDATE_REASON_CHANGED_UPDATE_EVERY \
    |RRD_FLAG_UPDATE_REASON_CHANGED_LINKING \
    |RRD_FLAG_UPDATE_REASON_CHANGED_NAME \
    |RRD_FLAG_UPDATE_REASON_CHANGED_UUID \
    |RRD_FLAG_UPDATE_REASON_NEW_OBJECT \
    |RRD_FLAG_UPDATE_REASON_ZERO_RETENTION \
    |RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T \
    |RRD_FLAG_UPDATE_REASON_CHANGED_LAST_TIME_T \
    |RRD_FLAG_UPDATE_REASON_CHANGED_CHART_TYPE \
    |RRD_FLAG_UPDATE_REASON_CHANGED_PRIORITY \
    |RRD_FLAG_UPDATE_REASON_CHANGED_UNITS \
    |RRD_FLAG_UPDATE_REASON_CHANGED_TITLE \
    |RRD_FLAG_UPDATE_REASON_CONNECTED_CHILD \
    |RRD_FLAG_UPDATE_REASON_DISCONNECTED_CHILD \
    |RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED \
    |RRD_FLAG_UPDATE_REASON_STARTED_BEING_COLLECTED \
    |RRD_FLAG_UPDATE_REASON_NETDATA_EXIT \
    |RRD_FLAG_UPDATE_REASON_LOAD_SQL \
)

#define rrd_flag_set_updated(obj, reason) (obj)->flags |= (RRD_FLAG_UPDATED | (reason))
#define rrd_flag_unset_updated(obj) (obj)->flags &= ~(RRD_FLAG_UPDATED|RRD_FLAG_UPDATE_REASONS)

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
    return (RRDCONTEXT_ACQUIRED *)dictionary_get_and_acquire_item((DICTIONARY *)host->rrdcontexts, name);
}

static inline void rrdcontext_release(RRDCONTEXT_ACQUIRED *rca) {
    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);
    dictionary_acquired_item_release((DICTIONARY *)rc->rrdhost->rrdcontexts, (DICTIONARY_ITEM *)rca);
}

// ----------------------------------------------------------------------------
// Updates triggers

static void rrdmetric_trigger_updates(RRDMETRIC *rm);
static void rrdinstance_trigger_updates(RRDINSTANCE *ri);
static void rrdcontext_trigger_updates(RRDCONTEXT *rc);

// ----------------------------------------------------------------------------
// logging of all data collected

#ifdef LOG_TRANSITIONS
static struct {
    RRD_FLAGS flag;
    const char *name;
} transitions[] = {
    { RRD_FLAG_UPDATE_REASON_NEW_OBJECT, "object created" },
    { RRD_FLAG_UPDATE_REASON_LOAD_SQL, "loaded from sql" },
    { RRD_FLAG_UPDATE_REASON_CHANGED_TITLE, "changed title" },
    { RRD_FLAG_UPDATE_REASON_CHANGED_UNITS, "changed units" },
    { RRD_FLAG_UPDATE_REASON_CHANGED_PRIORITY, "changed priority" },
    { RRD_FLAG_UPDATE_REASON_ZERO_RETENTION, "has no retention" },
    { RRD_FLAG_UPDATE_REASON_CHANGED_UUID, "changed uuid" },
    { RRD_FLAG_UPDATE_REASON_CHANGED_UPDATE_EVERY, "changed updated every" },
    { RRD_FLAG_UPDATE_REASON_CHANGED_LINKING, "changed rrd link" },
    { RRD_FLAG_UPDATE_REASON_CHANGED_NAME, "changed name" },
    { RRD_FLAG_UPDATE_REASON_CHANGED_FIRST_TIME_T, "updated first_time_t" },
    { RRD_FLAG_UPDATE_REASON_CHANGED_LAST_TIME_T, "updated last_time_t" },
    { RRD_FLAG_UPDATE_REASON_CHANGED_CHART_TYPE, "changed chart type" },
    { RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED, "stopped collected" },
    { RRD_FLAG_UPDATE_REASON_STARTED_BEING_COLLECTED, "started collected" },
    { RRD_FLAG_UPDATE_REASON_CONNECTED_CHILD, "child connected" },
    { RRD_FLAG_UPDATE_REASON_DISCONNECTED_CHILD, "child disconnected" },
    { RRD_FLAG_UPDATE_REASON_NETDATA_EXIT, "netdata exits" },
    { 0, NULL },
};

static void log_transition(STRING *metric, STRING *instance, STRING *context, RRD_FLAGS flags, const char *msg) {
    BUFFER *wb = buffer_create(1000);

    buffer_sprintf(wb, "RRD TRANSITION: context '%s'", string2str(context));

    if(instance)
        buffer_sprintf(wb, ", instance '%s'", string2str(instance));

    if(metric)
        buffer_sprintf(wb, ", metric '%s'", string2str(metric));

    buffer_sprintf(wb, ", triggered by %s: ", msg);

    size_t added = 0;
    for(int i = 0; transitions[i].name ;i++) {
        if(flags & transitions[i].flag) {
            if(added++) buffer_strcat(wb, ", ");
            buffer_strcat(wb, transitions[i].name);
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
                   ri->flags & RRD_FLAG_COLLECTED ?"COLLECTED ":"",
                   ri->flags & RRD_FLAG_ARCHIVED ?"ARCHIVED ":"",
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
static void rrdmetric_insert_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDINSTANCE *ri = data;
    RRDMETRIC *rm = value;

    // link it to its parent
    rm->ri = ri;

    // remove flags that we need to figure out at runtime
    rm->flags = rm->flags & (RRD_FLAG_ARCHIVED | RRD_FLAG_UPDATE_REASONS);

    // signal the react callback to do the job
    rrd_flag_set_updated(rm, RRD_FLAG_UPDATE_REASON_NEW_OBJECT);
}

// called when this rrdmetric is deleted from the rrdmetrics dictionary of a rrdinstance
static void rrdmetric_delete_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDINSTANCE *ri = data; (void)ri;
    RRDMETRIC *rm = value;

    // free the resources
    rrdmetric_free(rm);
}

// called when the same rrdmetric is inserted again to the rrdmetrics dictionary of a rrdinstance
static void rrdmetric_conflict_callback(const char *id, void *oldv, void *newv, void *data) {
    (void)id;
    RRDINSTANCE *ri   = data; (void)ri;
    RRDMETRIC *rm     = oldv;
    RRDMETRIC *rm_new = newv;

    if(rm->id != rm_new->id)
        fatal("RRDMETRIC: '%s' cannot change id to '%s'", string2str(rm->id), string2str(rm_new->id));

    if(uuid_compare(rm->uuid, rm_new->uuid) != 0) {
        char uuid1[UUID_STR_LEN], uuid2[UUID_STR_LEN];
        uuid_unparse(rm->uuid, uuid1);
        uuid_unparse(rm_new->uuid, uuid2);
        internal_error(true, "RRDMETRIC: '%s' of instance '%s' changed uuid from '%s' to '%s'", string2str(rm->id), string2str(ri->id), uuid1, uuid2);
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

    rm->flags |= rm_new->flags & (RRD_FLAG_ARCHIVED | RRD_FLAG_UPDATE_REASONS);

    if((rm->flags & RRD_FLAG_COLLECTED) && (rm->flags & RRD_FLAG_ARCHIVED))
        rm->flags &= ~RRD_FLAG_ARCHIVED;

    rrdmetric_free(rm_new);

    // the react callback will continue from here
}

static void rrdmetric_react_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDINSTANCE *ri = data; (void)ri;
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
    if(!(rm->flags & RRD_FLAG_UPDATED)) return;

    rrdmetric_update_retention(rm);

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

    if(unlikely(rm->flags & RRD_FLAG_COLLECTED)) {
        rm->flags |= RRD_FLAG_ARCHIVED;
        rm->flags &= ~RRD_FLAG_COLLECTED;
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
        if(unlikely(rm->flags & RRD_FLAG_COLLECTED)) {
            rm->flags |= RRD_FLAG_ARCHIVED;
            rm->flags &= ~RRD_FLAG_COLLECTED;
            rrd_flag_set_updated(rm, RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED);
        }
    }

    rrdmetric_trigger_updates(rm);
}

static inline void rrdmetric_collected_rrddim(RRDDIM *rd) {
    RRDMETRIC *rm = rrddim_get_rrdmetric(rd);

    if(unlikely(!(rm->flags & RRD_FLAG_COLLECTED))) {
        rm->flags |= RRD_FLAG_COLLECTED;
        rm->flags &= ~RRD_FLAG_ARCHIVED;
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

static void rrdinstance_insert_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDCONTEXT *rc = data;
    RRDINSTANCE *ri = value;

    // link it to its parent
    ri->rc = rc;

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
    rrdinstance_free(ri);
}

static void rrdinstance_conflict_callback(const char *id, void *oldv, void *newv, void *data) {
    (void)id;
    RRDCONTEXT *rc      = data; (void)rc;
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

    ri->flags |= ri_new->flags & (RRD_FLAG_ARCHIVED | RRD_FLAG_UPDATE_REASONS);

    rrdinstance_log(ri, "CONFLICT", false);

    // free the new one
    rrdinstance_free(ri_new);

    // the react callback will continue from here
}

static void rrdinstance_react_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDCONTEXT *rc = data; (void)rc;
    RRDINSTANCE *ri = value;

    rrdinstance_trigger_updates(ri);
}

void rrdinstances_create(RRDCONTEXT *rc) {
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

    RRD_FLAGS flags = RRD_FLAG_NONE;
    time_t min_first_time_t = LONG_MAX, max_last_time_t = 0;
    size_t metrics_active = 0, metrics_deleted = 0;
    {
        RRDMETRIC *rm;
        dfe_start_write((DICTIONARY *)ri->rrdmetrics, rm) {
            // find the combined flags of all the metrics
            flags |= rm->flags & (RRD_FLAG_COLLECTED | RRD_FLAG_DELETED | RRD_FLAG_UPDATE_REASONS);

            if (unlikely(rm->flags & RRD_FLAG_DELETED)) {
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

            if(unlikely(flags & RRD_FLAG_LIVE_RETENTION)) {
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

        if(likely(flags & RRD_FLAG_COLLECTED)) {
            if(unlikely(!(ri->flags & RRD_FLAG_COLLECTED))) {
                ri->flags |= RRD_FLAG_COLLECTED;
                ri->flags &= ~RRD_FLAG_ARCHIVED;
                rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_STARTED_BEING_COLLECTED);
            }
        }
        else {
            if(unlikely(!(ri->flags & RRD_FLAG_ARCHIVED))) {
                ri->flags |= RRD_FLAG_ARCHIVED;
                ri->flags &= ~RRD_FLAG_COLLECTED;
                rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED);
            }
        }
    }
    else {
        // no deleted metrics, no active metrics
        // just hanging there...

        if(unlikely(ri->flags & RRD_FLAG_COLLECTED)) {
            ri->flags &= ~RRD_FLAG_COLLECTED;
            ri->flags |= RRD_FLAG_ARCHIVED;
            rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED);
        }
    }

    if(unlikely(ri->flags & RRD_FLAG_UPDATED)) {
        ri->rc->flags |= RRD_FLAG_UPDATED;
        log_transition(NULL, ri->id, ri->rc->id, ri->flags, "RRDINSTANCE");
        rrdcontext_trigger_updates(ri->rc);
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

    RRDCONTEXT_ACQUIRED *rca = (RRDCONTEXT_ACQUIRED *)dictionary_set_and_acquire_item((DICTIONARY *)st->rrdhost->rrdcontexts, string2str(tc.id), &tc, sizeof(tc));
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
        rrd_flag_set_updated(rc_old, RRD_FLAG_UPDATE_REASON_CHANGED_LINKING);
        rrdcontext_trigger_updates(rc_old);
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

    if(unlikely(ri->flags & RRD_FLAG_COLLECTED)) {
        ri->flags |= RRD_FLAG_ARCHIVED;
        ri->flags &= ~RRD_FLAG_COLLECTED;
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED);
    }

    if(!(ri->flags & RRD_FLAG_OWNLABELS)) {
        ri->flags &= ~RRD_FLAG_OWNLABELS;
        ri->rrdlabels = rrdlabels_create();
        rrdlabels_copy(ri->rrdlabels, st->state->chart_labels);
    }

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
        ri->flags |= RRD_FLAG_ARCHIVED;
        ri->flags &= ~RRD_FLAG_COLLECTED;
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED);
    }

    rrdinstance_trigger_updates(ri);
}

static inline void rrdinstance_collected_rrdset(RRDSET *st) {
    RRDINSTANCE *ri = rrdset_get_rrdinstance(st);

    if(unlikely(!(ri->flags & RRD_FLAG_COLLECTED))) {
        ri->flags |= RRD_FLAG_COLLECTED;
        ri->flags &= ~RRD_FLAG_ARCHIVED;
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

static void check_if_we_need_to_emit_new_version(RRDCONTEXT *rc) {
    bool version_changed = false,
         id_changed = false,
         title_changed = false,
         units_changed = false,
         chart_type_changed = false,
         priority_changed = false,
         first_time_changed = false,
         last_time_changed = false,
         deleted_changed = false;

    if(unlikely(rc->version != rc->hub.version))
        version_changed = true;

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

    if(unlikely((uint64_t)((rc->flags & RRD_FLAG_COLLECTED) ? 0 : rc->last_time_t) != rc->hub.last_time_t))
        last_time_changed = true;

    if(unlikely(((rc->flags & RRD_FLAG_DELETED) ? true : false) != rc->hub.deleted))
        deleted_changed = true;

    if(unlikely(version_changed || id_changed || title_changed || units_changed || chart_type_changed || priority_changed || first_time_changed || last_time_changed || deleted_changed)) {
        rc->version = rc->hub.version = (rc->version > rc->hub.version ? rc->version : rc->hub.version) + 1;
        rc->hub.id = string2str(rc->id);
        rc->hub.title = string2str(rc->title);
        rc->hub.units = string2str(rc->units);
        rc->hub.chart_type = rrdset_type_name(rc->chart_type);
        rc->hub.priority = rc->priority;
        rc->hub.first_time_t = rc->first_time_t;
        rc->hub.last_time_t = (rc->flags & RRD_FLAG_COLLECTED) ? 0 : rc->last_time_t;
        rc->hub.deleted = (rc->flags & RRD_FLAG_DELETED) ? true : false;

        internal_error(true, "RRDCONTEXT: NEW VERSION '%s'%s version %zu%s, title '%s'%s, units '%s'%s, chart type '%s'%s, priority %lu%s, first_time_t %lu%s, last_time_t %lu%s, deleted '%s'%s",
                       rc->hub.id, id_changed ? " (CHANGED)" : "",
                       rc->hub.version, version_changed ? " (CHANGED)" : "",
                       rc->hub.title, title_changed ? " (CHANGED)" : "",
                       rc->hub.units, units_changed ? " (CHANGED)" : "",
                       rc->hub.chart_type, chart_type_changed ? " (CHANGED)" : "",
                       rc->hub.priority, priority_changed ? " (CHANGED)" : "",
                       rc->hub.first_time_t, first_time_changed ? " (CHANGED)" : "",
                       rc->hub.last_time_t, last_time_changed ? " (CHANGED)" : "",
                       rc->hub.deleted ? "true" : "false", deleted_changed ? " (CHANGED)" : ""
                       );

        if(ctx_store_context(&rc->rrdhost->host_uuid, &rc->hub) != 0)
            error("RRDCONTEXT: failed to save context '%s' version %lu to SQL.", rc->hub.id, rc->hub.version);

        // TODO save in the output queue
    }
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
                rc->flags |= RRD_FLAG_COLLECTED;
            else
                rc->flags |= RRD_FLAG_ARCHIVED;
        }
    }
    else {
        // we are adding this context now for the first time
        rc->version = now_realtime_sec();
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
    //               rc->flags & RRD_FLAG_COLLECTED ? "COLLECTED ":"",
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
    //               rc->flags & RRD_FLAG_COLLECTED ? "COLLECTED ":"",
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
    //               rc->flags & RRD_FLAG_COLLECTED ? "COLLECTED ":"",
    //               rc->flags & RRD_FLAG_UPDATED ? "UPDATED ": "");

    // free the resources of the new one
    rrdcontext_freez(rc_new);

    // the react callback will continue from here
}

static void rrdcontext_react_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDHOST *host = (RRDHOST *)data; (void)host;
    RRDCONTEXT *rc = (RRDCONTEXT *)value;

    rrdcontext_trigger_updates(rc);
}

void rrdhost_create_rrdcontexts(RRDHOST *host) {
    if(unlikely(!host)) return;
    if(likely(host->rrdcontexts)) return;
    host->rrdcontexts = (RRDCONTEXTS *)dictionary_create(DICTIONARY_FLAG_DONT_OVERWRITE_VALUE);
    dictionary_register_insert_callback((DICTIONARY *)host->rrdcontexts, rrdcontext_insert_callback, (void *)host);
    dictionary_register_delete_callback((DICTIONARY *)host->rrdcontexts, rrdcontext_delete_callback, (void *)host);
    dictionary_register_conflict_callback((DICTIONARY *)host->rrdcontexts, rrdcontext_conflict_callback, (void *)host);
    dictionary_register_react_callback((DICTIONARY *)host->rrdcontexts, rrdcontext_react_callback, (void *)host);
}

void rrdhost_destroy_rrdcontexts(RRDHOST *host) {
    if(unlikely(!host)) return;
    if(unlikely(!host->rrdcontexts)) return;
    dictionary_destroy((DICTIONARY *)host->rrdcontexts);
    host->rrdcontexts = NULL;
}

static void rrdcontext_trigger_updates(RRDCONTEXT *rc) {
    if(unlikely(!(rc->flags & RRD_FLAG_UPDATED))) return;

    netdata_mutex_lock(&rc->mutex);
    rrd_flag_unset_updated(rc);

    size_t min_priority = LONG_MAX;
    RRD_FLAGS flags = RRD_FLAG_NONE;
    time_t min_first_time_t = LONG_MAX, max_last_time_t = 0;
    size_t instances_active = 0, instances_deleted = 0;
    {
        RRDINSTANCE *ri;
        dfe_start_write(rc->rrdinstances, ri) {
            // find the combined flags of all the metrics
            flags |= ri->flags & (RRD_FLAG_COLLECTED | RRD_FLAG_DELETED | RRD_FLAG_UPDATE_REASONS);

            if (unlikely(ri->flags & RRD_FLAG_DELETED)) {
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

            if(unlikely(flags & RRD_FLAG_LIVE_RETENTION)) {
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

        if(likely(flags & RRD_FLAG_COLLECTED)) {
            if(unlikely(!(rc->flags & RRD_FLAG_COLLECTED))) {
                rc->flags |= RRD_FLAG_COLLECTED;
                rc->flags &= ~RRD_FLAG_ARCHIVED;
                rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_STARTED_BEING_COLLECTED);
            }
        }
        else {
            if(unlikely(!(rc->flags & RRD_FLAG_ARCHIVED))) {
                rc->flags |= RRD_FLAG_ARCHIVED;
                rc->flags &= ~RRD_FLAG_COLLECTED;
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

        if(unlikely(rc->flags & RRD_FLAG_COLLECTED)) {
            rc->flags &= ~RRD_FLAG_COLLECTED;
            rc->flags |= RRD_FLAG_ARCHIVED;
            rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_STOPPED_BEING_COLLECTED);
        }
    }

    if(unlikely(rc->flags & RRD_FLAG_UPDATED)) {
        log_transition(NULL, NULL, rc->id, rc->flags, "RRDCONTEXT");
        check_if_we_need_to_emit_new_version(rc);
        rrd_flag_unset_updated(rc);
    }

    netdata_mutex_unlock(&rc->mutex);
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

// ----------------------------------------------------------------------------
// load from SQL

static void rrdinstance_load_clabel(SQL_CLABEL_DATA *sld, void *data) {
    RRDINSTANCE *ri = data;

    internal_error(log_calls, "RRDCONTEXT: adding label '%s':'%s' for instance '%s' of context '%s' from SQL for host '%s'", sld->label_key, sld->label_value, string2str(ri->id), string2str(ri->rc->id), ri->rc->rrdhost->hostname);

    rrdlabels_add(ri->rrdlabels, sld->label_key, sld->label_value, sld->label_source);
}

static void rrdinstance_load_dimension(SQL_DIMENSION_DATA *sd, void *data) {
    RRDINSTANCE *ri = data;

    internal_error(log_calls, "RRDCONTEXT: adding metric '%s' for instance '%s' of context '%s' from SQL for host '%s'", sd->id, string2str(ri->id), string2str(ri->rc->id), ri->rc->rrdhost->hostname);

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

    internal_error(log_calls, "RRDCONTEXT: adding context '%s' and chart '%s' from SQL for host '%s'", sc->context, sc->id, host->hostname);

    RRDCONTEXT tc = {
        .id = string_strdupz(sc->context),
        .title = string_strdupz(sc->title),
        .units = string_strdupz(sc->units),
        .priority = sc->priority,
        .chart_type = sc->chart_type,
        .flags = RRD_FLAG_ARCHIVED | RRD_FLAG_UPDATE_REASON_LOAD_SQL,
        .rrdhost = host,
    };

    RRDCONTEXT_ACQUIRED *rca = (RRDCONTEXT_ACQUIRED *)dictionary_set_and_acquire_item((DICTIONARY *)host->rrdcontexts, string2str(tc.id), &tc, sizeof(tc));
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

    internal_error(log_calls, "RRDCONTEXT: loading context '%s' from SQL for host '%s'", ctx_data->id, host->hostname);

    RRDCONTEXT tmp = {
        .id = string_strdupz(ctx_data->id),

        // no need to set more data here
        // we only need the hub data

        .hub = *ctx_data,
    };
    dictionary_set((DICTIONARY *)host->rrdcontexts, string2str(tmp.id), &tmp, sizeof(tmp));
}

void rrdhost_load_rrdcontext_data(RRDHOST *host) {
    if(host->rrdcontexts) return;

    internal_error(log_calls, "RRDCONTEXT: loading SQL data for host '%s'", host->hostname);

    rrdhost_create_rrdcontexts(host);
    ctx_get_context_list(&host->host_uuid, rrdcontext_load_context_callback, host);
    ctx_get_chart_list(&host->host_uuid, rrdinstance_load_chart_callback, host);

    internal_error(log_calls, "RRDCONTEXT: finished loading SQL data for host '%s'", host->hostname);
}

// ----------------------------------------------------------------------------
// the worker thread

// TODO - cleanup contexts that no longer have any retention
//
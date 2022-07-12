// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdcontext.h"
#include "sqlite/sqlite_context.h"

typedef struct rrdmetric RRDMETRIC;
typedef struct rrdinstance RRDINSTANCE;
typedef struct rrdcontext RRDCONTEXT;
typedef struct rrdmetric_dictionary RRDMETRICS;
typedef struct rrdinstances_dictionary RRDINSTANCES;

static inline const char *rrdcontext_acquired_name(RRDCONTEXT_ACQUIRED *rca);
static inline RRDINSTANCE *rrdinstance_acquired_value(RRDINSTANCE_ACQUIRED *ria);

static void rrdmetric_check_updates(RRDMETRIC *rm, bool rrdmetrics_is_write_locked);
static void rrdinstance_check_updates(RRDINSTANCE *ri, bool rrdinstances_is_write_locked, bool rrdmetrics_is_write_locked);
static void rrdcontext_check_updates(RRDCONTEXT *rc, bool rrdinstances_is_write_locked);

static void rrdcontext_release(RRDCONTEXT_ACQUIRED *rca);
static inline RRDCONTEXT *rrdcontext_acquired_value(RRDCONTEXT_ACQUIRED *rca);

typedef enum {
    RRD_FLAG_NONE      = 0,
    RRD_FLAG_DELETED   = (1 << 0),
    RRD_FLAG_COLLECTED = (1 << 1),
    RRD_FLAG_UPDATED   = (1 << 2),
    RRD_FLAG_ARCHIVED  = (1 << 3),
    RRD_FLAG_OWNLABELS = (1 << 4),
} RRD_FLAGS;

struct rrdmetric {
    uuid_t uuid;

    STRING *id;
    STRING *name;

    RRDDIM *rd;

    time_t first_time_t;
    time_t last_time_t;
    RRD_FLAGS flags;

    RRDINSTANCE *ri;
};

struct rrdinstance {
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

    size_t rrdlabels_version;           // the version of rrdlabels the last time we checked
    DICTIONARY *rrdlabels;

    RRDCONTEXT *rc;                     // acquired rrdcontext item, or NULL
    DICTIONARY *rrdmetrics;
};

struct rrdcontext {
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
};

static void rrdinstance_log(RRDINSTANCE *ri, const char *msg, bool rrdmetrics_is_write_locked) {
    char uuid[UUID_STR_LEN + 1];

    uuid_unparse(ri->uuid, uuid);

    BUFFER *wb = buffer_create(1000);

    buffer_sprintf(wb,
                   "RRDINSTANCE: %s id '%s' (host '%s'), uuid '%s', name '%s', context '%s', title '%s', units '%s', priority %zu, chart type '%s', update every %d, rrdset '%s', flags %s%s%s%s%s, labels version %zu, first_time_t %ld, last_time_t %ld",
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
                   ri->rrdlabels_version,
                   ri->first_time_t,
                   ri->last_time_t
                   );

    buffer_strcat(wb, ", labels: { ");
    if(ri->rrdlabels) {
        if(!rrdlabels_to_buffer(ri->rrdlabels, wb, "", "=", "'", ",", NULL, NULL, NULL, NULL))
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
    if(unlikely(!ria)) return;
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    dictionary_acquired_item_release(ri->rc->rrdinstances, (DICTIONARY_ITEM *)ria);
}

static inline RRDCONTEXT_ACQUIRED *rrdcontext_dup(RRDCONTEXT_ACQUIRED *rca) {
    return (RRDCONTEXT_ACQUIRED *)dictionary_acquired_item_dup((DICTIONARY_ITEM *)rca);
}

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

    if(rm->rd) {
        min_first_time_t = rrddim_first_entry_t(rm->rd);
        max_last_time_t = rrddim_last_entry_t(rm->rd);
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
        rm->flags |= RRD_FLAG_UPDATED;
    }

    if (max_last_time_t != rm->last_time_t) {
        rm->last_time_t = max_last_time_t;
        rm->flags |= RRD_FLAG_UPDATED;
    }

    if(rm->first_time_t == 0 && rm->last_time_t == 0)
        rm->flags |= RRD_FLAG_DELETED | RRD_FLAG_UPDATED;
}

// called when this rrdmetric is inserted to the rrdmetrics dictionary of a rrdinstance
static void rrdmetric_insert_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDINSTANCE *ri = data;
    RRDMETRIC *rm = value;

    // link it to its parent
    rm->ri = ri;

    // signal the react callback to do the job
    rm->flags |= RRD_FLAG_UPDATED;
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

    if(uuid_compare(rm->uuid, rm_new->uuid) != 0) {
        char uuid1[UUID_STR_LEN], uuid2[UUID_STR_LEN];
        uuid_unparse(rm->uuid, uuid1);
        uuid_unparse(rm_new->uuid, uuid2);
        internal_error(true, "RRDMETRIC: '%s' of instance '%s' changed uuid from '%s' to '%s'", string2str(rm->id), string2str(ri->id), uuid1, uuid2);
        uuid_copy(rm->uuid, rm_new->uuid);
        rm->flags |= RRD_FLAG_UPDATED;
    }

    if(rm->id != rm_new->id)
        fatal("RRDMETRIC: '%s' cannot change id to '%s'", string2str(rm->id), string2str(rm_new->id));

    if(rm->name != rm_new->name) {
        STRING *old = rm->name;
        rm->name = string_dup(rm_new->name);
        string_freez(old);
        rm->flags |= RRD_FLAG_UPDATED;
    }

    if(!rm->first_time_t || (rm_new->first_time_t && rm_new->first_time_t < rm->first_time_t)) {
        rm->first_time_t = rm_new->first_time_t;
        rm->flags |= RRD_FLAG_UPDATED;
    }

    if(!rm->last_time_t || (rm_new->last_time_t && rm_new->last_time_t > rm->last_time_t)) {
        rm->last_time_t = rm_new->last_time_t;
        rm->flags |= RRD_FLAG_UPDATED;
    }

    rm->flags |= rm_new->flags;

    if(rm->flags & RRD_FLAG_DELETED) {
        rm->flags &= ~RRD_FLAG_DELETED;
        rm->flags |= RRD_FLAG_UPDATED;
    }

    if((rm->flags & RRD_FLAG_COLLECTED) && (rm->flags & RRD_FLAG_ARCHIVED))
        rm->flags &= ~RRD_FLAG_ARCHIVED;

    if(!rm->rd && rm_new->rd) {
        rm->rd = rm_new->rd;
        rm->flags |= RRD_FLAG_UPDATED;
    }

    rrdmetric_free(rm_new);

    // the react callback will continue from here
}

static void rrdmetric_react_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDINSTANCE *ri = data; (void)ri;
    RRDMETRIC *rm = value;

    rrdmetric_check_updates(rm, true);
}

static void rrdmetric_check_updates(RRDMETRIC *rm, bool rrdmetrics_is_write_locked) {
    if(!(rm->flags & RRD_FLAG_UPDATED)) return;

    rrdmetric_update_retention(rm);

    if(unlikely(rm->flags & RRD_FLAG_UPDATED)) {
        rm->ri->flags |= RRD_FLAG_UPDATED;
        rrdinstance_check_updates(rm->ri, false, rrdmetrics_is_write_locked);
        rm->flags &= ~RRD_FLAG_UPDATED;
    }
}

static inline RRDMETRIC *rrdmetric_acquired_value(RRDMETRIC_ACQUIRED *rma) {
    return dictionary_acquired_item_value((DICTIONARY_ITEM *)rma);
}

static inline void rrdmetric_release(RRDMETRIC_ACQUIRED *rma) {
    if(unlikely(!rma)) return;

    RRDMETRIC *rm = rrdmetric_acquired_value(rma);
    dictionary_acquired_item_release(rm->ri->rrdmetrics, (DICTIONARY_ITEM *)rma);
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

    RRDMETRIC tmp = {
        .id = string_strdupz(rd->id),
        .name = string_strdupz(rd->name),
        .flags = RRD_FLAG_COLLECTED,
        .rd = rd,
    };
    uuid_copy(tmp.uuid, rd->metric_uuid);

    RRDMETRIC_ACQUIRED *rma = (RRDMETRIC_ACQUIRED *)dictionary_set_and_acquire_item((DICTIONARY *)ri->rrdmetrics, string2str(tmp.id), &tmp, sizeof(tmp));

    if(rd->rrdmetric && rd->rrdmetric != rma)
        fatal("RRDMETRIC: dimension '%s' of chart '%s' changed rrdmetric!", rd->id, rd->rrdset->id);
    else if(!rd->rrdmetric)
        rd->rrdmetric = rma;
}

static inline void rrdmetric_rrddim_is_freed(RRDDIM *rd) {
    if(unlikely(!rd->rrdmetric))
        fatal("RRDINSTANCE: dimension '%s' is not linked to an RRDMETRIC", rd->id);

    RRDMETRIC *rm = rrdmetric_acquired_value(rd->rrdmetric);

    if(unlikely(rm->rd != rd))
        fatal("RRDMETRIC: '%s' is not linked to dimension '%s'", string2str(rm->id), rd->id);

    rm->flags |= RRD_FLAG_ARCHIVED | RRD_FLAG_UPDATED;

    if(rm->flags & RRD_FLAG_COLLECTED)
        rm->flags &= ~RRD_FLAG_COLLECTED;

    rm->rd = NULL;
    rrdmetric_check_updates(rm, false);
    rrdmetric_release(rd->rrdmetric);
    rd->rrdmetric = NULL;
}

static inline void rrdmetric_collected_rrddim(RRDDIM *rd) {
    if(unlikely(!rd->rrdmetric))
        fatal("RRDMETRIC: rrddim '%s' is not linked to a rrdmetric", rd->id);

    RRDMETRIC *rm = rrdmetric_acquired_value(rd->rrdmetric);
    if(unlikely(!(rm->flags & RRD_FLAG_COLLECTED)))
        rm->flags |= RRD_FLAG_COLLECTED|RRD_FLAG_UPDATED;

    if(unlikely(rm->flags & RRD_FLAG_ARCHIVED)) {
        rm->flags &= ~RRD_FLAG_ARCHIVED;
        rm->flags |= RRD_FLAG_UPDATED;
    }

    rrdmetric_check_updates(rm, false);
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
    ri->rrdlabels_version = dictionary_stats_version(ri->rrdlabels);

    rrdmetrics_create(ri);

    // rrdinstance_log(ri, "INSERT", false);

    // signal the react callback to do the job
    ri->flags |= RRD_FLAG_UPDATED;
}

static void rrdinstance_delete_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDCONTEXT *rc = data; (void)rc;
    RRDINSTANCE *ri = (RRDINSTANCE *)value;

    // rrdinstance_log(ri, "DELETE", false);
    rrdinstance_free(ri);
}

static void rrdinstance_conflict_callback(const char *id, void *oldv, void *newv, void *data) {
    (void)id;
    RRDCONTEXT *rc      = data; (void)rc;
    RRDINSTANCE *ri     = (RRDINSTANCE *)oldv;
    RRDINSTANCE *ri_new = (RRDINSTANCE *)newv;

    rrdinstance_check(ri_new);

    if(uuid_compare(ri->uuid, ri_new->uuid) != 0) {
        uuid_copy(ri->uuid, ri_new->uuid);
        ri->flags |= RRD_FLAG_UPDATED;
    }

    if(ri->id != ri_new->id)
        fatal("RRDINSTANCE: '%s' cannot change id to '%s'", string2str(ri->id), string2str(ri_new->id));

    if(ri->name != ri_new->name) {
        STRING *old = ri->name;
        ri->name = string_dup(ri_new->name);
        string_freez(old);
        ri->flags |= RRD_FLAG_UPDATED;
    }

    if(ri->title != ri_new->title) {
        STRING *old = ri->title;
        ri->title = string_dup(ri_new->title);
        string_freez(old);
        ri->flags |= RRD_FLAG_UPDATED;
    }

    if(ri->units != ri_new->units) {
        STRING *old = ri->units;
        ri->units = string_dup(ri_new->units);
        string_freez(old);
        ri->flags |= RRD_FLAG_UPDATED;
    }

    if(ri->chart_type != ri_new->chart_type) {
        ri->chart_type = ri_new->chart_type;
        ri->flags |= RRD_FLAG_UPDATED;
    }

    if(ri->priority != ri_new->priority) {
        ri->priority = ri_new->priority;
        ri->flags |= RRD_FLAG_UPDATED;
    }

    if(ri->update_every != ri_new->update_every) {
        ri->update_every = ri_new->update_every;
        ri->flags |= RRD_FLAG_UPDATED;
    }

    if(ri->rrdset != ri_new->rrdset) {

        if(ri->rrdset)
            fatal("RRDINSTANCE: '%s' changed rrdset from '%s' to '%s'!", string2str(ri->id), ri->rrdset->id, ri_new->rrdset->id);

        ri->rrdset = ri_new->rrdset;

        if(ri->flags & RRD_FLAG_OWNLABELS) {
            DICTIONARY *old = ri->rrdlabels;
            ri->rrdlabels = ri->rrdset->state->chart_labels;
            ri->flags &= ~RRD_FLAG_OWNLABELS;
            rrdlabels_destroy(old);
        }

        ri->flags |= RRD_FLAG_UPDATED;
    }

    // rrdinstance_log(ri, "CONFLICT", false);

    // free the new one
    rrdinstance_free(ri_new);

    // the react callback will continue from here
}

static void rrdinstance_react_callback(const char *id, void *value, void *data) {
    (void)id;
    RRDCONTEXT *rc = data; (void)rc;
    RRDINSTANCE *ri = value;

    rrdinstance_check_updates(ri, true, false);
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

static void rrdinstance_check_updates(RRDINSTANCE *ri, bool rrdinstances_is_write_locked, bool rrdmetrics_is_write_locked) {
    if(unlikely(ri->rrdlabels_version != dictionary_stats_version(ri->rrdlabels))) {
        ri->rrdlabels_version = dictionary_stats_version(ri->rrdlabels);
        ri->flags |= RRD_FLAG_UPDATED;
    }

    if(unlikely(!(ri->flags & RRD_FLAG_UPDATED)))
        return;

    RRD_FLAGS flags = RRD_FLAG_NONE;
    time_t min_first_time_t = LONG_MAX, max_last_time_t = 0;
    size_t metrics_active = 0, metrics_deleted = 0;
    {
        RRDMETRIC *rm;
        dfe_start_rw(
            (DICTIONARY *)ri->rrdmetrics, rm, rrdmetrics_is_write_locked ? DICTIONARY_LOCK_NONE : DICTIONARY_LOCK_READ)
        {
            if (rm->flags & RRD_FLAG_DELETED) {
                metrics_deleted++;
                continue;
            }

            metrics_active++;

            if (rm->flags & RRD_FLAG_COLLECTED)
                flags |= RRD_FLAG_COLLECTED;

            if (rm->first_time_t == 0 || rm->last_time_t == 0)
                continue;

            if (rm->first_time_t < min_first_time_t)
                min_first_time_t = rm->first_time_t;

            if (rm->last_time_t > max_last_time_t)
                max_last_time_t = rm->last_time_t;
        }
        dfe_done(rm);
    }

    if(metrics_active || metrics_deleted) {
        if (min_first_time_t == LONG_MAX)
            min_first_time_t = 0;

        ri->flags &= ~RRD_FLAG_DELETED;
        if (min_first_time_t == 0 || max_last_time_t == 0) {
            ri->first_time_t = 0;
            ri->last_time_t = 0;
            ri->flags |= RRD_FLAG_DELETED | RRD_FLAG_UPDATED;

            // TODO this instance does not have any retention

        }
        else {
            if (ri->first_time_t != min_first_time_t) {
                ri->first_time_t = min_first_time_t;
                ri->flags |= RRD_FLAG_UPDATED;
            }

            if (ri->last_time_t != max_last_time_t) {
                ri->last_time_t = max_last_time_t;
                ri->flags |= RRD_FLAG_UPDATED;
            }
        }

        if(flags & RRD_FLAG_COLLECTED) {
            ri->flags |= RRD_FLAG_COLLECTED | RRD_FLAG_UPDATED;

            if(ri->flags & RRD_FLAG_ARCHIVED)
                ri->flags &= ~RRD_FLAG_ARCHIVED;
        }
        else {
            ri->flags |= RRD_FLAG_ARCHIVED | RRD_FLAG_UPDATED;

            if(ri->flags & RRD_FLAG_COLLECTED)
                ri->flags &= ~RRD_FLAG_COLLECTED;
        }

    }
    else {
        if(ri->flags & RRD_FLAG_COLLECTED)
            ri->flags &= ~RRD_FLAG_COLLECTED;
    }

    if(ri->flags & RRD_FLAG_UPDATED) {
        ri->rc->flags |= RRD_FLAG_UPDATED;
        rrdcontext_check_updates(ri->rc, rrdinstances_is_write_locked);
        ri->flags &= ~RRD_FLAG_UPDATED;
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

    RRDINSTANCE ti = {
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
    uuid_copy(ti.uuid, *st->chart_uuid);

    RRDINSTANCE_ACQUIRED *ria = (RRDINSTANCE_ACQUIRED *)dictionary_set_and_acquire_item(rc->rrdinstances, string2str(ti.id), &ti, sizeof(ti));

    if(st->rrdinstance && st->rrdinstance != ria)
        fatal("RRDINSTANCE: chart '%s' changed rrdinstance.", st->id);

    st->rrdinstance = ria;

    if(st->rrdcontext && st->rrdcontext != rca) {
        // the chart changed context
        RRDCONTEXT *rcold = rrdcontext_acquired_value(st->rrdcontext);
        dictionary_del(rcold->rrdinstances, st->id);
        rcold->flags |= RRD_FLAG_UPDATED;
        rrdcontext_check_updates(rcold, false);
    }

    st->rrdcontext = rca;
}

static inline void rrdinstance_rrdset_is_freed(RRDSET *st) {
    if(unlikely(!st->rrdinstance))
        fatal("RRDINSTANCE: chart '%s' is not linked to an RRDINSTANCE", st->id);

    RRDINSTANCE *ri = rrdinstance_acquired_value(st->rrdinstance);

    if(unlikely(ri->rrdset != st))
        fatal("RRDINSTANCE: instance '%s' is not linked to chart '%s'", string2str(ri->id), st->id);

    ri->flags |= RRD_FLAG_ARCHIVED | RRD_FLAG_UPDATED;

    if(ri->flags & RRD_FLAG_COLLECTED)
        ri->flags &= ~RRD_FLAG_COLLECTED;

    if(!(ri->flags & RRD_FLAG_OWNLABELS)) {
        ri->flags &= ~RRD_FLAG_OWNLABELS;
        ri->rrdlabels = rrdlabels_create();
        rrdlabels_copy(ri->rrdlabels, st->state->chart_labels);
    }
    ri->rrdlabels_version = dictionary_stats_version(ri->rrdlabels);

    rrdinstance_check_updates(ri, false, false);
    rrdinstance_release(st->rrdinstance);
    st->rrdinstance = NULL;

    rrdcontext_release(st->rrdcontext);
    st->rrdcontext = NULL;
}

static inline void rrdinstance_updated_rrdset_name(RRDSET *st) {
    (void)st;
    ;
}

static inline void rrdinstance_updated_rrdset_flags(RRDSET *st) {
    (void)st;
    ;
}

static inline void rrdinstance_collected_rrdset(RRDSET *st) {
    if(unlikely(!st->rrdinstance))
        fatal("RRDINSTANCE: rrdset '%s' is not linked to an rrdinstance", st->id);

    RRDINSTANCE *ri = rrdinstance_acquired_value(st->rrdinstance);
    if(unlikely(!(ri->flags & RRD_FLAG_COLLECTED)))
        ri->flags |= RRD_FLAG_COLLECTED|RRD_FLAG_UPDATED;

    if(unlikely(ri->flags & RRD_FLAG_ARCHIVED)) {
        ri->flags &= ~RRD_FLAG_ARCHIVED;
        ri->flags |= RRD_FLAG_UPDATED;
    }

    rrdinstance_check_updates(ri, false, false);
}

// ----------------------------------------------------------------------------
// RRDCONTEXT

static void rrdcontext_freez(RRDCONTEXT *rc) {
    rrdinstances_destroy(rc);
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
    rc->flags |= RRD_FLAG_UPDATED;
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
        rc->flags |= RRD_FLAG_UPDATED;
    }

    if(rc->units != rc_new->units) {
        STRING *old_units = rc->units;
        rc->units = string_dup(rc_new->units);
        string_freez(old_units);
        rc->flags |= RRD_FLAG_UPDATED;
    }

    if(rc->chart_type != rc_new->chart_type) {
        rc->chart_type = rc_new->chart_type;
        rc->flags |= RRD_FLAG_UPDATED;
    }

    if(rc->priority != rc_new->priority) {
        rc->priority = rc_new->priority;
        rc->flags |= RRD_FLAG_UPDATED;
    }

    //internal_error(true, "RRDCONTEXT: UPDATE '%s' on host '%s', version %lu, title '%s', units '%s', chart type '%s', priority %zu, first_time_t %ld, last_time_t %ld, options %s%s%s",
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

    rrdcontext_check_updates(rc, false);
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

static void rrdcontext_check_updates(RRDCONTEXT *rc, bool rrdinstances_is_write_locked) {
    if(unlikely(!(rc->flags & RRD_FLAG_UPDATED)))
        return;

    size_t min_priority = LONG_MAX;
    RRD_FLAGS flags = RRD_FLAG_NONE;
    time_t min_first_time_t = LONG_MAX, max_last_time_t = 0;
    size_t instances_active = 0, instances_deleted = 0;
    {
        RRDINSTANCE *ri;
        dfe_start_rw(rc->rrdinstances, ri, rrdinstances_is_write_locked ? DICTIONARY_LOCK_NONE : DICTIONARY_LOCK_READ)
        {
            if (ri->flags & RRD_FLAG_DELETED) {
                instances_deleted++;
                continue;
            }

            instances_active++;

            // TODO check for different chart types
            // TODO check for different units

            if (ri->flags & RRD_FLAG_COLLECTED)
                flags |= RRD_FLAG_COLLECTED;

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

    if(instances_active || instances_deleted) {
        if (min_first_time_t == LONG_MAX)
            min_first_time_t = 0;

        rc->flags &= ~RRD_FLAG_DELETED;
        if (min_first_time_t == 0 && max_last_time_t == 0) {
            rc->first_time_t = 0;
            rc->last_time_t = 0;
            rc->flags |= RRD_FLAG_DELETED | RRD_FLAG_UPDATED;

            // TODO this context does not have any retention

        }
        else {
            if (rc->first_time_t != min_first_time_t) {
                rc->first_time_t = min_first_time_t;
                rc->flags |= RRD_FLAG_UPDATED;
            }

            if (rc->last_time_t != max_last_time_t) {
                rc->last_time_t = max_last_time_t;
                rc->flags |= RRD_FLAG_UPDATED;
            }
        }

        if(flags & RRD_FLAG_COLLECTED) {
            rc->flags |= RRD_FLAG_COLLECTED | RRD_FLAG_UPDATED;

            if(rc->flags & RRD_FLAG_ARCHIVED)
                rc->flags &= ~RRD_FLAG_ARCHIVED;
        }
        else {
            rc->flags |= RRD_FLAG_ARCHIVED | RRD_FLAG_UPDATED;

            if(rc->flags & RRD_FLAG_COLLECTED)
                rc->flags &= ~RRD_FLAG_COLLECTED;

        }

        if (min_priority != LONG_MAX && rc->priority != min_priority) {
            rc->priority = min_priority;
            rc->flags |= RRD_FLAG_UPDATED;
        }
    }
    else {
        if(rc->flags & RRD_FLAG_COLLECTED)
            rc->flags &= ~RRD_FLAG_COLLECTED;
    }

    check_if_we_need_to_emit_new_version(rc);
    rc->flags &= ~RRD_FLAG_UPDATED;
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
    if(unlikely(!rca)) return;

    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);
    dictionary_acquired_item_release((DICTIONARY *)rc->rrdhost->rrdcontexts, (DICTIONARY_ITEM *)rca);
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
    (void)rd;
    ;
}

void rrdcontext_updated_rrddim_multiplier(RRDDIM *rd) {
    (void)rd;
    ;
}

void rrdcontext_updated_rrddim_divisor(RRDDIM *rd) {
    (void)rd;
    ;
}

void rrdcontext_updated_rrddim_flags(RRDDIM *rd) {
    (void)rd;
    ;
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
    rrdlabels_add(ri->rrdlabels, sld->label_key, sld->label_value, sld->label_source);
}

static void rrdinstance_load_dimension(SQL_DIMENSION_DATA *sd, void *data) {
    RRDINSTANCE *ri = data;

    RRDMETRIC tmp = {
        .id = string_strdupz(sd->id),
        .name = string_strdupz(sd->name),
        .flags = RRD_FLAG_ARCHIVED,
    };
    uuid_copy(tmp.uuid, sd->dim_id);

    dictionary_set((DICTIONARY *)ri->rrdmetrics, string2str(tmp.id), &tmp, sizeof(tmp));
}

static void rrdinstance_load_chart_callback(SQL_CHART_DATA *sc, void *data) {
    RRDHOST *host = data;

    RRDCONTEXT tc = {
        .id = string_strdupz(sc->context),
        .title = string_strdupz(sc->title),
        .units = string_strdupz(sc->units),
        .priority = sc->priority,
        .chart_type = sc->chart_type,
        .flags = RRD_FLAG_ARCHIVED,
        .rrdhost = host,
    };

    RRDCONTEXT_ACQUIRED *rca = (RRDCONTEXT_ACQUIRED *)dictionary_set_and_acquire_item((DICTIONARY *)host->rrdcontexts, string2str(tc.id), &tc, sizeof(tc));
    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);

    RRDINSTANCE ti = {
        .id = string_strdupz(sc->id),
        .name = string_strdupz(sc->name),
        .title = string_strdupz(sc->title),
        .units = string_strdupz(sc->units),
        .chart_type = sc->chart_type,
        .priority = sc->priority,
        .update_every = sc->update_every,
        .flags = RRD_FLAG_ARCHIVED,
    };
    uuid_copy(ti.uuid, sc->chart_id);

    RRDINSTANCE_ACQUIRED *ria = (RRDINSTANCE_ACQUIRED *)dictionary_set_and_acquire_item(rc->rrdinstances, sc->id, &ti, sizeof(ti));
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
    dictionary_set((DICTIONARY *)host->rrdcontexts, string2str(tmp.id), &tmp, sizeof(tmp));
}

void rrdhost_load_rrdcontext_data(RRDHOST *host) {
    if(host->rrdcontexts) return;

    rrdhost_create_rrdcontexts(host);

    ctx_get_context_list(&host->host_uuid, rrdcontext_load_context_callback, host);
    ctx_get_chart_list(&host->host_uuid, rrdinstance_load_chart_callback, host);
}

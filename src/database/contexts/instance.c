// SPDX-License-Identifier: GPL-3.0-or-later

#include "internal.h"

// ----------------------------------------------------------------------------
// helper one-liners for RRDINSTANCE

bool rrdinstance_acquired_id_and_name_are_same(RRDINSTANCE_ACQUIRED *ria) {
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    return ri->id == ri->name;
}

inline const char *rrdinstance_acquired_id(RRDINSTANCE_ACQUIRED *ria) {
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    return string2str(ri->id);
}

inline const char *rrdinstance_acquired_name(RRDINSTANCE_ACQUIRED *ria) {
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    return string2str(ri->name);
}

inline bool rrdinstance_acquired_has_name(RRDINSTANCE_ACQUIRED *ria) {
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    return (ri->name && ri->name != ri->id);
}

inline const char *rrdinstance_acquired_units(RRDINSTANCE_ACQUIRED *ria) {
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    return string2str(ri->units);
}

inline STRING *rrdinstance_acquired_units_dup(RRDINSTANCE_ACQUIRED *ria) {
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    return string_dup(ri->units);
}

inline RRDLABELS *rrdinstance_labels(RRDINSTANCE *ri) {
    if(ri->rrdset)
        return ri->rrdset->rrdlabels;

    if (rrd_flag_check_all(ri, RRD_FLAG_OWN_LABELS | RRD_FLAG_DEMAND_LABELS)) {
        rrd_flag_clear(ri, RRD_FLAG_DEMAND_LABELS);
        load_instance_labels_on_demand(uuidmap_uuid_ptr(ri->uuid), ri);
        rrdinstance_trigger_updates(ri, __FUNCTION__ );
    }

    return ri->rrdlabels;
}

inline RRDLABELS *rrdinstance_acquired_labels(RRDINSTANCE_ACQUIRED *ria) {
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    return rrdinstance_labels(ri);
}

inline DICTIONARY *rrdinstance_acquired_functions(RRDINSTANCE_ACQUIRED *ria) {
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    if(!ri->rrdset) return NULL;
    return ri->rrdset->functions_view;
}

inline RRDHOST *rrdinstance_acquired_rrdhost(RRDINSTANCE_ACQUIRED *ria) {
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    return ri->rc->rrdhost;
}

inline bool rrdinstance_acquired_belongs_to_context(RRDINSTANCE_ACQUIRED *ria, RRDCONTEXT_ACQUIRED *rca) {
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);
    return ri->rc == rc;
}

inline time_t rrdinstance_acquired_update_every(RRDINSTANCE_ACQUIRED *ria) {
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    return ri->update_every_s;
}

// ----------------------------------------------------------------------------
// RRDINSTANCE

static void rrdinstance_free(RRDINSTANCE *ri) {

    if(rrd_flag_check(ri, RRD_FLAG_OWN_LABELS))
        rrdlabels_destroy(ri->rrdlabels);

    rrdmetrics_destroy_from_rrdinstance(ri);
    string_freez(ri->id);
    string_freez(ri->name);
    string_freez(ri->title);
    string_freez(ri->units);
    string_freez(ri->family);
    uuidmap_free(ri->uuid);

    ri->id = NULL;
    ri->name = NULL;
    ri->title = NULL;
    ri->units = NULL;
    ri->family = NULL;
    ri->uuid = 0;

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

    ri->rrdlabels = rrdlabels_create();

    if(ri->rrdset)
        ri->flags &= ~(RRD_FLAG_OWN_LABELS| RRD_FLAG_DEMAND_LABELS); // no need of atomics at the constructor
    else
        ri->flags |= (RRD_FLAG_OWN_LABELS | RRD_FLAG_DEMAND_LABELS); // no need of atomics at the constructor

    if(ri->rrdset) {
        if(unlikely(rrdset_flag_check(ri->rrdset, RRDSET_FLAG_HIDDEN)))
            ri->flags |= RRD_FLAG_HIDDEN; // no need of atomics at the constructor
        else
            ri->flags &= ~RRD_FLAG_HIDDEN; // no need of atomics at the constructor
    }

    rrdmetrics_create_in_rrdinstance(ri);

    // update the count of instances
    __atomic_add_fetch(&ri->rc->rrdhost->rrdctx.instances_count, 1, __ATOMIC_RELAXED);

    // signal the react callback to do the job
    rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_NEW_OBJECT);
}

static void rrdinstance_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *rrdcontext __maybe_unused) {
    RRDINSTANCE *ri = (RRDINSTANCE *)value;

    internal_error(ri->rrdset, "RRDINSTANCE: '%s' is freed but there is a RRDSET linked to it.", string2str(ri->id));

    // update the count of instances
    __atomic_sub_fetch(&ri->rc->rrdhost->rrdctx.instances_count, 1, __ATOMIC_RELAXED);

    rrdinstance_free(ri);
}

static bool rrdinstance_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *rrdcontext __maybe_unused) {
    RRDINSTANCE *ri     = (RRDINSTANCE *)old_value;
    RRDINSTANCE *ri_new = (RRDINSTANCE *)new_value;

    internal_error(ri->id != ri_new->id,
                   "RRDINSTANCE: '%s' cannot change id to '%s'",
                   string2str(ri->id), string2str(ri_new->id));

    if(ri->uuid != ri_new->uuid) {
#ifdef NETDATA_INTERNAL_CHECKS
        char uuid1[UUID_STR_LEN], uuid2[UUID_STR_LEN];
        uuid_unparse(*uuidmap_uuid_ptr(ri->uuid), uuid1);
        uuid_unparse(*uuidmap_uuid_ptr(ri_new->uuid), uuid2);
        internal_error(true, "RRDINSTANCE: '%s' of host '%s' changed UUID from '%s' to '%s'",
                       string2str(ri->id), rrdhost_hostname(ri->rc->rrdhost), uuid1, uuid2);
#endif

        SWAP(ri->uuid, ri_new->uuid);
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
    }

    if(ri->name != ri_new->name) {
        SWAP(ri->name, ri_new->name);
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
    }

    if(ri->title != ri_new->title) {
        SWAP(ri->title, ri_new->title);
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
    }

    if(ri->units != ri_new->units) {
        SWAP(ri->units, ri_new->units);
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
    }

    if(ri->family != ri_new->family) {
        SWAP(ri->family, ri_new->family);
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

    if(ri_new->rrdset && ri->rrdset != ri_new->rrdset) {
        ri->rrdset = ri_new->rrdset;
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_LINKING);
    }

    if(ri->rrdset) {
        // our labels are not needed any more
        rrdlabels_flush(ri->rrdlabels);

        if(unlikely(rrdset_flag_check(ri->rrdset, RRDSET_FLAG_HIDDEN)))
            rrd_flag_set(ri, RRD_FLAG_HIDDEN);
        else
            rrd_flag_clear(ri, RRD_FLAG_HIDDEN);
    }

    rrd_flag_set(ri, ri_new->flags & RRD_FLAGS_ALLOWED_EXTERNALLY_ON_NEW_OBJECTS); // no need for atomics on ri_new

    if(rrd_flag_is_collected(ri) && rrd_flag_is_archived(ri))
        rrdinstance_set_collected(ri);

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

void rrdinstance_trigger_updates(RRDINSTANCE *ri, const char *function) {
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

        rrdinstance_set_archived(ri);
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_LINKING);
    }

    if(rrd_flag_is_updated(ri) || !rrd_flag_check(ri, RRD_FLAG_LIVE_RETENTION)) {
        rrd_flag_set_updated(ri->rc, RRD_FLAG_UPDATE_REASON_TRIGGERED);
        rrdcontext_queue_for_post_processing(ri->rc, function, ri->flags);
    }
}

// ----------------------------------------------------------------------------
// RRDINSTANCE HOOKS ON RRDSET

inline void rrdinstance_from_rrdset(RRDSET *st) {
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

    RRDCONTEXT_ACQUIRED *rca = (RRDCONTEXT_ACQUIRED *)dictionary_set_and_acquire_item(st->rrdhost->rrdctx.contexts, string2str(trc.id), &trc, sizeof(trc));
    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);

    RRDINSTANCE tri = {
        .uuid = uuidmap_create(st->chart_uuid),
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

    RRDINSTANCE_ACQUIRED *ria = (RRDINSTANCE_ACQUIRED *)dictionary_set_and_acquire_item(rc->rrdinstances, string2str(tri.id), &tri, sizeof(tri));

    RRDCONTEXT_ACQUIRED *rca_old = st->rrdcontexts.rrdcontext;
    RRDINSTANCE_ACQUIRED *ria_old = st->rrdcontexts.rrdinstance;

    st->rrdcontexts.rrdcontext = rca;
    st->rrdcontexts.rrdinstance = ria;

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
                    if (!rd->rrdcontexts.rrdmetric) continue;

                    RRDMETRIC *rm_old = rrdmetric_acquired_value(rd->rrdcontexts.rrdmetric);
                    rrdmetric_set_deleted_overwrite(rm_old, RRD_FLAG_UPDATED|RRD_FLAG_LIVE_RETENTION|RRD_FLAG_UPDATE_REASON_UNUSED|RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);
                    rm_old->rrddim = NULL;
                    rm_old->first_time_s = 0;
                    rm_old->last_time_s = 0;

                    rrdmetric_release(rd->rrdcontexts.rrdmetric);
                    rd->rrdcontexts.rrdmetric = NULL;

                    rrdmetric_from_rrddim(rd);
                }
        rrddim_foreach_done(rd);

        // mark the old instance, ready to be deleted
        if(!rrd_flag_check(ri_old, RRD_FLAG_OWN_LABELS))
            ri_old->rrdlabels = rrdlabels_create();

        rrdinstance_set_deleted_overwrite(ri_old, RRD_FLAG_UPDATED|RRD_FLAG_OWN_LABELS|RRD_FLAG_LIVE_RETENTION|RRD_FLAG_UPDATE_REASON_UNUSED|RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);
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

#define rrdset_get_rrdinstance(st) rrdset_get_rrdinstance_with_trace(st, __FUNCTION__)
static inline RRDINSTANCE *rrdset_get_rrdinstance_with_trace(RRDSET *st, const char *function) {
    if(unlikely(!st->rrdcontexts.rrdinstance)) {
        netdata_log_error("RRDINSTANCE: RRDSET '%s' is not linked to an RRDINSTANCE at %s()", rrdset_id(st), function);
        return NULL;
    }

    RRDINSTANCE *ri = rrdinstance_acquired_value(st->rrdcontexts.rrdinstance);
    if(unlikely(!ri)) {
        netdata_log_error("RRDINSTANCE: RRDSET '%s' lost its link to an RRDINSTANCE at %s()", rrdset_id(st), function);
        return NULL;
    }

    if(unlikely(ri->rrdset != st))
        fatal("RRDINSTANCE: '%s' is not linked to RRDSET '%s' at %s()", string2str(ri->id), rrdset_id(st), function);

    return ri;
}

ALWAYS_INLINE void rrdinstance_rrdset_not_collected(RRDSET *st) {
    st->rrdcontexts.collected = false;

    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        rrdmetric_not_collected_rrddim(rd);
    }
    rrddim_foreach_done(rd);
}

inline void rrdinstance_rrdset_is_freed(RRDSET *st) {
    rrdinstance_rrdset_not_collected(st);

    RRDINSTANCE *ri = rrdset_get_rrdinstance(st);
    if(unlikely(!ri)) return;

    rrdinstance_set_archived(ri);

    SWAP(ri->rrdlabels, st->rrdlabels);
    rrd_flag_set(ri, RRD_FLAG_OWN_LABELS);

    ri->rrdset = NULL;

    rrdinstance_trigger_updates(ri, __FUNCTION__ );

    rrdinstance_release(st->rrdcontexts.rrdinstance);
    st->rrdcontexts.rrdinstance = NULL;

    rrdcontext_release(st->rrdcontexts.rrdcontext);
    st->rrdcontexts.rrdcontext = NULL;
}

ALWAYS_INLINE void rrdinstance_rrdset_has_updated_retention(RRDSET *st) {
    // rrdinstance_rrdset_not_collected(st);

    RRDINSTANCE *ri = rrdset_get_rrdinstance(st);
    if(unlikely(!ri)) return;

    rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_UPDATE_RETENTION);
    rrdinstance_trigger_updates(ri, __FUNCTION__ );
}

inline void rrdinstance_updated_rrdset_name(RRDSET *st) {
    rrdinstance_rrdset_not_collected(st);

    // the chart may not be initialized when this is called
    if(unlikely(!st->rrdcontexts.rrdinstance)) return;

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

inline void rrdinstance_updated_rrdset_flags_no_action(RRDINSTANCE *ri, RRDSET *st) {
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

inline void rrdinstance_updated_rrdset_flags(RRDSET *st) {
    rrdinstance_rrdset_not_collected(st);

    RRDINSTANCE *ri = rrdset_get_rrdinstance(st);
    if(unlikely(!ri)) return;

    if(unlikely(rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE)))
        rrdinstance_set_archived(ri);

    rrdinstance_updated_rrdset_flags_no_action(ri, st);

    rrdinstance_trigger_updates(ri, __FUNCTION__ );
}

ALWAYS_INLINE void rrdinstance_collected_rrdset(RRDSET *st) {
    if(st->rrdcontexts.collected)
        return;

    st->rrdcontexts.collected = true;

    RRDINSTANCE *ri = rrdset_get_rrdinstance(st);
    if(unlikely(!ri)) {
        rrdcontext_updated_rrdset(st);
        ri = rrdset_get_rrdinstance(st);
        if(unlikely(!ri))
            return;
    }

    rrdinstance_updated_rrdset_flags_no_action(ri, st);

    if(unlikely(ri->internal.collected_metrics_count && !rrd_flag_is_collected(ri)))
        rrdinstance_set_collected(ri);

    // we use this variable to detect BEGIN/END without SET
    ri->internal.collected_metrics_count = 0;

    rrdinstance_trigger_updates(ri, __FUNCTION__ );
}

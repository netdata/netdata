// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdcontext-internal.h"

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

// An instance's labels are its chart's while it is linked to one, and its own once archived.
// The chart's labels are destroyed with the chart, so they are handed out with a reference on it.
// The instance's own set lives as long as the instance, which the caller holds: it is created with the
// instance and destroyed with it, never swapped, only refilled.
RRDINSTANCE_LABELS_REF rrdinstance_acquired_labels_acquire(RRDINSTANCE_ACQUIRED *ria) {
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    RRDINSTANCE_LABELS_REF ref = { .rsa = NULL, .labels = NULL };

    ref.rsa = rrdinstance_rrdset_acquire(ria);
    if(ref.rsa) {
        ref.labels = rrdset_acquired_to_rrdset(ref.rsa)->rrdlabels;
        return ref;
    }

    // still linked, but the chart is going away right now: the instance's own set has not been
    // refilled from it yet, so there are no labels to give
    if(rrdinstance_rrdset_atomic_load(ri))
        return ref;

    if (rrd_flag_check_all(ri, RRD_FLAG_OWN_LABELS | RRD_FLAG_DEMAND_LABELS)) {
        rrd_flag_clear(ri, RRD_FLAG_DEMAND_LABELS);
        load_instance_labels_on_demand(uuidmap_uuid_ptr(ri->uuid), ri);
        rrdinstance_trigger_updates(ri, __FUNCTION__ );
    }

    ref.labels = ri->rrdlabels;
    return ref;
}

void rrdinstance_labels_release(RRDINSTANCE_LABELS_REF *ref) {
    rrdset_acquired_release(ref->rsa);
    ref->rsa = NULL;
    ref->labels = NULL;
}

// ri->rrdset is a raw back-pointer that holds no reference on the chart: holding the instance keeps the
// INSTANCE alive, not the CHART, and the obsolete-chart cleanup frees the chart while such a holder still
// uses it. So consumers must never dereference ri->rrdset - they acquire the chart by id instead.
// rrdset_acquire() has no side effects (it does not touch last_accessed), so it never keeps an obsolete
// chart from being freed. The identity check makes the result authoritative: it succeeds only when the
// acquired chart and THIS instance are linked to each other. Both sides are needed: when a chart changes
// context, rrdinstance_from_rrdset() links the new instance to the chart before the chart to it, and points
// the chart to the new instance before it unlinks the old one, so either side alone briefly accepts an
// instance the chart is not (or no longer) linked with. The chart side is only compared, never dereferenced.
RRDSET_ACQUIRED *rrdinstance_rrdset_acquire(RRDINSTANCE_ACQUIRED *ria) {
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);

    // archived instance: no chart to look up (only compared, never dereferenced)
    if(!rrdinstance_rrdset_atomic_load(ri))
        return NULL;

    RRDSET_ACQUIRED *rsa = rrdset_acquire(ri->rc->rrdhost, string2str(ri->id));
    if(unlikely(!rsa))
        return NULL;

    RRDSET *st = rrdset_acquired_to_rrdset(rsa);
    if(unlikely(rrdinstance_rrdset_atomic_load(ri) != st || st->rrdcontexts.rrdinstance != ria)) {
        rrdset_acquired_release(rsa);
        return NULL;
    }

    return rsa;
}

RRDSET_ACQUIRED *rrdinstance_acquired_rrdset_acquire(RRDINSTANCE_ACQUIRED *ria) {
    RRDSET_ACQUIRED *rsa = rrdinstance_rrdset_acquire(ria);

    // these callers are queries: they keep a queried chart from being freed, as they always did
    if(rsa)
        rrdset_touch_last_accessed_time_s(rrdset_acquired_to_rrdset(rsa));

    return rsa;
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

    // the instance always owns this set, linked to a chart or not
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
    rrdinstance_rrdset_atomic_store(ri, NULL);
}

static void rrdinstance_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *rrdcontext) {
    RRDINSTANCE *ri = value;

    // link it to its parent
    ri->rc = rrdcontext;
    spinlock_init(&ri->spinlock);
    RRDSET *st = rrdinstance_rrdset_atomic_load(ri);

    ri->flags = ri->flags & RRD_FLAGS_ALLOWED_EXTERNALLY_ON_NEW_OBJECTS; // no need for atomics

    if(!ri->name)
        ri->name = string_dup(ri->id);

    ri->rrdlabels = rrdlabels_create();

    if(st)
        ri->flags &= ~(RRD_FLAG_OWN_LABELS| RRD_FLAG_DEMAND_LABELS); // no need of atomics at the constructor
    else
        ri->flags |= (RRD_FLAG_OWN_LABELS | RRD_FLAG_DEMAND_LABELS); // no need of atomics at the constructor

    if(st) {
        if(unlikely(rrdset_flag_check(st, RRDSET_FLAG_HIDDEN)))
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

    internal_error(rrdinstance_rrdset_atomic_load(ri), "RRDINSTANCE: '%s' is freed but there is a RRDSET linked to it.", string2str(ri->id));

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

    // a chart re-created with the same id takes the instance over, even while the old chart still exists:
    // the old chart's teardown (rrdinstance_rrdset_is_freed()) then finds it is no longer the owner
    spinlock_lock(&ri->spinlock);

    RRDSET *st = rrdinstance_rrdset_atomic_load(ri);
    if(ri_new->rrdset && st != ri_new->rrdset) {
        st = ri_new->rrdset;
        rrdinstance_rrdset_atomic_store(ri, st);
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_LINKING);
    }

    if(st) {
        // our labels are not needed any more (the set itself stays: readers of the archived instance may hold it)
        rrdlabels_flush(ri->rrdlabels);
        rrd_flag_clear(ri, RRD_FLAG_OWN_LABELS | RRD_FLAG_DEMAND_LABELS);

        if(unlikely(rrdset_flag_check(st, RRDSET_FLAG_HIDDEN)))
            rrd_flag_set(ri, RRD_FLAG_HIDDEN);
        else
            rrd_flag_clear(ri, RRD_FLAG_HIDDEN);
    }

    spinlock_unlock(&ri->spinlock);

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
    // the chart cannot be destroyed while we hold the lock and it still owns the instance:
    // its teardown takes the lock to unlink itself first
    spinlock_lock(&ri->spinlock);

    RRDSET *st = rrdinstance_rrdset_atomic_load(ri);
    if(st != NULL) {
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
        // (under the lock, so that a chart taking the instance over cannot be archived by us)

        rrdinstance_set_archived(ri);
        rrd_flag_set_updated(ri, RRD_FLAG_UPDATE_REASON_CHANGED_LINKING);
    }

    spinlock_unlock(&ri->spinlock);

    if(rrd_flag_is_updated(ri) || !rrd_flag_check(ri, RRD_FLAG_LIVE_RETENTION)) {
        rrd_flag_set_updated(ri->rc, RRD_FLAG_UPDATE_REASON_TRIGGERED);
        rrdcontext_queue_for_post_processing(ri->rc, function, rrd_flags_get(ri));
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
    if(unlikely(!rca)) {
        // the contexts dictionary is being destroyed (host shutdown); the insert
        // callback never ran, so we still own what we duplicated above
        string_freez(trc.id);
        string_freez(trc.title);
        string_freez(trc.units);
        string_freez(trc.family);
        return;
    }

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
    if(unlikely(!ria)) {
        // the instances dictionary is being destroyed; free what we duplicated
        // above (mirrors rrdinstance_free()) and drop the context reference
        string_freez(tri.id);
        string_freez(tri.name);
        string_freez(tri.units);
        string_freez(tri.family);
        string_freez(tri.title);
        uuidmap_free(tri.uuid);
        rrdcontext_release(rca);
        return;
    }

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

                    // only if a newer dimension has not taken the metric over
                    spinlock_lock(&ri_old->spinlock);
                    if(rrdmetric_rrddim_atomic_load(rm_old) == rd) {
                        rrdmetric_set_deleted_overwrite(rm_old, RRD_FLAG_UPDATED|RRD_FLAG_LIVE_RETENTION|RRD_FLAG_UPDATE_REASON_UNUSED|RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);
                        rrdmetric_rrddim_atomic_store(rm_old, NULL);
                        rm_old->first_time_s = 0;
                        rm_old->last_time_s = 0;
                    }
                    spinlock_unlock(&ri_old->spinlock);

                    rrdmetric_release(rd->rrdcontexts.rrdmetric);
                    rd->rrdcontexts.rrdmetric = NULL;

                    rrdmetric_from_rrddim(rd);
                }
        rrddim_foreach_done(rd);

        // mark the old instance, ready to be deleted - only if a newer chart has not taken it over
        spinlock_lock(&ri_old->spinlock);
        bool owned = rrdinstance_rrdset_atomic_load(ri_old) == st;
        if(owned) {
            // the instance keeps its own (empty) labels from now on: it always has a set, created by the insert
            // callback and flushed while a chart owned it, so there is nothing to create
            if(unlikely(!ri_old->rrdlabels))
                ri_old->rrdlabels = rrdlabels_create();

            rrdinstance_set_deleted_overwrite(ri_old, RRD_FLAG_UPDATED|RRD_FLAG_OWN_LABELS|RRD_FLAG_LIVE_RETENTION|RRD_FLAG_UPDATE_REASON_UNUSED|RRD_FLAG_UPDATE_REASON_ZERO_RETENTION);
            rrdinstance_rrdset_atomic_store(ri_old, NULL);
            ri_old->first_time_s = 0;
            ri_old->last_time_s = 0;
        }
        spinlock_unlock(&ri_old->spinlock);

        if(owned)
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

    if(unlikely(rrdinstance_rrdset_atomic_load(ri) != st))
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

    if(unlikely(!st->rrdcontexts.rrdinstance)) {
        netdata_log_error("RRDINSTANCE: RRDSET '%s' is not linked to an RRDINSTANCE at %s()", rrdset_id(st), __FUNCTION__);
        return;
    }

    RRDINSTANCE *ri = rrdinstance_acquired_value(st->rrdcontexts.rrdinstance);
    if(unlikely(!ri)) {
        netdata_log_error("RRDINSTANCE: RRDSET '%s' lost its link to an RRDINSTANCE at %s()", rrdset_id(st), __FUNCTION__);
        return;
    }

    // A chart is destroyed when its last index item goes, which can be long after it was freed, and a chart
    // re-created with the same id meanwhile has taken the instance over (rrdinstance_conflict_callback()).
    // Then the instance is not ours to archive or to take our labels: only release our references to it.
    spinlock_lock(&ri->spinlock);
    bool owned = rrdinstance_rrdset_atomic_load(ri) == st;
    if(owned) {
        rrdinstance_set_archived(ri);

        // copy, never swap: the instance's set must outlive this chart, readers of the instance may hold it
        rrdlabels_migrate_to_these(ri->rrdlabels, st->rrdlabels);
        rrd_flag_set(ri, RRD_FLAG_OWN_LABELS);

        rrdinstance_rrdset_atomic_store(ri, NULL);
    }
    spinlock_unlock(&ri->spinlock);

    if(owned)
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
    if(unlikely(rrdinstance_rrdset_atomic_load(ri) != st))
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

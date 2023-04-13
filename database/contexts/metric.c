// SPDX-License-Identifier: GPL-3.0-or-later

#include "internal.h"

static void rrdmetric_trigger_updates(RRDMETRIC *rm, const char *function);

inline const char *rrdmetric_acquired_id(RRDMETRIC_ACQUIRED *rma) {
    RRDMETRIC *rm = rrdmetric_acquired_value(rma);
    return string2str(rm->id);
}

inline const char *rrdmetric_acquired_name(RRDMETRIC_ACQUIRED *rma) {
    RRDMETRIC *rm = rrdmetric_acquired_value(rma);
    return string2str(rm->name);
}

inline bool rrdmetric_acquired_has_name(RRDMETRIC_ACQUIRED *rma) {
    RRDMETRIC *rm = rrdmetric_acquired_value(rma);
    return (rm->name && rm->name != rm->id);
}

inline STRING *rrdmetric_acquired_id_dup(RRDMETRIC_ACQUIRED *rma) {
    RRDMETRIC *rm = rrdmetric_acquired_value(rma);
    return string_dup(rm->id);
}

inline STRING *rrdmetric_acquired_name_dup(RRDMETRIC_ACQUIRED *rma) {
    RRDMETRIC *rm = rrdmetric_acquired_value(rma);
    return string_dup(rm->name);
}

inline NETDATA_DOUBLE rrdmetric_acquired_last_stored_value(RRDMETRIC_ACQUIRED *rma) {
    RRDMETRIC *rm = rrdmetric_acquired_value(rma);

    if(rm->rrddim)
        return rm->rrddim->last_stored_value;

    return NAN;
}

inline bool rrdmetric_acquired_belongs_to_instance(RRDMETRIC_ACQUIRED *rma, RRDINSTANCE_ACQUIRED *ria) {
    RRDMETRIC *rm = rrdmetric_acquired_value(rma);
    RRDINSTANCE *ri = rrdinstance_acquired_value(ria);
    return rm->ri == ri;
}

inline time_t rrdmetric_acquired_first_entry(RRDMETRIC_ACQUIRED *rma) {
    RRDMETRIC *rm = rrdmetric_acquired_value(rma);
    return rm->first_time_s;
}

inline time_t rrdmetric_acquired_last_entry(RRDMETRIC_ACQUIRED *rma) {
    RRDMETRIC *rm = rrdmetric_acquired_value(rma);

    if(rrd_flag_check(rm, RRD_FLAG_COLLECTED))
        return 0;

    return rm->last_time_s;
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

    if(uuid_memcmp(&rm->uuid, &rm_new->uuid) != 0) {
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
    if(rm->rrddim && uuid_memcmp(&rm->uuid, &rm->rrddim->metric_uuid) != 0) {
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

void rrdmetrics_create_in_rrdinstance(RRDINSTANCE *ri) {
    if(unlikely(!ri)) return;
    if(likely(ri->rrdmetrics)) return;

    ri->rrdmetrics = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                &dictionary_stats_category_rrdcontext, sizeof(RRDMETRIC));

    dictionary_register_insert_callback(ri->rrdmetrics, rrdmetric_insert_callback, ri);
    dictionary_register_delete_callback(ri->rrdmetrics, rrdmetric_delete_callback, ri);
    dictionary_register_conflict_callback(ri->rrdmetrics, rrdmetric_conflict_callback, ri);
    dictionary_register_react_callback(ri->rrdmetrics, rrdmetric_react_callback, ri);
}

void rrdmetrics_destroy_from_rrdinstance(RRDINSTANCE *ri) {
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

void rrdmetric_from_rrddim(RRDDIM *rd) {
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

inline void rrdmetric_rrddim_is_freed(RRDDIM *rd) {
    RRDMETRIC *rm = rrddim_get_rrdmetric(rd);
    if(unlikely(!rm)) return;

    if(unlikely(rrd_flag_is_collected(rm)))
        rrd_flag_set_archived(rm);

    rm->rrddim = NULL;
    rrdmetric_trigger_updates(rm, __FUNCTION__ );
    rrdmetric_release(rd->rrdmetric);
    rd->rrdmetric = NULL;
}

inline void rrdmetric_updated_rrddim_flags(RRDDIM *rd) {
    RRDMETRIC *rm = rrddim_get_rrdmetric(rd);
    if(unlikely(!rm)) return;

    if(unlikely(rrddim_flag_check(rd, RRDDIM_FLAG_ARCHIVED|RRDDIM_FLAG_OBSOLETE))) {
        if(unlikely(rrd_flag_is_collected(rm)))
            rrd_flag_set_archived(rm);
    }

    rrdmetric_trigger_updates(rm, __FUNCTION__ );
}

inline void rrdmetric_collected_rrddim(RRDDIM *rd) {
    RRDMETRIC *rm = rrddim_get_rrdmetric(rd);
    if(unlikely(!rm)) return;

    if(unlikely(!rrd_flag_is_collected(rm)))
        rrd_flag_set_collected(rm);

    // we use this variable to detect BEGIN/END without SET
    rm->ri->internal.collected_metrics_count++;

    rrdmetric_trigger_updates(rm, __FUNCTION__ );
}


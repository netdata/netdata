// SPDX-License-Identifier: GPL-3.0-or-later

#include "internal.h"

inline const char *rrdcontext_acquired_id(RRDCONTEXT_ACQUIRED *rca) {
    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);
    return string2str(rc->id);
}

inline bool rrdcontext_acquired_belongs_to_host(RRDCONTEXT_ACQUIRED *rca, RRDHOST *host) {
    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);
    return rc->rrdhost == host;
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
            netdata_log_error("RRDCONTEXT: context '%s' is already initialized with version %"PRIu64", but it is loaded again from SQL with version %"PRIu64"", string2str(rc->id), rc->version, rc->hub.version);

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
            rrdcontext_set_deleted(rc, RRD_FLAG_NONE);
        else {
            if (rc->last_time_s == 0)
                rrdcontext_set_collected(rc);
            else
                rrdcontext_set_archived(rc);
        }

        rc->flags |= RRD_FLAG_UPDATE_REASON_LOAD_SQL; // no need for atomics at constructor
    }
    else {
        // we are adding this context now for the first time
        rc->version = now_realtime_sec();
    }

    rrdinstances_create_in_rrdcontext(rc);
    spinlock_init(&rc->spinlock);

    // update the count of contexts
    __atomic_add_fetch(&rc->rrdhost->rrdctx.contexts_count, 1, __ATOMIC_RELAXED);

    // signal the react callback to do the job
    rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_NEW_OBJECT);
}

static void rrdcontext_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *rrdhost __maybe_unused) {

    RRDCONTEXT *rc = (RRDCONTEXT *)value;

    // update the count of contexts
    __atomic_sub_fetch(&rc->rrdhost->rrdctx.contexts_count, 1, __ATOMIC_RELAXED);

    rrdinstances_destroy_from_rrdcontext(rc);
    rrdcontext_freez(rc);
}

typedef enum __attribute__((packed)) {
    OLDNEW_KEEP_OLD,
    OLDNEW_USE_NEW,
    OLDNEW_MERGE,
} OLDNEW;

static inline OLDNEW oldnew_decide(bool archived, bool new_archived) {
    if(archived && !new_archived)
        return OLDNEW_USE_NEW;

    if(!archived && new_archived)
        return OLDNEW_KEEP_OLD;

    return OLDNEW_MERGE;
}

static inline void string_replace(STRING **stringpp, STRING *new_string) {
    STRING *old = *stringpp;
    *stringpp = string_dup(new_string);
    string_freez(old);
}

static inline void string_merge(STRING **stringpp, STRING *new_string) {
    STRING *old = *stringpp;
    *stringpp = string_2way_merge(*stringpp, new_string);
    string_freez(old);
}

static void rrdcontext_merge_with(RRDCONTEXT *rc, bool archived, STRING *title, STRING *family, STRING *units, RRDSET_TYPE chart_type, uint32_t priority) {
    OLDNEW oldnew = oldnew_decide(rrd_flag_is_archived(rc), archived);

    switch(oldnew) {
        case OLDNEW_KEEP_OLD:
            break;

        case OLDNEW_USE_NEW:
            if(rc->title != title) {
                string_replace(&rc->title, title);
                rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
            }
            if(rc->family != family) {
                string_replace(&rc->family, family);
                rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
            }
            break;

        case OLDNEW_MERGE:
            if(rc->title != title) {
                string_merge(&rc->title, title);
                rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
            }
            if(rc->family != family) {
                string_merge(&rc->family, family);
                rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
            }
            break;
    }

    switch(oldnew) {
        case OLDNEW_KEEP_OLD:
            break;

        case OLDNEW_USE_NEW:
        case OLDNEW_MERGE:
            if(rc->units != units) {
                string_replace(&rc->units, units);
                rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
            }

            if(rc->chart_type != chart_type) {
                rc->chart_type = chart_type;
                rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
            }

            if(rc->priority != priority) {
                rc->priority = priority;
                rrd_flag_set_updated(rc, RRD_FLAG_UPDATE_REASON_CHANGED_METADATA);
            }
            break;
    }
}

void rrdcontext_update_from_collected_rrdinstance(RRDINSTANCE *ri) {
    rrdcontext_merge_with(ri->rc, rrd_flag_is_archived(ri),
                          ri->title, ri->family, ri->units, ri->chart_type, ri->priority);
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

    rrdcontext_merge_with(rc, rrd_flag_is_archived(rc_new),
                          rc_new->title, rc_new->family, rc_new->units, rc_new->chart_type, rc_new->priority);

    rrd_flag_set(rc, rc_new->flags & RRD_FLAGS_ALLOWED_EXTERNALLY_ON_NEW_OBJECTS); // no need for atomics on rc_new

    if(rrd_flag_is_collected(rc) && rrd_flag_is_archived(rc))
        rrdcontext_set_collected(rc);

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

void rrdcontext_trigger_updates(RRDCONTEXT *rc, const char *function) {
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

    // IMPORTANT:
    // Do not rely on this flag being absent, because the dictionaries have delayed deletions (garbage collect)
    // so, this flag may not be deleted immediately from the context.
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
    if(likely(host->rrdctx.contexts)) return;

    host->rrdctx.contexts = dictionary_create_advanced(
            DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            &dictionary_stats_category_rrdcontext, sizeof(RRDCONTEXT));

    dictionary_register_insert_callback(host->rrdctx.contexts, rrdcontext_insert_callback, host);
    dictionary_register_delete_callback(host->rrdctx.contexts, rrdcontext_delete_callback, host);
    dictionary_register_conflict_callback(host->rrdctx.contexts, rrdcontext_conflict_callback, host);
    dictionary_register_react_callback(host->rrdctx.contexts, rrdcontext_react_callback, host);

    host->rrdctx.hub_queue = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_VALUE_LINK_DONT_CLONE, &dictionary_stats_category_rrdcontext, 0);
    dictionary_register_insert_callback(host->rrdctx.hub_queue, rrdcontext_hub_queue_insert_callback, NULL);
    dictionary_register_delete_callback(host->rrdctx.hub_queue, rrdcontext_hub_queue_delete_callback, NULL);
    dictionary_register_conflict_callback(host->rrdctx.hub_queue, rrdcontext_hub_queue_conflict_callback, NULL);

    host->rrdctx.pp_queue = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_VALUE_LINK_DONT_CLONE, &dictionary_stats_category_rrdcontext, 0);
    dictionary_register_insert_callback(host->rrdctx.pp_queue, rrdcontext_post_processing_queue_insert_callback, NULL);
    dictionary_register_delete_callback(host->rrdctx.pp_queue, rrdcontext_post_processing_queue_delete_callback, NULL);
    dictionary_register_conflict_callback(host->rrdctx.pp_queue, rrdcontext_post_processing_queue_conflict_callback, NULL);
}

void rrdhost_destroy_rrdcontexts(RRDHOST *host) {
    if(unlikely(!host)) return;
    if(unlikely(!host->rrdctx.contexts)) return;

    DICTIONARY *old;

    if(host->rrdctx.hub_queue) {
        old = host->rrdctx.hub_queue;
        host->rrdctx.hub_queue = NULL;

        RRDCONTEXT *rc;
        dfe_start_write(old, rc) {
                    dictionary_del(old, string2str(rc->id));
                }
        dfe_done(rc);
        dictionary_destroy(old);
    }

    if(host->rrdctx.pp_queue) {
        old = host->rrdctx.pp_queue;
        host->rrdctx.pp_queue = NULL;

        RRDCONTEXT *rc;
        dfe_start_write(old, rc) {
                    dictionary_del(old, string2str(rc->id));
                }
        dfe_done(rc);
        dictionary_destroy(old);
    }

    old = host->rrdctx.contexts;
    host->rrdctx.contexts = NULL;
    dictionary_destroy(old);
}


// SPDX-License-Identifier: GPL-3.0-or-later

#define NETDATA_RRD_INTERNALS
#include "rrd.h"
#include <sched.h>
#include "storage_engine.h"

// ----------------------------------------------------------------------------
// RRDSET name index

static void rrdset_name_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdset, void *rrdhost __maybe_unused) {
    RRDSET *st = rrdset;
    rrdset_flag_set(st, RRDSET_FLAG_INDEXED_NAME);
}
static void rrdset_name_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdset, void *rrdhost __maybe_unused) {
    RRDSET *st = rrdset;
    rrdset_flag_clear(st, RRDSET_FLAG_INDEXED_NAME);
}

static inline void rrdset_index_add_name(RRDHOST *host, RRDSET *st) {
    if(!st->name) return;
    dictionary_set(host->rrdset_root_index_name, rrdset_name(st), st, sizeof(RRDSET));
}

static inline void rrdset_index_del_name(RRDHOST *host, RRDSET *st) {
    if(rrdset_flag_check(st, RRDSET_FLAG_INDEXED_NAME))
        dictionary_del(host->rrdset_root_index_name, rrdset_name(st));
}

static inline RRDSET *rrdset_index_find_name(RRDHOST *host, const char *name) {
    if (unlikely(!host->rrdset_root_index_name))
        return NULL;
    return dictionary_get(host->rrdset_root_index_name, name);
}

// ----------------------------------------------------------------------------
// RRDSET index

static inline void rrdset_update_permanent_labels(RRDSET *st) {
    if(!st->rrdlabels) return;

    rrdlabels_add(st->rrdlabels, "_collect_plugin", rrdset_plugin_name(st), RRDLABEL_SRC_AUTO| RRDLABEL_FLAG_PERMANENT);
    rrdlabels_add(st->rrdlabels, "_collect_module", rrdset_module_name(st), RRDLABEL_SRC_AUTO| RRDLABEL_FLAG_PERMANENT);
}

static STRING *rrdset_fix_name(RRDHOST *host, const char *chart_full_id, const char *type, const char *current_name, const char *name) {
    if(!name || !*name) return NULL;

    char full_name[RRD_ID_LENGTH_MAX + 1];
    char sanitized_name[CONFIG_MAX_VALUE + 1];
    char new_name[CONFIG_MAX_VALUE + 1];

    snprintfz(full_name, RRD_ID_LENGTH_MAX, "%s.%s", type, name);
    rrdset_strncpyz_name(sanitized_name, full_name, CONFIG_MAX_VALUE);
    strncpyz(new_name, sanitized_name, CONFIG_MAX_VALUE);

    if(rrdset_index_find_name(host, new_name)) {
        debug(D_RRD_CALLS, "RRDSET: chart name '%s' on host '%s' already exists.", new_name, rrdhost_hostname(host));
        if(!strcmp(chart_full_id, full_name) && (!current_name || !*current_name)) {
            unsigned i = 1;

            do {
                snprintfz(new_name, CONFIG_MAX_VALUE, "%s_%u", sanitized_name, i);
                i++;
            } while (rrdset_index_find_name(host, new_name));

            info("RRDSET: using name '%s' for chart '%s' on host '%s'.", new_name, full_name, rrdhost_hostname(host));
        }
        else
            return NULL;
    }

    return string_strdupz(new_name);
}

struct rrdset_constructor {
    RRDHOST *host;
    const char *type;
    const char *id;
    const char *name;
    const char *family;
    const char *context;
    const char *title;
    const char *units;
    const char *plugin;
    const char *module;
    long priority;
    int update_every;
    RRDSET_TYPE chart_type;
    RRD_MEMORY_MODE memory_mode;
    long history_entries;

    enum {
        RRDSET_REACT_NONE                   = 0,
        RRDSET_REACT_NEW                    = (1 << 0),
        RRDSET_REACT_UPDATED                = (1 << 1),
        RRDSET_REACT_PLUGIN_UPDATED         = (1 << 2),
        RRDSET_REACT_MODULE_UPDATED         = (1 << 3),
        RRDSET_REACT_CHART_ACTIVATED        = (1 << 4),
    } react_action;
};

// the constructor - the dictionary is write locked while this runs
static void rrdset_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdset, void *constructor_data) {
    struct rrdset_constructor *ctr = constructor_data;
    RRDHOST *host = ctr->host;
    RRDSET *st = rrdset;

    const char *chart_full_id = dictionary_acquired_item_name(item);

    st->id = string_strdupz(chart_full_id);

    st->name = rrdset_fix_name(host, chart_full_id, ctr->type, NULL, ctr->name);
    if(!st->name)
        st->name = rrdset_fix_name(host, chart_full_id, ctr->type, NULL, ctr->id);
    rrdset_index_add_name(host, st);

    st->parts.id = string_strdupz(ctr->id);
    st->parts.type = string_strdupz(ctr->type);
    st->parts.name = string_strdupz(ctr->name);

    st->family = (ctr->family && *ctr->family) ? rrd_string_strdupz(ctr->family) : rrd_string_strdupz(ctr->type);
    st->context = (ctr->context && *ctr->context) ? rrd_string_strdupz(ctr->context) : rrd_string_strdupz(chart_full_id);

    st->units = rrd_string_strdupz(ctr->units);
    st->title = rrd_string_strdupz(ctr->title);
    st->plugin_name = rrd_string_strdupz(ctr->plugin);
    st->module_name = rrd_string_strdupz(ctr->module);
    st->priority = ctr->priority;

    st->entries = (ctr->memory_mode != RRD_MEMORY_MODE_DBENGINE) ? align_entries_to_pagesize(ctr->memory_mode, ctr->history_entries) : 5;
    st->update_every = ctr->update_every;
    st->rrd_memory_mode = ctr->memory_mode;

    st->chart_type = ctr->chart_type;
    st->rrdhost = host;

    netdata_spinlock_init(&st->data_collection_lock);

    st->flags =   RRDSET_FLAG_SYNC_CLOCK
                | RRDSET_FLAG_INDEXED_ID
                | RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED
                | RRDSET_FLAG_SENDER_REPLICATION_FINISHED
                ;

    netdata_rwlock_init(&st->alerts.rwlock);

    if(st->rrd_memory_mode == RRD_MEMORY_MODE_SAVE || st->rrd_memory_mode == RRD_MEMORY_MODE_MAP) {
        if(!rrdset_memory_load_or_create_map_save(st, st->rrd_memory_mode)) {
            info("Failed to use db mode %s for chart '%s', falling back to ram mode.", (st->rrd_memory_mode == RRD_MEMORY_MODE_MAP)?"map":"save", rrdset_name(st));
            st->rrd_memory_mode = RRD_MEMORY_MODE_RAM;
        }
    }

    // initialize the db tiers
    {
        for(size_t tier = 0; tier < storage_tiers ; tier++) {
            STORAGE_ENGINE *eng = st->rrdhost->db[tier].eng;
            if(!eng) continue;

            st->storage_metrics_groups[tier] = storage_engine_metrics_group_get(eng->backend, host->db[tier].instance, &st->chart_uuid);
        }
    }

    rrddim_index_init(st);

    // chart variables - we need this for data collection to work (collector given chart variables) - not only health
    rrdsetvar_index_init(st);

    if (host->health.health_enabled) {
        st->rrdfamily = rrdfamily_add_and_acquire(host, rrdset_family(st));
        st->rrdvars = rrdvariables_create();
        rrddimvar_index_init(st);
    }

    st->rrdlabels = rrdlabels_create();
    rrdset_update_permanent_labels(st);

    st->green = NAN;
    st->red = NAN;

    ctr->react_action = RRDSET_REACT_NEW;

    ml_chart_new(st);
}

void pluginsd_rrdset_cleanup(RRDSET *st);

void rrdset_finalize_collection(RRDSET *st, bool dimensions_too) {
    RRDHOST *host = st->rrdhost;

    rrdset_flag_set(st, RRDSET_FLAG_COLLECTION_FINISHED);

    if(dimensions_too) {
        RRDDIM *rd;
        rrddim_foreach_read(rd, st)
            rrddim_finalize_collection_and_check_retention(rd);
        rrddim_foreach_done(rd);
    }

    for(size_t tier = 0; tier < storage_tiers ; tier++) {
        STORAGE_ENGINE *eng = st->rrdhost->db[tier].eng;
        if(!eng) continue;

        if(st->storage_metrics_groups[tier]) {
            storage_engine_metrics_group_release(eng->backend, host->db[tier].instance, st->storage_metrics_groups[tier]);
            st->storage_metrics_groups[tier] = NULL;
        }
    }

    pluginsd_rrdset_cleanup(st);
}

// the destructor - the dictionary is write locked while this runs
static void rrdset_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdset, void *rrdhost) {
    RRDHOST *host = rrdhost;
    RRDSET *st = rrdset;

    rrdset_flag_clear(st, RRDSET_FLAG_INDEXED_ID);

    rrdset_finalize_collection(st, false);

    // remove it from the name index
    rrdset_index_del_name(host, st);

    // release the collector info
    dictionary_destroy(st->functions_view);

    rrdcalc_unlink_all_rrdset_alerts(st);

    // ------------------------------------------------------------------------
    // the order of destruction is important here

    // 1. delete RRDDIMVAR index - this will speed up the destruction of RRDDIMs
    //    because each dimension loops to find its own variables in this index.
    //    There are no references to the items on this index from the dimensions.
    //    To find their own, they have to walk-through the dictionary.
    rrddimvar_index_destroy(st);                // destroy the rrddimvar index

    // 2. delete RRDSETVAR index
    rrdsetvar_index_destroy(st);                // destroy the rrdsetvar index

    // 3. delete RRDVAR index after the above, to avoid triggering its garbage collector (they have references on this)
    rrdvariables_destroy(st->rrdvars);      // free all variables and destroy the rrdvar dictionary

    // 4. delete RRDFAMILY - this has to be last, because RRDDIMVAR and RRDSETVAR need the reference counter
    rrdfamily_release(host, st->rrdfamily); // release the acquired rrdfamily -- has to be after all variables

    // 5. delete RRDDIMs, now their variables are not existing, so this is fast
    rrddim_index_destroy(st);                   // free all the dimensions and destroy the dimensions index

    // 6. this has to be after the dimensions are freed, but before labels are freed (contexts need the labels)
    rrdcontext_removed_rrdset(st);              // let contexts know

    // 7. destroy the chart labels
    rrdlabels_destroy(st->rrdlabels);  // destroy the labels, after letting the contexts know

    // 8. destroy the ml handle
    ml_chart_delete(st);

    rrdset_memory_file_free(st);                // remove files of db mode save and map

    // ------------------------------------------------------------------------
    // free it

    netdata_rwlock_destroy(&st->alerts.rwlock);

    string_freez(st->id);
    string_freez(st->name);
    string_freez(st->parts.id);
    string_freez(st->parts.type);
    string_freez(st->parts.name);
    string_freez(st->family);
    string_freez(st->title);
    string_freez(st->units);
    string_freez(st->context);
    string_freez(st->plugin_name);
    string_freez(st->module_name);

    freez(st->exporting_flags);
    freez(st->cache_dir);
}

// the item to be inserted, is already in the dictionary
// this callback deals with the situation, migrating the existing object to the new values
// the dictionary is write locked while this runs
static bool rrdset_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdset, void *new_rrdset, void *constructor_data) {
    (void)new_rrdset; // it is NULL

    struct rrdset_constructor *ctr = constructor_data;
    RRDSET *st = rrdset;

    rrdset_isnot_obsolete(st);

    ctr->react_action = RRDSET_REACT_NONE;

    if (rrdset_flag_check(st, RRDSET_FLAG_ARCHIVED)) {
        rrdset_flag_clear(st, RRDSET_FLAG_ARCHIVED);
        ctr->react_action |= RRDSET_REACT_CHART_ACTIVATED;
    }

    if (rrdset_reset_name(st, (ctr->name && *ctr->name) ? ctr->name : ctr->id) == 2)
        ctr->react_action |= RRDSET_REACT_UPDATED;

    if (unlikely(st->priority != ctr->priority)) {
        st->priority = ctr->priority;
        ctr->react_action |= RRDSET_REACT_UPDATED;
    }

    if (unlikely(st->update_every != ctr->update_every)) {
        rrdset_set_update_every_s(st, ctr->update_every);
        ctr->react_action |= RRDSET_REACT_UPDATED;
    }

    if(ctr->plugin && *ctr->plugin) {
        STRING *old_plugin = st->plugin_name;
        st->plugin_name = rrd_string_strdupz(ctr->plugin);
        if (old_plugin != st->plugin_name)
            ctr->react_action |= RRDSET_REACT_PLUGIN_UPDATED;
        string_freez(old_plugin);
    }

    if(ctr->module && *ctr->module) {
        STRING *old_module = st->module_name;
        st->module_name = rrd_string_strdupz(ctr->module);
        if (old_module != st->module_name)
            ctr->react_action |= RRDSET_REACT_MODULE_UPDATED;
        string_freez(old_module);
    }

    if(ctr->title && *ctr->title) {
        STRING *old_title = st->title;
        st->title = rrd_string_strdupz(ctr->title);
        if(old_title != st->title)
            ctr->react_action |= RRDSET_REACT_UPDATED;
        string_freez(old_title);
    }

    if(ctr->units && *ctr->units) {
        STRING *old_units = st->units;
        st->units = rrd_string_strdupz(ctr->units);
        if(old_units != st->units)
            ctr->react_action |= RRDSET_REACT_UPDATED;
        string_freez(old_units);
    }

    if(ctr->family && *ctr->family) {
        STRING *old_family = st->family;
        st->family = rrd_string_strdupz(ctr->family);
        if(old_family != st->family)
            ctr->react_action |= RRDSET_REACT_UPDATED;
        string_freez(old_family);

        // TODO - we should rename RRDFAMILY variables
    }

    if(ctr->context && *ctr->context) {
        STRING *old_context = st->context;
        st->context = rrd_string_strdupz(ctr->context);
        if(old_context != st->context)
            ctr->react_action |= RRDSET_REACT_UPDATED;
        string_freez(old_context);
    }

    if(st->chart_type != ctr->chart_type) {
        st->chart_type = ctr->chart_type;
        ctr->react_action |= RRDSET_REACT_UPDATED;
    }

    rrdset_update_permanent_labels(st);

    rrdset_flag_set(st, RRDSET_FLAG_SYNC_CLOCK);
    rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_EXPOSED);

    return ctr->react_action != RRDSET_REACT_NONE;
}

// this is called after all insertions/conflicts, with the dictionary unlocked, with a reference to RRDSET
// so, any actions requiring locks on other objects, should be placed here
static void rrdset_react_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdset, void *constructor_data) {
    struct rrdset_constructor *ctr = constructor_data;
    RRDSET *st = rrdset;
    RRDHOST *host = st->rrdhost;

    st->last_accessed_time_s = now_realtime_sec();

    if(host->health.health_enabled && (ctr->react_action & (RRDSET_REACT_NEW | RRDSET_REACT_CHART_ACTIVATED))) {
        rrdset_flag_set(st, RRDSET_FLAG_PENDING_HEALTH_INITIALIZATION);
        rrdhost_flag_set(st->rrdhost, RRDHOST_FLAG_PENDING_HEALTH_INITIALIZATION);
    }

    if(ctr->react_action & (RRDSET_REACT_NEW | RRDSET_REACT_PLUGIN_UPDATED | RRDSET_REACT_MODULE_UPDATED)) {
        if (ctr->react_action & RRDSET_REACT_NEW) {
            if(unlikely(rrdcontext_find_chart_uuid(st,  &st->chart_uuid)))
                uuid_generate(st->chart_uuid);
        }
        rrdset_flag_set(st, RRDSET_FLAG_METADATA_UPDATE);
        rrdhost_flag_set(st->rrdhost, RRDHOST_FLAG_METADATA_UPDATE);
    }

    rrdcontext_updated_rrdset(st);
}

void rrdset_index_init(RRDHOST *host) {
    if(!host->rrdset_root_index) {
        host->rrdset_root_index = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                             &dictionary_stats_category_rrdset_rrddim, sizeof(RRDSET));

        dictionary_register_insert_callback(host->rrdset_root_index, rrdset_insert_callback, NULL);
        dictionary_register_conflict_callback(host->rrdset_root_index, rrdset_conflict_callback, NULL);
        dictionary_register_react_callback(host->rrdset_root_index, rrdset_react_callback, NULL);
        dictionary_register_delete_callback(host->rrdset_root_index, rrdset_delete_callback, host);
    }

    if(!host->rrdset_root_index_name) {
        host->rrdset_root_index_name = dictionary_create_advanced(
            DICT_OPTION_NAME_LINK_DONT_CLONE | DICT_OPTION_VALUE_LINK_DONT_CLONE | DICT_OPTION_DONT_OVERWRITE_VALUE,
            &dictionary_stats_category_rrdset_rrddim, 0);

        dictionary_register_insert_callback(host->rrdset_root_index_name, rrdset_name_insert_callback, host);
        dictionary_register_delete_callback(host->rrdset_root_index_name, rrdset_name_delete_callback, host);
    }
}

void rrdset_index_destroy(RRDHOST *host) {
    // destroy the name index first
    dictionary_destroy(host->rrdset_root_index_name);
    host->rrdset_root_index_name = NULL;

    // destroy the id index last
    dictionary_destroy(host->rrdset_root_index);
    host->rrdset_root_index = NULL;
}

static inline RRDSET *rrdset_index_add(RRDHOST *host, const char *id, struct rrdset_constructor *st_ctr) {
    return dictionary_set_advanced(host->rrdset_root_index, id, -1, NULL, sizeof(RRDSET), st_ctr);
}

static inline void rrdset_index_del(RRDHOST *host, RRDSET *st) {
    if(rrdset_flag_check(st, RRDSET_FLAG_INDEXED_ID))
        dictionary_del(host->rrdset_root_index, rrdset_id(st));
}

static RRDSET *rrdset_index_find(RRDHOST *host, const char *id) {
    // TODO - the name index should have an acquired dictionary item, not just a pointer to RRDSET
    if (unlikely(!host->rrdset_root_index))
        return NULL;
    return dictionary_get(host->rrdset_root_index, id);
}

// ----------------------------------------------------------------------------
// RRDSET - find charts

inline RRDSET *rrdset_find(RRDHOST *host, const char *id) {
    debug(D_RRD_CALLS, "rrdset_find() for chart '%s' in host '%s'", id, rrdhost_hostname(host));
    RRDSET *st = rrdset_index_find(host, id);

    if(st)
        st->last_accessed_time_s = now_realtime_sec();

    return(st);
}

inline RRDSET *rrdset_find_bytype(RRDHOST *host, const char *type, const char *id) {
    debug(D_RRD_CALLS, "rrdset_find_bytype() for chart '%s.%s' in host '%s'", type, id, rrdhost_hostname(host));

    char buf[RRD_ID_LENGTH_MAX + 1];
    strncpyz(buf, type, RRD_ID_LENGTH_MAX - 1);
    strcat(buf, ".");
    int len = (int) strlen(buf);
    strncpyz(&buf[len], id, (size_t) (RRD_ID_LENGTH_MAX - len));

    return(rrdset_find(host, buf));
}

inline RRDSET *rrdset_find_byname(RRDHOST *host, const char *name) {
    debug(D_RRD_CALLS, "rrdset_find_byname() for chart '%s' in host '%s'", name, rrdhost_hostname(host));
    RRDSET *st = rrdset_index_find_name(host, name);
    return(st);
}

RRDSET_ACQUIRED *rrdset_find_and_acquire(RRDHOST *host, const char *id) {
    debug(D_RRD_CALLS, "rrdset_find_and_acquire() for host %s, chart %s", rrdhost_hostname(host), id);

    return (RRDSET_ACQUIRED *)dictionary_get_and_acquire_item(host->rrdset_root_index, id);
}

RRDSET *rrdset_acquired_to_rrdset(RRDSET_ACQUIRED *rsa) {
    if(unlikely(!rsa))
        return NULL;

    return (RRDSET *) dictionary_acquired_item_value((const DICTIONARY_ITEM *)rsa);
}

void rrdset_acquired_release(RRDSET_ACQUIRED *rsa) {
    if(unlikely(!rsa))
        return;

    RRDSET *rs = rrdset_acquired_to_rrdset(rsa);
    dictionary_acquired_item_release(rs->rrdhost->rrdset_root_index, (const DICTIONARY_ITEM *)rsa);
}

// ----------------------------------------------------------------------------
// RRDSET - rename charts

char *rrdset_strncpyz_name(char *to, const char *from, size_t length) {
    char c, *p = to;

    while (length-- && (c = *from++)) {
        if(c != '.' && c != '-' && !isalnum(c))
            c = '_';

        *p++ = c;
    }

    *p = '\0';

    return to;
}

int rrdset_reset_name(RRDSET *st, const char *name) {
    if(unlikely(!strcmp(rrdset_name(st), name)))
        return 1;

    RRDHOST *host = st->rrdhost;

    debug(D_RRD_CALLS, "rrdset_reset_name() old: '%s', new: '%s'", rrdset_name(st), name);

    STRING *name_string = rrdset_fix_name(host, rrdset_id(st), rrdset_parts_type(st), string2str(st->name), name);
    if(!name_string) return 0;

    if(st->name) {
        rrdset_index_del_name(host, st);
        string_freez(st->name);
        st->name = name_string;
        rrdsetvar_rename_all(st);
    }
    else
        st->name = name_string;

    RRDDIM *rd;
    rrddim_foreach_read(rd, st)
        rrddimvar_rename_all(rd);
    rrddim_foreach_done(rd);

    rrdset_index_add_name(host, st);

    rrdset_flag_clear(st, RRDSET_FLAG_EXPORTING_SEND);
    rrdset_flag_clear(st, RRDSET_FLAG_EXPORTING_IGNORE);
    rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_SEND);
    rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_IGNORE);
    rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_EXPOSED);

    rrdcontext_updated_rrdset_name(st);
    return 2;
}

// get the timestamp of the last entry in the round-robin database
time_t rrdset_last_entry_s(RRDSET *st) {
    RRDDIM *rd;
    time_t last_entry_s  = 0;

    rrddim_foreach_read(rd, st) {
        time_t t = rrddim_last_entry_s(rd);
        if(t > last_entry_s) last_entry_s = t;
    }
    rrddim_foreach_done(rd);

    return last_entry_s;
}

time_t rrdset_last_entry_s_of_tier(RRDSET *st, size_t tier) {
    RRDDIM *rd;
    time_t last_entry_s  = 0;

    rrddim_foreach_read(rd, st) {
                time_t t = rrddim_last_entry_s_of_tier(rd, tier);
                if(t > last_entry_s) last_entry_s = t;
            }
    rrddim_foreach_done(rd);

    return last_entry_s;
}

// get the timestamp of first entry in the round-robin database
time_t rrdset_first_entry_s(RRDSET *st) {
    RRDDIM *rd;
    time_t first_entry_s = LONG_MAX;

    rrddim_foreach_read(rd, st) {
        time_t t = rrddim_first_entry_s(rd);
        if(t < first_entry_s)
            first_entry_s = t;
    }
    rrddim_foreach_done(rd);

    if (unlikely(LONG_MAX == first_entry_s)) return 0;
    return first_entry_s;
}

time_t rrdset_first_entry_s_of_tier(RRDSET *st, size_t tier) {
    if(unlikely(tier > storage_tiers))
        return 0;

    RRDDIM *rd;
    time_t first_entry_s = LONG_MAX;

    rrddim_foreach_read(rd, st) {
        time_t t = rrddim_first_entry_s_of_tier(rd, tier);
        if(t && t < first_entry_s)
            first_entry_s = t;
    }
    rrddim_foreach_done(rd);

    if (unlikely(LONG_MAX == first_entry_s)) return 0;
    return first_entry_s;
}

void rrdset_get_retention_of_tier_for_collected_chart(RRDSET *st, time_t *first_time_s, time_t *last_time_s, time_t now_s, size_t tier) {
    if(!now_s)
        now_s = now_realtime_sec();

    time_t db_first_entry_s = rrdset_first_entry_s_of_tier(st, tier);
    time_t db_last_entry_s = st->last_updated.tv_sec; // we assume this is a collected RRDSET

    if(unlikely(!db_last_entry_s)) {
        db_last_entry_s = rrdset_last_entry_s_of_tier(st, tier);

        if (unlikely(!db_last_entry_s)) {
            // we assume this is a collected RRDSET
            db_first_entry_s = 0;
            db_last_entry_s = 0;
        }
    }

    if(unlikely(db_last_entry_s > now_s)) {
        internal_error(db_last_entry_s > now_s + 1,
                       "RRDSET: 'host:%s/chart:%s' latest db time %ld is in the future, adjusting it to now %ld",
                       rrdhost_hostname(st->rrdhost), rrdset_id(st),
                       db_last_entry_s, now_s);
        db_last_entry_s = now_s;
    }

    if(unlikely(db_first_entry_s && db_last_entry_s && db_first_entry_s >= db_last_entry_s)) {
        internal_error(db_first_entry_s > db_last_entry_s,
                       "RRDSET: 'host:%s/chart:%s' oldest db time %ld is bigger than latest db time %ld, adjusting it to (latest time %ld - update every %ld)",
                       rrdhost_hostname(st->rrdhost), rrdset_id(st),
                       db_first_entry_s, db_last_entry_s,
                       db_last_entry_s, (time_t)st->update_every);
        db_first_entry_s = db_last_entry_s - st->update_every;
    }

    if(unlikely(!db_first_entry_s && db_last_entry_s))
        // this can be the case on the first data collection of a chart
        db_first_entry_s = db_last_entry_s - st->update_every;

    *first_time_s = db_first_entry_s;
    *last_time_s = db_last_entry_s;
}

inline void rrdset_is_obsolete(RRDSET *st) {
    if(unlikely(rrdset_flag_check(st, RRDSET_FLAG_ARCHIVED))) {
        info("Cannot obsolete already archived chart %s", rrdset_name(st));
        return;
    }

    if(unlikely(!(rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE)))) {
        rrdset_flag_set(st, RRDSET_FLAG_OBSOLETE);
        rrdhost_flag_set(st->rrdhost, RRDHOST_FLAG_PENDING_OBSOLETE_CHARTS);

        st->last_accessed_time_s = now_realtime_sec();

        rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_EXPOSED);

        // the chart will not get more updates (data collection)
        // so, we have to push its definition now
        rrdset_push_chart_definition_now(st);
        rrdcontext_updated_rrdset_flags(st);
    }
}

inline void rrdset_isnot_obsolete(RRDSET *st) {
    if(unlikely((rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE)))) {
        rrdset_flag_clear(st, RRDSET_FLAG_OBSOLETE);
        st->last_accessed_time_s = now_realtime_sec();

        rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_EXPOSED);

        // the chart will be pushed upstream automatically
        // due to data collection
        rrdcontext_updated_rrdset_flags(st);
    }
}

inline void rrdset_update_heterogeneous_flag(RRDSET *st) {
    RRDHOST *host = st->rrdhost;
    (void)host;

    RRDDIM *rd;

    rrdset_flag_clear(st, RRDSET_FLAG_HOMOGENEOUS_CHECK);

    bool init = false, is_heterogeneous = false;
    RRD_ALGORITHM algorithm;
    collected_number multiplier;
    collected_number divisor;

    rrddim_foreach_read(rd, st) {
        if(!init) {
            algorithm = rd->algorithm;
            multiplier = rd->multiplier;
            divisor = ABS(rd->divisor);
            init = true;
            continue;
        }

        if(algorithm != rd->algorithm || multiplier != ABS(rd->multiplier) || divisor != ABS(rd->divisor)) {
            if(!rrdset_flag_check(st, RRDSET_FLAG_HETEROGENEOUS)) {
                #ifdef NETDATA_INTERNAL_CHECKS
                info("Dimension '%s' added on chart '%s' of host '%s' is not homogeneous to other dimensions already present (algorithm is '%s' vs '%s', multiplier is " COLLECTED_NUMBER_FORMAT " vs " COLLECTED_NUMBER_FORMAT ", divisor is " COLLECTED_NUMBER_FORMAT " vs " COLLECTED_NUMBER_FORMAT ").",
                     rrddim_name(rd),
                     rrdset_name(st),
                     rrdhost_hostname(host),
                     rrd_algorithm_name(rd->algorithm), rrd_algorithm_name(algorithm),
                     rd->multiplier, multiplier,
                     rd->divisor, divisor
                );
                #endif
                rrdset_flag_set(st, RRDSET_FLAG_HETEROGENEOUS);
            }

            is_heterogeneous = true;
            break;
        }
    }
    rrddim_foreach_done(rd);

    if(!is_heterogeneous) {
        rrdset_flag_clear(st, RRDSET_FLAG_HETEROGENEOUS);
        rrdcontext_updated_rrdset_flags(st);
    }
}

// ----------------------------------------------------------------------------
// RRDSET - reset a chart

void rrdset_reset(RRDSET *st) {
    debug(D_RRD_CALLS, "rrdset_reset() %s", rrdset_name(st));

    st->last_collected_time.tv_sec = 0;
    st->last_collected_time.tv_usec = 0;
    st->last_updated.tv_sec = 0;
    st->last_updated.tv_usec = 0;
    st->current_entry = 0;
    st->counter = 0;
    st->counter_done = 0;

    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        rd->last_collected_time.tv_sec = 0;
        rd->last_collected_time.tv_usec = 0;
        rd->collections_counter = 0;

        if(!rrddim_flag_check(rd, RRDDIM_FLAG_ARCHIVED)) {
            for(size_t tier = 0; tier < storage_tiers ;tier++)
                storage_engine_store_flush(rd->tiers[tier].db_collection_handle);
        }
    }
    rrddim_foreach_done(rd);
}

// ----------------------------------------------------------------------------
// RRDSET - helpers for rrdset_create()

inline long align_entries_to_pagesize(RRD_MEMORY_MODE mode, long entries) {
    if(mode == RRD_MEMORY_MODE_DBENGINE) return 0;
    if(mode == RRD_MEMORY_MODE_NONE) return 5;

    if(entries < 5) entries = 5;
    if(entries > RRD_HISTORY_ENTRIES_MAX) entries = RRD_HISTORY_ENTRIES_MAX;

    if(mode == RRD_MEMORY_MODE_MAP || mode == RRD_MEMORY_MODE_SAVE || mode == RRD_MEMORY_MODE_RAM) {
        long header_size = 0;

        if(mode == RRD_MEMORY_MODE_MAP || mode == RRD_MEMORY_MODE_SAVE)
            header_size = (long)rrddim_memory_file_header_size();

        long page = (long)sysconf(_SC_PAGESIZE);
        long size = (long)(header_size + entries * sizeof(storage_number));
        if (unlikely(size % page)) {
            size -= (size % page);
            size += page;

            long n = (long)((size - header_size) / sizeof(storage_number));
            return n;
        }
    }

    return entries;
}

static inline void last_collected_time_align(RRDSET *st) {
    st->last_collected_time.tv_sec -= st->last_collected_time.tv_sec % st->update_every;

    if(unlikely(rrdset_flag_check(st, RRDSET_FLAG_STORE_FIRST)))
        st->last_collected_time.tv_usec = 0;
    else
        st->last_collected_time.tv_usec = 500000;
}

static inline void last_updated_time_align(RRDSET *st) {
    st->last_updated.tv_sec -= st->last_updated.tv_sec % st->update_every;
    st->last_updated.tv_usec = 0;
}

// ----------------------------------------------------------------------------
// RRDSET - free a chart

void rrdset_free(RRDSET *st) {
    if(unlikely(!st)) return;
    rrdset_index_del(st->rrdhost, st);
}

void rrdset_save(RRDSET *st) {
    rrdset_memory_file_save(st);

    RRDDIM *rd;
    rrddim_foreach_read(rd, st)
        rrddim_memory_file_save(rd);
    rrddim_foreach_done(rd);
}

void rrdset_delete_files(RRDSET *st) {
    RRDDIM *rd;

    info("Deleting chart '%s' ('%s') from disk...", rrdset_id(st), rrdset_name(st));

    if(st->rrd_memory_mode == RRD_MEMORY_MODE_SAVE || st->rrd_memory_mode == RRD_MEMORY_MODE_MAP) {
        const char *cache_filename = rrdset_cache_filename(st);
        if(cache_filename) {
            info("Deleting chart header file '%s'.", cache_filename);
            if (unlikely(unlink(cache_filename) == -1))
                error("Cannot delete chart header file '%s'", cache_filename);
        }
        else
            error("Cannot find the cache filename of chart '%s'", rrdset_id(st));
    }

    rrddim_foreach_read(rd, st) {
        const char *cache_filename = rrddim_cache_filename(rd);
        if(!cache_filename) continue;

        info("Deleting dimension file '%s'.", cache_filename);
        if(unlikely(unlink(cache_filename) == -1))
            error("Cannot delete dimension file '%s'", cache_filename);
    }
    rrddim_foreach_done(rd);

    if(st->cache_dir)
        recursively_delete_dir(st->cache_dir, "left-over chart");
}

void rrdset_delete_obsolete_dimensions(RRDSET *st) {
    RRDDIM *rd;

    info("Deleting dimensions of chart '%s' ('%s') from disk...", rrdset_id(st), rrdset_name(st));

    rrddim_foreach_read(rd, st) {
        if(rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE)) {
            const char *cache_filename = rrddim_cache_filename(rd);
            if(!cache_filename) continue;
            info("Deleting dimension file '%s'.", cache_filename);
            if(unlikely(unlink(cache_filename) == -1))
                error("Cannot delete dimension file '%s'", cache_filename);
        }
    }
    rrddim_foreach_done(rd);
}

// ----------------------------------------------------------------------------
// RRDSET - create a chart

RRDSET *rrdset_create_custom(
          RRDHOST *host
        , const char *type
        , const char *id
        , const char *name
        , const char *family
        , const char *context
        , const char *title
        , const char *units
        , const char *plugin
        , const char *module
        , long priority
        , int update_every
        , RRDSET_TYPE chart_type
        , RRD_MEMORY_MODE memory_mode
        , long history_entries
) {
    if (host != localhost)
        host->child_last_chart_command = now_realtime_sec();

    if(!type || !type[0])
        fatal("Cannot create rrd stats without a type: id '%s', name '%s', family '%s', context '%s', title '%s', units '%s', plugin '%s', module '%s'."
              , (id && *id)?id:"<unset>"
              , (name && *name)?name:"<unset>"
              , (family && *family)?family:"<unset>"
              , (context && *context)?context:"<unset>"
              , (title && *title)?title:"<unset>"
              , (units && *units)?units:"<unset>"
              , (plugin && *plugin)?plugin:"<unset>"
              , (module && *module)?module:"<unset>"
        );

    if(!id || !id[0])
        fatal("Cannot create rrd stats without an id: type '%s', name '%s', family '%s', context '%s', title '%s', units '%s', plugin '%s', module '%s'."
              , type
              , (name && *name)?name:"<unset>"
              , (family && *family)?family:"<unset>"
              , (context && *context)?context:"<unset>"
              , (title && *title)?title:"<unset>"
              , (units && *units)?units:"<unset>"
              , (plugin && *plugin)?plugin:"<unset>"
              , (module && *module)?module:"<unset>"
        );

    // ------------------------------------------------------------------------
    // check if it already exists

    char full_id[RRD_ID_LENGTH_MAX + 1];
    snprintfz(full_id, RRD_ID_LENGTH_MAX, "%s.%s", type, id);

    // ------------------------------------------------------------------------
    // allocate it

    debug(D_RRD_CALLS, "Creating RRD_STATS for '%s.%s'.", type, id);

    struct rrdset_constructor tmp = {
        .host = host,
        .type = type,
        .id = id,
        .name = name,
        .family = family,
        .context = context,
        .title = title,
        .units = units,
        .plugin = plugin,
        .module = module,
        .priority = priority,
        .update_every = update_every,
        .chart_type = chart_type,
        .memory_mode = memory_mode,
        .history_entries = history_entries,
    };

    RRDSET *st = rrdset_index_add(host, full_id, &tmp);
    return(st);
}

// ----------------------------------------------------------------------------
// RRDSET - data collection iteration control

void rrdset_timed_next(RRDSET *st, struct timeval now, usec_t duration_since_last_update) {
    #ifdef NETDATA_INTERNAL_CHECKS
    char *discard_reason = NULL;
    usec_t discarded = duration_since_last_update;
    #endif

    if(unlikely(rrdset_flag_check(st, RRDSET_FLAG_SYNC_CLOCK))) {
        // the chart needs to be re-synced to current time
        rrdset_flag_clear(st, RRDSET_FLAG_SYNC_CLOCK);

        // discard the duration supplied
        duration_since_last_update = 0;

        #ifdef NETDATA_INTERNAL_CHECKS
        if(!discard_reason) discard_reason = "SYNC CLOCK FLAG";
        #endif
    }

    if(unlikely(!st->last_collected_time.tv_sec)) {
        // the first entry
        duration_since_last_update = st->update_every * USEC_PER_SEC;
        #ifdef NETDATA_INTERNAL_CHECKS
        if(!discard_reason) discard_reason = "FIRST DATA COLLECTION";
        #endif
    }
    else if(unlikely(!duration_since_last_update)) {
        // no dt given by the plugin
        duration_since_last_update = dt_usec(&now, &st->last_collected_time);
        #ifdef NETDATA_INTERNAL_CHECKS
        if(!discard_reason) discard_reason = "NO USEC GIVEN BY COLLECTOR";
        #endif
    }
    else {
        // microseconds has the time since the last collection
        susec_t since_last_usec = dt_usec_signed(&now, &st->last_collected_time);

        if(unlikely(since_last_usec < 0)) {
            // oops! the database is in the future
            #ifdef NETDATA_INTERNAL_CHECKS
            info("RRD database for chart '%s' on host '%s' is %0.5" NETDATA_DOUBLE_MODIFIER
                " secs in the future (counter #%zu, update #%zu). Adjusting it to current time."
                , rrdset_id(st)
                , rrdhost_hostname(st->rrdhost)
                , (NETDATA_DOUBLE)-since_last_usec / USEC_PER_SEC
                , st->counter
                , st->counter_done
                );
            #endif

            duration_since_last_update = 0;

            #ifdef NETDATA_INTERNAL_CHECKS
            if(!discard_reason) discard_reason = "COLLECTION TIME IN FUTURE";
            #endif
        }
        else if(unlikely((usec_t)since_last_usec > (usec_t)(st->update_every * 5 * USEC_PER_SEC))) {
            // oops! the database is too far behind
            #ifdef NETDATA_INTERNAL_CHECKS
            info("RRD database for chart '%s' on host '%s' is %0.5" NETDATA_DOUBLE_MODIFIER
                " secs in the past (counter #%zu, update #%zu). Adjusting it to current time.", rrdset_id(st), rrdhost_hostname(st->rrdhost), (NETDATA_DOUBLE)since_last_usec / USEC_PER_SEC, st->counter, st->counter_done);
            #endif

            duration_since_last_update = (usec_t)since_last_usec;

            #ifdef NETDATA_INTERNAL_CHECKS
            if(!discard_reason) discard_reason = "COLLECTION TIME TOO FAR IN THE PAST";
            #endif
        }

#ifdef NETDATA_INTERNAL_CHECKS
        if(since_last_usec > 0 && (susec_t) duration_since_last_update < since_last_usec) {
            static __thread susec_t min_delta = USEC_PER_SEC * 3600, permanent_min_delta = 0;
            static __thread time_t last_time_s = 0;

            // the first time initialize it so that it will make the check later
            if(last_time_s == 0) last_time_s = now.tv_sec + 60;

            susec_t delta = since_last_usec - (susec_t) duration_since_last_update;
            if(delta < min_delta) min_delta = delta;

            if(now.tv_sec >= last_time_s + 60) {
                last_time_s = now.tv_sec;

                if(min_delta > permanent_min_delta) {
                    info("MINIMUM MICROSECONDS DELTA of thread %d increased from %lld to %lld (+%lld)", gettid(), permanent_min_delta, min_delta, min_delta - permanent_min_delta);
                    permanent_min_delta = min_delta;
                }

                min_delta = USEC_PER_SEC * 3600;
            }
        }
#endif
    }

    debug(D_RRD_CALLS, "rrdset_timed_next() for chart %s with duration since last update %llu usec", rrdset_name(st), duration_since_last_update);
    rrdset_debug(st, "NEXT: %llu microseconds", duration_since_last_update);

    internal_error(discarded && discarded != duration_since_last_update,
                   "host '%s', chart '%s': discarded data collection time of %llu usec, "
                   "replaced with %llu usec, reason: '%s'"
                   , rrdhost_hostname(st->rrdhost)
                   , rrdset_id(st)
                   , discarded
                   , duration_since_last_update
                   , discard_reason?discard_reason:"UNDEFINED"
                   );

    st->usec_since_last_update = duration_since_last_update;
}

inline void rrdset_next_usec_unfiltered(RRDSET *st, usec_t duration_since_last_update) {
    if(unlikely(!st->last_collected_time.tv_sec || !duration_since_last_update || (rrdset_flag_check(st, RRDSET_FLAG_SYNC_CLOCK)))) {
        // call the full next_usec() function
        rrdset_next_usec(st, duration_since_last_update);
        return;
    }

    st->usec_since_last_update = duration_since_last_update;
}

inline void rrdset_next_usec(RRDSET *st, usec_t duration_since_last_update) {
    struct timeval now;

    now_realtime_timeval(&now);
    rrdset_timed_next(st, now, duration_since_last_update);
}

// ----------------------------------------------------------------------------
// RRDSET - process the collected values for all dimensions of a chart

static inline usec_t rrdset_init_last_collected_time(RRDSET *st, struct timeval now) {
    st->last_collected_time = now;
    last_collected_time_align(st);

    usec_t last_collect_ut = st->last_collected_time.tv_sec * USEC_PER_SEC + st->last_collected_time.tv_usec;

    rrdset_debug(st, "initialized last collected time to %0.3" NETDATA_DOUBLE_MODIFIER, (NETDATA_DOUBLE)last_collect_ut / USEC_PER_SEC);

    return last_collect_ut;
}

static inline usec_t rrdset_update_last_collected_time(RRDSET *st) {
    usec_t last_collect_ut = st->last_collected_time.tv_sec * USEC_PER_SEC + st->last_collected_time.tv_usec;
    usec_t ut = last_collect_ut + st->usec_since_last_update;
    st->last_collected_time.tv_sec = (time_t) (ut / USEC_PER_SEC);
    st->last_collected_time.tv_usec = (suseconds_t) (ut % USEC_PER_SEC);

    rrdset_debug(st, "updated last collected time to %0.3" NETDATA_DOUBLE_MODIFIER, (NETDATA_DOUBLE)last_collect_ut / USEC_PER_SEC);

    return last_collect_ut;
}

static inline void rrdset_init_last_updated_time(RRDSET *st) {
    // copy the last collected time to last updated time
    st->last_updated.tv_sec  = st->last_collected_time.tv_sec;
    st->last_updated.tv_usec = st->last_collected_time.tv_usec;

    if(rrdset_flag_check(st, RRDSET_FLAG_STORE_FIRST))
        st->last_updated.tv_sec -= st->update_every;

    last_updated_time_align(st);
}

static __thread size_t rrdset_done_statistics_points_stored_per_tier[RRD_STORAGE_TIERS];

static inline time_t tier_next_point_time_s(RRDDIM *rd, struct rrddim_tier *t, time_t now_s) {
    time_t loop = (time_t)rd->update_every * (time_t)t->tier_grouping;
    return now_s + loop - ((now_s + loop) % loop);
}

void store_metric_at_tier(RRDDIM *rd, size_t tier, struct rrddim_tier *t, STORAGE_POINT sp, usec_t now_ut __maybe_unused) {
    if (unlikely(!t->next_point_end_time_s))
        t->next_point_end_time_s = tier_next_point_time_s(rd, t, sp.end_time_s);

    if(unlikely(sp.start_time_s >= t->next_point_end_time_s)) {
        // flush the virtual point, it is done

        if (likely(!storage_point_is_unset(t->virtual_point))) {

            storage_engine_store_metric(
                t->db_collection_handle,
                t->next_point_end_time_s * USEC_PER_SEC,
                t->virtual_point.sum,
                t->virtual_point.min,
                t->virtual_point.max,
                t->virtual_point.count,
                t->virtual_point.anomaly_count,
                t->virtual_point.flags);
        }
        else {
            storage_engine_store_metric(
                t->db_collection_handle,
                t->next_point_end_time_s * USEC_PER_SEC,
                NAN,
                NAN,
                NAN,
                0,
                0, SN_FLAG_NONE);
        }

        rrdset_done_statistics_points_stored_per_tier[tier]++;
        t->virtual_point.count = 0; // make the point unset
        t->next_point_end_time_s = tier_next_point_time_s(rd, t, sp.end_time_s);
    }

    // merge the dates into our virtual point
    if (unlikely(sp.start_time_s < t->virtual_point.start_time_s))
        t->virtual_point.start_time_s = sp.start_time_s;

    if (likely(sp.end_time_s > t->virtual_point.end_time_s))
        t->virtual_point.end_time_s = sp.end_time_s;

    // merge the values into our virtual point
    if (likely(!storage_point_is_gap(sp))) {
        // we aggregate only non NULLs into higher tiers

        if (likely(!storage_point_is_unset(t->virtual_point))) {
            // merge the collected point to our virtual one
            t->virtual_point.sum += sp.sum;
            t->virtual_point.min = MIN(t->virtual_point.min, sp.min);
            t->virtual_point.max = MAX(t->virtual_point.max, sp.max);
            t->virtual_point.count += sp.count;
            t->virtual_point.anomaly_count += sp.anomaly_count;
            t->virtual_point.flags |= sp.flags;
        }
        else {
            // reset our virtual point to this one
            t->virtual_point = sp;
        }
    }
}
#ifdef NETDATA_LOG_COLLECTION_ERRORS
void rrddim_store_metric_with_trace(RRDDIM *rd, usec_t point_end_time_ut, NETDATA_DOUBLE n, SN_FLAGS flags, const char *function) {
#else // !NETDATA_LOG_COLLECTION_ERRORS
void rrddim_store_metric(RRDDIM *rd, usec_t point_end_time_ut, NETDATA_DOUBLE n, SN_FLAGS flags) {
#endif // !NETDATA_LOG_COLLECTION_ERRORS
#ifdef NETDATA_LOG_COLLECTION_ERRORS
    rd->rrddim_store_metric_count++;

    if(likely(rd->rrddim_store_metric_count > 1)) {
        usec_t expected = rd->rrddim_store_metric_last_ut + rd->update_every * USEC_PER_SEC;

        if(point_end_time_ut != rd->rrddim_store_metric_last_ut) {
            internal_error(true,
                           "%s COLLECTION: 'host:%s/chart:%s/dim:%s' granularity %d, collection %zu, expected to store at tier 0 a value at %llu, but it gave %llu [%s%llu usec] (called from %s(), previously by %s())",
                           (point_end_time_ut < rd->rrddim_store_metric_last_ut) ? "**PAST**" : "GAP",
                           rrdhost_hostname(rd->rrdset->rrdhost), rrdset_id(rd->rrdset), rrddim_id(rd),
                           rd->update_every,
                           rd->rrddim_store_metric_count,
                           expected, point_end_time_ut,
                           (point_end_time_ut < rd->rrddim_store_metric_last_ut)?"by -" : "gap ",
                           expected - point_end_time_ut,
                           function,
                           rd->rrddim_store_metric_last_caller?rd->rrddim_store_metric_last_caller:"none");
        }
    }

    rd->rrddim_store_metric_last_ut = point_end_time_ut;
    rd->rrddim_store_metric_last_caller = function;
#endif // NETDATA_LOG_COLLECTION_ERRORS

    // store the metric on tier 0
    storage_engine_store_metric(rd->tiers[0].db_collection_handle, point_end_time_ut,
                                n, 0, 0,
                                1, 0, flags);

    rrdset_done_statistics_points_stored_per_tier[0]++;

    time_t now_s = (time_t)(point_end_time_ut / USEC_PER_SEC);

    STORAGE_POINT sp = {
        .start_time_s = now_s - rd->update_every,
        .end_time_s = now_s,
        .min = n,
        .max = n,
        .sum = n,
        .count = 1,
        .anomaly_count = (flags & SN_FLAG_NOT_ANOMALOUS) ? 0 : 1,
        .flags = flags
    };

    for(size_t tier = 1; tier < storage_tiers ;tier++) {
        if(unlikely(!rd->tiers[tier].db_metric_handle)) continue;

        struct rrddim_tier *t = &rd->tiers[tier];

        if(!rrddim_option_check(rd, RRDDIM_OPTION_BACKFILLED_HIGH_TIERS)) {
            // we have not collected this tier before
            // let's fill any gap that may exist
            rrdr_fill_tier_gap_from_smaller_tiers(rd, tier, now_s);
            rrddim_option_set(rd, RRDDIM_OPTION_BACKFILLED_HIGH_TIERS);
        }

        store_metric_at_tier(rd, tier, t, sp, point_end_time_ut);
    }

    rrdcontext_collected_rrddim(rd);
}

void store_metric_collection_completed() {
    global_statistics_rrdset_done_chart_collection_completed(rrdset_done_statistics_points_stored_per_tier);
}

// caching of dimensions rrdset_done() and rrdset_done_interpolate() loop through
struct rda_item {
    const DICTIONARY_ITEM *item;
    RRDDIM *rd;
};

static __thread struct rda_item *thread_rda = NULL;
static __thread size_t thread_rda_entries = 0;

struct rda_item *rrdset_thread_rda_get(size_t *dimensions) {

    if(unlikely(!thread_rda || (*dimensions) > thread_rda_entries)) {
        size_t old_mem = thread_rda_entries * sizeof(struct rda_item);
        freez(thread_rda);
        thread_rda_entries = *dimensions;
        size_t new_mem = thread_rda_entries * sizeof(struct rda_item);
        thread_rda = mallocz(new_mem);

        __atomic_add_fetch(&netdata_buffers_statistics.rrdset_done_rda_size, new_mem - old_mem, __ATOMIC_RELAXED);
    }

    *dimensions = thread_rda_entries;
    return thread_rda;
}

void rrdset_thread_rda_free(void) {
    __atomic_sub_fetch(&netdata_buffers_statistics.rrdset_done_rda_size, thread_rda_entries * sizeof(struct rda_item), __ATOMIC_RELAXED);

    freez(thread_rda);
    thread_rda = NULL;
    thread_rda_entries = 0;
}

static inline size_t rrdset_done_interpolate(
        RRDSET_STREAM_BUFFER *rsb
        , RRDSET *st
        , struct rda_item *rda_base
        , size_t rda_slots
        , usec_t update_every_ut
        , usec_t last_stored_ut
        , usec_t next_store_ut
        , usec_t last_collect_ut
        , usec_t now_collect_ut
        , char store_this_entry
        , uint32_t has_reset_value
) {
    RRDDIM *rd;

    size_t stored_entries = 0;     // the number of entries we have stored in the db, during this call to rrdset_done()

    usec_t first_ut = last_stored_ut, last_ut = 0;
    (void)first_ut;

    ssize_t iterations = (ssize_t)((now_collect_ut - last_stored_ut) / (update_every_ut));
    if((now_collect_ut % (update_every_ut)) == 0) iterations++;

    size_t counter = st->counter;
    long current_entry = st->current_entry;

    SN_FLAGS storage_flags = SN_DEFAULT_FLAGS;

    if (has_reset_value)
        storage_flags |= SN_FLAG_RESET;

    for( ; next_store_ut <= now_collect_ut ; last_collect_ut = next_store_ut, next_store_ut += update_every_ut, iterations-- ) {

        internal_error(iterations < 0,
                       "RRDSET: '%s': iterations calculation wrapped! "
                       "first_ut = %llu, last_stored_ut = %llu, next_store_ut = %llu, now_collect_ut = %llu"
                       , rrdset_id(st)
                       , first_ut
                       , last_stored_ut
                       , next_store_ut
                       , now_collect_ut
                       );

        rrdset_debug(st, "last_stored_ut = %0.3" NETDATA_DOUBLE_MODIFIER " (last updated time)", (NETDATA_DOUBLE)last_stored_ut/USEC_PER_SEC);
        rrdset_debug(st, "next_store_ut  = %0.3" NETDATA_DOUBLE_MODIFIER " (next interpolation point)", (NETDATA_DOUBLE)next_store_ut/USEC_PER_SEC);

        last_ut = next_store_ut;

        ml_chart_update_begin(st);

        struct rda_item *rda;
        size_t dim_id;
        for(dim_id = 0, rda = rda_base ; dim_id < rda_slots ; ++dim_id, ++rda) {
            rd = rda->rd;
            if(unlikely(!rd)) continue;

            NETDATA_DOUBLE new_value;

            switch(rd->algorithm) {
                case RRD_ALGORITHM_INCREMENTAL:
                    new_value = (NETDATA_DOUBLE)
                            (      rd->calculated_value
                                   * (NETDATA_DOUBLE)(next_store_ut - last_collect_ut)
                                   / (NETDATA_DOUBLE)(now_collect_ut - last_collect_ut)
                            );

                    rrdset_debug(st, "%s: CALC2 INC " NETDATA_DOUBLE_FORMAT " = "
                                 NETDATA_DOUBLE_FORMAT
                                " * (%llu - %llu)"
                                " / (%llu - %llu)"
                              , rrddim_name(rd)
                              , new_value
                              , rd->calculated_value
                              , next_store_ut, last_collect_ut
                              , now_collect_ut, last_collect_ut
                    );

                    rd->calculated_value -= new_value;
                    new_value += rd->last_calculated_value;
                    rd->last_calculated_value = 0;
                    new_value /= (NETDATA_DOUBLE)st->update_every;

                    if(unlikely(next_store_ut - last_stored_ut < update_every_ut)) {

                        rrdset_debug(st, "%s: COLLECTION POINT IS SHORT " NETDATA_DOUBLE_FORMAT " - EXTRAPOLATING",
                                    rrddim_name(rd)
                                  , (NETDATA_DOUBLE)(next_store_ut - last_stored_ut)
                        );

                        new_value = new_value * (NETDATA_DOUBLE)(st->update_every * USEC_PER_SEC) / (NETDATA_DOUBLE)(next_store_ut - last_stored_ut);
                    }
                    break;

                case RRD_ALGORITHM_ABSOLUTE:
                case RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL:
                case RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL:
                default:
                    if(iterations == 1) {
                        // this is the last iteration
                        // do not interpolate
                        // just show the calculated value

                        new_value = rd->calculated_value;
                    }
                    else {
                        // we have missed an update
                        // interpolate in the middle values

                        new_value = (NETDATA_DOUBLE)
                                (   (     (rd->calculated_value - rd->last_calculated_value)
                                          * (NETDATA_DOUBLE)(next_store_ut - last_collect_ut)
                                          / (NETDATA_DOUBLE)(now_collect_ut - last_collect_ut)
                                    )
                                    +  rd->last_calculated_value
                                );

                        rrdset_debug(st, "%s: CALC2 DEF " NETDATA_DOUBLE_FORMAT " = ((("
                                            "(" NETDATA_DOUBLE_FORMAT " - " NETDATA_DOUBLE_FORMAT ")"
                                            " * %llu"
                                            " / %llu) + " NETDATA_DOUBLE_FORMAT, rrddim_name(rd)
                                  , new_value
                                  , rd->calculated_value, rd->last_calculated_value
                                  , (next_store_ut - first_ut)
                                  , (now_collect_ut - first_ut), rd->last_calculated_value
                        );
                    }
                    break;
            }

            time_t current_time_s = (time_t) (next_store_ut / USEC_PER_SEC);

            if(unlikely(!store_this_entry)) {
                (void) ml_dimension_is_anomalous(rd, current_time_s, 0, false);

                if(rsb->wb && rsb->v2)
                    rrddim_push_metrics_v2(rsb, rd, next_store_ut, NAN, SN_FLAG_NONE);

                rrddim_store_metric(rd, next_store_ut, NAN, SN_FLAG_NONE);
                continue;
            }

            if(likely(rd->updated && rd->collections_counter > 1 && iterations < gap_when_lost_iterations_above)) {
                uint32_t dim_storage_flags = storage_flags;

                if (ml_dimension_is_anomalous(rd, current_time_s, new_value, true)) {
                    // clear anomaly bit: 0 -> is anomalous, 1 -> not anomalous
                    dim_storage_flags &= ~((storage_number)SN_FLAG_NOT_ANOMALOUS);
                }

                if(rsb->wb && rsb->v2)
                    rrddim_push_metrics_v2(rsb, rd, next_store_ut, new_value, dim_storage_flags);

                rrddim_store_metric(rd, next_store_ut, new_value, dim_storage_flags);
                rd->last_stored_value = new_value;
            }
            else {
                (void) ml_dimension_is_anomalous(rd, current_time_s, 0, false);

                rrdset_debug(st, "%s: STORE[%ld] = NON EXISTING ", rrddim_name(rd), current_entry);

                if(rsb->wb && rsb->v2)
                    rrddim_push_metrics_v2(rsb, rd, next_store_ut, NAN, SN_FLAG_NONE);

                rrddim_store_metric(rd, next_store_ut, NAN, SN_FLAG_NONE);
                rd->last_stored_value = NAN;
            }

            stored_entries++;
        }

        ml_chart_update_end(st);

        // reset the storage flags for the next point, if any;
        storage_flags = SN_DEFAULT_FLAGS;

        st->counter = ++counter;
        st->current_entry = current_entry = ((current_entry + 1) >= st->entries) ? 0 : current_entry + 1;

        st->last_updated.tv_sec = (time_t) (last_ut / USEC_PER_SEC);
        st->last_updated.tv_usec = 0;

        last_stored_ut = next_store_ut;
    }

/*
    st->counter = counter;
    st->current_entry = current_entry;

    if(likely(last_ut)) {
        st->last_updated.tv_sec = (time_t) (last_ut / USEC_PER_SEC);
        st->last_updated.tv_usec = 0;
    }
*/

    return stored_entries;
}

void rrdset_done(RRDSET *st) {
    struct timeval now;

    now_realtime_timeval(&now);
    rrdset_timed_done(st, now, /* pending_rrdset_next = */ st->counter_done != 0);
}

void rrdset_timed_done(RRDSET *st, struct timeval now, bool pending_rrdset_next) {
    if(unlikely(!service_running(SERVICE_COLLECTORS))) return;

    RRDSET_STREAM_BUFFER stream_buffer = { .wb = NULL, };
    if(unlikely(rrdhost_has_rrdpush_sender_enabled(st->rrdhost)))
        stream_buffer = rrdset_push_metric_initialize(st, now.tv_sec);

    netdata_spinlock_lock(&st->data_collection_lock);

    if (pending_rrdset_next)
        rrdset_timed_next(st, now, 0ULL);

    debug(D_RRD_CALLS, "rrdset_done() for chart '%s'", rrdset_name(st));

    RRDDIM *rd;

    char
            store_this_entry = 1,   // boolean: 1 = store this entry, 0 = don't store this entry
            first_entry = 0;        // boolean: 1 = this is the first entry seen for this chart, 0 = all other entries

    usec_t
            last_collect_ut = 0,    // the timestamp in microseconds, of the last collected value
            now_collect_ut = 0,     // the timestamp in microseconds, of this collected value (this is NOW)
            last_stored_ut = 0,     // the timestamp in microseconds, of the last stored entry in the db
            next_store_ut = 0,      // the timestamp in microseconds, of the next entry to store in the db
            update_every_ut = st->update_every * USEC_PER_SEC; // st->update_every in microseconds

    RRDSET_FLAGS rrdset_flags = rrdset_flag_check(st, ~0);
    if(unlikely(rrdset_flags & RRDSET_FLAG_COLLECTION_FINISHED)) {
        netdata_spinlock_unlock(&st->data_collection_lock);
        return;
    }

    if (unlikely(rrdset_flags & RRDSET_FLAG_OBSOLETE)) {
        error("Chart '%s' has the OBSOLETE flag set, but it is collected.", rrdset_id(st));
        rrdset_isnot_obsolete(st);
    }

    // check if the chart has a long time to be updated
    if(unlikely(st->usec_since_last_update > MAX(st->entries, 60) * update_every_ut)) {
        info("host '%s', chart '%s': took too long to be updated (counter #%zu, update #%zu, %0.3" NETDATA_DOUBLE_MODIFIER
            " secs). Resetting it.", rrdhost_hostname(st->rrdhost), rrdset_id(st), st->counter, st->counter_done, (NETDATA_DOUBLE)st->usec_since_last_update / USEC_PER_SEC);
        rrdset_reset(st);
        st->usec_since_last_update = update_every_ut;
        store_this_entry = 0;
        first_entry = 1;
    }

    rrdset_debug(st, "microseconds since last update: %llu", st->usec_since_last_update);

    // set last_collected_time
    if(unlikely(!st->last_collected_time.tv_sec)) {
        // it is the first entry
        // set the last_collected_time to now
        last_collect_ut = rrdset_init_last_collected_time(st, now) - update_every_ut;

        // the first entry should not be stored
        store_this_entry = 0;
        first_entry = 1;
    }
    else {
        // it is not the first entry
        // calculate the proper last_collected_time, using usec_since_last_update
        last_collect_ut = rrdset_update_last_collected_time(st);
    }

    // if this set has not been updated in the past
    // we fake the last_update time to be = now - usec_since_last_update
    if(unlikely(!st->last_updated.tv_sec)) {
        // it has never been updated before
        // set a fake last_updated, in the past using usec_since_last_update
        rrdset_init_last_updated_time(st);

        // the first entry should not be stored
        store_this_entry = 0;
        first_entry = 1;
    }

    // check if we will re-write the entire data set
    if(unlikely(dt_usec(&st->last_collected_time, &st->last_updated) > st->entries * update_every_ut &&
                st->rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE)) {
        info(
            "'%s': too old data (last updated at %"PRId64".%"PRId64", last collected at %"PRId64".%"PRId64"). "
            "Resetting it. Will not store the next entry.",
            rrdset_id(st),
            (int64_t)st->last_updated.tv_sec,
            (int64_t)st->last_updated.tv_usec,
            (int64_t)st->last_collected_time.tv_sec,
            (int64_t)st->last_collected_time.tv_usec);
        rrdset_reset(st);
        rrdset_init_last_updated_time(st);

        st->usec_since_last_update = update_every_ut;

        // the first entry should not be stored
        store_this_entry = 0;
        first_entry = 1;
    }

    // these are the 3 variables that will help us in interpolation
    // last_stored_ut = the last time we added a value to the storage
    // now_collect_ut = the time the current value has been collected
    // next_store_ut  = the time of the next interpolation point
    now_collect_ut = st->last_collected_time.tv_sec * USEC_PER_SEC + st->last_collected_time.tv_usec;
    last_stored_ut = st->last_updated.tv_sec * USEC_PER_SEC + st->last_updated.tv_usec;
    next_store_ut  = (st->last_updated.tv_sec + st->update_every) * USEC_PER_SEC;

    if(unlikely(!st->counter_done)) {
        // set a fake last_updated to jump to current time
        rrdset_init_last_updated_time(st);

        last_stored_ut = st->last_updated.tv_sec * USEC_PER_SEC + st->last_updated.tv_usec;
        next_store_ut  = (st->last_updated.tv_sec + st->update_every) * USEC_PER_SEC;

        if(unlikely(rrdset_flags & RRDSET_FLAG_STORE_FIRST)) {
            store_this_entry = 1;
            last_collect_ut = next_store_ut - update_every_ut;

            rrdset_debug(st, "Fixed first entry.");
        }
        else {
            store_this_entry = 0;

            rrdset_debug(st, "Will not store the next entry.");
        }
    }

    st->counter_done++;

    if(stream_buffer.wb && !stream_buffer.v2)
        rrdset_push_metrics_v1(&stream_buffer, st);

    uint32_t has_reset_value = 0;

    size_t rda_slots = dictionary_entries(st->rrddim_root_index);
    struct rda_item *rda_base = rrdset_thread_rda_get(&rda_slots);

    size_t dim_id;
    size_t dimensions = 0;
    struct rda_item *rda = rda_base;
    total_number collected_total = 0;
    total_number last_collected_total = 0;
    rrddim_foreach_read(rd, st) {
        if(rd_dfe.counter >= rda_slots)
            break;

        rda = &rda_base[dimensions++];

        if(rrddim_flag_check(rd, RRDDIM_FLAG_ARCHIVED)) {
            rda->item = NULL;
            rda->rd = NULL;
            continue;
        }

        // store the dimension in the array
        rda->item = dictionary_acquired_item_dup(st->rrddim_root_index, rd_dfe.item);
        rda->rd = dictionary_acquired_item_value(rda->item);

        // calculate totals
        if(likely(rd->updated)) {
            // if the new is smaller than the old (an overflow, or reset), set the old equal to the new
            // to reset the calculation (it will give zero as the calculation for this second)
            if(unlikely(rd->algorithm == RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL && rd->last_collected_value > rd->collected_value)) {
                debug(D_RRD_STATS, "'%s' / '%s': RESET or OVERFLOW. Last collected value = " COLLECTED_NUMBER_FORMAT ", current = " COLLECTED_NUMBER_FORMAT
                , rrdset_id(st)
                , rrddim_name(rd)
                , rd->last_collected_value
                , rd->collected_value
                );

                if(!(rrddim_option_check(rd, RRDDIM_OPTION_DONT_DETECT_RESETS_OR_OVERFLOWS)))
                    has_reset_value = 1;

                rd->last_collected_value = rd->collected_value;
            }

            last_collected_total += rd->last_collected_value;
            collected_total += rd->collected_value;

            if(unlikely(rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE))) {
                error("Dimension %s in chart '%s' has the OBSOLETE flag set, but it is collected.", rrddim_name(rd), rrdset_id(st));
                rrddim_isnot_obsolete(st, rd);
            }
        }
    }
    rrddim_foreach_done(rd);
    rda_slots = dimensions;

    rrdset_debug(st, "last_collect_ut = %0.3" NETDATA_DOUBLE_MODIFIER " (last collection time)", (NETDATA_DOUBLE)last_collect_ut/USEC_PER_SEC);
    rrdset_debug(st, "now_collect_ut  = %0.3" NETDATA_DOUBLE_MODIFIER " (current collection time)", (NETDATA_DOUBLE)now_collect_ut/USEC_PER_SEC);
    rrdset_debug(st, "last_stored_ut  = %0.3" NETDATA_DOUBLE_MODIFIER " (last updated time)", (NETDATA_DOUBLE)last_stored_ut/USEC_PER_SEC);
    rrdset_debug(st, "next_store_ut   = %0.3" NETDATA_DOUBLE_MODIFIER " (next interpolation point)", (NETDATA_DOUBLE)next_store_ut/USEC_PER_SEC);

    // process all dimensions to calculate their values
    // based on the collected figures only
    // at this stage we do not interpolate anything
    for(dim_id = 0, rda = rda_base ; dim_id < rda_slots ; ++dim_id, ++rda) {
        rd = rda->rd;
        if(unlikely(!rd)) continue;

        if(unlikely(!rd->updated)) {
            rd->calculated_value = 0;
            continue;
        }

        rrdset_debug(st, "%s: START "
                " last_collected_value = " COLLECTED_NUMBER_FORMAT
                " collected_value = " COLLECTED_NUMBER_FORMAT
                " last_calculated_value = " NETDATA_DOUBLE_FORMAT
                " calculated_value = " NETDATA_DOUBLE_FORMAT
                     , rrddim_name(rd)
                     , rd->last_collected_value
                     , rd->collected_value
                     , rd->last_calculated_value
                     , rd->calculated_value
        );

        switch(rd->algorithm) {
            case RRD_ALGORITHM_ABSOLUTE:
                rd->calculated_value = (NETDATA_DOUBLE)rd->collected_value
                                       * (NETDATA_DOUBLE)rd->multiplier
                                       / (NETDATA_DOUBLE)rd->divisor;

                rrdset_debug(st, "%s: CALC ABS/ABS-NO-IN " NETDATA_DOUBLE_FORMAT " = "
                            COLLECTED_NUMBER_FORMAT
                            " * " NETDATA_DOUBLE_FORMAT
                            " / " NETDATA_DOUBLE_FORMAT
                          , rrddim_name(rd)
                          , rd->calculated_value
                          , rd->collected_value
                          , (NETDATA_DOUBLE)rd->multiplier
                          , (NETDATA_DOUBLE)rd->divisor
                );
                break;

            case RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL:
                if(unlikely(!collected_total))
                    rd->calculated_value = 0;
                else
                    // the percentage of the current value
                    // over the total of all dimensions
                    rd->calculated_value =
                            (NETDATA_DOUBLE)100
                            * (NETDATA_DOUBLE)rd->collected_value
                            / (NETDATA_DOUBLE)collected_total;

                rrdset_debug(st, "%s: CALC PCENT-ROW " NETDATA_DOUBLE_FORMAT " = 100"
                            " * " COLLECTED_NUMBER_FORMAT
                            " / " COLLECTED_NUMBER_FORMAT
                          , rrddim_name(rd)
                          , rd->calculated_value
                          , rd->collected_value
                          , collected_total
                );
                break;

            case RRD_ALGORITHM_INCREMENTAL:
                if(unlikely(rd->collections_counter <= 1)) {
                    rd->calculated_value = 0;
                    continue;
                }

                // If the new is smaller than the old (an overflow, or reset), set the old equal to the new
                // to reset the calculation (it will give zero as the calculation for this second).
                // It is imperative to set the comparison to uint64_t since type collected_number is signed and
                // produces wrong results as far as incremental counters are concerned.
                if(unlikely((uint64_t)rd->last_collected_value > (uint64_t)rd->collected_value)) {
                    debug(D_RRD_STATS, "'%s' / '%s': RESET or OVERFLOW. Last collected value = " COLLECTED_NUMBER_FORMAT ", current = " COLLECTED_NUMBER_FORMAT
                          , rrdset_id(st)
                          , rrddim_name(rd)
                          , rd->last_collected_value
                          , rd->collected_value);

                    if(!(rrddim_option_check(rd, RRDDIM_OPTION_DONT_DETECT_RESETS_OR_OVERFLOWS)))
                        has_reset_value = 1;

                    uint64_t last = (uint64_t)rd->last_collected_value;
                    uint64_t new = (uint64_t)rd->collected_value;
                    uint64_t max = (uint64_t)rd->collected_value_max;
                    uint64_t cap = 0;

                    // Signed values are handled by exploiting two's complement which will produce positive deltas
                    if (max > 0x00000000FFFFFFFFULL)
                        cap = 0xFFFFFFFFFFFFFFFFULL; // handles signed and unsigned 64-bit counters
                    else
                        cap = 0x00000000FFFFFFFFULL; // handles signed and unsigned 32-bit counters

                    uint64_t delta = cap - last + new;
                    uint64_t max_acceptable_rate = (cap / 100) * MAX_INCREMENTAL_PERCENT_RATE;

                    // If the delta is less than the maximum acceptable rate and the previous value was near the cap
                    // then this is an overflow. There can be false positives such that a reset is detected as an
                    // overflow.
                    // TODO: remember recent history of rates and compare with current rate to reduce this chance.
                    if (delta < max_acceptable_rate) {
                        rd->calculated_value +=
                                (NETDATA_DOUBLE) delta
                                * (NETDATA_DOUBLE) rd->multiplier
                                / (NETDATA_DOUBLE) rd->divisor;
                    } else {
                        // This is a reset. Any overflow with a rate greater than MAX_INCREMENTAL_PERCENT_RATE will also
                        // be detected as a reset instead.
                        rd->calculated_value += (NETDATA_DOUBLE)0;
                    }
                }
                else {
                    rd->calculated_value +=
                            (NETDATA_DOUBLE) (rd->collected_value - rd->last_collected_value)
                            * (NETDATA_DOUBLE) rd->multiplier
                            / (NETDATA_DOUBLE) rd->divisor;
                }

                rrdset_debug(st, "%s: CALC INC PRE " NETDATA_DOUBLE_FORMAT " = ("
                            COLLECTED_NUMBER_FORMAT " - " COLLECTED_NUMBER_FORMAT
                            ")"
                                    " * " NETDATA_DOUBLE_FORMAT
                            " / " NETDATA_DOUBLE_FORMAT
                          , rrddim_name(rd)
                          , rd->calculated_value
                          , rd->collected_value, rd->last_collected_value
                          , (NETDATA_DOUBLE)rd->multiplier
                          , (NETDATA_DOUBLE)rd->divisor
                );
                break;

            case RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL:
                if(unlikely(rd->collections_counter <= 1)) {
                    rd->calculated_value = 0;
                    continue;
                }

                // the percentage of the current increment
                // over the increment of all dimensions together
                if(unlikely(collected_total == last_collected_total))
                    rd->calculated_value = 0;
                else
                    rd->calculated_value =
                            (NETDATA_DOUBLE)100
                            * (NETDATA_DOUBLE)(rd->collected_value - rd->last_collected_value)
                            / (NETDATA_DOUBLE)(collected_total - last_collected_total);

                rrdset_debug(st, "%s: CALC PCENT-DIFF " NETDATA_DOUBLE_FORMAT " = 100"
                            " * (" COLLECTED_NUMBER_FORMAT " - " COLLECTED_NUMBER_FORMAT ")"
                            " / (" COLLECTED_NUMBER_FORMAT " - " COLLECTED_NUMBER_FORMAT ")"
                          , rrddim_name(rd)
                          , rd->calculated_value
                          , rd->collected_value, rd->last_collected_value
                          , collected_total, last_collected_total
                );
                break;

            default:
                // make the default zero, to make sure
                // it gets noticed when we add new types
                rd->calculated_value = 0;

                rrdset_debug(st, "%s: CALC " NETDATA_DOUBLE_FORMAT " = 0"
                          , rrddim_name(rd)
                          , rd->calculated_value
                );
                break;
        }

        rrdset_debug(st, "%s: PHASE2 "
                    " last_collected_value = " COLLECTED_NUMBER_FORMAT
                    " collected_value = " COLLECTED_NUMBER_FORMAT
                    " last_calculated_value = " NETDATA_DOUBLE_FORMAT
                    " calculated_value = " NETDATA_DOUBLE_FORMAT
                    , rrddim_name(rd)
                    , rd->last_collected_value
                    , rd->collected_value
                    , rd->last_calculated_value
                    , rd->calculated_value
        );
    }

    // at this point we have all the calculated values ready
    // it is now time to interpolate values on a second boundary

// #ifdef NETDATA_INTERNAL_CHECKS
//     if(unlikely(now_collect_ut < next_store_ut && st->counter_done > 1)) {
//         // this is collected in the same interpolation point
//         rrdset_debug(st, "THIS IS IN THE SAME INTERPOLATION POINT");
//         info("INTERNAL CHECK: host '%s', chart '%s' collection %zu is in the same interpolation point: short by %llu microseconds", st->rrdhost->hostname, rrdset_name(st), st->counter_done, next_store_ut - now_collect_ut);
//     }
// #endif

    rrdset_done_interpolate(
            &stream_buffer
            , st
            , rda_base
            , rda_slots
            , update_every_ut
            , last_stored_ut
            , next_store_ut
            , last_collect_ut
            , now_collect_ut
            , store_this_entry
            , has_reset_value
    );

    for(dim_id = 0, rda = rda_base ; dim_id < rda_slots ; ++dim_id, ++rda) {
        rd = rda->rd;
        if(unlikely(!rd)) continue;

        if(unlikely(!rd->updated))
            continue;

        rrdset_debug(st, "%s: setting last_collected_value (old: " COLLECTED_NUMBER_FORMAT ") to last_collected_value (new: " COLLECTED_NUMBER_FORMAT ")", rrddim_name(rd), rd->last_collected_value, rd->collected_value);

        rd->last_collected_value = rd->collected_value;

        switch(rd->algorithm) {
            case RRD_ALGORITHM_INCREMENTAL:
                if(unlikely(!first_entry)) {
                    rrdset_debug(st, "%s: setting last_calculated_value (old: " NETDATA_DOUBLE_FORMAT ") to "
                                     "last_calculated_value (new: " NETDATA_DOUBLE_FORMAT ")"
                        , rrddim_name(rd)
                        , rd->last_calculated_value + rd->calculated_value
                        , rd->calculated_value);

                    rd->last_calculated_value += rd->calculated_value;
                }
                else {
                    rrdset_debug(st, "THIS IS THE FIRST POINT");
                }
                break;

            case RRD_ALGORITHM_ABSOLUTE:
            case RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL:
            case RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL:
                rrdset_debug(st, "%s: setting last_calculated_value (old: " NETDATA_DOUBLE_FORMAT ") to "
                                 "last_calculated_value (new: " NETDATA_DOUBLE_FORMAT ")"
                    , rrddim_name(rd)
                    , rd->last_calculated_value
                    , rd->calculated_value);

                rd->last_calculated_value = rd->calculated_value;
                break;
        }

        rd->calculated_value = 0;
        rd->collected_value = 0;
        rd->updated = 0;

        rrdset_debug(st, "%s: END "
                    " last_collected_value = " COLLECTED_NUMBER_FORMAT
                    " collected_value = " COLLECTED_NUMBER_FORMAT
                    " last_calculated_value = " NETDATA_DOUBLE_FORMAT
                    " calculated_value = " NETDATA_DOUBLE_FORMAT
                    , rrddim_name(rd)
                    , rd->last_collected_value
                    , rd->collected_value
                    , rd->last_calculated_value
                    , rd->calculated_value
        );
    }

    netdata_spinlock_unlock(&st->data_collection_lock);
    rrdset_push_metrics_finished(&stream_buffer, st);

    // ALL DONE ABOUT THE DATA UPDATE
    // --------------------------------------------------------------------

    if(unlikely(st->rrd_memory_mode == RRD_MEMORY_MODE_MAP)) {
        // update the memory mapped files with the latest values

        rrdset_memory_file_update(st);

        for(dim_id = 0, rda = rda_base; dim_id < rda_slots ; ++dim_id, ++rda) {
            rd = rda->rd;
            if(unlikely(!rd)) continue;
            rrddim_memory_file_update(rd);
        }
    }

    for(dim_id = 0, rda = rda_base; dim_id < rda_slots ; ++dim_id, ++rda) {
        rd = rda->rd;
        if(unlikely(!rd)) continue;

        dictionary_acquired_item_release(st->rrddim_root_index, rda->item);
        rda->item = NULL;
        rda->rd = NULL;
    }

    rrdcontext_collected_rrdset(st);

    store_metric_collection_completed();
}

time_t rrdset_set_update_every_s(RRDSET *st, time_t update_every_s) {

    internal_error(true, "RRDSET '%s' switching update every from %d to %d",
                   rrdset_id(st), (int)st->update_every, (int)update_every_s);

    time_t prev_update_every_s = (time_t) st->update_every;
    st->update_every = (int) update_every_s;

    // switch update every to the storage engine
    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        for (size_t tier = 0; tier < storage_tiers; tier++) {
            if (rd->tiers[tier].db_collection_handle)
                storage_engine_store_change_collection_frequency(
                        rd->tiers[tier].db_collection_handle,
                        (int)(st->rrdhost->db[tier].tier_grouping * st->update_every));
        }

        assert(rd->update_every == (int) prev_update_every_s &&
               "chart's update every differs from the update every of its dimensions");
        rd->update_every = st->update_every;
    }
    rrddim_foreach_done(rd);

    return prev_update_every_s;
}

// ----------------------------------------------------------------------------
// compatibility layer for RRDSET files v019

#define RRDSET_MAGIC_V019 "NETDATA RRD SET FILE V019"
#define RRD_ID_LENGTH_MAX_V019 200

struct avl_element_v019 {
    void *avl_link[2];
    signed char avl_balance;
};
struct avl_tree_type_v019 {
    void *root;
    int (*compar)(void *a, void *b);
};
struct avl_tree_lock_v019 {
    struct avl_tree_type_v019 avl_tree;
    pthread_rwlock_t rwlock;
};
struct rrdset_map_save_v019 {
    struct avl_element_v019 avl;                    // ignored
    struct avl_element_v019 avlname;                // ignored
    char id[RRD_ID_LENGTH_MAX_V019 + 1];            // check to reset all - update on load
    void *name;                                     // ignored
    void *unused_ptr;                               // ignored
    void *type;                                     // ignored
    void *family;                                   // ignored
    void *title;                                    // ignored
    void *units;                                    // ignored
    void *context;                                  // ignored
    uint32_t hash_context;                          // ignored
    uint32_t chart_type;                            // ignored
    int update_every;                               // check to reset all - update on load
    long entries;                                   // check to reset all - update on load
    long current_entry;                             // NEEDS TO BE UPDATED - FIXED ON LOAD
    uint32_t flags;                                 // ignored
    void *exporting_flags;                          // ignored
    int gap_when_lost_iterations_above;             // ignored
    long priority;                                  // ignored
    uint32_t rrd_memory_mode;                       // ignored
    void *cache_dir;                                // ignored
    char cache_filename[FILENAME_MAX+1];            // ignored - update on load
    pthread_rwlock_t rrdset_rwlock;                 // ignored
    size_t counter;                                 // NEEDS TO BE UPDATED - maintained on load
    size_t counter_done;                            // ignored
    union {                                         //
        time_t last_accessed_time_s;                // ignored
        time_t last_entry_s;                        // ignored
    };                                              //
    time_t upstream_resync_time;                    // ignored
    void *plugin_name;                              // ignored
    void *module_name;                              // ignored
    void *chart_uuid;                               // ignored
    void *state;                                    // ignored
    size_t unused[3];                               // ignored
    size_t rrddim_page_alignment;                   // ignored
    uint32_t hash;                                  // ignored
    uint32_t hash_name;                             // ignored
    usec_t usec_since_last_update;                  // NEEDS TO BE UPDATED - maintained on load
    struct timeval last_updated;                    // NEEDS TO BE UPDATED - check to reset all - fixed on load
    struct timeval last_collected_time;             // ignored
    long long collected_total;                      // ignored
    long long last_collected_total;                 // ignored
    void *rrdfamily;                                // ignored
    void *rrdhost;                                  // ignored
    void *next;                                     // ignored
    long double green;                              // ignored
    long double red;                                // ignored
    struct avl_tree_lock_v019 rrdvar_root_index;    // ignored
    void *variables;                                // ignored
    void *alarms;                                   // ignored
    unsigned long memsize;                          // check to reset all - update on load
    char magic[sizeof(RRDSET_MAGIC_V019) + 1];      // check to reset all - update on load
    struct avl_tree_lock_v019 dimensions_index;     // ignored
    void *dimensions;                               // ignored
};

void rrdset_memory_file_update(RRDSET *st) {
    if(!st->st_on_file) return;
    struct rrdset_map_save_v019 *st_on_file = st->st_on_file;

    st_on_file->current_entry = st->current_entry;
    st_on_file->counter = st->counter;
    st_on_file->usec_since_last_update = st->usec_since_last_update;
    st_on_file->last_updated.tv_sec = st->last_updated.tv_sec;
    st_on_file->last_updated.tv_usec = st->last_updated.tv_usec;
}

const char *rrdset_cache_filename(RRDSET *st) {
    if(!st->st_on_file) return NULL;
    struct rrdset_map_save_v019 *st_on_file = st->st_on_file;
    return st_on_file->cache_filename;
}

const char *rrdset_cache_dir(RRDSET *st) {
    if(!st->cache_dir)
        st->cache_dir = rrdhost_cache_dir_for_rrdset_alloc(st->rrdhost, rrdset_id(st));

    return st->cache_dir;
}

void rrdset_memory_file_free(RRDSET *st) {
    if(!st->st_on_file) return;

    // needed for memory mode map, to save the latest state
    rrdset_memory_file_update(st);

    struct rrdset_map_save_v019 *st_on_file = st->st_on_file;
    __atomic_sub_fetch(&rrddim_db_memory_size, st_on_file->memsize, __ATOMIC_RELAXED);
    netdata_munmap(st_on_file, st_on_file->memsize);

    // remove the pointers from the RRDDIM
    st->st_on_file = NULL;
}

void rrdset_memory_file_save(RRDSET *st) {
    if(!st->st_on_file) return;

    rrdset_memory_file_update(st);

    struct rrdset_map_save_v019 *st_on_file = st->st_on_file;
    if(st_on_file->rrd_memory_mode != RRD_MEMORY_MODE_SAVE) return;

    memory_file_save(st_on_file->cache_filename, st->st_on_file, st_on_file->memsize);
}

bool rrdset_memory_load_or_create_map_save(RRDSET *st, RRD_MEMORY_MODE memory_mode) {
    if(memory_mode != RRD_MEMORY_MODE_SAVE && memory_mode != RRD_MEMORY_MODE_MAP)
        return false;

    char fullfilename[FILENAME_MAX + 1];
    snprintfz(fullfilename, FILENAME_MAX, "%s/main.db", rrdset_cache_dir(st));

    unsigned long size = sizeof(struct rrdset_map_save_v019);
    struct rrdset_map_save_v019 *st_on_file = (struct rrdset_map_save_v019 *)netdata_mmap(
        fullfilename, size, ((memory_mode == RRD_MEMORY_MODE_MAP) ? MAP_SHARED : MAP_PRIVATE), 0, false, NULL);

    if(!st_on_file) return false;

    time_t now_s = now_realtime_sec();

    st_on_file->magic[sizeof(RRDSET_MAGIC_V019)] = '\0';
    if(strcmp(st_on_file->magic, RRDSET_MAGIC_V019) != 0) {
        info("Initializing file '%s'.", fullfilename);
        memset(st_on_file, 0, size);
    }
    else if(strncmp(st_on_file->id, rrdset_id(st), RRD_ID_LENGTH_MAX_V019) != 0) {
        error("File '%s' contents are not for chart '%s'. Clearing it.", fullfilename, rrdset_id(st));
        memset(st_on_file, 0, size);
    }
    else if(st_on_file->memsize != size || st_on_file->entries != st->entries) {
        error("File '%s' does not have the desired size. Clearing it.", fullfilename);
        memset(st_on_file, 0, size);
    }
    else if(st_on_file->update_every != st->update_every) {
        error("File '%s' does not have the desired granularity. Clearing it.", fullfilename);
        memset(st_on_file, 0, size);
    }
    else if((now_s - st_on_file->last_updated.tv_sec) > st->update_every * st->entries) {
        info("File '%s' is too old. Clearing it.", fullfilename);
        memset(st_on_file, 0, size);
    }
    else if(st_on_file->last_updated.tv_sec > now_s + st->update_every) {
        error("File '%s' refers to the future by %zd secs. Resetting it to now.", fullfilename, (ssize_t)(st_on_file->last_updated.tv_sec - now_s));
        st_on_file->last_updated.tv_sec = now_s;
    }

    if(st_on_file->current_entry >= st_on_file->entries)
        st_on_file->current_entry = 0;

    // make sure the database is aligned
    bool align_last_updated = false;
    if(st_on_file->last_updated.tv_sec) {
        st_on_file->update_every = st->update_every;
        align_last_updated = true;
    }

    // copy the useful values to st
    st->current_entry = st_on_file->current_entry;
    st->counter = st_on_file->counter;
    st->usec_since_last_update = st_on_file->usec_since_last_update;
    st->last_updated.tv_sec = st_on_file->last_updated.tv_sec;
    st->last_updated.tv_usec = st_on_file->last_updated.tv_usec;

    // link it to st
    st->st_on_file = st_on_file;

    // clear everything
    memset(st_on_file, 0, size);

    // set the values we need
    strncpyz(st_on_file->id, rrdset_id(st), RRD_ID_LENGTH_MAX_V019);
    strcpy(st_on_file->cache_filename, fullfilename);
    strcpy(st_on_file->magic, RRDSET_MAGIC_V019);
    st_on_file->memsize = size;
    st_on_file->entries = st->entries;
    st_on_file->update_every = st->update_every;
    st_on_file->rrd_memory_mode = memory_mode;

    if(align_last_updated)
        last_updated_time_align(st);

    // copy the useful values back to st_on_file
    rrdset_memory_file_update(st);

    __atomic_add_fetch(&rrddim_db_memory_size, st_on_file->memsize, __ATOMIC_RELAXED);
    return true;
}

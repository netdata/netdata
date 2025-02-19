// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdset-index-id.h"
#include "rrdset-index-name.h"
#include "rrdset-slots.h"

// --------------------------------------------------------------------------------------------------------------------
// tier1/2 spread over time

static size_t global_rrdset_counter = 0;
static uint16_t rrdset_collection_modulo_init(void) {
    return __atomic_fetch_add(&global_rrdset_counter, 1, __ATOMIC_RELAXED) % 65535;
}

uint16_t rrddim_collection_modulo(RRDSET *st, uint32_t spread) {
    if(!spread) spread = 65535;
    spread = MIN(spread, 65535);
    return 1 + (st->collection_modulo % spread);
}

// --------------------------------------------------------------------------------------------------------------------

static inline void rrdset_update_permanent_labels(RRDSET *st) {
    if(!st->rrdlabels) return;

    rrdlabels_add(st->rrdlabels, "_collect_plugin", rrdset_plugin_name(st), RRDLABEL_SRC_AUTO | RRDLABEL_FLAG_DONT_DELETE);
    rrdlabels_add(st->rrdlabels, "_collect_module", rrdset_module_name(st), RRDLABEL_SRC_AUTO | RRDLABEL_FLAG_DONT_DELETE);
}

// --------------------------------------------------------------------------------------------------------------------
// RRDSET index

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
    RRD_DB_MODE memory_mode;
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

    st->collection_modulo = rrdset_collection_modulo_init();

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

    st->db.entries = (ctr->memory_mode != RRD_DB_MODE_DBENGINE) ? align_entries_to_pagesize(ctr->memory_mode, ctr->history_entries) : 5;
    st->update_every = ctr->update_every;
    st->rrd_memory_mode = ctr->memory_mode;

    st->chart_type = ctr->chart_type;
    st->rrdhost = host;

    rrdset_stream_send_chart_slot_assign(st);

    spinlock_init(&st->data_collection_lock);

    st->flags =   RRDSET_FLAG_SYNC_CLOCK
                | RRDSET_FLAG_INDEXED_ID
                | RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED
                | RRDSET_FLAG_SENDER_REPLICATION_FINISHED
        ;

    rw_spinlock_init(&st->alerts.spinlock);

    // initialize the db tiers
    {
        for(size_t tier = 0; tier < nd_profile.storage_tiers; tier++) {
            STORAGE_ENGINE *eng = st->rrdhost->db[tier].eng;
            if(!eng) continue;

            st->smg[tier] = storage_engine_metrics_group_get(eng->seb, host->db[tier].si, &st->chart_uuid);
        }
    }

    rrddim_index_init(st);

    st->rrdvars = rrdvariables_create();
    st->rrdlabels = rrdlabels_create();
    rrdset_update_permanent_labels(st);

    st->green = NAN;
    st->red = NAN;

    rrdset_pluginsd_receive_slots_initialize(st);

    rrdset_flag_set(st, RRDSET_FLAG_PENDING_HEALTH_INITIALIZATION);
    rrdhost_flag_set(host, RRDHOST_FLAG_PENDING_HEALTH_INITIALIZATION);

    ctr->react_action = RRDSET_REACT_NEW;

    ml_chart_new(st);
}

// the destructor - the dictionary is write locked while this runs
static void rrdset_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdset, void *rrdhost) {
    RRDHOST *host = rrdhost;
    RRDSET *st = rrdset;

    rrdset_flag_clear(st, RRDSET_FLAG_INDEXED_ID);

    rrdset_finalize_collection(st, false);

    rrdset_stream_send_chart_slot_release(st);

    // remove it from the name index
    rrdset_index_del_name(host, st);

    // release the collector info
    dictionary_destroy(st->functions_view);

    rrdcalc_unlink_and_delete_all_rrdset_alerts(st);

    // ------------------------------------------------------------------------
    // the order of destruction is important here

    // 1. delete RRDVAR index after the above, to avoid triggering its garbage collector (they have references on this)
    rrdvariables_destroy(st->rrdvars);      // free all variables and destroy the rrdvar dictionary

    // 2. delete RRDDIMs, now their variables are not existing, so this is fast
    rrddim_index_destroy(st);                   // free all the dimensions and destroy the dimensions index

    // 3. this has to be after the dimensions are freed, but before labels are freed (contexts need the labels)
    rrdcontext_removed_rrdset(st);              // let contexts know

    // 4. destroy the chart labels
    rrdlabels_destroy(st->rrdlabels);  // destroy the labels, after letting the contexts know

    // 5. destroy the ml handle
    ml_chart_delete(st);

    // ------------------------------------------------------------------------
    // free it

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
}

// the item to be inserted, is already in the dictionary
// this callback deals with the situation, migrating the existing object to the new values
// the dictionary is write locked while this runs
static bool rrdset_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdset, void *new_rrdset, void *constructor_data) {
    (void)new_rrdset; // it is NULL

    struct rrdset_constructor *ctr = constructor_data;
    RRDSET *st = rrdset;

    rrdset_isnot_obsolete___safe_from_collector_thread(st);

    ctr->react_action = RRDSET_REACT_NONE;

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
    rrdset_flag_set(st, RRDSET_FLAG_PENDING_HEALTH_INITIALIZATION);
    rrdhost_flag_set(st->rrdhost, RRDHOST_FLAG_PENDING_HEALTH_INITIALIZATION);

    return ctr->react_action != RRDSET_REACT_NONE;
}

// this is called after all insertions/conflicts, with the dictionary unlocked, with a reference to RRDSET
// so, any actions requiring locks on other objects, should be placed here
static void rrdset_react_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdset, void *constructor_data) {
    struct rrdset_constructor *ctr = constructor_data;
    RRDSET *st = rrdset;
    RRDHOST *host = st->rrdhost;

    st->last_accessed_time_s = now_realtime_sec();

    if(ctr->react_action & (RRDSET_REACT_NEW | RRDSET_REACT_PLUGIN_UPDATED | RRDSET_REACT_MODULE_UPDATED)) {
        if (ctr->react_action & RRDSET_REACT_NEW) {
            if(unlikely(rrdcontext_find_chart_uuid(st,  &st->chart_uuid)))
                uuid_generate(st->chart_uuid);
        }
        rrdset_flag_set(st, RRDSET_FLAG_METADATA_UPDATE);
        rrdhost_flag_set(host, RRDHOST_FLAG_METADATA_UPDATE);
    }

    rrdset_metadata_updated(st);
}

void rrdset_index_init(RRDHOST *host) {
    if(!host->rrdset_root_index) {
        host->rrdset_root_index = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                             &dictionary_stats_category_rrdset, sizeof(RRDSET));

        dictionary_register_insert_callback(host->rrdset_root_index, rrdset_insert_callback, NULL);
        dictionary_register_conflict_callback(host->rrdset_root_index, rrdset_conflict_callback, NULL);
        dictionary_register_react_callback(host->rrdset_root_index, rrdset_react_callback, NULL);
        dictionary_register_delete_callback(host->rrdset_root_index, rrdset_delete_callback, host);
    }

    rrdset_index_byname_init(host);
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

RRDSET *rrdset_find(RRDHOST *host, const char *id) {
    netdata_log_debug(D_RRD_CALLS, "rrdset_find() for chart '%s' in host '%s'", id, rrdhost_hostname(host));
    RRDSET *st = rrdset_index_find(host, id);

    if(st)
        st->last_accessed_time_s = now_realtime_sec();

    return(st);
}

RRDSET *rrdset_find_bytype(RRDHOST *host, const char *type, const char *id) {
    netdata_log_debug(D_RRD_CALLS, "rrdset_find_bytype() for chart '%s.%s' in host '%s'", type, id, rrdhost_hostname(host));

    char buf[RRD_ID_LENGTH_MAX + 1];
    strncpyz(buf, type, RRD_ID_LENGTH_MAX - 1);
    strcat(buf, ".");
    int len = (int) strlen(buf);
    strncpyz(&buf[len], id, (size_t) (RRD_ID_LENGTH_MAX - len));

    return(rrdset_find(host, buf));
}

RRDSET_ACQUIRED *rrdset_find_and_acquire(RRDHOST *host, const char *id) {
    netdata_log_debug(D_RRD_CALLS, "rrdset_find_and_acquire() for host %s, chart %s", rrdhost_hostname(host), id);

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
    ,
    RRD_DB_MODE memory_mode
    , long history_entries
) {
    if (host != localhost)
        host->stream.rcv.status.last_chart = now_realtime_sec();

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

    netdata_log_debug(D_RRD_CALLS, "Creating RRD_STATS for '%s.%s'.", type, id);

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

void rrdset_free(RRDSET *st) {
    if(unlikely(!st)) return;
    rrdset_index_del(st->rrdhost, st);
}

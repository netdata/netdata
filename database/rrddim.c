// SPDX-License-Identifier: GPL-3.0-or-later

#define NETDATA_RRD_INTERNALS
#include "rrd.h"
#include "storage_engine.h"

void rrddim_metadata_updated(RRDDIM *rd) {
    rrdcontext_updated_rrddim(rd);
    rrdset_metadata_updated(rd->rrdset);
}

// ----------------------------------------------------------------------------
// RRDDIM index

struct rrddim_constructor {
    RRDSET *st;
    const char *id;
    const char *name;
    collected_number multiplier;
    collected_number divisor;
    RRD_ALGORITHM algorithm;
    RRD_MEMORY_MODE memory_mode;

    enum {
        RRDDIM_REACT_NONE    = 0,
        RRDDIM_REACT_NEW     = (1 << 0),
        RRDDIM_REACT_UPDATED = (1 << 2),
    } react_action;

};

// isolated call to appear
// separate in statistics
static void *rrddim_alloc_db(size_t entries) {
    return callocz(entries, sizeof(storage_number));
}

static void rrddim_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrddim, void *constructor_data) {
    struct rrddim_constructor *ctr = constructor_data;
    RRDDIM *rd = rrddim;
    RRDSET *st = ctr->st;
    RRDHOST *host = st->rrdhost;

    rd->flags = RRDDIM_FLAG_NONE;

    rd->id = string_strdupz(ctr->id);
    rd->name = (ctr->name && *ctr->name)?rrd_string_strdupz(ctr->name):string_dup(rd->id);

    rd->algorithm = ctr->algorithm;
    rd->multiplier = ctr->multiplier;
    rd->divisor = ctr->divisor;
    if(!rd->divisor) rd->divisor = 1;

    rd->rrdset = st;

    rd->rrdpush.sender.dim_slot = __atomic_add_fetch(&st->rrdpush.sender.dim_last_slot_used, 1, __ATOMIC_RELAXED);

    if(rrdset_flag_check(st, RRDSET_FLAG_STORE_FIRST))
        rd->collector.counter = 1;

    if(ctr->memory_mode == RRD_MEMORY_MODE_RAM) {
        size_t entries = st->db.entries;
        if(!entries) entries = 5;

        rd->db.data = netdata_mmap(NULL, entries * sizeof(storage_number), MAP_PRIVATE, 1, false, NULL);
        if(!rd->db.data) {
            netdata_log_info("Failed to use memory mode ram for chart '%s', dimension '%s', falling back to alloc", rrdset_name(st), rrddim_name(rd));
            ctr->memory_mode = RRD_MEMORY_MODE_ALLOC;
        }
        else {
            rd->db.memsize = entries * sizeof(storage_number);
            __atomic_add_fetch(&rrddim_db_memory_size, rd->db.memsize, __ATOMIC_RELAXED);
        }
    }

    if(ctr->memory_mode == RRD_MEMORY_MODE_ALLOC || ctr->memory_mode == RRD_MEMORY_MODE_NONE) {
        size_t entries = st->db.entries;
        if(entries < 5) entries = 5;

        rd->db.data = rrddim_alloc_db(entries);
        rd->db.memsize = entries * sizeof(storage_number);
        __atomic_add_fetch(&rrddim_db_memory_size, rd->db.memsize, __ATOMIC_RELAXED);
    }

    rd->rrd_memory_mode = ctr->memory_mode;

    if (unlikely(rrdcontext_find_dimension_uuid(st, rrddim_id(rd), &(rd->metric_uuid))))
        uuid_generate(rd->metric_uuid);

    // initialize the db tiers
    {
        size_t initialized = 0;
        for(size_t tier = 0; tier < storage_tiers ; tier++) {
            STORAGE_ENGINE *eng = host->db[tier].eng;
            rd->tiers[tier].seb = eng->seb;
            rd->tiers[tier].tier_grouping = host->db[tier].tier_grouping;
            rd->tiers[tier].smh = eng->api.metric_get_or_create(rd, host->db[tier].si);
            storage_point_unset(rd->tiers[tier].virtual_point);
            initialized++;

            // internal_error(true, "TIER GROUPING of chart '%s', dimension '%s' for tier %d is set to %d", rd->rrdset->name, rd->name, tier, rd->tiers[tier]->tier_grouping);
        }

        if(!initialized)
            netdata_log_error("Failed to initialize all db tiers for chart '%s', dimension '%s", rrdset_name(st), rrddim_name(rd));

        if(!rd->tiers[0].smh)
            netdata_log_error("Failed to initialize the first db tier for chart '%s', dimension '%s", rrdset_name(st), rrddim_name(rd));
    }

    // initialize data collection for all tiers
    {
        size_t initialized = 0;
        for (size_t tier = 0; tier < storage_tiers; tier++) {
            if (rd->tiers[tier].smh) {
                rd->tiers[tier].sch =
                        storage_metric_store_init(rd->tiers[tier].seb, rd->tiers[tier].smh, st->rrdhost->db[tier].tier_grouping * st->update_every, rd->rrdset->smg[tier]);
                initialized++;
            }
        }

        if(!initialized)
            netdata_log_error("Failed to initialize data collection for all db tiers for chart '%s', dimension '%s", rrdset_name(st), rrddim_name(rd));
    }

    if(rrdset_number_of_dimensions(st) != 0) {
        RRDDIM *td;
        dfe_start_write(st->rrddim_root_index, td) {
            if(td) break;
        }
        dfe_done(td);

        if(td && (td->algorithm != rd->algorithm || ABS(td->multiplier) != ABS(rd->multiplier) || ABS(td->divisor) != ABS(rd->divisor))) {
            if(!rrdset_flag_check(st, RRDSET_FLAG_HETEROGENEOUS)) {
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("Dimension '%s' added on chart '%s' of host '%s' is not homogeneous to other dimensions already "
                     "present (algorithm is '%s' vs '%s', multiplier is %d vs %d, "
                     "divisor is %d vs %d).",
                     rrddim_name(rd),
                     rrdset_name(st),
                     rrdhost_hostname(host),
                     rrd_algorithm_name(rd->algorithm), rrd_algorithm_name(td->algorithm),
                     rd->multiplier, td->multiplier,
                     rd->divisor, td->divisor
                );
#endif
                rrdset_flag_set(st, RRDSET_FLAG_HETEROGENEOUS);
            }
        }
    }

    rrddim_flag_set(rd, RRDDIM_FLAG_PENDING_HEALTH_INITIALIZATION);
    rrdset_flag_set(rd->rrdset, RRDSET_FLAG_PENDING_HEALTH_INITIALIZATION);
    rrdhost_flag_set(rd->rrdset->rrdhost, RRDHOST_FLAG_PENDING_HEALTH_INITIALIZATION);

    // let the chart resync
    rrdset_flag_set(st, RRDSET_FLAG_SYNC_CLOCK);

    ml_dimension_new(rd);

    ctr->react_action = RRDDIM_REACT_NEW;

    internal_error(false, "RRDDIM: inserted dimension '%s' of chart '%s' of host '%s'",
                   rrddim_name(rd), rrdset_name(st), rrdhost_hostname(st->rrdhost));

}

bool rrddim_finalize_collection_and_check_retention(RRDDIM *rd) {
    size_t tiers_available = 0, tiers_said_no_retention = 0;

    for(size_t tier = 0; tier < storage_tiers ;tier++) {
        if(!rd->tiers[tier].sch)
            continue;

        tiers_available++;

        if(storage_engine_store_finalize(rd->tiers[tier].sch))
            tiers_said_no_retention++;

        rd->tiers[tier].sch = NULL;
    }

    // return true if the dimension has retention in the db
    return (!tiers_said_no_retention || tiers_available > tiers_said_no_retention);
}

static void rrddim_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrddim, void *rrdset) {
    RRDDIM *rd = rrddim;
    RRDSET *st = rrdset;
    RRDHOST *host = st->rrdhost;

    internal_error(false, "RRDDIM: deleting dimension '%s' of chart '%s' of host '%s'",
                   rrddim_name(rd), rrdset_name(st), rrdhost_hostname(host));

    rrdcontext_removed_rrddim(rd);

    ml_dimension_delete(rd);

    netdata_log_debug(D_RRD_CALLS, "rrddim_free() %s.%s", rrdset_name(st), rrddim_name(rd));

    if (!rrddim_finalize_collection_and_check_retention(rd) && rd->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
        /* This metric has no data and no references */
        metaqueue_delete_dimension_uuid(&rd->metric_uuid);
    }

    for(size_t tier = 0; tier < storage_tiers ;tier++) {
        if(!rd->tiers[tier].smh) continue;

        STORAGE_ENGINE* eng = host->db[tier].eng;
        eng->api.metric_release(rd->tiers[tier].smh);
        rd->tiers[tier].smh = NULL;
    }

    if(rd->db.data) {
        __atomic_sub_fetch(&rrddim_db_memory_size, rd->db.memsize, __ATOMIC_RELAXED);

        if(rd->rrd_memory_mode == RRD_MEMORY_MODE_RAM)
            netdata_munmap(rd->db.data, rd->db.memsize);
        else
            freez(rd->db.data);
    }

    string_freez(rd->id);
    string_freez(rd->name);
}

static bool rrddim_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrddim, void *new_rrddim, void *constructor_data) {
    (void)new_rrddim; // it is NULL

    struct rrddim_constructor *ctr = constructor_data;
    RRDDIM *rd = rrddim;
    RRDSET *st = ctr->st;

    ctr->react_action = RRDDIM_REACT_NONE;

    int rc = rrddim_reset_name(st, rd, ctr->name);
    rc += rrddim_set_algorithm(st, rd, ctr->algorithm);
    rc += rrddim_set_multiplier(st, rd, ctr->multiplier);
    rc += rrddim_set_divisor(st, rd, ctr->divisor);

    for(size_t tier = 0; tier < storage_tiers ;tier++) {
        if (!rd->tiers[tier].sch)
            rd->tiers[tier].sch =
                    storage_metric_store_init(rd->tiers[tier].seb, rd->tiers[tier].smh, st->rrdhost->db[tier].tier_grouping * st->update_every, rd->rrdset->smg[tier]);
    }

    if(rrddim_flag_check(rd, RRDDIM_FLAG_ARCHIVED)) {
        rrddim_flag_clear(rd, RRDDIM_FLAG_ARCHIVED);

        rrddim_flag_set(rd, RRDDIM_FLAG_PENDING_HEALTH_INITIALIZATION);
        rrdset_flag_set(rd->rrdset, RRDSET_FLAG_PENDING_HEALTH_INITIALIZATION);
        rrdhost_flag_set(rd->rrdset->rrdhost, RRDHOST_FLAG_PENDING_HEALTH_INITIALIZATION);
    }

    if(unlikely(rc))
        ctr->react_action = RRDDIM_REACT_UPDATED;

    return ctr->react_action == RRDDIM_REACT_UPDATED;
}

static void rrddim_react_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrddim, void *constructor_data) {
    struct rrddim_constructor *ctr = constructor_data;
    RRDDIM *rd = rrddim;
    RRDSET *st = ctr->st;

    if(ctr->react_action & (RRDDIM_REACT_UPDATED | RRDDIM_REACT_NEW)) {
        rrddim_flag_set(rd, RRDDIM_FLAG_METADATA_UPDATE);
        rrdhost_flag_set(rd->rrdset->rrdhost, RRDHOST_FLAG_METADATA_UPDATE);
    }

    if(ctr->react_action == RRDDIM_REACT_UPDATED) {
        // the chart needs to be updated to the parent
        rrdset_flag_set(st, RRDSET_FLAG_SYNC_CLOCK);
    }

    rrddim_metadata_updated(rd);
}

size_t rrddim_size(void) {
    return sizeof(RRDDIM) + storage_tiers * sizeof(struct rrddim_tier);
}

void rrddim_index_init(RRDSET *st) {
    if(!st->rrddim_root_index) {
        st->rrddim_root_index = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                           &dictionary_stats_category_rrdset_rrddim, rrddim_size());

        dictionary_register_insert_callback(st->rrddim_root_index, rrddim_insert_callback, NULL);
        dictionary_register_conflict_callback(st->rrddim_root_index, rrddim_conflict_callback, NULL);
        dictionary_register_delete_callback(st->rrddim_root_index, rrddim_delete_callback, st);
        dictionary_register_react_callback(st->rrddim_root_index, rrddim_react_callback, st);
    }
}

void rrddim_index_destroy(RRDSET *st) {
    dictionary_destroy(st->rrddim_root_index);
    st->rrddim_root_index = NULL;
}

static inline RRDDIM *rrddim_index_find(RRDSET *st, const char *id) {
    return dictionary_get(st->rrddim_root_index, id);
}

// ----------------------------------------------------------------------------
// RRDDIM - find a dimension

inline RRDDIM *rrddim_find(RRDSET *st, const char *id) {
    netdata_log_debug(D_RRD_CALLS, "rrddim_find() for chart %s, dimension %s", rrdset_name(st), id);

    return rrddim_index_find(st, id);
}

inline RRDDIM_ACQUIRED *rrddim_find_and_acquire(RRDSET *st, const char *id) {
    netdata_log_debug(D_RRD_CALLS, "rrddim_find_and_acquire() for chart %s, dimension %s", rrdset_name(st), id);

    return (RRDDIM_ACQUIRED *)dictionary_get_and_acquire_item(st->rrddim_root_index, id);
}

RRDDIM *rrddim_acquired_to_rrddim(RRDDIM_ACQUIRED *rda) {
    if(unlikely(!rda))
        return NULL;

    return (RRDDIM *) dictionary_acquired_item_value((const DICTIONARY_ITEM *)rda);
}

void rrddim_acquired_release(RRDDIM_ACQUIRED *rda) {
    if(unlikely(!rda))
        return;

    RRDDIM *rd = rrddim_acquired_to_rrddim(rda);
    dictionary_acquired_item_release(rd->rrdset->rrddim_root_index, (const DICTIONARY_ITEM *)rda);
}

// This will not return dimensions that are archived
RRDDIM *rrddim_find_active(RRDSET *st, const char *id) {
    RRDDIM *rd = rrddim_find(st, id);

    if (unlikely(rd && rrddim_flag_check(rd, RRDDIM_FLAG_ARCHIVED)))
        return NULL;

    return rd;
}

// ----------------------------------------------------------------------------
// RRDDIM rename a dimension

inline int rrddim_reset_name(RRDSET *st __maybe_unused, RRDDIM *rd, const char *name) {
    if(unlikely(!name || !*name || !strcmp(rrddim_name(rd), name)))
        return 0;

    netdata_log_debug(D_RRD_CALLS, "rrddim_reset_name() from %s.%s to %s.%s", rrdset_name(st), rrddim_name(rd), rrdset_name(st), name);

    STRING *old = rd->name;
    rd->name = rrd_string_strdupz(name);
    string_freez(old);

    rrddim_metadata_updated(rd);

    return 1;
}

inline int rrddim_set_algorithm(RRDSET *st, RRDDIM *rd, RRD_ALGORITHM algorithm) {
    if(unlikely(rd->algorithm == algorithm))
        return 0;

    netdata_log_debug(D_RRD_CALLS, "Updating algorithm of dimension '%s/%s' from %s to %s", rrdset_id(st), rrddim_name(rd), rrd_algorithm_name(rd->algorithm), rrd_algorithm_name(algorithm));
    rd->algorithm = algorithm;
    rrddim_metadata_updated(rd);
    rrdset_flag_set(st, RRDSET_FLAG_HOMOGENEOUS_CHECK);
    rrdcontext_updated_rrddim_algorithm(rd);
    return 1;
}

inline int rrddim_set_multiplier(RRDSET *st, RRDDIM *rd, int32_t multiplier) {
    if(unlikely(rd->multiplier == multiplier))
        return 0;

    netdata_log_debug(D_RRD_CALLS, "Updating multiplier of dimension '%s/%s' from %d to %d",
          rrdset_id(st), rrddim_name(rd), rd->multiplier, multiplier);
    rd->multiplier = multiplier;
    rrddim_metadata_updated(rd);
    rrdset_flag_set(st, RRDSET_FLAG_HOMOGENEOUS_CHECK);
    rrdcontext_updated_rrddim_multiplier(rd);
    return 1;
}

inline int rrddim_set_divisor(RRDSET *st, RRDDIM *rd, int32_t divisor) {
    if(unlikely(rd->divisor == divisor))
        return 0;

    netdata_log_debug(D_RRD_CALLS, "Updating divisor of dimension '%s/%s' from %d to %d",
          rrdset_id(st), rrddim_name(rd), rd->divisor, divisor);
    rd->divisor = divisor;
    rrddim_metadata_updated(rd);
    rrdset_flag_set(st, RRDSET_FLAG_HOMOGENEOUS_CHECK);
    rrdcontext_updated_rrddim_divisor(rd);
    return 1;
}

// ----------------------------------------------------------------------------

time_t rrddim_last_entry_s_of_tier(RRDDIM *rd, size_t tier) {
    if(unlikely(tier > storage_tiers || !rd->tiers[tier].smh))
        return 0;

    return storage_engine_latest_time_s(rd->tiers[tier].seb, rd->tiers[tier].smh);
}

// get the timestamp of the last entry in the round-robin database
time_t rrddim_last_entry_s(RRDDIM *rd) {
    time_t latest_time_s = rrddim_last_entry_s_of_tier(rd, 0);

    for(size_t tier = 1; tier < storage_tiers ;tier++) {
        if(unlikely(!rd->tiers[tier].smh)) continue;

        time_t t = rrddim_last_entry_s_of_tier(rd, tier);
        if(t > latest_time_s)
            latest_time_s = t;
    }

    return latest_time_s;
}

time_t rrddim_first_entry_s_of_tier(RRDDIM *rd, size_t tier) {
    if(unlikely(tier > storage_tiers || !rd->tiers[tier].smh))
        return 0;

    return storage_engine_oldest_time_s(rd->tiers[tier].seb, rd->tiers[tier].smh);
}

time_t rrddim_first_entry_s(RRDDIM *rd) {
    time_t oldest_time_s = 0;

    for(size_t tier = 0; tier < storage_tiers ;tier++) {
        time_t t = rrddim_first_entry_s_of_tier(rd, tier);
        if(t != 0 && (oldest_time_s == 0 || t < oldest_time_s))
            oldest_time_s = t;
    }

    return oldest_time_s;
}

RRDDIM *rrddim_add_custom(RRDSET *st
                          , const char *id
                          , const char *name
                          , collected_number multiplier
                          , collected_number divisor
                          , RRD_ALGORITHM algorithm
                          , RRD_MEMORY_MODE memory_mode
                          ) {
    struct rrddim_constructor tmp = {
        .st = st,
        .id = id,
        .name = name,
        .multiplier = multiplier,
        .divisor = divisor,
        .algorithm = algorithm,
        .memory_mode = memory_mode,
    };

    RRDDIM *rd = dictionary_set_advanced(st->rrddim_root_index, tmp.id, -1, NULL, rrddim_size(), &tmp);
    return(rd);
}

// ----------------------------------------------------------------------------
// RRDDIM remove / free a dimension

void rrddim_free(RRDSET *st, RRDDIM *rd) {
    dictionary_del(st->rrddim_root_index, string2str(rd->id));
}


// ----------------------------------------------------------------------------
// RRDDIM - set dimension options

int rrddim_hide(RRDSET *st, const char *id) {
    netdata_log_debug(D_RRD_CALLS, "rrddim_hide() for chart %s, dimension %s", rrdset_name(st), id);

    RRDHOST *host = st->rrdhost;

    RRDDIM *rd = rrddim_find(st, id);
    if(unlikely(!rd)) {
        netdata_log_error("Cannot find dimension with id '%s' on stats '%s' (%s) on host '%s'.", id, rrdset_name(st), rrdset_id(st), rrdhost_hostname(host));
        return 1;
    }
    if (!rrddim_flag_check(rd, RRDDIM_FLAG_META_HIDDEN)) {
        rrddim_flag_set(rd, RRDDIM_FLAG_META_HIDDEN | RRDDIM_FLAG_METADATA_UPDATE);
        rrdhost_flag_set(rd->rrdset->rrdhost, RRDHOST_FLAG_METADATA_UPDATE);
    }

    rrddim_option_set(rd, RRDDIM_OPTION_HIDDEN);
    rrdcontext_updated_rrddim_flags(rd);
    return 0;
}

int rrddim_unhide(RRDSET *st, const char *id) {
    netdata_log_debug(D_RRD_CALLS, "rrddim_unhide() for chart %s, dimension %s", rrdset_name(st), id);

    RRDHOST *host = st->rrdhost;
    RRDDIM *rd = rrddim_find(st, id);
    if(unlikely(!rd)) {
        netdata_log_error("Cannot find dimension with id '%s' on stats '%s' (%s) on host '%s'.", id, rrdset_name(st), rrdset_id(st), rrdhost_hostname(host));
        return 1;
    }
    if (rrddim_flag_check(rd, RRDDIM_FLAG_META_HIDDEN)) {
        rrddim_flag_clear(rd, RRDDIM_FLAG_META_HIDDEN);
        rrddim_flag_set(rd, RRDDIM_FLAG_METADATA_UPDATE);
        rrdhost_flag_set(rd->rrdset->rrdhost, RRDHOST_FLAG_METADATA_UPDATE);
    }

    rrddim_option_clear(rd, RRDDIM_OPTION_HIDDEN);
    rrdcontext_updated_rrddim_flags(rd);
    return 0;
}

inline void rrddim_is_obsolete___safe_from_collector_thread(RRDSET *st, RRDDIM *rd) {
    netdata_log_debug(D_RRD_CALLS, "rrddim_is_obsolete___safe_from_collector_thread() for chart %s, dimension %s", rrdset_name(st), rrddim_name(rd));

    if(unlikely(rrddim_flag_check(rd, RRDDIM_FLAG_ARCHIVED))) {
        netdata_log_info("Cannot obsolete already archived dimension %s from chart %s", rrddim_name(rd), rrdset_name(st));
        return;
    }
    rrddim_flag_set(rd, RRDDIM_FLAG_OBSOLETE);
    rrdset_flag_set(st, RRDSET_FLAG_OBSOLETE_DIMENSIONS);
    rrdhost_flag_set(st->rrdhost, RRDHOST_FLAG_PENDING_OBSOLETE_DIMENSIONS);
    rrdcontext_updated_rrddim_flags(rd);
}

inline void rrddim_isnot_obsolete___safe_from_collector_thread(RRDSET *st __maybe_unused, RRDDIM *rd) {
    netdata_log_debug(D_RRD_CALLS, "rrddim_isnot_obsolete___safe_from_collector_thread() for chart %s, dimension %s", rrdset_name(st), rrddim_name(rd));

    rrddim_flag_clear(rd, RRDDIM_FLAG_OBSOLETE);
    rrdcontext_updated_rrddim_flags(rd);
}

// ----------------------------------------------------------------------------
// RRDDIM - collect values for a dimension

inline collected_number rrddim_set_by_pointer(RRDSET *st, RRDDIM *rd, collected_number value) {
    struct timeval now;
    now_realtime_timeval(&now);

    return rrddim_timed_set_by_pointer(st, rd, now, value);
}

collected_number rrddim_timed_set_by_pointer(RRDSET *st __maybe_unused, RRDDIM *rd, struct timeval collected_time, collected_number value) {
    netdata_log_debug(D_RRD_CALLS, "rrddim_set_by_pointer() for chart %s, dimension %s, value " COLLECTED_NUMBER_FORMAT, rrdset_name(st), rrddim_name(rd), value);

    rd->collector.last_collected_time = collected_time;
    rd->collector.collected_value = value;
    rrddim_set_updated(rd);
    rd->collector.counter++;

    collected_number v = (value >= 0) ? value : -value;
    if (unlikely(v > rd->collector.collected_value_max))
        rd->collector.collected_value_max = v;

    return rd->collector.last_collected_value;
}


collected_number rrddim_set(RRDSET *st, const char *id, collected_number value) {
    RRDHOST *host = st->rrdhost;
    RRDDIM *rd = rrddim_find(st, id);
    if(unlikely(!rd)) {
        netdata_log_error("Cannot find dimension with id '%s' on stats '%s' (%s) on host '%s'.", id, rrdset_name(st), rrdset_id(st), rrdhost_hostname(host));
        return 0;
    }

    return rrddim_set_by_pointer(st, rd, value);
}

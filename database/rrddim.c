// SPDX-License-Identifier: GPL-3.0-or-later

#define NETDATA_RRD_INTERNALS
#include "rrd.h"
#include "database/rrddim_mem.h"
#ifdef ENABLE_DBENGINE
#include "database/engine/rrdengineapi.h"
#endif

// ----------------------------------------------------------------------------
// RRDDIM index

int rrddim_compare(void* a, void* b) {
    if(((RRDDIM *)a)->hash < ((RRDDIM *)b)->hash) return -1;
    else if(((RRDDIM *)a)->hash > ((RRDDIM *)b)->hash) return 1;
    else return strcmp(((RRDDIM *)a)->id, ((RRDDIM *)b)->id);
}

#define rrddim_index_add(st, rd) (RRDDIM *)avl_insert_lock(&((st)->dimensions_index), (avl_t *)(rd))
#define rrddim_index_del(st,rd ) (RRDDIM *)avl_remove_lock(&((st)->dimensions_index), (avl_t *)(rd))

static inline RRDDIM *rrddim_index_find(RRDSET *st, const char *id, uint32_t hash) {
    RRDDIM tmp = {
            .id = id,
            .hash = (hash)?hash:simple_hash(id)
    };
    return (RRDDIM *)avl_search_lock(&(st->dimensions_index), (avl_t *) &tmp);
}


// ----------------------------------------------------------------------------
// RRDDIM - find a dimension

inline RRDDIM *rrddim_find(RRDSET *st, const char *id) {
    debug(D_RRD_CALLS, "rrddim_find() for chart %s, dimension %s", st->name, id);

    return rrddim_index_find(st, id, 0);
}


// ----------------------------------------------------------------------------
// RRDDIM rename a dimension

inline int rrddim_set_name(RRDSET *st, RRDDIM *rd, const char *name) {
    if(unlikely(!name || !*name || !strcmp(rd->name, name)))
        return 0;

    debug(D_RRD_CALLS, "rrddim_set_name() from %s.%s to %s.%s", st->name, rd->name, st->name, name);

    char varname[CONFIG_MAX_NAME + 1];
    snprintfz(varname, CONFIG_MAX_NAME, "dim %s name", rd->id);
    rd->name = config_set_default(st->config_section, varname, name);
    rd->hash_name = simple_hash(rd->name);

    if (!st->state->is_ar_chart)
        rrddimvar_rename_all(rd);

    rd->exposed = 0;
    rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_EXPOSED);

    ml_dimension_update_name(st, rd, name);

    return 1;
}

inline int rrddim_set_algorithm(RRDSET *st, RRDDIM *rd, RRD_ALGORITHM algorithm) {
    if(unlikely(rd->algorithm == algorithm))
        return 0;

    debug(D_RRD_CALLS, "Updating algorithm of dimension '%s/%s' from %s to %s", st->id, rd->name, rrd_algorithm_name(rd->algorithm), rrd_algorithm_name(algorithm));
    rd->algorithm = algorithm;
    rd->exposed = 0;
    rrdset_flag_set(st, RRDSET_FLAG_HOMOGENEOUS_CHECK);
    rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_EXPOSED);
    return 1;
}

inline int rrddim_set_multiplier(RRDSET *st, RRDDIM *rd, collected_number multiplier) {
    if(unlikely(rd->multiplier == multiplier))
        return 0;

    debug(D_RRD_CALLS, "Updating multiplier of dimension '%s/%s' from " COLLECTED_NUMBER_FORMAT " to " COLLECTED_NUMBER_FORMAT, st->id, rd->name, rd->multiplier, multiplier);
    rd->multiplier = multiplier;
    rd->exposed = 0;
    rrdset_flag_set(st, RRDSET_FLAG_HOMOGENEOUS_CHECK);
    rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_EXPOSED);
    return 1;
}

inline int rrddim_set_divisor(RRDSET *st, RRDDIM *rd, collected_number divisor) {
    if(unlikely(rd->divisor == divisor))
        return 0;

    debug(D_RRD_CALLS, "Updating divisor of dimension '%s/%s' from " COLLECTED_NUMBER_FORMAT " to " COLLECTED_NUMBER_FORMAT, st->id, rd->name, rd->divisor, divisor);
    rd->divisor = divisor;
    rd->exposed = 0;
    rrdset_flag_set(st, RRDSET_FLAG_HOMOGENEOUS_CHECK);
    rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_EXPOSED);
    return 1;
}

// ----------------------------------------------------------------------------
// RRDDIM create a dimension

void rrdcalc_link_to_rrddim(RRDDIM *rd, RRDSET *st, RRDHOST *host) {
    RRDCALC *rrdc;
    for (rrdc = host->alarms_with_foreach; rrdc ; rrdc = rrdc->next) {
        if (simple_pattern_matches(rrdc->spdim, rd->id) || simple_pattern_matches(rrdc->spdim, rd->name)) {
            if (rrdc->hash_chart == st->hash_name || !strcmp(rrdc->chart, st->name) || !strcmp(rrdc->chart, st->id)) {
                char *name = alarm_name_with_dim(rrdc->name, strlen(rrdc->name), rd->name, strlen(rd->name));
                if (name) {
                    if(rrdcalc_exists(host, st->name, name, 0, 0)){
                        freez(name);
                        continue;
                    }

                    RRDCALC *child = rrdcalc_create_from_rrdcalc(rrdc, host, name, rd->name);
                    if (child) {
                        rrdcalc_add_to_host(host, child);
                        RRDCALC *rdcmp  = (RRDCALC *) avl_insert_lock(&(host)->alarms_idx_health_log,(avl_t *)child);
                        if (rdcmp != child) {
                            error("Cannot insert the alarm index ID %s",child->name);
                        }
                    } else {
                        error("Cannot allocate a new alarm.");
                        rrdc->foreachcounter--;
                    }
                }
            }
        }
    }
#ifdef ENABLE_ACLK
    rrdset_flag_clear(st, RRDSET_FLAG_ACLK);
#endif
}

RRDDIM *rrddim_add_custom(RRDSET *st, const char *id, const char *name, collected_number multiplier,
                          collected_number divisor, RRD_ALGORITHM algorithm, RRD_MEMORY_MODE memory_mode)
{
    RRDHOST *host = st->rrdhost;
    rrdset_wrlock(st);

    rrdset_flag_set(st, RRDSET_FLAG_SYNC_CLOCK);
    rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_EXPOSED);

    RRDDIM *rd = rrddim_find(st, id);
    if(unlikely(rd)) {
        debug(D_RRD_CALLS, "Cannot create rrd dimension '%s/%s', it already exists.", st->id, name?name:"<NONAME>");

        int rc = rrddim_set_name(st, rd, name);
        rc += rrddim_set_algorithm(st, rd, algorithm);
        rc += rrddim_set_multiplier(st, rd, multiplier);
        rc += rrddim_set_divisor(st, rd, divisor);
        if (rrddim_flag_check(rd, RRDDIM_FLAG_ARCHIVED)) {
            store_active_dimension(&rd->state->metric_uuid);
            rd->state->collect_ops.init(rd);
            rrddim_flag_clear(rd, RRDDIM_FLAG_ARCHIVED);
            rrddimvar_create(rd, RRDVAR_TYPE_CALCULATED, NULL, NULL, &rd->last_stored_value, RRDVAR_OPTION_DEFAULT);
            rrddimvar_create(rd, RRDVAR_TYPE_COLLECTED, NULL, "_raw", &rd->last_collected_value, RRDVAR_OPTION_DEFAULT);
            rrddimvar_create(rd, RRDVAR_TYPE_TIME_T, NULL, "_last_collected_t", &rd->last_collected_time.tv_sec, RRDVAR_OPTION_DEFAULT);

            rrddim_flag_set(rd, RRDDIM_FLAG_PENDING_FOREACH_ALARM);
            rrdset_flag_set(st, RRDSET_FLAG_PENDING_FOREACH_ALARMS);
            rrdhost_flag_set(host, RRDHOST_FLAG_PENDING_FOREACH_ALARMS);
        }
        if (unlikely(rc)) {
            debug(D_METADATALOG, "DIMENSION [%s] metadata updated", rd->id);
            (void)sql_store_dimension(&rd->state->metric_uuid, rd->rrdset->chart_uuid, rd->id, rd->name, rd->multiplier, rd->divisor,
                                      rd->algorithm);
        }
        rrdset_unlock(st);
        return rd;
    }

    char filename[FILENAME_MAX + 1];
    char fullfilename[FILENAME_MAX + 1];

    debug(D_RRD_CALLS, "Adding dimension '%s/%s'.", st->id, id);

    rrdset_strncpyz_name(filename, id, FILENAME_MAX);
    snprintfz(fullfilename, FILENAME_MAX, "%s/%s.db", st->cache_dir, filename);

    STORAGE_ENGINE* eng = engine_get(memory_mode);
    rd = eng->api.dim_ops.create(st, id, fullfilename, multiplier, divisor, algorithm);

    strcpy(rd->magic, RRDDIMENSION_MAGIC);

    rd->id = strdupz(id);
    rd->hash = simple_hash(rd->id);

    rd->cache_filename = strdupz(fullfilename);

    rd->name = (name && *name)?strdupz(name):strdupz(rd->id);
    rd->hash_name = simple_hash(rd->name);

    rd->algorithm = algorithm;

    rd->multiplier = multiplier;

    rd->divisor = divisor;
    if(!rd->divisor) rd->divisor = 1;

    rd->entries = st->entries;
    rd->update_every = st->update_every;

    if(rrdset_flag_check(st, RRDSET_FLAG_STORE_FIRST))
        rd->collections_counter = 1;
    else
        rd->collections_counter = 0;

    rd->updated = 0;
    rd->flags = 0x00000000;

    rd->calculated_value = 0;
    rd->last_calculated_value = 0;
    rd->collected_value = 0;
    rd->last_collected_value = 0;
    rd->collected_value_max = 0;
    rd->collected_volume = 0;
    rd->stored_volume = 0;
    rd->last_stored_value = 0;
    rd->last_collected_time.tv_sec = 0;
    rd->last_collected_time.tv_usec = 0;
    rd->rrdset = st;
    rd->state = callocz(1, sizeof(*rd->state));
#ifdef ENABLE_ACLK
    rd->state->aclk_live_status = -1;
#endif
    (void) find_dimension_uuid(st, rd, &(rd->state->metric_uuid));

    rd->state->collect_ops = eng->api.collect_ops;
    rd->state->query_ops = eng->api.query_ops;

    if(eng->api.dim_ops.init) {
        eng->api.dim_ops.init(rd);
    }
    store_active_dimension(&rd->state->metric_uuid);
    rd->state->collect_ops.init(rd);
    // append this dimension
    if(!st->dimensions)
        st->dimensions = rd;
    else {
        RRDDIM *td = st->dimensions;

        if(td->algorithm != rd->algorithm || ABS(td->multiplier) != ABS(rd->multiplier) || ABS(td->divisor) != ABS(rd->divisor)) {
            if(!rrdset_flag_check(st, RRDSET_FLAG_HETEROGENEOUS)) {
                #ifdef NETDATA_INTERNAL_CHECKS
                info("Dimension '%s' added on chart '%s' of host '%s' is not homogeneous to other dimensions already present (algorithm is '%s' vs '%s', multiplier is " COLLECTED_NUMBER_FORMAT " vs " COLLECTED_NUMBER_FORMAT ", divisor is " COLLECTED_NUMBER_FORMAT " vs " COLLECTED_NUMBER_FORMAT ").",
                        rd->name,
                        st->name,
                        host->hostname,
                        rrd_algorithm_name(rd->algorithm), rrd_algorithm_name(td->algorithm),
                        rd->multiplier, td->multiplier,
                        rd->divisor, td->divisor
                );
                #endif
                rrdset_flag_set(st, RRDSET_FLAG_HETEROGENEOUS);
            }
        }

        for(; td->next; td = td->next) ;
        td->next = rd;
    }

    if(host->health_enabled && !st->state->is_ar_chart) {
        rrddimvar_create(rd, RRDVAR_TYPE_CALCULATED, NULL, NULL, &rd->last_stored_value, RRDVAR_OPTION_DEFAULT);
        rrddimvar_create(rd, RRDVAR_TYPE_COLLECTED, NULL, "_raw", &rd->last_collected_value, RRDVAR_OPTION_DEFAULT);
        rrddimvar_create(rd, RRDVAR_TYPE_TIME_T, NULL, "_last_collected_t", &rd->last_collected_time.tv_sec, RRDVAR_OPTION_DEFAULT);
    }

    if(unlikely(rrddim_index_add(st, rd) != rd))
        error("RRDDIM: INTERNAL ERROR: attempt to index duplicate dimension '%s' on chart '%s'", rd->id, st->id);

    rrddim_flag_set(rd, RRDDIM_FLAG_PENDING_FOREACH_ALARM);
    rrdset_flag_set(st, RRDSET_FLAG_PENDING_FOREACH_ALARMS);
    rrdhost_flag_set(host, RRDHOST_FLAG_PENDING_FOREACH_ALARMS);

    ml_new_dimension(rd);

    rrdset_unlock(st);
#ifdef ENABLE_ACLK
    rrdset_flag_clear(st, RRDSET_FLAG_ACLK);
#endif
    return(rd);
}

// ----------------------------------------------------------------------------
// RRDDIM remove / free a dimension

void rrddim_free_custom(RRDSET *st, RRDDIM *rd, int db_rotated)
{
    ml_delete_dimension(rd);

#ifndef ENABLE_ACLK
    UNUSED(db_rotated);
#endif
    debug(D_RRD_CALLS, "rrddim_free() %s.%s", st->name, rd->name);

    if (!rrddim_flag_check(rd, RRDDIM_FLAG_ARCHIVED)) {
        uint8_t can_delete_metric = rd->state->collect_ops.finalize(rd);
        if (can_delete_metric && rd->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
            /* This metric has no data and no references */
            delete_dimension_uuid(&rd->state->metric_uuid);
        }
    }

    if(rd == st->dimensions)
        st->dimensions = rd->next;
    else {
        RRDDIM *i;
        for (i = st->dimensions; i && i->next != rd; i = i->next) ;

        if (i && i->next == rd)
            i->next = rd->next;
        else
            error("Request to free dimension '%s.%s' but it is not linked.", st->id, rd->name);
    }
    rd->next = NULL;

    while(rd->variables)
        rrddimvar_free(rd->variables);

    if(unlikely(rrddim_index_del(st, rd) != rd))
        error("RRDDIM: INTERNAL ERROR: attempt to remove from index dimension '%s' on chart '%s', removed a different dimension.", rd->id, st->id);

    // free(rd->annotations);
#if defined(ENABLE_ACLK) && defined(ENABLE_NEW_CLOUD_PROTOCOL)
    if (!netdata_exit)
        aclk_send_dimension_update(rd);
#endif

    RRD_MEMORY_MODE rrd_memory_mode = rd->rrd_memory_mode;
    switch(rrd_memory_mode) {
        case RRD_MEMORY_MODE_SAVE:
        case RRD_MEMORY_MODE_MAP:
        case RRD_MEMORY_MODE_RAM:
            debug(D_RRD_CALLS, "Unmapping dimension '%s'.", rd->name);
            freez((void *)rd->id);
            freez((void *)rd->name);
            freez(rd->cache_filename);
            freez(rd->state);
            munmap(rd, rd->memsize);
            break;

        case RRD_MEMORY_MODE_ALLOC:
        case RRD_MEMORY_MODE_NONE:
        case RRD_MEMORY_MODE_DBENGINE:
            debug(D_RRD_CALLS, "Removing dimension '%s'.", rd->name);
            freez((void *)rd->id);
            freez((void *)rd->name);
            freez(rd->cache_filename);
            freez(rd->state);
            freez(rd);
            break;
    }
#ifdef ENABLE_ACLK
    if (db_rotated || RRD_MEMORY_MODE_DBENGINE != rrd_memory_mode)
        rrdset_flag_clear(st, RRDSET_FLAG_ACLK);
#endif
}


// ----------------------------------------------------------------------------
// RRDDIM - set dimension options

int rrddim_hide(RRDSET *st, const char *id) {
    debug(D_RRD_CALLS, "rrddim_hide() for chart %s, dimension %s", st->name, id);

    RRDHOST *host = st->rrdhost;

    RRDDIM *rd = rrddim_find(st, id);
    if(unlikely(!rd)) {
        error("Cannot find dimension with id '%s' on stats '%s' (%s) on host '%s'.", id, st->name, st->id, host->hostname);
        return 1;
    }
    (void) sql_set_dimension_option(&rd->state->metric_uuid, "hidden");

    rrddim_flag_set(rd, RRDDIM_FLAG_HIDDEN);
#ifdef ENABLE_ACLK
    rrdset_flag_clear(st, RRDSET_FLAG_ACLK);
#endif
    return 0;
}

int rrddim_unhide(RRDSET *st, const char *id) {
    debug(D_RRD_CALLS, "rrddim_unhide() for chart %s, dimension %s", st->name, id);

    RRDHOST *host = st->rrdhost;
    RRDDIM *rd = rrddim_find(st, id);
    if(unlikely(!rd)) {
        error("Cannot find dimension with id '%s' on stats '%s' (%s) on host '%s'.", id, st->name, st->id, host->hostname);
        return 1;
    }
    (void) sql_set_dimension_option(&rd->state->metric_uuid, NULL);

    rrddim_flag_clear(rd, RRDDIM_FLAG_HIDDEN);
#ifdef ENABLE_ACLK
    rrdset_flag_clear(st, RRDSET_FLAG_ACLK);
#endif
    return 0;
}

inline void rrddim_is_obsolete(RRDSET *st, RRDDIM *rd) {
    debug(D_RRD_CALLS, "rrddim_is_obsolete() for chart %s, dimension %s", st->name, rd->name);

    if(unlikely(rrddim_flag_check(rd, RRDDIM_FLAG_ARCHIVED))) {
        info("Cannot obsolete already archived dimension %s from chart %s", rd->name, st->name);
        return;
    }
    rrddim_flag_set(rd, RRDDIM_FLAG_OBSOLETE);
    rrdset_flag_set(st, RRDSET_FLAG_OBSOLETE_DIMENSIONS);
#ifdef ENABLE_ACLK
    rrdset_flag_clear(st, RRDSET_FLAG_ACLK);
#endif
}

inline void rrddim_isnot_obsolete(RRDSET *st __maybe_unused, RRDDIM *rd) {
    debug(D_RRD_CALLS, "rrddim_isnot_obsolete() for chart %s, dimension %s", st->name, rd->name);

    rrddim_flag_clear(rd, RRDDIM_FLAG_OBSOLETE);
#ifdef ENABLE_ACLK
    rrdset_flag_clear(st, RRDSET_FLAG_ACLK);
#endif
}

// ----------------------------------------------------------------------------
// RRDDIM - collect values for a dimension

inline collected_number rrddim_set_by_pointer(RRDSET *st __maybe_unused, RRDDIM *rd, collected_number value) {
    debug(D_RRD_CALLS, "rrddim_set_by_pointer() for chart %s, dimension %s, value " COLLECTED_NUMBER_FORMAT, st->name, rd->name, value);

    now_realtime_timeval(&rd->last_collected_time);
    rd->collected_value = value;
    rd->updated = 1;

    rd->collections_counter++;

    collected_number v = (value >= 0) ? value : -value;
    if(unlikely(v > rd->collected_value_max)) rd->collected_value_max = v;

    // fprintf(stderr, "%s.%s %llu " COLLECTED_NUMBER_FORMAT " dt %0.6f" " rate " CALCULATED_NUMBER_FORMAT "\n", st->name, rd->name, st->usec_since_last_update, value, (float)((double)st->usec_since_last_update / (double)1000000), (calculated_number)((value - rd->last_collected_value) * (calculated_number)rd->multiplier / (calculated_number)rd->divisor * 1000000.0 / (calculated_number)st->usec_since_last_update));

    return rd->last_collected_value;
}

collected_number rrddim_set(RRDSET *st, const char *id, collected_number value) {
    RRDHOST *host = st->rrdhost;
    RRDDIM *rd = rrddim_find(st, id);
    if(unlikely(!rd)) {
        error("Cannot find dimension with id '%s' on stats '%s' (%s) on host '%s'.", id, st->name, st->id, host->hostname);
        return 0;
    }

    return rrddim_set_by_pointer(st, rd, value);
}

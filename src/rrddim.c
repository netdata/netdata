#define NETDATA_RRD_INTERNALS 1
#include "common.h"

// ----------------------------------------------------------------------------
// RRDDIM index

int rrddim_compare(void* a, void* b) {
    if(((RRDDIM *)a)->hash < ((RRDDIM *)b)->hash) return -1;
    else if(((RRDDIM *)a)->hash > ((RRDDIM *)b)->hash) return 1;
    else return strcmp(((RRDDIM *)a)->id, ((RRDDIM *)b)->id);
}

#define rrddim_index_add(st, rd) (RRDDIM *)avl_insert_lock(&((st)->dimensions_index), (avl *)(rd))
#define rrddim_index_del(st,rd ) (RRDDIM *)avl_remove_lock(&((st)->dimensions_index), (avl *)(rd))

static inline RRDDIM *rrddim_index_find(RRDSET *st, const char *id, uint32_t hash) {
    RRDDIM tmp = {
            .id = id,
            .hash = (hash)?hash:simple_hash(id)
    };
    return (RRDDIM *)avl_search_lock(&(st->dimensions_index), (avl *) &tmp);
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
    rrddimvar_rename_all(rd);
    rd->exposed = 0;
    return 1;
}

inline int rrddim_set_algorithm(RRDSET *st, RRDDIM *rd, RRD_ALGORITHM algorithm) {
    if(unlikely(rd->algorithm == algorithm))
        return 0;

    debug(D_RRD_CALLS, "Updating algorithm of dimension '%s/%s' from %s to %s", st->id, rd->name, rrd_algorithm_name(rd->algorithm), rrd_algorithm_name(algorithm));
    rd->algorithm = algorithm;
    rd->exposed = 0;
    rrdset_flag_set(st, RRDSET_FLAG_HOMEGENEOUS_CHECK);
    return 1;
}

inline int rrddim_set_multiplier(RRDSET *st, RRDDIM *rd, collected_number multiplier) {
    if(unlikely(rd->multiplier == multiplier))
        return 0;

    debug(D_RRD_CALLS, "Updating multiplier of dimension '%s/%s' from " COLLECTED_NUMBER_FORMAT " to " COLLECTED_NUMBER_FORMAT, st->id, rd->name, rd->multiplier, multiplier);
    rd->multiplier = multiplier;
    rd->exposed = 0;
    rrdset_flag_set(st, RRDSET_FLAG_HOMEGENEOUS_CHECK);
    return 1;
}

inline int rrddim_set_divisor(RRDSET *st, RRDDIM *rd, collected_number divisor) {
    if(unlikely(rd->divisor == divisor))
        return 0;

    debug(D_RRD_CALLS, "Updating divisor of dimension '%s/%s' from " COLLECTED_NUMBER_FORMAT " to " COLLECTED_NUMBER_FORMAT, st->id, rd->name, rd->divisor, divisor);
    rd->divisor = divisor;
    rd->exposed = 0;
    rrdset_flag_set(st, RRDSET_FLAG_HOMEGENEOUS_CHECK);
    return 1;
}

// ----------------------------------------------------------------------------
// RRDDIM create a dimension

RRDDIM *rrddim_add_custom(RRDSET *st, const char *id, const char *name, collected_number multiplier, collected_number divisor, RRD_ALGORITHM algorithm, RRD_MEMORY_MODE memory_mode) {
    RRDDIM *rd = rrddim_find(st, id);
    if(unlikely(rd)) {
        debug(D_RRD_CALLS, "Cannot create rrd dimension '%s/%s', it already exists.", st->id, name?name:"<NONAME>");

        rrddim_set_name(st, rd, name);
        rrddim_set_algorithm(st, rd, algorithm);
        rrddim_set_multiplier(st, rd, multiplier);
        rrddim_set_divisor(st, rd, divisor);

        return rd;
    }

    char filename[FILENAME_MAX + 1];
    char fullfilename[FILENAME_MAX + 1];

    char varname[CONFIG_MAX_NAME + 1];
    unsigned long size = sizeof(RRDDIM) + (st->entries * sizeof(storage_number));

    debug(D_RRD_CALLS, "Adding dimension '%s/%s'.", st->id, id);

    rrdset_strncpyz_name(filename, id, FILENAME_MAX);
    snprintfz(fullfilename, FILENAME_MAX, "%s/%s.db", st->cache_dir, filename);

    if(memory_mode == RRD_MEMORY_MODE_SAVE || memory_mode == RRD_MEMORY_MODE_MAP || memory_mode == RRD_MEMORY_MODE_RAM) {
        rd = (RRDDIM *)mymmap(
                  (memory_mode == RRD_MEMORY_MODE_RAM)?NULL:fullfilename
                , size
                , ((memory_mode == RRD_MEMORY_MODE_MAP) ? MAP_SHARED : MAP_PRIVATE)
                , 1
        );

        if(likely(rd)) {
            // we have a file mapped for rd

            memset(&rd->avl, 0, sizeof(avl));
            rd->id = NULL;
            rd->name = NULL;
            rd->cache_filename = NULL;
            rd->variables = NULL;
            rd->next = NULL;
            rd->rrdset = NULL;
            rd->exposed = 0;

            struct timeval now;
            now_realtime_timeval(&now);

            if(memory_mode == RRD_MEMORY_MODE_RAM) {
                memset(rd, 0, size);
            }
            else {
                int reset = 0;

                if(strcmp(rd->magic, RRDDIMENSION_MAGIC) != 0) {
                    info("Initializing file %s.", fullfilename);
                    memset(rd, 0, size);
                    reset = 1;
                }
                else if(rd->memsize != size) {
                    error("File %s does not have the desired size, expected %lu but found %lu. Clearing it.", fullfilename, size, rd->memsize);
                    memset(rd, 0, size);
                    reset = 1;
                }
                else if(rd->update_every != st->update_every) {
                    error("File %s does not have the same update frequency, expected %d but found %d. Clearing it.", fullfilename, st->update_every, rd->update_every);
                    memset(rd, 0, size);
                    reset = 1;
                }
                else if(dt_usec(&now, &rd->last_collected_time) > (rd->entries * rd->update_every * USEC_PER_SEC)) {
                    info("File %s is too old (last collected %llu seconds ago, but the database is %ld seconds). Clearing it.", fullfilename, dt_usec(&now, &rd->last_collected_time) / USEC_PER_SEC, rd->entries * rd->update_every);
                    memset(rd, 0, size);
                    reset = 1;
                }

                if(!reset) {
                    if(rd->algorithm != algorithm) {
                        info("File %s does not have the expected algorithm (expected %u '%s', found %u '%s'). Previous values may be wrong.",
                              fullfilename, algorithm, rrd_algorithm_name(algorithm), rd->algorithm, rrd_algorithm_name(rd->algorithm));
                    }

                    if(rd->multiplier != multiplier) {
                        info("File %s does not have the expected multiplier (expected " COLLECTED_NUMBER_FORMAT ", found " COLLECTED_NUMBER_FORMAT "). Previous values may be wrong.", fullfilename, multiplier, rd->multiplier);
                    }

                    if(rd->divisor != divisor) {
                        info("File %s does not have the expected divisor (expected " COLLECTED_NUMBER_FORMAT ", found " COLLECTED_NUMBER_FORMAT "). Previous values may be wrong.", fullfilename, divisor, rd->divisor);
                    }
                }
            }

            // make sure we have the right memory mode
            // even if we cleared the memory
            rd->rrd_memory_mode = memory_mode;
        }
    }

    if(unlikely(!rd)) {
        // if we didn't manage to get a mmap'd dimension, just create one
        rd = callocz(1, size);
        rd->rrd_memory_mode = (memory_mode == RRD_MEMORY_MODE_NONE) ? RRD_MEMORY_MODE_NONE : RRD_MEMORY_MODE_ALLOC;
    }

    rd->memsize = size;

    strcpy(rd->magic, RRDDIMENSION_MAGIC);

    rd->id = strdupz(id);
    rd->hash = simple_hash(rd->id);

    rd->cache_filename = strdupz(fullfilename);

    snprintfz(varname, CONFIG_MAX_NAME, "dim %s name", rd->id);
    rd->name = config_get(st->config_section, varname, (name && *name)?name:rd->id);
    rd->hash_name = simple_hash(rd->name);

    snprintfz(varname, CONFIG_MAX_NAME, "dim %s algorithm", rd->id);
    rd->algorithm = rrd_algorithm_id(config_get(st->config_section, varname, rrd_algorithm_name(algorithm)));

    snprintfz(varname, CONFIG_MAX_NAME, "dim %s multiplier", rd->id);
    rd->multiplier = config_get_number(st->config_section, varname, multiplier);

    snprintfz(varname, CONFIG_MAX_NAME, "dim %s divisor", rd->id);
    rd->divisor = config_get_number(st->config_section, varname, divisor);
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
    rd->collected_volume = 0;
    rd->stored_volume = 0;
    rd->last_stored_value = 0;
    rd->values[st->current_entry] = SN_EMPTY_SLOT; // pack_storage_number(0, SN_NOT_EXISTS);
    rd->last_collected_time.tv_sec = 0;
    rd->last_collected_time.tv_usec = 0;
    rd->rrdset = st;

    // append this dimension
    rrdset_wrlock(st);
    if(!st->dimensions)
        st->dimensions = rd;
    else {
        RRDDIM *td = st->dimensions;

        if(td->algorithm != rd->algorithm || abs(td->multiplier) != abs(rd->multiplier) || abs(td->divisor) != abs(rd->divisor)) {
            if(!rrdset_flag_check(st, RRDSET_FLAG_HETEROGENEOUS)) {
                #ifdef NETDATA_INTERNAL_CHECKS
                info("Dimension '%s' added on chart '%s' of host '%s' is not homogeneous to other dimensions already present (algorithm is '%s' vs '%s', multiplier is " COLLECTED_NUMBER_FORMAT " vs " COLLECTED_NUMBER_FORMAT ", divisor is " COLLECTED_NUMBER_FORMAT " vs " COLLECTED_NUMBER_FORMAT ").",
                        rd->name,
                        st->name,
                        st->rrdhost->hostname,
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

    if(st->rrdhost->health_enabled) {
        rrddimvar_create(rd, RRDVAR_TYPE_CALCULATED, NULL, NULL, &rd->last_stored_value, RRDVAR_OPTION_DEFAULT);
        rrddimvar_create(rd, RRDVAR_TYPE_COLLECTED, NULL, "_raw", &rd->last_collected_value, RRDVAR_OPTION_DEFAULT);
        rrddimvar_create(rd, RRDVAR_TYPE_TIME_T, NULL, "_last_collected_t", &rd->last_collected_time.tv_sec, RRDVAR_OPTION_DEFAULT);
    }

    rrdset_unlock(st);

    if(unlikely(rrddim_index_add(st, rd) != rd))
        error("RRDDIM: INTERNAL ERROR: attempt to index duplicate dimension '%s' on chart '%s'", rd->id, st->id);

    return(rd);
}

// ----------------------------------------------------------------------------
// RRDDIM remove / free a dimension

void rrddim_free(RRDSET *st, RRDDIM *rd)
{
    debug(D_RRD_CALLS, "rrddim_free() %s.%s", st->name, rd->name);

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

    switch(rd->rrd_memory_mode) {
        case RRD_MEMORY_MODE_SAVE:
        case RRD_MEMORY_MODE_MAP:
        case RRD_MEMORY_MODE_RAM:
            debug(D_RRD_CALLS, "Unmapping dimension '%s'.", rd->name);
            freez((void *)rd->id);
            freez(rd->cache_filename);
            munmap(rd, rd->memsize);
            break;

        case RRD_MEMORY_MODE_ALLOC:
        case RRD_MEMORY_MODE_NONE:
            debug(D_RRD_CALLS, "Removing dimension '%s'.", rd->name);
            freez((void *)rd->id);
            freez(rd->cache_filename);
            freez(rd);
            break;
    }
}


// ----------------------------------------------------------------------------
// RRDDIM - set dimension options

int rrddim_hide(RRDSET *st, const char *id) {
    debug(D_RRD_CALLS, "rrddim_hide() for chart %s, dimension %s", st->name, id);

    RRDDIM *rd = rrddim_find(st, id);
    if(unlikely(!rd)) {
        error("Cannot find dimension with id '%s' on stats '%s' (%s) on host '%s'.", id, st->name, st->id, st->rrdhost->hostname);
        return 1;
    }

    rrddim_flag_set(rd, RRDDIM_FLAG_HIDDEN);
    return 0;
}

int rrddim_unhide(RRDSET *st, const char *id) {
    debug(D_RRD_CALLS, "rrddim_unhide() for chart %s, dimension %s", st->name, id);

    RRDDIM *rd = rrddim_find(st, id);
    if(unlikely(!rd)) {
        error("Cannot find dimension with id '%s' on stats '%s' (%s) on host '%s'.", id, st->name, st->id, st->rrdhost->hostname);
        return 1;
    }

    rrddim_flag_clear(rd, RRDDIM_FLAG_HIDDEN);
    return 0;
}


// ----------------------------------------------------------------------------
// RRDDIM - collect values for a dimension

inline collected_number rrddim_set_by_pointer(RRDSET *st, RRDDIM *rd, collected_number value) {
    debug(D_RRD_CALLS, "rrddim_set_by_pointer() for chart %s, dimension %s, value " COLLECTED_NUMBER_FORMAT, st->name, rd->name, value);

    now_realtime_timeval(&rd->last_collected_time);
    rd->collected_value = value;
    rd->updated = 1;

    rd->collections_counter++;

    // fprintf(stderr, "%s.%s %llu " COLLECTED_NUMBER_FORMAT " dt %0.6f" " rate " CALCULATED_NUMBER_FORMAT "\n", st->name, rd->name, st->usec_since_last_update, value, (float)((double)st->usec_since_last_update / (double)1000000), (calculated_number)((value - rd->last_collected_value) * (calculated_number)rd->multiplier / (calculated_number)rd->divisor * 1000000.0 / (calculated_number)st->usec_since_last_update));

    return rd->last_collected_value;
}

collected_number rrddim_set(RRDSET *st, const char *id, collected_number value) {
    RRDDIM *rd = rrddim_find(st, id);
    if(unlikely(!rd)) {
        error("Cannot find dimension with id '%s' on stats '%s' (%s) on host '%s'.", id, st->name, st->id, st->rrdhost->hostname);
        return 0;
    }

    return rrddim_set_by_pointer(st, rd, value);
}

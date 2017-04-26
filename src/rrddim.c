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

inline void rrddim_set_name(RRDSET *st, RRDDIM *rd, const char *name) {
    if(unlikely(!strcmp(rd->name, name)))
        return;

    debug(D_RRD_CALLS, "rrddim_set_name() from %s.%s to %s.%s", st->name, rd->name, st->name, name);

    char varname[CONFIG_MAX_NAME + 1];
    snprintfz(varname, CONFIG_MAX_NAME, "dim %s name", rd->id);
    rd->name = config_set_default(st->config_section, varname, name);
    rd->hash_name = simple_hash(rd->name);

    rrddimvar_rename_all(rd);
}


// ----------------------------------------------------------------------------
// RRDDIM create a dimension

RRDDIM *rrddim_add_custom(RRDSET *st, const char *id, const char *name, collected_number multiplier, collected_number divisor, RRD_ALGORITHM algorithm, RRD_MEMORY_MODE memory_mode) {
    RRDDIM *rd = rrddim_find(st, id);
    if(unlikely(rd)) {
        debug(D_RRD_CALLS, "Cannot create rrd dimension '%s/%s', it already exists.", st->id, name?name:"<NONAME>");
        return rd;
    }

    char filename[FILENAME_MAX + 1];
    char fullfilename[FILENAME_MAX + 1];

    char varname[CONFIG_MAX_NAME + 1];
    unsigned long size = sizeof(RRDDIM) + (st->entries * sizeof(storage_number));

    debug(D_RRD_CALLS, "Adding dimension '%s/%s'.", st->id, id);

    rrdset_strncpyz_name(filename, id, FILENAME_MAX);
    snprintfz(fullfilename, FILENAME_MAX, "%s/%s.db", st->cache_dir, filename);

    if(memory_mode == RRD_MEMORY_MODE_SAVE || memory_mode == RRD_MEMORY_MODE_MAP) {
        rd = (RRDDIM *)mymmap(fullfilename, size, ((memory_mode == RRD_MEMORY_MODE_MAP) ? MAP_SHARED : MAP_PRIVATE), 1);
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

            if(strcmp(rd->magic, RRDDIMENSION_MAGIC) != 0) {
                errno = 0;
                info("Initializing file %s.", fullfilename);
                memset(rd, 0, size);
            }
            else if(rd->memsize != size) {
                errno = 0;
                error("File %s does not have the desired size. Clearing it.", fullfilename);
                memset(rd, 0, size);
            }
            else if(rd->multiplier != multiplier) {
                errno = 0;
                error("File %s does not have the same multiplier. Clearing it.", fullfilename);
                memset(rd, 0, size);
            }
            else if(rd->divisor != divisor) {
                errno = 0;
                error("File %s does not have the same divisor. Clearing it.", fullfilename);
                memset(rd, 0, size);
            }
            else if(rd->update_every != st->update_every) {
                errno = 0;
                error("File %s does not have the same refresh frequency. Clearing it.", fullfilename);
                memset(rd, 0, size);
            }
            else if(dt_usec(&now, &rd->last_collected_time) > (rd->entries * rd->update_every * USEC_PER_SEC)) {
                errno = 0;
                error("File %s is too old. Clearing it.", fullfilename);
                memset(rd, 0, size);
            }

            if(rd->algorithm && rd->algorithm != algorithm)
                error("File %s does not have the expected algorithm (expected %u '%s', found %u '%s'). Previous values may be wrong."
                      , fullfilename, algorithm, rrd_algorithm_name(algorithm), rd->algorithm,
                        rrd_algorithm_name(rd->algorithm));

            // make sure we have the right memory mode
            // even if we cleared the memory
            rd->rrd_memory_mode = memory_mode;
        }
    }

    if(unlikely(!rd)) {
        // if we didn't manage to get a mmap'd dimension, just create one
        rd = callocz(1, size);
        rd->rrd_memory_mode = (memory_mode == RRD_MEMORY_MODE_NONE) ? RRD_MEMORY_MODE_NONE : RRD_MEMORY_MODE_RAM;
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

    // prevent incremental calculation spikes
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
    rd->values[st->current_entry] = pack_storage_number(0, SN_NOT_EXISTS);
    rd->last_collected_time.tv_sec = 0;
    rd->last_collected_time.tv_usec = 0;
    rd->rrdset = st;

    // append this dimension
    rrdset_wrlock(st);
    if(!st->dimensions)
        st->dimensions = rd;
    else {
        RRDDIM *td = st->dimensions;
        for(; td->next; td = td->next) ;
        td->next = rd;
    }

    if(st->rrdhost->health_enabled) {
        rrddimvar_create(rd, RRDVAR_TYPE_CALCULATED, NULL, NULL, &rd->last_stored_value, 0);
        rrddimvar_create(rd, RRDVAR_TYPE_COLLECTED, NULL, "_raw", &rd->last_collected_value, 0);
        rrddimvar_create(rd, RRDVAR_TYPE_TIME_T, NULL, "_last_collected_t", &rd->last_collected_time.tv_sec, 0);
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
            debug(D_RRD_CALLS, "Unmapping dimension '%s'.", rd->name);
            freez((void *)rd->id);
            freez(rd->cache_filename);
            munmap(rd, rd->memsize);
            break;

        case RRD_MEMORY_MODE_NONE:
        case RRD_MEMORY_MODE_RAM:
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

// SPDX-License-Identifier: GPL-3.0-or-later
#define NETDATA_RRD_INTERNALS 1

#include "rrd.h"
#include "storage_engine.h"

// ----------------------------------------------------------------------------
// globals

/*
// if not zero it gives the time (in seconds) to remove un-updated dimensions
// DO NOT ENABLE
// if dimensions are removed, the chart generation will have to run again
int rrd_delete_unupdated_dimensions = 0;
*/

int default_rrd_update_every = UPDATE_EVERY;
int default_rrd_history_entries = RRD_DEFAULT_HISTORY_ENTRIES;
#ifdef ENABLE_DBENGINE
RRD_MEMORY_MODE default_rrd_memory_mode = RRD_MEMORY_MODE_DBENGINE;
#else
RRD_MEMORY_MODE default_rrd_memory_mode = RRD_MEMORY_MODE_SAVE;
#endif
int gap_when_lost_iterations_above = 1;


// ----------------------------------------------------------------------------
// RRD - memory modes

inline const char *rrd_memory_mode_name(RRD_MEMORY_MODE id) {
    switch(id) {
        case RRD_MEMORY_MODE_RAM:
            return RRD_MEMORY_MODE_RAM_NAME;

        case RRD_MEMORY_MODE_MAP:
            return RRD_MEMORY_MODE_MAP_NAME;

        case RRD_MEMORY_MODE_NONE:
            return RRD_MEMORY_MODE_NONE_NAME;

        case RRD_MEMORY_MODE_SAVE:
            return RRD_MEMORY_MODE_SAVE_NAME;

        case RRD_MEMORY_MODE_ALLOC:
            return RRD_MEMORY_MODE_ALLOC_NAME;

        case RRD_MEMORY_MODE_DBENGINE:
            return RRD_MEMORY_MODE_DBENGINE_NAME;
    }

    STORAGE_ENGINE* eng = storage_engine_get(id);
    if (eng) {
        return eng->name;
    }

    return RRD_MEMORY_MODE_SAVE_NAME;
}

RRD_MEMORY_MODE rrd_memory_mode_id(const char *name) {
    STORAGE_ENGINE* eng = storage_engine_find(name);
    if (eng) {
        return eng->id;
    }

    return RRD_MEMORY_MODE_SAVE;
}


// ----------------------------------------------------------------------------
// RRD - algorithms types

RRD_ALGORITHM rrd_algorithm_id(const char *name) {
    if(strcmp(name, RRD_ALGORITHM_INCREMENTAL_NAME) == 0)
        return RRD_ALGORITHM_INCREMENTAL;

    else if(strcmp(name, RRD_ALGORITHM_ABSOLUTE_NAME) == 0)
        return RRD_ALGORITHM_ABSOLUTE;

    else if(strcmp(name, RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL_NAME) == 0)
        return RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL;

    else if(strcmp(name, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL_NAME) == 0)
        return RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL;

    else
        return RRD_ALGORITHM_ABSOLUTE;
}

const char *rrd_algorithm_name(RRD_ALGORITHM algorithm) {
    switch(algorithm) {
        case RRD_ALGORITHM_ABSOLUTE:
        default:
            return RRD_ALGORITHM_ABSOLUTE_NAME;

        case RRD_ALGORITHM_INCREMENTAL:
            return RRD_ALGORITHM_INCREMENTAL_NAME;

        case RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL:
            return RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL_NAME;

        case RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL:
            return RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL_NAME;
    }
}


// ----------------------------------------------------------------------------
// RRD - chart types

inline RRDSET_TYPE rrdset_type_id(const char *name) {
    if(unlikely(strcmp(name, RRDSET_TYPE_AREA_NAME) == 0))
        return RRDSET_TYPE_AREA;

    else if(unlikely(strcmp(name, RRDSET_TYPE_STACKED_NAME) == 0))
        return RRDSET_TYPE_STACKED;

    else // if(unlikely(strcmp(name, RRDSET_TYPE_LINE_NAME) == 0))
        return RRDSET_TYPE_LINE;
}

const char *rrdset_type_name(RRDSET_TYPE chart_type) {
    switch(chart_type) {
        case RRDSET_TYPE_LINE:
        default:
            return RRDSET_TYPE_LINE_NAME;

        case RRDSET_TYPE_AREA:
            return RRDSET_TYPE_AREA_NAME;

        case RRDSET_TYPE_STACKED:
            return RRDSET_TYPE_STACKED_NAME;
    }
}

// ----------------------------------------------------------------------------
// RRD - cache directory

char *rrdset_cache_dir(RRDHOST *host, const char *id) {
    char *ret = NULL;

    char b[FILENAME_MAX + 1];
    char n[FILENAME_MAX + 1];
    rrdset_strncpyz_name(b, id, FILENAME_MAX);

    snprintfz(n, FILENAME_MAX, "%s/%s", host->cache_dir, b);
    ret = strdupz(n);

    if(host->rrd_memory_mode == RRD_MEMORY_MODE_MAP || host->rrd_memory_mode == RRD_MEMORY_MODE_SAVE) {
        int r = mkdir(ret, 0775);
        if(r != 0 && errno != EEXIST)
            error("Cannot create directory '%s'", ret);
    }

    return ret;
}

// ----------------------------------------------------------------------------
// RRD - string management

static void *rrd_string_cache_entry(const char *s, void *data) {
    (void)data;

    char *tmp = strdupz(s);
    json_fix_string(tmp);
    STRING *ret = string_strdupz(tmp);
    freez(tmp);
    return ret;
}

STRING *rrd_string_strdupz(const char *s) {
    if(unlikely(!s || !*s)) return string_strdupz(s);

    return string_dup(thread_cache_entry_get(s, rrd_string_cache_entry, NULL));
}

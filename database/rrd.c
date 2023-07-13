// SPDX-License-Identifier: GPL-3.0-or-later
#define NETDATA_RRD_INTERNALS 1

#include "rrd.h"

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

char *rrdhost_cache_dir_for_rrdset_alloc(RRDHOST *host, const char *id) {
    char *ret = NULL;

    char b[FILENAME_MAX + 1];
    char n[FILENAME_MAX + 1];
    rrdset_strncpyz_name(b, id, FILENAME_MAX);

    snprintfz(n, FILENAME_MAX, "%s/%s", host->cache_dir, b);
    ret = strdupz(n);

    if(host->storage_engine_id == STORAGE_ENGINE_MAP || host->storage_engine_id == STORAGE_ENGINE_SAVE) {
        int r = mkdir(ret, 0775);
        if(r != 0 && errno != EEXIST)
            netdata_log_error("Cannot create directory '%s'", ret);
    }

    return ret;
}

// ----------------------------------------------------------------------------
// RRD - string management

STRING *rrd_string_strdupz(const char *s) {
    if(unlikely(!s || !*s)) return string_strdupz(s);

    char *tmp = strdupz(s);
    json_fix_string(tmp);
    STRING *ret = string_strdupz(tmp);
    freez(tmp);
    return ret;
}

struct rrdb rrdb = {
    .rrdhost_root_index = NULL,
    .rrdhost_root_index_hostname = NULL,
    .unittest_running = false,
    .dbengine_enabled = false,
    .use_direct_io = true,
    .storage_tiers_grouping_iterations = {
        1,
        60,
        60,
        60,
        60
    },
    .storage_tiers_backfill = {
        RRD_BACKFILL_NEW,
        RRD_BACKFILL_NEW,
        RRD_BACKFILL_NEW,
        RRD_BACKFILL_NEW,
        RRD_BACKFILL_NEW
    },
    .default_rrd_update_every = UPDATE_EVERY_MIN,
    .default_rrd_history_entries = RRD_DEFAULT_HISTORY_ENTRIES,
    .gap_when_lost_iterations_above = 1,
    .rrdset_free_obsolete_time_s = RRD_DEFAULT_HISTORY_ENTRIES,
    .libuv_worker_threads = MIN_LIBUV_WORKER_THREADS,
    .ieee754_doubles = false,
    .rrdhost_free_orphan_time_s = RRD_DEFAULT_HISTORY_ENTRIES,
    .rrd_rwlock = NETDATA_RWLOCK_INITIALIZER,
    .localhost = NULL,

    #if defined(ENV32BIT)
    .default_rrdeng_page_cache_mb = 16,
    .default_rrdeng_extent_cache_mb = 0,
    #else
    .default_rrdeng_page_cache_mb = 32,
    .default_rrdeng_extent_cache_mb = 0,
    #endif

    .db_engine_journal_check = CONFIG_BOOLEAN_NO,

    .default_rrdeng_disk_quota_mb = 256,
    .default_multidb_disk_quota_mb = 256,

    .multidb_ctx = {
        NULL,
        NULL,
        NULL,
        NULL,
        NULL,
    },

    .page_type_size = {
        sizeof(storage_number),
        sizeof(storage_number_tier1_t)
    },
};

// SPDX-License-Identifier: GPL-3.0-or-later

#define PULSE_INTERNALS 1
#include "pulse-sqlite3.h"

static struct sqlite3_statistics {
    bool enabled;

    PAD64(uint64_t) sqlite3_queries_made;
    PAD64(uint64_t) sqlite3_queries_ok;
    PAD64(uint64_t) sqlite3_queries_failed;
    PAD64(uint64_t) sqlite3_queries_failed_busy;
    PAD64(uint64_t) sqlite3_queries_failed_locked;
    PAD64(uint64_t) sqlite3_rows;
    PAD64(uint64_t) sqlite3_metadata_cache_hit;
    PAD64(uint64_t) sqlite3_context_cache_hit;
    PAD64(uint64_t) sqlite3_metadata_cache_miss;
    PAD64(uint64_t) sqlite3_context_cache_miss;
    PAD64(uint64_t) sqlite3_metadata_cache_spill;
    PAD64(uint64_t) sqlite3_context_cache_spill;
    PAD64(uint64_t) sqlite3_metadata_cache_write;
    PAD64(uint64_t) sqlite3_context_cache_write;
} sqlite3_statistics = { 0 };

void pulse_sqlite3_query_completed(bool success, bool busy, bool locked) {
    if(!sqlite3_statistics.enabled) return;

    __atomic_fetch_add(&sqlite3_statistics.sqlite3_queries_made, 1, __ATOMIC_RELAXED);

    if(success) {
        __atomic_fetch_add(&sqlite3_statistics.sqlite3_queries_ok, 1, __ATOMIC_RELAXED);
    }
    else {
        __atomic_fetch_add(&sqlite3_statistics.sqlite3_queries_failed, 1, __ATOMIC_RELAXED);

        if(busy)
            __atomic_fetch_add(&sqlite3_statistics.sqlite3_queries_failed_busy, 1, __ATOMIC_RELAXED);

        if(locked)
            __atomic_fetch_add(&sqlite3_statistics.sqlite3_queries_failed_locked, 1, __ATOMIC_RELAXED);
    }
}

void pulse_sqlite3_row_completed(void) {
    if(!sqlite3_statistics.enabled) return;

    __atomic_fetch_add(&sqlite3_statistics.sqlite3_rows, 1, __ATOMIC_RELAXED);
}

static inline void sqlite3_statistics_copy(struct sqlite3_statistics *gs) {
    static usec_t last_run = 0;

    gs->sqlite3_queries_made          = __atomic_load_n(&sqlite3_statistics.sqlite3_queries_made, __ATOMIC_RELAXED);
    gs->sqlite3_queries_ok            = __atomic_load_n(&sqlite3_statistics.sqlite3_queries_ok, __ATOMIC_RELAXED);
    gs->sqlite3_queries_failed        = __atomic_load_n(&sqlite3_statistics.sqlite3_queries_failed, __ATOMIC_RELAXED);
    gs->sqlite3_queries_failed_busy   = __atomic_load_n(&sqlite3_statistics.sqlite3_queries_failed_busy, __ATOMIC_RELAXED);
    gs->sqlite3_queries_failed_locked = __atomic_load_n(&sqlite3_statistics.sqlite3_queries_failed_locked, __ATOMIC_RELAXED);
    gs->sqlite3_rows                  = __atomic_load_n(&sqlite3_statistics.sqlite3_rows, __ATOMIC_RELAXED);

    usec_t timeout = nd_profile.update_every * USEC_PER_SEC + nd_profile.update_every * USEC_PER_SEC / 3;
    usec_t now = now_monotonic_usec();
    if(!last_run)
        last_run = now;
    usec_t delta = now - last_run;
    bool query_sqlite3 = delta < timeout;

    if(query_sqlite3 && now_monotonic_usec() - last_run < timeout)
        gs->sqlite3_metadata_cache_hit = (uint64_t) sql_metadata_cache_stats(SQLITE_DBSTATUS_CACHE_HIT);
    else {
        gs->sqlite3_metadata_cache_hit = UINT64_MAX;
        query_sqlite3 = false;
    }

    if(query_sqlite3 && now_monotonic_usec() - last_run < timeout)
        gs->sqlite3_context_cache_hit = (uint64_t) sql_context_cache_stats(SQLITE_DBSTATUS_CACHE_HIT);
    else {
        gs->sqlite3_context_cache_hit = UINT64_MAX;
        query_sqlite3 = false;
    }

    if(query_sqlite3 && now_monotonic_usec() - last_run < timeout)
        gs->sqlite3_metadata_cache_miss = (uint64_t) sql_metadata_cache_stats(SQLITE_DBSTATUS_CACHE_MISS);
    else {
        gs->sqlite3_metadata_cache_miss = UINT64_MAX;
        query_sqlite3 = false;
    }

    if(query_sqlite3 && now_monotonic_usec() - last_run < timeout)
        gs->sqlite3_context_cache_miss = (uint64_t) sql_context_cache_stats(SQLITE_DBSTATUS_CACHE_MISS);
    else {
        gs->sqlite3_context_cache_miss = UINT64_MAX;
        query_sqlite3 = false;
    }

    if(query_sqlite3 && now_monotonic_usec() - last_run < timeout)
        gs->sqlite3_metadata_cache_spill = (uint64_t) sql_metadata_cache_stats(SQLITE_DBSTATUS_CACHE_SPILL);
    else {
        gs->sqlite3_metadata_cache_spill = UINT64_MAX;
        query_sqlite3 = false;
    }

    if(query_sqlite3 && now_monotonic_usec() - last_run < timeout)
        gs->sqlite3_context_cache_spill = (uint64_t) sql_context_cache_stats(SQLITE_DBSTATUS_CACHE_SPILL);
    else {
        gs->sqlite3_context_cache_spill = UINT64_MAX;
        query_sqlite3 = false;
    }

    if(query_sqlite3 && now_monotonic_usec() - last_run < timeout)
        gs->sqlite3_metadata_cache_write = (uint64_t) sql_metadata_cache_stats(SQLITE_DBSTATUS_CACHE_WRITE);
    else {
        gs->sqlite3_metadata_cache_write = UINT64_MAX;
        query_sqlite3 = false;
    }

    if(query_sqlite3 && now_monotonic_usec() - last_run < timeout)
        gs->sqlite3_context_cache_write = (uint64_t) sql_context_cache_stats(SQLITE_DBSTATUS_CACHE_WRITE);
    else {
        gs->sqlite3_context_cache_write = UINT64_MAX;
        query_sqlite3 = false;
    }

    last_run = now_monotonic_usec();
}

void pulse_sqlite3_do(bool extended) {
    if(!extended) return;
    sqlite3_statistics.enabled = true;

    struct sqlite3_statistics gs;
    sqlite3_statistics_copy(&gs);

    if(gs.sqlite3_queries_made) {
        static RRDSET *st_sqlite3_queries = NULL;
        static RRDDIM *rd_queries = NULL;

        if (unlikely(!st_sqlite3_queries)) {
            st_sqlite3_queries = rrdset_create_localhost(
                "netdata"
                , "sqlite3_queries"
                , NULL
                , "sqlite3"
                , NULL
                , "Netdata SQLite3 Queries"
                , "queries/s"
                , "netdata"
                , "pulse"
                , 131100
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );

            rd_queries = rrddim_add(st_sqlite3_queries, "queries", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_sqlite3_queries, rd_queries, (collected_number)gs.sqlite3_queries_made);

        rrdset_done(st_sqlite3_queries);
    }

    // ----------------------------------------------------------------

    if(gs.sqlite3_queries_ok || gs.sqlite3_queries_failed) {
        static RRDSET *st_sqlite3_queries_by_status = NULL;
        static RRDDIM *rd_ok = NULL, *rd_failed = NULL, *rd_busy = NULL, *rd_locked = NULL;

        if (unlikely(!st_sqlite3_queries_by_status)) {
            st_sqlite3_queries_by_status = rrdset_create_localhost(
                "netdata"
                , "sqlite3_queries_by_status"
                , NULL
                , "sqlite3"
                , NULL
                , "Netdata SQLite3 Queries by status"
                , "queries/s"
                , "netdata"
                , "pulse"
                , 131101
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );

            rd_ok     = rrddim_add(st_sqlite3_queries_by_status, "ok",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_failed = rrddim_add(st_sqlite3_queries_by_status, "failed", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_busy   = rrddim_add(st_sqlite3_queries_by_status, "busy",   NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_locked = rrddim_add(st_sqlite3_queries_by_status, "locked", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_sqlite3_queries_by_status, rd_ok,     (collected_number)gs.sqlite3_queries_made);
        rrddim_set_by_pointer(st_sqlite3_queries_by_status, rd_failed, (collected_number)gs.sqlite3_queries_failed);
        rrddim_set_by_pointer(st_sqlite3_queries_by_status, rd_busy,   (collected_number)gs.sqlite3_queries_failed_busy);
        rrddim_set_by_pointer(st_sqlite3_queries_by_status, rd_locked, (collected_number)gs.sqlite3_queries_failed_locked);

        rrdset_done(st_sqlite3_queries_by_status);
    }

    // ----------------------------------------------------------------

    if(gs.sqlite3_rows) {
        static RRDSET *st_sqlite3_rows = NULL;
        static RRDDIM *rd_rows = NULL;

        if (unlikely(!st_sqlite3_rows)) {
            st_sqlite3_rows = rrdset_create_localhost(
                "netdata"
                , "sqlite3_rows"
                , NULL
                , "sqlite3"
                , NULL
                , "Netdata SQLite3 Rows"
                , "rows/s"
                , "netdata"
                , "pulse"
                , 131102
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );

            rd_rows = rrddim_add(st_sqlite3_rows, "ok", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_sqlite3_rows, rd_rows, (collected_number)gs.sqlite3_rows);

        rrdset_done(st_sqlite3_rows);
    }

    if(gs.sqlite3_metadata_cache_hit) {
        static RRDSET *st_sqlite3_cache = NULL;
        static RRDDIM *rd_cache_hit = NULL;
        static RRDDIM *rd_cache_miss= NULL;
        static RRDDIM *rd_cache_spill= NULL;
        static RRDDIM *rd_cache_write= NULL;

        if (unlikely(!st_sqlite3_cache)) {
            st_sqlite3_cache = rrdset_create_localhost(
                "netdata"
                , "sqlite3_metatada_cache"
                , NULL
                , "sqlite3"
                , NULL
                , "Netdata SQLite3 metadata cache"
                , "ops/s"
                , "netdata"
                , "pulse"
                , 131103
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );

            rd_cache_hit = rrddim_add(st_sqlite3_cache, "cache_hit", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_cache_miss = rrddim_add(st_sqlite3_cache, "cache_miss", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_cache_spill = rrddim_add(st_sqlite3_cache, "cache_spill", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_cache_write = rrddim_add(st_sqlite3_cache, "cache_write", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        if(gs.sqlite3_metadata_cache_hit != UINT64_MAX)
            rrddim_set_by_pointer(st_sqlite3_cache, rd_cache_hit, (collected_number)gs.sqlite3_metadata_cache_hit);

        if(gs.sqlite3_metadata_cache_miss != UINT64_MAX)
            rrddim_set_by_pointer(st_sqlite3_cache, rd_cache_miss, (collected_number)gs.sqlite3_metadata_cache_miss);

        if(gs.sqlite3_metadata_cache_spill != UINT64_MAX)
            rrddim_set_by_pointer(st_sqlite3_cache, rd_cache_spill, (collected_number)gs.sqlite3_metadata_cache_spill);

        if(gs.sqlite3_metadata_cache_write != UINT64_MAX)
            rrddim_set_by_pointer(st_sqlite3_cache, rd_cache_write, (collected_number)gs.sqlite3_metadata_cache_write);

        rrdset_done(st_sqlite3_cache);
    }

    if(gs.sqlite3_context_cache_hit) {
        static RRDSET *st_sqlite3_cache = NULL;
        static RRDDIM *rd_cache_hit = NULL;
        static RRDDIM *rd_cache_miss= NULL;
        static RRDDIM *rd_cache_spill= NULL;
        static RRDDIM *rd_cache_write= NULL;

        if (unlikely(!st_sqlite3_cache)) {
            st_sqlite3_cache = rrdset_create_localhost(
                "netdata"
                , "sqlite3_context_cache"
                , NULL
                , "sqlite3"
                , NULL
                , "Netdata SQLite3 context cache"
                , "ops/s"
                , "netdata"
                , "pulse"
                , 131104
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );

            rd_cache_hit = rrddim_add(st_sqlite3_cache, "cache_hit", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_cache_miss = rrddim_add(st_sqlite3_cache, "cache_miss", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_cache_spill = rrddim_add(st_sqlite3_cache, "cache_spill", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_cache_write = rrddim_add(st_sqlite3_cache, "cache_write", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        if(gs.sqlite3_context_cache_hit != UINT64_MAX)
            rrddim_set_by_pointer(st_sqlite3_cache, rd_cache_hit, (collected_number)gs.sqlite3_context_cache_hit);

        if(gs.sqlite3_context_cache_miss != UINT64_MAX)
            rrddim_set_by_pointer(st_sqlite3_cache, rd_cache_miss, (collected_number)gs.sqlite3_context_cache_miss);

        if(gs.sqlite3_context_cache_spill != UINT64_MAX)
            rrddim_set_by_pointer(st_sqlite3_cache, rd_cache_spill, (collected_number)gs.sqlite3_context_cache_spill);

        if(gs.sqlite3_context_cache_write != UINT64_MAX)
            rrddim_set_by_pointer(st_sqlite3_cache, rd_cache_write, (collected_number)gs.sqlite3_context_cache_write);

        rrdset_done(st_sqlite3_cache);
    }
}

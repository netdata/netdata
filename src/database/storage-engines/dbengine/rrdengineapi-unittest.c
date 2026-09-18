// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdengine.h"
#include "database/storage-engines/dbengine/include/dbengine/dbengine-tests.h"

// Tests of the collect/query path that need to see the engine's own objects: the daemon-side test drivers reach
// the engine only through its public headers, so a check on the page a collect handle holds lives here.

typedef struct {
    time_t start_time_s;
    time_t end_time_s;
    NETDATA_DOUBLE value;
} EXPECTED_POINT;

static void store_point(STORAGE_COLLECT_HANDLE *sch, time_t end_time_s, NETDATA_DOUBLE value) {
    dbengine_store_next(sch, (usec_t)end_time_s * USEC_PER_SEC, value, value, value, 1, 0, SN_DEFAULT_FLAGS);
}

static int query_points(STORAGE_METRIC_HANDLE *smh, time_t start_time_s, time_t end_time_s,
                        const EXPECTED_POINT *expected, size_t expected_points, const char *id) {
    int errors = 0;
    struct storage_engine_query_handle seqh = { 0 };
    dbengine_query_init(smh, &seqh, start_time_s, end_time_s, STORAGE_PRIORITY_SYNCHRONOUS);

    size_t points = 0;
    bool finished = false;
    for(size_t safety = 0; safety < 8; safety++) {
        if(dbengine_query_is_finished(&seqh)) {
            finished = true;
            break;
        }

        STORAGE_POINT sp = dbengine_query_next(&seqh);
        if(points >= expected_points) {
            fprintf(stderr, " >>> DBENGINE: %s returned unexpected point %zu: %jd-%jd, value %f\n",
                    id, points, (intmax_t)sp.start_time_s, (intmax_t)sp.end_time_s, sp.sum);
            errors++;
            points++;
            continue;
        }

        const EXPECTED_POINT *ep = &expected[points];
        if(sp.start_time_s != ep->start_time_s || sp.end_time_s != ep->end_time_s ||
           sp.min != ep->value || sp.max != ep->value || sp.sum != ep->value ||
           storage_point_is_gap(sp) || sp.count != 1 || sp.anomaly_count != 0 ||
           sp.flags != SN_DEFAULT_FLAGS) {
            fprintf(stderr,
                    " >>> DBENGINE: %s point %zu is %jd-%jd min=%f max=%f sum=%f count=%u "
                    "anomalies=%u flags=0x%x; expected %jd-%jd value=%f\n",
                    id, points, (intmax_t)sp.start_time_s, (intmax_t)sp.end_time_s,
                    sp.min, sp.max, sp.sum, sp.count, sp.anomaly_count, (unsigned)sp.flags,
                    (intmax_t)ep->start_time_s, (intmax_t)ep->end_time_s, ep->value);
            errors++;
        }

        points++;
    }

    if(!finished && dbengine_query_is_finished(&seqh))
        finished = true;

    if(!finished) {
        fprintf(stderr, " >>> DBENGINE: %s query did not finish within 8 points\n", id);
        errors++;
    }

    if(points != expected_points) {
        fprintf(stderr, " >>> DBENGINE: %s returned %zu points, expected %zu\n", id, points, expected_points);
        errors++;
    }

    dbengine_query_finalize(&seqh);
    return errors;
}

// The open and extent caches size themselves from the main cache; once dbengine_destroy() has freed it, they
// settle on a fixed floor. Pinned through the helper both sizing callbacks share, asked about a main cache that
// does not exist: the very branch the leak-checking teardown relies on.
int dbengine_cache_floor_unittest(void) {
    int errors = 0;
    int64_t open_size = dbengine_follower_cache_size(NULL, OPEN_CACHE_PERCENT, OPEN_CACHE_MIN_SIZE);
    if(open_size != OPEN_CACHE_MIN_SIZE) {
        fprintf(stderr, " >>> DBENGINE: open cache size without a main cache is %" PRId64 ", expected %" PRId64 "\n",
                open_size, (int64_t)OPEN_CACHE_MIN_SIZE);
        errors++;
    }

    int64_t extent_size = dbengine_follower_cache_size(NULL, EXTENT_CACHE_PERCENT, EXTENT_CACHE_MIN_SIZE);
    if(extent_size != EXTENT_CACHE_MIN_SIZE) {
        fprintf(stderr, " >>> DBENGINE: extent cache size without a main cache is %" PRId64 ", expected %" PRId64 "\n",
                extent_size, (int64_t)EXTENT_CACHE_MIN_SIZE);
        errors++;
    }

    return errors;
}

// An embedder that never made an engine (the daemon in ram mode) still calls the engine's getters and verbs, with
// NULL: each must answer as an engine with nothing in it would. dbengine_shutdown(NULL) is left out on purpose,
// it records that no engine may be made afterwards. The buffers start as 0xff so that a getter that forgets to
// write its answer shows.
int dbengine_null_engine_unittest(void) {
    int errors = 0;

    for(DBENGINE_CACHE which = DBENGINE_CACHE_MAIN; which <= DBENGINE_CACHE_EXTENT; which++) {
        struct dbengine_cache_stats cache_stats, zero_cache_stats;
        memset(&cache_stats, 0xff, sizeof(cache_stats));
        memset(&zero_cache_stats, 0, sizeof(zero_cache_stats));
        if(dbengine_get_cache_stats(NULL, which, &cache_stats) || memcmp(&cache_stats, &zero_cache_stats, sizeof(cache_stats))) {
            fprintf(stderr, " >>> DBENGINE: cache %d stats of no engine are not false and zeroed\n", (int)which);
            errors++;
        }
    }

    if(dbengine_pages_pending_flush(NULL)) {
        fprintf(stderr, " >>> DBENGINE: no engine has pages to flush\n");
        errors++;
    }

    struct dbengine_metrics_registry_stats registry_stats, zero_registry_stats;
    memset(&registry_stats, 0xff, sizeof(registry_stats));
    memset(&zero_registry_stats, 0, sizeof(zero_registry_stats));
    if(dbengine_get_metrics_registry_stats(NULL, &registry_stats) || memcmp(&registry_stats, &zero_registry_stats, sizeof(registry_stats))) {
        fprintf(stderr, " >>> DBENGINE: the registry stats of no engine are not false and zeroed\n");
        errors++;
    }

    struct dbengine_cache_efficiency_stats efficiency = dbengine_get_cache_efficiency_stats(NULL), zero_efficiency;
    memset(&zero_efficiency, 0, sizeof(zero_efficiency));
    if(memcmp(&efficiency, &zero_efficiency, sizeof(efficiency))) {
        fprintf(stderr, " >>> DBENGINE: the cache efficiency stats of no engine are not zeroed\n");
        errors++;
    }

    struct dbengine_buffer_sizes sizes = dbengine_get_memory_sizes(NULL);
    const DBENGINE_MEM engine_slots[] = { DBENGINE_MEM_OPCODES, DBENGINE_MEM_HANDLES, DBENGINE_MEM_DESCRIPTORS,
                                          DBENGINE_MEM_WORKERS, DBENGINE_MEM_XT_IO };
    for(size_t i = 0; i < _countof(engine_slots); i++) {
        if(sizes.as[engine_slots[i]]) {
            fprintf(stderr, " >>> DBENGINE: the memory slot %d of no engine is set\n", (int)engine_slots[i]);
            errors++;
        }
    }
    if(sizes.wal) {
        fprintf(stderr, " >>> DBENGINE: the WAL bytes of no engine are not zero\n");
        errors++;
    }

    struct dbengine_work_request request = { .fn = NULL, .data = NULL };
    if(dbengine_work_available(NULL) || dbengine_enq_work(NULL, &request)) {
        fprintf(stderr, " >>> DBENGINE: no engine takes work\n");
        errors++;
    }

    dbengine_preload_release(NULL);
    if(dbengine_destroy(NULL)) {
        fprintf(stderr, " >>> DBENGINE: destroying no engine reports referenced metrics\n");
        errors++;
    }

    return errors;
}

// A collector that reports a zero cadence leaves the engine a page with no update-every; the first query of it
// must repair the cadence once (counted in pages_invalid_update_every_fixed) and serve the points, a repeated
// query must find nothing left to repair. The points sit far in the past so that they never meet live data.
int dbengine_zero_page_cadence_unittest(DBENGINE_ENGINE *engine, STORAGE_INSTANCE *si) {
    const time_t t = 200000000 + 4250000;
    int errors = 0;

    nd_uuid_t uuid;
    uuid_generate(uuid);
    UUIDMAP_ID id = uuidmap_create(uuid);
    STORAGE_METRIC_HANDLE *smh = dbengine_metric_get_or_create_by_id(si, id);
    STORAGE_METRICS_GROUP *smg = dbengine_metrics_group_get(si, &uuid);
    STORAGE_COLLECT_HANDLE *sch = smh ? dbengine_store_init(smh, 10, smg) : NULL;
    if(!smh || !sch) {
        fprintf(stderr, " >>> DBENGINE: zero-page-cadence metric initialization failed\n");
        errors++;
        goto cleanup;
    }

    dbengine_store_change_collection_frequency(sch, 0);
    store_point(sch, t + 100, 1);
    store_point(sch, t + 110, 2);

    struct dbengine_collect_handle *handle = (struct dbengine_collect_handle *)sch;
    if(!handle->pgc_page || pgc_page_data(handle->pgc_page) == PGD_EMPTY ||
       pgd_slots_used(pgc_page_data(handle->pgc_page)) != 2 ||
       pgc_page_start_time_s(handle->pgc_page) != t + 100 ||
       pgc_page_end_time_s(handle->pgc_page) != t + 110 ||
       pgc_page_update_every_s(handle->pgc_page) != 0) {
        fprintf(stderr, " >>> DBENGINE: zero-page-cadence fixture did not retain a nonempty 0-second page\n");
        errors++;
        goto cleanup;
    }

    const EXPECTED_POINT expected[] = {
        { t + 90,  t + 100, 1 },
        { t + 100, t + 110, 2 },
    };

    size_t invalid_before = dbengine_get_cache_efficiency_stats(engine).pages_invalid_update_every_fixed;
    errors += query_points(smh, t + 100, t + 110, expected, _countof(expected), "zero-page-cadence-first");
    size_t invalid_after_first = dbengine_get_cache_efficiency_stats(engine).pages_invalid_update_every_fixed;

    if(invalid_after_first != invalid_before + 1 || pgc_page_update_every_s(handle->pgc_page) != 10) {
        fprintf(stderr, " >>> DBENGINE: zero page cadence repairs=%zu cadence=%u, expected 1 and 10\n",
                invalid_after_first - invalid_before, pgc_page_update_every_s(handle->pgc_page));
        errors++;
    }

    errors += query_points(smh, t + 100, t + 110, expected, _countof(expected), "zero-page-cadence-repeat");
    size_t invalid_after_repeat = dbengine_get_cache_efficiency_stats(engine).pages_invalid_update_every_fixed;

    if(invalid_after_repeat != invalid_after_first) {
        fprintf(stderr, " >>> DBENGINE: repeated zero-page-cadence query reported %zu repairs, expected 0\n",
                invalid_after_repeat - invalid_after_first);
        errors++;
    }

cleanup:
    if(sch)
        dbengine_store_finalize(sch); // flushes the page it holds
    if(smg)
        dbengine_metrics_group_release(si, smg);
    if(smh)
        dbengine_metric_release(smh);
    uuidmap_free(id);
    return errors;
}

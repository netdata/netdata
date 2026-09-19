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
// NULL: each must answer as an engine with nothing in it would. dbengine_tier_init() is left out (it is fatal
// without an engine) and so is dbengine_shutdown() (it does nothing). The buffers start as 0xff so that a getter
// that forgets to write its answer shows.
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

    // the daemon runs every allocator statistics reader over every slot, the NULL ones included: a NULL slot (an
    // engine's own, or a page-details allocator before the first engine) reads as zero bytes and crashes nothing.
    // The process-wide slots are real statistics here and may well hold bytes.
    for(size_t i = 0; i < DBENGINE_MEM_MAX; i++) {
        if(sizes.as[i])
            continue;

        if(aral_structures_bytes_from_stats(sizes.as[i]) || aral_free_bytes_from_stats(sizes.as[i]) ||
           aral_used_bytes_from_stats(sizes.as[i]) || aral_padding_bytes_from_stats(sizes.as[i])) {
            fprintf(stderr, " >>> DBENGINE: the NULL memory slot %d reports bytes\n", (int)i);
            errors++;
        }
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

// one engine's whole life on a scratch directory: made, refused a second engine, given a tier that collects and is
// queried, stopped, refused a tier, destroyed with nothing referenced, and the static tiers released
static int engine_lifecycle_generation(const struct dbengine_config *cfg, const char *dir, const char *what) {
    int errors = 0;

    if(mkdir(dir, 0700) != 0 && errno != EEXIST) {
        fprintf(stderr, " >>> DBENGINE: %s: cannot create the scratch directory '%s'\n", what, dir);
        return 1;
    }

    DBENGINE_ENGINE *engine = dbengine_create(cfg);
    if(!engine) {
        fprintf(stderr, " >>> DBENGINE: %s: the engine did not come up\n", what);
        return 1;
    }

    if(dbengine_create(cfg)) {
        fprintf(stderr, " >>> DBENGINE: %s: a second engine was made while the first owns the static tiers\n", what);
        errors++;
    }

    struct dbengine_tier_config tc = {
        .tier = 0,
        .dbfiles_path = dir,
        .disk_space_mb = 0,
        .max_retention_s = 0,
        .page_type = DBENGINE_PAGE_TYPE_GORILLA_32BIT,
        .grouping = 1,
    };
    int rc = dbengine_tier_init(engine, &tc);
    if(rc) {
        fprintf(stderr, " >>> DBENGINE: %s: the tier did not come up: %s\n", what, uv_strerror(rc));
        errors++;
    }
    else {
        DBENGINE_TIER *tier = dbengine_multidb_tiers[0];
        dbengine_readiness_wait(tier);

        errors += dbengine_zero_page_cadence_unittest(engine, (STORAGE_INSTANCE *)tier);

        dbengine_tier_exit(tier);
    }

    dbengine_shutdown(engine);

    rc = dbengine_tier_init(engine, &tc);
    if(rc != UV_EIO) {
        fprintf(stderr, " >>> DBENGINE: %s: a tier came up on a stopped engine (returned %d)\n", what, rc);
        errors++;
    }

    // stopped but not destroyed: it still owns the static tiers
    if(dbengine_create(cfg)) {
        fprintf(stderr, " >>> DBENGINE: %s: an engine was made while a stopped one still owns the static tiers\n", what);
        errors++;
    }

    size_t referenced = dbengine_destroy(engine);
    if(referenced) {
        fprintf(stderr, " >>> DBENGINE: %s: %zu metrics stayed referenced across the destroy\n", what, referenced);
        errors++;
    }

    for(size_t i = 0; i < RRD_STORAGE_TIERS; i++) {
        if(dbengine_multidb_tiers[i]->engine) {
            fprintf(stderr, " >>> DBENGINE: %s: static tier %zu still points at an engine after the destroy\n", what, i);
            errors++;
        }
    }

    return errors;
}

static void engine_lifecycle_remove_dir(const char *dir) {
    DIR *d = opendir(dir);
    if(!d)
        return;

    struct dirent *de;
    while((de = readdir(d))) {
        if(de->d_name[0] == '.')
            continue;

        char path[FILENAME_MAX + 1];
        snprintfz(path, sizeof(path), "%s/%s", dir, de->d_name);
        unlink(path);
    }
    closedir(d);
    rmdir(dir);
}

// A stopped and destroyed engine leaves nothing behind that a new engine trips over: the second one, made with a
// differing configuration, comes up on the page allocators that already exist (the process-wide layer keeps its
// settings, and logs that the new ones are ignored), runs a tier, and is destroyed the same way. The floors of the
// caches and the NULL engine still hold between the two. Runs before the daemon's own engine, on two scratch
// directories inside scratch_dir, which it removes.
int dbengine_engine_lifecycle_unittest(const struct dbengine_config *cfg, const char *scratch_dir) {
    int errors = 0;
    fprintf(stderr, "\nTesting the life of two engines, one after the other...\n");

    // nothing of the daemon's: no preload from its metadata database, no rotation callback into its contexts
    struct dbengine_config first = *cfg;
    first.preload_metrics = NULL;
    first.on_db_rotation = NULL;

    char dir_a[FILENAME_MAX + 1], dir_b[FILENAME_MAX + 1];
    snprintfz(dir_a, sizeof(dir_a), "%s-a", scratch_dir);
    snprintfz(dir_b, sizeof(dir_b), "%s-b", scratch_dir);

    errors += engine_lifecycle_generation(&first, dir_a, "the first engine");

    uintptr_t layer = dbengine_allocator_layer_fingerprint();

    errors += dbengine_cache_floor_unittest();
    errors += dbengine_null_engine_unittest();

    // the second engine asks for other page allocator settings than the first one built them with
    struct dbengine_config second = first;
    second.allocator.partitions = (second.allocator.partitions ? second.allocator.partitions : second.cpus) + 1;
    second.allocator.arals_for_large_pages = !second.allocator.arals_for_large_pages;
    second.allocator.compression_statistics = !second.allocator.compression_statistics;

    errors += engine_lifecycle_generation(&second, dir_b, "the second engine");

    if(dbengine_allocator_layer_fingerprint() != layer) {
        fprintf(stderr, " >>> DBENGINE: the second engine changed the page allocators the first one built\n");
        errors++;
    }

    errors += dbengine_cache_floor_unittest();
    errors += dbengine_null_engine_unittest();

    engine_lifecycle_remove_dir(dir_a);
    engine_lifecycle_remove_dir(dir_b);

    fprintf(stderr, "Two engines, one after the other: %d ERROR(S)\n", errors);
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

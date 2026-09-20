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

    // no engine has a tier, and the tier verbs the daemon reaches through dbengine_tier() take that NULL as a
    // tier with nothing in it (the daemon's shutdown asks every tier slot whether it is up, engine or no engine)
    if(dbengine_tier(NULL, 0)) {
        fprintf(stderr, " >>> DBENGINE: no engine has a tier 0\n");
        errors++;
    }
    if(dbengine_tier_is_active(NULL)) {
        fprintf(stderr, " >>> DBENGINE: no tier is active\n");
        errors++;
    }
    if(dbengine_max_retention_s(NULL)) {
        fprintf(stderr, " >>> DBENGINE: no tier has a retention limit\n");
        errors++;
    }
    if(dbengine_collectors_running(NULL)) {
        fprintf(stderr, " >>> DBENGINE: no tier has collectors running\n");
        errors++;
    }
    if(dbengine_get_used_disk_space(NULL)) {
        fprintf(stderr, " >>> DBENGINE: no tier uses disk space\n");
        errors++;
    }
    if(dbengine_get_directory_free_bytes_space(NULL)) {
        fprintf(stderr, " >>> DBENGINE: no tier has free space in its directory\n");
        errors++;
    }
    struct dbengine_size_stats size_stats = dbengine_get_size_stats(NULL), zero_size_stats;
    memset(&zero_size_stats, 0, sizeof(zero_size_stats));
    if(memcmp(&size_stats, &zero_size_stats, sizeof(size_stats))) {
        fprintf(stderr, " >>> DBENGINE: the size stats of no tier are not zeroed\n");
        errors++;
    }
    dbengine_readiness_wait(NULL);
    if(dbengine_disk_space_max(NULL) || dbengine_disk_space_used(NULL) || dbengine_metrics(NULL) ||
       dbengine_samples(NULL) || dbengine_global_first_time_s(NULL)) {
        fprintf(stderr, " >>> DBENGINE: no tier reports a disk quota, disk space, metrics, samples or a first time\n");
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

// one engine's whole life on a scratch directory: made, with a second engine made and unmade next to it, given a
// tier that collects and is queried, stopped (blocking no other engine), refused a tier, destroyed with nothing
// referenced
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

    // the tiers are the engine's own: nothing in the process is claimed, and another engine can be made while
    // this one lives
    DBENGINE_ENGINE *other = dbengine_create(cfg);
    if(!other) {
        fprintf(stderr, " >>> DBENGINE: %s: a second engine could not be made while the first lives\n", what);
        errors++;
    }
    else {
        dbengine_shutdown(other);
        if(dbengine_destroy(other)) {
            fprintf(stderr, " >>> DBENGINE: %s: the second engine kept referenced metrics across its destroy\n", what);
            errors++;
        }
    }

    for(size_t i = 0; i < RRD_STORAGE_TIERS; i++) {
        DBENGINE_TIER *tier = dbengine_tier(engine, i);
        if(!tier || tier->engine != engine) {
            fprintf(stderr, " >>> DBENGINE: %s: tier %zu is not the engine's\n", what, i);
            errors++;
        }
    }

    if(dbengine_tier(engine, RRD_STORAGE_TIERS)) {
        fprintf(stderr, " >>> DBENGINE: %s: the engine has a tier %d\n", what, RRD_STORAGE_TIERS);
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
        DBENGINE_TIER *tier = dbengine_tier(engine, 0);
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

    // stopped but not destroyed: it blocks no other engine either
    other = dbengine_create(cfg);
    if(!other) {
        fprintf(stderr, " >>> DBENGINE: %s: an engine could not be made while a stopped one exists\n", what);
        errors++;
    }
    else {
        dbengine_shutdown(other);
        if(dbengine_destroy(other)) {
            fprintf(stderr, " >>> DBENGINE: %s: the engine made next to the stopped one kept referenced metrics\n", what);
            errors++;
        }
    }

    size_t referenced = dbengine_destroy(engine);
    if(referenced) {
        fprintf(stderr, " >>> DBENGINE: %s: %zu metrics stayed referenced across the destroy\n", what, referenced);
        errors++;
    }

    return errors;
}

static void engine_lifecycle_remove_dir(const char *dir);

// The configuration surface an embedder that is not the daemon relies on: dbengine_config_defaults() is the
// DBENGINE_CONFIG_DEFAULTS initialiser as a value; a zero max_reserved_file_descriptors resolves, at create, to a
// quarter of the soft limit libnetdata read; a budget below one tier's reservation refuses the tier with UV_EMFILE,
// nothing reserved and nothing written; a path that does not fit the tier is refused with UV_ENAMETOOLONG before
// the budget is even looked at; a budget of exactly one tier's reservation brings the tier up
static int engine_config_unittest(const struct dbengine_config *cfg, const char *dir, const char *what) {
    int errors = 0;

    struct dbengine_config from_macro = DBENGINE_CONFIG_DEFAULTS;
    struct dbengine_config from_function = dbengine_config_defaults();
#define CHECK_DEFAULT(field) do {                                                                               \
        if(from_macro.field != from_function.field) {                                                           \
            fprintf(stderr, " >>> DBENGINE: %s: dbengine_config_defaults() differs from the initialiser at "     \
                            #field "\n", what);                                                                  \
            errors++;                                                                                           \
        }                                                                                                       \
    } while(0)
    CHECK_DEFAULT(page_cache_mb);
    CHECK_DEFAULT(extent_cache_mb);
    CHECK_DEFAULT(out_of_memory_protection_bytes);
    CHECK_DEFAULT(use_all_ram_for_caches);
    CHECK_DEFAULT(cache_statistics);
    CHECK_DEFAULT(cpus);
    CHECK_DEFAULT(allocator.partitions);
    CHECK_DEFAULT(allocator.arals_for_large_pages);
    CHECK_DEFAULT(allocator.compression_statistics);
    CHECK_DEFAULT(direct_io);
    CHECK_DEFAULT(pages_per_extent);
    CHECK_DEFAULT(journal_integrity_check);
    CHECK_DEFAULT(journal_v2_unmount_time_s);
    CHECK_DEFAULT(max_reserved_file_descriptors);
    CHECK_DEFAULT(default_update_every_s);
    CHECK_DEFAULT(libuv_worker_threads);
    CHECK_DEFAULT(reserved_libuv_worker_threads);
    CHECK_DEFAULT(on_db_rotation);
    CHECK_DEFAULT(preload_metrics);
#undef CHECK_DEFAULT

    if(mkdir(dir, 0700) != 0 && errno != EEXIST) {
        fprintf(stderr, " >>> DBENGINE: %s: cannot create the scratch directory '%s'\n", what, dir);
        return errors + 1;
    }

    // the zero budget resolves at create, to the quarter of the soft limit, and the engine keeps the resolved copy
    struct dbengine_config zero_budget = *cfg;
    zero_budget.max_reserved_file_descriptors = 0;
    DBENGINE_ENGINE *engine = dbengine_create(&zero_budget);
    if(!engine) {
        fprintf(stderr, " >>> DBENGINE: %s: the engine with the zero budget did not come up\n", what);
        return errors + 1;
    }
    if(engine->cfg.max_reserved_file_descriptors != (size_t)(rlimit_nofile.rlim_cur / 4)) {
        fprintf(stderr, " >>> DBENGINE: %s: the zero budget resolved to %zu, not a quarter of the soft limit %zu\n",
                what, engine->cfg.max_reserved_file_descriptors, (size_t)rlimit_nofile.rlim_cur);
        errors++;
    }
    dbengine_shutdown(engine);
    if(dbengine_destroy(engine)) {
        fprintf(stderr, " >>> DBENGINE: %s: the engine with the zero budget kept referenced metrics\n", what);
        errors++;
    }

    // a budget one short of a tier's reservation refuses the tier: nothing stays reserved, nothing is written
    struct dbengine_config small = *cfg;
    small.max_reserved_file_descriptors = DBENGINE_FD_BUDGET_PER_TIER - 1;
    engine = dbengine_create(&small);
    if(!engine) {
        fprintf(stderr, " >>> DBENGINE: %s: the engine with the small budget did not come up\n", what);
        return errors + 1;
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
    if(rc != UV_EMFILE) {
        fprintf(stderr, " >>> DBENGINE: %s: a tier came up past the budget (returned %d)\n", what, rc);
        errors++;
    }
    if(__atomic_load_n(&engine->global_stats.dbengine_reserved_file_descriptors, __ATOMIC_RELAXED)) {
        fprintf(stderr, " >>> DBENGINE: %s: the refused tier left file descriptors reserved\n", what);
        errors++;
    }
    if(dbengine_tier_is_active(dbengine_tier(engine, 0)) || dbengine_dir_has_datafiles(dir)) {
        fprintf(stderr, " >>> DBENGINE: %s: the refused tier is up or wrote datafiles\n", what);
        errors++;
    }

    // a path longer than the tier's buffer is refused before the budget: the same engine, still nothing reserved
    char long_path[FILENAME_MAX + 64];
    memset(long_path, 'x', sizeof(long_path) - 1);
    long_path[0] = '/';
    long_path[sizeof(long_path) - 1] = '\0';
    tc.dbfiles_path = long_path;
    rc = dbengine_tier_init(engine, &tc);
    if(rc != UV_ENAMETOOLONG) {
        fprintf(stderr, " >>> DBENGINE: %s: an over-long path was not refused (returned %d)\n", what, rc);
        errors++;
    }
    if(__atomic_load_n(&engine->global_stats.dbengine_reserved_file_descriptors, __ATOMIC_RELAXED) ||
       dbengine_tier_is_active(dbengine_tier(engine, 0))) {
        fprintf(stderr, " >>> DBENGINE: %s: the over-long path left file descriptors reserved or the tier up\n", what);
        errors++;
    }
    dbengine_shutdown(engine);
    if(dbengine_destroy(engine)) {
        fprintf(stderr, " >>> DBENGINE: %s: the engine with the small budget kept referenced metrics\n", what);
        errors++;
    }

    // a budget of exactly one tier's reservation brings the tier up: the budget is the resolved value, not the limit
    struct dbengine_config exact = *cfg;
    exact.max_reserved_file_descriptors = DBENGINE_FD_BUDGET_PER_TIER;
    engine = dbengine_create(&exact);
    if(!engine) {
        fprintf(stderr, " >>> DBENGINE: %s: the engine with the exact budget did not come up\n", what);
        return errors + 1;
    }
    tc.dbfiles_path = dir;
    rc = dbengine_tier_init(engine, &tc);
    if(rc) {
        fprintf(stderr, " >>> DBENGINE: %s: the tier did not come up on the exact budget: %s\n", what, uv_strerror(rc));
        errors++;
    }
    else {
        if(__atomic_load_n(&engine->global_stats.dbengine_reserved_file_descriptors, __ATOMIC_RELAXED) != DBENGINE_FD_BUDGET_PER_TIER) {
            fprintf(stderr, " >>> DBENGINE: %s: the tier that came up did not reserve its file descriptors\n", what);
            errors++;
        }
        DBENGINE_TIER *tier = dbengine_tier(engine, 0);
        dbengine_readiness_wait(tier);
        dbengine_tier_exit(tier);
    }
    dbengine_shutdown(engine);
    if(dbengine_destroy(engine)) {
        fprintf(stderr, " >>> DBENGINE: %s: the engine with the exact budget kept referenced metrics\n", what);
        errors++;
    }

    return errors;
}

// Two engines alive at once, each with its tier 0 on a scratch directory of its own: the tiers are distinct
// objects, each engine collects and is queried on its own tier, a metric one engine created is unknown to the
// other's registry, the tier refusals are per engine (a second init of one engine's tier while the other's comes
// up; a stopped engine's tier while the other still serves), and one engine is stopped and destroyed while the
// other collects on, in the order the caller picks. The page allocator layer is untouched by the destroy.
static int engines_coexist(const struct dbengine_config *cfg, const char *dir_a, const char *dir_b, bool destroy_a_first) {
    int errors = 0;
    const char *what = destroy_a_first ? "two engines, A destroyed first" : "two engines, B destroyed first";

    if((mkdir(dir_a, 0700) != 0 && errno != EEXIST) || (mkdir(dir_b, 0700) != 0 && errno != EEXIST)) {
        fprintf(stderr, " >>> DBENGINE: %s: cannot create the scratch directories\n", what);
        return 1;
    }

    DBENGINE_ENGINE *a = dbengine_create(cfg);
    if(!a) {
        fprintf(stderr, " >>> DBENGINE: %s: engine A did not come up\n", what);
        return 1;
    }

    DBENGINE_ENGINE *b = dbengine_create(cfg);
    if(!b) {
        fprintf(stderr, " >>> DBENGINE: %s: engine B did not come up while A runs\n", what);
        dbengine_shutdown(a);
        dbengine_destroy(a);
        return 1;
    }

    struct dbengine_tier_config tc_a = {
        .tier = 0,
        .dbfiles_path = dir_a,
        .disk_space_mb = 0,
        .max_retention_s = 0,
        .page_type = DBENGINE_PAGE_TYPE_GORILLA_32BIT,
        .grouping = 1,
    };
    struct dbengine_tier_config tc_b = tc_a;
    tc_b.dbfiles_path = dir_b;

    int rc = dbengine_tier_init(a, &tc_a);
    if(rc) {
        fprintf(stderr, " >>> DBENGINE: %s: A's tier 0 did not come up: %s\n", what, uv_strerror(rc));
        errors++;
    }

    // a second init of A's tier is refused on A alone: B's tier of the same number comes up
    rc = dbengine_tier_init(a, &tc_a);
    if(rc != UV_EALREADY) {
        fprintf(stderr, " >>> DBENGINE: %s: A's tier 0 was initialized twice (returned %d)\n", what, rc);
        errors++;
    }
    rc = dbengine_tier_init(b, &tc_b);
    if(rc) {
        fprintf(stderr, " >>> DBENGINE: %s: B's tier 0 did not come up while A's is up: %s\n", what, uv_strerror(rc));
        errors++;
    }
    rc = dbengine_tier_init(b, &tc_b);
    if(rc != UV_EALREADY) {
        fprintf(stderr, " >>> DBENGINE: %s: B's tier 0 was initialized twice (returned %d)\n", what, rc);
        errors++;
    }

    DBENGINE_TIER *tier_a = dbengine_tier(a, 0), *tier_b = dbengine_tier(b, 0);
    if(!tier_a || !tier_b || tier_a == tier_b || tier_a->engine != a || tier_b->engine != b) {
        fprintf(stderr, " >>> DBENGINE: %s: the two engines do not have a tier 0 each\n", what);
        errors++;
    }

    if(dbengine_tier_is_active(tier_a))
        dbengine_readiness_wait(tier_a);
    if(dbengine_tier_is_active(tier_b))
        dbengine_readiness_wait(tier_b);

    if(dbengine_tier_is_active(tier_a) && dbengine_tier_is_active(tier_b)) {
        // each collects and is queried on its own tier
        errors += dbengine_zero_page_cadence_unittest(a, (STORAGE_INSTANCE *)tier_a);
        errors += dbengine_zero_page_cadence_unittest(b, (STORAGE_INSTANCE *)tier_b);

        // the registries are separate: a metric A created is unknown to B, and only A's registry grew by it (a
        // lookup on B's tier would miss in a shared registry too, since the sections differ; the counts would not).
        // The counts are exact because nothing else moves them here: the registry loads are awaited, no preload,
        // and the tiers have no quota and no retention limit, so nothing rotates or deletes
        struct dbengine_metrics_registry_stats a_before, b_before, a_after, b_after;
        dbengine_get_metrics_registry_stats(a, &a_before);
        dbengine_get_metrics_registry_stats(b, &b_before);
        nd_uuid_t uuid;
        uuid_generate(uuid);
        UUIDMAP_ID id = uuidmap_create(uuid);
        STORAGE_METRIC_HANDLE *on_a = dbengine_metric_get_or_create_by_id((STORAGE_INSTANCE *)tier_a, id);
        STORAGE_METRIC_HANDLE *on_b = dbengine_metric_get_by_uuid((STORAGE_INSTANCE *)tier_b, &uuid);
        dbengine_get_metrics_registry_stats(a, &a_after);
        dbengine_get_metrics_registry_stats(b, &b_after);
        if(!on_a) {
            fprintf(stderr, " >>> DBENGINE: %s: A did not create a metric on its tier\n", what);
            errors++;
        }
        if(a_after.entries != a_before.entries + 1 || b_after.entries != b_before.entries) {
            fprintf(stderr, " >>> DBENGINE: %s: the registries are not separate (A %zu -> %zu, B %zu -> %zu)\n",
                    what, a_before.entries, a_after.entries, b_before.entries, b_after.entries);
            errors++;
        }
        if(on_b) {
            fprintf(stderr, " >>> DBENGINE: %s: B's registry knows a metric A created\n", what);
            errors++;
            dbengine_metric_release(on_b);
        }
        if(on_a)
            dbengine_metric_release(on_a);
        uuidmap_free(id);
    }

    // the first to go: its tier exits and it stops; its tier is refused on it while the other still serves
    DBENGINE_ENGINE *first = destroy_a_first ? a : b, *second = destroy_a_first ? b : a;
    DBENGINE_TIER *tier_first = destroy_a_first ? tier_a : tier_b, *tier_second = destroy_a_first ? tier_b : tier_a;
    const struct dbengine_tier_config *tc_first = destroy_a_first ? &tc_a : &tc_b;

    if(dbengine_tier_is_active(tier_first))
        dbengine_tier_exit(tier_first);
    dbengine_shutdown(first);

    rc = dbengine_tier_init(first, tc_first);
    if(rc != UV_EIO) {
        fprintf(stderr, " >>> DBENGINE: %s: a tier came up on the stopped engine (returned %d)\n", what, rc);
        errors++;
    }

    // a tier that never came up on the stopped engine is refused as well: the refusal above is the tier's own
    // (it came up and exited); this one can only be the engine's, and a refusal that did not happen would have
    // created the tier's first datafile
    char probe_dir[FILENAME_MAX + 1];
    snprintfz(probe_dir, sizeof(probe_dir), "%s-probe", tc_first->dbfiles_path);
    if(mkdir(probe_dir, 0700) != 0 && errno != EEXIST) {
        fprintf(stderr, " >>> DBENGINE: %s: cannot create the probe directory\n", what);
        errors++;
    }
    else {
        struct dbengine_tier_config tc_probe = {
            .tier = 1,
            .dbfiles_path = probe_dir,
            .disk_space_mb = 0,
            .max_retention_s = 0,
            .page_type = DBENGINE_PAGE_TYPE_ARRAY_TIER1,
            .grouping = 60,
        };
        rc = dbengine_tier_init(first, &tc_probe);
        if(rc != UV_EIO) {
            fprintf(stderr, " >>> DBENGINE: %s: a never-used tier came up on the stopped engine (returned %d)\n", what, rc);
            errors++;
        }
        if(dbengine_dir_has_datafiles(probe_dir)) {
            fprintf(stderr, " >>> DBENGINE: %s: the stopped engine opened a tier in the probe directory\n", what);
            errors++;
        }
        engine_lifecycle_remove_dir(probe_dir);
    }
    if(!dbengine_tier_is_active(tier_second)) {
        fprintf(stderr, " >>> DBENGINE: %s: the other engine's tier went down with the stopped one\n", what);
        errors++;
    }

    uintptr_t layer = dbengine_allocator_layer_fingerprint();
    size_t referenced = dbengine_destroy(first);
    if(referenced) {
        fprintf(stderr, " >>> DBENGINE: %s: %zu metrics stayed referenced across the first destroy\n", what, referenced);
        errors++;
    }
    if(dbengine_allocator_layer_fingerprint() != layer) {
        fprintf(stderr, " >>> DBENGINE: %s: destroying one engine changed the page allocators the other uses\n", what);
        errors++;
    }

    // the survivor collects and is queried after the other engine is gone
    if(!dbengine_tier_is_active(tier_second)) {
        fprintf(stderr, " >>> DBENGINE: %s: the surviving engine's tier is not active after the destroy\n", what);
        errors++;
    }
    else
        errors += dbengine_zero_page_cadence_unittest(second, (STORAGE_INSTANCE *)tier_second);

    if(dbengine_tier_is_active(tier_second))
        dbengine_tier_exit(tier_second);
    dbengine_shutdown(second);
    referenced = dbengine_destroy(second);
    if(referenced) {
        fprintf(stderr, " >>> DBENGINE: %s: %zu metrics stayed referenced across the second destroy\n", what, referenced);
        errors++;
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
// caches and the NULL engine still hold between the two. Then two engines at once, each on a tier of its own,
// destroyed in either order (engines_coexist()). Runs before the daemon's own engine, on four scratch directories
// next to scratch_dir (its name with -a to -e), which it removes; the probe directories engines_coexist() makes
// next to two of them are removed by it.
int dbengine_engine_lifecycle_unittest(const struct dbengine_config *cfg, const char *scratch_dir) {
    int errors = 0;
    fprintf(stderr, "\nTesting the life of two engines, one after the other and at once...\n");

    // nothing of the daemon's: no preload from its metadata database, no rotation callback into its contexts
    struct dbengine_config first = *cfg;
    first.preload_metrics = NULL;
    first.on_db_rotation = NULL;

    char dir_a[FILENAME_MAX + 1], dir_b[FILENAME_MAX + 1], dir_c[FILENAME_MAX + 1], dir_d[FILENAME_MAX + 1],
         dir_e[FILENAME_MAX + 1];
    snprintfz(dir_a, sizeof(dir_a), "%s-a", scratch_dir);
    snprintfz(dir_b, sizeof(dir_b), "%s-b", scratch_dir);
    snprintfz(dir_c, sizeof(dir_c), "%s-c", scratch_dir);
    snprintfz(dir_d, sizeof(dir_d), "%s-d", scratch_dir);
    snprintfz(dir_e, sizeof(dir_e), "%s-e", scratch_dir);

    errors += engine_lifecycle_generation(&first, dir_a, "the first engine");
    errors += engine_config_unittest(&first, dir_e, "the configuration");

    uintptr_t layer = dbengine_allocator_layer_fingerprint();

    errors += dbengine_cache_floor_unittest();
    errors += dbengine_null_engine_unittest();

    // the second engine asks for other page allocator settings than the first one built them with
    struct dbengine_config second = first;
    second.allocator.partitions = (second.allocator.partitions ? second.allocator.partitions : second.cpus) + 1;
    second.allocator.arals_for_large_pages = !second.allocator.arals_for_large_pages;
    second.allocator.compression_statistics = !second.allocator.compression_statistics;

    errors += engine_lifecycle_generation(&second, dir_b, "the second engine");

    // two at once, in both destroy orders; the second run re-opens the directories the first left its datafiles in
    errors += engines_coexist(&first, dir_c, dir_d, true);
    errors += engines_coexist(&first, dir_c, dir_d, false);

    if(dbengine_allocator_layer_fingerprint() != layer) {
        fprintf(stderr, " >>> DBENGINE: the second engine changed the page allocators the first one built\n");
        errors++;
    }

    errors += dbengine_cache_floor_unittest();
    errors += dbengine_null_engine_unittest();

    engine_lifecycle_remove_dir(dir_a);
    engine_lifecycle_remove_dir(dir_b);
    engine_lifecycle_remove_dir(dir_c);
    engine_lifecycle_remove_dir(dir_d);
    engine_lifecycle_remove_dir(dir_e);

    fprintf(stderr, "Two engines, one after the other and at once: %d ERROR(S)\n", errors);
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

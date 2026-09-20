// SPDX-License-Identifier: GPL-3.0-or-later

#include "support.h"

#include <string>

// The engine's lifecycle and configuration surface, as dbengine-api.h and dbengine-config.h state them: an engine
// is made, holds tiers of its own, is stopped and destroyed, and each of those verbs refuses the calls the contract
// says it refuses. Every case here brings its own engine up on its own scratch directory and takes it back down, so
// none depends on another having run.

namespace {

// Every test that brings a tier up needs a directory nobody else writes: two tiers on one directory corrupt it, and
// nothing in the engine refuses the second one.

struct dbengine_tier_config tier_config(const Scratch &scratch) {
    struct dbengine_tier_config tc = {};

    tc.tier = 0;
    tc.dbfiles_path = scratch.c_str();
    tc.disk_space_mb = 0;
    tc.max_retention_s = 0;
    tc.page_type = DBENGINE_PAGE_TYPE_GORILLA_32BIT;
    tc.grouping = 1;

    return tc;
}

// An engine that is always taken down the way the contract requires - every tier exited, then shutdown, then
// destroy - so a case that fails an assertion still leaves the process able to run the next one.
class Engine {
public:
    explicit Engine(const struct dbengine_config &cfg) : engine_(dbengine_create(&cfg)) {}

    ~Engine() { reset(); }

    Engine(const Engine &) = delete;
    Engine &operator=(const Engine &) = delete;

    DBENGINE_ENGINE *get() const { return engine_; }
    explicit operator bool() const { return engine_ != nullptr; }

    // Returns what dbengine_destroy() reported: the registry metrics still referenced, which must be none.
    size_t reset() {
        if (!engine_)
            return 0;

        DBENGINE_ENGINE *engine = engine_;
        engine_ = nullptr;

        for (size_t i = 0; i < RRD_STORAGE_TIERS; i++) {
            DBENGINE_TIER *tier = dbengine_tier(engine, i);
            if (dbengine_tier_is_active(tier))
                dbengine_tier_exit(tier);
        }

        dbengine_shutdown(engine);
        return dbengine_destroy(engine);
    }

private:
    DBENGINE_ENGINE *engine_;
};

} // namespace

TEST(EngineLifecycle, ComesUpAndIsDestroyedWithNothingReferenced) {
    const struct dbengine_config cfg = netdata_test_config();

    Engine engine(cfg);
    ASSERT_TRUE(engine) << "the engine did not come up";

    EXPECT_EQ(engine.reset(), 0u) << "metrics stayed referenced across the destroy";
}

TEST(EngineLifecycle, HoldsItsOwnTiersAndNoMore) {
    const struct dbengine_config cfg = netdata_test_config();

    Engine engine(cfg);
    ASSERT_TRUE(engine);

    for (size_t i = 0; i < RRD_STORAGE_TIERS; i++) {
        SCOPED_TRACE(i);
        EXPECT_NE(dbengine_tier(engine.get(), i), nullptr) << "tier " << i << " is missing";
    }

    EXPECT_EQ(dbengine_tier(engine.get(), RRD_STORAGE_TIERS), nullptr)
        << "the engine answered for a tier number it does not have";
}

TEST(EngineLifecycle, TwoEnginesHaveTiersOfTheirOwn) {
    const struct dbengine_config cfg = netdata_test_config();

    Engine first(cfg);
    ASSERT_TRUE(first);

    // An engine is no obstacle to another: each has its tiers of its own, and nothing in the process is claimed.
    //
    // Neither engine is given a tier here, and that is not incidental. Both are told the pool has
    // DBENGINE_TEST_UV_THREADS threads while the process has one pool of that size between them, and support.h
    // explains what an over-stated figure costs: the process hangs and does not come back. With no tier there is
    // nothing to dispatch. Do not add one to this case.
    Engine second(cfg);
    ASSERT_TRUE(second) << "a second engine could not be made while the first lives";

    for (size_t i = 0; i < RRD_STORAGE_TIERS; i++) {
        SCOPED_TRACE(i);
        EXPECT_NE(dbengine_tier(first.get(), i), dbengine_tier(second.get(), i))
            << "the two engines share tier " << i;
    }

    EXPECT_EQ(second.reset(), 0u);
    EXPECT_EQ(first.reset(), 0u);
}

TEST(EngineLifecycle, ATierComesUpAndGoesDown) {
    const struct dbengine_config cfg = netdata_test_config();
    const Scratch scratch;
    ASSERT_TRUE(scratch.valid()) << "could not make a scratch directory";

    Engine engine(cfg);
    ASSERT_TRUE(engine);

    const struct dbengine_tier_config tc = tier_config(scratch);
    ASSERT_EQ(dbengine_tier_init(engine.get(), &tc), 0);

    DBENGINE_TIER *tier = dbengine_tier(engine.get(), 0);
    ASSERT_NE(tier, nullptr);
    dbengine_readiness_wait(tier);

    EXPECT_TRUE(dbengine_tier_is_active(tier));

    EXPECT_EQ(dbengine_tier_exit(tier), 0);
    EXPECT_FALSE(dbengine_tier_is_active(tier)) << "a tier that exited still reports itself active";

    EXPECT_EQ(engine.reset(), 0u);
}

TEST(TierInit, RefusesATierThatIsAlreadyUp) {
    const struct dbengine_config cfg = netdata_test_config();
    const Scratch scratch;
    ASSERT_TRUE(scratch.valid()) << "could not make a scratch directory";

    Engine engine(cfg);
    ASSERT_TRUE(engine);

    const struct dbengine_tier_config tc = tier_config(scratch);
    ASSERT_EQ(dbengine_tier_init(engine.get(), &tc), 0);
    dbengine_readiness_wait(dbengine_tier(engine.get(), 0));

    EXPECT_EQ(dbengine_tier_init(engine.get(), &tc), UV_EALREADY);

    EXPECT_EQ(engine.reset(), 0u);
}

TEST(TierInit, RefusesATierThatCameUpAndExited) {
    const struct dbengine_config cfg = netdata_test_config();
    const Scratch scratch;
    ASSERT_TRUE(scratch.valid()) << "could not make a scratch directory";

    Engine engine(cfg);
    ASSERT_TRUE(engine);

    const struct dbengine_tier_config tc = tier_config(scratch);
    ASSERT_EQ(dbengine_tier_init(engine.get(), &tc), 0);
    dbengine_readiness_wait(dbengine_tier(engine.get(), 0));
    ASSERT_EQ(dbengine_tier_exit(dbengine_tier(engine.get(), 0)), 0);

    // Its datafiles stay attached until the engine is destroyed, so the tier cannot be brought back up on it.
    EXPECT_EQ(dbengine_tier_init(engine.get(), &tc), UV_EIO);

    EXPECT_EQ(engine.reset(), 0u);
}

TEST(TierInit, RefusesATierOnAStoppedEngine) {
    const struct dbengine_config cfg = netdata_test_config();
    const Scratch scratch;
    ASSERT_TRUE(scratch.valid()) << "could not make a scratch directory";

    DBENGINE_ENGINE *engine = dbengine_create(&cfg);
    ASSERT_NE(engine, nullptr);

    dbengine_shutdown(engine);

    const struct dbengine_tier_config tc = tier_config(scratch);
    EXPECT_EQ(dbengine_tier_init(engine, &tc), UV_EIO);

    EXPECT_EQ(dbengine_destroy(engine), 0u);
}

TEST(TierInit, RefusesAPathWithNoRoomForTheFileNames) {
    const struct dbengine_config cfg = netdata_test_config();

    Engine engine(cfg);
    ASSERT_TRUE(engine);

    struct dbengine_tier_config tc = {};
    tc.tier = 0;
    tc.page_type = DBENGINE_PAGE_TYPE_GORILLA_32BIT;
    tc.grouping = 1;

    // Two lengths either side of the limit. They meet the same single check rather than two different ones, so
    // this is one contract sampled twice, not two code paths.
    const size_t lengths[] = {FILENAME_MAX + 32, DBENGINE_DBFILES_PATH_MAX + 1};

    for (size_t length : lengths) {
        SCOPED_TRACE(length);

        std::string path(length, 'x');
        path[0] = '/';
        tc.dbfiles_path = path.c_str();

        EXPECT_EQ(dbengine_tier_init(engine.get(), &tc), UV_ENAMETOOLONG);
        EXPECT_FALSE(dbengine_tier_is_active(dbengine_tier(engine.get(), 0)))
            << "the refused tier is up";
    }

    EXPECT_EQ(engine.reset(), 0u);
}

TEST(TierInit, RefusesATierThatWouldExceedTheFileDescriptorBudget) {
    struct dbengine_config cfg = netdata_test_config();
    const Scratch scratch;

    // One descriptor is below any tier's reservation, so the first tier is already too many. The size of that
    // reservation is not on the public surface, so the case is written not to need it.
    cfg.max_reserved_file_descriptors = 1;

    Engine engine(cfg);
    ASSERT_TRUE(engine);
    EXPECT_EQ(dbengine_max_reserved_file_descriptors(engine.get()), 1u)
        << "the budget the engine resolved is not the one it was given";

    const struct dbengine_tier_config tc = tier_config(scratch);
    EXPECT_EQ(dbengine_tier_init(engine.get(), &tc), UV_EMFILE);

    // The refusal leaves the tier untouched: not up, and nothing written to the directory.
    EXPECT_FALSE(dbengine_tier_is_active(dbengine_tier(engine.get(), 0)));
    EXPECT_FALSE(dbengine_dir_has_datafiles(scratch.c_str()))
        << "the refused tier wrote datafiles";

    EXPECT_EQ(engine.reset(), 0u);
}

TEST(EngineConfig, DefaultsFunctionMatchesTheInitialiser) {
    const struct dbengine_config from_macro = DBENGINE_CONFIG_DEFAULTS;
    const struct dbengine_config from_function = dbengine_config_defaults();

    // Field by field rather than a memcmp: padding between members is not part of the contract, and a mismatch
    // should name the field that drifted.
    EXPECT_EQ(from_macro.page_cache_mb, from_function.page_cache_mb);
    EXPECT_EQ(from_macro.extent_cache_mb, from_function.extent_cache_mb);
    EXPECT_EQ(from_macro.out_of_memory_protection_bytes, from_function.out_of_memory_protection_bytes);
    EXPECT_EQ(from_macro.use_all_ram_for_caches, from_function.use_all_ram_for_caches);
    EXPECT_EQ(from_macro.cache_statistics, from_function.cache_statistics);
    EXPECT_EQ(from_macro.cpus, from_function.cpus);
    EXPECT_EQ(from_macro.allocator.partitions, from_function.allocator.partitions);
    EXPECT_EQ(from_macro.allocator.arals_for_large_pages, from_function.allocator.arals_for_large_pages);
    EXPECT_EQ(from_macro.allocator.compression_statistics, from_function.allocator.compression_statistics);
    EXPECT_EQ(from_macro.direct_io, from_function.direct_io);
    EXPECT_EQ(from_macro.pages_per_extent, from_function.pages_per_extent);
    EXPECT_EQ(from_macro.journal_integrity_check, from_function.journal_integrity_check);
    EXPECT_EQ(from_macro.journal_v2_unmount_time_s, from_function.journal_v2_unmount_time_s);
    EXPECT_EQ(from_macro.max_reserved_file_descriptors, from_function.max_reserved_file_descriptors);
    EXPECT_EQ(from_macro.default_update_every_s, from_function.default_update_every_s);
    EXPECT_EQ(from_macro.libuv_worker_threads, from_function.libuv_worker_threads);
    EXPECT_EQ(from_macro.reserved_libuv_worker_threads, from_function.reserved_libuv_worker_threads);
    EXPECT_EQ(from_macro.on_db_rotation, from_function.on_db_rotation);
    EXPECT_EQ(from_macro.preload_metrics, from_function.preload_metrics);
}

TEST(EngineConfig, AZeroFileDescriptorBudgetResolvesToAShareOfTheLimit) {
    struct dbengine_config cfg = netdata_test_config();
    cfg.max_reserved_file_descriptors = 0;

    Engine engine(cfg);
    ASSERT_TRUE(engine);

    // 0 means "work it out from the process limit", and the engine takes a quarter of the soft limit, once, at
    // create. The exact share is the engine's business; that it resolved to something is the contract.
    EXPECT_GT(dbengine_max_reserved_file_descriptors(engine.get()), 0u)
        << "a zero budget did not resolve to a concrete value";

    EXPECT_EQ(engine.reset(), 0u);
}

TEST(EngineConfig, AGivenFileDescriptorBudgetIsKept) {
    struct dbengine_config cfg = netdata_test_config();
    cfg.max_reserved_file_descriptors = 4096;

    Engine engine(cfg);
    ASSERT_TRUE(engine);

    EXPECT_EQ(dbengine_max_reserved_file_descriptors(engine.get()), 4096u);

    EXPECT_EQ(engine.reset(), 0u);
}

TEST(EngineDir, ReportsWhetherADirectoryHoldsDatafiles) {
    const Scratch scratch;
    ASSERT_TRUE(scratch.valid()) << "could not make a scratch directory";

    // Reads the directory only, so it answers before any tier is up.
    EXPECT_FALSE(dbengine_dir_has_datafiles(scratch.c_str()));
    EXPECT_FALSE(dbengine_dir_has_datafiles("/nonexistent-dbengine-test-directory"))
        << "a directory that cannot be opened was reported to hold datafiles";

    // And the answer it exists to give: a directory a tier has used does hold datafiles. Without this the whole
    // case would pass with the function stubbed to always say no.
    const struct dbengine_config cfg = netdata_test_config();

    Engine engine(cfg);
    ASSERT_TRUE(engine);

    const struct dbengine_tier_config tc = tier_config(scratch);
    ASSERT_EQ(dbengine_tier_init(engine.get(), &tc), 0);
    dbengine_readiness_wait(dbengine_tier(engine.get(), 0));

    EXPECT_TRUE(dbengine_dir_has_datafiles(scratch.c_str()))
        << "a directory a tier came up on was reported to hold no datafiles";

    EXPECT_EQ(engine.reset(), 0u);

    // Still true once the engine is gone: it reads the directory, not the engine.
    EXPECT_TRUE(dbengine_dir_has_datafiles(scratch.c_str()));
}

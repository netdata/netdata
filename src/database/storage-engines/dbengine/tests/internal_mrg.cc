// SPDX-License-Identifier: GPL-3.0-or-later

#include "support.h"

extern "C" {
#include "database/storage-engines/dbengine/page.h"
#include "database/storage-engines/dbengine/mrg.h"
#include "database/storage-engines/dbengine/cache.h"
}

// The metrics registry, MRG: what the engine uses to find a metric and what it remembers about the metric's
// retention. The registry has a constructor for exactly this - one that belongs to no engine - so these cases need
// no engine, no tier and no directory.
//
// The existing registry tests live in one function whose later half asserts with fatal(), so the first failure ends
// the process and everything after it is never reached. These are separate cases for that reason: a registry
// regression should cost one result, not all of them.

namespace {

// A registry section is not a number: it is the address of the tier the metric belongs to, and the registry casts
// it back and follows it to the engine's main cache. Passing an arbitrary number as a section is a segfault waiting
// to happen, which is exactly what the first version of these cases did. So the fixture builds what the registry
// expects - a tier with its locks initialised, an engine with no event loop, and the main cache the registry
// reaches through the tier - and uses the tier's address as the section.
class MrgTest : public ::testing::Test {
protected:
    void SetUp() override {
        cfg_ = netdata_test_config();

        mrg_ = mrg_create_for_unittest();
        ASSERT_NE(mrg_, nullptr);

        engine_ = dbengine_engine_alloc(&cfg_);
        ASSERT_NE(engine_, nullptr);

        engine_->main_mrg = mrg_;
        engine_->main_cache = make_cache();
        ASSERT_NE(engine_->main_cache, nullptr);

        dbengine_tier_reset(&first_tier_);
        dbengine_tier_reset(&second_tier_);
        first_tier_.engine = engine_;
        second_tier_.engine = engine_;
    }

    void TearDown() override {
        if (mrg_) {
            mrg_destroy(mrg_);
            mrg_ = nullptr;
        }

        if (engine_) {
            if (engine_->main_cache) {
                pgc_destroy(engine_->main_cache, false);
                engine_->main_cache = nullptr;
            }

            engine_->main_mrg = nullptr;
            first_tier_.engine = nullptr;
            second_tier_.engine = nullptr;

            dbengine_engine_free(engine_);
            engine_ = nullptr;
        }
    }

    PGC *make_cache() {
        struct pgc_config cache_cfg = {};

        cache_cfg.name = "mrg-test-cache";
        cache_cfg.clean_size_bytes = 32 * 1024 * 1024;
        cache_cfg.free_clean_cb = nullptr;
        cache_cfg.max_dirty_pages_per_flush = 64;
        cache_cfg.save_init_cb = nullptr;
        cache_cfg.save_dirty_cb = nullptr;
        cache_cfg.max_pages_per_inline_eviction = 10;
        cache_cfg.max_inline_evictors = 10;
        cache_cfg.max_skip_pages_per_inline_eviction = 1000;
        cache_cfg.max_flushes_inline = 10;
        cache_cfg.options = static_cast<PGC_OPTIONS>(PGC_OPTIONS_DEFAULT);
        cache_cfg.partitions = 1;
        cache_cfg.additional_bytes_per_page = 0;
        cache_cfg.statistics = true;
        cache_cfg.use_all_ram = false;
        cache_cfg.out_of_memory_protection_bytes = 0;
        cache_cfg.cpus = cfg_.cpus;

        return pgc_create(&cache_cfg);
    }

    Word_t section() const { return reinterpret_cast<Word_t>(&first_tier_); }
    Word_t other_section() const { return reinterpret_cast<Word_t>(&second_tier_); }

    // A uuid nobody else uses. Registry ids are unique for the lifetime of the uuid map, which is never reset, so a
    // fresh uuid per case is what keeps the cases independent.
    static nd_uuid_t *fresh_uuid(nd_uuid_t &storage) {
        uuid_generate(storage);
        return &storage;
    }

    MRG_ENTRY entry(nd_uuid_t *uuid, Word_t in_section, time_t first_time_s = 1000, time_t last_time_s = 2000,
                    uint32_t update_every_s = 1) {
        MRG_ENTRY e = {};

        e.uuid = uuid;
        e.section = in_section;
        e.first_time_s = first_time_s;
        e.last_time_s = last_time_s;
        e.latest_update_every_s = update_every_s;

        return e;
    }

    MRG_ENTRY entry(nd_uuid_t *uuid) { return entry(uuid, section()); }

    struct dbengine_config cfg_ = {};
    MRG *mrg_ = nullptr;
    struct dbengine_engine *engine_ = nullptr;
    mutable struct dbengine_tier first_tier_ = {};
    mutable struct dbengine_tier second_tier_ = {};
};

} // namespace

TEST_F(MrgTest, AddsAMetricAndFindsItAgain) {
    nd_uuid_t uuid;
    fresh_uuid(uuid);

    bool added = false;
    METRIC *metric = mrg_metric_add_and_acquire(mrg_, entry(&uuid), &added);
    ASSERT_NE(metric, nullptr);
    EXPECT_TRUE(added);

    METRIC *found = mrg_metric_get_and_acquire_by_uuid(mrg_, &uuid, section());
    EXPECT_EQ(found, metric) << "the registry did not return the metric it had just been given";

    if (found)
        mrg_metric_release(mrg_, found);
    mrg_metric_release(mrg_, metric);
}

TEST_F(MrgTest, AddingTheSameMetricTwiceReturnsTheSameOne) {
    nd_uuid_t uuid;
    fresh_uuid(uuid);

    bool first_added = false;
    METRIC *first = mrg_metric_add_and_acquire(mrg_, entry(&uuid), &first_added);
    ASSERT_NE(first, nullptr);
    ASSERT_TRUE(first_added);

    bool second_added = true;
    METRIC *second = mrg_metric_add_and_acquire(mrg_, entry(&uuid), &second_added);
    ASSERT_NE(second, nullptr);

    EXPECT_FALSE(second_added) << "the registry reported an existing metric as newly added";
    EXPECT_EQ(first, second);

    mrg_metric_release(mrg_, second);
    mrg_metric_release(mrg_, first);
}

TEST_F(MrgTest, TheSameUuidInTwoSectionsIsTwoMetrics) {
    nd_uuid_t uuid;
    fresh_uuid(uuid);

    METRIC *in_first = mrg_metric_add_and_acquire(mrg_, entry(&uuid, section()), nullptr);
    METRIC *in_second = mrg_metric_add_and_acquire(mrg_, entry(&uuid, other_section()), nullptr);

    ASSERT_NE(in_first, nullptr);
    ASSERT_NE(in_second, nullptr);

    // A section is a tier: the same metric collected into two tiers is two entries with two retentions.
    EXPECT_NE(in_first, in_second) << "two tiers share one registry entry for the same metric";
    EXPECT_EQ(mrg_metric_section(in_first), section());
    EXPECT_EQ(mrg_metric_section(in_second), other_section());

    mrg_metric_release(mrg_, in_second);
    mrg_metric_release(mrg_, in_first);
}

TEST_F(MrgTest, AMetricThatWasNeverAddedIsNotFound) {
    nd_uuid_t uuid;
    fresh_uuid(uuid);

    EXPECT_EQ(mrg_metric_get_and_acquire_by_uuid(mrg_, &uuid, section()), nullptr);
}

TEST_F(MrgTest, RemembersTheRetentionItWasGiven) {
    nd_uuid_t uuid;
    fresh_uuid(uuid);

    METRIC *metric = mrg_metric_add_and_acquire(mrg_, entry(&uuid, section(), 1000, 2000, 5), nullptr);
    ASSERT_NE(metric, nullptr);

    time_t first = 0, last = 0;
    uint32_t update_every = 0;
    mrg_metric_get_retention(mrg_, metric, &first, &last, &update_every);

    EXPECT_EQ(first, 1000);
    EXPECT_EQ(last, 2000);
    EXPECT_EQ(update_every, 5u);

    mrg_metric_release(mrg_, metric);
}

TEST_F(MrgTest, RetentionExpandsToCoverWhatArrives) {
    nd_uuid_t uuid;
    fresh_uuid(uuid);

    METRIC *metric = mrg_metric_add_and_acquire(mrg_, entry(&uuid, section(), 1000, 2000, 1), nullptr);
    ASSERT_NE(metric, nullptr);

    // A journal read later can find data older and newer than the registry knew about.
    mrg_metric_expand_retention(mrg_, metric, 500, 3000, 1);

    time_t first = 0, last = 0;
    uint32_t update_every = 0;
    mrg_metric_get_retention(mrg_, metric, &first, &last, &update_every);

    EXPECT_EQ(first, 500) << "the registry did not take the older data";
    EXPECT_EQ(last, 3000) << "the registry did not take the newer data";

    mrg_metric_release(mrg_, metric);
}

TEST_F(MrgTest, RetentionDoesNotShrinkWhenOlderDataIsAlreadyKnown) {
    nd_uuid_t uuid;
    fresh_uuid(uuid);

    METRIC *metric = mrg_metric_add_and_acquire(mrg_, entry(&uuid, section(), 1000, 2000, 1), nullptr);
    ASSERT_NE(metric, nullptr);

    // Narrower than what is known: the registry must keep what it has.
    mrg_metric_expand_retention(mrg_, metric, 1500, 1800, 1);

    time_t first = 0, last = 0;
    uint32_t update_every = 0;
    mrg_metric_get_retention(mrg_, metric, &first, &last, &update_every);

    EXPECT_EQ(first, 1000) << "a narrower range moved the first time forward";
    EXPECT_EQ(last, 2000) << "a narrower range moved the last time back";

    mrg_metric_release(mrg_, metric);
}

TEST_F(MrgTest, TheFirstTimeOnlyGrowsWhenAskedForBigger) {
    nd_uuid_t uuid;
    fresh_uuid(uuid);

    METRIC *metric = mrg_metric_add_and_acquire(mrg_, entry(&uuid, section(), 1000, 2000, 1), nullptr);
    ASSERT_NE(metric, nullptr);

    EXPECT_FALSE(mrg_metric_set_first_time_s_if_bigger(mrg_, metric, 500))
        << "an older first time was accepted by the setter that only takes newer ones";
    EXPECT_EQ(mrg_metric_get_first_time_s(mrg_, metric), 1000);

    EXPECT_TRUE(mrg_metric_set_first_time_s_if_bigger(mrg_, metric, 1500));
    EXPECT_EQ(mrg_metric_get_first_time_s(mrg_, metric), 1500);

    mrg_metric_release(mrg_, metric);
}

TEST_F(MrgTest, ClearingRetentionLeavesTheMetricWithNone) {
    nd_uuid_t uuid;
    fresh_uuid(uuid);

    METRIC *metric = mrg_metric_add_and_acquire(mrg_, entry(&uuid, section(), 1000, 2000, 1), nullptr);
    ASSERT_NE(metric, nullptr);

    mrg_metric_clear_retention(mrg_, metric);

    // What clearing guarantees is that the registry forgets the times it held. (mrg_metric_has_zero_disk_retention()
    // is a different question - it walks the page cache for the metric's pages - and answering it needs pages in a
    // cache, which these cases do not build.)
    time_t first = 0, last = 0;
    uint32_t update_every = 0;
    mrg_metric_get_retention(mrg_, metric, &first, &last, &update_every);

    EXPECT_EQ(first, 0);
    EXPECT_EQ(last, 0);

    mrg_metric_release(mrg_, metric);
}

// The single-writer guard (mrg_metric_set_writer/clear_writer) exists only in internal-checks builds, so there is
// no case for it here: a case that vanishes with a build flag is worse than none, because the suite would quietly
// cover less in the build CI runs than in the one a developer runs locally.

TEST_F(MrgTest, TheUpdateEveryIsSetOnlyWhenItIsUnset) {
    nd_uuid_t uuid;
    fresh_uuid(uuid);

    METRIC *metric = mrg_metric_add_and_acquire(mrg_, entry(&uuid, section(), 1000, 2000, 0), nullptr);
    ASSERT_NE(metric, nullptr);

    EXPECT_TRUE(mrg_metric_set_update_every_s_if_zero(mrg_, metric, 10));
    EXPECT_EQ(mrg_metric_get_update_every_s(mrg_, metric), 10u);

    EXPECT_FALSE(mrg_metric_set_update_every_s_if_zero(mrg_, metric, 20))
        << "an already known collection frequency was overwritten";
    EXPECT_EQ(mrg_metric_get_update_every_s(mrg_, metric), 10u);

    mrg_metric_release(mrg_, metric);
}

TEST_F(MrgTest, CountsTheMetricsItHolds) {
    struct dbengine_metrics_registry_stats before = {};
    mrg_get_statistics(mrg_, &before);

    nd_uuid_t first_uuid, second_uuid;
    METRIC *first = mrg_metric_add_and_acquire(mrg_, entry(fresh_uuid(first_uuid)), nullptr);
    METRIC *second = mrg_metric_add_and_acquire(mrg_, entry(fresh_uuid(second_uuid)), nullptr);
    ASSERT_NE(first, nullptr);
    ASSERT_NE(second, nullptr);

    struct dbengine_metrics_registry_stats after = {};
    mrg_get_statistics(mrg_, &after);

    EXPECT_EQ(after.entries, before.entries + 2) << "the registry did not count both metrics";

    mrg_metric_release(mrg_, second);
    mrg_metric_release(mrg_, first);
}

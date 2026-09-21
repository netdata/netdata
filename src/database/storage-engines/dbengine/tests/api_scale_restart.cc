// SPDX-License-Identifier: GPL-3.0-or-later

#include "support.h"

#include <vector>

// A tier restarted on a directory it wrote, through the public surface. Every other case starts on an empty
// directory and ends with the engine destroyed, so the journal replay that a restart performs, the retention it
// recovers, the open cache it fills and the reads from disk that the first queries after it make all ran in nothing
// CI executes. Here the data is written, the engine is taken down the way the daemon takes it down, a new engine is
// brought up on the same directory, and the queries that follow are cold: every page they want is on disk.
//
// The points carry recent timestamps on purpose. At load the engine migrates the last datafile's journal to the
// indexed format when its data is older than a day, and leaves it as written otherwise; this case wants the latter,
// where the replay feeds the open cache and the queries take that path. The rotation case takes the former.
//
// The query priorities matter only for cold data: a query whose pages are all in the main cache routes nothing,
// whatever priority it names. The three routes are each taken exactly once here, one cold metric per priority.

namespace {

class ScaleRestartTest : public EngineFixture {
protected:
    struct dbengine_cache_efficiency_stats efficiency() { return dbengine_get_cache_efficiency_stats(engine_); }

    unsigned long long datafile_creations() {
        unsigned long long stats[DBENGINE_STATS_COUNT] = {};
        dbengine_get_stats(tier_, stats);
        return stats[23];
    }

    void expect_every_point(STORAGE_METRIC_HANDLE *smh, size_t m, time_t base, STORAGE_PRIORITY priority) {
        const std::vector<STORAGE_POINT> got = query_all(smh, base + 1, base + static_cast<time_t>(POINTS_PER_METRIC),
                                                         priority, 2048);
        ASSERT_EQ(got.size(), POINTS_PER_METRIC);
        for (size_t i = 0; i < got.size(); i++) {
            const time_t end = base + static_cast<time_t>(i + 1);
            expect_point(got[i], {end - 1, end, static_cast<NETDATA_DOUBLE>(m * 10000 + i + 1)});
            if (HasNonfatalFailure())
                break;
        }
    }

    static constexpr size_t METRICS = 3;
    static constexpr size_t POINTS_PER_METRIC = 1100;
};

} // namespace

TEST_F(ScaleRestartTest, WrittenDataSurvivesARestartAndIsReadFromDisk) {
    // Recent, so the last datafile is left as written at the reload (see the file comment), and in the past by
    // more than the points span, so nothing is collected into the future.
    const time_t base = now_realtime_sec() - 3000;

    std::vector<UUIDMAP_ID> ids;
    STORAGE_METRICS_GROUP *smg = dbengine_metrics_group_get();

    for (size_t m = 0; m < METRICS; m++) {
        const UUIDMAP_ID id = make_metric_id();
        STORAGE_METRIC_HANDLE *smh = dbengine_metric_get_or_create_by_id(si_, id);
        ASSERT_NE(smh, nullptr);
        STORAGE_COLLECT_HANDLE *sch = dbengine_store_init(smh, 1, smg);
        ASSERT_NE(sch, nullptr);

        for (size_t i = 1; i <= POINTS_PER_METRIC; i++)
            store_point(sch, base + static_cast<time_t>(i), static_cast<NETDATA_DOUBLE>(m * 10000 + i));

        dbengine_store_finalize(sch);

        // Before the restart the data is in the main cache and comes back from there.
        expect_every_point(smh, m, base, STORAGE_PRIORITY_SYNCHRONOUS);

        dbengine_metric_release(smh);
        ids.push_back(id);
    }
    dbengine_metrics_group_release(smg);

    // The daemon's shutdown order: the pages to disk while the tier still serves (a flush after the quiesce skips
    // the open cache, and this case wants the open cache filled by the reload the way a normal exit leaves it),
    // then no more queries, then the tier and the engine down. The destroy must find nothing referenced.
    ASSERT_TRUE(dbengine_flush_all_wait(tier_));
    dbengine_quiesce(tier_);
    take_down();
    ASSERT_FALSE(HasFatalFailure());

    // The same directory, a new engine.
    bring_up();
    ASSERT_FALSE(HasFatalFailure());

    // The last datafile was left as written: the reload created no new pair, and this tier's counters start at
    // zero, so the count is exactly that.
    EXPECT_EQ(datafile_creations(), 0u) << "the reload created a datafile pair it did not need";

    const struct dbengine_cache_efficiency_stats after_load = efficiency();

    // Retention was recovered from the journal for every metric, found again by the id it was created with.
    std::vector<STORAGE_METRIC_HANDLE *> metrics;
    for (size_t m = 0; m < METRICS; m++) {
        SCOPED_TRACE(m);

        time_t first = 0, last = 0;
        EXPECT_TRUE(dbengine_metric_retention_by_id(si_, ids[m], &first, &last));
        EXPECT_EQ(first, base + 1);
        EXPECT_EQ(last, base + static_cast<time_t>(POINTS_PER_METRIC));

        STORAGE_METRIC_HANDLE *smh = dbengine_metric_get_by_id(si_, ids[m]);
        ASSERT_NE(smh, nullptr) << "the metric was not found after the restart";
        metrics.push_back(smh);
    }

    // One cold query per priority, each reading every point of its metric from disk.
    const STORAGE_PRIORITY priorities[METRICS] = {
        STORAGE_PRIORITY_SYNCHRONOUS, STORAGE_PRIORITY_SYNCHRONOUS_FIRST, STORAGE_PRIORITY_NORMAL};
    for (size_t m = 0; m < METRICS; m++) {
        SCOPED_TRACE(m);
        expect_every_point(metrics[m], m, base, priorities[m]);
    }

    // A query returns as soon as its pages are published; the loader counts them as loaded a moment later, on its
    // own thread, so a sample taken the instant the last query returned can miss the last increment. Sampled once
    // every page the planner scheduled for loading is accounted for, from disk, the extent cache or a cache hit
    // while inserting, which is bounded by the loads themselves.
    struct dbengine_cache_efficiency_stats after_cold = efficiency();
    for (int i = 0; i < 1000; i++) {
        const size_t planned = after_cold.pages_to_load_from_disk - after_load.pages_to_load_from_disk;
        const size_t loaded = (after_cold.pages_data_source_disk - after_load.pages_data_source_disk) +
                              (after_cold.pages_data_source_extent_cache - after_load.pages_data_source_extent_cache) +
                              (after_cold.pages_load_ok_loaded_but_cache_hit_while_inserting -
                               after_load.pages_load_ok_loaded_but_cache_hit_while_inserting);
        if (loaded >= planned)
            break;
        sleep_usec(1000);
        after_cold = efficiency();
    }
    EXPECT_GE((after_cold.pages_data_source_disk - after_load.pages_data_source_disk) +
                  (after_cold.pages_data_source_extent_cache - after_load.pages_data_source_extent_cache) +
                  (after_cold.pages_load_ok_loaded_but_cache_hit_while_inserting -
                   after_load.pages_load_ok_loaded_but_cache_hit_while_inserting),
              after_cold.pages_to_load_from_disk - after_load.pages_to_load_from_disk)
        << "not every page the planner scheduled was loaded within a second";

    // Each route was taken exactly once: a route counter moves only for a query with pages to load from disk, and
    // each of the three queries was the first to touch its metric since the restart.
    EXPECT_EQ(after_cold.prep_time_to_route_sync.count - after_load.prep_time_to_route_sync.count, 1u);
    EXPECT_EQ(after_cold.prep_time_to_route_syncfirst.count - after_load.prep_time_to_route_syncfirst.count, 1u);
    EXPECT_EQ(after_cold.prep_time_to_route_async.count - after_load.prep_time_to_route_async.count, 1u);

    // Over the three as a block: the pages were found in the open cache the reload filled, and their data came
    // from disk. Per query this could not be asserted: three metrics written together may share an extent, and
    // the second and third to ask for it may be served from the extent cache instead.
    EXPECT_GT(after_cold.pages_meta_source_open_cache - after_load.pages_meta_source_open_cache, 0u);
    EXPECT_GT(after_cold.pages_data_source_disk - after_load.pages_data_source_disk, 0u);
    EXPECT_GT(after_cold.extents_loaded_from_disk - after_load.extents_loaded_from_disk, 0u);

    // Read once, the pages are in the main cache: a repeat is served from there and touches the disk no more.
    expect_every_point(metrics[0], 0, base, STORAGE_PRIORITY_SYNCHRONOUS);
    const struct dbengine_cache_efficiency_stats after_repeat = efficiency();
    EXPECT_GT(after_repeat.pages_data_source_main_cache - after_cold.pages_data_source_main_cache, 0u);
    EXPECT_EQ(after_repeat.pages_data_source_disk - after_cold.pages_data_source_disk, 0u);

    for (size_t m = 0; m < METRICS; m++) {
        dbengine_metric_release(metrics[m]);
        uuidmap_free(ids[m]);
    }
}

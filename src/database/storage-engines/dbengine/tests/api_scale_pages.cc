// SPDX-License-Identifier: GPL-3.0-or-later

#include "support.h"

#include <vector>

// The page boundary, through the public surface. Every other case in this suite stores a handful of points and
// never leaves the first page, so the code that ends a page and starts the next runs in nothing CI executes. These
// cases store enough to cross it, and drive each of the ways the store path ends a page early: a gap it fills, a
// gap it will not fill, a step too small for the metric and a step that is not a multiple of it.
//
// How many points a tier-0 page holds is not fixed: the engine sizes it from the metric's page alignment and the
// time of the first point, between a third of the page size and the whole of it (341 to 1024 points for this tier's
// point size). The counts here are chosen so the claims hold for any size in that range, and the page counts are
// asserted as lower bounds, never as exact figures.

namespace {

class ScalePagesTest : public EngineFixture {
protected:
    // A metric with a collect handle, released in the order the contract requires.
    struct Collector {
        UUIDMAP_ID id = 0;
        STORAGE_METRIC_HANDLE *smh = nullptr;
        STORAGE_METRICS_GROUP *smg = nullptr;
        STORAGE_COLLECT_HANDLE *sch = nullptr;

        // open() reports a handle it could not get, but cannot end the case itself: the case asserts this before
        // storing into the handles, so a refused handle fails the case by name rather than by a crash.
        bool opened() const { return smh && sch; }
    };

    Collector open(uint32_t update_every) {
        Collector c;
        c.id = make_metric_id();
        c.smh = dbengine_metric_get_or_create_by_id(si_, c.id);
        EXPECT_NE(c.smh, nullptr);
        c.smg = dbengine_metrics_group_get();
        c.sch = dbengine_store_init(c.smh, update_every, c.smg);
        EXPECT_NE(c.sch, nullptr);
        return c;
    }

    void close(Collector &c) {
        dbengine_store_finalize(c.sch);
        dbengine_metrics_group_release(c.smg);
        dbengine_metric_release(c.smh);
        uuidmap_free(c.id);
    }

    size_t main_cache_pages_added() {
        struct dbengine_cache_stats stats = {};
        EXPECT_TRUE(dbengine_get_cache_stats(engine_, DBENGINE_CACHE_MAIN, &stats));
        return stats.added_entries;
    }
};

const time_t BASE_TIME = 200000000;

// More points than the largest page holds, so at least two pages exist whatever size the engine picked.
const size_t POINTS_ACROSS_PAGES = 1100;

} // namespace

TEST_F(ScalePagesTest, PointsAcrossAPageBoundaryAllComeBack) {
    Collector c = open(1);
    ASSERT_TRUE(c.opened());
    const size_t pages_before = main_cache_pages_added();

    for (size_t i = 1; i <= POINTS_ACROSS_PAGES; i++)
        store_point(c.sch, BASE_TIME + static_cast<time_t>(i), static_cast<NETDATA_DOUBLE>(i));

    dbengine_store_flush(c.sch);

    // A single page cannot hold them, so the store path must have ended one and started another.
    EXPECT_GE(main_cache_pages_added() - pages_before, 2u) << "the points fitted in one page";

    // Retention spans everything stored, across the page boundary.
    EXPECT_EQ(dbengine_oldest_time_s(c.smh), BASE_TIME + 1);
    EXPECT_EQ(dbengine_latest_time_s(c.smh), BASE_TIME + static_cast<time_t>(POINTS_ACROSS_PAGES));

    // The query walks from one page into the next and returns every point in order, with its value.
    const std::vector<STORAGE_POINT> got = query_all(c.smh, BASE_TIME + 1, BASE_TIME + static_cast<time_t>(POINTS_ACROSS_PAGES),
                                                     STORAGE_PRIORITY_SYNCHRONOUS, 2048);
    ASSERT_EQ(got.size(), POINTS_ACROSS_PAGES);

    for (size_t i = 0; i < got.size(); i++) {
        const time_t end = BASE_TIME + static_cast<time_t>(i + 1);
        expect_point(got[i], {end - 1, end, static_cast<NETDATA_DOUBLE>(i + 1)});
        if (HasNonfatalFailure())
            break;
    }

    close(c);
}

TEST_F(ScalePagesTest, AShortGapIsFilledWithEmptySlotsOnTheSamePage) {
    Collector c = open(1);
    ASSERT_TRUE(c.opened());
    const size_t pages_before = main_cache_pages_added();

    // Two missing seconds between two points: the store path fills them so the page stays one page.
    store_point(c.sch, BASE_TIME + 1, 1);
    store_point(c.sch, BASE_TIME + 4, 4);
    dbengine_store_flush(c.sch);

    EXPECT_EQ(main_cache_pages_added() - pages_before, 1u) << "a short gap ended the page";

    const std::vector<STORAGE_POINT> got = query_all(c.smh, BASE_TIME + 1, BASE_TIME + 4);
    ASSERT_EQ(got.size(), 4u);

    expect_point(got[0], {BASE_TIME, BASE_TIME + 1, 1});
    // The filled slots come back as gaps, in their place, rather than being skipped over.
    EXPECT_EQ(got[1].end_time_s, BASE_TIME + 2);
    EXPECT_TRUE(storage_point_is_gap(got[1]));
    EXPECT_EQ(got[2].end_time_s, BASE_TIME + 3);
    EXPECT_TRUE(storage_point_is_gap(got[2]));
    expect_point(got[3], {BASE_TIME + 3, BASE_TIME + 4, 4});

    close(c);
}

TEST_F(ScalePagesTest, AGapLargerThanAPageStartsANewPage) {
    Collector c = open(1);
    ASSERT_TRUE(c.opened());
    const size_t pages_before = main_cache_pages_added();

    // A gap no page could hold: rather than fill it, the store path ends the page and starts another at the
    // point after the gap. 1100 empty slots exceed what remains of any page size the engine can pick.
    const time_t after_gap = BASE_TIME + 1 + 1100;
    store_point(c.sch, BASE_TIME + 1, 1);
    store_point(c.sch, after_gap, 2);
    dbengine_store_flush(c.sch);

    EXPECT_GE(main_cache_pages_added() - pages_before, 2u) << "the big gap did not end the page";

    EXPECT_EQ(dbengine_oldest_time_s(c.smh), BASE_TIME + 1);
    EXPECT_EQ(dbengine_latest_time_s(c.smh), after_gap);

    // The query shows the gap as a jump from one page to the next: two points, nothing filled in between.
    const std::vector<STORAGE_POINT> got = query_all(c.smh, BASE_TIME + 1, after_gap);
    ASSERT_EQ(got.size(), 2u);
    expect_point(got[0], {BASE_TIME, BASE_TIME + 1, 1});
    expect_point(got[1], {after_gap - 1, after_gap, 2});

    close(c);
}

TEST_F(ScalePagesTest, AStepSmallerThanTheMetricsEndsThePage) {
    // A metric collected every two seconds, then a point one second after the last: the page cannot represent
    // it, so the store path ends the page and the point starts the next one.
    Collector c = open(2);
    ASSERT_TRUE(c.opened());
    const size_t pages_before = main_cache_pages_added();

    store_point(c.sch, BASE_TIME + 2, 1);
    store_point(c.sch, BASE_TIME + 4, 2);
    store_point(c.sch, BASE_TIME + 5, 3);
    store_point(c.sch, BASE_TIME + 7, 4);
    dbengine_store_flush(c.sch);

    EXPECT_GE(main_cache_pages_added() - pages_before, 2u) << "the too-small step did not end the page";

    // Both pages hold their points. Each half is queried on its own grid: the second page starts at an odd second,
    // so a single query walking the metric's two-second grid from the first page would step over its first point.
    const std::vector<STORAGE_POINT> before = query_all(c.smh, BASE_TIME + 2, BASE_TIME + 4);
    ASSERT_EQ(before.size(), 2u);
    expect_point(before[0], {BASE_TIME, BASE_TIME + 2, 1});
    expect_point(before[1], {BASE_TIME + 2, BASE_TIME + 4, 2});

    const std::vector<STORAGE_POINT> after = query_all(c.smh, BASE_TIME + 5, BASE_TIME + 7);
    ASSERT_EQ(after.size(), 2u);
    expect_point(after[0], {BASE_TIME + 3, BASE_TIME + 5, 3});
    expect_point(after[1], {BASE_TIME + 5, BASE_TIME + 7, 4});

    close(c);
}

TEST_F(ScalePagesTest, AStepThatIsNotAMultipleOfTheMetricsEndsThePage) {
    // Every two seconds, then a point three seconds after the last: not a gap the page can fill on its grid, so
    // the page ends and the point starts the next one.
    Collector c = open(2);
    ASSERT_TRUE(c.opened());
    const size_t pages_before = main_cache_pages_added();

    store_point(c.sch, BASE_TIME + 2, 1);
    store_point(c.sch, BASE_TIME + 4, 2);
    store_point(c.sch, BASE_TIME + 7, 3);
    store_point(c.sch, BASE_TIME + 9, 4);
    dbengine_store_flush(c.sch);

    EXPECT_GE(main_cache_pages_added() - pages_before, 2u) << "the unaligned step did not end the page";

    const std::vector<STORAGE_POINT> before = query_all(c.smh, BASE_TIME + 2, BASE_TIME + 4);
    ASSERT_EQ(before.size(), 2u);
    expect_point(before[0], {BASE_TIME, BASE_TIME + 2, 1});
    expect_point(before[1], {BASE_TIME + 2, BASE_TIME + 4, 2});

    const std::vector<STORAGE_POINT> after = query_all(c.smh, BASE_TIME + 7, BASE_TIME + 9);
    ASSERT_EQ(after.size(), 2u);
    expect_point(after[0], {BASE_TIME + 5, BASE_TIME + 7, 3});
    expect_point(after[1], {BASE_TIME + 7, BASE_TIME + 9, 4});

    close(c);
}

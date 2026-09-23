// SPDX-License-Identifier: GPL-3.0-or-later

#include "support.h"

#include <vector>

// Pages leaving the cache for the disk, through the public surface. In every other case the pages a collector
// makes stay hot or dirty in the main cache until the tier exits, and the extent writer runs only then, with the
// test no longer looking. Here the extent size is set to two pages so a handful of metrics produce many extents,
// the flush is waited on, and what reached the disk is read off the tier's counters.
//
// The counters are sampled as deltas across the case, never as absolute values, and compared as bounds: the
// engine's own periodic flusher runs every second and may write some of the pages before the case asks for the
// flush, which changes when the counters move but not where they end up.

namespace {

class ScaleExtentsTest : public EngineFixture {
protected:
    struct dbengine_config engine_config() override {
        struct dbengine_config cfg = netdata_test_config();
        // Two pages per extent: enough to prove pages are packed into extents, few enough that four metrics of a
        // few pages each make many of them. The default of 109 would put everything here into one.
        cfg.pages_per_extent = 2;
        return cfg;
    }

    struct dbengine_cache_stats main_cache_stats() {
        struct dbengine_cache_stats stats = {};
        EXPECT_TRUE(dbengine_get_cache_stats(engine_, DBENGINE_CACHE_MAIN, &stats));
        return stats;
    }

    void tier_stats(unsigned long long *array) { dbengine_get_stats(tier_, array); }
};

// dbengine_get_stats() indexes; the array is the daemon's legacy chart layout and these are the entries it fills.
const size_t STAT_BYTES_BEFORE_COMPRESSION = 11;
const size_t STAT_BYTES_AFTER_COMPRESSION = 12;
const size_t STAT_IO_WRITE_REQUESTS = 16;
const size_t STAT_IO_ERRORS = 28;

const time_t BASE_TIME = 200000000;
const size_t METRICS = 4;
const size_t POINTS_PER_METRIC = 1100;

} // namespace

TEST_F(ScaleExtentsTest, AWaitedFlushPutsEveryPageOnDisk) {
    const struct dbengine_cache_stats cache_before = main_cache_stats();
    unsigned long long tier_before[DBENGINE_STATS_COUNT] = {};
    tier_stats(tier_before);

    std::vector<UUIDMAP_ID> ids;
    std::vector<STORAGE_METRIC_HANDLE *> metrics;
    STORAGE_METRICS_GROUP *smg = dbengine_metrics_group_get();

    for (size_t m = 0; m < METRICS; m++) {
        const UUIDMAP_ID id = make_metric_id();
        STORAGE_METRIC_HANDLE *smh = dbengine_metric_get_or_create_by_id(si_, id);
        ASSERT_NE(smh, nullptr);
        STORAGE_COLLECT_HANDLE *sch = dbengine_store_init(smh, 1, smg);
        ASSERT_NE(sch, nullptr);

        // Every metric gets more points than one page holds, so each makes at least two pages.
        for (size_t i = 1; i <= POINTS_PER_METRIC; i++)
            store_point(sch, BASE_TIME + static_cast<time_t>(i), static_cast<NETDATA_DOUBLE>(m * 10000 + i));

        // The collector is done before the flush is asked for: the wait covers what is hot or dirty at the call,
        // and a page still being collected into is neither.
        dbengine_store_finalize(sch);

        ids.push_back(id);
        metrics.push_back(smh);
    }

    ASSERT_TRUE(dbengine_flush_all_wait(tier_)) << "the engine refused the flush";

    // What the wait promises, read the moment it returns. Nothing collects any more and the flush emptied the
    // dirty queue, so both queues are empty; and every page made is on disk, which is visible as the number of
    // write requests the tier issued: with two pages per extent, N pages take at least N/2 extents, and each
    // extent is written by a request of its own (its journal record by another). A competing flusher's batch that
    // had not reached the disk yet would leave the count short, which is what the wait exists to prevent. The wait
    // also ends on an extent whose write failed and was dropped, which only the I/O error counter shows, and the
    // floor on write requests is too loose to notice a few missing extents: no write may have failed.
    const struct dbengine_cache_stats cache_after = main_cache_stats();
    unsigned long long tier_after[DBENGINE_STATS_COUNT] = {};
    tier_stats(tier_after);

    EXPECT_EQ(tier_after[STAT_IO_ERRORS], tier_before[STAT_IO_ERRORS]) << "a write failed, so pages may be lost";
    EXPECT_EQ(cache_after.queues[DBENGINE_CACHE_QUEUE_HOT].entries, 0u) << "pages stayed hot after the flush";
    EXPECT_EQ(cache_after.queues[DBENGINE_CACHE_QUEUE_DIRTY].entries, 0u) << "pages stayed dirty after the flush";

    const size_t pages_made = cache_after.added_entries - cache_before.added_entries;
    EXPECT_GE(pages_made, METRICS * 2) << "a metric fitted in one page";

    const unsigned long long writes = tier_after[STAT_IO_WRITE_REQUESTS] - tier_before[STAT_IO_WRITE_REQUESTS];
    EXPECT_GE(writes, (pages_made + 1) / 2) << "fewer extents reached the disk than the pages need";

    // The pages a flusher has written are counted out of "pending" a moment after their extent is on disk, by the
    // flusher's own thread; the wait returns on the extent, so this can trail it by microseconds. Bounded rather
    // than asserted at once: a second is far beyond the trail and far below a hang.
    size_t pending = dbengine_pages_pending_flush(engine_);
    for (int i = 0; i < 1000 && pending; i++) {
        sleep_usec(1000);
        pending = dbengine_pages_pending_flush(engine_);
    }
    EXPECT_EQ(pending, 0u) << "pages were still pending a flush a second after the wait returned";

    // Every page made was flushed; the cache counts pages (not extents) as it completes them.
    const size_t pages_flushed = main_cache_stats().flushes_completed - cache_before.flushes_completed;
    EXPECT_GE(pages_flushed, pages_made) << "pages were made that no flush completed";

    // Compression moves both byte counters, and never grows the data; a build without a compressor moves neither.
    const unsigned long long before_bytes = tier_after[STAT_BYTES_BEFORE_COMPRESSION] - tier_before[STAT_BYTES_BEFORE_COMPRESSION];
    const unsigned long long after_bytes = tier_after[STAT_BYTES_AFTER_COMPRESSION] - tier_before[STAT_BYTES_AFTER_COMPRESSION];
    EXPECT_LE(after_bytes, before_bytes);
    if (before_bytes > 0)
        EXPECT_GT(after_bytes, 0u);

    // Flushed pages are clean pages, still readable: every point of every metric comes back.
    for (size_t m = 0; m < METRICS; m++) {
        SCOPED_TRACE(m);
        const std::vector<STORAGE_POINT> got = query_all(metrics[m], BASE_TIME + 1, BASE_TIME + static_cast<time_t>(POINTS_PER_METRIC),
                                                         STORAGE_PRIORITY_SYNCHRONOUS, 2048);
        ASSERT_EQ(got.size(), POINTS_PER_METRIC);
        for (size_t i = 0; i < got.size(); i++) {
            const time_t end = BASE_TIME + static_cast<time_t>(i + 1);
            expect_point(got[i], {end - 1, end, static_cast<NETDATA_DOUBLE>(m * 10000 + i + 1)});
            if (HasNonfatalFailure())
                break;
        }
    }

    for (size_t m = 0; m < METRICS; m++) {
        dbengine_metric_release(metrics[m]);
        uuidmap_free(ids[m]);
    }
    dbengine_metrics_group_release(smg);
}

// SPDX-License-Identifier: GPL-3.0-or-later

#include "support.h"

#include <dirent.h>

#include <cstring>
#include <string>
#include <vector>

// Datafile rotation, through the public surface: a tier that fills datafiles, indexes the ones it has finished
// with, deletes the oldest when its retention limit says so, and after a restart reads what survived from the
// indexed journal. None of it ran in anything CI executed: no case gave a tier a retention limit or wrote enough
// to fill a second datafile, and without both the rotation is unreachable by design.
//
// The tier is configured to reach all of it with about a thousand points: one page per extent (an extent costs at
// least a disk block whatever it holds), the smallest disk quota (whose datafile target is the smallest datafile),
// a retention limit of one second and timestamps from 1976, so that the first datafile is past its retention the
// moment a second exists. The engine deletes only while more than two datafiles exist and keeps the last two, so
// three datafiles yield exactly one deletion.
//
// What proves the rotation is the tier's state once it settles: the counters, and the indexed journal of the
// surviving non-last datafile on disk. The rotation callback the engine offers is used only as a wake-up: it fires
// whether or not the deletion succeeded, so nothing is asserted on it.
//
// The old timestamps also select the other branch of the restart: the last datafile's data is older than a day,
// so at the reload the engine migrates its journal to the indexed format and starts a new pair. The restart case
// takes the branch that leaves the last file as written; between them the suite has both.

namespace {

// The completion the rotation callback marks. The engine holds the callback for as long as it lives, and the
// callback takes no argument, so this is a static of the file: initialised once, never destroyed, and reset only
// while no engine holds the callback (engine_config() runs before dbengine_create(), and after the previous
// engine's destroy). gtest runs the cases of a binary one at a time, so one completion serves.
struct completion &rotation_completion() {
    static struct completion c;
    static bool initialised = false;
    if (!initialised) {
        completion_init(&c);
        initialised = true;
    }
    return c;
}

void on_db_rotation() {
    completion_mark_complete(&rotation_completion());
}

class ScaleRotationTest : public EngineFixture {
protected:
    struct dbengine_config engine_config() override {
        completion_reset(&rotation_completion());

        struct dbengine_config cfg = netdata_test_config();
        // One page per extent, so every flushed page costs a disk block of datafile: the cheapest way to fill one.
        cfg.pages_per_extent = 1;
        cfg.on_db_rotation = on_db_rotation;
        return cfg;
    }

    struct dbengine_tier_config tier_config(size_t tier) override {
        struct dbengine_tier_config tc = EngineFixture::tier_config(tier);
        // The smallest quota the engine accepts; its datafile target is the smallest datafile (512 KiB). The
        // quota itself is never reached here.
        tc.disk_space_mb = 25;
        // A second of retention before the restart: the 1976 data is past it at once. None after: the restart
        // must reload what survived without rotating any further.
        tc.max_retention_s = restarted_ ? 0 : 1;
        return tc;
    }

    struct dbengine_cache_efficiency_stats efficiency() { return dbengine_get_cache_efficiency_stats(engine_); }

    void tier_stats(unsigned long long *array) { dbengine_get_stats(tier_, array); }

    size_t registry_references() {
        struct dbengine_metrics_registry_stats stats = {};
        EXPECT_TRUE(dbengine_get_metrics_registry_stats(engine_, &stats));
        return static_cast<size_t>(stats.current_references);
    }

    // Whether the scratch directory holds the indexed journal of the datafile with the given number. The name
    // carries the tier's own numbering as well, which is not part of the public surface; the suffix is enough.
    bool indexed_journal_exists(unsigned fileno) {
        char suffix[64];
        snprintf(suffix, sizeof(suffix), "-%010u.njfv2", fileno);

        bool found = false;
        DIR *dir = opendir(scratch_.c_str());
        if (!dir)
            return false;
        while (const struct dirent *entry = readdir(dir)) {
            const size_t n = strlen(entry->d_name), s = strlen(suffix);
            if (n >= s && strcmp(entry->d_name + n - s, suffix) == 0)
                found = true;
        }
        closedir(dir);
        return found;
    }

    bool restarted_ = false;

    static constexpr size_t METRICS = 8;
    static constexpr size_t POINTS_PER_ROUND = 3;
    // A generous bound on the pages the loop may write before it gives up: three datafiles of the smallest size
    // take about 3 x 127 block-sized extents, and every page here is one extent.
    static constexpr size_t MAX_PAGES = 2000;
};

const size_t STAT_DATAFILE_CREATIONS = 23;
const size_t STAT_DATAFILE_DELETIONS = 24;

const time_t BASE_TIME = 200000000;

} // namespace

TEST_F(ScaleRotationTest, TheOldestDatafileIsDeletedAndTheSurvivorsAreReadAfterARestart) {
    std::vector<UUIDMAP_ID> ids;
    std::vector<STORAGE_METRIC_HANDLE *> metrics;
    std::vector<STORAGE_COLLECT_HANDLE *> collectors;
    STORAGE_METRICS_GROUP *smg = dbengine_metrics_group_get();

    for (size_t m = 0; m < METRICS; m++) {
        const UUIDMAP_ID id = make_metric_id();
        STORAGE_METRIC_HANDLE *smh = dbengine_metric_get_or_create_by_id(si_, id);
        ASSERT_NE(smh, nullptr);
        STORAGE_COLLECT_HANDLE *sch = dbengine_store_init(smh, 1, smg);
        ASSERT_NE(sch, nullptr);
        ids.push_back(id);
        metrics.push_back(smh);
        collectors.push_back(sch);
    }

    // Every metric moves through time in lockstep, a few points per round, so the datafiles are ordered in time
    // and the first one written holds the oldest data of every metric. Each round ends every metric's page and
    // waits for the flush, so each round adds one extent per metric to the current datafile. A point's value is
    // its offset from BASE_TIME, so any point read back can be checked from its timestamp alone.
    unsigned long long stats[DBENGINE_STATS_COUNT] = {};
    time_t last_time = BASE_TIME;
    size_t pages_written = 0;
    do {
        for (size_t p = 0; p < POINTS_PER_ROUND; p++) {
            last_time++;
            for (size_t m = 0; m < METRICS; m++)
                store_point(collectors[m], last_time, static_cast<NETDATA_DOUBLE>(last_time - BASE_TIME));
        }
        for (size_t m = 0; m < METRICS; m++)
            dbengine_store_flush(collectors[m]);
        ASSERT_TRUE(dbengine_flush_all_wait(tier_));
        pages_written += METRICS;

        tier_stats(stats);
    } while (stats[STAT_DATAFILE_CREATIONS] < 3 && pages_written < MAX_PAGES);

    ASSERT_GE(stats[STAT_DATAFILE_CREATIONS], 3u) << "no third datafile after " << pages_written << " pages";

    // The collectors are done and the case holds no metric: the settle below waits for the registry to report no
    // references, and these would be some.
    for (size_t m = 0; m < METRICS; m++) {
        dbengine_store_finalize(collectors[m]);
        dbengine_metric_release(metrics[m]);
    }
    dbengine_metrics_group_release(smg);

    // The wake-up. Bounded because a hang here would otherwise be the suite's silent failure mode; not the proof,
    // because the callback fires even when the deletion could not acquire the file and gave up.
    EXPECT_TRUE(completion_timedwait_for(&rotation_completion(), 60)) << "no rotation callback within 60 s";

    // The proof, once the tier settles: three datafiles were made and one deleted (the engine keeps the last two),
    // the surviving non-last datafile (the second) has its indexed journal on disk, and nothing in the registry is
    // referenced any more (the indexing holds metrics while it runs, and the teardown does not wait for it). The
    // indexer is re-armed by the rotation for exactly one more file per round, which is what indexes the second
    // file after the first was deleted; the bound makes a skipped re-arm a loud failure rather than a hang.
    bool settled = false;
    for (int i = 0; i < 6000 && !settled; i++) {
        tier_stats(stats);
        settled = stats[STAT_DATAFILE_CREATIONS] == 3 && stats[STAT_DATAFILE_DELETIONS] == 1 &&
                  indexed_journal_exists(2) && registry_references() == 0;
        if (!settled)
            sleep_usec(10 * USEC_PER_MS);
    }
    tier_stats(stats);
    EXPECT_EQ(stats[STAT_DATAFILE_CREATIONS], 3u);
    EXPECT_EQ(stats[STAT_DATAFILE_DELETIONS], 1u) << "the oldest datafile was not deleted";
    EXPECT_TRUE(indexed_journal_exists(2)) << "the surviving second datafile has no indexed journal";
    EXPECT_EQ(registry_references(), 0u) << "the registry still holds references after the settle";

    const struct dbengine_cache_efficiency_stats after_rotation = efficiency();
    EXPECT_GE(after_rotation.datafile_deletion_started, 1u);
    EXPECT_GE(after_rotation.journal_v2_indexing_started, 1u);

    // Retention moved past the deleted file: the tier's oldest time is later than the first point stored, the
    // oldest window reads as nothing, and the newest still reads its points.
    EXPECT_GT(dbengine_global_first_time_s(si_), BASE_TIME + 1) << "the tier's first time did not move";

    STORAGE_METRIC_HANDLE *smh = dbengine_metric_get_by_id(si_, ids[0]);
    ASSERT_NE(smh, nullptr);

    const std::vector<STORAGE_POINT> oldest = query_all(smh, BASE_TIME + 1, BASE_TIME + POINTS_PER_ROUND);
    for (const STORAGE_POINT &sp : oldest)
        EXPECT_TRUE(storage_point_is_gap(sp)) << "a point of the deleted datafile came back at " << sp.end_time_s;

    const std::vector<STORAGE_POINT> newest = query_all(smh, last_time - (POINTS_PER_ROUND - 1), last_time);
    ASSERT_EQ(newest.size(), POINTS_PER_ROUND);
    for (size_t i = 0; i < newest.size(); i++) {
        const time_t end = last_time - static_cast<time_t>(POINTS_PER_ROUND - 1 - i);
        expect_point(newest[i], {end - 1, end, static_cast<NETDATA_DOUBLE>(end - BASE_TIME)});
    }
    dbengine_metric_release(smh);

    // The restart, with no retention limit: the reload must not rotate any further. With data older than a day
    // in the last file, the reload migrates its journal to the indexed format and starts a new pair, so this
    // tier counts one creation of its own and three datafiles exist.
    dbengine_quiesce(tier_);
    take_down();
    ASSERT_FALSE(HasFatalFailure());

    restarted_ = true;
    bring_up();
    ASSERT_FALSE(HasFatalFailure());

    tier_stats(stats);
    EXPECT_EQ(stats[STAT_DATAFILE_CREATIONS], 1u) << "the reload did not start the pair the migration calls for";
    EXPECT_EQ(stats[STAT_DATAFILE_DELETIONS], 0u) << "the reload rotated with no retention limit";

    const struct dbengine_cache_efficiency_stats after_load = efficiency();

    // What survived is read from the oldest time the registry recovered to the last point stored: the second
    // datafile's pages through its indexed journal, the migrated last file's through the open cache. Every point
    // in that range is present and carries the value its timestamp says.
    time_t first = 0, last = 0;
    ASSERT_TRUE(dbengine_metric_retention_by_id(si_, ids[0], &first, &last));
    EXPECT_GT(first, BASE_TIME + 1) << "the retention recovered at the reload includes the deleted file";
    EXPECT_EQ(last, last_time);

    smh = dbengine_metric_get_by_id(si_, ids[0]);
    ASSERT_NE(smh, nullptr);
    const std::vector<STORAGE_POINT> survived = query_all(smh, first, last, STORAGE_PRIORITY_SYNCHRONOUS, 2048);
    ASSERT_EQ(survived.size(), static_cast<size_t>(last - first + 1));
    for (size_t i = 0; i < survived.size(); i++) {
        const time_t end = first + static_cast<time_t>(i);
        expect_point(survived[i], {end - 1, end, static_cast<NETDATA_DOUBLE>(end - BASE_TIME)});
        if (HasNonfatalFailure())
            break;
    }
    dbengine_metric_release(smh);

    const struct dbengine_cache_efficiency_stats after_query = efficiency();
    EXPECT_GT(after_query.pages_meta_source_journal_v2 - after_load.pages_meta_source_journal_v2, 0u)
        << "no page was found through the indexed journal";

    EXPECT_FALSE(completion_is_done(&rotation_completion())) << "the restarted tier rotated";

    for (size_t m = 0; m < METRICS; m++)
        uuidmap_free(ids[m]);
}

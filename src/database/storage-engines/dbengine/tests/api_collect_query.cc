// SPDX-License-Identifier: GPL-3.0-or-later

#include "support.h"

#include <string>
#include <vector>

// The collect and query path, through the public surface only: a metric is created, points are stored into it, and
// the query gives them back. What comes back is compared field by field, including anomaly_count, which is easy to
// leave out of a comparison and silently never check.
//
// Two properties of the engine shape how the comparisons are written, and both were found by getting them wrong:
//
//  - a point covers the interval that ends at its timestamp, so its start_time_s is the time of the point before
//    it, not its own. For a metric collected every second, start is end minus one.
//  - tier 0 keeps values in the engine's own number format rather than as doubles, so a value does not always
//    come back with the bit pattern it went in with (30 comes back as 30.000000000000004). Values are compared as
//    doubles, which allows for the representation and still catches a wrong number.

namespace {

// One engine with tier 0 up and ready, torn down in the order the contract requires. Every case in this file needs
// the same thing, and needs it on a directory of its own.
class CollectQueryTest : public ::testing::Test {
protected:
    void SetUp() override {
        ASSERT_TRUE(scratch_.valid()) << "could not make a scratch directory";

        const struct dbengine_config cfg = netdata_test_config();

        engine_ = dbengine_create(&cfg);
        ASSERT_NE(engine_, nullptr) << "the engine did not come up";

        struct dbengine_tier_config tc = {};
        tc.tier = 0;
        tc.dbfiles_path = scratch_.c_str();
        tc.disk_space_mb = 0;
        tc.max_retention_s = 0;
        tc.page_type = DBENGINE_PAGE_TYPE_GORILLA_32BIT;
        tc.grouping = 1;

        ASSERT_EQ(dbengine_tier_init(engine_, &tc), 0) << "the tier did not come up";

        tier_ = dbengine_tier(engine_, 0);
        ASSERT_NE(tier_, nullptr);

        // Nothing may be collected or queried until the tier's registry load is done.
        dbengine_readiness_wait(tier_);

        si_ = reinterpret_cast<STORAGE_INSTANCE *>(tier_);
    }

    void TearDown() override {
        if (!engine_)
            return;

        if (dbengine_tier_is_active(tier_))
            dbengine_tier_exit(tier_);

        dbengine_shutdown(engine_);
        EXPECT_EQ(dbengine_destroy(engine_), 0u) << "metrics stayed referenced across the destroy";
        engine_ = nullptr;
    }

    // A metric nobody else in the process shares: uuid map ids are unique for the map's lifetime and it is never
    // reset, so a fresh uuid per case keeps the cases independent of each other.
    UUIDMAP_ID make_metric_id() {
        nd_uuid_t uuid;
        uuid_generate(uuid);
        return uuidmap_create(uuid);
    }

    DBENGINE_ENGINE *engine_ = nullptr;
    DBENGINE_TIER *tier_ = nullptr;
    STORAGE_INSTANCE *si_ = nullptr;

private:
    Scratch scratch_;
};

struct Point {
    time_t start_time_s;
    time_t end_time_s;
    NETDATA_DOUBLE value;
};

void store_point(STORAGE_COLLECT_HANDLE *sch, time_t end_time_s, NETDATA_DOUBLE value) {
    dbengine_store_next(sch, static_cast<usec_t>(end_time_s) * USEC_PER_SEC, value, value, value, 1, 0,
                        SN_DEFAULT_FLAGS);
}

// Reads the whole query and returns what it gave, rather than asserting inside the loop: a case that expected three
// points and got two should say so once, with both lists in hand.
std::vector<STORAGE_POINT> query_all(STORAGE_METRIC_HANDLE *smh, time_t start_time_s, time_t end_time_s) {
    std::vector<STORAGE_POINT> points;

    struct storage_engine_query_handle seqh = {};
    dbengine_query_init(smh, &seqh, start_time_s, end_time_s, STORAGE_PRIORITY_SYNCHRONOUS);

    // Bounded: a query that never reports itself finished is a failure, not a reason to spin.
    const size_t safety = 64;
    while (points.size() < safety && !dbengine_query_is_finished(&seqh))
        points.push_back(dbengine_query_next(&seqh));

    EXPECT_TRUE(dbengine_query_is_finished(&seqh)) << "the query did not finish within " << safety << " points";

    dbengine_query_finalize(&seqh);
    return points;
}

void expect_point(const STORAGE_POINT &sp, const Point &expected) {
    SCOPED_TRACE(static_cast<long long>(expected.end_time_s));

    EXPECT_EQ(sp.start_time_s, expected.start_time_s);
    EXPECT_EQ(sp.end_time_s, expected.end_time_s);
    EXPECT_DOUBLE_EQ(sp.min, expected.value);
    EXPECT_DOUBLE_EQ(sp.max, expected.value);
    EXPECT_DOUBLE_EQ(sp.sum, expected.value);
    EXPECT_EQ(sp.count, 1u);
    EXPECT_EQ(sp.anomaly_count, 0u);
    EXPECT_EQ(sp.flags, SN_DEFAULT_FLAGS);
    EXPECT_FALSE(storage_point_is_gap(sp));
}

const time_t BASE_TIME = 200000000;

} // namespace

TEST_F(CollectQueryTest, StoredPointsComeBack) {
    const UUIDMAP_ID id = make_metric_id();

    STORAGE_METRIC_HANDLE *smh = dbengine_metric_get_or_create_by_id(si_, id);
    ASSERT_NE(smh, nullptr);

    STORAGE_METRICS_GROUP *smg = dbengine_metrics_group_get();
    STORAGE_COLLECT_HANDLE *sch = dbengine_store_init(smh, 1, smg);
    ASSERT_NE(sch, nullptr);

    // start_time_s is the time of the previous point: the metric is collected every second, so each point covers
    // the second that ends at its own timestamp.
    const std::vector<Point> expected = {
        {BASE_TIME, BASE_TIME + 1, 10},
        {BASE_TIME + 1, BASE_TIME + 2, 20},
        {BASE_TIME + 2, BASE_TIME + 3, 30},
    };

    for (const Point &p : expected)
        store_point(sch, p.end_time_s, p.value);

    dbengine_store_flush(sch);

    const std::vector<STORAGE_POINT> got = query_all(smh, BASE_TIME, BASE_TIME + 10);
    ASSERT_EQ(got.size(), expected.size());

    for (size_t i = 0; i < expected.size(); i++)
        expect_point(got[i], expected[i]);

    dbengine_store_finalize(sch);
    dbengine_metrics_group_release(smg);
    dbengine_metric_release(smh);
    uuidmap_free(id);
}

TEST_F(CollectQueryTest, RetentionCoversWhatWasStored) {
    const UUIDMAP_ID id = make_metric_id();

    STORAGE_METRIC_HANDLE *smh = dbengine_metric_get_or_create_by_id(si_, id);
    ASSERT_NE(smh, nullptr);

    STORAGE_METRICS_GROUP *smg = dbengine_metrics_group_get();
    STORAGE_COLLECT_HANDLE *sch = dbengine_store_init(smh, 1, smg);
    ASSERT_NE(sch, nullptr);

    store_point(sch, BASE_TIME + 1, 1);
    store_point(sch, BASE_TIME + 2, 2);
    store_point(sch, BASE_TIME + 3, 3);
    dbengine_store_flush(sch);

    EXPECT_EQ(dbengine_oldest_time_s(smh), BASE_TIME + 1);
    EXPECT_EQ(dbengine_latest_time_s(smh), BASE_TIME + 3);

    time_t first = 0, last = 0;
    EXPECT_TRUE(dbengine_metric_retention_by_id(si_, id, &first, &last));
    EXPECT_EQ(first, BASE_TIME + 1);
    EXPECT_EQ(last, BASE_TIME + 3);

    dbengine_store_finalize(sch);
    dbengine_metrics_group_release(smg);
    dbengine_metric_release(smh);
    uuidmap_free(id);
}

TEST_F(CollectQueryTest, AMetricIsFoundAgainByIdAndByUuid) {
    nd_uuid_t uuid;
    uuid_generate(uuid);
    const UUIDMAP_ID id = uuidmap_create(uuid);

    STORAGE_METRIC_HANDLE *created = dbengine_metric_get_or_create_by_id(si_, id);
    ASSERT_NE(created, nullptr);

    STORAGE_METRIC_HANDLE *by_id = dbengine_metric_get_by_id(si_, id);
    EXPECT_NE(by_id, nullptr);

    STORAGE_METRIC_HANDLE *by_uuid = dbengine_metric_get_by_uuid(si_, &uuid);
    EXPECT_NE(by_uuid, nullptr);

    if (by_id)
        dbengine_metric_release(by_id);
    if (by_uuid)
        dbengine_metric_release(by_uuid);
    dbengine_metric_release(created);
    uuidmap_free(id);
}

TEST_F(CollectQueryTest, AMetricThatWasNeverCreatedIsNotFound) {
    nd_uuid_t uuid;
    uuid_generate(uuid);
    const UUIDMAP_ID id = uuidmap_create(uuid);

    EXPECT_EQ(dbengine_metric_get_by_id(si_, id), nullptr);
    EXPECT_EQ(dbengine_metric_get_by_uuid(si_, &uuid), nullptr);

    time_t first = 0, last = 0;
    EXPECT_FALSE(dbengine_metric_retention_by_id(si_, id, &first, &last));

    uuidmap_free(id);
}

TEST_F(CollectQueryTest, DuplicatingAMetricHandleKeepsItUsable) {
    const UUIDMAP_ID id = make_metric_id();

    STORAGE_METRIC_HANDLE *smh = dbengine_metric_get_or_create_by_id(si_, id);
    ASSERT_NE(smh, nullptr);

    STORAGE_METRIC_HANDLE *dup = dbengine_metric_dup(smh);
    ASSERT_NE(dup, nullptr);

    // Releasing one of the two leaves the other usable: they are references to one metric, not copies of it.
    dbengine_metric_release(smh);
    EXPECT_EQ(dbengine_latest_time_s(dup), 0) << "a metric with no data reported a latest time";

    dbengine_metric_release(dup);
    uuidmap_free(id);
}

TEST_F(CollectQueryTest, AQueryOutsideTheStoredRangeReturnsNothing) {
    const UUIDMAP_ID id = make_metric_id();

    STORAGE_METRIC_HANDLE *smh = dbengine_metric_get_or_create_by_id(si_, id);
    ASSERT_NE(smh, nullptr);

    STORAGE_METRICS_GROUP *smg = dbengine_metrics_group_get();
    STORAGE_COLLECT_HANDLE *sch = dbengine_store_init(smh, 1, smg);
    ASSERT_NE(sch, nullptr);

    store_point(sch, BASE_TIME + 1, 1);
    dbengine_store_flush(sch);

    const std::vector<STORAGE_POINT> got = query_all(smh, BASE_TIME + 1000, BASE_TIME + 2000);

    // The claim in the name, and the whole of it: a window past everything stored yields no points at all.
    //
    // An earlier version also looped over the result asserting each point was a gap, described as tolerating a
    // future engine that returned gaps instead of nothing. It could not do that: query_all() stops after 64 points
    // and fails, so a gap-filling engine would fail here long before the loop expressed any tolerance. Promising
    // something the code cannot deliver is worse than not promising it.
    EXPECT_TRUE(got.empty()) << "a query past the stored range returned " << got.size() << " points";

    dbengine_store_finalize(sch);
    dbengine_metrics_group_release(smg);
    dbengine_metric_release(smh);
    uuidmap_free(id);
}

TEST_F(CollectQueryTest, TheTierReportsWhatItHolds) {
    const UUIDMAP_ID id = make_metric_id();

    // Before the create, not after: a count sampled after the metric exists already includes it, and the
    // comparison below would then only be asking whether a counter went backwards.
    const uint64_t metrics_before = dbengine_metrics(si_);

    STORAGE_METRIC_HANDLE *smh = dbengine_metric_get_or_create_by_id(si_, id);
    ASSERT_NE(smh, nullptr);

    STORAGE_METRICS_GROUP *smg = dbengine_metrics_group_get();
    STORAGE_COLLECT_HANDLE *sch = dbengine_store_init(smh, 1, smg);
    ASSERT_NE(sch, nullptr);

    store_point(sch, BASE_TIME + 1, 1);
    store_point(sch, BASE_TIME + 2, 2);
    dbengine_store_flush(sch);

    EXPECT_EQ(dbengine_metrics(si_), metrics_before + 1) << "the tier did not count the metric it was given";
    // Exactly one, not merely more than none. Two points one second apart cover one second, and the tier counts
    // the span its pages cover rather than the points in them - so a counter stuck at any positive number, which
    // "greater than zero" would have accepted, fails here.
    EXPECT_EQ(dbengine_samples(si_), 1u) << "the tier did not count the second the two points cover";

    dbengine_store_finalize(sch);
    dbengine_metrics_group_release(smg);
    dbengine_metric_release(smh);
    uuidmap_free(id);
}

TEST_F(CollectQueryTest, AMetricsGroupIsBoundToNoTier) {
    // It holds nothing of a chart and nothing of a tier: the caller makes one, hands it to each of the chart's
    // metrics, and releases it when the chart goes.
    STORAGE_METRICS_GROUP *first = dbengine_metrics_group_get();
    STORAGE_METRICS_GROUP *second = dbengine_metrics_group_get();

    EXPECT_NE(first, nullptr);
    EXPECT_NE(second, nullptr);
    EXPECT_NE(first, second) << "two metrics groups are the same object, so charts would share page alignment";

    dbengine_metrics_group_release(first);
    dbengine_metrics_group_release(second);
}

// Disabled because it does not fail, it ends the process. Deleting a metric's retention leaves its dirty page in
// the cache holding the metric's address; the flush that the tier exit performs then reads that freed metric and
// reaches fatal() inside uuidmap_dup(), and fatal() takes the whole binary with it, so an enabled case here would
// cost every result after it.
//
// That is a defect of the engine, not of this suite, and the same fatal is reached in the field from the engine's
// periodic flush. Fixing it is not this change's work - this change adds tests - so the case stays here, disabled,
// as the reproducer, and googletest reports it as disabled on every run rather than letting it be forgotten.
// Enable it with the fix; the retention assertions below already pass.
TEST_F(CollectQueryTest, DISABLED_DeletingAMetricsRetentionLeavesItWithNone) {
    const UUIDMAP_ID id = make_metric_id();

    STORAGE_METRIC_HANDLE *smh = dbengine_metric_get_or_create_by_id(si_, id);
    ASSERT_NE(smh, nullptr);

    STORAGE_METRICS_GROUP *smg = dbengine_metrics_group_get();
    STORAGE_COLLECT_HANDLE *sch = dbengine_store_init(smh, 1, smg);
    ASSERT_NE(sch, nullptr);

    store_point(sch, BASE_TIME + 1, 1);
    dbengine_store_flush(sch);
    dbengine_store_finalize(sch);
    dbengine_metrics_group_release(smg);
    dbengine_metric_release(smh);

    time_t first = 0, last = 0;
    ASSERT_TRUE(dbengine_metric_retention_by_id(si_, id, &first, &last));

    dbengine_metric_retention_delete_by_id(si_, id);

    first = last = 0;
    EXPECT_FALSE(dbengine_metric_retention_by_id(si_, id, &first, &last))
        << "the metric still has retention after it was deleted";

    uuidmap_free(id);
}

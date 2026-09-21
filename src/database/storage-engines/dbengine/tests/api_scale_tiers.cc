// SPDX-License-Identifier: GPL-3.0-or-later

#include "support.h"

#include <vector>

// More than one tier on one engine, through the public surface. Every other case brings up tier 0 alone, so the
// page type the higher tiers must use, and what it keeps of a point, ran in nothing CI executed.
//
// A higher tier's page holds aggregates: for each point, the minimum, maximum and sum of the tier-0 points it
// stands for, how many there were, and how many of those were anomalous. The tier stores them as 32-bit floats,
// so the values here are ones that format holds exactly. The tier does not keep the flags it is given: on the
// way out it derives them from the anomaly count (none when any point was anomalous, "not anomalous" otherwise),
// where tier 0 hands back the flags it was given. Both are asserted as they are, the asymmetry included.
//
// Filling the higher tiers from tier 0 (the aggregation, and the backfill of a tier that came up late) is the
// daemon's work, not the engine's: the engine stores what it is handed, per tier, and that is what is tested.

namespace {

class ScaleTiersTest : public EngineFixture {
protected:
    // Tiers 1 and 2 get directories of their own: two tiers on one directory corrupt it. They are members rather
    // than locals of the case so that they outlive the teardown that exits the tiers.
    struct dbengine_tier_config tier_config(size_t tier) override {
        struct dbengine_tier_config tc = EngineFixture::tier_config(tier);
        if (tier == 0)
            return tc;

        tc.dbfiles_path = tier == 1 ? scratch1_.c_str() : scratch2_.c_str();
        tc.page_type = DBENGINE_PAGE_TYPE_ARRAY_TIER1;
        tc.grouping = tier == 1 ? 60 : 3600;
        return tc;
    }

    // Brings a higher tier up on the running engine and hands back its storage instance; nullptr when it did not
    // come up. The readiness wait is only made on a tier whose init succeeded: on one that failed there is no load
    // to wait for and the wait would never return, turning a failed case into a hung binary.
    STORAGE_INSTANCE *bring_up_tier(size_t tier) {
        const Scratch &scratch = tier == 1 ? scratch1_ : scratch2_;
        EXPECT_TRUE(scratch.valid()) << "could not make a scratch directory";
        if (!scratch.valid())
            return nullptr;

        const struct dbengine_tier_config tc = tier_config(tier);
        const int rc = dbengine_tier_init(engine_, &tc);
        EXPECT_EQ(rc, 0) << "tier " << tier << " did not come up";
        if (rc != 0)
            return nullptr;

        DBENGINE_TIER *t = dbengine_tier(engine_, tier);
        dbengine_readiness_wait(t);
        return reinterpret_cast<STORAGE_INSTANCE *>(t);
    }

    Scratch scratch1_;
    Scratch scratch2_;
};

// An aggregate point as a higher tier is handed it, and as it must come back.
struct Aggregate {
    time_t end_time_s;
    NETDATA_DOUBLE sum, min, max;
    uint16_t count, anomaly_count;
};

void expect_storage_point(const STORAGE_POINT &got, const STORAGE_POINT &expected) {
    SCOPED_TRACE(static_cast<long long>(expected.end_time_s));

    EXPECT_EQ(got.start_time_s, expected.start_time_s);
    EXPECT_EQ(got.end_time_s, expected.end_time_s);
    EXPECT_EQ(got.min, expected.min);
    EXPECT_EQ(got.max, expected.max);
    EXPECT_EQ(got.sum, expected.sum);
    EXPECT_EQ(got.count, expected.count);
    EXPECT_EQ(got.anomaly_count, expected.anomaly_count);
    EXPECT_EQ(got.flags, expected.flags);
}

// Stores the aggregates into a metric of the given tier and reads them back as one query, comparing every field.
void store_and_read_aggregates(STORAGE_INSTANCE *si, uint32_t update_every, const std::vector<Aggregate> &points) {
    const UUIDMAP_ID id = make_metric_id();
    STORAGE_METRIC_HANDLE *smh = dbengine_metric_get_or_create_by_id(si, id);
    ASSERT_NE(smh, nullptr);

    STORAGE_METRICS_GROUP *smg = dbengine_metrics_group_get();
    STORAGE_COLLECT_HANDLE *sch = dbengine_store_init(smh, update_every, smg);
    ASSERT_NE(sch, nullptr);

    for (const Aggregate &a : points)
        dbengine_store_next(sch, static_cast<usec_t>(a.end_time_s) * USEC_PER_SEC, a.sum, a.min, a.max, a.count,
                            a.anomaly_count, SN_DEFAULT_FLAGS);
    dbengine_store_flush(sch);

    const std::vector<STORAGE_POINT> got = query_all(smh, points.front().end_time_s, points.back().end_time_s);
    ASSERT_EQ(got.size(), points.size());

    for (size_t i = 0; i < points.size(); i++) {
        const Aggregate &a = points[i];
        STORAGE_POINT expected = {};
        expected.start_time_s = a.end_time_s - update_every;
        expected.end_time_s = a.end_time_s;
        expected.min = a.min;
        expected.max = a.max;
        expected.sum = a.sum;
        expected.count = a.count;
        expected.anomaly_count = a.anomaly_count;
        // Derived, not round-tripped: what the tier's read arm makes of the anomaly count.
        expected.flags = a.anomaly_count ? SN_FLAG_NONE : SN_FLAG_NOT_ANOMALOUS;
        expect_storage_point(got[i], expected);
    }

    dbengine_store_finalize(sch);
    dbengine_metrics_group_release(smg);
    dbengine_metric_release(smh);
    uuidmap_free(id);
}

// A multiple of both groupings, so the aggregate points sit on their tier's grid.
const time_t TIER_BASE = 200001600;

} // namespace

TEST_F(ScaleTiersTest, ThreeTiersHoldWhatEachIsGiven) {
    STORAGE_INSTANCE *tier1 = bring_up_tier(1);
    ASSERT_NE(tier1, nullptr);
    STORAGE_INSTANCE *tier2 = bring_up_tier(2);
    ASSERT_NE(tier2, nullptr);

    // Tier 0 as every other case uses it: samples, one per second, flags round-tripped.
    {
        const UUIDMAP_ID id = make_metric_id();
        STORAGE_METRIC_HANDLE *smh = dbengine_metric_get_or_create_by_id(si_, id);
        ASSERT_NE(smh, nullptr);
        STORAGE_METRICS_GROUP *smg = dbengine_metrics_group_get();
        STORAGE_COLLECT_HANDLE *sch = dbengine_store_init(smh, 1, smg);
        ASSERT_NE(sch, nullptr);

        store_point(sch, TIER_BASE + 1, 10);
        store_point(sch, TIER_BASE + 2, 20);
        dbengine_store_flush(sch);

        const std::vector<STORAGE_POINT> got = query_all(smh, TIER_BASE + 1, TIER_BASE + 2);
        ASSERT_EQ(got.size(), 2u);
        expect_point(got[0], {TIER_BASE, TIER_BASE + 1, 10});
        expect_point(got[1], {TIER_BASE + 1, TIER_BASE + 2, 20});

        dbengine_store_finalize(sch);
        dbengine_metrics_group_release(smg);
        dbengine_metric_release(smh);
        uuidmap_free(id);
    }

    // Tier 1: one point per minute, each standing for sixty samples; the second had anomalies, the first none.
    // min, max and sum differ so that a tier storing one of them for all three would be caught.
    store_and_read_aggregates(tier1, 60, {
        {TIER_BASE + 60, 120.0, 1.5, 2.5, 60, 0},
        {TIER_BASE + 120, 90.0, 0.5, 3.0, 60, 7},
    });

    // Tier 2: one point per hour, each standing for sixty tier-1 points' worth of samples.
    store_and_read_aggregates(tier2, 3600, {
        {TIER_BASE + 3600, 7200.0, 1.5, 2.5, 3600, 11},
        {TIER_BASE + 7200, 5400.0, 0.25, 4.0, 3600, 0},
    });
}

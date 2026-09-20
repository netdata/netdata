// SPDX-License-Identifier: GPL-3.0-or-later

#include "support.h"

extern "C" {
#include "database/storage-engines/dbengine/page.h"
}

#include <vector>

// Page descriptors, PGD: the thing a collector appends points to and the thing an extent is built from. Fresh cases
// rather than the ones in page_test.cc, which is left exactly as it is.
//
// The page allocator layer these use is process-wide and one-shot - the first caller fixes its partitions and size
// classes and every later one is ignored - so this fixture initialises it from the same configuration every other
// test in this binary uses. Calling it when it is already up is what it is built for, so these cases do not care
// whether an engine ran first.

namespace {

class PgdTest : public ::testing::Test {
protected:
    void SetUp() override {
        const struct dbengine_config cfg = netdata_test_config();
        pgd_init_arals(&cfg.allocator);
    }

    static void append(PGD *pg, uint32_t slot, NETDATA_DOUBLE value) {
        pgd_append_point(pg, static_cast<usec_t>(1000 + slot) * USEC_PER_SEC, value, value, value, 1, 0,
                         SN_DEFAULT_FLAGS, slot);
    }

    // constexpr, not const: the assertion macros take it by reference, which needs a definition, and C++17 gives
    // a constexpr static member one implicitly.
    static constexpr uint8_t TYPE = DBENGINE_PAGE_TYPE_GORILLA_32BIT;
};

} // namespace

TEST_F(PgdTest, ANewPageIsEmpty) {
    PGD *pg = pgd_create(TYPE, 16);
    ASSERT_NE(pg, nullptr);

    EXPECT_TRUE(pgd_is_empty(pg));
    EXPECT_EQ(pgd_slots_used(pg), 0u);
    EXPECT_EQ(pgd_type(pg), TYPE);

    pgd_free(pg);
}

TEST_F(PgdTest, AppendingPointsFillsSlots) {
    PGD *pg = pgd_create(TYPE, 16);
    ASSERT_NE(pg, nullptr);

    for (uint32_t slot = 0; slot < 8; slot++) {
        append(pg, slot, slot + 1);
        EXPECT_EQ(pgd_slots_used(pg), slot + 1) << "slot " << slot << " did not count";
    }

    EXPECT_FALSE(pgd_is_empty(pg));

    pgd_free(pg);
}

TEST_F(PgdTest, CapacityIsAtLeastWhatWasAskedFor) {
    // From two upwards: a page of a single slot is rejected as a programming error, for every page type, and the
    // rejection is an internal_fatal - so it ends the process, and only in builds that have internal checks on.
    // There is no case for it here because reaching it would take a death test, and this binary must not fork with
    // a libuv pool alive.
    for (uint32_t slots : {uint32_t(2), uint32_t(16), uint32_t(256), uint32_t(1024)}) {
        SCOPED_TRACE(slots);

        PGD *pg = pgd_create(TYPE, slots);
        ASSERT_NE(pg, nullptr);

        // A page that could hold fewer points than the collector was promised would drop data.
        EXPECT_GE(pgd_capacity(pg), slots);

        pgd_free(pg);
    }
}

TEST_F(PgdTest, PointsComeBackThroughACursor) {
    PGD *pg = pgd_create(TYPE, 16);
    ASSERT_NE(pg, nullptr);

    const std::vector<NETDATA_DOUBLE> values = {1, 2, 4, 8, 16, 32};
    for (uint32_t slot = 0; slot < values.size(); slot++)
        append(pg, slot, values[slot]);

    PGDC cursor = {};
    pgdc_reset(&cursor, pg, 0);

    for (uint32_t slot = 0; slot < values.size(); slot++) {
        SCOPED_TRACE(slot);

        STORAGE_POINT sp = {};
        ASSERT_TRUE(pgdc_get_next_point(&cursor, slot, &sp)) << "the cursor stopped at slot " << slot;
        EXPECT_DOUBLE_EQ(sp.sum, values[slot]);
        EXPECT_EQ(sp.count, 1u);
        EXPECT_EQ(sp.anomaly_count, 0u);
    }

    // Past the end the cursor has nothing left to give.
    STORAGE_POINT sp = {};
    EXPECT_FALSE(pgdc_get_next_point(&cursor, static_cast<uint32_t>(values.size()), &sp));

    pgd_free(pg);
}

TEST_F(PgdTest, AnEmptyPageHasNoPointsToGive) {
    PGD *pg = pgd_create(TYPE, 16);
    ASSERT_NE(pg, nullptr);

    PGDC cursor = {};
    pgdc_reset(&cursor, pg, 0);

    STORAGE_POINT sp = {};
    EXPECT_FALSE(pgdc_get_next_point(&cursor, 0, &sp));

    pgd_free(pg);
}

TEST_F(PgdTest, MemoryFootprintGrowsWithTheDataHeld) {
    PGD *pg = pgd_create(TYPE, 1024);
    ASSERT_NE(pg, nullptr);

    const uint32_t empty_footprint = pgd_memory_footprint(pg);
    EXPECT_GT(empty_footprint, 0u) << "a page occupies no memory at all";

    for (uint32_t slot = 0; slot < 512; slot++)
        append(pg, slot, slot % 7);

    EXPECT_GE(pgd_memory_footprint(pg), empty_footprint)
        << "the footprint shrank while the page was being filled";

    pgd_free(pg);
}

TEST_F(PgdTest, ItSurvivesBeingWrittenToAnExtentAndReadBack) {
    PGD *pg = pgd_create(TYPE, 16);
    ASSERT_NE(pg, nullptr);

    const std::vector<NETDATA_DOUBLE> values = {1, 2, 4, 8};
    for (uint32_t slot = 0; slot < values.size(); slot++)
        append(pg, slot, values[slot]);

    const uint32_t disk_size = pgd_disk_footprint(pg);
    ASSERT_GT(disk_size, 0u);

    std::vector<uint8_t> extent(disk_size);
    pgd_copy_to_extent(pg, extent.data(), disk_size);

    // This is the path a page takes to disk and back: what a query reads is built from the bytes, not from the page
    // the collector had.
    PGD *restored = pgd_create_from_disk_data(TYPE, extent.data(), disk_size);
    ASSERT_NE(restored, nullptr);

    PGDC cursor = {};
    pgdc_reset(&cursor, restored, 0);

    for (uint32_t slot = 0; slot < values.size(); slot++) {
        SCOPED_TRACE(slot);

        STORAGE_POINT sp = {};
        ASSERT_TRUE(pgdc_get_next_point(&cursor, slot, &sp));
        EXPECT_DOUBLE_EQ(sp.sum, values[slot]) << "the value did not survive the trip through an extent";
    }

    pgd_free(restored);
    pgd_free(pg);
}

TEST_F(PgdTest, TheAllocatorLayerKeepsWhatItWasFirstGiven) {
    // Documented behaviour, and the reason this binary uses one configuration throughout: a second call with
    // different settings is ignored rather than honoured, so nothing here can be made to see a different layer by
    // running in a different order.
    struct dbengine_config other = netdata_test_config();
    other.allocator.partitions = netdata_test_config().allocator.partitions + 3;

    const uint32_t fingerprint_before = static_cast<uint32_t>(dbengine_allocator_layer_fingerprint());
    pgd_init_arals(&other.allocator);
    const uint32_t fingerprint_after = static_cast<uint32_t>(dbengine_allocator_layer_fingerprint());

    EXPECT_EQ(fingerprint_before, fingerprint_after)
        << "a second allocator configuration changed the layer the first one built";
}

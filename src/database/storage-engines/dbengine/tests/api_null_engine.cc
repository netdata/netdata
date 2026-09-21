// SPDX-License-Identifier: GPL-3.0-or-later

#include "support.h"

#include <cstring>

// An embedder that never made an engine still calls the engine's getters and verbs, with NULL. dbengine-api.h
// states what must happen: NULL is an engine, or a tier, with nothing in it - verbs do nothing, getters report
// false or zeros. Nothing here creates an engine, so these cases hold whatever else the binary runs.
//
// dbengine_tier_init() is deliberately absent: it is fatal without an engine, by contract, and a fatal takes the
// whole process with it. dbengine_shutdown() is absent for the opposite reason - it does nothing and has nothing to
// observe.

namespace {

// A getter that forgets to write its answer would leave the caller's buffer as it was, and a buffer that started at
// zero would let that pass. These start at 0xff, so silence is a failure.
template <typename T>
T poisoned() {
    T value;
    memset(&value, 0xff, sizeof(value));
    return value;
}

template <typename T>
bool zeroed(const T &value) {
    T zero;
    memset(&zero, 0, sizeof(zero));
    return memcmp(&value, &zero, sizeof(value)) == 0;
}

} // namespace

TEST(NullEngine, CacheStatsAreRefusedAndZeroed) {
    for (DBENGINE_CACHE which = DBENGINE_CACHE_MAIN; which <= DBENGINE_CACHE_EXTENT;
         which = static_cast<DBENGINE_CACHE>(which + 1)) {
        SCOPED_TRACE(static_cast<int>(which));

        auto stats = poisoned<struct dbengine_cache_stats>();
        EXPECT_FALSE(dbengine_get_cache_stats(nullptr, which, &stats));
        EXPECT_TRUE(zeroed(stats));
    }
}

TEST(NullEngine, HasNoPagesPendingFlush) {
    EXPECT_EQ(dbengine_pages_pending_flush(nullptr), 0u);
}

TEST(NullEngine, MetricsRegistryStatsAreRefusedAndZeroed) {
    auto stats = poisoned<struct dbengine_metrics_registry_stats>();
    EXPECT_FALSE(dbengine_get_metrics_registry_stats(nullptr, &stats));
    EXPECT_TRUE(zeroed(stats));
}

TEST(NullEngine, CacheEfficiencyStatsAreZeroed) {
    EXPECT_TRUE(zeroed(dbengine_get_cache_efficiency_stats(nullptr)));
}

TEST(NullEngine, OwnMemorySlotsAreUnset) {
    const struct dbengine_buffer_sizes sizes = dbengine_get_memory_sizes(nullptr);

    // Only the slots an engine owns. The rest of the table is process-wide and may legitimately hold bytes from
    // whatever else the process has done.
    const DBENGINE_MEM engine_slots[] = {DBENGINE_MEM_OPCODES, DBENGINE_MEM_HANDLES, DBENGINE_MEM_DESCRIPTORS,
                                         DBENGINE_MEM_WORKERS, DBENGINE_MEM_XT_IO};

    for (DBENGINE_MEM slot : engine_slots) {
        SCOPED_TRACE(static_cast<int>(slot));
        EXPECT_EQ(sizes.as[slot], nullptr);
    }

    EXPECT_EQ(sizes.wal, 0u);

    // The daemon runs every allocator statistics reader over every slot without first checking which are populated,
    // so doing it here must not crash. There is deliberately no assertion on what comes back: those readers return
    // zero for an empty slot by construction, so comparing against zero would be a check that cannot fail - and a
    // case that cannot fail is what the rest of this review removed. The engine's side of it is the loop above.
    for (size_t i = 0; i < DBENGINE_MEM_MAX; i++) {
        aral_structures_bytes_from_stats(sizes.as[i]);
        aral_free_bytes_from_stats(sizes.as[i]);
        aral_used_bytes_from_stats(sizes.as[i]);
        aral_padding_bytes_from_stats(sizes.as[i]);
    }
}

TEST(NullEngine, HasNoTiers) {
    EXPECT_EQ(dbengine_tier(nullptr, 0), nullptr);
}

TEST(NullEngine, HasNoFileDescriptorBudget) {
    EXPECT_EQ(dbengine_max_reserved_file_descriptors(nullptr), 0u);
}

TEST(NullEngine, TakesNoWork) {
    struct dbengine_work_request request = {};

    EXPECT_FALSE(dbengine_work_available(nullptr));
    EXPECT_FALSE(dbengine_enq_work(nullptr, &request));
}

TEST(NullEngine, DestroyReportsNoReferencedMetrics) {
    EXPECT_EQ(dbengine_destroy(nullptr), 0u);
}

TEST(NullEngine, PreloadReleaseReturns) {
    dbengine_preload_release(nullptr);
}

// The tier readouts, reached through dbengine_tier() above: the daemon's shutdown asks every tier slot whether it is
// up, engine or no engine.

TEST(NullTier, IsNotActive) {
    EXPECT_FALSE(dbengine_tier_is_active(nullptr));
}

TEST(NullTier, HasNoRetentionLimit) {
    EXPECT_EQ(dbengine_max_retention_s(nullptr), 0);
}

TEST(NullTier, HasNoCollectorsRunning) {
    EXPECT_EQ(dbengine_collectors_running(nullptr), 0u);
}

TEST(NullTier, ReportsNoDiskSpace) {
    EXPECT_EQ(dbengine_get_used_disk_space(nullptr), 0u);
    EXPECT_EQ(dbengine_get_directory_free_bytes_space(nullptr), 0u);
}

TEST(NullTier, SizeStatsAreZeroed) {
    EXPECT_TRUE(zeroed(dbengine_get_size_stats(nullptr)));
}

TEST(NullTier, FlushVerbsReturn) {
    // The two fire-and-forget verbs have nothing to observe but their return; the waiting one reports that it
    // queued nothing.
    dbengine_flush_dirty(nullptr);
    dbengine_flush_all(nullptr);
    EXPECT_FALSE(dbengine_flush_all_wait(nullptr));
}

TEST(NullTier, ReadinessWaitReturns) {
    dbengine_readiness_wait(nullptr);
}

TEST(NullTier, ReportsNoQuotaSpaceMetricsSamplesOrFirstTime) {
    EXPECT_EQ(dbengine_disk_space_max(nullptr), 0u);
    EXPECT_EQ(dbengine_disk_space_used(nullptr), 0u);
    EXPECT_EQ(dbengine_metrics(nullptr), 0u);
    EXPECT_EQ(dbengine_samples(nullptr), 0u);
    EXPECT_EQ(dbengine_global_first_time_s(nullptr), 0);
}

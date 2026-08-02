#include "page.h"
#include "page_test.h"

#ifdef HAVE_GTEST

#include <gtest/gtest.h>
#include <limits>
#include <random>

bool operator==(const STORAGE_POINT lhs, const STORAGE_POINT rhs) {
    if (lhs.min != rhs.min)
        return false;

    if (lhs.max != rhs.max)
        return false;

    if (lhs.sum != rhs.sum)
        return false;

    if (lhs.start_time_s != rhs.start_time_s)
        return false;

    if (lhs.end_time_s != rhs.end_time_s)
        return false;

    if (lhs.count != rhs.count)
        return false;

    if (lhs.gap_count != rhs.gap_count)
        return false;

    if (lhs.anomaly_count != rhs.anomaly_count)
        return false;

    if (lhs.flags != rhs.flags)
        return false;

    return true;
}

// TODO: use value-parameterized tests
// http://google.github.io/googletest/advanced.html#value-parameterized-tests
static uint8_t page_type = PAGE_GORILLA_METRICS;

static STORAGE_POINT numeric_point(
    time_t start_time_s,
    time_t end_time_s,
    NETDATA_DOUBLE min,
    NETDATA_DOUBLE max,
    NETDATA_DOUBLE sum,
    uint32_t count,
    uint32_t gap_count,
    uint32_t anomaly_count,
    SN_FLAGS flags = SN_FLAG_NONE) {
    STORAGE_POINT sp = STORAGE_POINT_UNSET;
    sp.min = min;
    sp.max = max;
    sp.sum = sum;
    sp.start_time_s = start_time_s;
    sp.end_time_s = end_time_s;
    sp.count = count;
    sp.anomaly_count = anomaly_count;
    sp.flags = flags;
    sp.gap_count = gap_count;
    return sp;
}

TEST(StoragePoint, StateContractAndSize) {
#if UINTPTR_MAX == UINT64_MAX
    EXPECT_EQ(sizeof(STORAGE_POINT), 56u);
#endif

    STORAGE_POINT sp = STORAGE_POINT_UNSET;
    EXPECT_TRUE(storage_point_is_unset(sp));
    EXPECT_FALSE(storage_point_has_value(sp));
    EXPECT_FALSE(storage_point_is_gap(sp));
    EXPECT_FALSE(storage_point_is_partial(sp));
    EXPECT_EQ(storage_point_slots(sp), 0u);

    storage_point_empty(sp, 10, 20);
    EXPECT_FALSE(storage_point_is_unset(sp));
    EXPECT_FALSE(storage_point_has_value(sp));
    EXPECT_TRUE(storage_point_is_gap(sp));
    EXPECT_FALSE(storage_point_is_partial(sp));
    EXPECT_EQ(sp.count, 0u);
    EXPECT_EQ(sp.gap_count, 1u);
    EXPECT_EQ(storage_point_slots(sp), 1u);

    sp = numeric_point(10, 20, 1, 2, 3, 2, 0, 1);
    EXPECT_TRUE(storage_point_has_value(sp));
    EXPECT_TRUE(storage_point_is_complete(sp));
    EXPECT_FALSE(storage_point_is_partial(sp));
    EXPECT_EQ(storage_point_slots(sp), 2u);

    sp.gap_count = 3;
    EXPECT_TRUE(storage_point_has_value(sp));
    EXPECT_FALSE(storage_point_is_complete(sp));
    EXPECT_TRUE(storage_point_is_partial(sp));
    EXPECT_EQ(storage_point_slots(sp), 5u);
}

TEST(StoragePoint, MergePreservesGapEvidenceInEitherOrder) {
    STORAGE_POINT gap;
    storage_point_empty(gap, 0, 10);
    STORAGE_POINT numeric = numeric_point(10, 20, 2, 5, 7, 2, 1, 1, SN_FLAG_RESET);

    STORAGE_POINT gap_first = gap;
    storage_point_merge_to(gap_first, numeric);
    EXPECT_EQ(gap_first.start_time_s, 0);
    EXPECT_EQ(gap_first.end_time_s, 20);
    EXPECT_EQ(gap_first.min, 2);
    EXPECT_EQ(gap_first.max, 5);
    EXPECT_EQ(gap_first.sum, 7);
    EXPECT_EQ(gap_first.count, 2u);
    EXPECT_EQ(gap_first.gap_count, 2u);
    EXPECT_EQ(gap_first.anomaly_count, 1u);
    EXPECT_TRUE(gap_first.flags & SN_FLAG_RESET);
    EXPECT_TRUE(storage_point_is_partial(gap_first));

    STORAGE_POINT numeric_first = numeric;
    storage_point_merge_to(numeric_first, gap);
    EXPECT_EQ(numeric_first, gap_first);

    STORAGE_POINT second_gap;
    storage_point_empty(second_gap, 20, 30);
    storage_point_merge_to(gap, second_gap);
    EXPECT_TRUE(storage_point_is_gap(gap));
    EXPECT_EQ(gap.start_time_s, 0);
    EXPECT_EQ(gap.end_time_s, 30);
    EXPECT_EQ(gap.gap_count, 2u);
}

TEST(StoragePoint, AddPreservesGapEvidence) {
    STORAGE_POINT dst = numeric_point(0, 10, 2, 5, 7, 2, 1, 1);
    STORAGE_POINT src = numeric_point(10, 20, 3, 7, 11, 3, 2, 2);
    storage_point_add_to(dst, src);

    EXPECT_EQ(dst.start_time_s, 0);
    EXPECT_EQ(dst.end_time_s, 20);
    EXPECT_EQ(dst.min, 5);
    EXPECT_EQ(dst.max, 12);
    EXPECT_EQ(dst.sum, 18);
    EXPECT_EQ(dst.count, 5u);
    EXPECT_EQ(dst.gap_count, 3u);
    EXPECT_EQ(dst.anomaly_count, 3u);
    EXPECT_TRUE(storage_point_is_partial(dst));

    STORAGE_POINT gap;
    storage_point_empty(gap, 20, 30);
    storage_point_add_to(dst, gap);
    EXPECT_EQ(dst.sum, 18);
    EXPECT_EQ(dst.count, 5u);
    EXPECT_EQ(dst.gap_count, 4u);
}

TEST(StoragePoint, LegacyTierSlotNormalization) {
    STORAGE_POINT partial = numeric_point(0, 60, 10, 10, 400, 40, 0, 0);
    storage_point_normalize_legacy_slots(partial, 60);
    EXPECT_EQ(partial.count, 40u);
    EXPECT_EQ(partial.gap_count, 20u);
    EXPECT_TRUE(storage_point_is_partial(partial));

    STORAGE_POINT overfull = numeric_point(0, 60, 10, 10, 800, 80, 0, 0);
    storage_point_normalize_legacy_slots(overfull, 60);
    EXPECT_EQ(overfull.count, 80u);
    EXPECT_EQ(overfull.gap_count, 0u);
    EXPECT_TRUE(storage_point_is_complete(overfull));

    STORAGE_POINT gap;
    storage_point_empty(gap, 0, 60);
    storage_point_normalize_legacy_slots(gap, 60);
    EXPECT_EQ(gap.count, 0u);
    EXPECT_EQ(gap.gap_count, 60u);
    EXPECT_TRUE(storage_point_is_gap(gap));
}

TEST(PGD, DecodesTier0PointStatesExactly) {
    PGD *pg = pgd_create(RRDENG_PAGE_TYPE_GORILLA_32BIT, 2);
    pgd_append_point(pg, 1, 42, 42, 42, 1, 0, SN_FLAG_NOT_ANOMALOUS, 0);
    pgd_append_point(pg, 2, NAN, NAN, NAN, 0, 0, SN_FLAG_NONE, 1);

    PGDC cursor;
    pgdc_reset(&cursor, pg, 0, 1);

    STORAGE_POINT sp = STORAGE_POINT_UNSET;
    sp.start_time_s = 0;
    sp.end_time_s = 1;
    EXPECT_TRUE(pgdc_get_next_point(&cursor, 0, &sp));
    EXPECT_TRUE(storage_point_is_complete(sp));
    EXPECT_EQ(sp.count, 1u);
    EXPECT_EQ(sp.gap_count, 0u);

    sp.start_time_s = 1;
    sp.end_time_s = 2;
    EXPECT_TRUE(pgdc_get_next_point(&cursor, 1, &sp));
    EXPECT_TRUE(storage_point_is_gap(sp));
    EXPECT_EQ(sp.count, 0u);
    EXPECT_EQ(sp.gap_count, 1u);

    pgd_free(pg);
}

TEST(PGD, ReconstructsLegacyTierGapCount) {
    PGD *pg = pgd_create(RRDENG_PAGE_TYPE_ARRAY_TIER1, 2);
    pgd_append_point(pg, 60, 400, 10, 10, 40, 0, SN_FLAG_NOT_ANOMALOUS, 0);
    pgd_append_point(pg, 120, NAN, NAN, NAN, 0, 0, SN_FLAG_NONE, 1);

    PGDC cursor;
    pgdc_reset(&cursor, pg, 0, 60);

    STORAGE_POINT sp = STORAGE_POINT_UNSET;
    sp.start_time_s = 0;
    sp.end_time_s = 60;
    EXPECT_TRUE(pgdc_get_next_point(&cursor, 0, &sp));
    EXPECT_TRUE(storage_point_is_partial(sp));
    EXPECT_EQ(sp.count, 40u);
    EXPECT_EQ(sp.gap_count, 20u);

    sp.start_time_s = 60;
    sp.end_time_s = 120;
    EXPECT_TRUE(pgdc_get_next_point(&cursor, 1, &sp));
    EXPECT_TRUE(storage_point_is_gap(sp));
    EXPECT_EQ(sp.count, 0u);
    EXPECT_EQ(sp.gap_count, 60u);

    pgd_free(pg);
}

static size_t slots_for_page(size_t n) {
    switch (page_type) {
        case PAGE_METRICS:
            return 1024;
        case PAGE_GORILLA_METRICS:
            return n;
        default:
            fatal("Slots requested for unsupported page: %uc", page_type);
    }
}

TEST(PGD, EmptyOrNull) {
    PGD *pg = NULL;

    PGDC cursor;
    STORAGE_POINT sp;

    EXPECT_TRUE(pgd_is_empty(pg));
    EXPECT_EQ(pgd_slots_used(pg), 0);
    EXPECT_EQ(pgd_memory_footprint(pg), 0);
    EXPECT_EQ(pgd_disk_footprint(pg), 0);

    pgdc_reset(&cursor, pg, 0, 1);
    EXPECT_FALSE(pgdc_get_next_point(&cursor, 0, &sp));

    pgd_free(pg);

    pg = PGD_EMPTY;

    EXPECT_TRUE(pgd_is_empty(pg));
    EXPECT_EQ(pgd_slots_used(pg), 0);
    EXPECT_EQ(pgd_memory_footprint(pg), 0);
    EXPECT_EQ(pgd_disk_footprint(pg), 0);
    EXPECT_FALSE(pgdc_get_next_point(&cursor, 0, &sp));

    pgdc_reset(&cursor, pg, 0, 1);
    EXPECT_FALSE(pgdc_get_next_point(&cursor, 0, &sp));

    pgd_free(pg);
}

TEST(PGD, Create) {
    size_t slots = slots_for_page(1024 * 1024);
    PGD *pg = pgd_create(page_type, slots);

    EXPECT_EQ(pgd_type(pg), page_type);
    EXPECT_TRUE(pgd_is_empty(pg));
    EXPECT_EQ(pgd_slots_used(pg), 0);

    for (size_t i = 0; i != slots; i++) {
        pgd_append_point(pg, i, i, 0, 0, 1, 1, SN_DEFAULT_FLAGS, i);
        EXPECT_FALSE(pgd_is_empty(pg));
    }
    EXPECT_EQ(pgd_slots_used(pg), slots);

    EXPECT_DEATH(
        pgd_append_point(pg, slots, slots, 0, 0, 1, 1, SN_DEFAULT_FLAGS, slots),
        ".*"
    );

    pgd_free(pg);
}

TEST(PGD, CursorFullPage) {
    size_t slots = slots_for_page(1024 * 1024);
    PGD *pg = pgd_create(page_type, slots);

    for (size_t slot = 0; slot != slots; slot++)
        pgd_append_point(pg, slot, slot, 0, 0, 1, 1, SN_DEFAULT_FLAGS, slot);

    for (size_t i = 0; i != 2; i++) {
        PGDC cursor;
        pgdc_reset(&cursor, pg, 0, 1);

        STORAGE_POINT sp;
        for (size_t slot = 0; slot != slots; slot++) {
            EXPECT_TRUE(pgdc_get_next_point(&cursor, slot, &sp));

            EXPECT_EQ(slot, static_cast<size_t>(sp.min));
            EXPECT_EQ(sp.min, sp.max);
            EXPECT_EQ(sp.min, sp.sum);
            EXPECT_EQ(sp.count, 1);
            EXPECT_EQ(sp.anomaly_count, 0);
        }

        EXPECT_FALSE(pgdc_get_next_point(&cursor, slots, &sp));
    }

    for (size_t i = 0; i != 2; i++) {
        PGDC cursor;
        pgdc_reset(&cursor, pg, slots / 2, 1);

        STORAGE_POINT sp;
        for (size_t slot = slots / 2; slot != slots; slot++) {
            EXPECT_TRUE(pgdc_get_next_point(&cursor, slot, &sp));

            EXPECT_EQ(slot, static_cast<size_t>(sp.min));
            EXPECT_EQ(sp.min, sp.max);
            EXPECT_EQ(sp.min, sp.sum);
            EXPECT_EQ(sp.count, 1);
            EXPECT_EQ(sp.anomaly_count, 0);
        }

        EXPECT_FALSE(pgdc_get_next_point(&cursor, slots, &sp));
    }

    // out of bounds seek
    {
        PGDC cursor;
        pgdc_reset(&cursor, pg, 2 * slots, 1);

        STORAGE_POINT sp;
        EXPECT_FALSE(pgdc_get_next_point(&cursor, 2 * slots, &sp));
    }

    pgd_free(pg);
}

TEST(PGD, CursorHalfPage) {
    size_t slots = slots_for_page(1024 * 1024);
    PGD *pg = pgd_create(page_type, slots);

    PGDC cursor;
    STORAGE_POINT sp;

    // fill the 1st half of the page
    for (size_t slot = 0; slot != slots / 2; slot++)
        pgd_append_point(pg, slot, slot, 0, 0, 1, 1, SN_DEFAULT_FLAGS, slot);

    pgdc_reset(&cursor, pg, 0, 1);

    for (size_t slot = 0; slot != slots / 2; slot++) {
        EXPECT_TRUE(pgdc_get_next_point(&cursor, slot, &sp));

        EXPECT_EQ(slot, static_cast<size_t>(sp.min));
        EXPECT_EQ(sp.min, sp.max);
        EXPECT_EQ(sp.min, sp.sum);
        EXPECT_EQ(sp.count, 1);
        EXPECT_EQ(sp.anomaly_count, 0);
    }
    EXPECT_FALSE(pgdc_get_next_point(&cursor, slots / 2, &sp));

    // reset pgdc to the end of the page, we should not be getting more
    // points even if the page has grown in between.

    pgdc_reset(&cursor, pg, slots / 2, 1);

    for (size_t slot = slots / 2; slot != slots; slot++)
        pgd_append_point(pg, slot, slot, 0, 0, 1, 1, SN_DEFAULT_FLAGS, slot);

    for (size_t slot = slots / 2; slot != slots; slot++)
        EXPECT_FALSE(pgdc_get_next_point(&cursor, slot, &sp));

    EXPECT_FALSE(pgdc_get_next_point(&cursor, slots, &sp));

    pgd_free(pg);
}

TEST(PGD, MemoryFootprint) {
    size_t slots = slots_for_page(1024 * 1024);
    PGD *pg = pgd_create(page_type, slots);

    uint32_t footprint = 0;
    switch (pgd_type(pg)) {
        case PAGE_METRICS:
            footprint = slots * sizeof(uint32_t);
            break;
        case PAGE_GORILLA_METRICS:
            footprint = 128 * sizeof(uint32_t);
            break;
        default:
            fatal("Uknown page type: %uc", pgd_type(pg));
    }
    EXPECT_NEAR(pgd_memory_footprint(pg), footprint, 128);

    std::random_device rand_dev;
    std::mt19937 gen(rand_dev());
    std::uniform_int_distribution<uint32_t> distr(std::numeric_limits<uint32_t>::min(),
                                          std::numeric_limits<uint32_t>::max()); // define the range

    for (size_t slot = 0; slot != slots; slot++) {
        uint32_t n = distr(gen);
        pgd_append_point(pg, slot, n, 0, 0, 1, 1, SN_DEFAULT_FLAGS, slot);
    }

    footprint = slots * sizeof(uint32_t);

    uint32_t abs_error = 0;
    switch (pgd_type(pg)) {
        case PAGE_METRICS:
            abs_error = 128;
            break;
        case PAGE_GORILLA_METRICS:
            abs_error = footprint / 10;
            break;
        default:
            fatal("Uknown page type: %uc", pgd_type(pg));
    }

    EXPECT_NEAR(pgd_memory_footprint(pg), footprint, abs_error);
}

TEST(PGD, DiskFootprint) {
    size_t slots = slots_for_page(1024 * 1024);
    PGD *pg = pgd_create(page_type, slots);

    std::random_device rand_dev;
    std::mt19937 gen(rand_dev());
    std::uniform_int_distribution<uint32_t> distr(std::numeric_limits<uint32_t>::min(),
                                          std::numeric_limits<uint32_t>::max()); // define the range

    size_t used_slots = 16;

    for (size_t slot = 0; slot != used_slots; slot++) {
        uint32_t n = distr(gen);
        pgd_append_point(pg, slot, n, 0, 0, 1, 1, SN_DEFAULT_FLAGS, slot);
    }

    uint32_t footprint = 0;
    switch (pgd_type(pg)) {
        case PAGE_METRICS:
            footprint = used_slots * sizeof(uint32_t);
            break;
        case PAGE_GORILLA_METRICS:
            footprint = 128 * sizeof(uint32_t);
            break;
        default:
            fatal("Uknown page type: %uc", pgd_type(pg));
    }
    EXPECT_EQ(pgd_disk_footprint(pg), footprint);

    pgd_free(pg);

    pg = pgd_create(page_type, slots);

    used_slots = 128 + 64;

    for (size_t slot = 0; slot != used_slots; slot++) {
        uint32_t n = distr(gen);
        pgd_append_point(pg, slot, n, 0, 0, 1, 1, SN_DEFAULT_FLAGS, slot);
    }

    switch (pgd_type(pg)) {
        case PAGE_METRICS:
            footprint = used_slots * sizeof(uint32_t);
            break;
        case PAGE_GORILLA_METRICS:
            footprint = 2 * (128 * sizeof(uint32_t));
            break;
        default:
            fatal("Uknown page type: %uc", pgd_type(pg));
    }
    EXPECT_EQ(pgd_disk_footprint(pg), footprint);

    pgd_free(pg);
}

TEST(PGD, CopyToExtent) {
    size_t slots = slots_for_page(1024 * 1024);
    PGD *pg_collector = pgd_create(page_type, slots);

    uint32_t value = 666;
    pgd_append_point(pg_collector, 0, value, 0, 0, 1, 0, SN_DEFAULT_FLAGS, 0);

    uint32_t size_in_bytes = pgd_disk_footprint(pg_collector);
    EXPECT_EQ(size_in_bytes, 512);

    uint32_t size_in_words = size_in_bytes / sizeof(uint32_t);
    alignas(sizeof(uintptr_t)) uint32_t disk_buffer[size_in_words];

    for (size_t i = 0; i != size_in_words; i++) {
        disk_buffer[i] = std::numeric_limits<uint32_t>::max();
    }

    pgd_copy_to_extent(pg_collector, (uint8_t *) &disk_buffer[0], size_in_bytes);

    EXPECT_EQ(disk_buffer[0], NULL);
    EXPECT_EQ(disk_buffer[1], NULL);
    EXPECT_EQ(disk_buffer[2], 1);
    EXPECT_EQ(disk_buffer[3], 32);
    storage_number sn = pack_storage_number(value, SN_DEFAULT_FLAGS);
    EXPECT_EQ(disk_buffer[4], sn);

    // make sure the rest of the page is 0'ed so that it's amenable to compression
    for (size_t i = 5; i != size_in_words; i++)
        EXPECT_EQ(disk_buffer[i], 0);

    pgd_free(pg_collector);
}

TEST(PGD, Roundtrip) {
    size_t slots = slots_for_page(1024 * 1024);
    PGD *pg_collector = pgd_create(page_type, slots);

    for (size_t i = 0; i != slots; i++)
        pgd_append_point(pg_collector, i, i, 0, 0, 1, 1, SN_DEFAULT_FLAGS, i);

    uint32_t size_in_bytes = pgd_disk_footprint(pg_collector);
    uint32_t size_in_words = size_in_bytes / sizeof(uint32_t);

    alignas(sizeof(uintptr_t)) uint32_t disk_buffer[size_in_words];
    for (size_t i = 0; i != size_in_words; i++)
        disk_buffer[i] = std::numeric_limits<uint32_t>::max();

    pgd_copy_to_extent(pg_collector, (uint8_t *) &disk_buffer[0], size_in_bytes);

    PGD *pg_disk = pgd_create_from_disk_data(page_type, &disk_buffer[0], size_in_bytes);
    EXPECT_EQ(pgd_slots_used(pg_disk), slots);

    // Expected memory footprint is equal to the disk footprint + a couple
    // bytes for the PGD metadata.
    EXPECT_NEAR(pgd_memory_footprint(pg_disk), size_in_bytes, 128);

    // Do not allow calling disk footprint for pages created from disk.
    EXPECT_DEATH(pgd_disk_footprint(pg_disk), ".*");

    for (size_t i = 0; i != 10; i++) {
        PGDC cursor_collector;
        PGDC cursor_disk;

        pgdc_reset(&cursor_collector, pg_collector, i * 1024, 1);
        pgdc_reset(&cursor_disk, pg_disk, i * 1024, 1);

        STORAGE_POINT sp_collector = {};
        STORAGE_POINT sp_disk = {};

        for (size_t slot = i * 1024; slot != slots; slot++) {
            EXPECT_TRUE(pgdc_get_next_point(&cursor_collector, slot, &sp_collector));
            EXPECT_TRUE(pgdc_get_next_point(&cursor_disk, slot, &sp_disk));

            EXPECT_EQ(sp_collector, sp_disk);
        }

        EXPECT_FALSE(pgdc_get_next_point(&cursor_collector, slots, &sp_collector));
        EXPECT_FALSE(pgdc_get_next_point(&cursor_disk, slots, &sp_disk));
    }

    pgd_free(pg_disk);
    pgd_free(pg_collector);
}

TEST(PGD, RejectCorruptGorillaDiskChain) {
    size_t slots = slots_for_page(16);
    PGD *pg_collector = pgd_create(page_type, slots);

    for (size_t i = 0; i != slots; i++)
        pgd_append_point(pg_collector, i, i, 0, 0, 1, 1, SN_DEFAULT_FLAGS, i);

    uint32_t size_in_bytes = pgd_disk_footprint(pg_collector);
    uint32_t size_in_words = size_in_bytes / sizeof(uint32_t);

    alignas(sizeof(uintptr_t)) uint32_t disk_buffer[size_in_words];
    pgd_copy_to_extent(pg_collector, (uint8_t *) &disk_buffer[0], size_in_bytes);

    void *last_gbuf = &disk_buffer[size_in_words - RRDENG_GORILLA_32BIT_BUFFER_SLOTS];
    memcpy(last_gbuf, &last_gbuf, sizeof(last_gbuf));

    PGD *pg_disk = pgd_create_from_disk_data(page_type, &disk_buffer[0], size_in_bytes);
    EXPECT_EQ(pg_disk, PGD_EMPTY);

    pgd_free(pg_collector);
}

TEST(PGD, StopCorruptGorillaDiskEntriesAtEncodedBits) {
    PGD *pg_collector = pgd_create(page_type, slots_for_page(1));

    uint32_t value = 666;
    pgd_append_point(pg_collector, 0, value, 0, 0, 1, 1, SN_DEFAULT_FLAGS, 0);

    uint32_t size_in_bytes = pgd_disk_footprint(pg_collector);
    uint32_t size_in_words = size_in_bytes / sizeof(uint32_t);

    alignas(sizeof(uintptr_t)) uint32_t disk_buffer[size_in_words];
    pgd_copy_to_extent(pg_collector, (uint8_t *) &disk_buffer[0], size_in_bytes);

    auto *gbuf = static_cast<gorilla_buffer_t *>(static_cast<void *>(&disk_buffer[0]));
    gbuf->header.entries++;

    PGD *pg_disk = pgd_create_from_disk_data(page_type, &disk_buffer[0], size_in_bytes);
    EXPECT_EQ(pgd_slots_used(pg_disk), 2);

    PGDC cursor;
    pgdc_reset(&cursor, pg_disk, 0, 1);

    STORAGE_POINT sp = {};
    EXPECT_TRUE(pgdc_get_next_point(&cursor, 0, &sp));
    EXPECT_EQ(value, static_cast<uint32_t>(sp.min));
    EXPECT_FALSE(pgdc_get_next_point(&cursor, 1, &sp));

    pgd_free(pg_disk);
    pgd_free(pg_collector);
}

TEST(PGD, RejectCorruptGorillaDiskNbits) {
    PGD *pg_collector = pgd_create(page_type, slots_for_page(1));

    pgd_append_point(pg_collector, 0, 666, 0, 0, 1, 1, SN_DEFAULT_FLAGS, 0);

    uint32_t size_in_bytes = pgd_disk_footprint(pg_collector);
    uint32_t size_in_words = size_in_bytes / sizeof(uint32_t);

    alignas(sizeof(uintptr_t)) uint32_t disk_buffer[size_in_words];
    pgd_copy_to_extent(pg_collector, (uint8_t *) &disk_buffer[0], size_in_bytes);

    auto *gbuf = static_cast<gorilla_buffer_t *>(static_cast<void *>(&disk_buffer[0]));
    gbuf->header.nbits = RRDENG_GORILLA_32BIT_BUFFER_SIZE * CHAR_BIT;

    PGD *pg_disk = pgd_create_from_disk_data(page_type, &disk_buffer[0], size_in_bytes);
    EXPECT_EQ(pg_disk, PGD_EMPTY);

    pgd_free(pg_collector);
}

int pgd_test(int argc, char *argv[])
{
    // Dummy/necessary initialization stuff
    PGC *dummy_cache = pgc_create("pgd-tests-cache", 32 * 1024 * 1024, NULL, 64, NULL, NULL,
                                  10, 10, 1000, 10, PGC_OPTIONS_NONE, 1, 11);
    pgd_init_arals();

    ::testing::InitGoogleTest(&argc, argv);
    int rc = RUN_ALL_TESTS();

    pgc_destroy(dummy_cache);

    return rc;
}

#else // HAVE_GTEST

int pgd_test(int argc, char *argv[])
{
    (void) argc;
    (void) argv;
    fprintf(stderr, "Can not run PGD tests because the agent was not build with support for google tests.\n");
    return 0;
}

#endif // HAVE_GTEST

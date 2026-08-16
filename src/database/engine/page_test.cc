#include "page_test_bridge.h"
#include "page_test.h"

static size_t pgd_unittest_tier0(uint8_t page_type) {
    size_t errors = 0;

#define PGD_EXPECT(condition) do {                                                                                       \
    if(!(condition)) {                                                                                                  \
        fprintf(stderr, "PGD storage-point unittest failed at %s:%d: %s\n", __FILE__, __LINE__, #condition);           \
        errors++;                                                                                                       \
    }                                                                                                                   \
} while(0)

    PGD *pg = pgd_unittest_create(page_type, 4);
    pgd_unittest_append_point(pg, 1, 42, 42, 42, 1, 0, SN_DEFAULT_FLAGS, 0);
    pgd_unittest_append_point(pg, 2, NAN, NAN, NAN, 0, 0, SN_EMPTY_SLOT, 1);
    pgd_unittest_append_point(pg, 3, -7, -7, -7, 1, 0, SN_FLAG_RESET, 2);

    PGDC cursor = {};
    pgd_unittest_cursor_reset(&cursor, pg, 0, 1);

    STORAGE_POINT sp = STORAGE_POINT_UNSET;
    sp.start_time_s = 0;
    sp.end_time_s = 1;
    PGD_EXPECT(pgd_unittest_cursor_next(&cursor, 0, &sp));
    PGD_EXPECT(sp.start_time_s == 0 && sp.end_time_s == 1);
    PGD_EXPECT(storage_point_is_complete(sp));
    PGD_EXPECT(sp.min == 42 && sp.max == 42 && sp.sum == 42);
    PGD_EXPECT(sp.count == 1 && sp.gap_count == 0 && sp.anomaly_count == 0);
    PGD_EXPECT(sp.flags & SN_FLAG_NOT_ANOMALOUS);

    sp.start_time_s = 1;
    sp.end_time_s = 2;
    PGD_EXPECT(pgd_unittest_cursor_next(&cursor, 1, &sp));
    PGD_EXPECT(sp.start_time_s == 1 && sp.end_time_s == 2);
    PGD_EXPECT(storage_point_is_gap(sp));
    PGD_EXPECT(!netdata_double_isnumber(sp.min) && !netdata_double_isnumber(sp.max) &&
               !netdata_double_isnumber(sp.sum));
    PGD_EXPECT(sp.count == 0 && sp.gap_count == 1 && sp.anomaly_count == 0);
    PGD_EXPECT(sp.flags == SN_FLAG_NONE);

    sp.start_time_s = 2;
    sp.end_time_s = 3;
    PGD_EXPECT(pgd_unittest_cursor_next(&cursor, 2, &sp));
    PGD_EXPECT(sp.start_time_s == 2 && sp.end_time_s == 3);
    PGD_EXPECT(storage_point_is_complete(sp));
    PGD_EXPECT(sp.min == -7 && sp.max == -7 && sp.sum == -7);
    PGD_EXPECT(sp.count == 1 && sp.gap_count == 0 && sp.anomaly_count == 1);
    PGD_EXPECT((sp.flags & SN_FLAG_RESET) && !(sp.flags & SN_FLAG_NOT_ANOMALOUS));

    sp.start_time_s = 3;
    sp.end_time_s = 4;
    PGD_EXPECT(!pgd_unittest_cursor_next(&cursor, 3, &sp));
    PGD_EXPECT(sp.start_time_s == 3 && sp.end_time_s == 4);
    PGD_EXPECT(storage_point_is_gap(sp));
    PGD_EXPECT(!netdata_double_isnumber(sp.min) && !netdata_double_isnumber(sp.max) &&
               !netdata_double_isnumber(sp.sum));
    PGD_EXPECT(sp.count == 0 && sp.gap_count == 1 && sp.anomaly_count == 0);
    PGD_EXPECT(sp.flags == SN_FLAG_NONE);

    pgd_unittest_free(pg);
    return errors;
}

static size_t pgd_unittest_tier1(void) {
    size_t errors = 0;

    PGD *gap_pg = pgd_unittest_create(RRDENG_PAGE_TYPE_ARRAY_TIER1, 2);
    pgd_unittest_append_point(gap_pg, 60, NAN, NAN, NAN, 0, 0, SN_FLAG_NONE, 0);
    PGD_EXPECT(pgd_unittest_slots_used(gap_pg) == 1);
    PGD_EXPECT(pgd_unittest_is_empty(gap_pg));
    pgd_unittest_free(gap_pg);

    PGD *pg = pgd_unittest_create(RRDENG_PAGE_TYPE_ARRAY_TIER1, 6);
    pgd_unittest_append_point(pg, 60, 400, 9, 11, 40, 4, SN_DEFAULT_FLAGS, 0);
    pgd_unittest_append_point(pg, 120, 800, 9, 11, 80, 0, SN_DEFAULT_FLAGS, 1);
    pgd_unittest_append_point(pg, 180, 300, 9, 11, 0, 0, SN_DEFAULT_FLAGS, 2);
    pgd_unittest_append_point(pg, 240, NAN, NAN, NAN, 7, 1, SN_DEFAULT_FLAGS, 3);
    pgd_unittest_append_point(pg, 300, INFINITY, 9, 11, 7, 1, SN_DEFAULT_FLAGS, 4);

    PGDC cursor = {};
    pgd_unittest_cursor_reset(&cursor, pg, 0, 60);

    STORAGE_POINT sp = STORAGE_POINT_UNSET;
    sp.start_time_s = 0;
    sp.end_time_s = 60;
    PGD_EXPECT(pgd_unittest_cursor_next(&cursor, 0, &sp));
    PGD_EXPECT(storage_point_is_partial(sp));
    PGD_EXPECT(sp.start_time_s == 0 && sp.end_time_s == 60);
    PGD_EXPECT(sp.min == 9 && sp.max == 11 && sp.sum == 400);
    PGD_EXPECT(sp.count == 40 && sp.gap_count == 20 && sp.anomaly_count == 4);
    PGD_EXPECT(!(sp.flags & SN_FLAG_NOT_ANOMALOUS));

    sp.start_time_s = 60;
    sp.end_time_s = 120;
    PGD_EXPECT(pgd_unittest_cursor_next(&cursor, 1, &sp));
    PGD_EXPECT(storage_point_is_complete(sp));
    PGD_EXPECT(sp.start_time_s == 60 && sp.end_time_s == 120);
    PGD_EXPECT(sp.min == 9 && sp.max == 11 && sp.sum == 800);
    PGD_EXPECT(sp.count == 80 && sp.gap_count == 0 && sp.anomaly_count == 0);
    PGD_EXPECT(sp.flags & SN_FLAG_NOT_ANOMALOUS);

    sp.start_time_s = 120;
    sp.end_time_s = 180;
    PGD_EXPECT(pgd_unittest_cursor_next(&cursor, 2, &sp));
    PGD_EXPECT(storage_point_is_gap(sp));
    PGD_EXPECT(sp.start_time_s == 120 && sp.end_time_s == 180);
    PGD_EXPECT(!netdata_double_isnumber(sp.min) && !netdata_double_isnumber(sp.max) &&
               !netdata_double_isnumber(sp.sum));
    PGD_EXPECT(sp.count == 0 && sp.gap_count == 60 && sp.anomaly_count == 0);
    PGD_EXPECT(sp.flags == SN_FLAG_NONE);

    sp.start_time_s = 180;
    sp.end_time_s = 240;
    PGD_EXPECT(pgd_unittest_cursor_next(&cursor, 3, &sp));
    PGD_EXPECT(storage_point_is_gap(sp));
    PGD_EXPECT(sp.start_time_s == 180 && sp.end_time_s == 240);
    PGD_EXPECT(!netdata_double_isnumber(sp.min) && !netdata_double_isnumber(sp.max) &&
               !netdata_double_isnumber(sp.sum));
    PGD_EXPECT(sp.count == 0 && sp.gap_count == 60 && sp.anomaly_count == 0);
    PGD_EXPECT(sp.flags == SN_FLAG_NONE);

    sp.start_time_s = 240;
    sp.end_time_s = 300;
    PGD_EXPECT(pgd_unittest_cursor_next(&cursor, 4, &sp));
    PGD_EXPECT(storage_point_is_gap(sp));
    PGD_EXPECT(sp.start_time_s == 240 && sp.end_time_s == 300);
    PGD_EXPECT(!netdata_double_isnumber(sp.min) && !netdata_double_isnumber(sp.max) &&
               !netdata_double_isnumber(sp.sum));
    PGD_EXPECT(sp.count == 0 && sp.gap_count == 60 && sp.anomaly_count == 0);
    PGD_EXPECT(sp.flags == SN_FLAG_NONE);

    sp.start_time_s = 300;
    sp.end_time_s = 360;
    PGD_EXPECT(!pgd_unittest_cursor_next(&cursor, 5, &sp));
    PGD_EXPECT(storage_point_is_gap(sp));
    PGD_EXPECT(sp.start_time_s == 300 && sp.end_time_s == 360);
    PGD_EXPECT(!netdata_double_isnumber(sp.min) && !netdata_double_isnumber(sp.max) &&
               !netdata_double_isnumber(sp.sum));
    PGD_EXPECT(sp.count == 0 && sp.gap_count == 60 && sp.anomaly_count == 0);
    PGD_EXPECT(sp.flags == SN_FLAG_NONE);

    pgd_unittest_cursor_reset(&cursor, pg, 2, 65536);
    sp.start_time_s = 0;
    sp.end_time_s = 65536;
    PGD_EXPECT(pgd_unittest_cursor_next(&cursor, 2, &sp));
    PGD_EXPECT(storage_point_is_gap(sp));
    PGD_EXPECT(sp.start_time_s == 0 && sp.end_time_s == 65536);
    PGD_EXPECT(!netdata_double_isnumber(sp.min) && !netdata_double_isnumber(sp.max) &&
               !netdata_double_isnumber(sp.sum));
    PGD_EXPECT(sp.count == 0 && sp.gap_count == 65536);
    PGD_EXPECT(sp.flags == SN_FLAG_NONE);

    pgd_unittest_free(pg);
    return errors;
}

static size_t pgd_unittest_corrupt_gorilla_entries(void) {
    size_t errors = 0;

    PGD *pg_collector = pgd_unittest_create(RRDENG_PAGE_TYPE_GORILLA_32BIT, 2);
    pgd_unittest_append_point(pg_collector, 1, 666, 666, 666, 1, 0, SN_DEFAULT_FLAGS, 0);

    uint32_t size_in_bytes = pgd_unittest_disk_footprint(pg_collector);
    auto *disk_buffer = static_cast<uint32_t *>(mallocz(size_in_bytes));
    pgd_unittest_copy_to_extent(pg_collector, reinterpret_cast<uint8_t *>(disk_buffer), size_in_bytes);

    auto *gbuf = reinterpret_cast<gorilla_buffer_t *>(disk_buffer);
    gbuf->header.entries++;

    PGD *pg_disk = pgd_unittest_create_from_disk_data(
        RRDENG_PAGE_TYPE_GORILLA_32BIT, disk_buffer, size_in_bytes);
    PGD_EXPECT(pg_disk != PGD_EMPTY);
    PGD_EXPECT(pgd_unittest_slots_used(pg_disk) == 2);

    if(pg_disk != PGD_EMPTY) {
        PGDC cursor = {};
        pgd_unittest_cursor_reset(&cursor, pg_disk, 0, 1);

        STORAGE_POINT sp = STORAGE_POINT_UNSET;
        sp.start_time_s = 0;
        sp.end_time_s = 1;
        PGD_EXPECT(pgd_unittest_cursor_next(&cursor, 0, &sp));
        PGD_EXPECT(sp.start_time_s == 0 && sp.end_time_s == 1);
        PGD_EXPECT(storage_point_is_complete(sp));
        PGD_EXPECT(sp.min == 666 && sp.max == 666 && sp.sum == 666);
        PGD_EXPECT(sp.count == 1 && sp.gap_count == 0 && sp.anomaly_count == 0);
        PGD_EXPECT(sp.flags & SN_FLAG_NOT_ANOMALOUS);

        sp.start_time_s = 1;
        sp.end_time_s = 2;
        PGD_EXPECT(!pgd_unittest_cursor_next(&cursor, 1, &sp));
        PGD_EXPECT(sp.start_time_s == 1 && sp.end_time_s == 2);
        PGD_EXPECT(storage_point_is_gap(sp));
        PGD_EXPECT(!netdata_double_isnumber(sp.min) && !netdata_double_isnumber(sp.max) &&
                   !netdata_double_isnumber(sp.sum));
        PGD_EXPECT(sp.count == 0 && sp.gap_count == 1 && sp.anomaly_count == 0);
        PGD_EXPECT(sp.flags == SN_FLAG_NONE);

        pgd_unittest_free(pg_disk);
    }

    pgd_unittest_free(pg_collector);
    freez(disk_buffer);
    return errors;
}

int pgd_storage_point_unittest(void) {
    size_t errors = 0;

    PGDC cursor = {};
    STORAGE_POINT sp = STORAGE_POINT_UNSET;
    sp.start_time_s = 10;
    sp.end_time_s = 11;
    pgd_unittest_cursor_reset(&cursor, nullptr, UINT32_MAX, 0);
    PGD_EXPECT(cursor.slots_per_point == 1);
    PGD_EXPECT(!pgd_unittest_cursor_next(&cursor, 0, &sp));
    PGD_EXPECT(storage_point_is_gap(sp) && sp.gap_count == 1);

    sp.start_time_s = 10;
    sp.end_time_s = 65546;
    pgd_unittest_cursor_reset(&cursor, nullptr, UINT32_MAX, 65536);
    PGD_EXPECT(cursor.slots_per_point == 65536);
    PGD_EXPECT(!pgd_unittest_cursor_next(&cursor, 0, &sp));
    PGD_EXPECT(storage_point_is_gap(sp) && sp.gap_count == 65536);

    errors += pgd_unittest_tier0(RRDENG_PAGE_TYPE_GORILLA_32BIT);
    errors += pgd_unittest_tier0(RRDENG_PAGE_TYPE_ARRAY_32BIT);
    errors += pgd_unittest_tier1();
    errors += pgd_unittest_corrupt_gorilla_entries();

    fprintf(stderr, "PGD storage-point unittests: %s\n", errors ? "FAILED" : "PASSED");
    return (int)errors;
}

#undef PGD_EXPECT

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
    EXPECT_EQ(pgd_unittest_slots_used(pg), 0);
    EXPECT_EQ(pgd_memory_footprint(pg), 0);
    EXPECT_EQ(pgd_unittest_disk_footprint(pg), 0);

    pgd_unittest_cursor_reset(&cursor, pg, 0, 1);
    EXPECT_FALSE(pgd_unittest_cursor_next(&cursor, 0, &sp));

    pgd_unittest_free(pg);

    pg = PGD_EMPTY;

    EXPECT_TRUE(pgd_is_empty(pg));
    EXPECT_EQ(pgd_unittest_slots_used(pg), 0);
    EXPECT_EQ(pgd_memory_footprint(pg), 0);
    EXPECT_EQ(pgd_unittest_disk_footprint(pg), 0);
    EXPECT_FALSE(pgd_unittest_cursor_next(&cursor, 0, &sp));

    pgd_unittest_cursor_reset(&cursor, pg, 0, 1);
    EXPECT_FALSE(pgd_unittest_cursor_next(&cursor, 0, &sp));

    pgd_unittest_free(pg);
}

TEST(PGD, Create) {
    size_t slots = slots_for_page(1024 * 1024);
    PGD *pg = pgd_unittest_create(page_type, slots);

    EXPECT_EQ(pgd_type(pg), page_type);
    EXPECT_TRUE(pgd_is_empty(pg));
    EXPECT_EQ(pgd_unittest_slots_used(pg), 0);

    for (size_t i = 0; i != slots; i++) {
        pgd_unittest_append_point(pg, i, i, 0, 0, 1, 1, SN_DEFAULT_FLAGS, i);
        EXPECT_FALSE(pgd_is_empty(pg));
    }
    EXPECT_EQ(pgd_unittest_slots_used(pg), slots);

    EXPECT_DEATH(
        pgd_unittest_append_point(pg, slots, slots, 0, 0, 1, 1, SN_DEFAULT_FLAGS, slots),
        ".*"
    );

    pgd_unittest_free(pg);
}

TEST(PGD, CursorFullPage) {
    size_t slots = slots_for_page(1024 * 1024);
    PGD *pg = pgd_unittest_create(page_type, slots);

    for (size_t slot = 0; slot != slots; slot++)
        pgd_unittest_append_point(pg, slot, slot, 0, 0, 1, 1, SN_DEFAULT_FLAGS, slot);

    for (size_t i = 0; i != 2; i++) {
        PGDC cursor;
        pgd_unittest_cursor_reset(&cursor, pg, 0, 1);

        STORAGE_POINT sp;
        for (size_t slot = 0; slot != slots; slot++) {
            EXPECT_TRUE(pgd_unittest_cursor_next(&cursor, slot, &sp));

            EXPECT_EQ(slot, static_cast<size_t>(sp.min));
            EXPECT_EQ(sp.min, sp.max);
            EXPECT_EQ(sp.min, sp.sum);
            EXPECT_EQ(sp.count, 1);
            EXPECT_EQ(sp.anomaly_count, 0);
        }

        EXPECT_FALSE(pgd_unittest_cursor_next(&cursor, slots, &sp));
    }

    for (size_t i = 0; i != 2; i++) {
        PGDC cursor;
        pgd_unittest_cursor_reset(&cursor, pg, slots / 2, 1);

        STORAGE_POINT sp;
        for (size_t slot = slots / 2; slot != slots; slot++) {
            EXPECT_TRUE(pgd_unittest_cursor_next(&cursor, slot, &sp));

            EXPECT_EQ(slot, static_cast<size_t>(sp.min));
            EXPECT_EQ(sp.min, sp.max);
            EXPECT_EQ(sp.min, sp.sum);
            EXPECT_EQ(sp.count, 1);
            EXPECT_EQ(sp.anomaly_count, 0);
        }

        EXPECT_FALSE(pgd_unittest_cursor_next(&cursor, slots, &sp));
    }

    // out of bounds seek
    {
        PGDC cursor;
        pgd_unittest_cursor_reset(&cursor, pg, 2 * slots, 1);

        STORAGE_POINT sp;
        EXPECT_FALSE(pgd_unittest_cursor_next(&cursor, 2 * slots, &sp));
    }

    pgd_unittest_free(pg);
}

TEST(PGD, CursorHalfPage) {
    size_t slots = slots_for_page(1024 * 1024);
    PGD *pg = pgd_unittest_create(page_type, slots);

    PGDC cursor;
    STORAGE_POINT sp;

    // fill the 1st half of the page
    for (size_t slot = 0; slot != slots / 2; slot++)
        pgd_unittest_append_point(pg, slot, slot, 0, 0, 1, 1, SN_DEFAULT_FLAGS, slot);

    pgd_unittest_cursor_reset(&cursor, pg, 0, 1);

    for (size_t slot = 0; slot != slots / 2; slot++) {
        EXPECT_TRUE(pgd_unittest_cursor_next(&cursor, slot, &sp));

        EXPECT_EQ(slot, static_cast<size_t>(sp.min));
        EXPECT_EQ(sp.min, sp.max);
        EXPECT_EQ(sp.min, sp.sum);
        EXPECT_EQ(sp.count, 1);
        EXPECT_EQ(sp.anomaly_count, 0);
    }
    EXPECT_FALSE(pgd_unittest_cursor_next(&cursor, slots / 2, &sp));

    // reset pgdc to the end of the page, we should not be getting more
    // points even if the page has grown in between.

    pgd_unittest_cursor_reset(&cursor, pg, slots / 2, 1);

    for (size_t slot = slots / 2; slot != slots; slot++)
        pgd_unittest_append_point(pg, slot, slot, 0, 0, 1, 1, SN_DEFAULT_FLAGS, slot);

    for (size_t slot = slots / 2; slot != slots; slot++)
        EXPECT_FALSE(pgd_unittest_cursor_next(&cursor, slot, &sp));

    EXPECT_FALSE(pgd_unittest_cursor_next(&cursor, slots, &sp));

    pgd_unittest_free(pg);
}

TEST(PGD, MemoryFootprint) {
    size_t slots = slots_for_page(1024 * 1024);
    PGD *pg = pgd_unittest_create(page_type, slots);

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
        pgd_unittest_append_point(pg, slot, n, 0, 0, 1, 1, SN_DEFAULT_FLAGS, slot);
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
    PGD *pg = pgd_unittest_create(page_type, slots);

    std::random_device rand_dev;
    std::mt19937 gen(rand_dev());
    std::uniform_int_distribution<uint32_t> distr(std::numeric_limits<uint32_t>::min(),
                                          std::numeric_limits<uint32_t>::max()); // define the range

    size_t used_slots = 16;

    for (size_t slot = 0; slot != used_slots; slot++) {
        uint32_t n = distr(gen);
        pgd_unittest_append_point(pg, slot, n, 0, 0, 1, 1, SN_DEFAULT_FLAGS, slot);
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
    EXPECT_EQ(pgd_unittest_disk_footprint(pg), footprint);

    pgd_unittest_free(pg);

    pg = pgd_unittest_create(page_type, slots);

    used_slots = 128 + 64;

    for (size_t slot = 0; slot != used_slots; slot++) {
        uint32_t n = distr(gen);
        pgd_unittest_append_point(pg, slot, n, 0, 0, 1, 1, SN_DEFAULT_FLAGS, slot);
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
    EXPECT_EQ(pgd_unittest_disk_footprint(pg), footprint);

    pgd_unittest_free(pg);
}

TEST(PGD, CopyToExtent) {
    size_t slots = slots_for_page(1024 * 1024);
    PGD *pg_collector = pgd_unittest_create(page_type, slots);

    uint32_t value = 666;
    pgd_unittest_append_point(pg_collector, 0, value, 0, 0, 1, 0, SN_DEFAULT_FLAGS, 0);

    uint32_t size_in_bytes = pgd_unittest_disk_footprint(pg_collector);
    EXPECT_EQ(size_in_bytes, 512);

    uint32_t size_in_words = size_in_bytes / sizeof(uint32_t);
    alignas(sizeof(uintptr_t)) uint32_t disk_buffer[size_in_words];

    for (size_t i = 0; i != size_in_words; i++) {
        disk_buffer[i] = std::numeric_limits<uint32_t>::max();
    }

    pgd_unittest_copy_to_extent(pg_collector, (uint8_t *) &disk_buffer[0], size_in_bytes);

    EXPECT_EQ(disk_buffer[0], NULL);
    EXPECT_EQ(disk_buffer[1], NULL);
    EXPECT_EQ(disk_buffer[2], 1);
    EXPECT_EQ(disk_buffer[3], 32);
    storage_number sn = pack_storage_number(value, SN_DEFAULT_FLAGS);
    EXPECT_EQ(disk_buffer[4], sn);

    // make sure the rest of the page is 0'ed so that it's amenable to compression
    for (size_t i = 5; i != size_in_words; i++)
        EXPECT_EQ(disk_buffer[i], 0);

    pgd_unittest_free(pg_collector);
}

TEST(PGD, Roundtrip) {
    size_t slots = slots_for_page(1024 * 1024);
    PGD *pg_collector = pgd_unittest_create(page_type, slots);

    for (size_t i = 0; i != slots; i++)
        pgd_unittest_append_point(pg_collector, i, i, 0, 0, 1, 1, SN_DEFAULT_FLAGS, i);

    uint32_t size_in_bytes = pgd_unittest_disk_footprint(pg_collector);
    uint32_t size_in_words = size_in_bytes / sizeof(uint32_t);

    alignas(sizeof(uintptr_t)) uint32_t disk_buffer[size_in_words];
    for (size_t i = 0; i != size_in_words; i++)
        disk_buffer[i] = std::numeric_limits<uint32_t>::max();

    pgd_unittest_copy_to_extent(pg_collector, (uint8_t *) &disk_buffer[0], size_in_bytes);

    PGD *pg_disk = pgd_unittest_create_from_disk_data(page_type, &disk_buffer[0], size_in_bytes);
    EXPECT_EQ(pgd_unittest_slots_used(pg_disk), slots);

    // Expected memory footprint is equal to the disk footprint + a couple
    // bytes for the PGD metadata.
    EXPECT_NEAR(pgd_memory_footprint(pg_disk), size_in_bytes, 128);

    // Do not allow calling disk footprint for pages created from disk.
    EXPECT_DEATH(pgd_unittest_disk_footprint(pg_disk), ".*");

    for (size_t i = 0; i != 10; i++) {
        PGDC cursor_collector;
        PGDC cursor_disk;

        pgd_unittest_cursor_reset(&cursor_collector, pg_collector, i * 1024, 1);
        pgd_unittest_cursor_reset(&cursor_disk, pg_disk, i * 1024, 1);

        STORAGE_POINT sp_collector = {};
        STORAGE_POINT sp_disk = {};

        for (size_t slot = i * 1024; slot != slots; slot++) {
            EXPECT_TRUE(pgd_unittest_cursor_next(&cursor_collector, slot, &sp_collector));
            EXPECT_TRUE(pgd_unittest_cursor_next(&cursor_disk, slot, &sp_disk));

            EXPECT_EQ(sp_collector, sp_disk);
        }

        EXPECT_FALSE(pgd_unittest_cursor_next(&cursor_collector, slots, &sp_collector));
        EXPECT_FALSE(pgd_unittest_cursor_next(&cursor_disk, slots, &sp_disk));
    }

    pgd_unittest_free(pg_disk);
    pgd_unittest_free(pg_collector);
}

TEST(PGD, RejectCorruptGorillaDiskChain) {
    size_t slots = slots_for_page(16);
    PGD *pg_collector = pgd_unittest_create(page_type, slots);

    for (size_t i = 0; i != slots; i++)
        pgd_unittest_append_point(pg_collector, i, i, 0, 0, 1, 1, SN_DEFAULT_FLAGS, i);

    uint32_t size_in_bytes = pgd_unittest_disk_footprint(pg_collector);
    uint32_t size_in_words = size_in_bytes / sizeof(uint32_t);

    alignas(sizeof(uintptr_t)) uint32_t disk_buffer[size_in_words];
    pgd_unittest_copy_to_extent(pg_collector, (uint8_t *) &disk_buffer[0], size_in_bytes);

    void *last_gbuf = &disk_buffer[size_in_words - RRDENG_GORILLA_32BIT_BUFFER_SLOTS];
    memcpy(last_gbuf, &last_gbuf, sizeof(last_gbuf));

    PGD *pg_disk = pgd_unittest_create_from_disk_data(page_type, &disk_buffer[0], size_in_bytes);
    EXPECT_EQ(pg_disk, PGD_EMPTY);

    pgd_unittest_free(pg_collector);
}

TEST(PGD, StopCorruptGorillaDiskEntriesAtEncodedBits) {
    PGD *pg_collector = pgd_unittest_create(page_type, slots_for_page(1));

    uint32_t value = 666;
    pgd_unittest_append_point(pg_collector, 0, value, 0, 0, 1, 1, SN_DEFAULT_FLAGS, 0);

    uint32_t size_in_bytes = pgd_unittest_disk_footprint(pg_collector);
    uint32_t size_in_words = size_in_bytes / sizeof(uint32_t);

    alignas(sizeof(uintptr_t)) uint32_t disk_buffer[size_in_words];
    pgd_unittest_copy_to_extent(pg_collector, (uint8_t *) &disk_buffer[0], size_in_bytes);

    auto *gbuf = static_cast<gorilla_buffer_t *>(static_cast<void *>(&disk_buffer[0]));
    gbuf->header.entries++;

    PGD *pg_disk = pgd_unittest_create_from_disk_data(page_type, &disk_buffer[0], size_in_bytes);
    EXPECT_EQ(pgd_unittest_slots_used(pg_disk), 2);

    PGDC cursor;
    pgd_unittest_cursor_reset(&cursor, pg_disk, 0, 1);

    STORAGE_POINT sp = {};
    EXPECT_TRUE(pgd_unittest_cursor_next(&cursor, 0, &sp));
    EXPECT_EQ(value, static_cast<uint32_t>(sp.min));
    EXPECT_FALSE(pgd_unittest_cursor_next(&cursor, 1, &sp));

    pgd_unittest_free(pg_disk);
    pgd_unittest_free(pg_collector);
}

TEST(PGD, RejectCorruptGorillaDiskNbits) {
    PGD *pg_collector = pgd_unittest_create(page_type, slots_for_page(1));

    pgd_unittest_append_point(pg_collector, 0, 666, 0, 0, 1, 1, SN_DEFAULT_FLAGS, 0);

    uint32_t size_in_bytes = pgd_unittest_disk_footprint(pg_collector);
    uint32_t size_in_words = size_in_bytes / sizeof(uint32_t);

    alignas(sizeof(uintptr_t)) uint32_t disk_buffer[size_in_words];
    pgd_unittest_copy_to_extent(pg_collector, (uint8_t *) &disk_buffer[0], size_in_bytes);

    auto *gbuf = static_cast<gorilla_buffer_t *>(static_cast<void *>(&disk_buffer[0]));
    gbuf->header.nbits = RRDENG_GORILLA_32BIT_BUFFER_SIZE * CHAR_BIT;

    PGD *pg_disk = pgd_unittest_create_from_disk_data(page_type, &disk_buffer[0], size_in_bytes);
    EXPECT_EQ(pg_disk, PGD_EMPTY);

    pgd_unittest_free(pg_collector);
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

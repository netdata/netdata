// SPDX-License-Identifier: GPL-3.0-or-later

#include "page_test_bridge.h"

NOINLINE PGD *pgd_unittest_create(uint8_t type, uint32_t slots) {
    return pgd_create(type, slots);
}

NOINLINE PGD *pgd_unittest_create_from_disk_data(uint8_t type, void *base, uint32_t size) {
    return pgd_create_from_disk_data(type, base, size);
}

NOINLINE void pgd_unittest_free(PGD *pg) {
    pgd_free(pg);
}

NOINLINE uint32_t pgd_unittest_slots_used(PGD *pg) {
    return pgd_slots_used(pg);
}

NOINLINE bool pgd_unittest_is_empty(PGD *pg) {
    return pgd_is_empty(pg);
}

NOINLINE uint32_t pgd_unittest_disk_footprint(PGD *pg) {
    return pgd_disk_footprint(pg);
}

NOINLINE void pgd_unittest_copy_to_extent(PGD *pg, uint8_t *dst, uint32_t dst_size) {
    pgd_copy_to_extent(pg, dst, dst_size);
}

NOINLINE size_t pgd_unittest_append_point(
    PGD *pg,
    usec_t point_in_time_ut,
    NETDATA_DOUBLE n,
    NETDATA_DOUBLE min_value,
    NETDATA_DOUBLE max_value,
    uint16_t count,
    uint16_t anomaly_count,
    SN_FLAGS flags,
    uint32_t expected_slot) {
    return pgd_append_point(
        pg, point_in_time_ut, n, min_value, max_value, count, anomaly_count, flags, expected_slot);
}

NOINLINE void pgd_unittest_cursor_reset(
    PGDC *pgdc, PGD *pgd, uint32_t position, uint32_t slots_per_point) {
    pgdc_reset(pgdc, pgd, position, slots_per_point);
}

NOINLINE bool pgd_unittest_cursor_next(
    PGDC *pgdc, uint32_t expected_position, STORAGE_POINT *sp) {
    return pgdc_get_next_point(pgdc, expected_position, sp);
}

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DBENGINE_PAGE_TEST_BRIDGE_H
#define NETDATA_DBENGINE_PAGE_TEST_BRIDGE_H

#include "page.h"

PGD *pgd_unittest_create(uint8_t type, uint32_t slots);
PGD *pgd_unittest_create_from_disk_data(uint8_t type, void *base, uint32_t size);
void pgd_unittest_free(PGD *pg);
uint32_t pgd_unittest_slots_used(PGD *pg);
bool pgd_unittest_is_empty(PGD *pg);
uint32_t pgd_unittest_disk_footprint(PGD *pg);
void pgd_unittest_copy_to_extent(PGD *pg, uint8_t *dst, uint32_t dst_size);

size_t pgd_unittest_append_point(
    PGD *pg,
    usec_t point_in_time_ut,
    NETDATA_DOUBLE n,
    NETDATA_DOUBLE min_value,
    NETDATA_DOUBLE max_value,
    uint16_t count,
    uint16_t anomaly_count,
    SN_FLAGS flags,
    uint32_t expected_slot);

void pgd_unittest_cursor_reset(PGDC *pgdc, PGD *pgd, uint32_t position, uint32_t slots_per_point);
bool pgd_unittest_cursor_next(PGDC *pgdc, uint32_t expected_position, STORAGE_POINT *sp);

#endif // NETDATA_DBENGINE_PAGE_TEST_BRIDGE_H

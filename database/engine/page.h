// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef DBENGINE_PAGE_H
#define DBENGINE_PAGE_H

#include "libnetdata/libnetdata.h"

typedef struct pgd_cursor {
    struct pgd *pgd;
    uint32_t position;
} PGDC;

#include "rrdengine.h"

typedef struct pgd PGD;

#define PGD_EMPTY (PGD *)(-1)

void pgd_init(void);

PGD *pgd_create(uint8_t type, uint32_t slots);
PGD *pgd_create_from_disk_data(uint8_t type, void *base, uint32_t size);

void pgd_free(PGD *pg);

void pgd_append_point(PGD *pg,
                      usec_t point_in_time_ut,
                      NETDATA_DOUBLE n,
                      NETDATA_DOUBLE min_value,
                      NETDATA_DOUBLE max_value,
                      uint16_t count,
                      uint16_t anomaly_count,
                      SN_FLAGS flags,
                      uint32_t expected_slot);

bool pgd_is_empty(PGD *pg);

uint32_t pgd_memory_footprint(PGD *pg);
uint32_t pgd_disk_footprint_size(PGD *pg);
void pgd_copy_to_extent(PGD *pg, uint8_t *dst, uint32_t dst_size);

uint32_t pgd_type(PGD *pg);
uint32_t pgd_slots_used(PGD *pg);

void pgdc_reset(PGDC *pgdc, PGD *pgd, uint32_t position);
#define pgdc_clear(pgdc) pgdc_reset(pgdc, NULL, UINT32_MAX)

bool pgdc_get_next_point(PGDC *pgdc, uint32_t expected_position, STORAGE_POINT *sp);

#endif // DBENGINE_PAGE_H

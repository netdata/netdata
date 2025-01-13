// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef DBENGINE_PAGE_H
#define DBENGINE_PAGE_H

#ifdef __cplusplus
extern "C" {
#endif

#include "libnetdata/libnetdata.h"

typedef struct pgd_cursor {
    struct pgd *pgd;
    uint32_t position;
    uint32_t slots;

    gorilla_reader_t gr;
} PGDC;

#include "rrdengine.h"

typedef struct pgd PGD;

#define PGD_EMPTY (PGD *)(-1)

void pgd_init_arals(void);

PGD *pgd_create(uint8_t type, uint32_t slots);
PGD *pgd_create_from_disk_data(uint8_t type, void *base, uint32_t size);
void pgd_free(PGD *pg);

uint32_t pgd_type(PGD *pg);
bool pgd_is_empty(PGD *pg);
uint32_t pgd_slots_used(PGD *pg);

uint32_t pgd_buffer_memory_footprint(PGD *pg);
uint32_t pgd_memory_footprint(PGD *pg);
uint32_t pgd_capacity(PGD *pg);
uint32_t pgd_disk_footprint(PGD *pg);

struct aral_statistics *pgd_aral_stats(void);
size_t pgd_padding_bytes(void);

void pgd_copy_to_extent(PGD *pg, uint8_t *dst, uint32_t dst_size);

size_t pgd_append_point(PGD *pg,
                      usec_t point_in_time_ut,
                      NETDATA_DOUBLE n,
                      NETDATA_DOUBLE min_value,
                      NETDATA_DOUBLE max_value,
                      uint16_t count,
                      uint16_t anomaly_count,
                      SN_FLAGS flags,
                      uint32_t expected_slot);

void pgdc_reset(PGDC *pgdc, PGD *pgd, uint32_t position);
bool pgdc_get_next_point(PGDC *pgdc, uint32_t expected_position, STORAGE_POINT *sp);

void *dbengine_extent_alloc(size_t size);
void dbengine_extent_free(void *extent, size_t size);

#ifdef __cplusplus
}
#endif

#endif // DBENGINE_PAGE_H

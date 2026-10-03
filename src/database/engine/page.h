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
int pgd_unittest(void);

PGD *pgd_create(uint8_t type, uint32_t slots);
PGD *pgd_create_from_disk_data(uint8_t type, void *base, uint32_t size);
// Every place a PGD can be freed, enumerated. The freed PGD records which one freed it,
// so a double free names BOTH sites - and those tell apart three different bugs:
//
//   MAIN_CACHE_EVICT   twice -> the PGC evicted one page twice (a PGC_PAGE double delete)
//   EXTENT_INSERT_LOST       -> pgc_page_add() reported !added but consumed the PGD anyway
//   DISK_GORILLA_INVALID     -> a PGD freed on the from-disk error path was still published
//
// See the PGD lifetime guard in page.c.
typedef enum {
    PGD_FREE_SITE_UNKNOWN = 0,
    PGD_FREE_SITE_MAIN_CACHE_EVICT,      // pagecache.c: main_cache_free_clean_page_callback()
    PGD_FREE_SITE_EXTENT_INSERT_LOST,    // pdc.c: lost the main cache insert race
    PGD_FREE_SITE_DISK_GORILLA_INVALID,  // page.c: invalid gorilla chain loaded from disk
    PGD_FREE_SITE_CREATE_BAD_TYPE,       // page.c: unknown page type while creating
    PGD_FREE_SITE_UNITTEST,              // test teardown - never reached in a running agent
} PGD_FREE_SITE;

void pgd_free_with_trace(PGD *pg, PGD_FREE_SITE site, const char *func);
#define pgd_free(pg, site) pgd_free_with_trace(pg, site, __FUNCTION__)

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

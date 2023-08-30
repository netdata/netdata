// SPDX-License-Identifier: GPL-3.0-or-later

#include "page.h"

typedef enum __attribute__((packed)) {
    PAGE_OPTION_ALL_VALUES_EMPTY = (1 << 0),
} PAGE_OPTIONS;

struct pgd {
    uint8_t type;           // the page type
    PAGE_OPTIONS options;   // options related to the page

    uint32_t used;          // the uses number of slots in the page
    uint32_t slots;         // the total number of slots available in the page

    uint32_t size;          // the size of data in bytes

    usec_t page_start_time_ut; // the first timestamp on the page

    uint8_t data[];         // the data of the page
};

// ----------------------------------------------------------------------------

struct {
    ARAL *aral[RRD_STORAGE_TIERS];
} pgd_alloc_globals = {};

static inline ARAL *pgd_size_lookup(size_t size) {
    for(size_t tier = 0; tier < storage_tiers ;tier++)
        if(size == tier_page_size[tier] + sizeof(PGD))
            return pgd_alloc_globals.aral[tier];

    return NULL;
}

void pgd_init(void) {
    for(size_t i = storage_tiers; i > 0 ;i--) {
        size_t tier = storage_tiers - i;

        char buf[20 + 1];
        snprintfz(buf, 20, "tier%zu-pages", tier);

        pgd_alloc_globals.aral[tier] = aral_create(
                buf,
                tier_page_size[tier] + sizeof(PGD),
                64,
                512 * (tier_page_size[tier] + sizeof(PGD)),
                pgc_aral_statistics(),
                NULL, NULL, false, false);
    }
}

static inline void *pgd_alloc_internal(size_t size) {
    ARAL *ar = pgd_size_lookup(size);
    if(ar) return aral_mallocz(ar);

    return mallocz(size);
}

static inline void pgd_free_internal(void *page, size_t size __maybe_unused) {
    ARAL *ar = pgd_size_lookup(size);
    if(ar)
        aral_freez(ar, page);
    else
        freez(page);
}

// ----------------------------------------------------------------------------

void pgd_append_point(PGD *pg,
                      usec_t point_in_time_ut,
                      NETDATA_DOUBLE n,
                      NETDATA_DOUBLE min_value,
                      NETDATA_DOUBLE max_value,
                      uint16_t count,
                      uint16_t anomaly_count,
                      SN_FLAGS flags,
                      uint32_t expected_slot) {

    if(unlikely(pg->used >= pg->slots))
        fatal("DBENGINE: attempted to write beyond page size (page type %u, slots %u, used %u, size %u)",
              pg->type, pg->slots, pg->used, pg->size);

    if(unlikely(pg->used != expected_slot))
        fatal("DBENGINE: page is not aligned to expected slot (used %u, expected %u)",
              pg->used, expected_slot);

    if(unlikely(!pg->used))
        pg->page_start_time_ut = point_in_time_ut;

    switch(pg->type) {
        case PAGE_METRICS: {
            storage_number *tier0_metric_data = (storage_number *)pg->data;
            storage_number t = pack_storage_number(n, flags);
            tier0_metric_data[pg->used++] = t;

            if((pg->options & PAGE_OPTION_ALL_VALUES_EMPTY) && does_storage_number_exist(t))
                pg->options &= ~PAGE_OPTION_ALL_VALUES_EMPTY;
        }
        break;

        case PAGE_TIER: {
            storage_number_tier1_t *tier12_metric_data = (storage_number_tier1_t *)pg->data;
            storage_number_tier1_t t;
            t.sum_value = (float) n;
            t.min_value = (float) min_value;
            t.max_value = (float) max_value;
            t.anomaly_count = anomaly_count;
            t.count = count;
            tier12_metric_data[pg->used++] = t;

            if((pg->options & PAGE_OPTION_ALL_VALUES_EMPTY) && fpclassify(n) != FP_NAN)
                pg->options &= ~PAGE_OPTION_ALL_VALUES_EMPTY;
        }
        break;

        default:
            fatal("DBENGINE: unknown page type id %d", pg->type);
            break;
    }
}

bool pgd_is_empty(PGD *pg) {
    return pg == PGD_EMPTY || !pg->used || (pg->options & PAGE_OPTION_ALL_VALUES_EMPTY);
}

PGD *pgd_create(uint8_t type, uint32_t slots) {
    uint32_t size = slots * page_type_size[type];

    internal_fatal(!size || slots == 1, "DBENGINE: invalid number of slots (%u) or page type (%u)",
                   slots, type);

    PGD *pg = pgd_alloc_internal(sizeof(PGD) + size);
    pg->type = type;
    pg->size = size;
    pg->used = 0;
    pg->slots = slots;
    pg->options = PAGE_OPTION_ALL_VALUES_EMPTY;

    return pg;
}

PGD *pgd_create_and_copy(uint8_t type, void *base, uint32_t size) {
    if(!size || size < page_type_size[type])
        return PGD_EMPTY;

    PGD *pg = pgd_alloc_internal(sizeof(PGD) + size);
    pg->type = type;
    pg->size = size;
    pg->used = size / page_type_size[type];
    pg->slots = pg->used;
    pg->options = 0;

    memcpy(pg->data, base, size);

    return pg;
}

void pgd_free(PGD *pg) {
    if(pg && pg != PGD_EMPTY)
        pgd_free_internal(pg, sizeof(PGD) + pg->size);
}

uint32_t pgd_length(PGD *pg) {
    if(pg && pg != PGD_EMPTY)
        return pg->size;

    return 0;
}

uint32_t pgd_footprint(PGD *pg) {
    if(pg && pg != PGD_EMPTY)
        return sizeof(PGD) + pg->size;

    return 0;
}

uint32_t pgd_type(PGD *pg) {
    return pg->type;
}

uint32_t pgd_slots(PGD *pg) {
    return pg->slots;
}

void *pgd_raw_data_pointer(PGD *pg) {
    if(pg && pg != PGD_EMPTY)
        return pg->data;

    return NULL;
}

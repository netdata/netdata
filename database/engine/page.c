// SPDX-License-Identifier: GPL-3.0-or-later

#include "page.h"

typedef enum __attribute__((packed)) {
    PAGE_OPTION_ALL_VALUES_EMPTY = (1 << 0),
} PAGE_OPTIONS;

struct dbengine_page_data {
    uint8_t type;           // the page type
    PAGE_OPTIONS options;   // options related to the page

    uint32_t used;          // the uses number of slots in the page
    uint32_t slots;         // the total number of slots available in the page

    uint32_t size;          // the size of data in bytes

    usec_t page_start_time_ut; // the first timestamp on the page

    uint8_t data[];         // the data of the page
};

void dbengine_page_data_append_point(DBENGINE_PAGE_DATA *pg,
                                     const usec_t point_in_time_ut,
                                     const NETDATA_DOUBLE n,
                                     const NETDATA_DOUBLE min_value,
                                     const NETDATA_DOUBLE max_value,
                                     const uint16_t count,
                                     const uint16_t anomaly_count,
                                     const SN_FLAGS flags,
                                     const uint32_t expected_slot) {

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

DBENGINE_PAGE_DATA *dbengine_page_data_create(uint8_t type, uint32_t slots) {
    uint32_t size = slots * page_type_size[type];

    internal_fatal(!size || slots == 1, "DBENGINE: invalid number of slots (%u) or page type (%u)",
                   slots, type);

    DBENGINE_PAGE_DATA *pg = dbengine_page_alloc(sizeof(DBENGINE_PAGE_DATA) + size);
    pg->type = type;
    pg->size = size;
    pg->used = 0;
    pg->slots = slots;
    pg->options = PAGE_OPTION_ALL_VALUES_EMPTY;

    return pg;
}

bool dbengine_page_data_is_empty(DBENGINE_PAGE_DATA *pg) {
    return ((!pg->used) || (pg->options & PAGE_OPTION_ALL_VALUES_EMPTY));
}

// SPDX-License-Identifier: GPL-3.0-or-later

#include "page.h"

struct dbengine_page {
    uint8_t page_type;
    uint32_t used;
    uint32_t slots;
    uint32_t data_size;
    uint8_t data[];
};

void dbengine_page_append_point(DBENGINE_PAGE *page,
                                             const usec_t point_in_time_ut,
                                             const NETDATA_DOUBLE n,
                                             const NETDATA_DOUBLE min_value,
                                             const NETDATA_DOUBLE max_value,
                                             const uint16_t count,
                                             const uint16_t anomaly_count,
                                             const SN_FLAGS flags) {

    if(unlikely(page->used >= page->slots))
        fatal("DBENGINE: attempted to write beyond page size (page type %u, slots %u, used %u, size %u)",
              page->page_type, page->slots, page->used, page->data_size);

    switch(page->page_type) {
        case PAGE_METRICS: {
            storage_number *tier0_metric_data = (void *)page->data;
            tier0_metric_data[page->used++] = pack_storage_number(n, flags);
        }
        break;

        case PAGE_TIER: {
            storage_number_tier1_t *tier12_metric_data = (void *)page->data;
            storage_number_tier1_t number_tier1;
            number_tier1.sum_value = (float) n;
            number_tier1.min_value = (float) min_value;
            number_tier1.max_value = (float) max_value;
            number_tier1.anomaly_count = anomaly_count;
            number_tier1.count = count;
            tier12_metric_data[page->used++] = number_tier1;
        }
        break;

        default:
            fatal("DBENGINE: unknown page type id %d", page->page_type);
            break;
    }
}

DBENGINE_PAGE *dbengine_page_create(uint8_t type, uint32_t slots) {
    uint32_t size = slots * page_type_size[type];

    internal_fatal(!size || slots == 1, "DBENGINE: invalid number of slots (%u) or page type (%u)",
                   slots, type);

    DBENGINE_PAGE *pg = dbengine_page_alloc(sizeof(DBENGINE_PAGE) + size);
    pg->page_type = type;
    pg->data_size = size;
    pg->slots = slots;

    return pg;
}

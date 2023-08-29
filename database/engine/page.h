// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef DBENGINE_PAGE_H
#define DBENGINE_PAGE_H

#include "rrdengine.h"

struct dbengine_page_data;
typedef struct dbengine_page_data DBENGINE_PAGE_DATA;


DBENGINE_PAGE_DATA *dbengine_page_data_create(uint8_t type, uint32_t slots);

void dbengine_page_data_append_point(DBENGINE_PAGE_DATA *pg,
                                     const usec_t point_in_time_ut,
                                     const NETDATA_DOUBLE n,
                                     const NETDATA_DOUBLE min_value,
                                     const NETDATA_DOUBLE max_value,
                                     const uint16_t count,
                                     const uint16_t anomaly_count,
                                     const SN_FLAGS flags,
                                     const uint32_t expected_slot);

bool dbengine_page_data_is_empty(DBENGINE_PAGE_DATA *pg);

#endif // DBENGINE_PAGE_H

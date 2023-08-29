// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef DBENGINE_PAGE_H
#define DBENGINE_PAGE_H

#include "rrdengine.h"

struct dbengine_page;
typedef struct dbengine_page DBENGINE_PAGE;


DBENGINE_PAGE *dbengine_page_create(uint8_t type, uint32_t slots);

void dbengine_page_append_point(DBENGINE_PAGE *page,
                                usec_t point_in_time_ut,
                                NETDATA_DOUBLE n,
                                NETDATA_DOUBLE min_value,
                                NETDATA_DOUBLE max_value,
                                uint16_t count,
                                uint16_t anomaly_count,
                                SN_FLAGS flags);

#endif // DBENGINE_PAGE_H

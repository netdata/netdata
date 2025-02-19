// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDDIM_COLLECTION_H
#define NETDATA_RRDDIM_COLLECTION_H

#include "rrddim.h"

void store_metric_at_tier(RRDDIM *rd, size_t tier, struct rrddim_tier *t, STORAGE_POINT sp, usec_t now_ut);
void store_metric_collection_completed(void);

#ifdef NETDATA_LOG_COLLECTION_ERRORS
#define rrddim_store_metric(rd, point_end_time_ut, n, flags) rrddim_store_metric_with_trace(rd, point_end_time_ut, n, flags, __FUNCTION__)
void rrddim_store_metric_with_trace(RRDDIM *rd, usec_t point_end_time_ut, NETDATA_DOUBLE n, SN_FLAGS flags, const char *function);
#else
void rrddim_store_metric(RRDDIM *rd, usec_t point_end_time_ut, NETDATA_DOUBLE n, SN_FLAGS flags);
#endif

void store_metric_at_tier_flush_last_completed(RRDDIM *rd, size_t tier, struct rrddim_tier *t);

#endif //NETDATA_RRDDIM_COLLECTION_H

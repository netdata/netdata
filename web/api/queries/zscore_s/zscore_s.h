// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERY_ZSCORE_S_H
#define NETDATA_API_QUERY_ZSCORE_S_H

#include "../query.h"
#include "../rrdr.h"

extern void *stats_create_zscore(RRDR *r);
extern void stats_reset_zscore(RRDR *r, int index);
extern void stats_free_zscore(RRDR *r, int index);
extern void stats_add_zscore(RRDR *r, calculated_number value, int index);
extern calculated_number stats_flush_zscore(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr, int index);

#endif //NETDATA_API_QUERY_ZSCORE_S_H
// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERY_INCREMENTAL_SUM_H
#define NETDATA_API_QUERY_INCREMENTAL_SUM_H

#include "../query.h"
#include "../rrdr.h"

extern void grouping_create_incremental_sum(RRDR *r, const char *options __maybe_unused);
extern void grouping_reset_incremental_sum(RRDR *r);
extern void grouping_free_incremental_sum(RRDR *r);
extern void grouping_add_incremental_sum(RRDR *r, NETDATA_DOUBLE value);
extern NETDATA_DOUBLE grouping_flush_incremental_sum(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr);

#endif //NETDATA_API_QUERY_INCREMENTAL_SUM_H

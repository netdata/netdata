// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERIES_MEDIAN_H
#define NETDATA_API_QUERIES_MEDIAN_H

#include "../query.h"
#include "../rrdr.h"

extern void grouping_create_median(RRDR *r, const char *options __maybe_unused);
extern void grouping_reset_median(RRDR *r);
extern void grouping_free_median(RRDR *r);
extern void grouping_add_median(RRDR *r, NETDATA_DOUBLE value);
extern NETDATA_DOUBLE grouping_flush_median(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr);

#endif //NETDATA_API_QUERIES_MEDIAN_H

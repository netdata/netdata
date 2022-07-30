// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERIES_PERCENTILE_H
#define NETDATA_API_QUERIES_PERCENTILE_H

#include "../query.h"
#include "../rrdr.h"

extern void grouping_create_percentile(RRDR *r, const char *options __maybe_unused);
extern void grouping_reset_percentile(RRDR *r);
extern void grouping_free_percentile(RRDR *r);
extern void grouping_add_percentile(RRDR *r, NETDATA_DOUBLE value);
extern NETDATA_DOUBLE grouping_flush_percentile(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr);

#endif //NETDATA_API_QUERIES_PERCENTILE_H

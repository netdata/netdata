// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERY_COUNTIF_H
#define NETDATA_API_QUERY_COUNTIF_H

#include "../query.h"
#include "../rrdr.h"

extern void grouping_create_countif(RRDR *r, const char *options __maybe_unused);
extern void grouping_reset_countif(RRDR *r);
extern void grouping_free_countif(RRDR *r);
extern void grouping_add_countif(RRDR *r, calculated_number value);
extern calculated_number grouping_flush_countif(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr);

#endif //NETDATA_API_QUERY_COUNTIF_H

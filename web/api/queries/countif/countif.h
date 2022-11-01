// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERY_COUNTIF_H
#define NETDATA_API_QUERY_COUNTIF_H

#include "../query.h"
#include "../rrdr.h"

void grouping_create_countif(RRDR *r, const char *options __maybe_unused);
void grouping_reset_countif(RRDR *r);
void grouping_free_countif(RRDR *r);
void grouping_add_countif(RRDR *r, NETDATA_DOUBLE value);
NETDATA_DOUBLE grouping_flush_countif(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr);

#endif //NETDATA_API_QUERY_COUNTIF_H

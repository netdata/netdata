// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERY_MIN_H
#define NETDATA_API_QUERY_MIN_H

#include "../query.h"
#include "../rrdr.h"

void grouping_create_min(RRDR *r, const char *options __maybe_unused);
void grouping_reset_min(RRDR *r);
void grouping_free_min(RRDR *r);
void grouping_add_min(RRDR *r, NETDATA_DOUBLE value);
NETDATA_DOUBLE grouping_flush_min(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr);

#endif //NETDATA_API_QUERY_MIN_H

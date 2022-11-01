// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERY_AVERAGE_H
#define NETDATA_API_QUERY_AVERAGE_H

#include "../query.h"
#include "../rrdr.h"

void grouping_create_average(RRDR *r, const char *options __maybe_unused);
void grouping_reset_average(RRDR *r);
void grouping_free_average(RRDR *r);
void grouping_add_average(RRDR *r, NETDATA_DOUBLE value);
NETDATA_DOUBLE grouping_flush_average(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr);

#endif //NETDATA_API_QUERY_AVERAGE_H

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERY_MAX_H
#define NETDATA_API_QUERY_MAX_H

#include "../query.h"
#include "../rrdr.h"

void grouping_create_max(RRDR *r, const char *options __maybe_unused);
void grouping_reset_max(RRDR *r);
void grouping_free_max(RRDR *r);
void grouping_add_max(RRDR *r, NETDATA_DOUBLE value);
NETDATA_DOUBLE grouping_flush_max(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr);

#endif //NETDATA_API_QUERY_MAX_H

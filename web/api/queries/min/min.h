// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERY_MIN_H
#define NETDATA_API_QUERY_MIN_H

#include "../query.h"
#include "../rrdr.h"

extern void *grouping_init_min(RRDR *r);
extern void grouping_reset_min(RRDR *r);
extern void grouping_free_min(RRDR *r);
extern void grouping_add_min(RRDR *r, calculated_number value);
extern calculated_number grouping_flush_min(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr);

#endif //NETDATA_API_QUERY_MIN_H

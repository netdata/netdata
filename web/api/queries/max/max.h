// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERY_MAX_H
#define NETDATA_API_QUERY_MAX_H

#include "../query.h"
#include "../rrdr.h"

extern void *grouping_init_max(RRDR *r);
extern void grouping_reset_max(RRDR *r);
extern void grouping_free_max(RRDR *r);
extern void grouping_add_max(RRDR *r, calculated_number value);
extern calculated_number grouping_flush_max(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr);

#endif //NETDATA_API_QUERY_MAX_H

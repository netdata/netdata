// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERY_AVERAGE_H
#define NETDATA_API_QUERY_AVERAGE_H

#include "../query.h"
#include "../rrdr.h"

extern void *grouping_create_average(RRDR *r);
extern void grouping_reset_average(RRDR *r, unsigned int index);
extern void grouping_free_average(RRDR *r, unsigned int index);
extern void grouping_add_average(RRDR *r, calculated_number value, unsigned int index);
extern calculated_number grouping_flush_average(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr, unsigned int index);

#endif //NETDATA_API_QUERY_AVERAGE_H

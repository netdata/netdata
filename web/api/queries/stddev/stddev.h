// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERIES_STDDEV_H
#define NETDATA_API_QUERIES_STDDEV_H

#include "../query.h"
#include "../rrdr.h"

extern void *grouping_create_stddev(RRDR *r);
extern void grouping_reset_stddev(RRDR *r);
extern void grouping_free_stddev(RRDR *r);
extern void grouping_add_stddev(RRDR *r, calculated_number value);
extern calculated_number grouping_flush_stddev(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr);
extern calculated_number grouping_flush_coefficient_of_variation(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr);
// extern calculated_number grouping_flush_mean(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr);
// extern calculated_number grouping_flush_variance(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr);

#endif //NETDATA_API_QUERIES_STDDEV_H

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERIES_DES_H
#define NETDATA_API_QUERIES_DES_H

#include "../query.h"
#include "../rrdr.h"

extern void *grouping_init_des(RRDR *r);
extern void grouping_reset_des(RRDR *r);
extern void grouping_free_des(RRDR *r);
extern void grouping_add_des(RRDR *r, calculated_number value);
extern calculated_number grouping_flush_des(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr);

#endif //NETDATA_API_QUERIES_DES_H

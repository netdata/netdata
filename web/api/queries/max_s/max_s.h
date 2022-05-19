// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERY_MAX_S_H
#define NETDATA_API_QUERY_MAX_S_H

#include "../query.h"
#include "../rrdr.h"

extern void *stats_create_max(RRDR *r);
extern void stats_reset_max(RRDR *r, int index);
extern void stats_free_max(RRDR *r, int index);
extern void stats_add_max(RRDR *r, calculated_number value, int index);
extern calculated_number stats_flush_max(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr, int index);

#endif //NETDATA_API_QUERY_MAX_S_H

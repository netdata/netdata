// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERIES_PERCENTILE_H
#define NETDATA_API_QUERIES_PERCENTILE_H

#include "../query.h"
#include "../rrdr.h"

void grouping_create_percentile25(RRDR *r, const char *options);
void grouping_create_percentile50(RRDR *r, const char *options);
void grouping_create_percentile75(RRDR *r, const char *options);
void grouping_create_percentile80(RRDR *r, const char *options);
void grouping_create_percentile90(RRDR *r, const char *options);
void grouping_create_percentile95(RRDR *r, const char *options);
void grouping_create_percentile97(RRDR *r, const char *options);
void grouping_create_percentile98(RRDR *r, const char *options);
void grouping_create_percentile99(RRDR *r, const char *options );
void grouping_reset_percentile(RRDR *r);
void grouping_free_percentile(RRDR *r);
void grouping_add_percentile(RRDR *r, NETDATA_DOUBLE value);
NETDATA_DOUBLE grouping_flush_percentile(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr);

#endif //NETDATA_API_QUERIES_PERCENTILE_H

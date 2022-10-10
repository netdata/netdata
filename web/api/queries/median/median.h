// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERIES_MEDIAN_H
#define NETDATA_API_QUERIES_MEDIAN_H

#include "../query.h"
#include "../rrdr.h"

void grouping_create_median(RRDR *r, const char *options);
void grouping_create_trimmed_median1(RRDR *r, const char *options);
void grouping_create_trimmed_median2(RRDR *r, const char *options);
void grouping_create_trimmed_median3(RRDR *r, const char *options);
void grouping_create_trimmed_median5(RRDR *r, const char *options);
void grouping_create_trimmed_median10(RRDR *r, const char *options);
void grouping_create_trimmed_median15(RRDR *r, const char *options);
void grouping_create_trimmed_median20(RRDR *r, const char *options);
void grouping_create_trimmed_median25(RRDR *r, const char *options);
void grouping_reset_median(RRDR *r);
void grouping_free_median(RRDR *r);
void grouping_add_median(RRDR *r, NETDATA_DOUBLE value);
NETDATA_DOUBLE grouping_flush_median(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr);

#endif //NETDATA_API_QUERIES_MEDIAN_H

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERIES_TRIMMED_MEAN_H
#define NETDATA_API_QUERIES_TRIMMED_MEAN_H

#include "../query.h"
#include "../rrdr.h"

extern void grouping_create_trimmed_mean1(RRDR *r, const char *options);
extern void grouping_create_trimmed_mean2(RRDR *r, const char *options);
extern void grouping_create_trimmed_mean3(RRDR *r, const char *options);
extern void grouping_create_trimmed_mean5(RRDR *r, const char *options);
extern void grouping_create_trimmed_mean10(RRDR *r, const char *options);
extern void grouping_create_trimmed_mean15(RRDR *r, const char *options);
extern void grouping_create_trimmed_mean20(RRDR *r, const char *options);
extern void grouping_create_trimmed_mean25(RRDR *r, const char *options);
extern void grouping_reset_trimmed_mean(RRDR *r);
extern void grouping_free_trimmed_mean(RRDR *r);
extern void grouping_add_trimmed_mean(RRDR *r, NETDATA_DOUBLE value);
extern NETDATA_DOUBLE grouping_flush_trimmed_mean(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr);

#endif //NETDATA_API_QUERIES_TRIMMED_MEAN_H

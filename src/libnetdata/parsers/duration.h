// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DURATION_H
#define NETDATA_DURATION_H

#include "parsers.h"

// duration (string to number)
bool duration_str_to_nsec_t(const char *duration, snsec_t *result, const char *default_unit);
bool duration_str_to_usec_t(const char *duration, susec_t *result);
bool duration_str_to_time_t(const char *duration, stime_t *result);
bool duration_str_to_days(const char *duration, int *result);

// duration (number to string)
ssize_t duration_snprintf_from_nsec_t(char *dst, size_t dst_size, snsec_t nsec, const char *minimum_unit);
ssize_t duration_snprintf_from_usec_t(char *dst, size_t size, susec_t value);
ssize_t duration_snprintf_from_time_t(char *dst, size_t size, stime_t value);
ssize_t duration_snprintf_from_days(char *dst, size_t size, int value);

// round nsec_t to given unit
int64_t nsec_to_unit(snsec_t nsec, const char *unit);

bool duration_str_to_seconds(const char *str, int *result);

#endif //NETDATA_DURATION_H

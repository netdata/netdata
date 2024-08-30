// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DURATION_H
#define NETDATA_DURATION_H

#include "parsers.h"

// duration (string to number)
nsec_t duration_str_to_nsec_t(const char *duration, nsec_t default_value, const char *default_unit);
usec_t duration_str_to_usec_t(const char *duration, usec_t default_value);
time_t duration_str_to_time_t(const char *duration, time_t default_value);
unsigned duration_str_to_days(const char *duration, unsigned default_value);

// duration (number to string)
size_t duration_str_from_nsec_t(char *dst, size_t dst_size, nsec_t nsec, const char *minimum_unit);
size_t duration_str_from_usec_t(char *dst, size_t size, usec_t value);
size_t duration_str_from_time_t(char *dst, size_t size, time_t value);
size_t duration_str_from_days(char *dst, size_t size, unsigned value);

// found nsec_t to specific unit
uint64_t duration_to_unit(nsec_t value, const char *unit);

#endif //NETDATA_DURATION_H

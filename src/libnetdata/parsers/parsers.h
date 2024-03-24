// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PARSERS_H
#define NETDATA_PARSERS_H

#include "../libnetdata.h"

nsec_t duration_to_nsec_t(const char *duration, nsec_t default_value, const char *default_unit);
usec_t duration_to_usec_t(const char *duration, usec_t default_value, const char *default_unit);
time_t duration_to_time_t(const char *duration, time_t default_value, const char *default_unit);

// return number of bytes written to dst
size_t duration_from_nsec_t(char *dst, size_t size, nsec_t value, const char *default_unit);
size_t duration_from_usec_t(char *dst, size_t size, usec_t value, const char *default_unit);
size_t duration_from_time_t(char *dst, size_t size, time_t value, const char *default_unit);

#endif //NETDATA_PARSERS_H

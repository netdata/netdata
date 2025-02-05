// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef LIBNETDATA_PARSERS_DURATION_H
#define LIBNETDATA_PARSERS_DURATION_H

#include "parsers.h"

int64_t duration_round_to_resolution(int64_t value, int64_t resolution);

// duration (string to number)
bool duration_parse(const char *duration, int64_t *result, const char *default_unit, const char *output_unit);
#define duration_parse_nsec_t(duration, ns_ptr) duration_parse(duration, ns_ptr, "ns", "ns")
#define duration_parse_usec_t(duration, us_ptr) duration_parse(duration, us_ptr, "us", "us")
#define duration_parse_msec_t(duration, ms_ptr) duration_parse(duration, ms_ptr, "ms", "ms")
#define duration_parse_time_t(duration, secs_ptr) duration_parse(duration, secs_ptr, "s", "s")
#define duration_parse_mins(duration, mins_ptr) duration_parse(duration, mins_ptr, "m", "m")
#define duration_parse_hours(duration, hours_ptr) duration_parse(duration, hours_ptr, "h", "h")
#define duration_parse_days(duration, days_ptr) duration_parse(duration, days_ptr, "d", "d")

// duration (number to string)
ssize_t duration_snprintf(char *dst, size_t dst_size, int64_t value, const char *unit, bool add_spaces);
#define duration_snprintf_nsec_t(dst, dst_size, ns) duration_snprintf(dst, dst_size, ns, "ns", false)
#define duration_snprintf_usec_t(dst, dst_size, us) duration_snprintf(dst, dst_size, us, "us", false)
#define duration_snprintf_msec_t(dst, dst_size, ms) duration_snprintf(dst, dst_size, ms, "ms", false)
#define duration_snprintf_time_t(dst, dst_size, secs) duration_snprintf(dst, dst_size, secs, "s", false)
#define duration_snprintf_mins(dst, dst_size, mins) duration_snprintf(dst, dst_size, mins, "m", false)
#define duration_snprintf_hours(dst, dst_size, hours) duration_snprintf(dst, dst_size, hours, "h", false)
#define duration_snprintf_days(dst, dst_size, days) duration_snprintf(dst, dst_size, days, "d", false)

bool duration_parse_seconds(const char *str, int *result);

#endif //LIBNETDATA_PARSERS_DURATION_H

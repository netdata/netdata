// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DURATION_H
#define NETDATA_DURATION_H

#include "parsers.h"

int64_t duration_round_to_resolution(int64_t value, int64_t resolution);

// duration (string to number)
bool duration_parse_nsec_t(const char *duration, snsec_t *ns);
bool duration_parse_usec_t(const char *duration, susec_t *us);
bool duration_parse_msec_t(const char *duration, smsec_t *ms);
bool duration_parse_time_t(const char *duration, stime_t *secs);
bool duration_parse_mins(const char *duration, int *mins);
bool duration_parse_hours(const char *duration, int *hours);
bool duration_parse_days(const char *duration, int *days);

// duration (number to string)
ssize_t duration_snprintf_nsec_t(char *dst, size_t size, snsec_t ns);
ssize_t duration_snprintf_usec_t(char *dst, size_t size, susec_t us);
ssize_t duration_snprintf_msec_t(char *dst, size_t size, smsec_t ms);
ssize_t duration_snprintf_time_t(char *dst, size_t size, stime_t secs);
ssize_t duration_snprintf_mins(char *dst, size_t size, int mins);
ssize_t duration_snprintf_hours(char *dst, size_t size, int hours);
ssize_t duration_snprintf_days(char *dst, size_t size, int days);

// round nsec_t to given unit
int64_t nsec_to_unit(snsec_t nsec, const char *unit);

bool duration_parse_seconds(const char *str, int *result);

#endif //NETDATA_DURATION_H

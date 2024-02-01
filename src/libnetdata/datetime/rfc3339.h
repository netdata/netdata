// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#ifndef NETDATA_RFC3339_H
#define NETDATA_RFC3339_H

#define RFC3339_MAX_LENGTH 36
size_t rfc3339_datetime_ut(char *buffer, size_t len, usec_t now_ut, size_t fractional_digits, bool utc);
usec_t rfc3339_parse_ut(const char *rfc3339, char **endptr);

#endif //NETDATA_RFC3339_H

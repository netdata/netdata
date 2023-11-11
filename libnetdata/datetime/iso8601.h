// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#ifndef NETDATA_ISO8601_H
#define NETDATA_ISO8601_H

#define ISO8601_MAX_LENGTH 64
size_t iso8601_datetime_utc_ut(char *buffer, size_t len, usec_t now_ut);
size_t iso8601_datetime_usec_utc_ut(char *buffer, size_t len, usec_t now_ut);
size_t iso8601_datetime_with_local_timezone_ut(char *buffer, size_t len, usec_t now_ut);

#endif //NETDATA_ISO8601_H

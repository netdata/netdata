// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#ifndef NETDATA_ISO8601_H
#define NETDATA_ISO8601_H

typedef enum __attribute__((__packed__)) {
    ISO8601_UTC             = (1 << 0),
    ISO8601_LOCAL_TIMEZONE  = (1 << 1),
    ISO8601_MILLISECONDS    = (1 << 2),
    ISO8601_MICROSECONDS    = (1 << 3),
} ISO8601_OPTIONS;

#define ISO8601_MAX_LENGTH 64
size_t iso8601_datetime_ut(char *buffer, size_t len, usec_t now_ut, ISO8601_OPTIONS options);

#endif //NETDATA_ISO8601_H

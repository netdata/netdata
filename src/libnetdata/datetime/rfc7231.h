// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#ifndef NETDATA_RFC7231_H
#define NETDATA_RFC7231_H

#define RFC7231_MAX_LENGTH 30
size_t rfc7231_datetime(char *buffer, size_t len, time_t now_t);
size_t rfc7231_datetime_ut(char *buffer, size_t len, usec_t now_ut);

#endif //NETDATA_RFC7231_H

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_FORMATTER_SSV_H
#define NETDATA_API_FORMATTER_SSV_H

#include "../rrd2json.h"

extern void rrdr2ssv(RRDR *r, BUFFER *wb, RRDR_OPTIONS options, const char *prefix, const char *separator, const char *suffix);

#endif //NETDATA_API_FORMATTER_SSV_H

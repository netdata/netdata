// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_FORMATTER_JSON_H
#define NETDATA_API_FORMATTER_JSON_H

#include "../rrd2json.h"

extern void rrdr2json(RRDR *r, BUFFER *wb, RRDR_OPTIONS options, int datatable);

#endif //NETDATA_API_FORMATTER_JSON_H

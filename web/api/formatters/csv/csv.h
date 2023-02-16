// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_FORMATTER_CSV_H
#define NETDATA_API_FORMATTER_CSV_H

#include "web/api/queries/rrdr.h"

void rrdr2csv(RRDR *r, BUFFER *wb, uint32_t format, RRDR_OPTIONS options, const char *startline, const char *separator, const char *endline, const char *betweenlines);

#include "../rrd2json.h"

#endif //NETDATA_API_FORMATTER_CSV_H

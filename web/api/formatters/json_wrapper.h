// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_FORMATTER_JSON_WRAPPER_H
#define NETDATA_API_FORMATTER_JSON_WRAPPER_H

#include "rrd2json.h"

extern void rrdr_json_wrapper_begin(RRDR *r, BUFFER *wb, uint32_t format, RRDR_OPTIONS options, int string_value);
extern void rrdr_json_wrapper_end(RRDR *r, BUFFER *wb, uint32_t format, uint32_t options, int string_value);

#endif //NETDATA_API_FORMATTER_JSON_WRAPPER_H

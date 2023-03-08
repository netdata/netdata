// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_FORMATTER_JSON_WRAPPER_H
#define NETDATA_API_FORMATTER_JSON_WRAPPER_H

#include "rrd2json.h"
#include "web/api/queries/query.h"

typedef void (*wrapper_begin_t)(RRDR *r, BUFFER *wb, DATASOURCE_FORMAT format, RRDR_OPTIONS options, RRDR_TIME_GROUPING group_method);
typedef void (*wrapper_end_t)(RRDR *r, BUFFER *wb, DATASOURCE_FORMAT format, RRDR_OPTIONS options);

void rrdr_json_wrapper_begin(RRDR *r, BUFFER *wb, DATASOURCE_FORMAT format, RRDR_OPTIONS options, RRDR_TIME_GROUPING group_method);
void rrdr2json_v2(RRDR *r, BUFFER *wb, DATASOURCE_FORMAT format, RRDR_OPTIONS options);
void rrdr_json_wrapper_end(RRDR *r, BUFFER *wb, DATASOURCE_FORMAT format, RRDR_OPTIONS options);

void rrdr_json_wrapper_begin2(RRDR *r, BUFFER *wb, DATASOURCE_FORMAT format, RRDR_OPTIONS options, RRDR_TIME_GROUPING group_method);

#endif //NETDATA_API_FORMATTER_JSON_WRAPPER_H

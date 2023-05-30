// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_FORMATTER_JSON_WRAPPER_H
#define NETDATA_API_FORMATTER_JSON_WRAPPER_H

#include "rrd2json.h"
#include "web/api/queries/query.h"

typedef void (*wrapper_begin_t)(RRDR *r, BUFFER *wb);
typedef void (*wrapper_end_t)(RRDR *r, BUFFER *wb);

void rrdr_json_wrapper_begin(RRDR *r, BUFFER *wb);
void rrdr_json_wrapper_end(RRDR *r, BUFFER *wb);

void rrdr_json_wrapper_begin2(RRDR *r, BUFFER *wb);
void rrdr_json_wrapper_end2(RRDR *r, BUFFER *wb);

struct query_versions;
void version_hashes_api_v2(BUFFER *wb, struct query_versions *versions);

#endif //NETDATA_API_FORMATTER_JSON_WRAPPER_H

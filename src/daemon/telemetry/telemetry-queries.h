// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_TELEMETRY_QUERIES_H
#define NETDATA_TELEMETRY_QUERIES_H

#include "daemon/common.h"

void telemetry_queries_ml_query_completed(size_t points_read);
void telemetry_queries_exporters_query_completed(size_t points_read);
void telemetry_queries_backfill_query_completed(size_t points_read);
void telemetry_queries_rrdr_query_completed(size_t queries, uint64_t db_points_read, uint64_t result_points_generated, QUERY_SOURCE query_source);

#if defined(TELEMETRY_INTERNALS)
void telemetry_queries_do(bool extended);
#endif

#endif //NETDATA_TELEMETRY_QUERIES_H

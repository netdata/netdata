// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PULSE_QUERIES_H
#define NETDATA_PULSE_QUERIES_H

#include "daemon/common.h"

void pulse_queries_ml_query_completed(size_t points_read);
void pulse_queries_exporters_query_completed(size_t points_read);
void pulse_queries_backfill_query_completed(size_t points_read);
void pulse_queries_rrdr_query_completed(size_t queries, uint64_t db_points_read, uint64_t result_points_generated, QUERY_SOURCE query_source);

#if defined(PULSE_INTERNALS)
void pulse_queries_do(bool extended);
#endif

#endif //NETDATA_PULSE_QUERIES_H

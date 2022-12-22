// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_GLOBAL_STATISTICS_H
#define NETDATA_GLOBAL_STATISTICS_H 1

#include "database/rrd.h"

// ----------------------------------------------------------------------------
// global statistics

void global_statistics_ml_query_completed(size_t points_read);
void global_statistics_exporters_query_completed(size_t points_read);
void global_statistics_backfill_query_completed(size_t points_read);
void global_statistics_rrdr_query_completed(size_t queries, uint64_t db_points_read, uint64_t result_points_generated, QUERY_SOURCE query_source);
void global_statistics_sqlite3_query_completed(bool success, bool busy, bool locked);
void global_statistics_sqlite3_row_completed(void);
void global_statistics_rrdset_done_chart_collection_completed(size_t *points_read_per_tier_array);

void global_statistics_web_request_completed(uint64_t dt,
                                             uint64_t bytes_received,
                                             uint64_t bytes_sent,
                                             uint64_t content_size,
                                             uint64_t compressed_content_size);

uint64_t global_statistics_web_client_connected(void);
void global_statistics_web_client_disconnected(void);

extern bool global_statistics_enabled;

#endif /* NETDATA_GLOBAL_STATISTICS_H */

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_GLOBAL_STATISTICS_H
#define NETDATA_GLOBAL_STATISTICS_H 1

#include "common.h"

// ----------------------------------------------------------------------------
// global statistics

extern void rrdr_query_completed(uint64_t db_points_read, uint64_t result_points_generated);
extern void sqlite3_query_completed(bool success, bool busy, bool locked);
extern void sqlite3_row_completed(void);

extern void finished_web_request_statistics(uint64_t dt,
                                     uint64_t bytes_received,
                                     uint64_t bytes_sent,
                                     uint64_t content_size,
                                     uint64_t compressed_content_size);

extern uint64_t web_client_connected(void);
extern void web_client_disconnected(void);

#endif /* NETDATA_GLOBAL_STATISTICS_H */

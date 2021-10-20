// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_GLOBAL_STATISTICS_H
#define NETDATA_GLOBAL_STATISTICS_H 1

#include "common.h"

// ----------------------------------------------------------------------------
// global statistics

#define NETDATA_PLUGIN_HOOK_GLOBAL_STATISTICS                                                                          \
    {.name = "GLOBAL_STATS",                                                                                           \
     .config_section = NULL,                                                                                           \
     .config_name = NULL,                                                                                              \
     .enabled = 1,                                                                                                     \
     .thread = NULL,                                                                                                   \
     .init_routine = NULL,                                                                                             \
     .start_routine = global_statistics_main},

extern void *global_statistics_main(void *ptr);

extern void rrdr_query_completed(uint64_t db_points_read, uint64_t result_points_generated);

extern void finished_web_request_statistics(uint64_t dt,
                                     uint64_t bytes_received,
                                     uint64_t bytes_sent,
                                     uint64_t content_size,
                                     uint64_t compressed_content_size);

extern uint64_t web_client_connected(void);
extern void web_client_disconnected(void);
extern void global_statistics_charts(void);

#endif /* NETDATA_GLOBAL_STATISTICS_H */

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_GLOBAL_STATISTICS_H
#define NETDATA_GLOBAL_STATISTICS_H 1

#include "common.h"

// ----------------------------------------------------------------------------
// global statistics

struct global_statistics {
    volatile uint16_t connected_clients;

    volatile uint64_t web_requests;
    volatile uint64_t web_usec;
    volatile uint64_t web_usec_max;
    volatile uint64_t bytes_received;
    volatile uint64_t bytes_sent;
    volatile uint64_t content_size;
    volatile uint64_t compressed_content_size;

    volatile uint64_t web_client_count;
};

extern volatile struct global_statistics global_statistics;

extern void global_statistics_lock(void);
extern void global_statistics_unlock(void);
extern void finished_web_request_statistics(uint64_t dt,
                                     uint64_t bytes_received,
                                     uint64_t bytes_sent,
                                     uint64_t content_size,
                                     uint64_t compressed_content_size);

extern uint64_t web_client_connected(void);
extern void web_client_disconnected(void);

#define GLOBAL_STATS_RESET_WEB_USEC_MAX 0x01
extern void global_statistics_copy(struct global_statistics *gs, uint8_t options);
extern void global_statistics_charts(void);

#endif /* NETDATA_GLOBAL_STATISTICS_H */

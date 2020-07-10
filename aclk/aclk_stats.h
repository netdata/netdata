// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ACLK_STATS_H
#define NETDATA_ACLK_STATS_H

#include "../daemon/common.h"
#include "libnetdata/libnetdata.h"
#include "aclk_common.h"

#define ACLK_STATS_THREAD_NAME "ACLK_Stats"

extern netdata_mutex_t aclk_stats_mutex;

#define ACLK_STATS_LOCK netdata_mutex_lock(&aclk_stats_mutex)
#define ACLK_STATS_UNLOCK netdata_mutex_unlock(&aclk_stats_mutex)

extern int aclk_stats_enabled;

struct aclk_stats_thread {
    netdata_thread_t *thread;
    int query_thread_count;
};

// preserve between samples
struct aclk_metrics {
    volatile uint8_t online;
};

// reset to 0 on every sample
extern struct aclk_metrics_per_sample {
    /* in the unlikely event of ACLK disconnecting
       and reconnecting under 1 sampling rate
       we want to make sure we record the disconnection
       despite it being then seemingly longer in graph */
    volatile uint8_t offline_during_sample;

    volatile uint32_t queries_queued;
    volatile uint32_t queries_dispatched;

#ifdef NETDATA_INTERNAL_CHECKS
    volatile uint32_t latency_max;
    volatile uint32_t latency_total;
    volatile uint32_t latency_count;
#endif

    volatile uint32_t write_q_added;
    volatile uint32_t write_q_consumed;

    volatile uint32_t read_q_added;
    volatile uint32_t read_q_consumed;

    volatile uint32_t cloud_req_recvd;
    volatile uint32_t cloud_req_err;

    volatile uint32_t cloud_q_process_total;
    volatile uint32_t cloud_q_process_count;
    volatile uint32_t cloud_q_process_max;
} aclk_metrics_per_sample;

extern uint32_t *aclk_queries_per_thread;

void *aclk_stats_main_thread(void *ptr);
void aclk_stats_thread_cleanup();
void aclk_stats_upd_online(int online);

#endif /* NETDATA_ACLK_STATS_H */

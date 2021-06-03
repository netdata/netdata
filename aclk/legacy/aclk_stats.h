// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ACLK_STATS_H
#define NETDATA_ACLK_STATS_H

#include "daemon/common.h"
#include "libnetdata/libnetdata.h"
#include "aclk_common.h"

#define ACLK_STATS_THREAD_NAME "ACLK_Stats"

extern netdata_mutex_t legacy_aclk_stats_mutex;

#define LEGACY_ACLK_STATS_LOCK netdata_mutex_lock(&legacy_aclk_stats_mutex)
#define LEGACY_ACLK_STATS_UNLOCK netdata_mutex_unlock(&legacy_aclk_stats_mutex)

struct aclk_stats_thread {
    netdata_thread_t *thread;
    int query_thread_count;
};

// preserve between samples
struct legacy_aclk_metrics {
    volatile uint8_t online;
};

//mat = max average total
struct aclk_metric_mat_data {
    volatile uint32_t total;
    volatile uint32_t count;
    volatile uint32_t max;
};

//mat = max average total
struct aclk_metric_mat {
    char *name;
    char *title;
    RRDSET *st;
    RRDDIM *rd_avg;
    RRDDIM *rd_max;
    RRDDIM *rd_total;
    long prio;
    char *unit;
};

extern struct aclk_mat_metrics {
#ifdef NETDATA_INTERNAL_CHECKS
    struct aclk_metric_mat latency;
#endif
    struct aclk_metric_mat cloud_q_db_query_time;
    struct aclk_metric_mat cloud_q_recvd_to_processed;
} aclk_mat_metrics;

void legacy_aclk_metric_mat_update(struct aclk_metric_mat_data *metric, usec_t measurement);

#define ACLK_STATS_CLOUD_REQ_TYPE_CNT 7
// if you change update cloud_req_type_names

int aclk_cloud_req_type_to_idx(const char *name);

// reset to 0 on every sample
extern struct legacy_aclk_metrics_per_sample {
    /* in the unlikely event of ACLK disconnecting
       and reconnecting under 1 sampling rate
       we want to make sure we record the disconnection
       despite it being then seemingly longer in graph */
    volatile uint8_t offline_during_sample;

    volatile uint32_t queries_queued;
    volatile uint32_t queries_dispatched;

    volatile uint32_t write_q_added;
    volatile uint32_t write_q_consumed;

    volatile uint32_t read_q_added;
    volatile uint32_t read_q_consumed;

    volatile uint32_t cloud_req_ok;
    volatile uint32_t cloud_req_err;

    volatile uint16_t cloud_req_v1;
    volatile uint16_t cloud_req_v2;

    volatile uint16_t cloud_req_by_type[ACLK_STATS_CLOUD_REQ_TYPE_CNT];

#ifdef NETDATA_INTERNAL_CHECKS
    struct aclk_metric_mat_data latency;
#endif
    struct aclk_metric_mat_data cloud_q_db_query_time;
    struct aclk_metric_mat_data cloud_q_recvd_to_processed;
} legacy_aclk_metrics_per_sample;

extern uint32_t *legacy_aclk_queries_per_thread;
extern struct rusage *rusage_per_thread;

void *legacy_aclk_stats_main_thread(void *ptr);
void legacy_aclk_stats_thread_cleanup();
void legacy_aclk_stats_upd_online(int online);

#endif /* NETDATA_ACLK_STATS_H */

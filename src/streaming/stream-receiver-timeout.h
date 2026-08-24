// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_RECEIVER_TIMEOUT_H
#define NETDATA_STREAM_RECEIVER_TIMEOUT_H

#include "libnetdata/libnetdata.h"

#define STREAM_RECEIVER_IDLE_TIMEOUT_MIN_SECONDS 600ULL
#define STREAM_RECEIVER_UPDATE_EVERY_MAX_SECONDS 86400U

typedef struct stream_receiver_timeout_chart {
    uint32_t update_every_s;
    uint32_t generation;
} STREAM_RECEIVER_TIMEOUT_CHART;

typedef struct stream_receiver_timeout {
    // Owned by the host. The lock protects mutation; readers use the published timeout.
    SPINLOCK spinlock;
    Pvoid_t intervals; // chart count by update interval for the current connection
    uint32_t generation;
    uint32_t provisional_update_every_s;
    uint32_t minimum_update_every_s;
    uint32_t learned_update_every_s;
    uint64_t effective_timeout_s __attribute__((aligned(8)));
} STREAM_RECEIVER_TIMEOUT;

void stream_receiver_timeout_init(STREAM_RECEIVER_TIMEOUT *timeout);
void stream_receiver_timeout_connection_start(STREAM_RECEIVER_TIMEOUT *timeout, int64_t handshake_update_every_s);
void stream_receiver_timeout_destroy(STREAM_RECEIVER_TIMEOUT *timeout);

void stream_receiver_timeout_chart_track(
    STREAM_RECEIVER_TIMEOUT *timeout,
    STREAM_RECEIVER_TIMEOUT_CHART *chart,
    int64_t update_every_s,
    bool active);
void stream_receiver_timeout_chart_forget(
    STREAM_RECEIVER_TIMEOUT *timeout,
    STREAM_RECEIVER_TIMEOUT_CHART *chart);

uint64_t stream_receiver_timeout_seconds(const STREAM_RECEIVER_TIMEOUT *timeout);
usec_t stream_receiver_timeout_usec(const STREAM_RECEIVER_TIMEOUT *timeout);
bool stream_receiver_timeout_expired(const STREAM_RECEIVER_TIMEOUT *timeout, usec_t idle_ut);
int stream_receiver_timeout_unittest(void);

struct receiver_state;
struct rrdset;
void stream_receiver_timeout_chart_refresh(struct receiver_state *rpt, struct rrdset *st);

#endif // NETDATA_STREAM_RECEIVER_TIMEOUT_H

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_RECEIVER_CADENCE_H
#define NETDATA_STREAM_RECEIVER_CADENCE_H

#include "libnetdata/libnetdata.h"

#define STREAM_RECEIVER_IDLE_TIMEOUT_MIN_SECONDS 600ULL
#define STREAM_RECEIVER_UPDATE_EVERY_MAX_SECONDS 86400U
#define STREAM_RECEIVER_KEEPALIVE_IDLE_MIN_SECONDS 30U
#define STREAM_RECEIVER_KEEPALIVE_IDLE_MAX_SECONDS 3600U

typedef struct stream_receiver_cadence_chart {
    uint32_t update_every_s;
    uint32_t generation;
} STREAM_RECEIVER_CADENCE_CHART;

typedef struct stream_receiver_cadence {
    // The host owns this state. Mutation is locked; receiver threads consume the published values atomically.
    SPINLOCK spinlock;
    Pvoid_t intervals; // chart count by update interval for the current connection
    uint32_t generation;
    uint32_t provisional_update_every_s;
    uint32_t minimum_update_every_s;
    uint32_t learned_update_every_s;
    uint32_t automatic_keepalive_idle_s;
    uint64_t application_timeout_s __attribute__((aligned(8)));
} STREAM_RECEIVER_CADENCE;

void stream_receiver_cadence_init(STREAM_RECEIVER_CADENCE *cadence);
void stream_receiver_cadence_connection_start(STREAM_RECEIVER_CADENCE *cadence, int64_t handshake_update_every_s);
void stream_receiver_cadence_destroy(STREAM_RECEIVER_CADENCE *cadence);

void stream_receiver_cadence_chart_track(
    STREAM_RECEIVER_CADENCE *cadence,
    STREAM_RECEIVER_CADENCE_CHART *chart,
    int64_t update_every_s,
    bool active);
void stream_receiver_cadence_chart_forget(
    STREAM_RECEIVER_CADENCE *cadence,
    STREAM_RECEIVER_CADENCE_CHART *chart);

uint64_t stream_receiver_cadence_application_timeout_seconds(const STREAM_RECEIVER_CADENCE *cadence);
usec_t stream_receiver_cadence_application_timeout_usec(const STREAM_RECEIVER_CADENCE *cadence);
bool stream_receiver_cadence_application_timeout_expired(const STREAM_RECEIVER_CADENCE *cadence, usec_t idle_ut);
uint32_t stream_receiver_cadence_automatic_keepalive_idle_seconds(const STREAM_RECEIVER_CADENCE *cadence);
int stream_receiver_cadence_unittest(void);

struct receiver_state;
struct rrdset;
void stream_receiver_cadence_chart_refresh(struct receiver_state *rpt, struct rrdset *st);

#endif // NETDATA_STREAM_RECEIVER_CADENCE_H

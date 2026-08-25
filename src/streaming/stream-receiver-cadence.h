// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_RECEIVER_CADENCE_H
#define NETDATA_STREAM_RECEIVER_CADENCE_H

#include "libnetdata/libnetdata.h"

#define STREAM_RECEIVER_IDLE_TIMEOUT_MIN_SECONDS 600ULL
#define STREAM_RECEIVER_UPDATE_EVERY_MAX_SECONDS 86400U
#define STREAM_RECEIVER_KEEPALIVE_IDLE_MIN_SECONDS 30U
#define STREAM_RECEIVER_KEEPALIVE_IDLE_MAX_SECONDS 3600U

typedef struct stream_receiver_cadence {
    uint32_t provisional_update_every_s; // used until a chart is observed on the connection
    uint32_t minimum_update_every_s;     // UINT32_MAX until the first chart, then monotonically decreases
} STREAM_RECEIVER_CADENCE;

void stream_receiver_cadence_init(STREAM_RECEIVER_CADENCE *cadence);
void stream_receiver_cadence_connection_start(STREAM_RECEIVER_CADENCE *cadence, int64_t handshake_update_every_s);
void stream_receiver_cadence_observe(STREAM_RECEIVER_CADENCE *cadence, int64_t update_every_s);

uint64_t stream_receiver_cadence_application_timeout_seconds(const STREAM_RECEIVER_CADENCE *cadence);
usec_t stream_receiver_cadence_application_timeout_usec(const STREAM_RECEIVER_CADENCE *cadence);
bool stream_receiver_cadence_application_timeout_expired(const STREAM_RECEIVER_CADENCE *cadence, usec_t idle_ut);
uint32_t stream_receiver_cadence_automatic_keepalive_idle_seconds(const STREAM_RECEIVER_CADENCE *cadence);
int stream_receiver_cadence_unittest(void);

struct receiver_state;
struct rrdset;
void stream_receiver_cadence_observe_chart(struct receiver_state *rpt, struct rrdset *st);

#endif // NETDATA_STREAM_RECEIVER_CADENCE_H

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_REPLICATION_TRACKING_H
#define NETDATA_STREAM_REPLICATION_TRACKING_H

#include "libnetdata/libnetdata.h"

// #define REPLICATION_TRACKING 1

#ifdef REPLICATION_TRACKING

typedef enum __attribute__((packed)) {
    REPLAY_WHO_UNKNOWN = 0, // default value
    REPLAY_WHO_ME,          // I have to respond
    REPLAY_WHO_THEM,        // they have to respond
    REPLAY_WHO_FINISHED,    // replication finished

    // terminator
    REPLAY_WHO_MAX,
} REPLAY_WHO;

struct replay_who_counters {
    size_t rcv[REPLAY_WHO_MAX];
    size_t snd[REPLAY_WHO_MAX];
};

struct rrdhost;
void replication_tracking_counters(struct rrdhost *host, struct replay_who_counters *c);

#endif

#endif //NETDATA_STREAM_REPLICATION_TRACKING_H

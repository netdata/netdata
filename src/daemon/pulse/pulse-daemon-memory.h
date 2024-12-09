// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PULSE_DAEMON_MEMORY_H
#define NETDATA_PULSE_DAEMON_MEMORY_H

#include "daemon/common.h"

extern struct netdata_buffers_statistics {
    size_t rrdhost_allocations_size;
    size_t rrdhost_senders;
    size_t rrdhost_receivers;
    size_t query_targets_size;
    size_t rrdset_done_rda_size;
    size_t buffers_aclk;
    size_t buffers_api;
    size_t buffers_functions;
    size_t buffers_sqlite;
    size_t buffers_exporters;
    size_t buffers_health;
    size_t buffers_streaming;
    size_t cbuffers_streaming;
    size_t buffers_web;
} netdata_buffers_statistics;

#if defined(PULSE_INTERNALS)
void pulse_daemon_memory_do(bool extended);
#endif

#endif //NETDATA_PULSE_DAEMON_MEMORY_H

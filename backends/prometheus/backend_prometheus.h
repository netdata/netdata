// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_BACKEND_PROMETHEUS_H
#define NETDATA_BACKEND_PROMETHEUS_H 1

#include "backends/backends.h"

typedef enum prometheus_output_flags {
    PROMETHEUS_OUTPUT_NONE       = 0,
    PROMETHEUS_OUTPUT_HELP       = (1 << 0),
    PROMETHEUS_OUTPUT_TYPES      = (1 << 1),
    PROMETHEUS_OUTPUT_NAMES      = (1 << 2),
    PROMETHEUS_OUTPUT_TIMESTAMPS = (1 << 3),
    PROMETHEUS_OUTPUT_VARIABLES  = (1 << 4)
} PROMETHEUS_OUTPUT_OPTIONS;

extern void rrd_stats_api_v1_charts_allmetrics_prometheus_single_host(RRDHOST *host, BUFFER *wb, const char *server, const char *prefix, BACKEND_OPTIONS backend_options, PROMETHEUS_OUTPUT_OPTIONS output_options);
extern void rrd_stats_api_v1_charts_allmetrics_prometheus_all_hosts(RRDHOST *host, BUFFER *wb, const char *server, const char *prefix, BACKEND_OPTIONS backend_options, PROMETHEUS_OUTPUT_OPTIONS output_options);

#endif //NETDATA_BACKEND_PROMETHEUS_H

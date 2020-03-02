// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EXPORTING_PROMETHEUS_H
#define NETDATA_EXPORTING_PROMETHEUS_H 1

#include "exporting/exporting_engine.h"

#if ENABLE_PROMETHEUS_REMOTE_WRITE
#include "remote_write/remote_write.h"
#endif

typedef enum prometheus_output_flags {
    PROMETHEUS_OUTPUT_NONE       = 0,
    PROMETHEUS_OUTPUT_HELP       = (1 << 0),
    PROMETHEUS_OUTPUT_TYPES      = (1 << 1),
    PROMETHEUS_OUTPUT_NAMES      = (1 << 2),
    PROMETHEUS_OUTPUT_TIMESTAMPS = (1 << 3),
    PROMETHEUS_OUTPUT_VARIABLES  = (1 << 4),
	PROMETHEUS_OUTPUT_OLDUNITS   = (1 << 5),
	PROMETHEUS_OUTPUT_HIDEUNITS  = (1 << 6)
} PROMETHEUS_OUTPUT_OPTIONS;

extern int can_send_rrdset(struct instance *instance, BACKEND_OPTIONS backend_options, RRDSET *st);

extern void rrd_stats_api_v1_charts_allmetrics_prometheus_single_host(struct instance *instance, RRDHOST *host, BUFFER *wb, const char *server, const char *prefix, EXPORTING_OPTIONS exporting_options, PROMETHEUS_OUTPUT_OPTIONS output_options);
extern void rrd_stats_api_v1_charts_allmetrics_prometheus_all_hosts(struct instance *instance, RRDHOST *host, BUFFER *wb, const char *server, const char *prefix, EXPORTING_OPTIONS exporting_options, PROMETHEUS_OUTPUT_OPTIONS output_options);

#if ENABLE_PROMETHEUS_REMOTE_WRITE
extern void rrd_stats_remote_write_allmetrics_prometheus(
        struct instance *instance
        , RRDHOST *host
        , const char *__hostname
        , const char *prefix
        , EXPORTING_OPTIONS exporting_options
        , time_t after
        , time_t before
        , size_t *count_charts
        , size_t *count_dims
        , size_t *count_dims_skipped
);
extern int process_prometheus_remote_write_response(BUFFER *b, struct instance *instance);
#endif

#endif //NETDATA_EXPORTING_PROMETHEUS_H

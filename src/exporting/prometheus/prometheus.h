// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EXPORTING_PROMETHEUS_H
#define NETDATA_EXPORTING_PROMETHEUS_H 1

#include "exporting/exporting_engine.h"

#define PROMETHEUS_ELEMENT_MAX  256
#define PROMETHEUS_LABELS_MAX   1024
#define PROMETHEUS_VARIABLE_MAX 256

#define PROMETHEUS_LABELS_MAX_NUMBER 128

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

extern void rrd_stats_api_v1_charts_allmetrics_prometheus_single_host(
    RRDHOST *host, BUFFER *wb, const char *server, const char *prefix,
    EXPORTING_OPTIONS exporting_options, PROMETHEUS_OUTPUT_OPTIONS output_options);
extern void rrd_stats_api_v1_charts_allmetrics_prometheus_all_hosts(
    RRDHOST *host, BUFFER *wb, const char *server, const char *prefix,
    EXPORTING_OPTIONS exporting_options, PROMETHEUS_OUTPUT_OPTIONS output_options);

int can_send_rrdset(struct instance *instance, RRDSET *st);
size_t prometheus_name_copy(char *d, const char *s, size_t usable);
size_t prometheus_label_copy(char *d, const char *s, size_t usable);
char *prometheus_units_copy(char *d, const char *s, size_t usable, int showoldunits);

void format_host_labels_prometheus(struct instance *instance, RRDHOST *host);

extern void prometheus_clean_server_root();

#endif //NETDATA_EXPORTING_PROMETHEUS_H

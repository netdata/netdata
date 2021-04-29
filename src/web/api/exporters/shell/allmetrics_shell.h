// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_ALLMETRICS_SHELL_H
#define NETDATA_API_ALLMETRICS_SHELL_H

#include "../allmetrics.h"

#define ALLMETRICS_FORMAT_SHELL                 "shell"
#define ALLMETRICS_FORMAT_PROMETHEUS            "prometheus"
#define ALLMETRICS_FORMAT_PROMETHEUS_ALL_HOSTS  "prometheus_all_hosts"
#define ALLMETRICS_FORMAT_JSON                  "json"

#define ALLMETRICS_SHELL                        1
#define ALLMETRICS_PROMETHEUS                   2
#define ALLMETRICS_JSON                         3
#define ALLMETRICS_PROMETHEUS_ALL_HOSTS         4

extern void rrd_stats_api_v1_charts_allmetrics_json(RRDHOST *host, BUFFER *wb);
extern void rrd_stats_api_v1_charts_allmetrics_shell(RRDHOST *host, BUFFER *wb);

#endif //NETDATA_API_ALLMETRICS_SHELL_H

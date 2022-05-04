// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_METRIC_CORRELATIONS_H
#define NETDATA_METRIC_CORRELATIONS_H 1

extern int enable_metric_correlations;

void metric_correlations (RRDHOST *host, BUFFER *wb, long long selected_after, long long selected_before, long long reference_after, long long reference_before, long long max_points);

#endif //NETDATA_METRIC_CORRELATIONS_H

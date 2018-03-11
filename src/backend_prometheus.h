//
// Created by costa on 09/07/17.
//

#ifndef NETDATA_BACKEND_PROMETHEUS_H
#define NETDATA_BACKEND_PROMETHEUS_H

extern void rrd_stats_api_v1_charts_allmetrics_prometheus_single_host(RRDHOST *host, BUFFER *wb, const char *server, const char *prefix, uint32_t options, int help, int types, int names, int timestamps);
extern void rrd_stats_api_v1_charts_allmetrics_prometheus_all_hosts(RRDHOST *host, BUFFER *wb, const char *server, const char *prefix, uint32_t options, int help, int types, int names, int timestamps);

#endif //NETDATA_BACKEND_PROMETHEUS_H

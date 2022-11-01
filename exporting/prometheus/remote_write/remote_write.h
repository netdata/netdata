// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EXPORTING_PROMETHEUS_REMOTE_WRITE_H
#define NETDATA_EXPORTING_PROMETHEUS_REMOTE_WRITE_H

#include "exporting/exporting_engine.h"
#include "exporting/prometheus/prometheus.h"
#include "remote_write_request.h"

struct prometheus_remote_write_specific_data {
    void *write_request;
};

struct prometheus_remote_write_variables_callback_options {
    RRDHOST *host;
    time_t now;
    struct instance *instance;
};

int init_prometheus_remote_write_instance(struct instance *instance);
void clean_prometheus_remote_write(struct instance *instance);

int format_host_prometheus_remote_write(struct instance *instance, RRDHOST *host);
int format_chart_prometheus_remote_write(struct instance *instance, RRDSET *st);
int format_dimension_prometheus_remote_write(struct instance *instance, RRDDIM *rd);
int format_variables_prometheus_remote_write(struct instance *instance, RRDHOST *host);
int format_batch_prometheus_remote_write(struct instance *instance);

void prometheus_remote_write_prepare_header(struct instance *instance);
int process_prometheus_remote_write_response(BUFFER *buffer, struct instance *instance);

#endif //NETDATA_EXPORTING_PROMETHEUS_REMOTE_WRITE_H

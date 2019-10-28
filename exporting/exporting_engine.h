// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EXPORTING_ENGINE_H
#define NETDATA_EXPORTING_ENGINE_H 1

#include "daemon/common.h"

struct engine;

struct instance_config {
    const char *destination;
    int update_every;
    int buffer_on_failures;
    long timeoutms;
    SIMPLE_PATTERN *charts_pattern;
    SIMPLE_PATTERN *hosts_pattern;
    int send_names_instead_of_ids;
    void *connector_specific_config;
};

struct connector_config {
    BACKEND_TYPE type;
};

struct engine_config {
    const char *prefix;
    const char *hostname;
    int update_every;
    BACKEND_OPTIONS options; // TODO: Rename to EXPORTING_OPTIONS
};

struct instance {
    struct instance_config config;
    void *buffer;
    void *stats;

    int (*start_batch_formatting)(struct engine *);
    int (*start_host_formatting)(struct engine *);
    int (*start_chart_formatting)(struct engine *);
    int (*metric_formatting)(struct engine *);
    int (*end_chart_formatting)(struct engine *);
    int (*end_host_formatting)(struct engine *);
    int (*end_batch_formatting)(struct engine *);

    struct instance *next;
    struct connector *connector;
};

struct connector {
    struct connector_config config;
    struct instance *instance_root;
    struct connector *next;
    struct engine *engine;
};

struct engine {
    struct engine_config config;
    struct connector *connector_root;
    time_t after;
    time_t before;
};

void *exporting_main(void *ptr);

struct engine *read_exporting_config();

int init_connectors(struct engine *);

int mark_scheduled_instances(struct engine *);
int prepare_buffers(struct engine *);
int notify_workers(struct engine *);

int send_internal_metrics(struct engine *);

#endif /* NETDATA_EXPORTING_ENGINE_H */

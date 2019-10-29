// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EXPORTING_ENGINE_H
#define NETDATA_EXPORTING_ENGINE_H 1

#include "daemon/common.h"

struct engine;

struct instance_config {
    const char *name;
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

    struct instance *next;
    struct connector *connector;
};

struct connector {
    struct connector_config config;

    int (*start_batch_formatting)(struct instance *instance);
    int (*start_host_formatting)(struct instance *instance);
    int (*start_chart_formatting)(struct instance *instance);
    int (*metric_formatting)(struct instance *instance, RRDDIM *rd);
    int (*end_chart_formatting)(struct instance *instance);
    int (*end_host_formatting)(struct instance *instance);
    int (*end_batch_formatting)(struct instance *instance);

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

int init_connectors(struct engine *engine);

int mark_scheduled_instances(struct engine *engine);
int prepare_buffers(struct engine *engine);
int notify_workers(struct engine *engine);

size_t exporting_name_copy(char *dst, const char *src, size_t max_len);

int start_batch_formatting(struct engine *engine);
int start_host_formatting(struct engine *engine);
int start_chart_formatting(struct engine *engine);
int metric_formatting(struct engine *engine, RRDDIM *rd);
int end_chart_formatting(struct engine *engine);
int end_host_formatting(struct engine *engine);
int end_batch_formatting(struct engine *engine);

int send_internal_metrics(struct engine *engine);

#endif /* NETDATA_EXPORTING_ENGINE_H */

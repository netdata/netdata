// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EXPORTING_ENGINE_H
#define NETDATA_EXPORTING_ENGINE_H 1

#include "daemon/common.h"

struct instance_config {
    const char *source;
    SIMPLE_PATTERN *charts_pattern;
    SIMPLE_PATTERN *hosts_pattern;
    int buffer_on_failures;
    long timeoutms;
    void *connector_specific_config;
};

struct connector_config {
    void *connector_specific_config;
};

struct engine_config {
    const char *prefix;
    int update_every;
    BACKEND_OPTIONS options; // TODO: Rename to EXPORTING_OPTIONS
    const char *hostname;
};

struct instance {
    struct instance_config *config;
    void *buffer;
    void *stats;
    struct instance *next;
};

struct connector {
    int type;
    struct connector_config *config;
    struct instance *instance_root;
    struct connector *next;
};

struct engine {
    struct engine_config *config;
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

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EXPORTING_ENGINE_H
#define NETDATA_EXPORTING_ENGINE_H 1

#include "daemon/common.h"

#include <uv.h>

extern char *expconfig_get(struct config *root, const char *section, const char *name, const char *default_value);
extern long long expconfig_get_number(struct config *root, const char *section, const char *name, long long value);
extern int expconfig_get_boolean(struct config *root, const char *section, const char *name, int value);

extern int is_valid_connector(char *type);
extern struct _connector_instance *global_connector_instance;
extern struct config exporting_config;

#define EXPORTER_DESTINATION                "destination"
#define EXPORTER_DESTINATION_DEFAULT        "localhost"

#define EXPORTER_UPDATE_EVERY               "update every"
#define EXPORTER_UPDATE_EVERY_DEFAULT       10

#define EXPORTER_BUF_ONFAIL                 "buffer on failures"
#define EXPORTER_BUF_ONFAIL_DEFAULT         10

#define EXPORTER_TIMEOUT_MS                 "timeout ms"
#define EXPORTER_TIMEOUT_MS_DEFAULT         10000

#define EXPORTER_SEND_CHART_MATCH           "send charts matching"
#define EXPORTER_SEND_CHART_MATCH_DEFAULT   "*"

#define EXPORTER_SEND_HOST_MATCH            "send hosts matching"
#define EXPORTER_SEND_HOST_MATCH_DEFAULT    "localhost *"

#define EXPORTER_SEND_NAMES                 "send names instead of ids"
#define EXPORTER_SEND_NAMES_DEFAULT         1

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

struct simple_connector_config {
    int default_port;
};

struct connector_config {
    BACKEND_TYPE type;
    void *connector_specific_config;
};

struct engine_config {
    const char *prefix;
    const char *hostname;
    int update_every;
    BACKEND_OPTIONS options; // TODO: Rename to EXPORTING_OPTIONS
};

struct stats {
    collected_number chart_buffered_metrics;
    collected_number chart_lost_metrics;
    collected_number chart_sent_metrics;
    collected_number chart_buffered_bytes;
    collected_number chart_received_bytes;
    collected_number chart_sent_bytes;
    collected_number chart_receptions;
    collected_number chart_transmission_successes;
    collected_number chart_transmission_failures;
    collected_number chart_data_lost_events;
    collected_number chart_lost_bytes;
    collected_number chart_reconnects;
};

struct instance {
    struct instance_config config;
    void *buffer;
    struct stats stats;

    int scheduled;
    time_t after;
    time_t before;

    uv_thread_t thread;
    uv_mutex_t mutex;
    uv_cond_t cond_var;

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

    void (*worker)(void *instance_p);

    struct instance *instance_root;
    struct connector *next;
    struct engine *engine;
};

struct engine {
    struct engine_config config;
    struct connector *connector_root;
    time_t now;
};

void *exporting_main(void *ptr);

struct engine *read_exporting_config();
BACKEND_TYPE exporting_select_type(const char *type);

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

int exporting_discard_response(BUFFER *buffer, struct instance *instance);
void simple_connector_receive_response(int *sock, struct instance *instance);
void simple_connector_send_buffer(int *sock, int *failures, struct instance *instance);
void simple_connector_worker(void *instance_p);

int send_internal_metrics(struct engine *engine);

#endif /* NETDATA_EXPORTING_ENGINE_H */

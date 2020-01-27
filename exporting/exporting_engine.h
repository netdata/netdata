// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EXPORTING_ENGINE_H
#define NETDATA_EXPORTING_ENGINE_H 1

#include "daemon/common.h"

#include <uv.h>

#define exporter_get(section, name, value) expconfig_get(&exporting_config, section, name, value)
#define exporter_get_number(section, name, value) expconfig_get_number(&exporting_config, section, name, value)
#define exporter_get_boolean(section, name, value) expconfig_get_boolean(&exporting_config, section, name, value)

extern struct config exporting_config;

#define EXPORTER_DATA_SOURCE                    "data source"
#define EXPORTER_DATA_SOURCE_DEFAULT            "average"

#define EXPORTER_DESTINATION                    "destination"
#define EXPORTER_DESTINATION_DEFAULT            "localhost"

#define EXPORTER_UPDATE_EVERY                   "update every"
#define EXPORTER_UPDATE_EVERY_DEFAULT           10

#define EXPORTER_BUF_ONFAIL                     "buffer on failures"
#define EXPORTER_BUF_ONFAIL_DEFAULT             10

#define EXPORTER_TIMEOUT_MS                     "timeout ms"
#define EXPORTER_TIMEOUT_MS_DEFAULT             10000

#define EXPORTER_SEND_CHART_MATCH               "send charts matching"
#define EXPORTER_SEND_CHART_MATCH_DEFAULT       "*"

#define EXPORTER_SEND_HOST_MATCH                "send hosts matching"
#define EXPORTER_SEND_HOST_MATCH_DEFAULT        "localhost *"

#define EXPORTER_SEND_CONFIGURED_LABELS         "send configured labels"
#define EXPORTER_SEND_CONFIGURED_LABELS_DEFAULT CONFIG_BOOLEAN_YES

#define EXPORTER_SEND_AUTOMATIC_LABELS          "send automatic labels"
#define EXPORTER_SEND_AUTOMATIC_LABELS_DEFAULT  CONFIG_BOOLEAN_NO

#define EXPORTER_SEND_NAMES                     "send names instead of ids"
#define EXPORTER_SEND_NAMES_DEFAULT             CONFIG_BOOLEAN_YES

typedef enum exporting_options {
    EXPORTING_OPTION_NONE                   = 0,

    EXPORTING_SOURCE_DATA_AS_COLLECTED      = (1 << 0),
    EXPORTING_SOURCE_DATA_AVERAGE           = (1 << 1),
    EXPORTING_SOURCE_DATA_SUM               = (1 << 2),

    EXPORTING_OPTION_SEND_CONFIGURED_LABELS = (1 << 3),
    EXPORTING_OPTION_SEND_AUTOMATIC_LABELS  = (1 << 4),

    EXPORTING_OPTION_SEND_NAMES             = (1 << 16)
} EXPORTING_OPTIONS;

#define EXPORTING_OPTIONS_SOURCE_BITS                                                                                  \
    (EXPORTING_SOURCE_DATA_AS_COLLECTED | EXPORTING_SOURCE_DATA_AVERAGE | EXPORTING_SOURCE_DATA_SUM)
#define EXPORTING_OPTIONS_DATA_SOURCE(exporting_options) (exporting_options & EXPORTING_OPTIONS_SOURCE_BITS)

#define sending_labels_configured(instance)                                                                            \
    (instance->config.options & (EXPORTING_OPTION_SEND_CONFIGURED_LABELS | EXPORTING_OPTION_SEND_AUTOMATIC_LABELS))

#define should_send_label(instance, label)                                                                             \
    ((instance->config.options & EXPORTING_OPTION_SEND_CONFIGURED_LABELS &&                                            \
      label->label_source == LABEL_SOURCE_NETDATA_CONF) ||                                                             \
     (instance->config.options & EXPORTING_OPTION_SEND_AUTOMATIC_LABELS &&                                             \
      label->label_source != LABEL_SOURCE_NETDATA_CONF))

struct engine;

struct instance_config {
    const char *name;
    const char *destination;

    int update_every;
    int buffer_on_failures;
    long timeoutms;

    EXPORTING_OPTIONS options;
    SIMPLE_PATTERN *charts_pattern;
    SIMPLE_PATTERN *hosts_pattern;

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
    int skip_host;
    int skip_chart;

    BUFFER *labels;

    time_t after;
    time_t before;

    uv_thread_t thread;
    uv_mutex_t mutex;
    uv_cond_t cond_var;

    int (*start_batch_formatting)(struct instance *instance);
    int (*start_host_formatting)(struct instance *instance, RRDHOST *host);
    int (*start_chart_formatting)(struct instance *instance, RRDSET *st);
    int (*metric_formatting)(struct instance *instance, RRDDIM *rd);
    int (*end_chart_formatting)(struct instance *instance, RRDSET *st);
    int (*end_host_formatting)(struct instance *instance, RRDHOST *host);
    int (*end_batch_formatting)(struct instance *instance);

    size_t index;
    struct instance *next;
    struct connector *connector;
};

struct connector {
    struct connector_config config;

    void (*worker)(void *instance_p);

    struct instance *instance_root;
    struct connector *next;
    struct engine *engine;
};

struct engine {
    struct engine_config config;

    size_t instance_num;
    time_t now;

    struct connector *connector_root;
};

void *exporting_main(void *ptr);

struct engine *read_exporting_config();
BACKEND_TYPE exporting_select_type(const char *type);

int init_connectors(struct engine *engine);

int mark_scheduled_instances(struct engine *engine);
int prepare_buffers(struct engine *engine);
int notify_workers(struct engine *engine);

size_t exporting_name_copy(char *dst, const char *src, size_t max_len);

int rrdhost_is_exportable(struct instance *instance, RRDHOST *host);
int rrdset_is_exportable(struct instance *instance, RRDSET *st);

calculated_number exporting_calculate_value_from_stored_data(
    struct instance *instance,
    RRDDIM *rd,
    time_t *last_timestamp);

int start_batch_formatting(struct engine *engine);
int start_host_formatting(struct engine *engine, RRDHOST *host);
int start_chart_formatting(struct engine *engine, RRDSET *st);
int metric_formatting(struct engine *engine, RRDDIM *rd);
int end_chart_formatting(struct engine *engine, RRDSET *st);
int end_host_formatting(struct engine *engine, RRDHOST *host);
int end_batch_formatting(struct engine *engine);
int flush_host_labels(struct instance *instance, RRDHOST *host);

int exporting_discard_response(BUFFER *buffer, struct instance *instance);
void simple_connector_receive_response(int *sock, struct instance *instance);
void simple_connector_send_buffer(int *sock, int *failures, struct instance *instance);
void simple_connector_worker(void *instance_p);

int send_internal_metrics(struct engine *engine);

#endif /* NETDATA_EXPORTING_ENGINE_H */

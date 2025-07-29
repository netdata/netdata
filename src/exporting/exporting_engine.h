// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EXPORTING_ENGINE_H
#define NETDATA_EXPORTING_ENGINE_H 1

#include "database/rrd.h"
#include <uv.h>

#define exporter_get(section, name, value) expconfig_get(&exporting_config, section, name, value)
#define exporter_get_number(section, name, value) expconfig_get_number(&exporting_config, section, name, value)
#define exporter_get_boolean(section, name, value) expconfig_get_boolean(&exporting_config, section, name, value)

extern struct config exporting_config;

#define EXPORTING_UPDATE_EVERY_OPTION_NAME "update every"
#define EXPORTING_UPDATE_EVERY_DEFAULT 10

typedef enum exporting_options {
    EXPORTING_OPTION_NON                    = 0,

    EXPORTING_SOURCE_DATA_AS_COLLECTED      = (1 << 0),
    EXPORTING_SOURCE_DATA_AVERAGE           = (1 << 1),
    EXPORTING_SOURCE_DATA_SUM               = (1 << 2),

    EXPORTING_OPTION_SEND_CONFIGURED_LABELS = (1 << 3),
    EXPORTING_OPTION_SEND_AUTOMATIC_LABELS  = (1 << 4),
    EXPORTING_OPTION_USE_TLS                = (1 << 5),

    EXPORTING_OPTION_SEND_NAMES             = (1 << 16),
    EXPORTING_OPTION_SEND_VARIABLES         = (1 << 17)
} EXPORTING_OPTIONS;

#define EXPORTING_OPTIONS_SOURCE_BITS                                                                                  \
    (EXPORTING_SOURCE_DATA_AS_COLLECTED | EXPORTING_SOURCE_DATA_AVERAGE | EXPORTING_SOURCE_DATA_SUM)
#define EXPORTING_OPTIONS_DATA_SOURCE(exporting_options) ((exporting_options) & EXPORTING_OPTIONS_SOURCE_BITS)

extern EXPORTING_OPTIONS global_exporting_options;
extern const char *global_exporting_prefix;

#define sending_labels_configured(instance)                                                                            \
    ((instance)->config.options & (EXPORTING_OPTION_SEND_CONFIGURED_LABELS | EXPORTING_OPTION_SEND_AUTOMATIC_LABELS))

#define should_send_label(instance, label_source)                                                                      \
    (((instance)->config.options & EXPORTING_OPTION_SEND_CONFIGURED_LABELS && (label_source)&RRDLABEL_SRC_CONFIG) ||   \
     ((instance)->config.options & EXPORTING_OPTION_SEND_AUTOMATIC_LABELS && (label_source)&RRDLABEL_SRC_AUTO))

#define should_send_variables(instance) ((instance)->config.options & EXPORTING_OPTION_SEND_VARIABLES)

typedef enum exporting_connector_types {
    EXPORTING_CONNECTOR_TYPE_UNKNOWN,                 // Invalid type
    EXPORTING_CONNECTOR_TYPE_GRAPHITE,                // Send plain text to Graphite
    EXPORTING_CONNECTOR_TYPE_GRAPHITE_HTTP,           // Send data to Graphite using HTTP API
    EXPORTING_CONNECTOR_TYPE_JSON,                    // Send data in JSON format
    EXPORTING_CONNECTOR_TYPE_JSON_HTTP,               // Send data in JSON format using HTTP API
    EXPORTING_CONNECTOR_TYPE_OPENTSDB,                // Send data to OpenTSDB using telnet API
    EXPORTING_CONNECTOR_TYPE_OPENTSDB_HTTP,           // Send data to OpenTSDB using HTTP API
    EXPORTING_CONNECTOR_TYPE_PROMETHEUS_REMOTE_WRITE, // Send data using Prometheus remote write protocol
    EXPORTING_CONNECTOR_TYPE_KINESIS,                 // Send message to AWS Kinesis
    EXPORTING_CONNECTOR_TYPE_PUBSUB,                  // Send message to Google Cloud Pub/Sub
    EXPORTING_CONNECTOR_TYPE_MONGODB,                 // Send data to MongoDB collection
    EXPORTING_CONNECTOR_TYPE_NUM                      // Number of exporting connector types
} EXPORTING_CONNECTOR_TYPE;

struct engine;

struct instance_config {
    EXPORTING_CONNECTOR_TYPE type;
    const char *type_name;

    const char *name;
    const char *destination;
    const char *username;
    const char *password;
    const char *prefix;
    const char *label_prefix;
    const char *hostname;
    const char *thread_tag;

    int update_every;
    int buffer_on_failures;
    long timeoutms;

    EXPORTING_OPTIONS options;
    SIMPLE_PATTERN *charts_pattern;
    SIMPLE_PATTERN *hosts_pattern;

    int initialized;

    void *connector_specific_config;
};

struct simple_connector_config {
    int default_port;
};

struct simple_connector_buffer {
    BUFFER *header;
    BUFFER *buffer;

    size_t buffered_metrics;
    size_t buffered_bytes;

    int used;

    struct simple_connector_buffer *next;
};

#define CONNECTED_TO_MAX 1024

struct simple_connector_data {
    void *connector_specific_data;

    char connected_to[CONNECTED_TO_MAX];
    
    char *auth_string;

    size_t total_buffered_metrics;

    BUFFER *header;
    BUFFER *buffer;
    size_t buffered_metrics;
    size_t buffered_bytes;

    struct simple_connector_buffer *previous_buffer;
    struct simple_connector_buffer *first_buffer;
    struct simple_connector_buffer *last_buffer;

    NETDATA_SSL ssl;
};

struct prometheus_remote_write_specific_config {
    char *remote_write_path;
};

struct aws_kinesis_specific_config {
    char *stream_name;
    char *auth_key_id;
    char *secure_key;
};

struct pubsub_specific_config {
    char *credentials_file;
    char *project_id;
    char *topic_id;
};

struct mongodb_specific_config {
    char *database;
    char *collection;
};

struct engine_config {
    const char *hostname;
    int update_every;
};

struct stats {
    collected_number buffered_metrics;
    collected_number lost_metrics;
    collected_number sent_metrics;
    collected_number buffered_bytes;
    collected_number lost_bytes;
    collected_number sent_bytes;
    collected_number received_bytes;
    collected_number transmission_successes;
    collected_number data_lost_events;
    collected_number reconnects;
    collected_number transmission_failures;
    collected_number receptions;

    int initialized;

    RRDSET *st_metrics;
    RRDDIM *rd_buffered_metrics;
    RRDDIM *rd_lost_metrics;
    RRDDIM *rd_sent_metrics;

    RRDSET *st_bytes;
    RRDDIM *rd_buffered_bytes;
    RRDDIM *rd_lost_bytes;
    RRDDIM *rd_sent_bytes;
    RRDDIM *rd_received_bytes;

    RRDSET *st_ops;
    RRDDIM *rd_transmission_successes;
    RRDDIM *rd_data_lost_events;
    RRDDIM *rd_reconnects;
    RRDDIM *rd_transmission_failures;
    RRDDIM *rd_receptions;

    RRDSET *st_rusage;
    RRDDIM *rd_user;
    RRDDIM *rd_system;
};

struct instance {
    struct instance_config config;
    void *buffer;
    void (*worker)(void *instance_p);
    struct stats stats;

    int scheduled;
    int disabled;
    int skip_host;
    int skip_chart;

    BUFFER *labels_buffer;

    time_t after;
    time_t before;

    ND_THREAD *thread;
    uv_mutex_t mutex;
    uv_cond_t cond_var;
    int data_is_ready;

    int (*start_batch_formatting)(struct instance *instance);
    int (*start_host_formatting)(struct instance *instance, RRDHOST *host);
    int (*start_chart_formatting)(struct instance *instance, RRDSET *st);
    int (*metric_formatting)(struct instance *instance, RRDDIM *rd);
    int (*end_chart_formatting)(struct instance *instance, RRDSET *st);
    int (*variables_formatting)(struct instance *instance, RRDHOST *host);
    int (*end_host_formatting)(struct instance *instance, RRDHOST *host);
    int (*end_batch_formatting)(struct instance *instance);

    void (*prepare_header)(struct instance *instance);
    int (*check_response)(BUFFER *buffer, struct instance *instance);

    void *connector_specific_data;

    size_t index;
    struct instance *next;
    struct engine *engine;

    volatile sig_atomic_t exited;
};

struct engine {
    struct engine_config config;

    size_t instance_num;
    time_t now;

    int aws_sdk_initialized;
    int protocol_buffers_initialized;
    int mongoc_initialized;

    struct instance *instance_root;

    volatile sig_atomic_t exit;
};

extern struct instance *prometheus_exporter_instance;

void exporting_main(void *ptr);

struct engine *read_exporting_config();
EXPORTING_CONNECTOR_TYPE exporting_select_type(const char *type);

int init_connectors(struct engine *engine);
void simple_connector_init(struct instance *instance);

int mark_scheduled_instances(struct engine *engine);
void prepare_buffers(struct engine *engine);

size_t exporting_name_copy(char *dst, const char *src, size_t max_len);

int rrdhost_is_exportable(struct instance *instance, RRDHOST *host);
int rrdset_is_exportable(struct instance *instance, RRDSET *st);

EXPORTING_OPTIONS exporting_parse_data_source(const char *source, EXPORTING_OPTIONS exporting_options);

NETDATA_DOUBLE
exporting_calculate_value_from_stored_data(
    struct instance *instance,
    RRDDIM *rd,
    time_t *last_timestamp);

void start_batch_formatting(struct engine *engine);
void start_host_formatting(struct engine *engine, RRDHOST *host);
void start_chart_formatting(struct engine *engine, RRDSET *st);
void metric_formatting(struct engine *engine, RRDDIM *rd);
void end_chart_formatting(struct engine *engine, RRDSET *st);
void variables_formatting(struct engine *engine, RRDHOST *host);
void end_host_formatting(struct engine *engine, RRDHOST *host);
void end_batch_formatting(struct engine *engine);
int flush_host_labels(struct instance *instance, RRDHOST *host);
int simple_connector_end_batch(struct instance *instance);

int exporting_discard_response(BUFFER *buffer, struct instance *instance);
void simple_connector_receive_response(int *sock, struct instance *instance);
void simple_connector_send_buffer(
    int *sock, int *failures, struct instance *instance, BUFFER *header, BUFFER *buffer, size_t buffered_metrics);
void simple_connector_worker(void *instance_p);

void create_main_rusage_chart(RRDSET **st_rusage, RRDDIM **rd_user, RRDDIM **rd_system);
void send_main_rusage(RRDSET *st_rusage, RRDDIM *rd_user, RRDDIM *rd_system);
void send_internal_metrics(struct instance *instance);

void clean_instance(struct instance *ptr);
void simple_connector_cleanup(struct instance *instance);

/**
 * Free exporting configuration
 * 
 * Free all memory associated with the exporting configuration.
 * Called during shutdown to prevent memory leaks.
 */
void exporting_config_free(void);

static inline void disable_instance(struct instance *instance)
{
    instance->disabled = 1;
    instance->scheduled = 0;
    uv_mutex_unlock(&instance->mutex);
    netdata_log_error("EXPORTING: Instance %s disabled", instance->config.name);
}

#include "exporting/prometheus/prometheus.h"
#include "exporting/opentsdb/opentsdb.h"
#ifdef ENABLE_PROMETHEUS_REMOTE_WRITE
#include "exporting/prometheus/remote_write/remote_write.h"
#endif

#if HAVE_KINESIS
#include "exporting/aws_kinesis/aws_kinesis.h"
#endif

#endif /* NETDATA_EXPORTING_ENGINE_H */

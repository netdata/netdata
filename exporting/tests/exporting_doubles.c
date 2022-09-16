// SPDX-License-Identifier: GPL-3.0-or-later

#include "test_exporting_engine.h"

struct engine *__real_read_exporting_config();
struct engine *__wrap_read_exporting_config()
{
    function_called();
    return mock_ptr_type(struct engine *);
}

struct engine *__mock_read_exporting_config()
{
    struct engine *engine = calloc(1, sizeof(struct engine));
    engine->config.hostname = strdupz("test_engine_host");
    engine->config.update_every = 3;


    engine->instance_root = calloc(1, sizeof(struct instance));
    struct instance *instance = engine->instance_root;
    instance->engine = engine;
    instance->config.type = EXPORTING_CONNECTOR_TYPE_GRAPHITE;
    instance->config.name = strdupz("instance_name");
    instance->config.destination = strdupz("localhost");
    instance->config.username = strdupz("");
    instance->config.password = strdupz("");
    instance->config.prefix = strdupz("netdata");
    instance->config.hostname = strdupz("test-host");
    instance->config.update_every = 1;
    instance->config.buffer_on_failures = 10;
    instance->config.timeoutms = 10000;
    instance->config.charts_pattern = simple_pattern_create("*", NULL, SIMPLE_PATTERN_EXACT);
    instance->config.hosts_pattern = simple_pattern_create("*", NULL, SIMPLE_PATTERN_EXACT);
    instance->config.options = EXPORTING_SOURCE_DATA_AS_COLLECTED | EXPORTING_OPTION_SEND_NAMES;

    return engine;
}

int __real_init_connectors(struct engine *engine);
int __wrap_init_connectors(struct engine *engine)
{
    function_called();
    check_expected_ptr(engine);
    return mock_type(int);
}

int __real_mark_scheduled_instances(struct engine *engine);
int __wrap_mark_scheduled_instances(struct engine *engine)
{
    function_called();
    check_expected_ptr(engine);
    return mock_type(int);
}

NETDATA_DOUBLE __real_exporting_calculate_value_from_stored_data(
    struct instance *instance,
    RRDDIM *rd,
    time_t *last_timestamp);
NETDATA_DOUBLE __wrap_exporting_calculate_value_from_stored_data(
    struct instance *instance,
    RRDDIM *rd,
    time_t *last_timestamp)
{
    (void)instance;
    (void)rd;

    *last_timestamp = 15052;

    function_called();
    return mock_type(NETDATA_DOUBLE);
}

int __real_prepare_buffers(struct engine *engine);
int __wrap_prepare_buffers(struct engine *engine)
{
    function_called();
    check_expected_ptr(engine);
    return mock_type(int);
}

void __wrap_create_main_rusage_chart(RRDSET **st_rusage, RRDDIM **rd_user, RRDDIM **rd_system)
{
    function_called();
    check_expected_ptr(st_rusage);
    check_expected_ptr(rd_user);
    check_expected_ptr(rd_system);
}

void __wrap_send_main_rusage(RRDSET *st_rusage, RRDDIM *rd_user, RRDDIM *rd_system)
{
    function_called();
    check_expected_ptr(st_rusage);
    check_expected_ptr(rd_user);
    check_expected_ptr(rd_system);
}

int __wrap_send_internal_metrics(struct instance *instance)
{
    function_called();
    check_expected_ptr(instance);
    return mock_type(int);
}

int __wrap_rrdhost_is_exportable(struct instance *instance, RRDHOST *host)
{
    function_called();
    check_expected_ptr(instance);
    check_expected_ptr(host);
    return mock_type(int);
}

int __wrap_rrdset_is_exportable(struct instance *instance, RRDSET *st)
{
    function_called();
    check_expected_ptr(instance);
    check_expected_ptr(st);
    return mock_type(int);
}

int __mock_start_batch_formatting(struct instance *instance)
{
    function_called();
    check_expected_ptr(instance);
    return mock_type(int);
}

int __mock_start_host_formatting(struct instance *instance, RRDHOST *host)
{
    function_called();
    check_expected_ptr(instance);
    check_expected_ptr(host);
    return mock_type(int);
}

int __mock_start_chart_formatting(struct instance *instance, RRDSET *st)
{
    function_called();
    check_expected_ptr(instance);
    check_expected_ptr(st);
    return mock_type(int);
}

int __mock_metric_formatting(struct instance *instance, RRDDIM *rd)
{
    function_called();
    check_expected_ptr(instance);
    check_expected_ptr(rd);
    return mock_type(int);
}

int __mock_end_chart_formatting(struct instance *instance, RRDSET *st)
{
    function_called();
    check_expected_ptr(instance);
    check_expected_ptr(st);
    return mock_type(int);
}

int __mock_variables_formatting(struct instance *instance, RRDHOST *host)
{
    function_called();
    check_expected_ptr(instance);
    check_expected_ptr(host);
    return mock_type(int);
}

int __mock_end_host_formatting(struct instance *instance, RRDHOST *host)
{
    function_called();
    check_expected_ptr(instance);
    check_expected_ptr(host);
    return mock_type(int);
}

int __mock_end_batch_formatting(struct instance *instance)
{
    function_called();
    check_expected_ptr(instance);
    return mock_type(int);
}

int __wrap_simple_connector_end_batch(struct instance *instance)
{
    function_called();
    check_expected_ptr(instance);
    return mock_type(int);
}

#if ENABLE_PROMETHEUS_REMOTE_WRITE
void *__wrap_init_write_request()
{
    function_called();
    return mock_ptr_type(void *);
}

void __wrap_add_host_info(
    void *write_request_p,
    const char *name, const char *instance, const char *application, const char *version, const int64_t timestamp)
{
    function_called();
    check_expected_ptr(write_request_p);
    check_expected_ptr(name);
    check_expected_ptr(instance);
    check_expected_ptr(application);
    check_expected_ptr(version);
    check_expected(timestamp);
}

void __wrap_add_label(void *write_request_p, char *key, char *value)
{
    function_called();
    check_expected_ptr(write_request_p);
    check_expected_ptr(key);
    check_expected_ptr(value);
}

void __wrap_add_metric(
    void *write_request_p,
    const char *name, const char *chart, const char *family, const char *dimension,
    const char *instance, const double value, const int64_t timestamp)
{
    function_called();
    check_expected_ptr(write_request_p);
    check_expected_ptr(name);
    check_expected_ptr(chart);
    check_expected_ptr(family);
    check_expected_ptr(dimension);
    check_expected_ptr(instance);
    check_expected(value);
    check_expected(timestamp);
}
#endif // ENABLE_PROMETHEUS_REMOTE_WRITE

#if HAVE_KINESIS
void __wrap_aws_sdk_init()
{
    function_called();
}

void __wrap_kinesis_init(
    void *kinesis_specific_data_p, const char *region, const char *access_key_id, const char *secret_key,
    const long timeout)
{
    function_called();
    check_expected_ptr(kinesis_specific_data_p);
    check_expected_ptr(region);
    check_expected_ptr(access_key_id);
    check_expected_ptr(secret_key);
    check_expected(timeout);
}

void __wrap_kinesis_put_record(
    void *kinesis_specific_data_p, const char *stream_name, const char *partition_key, const char *data,
    size_t data_len)
{
    function_called();
    check_expected_ptr(kinesis_specific_data_p);
    check_expected_ptr(stream_name);
    check_expected_ptr(partition_key);
    check_expected_ptr(data);
    check_expected_ptr(data);
    check_expected(data_len);
}

int __wrap_kinesis_get_result(void *request_outcomes_p, char *error_message, size_t *sent_bytes, size_t *lost_bytes)
{
    function_called();
    check_expected_ptr(request_outcomes_p);
    check_expected_ptr(error_message);
    check_expected_ptr(sent_bytes);
    check_expected_ptr(lost_bytes);
    return mock_type(int);
}
#endif // HAVE_KINESIS

#if ENABLE_EXPORTING_PUBSUB
int __wrap_pubsub_init(
    void *pubsub_specific_data_p, char *error_message, const char *destination, const char *credentials_file,
    const char *project_id, const char *topic_id)
{
    function_called();
    check_expected_ptr(pubsub_specific_data_p);
    check_expected_ptr(error_message);
    check_expected_ptr(destination);
    check_expected_ptr(credentials_file);
    check_expected_ptr(project_id);
    check_expected_ptr(topic_id);
    return mock_type(int);
}

int __wrap_pubsub_add_message(void *pubsub_specific_data_p, char *data)
{
    function_called();
    check_expected_ptr(pubsub_specific_data_p);
    check_expected_ptr(data);
    return mock_type(int);
}

int __wrap_pubsub_publish(
    void *pubsub_specific_data_p, char *error_message, size_t buffered_metrics, size_t buffered_bytes)
{
    function_called();
    check_expected_ptr(pubsub_specific_data_p);
    check_expected_ptr(error_message);
    check_expected(buffered_metrics);
    check_expected(buffered_bytes);
    return mock_type(int);
}

int __wrap_pubsub_get_result(
    void *pubsub_specific_data_p, char *error_message,
    size_t *sent_metrics, size_t *sent_bytes, size_t *lost_metrics, size_t *lost_bytes)
{
    function_called();
    check_expected_ptr(pubsub_specific_data_p);
    check_expected_ptr(error_message);
    check_expected_ptr(sent_metrics);
    check_expected_ptr(sent_bytes);
    check_expected_ptr(lost_metrics);
    check_expected_ptr(lost_bytes);
    return mock_type(int);
}
#endif // ENABLE_EXPORTING_PUBSUB

#if HAVE_MONGOC
void __wrap_mongoc_init()
{
    function_called();
}

mongoc_uri_t * __wrap_mongoc_uri_new_with_error (const char *uri_string, bson_error_t *error)
{
    function_called();
    check_expected_ptr(uri_string);
    check_expected_ptr(error);
    return mock_ptr_type(mongoc_uri_t *);
}

int32_t __wrap_mongoc_uri_get_option_as_int32(const mongoc_uri_t *uri, const char *option, int32_t fallback)
{
    function_called();
    check_expected_ptr(uri);
    check_expected_ptr(option);
    check_expected(fallback);
    return mock_type(int32_t);
}

bool __wrap_mongoc_uri_set_option_as_int32 (const mongoc_uri_t *uri, const char *option, int32_t value)
{
    function_called();
    check_expected_ptr(uri);
    check_expected_ptr(option);
    check_expected(value);
    return mock_type(bool);
}

mongoc_client_t * __wrap_mongoc_client_new_from_uri (const mongoc_uri_t *uri)
{
    function_called();
    check_expected_ptr(uri);
    return mock_ptr_type(mongoc_client_t *);
}

bool __wrap_mongoc_client_set_appname (mongoc_client_t *client, const char *appname)
{
    function_called();
    check_expected_ptr(client);
    check_expected_ptr(appname);
    return mock_type(bool);
}

mongoc_collection_t *
__wrap_mongoc_client_get_collection(mongoc_client_t *client, const char *db, const char *collection)
{
    function_called();
    check_expected_ptr(client);
    check_expected_ptr(db);
    check_expected_ptr(collection);
    return mock_ptr_type(mongoc_collection_t *);
}

void __wrap_mongoc_uri_destroy (mongoc_uri_t *uri)
{
    function_called();
    check_expected_ptr(uri);
}

bool __wrap_mongoc_collection_insert_many(
    mongoc_collection_t *collection,
    const bson_t **documents,
    size_t n_documents,
    const bson_t *opts,
    bson_t *reply,
    bson_error_t *error)
{
    function_called();
    check_expected_ptr(collection);
    check_expected_ptr(documents);
    check_expected(n_documents);
    check_expected_ptr(opts);
    check_expected_ptr(reply);
    check_expected_ptr(error);
    return mock_type(bool);
}
#endif // HAVE_MONGOC

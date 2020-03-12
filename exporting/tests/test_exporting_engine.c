// SPDX-License-Identifier: GPL-3.0-or-later

#include "test_exporting_engine.h"
#include "libnetdata/required_dummies.h"

RRDHOST *localhost;
netdata_rwlock_t rrd_rwlock;

// global variables needed by read_exporting_config()
struct config netdata_config;
char *netdata_configured_user_config_dir = ".";
char *netdata_configured_stock_config_dir = ".";
char *netdata_configured_hostname = "test_host";

char log_line[MAX_LOG_LINE + 1];

void init_connectors_in_tests(struct engine *engine)
{
    expect_function_call(__wrap_now_realtime_sec);
    will_return(__wrap_now_realtime_sec, 2);

    expect_function_call(__wrap_uv_thread_create);

    expect_value(__wrap_uv_thread_create, thread, &engine->instance_root->thread);
    expect_value(__wrap_uv_thread_create, worker, simple_connector_worker);
    expect_value(__wrap_uv_thread_create, arg, engine->instance_root);

    expect_function_call(__wrap_uv_thread_set_name_np);

    assert_int_equal(__real_init_connectors(engine), 0);

    assert_int_equal(engine->now, 2);
    assert_int_equal(engine->instance_root->after, 2);
}

static void test_exporting_engine(void **state)
{
    struct engine *engine = *state;

    expect_function_call(__wrap_read_exporting_config);
    will_return(__wrap_read_exporting_config, engine);

    expect_function_call(__wrap_init_connectors);
    expect_memory(__wrap_init_connectors, engine, engine, sizeof(struct engine));
    will_return(__wrap_init_connectors, 0);

    expect_function_call(__wrap_now_realtime_sec);
    will_return(__wrap_now_realtime_sec, 2);

    expect_function_call(__wrap_mark_scheduled_instances);
    expect_memory(__wrap_mark_scheduled_instances, engine, engine, sizeof(struct engine));
    will_return(__wrap_mark_scheduled_instances, 1);

    expect_function_call(__wrap_prepare_buffers);
    expect_memory(__wrap_prepare_buffers, engine, engine, sizeof(struct engine));
    will_return(__wrap_prepare_buffers, 0);

    expect_function_call(__wrap_notify_workers);
    expect_memory(__wrap_notify_workers, engine, engine, sizeof(struct engine));
    will_return(__wrap_notify_workers, 0);

    expect_function_call(__wrap_send_internal_metrics);
    expect_memory(__wrap_send_internal_metrics, engine, engine, sizeof(struct engine));
    will_return(__wrap_send_internal_metrics, 0);

    expect_function_call(__wrap_info_int);

    void *ptr = malloc(sizeof(struct netdata_static_thread));
    assert_ptr_equal(exporting_main(ptr), NULL);
    assert_int_equal(engine->now, 2);
    free(ptr);
}

static void test_read_exporting_config(void **state)
{
    struct engine *engine = __mock_read_exporting_config(); // TODO: use real read_exporting_config() function
    *state = engine;

    assert_ptr_not_equal(engine, NULL);
    assert_string_equal(engine->config.prefix, "netdata");
    assert_string_equal(engine->config.hostname, "test-host");
    assert_int_equal(engine->config.update_every, 3);
    assert_int_equal(engine->instance_num, 0);


    struct instance *instance = engine->instance_root;
    assert_ptr_not_equal(instance, NULL);
    assert_ptr_equal(instance->next, NULL);
    assert_ptr_equal(instance->engine, engine);
    assert_int_equal(instance->config.type, BACKEND_TYPE_GRAPHITE);
    assert_string_equal(instance->config.destination, "localhost");
    assert_int_equal(instance->config.update_every, 1);
    assert_int_equal(instance->config.buffer_on_failures, 10);
    assert_int_equal(instance->config.timeoutms, 10000);
    assert_true(simple_pattern_matches(instance->config.charts_pattern, "any_chart"));
    assert_true(simple_pattern_matches(instance->config.hosts_pattern, "anyt_host"));
    assert_int_equal(instance->config.options, EXPORTING_SOURCE_DATA_AS_COLLECTED | EXPORTING_OPTION_SEND_NAMES);

    teardown_configured_engine(state);
}

static void test_init_connectors(void **state)
{
    struct engine *engine = *state;

    init_connectors_in_tests(engine);

    assert_int_equal(engine->instance_num, 1);

    struct instance *instance = engine->instance_root;

    assert_ptr_equal(instance->next, NULL);
    assert_int_equal(instance->index, 0);

    struct simple_connector_config *connector_specific_config = instance->config.connector_specific_config;
    assert_int_equal(connector_specific_config->default_port, 2003);

    assert_ptr_equal(instance->worker, simple_connector_worker);
    assert_ptr_equal(instance->start_batch_formatting, NULL);
    assert_ptr_equal(instance->start_host_formatting, format_host_labels_graphite_plaintext);
    assert_ptr_equal(instance->start_chart_formatting, NULL);
    assert_ptr_equal(instance->metric_formatting, format_dimension_collected_graphite_plaintext);
    assert_ptr_equal(instance->end_chart_formatting, NULL);
    assert_ptr_equal(instance->end_host_formatting, flush_host_labels);
    assert_ptr_equal(instance->end_batch_formatting, NULL);

    BUFFER *buffer = instance->buffer;
    assert_ptr_not_equal(buffer, NULL);
    buffer_sprintf(buffer, "%s", "graphite test");
    assert_string_equal(buffer_tostring(buffer), "graphite test");
}

static void test_init_graphite_instance(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;

    instance->config.options = EXPORTING_SOURCE_DATA_AS_COLLECTED | EXPORTING_OPTION_SEND_NAMES;
    assert_int_equal(init_graphite_instance(instance), 0);
    assert_int_equal(
        ((struct simple_connector_config *)(instance->config.connector_specific_config))->default_port, 2003);
    freez(instance->config.connector_specific_config);
    assert_ptr_equal(instance->metric_formatting, format_dimension_collected_graphite_plaintext);
    assert_ptr_not_equal(instance->buffer, NULL);
    buffer_free(instance->buffer);

    instance->config.options = EXPORTING_SOURCE_DATA_AVERAGE | EXPORTING_OPTION_SEND_NAMES;
    assert_int_equal(init_graphite_instance(instance), 0);
    assert_ptr_equal(instance->metric_formatting, format_dimension_stored_graphite_plaintext);
}

static void test_init_json_instance(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;

    instance->config.options = EXPORTING_SOURCE_DATA_AS_COLLECTED | EXPORTING_OPTION_SEND_NAMES;
    assert_int_equal(init_json_instance(instance), 0);
    assert_int_equal(
        ((struct simple_connector_config *)(instance->config.connector_specific_config))->default_port, 5448);
    freez(instance->config.connector_specific_config);
    assert_ptr_equal(instance->metric_formatting, format_dimension_collected_json_plaintext);
    assert_ptr_not_equal(instance->buffer, NULL);
    buffer_free(instance->buffer);

    instance->config.options = EXPORTING_SOURCE_DATA_AVERAGE | EXPORTING_OPTION_SEND_NAMES;
    assert_int_equal(init_json_instance(instance), 0);
    assert_ptr_equal(instance->metric_formatting, format_dimension_stored_json_plaintext);
}

static void test_init_opentsdb_telnet_instance(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;

    instance->config.options = EXPORTING_SOURCE_DATA_AS_COLLECTED | EXPORTING_OPTION_SEND_NAMES;
    assert_int_equal(init_opentsdb_telnet_instance(instance), 0);
    assert_int_equal(
        ((struct simple_connector_config *)(instance->config.connector_specific_config))->default_port, 4242);
    freez(instance->config.connector_specific_config);
    assert_ptr_equal(instance->metric_formatting, format_dimension_collected_opentsdb_telnet);
    assert_ptr_not_equal(instance->buffer, NULL);
    buffer_free(instance->buffer);

    instance->config.options = EXPORTING_SOURCE_DATA_AVERAGE | EXPORTING_OPTION_SEND_NAMES;
    assert_int_equal(init_opentsdb_telnet_instance(instance), 0);
    assert_ptr_equal(instance->metric_formatting, format_dimension_stored_opentsdb_telnet);
}

static void test_init_opentsdb_http_instance(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;

    instance->config.options = EXPORTING_SOURCE_DATA_AS_COLLECTED | EXPORTING_OPTION_SEND_NAMES;
    assert_int_equal(init_opentsdb_http_instance(instance), 0);
    assert_int_equal(
        ((struct simple_connector_config *)(instance->config.connector_specific_config))->default_port, 4242);
    freez(instance->config.connector_specific_config);
    assert_ptr_equal(instance->metric_formatting, format_dimension_collected_opentsdb_http);
    assert_ptr_not_equal(instance->buffer, NULL);
    buffer_free(instance->buffer);

    instance->config.options = EXPORTING_SOURCE_DATA_AVERAGE | EXPORTING_OPTION_SEND_NAMES;
    assert_int_equal(init_opentsdb_http_instance(instance), 0);
    assert_ptr_equal(instance->metric_formatting, format_dimension_stored_opentsdb_http);
}

static void test_mark_scheduled_instances(void **state)
{
    struct engine *engine = *state;

    assert_int_equal(__real_mark_scheduled_instances(engine), 1);

    struct instance *instance = engine->instance_root;
    assert_int_equal(instance->scheduled, 1);
    assert_int_equal(instance->before, 2);
}

static void test_rrdhost_is_exportable(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;

    expect_function_call(__wrap_info_int);

    assert_ptr_equal(localhost->exporting_flags, NULL);

    assert_int_equal(__real_rrdhost_is_exportable(instance, localhost), 1);

    assert_string_equal(log_line, "enabled exporting of host 'localhost' for instance 'instance_name'");

    assert_ptr_not_equal(localhost->exporting_flags, NULL);
    assert_int_equal(localhost->exporting_flags[0], RRDHOST_FLAG_BACKEND_SEND);
}

static void test_false_rrdhost_is_exportable(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;

    simple_pattern_free(instance->config.hosts_pattern);
    instance->config.hosts_pattern = simple_pattern_create("!*", NULL, SIMPLE_PATTERN_EXACT);

    expect_function_call(__wrap_info_int);

    assert_ptr_equal(localhost->exporting_flags, NULL);

    assert_int_equal(__real_rrdhost_is_exportable(instance, localhost), 0);

    assert_string_equal(log_line, "disabled exporting of host 'localhost' for instance 'instance_name'");

    assert_ptr_not_equal(localhost->exporting_flags, NULL);
    assert_int_equal(localhost->exporting_flags[0], RRDHOST_FLAG_BACKEND_DONT_SEND);
}

static void test_rrdset_is_exportable(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;
    RRDSET *st = localhost->rrdset_root;

    assert_ptr_equal(st->exporting_flags, NULL);

    assert_int_equal(__real_rrdset_is_exportable(instance, st), 1);

    assert_ptr_not_equal(st->exporting_flags, NULL);
    assert_int_equal(st->exporting_flags[0], RRDSET_FLAG_BACKEND_SEND);
}

static void test_false_rrdset_is_exportable(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;
    RRDSET *st = localhost->rrdset_root;

    simple_pattern_free(instance->config.charts_pattern);
    instance->config.charts_pattern = simple_pattern_create("!*", NULL, SIMPLE_PATTERN_EXACT);

    assert_ptr_equal(st->exporting_flags, NULL);

    assert_int_equal(__real_rrdset_is_exportable(instance, st), 0);

    assert_ptr_not_equal(st->exporting_flags, NULL);
    assert_int_equal(st->exporting_flags[0], RRDSET_FLAG_BACKEND_IGNORE);
}

static void test_exporting_calculate_value_from_stored_data(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;
    RRDDIM *rd = localhost->rrdset_root->dimensions;
    time_t timestamp;

    instance->after = 3;
    instance->before = 10;

    expect_function_call(__mock_rrddim_query_oldest_time);
    will_return(__mock_rrddim_query_oldest_time, 1);

    expect_function_call(__mock_rrddim_query_latest_time);
    will_return(__mock_rrddim_query_latest_time, 2);

    expect_function_call(__mock_rrddim_query_init);
    expect_value(__mock_rrddim_query_init, start_time, 1);
    expect_value(__mock_rrddim_query_init, end_time, 2);

    expect_function_call(__mock_rrddim_query_is_finished);
    will_return(__mock_rrddim_query_is_finished, 0);
    expect_function_call(__mock_rrddim_query_next_metric);
    will_return(__mock_rrddim_query_next_metric, pack_storage_number(27, SN_EXISTS));

    expect_function_call(__mock_rrddim_query_is_finished);
    will_return(__mock_rrddim_query_is_finished, 0);
    expect_function_call(__mock_rrddim_query_next_metric);
    will_return(__mock_rrddim_query_next_metric, pack_storage_number(45, SN_EXISTS));

    expect_function_call(__mock_rrddim_query_is_finished);
    will_return(__mock_rrddim_query_is_finished, 1);

    expect_function_call(__mock_rrddim_query_finalize);

    assert_int_equal(__real_exporting_calculate_value_from_stored_data(instance, rd, &timestamp), 36);
}

static void test_prepare_buffers(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;

    instance->start_batch_formatting = __mock_start_batch_formatting;
    instance->start_host_formatting = __mock_start_host_formatting;
    instance->start_chart_formatting = __mock_start_chart_formatting;
    instance->metric_formatting = __mock_metric_formatting;
    instance->end_chart_formatting = __mock_end_chart_formatting;
    instance->end_host_formatting = __mock_end_host_formatting;
    instance->end_batch_formatting = __mock_end_batch_formatting;
    __real_mark_scheduled_instances(engine);

    expect_function_call(__mock_start_batch_formatting);
    expect_value(__mock_start_batch_formatting, instance, instance);
    will_return(__mock_start_batch_formatting, 0);

    expect_function_call(__wrap_rrdhost_is_exportable);
    expect_value(__wrap_rrdhost_is_exportable, instance, instance);
    expect_value(__wrap_rrdhost_is_exportable, host, localhost);
    will_return(__wrap_rrdhost_is_exportable, 1);

    expect_function_call(__mock_start_host_formatting);
    expect_value(__mock_start_host_formatting, instance, instance);
    expect_value(__mock_start_host_formatting, host, localhost);
    will_return(__mock_start_host_formatting, 0);

    RRDSET *st = localhost->rrdset_root;
    expect_function_call(__wrap_rrdset_is_exportable);
    expect_value(__wrap_rrdset_is_exportable, instance, instance);
    expect_value(__wrap_rrdset_is_exportable, st, st);
    will_return(__wrap_rrdset_is_exportable, 1);

    expect_function_call(__mock_start_chart_formatting);
    expect_value(__mock_start_chart_formatting, instance, instance);
    expect_value(__mock_start_chart_formatting, st, st);
    will_return(__mock_start_chart_formatting, 0);

    RRDDIM *rd = localhost->rrdset_root->dimensions;
    expect_function_call(__mock_metric_formatting);
    expect_value(__mock_metric_formatting, instance, instance);
    expect_value(__mock_metric_formatting, rd, rd);
    will_return(__mock_metric_formatting, 0);

    expect_function_call(__mock_end_chart_formatting);
    expect_value(__mock_end_chart_formatting, instance, instance);
    expect_value(__mock_end_chart_formatting, st, st);
    will_return(__mock_end_chart_formatting, 0);

    expect_function_call(__mock_end_host_formatting);
    expect_value(__mock_end_host_formatting, instance, instance);
    expect_value(__mock_end_host_formatting, host, localhost);
    will_return(__mock_end_host_formatting, 0);

    expect_function_call(__mock_end_batch_formatting);
    expect_value(__mock_end_batch_formatting, instance, instance);
    will_return(__mock_end_batch_formatting, 0);

    assert_int_equal(__real_prepare_buffers(engine), 0);

    assert_int_equal(instance->stats.chart_buffered_metrics, 1);

    // check with NULL functions
    instance->start_batch_formatting = NULL;
    instance->start_host_formatting = NULL;
    instance->start_chart_formatting = NULL;
    instance->metric_formatting = NULL;
    instance->end_chart_formatting = NULL;
    instance->end_host_formatting = NULL;
    instance->end_batch_formatting = NULL;
    assert_int_equal(__real_prepare_buffers(engine), 0);

    assert_int_equal(instance->scheduled, 0);
    assert_int_equal(instance->after, 2);
}

static void test_exporting_name_copy(void **state)
{
    (void)state;

    char *source_name = "test.name-with/special#characters_";
    char destination_name[RRD_ID_LENGTH_MAX + 1];

    assert_int_equal(exporting_name_copy(destination_name, source_name, RRD_ID_LENGTH_MAX), 34);
    assert_string_equal(destination_name, "test.name_with_special_characters_");
}

static void test_format_dimension_collected_graphite_plaintext(void **state)
{
    struct engine *engine = *state;

    RRDDIM *rd = localhost->rrdset_root->dimensions;
    assert_int_equal(format_dimension_collected_graphite_plaintext(engine->instance_root, rd), 0);
    assert_string_equal(
        buffer_tostring(engine->instance_root->buffer),
        "netdata.test-host.chart_name.dimension_name;TAG1=VALUE1 TAG2=VALUE2 123000321 15051\n");
}

static void test_format_dimension_stored_graphite_plaintext(void **state)
{
    struct engine *engine = *state;

    expect_function_call(__wrap_exporting_calculate_value_from_stored_data);
    will_return(__wrap_exporting_calculate_value_from_stored_data, pack_storage_number(27, SN_EXISTS));

    RRDDIM *rd = localhost->rrdset_root->dimensions;
    assert_int_equal(format_dimension_stored_graphite_plaintext(engine->instance_root, rd), 0);
    assert_string_equal(
        buffer_tostring(engine->instance_root->buffer),
        "netdata.test-host.chart_name.dimension_name;TAG1=VALUE1 TAG2=VALUE2 690565856.0000000 15052\n");
}

static void test_format_dimension_collected_json_plaintext(void **state)
{
    struct engine *engine = *state;

    RRDDIM *rd = localhost->rrdset_root->dimensions;
    assert_int_equal(format_dimension_collected_json_plaintext(engine->instance_root, rd), 0);
    assert_string_equal(
        buffer_tostring(engine->instance_root->buffer),
        "{\"prefix\":\"netdata\",\"hostname\":\"test-host\",\"host_tags\":\"TAG1=VALUE1 TAG2=VALUE2\","
        "\"chart_id\":\"chart_id\",\"chart_name\":\"chart_name\",\"chart_family\":\"(null)\","
        "\"chart_context\":\"(null)\",\"chart_type\":\"(null)\",\"units\":\"(null)\",\"id\":\"dimension_id\","
        "\"name\":\"dimension_name\",\"value\":123000321,\"timestamp\":15051}\n");
}

static void test_format_dimension_stored_json_plaintext(void **state)
{
    struct engine *engine = *state;

    expect_function_call(__wrap_exporting_calculate_value_from_stored_data);
    will_return(__wrap_exporting_calculate_value_from_stored_data, pack_storage_number(27, SN_EXISTS));

    RRDDIM *rd = localhost->rrdset_root->dimensions;
    assert_int_equal(format_dimension_stored_json_plaintext(engine->instance_root, rd), 0);
    assert_string_equal(
        buffer_tostring(engine->instance_root->buffer),
        "{\"prefix\":\"netdata\",\"hostname\":\"test-host\",\"host_tags\":\"TAG1=VALUE1 TAG2=VALUE2\","
        "\"chart_id\":\"chart_id\",\"chart_name\":\"chart_name\",\"chart_family\":\"(null)\"," \
        "\"chart_context\": \"(null)\",\"chart_type\":\"(null)\",\"units\": \"(null)\",\"id\":\"dimension_id\","
        "\"name\":\"dimension_name\",\"value\":690565856.0000000,\"timestamp\": 15052}\n");
}

static void test_format_dimension_collected_opentsdb_telnet(void **state)
{
    struct engine *engine = *state;

    RRDDIM *rd = localhost->rrdset_root->dimensions;
    assert_int_equal(format_dimension_collected_opentsdb_telnet(engine->instance_root, rd), 0);
    assert_string_equal(
        buffer_tostring(engine->instance_root->buffer),
        "put netdata.chart_name.dimension_name 15051 123000321 host=test-host TAG1=VALUE1 TAG2=VALUE2\n");
}

static void test_format_dimension_stored_opentsdb_telnet(void **state)
{
    struct engine *engine = *state;

    expect_function_call(__wrap_exporting_calculate_value_from_stored_data);
    will_return(__wrap_exporting_calculate_value_from_stored_data, pack_storage_number(27, SN_EXISTS));

    RRDDIM *rd = localhost->rrdset_root->dimensions;
    assert_int_equal(format_dimension_stored_opentsdb_telnet(engine->instance_root, rd), 0);
    assert_string_equal(
        buffer_tostring(engine->instance_root->buffer),
        "put netdata.chart_name.dimension_name 15052 690565856.0000000 host=test-host TAG1=VALUE1 TAG2=VALUE2\n");
}

static void test_format_dimension_collected_opentsdb_http(void **state)
{
    struct engine *engine = *state;

    RRDDIM *rd = localhost->rrdset_root->dimensions;
    assert_int_equal(format_dimension_collected_opentsdb_http(engine->instance_root, rd), 0);
    assert_string_equal(
        buffer_tostring(engine->instance_root->buffer),
        "POST /api/put HTTP/1.1\r\n"
        "Host: test-host\r\n"
        "Content-Type: application/json\r\n"
        "Content-Length: 153\r\n\r\n"
        "{  \"metric\": \"netdata.chart_name.dimension_name\",  "
        "\"timestamp\": 15051,  "
        "\"value\": 123000321,  "
        "\"tags\": {    \"host\": \"test-host TAG1=VALUE1 TAG2=VALUE2\"  }}");
}

static void test_format_dimension_stored_opentsdb_http(void **state)
{
    struct engine *engine = *state;

    expect_function_call(__wrap_exporting_calculate_value_from_stored_data);
    will_return(__wrap_exporting_calculate_value_from_stored_data, pack_storage_number(27, SN_EXISTS));

    RRDDIM *rd = localhost->rrdset_root->dimensions;
    assert_int_equal(format_dimension_stored_opentsdb_http(engine->instance_root, rd), 0);
    assert_string_equal(
        buffer_tostring(engine->instance_root->buffer),
        "POST /api/put HTTP/1.1\r\n"
        "Host: test-host\r\n"
        "Content-Type: application/json\r\n"
        "Content-Length: 161\r\n\r\n"
        "{  \"metric\": \"netdata.chart_name.dimension_name\",  "
        "\"timestamp\": 15052,  "
        "\"value\": 690565856.0000000,  "
        "\"tags\": {    \"host\": \"test-host TAG1=VALUE1 TAG2=VALUE2\"  }}");
}

static void test_exporting_discard_response(void **state)
{
    struct engine *engine = *state;

    BUFFER *response = buffer_create(0);
    buffer_sprintf(response, "Test response");

    expect_function_call(__wrap_info_int);

    assert_int_equal(exporting_discard_response(response, engine->instance_root), 0);

    assert_string_equal(
        log_line,
        "EXPORTING: received 13 bytes from instance_name connector instance. Ignoring them. Sample: 'Test response'");

    assert_int_equal(buffer_strlen(response), 0);

    buffer_free(response);
}

static void test_simple_connector_receive_response(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;
    struct stats *stats = &instance->stats;

    int sock = 1;

    expect_function_call(__wrap_recv);
    expect_value(__wrap_recv, sockfd, 1);
    expect_not_value(__wrap_recv, buf, 0);
    expect_value(__wrap_recv, len, 4096);
    expect_value(__wrap_recv, flags, MSG_DONTWAIT);

    expect_function_call(__wrap_info_int);

    simple_connector_receive_response(&sock, instance);

    assert_string_equal(
        log_line,
        "EXPORTING: received 9 bytes from instance_name connector instance. Ignoring them. Sample: 'Test recv'");

    assert_int_equal(stats->chart_received_bytes, 9);
    assert_int_equal(stats->chart_receptions, 1);
    assert_int_equal(sock, 1);
}

static void test_simple_connector_send_buffer(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;
    struct stats *stats = &instance->stats;
    BUFFER *buffer = instance->buffer;

    int sock = 1;
    int failures = 3;

    __real_mark_scheduled_instances(engine);

    expect_function_call(__wrap_rrdhost_is_exportable);
    expect_value(__wrap_rrdhost_is_exportable, instance, instance);
    expect_value(__wrap_rrdhost_is_exportable, host, localhost);
    will_return(__wrap_rrdhost_is_exportable, 1);

    RRDSET *st = localhost->rrdset_root;
    expect_function_call(__wrap_rrdset_is_exportable);
    expect_value(__wrap_rrdset_is_exportable, instance, instance);
    expect_value(__wrap_rrdset_is_exportable, st, st);
    will_return(__wrap_rrdset_is_exportable, 1);

    __real_prepare_buffers(engine);

    expect_function_call(__wrap_send);
    expect_value(__wrap_send, sockfd, 1);
    expect_value(__wrap_send, buf, buffer_tostring(buffer));
    expect_string(
        __wrap_send, buf, "netdata.test-host.chart_name.dimension_name;TAG1=VALUE1 TAG2=VALUE2 123000321 15051\n");
    expect_value(__wrap_send, len, 84);
    expect_value(__wrap_send, flags, MSG_NOSIGNAL);

    simple_connector_send_buffer(&sock, &failures, instance);

    assert_int_equal(failures, 0);
    assert_int_equal(stats->chart_transmission_successes, 1);
    assert_int_equal(stats->chart_sent_bytes, 84);
    assert_int_equal(stats->chart_sent_metrics, 1);
    assert_int_equal(stats->chart_transmission_failures, 0);

    assert_int_equal(buffer_strlen(buffer), 0);

    assert_int_equal(sock, 1);
}

static void test_simple_connector_worker(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;
    BUFFER *buffer = instance->buffer;

    __real_mark_scheduled_instances(engine);

    expect_function_call(__wrap_rrdhost_is_exportable);
    expect_value(__wrap_rrdhost_is_exportable, instance, instance);
    expect_value(__wrap_rrdhost_is_exportable, host, localhost);
    will_return(__wrap_rrdhost_is_exportable, 1);

    RRDSET *st = localhost->rrdset_root;
    expect_function_call(__wrap_rrdset_is_exportable);
    expect_value(__wrap_rrdset_is_exportable, instance, instance);
    expect_value(__wrap_rrdset_is_exportable, st, st);
    will_return(__wrap_rrdset_is_exportable, 1);

    __real_prepare_buffers(engine);

    expect_function_call(__wrap_connect_to_one_of);
    expect_string(__wrap_connect_to_one_of, destination, "localhost");
    expect_value(__wrap_connect_to_one_of, default_port, 2003);
    expect_not_value(__wrap_connect_to_one_of, reconnects_counter, 0);
    expect_value(__wrap_connect_to_one_of, connected_to, 0);
    expect_value(__wrap_connect_to_one_of, connected_to_size, 0);
    will_return(__wrap_connect_to_one_of, 2);

    expect_function_call(__wrap_send);
    expect_value(__wrap_send, sockfd, 2);
    expect_value(__wrap_send, buf, buffer_tostring(buffer));
    expect_string(
        __wrap_send, buf, "netdata.test-host.chart_name.dimension_name;TAG1=VALUE1 TAG2=VALUE2 123000321 15051\n");
    expect_value(__wrap_send, len, 84);
    expect_value(__wrap_send, flags, MSG_NOSIGNAL);

    simple_connector_worker(instance);
}

static void test_sanitize_json_string(void **state)
{
    (void)state;

    char *src = "check \t\\\" string";
    char dst[19 + 1];

    sanitize_json_string(dst, src, 19);

    assert_string_equal(dst, "check _\\\\\\\" string");
}

static void test_sanitize_graphite_label_value(void **state)
{
    (void)state;

    char *src = "check ;~ string";
    char dst[15 + 1];

    sanitize_graphite_label_value(dst, src, 15);

    assert_string_equal(dst, "check____string");
}

static void test_sanitize_opentsdb_label_value(void **state)
{
    (void)state;

    char *src = "check \t\\\" #&$? -_./ string";
    char dst[26 + 1];

    sanitize_opentsdb_label_value(dst, src, 26);

    assert_string_equal(dst, "check__________-_./_string");
}

static void test_format_host_labels_json_plaintext(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;

    instance->config.options |= EXPORTING_OPTION_SEND_CONFIGURED_LABELS;
    instance->config.options |= EXPORTING_OPTION_SEND_AUTOMATIC_LABELS;

    assert_int_equal(format_host_labels_json_plaintext(instance, localhost), 0);
    assert_string_equal(buffer_tostring(instance->labels), "\"labels\":{\"key1\":\"value1\",\"key2\":\"value2\"},");
}

static void test_format_host_labels_graphite_plaintext(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;

    instance->config.options |= EXPORTING_OPTION_SEND_CONFIGURED_LABELS;
    instance->config.options |= EXPORTING_OPTION_SEND_AUTOMATIC_LABELS;

    assert_int_equal(format_host_labels_graphite_plaintext(instance, localhost), 0);
    assert_string_equal(buffer_tostring(instance->labels), ";key1=value1;key2=value2");
}

static void test_format_host_labels_opentsdb_telnet(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;

    instance->config.options |= EXPORTING_OPTION_SEND_CONFIGURED_LABELS;
    instance->config.options |= EXPORTING_OPTION_SEND_AUTOMATIC_LABELS;

    assert_int_equal(format_host_labels_opentsdb_telnet(instance, localhost), 0);
    assert_string_equal(buffer_tostring(instance->labels), " key1=value1 key2=value2");
}

static void test_format_host_labels_opentsdb_http(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;

    instance->config.options |= EXPORTING_OPTION_SEND_CONFIGURED_LABELS;
    instance->config.options |= EXPORTING_OPTION_SEND_AUTOMATIC_LABELS;

    assert_int_equal(format_host_labels_opentsdb_http(instance, localhost), 0);
    assert_string_equal(buffer_tostring(instance->labels), ",\"key1\":\"value1\",\"key2\":\"value2\"");
}

static void test_flush_host_labels(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;

    instance->labels = buffer_create(12);
    buffer_strcat(instance->labels, "check string");
    assert_int_equal(buffer_strlen(instance->labels), 12);

    assert_int_equal(flush_host_labels(instance, localhost), 0);
    assert_int_equal(buffer_strlen(instance->labels), 0);
}

#if ENABLE_PROMETHEUS_REMOTE_WRITE
static void test_init_prometheus_remote_write_instance(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;

    expect_function_call(__wrap_init_write_request);
    will_return(__wrap_init_write_request, 0xff);

    assert_int_equal(init_prometheus_remote_write_instance(instance), 0);

    assert_ptr_equal(instance->worker, simple_connector_worker);
    assert_ptr_equal(instance->start_batch_formatting, NULL);
    assert_ptr_equal(instance->start_host_formatting, format_host_prometheus_remote_write);
    assert_ptr_equal(instance->start_chart_formatting, format_chart_prometheus_remote_write);
    assert_ptr_equal(instance->metric_formatting, format_dimension_prometheus_remote_write);
    assert_ptr_equal(instance->end_chart_formatting, NULL);
    assert_ptr_equal(instance->end_host_formatting, NULL);
    assert_ptr_equal(instance->end_batch_formatting, format_batch_prometheus_remote_write);
    assert_ptr_equal(instance->send_header, prometheus_remote_write_send_header);
    assert_ptr_equal(instance->check_response, process_prometheus_remote_write_response);

    assert_ptr_not_equal(instance->buffer, NULL);
    buffer_free(instance->buffer);

    struct prometheus_remote_write_specific_data *connector_specific_data =
        (struct prometheus_remote_write_specific_data *)instance->connector_specific_data;

    assert_ptr_not_equal(instance->connector_specific_data, NULL);
    assert_ptr_equal(connector_specific_data->write_request, 0xff);
    freez(instance->connector_specific_data);
}

static void test_prometheus_remote_write_send_header(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;
    int sock = 1;

    struct prometheus_remote_write_specific_config *connector_specific_config =
        callocz(1, sizeof(struct prometheus_remote_write_specific_config));
    instance->config.connector_specific_config = connector_specific_config;
    connector_specific_config->remote_write_path = strdupz("/receive");

    buffer_sprintf(instance->buffer, "test buffer");

    expect_function_call(__wrap_send);
    expect_value(__wrap_send, sockfd, 1);
    expect_not_value(__wrap_send, buf, NULL);
    expect_string(
        __wrap_send, buf,
        "POST /receive HTTP/1.1\r\n"
        "Host: test-host\r\n"
        "Accept: */*\r\n"
        "Content-Length: 11\r\n"
        "Content-Type: application/x-www-form-urlencoded\r\n\r\n");
    expect_value(__wrap_send, len, 125);
    expect_value(__wrap_send, flags, MSG_NOSIGNAL);

    assert_int_equal(prometheus_remote_write_send_header(&sock, instance),0);

    free(connector_specific_config->remote_write_path);
}

static void test_process_prometheus_remote_write_response(void **state)
{
    (void)state;
    BUFFER *buffer = buffer_create(0);

    buffer_sprintf(buffer, "HTTP/1.1 200 OK\r\n");
    assert_int_equal(process_prometheus_remote_write_response(buffer, NULL), 0);

    buffer_free(buffer);
}

static void test_format_host_prometheus_remote_write(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;

    instance->config.options |= EXPORTING_OPTION_SEND_CONFIGURED_LABELS;
    instance->config.options |= EXPORTING_OPTION_SEND_AUTOMATIC_LABELS;

    struct prometheus_remote_write_specific_data *connector_specific_data =
        mallocz(sizeof(struct prometheus_remote_write_specific_data *));
    instance->connector_specific_data = (void *)connector_specific_data;
    connector_specific_data->write_request = (void *)0xff;

    localhost->program_name = strdupz("test_program");
    localhost->program_version = strdupz("test_version");

    expect_function_call(__wrap_add_host_info);
    expect_value(__wrap_add_host_info, write_request_p, 0xff);
    expect_string(__wrap_add_host_info, name, "netdata_info");
    expect_string(__wrap_add_host_info, instance, "test-host");
    expect_string(__wrap_add_host_info, application, "test_program");
    expect_string(__wrap_add_host_info, version, "test_version");
    expect_in_range(
        __wrap_add_host_info, timestamp, now_realtime_usec() / USEC_PER_MS - 1000, now_realtime_usec() / USEC_PER_MS);

    expect_function_call(__wrap_add_label);
    expect_value(__wrap_add_label, write_request_p, 0xff);
    expect_string(__wrap_add_label, key, "key1");
    expect_string(__wrap_add_label, value, "value1");

    expect_function_call(__wrap_add_label);
    expect_value(__wrap_add_label, write_request_p, 0xff);
    expect_string(__wrap_add_label, key, "key2");
    expect_string(__wrap_add_label, value, "value2");

    assert_int_equal(format_host_prometheus_remote_write(instance, localhost), 0);

    freez(connector_specific_data);
    free(localhost->program_name);
    free(localhost->program_version);
}

static void test_format_dimension_prometheus_remote_write(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;

    struct prometheus_remote_write_specific_data *connector_specific_data =
        mallocz(sizeof(struct prometheus_remote_write_specific_data *));
    instance->connector_specific_data = (void *)connector_specific_data;
    connector_specific_data->write_request = (void *)0xff;

    RRDDIM *rd = localhost->rrdset_root->dimensions;

    expect_function_call(__wrap_exporting_calculate_value_from_stored_data);
    will_return(__wrap_exporting_calculate_value_from_stored_data, pack_storage_number(27, SN_EXISTS));

    expect_function_call(__wrap_add_metric);
    expect_value(__wrap_add_metric, write_request_p, 0xff);
    expect_string(__wrap_add_metric, name, "netdata_");
    expect_string(__wrap_add_metric, chart, "");
    expect_string(__wrap_add_metric, family, "");
    expect_string(__wrap_add_metric, dimension, "dimension_name");
    expect_string(__wrap_add_metric, instance, "test-host");
    expect_value(__wrap_add_metric, value, 0x292932e0);
    expect_value(__wrap_add_metric, timestamp, 15052 * MSEC_PER_SEC);

    assert_int_equal(format_dimension_prometheus_remote_write(instance, rd), 0);
}

static void test_format_batch_prometheus_remote_write(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;

    struct prometheus_remote_write_specific_data *connector_specific_data =
        mallocz(sizeof(struct prometheus_remote_write_specific_data *));
    instance->connector_specific_data = (void *)connector_specific_data;
    connector_specific_data->write_request = __real_init_write_request();

    __real_add_host_info(
        connector_specific_data->write_request,
        "test_name", "test_instance", "test_application", "test_version", 15051);

    __real_add_label(connector_specific_data->write_request, "test_key", "test_value");

    __real_add_metric(
        connector_specific_data->write_request,
        "test_name", "test chart", "test_family", "test_dimension", "test_instance",
        123000321, 15052);

    assert_int_equal(format_batch_prometheus_remote_write(instance), 0);

    BUFFER *buffer = instance->buffer;
    assert_int_equal(buffer_strlen(buffer), 192);

    BUFFER *escaped_buffer = buffer_create(850);
    size_t len = buffer_strlen(buffer);
    char *ch = (char *)buffer_tostring(buffer);
    for (; len > 0; ch++, len--)
        buffer_sprintf(escaped_buffer, "\\%03o", (unsigned int)*ch);
    assert_string_equal(
        buffer_tostring(escaped_buffer),
        "\\37777777641\\002\\120\\012\\37777777622\\001\\012\\025\\012\\010\\137\\137\\156\\141\\155\\145\\137\\137"
        "\\022\\011\\164\\145\\163\\164\\005\\015\\064\\012\\031\\012\\010\\151\\156\\163\\164\\141\\156\\143\\145\\022"
        "\\015\\005\\027\\021\\017\\100\\012\\037\\012\\013\\141\\160\\160\\154\\151\\143\\141\\164\\151\\157\\156\\022"
        "\\020\\005\\036\\035\\022\\034\\012\\027\\012\\007\\166\\145\\162\\163\\001\\035\\000\\014\\005\\035\\015\\016"
        "\\014\\012\\026\\012\\010\\005\\020\\020\\153\\145\\171\\022\\012\\005\\012\\040\\166\\141\\154\\165\\145\\022"
        "\\014\\011\\000\\005\\001\\030\\37777777760\\077\\020\\37777777713\\165\\012\\37777777611\\142\\37777777625"
        "\\000\\034\\023\\012\\005\\143\\150\\141\\162\\164\\011\\075\\000\\040\\005\\014\\054\\012\\025\\012\\006\\146"
        "\\141\\155\\151\\154\\171\\022\\013\\005\\123\\011\\015\\040\\012\\033\\012\\011\\144\\151\\155\\145\\156\\005"
        "\\37777777607\\000\\016\\005\\032\\025\\020\\000\\012\\146\\37777777736\\000\\064\\022\\014\\011\\000\\000\\000"
        "\\004\\130\\123\\37777777635\\101\\020\\37777777714\\165");

    buffer_free(escaped_buffer);
    protocol_buffers_shutdown();
}
#endif // ENABLE_PROMETHEUS_REMOTE_WRITE

#if HAVE_KINESIS
static void test_init_aws_kinesis_instance(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;

    instance->config.options = EXPORTING_SOURCE_DATA_AS_COLLECTED | EXPORTING_OPTION_SEND_NAMES;

    struct aws_kinesis_specific_config *connector_specific_config =
        callocz(1, sizeof(struct aws_kinesis_specific_config));
    instance->config.connector_specific_config = connector_specific_config;
    connector_specific_config->stream_name = strdupz("test_stream");
    connector_specific_config->auth_key_id = strdupz("test_auth_key_id");
    connector_specific_config->secure_key = strdupz("test_secure_key");

    expect_function_call(__wrap_aws_sdk_init);
    expect_function_call(__wrap_kinesis_init);
    expect_not_value(__wrap_kinesis_init, kinesis_specific_data_p, NULL);
    expect_string(__wrap_kinesis_init, region, "localhost");
    expect_string(__wrap_kinesis_init, access_key_id, "test_auth_key_id");
    expect_string(__wrap_kinesis_init, secret_key, "test_secure_key");
    expect_value(__wrap_kinesis_init, timeout, 10000);
    assert_int_equal(init_aws_kinesis_instance(instance), 0);

    assert_ptr_equal(instance->worker, aws_kinesis_connector_worker);
    assert_ptr_equal(instance->start_batch_formatting, NULL);
    assert_ptr_equal(instance->start_host_formatting, format_host_labels_json_plaintext);
    assert_ptr_equal(instance->start_chart_formatting, NULL);
    assert_ptr_equal(instance->metric_formatting, format_dimension_collected_json_plaintext);
    assert_ptr_equal(instance->end_chart_formatting, NULL);
    assert_ptr_equal(instance->end_host_formatting, flush_host_labels);
    assert_ptr_equal(instance->end_batch_formatting, NULL);
    assert_ptr_not_equal(instance->buffer, NULL);
    buffer_free(instance->buffer);
    assert_ptr_not_equal(instance->connector_specific_data, NULL);
    freez(instance->connector_specific_data);

    instance->config.options = EXPORTING_SOURCE_DATA_AVERAGE | EXPORTING_OPTION_SEND_NAMES;

    expect_function_call(__wrap_kinesis_init);
    expect_not_value(__wrap_kinesis_init, kinesis_specific_data_p, NULL);
    expect_string(__wrap_kinesis_init, region, "localhost");
    expect_string(__wrap_kinesis_init, access_key_id, "test_auth_key_id");
    expect_string(__wrap_kinesis_init, secret_key, "test_secure_key");
    expect_value(__wrap_kinesis_init, timeout, 10000);

    assert_int_equal(init_aws_kinesis_instance(instance), 0);
    assert_ptr_equal(instance->metric_formatting, format_dimension_stored_json_plaintext);

    free(connector_specific_config->stream_name);
    free(connector_specific_config->auth_key_id);
    free(connector_specific_config->secure_key);
}

static void test_aws_kinesis_connector_worker(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;
    BUFFER *buffer = instance->buffer;

    __real_mark_scheduled_instances(engine);

    expect_function_call(__wrap_rrdhost_is_exportable);
    expect_value(__wrap_rrdhost_is_exportable, instance, instance);
    expect_value(__wrap_rrdhost_is_exportable, host, localhost);
    will_return(__wrap_rrdhost_is_exportable, 1);

    RRDSET *st = localhost->rrdset_root;
    expect_function_call(__wrap_rrdset_is_exportable);
    expect_value(__wrap_rrdset_is_exportable, instance, instance);
    expect_value(__wrap_rrdset_is_exportable, st, st);
    will_return(__wrap_rrdset_is_exportable, 1);

    __real_prepare_buffers(engine);

    struct aws_kinesis_specific_config *connector_specific_config =
        callocz(1, sizeof(struct aws_kinesis_specific_config));
    instance->config.connector_specific_config = connector_specific_config;
    connector_specific_config->stream_name = strdupz("test_stream");
    connector_specific_config->auth_key_id = strdupz("test_auth_key_id");
    connector_specific_config->secure_key = strdupz("test_secure_key");

    struct aws_kinesis_specific_data *connector_specific_data = callocz(1, sizeof(struct aws_kinesis_specific_data));
    instance->connector_specific_data = (void *)connector_specific_data;

    expect_function_call(__wrap_kinesis_put_record);
    expect_not_value(__wrap_kinesis_put_record, kinesis_specific_data_p, NULL);
    expect_string(__wrap_kinesis_put_record, stream_name, "test_stream");
    expect_string(__wrap_kinesis_put_record, partition_key, "netdata_0");
    expect_value(__wrap_kinesis_put_record, data, buffer_tostring(buffer));
    // The buffer is prepared by Graphite exporting connector
    expect_string(
        __wrap_kinesis_put_record, data,
        "netdata.test-host.chart_name.dimension_name;TAG1=VALUE1 TAG2=VALUE2 123000321 15051\n");
    expect_value(__wrap_kinesis_put_record, data_len, 84);

    expect_function_call(__wrap_kinesis_get_result);
    expect_value(__wrap_kinesis_get_result, request_outcomes_p, NULL);
    expect_not_value(__wrap_kinesis_get_result, error_message, NULL);
    expect_not_value(__wrap_kinesis_get_result, sent_bytes, NULL);
    expect_not_value(__wrap_kinesis_get_result, lost_bytes, NULL);
    will_return(__wrap_kinesis_get_result, 0);

    aws_kinesis_connector_worker(instance);

    free(connector_specific_config->stream_name);
    free(connector_specific_config->auth_key_id);
    free(connector_specific_config->secure_key);
}
#endif // HAVE_KINESIS

int main(void)
{
    const struct CMUnitTest tests[] = {
        cmocka_unit_test_setup_teardown(test_exporting_engine, setup_initialized_engine, teardown_initialized_engine),
        cmocka_unit_test(test_read_exporting_config),
        cmocka_unit_test_setup_teardown(test_init_connectors, setup_configured_engine, teardown_configured_engine),
        cmocka_unit_test_setup_teardown(
            test_init_graphite_instance, setup_configured_engine, teardown_configured_engine),
        cmocka_unit_test_setup_teardown(
            test_init_json_instance, setup_configured_engine, teardown_configured_engine),
        cmocka_unit_test_setup_teardown(
            test_init_opentsdb_telnet_instance, setup_configured_engine, teardown_configured_engine),
        cmocka_unit_test_setup_teardown(
            test_init_opentsdb_http_instance, setup_configured_engine, teardown_configured_engine),
        cmocka_unit_test_setup_teardown(
            test_mark_scheduled_instances, setup_initialized_engine, teardown_initialized_engine),
        cmocka_unit_test_setup_teardown(
            test_rrdhost_is_exportable, setup_initialized_engine, teardown_initialized_engine),
        cmocka_unit_test_setup_teardown(
            test_false_rrdhost_is_exportable, setup_initialized_engine, teardown_initialized_engine),
        cmocka_unit_test_setup_teardown(
            test_rrdset_is_exportable, setup_initialized_engine, teardown_initialized_engine),
        cmocka_unit_test_setup_teardown(
            test_false_rrdset_is_exportable, setup_initialized_engine, teardown_initialized_engine),
        cmocka_unit_test_setup_teardown(
            test_exporting_calculate_value_from_stored_data, setup_initialized_engine, teardown_initialized_engine),
        cmocka_unit_test_setup_teardown(test_prepare_buffers, setup_initialized_engine, teardown_initialized_engine),
        cmocka_unit_test(test_exporting_name_copy),
        cmocka_unit_test_setup_teardown(
            test_format_dimension_collected_graphite_plaintext, setup_initialized_engine, teardown_initialized_engine),
        cmocka_unit_test_setup_teardown(
            test_format_dimension_stored_graphite_plaintext, setup_initialized_engine, teardown_initialized_engine),
        cmocka_unit_test_setup_teardown(
            test_format_dimension_collected_json_plaintext, setup_initialized_engine, teardown_initialized_engine),
        cmocka_unit_test_setup_teardown(
            test_format_dimension_stored_json_plaintext, setup_initialized_engine, teardown_initialized_engine),
        cmocka_unit_test_setup_teardown(
            test_format_dimension_collected_opentsdb_telnet, setup_initialized_engine, teardown_initialized_engine),
        cmocka_unit_test_setup_teardown(
            test_format_dimension_stored_opentsdb_telnet, setup_initialized_engine, teardown_initialized_engine),
        cmocka_unit_test_setup_teardown(
            test_format_dimension_collected_opentsdb_http, setup_initialized_engine, teardown_initialized_engine),
        cmocka_unit_test_setup_teardown(
            test_format_dimension_stored_opentsdb_http, setup_initialized_engine, teardown_initialized_engine),
        cmocka_unit_test_setup_teardown(
            test_exporting_discard_response, setup_initialized_engine, teardown_initialized_engine),
        cmocka_unit_test_setup_teardown(
            test_simple_connector_receive_response, setup_initialized_engine, teardown_initialized_engine),
        cmocka_unit_test_setup_teardown(
            test_simple_connector_send_buffer, setup_initialized_engine, teardown_initialized_engine),
        cmocka_unit_test_setup_teardown(
            test_simple_connector_worker, setup_initialized_engine, teardown_initialized_engine),
    };

    const struct CMUnitTest label_tests[] = {
        cmocka_unit_test(test_sanitize_json_string),
        cmocka_unit_test(test_sanitize_graphite_label_value),
        cmocka_unit_test(test_sanitize_opentsdb_label_value),
        cmocka_unit_test_setup_teardown(
            test_format_host_labels_json_plaintext, setup_initialized_engine, teardown_initialized_engine),
        cmocka_unit_test_setup_teardown(
            test_format_host_labels_graphite_plaintext, setup_initialized_engine, teardown_initialized_engine),
        cmocka_unit_test_setup_teardown(
            test_format_host_labels_opentsdb_telnet, setup_initialized_engine, teardown_initialized_engine),
        cmocka_unit_test_setup_teardown(
            test_format_host_labels_opentsdb_http, setup_initialized_engine, teardown_initialized_engine),
        cmocka_unit_test_setup_teardown(test_flush_host_labels, setup_initialized_engine, teardown_initialized_engine),
    };

    int test_res = cmocka_run_group_tests_name("exporting_engine", tests, NULL, NULL) +
                   cmocka_run_group_tests_name("labels_in_exporting_engine", label_tests, NULL, NULL);

#if ENABLE_PROMETHEUS_REMOTE_WRITE
    const struct CMUnitTest prometheus_remote_write_tests[] = {
        cmocka_unit_test_setup_teardown(
            test_init_prometheus_remote_write_instance, setup_configured_engine, teardown_configured_engine),
        cmocka_unit_test_setup_teardown(
            test_prometheus_remote_write_send_header, setup_initialized_engine, teardown_initialized_engine),
        cmocka_unit_test(test_process_prometheus_remote_write_response),
        cmocka_unit_test_setup_teardown(
            test_format_host_prometheus_remote_write, setup_initialized_engine, teardown_initialized_engine),
        cmocka_unit_test_setup_teardown(
            test_format_dimension_prometheus_remote_write, setup_initialized_engine, teardown_initialized_engine),
        cmocka_unit_test_setup_teardown(
            test_format_batch_prometheus_remote_write, setup_initialized_engine, teardown_initialized_engine),
    };

    test_res += cmocka_run_group_tests_name(
        "prometheus_remote_write_exporting_connector", prometheus_remote_write_tests, NULL, NULL);
#endif

#if HAVE_KINESIS
    const struct CMUnitTest kinesis_tests[] = {
        cmocka_unit_test_setup_teardown(
            test_init_aws_kinesis_instance, setup_configured_engine, teardown_configured_engine),
        cmocka_unit_test_setup_teardown(
            test_aws_kinesis_connector_worker, setup_initialized_engine, teardown_initialized_engine),
    };

    test_res += cmocka_run_group_tests_name("kinesis_exporting_connector", kinesis_tests, NULL, NULL);
#endif

    return test_res;
}

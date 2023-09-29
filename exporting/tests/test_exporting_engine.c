// SPDX-License-Identifier: GPL-3.0-or-later

#include "test_exporting_engine.h"
#include "libnetdata/required_dummies.h"

RRDHOST *localhost;
netdata_rwlock_t rrd_rwlock;

// global variables needed by read_exporting_config()
struct config netdata_config;
char *netdata_configured_user_config_dir = ".";
char *netdata_configured_stock_config_dir = ".";
char *netdata_configured_hostname = "test_global_host";
bool global_statistics_enabled = true;

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

    expect_function_call(__wrap_create_main_rusage_chart);
    expect_not_value(__wrap_create_main_rusage_chart, st_rusage, NULL);
    expect_not_value(__wrap_create_main_rusage_chart, rd_user, NULL);
    expect_not_value(__wrap_create_main_rusage_chart, rd_system, NULL);

    expect_function_call(__wrap_now_realtime_sec);
    will_return(__wrap_now_realtime_sec, 2);

    expect_function_call(__wrap_mark_scheduled_instances);
    expect_memory(__wrap_mark_scheduled_instances, engine, engine, sizeof(struct engine));
    will_return(__wrap_mark_scheduled_instances, 1);

    expect_function_call(__wrap_prepare_buffers);
    expect_memory(__wrap_prepare_buffers, engine, engine, sizeof(struct engine));
    will_return(__wrap_prepare_buffers, 0);

    expect_function_call(__wrap_send_main_rusage);
    expect_value(__wrap_send_main_rusage, st_rusage, NULL);
    expect_value(__wrap_send_main_rusage, rd_user, NULL);
    expect_value(__wrap_send_main_rusage, rd_system, NULL);

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
    assert_string_equal(engine->config.hostname, "test_engine_host");
    assert_int_equal(engine->config.update_every, 3);
    assert_int_equal(engine->instance_num, 0);


    struct instance *instance = engine->instance_root;
    assert_ptr_not_equal(instance, NULL);
    assert_ptr_equal(instance->next, NULL);
    assert_ptr_equal(instance->engine, engine);
    assert_int_equal(instance->config.type, EXPORTING_CONNECTOR_TYPE_GRAPHITE);
    assert_string_equal(instance->config.destination, "localhost");
    assert_string_equal(instance->config.prefix, "netdata");
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
    assert_int_equal(localhost->exporting_flags[0], RRDHOST_FLAG_EXPORTING_SEND);
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
    assert_int_equal(localhost->exporting_flags[0], RRDHOST_FLAG_EXPORTING_DONT_SEND);
}

static void test_rrdset_is_exportable(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;
    RRDSET *st;
    rrdset_foreach_read(st, localhost);
        break;
    rrdset_foreach_done(st);

    assert_ptr_equal(st->exporting_flags, NULL);

    assert_int_equal(__real_rrdset_is_exportable(instance, st), 1);

    assert_ptr_not_equal(st->exporting_flags, NULL);
    assert_int_equal(st->exporting_flags[0], RRDSET_FLAG_EXPORTING_SEND);
}

static void test_false_rrdset_is_exportable(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;
    RRDSET *st;
    rrdset_foreach_read(st, localhost);
        break;
    rrdset_foreach_done(st);

    simple_pattern_free(instance->config.charts_pattern);
    instance->config.charts_pattern = simple_pattern_create("!*", NULL, SIMPLE_PATTERN_EXACT);

    assert_ptr_equal(st->exporting_flags, NULL);

    assert_int_equal(__real_rrdset_is_exportable(instance, st), 0);

    assert_ptr_not_equal(st->exporting_flags, NULL);
    assert_int_equal(st->exporting_flags[0], RRDSET_FLAG_EXPORTING_IGNORE);
}

static void test_exporting_calculate_value_from_stored_data(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;
    
    RRDSET *st;
    rrdset_foreach_read(st, localhost);
        break;
    rrdset_foreach_done(st);

    RRDDIM *rd;
    rrddim_foreach_read(rd, st);
        break;
    rrddim_foreach_done(rd);

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

    expect_function_call(__mock_rrddim_query_is_finished);
    will_return(__mock_rrddim_query_is_finished, 0);
    expect_function_call(__mock_rrddim_query_next_metric);

    expect_function_call(__mock_rrddim_query_is_finished);
    will_return(__mock_rrddim_query_is_finished, 1);

    expect_function_call(__mock_rrddim_query_finalize);

    assert_float_equal(__real_exporting_calculate_value_from_stored_data(instance, rd, &timestamp), 36, 0.1);
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

    RRDSET *st;
    rrdset_foreach_read(st, localhost);
        break;
    rrdset_foreach_done(st);
    
    expect_function_call(__wrap_rrdset_is_exportable);
    expect_value(__wrap_rrdset_is_exportable, instance, instance);
    expect_value(__wrap_rrdset_is_exportable, st, st);
    will_return(__wrap_rrdset_is_exportable, 1);

    expect_function_call(__mock_start_chart_formatting);
    expect_value(__mock_start_chart_formatting, instance, instance);
    expect_value(__mock_start_chart_formatting, st, st);
    will_return(__mock_start_chart_formatting, 0);

    RRDDIM *rd;
    rrddim_foreach_read(rd, st);
        break;
    rrddim_foreach_done(rd);
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

    __real_prepare_buffers(engine);

    assert_int_equal(instance->stats.buffered_metrics, 1);

    // check with NULL functions
    instance->start_batch_formatting = NULL;
    instance->start_host_formatting = NULL;
    instance->start_chart_formatting = NULL;
    instance->metric_formatting = NULL;
    instance->end_chart_formatting = NULL;
    instance->end_host_formatting = NULL;
    instance->end_batch_formatting = NULL;
    __real_prepare_buffers(engine);

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

    RRDSET *st;
    rrdset_foreach_read(st, localhost);
        break;
    rrdset_foreach_done(st);

    RRDDIM *rd;
    rrddim_foreach_read(rd, st);
        break;
    rrddim_foreach_done(rd);
    assert_int_equal(format_dimension_collected_graphite_plaintext(engine->instance_root, rd), 0);
    assert_string_equal(
        buffer_tostring(engine->instance_root->buffer),
        "netdata.test-host.chart_name.dimension_name;TAG1=VALUE1 TAG2=VALUE2 123000321 15051\n");
}

static void test_format_dimension_stored_graphite_plaintext(void **state)
{
    struct engine *engine = *state;

    expect_function_call(__wrap_exporting_calculate_value_from_stored_data);
    will_return(__wrap_exporting_calculate_value_from_stored_data, pack_storage_number(27, SN_DEFAULT_FLAGS));

    RRDSET *st;
    rrdset_foreach_read(st, localhost);
        break;
    rrdset_foreach_done(st);
    
    RRDDIM *rd;
    rrddim_foreach_read(rd, st);
        break;
    rrddim_foreach_done(rd);
    assert_int_equal(format_dimension_stored_graphite_plaintext(engine->instance_root, rd), 0);
    assert_string_equal(
        buffer_tostring(engine->instance_root->buffer),
        "netdata.test-host.chart_name.dimension_name;TAG1=VALUE1 TAG2=VALUE2 690565856.0000000 15052\n");
}

static void test_format_dimension_collected_json_plaintext(void **state)
{
    struct engine *engine = *state;

    RRDSET *st;
    rrdset_foreach_read(st, localhost);
        break;
    rrdset_foreach_done(st);

    RRDDIM *rd;
    rrddim_foreach_read(rd, st);
        break;
    rrddim_foreach_done(rd);
    assert_int_equal(format_dimension_collected_json_plaintext(engine->instance_root, rd), 0);
    assert_string_equal(
        buffer_tostring(engine->instance_root->buffer),
        "{\"prefix\":\"netdata\",\"hostname\":\"test-host\",\"host_tags\":\"TAG1=VALUE1 TAG2=VALUE2\","
        "\"chart_id\":\"chart_id\",\"chart_name\":\"chart_name\",\"chart_family\":\"\","
        "\"chart_context\":\"\",\"chart_type\":\"\",\"units\":\"\",\"id\":\"dimension_id\","
        "\"name\":\"dimension_name\",\"value\":123000321,\"timestamp\":15051}\n");
}

static void test_format_dimension_stored_json_plaintext(void **state)
{
    struct engine *engine = *state;

    expect_function_call(__wrap_exporting_calculate_value_from_stored_data);
    will_return(__wrap_exporting_calculate_value_from_stored_data, pack_storage_number(27, SN_DEFAULT_FLAGS));

    RRDSET *st;
    rrdset_foreach_read(st, localhost);
        break;
    rrdset_foreach_done(st);

    RRDDIM *rd;
    rrddim_foreach_read(rd, st);
        break;
    rrddim_foreach_done(rd);
    assert_int_equal(format_dimension_stored_json_plaintext(engine->instance_root, rd), 0);
    assert_string_equal(
        buffer_tostring(engine->instance_root->buffer),
        "{\"prefix\":\"netdata\",\"hostname\":\"test-host\",\"host_tags\":\"TAG1=VALUE1 TAG2=VALUE2\","
        "\"chart_id\":\"chart_id\",\"chart_name\":\"chart_name\",\"chart_family\":\"\"," \
        "\"chart_context\": \"\",\"chart_type\":\"\",\"units\": \"\",\"id\":\"dimension_id\","
        "\"name\":\"dimension_name\",\"value\":690565856.0000000,\"timestamp\": 15052}\n");
}

static void test_format_dimension_collected_opentsdb_telnet(void **state)
{
    struct engine *engine = *state;

    RRDSET *st;
    rrdset_foreach_read(st, localhost);
        break;
    rrdset_foreach_done(st);

    RRDDIM *rd;
    rrddim_foreach_read(rd, st);
        break;
    rrddim_foreach_done(rd);
    assert_int_equal(format_dimension_collected_opentsdb_telnet(engine->instance_root, rd), 0);
    assert_string_equal(
        buffer_tostring(engine->instance_root->buffer),
        "put netdata.chart_name.dimension_name 15051 123000321 host=test-host TAG1=VALUE1 TAG2=VALUE2\n");
}

static void test_format_dimension_stored_opentsdb_telnet(void **state)
{
    struct engine *engine = *state;

    expect_function_call(__wrap_exporting_calculate_value_from_stored_data);
    will_return(__wrap_exporting_calculate_value_from_stored_data, pack_storage_number(27, SN_DEFAULT_FLAGS));

    RRDSET *st;
    rrdset_foreach_read(st, localhost);
        break;
    rrdset_foreach_done(st);

    RRDDIM *rd;
    rrddim_foreach_read(rd, st);
        break;
    rrddim_foreach_done(rd);
    assert_int_equal(format_dimension_stored_opentsdb_telnet(engine->instance_root, rd), 0);
    assert_string_equal(
        buffer_tostring(engine->instance_root->buffer),
        "put netdata.chart_name.dimension_name 15052 690565856.0000000 host=test-host TAG1=VALUE1 TAG2=VALUE2\n");
}

static void test_format_dimension_collected_opentsdb_http(void **state)
{
    struct engine *engine = *state;

    RRDSET *st;
    rrdset_foreach_read(st, localhost);
        break;
    rrdset_foreach_done(st);
    
    RRDDIM *rd;
    rrddim_foreach_read(rd, st);
        break;
    rrddim_foreach_done(rd);
    assert_int_equal(format_dimension_collected_opentsdb_http(engine->instance_root, rd), 0);
    assert_string_equal(
        buffer_tostring(engine->instance_root->buffer),
        "{\"metric\":\"netdata.chart_name.dimension_name\","
        "\"timestamp\":15051,"
        "\"value\":123000321,"
        "\"tags\":{\"host\":\"test-host TAG1=VALUE1 TAG2=VALUE2\"}}");
}

static void test_format_dimension_stored_opentsdb_http(void **state)
{
    struct engine *engine = *state;

    expect_function_call(__wrap_exporting_calculate_value_from_stored_data);
    will_return(__wrap_exporting_calculate_value_from_stored_data, pack_storage_number(27, SN_DEFAULT_FLAGS));

    RRDSET *st;
    rrdset_foreach_read(st, localhost);
        break;
    rrdset_foreach_done(st);
    
    RRDDIM *rd;
    rrddim_foreach_read(rd, st);
        break;
    rrddim_foreach_done(rd);
    assert_int_equal(format_dimension_stored_opentsdb_http(engine->instance_root, rd), 0);
    assert_string_equal(
        buffer_tostring(engine->instance_root->buffer),
        "{\"metric\":\"netdata.chart_name.dimension_name\","
        "\"timestamp\":15052,"
        "\"value\":690565856.0000000,"
        "\"tags\":{\"host\":\"test-host TAG1=VALUE1 TAG2=VALUE2\"}}");
}

static void test_exporting_discard_response(void **state)
{
    struct engine *engine = *state;

    BUFFER *response = buffer_create(0, NULL);
    buffer_sprintf(response, "Test response");

    assert_int_equal(exporting_discard_response(response, engine->instance_root), 0);
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

    simple_connector_receive_response(&sock, instance);

    assert_int_equal(stats->received_bytes, 9);
    assert_int_equal(stats->receptions, 1);
    assert_int_equal(sock, 1);
}

static void test_simple_connector_send_buffer(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;
    struct stats *stats = &instance->stats;

    int sock = 1;
    int failures = 3;
    size_t buffered_metrics = 1;
    BUFFER *header = buffer_create(0, NULL);
    BUFFER *buffer = buffer_create(0, NULL);
    buffer_strcat(header, "test header\n");
    buffer_strcat(buffer, "test buffer\n");

    expect_function_call(__wrap_send);
    expect_value(__wrap_send, sockfd, 1);
    expect_value(__wrap_send, buf, buffer_tostring(header));
    expect_string(__wrap_send, buf, "test header\n");
    expect_value(__wrap_send, len, 12);
    expect_value(__wrap_send, flags, MSG_NOSIGNAL);

    expect_function_call(__wrap_send);
    expect_value(__wrap_send, sockfd, 1);
    expect_value(__wrap_send, buf, buffer_tostring(buffer));
    expect_string(__wrap_send, buf, "test buffer\n");
    expect_value(__wrap_send, len, 12);
    expect_value(__wrap_send, flags, MSG_NOSIGNAL);

    simple_connector_send_buffer(&sock, &failures, instance, header, buffer, buffered_metrics);

    assert_int_equal(failures, 0);
    assert_int_equal(stats->transmission_successes, 1);
    assert_int_equal(stats->sent_bytes, 12);
    assert_int_equal(stats->sent_metrics, 1);
    assert_int_equal(stats->transmission_failures, 0);

    assert_int_equal(buffer_strlen(buffer), 0);

    assert_int_equal(sock, 1);
}

static void test_simple_connector_worker(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;
    struct stats *stats = &instance->stats;

    __real_mark_scheduled_instances(engine);

    struct simple_connector_data *simple_connector_data = callocz(1, sizeof(struct simple_connector_data));
    instance->connector_specific_data = simple_connector_data;
    simple_connector_data->last_buffer = callocz(1, sizeof(struct simple_connector_buffer));
    simple_connector_data->first_buffer = simple_connector_data->last_buffer;
    simple_connector_data->header = buffer_create(0, NULL);
    simple_connector_data->buffer = buffer_create(0, NULL);
    simple_connector_data->last_buffer->header = buffer_create(0, NULL);
    simple_connector_data->last_buffer->buffer = buffer_create(0, NULL);
    strcpy(simple_connector_data->connected_to, "localhost");

    buffer_sprintf(simple_connector_data->last_buffer->header, "test header");
    buffer_sprintf(simple_connector_data->last_buffer->buffer, "test buffer");

    expect_function_call(__wrap_now_realtime_sec);
    will_return(__wrap_now_realtime_sec, 2);

    expect_function_call(__wrap_now_realtime_sec);
    will_return(__wrap_now_realtime_sec, 2);

    expect_function_call(__wrap_now_realtime_sec);
    will_return(__wrap_now_realtime_sec, 2);

    expect_function_call(__wrap_send_internal_metrics);
    expect_value(__wrap_send_internal_metrics, instance, instance);
    will_return(__wrap_send_internal_metrics, 0);

    simple_connector_worker(instance);

    assert_int_equal(stats->buffered_metrics, 0);
    assert_int_equal(stats->buffered_bytes, 0);
    assert_int_equal(stats->received_bytes, 0);
    assert_int_equal(stats->sent_bytes, 0);
    assert_int_equal(stats->sent_metrics, 0);
    assert_int_equal(stats->lost_metrics, 0);
    assert_int_equal(stats->receptions, 0);
    assert_int_equal(stats->transmission_successes, 0);
    assert_int_equal(stats->transmission_failures, 0);
    assert_int_equal(stats->data_lost_events, 0);
    assert_int_equal(stats->lost_bytes, 0);
    assert_int_equal(stats->reconnects, 0);
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
    assert_string_equal(buffer_tostring(instance->labels_buffer), "\"labels\":{\"key1\":\"value1\",\"key2\":\"value2\"},");
}

static void test_format_host_labels_graphite_plaintext(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;

    instance->config.options |= EXPORTING_OPTION_SEND_CONFIGURED_LABELS;
    instance->config.options |= EXPORTING_OPTION_SEND_AUTOMATIC_LABELS;

    assert_int_equal(format_host_labels_graphite_plaintext(instance, localhost), 0);
    assert_string_equal(buffer_tostring(instance->labels_buffer), ";key1=value1;key2=value2");
}

static void test_format_host_labels_opentsdb_telnet(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;

    instance->config.options |= EXPORTING_OPTION_SEND_CONFIGURED_LABELS;
    instance->config.options |= EXPORTING_OPTION_SEND_AUTOMATIC_LABELS;

    assert_int_equal(format_host_labels_opentsdb_telnet(instance, localhost), 0);
    assert_string_equal(buffer_tostring(instance->labels_buffer), " key1=value1 key2=value2");
}

static void test_format_host_labels_opentsdb_http(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;

    instance->config.options |= EXPORTING_OPTION_SEND_CONFIGURED_LABELS;
    instance->config.options |= EXPORTING_OPTION_SEND_AUTOMATIC_LABELS;

    assert_int_equal(format_host_labels_opentsdb_http(instance, localhost), 0);
    assert_string_equal(buffer_tostring(instance->labels_buffer), ",\"key1\":\"value1\",\"key2\":\"value2\"");
}

static void test_flush_host_labels(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;

    instance->labels_buffer = buffer_create(12, NULL);
    buffer_strcat(instance->labels_buffer, "check string");
    assert_int_equal(buffer_strlen(instance->labels_buffer), 12);

    assert_int_equal(flush_host_labels(instance, localhost), 0);
    assert_int_equal(buffer_strlen(instance->labels_buffer), 0);
}

static void test_create_main_rusage_chart(void **state)
{
    UNUSED(state);

    RRDSET *st_rusage = calloc(1, sizeof(RRDSET));
    RRDDIM *rd_user = NULL;
    RRDDIM *rd_system = NULL;

    expect_function_call(rrdset_create_custom);
    expect_value(rrdset_create_custom, host, localhost);
    expect_string(rrdset_create_custom, type, "netdata");
    expect_string(rrdset_create_custom, id, "exporting_main_thread_cpu");
    expect_value(rrdset_create_custom, name, NULL);
    expect_string(rrdset_create_custom, family, "exporting");
    expect_string(rrdset_create_custom, context, "exporting_cpu_usage");
    expect_string(rrdset_create_custom, units, "milliseconds/s");
    expect_string(rrdset_create_custom, plugin, "exporting");
    expect_value(rrdset_create_custom, module, NULL);
    expect_value(rrdset_create_custom, priority, 130600);
    expect_value(rrdset_create_custom, update_every, localhost->rrd_update_every);
    expect_value(rrdset_create_custom, chart_type, RRDSET_TYPE_STACKED);
    will_return(rrdset_create_custom, st_rusage);

    expect_function_calls(rrddim_add_custom, 2);
    expect_value_count(rrddim_add_custom, st, st_rusage, 2);
    expect_value_count(rrddim_add_custom, name, NULL, 2);
    expect_value_count(rrddim_add_custom, multiplier, 1, 2);
    expect_value_count(rrddim_add_custom, divisor, 1000, 2);
    expect_value_count(rrddim_add_custom, algorithm, RRD_ALGORITHM_INCREMENTAL, 2);

    __real_create_main_rusage_chart(&st_rusage, &rd_user, &rd_system);

    free(st_rusage);
}

static void test_send_main_rusage(void **state)
{
    UNUSED(state);

    RRDSET *st_rusage = calloc(1, sizeof(RRDSET));
    st_rusage->counter_done = 1;

    expect_function_call(rrdset_next_usec);
    expect_value(rrdset_next_usec, st, st_rusage);

    expect_function_calls(rrddim_set_by_pointer, 2);
    expect_value_count(rrddim_set_by_pointer, st, st_rusage, 2);

    expect_function_call(rrdset_done);
    expect_value(rrdset_done, st, st_rusage);

    __real_send_main_rusage(st_rusage, NULL, NULL);

    free(st_rusage);
}

static void test_send_internal_metrics(void **state)
{
    UNUSED(state);

    struct instance *instance = calloc(1, sizeof(struct instance));
    instance->config.name = (const char *)strdupz("test_instance");
    instance->config.update_every = 2;

    struct stats *stats = &instance->stats;

    stats->st_metrics = calloc(1, sizeof(RRDSET));
    stats->st_metrics->counter_done = 1;
    stats->st_bytes = calloc(1, sizeof(RRDSET));
    stats->st_bytes->counter_done = 1;
    stats->st_ops = calloc(1, sizeof(RRDSET));
    stats->st_ops->counter_done = 1;
    stats->st_rusage = calloc(1, sizeof(RRDSET));
    stats->st_rusage->counter_done = 1;

    // ------------------------------------------------------------------------

    expect_function_call(rrdset_create_custom);
    expect_value(rrdset_create_custom, host, localhost);
    expect_string(rrdset_create_custom, type, "netdata");
    expect_string(rrdset_create_custom, id, "exporting_test_instance_metrics");
    expect_value(rrdset_create_custom, name, NULL);
    expect_string(rrdset_create_custom, family, "exporting_test_instance");
    expect_string(rrdset_create_custom, context, "exporting_buffer");
    expect_string(rrdset_create_custom, units, "metrics");
    expect_string(rrdset_create_custom, plugin, "exporting");
    expect_value(rrdset_create_custom, module, NULL);
    expect_value(rrdset_create_custom, priority, 130610);
    expect_value(rrdset_create_custom, update_every, 2);
    expect_value(rrdset_create_custom, chart_type, RRDSET_TYPE_LINE);
    will_return(rrdset_create_custom, stats->st_metrics);

    expect_function_calls(rrddim_add_custom, 3);
    expect_value_count(rrddim_add_custom, st, stats->st_metrics, 3);
    expect_value_count(rrddim_add_custom, name, NULL, 3);
    expect_value_count(rrddim_add_custom, multiplier, 1, 3);
    expect_value_count(rrddim_add_custom, divisor, 1, 3);
    expect_value_count(rrddim_add_custom, algorithm, RRD_ALGORITHM_ABSOLUTE, 3);

    // ------------------------------------------------------------------------

    expect_function_call(rrdset_create_custom);
    expect_value(rrdset_create_custom, host, localhost);
    expect_string(rrdset_create_custom, type, "netdata");
    expect_string(rrdset_create_custom, id, "exporting_test_instance_bytes");
    expect_value(rrdset_create_custom, name, NULL);
    expect_string(rrdset_create_custom, family, "exporting_test_instance");
    expect_string(rrdset_create_custom, context, "exporting_data_size");
    expect_string(rrdset_create_custom, units, "KiB");
    expect_string(rrdset_create_custom, plugin, "exporting");
    expect_value(rrdset_create_custom, module, NULL);
    expect_value(rrdset_create_custom, priority, 130620);
    expect_value(rrdset_create_custom, update_every, 2);
    expect_value(rrdset_create_custom, chart_type, RRDSET_TYPE_AREA);
    will_return(rrdset_create_custom, stats->st_bytes);

    expect_function_calls(rrddim_add_custom, 4);
    expect_value_count(rrddim_add_custom, st, stats->st_bytes, 4);
    expect_value_count(rrddim_add_custom, name, NULL, 4);
    expect_value_count(rrddim_add_custom, multiplier, 1, 4);
    expect_value_count(rrddim_add_custom, divisor, 1024, 4);
    expect_value_count(rrddim_add_custom, algorithm, RRD_ALGORITHM_ABSOLUTE, 4);

    // ------------------------------------------------------------------------

    expect_function_call(rrdset_create_custom);
    expect_value(rrdset_create_custom, host, localhost);
    expect_string(rrdset_create_custom, type, "netdata");
    expect_string(rrdset_create_custom, id, "exporting_test_instance_ops");
    expect_value(rrdset_create_custom, name, NULL);
    expect_string(rrdset_create_custom, family, "exporting_test_instance");
    expect_string(rrdset_create_custom, context, "exporting_operations");
    expect_string(rrdset_create_custom, units, "operations");
    expect_string(rrdset_create_custom, plugin, "exporting");
    expect_value(rrdset_create_custom, module, NULL);
    expect_value(rrdset_create_custom, priority, 130630);
    expect_value(rrdset_create_custom, update_every, 2);
    expect_value(rrdset_create_custom, chart_type, RRDSET_TYPE_LINE);
    will_return(rrdset_create_custom, stats->st_ops);

    expect_function_calls(rrddim_add_custom, 5);
    expect_value_count(rrddim_add_custom, st, stats->st_ops, 5);
    expect_value_count(rrddim_add_custom, name, NULL, 5);
    expect_value_count(rrddim_add_custom, multiplier, 1, 5);
    expect_value_count(rrddim_add_custom, divisor, 1, 5);
    expect_value_count(rrddim_add_custom, algorithm, RRD_ALGORITHM_ABSOLUTE, 5);

    // ------------------------------------------------------------------------

    expect_function_call(rrdset_create_custom);
    expect_value(rrdset_create_custom, host, localhost);
    expect_string(rrdset_create_custom, type, "netdata");
    expect_string(rrdset_create_custom, id, "exporting_test_instance_thread_cpu");
    expect_value(rrdset_create_custom, name, NULL);
    expect_string(rrdset_create_custom, family, "exporting_test_instance");
    expect_string(rrdset_create_custom, context, "exporting_instance");
    expect_string(rrdset_create_custom, units, "milliseconds/s");
    expect_string(rrdset_create_custom, plugin, "exporting");
    expect_value(rrdset_create_custom, module, NULL);
    expect_value(rrdset_create_custom, priority, 130640);
    expect_value(rrdset_create_custom, update_every, 2);
    expect_value(rrdset_create_custom, chart_type, RRDSET_TYPE_STACKED);
    will_return(rrdset_create_custom, stats->st_rusage);

    expect_function_calls(rrddim_add_custom, 2);
    expect_value_count(rrddim_add_custom, st, stats->st_rusage, 2);
    expect_value_count(rrddim_add_custom, name, NULL, 2);
    expect_value_count(rrddim_add_custom, multiplier, 1, 2);
    expect_value_count(rrddim_add_custom, divisor, 1000, 2);
    expect_value_count(rrddim_add_custom, algorithm, RRD_ALGORITHM_INCREMENTAL, 2);

    // ------------------------------------------------------------------------

    expect_function_call(rrdset_next_usec);
    expect_value(rrdset_next_usec, st, stats->st_metrics);

    expect_function_calls(rrddim_set_by_pointer, 3);
    expect_value_count(rrddim_set_by_pointer, st, stats->st_metrics, 3);

    expect_function_call(rrdset_done);
    expect_value(rrdset_done, st, stats->st_metrics);

    // ------------------------------------------------------------------------

    expect_function_call(rrdset_next_usec);
    expect_value(rrdset_next_usec, st, stats->st_bytes);

    expect_function_calls(rrddim_set_by_pointer, 4);
    expect_value_count(rrddim_set_by_pointer, st, stats->st_bytes, 4);

    expect_function_call(rrdset_done);
    expect_value(rrdset_done, st, stats->st_bytes);

    // ------------------------------------------------------------------------

    expect_function_call(rrdset_next_usec);
    expect_value(rrdset_next_usec, st, stats->st_ops);

    expect_function_calls(rrddim_set_by_pointer, 5);
    expect_value_count(rrddim_set_by_pointer, st, stats->st_ops, 5);

    expect_function_call(rrdset_done);
    expect_value(rrdset_done, st, stats->st_ops);

    // ------------------------------------------------------------------------

    expect_function_call(rrdset_next_usec);
    expect_value(rrdset_next_usec, st, stats->st_rusage);

    expect_function_calls(rrddim_set_by_pointer, 2);
    expect_value_count(rrddim_set_by_pointer, st, stats->st_rusage, 2);

    expect_function_call(rrdset_done);
    expect_value(rrdset_done, st, stats->st_rusage);

    // ------------------------------------------------------------------------

    __real_send_internal_metrics(instance);

    free(stats->st_metrics);
    free(stats->st_bytes);
    free(stats->st_ops);
    free(stats->st_rusage);
    free((void *)instance->config.name);
    free(instance);
}

static void test_can_send_rrdset(void **state)
{
    (void)*state;

    RRDSET *st;
    rrdset_foreach_read(st, localhost);
        break;
    rrdset_foreach_done(st);

    assert_int_equal(can_send_rrdset(prometheus_exporter_instance, st, NULL), 1);

    rrdset_flag_set(st, RRDSET_FLAG_EXPORTING_IGNORE);
    assert_int_equal(can_send_rrdset(prometheus_exporter_instance, st, NULL), 0);
    rrdset_flag_clear(st, RRDSET_FLAG_EXPORTING_IGNORE);

    // TODO: test with a denying simple pattern

    rrdset_flag_set(st, RRDSET_FLAG_OBSOLETE);
    assert_int_equal(can_send_rrdset(prometheus_exporter_instance, st, NULL), 0);
    rrdset_flag_clear(st, RRDSET_FLAG_OBSOLETE);

    st->rrd_memory_mode = RRD_MEMORY_MODE_NONE;
    prometheus_exporter_instance->config.options |= EXPORTING_SOURCE_DATA_AVERAGE;
    assert_int_equal(can_send_rrdset(prometheus_exporter_instance, st, NULL), 0);
}

static void test_prometheus_name_copy(void **state)
{
    (void)*state;

    char destination_name[PROMETHEUS_ELEMENT_MAX + 1];
    assert_int_equal(prometheus_name_copy(destination_name, "test-name", PROMETHEUS_ELEMENT_MAX), 9);

    assert_string_equal(destination_name, "test_name");
}

static void test_prometheus_label_copy(void **state)
{
    (void)*state;

    char destination_name[PROMETHEUS_ELEMENT_MAX + 1];
    assert_int_equal(prometheus_label_copy(destination_name, "test\"\\\nlabel", PROMETHEUS_ELEMENT_MAX), 15);

    assert_string_equal(destination_name, "test\\\"\\\\\\\nlabel");
}

static void test_prometheus_units_copy(void **state)
{
    (void)*state;

    char destination_name[PROMETHEUS_ELEMENT_MAX + 1];
    assert_string_equal(prometheus_units_copy(destination_name, "test-units", PROMETHEUS_ELEMENT_MAX, 0), "_test_units");
    assert_string_equal(destination_name, "_test_units");

    assert_string_equal(prometheus_units_copy(destination_name, "%", PROMETHEUS_ELEMENT_MAX, 0), "_percent");
    assert_string_equal(prometheus_units_copy(destination_name, "test-units/s", PROMETHEUS_ELEMENT_MAX, 0), "_test_units_persec");

    assert_string_equal(prometheus_units_copy(destination_name, "KiB", PROMETHEUS_ELEMENT_MAX, 1), "_KB");
}

static void test_format_host_labels_prometheus(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;

    instance->config.options |= EXPORTING_OPTION_SEND_CONFIGURED_LABELS;
    instance->config.options |= EXPORTING_OPTION_SEND_AUTOMATIC_LABELS;

    format_host_labels_prometheus(instance, localhost);
    assert_string_equal(buffer_tostring(instance->labels_buffer), "key1=\"value1\",key2=\"value2\"");
}

static void rrd_stats_api_v1_charts_allmetrics_prometheus(void **state)
{
    (void)state;

    BUFFER *buffer = buffer_create(0, NULL);

    RRDSET *st;
    rrdset_foreach_read(st, localhost);
        break;
    rrdset_foreach_done(st);

    localhost->hostname = string_strdupz("test_hostname");
    st->family = string_strdupz("test_family");
    st->context = string_strdupz("test_context");

    expect_function_call(__wrap_now_realtime_sec);
    will_return(__wrap_now_realtime_sec, 2);

    expect_function_call(__wrap_exporting_calculate_value_from_stored_data);
    will_return(__wrap_exporting_calculate_value_from_stored_data, pack_storage_number(27, SN_DEFAULT_FLAGS));

    rrd_stats_api_v1_charts_allmetrics_prometheus_single_host(localhost, NULL, buffer, "test_server", "test_prefix", 0, 0);

    assert_string_equal(
        buffer_tostring(buffer),
        "netdata_info{instance=\"test_hostname\",application=\"\",version=\"\",key1=\"value1\",key2=\"value2\"} 1\n"
        "test_prefix_test_context{chart=\"chart_id\",family=\"test_family\",dimension=\"dimension_id\"} 690565856.0000000\n");

    buffer_flush(buffer);

    expect_function_call(__wrap_now_realtime_sec);
    will_return(__wrap_now_realtime_sec, 2);

    expect_function_call(__wrap_exporting_calculate_value_from_stored_data);
    will_return(__wrap_exporting_calculate_value_from_stored_data, pack_storage_number(27, SN_DEFAULT_FLAGS));

    rrd_stats_api_v1_charts_allmetrics_prometheus_single_host(
        localhost, NULL, buffer, "test_server", "test_prefix", 0, PROMETHEUS_OUTPUT_NAMES | PROMETHEUS_OUTPUT_TYPES);

    assert_string_equal(
        buffer_tostring(buffer),
        "netdata_info{instance=\"test_hostname\",application=\"\",version=\"\",key1=\"value1\",key2=\"value2\"} 1\n"
        "# TYPE test_prefix_test_context gauge\n"
        "test_prefix_test_context{chart=\"chart_name\",family=\"test_family\",dimension=\"dimension_name\"} 690565856.0000000\n");

    buffer_flush(buffer);

    expect_function_call(__wrap_now_realtime_sec);
    will_return(__wrap_now_realtime_sec, 2);

    expect_function_call(__wrap_exporting_calculate_value_from_stored_data);
    will_return(__wrap_exporting_calculate_value_from_stored_data, pack_storage_number(27, SN_DEFAULT_FLAGS));

    rrd_stats_api_v1_charts_allmetrics_prometheus_all_hosts(localhost, NULL, buffer, "test_server", "test_prefix", 0, 0);

    assert_string_equal(
        buffer_tostring(buffer),
        "netdata_info{instance=\"test_hostname\",application=\"\",version=\"\",key1=\"value1\",key2=\"value2\"} 1\n"
        "test_prefix_test_context{chart=\"chart_id\",family=\"test_family\",dimension=\"dimension_id\",instance=\"test_hostname\"} 690565856.0000000\n");

    free(st->context);
    free(st->family);
    free(localhost->hostname);
    buffer_free(buffer);
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
    assert_ptr_equal(instance->prepare_header, prometheus_remote_write_prepare_header);
    assert_ptr_equal(instance->check_response, process_prometheus_remote_write_response);

    assert_ptr_not_equal(instance->buffer, NULL);
    buffer_free(instance->buffer);

    struct prometheus_remote_write_specific_data *connector_specific_data =
        (struct prometheus_remote_write_specific_data *)instance->connector_specific_data;

    assert_ptr_not_equal(instance->connector_specific_data, NULL);
    assert_ptr_not_equal(connector_specific_data->write_request, NULL);
    freez(instance->connector_specific_data);
}

static void test_prometheus_remote_write_prepare_header(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;

    struct prometheus_remote_write_specific_config *connector_specific_config =
        callocz(1, sizeof(struct prometheus_remote_write_specific_config));
    instance->config.connector_specific_config = connector_specific_config;
    connector_specific_config->remote_write_path = strdupz("/receive");

    struct simple_connector_data *simple_connector_data = callocz(1, sizeof(struct simple_connector_data));
    instance->connector_specific_data = simple_connector_data;
    simple_connector_data->last_buffer = callocz(1, sizeof(struct simple_connector_buffer));
    simple_connector_data->last_buffer->header = buffer_create(0, NULL);
    simple_connector_data->last_buffer->buffer = buffer_create(0, NULL);
    strcpy(simple_connector_data->connected_to, "localhost");

    buffer_sprintf(simple_connector_data->last_buffer->buffer, "test buffer");

    prometheus_remote_write_prepare_header(instance);

    assert_string_equal(
        buffer_tostring(simple_connector_data->last_buffer->header),
        "POST /receive HTTP/1.1\r\n"
        "Host: localhost\r\n"
        "Accept: */*\r\n"
        "Content-Encoding: snappy\r\n"
        "Content-Type: application/x-protobuf\r\n"
        "X-Prometheus-Remote-Write-Version: 0.1.0\r\n"
        "Content-Length: 11\r\n"
        "\r\n");

    free(connector_specific_config->remote_write_path);

    buffer_free(simple_connector_data->last_buffer->header);
    buffer_free(simple_connector_data->last_buffer->buffer);
}

static void test_process_prometheus_remote_write_response(void **state)
{
    (void)state;
    BUFFER *buffer = buffer_create(0, NULL);

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

    struct simple_connector_data *simple_connector_data = mallocz(sizeof(struct simple_connector_data *));
    instance->connector_specific_data = simple_connector_data;
    struct prometheus_remote_write_specific_data *connector_specific_data =
        mallocz(sizeof(struct prometheus_remote_write_specific_data *));
    simple_connector_data->connector_specific_data = (void *)connector_specific_data;
    connector_specific_data->write_request = (void *)0xff;

    localhost->program_name = string_strdupz("test_program");
    localhost->program_version = string_strdupz("test_version");

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
    freez(simple_connector_data);
    free(localhost->program_name);
    free(localhost->program_version);
}

static void test_format_dimension_prometheus_remote_write(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;

    struct simple_connector_data *simple_connector_data = mallocz(sizeof(struct simple_connector_data *));
    instance->connector_specific_data = simple_connector_data;
    struct prometheus_remote_write_specific_data *connector_specific_data =
        mallocz(sizeof(struct prometheus_remote_write_specific_data *));
    simple_connector_data->connector_specific_data = (void *)connector_specific_data;
    connector_specific_data->write_request = (void *)0xff;

    RRDSET *st;
    rrdset_foreach_read(st, localhost);
        break;
    rrdset_foreach_done(st);
    
    RRDDIM *rd;
    rrddim_foreach_read(rd, st);
        break;
    rrddim_foreach_done(rd);

    expect_function_call(__wrap_exporting_calculate_value_from_stored_data);
    will_return(__wrap_exporting_calculate_value_from_stored_data, pack_storage_number(27, SN_DEFAULT_FLAGS));

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

    struct simple_connector_data *simple_connector_data = mallocz(sizeof(struct simple_connector_data *));
    instance->connector_specific_data = simple_connector_data;
    struct prometheus_remote_write_specific_data *connector_specific_data =
        mallocz(sizeof(struct prometheus_remote_write_specific_data *));
    simple_connector_data->connector_specific_data = (void *)connector_specific_data;
    connector_specific_data->write_request = __real_init_write_request();

    expect_function_call(__wrap_simple_connector_end_batch);
    expect_value(__wrap_simple_connector_end_batch, instance, instance);
    will_return(__wrap_simple_connector_end_batch, 0);
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
    char *write_request_string = calloc(1, 1000);
    convert_write_request_to_string(buffer_tostring(buffer), buffer_strlen(buffer), write_request_string, 999);
    assert_int_equal(strlen(write_request_string), 753);
    assert_string_equal(
        write_request_string,
        "timeseries {\n"
        "  labels {\n"
        "    name: \"__name__\"\n"
        "    value: \"test_name\"\n"
        "  }\n"
        "  labels {\n"
        "    name: \"instance\"\n"
        "    value: \"test_instance\"\n"
        "  }\n"
        "  labels {\n"
        "    name: \"application\"\n"
        "    value: \"test_application\"\n"
        "  }\n"
        "  labels {\n"
        "    name: \"version\"\n"
        "    value: \"test_version\"\n"
        "  }\n"
        "  labels {\n"
        "    name: \"test_key\"\n"
        "    value: \"test_value\"\n"
        "  }\n"
        "  samples {\n"
        "    value: 1\n"
        "    timestamp: 15051\n"
        "  }\n"
        "}\n"
        "timeseries {\n"
        "  labels {\n"
        "    name: \"__name__\"\n"
        "    value: \"test_name\"\n"
        "  }\n"
        "  labels {\n"
        "    name: \"chart\"\n"
        "    value: \"test chart\"\n"
        "  }\n"
        "  labels {\n"
        "    name: \"family\"\n"
        "    value: \"test_family\"\n"
        "  }\n"
        "  labels {\n"
        "    name: \"dimension\"\n"
        "    value: \"test_dimension\"\n"
        "  }\n"
        "  labels {\n"
        "    name: \"instance\"\n"
        "    value: \"test_instance\"\n"
        "  }\n"
        "  samples {\n"
        "    value: 123000321\n"
        "    timestamp: 15052\n"
        "  }\n"
        "}\n");
    free(write_request_string);

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
    struct stats *stats = &instance->stats;
    BUFFER *buffer = instance->buffer;

    __real_mark_scheduled_instances(engine);

    expect_function_call(__wrap_rrdhost_is_exportable);
    expect_value(__wrap_rrdhost_is_exportable, instance, instance);
    expect_value(__wrap_rrdhost_is_exportable, host, localhost);
    will_return(__wrap_rrdhost_is_exportable, 1);

    RRDSET *st;
    rrdset_foreach_read(st, localhost);
        break;
    rrdset_foreach_done(st);

    expect_function_call(__wrap_rrdset_is_exportable);
    expect_value(__wrap_rrdset_is_exportable, instance, instance);
    expect_value(__wrap_rrdset_is_exportable, st, st);
    will_return(__wrap_rrdset_is_exportable, 1);

    expect_function_call(__wrap_simple_connector_end_batch);
    expect_value(__wrap_simple_connector_end_batch, instance, instance);
    will_return(__wrap_simple_connector_end_batch, 0);
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

    expect_function_call(__wrap_send_internal_metrics);
    expect_value(__wrap_send_internal_metrics, instance, instance);
    will_return(__wrap_send_internal_metrics, 0);

    aws_kinesis_connector_worker(instance);

    assert_int_equal(stats->buffered_metrics, 0);
    assert_int_equal(stats->buffered_bytes, 84);
    assert_int_equal(stats->received_bytes, 0);
    assert_int_equal(stats->sent_bytes, 84);
    assert_int_equal(stats->sent_metrics, 1);
    assert_int_equal(stats->lost_metrics, 0);
    assert_int_equal(stats->receptions, 1);
    assert_int_equal(stats->transmission_successes, 1);
    assert_int_equal(stats->transmission_failures, 0);
    assert_int_equal(stats->data_lost_events, 0);
    assert_int_equal(stats->lost_bytes, 0);
    assert_int_equal(stats->reconnects, 0);

    free(connector_specific_config->stream_name);
    free(connector_specific_config->auth_key_id);
    free(connector_specific_config->secure_key);
}
#endif // HAVE_KINESIS

#if ENABLE_EXPORTING_PUBSUB
static void test_init_pubsub_instance(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;

    instance->config.options = EXPORTING_SOURCE_DATA_AS_COLLECTED | EXPORTING_OPTION_SEND_NAMES;

    struct pubsub_specific_config *connector_specific_config =
        callocz(1, sizeof(struct pubsub_specific_config));
    instance->config.connector_specific_config = connector_specific_config;
    connector_specific_config->credentials_file = strdupz("/test/credentials/file");
    connector_specific_config->project_id = strdupz("test_project_id");
    connector_specific_config->topic_id = strdupz("test_topic_id");

    expect_function_call(__wrap_pubsub_init);
    expect_not_value(__wrap_pubsub_init, pubsub_specific_data_p, NULL);
    expect_string(__wrap_pubsub_init, destination, "localhost");
    expect_string(__wrap_pubsub_init, error_message, "");
    expect_string(__wrap_pubsub_init, credentials_file, "/test/credentials/file");
    expect_string(__wrap_pubsub_init, project_id, "test_project_id");
    expect_string(__wrap_pubsub_init, topic_id, "test_topic_id");
    will_return(__wrap_pubsub_init, 0);

    assert_int_equal(init_pubsub_instance(instance), 0);

    assert_ptr_equal(instance->worker, pubsub_connector_worker);
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

    expect_function_call(__wrap_pubsub_init);
    expect_not_value(__wrap_pubsub_init, pubsub_specific_data_p, NULL);
    expect_string(__wrap_pubsub_init, destination, "localhost");
    expect_string(__wrap_pubsub_init, error_message, "");
    expect_string(__wrap_pubsub_init, credentials_file, "/test/credentials/file");
    expect_string(__wrap_pubsub_init, project_id, "test_project_id");
    expect_string(__wrap_pubsub_init, topic_id, "test_topic_id");
    will_return(__wrap_pubsub_init, 0);

    assert_int_equal(init_pubsub_instance(instance), 0);
    assert_ptr_equal(instance->metric_formatting, format_dimension_stored_json_plaintext);

    free(connector_specific_config->credentials_file);
    free(connector_specific_config->project_id);
    free(connector_specific_config->topic_id);
}

static void test_pubsub_connector_worker(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;
    struct stats *stats = &instance->stats;

    __real_mark_scheduled_instances(engine);

    expect_function_call(__wrap_rrdhost_is_exportable);
    expect_value(__wrap_rrdhost_is_exportable, instance, instance);
    expect_value(__wrap_rrdhost_is_exportable, host, localhost);
    will_return(__wrap_rrdhost_is_exportable, 1);

    RRDSET *st;
    rrdset_foreach_read(st, localhost);
        break;
    rrdset_foreach_done(st);

    expect_function_call(__wrap_rrdset_is_exportable);
    expect_value(__wrap_rrdset_is_exportable, instance, instance);
    expect_value(__wrap_rrdset_is_exportable, st, st);
    will_return(__wrap_rrdset_is_exportable, 1);

    expect_function_call(__wrap_simple_connector_end_batch);
    expect_value(__wrap_simple_connector_end_batch, instance, instance);
    will_return(__wrap_simple_connector_end_batch, 0);
    __real_prepare_buffers(engine);

    struct pubsub_specific_config *connector_specific_config =
        callocz(1, sizeof(struct pubsub_specific_config));
    instance->config.connector_specific_config = connector_specific_config;
    connector_specific_config->credentials_file = strdupz("/test/credentials/file");
    connector_specific_config->project_id = strdupz("test_project_id");
    connector_specific_config->topic_id = strdupz("test_topic_id");

    struct pubsub_specific_data *connector_specific_data = callocz(1, sizeof(struct pubsub_specific_data));
    instance->connector_specific_data = (void *)connector_specific_data;

    expect_function_call(__wrap_pubsub_add_message);
    expect_not_value(__wrap_pubsub_add_message, pubsub_specific_data_p, NULL);
    // The buffer is prepared by Graphite exporting connector
    expect_string(
        __wrap_pubsub_add_message, data,
        "netdata.test-host.chart_name.dimension_name;TAG1=VALUE1 TAG2=VALUE2 123000321 15051\n");
    will_return(__wrap_pubsub_add_message, 0);

    expect_function_call(__wrap_pubsub_publish);
    expect_not_value(__wrap_pubsub_publish, pubsub_specific_data_p, NULL);
    expect_string(__wrap_pubsub_publish, error_message, "");
    expect_value(__wrap_pubsub_publish, buffered_metrics, 1);
    expect_value(__wrap_pubsub_publish, buffered_bytes, 84);
    will_return(__wrap_pubsub_publish, 0);

    expect_function_call(__wrap_pubsub_get_result);
    expect_not_value(__wrap_pubsub_get_result, pubsub_specific_data_p, NULL);
    expect_not_value(__wrap_pubsub_get_result, error_message, NULL);
    expect_not_value(__wrap_pubsub_get_result, sent_metrics, NULL);
    expect_not_value(__wrap_pubsub_get_result, sent_bytes, NULL);
    expect_not_value(__wrap_pubsub_get_result, lost_metrics, NULL);
    expect_not_value(__wrap_pubsub_get_result, lost_bytes, NULL);
    will_return(__wrap_pubsub_get_result, 0);

    expect_function_call(__wrap_send_internal_metrics);
    expect_value(__wrap_send_internal_metrics, instance, instance);
    will_return(__wrap_send_internal_metrics, 0);

    pubsub_connector_worker(instance);

    assert_int_equal(stats->buffered_metrics, 0);
    assert_int_equal(stats->buffered_bytes, 84);
    assert_int_equal(stats->received_bytes, 0);
    assert_int_equal(stats->sent_bytes, 84);
    assert_int_equal(stats->sent_metrics, 0);
    assert_int_equal(stats->lost_metrics, 0);
    assert_int_equal(stats->receptions, 1);
    assert_int_equal(stats->transmission_successes, 1);
    assert_int_equal(stats->transmission_failures, 0);
    assert_int_equal(stats->data_lost_events, 0);
    assert_int_equal(stats->lost_bytes, 0);
    assert_int_equal(stats->reconnects, 0);

    free(connector_specific_config->credentials_file);
    free(connector_specific_config->project_id);
    free(connector_specific_config->topic_id);
}
#endif // ENABLE_EXPORTING_PUBSUB

#if HAVE_MONGOC
static void test_init_mongodb_instance(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;

    instance->config.options = EXPORTING_SOURCE_DATA_AS_COLLECTED | EXPORTING_OPTION_SEND_NAMES;

    struct mongodb_specific_config *connector_specific_config = callocz(1, sizeof(struct mongodb_specific_config));
    instance->config.connector_specific_config = connector_specific_config;
    connector_specific_config->database = strdupz("test_database");
    connector_specific_config->collection = strdupz("test_collection");
    instance->config.buffer_on_failures = 10;

    expect_function_call(__wrap_mongoc_init);
    expect_function_call(__wrap_mongoc_uri_new_with_error);
    expect_string(__wrap_mongoc_uri_new_with_error, uri_string, "localhost");
    expect_not_value(__wrap_mongoc_uri_new_with_error, error, NULL);
    will_return(__wrap_mongoc_uri_new_with_error, 0xf1);

    expect_function_call(__wrap_mongoc_uri_get_option_as_int32);
    expect_value(__wrap_mongoc_uri_get_option_as_int32, uri, 0xf1);
    expect_string(__wrap_mongoc_uri_get_option_as_int32, option, MONGOC_URI_SOCKETTIMEOUTMS);
    expect_value(__wrap_mongoc_uri_get_option_as_int32, fallback, 1000);
    will_return(__wrap_mongoc_uri_get_option_as_int32, 1000);

    expect_function_call(__wrap_mongoc_uri_set_option_as_int32);
    expect_value(__wrap_mongoc_uri_set_option_as_int32, uri, 0xf1);
    expect_string(__wrap_mongoc_uri_set_option_as_int32, option, MONGOC_URI_SOCKETTIMEOUTMS);
    expect_value(__wrap_mongoc_uri_set_option_as_int32, value, 1000);
    will_return(__wrap_mongoc_uri_set_option_as_int32, true);

    expect_function_call(__wrap_mongoc_client_new_from_uri);
    expect_value(__wrap_mongoc_client_new_from_uri, uri, 0xf1);
    will_return(__wrap_mongoc_client_new_from_uri, 0xf2);

    expect_function_call(__wrap_mongoc_client_set_appname);
    expect_value(__wrap_mongoc_client_set_appname, client, 0xf2);
    expect_string(__wrap_mongoc_client_set_appname, appname, "netdata");
    will_return(__wrap_mongoc_client_set_appname, true);

    expect_function_call(__wrap_mongoc_client_get_collection);
    expect_value(__wrap_mongoc_client_get_collection, client, 0xf2);
    expect_string(__wrap_mongoc_client_get_collection, db, "test_database");
    expect_string(__wrap_mongoc_client_get_collection, collection, "test_collection");
    will_return(__wrap_mongoc_client_get_collection, 0xf3);

    expect_function_call(__wrap_mongoc_uri_destroy);
    expect_value(__wrap_mongoc_uri_destroy, uri, 0xf1);

    assert_int_equal(init_mongodb_instance(instance), 0);

    assert_ptr_equal(instance->worker, mongodb_connector_worker);
    assert_ptr_equal(instance->start_batch_formatting, NULL);
    assert_ptr_equal(instance->start_host_formatting, format_host_labels_json_plaintext);
    assert_ptr_equal(instance->start_chart_formatting, NULL);
    assert_ptr_equal(instance->metric_formatting, format_dimension_collected_json_plaintext);
    assert_ptr_equal(instance->end_chart_formatting, NULL);
    assert_ptr_equal(instance->end_host_formatting, flush_host_labels);
    assert_ptr_equal(instance->end_batch_formatting, format_batch_mongodb);
    assert_ptr_equal(instance->prepare_header, NULL);
    assert_ptr_equal(instance->check_response, NULL);

    assert_ptr_not_equal(instance->buffer, NULL);
    buffer_free(instance->buffer);

    assert_ptr_not_equal(instance->connector_specific_data, NULL);

    struct mongodb_specific_data *connector_specific_data =
        (struct mongodb_specific_data *)instance->connector_specific_data;
    size_t number_of_buffers = 1;
    struct bson_buffer *current_buffer = connector_specific_data->first_buffer;
    while (current_buffer->next != connector_specific_data->first_buffer) {
        current_buffer = current_buffer->next;
        number_of_buffers++;
        if (number_of_buffers == (size_t)(instance->config.buffer_on_failures + 1)) {
            number_of_buffers = 0;
            break;
        }
    }
    assert_int_equal(number_of_buffers, 9);

    free(connector_specific_config->database);
    free(connector_specific_config->collection);
}

static void test_format_batch_mongodb(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;
    struct stats *stats = &instance->stats;

    struct mongodb_specific_data *connector_specific_data = mallocz(sizeof(struct mongodb_specific_data));
    instance->connector_specific_data = (void *)connector_specific_data;

    struct bson_buffer *current_buffer = callocz(1, sizeof(struct bson_buffer));
    connector_specific_data->first_buffer = current_buffer;
    connector_specific_data->first_buffer->next = current_buffer;
    connector_specific_data->last_buffer = current_buffer;

    BUFFER *buffer = buffer_create(0, NULL);
    buffer_sprintf(buffer, "{ \"metric\": \"test_metric\" }\n");
    instance->buffer = buffer;
    stats->buffered_metrics = 1;

    assert_int_equal(format_batch_mongodb(instance), 0);

    assert_int_equal(connector_specific_data->last_buffer->documents_inserted, 1);
    assert_int_equal(buffer_strlen(buffer), 0);

    size_t len;
    char *str = bson_as_canonical_extended_json(connector_specific_data->last_buffer->insert[0], &len);
    assert_string_equal(str, "{ \"metric\" : \"test_metric\" }");

    freez(str);
    buffer_free(buffer);
}

static void test_mongodb_connector_worker(void **state)
{
    struct engine *engine = *state;
    struct instance *instance = engine->instance_root;

    struct mongodb_specific_config *connector_specific_config = callocz(1, sizeof(struct mongodb_specific_config));
    instance->config.connector_specific_config = connector_specific_config;
    connector_specific_config->database = strdupz("test_database");

    struct mongodb_specific_data *connector_specific_data = callocz(1, sizeof(struct mongodb_specific_data));
    instance->connector_specific_data = (void *)connector_specific_data;
    connector_specific_config->collection = strdupz("test_collection");

    struct bson_buffer *buffer = callocz(1, sizeof(struct bson_buffer));
    buffer->documents_inserted = 1;
    connector_specific_data->first_buffer = buffer;
    connector_specific_data->first_buffer->next = buffer;

    connector_specific_data->first_buffer->insert = callocz(1, sizeof(bson_t *));
    bson_error_t bson_error;
    connector_specific_data->first_buffer->insert[0] =
        bson_new_from_json((const uint8_t *)"{ \"test_key\" : \"test_value\" }", -1, &bson_error);

    connector_specific_data->client = mongoc_client_new("mongodb://localhost");
    connector_specific_data->collection =
        __real_mongoc_client_get_collection(connector_specific_data->client, "test_database", "test_collection");

    expect_function_call(__wrap_mongoc_collection_insert_many);
    expect_value(__wrap_mongoc_collection_insert_many, collection, connector_specific_data->collection);
    expect_value(__wrap_mongoc_collection_insert_many, documents, connector_specific_data->first_buffer->insert);
    expect_value(__wrap_mongoc_collection_insert_many, n_documents, 1);
    expect_value(__wrap_mongoc_collection_insert_many, opts, NULL);
    expect_value(__wrap_mongoc_collection_insert_many, reply, NULL);
    expect_not_value(__wrap_mongoc_collection_insert_many, error, NULL);
    will_return(__wrap_mongoc_collection_insert_many, true);

    expect_function_call(__wrap_send_internal_metrics);
    expect_value(__wrap_send_internal_metrics, instance, instance);
    will_return(__wrap_send_internal_metrics, 0);

    mongodb_connector_worker(instance);

    assert_ptr_equal(connector_specific_data->first_buffer->insert, NULL);
    assert_int_equal(connector_specific_data->first_buffer->documents_inserted, 0);
    assert_ptr_equal(connector_specific_data->first_buffer, connector_specific_data->first_buffer->next);

    struct stats *stats = &instance->stats;
    assert_int_equal(stats->buffered_metrics, 0);
    assert_int_equal(stats->buffered_bytes, 0);
    assert_int_equal(stats->received_bytes, 0);
    assert_int_equal(stats->sent_bytes, 30);
    assert_int_equal(stats->sent_metrics, 1);
    assert_int_equal(stats->lost_metrics, 0);
    assert_int_equal(stats->receptions, 1);
    assert_int_equal(stats->transmission_successes, 1);
    assert_int_equal(stats->transmission_failures, 0);
    assert_int_equal(stats->data_lost_events, 0);
    assert_int_equal(stats->lost_bytes, 0);
    assert_int_equal(stats->reconnects, 0);

    free(connector_specific_config->database);
    free(connector_specific_config->collection);
}
#endif // HAVE_MONGOC

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

    const struct CMUnitTest internal_metrics_tests[] = {
        cmocka_unit_test_setup_teardown(test_create_main_rusage_chart, setup_rrdhost, teardown_rrdhost),
        cmocka_unit_test(test_send_main_rusage),
        cmocka_unit_test(test_send_internal_metrics),
    };

    test_res += cmocka_run_group_tests_name("internal_metrics", internal_metrics_tests, NULL, NULL);

    const struct CMUnitTest prometheus_web_api_tests[] = {
        cmocka_unit_test_setup_teardown(test_can_send_rrdset, setup_prometheus, teardown_prometheus),
        cmocka_unit_test_setup_teardown(test_prometheus_name_copy, setup_prometheus, teardown_prometheus),
        cmocka_unit_test_setup_teardown(test_prometheus_label_copy, setup_prometheus, teardown_prometheus),
        cmocka_unit_test_setup_teardown(test_prometheus_units_copy, setup_prometheus, teardown_prometheus),
        cmocka_unit_test_setup_teardown(
            test_format_host_labels_prometheus, setup_initialized_engine, teardown_initialized_engine),
        cmocka_unit_test_setup_teardown(
            rrd_stats_api_v1_charts_allmetrics_prometheus, setup_prometheus, teardown_prometheus),
    };

    test_res += cmocka_run_group_tests_name("prometheus_web_api", prometheus_web_api_tests, NULL, NULL);

#if ENABLE_PROMETHEUS_REMOTE_WRITE
    const struct CMUnitTest prometheus_remote_write_tests[] = {
        cmocka_unit_test_setup_teardown(
            test_init_prometheus_remote_write_instance, setup_configured_engine, teardown_configured_engine),
        cmocka_unit_test_setup_teardown(
            test_prometheus_remote_write_prepare_header, setup_initialized_engine, teardown_initialized_engine),
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

#if ENABLE_EXPORTING_PUBSUB
    const struct CMUnitTest pubsub_tests[] = {
        cmocka_unit_test_setup_teardown(
            test_init_pubsub_instance, setup_configured_engine, teardown_configured_engine),
        cmocka_unit_test_setup_teardown(
            test_pubsub_connector_worker, setup_initialized_engine, teardown_initialized_engine),
    };

    test_res += cmocka_run_group_tests_name("pubsub_exporting_connector", pubsub_tests, NULL, NULL);
#endif

#if HAVE_MONGOC
    const struct CMUnitTest mongodb_tests[] = {
        cmocka_unit_test_setup_teardown(
            test_init_mongodb_instance, setup_configured_engine, teardown_configured_engine),
        cmocka_unit_test_setup_teardown(
            test_format_batch_mongodb, setup_configured_engine, teardown_configured_engine),
        cmocka_unit_test_setup_teardown(
            test_mongodb_connector_worker, setup_configured_engine, teardown_configured_engine),
    };

    test_res += cmocka_run_group_tests_name("mongodb_exporting_connector", mongodb_tests, NULL, NULL);
#endif

    return test_res;
}

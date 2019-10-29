// SPDX-License-Identifier: GPL-3.0-or-later

#include "../../libnetdata/libnetdata.h"
#include "../../libnetdata/required_dummies.h"

#include "../exporting_engine.h"
#include "../graphite/graphite.h"

#include <setjmp.h>
#include <cmocka.h>

time_t __wrap_now_realtime_sec(void)
{
    function_called();
    return mock_type(time_t);
}

struct engine *__real_read_exporting_config();
struct engine *__wrap_read_exporting_config()
{
    function_called();
    return mock_ptr_type(struct engine *);
}

int __real_init_connectors(struct engine *engine);
int __wrap_init_connectors(struct engine *engine)
{
    function_called();
    check_expected_ptr(engine);
    return mock_type(int);
}

int __wrap_mark_scheduled_instances(struct engine *engine)
{
    function_called();
    check_expected_ptr(engine);
    return mock_type(int);
}

int __real_prepare_buffers(struct engine *engine);
int __wrap_prepare_buffers(struct engine *engine)
{
    function_called();
    check_expected_ptr(engine);
    return mock_type(int);
}

int __wrap_notify_workers(struct engine *engine)
{
    function_called();
    check_expected_ptr(engine);
    return mock_type(int);
}

int __wrap_send_internal_metrics(struct engine *engine)
{
    function_called();
    check_expected_ptr(engine);
    return mock_type(int);
}

int __mock_start_batch_formatting(struct instance *instance)
{
    function_called();
    check_expected_ptr(instance);
    return mock_type(int);
}

int __mock_start_host_formatting(struct instance *instance)
{
    function_called();
    check_expected_ptr(instance);
    return mock_type(int);
}

int __mock_start_chart_formatting(struct instance *instance)
{
    function_called();
    check_expected_ptr(instance);
    return mock_type(int);
}

int __mock_metric_formatting(struct instance *instance, RRDDIM *rd)
{
    function_called();
    check_expected_ptr(instance);
    check_expected_ptr(rd);
    return mock_type(int);
}

int __mock_end_chart_formatting(struct instance *instance)
{
    function_called();
    check_expected_ptr(instance);
    return mock_type(int);
}

int __mock_end_host_formatting(struct instance *instance)
{
    function_called();
    check_expected_ptr(instance);
    return mock_type(int);
}

int __mock_end_batch_formatting(struct instance *instance)
{
    function_called();
    check_expected_ptr(instance);
    return mock_type(int);
}

RRDHOST *localhost;
netdata_rwlock_t rrd_rwlock;

void __rrdhost_check_rdlock(RRDHOST *host, const char *file, const char *function, const unsigned long line)
{
    (void)host;
    (void)file;
    (void)function;
    (void)line;
}

void __rrdset_check_rdlock(RRDSET *st, const char *file, const char *function, const unsigned long line)
{
    (void)st;
    (void)file;
    (void)function;
    (void)line;
}

void __rrd_check_rdlock(const char *file, const char *function, const unsigned long line)
{
    (void)file;
    (void)function;
    (void)line;
}

static void test_exporting_engine(void **state)
{
    (void)state;

    struct engine *engine = (struct engine *)malloc(sizeof(struct engine));
    memset(engine, 0xDB, sizeof(struct engine));
    engine->after = 1;
    engine->before = 2;

    expect_function_call(__wrap_read_exporting_config);
    will_return(__wrap_read_exporting_config, engine);

    expect_function_call(__wrap_init_connectors);
    expect_memory(__wrap_init_connectors, engine, engine, sizeof(struct engine));
    will_return(__wrap_init_connectors, 0);

    expect_function_calls(__wrap_now_realtime_sec, 2);
    will_return(__wrap_now_realtime_sec, 1);
    will_return(__wrap_now_realtime_sec, 2);

    expect_function_call(__wrap_mark_scheduled_instances);
    expect_memory(__wrap_mark_scheduled_instances, engine, engine, sizeof(struct engine));
    will_return(__wrap_mark_scheduled_instances, 0);

    expect_function_call(__wrap_prepare_buffers);
    expect_memory(__wrap_prepare_buffers, engine, engine, sizeof(struct engine));
    will_return(__wrap_prepare_buffers, 0);

    expect_function_call(__wrap_notify_workers);
    expect_memory(__wrap_notify_workers, engine, engine, sizeof(struct engine));
    will_return(__wrap_notify_workers, 0);

    expect_function_call(__wrap_send_internal_metrics);
    expect_memory(__wrap_send_internal_metrics, engine, engine, sizeof(struct engine));
    will_return(__wrap_send_internal_metrics, 0);

    void *ptr = malloc(sizeof(int));
    assert_ptr_equal(exporting_main(ptr), NULL);

    free(engine);
    free(ptr);
}

static void test_prepare_buffers(void **state)
{
    (void)state;

    struct engine *engine = (struct engine *)malloc(sizeof(struct engine));
    memset(engine, 0xD1, sizeof(struct engine));
    engine->after = 1;
    engine->before = 2;

    engine->connector_root = (struct connector *)malloc(sizeof(struct connector));
    struct connector *connector = engine->connector_root;
    connector->next = NULL;
    connector->start_batch_formatting = __mock_start_batch_formatting;
    connector->start_host_formatting = __mock_start_host_formatting;
    connector->start_chart_formatting = __mock_start_chart_formatting;
    connector->metric_formatting = __mock_metric_formatting;
    connector->end_chart_formatting = __mock_end_chart_formatting;
    connector->end_host_formatting = __mock_end_host_formatting;
    connector->end_batch_formatting = __mock_end_batch_formatting;

    connector->instance_root = (struct instance *)malloc(sizeof(struct instance));

    localhost = (RRDHOST *)calloc(1, sizeof(RRDHOST));
    localhost->rrdset_root = (RRDSET *)calloc(1, sizeof(RRDSET));
    localhost->rrdset_root->dimensions = (RRDDIM *)calloc(1, sizeof(RRDDIM));
    RRDDIM *rd = localhost->rrdset_root->dimensions;
    memset(rd, 0xD2, sizeof(RRDDIM));
    rd->next = NULL;

    struct instance *instance = connector->instance_root;
    instance->next = NULL;

    expect_function_call(__mock_start_batch_formatting);
    expect_memory(__mock_start_batch_formatting, instance, instance, sizeof(struct instance));
    will_return(__mock_start_batch_formatting, 0);

    // ignore_function_calls(__wrap_now_realtime_sec);
    // will_return_always(__wrap_now_realtime_sec, 1);

    expect_function_call(__mock_start_host_formatting);
    expect_memory(__mock_start_host_formatting, instance, instance, sizeof(struct instance));
    will_return(__mock_start_host_formatting, 0);

    expect_function_call(__mock_start_chart_formatting);
    expect_memory(__mock_start_chart_formatting, instance, instance, sizeof(struct instance));
    will_return(__mock_start_chart_formatting, 0);

    expect_function_call(__mock_metric_formatting);
    expect_memory(__mock_metric_formatting, instance, instance, sizeof(struct instance));
    expect_memory(__mock_metric_formatting, rd, rd, sizeof(RRDDIM));
    will_return(__mock_metric_formatting, 0);

    expect_function_call(__mock_end_chart_formatting);
    expect_memory(__mock_end_chart_formatting, instance, instance, sizeof(struct instance));
    will_return(__mock_end_chart_formatting, 0);

    expect_function_call(__mock_end_host_formatting);
    expect_memory(__mock_end_host_formatting, instance, instance, sizeof(struct instance));
    will_return(__mock_end_host_formatting, 0);

    expect_function_call(__mock_end_batch_formatting);
    expect_memory(__mock_end_batch_formatting, instance, instance, sizeof(struct instance));
    will_return(__mock_end_batch_formatting, 0);

    assert_int_equal(__real_prepare_buffers(engine), 0);

    // check with NULL functions
    connector->start_batch_formatting = NULL;
    connector->start_host_formatting = NULL;
    connector->start_chart_formatting = NULL;
    connector->metric_formatting = NULL;
    connector->end_chart_formatting = NULL;
    connector->end_host_formatting = NULL;
    connector->end_batch_formatting = NULL;
    assert_int_equal(__real_prepare_buffers(engine), 0);

    free(localhost->rrdset_root->dimensions);
    free(localhost->rrdset_root);
    free(localhost);
    free(engine->connector_root->instance_root);
    free(engine->connector_root);
    free(engine);
}

static void test_read_exporting_config(void **state)
{
    (void)state;

    struct engine *engine = __real_read_exporting_config();

    assert_ptr_not_equal(engine, NULL);
    assert_string_equal(engine->config.prefix, "netdata");
    assert_string_equal(engine->config.hostname, "test-host");
    assert_int_equal(engine->config.update_every, 3);
    assert_int_equal(engine->config.options, BACKEND_SOURCE_DATA_AVERAGE | BACKEND_OPTION_SEND_NAMES);

    struct connector *connector = engine->connector_root;
    assert_ptr_not_equal(connector, NULL);
    assert_ptr_equal(connector->next, NULL);
    assert_ptr_equal(connector->engine, engine);
    assert_int_equal(connector->config.type, BACKEND_TYPE_GRAPHITE);

    struct instance *instance = connector->instance_root;
    assert_ptr_not_equal(instance, NULL);
    assert_ptr_equal(instance->next, NULL);
    assert_ptr_equal(instance->connector, connector);
    assert_string_equal(instance->config.destination, "localhost");
    assert_int_equal(instance->config.update_every, 1);
    assert_int_equal(instance->config.buffer_on_failures, 10);
    assert_int_equal(instance->config.timeoutms, 10000);
    assert_string_equal(instance->config.charts_pattern, "*");
    assert_string_equal(instance->config.hosts_pattern, "localhost *");
    assert_int_equal(instance->config.send_names_instead_of_ids, 1);
}

static void test_init_connectors(void **state)
{
    (void)state;

    struct engine *engine = __real_read_exporting_config();

    assert_int_equal(__real_init_connectors(engine), 0);

    struct connector *connector = engine->connector_root;
    assert_ptr_equal(connector->start_batch_formatting, NULL);
    assert_ptr_equal(connector->start_host_formatting, NULL);
    assert_ptr_equal(connector->start_chart_formatting, NULL);
    assert_ptr_equal(connector->metric_formatting, format_dimension_collected_graphite_plaintext);
    assert_ptr_equal(connector->end_chart_formatting, NULL);
    assert_ptr_equal(connector->end_host_formatting, NULL);
    assert_ptr_equal(connector->end_batch_formatting, NULL);

    BUFFER *buffer = (BUFFER *)connector->instance_root->buffer;
    assert_ptr_not_equal(buffer, NULL);
    buffer_sprintf(buffer, "%s", "graphite test");
    assert_string_equal(buffer_tostring(buffer), "graphite test");
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
    (void)state;

    struct engine *engine = __real_read_exporting_config();
    __real_init_connectors(engine);

    localhost = (RRDHOST *)calloc(1, sizeof(RRDHOST));
    localhost->tags = strdup("TAG1=VALUE1 TAG2=VALUE2");

    localhost->rrdset_root = (RRDSET *)calloc(1, sizeof(RRDSET));
    RRDSET *st = localhost->rrdset_root;
    st->rrdhost = localhost;
    strcpy(st->id, "chart_id");
    st->name = strdup("chart_name");

    localhost->rrdset_root->dimensions = (RRDDIM *)calloc(1, sizeof(RRDDIM));
    RRDDIM *rd = localhost->rrdset_root->dimensions;
    rd->rrdset = st;
    rd->id = strdup("dimension_id");
    rd->name = strdup("dimension_name");
    rd->last_collected_value = 123000321;
    rd->last_collected_time.tv_sec = 15051;

    assert_int_equal(format_dimension_collected_graphite_plaintext(engine->connector_root->instance_root, rd), 0);
    printf(buffer_tostring(engine->connector_root->instance_root->buffer));
    assert_string_equal(
        buffer_tostring(engine->connector_root->instance_root->buffer),
        "netdata.test-host.chart_name.dimension_name;TAG1=VALUE1 TAG2=VALUE2 123000321 15051\n");

    free(rd);
    free(st);
    free(localhost);
}

int main(void)
{
    const struct CMUnitTest tests[] = {
        cmocka_unit_test(test_exporting_engine),
        cmocka_unit_test(test_prepare_buffers),
        cmocka_unit_test(test_read_exporting_config),
        cmocka_unit_test(test_init_connectors),
        cmocka_unit_test(test_exporting_name_copy),
        cmocka_unit_test(test_format_dimension_collected_graphite_plaintext),
    };

    return cmocka_run_group_tests_name("exporting_engine", tests, NULL, NULL);
}

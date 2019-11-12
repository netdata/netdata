// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"

#include "exporting/exporting_engine.h"
#include "exporting/graphite/graphite.h"

#include <stdarg.h>
#include <stddef.h>
#include <setjmp.h>
#include <stdint.h>
#include <cmocka.h>

RRDHOST *localhost;
netdata_rwlock_t rrd_rwlock;

// Use memomy allocation functions guarded by CMocka in strdupz, mallocz, callocz, and reallocz
const char *__wrap_strdupz(const char *s)
{
    char *duplicate = malloc(sizeof(char) * (strlen(s) + 1));
    strcpy(duplicate, s);

    return duplicate;
}

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

static struct engine *__mock_read_exporting_config()
{
    struct engine *engine = calloc(1, sizeof(struct engine));
    engine->config.prefix = strdupz("netdata");
    engine->config.hostname = strdupz("test-host");
    engine->config.update_every = 3;
    engine->config.options = BACKEND_SOURCE_DATA_AVERAGE | BACKEND_OPTION_SEND_NAMES;

    engine->connector_root = calloc(1, sizeof(struct connector));
    engine->connector_root->config.type = BACKEND_TYPE_GRAPHITE;
    engine->connector_root->engine = engine;

    engine->connector_root->instance_root = calloc(1, sizeof(struct instance));
    struct instance *instance = engine->connector_root->instance_root;
    instance->connector = engine->connector_root;
    instance->config.destination = strdupz("localhost");
    instance->config.update_every = 1;
    instance->config.buffer_on_failures = 10;
    instance->config.timeoutms = 10000;
    instance->config.charts_pattern = strdupz("*");
    instance->config.hosts_pattern = strdupz("localhost *");
    instance->config.send_names_instead_of_ids = 1;

    return engine;
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

void __wrap_uv_thread_create(uv_thread_t thread, void (*worker)(void *arg), void *arg)
{
    function_called();

    check_expected_ptr(thread);
    check_expected_ptr(worker);
    check_expected_ptr(arg);
}

void __wrap_uv_mutex_lock(uv_mutex_t *mutex)
{
    (void)mutex;
}

void __wrap_uv_mutex_unlock(uv_mutex_t *mutex)
{
    (void)mutex;
}

void __wrap_uv_cond_signal(uv_cond_t *cond_var)
{
    (void)cond_var;
}

void __wrap_uv_cond_wait(uv_cond_t *cond_var, uv_mutex_t *mutex)
{
    (void)cond_var;
    (void)mutex;
}

static int setup_configured_engine(void **state)
{
    struct engine *engine = __mock_read_exporting_config();

    engine->after = 1;
    engine->before = 2;

    *state = engine;

    return 0;
}

static int teardown_configured_engine(void **state)
{
    struct engine *engine = *state;

    struct instance *instance = engine->connector_root->instance_root;
    free((void *)instance->config.destination);
    free(instance->config.charts_pattern);
    free(instance->config.hosts_pattern);
    free(instance);

    free(engine->connector_root);

    free((void *)engine->config.prefix);
    free((void *)engine->config.hostname);
    free(engine);

    return 0;
}

static int setup_rrdhost()
{
    return 0;
}

static int teardown_rrdhost()
{
    return 0;
}

static int setup_initialized_engine(void **state)
{
    (void)state;

    return 0;
}

static int teardown_initialized_engine(void **state)
{
    (void)state;

    return 0;
}

static void init_connectors_in_tests(struct engine *engine)
{
    expect_function_call(__wrap_uv_thread_create);

    expect_value(__wrap_uv_thread_create, thread, &engine->connector_root->instance_root->thread);
    expect_value(__wrap_uv_thread_create, worker, simple_connector_worker);
    expect_value(__wrap_uv_thread_create, arg, engine->connector_root->instance_root);

    assert_int_equal(__real_init_connectors(engine), 0);
}

static void test_exporting_engine(void **state)
{
    struct engine *engine = *state;

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
    free(ptr);
}

static void test_read_exporting_config(void **state)
{
    struct engine *engine = *state; // TODO: use real read_exporting_config() function

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
    struct engine *engine = *state;

    init_connectors_in_tests(engine);

    struct connector *connector = engine->connector_root;
    assert_ptr_equal(connector->start_batch_formatting, NULL);
    assert_ptr_equal(connector->start_host_formatting, NULL);
    assert_ptr_equal(connector->start_chart_formatting, NULL);
    assert_ptr_equal(connector->metric_formatting, format_dimension_collected_graphite_plaintext);
    assert_ptr_equal(connector->end_chart_formatting, NULL);
    assert_ptr_equal(connector->end_host_formatting, NULL);
    assert_ptr_equal(connector->end_batch_formatting, NULL);
    assert_ptr_equal(connector->worker, simple_connector_worker);

    struct simple_connector_config *connector_specific_config = connector->config.connector_specific_config;
    assert_int_equal(connector_specific_config->default_port, 2003);

    BUFFER *buffer = connector->instance_root->buffer;
    assert_ptr_not_equal(buffer, NULL);
    buffer_sprintf(buffer, "%s", "graphite test");
    assert_string_equal(buffer_tostring(buffer), "graphite test");
}

static void test_prepare_buffers(void **state)
{
    (void)state;

    struct engine *engine = malloc(sizeof(struct engine));
    memset(engine, 0xD1, sizeof(struct engine));
    engine->after = 1;
    engine->before = 2;

    engine->connector_root = malloc(sizeof(struct connector));
    struct connector *connector = engine->connector_root;
    connector->next = NULL;
    connector->start_batch_formatting = __mock_start_batch_formatting;
    connector->start_host_formatting = __mock_start_host_formatting;
    connector->start_chart_formatting = __mock_start_chart_formatting;
    connector->metric_formatting = __mock_metric_formatting;
    connector->end_chart_formatting = __mock_end_chart_formatting;
    connector->end_host_formatting = __mock_end_host_formatting;
    connector->end_batch_formatting = __mock_end_batch_formatting;

    connector->instance_root = malloc(sizeof(struct instance));

    localhost = calloc(1, sizeof(RRDHOST));
    localhost->rrdset_root = calloc(1, sizeof(RRDSET));
    localhost->rrdset_root->dimensions = calloc(1, sizeof(RRDDIM));
    RRDDIM *rd = localhost->rrdset_root->dimensions;
    memset(rd, 0xD2, sizeof(RRDDIM));
    rd->next = NULL;

    struct instance *instance = connector->instance_root;
    instance->next = NULL;

    expect_function_call(__mock_start_batch_formatting);
    expect_memory(__mock_start_batch_formatting, instance, instance, sizeof(struct instance));
    will_return(__mock_start_batch_formatting, 0);

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

    init_connectors_in_tests(engine);

    localhost = calloc(1, sizeof(RRDHOST));
    localhost->tags = strdupz("TAG1=VALUE1 TAG2=VALUE2");

    localhost->rrdset_root = calloc(1, sizeof(RRDSET));
    RRDSET *st = localhost->rrdset_root;
    st->rrdhost = localhost;
    strcpy(st->id, "chart_id");
    st->name = strdupz("chart_name");

    localhost->rrdset_root->dimensions = calloc(1, sizeof(RRDDIM));
    RRDDIM *rd = localhost->rrdset_root->dimensions;
    rd->rrdset = st;
    rd->id = strdupz("dimension_id");
    rd->name = strdupz("dimension_name");
    rd->last_collected_value = 123000321;
    rd->last_collected_time.tv_sec = 15051;

    assert_int_equal(format_dimension_collected_graphite_plaintext(engine->connector_root->instance_root, rd), 0);
    assert_string_equal(
        buffer_tostring(engine->connector_root->instance_root->buffer),
        "netdata.test-host.chart_name.dimension_name;TAG1=VALUE1 TAG2=VALUE2 123000321 15051\n");

    free((void *)rd->name);
    free((void *)rd->id);
    free(rd);

    free((void *)st->name);
    free(st);

    free((void *)localhost->tags);
    free(localhost);
}

static void test_init_graphite_instance(void **state)
{
    (void)state;

    // receive responce
    // reconnect if it's needed
    // send data
    // process failures
}

int main(void)
{
    const struct CMUnitTest tests[] = {
        cmocka_unit_test_setup_teardown(test_exporting_engine, setup_configured_engine, teardown_configured_engine),
        cmocka_unit_test_setup_teardown(test_read_exporting_config, setup_configured_engine, teardown_configured_engine),
        cmocka_unit_test_setup_teardown(test_init_connectors, setup_configured_engine, teardown_configured_engine),
        cmocka_unit_test(test_prepare_buffers),
        cmocka_unit_test(test_exporting_name_copy),
        cmocka_unit_test_setup_teardown(
            test_format_dimension_collected_graphite_plaintext, setup_configured_engine, teardown_configured_engine),
        cmocka_unit_test(test_init_graphite_instance),
    };

    return cmocka_run_group_tests_name("exporting_engine", tests, NULL, NULL);
}

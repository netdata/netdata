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
    engine->config.prefix = strdupz("netdata");
    engine->config.hostname = strdupz("test-host");
    engine->config.update_every = 3;

    engine->connector_root = calloc(1, sizeof(struct connector));
    engine->connector_root->config.type = BACKEND_TYPE_GRAPHITE;
    engine->connector_root->engine = engine;

    engine->connector_root->instance_root = calloc(1, sizeof(struct instance));
    struct instance *instance = engine->connector_root->instance_root;
    instance->connector = engine->connector_root;
    instance->config.name = strdupz("instance_name");
    instance->config.destination = strdupz("localhost");
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

calculated_number __real_exporting_calculate_value_from_stored_data(
    struct instance *instance,
    RRDDIM *rd,
    time_t *last_timestamp);
calculated_number __wrap_exporting_calculate_value_from_stored_data(
    struct instance *instance,
    RRDDIM *rd,
    time_t *last_timestamp)
{
    (void)instance;
    (void)rd;

    *last_timestamp = 15052;

    function_called();
    return mock_type(calculated_number);
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

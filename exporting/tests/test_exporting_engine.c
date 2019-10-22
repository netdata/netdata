// SPDX-License-Identifier: GPL-3.0-or-later

#include "../../libnetdata/libnetdata.h"
#include "../../libnetdata/required_dummies.h"

#include "../exporting_engine.h"

#include <setjmp.h>
#include <cmocka.h>

time_t __wrap_now_realtime_sec(void)
{
    function_called();
    return mock_type(time_t);
}

struct engine *__wrap_read_exporting_config()
{
    function_called();
    return mock_ptr_type(struct engine *);
}

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

int __mock_start_batch_formatting(struct engine *engine)
{
    function_called();
    check_expected_ptr(engine);
    return mock_type(int);
}

int __mock_start_host_formatting(struct engine *engine)
{
    function_called();
    check_expected_ptr(engine);
    return mock_type(int);
}

int __mock_start_chart_formatting(struct engine *engine)
{
    function_called();
    check_expected_ptr(engine);
    return mock_type(int);
}

int __mock_metric_formatting(struct engine *engine)
{
    function_called();
    check_expected_ptr(engine);
    return mock_type(int);
}

int __mock_end_chart_formatting(struct engine *engine)
{
    function_called();
    check_expected_ptr(engine);
    return mock_type(int);
}

int __mock_end_host_formatting(struct engine *engine)
{
    function_called();
    check_expected_ptr(engine);
    return mock_type(int);
}

int __mock_end_batch_formatting(struct engine *engine)
{
    function_called();
    check_expected_ptr(engine);
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
    memset(engine, 0xDB, sizeof(struct engine));
    engine->after = 1;
    engine->before = 2;

    engine->connector_root = (struct connector *)malloc(sizeof(struct connector));

    engine->connector_root->instance_root = (struct instance *)malloc(sizeof(struct instance));
    engine->connector_root->instance_root->start_batch_formatting = __mock_start_batch_formatting;
    engine->connector_root->instance_root->start_host_formatting = __mock_start_host_formatting;
    engine->connector_root->instance_root->start_chart_formatting = __mock_start_chart_formatting;
    engine->connector_root->instance_root->metric_formatting = __mock_metric_formatting;
    engine->connector_root->instance_root->end_chart_formatting = __mock_end_chart_formatting;
    engine->connector_root->instance_root->end_host_formatting = __mock_end_host_formatting;
    engine->connector_root->instance_root->end_batch_formatting = __mock_end_batch_formatting;

    localhost = (RRDHOST *)calloc(1, sizeof(RRDHOST));
    localhost->rrdset_root = (RRDSET *)calloc(1, sizeof(RRDSET));
    localhost->rrdset_root->dimensions = (RRDDIM *)calloc(1, sizeof(RRDDIM));

    expect_function_call(__mock_start_batch_formatting);
    expect_memory(__mock_start_batch_formatting, engine, engine, sizeof(struct engine));
    will_return(__mock_start_batch_formatting, 0);

    // ignore_function_calls(__wrap_now_realtime_sec);
    // will_return_always(__wrap_now_realtime_sec, 1);

    expect_function_call(__mock_start_host_formatting);
    expect_memory(__mock_start_host_formatting, engine, engine, sizeof(struct engine));
    will_return(__mock_start_host_formatting, 0);

    expect_function_call(__mock_start_chart_formatting);
    expect_memory(__mock_start_chart_formatting, engine, engine, sizeof(struct engine));
    will_return(__mock_start_chart_formatting, 0);

    expect_function_call(__mock_metric_formatting);
    expect_memory(__mock_metric_formatting, engine, engine, sizeof(struct engine));
    will_return(__mock_metric_formatting, 0);

    expect_function_call(__mock_end_chart_formatting);
    expect_memory(__mock_end_chart_formatting, engine, engine, sizeof(struct engine));
    will_return(__mock_end_chart_formatting, 0);

    expect_function_call(__mock_end_host_formatting);
    expect_memory(__mock_end_host_formatting, engine, engine, sizeof(struct engine));
    will_return(__mock_end_host_formatting, 0);

    expect_function_call(__mock_end_batch_formatting);
    expect_memory(__mock_end_batch_formatting, engine, engine, sizeof(struct engine));
    will_return(__mock_end_batch_formatting, 0);

    assert_int_equal(__real_prepare_buffers(engine), 0);

    free(localhost->rrdset_root->dimensions);
    free(localhost->rrdset_root);
    free(localhost);
    free(engine->connector_root->instance_root);
    free(engine->connector_root);
    free(engine);
}

int main(void)
{
    const struct CMUnitTest tests[] = { cmocka_unit_test(test_exporting_engine),
                                        cmocka_unit_test(test_prepare_buffers) };

    return cmocka_run_group_tests_name("exporting_engine", tests, NULL, NULL);
}

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

int main(void)
{
    const struct CMUnitTest tests[] = { cmocka_unit_test(test_exporting_engine) };

    return cmocka_run_group_tests_name("exporting_engine", tests, NULL, NULL);
}

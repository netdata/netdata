// SPDX-License-Identifier: GPL-3.0-or-later

#include "../../libnetdata/libnetdata.h"
#include "../../libnetdata/required_dummies.h"
#include <setjmp.h>
#include <cmocka.h>

static void test_exporting_engine(void **state)
{
    (void)state;

    assert_true(1);
}

int main(void)
{
    const struct CMUnitTest tests[] = {
        cmocka_unit_test(test_exporting_engine)
    };

    return cmocka_run_group_tests_name("exporting_engine", tests, NULL, NULL);
}

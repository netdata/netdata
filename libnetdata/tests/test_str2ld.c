// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"
#include "../required_dummies.h"
#include <setjmp.h>
#include <cmocka.h>

static void test_str2ld(void **state)
{
    (void)state;
    char *values[] = {
        "1.2345678",
        "-35.6",
        "0.00123",
        "23842384234234.2",
        ".1",
        "1.2e-10",
        "hello",
        "1wrong",
        "nan",
        "inf",
        NULL
    };

    for (int i = 0; values[i]; i++) {
        char *e_mine = "hello", *e_sys = "world";
        LONG_DOUBLE mine = str2ld(values[i], &e_mine);
        LONG_DOUBLE sys = strtold(values[i], &e_sys);

        if (isnan(mine))
            assert_true(isnan(sys));
        else if (isinf(mine))
            assert_true(isinf(sys));
        else if (mine != sys)
            assert_false(abs(mine - sys) > 0.000001);

        assert_ptr_equal(e_mine, e_sys);
    }
}

int main(void)
{
    const struct CMUnitTest tests[] = {
        cmocka_unit_test(test_str2ld)
    };

    return cmocka_run_group_tests_name("str2ld", tests, NULL, NULL);
}

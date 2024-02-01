// SPDX-License-Identifier: GPL-3.0-or-later

#include "../../libnetdata.h"
#include "../../required_dummies.h"
#include <setjmp.h>
#include <cmocka.h>

static void test_number_printing(void **state)
{
    (void)state;

    char value[50];

    print_netdata_double(value, 0);
    assert_string_equal(value, "0");

    print_netdata_double(value, 0.0000001);
    assert_string_equal(value, "0.0000001");

    print_netdata_double(value, 0.00000009);
    assert_string_equal(value, "0.0000001");

    print_netdata_double(value, 0.000000001);
    assert_string_equal(value, "0");

    print_netdata_double(value, 99.99999999999999999);
    assert_string_equal(value, "100");

    print_netdata_double(value, -99.99999999999999999);
    assert_string_equal(value, "-100");

    print_netdata_double(value, 123.4567890123456789);
    assert_string_equal(value, "123.456789");

    print_netdata_double(value, 9999.9999999);
    assert_string_equal(value, "9999.9999999");

    print_netdata_double(value, -9999.9999999);
    assert_string_equal(value, "-9999.9999999");

    print_netdata_double(value, unpack_storage_number(pack_storage_number(16.777218L, SN_DEFAULT_FLAGS)));
    assert_string_equal(value, "16.77722");
}

int main(void)
{
    const struct CMUnitTest tests[] = {
        cmocka_unit_test(test_number_printing)
    };

    return cmocka_run_group_tests_name("storage_number", tests, NULL, NULL);
}

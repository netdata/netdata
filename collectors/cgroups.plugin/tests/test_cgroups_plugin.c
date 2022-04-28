// SPDX-License-Identifier: GPL-3.0-or-later

#include "test_cgroups_plugin.h"
#include "libnetdata/required_dummies.h"

RRDHOST *localhost;
int netdata_zero_metrics_enabled = 1;
struct config netdata_config;
char *netdata_configured_primary_plugins_dir = NULL;

static void test_k8s_parse_resolved_name(void **state)
{
    UNUSED(state);

    struct label *labels = (struct label *)0xff;

    struct k8s_test_data {
        char *data;
        char *name;
        char *key[3];
        char *value[3];
    };

    struct k8s_test_data test_data[] = {
        // One label
        { .data = "name label1=\"value1\"",
          .name = "name",
          .key[0] = "label1", .value[0] = "value1" },

        // Three labels
        { .data = "name label1=\"value1\",label2=\"value2\",label3=\"value3\"",
          .name = "name",
          .key[0] = "label1", .value[0] = "value1",
          .key[1] = "label2", .value[1] = "value2",
          .key[2] = "label3", .value[2] = "value3" },

        // Comma at the end of the data string
        { .data = "name label1=\"value1\",",
          .name = "name",
          .key[0] = "label1", .value[0] = "value1" },

        // Equals sign in the value
        { .data = "name label1=\"value=1\"",
          .name = "name",
          .key[0] = "label1", .value[0] = "value=1" },

        // Double quotation mark in the value
        { .data = "name label1=\"value\"1\"",
          .name = "name",
          .key[0] = "label1", .value[0] = "value" },

        // Escaped double quotation mark in the value
        { .data = "name label1=\"value\\\"1\"",
          .name = "name",
          .key[0] = "label1", .value[0] = "value\\\"1" },

        // Equals sign in the key
        { .data = "name label=1=\"value1\"",
          .name = "name",
          .key[0] = "label", .value[0] = "1=\"value1\"" },

        // Skipped value
        { .data = "name label1=,label2=\"value2\"",
          .name = "name",
          .key[0] = "label2", .value[0] = "value2" },

        // A pair of equals signs
        { .data = "name= =",
          .name = "name=" },

        // A pair of commas
        { .data = "name, ,",
          .name = "name," },

        { .data = NULL }
    };

    for (int i = 0; test_data[i].data != NULL; i++) {
        char *data = strdup(test_data[i].data);

        for (int l = 0; l < 3 && test_data[i].key[l] != NULL; l++) {
            char *key = test_data[i].key[l];
            char *value = test_data[i].value[l];

            expect_function_call(__wrap_add_label_to_list);
            expect_value(__wrap_add_label_to_list, l, 0xff);
            expect_string(__wrap_add_label_to_list, key, key);
            expect_string(__wrap_add_label_to_list, value, value);
            expect_value(__wrap_add_label_to_list, label_source, LABEL_SOURCE_KUBERNETES);    
        }

        char *name = k8s_parse_resolved_name(&labels, data);

        assert_string_equal(name, test_data[i].name);
        assert_ptr_equal(labels, 0xff);

        free(data);
    }
}

int main(void)
{
    const struct CMUnitTest tests[] = {
        cmocka_unit_test(test_k8s_parse_resolved_name),
    };

    int test_res = cmocka_run_group_tests_name("test_k8s_parse_resolved_name", tests, NULL, NULL);

    return test_res;
}

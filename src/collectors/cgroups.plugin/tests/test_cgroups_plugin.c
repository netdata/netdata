// SPDX-License-Identifier: GPL-3.0-or-later

#include "test_cgroups_plugin.h"
#include "libnetdata/required_dummies.h"

RRDHOST *localhost;
struct config netdata_config;
char *netdata_configured_primary_plugins_dir = NULL;

struct k8s_test_data {
    char *data;
    char *name;
    char *key[3];
    char *value[3];
    
    const char *result_key[3];
    const char *result_value[3];
    int result_ls[3];
    int i;
};

static int read_label_callback(const char *name, const char *value, void *data)
{
  struct k8s_test_data *test_data = (struct k8s_test_data *)data;

  test_data->result_key[test_data->i] = name;
  test_data->result_value[test_data->i] = value;

  test_data->i++;

  return 1;
}

static void test_cgroup_parse_resolved_name(void **state)
{
    UNUSED(state);

    RRDLABELS *labels = rrdlabels_create();

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
        // { .data = "name label1=\"value=1\"",
        //   .name = "name",
        //   .key[0] = "label1", .value[0] = "value=1" },

        // Double quotation mark in the value
        // { .data = "name label1=\"value\"1\"",
        //   .name = "name",
        //   .key[0] = "label1", .value[0] = "value" },

        // Escaped double quotation mark in the value
        // { .data = "name label1=\"value\\\"1\"",
        //   .name = "name",
        //   .key[0] = "label1", .value[0] = "value\\\"1" },

        // Equals sign in the key
        // { .data = "name label=1=\"value1\"",
        //   .name = "name",
        //   .key[0] = "label", .value[0] = "1=\"value1\"" },

        // Skipped value
        // { .data = "name label1=,label2=\"value2\"",
        //   .name = "name",
        //   .key[0] = "label2", .value[0] = "value2" },

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

        char *name = cgroup_parse_resolved_name_and_labels(labels, data);

        assert_string_equal(name, test_data[i].name);

        rrdlabels_walkthrough_read(labels, read_label_callback, &test_data[i]);

        for (int l = 0; l < 3 && test_data[i].key[l] != NULL; l++) {
            char *key = test_data[i].key[l];
            char *value = test_data[i].value[l];

            const char *result_key = test_data[i].result_key[l];
            const char *result_value = test_data[i].result_value[l];
            int ls = test_data[i].result_ls[l];

            assert_string_equal(key, result_key);
            assert_string_equal(value, result_value);
            assert_int_equal(RRDLABEL_SRC_AUTO | RRDLABEL_SRC_K8S, ls);
        }

        free(data);
    }
}

int main(void)
{
    const struct CMUnitTest tests[] = {
        cmocka_unit_test(test_cgroup_parse_resolved_name),
    };

    int test_res = cmocka_run_group_tests_name("test_cgroup_parse_resolved_name", tests, NULL, NULL);

    return test_res;
}

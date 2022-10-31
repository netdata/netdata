#include <gtest/gtest.h>
#include "daemon/common.h"

extern "C" {
#include "collectors/cgroups.plugin/sys_fs_cgroup.h"
}

struct k8s_test_data {
    const char *data;
    const char *name;
    const char *key[3];
    const char *value[3];

    const char *result_key[3];
    const char *result_value[3];
    int result_ls[3];
    int i;
};

static int read_label_callback(const char *name, const char *value, RRDLABEL_SRC ls, void *data)
{
    struct k8s_test_data *test_data = (struct k8s_test_data *)data;

    test_data->result_key[test_data->i] = name;
    test_data->result_value[test_data->i] = value;
    test_data->result_ls[test_data->i] = ls;

    test_data->i++;

    return 1;
}

TEST(cgroups_plugin, k8s_parse_resolved_name)
{
    DICTIONARY *labels = rrdlabels_create();

    struct k8s_test_data test_data[] = {
        // One label
        {
             .data = "name label1=\"value1\"",
             .name = "name",
             .key = {"label1", NULL, NULL},
             .value = {"value1", NULL, NULL}
        },

        // Three labels
        {
            .data = "name label1=\"value1\",label2=\"value2\",label3=\"value3\"",
            .name = "name",
            .key = {"label1", "label2", "label3"},
            .value = {"value1", "value2", "value3"},
        },

        // Comma at the end of the data string
        {
            .data = "name label1=\"value1\",",
            .name = "name",
            .key = { "label1", NULL, NULL },
            .value = { "value1", NULL, NULL },
        },

        // A pair of equals signs
        {
            .data = "name= =",
            .name = "name="
        },

        // A pair of commas
        {
            .data = "name, ,",
            .name = "name,"
        },

        { .data = NULL, }
    };

    for (int i = 0; test_data[i].data != NULL; i++) {
        char *data = strdupz(test_data[i].data);
        char *name = k8s_parse_resolved_name_and_labels(labels, data);

        rrdlabels_walkthrough_read(labels, read_label_callback, &test_data[i]);

        EXPECT_STREQ(name, test_data[i].name);
        for (int l = 0; l < 3 && test_data[i].key[l] != NULL; l++) {
            EXPECT_STREQ(test_data[i].key[l], test_data[i].result_key[l]);
            EXPECT_STREQ(test_data[i].value[l], test_data[i].result_value[l]);
            EXPECT_EQ(test_data[i].result_ls[l], RRDLABEL_SRC_AUTO | RRDLABEL_SRC_K8S);
        }

        freez(data);
    }
}

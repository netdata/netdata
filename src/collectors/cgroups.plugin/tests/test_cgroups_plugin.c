// SPDX-License-Identifier: GPL-3.0-or-later

#include "test_cgroups_plugin.h"
#include "../cgroup-name-labels.h"

#include <sys/wait.h>

RRDHOST *localhost;
struct config netdata_config;
const char *cgroups_rename_script;
SIMPLE_PATTERN *enabled_cgroup_renames;

struct label_case {
    const char *name;
    const char *data;
    const char *expected_name;
    bool expected_ignored;
    const char *key[3];
    const char *value[3];
};

struct collected_label {
    const char *name;
    const char *value;
    RRDLABEL_SRC ls;
};

struct collected_labels {
    struct collected_label entry[3];
    int count;
};

static int read_label_callback(const char *name, const char *value, RRDLABEL_SRC ls, void *data)
{
    struct collected_labels *collected = (struct collected_labels *)data;

    if (collected->count < (int)(sizeof(collected->entry) / sizeof(collected->entry[0]))) {
        collected->entry[collected->count].name = name;
        collected->entry[collected->count].value = value;
        collected->entry[collected->count].ls = ls;
        collected->count++;
    }

    return 1;
}

static bool run_case(const struct label_case *tc)
{
    RRDLABELS *labels = rrdlabels_create();
    char *data = strdupz(tc->data);

    bool ignored = false;
    char *name = cgroup_parse_name_and_labels(labels, data, &ignored);

    bool ok = true;

    if (strcmp(name, tc->expected_name) != 0) {
        fprintf(stderr, "%s: expected name '%s', got '%s'\n", tc->name, tc->expected_name, name);
        ok = false;
    }

    if (ignored != tc->expected_ignored) {
        fprintf(stderr, "%s: expected ignored=%s, got %s\n",
                tc->name,
                tc->expected_ignored ? "true" : "false",
                ignored ? "true" : "false");
        ok = false;
    }

    struct collected_labels collected = { 0 };
    rrdlabels_walkthrough_read(labels, read_label_callback, &collected);

    int expected_count = 0;
    while (expected_count < 3 && tc->key[expected_count] != NULL)
        expected_count++;
    if (collected.count != expected_count) {
        fprintf(stderr, "%s: expected %d ordinary labels, got %d\n", tc->name, expected_count, collected.count);
        ok = false;
    }

    for (int l = 0; l < 3 && tc->key[l] != NULL; l++) {
        const char *key = tc->key[l];
        const char *value = tc->value[l];

        // rrdlabels iterates in interned-string order, not insertion order,
        // so locate the walked entry by key rather than by position.
        int found = -1;
        for (int r = 0; r < collected.count; r++) {
            if (strcmp(key, collected.entry[r].name) == 0) {
                found = r;
                break;
            }
        }
        if (found == -1) {
            fprintf(stderr, "%s: expected label key '%s' was not collected\n", tc->name, key);
            ok = false;
            continue;
        }

        if (strcmp(value, collected.entry[found].value) != 0) {
            fprintf(
                stderr,
                "%s: label '%s' expected value '%s', got '%s'\n",
                tc->name,
                key,
                value,
                collected.entry[found].value);
            ok = false;
        }

        // walkthrough_read exposes rrdlabels' internal FLAG_NEW/OLD bits;
        // mask them out to compare just the label source (see rrdlabels.c).
        RRDLABEL_SRC ls = collected.entry[found].ls & ~RRDLABEL_FLAG_INTERNAL;
        if (ls != (RRDLABEL_SRC_AUTO | RRDLABEL_SRC_K8S)) {
            fprintf(
                stderr,
                "%s: label '%s' expected source 0x%x, got 0x%x\n",
                tc->name,
                key,
                (unsigned)(RRDLABEL_SRC_AUTO | RRDLABEL_SRC_K8S),
                (unsigned)ls);
            ok = false;
        }
    }

    freez(data);
    rrdlabels_destroy(labels);

    return ok;
}

static bool test_cgroup_name_helper_gate(void)
{
    bool ok = true;
    enabled_cgroup_renames = simple_pattern_create("*", NULL, SIMPLE_PATTERN_EXACT, true);

    cgroups_rename_script = NULL;
    if (matches_enabled_cgroup_renames("/docker/fixture")) {
        fprintf(stderr, "missing cgroup-name helper did not disable rename matching\n");
        ok = false;
    }

    cgroups_rename_script = "/fixture/cgroup-name";
    if (!matches_enabled_cgroup_renames("/docker/fixture")) {
        fprintf(stderr, "available cgroup-name helper disabled rename matching\n");
        ok = false;
    }

    simple_pattern_free(enabled_cgroup_renames);
    enabled_cgroup_renames = NULL;
    cgroups_rename_script = NULL;
    return ok;
}

static bool test_cgroup_name_read_protocol(void)
{
    char maximum_line[CGROUP_NAME_LINE_MAX + 2];
    memset(maximum_line, 'x', CGROUP_NAME_LINE_MAX);
    maximum_line[CGROUP_NAME_LINE_MAX] = '\n';
    maximum_line[CGROUP_NAME_LINE_MAX + 1] = '\0';

    char oversized_record[CGROUP_NAME_LINE_MAX + 2];
    memset(oversized_record, 'x', CGROUP_NAME_LINE_MAX + 1);
    oversized_record[CGROUP_NAME_LINE_MAX + 1] = '\0';

    struct read_case {
        const char *name;
        const char *payload;
        bool close_writer;
        int timeout_ms;
        CGROUP_NAME_READ_RESULT expected;
    };
    const struct read_case cases[] = {
        { .name = "complete line", .payload = "name label=\"value\"\n", .close_writer = true,
          .timeout_ms = 100, .expected = CGROUP_NAME_READ_COMPLETE },
        { .name = "maximum line", .payload = maximum_line, .close_writer = true,
          .timeout_ms = 100, .expected = CGROUP_NAME_READ_COMPLETE },
        { .name = "oversized record", .payload = oversized_record, .close_writer = true,
          .timeout_ms = 100, .expected = CGROUP_NAME_READ_INVALID },
        { .name = "incomplete EOF", .payload = "name label=\"value\"", .close_writer = true,
          .timeout_ms = 100, .expected = CGROUP_NAME_READ_INVALID },
        { .name = "empty EOF", .payload = "", .close_writer = true,
          .timeout_ms = 100, .expected = CGROUP_NAME_READ_EMPTY },
        { .name = "partial writer hangs", .payload = "x", .close_writer = false,
          .timeout_ms = 25, .expected = CGROUP_NAME_READ_TIMEOUT },
    };

    bool ok = true;
    for (size_t i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        int fds[2];
        if (pipe(fds) != 0) {
            perror("pipe");
            return false;
        }

        size_t length = strlen(cases[i].payload);
        if (length && write(fds[1], cases[i].payload, length) != (ssize_t)length) {
            perror("write");
            ok = false;
        }
        if (cases[i].close_writer) {
            close(fds[1]);
            fds[1] = -1;
        }

        char buffer[CGROUP_NAME_LINE_MAX + 2];
        CGROUP_NAME_READ_RESULT result =
            cgroup_name_read_response(fds[0], buffer, sizeof(buffer), cases[i].timeout_ms);
        if (result != cases[i].expected) {
            fprintf(stderr, "%s: expected read result %d, got %d\n",
                    cases[i].name, cases[i].expected, result);
            ok = false;
        }

        close(fds[0]);
        if (fds[1] >= 0)
            close(fds[1]);
    }
    return ok;
}

static bool test_cgroup_name_read_deadline(void)
{
    int fds[2];
    if (pipe(fds) != 0) {
        perror("pipe");
        return false;
    }

    if (write(fds[1], "x", 1) != 1) {
        perror("write");
        close(fds[0]);
        close(fds[1]);
        return false;
    }

    pid_t pid = fork();
    if (pid < 0) {
        perror("fork");
        close(fds[0]);
        close(fds[1]);
        return false;
    }
    if (pid == 0) {
        close(fds[0]);
        for (int i = 0; i < 5; i++) {
            sleep_usec(10 * USEC_PER_MS);
            if (write(fds[1], "x", 1) != 1)
                _exit(1);
        }
        close(fds[1]);
        _exit(0);
    }

    close(fds[1]);
    char buffer[CGROUP_NAME_LINE_MAX + 2];
    CGROUP_NAME_READ_RESULT result = cgroup_name_read_response(fds[0], buffer, sizeof(buffer), 25);

    int status = 0;
    pid_t waited;
    do
        waited = waitpid(pid, &status, 0);
    while (waited < 0 && errno == EINTR);

    bool ok = waited == pid && WIFEXITED(status) && WEXITSTATUS(status) == 0;
    if (!ok)
        fprintf(stderr, "slow writer process failed\n");
    if (result != CGROUP_NAME_READ_TIMEOUT) {
        fprintf(stderr, "slow writer: expected read result %d, got %d\n",
                CGROUP_NAME_READ_TIMEOUT, result);
        ok = false;
    }

    close(fds[0]);
    return ok;
}

int main(void)
{
    // rrdlabels_create() allocates from a module-private aral that the daemon
    // sets up at startup; initialize it here so the parser test can create
    // RRDLABELS.
    rrdlabels_aral_init(false);

    static const struct label_case cases[] = {
        // One label
        { .name = "one label",
          .data = "name label1=\"value1\"",
          .expected_name = "name",
          .key[0] = "label1", .value[0] = "value1" },

        // Three labels
        { .name = "three labels",
          .data = "name label1=\"value1\",label2=\"value2\",label3=\"value3\"",
          .expected_name = "name",
          .key[0] = "label1", .value[0] = "value1",
          .key[1] = "label2", .value[1] = "value2",
          .key[2] = "label3", .value[2] = "value3" },

        // Quoted separators and escaped quotes in values
        { .name = "quoted separators and escaped quotes",
          .data = "name label1=\"value,1\",label2=\"value=2\",label3=\"value\\\"3\"",
          .expected_name = "name",
          .key[0] = "label1", .value[0] = "value.1",
          .key[1] = "label2", .value[1] = "value:2",
          .key[2] = "label3", .value[2] = "value_3" },

        // Comma at the end of the data string
        { .name = "trailing comma",
          .data = "name label1=\"value1\",",
          .expected_name = "name",
          .key[0] = "label1", .value[0] = "value1" },

        // Equals sign in the value
        // { .name = "equals sign in value",
        //   .data = "name label1=\"value=1\"",
        //   .expected_name = "name",
        //   .key[0] = "label1", .value[0] = "value=1" },

        // Double quotation mark in the value
        // { .name = "double quote in value",
        //   .data = "name label1=\"value\"1\"",
        //   .expected_name = "name",
        //   .key[0] = "label1", .value[0] = "value" },

        // Escaped double quotation mark in the value
        // { .name = "escaped double quote in value",
        //   .data = "name label1=\"value\\\"1\"",
        //   .expected_name = "name",
        //   .key[0] = "label1", .value[0] = "value\\\"1" },

        // Equals sign in the key
        // { .name = "equals sign in key",
        //   .data = "name label=1=\"value1\"",
        //   .expected_name = "name",
        //   .key[0] = "label", .value[0] = "1=\"value1\"" },

        // Skipped value
        // { .name = "skipped value",
        //   .data = "name label1=,label2=\"value2\"",
        //   .expected_name = "name",
        //   .key[0] = "label2", .value[0] = "value2" },

        // A pair of equals signs
        { .name = "pair of equals signs",
          .data = "name= =",
          .expected_name = "name=" },

        // A pair of commas
        { .name = "pair of commas",
          .data = "name, ,",
          .expected_name = "name," },

        { .name = "docker name override",
          .data = "name netdata.cloud/cgroup.name=\"override\"",
          .expected_name = "override" },

        { .name = "docker ignore control",
          .data = "name netdata.cloud/ignore=\"true\"",
          .expected_name = "name",
          .expected_ignored = true },

        { .name = "kubernetes name override",
          .data = "name k8s_netdata.cloud/cgroup.name=\"override\"",
          .expected_name = "override" },

        { .name = "kubernetes ignore control",
          .data = "name k8s_netdata.cloud/ignore=\"yes\"",
          .expected_name = "name",
          .expected_ignored = true },
    };

    size_t failures = 0;
    for (size_t i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        if (!run_case(&cases[i]))
            failures++;
    }

    if (!test_cgroup_name_helper_gate())
        failures++;
    if (!test_cgroup_name_read_protocol())
        failures++;
    if (!test_cgroup_name_read_deadline())
        failures++;

    if (failures) {
        fprintf(stderr, "%zu cgroups.plugin tests failed\n", failures);
        return 1;
    }

    return 0;
}

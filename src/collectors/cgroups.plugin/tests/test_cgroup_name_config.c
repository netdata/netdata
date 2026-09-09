// SPDX-License-Identifier: GPL-3.0-or-later

#include "../cgroup-name-config.h"

#include <limits.h>
#include <stdio.h>

static int test_timeout_conversion(void)
{
    struct timeout_case {
        const char *name;
        time_t seconds;
        int expected_ms;
    };
    static const struct timeout_case cases[] = {
        { .name = "negative timeout is unbounded", .seconds = -1, .expected_ms = 0 },
        { .name = "zero timeout is unbounded", .seconds = 0, .expected_ms = 0 },
        { .name = "one second", .seconds = 1, .expected_ms = 1000 },
        { .name = "default timeout", .seconds = 120, .expected_ms = 120000 },
        {
            .name = "largest whole-second timeout",
            .seconds = INT_MAX / 1000,
            .expected_ms = (INT_MAX / 1000) * 1000,
        },
        {
            .name = "overflow clamps to int max",
            .seconds = (time_t)(INT_MAX / 1000) + 1,
            .expected_ms = INT_MAX,
        },
    };

    int failures = 0;
    for (size_t i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        int actual = cgroup_name_timeout_ms_from_seconds(cases[i].seconds);
        if (actual != cases[i].expected_ms) {
            fprintf(stderr, "%s: expected %d, got %d\n", cases[i].name, cases[i].expected_ms, actual);
            failures++;
        }
    }

    return failures;
}

int main(void)
{
    struct migration_case {
        const char *name;
        const char *configured;
        const char *legacy_default;
        bool expected;
    };
    static const struct migration_case cases[] = {
        {
            .name = "exact legacy default migrates",
            .configured = "/usr/libexec/netdata/plugins.d/cgroup-name.sh",
            .legacy_default = "/usr/libexec/netdata/plugins.d/cgroup-name.sh",
            .expected = true,
        },
        {
            .name = "current default is preserved",
            .configured = "/usr/libexec/netdata/plugins.d/cgroup-name",
            .legacy_default = "/usr/libexec/netdata/plugins.d/cgroup-name.sh",
        },
        {
            .name = "custom helper is preserved",
            .configured = "/opt/custom/cgroup-name.sh",
            .legacy_default = "/usr/libexec/netdata/plugins.d/cgroup-name.sh",
        },
        {
            .name = "similar legacy path is preserved",
            .configured = "/usr/libexec/netdata/plugins.d/cgroup-name.sh.backup",
            .legacy_default = "/usr/libexec/netdata/plugins.d/cgroup-name.sh",
        },
        {
            .name = "empty helper is preserved",
            .configured = "",
            .legacy_default = "/usr/libexec/netdata/plugins.d/cgroup-name.sh",
        },
        {
            .name = "missing configured helper is preserved",
            .legacy_default = "/usr/libexec/netdata/plugins.d/cgroup-name.sh",
        },
        {
            .name = "missing legacy default is preserved",
            .configured = "/usr/libexec/netdata/plugins.d/cgroup-name.sh",
        },
    };

    int failures = 0;
    for (size_t i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        bool actual = cgroup_name_is_legacy_default_helper(cases[i].configured, cases[i].legacy_default);
        if (actual != cases[i].expected) {
            fprintf(stderr, "%s: expected %s, got %s\n",
                    cases[i].name,
                    cases[i].expected ? "true" : "false",
                    actual ? "true" : "false");
            failures++;
        }
    }

    failures += test_timeout_conversion();

    return failures ? 1 : 0;
}

// SPDX-License-Identifier: GPL-3.0-or-later

#include "../cgroup-name-config.h"

#include <stdio.h>

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

    return failures ? 1 : 0;
}

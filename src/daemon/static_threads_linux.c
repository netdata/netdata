// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

void cgroups_main(void *ptr);
void proc_main(void *ptr);
void diskspace_main(void *ptr);
void tc_main(void *ptr);
void timex_main(void *ptr);

static const struct netdata_static_thread static_threads_linux[] = {
    {
        .name = "P[tc]",
        .config_section = CONFIG_SECTION_PLUGINS,
        .config_name = "tc",
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = tc_main
    },
    {
        .name = "P[diskspace]",
        .config_section = CONFIG_SECTION_PLUGINS,
        .config_name = "diskspace",
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = diskspace_main
    },
    {
        .name = "P[proc]",
        .config_section = CONFIG_SECTION_PLUGINS,
        .config_name = "proc",
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = proc_main
    },
    {
        .name = "P[cgroups]",
        .config_section = CONFIG_SECTION_PLUGINS,
        .config_name = "cgroups",
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = cgroups_main
    },
    {
        .name = "P[timex]",
        .config_section = CONFIG_SECTION_PLUGINS,
        .config_name = "timex",
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = timex_main
    },

    // terminator
    {
        .name = NULL,
        .config_section = NULL,
        .config_name = NULL,
        .env_name = NULL,
        .enabled = 0,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = NULL
    }
};

struct netdata_static_thread *static_threads_get() {
    return static_threads_concat(static_threads_common, static_threads_linux);
}

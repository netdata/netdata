// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

extern void *cgroups_main(void *ptr);
extern void *proc_main(void *ptr);
extern void *diskspace_main(void *ptr);
extern void *tc_main(void *ptr);
extern void *timex_main(void *ptr);

const struct netdata_static_thread static_threads_linux[] = {
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

const struct netdata_static_thread static_threads_freebsd[] = {
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

const struct netdata_static_thread static_threads_macos[] = {
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

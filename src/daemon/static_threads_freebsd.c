// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

void *freebsd_main(void *ptr);
void *timex_main(void *ptr);

static const struct netdata_static_thread static_threads_freebsd[] = {
    {
        .name = "P[freebsd]",
        .config_section = CONFIG_SECTION_PLUGINS,
        .config_name = "freebsd",
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = freebsd_main
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

    {NULL, NULL, NULL, 0, NULL, NULL, NULL}
};

struct netdata_static_thread *static_threads_get() {
    return static_threads_concat(static_threads_common, static_threads_freebsd);
}

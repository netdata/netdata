// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

extern void *macos_main(void *ptr);

const struct netdata_static_thread static_threads_macos[] = {
    {
        .name = "P[macos]",
        .config_section = CONFIG_SECTION_PLUGINS,
        .config_name = "macos",
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = macos_main,
        .env_name = NULL,
        .global_variable = NULL,
    },

    {NULL, NULL, NULL, 0, NULL, NULL, NULL, NULL, NULL}
};

const struct netdata_static_thread static_threads_freebsd[] = {
    {NULL, NULL, NULL, 0, NULL, NULL, NULL, NULL, NULL}
};

const struct netdata_static_thread static_threads_linux[] = {
    {NULL, NULL, NULL, 0, NULL, NULL, NULL, NULL, NULL}
};

struct netdata_static_thread *static_threads_get() {
    return static_threads_concat(static_threads_common, static_threads_macos);
}

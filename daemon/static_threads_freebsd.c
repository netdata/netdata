// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

extern void *freebsd_main(void *ptr);

const struct netdata_static_thread static_threads_freebsd[] = {
    {
        .name = "P[freebsd]",
        .config_section = CONFIG_SECTION_PLUGINS,
        .config_name = "freebsd",
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = freebsd_main
    },

    {NULL, NULL, NULL, 0, NULL, NULL, NULL}
};

const struct netdata_static_thread static_threads_linux[] = {
    {NULL, NULL, NULL, 0, NULL, NULL, NULL}
};

const struct netdata_static_thread static_threads_macos[] = {
    {NULL, NULL, NULL, 0, NULL, NULL, NULL}
};

struct netdata_static_thread *static_threads_get() {
    return static_threads_concat(static_threads_common, static_threads_freebsd);
}

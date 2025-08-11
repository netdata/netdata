// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

#include <AvailabilityMacros.h>

void macos_main(void *ptr);

#if MAC_OS_X_VERSION_MIN_REQUIRED >= 101300
    void timex_main(void *ptr);
#endif

static const struct netdata_static_thread static_threads_macos[] = {
#if MAC_OS_X_VERSION_MIN_REQUIRED >= 101300
    {
        .name = "P[timex]",
        .config_section = CONFIG_SECTION_PLUGINS,
        .config_name = "timex",
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = timex_main
    },
#endif
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

struct netdata_static_thread *static_threads_get() {
    return static_threads_concat(static_threads_common, static_threads_macos);
}

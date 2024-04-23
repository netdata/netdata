// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

static const struct netdata_static_thread static_threads_windows[] = {
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
    return static_threads_concat(static_threads_common, static_threads_windows);
}

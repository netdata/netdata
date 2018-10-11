// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PLUGIN_IDLEJITTER_H
#define NETDATA_PLUGIN_IDLEJITTER_H 1

#include "../../common.h"

#define NETDATA_PLUGIN_HOOK_IDLEJITTER \
    { \
        .name = "PLUGIN[idlejitter]", \
        .config_section = CONFIG_SECTION_PLUGINS, \
        .config_name = "idlejitter", \
        .enabled = 1, \
        .thread = NULL, \
        .init_routine = NULL, \
        .start_routine = cpuidlejitter_main \
    },

extern void *cpuidlejitter_main(void *ptr);

#endif /* NETDATA_PLUGIN_IDLEJITTER_H */

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PLUGIN_CHECKS_H
#define NETDATA_PLUGIN_CHECKS_H 1

#include "../../daemon/common.h"

#ifdef NETDATA_INTERNAL_CHECKS

#define NETDATA_PLUGIN_HOOK_CHECKS \
    { \
        .name = "PLUGIN[check]", \
        .config_section = CONFIG_SECTION_PLUGINS, \
        .config_name = "checks", \
        .enabled = 0, \
        .thread = NULL, \
        .init_routine = NULL, \
        .start_routine = checks_main \
    },

extern void *checks_main(void *ptr);

#else // !NETDATA_INTERNAL_CHECKS

#define NETDATA_PLUGIN_HOOK_CHECKS

#endif // NETDATA_INTERNAL_CHECKS

#endif // NETDATA_PLUGIN_CHECKS_H

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PLUGIN_TIMEX_H
#define NETDATA_PLUGIN_TIMEX_H

#include "daemon/common.h"

#if (TARGET_OS == OS_LINUX)

#define NETDATA_PLUGIN_HOOK_LINUX_TIMEX \
    { \
        .name = "PLUGIN[timex]", \
        .config_section = CONFIG_SECTION_PLUGINS, \
        .config_name = "timex", \
        .enabled = 1, \
        .thread = NULL, \
        .init_routine = NULL, \
        .start_routine = timex_main \
    },

extern void *timex_main(void *ptr);

#else // (TARGET_OS == OS_LINUX)

#define NETDATA_PLUGIN_HOOK_LINUX_TIMEX

#endif // (TARGET_OS == OS_LINUX)

#endif //NETDATA_PLUGIN_TIMEX_H

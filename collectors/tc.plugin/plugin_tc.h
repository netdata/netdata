// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PLUGIN_TC_H
#define NETDATA_PLUGIN_TC_H 1

#include "../../daemon/common.h"

#if (TARGET_OS == OS_LINUX)

#define NETDATA_PLUGIN_HOOK_LINUX_TC \
    { \
        .name = "PLUGIN[tc]", \
        .config_section = CONFIG_SECTION_PLUGINS, \
        .config_name = "tc", \
        .enabled = 1, \
        .thread = NULL, \
        .init_routine = NULL, \
        .start_routine = tc_main \
    },

extern void *tc_main(void *ptr);

#else // (TARGET_OS == OS_LINUX)

#define NETDATA_PLUGIN_HOOK_LINUX_TC

#endif // (TARGET_OS == OS_LINUX)


#endif /* NETDATA_PLUGIN_TC_H */


// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SYS_FS_CGROUP_H
#define NETDATA_SYS_FS_CGROUP_H 1

#include "../../daemon/common.h"

#if (TARGET_OS == OS_LINUX)

#define NETDATA_PLUGIN_HOOK_LINUX_CGROUPS \
    { \
        .name = "PLUGIN[cgroups]", \
        .config_section = CONFIG_SECTION_PLUGINS, \
        .config_name = "cgroups", \
        .enabled = 1, \
        .thread = NULL, \
        .init_routine = NULL, \
        .start_routine = cgroups_main \
    },

extern void *cgroups_main(void *ptr);

#include "../proc.plugin/plugin_proc.h"

#else // (TARGET_OS == OS_LINUX)

#define NETDATA_PLUGIN_HOOK_LINUX_CGROUPS

#endif // (TARGET_OS == OS_LINUX)

#endif //NETDATA_SYS_FS_CGROUP_H

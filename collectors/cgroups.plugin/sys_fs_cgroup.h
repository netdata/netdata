// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SYS_FS_CGROUP_H
#define NETDATA_SYS_FS_CGROUP_H 1

#include "daemon/common.h"

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

#define CGROUP_OPTIONS_DISABLED_DUPLICATE   0x00000001
#define CGROUP_OPTIONS_SYSTEM_SLICE_SERVICE 0x00000002
#define CGROUP_OPTIONS_IS_UNIFIED           0x00000004

#include "../proc.plugin/plugin_proc.h"

#else // (TARGET_OS == OS_LINUX)

#define NETDATA_PLUGIN_HOOK_LINUX_CGROUPS

#endif // (TARGET_OS == OS_LINUX)

extern char *parse_k8s_data(struct label **labels, char *data);

#endif //NETDATA_SYS_FS_CGROUP_H

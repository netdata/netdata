// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PLUGIN_PROC_DISKSPACE_H
#define NETDATA_PLUGIN_PROC_DISKSPACE_H

#include "../../daemon/common.h"


#if (TARGET_OS == OS_LINUX)

#define NETDATA_PLUGIN_HOOK_LINUX_DISKSPACE \
    { \
        .name = "PLUGIN[diskspace]", \
        .config_section = CONFIG_SECTION_PLUGINS, \
        .config_name = "diskspace", \
        .enabled = 1, \
        .thread = NULL, \
        .init_routine = NULL, \
        .start_routine = diskspace_main \
    },

extern void *diskspace_main(void *ptr);

#include "../proc.plugin/plugin_proc.h"

#else // (TARGET_OS == OS_LINUX)

#define NETDATA_PLUGIN_HOOK_LINUX_DISKSPACE

#endif // (TARGET_OS == OS_LINUX)



#endif //NETDATA_PLUGIN_PROC_DISKSPACE_H

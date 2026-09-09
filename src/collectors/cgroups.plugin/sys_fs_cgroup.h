// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SYS_FS_CGROUP_H
#define NETDATA_SYS_FS_CGROUP_H 1

#include "database/rrd.h"

#define PLUGIN_CGROUPS_NAME "cgroups.plugin"
#define PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME "systemd"
#define PLUGIN_CGROUPS_MODULE_CGROUPS_NAME "/sys/fs/cgroup"

#define CGROUP_OPTIONS_DISABLED_DUPLICATE   (1 << 0)
#define CGROUP_OPTIONS_SYSTEM_SLICE_SERVICE (1 << 1)
#define CGROUP_OPTIONS_IS_UNIFIED           (1 << 2)
#define CGROUP_OPTIONS_DISABLED_EXCLUDED    (1 << 3)

// legacy SHM structs removed — cgroup metadata is shared via netipc (cgroup-netipc.c)

#include "../proc.plugin/plugin_proc.h"

#endif //NETDATA_SYS_FS_CGROUP_H

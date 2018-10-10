// SPDX-License-Identifier: GPL-3.0-or-later

#include "../common.h"

#ifndef NETDATA_ALL_H
#define NETDATA_ALL_H 1

// netdata internal data collection plugins

#include "checks.plugin/plugin_checks.h"
#include "freebsd.plugin/plugin_freebsd.h"
#include "idlejitter.plugin/plugin_idlejitter.h"
#include "linux-cgroups.plugin/sys_fs_cgroup.h"
#include "linux-diskspace.plugin/plugin_diskspace.h"
#include "linux-nfacct.plugin/plugin_nfacct.h"
#include "linux-proc.plugin/plugin_proc.h"
#include "linux-tc.plugin/plugin_tc.h"
#include "macos.plugin/plugin_macos.h"
#include "plugins.d.plugin/plugins_d.h"

#endif //NETDATA_ALL_H

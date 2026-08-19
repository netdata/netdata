// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_APPS_CGROUPS_PATH_H
#define NETDATA_APPS_CGROUPS_PATH_H 1

#include "apps_plugin.h"

#ifdef OS_LINUX
bool apps_cgroup_parse_proc_pid_cgroup_content(const char *content, char *dst, size_t dst_size);
#endif

#endif /* NETDATA_APPS_CGROUPS_PATH_H */

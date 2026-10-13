// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CGROUP_PATH_H
#define NETDATA_CGROUP_PATH_H 1

#include "libnetdata/libnetdata.h"

bool cgroup_path_parse_proc_pid_cgroup_content(const char *content, char *dst, size_t dst_size);
bool cgroup_path_is_namespace_relative(const char *path);

#endif /* NETDATA_CGROUP_PATH_H */

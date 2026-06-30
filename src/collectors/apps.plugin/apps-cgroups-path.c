// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps-cgroups-path.h"
#include "collectors/common-cgroups/cgroup-path.h"

#ifdef OS_LINUX

bool apps_cgroup_parse_proc_pid_cgroup_content(const char *content, char *dst, size_t dst_size)
{
    return cgroup_path_parse_proc_pid_cgroup_content(content, dst, dst_size);
}

#endif /* OS_LINUX */

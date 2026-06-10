// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CGROUP_EBPFGO_SHARED_MEMORY_H
#define NETDATA_CGROUP_EBPFGO_SHARED_MEMORY_H 1

#include "libnetdata/libnetdata.h"

#if defined(OS_LINUX)

#include "../collectors-ipc/ebpfgo_shared_memory.h"

bool cgroup_ebpfgo_shared_memory_refresh(void);
const struct ebpf_pid_stat *cgroup_ebpfgo_shared_memory_lookup(pid_t pid);
void cgroup_ebpfgo_shared_memory_close(void);

#endif

#endif

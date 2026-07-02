// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NETWORK_VIEWER_EBPF_SHARED_MEMORY_H
#define NETDATA_NETWORK_VIEWER_EBPF_SHARED_MEMORY_H 1

#include "libnetdata/libnetdata.h"

#if defined(OS_LINUX)

#include "../collectors-ipc/ebpfgo_shared_memory.h"

bool network_viewer_ebpf_shared_memory_refresh(void);
const struct ebpf_pid_stat *network_viewer_ebpf_shared_memory_lookup(pid_t pid);
void network_viewer_ebpf_shared_memory_close(void);

#endif

#endif

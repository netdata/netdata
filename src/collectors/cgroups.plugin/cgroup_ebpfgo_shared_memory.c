// SPDX-License-Identifier: GPL-3.0-or-later

#include "cgroup_ebpfgo_shared_memory.h"

#if defined(OS_LINUX)

static netdata_ebpfgo_shared_pid_memory_t cgroup_ebpfgo_shared_memory_ctx = {
    .shm_fd = -1,
    .sem = SEM_FAILED,
};

bool cgroup_ebpfgo_shared_memory_refresh(void)
{
    return netdata_ebpfgo_shared_pid_memory_refresh(
        &cgroup_ebpfgo_shared_memory_ctx,
        NETDATA_EBPFGO_INTEGRATION_NAME,
        NETDATA_EBPFGO_SHM_INTEGRATION_NAME);
}

const struct ebpf_pid_stat *cgroup_ebpfgo_shared_memory_lookup(pid_t pid)
{
    return netdata_ebpfgo_shared_pid_memory_lookup(&cgroup_ebpfgo_shared_memory_ctx, pid);
}

void cgroup_ebpfgo_shared_memory_close(void)
{
    netdata_ebpfgo_shared_pid_memory_close(&cgroup_ebpfgo_shared_memory_ctx);
}

#endif

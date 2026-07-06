// SPDX-License-Identifier: GPL-3.0-or-later

#include "network_viewer_ebpf_shared_memory.h"

#if defined(OS_LINUX)

static netdata_ebpfgo_shared_pid_memory_t nv_ebpfgo_shared_memory_ctx = {
    .shm_fd = -1,
    .sem = SEM_FAILED,
};

bool network_viewer_ebpf_shared_memory_refresh(void)
{
    return netdata_ebpfgo_shared_pid_memory_refresh(
        &nv_ebpfgo_shared_memory_ctx,
        NETDATA_EBPFGO_INTEGRATION_NAME,
        NETDATA_EBPFGO_SHM_INTEGRATION_NAME);
}

const struct ebpf_pid_stat *network_viewer_ebpf_shared_memory_lookup(pid_t pid)
{
    if (!(netdata_ebpfgo_shared_pid_memory_flags(&nv_ebpfgo_shared_memory_ctx) & EBPFGO_SHM_FLAG_SOCKET))
        return NULL;

    return netdata_ebpfgo_shared_pid_memory_lookup(&nv_ebpfgo_shared_memory_ctx, pid);
}

void network_viewer_ebpf_shared_memory_close(void)
{
    netdata_ebpfgo_shared_pid_memory_close(&nv_ebpfgo_shared_memory_ctx);
}

#endif

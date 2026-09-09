// SPDX-License-Identifier: GPL-3.0-or-later

#include "network_viewer_ebpf_shared_memory.h"

#if defined(OS_LINUX)

static netdata_ebpfgo_shared_pid_memory_t nv_ebpfgo_shared_memory_ctx = {
    .shm_fd = -1,
    .sem = SEM_FAILED,
};

/* Protects nv_ebpfgo_shared_memory_ctx against concurrent refresh and lookup.
 * refresh() can reallocz/freez ctx->snapshot; lookup() returns a pointer into
 * that same buffer, so both operations must be serialized. */
static netdata_mutex_t nv_ebpfgo_shm_mutex;

static void __attribute__((constructor)) nv_ebpfgo_shm_mutex_ctor(void) {
    netdata_mutex_init(&nv_ebpfgo_shm_mutex);
}

static void __attribute__((destructor)) nv_ebpfgo_shm_mutex_dtor(void) {
    netdata_mutex_destroy(&nv_ebpfgo_shm_mutex);
}

bool network_viewer_ebpf_shared_memory_refresh(void)
{
    netdata_mutex_lock(&nv_ebpfgo_shm_mutex);
    bool ok = netdata_ebpfgo_shared_pid_memory_refresh(
        &nv_ebpfgo_shared_memory_ctx,
        NETDATA_EBPFGO_INTEGRATION_NAME,
        NETDATA_EBPFGO_SHM_INTEGRATION_NAME);
    netdata_mutex_unlock(&nv_ebpfgo_shm_mutex);
    return ok;
}

bool network_viewer_ebpf_shared_memory_lookup(pid_t pid, struct ebpf_pid_stat *out)
{
    netdata_mutex_lock(&nv_ebpfgo_shm_mutex);

    bool found = false;
    if (netdata_ebpfgo_shared_pid_memory_flags(&nv_ebpfgo_shared_memory_ctx) & EBPFGO_SHM_FLAG_SOCKET) {
        const struct ebpf_pid_stat *item =
            netdata_ebpfgo_shared_pid_memory_lookup(&nv_ebpfgo_shared_memory_ctx, pid);
        if (item) {
            *out = *item;  /* copy under lock so the caller owns a stable snapshot */
            found = true;
        }
    }

    netdata_mutex_unlock(&nv_ebpfgo_shm_mutex);
    return found;
}

void network_viewer_ebpf_shared_memory_close(void)
{
    netdata_mutex_lock(&nv_ebpfgo_shm_mutex);
    netdata_ebpfgo_shared_pid_memory_close(&nv_ebpfgo_shared_memory_ctx);
    netdata_mutex_unlock(&nv_ebpfgo_shm_mutex);
}

#endif

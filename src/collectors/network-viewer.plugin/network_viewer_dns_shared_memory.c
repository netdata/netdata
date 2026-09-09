// SPDX-License-Identifier: GPL-3.0-or-later

#include "network_viewer_dns_shared_memory.h"

#if defined(OS_LINUX)

/* Context is large (~312 KB data copy) because struct ebpfgo_dns_shared
 * embeds the full 1000-record flow ring. Allocated as a static global so
 * it lives in BSS rather than the stack. */
static netdata_ebpfgo_dns_shared_memory_t nv_ebpfgo_dns_memory_ctx = {
    .shm_fd = -1,
    .sem = SEM_FAILED,
};

/* Serializes refresh (which overwrites ctx->data via 312 KB memcpy) against
 * snapshot (which copies ctx->data to the caller's buffer).  Without this
 * lock two concurrent DNS-query handlers can overlap a refresh write with a
 * snapshot read, producing torn data. */
static netdata_mutex_t nv_dns_shm_mutex;

static void __attribute__((constructor)) nv_dns_shm_mutex_ctor(void) {
    netdata_mutex_init(&nv_dns_shm_mutex);
}

static void __attribute__((destructor)) nv_dns_shm_mutex_dtor(void) {
    netdata_mutex_destroy(&nv_dns_shm_mutex);
}

bool network_viewer_dns_shared_memory_snapshot(struct ebpfgo_dns_shared *out)
{
    netdata_mutex_lock(&nv_dns_shm_mutex);

    bool ok = netdata_ebpfgo_dns_shared_memory_refresh(
        &nv_ebpfgo_dns_memory_ctx,
        NETDATA_EBPFGO_DNS_SHM_NAME,
        NETDATA_EBPFGO_DNS_SEM_NAME);

    if (ok)
        *out = nv_ebpfgo_dns_memory_ctx.data;  /* copy under lock; caller owns the snapshot */

    netdata_mutex_unlock(&nv_dns_shm_mutex);
    return ok;
}

void network_viewer_dns_shared_memory_close(void)
{
    netdata_mutex_lock(&nv_dns_shm_mutex);
    netdata_ebpfgo_dns_shared_memory_close(&nv_ebpfgo_dns_memory_ctx);
    netdata_mutex_unlock(&nv_dns_shm_mutex);
}

#endif /* OS_LINUX */

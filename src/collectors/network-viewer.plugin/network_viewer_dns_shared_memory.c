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

bool network_viewer_dns_shared_memory_refresh(void)
{
    return netdata_ebpfgo_dns_shared_memory_refresh(
        &nv_ebpfgo_dns_memory_ctx,
        NETDATA_EBPFGO_DNS_SHM_NAME,
        NETDATA_EBPFGO_DNS_SEM_NAME);
}

const struct ebpfgo_dns_aggregate *network_viewer_dns_shared_memory_get(void)
{
    if (!nv_ebpfgo_dns_memory_ctx.has_data)
        return NULL;
    return &nv_ebpfgo_dns_memory_ctx.data.agg;
}

const struct ebpfgo_dns_flow_record *network_viewer_dns_shared_memory_get_flows(uint32_t *count_out)
{
    if (!nv_ebpfgo_dns_memory_ctx.has_data) {
        if (count_out)
            *count_out = 0;
        return NULL;
    }

    uint32_t n = nv_ebpfgo_dns_memory_ctx.data.ring_count;
    if (n > NETDATA_EBPFGO_DNS_FLOW_RING_CAP)
        n = NETDATA_EBPFGO_DNS_FLOW_RING_CAP;

    if (count_out)
        *count_out = n;

    return nv_ebpfgo_dns_memory_ctx.data.ring;
}

void network_viewer_dns_shared_memory_close(void)
{
    netdata_ebpfgo_dns_shared_memory_close(&nv_ebpfgo_dns_memory_ctx);
}

#endif /* OS_LINUX */

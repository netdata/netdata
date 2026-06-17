// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NETWORK_VIEWER_DNS_SHARED_MEMORY_H
#define NETDATA_NETWORK_VIEWER_DNS_SHARED_MEMORY_H 1

#include "libnetdata/libnetdata.h"

#if defined(OS_LINUX)

#include "../collectors-ipc/ebpfgo_dns_shared_memory.h"

bool network_viewer_dns_shared_memory_refresh(void);
const struct ebpfgo_dns_aggregate    *network_viewer_dns_shared_memory_get(void);
const struct ebpfgo_dns_flow_record  *network_viewer_dns_shared_memory_get_flows(uint32_t *count_out);
void network_viewer_dns_shared_memory_close(void);

#endif /* OS_LINUX */

#endif /* NETDATA_NETWORK_VIEWER_DNS_SHARED_MEMORY_H */

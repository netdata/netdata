// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NETWORK_VIEWER_DNS_SHARED_MEMORY_H
#define NETDATA_NETWORK_VIEWER_DNS_SHARED_MEMORY_H 1

#include "libnetdata/libnetdata.h"

#if defined(OS_LINUX)

#include "../collectors-ipc/ebpfgo_dns_shared_memory.h"

/* Atomically refreshes the DNS SHM and copies the full snapshot into *out.
 * *out must point to caller-allocated storage (use mallocz(sizeof(*out))).
 * The caller owns *out and may read it without holding any lock.
 * Returns false when no live data is available (producer not running or data
 * too stale). */
bool network_viewer_dns_shared_memory_snapshot(struct ebpfgo_dns_shared *out);
void network_viewer_dns_shared_memory_close(void);

#endif /* OS_LINUX */

#endif /* NETDATA_NETWORK_VIEWER_DNS_SHARED_MEMORY_H */

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SHARED_DNS_MEMORY_H
#define NETDATA_SHARED_DNS_MEMORY_H 1

#include "apps_ebpf_shared_dns_row.h"

struct shared_dns_memory;

/* update_every_s: the publisher's collection interval in seconds.  Written into
 * the SHM header so the reader can compute a correct stale-timeout window. */
struct shared_dns_memory *shared_dns_memory_open(uint32_t update_every_s);

void shared_dns_memory_publish(
    struct shared_dns_memory *ctx,
    const struct ebpfgo_dns_aggregate *agg,
    const struct ebpfgo_dns_flow_record *flows,
    uint32_t flow_count);
/* Close invalidates the liveness marker before releasing local handles so
 * persistent SHM readers stop treating the last payload as live immediately. */

void shared_dns_memory_close(struct shared_dns_memory *ctx);

#endif /* NETDATA_SHARED_DNS_MEMORY_H */

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_APPS_EBPF_SHARED_DNS_ROW_H
#define NETDATA_APPS_EBPF_SHARED_DNS_ROW_H 1

#include <stdint.h>

#define NETDATA_EBPFGO_DNS_SHM_NAME "/netdata_shm_ebpfgo_dns"
#define NETDATA_EBPFGO_DNS_SEM_NAME "/netdata_sem_ebpfgo_dns"

/* Maximum per-query flow records kept in one SHM publish. */
#define NETDATA_EBPFGO_DNS_FLOW_RING_CAP 1000

/* Maximum domain name length (bytes, including NUL). */
#define NETDATA_EBPFGO_DNS_DOMAIN_MAX 256

/* Per-query completed DNS flow record.
 * sizeof == 320 bytes (verified by assertSharedDnsMemoryLayout). */
struct ebpfgo_dns_flow_record {
    uint64_t timestamp_us;                             /* CLOCK_MONOTONIC, µs             */
    uint64_t latency_us;                               /* query→response latency; 0 = n/a */
    uint32_t server_ip[4];                             /* IPv4: [0] only; IPv6: all 4     */
    uint32_t client_ip[4];
    char     domain[NETDATA_EBPFGO_DNS_DOMAIN_MAX];
    uint16_t client_port;
    uint16_t query_type;                               /* DNS QTYPE: 1=A, 28=AAAA, …      */
    uint16_t rcode;                                    /* DNS RCODE: 0=NOERROR, …         */
    uint8_t  protocol;                                 /* IPPROTO_UDP or IPPROTO_TCP       */
    uint8_t  ip_version;                               /* 4 or 6                           */
    uint8_t  timed_out;                                /* 1 if no response in 5 s          */
    uint8_t  _pad[7];                                  /* explicit pad → sizeof == 320     */
};

/* Aggregate DNS packet counters (one set per collection cycle). */
struct ebpfgo_dns_aggregate {
    uint64_t queries_udp4;
    uint64_t queries_udp6;
    uint64_t queries_tcp4;
    uint64_t queries_tcp6;
    uint64_t responses_udp4;
    uint64_t responses_udp6;
    uint64_t responses_tcp4;
    uint64_t responses_tcp6;
};

/* Full SHM region: aggregate counters + flat array of live per-query records.
 *
 * Writer publishes the current 20-second live set as ring[0..ring_count-1].
 * Reader copies all ring_count records under semaphore and scans them.
 * ring_count is always ≤ NETDATA_EBPFGO_DNS_FLOW_RING_CAP. */
struct ebpfgo_dns_shared {
    struct ebpfgo_dns_aggregate agg;                              /* offset   0 size  64 */
    uint32_t                    ring_count;                       /* offset  64 size   4 */
    uint32_t                    _pad;                             /* offset  68 size   4 */
    struct ebpfgo_dns_flow_record ring[NETDATA_EBPFGO_DNS_FLOW_RING_CAP]; /* offset 72 size 320000 */
};
/* sizeof(struct ebpfgo_dns_shared) == 320072 (~312 KB) */

#endif /* NETDATA_APPS_EBPF_SHARED_DNS_ROW_H */

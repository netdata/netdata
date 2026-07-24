// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_APPS_EBPF_SHARED_DNS_ROW_H
#define NETDATA_APPS_EBPF_SHARED_DNS_ROW_H 1

/* struct ebpfgo_shm_header is defined in apps_ebpf_shared_pid_row.h.
 * Including it here so both PID and DNS SHM share the same out-of-band
 * header protocol (flags, update_every_s, last_publish_ut, live_count). */
#include "apps_ebpf_shared_pid_row.h"

/* v1: initial versioned release; struct ebpfgo_shm_header moved to the
 * front of ebpfgo_dns_shared, replacing the embedded last_publish_ut /
 * ring_count / update_every_s fields.  Version suffix prevents old readers
 * from mapping the new layout at the wrong offset. */
#define NETDATA_EBPFGO_DNS_SHM_NAME "/netdata_shm_ebpfgo_dns_v1"
#define NETDATA_EBPFGO_DNS_SEM_NAME "/netdata_sem_ebpfgo_dns_v1"

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

/* Full SHM region.  struct ebpfgo_shm_header is at offset 0, matching the PID
 * SHM layout so both segments share the same liveness protocol.
 *
 *   hdr.last_publish_ut  — producer liveness marker; 0 = no live producer
 *   hdr.live_count       — valid flow records: ring[0..hdr.live_count-1]
 *   hdr.update_every_s   — publish interval; reader uses it for stale timeout
 *   hdr.flags            — reserved; 0 for this segment
 *
 * Writer publishes the current 20-second live set as ring[0..hdr.live_count-1].
 * Reader copies hdr + agg + only the live ring entries under semaphore. */
struct ebpfgo_dns_shared {
    struct ebpfgo_shm_header hdr;                                    /* offset   0 size  24 */
    struct ebpfgo_dns_aggregate agg;                                 /* offset  24 size  64 */
    struct ebpfgo_dns_flow_record ring[NETDATA_EBPFGO_DNS_FLOW_RING_CAP]; /* offset  88 size 320000 */
};
/* sizeof(struct ebpfgo_dns_shared) == 320088 (~312 KB) */

#endif /* NETDATA_APPS_EBPF_SHARED_DNS_ROW_H */

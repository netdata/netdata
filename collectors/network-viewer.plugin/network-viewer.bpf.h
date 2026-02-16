// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NETWORK_VIEWER_BPF_H
#define NETDATA_NETWORK_VIEWER_BPF_H

#include "vmlinux.h"

#ifndef AF_INET
#define AF_INET (u16)2
#endif

#ifndef AF_INET6
#define AF_INET6 (u16)10
#endif

#ifndef IPPROTO_TCP
#define IPPROTO_TCP 6
#endif

#ifndef IPPROTO_UDP
#define IPPROTO_UDP 17
#endif

typedef enum __attribute__((packed)) {
    CONN_TYPE_LOADED = 0,
    CONN_TYPE_DETECTED = 1,
} CONN_TYPE;

struct bpf_connection_key {
    u32 pid;        // Process ID
    u16 protocol;   // Protocol (TCP/UDP)
    u16 family;     // Protocol Family
    u16 src_port;   // Source protocol      network byte order conversion at user-space
    u16 dst_port;   // Destination port     network byte order conversion at user-space
    union {
        u32 ipv4;   //                      network byte order conversion at user-space
        struct in6_addr ipv6;
    } src_ip;
    union {
        u32 ipv4;   //                      network byte order conversion at user-space
        struct in6_addr ipv6;
    } dst_ip;
};

struct bpf_connection_data {
    int state;
    CONN_TYPE type;
    u64 timestamp_first_seen;
    u64 timestamp_last_seen;
    u64 total_bytes_sent;
    char comm[TASK_COMM_LEN];
};

#endif //NETDATA_NETWORK_VIEWER_BPF_H

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NETWORK_VIEWER_USERSPACE_BPF_H
#define NETDATA_NETWORK_VIEWER_USERSPACE_BPF_H

union bpf_ipv46 {
    uint32_t ipv4;
    struct in6_addr ipv6;
};

struct bpf_socket_endpoint {
    uint16_t port;
    union bpf_ipv46 ip;
};

typedef struct bpf_connection {
    uint16_t protocol;
    uint16_t family;
    int state;
    pid_t pid;

    int type;
    uint64_t first_seen_s;
    uint64_t last_seen_s;

    struct bpf_socket_endpoint local;
    struct bpf_socket_endpoint remote;

    char comm[TASK_COMM_LEN];
} BPF_CONNECTION;

uint32_t bpf_connection_data_size(void);
uint32_t bpf_connection_key_size(void);
void populate_connection_key_and_data(void *key, void *data, BPF_CONNECTION *c);
void populate_connection_from_key_and_data(BPF_CONNECTION *c, void *key, void *data);

#endif //NETDATA_NETWORK_VIEWER_USERSPACE_BPF_H

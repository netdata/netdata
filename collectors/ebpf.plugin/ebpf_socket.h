// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_SOCKET_H
#define NETDATA_EBPF_SOCKET_H 1

#define NETDATA_SOCKET_COUNTER 13

#define NETDATA_MAX_SOCKET_VECTOR 5

#define NETDATA_UDP_START 3

typedef enum ebpf_socket_idx {
    NETDATA_KEY_CALLS_TCP_SENDMSG,
    NETDATA_KEY_ERROR_TCP_SENDMSG,
    NETDATA_KEY_BYTES_TCP_SENDMSG,

    NETDATA_KEY_CALLS_TCP_CLEANUP_RBUF,
    NETDATA_KEY_ERROR_TCP_CLEANUP_RBUF,
    NETDATA_KEY_BYTES_TCP_CLEANUP_RBUF,

    NETDATA_KEY_CALLS_TCP_CLOSE,

    NETDATA_KEY_CALLS_UDP_RECVMSG,
    NETDATA_KEY_ERROR_UDP_RECVMSG,
    NETDATA_KEY_BYTES_UDP_RECVMSG,

    NETDATA_KEY_CALLS_UDP_SENDMSG,
    NETDATA_KEY_ERROR_UDP_SENDMSG,
    NETDATA_KEY_BYTES_UDP_SENDMSG
} ebpf_socket_index_t;

#define NETDATA_SOCKET_GROUP "Socket"

// Global chart name
#define NETDATA_TCP_FUNCTION_COUNT "tcp_functions"
#define NETDATA_TCP_FUNCTION_BYTES "tcp_bandwidth"
#define NETDATA_TCP_FUNCTION_ERROR "tcp_error"
#define NETDATA_UDP_FUNCTION_COUNT "udp_functions"
#define NETDATA_UDP_FUNCTION_BYTES "udp_bandwidth"
#define NETDATA_UDP_FUNCTION_ERROR "udp_error"

// Charts created on Apps submenu
#define NETDATA_NET_APPS_BANDWIDTH_SENT "bandwidth_sent"
#define NETDATA_NET_APPS_BANDWIDTH_RECV "bandwidth_recv"

typedef struct ebpf_socket_publish_apps {
    // Data read
    uint64_t sent;
    uint64_t received;

    // Publish information.
    uint64_t publish_sent;
    uint64_t publish_recv;
} ebpf_socket_publish_apps_t;

#endif /* NETDATA_EBPF_SOCKET_H */

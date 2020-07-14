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

//Port range
# define NETDATA_MINIMUM_PORT_VALUE 1
# define NETDATA_MAXIMUM_PORT_VALUE 65535

# define NETDATA_MINIMUM_IPV4_CIDR 0
# define NETDATA_MAXIMUM_IPV4_CIDR 32

typedef struct ebpf_socket_publish_apps {
    // Data read
    uint64_t sent;
    uint64_t received;

    // Publish information.
    uint64_t publish_sent;
    uint64_t publish_recv;
} ebpf_socket_publish_apps_t;

typedef struct ebpf_network_viewer_dimension_names {
    char *name;
    uint32_t hash;

    uint16_t port;

    struct ebpf_network_viewer_dimension_names *next;
} ebpf_network_viewer_dim_name_t ;

typedef struct ebpf_network_viewer_port_list {
    char *value;
    uint32_t hash;

    uint16_t first;
    uint16_t last;
    struct ebpf_network_viewer_port_list *next;
} ebpf_network_viewer_port_list_t;

union netdata_ip_t {
    uint8_t  addr8[16];
    uint16_t addr16[8];
    uint32_t addr32[4];
};

typedef struct ebpf_network_viewer_ip_list {
    char *value;            //IP value
    uint32_t hash;          //IP hash

    uint8_t ver;            //IP version

    union netdata_ip_t first;        //The IP address informed
    union netdata_ip_t last;        //The IP address informed

    struct ebpf_network_viewer_ip_list *next;
} ebpf_network_viewer_ip_list_t;

typedef struct ebpf_network_viewer_hostname_list {
    char *value;            //IP value
    uint32_t hash;          //IP hash

    SIMPLE_PATTERN *value_pattern;

    struct ebpf_network_viewer_hostname_list *next;
} ebpf_network_viewer_hostname_list_t;

typedef struct ebpf_network_viewer_options {
    uint32_t max_dim;   //Store value read from 'maximum dimensions'

    ebpf_network_viewer_port_list_t *excluded_port;
    ebpf_network_viewer_port_list_t *included_port;

    ebpf_network_viewer_dim_name_t *names;

    ebpf_network_viewer_ip_list_t *excluded_ips;
    ebpf_network_viewer_ip_list_t *included_ips;

    ebpf_network_viewer_hostname_list_t *excluded_hostnames;
    ebpf_network_viewer_hostname_list_t *included_hostnames;
} ebpf_network_viewer_options_t;

extern ebpf_network_viewer_options_t network_viewer_opt;

#endif

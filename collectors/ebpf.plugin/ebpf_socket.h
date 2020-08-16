// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef NETDATA_EBPF_SOCKET_H
#define NETDATA_EBPF_SOCKET_H 1
#include <stdint.h>
#include "libnetdata/avl/avl.h"

// Vector indexes
#define NETDATA_MAX_SOCKET_VECTOR 6
#define NETDATA_UDP_START 3
#define NETDATA_RETRANSMIT_START 5

#define NETDATA_SOCKET_APPS_HASH_TABLE 0
#define NETDATA_SOCKET_IPV4_HASH_TABLE 1
#define NETDATA_SOCKET_IPV6_HASH_TABLE 2
#define NETDATA_SOCKET_GLOBAL_HASH_TABLE 4
#define NETDATA_SOCKET_LISTEN_TABLE 5

#define NETDATA_SOCKET_READ_SLEEP_MS 800000ULL

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
    NETDATA_KEY_BYTES_UDP_SENDMSG,

    NETDATA_KEY_TCP_RETRANSMIT,

    NETDATA_SOCKET_COUNTER
} ebpf_socket_index_t;

#define NETDATA_SOCKET_GROUP "Socket"
#define NETDATA_NETWORK_CONNECTIONS_GROUP "Network connections"

// Global chart name
#define NETDATA_TCP_FUNCTION_COUNT "tcp_functions"
#define NETDATA_TCP_FUNCTION_BYTES "tcp_bandwidth"
#define NETDATA_TCP_FUNCTION_ERROR "tcp_error"
#define NETDATA_TCP_RETRANSMIT "tcp_retransmit"
#define NETDATA_UDP_FUNCTION_COUNT "udp_functions"
#define NETDATA_UDP_FUNCTION_BYTES "udp_bandwidth"
#define NETDATA_UDP_FUNCTION_ERROR "udp_error"

// Charts created on Apps submenu
#define NETDATA_NET_APPS_BANDWIDTH_SENT "bandwidth_sent"
#define NETDATA_NET_APPS_BANDWIDTH_RECV "bandwidth_recv"

// Network viewer charts
#define NETDATA_NV_OUTBOUND_BYTES "outbound_bytes"
#define NETDATA_NV_OUTBOUND_PACKETS "outbound_packets"
#define NETDATA_NV_OUTBOUND_RETRANSMIT "outbound_retransmit"
#define NETDATA_NV_INBOUND_BYTES "inbound_bytes"
#define NETDATA_NV_INBOUND_PACKETS "inbound_packets"

// Port range
#define NETDATA_MINIMUM_PORT_VALUE 1
#define NETDATA_MAXIMUM_PORT_VALUE 65535

#define NETDATA_MINIMUM_IPV4_CIDR 0
#define NETDATA_MAXIMUM_IPV4_CIDR 32

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

    uint16_t cmp_first;
    uint16_t cmp_last;

    uint8_t protocol;
    struct ebpf_network_viewer_port_list *next;
} ebpf_network_viewer_port_list_t;

/**
 * Union used to store ip addresses
 */
union netdata_ip_t {
    uint8_t  addr8[16];
    uint16_t addr16[8];
    uint32_t addr32[4];
    uint64_t addr64[2];
};

typedef struct ebpf_network_viewer_ip_list {
    char *value;            // IP value
    uint32_t hash;          // IP hash

    uint8_t ver;            // IP version

    union netdata_ip_t first;        // The IP address informed
    union netdata_ip_t last;        // The IP address informed

    struct ebpf_network_viewer_ip_list *next;
} ebpf_network_viewer_ip_list_t;

typedef struct ebpf_network_viewer_hostname_list {
    char *value;            // IP value
    uint32_t hash;          // IP hash

    SIMPLE_PATTERN *value_pattern;

    struct ebpf_network_viewer_hostname_list *next;
} ebpf_network_viewer_hostname_list_t;

#define NETDATA_NV_CAP_VALUE 50L
typedef struct ebpf_network_viewer_options {
    uint32_t max_dim;   // Store value read from 'maximum dimensions'

    uint32_t hostname_resolution_enabled;
    uint32_t service_resolution_enabled;

    ebpf_network_viewer_port_list_t *excluded_port;
    ebpf_network_viewer_port_list_t *included_port;

    ebpf_network_viewer_dim_name_t *names;

    ebpf_network_viewer_ip_list_t *excluded_ips;
    ebpf_network_viewer_ip_list_t *included_ips;

    ebpf_network_viewer_hostname_list_t *excluded_hostnames;
    ebpf_network_viewer_hostname_list_t *included_hostnames;

    ebpf_network_viewer_ip_list_t *ipv4_local_ip;
    ebpf_network_viewer_ip_list_t *ipv6_local_ip;
} ebpf_network_viewer_options_t;

extern ebpf_network_viewer_options_t network_viewer_opt;

/**
 * Structure to store socket information
 */
typedef struct netdata_socket {
    uint64_t recv_packets;
    uint64_t sent_packets;
    uint64_t recv_bytes;
    uint64_t sent_bytes;
    uint64_t first; // First timestamp
    uint64_t ct;   // Current timestamp
    uint16_t retransmit; // It is never used with UDP
    uint8_t protocol;
    uint8_t removeme;
    uint32_t reserved;
} netdata_socket_t __attribute__((__aligned__(8)));


typedef struct netdata_plot_values {
    // Values used in the previous iteration
    uint64_t recv_packets;
    uint64_t sent_packets;
    uint64_t recv_bytes;
    uint64_t sent_bytes;
    uint16_t retransmit;

    uint64_t last_time;

    // Values used to plot
    uint64_t plot_recv_packets;
    uint64_t plot_sent_packets;
    uint64_t plot_recv_bytes;
    uint64_t plot_sent_bytes;
    uint16_t plot_retransmit;
} netdata_plot_values_t;

/**
 * Index used together previous structure
 */
typedef struct netdata_socket_idx {
    union netdata_ip_t saddr;
    uint16_t sport;
    union netdata_ip_t daddr;
    uint16_t dport;
} netdata_socket_idx_t __attribute__((__aligned__(8)));

// Next values were defined according getnameinfo(3)
#define NETDATA_MAX_NETWORK_COMBINED_LENGTH 1018
#define NETDATA_DOTS_PROTOCOL_COMBINED_LENGTH 5 // :TCP:
#define NETDATA_DIM_LENGTH_WITHOUT_SERVICE_PROTOCOL 979

#define NETDATA_INBOUND_DIRECTION (uint32_t)1
#define NETDATA_OUTBOUND_DIRECTION (uint32_t)2
/**
 * Allocate the maximum number of structures in the beginning, this can force the collector to use more memory
 * in the long term, on the other had it is faster.
 */
typedef struct netdata_socket_plot {
    // Search
    avl avl;
    netdata_socket_idx_t index;

    // Current data
    netdata_socket_t sock;

    // Previous values and values used to write on chart.
    netdata_plot_values_t plot;

    int family;                     // AF_INET or AF_INET6
    char *resolved_name;            // Resolve only in the first call
    unsigned char resolved;

    char *dimension_sent;
    char *dimension_recv;
    char *dimension_retransmit;

    uint32_t flags;
} netdata_socket_plot_t;

#define NETWORK_VIEWER_CHARTS_CREATED (uint32_t)1
typedef struct netdata_vector_plot {
    netdata_socket_plot_t *plot;    // Vector used to plot charts

    avl_tree_lock tree;             // AVL tree to speed up search
    uint32_t last;                  // The 'other' dimension, the last chart accepted.
    uint32_t next;                  // The next position to store in the vector.
    uint32_t max_plot;              // Max number of elements to plot.
    uint32_t last_plot;             // Last element plot

    uint32_t flags;                 // Flags

} netdata_vector_plot_t;

extern void clean_port_structure(ebpf_network_viewer_port_list_t **clean);
extern ebpf_network_viewer_port_list_t *listen_ports;
extern void update_listen_table(uint16_t value, uint8_t proto);

#endif

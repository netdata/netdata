// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef NETDATA_EBPF_SOCKET_H
#define NETDATA_EBPF_SOCKET_H 1
#include <stdint.h>
#include "libnetdata/avl/avl.h"

#include <sys/socket.h>
#ifdef HAVE_NETDB_H
#include <netdb.h>
#endif

// Module name & description
#define NETDATA_EBPF_MODULE_NAME_SOCKET "socket"
#define NETDATA_EBPF_SOCKET_MODULE_DESC                                                                                \
    "Monitors TCP and UDP bandwidth. This thread is integrated with apps and cgroup."

// Vector indexes
#define NETDATA_UDP_START 3

// dimensions
#define EBPF_COMMON_UNITS_CONNECTIONS "connections/s"
#define EBPF_COMMON_UNITS_KILOBITS "kilobits/s"

// config file
#define NETDATA_NETWORK_CONFIG_FILE "network.conf"
#define EBPF_NETWORK_VIEWER_SECTION "network connections"
#define EBPF_SERVICE_NAME_SECTION "service name"
#define EBPF_CONFIG_RESOLVE_HOSTNAME "resolve hostnames"
#define EBPF_CONFIG_RESOLVE_SERVICE "resolve service names"
#define EBPF_CONFIG_PORTS "ports"
#define EBPF_CONFIG_HOSTNAMES "hostnames"
#define EBPF_CONFIG_SOCKET_MONITORING_SIZE "socket monitoring table size"
#define EBPF_CONFIG_UDP_SIZE "udp connection table size"

enum ebpf_socket_table_list {
    NETDATA_SOCKET_GLOBAL,
    NETDATA_SOCKET_LPORTS,
    NETDATA_SOCKET_OPEN_SOCKET,
    NETDATA_SOCKET_TABLE_UDP,
    NETDATA_SOCKET_TABLE_CTRL
};

enum ebpf_socket_publish_index {
    NETDATA_IDX_TCP_SENDMSG,
    NETDATA_IDX_TCP_CLEANUP_RBUF,
    NETDATA_IDX_TCP_CLOSE,
    NETDATA_IDX_UDP_RECVBUF,
    NETDATA_IDX_UDP_SENDMSG,
    NETDATA_IDX_TCP_RETRANSMIT,
    NETDATA_IDX_TCP_CONNECTION_V4,
    NETDATA_IDX_TCP_CONNECTION_V6,
    NETDATA_IDX_INCOMING_CONNECTION_TCP,
    NETDATA_IDX_INCOMING_CONNECTION_UDP,

    // Keep this as last and don't skip numbers as it is used as element counter
    NETDATA_MAX_SOCKET_VECTOR
};

enum socket_functions {
    NETDATA_FCNT_INET_CSK_ACCEPT,
    NETDATA_FCNT_TCP_RETRANSMIT,
    NETDATA_FCNT_CLEANUP_RBUF,
    NETDATA_FCNT_TCP_CLOSE,
    NETDATA_FCNT_UDP_RECEVMSG,
    NETDATA_FCNT_TCP_SENDMSG,
    NETDATA_FCNT_UDP_SENDMSG,
    NETDATA_FCNT_TCP_V4_CONNECT,
    NETDATA_FCNT_TCP_V6_CONNECT
};

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

    NETDATA_KEY_CALLS_TCP_CONNECT_IPV4,
    NETDATA_KEY_ERROR_TCP_CONNECT_IPV4,

    NETDATA_KEY_CALLS_TCP_CONNECT_IPV6,
    NETDATA_KEY_ERROR_TCP_CONNECT_IPV6,

    // Keep this as last and don't skip numbers as it is used as element counter
    NETDATA_SOCKET_COUNTER
} ebpf_socket_index_t;

#define NETDATA_SOCKET_KERNEL_FUNCTIONS "kernel"
#define NETDATA_CGROUP_NET_GROUP "network"

// Global chart name
#define NETDATA_TCP_OUTBOUND_CONNECTIONS "tcp_outbound_conn"
#define NETDATA_INBOUND_CONNECTIONS "inbound_conn"
#define NETDATA_TCP_FUNCTION_COUNT "tcp_functions"
#define NETDATA_TCP_FUNCTION_BITS "total_tcp_bandwidth"
#define NETDATA_TCP_FUNCTION_ERROR "tcp_error"
#define NETDATA_TCP_RETRANSMIT "tcp_retransmit"
#define NETDATA_UDP_FUNCTION_COUNT "udp_functions"
#define NETDATA_UDP_FUNCTION_BITS "total_udp_bandwidth"
#define NETDATA_UDP_FUNCTION_ERROR "udp_error"

// Charts created (id or suffix)
#define NETDATA_SOCK_ID_OR_SUFFIX_CONNECTION_TCP_V4 "outbound_conn_v4"
#define NETDATA_SOCK_ID_OR_SUFFIX_CONNECTION_TCP_V6 "outbound_conn_v6"
#define NETDATA_SOCK_ID_OR_SUFFIX_BANDWIDTH "total_bandwidth"
#define NETDATA_SOCK_ID_OR_SUFFIX_BANDWIDTH_TCP_SEND_CALLS "bandwidth_tcp_send"
#define NETDATA_SOCK_ID_OR_SUFFIX_BANDWIDTH_TCP_RECV_CALLS "bandwidth_tcp_recv"
#define NETDATA_SOCK_ID_OR_SUFFIX_BANDWIDTH_TCP_RETRANSMIT "bandwidth_tcp_retransmit"
#define NETDATA_SOCK_ID_OR_SUFFIX_BANDWIDTH_UDP_SEND_CALLS "bandwidth_udp_send"
#define NETDATA_SOCK_ID_OR_SUFFIX_BANDWIDTH_UDP_RECV_CALLS "bandwidth_udp_recv"

// Port range
#define NETDATA_MINIMUM_PORT_VALUE 1
#define NETDATA_MAXIMUM_PORT_VALUE 65535
#define NETDATA_COMPILED_CONNECTIONS_ALLOWED 65535U
#define NETDATA_MAXIMUM_CONNECTIONS_ALLOWED 16384U
#define NETDATA_COMPILED_UDP_CONNECTIONS_ALLOWED 8192U
#define NETDATA_MAXIMUM_UDP_CONNECTIONS_ALLOWED 4096U

#define NETDATA_MINIMUM_IPV4_CIDR 0
#define NETDATA_MAXIMUM_IPV4_CIDR 32

// Contexts
#define NETDATA_CGROUP_TCP_V4_CONN_CONTEXT "cgroup.net_conn_ipv4"
#define NETDATA_CGROUP_TCP_V6_CONN_CONTEXT "cgroup.net_conn_ipv6"
#define NETDATA_CGROUP_SOCKET_TCP_BANDWIDTH_CONTEXT "cgroup.net_total_bandwidth"
#define NETDATA_CGROUP_SOCKET_TCP_RECV_CONTEXT "cgroup.net_tcp_recv"
#define NETDATA_CGROUP_SOCKET_TCP_SEND_CONTEXT "cgroup.net_tcp_send"
#define NETDATA_CGROUP_SOCKET_TCP_RETRANSMIT_CONTEXT "cgroup.net_retransmit"
#define NETDATA_CGROUP_SOCKET_UDP_RECV_CONTEXT "cgroup.net_udp_recv"
#define NETDATA_CGROUP_SOCKET_UDP_SEND_CONTEXT "cgroup.net_udp_send"

#define NETDATA_SERVICES_SOCKET_TCP_V4_CONN_CONTEXT "systemd.service.net_conn_ipv4"
#define NETDATA_SERVICES_SOCKET_TCP_V6_CONN_CONTEXT "systemd.service.net_conn_ipv6"
#define NETDATA_SERVICES_SOCKET_TCP_BANDWIDTH_CONTEXT "systemd.service.net_total_bandwidth"
#define NETDATA_SERVICES_SOCKET_TCP_RECV_CONTEXT "systemd.service.net_tcp_recv"
#define NETDATA_SERVICES_SOCKET_TCP_SEND_CONTEXT "systemd.service.net_tcp_send"
#define NETDATA_SERVICES_SOCKET_TCP_RETRANSMIT_CONTEXT "systemd.service.net_retransmit"
#define NETDATA_SERVICES_SOCKET_UDP_RECV_CONTEXT "systemd.service.net_udp_recv"
#define NETDATA_SERVICES_SOCKET_UDP_SEND_CONTEXT "systemd.service.net_udp_send"

// ARAL name
#define NETDATA_EBPF_SOCKET_ARAL_NAME "ebpf_socket"
#define NETDATA_EBPF_PID_SOCKET_ARAL_TABLE_NAME "ebpf_pid_socket"
#define NETDATA_EBPF_SOCKET_ARAL_TABLE_NAME "ebpf_socket_tbl"

typedef struct __attribute__((packed)) ebpf_socket_publish_apps {
    // Data read
    uint64_t bytes_sent;             // Bytes sent
    uint64_t bytes_received;         // Bytes received
    uint64_t call_tcp_sent;          // Number of times tcp_sendmsg was called
    uint64_t call_tcp_received;      // Number of times tcp_cleanup_rbuf was called
    uint64_t retransmit;             // Number of times tcp_retransmit was called
    uint64_t call_udp_sent;          // Number of times udp_sendmsg was called
    uint64_t call_udp_received;      // Number of times udp_recvmsg was called
    uint64_t call_close;             // Number of times tcp_close was called
    uint64_t call_tcp_v4_connection; // Number of times tcp_v4_connect was called
    uint64_t call_tcp_v6_connection; // Number of times tcp_v6_connect was called
} ebpf_socket_publish_apps_t;

typedef struct ebpf_network_viewer_dimension_names {
    char *name;
    uint32_t hash;

    uint16_t port;

    struct ebpf_network_viewer_dimension_names *next;
} ebpf_network_viewer_dim_name_t;

typedef struct ebpf_network_viewer_port_list {
    char *value;
    uint32_t hash;

    uint16_t first;
    uint16_t last;

    uint16_t cmp_first;
    uint16_t cmp_last;

    uint16_t protocol;
    uint32_t pid;
    uint32_t tgid;
    uint64_t connections;
    struct ebpf_network_viewer_port_list *next;
} ebpf_network_viewer_port_list_t;

typedef struct netdata_passive_connection {
    uint32_t tgid;
    uint32_t pid;
    uint64_t counter;
} netdata_passive_connection_t;

typedef struct netdata_passive_connection_idx {
    uint16_t protocol;
    uint16_t port;
} netdata_passive_connection_idx_t;

/**
 * Union used to store ip addresses
 */
union netdata_ip_t {
    uint8_t addr8[16];
    uint16_t addr16[8];
    uint32_t addr32[4];
    uint64_t addr64[2];
};

typedef struct ebpf_network_viewer_ip_list {
    char *value;   // IP value
    uint32_t hash; // IP hash

    uint8_t ver; // IP version

    union netdata_ip_t first; // The IP address informed
    union netdata_ip_t last;  // The IP address informed

    struct ebpf_network_viewer_ip_list *next;
} ebpf_network_viewer_ip_list_t;

typedef struct ebpf_network_viewer_hostname_list {
    char *value;   // IP value
    uint32_t hash; // IP hash

    SIMPLE_PATTERN *value_pattern;

    struct ebpf_network_viewer_hostname_list *next;
} ebpf_network_viewer_hostname_list_t;

typedef struct ebpf_network_viewer_options {
    RW_SPINLOCK rw_spinlock;

    uint32_t enabled;
    uint32_t family; // AF_INET, AF_INET6 or AF_UNSPEC (both)

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

typedef enum netdata_socket_flags { NETDATA_SOCKET_FLAGS_ALREADY_OPEN = (1 << 0) } netdata_socket_flags_t;

typedef enum netdata_socket_src_ip_origin {
    NETDATA_EBPF_SRC_IP_ORIGIN_LOCAL,
    NETDATA_EBPF_SRC_IP_ORIGIN_EXTERNAL
} netdata_socket_src_ip_origin_t;

typedef struct netata_socket_plus {
    netdata_socket_t data; // Data read from database
    uint32_t pid;
    time_t last_update;
    netdata_socket_flags_t flags;

    struct {
        char src_ip[INET6_ADDRSTRLEN + 1];
        //       uint16_t src_port;
        char dst_ip[INET6_ADDRSTRLEN + 1];
        char dst_port[NI_MAXSERV + 1];
    } socket_string;
} netdata_socket_plus_t;

extern ARAL *aral_socket_table;

/**
 * Index used together previous structure
 */
typedef struct netdata_socket_idx {
    union netdata_ip_t saddr;
    //uint16_t sport;
    union netdata_ip_t daddr;
    uint16_t dport;
    uint32_t pid;
} netdata_socket_idx_t;

void ebpf_clean_port_structure(ebpf_network_viewer_port_list_t **clean);
extern ebpf_network_viewer_port_list_t *listen_ports;
void update_listen_table(uint16_t value, uint16_t proto, netdata_passive_connection_t *values);
void ebpf_fill_ip_list_unsafe(ebpf_network_viewer_ip_list_t **out, ebpf_network_viewer_ip_list_t *in, char *table);
void ebpf_parse_service_name_section(struct config *cfg);
void ebpf_parse_ips_unsafe(const char *ptr);
void ebpf_parse_ports(const char *ptr);
void ebpf_socket_read_open_connections(BUFFER *buf, struct ebpf_module *em);
void ebpf_socket_fill_publish_apps(ebpf_socket_publish_apps_t *curr, netdata_socket_t *ns);

extern struct config socket_config;
extern netdata_ebpf_targets_t socket_targets[];

#endif

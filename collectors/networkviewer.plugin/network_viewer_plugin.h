#ifndef _NETDATA_NETWORK_VIEWER_H_
# define _NETDATA_NETWORK_VIEWER_H_ 1

# ifndef __FreeBSD__
#   include <linux/perf_event.h>
# endif
# include <stdint.h>
# include <errno.h>
# include <signal.h>
# include <stdio.h>
# include <stdint.h>
# include <stdlib.h>
# include <string.h>
# include <unistd.h>
# include <dlfcn.h>

# include <fcntl.h>
# include <ctype.h>
# include <netinet/in.h>
# include <arpa/inet.h>
# include <string.h>
# include <net/if.h>

# include <arpa/inet.h>
# include <sys/socket.h>
# include <sys/types.h>

# ifndef __FreeBSD__
#   include <linux/netlink.h>
#   include <linux/rtnetlink.h>
# endif

# include <netinet/in.h>
# include <ifaddrs.h>

# include <sys/sysinfo.h>
# include <pthread.h>

//From libnetdata.h
# include "../../libnetdata/threads/threads.h"
# include "../../libnetdata/locks/locks.h"
# include "../../libnetdata/avl/avl.h"
# include "../../libnetdata/clocks/clocks.h"

union netdata_ip {
    __u64 addr64[2];
    __u32 addr32[4];
};

typedef struct {
    uint64_t first;
    uint64_t ct;
    union netdata_ip saddr;
    union netdata_ip daddr;
    uint16_t dport;
    uint16_t retransmit;
    uint64_t sent;
    uint64_t recv;
    uint8_t protocol;
    uint16_t family;
    uint8_t removeme;
}netdata_kern_stats_t;

typedef struct parse_text_input{
    char *value;
    uint16_t port;

    struct parse_text_input *next;
}parse_text_input_t;

typedef struct  netdata_conn_stats{
    avl avl;

    uint64_t first;
    uint64_t ct;
    union netdata_ip saddr;
    union netdata_ip daddr;
    uint16_t dport;
    uint16_t retransmit;
    uint64_t sent;
    uint64_t recv;
    uint8_t protocol;
    uint16_t family;
    time_t remove_time;

    struct netdata_conn_stats *prev;
    struct netdata_conn_stats * next;
}netdata_conn_stats_t;

typedef struct  netdata_port_stats {
    avl avl;

    uint16_t port;
    uint8_t protocol;
    uint16_t family;

    uint64_t iprev;
    uint64_t inow;
    uint32_t itot;

    uint64_t eprev;
    uint64_t enow;
    uint32_t etot;

    char *dimension;

    struct netdata_port_stats *next;

    avl_tree_lock destination_port;
}netdata_port_stats_t;

typedef struct netdata_port_list {
    avl avl;

    uint16_t port;
    char *dimension;

    struct netdata_port_list *next;
}netdata_port_list_t;

typedef struct netdata_control_connection{
    avl_tree_lock port_stat_ipv4;
    avl_tree_lock port_stat_ipv6;

    avl_tree_lock port_list;

    netdata_conn_stats_t *tree;
    netdata_conn_stats_t *last_connection;

    netdata_port_stats_t *ports_ipv4;
    netdata_port_stats_t *last_port_ipv4;

    netdata_port_stats_t *ports_ipv6;
    netdata_port_stats_t *last_port_ipv6;

    uint16_t maxports;
    parse_text_input_t *pti;
} netdata_control_connection_t;

typedef struct netdata_network {
    in_addr_t ipv4addr;
    in_addr_t first;
    in_addr_t netmask;

    struct netdata_network *next;
} netdata_network_t;

# define NETDATA_MAX_PROCESSOR 128

# define NETDATA_MAX_DIMENSION 50

# define NETWORK_VIEWER_FAMILY "network_viewer"
# define NETWORK_VIEWER_TCP_INBOUND_IPV4 "TCP_transf_inbound_ipv4"
# define NETWORK_VIEWER_TCP_OUTBOUND_IPV4 "TCP_transf_outbound_ipv4"
# define NETWORK_VIEWER_UDP_INBOUND_IPV4 "UDP_transf_inbound_ipv4"
# define NETWORK_VIEWER_UDP_OUTBOUND_IPV4 "UDP_transf_outbound_ipv4"

//# define NETWORK_VIEWER_CHART5 "TCP_conn_inbound_ipv4"
# define NETWORK_VIEWER_TCP_CONNECTION_OUTBOUND_IPV4 "TCP_conn_outbound_ipv4"
//# define NETWORK_VIEWER_CHART7 "UDP_conn_inbound_ipv4"
# define NETWORK_VIEWER_UDP_CONNECTION_OUTBOUND_IPV4 "UDP_conn_outbound_ipv4"

# define NETWORK_VIEWER_TCP_INBOUND_IPV6 "TCP_transf_inbound_ipv6"
# define NETWORK_VIEWER_TCP_OUTBOUND_IPV6 "TCP_transf_outbound_ipv6"
# define NETWORK_VIEWER_UDP_INBOUND_IPV6 "UDP_transf_inbound_ipv6"
# define NETWORK_VIEWER_UDP_OUTBOUND_IPV6 "UDP_transf_outbound_ipv6"

//# define NETWORK_VIEWER_CHART13 "TCP_conn_inbound_ipv6"
# define NETWORK_VIEWER_TCP_CONNECTION_OUTBOUND_IPV6 "TCP_conn_outbound_ipv6"
//# define NETWORK_VIEWER_CHART15 "UDP_conn_inbound_ipv6"
# define NETWORK_VIEWER_UDP_CONNECTION_OUTBOUND_IPV6 "UDP_conn_outbound_ipv6"


#endif

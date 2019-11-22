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

typedef struct {
    uint64_t first;
    uint64_t ct;
    uint32_t saddr;
    uint32_t daddr;
    uint16_t dport;
    uint16_t retransmit;
    uint32_t sent;
    uint64_t recv;
    uint8_t protocol;
}netdata_kern_stats_t;

typedef struct  netdata_conn_stats{
    uint64_t first;
    uint64_t ct;
    uint32_t saddr;
    uint32_t daddr;
    uint32_t internal;
    uint16_t dport;
    uint16_t retransmit;
    uint32_t sent;
    uint64_t recv;
    uint8_t protocol;
    uint8_t removeme;

    struct netdata_conn_stats * next;
}netdata_conn_stats_t;

typedef struct {
    avl avl;

    avl_tree_lock bytessent;
    avl_tree_lock destination_port;

    netdata_conn_stats_t *tree;
} netdata_control_connection_t;

typedef struct netdata_network
{
    in_addr_t ipv4addr;
    in_addr_t netmask;
    in_addr_t router;
    int isloopback;

    struct netdata_network *next;
} netdata_network_t;


# define NETDATA_MAX_PROCESSOR 128

#endif

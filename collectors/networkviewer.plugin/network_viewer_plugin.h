#ifndef _NETDATA_NETWORK_VIEWER_H_
# define _NETDATA_NETWORK_VIEWER_H_ 1

# include <linux/perf_event.h>
# include <stdint.h>
# include <errno.h>
# include <signal.h>
# include <stdio.h>
# include <stdint.h>
# include <stdlib.h>
# include <string.h>
# include <unistd.h>
# include <dlfcn.h>

# include <sys/sysinfo.h>

typedef struct
{
    uint64_t pid;
    uint32_t saddr;
    uint32_t daddr;
    uint16_t dport;
    uint16_t retransmit;
    uint32_t sent;
    uint64_t recv;
    uint8_t protocol;
}netdata_conn_stats_t;

# define NETDATA_MAX_PROCESSOR 12
# define NETWORK_VIEWER_MAX_CONNECTIONS 8192

#endif

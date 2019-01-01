// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ALL_H
#define NETDATA_ALL_H 1

#include "../daemon/common.h"

// netdata internal data collection plugins

#include "checks.plugin/plugin_checks.h"
#include "freebsd.plugin/plugin_freebsd.h"
#include "idlejitter.plugin/plugin_idlejitter.h"
#include "cgroups.plugin/sys_fs_cgroup.h"
#include "diskspace.plugin/plugin_diskspace.h"
#include "nfacct.plugin/plugin_nfacct.h"
#include "proc.plugin/plugin_proc.h"
#include "tc.plugin/plugin_tc.h"
#include "macos.plugin/plugin_macos.h"
#include "statsd.plugin/statsd.h"

#include "plugins.d/plugins_d.h"


// ----------------------------------------------------------------------------
// netdata chart priorities

// This is a work in progress - to scope is to collect here all chart priorities.
// These should be based on the CONTEXT of the charts + the chart id when needed
// - for each SECTION +1000 (or +X000 for big sections)
// - for each FAMILY  +100
// - for each CHART   +10

#define NETDATA_CHART_PRIO_SYSTEM_CPU                  100
#define NETDATA_CHART_PRIO_SYSTEM_LOAD                 100
#define NETDATA_CHART_PRIO_SYSTEM_IO                   150
#define NETDATA_CHART_PRIO_SYSTEM_PGPGIO               151
#define NETDATA_CHART_PRIO_SYSTEM_RAM                  200
#define NETDATA_CHART_PRIO_SYSTEM_SWAP                 201
#define NETDATA_CHART_PRIO_SYSTEM_SWAPIO               250
#define NETDATA_CHART_PRIO_SYSTEM_NET                  500
#define NETDATA_CHART_PRIO_SYSTEM_IPV4                 500 // freebsd only
#define NETDATA_CHART_PRIO_SYSTEM_IP                   501
#define NETDATA_CHART_PRIO_SYSTEM_IPV6                 502
#define NETDATA_CHART_PRIO_SYSTEM_PROCESSES            600
#define NETDATA_CHART_PRIO_SYSTEM_FORKS                700
#define NETDATA_CHART_PRIO_SYSTEM_ACTIVE_PROCESSES     750
#define NETDATA_CHART_PRIO_SYSTEM_CTXT                 800
#define NETDATA_CHART_PRIO_SYSTEM_IDLEJITTER           800
#define NETDATA_CHART_PRIO_SYSTEM_INTR                 900
#define NETDATA_CHART_PRIO_SYSTEM_SOFTIRQS             950
#define NETDATA_CHART_PRIO_SYSTEM_SOFTNET_STAT         955
#define NETDATA_CHART_PRIO_SYSTEM_INTERRUPTS          1000
#define NETDATA_CHART_PRIO_SYSTEM_DEV_INTR            1000 // freebsd only
#define NETDATA_CHART_PRIO_SYSTEM_SOFT_INTR           1100 // freebsd only
#define NETDATA_CHART_PRIO_SYSTEM_ENTROPY             1000
#define NETDATA_CHART_PRIO_SYSTEM_UPTIME              1000
#define NETDATA_CHART_PRIO_SYSTEM_IPC_MSQ_QUEUES       990 // freebsd only
#define NETDATA_CHART_PRIO_SYSTEM_IPC_MSQ_MESSAGES    1000
#define NETDATA_CHART_PRIO_SYSTEM_IPC_MSQ_SIZE        1100
#define NETDATA_CHART_PRIO_SYSTEM_IPC_SEMAPHORES      1000
#define NETDATA_CHART_PRIO_SYSTEM_IPC_SEM_ARRAYS      1000
#define NETDATA_CHART_PRIO_SYSTEM_IPC_SHARED_MEM_SEGS 1000 // freebsd only
#define NETDATA_CHART_PRIO_SYSTEM_IPC_SHARED_MEM_SIZE 1000 // freebsd only
#define NETDATA_CHART_PRIO_SYSTEM_PACKETS             7001 // freebsd only


// CPU per core

#define NETDATA_CHART_PRIO_CPU_PER_CORE               1000 // +1 per core
#define NETDATA_CHART_PRIO_CPU_TEMPERATURE            1050 // freebsd only
#define NETDATA_CHART_PRIO_CPUFREQ_SCALING_CUR_FREQ   5003 // freebsd only
#define NETDATA_CHART_PRIO_CPUIDLE                    6000

#define NETDATA_CHART_PRIO_CORE_THROTTLING            5001
#define NETDATA_CHART_PRIO_PACKAGE_THROTTLING         5002

// Interrupts per core

#define NETDATA_CHART_PRIO_INTERRUPTS_PER_CORE        1100 // +1 per core

// Memory Section - 1xxx

#define NETDATA_CHART_PRIO_MEM_SYSTEM_AVAILABLE       1010
#define NETDATA_CHART_PRIO_MEM_SYSTEM_COMMITTED       1020
#define NETDATA_CHART_PRIO_MEM_SYSTEM_PGFAULTS        1030
#define NETDATA_CHART_PRIO_MEM_KERNEL                 1100
#define NETDATA_CHART_PRIO_MEM_SLAB                   1200
#define NETDATA_CHART_PRIO_MEM_HUGEPAGES              1250
#define NETDATA_CHART_PRIO_MEM_KSM                    1300
#define NETDATA_CHART_PRIO_MEM_KSM_SAVINGS            1301
#define NETDATA_CHART_PRIO_MEM_KSM_RATIOS             1302
#define NETDATA_CHART_PRIO_MEM_NUMA                   1400
#define NETDATA_CHART_PRIO_MEM_NUMA_NODES             1410
#define NETDATA_CHART_PRIO_MEM_HW                     1500
#define NETDATA_CHART_PRIO_MEM_HW_ECC_CE              1550
#define NETDATA_CHART_PRIO_MEM_HW_ECC_UE              1560

// Disks

#define NETDATA_CHART_PRIO_DISK_IO                    2000
#define NETDATA_CHART_PRIO_DISK_OPS                   2001
#define NETDATA_CHART_PRIO_DISK_QOPS                  2002
#define NETDATA_CHART_PRIO_DISK_BACKLOG               2003
#define NETDATA_CHART_PRIO_DISK_UTIL                  2004
#define NETDATA_CHART_PRIO_DISK_AWAIT                 2005
#define NETDATA_CHART_PRIO_DISK_AVGSZ                 2006
#define NETDATA_CHART_PRIO_DISK_SVCTM                 2007
#define NETDATA_CHART_PRIO_DISK_MOPS                  2021
#define NETDATA_CHART_PRIO_DISK_IOTIME                2022
#define NETDATA_CHART_PRIO_BCACHE_CACHE_ALLOC         2120
#define NETDATA_CHART_PRIO_BCACHE_HIT_RATIO           2120
#define NETDATA_CHART_PRIO_BCACHE_RATES               2121
#define NETDATA_CHART_PRIO_BCACHE_SIZE                2122
#define NETDATA_CHART_PRIO_BCACHE_USAGE               2123
#define NETDATA_CHART_PRIO_BCACHE_OPS                 2124
#define NETDATA_CHART_PRIO_BCACHE_BYPASS              2125
#define NETDATA_CHART_PRIO_BCACHE_CACHE_READ_RACES    2126

#define NETDATA_CHART_PRIO_DISKSPACE_SPACE            2023
#define NETDATA_CHART_PRIO_DISKSPACE_INODES           2024

// NFS (server)

#define NETDATA_CHART_PRIO_NFSD_READCACHE             2100
#define NETDATA_CHART_PRIO_NFSD_FILEHANDLES           2101
#define NETDATA_CHART_PRIO_NFSD_IO                    2102
#define NETDATA_CHART_PRIO_NFSD_THREADS               2103
#define NETDATA_CHART_PRIO_NFSD_THREADS_FULLCNT       2104
#define NETDATA_CHART_PRIO_NFSD_THREADS_HISTOGRAM     2105
#define NETDATA_CHART_PRIO_NFSD_READAHEAD             2105
#define NETDATA_CHART_PRIO_NFSD_NET                   2107
#define NETDATA_CHART_PRIO_NFSD_RPC                   2108
#define NETDATA_CHART_PRIO_NFSD_PROC2                 2109
#define NETDATA_CHART_PRIO_NFSD_PROC3                 2110
#define NETDATA_CHART_PRIO_NFSD_PROC4                 2111
#define NETDATA_CHART_PRIO_NFSD_PROC4OPS              2112

// NFS (client)

#define NETDATA_CHART_PRIO_NFS_NET                    2207
#define NETDATA_CHART_PRIO_NFS_RPC                    2208
#define NETDATA_CHART_PRIO_NFS_PROC2                  2209
#define NETDATA_CHART_PRIO_NFS_PROC3                  2210
#define NETDATA_CHART_PRIO_NFS_PROC4                  2211

// BTRFS

#define NETDATA_CHART_PRIO_BTRFS_DISK                 2300
#define NETDATA_CHART_PRIO_BTRFS_DATA                 2301
#define NETDATA_CHART_PRIO_BTRFS_METADATA             2302
#define NETDATA_CHART_PRIO_BTRFS_SYSTEM               2303

// ZFS

#define NETDATA_CHART_PRIO_ZFS_ARC_SIZE               2500
#define NETDATA_CHART_PRIO_ZFS_L2_SIZE                2500
#define NETDATA_CHART_PRIO_ZFS_READS                  2510
#define NETDATA_CHART_PRIO_ZFS_ACTUAL_HITS            2519
#define NETDATA_CHART_PRIO_ZFS_ARC_SIZE_BREAKDOWN     2520
#define NETDATA_CHART_PRIO_ZFS_IMPORTANT_OPS          2522
#define NETDATA_CHART_PRIO_ZFS_MEMORY_OPS             2523
#define NETDATA_CHART_PRIO_ZFS_IO                     2700
#define NETDATA_CHART_PRIO_ZFS_HITS                   2520
#define NETDATA_CHART_PRIO_ZFS_DHITS                  2530
#define NETDATA_CHART_PRIO_ZFS_DEMAND_DATA_HITS       2531
#define NETDATA_CHART_PRIO_ZFS_PREFETCH_DATA_HITS     2532
#define NETDATA_CHART_PRIO_ZFS_PHITS                  2540
#define NETDATA_CHART_PRIO_ZFS_MHITS                  2550
#define NETDATA_CHART_PRIO_ZFS_L2HITS                 2560
#define NETDATA_CHART_PRIO_ZFS_LIST_HITS              2600
#define NETDATA_CHART_PRIO_ZFS_HASH_ELEMENTS          2800
#define NETDATA_CHART_PRIO_ZFS_HASH_CHAINS            2810


// SOFTIRQs

#define NETDATA_CHART_PRIO_SOFTIRQS_PER_CORE          3000 // +1 per core

// IPFW (freebsd)

#define NETDATA_CHART_PRIO_IPFW_PACKETS               3001
#define NETDATA_CHART_PRIO_IPFW_BYTES                 3002
#define NETDATA_CHART_PRIO_IPFW_ACTIVE                3003
#define NETDATA_CHART_PRIO_IPFW_EXPIRED               3004
#define NETDATA_CHART_PRIO_IPFW_MEM                   3005


// IPVS

#define NETDATA_CHART_PRIO_IPVS_NET                   3100
#define NETDATA_CHART_PRIO_IPVS_SOCKETS               3101
#define NETDATA_CHART_PRIO_IPVS_PACKETS               3102

// Softnet

#define NETDATA_CHART_PRIO_SOFTNET_PER_CORE           4101 // +1 per core

// IP STACK

#define NETDATA_CHART_PRIO_IP_ERRORS                  4100
#define NETDATA_CHART_PRIO_IP_TCP_CONNABORTS          4210
#define NETDATA_CHART_PRIO_IP_TCP_SYN_QUEUE           4215
#define NETDATA_CHART_PRIO_IP_TCP_ACCEPT_QUEUE        4216
#define NETDATA_CHART_PRIO_IP_TCP_REORDERS            4220
#define NETDATA_CHART_PRIO_IP_TCP_OFO                 4250
#define NETDATA_CHART_PRIO_IP_TCP_SYNCOOKIES          4260
#define NETDATA_CHART_PRIO_IP_TCP_MEM                 4290
#define NETDATA_CHART_PRIO_IP_BCAST                   4500
#define NETDATA_CHART_PRIO_IP_BCAST_PACKETS           4510
#define NETDATA_CHART_PRIO_IP_MCAST                   4600
#define NETDATA_CHART_PRIO_IP_MCAST_PACKETS           4610
#define NETDATA_CHART_PRIO_IP_ECN                     4700

// IPv4

#define NETDATA_CHART_PRIO_IPV4_SOCKETS               5100
#define NETDATA_CHART_PRIO_IPV4_PACKETS               5130
#define NETDATA_CHART_PRIO_IPV4_ERRORS                5150
#define NETDATA_CHART_PRIO_IPV4_ICMP                  5170
#define NETDATA_CHART_PRIO_IPV4_TCP                   5200
#define NETDATA_CHART_PRIO_IPV4_TCP_SOCKETS           5201
#define NETDATA_CHART_PRIO_IPV4_TCP_MEM               5290
#define NETDATA_CHART_PRIO_IPV4_UDP                   5300
#define NETDATA_CHART_PRIO_IPV4_UDP_MEM               5390
#define NETDATA_CHART_PRIO_IPV4_UDPLITE               5400
#define NETDATA_CHART_PRIO_IPV4_RAW                   5450
#define NETDATA_CHART_PRIO_IPV4_FRAGMENTS             5460
#define NETDATA_CHART_PRIO_IPV4_FRAGMENTS_MEM         5470

// IPv6

#define NETDATA_CHART_PRIO_IPV6_PACKETS               6200
#define NETDATA_CHART_PRIO_IPV6_ECT                   6210
#define NETDATA_CHART_PRIO_IPV6_ERRORS                6300
#define NETDATA_CHART_PRIO_IPV6_FRAGMENTS             6400
#define NETDATA_CHART_PRIO_IPV6_FRAGSOUT              6401
#define NETDATA_CHART_PRIO_IPV6_FRAGSIN               6402
#define NETDATA_CHART_PRIO_IPV6_TCP                   6500
#define NETDATA_CHART_PRIO_IPV6_UDP                   6600
#define NETDATA_CHART_PRIO_IPV6_UDP_PACKETS           6601
#define NETDATA_CHART_PRIO_IPV6_UDP_ERRORS            6610
#define NETDATA_CHART_PRIO_IPV6_UDPLITE               6700
#define NETDATA_CHART_PRIO_IPV6_UDPLITE_PACKETS       6701
#define NETDATA_CHART_PRIO_IPV6_UDPLITE_ERRORS        6710
#define NETDATA_CHART_PRIO_IPV6_RAW                   6800
#define NETDATA_CHART_PRIO_IPV6_BCAST                 6840
#define NETDATA_CHART_PRIO_IPV6_MCAST                 6850
#define NETDATA_CHART_PRIO_IPV6_MCAST_PACKETS         6851
#define NETDATA_CHART_PRIO_IPV6_ICMP                  6900
#define NETDATA_CHART_PRIO_IPV6_ICMP_REDIR            6910
#define NETDATA_CHART_PRIO_IPV6_ICMP_ERRORS           6920
#define NETDATA_CHART_PRIO_IPV6_ICMP_ECHOS            6930
#define NETDATA_CHART_PRIO_IPV6_ICMP_GROUPMEMB        6940
#define NETDATA_CHART_PRIO_IPV6_ICMP_ROUTER           6950
#define NETDATA_CHART_PRIO_IPV6_ICMP_NEIGHBOR         6960
#define NETDATA_CHART_PRIO_IPV6_ICMP_LDV2             6970
#define NETDATA_CHART_PRIO_IPV6_ICMP_TYPES            6980


// Network interfaces

#define NETDATA_CHART_PRIO_FIRST_NET_IFACE            7000 // 6 charts per interface
#define NETDATA_CHART_PRIO_FIRST_NET_PACKETS          7001
#define NETDATA_CHART_PRIO_FIRST_NET_ERRORS           7002
#define NETDATA_CHART_PRIO_FIRST_NET_DROPS            7003
#define NETDATA_CHART_PRIO_FIRST_NET_EVENTS           7006
#define NETDATA_CHART_PRIO_CGROUP_NET_IFACE          43000

// SCTP

#define NETDATA_CHART_PRIO_SCTP                       7000

// QoS

#define NETDATA_CHART_PRIO_TC_QOS                     7000
#define NETDATA_CHART_PRIO_TC_QOS_PACKETS             7010
#define NETDATA_CHART_PRIO_TC_QOS_DROPPED             7020
#define NETDATA_CHART_PRIO_TC_QOS_TOCKENS             7030
#define NETDATA_CHART_PRIO_TC_QOS_CTOCKENS            7040


// Netfilter

#define NETDATA_CHART_PRIO_NETFILTER_SOCKETS          8700
#define NETDATA_CHART_PRIO_NETFILTER_NEW              8701
#define NETDATA_CHART_PRIO_NETFILTER_CHANGES          8702
#define NETDATA_CHART_PRIO_NETFILTER_EXPECT           8703
#define NETDATA_CHART_PRIO_NETFILTER_ERRORS           8705
#define NETDATA_CHART_PRIO_NETFILTER_SEARCH           8710

#define NETDATA_CHART_PRIO_NETFILTER_PACKETS          8906
#define NETDATA_CHART_PRIO_NETFILTER_BYTES            8907

// SYNPROXY

#define NETDATA_CHART_PRIO_SYNPROXY_SYN_RECEIVED      8751
#define NETDATA_CHART_PRIO_SYNPROXY_COOKIES           8752
#define NETDATA_CHART_PRIO_SYNPROXY_CONN_OPEN         8753
#define NETDATA_CHART_PRIO_SYNPROXY_ENTRIES           8754

// MDSTAT

#define NETDATA_CHART_PRIO_MDSTAT_HEALTH              9000
#define NETDATA_CHART_PRIO_MDSTAT_NONREDUNDANT        9001
#define NETDATA_CHART_PRIO_MDSTAT_DISKS               9002 // 5 charts per raid
#define NETDATA_CHART_PRIO_MDSTAT_MISMATCH            9003
#define NETDATA_CHART_PRIO_MDSTAT_OPERATION           9004
#define NETDATA_CHART_PRIO_MDSTAT_FINISH              9005
#define NETDATA_CHART_PRIO_MDSTAT_SPEED               9006

// Linux Power Supply
#define NETDATA_CHART_PRIO_POWER_SUPPLY_CAPACITY      9500 // 4 charts per power supply
#define NETDATA_CHART_PRIO_POWER_SUPPLY_CHARGE        9501
#define NETDATA_CHART_PRIO_POWER_SUPPLY_ENERGY        9502
#define NETDATA_CHART_PRIO_POWER_SUPPLY_VOLTAGE       9503

// CGROUPS

#define NETDATA_CHART_PRIO_CGROUPS_SYSTEMD           19000 // many charts
#define NETDATA_CHART_PRIO_CGROUPS_CONTAINERS        40000 // many charts

// STATSD

#define NETDATA_CHART_PRIO_STATSD_PRIVATE            90000 // many charts

// INTERNAL NETDATA INFO

#define NETDATA_CHART_PRIO_CHECKS                    99999

#define NETDATA_CHART_PRIO_NETDATA_DISKSPACE        132020
#define NETDATA_CHART_PRIO_NETDATA_TC_CPU           135000
#define NETDATA_CHART_PRIO_NETDATA_TC_TIME          135001


#endif //NETDATA_ALL_H

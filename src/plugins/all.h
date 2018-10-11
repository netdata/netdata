// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ALL_H
#define NETDATA_ALL_H 1

#include "../common.h"

// netdata internal data collection plugins

#include "checks.plugin/plugin_checks.h"
#include "freebsd.plugin/plugin_freebsd.h"
#include "idlejitter.plugin/plugin_idlejitter.h"
#include "linux-cgroups.plugin/sys_fs_cgroup.h"
#include "linux-diskspace.plugin/plugin_diskspace.h"
#include "linux-nfacct.plugin/plugin_nfacct.h"
#include "linux-proc.plugin/plugin_proc.h"
#include "linux-tc.plugin/plugin_tc.h"
#include "macos.plugin/plugin_macos.h"
#include "plugins.d.plugin/plugins_d.h"
#include "statsd.plugin/statsd.h"


// ----------------------------------------------------------------------------
// netdata chart priorities

// This is a work in progress - to scope is to collect here all chart priorities.
// These should be based on the CONTEXT of the charts + the chart id when needed
// - for each SECTION +1000 (or +X000 for big sections)
// - for each FAMILY  +100
// - for each CHART   +10

#define NETDATA_CHART_PRIO_SYSTEM_IP               501
#define NETDATA_CHART_PRIO_SYSTEM_IPV6             502

// Memory Section - 1xxx
#define NETDATA_CHART_PRIO_MEM_SYSTEM              1000
#define NETDATA_CHART_PRIO_MEM_SYSTEM_AVAILABLE    1010
#define NETDATA_CHART_PRIO_MEM_SYSTEM_COMMITTED    1020
#define NETDATA_CHART_PRIO_MEM_SYSTEM_PGFAULTS     1030
#define NETDATA_CHART_PRIO_MEM_KERNEL              1100
#define NETDATA_CHART_PRIO_MEM_SLAB                1200
#define NETDATA_CHART_PRIO_MEM_HUGEPAGES           1250
#define NETDATA_CHART_PRIO_MEM_KSM                 1300
#define NETDATA_CHART_PRIO_MEM_NUMA                1400
#define NETDATA_CHART_PRIO_MEM_HW                  1500


// IP

#define NETDATA_CHART_PRIO_IP                      4000
#define NETDATA_CHART_PRIO_IP_ERRORS               4100
#define NETDATA_CHART_PRIO_IP_TCP                  4200
#define NETDATA_CHART_PRIO_IP_TCP_MEM              4290
#define NETDATA_CHART_PRIO_IP_BCAST                4500
#define NETDATA_CHART_PRIO_IP_MCAST                4600
#define NETDATA_CHART_PRIO_IP_ECN                  4700


// IPv4

#define NETDATA_CHART_PRIO_IPV4                    5100
#define NETDATA_CHART_PRIO_IPV4_SOCKETS            5100
#define NETDATA_CHART_PRIO_IPV4_PACKETS            5130
#define NETDATA_CHART_PRIO_IPV4_ERRORS             5150
#define NETDATA_CHART_PRIO_IPV4_ICMP               5170
#define NETDATA_CHART_PRIO_IPV4_TCP                5200
#define NETDATA_CHART_PRIO_IPV4_TCP_MEM            5290
#define NETDATA_CHART_PRIO_IPV4_UDP                5300
#define NETDATA_CHART_PRIO_IPV4_UDP_MEM            5390
#define NETDATA_CHART_PRIO_IPV4_UDPLITE            5400
#define NETDATA_CHART_PRIO_IPV4_RAW                5450
#define NETDATA_CHART_PRIO_IPV4_FRAGMENTS          5460
#define NETDATA_CHART_PRIO_IPV4_FRAGMENTS_MEM      5470

// IPv6

#define NETDATA_CHART_PRIO_IPV6                    6200
#define NETDATA_CHART_PRIO_IPV6_PACKETS            6200
#define NETDATA_CHART_PRIO_IPV6_ERRORS             6300
#define NETDATA_CHART_PRIO_IPV6_FRAGMENTS          6400
#define NETDATA_CHART_PRIO_IPV6_TCP                6500
#define NETDATA_CHART_PRIO_IPV6_UDP                6600
#define NETDATA_CHART_PRIO_IPV6_UDP_ERRORS         6610
#define NETDATA_CHART_PRIO_IPV6_UDPLITE            6700
#define NETDATA_CHART_PRIO_IPV6_UDPLITE_ERRORS     6710
#define NETDATA_CHART_PRIO_IPV6_RAW                6800
#define NETDATA_CHART_PRIO_IPV6_BCAST              6840
#define NETDATA_CHART_PRIO_IPV6_MCAST              6850
#define NETDATA_CHART_PRIO_IPV6_ICMP               6900


// SCTP

#define NETDATA_CHART_PRIO_SCTP                    7000


// Netfilter

#define NETDATA_CHART_PRIO_NETFILTER               8700
#define NETDATA_CHART_PRIO_SYNPROXY                8750


#endif //NETDATA_ALL_H

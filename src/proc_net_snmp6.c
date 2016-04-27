#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include "common.h"
#include "log.h"
#include "appconfig.h"
#include "procfile.h"
#include "rrd.h"
#include "plugin_proc.h"

#define RRD_TYPE_NET_SNMP6			"ipv6"
#define RRD_TYPE_NET_SNMP6_LEN		strlen(RRD_TYPE_NET_SNMP6)

int do_proc_net_snmp6(int update_every, unsigned long long dt) {
	static procfile *ff = NULL;
	static int gen_hashes = -1;

	static int do_ip_packets = -1, do_ip_fragsout = -1, do_ip_fragsin = -1, do_ip_errors = -1,
		do_udplite_packets = -1, do_udplite_errors = -1,
		do_udp_packets = -1, do_udp_errors = -1,
		do_bandwidth = -1, do_mcast = -1, do_bcast = -1, do_mcast_p = -1,
		do_icmp = -1, do_icmp_redir = -1, do_icmp_errors = -1, do_icmp_echos = -1, do_icmp_groupmemb = -1,
		do_icmp_router = -1, do_icmp_neighbor = -1, do_icmp_mldv2 = -1, do_icmp_types = -1, do_ect = -1;

	static uint32_t hash_Ip6InReceives = -1;

	static uint32_t hash_Ip6InHdrErrors = -1;
	static uint32_t hash_Ip6InTooBigErrors = -1;
	static uint32_t hash_Ip6InNoRoutes = -1;
	static uint32_t hash_Ip6InAddrErrors = -1;
	static uint32_t hash_Ip6InUnknownProtos = -1;
	static uint32_t hash_Ip6InTruncatedPkts = -1;
	static uint32_t hash_Ip6InDiscards = -1;
	static uint32_t hash_Ip6InDelivers = -1;

	static uint32_t hash_Ip6OutForwDatagrams = -1;
	static uint32_t hash_Ip6OutRequests = -1;
	static uint32_t hash_Ip6OutDiscards = -1;
	static uint32_t hash_Ip6OutNoRoutes = -1;

	static uint32_t hash_Ip6ReasmTimeout = -1;
	static uint32_t hash_Ip6ReasmReqds = -1;
	static uint32_t hash_Ip6ReasmOKs = -1;
	static uint32_t hash_Ip6ReasmFails = -1;

	static uint32_t hash_Ip6FragOKs = -1;
	static uint32_t hash_Ip6FragFails = -1;
	static uint32_t hash_Ip6FragCreates = -1;

	static uint32_t hash_Ip6InMcastPkts = -1;
	static uint32_t hash_Ip6OutMcastPkts = -1;

	static uint32_t hash_Ip6InOctets = -1;
	static uint32_t hash_Ip6OutOctets = -1;

	static uint32_t hash_Ip6InMcastOctets = -1;
	static uint32_t hash_Ip6OutMcastOctets = -1;
	static uint32_t hash_Ip6InBcastOctets = -1;
	static uint32_t hash_Ip6OutBcastOctets = -1;

	static uint32_t hash_Ip6InNoECTPkts = -1;
	static uint32_t hash_Ip6InECT1Pkts = -1;
	static uint32_t hash_Ip6InECT0Pkts = -1;
	static uint32_t hash_Ip6InCEPkts = -1;

	static uint32_t hash_Icmp6InMsgs = -1;
	static uint32_t hash_Icmp6InErrors = -1;
	static uint32_t hash_Icmp6OutMsgs = -1;
	static uint32_t hash_Icmp6OutErrors = -1;
	static uint32_t hash_Icmp6InCsumErrors = -1;
	static uint32_t hash_Icmp6InDestUnreachs = -1;
	static uint32_t hash_Icmp6InPktTooBigs = -1;
	static uint32_t hash_Icmp6InTimeExcds = -1;
	static uint32_t hash_Icmp6InParmProblems = -1;
	static uint32_t hash_Icmp6InEchos = -1;
	static uint32_t hash_Icmp6InEchoReplies = -1;
	static uint32_t hash_Icmp6InGroupMembQueries = -1;
	static uint32_t hash_Icmp6InGroupMembResponses = -1;
	static uint32_t hash_Icmp6InGroupMembReductions = -1;
	static uint32_t hash_Icmp6InRouterSolicits = -1;
	static uint32_t hash_Icmp6InRouterAdvertisements = -1;
	static uint32_t hash_Icmp6InNeighborSolicits = -1;
	static uint32_t hash_Icmp6InNeighborAdvertisements = -1;
	static uint32_t hash_Icmp6InRedirects = -1;
	static uint32_t hash_Icmp6InMLDv2Reports = -1;
	static uint32_t hash_Icmp6OutDestUnreachs = -1;
	static uint32_t hash_Icmp6OutPktTooBigs = -1;
	static uint32_t hash_Icmp6OutTimeExcds = -1;
	static uint32_t hash_Icmp6OutParmProblems = -1;
	static uint32_t hash_Icmp6OutEchos = -1;
	static uint32_t hash_Icmp6OutEchoReplies = -1;
	static uint32_t hash_Icmp6OutGroupMembQueries = -1;
	static uint32_t hash_Icmp6OutGroupMembResponses = -1;
	static uint32_t hash_Icmp6OutGroupMembReductions = -1;
	static uint32_t hash_Icmp6OutRouterSolicits = -1;
	static uint32_t hash_Icmp6OutRouterAdvertisements = -1;
	static uint32_t hash_Icmp6OutNeighborSolicits = -1;
	static uint32_t hash_Icmp6OutNeighborAdvertisements = -1;
	static uint32_t hash_Icmp6OutRedirects = -1;
	static uint32_t hash_Icmp6OutMLDv2Reports = -1;
	static uint32_t hash_Icmp6InType1 = -1;
	static uint32_t hash_Icmp6InType128 = -1;
	static uint32_t hash_Icmp6InType129 = -1;
	static uint32_t hash_Icmp6InType136 = -1;
	static uint32_t hash_Icmp6OutType1 = -1;
	static uint32_t hash_Icmp6OutType128 = -1;
	static uint32_t hash_Icmp6OutType129 = -1;
	static uint32_t hash_Icmp6OutType133 = -1;
	static uint32_t hash_Icmp6OutType135 = -1;
	static uint32_t hash_Icmp6OutType143 = -1;

	static uint32_t hash_Udp6InDatagrams = -1;
	static uint32_t hash_Udp6NoPorts = -1;
	static uint32_t hash_Udp6InErrors = -1;
	static uint32_t hash_Udp6OutDatagrams = -1;
	static uint32_t hash_Udp6RcvbufErrors = -1;
	static uint32_t hash_Udp6SndbufErrors = -1;
	static uint32_t hash_Udp6InCsumErrors = -1;
	static uint32_t hash_Udp6IgnoredMulti = -1;

	static uint32_t hash_UdpLite6InDatagrams = -1;
	static uint32_t hash_UdpLite6NoPorts = -1;
	static uint32_t hash_UdpLite6InErrors = -1;
	static uint32_t hash_UdpLite6OutDatagrams = -1;
	static uint32_t hash_UdpLite6RcvbufErrors = -1;
	static uint32_t hash_UdpLite6SndbufErrors = -1;
	static uint32_t hash_UdpLite6InCsumErrors = -1;

	if(gen_hashes != 1) {
		gen_hashes = 1;
		hash_Ip6InReceives = simple_hash("Ip6InReceives");
		hash_Ip6InHdrErrors = simple_hash("Ip6InHdrErrors");
		hash_Ip6InTooBigErrors = simple_hash("Ip6InTooBigErrors");
		hash_Ip6InNoRoutes = simple_hash("Ip6InNoRoutes");
		hash_Ip6InAddrErrors = simple_hash("Ip6InAddrErrors");
		hash_Ip6InUnknownProtos = simple_hash("Ip6InUnknownProtos");
		hash_Ip6InTruncatedPkts = simple_hash("Ip6InTruncatedPkts");
		hash_Ip6InDiscards = simple_hash("Ip6InDiscards");
		hash_Ip6InDelivers = simple_hash("Ip6InDelivers");
		hash_Ip6OutForwDatagrams = simple_hash("Ip6OutForwDatagrams");
		hash_Ip6OutRequests = simple_hash("Ip6OutRequests");
		hash_Ip6OutDiscards = simple_hash("Ip6OutDiscards");
		hash_Ip6OutNoRoutes = simple_hash("Ip6OutNoRoutes");
		hash_Ip6ReasmTimeout = simple_hash("Ip6ReasmTimeout");
		hash_Ip6ReasmReqds = simple_hash("Ip6ReasmReqds");
		hash_Ip6ReasmOKs = simple_hash("Ip6ReasmOKs");
		hash_Ip6ReasmFails = simple_hash("Ip6ReasmFails");
		hash_Ip6FragOKs = simple_hash("Ip6FragOKs");
		hash_Ip6FragFails = simple_hash("Ip6FragFails");
		hash_Ip6FragCreates = simple_hash("Ip6FragCreates");
		hash_Ip6InMcastPkts = simple_hash("Ip6InMcastPkts");
		hash_Ip6OutMcastPkts = simple_hash("Ip6OutMcastPkts");
		hash_Ip6InOctets = simple_hash("Ip6InOctets");
		hash_Ip6OutOctets = simple_hash("Ip6OutOctets");
		hash_Ip6InMcastOctets = simple_hash("Ip6InMcastOctets");
		hash_Ip6OutMcastOctets = simple_hash("Ip6OutMcastOctets");
		hash_Ip6InBcastOctets = simple_hash("Ip6InBcastOctets");
		hash_Ip6OutBcastOctets = simple_hash("Ip6OutBcastOctets");
		hash_Ip6InNoECTPkts = simple_hash("Ip6InNoECTPkts");
		hash_Ip6InECT1Pkts = simple_hash("Ip6InECT1Pkts");
		hash_Ip6InECT0Pkts = simple_hash("Ip6InECT0Pkts");
		hash_Ip6InCEPkts = simple_hash("Ip6InCEPkts");
		hash_Icmp6InMsgs = simple_hash("Icmp6InMsgs");
		hash_Icmp6InErrors = simple_hash("Icmp6InErrors");
		hash_Icmp6OutMsgs = simple_hash("Icmp6OutMsgs");
		hash_Icmp6OutErrors = simple_hash("Icmp6OutErrors");
		hash_Icmp6InCsumErrors = simple_hash("Icmp6InCsumErrors");
		hash_Icmp6InDestUnreachs = simple_hash("Icmp6InDestUnreachs");
		hash_Icmp6InPktTooBigs = simple_hash("Icmp6InPktTooBigs");
		hash_Icmp6InTimeExcds = simple_hash("Icmp6InTimeExcds");
		hash_Icmp6InParmProblems = simple_hash("Icmp6InParmProblems");
		hash_Icmp6InEchos = simple_hash("Icmp6InEchos");
		hash_Icmp6InEchoReplies = simple_hash("Icmp6InEchoReplies");
		hash_Icmp6InGroupMembQueries = simple_hash("Icmp6InGroupMembQueries");
		hash_Icmp6InGroupMembResponses = simple_hash("Icmp6InGroupMembResponses");
		hash_Icmp6InGroupMembReductions = simple_hash("Icmp6InGroupMembReductions");
		hash_Icmp6InRouterSolicits = simple_hash("Icmp6InRouterSolicits");
		hash_Icmp6InRouterAdvertisements = simple_hash("Icmp6InRouterAdvertisements");
		hash_Icmp6InNeighborSolicits = simple_hash("Icmp6InNeighborSolicits");
		hash_Icmp6InNeighborAdvertisements = simple_hash("Icmp6InNeighborAdvertisements");
		hash_Icmp6InRedirects = simple_hash("Icmp6InRedirects");
		hash_Icmp6InMLDv2Reports = simple_hash("Icmp6InMLDv2Reports");
		hash_Icmp6OutDestUnreachs = simple_hash("Icmp6OutDestUnreachs");
		hash_Icmp6OutPktTooBigs = simple_hash("Icmp6OutPktTooBigs");
		hash_Icmp6OutTimeExcds = simple_hash("Icmp6OutTimeExcds");
		hash_Icmp6OutParmProblems = simple_hash("Icmp6OutParmProblems");
		hash_Icmp6OutEchos = simple_hash("Icmp6OutEchos");
		hash_Icmp6OutEchoReplies = simple_hash("Icmp6OutEchoReplies");
		hash_Icmp6OutGroupMembQueries = simple_hash("Icmp6OutGroupMembQueries");
		hash_Icmp6OutGroupMembResponses = simple_hash("Icmp6OutGroupMembResponses");
		hash_Icmp6OutGroupMembReductions = simple_hash("Icmp6OutGroupMembReductions");
		hash_Icmp6OutRouterSolicits = simple_hash("Icmp6OutRouterSolicits");
		hash_Icmp6OutRouterAdvertisements = simple_hash("Icmp6OutRouterAdvertisements");
		hash_Icmp6OutNeighborSolicits = simple_hash("Icmp6OutNeighborSolicits");
		hash_Icmp6OutNeighborAdvertisements = simple_hash("Icmp6OutNeighborAdvertisements");
		hash_Icmp6OutRedirects = simple_hash("Icmp6OutRedirects");
		hash_Icmp6OutMLDv2Reports = simple_hash("Icmp6OutMLDv2Reports");
		hash_Icmp6InType1 = simple_hash("Icmp6InType1");
		hash_Icmp6InType128 = simple_hash("Icmp6InType128");
		hash_Icmp6InType129 = simple_hash("Icmp6InType129");
		hash_Icmp6InType136 = simple_hash("Icmp6InType136");
		hash_Icmp6OutType1 = simple_hash("Icmp6OutType1");
		hash_Icmp6OutType128 = simple_hash("Icmp6OutType128");
		hash_Icmp6OutType129 = simple_hash("Icmp6OutType129");
		hash_Icmp6OutType133 = simple_hash("Icmp6OutType133");
		hash_Icmp6OutType135 = simple_hash("Icmp6OutType135");
		hash_Icmp6OutType143 = simple_hash("Icmp6OutType143");
		hash_Udp6InDatagrams = simple_hash("Udp6InDatagrams");
		hash_Udp6NoPorts = simple_hash("Udp6NoPorts");
		hash_Udp6InErrors = simple_hash("Udp6InErrors");
		hash_Udp6OutDatagrams = simple_hash("Udp6OutDatagrams");
		hash_Udp6RcvbufErrors = simple_hash("Udp6RcvbufErrors");
		hash_Udp6SndbufErrors = simple_hash("Udp6SndbufErrors");
		hash_Udp6InCsumErrors = simple_hash("Udp6InCsumErrors");
		hash_Udp6IgnoredMulti = simple_hash("Udp6IgnoredMulti");
		hash_UdpLite6InDatagrams = simple_hash("UdpLite6InDatagrams");
		hash_UdpLite6NoPorts = simple_hash("UdpLite6NoPorts");
		hash_UdpLite6InErrors = simple_hash("UdpLite6InErrors");
		hash_UdpLite6OutDatagrams = simple_hash("UdpLite6OutDatagrams");
		hash_UdpLite6RcvbufErrors = simple_hash("UdpLite6RcvbufErrors");
		hash_UdpLite6SndbufErrors = simple_hash("UdpLite6SndbufErrors");
		hash_UdpLite6InCsumErrors = simple_hash("UdpLite6InCsumErrors");
	}

	if(do_ip_packets == -1)			do_ip_packets 		= config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "ipv6 packets", CONFIG_ONDEMAND_ONDEMAND);
	if(do_ip_fragsout == -1)		do_ip_fragsout 		= config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "ipv6 fragments sent", CONFIG_ONDEMAND_ONDEMAND);
	if(do_ip_fragsin == -1)			do_ip_fragsin 		= config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "ipv6 fragments assembly", CONFIG_ONDEMAND_ONDEMAND);
	if(do_ip_errors == -1)			do_ip_errors 		= config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "ipv6 errors", CONFIG_ONDEMAND_ONDEMAND);
	if(do_udp_packets == -1)		do_udp_packets 		= config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "ipv6 UDP packets", CONFIG_ONDEMAND_ONDEMAND);
	if(do_udp_errors == -1)			do_udp_errors 		= config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "ipv6 UDP errors", CONFIG_ONDEMAND_ONDEMAND);
	if(do_udplite_packets == -1)	do_udplite_packets	= config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "ipv6 UDPlite packets", CONFIG_ONDEMAND_ONDEMAND);
	if(do_udplite_errors == -1)		do_udplite_errors	= config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "ipv6 UDPlite errors", CONFIG_ONDEMAND_ONDEMAND);
	if(do_bandwidth == -1)			do_bandwidth		= config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "bandwidth", CONFIG_ONDEMAND_ONDEMAND);
	if(do_mcast == -1)				do_mcast 			= config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "multicast bandwidth", CONFIG_ONDEMAND_ONDEMAND);
	if(do_bcast == -1)				do_bcast 			= config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "broadcast bandwidth", CONFIG_ONDEMAND_ONDEMAND);
	if(do_mcast_p == -1)			do_mcast_p 			= config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "multicast packets", CONFIG_ONDEMAND_ONDEMAND);
	if(do_icmp == -1)				do_icmp 			= config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "icmp", CONFIG_ONDEMAND_ONDEMAND);
	if(do_icmp_redir == -1)			do_icmp_redir 		= config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "icmp redirects", CONFIG_ONDEMAND_ONDEMAND);
	if(do_icmp_errors == -1)		do_icmp_errors 		= config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "icmp errors", CONFIG_ONDEMAND_ONDEMAND);
	if(do_icmp_echos == -1)			do_icmp_echos 		= config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "icmp echos", CONFIG_ONDEMAND_ONDEMAND);
	if(do_icmp_groupmemb == -1)		do_icmp_groupmemb 	= config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "icmp group membership", CONFIG_ONDEMAND_ONDEMAND);
	if(do_icmp_router == -1)		do_icmp_router 		= config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "icmp router", CONFIG_ONDEMAND_ONDEMAND);
	if(do_icmp_neighbor == -1)		do_icmp_neighbor 	= config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "icmp neighbor", CONFIG_ONDEMAND_ONDEMAND);
	if(do_icmp_mldv2 == -1)			do_icmp_mldv2 		= config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "icmp mldv2", CONFIG_ONDEMAND_ONDEMAND);
	if(do_icmp_types == -1)			do_icmp_types 		= config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "icmp types", CONFIG_ONDEMAND_ONDEMAND);
	if(do_ect == -1)				do_ect 				= config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "ect", CONFIG_ONDEMAND_ONDEMAND);

	if(dt) {};

	if(!ff) {
		char filename[FILENAME_MAX + 1];
		snprintfz(filename, FILENAME_MAX, "%s%s", global_host_prefix, "/proc/net/snmp6");
		ff = procfile_open(config_get("plugin:proc:/proc/net/snmp6", "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
	}
	if(!ff) return 1;

	ff = procfile_readall(ff);
	if(!ff) return 0; // we return 0, so that we will retry to open it next time

	uint32_t lines = procfile_lines(ff), l;
	uint32_t words;

	unsigned long long Ip6InReceives = 0ULL;
	unsigned long long Ip6InHdrErrors = 0ULL;
	unsigned long long Ip6InTooBigErrors = 0ULL;
	unsigned long long Ip6InNoRoutes = 0ULL;
	unsigned long long Ip6InAddrErrors = 0ULL;
	unsigned long long Ip6InUnknownProtos = 0ULL;
	unsigned long long Ip6InTruncatedPkts = 0ULL;
	unsigned long long Ip6InDiscards = 0ULL;
	unsigned long long Ip6InDelivers = 0ULL;
	unsigned long long Ip6OutForwDatagrams = 0ULL;
	unsigned long long Ip6OutRequests = 0ULL;
	unsigned long long Ip6OutDiscards = 0ULL;
	unsigned long long Ip6OutNoRoutes = 0ULL;
	unsigned long long Ip6ReasmTimeout = 0ULL;
	unsigned long long Ip6ReasmReqds = 0ULL;
	unsigned long long Ip6ReasmOKs = 0ULL;
	unsigned long long Ip6ReasmFails = 0ULL;
	unsigned long long Ip6FragOKs = 0ULL;
	unsigned long long Ip6FragFails = 0ULL;
	unsigned long long Ip6FragCreates = 0ULL;
	unsigned long long Ip6InMcastPkts = 0ULL;
	unsigned long long Ip6OutMcastPkts = 0ULL;
	unsigned long long Ip6InOctets = 0ULL;
	unsigned long long Ip6OutOctets = 0ULL;
	unsigned long long Ip6InMcastOctets = 0ULL;
	unsigned long long Ip6OutMcastOctets = 0ULL;
	unsigned long long Ip6InBcastOctets = 0ULL;
	unsigned long long Ip6OutBcastOctets = 0ULL;
	unsigned long long Ip6InNoECTPkts = 0ULL;
	unsigned long long Ip6InECT1Pkts = 0ULL;
	unsigned long long Ip6InECT0Pkts = 0ULL;
	unsigned long long Ip6InCEPkts = 0ULL;
	unsigned long long Icmp6InMsgs = 0ULL;
	unsigned long long Icmp6InErrors = 0ULL;
	unsigned long long Icmp6OutMsgs = 0ULL;
	unsigned long long Icmp6OutErrors = 0ULL;
	unsigned long long Icmp6InCsumErrors = 0ULL;
	unsigned long long Icmp6InDestUnreachs = 0ULL;
	unsigned long long Icmp6InPktTooBigs = 0ULL;
	unsigned long long Icmp6InTimeExcds = 0ULL;
	unsigned long long Icmp6InParmProblems = 0ULL;
	unsigned long long Icmp6InEchos = 0ULL;
	unsigned long long Icmp6InEchoReplies = 0ULL;
	unsigned long long Icmp6InGroupMembQueries = 0ULL;
	unsigned long long Icmp6InGroupMembResponses = 0ULL;
	unsigned long long Icmp6InGroupMembReductions = 0ULL;
	unsigned long long Icmp6InRouterSolicits = 0ULL;
	unsigned long long Icmp6InRouterAdvertisements = 0ULL;
	unsigned long long Icmp6InNeighborSolicits = 0ULL;
	unsigned long long Icmp6InNeighborAdvertisements = 0ULL;
	unsigned long long Icmp6InRedirects = 0ULL;
	unsigned long long Icmp6InMLDv2Reports = 0ULL;
	unsigned long long Icmp6OutDestUnreachs = 0ULL;
	unsigned long long Icmp6OutPktTooBigs = 0ULL;
	unsigned long long Icmp6OutTimeExcds = 0ULL;
	unsigned long long Icmp6OutParmProblems = 0ULL;
	unsigned long long Icmp6OutEchos = 0ULL;
	unsigned long long Icmp6OutEchoReplies = 0ULL;
	unsigned long long Icmp6OutGroupMembQueries = 0ULL;
	unsigned long long Icmp6OutGroupMembResponses = 0ULL;
	unsigned long long Icmp6OutGroupMembReductions = 0ULL;
	unsigned long long Icmp6OutRouterSolicits = 0ULL;
	unsigned long long Icmp6OutRouterAdvertisements = 0ULL;
	unsigned long long Icmp6OutNeighborSolicits = 0ULL;
	unsigned long long Icmp6OutNeighborAdvertisements = 0ULL;
	unsigned long long Icmp6OutRedirects = 0ULL;
	unsigned long long Icmp6OutMLDv2Reports = 0ULL;
	unsigned long long Icmp6InType1 = 0ULL;
	unsigned long long Icmp6InType128 = 0ULL;
	unsigned long long Icmp6InType129 = 0ULL;
	unsigned long long Icmp6InType136 = 0ULL;
	unsigned long long Icmp6OutType1 = 0ULL;
	unsigned long long Icmp6OutType128 = 0ULL;
	unsigned long long Icmp6OutType129 = 0ULL;
	unsigned long long Icmp6OutType133 = 0ULL;
	unsigned long long Icmp6OutType135 = 0ULL;
	unsigned long long Icmp6OutType143 = 0ULL;
	unsigned long long Udp6InDatagrams = 0ULL;
	unsigned long long Udp6NoPorts = 0ULL;
	unsigned long long Udp6InErrors = 0ULL;
	unsigned long long Udp6OutDatagrams = 0ULL;
	unsigned long long Udp6RcvbufErrors = 0ULL;
	unsigned long long Udp6SndbufErrors = 0ULL;
	unsigned long long Udp6InCsumErrors = 0ULL;
	unsigned long long Udp6IgnoredMulti = 0ULL;
	unsigned long long UdpLite6InDatagrams = 0ULL;
	unsigned long long UdpLite6NoPorts = 0ULL;
	unsigned long long UdpLite6InErrors = 0ULL;
	unsigned long long UdpLite6OutDatagrams = 0ULL;
	unsigned long long UdpLite6RcvbufErrors = 0ULL;
	unsigned long long UdpLite6SndbufErrors = 0ULL;
	unsigned long long UdpLite6InCsumErrors = 0ULL;

	for(l = 0; l < lines ;l++) {
		words = procfile_linewords(ff, l);
		if(words < 2) {
			if(words) error("Cannot read /proc/net/snmp6 line %d. Expected 2 params, read %d.", l, words);
			continue;
		}

		char *name = procfile_lineword(ff, l, 0);
		char * value = procfile_lineword(ff, l, 1);
		if(!name || !*name || !value || !*value) continue;

		uint32_t hash = simple_hash(name);

		if(0) ;
		else if(hash == hash_Ip6InReceives && strcmp(name, "Ip6InReceives") == 0) Ip6InReceives = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6InHdrErrors && strcmp(name, "Ip6InHdrErrors") == 0) Ip6InHdrErrors = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6InTooBigErrors && strcmp(name, "Ip6InTooBigErrors") == 0) Ip6InTooBigErrors = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6InNoRoutes && strcmp(name, "Ip6InNoRoutes") == 0) Ip6InNoRoutes = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6InAddrErrors && strcmp(name, "Ip6InAddrErrors") == 0) Ip6InAddrErrors = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6InUnknownProtos && strcmp(name, "Ip6InUnknownProtos") == 0) Ip6InUnknownProtos = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6InTruncatedPkts && strcmp(name, "Ip6InTruncatedPkts") == 0) Ip6InTruncatedPkts = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6InDiscards && strcmp(name, "Ip6InDiscards") == 0) Ip6InDiscards = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6InDelivers && strcmp(name, "Ip6InDelivers") == 0) Ip6InDelivers = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6OutForwDatagrams && strcmp(name, "Ip6OutForwDatagrams") == 0) Ip6OutForwDatagrams = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6OutRequests && strcmp(name, "Ip6OutRequests") == 0) Ip6OutRequests = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6OutDiscards && strcmp(name, "Ip6OutDiscards") == 0) Ip6OutDiscards = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6OutNoRoutes && strcmp(name, "Ip6OutNoRoutes") == 0) Ip6OutNoRoutes = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6ReasmTimeout && strcmp(name, "Ip6ReasmTimeout") == 0) Ip6ReasmTimeout = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6ReasmReqds && strcmp(name, "Ip6ReasmReqds") == 0) Ip6ReasmReqds = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6ReasmOKs && strcmp(name, "Ip6ReasmOKs") == 0) Ip6ReasmOKs = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6ReasmFails && strcmp(name, "Ip6ReasmFails") == 0) Ip6ReasmFails = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6FragOKs && strcmp(name, "Ip6FragOKs") == 0) Ip6FragOKs = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6FragFails && strcmp(name, "Ip6FragFails") == 0) Ip6FragFails = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6FragCreates && strcmp(name, "Ip6FragCreates") == 0) Ip6FragCreates = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6InMcastPkts && strcmp(name, "Ip6InMcastPkts") == 0) Ip6InMcastPkts = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6OutMcastPkts && strcmp(name, "Ip6OutMcastPkts") == 0) Ip6OutMcastPkts = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6InOctets && strcmp(name, "Ip6InOctets") == 0) Ip6InOctets = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6OutOctets && strcmp(name, "Ip6OutOctets") == 0) Ip6OutOctets = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6InMcastOctets && strcmp(name, "Ip6InMcastOctets") == 0) Ip6InMcastOctets = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6OutMcastOctets && strcmp(name, "Ip6OutMcastOctets") == 0) Ip6OutMcastOctets = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6InBcastOctets && strcmp(name, "Ip6InBcastOctets") == 0) Ip6InBcastOctets = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6OutBcastOctets && strcmp(name, "Ip6OutBcastOctets") == 0) Ip6OutBcastOctets = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6InNoECTPkts && strcmp(name, "Ip6InNoECTPkts") == 0) Ip6InNoECTPkts = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6InECT1Pkts && strcmp(name, "Ip6InECT1Pkts") == 0) Ip6InECT1Pkts = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6InECT0Pkts && strcmp(name, "Ip6InECT0Pkts") == 0) Ip6InECT0Pkts = strtoull(value, NULL, 10);
		else if(hash == hash_Ip6InCEPkts && strcmp(name, "Ip6InCEPkts") == 0) Ip6InCEPkts = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6InMsgs && strcmp(name, "Icmp6InMsgs") == 0) Icmp6InMsgs = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6InErrors && strcmp(name, "Icmp6InErrors") == 0) Icmp6InErrors = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6OutMsgs && strcmp(name, "Icmp6OutMsgs") == 0) Icmp6OutMsgs = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6OutErrors && strcmp(name, "Icmp6OutErrors") == 0) Icmp6OutErrors = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6InCsumErrors && strcmp(name, "Icmp6InCsumErrors") == 0) Icmp6InCsumErrors = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6InDestUnreachs && strcmp(name, "Icmp6InDestUnreachs") == 0) Icmp6InDestUnreachs = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6InPktTooBigs && strcmp(name, "Icmp6InPktTooBigs") == 0) Icmp6InPktTooBigs = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6InTimeExcds && strcmp(name, "Icmp6InTimeExcds") == 0) Icmp6InTimeExcds = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6InParmProblems && strcmp(name, "Icmp6InParmProblems") == 0) Icmp6InParmProblems = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6InEchos && strcmp(name, "Icmp6InEchos") == 0) Icmp6InEchos = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6InEchoReplies && strcmp(name, "Icmp6InEchoReplies") == 0) Icmp6InEchoReplies = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6InGroupMembQueries && strcmp(name, "Icmp6InGroupMembQueries") == 0) Icmp6InGroupMembQueries = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6InGroupMembResponses && strcmp(name, "Icmp6InGroupMembResponses") == 0) Icmp6InGroupMembResponses = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6InGroupMembReductions && strcmp(name, "Icmp6InGroupMembReductions") == 0) Icmp6InGroupMembReductions = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6InRouterSolicits && strcmp(name, "Icmp6InRouterSolicits") == 0) Icmp6InRouterSolicits = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6InRouterAdvertisements && strcmp(name, "Icmp6InRouterAdvertisements") == 0) Icmp6InRouterAdvertisements = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6InNeighborSolicits && strcmp(name, "Icmp6InNeighborSolicits") == 0) Icmp6InNeighborSolicits = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6InNeighborAdvertisements && strcmp(name, "Icmp6InNeighborAdvertisements") == 0) Icmp6InNeighborAdvertisements = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6InRedirects && strcmp(name, "Icmp6InRedirects") == 0) Icmp6InRedirects = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6InMLDv2Reports && strcmp(name, "Icmp6InMLDv2Reports") == 0) Icmp6InMLDv2Reports = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6OutDestUnreachs && strcmp(name, "Icmp6OutDestUnreachs") == 0) Icmp6OutDestUnreachs = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6OutPktTooBigs && strcmp(name, "Icmp6OutPktTooBigs") == 0) Icmp6OutPktTooBigs = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6OutTimeExcds && strcmp(name, "Icmp6OutTimeExcds") == 0) Icmp6OutTimeExcds = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6OutParmProblems && strcmp(name, "Icmp6OutParmProblems") == 0) Icmp6OutParmProblems = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6OutEchos && strcmp(name, "Icmp6OutEchos") == 0) Icmp6OutEchos = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6OutEchoReplies && strcmp(name, "Icmp6OutEchoReplies") == 0) Icmp6OutEchoReplies = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6OutGroupMembQueries && strcmp(name, "Icmp6OutGroupMembQueries") == 0) Icmp6OutGroupMembQueries = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6OutGroupMembResponses && strcmp(name, "Icmp6OutGroupMembResponses") == 0) Icmp6OutGroupMembResponses = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6OutGroupMembReductions && strcmp(name, "Icmp6OutGroupMembReductions") == 0) Icmp6OutGroupMembReductions = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6OutRouterSolicits && strcmp(name, "Icmp6OutRouterSolicits") == 0) Icmp6OutRouterSolicits = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6OutRouterAdvertisements && strcmp(name, "Icmp6OutRouterAdvertisements") == 0) Icmp6OutRouterAdvertisements = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6OutNeighborSolicits && strcmp(name, "Icmp6OutNeighborSolicits") == 0) Icmp6OutNeighborSolicits = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6OutNeighborAdvertisements && strcmp(name, "Icmp6OutNeighborAdvertisements") == 0) Icmp6OutNeighborAdvertisements = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6OutRedirects && strcmp(name, "Icmp6OutRedirects") == 0) Icmp6OutRedirects = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6OutMLDv2Reports && strcmp(name, "Icmp6OutMLDv2Reports") == 0) Icmp6OutMLDv2Reports = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6InType1 && strcmp(name, "Icmp6InType1") == 0) Icmp6InType1 = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6InType128 && strcmp(name, "Icmp6InType128") == 0) Icmp6InType128 = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6InType129 && strcmp(name, "Icmp6InType129") == 0) Icmp6InType129 = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6InType136 && strcmp(name, "Icmp6InType136") == 0) Icmp6InType136 = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6OutType1 && strcmp(name, "Icmp6OutType1") == 0) Icmp6OutType1 = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6OutType128 && strcmp(name, "Icmp6OutType128") == 0) Icmp6OutType128 = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6OutType129 && strcmp(name, "Icmp6OutType129") == 0) Icmp6OutType129 = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6OutType133 && strcmp(name, "Icmp6OutType133") == 0) Icmp6OutType133 = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6OutType135 && strcmp(name, "Icmp6OutType135") == 0) Icmp6OutType135 = strtoull(value, NULL, 10);
		else if(hash == hash_Icmp6OutType143 && strcmp(name, "Icmp6OutType143") == 0) Icmp6OutType143 = strtoull(value, NULL, 10);
		else if(hash == hash_Udp6InDatagrams && strcmp(name, "Udp6InDatagrams") == 0) Udp6InDatagrams = strtoull(value, NULL, 10);
		else if(hash == hash_Udp6NoPorts && strcmp(name, "Udp6NoPorts") == 0) Udp6NoPorts = strtoull(value, NULL, 10);
		else if(hash == hash_Udp6InErrors && strcmp(name, "Udp6InErrors") == 0) Udp6InErrors = strtoull(value, NULL, 10);
		else if(hash == hash_Udp6OutDatagrams && strcmp(name, "Udp6OutDatagrams") == 0) Udp6OutDatagrams = strtoull(value, NULL, 10);
		else if(hash == hash_Udp6RcvbufErrors && strcmp(name, "Udp6RcvbufErrors") == 0) Udp6RcvbufErrors = strtoull(value, NULL, 10);
		else if(hash == hash_Udp6SndbufErrors && strcmp(name, "Udp6SndbufErrors") == 0) Udp6SndbufErrors = strtoull(value, NULL, 10);
		else if(hash == hash_Udp6InCsumErrors && strcmp(name, "Udp6InCsumErrors") == 0) Udp6InCsumErrors = strtoull(value, NULL, 10);
		else if(hash == hash_Udp6IgnoredMulti && strcmp(name, "Udp6IgnoredMulti") == 0) Udp6IgnoredMulti = strtoull(value, NULL, 10);
		else if(hash == hash_UdpLite6InDatagrams && strcmp(name, "UdpLite6InDatagrams") == 0) UdpLite6InDatagrams = strtoull(value, NULL, 10);
		else if(hash == hash_UdpLite6NoPorts && strcmp(name, "UdpLite6NoPorts") == 0) UdpLite6NoPorts = strtoull(value, NULL, 10);
		else if(hash == hash_UdpLite6InErrors && strcmp(name, "UdpLite6InErrors") == 0) UdpLite6InErrors = strtoull(value, NULL, 10);
		else if(hash == hash_UdpLite6OutDatagrams && strcmp(name, "UdpLite6OutDatagrams") == 0) UdpLite6OutDatagrams = strtoull(value, NULL, 10);
		else if(hash == hash_UdpLite6RcvbufErrors && strcmp(name, "UdpLite6RcvbufErrors") == 0) UdpLite6RcvbufErrors = strtoull(value, NULL, 10);
		else if(hash == hash_UdpLite6SndbufErrors && strcmp(name, "UdpLite6SndbufErrors") == 0) UdpLite6SndbufErrors = strtoull(value, NULL, 10);
		else if(hash == hash_UdpLite6InCsumErrors && strcmp(name, "UdpLite6InCsumErrors") == 0) UdpLite6InCsumErrors = strtoull(value, NULL, 10);
	}

	RRDSET *st;

	// --------------------------------------------------------------------

	if(do_bandwidth == CONFIG_ONDEMAND_YES || (do_bandwidth == CONFIG_ONDEMAND_ONDEMAND && (Ip6InOctets || Ip6OutOctets))) {
		do_bandwidth = CONFIG_ONDEMAND_YES;
		st = rrdset_find("system.ipv6");
		if(!st) {
			st = rrdset_create("system", "ipv6", NULL, "network", NULL, "IPv6 Bandwidth", "kilobits/s", 500, update_every, RRDSET_TYPE_AREA);

			rrddim_add(st, "received", NULL, 8, 1024, RRDDIM_INCREMENTAL);
			rrddim_add(st, "sent", NULL, -8, 1024, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "sent", Ip6OutOctets);
		rrddim_set(st, "received", Ip6InOctets);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_ip_packets == CONFIG_ONDEMAND_YES || (do_ip_packets == CONFIG_ONDEMAND_ONDEMAND && (Ip6InReceives || Ip6OutRequests || Ip6InDelivers || Ip6OutForwDatagrams))) {
		do_ip_packets = CONFIG_ONDEMAND_YES;
		st = rrdset_find(RRD_TYPE_NET_SNMP6 ".packets");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_SNMP6, "packets", NULL, "packets", NULL, "IPv6 Packets", "packets/s", 3000, update_every, RRDSET_TYPE_LINE);

			rrddim_add(st, "received", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "sent", NULL, -1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "forwarded", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "delivers", NULL, -1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "sent", Ip6OutRequests);
		rrddim_set(st, "received", Ip6InReceives);
		rrddim_set(st, "forwarded", Ip6InDelivers);
		rrddim_set(st, "delivers", Ip6OutForwDatagrams);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_ip_fragsout == CONFIG_ONDEMAND_YES || (do_ip_fragsout == CONFIG_ONDEMAND_ONDEMAND && (Ip6FragOKs || Ip6FragFails || Ip6FragCreates))) {
		do_ip_fragsout = CONFIG_ONDEMAND_YES;
		st = rrdset_find(RRD_TYPE_NET_SNMP6 ".fragsout");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_SNMP6, "fragsout", NULL, "fragments", NULL, "IPv6 Fragments Sent", "packets/s", 3010, update_every, RRDSET_TYPE_LINE);
			st->isdetail = 1;

			rrddim_add(st, "ok", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "failed", NULL, -1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "all", NULL, 1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "ok", Ip6FragOKs);
		rrddim_set(st, "failed", Ip6FragFails);
		rrddim_set(st, "all", Ip6FragCreates);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_ip_fragsin == CONFIG_ONDEMAND_YES || (do_ip_fragsin == CONFIG_ONDEMAND_ONDEMAND
		&& (
			Ip6ReasmOKs
			|| Ip6ReasmFails
			|| Ip6ReasmTimeout
			|| Ip6ReasmReqds
			))) {
		do_ip_fragsin = CONFIG_ONDEMAND_YES;
		st = rrdset_find(RRD_TYPE_NET_SNMP6 ".fragsin");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_SNMP6, "fragsin", NULL, "fragments", NULL, "IPv6 Fragments Reassembly", "packets/s", 3011, update_every, RRDSET_TYPE_LINE);
			st->isdetail = 1;

			rrddim_add(st, "ok", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "failed", NULL, -1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "timeout", NULL, -1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "all", NULL, 1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "ok", Ip6ReasmOKs);
		rrddim_set(st, "failed", Ip6ReasmFails);
		rrddim_set(st, "timeout", Ip6ReasmTimeout);
		rrddim_set(st, "all", Ip6ReasmReqds);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_ip_errors == CONFIG_ONDEMAND_YES || (do_ip_errors == CONFIG_ONDEMAND_ONDEMAND
		&& (
			Ip6InDiscards
			|| Ip6OutDiscards
			|| Ip6InHdrErrors
			|| Ip6InAddrErrors
			|| Ip6InUnknownProtos
			|| Ip6InTooBigErrors
			|| Ip6InTruncatedPkts
			|| Ip6InNoRoutes
		))) {
		do_ip_errors = CONFIG_ONDEMAND_YES;
		st = rrdset_find(RRD_TYPE_NET_SNMP6 ".errors");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_SNMP6, "errors", NULL, "errors", NULL, "IPv6 Errors", "packets/s", 3002, update_every, RRDSET_TYPE_LINE);
			st->isdetail = 1;

			rrddim_add(st, "InDiscards", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "OutDiscards", NULL, -1, 1, RRDDIM_INCREMENTAL);

			rrddim_add(st, "InHdrErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "InAddrErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "InUnknownProtos", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "InTooBigErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "InTruncatedPkts", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "InNoRoutes", NULL, 1, 1, RRDDIM_INCREMENTAL);

			rrddim_add(st, "OutNoRoutes", NULL, -1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "InDiscards", Ip6InDiscards);
		rrddim_set(st, "OutDiscards", Ip6OutDiscards);

		rrddim_set(st, "InHdrErrors", Ip6InHdrErrors);
		rrddim_set(st, "InAddrErrors", Ip6InAddrErrors);
		rrddim_set(st, "InUnknownProtos", Ip6InUnknownProtos);
		rrddim_set(st, "InTooBigErrors", Ip6InTooBigErrors);
		rrddim_set(st, "InTruncatedPkts", Ip6InTruncatedPkts);
		rrddim_set(st, "InNoRoutes", Ip6InNoRoutes);

		rrddim_set(st, "OutNoRoutes", Ip6OutNoRoutes);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_udp_packets == CONFIG_ONDEMAND_YES || (do_udp_packets == CONFIG_ONDEMAND_ONDEMAND && (Udp6InDatagrams || Udp6OutDatagrams))) {
		do_udp_packets = CONFIG_ONDEMAND_YES;
		st = rrdset_find(RRD_TYPE_NET_SNMP6 ".udppackets");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_SNMP6, "udppackets", NULL, "udp", NULL, "IPv6 UDP Packets", "packets/s", 3601, update_every, RRDSET_TYPE_LINE);

			rrddim_add(st, "received", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "sent", NULL, -1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "received", Udp6InDatagrams);
		rrddim_set(st, "sent", Udp6OutDatagrams);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_udp_errors == CONFIG_ONDEMAND_YES || (do_udp_errors == CONFIG_ONDEMAND_ONDEMAND
		&& (
			Udp6InErrors
			|| Udp6NoPorts
			|| Udp6RcvbufErrors
			|| Udp6SndbufErrors
			|| Udp6InCsumErrors
			|| Udp6IgnoredMulti
		))) {
		do_udp_errors = CONFIG_ONDEMAND_YES;
		st = rrdset_find(RRD_TYPE_NET_SNMP6 ".udperrors");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_SNMP6, "udperrors", NULL, "udp", NULL, "IPv6 UDP Errors", "events/s", 3701, update_every, RRDSET_TYPE_LINE);
			st->isdetail = 1;

			rrddim_add(st, "RcvbufErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "SndbufErrors", NULL, -1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "InErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "NoPorts", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "InCsumErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "IgnoredMulti", NULL, 1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "InErrors", Udp6InErrors);
		rrddim_set(st, "NoPorts", Udp6NoPorts);
		rrddim_set(st, "RcvbufErrors", Udp6RcvbufErrors);
		rrddim_set(st, "SndbufErrors", Udp6SndbufErrors);
		rrddim_set(st, "InCsumErrors", Udp6InCsumErrors);
		rrddim_set(st, "IgnoredMulti", Udp6IgnoredMulti);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_udplite_packets == CONFIG_ONDEMAND_YES || (do_udplite_packets == CONFIG_ONDEMAND_ONDEMAND && (UdpLite6InDatagrams || UdpLite6OutDatagrams))) {
		do_udplite_packets = CONFIG_ONDEMAND_YES;
		st = rrdset_find(RRD_TYPE_NET_SNMP6 ".udplitepackets");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_SNMP6, "udplitepackets", NULL, "udplite", NULL, "IPv6 UDPlite Packets", "packets/s", 3601, update_every, RRDSET_TYPE_LINE);

			rrddim_add(st, "received", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "sent", NULL, -1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "received", UdpLite6InDatagrams);
		rrddim_set(st, "sent", UdpLite6OutDatagrams);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_udplite_errors == CONFIG_ONDEMAND_YES || (do_udplite_errors == CONFIG_ONDEMAND_ONDEMAND
		&& (
			UdpLite6InErrors
			|| UdpLite6NoPorts
			|| UdpLite6RcvbufErrors
			|| UdpLite6SndbufErrors
			|| Udp6InCsumErrors
			|| UdpLite6InCsumErrors
		))) {
		do_udplite_errors = CONFIG_ONDEMAND_YES;
		st = rrdset_find(RRD_TYPE_NET_SNMP6 ".udpliteerrors");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_SNMP6, "udpliteerrors", NULL, "udplite", NULL, "IPv6 UDP Lite Errors", "events/s", 3701, update_every, RRDSET_TYPE_LINE);
			st->isdetail = 1;

			rrddim_add(st, "RcvbufErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "SndbufErrors", NULL, -1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "InErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "NoPorts", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "InCsumErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "InErrors", UdpLite6InErrors);
		rrddim_set(st, "NoPorts", UdpLite6NoPorts);
		rrddim_set(st, "RcvbufErrors", UdpLite6RcvbufErrors);
		rrddim_set(st, "SndbufErrors", UdpLite6SndbufErrors);
		rrddim_set(st, "InCsumErrors", UdpLite6InCsumErrors);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_mcast == CONFIG_ONDEMAND_YES || (do_mcast == CONFIG_ONDEMAND_ONDEMAND && (Ip6OutMcastOctets || Ip6InMcastOctets))) {
		do_mcast = CONFIG_ONDEMAND_YES;
		st = rrdset_find(RRD_TYPE_NET_SNMP6 ".mcast");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_SNMP6, "mcast", NULL, "multicast", NULL, "IPv6 Multicast Bandwidth", "kilobits/s", 9000, update_every, RRDSET_TYPE_AREA);
			st->isdetail = 1;

			rrddim_add(st, "received", NULL, 8, 1024, RRDDIM_INCREMENTAL);
			rrddim_add(st, "sent", NULL, -8, 1024, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "sent", Ip6OutMcastOctets);
		rrddim_set(st, "received", Ip6InMcastOctets);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_bcast == CONFIG_ONDEMAND_YES || (do_bcast == CONFIG_ONDEMAND_ONDEMAND && (Ip6OutBcastOctets || Ip6InBcastOctets))) {
		do_bcast = CONFIG_ONDEMAND_YES;
		st = rrdset_find(RRD_TYPE_NET_SNMP6 ".bcast");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_SNMP6, "bcast", NULL, "broadcast", NULL, "IPv6 Broadcast Bandwidth", "kilobits/s", 8000, update_every, RRDSET_TYPE_AREA);
			st->isdetail = 1;

			rrddim_add(st, "received", NULL, 8, 1024, RRDDIM_INCREMENTAL);
			rrddim_add(st, "sent", NULL, -8, 1024, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "sent", Ip6OutBcastOctets);
		rrddim_set(st, "received", Ip6InBcastOctets);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_mcast_p == CONFIG_ONDEMAND_YES || (do_mcast_p == CONFIG_ONDEMAND_ONDEMAND && (Ip6OutMcastPkts || Ip6InMcastPkts))) {
		do_mcast_p = CONFIG_ONDEMAND_YES;
		st = rrdset_find(RRD_TYPE_NET_SNMP6 ".mcastpkts");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_SNMP6, "mcastpkts", NULL, "multicast", NULL, "IPv6 Multicast Packets", "packets/s", 9500, update_every, RRDSET_TYPE_LINE);
			st->isdetail = 1;

			rrddim_add(st, "received", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "sent", NULL, -1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "sent", Ip6OutMcastPkts);
		rrddim_set(st, "received", Ip6InMcastPkts);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_icmp == CONFIG_ONDEMAND_YES || (do_icmp == CONFIG_ONDEMAND_ONDEMAND && (Icmp6InMsgs || Icmp6OutMsgs))) {
		do_icmp = CONFIG_ONDEMAND_YES;
		st = rrdset_find(RRD_TYPE_NET_SNMP6 ".icmp");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_SNMP6, "icmp", NULL, "icmp", NULL, "IPv6 ICMP Messages", "messages/s", 10000, update_every, RRDSET_TYPE_LINE);

			rrddim_add(st, "received", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "sent", NULL, -1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "sent", Icmp6InMsgs);
		rrddim_set(st, "received", Icmp6OutMsgs);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_icmp_redir == CONFIG_ONDEMAND_YES || (do_icmp_redir == CONFIG_ONDEMAND_ONDEMAND && (Icmp6InRedirects || Icmp6OutRedirects))) {
		do_icmp_redir = CONFIG_ONDEMAND_YES;
		st = rrdset_find(RRD_TYPE_NET_SNMP6 ".icmpredir");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_SNMP6, "icmpredir", NULL, "icmp", NULL, "IPv6 ICMP Redirects", "redirects/s", 10050, update_every, RRDSET_TYPE_LINE);

			rrddim_add(st, "received", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "sent", NULL, -1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "sent", Icmp6InRedirects);
		rrddim_set(st, "received", Icmp6OutRedirects);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_icmp_errors == CONFIG_ONDEMAND_YES || (do_icmp_errors == CONFIG_ONDEMAND_ONDEMAND
		&& (
			Icmp6InErrors
			|| Icmp6OutErrors
			|| Icmp6InCsumErrors
			|| Icmp6InDestUnreachs
			|| Icmp6InPktTooBigs
			|| Icmp6InTimeExcds
			|| Icmp6InParmProblems
			|| Icmp6OutDestUnreachs
			|| Icmp6OutPktTooBigs
			|| Icmp6OutTimeExcds
			|| Icmp6OutParmProblems
		))) {
		do_icmp_errors = CONFIG_ONDEMAND_YES;
		st = rrdset_find(RRD_TYPE_NET_SNMP6 ".icmperrors");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_SNMP6, "icmperrors", NULL, "icmp", NULL, "IPv6 ICMP Errors", "errors/s", 10100, update_every, RRDSET_TYPE_LINE);

			rrddim_add(st, "InErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "OutErrors", NULL, -1, 1, RRDDIM_INCREMENTAL);

			rrddim_add(st, "InCsumErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "InDestUnreachs", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "InPktTooBigs", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "InTimeExcds", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "InParmProblems", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "OutDestUnreachs", NULL, -1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "OutPktTooBigs", NULL, -1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "OutTimeExcds", NULL, -1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "OutParmProblems", NULL, -1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "InErrors", Icmp6InErrors);
		rrddim_set(st, "OutErrors", Icmp6OutErrors);
		rrddim_set(st, "InCsumErrors", Icmp6InCsumErrors);
		rrddim_set(st, "InDestUnreachs", Icmp6InDestUnreachs);
		rrddim_set(st, "InPktTooBigs", Icmp6InPktTooBigs);
		rrddim_set(st, "InTimeExcds", Icmp6InTimeExcds);
		rrddim_set(st, "InParmProblems", Icmp6InParmProblems);
		rrddim_set(st, "OutDestUnreachs", Icmp6OutDestUnreachs);
		rrddim_set(st, "OutPktTooBigs", Icmp6OutPktTooBigs);
		rrddim_set(st, "OutTimeExcds", Icmp6OutTimeExcds);
		rrddim_set(st, "OutParmProblems", Icmp6OutParmProblems);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_icmp_echos == CONFIG_ONDEMAND_YES || (do_icmp_echos == CONFIG_ONDEMAND_ONDEMAND
		&& (
			Icmp6InEchos
			|| Icmp6OutEchos
			|| Icmp6InEchoReplies
			|| Icmp6OutEchoReplies
		))) {
		do_icmp_echos = CONFIG_ONDEMAND_YES;
		st = rrdset_find(RRD_TYPE_NET_SNMP6 ".icmpechos");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_SNMP6, "icmpechos", NULL, "icmp", NULL, "IPv6 ICMP Echo", "messages/s", 10200, update_every, RRDSET_TYPE_LINE);

			rrddim_add(st, "InEchos", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "OutEchos", NULL, -1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "InEchoReplies", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "OutEchoReplies", NULL, -1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "InEchos", Icmp6InEchos);
		rrddim_set(st, "OutEchos", Icmp6OutEchos);
		rrddim_set(st, "InEchoReplies", Icmp6InEchoReplies);
		rrddim_set(st, "OutEchoReplies", Icmp6OutEchoReplies);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_icmp_groupmemb == CONFIG_ONDEMAND_YES || (do_icmp_groupmemb == CONFIG_ONDEMAND_ONDEMAND
		&& (
			Icmp6InGroupMembQueries
			|| Icmp6OutGroupMembQueries
			|| Icmp6InGroupMembResponses
			|| Icmp6OutGroupMembResponses
			|| Icmp6InGroupMembReductions
			|| Icmp6OutGroupMembReductions
		))) {
		do_icmp_groupmemb = CONFIG_ONDEMAND_YES;
		st = rrdset_find(RRD_TYPE_NET_SNMP6 ".groupmemb");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_SNMP6, "groupmemb", NULL, "icmp", NULL, "IPv6 ICMP Group Membership", "messages/s", 10300, update_every, RRDSET_TYPE_LINE);

			rrddim_add(st, "InQueries", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "OutQueries", NULL, -1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "InResponses", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "OutResponses", NULL, -1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "InReductions", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "OutReductions", NULL, -1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "InQueries", Icmp6InGroupMembQueries);
		rrddim_set(st, "OutQueries", Icmp6OutGroupMembQueries);
		rrddim_set(st, "InResponses", Icmp6InGroupMembResponses);
		rrddim_set(st, "OutResponses", Icmp6OutGroupMembResponses);
		rrddim_set(st, "InReductions", Icmp6InGroupMembReductions);
		rrddim_set(st, "OutReductions", Icmp6OutGroupMembReductions);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_icmp_router == CONFIG_ONDEMAND_YES || (do_icmp_router == CONFIG_ONDEMAND_ONDEMAND
		&& (
			Icmp6InRouterSolicits
			|| Icmp6OutRouterSolicits
			|| Icmp6InRouterAdvertisements
			|| Icmp6OutRouterAdvertisements
		))) {
		do_icmp_router = CONFIG_ONDEMAND_YES;
		st = rrdset_find(RRD_TYPE_NET_SNMP6 ".icmprouter");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_SNMP6, "icmprouter", NULL, "icmp", NULL, "IPv6 Router Messages", "messages/s", 10400, update_every, RRDSET_TYPE_LINE);

			rrddim_add(st, "InSolicits", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "OutSolicits", NULL, -1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "InAdvertisements", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "OutAdvertisements", NULL, -1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "InSolicits", Icmp6InRouterSolicits);
		rrddim_set(st, "OutSolicits", Icmp6OutRouterSolicits);
		rrddim_set(st, "InAdvertisements", Icmp6InRouterAdvertisements);
		rrddim_set(st, "OutAdvertisements", Icmp6OutRouterAdvertisements);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_icmp_neighbor == CONFIG_ONDEMAND_YES || (do_icmp_neighbor == CONFIG_ONDEMAND_ONDEMAND
		&& (
			Icmp6InNeighborSolicits
			|| Icmp6OutNeighborSolicits
			|| Icmp6InNeighborAdvertisements
			|| Icmp6OutNeighborAdvertisements
		))) {
		do_icmp_neighbor = CONFIG_ONDEMAND_YES;
		st = rrdset_find(RRD_TYPE_NET_SNMP6 ".icmpneighbor");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_SNMP6, "icmpneighbor", NULL, "icmp", NULL, "IPv6 Neighbor Messages", "messages/s", 10500, update_every, RRDSET_TYPE_LINE);

			rrddim_add(st, "InSolicits", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "OutSolicits", NULL, -1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "InAdvertisements", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "OutAdvertisements", NULL, -1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "InSolicits", Icmp6InNeighborSolicits);
		rrddim_set(st, "OutSolicits", Icmp6OutNeighborSolicits);
		rrddim_set(st, "InAdvertisements", Icmp6InNeighborAdvertisements);
		rrddim_set(st, "OutAdvertisements", Icmp6OutNeighborAdvertisements);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_icmp_mldv2 == CONFIG_ONDEMAND_YES || (do_icmp_mldv2 == CONFIG_ONDEMAND_ONDEMAND && (Icmp6InMLDv2Reports || Icmp6OutMLDv2Reports))) {
		do_icmp_mldv2 = CONFIG_ONDEMAND_YES;
		st = rrdset_find(RRD_TYPE_NET_SNMP6 ".icmpmldv2");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_SNMP6, "icmpmldv2", NULL, "icmp", NULL, "IPv6 ICMP MLDv2 Reports", "reports/s", 10600, update_every, RRDSET_TYPE_LINE);

			rrddim_add(st, "received", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "sent", NULL, -1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "sent", Icmp6InMLDv2Reports);
		rrddim_set(st, "received", Icmp6OutMLDv2Reports);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_icmp_types == CONFIG_ONDEMAND_YES || (do_icmp_types == CONFIG_ONDEMAND_ONDEMAND
		&& (
			Icmp6InType1
			|| Icmp6InType128
			|| Icmp6InType129
			|| Icmp6InType136
			|| Icmp6OutType1
			|| Icmp6OutType128
			|| Icmp6OutType129
			|| Icmp6OutType133
			|| Icmp6OutType135
			|| Icmp6OutType143
		))) {
		do_icmp_types = CONFIG_ONDEMAND_YES;
		st = rrdset_find(RRD_TYPE_NET_SNMP6 ".icmptypes");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_SNMP6, "icmptypes", NULL, "icmp", NULL, "IPv6 ICMP Types", "messages/s", 10700, update_every, RRDSET_TYPE_LINE);

			rrddim_add(st, "InType1", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "InType128", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "InType129", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "InType136", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "OutType1", NULL, -1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "OutType128", NULL, -1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "OutType129", NULL, -1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "OutType133", NULL, -1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "OutType135", NULL, -1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "OutType143", NULL, -1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "InType1", Icmp6InType1);
		rrddim_set(st, "InType128", Icmp6InType128);
		rrddim_set(st, "InType129", Icmp6InType129);
		rrddim_set(st, "InType136", Icmp6InType136);
		rrddim_set(st, "OutType1", Icmp6OutType1);
		rrddim_set(st, "OutType128", Icmp6OutType128);
		rrddim_set(st, "OutType129", Icmp6OutType129);
		rrddim_set(st, "OutType133", Icmp6OutType133);
		rrddim_set(st, "OutType135", Icmp6OutType135);
		rrddim_set(st, "OutType143", Icmp6OutType143);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_ect == CONFIG_ONDEMAND_YES || (do_ect == CONFIG_ONDEMAND_ONDEMAND
		&& (
			Ip6InNoECTPkts
			|| Ip6InECT1Pkts
			|| Ip6InECT0Pkts
			|| Ip6InCEPkts
		))) {
		do_ect = CONFIG_ONDEMAND_YES;
		st = rrdset_find(RRD_TYPE_NET_SNMP6 ".ect");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_SNMP6, "ect", NULL, "packets", NULL, "IPv6 ECT Packets", "packets/s", 10800, update_every, RRDSET_TYPE_LINE);

			rrddim_add(st, "InNoECTPkts", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "InECT1Pkts", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "InECT0Pkts", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "InCEPkts", NULL, 1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "InNoECTPkts", Ip6InNoECTPkts);
		rrddim_set(st, "InECT1Pkts", Ip6InECT1Pkts);
		rrddim_set(st, "InECT0Pkts", Ip6InECT0Pkts);
		rrddim_set(st, "InCEPkts", Ip6InCEPkts);
		rrdset_done(st);
	}

	return 0;
}


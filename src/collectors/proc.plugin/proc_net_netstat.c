// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

#define RRD_TYPE_NET_IP "ip"
#define RRD_TYPE_NET_IP4 "ipv4"
#define RRD_TYPE_NET_IP6 "ipv6"
#define PLUGIN_PROC_MODULE_NETSTAT_NAME "/proc/net/netstat"
#define CONFIG_SECTION_PLUGIN_PROC_NETSTAT "plugin:" PLUGIN_PROC_CONFIG_NAME ":" PLUGIN_PROC_MODULE_NETSTAT_NAME

static struct proc_net_snmp {
    // kernel_uint_t ip_Forwarding;
    kernel_uint_t ip_DefaultTTL;
    kernel_uint_t ip_InReceives;
    kernel_uint_t ip_InHdrErrors;
    kernel_uint_t ip_InAddrErrors;
    kernel_uint_t ip_ForwDatagrams;
    kernel_uint_t ip_InUnknownProtos;
    kernel_uint_t ip_InDiscards;
    kernel_uint_t ip_InDelivers;
    kernel_uint_t ip_OutRequests;
    kernel_uint_t ip_OutDiscards;
    kernel_uint_t ip_OutNoRoutes;
    kernel_uint_t ip_ReasmTimeout;
    kernel_uint_t ip_ReasmReqds;
    kernel_uint_t ip_ReasmOKs;
    kernel_uint_t ip_ReasmFails;
    kernel_uint_t ip_FragOKs;
    kernel_uint_t ip_FragFails;
    kernel_uint_t ip_FragCreates;

    kernel_uint_t icmp_InMsgs;
    kernel_uint_t icmp_OutMsgs;
    kernel_uint_t icmp_InErrors;
    kernel_uint_t icmp_OutErrors;
    kernel_uint_t icmp_InCsumErrors;

    kernel_uint_t icmpmsg_InEchoReps;
    kernel_uint_t icmpmsg_OutEchoReps;
    kernel_uint_t icmpmsg_InDestUnreachs;
    kernel_uint_t icmpmsg_OutDestUnreachs;
    kernel_uint_t icmpmsg_InRedirects;
    kernel_uint_t icmpmsg_OutRedirects;
    kernel_uint_t icmpmsg_InEchos;
    kernel_uint_t icmpmsg_OutEchos;
    kernel_uint_t icmpmsg_InRouterAdvert;
    kernel_uint_t icmpmsg_OutRouterAdvert;
    kernel_uint_t icmpmsg_InRouterSelect;
    kernel_uint_t icmpmsg_OutRouterSelect;
    kernel_uint_t icmpmsg_InTimeExcds;
    kernel_uint_t icmpmsg_OutTimeExcds;
    kernel_uint_t icmpmsg_InParmProbs;
    kernel_uint_t icmpmsg_OutParmProbs;
    kernel_uint_t icmpmsg_InTimestamps;
    kernel_uint_t icmpmsg_OutTimestamps;
    kernel_uint_t icmpmsg_InTimestampReps;
    kernel_uint_t icmpmsg_OutTimestampReps;

    //kernel_uint_t tcp_RtoAlgorithm;
    //kernel_uint_t tcp_RtoMin;
    //kernel_uint_t tcp_RtoMax;
    ssize_t tcp_MaxConn;
    kernel_uint_t tcp_ActiveOpens;
    kernel_uint_t tcp_PassiveOpens;
    kernel_uint_t tcp_AttemptFails;
    kernel_uint_t tcp_EstabResets;
    kernel_uint_t tcp_CurrEstab;
    kernel_uint_t tcp_InSegs;
    kernel_uint_t tcp_OutSegs;
    kernel_uint_t tcp_RetransSegs;
    kernel_uint_t tcp_InErrs;
    kernel_uint_t tcp_OutRsts;
    kernel_uint_t tcp_InCsumErrors;

    kernel_uint_t udp_InDatagrams;
    kernel_uint_t udp_NoPorts;
    kernel_uint_t udp_InErrors;
    kernel_uint_t udp_OutDatagrams;
    kernel_uint_t udp_RcvbufErrors;
    kernel_uint_t udp_SndbufErrors;
    kernel_uint_t udp_InCsumErrors;
    kernel_uint_t udp_IgnoredMulti;

    kernel_uint_t udplite_InDatagrams;
    kernel_uint_t udplite_NoPorts;
    kernel_uint_t udplite_InErrors;
    kernel_uint_t udplite_OutDatagrams;
    kernel_uint_t udplite_RcvbufErrors;
    kernel_uint_t udplite_SndbufErrors;
    kernel_uint_t udplite_InCsumErrors;
    kernel_uint_t udplite_IgnoredMulti;
} snmp_root = { 0 };

static void parse_line_pair(procfile *ff_netstat, ARL_BASE *base, size_t header_line, size_t values_line) {
    size_t hwords = procfile_linewords(ff_netstat, header_line);
    size_t vwords = procfile_linewords(ff_netstat, values_line);
    size_t w;

    if(unlikely(vwords > hwords)) {
        collector_error("File /proc/net/netstat on header line %zu has %zu words, but on value line %zu has %zu words.", header_line, hwords, values_line, vwords);
        vwords = hwords;
    }

    for(w = 1; w < vwords ;w++) {
        if(unlikely(arl_check(base, procfile_lineword(ff_netstat, header_line, w), procfile_lineword(ff_netstat, values_line, w))))
            break;
    }
}

static void do_proc_net_snmp6(int update_every) {
    static bool do_snmp6 = true;

    if (!do_snmp6) {
        return;
    }

    static int do_ip6_packets = -1, do_ip6_fragsout = -1, do_ip6_fragsin = -1, do_ip6_errors = -1,
               do_ip6_udplite_packets = -1, do_ip6_udplite_errors = -1, do_ip6_udp_packets = -1, do_ip6_udp_errors = -1,
               do_ip6_bandwidth = -1, do_ip6_mcast = -1, do_ip6_bcast = -1, do_ip6_mcast_p = -1, do_ip6_icmp = -1,
               do_ip6_icmp_redir = -1, do_ip6_icmp_errors = -1, do_ip6_icmp_echos = -1, do_ip6_icmp_groupmemb = -1,
               do_ip6_icmp_router = -1, do_ip6_icmp_neighbor = -1, do_ip6_icmp_mldv2 = -1, do_ip6_icmp_types = -1,
               do_ip6_ect = -1;

    static procfile *ff_snmp6 = NULL;

    static ARL_BASE *arl_ipv6 = NULL;

    static unsigned long long Ip6InReceives = 0ULL;
    static unsigned long long Ip6InHdrErrors = 0ULL;
    static unsigned long long Ip6InTooBigErrors = 0ULL;
    static unsigned long long Ip6InNoRoutes = 0ULL;
    static unsigned long long Ip6InAddrErrors = 0ULL;
    static unsigned long long Ip6InUnknownProtos = 0ULL;
    static unsigned long long Ip6InTruncatedPkts = 0ULL;
    static unsigned long long Ip6InDiscards = 0ULL;
    static unsigned long long Ip6InDelivers = 0ULL;
    static unsigned long long Ip6OutForwDatagrams = 0ULL;
    static unsigned long long Ip6OutRequests = 0ULL;
    static unsigned long long Ip6OutDiscards = 0ULL;
    static unsigned long long Ip6OutNoRoutes = 0ULL;
    static unsigned long long Ip6ReasmTimeout = 0ULL;
    static unsigned long long Ip6ReasmReqds = 0ULL;
    static unsigned long long Ip6ReasmOKs = 0ULL;
    static unsigned long long Ip6ReasmFails = 0ULL;
    static unsigned long long Ip6FragOKs = 0ULL;
    static unsigned long long Ip6FragFails = 0ULL;
    static unsigned long long Ip6FragCreates = 0ULL;
    static unsigned long long Ip6InMcastPkts = 0ULL;
    static unsigned long long Ip6OutMcastPkts = 0ULL;
    static unsigned long long Ip6InOctets = 0ULL;
    static unsigned long long Ip6OutOctets = 0ULL;
    static unsigned long long Ip6InMcastOctets = 0ULL;
    static unsigned long long Ip6OutMcastOctets = 0ULL;
    static unsigned long long Ip6InBcastOctets = 0ULL;
    static unsigned long long Ip6OutBcastOctets = 0ULL;
    static unsigned long long Ip6InNoECTPkts = 0ULL;
    static unsigned long long Ip6InECT1Pkts = 0ULL;
    static unsigned long long Ip6InECT0Pkts = 0ULL;
    static unsigned long long Ip6InCEPkts = 0ULL;
    static unsigned long long Icmp6InMsgs = 0ULL;
    static unsigned long long Icmp6InErrors = 0ULL;
    static unsigned long long Icmp6OutMsgs = 0ULL;
    static unsigned long long Icmp6OutErrors = 0ULL;
    static unsigned long long Icmp6InCsumErrors = 0ULL;
    static unsigned long long Icmp6InDestUnreachs = 0ULL;
    static unsigned long long Icmp6InPktTooBigs = 0ULL;
    static unsigned long long Icmp6InTimeExcds = 0ULL;
    static unsigned long long Icmp6InParmProblems = 0ULL;
    static unsigned long long Icmp6InEchos = 0ULL;
    static unsigned long long Icmp6InEchoReplies = 0ULL;
    static unsigned long long Icmp6InGroupMembQueries = 0ULL;
    static unsigned long long Icmp6InGroupMembResponses = 0ULL;
    static unsigned long long Icmp6InGroupMembReductions = 0ULL;
    static unsigned long long Icmp6InRouterSolicits = 0ULL;
    static unsigned long long Icmp6InRouterAdvertisements = 0ULL;
    static unsigned long long Icmp6InNeighborSolicits = 0ULL;
    static unsigned long long Icmp6InNeighborAdvertisements = 0ULL;
    static unsigned long long Icmp6InRedirects = 0ULL;
    static unsigned long long Icmp6InMLDv2Reports = 0ULL;
    static unsigned long long Icmp6OutDestUnreachs = 0ULL;
    static unsigned long long Icmp6OutPktTooBigs = 0ULL;
    static unsigned long long Icmp6OutTimeExcds = 0ULL;
    static unsigned long long Icmp6OutParmProblems = 0ULL;
    static unsigned long long Icmp6OutEchos = 0ULL;
    static unsigned long long Icmp6OutEchoReplies = 0ULL;
    static unsigned long long Icmp6OutGroupMembQueries = 0ULL;
    static unsigned long long Icmp6OutGroupMembResponses = 0ULL;
    static unsigned long long Icmp6OutGroupMembReductions = 0ULL;
    static unsigned long long Icmp6OutRouterSolicits = 0ULL;
    static unsigned long long Icmp6OutRouterAdvertisements = 0ULL;
    static unsigned long long Icmp6OutNeighborSolicits = 0ULL;
    static unsigned long long Icmp6OutNeighborAdvertisements = 0ULL;
    static unsigned long long Icmp6OutRedirects = 0ULL;
    static unsigned long long Icmp6OutMLDv2Reports = 0ULL;
    static unsigned long long Icmp6InType1 = 0ULL;
    static unsigned long long Icmp6InType128 = 0ULL;
    static unsigned long long Icmp6InType129 = 0ULL;
    static unsigned long long Icmp6InType136 = 0ULL;
    static unsigned long long Icmp6OutType1 = 0ULL;
    static unsigned long long Icmp6OutType128 = 0ULL;
    static unsigned long long Icmp6OutType129 = 0ULL;
    static unsigned long long Icmp6OutType133 = 0ULL;
    static unsigned long long Icmp6OutType135 = 0ULL;
    static unsigned long long Icmp6OutType143 = 0ULL;
    static unsigned long long Udp6InDatagrams = 0ULL;
    static unsigned long long Udp6NoPorts = 0ULL;
    static unsigned long long Udp6InErrors = 0ULL;
    static unsigned long long Udp6OutDatagrams = 0ULL;
    static unsigned long long Udp6RcvbufErrors = 0ULL;
    static unsigned long long Udp6SndbufErrors = 0ULL;
    static unsigned long long Udp6InCsumErrors = 0ULL;
    static unsigned long long Udp6IgnoredMulti = 0ULL;
    static unsigned long long UdpLite6InDatagrams = 0ULL;
    static unsigned long long UdpLite6NoPorts = 0ULL;
    static unsigned long long UdpLite6InErrors = 0ULL;
    static unsigned long long UdpLite6OutDatagrams = 0ULL;
    static unsigned long long UdpLite6RcvbufErrors = 0ULL;
    static unsigned long long UdpLite6SndbufErrors = 0ULL;
    static unsigned long long UdpLite6InCsumErrors = 0ULL;

    // prepare for /proc/net/snmp6 parsing

    if(unlikely(!arl_ipv6)) {
        do_ip6_packets          = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp6", "ipv6 packets", CONFIG_BOOLEAN_AUTO);
        do_ip6_fragsout         = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp6", "ipv6 fragments sent", CONFIG_BOOLEAN_AUTO);
        do_ip6_fragsin          = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp6", "ipv6 fragments assembly", CONFIG_BOOLEAN_AUTO);
        do_ip6_errors           = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp6", "ipv6 errors", CONFIG_BOOLEAN_AUTO);
        do_ip6_udp_packets      = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp6", "ipv6 UDP packets", CONFIG_BOOLEAN_AUTO);
        do_ip6_udp_errors       = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp6", "ipv6 UDP errors", CONFIG_BOOLEAN_AUTO);
        do_ip6_udplite_packets  = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp6", "ipv6 UDPlite packets", CONFIG_BOOLEAN_AUTO);
        do_ip6_udplite_errors   = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp6", "ipv6 UDPlite errors", CONFIG_BOOLEAN_AUTO);
        do_ip6_bandwidth        = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp6", "bandwidth", CONFIG_BOOLEAN_AUTO);
        do_ip6_mcast            = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp6", "multicast bandwidth", CONFIG_BOOLEAN_AUTO);
        do_ip6_bcast            = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp6", "broadcast bandwidth", CONFIG_BOOLEAN_AUTO);
        do_ip6_mcast_p          = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp6", "multicast packets", CONFIG_BOOLEAN_AUTO);
        do_ip6_icmp             = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp6", "icmp", CONFIG_BOOLEAN_AUTO);
        do_ip6_icmp_redir       = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp6", "icmp redirects", CONFIG_BOOLEAN_AUTO);
        do_ip6_icmp_errors      = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp6", "icmp errors", CONFIG_BOOLEAN_AUTO);
        do_ip6_icmp_echos       = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp6", "icmp echos", CONFIG_BOOLEAN_AUTO);
        do_ip6_icmp_groupmemb   = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp6", "icmp group membership", CONFIG_BOOLEAN_AUTO);
        do_ip6_icmp_router      = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp6", "icmp router", CONFIG_BOOLEAN_AUTO);
        do_ip6_icmp_neighbor    = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp6", "icmp neighbor", CONFIG_BOOLEAN_AUTO);
        do_ip6_icmp_mldv2       = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp6", "icmp mldv2", CONFIG_BOOLEAN_AUTO);
        do_ip6_icmp_types       = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp6", "icmp types", CONFIG_BOOLEAN_AUTO);
        do_ip6_ect              = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp6", "ect", CONFIG_BOOLEAN_AUTO);

        arl_ipv6 = arl_create("snmp6", NULL, 60);
        arl_expect(arl_ipv6, "Ip6InReceives", &Ip6InReceives);
        arl_expect(arl_ipv6, "Ip6InHdrErrors", &Ip6InHdrErrors);
        arl_expect(arl_ipv6, "Ip6InTooBigErrors", &Ip6InTooBigErrors);
        arl_expect(arl_ipv6, "Ip6InNoRoutes", &Ip6InNoRoutes);
        arl_expect(arl_ipv6, "Ip6InAddrErrors", &Ip6InAddrErrors);
        arl_expect(arl_ipv6, "Ip6InUnknownProtos", &Ip6InUnknownProtos);
        arl_expect(arl_ipv6, "Ip6InTruncatedPkts", &Ip6InTruncatedPkts);
        arl_expect(arl_ipv6, "Ip6InDiscards", &Ip6InDiscards);
        arl_expect(arl_ipv6, "Ip6InDelivers", &Ip6InDelivers);
        arl_expect(arl_ipv6, "Ip6OutForwDatagrams", &Ip6OutForwDatagrams);
        arl_expect(arl_ipv6, "Ip6OutRequests", &Ip6OutRequests);
        arl_expect(arl_ipv6, "Ip6OutDiscards", &Ip6OutDiscards);
        arl_expect(arl_ipv6, "Ip6OutNoRoutes", &Ip6OutNoRoutes);
        arl_expect(arl_ipv6, "Ip6ReasmTimeout", &Ip6ReasmTimeout);
        arl_expect(arl_ipv6, "Ip6ReasmReqds", &Ip6ReasmReqds);
        arl_expect(arl_ipv6, "Ip6ReasmOKs", &Ip6ReasmOKs);
        arl_expect(arl_ipv6, "Ip6ReasmFails", &Ip6ReasmFails);
        arl_expect(arl_ipv6, "Ip6FragOKs", &Ip6FragOKs);
        arl_expect(arl_ipv6, "Ip6FragFails", &Ip6FragFails);
        arl_expect(arl_ipv6, "Ip6FragCreates", &Ip6FragCreates);
        arl_expect(arl_ipv6, "Ip6InMcastPkts", &Ip6InMcastPkts);
        arl_expect(arl_ipv6, "Ip6OutMcastPkts", &Ip6OutMcastPkts);
        arl_expect(arl_ipv6, "Ip6InOctets", &Ip6InOctets);
        arl_expect(arl_ipv6, "Ip6OutOctets", &Ip6OutOctets);
        arl_expect(arl_ipv6, "Ip6InMcastOctets", &Ip6InMcastOctets);
        arl_expect(arl_ipv6, "Ip6OutMcastOctets", &Ip6OutMcastOctets);
        arl_expect(arl_ipv6, "Ip6InBcastOctets", &Ip6InBcastOctets);
        arl_expect(arl_ipv6, "Ip6OutBcastOctets", &Ip6OutBcastOctets);
        arl_expect(arl_ipv6, "Ip6InNoECTPkts", &Ip6InNoECTPkts);
        arl_expect(arl_ipv6, "Ip6InECT1Pkts", &Ip6InECT1Pkts);
        arl_expect(arl_ipv6, "Ip6InECT0Pkts", &Ip6InECT0Pkts);
        arl_expect(arl_ipv6, "Ip6InCEPkts", &Ip6InCEPkts);
        arl_expect(arl_ipv6, "Icmp6InMsgs", &Icmp6InMsgs);
        arl_expect(arl_ipv6, "Icmp6InErrors", &Icmp6InErrors);
        arl_expect(arl_ipv6, "Icmp6OutMsgs", &Icmp6OutMsgs);
        arl_expect(arl_ipv6, "Icmp6OutErrors", &Icmp6OutErrors);
        arl_expect(arl_ipv6, "Icmp6InCsumErrors", &Icmp6InCsumErrors);
        arl_expect(arl_ipv6, "Icmp6InDestUnreachs", &Icmp6InDestUnreachs);
        arl_expect(arl_ipv6, "Icmp6InPktTooBigs", &Icmp6InPktTooBigs);
        arl_expect(arl_ipv6, "Icmp6InTimeExcds", &Icmp6InTimeExcds);
        arl_expect(arl_ipv6, "Icmp6InParmProblems", &Icmp6InParmProblems);
        arl_expect(arl_ipv6, "Icmp6InEchos", &Icmp6InEchos);
        arl_expect(arl_ipv6, "Icmp6InEchoReplies", &Icmp6InEchoReplies);
        arl_expect(arl_ipv6, "Icmp6InGroupMembQueries", &Icmp6InGroupMembQueries);
        arl_expect(arl_ipv6, "Icmp6InGroupMembResponses", &Icmp6InGroupMembResponses);
        arl_expect(arl_ipv6, "Icmp6InGroupMembReductions", &Icmp6InGroupMembReductions);
        arl_expect(arl_ipv6, "Icmp6InRouterSolicits", &Icmp6InRouterSolicits);
        arl_expect(arl_ipv6, "Icmp6InRouterAdvertisements", &Icmp6InRouterAdvertisements);
        arl_expect(arl_ipv6, "Icmp6InNeighborSolicits", &Icmp6InNeighborSolicits);
        arl_expect(arl_ipv6, "Icmp6InNeighborAdvertisements", &Icmp6InNeighborAdvertisements);
        arl_expect(arl_ipv6, "Icmp6InRedirects", &Icmp6InRedirects);
        arl_expect(arl_ipv6, "Icmp6InMLDv2Reports", &Icmp6InMLDv2Reports);
        arl_expect(arl_ipv6, "Icmp6OutDestUnreachs", &Icmp6OutDestUnreachs);
        arl_expect(arl_ipv6, "Icmp6OutPktTooBigs", &Icmp6OutPktTooBigs);
        arl_expect(arl_ipv6, "Icmp6OutTimeExcds", &Icmp6OutTimeExcds);
        arl_expect(arl_ipv6, "Icmp6OutParmProblems", &Icmp6OutParmProblems);
        arl_expect(arl_ipv6, "Icmp6OutEchos", &Icmp6OutEchos);
        arl_expect(arl_ipv6, "Icmp6OutEchoReplies", &Icmp6OutEchoReplies);
        arl_expect(arl_ipv6, "Icmp6OutGroupMembQueries", &Icmp6OutGroupMembQueries);
        arl_expect(arl_ipv6, "Icmp6OutGroupMembResponses", &Icmp6OutGroupMembResponses);
        arl_expect(arl_ipv6, "Icmp6OutGroupMembReductions", &Icmp6OutGroupMembReductions);
        arl_expect(arl_ipv6, "Icmp6OutRouterSolicits", &Icmp6OutRouterSolicits);
        arl_expect(arl_ipv6, "Icmp6OutRouterAdvertisements", &Icmp6OutRouterAdvertisements);
        arl_expect(arl_ipv6, "Icmp6OutNeighborSolicits", &Icmp6OutNeighborSolicits);
        arl_expect(arl_ipv6, "Icmp6OutNeighborAdvertisements", &Icmp6OutNeighborAdvertisements);
        arl_expect(arl_ipv6, "Icmp6OutRedirects", &Icmp6OutRedirects);
        arl_expect(arl_ipv6, "Icmp6OutMLDv2Reports", &Icmp6OutMLDv2Reports);
        arl_expect(arl_ipv6, "Icmp6InType1", &Icmp6InType1);
        arl_expect(arl_ipv6, "Icmp6InType128", &Icmp6InType128);
        arl_expect(arl_ipv6, "Icmp6InType129", &Icmp6InType129);
        arl_expect(arl_ipv6, "Icmp6InType136", &Icmp6InType136);
        arl_expect(arl_ipv6, "Icmp6OutType1", &Icmp6OutType1);
        arl_expect(arl_ipv6, "Icmp6OutType128", &Icmp6OutType128);
        arl_expect(arl_ipv6, "Icmp6OutType129", &Icmp6OutType129);
        arl_expect(arl_ipv6, "Icmp6OutType133", &Icmp6OutType133);
        arl_expect(arl_ipv6, "Icmp6OutType135", &Icmp6OutType135);
        arl_expect(arl_ipv6, "Icmp6OutType143", &Icmp6OutType143);
        arl_expect(arl_ipv6, "Udp6InDatagrams", &Udp6InDatagrams);
        arl_expect(arl_ipv6, "Udp6NoPorts", &Udp6NoPorts);
        arl_expect(arl_ipv6, "Udp6InErrors", &Udp6InErrors);
        arl_expect(arl_ipv6, "Udp6OutDatagrams", &Udp6OutDatagrams);
        arl_expect(arl_ipv6, "Udp6RcvbufErrors", &Udp6RcvbufErrors);
        arl_expect(arl_ipv6, "Udp6SndbufErrors", &Udp6SndbufErrors);
        arl_expect(arl_ipv6, "Udp6InCsumErrors", &Udp6InCsumErrors);
        arl_expect(arl_ipv6, "Udp6IgnoredMulti", &Udp6IgnoredMulti);
        arl_expect(arl_ipv6, "UdpLite6InDatagrams", &UdpLite6InDatagrams);
        arl_expect(arl_ipv6, "UdpLite6NoPorts", &UdpLite6NoPorts);
        arl_expect(arl_ipv6, "UdpLite6InErrors", &UdpLite6InErrors);
        arl_expect(arl_ipv6, "UdpLite6OutDatagrams", &UdpLite6OutDatagrams);
        arl_expect(arl_ipv6, "UdpLite6RcvbufErrors", &UdpLite6RcvbufErrors);
        arl_expect(arl_ipv6, "UdpLite6SndbufErrors", &UdpLite6SndbufErrors);
        arl_expect(arl_ipv6, "UdpLite6InCsumErrors", &UdpLite6InCsumErrors);
    }

    // parse /proc/net/snmp

    if (unlikely(!ff_snmp6)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/net/snmp6");
        ff_snmp6 = procfile_open(
            inicfg_get(&netdata_config, "plugin:proc:/proc/net/snmp6", "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
        if (unlikely(!ff_snmp6)) {
            do_snmp6 = false;
            return;
        }
    }

    ff_snmp6 = procfile_readall(ff_snmp6);
    if (unlikely(!ff_snmp6))
        return;

    size_t lines, l;

    lines = procfile_lines(ff_snmp6);

    arl_begin(arl_ipv6);

    for (l = 0; l < lines; l++) {
        size_t words = procfile_linewords(ff_snmp6, l);
        if (unlikely(words < 2)) {
            if (unlikely(words)) {
                collector_error("Cannot read /proc/net/snmp6 line %zu. Expected 2 params, read %zu.", l, words);
                continue;
            }
        }

        if (unlikely(arl_check(arl_ipv6, procfile_lineword(ff_snmp6, l, 0), procfile_lineword(ff_snmp6, l, 1))))
            break;
    }

    if (do_ip6_bandwidth == CONFIG_BOOLEAN_YES || do_ip6_bandwidth == CONFIG_BOOLEAN_AUTO) {
        do_ip6_bandwidth = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_received = NULL,
                      *rd_sent = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "system"
                    , "ipv6"
                    , NULL
                    , "network"
                    , NULL
                    , "IPv6 Bandwidth"
                    , "kilobits/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_SYSTEM_IPV6
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rd_received = rrddim_add(st, "received", NULL,  8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
            rd_sent     = rrddim_add(st, "sent",     NULL, -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_received, Ip6InOctets);
        rrddim_set_by_pointer(st, rd_sent,     Ip6OutOctets);
        rrdset_done(st);
    }

    if (do_ip6_packets == CONFIG_BOOLEAN_YES || do_ip6_packets == CONFIG_BOOLEAN_AUTO) {
        do_ip6_packets = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_received = NULL,
                      *rd_sent = NULL,
                      *rd_forwarded = NULL,
                      *rd_delivers = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP6
                    , "packets"
                    , NULL
                    , "packets"
                    , NULL
                    , "IPv6 Packets"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV6_PACKETS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_received  = rrddim_add(st, "received",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_sent      = rrddim_add(st, "sent",      NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_forwarded = rrddim_add(st, "forwarded", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_delivers  = rrddim_add(st, "delivered", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_received,  Ip6InReceives);
        rrddim_set_by_pointer(st, rd_sent,      Ip6OutRequests);
        rrddim_set_by_pointer(st, rd_forwarded, Ip6OutForwDatagrams);
        rrddim_set_by_pointer(st, rd_delivers,  Ip6InDelivers);
        rrdset_done(st);
    }

    if (do_ip6_fragsout == CONFIG_BOOLEAN_YES || do_ip6_fragsout == CONFIG_BOOLEAN_AUTO) {
        do_ip6_fragsout = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_ok = NULL,
                      *rd_failed = NULL,
                      *rd_all = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP6
                    , "fragsout"
                    , NULL
                    , "fragments6"
                    , NULL
                    , "IPv6 Fragments Sent"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV6_FRAGSOUT
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_ok     = rrddim_add(st, "FragOKs",     "ok",      1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_failed = rrddim_add(st, "FragFails",   "failed", -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_all    = rrddim_add(st, "FragCreates", "all",     1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_ok,     Ip6FragOKs);
        rrddim_set_by_pointer(st, rd_failed, Ip6FragFails);
        rrddim_set_by_pointer(st, rd_all,    Ip6FragCreates);
        rrdset_done(st);
    }

    if (do_ip6_fragsin == CONFIG_BOOLEAN_YES || do_ip6_fragsin == CONFIG_BOOLEAN_AUTO) {
        do_ip6_fragsin = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        static RRDDIM *rd_ok = NULL,
                      *rd_failed = NULL,
                      *rd_timeout = NULL,
                      *rd_all = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP6
                    , "fragsin"
                    , NULL
                    , "fragments6"
                    , NULL
                    , "IPv6 Fragments Reassembly"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV6_FRAGSIN
                    , update_every
                    , RRDSET_TYPE_LINE);

            rd_ok      = rrddim_add(st, "ReasmOKs",     "ok",       1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_failed  = rrddim_add(st, "ReasmFails",   "failed",  -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_timeout = rrddim_add(st, "ReasmTimeout", "timeout", -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_all     = rrddim_add(st, "ReasmReqds",   "all",      1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_ok,      Ip6ReasmOKs);
        rrddim_set_by_pointer(st, rd_failed,  Ip6ReasmFails);
        rrddim_set_by_pointer(st, rd_timeout, Ip6ReasmTimeout);
        rrddim_set_by_pointer(st, rd_all,     Ip6ReasmReqds);
        rrdset_done(st);
    }

    if (do_ip6_errors == CONFIG_BOOLEAN_YES || do_ip6_errors == CONFIG_BOOLEAN_AUTO) {
        do_ip6_errors = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_InDiscards      = NULL,
                      *rd_OutDiscards     = NULL,
                      *rd_InHdrErrors     = NULL,
                      *rd_InAddrErrors    = NULL,
                      *rd_InUnknownProtos = NULL,
                      *rd_InTooBigErrors  = NULL,
                      *rd_InTruncatedPkts = NULL,
                      *rd_InNoRoutes      = NULL,
                      *rd_OutNoRoutes     = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP6
                    , "errors"
                    , NULL
                    , "errors"
                    , NULL
                    , "IPv6 Errors"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV6_ERRORS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_InDiscards      = rrddim_add(st, "InDiscards",      NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutDiscards     = rrddim_add(st, "OutDiscards",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InHdrErrors     = rrddim_add(st, "InHdrErrors",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InAddrErrors    = rrddim_add(st, "InAddrErrors",    NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InUnknownProtos = rrddim_add(st, "InUnknownProtos", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InTooBigErrors  = rrddim_add(st, "InTooBigErrors",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InTruncatedPkts = rrddim_add(st, "InTruncatedPkts", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InNoRoutes      = rrddim_add(st, "InNoRoutes",      NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutNoRoutes     = rrddim_add(st, "OutNoRoutes",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_InDiscards,      Ip6InDiscards);
        rrddim_set_by_pointer(st, rd_OutDiscards,     Ip6OutDiscards);
        rrddim_set_by_pointer(st, rd_InHdrErrors,     Ip6InHdrErrors);
        rrddim_set_by_pointer(st, rd_InAddrErrors,    Ip6InAddrErrors);
        rrddim_set_by_pointer(st, rd_InUnknownProtos, Ip6InUnknownProtos);
        rrddim_set_by_pointer(st, rd_InTooBigErrors,  Ip6InTooBigErrors);
        rrddim_set_by_pointer(st, rd_InTruncatedPkts, Ip6InTruncatedPkts);
        rrddim_set_by_pointer(st, rd_InNoRoutes,      Ip6InNoRoutes);
        rrddim_set_by_pointer(st, rd_OutNoRoutes,     Ip6OutNoRoutes);
        rrdset_done(st);
    }

    if (do_ip6_udp_packets == CONFIG_BOOLEAN_YES || do_ip6_udp_packets == CONFIG_BOOLEAN_AUTO) {
        static RRDSET *st = NULL;
        static RRDDIM *rd_received = NULL,
                      *rd_sent     = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP6
                    , "udppackets"
                    , NULL
                    , "udp6"
                    , NULL
                    , "IPv6 UDP Packets"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV6_UDP_PACKETS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_received = rrddim_add(st, "received", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_sent     = rrddim_add(st, "sent",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_received, Udp6InDatagrams);
        rrddim_set_by_pointer(st, rd_sent,     Udp6OutDatagrams);
        rrdset_done(st);
    }

    if (do_ip6_udp_errors == CONFIG_BOOLEAN_YES || do_ip6_udp_errors == CONFIG_BOOLEAN_AUTO) {
        do_ip6_udp_errors = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_RcvbufErrors = NULL,
                      *rd_SndbufErrors = NULL,
                      *rd_InErrors     = NULL,
                      *rd_NoPorts      = NULL,
                      *rd_InCsumErrors = NULL,
                      *rd_IgnoredMulti = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP6
                    , "udperrors"
                    , NULL
                    , "udp6"
                    , NULL
                    , "IPv6 UDP Errors"
                    , "events/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV6_UDP_ERRORS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_RcvbufErrors = rrddim_add(st, "RcvbufErrors", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_SndbufErrors = rrddim_add(st, "SndbufErrors", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InErrors     = rrddim_add(st, "InErrors",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_NoPorts      = rrddim_add(st, "NoPorts",      NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InCsumErrors = rrddim_add(st, "InCsumErrors", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_IgnoredMulti = rrddim_add(st, "IgnoredMulti", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_RcvbufErrors, Udp6RcvbufErrors);
        rrddim_set_by_pointer(st, rd_SndbufErrors, Udp6SndbufErrors);
        rrddim_set_by_pointer(st, rd_InErrors,     Udp6InErrors);
        rrddim_set_by_pointer(st, rd_NoPorts,      Udp6NoPorts);
        rrddim_set_by_pointer(st, rd_InCsumErrors, Udp6InCsumErrors);
        rrddim_set_by_pointer(st, rd_IgnoredMulti, Udp6IgnoredMulti);
        rrdset_done(st);
    }

    if (do_ip6_udplite_packets == CONFIG_BOOLEAN_YES || do_ip6_udplite_packets == CONFIG_BOOLEAN_AUTO) {
        static RRDSET *st = NULL;
        static RRDDIM *rd_received = NULL,
                      *rd_sent     = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP6
                    , "udplitepackets"
                    , NULL
                    , "udplite6"
                    , NULL
                    , "IPv6 UDPlite Packets"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV6_UDPLITE_PACKETS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_received = rrddim_add(st, "received", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_sent     = rrddim_add(st, "sent",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_received, UdpLite6InDatagrams);
        rrddim_set_by_pointer(st, rd_sent,     UdpLite6OutDatagrams);
        rrdset_done(st);
    }

    if (do_ip6_udplite_errors == CONFIG_BOOLEAN_YES || do_ip6_udplite_errors == CONFIG_BOOLEAN_AUTO) {
        do_ip6_udplite_errors = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_RcvbufErrors = NULL,
                      *rd_SndbufErrors = NULL,
                      *rd_InErrors     = NULL,
                      *rd_NoPorts      = NULL,
                      *rd_InCsumErrors = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP6
                    , "udpliteerrors"
                    , NULL
                    , "udplite6"
                    , NULL
                    , "IPv6 UDP Lite Errors"
                    , "events/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV6_UDPLITE_ERRORS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_RcvbufErrors = rrddim_add(st, "RcvbufErrors", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_SndbufErrors = rrddim_add(st, "SndbufErrors", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InErrors     = rrddim_add(st, "InErrors",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_NoPorts      = rrddim_add(st, "NoPorts",      NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InCsumErrors = rrddim_add(st, "InCsumErrors", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_InErrors,     UdpLite6InErrors);
        rrddim_set_by_pointer(st, rd_NoPorts,      UdpLite6NoPorts);
        rrddim_set_by_pointer(st, rd_RcvbufErrors, UdpLite6RcvbufErrors);
        rrddim_set_by_pointer(st, rd_SndbufErrors, UdpLite6SndbufErrors);
        rrddim_set_by_pointer(st, rd_InCsumErrors, UdpLite6InCsumErrors);
        rrdset_done(st);
    }

    if (do_ip6_mcast == CONFIG_BOOLEAN_YES || do_ip6_mcast == CONFIG_BOOLEAN_AUTO) {
        do_ip6_mcast = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_Ip6InMcastOctets  = NULL,
                      *rd_Ip6OutMcastOctets = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP6
                    , "mcast"
                    , NULL
                    , "multicast6"
                    , NULL
                    , "IPv6 Multicast Bandwidth"
                    , "kilobits/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV6_MCAST
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rd_Ip6InMcastOctets  = rrddim_add(st, "received", NULL,  8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
            rd_Ip6OutMcastOctets = rrddim_add(st, "sent",     NULL, -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_Ip6InMcastOctets,  Ip6InMcastOctets);
        rrddim_set_by_pointer(st, rd_Ip6OutMcastOctets, Ip6OutMcastOctets);
        rrdset_done(st);
    }

    if (do_ip6_bcast == CONFIG_BOOLEAN_YES || do_ip6_bcast == CONFIG_BOOLEAN_AUTO) {
        do_ip6_bcast = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_Ip6InBcastOctets  = NULL,
                      *rd_Ip6OutBcastOctets = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP6
                    , "bcast"
                    , NULL
                    , "broadcast6"
                    , NULL
                    , "IPv6 Broadcast Bandwidth"
                    , "kilobits/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV6_BCAST
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rd_Ip6InBcastOctets  = rrddim_add(st, "received", NULL,  8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
            rd_Ip6OutBcastOctets = rrddim_add(st, "sent",     NULL, -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_Ip6InBcastOctets,  Ip6InBcastOctets);
        rrddim_set_by_pointer(st, rd_Ip6OutBcastOctets, Ip6OutBcastOctets);
        rrdset_done(st);
    }

    if (do_ip6_mcast_p == CONFIG_BOOLEAN_YES || do_ip6_mcast_p == CONFIG_BOOLEAN_AUTO) {
        do_ip6_mcast_p = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_Ip6InMcastPkts  = NULL,
                      *rd_Ip6OutMcastPkts = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP6
                    , "mcastpkts"
                    , NULL
                    , "multicast6"
                    , NULL
                    , "IPv6 Multicast Packets"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV6_MCAST_PACKETS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_Ip6InMcastPkts  = rrddim_add(st, "received", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_Ip6OutMcastPkts = rrddim_add(st, "sent",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_Ip6InMcastPkts,  Ip6InMcastPkts);
        rrddim_set_by_pointer(st, rd_Ip6OutMcastPkts, Ip6OutMcastPkts);
        rrdset_done(st);
    }

    if (do_ip6_icmp == CONFIG_BOOLEAN_YES || do_ip6_icmp == CONFIG_BOOLEAN_AUTO) {
        do_ip6_icmp = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_Icmp6InMsgs  = NULL,
                      *rd_Icmp6OutMsgs = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP6
                    , "icmp"
                    , NULL
                    , "icmp6"
                    , NULL
                    , "IPv6 ICMP Messages"
                    , "messages/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV6_ICMP
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_Icmp6InMsgs  = rrddim_add(st, "received", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_Icmp6OutMsgs = rrddim_add(st, "sent",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_Icmp6InMsgs,  Icmp6InMsgs);
        rrddim_set_by_pointer(st, rd_Icmp6OutMsgs, Icmp6OutMsgs);
        rrdset_done(st);
    }

    if (do_ip6_icmp_redir == CONFIG_BOOLEAN_YES || do_ip6_icmp_redir == CONFIG_BOOLEAN_AUTO) {
        do_ip6_icmp_redir = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_Icmp6InRedirects  = NULL,
                      *rd_Icmp6OutRedirects = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP6
                    , "icmpredir"
                    , NULL
                    , "icmp6"
                    , NULL
                    , "IPv6 ICMP Redirects"
                    , "redirects/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV6_ICMP_REDIR
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_Icmp6InRedirects  = rrddim_add(st, "received", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_Icmp6OutRedirects = rrddim_add(st, "sent",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_Icmp6InRedirects,  Icmp6InRedirects);
        rrddim_set_by_pointer(st, rd_Icmp6OutRedirects, Icmp6OutRedirects);
        rrdset_done(st);
    }

    if (do_ip6_icmp_errors == CONFIG_BOOLEAN_YES || do_ip6_icmp_errors == CONFIG_BOOLEAN_AUTO) {
        do_ip6_icmp_errors = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_InErrors        = NULL,
                      *rd_OutErrors       = NULL,
                      *rd_InCsumErrors    = NULL,
                      *rd_InDestUnreachs  = NULL,
                      *rd_InPktTooBigs    = NULL,
                      *rd_InTimeExcds     = NULL,
                      *rd_InParmProblems  = NULL,
                      *rd_OutDestUnreachs = NULL,
                      *rd_OutPktTooBigs   = NULL,
                      *rd_OutTimeExcds    = NULL,
                      *rd_OutParmProblems = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP6
                    , "icmperrors"
                    , NULL
                    , "icmp6"
                    , NULL
                    , "IPv6 ICMP Errors"
                    , "errors/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV6_ICMP_ERRORS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_InErrors        = rrddim_add(st, "InErrors",        NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutErrors       = rrddim_add(st, "OutErrors",       NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InCsumErrors    = rrddim_add(st, "InCsumErrors",    NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InDestUnreachs  = rrddim_add(st, "InDestUnreachs",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InPktTooBigs    = rrddim_add(st, "InPktTooBigs",    NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InTimeExcds     = rrddim_add(st, "InTimeExcds",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InParmProblems  = rrddim_add(st, "InParmProblems",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutDestUnreachs = rrddim_add(st, "OutDestUnreachs", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutPktTooBigs   = rrddim_add(st, "OutPktTooBigs",   NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutTimeExcds    = rrddim_add(st, "OutTimeExcds",    NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutParmProblems = rrddim_add(st, "OutParmProblems", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_InErrors,        Icmp6InErrors);
        rrddim_set_by_pointer(st, rd_OutErrors,       Icmp6OutErrors);
        rrddim_set_by_pointer(st, rd_InCsumErrors,    Icmp6InCsumErrors);
        rrddim_set_by_pointer(st, rd_InDestUnreachs,  Icmp6InDestUnreachs);
        rrddim_set_by_pointer(st, rd_InPktTooBigs,    Icmp6InPktTooBigs);
        rrddim_set_by_pointer(st, rd_InTimeExcds,     Icmp6InTimeExcds);
        rrddim_set_by_pointer(st, rd_InParmProblems,  Icmp6InParmProblems);
        rrddim_set_by_pointer(st, rd_OutDestUnreachs, Icmp6OutDestUnreachs);
        rrddim_set_by_pointer(st, rd_OutPktTooBigs,   Icmp6OutPktTooBigs);
        rrddim_set_by_pointer(st, rd_OutTimeExcds,    Icmp6OutTimeExcds);
        rrddim_set_by_pointer(st, rd_OutParmProblems, Icmp6OutParmProblems);
        rrdset_done(st);
    }

    if (do_ip6_icmp_echos == CONFIG_BOOLEAN_YES || do_ip6_icmp_echos == CONFIG_BOOLEAN_AUTO) {
        do_ip6_icmp_echos = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_InEchos        = NULL,
                      *rd_OutEchos       = NULL,
                      *rd_InEchoReplies  = NULL,
                      *rd_OutEchoReplies = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP6
                    , "icmpechos"
                    , NULL
                    , "icmp6"
                    , NULL
                    , "IPv6 ICMP Echo"
                    , "messages/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV6_ICMP_ECHOS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_InEchos        = rrddim_add(st, "InEchos",        NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutEchos       = rrddim_add(st, "OutEchos",       NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InEchoReplies  = rrddim_add(st, "InEchoReplies",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutEchoReplies = rrddim_add(st, "OutEchoReplies", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_InEchos,        Icmp6InEchos);
        rrddim_set_by_pointer(st, rd_OutEchos,       Icmp6OutEchos);
        rrddim_set_by_pointer(st, rd_InEchoReplies,  Icmp6InEchoReplies);
        rrddim_set_by_pointer(st, rd_OutEchoReplies, Icmp6OutEchoReplies);
        rrdset_done(st);
    }

    if (do_ip6_icmp_groupmemb == CONFIG_BOOLEAN_YES || do_ip6_icmp_groupmemb == CONFIG_BOOLEAN_AUTO) {
        do_ip6_icmp_groupmemb = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_InQueries     = NULL,
                      *rd_OutQueries    = NULL,
                      *rd_InResponses   = NULL,
                      *rd_OutResponses  = NULL,
                      *rd_InReductions  = NULL,
                      *rd_OutReductions = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP6
                    , "groupmemb"
                    , NULL
                    , "icmp6"
                    , NULL
                    , "IPv6 ICMP Group Membership"
                    , "messages/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV6_ICMP_GROUPMEMB
                    , update_every
                    , RRDSET_TYPE_LINE);

            rd_InQueries     = rrddim_add(st, "InQueries",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutQueries    = rrddim_add(st, "OutQueries",    NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InResponses   = rrddim_add(st, "InResponses",   NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutResponses  = rrddim_add(st, "OutResponses",  NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InReductions  = rrddim_add(st, "InReductions",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutReductions = rrddim_add(st, "OutReductions", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_InQueries,     Icmp6InGroupMembQueries);
        rrddim_set_by_pointer(st, rd_OutQueries,    Icmp6OutGroupMembQueries);
        rrddim_set_by_pointer(st, rd_InResponses,   Icmp6InGroupMembResponses);
        rrddim_set_by_pointer(st, rd_OutResponses,  Icmp6OutGroupMembResponses);
        rrddim_set_by_pointer(st, rd_InReductions,  Icmp6InGroupMembReductions);
        rrddim_set_by_pointer(st, rd_OutReductions, Icmp6OutGroupMembReductions);
        rrdset_done(st);
    }

    if (do_ip6_icmp_router == CONFIG_BOOLEAN_YES || do_ip6_icmp_router == CONFIG_BOOLEAN_AUTO) {
        do_ip6_icmp_router = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_InSolicits        = NULL,
                      *rd_OutSolicits       = NULL,
                      *rd_InAdvertisements  = NULL,
                      *rd_OutAdvertisements = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP6
                    , "icmprouter"
                    , NULL
                    , "icmp6"
                    , NULL
                    , "IPv6 Router Messages"
                    , "messages/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV6_ICMP_ROUTER
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_InSolicits        = rrddim_add(st, "InSolicits",        NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutSolicits       = rrddim_add(st, "OutSolicits",       NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InAdvertisements  = rrddim_add(st, "InAdvertisements",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutAdvertisements = rrddim_add(st, "OutAdvertisements", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_InSolicits,        Icmp6InRouterSolicits);
        rrddim_set_by_pointer(st, rd_OutSolicits,       Icmp6OutRouterSolicits);
        rrddim_set_by_pointer(st, rd_InAdvertisements,  Icmp6InRouterAdvertisements);
        rrddim_set_by_pointer(st, rd_OutAdvertisements, Icmp6OutRouterAdvertisements);
        rrdset_done(st);
    }

    if (do_ip6_icmp_neighbor == CONFIG_BOOLEAN_YES || do_ip6_icmp_neighbor == CONFIG_BOOLEAN_AUTO) {
        do_ip6_icmp_neighbor = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_InSolicits        = NULL,
                      *rd_OutSolicits       = NULL,
                      *rd_InAdvertisements  = NULL,
                      *rd_OutAdvertisements = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP6
                    , "icmpneighbor"
                    , NULL
                    , "icmp6"
                    , NULL
                    , "IPv6 Neighbor Messages"
                    , "messages/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV6_ICMP_NEIGHBOR
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_InSolicits        = rrddim_add(st, "InSolicits",        NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutSolicits       = rrddim_add(st, "OutSolicits",       NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InAdvertisements  = rrddim_add(st, "InAdvertisements",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutAdvertisements = rrddim_add(st, "OutAdvertisements", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_InSolicits,        Icmp6InNeighborSolicits);
        rrddim_set_by_pointer(st, rd_OutSolicits,       Icmp6OutNeighborSolicits);
        rrddim_set_by_pointer(st, rd_InAdvertisements,  Icmp6InNeighborAdvertisements);
        rrddim_set_by_pointer(st, rd_OutAdvertisements, Icmp6OutNeighborAdvertisements);
        rrdset_done(st);
    }

    if (do_ip6_icmp_mldv2 == CONFIG_BOOLEAN_YES || do_ip6_icmp_mldv2 == CONFIG_BOOLEAN_AUTO) {
        do_ip6_icmp_mldv2 = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_InMLDv2Reports  = NULL,
                      *rd_OutMLDv2Reports = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP6
                    , "icmpmldv2"
                    , NULL
                    , "icmp6"
                    , NULL
                    , "IPv6 ICMP MLDv2 Reports"
                    , "reports/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV6_ICMP_LDV2
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_InMLDv2Reports  = rrddim_add(st, "received", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutMLDv2Reports = rrddim_add(st, "sent",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_InMLDv2Reports,  Icmp6InMLDv2Reports);
        rrddim_set_by_pointer(st, rd_OutMLDv2Reports, Icmp6OutMLDv2Reports);
        rrdset_done(st);
    }

    if (do_ip6_icmp_types == CONFIG_BOOLEAN_YES || do_ip6_icmp_types == CONFIG_BOOLEAN_AUTO) {
        do_ip6_icmp_types = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_InType1    = NULL,
                      *rd_InType128  = NULL,
                      *rd_InType129  = NULL,
                      *rd_InType136  = NULL,
                      *rd_OutType1   = NULL,
                      *rd_OutType128 = NULL,
                      *rd_OutType129 = NULL,
                      *rd_OutType133 = NULL,
                      *rd_OutType135 = NULL,
                      *rd_OutType143 = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP6
                    , "icmptypes"
                    , NULL
                    , "icmp6"
                    , NULL
                    , "IPv6 ICMP Types"
                    , "messages/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV6_ICMP_TYPES
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_InType1    = rrddim_add(st, "InType1",    NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InType128  = rrddim_add(st, "InType128",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InType129  = rrddim_add(st, "InType129",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InType136  = rrddim_add(st, "InType136",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutType1   = rrddim_add(st, "OutType1",   NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutType128 = rrddim_add(st, "OutType128", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutType129 = rrddim_add(st, "OutType129", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutType133 = rrddim_add(st, "OutType133", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutType135 = rrddim_add(st, "OutType135", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutType143 = rrddim_add(st, "OutType143", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_InType1,    Icmp6InType1);
        rrddim_set_by_pointer(st, rd_InType128,  Icmp6InType128);
        rrddim_set_by_pointer(st, rd_InType129,  Icmp6InType129);
        rrddim_set_by_pointer(st, rd_InType136,  Icmp6InType136);
        rrddim_set_by_pointer(st, rd_OutType1,   Icmp6OutType1);
        rrddim_set_by_pointer(st, rd_OutType128, Icmp6OutType128);
        rrddim_set_by_pointer(st, rd_OutType129, Icmp6OutType129);
        rrddim_set_by_pointer(st, rd_OutType133, Icmp6OutType133);
        rrddim_set_by_pointer(st, rd_OutType135, Icmp6OutType135);
        rrddim_set_by_pointer(st, rd_OutType143, Icmp6OutType143);
        rrdset_done(st);
    }

    if (do_ip6_ect == CONFIG_BOOLEAN_YES || do_ip6_ect == CONFIG_BOOLEAN_AUTO) {
        do_ip6_ect = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_InNoECTPkts = NULL, *rd_InECT1Pkts = NULL, *rd_InECT0Pkts = NULL, *rd_InCEPkts = NULL;

        if (unlikely(!st)) {
            st = rrdset_create_localhost(
                RRD_TYPE_NET_IP6,
                "ect",
                NULL,
                "packets",
                NULL,
                "IPv6 ECT Packets",
                "packets/s",
                PLUGIN_PROC_NAME,
                PLUGIN_PROC_MODULE_NETSTAT_NAME,
                NETDATA_CHART_PRIO_IPV6_ECT,
                update_every,
                RRDSET_TYPE_LINE);

            rd_InNoECTPkts = rrddim_add(st, "InNoECTPkts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InECT1Pkts = rrddim_add(st, "InECT1Pkts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InECT0Pkts = rrddim_add(st, "InECT0Pkts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InCEPkts = rrddim_add(st, "InCEPkts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_InNoECTPkts, Ip6InNoECTPkts);
        rrddim_set_by_pointer(st, rd_InECT1Pkts, Ip6InECT1Pkts);
        rrddim_set_by_pointer(st, rd_InECT0Pkts, Ip6InECT0Pkts);
        rrddim_set_by_pointer(st, rd_InCEPkts, Ip6InCEPkts);
        rrdset_done(st);
    }
}

int do_proc_net_netstat(int update_every, usec_t dt) {
    (void)dt;

    static int do_bandwidth = -1, do_inerrors = -1, do_mcast = -1, do_bcast = -1, do_mcast_p = -1, do_bcast_p = -1, do_ecn = -1, \
        do_tcpext_reorder = -1, do_tcpext_syscookies = -1, do_tcpext_ofo = -1, do_tcpext_connaborts = -1, do_tcpext_memory = -1,
        do_tcpext_syn_queue = -1, do_tcpext_accept_queue = -1;

    static int do_ip_packets = -1, do_ip_fragsout = -1, do_ip_fragsin = -1, do_ip_errors = -1,
        do_tcp_sockets = -1, do_tcp_packets = -1, do_tcp_errors = -1, do_tcp_handshake = -1, do_tcp_opens = -1,
        do_udp_packets = -1, do_udp_errors = -1, do_icmp_packets = -1, do_icmpmsg = -1, do_udplite_packets = -1;

    static uint32_t hash_ipext = 0, hash_tcpext = 0;
    static uint32_t hash_ip = 0, hash_icmp = 0, hash_tcp = 0, hash_udp = 0, hash_icmpmsg = 0, hash_udplite = 0;

    static procfile *ff_netstat = NULL;
    static procfile *ff_snmp = NULL;

    static ARL_BASE *arl_tcpext = NULL;
    static ARL_BASE *arl_ipext = NULL;

    static ARL_BASE *arl_ip = NULL;
    static ARL_BASE *arl_icmp = NULL;
    static ARL_BASE *arl_icmpmsg = NULL;
    static ARL_BASE *arl_tcp = NULL;
    static ARL_BASE *arl_udp = NULL;
    static ARL_BASE *arl_udplite = NULL;

    static const RRDVAR_ACQUIRED *tcp_max_connections_var = NULL;

    // --------------------------------------------------------------------
    // IP

    // IP bandwidth
    static unsigned long long ipext_InOctets = 0;
    static unsigned long long ipext_OutOctets = 0;

    // IP input errors
    static unsigned long long ipext_InNoRoutes = 0;
    static unsigned long long ipext_InTruncatedPkts = 0;
    static unsigned long long ipext_InCsumErrors = 0;

    // IP multicast bandwidth
    static unsigned long long ipext_InMcastOctets = 0;
    static unsigned long long ipext_OutMcastOctets = 0;

    // IP multicast packets
    static unsigned long long ipext_InMcastPkts = 0;
    static unsigned long long ipext_OutMcastPkts = 0;

    // IP broadcast bandwidth
    static unsigned long long ipext_InBcastOctets = 0;
    static unsigned long long ipext_OutBcastOctets = 0;

    // IP broadcast packets
    static unsigned long long ipext_InBcastPkts = 0;
    static unsigned long long ipext_OutBcastPkts = 0;

    // IP ECN
    static unsigned long long ipext_InNoECTPkts = 0;
    static unsigned long long ipext_InECT1Pkts = 0;
    static unsigned long long ipext_InECT0Pkts = 0;
    static unsigned long long ipext_InCEPkts = 0;

    // --------------------------------------------------------------------
    // IP TCP

    // IP TCP Reordering
    static unsigned long long tcpext_TCPRenoReorder = 0;
    static unsigned long long tcpext_TCPFACKReorder = 0;
    static unsigned long long tcpext_TCPSACKReorder = 0;
    static unsigned long long tcpext_TCPTSReorder = 0;

    // IP TCP SYN Cookies
    static unsigned long long tcpext_SyncookiesSent = 0;
    static unsigned long long tcpext_SyncookiesRecv = 0;
    static unsigned long long tcpext_SyncookiesFailed = 0;

    // IP TCP Out Of Order Queue
    // http://www.spinics.net/lists/netdev/msg204696.html
    static unsigned long long tcpext_TCPOFOQueue = 0; // Number of packets queued in OFO queue
    static unsigned long long tcpext_TCPOFODrop = 0;  // Number of packets meant to be queued in OFO but dropped because socket rcvbuf limit hit.
    static unsigned long long tcpext_TCPOFOMerge = 0; // Number of packets in OFO that were merged with other packets.
    static unsigned long long tcpext_OfoPruned = 0;   // packets dropped from out-of-order queue because of socket buffer overrun

    // IP TCP connection resets
    // https://github.com/ecki/net-tools/blob/bd8bceaed2311651710331a7f8990c3e31be9840/statistics.c
    static unsigned long long tcpext_TCPAbortOnData = 0;    // connections reset due to unexpected data
    static unsigned long long tcpext_TCPAbortOnClose = 0;   // connections reset due to early user close
    static unsigned long long tcpext_TCPAbortOnMemory = 0;  // connections aborted due to memory pressure
    static unsigned long long tcpext_TCPAbortOnTimeout = 0; // connections aborted due to timeout
    static unsigned long long tcpext_TCPAbortOnLinger = 0;  // connections aborted after user close in linger timeout
    static unsigned long long tcpext_TCPAbortFailed = 0;    // times unable to send RST due to no memory

    // https://perfchron.com/2015/12/26/investigating-linux-network-issues-with-netstat-and-nstat/
    static unsigned long long tcpext_ListenOverflows = 0;   // times the listen queue of a socket overflowed
    static unsigned long long tcpext_ListenDrops = 0;       // SYNs to LISTEN sockets ignored

    // IP TCP memory pressures
    static unsigned long long tcpext_TCPMemoryPressures = 0;

    static unsigned long long tcpext_TCPReqQFullDrop = 0;
    static unsigned long long tcpext_TCPReqQFullDoCookies = 0;

    static unsigned long long tcpext_TCPSynRetrans = 0;

    // prepare for /proc/net/netstat parsing

    if(unlikely(!arl_ipext)) {
        hash_ipext = simple_hash("IpExt");
        hash_tcpext = simple_hash("TcpExt");

        do_bandwidth = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_NETSTAT, "bandwidth", CONFIG_BOOLEAN_AUTO);
        do_inerrors  = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_NETSTAT, "input errors", CONFIG_BOOLEAN_AUTO);
        do_mcast     = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_NETSTAT, "multicast bandwidth", CONFIG_BOOLEAN_AUTO);
        do_bcast     = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_NETSTAT, "broadcast bandwidth", CONFIG_BOOLEAN_AUTO);
        do_mcast_p   = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_NETSTAT, "multicast packets", CONFIG_BOOLEAN_AUTO);
        do_bcast_p   = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_NETSTAT, "broadcast packets", CONFIG_BOOLEAN_AUTO);
        do_ecn       = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_NETSTAT, "ECN packets", CONFIG_BOOLEAN_AUTO);

        do_tcpext_reorder    = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_NETSTAT, "TCP reorders", CONFIG_BOOLEAN_AUTO);
        do_tcpext_syscookies = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_NETSTAT, "TCP SYN cookies", CONFIG_BOOLEAN_AUTO);
        do_tcpext_ofo        = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_NETSTAT, "TCP out-of-order queue", CONFIG_BOOLEAN_AUTO);
        do_tcpext_connaborts = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_NETSTAT, "TCP connection aborts", CONFIG_BOOLEAN_AUTO);
        do_tcpext_memory     = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_NETSTAT, "TCP memory pressures", CONFIG_BOOLEAN_AUTO);

        do_tcpext_syn_queue    = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_NETSTAT, "TCP SYN queue", CONFIG_BOOLEAN_AUTO);
        do_tcpext_accept_queue = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_NETSTAT, "TCP accept queue", CONFIG_BOOLEAN_AUTO);

        arl_ipext  = arl_create("netstat/ipext", NULL, 60);
        arl_tcpext = arl_create("netstat/tcpext", NULL, 60);

        // --------------------------------------------------------------------
        // IP

        if(do_bandwidth != CONFIG_BOOLEAN_NO) {
            arl_expect(arl_ipext, "InOctets",  &ipext_InOctets);
            arl_expect(arl_ipext, "OutOctets", &ipext_OutOctets);
        }

        if(do_inerrors != CONFIG_BOOLEAN_NO) {
            arl_expect(arl_ipext, "InNoRoutes",      &ipext_InNoRoutes);
            arl_expect(arl_ipext, "InTruncatedPkts", &ipext_InTruncatedPkts);
            arl_expect(arl_ipext, "InCsumErrors",    &ipext_InCsumErrors);
        }

        if(do_mcast != CONFIG_BOOLEAN_NO) {
            arl_expect(arl_ipext, "InMcastOctets", &ipext_InMcastOctets);
            arl_expect(arl_ipext, "OutMcastOctets", &ipext_OutMcastOctets);
        }

        if(do_mcast_p != CONFIG_BOOLEAN_NO) {
            arl_expect(arl_ipext, "InMcastPkts",  &ipext_InMcastPkts);
            arl_expect(arl_ipext, "OutMcastPkts", &ipext_OutMcastPkts);
        }

        if(do_bcast != CONFIG_BOOLEAN_NO) {
            arl_expect(arl_ipext, "InBcastPkts",  &ipext_InBcastPkts);
            arl_expect(arl_ipext, "OutBcastPkts", &ipext_OutBcastPkts);
        }

        if(do_bcast_p != CONFIG_BOOLEAN_NO) {
            arl_expect(arl_ipext, "InBcastOctets",  &ipext_InBcastOctets);
            arl_expect(arl_ipext, "OutBcastOctets", &ipext_OutBcastOctets);
        }

        if(do_ecn != CONFIG_BOOLEAN_NO) {
            arl_expect(arl_ipext, "InNoECTPkts", &ipext_InNoECTPkts);
            arl_expect(arl_ipext, "InECT1Pkts",  &ipext_InECT1Pkts);
            arl_expect(arl_ipext, "InECT0Pkts",  &ipext_InECT0Pkts);
            arl_expect(arl_ipext, "InCEPkts",    &ipext_InCEPkts);
        }

        // --------------------------------------------------------------------
        // IP TCP

        if(do_tcpext_reorder != CONFIG_BOOLEAN_NO) {
            arl_expect(arl_tcpext, "TCPFACKReorder", &tcpext_TCPFACKReorder);
            arl_expect(arl_tcpext, "TCPSACKReorder", &tcpext_TCPSACKReorder);
            arl_expect(arl_tcpext, "TCPRenoReorder", &tcpext_TCPRenoReorder);
            arl_expect(arl_tcpext, "TCPTSReorder",   &tcpext_TCPTSReorder);
        }

        if(do_tcpext_syscookies != CONFIG_BOOLEAN_NO) {
            arl_expect(arl_tcpext, "SyncookiesSent",   &tcpext_SyncookiesSent);
            arl_expect(arl_tcpext, "SyncookiesRecv",   &tcpext_SyncookiesRecv);
            arl_expect(arl_tcpext, "SyncookiesFailed", &tcpext_SyncookiesFailed);
        }

        if(do_tcpext_ofo != CONFIG_BOOLEAN_NO) {
            arl_expect(arl_tcpext, "TCPOFOQueue", &tcpext_TCPOFOQueue);
            arl_expect(arl_tcpext, "TCPOFODrop",  &tcpext_TCPOFODrop);
            arl_expect(arl_tcpext, "TCPOFOMerge", &tcpext_TCPOFOMerge);
            arl_expect(arl_tcpext, "OfoPruned",   &tcpext_OfoPruned);
        }

        if(do_tcpext_connaborts != CONFIG_BOOLEAN_NO) {
            arl_expect(arl_tcpext, "TCPAbortOnData",    &tcpext_TCPAbortOnData);
            arl_expect(arl_tcpext, "TCPAbortOnClose",   &tcpext_TCPAbortOnClose);
            arl_expect(arl_tcpext, "TCPAbortOnMemory",  &tcpext_TCPAbortOnMemory);
            arl_expect(arl_tcpext, "TCPAbortOnTimeout", &tcpext_TCPAbortOnTimeout);
            arl_expect(arl_tcpext, "TCPAbortOnLinger",  &tcpext_TCPAbortOnLinger);
            arl_expect(arl_tcpext, "TCPAbortFailed",    &tcpext_TCPAbortFailed);
        }

        if(do_tcpext_memory != CONFIG_BOOLEAN_NO) {
            arl_expect(arl_tcpext, "TCPMemoryPressures", &tcpext_TCPMemoryPressures);
        }

        if(do_tcpext_accept_queue != CONFIG_BOOLEAN_NO) {
            arl_expect(arl_tcpext, "ListenOverflows", &tcpext_ListenOverflows);
            arl_expect(arl_tcpext, "ListenDrops",     &tcpext_ListenDrops);
        }

        if(do_tcpext_syn_queue != CONFIG_BOOLEAN_NO) {
            arl_expect(arl_tcpext, "TCPReqQFullDrop",      &tcpext_TCPReqQFullDrop);
            arl_expect(arl_tcpext, "TCPReqQFullDoCookies", &tcpext_TCPReqQFullDoCookies);
        }

        arl_expect(arl_tcpext, "TCPSynRetrans", &tcpext_TCPSynRetrans);
    }

    // prepare for /proc/net/snmp parsing

    if(unlikely(!arl_ip)) {
        do_ip_packets       = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp", "ipv4 packets", CONFIG_BOOLEAN_AUTO);
        do_ip_fragsout      = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp", "ipv4 fragments sent", CONFIG_BOOLEAN_AUTO);
        do_ip_fragsin       = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp", "ipv4 fragments assembly", CONFIG_BOOLEAN_AUTO);
        do_ip_errors        = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp", "ipv4 errors", CONFIG_BOOLEAN_AUTO);
        do_tcp_sockets      = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp", "ipv4 TCP connections", CONFIG_BOOLEAN_AUTO);
        do_tcp_packets      = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp", "ipv4 TCP packets", CONFIG_BOOLEAN_AUTO);
        do_tcp_errors       = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp", "ipv4 TCP errors", CONFIG_BOOLEAN_AUTO);
        do_tcp_opens        = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp", "ipv4 TCP opens", CONFIG_BOOLEAN_AUTO);
        do_tcp_handshake    = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp", "ipv4 TCP handshake issues", CONFIG_BOOLEAN_AUTO);
        do_udp_packets      = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp", "ipv4 UDP packets", CONFIG_BOOLEAN_AUTO);
        do_udp_errors       = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp", "ipv4 UDP errors", CONFIG_BOOLEAN_AUTO);
        do_icmp_packets     = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp", "ipv4 ICMP packets", CONFIG_BOOLEAN_AUTO);
        do_icmpmsg          = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp", "ipv4 ICMP messages", CONFIG_BOOLEAN_AUTO);
        do_udplite_packets  = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/net/snmp", "ipv4 UDPLite packets", CONFIG_BOOLEAN_AUTO);

        hash_ip = simple_hash("Ip");
        hash_tcp = simple_hash("Tcp");
        hash_udp = simple_hash("Udp");
        hash_icmp = simple_hash("Icmp");
        hash_icmpmsg = simple_hash("IcmpMsg");
        hash_udplite = simple_hash("UdpLite");

        arl_ip = arl_create("snmp/Ip", arl_callback_str2kernel_uint_t, 60);
        // arl_expect(arl_ip, "Forwarding", &snmp_root.ip_Forwarding);
        arl_expect(arl_ip, "DefaultTTL", &snmp_root.ip_DefaultTTL);
        arl_expect(arl_ip, "InReceives", &snmp_root.ip_InReceives);
        arl_expect(arl_ip, "InHdrErrors", &snmp_root.ip_InHdrErrors);
        arl_expect(arl_ip, "InAddrErrors", &snmp_root.ip_InAddrErrors);
        arl_expect(arl_ip, "ForwDatagrams", &snmp_root.ip_ForwDatagrams);
        arl_expect(arl_ip, "InUnknownProtos", &snmp_root.ip_InUnknownProtos);
        arl_expect(arl_ip, "InDiscards", &snmp_root.ip_InDiscards);
        arl_expect(arl_ip, "InDelivers", &snmp_root.ip_InDelivers);
        arl_expect(arl_ip, "OutRequests", &snmp_root.ip_OutRequests);
        arl_expect(arl_ip, "OutDiscards", &snmp_root.ip_OutDiscards);
        arl_expect(arl_ip, "OutNoRoutes", &snmp_root.ip_OutNoRoutes);
        arl_expect(arl_ip, "ReasmTimeout", &snmp_root.ip_ReasmTimeout);
        arl_expect(arl_ip, "ReasmReqds", &snmp_root.ip_ReasmReqds);
        arl_expect(arl_ip, "ReasmOKs", &snmp_root.ip_ReasmOKs);
        arl_expect(arl_ip, "ReasmFails", &snmp_root.ip_ReasmFails);
        arl_expect(arl_ip, "FragOKs", &snmp_root.ip_FragOKs);
        arl_expect(arl_ip, "FragFails", &snmp_root.ip_FragFails);
        arl_expect(arl_ip, "FragCreates", &snmp_root.ip_FragCreates);

        arl_icmp = arl_create("snmp/Icmp", arl_callback_str2kernel_uint_t, 60);
        arl_expect(arl_icmp, "InMsgs", &snmp_root.icmp_InMsgs);
        arl_expect(arl_icmp, "OutMsgs", &snmp_root.icmp_OutMsgs);
        arl_expect(arl_icmp, "InErrors", &snmp_root.icmp_InErrors);
        arl_expect(arl_icmp, "OutErrors", &snmp_root.icmp_OutErrors);
        arl_expect(arl_icmp, "InCsumErrors", &snmp_root.icmp_InCsumErrors);

        arl_icmpmsg = arl_create("snmp/Icmpmsg", arl_callback_str2kernel_uint_t, 60);
        arl_expect(arl_icmpmsg, "InType0", &snmp_root.icmpmsg_InEchoReps);
        arl_expect(arl_icmpmsg, "OutType0", &snmp_root.icmpmsg_OutEchoReps);
        arl_expect(arl_icmpmsg, "InType3", &snmp_root.icmpmsg_InDestUnreachs);
        arl_expect(arl_icmpmsg, "OutType3", &snmp_root.icmpmsg_OutDestUnreachs);
        arl_expect(arl_icmpmsg, "InType5", &snmp_root.icmpmsg_InRedirects);
        arl_expect(arl_icmpmsg, "OutType5", &snmp_root.icmpmsg_OutRedirects);
        arl_expect(arl_icmpmsg, "InType8", &snmp_root.icmpmsg_InEchos);
        arl_expect(arl_icmpmsg, "OutType8", &snmp_root.icmpmsg_OutEchos);
        arl_expect(arl_icmpmsg, "InType9", &snmp_root.icmpmsg_InRouterAdvert);
        arl_expect(arl_icmpmsg, "OutType9", &snmp_root.icmpmsg_OutRouterAdvert);
        arl_expect(arl_icmpmsg, "InType10", &snmp_root.icmpmsg_InRouterSelect);
        arl_expect(arl_icmpmsg, "OutType10", &snmp_root.icmpmsg_OutRouterSelect);
        arl_expect(arl_icmpmsg, "InType11", &snmp_root.icmpmsg_InTimeExcds);
        arl_expect(arl_icmpmsg, "OutType11", &snmp_root.icmpmsg_OutTimeExcds);
        arl_expect(arl_icmpmsg, "InType12", &snmp_root.icmpmsg_InParmProbs);
        arl_expect(arl_icmpmsg, "OutType12", &snmp_root.icmpmsg_OutParmProbs);
        arl_expect(arl_icmpmsg, "InType13", &snmp_root.icmpmsg_InTimestamps);
        arl_expect(arl_icmpmsg, "OutType13", &snmp_root.icmpmsg_OutTimestamps);
        arl_expect(arl_icmpmsg, "InType14", &snmp_root.icmpmsg_InTimestampReps);
        arl_expect(arl_icmpmsg, "OutType14", &snmp_root.icmpmsg_OutTimestampReps);

        arl_tcp = arl_create("snmp/Tcp", arl_callback_str2kernel_uint_t, 60);
        // arl_expect(arl_tcp, "RtoAlgorithm", &snmp_root.tcp_RtoAlgorithm);
        // arl_expect(arl_tcp, "RtoMin", &snmp_root.tcp_RtoMin);
        // arl_expect(arl_tcp, "RtoMax", &snmp_root.tcp_RtoMax);
        arl_expect_custom(arl_tcp, "MaxConn", arl_callback_ssize_t, &snmp_root.tcp_MaxConn);
        arl_expect(arl_tcp, "ActiveOpens", &snmp_root.tcp_ActiveOpens);
        arl_expect(arl_tcp, "PassiveOpens", &snmp_root.tcp_PassiveOpens);
        arl_expect(arl_tcp, "AttemptFails", &snmp_root.tcp_AttemptFails);
        arl_expect(arl_tcp, "EstabResets", &snmp_root.tcp_EstabResets);
        arl_expect(arl_tcp, "CurrEstab", &snmp_root.tcp_CurrEstab);
        arl_expect(arl_tcp, "InSegs", &snmp_root.tcp_InSegs);
        arl_expect(arl_tcp, "OutSegs", &snmp_root.tcp_OutSegs);
        arl_expect(arl_tcp, "RetransSegs", &snmp_root.tcp_RetransSegs);
        arl_expect(arl_tcp, "InErrs", &snmp_root.tcp_InErrs);
        arl_expect(arl_tcp, "OutRsts", &snmp_root.tcp_OutRsts);
        arl_expect(arl_tcp, "InCsumErrors", &snmp_root.tcp_InCsumErrors);

        arl_udp = arl_create("snmp/Udp", arl_callback_str2kernel_uint_t, 60);
        arl_expect(arl_udp, "InDatagrams", &snmp_root.udp_InDatagrams);
        arl_expect(arl_udp, "NoPorts", &snmp_root.udp_NoPorts);
        arl_expect(arl_udp, "InErrors", &snmp_root.udp_InErrors);
        arl_expect(arl_udp, "OutDatagrams", &snmp_root.udp_OutDatagrams);
        arl_expect(arl_udp, "RcvbufErrors", &snmp_root.udp_RcvbufErrors);
        arl_expect(arl_udp, "SndbufErrors", &snmp_root.udp_SndbufErrors);
        arl_expect(arl_udp, "InCsumErrors", &snmp_root.udp_InCsumErrors);
        arl_expect(arl_udp, "IgnoredMulti", &snmp_root.udp_IgnoredMulti);

        arl_udplite = arl_create("snmp/Udplite", arl_callback_str2kernel_uint_t, 60);
        arl_expect(arl_udplite, "InDatagrams", &snmp_root.udplite_InDatagrams);
        arl_expect(arl_udplite, "NoPorts", &snmp_root.udplite_NoPorts);
        arl_expect(arl_udplite, "InErrors", &snmp_root.udplite_InErrors);
        arl_expect(arl_udplite, "OutDatagrams", &snmp_root.udplite_OutDatagrams);
        arl_expect(arl_udplite, "RcvbufErrors", &snmp_root.udplite_RcvbufErrors);
        arl_expect(arl_udplite, "SndbufErrors", &snmp_root.udplite_SndbufErrors);
        arl_expect(arl_udplite, "InCsumErrors", &snmp_root.udplite_InCsumErrors);
        arl_expect(arl_udplite, "IgnoredMulti", &snmp_root.udplite_IgnoredMulti);

        tcp_max_connections_var = rrdvar_host_variable_add_and_acquire(localhost, "tcp_max_connections");
    }

    size_t lines, l, words;

    // parse /proc/net/netstat

    if(unlikely(!ff_netstat)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/net/netstat");
        ff_netstat = procfile_open(inicfg_get(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_NETSTAT, "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff_netstat)) return 1;
    }

    ff_netstat = procfile_readall(ff_netstat);
    if(unlikely(!ff_netstat)) return 0; // we return 0, so that we will retry to open it next time

    lines = procfile_lines(ff_netstat);

    arl_begin(arl_ipext);
    arl_begin(arl_tcpext);

    for(l = 0; l < lines ;l++) {
        char *key = procfile_lineword(ff_netstat, l, 0);
        uint32_t hash = simple_hash(key);

        if(unlikely(hash == hash_ipext && strcmp(key, "IpExt") == 0)) {
            size_t h = l++;

            words = procfile_linewords(ff_netstat, l);
            if(unlikely(words < 2)) {
                collector_error("Cannot read /proc/net/netstat IpExt line. Expected 2+ params, read %zu.", words);
                continue;
            }

            parse_line_pair(ff_netstat, arl_ipext, h, l);

        }
        else if(unlikely(hash == hash_tcpext && strcmp(key, "TcpExt") == 0)) {
            size_t h = l++;

            words = procfile_linewords(ff_netstat, l);
            if(unlikely(words < 2)) {
                collector_error("Cannot read /proc/net/netstat TcpExt line. Expected 2+ params, read %zu.", words);
                continue;
            }

            parse_line_pair(ff_netstat, arl_tcpext, h, l);
        }
    }

    // parse /proc/net/snmp

    if(unlikely(!ff_snmp)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/net/snmp");
        ff_snmp = procfile_open(inicfg_get(&netdata_config, "plugin:proc:/proc/net/snmp", "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff_snmp)) return 1;
    }

    ff_snmp = procfile_readall(ff_snmp);
    if(unlikely(!ff_snmp)) return 0; // we return 0, so that we will retry to open it next time

    lines = procfile_lines(ff_snmp);
    size_t w;

    for(l = 0; l < lines ;l++) {
        char *key = procfile_lineword(ff_snmp, l, 0);
        uint32_t hash = simple_hash(key);

        if(unlikely(hash == hash_ip && strcmp(key, "Ip") == 0)) {
            size_t h = l++;

            if(strcmp(procfile_lineword(ff_snmp, l, 0), "Ip") != 0) {
                collector_error("Cannot read Ip line from /proc/net/snmp.");
                break;
            }

            words = procfile_linewords(ff_snmp, l);
            if(words < 3) {
                collector_error("Cannot read /proc/net/snmp Ip line. Expected 3+ params, read %zu.", words);
                continue;
            }

            arl_begin(arl_ip);
            for(w = 1; w < words ; w++) {
                if (unlikely(arl_check(arl_ip, procfile_lineword(ff_snmp, h, w), procfile_lineword(ff_snmp, l, w)) != 0))
                    break;
            }
        }
        else if(unlikely(hash == hash_icmp && strcmp(key, "Icmp") == 0)) {
            size_t h = l++;

            if(strcmp(procfile_lineword(ff_snmp, l, 0), "Icmp") != 0) {
                collector_error("Cannot read Icmp line from /proc/net/snmp.");
                break;
            }

            words = procfile_linewords(ff_snmp, l);
            if(words < 3) {
                collector_error("Cannot read /proc/net/snmp Icmp line. Expected 3+ params, read %zu.", words);
                continue;
            }

            arl_begin(arl_icmp);
            for(w = 1; w < words ; w++) {
                if (unlikely(arl_check(arl_icmp, procfile_lineword(ff_snmp, h, w), procfile_lineword(ff_snmp, l, w)) != 0))
                    break;
            }
        }
        else if(unlikely(hash == hash_icmpmsg && strcmp(key, "IcmpMsg") == 0)) {
            size_t h = l++;

            if(strcmp(procfile_lineword(ff_snmp, l, 0), "IcmpMsg") != 0) {
                collector_error("Cannot read IcmpMsg line from /proc/net/snmp.");
                break;
            }

            words = procfile_linewords(ff_snmp, l);
            if(words < 2) {
                collector_error("Cannot read /proc/net/snmp IcmpMsg line. Expected 2+ params, read %zu.", words);
                continue;
            }

            arl_begin(arl_icmpmsg);
            for(w = 1; w < words ; w++) {
                if (unlikely(arl_check(arl_icmpmsg, procfile_lineword(ff_snmp, h, w), procfile_lineword(ff_snmp, l, w)) != 0))
                    break;
            }
        }
        else if(unlikely(hash == hash_tcp && strcmp(key, "Tcp") == 0)) {
            size_t h = l++;

            if(strcmp(procfile_lineword(ff_snmp, l, 0), "Tcp") != 0) {
                collector_error("Cannot read Tcp line from /proc/net/snmp.");
                break;
            }

            words = procfile_linewords(ff_snmp, l);
            if(words < 3) {
                collector_error("Cannot read /proc/net/snmp Tcp line. Expected 3+ params, read %zu.", words);
                continue;
            }

            arl_begin(arl_tcp);
            for(w = 1; w < words ; w++) {
                if (unlikely(arl_check(arl_tcp, procfile_lineword(ff_snmp, h, w), procfile_lineword(ff_snmp, l, w)) != 0))
                    break;
            }
        }
        else if(unlikely(hash == hash_udp && strcmp(key, "Udp") == 0)) {
            size_t h = l++;

            if(strcmp(procfile_lineword(ff_snmp, l, 0), "Udp") != 0) {
                collector_error("Cannot read Udp line from /proc/net/snmp.");
                break;
            }

            words = procfile_linewords(ff_snmp, l);
            if(words < 3) {
                collector_error("Cannot read /proc/net/snmp Udp line. Expected 3+ params, read %zu.", words);
                continue;
            }

            arl_begin(arl_udp);
            for(w = 1; w < words ; w++) {
                if (unlikely(arl_check(arl_udp, procfile_lineword(ff_snmp, h, w), procfile_lineword(ff_snmp, l, w)) != 0))
                    break;
            }
        }
        else if(unlikely(hash == hash_udplite && strcmp(key, "UdpLite") == 0)) {
            size_t h = l++;

            if(strcmp(procfile_lineword(ff_snmp, l, 0), "UdpLite") != 0) {
                collector_error("Cannot read UdpLite line from /proc/net/snmp.");
                break;
            }

            words = procfile_linewords(ff_snmp, l);
            if(words < 3) {
                collector_error("Cannot read /proc/net/snmp UdpLite line. Expected 3+ params, read %zu.", words);
                continue;
            }

            arl_begin(arl_udplite);
            for(w = 1; w < words ; w++) {
                if (unlikely(arl_check(arl_udplite, procfile_lineword(ff_snmp, h, w), procfile_lineword(ff_snmp, l, w)) != 0))
                    break;
            }
        }
    }

    // netstat IpExt charts

    if (do_bandwidth == CONFIG_BOOLEAN_YES || do_bandwidth == CONFIG_BOOLEAN_AUTO) {
        do_bandwidth = CONFIG_BOOLEAN_YES;
        static RRDSET *st_system_ip = NULL;
        static RRDDIM *rd_in = NULL, *rd_out = NULL;

        if(unlikely(!st_system_ip)) {
            st_system_ip = rrdset_create_localhost(
                    "system"
                    , "ip" // FIXME: this is ipv4. Not changing it because it will require to do changes in cloud-frontend too
                    , NULL
                    , "network"
                    , NULL
                    , "IPv4 Bandwidth"
                    , "kilobits/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_SYSTEM_IP
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rd_in  = rrddim_add(st_system_ip, "received", NULL,  8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
            rd_out = rrddim_add(st_system_ip, "sent",     NULL, -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_system_ip, rd_in,  ipext_InOctets);
        rrddim_set_by_pointer(st_system_ip, rd_out, ipext_OutOctets);
        rrdset_done(st_system_ip);
    }

    if (do_mcast == CONFIG_BOOLEAN_YES || do_mcast == CONFIG_BOOLEAN_AUTO) {
        do_mcast = CONFIG_BOOLEAN_YES;
        static RRDSET *st_ip_mcast = NULL;
        static RRDDIM *rd_in = NULL, *rd_out = NULL;

        if(unlikely(!st_ip_mcast)) {
            st_ip_mcast = rrdset_create_localhost(
                    RRD_TYPE_NET_IP4
                    , "mcast"
                    , NULL
                    , "multicast"
                    , NULL
                    , "IP Multicast Bandwidth"
                    , "kilobits/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV4_MCAST
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rd_in  = rrddim_add(st_ip_mcast, "received", NULL,  8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
            rd_out = rrddim_add(st_ip_mcast, "sent",     NULL, -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_ip_mcast, rd_in,  ipext_InMcastOctets);
        rrddim_set_by_pointer(st_ip_mcast, rd_out, ipext_OutMcastOctets);

        rrdset_done(st_ip_mcast);
    }

    // --------------------------------------------------------------------

    if (do_bcast == CONFIG_BOOLEAN_YES || do_bcast == CONFIG_BOOLEAN_AUTO) {
        do_bcast = CONFIG_BOOLEAN_YES;

        static RRDSET *st_ip_bcast = NULL;
        static RRDDIM *rd_in = NULL, *rd_out = NULL;

        if(unlikely(!st_ip_bcast)) {
            st_ip_bcast = rrdset_create_localhost(
                    RRD_TYPE_NET_IP4
                    , "bcast"
                    , NULL
                    , "broadcast"
                    , NULL
                    , "IPv4 Broadcast Bandwidth"
                    , "kilobits/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV4_BCAST
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rd_in  = rrddim_add(st_ip_bcast, "received", NULL,  8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
            rd_out = rrddim_add(st_ip_bcast, "sent",     NULL, -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_ip_bcast, rd_in,  ipext_InBcastOctets);
        rrddim_set_by_pointer(st_ip_bcast, rd_out, ipext_OutBcastOctets);

        rrdset_done(st_ip_bcast);
    }

    // --------------------------------------------------------------------

    if (do_mcast_p == CONFIG_BOOLEAN_YES || do_mcast_p == CONFIG_BOOLEAN_AUTO) {
        do_mcast_p = CONFIG_BOOLEAN_YES;

        static RRDSET *st_ip_mcastpkts = NULL;
        static RRDDIM *rd_in = NULL, *rd_out = NULL;

        if(unlikely(!st_ip_mcastpkts)) {
            st_ip_mcastpkts = rrdset_create_localhost(
                    RRD_TYPE_NET_IP4
                    , "mcastpkts"
                    , NULL
                    , "multicast"
                    , NULL
                    , "IPv4 Multicast Packets"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV4_MCAST_PACKETS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_in  = rrddim_add(st_ip_mcastpkts, "received", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_out = rrddim_add(st_ip_mcastpkts, "sent",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_ip_mcastpkts, rd_in,  ipext_InMcastPkts);
        rrddim_set_by_pointer(st_ip_mcastpkts, rd_out, ipext_OutMcastPkts);
        rrdset_done(st_ip_mcastpkts);
    }

    if (do_bcast_p == CONFIG_BOOLEAN_YES || do_bcast_p == CONFIG_BOOLEAN_AUTO) {
        do_bcast_p = CONFIG_BOOLEAN_YES;

        static RRDSET *st_ip_bcastpkts = NULL;
        static RRDDIM *rd_in = NULL, *rd_out = NULL;

        if(unlikely(!st_ip_bcastpkts)) {
            st_ip_bcastpkts = rrdset_create_localhost(
                    RRD_TYPE_NET_IP4
                    , "bcastpkts"
                    , NULL
                    , "broadcast"
                    , NULL
                    , "IPv4 Broadcast Packets"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV4_BCAST_PACKETS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_in  = rrddim_add(st_ip_bcastpkts, "received", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_out = rrddim_add(st_ip_bcastpkts, "sent",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_ip_bcastpkts, rd_in,  ipext_InBcastPkts);
        rrddim_set_by_pointer(st_ip_bcastpkts, rd_out, ipext_OutBcastPkts);
        rrdset_done(st_ip_bcastpkts);
    }

    if (do_ecn == CONFIG_BOOLEAN_YES || do_ecn == CONFIG_BOOLEAN_AUTO) {
        do_ecn = CONFIG_BOOLEAN_YES;

        static RRDSET *st_ecnpkts = NULL;
        static RRDDIM *rd_cep = NULL, *rd_noectp = NULL, *rd_ectp0 = NULL, *rd_ectp1 = NULL;

        if(unlikely(!st_ecnpkts)) {
            st_ecnpkts = rrdset_create_localhost(
                    RRD_TYPE_NET_IP4
                    , "ecnpkts"
                    , NULL
                    , "ecn"
                    , NULL
                    , "IPv4 ECN Statistics"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV4_ECN
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_cep    = rrddim_add(st_ecnpkts, "InCEPkts",    "CEP",     1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_noectp = rrddim_add(st_ecnpkts, "InNoECTPkts", "NoECTP", -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_ectp0  = rrddim_add(st_ecnpkts, "InECT0Pkts",  "ECTP0",   1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_ectp1  = rrddim_add(st_ecnpkts, "InECT1Pkts",  "ECTP1",   1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_ecnpkts, rd_cep,    ipext_InCEPkts);
        rrddim_set_by_pointer(st_ecnpkts, rd_noectp, ipext_InNoECTPkts);
        rrddim_set_by_pointer(st_ecnpkts, rd_ectp0,  ipext_InECT0Pkts);
        rrddim_set_by_pointer(st_ecnpkts, rd_ectp1,  ipext_InECT1Pkts);
        rrdset_done(st_ecnpkts);
    }

    // netstat TcpExt charts

    if (do_tcpext_memory == CONFIG_BOOLEAN_YES || do_tcpext_memory == CONFIG_BOOLEAN_AUTO) {
        do_tcpext_memory = CONFIG_BOOLEAN_YES;

        static RRDSET *st_tcpmemorypressures = NULL;
        static RRDDIM *rd_pressures = NULL;

        if(unlikely(!st_tcpmemorypressures)) {
            st_tcpmemorypressures = rrdset_create_localhost(
                    RRD_TYPE_NET_IP
                    , "tcpmemorypressures"
                    , NULL
                    , "tcp"
                    , NULL
                    , "TCP Memory Pressures"
                    , "events/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IP_TCP_MEM_PRESSURE
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_pressures = rrddim_add(st_tcpmemorypressures, "TCPMemoryPressures",   "pressures",  1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_tcpmemorypressures, rd_pressures, tcpext_TCPMemoryPressures);
        rrdset_done(st_tcpmemorypressures);
    }

    if (do_tcpext_connaborts == CONFIG_BOOLEAN_YES || do_tcpext_connaborts == CONFIG_BOOLEAN_AUTO) {
        do_tcpext_connaborts = CONFIG_BOOLEAN_YES;

        static RRDSET *st_tcpconnaborts = NULL;
        static RRDDIM *rd_baddata = NULL, *rd_userclosed = NULL, *rd_nomemory = NULL, *rd_timeout = NULL, *rd_linger = NULL, *rd_failed = NULL;

        if(unlikely(!st_tcpconnaborts)) {
            st_tcpconnaborts = rrdset_create_localhost(
                    RRD_TYPE_NET_IP
                    , "tcpconnaborts"
                    , NULL
                    , "tcp"
                    , NULL
                    , "TCP Connection Aborts"
                    , "connections/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IP_TCP_CONNABORTS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_baddata    = rrddim_add(st_tcpconnaborts, "TCPAbortOnData",    "baddata",     1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_userclosed = rrddim_add(st_tcpconnaborts, "TCPAbortOnClose",   "userclosed",  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_nomemory   = rrddim_add(st_tcpconnaborts, "TCPAbortOnMemory",  "nomemory",    1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_timeout    = rrddim_add(st_tcpconnaborts, "TCPAbortOnTimeout", "timeout",     1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_linger     = rrddim_add(st_tcpconnaborts, "TCPAbortOnLinger",  "linger",      1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_failed     = rrddim_add(st_tcpconnaborts, "TCPAbortFailed",    "failed",     -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_tcpconnaborts, rd_baddata,    tcpext_TCPAbortOnData);
        rrddim_set_by_pointer(st_tcpconnaborts, rd_userclosed, tcpext_TCPAbortOnClose);
        rrddim_set_by_pointer(st_tcpconnaborts, rd_nomemory,   tcpext_TCPAbortOnMemory);
        rrddim_set_by_pointer(st_tcpconnaborts, rd_timeout,    tcpext_TCPAbortOnTimeout);
        rrddim_set_by_pointer(st_tcpconnaborts, rd_linger,     tcpext_TCPAbortOnLinger);
        rrddim_set_by_pointer(st_tcpconnaborts, rd_failed,     tcpext_TCPAbortFailed);
        rrdset_done(st_tcpconnaborts);
    }

    if (do_tcpext_reorder == CONFIG_BOOLEAN_YES || do_tcpext_reorder == CONFIG_BOOLEAN_AUTO) {
        do_tcpext_reorder = CONFIG_BOOLEAN_YES;

        static RRDSET *st_tcpreorders = NULL;
        static RRDDIM *rd_timestamp = NULL, *rd_sack = NULL, *rd_fack = NULL, *rd_reno = NULL;

        if(unlikely(!st_tcpreorders)) {
            st_tcpreorders = rrdset_create_localhost(
                    RRD_TYPE_NET_IP
                    , "tcpreorders"
                    , NULL
                    , "tcp"
                    , NULL
                    , "TCP Reordered Packets by Detection Method"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IP_TCP_REORDERS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_timestamp = rrddim_add(st_tcpreorders, "TCPTSReorder",   "timestamp",   1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_sack      = rrddim_add(st_tcpreorders, "TCPSACKReorder", "sack",        1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_fack      = rrddim_add(st_tcpreorders, "TCPFACKReorder", "fack",        1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_reno      = rrddim_add(st_tcpreorders, "TCPRenoReorder", "reno",        1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_tcpreorders, rd_timestamp, tcpext_TCPTSReorder);
        rrddim_set_by_pointer(st_tcpreorders, rd_sack,      tcpext_TCPSACKReorder);
        rrddim_set_by_pointer(st_tcpreorders, rd_fack,      tcpext_TCPFACKReorder);
        rrddim_set_by_pointer(st_tcpreorders, rd_reno,      tcpext_TCPRenoReorder);
        rrdset_done(st_tcpreorders);
    }

    // --------------------------------------------------------------------

    if (do_tcpext_ofo == CONFIG_BOOLEAN_YES || do_tcpext_ofo == CONFIG_BOOLEAN_AUTO) {
        do_tcpext_ofo = CONFIG_BOOLEAN_YES;

        static RRDSET *st_ip_tcpofo = NULL;
        static RRDDIM *rd_inqueue = NULL, *rd_dropped = NULL, *rd_merged = NULL, *rd_pruned = NULL;

        if(unlikely(!st_ip_tcpofo)) {

            st_ip_tcpofo = rrdset_create_localhost(
                    RRD_TYPE_NET_IP
                    , "tcpofo"
                    , NULL
                    , "tcp"
                    , NULL
                    , "TCP Out-Of-Order Queue"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IP_TCP_OFO
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_inqueue = rrddim_add(st_ip_tcpofo, "TCPOFOQueue", "inqueue",  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_dropped = rrddim_add(st_ip_tcpofo, "TCPOFODrop",  "dropped", -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_merged  = rrddim_add(st_ip_tcpofo, "TCPOFOMerge", "merged",   1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_pruned  = rrddim_add(st_ip_tcpofo, "OfoPruned",   "pruned",  -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_ip_tcpofo, rd_inqueue, tcpext_TCPOFOQueue);
        rrddim_set_by_pointer(st_ip_tcpofo, rd_dropped, tcpext_TCPOFODrop);
        rrddim_set_by_pointer(st_ip_tcpofo, rd_merged,  tcpext_TCPOFOMerge);
        rrddim_set_by_pointer(st_ip_tcpofo, rd_pruned,  tcpext_OfoPruned);
        rrdset_done(st_ip_tcpofo);
    }

    if (do_tcpext_syscookies == CONFIG_BOOLEAN_YES || do_tcpext_syscookies == CONFIG_BOOLEAN_AUTO) {
        do_tcpext_syscookies = CONFIG_BOOLEAN_YES;

        static RRDSET *st_syncookies = NULL;
        static RRDDIM *rd_received = NULL, *rd_sent = NULL, *rd_failed = NULL;

        if(unlikely(!st_syncookies)) {

            st_syncookies = rrdset_create_localhost(
                    RRD_TYPE_NET_IP
                    , "tcpsyncookies"
                    , NULL
                    , "tcp"
                    , NULL
                    , "TCP SYN Cookies"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IP_TCP_SYNCOOKIES
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_received = rrddim_add(st_syncookies, "received", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_sent     = rrddim_add(st_syncookies, "sent",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_failed   = rrddim_add(st_syncookies, "failed",   NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_syncookies, rd_received, tcpext_SyncookiesRecv);
        rrddim_set_by_pointer(st_syncookies, rd_sent,     tcpext_SyncookiesSent);
        rrddim_set_by_pointer(st_syncookies, rd_failed,   tcpext_SyncookiesFailed);
        rrdset_done(st_syncookies);
    }

    if (do_tcpext_syn_queue == CONFIG_BOOLEAN_YES || do_tcpext_syn_queue == CONFIG_BOOLEAN_AUTO) {
        do_tcpext_syn_queue = CONFIG_BOOLEAN_YES;

        static RRDSET *st_syn_queue = NULL;
        static RRDDIM
                *rd_TCPReqQFullDrop = NULL,
                *rd_TCPReqQFullDoCookies = NULL;

        if(unlikely(!st_syn_queue)) {

            st_syn_queue = rrdset_create_localhost(
                    RRD_TYPE_NET_IP
                    , "tcp_syn_queue"
                    , NULL
                    , "tcp"
                    , NULL
                    , "TCP SYN Queue Issues"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IP_TCP_SYN_QUEUE
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_TCPReqQFullDrop      = rrddim_add(st_syn_queue, "TCPReqQFullDrop",      "drops",   1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_TCPReqQFullDoCookies = rrddim_add(st_syn_queue, "TCPReqQFullDoCookies", "cookies", 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_syn_queue, rd_TCPReqQFullDrop,      tcpext_TCPReqQFullDrop);
        rrddim_set_by_pointer(st_syn_queue, rd_TCPReqQFullDoCookies, tcpext_TCPReqQFullDoCookies);
        rrdset_done(st_syn_queue);
    }

    if (do_tcpext_accept_queue == CONFIG_BOOLEAN_YES || do_tcpext_accept_queue == CONFIG_BOOLEAN_AUTO) {
        do_tcpext_accept_queue = CONFIG_BOOLEAN_YES;

        static RRDSET *st_accept_queue = NULL;
        static RRDDIM *rd_overflows = NULL,
            *rd_drops = NULL;

        if(unlikely(!st_accept_queue)) {

            st_accept_queue = rrdset_create_localhost(
                    RRD_TYPE_NET_IP
                    , "tcp_accept_queue"
                    , NULL
                    , "tcp"
                    , NULL
                    , "TCP Accept Queue Issues"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IP_TCP_ACCEPT_QUEUE
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_overflows = rrddim_add(st_accept_queue, "ListenOverflows", "overflows", 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_drops     = rrddim_add(st_accept_queue, "ListenDrops",     "drops",     1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_accept_queue, rd_overflows, tcpext_ListenOverflows);
        rrddim_set_by_pointer(st_accept_queue, rd_drops,     tcpext_ListenDrops);
        rrdset_done(st_accept_queue);
    }

    // snmp Ip charts

    if (do_ip_packets == CONFIG_BOOLEAN_YES || do_ip_packets == CONFIG_BOOLEAN_AUTO) {
        do_ip_packets = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        static RRDDIM *rd_InReceives = NULL,
                        *rd_OutRequests = NULL,
                        *rd_ForwDatagrams = NULL,
                        *rd_InDelivers = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP4
                    , "packets"
                    , NULL
                    , "packets"
                    , NULL
                    , "IPv4 Packets"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV4_PACKETS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_InReceives    = rrddim_add(st, "received",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutRequests   = rrddim_add(st, "sent",      NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_ForwDatagrams = rrddim_add(st, "forwarded", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InDelivers    = rrddim_add(st, "delivered", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_OutRequests,   (collected_number)snmp_root.ip_OutRequests);
        rrddim_set_by_pointer(st, rd_InReceives,    (collected_number)snmp_root.ip_InReceives);
        rrddim_set_by_pointer(st, rd_ForwDatagrams, (collected_number)snmp_root.ip_ForwDatagrams);
        rrddim_set_by_pointer(st, rd_InDelivers,    (collected_number)snmp_root.ip_InDelivers);
        rrdset_done(st);
    }

    if (do_ip_fragsout == CONFIG_BOOLEAN_YES || do_ip_fragsout == CONFIG_BOOLEAN_AUTO) {
        do_ip_fragsout = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        static RRDDIM *rd_FragOKs = NULL,
                        *rd_FragFails = NULL,
                        *rd_FragCreates = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP4
                    , "fragsout"
                    , NULL
                    , "fragments"
                    , NULL
                    , "IPv4 Fragments Sent"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV4_FRAGMENTS_OUT
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_FragOKs     = rrddim_add(st, "FragOKs",     "ok",      1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_FragFails   = rrddim_add(st, "FragFails",   "failed", -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_FragCreates = rrddim_add(st, "FragCreates", "created", 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_FragOKs,     (collected_number)snmp_root.ip_FragOKs);
        rrddim_set_by_pointer(st, rd_FragFails,   (collected_number)snmp_root.ip_FragFails);
        rrddim_set_by_pointer(st, rd_FragCreates, (collected_number)snmp_root.ip_FragCreates);
        rrdset_done(st);
    }

    if (do_ip_fragsin == CONFIG_BOOLEAN_YES || do_ip_fragsin == CONFIG_BOOLEAN_AUTO) {
        do_ip_fragsin = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        static RRDDIM *rd_ReasmOKs = NULL,
                        *rd_ReasmFails = NULL,
                        *rd_ReasmReqds = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP4
                    , "fragsin"
                    , NULL
                    , "fragments"
                    , NULL
                    , "IPv4 Fragments Reassembly"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV4_FRAGMENTS_IN
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_ReasmOKs   = rrddim_add(st, "ReasmOKs",   "ok",      1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_ReasmFails = rrddim_add(st, "ReasmFails", "failed", -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_ReasmReqds = rrddim_add(st, "ReasmReqds", "all",     1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_ReasmOKs,   (collected_number)snmp_root.ip_ReasmOKs);
        rrddim_set_by_pointer(st, rd_ReasmFails, (collected_number)snmp_root.ip_ReasmFails);
        rrddim_set_by_pointer(st, rd_ReasmReqds, (collected_number)snmp_root.ip_ReasmReqds);
        rrdset_done(st);
    }

    if (do_ip_errors == CONFIG_BOOLEAN_YES || do_ip_errors == CONFIG_BOOLEAN_AUTO) {
        do_ip_errors = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        static RRDDIM *rd_InDiscards = NULL,
                        *rd_OutDiscards = NULL,
                        *rd_InHdrErrors = NULL,
                        *rd_InNoRoutes = NULL,
                        *rd_OutNoRoutes = NULL,
                        *rd_InAddrErrors = NULL,
                        *rd_InTruncatedPkts = NULL,
                        *rd_InCsumErrors = NULL,
                        *rd_InUnknownProtos = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP4
                    , "errors"
                    , NULL
                    , "errors"
                    , NULL
                    , "IPv4 Errors"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV4_ERRORS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_InDiscards      = rrddim_add(st, "InDiscards",      NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutDiscards     = rrddim_add(st, "OutDiscards",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

            rd_InNoRoutes      = rrddim_add(st, "InNoRoutes",      NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutNoRoutes     = rrddim_add(st, "OutNoRoutes",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

            rd_InHdrErrors     = rrddim_add(st, "InHdrErrors",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InAddrErrors    = rrddim_add(st, "InAddrErrors",    NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InUnknownProtos = rrddim_add(st, "InUnknownProtos", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InTruncatedPkts = rrddim_add(st, "InTruncatedPkts", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InCsumErrors    = rrddim_add(st, "InCsumErrors",    NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_InDiscards,      (collected_number)snmp_root.ip_InDiscards);
        rrddim_set_by_pointer(st, rd_OutDiscards,     (collected_number)snmp_root.ip_OutDiscards);
        rrddim_set_by_pointer(st, rd_InHdrErrors,     (collected_number)snmp_root.ip_InHdrErrors);
        rrddim_set_by_pointer(st, rd_InAddrErrors,    (collected_number)snmp_root.ip_InAddrErrors);
        rrddim_set_by_pointer(st, rd_InUnknownProtos, (collected_number)snmp_root.ip_InUnknownProtos);
        rrddim_set_by_pointer(st, rd_InNoRoutes,      (collected_number)ipext_InNoRoutes);
        rrddim_set_by_pointer(st, rd_OutNoRoutes,     (collected_number)snmp_root.ip_OutNoRoutes);
        rrddim_set_by_pointer(st, rd_InTruncatedPkts, (collected_number)ipext_InTruncatedPkts);
        rrddim_set_by_pointer(st, rd_InCsumErrors,    (collected_number)ipext_InCsumErrors);
        rrdset_done(st);
    }

    // snmp Icmp charts

    if (do_icmp_packets == CONFIG_BOOLEAN_YES || do_icmp_packets == CONFIG_BOOLEAN_AUTO) {
        do_icmp_packets = CONFIG_BOOLEAN_YES;

        {
            static RRDSET *st_packets = NULL;
            static RRDDIM *rd_InMsgs = NULL,
                            *rd_OutMsgs = NULL;

            if(unlikely(!st_packets)) {
                st_packets = rrdset_create_localhost(
                        RRD_TYPE_NET_IP4
                        , "icmp"
                        , NULL
                        , "icmp"
                        , NULL
                        , "IPv4 ICMP Packets"
                        , "packets/s"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_NETSTAT_NAME
                        , NETDATA_CHART_PRIO_IPV4_ICMP_PACKETS
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rd_InMsgs  = rrddim_add(st_packets, "received", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                rd_OutMsgs = rrddim_add(st_packets, "sent",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            }

            rrddim_set_by_pointer(st_packets, rd_InMsgs,  (collected_number)snmp_root.icmp_InMsgs);
            rrddim_set_by_pointer(st_packets, rd_OutMsgs, (collected_number)snmp_root.icmp_OutMsgs);
            rrdset_done(st_packets);
        }

        {
            static RRDSET *st_errors = NULL;
            static RRDDIM *rd_InErrors = NULL,
                            *rd_OutErrors = NULL,
                            *rd_InCsumErrors = NULL;

            if(unlikely(!st_errors)) {
                st_errors = rrdset_create_localhost(
                        RRD_TYPE_NET_IP4
                        , "icmp_errors"
                        , NULL
                        , "icmp"
                        , NULL
                        , "IPv4 ICMP Errors"
                        , "packets/s"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_NETSTAT_NAME
                        , NETDATA_CHART_PRIO_IPV4_ICMP_ERRORS
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rd_InErrors     = rrddim_add(st_errors, "InErrors",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                rd_OutErrors    = rrddim_add(st_errors, "OutErrors",    NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                rd_InCsumErrors = rrddim_add(st_errors, "InCsumErrors", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            }

            rrddim_set_by_pointer(st_errors, rd_InErrors,     (collected_number)snmp_root.icmp_InErrors);
            rrddim_set_by_pointer(st_errors, rd_OutErrors,    (collected_number)snmp_root.icmp_OutErrors);
            rrddim_set_by_pointer(st_errors, rd_InCsumErrors, (collected_number)snmp_root.icmp_InCsumErrors);
            rrdset_done(st_errors);
        }
    }

    // snmp IcmpMsg charts

    if (do_icmpmsg == CONFIG_BOOLEAN_YES || do_icmpmsg == CONFIG_BOOLEAN_AUTO) {
        do_icmpmsg = CONFIG_BOOLEAN_YES;

        static RRDSET *st                  = NULL;
        static RRDDIM *rd_InEchoReps       = NULL,
                        *rd_OutEchoReps      = NULL,
                        *rd_InDestUnreachs   = NULL,
                        *rd_OutDestUnreachs  = NULL,
                        *rd_InRedirects      = NULL,
                        *rd_OutRedirects     = NULL,
                        *rd_InEchos          = NULL,
                        *rd_OutEchos         = NULL,
                        *rd_InRouterAdvert   = NULL,
                        *rd_OutRouterAdvert  = NULL,
                        *rd_InRouterSelect   = NULL,
                        *rd_OutRouterSelect  = NULL,
                        *rd_InTimeExcds      = NULL,
                        *rd_OutTimeExcds     = NULL,
                        *rd_InParmProbs      = NULL,
                        *rd_OutParmProbs     = NULL,
                        *rd_InTimestamps     = NULL,
                        *rd_OutTimestamps    = NULL,
                        *rd_InTimestampReps  = NULL,
                        *rd_OutTimestampReps = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP4
                    , "icmpmsg"
                    , NULL
                    , "icmp"
                    , NULL
                    , "IPv4 ICMP Messages"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV4_ICMP_MESSAGES
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_InEchoReps       = rrddim_add(st, "InType0", "InEchoReps", 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutEchoReps      = rrddim_add(st, "OutType0", "OutEchoReps", -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InDestUnreachs   = rrddim_add(st, "InType3", "InDestUnreachs", 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutDestUnreachs  = rrddim_add(st, "OutType3", "OutDestUnreachs", -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InRedirects      = rrddim_add(st, "InType5", "InRedirects", 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutRedirects     = rrddim_add(st, "OutType5", "OutRedirects", -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InEchos          = rrddim_add(st, "InType8", "InEchos", 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutEchos         = rrddim_add(st, "OutType8", "OutEchos", -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InRouterAdvert   = rrddim_add(st, "InType9", "InRouterAdvert", 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutRouterAdvert  = rrddim_add(st, "OutType9", "OutRouterAdvert", -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InRouterSelect   = rrddim_add(st, "InType10", "InRouterSelect", 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutRouterSelect  = rrddim_add(st, "OutType10", "OutRouterSelect", -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InTimeExcds      = rrddim_add(st, "InType11", "InTimeExcds", 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutTimeExcds     = rrddim_add(st, "OutType11", "OutTimeExcds", -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InParmProbs      = rrddim_add(st, "InType12", "InParmProbs", 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutParmProbs     = rrddim_add(st, "OutType12", "OutParmProbs", -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InTimestamps     = rrddim_add(st, "InType13", "InTimestamps", 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutTimestamps    = rrddim_add(st, "OutType13", "OutTimestamps", -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InTimestampReps  = rrddim_add(st, "InType14", "InTimestampReps", 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutTimestampReps = rrddim_add(st, "OutType14", "OutTimestampReps", -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_InEchoReps, (collected_number)snmp_root.icmpmsg_InEchoReps);
        rrddim_set_by_pointer(st, rd_OutEchoReps, (collected_number)snmp_root.icmpmsg_OutEchoReps);
        rrddim_set_by_pointer(st, rd_InDestUnreachs, (collected_number)snmp_root.icmpmsg_InDestUnreachs);
        rrddim_set_by_pointer(st, rd_OutDestUnreachs, (collected_number)snmp_root.icmpmsg_OutDestUnreachs);
        rrddim_set_by_pointer(st, rd_InRedirects, (collected_number)snmp_root.icmpmsg_InRedirects);
        rrddim_set_by_pointer(st, rd_OutRedirects, (collected_number)snmp_root.icmpmsg_OutRedirects);
        rrddim_set_by_pointer(st, rd_InEchos, (collected_number)snmp_root.icmpmsg_InEchos);
        rrddim_set_by_pointer(st, rd_OutEchos, (collected_number)snmp_root.icmpmsg_OutEchos);
        rrddim_set_by_pointer(st, rd_InRouterAdvert, (collected_number)snmp_root.icmpmsg_InRouterAdvert);
        rrddim_set_by_pointer(st, rd_OutRouterAdvert, (collected_number)snmp_root.icmpmsg_OutRouterAdvert);
        rrddim_set_by_pointer(st, rd_InRouterSelect, (collected_number)snmp_root.icmpmsg_InRouterSelect);
        rrddim_set_by_pointer(st, rd_OutRouterSelect, (collected_number)snmp_root.icmpmsg_OutRouterSelect);
        rrddim_set_by_pointer(st, rd_InTimeExcds, (collected_number)snmp_root.icmpmsg_InTimeExcds);
        rrddim_set_by_pointer(st, rd_OutTimeExcds, (collected_number)snmp_root.icmpmsg_OutTimeExcds);
        rrddim_set_by_pointer(st, rd_InParmProbs, (collected_number)snmp_root.icmpmsg_InParmProbs);
        rrddim_set_by_pointer(st, rd_OutParmProbs, (collected_number)snmp_root.icmpmsg_OutParmProbs);
        rrddim_set_by_pointer(st, rd_InTimestamps, (collected_number)snmp_root.icmpmsg_InTimestamps);
        rrddim_set_by_pointer(st, rd_OutTimestamps, (collected_number)snmp_root.icmpmsg_OutTimestamps);
        rrddim_set_by_pointer(st, rd_InTimestampReps, (collected_number)snmp_root.icmpmsg_InTimestampReps);
        rrddim_set_by_pointer(st, rd_OutTimestampReps, (collected_number)snmp_root.icmpmsg_OutTimestampReps);

        rrdset_done(st);
    }

    // snmp Tcp charts

    // this is smart enough to update it, only when it is changed
    rrdvar_host_variable_set(localhost, tcp_max_connections_var, snmp_root.tcp_MaxConn);

    // see http://net-snmp.sourceforge.net/docs/mibs/tcp.html
    if (do_tcp_sockets == CONFIG_BOOLEAN_YES || do_tcp_sockets == CONFIG_BOOLEAN_AUTO) {
        do_tcp_sockets = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        static RRDDIM *rd_CurrEstab = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP
                    , "tcpsock"
                    , NULL
                    , "tcp"
                    , NULL
                    , "TCP Connections"
                    , "active connections"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IP_TCP_ESTABLISHED_CONNS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_CurrEstab = rrddim_add(st, "CurrEstab", "connections", 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(st, rd_CurrEstab, (collected_number)snmp_root.tcp_CurrEstab);
        rrdset_done(st);
    }

    if (do_tcp_packets == CONFIG_BOOLEAN_YES || do_tcp_packets == CONFIG_BOOLEAN_AUTO) {
        do_tcp_packets = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        static RRDDIM *rd_InSegs = NULL,
                        *rd_OutSegs = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP
                    , "tcppackets"
                    , NULL
                    , "tcp"
                    , NULL
                    , "IPv4 TCP Packets"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IP_TCP_PACKETS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_InSegs  = rrddim_add(st, "received", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutSegs = rrddim_add(st, "sent",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_InSegs,  (collected_number)snmp_root.tcp_InSegs);
        rrddim_set_by_pointer(st, rd_OutSegs, (collected_number)snmp_root.tcp_OutSegs);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if (do_tcp_errors == CONFIG_BOOLEAN_YES || do_tcp_errors == CONFIG_BOOLEAN_AUTO) {
        do_tcp_errors = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        static RRDDIM *rd_InErrs = NULL,
                        *rd_InCsumErrors = NULL,
                        *rd_RetransSegs = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP
                    , "tcperrors"
                    , NULL
                    , "tcp"
                    , NULL
                    , "IPv4 TCP Errors"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IP_TCP_ERRORS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_InErrs       = rrddim_add(st, "InErrs",       NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InCsumErrors = rrddim_add(st, "InCsumErrors", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_RetransSegs  = rrddim_add(st, "RetransSegs",  NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_InErrs,       (collected_number)snmp_root.tcp_InErrs);
        rrddim_set_by_pointer(st, rd_InCsumErrors, (collected_number)snmp_root.tcp_InCsumErrors);
        rrddim_set_by_pointer(st, rd_RetransSegs,  (collected_number)snmp_root.tcp_RetransSegs);
        rrdset_done(st);
    }

    if (do_tcp_opens == CONFIG_BOOLEAN_YES || do_tcp_opens == CONFIG_BOOLEAN_AUTO) {
        do_tcp_opens = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        static RRDDIM *rd_ActiveOpens = NULL,
                        *rd_PassiveOpens = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP
                    , "tcpopens"
                    , NULL
                    , "tcp"
                    , NULL
                    , "IPv4 TCP Opens"
                    , "connections/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IP_TCP_OPENS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_ActiveOpens   = rrddim_add(st, "ActiveOpens",   "active", 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_PassiveOpens  = rrddim_add(st, "PassiveOpens",  "passive", 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_ActiveOpens,   (collected_number)snmp_root.tcp_ActiveOpens);
        rrddim_set_by_pointer(st, rd_PassiveOpens,  (collected_number)snmp_root.tcp_PassiveOpens);
        rrdset_done(st);
    }

    if (do_tcp_handshake == CONFIG_BOOLEAN_YES || do_tcp_handshake == CONFIG_BOOLEAN_AUTO) {
        do_tcp_handshake = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        static RRDDIM *rd_EstabResets = NULL,
                        *rd_OutRsts = NULL,
                        *rd_AttemptFails = NULL,
                        *rd_TCPSynRetrans = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP
                    , "tcphandshake"
                    , NULL
                    , "tcp"
                    , NULL
                    , "IPv4 TCP Handshake Issues"
                    , "events/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IP_TCP_HANDSHAKE
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_EstabResets     = rrddim_add(st, "EstabResets",          NULL,                1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutRsts         = rrddim_add(st, "OutRsts",              NULL,                1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_AttemptFails    = rrddim_add(st, "AttemptFails",         NULL,                1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_TCPSynRetrans   = rrddim_add(st, "TCPSynRetrans",        "SynRetrans",        1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_EstabResets,     (collected_number)snmp_root.tcp_EstabResets);
        rrddim_set_by_pointer(st, rd_OutRsts,         (collected_number)snmp_root.tcp_OutRsts);
        rrddim_set_by_pointer(st, rd_AttemptFails,    (collected_number)snmp_root.tcp_AttemptFails);
        rrddim_set_by_pointer(st, rd_TCPSynRetrans,   tcpext_TCPSynRetrans);
        rrdset_done(st);
    }

    // snmp Udp charts

    // see http://net-snmp.sourceforge.net/docs/mibs/udp.html
    if (do_udp_packets == CONFIG_BOOLEAN_YES || do_udp_packets == CONFIG_BOOLEAN_AUTO) {
        do_udp_packets = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        static RRDDIM *rd_InDatagrams = NULL,
                        *rd_OutDatagrams = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP4
                    , "udppackets"
                    , NULL
                    , "udp"
                    , NULL
                    , "IPv4 UDP Packets"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV4_UDP_PACKETS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_InDatagrams  = rrddim_add(st, "received", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutDatagrams = rrddim_add(st, "sent",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_InDatagrams,  (collected_number)snmp_root.udp_InDatagrams);
        rrddim_set_by_pointer(st, rd_OutDatagrams, (collected_number)snmp_root.udp_OutDatagrams);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if (do_udp_errors == CONFIG_BOOLEAN_YES || do_udp_errors == CONFIG_BOOLEAN_AUTO) {
        do_udp_errors = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        static RRDDIM *rd_RcvbufErrors = NULL,
                        *rd_SndbufErrors = NULL,
                        *rd_InErrors = NULL,
                        *rd_NoPorts = NULL,
                        *rd_InCsumErrors = NULL,
                        *rd_IgnoredMulti = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IP4
                    , "udperrors"
                    , NULL
                    , "udp"
                    , NULL
                    , "IPv4 UDP Errors"
                    , "events/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETSTAT_NAME
                    , NETDATA_CHART_PRIO_IPV4_UDP_ERRORS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_RcvbufErrors = rrddim_add(st, "RcvbufErrors", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_SndbufErrors = rrddim_add(st, "SndbufErrors", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InErrors     = rrddim_add(st, "InErrors",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_NoPorts      = rrddim_add(st, "NoPorts",      NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InCsumErrors = rrddim_add(st, "InCsumErrors", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_IgnoredMulti = rrddim_add(st, "IgnoredMulti", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_InErrors,     (collected_number)snmp_root.udp_InErrors);
        rrddim_set_by_pointer(st, rd_NoPorts,      (collected_number)snmp_root.udp_NoPorts);
        rrddim_set_by_pointer(st, rd_RcvbufErrors, (collected_number)snmp_root.udp_RcvbufErrors);
        rrddim_set_by_pointer(st, rd_SndbufErrors, (collected_number)snmp_root.udp_SndbufErrors);
        rrddim_set_by_pointer(st, rd_InCsumErrors, (collected_number)snmp_root.udp_InCsumErrors);
        rrddim_set_by_pointer(st, rd_IgnoredMulti, (collected_number)snmp_root.udp_IgnoredMulti);
        rrdset_done(st);
    }

    // snmp UdpLite charts

    if (do_udplite_packets == CONFIG_BOOLEAN_YES || do_udplite_packets == CONFIG_BOOLEAN_AUTO) {
        do_udplite_packets = CONFIG_BOOLEAN_YES;

        {
            static RRDSET *st = NULL;
            static RRDDIM *rd_InDatagrams = NULL,
                            *rd_OutDatagrams = NULL;

            if(unlikely(!st)) {
                st = rrdset_create_localhost(
                        RRD_TYPE_NET_IP4
                        , "udplite"
                        , NULL
                        , "udplite"
                        , NULL
                        , "IPv4 UDPLite Packets"
                        , "packets/s"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_NETSTAT_NAME
                        , NETDATA_CHART_PRIO_IPV4_UDPLITE_PACKETS
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rd_InDatagrams  = rrddim_add(st, "received", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                rd_OutDatagrams = rrddim_add(st, "sent",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            }

            rrddim_set_by_pointer(st, rd_InDatagrams,  (collected_number)snmp_root.udplite_InDatagrams);
            rrddim_set_by_pointer(st, rd_OutDatagrams, (collected_number)snmp_root.udplite_OutDatagrams);
            rrdset_done(st);
        }

        {
            static RRDSET *st = NULL;
            static RRDDIM *rd_RcvbufErrors = NULL,
                            *rd_SndbufErrors = NULL,
                            *rd_InErrors = NULL,
                            *rd_NoPorts = NULL,
                            *rd_InCsumErrors = NULL,
                            *rd_IgnoredMulti = NULL;

            if(unlikely(!st)) {
                st = rrdset_create_localhost(
                        RRD_TYPE_NET_IP4
                        , "udplite_errors"
                        , NULL
                        , "udplite"
                        , NULL
                        , "IPv4 UDPLite Errors"
                        , "packets/s"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_NETSTAT_NAME
                        , NETDATA_CHART_PRIO_IPV4_UDPLITE_ERRORS
                        , update_every
                        , RRDSET_TYPE_LINE);

                rd_RcvbufErrors = rrddim_add(st, "RcvbufErrors", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                rd_SndbufErrors = rrddim_add(st, "SndbufErrors", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                rd_InErrors     = rrddim_add(st, "InErrors",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                rd_NoPorts      = rrddim_add(st, "NoPorts",      NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                rd_InCsumErrors = rrddim_add(st, "InCsumErrors", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                rd_IgnoredMulti = rrddim_add(st, "IgnoredMulti", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            }

            rrddim_set_by_pointer(st, rd_NoPorts,      (collected_number)snmp_root.udplite_NoPorts);
            rrddim_set_by_pointer(st, rd_InErrors,     (collected_number)snmp_root.udplite_InErrors);
            rrddim_set_by_pointer(st, rd_InCsumErrors, (collected_number)snmp_root.udplite_InCsumErrors);
            rrddim_set_by_pointer(st, rd_RcvbufErrors, (collected_number)snmp_root.udplite_RcvbufErrors);
            rrddim_set_by_pointer(st, rd_SndbufErrors, (collected_number)snmp_root.udplite_SndbufErrors);
            rrddim_set_by_pointer(st, rd_IgnoredMulti, (collected_number)snmp_root.udplite_IgnoredMulti);
            rrdset_done(st);
        }
    }

    do_proc_net_snmp6(update_every);
  
    return 0;
}

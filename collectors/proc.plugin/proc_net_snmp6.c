// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

#define RRD_TYPE_NET_SNMP6          "ipv6"
#define PLUGIN_PROC_MODULE_NET_SNMP6_NAME "/proc/net/snmp6"

int do_proc_net_snmp6(int update_every, usec_t dt) {
    (void)dt;

    static procfile *ff = NULL;

    static int do_ip_packets = -1,
            do_ip_fragsout = -1,
            do_ip_fragsin = -1,
            do_ip_errors = -1,
            do_udplite_packets = -1,
            do_udplite_errors = -1,
            do_udp_packets = -1,
            do_udp_errors = -1,
            do_bandwidth = -1,
            do_mcast = -1,
            do_bcast = -1,
            do_mcast_p = -1,
            do_icmp = -1,
            do_icmp_redir = -1,
            do_icmp_errors = -1,
            do_icmp_echos = -1,
            do_icmp_groupmemb = -1,
            do_icmp_router = -1,
            do_icmp_neighbor = -1,
            do_icmp_mldv2 = -1,
            do_icmp_types = -1,
            do_ect = -1;

    static ARL_BASE *arl_base = NULL;

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

    if(unlikely(!arl_base)) {
        do_ip_packets       = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "ipv6 packets", CONFIG_BOOLEAN_AUTO);
        do_ip_fragsout      = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "ipv6 fragments sent", CONFIG_BOOLEAN_AUTO);
        do_ip_fragsin       = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "ipv6 fragments assembly", CONFIG_BOOLEAN_AUTO);
        do_ip_errors        = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "ipv6 errors", CONFIG_BOOLEAN_AUTO);
        do_udp_packets      = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "ipv6 UDP packets", CONFIG_BOOLEAN_AUTO);
        do_udp_errors       = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "ipv6 UDP errors", CONFIG_BOOLEAN_AUTO);
        do_udplite_packets  = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "ipv6 UDPlite packets", CONFIG_BOOLEAN_AUTO);
        do_udplite_errors   = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "ipv6 UDPlite errors", CONFIG_BOOLEAN_AUTO);
        do_bandwidth        = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "bandwidth", CONFIG_BOOLEAN_AUTO);
        do_mcast            = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "multicast bandwidth", CONFIG_BOOLEAN_AUTO);
        do_bcast            = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "broadcast bandwidth", CONFIG_BOOLEAN_AUTO);
        do_mcast_p          = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "multicast packets", CONFIG_BOOLEAN_AUTO);
        do_icmp             = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "icmp", CONFIG_BOOLEAN_AUTO);
        do_icmp_redir       = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "icmp redirects", CONFIG_BOOLEAN_AUTO);
        do_icmp_errors      = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "icmp errors", CONFIG_BOOLEAN_AUTO);
        do_icmp_echos       = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "icmp echos", CONFIG_BOOLEAN_AUTO);
        do_icmp_groupmemb   = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "icmp group membership", CONFIG_BOOLEAN_AUTO);
        do_icmp_router      = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "icmp router", CONFIG_BOOLEAN_AUTO);
        do_icmp_neighbor    = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "icmp neighbor", CONFIG_BOOLEAN_AUTO);
        do_icmp_mldv2       = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "icmp mldv2", CONFIG_BOOLEAN_AUTO);
        do_icmp_types       = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "icmp types", CONFIG_BOOLEAN_AUTO);
        do_ect              = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp6", "ect", CONFIG_BOOLEAN_AUTO);

        arl_base = arl_create("snmp6", NULL, 60);
        arl_expect(arl_base, "Ip6InReceives", &Ip6InReceives);
        arl_expect(arl_base, "Ip6InHdrErrors", &Ip6InHdrErrors);
        arl_expect(arl_base, "Ip6InTooBigErrors", &Ip6InTooBigErrors);
        arl_expect(arl_base, "Ip6InNoRoutes", &Ip6InNoRoutes);
        arl_expect(arl_base, "Ip6InAddrErrors", &Ip6InAddrErrors);
        arl_expect(arl_base, "Ip6InUnknownProtos", &Ip6InUnknownProtos);
        arl_expect(arl_base, "Ip6InTruncatedPkts", &Ip6InTruncatedPkts);
        arl_expect(arl_base, "Ip6InDiscards", &Ip6InDiscards);
        arl_expect(arl_base, "Ip6InDelivers", &Ip6InDelivers);
        arl_expect(arl_base, "Ip6OutForwDatagrams", &Ip6OutForwDatagrams);
        arl_expect(arl_base, "Ip6OutRequests", &Ip6OutRequests);
        arl_expect(arl_base, "Ip6OutDiscards", &Ip6OutDiscards);
        arl_expect(arl_base, "Ip6OutNoRoutes", &Ip6OutNoRoutes);
        arl_expect(arl_base, "Ip6ReasmTimeout", &Ip6ReasmTimeout);
        arl_expect(arl_base, "Ip6ReasmReqds", &Ip6ReasmReqds);
        arl_expect(arl_base, "Ip6ReasmOKs", &Ip6ReasmOKs);
        arl_expect(arl_base, "Ip6ReasmFails", &Ip6ReasmFails);
        arl_expect(arl_base, "Ip6FragOKs", &Ip6FragOKs);
        arl_expect(arl_base, "Ip6FragFails", &Ip6FragFails);
        arl_expect(arl_base, "Ip6FragCreates", &Ip6FragCreates);
        arl_expect(arl_base, "Ip6InMcastPkts", &Ip6InMcastPkts);
        arl_expect(arl_base, "Ip6OutMcastPkts", &Ip6OutMcastPkts);
        arl_expect(arl_base, "Ip6InOctets", &Ip6InOctets);
        arl_expect(arl_base, "Ip6OutOctets", &Ip6OutOctets);
        arl_expect(arl_base, "Ip6InMcastOctets", &Ip6InMcastOctets);
        arl_expect(arl_base, "Ip6OutMcastOctets", &Ip6OutMcastOctets);
        arl_expect(arl_base, "Ip6InBcastOctets", &Ip6InBcastOctets);
        arl_expect(arl_base, "Ip6OutBcastOctets", &Ip6OutBcastOctets);
        arl_expect(arl_base, "Ip6InNoECTPkts", &Ip6InNoECTPkts);
        arl_expect(arl_base, "Ip6InECT1Pkts", &Ip6InECT1Pkts);
        arl_expect(arl_base, "Ip6InECT0Pkts", &Ip6InECT0Pkts);
        arl_expect(arl_base, "Ip6InCEPkts", &Ip6InCEPkts);
        arl_expect(arl_base, "Icmp6InMsgs", &Icmp6InMsgs);
        arl_expect(arl_base, "Icmp6InErrors", &Icmp6InErrors);
        arl_expect(arl_base, "Icmp6OutMsgs", &Icmp6OutMsgs);
        arl_expect(arl_base, "Icmp6OutErrors", &Icmp6OutErrors);
        arl_expect(arl_base, "Icmp6InCsumErrors", &Icmp6InCsumErrors);
        arl_expect(arl_base, "Icmp6InDestUnreachs", &Icmp6InDestUnreachs);
        arl_expect(arl_base, "Icmp6InPktTooBigs", &Icmp6InPktTooBigs);
        arl_expect(arl_base, "Icmp6InTimeExcds", &Icmp6InTimeExcds);
        arl_expect(arl_base, "Icmp6InParmProblems", &Icmp6InParmProblems);
        arl_expect(arl_base, "Icmp6InEchos", &Icmp6InEchos);
        arl_expect(arl_base, "Icmp6InEchoReplies", &Icmp6InEchoReplies);
        arl_expect(arl_base, "Icmp6InGroupMembQueries", &Icmp6InGroupMembQueries);
        arl_expect(arl_base, "Icmp6InGroupMembResponses", &Icmp6InGroupMembResponses);
        arl_expect(arl_base, "Icmp6InGroupMembReductions", &Icmp6InGroupMembReductions);
        arl_expect(arl_base, "Icmp6InRouterSolicits", &Icmp6InRouterSolicits);
        arl_expect(arl_base, "Icmp6InRouterAdvertisements", &Icmp6InRouterAdvertisements);
        arl_expect(arl_base, "Icmp6InNeighborSolicits", &Icmp6InNeighborSolicits);
        arl_expect(arl_base, "Icmp6InNeighborAdvertisements", &Icmp6InNeighborAdvertisements);
        arl_expect(arl_base, "Icmp6InRedirects", &Icmp6InRedirects);
        arl_expect(arl_base, "Icmp6InMLDv2Reports", &Icmp6InMLDv2Reports);
        arl_expect(arl_base, "Icmp6OutDestUnreachs", &Icmp6OutDestUnreachs);
        arl_expect(arl_base, "Icmp6OutPktTooBigs", &Icmp6OutPktTooBigs);
        arl_expect(arl_base, "Icmp6OutTimeExcds", &Icmp6OutTimeExcds);
        arl_expect(arl_base, "Icmp6OutParmProblems", &Icmp6OutParmProblems);
        arl_expect(arl_base, "Icmp6OutEchos", &Icmp6OutEchos);
        arl_expect(arl_base, "Icmp6OutEchoReplies", &Icmp6OutEchoReplies);
        arl_expect(arl_base, "Icmp6OutGroupMembQueries", &Icmp6OutGroupMembQueries);
        arl_expect(arl_base, "Icmp6OutGroupMembResponses", &Icmp6OutGroupMembResponses);
        arl_expect(arl_base, "Icmp6OutGroupMembReductions", &Icmp6OutGroupMembReductions);
        arl_expect(arl_base, "Icmp6OutRouterSolicits", &Icmp6OutRouterSolicits);
        arl_expect(arl_base, "Icmp6OutRouterAdvertisements", &Icmp6OutRouterAdvertisements);
        arl_expect(arl_base, "Icmp6OutNeighborSolicits", &Icmp6OutNeighborSolicits);
        arl_expect(arl_base, "Icmp6OutNeighborAdvertisements", &Icmp6OutNeighborAdvertisements);
        arl_expect(arl_base, "Icmp6OutRedirects", &Icmp6OutRedirects);
        arl_expect(arl_base, "Icmp6OutMLDv2Reports", &Icmp6OutMLDv2Reports);
        arl_expect(arl_base, "Icmp6InType1", &Icmp6InType1);
        arl_expect(arl_base, "Icmp6InType128", &Icmp6InType128);
        arl_expect(arl_base, "Icmp6InType129", &Icmp6InType129);
        arl_expect(arl_base, "Icmp6InType136", &Icmp6InType136);
        arl_expect(arl_base, "Icmp6OutType1", &Icmp6OutType1);
        arl_expect(arl_base, "Icmp6OutType128", &Icmp6OutType128);
        arl_expect(arl_base, "Icmp6OutType129", &Icmp6OutType129);
        arl_expect(arl_base, "Icmp6OutType133", &Icmp6OutType133);
        arl_expect(arl_base, "Icmp6OutType135", &Icmp6OutType135);
        arl_expect(arl_base, "Icmp6OutType143", &Icmp6OutType143);
        arl_expect(arl_base, "Udp6InDatagrams", &Udp6InDatagrams);
        arl_expect(arl_base, "Udp6NoPorts", &Udp6NoPorts);
        arl_expect(arl_base, "Udp6InErrors", &Udp6InErrors);
        arl_expect(arl_base, "Udp6OutDatagrams", &Udp6OutDatagrams);
        arl_expect(arl_base, "Udp6RcvbufErrors", &Udp6RcvbufErrors);
        arl_expect(arl_base, "Udp6SndbufErrors", &Udp6SndbufErrors);
        arl_expect(arl_base, "Udp6InCsumErrors", &Udp6InCsumErrors);
        arl_expect(arl_base, "Udp6IgnoredMulti", &Udp6IgnoredMulti);
        arl_expect(arl_base, "UdpLite6InDatagrams", &UdpLite6InDatagrams);
        arl_expect(arl_base, "UdpLite6NoPorts", &UdpLite6NoPorts);
        arl_expect(arl_base, "UdpLite6InErrors", &UdpLite6InErrors);
        arl_expect(arl_base, "UdpLite6OutDatagrams", &UdpLite6OutDatagrams);
        arl_expect(arl_base, "UdpLite6RcvbufErrors", &UdpLite6RcvbufErrors);
        arl_expect(arl_base, "UdpLite6SndbufErrors", &UdpLite6SndbufErrors);
        arl_expect(arl_base, "UdpLite6InCsumErrors", &UdpLite6InCsumErrors);
    }

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/net/snmp6");
        ff = procfile_open(config_get("plugin:proc:/proc/net/snmp6", "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff))
            return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff))
        return 0; // we return 0, so that we will retry to open it next time

    size_t lines = procfile_lines(ff), l;

    arl_begin(arl_base);

    for(l = 0; l < lines ;l++) {
        size_t words = procfile_linewords(ff, l);
        if(unlikely(words < 2)) {
            if(unlikely(words)) error("Cannot read /proc/net/snmp6 line %zu. Expected 2 params, read %zu.", l, words);
            continue;
        }

        if(unlikely(arl_check(arl_base,
                procfile_lineword(ff, l, 0),
                procfile_lineword(ff, l, 1)))) break;
    }

    // --------------------------------------------------------------------

    if(do_bandwidth == CONFIG_BOOLEAN_YES || (do_bandwidth == CONFIG_BOOLEAN_AUTO &&
                                              (Ip6InOctets ||
                                               Ip6OutOctets ||
                                               netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        do_bandwidth = CONFIG_BOOLEAN_YES;
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
                    , PLUGIN_PROC_MODULE_NET_SNMP6_NAME
                    , NETDATA_CHART_PRIO_SYSTEM_IPV6
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rd_received = rrddim_add(st, "InOctets",  "received",  8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
            rd_sent     = rrddim_add(st, "OutOctets", "sent",     -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_received, Ip6InOctets);
        rrddim_set_by_pointer(st, rd_sent,     Ip6OutOctets);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_ip_packets == CONFIG_BOOLEAN_YES || (do_ip_packets == CONFIG_BOOLEAN_AUTO &&
                                               (Ip6InReceives ||
                                                Ip6OutRequests ||
                                                Ip6InDelivers ||
                                                Ip6OutForwDatagrams ||
                                                netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        do_ip_packets = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_received = NULL,
                      *rd_sent = NULL,
                      *rd_forwarded = NULL,
                      *rd_delivers = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "packets"
                    , NULL
                    , "packets"
                    , NULL
                    , "IPv6 Packets"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NET_SNMP6_NAME
                    , NETDATA_CHART_PRIO_IPV6_PACKETS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_received  = rrddim_add(st, "InReceives",       "received",   1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_sent      = rrddim_add(st, "OutRequests",      "sent",      -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_forwarded = rrddim_add(st, "OutForwDatagrams", "forwarded", -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_delivers  = rrddim_add(st, "InDelivers",       "delivers",   1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_received,  Ip6InReceives);
        rrddim_set_by_pointer(st, rd_sent,      Ip6OutRequests);
        rrddim_set_by_pointer(st, rd_forwarded, Ip6OutForwDatagrams);
        rrddim_set_by_pointer(st, rd_delivers,  Ip6InDelivers);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_ip_fragsout == CONFIG_BOOLEAN_YES || (do_ip_fragsout == CONFIG_BOOLEAN_AUTO &&
                                                (Ip6FragOKs ||
                                                 Ip6FragFails ||
                                                 Ip6FragCreates ||
                                                 netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        do_ip_fragsout = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_ok = NULL,
                      *rd_failed = NULL,
                      *rd_all = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "fragsout"
                    , NULL
                    , "fragments6"
                    , NULL
                    , "IPv6 Fragments Sent"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NET_SNMP6_NAME
                    , NETDATA_CHART_PRIO_IPV6_FRAGSOUT
                    , update_every
                    , RRDSET_TYPE_LINE
            );
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rd_ok     = rrddim_add(st, "FragOKs",     "ok",      1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_failed = rrddim_add(st, "FragFails",   "failed", -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_all    = rrddim_add(st, "FragCreates", "all",     1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_ok,     Ip6FragOKs);
        rrddim_set_by_pointer(st, rd_failed, Ip6FragFails);
        rrddim_set_by_pointer(st, rd_all,    Ip6FragCreates);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_ip_fragsin == CONFIG_BOOLEAN_YES || (do_ip_fragsin == CONFIG_BOOLEAN_AUTO &&
                                               (Ip6ReasmOKs ||
                                                Ip6ReasmFails ||
                                                Ip6ReasmTimeout ||
                                                Ip6ReasmReqds  ||
                                                netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        do_ip_fragsin = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        static RRDDIM *rd_ok = NULL,
                      *rd_failed = NULL,
                      *rd_timeout = NULL,
                      *rd_all = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "fragsin"
                    , NULL
                    , "fragments6"
                    , NULL
                    , "IPv6 Fragments Reassembly"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NET_SNMP6_NAME
                    , NETDATA_CHART_PRIO_IPV6_FRAGSIN
                    , update_every
                    , RRDSET_TYPE_LINE);
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rd_ok      = rrddim_add(st, "ReasmOKs",     "ok",       1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_failed  = rrddim_add(st, "ReasmFails",   "failed",  -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_timeout = rrddim_add(st, "ReasmTimeout", "timeout", -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_all     = rrddim_add(st, "ReasmReqds",   "all",      1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_ok,      Ip6ReasmOKs);
        rrddim_set_by_pointer(st, rd_failed,  Ip6ReasmFails);
        rrddim_set_by_pointer(st, rd_timeout, Ip6ReasmTimeout);
        rrddim_set_by_pointer(st, rd_all,     Ip6ReasmReqds);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_ip_errors == CONFIG_BOOLEAN_YES || (do_ip_errors == CONFIG_BOOLEAN_AUTO &&
                                              (Ip6InDiscards ||
                                               Ip6OutDiscards ||
                                               Ip6InHdrErrors ||
                                               Ip6InAddrErrors ||
                                               Ip6InUnknownProtos ||
                                               Ip6InTooBigErrors ||
                                               Ip6InTruncatedPkts ||
                                               Ip6InNoRoutes ||
                                               netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        do_ip_errors = CONFIG_BOOLEAN_YES;
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
                    RRD_TYPE_NET_SNMP6
                    , "errors"
                    , NULL
                    , "errors"
                    , NULL
                    , "IPv6 Errors"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NET_SNMP6_NAME
                    , NETDATA_CHART_PRIO_IPV6_ERRORS
                    , update_every
                    , RRDSET_TYPE_LINE
            );
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

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
        else rrdset_next(st);

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

    // --------------------------------------------------------------------

    if(do_udp_packets == CONFIG_BOOLEAN_YES || (do_udp_packets == CONFIG_BOOLEAN_AUTO &&
                                                (Udp6InDatagrams ||
                                                 Udp6OutDatagrams ||
                                                 netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        do_udp_packets = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_received = NULL,
                      *rd_sent     = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "udppackets"
                    , NULL
                    , "udp6"
                    , NULL
                    , "IPv6 UDP Packets"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NET_SNMP6_NAME
                    , NETDATA_CHART_PRIO_IPV6_UDP_PACKETS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_received = rrddim_add(st, "InDatagrams",  "received",  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_sent     = rrddim_add(st, "OutDatagrams", "sent",     -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_received, Udp6InDatagrams);
        rrddim_set_by_pointer(st, rd_sent,     Udp6OutDatagrams);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_udp_errors == CONFIG_BOOLEAN_YES || (do_udp_errors == CONFIG_BOOLEAN_AUTO &&
                                               (Udp6InErrors ||
                                                Udp6NoPorts ||
                                                Udp6RcvbufErrors ||
                                                Udp6SndbufErrors ||
                                                Udp6InCsumErrors ||
                                                Udp6IgnoredMulti ||
                                                netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        do_udp_errors = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_RcvbufErrors = NULL,
                      *rd_SndbufErrors = NULL,
                      *rd_InErrors     = NULL,
                      *rd_NoPorts      = NULL,
                      *rd_InCsumErrors = NULL,
                      *rd_IgnoredMulti = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "udperrors"
                    , NULL
                    , "udp6"
                    , NULL
                    , "IPv6 UDP Errors"
                    , "events/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NET_SNMP6_NAME
                    , NETDATA_CHART_PRIO_IPV6_UDP_ERRORS
                    , update_every
                    , RRDSET_TYPE_LINE
            );
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rd_RcvbufErrors = rrddim_add(st, "RcvbufErrors", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_SndbufErrors = rrddim_add(st, "SndbufErrors", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InErrors     = rrddim_add(st, "InErrors",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_NoPorts      = rrddim_add(st, "NoPorts",      NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InCsumErrors = rrddim_add(st, "InCsumErrors", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_IgnoredMulti = rrddim_add(st, "IgnoredMulti", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_RcvbufErrors, Udp6RcvbufErrors);
        rrddim_set_by_pointer(st, rd_SndbufErrors, Udp6SndbufErrors);
        rrddim_set_by_pointer(st, rd_InErrors,     Udp6InErrors);
        rrddim_set_by_pointer(st, rd_NoPorts,      Udp6NoPorts);
        rrddim_set_by_pointer(st, rd_InCsumErrors, Udp6InCsumErrors);
        rrddim_set_by_pointer(st, rd_IgnoredMulti, Udp6IgnoredMulti);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_udplite_packets == CONFIG_BOOLEAN_YES || (do_udplite_packets == CONFIG_BOOLEAN_AUTO &&
                                                    (UdpLite6InDatagrams ||
                                                     UdpLite6OutDatagrams ||
                                                     netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        do_udplite_packets = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_received = NULL,
                      *rd_sent     = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "udplitepackets"
                    , NULL
                    , "udplite6"
                    , NULL
                    , "IPv6 UDPlite Packets"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NET_SNMP6_NAME
                    , NETDATA_CHART_PRIO_IPV6_UDPLITE_PACKETS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_received = rrddim_add(st, "InDatagrams",  "received",  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_sent     = rrddim_add(st, "OutDatagrams", "sent",     -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_received, UdpLite6InDatagrams);
        rrddim_set_by_pointer(st, rd_sent,     UdpLite6OutDatagrams);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_udplite_errors == CONFIG_BOOLEAN_YES || (do_udplite_errors == CONFIG_BOOLEAN_AUTO &&
                                                   (UdpLite6InErrors ||
                                                    UdpLite6NoPorts ||
                                                    UdpLite6RcvbufErrors ||
                                                    UdpLite6SndbufErrors ||
                                                    Udp6InCsumErrors ||
                                                    UdpLite6InCsumErrors ||
                                                    netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        do_udplite_errors = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_RcvbufErrors = NULL,
                      *rd_SndbufErrors = NULL,
                      *rd_InErrors     = NULL,
                      *rd_NoPorts      = NULL,
                      *rd_InCsumErrors = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "udpliteerrors"
                    , NULL
                    , "udplite6"
                    , NULL
                    , "IPv6 UDP Lite Errors"
                    , "events/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NET_SNMP6_NAME
                    , NETDATA_CHART_PRIO_IPV6_UDPLITE_ERRORS
                    , update_every
                    , RRDSET_TYPE_LINE
            );
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rd_RcvbufErrors = rrddim_add(st, "RcvbufErrors", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_SndbufErrors = rrddim_add(st, "SndbufErrors", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InErrors     = rrddim_add(st, "InErrors",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_NoPorts      = rrddim_add(st, "NoPorts",      NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InCsumErrors = rrddim_add(st, "InCsumErrors", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_InErrors,     UdpLite6InErrors);
        rrddim_set_by_pointer(st, rd_NoPorts,      UdpLite6NoPorts);
        rrddim_set_by_pointer(st, rd_RcvbufErrors, UdpLite6RcvbufErrors);
        rrddim_set_by_pointer(st, rd_SndbufErrors, UdpLite6SndbufErrors);
        rrddim_set_by_pointer(st, rd_InCsumErrors, UdpLite6InCsumErrors);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_mcast == CONFIG_BOOLEAN_YES || (do_mcast == CONFIG_BOOLEAN_AUTO &&
                                          (Ip6OutMcastOctets ||
                                           Ip6InMcastOctets ||
                                           netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        do_mcast = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_Ip6InMcastOctets  = NULL,
                      *rd_Ip6OutMcastOctets = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "mcast"
                    , NULL
                    , "multicast6"
                    , NULL
                    , "IPv6 Multicast Bandwidth"
                    , "kilobits/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NET_SNMP6_NAME
                    , NETDATA_CHART_PRIO_IPV6_MCAST
                    , update_every
                    , RRDSET_TYPE_AREA
            );
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rd_Ip6InMcastOctets  = rrddim_add(st, "InMcastOctets",  "received",  8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
            rd_Ip6OutMcastOctets = rrddim_add(st, "OutMcastOctets", "sent",     -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_Ip6InMcastOctets,  Ip6InMcastOctets);
        rrddim_set_by_pointer(st, rd_Ip6OutMcastOctets, Ip6OutMcastOctets);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_bcast == CONFIG_BOOLEAN_YES || (do_bcast == CONFIG_BOOLEAN_AUTO &&
                                          (Ip6OutBcastOctets ||
                                           Ip6InBcastOctets ||
                                           netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        do_bcast = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_Ip6InBcastOctets  = NULL,
                      *rd_Ip6OutBcastOctets = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "bcast"
                    , NULL
                    , "broadcast6"
                    , NULL
                    , "IPv6 Broadcast Bandwidth"
                    , "kilobits/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NET_SNMP6_NAME
                    , NETDATA_CHART_PRIO_IPV6_BCAST
                    , update_every
                    , RRDSET_TYPE_AREA
            );
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rd_Ip6InBcastOctets  = rrddim_add(st, "InBcastOctets",  "received",  8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
            rd_Ip6OutBcastOctets = rrddim_add(st, "OutBcastOctets", "sent",     -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_Ip6InBcastOctets,  Ip6InBcastOctets);
        rrddim_set_by_pointer(st, rd_Ip6OutBcastOctets, Ip6OutBcastOctets);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_mcast_p == CONFIG_BOOLEAN_YES || (do_mcast_p == CONFIG_BOOLEAN_AUTO &&
                                            (Ip6OutMcastPkts ||
                                             Ip6InMcastPkts ||
                                             netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        do_mcast_p = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_Ip6InMcastPkts  = NULL,
                      *rd_Ip6OutMcastPkts = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "mcastpkts"
                    , NULL
                    , "multicast6"
                    , NULL
                    , "IPv6 Multicast Packets"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NET_SNMP6_NAME
                    , NETDATA_CHART_PRIO_IPV6_MCAST_PACKETS
                    , update_every
                    , RRDSET_TYPE_LINE
            );
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rd_Ip6InMcastPkts  = rrddim_add(st, "InMcastPkts",  "received", 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_Ip6OutMcastPkts = rrddim_add(st, "OutMcastPkts", "sent",    -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_Ip6InMcastPkts,  Ip6InMcastPkts);
        rrddim_set_by_pointer(st, rd_Ip6OutMcastPkts, Ip6OutMcastPkts);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_icmp == CONFIG_BOOLEAN_YES || (do_icmp == CONFIG_BOOLEAN_AUTO &&
                                         (Icmp6InMsgs ||
                                          Icmp6OutMsgs ||
                                          netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        do_icmp = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_Icmp6InMsgs  = NULL,
                      *rd_Icmp6OutMsgs = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "icmp"
                    , NULL
                    , "icmp6"
                    , NULL
                    , "IPv6 ICMP Messages"
                    , "messages/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NET_SNMP6_NAME
                    , NETDATA_CHART_PRIO_IPV6_ICMP
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_Icmp6InMsgs  = rrddim_add(st, "InMsgs",  "received", 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_Icmp6OutMsgs = rrddim_add(st, "OutMsgs", "sent",    -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_Icmp6InMsgs,  Icmp6InMsgs);
        rrddim_set_by_pointer(st, rd_Icmp6OutMsgs, Icmp6OutMsgs);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_icmp_redir == CONFIG_BOOLEAN_YES || (do_icmp_redir == CONFIG_BOOLEAN_AUTO &&
                                               (Icmp6InRedirects ||
                                                Icmp6OutRedirects ||
                                                netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        do_icmp_redir = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_Icmp6InRedirects  = NULL,
                      *rd_Icmp6OutRedirects = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "icmpredir"
                    , NULL
                    , "icmp6"
                    , NULL
                    , "IPv6 ICMP Redirects"
                    , "redirects/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NET_SNMP6_NAME
                    , NETDATA_CHART_PRIO_IPV6_ICMP_REDIR
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_Icmp6InRedirects  = rrddim_add(st, "InRedirects",  "received", 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_Icmp6OutRedirects = rrddim_add(st, "OutRedirects", "sent",    -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_Icmp6InRedirects,  Icmp6InRedirects);
        rrddim_set_by_pointer(st, rd_Icmp6OutRedirects, Icmp6OutRedirects);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_icmp_errors == CONFIG_BOOLEAN_YES || (do_icmp_errors == CONFIG_BOOLEAN_AUTO &&
                                                (Icmp6InErrors ||
                                                 Icmp6OutErrors ||
                                                 Icmp6InCsumErrors ||
                                                 Icmp6InDestUnreachs ||
                                                 Icmp6InPktTooBigs ||
                                                 Icmp6InTimeExcds ||
                                                 Icmp6InParmProblems ||
                                                 Icmp6OutDestUnreachs ||
                                                 Icmp6OutPktTooBigs ||
                                                 Icmp6OutTimeExcds ||
                                                 Icmp6OutParmProblems ||
                                                 netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        do_icmp_errors = CONFIG_BOOLEAN_YES;
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
                    RRD_TYPE_NET_SNMP6
                    , "icmperrors"
                    , NULL
                    , "icmp6"
                    , NULL
                    , "IPv6 ICMP Errors"
                    , "errors/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NET_SNMP6_NAME
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
        else rrdset_next(st);

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

    // --------------------------------------------------------------------

    if(do_icmp_echos == CONFIG_BOOLEAN_YES || (do_icmp_echos == CONFIG_BOOLEAN_AUTO &&
                                               (Icmp6InEchos ||
                                                Icmp6OutEchos ||
                                                Icmp6InEchoReplies ||
                                                Icmp6OutEchoReplies ||
                                                netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        do_icmp_echos = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_InEchos        = NULL,
                      *rd_OutEchos       = NULL,
                      *rd_InEchoReplies  = NULL,
                      *rd_OutEchoReplies = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "icmpechos"
                    , NULL
                    , "icmp6"
                    , NULL
                    , "IPv6 ICMP Echo"
                    , "messages/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NET_SNMP6_NAME
                    , NETDATA_CHART_PRIO_IPV6_ICMP_ECHOS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_InEchos        = rrddim_add(st, "InEchos",        NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutEchos       = rrddim_add(st, "OutEchos",       NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InEchoReplies  = rrddim_add(st, "InEchoReplies",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutEchoReplies = rrddim_add(st, "OutEchoReplies", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_InEchos,        Icmp6InEchos);
        rrddim_set_by_pointer(st, rd_OutEchos,       Icmp6OutEchos);
        rrddim_set_by_pointer(st, rd_InEchoReplies,  Icmp6InEchoReplies);
        rrddim_set_by_pointer(st, rd_OutEchoReplies, Icmp6OutEchoReplies);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_icmp_groupmemb == CONFIG_BOOLEAN_YES || (do_icmp_groupmemb == CONFIG_BOOLEAN_AUTO &&
                                                   (Icmp6InGroupMembQueries ||
                                                    Icmp6OutGroupMembQueries ||
                                                    Icmp6InGroupMembResponses ||
                                                    Icmp6OutGroupMembResponses ||
                                                    Icmp6InGroupMembReductions ||
                                                    Icmp6OutGroupMembReductions ||
                                                    netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        do_icmp_groupmemb = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_InQueries     = NULL,
                      *rd_OutQueries    = NULL,
                      *rd_InResponses   = NULL,
                      *rd_OutResponses  = NULL,
                      *rd_InReductions  = NULL,
                      *rd_OutReductions = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "groupmemb"
                    , NULL
                    , "icmp6"
                    , NULL
                    , "IPv6 ICMP Group Membership"
                    , "messages/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NET_SNMP6_NAME
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
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_InQueries,     Icmp6InGroupMembQueries);
        rrddim_set_by_pointer(st, rd_OutQueries,    Icmp6OutGroupMembQueries);
        rrddim_set_by_pointer(st, rd_InResponses,   Icmp6InGroupMembResponses);
        rrddim_set_by_pointer(st, rd_OutResponses,  Icmp6OutGroupMembResponses);
        rrddim_set_by_pointer(st, rd_InReductions,  Icmp6InGroupMembReductions);
        rrddim_set_by_pointer(st, rd_OutReductions, Icmp6OutGroupMembReductions);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_icmp_router == CONFIG_BOOLEAN_YES || (do_icmp_router == CONFIG_BOOLEAN_AUTO &&
                                                (Icmp6InRouterSolicits ||
                                                 Icmp6OutRouterSolicits ||
                                                 Icmp6InRouterAdvertisements ||
                                                 Icmp6OutRouterAdvertisements ||
                                                 netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        do_icmp_router = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_InSolicits        = NULL,
                      *rd_OutSolicits       = NULL,
                      *rd_InAdvertisements  = NULL,
                      *rd_OutAdvertisements = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "icmprouter"
                    , NULL
                    , "icmp6"
                    , NULL
                    , "IPv6 Router Messages"
                    , "messages/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NET_SNMP6_NAME
                    , NETDATA_CHART_PRIO_IPV6_ICMP_ROUTER
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_InSolicits        = rrddim_add(st, "InSolicits",        NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutSolicits       = rrddim_add(st, "OutSolicits",       NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InAdvertisements  = rrddim_add(st, "InAdvertisements",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutAdvertisements = rrddim_add(st, "OutAdvertisements", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_InSolicits,        Icmp6InRouterSolicits);
        rrddim_set_by_pointer(st, rd_OutSolicits,       Icmp6OutRouterSolicits);
        rrddim_set_by_pointer(st, rd_InAdvertisements,  Icmp6InRouterAdvertisements);
        rrddim_set_by_pointer(st, rd_OutAdvertisements, Icmp6OutRouterAdvertisements);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_icmp_neighbor == CONFIG_BOOLEAN_YES || (do_icmp_neighbor == CONFIG_BOOLEAN_AUTO &&
                                                  (Icmp6InNeighborSolicits ||
                                                   Icmp6OutNeighborSolicits ||
                                                   Icmp6InNeighborAdvertisements ||
                                                   Icmp6OutNeighborAdvertisements ||
                                                   netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        do_icmp_neighbor = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_InSolicits        = NULL,
                      *rd_OutSolicits       = NULL,
                      *rd_InAdvertisements  = NULL,
                      *rd_OutAdvertisements = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "icmpneighbor"
                    , NULL
                    , "icmp6"
                    , NULL
                    , "IPv6 Neighbor Messages"
                    , "messages/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NET_SNMP6_NAME
                    , NETDATA_CHART_PRIO_IPV6_ICMP_NEIGHBOR
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_InSolicits        = rrddim_add(st, "InSolicits",        NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutSolicits       = rrddim_add(st, "OutSolicits",       NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InAdvertisements  = rrddim_add(st, "InAdvertisements",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutAdvertisements = rrddim_add(st, "OutAdvertisements", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_InSolicits,        Icmp6InNeighborSolicits);
        rrddim_set_by_pointer(st, rd_OutSolicits,       Icmp6OutNeighborSolicits);
        rrddim_set_by_pointer(st, rd_InAdvertisements,  Icmp6InNeighborAdvertisements);
        rrddim_set_by_pointer(st, rd_OutAdvertisements, Icmp6OutNeighborAdvertisements);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_icmp_mldv2 == CONFIG_BOOLEAN_YES || (do_icmp_mldv2 == CONFIG_BOOLEAN_AUTO &&
                                               (Icmp6InMLDv2Reports ||
                                                Icmp6OutMLDv2Reports ||
                                                netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        do_icmp_mldv2 = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_InMLDv2Reports  = NULL,
                      *rd_OutMLDv2Reports = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "icmpmldv2"
                    , NULL
                    , "icmp6"
                    , NULL
                    , "IPv6 ICMP MLDv2 Reports"
                    , "reports/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NET_SNMP6_NAME
                    , NETDATA_CHART_PRIO_IPV6_ICMP_LDV2
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_InMLDv2Reports  = rrddim_add(st, "InMLDv2Reports",  "received", 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutMLDv2Reports = rrddim_add(st, "OutMLDv2Reports", "sent",    -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_InMLDv2Reports,  Icmp6InMLDv2Reports);
        rrddim_set_by_pointer(st, rd_OutMLDv2Reports, Icmp6OutMLDv2Reports);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_icmp_types == CONFIG_BOOLEAN_YES || (do_icmp_types == CONFIG_BOOLEAN_AUTO &&
                                               (Icmp6InType1 ||
                                                Icmp6InType128 ||
                                                Icmp6InType129 ||
                                                Icmp6InType136 ||
                                                Icmp6OutType1 ||
                                                Icmp6OutType128 ||
                                                Icmp6OutType129 ||
                                                Icmp6OutType133 ||
                                                Icmp6OutType135 ||
                                                Icmp6OutType143 ||
                                                netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        do_icmp_types = CONFIG_BOOLEAN_YES;
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
                    RRD_TYPE_NET_SNMP6
                    , "icmptypes"
                    , NULL
                    , "icmp6"
                    , NULL
                    , "IPv6 ICMP Types"
                    , "messages/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NET_SNMP6_NAME
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
        else rrdset_next(st);

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

    // --------------------------------------------------------------------

    if(do_ect == CONFIG_BOOLEAN_YES || (do_ect == CONFIG_BOOLEAN_AUTO &&
                                        (Ip6InNoECTPkts ||
                                         Ip6InECT1Pkts ||
                                         Ip6InECT0Pkts ||
                                         Ip6InCEPkts ||
                                         netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        do_ect = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_InNoECTPkts = NULL,
                      *rd_InECT1Pkts  = NULL,
                      *rd_InECT0Pkts  = NULL,
                      *rd_InCEPkts    = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "ect"
                    , NULL
                    , "packets"
                    , NULL
                    , "IPv6 ECT Packets"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NET_SNMP6_NAME
                    , NETDATA_CHART_PRIO_IPV6_ECT
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_InNoECTPkts = rrddim_add(st, "InNoECTPkts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InECT1Pkts  = rrddim_add(st, "InECT1Pkts",  NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InECT0Pkts  = rrddim_add(st, "InECT0Pkts",  NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InCEPkts    = rrddim_add(st, "InCEPkts",    NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_InNoECTPkts, Ip6InNoECTPkts);
        rrddim_set_by_pointer(st, rd_InECT1Pkts,  Ip6InECT1Pkts);
        rrddim_set_by_pointer(st, rd_InECT0Pkts,  Ip6InECT0Pkts);
        rrddim_set_by_pointer(st, rd_InCEPkts,    Ip6InCEPkts);
        rrdset_done(st);
    }

    return 0;
}


#include "common.h"

#define RRD_TYPE_NET_SNMP6          "ipv6"

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

    RRDSET *st;

    // --------------------------------------------------------------------

    if(do_bandwidth == CONFIG_BOOLEAN_YES || (do_bandwidth == CONFIG_BOOLEAN_AUTO && (Ip6InOctets || Ip6OutOctets))) {
        do_bandwidth = CONFIG_BOOLEAN_YES;
        st = rrdset_find_localhost("system.ipv6");
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "system"
                    , "ipv6"
                    , NULL
                    , "network"
                    , NULL
                    , "IPv6 Bandwidth"
                    , "kilobits/s"
                    , "proc"
                    , "net/snmp6"
                    , 500
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rrddim_add(st, "received", NULL,  8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "sent",     NULL, -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set(st, "sent", Ip6OutOctets);
        rrddim_set(st, "received", Ip6InOctets);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_ip_packets == CONFIG_BOOLEAN_YES || (do_ip_packets == CONFIG_BOOLEAN_AUTO && (Ip6InReceives || Ip6OutRequests || Ip6InDelivers || Ip6OutForwDatagrams))) {
        do_ip_packets = CONFIG_BOOLEAN_YES;
        st = rrdset_find_localhost(RRD_TYPE_NET_SNMP6 ".packets");
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "packets"
                    , NULL
                    , "packets"
                    , NULL
                    , "IPv6 Packets"
                    , "packets/s"
                    , "proc"
                    , "net/snmp6"
                    , 3000
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rrddim_add(st, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "sent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "forwarded", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "delivers", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set(st, "sent", Ip6OutRequests);
        rrddim_set(st, "received", Ip6InReceives);
        rrddim_set(st, "forwarded", Ip6OutForwDatagrams);
        rrddim_set(st, "delivers", Ip6InDelivers);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_ip_fragsout == CONFIG_BOOLEAN_YES || (do_ip_fragsout == CONFIG_BOOLEAN_AUTO && (Ip6FragOKs || Ip6FragFails || Ip6FragCreates))) {
        do_ip_fragsout = CONFIG_BOOLEAN_YES;
        st = rrdset_find_localhost(RRD_TYPE_NET_SNMP6 ".fragsout");
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "fragsout"
                    , NULL
                    , "fragments"
                    , NULL
                    , "IPv6 Fragments Sent"
                    , "packets/s"
                    , "proc"
                    , "net/snmp6"
                    , 3010
                    , update_every
                    , RRDSET_TYPE_LINE
            );
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rrddim_add(st, "ok", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "failed", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "all", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set(st, "ok", Ip6FragOKs);
        rrddim_set(st, "failed", Ip6FragFails);
        rrddim_set(st, "all", Ip6FragCreates);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_ip_fragsin == CONFIG_BOOLEAN_YES || (do_ip_fragsin == CONFIG_BOOLEAN_AUTO
        && (
            Ip6ReasmOKs
            || Ip6ReasmFails
            || Ip6ReasmTimeout
            || Ip6ReasmReqds
            ))) {
        do_ip_fragsin = CONFIG_BOOLEAN_YES;
        st = rrdset_find_localhost(RRD_TYPE_NET_SNMP6 ".fragsin");
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "fragsin"
                    , NULL
                    , "fragments"
                    , NULL
                    , "IPv6 Fragments Reassembly"
                    , "packets/s"
                    , "proc"
                    , "net/snmp6"
                    , 3011
                    , update_every
                    , RRDSET_TYPE_LINE);
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rrddim_add(st, "ok", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "failed", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "timeout", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "all", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set(st, "ok", Ip6ReasmOKs);
        rrddim_set(st, "failed", Ip6ReasmFails);
        rrddim_set(st, "timeout", Ip6ReasmTimeout);
        rrddim_set(st, "all", Ip6ReasmReqds);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_ip_errors == CONFIG_BOOLEAN_YES || (do_ip_errors == CONFIG_BOOLEAN_AUTO
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
        do_ip_errors = CONFIG_BOOLEAN_YES;
        st = rrdset_find_localhost(RRD_TYPE_NET_SNMP6 ".errors");
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "errors"
                    , NULL
                    , "errors"
                    , NULL
                    , "IPv6 Errors"
                    , "packets/s"
                    , "proc"
                    , "net/snmp6"
                    , 3002
                    , update_every
                    , RRDSET_TYPE_LINE
            );
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rrddim_add(st, "InDiscards", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "OutDiscards", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrddim_add(st, "InHdrErrors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "InAddrErrors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "InUnknownProtos", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "InTooBigErrors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "InTruncatedPkts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "InNoRoutes", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrddim_add(st, "OutNoRoutes", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
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

    if(do_udp_packets == CONFIG_BOOLEAN_YES || (do_udp_packets == CONFIG_BOOLEAN_AUTO && (Udp6InDatagrams || Udp6OutDatagrams))) {
        do_udp_packets = CONFIG_BOOLEAN_YES;
        st = rrdset_find_localhost(RRD_TYPE_NET_SNMP6 ".udppackets");
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "udppackets"
                    , NULL
                    , "udp"
                    , NULL
                    , "IPv6 UDP Packets"
                    , "packets/s"
                    , "proc"
                    , "net/snmp6"
                    , 3601
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rrddim_add(st, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "sent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set(st, "received", Udp6InDatagrams);
        rrddim_set(st, "sent", Udp6OutDatagrams);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_udp_errors == CONFIG_BOOLEAN_YES || (do_udp_errors == CONFIG_BOOLEAN_AUTO
        && (
            Udp6InErrors
            || Udp6NoPorts
            || Udp6RcvbufErrors
            || Udp6SndbufErrors
            || Udp6InCsumErrors
            || Udp6IgnoredMulti
        ))) {
        do_udp_errors = CONFIG_BOOLEAN_YES;
        st = rrdset_find_localhost(RRD_TYPE_NET_SNMP6 ".udperrors");
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "udperrors"
                    , NULL
                    , "udp"
                    , NULL
                    , "IPv6 UDP Errors"
                    , "events/s"
                    , "proc"
                    , "net/snmp6"
                    , 3701
                    , update_every
                    , RRDSET_TYPE_LINE
            );
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rrddim_add(st, "RcvbufErrors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "SndbufErrors", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "InErrors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "NoPorts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "InCsumErrors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "IgnoredMulti", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
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

    if(do_udplite_packets == CONFIG_BOOLEAN_YES || (do_udplite_packets == CONFIG_BOOLEAN_AUTO && (UdpLite6InDatagrams || UdpLite6OutDatagrams))) {
        do_udplite_packets = CONFIG_BOOLEAN_YES;
        st = rrdset_find_localhost(RRD_TYPE_NET_SNMP6 ".udplitepackets");
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "udplitepackets"
                    , NULL
                    , "udplite"
                    , NULL
                    , "IPv6 UDPlite Packets"
                    , "packets/s"
                    , "proc"
                    , "net/snmp6"
                    , 3601
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rrddim_add(st, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "sent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set(st, "received", UdpLite6InDatagrams);
        rrddim_set(st, "sent", UdpLite6OutDatagrams);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_udplite_errors == CONFIG_BOOLEAN_YES || (do_udplite_errors == CONFIG_BOOLEAN_AUTO
        && (
            UdpLite6InErrors
            || UdpLite6NoPorts
            || UdpLite6RcvbufErrors
            || UdpLite6SndbufErrors
            || Udp6InCsumErrors
            || UdpLite6InCsumErrors
        ))) {
        do_udplite_errors = CONFIG_BOOLEAN_YES;
        st = rrdset_find_localhost(RRD_TYPE_NET_SNMP6 ".udpliteerrors");
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "udpliteerrors"
                    , NULL
                    , "udplite"
                    , NULL
                    , "IPv6 UDP Lite Errors"
                    , "events/s"
                    , "proc"
                    , "net/snmp6"
                    , 3701
                    , update_every
                    , RRDSET_TYPE_LINE
            );
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rrddim_add(st, "RcvbufErrors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "SndbufErrors", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "InErrors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "NoPorts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "InCsumErrors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
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

    if(do_mcast == CONFIG_BOOLEAN_YES || (do_mcast == CONFIG_BOOLEAN_AUTO && (Ip6OutMcastOctets || Ip6InMcastOctets))) {
        do_mcast = CONFIG_BOOLEAN_YES;
        st = rrdset_find_localhost(RRD_TYPE_NET_SNMP6 ".mcast");
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "mcast"
                    , NULL
                    , "multicast"
                    , NULL
                    , "IPv6 Multicast Bandwidth"
                    , "kilobits/s"
                    , "proc"
                    , "net/snmp6"
                    , 9000
                    , update_every
                    , RRDSET_TYPE_AREA
            );
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rrddim_add(st, "received", NULL,  8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "sent",     NULL, -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set(st, "sent", Ip6OutMcastOctets);
        rrddim_set(st, "received", Ip6InMcastOctets);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_bcast == CONFIG_BOOLEAN_YES || (do_bcast == CONFIG_BOOLEAN_AUTO && (Ip6OutBcastOctets || Ip6InBcastOctets))) {
        do_bcast = CONFIG_BOOLEAN_YES;
        st = rrdset_find_localhost(RRD_TYPE_NET_SNMP6 ".bcast");
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "bcast"
                    , NULL
                    , "broadcast"
                    , NULL
                    , "IPv6 Broadcast Bandwidth"
                    , "kilobits/s"
                    , "proc"
                    , "net/snmp6"
                    , 8000
                    , update_every
                    , RRDSET_TYPE_AREA
            );
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rrddim_add(st, "received", NULL,  8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "sent",     NULL, -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set(st, "sent", Ip6OutBcastOctets);
        rrddim_set(st, "received", Ip6InBcastOctets);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_mcast_p == CONFIG_BOOLEAN_YES || (do_mcast_p == CONFIG_BOOLEAN_AUTO && (Ip6OutMcastPkts || Ip6InMcastPkts))) {
        do_mcast_p = CONFIG_BOOLEAN_YES;
        st = rrdset_find_localhost(RRD_TYPE_NET_SNMP6 ".mcastpkts");
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "mcastpkts"
                    , NULL
                    , "multicast"
                    , NULL
                    , "IPv6 Multicast Packets"
                    , "packets/s"
                    , "proc"
                    , "net/snmp6"
                    , 9500
                    , update_every
                    , RRDSET_TYPE_LINE
            );
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rrddim_add(st, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "sent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set(st, "sent", Ip6OutMcastPkts);
        rrddim_set(st, "received", Ip6InMcastPkts);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_icmp == CONFIG_BOOLEAN_YES || (do_icmp == CONFIG_BOOLEAN_AUTO && (Icmp6InMsgs || Icmp6OutMsgs))) {
        do_icmp = CONFIG_BOOLEAN_YES;
        st = rrdset_find_localhost(RRD_TYPE_NET_SNMP6 ".icmp");
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "icmp"
                    , NULL
                    , "icmp"
                    , NULL
                    , "IPv6 ICMP Messages"
                    , "messages/s"
                    , "proc"
                    , "net/snmp6"
                    , 10000
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rrddim_add(st, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "sent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set(st, "sent", Icmp6InMsgs);
        rrddim_set(st, "received", Icmp6OutMsgs);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_icmp_redir == CONFIG_BOOLEAN_YES || (do_icmp_redir == CONFIG_BOOLEAN_AUTO && (Icmp6InRedirects || Icmp6OutRedirects))) {
        do_icmp_redir = CONFIG_BOOLEAN_YES;
        st = rrdset_find_localhost(RRD_TYPE_NET_SNMP6 ".icmpredir");
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "icmpredir"
                    , NULL
                    , "icmp"
                    , NULL
                    , "IPv6 ICMP Redirects"
                    , "redirects/s"
                    , "proc"
                    , "net/snmp6"
                    , 10050
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rrddim_add(st, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "sent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set(st, "sent", Icmp6InRedirects);
        rrddim_set(st, "received", Icmp6OutRedirects);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_icmp_errors == CONFIG_BOOLEAN_YES || (do_icmp_errors == CONFIG_BOOLEAN_AUTO
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
        do_icmp_errors = CONFIG_BOOLEAN_YES;
        st = rrdset_find_localhost(RRD_TYPE_NET_SNMP6 ".icmperrors");
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "icmperrors"
                    , NULL
                    , "icmp"
                    , NULL
                    , "IPv6 ICMP Errors"
                    , "errors/s"
                    , "proc"
                    , "net/snmp6"
                    , 10100
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rrddim_add(st, "InErrors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "OutErrors", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrddim_add(st, "InCsumErrors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "InDestUnreachs", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "InPktTooBigs", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "InTimeExcds", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "InParmProblems", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "OutDestUnreachs", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "OutPktTooBigs", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "OutTimeExcds", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "OutParmProblems", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
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

    if(do_icmp_echos == CONFIG_BOOLEAN_YES || (do_icmp_echos == CONFIG_BOOLEAN_AUTO
        && (
            Icmp6InEchos
            || Icmp6OutEchos
            || Icmp6InEchoReplies
            || Icmp6OutEchoReplies
        ))) {
        do_icmp_echos = CONFIG_BOOLEAN_YES;
        st = rrdset_find_localhost(RRD_TYPE_NET_SNMP6 ".icmpechos");
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "icmpechos"
                    , NULL
                    , "icmp"
                    , NULL
                    , "IPv6 ICMP Echo"
                    , "messages/s"
                    , "proc"
                    , "net/snmp6"
                    , 10200
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rrddim_add(st, "InEchos", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "OutEchos", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "InEchoReplies", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "OutEchoReplies", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set(st, "InEchos", Icmp6InEchos);
        rrddim_set(st, "OutEchos", Icmp6OutEchos);
        rrddim_set(st, "InEchoReplies", Icmp6InEchoReplies);
        rrddim_set(st, "OutEchoReplies", Icmp6OutEchoReplies);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_icmp_groupmemb == CONFIG_BOOLEAN_YES || (do_icmp_groupmemb == CONFIG_BOOLEAN_AUTO
        && (
            Icmp6InGroupMembQueries
            || Icmp6OutGroupMembQueries
            || Icmp6InGroupMembResponses
            || Icmp6OutGroupMembResponses
            || Icmp6InGroupMembReductions
            || Icmp6OutGroupMembReductions
        ))) {
        do_icmp_groupmemb = CONFIG_BOOLEAN_YES;
        st = rrdset_find_localhost(RRD_TYPE_NET_SNMP6 ".groupmemb");
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "groupmemb"
                    , NULL
                    , "icmp"
                    , NULL
                    , "IPv6 ICMP Group Membership"
                    , "messages/s"
                    , "proc"
                    , "net/snmp6"
                    , 10300
                    , update_every
                    , RRDSET_TYPE_LINE);

            rrddim_add(st, "InQueries", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "OutQueries", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "InResponses", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "OutResponses", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "InReductions", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "OutReductions", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
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

    if(do_icmp_router == CONFIG_BOOLEAN_YES || (do_icmp_router == CONFIG_BOOLEAN_AUTO
        && (
            Icmp6InRouterSolicits
            || Icmp6OutRouterSolicits
            || Icmp6InRouterAdvertisements
            || Icmp6OutRouterAdvertisements
        ))) {
        do_icmp_router = CONFIG_BOOLEAN_YES;
        st = rrdset_find_localhost(RRD_TYPE_NET_SNMP6 ".icmprouter");
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "icmprouter"
                    , NULL
                    , "icmp"
                    , NULL
                    , "IPv6 Router Messages"
                    , "messages/s"
                    , "proc"
                    , "net/snmp6"
                    , 10400
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rrddim_add(st, "InSolicits", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "OutSolicits", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "InAdvertisements", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "OutAdvertisements", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set(st, "InSolicits", Icmp6InRouterSolicits);
        rrddim_set(st, "OutSolicits", Icmp6OutRouterSolicits);
        rrddim_set(st, "InAdvertisements", Icmp6InRouterAdvertisements);
        rrddim_set(st, "OutAdvertisements", Icmp6OutRouterAdvertisements);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_icmp_neighbor == CONFIG_BOOLEAN_YES || (do_icmp_neighbor == CONFIG_BOOLEAN_AUTO
        && (
            Icmp6InNeighborSolicits
            || Icmp6OutNeighborSolicits
            || Icmp6InNeighborAdvertisements
            || Icmp6OutNeighborAdvertisements
        ))) {
        do_icmp_neighbor = CONFIG_BOOLEAN_YES;
        st = rrdset_find_localhost(RRD_TYPE_NET_SNMP6 ".icmpneighbor");
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "icmpneighbor"
                    , NULL
                    , "icmp"
                    , NULL
                    , "IPv6 Neighbor Messages"
                    , "messages/s"
                    , "proc"
                    , "net/snmp6"
                    , 10500
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rrddim_add(st, "InSolicits", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "OutSolicits", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "InAdvertisements", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "OutAdvertisements", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set(st, "InSolicits", Icmp6InNeighborSolicits);
        rrddim_set(st, "OutSolicits", Icmp6OutNeighborSolicits);
        rrddim_set(st, "InAdvertisements", Icmp6InNeighborAdvertisements);
        rrddim_set(st, "OutAdvertisements", Icmp6OutNeighborAdvertisements);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_icmp_mldv2 == CONFIG_BOOLEAN_YES || (do_icmp_mldv2 == CONFIG_BOOLEAN_AUTO && (Icmp6InMLDv2Reports || Icmp6OutMLDv2Reports))) {
        do_icmp_mldv2 = CONFIG_BOOLEAN_YES;
        st = rrdset_find_localhost(RRD_TYPE_NET_SNMP6 ".icmpmldv2");
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "icmpmldv2"
                    , NULL
                    , "icmp"
                    , NULL
                    , "IPv6 ICMP MLDv2 Reports"
                    , "reports/s"
                    , "proc"
                    , "net/snmp6"
                    , 10600
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rrddim_add(st, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "sent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set(st, "sent", Icmp6InMLDv2Reports);
        rrddim_set(st, "received", Icmp6OutMLDv2Reports);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_icmp_types == CONFIG_BOOLEAN_YES || (do_icmp_types == CONFIG_BOOLEAN_AUTO
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
        do_icmp_types = CONFIG_BOOLEAN_YES;
        st = rrdset_find_localhost(RRD_TYPE_NET_SNMP6 ".icmptypes");
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "icmptypes"
                    , NULL
                    , "icmp"
                    , NULL
                    , "IPv6 ICMP Types"
                    , "messages/s"
                    , "proc"
                    , "net/snmp6"
                    , 10700
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rrddim_add(st, "InType1", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "InType128", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "InType129", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "InType136", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "OutType1", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "OutType128", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "OutType129", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "OutType133", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "OutType135", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "OutType143", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
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

    if(do_ect == CONFIG_BOOLEAN_YES || (do_ect == CONFIG_BOOLEAN_AUTO
        && (
            Ip6InNoECTPkts
            || Ip6InECT1Pkts
            || Ip6InECT0Pkts
            || Ip6InCEPkts
        ))) {
        do_ect = CONFIG_BOOLEAN_YES;
        st = rrdset_find_localhost(RRD_TYPE_NET_SNMP6 ".ect");
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_SNMP6
                    , "ect"
                    , NULL
                    , "packets"
                    , NULL
                    , "IPv6 ECT Packets"
                    , "packets/s"
                    , "proc"
                    , "net/snmp6"
                    , 10800
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rrddim_add(st, "InNoECTPkts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "InECT1Pkts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "InECT0Pkts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "InCEPkts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
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


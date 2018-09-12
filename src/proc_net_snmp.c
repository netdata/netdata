// SPDX-License-Identifier: GPL-3.0+
#include "common.h"

#define RRD_TYPE_NET_SNMP           "ipv4"

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

int do_proc_net_snmp(int update_every, usec_t dt) {
    (void)dt;

    static procfile *ff = NULL;
    static int do_ip_packets = -1, do_ip_fragsout = -1, do_ip_fragsin = -1, do_ip_errors = -1,
        do_tcp_sockets = -1, do_tcp_packets = -1, do_tcp_errors = -1, do_tcp_handshake = -1, do_tcp_opens = -1,
        do_udp_packets = -1, do_udp_errors = -1, do_icmp_packets = -1, do_icmpmsg = -1, do_udplite_packets = -1;
    static uint32_t hash_ip = 0, hash_icmp = 0, hash_tcp = 0, hash_udp = 0, hash_icmpmsg = 0, hash_udplite = 0;

    static ARL_BASE *arl_ip = NULL,
             *arl_icmp = NULL,
             *arl_icmpmsg = NULL,
             *arl_tcp = NULL,
             *arl_udp = NULL,
             *arl_udplite = NULL;

    static RRDVAR *tcp_max_connections_var = NULL;
    static ssize_t last_max_connections = 0;

    if(unlikely(!arl_ip)) {
        do_ip_packets       = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp", "ipv4 packets", CONFIG_BOOLEAN_AUTO);
        do_ip_fragsout      = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp", "ipv4 fragments sent", CONFIG_BOOLEAN_AUTO);
        do_ip_fragsin       = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp", "ipv4 fragments assembly", CONFIG_BOOLEAN_AUTO);
        do_ip_errors        = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp", "ipv4 errors", CONFIG_BOOLEAN_AUTO);
        do_tcp_sockets      = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp", "ipv4 TCP connections", CONFIG_BOOLEAN_AUTO);
        do_tcp_packets      = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp", "ipv4 TCP packets", CONFIG_BOOLEAN_AUTO);
        do_tcp_errors       = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp", "ipv4 TCP errors", CONFIG_BOOLEAN_AUTO);
        do_tcp_opens        = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp", "ipv4 TCP opens", CONFIG_BOOLEAN_AUTO);
        do_tcp_handshake    = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp", "ipv4 TCP handshake issues", CONFIG_BOOLEAN_AUTO);
        do_udp_packets      = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp", "ipv4 UDP packets", CONFIG_BOOLEAN_AUTO);
        do_udp_errors       = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp", "ipv4 UDP errors", CONFIG_BOOLEAN_AUTO);
        do_icmp_packets     = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp", "ipv4 ICMP packets", CONFIG_BOOLEAN_AUTO);
        do_icmpmsg          = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp", "ipv4 ICMP messages", CONFIG_BOOLEAN_AUTO);
        do_udplite_packets  = config_get_boolean_ondemand("plugin:proc:/proc/net/snmp", "ipv4 UDPLite packets", CONFIG_BOOLEAN_AUTO);

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
        arl_expect(arl_tcp, "MaxConn", &snmp_root.tcp_MaxConn);
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

        tcp_max_connections_var = rrdvar_custom_host_variable_create(localhost, "tcp_max_connections");
    }

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/net/snmp");
        ff = procfile_open(config_get("plugin:proc:/proc/net/snmp", "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 0; // we return 0, so that we will retry to open it next time

    size_t lines = procfile_lines(ff), l;
    size_t words, w;

    for(l = 0; l < lines ;l++) {
        char *key = procfile_lineword(ff, l, 0);
        uint32_t hash = simple_hash(key);

        if(unlikely(hash == hash_ip && strcmp(key, "Ip") == 0)) {
            size_t h = l++;

            if(strcmp(procfile_lineword(ff, l, 0), "Ip") != 0) {
                error("Cannot read Ip line from /proc/net/snmp.");
                break;
            }

            words = procfile_linewords(ff, l);
            if(words < 3) {
                error("Cannot read /proc/net/snmp Ip line. Expected 3+ params, read %zu.", words);
                continue;
            }

            arl_begin(arl_ip);
            for(w = 1; w < words ; w++) {
                if (unlikely(arl_check(arl_ip, procfile_lineword(ff, h, w), procfile_lineword(ff, l, w)) != 0))
                    break;
            }

            // --------------------------------------------------------------------

            if(do_ip_packets == CONFIG_BOOLEAN_YES || (do_ip_packets == CONFIG_BOOLEAN_AUTO && (snmp_root.ip_OutRequests || snmp_root.ip_InReceives || snmp_root.ip_ForwDatagrams || snmp_root.ip_InDelivers))) {
                do_ip_packets = CONFIG_BOOLEAN_YES;

                static RRDSET *st = NULL;
                static RRDDIM *rd_InReceives = NULL,
                              *rd_OutRequests = NULL,
                              *rd_ForwDatagrams = NULL,
                              *rd_InDelivers = NULL;

                if(unlikely(!st)) {
                    st = rrdset_create_localhost(
                            RRD_TYPE_NET_SNMP
                            , "packets"
                            , NULL
                            , "packets"
                            , NULL
                            , "IPv4 Packets"
                            , "packets/s"
                            , "proc"
                            , "net/snmp"
                            , NETDATA_CHART_PRIO_IPV4_PACKETS
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rd_InReceives    = rrddim_add(st, "InReceives",    "received",  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_OutRequests   = rrddim_add(st, "OutRequests",   "sent",     -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_ForwDatagrams = rrddim_add(st, "ForwDatagrams", "forwarded", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_InDelivers    = rrddim_add(st, "InDelivers",    "delivered", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set_by_pointer(st, rd_OutRequests,   (collected_number)snmp_root.ip_OutRequests);
                rrddim_set_by_pointer(st, rd_InReceives,    (collected_number)snmp_root.ip_InReceives);
                rrddim_set_by_pointer(st, rd_ForwDatagrams, (collected_number)snmp_root.ip_ForwDatagrams);
                rrddim_set_by_pointer(st, rd_InDelivers,    (collected_number)snmp_root.ip_InDelivers);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_ip_fragsout == CONFIG_BOOLEAN_YES || (do_ip_fragsout == CONFIG_BOOLEAN_AUTO && (snmp_root.ip_FragOKs || snmp_root.ip_FragFails || snmp_root.ip_FragCreates))) {
                do_ip_fragsout = CONFIG_BOOLEAN_YES;

                static RRDSET *st = NULL;
                static RRDDIM *rd_FragOKs = NULL,
                              *rd_FragFails = NULL,
                              *rd_FragCreates = NULL;

                if(unlikely(!st)) {
                    st = rrdset_create_localhost(
                            RRD_TYPE_NET_SNMP
                            , "fragsout"
                            , NULL
                            , "fragments"
                            , NULL
                            , "IPv4 Fragments Sent"
                            , "packets/s"
                            , "proc"
                            , "net/snmp"
                            , NETDATA_CHART_PRIO_IPV4_FRAGMENTS
                            , update_every
                            , RRDSET_TYPE_LINE
                    );
                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rd_FragOKs     = rrddim_add(st, "FragOKs",     "ok",      1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_FragFails   = rrddim_add(st, "FragFails",   "failed", -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_FragCreates = rrddim_add(st, "FragCreates", "created", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set_by_pointer(st, rd_FragOKs,     (collected_number)snmp_root.ip_FragOKs);
                rrddim_set_by_pointer(st, rd_FragFails,   (collected_number)snmp_root.ip_FragFails);
                rrddim_set_by_pointer(st, rd_FragCreates, (collected_number)snmp_root.ip_FragCreates);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_ip_fragsin == CONFIG_BOOLEAN_YES || (do_ip_fragsin == CONFIG_BOOLEAN_AUTO && (snmp_root.ip_ReasmOKs || snmp_root.ip_ReasmFails || snmp_root.ip_ReasmReqds))) {
                do_ip_fragsin = CONFIG_BOOLEAN_YES;

                static RRDSET *st = NULL;
                static RRDDIM *rd_ReasmOKs = NULL,
                              *rd_ReasmFails = NULL,
                              *rd_ReasmReqds = NULL;

                if(unlikely(!st)) {
                    st = rrdset_create_localhost(
                            RRD_TYPE_NET_SNMP
                            , "fragsin"
                            , NULL
                            , "fragments"
                            , NULL
                            , "IPv4 Fragments Reassembly"
                            , "packets/s"
                            , "proc"
                            , "net/snmp"
                            , NETDATA_CHART_PRIO_IPV4_FRAGMENTS + 1
                            , update_every
                            , RRDSET_TYPE_LINE
                    );
                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rd_ReasmOKs   = rrddim_add(st, "ReasmOKs",   "ok",      1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_ReasmFails = rrddim_add(st, "ReasmFails", "failed", -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_ReasmReqds = rrddim_add(st, "ReasmReqds", "all",     1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set_by_pointer(st, rd_ReasmOKs,   (collected_number)snmp_root.ip_ReasmOKs);
                rrddim_set_by_pointer(st, rd_ReasmFails, (collected_number)snmp_root.ip_ReasmFails);
                rrddim_set_by_pointer(st, rd_ReasmReqds, (collected_number)snmp_root.ip_ReasmReqds);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_ip_errors == CONFIG_BOOLEAN_YES || (do_ip_errors == CONFIG_BOOLEAN_AUTO && (snmp_root.ip_InDiscards || snmp_root.ip_OutDiscards || snmp_root.ip_InHdrErrors || snmp_root.ip_InAddrErrors || snmp_root.ip_InUnknownProtos || snmp_root.ip_OutNoRoutes))) {
                do_ip_errors = CONFIG_BOOLEAN_YES;

                static RRDSET *st = NULL;
                static RRDDIM *rd_InDiscards = NULL,
                              *rd_OutDiscards = NULL,
                              *rd_InHdrErrors = NULL,
                              *rd_OutNoRoutes = NULL,
                              *rd_InAddrErrors = NULL,
                              *rd_InUnknownProtos = NULL;

                if(unlikely(!st)) {
                    st = rrdset_create_localhost(
                            RRD_TYPE_NET_SNMP
                            , "errors"
                            , NULL
                            , "errors"
                            , NULL
                            , "IPv4 Errors"
                            , "packets/s"
                            , "proc"
                            , "net/snmp"
                            , NETDATA_CHART_PRIO_IPV4_ERRORS
                            , update_every
                            , RRDSET_TYPE_LINE
                    );
                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rd_InDiscards      = rrddim_add(st, "InDiscards",      NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_OutDiscards     = rrddim_add(st, "OutDiscards",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

                    rd_InHdrErrors     = rrddim_add(st, "InHdrErrors",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_OutNoRoutes     = rrddim_add(st, "OutNoRoutes",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

                    rd_InAddrErrors    = rrddim_add(st, "InAddrErrors",    NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_InUnknownProtos = rrddim_add(st, "InUnknownProtos", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set_by_pointer(st, rd_InDiscards,      (collected_number)snmp_root.ip_InDiscards);
                rrddim_set_by_pointer(st, rd_OutDiscards,     (collected_number)snmp_root.ip_OutDiscards);
                rrddim_set_by_pointer(st, rd_InHdrErrors,     (collected_number)snmp_root.ip_InHdrErrors);
                rrddim_set_by_pointer(st, rd_InAddrErrors,    (collected_number)snmp_root.ip_InAddrErrors);
                rrddim_set_by_pointer(st, rd_InUnknownProtos, (collected_number)snmp_root.ip_InUnknownProtos);
                rrddim_set_by_pointer(st, rd_OutNoRoutes,     (collected_number)snmp_root.ip_OutNoRoutes);
                rrdset_done(st);
            }
        }
        else if(unlikely(hash == hash_icmp && strcmp(key, "Icmp") == 0)) {
            size_t h = l++;

            if(strcmp(procfile_lineword(ff, l, 0), "Icmp") != 0) {
                error("Cannot read Icmp line from /proc/net/snmp.");
                break;
            }

            words = procfile_linewords(ff, l);
            if(words < 3) {
                error("Cannot read /proc/net/snmp Icmp line. Expected 3+ params, read %zu.", words);
                continue;
            }

            arl_begin(arl_icmp);
            for(w = 1; w < words ; w++) {
                if (unlikely(arl_check(arl_icmp, procfile_lineword(ff, h, w), procfile_lineword(ff, l, w)) != 0))
                    break;
            }

            // --------------------------------------------------------------------

            if(do_icmp_packets == CONFIG_BOOLEAN_YES || (do_icmp_packets == CONFIG_BOOLEAN_AUTO && (snmp_root.icmp_InMsgs || snmp_root.icmp_OutMsgs || snmp_root.icmp_InErrors || snmp_root.icmp_OutErrors || snmp_root.icmp_InCsumErrors))) {
                do_icmp_packets = CONFIG_BOOLEAN_YES;

                {
                    static RRDSET *st_packets = NULL;
                    static RRDDIM *rd_InMsgs = NULL,
                                  *rd_OutMsgs = NULL;

                    if(unlikely(!st_packets)) {
                        st_packets = rrdset_create_localhost(
                                RRD_TYPE_NET_SNMP
                                , "icmp"
                                , NULL
                                , "icmp"
                                , NULL
                                , "IPv4 ICMP Packets"
                                , "packets/s"
                                , "proc"
                                , "net/snmp"
                                , NETDATA_CHART_PRIO_IPV4_ICMP
                                , update_every
                                , RRDSET_TYPE_LINE
                        );

                        rd_InMsgs  = rrddim_add(st_packets, "InMsgs",  "received",  1, 1, RRD_ALGORITHM_INCREMENTAL);
                        rd_OutMsgs = rrddim_add(st_packets, "OutMsgs", "sent",     -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    }
                    else rrdset_next(st_packets);

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
                                RRD_TYPE_NET_SNMP
                                , "icmp_errors"
                                , NULL
                                , "icmp"
                                , NULL
                                , "IPv4 ICMP Errors"
                                , "packets/s"
                                , "proc"
                                , "net/snmp"
                                , NETDATA_CHART_PRIO_IPV4_ICMP + 1
                                , update_every
                                , RRDSET_TYPE_LINE
                        );

                        rd_InErrors     = rrddim_add(st_errors, "InErrors",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                        rd_OutErrors    = rrddim_add(st_errors, "OutErrors",    NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                        rd_InCsumErrors = rrddim_add(st_errors, "InCsumErrors", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    }
                    else rrdset_next(st_errors);

                    rrddim_set_by_pointer(st_errors, rd_InErrors,     (collected_number)snmp_root.icmp_InErrors);
                    rrddim_set_by_pointer(st_errors, rd_OutErrors,    (collected_number)snmp_root.icmp_OutErrors);
                    rrddim_set_by_pointer(st_errors, rd_InCsumErrors, (collected_number)snmp_root.icmp_InCsumErrors);

                    rrdset_done(st_errors);
                }
            }
        }
        else if(unlikely(hash == hash_icmpmsg && strcmp(key, "IcmpMsg") == 0)) {
            size_t h = l++;

            if(strcmp(procfile_lineword(ff, l, 0), "IcmpMsg") != 0) {
                error("Cannot read IcmpMsg line from /proc/net/snmp.");
                break;
            }

            words = procfile_linewords(ff, l);
            if(words < 2) {
                error("Cannot read /proc/net/snmp IcmpMsg line. Expected 2+ params, read %zu.", words);
                continue;
            }

            arl_begin(arl_icmpmsg);
            for(w = 1; w < words ; w++) {
                if (unlikely(arl_check(arl_icmpmsg, procfile_lineword(ff, h, w), procfile_lineword(ff, l, w)) != 0))
                    break;
            }

            // --------------------------------------------------------------------

            if(do_icmpmsg == CONFIG_BOOLEAN_YES || (do_icmpmsg == CONFIG_BOOLEAN_AUTO && (
                    snmp_root.icmpmsg_InEchoReps
                    || snmp_root.icmpmsg_OutEchoReps
                    || snmp_root.icmpmsg_InDestUnreachs
                    || snmp_root.icmpmsg_OutDestUnreachs
                    || snmp_root.icmpmsg_InRedirects
                    || snmp_root.icmpmsg_OutRedirects
                    || snmp_root.icmpmsg_InEchos
                    || snmp_root.icmpmsg_OutEchos
                    || snmp_root.icmpmsg_InRouterAdvert
                    || snmp_root.icmpmsg_OutRouterAdvert
                    || snmp_root.icmpmsg_InRouterSelect
                    || snmp_root.icmpmsg_OutRouterSelect
                    || snmp_root.icmpmsg_InTimeExcds
                    || snmp_root.icmpmsg_OutTimeExcds
                    || snmp_root.icmpmsg_InParmProbs
                    || snmp_root.icmpmsg_OutParmProbs
                    || snmp_root.icmpmsg_InTimestamps
                    || snmp_root.icmpmsg_OutTimestamps
                    || snmp_root.icmpmsg_InTimestampReps
                    || snmp_root.icmpmsg_OutTimestampReps
                    ))) {
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
                            RRD_TYPE_NET_SNMP
                            , "icmpmsg"
                            , NULL
                            , "icmp"
                            , NULL
                            , "IPv4 ICMP Messages"
                            , "packets/s"
                            , "proc"
                            , "net/snmp"
                            , NETDATA_CHART_PRIO_IPV4_ICMP + 2
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
                else rrdset_next(st);

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
        }
        else if(unlikely(hash == hash_tcp && strcmp(key, "Tcp") == 0)) {
            size_t h = l++;

            if(strcmp(procfile_lineword(ff, l, 0), "Tcp") != 0) {
                error("Cannot read Tcp line from /proc/net/snmp.");
                break;
            }

            words = procfile_linewords(ff, l);
            if(words < 3) {
                error("Cannot read /proc/net/snmp Tcp line. Expected 3+ params, read %zu.", words);
                continue;
            }

            arl_begin(arl_tcp);
            for(w = 1; w < words ; w++) {
                if (unlikely(arl_check(arl_tcp, procfile_lineword(ff, h, w), procfile_lineword(ff, l, w)) != 0))
                    break;
            }

            // --------------------------------------------------------------------

            if(snmp_root.tcp_MaxConn != last_max_connections) {
                last_max_connections = snmp_root.tcp_MaxConn;
                rrdvar_custom_host_variable_set(localhost, tcp_max_connections_var, last_max_connections);
            }

            // --------------------------------------------------------------------

            // see http://net-snmp.sourceforge.net/docs/mibs/tcp.html
            if(do_tcp_sockets == CONFIG_BOOLEAN_YES || (do_tcp_sockets == CONFIG_BOOLEAN_AUTO && snmp_root.tcp_CurrEstab)) {
                do_tcp_sockets = CONFIG_BOOLEAN_YES;

                static RRDSET *st = NULL;
                static RRDDIM *rd_CurrEstab = NULL;

                if(unlikely(!st)) {
                    st = rrdset_create_localhost(
                            RRD_TYPE_NET_SNMP
                            , "tcpsock"
                            , NULL
                            , "tcp"
                            , NULL
                            , "IPv4 TCP Connections"
                            , "active connections"
                            , "proc"
                            , "net/snmp"
                            , NETDATA_CHART_PRIO_IPV4_TCP
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rd_CurrEstab = rrddim_add(st, "CurrEstab", "connections", 1, 1, RRD_ALGORITHM_ABSOLUTE);
                }
                else rrdset_next(st);

                rrddim_set_by_pointer(st, rd_CurrEstab, (collected_number)snmp_root.tcp_CurrEstab);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_tcp_packets == CONFIG_BOOLEAN_YES || (do_tcp_packets == CONFIG_BOOLEAN_AUTO && (snmp_root.tcp_InSegs || snmp_root.tcp_OutSegs))) {
                do_tcp_packets = CONFIG_BOOLEAN_YES;

                static RRDSET *st = NULL;
                static RRDDIM *rd_InSegs = NULL,
                              *rd_OutSegs = NULL;

                if(unlikely(!st)) {
                    st = rrdset_create_localhost(
                            RRD_TYPE_NET_SNMP
                            , "tcppackets"
                            , NULL
                            , "tcp"
                            , NULL
                            , "IPv4 TCP Packets"
                            , "packets/s"
                            , "proc"
                            , "net/snmp"
                            , NETDATA_CHART_PRIO_IPV4_TCP + 10
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rd_InSegs  = rrddim_add(st, "InSegs",  "received", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_OutSegs = rrddim_add(st, "OutSegs", "sent",    -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set_by_pointer(st, rd_InSegs,  (collected_number)snmp_root.tcp_InSegs);
                rrddim_set_by_pointer(st, rd_OutSegs, (collected_number)snmp_root.tcp_OutSegs);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_tcp_errors == CONFIG_BOOLEAN_YES || (do_tcp_errors == CONFIG_BOOLEAN_AUTO && (snmp_root.tcp_InErrs || snmp_root.tcp_InCsumErrors || snmp_root.tcp_RetransSegs))) {
                do_tcp_errors = CONFIG_BOOLEAN_YES;

                static RRDSET *st = NULL;
                static RRDDIM *rd_InErrs = NULL,
                              *rd_InCsumErrors = NULL,
                              *rd_RetransSegs = NULL;

                if(unlikely(!st)) {
                    st = rrdset_create_localhost(
                            RRD_TYPE_NET_SNMP
                            , "tcperrors"
                            , NULL
                            , "tcp"
                            , NULL
                            , "IPv4 TCP Errors"
                            , "packets/s"
                            , "proc"
                            , "net/snmp"
                            , NETDATA_CHART_PRIO_IPV4_TCP + 20
                            , update_every
                            , RRDSET_TYPE_LINE
                    );
                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rd_InErrs       = rrddim_add(st, "InErrs",       NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_InCsumErrors = rrddim_add(st, "InCsumErrors", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_RetransSegs  = rrddim_add(st, "RetransSegs",  NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set_by_pointer(st, rd_InErrs,       (collected_number)snmp_root.tcp_InErrs);
                rrddim_set_by_pointer(st, rd_InCsumErrors, (collected_number)snmp_root.tcp_InCsumErrors);
                rrddim_set_by_pointer(st, rd_RetransSegs,  (collected_number)snmp_root.tcp_RetransSegs);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_tcp_opens == CONFIG_BOOLEAN_YES || (do_tcp_opens == CONFIG_BOOLEAN_AUTO && (snmp_root.tcp_ActiveOpens || snmp_root.tcp_PassiveOpens))) {
                do_tcp_opens = CONFIG_BOOLEAN_YES;

                static RRDSET *st = NULL;
                static RRDDIM *rd_ActiveOpens = NULL,
                              *rd_PassiveOpens = NULL;

                if(unlikely(!st)) {
                    st = rrdset_create_localhost(
                            RRD_TYPE_NET_SNMP
                            , "tcpopens"
                            , NULL
                            , "tcp"
                            , NULL
                            , "IPv4 TCP Opens"
                            , "connections/s"
                            , "proc"
                            , "net/snmp"
                            , NETDATA_CHART_PRIO_IPV4_TCP + 5
                            , update_every
                            , RRDSET_TYPE_LINE
                    );
                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rd_ActiveOpens   = rrddim_add(st, "ActiveOpens",   "active", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_PassiveOpens  = rrddim_add(st, "PassiveOpens",  "passive", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set_by_pointer(st, rd_ActiveOpens,   (collected_number)snmp_root.tcp_ActiveOpens);
                rrddim_set_by_pointer(st, rd_PassiveOpens,  (collected_number)snmp_root.tcp_PassiveOpens);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_tcp_handshake == CONFIG_BOOLEAN_YES || (do_tcp_handshake == CONFIG_BOOLEAN_AUTO && (snmp_root.tcp_EstabResets || snmp_root.tcp_OutRsts || snmp_root.tcp_AttemptFails))) {
                do_tcp_handshake = CONFIG_BOOLEAN_YES;

                static RRDSET *st = NULL;
                static RRDDIM *rd_EstabResets = NULL,
                              *rd_OutRsts = NULL,
                              *rd_AttemptFails = NULL,
                              *rd_TCPSynRetrans = NULL;

                if(unlikely(!st)) {
                    st = rrdset_create_localhost(
                            RRD_TYPE_NET_SNMP
                            , "tcphandshake"
                            , NULL
                            , "tcp"
                            , NULL
                            , "IPv4 TCP Handshake Issues"
                            , "events/s"
                            , "proc"
                            , "net/snmp"
                            , NETDATA_CHART_PRIO_IPV4_TCP + 30
                            , update_every
                            , RRDSET_TYPE_LINE
                    );
                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rd_EstabResets   = rrddim_add(st, "EstabResets",   NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_OutRsts       = rrddim_add(st, "OutRsts",       NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_AttemptFails  = rrddim_add(st, "AttemptFails",  NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_TCPSynRetrans = rrddim_add(st, "TCPSynRetrans", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set_by_pointer(st, rd_EstabResets,   (collected_number)snmp_root.tcp_EstabResets);
                rrddim_set_by_pointer(st, rd_OutRsts,       (collected_number)snmp_root.tcp_OutRsts);
                rrddim_set_by_pointer(st, rd_AttemptFails,  (collected_number)snmp_root.tcp_AttemptFails);
                rrddim_set_by_pointer(st, rd_TCPSynRetrans, tcpext_TCPSynRetrans);
                rrdset_done(st);
            }
        }
        else if(unlikely(hash == hash_udp && strcmp(key, "Udp") == 0)) {
            size_t h = l++;

            if(strcmp(procfile_lineword(ff, l, 0), "Udp") != 0) {
                error("Cannot read Udp line from /proc/net/snmp.");
                break;
            }

            words = procfile_linewords(ff, l);
            if(words < 3) {
                error("Cannot read /proc/net/snmp Udp line. Expected 3+ params, read %zu.", words);
                continue;
            }

            arl_begin(arl_udp);
            for(w = 1; w < words ; w++) {
                if (unlikely(arl_check(arl_udp, procfile_lineword(ff, h, w), procfile_lineword(ff, l, w)) != 0))
                    break;
            }

            // --------------------------------------------------------------------

            // see http://net-snmp.sourceforge.net/docs/mibs/udp.html
            if(do_udp_packets == CONFIG_BOOLEAN_YES || (do_udp_packets == CONFIG_BOOLEAN_AUTO && (snmp_root.udp_InDatagrams || snmp_root.udp_OutDatagrams))) {
                do_udp_packets = CONFIG_BOOLEAN_YES;

                static RRDSET *st = NULL;
                static RRDDIM *rd_InDatagrams = NULL,
                              *rd_OutDatagrams = NULL;

                if(unlikely(!st)) {
                    st = rrdset_create_localhost(
                            RRD_TYPE_NET_SNMP
                            , "udppackets"
                            , NULL
                            , "udp"
                            , NULL
                            , "IPv4 UDP Packets"
                            , "packets/s"
                            , "proc"
                            , "net/snmp"
                            , NETDATA_CHART_PRIO_IPV4_UDP
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rd_InDatagrams  = rrddim_add(st, "InDatagrams",  "received", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_OutDatagrams = rrddim_add(st, "OutDatagrams", "sent",    -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set_by_pointer(st, rd_InDatagrams,  (collected_number)snmp_root.udp_InDatagrams);
                rrddim_set_by_pointer(st, rd_OutDatagrams, (collected_number)snmp_root.udp_OutDatagrams);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_udp_errors == CONFIG_BOOLEAN_YES || (do_udp_errors == CONFIG_BOOLEAN_AUTO && (
                    snmp_root.udp_InErrors
                    || snmp_root.udp_NoPorts
                    || snmp_root.udp_RcvbufErrors
                    || snmp_root.udp_SndbufErrors
                    || snmp_root.udp_InCsumErrors
                    || snmp_root.udp_IgnoredMulti
                    ))) {
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
                            RRD_TYPE_NET_SNMP
                            , "udperrors"
                            , NULL
                            , "udp"
                            , NULL
                            , "IPv4 UDP Errors"
                            , "events/s"
                            , "proc"
                            , "net/snmp"
                            , NETDATA_CHART_PRIO_IPV4_UDP + 10
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

                rrddim_set_by_pointer(st, rd_InErrors,     (collected_number)snmp_root.udp_InErrors);
                rrddim_set_by_pointer(st, rd_NoPorts,      (collected_number)snmp_root.udp_NoPorts);
                rrddim_set_by_pointer(st, rd_RcvbufErrors, (collected_number)snmp_root.udp_RcvbufErrors);
                rrddim_set_by_pointer(st, rd_SndbufErrors, (collected_number)snmp_root.udp_SndbufErrors);
                rrddim_set_by_pointer(st, rd_InCsumErrors, (collected_number)snmp_root.udp_InCsumErrors);
                rrddim_set_by_pointer(st, rd_IgnoredMulti, (collected_number)snmp_root.udp_IgnoredMulti);
                rrdset_done(st);
            }
        }
        else if(unlikely(hash == hash_udplite && strcmp(key, "UdpLite") == 0)) {
            size_t h = l++;

            if(strcmp(procfile_lineword(ff, l, 0), "UdpLite") != 0) {
                error("Cannot read UdpLite line from /proc/net/snmp.");
                break;
            }

            words = procfile_linewords(ff, l);
            if(words < 3) {
                error("Cannot read /proc/net/snmp UdpLite line. Expected 3+ params, read %zu.", words);
                continue;
            }

            arl_begin(arl_udplite);
            for(w = 1; w < words ; w++) {
                if (unlikely(arl_check(arl_udplite, procfile_lineword(ff, h, w), procfile_lineword(ff, l, w)) != 0))
                    break;
            }

            // --------------------------------------------------------------------

            if(do_udplite_packets == CONFIG_BOOLEAN_YES || (do_udplite_packets == CONFIG_BOOLEAN_AUTO && (
                    snmp_root.udplite_InDatagrams
                    || snmp_root.udplite_OutDatagrams
                    || snmp_root.udplite_NoPorts
                    || snmp_root.udplite_InErrors
                    || snmp_root.udplite_InCsumErrors
                    || snmp_root.udplite_RcvbufErrors
                    || snmp_root.udplite_SndbufErrors
                    || snmp_root.udplite_IgnoredMulti
                    ))) {
                do_udplite_packets = CONFIG_BOOLEAN_YES;

                {
                    static RRDSET *st = NULL;
                    static RRDDIM *rd_InDatagrams = NULL,
                                  *rd_OutDatagrams = NULL;

                    if(unlikely(!st)) {
                        st = rrdset_create_localhost(
                                RRD_TYPE_NET_SNMP
                                , "udplite"
                                , NULL
                                , "udplite"
                                , NULL
                                , "IPv4 UDPLite Packets"
                                , "packets/s"
                                , "proc"
                                , "net/snmp"
                                , NETDATA_CHART_PRIO_IPV4_UDPLITE
                                , update_every
                                , RRDSET_TYPE_LINE
                        );

                        rd_InDatagrams  = rrddim_add(st, "InDatagrams",  "received", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                        rd_OutDatagrams = rrddim_add(st, "OutDatagrams", "sent",    -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    }
                    else rrdset_next(st);

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
                                RRD_TYPE_NET_SNMP
                                , "udplite_errors"
                                , NULL
                                , "udplite"
                                , NULL
                                , "IPv4 UDPLite Errors"
                                , "packets/s"
                                , "proc"
                                , "net/snmp"
                                , NETDATA_CHART_PRIO_IPV4_UDPLITE + 10
                                , update_every
                                , RRDSET_TYPE_LINE);

                        rd_RcvbufErrors = rrddim_add(st, "RcvbufErrors", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                        rd_SndbufErrors = rrddim_add(st, "SndbufErrors", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                        rd_InErrors     = rrddim_add(st, "InErrors",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                        rd_NoPorts      = rrddim_add(st, "NoPorts",      NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                        rd_InCsumErrors = rrddim_add(st, "InCsumErrors", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                        rd_IgnoredMulti = rrddim_add(st, "IgnoredMulti", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    }
                    else rrdset_next(st);

                    rrddim_set_by_pointer(st, rd_NoPorts,      (collected_number)snmp_root.udplite_NoPorts);
                    rrddim_set_by_pointer(st, rd_InErrors,     (collected_number)snmp_root.udplite_InErrors);
                    rrddim_set_by_pointer(st, rd_InCsumErrors, (collected_number)snmp_root.udplite_InCsumErrors);
                    rrddim_set_by_pointer(st, rd_RcvbufErrors, (collected_number)snmp_root.udplite_RcvbufErrors);
                    rrddim_set_by_pointer(st, rd_SndbufErrors, (collected_number)snmp_root.udplite_SndbufErrors);
                    rrddim_set_by_pointer(st, rd_IgnoredMulti, (collected_number)snmp_root.udplite_IgnoredMulti);
                    rrdset_done(st);
                }
            }
        }
    }

    return 0;
}


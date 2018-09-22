// SPDX-License-Identifier: GPL-3.0+

#include "common.h"

unsigned long long tcpext_TCPSynRetrans = 0;
unsigned long long tcpext_TCPReqQFullDrop = 0;

static void parse_line_pair(procfile *ff, ARL_BASE *base, size_t header_line, size_t values_line) {
    size_t hwords = procfile_linewords(ff, header_line);
    size_t vwords = procfile_linewords(ff, values_line);
    size_t w;

    if(unlikely(vwords > hwords)) {
        error("File /proc/net/netstat on header line %zu has %zu words, but on value line %zu has %zu words.", header_line, hwords, values_line, vwords);
        vwords = hwords;
    }

    for(w = 1; w < vwords ;w++) {
        if(unlikely(arl_check(base, procfile_lineword(ff, header_line, w), procfile_lineword(ff, values_line, w))))
            break;
    }
}

int do_proc_net_netstat(int update_every, usec_t dt) {
    (void)dt;

    static int do_bandwidth = -1, do_inerrors = -1, do_mcast = -1, do_bcast = -1, do_mcast_p = -1, do_bcast_p = -1, do_ecn = -1, \
        do_tcpext_reorder = -1, do_tcpext_syscookies = -1, do_tcpext_ofo = -1, do_tcpext_connaborts = -1, do_tcpext_memory = -1,
        do_tcpext_listen = -1;

    static uint32_t hash_ipext = 0, hash_tcpext = 0;
    static procfile *ff = NULL;

    static ARL_BASE *arl_tcpext = NULL;
    static ARL_BASE *arl_ipext = NULL;

    // --------------------------------------------------------------------
    // IPv4

    // IPv4 bandwidth
    static unsigned long long ipext_InOctets = 0;
    static unsigned long long ipext_OutOctets = 0;

    // IPv4 input errors
    static unsigned long long ipext_InNoRoutes = 0;
    static unsigned long long ipext_InTruncatedPkts = 0;
    static unsigned long long ipext_InCsumErrors = 0;

    // IPv4 multicast bandwidth
    static unsigned long long ipext_InMcastOctets = 0;
    static unsigned long long ipext_OutMcastOctets = 0;

    // IPv4 multicast packets
    static unsigned long long ipext_InMcastPkts = 0;
    static unsigned long long ipext_OutMcastPkts = 0;

    // IPv4 broadcast bandwidth
    static unsigned long long ipext_InBcastOctets = 0;
    static unsigned long long ipext_OutBcastOctets = 0;

    // IPv4 broadcast packets
    static unsigned long long ipext_InBcastPkts = 0;
    static unsigned long long ipext_OutBcastPkts = 0;

    // IPv4 ECN
    static unsigned long long ipext_InNoECTPkts = 0;
    static unsigned long long ipext_InECT1Pkts = 0;
    static unsigned long long ipext_InECT0Pkts = 0;
    static unsigned long long ipext_InCEPkts = 0;

    // --------------------------------------------------------------------
    // IPv4 TCP

    // IPv4 TCP Reordering
    static unsigned long long tcpext_TCPRenoReorder = 0;
    static unsigned long long tcpext_TCPFACKReorder = 0;
    static unsigned long long tcpext_TCPSACKReorder = 0;
    static unsigned long long tcpext_TCPTSReorder = 0;

    // IPv4 TCP SYN Cookies
    static unsigned long long tcpext_SyncookiesSent = 0;
    static unsigned long long tcpext_SyncookiesRecv = 0;
    static unsigned long long tcpext_SyncookiesFailed = 0;

    // IPv4 TCP Out Of Order Queue
    // http://www.spinics.net/lists/netdev/msg204696.html
    static unsigned long long tcpext_TCPOFOQueue = 0; // Number of packets queued in OFO queue
    static unsigned long long tcpext_TCPOFODrop = 0;  // Number of packets meant to be queued in OFO but dropped because socket rcvbuf limit hit.
    static unsigned long long tcpext_TCPOFOMerge = 0; // Number of packets in OFO that were merged with other packets.
    static unsigned long long tcpext_OfoPruned = 0;   // packets dropped from out-of-order queue because of socket buffer overrun

    // IPv4 TCP connection resets
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

    // IPv4 TCP memory pressures
    static unsigned long long tcpext_TCPMemoryPressures = 0;

    // shared tcpext_TCPReqQFullDrop
    // shared: tcpext_TCPSynRetrans


    if(unlikely(!arl_ipext)) {
        hash_ipext = simple_hash("IpExt");
        hash_tcpext = simple_hash("TcpExt");

        do_bandwidth = config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "bandwidth", CONFIG_BOOLEAN_AUTO);
        do_inerrors  = config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "input errors", CONFIG_BOOLEAN_AUTO);
        do_mcast     = config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "multicast bandwidth", CONFIG_BOOLEAN_AUTO);
        do_bcast     = config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "broadcast bandwidth", CONFIG_BOOLEAN_AUTO);
        do_mcast_p   = config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "multicast packets", CONFIG_BOOLEAN_AUTO);
        do_bcast_p   = config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "broadcast packets", CONFIG_BOOLEAN_AUTO);
        do_ecn       = config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "ECN packets", CONFIG_BOOLEAN_AUTO);

        do_tcpext_reorder    = config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "TCP reorders", CONFIG_BOOLEAN_AUTO);
        do_tcpext_syscookies = config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "TCP SYN cookies", CONFIG_BOOLEAN_AUTO);
        do_tcpext_ofo        = config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "TCP out-of-order queue", CONFIG_BOOLEAN_AUTO);
        do_tcpext_connaborts = config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "TCP connection aborts", CONFIG_BOOLEAN_AUTO);
        do_tcpext_memory     = config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "TCP memory pressures", CONFIG_BOOLEAN_AUTO);
        do_tcpext_listen     = config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "TCP listen issues", CONFIG_BOOLEAN_AUTO);

        arl_ipext  = arl_create("netstat/ipext", NULL, 60);
        arl_tcpext = arl_create("netstat/tcpext", NULL, 60);

        // --------------------------------------------------------------------
        // IPv4

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
        // IPv4 TCP

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

        if(do_tcpext_listen != CONFIG_BOOLEAN_NO) {
            arl_expect(arl_tcpext, "ListenOverflows", &tcpext_ListenOverflows);
            arl_expect(arl_tcpext, "ListenDrops",     &tcpext_ListenDrops);
        }

        // shared metrics
        arl_expect(arl_tcpext, "TCPSynRetrans", &tcpext_TCPSynRetrans);
        arl_expect(arl_tcpext, "TCPReqQFullDrop", &tcpext_TCPReqQFullDrop);
    }

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/net/netstat");
        ff = procfile_open(config_get("plugin:proc:/proc/net/netstat", "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 0; // we return 0, so that we will retry to open it next time

    size_t lines = procfile_lines(ff), l;
    size_t words;

    arl_begin(arl_ipext);
    arl_begin(arl_tcpext);

    for(l = 0; l < lines ;l++) {
        char *key = procfile_lineword(ff, l, 0);
        uint32_t hash = simple_hash(key);

        if(unlikely(hash == hash_ipext && strcmp(key, "IpExt") == 0)) {
            size_t h = l++;

            words = procfile_linewords(ff, l);
            if(unlikely(words < 2)) {
                error("Cannot read /proc/net/netstat IpExt line. Expected 2+ params, read %zu.", words);
                continue;
            }

            parse_line_pair(ff, arl_ipext, h, l);

            // --------------------------------------------------------------------

            if(do_bandwidth == CONFIG_BOOLEAN_YES || (do_bandwidth == CONFIG_BOOLEAN_AUTO && (ipext_InOctets || ipext_OutOctets))) {
                do_bandwidth = CONFIG_BOOLEAN_YES;
                static RRDSET *st_system_ipv4 = NULL;
                static RRDDIM *rd_in = NULL, *rd_out = NULL;

                if(unlikely(!st_system_ipv4)) {
                    st_system_ipv4 = rrdset_create_localhost(
                            "system"
                            , "ipv4"
                            , NULL
                            , "network"
                            , NULL
                            , "IPv4 Bandwidth"
                            , "kilobits/s"
                            , "proc"
                            , "net/netstat"
                            , NETDATA_CHART_PRIO_SYSTEM_IPV4
                            , update_every
                            , RRDSET_TYPE_AREA
                    );

                    rd_in  = rrddim_add(st_system_ipv4, "InOctets",  "received", 8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                    rd_out = rrddim_add(st_system_ipv4, "OutOctets", "sent",    -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                }
                else
                    rrdset_next(st_system_ipv4);

                rrddim_set_by_pointer(st_system_ipv4, rd_in,  ipext_InOctets);
                rrddim_set_by_pointer(st_system_ipv4, rd_out, ipext_OutOctets);

                rrdset_done(st_system_ipv4);
            }

            // --------------------------------------------------------------------

            if(do_inerrors == CONFIG_BOOLEAN_YES || (do_inerrors == CONFIG_BOOLEAN_AUTO && (ipext_InNoRoutes || ipext_InTruncatedPkts))) {
                do_inerrors = CONFIG_BOOLEAN_YES;
                static RRDSET *st_ipv4_inerrors = NULL;
                static RRDDIM *rd_noroutes = NULL, *rd_truncated = NULL, *rd_checksum = NULL;

                if(unlikely(!st_ipv4_inerrors)) {
                    st_ipv4_inerrors = rrdset_create_localhost(
                            "ipv4"
                            , "inerrors"
                            , NULL
                            , "errors"
                            , NULL
                            , "IPv4 Input Errors"
                            , "packets/s"
                            , "proc"
                            , "net/netstat"
                            , NETDATA_CHART_PRIO_IPV4_ERRORS
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrdset_flag_set(st_ipv4_inerrors, RRDSET_FLAG_DETAIL);

                    rd_noroutes  = rrddim_add(st_ipv4_inerrors, "InNoRoutes",      "noroutes",  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_truncated = rrddim_add(st_ipv4_inerrors, "InTruncatedPkts", "truncated", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_checksum  = rrddim_add(st_ipv4_inerrors, "InCsumErrors",    "checksum",  1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else
                    rrdset_next(st_ipv4_inerrors);

                rrddim_set_by_pointer(st_ipv4_inerrors, rd_noroutes,  ipext_InNoRoutes);
                rrddim_set_by_pointer(st_ipv4_inerrors, rd_truncated, ipext_InTruncatedPkts);
                rrddim_set_by_pointer(st_ipv4_inerrors, rd_checksum,  ipext_InCsumErrors);

                rrdset_done(st_ipv4_inerrors);
            }

            // --------------------------------------------------------------------

            if(do_mcast == CONFIG_BOOLEAN_YES || (do_mcast == CONFIG_BOOLEAN_AUTO && (ipext_InMcastOctets || ipext_OutMcastOctets))) {
                do_mcast = CONFIG_BOOLEAN_YES;
                static RRDSET *st_ipv4_mcast = NULL;
                static RRDDIM *rd_in = NULL, *rd_out = NULL;

                if(unlikely(!st_ipv4_mcast)) {
                    st_ipv4_mcast = rrdset_create_localhost(
                            "ipv4"
                            , "mcast"
                            , NULL
                            , "multicast"
                            , NULL
                            , "IPv4 Multicast Bandwidth"
                            , "kilobits/s"
                            , "proc"
                            , "net/netstat"
                            , NETDATA_CHART_PRIO_IPV4_MCAST
                            , update_every
                            , RRDSET_TYPE_AREA
                    );

                    rrdset_flag_set(st_ipv4_mcast, RRDSET_FLAG_DETAIL);

                    rd_in  = rrddim_add(st_ipv4_mcast, "InMcastOctets",  "received", 8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                    rd_out = rrddim_add(st_ipv4_mcast, "OutMcastOctets", "sent",    -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                }
                else
                    rrdset_next(st_ipv4_mcast);

                rrddim_set_by_pointer(st_ipv4_mcast, rd_in,  ipext_InMcastOctets);
                rrddim_set_by_pointer(st_ipv4_mcast, rd_out, ipext_OutMcastOctets);

                rrdset_done(st_ipv4_mcast);
            }

            // --------------------------------------------------------------------

            if(do_bcast == CONFIG_BOOLEAN_YES || (do_bcast == CONFIG_BOOLEAN_AUTO && (ipext_InBcastOctets || ipext_OutBcastOctets))) {
                do_bcast = CONFIG_BOOLEAN_YES;

                static RRDSET *st_ipv4_bcast = NULL;
                static RRDDIM *rd_in = NULL, *rd_out = NULL;

                if(unlikely(!st_ipv4_bcast)) {
                    st_ipv4_bcast = rrdset_create_localhost(
                            "ipv4"
                            , "bcast"
                            , NULL
                            , "broadcast"
                            , NULL
                            , "IPv4 Broadcast Bandwidth"
                            , "kilobits/s"
                            , "proc"
                            , "net/netstat"
                            , NETDATA_CHART_PRIO_IPV4_BCAST
                            , update_every
                            , RRDSET_TYPE_AREA
                    );

                    rrdset_flag_set(st_ipv4_bcast, RRDSET_FLAG_DETAIL);

                    rd_in  = rrddim_add(st_ipv4_bcast, "InBcastOctets",  "received", 8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                    rd_out = rrddim_add(st_ipv4_bcast, "OutBcastOctets", "sent",    -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                }
                else
                    rrdset_next(st_ipv4_bcast);

                rrddim_set_by_pointer(st_ipv4_bcast, rd_in,  ipext_InBcastOctets);
                rrddim_set_by_pointer(st_ipv4_bcast, rd_out, ipext_OutBcastOctets);

                rrdset_done(st_ipv4_bcast);
            }

            // --------------------------------------------------------------------

            if(do_mcast_p == CONFIG_BOOLEAN_YES || (do_mcast_p == CONFIG_BOOLEAN_AUTO && (ipext_InMcastPkts || ipext_OutMcastPkts))) {
                do_mcast_p = CONFIG_BOOLEAN_YES;

                static RRDSET *st_ipv4_mcastpkts = NULL;
                static RRDDIM *rd_in = NULL, *rd_out = NULL;

                if(unlikely(!st_ipv4_mcastpkts)) {
                    st_ipv4_mcastpkts = rrdset_create_localhost(
                            "ipv4"
                            , "mcastpkts"
                            , NULL
                            , "multicast"
                            , NULL
                            , "IPv4 Multicast Packets"
                            , "packets/s"
                            , "proc"
                            , "net/netstat"
                            , NETDATA_CHART_PRIO_IPV4_MCAST + 10
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrdset_flag_set(st_ipv4_mcastpkts, RRDSET_FLAG_DETAIL);

                    rd_in  = rrddim_add(st_ipv4_mcastpkts, "InMcastPkts",  "received", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out = rrddim_add(st_ipv4_mcastpkts, "OutMcastPkts", "sent",    -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st_ipv4_mcastpkts);

                rrddim_set_by_pointer(st_ipv4_mcastpkts, rd_in,  ipext_InMcastPkts);
                rrddim_set_by_pointer(st_ipv4_mcastpkts, rd_out, ipext_OutMcastPkts);

                rrdset_done(st_ipv4_mcastpkts);
            }

            // --------------------------------------------------------------------

            if(do_bcast_p == CONFIG_BOOLEAN_YES || (do_bcast_p == CONFIG_BOOLEAN_AUTO && (ipext_InBcastPkts || ipext_OutBcastPkts))) {
                do_bcast_p = CONFIG_BOOLEAN_YES;

                static RRDSET *st_ipv4_bcastpkts = NULL;
                static RRDDIM *rd_in = NULL, *rd_out = NULL;

                if(unlikely(!st_ipv4_bcastpkts)) {
                    st_ipv4_bcastpkts = rrdset_create_localhost(
                            "ipv4"
                            , "bcastpkts"
                            , NULL
                            , "broadcast"
                            , NULL
                            , "IPv4 Broadcast Packets"
                            , "packets/s"
                            , "proc"
                            , "net/netstat"
                            , NETDATA_CHART_PRIO_IPV4_BCAST + 10
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrdset_flag_set(st_ipv4_bcastpkts, RRDSET_FLAG_DETAIL);

                    rd_in  = rrddim_add(st_ipv4_bcastpkts, "InBcastPkts",  "received", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_out = rrddim_add(st_ipv4_bcastpkts, "OutBcastPkts", "sent",    -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else
                    rrdset_next(st_ipv4_bcastpkts);

                rrddim_set_by_pointer(st_ipv4_bcastpkts, rd_in,  ipext_InBcastPkts);
                rrddim_set_by_pointer(st_ipv4_bcastpkts, rd_out, ipext_OutBcastPkts);

                rrdset_done(st_ipv4_bcastpkts);
            }

            // --------------------------------------------------------------------

            if(do_ecn == CONFIG_BOOLEAN_YES || (do_ecn == CONFIG_BOOLEAN_AUTO && (ipext_InCEPkts || ipext_InECT0Pkts || ipext_InECT1Pkts || ipext_InNoECTPkts))) {
                do_ecn = CONFIG_BOOLEAN_YES;

                static RRDSET *st_ecnpkts = NULL;
                static RRDDIM *rd_cep = NULL, *rd_noectp = NULL, *rd_ectp0 = NULL, *rd_ectp1 = NULL;

                if(unlikely(!st_ecnpkts)) {
                    st_ecnpkts = rrdset_create_localhost(
                            "ipv4"
                            , "ecnpkts"
                            , NULL
                            , "ecn"
                            , NULL
                            , "IPv4 ECN Statistics"
                            , "packets/s"
                            , "proc"
                            , "net/netstat"
                            , NETDATA_CHART_PRIO_IPV4_ECN
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrdset_flag_set(st_ecnpkts, RRDSET_FLAG_DETAIL);

                    rd_cep    = rrddim_add(st_ecnpkts, "InCEPkts",    "CEP",     1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_noectp = rrddim_add(st_ecnpkts, "InNoECTPkts", "NoECTP", -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_ectp0  = rrddim_add(st_ecnpkts, "InECT0Pkts",  "ECTP0",   1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_ectp1  = rrddim_add(st_ecnpkts, "InECT1Pkts",  "ECTP1",   1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st_ecnpkts);

                rrddim_set_by_pointer(st_ecnpkts, rd_cep,    ipext_InCEPkts);
                rrddim_set_by_pointer(st_ecnpkts, rd_noectp, ipext_InNoECTPkts);
                rrddim_set_by_pointer(st_ecnpkts, rd_ectp0,  ipext_InECT0Pkts);
                rrddim_set_by_pointer(st_ecnpkts, rd_ectp1,  ipext_InECT1Pkts);

                rrdset_done(st_ecnpkts);
            }
        }
        else if(unlikely(hash == hash_tcpext && strcmp(key, "TcpExt") == 0)) {
            size_t h = l++;

            words = procfile_linewords(ff, l);
            if(unlikely(words < 2)) {
                error("Cannot read /proc/net/netstat TcpExt line. Expected 2+ params, read %zu.", words);
                continue;
            }

            parse_line_pair(ff, arl_tcpext, h, l);

            // --------------------------------------------------------------------

            if(do_tcpext_memory == CONFIG_BOOLEAN_YES || (do_tcpext_memory == CONFIG_BOOLEAN_AUTO && (tcpext_TCPMemoryPressures))) {
                do_tcpext_memory = CONFIG_BOOLEAN_YES;

                static RRDSET *st_tcpmemorypressures = NULL;
                static RRDDIM *rd_pressures = NULL;

                if(unlikely(!st_tcpmemorypressures)) {
                    st_tcpmemorypressures = rrdset_create_localhost(
                            "ipv4"
                            , "tcpmemorypressures"
                            , NULL
                            , "tcp"
                            , NULL
                            , "TCP Memory Pressures"
                            , "events/s"
                            , "proc"
                            , "net/netstat"
                            , NETDATA_CHART_PRIO_IPV4_TCP_MEM
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rd_pressures = rrddim_add(st_tcpmemorypressures, "TCPMemoryPressures",   "pressures",  1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else
                    rrdset_next(st_tcpmemorypressures);

                rrddim_set_by_pointer(st_tcpmemorypressures, rd_pressures, tcpext_TCPMemoryPressures);

                rrdset_done(st_tcpmemorypressures);
            }

            // --------------------------------------------------------------------

            if(do_tcpext_connaborts == CONFIG_BOOLEAN_YES || (do_tcpext_connaborts == CONFIG_BOOLEAN_AUTO && (tcpext_TCPAbortOnData || tcpext_TCPAbortOnClose || tcpext_TCPAbortOnMemory || tcpext_TCPAbortOnTimeout || tcpext_TCPAbortOnLinger || tcpext_TCPAbortFailed))) {
                do_tcpext_connaborts = CONFIG_BOOLEAN_YES;

                static RRDSET *st_tcpconnaborts = NULL;
                static RRDDIM *rd_baddata = NULL, *rd_userclosed = NULL, *rd_nomemory = NULL, *rd_timeout = NULL, *rd_linger = NULL, *rd_failed = NULL;

                if(unlikely(!st_tcpconnaborts)) {
                    st_tcpconnaborts = rrdset_create_localhost(
                            "ipv4"
                            , "tcpconnaborts"
                            , NULL
                            , "tcp"
                            , NULL
                            , "TCP Connection Aborts"
                            , "connections/s"
                            , "proc"
                            , "net/netstat"
                            , NETDATA_CHART_PRIO_IPV4_TCP + 10
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
                else
                    rrdset_next(st_tcpconnaborts);

                rrddim_set_by_pointer(st_tcpconnaborts, rd_baddata,    tcpext_TCPAbortOnData);
                rrddim_set_by_pointer(st_tcpconnaborts, rd_userclosed, tcpext_TCPAbortOnClose);
                rrddim_set_by_pointer(st_tcpconnaborts, rd_nomemory,   tcpext_TCPAbortOnMemory);
                rrddim_set_by_pointer(st_tcpconnaborts, rd_timeout,    tcpext_TCPAbortOnTimeout);
                rrddim_set_by_pointer(st_tcpconnaborts, rd_linger,     tcpext_TCPAbortOnLinger);
                rrddim_set_by_pointer(st_tcpconnaborts, rd_failed,     tcpext_TCPAbortFailed);

                rrdset_done(st_tcpconnaborts);
            }

            // --------------------------------------------------------------------

            if(do_tcpext_reorder == CONFIG_BOOLEAN_YES || (do_tcpext_reorder == CONFIG_BOOLEAN_AUTO && (tcpext_TCPRenoReorder || tcpext_TCPFACKReorder || tcpext_TCPSACKReorder || tcpext_TCPTSReorder))) {
                do_tcpext_reorder = CONFIG_BOOLEAN_YES;

                static RRDSET *st_tcpreorders = NULL;
                static RRDDIM *rd_timestamp = NULL, *rd_sack = NULL, *rd_fack = NULL, *rd_reno = NULL;

                if(unlikely(!st_tcpreorders)) {
                    st_tcpreorders = rrdset_create_localhost(
                            "ipv4"
                            , "tcpreorders"
                            , NULL
                            , "tcp"
                            , NULL
                            , "TCP Reordered Packets by Detection Method"
                            , "packets/s"
                            , "proc"
                            , "net/netstat"
                            , NETDATA_CHART_PRIO_IPV4_TCP + 20
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rd_timestamp = rrddim_add(st_tcpreorders, "TCPTSReorder",   "timestamp",   1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_sack      = rrddim_add(st_tcpreorders, "TCPSACKReorder", "sack",        1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_fack      = rrddim_add(st_tcpreorders, "TCPFACKReorder", "fack",        1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_reno      = rrddim_add(st_tcpreorders, "TCPRenoReorder", "reno",        1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else
                    rrdset_next(st_tcpreorders);

                rrddim_set_by_pointer(st_tcpreorders, rd_timestamp, tcpext_TCPTSReorder);
                rrddim_set_by_pointer(st_tcpreorders, rd_sack,      tcpext_TCPSACKReorder);
                rrddim_set_by_pointer(st_tcpreorders, rd_fack,      tcpext_TCPFACKReorder);
                rrddim_set_by_pointer(st_tcpreorders, rd_reno,      tcpext_TCPRenoReorder);

                rrdset_done(st_tcpreorders);
            }

            // --------------------------------------------------------------------

            if(do_tcpext_ofo == CONFIG_BOOLEAN_YES || (do_tcpext_ofo == CONFIG_BOOLEAN_AUTO && (tcpext_TCPOFOQueue || tcpext_TCPOFODrop || tcpext_TCPOFOMerge))) {
                do_tcpext_ofo = CONFIG_BOOLEAN_YES;

                static RRDSET *st_ipv4_tcpofo = NULL;
                static RRDDIM *rd_inqueue = NULL, *rd_dropped = NULL, *rd_merged = NULL, *rd_pruned = NULL;

                if(unlikely(!st_ipv4_tcpofo)) {

                    st_ipv4_tcpofo = rrdset_create_localhost(
                            "ipv4"
                            , "tcpofo"
                            , NULL
                            , "tcp"
                            , NULL
                            , "TCP Out-Of-Order Queue"
                            , "packets/s"
                            , "proc"
                            , "net/netstat"
                            , NETDATA_CHART_PRIO_IPV4_TCP + 50
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rd_inqueue = rrddim_add(st_ipv4_tcpofo, "TCPOFOQueue", "inqueue",  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_dropped = rrddim_add(st_ipv4_tcpofo, "TCPOFODrop",  "dropped", -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_merged  = rrddim_add(st_ipv4_tcpofo, "TCPOFOMerge", "merged",   1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_pruned  = rrddim_add(st_ipv4_tcpofo, "OfoPruned",   "pruned",  -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else
                    rrdset_next(st_ipv4_tcpofo);

                rrddim_set_by_pointer(st_ipv4_tcpofo, rd_inqueue, tcpext_TCPOFOQueue);
                rrddim_set_by_pointer(st_ipv4_tcpofo, rd_dropped, tcpext_TCPOFODrop);
                rrddim_set_by_pointer(st_ipv4_tcpofo, rd_merged,  tcpext_TCPOFOMerge);
                rrddim_set_by_pointer(st_ipv4_tcpofo, rd_pruned,  tcpext_OfoPruned);

                rrdset_done(st_ipv4_tcpofo);
            }

            // --------------------------------------------------------------------

            if(do_tcpext_syscookies == CONFIG_BOOLEAN_YES || (do_tcpext_syscookies == CONFIG_BOOLEAN_AUTO && (tcpext_SyncookiesSent || tcpext_SyncookiesRecv || tcpext_SyncookiesFailed))) {
                do_tcpext_syscookies = CONFIG_BOOLEAN_YES;

                static RRDSET *st_syncookies = NULL;
                static RRDDIM *rd_received = NULL, *rd_sent = NULL, *rd_failed = NULL;

                if(unlikely(!st_syncookies)) {

                    st_syncookies = rrdset_create_localhost(
                            "ipv4"
                            , "tcpsyncookies"
                            , NULL
                            , "tcp"
                            , NULL
                            , "TCP SYN Cookies"
                            , "packets/s"
                            , "proc"
                            , "net/netstat"
                            , NETDATA_CHART_PRIO_IPV4_TCP + 60
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rd_received = rrddim_add(st_syncookies, "SyncookiesRecv",   "received",  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_sent     = rrddim_add(st_syncookies, "SyncookiesSent",   "sent",     -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_failed   = rrddim_add(st_syncookies, "SyncookiesFailed", "failed",   -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else
                    rrdset_next(st_syncookies);

                rrddim_set_by_pointer(st_syncookies, rd_received, tcpext_SyncookiesRecv);
                rrddim_set_by_pointer(st_syncookies, rd_sent,     tcpext_SyncookiesSent);
                rrddim_set_by_pointer(st_syncookies, rd_failed,   tcpext_SyncookiesFailed);

                rrdset_done(st_syncookies);
            }

            // --------------------------------------------------------------------

            if(do_tcpext_listen == CONFIG_BOOLEAN_YES || (do_tcpext_listen == CONFIG_BOOLEAN_AUTO && (tcpext_ListenOverflows || tcpext_ListenDrops))) {
                do_tcpext_listen = CONFIG_BOOLEAN_YES;

                static RRDSET *st_listen = NULL;
                static RRDDIM *rd_overflows = NULL, *rd_drops = NULL;

                if(unlikely(!st_listen)) {

                    st_listen = rrdset_create_localhost(
                            "ipv4"
                            , "tcplistenissues"
                            , NULL
                            , "tcp"
                            , NULL
                            , "TCP Listen Socket Issues"
                            , "packets/s"
                            , "proc"
                            , "net/netstat"
                            , NETDATA_CHART_PRIO_IPV4_TCP + 15
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rd_overflows = rrddim_add(st_listen, "ListenOverflows", "overflows",  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_drops     = rrddim_add(st_listen, "ListenDrops",     "drops",      1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else
                    rrdset_next(st_listen);

                rrddim_set_by_pointer(st_listen, rd_overflows, tcpext_ListenOverflows);
                rrddim_set_by_pointer(st_listen, rd_drops,     tcpext_ListenDrops);

                rrdset_done(st_listen);
            }
        }
    }

    return 0;
}

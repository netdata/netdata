#include "common.h"

unsigned long long tcpext_TCPSynRetrans;

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
        do_tcpext_reorder = -1, do_tcpext_syscookies = -1, do_tcpext_ofo = -1, do_tcpext_connaborts = -1, do_tcpext_memory = -1;
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

    // IPv4 TCP memory pressures
    static unsigned long long tcpext_TCPMemoryPressures = 0;

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

        // shared metrics
        arl_expect(arl_tcpext, "TCPSynRetrans", &tcpext_TCPSynRetrans);
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

            RRDSET *st;

            // --------------------------------------------------------------------

            if(do_bandwidth == CONFIG_BOOLEAN_YES || (do_bandwidth == CONFIG_BOOLEAN_AUTO && (ipext_InOctets || ipext_OutOctets))) {
                do_bandwidth = CONFIG_BOOLEAN_YES;
                st = rrdset_find_localhost("system.ipv4");
                if(unlikely(!st)) {
                    st = rrdset_create_localhost("system", "ipv4", NULL, "network", NULL, "IPv4 Bandwidth", "kilobits/s"
                                                 , 500, update_every, RRDSET_TYPE_AREA);

                    rrddim_add(st, "InOctets",  "received", 8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "OutOctets", "sent",    -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "InOctets", ipext_InOctets);
                rrddim_set(st, "OutOctets", ipext_OutOctets);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_inerrors == CONFIG_BOOLEAN_YES || (do_inerrors == CONFIG_BOOLEAN_AUTO && (ipext_InNoRoutes || ipext_InTruncatedPkts))) {
                do_inerrors = CONFIG_BOOLEAN_YES;
                st = rrdset_find_localhost("ipv4.inerrors");
                if(unlikely(!st)) {
                    st = rrdset_create_localhost("ipv4", "inerrors", NULL, "errors", NULL, "IPv4 Input Errors"
                                                 , "packets/s", 4000, update_every, RRDSET_TYPE_LINE);
                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rrddim_add(st, "InNoRoutes", "noroutes", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "InTruncatedPkts", "truncated", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "InCsumErrors", "checksum", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "InNoRoutes", ipext_InNoRoutes);
                rrddim_set(st, "InTruncatedPkts", ipext_InTruncatedPkts);
                rrddim_set(st, "InCsumErrors", ipext_InCsumErrors);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_mcast == CONFIG_BOOLEAN_YES || (do_mcast == CONFIG_BOOLEAN_AUTO && (ipext_InMcastOctets || ipext_OutMcastOctets))) {
                do_mcast = CONFIG_BOOLEAN_YES;
                st = rrdset_find_localhost("ipv4.mcast");
                if(unlikely(!st)) {
                    st = rrdset_create_localhost("ipv4", "mcast", NULL, "multicast", NULL, "IPv4 Multicast Bandwidth"
                                                 , "kilobits/s", 9000, update_every, RRDSET_TYPE_AREA);
                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rrddim_add(st, "InMcastOctets",  "received", 8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "OutMcastOctets", "sent",    -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "InMcastOctets", ipext_InMcastOctets);
                rrddim_set(st, "OutMcastOctets", ipext_OutMcastOctets);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_bcast == CONFIG_BOOLEAN_YES || (do_bcast == CONFIG_BOOLEAN_AUTO && (ipext_InBcastOctets || ipext_OutBcastOctets))) {
                do_bcast = CONFIG_BOOLEAN_YES;
                st = rrdset_find_localhost("ipv4.bcast");
                if(unlikely(!st)) {
                    st = rrdset_create_localhost("ipv4", "bcast", NULL, "broadcast", NULL, "IPv4 Broadcast Bandwidth"
                                                 , "kilobits/s", 8000, update_every, RRDSET_TYPE_AREA);
                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rrddim_add(st, "InBcastOctets",  "received", 8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "OutBcastOctets", "sent",    -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "InBcastOctets", ipext_InBcastOctets);
                rrddim_set(st, "OutBcastOctets", ipext_OutBcastOctets);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_mcast_p == CONFIG_BOOLEAN_YES || (do_mcast_p == CONFIG_BOOLEAN_AUTO && (ipext_InMcastPkts || ipext_OutMcastPkts))) {
                do_mcast_p = CONFIG_BOOLEAN_YES;
                st = rrdset_find_localhost("ipv4.mcastpkts");
                if(unlikely(!st)) {
                    st = rrdset_create_localhost("ipv4", "mcastpkts", NULL, "multicast", NULL, "IPv4 Multicast Packets"
                                                 , "packets/s", 8600, update_every, RRDSET_TYPE_LINE);
                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rrddim_add(st, "InMcastPkts", "received", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "OutMcastPkts", "sent", -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "InMcastPkts", ipext_InMcastPkts);
                rrddim_set(st, "OutMcastPkts", ipext_OutMcastPkts);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_bcast_p == CONFIG_BOOLEAN_YES || (do_bcast_p == CONFIG_BOOLEAN_AUTO && (ipext_InBcastPkts || ipext_OutBcastPkts))) {
                do_bcast_p = CONFIG_BOOLEAN_YES;
                st = rrdset_find_localhost("ipv4.bcastpkts");
                if(unlikely(!st)) {
                    st = rrdset_create_localhost("ipv4", "bcastpkts", NULL, "broadcast", NULL, "IPv4 Broadcast Packets"
                                                 , "packets/s", 8500, update_every, RRDSET_TYPE_LINE);
                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rrddim_add(st, "InBcastPkts", "received", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "OutBcastPkts", "sent", -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "InBcastPkts", ipext_InBcastPkts);
                rrddim_set(st, "OutBcastPkts", ipext_OutBcastPkts);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_ecn == CONFIG_BOOLEAN_YES || (do_ecn == CONFIG_BOOLEAN_AUTO && (ipext_InCEPkts || ipext_InECT0Pkts || ipext_InECT1Pkts || ipext_InNoECTPkts))) {
                do_ecn = CONFIG_BOOLEAN_YES;
                st = rrdset_find_localhost("ipv4.ecnpkts");
                if(unlikely(!st)) {
                    st = rrdset_create_localhost("ipv4", "ecnpkts", NULL, "ecn", NULL, "IPv4 ECN Statistics"
                                                 , "packets/s", 8700, update_every, RRDSET_TYPE_LINE);
                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rrddim_add(st, "InCEPkts", "CEP", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "InNoECTPkts", "NoECTP", -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "InECT0Pkts", "ECTP0", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "InECT1Pkts", "ECTP1", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "InCEPkts", ipext_InCEPkts);
                rrddim_set(st, "InNoECTPkts", ipext_InNoECTPkts);
                rrddim_set(st, "InECT0Pkts", ipext_InECT0Pkts);
                rrddim_set(st, "InECT1Pkts", ipext_InECT1Pkts);
                rrdset_done(st);
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

            RRDSET *st;

            // --------------------------------------------------------------------

            if(do_tcpext_memory == CONFIG_BOOLEAN_YES || (do_tcpext_memory == CONFIG_BOOLEAN_AUTO && (tcpext_TCPMemoryPressures))) {
                do_tcpext_memory = CONFIG_BOOLEAN_YES;
                st = rrdset_find_localhost("ipv4.tcpmemorypressures");
                if(unlikely(!st)) {
                    st = rrdset_create_localhost("ipv4", "tcpmemorypressures", NULL, "tcp", NULL, "TCP Memory Pressures"
                                                 , "events/s", 3000, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "TCPMemoryPressures",   "pressures",  1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "TCPMemoryPressures", tcpext_TCPMemoryPressures);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_tcpext_connaborts == CONFIG_BOOLEAN_YES || (do_tcpext_connaborts == CONFIG_BOOLEAN_AUTO && (tcpext_TCPAbortOnData || tcpext_TCPAbortOnClose || tcpext_TCPAbortOnMemory || tcpext_TCPAbortOnTimeout || tcpext_TCPAbortOnLinger || tcpext_TCPAbortFailed))) {
                do_tcpext_connaborts = CONFIG_BOOLEAN_YES;
                st = rrdset_find_localhost("ipv4.tcpconnaborts");
                if(unlikely(!st)) {
                    st = rrdset_create_localhost("ipv4", "tcpconnaborts", NULL, "tcp", NULL, "TCP Connection Aborts"
                                                 , "connections/s", 3010, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "TCPAbortOnData",    "baddata",     1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "TCPAbortOnClose",   "userclosed",  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "TCPAbortOnMemory",  "nomemory",    1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "TCPAbortOnTimeout", "timeout",     1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "TCPAbortOnLinger",  "linger",      1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "TCPAbortFailed",    "failed",     -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "TCPAbortOnData",    tcpext_TCPAbortOnData);
                rrddim_set(st, "TCPAbortOnClose",   tcpext_TCPAbortOnClose);
                rrddim_set(st, "TCPAbortOnMemory",  tcpext_TCPAbortOnMemory);
                rrddim_set(st, "TCPAbortOnTimeout", tcpext_TCPAbortOnTimeout);
                rrddim_set(st, "TCPAbortOnLinger",  tcpext_TCPAbortOnLinger);
                rrddim_set(st, "TCPAbortFailed",    tcpext_TCPAbortFailed);
                rrdset_done(st);
            }
            // --------------------------------------------------------------------

            if(do_tcpext_reorder == CONFIG_BOOLEAN_YES || (do_tcpext_reorder == CONFIG_BOOLEAN_AUTO && (tcpext_TCPRenoReorder || tcpext_TCPFACKReorder || tcpext_TCPSACKReorder || tcpext_TCPTSReorder))) {
                do_tcpext_reorder = CONFIG_BOOLEAN_YES;
                st = rrdset_find_localhost("ipv4.tcpreorders");
                if(unlikely(!st)) {
                    st = rrdset_create_localhost("ipv4", "tcpreorders", NULL, "tcp", NULL
                                                 , "TCP Reordered Packets by Detection Method", "packets/s", 3020
                                                 , update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "TCPTSReorder",   "timestamp",   1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "TCPSACKReorder", "sack",        1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "TCPFACKReorder", "fack",        1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "TCPRenoReorder", "reno",        1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "TCPTSReorder",   tcpext_TCPTSReorder);
                rrddim_set(st, "TCPSACKReorder", tcpext_TCPSACKReorder);
                rrddim_set(st, "TCPFACKReorder", tcpext_TCPFACKReorder);
                rrddim_set(st, "TCPRenoReorder", tcpext_TCPRenoReorder);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_tcpext_ofo == CONFIG_BOOLEAN_YES || (do_tcpext_ofo == CONFIG_BOOLEAN_AUTO && (tcpext_TCPOFOQueue || tcpext_TCPOFODrop || tcpext_TCPOFOMerge))) {
                do_tcpext_ofo = CONFIG_BOOLEAN_YES;
                st = rrdset_find_localhost("ipv4.tcpofo");
                if(unlikely(!st)) {
                    st = rrdset_create_localhost("ipv4", "tcpofo", NULL, "tcp", NULL, "TCP Out-Of-Order Queue"
                                                 , "packets/s", 3050, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "TCPOFOQueue", "inqueue",  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "TCPOFODrop",  "dropped", -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "TCPOFOMerge", "merged",   1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "OfoPruned",   "pruned",  -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "TCPOFOQueue",   tcpext_TCPOFOQueue);
                rrddim_set(st, "TCPOFODrop",    tcpext_TCPOFODrop);
                rrddim_set(st, "TCPOFOMerge",   tcpext_TCPOFOMerge);
                rrddim_set(st, "OfoPruned",     tcpext_OfoPruned);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_tcpext_syscookies == CONFIG_BOOLEAN_YES || (do_tcpext_syscookies == CONFIG_BOOLEAN_AUTO && (tcpext_SyncookiesSent || tcpext_SyncookiesRecv || tcpext_SyncookiesFailed))) {
                do_tcpext_syscookies = CONFIG_BOOLEAN_YES;
                st = rrdset_find_localhost("ipv4.tcpsyncookies");
                if(unlikely(!st)) {
                    st = rrdset_create_localhost("ipv4", "tcpsyncookies", NULL, "tcp", NULL, "TCP SYN Cookies"
                                                 , "packets/s", 3100, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "SyncookiesRecv",   "received",  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "SyncookiesSent",   "sent",     -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "SyncookiesFailed", "failed",   -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "SyncookiesRecv",   tcpext_SyncookiesRecv);
                rrddim_set(st, "SyncookiesSent",   tcpext_SyncookiesSent);
                rrddim_set(st, "SyncookiesFailed", tcpext_SyncookiesFailed);
                rrdset_done(st);
            }

        }
    }

    return 0;
}

#include "common.h"

#define RRD_TYPE_NET_SNMP           "ipv4"
#define RRD_TYPE_NET_SNMP_LEN       strlen(RRD_TYPE_NET_SNMP)

int do_proc_net_snmp(int update_every, unsigned long long dt) {
    (void)dt;

    static procfile *ff = NULL;
    static int do_ip_packets = -1, do_ip_fragsout = -1, do_ip_fragsin = -1, do_ip_errors = -1,
        do_tcp_sockets = -1, do_tcp_packets = -1, do_tcp_errors = -1, do_tcp_handshake = -1,
        do_udp_packets = -1, do_udp_errors = -1, do_icmp_packets = -1, do_icmpmsg = -1, do_udplite_packets = -1;
    static uint32_t hash_ip = 0, hash_icmp = 0, hash_tcp = 0, hash_udp = 0, hash_icmpmsg = 0, hash_udplite = 0;

    if(unlikely(do_ip_packets == -1)) {
        do_ip_packets       = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 packets", 1);
        do_ip_fragsout      = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 fragments sent", 1);
        do_ip_fragsin       = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 fragments assembly", 1);
        do_ip_errors        = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 errors", 1);
        do_tcp_sockets      = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 TCP connections", 1);
        do_tcp_packets      = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 TCP packets", 1);
        do_tcp_errors       = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 TCP errors", 1);
        do_tcp_handshake    = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 TCP handshake issues", 1);
        do_udp_packets      = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 UDP packets", 1);
        do_udp_errors       = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 UDP errors", 1);
        do_icmp_packets     = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 ICMP packets", 1);
        do_icmpmsg          = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 ICMP messages", 1);
        do_udplite_packets  = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 UDPLite packets", 1);

        hash_ip = simple_hash("Ip");
        hash_tcp = simple_hash("Tcp");
        hash_udp = simple_hash("Udp");
        hash_icmp = simple_hash("Icmp");
        hash_icmpmsg = simple_hash("IcmpMsg");
        hash_udplite = simple_hash("UdpLite");
    }

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", global_host_prefix, "/proc/net/snmp");
        ff = procfile_open(config_get("plugin:proc:/proc/net/snmp", "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
    }
    if(unlikely(!ff)) return 1;

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 0; // we return 0, so that we will retry to open it next time

    uint32_t lines = procfile_lines(ff), l;
    uint32_t words;

    RRDSET *st;

    for(l = 0; l < lines ;l++) {
        char *key = procfile_lineword(ff, l, 0);
        uint32_t hash = simple_hash(key);

        if(unlikely(hash == hash_ip && strcmp(key, "Ip") == 0)) {
            l++;

            if(strcmp(procfile_lineword(ff, l, 0), "Ip") != 0) {
                error("Cannot read Ip line from /proc/net/snmp.");
                break;
            }

            words = procfile_linewords(ff, l);
            if(words < 20) {
                error("Cannot read /proc/net/snmp Ip line. Expected 20 params, read %u.", words);
                continue;
            }

            // see also http://net-snmp.sourceforge.net/docs/mibs/ip.html
            unsigned long long Forwarding, DefaultTTL, InReceives, InHdrErrors, InAddrErrors, ForwDatagrams, InUnknownProtos, InDiscards, InDelivers,
                OutRequests, OutDiscards, OutNoRoutes, ReasmTimeout, ReasmReqds, ReasmOKs, ReasmFails, FragOKs, FragFails, FragCreates;

            //Forwarding      = strtoull(procfile_lineword(ff, l, 1), NULL, 10);
            //DefaultTTL      = strtoull(procfile_lineword(ff, l, 2), NULL, 10);
            InReceives      = strtoull(procfile_lineword(ff, l, 3), NULL, 10);
            InHdrErrors     = strtoull(procfile_lineword(ff, l, 4), NULL, 10);
            InAddrErrors    = strtoull(procfile_lineword(ff, l, 5), NULL, 10);
            ForwDatagrams   = strtoull(procfile_lineword(ff, l, 6), NULL, 10);
            InUnknownProtos = strtoull(procfile_lineword(ff, l, 7), NULL, 10);
            InDiscards      = strtoull(procfile_lineword(ff, l, 8), NULL, 10);
            InDelivers      = strtoull(procfile_lineword(ff, l, 9), NULL, 10);
            OutRequests     = strtoull(procfile_lineword(ff, l, 10), NULL, 10);
            OutDiscards     = strtoull(procfile_lineword(ff, l, 11), NULL, 10);
            OutNoRoutes     = strtoull(procfile_lineword(ff, l, 12), NULL, 10);
            //ReasmTimeout    = strtoull(procfile_lineword(ff, l, 13), NULL, 10);
            ReasmReqds      = strtoull(procfile_lineword(ff, l, 14), NULL, 10);
            ReasmOKs        = strtoull(procfile_lineword(ff, l, 15), NULL, 10);
            ReasmFails      = strtoull(procfile_lineword(ff, l, 16), NULL, 10);
            FragOKs         = strtoull(procfile_lineword(ff, l, 17), NULL, 10);
            FragFails       = strtoull(procfile_lineword(ff, l, 18), NULL, 10);
            FragCreates     = strtoull(procfile_lineword(ff, l, 19), NULL, 10);

            // these are not counters
            (void)Forwarding;      // is forwarding enabled?
            (void)DefaultTTL;      // the default ttl on packets
            (void)ReasmTimeout;    // Reassembly timeout

            // --------------------------------------------------------------------

            if(do_ip_packets) {
                st = rrdset_find(RRD_TYPE_NET_SNMP ".packets");
                if(!st) {
                    st = rrdset_create(RRD_TYPE_NET_SNMP, "packets", NULL, "packets", NULL, "IPv4 Packets", "packets/s", 3000, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "received", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "sent", NULL, -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "forwarded", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "delivered", NULL, 1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "sent", OutRequests);
                rrddim_set(st, "received", InReceives);
                rrddim_set(st, "forwarded", ForwDatagrams);
                rrddim_set(st, "delivered", InDelivers);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_ip_fragsout) {
                st = rrdset_find(RRD_TYPE_NET_SNMP ".fragsout");
                if(!st) {
                    st = rrdset_create(RRD_TYPE_NET_SNMP, "fragsout", NULL, "fragments", NULL, "IPv4 Fragments Sent", "packets/s", 3010, update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "ok", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "failed", NULL, -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "all", NULL, 1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "ok", FragOKs);
                rrddim_set(st, "failed", FragFails);
                rrddim_set(st, "all", FragCreates);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_ip_fragsin) {
                st = rrdset_find(RRD_TYPE_NET_SNMP ".fragsin");
                if(!st) {
                    st = rrdset_create(RRD_TYPE_NET_SNMP, "fragsin", NULL, "fragments", NULL, "IPv4 Fragments Reassembly", "packets/s", 3011, update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "ok", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "failed", NULL, -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "all", NULL, 1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "ok", ReasmOKs);
                rrddim_set(st, "failed", ReasmFails);
                rrddim_set(st, "all", ReasmReqds);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_ip_errors) {
                st = rrdset_find(RRD_TYPE_NET_SNMP ".errors");
                if(!st) {
                    st = rrdset_create(RRD_TYPE_NET_SNMP, "errors", NULL, "errors", NULL, "IPv4 Errors", "packets/s", 3002, update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "InDiscards", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutDiscards", NULL, -1, 1, RRDDIM_INCREMENTAL);

                    rrddim_add(st, "InHdrErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutNoRoutes", NULL, -1, 1, RRDDIM_INCREMENTAL);

                    rrddim_add(st, "InAddrErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InUnknownProtos", NULL, 1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "InDiscards", InDiscards);
                rrddim_set(st, "OutDiscards", OutDiscards);
                rrddim_set(st, "InHdrErrors", InHdrErrors);
                rrddim_set(st, "InAddrErrors", InAddrErrors);
                rrddim_set(st, "InUnknownProtos", InUnknownProtos);
                rrddim_set(st, "OutNoRoutes", OutNoRoutes);
                rrdset_done(st);
            }
        }
        else if(unlikely(hash == hash_icmp && strcmp(key, "Icmp") == 0)) {
            l++;

            if(strcmp(procfile_lineword(ff, l, 0), "Icmp") != 0) {
                error("Cannot read Icmp line from /proc/net/snmp.");
                break;
            }

            words = procfile_linewords(ff, l);
            if(words < 28) {
                error("Cannot read /proc/net/snmp Icmp line. Expected 28 params, read %u.", words);
                continue;
            }

            unsigned long long InMsgs, InErrors, InCsumErrors, InDestUnreachs, InTimeExcds, InParmProbs, InSrcQuenchs, InRedirects, InEchos,
                InEchoReps, InTimestamps, InTimestampReps, InAddrMasks, InAddrMaskReps, OutMsgs, OutErrors, OutDestUnreachs, OutTimeExcds,
                OutParmProbs, OutSrcQuenchs, OutRedirects, OutEchos, OutEchoReps, OutTimestamps, OutTimestampReps, OutAddrMasks, OutAddrMaskReps;

            InMsgs           = strtoull(procfile_lineword(ff, l, 1), NULL, 10);
            InErrors         = strtoull(procfile_lineword(ff, l, 2), NULL, 10);
            InCsumErrors     = strtoull(procfile_lineword(ff, l, 3), NULL, 10);
            InDestUnreachs   = strtoull(procfile_lineword(ff, l, 4), NULL, 10);
            InTimeExcds      = strtoull(procfile_lineword(ff, l, 5), NULL, 10);
            InParmProbs      = strtoull(procfile_lineword(ff, l, 6), NULL, 10);
            InSrcQuenchs     = strtoull(procfile_lineword(ff, l, 7), NULL, 10);
            InRedirects      = strtoull(procfile_lineword(ff, l, 8), NULL, 10);
            InEchos          = strtoull(procfile_lineword(ff, l, 9), NULL, 10);
            InEchoReps       = strtoull(procfile_lineword(ff, l, 10), NULL, 10);
            InTimestamps     = strtoull(procfile_lineword(ff, l, 11), NULL, 10);
            InTimestampReps  = strtoull(procfile_lineword(ff, l, 12), NULL, 10);
            InAddrMasks      = strtoull(procfile_lineword(ff, l, 13), NULL, 10);
            InAddrMaskReps   = strtoull(procfile_lineword(ff, l, 14), NULL, 10);

            OutMsgs          = strtoull(procfile_lineword(ff, l, 15), NULL, 10);
            OutErrors        = strtoull(procfile_lineword(ff, l, 16), NULL, 10);
            OutDestUnreachs  = strtoull(procfile_lineword(ff, l, 17), NULL, 10);
            OutTimeExcds     = strtoull(procfile_lineword(ff, l, 18), NULL, 10);
            OutParmProbs     = strtoull(procfile_lineword(ff, l, 19), NULL, 10);
            OutSrcQuenchs    = strtoull(procfile_lineword(ff, l, 20), NULL, 10);
            OutRedirects     = strtoull(procfile_lineword(ff, l, 21), NULL, 10);
            OutEchos         = strtoull(procfile_lineword(ff, l, 22), NULL, 10);
            OutEchoReps      = strtoull(procfile_lineword(ff, l, 23), NULL, 10);
            OutTimestamps    = strtoull(procfile_lineword(ff, l, 24), NULL, 10);
            OutTimestampReps = strtoull(procfile_lineword(ff, l, 25), NULL, 10);
            OutAddrMasks     = strtoull(procfile_lineword(ff, l, 26), NULL, 10);
            OutAddrMaskReps  = strtoull(procfile_lineword(ff, l, 27), NULL, 10);

            // --------------------------------------------------------------------

            if(do_icmp_packets) {
                st = rrdset_find(RRD_TYPE_NET_SNMP ".icmp");
                if(!st) {
                    st = rrdset_create(RRD_TYPE_NET_SNMP, "icmp", NULL, "icmp", NULL, "IPv4 ICMP Packets", "packets/s", 2602, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "InMsgs",           NULL,  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutMsgs",          NULL, -1, 1, RRDDIM_INCREMENTAL);

                    rrddim_add(st, "InErrors",         NULL,  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutErrors",        NULL, -1, 1, RRDDIM_INCREMENTAL);

                    rrddim_add(st, "InDestUnreachs",   NULL,  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutDestUnreachs",  NULL, -1, 1, RRDDIM_INCREMENTAL);

                    rrddim_add(st, "InTimeExcds",      NULL,  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutTimeExcds",     NULL, -1, 1, RRDDIM_INCREMENTAL);

                    rrddim_add(st, "InParmProbs",      NULL,  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutParmProbs",     NULL, -1, 1, RRDDIM_INCREMENTAL);

                    rrddim_add(st, "InSrcQuenchs",     NULL,  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutSrcQuenchs",    NULL, -1, 1, RRDDIM_INCREMENTAL);

                    rrddim_add(st, "InRedirects",      NULL,  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutRedirects",     NULL, -1, 1, RRDDIM_INCREMENTAL);

                    rrddim_add(st, "InEchos",          NULL,  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutEchos",         NULL, -1, 1, RRDDIM_INCREMENTAL);

                    rrddim_add(st, "InEchoReps",       NULL,  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutEchoReps",      NULL, -1, 1, RRDDIM_INCREMENTAL);

                    rrddim_add(st, "InTimestamps",     NULL,  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutTimestamps",    NULL, -1, 1, RRDDIM_INCREMENTAL);

                    rrddim_add(st, "InTimestampReps",  NULL,  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutTimestampReps", NULL, -1, 1, RRDDIM_INCREMENTAL);

                    rrddim_add(st, "InAddrMasks",      NULL,  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutAddrMasks",     NULL, -1, 1, RRDDIM_INCREMENTAL);

                    rrddim_add(st, "InAddrMaskReps",   NULL,  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutAddrMaskReps",  NULL, -1, 1, RRDDIM_INCREMENTAL);

                    rrddim_add(st, "InCsumErrors",     NULL, 1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "InMsgs", InMsgs);
                rrddim_set(st, "InErrors", InErrors);
                rrddim_set(st, "InCsumErrors", InCsumErrors);
                rrddim_set(st, "InDestUnreachs", InDestUnreachs);
                rrddim_set(st, "InTimeExcds", InTimeExcds);
                rrddim_set(st, "InParmProbs", InParmProbs);
                rrddim_set(st, "InSrcQuenchs", InSrcQuenchs);
                rrddim_set(st, "InRedirects", InRedirects);
                rrddim_set(st, "InEchos", InEchos);
                rrddim_set(st, "InEchoReps", InEchoReps);
                rrddim_set(st, "InTimestamps", InTimestamps);
                rrddim_set(st, "InTimestampReps", InTimestampReps);
                rrddim_set(st, "InAddrMasks", InAddrMasks);
                rrddim_set(st, "InAddrMaskReps", InAddrMaskReps);

                rrddim_set(st, "OutMsgs", OutMsgs);
                rrddim_set(st, "OutErrors", OutErrors);
                rrddim_set(st, "OutDestUnreachs", OutDestUnreachs);
                rrddim_set(st, "OutTimeExcds", OutTimeExcds);
                rrddim_set(st, "OutParmProbs", OutParmProbs);
                rrddim_set(st, "OutSrcQuenchs", OutSrcQuenchs);
                rrddim_set(st, "OutRedirects", OutRedirects);
                rrddim_set(st, "OutEchos", OutEchos);
                rrddim_set(st, "OutEchoReps", OutEchoReps);
                rrddim_set(st, "OutTimestamps", OutTimestamps);
                rrddim_set(st, "OutTimestampReps", OutTimestampReps);
                rrddim_set(st, "OutAddrMasks", OutAddrMasks);
                rrddim_set(st, "OutAddrMaskReps", OutAddrMaskReps);
                rrdset_done(st);
            }
        }
        else if(unlikely(hash == hash_icmpmsg && strcmp(key, "IcmpMsg") == 0)) {
            l++;

            if(strcmp(procfile_lineword(ff, l, 0), "IcmpMsg") != 0) {
                error("Cannot read IcmpMsg line from /proc/net/snmp.");
                break;
            }

            words = procfile_linewords(ff, l);
            if(words < 12) {
                error("Cannot read /proc/net/snmp IcmpMsg line. Expected 12 params, read %u.", words);
                continue;
            }

            unsigned long long InType0, InType3, InType4, InType5, InType8, InType11, OutType0, OutType3, OutType5, OutType8, OutType11;

            InType0   = strtoull(procfile_lineword(ff, l, 1), NULL, 10);
            InType3   = strtoull(procfile_lineword(ff, l, 2), NULL, 10);
            InType4   = strtoull(procfile_lineword(ff, l, 3), NULL, 10);
            InType5   = strtoull(procfile_lineword(ff, l, 4), NULL, 10);
            InType8   = strtoull(procfile_lineword(ff, l, 5), NULL, 10);
            InType11  = strtoull(procfile_lineword(ff, l, 6), NULL, 10);

            OutType0  = strtoull(procfile_lineword(ff, l, 7), NULL, 10);
            OutType3  = strtoull(procfile_lineword(ff, l, 8), NULL, 10);
            OutType5  = strtoull(procfile_lineword(ff, l, 9), NULL, 10);
            OutType8  = strtoull(procfile_lineword(ff, l, 10), NULL, 10);
            OutType11 = strtoull(procfile_lineword(ff, l, 11), NULL, 10);

            // --------------------------------------------------------------------

            if(do_icmpmsg) {
                st = rrdset_find(RRD_TYPE_NET_SNMP ".icmpmsg");
                if(!st) {
                    st = rrdset_create(RRD_TYPE_NET_SNMP, "icmpmsg", NULL, "icmp", NULL, "IPv4 ICMP Messsages", "packets/s", 2603, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "InType0",  NULL,  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutType0", NULL, -1, 1, RRDDIM_INCREMENTAL);

                    rrddim_add(st, "InType3",  NULL,  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutType3", NULL, -1, 1, RRDDIM_INCREMENTAL);

                    rrddim_add(st, "InType5",  NULL,  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutType5", NULL, -1, 1, RRDDIM_INCREMENTAL);

                    rrddim_add(st, "InType8",  NULL,  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutType8", NULL, -1, 1, RRDDIM_INCREMENTAL);

                    rrddim_add(st, "InType11",  NULL,  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutType11", NULL, -1, 1, RRDDIM_INCREMENTAL);

                    rrddim_add(st, "InType4", NULL, 1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "InType0", InType0);
                rrddim_set(st, "InType3", InType3);
                rrddim_set(st, "InType4", InType4);
                rrddim_set(st, "InType5", InType5);
                rrddim_set(st, "InType8", InType8);
                rrddim_set(st, "InType11", InType11);

                rrddim_set(st, "OutType0", OutType0);
                rrddim_set(st, "OutType3", OutType3);
                rrddim_set(st, "OutType5", OutType5);
                rrddim_set(st, "OutType8", OutType8);
                rrddim_set(st, "OutType11", OutType11);
                rrdset_done(st);
            }
        }
        else if(unlikely(hash == hash_tcp && strcmp(key, "Tcp") == 0)) {
            l++;

            if(strcmp(procfile_lineword(ff, l, 0), "Tcp") != 0) {
                error("Cannot read Tcp line from /proc/net/snmp.");
                break;
            }

            words = procfile_linewords(ff, l);
            if(words < 15) {
                error("Cannot read /proc/net/snmp Tcp line. Expected 15 params, read %u.", words);
                continue;
            }

            unsigned long long RtoAlgorithm, RtoMin, RtoMax, MaxConn, ActiveOpens, PassiveOpens, AttemptFails, EstabResets,
                CurrEstab, InSegs, OutSegs, RetransSegs, InErrs, OutRsts;

            //RtoAlgorithm    = strtoull(procfile_lineword(ff, l, 1), NULL, 10);
            //RtoMin          = strtoull(procfile_lineword(ff, l, 2), NULL, 10);
            //RtoMax          = strtoull(procfile_lineword(ff, l, 3), NULL, 10);
            //MaxConn         = strtoull(procfile_lineword(ff, l, 4), NULL, 10);
            ActiveOpens     = strtoull(procfile_lineword(ff, l, 5), NULL, 10);
            PassiveOpens    = strtoull(procfile_lineword(ff, l, 6), NULL, 10);
            AttemptFails    = strtoull(procfile_lineword(ff, l, 7), NULL, 10);
            EstabResets     = strtoull(procfile_lineword(ff, l, 8), NULL, 10);
            CurrEstab       = strtoull(procfile_lineword(ff, l, 9), NULL, 10);
            InSegs          = strtoull(procfile_lineword(ff, l, 10), NULL, 10);
            OutSegs         = strtoull(procfile_lineword(ff, l, 11), NULL, 10);
            RetransSegs     = strtoull(procfile_lineword(ff, l, 12), NULL, 10);
            InErrs          = strtoull(procfile_lineword(ff, l, 13), NULL, 10);
            OutRsts         = strtoull(procfile_lineword(ff, l, 14), NULL, 10);

            // these are not counters
            (void)RtoAlgorithm;
            (void)RtoMin;
            (void)RtoMax;
            (void)MaxConn;

            // --------------------------------------------------------------------

            // see http://net-snmp.sourceforge.net/docs/mibs/tcp.html
            if(do_tcp_sockets) {
                st = rrdset_find(RRD_TYPE_NET_SNMP ".tcpsock");
                if(!st) {
                    st = rrdset_create(RRD_TYPE_NET_SNMP, "tcpsock", NULL, "tcp", NULL, "IPv4 TCP Connections", "active connections", 2500, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "connections", NULL, 1, 1, RRDDIM_ABSOLUTE);
                }
                else rrdset_next(st);

                rrddim_set(st, "connections", CurrEstab);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_tcp_packets) {
                st = rrdset_find(RRD_TYPE_NET_SNMP ".tcppackets");
                if(!st) {
                    st = rrdset_create(RRD_TYPE_NET_SNMP, "tcppackets", NULL, "tcp", NULL, "IPv4 TCP Packets", "packets/s", 2600, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "received", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "sent", NULL, -1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "received", InSegs);
                rrddim_set(st, "sent", OutSegs);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_tcp_errors) {
                st = rrdset_find(RRD_TYPE_NET_SNMP ".tcperrors");
                if(!st) {
                    st = rrdset_create(RRD_TYPE_NET_SNMP, "tcperrors", NULL, "tcp", NULL, "IPv4 TCP Errors", "packets/s", 2700, update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "InErrs", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "RetransSegs", NULL, -1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "InErrs", InErrs);
                rrddim_set(st, "RetransSegs", RetransSegs);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_tcp_handshake) {
                st = rrdset_find(RRD_TYPE_NET_SNMP ".tcphandshake");
                if(!st) {
                    st = rrdset_create(RRD_TYPE_NET_SNMP, "tcphandshake", NULL, "tcp", NULL, "IPv4 TCP Handshake Issues", "events/s", 2900, update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "EstabResets", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutRsts", NULL, -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "ActiveOpens", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "PassiveOpens", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "AttemptFails", NULL, 1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "EstabResets", EstabResets);
                rrddim_set(st, "OutRsts", OutRsts);
                rrddim_set(st, "ActiveOpens", ActiveOpens);
                rrddim_set(st, "PassiveOpens", PassiveOpens);
                rrddim_set(st, "AttemptFails", AttemptFails);
                rrdset_done(st);
            }
        }
        else if(unlikely(hash == hash_udp && strcmp(key, "Udp") == 0)) {
            l++;

            if(strcmp(procfile_lineword(ff, l, 0), "Udp") != 0) {
                error("Cannot read Udp line from /proc/net/snmp.");
                break;
            }

            words = procfile_linewords(ff, l);
            if(words < 7) {
                error("Cannot read /proc/net/snmp Udp line. Expected 7 params, read %u.", words);
                continue;
            }

            unsigned long long InDatagrams, NoPorts, InErrors, OutDatagrams, RcvbufErrors, SndbufErrors;

            InDatagrams     = strtoull(procfile_lineword(ff, l, 1), NULL, 10);
            NoPorts         = strtoull(procfile_lineword(ff, l, 2), NULL, 10);
            InErrors        = strtoull(procfile_lineword(ff, l, 3), NULL, 10);
            OutDatagrams    = strtoull(procfile_lineword(ff, l, 4), NULL, 10);
            RcvbufErrors    = strtoull(procfile_lineword(ff, l, 5), NULL, 10);
            SndbufErrors    = strtoull(procfile_lineword(ff, l, 6), NULL, 10);

            // --------------------------------------------------------------------

            // see http://net-snmp.sourceforge.net/docs/mibs/udp.html
            if(do_udp_packets) {
                st = rrdset_find(RRD_TYPE_NET_SNMP ".udppackets");
                if(!st) {
                    st = rrdset_create(RRD_TYPE_NET_SNMP, "udppackets", NULL, "udp", NULL, "IPv4 UDP Packets", "packets/s", 2601, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "received", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "sent", NULL, -1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "received", InDatagrams);
                rrddim_set(st, "sent", OutDatagrams);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_udp_errors) {
                st = rrdset_find(RRD_TYPE_NET_SNMP ".udperrors");
                if(!st) {
                    st = rrdset_create(RRD_TYPE_NET_SNMP, "udperrors", NULL, "udp", NULL, "IPv4 UDP Errors", "events/s", 2701, update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "RcvbufErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "SndbufErrors", NULL, -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "NoPorts", NULL, 1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "InErrors", InErrors);
                rrddim_set(st, "NoPorts", NoPorts);
                rrddim_set(st, "RcvbufErrors", RcvbufErrors);
                rrddim_set(st, "SndbufErrors", SndbufErrors);
                rrdset_done(st);
            }
        }
        else if(unlikely(hash == hash_udplite && strcmp(key, "UdpLite") == 0)) {
            l++;

            if(strcmp(procfile_lineword(ff, l, 0), "UdpLite") != 0) {
                error("Cannot read UdpLite line from /proc/net/snmp.");
                break;
            }

            words = procfile_linewords(ff, l);
            if(words < 9) {
                error("Cannot read /proc/net/snmp UdpLite line. Expected 9 params, read %u.", words);
                continue;
            }

            unsigned long long InDatagrams, NoPorts, InErrors, OutDatagrams, RcvbufErrors, SndbufErrors, InCsumErrors, IgnoredMulti;

            InDatagrams  = strtoull(procfile_lineword(ff, l, 1), NULL, 10);
            NoPorts      = strtoull(procfile_lineword(ff, l, 2), NULL, 10);
            InErrors     = strtoull(procfile_lineword(ff, l, 3), NULL, 10);
            OutDatagrams = strtoull(procfile_lineword(ff, l, 4), NULL, 10);
            RcvbufErrors = strtoull(procfile_lineword(ff, l, 5), NULL, 10);
            SndbufErrors = strtoull(procfile_lineword(ff, l, 6), NULL, 10);
            InCsumErrors = strtoull(procfile_lineword(ff, l, 7), NULL, 10);
            IgnoredMulti = strtoull(procfile_lineword(ff, l, 8), NULL, 10);

            // --------------------------------------------------------------------

            if(do_udplite_packets) {
                st = rrdset_find(RRD_TYPE_NET_SNMP ".udplite");
                if(!st) {
                    st = rrdset_create(RRD_TYPE_NET_SNMP, "udplite", NULL, "udplite", NULL, "IPv4 UDPLite Packets", "packets/s", 2603, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "InDatagrams", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutDatagrams", NULL, -1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "InDatagrams", InDatagrams);
                rrddim_set(st, "OutDatagrams", OutDatagrams);
                rrdset_done(st);

                st = rrdset_find(RRD_TYPE_NET_SNMP ".udplite_errors");
                if(!st) {
                    st = rrdset_create(RRD_TYPE_NET_SNMP, "udplite_errors", NULL, "udplite", NULL, "IPv4 UDPLite Errors", "packets/s", 2604, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "RcvbufErrors", NULL,  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "SndbufErrors", NULL, -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "NoPorts",      NULL,  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "IgnoredMulti", NULL,  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InErrors",     NULL,  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InCsumErrors", NULL,  1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "NoPorts", NoPorts);
                rrddim_set(st, "InErrors", InErrors);
                rrddim_set(st, "InCsumErrors", InCsumErrors);
                rrddim_set(st, "RcvbufErrors", RcvbufErrors);
                rrddim_set(st, "SndbufErrors", SndbufErrors);
                rrddim_set(st, "IgnoredMulti", IgnoredMulti);
                rrdset_done(st);
            }
        }
    }

    return 0;
}


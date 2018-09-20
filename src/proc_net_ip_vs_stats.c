// SPDX-License-Identifier: GPL-3.0+

#include "common.h"

#define RRD_TYPE_NET_IPVS           "ipvs"

int do_proc_net_ip_vs_stats(int update_every, usec_t dt) {
    (void)dt;
    static int do_bandwidth = -1, do_sockets = -1, do_packets = -1;
    static procfile *ff = NULL;

    if(do_bandwidth == -1)  do_bandwidth    = config_get_boolean("plugin:proc:/proc/net/ip_vs_stats", "IPVS bandwidth", 1);
    if(do_sockets == -1)    do_sockets      = config_get_boolean("plugin:proc:/proc/net/ip_vs_stats", "IPVS connections", 1);
    if(do_packets == -1)    do_packets      = config_get_boolean("plugin:proc:/proc/net/ip_vs_stats", "IPVS packets", 1);

    if(!ff) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/net/ip_vs_stats");
        ff = procfile_open(config_get("plugin:proc:/proc/net/ip_vs_stats", "filename to monitor", filename), " \t,:|", PROCFILE_FLAG_DEFAULT);
    }
    if(!ff) return 1;

    ff = procfile_readall(ff);
    if(!ff) return 0; // we return 0, so that we will retry to open it next time

    // make sure we have 3 lines
    if(procfile_lines(ff) < 3) return 1;

    // make sure we have 5 words on the 3rd line
    if(procfile_linewords(ff, 2) < 5) return 1;

    unsigned long long entries, InPackets, OutPackets, InBytes, OutBytes;

    entries     = strtoull(procfile_lineword(ff, 2, 0), NULL, 16);
    InPackets   = strtoull(procfile_lineword(ff, 2, 1), NULL, 16);
    OutPackets  = strtoull(procfile_lineword(ff, 2, 2), NULL, 16);
    InBytes     = strtoull(procfile_lineword(ff, 2, 3), NULL, 16);
    OutBytes    = strtoull(procfile_lineword(ff, 2, 4), NULL, 16);


    // --------------------------------------------------------------------

    if(do_sockets) {
        static RRDSET *st = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IPVS
                    , "sockets"
                    , NULL
                    , RRD_TYPE_NET_IPVS
                    , NULL
                    , "IPVS New Connections"
                    , "connections/s"
                    , "proc"
                    , "net/ip_vs_stats"
                    , 3101
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rrddim_add(st, "connections", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set(st, "connections", entries);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_packets) {
        static RRDSET *st = NULL;
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IPVS
                    , "packets"
                    , NULL
                    , RRD_TYPE_NET_IPVS
                    , NULL
                    , "IPVS Packets"
                    , "packets/s"
                    , "proc"
                    , "net/ip_vs_stats"
                    , 3102
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rrddim_add(st, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "sent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set(st, "received", InPackets);
        rrddim_set(st, "sent", OutPackets);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_bandwidth) {
        static RRDSET *st = NULL;
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_IPVS
                    , "net"
                    , NULL
                    , RRD_TYPE_NET_IPVS
                    , NULL
                    , "IPVS Bandwidth"
                    , "kilobits/s"
                    , "proc"
                    , "net/ip_vs_stats"
                    , 3100
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rrddim_add(st, "received", NULL, 8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "sent", NULL,    -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set(st, "received", InBytes);
        rrddim_set(st, "sent", OutBytes);
        rrdset_done(st);
    }

    return 0;
}

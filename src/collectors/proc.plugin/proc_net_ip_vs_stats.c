// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

#define RRD_TYPE_NET_IPVS "ipvs"
#define PLUGIN_PROC_MODULE_NET_IPVS_NAME "/proc/net/ip_vs_stats"
#define CONFIG_SECTION_PLUGIN_PROC_NET_IPVS "plugin:" PLUGIN_PROC_CONFIG_NAME ":" PLUGIN_PROC_MODULE_NET_IPVS_NAME

int do_proc_net_ip_vs_stats(int update_every, usec_t dt) {
    (void)dt;
    static int do_bandwidth = -1, do_sockets = -1, do_packets = -1;
    static procfile *ff = NULL;

    if(do_bandwidth == -1)  do_bandwidth    = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_NET_IPVS, "IPVS bandwidth", 1);
    if(do_sockets == -1)    do_sockets      = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_NET_IPVS, "IPVS connections", 1);
    if(do_packets == -1)    do_packets      = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_NET_IPVS, "IPVS packets", 1);

    if(!ff) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/net/ip_vs_stats");
        ff = procfile_open(inicfg_get(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_NET_IPVS, "filename to monitor", filename), " \t,:|", PROCFILE_FLAG_DEFAULT);
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
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NET_IPVS_NAME
                    , NETDATA_CHART_PRIO_IPVS_SOCKETS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rrddim_add(st, "connections", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set(st, "connections", entries);
        rrdset_done(st);
    }

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
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NET_IPVS_NAME
                    , NETDATA_CHART_PRIO_IPVS_PACKETS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rrddim_add(st, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "sent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set(st, "received", InPackets);
        rrddim_set(st, "sent", OutPackets);
        rrdset_done(st);
    }

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
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NET_IPVS_NAME
                    , NETDATA_CHART_PRIO_IPVS_NET
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rrddim_add(st, "received", NULL, 8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "sent", NULL,    -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set(st, "received", InBytes);
        rrddim_set(st, "sent", OutBytes);
        rrdset_done(st);
    }

    return 0;
}

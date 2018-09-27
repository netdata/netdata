// SPDX-License-Identifier: GPL-3.0+

#include "common.h"

static struct proc_net_sockstat6 {
    kernel_uint_t tcp6_inuse;
    kernel_uint_t udp6_inuse;
    kernel_uint_t udplite6_inuse;
    kernel_uint_t raw6_inuse;
    kernel_uint_t frag6_inuse;
} sockstat6_root = { 0 };

int do_proc_net_sockstat6(int update_every, usec_t dt) {
    (void)dt;

    static procfile *ff = NULL;

    static uint32_t hash_raw = 0,
                    hash_frag = 0,
                    hash_tcp = 0,
                    hash_udp = 0,
                    hash_udplite = 0;

    static ARL_BASE *arl_tcp = NULL;
    static ARL_BASE *arl_udp = NULL;
    static ARL_BASE *arl_udplite = NULL;
    static ARL_BASE *arl_raw = NULL;
    static ARL_BASE *arl_frag = NULL;

    static int do_tcp_sockets = -1, do_udp_sockets = -1, do_udplite_sockets = -1, do_raw_sockets = -1, do_frag_sockets = -1;

    static char     *keys[6]  = { NULL };
    static uint32_t hashes[6] = { 0 };
    static ARL_BASE *bases[6] = { NULL };

    if(unlikely(!arl_tcp)) {
        do_tcp_sockets     = config_get_boolean_ondemand("plugin:proc:/proc/net/sockstat6", "ipv6 TCP sockets", CONFIG_BOOLEAN_AUTO);
        do_udp_sockets     = config_get_boolean_ondemand("plugin:proc:/proc/net/sockstat6", "ipv6 UDP sockets", CONFIG_BOOLEAN_AUTO);
        do_udplite_sockets = config_get_boolean_ondemand("plugin:proc:/proc/net/sockstat6", "ipv6 UDPLITE sockets", CONFIG_BOOLEAN_AUTO);
        do_raw_sockets     = config_get_boolean_ondemand("plugin:proc:/proc/net/sockstat6", "ipv6 RAW sockets", CONFIG_BOOLEAN_AUTO);
        do_frag_sockets    = config_get_boolean_ondemand("plugin:proc:/proc/net/sockstat6", "ipv6 FRAG sockets", CONFIG_BOOLEAN_AUTO);

        arl_tcp = arl_create("sockstat6/TCP6", arl_callback_str2kernel_uint_t, 60);
        arl_expect(arl_tcp, "inuse",  &sockstat6_root.tcp6_inuse);

        arl_udp = arl_create("sockstat6/UDP6", arl_callback_str2kernel_uint_t, 60);
        arl_expect(arl_udp, "inuse", &sockstat6_root.udp6_inuse);

        arl_udplite = arl_create("sockstat6/UDPLITE6", arl_callback_str2kernel_uint_t, 60);
        arl_expect(arl_udplite, "inuse", &sockstat6_root.udplite6_inuse);

        arl_raw = arl_create("sockstat6/RAW6", arl_callback_str2kernel_uint_t, 60);
        arl_expect(arl_raw, "inuse", &sockstat6_root.raw6_inuse);

        arl_frag = arl_create("sockstat6/FRAG6", arl_callback_str2kernel_uint_t, 60);
        arl_expect(arl_frag, "inuse", &sockstat6_root.frag6_inuse);

        hash_tcp = simple_hash("TCP6");
        hash_udp = simple_hash("UDP6");
        hash_udplite = simple_hash("UDPLITE6");
        hash_raw = simple_hash("RAW6");
        hash_frag = simple_hash("FRAG6");

        keys[0] = "TCP6";     hashes[0] = hash_tcp;     bases[0] = arl_tcp;
        keys[1] = "UDP6";     hashes[1] = hash_udp;     bases[1] = arl_udp;
        keys[2] = "UDPLITE6"; hashes[2] = hash_udplite; bases[2] = arl_udplite;
        keys[3] = "RAW6";     hashes[3] = hash_raw;     bases[3] = arl_raw;
        keys[4] = "FRAG6";    hashes[4] = hash_frag;    bases[4] = arl_frag;
        keys[5] = NULL; // terminator
    }

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/net/sockstat6");
        ff = procfile_open(config_get("plugin:proc:/proc/net/sockstat6", "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 0; // we return 0, so that we will retry to open it next time

    size_t lines = procfile_lines(ff), l;

    for(l = 0; l < lines ;l++) {
        size_t  words = procfile_linewords(ff, l);
        char     *key = procfile_lineword(ff, l, 0);
        uint32_t hash = simple_hash(key);

        int k;
        for(k = 0; keys[k] ; k++) {
            if(unlikely(hash == hashes[k] && strcmp(key, keys[k]) == 0)) {
                // fprintf(stderr, "KEY: '%s', l=%zu, w=1, words=%zu\n", key, l, words);
                ARL_BASE *arl = bases[k];
                arl_begin(arl);
                size_t w = 1;

                while(w + 1 < words) {
                    char *name  = procfile_lineword(ff, l, w); w++;
                    char *value = procfile_lineword(ff, l, w); w++;
                    // fprintf(stderr, " > NAME '%s', VALUE '%s', l=%zu, w=%zu, words=%zu\n", name, value, l, w, words);
                    if(unlikely(arl_check(arl, name, value) != 0))
                        break;
                }

                break;
            }
        }
    }

    // ------------------------------------------------------------------------

    if(do_tcp_sockets == CONFIG_BOOLEAN_YES || (do_tcp_sockets == CONFIG_BOOLEAN_AUTO && (sockstat6_root.tcp6_inuse))) {
        do_tcp_sockets = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        static RRDDIM *rd_inuse = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "ipv6"
                    , "sockstat6_tcp_sockets"
                    , NULL
                    , "tcp6"
                    , NULL
                    , "IPv6 TCP Sockets"
                    , "sockets"
                    , "proc"
                    , "net/sockstat6"
                    , NETDATA_CHART_PRIO_IPV6_TCP
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_inuse    = rrddim_add(st, "inuse",     NULL,   1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_inuse,    (collected_number)sockstat6_root.tcp6_inuse);
        rrdset_done(st);
    }

    // ------------------------------------------------------------------------

    if(do_udp_sockets == CONFIG_BOOLEAN_YES || (do_udp_sockets == CONFIG_BOOLEAN_AUTO && sockstat6_root.udp6_inuse)) {
        do_udp_sockets = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        static RRDDIM *rd_inuse = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "ipv6"
                    , "sockstat6_udp_sockets"
                    , NULL
                    , "udp6"
                    , NULL
                    , "IPv6 UDP Sockets"
                    , "sockets"
                    , "proc"
                    , "net/sockstat6"
                    , NETDATA_CHART_PRIO_IPV6_UDP
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_inuse    = rrddim_add(st, "inuse",     NULL,   1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_inuse,    (collected_number)sockstat6_root.udp6_inuse);
        rrdset_done(st);
    }

    // ------------------------------------------------------------------------

    if(do_udplite_sockets == CONFIG_BOOLEAN_YES || (do_udplite_sockets == CONFIG_BOOLEAN_AUTO && sockstat6_root.udplite6_inuse)) {
        do_udplite_sockets = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        static RRDDIM *rd_inuse = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "ipv6"
                    , "sockstat6_udplite_sockets"
                    , NULL
                    , "udplite6"
                    , NULL
                    , "IPv6 UDPLITE Sockets"
                    , "sockets"
                    , "proc"
                    , "net/sockstat6"
                    , NETDATA_CHART_PRIO_IPV6_UDPLITE
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_inuse    = rrddim_add(st, "inuse",     NULL,   1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_inuse,    (collected_number)sockstat6_root.udplite6_inuse);
        rrdset_done(st);
    }

    // ------------------------------------------------------------------------

    if(do_raw_sockets == CONFIG_BOOLEAN_YES || (do_raw_sockets == CONFIG_BOOLEAN_AUTO && sockstat6_root.raw6_inuse)) {
        do_raw_sockets = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        static RRDDIM *rd_inuse = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "ipv6"
                    , "sockstat6_raw_sockets"
                    , NULL
                    , "raw6"
                    , NULL
                    , "IPv6 RAW Sockets"
                    , "sockets"
                    , "proc"
                    , "net/sockstat6"
                    , NETDATA_CHART_PRIO_IPV6_RAW
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_inuse    = rrddim_add(st, "inuse",     NULL,   1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_inuse,    (collected_number)sockstat6_root.raw6_inuse);
        rrdset_done(st);
    }

    // ------------------------------------------------------------------------

    if(do_frag_sockets == CONFIG_BOOLEAN_YES || (do_frag_sockets == CONFIG_BOOLEAN_AUTO && sockstat6_root.frag6_inuse)) {
        do_frag_sockets = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        static RRDDIM *rd_inuse = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "ipv6"
                    , "sockstat6_frag_sockets"
                    , NULL
                    , "fragments6"
                    , NULL
                    , "IPv6 FRAG Sockets"
                    , "fragments"
                    , "proc"
                    , "net/sockstat6"
                    , NETDATA_CHART_PRIO_IPV6_FRAGMENTS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_inuse    = rrddim_add(st, "inuse",     NULL,   1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_inuse,    (collected_number)sockstat6_root.frag6_inuse);
        rrdset_done(st);
    }

    return 0;
}

// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_macos.h"

#include <Availability.h>
// NEEDED BY: do_bandwidth
#include <net/route.h>
// NEEDED BY do_tcp...
#include <sys/socketvar.h>
#include <netinet/tcp_var.h>
#include <netinet/tcp_fsm.h>
// NEEDED BY do_udp..., do_ip...
#include <netinet/ip_var.h>
// NEEDED BY do_udp...
#include <netinet/udp.h>
#include <netinet/udp_var.h>
// NEEDED BY do_icmp...
#include <netinet/ip.h>
#include <netinet/ip_icmp.h>
#include <netinet/icmp_var.h>
// NEEDED BY do_icmp6...
#include <netinet/icmp6.h>
// NEEDED BY do_uptime
#include <time.h>

// MacOS calculates load averages once every 5 seconds
#define MIN_LOADAVG_UPDATE_EVERY 5

int do_macos_sysctl(int update_every, usec_t dt) {
    static int do_loadavg = -1, do_swap = -1, do_bandwidth = -1,
               do_tcp_packets = -1, do_tcp_errors = -1, do_tcp_handshake = -1, do_ecn = -1,
               do_tcpext_syscookies = -1, do_tcpext_ofo = -1, do_tcpext_connaborts = -1,
               do_udp_packets = -1, do_udp_errors = -1, do_icmp_packets = -1, do_icmpmsg = -1,
               do_ip_packets = -1, do_ip_fragsout = -1, do_ip_fragsin = -1, do_ip_errors = -1,
               do_ip6_packets = -1, do_ip6_fragsout = -1, do_ip6_fragsin = -1, do_ip6_errors = -1,
               do_icmp6 = -1, do_icmp6_redir = -1, do_icmp6_errors = -1, do_icmp6_echos = -1,
               do_icmp6_router = -1, do_icmp6_neighbor = -1, do_icmp6_types = -1, do_uptime = -1;


    if (unlikely(do_loadavg == -1)) {
        do_loadavg              = inicfg_get_boolean(&netdata_config, "plugin:macos:sysctl", "enable load average", 1);
        do_swap                 = inicfg_get_boolean(&netdata_config, "plugin:macos:sysctl", "system swap", 1);
        do_bandwidth            = inicfg_get_boolean(&netdata_config, "plugin:macos:sysctl", "bandwidth", 1);
        do_tcp_packets          = inicfg_get_boolean(&netdata_config, "plugin:macos:sysctl", "ipv4 TCP packets", 1);
        do_tcp_errors           = inicfg_get_boolean(&netdata_config, "plugin:macos:sysctl", "ipv4 TCP errors", 1);
        do_tcp_handshake        = inicfg_get_boolean(&netdata_config, "plugin:macos:sysctl", "ipv4 TCP handshake issues", 1);
        do_ecn                  = inicfg_get_boolean_ondemand(&netdata_config, "plugin:macos:sysctl", "ECN packets", CONFIG_BOOLEAN_AUTO);
        do_tcpext_syscookies    = inicfg_get_boolean_ondemand(&netdata_config, "plugin:macos:sysctl", "TCP SYN cookies", CONFIG_BOOLEAN_AUTO);
        do_tcpext_ofo           = inicfg_get_boolean_ondemand(&netdata_config, "plugin:macos:sysctl", "TCP out-of-order queue", CONFIG_BOOLEAN_AUTO);
        do_tcpext_connaborts    = inicfg_get_boolean_ondemand(&netdata_config, "plugin:macos:sysctl", "TCP connection aborts", CONFIG_BOOLEAN_AUTO);
        do_udp_packets          = inicfg_get_boolean(&netdata_config, "plugin:macos:sysctl", "ipv4 UDP packets", 1);
        do_udp_errors           = inicfg_get_boolean(&netdata_config, "plugin:macos:sysctl", "ipv4 UDP errors", 1);
        do_icmp_packets         = inicfg_get_boolean(&netdata_config, "plugin:macos:sysctl", "ipv4 ICMP packets", 1);
        do_icmpmsg              = inicfg_get_boolean(&netdata_config, "plugin:macos:sysctl", "ipv4 ICMP messages", 1);
        do_ip_packets           = inicfg_get_boolean(&netdata_config, "plugin:macos:sysctl", "ipv4 packets", 1);
        do_ip_fragsout          = inicfg_get_boolean(&netdata_config, "plugin:macos:sysctl", "ipv4 fragments sent", 1);
        do_ip_fragsin           = inicfg_get_boolean(&netdata_config, "plugin:macos:sysctl", "ipv4 fragments assembly", 1);
        do_ip_errors            = inicfg_get_boolean(&netdata_config, "plugin:macos:sysctl", "ipv4 errors", 1);
        do_ip6_packets          = inicfg_get_boolean_ondemand(&netdata_config, "plugin:macos:sysctl", "ipv6 packets", CONFIG_BOOLEAN_AUTO);
        do_ip6_fragsout         = inicfg_get_boolean_ondemand(&netdata_config, "plugin:macos:sysctl", "ipv6 fragments sent", CONFIG_BOOLEAN_AUTO);
        do_ip6_fragsin          = inicfg_get_boolean_ondemand(&netdata_config, "plugin:macos:sysctl", "ipv6 fragments assembly", CONFIG_BOOLEAN_AUTO);
        do_ip6_errors           = inicfg_get_boolean_ondemand(&netdata_config, "plugin:macos:sysctl", "ipv6 errors", CONFIG_BOOLEAN_AUTO);
        do_icmp6                = inicfg_get_boolean_ondemand(&netdata_config, "plugin:macos:sysctl", "icmp", CONFIG_BOOLEAN_AUTO);
        do_icmp6_redir          = inicfg_get_boolean_ondemand(&netdata_config, "plugin:macos:sysctl", "icmp redirects", CONFIG_BOOLEAN_AUTO);
        do_icmp6_errors         = inicfg_get_boolean_ondemand(&netdata_config, "plugin:macos:sysctl", "icmp errors", CONFIG_BOOLEAN_AUTO);
        do_icmp6_echos          = inicfg_get_boolean_ondemand(&netdata_config, "plugin:macos:sysctl", "icmp echos", CONFIG_BOOLEAN_AUTO);
        do_icmp6_router         = inicfg_get_boolean_ondemand(&netdata_config, "plugin:macos:sysctl", "icmp router", CONFIG_BOOLEAN_AUTO);
        do_icmp6_neighbor       = inicfg_get_boolean_ondemand(&netdata_config, "plugin:macos:sysctl", "icmp neighbor", CONFIG_BOOLEAN_AUTO);
        do_icmp6_types          = inicfg_get_boolean_ondemand(&netdata_config, "plugin:macos:sysctl", "icmp types", CONFIG_BOOLEAN_AUTO);
        do_uptime               = inicfg_get_boolean(&netdata_config, "plugin:macos:sysctl", "system uptime", 1);
    }

    RRDSET *st = NULL;

    int i;
    size_t size;

    // NEEDED BY: do_loadavg
    static usec_t next_loadavg_dt = 0;
    struct loadavg sysload;

    // NEEDED BY: do_swap
    struct xsw_usage swap_usage;

    // NEEDED BY: do_bandwidth
    int mib[6];
    static char *ifstatdata = NULL;
    char *lim, *next;
    struct if_msghdr *ifm;
    struct iftot {
        u_long  ift_ibytes;
        u_long  ift_obytes;
    } iftot = {0, 0};

    // NEEDED BY: do_tcp...
    struct tcpstat tcpstat;

    // NEEDED BY: do_udp...
    struct udpstat udpstat;

    // NEEDED BY: do_icmp...
    struct icmpstat icmpstat;
    struct icmp_total {
        u_long  msgs_in;
        u_long  msgs_out;
    } icmp_total = {0, 0};

    // NEEDED BY: do_ip...
    struct ipstat ipstat;

    // NEEDED BY: do_ip6...
    /*
     * Dirty workaround for /usr/include/netinet6/ip6_var.h absence.
     * Struct ip6stat was copied from bsd/netinet6/ip6_var.h from xnu sources.
     * Do the same for previously missing scope6_var.h on OS X < 10.11.
     */
#define	IP6S_SRCRULE_COUNT 16

#if (defined __MAC_OS_X_VERSION_MIN_REQUIRED && __MAC_OS_X_VERSION_MIN_REQUIRED < 101100)
#ifndef _NETINET6_SCOPE6_VAR_H_
#define _NETINET6_SCOPE6_VAR_H_
#include <sys/appleapiopts.h>

#define	SCOPE6_ID_MAX	16
#endif
#else
#include <netinet6/scope6_var.h>
#endif

    struct	ip6stat {
        u_quad_t ip6s_total;		/* total packets received */
        u_quad_t ip6s_tooshort;		/* packet too short */
        u_quad_t ip6s_toosmall;		/* not enough data */
        u_quad_t ip6s_fragments;	/* fragments received */
        u_quad_t ip6s_fragdropped;	/* frags dropped(dups, out of space) */
        u_quad_t ip6s_fragtimeout;	/* fragments timed out */
        u_quad_t ip6s_fragoverflow;	/* fragments that exceeded limit */
        u_quad_t ip6s_forward;		/* packets forwarded */
        u_quad_t ip6s_cantforward;	/* packets rcvd for unreachable dest */
        u_quad_t ip6s_redirectsent;	/* packets forwarded on same net */
        u_quad_t ip6s_delivered;	/* datagrams delivered to upper level */
        u_quad_t ip6s_localout;		/* total ip packets generated here */
        u_quad_t ip6s_odropped;		/* lost packets due to nobufs, etc. */
        u_quad_t ip6s_reassembled;	/* total packets reassembled ok */
        u_quad_t ip6s_atmfrag_rcvd;	/* atomic fragments received */
        u_quad_t ip6s_fragmented;	/* datagrams successfully fragmented */
        u_quad_t ip6s_ofragments;	/* output fragments created */
        u_quad_t ip6s_cantfrag;		/* don't fragment flag was set, etc. */
        u_quad_t ip6s_badoptions;	/* error in option processing */
        u_quad_t ip6s_noroute;		/* packets discarded due to no route */
        u_quad_t ip6s_badvers;		/* ip6 version != 6 */
        u_quad_t ip6s_rawout;		/* total raw ip packets generated */
        u_quad_t ip6s_badscope;		/* scope error */
        u_quad_t ip6s_notmember;	/* don't join this multicast group */
        u_quad_t ip6s_nxthist[256];	/* next header history */
        u_quad_t ip6s_m1;		/* one mbuf */
        u_quad_t ip6s_m2m[32];		/* two or more mbuf */
        u_quad_t ip6s_mext1;		/* one ext mbuf */
        u_quad_t ip6s_mext2m;		/* two or more ext mbuf */
        u_quad_t ip6s_exthdrtoolong;	/* ext hdr are not continuous */
        u_quad_t ip6s_nogif;		/* no match gif found */
        u_quad_t ip6s_toomanyhdr;	/* discarded due to too many headers */

        /*
         * statistics for improvement of the source address selection
         * algorithm:
         */
        /* number of times that address selection fails */
        u_quad_t ip6s_sources_none;
        /* number of times that an address on the outgoing I/F is chosen */
        u_quad_t ip6s_sources_sameif[SCOPE6_ID_MAX];
        /* number of times that an address on a non-outgoing I/F is chosen */
        u_quad_t ip6s_sources_otherif[SCOPE6_ID_MAX];
        /*
         * number of times that an address that has the same scope
         * from the destination is chosen.
         */
        u_quad_t ip6s_sources_samescope[SCOPE6_ID_MAX];
        /*
         * number of times that an address that has a different scope
         * from the destination is chosen.
         */
        u_quad_t ip6s_sources_otherscope[SCOPE6_ID_MAX];
        /* number of times that a deprecated address is chosen */
        u_quad_t ip6s_sources_deprecated[SCOPE6_ID_MAX];

        u_quad_t ip6s_forward_cachehit;
        u_quad_t ip6s_forward_cachemiss;

        /* number of times that each rule of source selection is applied. */
        u_quad_t ip6s_sources_rule[IP6S_SRCRULE_COUNT];

        /* number of times we ignored address on expensive secondary interfaces */
        u_quad_t ip6s_sources_skip_expensive_secondary_if;

        /* pkt dropped, no mbufs for control data */
        u_quad_t ip6s_pktdropcntrl;

        /* total packets trimmed/adjusted  */
        u_quad_t ip6s_adj;
        /* hwcksum info discarded during adjustment */
        u_quad_t ip6s_adj_hwcsum_clr;

        /* duplicate address detection collisions */
        u_quad_t ip6s_dad_collide;

        /* DAD NS looped back */
        u_quad_t ip6s_dad_loopcount;
    } ip6stat;

    // NEEDED BY: do_icmp6...
    struct icmp6stat icmp6stat;
    struct icmp6_total {
        u_long  msgs_in;
        u_long  msgs_out;
    } icmp6_total = {0, 0};

    // NEEDED BY: do_uptime
    struct timespec boot_time, cur_time;

    if (next_loadavg_dt <= dt) {
        if (likely(do_loadavg)) {
            if (unlikely(GETSYSCTL_BY_NAME("vm.loadavg", sysload))) {
                do_loadavg = 0;
                collector_error("DISABLED: system.load");
            } else {

                st = rrdset_find_active_bytype_localhost("system", "load");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "system"
                            , "load"
                            , NULL
                            , "load"
                            , NULL
                            , "System Load Average"
                            , "load"
                            , "macos.plugin"
                            , "sysctl"
                            , 100
                            , (update_every < MIN_LOADAVG_UPDATE_EVERY) ? MIN_LOADAVG_UPDATE_EVERY : update_every
                            , RRDSET_TYPE_LINE
                    );
                    rrddim_add(st, "load1", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
                    rrddim_add(st, "load5", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
                    rrddim_add(st, "load15", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
                }

                rrddim_set(st, "load1", (collected_number) ((double)sysload.ldavg[0] / sysload.fscale * 1000));
                rrddim_set(st, "load5", (collected_number) ((double)sysload.ldavg[1] / sysload.fscale * 1000));
                rrddim_set(st, "load15", (collected_number) ((double)sysload.ldavg[2] / sysload.fscale * 1000));
                rrdset_done(st);
            }
        }

        next_loadavg_dt = st->update_every * USEC_PER_SEC;
    }
    else next_loadavg_dt -= dt;

    if (likely(do_swap)) {
        if (unlikely(GETSYSCTL_BY_NAME("vm.swapusage", swap_usage))) {
            do_swap = 0;
            collector_error("DISABLED: mem.swap");
        } else {
            st = rrdset_find_active_localhost("mem.swap");
            if (unlikely(!st)) {
                st = rrdset_create_localhost(
                        "mem"
                        , "swap"
                        , NULL
                        , "swap"
                        , NULL
                        , "System Swap"
                        , "MiB"
                        , "macos.plugin"
                        , "sysctl"
                        , 201
                        , update_every
                        , RRDSET_TYPE_STACKED
                );

                rrddim_add(st, "free",    NULL, 1, 1048576, RRD_ALGORITHM_ABSOLUTE);
                rrddim_add(st, "used",    NULL, 1, 1048576, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set(st, "free", swap_usage.xsu_avail);
            rrddim_set(st, "used", swap_usage.xsu_used);
            rrdset_done(st);
        }
    }

    if (likely(do_bandwidth)) {
        mib[0] = CTL_NET;
        mib[1] = PF_ROUTE;
        mib[2] = 0;
        mib[3] = AF_INET;
        mib[4] = NET_RT_IFLIST2;
        mib[5] = 0;
        if (unlikely(sysctl(mib, 6, NULL, &size, NULL, 0))) {
            collector_error("MACOS: sysctl(%s...) failed: %s", "net interfaces", strerror(errno));
            do_bandwidth = 0;
            collector_error("DISABLED: system.ipv4");
        } else {
            ifstatdata = reallocz(ifstatdata, size);
            if (unlikely(sysctl(mib, 6, ifstatdata, &size, NULL, 0) < 0)) {
                collector_error("MACOS: sysctl(%s...) failed: %s", "net interfaces", strerror(errno));
                do_bandwidth = 0;
                collector_error("DISABLED: system.ipv4");
            } else {
                lim = ifstatdata + size;
                iftot.ift_ibytes = iftot.ift_obytes = 0;
                for (next = ifstatdata; next < lim; ) {
                    ifm = (struct if_msghdr *)next;
                    next += ifm->ifm_msglen;

                    if (ifm->ifm_type == RTM_IFINFO2) {
                        struct if_msghdr2 *if2m = (struct if_msghdr2 *)ifm;

                        iftot.ift_ibytes += if2m->ifm_data.ifi_ibytes;
                        iftot.ift_obytes += if2m->ifm_data.ifi_obytes;
                    }
                }
                st = rrdset_find_active_localhost("system.ipv4");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "system"
                            , "ipv4"
                            , NULL
                            , "network"
                            , NULL
                            , "IPv4 Bandwidth"
                            , "kilobits/s"
                            , "macos.plugin"
                            , "sysctl"
                            , 500
                            , update_every
                            , RRDSET_TYPE_AREA
                    );

                    rrddim_add(st, "InOctets",  "received", 8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "OutOctets", "sent",    -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set(st, "InOctets", iftot.ift_ibytes);
                rrddim_set(st, "OutOctets", iftot.ift_obytes);
                rrdset_done(st);
            }
        }
    }

    // see http://net-snmp.sourceforge.net/docs/mibs/tcp.html
    if (likely(do_tcp_packets || do_tcp_errors || do_tcp_handshake || do_tcpext_connaborts || do_tcpext_ofo || do_tcpext_syscookies || do_ecn)) {
        if (unlikely(GETSYSCTL_BY_NAME("net.inet.tcp.stats", tcpstat))){
            do_tcp_packets = 0;
            collector_error("DISABLED: ipv4.tcppackets");
            do_tcp_errors = 0;
            collector_error("DISABLED: ipv4.tcperrors");
            do_tcp_handshake = 0;
            collector_error("DISABLED: ipv4.tcphandshake");
            do_tcpext_connaborts = 0;
            collector_error("DISABLED: ipv4.tcpconnaborts");
            do_tcpext_ofo = 0;
            collector_error("DISABLED: ipv4.tcpofo");
            do_tcpext_syscookies = 0;
            collector_error("DISABLED: ipv4.tcpsyncookies");
            do_ecn = 0;
            collector_error("DISABLED: ipv4.ecnpkts");
        } else {
            if (likely(do_tcp_packets)) {
                st = rrdset_find_active_localhost("ipv4.tcppackets");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "ipv4"
                            , "tcppackets"
                            , NULL
                            , "tcp"
                            , NULL
                            , "IPv4 TCP Packets"
                            , "packets/s"
                            , "macos.plugin"
                            , "sysctl"
                            , 2600
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "InSegs", "received", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "OutSegs", "sent", -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set(st, "InSegs", tcpstat.tcps_rcvtotal);
                rrddim_set(st, "OutSegs", tcpstat.tcps_sndtotal);
                rrdset_done(st);
            }

            if (likely(do_tcp_errors)) {
                st = rrdset_find_active_localhost("ipv4.tcperrors");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "ipv4"
                            , "tcperrors"
                            , NULL
                            , "tcp"
                            , NULL
                            , "IPv4 TCP Errors"
                            , "packets/s"
                            , "macos.plugin"
                            , "sysctl"
                            , 2700
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "InErrs", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "InCsumErrors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "RetransSegs", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set(st, "InErrs", tcpstat.tcps_rcvbadoff + tcpstat.tcps_rcvshort);
                rrddim_set(st, "InCsumErrors", tcpstat.tcps_rcvbadsum);
                rrddim_set(st, "RetransSegs", tcpstat.tcps_sndrexmitpack);
                rrdset_done(st);
            }

            if (likely(do_tcp_handshake)) {
                st = rrdset_find_active_localhost("ipv4.tcphandshake");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "ipv4"
                            , "tcphandshake"
                            , NULL
                            , "tcp"
                            , NULL
                            , "IPv4 TCP Handshake Issues"
                            , "events/s"
                            , "macos.plugin"
                            , "sysctl"
                            , 2900
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "EstabResets", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "ActiveOpens", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "PassiveOpens", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "AttemptFails", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set(st, "EstabResets", tcpstat.tcps_drops);
                rrddim_set(st, "ActiveOpens", tcpstat.tcps_connattempt);
                rrddim_set(st, "PassiveOpens", tcpstat.tcps_accepts);
                rrddim_set(st, "AttemptFails", tcpstat.tcps_conndrops);
                rrdset_done(st);
            }

            if (do_tcpext_connaborts == CONFIG_BOOLEAN_YES || do_tcpext_connaborts == CONFIG_BOOLEAN_AUTO) {
                do_tcpext_connaborts = CONFIG_BOOLEAN_YES;
                st = rrdset_find_active_localhost("ipv4.tcpconnaborts");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "ipv4"
                            , "tcpconnaborts"
                            , NULL
                            , "tcp"
                            , NULL
                            , "TCP Connection Aborts"
                            , "connections/s"
                            , "macos.plugin"
                            , "sysctl"
                            , 3010
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "TCPAbortOnData",    "baddata",     1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "TCPAbortOnClose",   "userclosed",  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "TCPAbortOnMemory",  "nomemory",    1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "TCPAbortOnTimeout", "timeout",     1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set(st, "TCPAbortOnData",    tcpstat.tcps_rcvpackafterwin);
                rrddim_set(st, "TCPAbortOnClose",   tcpstat.tcps_rcvafterclose);
                rrddim_set(st, "TCPAbortOnMemory",  tcpstat.tcps_rcvmemdrop);
                rrddim_set(st, "TCPAbortOnTimeout", tcpstat.tcps_persistdrop);
                rrdset_done(st);
            }

            if (do_tcpext_ofo == CONFIG_BOOLEAN_YES || do_tcpext_ofo == CONFIG_BOOLEAN_AUTO) {
                do_tcpext_ofo = CONFIG_BOOLEAN_YES;
                st = rrdset_find_active_localhost("ipv4.tcpofo");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "ipv4"
                            , "tcpofo"
                            , NULL
                            , "tcp"
                            , NULL
                            , "TCP Out-Of-Order Queue"
                            , "packets/s"
                            , "macos.plugin"
                            , "sysctl"
                            , 3050
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "TCPOFOQueue", "inqueue",  1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set(st, "TCPOFOQueue",   tcpstat.tcps_rcvoopack);
                rrdset_done(st);
            }

            if (do_tcpext_syscookies == CONFIG_BOOLEAN_YES || do_tcpext_syscookies == CONFIG_BOOLEAN_AUTO) {
                do_tcpext_syscookies = CONFIG_BOOLEAN_YES;

                st = rrdset_find_active_localhost("ipv4.tcpsyncookies");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "ipv4"
                            , "tcpsyncookies"
                            , NULL
                            , "tcp"
                            , NULL
                            , "TCP SYN Cookies"
                            , "packets/s"
                            , "macos.plugin"
                            , "sysctl"
                            , 3100
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "SyncookiesRecv",   "received",  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "SyncookiesSent",   "sent",     -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "SyncookiesFailed", "failed",   -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set(st, "SyncookiesRecv",   tcpstat.tcps_sc_recvcookie);
                rrddim_set(st, "SyncookiesSent",   tcpstat.tcps_sc_sendcookie);
                rrddim_set(st, "SyncookiesFailed", tcpstat.tcps_sc_zonefail);
                rrdset_done(st);
            }

#if (defined __MAC_OS_X_VERSION_MIN_REQUIRED && __MAC_OS_X_VERSION_MIN_REQUIRED >= 101100)
            if (do_ecn == CONFIG_BOOLEAN_YES || do_ecn == CONFIG_BOOLEAN_AUTO) {
                do_ecn = CONFIG_BOOLEAN_YES;
                st = rrdset_find_active_localhost("ipv4.ecnpkts");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "ipv4"
                            , "ecnpkts"
                            , NULL
                            , "ecn"
                            , NULL
                            , "IPv4 ECN Statistics"
                            , "packets/s"
                            , "macos.plugin"
                            , "sysctl"
                            , 8700
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "InCEPkts", "CEP", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "InNoECTPkts", "NoECTP", -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set(st, "InCEPkts", tcpstat.tcps_ecn_recv_ce);
                rrddim_set(st, "InNoECTPkts", tcpstat.tcps_ecn_not_supported);
                rrdset_done(st);
            }
#endif

        }
    }

    // see http://net-snmp.sourceforge.net/docs/mibs/udp.html
    if (likely(do_udp_packets || do_udp_errors)) {
        if (unlikely(GETSYSCTL_BY_NAME("net.inet.udp.stats", udpstat))) {
            do_udp_packets = 0;
            collector_error("DISABLED: ipv4.udppackets");
            do_udp_errors = 0;
            collector_error("DISABLED: ipv4.udperrors");
        } else {
            if (likely(do_udp_packets)) {
                st = rrdset_find_active_localhost("ipv4.udppackets");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "ipv4"
                            , "udppackets"
                            , NULL
                            , "udp"
                            , NULL
                            , "IPv4 UDP Packets"
                            , "packets/s"
                            , "macos.plugin"
                            , "sysctl"
                            , 2601
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "InDatagrams", "received", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "OutDatagrams", "sent", -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set(st, "InDatagrams", udpstat.udps_ipackets);
                rrddim_set(st, "OutDatagrams", udpstat.udps_opackets);
                rrdset_done(st);
            }

            if (likely(do_udp_errors)) {
                st = rrdset_find_active_localhost("ipv4.udperrors");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "ipv4"
                            , "udperrors"
                            , NULL
                            , "udp"
                            , NULL
                            , "IPv4 UDP Errors"
                            , "events/s"
                            , "macos.plugin"
                            , "sysctl"
                            , 2701
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "RcvbufErrors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "InErrors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "NoPorts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "InCsumErrors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
#if (defined __MAC_OS_X_VERSION_MIN_REQUIRED && __MAC_OS_X_VERSION_MIN_REQUIRED >= 1090)
                    rrddim_add(st, "IgnoredMulti", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
#endif
                }

                rrddim_set(st, "InErrors", udpstat.udps_hdrops + udpstat.udps_badlen);
                rrddim_set(st, "NoPorts", udpstat.udps_noport);
                rrddim_set(st, "RcvbufErrors", udpstat.udps_fullsock);
#if (defined __MAC_OS_X_VERSION_MIN_REQUIRED && __MAC_OS_X_VERSION_MIN_REQUIRED >= 1090)
                rrddim_set(st, "InCsumErrors", udpstat.udps_badsum + udpstat.udps_nosum);
                rrddim_set(st, "IgnoredMulti", udpstat.udps_filtermcast);
#else
                rrddim_set(st, "InCsumErrors", udpstat.udps_badsum);
#endif
                rrdset_done(st);
            }
        }
    }

    if (likely(do_icmp_packets || do_icmpmsg)) {
        if (unlikely(GETSYSCTL_BY_NAME("net.inet.icmp.stats", icmpstat))) {
            do_icmp_packets = 0;
            collector_error("DISABLED: ipv4.icmp");
            collector_error("DISABLED: ipv4.icmp_errors");
            do_icmpmsg = 0;
            collector_error("DISABLED: ipv4.icmpmsg");
        } else {
            for (i = 0; i <= ICMP_MAXTYPE; i++) {
                icmp_total.msgs_in += icmpstat.icps_inhist[i];
                icmp_total.msgs_out += icmpstat.icps_outhist[i];
            }
            icmp_total.msgs_in += icmpstat.icps_badcode + icmpstat.icps_badlen + icmpstat.icps_checksum + icmpstat.icps_tooshort;

            // --------------------------------------------------------------------

            if (likely(do_icmp_packets)) {
                st = rrdset_find_active_localhost("ipv4.icmp");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "ipv4"
                            , "icmp"
                            , NULL
                            , "icmp"
                            , NULL
                            , "IPv4 ICMP Packets"
                            , "packets/s"
                            , "macos.plugin"
                            , "sysctl"
                            , 2602
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "InMsgs", "received", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "OutMsgs", "sent", -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set(st, "InMsgs", icmp_total.msgs_in);
                rrddim_set(st, "OutMsgs", icmp_total.msgs_out);
                rrdset_done(st);

                st = rrdset_find_active_localhost("ipv4.icmp_errors");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "ipv4"
                            , "icmp_errors"
                            , NULL
                            , "icmp"
                            , NULL
                            , "IPv4 ICMP Errors"
                            , "packets/s"
                            , "macos.plugin"
                            , "sysctl"
                            , 2603
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "InErrors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "OutErrors", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "InCsumErrors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set(st, "InErrors", icmpstat.icps_badcode + icmpstat.icps_badlen + icmpstat.icps_checksum + icmpstat.icps_tooshort);
                rrddim_set(st, "OutErrors", icmpstat.icps_error);
                rrddim_set(st, "InCsumErrors", icmpstat.icps_checksum);
                rrdset_done(st);
            }

            if (likely(do_icmpmsg)) {
                st = rrdset_find_active_localhost("ipv4.icmpmsg");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "ipv4"
                            , "icmpmsg"
                            , NULL
                            , "icmp"
                            , NULL
                            , "IPv4 ICMP Messages"
                            , "packets/s"
                            , "macos.plugin"
                            , "sysctl"
                            , 2604
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "InEchoReps", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "OutEchoReps", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "InEchos", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "OutEchos", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set(st, "InEchoReps", icmpstat.icps_inhist[ICMP_ECHOREPLY]);
                rrddim_set(st, "OutEchoReps", icmpstat.icps_outhist[ICMP_ECHOREPLY]);
                rrddim_set(st, "InEchos", icmpstat.icps_inhist[ICMP_ECHO]);
                rrddim_set(st, "OutEchos", icmpstat.icps_outhist[ICMP_ECHO]);
                rrdset_done(st);
            }
        }
    }

    // see also http://net-snmp.sourceforge.net/docs/mibs/ip.html
    if (likely(do_ip_packets || do_ip_fragsout || do_ip_fragsin || do_ip_errors)) {
        if (unlikely(GETSYSCTL_BY_NAME("net.inet.ip.stats", ipstat))) {
            do_ip_packets = 0;
            collector_error("DISABLED: ipv4.packets");
            do_ip_fragsout = 0;
            collector_error("DISABLED: ipv4.fragsout");
            do_ip_fragsin = 0;
            collector_error("DISABLED: ipv4.fragsin");
            do_ip_errors = 0;
            collector_error("DISABLED: ipv4.errors");
        } else {
            if (likely(do_ip_packets)) {
                st = rrdset_find_active_localhost("ipv4.packets");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "ipv4"
                            , "packets"
                            , NULL
                            , "packets"
                            , NULL
                            , "IPv4 Packets"
                            , "packets/s"
                            , "macos.plugin"
                            , "sysctl"
                            , 3000
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "InReceives", "received", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "OutRequests", "sent", -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "ForwDatagrams", "forwarded", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "InDelivers", "delivered", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set(st, "OutRequests", ipstat.ips_localout);
                rrddim_set(st, "InReceives", ipstat.ips_total);
                rrddim_set(st, "ForwDatagrams", ipstat.ips_forward);
                rrddim_set(st, "InDelivers", ipstat.ips_delivered);
                rrdset_done(st);
            }

            if (likely(do_ip_fragsout)) {
                st = rrdset_find_active_localhost("ipv4.fragsout");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "ipv4"
                            , "fragsout"
                            , NULL
                            , "fragments"
                            , NULL
                            , "IPv4 Fragments Sent"
                            , "packets/s"
                            , "macos.plugin"
                            , "sysctl"
                            , 3010
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "FragOKs", "ok", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "FragFails", "failed", -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "FragCreates", "created", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set(st, "FragOKs", ipstat.ips_fragmented);
                rrddim_set(st, "FragFails", ipstat.ips_cantfrag);
                rrddim_set(st, "FragCreates", ipstat.ips_ofragments);
                rrdset_done(st);
            }

            if (likely(do_ip_fragsin)) {
                st = rrdset_find_active_localhost("ipv4.fragsin");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "ipv4"
                            , "fragsin"
                            , NULL
                            , "fragments"
                            , NULL
                            , "IPv4 Fragments Reassembly"
                            , "packets/s"
                            , "macos.plugin"
                            , "sysctl"
                            , 3011
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "ReasmOKs", "ok", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "ReasmFails", "failed", -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "ReasmReqds", "all", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set(st, "ReasmOKs", ipstat.ips_fragments);
                rrddim_set(st, "ReasmFails", ipstat.ips_fragdropped);
                rrddim_set(st, "ReasmReqds", ipstat.ips_reassembled);
                rrdset_done(st);
            }

            if (likely(do_ip_errors)) {
                st = rrdset_find_active_localhost("ipv4.errors");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "ipv4"
                            , "errors"
                            , NULL
                            , "errors"
                            , NULL
                            , "IPv4 Errors"
                            , "packets/s"
                            , "macos.plugin"
                            , "sysctl"
                            , 3002
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "InDiscards", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "OutDiscards", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

                    rrddim_add(st, "InHdrErrors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "OutNoRoutes", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

                    rrddim_add(st, "InAddrErrors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "InUnknownProtos", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set(st, "InDiscards", ipstat.ips_badsum + ipstat.ips_tooshort + ipstat.ips_toosmall + ipstat.ips_toolong);
                rrddim_set(st, "OutDiscards", ipstat.ips_odropped);
                rrddim_set(st, "InHdrErrors", ipstat.ips_badhlen + ipstat.ips_badlen + ipstat.ips_badoptions + ipstat.ips_badvers);
                rrddim_set(st, "InAddrErrors", ipstat.ips_badaddr);
                rrddim_set(st, "InUnknownProtos", ipstat.ips_noproto);
                rrddim_set(st, "OutNoRoutes", ipstat.ips_noroute);
                rrdset_done(st);
            }
        }
    }

    if (likely(do_ip6_packets || do_ip6_fragsout || do_ip6_fragsin || do_ip6_errors)) {
        if (unlikely(GETSYSCTL_BY_NAME("net.inet6.ip6.stats", ip6stat))) {
            do_ip6_packets = 0;
            collector_error("DISABLED: ipv6.packets");
            do_ip6_fragsout = 0;
            collector_error("DISABLED: ipv6.fragsout");
            do_ip6_fragsin = 0;
            collector_error("DISABLED: ipv6.fragsin");
            do_ip6_errors = 0;
            collector_error("DISABLED: ipv6.errors");
        } else {
            if (do_ip6_packets == CONFIG_BOOLEAN_YES || do_ip6_packets == CONFIG_BOOLEAN_AUTO) {
                do_ip6_packets = CONFIG_BOOLEAN_YES;
                st = rrdset_find_active_localhost("ipv6.packets");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "ipv6"
                            , "packets"
                            , NULL
                            , "packets"
                            , NULL
                            , "IPv6 Packets"
                            , "packets/s"
                            , "macos.plugin"
                            , "sysctl"
                            , 3000
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "sent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "forwarded", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "delivers", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set(st, "sent", ip6stat.ip6s_localout);
                rrddim_set(st, "received", ip6stat.ip6s_total);
                rrddim_set(st, "forwarded", ip6stat.ip6s_forward);
                rrddim_set(st, "delivers", ip6stat.ip6s_delivered);
                rrdset_done(st);
            }

            if (do_ip6_fragsout == CONFIG_BOOLEAN_YES || do_ip6_fragsout == CONFIG_BOOLEAN_AUTO) {
                do_ip6_fragsout = CONFIG_BOOLEAN_YES;
                st = rrdset_find_active_localhost("ipv6.fragsout");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "ipv6"
                            , "fragsout"
                            , NULL
                            , "fragments"
                            , NULL
                            , "IPv6 Fragments Sent"
                            , "packets/s"
                            , "macos.plugin"
                            , "sysctl"
                            , 3010
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "ok", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "failed", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "all", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set(st, "ok", ip6stat.ip6s_fragmented);
                rrddim_set(st, "failed", ip6stat.ip6s_cantfrag);
                rrddim_set(st, "all", ip6stat.ip6s_ofragments);
                rrdset_done(st);
            }

            if (do_ip6_fragsin == CONFIG_BOOLEAN_YES || do_ip6_fragsin == CONFIG_BOOLEAN_AUTO) {
                do_ip6_fragsin = CONFIG_BOOLEAN_YES;
                st = rrdset_find_active_localhost("ipv6.fragsin");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "ipv6"
                            , "fragsin"
                            , NULL
                            , "fragments"
                            , NULL
                            , "IPv6 Fragments Reassembly"
                            , "packets/s"
                            , "macos.plugin"
                            , "sysctl"
                            , 3011
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "ok", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "failed", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "timeout", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "all", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set(st, "ok", ip6stat.ip6s_reassembled);
                rrddim_set(st, "failed", ip6stat.ip6s_fragdropped);
                rrddim_set(st, "timeout", ip6stat.ip6s_fragtimeout);
                rrddim_set(st, "all", ip6stat.ip6s_fragments);
                rrdset_done(st);
            }

            if (do_ip6_errors == CONFIG_BOOLEAN_YES || do_ip6_errors == CONFIG_BOOLEAN_AUTO) {
                do_ip6_errors = CONFIG_BOOLEAN_YES;
                st = rrdset_find_active_localhost("ipv6.errors");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "ipv6"
                            , "errors"
                            , NULL
                            , "errors"
                            , NULL
                            , "IPv6 Errors"
                            , "packets/s"
                            , "macos.plugin"
                            , "sysctl"
                            , 3002
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "InDiscards", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "OutDiscards", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

                    rrddim_add(st, "InHdrErrors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "InAddrErrors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "InTruncatedPkts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "InNoRoutes", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

                    rrddim_add(st, "OutNoRoutes", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set(st, "InDiscards", ip6stat.ip6s_toosmall);
                rrddim_set(st, "OutDiscards", ip6stat.ip6s_odropped);

                rrddim_set(st, "InHdrErrors",
                           ip6stat.ip6s_badoptions + ip6stat.ip6s_badvers + ip6stat.ip6s_exthdrtoolong);
                rrddim_set(st, "InAddrErrors", ip6stat.ip6s_sources_none);
                rrddim_set(st, "InTruncatedPkts", ip6stat.ip6s_tooshort);
                rrddim_set(st, "InNoRoutes", ip6stat.ip6s_cantforward);

                rrddim_set(st, "OutNoRoutes", ip6stat.ip6s_noroute);
                rrdset_done(st);
            }
        }
    }

    if (likely(do_icmp6 || do_icmp6_redir || do_icmp6_errors || do_icmp6_echos || do_icmp6_router || do_icmp6_neighbor || do_icmp6_types)) {
        if (unlikely(GETSYSCTL_BY_NAME("net.inet6.icmp6.stats", icmp6stat))) {
            do_icmp6 = 0;
            collector_error("DISABLED: ipv6.icmp");
        } else {
            for (i = 0; i <= ICMP6_MAXTYPE; i++) {
                icmp6_total.msgs_in += icmp6stat.icp6s_inhist[i];
                icmp6_total.msgs_out += icmp6stat.icp6s_outhist[i];
            }
            icmp6_total.msgs_in += icmp6stat.icp6s_badcode + icmp6stat.icp6s_badlen + icmp6stat.icp6s_checksum + icmp6stat.icp6s_tooshort;

            if (do_icmp6 == CONFIG_BOOLEAN_YES || do_icmp6 == CONFIG_BOOLEAN_AUTO) {
                do_icmp6 = CONFIG_BOOLEAN_YES;
                st = rrdset_find_active_localhost("ipv6.icmp");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "ipv6"
                            , "icmp"
                            , NULL
                            , "icmp"
                            , NULL
                            , "IPv6 ICMP Messages"
                            , "messages/s"
                            , "macos.plugin"
                            , "sysctl"
                            , 10000
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "sent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set(st, "sent", icmp6_total.msgs_in);
                rrddim_set(st, "received", icmp6_total.msgs_out);
                rrdset_done(st);
            }

            if (do_icmp6_redir == CONFIG_BOOLEAN_YES || do_icmp6_redir == CONFIG_BOOLEAN_AUTO) {
                do_icmp6_redir = CONFIG_BOOLEAN_YES;
                st = rrdset_find_active_localhost("ipv6.icmpredir");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "ipv6"
                            , "icmpredir"
                            , NULL
                            , "icmp"
                            , NULL
                            , "IPv6 ICMP Redirects"
                            , "redirects/s"
                            , "macos.plugin"
                            , "sysctl"
                            , 10050
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "sent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set(st, "sent", icmp6stat.icp6s_inhist[ND_REDIRECT]);
                rrddim_set(st, "received", icmp6stat.icp6s_outhist[ND_REDIRECT]);
                rrdset_done(st);
            }

            if (do_icmp6_errors == CONFIG_BOOLEAN_YES || do_icmp6_errors == CONFIG_BOOLEAN_AUTO) {
                do_icmp6_errors = CONFIG_BOOLEAN_YES;
                st = rrdset_find_active_localhost("ipv6.icmperrors");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "ipv6"
                            , "icmperrors"
                            , NULL
                            , "icmp"
                            , NULL
                            , "IPv6 ICMP Errors"
                            , "errors/s"
                            , "macos.plugin"
                            , "sysctl"
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
                    rrddim_add(st, "OutTimeExcds", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "OutParmProblems", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set(st, "InErrors", icmp6stat.icp6s_badcode + icmp6stat.icp6s_badlen + icmp6stat.icp6s_checksum + icmp6stat.icp6s_tooshort);
                rrddim_set(st, "OutErrors", icmp6stat.icp6s_error);
                rrddim_set(st, "InCsumErrors", icmp6stat.icp6s_checksum);
                rrddim_set(st, "InDestUnreachs", icmp6stat.icp6s_inhist[ICMP6_DST_UNREACH]);
                rrddim_set(st, "InPktTooBigs", icmp6stat.icp6s_badlen);
                rrddim_set(st, "InTimeExcds", icmp6stat.icp6s_inhist[ICMP6_TIME_EXCEEDED]);
                rrddim_set(st, "InParmProblems", icmp6stat.icp6s_inhist[ICMP6_PARAM_PROB]);
                rrddim_set(st, "OutDestUnreachs", icmp6stat.icp6s_outhist[ICMP6_DST_UNREACH]);
                rrddim_set(st, "OutTimeExcds", icmp6stat.icp6s_outhist[ICMP6_TIME_EXCEEDED]);
                rrddim_set(st, "OutParmProblems", icmp6stat.icp6s_outhist[ICMP6_PARAM_PROB]);
                rrdset_done(st);
            }

            if (do_icmp6_echos == CONFIG_BOOLEAN_YES || do_icmp6_echos == CONFIG_BOOLEAN_AUTO) {
                do_icmp6_echos = CONFIG_BOOLEAN_YES;
                st = rrdset_find_active_localhost("ipv6.icmpechos");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "ipv6"
                            , "icmpechos"
                            , NULL
                            , "icmp"
                            , NULL
                            , "IPv6 ICMP Echo"
                            , "messages/s"
                            , "macos.plugin"
                            , "sysctl"
                            , 10200
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "InEchos", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "OutEchos", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "InEchoReplies", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "OutEchoReplies", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set(st, "InEchos", icmp6stat.icp6s_inhist[ICMP6_ECHO_REQUEST]);
                rrddim_set(st, "OutEchos", icmp6stat.icp6s_outhist[ICMP6_ECHO_REQUEST]);
                rrddim_set(st, "InEchoReplies", icmp6stat.icp6s_inhist[ICMP6_ECHO_REPLY]);
                rrddim_set(st, "OutEchoReplies", icmp6stat.icp6s_outhist[ICMP6_ECHO_REPLY]);
                rrdset_done(st);
            }

            if (do_icmp6_router == CONFIG_BOOLEAN_YES || do_icmp6_router == CONFIG_BOOLEAN_AUTO) {
                do_icmp6_router = CONFIG_BOOLEAN_YES;
                st = rrdset_find_active_localhost("ipv6.icmprouter");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "ipv6"
                            , "icmprouter"
                            , NULL
                            , "icmp"
                            , NULL
                            , "IPv6 Router Messages"
                            , "messages/s"
                            , "macos.plugin"
                            , "sysctl"
                            , 10400
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "InSolicits", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "OutSolicits", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "InAdvertisements", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "OutAdvertisements", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set(st, "InSolicits", icmp6stat.icp6s_inhist[ND_ROUTER_SOLICIT]);
                rrddim_set(st, "OutSolicits", icmp6stat.icp6s_outhist[ND_ROUTER_SOLICIT]);
                rrddim_set(st, "InAdvertisements", icmp6stat.icp6s_inhist[ND_ROUTER_ADVERT]);
                rrddim_set(st, "OutAdvertisements", icmp6stat.icp6s_outhist[ND_ROUTER_ADVERT]);
                rrdset_done(st);
            }

            if (do_icmp6_neighbor == CONFIG_BOOLEAN_YES || do_icmp6_neighbor == CONFIG_BOOLEAN_AUTO) {
                do_icmp6_neighbor = CONFIG_BOOLEAN_YES;
                st = rrdset_find_active_localhost("ipv6.icmpneighbor");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "ipv6"
                            , "icmpneighbor"
                            , NULL
                            , "icmp"
                            , NULL
                            , "IPv6 Neighbor Messages"
                            , "messages/s"
                            , "macos.plugin"
                            , "sysctl"
                            , 10500
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrddim_add(st, "InSolicits", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "OutSolicits", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "InAdvertisements", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_add(st, "OutAdvertisements", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set(st, "InSolicits", icmp6stat.icp6s_inhist[ND_NEIGHBOR_SOLICIT]);
                rrddim_set(st, "OutSolicits", icmp6stat.icp6s_outhist[ND_NEIGHBOR_SOLICIT]);
                rrddim_set(st, "InAdvertisements", icmp6stat.icp6s_inhist[ND_NEIGHBOR_ADVERT]);
                rrddim_set(st, "OutAdvertisements", icmp6stat.icp6s_outhist[ND_NEIGHBOR_ADVERT]);
            }

            if (do_icmp6_types == CONFIG_BOOLEAN_YES || do_icmp6_types == CONFIG_BOOLEAN_AUTO) {
                do_icmp6_types = CONFIG_BOOLEAN_YES;
                st = rrdset_find_active_localhost("ipv6.icmptypes");
                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                            "ipv6"
                            , "icmptypes"
                            , NULL
                            , "icmp"
                            , NULL
                            , "IPv6 ICMP Types"
                            , "messages/s"
                            , "macos.plugin"
                            , "sysctl"
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

                rrddim_set(st, "InType1", icmp6stat.icp6s_inhist[1]);
                rrddim_set(st, "InType128", icmp6stat.icp6s_inhist[128]);
                rrddim_set(st, "InType129", icmp6stat.icp6s_inhist[129]);
                rrddim_set(st, "InType136", icmp6stat.icp6s_inhist[136]);
                rrddim_set(st, "OutType1", icmp6stat.icp6s_outhist[1]);
                rrddim_set(st, "OutType128", icmp6stat.icp6s_outhist[128]);
                rrddim_set(st, "OutType129", icmp6stat.icp6s_outhist[129]);
                rrddim_set(st, "OutType133", icmp6stat.icp6s_outhist[133]);
                rrddim_set(st, "OutType135", icmp6stat.icp6s_outhist[135]);
                rrddim_set(st, "OutType143", icmp6stat.icp6s_outhist[143]);
                rrdset_done(st);
            }
        }
    }

    if (likely(do_uptime)) {
        if (unlikely(GETSYSCTL_BY_NAME("kern.boottime", boot_time))) {
            do_uptime = 0;
            collector_error("DISABLED: system.uptime");
        } else {
            clock_gettime(CLOCK_REALTIME, &cur_time);
            st = rrdset_find_active_localhost("system.uptime");

            if(unlikely(!st)) {
                st = rrdset_create_localhost(
                        "system"
                        , "uptime"
                        , NULL
                        , "uptime"
                        , NULL
                        , "System Uptime"
                        , "seconds"
                        , "macos.plugin"
                        , "sysctl"
                        , 1000
                        , update_every
                        , RRDSET_TYPE_LINE
                );
                rrddim_add(st, "uptime", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set(st, "uptime", cur_time.tv_sec - boot_time.tv_sec);
            rrdset_done(st);
        }
    }

    return 0;
}

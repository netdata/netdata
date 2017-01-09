#include "common.h"
#include <sys/sysctl.h>
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

#define GETSYSCTL(name, var) getsysctl(name, &(var), sizeof(var))

// MacOS calculates load averages once every 5 seconds
#define MIN_LOADAVG_UPDATE_EVERY 5

int getsysctl(const char *name, void *ptr, size_t len);

int do_macos_sysctl(int update_every, usec_t dt) {
    (void)dt;

    static int do_loadavg = -1, do_swap = -1, do_bandwidth = -1,
               do_tcp_packets = -1, do_tcp_errors = -1, do_tcp_handshake = -1, do_ecn = -1,
               do_tcpext_syscookies = -1, do_tcpext_ofo = -1, do_tcpext_connaborts = -1,
               do_udp_packets = -1, do_udp_errors = -1, do_icmp_packets = -1, do_icmpmsg = -1;


    if (unlikely(do_loadavg == -1)) {
        do_loadavg              = config_get_boolean("plugin:macos:sysctl", "enable load average", 1);
        do_swap                 = config_get_boolean("plugin:macos:sysctl", "system swap", 1);
        do_bandwidth            = config_get_boolean("plugin:macos:sysctl", "bandwidth", 1);
        do_tcp_packets          = config_get_boolean("plugin:macos:sysctl", "ipv4 TCP packets", 1);
        do_tcp_errors           = config_get_boolean("plugin:macos:sysctl", "ipv4 TCP errors", 1);
        do_tcp_handshake        = config_get_boolean("plugin:macos:sysctl", "ipv4 TCP handshake issues", 1);
        do_ecn                  = config_get_boolean_ondemand("plugin:macos:sysctl", "ECN packets", CONFIG_ONDEMAND_ONDEMAND);
        do_tcpext_syscookies    = config_get_boolean_ondemand("plugin:macos:sysctl", "TCP SYN cookies", CONFIG_ONDEMAND_ONDEMAND);
        do_tcpext_ofo           = config_get_boolean_ondemand("plugin:macos:sysctl", "TCP out-of-order queue", CONFIG_ONDEMAND_ONDEMAND);
        do_tcpext_connaborts    = config_get_boolean_ondemand("plugin:macos:sysctl", "TCP connection aborts", CONFIG_ONDEMAND_ONDEMAND);
        do_udp_packets          = config_get_boolean("plugin:macos:sysctl", "ipv4 UDP packets", 1);
        do_udp_errors           = config_get_boolean("plugin:macos:sysctl", "ipv4 UDP errors", 1);
        do_icmp_packets         = config_get_boolean("plugin:macos:sysctl", "ipv4 ICMP packets", 1);
        do_icmpmsg              = config_get_boolean("plugin:macos:sysctl", "ipv4 ICMP messages", 1);
    }

    RRDSET *st;

    int system_pagesize = getpagesize(); // wouldn't it be better to get value directly from hw.pagesize?
    int i, n;
    int common_error = 0;
    size_t size;

    // NEEDED BY: do_loadavg
    static usec_t last_loadavg_usec = 0;
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
    uint64_t tcps_states[TCP_NSTATES];

    // NEEDED BY: do_udp...
    struct udpstat udpstat;

    // NEEDED BY: do_icmp...
    struct icmpstat icmpstat;
    struct icmp_total {
        u_long  msgs_in;
        u_long  msgs_out;
    } icmp_total = {0, 0};

    // --------------------------------------------------------------------

    if (last_loadavg_usec <= dt) {
        if (likely(do_loadavg)) {
            if (unlikely(GETSYSCTL("vm.loadavg", sysload))) {
                do_loadavg = 0;
                error("DISABLED: system.load");
            } else {

                st = rrdset_find_bytype("system", "load");
                if (unlikely(!st)) {
                    st = rrdset_create("system", "load", NULL, "load", NULL, "System Load Average", "load", 100, (update_every < MIN_LOADAVG_UPDATE_EVERY) ? MIN_LOADAVG_UPDATE_EVERY : update_every, RRDSET_TYPE_LINE);
                    rrddim_add(st, "load1", NULL, 1, 1000, RRDDIM_ABSOLUTE);
                    rrddim_add(st, "load5", NULL, 1, 1000, RRDDIM_ABSOLUTE);
                    rrddim_add(st, "load15", NULL, 1, 1000, RRDDIM_ABSOLUTE);
                }
                else rrdset_next(st);

                rrddim_set(st, "load1", (collected_number) ((double)sysload.ldavg[0] / sysload.fscale * 1000));
                rrddim_set(st, "load5", (collected_number) ((double)sysload.ldavg[1] / sysload.fscale * 1000));
                rrddim_set(st, "load15", (collected_number) ((double)sysload.ldavg[2] / sysload.fscale * 1000));
                rrdset_done(st);
            }
        }

        last_loadavg_usec = st->update_every * USEC_PER_SEC;
    }
    else last_loadavg_usec -= dt;

    // --------------------------------------------------------------------

    if (likely(do_swap)) {
        if (unlikely(GETSYSCTL("vm.swapusage", swap_usage))) {
            do_swap = 0;
            error("DISABLED: system.swap");
        } else {
            st = rrdset_find("system.swap");
            if (unlikely(!st)) {
                st = rrdset_create("system", "swap", NULL, "swap", NULL, "System Swap", "MB", 201, update_every, RRDSET_TYPE_STACKED);
                st->isdetail = 1;

                rrddim_add(st, "free",    NULL, 1, 1048576, RRDDIM_ABSOLUTE);
                rrddim_add(st, "used",    NULL, 1, 1048576, RRDDIM_ABSOLUTE);
            }
            else rrdset_next(st);

            rrddim_set(st, "free", swap_usage.xsu_avail);
            rrddim_set(st, "used", swap_usage.xsu_used);
            rrdset_done(st);
        }
    }

    // --------------------------------------------------------------------

    if (likely(do_bandwidth)) {
        mib[0] = CTL_NET;
        mib[1] = PF_ROUTE;
        mib[2] = 0;
        mib[3] = AF_INET;
        mib[4] = NET_RT_IFLIST2;
        mib[5] = 0;
        if (unlikely(sysctl(mib, 6, NULL, &size, NULL, 0))) {
            error("MACOS: sysctl(%s...) failed: %s", "net interfaces", strerror(errno));
            do_bandwidth = 0;
            error("DISABLED: system.ipv4");
        } else {
            ifstatdata = reallocz(ifstatdata, size);
            if (unlikely(sysctl(mib, 6, ifstatdata, &size, NULL, 0) < 0)) {
                error("MACOS: sysctl(%s...) failed: %s", "net interfaces", strerror(errno));
                do_bandwidth = 0;
                error("DISABLED: system.ipv4");
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
                st = rrdset_find("system.ipv4");
                if (unlikely(!st)) {
                    st = rrdset_create("system", "ipv4", NULL, "network", NULL, "IPv4 Bandwidth", "kilobits/s", 500, update_every, RRDSET_TYPE_AREA);

                    rrddim_add(st, "InOctets", "received", 8, 1024, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutOctets", "sent", -8, 1024, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "InOctets", iftot.ift_ibytes);
                rrddim_set(st, "OutOctets", iftot.ift_obytes);
                rrdset_done(st);
            }
        }
    }

    // --------------------------------------------------------------------

    // see http://net-snmp.sourceforge.net/docs/mibs/tcp.html
    if (likely(do_tcp_packets || do_tcp_errors || do_tcp_handshake || do_tcpext_connaborts || do_tcpext_ofo || do_tcpext_syscookies || do_ecn)) {
        if (unlikely(GETSYSCTL("net.inet.tcp.stats", tcpstat))){
            do_tcp_packets = 0;
            error("DISABLED: ipv4.tcppackets");
            do_tcp_errors = 0;
            error("DISABLED: ipv4.tcperrors");
            do_tcp_handshake = 0;
            error("DISABLED: ipv4.tcphandshake");
            do_tcpext_connaborts = 0;
            error("DISABLED: ipv4.tcpconnaborts");
            do_tcpext_ofo = 0;
            error("DISABLED: ipv4.tcpofo");
            do_tcpext_syscookies = 0;
            error("DISABLED: ipv4.tcpsyncookies");
            do_ecn = 0;
            error("DISABLED: ipv4.ecnpkts");
        } else {
            if (likely(do_tcp_packets)) {
                st = rrdset_find("ipv4.tcppackets");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "tcppackets", NULL, "tcp", NULL, "IPv4 TCP Packets",
                                       "packets/s",
                                       2600, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "InSegs", "received", 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutSegs", "sent", -1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set(st, "InSegs", tcpstat.tcps_rcvtotal);
                rrddim_set(st, "OutSegs", tcpstat.tcps_sndtotal);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (likely(do_tcp_errors)) {
                st = rrdset_find("ipv4.tcperrors");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "tcperrors", NULL, "tcp", NULL, "IPv4 TCP Errors",
                                       "packets/s",
                                       2700, update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "InErrs", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InCsumErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "RetransSegs", NULL, -1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set(st, "InErrs", tcpstat.tcps_rcvbadoff + tcpstat.tcps_rcvshort);
                rrddim_set(st, "InCsumErrors", tcpstat.tcps_rcvbadsum);
                rrddim_set(st, "RetransSegs", tcpstat.tcps_sndrexmitpack);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (likely(do_tcp_handshake)) {
                st = rrdset_find("ipv4.tcphandshake");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "tcphandshake", NULL, "tcp", NULL,
                                       "IPv4 TCP Handshake Issues",
                                       "events/s", 2900, update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "EstabResets", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "ActiveOpens", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "PassiveOpens", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "AttemptFails", NULL, 1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set(st, "EstabResets", tcpstat.tcps_drops);
                rrddim_set(st, "ActiveOpens", tcpstat.tcps_connattempt);
                rrddim_set(st, "PassiveOpens", tcpstat.tcps_accepts);
                rrddim_set(st, "AttemptFails", tcpstat.tcps_conndrops);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_tcpext_connaborts == CONFIG_ONDEMAND_YES || (do_tcpext_connaborts == CONFIG_ONDEMAND_ONDEMAND && (tcpstat.tcps_rcvpackafterwin || tcpstat.tcps_rcvafterclose || tcpstat.tcps_rcvmemdrop || tcpstat.tcps_persistdrop))) {
                do_tcpext_connaborts = CONFIG_ONDEMAND_YES;
                st = rrdset_find("ipv4.tcpconnaborts");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "tcpconnaborts", NULL, "tcp", NULL, "TCP Connection Aborts", "connections/s", 3010, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "TCPAbortOnData",    "baddata",     1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "TCPAbortOnClose",   "userclosed",  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "TCPAbortOnMemory",  "nomemory",    1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "TCPAbortOnTimeout", "timeout",     1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "TCPAbortOnData",    tcpstat.tcps_rcvpackafterwin);
                rrddim_set(st, "TCPAbortOnClose",   tcpstat.tcps_rcvafterclose);
                rrddim_set(st, "TCPAbortOnMemory",  tcpstat.tcps_rcvmemdrop);
                rrddim_set(st, "TCPAbortOnTimeout", tcpstat.tcps_persistdrop);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_tcpext_ofo == CONFIG_ONDEMAND_YES || (do_tcpext_ofo == CONFIG_ONDEMAND_ONDEMAND && tcpstat.tcps_rcvoopack)) {
                do_tcpext_ofo = CONFIG_ONDEMAND_YES;
                st = rrdset_find("ipv4.tcpofo");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "tcpofo", NULL, "tcp", NULL, "TCP Out-Of-Order Queue", "packets/s", 3050, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "TCPOFOQueue", "inqueue",  1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "TCPOFOQueue",   tcpstat.tcps_rcvoopack);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_tcpext_syscookies == CONFIG_ONDEMAND_YES || (do_tcpext_syscookies == CONFIG_ONDEMAND_ONDEMAND && (tcpstat.tcps_sc_sendcookie || tcpstat.tcps_sc_recvcookie || tcpstat.tcps_sc_zonefail))) {
                do_tcpext_syscookies = CONFIG_ONDEMAND_YES;

                st = rrdset_find("ipv4.tcpsyncookies");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "tcpsyncookies", NULL, "tcp", NULL, "TCP SYN Cookies", "packets/s", 3100, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "SyncookiesRecv",   "received",  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "SyncookiesSent",   "sent",     -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "SyncookiesFailed", "failed",   -1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "SyncookiesRecv",   tcpstat.tcps_sc_recvcookie);
                rrddim_set(st, "SyncookiesSent",   tcpstat.tcps_sc_sendcookie);
                rrddim_set(st, "SyncookiesFailed", tcpstat.tcps_sc_zonefail);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (do_ecn == CONFIG_ONDEMAND_YES || (do_ecn == CONFIG_ONDEMAND_ONDEMAND && (tcpstat.tcps_ecn_recv_ce || tcpstat.tcps_ecn_not_supported))) {
                do_ecn = CONFIG_ONDEMAND_YES;
                st = rrdset_find("ipv4.ecnpkts");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "ecnpkts", NULL, "ecn", NULL, "IPv4 ECN Statistics", "packets/s", 8700, update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "InCEPkts", "CEP", 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InNoECTPkts", "NoECTP", -1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "InCEPkts", tcpstat.tcps_ecn_recv_ce);
                rrddim_set(st, "InNoECTPkts", tcpstat.tcps_ecn_not_supported);
                rrdset_done(st);
            }

        }
    }

    // --------------------------------------------------------------------

    // see http://net-snmp.sourceforge.net/docs/mibs/udp.html
    if (likely(do_udp_packets || do_udp_errors)) {
        if (unlikely(GETSYSCTL("net.inet.udp.stats", udpstat))) {
            do_udp_packets = 0;
            error("DISABLED: ipv4.udppackets");
            do_udp_errors = 0;
            error("DISABLED: ipv4.udperrors");
        } else {
            if (likely(do_udp_packets)) {
                st = rrdset_find("ipv4.udppackets");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "udppackets", NULL, "udp", NULL, "IPv4 UDP Packets",
                                       "packets/s", 2601, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "InDatagrams", "received", 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutDatagrams", "sent", -1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set(st, "InDatagrams", udpstat.udps_ipackets);
                rrddim_set(st, "OutDatagrams", udpstat.udps_opackets);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (likely(do_udp_errors)) {
                st = rrdset_find("ipv4.udperrors");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "udperrors", NULL, "udp", NULL, "IPv4 UDP Errors", "events/s",
                                       2701, update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "RcvbufErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "NoPorts", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InCsumErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "IgnoredMulti", NULL, 1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set(st, "InErrors", udpstat.udps_hdrops + udpstat.udps_badlen);
                rrddim_set(st, "NoPorts", udpstat.udps_noport);
                rrddim_set(st, "RcvbufErrors", udpstat.udps_fullsock);
                rrddim_set(st, "InCsumErrors", udpstat.udps_badsum + udpstat.udps_nosum);
                rrddim_set(st, "IgnoredMulti", udpstat.udps_filtermcast);
                rrdset_done(st);
            }
        }
    }

    // --------------------------------------------------------------------

    if (likely(do_icmp_packets || do_icmpmsg)) {
        if (unlikely(GETSYSCTL("net.inet.icmp.stats", icmpstat))) {
            do_icmp_packets = 0;
            error("DISABLED: ipv4.icmp");
            error("DISABLED: ipv4.icmp_errors");
            do_icmpmsg = 0;
            error("DISABLED: ipv4.icmpmsg");
        } else {
            for (i = 0; i <= ICMP_MAXTYPE; i++) {
                icmp_total.msgs_in += icmpstat.icps_inhist[i];
                icmp_total.msgs_out += icmpstat.icps_outhist[i];
            }
            icmp_total.msgs_in += icmpstat.icps_badcode + icmpstat.icps_badlen + icmpstat.icps_checksum + icmpstat.icps_tooshort;

            // --------------------------------------------------------------------

            if (likely(do_icmp_packets)) {
                st = rrdset_find("ipv4.icmp");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "icmp", NULL, "icmp", NULL, "IPv4 ICMP Packets", "packets/s",
                                       2602,
                                       update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "InMsgs", "received", 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutMsgs", "sent", -1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set(st, "InMsgs", icmp_total.msgs_in);
                rrddim_set(st, "OutMsgs", icmp_total.msgs_out);

                rrdset_done(st);

                // --------------------------------------------------------------------

                st = rrdset_find("ipv4.icmp_errors");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "icmp_errors", NULL, "icmp", NULL, "IPv4 ICMP Errors",
                                       "packets/s",
                                       2603, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "InErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutErrors", NULL, -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InCsumErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set(st, "InErrors", icmpstat.icps_badcode + icmpstat.icps_badlen + icmpstat.icps_checksum + icmpstat.icps_tooshort);
                rrddim_set(st, "OutErrors", icmpstat.icps_error);
                rrddim_set(st, "InCsumErrors", icmpstat.icps_checksum);

                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (likely(do_icmpmsg)) {
                st = rrdset_find("ipv4.icmpmsg");
                if (unlikely(!st)) {
                    st = rrdset_create("ipv4", "icmpmsg", NULL, "icmp", NULL, "IPv4 ICMP Messsages",
                                       "packets/s", 2604, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "InEchoReps", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutEchoReps", NULL, -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InEchos", NULL, 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutEchos", NULL, -1, 1, RRDDIM_INCREMENTAL);
                } else
                    rrdset_next(st);

                    rrddim_set(st, "InEchoReps", icmpstat.icps_inhist[ICMP_ECHOREPLY]);
                    rrddim_set(st, "OutEchoReps", icmpstat.icps_outhist[ICMP_ECHOREPLY]);
                    rrddim_set(st, "InEchos", icmpstat.icps_inhist[ICMP_ECHO]);
                    rrddim_set(st, "OutEchos", icmpstat.icps_outhist[ICMP_ECHO]);

                rrdset_done(st);
            }
        }
    }

    return 0;
}

int getsysctl(const char *name, void *ptr, size_t len)
{
    size_t nlen = len;

    if (unlikely(sysctlbyname(name, ptr, &nlen, NULL, 0) == -1)) {
        error("MACOS: sysctl(%s...) failed: %s", name, strerror(errno));
        return 1;
    }
    if (unlikely(nlen != len)) {
        error("MACOS: sysctl(%s...) expected %lu, got %lu", name, (unsigned long)len, (unsigned long)nlen);
        return 1;
    }
    return 0;
}

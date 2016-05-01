#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "common.h"
#include "log.h"
#include "appconfig.h"
#include "procfile.h"
#include "rrd.h"
#include "plugin_proc.h"

#define RRD_TYPE_NET_SNMP			"ipv4"
#define RRD_TYPE_NET_SNMP_LEN		strlen(RRD_TYPE_NET_SNMP)

int do_proc_net_snmp(int update_every, unsigned long long dt) {
	static procfile *ff = NULL;
	static int do_ip_packets = -1, do_ip_fragsout = -1, do_ip_fragsin = -1, do_ip_errors = -1,
		do_tcp_sockets = -1, do_tcp_packets = -1, do_tcp_errors = -1, do_tcp_handshake = -1,
		do_udp_packets = -1, do_udp_errors = -1;

	if(do_ip_packets == -1)		do_ip_packets 		= config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 packets", 1);
	if(do_ip_fragsout == -1)	do_ip_fragsout 		= config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 fragments sent", 1);
	if(do_ip_fragsin == -1)		do_ip_fragsin 		= config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 fragments assembly", 1);
	if(do_ip_errors == -1)		do_ip_errors 		= config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 errors", 1);
	if(do_tcp_sockets == -1)	do_tcp_sockets 		= config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 TCP connections", 1);
	if(do_tcp_packets == -1)	do_tcp_packets 		= config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 TCP packets", 1);
	if(do_tcp_errors == -1)		do_tcp_errors 		= config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 TCP errors", 1);
	if(do_tcp_handshake == -1)	do_tcp_handshake 	= config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 TCP handshake issues", 1);
	if(do_udp_packets == -1)	do_udp_packets 		= config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 UDP packets", 1);
	if(do_udp_errors == -1)		do_udp_errors 		= config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 UDP errors", 1);

	if(dt) {};

	if(!ff) {
		char filename[FILENAME_MAX + 1];
		snprintfz(filename, FILENAME_MAX, "%s%s", global_host_prefix, "/proc/net/snmp");
		ff = procfile_open(config_get("plugin:proc:/proc/net/snmp", "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
	}
	if(!ff) return 1;

	ff = procfile_readall(ff);
	if(!ff) return 0; // we return 0, so that we will retry to open it next time

	uint32_t lines = procfile_lines(ff), l;
	uint32_t words;

	RRDSET *st;

	for(l = 0; l < lines ;l++) {
		if(strcmp(procfile_lineword(ff, l, 0), "Ip") == 0) {
			l++;

			if(strcmp(procfile_lineword(ff, l, 0), "Ip") != 0) {
				error("Cannot read Ip line from /proc/net/snmp.");
				break;
			}

			words = procfile_linewords(ff, l);
			if(words < 20) {
				error("Cannot read /proc/net/snmp Ip line. Expected 20 params, read %d.", words);
				continue;
			}

			// see also http://net-snmp.sourceforge.net/docs/mibs/ip.html
			unsigned long long Forwarding, DefaultTTL, InReceives, InHdrErrors, InAddrErrors, ForwDatagrams, InUnknownProtos, InDiscards, InDelivers,
				OutRequests, OutDiscards, OutNoRoutes, ReasmTimeout, ReasmReqds, ReasmOKs, ReasmFails, FragOKs, FragFails, FragCreates;

			Forwarding		= strtoull(procfile_lineword(ff, l, 1), NULL, 10);
			DefaultTTL		= strtoull(procfile_lineword(ff, l, 2), NULL, 10);
			InReceives		= strtoull(procfile_lineword(ff, l, 3), NULL, 10);
			InHdrErrors		= strtoull(procfile_lineword(ff, l, 4), NULL, 10);
			InAddrErrors	= strtoull(procfile_lineword(ff, l, 5), NULL, 10);
			ForwDatagrams	= strtoull(procfile_lineword(ff, l, 6), NULL, 10);
			InUnknownProtos	= strtoull(procfile_lineword(ff, l, 7), NULL, 10);
			InDiscards		= strtoull(procfile_lineword(ff, l, 8), NULL, 10);
			InDelivers		= strtoull(procfile_lineword(ff, l, 9), NULL, 10);
			OutRequests		= strtoull(procfile_lineword(ff, l, 10), NULL, 10);
			OutDiscards		= strtoull(procfile_lineword(ff, l, 11), NULL, 10);
			OutNoRoutes		= strtoull(procfile_lineword(ff, l, 12), NULL, 10);
			ReasmTimeout	= strtoull(procfile_lineword(ff, l, 13), NULL, 10);
			ReasmReqds		= strtoull(procfile_lineword(ff, l, 14), NULL, 10);
			ReasmOKs		= strtoull(procfile_lineword(ff, l, 15), NULL, 10);
			ReasmFails		= strtoull(procfile_lineword(ff, l, 16), NULL, 10);
			FragOKs			= strtoull(procfile_lineword(ff, l, 17), NULL, 10);
			FragFails		= strtoull(procfile_lineword(ff, l, 18), NULL, 10);
			FragCreates		= strtoull(procfile_lineword(ff, l, 19), NULL, 10);

			// these are not counters
			if(Forwarding) {};		// is forwarding enabled?
			if(DefaultTTL) {};		// the default ttl on packets
			if(ReasmTimeout) {};	// Reassembly timeout

			// this counter is not used
			if(InDelivers) {};		// total number of packets delivered to IP user-protocols

			// --------------------------------------------------------------------

			if(do_ip_packets) {
				st = rrdset_find(RRD_TYPE_NET_SNMP ".packets");
				if(!st) {
					st = rrdset_create(RRD_TYPE_NET_SNMP, "packets", NULL, "packets", NULL, "IPv4 Packets", "packets/s", 3000, update_every, RRDSET_TYPE_LINE);

					rrddim_add(st, "received", NULL, 1, 1, RRDDIM_INCREMENTAL);
					rrddim_add(st, "sent", NULL, -1, 1, RRDDIM_INCREMENTAL);
					rrddim_add(st, "forwarded", NULL, 1, 1, RRDDIM_INCREMENTAL);
				}
				else rrdset_next(st);

				rrddim_set(st, "sent", OutRequests);
				rrddim_set(st, "received", InReceives);
				rrddim_set(st, "forwarded", ForwDatagrams);
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
					rrddim_add(st, "InAddrErrors", NULL, 1, 1, RRDDIM_INCREMENTAL);
					rrddim_add(st, "InUnknownProtos", NULL, 1, 1, RRDDIM_INCREMENTAL);

					rrddim_add(st, "OutNoRoutes", NULL, -1, 1, RRDDIM_INCREMENTAL);
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
		else if(strcmp(procfile_lineword(ff, l, 0), "Tcp") == 0) {
			l++;

			if(strcmp(procfile_lineword(ff, l, 0), "Tcp") != 0) {
				error("Cannot read Tcp line from /proc/net/snmp.");
				break;
			}

			words = procfile_linewords(ff, l);
			if(words < 15) {
				error("Cannot read /proc/net/snmp Tcp line. Expected 15 params, read %d.", words);
				continue;
			}

			unsigned long long RtoAlgorithm, RtoMin, RtoMax, MaxConn, ActiveOpens, PassiveOpens, AttemptFails, EstabResets,
				CurrEstab, InSegs, OutSegs, RetransSegs, InErrs, OutRsts;

			RtoAlgorithm	= strtoull(procfile_lineword(ff, l, 1), NULL, 10);
			RtoMin			= strtoull(procfile_lineword(ff, l, 2), NULL, 10);
			RtoMax			= strtoull(procfile_lineword(ff, l, 3), NULL, 10);
			MaxConn			= strtoull(procfile_lineword(ff, l, 4), NULL, 10);
			ActiveOpens		= strtoull(procfile_lineword(ff, l, 5), NULL, 10);
			PassiveOpens	= strtoull(procfile_lineword(ff, l, 6), NULL, 10);
			AttemptFails	= strtoull(procfile_lineword(ff, l, 7), NULL, 10);
			EstabResets		= strtoull(procfile_lineword(ff, l, 8), NULL, 10);
			CurrEstab		= strtoull(procfile_lineword(ff, l, 9), NULL, 10);
			InSegs			= strtoull(procfile_lineword(ff, l, 10), NULL, 10);
			OutSegs			= strtoull(procfile_lineword(ff, l, 11), NULL, 10);
			RetransSegs		= strtoull(procfile_lineword(ff, l, 12), NULL, 10);
			InErrs			= strtoull(procfile_lineword(ff, l, 13), NULL, 10);
			OutRsts			= strtoull(procfile_lineword(ff, l, 14), NULL, 10);

			// these are not counters
			if(RtoAlgorithm) {};
			if(RtoMin) {};
			if(RtoMax) {};
			if(MaxConn) {};

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
		else if(strcmp(procfile_lineword(ff, l, 0), "Udp") == 0) {
			l++;

			if(strcmp(procfile_lineword(ff, l, 0), "Udp") != 0) {
				error("Cannot read Udp line from /proc/net/snmp.");
				break;
			}

			words = procfile_linewords(ff, l);
			if(words < 7) {
				error("Cannot read /proc/net/snmp Udp line. Expected 7 params, read %d.", words);
				continue;
			}

			unsigned long long InDatagrams, NoPorts, InErrors, OutDatagrams, RcvbufErrors, SndbufErrors;

			InDatagrams		= strtoull(procfile_lineword(ff, l, 1), NULL, 10);
			NoPorts			= strtoull(procfile_lineword(ff, l, 2), NULL, 10);
			InErrors		= strtoull(procfile_lineword(ff, l, 3), NULL, 10);
			OutDatagrams	= strtoull(procfile_lineword(ff, l, 4), NULL, 10);
			RcvbufErrors	= strtoull(procfile_lineword(ff, l, 5), NULL, 10);
			SndbufErrors	= strtoull(procfile_lineword(ff, l, 6), NULL, 10);

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
	}

	return 0;
}


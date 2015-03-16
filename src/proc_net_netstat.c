#include <inttypes.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "log.h"
#include "config.h"
#include "procfile.h"
#include "rrd.h"
#include "plugin_proc.h"

int do_proc_net_netstat(int update_every, unsigned long long dt) {
	static int do_bandwidth = -1, do_inerrors = -1, do_mcast = -1, do_bcast = -1, do_mcast_p = -1, do_bcast_p = -1;
	static procfile *ff = NULL;

	if(do_bandwidth == -1)	do_bandwidth	= config_get_boolean("plugin:proc:/proc/net/netstat", "bandwidth", 1);
	if(do_inerrors == -1)	do_inerrors		= config_get_boolean("plugin:proc:/proc/net/netstat", "input errors", 1);
	if(do_mcast == -1)		do_mcast 		= config_get_boolean("plugin:proc:/proc/net/netstat", "multicast bandwidth", 1);
	if(do_bcast == -1)		do_bcast 		= config_get_boolean("plugin:proc:/proc/net/netstat", "broadcast bandwidth", 1);
	if(do_mcast_p == -1)	do_mcast_p 		= config_get_boolean("plugin:proc:/proc/net/netstat", "multicast packets", 1);
	if(do_bcast_p == -1)	do_bcast_p 		= config_get_boolean("plugin:proc:/proc/net/netstat", "broadcast packets", 1);

	if(dt) {};

	if(!ff) ff = procfile_open("/proc/net/netstat", " \t:");
	if(!ff) return 1;

	ff = procfile_readall(ff);
	if(!ff) return 0; // we return 0, so that we will retry to open it next time

	uint32_t lines = procfile_lines(ff), l;
	uint32_t words;

	for(l = 0; l < lines ;l++) {
		if(strcmp(procfile_lineword(ff, l, 0), "IpExt") == 0) {
			l++; // we need the next line

			if(strcmp(procfile_lineword(ff, l, 0), "IpExt") != 0) {
				error("Cannot read IpExt line from /proc/net/netstat.");
				break;
			}
			words = procfile_linewords(ff, l);
			if(words < 12) {
				error("Cannot read /proc/net/netstat IpExt line. Expected 12 params, read %d.", words);
				continue;
			}

			unsigned long long
				InNoRoutes = 0, InTruncatedPkts = 0,
				InOctets = 0,  InMcastPkts = 0,  InBcastPkts = 0,  InMcastOctets = 0,  InBcastOctets = 0,
				OutOctets = 0, OutMcastPkts = 0, OutBcastPkts = 0, OutMcastOctets = 0, OutBcastOctets = 0;
			
			InNoRoutes 			= strtoull(procfile_lineword(ff, l, 1), NULL, 10);
			InTruncatedPkts 	= strtoull(procfile_lineword(ff, l, 2), NULL, 10);
			InMcastPkts 		= strtoull(procfile_lineword(ff, l, 3), NULL, 10);
			OutMcastPkts 		= strtoull(procfile_lineword(ff, l, 4), NULL, 10);
			InBcastPkts 		= strtoull(procfile_lineword(ff, l, 5), NULL, 10);
			OutBcastPkts 		= strtoull(procfile_lineword(ff, l, 6), NULL, 10);
			InOctets 			= strtoull(procfile_lineword(ff, l, 7), NULL, 10);
			OutOctets 			= strtoull(procfile_lineword(ff, l, 8), NULL, 10);
			InMcastOctets 		= strtoull(procfile_lineword(ff, l, 9), NULL, 10);
			OutMcastOctets 		= strtoull(procfile_lineword(ff, l, 10), NULL, 10);
			InBcastOctets 		= strtoull(procfile_lineword(ff, l, 11), NULL, 10);
			OutBcastOctets 		= strtoull(procfile_lineword(ff, l, 12), NULL, 10);

			RRD_STATS *st;

			// --------------------------------------------------------------------

			if(do_bandwidth) {
				st = rrd_stats_find("system.ipv4");
				if(!st) {
					st = rrd_stats_create("system", "ipv4", NULL, "ipv4", "IPv4 Bandwidth", "kilobits/s", 2000, update_every, CHART_TYPE_AREA);

					rrd_stats_dimension_add(st, "received", NULL, 8, 1024 * update_every, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "sent", NULL, -8, 1024 * update_every, RRD_DIMENSION_INCREMENTAL);
				}
				else rrd_stats_next(st);

				rrd_stats_dimension_set(st, "sent", OutOctets);
				rrd_stats_dimension_set(st, "received", InOctets);
				rrd_stats_done(st);
			}

			// --------------------------------------------------------------------

			if(do_inerrors) {
				st = rrd_stats_find("ipv4.inerrors");
				if(!st) {
					st = rrd_stats_create("ipv4", "inerrors", NULL, "ipv4", "IPv4 Input Errors", "packets/s", 4000, update_every, CHART_TYPE_LINE);
					st->isdetail = 1;

					rrd_stats_dimension_add(st, "noroutes", NULL, 1, 1 * update_every, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "trunkated", NULL, 1, 1 * update_every, RRD_DIMENSION_INCREMENTAL);
				}
				else rrd_stats_next(st);

				rrd_stats_dimension_set(st, "noroutes", InNoRoutes);
				rrd_stats_dimension_set(st, "trunkated", InTruncatedPkts);
				rrd_stats_done(st);
			}

			// --------------------------------------------------------------------

			if(do_mcast) {
				st = rrd_stats_find("ipv4.mcast");
				if(!st) {
					st = rrd_stats_create("ipv4", "mcast", NULL, "ipv4", "IPv4 Multicast Bandwidth", "kilobits/s", 9000, update_every, CHART_TYPE_AREA);
					st->isdetail = 1;

					rrd_stats_dimension_add(st, "received", NULL, 8, 1024 * update_every, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "sent", NULL, -8, 1024 * update_every, RRD_DIMENSION_INCREMENTAL);
				}
				else rrd_stats_next(st);

				rrd_stats_dimension_set(st, "sent", OutMcastOctets);
				rrd_stats_dimension_set(st, "received", InMcastOctets);
				rrd_stats_done(st);
			}

			// --------------------------------------------------------------------

			if(do_bcast) {
				st = rrd_stats_find("ipv4.bcast");
				if(!st) {
					st = rrd_stats_create("ipv4", "bcast", NULL, "ipv4", "IPv4 Broadcast Bandwidth", "kilobits/s", 8000, update_every, CHART_TYPE_AREA);
					st->isdetail = 1;

					rrd_stats_dimension_add(st, "received", NULL, 8, 1024 * update_every, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "sent", NULL, -8, 1024 * update_every, RRD_DIMENSION_INCREMENTAL);
				}
				else rrd_stats_next(st);

				rrd_stats_dimension_set(st, "sent", OutBcastOctets);
				rrd_stats_dimension_set(st, "received", InBcastOctets);
				rrd_stats_done(st);
			}

			// --------------------------------------------------------------------

			if(do_mcast_p) {
				st = rrd_stats_find("ipv4.mcastpkts");
				if(!st) {
					st = rrd_stats_create("ipv4", "mcastpkts", NULL, "ipv4", "IPv4 Multicast Packets", "packets/s", 9500, update_every, CHART_TYPE_LINE);
					st->isdetail = 1;

					rrd_stats_dimension_add(st, "received", NULL, 1, 1 * update_every, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "sent", NULL, -1, 1 * update_every, RRD_DIMENSION_INCREMENTAL);
				}
				else rrd_stats_next(st);

				rrd_stats_dimension_set(st, "sent", OutMcastPkts);
				rrd_stats_dimension_set(st, "received", InMcastPkts);
				rrd_stats_done(st);
			}

			// --------------------------------------------------------------------

			if(do_bcast_p) {
				st = rrd_stats_find("ipv4.bcastpkts");
				if(!st) {
					st = rrd_stats_create("ipv4", "bcastpkts", NULL, "ipv4", "IPv4 Broadcast Packets", "packets/s", 8500, update_every, CHART_TYPE_LINE);
					st->isdetail = 1;

					rrd_stats_dimension_add(st, "received", NULL, 1, 1 * update_every, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "sent", NULL, -1, 1 * update_every, RRD_DIMENSION_INCREMENTAL);
				}
				else rrd_stats_next(st);

				rrd_stats_dimension_set(st, "sent", OutBcastPkts);
				rrd_stats_dimension_set(st, "received", InBcastPkts);
				rrd_stats_done(st);
			}
		}
	}
	
	return 0;
}

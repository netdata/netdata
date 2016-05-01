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

int do_proc_net_netstat(int update_every, unsigned long long dt) {
	static int do_bandwidth = -1, do_inerrors = -1, do_mcast = -1, do_bcast = -1, do_mcast_p = -1, do_bcast_p = -1;
	static procfile *ff = NULL;

	if(do_bandwidth == -1)	do_bandwidth	= config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "bandwidth", CONFIG_ONDEMAND_ONDEMAND);
	if(do_inerrors == -1)	do_inerrors		= config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "input errors", CONFIG_ONDEMAND_ONDEMAND);
	if(do_mcast == -1)		do_mcast 		= config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "multicast bandwidth", CONFIG_ONDEMAND_ONDEMAND);
	if(do_bcast == -1)		do_bcast 		= config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "broadcast bandwidth", CONFIG_ONDEMAND_ONDEMAND);
	if(do_mcast_p == -1)	do_mcast_p 		= config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "multicast packets", CONFIG_ONDEMAND_ONDEMAND);
	if(do_bcast_p == -1)	do_bcast_p 		= config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "broadcast packets", CONFIG_ONDEMAND_ONDEMAND);

	if(dt) {};

	if(!ff) {
		char filename[FILENAME_MAX + 1];
		snprintfz(filename, FILENAME_MAX, "%s%s", global_host_prefix, "/proc/net/netstat");
		ff = procfile_open(config_get("plugin:proc:/proc/net/netstat", "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
	}
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

			RRDSET *st;

			// --------------------------------------------------------------------

			if(do_bandwidth == CONFIG_ONDEMAND_YES || (do_bandwidth == CONFIG_ONDEMAND_ONDEMAND && (InOctets || OutOctets))) {
				do_bandwidth = CONFIG_ONDEMAND_YES;
				st = rrdset_find("system.ipv4");
				if(!st) {
					st = rrdset_create("system", "ipv4", NULL, "network", NULL, "IPv4 Bandwidth", "kilobits/s", 500, update_every, RRDSET_TYPE_AREA);

					rrddim_add(st, "received", NULL, 8, 1024, RRDDIM_INCREMENTAL);
					rrddim_add(st, "sent", NULL, -8, 1024, RRDDIM_INCREMENTAL);
				}
				else rrdset_next(st);

				rrddim_set(st, "sent", OutOctets);
				rrddim_set(st, "received", InOctets);
				rrdset_done(st);
			}

			// --------------------------------------------------------------------

			if(do_inerrors == CONFIG_ONDEMAND_YES || (do_inerrors == CONFIG_ONDEMAND_ONDEMAND && (InNoRoutes || InTruncatedPkts))) {
				do_inerrors = CONFIG_ONDEMAND_YES;
				st = rrdset_find("ipv4.inerrors");
				if(!st) {
					st = rrdset_create("ipv4", "inerrors", NULL, "errors", NULL, "IPv4 Input Errors", "packets/s", 4000, update_every, RRDSET_TYPE_LINE);
					st->isdetail = 1;

					rrddim_add(st, "noroutes", NULL, 1, 1, RRDDIM_INCREMENTAL);
					rrddim_add(st, "truncated", NULL, 1, 1, RRDDIM_INCREMENTAL);
				}
				else rrdset_next(st);

				rrddim_set(st, "noroutes", InNoRoutes);
				rrddim_set(st, "truncated", InTruncatedPkts);
				rrdset_done(st);
			}

			// --------------------------------------------------------------------

			if(do_mcast == CONFIG_ONDEMAND_YES || (do_mcast == CONFIG_ONDEMAND_ONDEMAND && (InMcastOctets || OutMcastOctets))) {
				do_mcast = CONFIG_ONDEMAND_YES;
				st = rrdset_find("ipv4.mcast");
				if(!st) {
					st = rrdset_create("ipv4", "mcast", NULL, "multicast", NULL, "IPv4 Multicast Bandwidth", "kilobits/s", 9000, update_every, RRDSET_TYPE_AREA);
					st->isdetail = 1;

					rrddim_add(st, "received", NULL, 8, 1024, RRDDIM_INCREMENTAL);
					rrddim_add(st, "sent", NULL, -8, 1024, RRDDIM_INCREMENTAL);
				}
				else rrdset_next(st);

				rrddim_set(st, "sent", OutMcastOctets);
				rrddim_set(st, "received", InMcastOctets);
				rrdset_done(st);
			}

			// --------------------------------------------------------------------

			if(do_bcast == CONFIG_ONDEMAND_YES || (do_bcast == CONFIG_ONDEMAND_ONDEMAND && (InBcastOctets || OutBcastOctets))) {
				do_bcast = CONFIG_ONDEMAND_YES;
				st = rrdset_find("ipv4.bcast");
				if(!st) {
					st = rrdset_create("ipv4", "bcast", NULL, "broadcast", NULL, "IPv4 Broadcast Bandwidth", "kilobits/s", 8000, update_every, RRDSET_TYPE_AREA);
					st->isdetail = 1;

					rrddim_add(st, "received", NULL, 8, 1024, RRDDIM_INCREMENTAL);
					rrddim_add(st, "sent", NULL, -8, 1024, RRDDIM_INCREMENTAL);
				}
				else rrdset_next(st);

				rrddim_set(st, "sent", OutBcastOctets);
				rrddim_set(st, "received", InBcastOctets);
				rrdset_done(st);
			}

			// --------------------------------------------------------------------

			if(do_mcast_p == CONFIG_ONDEMAND_YES || (do_mcast_p == CONFIG_ONDEMAND_ONDEMAND && (InMcastPkts || OutMcastPkts))) {
				do_mcast_p = CONFIG_ONDEMAND_YES;
				st = rrdset_find("ipv4.mcastpkts");
				if(!st) {
					st = rrdset_create("ipv4", "mcastpkts", NULL, "multicast", NULL, "IPv4 Multicast Packets", "packets/s", 9500, update_every, RRDSET_TYPE_LINE);
					st->isdetail = 1;

					rrddim_add(st, "received", NULL, 1, 1, RRDDIM_INCREMENTAL);
					rrddim_add(st, "sent", NULL, -1, 1, RRDDIM_INCREMENTAL);
				}
				else rrdset_next(st);

				rrddim_set(st, "sent", OutMcastPkts);
				rrddim_set(st, "received", InMcastPkts);
				rrdset_done(st);
			}

			// --------------------------------------------------------------------

			if(do_bcast_p == CONFIG_ONDEMAND_YES || (do_bcast_p == CONFIG_ONDEMAND_ONDEMAND && (InBcastPkts || OutBcastPkts))) {
				do_bcast_p = CONFIG_ONDEMAND_YES;
				st = rrdset_find("ipv4.bcastpkts");
				if(!st) {
					st = rrdset_create("ipv4", "bcastpkts", NULL, "broadcast", NULL, "IPv4 Broadcast Packets", "packets/s", 8500, update_every, RRDSET_TYPE_LINE);
					st->isdetail = 1;

					rrddim_add(st, "received", NULL, 1, 1, RRDDIM_INCREMENTAL);
					rrddim_add(st, "sent", NULL, -1, 1, RRDDIM_INCREMENTAL);
				}
				else rrdset_next(st);

				rrddim_set(st, "sent", OutBcastPkts);
				rrddim_set(st, "received", InBcastPkts);
				rrdset_done(st);
			}
		}
	}

	return 0;
}

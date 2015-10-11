#include <inttypes.h>
#include <stdio.h>
#include <stdlib.h>

#include "common.h"
#include "config.h"
#include "procfile.h"
#include "rrd.h"
#include "plugin_proc.h"

int do_proc_net_dev(int update_every, unsigned long long dt) {
	static procfile *ff = NULL;
	static int enable_new_interfaces = -1;
	static int do_bandwidth = -1, do_packets = -1, do_errors = -1, do_drops = -1, do_fifo = -1, do_compressed = -1, do_events = -1;

	if(dt) {};

	if(!ff) ff = procfile_open(config_get("plugin:proc:/proc/net/dev", "filename to monitor", "/proc/net/dev"), " \t,:|", PROCFILE_FLAG_DEFAULT);
	if(!ff) return 1;

	ff = procfile_readall(ff);
	if(!ff) return 0; // we return 0, so that we will retry to open it next time

	if(enable_new_interfaces == -1)	enable_new_interfaces = config_get_boolean("plugin:proc:/proc/net/dev", "enable new interfaces detected at runtime", 1);

	if(do_bandwidth == -1)	do_bandwidth 	= config_get_boolean("plugin:proc:/proc/net/dev", "bandwidth for all interfaces", 1);
	if(do_packets == -1)	do_packets 		= config_get_boolean("plugin:proc:/proc/net/dev", "packets for all interfaces", 1);
	if(do_errors == -1)		do_errors 		= config_get_boolean("plugin:proc:/proc/net/dev", "errors for all interfaces", 1);
	if(do_drops == -1)		do_drops 		= config_get_boolean("plugin:proc:/proc/net/dev", "drops for all interfaces", 1);
	if(do_fifo == -1) 		do_fifo 		= config_get_boolean("plugin:proc:/proc/net/dev", "fifo for all interfaces", 1);
	if(do_compressed == -1)	do_compressed 	= config_get_boolean("plugin:proc:/proc/net/dev", "compressed packets for all interfaces", 1);
	if(do_events == -1)		do_events 		= config_get_boolean("plugin:proc:/proc/net/dev", "frames, collisions, carrier coutners for all interfaces", 1);

	uint32_t lines = procfile_lines(ff), l;
	uint32_t words;

	char *iface;
	unsigned long long rbytes, rpackets, rerrors, rdrops, rfifo, rframe, rcompressed, rmulticast;
	unsigned long long tbytes, tpackets, terrors, tdrops, tfifo, tcollisions, tcarrier, tcompressed;

	for(l = 2; l < lines ;l++) {
		words = procfile_linewords(ff, l);
		if(words < 17) continue;

		iface		= procfile_lineword(ff, l, 0);

		rbytes		= strtoull(procfile_lineword(ff, l, 1), NULL, 10);
		rpackets	= strtoull(procfile_lineword(ff, l, 2), NULL, 10);
		rerrors		= strtoull(procfile_lineword(ff, l, 3), NULL, 10);
		rdrops		= strtoull(procfile_lineword(ff, l, 4), NULL, 10);
		rfifo		= strtoull(procfile_lineword(ff, l, 5), NULL, 10);
		rframe		= strtoull(procfile_lineword(ff, l, 6), NULL, 10);
		rcompressed	= strtoull(procfile_lineword(ff, l, 7), NULL, 10);
		rmulticast	= strtoull(procfile_lineword(ff, l, 8), NULL, 10);

		tbytes		= strtoull(procfile_lineword(ff, l, 9), NULL, 10);
		tpackets	= strtoull(procfile_lineword(ff, l, 10), NULL, 10);
		terrors		= strtoull(procfile_lineword(ff, l, 11), NULL, 10);
		tdrops		= strtoull(procfile_lineword(ff, l, 12), NULL, 10);
		tfifo		= strtoull(procfile_lineword(ff, l, 13), NULL, 10);
		tcollisions	= strtoull(procfile_lineword(ff, l, 14), NULL, 10);
		tcarrier	= strtoull(procfile_lineword(ff, l, 15), NULL, 10);
		tcompressed	= strtoull(procfile_lineword(ff, l, 16), NULL, 10);

		{
			char var_name[4096 + 1];
			snprintf(var_name, 4096, "interface %s", iface);
			if(!config_get_boolean("plugin:proc:/proc/net/dev", var_name, enable_new_interfaces)) continue;
		}

		RRDSET *st;

		// --------------------------------------------------------------------

		if(do_bandwidth) {
			st = rrdset_find_bytype("net", iface);
			if(!st) {
				st = rrdset_create("net", iface, NULL, iface, "Bandwidth", "kilobits/s", 1000, update_every, RRDSET_TYPE_AREA);

				rrddim_add(st, "received", NULL, 8, 1024 * update_every, RRDDIM_INCREMENTAL);
				rrddim_add(st, "sent", NULL, -8, 1024 * update_every, RRDDIM_INCREMENTAL);
			}
			else rrdset_next(st);

			rrddim_set(st, "received", rbytes);
			rrddim_set(st, "sent", tbytes);
			rrdset_done(st);
		}

		// --------------------------------------------------------------------

		if(do_packets) {
			st = rrdset_find_bytype("net_packets", iface);
			if(!st) {
				st = rrdset_create("net_packets", iface, NULL, iface, "Packets", "packets/s", 1001, update_every, RRDSET_TYPE_LINE);
				st->isdetail = 1;

				rrddim_add(st, "received", NULL, 1, 1 * update_every, RRDDIM_INCREMENTAL);
				rrddim_add(st, "sent", NULL, -1, 1 * update_every, RRDDIM_INCREMENTAL);
				rrddim_add(st, "multicast", NULL, 1, 1 * update_every, RRDDIM_INCREMENTAL);
			}
			else rrdset_next(st);

			rrddim_set(st, "received", rpackets);
			rrddim_set(st, "sent", tpackets);
			rrddim_set(st, "multicast", rmulticast);
			rrdset_done(st);
		}

		// --------------------------------------------------------------------

		if(do_errors) {
			st = rrdset_find_bytype("net_errors", iface);
			if(!st) {
				st = rrdset_create("net_errors", iface, NULL, iface, "Interface Errors", "errors/s", 1002, update_every, RRDSET_TYPE_LINE);
				st->isdetail = 1;

				rrddim_add(st, "inbound", NULL, 1, 1 * update_every, RRDDIM_INCREMENTAL);
				rrddim_add(st, "outbound", NULL, -1, 1 * update_every, RRDDIM_INCREMENTAL);
			}
			else rrdset_next(st);

			rrddim_set(st, "inbound", rerrors);
			rrddim_set(st, "outbound", terrors);
			rrdset_done(st);
		}

		// --------------------------------------------------------------------

		if(do_drops) {
			st = rrdset_find_bytype("net_drops", iface);
			if(!st) {
				st = rrdset_create("net_drops", iface, NULL, iface, "Interface Drops", "drops/s", 1003, update_every, RRDSET_TYPE_LINE);
				st->isdetail = 1;

				rrddim_add(st, "inbound", NULL, 1, 1 * update_every, RRDDIM_INCREMENTAL);
				rrddim_add(st, "outbound", NULL, -1, 1 * update_every, RRDDIM_INCREMENTAL);
			}
			else rrdset_next(st);

			rrddim_set(st, "inbound", rdrops);
			rrddim_set(st, "outbound", tdrops);
			rrdset_done(st);
		}

		// --------------------------------------------------------------------

		if(do_fifo) {
			st = rrdset_find_bytype("net_fifo", iface);
			if(!st) {
				st = rrdset_create("net_fifo", iface, NULL, iface, "Interface Queue", "packets", 1100, update_every, RRDSET_TYPE_LINE);
				st->isdetail = 1;

				rrddim_add(st, "receive", NULL, 1, 1, RRDDIM_ABSOLUTE);
				rrddim_add(st, "transmit", NULL, -1, 1, RRDDIM_ABSOLUTE);
			}
			else rrdset_next(st);

			rrddim_set(st, "receive", rfifo);
			rrddim_set(st, "transmit", tfifo);
			rrdset_done(st);
		}

		// --------------------------------------------------------------------

		if(do_compressed) {
			st = rrdset_find_bytype("net_compressed", iface);
			if(!st) {
				st = rrdset_create("net_compressed", iface, NULL, iface, "Compressed Packets", "packets/s", 1200, update_every, RRDSET_TYPE_LINE);
				st->isdetail = 1;

				rrddim_add(st, "received", NULL, 1, 1 * update_every, RRDDIM_INCREMENTAL);
				rrddim_add(st, "sent", NULL, -1, 1 * update_every, RRDDIM_INCREMENTAL);
			}
			else rrdset_next(st);

			rrddim_set(st, "received", rcompressed);
			rrddim_set(st, "sent", tcompressed);
			rrdset_done(st);
		}

		// --------------------------------------------------------------------

		if(do_events) {
			st = rrdset_find_bytype("net_events", iface);
			if(!st) {
				st = rrdset_create("net_events", iface, NULL, iface, "Network Interface Events", "events/s", 1200, update_every, RRDSET_TYPE_LINE);
				st->isdetail = 1;

				rrddim_add(st, "frames", NULL, 1, 1 * update_every, RRDDIM_INCREMENTAL);
				rrddim_add(st, "collisions", NULL, -1, 1 * update_every, RRDDIM_INCREMENTAL);
				rrddim_add(st, "carrier", NULL, -1, 1 * update_every, RRDDIM_INCREMENTAL);
			}
			else rrdset_next(st);

			rrddim_set(st, "frames", rframe);
			rrddim_set(st, "collisions", tcollisions);
			rrddim_set(st, "carrier", tcarrier);
			rrdset_done(st);
		}
	}

	return 0;
}

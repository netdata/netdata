#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "common.h"
#include "appconfig.h"
#include "procfile.h"
#include "rrd.h"
#include "plugin_proc.h"

int do_proc_net_dev(int update_every, unsigned long long dt) {
	static procfile *ff = NULL;
	static int enable_new_interfaces = -1, enable_ifb_interfaces = -1;
	static int do_bandwidth = -1, do_packets = -1, do_errors = -1, do_drops = -1, do_fifo = -1, do_compressed = -1, do_events = -1;

	if(dt) {};

	if(!ff) {
		char filename[FILENAME_MAX + 1];
		snprintfz(filename, FILENAME_MAX, "%s%s", global_host_prefix, "/proc/net/dev");
		ff = procfile_open(config_get("plugin:proc:/proc/net/dev", "filename to monitor", filename), " \t,:|", PROCFILE_FLAG_DEFAULT);
	}
	if(!ff) return 1;

	ff = procfile_readall(ff);
	if(!ff) return 0; // we return 0, so that we will retry to open it next time

	if(enable_new_interfaces == -1)	enable_new_interfaces = config_get_boolean_ondemand("plugin:proc:/proc/net/dev", "enable new interfaces detected at runtime", CONFIG_ONDEMAND_ONDEMAND);
	if(enable_ifb_interfaces == -1)	enable_ifb_interfaces = config_get_boolean_ondemand("plugin:proc:/proc/net/dev", "enable ifb interfaces", CONFIG_ONDEMAND_NO);

	if(do_bandwidth == -1)	do_bandwidth 	= config_get_boolean_ondemand("plugin:proc:/proc/net/dev", "bandwidth for all interfaces", CONFIG_ONDEMAND_ONDEMAND);
	if(do_packets == -1)	do_packets 		= config_get_boolean_ondemand("plugin:proc:/proc/net/dev", "packets for all interfaces", CONFIG_ONDEMAND_ONDEMAND);
	if(do_errors == -1)		do_errors 		= config_get_boolean_ondemand("plugin:proc:/proc/net/dev", "errors for all interfaces", CONFIG_ONDEMAND_ONDEMAND);
	if(do_drops == -1)		do_drops 		= config_get_boolean_ondemand("plugin:proc:/proc/net/dev", "drops for all interfaces", CONFIG_ONDEMAND_ONDEMAND);
	if(do_fifo == -1) 		do_fifo 		= config_get_boolean_ondemand("plugin:proc:/proc/net/dev", "fifo for all interfaces", CONFIG_ONDEMAND_ONDEMAND);
	if(do_compressed == -1)	do_compressed 	= config_get_boolean_ondemand("plugin:proc:/proc/net/dev", "compressed packets for all interfaces", CONFIG_ONDEMAND_ONDEMAND);
	if(do_events == -1)		do_events 		= config_get_boolean_ondemand("plugin:proc:/proc/net/dev", "frames, collisions, carrier counters for all interfaces", CONFIG_ONDEMAND_ONDEMAND);

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

		int ddo_bandwidth = do_bandwidth, ddo_packets = do_packets, ddo_errors = do_errors, ddo_drops = do_drops, ddo_fifo = do_fifo, ddo_compressed = do_compressed, ddo_events = do_events;

		int default_enable = enable_new_interfaces;

		// prevent unused interfaces from creating charts
		if(strcmp(iface, "lo") == 0)
			default_enable = 0;
		else {
			int len = strlen(iface);
			if(len >= 4 && strcmp(&iface[len-4], "-ifb") == 0)
				default_enable = enable_ifb_interfaces;
		}

		// check if the user wants it
		{
			char var_name[512 + 1];
			snprintfz(var_name, 512, "plugin:proc:/proc/net/dev:%s", iface);
			default_enable = config_get_boolean_ondemand(var_name, "enabled", default_enable);
			if(default_enable == CONFIG_ONDEMAND_NO) continue;
			if(default_enable == CONFIG_ONDEMAND_ONDEMAND && !rbytes && !tbytes) continue;

			ddo_bandwidth = config_get_boolean_ondemand(var_name, "bandwidth", ddo_bandwidth);
			ddo_packets = config_get_boolean_ondemand(var_name, "packets", ddo_packets);
			ddo_errors = config_get_boolean_ondemand(var_name, "errors", ddo_errors);
			ddo_drops = config_get_boolean_ondemand(var_name, "drops", ddo_drops);
			ddo_fifo = config_get_boolean_ondemand(var_name, "fifo", ddo_fifo);
			ddo_compressed = config_get_boolean_ondemand(var_name, "compressed", ddo_compressed);
			ddo_events = config_get_boolean_ondemand(var_name, "events", ddo_events);

			if(ddo_bandwidth == CONFIG_ONDEMAND_ONDEMAND && rbytes == 0 && tbytes == 0) ddo_bandwidth = 0;
			if(ddo_errors == CONFIG_ONDEMAND_ONDEMAND && rerrors == 0 && terrors == 0) ddo_errors = 0;
			if(ddo_drops == CONFIG_ONDEMAND_ONDEMAND && rdrops == 0 && tdrops == 0) ddo_drops = 0;
			if(ddo_fifo == CONFIG_ONDEMAND_ONDEMAND && rfifo == 0 && tfifo == 0) ddo_fifo = 0;
			if(ddo_compressed == CONFIG_ONDEMAND_ONDEMAND && rcompressed == 0 && tcompressed == 0) ddo_compressed = 0;
			if(ddo_events == CONFIG_ONDEMAND_ONDEMAND && rframe == 0 && tcollisions == 0 && tcarrier == 0) ddo_events = 0;

			// for absolute values, we need to switch the setting to 'yes'
			// to allow it refresh from now on
			// if(ddo_fifo == CONFIG_ONDEMAND_ONDEMAND) config_set(var_name, "fifo", "yes");
		}

		RRDSET *st;

		// --------------------------------------------------------------------

		if(ddo_bandwidth) {
			st = rrdset_find_bytype("net", iface);
			if(!st) {
				st = rrdset_create("net", iface, NULL, iface, "net.net", "Bandwidth", "kilobits/s", 7000, update_every, RRDSET_TYPE_AREA);

				rrddim_add(st, "received", NULL, 8, 1024, RRDDIM_INCREMENTAL);
				rrddim_add(st, "sent", NULL, -8, 1024, RRDDIM_INCREMENTAL);
			}
			else rrdset_next(st);

			rrddim_set(st, "received", rbytes);
			rrddim_set(st, "sent", tbytes);
			rrdset_done(st);
		}

		// --------------------------------------------------------------------

		if(ddo_packets) {
			st = rrdset_find_bytype("net_packets", iface);
			if(!st) {
				st = rrdset_create("net_packets", iface, NULL, iface, "net.packets", "Packets", "packets/s", 7001, update_every, RRDSET_TYPE_LINE);
				st->isdetail = 1;

				rrddim_add(st, "received", NULL, 1, 1, RRDDIM_INCREMENTAL);
				rrddim_add(st, "sent", NULL, -1, 1, RRDDIM_INCREMENTAL);
				rrddim_add(st, "multicast", NULL, 1, 1, RRDDIM_INCREMENTAL);
			}
			else rrdset_next(st);

			rrddim_set(st, "received", rpackets);
			rrddim_set(st, "sent", tpackets);
			rrddim_set(st, "multicast", rmulticast);
			rrdset_done(st);
		}

		// --------------------------------------------------------------------

		if(ddo_errors) {
			st = rrdset_find_bytype("net_errors", iface);
			if(!st) {
				st = rrdset_create("net_errors", iface, NULL, iface, "net.errors", "Interface Errors", "errors/s", 7002, update_every, RRDSET_TYPE_LINE);
				st->isdetail = 1;

				rrddim_add(st, "inbound", NULL, 1, 1, RRDDIM_INCREMENTAL);
				rrddim_add(st, "outbound", NULL, -1, 1, RRDDIM_INCREMENTAL);
			}
			else rrdset_next(st);

			rrddim_set(st, "inbound", rerrors);
			rrddim_set(st, "outbound", terrors);
			rrdset_done(st);
		}

		// --------------------------------------------------------------------

		if(ddo_drops) {
			st = rrdset_find_bytype("net_drops", iface);
			if(!st) {
				st = rrdset_create("net_drops", iface, NULL, iface, "net.drops", "Interface Drops", "drops/s", 7003, update_every, RRDSET_TYPE_LINE);
				st->isdetail = 1;

				rrddim_add(st, "inbound", NULL, 1, 1, RRDDIM_INCREMENTAL);
				rrddim_add(st, "outbound", NULL, -1, 1, RRDDIM_INCREMENTAL);
			}
			else rrdset_next(st);

			rrddim_set(st, "inbound", rdrops);
			rrddim_set(st, "outbound", tdrops);
			rrdset_done(st);
		}

		// --------------------------------------------------------------------

		if(ddo_fifo) {
			st = rrdset_find_bytype("net_fifo", iface);
			if(!st) {
				st = rrdset_create("net_fifo", iface, NULL, iface, "net.fifo", "Interface FIFO Buffer Errors", "errors", 7004, update_every, RRDSET_TYPE_LINE);
				st->isdetail = 1;

				rrddim_add(st, "receive", NULL, 1, 1, RRDDIM_INCREMENTAL);
				rrddim_add(st, "transmit", NULL, -1, 1, RRDDIM_INCREMENTAL);
			}
			else rrdset_next(st);

			rrddim_set(st, "receive", rfifo);
			rrddim_set(st, "transmit", tfifo);
			rrdset_done(st);
		}

		// --------------------------------------------------------------------

		if(ddo_compressed) {
			st = rrdset_find_bytype("net_compressed", iface);
			if(!st) {
				st = rrdset_create("net_compressed", iface, NULL, iface, "net.compressed", "Compressed Packets", "packets/s", 7005, update_every, RRDSET_TYPE_LINE);
				st->isdetail = 1;

				rrddim_add(st, "received", NULL, 1, 1, RRDDIM_INCREMENTAL);
				rrddim_add(st, "sent", NULL, -1, 1, RRDDIM_INCREMENTAL);
			}
			else rrdset_next(st);

			rrddim_set(st, "received", rcompressed);
			rrddim_set(st, "sent", tcompressed);
			rrdset_done(st);
		}

		// --------------------------------------------------------------------

		if(ddo_events) {
			st = rrdset_find_bytype("net_events", iface);
			if(!st) {
				st = rrdset_create("net_events", iface, NULL, iface, "net.events", "Network Interface Events", "events/s", 7006, update_every, RRDSET_TYPE_LINE);
				st->isdetail = 1;

				rrddim_add(st, "frames", NULL, 1, 1, RRDDIM_INCREMENTAL);
				rrddim_add(st, "collisions", NULL, -1, 1, RRDDIM_INCREMENTAL);
				rrddim_add(st, "carrier", NULL, -1, 1, RRDDIM_INCREMENTAL);
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

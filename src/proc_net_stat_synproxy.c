#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
#include <stdio.h>
#include <stdlib.h>

#include "common.h"
#include "appconfig.h"
#include "procfile.h"
#include "rrd.h"
#include "plugin_proc.h"
#include "log.h"

#define RRD_TYPE_NET_STAT_NETFILTER			"netfilter"
#define RRD_TYPE_NET_STAT_SYNPROXY 			"synproxy"
#define RRD_TYPE_NET_STAT_SYNPROXY_LEN		strlen(RRD_TYPE_NET_STAT_SYNPROXY)

int do_proc_net_stat_synproxy(int update_every, unsigned long long dt) {
	static int do_entries = -1, do_cookies = -1, do_syns = -1, do_reopened = -1;
	static procfile *ff = NULL;

	if(do_entries == -1)	do_entries 	= config_get_boolean_ondemand("plugin:proc:/proc/net/stat/synproxy", "SYNPROXY entries", CONFIG_ONDEMAND_ONDEMAND);
	if(do_cookies == -1)	do_cookies 	= config_get_boolean_ondemand("plugin:proc:/proc/net/stat/synproxy", "SYNPROXY cookies", CONFIG_ONDEMAND_ONDEMAND);
	if(do_syns == -1)		do_syns 	= config_get_boolean_ondemand("plugin:proc:/proc/net/stat/synproxy", "SYNPROXY SYN received", CONFIG_ONDEMAND_ONDEMAND);
	if(do_reopened == -1)	do_reopened = config_get_boolean_ondemand("plugin:proc:/proc/net/stat/synproxy", "SYNPROXY connections reopened", CONFIG_ONDEMAND_ONDEMAND);

	if(dt) {};

	if(!ff) {
		char filename[FILENAME_MAX + 1];
		snprintfz(filename, FILENAME_MAX, "%s%s", global_host_prefix, "/proc/net/stat/synproxy");
		ff = procfile_open(config_get("plugin:proc:/proc/net/stat/synproxy", "filename to monitor", filename), " \t,:|", PROCFILE_FLAG_DEFAULT);
	}
	if(!ff) return 1;

	ff = procfile_readall(ff);
	if(!ff) return 0; // we return 0, so that we will retry to open it next time

	// make sure we have 3 lines
	unsigned long lines = procfile_lines(ff), l;
	if(lines < 2) {
		error("/proc/net/stat/synproxy has %d lines, expected no less than 2. Disabling it.", lines);
		return 1;
	}

	unsigned long long entries = 0, syn_received = 0, cookie_invalid = 0, cookie_valid = 0, cookie_retrans = 0, conn_reopened = 0;

	// synproxy gives its values per CPU
	for(l = 1; l < lines ;l++) {
		int words = procfile_linewords(ff, l);
		if(words < 6) continue;

		entries 		+= strtoull(procfile_lineword(ff, l, 0), NULL, 16);
		syn_received 	+= strtoull(procfile_lineword(ff, l, 1), NULL, 16);
		cookie_invalid 	+= strtoull(procfile_lineword(ff, l, 2), NULL, 16);
		cookie_valid 	+= strtoull(procfile_lineword(ff, l, 3), NULL, 16);
		cookie_retrans 	+= strtoull(procfile_lineword(ff, l, 4), NULL, 16);
		conn_reopened 	+= strtoull(procfile_lineword(ff, l, 5), NULL, 16);
	}

	unsigned long long events = entries + syn_received + cookie_invalid + cookie_valid + cookie_retrans + conn_reopened;

	RRDSET *st;

	// --------------------------------------------------------------------

	if((do_entries == CONFIG_ONDEMAND_ONDEMAND && events) || do_entries == CONFIG_ONDEMAND_YES) {
		do_entries = CONFIG_ONDEMAND_YES;

		st = rrdset_find(RRD_TYPE_NET_STAT_NETFILTER "." RRD_TYPE_NET_STAT_SYNPROXY "_entries");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_STAT_NETFILTER, RRD_TYPE_NET_STAT_SYNPROXY "_entries", NULL, RRD_TYPE_NET_STAT_SYNPROXY, NULL, "SYNPROXY Entries Used", "entries", 1004, update_every, RRDSET_TYPE_LINE);

			rrddim_add(st, "entries", NULL, 1, 1, RRDDIM_ABSOLUTE);
		}
		else rrdset_next(st);

		rrddim_set(st, "entries", entries);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if((do_syns == CONFIG_ONDEMAND_ONDEMAND && events) || do_syns == CONFIG_ONDEMAND_YES) {
		do_syns = CONFIG_ONDEMAND_YES;

		st = rrdset_find(RRD_TYPE_NET_STAT_NETFILTER "." RRD_TYPE_NET_STAT_SYNPROXY "_syn_received");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_STAT_NETFILTER, RRD_TYPE_NET_STAT_SYNPROXY "_syn_received", NULL, RRD_TYPE_NET_STAT_SYNPROXY, NULL, "SYNPROXY SYN Packets received", "SYN/s", 1001, update_every, RRDSET_TYPE_LINE);

			rrddim_add(st, "received", NULL, 1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "received", syn_received);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if((do_reopened == CONFIG_ONDEMAND_ONDEMAND && events) || do_reopened == CONFIG_ONDEMAND_YES) {
		do_reopened = CONFIG_ONDEMAND_YES;

		st = rrdset_find(RRD_TYPE_NET_STAT_NETFILTER "." RRD_TYPE_NET_STAT_SYNPROXY "_conn_reopened");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_STAT_NETFILTER, RRD_TYPE_NET_STAT_SYNPROXY "_conn_reopened", NULL, RRD_TYPE_NET_STAT_SYNPROXY, NULL, "SYNPROXY Connections Reopened", "connections/s", 1003, update_every, RRDSET_TYPE_LINE);

			rrddim_add(st, "reopened", NULL, 1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "reopened", conn_reopened);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if((do_cookies == CONFIG_ONDEMAND_ONDEMAND && events) || do_cookies == CONFIG_ONDEMAND_YES) {
		do_cookies = CONFIG_ONDEMAND_YES;

		st = rrdset_find(RRD_TYPE_NET_STAT_NETFILTER "." RRD_TYPE_NET_STAT_SYNPROXY "_cookies");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_STAT_NETFILTER, RRD_TYPE_NET_STAT_SYNPROXY "_cookies", NULL, RRD_TYPE_NET_STAT_SYNPROXY, NULL, "SYNPROXY TCP Cookies", "cookies/s", 1002, update_every, RRDSET_TYPE_LINE);

			rrddim_add(st, "valid", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "invalid", NULL, -1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "retransmits", NULL, 1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "valid", cookie_valid);
		rrddim_set(st, "invalid", cookie_invalid);
		rrddim_set(st, "retransmits", cookie_retrans);
		rrdset_done(st);
	}

	return 0;
}

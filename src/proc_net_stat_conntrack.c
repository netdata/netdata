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

#define RRD_TYPE_NET_STAT_NETFILTER		"netfilter"
#define RRD_TYPE_NET_STAT_CONNTRACK 	"conntrack"
#define RRD_TYPE_NET_STAT_CONNTRACK_LEN	strlen(RRD_TYPE_NET_STAT_CONNTRACK)

int do_proc_net_stat_conntrack(int update_every, unsigned long long dt) {
	static procfile *ff = NULL;
	static int do_sockets = -1, do_new = -1, do_changes = -1, do_expect = -1, do_search = -1, do_errors = -1;

	if(do_sockets == -1)	do_sockets = config_get_boolean("plugin:proc:/proc/net/stat/nf_conntrack", "netfilter connections", 1);
	if(do_new == -1)		do_new = config_get_boolean("plugin:proc:/proc/net/stat/nf_conntrack", "netfilter new connections", 1);
	if(do_changes == -1)	do_changes = config_get_boolean("plugin:proc:/proc/net/stat/nf_conntrack", "netfilter connection changes", 1);
	if(do_expect == -1)		do_expect = config_get_boolean("plugin:proc:/proc/net/stat/nf_conntrack", "netfilter connection expectations", 1);
	if(do_search == -1)		do_search = config_get_boolean("plugin:proc:/proc/net/stat/nf_conntrack", "netfilter connection searches", 1);
	if(do_errors == -1)		do_errors = config_get_boolean("plugin:proc:/proc/net/stat/nf_conntrack", "netfilter errors", 1);

	if(dt) {};

	if(!ff) {
		char filename[FILENAME_MAX + 1];
		snprintfz(filename, FILENAME_MAX, "%s%s", global_host_prefix, "/proc/net/stat/nf_conntrack");
		ff = procfile_open(config_get("plugin:proc:/proc/net/stat/nf_conntrack", "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
	}
	if(!ff) return 1;

	ff = procfile_readall(ff);
	if(!ff) return 0; // we return 0, so that we will retry to open it next time

	uint32_t lines = procfile_lines(ff), l;
	uint32_t words;

	unsigned long long aentries = 0, asearched = 0, afound = 0, anew = 0, ainvalid = 0, aignore = 0, adelete = 0, adelete_list = 0,
		ainsert = 0, ainsert_failed = 0, adrop = 0, aearly_drop = 0, aicmp_error = 0, aexpect_new = 0, aexpect_create = 0, aexpect_delete = 0, asearch_restart = 0;

	for(l = 1; l < lines ;l++) {
		words = procfile_linewords(ff, l);
		if(words < 17) {
			if(words) error("Cannot read /proc/net/stat/nf_conntrack line. Expected 17 params, read %d.", words);
			continue;
		}

		unsigned long long tentries = 0, tsearched = 0, tfound = 0, tnew = 0, tinvalid = 0, tignore = 0, tdelete = 0, tdelete_list = 0, tinsert = 0, tinsert_failed = 0, tdrop = 0, tearly_drop = 0, ticmp_error = 0, texpect_new = 0, texpect_create = 0, texpect_delete = 0, tsearch_restart = 0;

		tentries		= strtoull(procfile_lineword(ff, l, 0), NULL, 16);
		tsearched		= strtoull(procfile_lineword(ff, l, 1), NULL, 16);
		tfound			= strtoull(procfile_lineword(ff, l, 2), NULL, 16);
		tnew			= strtoull(procfile_lineword(ff, l, 3), NULL, 16);
		tinvalid		= strtoull(procfile_lineword(ff, l, 4), NULL, 16);
		tignore			= strtoull(procfile_lineword(ff, l, 5), NULL, 16);
		tdelete			= strtoull(procfile_lineword(ff, l, 6), NULL, 16);
		tdelete_list	= strtoull(procfile_lineword(ff, l, 7), NULL, 16);
		tinsert			= strtoull(procfile_lineword(ff, l, 8), NULL, 16);
		tinsert_failed	= strtoull(procfile_lineword(ff, l, 9), NULL, 16);
		tdrop			= strtoull(procfile_lineword(ff, l, 10), NULL, 16);
		tearly_drop		= strtoull(procfile_lineword(ff, l, 11), NULL, 16);
		ticmp_error		= strtoull(procfile_lineword(ff, l, 12), NULL, 16);
		texpect_new		= strtoull(procfile_lineword(ff, l, 13), NULL, 16);
		texpect_create	= strtoull(procfile_lineword(ff, l, 14), NULL, 16);
		texpect_delete	= strtoull(procfile_lineword(ff, l, 15), NULL, 16);
		tsearch_restart	= strtoull(procfile_lineword(ff, l, 16), NULL, 16);

		if(!aentries) aentries =  tentries;

		// sum all the cpus together
		asearched 			+= tsearched;		// conntrack.search
		afound 				+= tfound;			// conntrack.search
		anew 				+= tnew;			// conntrack.new
		ainvalid 			+= tinvalid;		// conntrack.new
		aignore 			+= tignore;			// conntrack.new
		adelete 			+= tdelete;			// conntrack.changes
		adelete_list 		+= tdelete_list;	// conntrack.changes
		ainsert 			+= tinsert;			// conntrack.changes
		ainsert_failed 		+= tinsert_failed;	// conntrack.errors
		adrop 				+= tdrop;			// conntrack.errors
		aearly_drop 		+= tearly_drop;		// conntrack.errors
		aicmp_error 		+= ticmp_error;		// conntrack.errors
		aexpect_new 		+= texpect_new;		// conntrack.expect
		aexpect_create 		+= texpect_create;	// conntrack.expect
		aexpect_delete 		+= texpect_delete;	// conntrack.expect
		asearch_restart 	+= tsearch_restart;	// conntrack.search
	}

	RRDSET *st;

	// --------------------------------------------------------------------

	if(do_sockets) {
		st = rrdset_find(RRD_TYPE_NET_STAT_NETFILTER "." RRD_TYPE_NET_STAT_CONNTRACK "_sockets");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_STAT_NETFILTER, RRD_TYPE_NET_STAT_CONNTRACK "_sockets", NULL, RRD_TYPE_NET_STAT_CONNTRACK, NULL, "Connection Tracker Connections", "active connections", 1000, update_every, RRDSET_TYPE_LINE);

			rrddim_add(st, "connections", NULL, 1, 1, RRDDIM_ABSOLUTE);
		}
		else rrdset_next(st);

		rrddim_set(st, "connections", aentries);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_new) {
		st = rrdset_find(RRD_TYPE_NET_STAT_NETFILTER "." RRD_TYPE_NET_STAT_CONNTRACK "_new");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_STAT_NETFILTER, RRD_TYPE_NET_STAT_CONNTRACK "_new", NULL, RRD_TYPE_NET_STAT_CONNTRACK, NULL, "Connection Tracker New Connections", "connections/s", 1001, update_every, RRDSET_TYPE_LINE);

			rrddim_add(st, "new", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "ignore", NULL, -1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "invalid", NULL, -1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "new", anew);
		rrddim_set(st, "ignore", aignore);
		rrddim_set(st, "invalid", ainvalid);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_changes) {
		st = rrdset_find(RRD_TYPE_NET_STAT_NETFILTER "." RRD_TYPE_NET_STAT_CONNTRACK "_changes");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_STAT_NETFILTER, RRD_TYPE_NET_STAT_CONNTRACK "_changes", NULL, RRD_TYPE_NET_STAT_CONNTRACK, NULL, "Connection Tracker Changes", "changes/s", 1002, update_every, RRDSET_TYPE_LINE);
			st->isdetail = 1;

			rrddim_add(st, "inserted", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "deleted", NULL, -1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "delete_list", NULL, -1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "inserted", ainsert);
		rrddim_set(st, "deleted", adelete);
		rrddim_set(st, "delete_list", adelete_list);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_expect) {
		st = rrdset_find(RRD_TYPE_NET_STAT_NETFILTER "." RRD_TYPE_NET_STAT_CONNTRACK "_expect");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_STAT_NETFILTER, RRD_TYPE_NET_STAT_CONNTRACK "_expect", NULL, RRD_TYPE_NET_STAT_CONNTRACK, NULL, "Connection Tracker Expectations", "expectations/s", 1003, update_every, RRDSET_TYPE_LINE);
			st->isdetail = 1;

			rrddim_add(st, "created", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "deleted", NULL, -1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "new", NULL, 1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "created", aexpect_create);
		rrddim_set(st, "deleted", aexpect_delete);
		rrddim_set(st, "new", aexpect_new);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_search) {
		st = rrdset_find(RRD_TYPE_NET_STAT_NETFILTER "." RRD_TYPE_NET_STAT_CONNTRACK "_search");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_STAT_NETFILTER, RRD_TYPE_NET_STAT_CONNTRACK "_search", NULL, RRD_TYPE_NET_STAT_CONNTRACK, NULL, "Connection Tracker Searches", "searches/s", 1010, update_every, RRDSET_TYPE_LINE);
			st->isdetail = 1;

			rrddim_add(st, "searched", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "restarted", NULL, -1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "found", NULL, 1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "searched", asearched);
		rrddim_set(st, "restarted", asearch_restart);
		rrddim_set(st, "found", afound);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_errors) {
		st = rrdset_find(RRD_TYPE_NET_STAT_NETFILTER "." RRD_TYPE_NET_STAT_CONNTRACK "_errors");
		if(!st) {
			st = rrdset_create(RRD_TYPE_NET_STAT_NETFILTER, RRD_TYPE_NET_STAT_CONNTRACK "_errors", NULL, RRD_TYPE_NET_STAT_CONNTRACK, NULL, "Connection Tracker Errors", "events/s", 1005, update_every, RRDSET_TYPE_LINE);
			st->isdetail = 1;

			rrddim_add(st, "icmp_error", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "insert_failed", NULL, -1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "drop", NULL, -1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "early_drop", NULL, -1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "icmp_error", aicmp_error);
		rrddim_set(st, "insert_failed", ainsert_failed);
		rrddim_set(st, "drop", adrop);
		rrddim_set(st, "early_drop", aearly_drop);
		rrdset_done(st);
	}

	return 0;
}

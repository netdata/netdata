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

struct nfsd_procs {
	char name[30];
	unsigned long long proc2;
	unsigned long long proc3;
	unsigned long long proc4;
	int present2;
	int present3;
	int present4;
};

struct nfsd_procs nfsd_proc_values[] = {
	{ "null", 0ULL, 0ULL, 0ULL, 0, 0, 0 },
	{ "getattr", 0ULL, 0ULL, 0ULL, 0, 0, 0 },
	{ "setattr", 0ULL, 0ULL, 0ULL, 0, 0, 0 },
	{ "lookup", 0ULL, 0ULL, 0ULL, 0, 0, 0 },
	{ "access", 0ULL, 0ULL, 0ULL, 0, 0, 0 },
	{ "readlink", 0ULL, 0ULL, 0ULL, 0, 0, 0 },
	{ "read", 0ULL, 0ULL, 0ULL, 0, 0, 0 },
	{ "write", 0ULL, 0ULL, 0ULL, 0, 0, 0 },
	{ "create", 0ULL, 0ULL, 0ULL, 0, 0, 0 },
	{ "mkdir", 0ULL, 0ULL, 0ULL, 0, 0, 0 },
	{ "symlink", 0ULL, 0ULL, 0ULL, 0, 0, 0 },
	{ "mknod", 0ULL, 0ULL, 0ULL, 0, 0, 0 },
	{ "remove", 0ULL, 0ULL, 0ULL, 0, 0, 0 },
	{ "rmdir", 0ULL, 0ULL, 0ULL, 0, 0, 0 },
	{ "rename", 0ULL, 0ULL, 0ULL, 0, 0, 0 },
	{ "link", 0ULL, 0ULL, 0ULL, 0, 0, 0 },
	{ "readdir", 0ULL, 0ULL, 0ULL, 0, 0, 0 },
	{ "readdirplus", 0ULL, 0ULL, 0ULL, 0, 0, 0 },
	{ "fsstat", 0ULL, 0ULL, 0ULL, 0, 0, 0 },
	{ "fsinfo", 0ULL, 0ULL, 0ULL, 0, 0, 0 },
	{ "pathconf", 0ULL, 0ULL, 0ULL, 0, 0, 0 },
	{ "commit", 0ULL, 0ULL, 0ULL, 0, 0, 0 },
	{ "", 0ULL, 0ULL, 0ULL, 0, 0, 0 },
};

struct nfsd4_ops {
	char name[30];
	unsigned long long value;
	int present;
};

struct nfsd4_ops nfsd4_ops_values[] = {
	{ "access", 0ULL, 0},
	{ "close", 0ULL, 0},
	{ "commit", 0ULL, 0},
	{ "create", 0ULL, 0},
	{ "delegpurge", 0ULL, 0},
	{ "delegreturn", 0ULL, 0},
	{ "getattr", 0ULL, 0},
	{ "getfh", 0ULL, 0},
	{ "link", 0ULL, 0},
	{ "lock", 0ULL, 0},
	{ "lockt", 0ULL, 0},
	{ "locku", 0ULL, 0},
	{ "lookup", 0ULL, 0},
	{ "lookupp", 0ULL, 0},
	{ "nverify", 0ULL, 0},
	{ "open", 0ULL, 0},
	{ "openattr", 0ULL, 0},
	{ "open_confirm", 0ULL, 0},
	{ "open_downgrade", 0ULL, 0},
	{ "putfh", 0ULL, 0},
	{ "putpubfh", 0ULL, 0},
	{ "putrootfh", 0ULL, 0},
	{ "read", 0ULL, 0},
	{ "readdir", 0ULL, 0},
	{ "readlink", 0ULL, 0},
	{ "remove", 0ULL, 0},
	{ "rename", 0ULL, 0},
	{ "renew", 0ULL, 0},
	{ "restorefh", 0ULL, 0},
	{ "savefh", 0ULL, 0},
	{ "secinfo", 0ULL, 0},
	{ "setattr", 0ULL, 0},
	{ "setclientid", 0ULL, 0},
	{ "setclientid_confirm", 0ULL, 0},
	{ "verify", 0ULL, 0},
	{ "write", 0ULL, 0},
	{ "release_lockowner", 0ULL, 0},

	/* nfs41 */
	{ "backchannel_ctl", 0ULL, 0},
	{ "bind_conn_to_session", 0ULL, 0},
	{ "exchange_id", 0ULL, 0},
	{ "create_session", 0ULL, 0},
	{ "destroy_session", 0ULL, 0},
	{ "free_stateid", 0ULL, 0},
	{ "get_dir_delegation", 0ULL, 0},
	{ "getdeviceinfo", 0ULL, 0},
	{ "getdevicelist", 0ULL, 0},
	{ "layoutcommit", 0ULL, 0},
	{ "layoutget", 0ULL, 0},
	{ "layoutreturn", 0ULL, 0},
	{ "secinfo_no_name", 0ULL, 0},
	{ "sequence", 0ULL, 0},
	{ "set_ssv", 0ULL, 0},
	{ "test_stateid", 0ULL, 0},
	{ "want_delegation", 0ULL, 0},
	{ "destroy_clientid", 0ULL, 0},
	{ "reclaim_complete", 0ULL, 0},

	/* nfs42 */
	{ "allocate", 0ULL, 0},
	{ "copy", 0ULL, 0},
	{ "copy_notify", 0ULL, 0},
	{ "deallocate", 0ULL, 0},
	{ "io_advise", 0ULL, 0},
	{ "layouterror", 0ULL, 0},
	{ "layoutstats", 0ULL, 0},
	{ "offload_cancel", 0ULL, 0},
	{ "offload_status", 0ULL, 0},
	{ "read_plus", 0ULL, 0},
	{ "seek", 0ULL, 0},
	{ "write_same", 0ULL, 0},

	/* termination */
	{ "", 0ULL, 0 }
};


int do_proc_net_rpc_nfsd(int update_every, unsigned long long dt) {
	static procfile *ff = NULL;
	static int do_rc = -1, do_fh = -1, do_io = -1, do_th = -1, do_ra = -1, do_net = -1, do_rpc = -1, do_proc2 = -1, do_proc3 = -1, do_proc4 = -1, do_proc4ops = -1;
	static int ra_warning = 0, th_warning = 0, proc2_warning = 0, proc3_warning = 0, proc4_warning = 0, proc4ops_warning = 0;

	if(dt) {};

	if(!ff) {
		char filename[FILENAME_MAX + 1];
		snprintfz(filename, FILENAME_MAX, "%s%s", global_host_prefix, "/proc/net/rpc/nfsd");
		ff = procfile_open(config_get("plugin:proc:/proc/net/rpc/nfsd", "filename to monitor", filename), " \t", PROCFILE_FLAG_DEFAULT);
	}
	if(!ff) return 1;

	ff = procfile_readall(ff);
	if(!ff) return 0; // we return 0, so that we will retry to open it next time

	if(do_rc == -1) do_rc = config_get_boolean("plugin:proc:/proc/net/rpc/nfsd", "read cache", 1);
	if(do_fh == -1) do_fh = config_get_boolean("plugin:proc:/proc/net/rpc/nfsd", "file handles", 1);
	if(do_io == -1) do_io = config_get_boolean("plugin:proc:/proc/net/rpc/nfsd", "I/O", 1);
	if(do_th == -1) do_th = config_get_boolean("plugin:proc:/proc/net/rpc/nfsd", "threads", 1);
	if(do_ra == -1) do_ra = config_get_boolean("plugin:proc:/proc/net/rpc/nfsd", "read ahead", 1);
	if(do_net == -1) do_net = config_get_boolean("plugin:proc:/proc/net/rpc/nfsd", "network", 1);
	if(do_rpc == -1) do_rpc = config_get_boolean("plugin:proc:/proc/net/rpc/nfsd", "rpc", 1);
	if(do_proc2 == -1) do_proc2 = config_get_boolean("plugin:proc:/proc/net/rpc/nfsd", "NFS v2 procedures", 1);
	if(do_proc3 == -1) do_proc3 = config_get_boolean("plugin:proc:/proc/net/rpc/nfsd", "NFS v3 procedures", 1);
	if(do_proc4 == -1) do_proc4 = config_get_boolean("plugin:proc:/proc/net/rpc/nfsd", "NFS v4 procedures", 1);
	if(do_proc4ops == -1) do_proc4ops = config_get_boolean("plugin:proc:/proc/net/rpc/nfsd", "NFS v4 operations", 1);

	// if they are enabled, reset them to 1
	// later we do them =2 to avoid doing strcmp for all lines
	if(do_rc) do_rc = 1;
	if(do_fh) do_fh = 1;
	if(do_io) do_io = 1;
	if(do_th) do_th = 1;
	if(do_ra) do_ra = 1;
	if(do_net) do_net = 1;
	if(do_rpc) do_rpc = 1;
	if(do_proc2) do_proc2 = 1;
	if(do_proc3) do_proc3 = 1;
	if(do_proc4) do_proc4 = 1;
	if(do_proc4ops) do_proc4ops = 1;

	uint32_t lines = procfile_lines(ff), l;
	uint32_t words;

	char *type;
	unsigned long long rc_hits = 0, rc_misses = 0, rc_nocache = 0;
	unsigned long long fh_stale = 0, fh_total_lookups = 0, fh_anonymous_lookups = 0, fh_dir_not_in_dcache = 0, fh_non_dir_not_in_dcache = 0;
	unsigned long long io_read = 0, io_write = 0;
	unsigned long long th_threads = 0, th_fullcnt = 0, th_hist10 = 0, th_hist20 = 0, th_hist30 = 0, th_hist40 = 0, th_hist50 = 0, th_hist60 = 0, th_hist70 = 0, th_hist80 = 0, th_hist90 = 0, th_hist100 = 0;
	unsigned long long ra_size = 0, ra_hist10 = 0, ra_hist20 = 0, ra_hist30 = 0, ra_hist40 = 0, ra_hist50 = 0, ra_hist60 = 0, ra_hist70 = 0, ra_hist80 = 0, ra_hist90 = 0, ra_hist100 = 0, ra_none = 0;
	unsigned long long net_count = 0, net_udp_count = 0, net_tcp_count = 0, net_tcp_connections = 0;
	unsigned long long rpc_count = 0, rpc_bad_format = 0, rpc_bad_auth = 0, rpc_bad_client = 0;

	for(l = 0; l < lines ;l++) {
		words = procfile_linewords(ff, l);
		if(!words) continue;

		type		= procfile_lineword(ff, l, 0);

		if(do_rc == 1 && strcmp(type, "rc") == 0) {
			if(words < 4) {
				error("%s line of /proc/net/rpc/nfsd has %d words, expected %d", type, words, 4);
				continue;
			}

			rc_hits = strtoull(procfile_lineword(ff, l, 1), NULL, 10);
			rc_misses = strtoull(procfile_lineword(ff, l, 2), NULL, 10);
			rc_nocache = strtoull(procfile_lineword(ff, l, 3), NULL, 10);

			unsigned long long sum = rc_hits + rc_misses + rc_nocache;
			if(sum == 0ULL) do_rc = -1;
			else do_rc = 2;
		}
		else if(do_fh == 1 && strcmp(type, "fh") == 0) {
			if(words < 6) {
				error("%s line of /proc/net/rpc/nfsd has %d words, expected %d", type, words, 6);
				continue;
			}

			fh_stale = strtoull(procfile_lineword(ff, l, 1), NULL, 10);
			fh_total_lookups = strtoull(procfile_lineword(ff, l, 2), NULL, 10);
			fh_anonymous_lookups = strtoull(procfile_lineword(ff, l, 3), NULL, 10);
			fh_dir_not_in_dcache = strtoull(procfile_lineword(ff, l, 4), NULL, 10);
			fh_non_dir_not_in_dcache = strtoull(procfile_lineword(ff, l, 5), NULL, 10);

			unsigned long long sum = fh_stale + fh_total_lookups + fh_anonymous_lookups + fh_dir_not_in_dcache + fh_non_dir_not_in_dcache;
			if(sum == 0ULL) do_fh = -1;
			else do_fh = 2;
		}
		else if(do_io == 1 && strcmp(type, "io") == 0) {
			if(words < 3) {
				error("%s line of /proc/net/rpc/nfsd has %d words, expected %d", type, words, 3);
				continue;
			}

			io_read = strtoull(procfile_lineword(ff, l, 1), NULL, 10);
			io_write = strtoull(procfile_lineword(ff, l, 2), NULL, 10);

			unsigned long long sum = io_read + io_write;
			if(sum == 0ULL) do_io = -1;
			else do_io = 2;
		}
		else if(do_th == 1 && strcmp(type, "th") == 0) {
			if(words < 13) {
				error("%s line of /proc/net/rpc/nfsd has %d words, expected %d", type, words, 13);
				continue;
			}

			th_threads = strtoull(procfile_lineword(ff, l, 1), NULL, 10);
			th_fullcnt = strtoull(procfile_lineword(ff, l, 2), NULL, 10);
			th_hist10 = (unsigned long long)(atof(procfile_lineword(ff, l, 3)) * 1000.0);
			th_hist20 = (unsigned long long)(atof(procfile_lineword(ff, l, 4)) * 1000.0);
			th_hist30 = (unsigned long long)(atof(procfile_lineword(ff, l, 5)) * 1000.0);
			th_hist40 = (unsigned long long)(atof(procfile_lineword(ff, l, 6)) * 1000.0);
			th_hist50 = (unsigned long long)(atof(procfile_lineword(ff, l, 7)) * 1000.0);
			th_hist60 = (unsigned long long)(atof(procfile_lineword(ff, l, 8)) * 1000.0);
			th_hist70 = (unsigned long long)(atof(procfile_lineword(ff, l, 9)) * 1000.0);
			th_hist80 = (unsigned long long)(atof(procfile_lineword(ff, l, 10)) * 1000.0);
			th_hist90 = (unsigned long long)(atof(procfile_lineword(ff, l, 11)) * 1000.0);
			th_hist100 = (unsigned long long)(atof(procfile_lineword(ff, l, 12)) * 1000.0);

			// threads histogram has been disabled on recent kernels
			// http://permalink.gmane.org/gmane.linux.nfs/24528
			unsigned long long sum = th_hist10 + th_hist20 + th_hist30 + th_hist40 + th_hist50 + th_hist60 + th_hist70 + th_hist80 + th_hist90 + th_hist100;
			if(sum == 0ULL) {
				if(!th_warning) {
					info("Disabling /proc/net/rpc/nfsd threads histogram. It seems unused on this machine. It will be enabled automatically when found with data in it.");
					th_warning = 1;
				}
				do_th = -1;
			}
			else do_th = 2;
		}
		else if(do_ra == 1 && strcmp(type, "ra") == 0) {
			if(words < 13) {
				error("%s line of /proc/net/rpc/nfsd has %d words, expected %d", type, words, 13);
				continue;
			}

			ra_size = strtoull(procfile_lineword(ff, l, 1), NULL, 10);
			ra_hist10 = strtoull(procfile_lineword(ff, l, 2), NULL, 10);
			ra_hist20 = strtoull(procfile_lineword(ff, l, 3), NULL, 10);
			ra_hist30 = strtoull(procfile_lineword(ff, l, 4), NULL, 10);
			ra_hist40 = strtoull(procfile_lineword(ff, l, 5), NULL, 10);
			ra_hist50 = strtoull(procfile_lineword(ff, l, 6), NULL, 10);
			ra_hist60 = strtoull(procfile_lineword(ff, l, 7), NULL, 10);
			ra_hist70 = strtoull(procfile_lineword(ff, l, 8), NULL, 10);
			ra_hist80 = strtoull(procfile_lineword(ff, l, 9), NULL, 10);
			ra_hist90 = strtoull(procfile_lineword(ff, l, 10), NULL, 10);
			ra_hist100 = strtoull(procfile_lineword(ff, l, 11), NULL, 10);
			ra_none = strtoull(procfile_lineword(ff, l, 12), NULL, 10);

			unsigned long long sum = ra_hist10 + ra_hist20 + ra_hist30 + ra_hist40 + ra_hist50 + ra_hist60 + ra_hist70 + ra_hist80 + ra_hist90 + ra_hist100 + ra_none;
			if(sum == 0ULL) {
				if(!ra_warning) {
					info("Disabling /proc/net/rpc/nfsd read ahead histogram. It seems unused on this machine. It will be enabled automatically when found with data in it.");
					ra_warning = 1;
				}
				do_ra = -1;
			}
			else do_ra = 2;
		}
		else if(do_net == 1 && strcmp(type, "net") == 0) {
			if(words < 5) {
				error("%s line of /proc/net/rpc/nfsd has %d words, expected %d", type, words, 5);
				continue;
			}

			net_count = strtoull(procfile_lineword(ff, l, 1), NULL, 10);
			net_udp_count = strtoull(procfile_lineword(ff, l, 2), NULL, 10);
			net_tcp_count = strtoull(procfile_lineword(ff, l, 3), NULL, 10);
			net_tcp_connections = strtoull(procfile_lineword(ff, l, 4), NULL, 10);

			unsigned long long sum = net_count + net_udp_count + net_tcp_count + net_tcp_connections;
			if(sum == 0ULL) do_net = -1;
			else do_net = 2;
		}
		else if(do_rpc == 1 && strcmp(type, "rpc") == 0) {
			if(words < 6) {
				error("%s line of /proc/net/rpc/nfsd has %d words, expected %d", type, words, 6);
				continue;
			}

			rpc_count = strtoull(procfile_lineword(ff, l, 1), NULL, 10);
			rpc_bad_format = strtoull(procfile_lineword(ff, l, 2), NULL, 10);
			rpc_bad_auth = strtoull(procfile_lineword(ff, l, 3), NULL, 10);
			rpc_bad_client = strtoull(procfile_lineword(ff, l, 4), NULL, 10);

			unsigned long long sum = rpc_count + rpc_bad_format + rpc_bad_auth + rpc_bad_client;
			if(sum == 0ULL) do_rpc = -1;
			else do_rpc = 2;
		}
		else if(do_proc2 == 1 && strcmp(type, "proc2") == 0) {
			// the first number is the count of numbers present
			// so we start for word 2

			unsigned long long sum = 0;
			unsigned int i, j;
			for(i = 0, j = 2; j < words && nfsd_proc_values[i].name[0] ; i++, j++) {
				nfsd_proc_values[i].proc2 = strtoull(procfile_lineword(ff, l, j), NULL, 10);
				nfsd_proc_values[i].present2 = 1;
				sum += nfsd_proc_values[i].proc2;
			}

			if(sum == 0ULL) {
				if(!proc2_warning) {
					error("Disabling /proc/net/rpc/nfsd v2 procedure calls chart. It seems unused on this machine. It will be enabled automatically when found with data in it.");
					proc2_warning = 1;
				}
				do_proc2 = 0;
			}
			else do_proc2 = 2;
		}
		else if(do_proc3 == 1 && strcmp(type, "proc3") == 0) {
			// the first number is the count of numbers present
			// so we start for word 2

			unsigned long long sum = 0;
			unsigned int i, j;
			for(i = 0, j = 2; j < words && nfsd_proc_values[i].name[0] ; i++, j++) {
				nfsd_proc_values[i].proc3 = strtoull(procfile_lineword(ff, l, j), NULL, 10);
				nfsd_proc_values[i].present3 = 1;
				sum += nfsd_proc_values[i].proc3;
			}

			if(sum == 0ULL) {
				if(!proc3_warning) {
					info("Disabling /proc/net/rpc/nfsd v3 procedure calls chart. It seems unused on this machine. It will be enabled automatically when found with data in it.");
					proc3_warning = 1;
				}
				do_proc3 = 0;
			}
			else do_proc3 = 2;
		}
		else if(do_proc4 == 1 && strcmp(type, "proc4") == 0) {
			// the first number is the count of numbers present
			// so we start for word 2

			unsigned long long sum = 0;
			unsigned int i, j;
			for(i = 0, j = 2; j < words && nfsd_proc_values[i].name[0] ; i++, j++) {
				nfsd_proc_values[i].proc4 = strtoull(procfile_lineword(ff, l, j), NULL, 10);
				nfsd_proc_values[i].present4 = 1;
				sum += nfsd_proc_values[i].proc4;
			}

			if(sum == 0ULL) {
				if(!proc4_warning) {
					info("Disabling /proc/net/rpc/nfsd v4 procedure calls chart. It seems unused on this machine. It will be enabled automatically when found with data in it.");
					proc4_warning = 1;
				}
				do_proc4 = 0;
			}
			else do_proc4 = 2;
		}
		else if(do_proc4ops == 1 && strcmp(type, "proc4ops") == 0) {
			// the first number is the count of numbers present
			// so we start for word 2

			unsigned long long sum = 0;
			unsigned int i, j;
			for(i = 0, j = 2; j < words && nfsd4_ops_values[i].name[0] ; i++, j++) {
				nfsd4_ops_values[i].value = strtoull(procfile_lineword(ff, l, j), NULL, 10);
				nfsd4_ops_values[i].present = 1;
				sum += nfsd4_ops_values[i].value;
			}

			if(sum == 0ULL) {
				if(!proc4ops_warning) {
					info("Disabling /proc/net/rpc/nfsd v4 operations chart. It seems unused on this machine. It will be enabled automatically when found with data in it.");
					proc4ops_warning = 1;
				}
				do_proc4ops = 0;
			}
			else do_proc4ops = 2;
		}
	}

	RRDSET *st;

	// --------------------------------------------------------------------

	if(do_rc == 2) {
		st = rrdset_find_bytype("nfsd", "readcache");
		if(!st) {
			st = rrdset_create("nfsd", "readcache", NULL, "nfsd", NULL, "Read Cache", "reads/s", 5000, update_every, RRDSET_TYPE_STACKED);

			rrddim_add(st, "hits", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "misses", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "nocache", NULL, 1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "hits", rc_hits);
		rrddim_set(st, "misses", rc_misses);
		rrddim_set(st, "nocache", rc_nocache);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_fh == 2) {
		st = rrdset_find_bytype("nfsd", "filehandles");
		if(!st) {
			st = rrdset_create("nfsd", "filehandles", NULL, "nfsd", NULL, "File Handles", "handles/s", 5001, update_every, RRDSET_TYPE_LINE);
			st->isdetail = 1;

			rrddim_add(st, "stale", NULL, 1, 1, RRDDIM_ABSOLUTE);
			rrddim_add(st, "total_lookups", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "anonymous_lookups", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "dir_not_in_dcache", NULL, -1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "non_dir_not_in_dcache", NULL, -1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "stale", fh_stale);
		rrddim_set(st, "total_lookups", fh_total_lookups);
		rrddim_set(st, "anonymous_lookups", fh_anonymous_lookups);
		rrddim_set(st, "dir_not_in_dcache", fh_dir_not_in_dcache);
		rrddim_set(st, "non_dir_not_in_dcache", fh_non_dir_not_in_dcache);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_io == 2) {
		st = rrdset_find_bytype("nfsd", "io");
		if(!st) {
			st = rrdset_create("nfsd", "io", NULL, "nfsd", NULL, "I/O", "kilobytes/s", 5002, update_every, RRDSET_TYPE_AREA);

			rrddim_add(st, "read", NULL, 1, 1000, RRDDIM_INCREMENTAL);
			rrddim_add(st, "write", NULL, -1, 1000, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "read", io_read);
		rrddim_set(st, "write", io_write);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_th == 2) {
		st = rrdset_find_bytype("nfsd", "threads");
		if(!st) {
			st = rrdset_create("nfsd", "threads", NULL, "nfsd", NULL, "Threads", "threads", 5003, update_every, RRDSET_TYPE_LINE);

			rrddim_add(st, "threads", NULL, 1, 1, RRDDIM_ABSOLUTE);
		}
		else rrdset_next(st);

		rrddim_set(st, "threads", th_threads);
		rrdset_done(st);

		st = rrdset_find_bytype("nfsd", "threads_fullcnt");
		if(!st) {
			st = rrdset_create("nfsd", "threads_fullcnt", NULL, "nfsd", NULL, "Threads Full Count", "ops/s", 5004, update_every, RRDSET_TYPE_LINE);

			rrddim_add(st, "full_count", NULL, 1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "full_count", th_fullcnt);
		rrdset_done(st);

		st = rrdset_find_bytype("nfsd", "threads_histogram");
		if(!st) {
			st = rrdset_create("nfsd", "threads_histogram", NULL, "nfsd", NULL, "Threads Usage Histogram", "percentage", 5005, update_every, RRDSET_TYPE_LINE);

			rrddim_add(st, "0%-10%", NULL, 1, 1000, RRDDIM_ABSOLUTE);
			rrddim_add(st, "10%-20%", NULL, 1, 1000, RRDDIM_ABSOLUTE);
			rrddim_add(st, "20%-30%", NULL, 1, 1000, RRDDIM_ABSOLUTE);
			rrddim_add(st, "30%-40%", NULL, 1, 1000, RRDDIM_ABSOLUTE);
			rrddim_add(st, "40%-50%", NULL, 1, 1000, RRDDIM_ABSOLUTE);
			rrddim_add(st, "50%-60%", NULL, 1, 1000, RRDDIM_ABSOLUTE);
			rrddim_add(st, "60%-70%", NULL, 1, 1000, RRDDIM_ABSOLUTE);
			rrddim_add(st, "70%-80%", NULL, 1, 1000, RRDDIM_ABSOLUTE);
			rrddim_add(st, "80%-90%", NULL, 1, 1000, RRDDIM_ABSOLUTE);
			rrddim_add(st, "90%-100%", NULL, 1, 1000, RRDDIM_ABSOLUTE);
		}
		else rrdset_next(st);

		rrddim_set(st, "0%-10%", th_hist10);
		rrddim_set(st, "10%-20%", th_hist20);
		rrddim_set(st, "20%-30%", th_hist30);
		rrddim_set(st, "30%-40%", th_hist40);
		rrddim_set(st, "40%-50%", th_hist50);
		rrddim_set(st, "50%-60%", th_hist60);
		rrddim_set(st, "60%-70%", th_hist70);
		rrddim_set(st, "70%-80%", th_hist80);
		rrddim_set(st, "80%-90%", th_hist90);
		rrddim_set(st, "90%-100%", th_hist100);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_ra == 2) {
		st = rrdset_find_bytype("nfsd", "readahead");
		if(!st) {
			st = rrdset_create("nfsd", "readahead", NULL, "nfsd", NULL, "Read Ahead Depth", "percentage", 5005, update_every, RRDSET_TYPE_STACKED);

			rrddim_add(st, "10%", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
			rrddim_add(st, "20%", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
			rrddim_add(st, "30%", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
			rrddim_add(st, "40%", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
			rrddim_add(st, "50%", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
			rrddim_add(st, "60%", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
			rrddim_add(st, "70%", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
			rrddim_add(st, "80%", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
			rrddim_add(st, "90%", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
			rrddim_add(st, "100%", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
			rrddim_add(st, "misses", NULL, 1, 1, RRDDIM_PCENT_OVER_DIFF_TOTAL);
		}
		else rrdset_next(st);

		// ignore ra_size
		if(ra_size) {};

		rrddim_set(st, "10%", ra_hist10);
		rrddim_set(st, "20%", ra_hist20);
		rrddim_set(st, "30%", ra_hist30);
		rrddim_set(st, "40%", ra_hist40);
		rrddim_set(st, "50%", ra_hist50);
		rrddim_set(st, "60%", ra_hist60);
		rrddim_set(st, "70%", ra_hist70);
		rrddim_set(st, "80%", ra_hist80);
		rrddim_set(st, "90%", ra_hist90);
		rrddim_set(st, "100%", ra_hist100);
		rrddim_set(st, "misses", ra_none);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_net == 2) {
		st = rrdset_find_bytype("nfsd", "net");
		if(!st) {
			st = rrdset_create("nfsd", "net", NULL, "nfsd", NULL, "Network Reads", "reads/s", 5007, update_every, RRDSET_TYPE_STACKED);
			st->isdetail = 1;

			rrddim_add(st, "udp", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "tcp", NULL, 1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		// ignore net_count, net_tcp_connections
		if(net_count) {};
		if(net_tcp_connections) {};

		rrddim_set(st, "udp", net_udp_count);
		rrddim_set(st, "tcp", net_tcp_count);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_rpc == 2) {
		st = rrdset_find_bytype("nfsd", "rpc");
		if(!st) {
			st = rrdset_create("nfsd", "rpc", NULL, "nfsd", NULL, "Remote Procedure Calls", "calls/s", 5008, update_every, RRDSET_TYPE_LINE);
			st->isdetail = 1;

			rrddim_add(st, "all", NULL, 1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "bad_format", NULL, -1, 1, RRDDIM_INCREMENTAL);
			rrddim_add(st, "bad_auth", NULL, -1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		// ignore rpc_bad_client
		if(rpc_bad_client) {};

		rrddim_set(st, "all", rpc_count);
		rrddim_set(st, "bad_format", rpc_bad_format);
		rrddim_set(st, "bad_auth", rpc_bad_auth);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_proc2 == 2) {
		unsigned int i;
		st = rrdset_find_bytype("nfsd", "proc2");
		if(!st) {
			st = rrdset_create("nfsd", "proc2", NULL, "nfsd", NULL, "NFS v2 Calls", "calls/s", 5009, update_every, RRDSET_TYPE_STACKED);

			for(i = 0; nfsd_proc_values[i].present2 ; i++)
				rrddim_add(st, nfsd_proc_values[i].name, NULL, 1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		for(i = 0; nfsd_proc_values[i].present2 ; i++)
			rrddim_set(st, nfsd_proc_values[i].name, nfsd_proc_values[i].proc2);

		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_proc3 == 2) {
		unsigned int i;
		st = rrdset_find_bytype("nfsd", "proc3");
		if(!st) {
			st = rrdset_create("nfsd", "proc3", NULL, "nfsd", NULL, "NFS v3 Calls", "calls/s", 5010, update_every, RRDSET_TYPE_STACKED);

			for(i = 0; nfsd_proc_values[i].present3 ; i++)
				rrddim_add(st, nfsd_proc_values[i].name, NULL, 1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		for(i = 0; nfsd_proc_values[i].present3 ; i++)
			rrddim_set(st, nfsd_proc_values[i].name, nfsd_proc_values[i].proc3);

		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_proc4 == 2) {
		unsigned int i;
		st = rrdset_find_bytype("nfsd", "proc4");
		if(!st) {
			st = rrdset_create("nfsd", "proc4", NULL, "nfsd", NULL, "NFS v4 Calls", "calls/s", 5011, update_every, RRDSET_TYPE_STACKED);

			for(i = 0; nfsd_proc_values[i].present4 ; i++)
				rrddim_add(st, nfsd_proc_values[i].name, NULL, 1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		for(i = 0; nfsd_proc_values[i].present4 ; i++)
			rrddim_set(st, nfsd_proc_values[i].name, nfsd_proc_values[i].proc4);

		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_proc4ops == 2) {
		unsigned int i;
		st = rrdset_find_bytype("nfsd", "proc4ops");
		if(!st) {
			st = rrdset_create("nfsd", "proc4ops", NULL, "nfsd", NULL, "NFS v4 Operations", "operations/s", 5012, update_every, RRDSET_TYPE_STACKED);

			for(i = 0; nfsd4_ops_values[i].present ; i++)
				rrddim_add(st, nfsd4_ops_values[i].name, NULL, 1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		for(i = 0; nfsd4_ops_values[i].present ; i++)
			rrddim_set(st, nfsd4_ops_values[i].name, nfsd4_ops_values[i].value);

		rrdset_done(st);
	}

	return 0;
}

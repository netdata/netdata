#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "log.h"
#include "common.h"
#include "appconfig.h"
#include "procfile.h"
#include "rrd.h"
#include "plugin_proc.h"

int do_proc_loadavg(int update_every, unsigned long long dt) {
	static procfile *ff = NULL;
	static int do_loadavg = -1, do_all_processes = -1;

	if(dt) {};

	if(!ff) {
		char filename[FILENAME_MAX + 1];
		snprintfz(filename, FILENAME_MAX, "%s%s", global_host_prefix, "/proc/loadavg");
		ff = procfile_open(config_get("plugin:proc:/proc/loadavg", "filename to monitor", filename), " \t,:|/", PROCFILE_FLAG_DEFAULT);
	}
	if(!ff) return 1;

	ff = procfile_readall(ff);
	if(!ff) return 0; // we return 0, so that we will retry to open it next time

	if(do_loadavg == -1)		do_loadavg 			= config_get_boolean("plugin:proc:/proc/loadavg", "enable load average", 1);
	if(do_all_processes == -1)	do_all_processes 	= config_get_boolean("plugin:proc:/proc/loadavg", "enable total processes", 1);

	if(procfile_lines(ff) < 1) {
		error("/proc/loadavg has no lines.");
		return 1;
	}
	if(procfile_linewords(ff, 0) < 6) {
		error("/proc/loadavg has less than 6 words in it.");
		return 1;
	}

	double load1 = strtod(procfile_lineword(ff, 0, 0), NULL);
	double load5 = strtod(procfile_lineword(ff, 0, 1), NULL);
	double load15 = strtod(procfile_lineword(ff, 0, 2), NULL);

	//unsigned long long running_processes	= strtoull(procfile_lineword(ff, 0, 3), NULL, 10);
	unsigned long long active_processes		= strtoull(procfile_lineword(ff, 0, 4), NULL, 10);
	//unsigned long long next_pid				= strtoull(procfile_lineword(ff, 0, 5), NULL, 10);


	RRDSET *st;

	// --------------------------------------------------------------------

	if(do_loadavg) {
		st = rrdset_find_byname("system.load");
		if(!st) {
			st = rrdset_create("system", "load", NULL, "load", NULL, "System Load Average", "load", 100, update_every, RRDSET_TYPE_LINE);

			rrddim_add(st, "load1", NULL, 1, 1000, RRDDIM_ABSOLUTE);
			rrddim_add(st, "load5", NULL, 1, 1000, RRDDIM_ABSOLUTE);
			rrddim_add(st, "load15", NULL, 1, 1000, RRDDIM_ABSOLUTE);
		}
		else rrdset_next(st);

		rrddim_set(st, "load1", load1 * 1000);
		rrddim_set(st, "load5", load5 * 1000);
		rrddim_set(st, "load15", load15 * 1000);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_all_processes) {
		st = rrdset_find_byname("system.active_processes");
		if(!st) {
			st = rrdset_create("system", "active_processes", NULL, "processes", NULL, "System Active Processes", "processes", 750, update_every, RRDSET_TYPE_LINE);

			rrddim_add(st, "active", NULL, 1, 1, RRDDIM_ABSOLUTE);
		}
		else rrdset_next(st);

		rrddim_set(st, "active", active_processes);
		rrdset_done(st);
	}

	return 0;
}

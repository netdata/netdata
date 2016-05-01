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

#define RRD_TYPE_STAT 				"cpu"
#define RRD_TYPE_STAT_LEN			strlen(RRD_TYPE_STAT)

int do_proc_stat(int update_every, unsigned long long dt) {
	static procfile *ff = NULL;
	static int do_cpu = -1, do_cpu_cores = -1, do_interrupts = -1, do_context = -1, do_forks = -1, do_processes = -1;

	if(do_cpu == -1)		do_cpu 			= config_get_boolean("plugin:proc:/proc/stat", "cpu utilization", 1);
	if(do_cpu_cores == -1)	do_cpu_cores 	= config_get_boolean("plugin:proc:/proc/stat", "per cpu core utilization", 1);
	if(do_interrupts == -1)	do_interrupts 	= config_get_boolean("plugin:proc:/proc/stat", "cpu interrupts", 1);
	if(do_context == -1)	do_context 		= config_get_boolean("plugin:proc:/proc/stat", "context switches", 1);
	if(do_forks == -1)		do_forks 		= config_get_boolean("plugin:proc:/proc/stat", "processes started", 1);
	if(do_processes == -1)	do_processes 	= config_get_boolean("plugin:proc:/proc/stat", "processes running", 1);

	if(dt) {};

	if(!ff) {
		char filename[FILENAME_MAX + 1];
		snprintfz(filename, FILENAME_MAX, "%s%s", global_host_prefix, "/proc/stat");
		ff = procfile_open(config_get("plugin:proc:/proc/stat", "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
	}
	if(!ff) return 1;

	ff = procfile_readall(ff);
	if(!ff) return 0; // we return 0, so that we will retry to open it next time

	uint32_t lines = procfile_lines(ff), l;
	uint32_t words;

	unsigned long long processes = 0, running = 0 , blocked = 0;
	RRDSET *st;

	for(l = 0; l < lines ;l++) {
		if(strncmp(procfile_lineword(ff, l, 0), "cpu", 3) == 0) {
			words = procfile_linewords(ff, l);
			if(words < 9) {
				error("Cannot read /proc/stat cpu line. Expected 9 params, read %d.", words);
				continue;
			}

			char *id;
			unsigned long long user = 0, nice = 0, system = 0, idle = 0, iowait = 0, irq = 0, softirq = 0, steal = 0, guest = 0, guest_nice = 0;

			id			= procfile_lineword(ff, l, 0);
			user		= strtoull(procfile_lineword(ff, l, 1), NULL, 10);
			nice		= strtoull(procfile_lineword(ff, l, 2), NULL, 10);
			system		= strtoull(procfile_lineword(ff, l, 3), NULL, 10);
			idle		= strtoull(procfile_lineword(ff, l, 4), NULL, 10);
			iowait		= strtoull(procfile_lineword(ff, l, 5), NULL, 10);
			irq			= strtoull(procfile_lineword(ff, l, 6), NULL, 10);
			softirq		= strtoull(procfile_lineword(ff, l, 7), NULL, 10);
			steal		= strtoull(procfile_lineword(ff, l, 8), NULL, 10);
			if(words >= 10) guest		= strtoull(procfile_lineword(ff, l, 9), NULL, 10);
			if(words >= 11) guest_nice	= strtoull(procfile_lineword(ff, l, 10), NULL, 10);

			char *title = "Core utilization";
			char *type = RRD_TYPE_STAT;
			char *context = "cpu.cpu";
			char *family = "utilization";
			long priority = 1000;
			int isthistotal = 0;

			if(strcmp(id, "cpu") == 0) {
				isthistotal = 1;
				type = "system";
				title = "Total CPU utilization";
				context = "system.cpu";
				family = id;
				priority = 100;
			}

			if((isthistotal && do_cpu) || (!isthistotal && do_cpu_cores)) {
				st = rrdset_find_bytype(type, id);
				if(!st) {
					st = rrdset_create(type, id, NULL, family, context, title, "percentage", priority, update_every, RRDSET_TYPE_STACKED);

					long multiplier = 1;
					long divisor = 1; // sysconf(_SC_CLK_TCK);

					rrddim_add(st, "guest_nice", NULL, multiplier, divisor, RRDDIM_PCENT_OVER_DIFF_TOTAL);
					rrddim_add(st, "guest", NULL, multiplier, divisor, RRDDIM_PCENT_OVER_DIFF_TOTAL);
					rrddim_add(st, "steal", NULL, multiplier, divisor, RRDDIM_PCENT_OVER_DIFF_TOTAL);
					rrddim_add(st, "softirq", NULL, multiplier, divisor, RRDDIM_PCENT_OVER_DIFF_TOTAL);
					rrddim_add(st, "irq", NULL, multiplier, divisor, RRDDIM_PCENT_OVER_DIFF_TOTAL);
					rrddim_add(st, "user", NULL, multiplier, divisor, RRDDIM_PCENT_OVER_DIFF_TOTAL);
					rrddim_add(st, "system", NULL, multiplier, divisor, RRDDIM_PCENT_OVER_DIFF_TOTAL);
					rrddim_add(st, "nice", NULL, multiplier, divisor, RRDDIM_PCENT_OVER_DIFF_TOTAL);
					rrddim_add(st, "iowait", NULL, multiplier, divisor, RRDDIM_PCENT_OVER_DIFF_TOTAL);

					rrddim_add(st, "idle", NULL, multiplier, divisor, RRDDIM_PCENT_OVER_DIFF_TOTAL);
					rrddim_hide(st, "idle");
				}
				else rrdset_next(st);

				rrddim_set(st, "user", user);
				rrddim_set(st, "nice", nice);
				rrddim_set(st, "system", system);
				rrddim_set(st, "idle", idle);
				rrddim_set(st, "iowait", iowait);
				rrddim_set(st, "irq", irq);
				rrddim_set(st, "softirq", softirq);
				rrddim_set(st, "steal", steal);
				rrddim_set(st, "guest", guest);
				rrddim_set(st, "guest_nice", guest_nice);
				rrdset_done(st);
			}
		}
		else if(strcmp(procfile_lineword(ff, l, 0), "intr") == 0) {
			unsigned long long value = strtoull(procfile_lineword(ff, l, 1), NULL, 10);

			// --------------------------------------------------------------------

			if(do_interrupts) {
				st = rrdset_find_bytype("system", "intr");
				if(!st) {
					st = rrdset_create("system", "intr", NULL, "interrupts", NULL, "CPU Interrupts", "interrupts/s", 900, update_every, RRDSET_TYPE_LINE);
					st->isdetail = 1;

					rrddim_add(st, "interrupts", NULL, 1, 1, RRDDIM_INCREMENTAL);
				}
				else rrdset_next(st);

				rrddim_set(st, "interrupts", value);
				rrdset_done(st);
			}
		}
		else if(strcmp(procfile_lineword(ff, l, 0), "ctxt") == 0) {
			unsigned long long value = strtoull(procfile_lineword(ff, l, 1), NULL, 10);

			// --------------------------------------------------------------------

			if(do_context) {
				st = rrdset_find_bytype("system", "ctxt");
				if(!st) {
					st = rrdset_create("system", "ctxt", NULL, "processes", NULL, "CPU Context Switches", "context switches/s", 800, update_every, RRDSET_TYPE_LINE);

					rrddim_add(st, "switches", NULL, 1, 1, RRDDIM_INCREMENTAL);
				}
				else rrdset_next(st);

				rrddim_set(st, "switches", value);
				rrdset_done(st);
			}
		}
		else if(!processes && strcmp(procfile_lineword(ff, l, 0), "processes") == 0) {
			processes = strtoull(procfile_lineword(ff, l, 1), NULL, 10);
		}
		else if(!running && strcmp(procfile_lineword(ff, l, 0), "procs_running") == 0) {
			running = strtoull(procfile_lineword(ff, l, 1), NULL, 10);
		}
		else if(!blocked && strcmp(procfile_lineword(ff, l, 0), "procs_blocked") == 0) {
			blocked = strtoull(procfile_lineword(ff, l, 1), NULL, 10);
		}
	}

	// --------------------------------------------------------------------

	if(do_forks) {
		st = rrdset_find_bytype("system", "forks");
		if(!st) {
			st = rrdset_create("system", "forks", NULL, "processes", NULL, "Started Processes", "processes/s", 700, update_every, RRDSET_TYPE_LINE);
			st->isdetail = 1;

			rrddim_add(st, "started", NULL, 1, 1, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "started", processes);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_processes) {
		st = rrdset_find_bytype("system", "processes");
		if(!st) {
			st = rrdset_create("system", "processes", NULL, "processes", NULL, "System Processes", "processes", 600, update_every, RRDSET_TYPE_LINE);

			rrddim_add(st, "running", NULL, 1, 1, RRDDIM_ABSOLUTE);
			rrddim_add(st, "blocked", NULL, -1, 1, RRDDIM_ABSOLUTE);
		}
		else rrdset_next(st);

		rrddim_set(st, "running", running);
		rrddim_set(st, "blocked", blocked);
		rrdset_done(st);
	}

	return 0;
}

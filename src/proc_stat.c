#include <inttypes.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "log.h"
#include "config.h"
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

	if(!ff) ff = procfile_open("/proc/stat", " \t:");
	if(!ff) return 1;

	ff = procfile_readall(ff);
	if(!ff) return 0; // we return 0, so that we will retry to open it next time

	uint32_t lines = procfile_lines(ff), l;
	uint32_t words;

	unsigned long long processes = 0, running = 0 , blocked = 0;
	RRD_STATS *st;

	for(l = 0; l < lines ;l++) {
		if(strncmp(procfile_lineword(ff, l, 0), "cpu", 3) == 0) {
			words = procfile_linewords(ff, l);
			if(words < 11) {
				error("Cannot read /proc/stat cpu line. Expected 11 params, read %d.", words);
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
			guest		= strtoull(procfile_lineword(ff, l, 9), NULL, 10);
			guest_nice	= strtoull(procfile_lineword(ff, l, 10), NULL, 10);

			char *title = "Core utilization";
			char *type = RRD_TYPE_STAT;
			long priority = 1000;
			int isthistotal = 0;
			if(strcmp(id, "cpu") == 0) {
				isthistotal = 1;
				title = "Total CPU utilization";
				type = "system";
				priority = 100;
			}

			if((isthistotal && do_cpu) || (!isthistotal && do_cpu_cores)) {
				st = rrd_stats_find_bytype(type, id);
				if(!st) {
					st = rrd_stats_create(type, id, NULL, "cpu", title, "percentage", priority, update_every, CHART_TYPE_STACKED);

					long multiplier = 1;
					long divisor = 1; // sysconf(_SC_CLK_TCK);

					rrd_stats_dimension_add(st, "guest_nice", NULL, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL);
					rrd_stats_dimension_add(st, "guest", NULL, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL);
					rrd_stats_dimension_add(st, "steal", NULL, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL);
					rrd_stats_dimension_add(st, "softirq", NULL, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL);
					rrd_stats_dimension_add(st, "irq", NULL, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL);
					rrd_stats_dimension_add(st, "user", NULL, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL);
					rrd_stats_dimension_add(st, "system", NULL, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL);
					rrd_stats_dimension_add(st, "nice", NULL, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL);
					rrd_stats_dimension_add(st, "iowait", NULL, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL);

					rrd_stats_dimension_add(st, "idle", NULL, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL);
					rrd_stats_dimension_hide(st, "idle");
				}
				else rrd_stats_next(st);

				rrd_stats_dimension_set(st, "user", user);
				rrd_stats_dimension_set(st, "nice", nice);
				rrd_stats_dimension_set(st, "system", system);
				rrd_stats_dimension_set(st, "idle", idle);
				rrd_stats_dimension_set(st, "iowait", iowait);
				rrd_stats_dimension_set(st, "irq", irq);
				rrd_stats_dimension_set(st, "softirq", softirq);
				rrd_stats_dimension_set(st, "steal", steal);
				rrd_stats_dimension_set(st, "guest", guest);
				rrd_stats_dimension_set(st, "guest_nice", guest_nice);
				rrd_stats_done(st);
			}
		}
		else if(strcmp(procfile_lineword(ff, l, 0), "intr") == 0) {
			unsigned long long value = strtoull(procfile_lineword(ff, l, 1), NULL, 10);

			// --------------------------------------------------------------------
	
			if(do_interrupts) {
				st = rrd_stats_find_bytype("system", "intr");
				if(!st) {
					st = rrd_stats_create("system", "intr", NULL, "cpu", "CPU Interrupts", "interrupts/s", 900, update_every, CHART_TYPE_LINE);
					st->isdetail = 1;

					rrd_stats_dimension_add(st, "interrupts", NULL, 1, 1 * update_every, RRD_DIMENSION_INCREMENTAL);
				}
				else rrd_stats_next(st);

				rrd_stats_dimension_set(st, "interrupts", value);
				rrd_stats_done(st);
			}
		}
		else if(strcmp(procfile_lineword(ff, l, 0), "ctxt") == 0) {
			unsigned long long value = strtoull(procfile_lineword(ff, l, 1), NULL, 10);

			// --------------------------------------------------------------------
	
			if(do_context) {
				st = rrd_stats_find_bytype("system", "ctxt");
				if(!st) {
					st = rrd_stats_create("system", "ctxt", NULL, "cpu", "CPU Context Switches", "context switches/s", 800, update_every, CHART_TYPE_LINE);

					rrd_stats_dimension_add(st, "switches", NULL, 1, 1 * update_every, RRD_DIMENSION_INCREMENTAL);
				}
				else rrd_stats_next(st);

				rrd_stats_dimension_set(st, "switches", value);
				rrd_stats_done(st);
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
		st = rrd_stats_find_bytype("system", "forks");
		if(!st) {
			st = rrd_stats_create("system", "forks", NULL, "cpu", "New Processes", "processes/s", 700, update_every, CHART_TYPE_LINE);
			st->isdetail = 1;

			rrd_stats_dimension_add(st, "started", NULL, 1, 1 * update_every, RRD_DIMENSION_INCREMENTAL);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "started", processes);
		rrd_stats_done(st);
	}

	// --------------------------------------------------------------------

	if(do_processes) {
		st = rrd_stats_find_bytype("system", "processes");
		if(!st) {
			st = rrd_stats_create("system", "processes", NULL, "cpu", "Processes", "processes", 600, update_every, CHART_TYPE_LINE);

			rrd_stats_dimension_add(st, "running", NULL, 1, 1, RRD_DIMENSION_ABSOLUTE);
			rrd_stats_dimension_add(st, "blocked", NULL, -1, 1, RRD_DIMENSION_ABSOLUTE);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "running", running);
		rrd_stats_dimension_set(st, "blocked", blocked);
		rrd_stats_done(st);
	}

	return 0;
}

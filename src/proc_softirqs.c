#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <ctype.h>

#include "common.h"
#include "appconfig.h"
#include "procfile.h"
#include "rrd.h"
#include "plugin_proc.h"
#include "log.h"

#define MAX_INTERRUPT_NAME 50

struct interrupt {
	int used;
	char *id;
	char name[MAX_INTERRUPT_NAME + 1];
	unsigned long long total;
	unsigned long long value[];
};

// since each interrupt is variable in size
// we use this to calculate its record size
#define recordsize(cpus) (sizeof(struct interrupt) + (cpus * sizeof(unsigned long long)))

// given a base, get a pointer to each record
#define irrindex(base, line, cpus) ((struct interrupt *)&((char *)(base))[line * recordsize(cpus)])

static inline struct interrupt *get_interrupts_array(int lines, int cpus) {
	static struct interrupt *irrs = NULL;
	static int allocated = 0;

	if(lines < allocated) return irrs;
	else {
		irrs = (struct interrupt *)realloc(irrs, lines * recordsize(cpus));
		if(!irrs)
			fatal("Cannot allocate memory for %d interrupts", lines);

		allocated = lines;
	}

	return irrs;
}

int do_proc_softirqs(int update_every, unsigned long long dt) {
	static procfile *ff = NULL;
	static int cpus = -1, do_per_core = -1;

	struct interrupt *irrs = NULL;

	if(dt) {};

	if(do_per_core == -1) do_per_core = config_get_boolean("plugin:proc:/proc/softirqs", "interrupts per core", 1);

	if(!ff) {
		char filename[FILENAME_MAX + 1];
		snprintfz(filename, FILENAME_MAX, "%s%s", global_host_prefix, "/proc/softirqs");
		ff = procfile_open(config_get("plugin:proc:/proc/softirqs", "filename to monitor", filename), " \t", PROCFILE_FLAG_DEFAULT);
	}
	if(!ff) return 1;

	ff = procfile_readall(ff);
	if(!ff) return 0; // we return 0, so that we will retry to open it next time

	uint32_t lines = procfile_lines(ff), l;
	uint32_t words = procfile_linewords(ff, 0), w;

	if(!lines) {
		error("Cannot read /proc/softirqs, zero lines reported.");
		return 1;
	}

	// find how many CPUs are there
	if(cpus == -1) {
		cpus = 0;
		for(w = 0; w < words ; w++) {
			if(strncmp(procfile_lineword(ff, 0, w), "CPU", 3) == 0)
				cpus++;
		}
	}

	if(!cpus) {
		error("PLUGIN: PROC_SOFTIRQS: Cannot find the number of CPUs in /proc/softirqs");
		return 1;
	}

	// allocate the size we need;
	irrs = get_interrupts_array(lines, cpus);
	irrs[0].used = 0;

	// loop through all lines
	for(l = 1; l < lines ;l++) {
		struct interrupt *irr = irrindex(irrs, l, cpus);
		irr->used = 0;
		irr->total = 0;

		words = procfile_linewords(ff, l);
		if(!words) continue;

		irr->id = procfile_lineword(ff, l, 0);
		if(!irr->id || !irr->id[0]) continue;

		int idlen = strlen(irr->id);
		if(irr->id[idlen - 1] == ':')
			irr->id[idlen - 1] = '\0';

		int c;
		for(c = 0; c < cpus ;c++) {
			if((c + 1) < (int)words)
				irr->value[c] = strtoull(procfile_lineword(ff, l, (uint32_t)(c + 1)), NULL, 10);
			else
				irr->value[c] = 0;

			irr->total += irr->value[c];
		}

		strncpyz(irr->name, irr->id, MAX_INTERRUPT_NAME);

		irr->used = 1;
	}

	RRDSET *st;

	// --------------------------------------------------------------------

	st = rrdset_find_bytype("system", "softirqs");
	if(!st) {
		st = rrdset_create("system", "softirqs", NULL, "softirqs", NULL, "System softirqs", "softirqs/s", 950, update_every, RRDSET_TYPE_STACKED);

		for(l = 0; l < lines ;l++) {
			struct interrupt *irr = irrindex(irrs, l, cpus);
			if(!irr->used) continue;
			rrddim_add(st, irr->id, irr->name, 1, 1, RRDDIM_INCREMENTAL);
		}
	}
	else rrdset_next(st);

	for(l = 0; l < lines ;l++) {
		struct interrupt *irr = irrindex(irrs, l, cpus);
		if(!irr->used) continue;
		rrddim_set(st, irr->id, irr->total);
	}
	rrdset_done(st);

	if(do_per_core) {
		int c;

		for(c = 0; c < cpus ; c++) {
			char id[256+1];
			snprintfz(id, 256, "cpu%d_softirqs", c);

			st = rrdset_find_bytype("cpu", id);
			if(!st) {
				// find if everything is zero
				unsigned long long core_sum = 0 ;
				for(l = 0; l < lines ;l++) {
					struct interrupt *irr = irrindex(irrs, l, cpus);
					if(!irr->used) continue;
					core_sum += irr->value[c];
				}
				if(core_sum == 0) continue; // try next core

				char name[256+1], title[256+1];
				snprintfz(name, 256, "cpu%d_softirqs", c);
				snprintfz(title, 256, "CPU%d softirqs", c);
				st = rrdset_create("cpu", id, name, "softirqs", "cpu.softirqs", title, "softirqs/s", 3000 + c, update_every, RRDSET_TYPE_STACKED);

				for(l = 0; l < lines ;l++) {
					struct interrupt *irr = irrindex(irrs, l, cpus);
					if(!irr->used) continue;
					rrddim_add(st, irr->id, irr->name, 1, 1, RRDDIM_INCREMENTAL);
				}
			}
			else rrdset_next(st);

			for(l = 0; l < lines ;l++) {
				struct interrupt *irr = irrindex(irrs, l, cpus);
				if(!irr->used) continue;
				rrddim_set(st, irr->id, irr->value[c]);
			}
			rrdset_done(st);
		}
	}

	return 0;
}

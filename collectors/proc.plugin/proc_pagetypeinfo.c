// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

// For PAGE_SIZE
#include <sys/user.h>
// For ULONG_MAX
#include <limits.h>

#define PLUGIN_PROC_MODULE_PAGETYPEINFO_NAME "/proc/pagetypeinfo"
#define CONFIG_SECTION_PLUGIN_PROC_PAGETYPEINFO "plugin:" PLUGIN_PROC_CONFIG_NAME ":" PLUGIN_PROC_MODULE_PAGETYPEINFO_NAME

// Zone struct is pglist_data, in include/linux/mmzone.h
// MAX_NR_ZONES is from __MAX_NR_ZONE, which is the last value of the enum.
#define MAX_PAGETYPE_ORDER 11

// Names are in mm/page_alloc.c :: migratetype_names. Max size = 10.
#define MAX_ZONETYPE_NAME 16
#define MAX_PAGETYPE_NAME 16

// Defined in include/linux/mmzone.h as __MAX_NR_ZONE (last enum of zone_type)
#define MAX_ZONETYPE  6
// Defined in include/linux/mmzone.h as MIGRATE_TYPES (last enum of migratetype)
#define MAX_PAGETYPE  7

// Defined as 2^CONFIG_cnt_nodes_SHIFT. We'll use 16 for now (8 max * 2 as margin):
// - Up to 4 Intel Xeon, each with SNC (x2) = 8 cnt_nodes.
// - Up to 2 AMD  EPYC, each with 4 CPUs = 8 cnt_nodes.
#define MAX_NUMA_cnt_nodes 16

//
// /proc/pagetypeinfo is declared in mm/vmstat.c :: init_mm_internals
//
struct pageline {
	int node;
	char *zone;
	char *type;
	int line;
	uint64_t free_pages[MAX_PAGETYPE_ORDER];
	RRDDIM  *rd[MAX_PAGETYPE_ORDER];
};

struct nodezone {
	int node;
	char *zone;
	struct pageline* lines[MAX_PAGETYPE];
};

struct systemorder {
	uint64_t count;
	RRDDIM *rd;
};



static inline uint64_t pageline_total_size(struct pageline *p) {
	uint64_t sum = 0, o;
	for (o=0; o<MAX_PAGETYPE_ORDER; o++)
		sum += p->free_pages[o] * (o+1) * PAGE_SIZE;
	return sum;
}

static inline uint64_t pageline_total_count(struct pageline *p) {
	uint64_t sum = 0, o;
	for (o=0; o<MAX_PAGETYPE_ORDER; o++)
		sum += p->free_pages[o];
	return sum;
}

// Check if a line of /proc/pagetypeinfo is valid to use
// Free block lines starts by "Node" && 4th col is "type"
#define pagetypeinfo_line_valid(ff, l) (strncmp(procfile_lineword(ff, l, 0), "Node", 4) == 0 && strncmp(procfile_lineword(ff, l, 4), "type", 4) == 0)

// Dimension name from the order
#define dim_name(s, o) (snprintfz(s, 16,"%luKB (%lu)", (1 << o) * PAGE_SIZE / 1024, o))

int do_proc_pagetypeinfo(int update_every, usec_t dt) {
	(void)dt;

	// Counters from parsing the file, that doesn't change after boot
	static int cnt_nodes = -1, cnt_zones = -1, cnt_pagetypes = -1, cnt_pageorders = -1;
	static struct systemorder systemorders[MAX_PAGETYPE_ORDER] = {};
	static struct pageline* pagelines = NULL;
	static size_t pagelines_cnt = -1, lines = -1;

	static procfile *ff = NULL;

	static RRDSET *st_order = NULL;
	//static RRDSET **st_nodezone = NULL;
	static RRDSET **st_nodezonetype = NULL;

	// Local temp variables
	size_t l, o, p;
	struct pageline *pgl = NULL;

	// --------------------------------------------------------------------
	// Startup: Open /proc/pagetypeinfo
	if(unlikely(!ff)) {
		ff = procfile_open(PLUGIN_PROC_MODULE_PAGETYPEINFO_NAME, " \t,", PROCFILE_FLAG_DEFAULT);
	}
	if(unlikely(!ff))
		return 1;

	ff = procfile_readall(ff);
	if(unlikely(!ff))
		return 0; // we return 0, so that we will retry to open it next time

	// --------------------------------------------------------------------
	// Init: find how many cnt_nodes, Zones and Types
	if(unlikely(cnt_nodes == -1)) {
		size_t nodenumlast = -1;
		char *zonenamelast;

		lines = procfile_lines(ff);
		if(unlikely(!lines)) {
			error("PLUGIN: PROC_PAGETYPEINFO: Cannot read %s, zero lines reported.", PLUGIN_PROC_MODULE_PAGETYPEINFO_NAME);
			return 1;
		}

		// 4th line is the "Free pages count...". Just substract the 8 words.
		cnt_pageorders = procfile_linewords(ff, 3) - 9;
		cnt_nodes = 0;
		cnt_zones = 0;
		pagelines_cnt = 0;

		// Pass 1: how many lines would be valid
		for (l = 4; l < lines; l++) {
			if (!pagetypeinfo_line_valid(ff, l))
				continue;

			pagelines_cnt++;
		}

		// Init pagelines from scanned lines
		if (!pagelines) {
			pagelines = callocz(pagelines_cnt, sizeof(struct pageline));
			if (!pagelines) {
				error("PLUGIN: PROC_PAGETYPEINFO: Cannot allocate %lu pagelines of %lu B", pagelines_cnt, sizeof(struct pageline));
				return 1;
			}
		}

		// Pass 2: Scan the file again, with details
		p = 0;
		for (l=4; l < lines; l++) {

			if (!pagetypeinfo_line_valid(ff, l))
				continue;

			size_t nodenum = strtoul(procfile_lineword(ff, l, 1), NULL, 10);
			char *zonename = procfile_lineword(ff, l, 3);
			char *typename = procfile_lineword(ff, l, 5);

			// We changed node or zone
			if (nodenum != nodenumlast || !zonenamelast ||  strncmp(zonename, zonenamelast, 6) != 0) {
				cnt_zones++;
				zonenamelast = zonename;
			}

			// Count the number of numa cnt_nodes
			if( nodenum != nodenumlast ) {
				cnt_nodes++;
				nodenumlast = nodenum;
			}

			// Unmovable is always the first in the enum. The first line higher than 4
			if (strncmp(typename, "Unmovable", 10) == 0 && l >4) {
				if (cnt_pagetypes == -1)
					cnt_pagetypes = l - 4;
			}

			// Populate the line
			pgl = &pagelines[p];

			pgl->line = l;
			pgl->node = nodenum;
			pgl->type = typename;
			pgl->zone = zonename;
			for (o = 0; o < MAX_PAGETYPE_ORDER; o++)
				pgl->free_pages[o] = str2uint64_t(procfile_lineword(ff, l, o+6));

			p++;
		}

		// Init the RRD graphs

		// Per-Order: sum of all node, zone, type Grouped by order
		st_order = rrdset_create_localhost(
			"mem"
			, "pagetype_orders"
			, NULL
			, "pagetype"
			, NULL
			, "System orders available"
			, "B"
			, PLUGIN_PROC_NAME
			, PLUGIN_PROC_MODULE_PAGETYPEINFO_NAME
			, NETDATA_CHART_PRIO_SYSTEM_MEMFRAG
			, update_every
			, RRDSET_TYPE_STACKED
		);
		for (o = 0; o < MAX_PAGETYPE_ORDER; o++) {
			char id[3+1];
			snprintfz(id, 3, "%lu", o);

			char name[20+1];
			dim_name(name, o);

			systemorders[o].rd = rrddim_add(st_order, id, name, PAGE_SIZE, 1, RRD_ALGORITHM_ABSOLUTE);
			rrddim_set_by_pointer(st_order, systemorders[o].rd, systemorders[o].count);
		}

		// Per Numa-Node & Zone
		// TODO

		// Per-Numa Node & Zone & Type (full detail). Only if sum(line) > 0
		st_nodezonetype = callocz(cnt_zones, sizeof(RRDSET));
		for (p = 0; p < pagelines_cnt; p++) {
			pgl = &pagelines[p];

			// Skip empty pagelines
			// TODO: add configuration for override
			if (!pgl || pageline_total_count(pgl) == 0) {
				error("Skipping zero total for page %lu", p);
				continue;
			}

			char id[4+2+1+MAX_ZONETYPE_NAME+1+MAX_PAGETYPE_NAME+1];
			snprintfz(id, 4+2+1+MAX_ZONETYPE_NAME+1+MAX_PAGETYPE_NAME, "node%d_%s_%s", pgl->node, pgl->zone, pgl->type);

			char name[MAX_ZONETYPE_NAME + MAX_PAGETYPE_NAME +1];
			snprintfz(name, MAX_ZONETYPE_NAME + MAX_PAGETYPE_NAME, "Node %d %s %s",
				pgl->node, pgl->zone, pgl->type);

			st_nodezonetype[p] = rrdset_create_localhost(
					"mem"
					, id
					, NULL
					, "pagetype"
					, NULL
					, name
					, "B"
					, PLUGIN_PROC_NAME
					, PLUGIN_PROC_MODULE_PAGETYPEINFO_NAME
					, NETDATA_CHART_PRIO_MEM_PAGEFRAG + p
					, update_every
					, RRDSET_TYPE_STACKED
			);
			for (o = 0; o < MAX_PAGETYPE_ORDER; o++) {
				char id[3+1];
				snprintfz(id, 3, "%lu", o);
				char name[20+1];
				dim_name(name, o);

				pgl->rd[o] = rrddim_add(st_nodezonetype[p], id, name, PAGE_SIZE, 1, RRD_ALGORITHM_ABSOLUTE);
				rrddim_set_by_pointer(st_order, pgl->rd[o], pgl->free_pages[o]);
			}
		}
	}

	if(unlikely(!cnt_nodes)) {
		error("PLUGIN: PROC_PAGETPEINFO: Cannot find the number of CPUs in %s", PLUGIN_PROC_MODULE_PAGETYPEINFO_NAME);
		return 1;
	}

	// --------------------------------------------------------------------
	// Update pagelines

	// Process each line
	p = 0;
	for (l=4; l<lines; l++) {

		if (!pagetypeinfo_line_valid(ff, l))
			continue;

		int words = procfile_linewords(ff, l);

		if (words < 6+cnt_pageorders) {
			error("Unable to read line %lu, only %d words found instead of %d", l, words, 6 + cnt_pageorders);
			break;
		}

		for (o = 0; o < MAX_PAGETYPE_ORDER; o++) {
			// Reset counter
			if (p == 0)
				systemorders[o].count = 0;

			// Update orders of the current line
			pagelines[p].free_pages[o] = str2uint64_t(procfile_lineword(ff, l, o+6));

			// Update sum by order
			systemorders[o].count += pagelines[p].free_pages[o];
		}

		assert(p < pagelines_cnt);
		p++;
	}

	// --------------------------------------------------------------------
	// update RRD values

	// Global system per order
	rrdset_next(st_order);
	for (o = 0; o < MAX_PAGETYPE_ORDER; o++) {
		rrddim_set_by_pointer(st_order, systemorders[o].rd, systemorders[o].count);
	}
	rrdset_done(st_order);


	// Per Node-Zone-Type
	for (p = 0; p < pagelines_cnt; p++) {
		// Skip empty graphs
		if (!st_nodezonetype[p]) {
			error("Skipping line %lu", p);
			continue;
		}

		rrdset_next(st_nodezonetype[p]);
		for (o = 0; o < MAX_PAGETYPE_ORDER; o++)
			rrddim_set_by_pointer(st_nodezonetype[p], pagelines[p].rd[o], pagelines[p].free_pages[o]);

		rrdset_done(st_nodezonetype[p]);
	}

	return 0;
}

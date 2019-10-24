// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

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


//
// /proc/pagetypeinfo is declared in mm/vmstat.c :: init_mm_internals
//

// One line of /proc/pagetypeinfo
struct pageline {
    int node;
    char *zone;
    char *type;
    int line;
    uint64_t free_pages_size[MAX_PAGETYPE_ORDER];
    RRDDIM  *rd[MAX_PAGETYPE_ORDER];
};

// Sum of all orders
struct systemorder {
    uint64_t size;
    RRDDIM *rd;
};


static inline uint64_t pageline_total_count(struct pageline *p) {
    uint64_t sum = 0, o;
    for (o=0; o<MAX_PAGETYPE_ORDER; o++)
        sum += p->free_pages_size[o];
    return sum;
}

// Check if a line of /proc/pagetypeinfo is valid to use
// Free block lines starts by "Node" && 4th col is "type"
#define pagetypeinfo_line_valid(ff, l) (strncmp(procfile_lineword(ff, l, 0), "Node", 4) == 0 && strncmp(procfile_lineword(ff, l, 4), "type", 4) == 0)

// Dimension name from the order
#define dim_name(s, o, pagesize) (snprintfz(s, 16,"%ldKB (%lu)", (1 << o) * pagesize / 1024, o))

int do_proc_pagetypeinfo(int update_every, usec_t dt) {
    (void)dt;

    // Config
    static int do_global, do_detail;
    static SIMPLE_PATTERN *filter_types = NULL;

    // Counters from parsing the file, that doesn't change after boot
    static struct systemorder systemorders[MAX_PAGETYPE_ORDER] = {};
    static struct pageline* pagelines = NULL;
    static long pagesize = 0;
    static size_t pageorders_cnt = 0, pagelines_cnt = 0, ff_lines = 0;

    // Handle
    static procfile *ff = NULL;
    static char ff_path[FILENAME_MAX + 1];

    // RRD Sets
    static RRDSET *st_order = NULL;
    static RRDSET **st_nodezonetype = NULL;

    // Local temp variables
    size_t l, o, p;
    struct pageline *pgl = NULL;

    // --------------------------------------------------------------------
    // Startup: Init arch and open /proc/pagetypeinfo
    if (unlikely(!pagesize)) {
        pagesize = sysconf(_SC_PAGESIZE);
    }

    if(unlikely(!ff)) {
        snprintfz(ff_path, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, PLUGIN_PROC_MODULE_PAGETYPEINFO_NAME);
        ff = procfile_open(config_get(CONFIG_SECTION_PLUGIN_PROC_PAGETYPEINFO, "filename to monitor", ff_path), " \t:", PROCFILE_FLAG_DEFAULT);

        if(unlikely(!ff)) {
            strncpyz(ff_path, PLUGIN_PROC_MODULE_PAGETYPEINFO_NAME, FILENAME_MAX);
            ff = procfile_open(PLUGIN_PROC_MODULE_PAGETYPEINFO_NAME, " \t,", PROCFILE_FLAG_DEFAULT);
        }
    }
    if(unlikely(!ff))
        return 1;

    ff = procfile_readall(ff);
    if(unlikely(!ff))
        return 0; // we return 0, so that we will retry to open it next time

    // --------------------------------------------------------------------
    // Init: find how many Nodes, Zones and Types
    if(unlikely(pagelines_cnt == 0)) {
        size_t nodenumlast = -1;
        char *zonenamelast = NULL;

        ff_lines = procfile_lines(ff);
        if(unlikely(!ff_lines)) {
            error("PLUGIN: PROC_PAGETYPEINFO: Cannot read %s, zero lines reported.", ff_path);
            return 1;
        }

        // Configuration
        do_global = config_get_boolean(CONFIG_SECTION_PLUGIN_PROC_PAGETYPEINFO, "enable system summary", CONFIG_BOOLEAN_YES);
        do_detail = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_PAGETYPEINFO, "enable detail per-type", CONFIG_BOOLEAN_AUTO);
        filter_types = simple_pattern_create(
                config_get(CONFIG_SECTION_PLUGIN_PROC_PAGETYPEINFO, "hide charts id matching", "")
                , NULL
                , SIMPLE_PATTERN_SUFFIX
        );

        pagelines_cnt = 0;

        // Pass 1: how many lines would be valid
        for (l = 4; l < ff_lines; l++) {
            if (!pagetypeinfo_line_valid(ff, l))
                continue;

            pagelines_cnt++;
        }
        if (pagelines_cnt == 0) {
            error("PLUGIN: PROC_PAGETYPEINFO: Unable to parse any valid line in %s", ff_path);
            return 1;
        }

        // 4th line is the "Free pages count per migrate type at order". Just substract these 8 words.
        pageorders_cnt = procfile_linewords(ff, 3);
        if (pageorders_cnt < 9) {
            error("PLUGIN: PROC_PAGETYPEINFO: Unable to parse Line 4 of %s", ff_path);
            return 1;
        }

        pageorders_cnt -= 9;

        if (pageorders_cnt > MAX_PAGETYPE_ORDER) {
            error("PLUGIN: PROC_PAGETYPEINFO: pageorder found (%lu) is higher than max %d", pageorders_cnt, MAX_PAGETYPE_ORDER);
            return 1;
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
        for (l=4; l < ff_lines; l++) {

            if (!pagetypeinfo_line_valid(ff, l))
                continue;

            size_t nodenum = strtoul(procfile_lineword(ff, l, 1), NULL, 10);
            char *zonename = procfile_lineword(ff, l, 3);
            char *typename = procfile_lineword(ff, l, 5);

            // We changed node or zone
            if (nodenum != nodenumlast || !zonenamelast ||  strncmp(zonename, zonenamelast, 6) != 0) {
                zonenamelast = zonename;
            }

            // Populate the line
            pgl = &pagelines[p];

            pgl->line = l;
            pgl->node = nodenum;
            pgl->type = typename;
            pgl->zone = zonename;
            for (o = 0; o < pageorders_cnt; o++)
                pgl->free_pages_size[o] = str2uint64_t(procfile_lineword(ff, l, o+6)) * 1 << o;

            p++;
        }

        // Init the RRD graphs

        // Per-Order: sum of all node, zone, type Grouped by order
        if (do_global != CONFIG_BOOLEAN_NO) {
            st_order = rrdset_create_localhost(
                "mem"
                , "pagetype_global"
                , NULL
                , "pagetype"
                , NULL
                , "System orders available"
                , "B"
                , PLUGIN_PROC_NAME
                , PLUGIN_PROC_MODULE_PAGETYPEINFO_NAME
                , NETDATA_CHART_PRIO_MEM_PAGEFRAG
                , update_every
                , RRDSET_TYPE_STACKED
            );
            for (o = 0; o < pageorders_cnt; o++) {
                char id[3+1];
                snprintfz(id, 3, "%lu", o);

                char name[20+1];
                dim_name(name, o, pagesize);

                systemorders[o].rd = rrddim_add(st_order, id, name, pagesize, 1, RRD_ALGORITHM_ABSOLUTE);
            }
        }


        // Per-Numa Node & Zone & Type (full detail). Only if sum(line) > 0
        st_nodezonetype = callocz(pagelines_cnt, sizeof(RRDSET *));
        for (p = 0; p < pagelines_cnt; p++) {
            pgl = &pagelines[p];

            // Skip invalid, refused or empty pagelines if not explicitely requested
            if (!pgl
                || do_detail == CONFIG_BOOLEAN_NO
                || (do_detail == CONFIG_BOOLEAN_AUTO && pageline_total_count(pgl) == 0 && netdata_zero_metrics_enabled != CONFIG_BOOLEAN_YES))
                continue;

            // "pagetype Node" + NUMA-NodeId + ZoneName + TypeName
            char setid[13+1+2+1+MAX_ZONETYPE_NAME+1+MAX_PAGETYPE_NAME+1];
            snprintfz(setid, 13+1+2+1+MAX_ZONETYPE_NAME+1+MAX_PAGETYPE_NAME, "pagetype_Node%d_%s_%s", pgl->node, pgl->zone, pgl->type);

            // Skip explicitely refused charts
            if (simple_pattern_matches(filter_types, setid))
                continue;

            // "Node" + NUMA-NodeID + ZoneName + TypeName
            char setname[4+1+MAX_ZONETYPE_NAME+1+MAX_PAGETYPE_NAME +1];
            snprintfz(setname, MAX_ZONETYPE_NAME + MAX_PAGETYPE_NAME, "Node %d %s %s",
                pgl->node, pgl->zone, pgl->type);

            st_nodezonetype[p] = rrdset_create_localhost(
                    "mem"
                    , setid
                    , NULL
                    , "pagetype"
                    , NULL
                    , setname
                    , "B"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_PAGETYPEINFO_NAME
                    , NETDATA_CHART_PRIO_MEM_PAGEFRAG + 1 + p
                    , update_every
                    , RRDSET_TYPE_STACKED
            );
            for (o = 0; o < pageorders_cnt; o++) {
                char dimid[3+1];
                snprintfz(dimid, 3, "%lu", o);
                char dimname[20+1];
                dim_name(dimname, o, pagesize);

                pgl->rd[o] = rrddim_add(st_nodezonetype[p], dimid, dimname, pagesize, 1, RRD_ALGORITHM_ABSOLUTE);
            }
        }
    }

    // --------------------------------------------------------------------
    // Update pagelines

    // Process each line
    p = 0;
    for (l=4; l<ff_lines; l++) {

        if (!pagetypeinfo_line_valid(ff, l))
            continue;

        size_t words = procfile_linewords(ff, l);

        if (words != 7+pageorders_cnt) {
            error("PLUGIN: PROC_PAGETYPEINFO: Unable to read line %lu, %lu words found instead of %lu", l+1, words, 7+pageorders_cnt);
            break;
        }

        for (o = 0; o < pageorders_cnt; o++) {
            // Reset counter
            if (p == 0)
                systemorders[o].size = 0;

            // Update orders of the current line
            pagelines[p].free_pages_size[o] = str2uint64_t(procfile_lineword(ff, l, o+6)) * 1 << o;

            // Update sum by order
            systemorders[o].size += pagelines[p].free_pages_size[o];
        }

        p++;
    }

    // --------------------------------------------------------------------
    // update RRD values

    // Global system per order
    if (st_order) {
        rrdset_next(st_order);
        for (o = 0; o < pageorders_cnt; o++) {
            rrddim_set_by_pointer(st_order, systemorders[o].rd, systemorders[o].size);
        }
        rrdset_done(st_order);
    }

    // Per Node-Zone-Type
    if (do_detail) {
        for (p = 0; p < pagelines_cnt; p++) {
            // Skip empty graphs
            if (!st_nodezonetype[p])
                continue;

            rrdset_next(st_nodezonetype[p]);
            for (o = 0; o < pageorders_cnt; o++)
                rrddim_set_by_pointer(st_nodezonetype[p], pagelines[p].rd[o], pagelines[p].free_pages_size[o]);

            rrdset_done(st_nodezonetype[p]);
        }
    }

    return 0;
}

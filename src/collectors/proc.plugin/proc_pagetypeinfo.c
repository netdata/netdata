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

// One line of /proc/pagetypeinfo, identified by (node, zone, type).
// The kernel prints only populated zones, so memory hotplug can add or remove lines at runtime.
struct pageline {
    int node;
    char *zone;
    char *type;
    bool seen; // found in the current read
    uint64_t free_pages_size[MAX_PAGETYPE_ORDER];
    RRDSET *st;
    RRDDIM *rd[MAX_PAGETYPE_ORDER];
};

// Sum of all orders
struct systemorder {
    uint64_t size;
    RRDDIM *rd;
};

static struct pageline *pagelines = NULL;
static size_t pagelines_cnt = 0, pagelines_size = 0;

static inline uint64_t pageline_total_count(struct pageline *p) {
    uint64_t sum = 0, o;
    for (o=0; o<MAX_PAGETYPE_ORDER; o++)
        sum += p->free_pages_size[o];
    return sum;
}

static inline bool pageline_matches(struct pageline *pgl, int node, const char *zone, const char *type) {
    return pgl->node == node && strcmp(pgl->zone, zone) == 0 && strcmp(pgl->type, type) == 0;
}

// Lines usually keep their order, so try the slot after the previous match first.
static struct pageline *pageline_find(size_t *hint, int node, const char *zone, const char *type) {
    if (*hint < pagelines_cnt && pageline_matches(&pagelines[*hint], node, zone, type))
        return &pagelines[(*hint)++];

    for (size_t i = 0; i < pagelines_cnt; i++) {
        if (pageline_matches(&pagelines[i], node, zone, type)) {
            *hint = i + 1;
            return &pagelines[i];
        }
    }

    return NULL;
}

static struct pageline *pageline_add(int node, const char *zone, const char *type) {
    if (pagelines_cnt == pagelines_size) {
        pagelines_size = pagelines_size ? pagelines_size * 2 : 16;
        pagelines = reallocz(pagelines, pagelines_size * sizeof(struct pageline));
    }

    struct pageline *pgl = &pagelines[pagelines_cnt++];
    memset(pgl, 0, sizeof(*pgl));
    pgl->node = node;
    pgl->zone = strdupz(zone);
    pgl->type = strdupz(type);
    return pgl;
}

// Check if a line of /proc/pagetypeinfo is valid to use
// Free block lines starts by "Node" && 4th col is "type"
#define pagetypeinfo_line_valid(ff, l) (strncmp(procfile_lineword(ff, l, 0), "Node", 4) == 0 && strncmp(procfile_lineword(ff, l, 4), "type", 4) == 0)

// Dimension name from the order
#define dim_name(s, o, pagesize) (snprintfz(s, 16,"%ldKB (%lu)", (1 << o) * pagesize / 1024, o))

// Per-Numa Node & Zone & Type (full detail), decided once, on the first sample of the line.
static void pageline_chart_create(struct pageline *pgl, size_t index, int do_detail, SIMPLE_PATTERN *filter_types,
                                  size_t pageorders_cnt, long pagesize, int update_every) {
    // Skip refused or empty pagelines if not explicitly requested
    if (do_detail == CONFIG_BOOLEAN_NO ||
        (do_detail == CONFIG_BOOLEAN_AUTO && pageline_total_count(pgl) == 0))
        return;

    // "pagetype Node" + NUMA-NodeId + ZoneName + TypeName
    char setid[13+1+2+1+MAX_ZONETYPE_NAME+1+MAX_PAGETYPE_NAME+1];
    snprintfz(setid, sizeof(setid) - 1, "pagetype_Node%d_%s_%s", pgl->node, pgl->zone, pgl->type);

    // Skip explicitly refused charts
    if (simple_pattern_matches(filter_types, setid))
        return;

    // "Node" + NUMA-NodeID + ZoneName + TypeName
    char setname[4+1+MAX_ZONETYPE_NAME+1+MAX_PAGETYPE_NAME +1];
    snprintfz(setname, MAX_ZONETYPE_NAME + MAX_PAGETYPE_NAME, "Node %d %s %s", pgl->node, pgl->zone, pgl->type);

    pgl->st = rrdset_create_localhost(
            "mem"
            , setid
            , NULL
            , "pagetype"
            , "mem.pagetype"
            , setname
            , "B"
            , PLUGIN_PROC_NAME
            , PLUGIN_PROC_MODULE_PAGETYPEINFO_NAME
            , NETDATA_CHART_PRIO_MEM_PAGEFRAG + 1 + index
            , update_every
            , RRDSET_TYPE_STACKED
    );

    char node[50+1];
    snprintfz(node, sizeof(node) - 1, "node%d", pgl->node);
    rrdlabels_add(pgl->st->rrdlabels, "node_id", node, RRDLABEL_SRC_AUTO);
    rrdlabels_add(pgl->st->rrdlabels, "node_zone", pgl->zone, RRDLABEL_SRC_AUTO);
    rrdlabels_add(pgl->st->rrdlabels, "node_type", pgl->type, RRDLABEL_SRC_AUTO);

    for (size_t o = 0; o < pageorders_cnt; o++) {
        char dimid[3+1];
        snprintfz(dimid, sizeof(dimid) - 1, "%lu", o);
        char dimname[20+1];
        dim_name(dimname, o, pagesize);

        pgl->rd[o] = rrddim_add(pgl->st, dimid, dimname, pagesize, 1, RRD_ALGORITHM_ABSOLUTE);
    }
}

int do_proc_pagetypeinfo(int update_every, usec_t dt) {
    (void)dt;

    // Config
    static int do_global, do_detail;
    static SIMPLE_PATTERN *filter_types = NULL;

    // Parsed once at startup (the rows themselves live in pagelines, which can change at runtime)
    static struct systemorder systemorders[MAX_PAGETYPE_ORDER] = {};
    static long pagesize = 0;
    static size_t pageorders_cnt = 0;
    static bool initialized = false;

    // Handle
    static procfile *ff = NULL;
    static char ff_path[FILENAME_MAX + 1];

    // RRD Sets
    static RRDSET *st_order = NULL;

    // Local temp variables
    long unsigned int l, o, p;

    // --------------------------------------------------------------------
    // Startup: Init arch and open /proc/pagetypeinfo
    if (unlikely(!pagesize)) {
        pagesize = sysconf(_SC_PAGESIZE);
    }

    if(unlikely(!ff)) {
        snprintfz(ff_path, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, PLUGIN_PROC_MODULE_PAGETYPEINFO_NAME);
        ff = procfile_open(inicfg_get(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_PAGETYPEINFO, "filename to monitor", ff_path), " \t:", PROCFILE_FLAG_DEFAULT);

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

    size_t ff_lines = procfile_lines(ff);

    // --------------------------------------------------------------------
    // Init: configuration and the number of orders
    if(unlikely(!initialized)) {
        if(unlikely(!ff_lines)) {
            collector_error("PLUGIN: PROC_PAGETYPEINFO: Cannot read %s, zero lines reported.", ff_path);
            return 1;
        }

        // Configuration
        do_global = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_PAGETYPEINFO, "enable system summary", CONFIG_BOOLEAN_YES);
        do_detail = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_PAGETYPEINFO, "enable detail per-type", CONFIG_BOOLEAN_AUTO);
        filter_types = simple_pattern_create(
                inicfg_get(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_PAGETYPEINFO, "hide charts id matching", ""), NULL,
                SIMPLE_PATTERN_SUFFIX, true);

        // 4th line is the "Free pages count per migrate type at order". Just subtract these 8 words.
        size_t words = procfile_linewords(ff, 3);
        if (words < 9) {
            collector_error("PLUGIN: PROC_PAGETYPEINFO: Unable to parse Line 4 of %s", ff_path);
            return 1;
        }

        words -= 9;

        if (words > MAX_PAGETYPE_ORDER) {
            collector_error("PLUGIN: PROC_PAGETYPEINFO: pageorder found (%lu) is higher than max %d",
                  (long unsigned int) words, MAX_PAGETYPE_ORDER);
            return 1;
        }

        pageorders_cnt = words;
        initialized = true;
    }

    // --------------------------------------------------------------------
    // Update pagelines, matching each line to its (node, zone, type)

    for (p = 0; p < pagelines_cnt; p++)
        pagelines[p].seen = false;

    for (o = 0; o < pageorders_cnt; o++)
        systemorders[o].size = 0;

    bool global_complete = true;
    size_t lines_seen = 0, hint = 0;

    for (l=4; l<ff_lines; l++) {

        if (!pagetypeinfo_line_valid(ff, l))
            continue;

        size_t words = procfile_linewords(ff, l);

        if (words != 7+pageorders_cnt) {
            collector_error("PLUGIN: PROC_PAGETYPEINFO: Unable to read line %lu, %lu words found instead of %lu",
                            l+1, (long unsigned int) words, (long unsigned int) 7+pageorders_cnt);
            global_complete = false;
            continue;
        }

        int node = (int)strtoul(procfile_lineword(ff, l, 1), NULL, 10);
        const char *zone = procfile_lineword(ff, l, 3);
        const char *type = procfile_lineword(ff, l, 5);

        struct pageline *pgl = pageline_find(&hint, node, zone, type);
        if (unlikely(pgl && pgl->seen))
            continue; // the same line twice in one read

        bool added = false;
        if (unlikely(!pgl)) {
            pgl = pageline_add(node, zone, type);
            hint = pagelines_cnt;
            added = true;
        }

        for (o = 0; o < pageorders_cnt; o++) {
            // Update orders of the current line
            pgl->free_pages_size[o] = str2uint64_t(procfile_lineword(ff, l, o + 6), NULL) * 1 << o;

            // Update sum by order
            systemorders[o].size += pgl->free_pages_size[o];
        }

        pgl->seen = true;
        lines_seen++;

        if (unlikely(added))
            pageline_chart_create(pgl, pagelines_cnt - 1, do_detail, filter_types, pageorders_cnt, pagesize, update_every);
    }

    if (unlikely(!pagelines_cnt)) {
        collector_error("PLUGIN: PROC_PAGETYPEINFO: Unable to parse any valid line in %s", ff_path);
        return 1;
    }

    // --------------------------------------------------------------------
    // update RRD values

    // Global system per order: a gap when the read was incomplete
    if (unlikely(!st_order && do_global != CONFIG_BOOLEAN_NO)) {
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
            snprintfz(id, sizeof(id) - 1, "%lu", o);

            char name[20+1];
            dim_name(name, o, pagesize);

            systemorders[o].rd = rrddim_add(st_order, id, name, pagesize, 1, RRD_ALGORITHM_ABSOLUTE);
        }
    }

    if (st_order) {
        if (global_complete && lines_seen) {
            for (o = 0; o < pageorders_cnt; o++)
                rrddim_set_by_pointer(st_order, systemorders[o].rd, systemorders[o].size);
        }
        rrdset_done(st_order);
    }

    // Per Node-Zone-Type: lines missing from this read get a gap
    for (p = 0; p < pagelines_cnt; p++) {
        struct pageline *pgl = &pagelines[p];

        // Skip empty graphs
        if (!pgl->st)
            continue;

        if (pgl->seen) {
            for (o = 0; o < pageorders_cnt; o++)
                rrddim_set_by_pointer(pgl->st, pgl->rd[o], pgl->free_pages_size[o]);
        }
        rrdset_done(pgl->st);
    }

    return 0;
}

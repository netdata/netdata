// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

#define PLUGIN_PROC_MODULE_VMSTAT_NAME "/proc/vmstat"

#define OOM_KILL_STRING "oom_kill"

#define _COMMON_PLUGIN_NAME PLUGIN_PROC_NAME
#define _COMMON_PLUGIN_MODULE_NAME PLUGIN_PROC_MODULE_VMSTAT_NAME
#include "../common-contexts/common-contexts.h"


int do_proc_vmstat(int update_every, usec_t dt) {
    (void)dt;

    static procfile *ff = NULL;
    static int do_swapio = -1, do_io = -1, do_pgfaults = -1, do_oom_kill = -1, do_numa = -1, do_thp = -1, do_zswapio = -1, do_balloon = -1, do_ksm = -1;
    static int has_numa = -1;

    static ARL_BASE *arl_base = NULL;
    static unsigned long long numa_foreign = 0ULL;
    static unsigned long long numa_hint_faults = 0ULL;
    static unsigned long long numa_hint_faults_local = 0ULL;
    static unsigned long long numa_huge_pte_updates = 0ULL;
    static unsigned long long numa_interleave = 0ULL;
    static unsigned long long numa_local = 0ULL;
    static unsigned long long numa_other = 0ULL;
    static unsigned long long numa_pages_migrated = 0ULL;
    static unsigned long long numa_pte_updates = 0ULL;
    static unsigned long long pgfault = 0ULL;
    static unsigned long long pgmajfault = 0ULL;
    static unsigned long long pgpgin = 0ULL;
    static unsigned long long pgpgout = 0ULL;
    static unsigned long long pswpin = 0ULL;
    static unsigned long long pswpout = 0ULL;
    static unsigned long long oom_kill = 0ULL;

    // THP page migration
//    static unsigned long long pgmigrate_success = 0ULL;
//    static unsigned long long pgmigrate_fail = 0ULL;
//    static unsigned long long thp_migration_success = 0ULL;
//    static unsigned long long thp_migration_fail = 0ULL;
//    static unsigned long long thp_migration_split = 0ULL;

    // Compaction cost model
    // https://lore.kernel.org/lkml/20121022080525.GB2198@suse.de/
//    static unsigned long long compact_migrate_scanned = 0ULL;
//    static unsigned long long compact_free_scanned = 0ULL;
//    static unsigned long long compact_isolated = 0ULL;

    // THP defragmentation
    static unsigned long long compact_stall = 0ULL; // incremented when an application stalls allocating THP
    static unsigned long long compact_fail = 0ULL; // defragmentation events that failed
    static unsigned long long compact_success = 0ULL; // defragmentation events that succeeded

    // ?
//    static unsigned long long compact_daemon_wake = 0ULL;
//    static unsigned long long compact_daemon_migrate_scanned = 0ULL;
//    static unsigned long long compact_daemon_free_scanned = 0ULL;

    // ?
//    static unsigned long long htlb_buddy_alloc_success = 0ULL;
//    static unsigned long long htlb_buddy_alloc_fail = 0ULL;

    // ?
//    static unsigned long long cma_alloc_success = 0ULL;
//    static unsigned long long cma_alloc_fail = 0ULL;

    // ?
//    static unsigned long long unevictable_pgs_culled = 0ULL;
//    static unsigned long long unevictable_pgs_scanned = 0ULL;
//    static unsigned long long unevictable_pgs_rescued = 0ULL;
//    static unsigned long long unevictable_pgs_mlocked = 0ULL;
//    static unsigned long long unevictable_pgs_munlocked = 0ULL;
//    static unsigned long long unevictable_pgs_cleared = 0ULL;
//    static unsigned long long unevictable_pgs_stranded = 0ULL;

    // THP handling of page faults
    static unsigned long long thp_fault_alloc = 0ULL; // is incremented every time a huge page is successfully allocated to handle a page fault. This applies to both the first time a page is faulted and for COW faults.
    static unsigned long long thp_fault_fallback = 0ULL; // is incremented if a page fault fails to allocate a huge page and instead falls back to using small pages.
    static unsigned long long thp_fault_fallback_charge = 0ULL; // is incremented if a page fault fails to charge a huge page and instead falls back to using small pages even though the allocation was successful.

    // khugepaged collapsing of small pages into huge pages
    static unsigned long long thp_collapse_alloc = 0ULL; // is incremented by khugepaged when it has found a range of pages to collapse into one huge page and has successfully allocated a new huge page to store the data.
    static unsigned long long thp_collapse_alloc_failed = 0ULL; // is incremented if khugepaged found a range of pages that should be collapsed into one huge page but failed the allocation.

    // THP handling of file allocations
    static unsigned long long thp_file_alloc = 0ULL; // is incremented every time a file huge page is successfully allocated
    static unsigned long long thp_file_fallback = 0ULL; // is incremented if a file huge page is attempted to be allocated but fails and instead falls back to using small pages
    static unsigned long long thp_file_fallback_charge = 0ULL; // is incremented if a file huge page cannot be charged and instead falls back to using small pages even though the allocation was successful
    static unsigned long long thp_file_mapped = 0ULL; // is incremented every time a file huge page is mapped into user address space

    // THP splitting of huge pages into small pages
    static unsigned long long thp_split_page = 0ULL;
    static unsigned long long thp_split_page_failed = 0ULL;
    static unsigned long long thp_deferred_split_page = 0ULL; // is incremented when a huge page is put onto split queue. This happens when a huge page is partially unmapped and splitting it would free up some memory. Pages on split queue are going to be split under memory pressure
    static unsigned long long thp_split_pmd = 0ULL; // is incremented every time a PMD split into table of PTEs. This can happen, for instance, when application calls mprotect() or munmap() on part of huge page. It doesn’t split huge page, only page table entry

    // ?
//    static unsigned long long thp_scan_exceed_none_pte = 0ULL;
//    static unsigned long long thp_scan_exceed_swap_pte = 0ULL;
//    static unsigned long long thp_scan_exceed_share_pte = 0ULL;
//    static unsigned long long thp_split_pud = 0ULL;

    // THP Zero Huge Page
    static unsigned long long thp_zero_page_alloc = 0ULL; // is incremented every time a huge zero page used for thp is successfully allocated. Note, it doesn’t count every map of the huge zero page, only its allocation
    static unsigned long long thp_zero_page_alloc_failed = 0ULL; // is incremented if kernel fails to allocate huge zero page and falls back to using small pages

    // THP Swap Out
    static unsigned long long thp_swpout = 0ULL; // is incremented every time a huge page is swapout in one piece without splitting
    static unsigned long long thp_swpout_fallback = 0ULL; // is incremented if a huge page has to be split before swapout. Usually because failed to allocate some continuous swap space for the huge page

    // memory ballooning
    // Current size of balloon is (balloon_inflate - balloon_deflate) pages
    static unsigned long long balloon_inflate = 0ULL;
    static unsigned long long balloon_deflate = 0ULL;
    static unsigned long long balloon_migrate = 0ULL;

    // ?
//    static unsigned long long swap_ra = 0ULL;
//    static unsigned long long swap_ra_hit = 0ULL;

    static unsigned long long ksm_swpin_copy = 0ULL; // is incremented every time a KSM page is copied when swapping in
    static unsigned long long cow_ksm = 0ULL; // is incremented every time a KSM page triggers copy on write (COW) when users try to write to a KSM page, we have to make a copy

    // zswap
    static unsigned long long zswpin = 0ULL;
    static unsigned long long zswpout = 0ULL;

    // ?
//    static unsigned long long direct_map_level2_splits = 0ULL;
//    static unsigned long long direct_map_level3_splits = 0ULL;
//    static unsigned long long nr_unstable = 0ULL;

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/vmstat");
        ff = procfile_open(inicfg_get(&netdata_config, "plugin:proc:/proc/vmstat", "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 0; // we return 0, so that we will retry to open it next time

    size_t lines = procfile_lines(ff), l;

    if(unlikely(!arl_base)) {
        do_swapio = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/vmstat", "swap i/o", CONFIG_BOOLEAN_AUTO);
        do_io = inicfg_get_boolean(&netdata_config, "plugin:proc:/proc/vmstat", "disk i/o", CONFIG_BOOLEAN_YES);
        do_pgfaults = inicfg_get_boolean(&netdata_config, "plugin:proc:/proc/vmstat", "memory page faults", CONFIG_BOOLEAN_YES);
        do_oom_kill = inicfg_get_boolean(&netdata_config, "plugin:proc:/proc/vmstat", "out of memory kills", CONFIG_BOOLEAN_AUTO);
        do_numa = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/vmstat", "system-wide numa metric summary", CONFIG_BOOLEAN_AUTO);
        do_thp = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/vmstat", "transparent huge pages", CONFIG_BOOLEAN_AUTO);
        do_zswapio = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/vmstat", "zswap i/o", CONFIG_BOOLEAN_AUTO);
        do_balloon = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/vmstat", "memory ballooning", CONFIG_BOOLEAN_AUTO);
        do_ksm = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/proc/vmstat", "kernel same memory", CONFIG_BOOLEAN_AUTO);

        arl_base = arl_create("vmstat", NULL, 60);
        arl_expect(arl_base, "pgfault", &pgfault);
        arl_expect(arl_base, "pgmajfault", &pgmajfault);
        arl_expect(arl_base, "pgpgin", &pgpgin);
        arl_expect(arl_base, "pgpgout", &pgpgout);
        arl_expect(arl_base, "pswpin", &pswpin);
        arl_expect(arl_base, "pswpout", &pswpout);

        int has_oom_kill = 0;
        
        for (l = 0; l < lines; l++) {
            if (!strcmp(procfile_lineword(ff, l, 0), OOM_KILL_STRING)) {
                has_oom_kill = 1;
                break;
            }
        }

        if (has_oom_kill)
            arl_expect(arl_base, OOM_KILL_STRING, &oom_kill);
        else
            do_oom_kill = CONFIG_BOOLEAN_NO;

        if (do_numa == CONFIG_BOOLEAN_YES || (do_numa == CONFIG_BOOLEAN_AUTO && get_numa_node_count() >= 2)) {
            arl_expect(arl_base, "numa_foreign", &numa_foreign);
            arl_expect(arl_base, "numa_hint_faults_local", &numa_hint_faults_local);
            arl_expect(arl_base, "numa_hint_faults", &numa_hint_faults);
            arl_expect(arl_base, "numa_huge_pte_updates", &numa_huge_pte_updates);
            arl_expect(arl_base, "numa_interleave", &numa_interleave);
            arl_expect(arl_base, "numa_local", &numa_local);
            arl_expect(arl_base, "numa_other", &numa_other);
            arl_expect(arl_base, "numa_pages_migrated", &numa_pages_migrated);
            arl_expect(arl_base, "numa_pte_updates", &numa_pte_updates);
        } else {
            // Do not expect numa metrics when they are not needed.
            // By not adding them, the ARL will stop processing the file
            // when all the expected metrics are collected.
            // Also ARL will not parse their values.
            has_numa = 0;
            do_numa = CONFIG_BOOLEAN_NO;
        }

        if(do_thp == CONFIG_BOOLEAN_YES || do_thp == CONFIG_BOOLEAN_AUTO) {
//            arl_expect(arl_base, "pgmigrate_success", &pgmigrate_success);
//            arl_expect(arl_base, "pgmigrate_fail", &pgmigrate_fail);
//            arl_expect(arl_base, "thp_migration_success", &thp_migration_success);
//            arl_expect(arl_base, "thp_migration_fail", &thp_migration_fail);
//            arl_expect(arl_base, "thp_migration_split", &thp_migration_split);
//            arl_expect(arl_base, "compact_migrate_scanned", &compact_migrate_scanned);
//            arl_expect(arl_base, "compact_free_scanned", &compact_free_scanned);
//            arl_expect(arl_base, "compact_isolated", &compact_isolated);
            arl_expect(arl_base, "compact_stall", &compact_stall);
            arl_expect(arl_base, "compact_fail", &compact_fail);
            arl_expect(arl_base, "compact_success", &compact_success);
//            arl_expect(arl_base, "compact_daemon_wake", &compact_daemon_wake);
//            arl_expect(arl_base, "compact_daemon_migrate_scanned", &compact_daemon_migrate_scanned);
//            arl_expect(arl_base, "compact_daemon_free_scanned", &compact_daemon_free_scanned);
            arl_expect(arl_base, "thp_fault_alloc", &thp_fault_alloc);
            arl_expect(arl_base, "thp_fault_fallback", &thp_fault_fallback);
            arl_expect(arl_base, "thp_fault_fallback_charge", &thp_fault_fallback_charge);
            arl_expect(arl_base, "thp_collapse_alloc", &thp_collapse_alloc);
            arl_expect(arl_base, "thp_collapse_alloc_failed", &thp_collapse_alloc_failed);
            arl_expect(arl_base, "thp_file_alloc", &thp_file_alloc);
            arl_expect(arl_base, "thp_file_fallback", &thp_file_fallback);
            arl_expect(arl_base, "thp_file_fallback_charge", &thp_file_fallback_charge);
            arl_expect(arl_base, "thp_file_mapped", &thp_file_mapped);
            arl_expect(arl_base, "thp_split_page", &thp_split_page);
            arl_expect(arl_base, "thp_split_page_failed", &thp_split_page_failed);
            arl_expect(arl_base, "thp_deferred_split_page", &thp_deferred_split_page);
            arl_expect(arl_base, "thp_split_pmd", &thp_split_pmd);
            arl_expect(arl_base, "thp_zero_page_alloc", &thp_zero_page_alloc);
            arl_expect(arl_base, "thp_zero_page_alloc_failed", &thp_zero_page_alloc_failed);
            arl_expect(arl_base, "thp_swpout", &thp_swpout);
            arl_expect(arl_base, "thp_swpout_fallback", &thp_swpout_fallback);
        }

        if(do_balloon == CONFIG_BOOLEAN_YES || do_balloon == CONFIG_BOOLEAN_AUTO) {
            arl_expect(arl_base, "balloon_inflate", &balloon_inflate);
            arl_expect(arl_base, "balloon_deflate", &balloon_deflate);
            arl_expect(arl_base, "balloon_migrate", &balloon_migrate);
        }

        if(do_ksm == CONFIG_BOOLEAN_YES || do_ksm == CONFIG_BOOLEAN_AUTO) {
            arl_expect(arl_base, "ksm_swpin_copy", &ksm_swpin_copy);
            arl_expect(arl_base, "cow_ksm", &cow_ksm);
        }

        if(do_zswapio == CONFIG_BOOLEAN_YES || do_zswapio == CONFIG_BOOLEAN_AUTO) {
            arl_expect(arl_base, "zswpin", &zswpin);
            arl_expect(arl_base, "zswpout", &zswpout);
        }
    }

    arl_begin(arl_base);
    for(l = 0; l < lines ;l++) {
        size_t words = procfile_linewords(ff, l);
        if(unlikely(words < 2)) {
            if(unlikely(words)) collector_error("Cannot read /proc/vmstat line %zu. Expected 2 params, read %zu.", l, words);
            continue;
        }

        if(unlikely(arl_check(arl_base,
                procfile_lineword(ff, l, 0),
                procfile_lineword(ff, l, 1)))) break;
    }

    // --------------------------------------------------------------------

    if (is_mem_swap_enabled && (do_swapio == CONFIG_BOOLEAN_YES || do_swapio == CONFIG_BOOLEAN_AUTO)) {
        do_swapio = CONFIG_BOOLEAN_YES;

        static RRDSET *st_swapio = NULL;
        static RRDDIM *rd_in = NULL, *rd_out = NULL;

        if(unlikely(!st_swapio)) {
            st_swapio = rrdset_create_localhost(
                    "mem"
                    , "swapio"
                    , NULL
                    , "swap"
                    , NULL
                    , "Swap I/O"
                    , "KiB/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_VMSTAT_NAME
                    , NETDATA_CHART_PRIO_MEM_SWAPIO
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rd_in  = rrddim_add(st_swapio, "in",  NULL,  sysconf(_SC_PAGESIZE), 1024, RRD_ALGORITHM_INCREMENTAL);
            rd_out = rrddim_add(st_swapio, "out", NULL, -sysconf(_SC_PAGESIZE), 1024, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_swapio, rd_in, pswpin);
        rrddim_set_by_pointer(st_swapio, rd_out, pswpout);
        rrdset_done(st_swapio);
    }

    // --------------------------------------------------------------------

    if(do_io) {
        static RRDSET *st_io = NULL;
        static RRDDIM *rd_in = NULL, *rd_out = NULL;

        if(unlikely(!st_io)) {
            st_io = rrdset_create_localhost(
                    "system"
                    , "pgpgio"
                    , NULL
                    , "disk"
                    , NULL
                    , "Memory Paged from/to disk"
                    , "KiB/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_VMSTAT_NAME
                    , NETDATA_CHART_PRIO_SYSTEM_PGPGIO
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rd_in  = rrddim_add(st_io, "in",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_out = rrddim_add(st_io, "out", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_io, rd_in, pgpgin);
        rrddim_set_by_pointer(st_io, rd_out, pgpgout);
        rrdset_done(st_io);
    }

    // --------------------------------------------------------------------

    if(do_pgfaults) {
        common_mem_pgfaults(pgfault, pgmajfault, update_every);
    }

        // --------------------------------------------------------------------

    if (do_oom_kill == CONFIG_BOOLEAN_YES || do_oom_kill == CONFIG_BOOLEAN_AUTO) {
        static RRDSET *st_oom_kill = NULL;
        static RRDDIM *rd_oom_kill = NULL;

        do_oom_kill = CONFIG_BOOLEAN_YES;

        if(unlikely(!st_oom_kill)) {
            st_oom_kill = rrdset_create_localhost(
                    "mem"
                    , "oom_kill"
                    , NULL
                    , "OOM kills"
                    , NULL
                    , "Out of Memory Kills"
                    , "kills/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_VMSTAT_NAME
                    , NETDATA_CHART_PRIO_MEM_SYSTEM_OOM_KILL
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_oom_kill = rrddim_add(st_oom_kill, "kills", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_oom_kill, rd_oom_kill, oom_kill);
        rrdset_done(st_oom_kill);
    }

    // --------------------------------------------------------------------

    // Ondemand criteria for NUMA. Since this won't change at run time, we
    // check it only once. We check whether the node count is >= 2 because
    // single-node systems have uninteresting statistics (since all accesses
    // are local).
    if(unlikely(has_numa == -1))

        has_numa = (numa_local || numa_foreign || numa_interleave || numa_other || numa_pte_updates ||
                     numa_huge_pte_updates || numa_hint_faults || numa_hint_faults_local || numa_pages_migrated) ? 1 : 0;

    if(do_numa == CONFIG_BOOLEAN_YES || (do_numa == CONFIG_BOOLEAN_AUTO && has_numa)) {
        do_numa = CONFIG_BOOLEAN_YES;

        static RRDSET *st_numa = NULL;
        static RRDDIM *rd_local = NULL, *rd_foreign = NULL, *rd_interleave = NULL, *rd_other = NULL, *rd_pte_updates = NULL, *rd_huge_pte_updates = NULL, *rd_hint_faults = NULL, *rd_hint_faults_local = NULL, *rd_pages_migrated = NULL;

        if(unlikely(!st_numa)) {
            st_numa = rrdset_create_localhost(
                    "mem"
                    , "numa"
                    , NULL
                    , "numa"
                    , NULL
                    , "NUMA events"
                    , "events/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_VMSTAT_NAME
                    , NETDATA_CHART_PRIO_MEM_NUMA
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            // These depend on CONFIG_NUMA in the kernel.
            rd_local             = rrddim_add(st_numa, "local",             NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_foreign           = rrddim_add(st_numa, "foreign",           NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_interleave        = rrddim_add(st_numa, "interleave",        NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_other             = rrddim_add(st_numa, "other",             NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            // The following stats depend on CONFIG_NUMA_BALANCING in the
            // kernel.
            rd_pte_updates       = rrddim_add(st_numa, "pte_updates",       NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_huge_pte_updates  = rrddim_add(st_numa, "huge_pte_updates",  NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_hint_faults       = rrddim_add(st_numa, "hint_faults",       NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_hint_faults_local = rrddim_add(st_numa, "hint_faults_local", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_pages_migrated    = rrddim_add(st_numa, "pages_migrated",    NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_numa, rd_local,             numa_local);
        rrddim_set_by_pointer(st_numa, rd_foreign,           numa_foreign);
        rrddim_set_by_pointer(st_numa, rd_interleave,        numa_interleave);
        rrddim_set_by_pointer(st_numa, rd_other,             numa_other);

        rrddim_set_by_pointer(st_numa, rd_pte_updates,       numa_pte_updates);
        rrddim_set_by_pointer(st_numa, rd_huge_pte_updates,  numa_huge_pte_updates);
        rrddim_set_by_pointer(st_numa, rd_hint_faults,       numa_hint_faults);
        rrddim_set_by_pointer(st_numa, rd_hint_faults_local, numa_hint_faults_local);
        rrddim_set_by_pointer(st_numa, rd_pages_migrated,    numa_pages_migrated);

        rrdset_done(st_numa);
    }

    // --------------------------------------------------------------------

    if (do_balloon == CONFIG_BOOLEAN_YES || do_balloon == CONFIG_BOOLEAN_AUTO) {
        do_balloon = CONFIG_BOOLEAN_YES;

        static RRDSET *st_balloon = NULL;
        static RRDDIM *rd_inflate = NULL, *rd_deflate = NULL, *rd_migrate = NULL;

        if(unlikely(!st_balloon)) {
            st_balloon = rrdset_create_localhost(
                    "mem"
                    , "balloon"
                    , NULL
                    , "balloon"
                    , NULL
                    , "Memory Ballooning Operations"
                    , "KiB/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_VMSTAT_NAME
                    , NETDATA_CHART_PRIO_MEM_BALLOON
                    , update_every
                    , RRDSET_TYPE_LINE
                    );

            rd_inflate  = rrddim_add(st_balloon, "inflate", NULL, sysconf(_SC_PAGESIZE), 1024, RRD_ALGORITHM_INCREMENTAL);
            rd_deflate = rrddim_add(st_balloon, "deflate", NULL, -sysconf(_SC_PAGESIZE), 1024, RRD_ALGORITHM_INCREMENTAL);
            rd_migrate = rrddim_add(st_balloon, "migrate", NULL, sysconf(_SC_PAGESIZE), 1024, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_balloon, rd_inflate, balloon_inflate);
        rrddim_set_by_pointer(st_balloon, rd_deflate, balloon_deflate);
        rrddim_set_by_pointer(st_balloon, rd_migrate, balloon_migrate);

        rrdset_done(st_balloon);
    }

    // --------------------------------------------------------------------

    if (is_mem_zswap_enabled && (do_zswapio == CONFIG_BOOLEAN_YES || do_zswapio == CONFIG_BOOLEAN_AUTO)) {
        do_zswapio = CONFIG_BOOLEAN_YES;

        static RRDSET *st_zswapio = NULL;
        static RRDDIM *rd_in = NULL, *rd_out = NULL;

        if(unlikely(!st_zswapio)) {
            st_zswapio = rrdset_create_localhost(
                    "mem"
                    , "zswapio"
                    , NULL
                    , "zswap"
                    , NULL
                    , "ZSwap I/O"
                    , "KiB/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_VMSTAT_NAME
                    , NETDATA_CHART_PRIO_MEM_ZSWAPIO
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rd_in  = rrddim_add(st_zswapio, "in", NULL, sysconf(_SC_PAGESIZE), 1024, RRD_ALGORITHM_INCREMENTAL);
            rd_out = rrddim_add(st_zswapio, "out", NULL, -sysconf(_SC_PAGESIZE), 1024, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_zswapio, rd_in, zswpin);
        rrddim_set_by_pointer(st_zswapio, rd_out, zswpout);
        rrdset_done(st_zswapio);
    }

    // --------------------------------------------------------------------

    if (is_mem_ksm_enabled && (do_ksm == CONFIG_BOOLEAN_YES || do_ksm == CONFIG_BOOLEAN_AUTO)) {
        do_ksm = CONFIG_BOOLEAN_YES;

        static RRDSET *st_ksm_cow = NULL;
        static RRDDIM *rd_swapin = NULL, *rd_write = NULL;

        if(unlikely(!st_ksm_cow)) {
            st_ksm_cow = rrdset_create_localhost(
                    "mem"
            , "ksm_cow"
            , NULL
            , "ksm"
            , NULL
            , "KSM Copy On Write Operations"
            , "KiB/s"
            , PLUGIN_PROC_NAME
            , PLUGIN_PROC_MODULE_VMSTAT_NAME
            , NETDATA_CHART_PRIO_MEM_KSM_COW
            , update_every
            , RRDSET_TYPE_LINE
            );

            rd_swapin  = rrddim_add(st_ksm_cow, "swapin", NULL, sysconf(_SC_PAGESIZE), 1024, RRD_ALGORITHM_INCREMENTAL);
            rd_write = rrddim_add(st_ksm_cow, "write", NULL, sysconf(_SC_PAGESIZE), 1024, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_ksm_cow, rd_swapin, ksm_swpin_copy);
        rrddim_set_by_pointer(st_ksm_cow, rd_write, cow_ksm);

        rrdset_done(st_ksm_cow);
    }

    // --------------------------------------------------------------------

    if(do_thp == CONFIG_BOOLEAN_YES || do_thp == CONFIG_BOOLEAN_AUTO) {

        static RRDSET *st_thp_fault = NULL;
        static RRDDIM *rd_alloc = NULL, *rd_fallback = NULL, *rd_fallback_charge = NULL;

        if(unlikely(!st_thp_fault)) {
            st_thp_fault = rrdset_create_localhost(
                    "mem"
                    , "thp_faults"
                    , NULL
                    , "hugepages"
                    , NULL
                    , "Transparent Huge Page Fault Allocations"
                    , "events/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_VMSTAT_NAME
                    , NETDATA_CHART_PRIO_MEM_HUGEPAGES_FAULTS
                    , update_every
                    , RRDSET_TYPE_LINE
                    );

            rd_alloc  = rrddim_add(st_thp_fault, "alloc", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_fallback = rrddim_add(st_thp_fault, "fallback", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_fallback_charge = rrddim_add(st_thp_fault, "fallback_charge", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_thp_fault, rd_alloc, thp_fault_alloc);
        rrddim_set_by_pointer(st_thp_fault, rd_fallback, thp_fault_fallback);
        rrddim_set_by_pointer(st_thp_fault, rd_fallback_charge, thp_fault_fallback_charge);

        rrdset_done(st_thp_fault);
    }

    if (do_thp == CONFIG_BOOLEAN_YES || do_thp == CONFIG_BOOLEAN_AUTO) {
        static RRDSET *st_thp_file = NULL;
        static RRDDIM *rd_alloc = NULL, *rd_fallback = NULL, *rd_fallback_charge = NULL, *rd_mapped = NULL;

        if(unlikely(!st_thp_file)) {
            st_thp_file = rrdset_create_localhost(
                    "mem"
                    , "thp_file"
                    , NULL
                    , "hugepages"
                    , NULL
                    , "Transparent Huge Page File Allocations"
                    , "events/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_VMSTAT_NAME
                    , NETDATA_CHART_PRIO_MEM_HUGEPAGES_FILE
                    , update_every
                    , RRDSET_TYPE_LINE
                    );

            rd_alloc  = rrddim_add(st_thp_file, "alloc", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_fallback = rrddim_add(st_thp_file, "fallback", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_mapped = rrddim_add(st_thp_file, "mapped", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_fallback_charge = rrddim_add(st_thp_file, "fallback_charge", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_thp_file, rd_alloc, thp_file_alloc);
        rrddim_set_by_pointer(st_thp_file, rd_fallback, thp_file_fallback);
        rrddim_set_by_pointer(st_thp_file, rd_mapped, thp_file_fallback_charge);
        rrddim_set_by_pointer(st_thp_file, rd_fallback_charge, thp_file_fallback_charge);

        rrdset_done(st_thp_file);
    }

    if (do_thp == CONFIG_BOOLEAN_YES || do_thp == CONFIG_BOOLEAN_AUTO) {
        static RRDSET *st_thp_zero = NULL;
        static RRDDIM *rd_alloc = NULL, *rd_failed = NULL;

        if(unlikely(!st_thp_zero)) {
            st_thp_zero = rrdset_create_localhost(
                    "mem"
                    , "thp_zero"
                    , NULL
                    , "hugepages"
                    , NULL
                    , "Transparent Huge Zero Page Allocations"
                    , "events/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_VMSTAT_NAME
                    , NETDATA_CHART_PRIO_MEM_HUGEPAGES_ZERO
                    , update_every
                    , RRDSET_TYPE_LINE
                    );

            rd_alloc  = rrddim_add(st_thp_zero, "alloc", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_failed = rrddim_add(st_thp_zero, "failed", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_thp_zero, rd_alloc, thp_zero_page_alloc);
        rrddim_set_by_pointer(st_thp_zero, rd_failed, thp_zero_page_alloc_failed);

        rrdset_done(st_thp_zero);
    }

    if (do_thp == CONFIG_BOOLEAN_YES || do_thp == CONFIG_BOOLEAN_AUTO) {
        static RRDSET *st_khugepaged = NULL;
        static RRDDIM *rd_alloc = NULL, *rd_failed = NULL;

        if(unlikely(!st_khugepaged)) {
            st_khugepaged = rrdset_create_localhost(
                    "mem"
                    , "thp_collapse"
                    , NULL
                    , "hugepages"
                    , NULL
                    , "Transparent Huge Pages Collapsed by khugepaged"
                    , "events/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_VMSTAT_NAME
                    , NETDATA_CHART_PRIO_MEM_HUGEPAGES_KHUGEPAGED
                    , update_every
                    , RRDSET_TYPE_LINE
                    );

            rd_alloc  = rrddim_add(st_khugepaged, "alloc", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_failed = rrddim_add(st_khugepaged, "failed", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_khugepaged, rd_alloc, thp_collapse_alloc);
        rrddim_set_by_pointer(st_khugepaged, rd_failed, thp_collapse_alloc_failed);

        rrdset_done(st_khugepaged);
    }

    if (do_thp == CONFIG_BOOLEAN_YES || do_thp == CONFIG_BOOLEAN_AUTO) {
        static RRDSET *st_thp_split = NULL;
        static RRDDIM *rd_split = NULL, *rd_failed = NULL, *rd_deferred_split = NULL, *rd_split_pmd = NULL;

        if(unlikely(!st_thp_split)) {
            st_thp_split = rrdset_create_localhost(
                    "mem"
                    , "thp_split"
                    , NULL
                    , "hugepages"
                    , NULL
                    , "Transparent Huge Page Splits"
                    , "events/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_VMSTAT_NAME
                    , NETDATA_CHART_PRIO_MEM_HUGEPAGES_SPLITS
                    , update_every
                    , RRDSET_TYPE_LINE
                    );

            rd_split  = rrddim_add(st_thp_split, "split", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_failed = rrddim_add(st_thp_split, "failed", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_split_pmd = rrddim_add(st_thp_split, "split_pmd", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_deferred_split = rrddim_add(st_thp_split, "split_deferred", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_thp_split, rd_split, thp_split_page);
        rrddim_set_by_pointer(st_thp_split, rd_failed, thp_split_page_failed);
        rrddim_set_by_pointer(st_thp_split, rd_split_pmd, thp_split_pmd);
        rrddim_set_by_pointer(st_thp_split, rd_deferred_split, thp_deferred_split_page);

        rrdset_done(st_thp_split);
    }

    if (do_thp == CONFIG_BOOLEAN_YES || do_thp == CONFIG_BOOLEAN_AUTO) {
        static RRDSET *st_tmp_swapout = NULL;
        static RRDDIM *rd_swapout = NULL, *rd_fallback = NULL;

        if(unlikely(!st_tmp_swapout)) {
            st_tmp_swapout = rrdset_create_localhost(
                    "mem"
                    , "thp_swapout"
                    , NULL
                    , "hugepages"
                    , NULL
                    , "Transparent Huge Pages Swap Out"
                    , "events/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_VMSTAT_NAME
                    , NETDATA_CHART_PRIO_MEM_HUGEPAGES_SWAPOUT
                    , update_every
                    , RRDSET_TYPE_LINE
                    );

            rd_swapout  = rrddim_add(st_tmp_swapout, "swapout", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_fallback = rrddim_add(st_tmp_swapout, "fallback", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_tmp_swapout, rd_swapout, thp_swpout);
        rrddim_set_by_pointer(st_tmp_swapout, rd_fallback, thp_swpout_fallback);

        rrdset_done(st_tmp_swapout);
    }

    if (do_thp == CONFIG_BOOLEAN_YES || do_thp == CONFIG_BOOLEAN_AUTO) {
        static RRDSET *st_thp_compact = NULL;
        static RRDDIM *rd_success = NULL, *rd_fail = NULL, *rd_stall = NULL;

        if(unlikely(!st_thp_compact)) {
            st_thp_compact = rrdset_create_localhost(
                    "mem"
                    , "thp_compact"
                    , NULL
                    , "hugepages"
                    , NULL
                    , "Transparent Huge Pages Compaction"
                    , "events/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_VMSTAT_NAME
                    , NETDATA_CHART_PRIO_MEM_HUGEPAGES_COMPACT
                    , update_every
                    , RRDSET_TYPE_LINE
                    );

            rd_success  = rrddim_add(st_thp_compact, "success", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_fail = rrddim_add(st_thp_compact, "fail", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_stall = rrddim_add(st_thp_compact, "stall", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_thp_compact, rd_success, compact_success);
        rrddim_set_by_pointer(st_thp_compact, rd_fail, compact_fail);
        rrddim_set_by_pointer(st_thp_compact, rd_stall, compact_stall);

        rrdset_done(st_thp_compact);
    }

    return 0;
}


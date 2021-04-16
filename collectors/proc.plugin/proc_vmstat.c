// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

#define PLUGIN_PROC_MODULE_VMSTAT_NAME "/proc/vmstat"

#define OOM_KILL_STRING "oom_kill"

int do_proc_vmstat(int update_every, usec_t dt) {
    (void)dt;

    static procfile *ff = NULL;
    static int do_swapio = -1, do_io = -1, do_pgfaults = -1, do_oom_kill = -1, do_numa = -1;
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

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/vmstat");
        ff = procfile_open(config_get("plugin:proc:/proc/vmstat", "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 0; // we return 0, so that we will retry to open it next time

    size_t lines = procfile_lines(ff), l;

    if(unlikely(!arl_base)) {
        do_swapio = config_get_boolean_ondemand("plugin:proc:/proc/vmstat", "swap i/o", CONFIG_BOOLEAN_AUTO);
        do_io = config_get_boolean("plugin:proc:/proc/vmstat", "disk i/o", CONFIG_BOOLEAN_YES);
        do_pgfaults = config_get_boolean("plugin:proc:/proc/vmstat", "memory page faults", CONFIG_BOOLEAN_YES);
        do_oom_kill = config_get_boolean("plugin:proc:/proc/vmstat", "out of memory kills", CONFIG_BOOLEAN_AUTO);
        do_numa = config_get_boolean_ondemand("plugin:proc:/proc/vmstat", "system-wide numa metric summary", CONFIG_BOOLEAN_AUTO);


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

        if(do_numa == CONFIG_BOOLEAN_YES || (do_numa == CONFIG_BOOLEAN_AUTO &&
                                             (get_numa_node_count() >= 2 ||
                                              netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
            arl_expect(arl_base, "numa_foreign", &numa_foreign);
            arl_expect(arl_base, "numa_hint_faults_local", &numa_hint_faults_local);
            arl_expect(arl_base, "numa_hint_faults", &numa_hint_faults);
            arl_expect(arl_base, "numa_huge_pte_updates", &numa_huge_pte_updates);
            arl_expect(arl_base, "numa_interleave", &numa_interleave);
            arl_expect(arl_base, "numa_local", &numa_local);
            arl_expect(arl_base, "numa_other", &numa_other);
            arl_expect(arl_base, "numa_pages_migrated", &numa_pages_migrated);
            arl_expect(arl_base, "numa_pte_updates", &numa_pte_updates);
        }
        else {
            // Do not expect numa metrics when they are not needed.
            // By not adding them, the ARL will stop processing the file
            // when all the expected metrics are collected.
            // Also ARL will not parse their values.
            has_numa = 0;
            do_numa = CONFIG_BOOLEAN_NO;
        }
    }

    arl_begin(arl_base);
    for(l = 0; l < lines ;l++) {
        size_t words = procfile_linewords(ff, l);
        if(unlikely(words < 2)) {
            if(unlikely(words)) error("Cannot read /proc/vmstat line %zu. Expected 2 params, read %zu.", l, words);
            continue;
        }

        if(unlikely(arl_check(arl_base,
                procfile_lineword(ff, l, 0),
                procfile_lineword(ff, l, 1)))) break;
    }

    // --------------------------------------------------------------------

    if(do_swapio == CONFIG_BOOLEAN_YES || (do_swapio == CONFIG_BOOLEAN_AUTO &&
                                           (pswpin || pswpout ||
                                            netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        do_swapio = CONFIG_BOOLEAN_YES;

        static RRDSET *st_swapio = NULL;
        static RRDDIM *rd_in = NULL, *rd_out = NULL;

        if(unlikely(!st_swapio)) {
            st_swapio = rrdset_create_localhost(
                    "system"
                    , "swapio"
                    , NULL
                    , "swap"
                    , NULL
                    , "Swap I/O"
                    , "KiB/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_VMSTAT_NAME
                    , NETDATA_CHART_PRIO_SYSTEM_SWAPIO
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rd_in  = rrddim_add(st_swapio, "in",  NULL,  sysconf(_SC_PAGESIZE), 1024, RRD_ALGORITHM_INCREMENTAL);
            rd_out = rrddim_add(st_swapio, "out", NULL, -sysconf(_SC_PAGESIZE), 1024, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st_swapio);

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
        else rrdset_next(st_io);

        rrddim_set_by_pointer(st_io, rd_in, pgpgin);
        rrddim_set_by_pointer(st_io, rd_out, pgpgout);
        rrdset_done(st_io);
    }

    // --------------------------------------------------------------------

    if(do_pgfaults) {
        static RRDSET *st_pgfaults = NULL;
        static RRDDIM *rd_minor = NULL, *rd_major = NULL;

        if(unlikely(!st_pgfaults)) {
            st_pgfaults = rrdset_create_localhost(
                    "mem"
                    , "pgfaults"
                    , NULL
                    , "system"
                    , NULL
                    , "Memory Page Faults"
                    , "faults/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_VMSTAT_NAME
                    , NETDATA_CHART_PRIO_MEM_SYSTEM_PGFAULTS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rrdset_flag_set(st_pgfaults, RRDSET_FLAG_DETAIL);

            rd_minor = rrddim_add(st_pgfaults, "minor", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_major = rrddim_add(st_pgfaults, "major", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st_pgfaults);

        rrddim_set_by_pointer(st_pgfaults, rd_minor, pgfault);
        rrddim_set_by_pointer(st_pgfaults, rd_major, pgmajfault);
        rrdset_done(st_pgfaults);
    }

        // --------------------------------------------------------------------

    if (do_oom_kill == CONFIG_BOOLEAN_YES ||
        (do_oom_kill == CONFIG_BOOLEAN_AUTO && (oom_kill || netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        static RRDSET *st_oom_kill = NULL;
        static RRDDIM *rd_oom_kill = NULL;

        do_oom_kill = CONFIG_BOOLEAN_YES;

        if(unlikely(!st_oom_kill)) {
            st_oom_kill = rrdset_create_localhost(
                    "mem"
                    , "oom_kill"
                    , NULL
                    , "system"
                    , NULL
                    , "Out of Memory Kills"
                    , "kills/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_VMSTAT_NAME
                    , NETDATA_CHART_PRIO_MEM_SYSTEM_OOM_KILL
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rrdset_flag_set(st_oom_kill, RRDSET_FLAG_DETAIL);

            rd_oom_kill = rrddim_add(st_oom_kill, "kills", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st_oom_kill);

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

            rrdset_flag_set(st_numa, RRDSET_FLAG_DETAIL);

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
        else rrdset_next(st_numa);

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

    return 0;
}


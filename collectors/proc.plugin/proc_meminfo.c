// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

#define PLUGIN_PROC_MODULE_MEMINFO_NAME "/proc/meminfo"
#define CONFIG_SECTION_PLUGIN_PROC_MEMINFO "plugin:" PLUGIN_PROC_CONFIG_NAME ":" PLUGIN_PROC_MODULE_MEMINFO_NAME

int do_proc_meminfo(int update_every, usec_t dt) {
    (void)dt;

    static procfile *ff = NULL;
    static int do_ram = -1, do_swap = -1, do_hwcorrupt = -1, do_committed = -1, do_writeback = -1, do_kernel = -1, do_slab = -1, do_hugepages = -1, do_transparent_hugepages = -1;
    static int do_percpu = 0;

    static ARL_BASE *arl_base = NULL;
    static ARL_ENTRY *arl_hwcorrupted = NULL, *arl_memavailable = NULL;

    static unsigned long long
            MemTotal = 0,
            MemFree = 0,
            MemAvailable = 0,
            Buffers = 0,
            Cached = 0,
            //SwapCached = 0,
            //Active = 0,
            //Inactive = 0,
            //ActiveAnon = 0,
            //InactiveAnon = 0,
            //ActiveFile = 0,
            //InactiveFile = 0,
            //Unevictable = 0,
            //Mlocked = 0,
            SwapTotal = 0,
            SwapFree = 0,
            Dirty = 0,
            Writeback = 0,
            //AnonPages = 0,
            //Mapped = 0,
            Shmem = 0,
            Slab = 0,
            SReclaimable = 0,
            SUnreclaim = 0,
            KernelStack = 0,
            PageTables = 0,
            NFS_Unstable = 0,
            Bounce = 0,
            WritebackTmp = 0,
            //CommitLimit = 0,
            Committed_AS = 0,
            //VmallocTotal = 0,
            VmallocUsed = 0,
            //VmallocChunk = 0,
            Percpu = 0,
            AnonHugePages = 0,
            ShmemHugePages = 0,
            HugePages_Total = 0,
            HugePages_Free = 0,
            HugePages_Rsvd = 0,
            HugePages_Surp = 0,
            Hugepagesize = 0,
            //DirectMap4k = 0,
            //DirectMap2M = 0,
            HardwareCorrupted = 0;

    if(unlikely(!arl_base)) {
        do_ram          = config_get_boolean(CONFIG_SECTION_PLUGIN_PROC_MEMINFO, "system ram", 1);
        do_swap         = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_MEMINFO, "system swap", CONFIG_BOOLEAN_AUTO);
        do_hwcorrupt    = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_MEMINFO, "hardware corrupted ECC", CONFIG_BOOLEAN_AUTO);
        do_committed    = config_get_boolean(CONFIG_SECTION_PLUGIN_PROC_MEMINFO, "committed memory", 1);
        do_writeback    = config_get_boolean(CONFIG_SECTION_PLUGIN_PROC_MEMINFO, "writeback memory", 1);
        do_kernel       = config_get_boolean(CONFIG_SECTION_PLUGIN_PROC_MEMINFO, "kernel memory", 1);
        do_slab         = config_get_boolean(CONFIG_SECTION_PLUGIN_PROC_MEMINFO, "slab memory", 1);
        do_hugepages    = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_MEMINFO, "hugepages", CONFIG_BOOLEAN_AUTO);
        do_transparent_hugepages = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_MEMINFO, "transparent hugepages", CONFIG_BOOLEAN_AUTO);

        arl_base = arl_create("meminfo", NULL, 60);
        arl_expect(arl_base, "MemTotal", &MemTotal);
        arl_expect(arl_base, "MemFree", &MemFree);
        arl_memavailable = arl_expect(arl_base, "MemAvailable", &MemAvailable);
        arl_expect(arl_base, "Buffers", &Buffers);
        arl_expect(arl_base, "Cached", &Cached);
        //arl_expect(arl_base, "SwapCached", &SwapCached);
        //arl_expect(arl_base, "Active", &Active);
        //arl_expect(arl_base, "Inactive", &Inactive);
        //arl_expect(arl_base, "ActiveAnon", &ActiveAnon);
        //arl_expect(arl_base, "InactiveAnon", &InactiveAnon);
        //arl_expect(arl_base, "ActiveFile", &ActiveFile);
        //arl_expect(arl_base, "InactiveFile", &InactiveFile);
        //arl_expect(arl_base, "Unevictable", &Unevictable);
        //arl_expect(arl_base, "Mlocked", &Mlocked);
        arl_expect(arl_base, "SwapTotal", &SwapTotal);
        arl_expect(arl_base, "SwapFree", &SwapFree);
        arl_expect(arl_base, "Dirty", &Dirty);
        arl_expect(arl_base, "Writeback", &Writeback);
        //arl_expect(arl_base, "AnonPages", &AnonPages);
        //arl_expect(arl_base, "Mapped", &Mapped);
        arl_expect(arl_base, "Shmem", &Shmem);
        arl_expect(arl_base, "Slab", &Slab);
        arl_expect(arl_base, "SReclaimable", &SReclaimable);
        arl_expect(arl_base, "SUnreclaim", &SUnreclaim);
        arl_expect(arl_base, "KernelStack", &KernelStack);
        arl_expect(arl_base, "PageTables", &PageTables);
        arl_expect(arl_base, "NFS_Unstable", &NFS_Unstable);
        arl_expect(arl_base, "Bounce", &Bounce);
        arl_expect(arl_base, "WritebackTmp", &WritebackTmp);
        //arl_expect(arl_base, "CommitLimit", &CommitLimit);
        arl_expect(arl_base, "Committed_AS", &Committed_AS);
        //arl_expect(arl_base, "VmallocTotal", &VmallocTotal);
        arl_expect(arl_base, "VmallocUsed", &VmallocUsed);
        //arl_expect(arl_base, "VmallocChunk", &VmallocChunk);
        arl_expect(arl_base, "Percpu", &Percpu);
        arl_hwcorrupted = arl_expect(arl_base, "HardwareCorrupted", &HardwareCorrupted);
        arl_expect(arl_base, "AnonHugePages", &AnonHugePages);
        arl_expect(arl_base, "ShmemHugePages", &ShmemHugePages);
        arl_expect(arl_base, "HugePages_Total", &HugePages_Total);
        arl_expect(arl_base, "HugePages_Free", &HugePages_Free);
        arl_expect(arl_base, "HugePages_Rsvd", &HugePages_Rsvd);
        arl_expect(arl_base, "HugePages_Surp", &HugePages_Surp);
        arl_expect(arl_base, "Hugepagesize", &Hugepagesize);
        //arl_expect(arl_base, "DirectMap4k", &DirectMap4k);
        //arl_expect(arl_base, "DirectMap2M", &DirectMap2M);
    }

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/meminfo");
        ff = procfile_open(config_get(CONFIG_SECTION_PLUGIN_PROC_MEMINFO, "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff))
            return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff))
        return 0; // we return 0, so that we will retry to open it next time

    size_t lines = procfile_lines(ff), l;

    arl_begin(arl_base);

    static int first_ff_read = 1;

    for(l = 0; l < lines ;l++) {
        size_t words = procfile_linewords(ff, l);
        if(unlikely(words < 2)) continue;

        if (first_ff_read && !strcmp(procfile_lineword(ff, l, 0), "Percpu"))
            do_percpu = 1;

        if(unlikely(arl_check(arl_base,
                procfile_lineword(ff, l, 0),
                procfile_lineword(ff, l, 1)))) break;
    }

    if (first_ff_read)
        first_ff_read = 0;

    // --------------------------------------------------------------------

    // http://calimeroteknik.free.fr/blag/?article20/really-used-memory-on-gnu-linux
    unsigned long long MemCached = Cached + SReclaimable - Shmem;
    unsigned long long MemUsed = MemTotal - MemFree - MemCached - Buffers;

    if(do_ram) {
        {
            static RRDSET *st_system_ram = NULL;
            static RRDDIM *rd_free = NULL, *rd_used = NULL, *rd_cached = NULL, *rd_buffers = NULL;

            if(unlikely(!st_system_ram)) {
                st_system_ram = rrdset_create_localhost(
                        "system"
                        , "ram"
                        , NULL
                        , "ram"
                        , NULL
                        , "System RAM"
                        , "MiB"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_MEMINFO_NAME
                        , NETDATA_CHART_PRIO_SYSTEM_RAM
                        , update_every
                        , RRDSET_TYPE_STACKED
                );

                rd_free    = rrddim_add(st_system_ram, "free",    NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
                rd_used    = rrddim_add(st_system_ram, "used",    NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
                rd_cached  = rrddim_add(st_system_ram, "cached",  NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
                rd_buffers = rrddim_add(st_system_ram, "buffers", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            }
            else rrdset_next(st_system_ram);

            rrddim_set_by_pointer(st_system_ram, rd_free,    MemFree);
            rrddim_set_by_pointer(st_system_ram, rd_used,    MemUsed);
            rrddim_set_by_pointer(st_system_ram, rd_cached,  MemCached);
            rrddim_set_by_pointer(st_system_ram, rd_buffers, Buffers);

            rrdset_done(st_system_ram);
        }

        if(arl_memavailable->flags & ARL_ENTRY_FLAG_FOUND) {
            static RRDSET *st_mem_available = NULL;
            static RRDDIM *rd_avail = NULL;

            if(unlikely(!st_mem_available)) {
                st_mem_available = rrdset_create_localhost(
                        "mem"
                        , "available"
                        , NULL
                        , "system"
                        , NULL
                        , "Available RAM for applications"
                        , "MiB"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_MEMINFO_NAME
                        , NETDATA_CHART_PRIO_MEM_SYSTEM_AVAILABLE
                        , update_every
                        , RRDSET_TYPE_AREA
                );

                rd_avail   = rrddim_add(st_mem_available, "MemAvailable", "avail", 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            }
            else rrdset_next(st_mem_available);

            rrddim_set_by_pointer(st_mem_available, rd_avail, MemAvailable);

            rrdset_done(st_mem_available);
        }
    }

    // --------------------------------------------------------------------

    unsigned long long SwapUsed = SwapTotal - SwapFree;

    if(do_swap == CONFIG_BOOLEAN_YES || (do_swap == CONFIG_BOOLEAN_AUTO &&
                                         (SwapTotal || SwapUsed || SwapFree ||
                                          netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        do_swap = CONFIG_BOOLEAN_YES;

        static RRDSET *st_system_swap = NULL;
        static RRDDIM *rd_free = NULL, *rd_used = NULL;

        if(unlikely(!st_system_swap)) {
            st_system_swap = rrdset_create_localhost(
                    "system"
                    , "swap"
                    , NULL
                    , "swap"
                    , NULL
                    , "System Swap"
                    , "MiB"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_MEMINFO_NAME
                    , NETDATA_CHART_PRIO_SYSTEM_SWAP
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

            rrdset_flag_set(st_system_swap, RRDSET_FLAG_DETAIL);

            rd_free = rrddim_add(st_system_swap, "free",    NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_used = rrddim_add(st_system_swap, "used",    NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st_system_swap);

        rrddim_set_by_pointer(st_system_swap, rd_used, SwapUsed);
        rrddim_set_by_pointer(st_system_swap, rd_free, SwapFree);

        rrdset_done(st_system_swap);
    }

    // --------------------------------------------------------------------

    if(arl_hwcorrupted->flags & ARL_ENTRY_FLAG_FOUND &&
       (do_hwcorrupt == CONFIG_BOOLEAN_YES || (do_hwcorrupt == CONFIG_BOOLEAN_AUTO &&
                                               (HardwareCorrupted > 0 ||
                                                netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES)))) {
        do_hwcorrupt = CONFIG_BOOLEAN_YES;

        static RRDSET *st_mem_hwcorrupt = NULL;
        static RRDDIM *rd_corrupted = NULL;

        if(unlikely(!st_mem_hwcorrupt)) {
            st_mem_hwcorrupt = rrdset_create_localhost(
                    "mem"
                    , "hwcorrupt"
                    , NULL
                    , "ecc"
                    , NULL
                    , "Corrupted Memory, detected by ECC"
                    , "MiB"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_MEMINFO_NAME
                    , NETDATA_CHART_PRIO_MEM_HW
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rrdset_flag_set(st_mem_hwcorrupt, RRDSET_FLAG_DETAIL);

            rd_corrupted = rrddim_add(st_mem_hwcorrupt, "HardwareCorrupted", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st_mem_hwcorrupt);

        rrddim_set_by_pointer(st_mem_hwcorrupt, rd_corrupted, HardwareCorrupted);

        rrdset_done(st_mem_hwcorrupt);
    }

    // --------------------------------------------------------------------

    if(do_committed) {
        static RRDSET *st_mem_committed = NULL;
        static RRDDIM *rd_committed = NULL;

        if(unlikely(!st_mem_committed)) {
            st_mem_committed = rrdset_create_localhost(
                    "mem"
                    , "committed"
                    , NULL
                    , "system"
                    , NULL
                    , "Committed (Allocated) Memory"
                    , "MiB"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_MEMINFO_NAME
                    , NETDATA_CHART_PRIO_MEM_SYSTEM_COMMITTED
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rrdset_flag_set(st_mem_committed, RRDSET_FLAG_DETAIL);

            rd_committed = rrddim_add(st_mem_committed, "Committed_AS", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st_mem_committed);

        rrddim_set_by_pointer(st_mem_committed, rd_committed, Committed_AS);

        rrdset_done(st_mem_committed);
    }

    // --------------------------------------------------------------------

    if(do_writeback) {
        static RRDSET *st_mem_writeback = NULL;
        static RRDDIM *rd_dirty = NULL, *rd_writeback = NULL, *rd_fusewriteback = NULL, *rd_nfs_writeback = NULL, *rd_bounce = NULL;

        if(unlikely(!st_mem_writeback)) {
            st_mem_writeback = rrdset_create_localhost(
                    "mem"
                    , "writeback"
                    , NULL
                    , "kernel"
                    , NULL
                    , "Writeback Memory"
                    , "MiB"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_MEMINFO_NAME
                    , NETDATA_CHART_PRIO_MEM_KERNEL
                    , update_every
                    , RRDSET_TYPE_LINE
            );
            rrdset_flag_set(st_mem_writeback, RRDSET_FLAG_DETAIL);

            rd_dirty         = rrddim_add(st_mem_writeback, "Dirty",         NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_writeback     = rrddim_add(st_mem_writeback, "Writeback",     NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_fusewriteback = rrddim_add(st_mem_writeback, "FuseWriteback", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_nfs_writeback = rrddim_add(st_mem_writeback, "NfsWriteback",  NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_bounce        = rrddim_add(st_mem_writeback, "Bounce",        NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st_mem_writeback);

        rrddim_set_by_pointer(st_mem_writeback, rd_dirty,         Dirty);
        rrddim_set_by_pointer(st_mem_writeback, rd_writeback,     Writeback);
        rrddim_set_by_pointer(st_mem_writeback, rd_fusewriteback, WritebackTmp);
        rrddim_set_by_pointer(st_mem_writeback, rd_nfs_writeback, NFS_Unstable);
        rrddim_set_by_pointer(st_mem_writeback, rd_bounce,        Bounce);

        rrdset_done(st_mem_writeback);
    }

    // --------------------------------------------------------------------

    if(do_kernel) {
        static RRDSET *st_mem_kernel = NULL;
        static RRDDIM *rd_slab = NULL, *rd_kernelstack = NULL, *rd_pagetables = NULL, *rd_vmallocused = NULL,
                      *rd_percpu = NULL;

        if(unlikely(!st_mem_kernel)) {
            st_mem_kernel = rrdset_create_localhost(
                    "mem"
                    , "kernel"
                    , NULL
                    , "kernel"
                    , NULL
                    , "Memory Used by Kernel"
                    , "MiB"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_MEMINFO_NAME
                    , NETDATA_CHART_PRIO_MEM_KERNEL + 1
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

            rrdset_flag_set(st_mem_kernel, RRDSET_FLAG_DETAIL);

            rd_slab        = rrddim_add(st_mem_kernel, "Slab",        NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_kernelstack = rrddim_add(st_mem_kernel, "KernelStack", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_pagetables  = rrddim_add(st_mem_kernel, "PageTables",  NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_vmallocused = rrddim_add(st_mem_kernel, "VmallocUsed", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            if (do_percpu)
                rd_percpu  = rrddim_add(st_mem_kernel, "Percpu",      NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st_mem_kernel);

        rrddim_set_by_pointer(st_mem_kernel, rd_slab,        Slab);
        rrddim_set_by_pointer(st_mem_kernel, rd_kernelstack, KernelStack);
        rrddim_set_by_pointer(st_mem_kernel, rd_pagetables,  PageTables);
        rrddim_set_by_pointer(st_mem_kernel, rd_vmallocused, VmallocUsed);
        if (do_percpu)
            rrddim_set_by_pointer(st_mem_kernel, rd_percpu, Percpu);

        rrdset_done(st_mem_kernel);
    }

    // --------------------------------------------------------------------

    if(do_slab) {
        static RRDSET *st_mem_slab = NULL;
        static RRDDIM *rd_reclaimable = NULL, *rd_unreclaimable = NULL;

        if(unlikely(!st_mem_slab)) {
            st_mem_slab = rrdset_create_localhost(
                    "mem"
                    , "slab"
                    , NULL
                    , "slab"
                    , NULL
                    , "Reclaimable Kernel Memory"
                    , "MiB"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_MEMINFO_NAME
                    , NETDATA_CHART_PRIO_MEM_SLAB
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

            rrdset_flag_set(st_mem_slab, RRDSET_FLAG_DETAIL);

            rd_reclaimable   = rrddim_add(st_mem_slab, "reclaimable",   NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_unreclaimable = rrddim_add(st_mem_slab, "unreclaimable", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st_mem_slab);

        rrddim_set_by_pointer(st_mem_slab, rd_reclaimable, SReclaimable);
        rrddim_set_by_pointer(st_mem_slab, rd_unreclaimable, SUnreclaim);

        rrdset_done(st_mem_slab);
    }

    // --------------------------------------------------------------------

    if(do_hugepages == CONFIG_BOOLEAN_YES || (do_hugepages == CONFIG_BOOLEAN_AUTO &&
                                              ((Hugepagesize && HugePages_Total) ||
                                               netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        do_hugepages = CONFIG_BOOLEAN_YES;

        static RRDSET *st_mem_hugepages = NULL;
        static RRDDIM *rd_used = NULL, *rd_free = NULL, *rd_rsvd = NULL, *rd_surp = NULL;

        if(unlikely(!st_mem_hugepages)) {
            st_mem_hugepages = rrdset_create_localhost(
                    "mem"
                    , "hugepages"
                    , NULL
                    , "hugepages"
                    , NULL
                    , "Dedicated HugePages Memory"
                    , "MiB"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_MEMINFO_NAME
                    , NETDATA_CHART_PRIO_MEM_HUGEPAGES + 1
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

            rrdset_flag_set(st_mem_hugepages, RRDSET_FLAG_DETAIL);

            rd_free = rrddim_add(st_mem_hugepages, "free",     NULL, Hugepagesize, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_used = rrddim_add(st_mem_hugepages, "used",     NULL, Hugepagesize, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_surp = rrddim_add(st_mem_hugepages, "surplus",  NULL, Hugepagesize, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_rsvd = rrddim_add(st_mem_hugepages, "reserved", NULL, Hugepagesize, 1024, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st_mem_hugepages);

        rrddim_set_by_pointer(st_mem_hugepages, rd_used, HugePages_Total - HugePages_Free - HugePages_Rsvd);
        rrddim_set_by_pointer(st_mem_hugepages, rd_free, HugePages_Free);
        rrddim_set_by_pointer(st_mem_hugepages, rd_rsvd, HugePages_Rsvd);
        rrddim_set_by_pointer(st_mem_hugepages, rd_surp, HugePages_Surp);

        rrdset_done(st_mem_hugepages);
    }

    // --------------------------------------------------------------------

    if(do_transparent_hugepages == CONFIG_BOOLEAN_YES || (do_transparent_hugepages == CONFIG_BOOLEAN_AUTO &&
                                                          (AnonHugePages ||
                                                           ShmemHugePages ||
                                                           netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        do_transparent_hugepages = CONFIG_BOOLEAN_YES;

        static RRDSET *st_mem_transparent_hugepages = NULL;
        static RRDDIM *rd_anonymous = NULL, *rd_shared = NULL;

        if(unlikely(!st_mem_transparent_hugepages)) {
            st_mem_transparent_hugepages = rrdset_create_localhost(
                    "mem"
                    , "transparent_hugepages"
                    , NULL
                    , "hugepages"
                    , NULL
                    , "Transparent HugePages Memory"
                    , "MiB"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_MEMINFO_NAME
                    , NETDATA_CHART_PRIO_MEM_HUGEPAGES
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

            rrdset_flag_set(st_mem_transparent_hugepages, RRDSET_FLAG_DETAIL);

            rd_anonymous = rrddim_add(st_mem_transparent_hugepages, "anonymous",  NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_shared    = rrddim_add(st_mem_transparent_hugepages, "shmem",      NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st_mem_transparent_hugepages);

        rrddim_set_by_pointer(st_mem_transparent_hugepages, rd_anonymous, AnonHugePages);
        rrddim_set_by_pointer(st_mem_transparent_hugepages, rd_shared, ShmemHugePages);

        rrdset_done(st_mem_transparent_hugepages);
    }

    return 0;
}

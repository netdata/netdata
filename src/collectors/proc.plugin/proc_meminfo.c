// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

#define PLUGIN_PROC_MODULE_MEMINFO_NAME "/proc/meminfo"
#define CONFIG_SECTION_PLUGIN_PROC_MEMINFO "plugin:" PLUGIN_PROC_CONFIG_NAME ":" PLUGIN_PROC_MODULE_MEMINFO_NAME

#define _COMMON_PLUGIN_NAME PLUGIN_PROC_NAME
#define _COMMON_PLUGIN_MODULE_NAME PLUGIN_PROC_MODULE_MEMINFO_NAME
#include "../common-contexts/common-contexts.h"

int do_proc_meminfo(int update_every, usec_t dt) {
    (void)dt;

    static bool swap_configured = false;

    static procfile *ff = NULL;
    static int do_ram = -1
            , do_swap = -1
            , do_hwcorrupt = -1
            , do_committed = -1
            , do_writeback = -1
            , do_kernel = -1
            , do_slab = -1
            , do_hugepages = -1
            , do_transparent_hugepages = -1
            , do_reclaiming = -1
            , do_high_low = -1
            , do_cma = -1
            , do_directmap = -1;

    static ARL_BASE *arl_base = NULL;
    static ARL_ENTRY *arl_hwcorrupted = NULL, *arl_memavailable = NULL, *arl_hugepages_total = NULL,
        *arl_zswapped = NULL, *arl_high_low = NULL,
        *arl_directmap4k = NULL, *arl_directmap2m = NULL, *arl_directmap4m = NULL, *arl_directmap1g = NULL;

    static unsigned long long
              MemTotal = 0
            , MemFree = 0
            , MemAvailable = 0
            , Buffers = 0
            , Cached = 0
            , SwapCached = 0
            , Active = 0
            , Inactive = 0
            , ActiveAnon = 0
            , InactiveAnon = 0
            , ActiveFile = 0
            , InactiveFile = 0
            , Unevictable = 0
            , Mlocked = 0
            , HighTotal = 0
            , HighFree  = 0
            , LowTotal = 0
            , LowFree = 0
            , MmapCopy = 0
            , SwapTotal = 0
            , SwapFree = 0
            , Zswap = 0
            , Zswapped = 0
            , Dirty = 0
            , Writeback = 0
            , AnonPages = 0
            , Mapped = 0
            , Shmem = 0
            , KReclaimable = 0
            , Slab = 0
            , SReclaimable = 0
            , SUnreclaim = 0
            , KernelStack = 0
            , ShadowCallStack = 0
            , PageTables = 0
            , SecPageTables = 0
            , NFS_Unstable = 0
            , Bounce = 0
            , WritebackTmp = 0
            , CommitLimit = 0
            , Committed_AS = 0
            , VmallocTotal = 0
            , VmallocUsed = 0
            , VmallocChunk = 0
            , Percpu = 0
            //, EarlyMemtestBad = 0
            , HardwareCorrupted = 0
            , AnonHugePages = 0
            , ShmemHugePages = 0
            , ShmemPmdMapped = 0
            , FileHugePages = 0
            , FilePmdMapped = 0
            , CmaTotal = 0
            , CmaFree = 0
            //, Unaccepted = 0
            , HugePages_Total = 0
            , HugePages_Free = 0
            , HugePages_Rsvd = 0
            , HugePages_Surp = 0
            , Hugepagesize = 0
            //, Hugetlb = 0
            , DirectMap4k = 0
            , DirectMap2M = 0
            , DirectMap4M = 0
            , DirectMap1G = 0
            ;

    if(unlikely(!arl_base)) {
        do_ram          = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_MEMINFO, "system ram", CONFIG_BOOLEAN_YES);
        do_swap         = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_MEMINFO, "system swap", CONFIG_BOOLEAN_AUTO);
        do_hwcorrupt    = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_MEMINFO, "hardware corrupted ECC", CONFIG_BOOLEAN_AUTO);
        do_committed    = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_MEMINFO, "committed memory", CONFIG_BOOLEAN_YES);
        do_writeback    = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_MEMINFO, "writeback memory", CONFIG_BOOLEAN_YES);
        do_kernel       = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_MEMINFO, "kernel memory", CONFIG_BOOLEAN_YES);
        do_slab         = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_MEMINFO, "slab memory", CONFIG_BOOLEAN_YES);
        do_hugepages    = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_MEMINFO, "hugepages", CONFIG_BOOLEAN_AUTO);
        do_transparent_hugepages = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_MEMINFO, "transparent hugepages", CONFIG_BOOLEAN_AUTO);
        do_reclaiming   = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_MEMINFO, "memory reclaiming", CONFIG_BOOLEAN_AUTO);
        do_high_low     = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_MEMINFO, "high low memory", CONFIG_BOOLEAN_AUTO);
        do_cma          = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_MEMINFO, "cma memory", CONFIG_BOOLEAN_AUTO);
        do_directmap    = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_MEMINFO, "direct maps", CONFIG_BOOLEAN_AUTO);

        // https://github.com/torvalds/linux/blob/master/fs/proc/meminfo.c

        arl_base = arl_create("meminfo", NULL, 60);
        arl_expect(arl_base, "MemTotal", &MemTotal);
        arl_expect(arl_base, "MemFree", &MemFree);
        arl_memavailable = arl_expect(arl_base, "MemAvailable", &MemAvailable);
        arl_expect(arl_base, "Buffers", &Buffers);
        arl_expect(arl_base, "Cached", &Cached);
        arl_expect(arl_base, "SwapCached", &SwapCached);
        arl_expect(arl_base, "Active", &Active);
        arl_expect(arl_base, "Inactive", &Inactive);
        arl_expect(arl_base, "Active(anon)", &ActiveAnon);
        arl_expect(arl_base, "Inactive(anon)", &InactiveAnon);
        arl_expect(arl_base, "Active(file)", &ActiveFile);
        arl_expect(arl_base, "Inactive(file)", &InactiveFile);
        arl_expect(arl_base, "Unevictable", &Unevictable);
        arl_expect(arl_base, "Mlocked", &Mlocked);

        // CONFIG_HIGHMEM
        arl_high_low = arl_expect(arl_base, "HighTotal", &HighTotal);
        arl_expect(arl_base, "HighFree", &HighFree);
        arl_expect(arl_base, "LowTotal", &LowTotal);
        arl_expect(arl_base, "LowFree", &LowFree);

        // CONFIG_MMU
        arl_expect(arl_base, "MmapCopy", &MmapCopy);

        arl_expect(arl_base, "SwapTotal", &SwapTotal);
        arl_expect(arl_base, "SwapFree", &SwapFree);

        // CONFIG_ZSWAP
        arl_zswapped = arl_expect(arl_base, "Zswap", &Zswap);
        arl_expect(arl_base, "Zswapped", &Zswapped);

        arl_expect(arl_base, "Dirty", &Dirty);
        arl_expect(arl_base, "Writeback", &Writeback);
        arl_expect(arl_base, "AnonPages", &AnonPages);
        arl_expect(arl_base, "Mapped", &Mapped);
        arl_expect(arl_base, "Shmem", &Shmem);
        arl_expect(arl_base, "KReclaimable", &KReclaimable);
        arl_expect(arl_base, "Slab", &Slab);
        arl_expect(arl_base, "SReclaimable", &SReclaimable);
        arl_expect(arl_base, "SUnreclaim", &SUnreclaim);
        arl_expect(arl_base, "KernelStack", &KernelStack);

        // CONFIG_SHADOW_CALL_STACK
        arl_expect(arl_base, "ShadowCallStack", &ShadowCallStack);

        arl_expect(arl_base, "PageTables", &PageTables);
        arl_expect(arl_base, "SecPageTables", &SecPageTables);
        arl_expect(arl_base, "NFS_Unstable", &NFS_Unstable);
        arl_expect(arl_base, "Bounce", &Bounce);
        arl_expect(arl_base, "WritebackTmp", &WritebackTmp);
        arl_expect(arl_base, "CommitLimit", &CommitLimit);
        arl_expect(arl_base, "Committed_AS", &Committed_AS);
        arl_expect(arl_base, "VmallocTotal", &VmallocTotal);
        arl_expect(arl_base, "VmallocUsed", &VmallocUsed);
        arl_expect(arl_base, "VmallocChunk", &VmallocChunk);
        arl_expect(arl_base, "Percpu", &Percpu);

        // CONFIG_MEMTEST
        //arl_expect(arl_base, "EarlyMemtestBad", &EarlyMemtestBad);

        // CONFIG_MEMORY_FAILURE
        arl_hwcorrupted = arl_expect(arl_base, "HardwareCorrupted", &HardwareCorrupted);

        // CONFIG_TRANSPARENT_HUGEPAGE
        arl_expect(arl_base, "AnonHugePages", &AnonHugePages);
        arl_expect(arl_base, "ShmemHugePages", &ShmemHugePages);
        arl_expect(arl_base, "ShmemPmdMapped", &ShmemPmdMapped);
        arl_expect(arl_base, "FileHugePages", &FileHugePages);
        arl_expect(arl_base, "FilePmdMapped", &FilePmdMapped);

        // CONFIG_CMA
        arl_expect(arl_base, "CmaTotal", &CmaTotal);
        arl_expect(arl_base, "CmaFree", &CmaFree);

        // CONFIG_UNACCEPTED_MEMORY
        //arl_expect(arl_base, "Unaccepted", &Unaccepted);

        // these appear only when hugepages are supported
        arl_hugepages_total = arl_expect(arl_base, "HugePages_Total", &HugePages_Total);
        arl_expect(arl_base, "HugePages_Free", &HugePages_Free);
        arl_expect(arl_base, "HugePages_Rsvd", &HugePages_Rsvd);
        arl_expect(arl_base, "HugePages_Surp", &HugePages_Surp);
        arl_expect(arl_base, "Hugepagesize", &Hugepagesize);
        //arl_expect(arl_base, "Hugetlb", &Hugetlb);

        arl_directmap4k = arl_expect(arl_base, "DirectMap4k", &DirectMap4k);
        arl_directmap2m = arl_expect(arl_base, "DirectMap2M", &DirectMap2M);
        arl_directmap4m = arl_expect(arl_base, "DirectMap4M", &DirectMap4M);
        arl_directmap1g = arl_expect(arl_base, "DirectMap1G", &DirectMap1G);
    }

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/meminfo");
        ff = procfile_open(inicfg_get(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_MEMINFO, "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff))
            return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff))
        return 0; // we return 0, so that we will retry to open it next time

    size_t lines = procfile_lines(ff), l;

    arl_begin(arl_base);
    for(l = 0; l < lines ;l++) {
        size_t words = procfile_linewords(ff, l);
        if(unlikely(words < 2)) continue;

        if(unlikely(arl_check(arl_base,
                procfile_lineword(ff, l, 0),
                procfile_lineword(ff, l, 1)))) break;
    }

    // http://calimeroteknik.free.fr/blag/?article20/really-used-memory-on-gnu-linux
    // KReclaimable includes SReclaimable, it was added in kernel v4.20
    unsigned long long reclaimable = inside_lxc_container ? 0 : (KReclaimable > 0 ? KReclaimable : SReclaimable);
    unsigned long long MemCached = Cached + reclaimable - Shmem;
    unsigned long long MemUsed = MemTotal - MemFree - MemCached - Buffers;
    // The Linux kernel doesn't report ZFS ARC usage as cache memory (the ARC is included in the total used system memory)
    if (!inside_lxc_container) {
        MemCached += (zfs_arcstats_shrinkable_cache_size_bytes / 1024);
        MemUsed -= (zfs_arcstats_shrinkable_cache_size_bytes / 1024);
        MemAvailable += (zfs_arcstats_shrinkable_cache_size_bytes / 1024);
    }

    if(do_ram) {
        common_system_ram(MemFree * 1024, MemUsed * 1024, MemCached * 1024, Buffers * 1024, update_every);

        if(arl_memavailable->flags & ARL_ENTRY_FLAG_FOUND)
            common_mem_available(MemAvailable * 1024, update_every);
    }

    unsigned long long SwapUsed = SwapTotal - SwapFree;

    if (SwapTotal && (do_swap == CONFIG_BOOLEAN_YES || do_swap == CONFIG_BOOLEAN_AUTO)) {
        do_swap = CONFIG_BOOLEAN_YES;
        common_mem_swap(SwapFree * 1024, SwapUsed * 1024, update_every);
        swap_configured = true;

        {
            static RRDSET *st_mem_swap_cached = NULL;
            static RRDDIM *rd_cached = NULL;

            if (unlikely(!st_mem_swap_cached)) {
                st_mem_swap_cached = rrdset_create_localhost(
                        "mem"
                        , "swap_cached"
                        , NULL
                        , "swap"
                        , NULL
                        , "Swap Memory Cached in RAM"
                        , "MiB"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_MEMINFO_NAME
                        , NETDATA_CHART_PRIO_MEM_SWAP + 1
                        , update_every
                        , RRDSET_TYPE_AREA
                );

                rd_cached = rrddim_add(st_mem_swap_cached, "cached", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set_by_pointer(st_mem_swap_cached, rd_cached, SwapCached);
            rrdset_done(st_mem_swap_cached);
        }

        if (is_mem_zswap_enabled && (arl_zswapped->flags & ARL_ENTRY_FLAG_FOUND)) {
            static RRDSET *st_mem_zswap = NULL;
            static RRDDIM *rd_zswap = NULL, *rd_zswapped = NULL;

            if (unlikely(!st_mem_zswap)) {
                st_mem_zswap = rrdset_create_localhost(
                        "mem"
                        , "zswap"
                        , NULL
                        , "zswap"
                        , NULL
                        , "Zswap Usage"
                        , "MiB"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_MEMINFO_NAME
                        , NETDATA_CHART_PRIO_MEM_ZSWAP
                        , update_every
                        , RRDSET_TYPE_STACKED
                );

                rd_zswap = rrddim_add(st_mem_zswap, "zswap", "in-ram", 1, 1024, RRD_ALGORITHM_ABSOLUTE);
                rd_zswapped = rrddim_add(st_mem_zswap, "zswapped", "on-disk", 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set_by_pointer(st_mem_zswap, rd_zswap, Zswap);
            rrddim_set_by_pointer(st_mem_zswap, rd_zswapped, Zswapped);
            rrdset_done(st_mem_zswap);
        }
    } else {
        if (swap_configured) {
            common_mem_swap(SwapFree * 1024, SwapUsed * 1024, update_every);
            swap_configured = false;
        }
    }


    if (arl_hwcorrupted->flags & ARL_ENTRY_FLAG_FOUND &&
        (do_hwcorrupt == CONFIG_BOOLEAN_YES || do_hwcorrupt == CONFIG_BOOLEAN_AUTO)) {
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

            rd_corrupted = rrddim_add(st_mem_hwcorrupt, "HardwareCorrupted", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(st_mem_hwcorrupt, rd_corrupted, HardwareCorrupted);
        rrdset_done(st_mem_hwcorrupt);
    }

    if(do_committed) {
        static RRDSET *st_mem_committed = NULL;
        static RRDDIM *rd_committed = NULL;

        if(unlikely(!st_mem_committed)) {
            st_mem_committed = rrdset_create_localhost(
                    "mem"
                    , "committed"
                    , NULL
                    , "overview"
                    , NULL
                    , "Committed (Allocated) Memory"
                    , "MiB"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_MEMINFO_NAME
                    , NETDATA_CHART_PRIO_MEM_SYSTEM_COMMITTED
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rd_committed = rrddim_add(st_mem_committed, "Committed_AS", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(st_mem_committed, rd_committed, Committed_AS);
        rrdset_done(st_mem_committed);
    }

    if(do_writeback) {
        static RRDSET *st_mem_writeback = NULL;
        static RRDDIM *rd_dirty = NULL, *rd_writeback = NULL, *rd_fusewriteback = NULL, *rd_nfs_writeback = NULL, *rd_bounce = NULL;

        if(unlikely(!st_mem_writeback)) {
            st_mem_writeback = rrdset_create_localhost(
                    "mem"
                    , "writeback"
                    , NULL
                    , "writeback"
                    , NULL
                    , "Writeback Memory"
                    , "MiB"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_MEMINFO_NAME
                    , NETDATA_CHART_PRIO_MEM_KERNEL
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_dirty         = rrddim_add(st_mem_writeback, "Dirty",         NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_writeback     = rrddim_add(st_mem_writeback, "Writeback",     NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_fusewriteback = rrddim_add(st_mem_writeback, "FuseWriteback", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_nfs_writeback = rrddim_add(st_mem_writeback, "NfsWriteback",  NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_bounce        = rrddim_add(st_mem_writeback, "Bounce",        NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        }

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
                      *rd_percpu = NULL, *rd_kreclaimable = NULL;

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

            rd_slab        = rrddim_add(st_mem_kernel, "Slab",        NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_kernelstack = rrddim_add(st_mem_kernel, "KernelStack", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_pagetables  = rrddim_add(st_mem_kernel, "PageTables",  NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_vmallocused = rrddim_add(st_mem_kernel, "VmallocUsed", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_percpu      = rrddim_add(st_mem_kernel, "Percpu",      NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_kreclaimable = rrddim_add(st_mem_kernel, "KReclaimable", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(st_mem_kernel, rd_slab,           Slab);
        rrddim_set_by_pointer(st_mem_kernel, rd_kernelstack,    KernelStack);
        rrddim_set_by_pointer(st_mem_kernel, rd_pagetables,     PageTables);
        rrddim_set_by_pointer(st_mem_kernel, rd_vmallocused,    VmallocUsed);
        rrddim_set_by_pointer(st_mem_kernel, rd_percpu,         Percpu);
        rrddim_set_by_pointer(st_mem_kernel, rd_kreclaimable,   KReclaimable);

        rrdset_done(st_mem_kernel);
    }

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

            rd_reclaimable   = rrddim_add(st_mem_slab, "reclaimable",   NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_unreclaimable = rrddim_add(st_mem_slab, "unreclaimable", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(st_mem_slab, rd_reclaimable, SReclaimable);
        rrddim_set_by_pointer(st_mem_slab, rd_unreclaimable, SUnreclaim);
        rrdset_done(st_mem_slab);
    }

    if (arl_hugepages_total->flags & ARL_ENTRY_FLAG_FOUND && HugePages_Total &&
        (do_hugepages == CONFIG_BOOLEAN_YES || do_hugepages == CONFIG_BOOLEAN_AUTO)) {
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
                    , NETDATA_CHART_PRIO_MEM_HUGEPAGES
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

            rd_free = rrddim_add(st_mem_hugepages, "free",     NULL, Hugepagesize, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_used = rrddim_add(st_mem_hugepages, "used",     NULL, Hugepagesize, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_surp = rrddim_add(st_mem_hugepages, "surplus",  NULL, Hugepagesize, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_rsvd = rrddim_add(st_mem_hugepages, "reserved", NULL, Hugepagesize, 1024, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(st_mem_hugepages, rd_used, HugePages_Total - HugePages_Free - HugePages_Rsvd);
        rrddim_set_by_pointer(st_mem_hugepages, rd_free, HugePages_Free);
        rrddim_set_by_pointer(st_mem_hugepages, rd_rsvd, HugePages_Rsvd);
        rrddim_set_by_pointer(st_mem_hugepages, rd_surp, HugePages_Surp);
        rrdset_done(st_mem_hugepages);
    }

    if (do_transparent_hugepages == CONFIG_BOOLEAN_YES || do_transparent_hugepages == CONFIG_BOOLEAN_AUTO) {
        do_transparent_hugepages = CONFIG_BOOLEAN_YES;

        static RRDSET *st_mem_transparent_hugepages = NULL;
        static RRDDIM *rd_anonymous = NULL, *rd_shared = NULL;

        if(unlikely(!st_mem_transparent_hugepages)) {
            st_mem_transparent_hugepages = rrdset_create_localhost(
                    "mem"
                    , "thp"
                    , NULL
                    , "hugepages"
                    , NULL
                    , "Transparent HugePages Memory"
                    , "MiB"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_MEMINFO_NAME
                    , NETDATA_CHART_PRIO_MEM_HUGEPAGES + 1
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

            rd_anonymous = rrddim_add(st_mem_transparent_hugepages, "anonymous",  NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_shared    = rrddim_add(st_mem_transparent_hugepages, "shmem",      NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(st_mem_transparent_hugepages, rd_anonymous, AnonHugePages);
        rrddim_set_by_pointer(st_mem_transparent_hugepages, rd_shared, ShmemHugePages);
        rrdset_done(st_mem_transparent_hugepages);

        {
            static RRDSET *st_mem_thp_details = NULL;
            static RRDDIM *rd_shmem_pmd_mapped = NULL, *rd_file_huge_pages = NULL, *rd_file_pmd_mapped = NULL;

            if(unlikely(!st_mem_thp_details)) {
                st_mem_thp_details = rrdset_create_localhost(
                        "mem"
                        , "thp_details"
                        , NULL
                        , "hugepages"
                        , NULL
                        , "Details of Transparent HugePages Usage"
                        , "MiB"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_MEMINFO_NAME
                        , NETDATA_CHART_PRIO_MEM_HUGEPAGES_DETAILS
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rd_shmem_pmd_mapped = rrddim_add(st_mem_thp_details, "shmem_pmd", "ShmemPmdMapped", 1, 1024, RRD_ALGORITHM_ABSOLUTE);
                rd_file_huge_pages = rrddim_add(st_mem_thp_details, "file", "FileHugePages", 1, 1024, RRD_ALGORITHM_ABSOLUTE);
                rd_file_pmd_mapped = rrddim_add(st_mem_thp_details, "file_pmd", "FilePmdMapped", 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set_by_pointer(st_mem_thp_details, rd_shmem_pmd_mapped, ShmemPmdMapped);
            rrddim_set_by_pointer(st_mem_thp_details, rd_file_huge_pages, FileHugePages);
            rrddim_set_by_pointer(st_mem_thp_details, rd_file_pmd_mapped, FilePmdMapped);
            rrdset_done(st_mem_thp_details);
        }
    }

    if(do_reclaiming != CONFIG_BOOLEAN_NO) {
        static RRDSET *st_mem_reclaiming = NULL;
        static RRDDIM *rd_active = NULL, *rd_inactive = NULL,
                *rd_active_anon = NULL, *rd_inactive_anon = NULL,
                *rd_active_file = NULL, *rd_inactive_file = NULL,
                *rd_unevictable = NULL, *rd_mlocked = NULL;

        if(unlikely(!st_mem_reclaiming)) {
            st_mem_reclaiming = rrdset_create_localhost(
                    "mem"
                    , "reclaiming"
                    , NULL
                    , "reclaiming"
                    , NULL
                    , "Memory Reclaiming"
                    , "MiB"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_MEMINFO_NAME
                    , NETDATA_CHART_PRIO_MEM_RECLAIMING
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_active        = rrddim_add(st_mem_reclaiming, "active",        "Active", 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_inactive      = rrddim_add(st_mem_reclaiming, "inactive",      "Inactive", 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_active_anon   = rrddim_add(st_mem_reclaiming, "active_anon",   "Active(anon)", 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_inactive_anon = rrddim_add(st_mem_reclaiming, "inactive_anon", "Inactive(anon)", 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_active_file   = rrddim_add(st_mem_reclaiming, "active_file",   "Active(file)", 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_inactive_file = rrddim_add(st_mem_reclaiming, "inactive_file", "Inactive(file)", 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_unevictable   = rrddim_add(st_mem_reclaiming, "unevictable",   "Unevictable", 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_mlocked       = rrddim_add(st_mem_reclaiming, "mlocked",       "Mlocked", 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(st_mem_reclaiming, rd_active,        Active);
        rrddim_set_by_pointer(st_mem_reclaiming, rd_inactive,      Inactive);
        rrddim_set_by_pointer(st_mem_reclaiming, rd_active_anon,   ActiveAnon);
        rrddim_set_by_pointer(st_mem_reclaiming, rd_inactive_anon, InactiveAnon);
        rrddim_set_by_pointer(st_mem_reclaiming, rd_active_file,   ActiveFile);
        rrddim_set_by_pointer(st_mem_reclaiming, rd_inactive_file, InactiveFile);
        rrddim_set_by_pointer(st_mem_reclaiming, rd_unevictable,   Unevictable);
        rrddim_set_by_pointer(st_mem_reclaiming, rd_mlocked,       Mlocked);

        rrdset_done(st_mem_reclaiming);
    }

    if(do_high_low != CONFIG_BOOLEAN_NO && (arl_high_low->flags & ARL_ENTRY_FLAG_FOUND)) {
        static RRDSET *st_mem_high_low = NULL;
        static RRDDIM *rd_high_used = NULL, *rd_low_used = NULL;
        static RRDDIM *rd_high_free = NULL, *rd_low_free = NULL;

        if(unlikely(!st_mem_high_low)) {
            st_mem_high_low = rrdset_create_localhost(
                    "mem"
                    , "high_low"
                    , NULL
                    , "high_low"
                    , NULL
                    , "High and Low Used and Free Memory Areas"
                    , "MiB"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_MEMINFO_NAME
                    , NETDATA_CHART_PRIO_MEM_HIGH_LOW
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

            rd_high_used = rrddim_add(st_mem_high_low, "high_used",  NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_low_used  = rrddim_add(st_mem_high_low, "low_used",   NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_high_free = rrddim_add(st_mem_high_low, "high_free",  NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_low_free  = rrddim_add(st_mem_high_low, "low_free",   NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(st_mem_high_low, rd_high_used, HighTotal - HighFree);
        rrddim_set_by_pointer(st_mem_high_low, rd_low_used, LowTotal - LowFree);
        rrddim_set_by_pointer(st_mem_high_low, rd_high_free, HighFree);
        rrddim_set_by_pointer(st_mem_high_low, rd_low_free, LowFree);
        rrdset_done(st_mem_high_low);
    }

    if (CmaTotal && do_cma != CONFIG_BOOLEAN_NO) {
        do_cma = CONFIG_BOOLEAN_YES;

        static RRDSET *st_mem_cma = NULL;
        static RRDDIM *rd_used = NULL, *rd_free = NULL;

        if(unlikely(!st_mem_cma)) {
            st_mem_cma = rrdset_create_localhost(
                    "mem"
                    , "cma"
                    , NULL
                    , "cma"
                    , NULL
                    , "Contiguous Memory Allocator (CMA) Memory"
                    , "MiB"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_MEMINFO_NAME
                    , NETDATA_CHART_PRIO_MEM_CMA
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

            rd_used = rrddim_add(st_mem_cma, "used", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_free = rrddim_add(st_mem_cma, "free", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(st_mem_cma, rd_used, CmaTotal - CmaFree);
        rrddim_set_by_pointer(st_mem_cma, rd_free, CmaFree);
        rrdset_done(st_mem_cma);
    }

    if(do_directmap != CONFIG_BOOLEAN_NO &&
            ((arl_directmap4k->flags & ARL_ENTRY_FLAG_FOUND) ||
             (arl_directmap2m->flags & ARL_ENTRY_FLAG_FOUND) ||
             (arl_directmap4m->flags & ARL_ENTRY_FLAG_FOUND) ||
             (arl_directmap1g->flags & ARL_ENTRY_FLAG_FOUND)))
    {
        static RRDSET *st_mem_directmap = NULL;
        static RRDDIM *rd_4k = NULL, *rd_2m = NULL, *rd_1g = NULL, *rd_4m = NULL;

        if(unlikely(!st_mem_directmap)) {
            st_mem_directmap = rrdset_create_localhost(
                    "mem"
                    , "directmaps"
                    , NULL
                    , "overview"
                    , NULL
                    , "Direct Memory Mappings"
                    , "MiB"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_MEMINFO_NAME
                    , NETDATA_CHART_PRIO_MEM_DIRECTMAP
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

            if(arl_directmap4k->flags & ARL_ENTRY_FLAG_FOUND)
                rd_4k = rrddim_add(st_mem_directmap, "4k", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);

            if(arl_directmap2m->flags & ARL_ENTRY_FLAG_FOUND)
                rd_2m = rrddim_add(st_mem_directmap, "2m", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);

            if(arl_directmap4m->flags & ARL_ENTRY_FLAG_FOUND)
                rd_4m = rrddim_add(st_mem_directmap, "4m", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);

            if(arl_directmap1g->flags & ARL_ENTRY_FLAG_FOUND)
                rd_1g = rrddim_add(st_mem_directmap, "1g", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        }

        if(rd_4k)
            rrddim_set_by_pointer(st_mem_directmap, rd_4k, DirectMap4k);

        if(rd_2m)
            rrddim_set_by_pointer(st_mem_directmap, rd_2m, DirectMap2M);

        if(rd_4m)
            rrddim_set_by_pointer(st_mem_directmap, rd_4m, DirectMap4M);

        if(rd_1g)
            rrddim_set_by_pointer(st_mem_directmap, rd_1g, DirectMap1G);

        rrdset_done(st_mem_directmap);
    }

    return 0;
}

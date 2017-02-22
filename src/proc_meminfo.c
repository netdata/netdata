#include "common.h"

int do_proc_meminfo(int update_every, usec_t dt) {
    (void)dt;

    static procfile *ff = NULL;
    static int do_ram = -1, do_swap = -1, do_hwcorrupt = -1, do_committed = -1, do_writeback = -1, do_kernel = -1, do_slab = -1;

    static ARL_BASE *arl_base = NULL;
    static ARL_ENTRY *arl_hwcorrupted = NULL;

    static unsigned long long
            MemTotal = 0,
            MemFree = 0,
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
            //Shmem = 0,
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
            //AnonHugePages = 0,
            //HugePages_Total = 0,
            //HugePages_Free = 0,
            //HugePages_Rsvd = 0,
            //HugePages_Surp = 0,
            //Hugepagesize = 0,
            //DirectMap4k = 0,
            //DirectMap2M = 0,
            HardwareCorrupted = 0;

    if(unlikely(!arl_base)) {
        do_ram          = config_get_boolean("plugin:proc:/proc/meminfo", "system ram", 1);
        do_swap         = config_get_boolean_ondemand("plugin:proc:/proc/meminfo", "system swap", CONFIG_BOOLEAN_AUTO);
        do_hwcorrupt    = config_get_boolean_ondemand("plugin:proc:/proc/meminfo", "hardware corrupted ECC", CONFIG_BOOLEAN_AUTO);
        do_committed    = config_get_boolean("plugin:proc:/proc/meminfo", "committed memory", 1);
        do_writeback    = config_get_boolean("plugin:proc:/proc/meminfo", "writeback memory", 1);
        do_kernel       = config_get_boolean("plugin:proc:/proc/meminfo", "kernel memory", 1);
        do_slab         = config_get_boolean("plugin:proc:/proc/meminfo", "slab memory", 1);

        arl_base = arl_create("meminfo", NULL, 60);
        arl_expect(arl_base, "MemTotal", &MemTotal);
        arl_expect(arl_base, "MemFree", &MemFree);
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
        //arl_expect(arl_base, "Shmem", &Shmem);
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
        arl_hwcorrupted = arl_expect(arl_base, "HardwareCorrupted", &HardwareCorrupted);
        //arl_expect(arl_base, "AnonHugePages", &AnonHugePages);
        //arl_expect(arl_base, "HugePages_Total", &HugePages_Total);
        //arl_expect(arl_base, "HugePages_Free", &HugePages_Free);
        //arl_expect(arl_base, "HugePages_Rsvd", &HugePages_Rsvd);
        //arl_expect(arl_base, "HugePages_Surp", &HugePages_Surp);
        //arl_expect(arl_base, "Hugepagesize", &Hugepagesize);
        //arl_expect(arl_base, "DirectMap4k", &DirectMap4k);
        //arl_expect(arl_base, "DirectMap2M", &DirectMap2M);
    }

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/meminfo");
        ff = procfile_open(config_get("plugin:proc:/proc/meminfo", "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
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

    RRDSET *st;

    // --------------------------------------------------------------------

    // http://stackoverflow.com/questions/3019748/how-to-reliably-measure-available-memory-in-linux
    unsigned long long MemUsed = MemTotal - MemFree - Cached - Buffers;

    if(do_ram) {
        st = rrdset_find_localhost("system.ram");
        if(!st) {
            st = rrdset_create_localhost("system", "ram", NULL, "ram", NULL, "System RAM", "MB", 200, update_every
                                         , RRDSET_TYPE_STACKED);

            rrddim_add(st, "free",    NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rrddim_add(st, "used",    NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rrddim_add(st, "cached",  NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rrddim_add(st, "buffers", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set(st, "free", MemFree);
        rrddim_set(st, "used", MemUsed);
        rrddim_set(st, "cached", Cached);
        rrddim_set(st, "buffers", Buffers);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    unsigned long long SwapUsed = SwapTotal - SwapFree;

    if(SwapTotal || SwapUsed || SwapFree || do_swap == CONFIG_BOOLEAN_YES) {
        do_swap = CONFIG_BOOLEAN_YES;

        st = rrdset_find_localhost("system.swap");
        if(!st) {
            st = rrdset_create_localhost("system", "swap", NULL, "swap", NULL, "System Swap", "MB", 201, update_every
                                         , RRDSET_TYPE_STACKED);
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rrddim_add(st, "free",    NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rrddim_add(st, "used",    NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set(st, "used", SwapUsed);
        rrddim_set(st, "free", SwapFree);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(arl_hwcorrupted->flags & ARL_ENTRY_FLAG_FOUND && (do_hwcorrupt == CONFIG_BOOLEAN_YES || (do_hwcorrupt == CONFIG_BOOLEAN_AUTO && HardwareCorrupted > 0))) {
        do_hwcorrupt = CONFIG_BOOLEAN_YES;

        st = rrdset_find_localhost("mem.hwcorrupt");
        if(!st) {
            st = rrdset_create_localhost("mem", "hwcorrupt", NULL, "ecc", NULL, "Hardware Corrupted ECC", "MB", 9000
                                         , update_every, RRDSET_TYPE_LINE);
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rrddim_add(st, "HardwareCorrupted", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set(st, "HardwareCorrupted", HardwareCorrupted);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_committed) {
        st = rrdset_find_localhost("mem.committed");
        if(!st) {
            st = rrdset_create_localhost("mem", "committed", NULL, "system", NULL, "Committed (Allocated) Memory", "MB"
                                         , 5000, update_every, RRDSET_TYPE_AREA);
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rrddim_add(st, "Committed_AS", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set(st, "Committed_AS", Committed_AS);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_writeback) {
        st = rrdset_find_localhost("mem.writeback");
        if(!st) {
            st = rrdset_create_localhost("mem", "writeback", NULL, "kernel", NULL, "Writeback Memory", "MB", 4000
                                         , update_every, RRDSET_TYPE_LINE);
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rrddim_add(st, "Dirty", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rrddim_add(st, "Writeback", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rrddim_add(st, "FuseWriteback", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rrddim_add(st, "NfsWriteback", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rrddim_add(st, "Bounce", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set(st, "Dirty", Dirty);
        rrddim_set(st, "Writeback", Writeback);
        rrddim_set(st, "FuseWriteback", WritebackTmp);
        rrddim_set(st, "NfsWriteback", NFS_Unstable);
        rrddim_set(st, "Bounce", Bounce);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_kernel) {
        st = rrdset_find_localhost("mem.kernel");
        if(!st) {
            st = rrdset_create_localhost("mem", "kernel", NULL, "kernel", NULL, "Memory Used by Kernel", "MB", 6000
                                         , update_every, RRDSET_TYPE_STACKED);
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rrddim_add(st, "Slab", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rrddim_add(st, "KernelStack", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rrddim_add(st, "PageTables", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rrddim_add(st, "VmallocUsed", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set(st, "KernelStack", KernelStack);
        rrddim_set(st, "Slab", Slab);
        rrddim_set(st, "PageTables", PageTables);
        rrddim_set(st, "VmallocUsed", VmallocUsed);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_slab) {
        st = rrdset_find_localhost("mem.slab");
        if(!st) {
            st = rrdset_create_localhost("mem", "slab", NULL, "slab", NULL, "Reclaimable Kernel Memory", "MB", 6500
                                         , update_every, RRDSET_TYPE_STACKED);
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rrddim_add(st, "reclaimable", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            rrddim_add(st, "unreclaimable", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set(st, "reclaimable", SReclaimable);
        rrddim_set(st, "unreclaimable", SUnreclaim);
        rrdset_done(st);
    }

    return 0;
}


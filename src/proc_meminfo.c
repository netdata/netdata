#include "common.h"

int do_proc_meminfo(int update_every, usec_t dt) {
    (void)dt;

    static procfile *ff = NULL;
    static int do_ram = -1, do_swap = -1, do_hwcorrupt = -1, do_committed = -1, do_writeback = -1, do_kernel = -1, do_slab = -1;
    static uint32_t MemTotal_hash = 0,
            MemFree_hash = 0,
            Buffers_hash = 0,
            Cached_hash = 0,
            //SwapCached_hash = 0,
            //Active_hash = 0,
            //Inactive_hash = 0,
            //ActiveAnon_hash = 0,
            //InactiveAnon_hash = 0,
            //ActiveFile_hash = 0,
            //InactiveFile_hash = 0,
            //Unevictable_hash = 0,
            //Mlocked_hash = 0,
            SwapTotal_hash = 0,
            SwapFree_hash = 0,
            Dirty_hash = 0,
            Writeback_hash = 0,
            //AnonPages_hash = 0,
            //Mapped_hash = 0,
            //Shmem_hash = 0,
            Slab_hash = 0,
            SReclaimable_hash = 0,
            SUnreclaim_hash = 0,
            KernelStack_hash = 0,
            PageTables_hash = 0,
            NFS_Unstable_hash = 0,
            Bounce_hash = 0,
            WritebackTmp_hash = 0,
            //CommitLimit_hash = 0,
            Committed_AS_hash = 0,
            //VmallocTotal_hash = 0,
            VmallocUsed_hash = 0,
            //VmallocChunk_hash = 0,
            //AnonHugePages_hash = 0,
            //HugePages_Total_hash = 0,
            //HugePages_Free_hash = 0,
            //HugePages_Rsvd_hash = 0,
            //HugePages_Surp_hash = 0,
            //Hugepagesize_hash = 0,
            //DirectMap4k_hash = 0,
            //DirectMap2M_hash = 0,
            HardwareCorrupted_hash = 0;

    if(unlikely(do_ram == -1)) {
        do_ram          = config_get_boolean("plugin:proc:/proc/meminfo", "system ram", 1);
        do_swap         = config_get_boolean_ondemand("plugin:proc:/proc/meminfo", "system swap", CONFIG_ONDEMAND_ONDEMAND);
        do_hwcorrupt    = config_get_boolean_ondemand("plugin:proc:/proc/meminfo", "hardware corrupted ECC", CONFIG_ONDEMAND_ONDEMAND);
        do_committed    = config_get_boolean("plugin:proc:/proc/meminfo", "committed memory", 1);
        do_writeback    = config_get_boolean("plugin:proc:/proc/meminfo", "writeback memory", 1);
        do_kernel       = config_get_boolean("plugin:proc:/proc/meminfo", "kernel memory", 1);
        do_slab         = config_get_boolean("plugin:proc:/proc/meminfo", "slab memory", 1);

        MemTotal_hash = simple_hash("MemTotal");
        MemFree_hash = simple_hash("MemFree");
        Buffers_hash = simple_hash("Buffers");
        Cached_hash = simple_hash("Cached");
        //SwapCached_hash = simple_hash("SwapCached");
        //Active_hash = simple_hash("Active");
        //Inactive_hash = simple_hash("Inactive");
        //ActiveAnon_hash = simple_hash("ActiveAnon");
        //InactiveAnon_hash = simple_hash("InactiveAnon");
        //ActiveFile_hash = simple_hash("ActiveFile");
        //InactiveFile_hash = simple_hash("InactiveFile");
        //Unevictable_hash = simple_hash("Unevictable");
        //Mlocked_hash = simple_hash("Mlocked");
        SwapTotal_hash = simple_hash("SwapTotal");
        SwapFree_hash = simple_hash("SwapFree");
        Dirty_hash = simple_hash("Dirty");
        Writeback_hash = simple_hash("Writeback");
        //AnonPages_hash = simple_hash("AnonPages");
        //Mapped_hash = simple_hash("Mapped");
        //Shmem_hash = simple_hash("Shmem");
        Slab_hash = simple_hash("Slab");
        SReclaimable_hash = simple_hash("SReclaimable");
        SUnreclaim_hash = simple_hash("SUnreclaim");
        KernelStack_hash = simple_hash("KernelStack");
        PageTables_hash = simple_hash("PageTables");
        NFS_Unstable_hash = simple_hash("NFS_Unstable");
        Bounce_hash = simple_hash("Bounce");
        WritebackTmp_hash = simple_hash("WritebackTmp");
        //CommitLimit_hash = simple_hash("CommitLimit");
        Committed_AS_hash = simple_hash("Committed_AS");
        //VmallocTotal_hash = simple_hash("VmallocTotal");
        VmallocUsed_hash = simple_hash("VmallocUsed");
        //VmallocChunk_hash = simple_hash("VmallocChunk");
        HardwareCorrupted_hash = simple_hash("HardwareCorrupted");
        //AnonHugePages_hash = simple_hash("AnonHugePages");
        //HugePages_Total_hash = simple_hash("HugePages_Total");
        //HugePages_Free_hash = simple_hash("HugePages_Free");
        //HugePages_Rsvd_hash = simple_hash("HugePages_Rsvd");
        //HugePages_Surp_hash = simple_hash("HugePages_Surp");
        //Hugepagesize_hash = simple_hash("Hugepagesize");
        //DirectMap4k_hash = simple_hash("DirectMap4k");
        //DirectMap2M_hash = simple_hash("DirectMap2M");
    }

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", global_host_prefix, "/proc/meminfo");
        ff = procfile_open(config_get("plugin:proc:/proc/meminfo", "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff))
            return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff))
        return 0; // we return 0, so that we will retry to open it next time

    uint32_t lines = procfile_lines(ff), l;

    int hwcorrupted = 0;

    unsigned long long MemTotal = 0,
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

    for(l = 0; l < lines ;l++) {
        uint32_t words = procfile_linewords(ff, l);
        if(unlikely(words < 2)) continue;

        char *name = procfile_lineword(ff, l, 0);
        uint32_t hash = simple_hash(name);
        unsigned long long value = strtoull(procfile_lineword(ff, l, 1), NULL, 10);

             if(hash == MemTotal_hash && strcmp(name, "MemTotal") == 0) MemTotal = value;
        else if(hash == MemFree_hash && strcmp(name, "MemFree") == 0) MemFree = value;
        else if(hash == Buffers_hash && strcmp(name, "Buffers") == 0) Buffers = value;
        else if(hash == Cached_hash && strcmp(name, "Cached") == 0) Cached = value;
        //else if(hash == SwapCached_hash && strcmp(name, "SwapCached") == 0) SwapCached = value;
        //else if(hash == Active_hash && strcmp(name, "Active") == 0) Active = value;
        //else if(hash == Inactive_hash && strcmp(name, "Inactive") == 0) Inactive = value;
        //else if(hash == ActiveAnon_hash && strcmp(name, "ActiveAnon") == 0) ActiveAnon = value;
        //else if(hash == InactiveAnon_hash && strcmp(name, "InactiveAnon") == 0) InactiveAnon = value;
        //else if(hash == ActiveFile_hash && strcmp(name, "ActiveFile") == 0) ActiveFile = value;
        //else if(hash == InactiveFile_hash && strcmp(name, "InactiveFile") == 0) InactiveFile = value;
        //else if(hash == Unevictable_hash && strcmp(name, "Unevictable") == 0) Unevictable = value;
        //else if(hash == Mlocked_hash && strcmp(name, "Mlocked") == 0) Mlocked = value;
        else if(hash == SwapTotal_hash && strcmp(name, "SwapTotal") == 0) SwapTotal = value;
        else if(hash == SwapFree_hash && strcmp(name, "SwapFree") == 0) SwapFree = value;
        else if(hash == Dirty_hash && strcmp(name, "Dirty") == 0) Dirty = value;
        else if(hash == Writeback_hash && strcmp(name, "Writeback") == 0) Writeback = value;
        //else if(hash == AnonPages_hash && strcmp(name, "AnonPages") == 0) AnonPages = value;
        //else if(hash == Mapped_hash && strcmp(name, "Mapped") == 0) Mapped = value;
        //else if(hash == Shmem_hash && strcmp(name, "Shmem") == 0) Shmem = value;
        else if(hash == Slab_hash && strcmp(name, "Slab") == 0) Slab = value;
        else if(hash == SReclaimable_hash && strcmp(name, "SReclaimable") == 0) SReclaimable = value;
        else if(hash == SUnreclaim_hash && strcmp(name, "SUnreclaim") == 0) SUnreclaim = value;
        else if(hash == KernelStack_hash && strcmp(name, "KernelStack") == 0) KernelStack = value;
        else if(hash == PageTables_hash && strcmp(name, "PageTables") == 0) PageTables = value;
        else if(hash == NFS_Unstable_hash && strcmp(name, "NFS_Unstable") == 0) NFS_Unstable = value;
        else if(hash == Bounce_hash && strcmp(name, "Bounce") == 0) Bounce = value;
        else if(hash == WritebackTmp_hash && strcmp(name, "WritebackTmp") == 0) WritebackTmp = value;
        //else if(hash == CommitLimit_hash && strcmp(name, "CommitLimit") == 0) CommitLimit = value;
        else if(hash == Committed_AS_hash && strcmp(name, "Committed_AS") == 0) Committed_AS = value;
        //else if(hash == VmallocTotal_hash && strcmp(name, "VmallocTotal") == 0) VmallocTotal = value;
        else if(hash == VmallocUsed_hash && strcmp(name, "VmallocUsed") == 0) VmallocUsed = value;
        //else if(hash == VmallocChunk_hash && strcmp(name, "VmallocChunk") == 0) VmallocChunk = value;
        else if(hash == HardwareCorrupted_hash && strcmp(name, "HardwareCorrupted") == 0) { HardwareCorrupted = value; hwcorrupted = 1; }
        //else if(hash == AnonHugePages_hash && strcmp(name, "AnonHugePages") == 0) AnonHugePages = value;
        //else if(hash == HugePages_Total_hash && strcmp(name, "HugePages_Total") == 0) HugePages_Total = value;
        //else if(hash == HugePages_Free_hash && strcmp(name, "HugePages_Free") == 0) HugePages_Free = value;
        //else if(hash == HugePages_Rsvd_hash && strcmp(name, "HugePages_Rsvd") == 0) HugePages_Rsvd = value;
        //else if(hash == HugePages_Surp_hash && strcmp(name, "HugePages_Surp") == 0) HugePages_Surp = value;
        //else if(hash == Hugepagesize_hash && strcmp(name, "Hugepagesize") == 0) Hugepagesize = value;
        //else if(hash == DirectMap4k_hash && strcmp(name, "DirectMap4k") == 0) DirectMap4k = value;
        //else if(hash == DirectMap2M_hash && strcmp(name, "DirectMap2M") == 0) DirectMap2M = value;
    }

    RRDSET *st;

    // --------------------------------------------------------------------

    // http://stackoverflow.com/questions/3019748/how-to-reliably-measure-available-memory-in-linux
    unsigned long long MemUsed = MemTotal - MemFree - Cached - Buffers;

    if(do_ram) {
        st = rrdset_find("system.ram");
        if(!st) {
            st = rrdset_create("system", "ram", NULL, "ram", NULL, "System RAM", "MB", 200, update_every, RRDSET_TYPE_STACKED);

            rrddim_add(st, "free",    NULL, 1, 1024, RRDDIM_ABSOLUTE);
            rrddim_add(st, "used",    NULL, 1, 1024, RRDDIM_ABSOLUTE);
            rrddim_add(st, "cached",  NULL, 1, 1024, RRDDIM_ABSOLUTE);
            rrddim_add(st, "buffers", NULL, 1, 1024, RRDDIM_ABSOLUTE);
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

    if(SwapTotal || SwapUsed || SwapFree || do_swap == CONFIG_ONDEMAND_YES) {
        do_swap = CONFIG_ONDEMAND_YES;

        st = rrdset_find("system.swap");
        if(!st) {
            st = rrdset_create("system", "swap", NULL, "swap", NULL, "System Swap", "MB", 201, update_every, RRDSET_TYPE_STACKED);
            st->isdetail = 1;

            rrddim_add(st, "free",    NULL, 1, 1024, RRDDIM_ABSOLUTE);
            rrddim_add(st, "used",    NULL, 1, 1024, RRDDIM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set(st, "used", SwapUsed);
        rrddim_set(st, "free", SwapFree);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(hwcorrupted && do_hwcorrupt && HardwareCorrupted > 0) {
        do_hwcorrupt = CONFIG_ONDEMAND_YES;

        st = rrdset_find("mem.hwcorrupt");
        if(!st) {
            st = rrdset_create("mem", "hwcorrupt", NULL, "errors", NULL, "Hardware Corrupted ECC", "MB", 9000, update_every, RRDSET_TYPE_LINE);
            st->isdetail = 1;

            rrddim_add(st, "HardwareCorrupted", NULL, 1, 1024, RRDDIM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set(st, "HardwareCorrupted", HardwareCorrupted);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_committed) {
        st = rrdset_find("mem.committed");
        if(!st) {
            st = rrdset_create("mem", "committed", NULL, "system", NULL, "Committed (Allocated) Memory", "MB", 5000, update_every, RRDSET_TYPE_AREA);
            st->isdetail = 1;

            rrddim_add(st, "Committed_AS", NULL, 1, 1024, RRDDIM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set(st, "Committed_AS", Committed_AS);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_writeback) {
        st = rrdset_find("mem.writeback");
        if(!st) {
            st = rrdset_create("mem", "writeback", NULL, "kernel", NULL, "Writeback Memory", "MB", 4000, update_every, RRDSET_TYPE_LINE);
            st->isdetail = 1;

            rrddim_add(st, "Dirty", NULL, 1, 1024, RRDDIM_ABSOLUTE);
            rrddim_add(st, "Writeback", NULL, 1, 1024, RRDDIM_ABSOLUTE);
            rrddim_add(st, "FuseWriteback", NULL, 1, 1024, RRDDIM_ABSOLUTE);
            rrddim_add(st, "NfsWriteback", NULL, 1, 1024, RRDDIM_ABSOLUTE);
            rrddim_add(st, "Bounce", NULL, 1, 1024, RRDDIM_ABSOLUTE);
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
        st = rrdset_find("mem.kernel");
        if(!st) {
            st = rrdset_create("mem", "kernel", NULL, "kernel", NULL, "Memory Used by Kernel", "MB", 6000, update_every, RRDSET_TYPE_STACKED);
            st->isdetail = 1;

            rrddim_add(st, "Slab", NULL, 1, 1024, RRDDIM_ABSOLUTE);
            rrddim_add(st, "KernelStack", NULL, 1, 1024, RRDDIM_ABSOLUTE);
            rrddim_add(st, "PageTables", NULL, 1, 1024, RRDDIM_ABSOLUTE);
            rrddim_add(st, "VmallocUsed", NULL, 1, 1024, RRDDIM_ABSOLUTE);
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
        st = rrdset_find("mem.slab");
        if(!st) {
            st = rrdset_create("mem", "slab", NULL, "slab", NULL, "Reclaimable Kernel Memory", "MB", 6500, update_every, RRDSET_TYPE_STACKED);
            st->isdetail = 1;

            rrddim_add(st, "reclaimable", NULL, 1, 1024, RRDDIM_ABSOLUTE);
            rrddim_add(st, "unreclaimable", NULL, 1, 1024, RRDDIM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set(st, "reclaimable", SReclaimable);
        rrddim_set(st, "unreclaimable", SUnreclaim);
        rrdset_done(st);
    }

    return 0;
}


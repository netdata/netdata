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

    size_t lines = procfile_lines(ff), l;

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

    unsigned long long *value = NULL;
    for(l = 0; l < lines ;l++) {
        size_t words = procfile_linewords(ff, l);
        if(unlikely(words < 2)) continue;

        char *name = procfile_lineword(ff, l, 0);
        uint32_t hash = simple_hash(name);

             if(hash == MemTotal_hash && strsame(name, "MemTotal") == 0) value = &MemTotal;
        else if(hash == MemFree_hash && strsame(name, "MemFree") == 0) value = &MemFree;
        else if(hash == Buffers_hash && strsame(name, "Buffers") == 0) value = &Buffers;
        else if(hash == Cached_hash && strsame(name, "Cached") == 0) value = &Cached;
        //else if(hash == SwapCached_hash && strsame(name, "SwapCached") == 0) value = &SwapCached;
        //else if(hash == Active_hash && strsame(name, "Active") == 0) value = &Active;
        //else if(hash == Inactive_hash && strsame(name, "Inactive") == 0) value = &Inactive;
        //else if(hash == ActiveAnon_hash && strsame(name, "ActiveAnon") == 0) value = &ActiveAnon;
        //else if(hash == InactiveAnon_hash && strsame(name, "InactiveAnon") == 0) value = &InactiveAnon;
        //else if(hash == ActiveFile_hash && strsame(name, "ActiveFile") == 0) value = &ActiveFile;
        //else if(hash == InactiveFile_hash && strsame(name, "InactiveFile") == 0) value = &InactiveFile;
        //else if(hash == Unevictable_hash && strsame(name, "Unevictable") == 0) value = &Unevictable;
        //else if(hash == Mlocked_hash && strsame(name, "Mlocked") == 0) value = &Mlocked;
        else if(hash == SwapTotal_hash && strsame(name, "SwapTotal") == 0) value = &SwapTotal;
        else if(hash == SwapFree_hash && strsame(name, "SwapFree") == 0) value = &SwapFree;
        else if(hash == Dirty_hash && strsame(name, "Dirty") == 0) value = &Dirty;
        else if(hash == Writeback_hash && strsame(name, "Writeback") == 0) value = &Writeback;
        //else if(hash == AnonPages_hash && strsame(name, "AnonPages") == 0) value = &AnonPages;
        //else if(hash == Mapped_hash && strsame(name, "Mapped") == 0) value = &Mapped;
        //else if(hash == Shmem_hash && strsame(name, "Shmem") == 0) value = &Shmem;
        else if(hash == Slab_hash && strsame(name, "Slab") == 0) value = &Slab;
        else if(hash == SReclaimable_hash && strsame(name, "SReclaimable") == 0) value = &SReclaimable;
        else if(hash == SUnreclaim_hash && strsame(name, "SUnreclaim") == 0) value = &SUnreclaim;
        else if(hash == KernelStack_hash && strsame(name, "KernelStack") == 0) value = &KernelStack;
        else if(hash == PageTables_hash && strsame(name, "PageTables") == 0) value = &PageTables;
        else if(hash == NFS_Unstable_hash && strsame(name, "NFS_Unstable") == 0) value = &NFS_Unstable;
        else if(hash == Bounce_hash && strsame(name, "Bounce") == 0) value = &Bounce;
        else if(hash == WritebackTmp_hash && strsame(name, "WritebackTmp") == 0) value = &WritebackTmp;
        //else if(hash == CommitLimit_hash && strsame(name, "CommitLimit") == 0) value = &CommitLimit;
        else if(hash == Committed_AS_hash && strsame(name, "Committed_AS") == 0) value = &Committed_AS;
        //else if(hash == VmallocTotal_hash && strsame(name, "VmallocTotal") == 0) value = &VmallocTotal;
        else if(hash == VmallocUsed_hash && strsame(name, "VmallocUsed") == 0) value = &VmallocUsed;
        //else if(hash == VmallocChunk_hash && strsame(name, "VmallocChunk") == 0) value = &VmallocChunk;
        else if(hash == HardwareCorrupted_hash && strsame(name, "HardwareCorrupted") == 0) { value = &HardwareCorrupted; hwcorrupted = 1; }
        //else if(hash == AnonHugePages_hash && strsame(name, "AnonHugePages") == 0) value = &AnonHugePages;
        //else if(hash == HugePages_Total_hash && strsame(name, "HugePages_Total") == 0) value = &HugePages_Total;
        //else if(hash == HugePages_Free_hash && strsame(name, "HugePages_Free") == 0) value = &HugePages_Free;
        //else if(hash == HugePages_Rsvd_hash && strsame(name, "HugePages_Rsvd") == 0) value = &HugePages_Rsvd;
        //else if(hash == HugePages_Surp_hash && strsame(name, "HugePages_Surp") == 0) value = &HugePages_Surp;
        //else if(hash == Hugepagesize_hash && strsame(name, "Hugepagesize") == 0) value = &Hugepagesize;
        //else if(hash == DirectMap4k_hash && strsame(name, "DirectMap4k") == 0) value = &DirectMap4k;
        //else if(hash == DirectMap2M_hash && strsame(name, "DirectMap2M") == 0) value = &DirectMap2M;

        if(value) {
            *value = str2ull(procfile_lineword(ff, l, 1));
            value = NULL;
        }
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

    if(hwcorrupted && (do_hwcorrupt == CONFIG_ONDEMAND_YES || (do_hwcorrupt == CONFIG_ONDEMAND_ONDEMAND && HardwareCorrupted > 0))) {
        do_hwcorrupt = CONFIG_ONDEMAND_YES;

        st = rrdset_find("mem.hwcorrupt");
        if(!st) {
            st = rrdset_create("mem", "hwcorrupt", NULL, "ecc", NULL, "Hardware Corrupted ECC", "MB", 9000, update_every, RRDSET_TYPE_LINE);
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


#include "common.h"

int do_proc_vmstat(int update_every, usec_t dt) {
    (void)dt;

    static procfile *ff = NULL;
    static int do_swapio = -1, do_io = -1, do_pgfaults = -1, do_numa = -1;
    static int has_numa = -1;

    // static uint32_t hash_allocstall_dma = 0;
    // static uint32_t hash_allocstall_dma32 = 0;
    // static uint32_t hash_allocstall_movable = 0;
    // static uint32_t hash_allocstall_normal = 0;
    // static uint32_t hash_balloon_deflate = 0;
    // static uint32_t hash_balloon_inflate = 0;
    // static uint32_t hash_balloon_migrate = 0;
    // static uint32_t hash_compact_daemon_wake = 0;
    // static uint32_t hash_compact_fail = 0;
    // static uint32_t hash_compact_free_scanned = 0;
    // static uint32_t hash_compact_isolated = 0;
    // static uint32_t hash_compact_migrate_scanned = 0;
    // static uint32_t hash_compact_stall = 0;
    // static uint32_t hash_compact_success = 0;
    // static uint32_t hash_drop_pagecache = 0;
    // static uint32_t hash_drop_slab = 0;
    // static uint32_t hash_htlb_buddy_alloc_fail = 0;
    // static uint32_t hash_htlb_buddy_alloc_success = 0;
    // static uint32_t hash_kswapd_high_wmark_hit_quickly = 0;
    // static uint32_t hash_kswapd_inodesteal = 0;
    // static uint32_t hash_kswapd_low_wmark_hit_quickly = 0;
    // static uint32_t hash_nr_active_anon = 0;
    // static uint32_t hash_nr_active_file = 0;
    // static uint32_t hash_nr_anon_pages = 0;
    // static uint32_t hash_nr_anon_transparent_hugepages = 0;
    // static uint32_t hash_nr_bounce = 0;
    // static uint32_t hash_nr_dirtied = 0;
    // static uint32_t hash_nr_dirty = 0;
    // static uint32_t hash_nr_dirty_background_threshold = 0;
    // static uint32_t hash_nr_dirty_threshold = 0;
    // static uint32_t hash_nr_file_pages = 0;
    // static uint32_t hash_nr_free_cma = 0;
    // static uint32_t hash_nr_free_pages = 0;
    // static uint32_t hash_nr_inactive_anon = 0;
    // static uint32_t hash_nr_inactive_file = 0;
    // static uint32_t hash_nr_isolated_anon = 0;
    // static uint32_t hash_nr_isolated_file = 0;
    // static uint32_t hash_nr_kernel_stack = 0;
    // static uint32_t hash_nr_mapped = 0;
    // static uint32_t hash_nr_mlock = 0;
    // static uint32_t hash_nr_pages_scanned = 0;
    // static uint32_t hash_nr_page_table_pages = 0;
    // static uint32_t hash_nr_shmem = 0;
    // static uint32_t hash_nr_shmem_hugepages = 0;
    // static uint32_t hash_nr_shmem_pmdmapped = 0;
    // static uint32_t hash_nr_slab_reclaimable = 0;
    // static uint32_t hash_nr_slab_unreclaimable = 0;
    // static uint32_t hash_nr_unevictable = 0;
    // static uint32_t hash_nr_unstable = 0;
    // static uint32_t hash_nr_vmscan_immediate_reclaim = 0;
    // static uint32_t hash_nr_vmscan_write = 0;
    // static uint32_t hash_nr_writeback = 0;
    // static uint32_t hash_nr_writeback_temp = 0;
    // static uint32_t hash_nr_written = 0;
    // static uint32_t hash_nr_zone_active_anon = 0;
    // static uint32_t hash_nr_zone_active_file = 0;
    // static uint32_t hash_nr_zone_inactive_anon = 0;
    // static uint32_t hash_nr_zone_inactive_file = 0;
    // static uint32_t hash_nr_zone_unevictable = 0;
    // static uint32_t hash_nr_zone_write_pending = 0;
    // static uint32_t hash_nr_zspages = 0;
    static uint32_t hash_numa_foreign = 0;
    static uint32_t hash_numa_hint_faults = 0;
    static uint32_t hash_numa_hint_faults_local = 0;
    //static uint32_t hash_numa_hit = 0;
    static uint32_t hash_numa_huge_pte_updates = 0;
    static uint32_t hash_numa_interleave = 0;
    static uint32_t hash_numa_local = 0;
    //static uint32_t hash_numa_miss = 0;
    static uint32_t hash_numa_other = 0;
    static uint32_t hash_numa_pages_migrated = 0;
    static uint32_t hash_numa_pte_updates = 0;
    // static uint32_t hash_pageoutrun = 0;
    // static uint32_t hash_pgactivate = 0;
    // static uint32_t hash_pgalloc_dma = 0;
    // static uint32_t hash_pgalloc_dma32 = 0;
    // static uint32_t hash_pgalloc_movable = 0;
    // static uint32_t hash_pgalloc_normal = 0;
    // static uint32_t hash_pgdeactivate = 0;
    static uint32_t hash_pgfault = 0;
    // static uint32_t hash_pgfree = 0;
    // static uint32_t hash_pginodesteal = 0;
    // static uint32_t hash_pglazyfreed = 0;
    static uint32_t hash_pgmajfault = 0;
    // static uint32_t hash_pgmigrate_fail = 0;
    // static uint32_t hash_pgmigrate_success = 0;
    static uint32_t hash_pgpgin = 0;
    static uint32_t hash_pgpgout = 0;
    // static uint32_t hash_pgrefill = 0;
    // static uint32_t hash_pgrotated = 0;
    // static uint32_t hash_pgscan_direct = 0;
    // static uint32_t hash_pgscan_direct_throttle = 0;
    // static uint32_t hash_pgscan_kswapd = 0;
    // static uint32_t hash_pgskip_dma = 0;
    // static uint32_t hash_pgskip_dma32 = 0;
    // static uint32_t hash_pgskip_movable = 0;
    // static uint32_t hash_pgskip_normal = 0;
    // static uint32_t hash_pgsteal_direct = 0;
    // static uint32_t hash_pgsteal_kswapd = 0;
    static uint32_t hash_pswpin = 0;
    static uint32_t hash_pswpout = 0;
    // static uint32_t hash_slabs_scanned = 0;
    // static uint32_t hash_thp_collapse_alloc = 0;
    // static uint32_t hash_thp_collapse_alloc_failed = 0;
    // static uint32_t hash_thp_deferred_split_page = 0;
    // static uint32_t hash_thp_fault_alloc = 0;
    // static uint32_t hash_thp_fault_fallback = 0;
    // static uint32_t hash_thp_file_alloc = 0;
    // static uint32_t hash_thp_file_mapped = 0;
    // static uint32_t hash_thp_split_page = 0;
    // static uint32_t hash_thp_split_page_failed = 0;
    // static uint32_t hash_thp_split_pmd = 0;
    // static uint32_t hash_thp_zero_page_alloc = 0;
    // static uint32_t hash_thp_zero_page_alloc_failed = 0;
    // static uint32_t hash_unevictable_pgs_cleared = 0;
    // static uint32_t hash_unevictable_pgs_culled = 0;
    // static uint32_t hash_unevictable_pgs_mlocked = 0;
    // static uint32_t hash_unevictable_pgs_munlocked = 0;
    // static uint32_t hash_unevictable_pgs_rescued = 0;
    // static uint32_t hash_unevictable_pgs_scanned = 0;
    // static uint32_t hash_unevictable_pgs_stranded = 0;
    // static uint32_t hash_workingset_activate = 0;
    // static uint32_t hash_workingset_nodereclaim = 0;
    // static uint32_t hash_workingset_refault = 0;
    // static uint32_t hash_zone_reclaim_failed = 0;

    if(unlikely(do_swapio == -1)) {
        do_swapio = config_get_boolean_ondemand("plugin:proc:/proc/vmstat", "swap i/o", CONFIG_ONDEMAND_ONDEMAND);
        do_io = config_get_boolean("plugin:proc:/proc/vmstat", "disk i/o", 1);
        do_pgfaults = config_get_boolean("plugin:proc:/proc/vmstat", "memory page faults", 1);
        do_numa = config_get_boolean_ondemand("plugin:proc:/proc/vmstat", "system-wide numa metric summary", CONFIG_ONDEMAND_ONDEMAND);

        // hash_allocstall_dma32 = simple_hash("allocstall_dma32");
        // hash_allocstall_dma = simple_hash("allocstall_dma");
        // hash_allocstall_movable = simple_hash("allocstall_movable");
        // hash_allocstall_normal = simple_hash("allocstall_normal");
        // hash_balloon_deflate = simple_hash("balloon_deflate");
        // hash_balloon_inflate = simple_hash("balloon_inflate");
        // hash_balloon_migrate = simple_hash("balloon_migrate");
        // hash_compact_daemon_wake = simple_hash("compact_daemon_wake");
        // hash_compact_fail = simple_hash("compact_fail");
        // hash_compact_free_scanned = simple_hash("compact_free_scanned");
        // hash_compact_isolated = simple_hash("compact_isolated");
        // hash_compact_migrate_scanned = simple_hash("compact_migrate_scanned");
        // hash_compact_stall = simple_hash("compact_stall");
        // hash_compact_success = simple_hash("compact_success");
        // hash_drop_pagecache = simple_hash("drop_pagecache");
        // hash_drop_slab = simple_hash("drop_slab");
        // hash_htlb_buddy_alloc_fail = simple_hash("htlb_buddy_alloc_fail");
        // hash_htlb_buddy_alloc_success = simple_hash("htlb_buddy_alloc_success");
        // hash_kswapd_high_wmark_hit_quickly = simple_hash("kswapd_high_wmark_hit_quickly");
        // hash_kswapd_inodesteal = simple_hash("kswapd_inodesteal");
        // hash_kswapd_low_wmark_hit_quickly = simple_hash("kswapd_low_wmark_hit_quickly");
        // hash_nr_active_anon = simple_hash("nr_active_anon");
        // hash_nr_active_file = simple_hash("nr_active_file");
        // hash_nr_anon_pages = simple_hash("nr_anon_pages");
        // hash_nr_anon_transparent_hugepages = simple_hash("nr_anon_transparent_hugepages");
        // hash_nr_bounce = simple_hash("nr_bounce");
        // hash_nr_dirtied = simple_hash("nr_dirtied");
        // hash_nr_dirty_background_threshold = simple_hash("nr_dirty_background_threshold");
        // hash_nr_dirty = simple_hash("nr_dirty");
        // hash_nr_dirty_threshold = simple_hash("nr_dirty_threshold");
        // hash_nr_file_pages = simple_hash("nr_file_pages");
        // hash_nr_free_cma = simple_hash("nr_free_cma");
        // hash_nr_free_pages = simple_hash("nr_free_pages");
        // hash_nr_inactive_anon = simple_hash("nr_inactive_anon");
        // hash_nr_inactive_file = simple_hash("nr_inactive_file");
        // hash_nr_isolated_anon = simple_hash("nr_isolated_anon");
        // hash_nr_isolated_file = simple_hash("nr_isolated_file");
        // hash_nr_kernel_stack = simple_hash("nr_kernel_stack");
        // hash_nr_mapped = simple_hash("nr_mapped");
        // hash_nr_mlock = simple_hash("nr_mlock");
        // hash_nr_pages_scanned = simple_hash("nr_pages_scanned");
        // hash_nr_page_table_pages = simple_hash("nr_page_table_pages");
        // hash_nr_shmem_hugepages = simple_hash("nr_shmem_hugepages");
        // hash_nr_shmem_pmdmapped = simple_hash("nr_shmem_pmdmapped");
        // hash_nr_shmem = simple_hash("nr_shmem");
        // hash_nr_slab_reclaimable = simple_hash("nr_slab_reclaimable");
        // hash_nr_slab_unreclaimable = simple_hash("nr_slab_unreclaimable");
        // hash_nr_unevictable = simple_hash("nr_unevictable");
        // hash_nr_unstable = simple_hash("nr_unstable");
        // hash_nr_vmscan_immediate_reclaim = simple_hash("nr_vmscan_immediate_reclaim");
        // hash_nr_vmscan_write = simple_hash("nr_vmscan_write");
        // hash_nr_writeback = simple_hash("nr_writeback");
        // hash_nr_writeback_temp = simple_hash("nr_writeback_temp");
        // hash_nr_written = simple_hash("nr_written");
        // hash_nr_zone_active_anon = simple_hash("nr_zone_active_anon");
        // hash_nr_zone_active_file = simple_hash("nr_zone_active_file");
        // hash_nr_zone_inactive_anon = simple_hash("nr_zone_inactive_anon");
        // hash_nr_zone_inactive_file = simple_hash("nr_zone_inactive_file");
        // hash_nr_zone_unevictable = simple_hash("nr_zone_unevictable");
        // hash_nr_zone_write_pending = simple_hash("nr_zone_write_pending");
        // hash_nr_zspages = simple_hash("nr_zspages");
        hash_numa_foreign = simple_hash("numa_foreign");
        hash_numa_hint_faults_local = simple_hash("numa_hint_faults_local");
        hash_numa_hint_faults = simple_hash("numa_hint_faults");
        //hash_numa_hit = simple_hash("numa_hit");
        hash_numa_huge_pte_updates = simple_hash("numa_huge_pte_updates");
        hash_numa_interleave = simple_hash("numa_interleave");
        hash_numa_local = simple_hash("numa_local");
        //hash_numa_miss = simple_hash("numa_miss");
        hash_numa_other = simple_hash("numa_other");
        hash_numa_pages_migrated = simple_hash("numa_pages_migrated");
        hash_numa_pte_updates = simple_hash("numa_pte_updates");
        // hash_pageoutrun = simple_hash("pageoutrun");
        // hash_pgactivate = simple_hash("pgactivate");
        // hash_pgalloc_dma32 = simple_hash("pgalloc_dma32");
        // hash_pgalloc_dma = simple_hash("pgalloc_dma");
        // hash_pgalloc_movable = simple_hash("pgalloc_movable");
        // hash_pgalloc_normal = simple_hash("pgalloc_normal");
        // hash_pgdeactivate = simple_hash("pgdeactivate");
        hash_pgfault = simple_hash("pgfault");
        // hash_pgfree = simple_hash("pgfree");
        // hash_pginodesteal = simple_hash("pginodesteal");
        // hash_pglazyfreed = simple_hash("pglazyfreed");
        hash_pgmajfault = simple_hash("pgmajfault");
        // hash_pgmigrate_fail = simple_hash("pgmigrate_fail");
        // hash_pgmigrate_success = simple_hash("pgmigrate_success");
        hash_pgpgin = simple_hash("pgpgin");
        hash_pgpgout = simple_hash("pgpgout");
        // hash_pgrefill = simple_hash("pgrefill");
        // hash_pgrotated = simple_hash("pgrotated");
        // hash_pgscan_direct = simple_hash("pgscan_direct");
        // hash_pgscan_direct_throttle = simple_hash("pgscan_direct_throttle");
        // hash_pgscan_kswapd = simple_hash("pgscan_kswapd");
        // hash_pgskip_dma32 = simple_hash("pgskip_dma32");
        // hash_pgskip_dma = simple_hash("pgskip_dma");
        // hash_pgskip_movable = simple_hash("pgskip_movable");
        // hash_pgskip_normal = simple_hash("pgskip_normal");
        // hash_pgsteal_direct = simple_hash("pgsteal_direct");
        // hash_pgsteal_kswapd = simple_hash("pgsteal_kswapd");
        hash_pswpin = simple_hash("pswpin");
        hash_pswpout = simple_hash("pswpout");
        // hash_slabs_scanned = simple_hash("slabs_scanned");
        // hash_thp_collapse_alloc_failed = simple_hash("thp_collapse_alloc_failed");
        // hash_thp_collapse_alloc = simple_hash("thp_collapse_alloc");
        // hash_thp_deferred_split_page = simple_hash("thp_deferred_split_page");
        // hash_thp_fault_alloc = simple_hash("thp_fault_alloc");
        // hash_thp_fault_fallback = simple_hash("thp_fault_fallback");
        // hash_thp_file_alloc = simple_hash("thp_file_alloc");
        // hash_thp_file_mapped = simple_hash("thp_file_mapped");
        // hash_thp_split_page_failed = simple_hash("thp_split_page_failed");
        // hash_thp_split_page = simple_hash("thp_split_page");
        // hash_thp_split_pmd = simple_hash("thp_split_pmd");
        // hash_thp_zero_page_alloc_failed = simple_hash("thp_zero_page_alloc_failed");
        // hash_thp_zero_page_alloc = simple_hash("thp_zero_page_alloc");
        // hash_unevictable_pgs_cleared = simple_hash("unevictable_pgs_cleared");
        // hash_unevictable_pgs_culled = simple_hash("unevictable_pgs_culled");
        // hash_unevictable_pgs_mlocked = simple_hash("unevictable_pgs_mlocked");
        // hash_unevictable_pgs_munlocked = simple_hash("unevictable_pgs_munlocked");
        // hash_unevictable_pgs_rescued = simple_hash("unevictable_pgs_rescued");
        // hash_unevictable_pgs_scanned = simple_hash("unevictable_pgs_scanned");
        // hash_unevictable_pgs_stranded = simple_hash("unevictable_pgs_stranded");
        // hash_workingset_activate = simple_hash("workingset_activate");
        // hash_workingset_nodereclaim = simple_hash("workingset_nodereclaim");
        // hash_workingset_refault = simple_hash("workingset_refault");
        // hash_zone_reclaim_failed = simple_hash("zone_reclaim_failed");
    }

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", global_host_prefix, "/proc/vmstat");
        ff = procfile_open(config_get("plugin:proc:/proc/vmstat", "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 0; // we return 0, so that we will retry to open it next time

    uint32_t lines = procfile_lines(ff), l;

    // unsigned long long allocstall_dma = 0ULL;
    // unsigned long long allocstall_dma32 = 0ULL;
    // unsigned long long allocstall_movable = 0ULL;
    // unsigned long long allocstall_normal = 0ULL;
    // unsigned long long balloon_deflate = 0ULL;
    // unsigned long long balloon_inflate = 0ULL;
    // unsigned long long balloon_migrate = 0ULL;
    // unsigned long long compact_daemon_wake = 0ULL;
    // unsigned long long compact_fail = 0ULL;
    // unsigned long long compact_free_scanned = 0ULL;
    // unsigned long long compact_isolated = 0ULL;
    // unsigned long long compact_migrate_scanned = 0ULL;
    // unsigned long long compact_stall = 0ULL;
    // unsigned long long compact_success = 0ULL;
    // unsigned long long drop_pagecache = 0ULL;
    // unsigned long long drop_slab = 0ULL;
    // unsigned long long htlb_buddy_alloc_fail = 0ULL;
    // unsigned long long htlb_buddy_alloc_success = 0ULL;
    // unsigned long long kswapd_high_wmark_hit_quickly = 0ULL;
    // unsigned long long kswapd_inodesteal = 0ULL;
    // unsigned long long kswapd_low_wmark_hit_quickly = 0ULL;
    // unsigned long long nr_active_anon = 0ULL;
    // unsigned long long nr_active_file = 0ULL;
    // unsigned long long nr_anon_pages = 0ULL;
    // unsigned long long nr_anon_transparent_hugepages = 0ULL;
    // unsigned long long nr_bounce = 0ULL;
    // unsigned long long nr_dirtied = 0ULL;
    // unsigned long long nr_dirty = 0ULL;
    // unsigned long long nr_dirty_background_threshold = 0ULL;
    // unsigned long long nr_dirty_threshold = 0ULL;
    // unsigned long long nr_file_pages = 0ULL;
    // unsigned long long nr_free_cma = 0ULL;
    // unsigned long long nr_free_pages = 0ULL;
    // unsigned long long nr_inactive_anon = 0ULL;
    // unsigned long long nr_inactive_file = 0ULL;
    // unsigned long long nr_isolated_anon = 0ULL;
    // unsigned long long nr_isolated_file = 0ULL;
    // unsigned long long nr_kernel_stack = 0ULL;
    // unsigned long long nr_mapped = 0ULL;
    // unsigned long long nr_mlock = 0ULL;
    // unsigned long long nr_pages_scanned = 0ULL;
    // unsigned long long nr_page_table_pages = 0ULL;
    // unsigned long long nr_shmem = 0ULL;
    // unsigned long long nr_shmem_hugepages = 0ULL;
    // unsigned long long nr_shmem_pmdmapped = 0ULL;
    // unsigned long long nr_slab_reclaimable = 0ULL;
    // unsigned long long nr_slab_unreclaimable = 0ULL;
    // unsigned long long nr_unevictable = 0ULL;
    // unsigned long long nr_unstable = 0ULL;
    // unsigned long long nr_vmscan_immediate_reclaim = 0ULL;
    // unsigned long long nr_vmscan_write = 0ULL;
    // unsigned long long nr_writeback = 0ULL;
    // unsigned long long nr_writeback_temp = 0ULL;
    // unsigned long long nr_written = 0ULL;
    // unsigned long long nr_zone_active_anon = 0ULL;
    // unsigned long long nr_zone_active_file = 0ULL;
    // unsigned long long nr_zone_inactive_anon = 0ULL;
    // unsigned long long nr_zone_inactive_file = 0ULL;
    // unsigned long long nr_zone_unevictable = 0ULL;
    // unsigned long long nr_zone_write_pending = 0ULL;
    // unsigned long long nr_zspages = 0ULL;
    unsigned long long numa_foreign = 0ULL;
    unsigned long long numa_hint_faults = 0ULL;
    unsigned long long numa_hint_faults_local = 0ULL;
    //unsigned long long numa_hit = 0ULL;
    unsigned long long numa_huge_pte_updates = 0ULL;
    unsigned long long numa_interleave = 0ULL;
    unsigned long long numa_local = 0ULL;
    //unsigned long long numa_miss = 0ULL;
    unsigned long long numa_other = 0ULL;
    unsigned long long numa_pages_migrated = 0ULL;
    unsigned long long numa_pte_updates = 0ULL;
    // unsigned long long pageoutrun = 0ULL;
    // unsigned long long pgactivate = 0ULL;
    // unsigned long long pgalloc_dma = 0ULL;
    // unsigned long long pgalloc_dma32 = 0ULL;
    // unsigned long long pgalloc_movable = 0ULL;
    // unsigned long long pgalloc_normal = 0ULL;
    // unsigned long long pgdeactivate = 0ULL;
    unsigned long long pgfault = 0ULL;
    // unsigned long long pgfree = 0ULL;
    // unsigned long long pginodesteal = 0ULL;
    // unsigned long long pglazyfreed = 0ULL;
    unsigned long long pgmajfault = 0ULL;
    // unsigned long long pgmigrate_fail = 0ULL;
    // unsigned long long pgmigrate_success = 0ULL;
    unsigned long long pgpgin = 0ULL;
    unsigned long long pgpgout = 0ULL;
    // unsigned long long pgrefill = 0ULL;
    // unsigned long long pgrotated = 0ULL;
    // unsigned long long pgscan_direct = 0ULL;
    // unsigned long long pgscan_direct_throttle = 0ULL;
    // unsigned long long pgscan_kswapd = 0ULL;
    // unsigned long long pgskip_dma = 0ULL;
    // unsigned long long pgskip_dma32 = 0ULL;
    // unsigned long long pgskip_movable = 0ULL;
    // unsigned long long pgskip_normal = 0ULL;
    // unsigned long long pgsteal_direct = 0ULL;
    // unsigned long long pgsteal_kswapd = 0ULL;
    unsigned long long pswpin = 0ULL;
    unsigned long long pswpout = 0ULL;
    // unsigned long long slabs_scanned = 0ULL;
    // unsigned long long thp_collapse_alloc = 0ULL;
    // unsigned long long thp_collapse_alloc_failed = 0ULL;
    // unsigned long long thp_deferred_split_page = 0ULL;
    // unsigned long long thp_fault_alloc = 0ULL;
    // unsigned long long thp_fault_fallback = 0ULL;
    // unsigned long long thp_file_alloc = 0ULL;
    // unsigned long long thp_file_mapped = 0ULL;
    // unsigned long long thp_split_page = 0ULL;
    // unsigned long long thp_split_page_failed = 0ULL;
    // unsigned long long thp_split_pmd = 0ULL;
    // unsigned long long thp_zero_page_alloc = 0ULL;
    // unsigned long long thp_zero_page_alloc_failed = 0ULL;
    // unsigned long long unevictable_pgs_cleared = 0ULL;
    // unsigned long long unevictable_pgs_culled = 0ULL;
    // unsigned long long unevictable_pgs_mlocked = 0ULL;
    // unsigned long long unevictable_pgs_munlocked = 0ULL;
    // unsigned long long unevictable_pgs_rescued = 0ULL;
    // unsigned long long unevictable_pgs_scanned = 0ULL;
    // unsigned long long unevictable_pgs_stranded = 0ULL;
    // unsigned long long workingset_activate = 0ULL;
    // unsigned long long workingset_nodereclaim = 0ULL;
    // unsigned long long workingset_refault = 0ULL;
    // unsigned long long zone_reclaim_failed = 0ULL;

    for(l = 0; l < lines ;l++) {
        uint32_t words = procfile_linewords(ff, l);
        if(unlikely(words < 2)) {
            if(unlikely(words)) error("Cannot read /proc/vmstat line %u. Expected 2 params, read %u.", l, words);
            continue;
        }

        char *name = procfile_lineword(ff, l, 0);
        char * value = procfile_lineword(ff, l, 1);
        if(unlikely(!name || !*name || !value || !*value)) continue;

        uint32_t hash = simple_hash(name);

        if(unlikely(0)) ;
        // else if(unlikely(hash == hash_allocstall_dma32 && strcmp(name, "allocstall_dma32") == 0)) allocstall_dma32 = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_allocstall_dma && strcmp(name, "allocstall_dma") == 0)) allocstall_dma = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_allocstall_movable && strcmp(name, "allocstall_movable") == 0)) allocstall_movable = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_allocstall_normal && strcmp(name, "allocstall_normal") == 0)) allocstall_normal = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_balloon_deflate && strcmp(name, "balloon_deflate") == 0)) balloon_deflate = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_balloon_inflate && strcmp(name, "balloon_inflate") == 0)) balloon_inflate = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_balloon_migrate && strcmp(name, "balloon_migrate") == 0)) balloon_migrate = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_compact_daemon_wake && strcmp(name, "compact_daemon_wake") == 0)) compact_daemon_wake = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_compact_fail && strcmp(name, "compact_fail") == 0)) compact_fail = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_compact_free_scanned && strcmp(name, "compact_free_scanned") == 0)) compact_free_scanned = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_compact_isolated && strcmp(name, "compact_isolated") == 0)) compact_isolated = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_compact_migrate_scanned && strcmp(name, "compact_migrate_scanned") == 0)) compact_migrate_scanned = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_compact_stall && strcmp(name, "compact_stall") == 0)) compact_stall = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_compact_success && strcmp(name, "compact_success") == 0)) compact_success = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_drop_pagecache && strcmp(name, "drop_pagecache") == 0)) drop_pagecache = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_drop_slab && strcmp(name, "drop_slab") == 0)) drop_slab = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_htlb_buddy_alloc_fail && strcmp(name, "htlb_buddy_alloc_fail") == 0)) htlb_buddy_alloc_fail = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_htlb_buddy_alloc_success && strcmp(name, "htlb_buddy_alloc_success") == 0)) htlb_buddy_alloc_success = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_kswapd_high_wmark_hit_quickly && strcmp(name, "kswapd_high_wmark_hit_quickly") == 0)) kswapd_high_wmark_hit_quickly = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_kswapd_inodesteal && strcmp(name, "kswapd_inodesteal") == 0)) kswapd_inodesteal = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_kswapd_low_wmark_hit_quickly && strcmp(name, "kswapd_low_wmark_hit_quickly") == 0)) kswapd_low_wmark_hit_quickly = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_active_anon && strcmp(name, "nr_active_anon") == 0)) nr_active_anon = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_active_file && strcmp(name, "nr_active_file") == 0)) nr_active_file = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_anon_pages && strcmp(name, "nr_anon_pages") == 0)) nr_anon_pages = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_anon_transparent_hugepages && strcmp(name, "nr_anon_transparent_hugepages") == 0)) nr_anon_transparent_hugepages = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_bounce && strcmp(name, "nr_bounce") == 0)) nr_bounce = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_dirtied && strcmp(name, "nr_dirtied") == 0)) nr_dirtied = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_dirty_background_threshold && strcmp(name, "nr_dirty_background_threshold") == 0)) nr_dirty_background_threshold = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_dirty && strcmp(name, "nr_dirty") == 0)) nr_dirty = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_dirty_threshold && strcmp(name, "nr_dirty_threshold") == 0)) nr_dirty_threshold = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_file_pages && strcmp(name, "nr_file_pages") == 0)) nr_file_pages = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_free_cma && strcmp(name, "nr_free_cma") == 0)) nr_free_cma = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_free_pages && strcmp(name, "nr_free_pages") == 0)) nr_free_pages = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_inactive_anon && strcmp(name, "nr_inactive_anon") == 0)) nr_inactive_anon = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_inactive_file && strcmp(name, "nr_inactive_file") == 0)) nr_inactive_file = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_isolated_anon && strcmp(name, "nr_isolated_anon") == 0)) nr_isolated_anon = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_isolated_file && strcmp(name, "nr_isolated_file") == 0)) nr_isolated_file = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_kernel_stack && strcmp(name, "nr_kernel_stack") == 0)) nr_kernel_stack = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_mapped && strcmp(name, "nr_mapped") == 0)) nr_mapped = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_mlock && strcmp(name, "nr_mlock") == 0)) nr_mlock = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_pages_scanned && strcmp(name, "nr_pages_scanned") == 0)) nr_pages_scanned = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_page_table_pages && strcmp(name, "nr_page_table_pages") == 0)) nr_page_table_pages = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_shmem_hugepages && strcmp(name, "nr_shmem_hugepages") == 0)) nr_shmem_hugepages = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_shmem_pmdmapped && strcmp(name, "nr_shmem_pmdmapped") == 0)) nr_shmem_pmdmapped = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_shmem && strcmp(name, "nr_shmem") == 0)) nr_shmem = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_slab_reclaimable && strcmp(name, "nr_slab_reclaimable") == 0)) nr_slab_reclaimable = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_slab_unreclaimable && strcmp(name, "nr_slab_unreclaimable") == 0)) nr_slab_unreclaimable = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_unevictable && strcmp(name, "nr_unevictable") == 0)) nr_unevictable = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_unstable && strcmp(name, "nr_unstable") == 0)) nr_unstable = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_vmscan_immediate_reclaim && strcmp(name, "nr_vmscan_immediate_reclaim") == 0)) nr_vmscan_immediate_reclaim = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_vmscan_write && strcmp(name, "nr_vmscan_write") == 0)) nr_vmscan_write = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_writeback && strcmp(name, "nr_writeback") == 0)) nr_writeback = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_writeback_temp && strcmp(name, "nr_writeback_temp") == 0)) nr_writeback_temp = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_written && strcmp(name, "nr_written") == 0)) nr_written = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_zone_active_anon && strcmp(name, "nr_zone_active_anon") == 0)) nr_zone_active_anon = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_zone_active_file && strcmp(name, "nr_zone_active_file") == 0)) nr_zone_active_file = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_zone_inactive_anon && strcmp(name, "nr_zone_inactive_anon") == 0)) nr_zone_inactive_anon = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_zone_inactive_file && strcmp(name, "nr_zone_inactive_file") == 0)) nr_zone_inactive_file = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_zone_unevictable && strcmp(name, "nr_zone_unevictable") == 0)) nr_zone_unevictable = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_zone_write_pending && strcmp(name, "nr_zone_write_pending") == 0)) nr_zone_write_pending = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_nr_zspages && strcmp(name, "nr_zspages") == 0)) nr_zspages = strtoull(value, NULL, 10);
        else if(unlikely(hash == hash_numa_foreign && strcmp(name, "numa_foreign") == 0)) numa_foreign = strtoull(value, NULL, 10);
        else if(unlikely(hash == hash_numa_hint_faults_local && strcmp(name, "numa_hint_faults_local") == 0)) numa_hint_faults_local = strtoull(value, NULL, 10);
        else if(unlikely(hash == hash_numa_hint_faults && strcmp(name, "numa_hint_faults") == 0)) numa_hint_faults = strtoull(value, NULL, 10);
        //else if(unlikely(hash == hash_numa_hit && strcmp(name, "numa_hit") == 0)) numa_hit = strtoull(value, NULL, 10);
        else if(unlikely(hash == hash_numa_huge_pte_updates && strcmp(name, "numa_huge_pte_updates") == 0)) numa_huge_pte_updates = strtoull(value, NULL, 10);
        else if(unlikely(hash == hash_numa_interleave && strcmp(name, "numa_interleave") == 0)) numa_interleave = strtoull(value, NULL, 10);
        else if(unlikely(hash == hash_numa_local && strcmp(name, "numa_local") == 0)) numa_local = strtoull(value, NULL, 10);
        //else if(unlikely(hash == hash_numa_miss && strcmp(name, "numa_miss") == 0)) numa_miss = strtoull(value, NULL, 10);
        else if(unlikely(hash == hash_numa_other && strcmp(name, "numa_other") == 0)) numa_other = strtoull(value, NULL, 10);
        else if(unlikely(hash == hash_numa_pages_migrated && strcmp(name, "numa_pages_migrated") == 0)) numa_pages_migrated = strtoull(value, NULL, 10);
        else if(unlikely(hash == hash_numa_pte_updates && strcmp(name, "numa_pte_updates") == 0)) numa_pte_updates = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_pageoutrun && strcmp(name, "pageoutrun") == 0)) pageoutrun = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_pgactivate && strcmp(name, "pgactivate") == 0)) pgactivate = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_pgalloc_dma32 && strcmp(name, "pgalloc_dma32") == 0)) pgalloc_dma32 = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_pgalloc_dma && strcmp(name, "pgalloc_dma") == 0)) pgalloc_dma = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_pgalloc_movable && strcmp(name, "pgalloc_movable") == 0)) pgalloc_movable = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_pgalloc_normal && strcmp(name, "pgalloc_normal") == 0)) pgalloc_normal = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_pgdeactivate && strcmp(name, "pgdeactivate") == 0)) pgdeactivate = strtoull(value, NULL, 10);
        else if(unlikely(hash == hash_pgfault && strcmp(name, "pgfault") == 0)) pgfault = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_pgfree && strcmp(name, "pgfree") == 0)) pgfree = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_pginodesteal && strcmp(name, "pginodesteal") == 0)) pginodesteal = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_pglazyfreed && strcmp(name, "pglazyfreed") == 0)) pglazyfreed = strtoull(value, NULL, 10);
        else if(unlikely(hash == hash_pgmajfault && strcmp(name, "pgmajfault") == 0)) pgmajfault = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_pgmigrate_fail && strcmp(name, "pgmigrate_fail") == 0)) pgmigrate_fail = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_pgmigrate_success && strcmp(name, "pgmigrate_success") == 0)) pgmigrate_success = strtoull(value, NULL, 10);
        else if(unlikely(hash == hash_pgpgin && strcmp(name, "pgpgin") == 0)) pgpgin = strtoull(value, NULL, 10);
        else if(unlikely(hash == hash_pgpgout && strcmp(name, "pgpgout") == 0)) pgpgout = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_pgrefill && strcmp(name, "pgrefill") == 0)) pgrefill = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_pgrotated && strcmp(name, "pgrotated") == 0)) pgrotated = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_pgscan_direct && strcmp(name, "pgscan_direct") == 0)) pgscan_direct = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_pgscan_direct_throttle && strcmp(name, "pgscan_direct_throttle") == 0)) pgscan_direct_throttle = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_pgscan_kswapd && strcmp(name, "pgscan_kswapd") == 0)) pgscan_kswapd = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_pgskip_dma32 && strcmp(name, "pgskip_dma32") == 0)) pgskip_dma32 = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_pgskip_dma && strcmp(name, "pgskip_dma") == 0)) pgskip_dma = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_pgskip_movable && strcmp(name, "pgskip_movable") == 0)) pgskip_movable = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_pgskip_normal && strcmp(name, "pgskip_normal") == 0)) pgskip_normal = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_pgsteal_direct && strcmp(name, "pgsteal_direct") == 0)) pgsteal_direct = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_pgsteal_kswapd && strcmp(name, "pgsteal_kswapd") == 0)) pgsteal_kswapd = strtoull(value, NULL, 10);
        else if(unlikely(hash == hash_pswpin && strcmp(name, "pswpin") == 0)) pswpin = strtoull(value, NULL, 10);
        else if(unlikely(hash == hash_pswpout && strcmp(name, "pswpout") == 0)) pswpout = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_slabs_scanned && strcmp(name, "slabs_scanned") == 0)) slabs_scanned = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_thp_collapse_alloc_failed && strcmp(name, "thp_collapse_alloc_failed") == 0)) thp_collapse_alloc_failed = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_thp_collapse_alloc && strcmp(name, "thp_collapse_alloc") == 0)) thp_collapse_alloc = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_thp_deferred_split_page && strcmp(name, "thp_deferred_split_page") == 0)) thp_deferred_split_page = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_thp_fault_alloc && strcmp(name, "thp_fault_alloc") == 0)) thp_fault_alloc = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_thp_fault_fallback && strcmp(name, "thp_fault_fallback") == 0)) thp_fault_fallback = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_thp_file_alloc && strcmp(name, "thp_file_alloc") == 0)) thp_file_alloc = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_thp_file_mapped && strcmp(name, "thp_file_mapped") == 0)) thp_file_mapped = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_thp_split_page_failed && strcmp(name, "thp_split_page_failed") == 0)) thp_split_page_failed = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_thp_split_page && strcmp(name, "thp_split_page") == 0)) thp_split_page = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_thp_split_pmd && strcmp(name, "thp_split_pmd") == 0)) thp_split_pmd = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_thp_zero_page_alloc_failed && strcmp(name, "thp_zero_page_alloc_failed") == 0)) thp_zero_page_alloc_failed = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_thp_zero_page_alloc && strcmp(name, "thp_zero_page_alloc") == 0)) thp_zero_page_alloc = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_unevictable_pgs_cleared && strcmp(name, "unevictable_pgs_cleared") == 0)) unevictable_pgs_cleared = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_unevictable_pgs_culled && strcmp(name, "unevictable_pgs_culled") == 0)) unevictable_pgs_culled = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_unevictable_pgs_mlocked && strcmp(name, "unevictable_pgs_mlocked") == 0)) unevictable_pgs_mlocked = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_unevictable_pgs_munlocked && strcmp(name, "unevictable_pgs_munlocked") == 0)) unevictable_pgs_munlocked = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_unevictable_pgs_rescued && strcmp(name, "unevictable_pgs_rescued") == 0)) unevictable_pgs_rescued = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_unevictable_pgs_scanned && strcmp(name, "unevictable_pgs_scanned") == 0)) unevictable_pgs_scanned = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_unevictable_pgs_stranded && strcmp(name, "unevictable_pgs_stranded") == 0)) unevictable_pgs_stranded = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_workingset_activate && strcmp(name, "workingset_activate") == 0)) workingset_activate = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_workingset_nodereclaim && strcmp(name, "workingset_nodereclaim") == 0)) workingset_nodereclaim = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_workingset_refault && strcmp(name, "workingset_refault") == 0)) workingset_refault = strtoull(value, NULL, 10);
        // else if(unlikely(hash == hash_zone_reclaim_failed && strcmp(name, "zone_reclaim_failed") == 0)) zone_reclaim_failed = strtoull(value, NULL, 10);
    }

    // --------------------------------------------------------------------

    if(pswpin || pswpout || do_swapio == CONFIG_ONDEMAND_YES) {
        do_swapio = CONFIG_ONDEMAND_YES;

        static RRDSET *st_swapio = NULL;
        if(unlikely(!st_swapio)) {
            st_swapio = rrdset_create("system", "swapio", NULL, "swap", NULL, "Swap I/O", "kilobytes/s", 250, update_every, RRDSET_TYPE_AREA);

            rrddim_add(st_swapio, "in",  NULL, sysconf(_SC_PAGESIZE), 1024, RRDDIM_INCREMENTAL);
            rrddim_add(st_swapio, "out", NULL, -sysconf(_SC_PAGESIZE), 1024, RRDDIM_INCREMENTAL);
        }
        else rrdset_next(st_swapio);

        rrddim_set(st_swapio, "in", pswpin);
        rrddim_set(st_swapio, "out", pswpout);
        rrdset_done(st_swapio);
    }

    // --------------------------------------------------------------------

    if(do_io) {
        static RRDSET *st_io = NULL;
        if(unlikely(!st_io)) {
            st_io = rrdset_create("system", "io", NULL, "disk", NULL, "Disk I/O", "kilobytes/s", 150, update_every, RRDSET_TYPE_AREA);

            rrddim_add(st_io, "in",  NULL,  1, 1, RRDDIM_INCREMENTAL);
            rrddim_add(st_io, "out", NULL, -1, 1, RRDDIM_INCREMENTAL);
        }
        else rrdset_next(st_io);

        rrddim_set(st_io, "in", pgpgin);
        rrddim_set(st_io, "out", pgpgout);
        rrdset_done(st_io);
    }

    // --------------------------------------------------------------------

    if(do_pgfaults) {
        static RRDSET *st_pgfaults = NULL;
        if(unlikely(!st_pgfaults)) {
            st_pgfaults = rrdset_create("mem", "pgfaults", NULL, "system", NULL, "Memory Page Faults", "page faults/s", 500, update_every, RRDSET_TYPE_LINE);
            st_pgfaults->isdetail = 1;

            rrddim_add(st_pgfaults, "minor",  NULL,  1, 1, RRDDIM_INCREMENTAL);
            rrddim_add(st_pgfaults, "major", NULL, -1, 1, RRDDIM_INCREMENTAL);
        }
        else rrdset_next(st_pgfaults);

        rrddim_set(st_pgfaults, "minor", pgfault);
        rrddim_set(st_pgfaults, "major", pgmajfault);
        rrdset_done(st_pgfaults);
    }

    // --------------------------------------------------------------------

    // Ondemand criteria for NUMA. Since this won't change at run time, we
    // check it only once. We check whether the node count is >= 2 because
    // single-node systems have uninteresting statistics (since all accesses
    // are local).
    if(unlikely(has_numa == -1)) {
        has_numa = (get_numa_node_count() >= 2 &&
                    (numa_local || numa_foreign || numa_interleave || numa_other || numa_pte_updates ||
                     numa_huge_pte_updates || numa_hint_faults || numa_hint_faults_local || numa_pages_migrated)) ? 1 : 0;
    }

    if(do_numa == CONFIG_ONDEMAND_YES || (do_numa == CONFIG_ONDEMAND_ONDEMAND && has_numa)) {
        static RRDSET *st_numa = NULL;
        if(unlikely(!st_numa)) {
            st_numa = rrdset_create("mem", "numa", NULL, "numa", NULL, "NUMA events", "events/s", 800, update_every, RRDSET_TYPE_LINE);
            st_numa->isdetail = 1;

            // These depend on CONFIG_NUMA in the kernel.
            rrddim_add(st_numa, "local", NULL, 1, 1, RRDDIM_INCREMENTAL);
            rrddim_add(st_numa, "foreign", NULL, 1, 1, RRDDIM_INCREMENTAL);
            rrddim_add(st_numa, "interleave", NULL, 1, 1, RRDDIM_INCREMENTAL);
            rrddim_add(st_numa, "other", NULL, 1, 1, RRDDIM_INCREMENTAL);

            // The following stats depend on CONFIG_NUMA_BALANCING in the
            // kernel.
            rrddim_add(st_numa, "pte updates", NULL, 1, 1, RRDDIM_INCREMENTAL);
            rrddim_add(st_numa, "huge pte updates", NULL, 1, 1, RRDDIM_INCREMENTAL);
            rrddim_add(st_numa, "hint faults", NULL, 1, 1, RRDDIM_INCREMENTAL);
            rrddim_add(st_numa, "hint faults local", NULL, 1, 1, RRDDIM_INCREMENTAL);
            rrddim_add(st_numa, "pages migrated", NULL, 1, 1, RRDDIM_INCREMENTAL);
        }
        else rrdset_next(st_numa);

        rrddim_set(st_numa, "local", numa_local);
        rrddim_set(st_numa, "foreign", numa_foreign);
        rrddim_set(st_numa, "interleave", numa_interleave);
        rrddim_set(st_numa, "other", numa_other);

        rrddim_set(st_numa, "pte updates", numa_pte_updates);
        rrddim_set(st_numa, "huge pte updates", numa_huge_pte_updates);
        rrddim_set(st_numa, "hint faults", numa_hint_faults);
        rrddim_set(st_numa, "hint faults local", numa_hint_faults_local);
        rrddim_set(st_numa, "pages migrated", numa_pages_migrated);

        rrdset_done(st_numa);
    }

    return 0;
}


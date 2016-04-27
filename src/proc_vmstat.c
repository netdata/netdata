#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include "common.h"
#include "log.h"
#include "appconfig.h"
#include "procfile.h"
#include "rrd.h"
#include "plugin_proc.h"

int do_proc_vmstat(int update_every, unsigned long long dt) {
	static procfile *ff = NULL;
	static int do_swapio = -1, do_io = -1, do_pgfaults = -1, gen_hashes = -1;

	// static uint32_t hash_allocstall = -1;
	// static uint32_t hash_compact_blocks_moved = -1;
	// static uint32_t hash_compact_fail = -1;
	// static uint32_t hash_compact_pagemigrate_failed = -1;
	// static uint32_t hash_compact_pages_moved = -1;
	// static uint32_t hash_compact_stall = -1;
	// static uint32_t hash_compact_success = -1;
	// static uint32_t hash_htlb_buddy_alloc_fail = -1;
	// static uint32_t hash_htlb_buddy_alloc_success = -1;
	// static uint32_t hash_kswapd_high_wmark_hit_quickly = -1;
	// static uint32_t hash_kswapd_inodesteal = -1;
	// static uint32_t hash_kswapd_low_wmark_hit_quickly = -1;
	// static uint32_t hash_kswapd_skip_congestion_wait = -1;
	// static uint32_t hash_nr_active_anon = -1;
	// static uint32_t hash_nr_active_file = -1;
	// static uint32_t hash_nr_anon_pages = -1;
	// static uint32_t hash_nr_anon_transparent_hugepages = -1;
	// static uint32_t hash_nr_bounce = -1;
	// static uint32_t hash_nr_dirtied = -1;
	// static uint32_t hash_nr_dirty = -1;
	// static uint32_t hash_nr_dirty_background_threshold = -1;
	// static uint32_t hash_nr_dirty_threshold = -1;
	// static uint32_t hash_nr_file_pages = -1;
	// static uint32_t hash_nr_free_pages = -1;
	// static uint32_t hash_nr_inactive_anon = -1;
	// static uint32_t hash_nr_inactive_file = -1;
	// static uint32_t hash_nr_isolated_anon = -1;
	// static uint32_t hash_nr_isolated_file = -1;
	// static uint32_t hash_nr_kernel_stack = -1;
	// static uint32_t hash_nr_mapped = -1;
	// static uint32_t hash_nr_mlock = -1;
	// static uint32_t hash_nr_page_table_pages = -1;
	// static uint32_t hash_nr_shmem = -1;
	// static uint32_t hash_nr_slab_reclaimable = -1;
	// static uint32_t hash_nr_slab_unreclaimable = -1;
	// static uint32_t hash_nr_unevictable = -1;
	// static uint32_t hash_nr_unstable = -1;
	// static uint32_t hash_nr_vmscan_immediate_reclaim = -1;
	// static uint32_t hash_nr_vmscan_write = -1;
	// static uint32_t hash_nr_writeback = -1;
	// static uint32_t hash_nr_writeback_temp = -1;
	// static uint32_t hash_nr_written = -1;
	// static uint32_t hash_pageoutrun = -1;
	// static uint32_t hash_pgactivate = -1;
	// static uint32_t hash_pgalloc_dma = -1;
	// static uint32_t hash_pgalloc_dma32 = -1;
	// static uint32_t hash_pgalloc_movable = -1;
	// static uint32_t hash_pgalloc_normal = -1;
	// static uint32_t hash_pgdeactivate = -1;
	static uint32_t hash_pgfault = -1;
	// static uint32_t hash_pgfree = -1;
	// static uint32_t hash_pginodesteal = -1;
	static uint32_t hash_pgmajfault = -1;
	static uint32_t hash_pgpgin = -1;
	static uint32_t hash_pgpgout = -1;
	// static uint32_t hash_pgrefill_dma = -1;
	// static uint32_t hash_pgrefill_dma32 = -1;
	// static uint32_t hash_pgrefill_movable = -1;
	// static uint32_t hash_pgrefill_normal = -1;
	// static uint32_t hash_pgrotated = -1;
	// static uint32_t hash_pgscan_direct_dma = -1;
	// static uint32_t hash_pgscan_direct_dma32 = -1;
	// static uint32_t hash_pgscan_direct_movable = -1;
	// static uint32_t hash_pgscan_direct_normal = -1;
	// static uint32_t hash_pgscan_kswapd_dma = -1;
	// static uint32_t hash_pgscan_kswapd_dma32 = -1;
	// static uint32_t hash_pgscan_kswapd_movable = -1;
	// static uint32_t hash_pgscan_kswapd_normal = -1;
	// static uint32_t hash_pgsteal_direct_dma = -1;
	// static uint32_t hash_pgsteal_direct_dma32 = -1;
	// static uint32_t hash_pgsteal_direct_movable = -1;
	// static uint32_t hash_pgsteal_direct_normal = -1;
	// static uint32_t hash_pgsteal_kswapd_dma = -1;
	// static uint32_t hash_pgsteal_kswapd_dma32 = -1;
	// static uint32_t hash_pgsteal_kswapd_movable = -1;
	// static uint32_t hash_pgsteal_kswapd_normal = -1;
	static uint32_t hash_pswpin = -1;
	static uint32_t hash_pswpout = -1;
	// static uint32_t hash_slabs_scanned = -1;
	// static uint32_t hash_thp_collapse_alloc = -1;
	// static uint32_t hash_thp_collapse_alloc_failed = -1;
	// static uint32_t hash_thp_fault_alloc = -1;
	// static uint32_t hash_thp_fault_fallback = -1;
	// static uint32_t hash_thp_split = -1;
	// static uint32_t hash_unevictable_pgs_cleared = -1;
	// static uint32_t hash_unevictable_pgs_culled = -1;
	// static uint32_t hash_unevictable_pgs_mlocked = -1;
	// static uint32_t hash_unevictable_pgs_mlockfreed = -1;
	// static uint32_t hash_unevictable_pgs_munlocked = -1;
	// static uint32_t hash_unevictable_pgs_rescued = -1;
	// static uint32_t hash_unevictable_pgs_scanned = -1;
	// static uint32_t hash_unevictable_pgs_stranded = -1;

	if(gen_hashes != 1) {
		gen_hashes = 1;
		// hash_allocstall = simple_hash("allocstall");
		// hash_compact_blocks_moved = simple_hash("compact_blocks_moved");
		// hash_compact_fail = simple_hash("compact_fail");
		// hash_compact_pagemigrate_failed = simple_hash("compact_pagemigrate_failed");
		// hash_compact_pages_moved = simple_hash("compact_pages_moved");
		// hash_compact_stall = simple_hash("compact_stall");
		// hash_compact_success = simple_hash("compact_success");
		// hash_htlb_buddy_alloc_fail = simple_hash("htlb_buddy_alloc_fail");
		// hash_htlb_buddy_alloc_success = simple_hash("htlb_buddy_alloc_success");
		// hash_kswapd_high_wmark_hit_quickly = simple_hash("kswapd_high_wmark_hit_quickly");
		// hash_kswapd_inodesteal = simple_hash("kswapd_inodesteal");
		// hash_kswapd_low_wmark_hit_quickly = simple_hash("kswapd_low_wmark_hit_quickly");
		// hash_kswapd_skip_congestion_wait = simple_hash("kswapd_skip_congestion_wait");
		// hash_nr_active_anon = simple_hash("nr_active_anon");
		// hash_nr_active_file = simple_hash("nr_active_file");
		// hash_nr_anon_pages = simple_hash("nr_anon_pages");
		// hash_nr_anon_transparent_hugepages = simple_hash("nr_anon_transparent_hugepages");
		// hash_nr_bounce = simple_hash("nr_bounce");
		// hash_nr_dirtied = simple_hash("nr_dirtied");
		// hash_nr_dirty = simple_hash("nr_dirty");
		// hash_nr_dirty_background_threshold = simple_hash("nr_dirty_background_threshold");
		// hash_nr_dirty_threshold = simple_hash("nr_dirty_threshold");
		// hash_nr_file_pages = simple_hash("nr_file_pages");
		// hash_nr_free_pages = simple_hash("nr_free_pages");
		// hash_nr_inactive_anon = simple_hash("nr_inactive_anon");
		// hash_nr_inactive_file = simple_hash("nr_inactive_file");
		// hash_nr_isolated_anon = simple_hash("nr_isolated_anon");
		// hash_nr_isolated_file = simple_hash("nr_isolated_file");
		// hash_nr_kernel_stack = simple_hash("nr_kernel_stack");
		// hash_nr_mapped = simple_hash("nr_mapped");
		// hash_nr_mlock = simple_hash("nr_mlock");
		// hash_nr_page_table_pages = simple_hash("nr_page_table_pages");
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
		// hash_pageoutrun = simple_hash("pageoutrun");
		// hash_pgactivate = simple_hash("pgactivate");
		// hash_pgalloc_dma = simple_hash("pgalloc_dma");
		// hash_pgalloc_dma32 = simple_hash("pgalloc_dma32");
		// hash_pgalloc_movable = simple_hash("pgalloc_movable");
		// hash_pgalloc_normal = simple_hash("pgalloc_normal");
		// hash_pgdeactivate = simple_hash("pgdeactivate");
		hash_pgfault = simple_hash("pgfault");
		// hash_pgfree = simple_hash("pgfree");
		// hash_pginodesteal = simple_hash("pginodesteal");
		hash_pgmajfault = simple_hash("pgmajfault");
		hash_pgpgin = simple_hash("pgpgin");
		hash_pgpgout = simple_hash("pgpgout");
		// hash_pgrefill_dma = simple_hash("pgrefill_dma");
		// hash_pgrefill_dma32 = simple_hash("pgrefill_dma32");
		// hash_pgrefill_movable = simple_hash("pgrefill_movable");
		// hash_pgrefill_normal = simple_hash("pgrefill_normal");
		// hash_pgrotated = simple_hash("pgrotated");
		// hash_pgscan_direct_dma = simple_hash("pgscan_direct_dma");
		// hash_pgscan_direct_dma32 = simple_hash("pgscan_direct_dma32");
		// hash_pgscan_direct_movable = simple_hash("pgscan_direct_movable");
		// hash_pgscan_direct_normal = simple_hash("pgscan_direct_normal");
		// hash_pgscan_kswapd_dma = simple_hash("pgscan_kswapd_dma");
		// hash_pgscan_kswapd_dma32 = simple_hash("pgscan_kswapd_dma32");
		// hash_pgscan_kswapd_movable = simple_hash("pgscan_kswapd_movable");
		// hash_pgscan_kswapd_normal = simple_hash("pgscan_kswapd_normal");
		// hash_pgsteal_direct_dma = simple_hash("pgsteal_direct_dma");
		// hash_pgsteal_direct_dma32 = simple_hash("pgsteal_direct_dma32");
		// hash_pgsteal_direct_movable = simple_hash("pgsteal_direct_movable");
		// hash_pgsteal_direct_normal = simple_hash("pgsteal_direct_normal");
		// hash_pgsteal_kswapd_dma = simple_hash("pgsteal_kswapd_dma");
		// hash_pgsteal_kswapd_dma32 = simple_hash("pgsteal_kswapd_dma32");
		// hash_pgsteal_kswapd_movable = simple_hash("pgsteal_kswapd_movable");
		// hash_pgsteal_kswapd_normal = simple_hash("pgsteal_kswapd_normal");
		hash_pswpin = simple_hash("pswpin");
		hash_pswpout = simple_hash("pswpout");
		// hash_slabs_scanned = simple_hash("slabs_scanned");
		// hash_thp_collapse_alloc = simple_hash("thp_collapse_alloc");
		// hash_thp_collapse_alloc_failed = simple_hash("thp_collapse_alloc_failed");
		// hash_thp_fault_alloc = simple_hash("thp_fault_alloc");
		// hash_thp_fault_fallback = simple_hash("thp_fault_fallback");
		// hash_thp_split = simple_hash("thp_split");
		// hash_unevictable_pgs_cleared = simple_hash("unevictable_pgs_cleared");
		// hash_unevictable_pgs_culled = simple_hash("unevictable_pgs_culled");
		// hash_unevictable_pgs_mlocked = simple_hash("unevictable_pgs_mlocked");
		// hash_unevictable_pgs_mlockfreed = simple_hash("unevictable_pgs_mlockfreed");
		// hash_unevictable_pgs_munlocked = simple_hash("unevictable_pgs_munlocked");
		// hash_unevictable_pgs_rescued = simple_hash("unevictable_pgs_rescued");
		// hash_unevictable_pgs_scanned = simple_hash("unevictable_pgs_scanned");
		// hash_unevictable_pgs_stranded = simple_hash("unevictable_pgs_stranded");
	}

	if(do_swapio == -1)	do_swapio = config_get_boolean("plugin:proc:/proc/vmstat", "swap i/o", 1);
	if(do_io == -1)		do_io = config_get_boolean("plugin:proc:/proc/vmstat", "disk i/o", 1);
	if(do_pgfaults == -1)	do_pgfaults = config_get_boolean("plugin:proc:/proc/vmstat", "memory page faults", 1);

	if(dt) {};

	if(!ff) {
		char filename[FILENAME_MAX + 1];
		snprintfz(filename, FILENAME_MAX, "%s%s", global_host_prefix, "/proc/vmstat");
		ff = procfile_open(config_get("plugin:proc:/proc/vmstat", "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
	}
	if(!ff) return 1;

	ff = procfile_readall(ff);
	if(!ff) return 0; // we return 0, so that we will retry to open it next time

	uint32_t lines = procfile_lines(ff), l;
	uint32_t words;

	// unsigned long long allocstall = 0ULL;
	// unsigned long long compact_blocks_moved = 0ULL;
	// unsigned long long compact_fail = 0ULL;
	// unsigned long long compact_pagemigrate_failed = 0ULL;
	// unsigned long long compact_pages_moved = 0ULL;
	// unsigned long long compact_stall = 0ULL;
	// unsigned long long compact_success = 0ULL;
	// unsigned long long htlb_buddy_alloc_fail = 0ULL;
	// unsigned long long htlb_buddy_alloc_success = 0ULL;
	// unsigned long long kswapd_high_wmark_hit_quickly = 0ULL;
	// unsigned long long kswapd_inodesteal = 0ULL;
	// unsigned long long kswapd_low_wmark_hit_quickly = 0ULL;
	// unsigned long long kswapd_skip_congestion_wait = 0ULL;
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
	// unsigned long long nr_free_pages = 0ULL;
	// unsigned long long nr_inactive_anon = 0ULL;
	// unsigned long long nr_inactive_file = 0ULL;
	// unsigned long long nr_isolated_anon = 0ULL;
	// unsigned long long nr_isolated_file = 0ULL;
	// unsigned long long nr_kernel_stack = 0ULL;
	// unsigned long long nr_mapped = 0ULL;
	// unsigned long long nr_mlock = 0ULL;
	// unsigned long long nr_page_table_pages = 0ULL;
	// unsigned long long nr_shmem = 0ULL;
	// unsigned long long nr_slab_reclaimable = 0ULL;
	// unsigned long long nr_slab_unreclaimable = 0ULL;
	// unsigned long long nr_unevictable = 0ULL;
	// unsigned long long nr_unstable = 0ULL;
	// unsigned long long nr_vmscan_immediate_reclaim = 0ULL;
	// unsigned long long nr_vmscan_write = 0ULL;
	// unsigned long long nr_writeback = 0ULL;
	// unsigned long long nr_writeback_temp = 0ULL;
	// unsigned long long nr_written = 0ULL;
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
	unsigned long long pgmajfault = 0ULL;
	unsigned long long pgpgin = 0ULL;
	unsigned long long pgpgout = 0ULL;
	// unsigned long long pgrefill_dma = 0ULL;
	// unsigned long long pgrefill_dma32 = 0ULL;
	// unsigned long long pgrefill_movable = 0ULL;
	// unsigned long long pgrefill_normal = 0ULL;
	// unsigned long long pgrotated = 0ULL;
	// unsigned long long pgscan_direct_dma = 0ULL;
	// unsigned long long pgscan_direct_dma32 = 0ULL;
	// unsigned long long pgscan_direct_movable = 0ULL;
	// unsigned long long pgscan_direct_normal = 0ULL;
	// unsigned long long pgscan_kswapd_dma = 0ULL;
	// unsigned long long pgscan_kswapd_dma32 = 0ULL;
	// unsigned long long pgscan_kswapd_movable = 0ULL;
	// unsigned long long pgscan_kswapd_normal = 0ULL;
	// unsigned long long pgsteal_direct_dma = 0ULL;
	// unsigned long long pgsteal_direct_dma32 = 0ULL;
	// unsigned long long pgsteal_direct_movable = 0ULL;
	// unsigned long long pgsteal_direct_normal = 0ULL;
	// unsigned long long pgsteal_kswapd_dma = 0ULL;
	// unsigned long long pgsteal_kswapd_dma32 = 0ULL;
	// unsigned long long pgsteal_kswapd_movable = 0ULL;
	// unsigned long long pgsteal_kswapd_normal = 0ULL;
	unsigned long long pswpin = 0ULL;
	unsigned long long pswpout = 0ULL;
	// unsigned long long slabs_scanned = 0ULL;
	// unsigned long long thp_collapse_alloc = 0ULL;
	// unsigned long long thp_collapse_alloc_failed = 0ULL;
	// unsigned long long thp_fault_alloc = 0ULL;
	// unsigned long long thp_fault_fallback = 0ULL;
	// unsigned long long thp_split = 0ULL;
	// unsigned long long unevictable_pgs_cleared = 0ULL;
	// unsigned long long unevictable_pgs_culled = 0ULL;
	// unsigned long long unevictable_pgs_mlocked = 0ULL;
	// unsigned long long unevictable_pgs_mlockfreed = 0ULL;
	// unsigned long long unevictable_pgs_munlocked = 0ULL;
	// unsigned long long unevictable_pgs_rescued = 0ULL;
	// unsigned long long unevictable_pgs_scanned = 0ULL;
	// unsigned long long unevictable_pgs_stranded = 0ULL;

	for(l = 0; l < lines ;l++) {
		words = procfile_linewords(ff, l);
		if(words < 2) {
			if(words) error("Cannot read /proc/vmstat line %d. Expected 2 params, read %d.", l, words);
			continue;
		}

		char *name = procfile_lineword(ff, l, 0);
		char * value = procfile_lineword(ff, l, 1);
		if(!name || !*name || !value || !*value) continue;

		uint32_t hash = simple_hash(name);

		if(0) ;
		// else if(hash == hash_allocstall && strcmp(name, "allocstall") == 0) allocstall = strtoull(value, NULL, 10);
		// else if(hash == hash_compact_blocks_moved && strcmp(name, "compact_blocks_moved") == 0) compact_blocks_moved = strtoull(value, NULL, 10);
		// else if(hash == hash_compact_fail && strcmp(name, "compact_fail") == 0) compact_fail = strtoull(value, NULL, 10);
		// else if(hash == hash_compact_pagemigrate_failed && strcmp(name, "compact_pagemigrate_failed") == 0) compact_pagemigrate_failed = strtoull(value, NULL, 10);
		// else if(hash == hash_compact_pages_moved && strcmp(name, "compact_pages_moved") == 0) compact_pages_moved = strtoull(value, NULL, 10);
		// else if(hash == hash_compact_stall && strcmp(name, "compact_stall") == 0) compact_stall = strtoull(value, NULL, 10);
		// else if(hash == hash_compact_success && strcmp(name, "compact_success") == 0) compact_success = strtoull(value, NULL, 10);
		// else if(hash == hash_htlb_buddy_alloc_fail && strcmp(name, "htlb_buddy_alloc_fail") == 0) htlb_buddy_alloc_fail = strtoull(value, NULL, 10);
		// else if(hash == hash_htlb_buddy_alloc_success && strcmp(name, "htlb_buddy_alloc_success") == 0) htlb_buddy_alloc_success = strtoull(value, NULL, 10);
		// else if(hash == hash_kswapd_high_wmark_hit_quickly && strcmp(name, "kswapd_high_wmark_hit_quickly") == 0) kswapd_high_wmark_hit_quickly = strtoull(value, NULL, 10);
		// else if(hash == hash_kswapd_inodesteal && strcmp(name, "kswapd_inodesteal") == 0) kswapd_inodesteal = strtoull(value, NULL, 10);
		// else if(hash == hash_kswapd_low_wmark_hit_quickly && strcmp(name, "kswapd_low_wmark_hit_quickly") == 0) kswapd_low_wmark_hit_quickly = strtoull(value, NULL, 10);
		// else if(hash == hash_kswapd_skip_congestion_wait && strcmp(name, "kswapd_skip_congestion_wait") == 0) kswapd_skip_congestion_wait = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_active_anon && strcmp(name, "nr_active_anon") == 0) nr_active_anon = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_active_file && strcmp(name, "nr_active_file") == 0) nr_active_file = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_anon_pages && strcmp(name, "nr_anon_pages") == 0) nr_anon_pages = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_anon_transparent_hugepages && strcmp(name, "nr_anon_transparent_hugepages") == 0) nr_anon_transparent_hugepages = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_bounce && strcmp(name, "nr_bounce") == 0) nr_bounce = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_dirtied && strcmp(name, "nr_dirtied") == 0) nr_dirtied = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_dirty && strcmp(name, "nr_dirty") == 0) nr_dirty = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_dirty_background_threshold && strcmp(name, "nr_dirty_background_threshold") == 0) nr_dirty_background_threshold = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_dirty_threshold && strcmp(name, "nr_dirty_threshold") == 0) nr_dirty_threshold = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_file_pages && strcmp(name, "nr_file_pages") == 0) nr_file_pages = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_free_pages && strcmp(name, "nr_free_pages") == 0) nr_free_pages = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_inactive_anon && strcmp(name, "nr_inactive_anon") == 0) nr_inactive_anon = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_inactive_file && strcmp(name, "nr_inactive_file") == 0) nr_inactive_file = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_isolated_anon && strcmp(name, "nr_isolated_anon") == 0) nr_isolated_anon = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_isolated_file && strcmp(name, "nr_isolated_file") == 0) nr_isolated_file = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_kernel_stack && strcmp(name, "nr_kernel_stack") == 0) nr_kernel_stack = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_mapped && strcmp(name, "nr_mapped") == 0) nr_mapped = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_mlock && strcmp(name, "nr_mlock") == 0) nr_mlock = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_page_table_pages && strcmp(name, "nr_page_table_pages") == 0) nr_page_table_pages = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_shmem && strcmp(name, "nr_shmem") == 0) nr_shmem = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_slab_reclaimable && strcmp(name, "nr_slab_reclaimable") == 0) nr_slab_reclaimable = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_slab_unreclaimable && strcmp(name, "nr_slab_unreclaimable") == 0) nr_slab_unreclaimable = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_unevictable && strcmp(name, "nr_unevictable") == 0) nr_unevictable = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_unstable && strcmp(name, "nr_unstable") == 0) nr_unstable = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_vmscan_immediate_reclaim && strcmp(name, "nr_vmscan_immediate_reclaim") == 0) nr_vmscan_immediate_reclaim = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_vmscan_write && strcmp(name, "nr_vmscan_write") == 0) nr_vmscan_write = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_writeback && strcmp(name, "nr_writeback") == 0) nr_writeback = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_writeback_temp && strcmp(name, "nr_writeback_temp") == 0) nr_writeback_temp = strtoull(value, NULL, 10);
		// else if(hash == hash_nr_written && strcmp(name, "nr_written") == 0) nr_written = strtoull(value, NULL, 10);
		// else if(hash == hash_pageoutrun && strcmp(name, "pageoutrun") == 0) pageoutrun = strtoull(value, NULL, 10);
		// else if(hash == hash_pgactivate && strcmp(name, "pgactivate") == 0) pgactivate = strtoull(value, NULL, 10);
		// else if(hash == hash_pgalloc_dma && strcmp(name, "pgalloc_dma") == 0) pgalloc_dma = strtoull(value, NULL, 10);
		// else if(hash == hash_pgalloc_dma32 && strcmp(name, "pgalloc_dma32") == 0) pgalloc_dma32 = strtoull(value, NULL, 10);
		// else if(hash == hash_pgalloc_movable && strcmp(name, "pgalloc_movable") == 0) pgalloc_movable = strtoull(value, NULL, 10);
		// else if(hash == hash_pgalloc_normal && strcmp(name, "pgalloc_normal") == 0) pgalloc_normal = strtoull(value, NULL, 10);
		// else if(hash == hash_pgdeactivate && strcmp(name, "pgdeactivate") == 0) pgdeactivate = strtoull(value, NULL, 10);
		else if(hash == hash_pgfault && strcmp(name, "pgfault") == 0) pgfault = strtoull(value, NULL, 10);
		// else if(hash == hash_pgfree && strcmp(name, "pgfree") == 0) pgfree = strtoull(value, NULL, 10);
		// else if(hash == hash_pginodesteal && strcmp(name, "pginodesteal") == 0) pginodesteal = strtoull(value, NULL, 10);
		else if(hash == hash_pgmajfault && strcmp(name, "pgmajfault") == 0) pgmajfault = strtoull(value, NULL, 10);
		else if(hash == hash_pgpgin && strcmp(name, "pgpgin") == 0) pgpgin = strtoull(value, NULL, 10);
		else if(hash == hash_pgpgout && strcmp(name, "pgpgout") == 0) pgpgout = strtoull(value, NULL, 10);
		// else if(hash == hash_pgrefill_dma && strcmp(name, "pgrefill_dma") == 0) pgrefill_dma = strtoull(value, NULL, 10);
		// else if(hash == hash_pgrefill_dma32 && strcmp(name, "pgrefill_dma32") == 0) pgrefill_dma32 = strtoull(value, NULL, 10);
		// else if(hash == hash_pgrefill_movable && strcmp(name, "pgrefill_movable") == 0) pgrefill_movable = strtoull(value, NULL, 10);
		// else if(hash == hash_pgrefill_normal && strcmp(name, "pgrefill_normal") == 0) pgrefill_normal = strtoull(value, NULL, 10);
		// else if(hash == hash_pgrotated && strcmp(name, "pgrotated") == 0) pgrotated = strtoull(value, NULL, 10);
		// else if(hash == hash_pgscan_direct_dma && strcmp(name, "pgscan_direct_dma") == 0) pgscan_direct_dma = strtoull(value, NULL, 10);
		// else if(hash == hash_pgscan_direct_dma32 && strcmp(name, "pgscan_direct_dma32") == 0) pgscan_direct_dma32 = strtoull(value, NULL, 10);
		// else if(hash == hash_pgscan_direct_movable && strcmp(name, "pgscan_direct_movable") == 0) pgscan_direct_movable = strtoull(value, NULL, 10);
		// else if(hash == hash_pgscan_direct_normal && strcmp(name, "pgscan_direct_normal") == 0) pgscan_direct_normal = strtoull(value, NULL, 10);
		// else if(hash == hash_pgscan_kswapd_dma && strcmp(name, "pgscan_kswapd_dma") == 0) pgscan_kswapd_dma = strtoull(value, NULL, 10);
		// else if(hash == hash_pgscan_kswapd_dma32 && strcmp(name, "pgscan_kswapd_dma32") == 0) pgscan_kswapd_dma32 = strtoull(value, NULL, 10);
		// else if(hash == hash_pgscan_kswapd_movable && strcmp(name, "pgscan_kswapd_movable") == 0) pgscan_kswapd_movable = strtoull(value, NULL, 10);
		// else if(hash == hash_pgscan_kswapd_normal && strcmp(name, "pgscan_kswapd_normal") == 0) pgscan_kswapd_normal = strtoull(value, NULL, 10);
		// else if(hash == hash_pgsteal_direct_dma && strcmp(name, "pgsteal_direct_dma") == 0) pgsteal_direct_dma = strtoull(value, NULL, 10);
		// else if(hash == hash_pgsteal_direct_dma32 && strcmp(name, "pgsteal_direct_dma32") == 0) pgsteal_direct_dma32 = strtoull(value, NULL, 10);
		// else if(hash == hash_pgsteal_direct_movable && strcmp(name, "pgsteal_direct_movable") == 0) pgsteal_direct_movable = strtoull(value, NULL, 10);
		// else if(hash == hash_pgsteal_direct_normal && strcmp(name, "pgsteal_direct_normal") == 0) pgsteal_direct_normal = strtoull(value, NULL, 10);
		// else if(hash == hash_pgsteal_kswapd_dma && strcmp(name, "pgsteal_kswapd_dma") == 0) pgsteal_kswapd_dma = strtoull(value, NULL, 10);
		// else if(hash == hash_pgsteal_kswapd_dma32 && strcmp(name, "pgsteal_kswapd_dma32") == 0) pgsteal_kswapd_dma32 = strtoull(value, NULL, 10);
		// else if(hash == hash_pgsteal_kswapd_movable && strcmp(name, "pgsteal_kswapd_movable") == 0) pgsteal_kswapd_movable = strtoull(value, NULL, 10);
		// else if(hash == hash_pgsteal_kswapd_normal && strcmp(name, "pgsteal_kswapd_normal") == 0) pgsteal_kswapd_normal = strtoull(value, NULL, 10);
		else if(hash == hash_pswpin && strcmp(name, "pswpin") == 0) pswpin = strtoull(value, NULL, 10);
		else if(hash == hash_pswpout && strcmp(name, "pswpout") == 0) pswpout = strtoull(value, NULL, 10);
		// else if(hash == hash_slabs_scanned && strcmp(name, "slabs_scanned") == 0) slabs_scanned = strtoull(value, NULL, 10);
		// else if(hash == hash_thp_collapse_alloc && strcmp(name, "thp_collapse_alloc") == 0) thp_collapse_alloc = strtoull(value, NULL, 10);
		// else if(hash == hash_thp_collapse_alloc_failed && strcmp(name, "thp_collapse_alloc_failed") == 0) thp_collapse_alloc_failed = strtoull(value, NULL, 10);
		// else if(hash == hash_thp_fault_alloc && strcmp(name, "thp_fault_alloc") == 0) thp_fault_alloc = strtoull(value, NULL, 10);
		// else if(hash == hash_thp_fault_fallback && strcmp(name, "thp_fault_fallback") == 0) thp_fault_fallback = strtoull(value, NULL, 10);
		// else if(hash == hash_thp_split && strcmp(name, "thp_split") == 0) thp_split = strtoull(value, NULL, 10);
		// else if(hash == hash_unevictable_pgs_cleared && strcmp(name, "unevictable_pgs_cleared") == 0) unevictable_pgs_cleared = strtoull(value, NULL, 10);
		// else if(hash == hash_unevictable_pgs_culled && strcmp(name, "unevictable_pgs_culled") == 0) unevictable_pgs_culled = strtoull(value, NULL, 10);
		// else if(hash == hash_unevictable_pgs_mlocked && strcmp(name, "unevictable_pgs_mlocked") == 0) unevictable_pgs_mlocked = strtoull(value, NULL, 10);
		// else if(hash == hash_unevictable_pgs_mlockfreed && strcmp(name, "unevictable_pgs_mlockfreed") == 0) unevictable_pgs_mlockfreed = strtoull(value, NULL, 10);
		// else if(hash == hash_unevictable_pgs_munlocked && strcmp(name, "unevictable_pgs_munlocked") == 0) unevictable_pgs_munlocked = strtoull(value, NULL, 10);
		// else if(hash == hash_unevictable_pgs_rescued && strcmp(name, "unevictable_pgs_rescued") == 0) unevictable_pgs_rescued = strtoull(value, NULL, 10);
		// else if(hash == hash_unevictable_pgs_scanned && strcmp(name, "unevictable_pgs_scanned") == 0) unevictable_pgs_scanned = strtoull(value, NULL, 10);
		// else if(hash == hash_unevictable_pgs_stranded && strcmp(name, "unevictable_pgs_stranded") == 0) unevictable_pgs_stranded = strtoull(value, NULL, 10);
	}

	// --------------------------------------------------------------------

	if(do_swapio) {
		static RRDSET *st_swapio = NULL;
		if(!st_swapio) {
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
		if(!st_io) {
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
		if(!st_pgfaults) {
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

	return 0;
}

